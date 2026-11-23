import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import test from "node:test";
import YAML from "yaml";
import {
  type Gh,
  type GhResult,
  TITLE,
  openOrUpdate,
  reminder,
  reminderBody,
} from "./ci-reminders.ts";

const RUN_URL = "https://github.com/owner/repo/actions/runs/42";
const ENV = { GH_TOKEN: "token", RUN_URL };

interface Call {
  args: string[];
  env: NodeJS.ProcessEnv;
}

// A fake gh: `open` is the number of the open tracking issue, or undefined when there is none.
function fakeGh(
  options: {
    open?: string;
    failOn?: string;
  } = {},
): { gh: Gh; calls: Call[] } {
  const calls: Call[] = [];
  const gh: Gh = (args, env): GhResult => {
    calls.push({ args, env });
    if (options.failOn === args[1]) {
      return { status: 1, stdout: "", stderr: `boom ${args[1]}` };
    }
    if (args[1] === "list") {
      return {
        status: 0,
        stdout: options.open ? `${options.open}\n` : "\n",
        stderr: "",
      };
    }
    return {
      status: 0,
      stdout: "https://github.com/owner/repo/issues/9\n",
      stderr: "",
    };
  };
  return { gh, calls };
}

test("no open tracking issue creates one with the current title and body", () => {
  const { gh, calls } = fakeGh();
  const message = openOrUpdate(ENV, gh);

  assert.equal(calls.length, 2);
  assert.deepEqual(calls[0]!.args, [
    "issue",
    "list",
    "--state",
    "open",
    "--search",
    `in:title "${TITLE}"`,
    "--json",
    "number",
    "--jq",
    ".[0].number // empty",
  ]);
  assert.deepEqual(calls[1]!.args, [
    "issue",
    "create",
    "--title",
    "Surface upstream-docs review due",
    "--body",
    reminderBody(RUN_URL),
  ]);
  assert.equal(message, "https://github.com/owner/repo/issues/9");
});

test("an open tracking issue gets a comment with the body and no new issue", () => {
  const { gh, calls } = fakeGh({ open: "7" });
  const message = openOrUpdate(ENV, gh);

  assert.equal(calls.length, 2);
  assert.deepEqual(calls[1]!.args, [
    "issue",
    "comment",
    "7",
    "--body",
    reminderBody(RUN_URL),
  ]);
  assert.ok(!calls.some((call) => call.args[1] === "create"));
  assert.equal(message, "Commented on existing issue #7");
});

test("the body names the run and the 30-day review budget", () => {
  const body = reminderBody(RUN_URL);
  assert.ok(body.includes("over the 30-day review budget."));
  assert.ok(body.includes("Run the review playbook in that file"));
  assert.ok(
    body.endsWith(
      `Failure detail: ${RUN_URL} (see the "Run staleness check" step log).`,
    ),
  );
});

test("gh runs with the token from the environment", () => {
  const { gh, calls } = fakeGh({ open: "3" });
  openOrUpdate(ENV, gh);
  for (const call of calls) assert.equal(call.env.GH_TOKEN, "token");
});

test("a failed lookup or write throws an error that names the module and writes nothing further", () => {
  const listFails = fakeGh({ failOn: "list" });
  assert.throws(
    () => openOrUpdate(ENV, listFails.gh),
    (error: unknown) =>
      error instanceof Error &&
      error.message.startsWith("ci-reminders: gh issue list failed: boom list"),
  );
  assert.equal(listFails.calls.length, 1);

  const createFails = fakeGh({ failOn: "create" });
  assert.throws(() => openOrUpdate(ENV, createFails.gh), /boom create/);

  const commentFails = fakeGh({ open: "5", failOn: "comment" });
  assert.throws(() => openOrUpdate(ENV, commentFails.gh), /boom comment/);
});

test("the entry point exits 0 with the message on success and 1 with the prefixed error on failure", () => {
  const ok = reminder(ENV, fakeGh().gh);
  assert.equal(ok.exitCode, 0);
  assert.equal(ok.stdout, "https://github.com/owner/repo/issues/9\n");
  assert.equal(ok.stderr, "");

  const failed = reminder(ENV, fakeGh({ failOn: "list" }).gh);
  assert.equal(failed.exitCode, 1);
  assert.equal(failed.stdout, "");
  assert.match(failed.stderr, /^ci-reminders: gh issue list failed: boom list/);
});

test("the workflow step runs this module after the conformance action, with the token and run URL in its env", () => {
  const file = path.resolve(
    import.meta.dirname,
    "../../../../.github/workflows/reminders.yml",
  );
  const workflow = YAML.parse(fs.readFileSync(file, "utf8"));
  const steps: {
    name?: string;
    uses?: string;
    run?: string;
    env?: Record<string, string>;
  }[] = workflow.jobs["check-ledger-staleness"].steps;

  const conformance = steps.findIndex((s) =>
    s.uses?.startsWith("$/.github/actions/conformance"),
  );
  const step = steps.findIndex(
    (s) => s.name === "Open or update tracking issue",
  );
  assert.ok(conformance >= 0, "conformance action step missing");
  assert.ok(step > conformance, "tracking issue step must follow conformance");
  assert.equal(
    steps[step]!.run?.trim(),
    "node .github/tools/src/scripts/ci-reminders.ts",
  );
  assert.ok(steps[step]!.env && "GH_TOKEN" in steps[step]!.env);
  assert.ok(steps[step]!.env && "RUN_URL" in steps[step]!.env);
});
