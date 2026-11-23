import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import test from "node:test";
import { Script } from "node:vm";
import {
  inlineNodeScript,
  lookupWorkflowStep,
  type WorkflowStepData,
} from "./workflow-steps.ts";

const ROOT = path.resolve(import.meta.dirname, "../../../../..");
const VALIDATE = ".github/workflows/validate.yml";
const METADATA = "pr-metadata";
const TITLE = "Check the PR title";
const MARKERS = "Check the release markers";
const DESCRIPTION = "Check the change description";
const REMINDERS = ".github/workflows/reminders.yml";
const validate = readFileSync(path.join(ROOT, VALIDATE), "utf8").replaceAll(
  "\r\n",
  "\n",
);

test("a matching name in an earlier job cannot replace the selected workflow step", () => {
  const source = validate.replace(
    "jobs:\n",
    `jobs:\n  decoy:\n    steps:\n      - name: ${TITLE}\n        run: exit 73\n`,
  );
  assert.notEqual(source, validate);
  assert.match(
    lookupWorkflowStep(source, VALIDATE, METADATA, TITLE).step.run!,
    /^node <<'NODE'\n/,
  );
  assert.equal(
    lookupWorkflowStep(source, VALIDATE, "decoy", TITLE).step.run,
    "exit 73",
  );
});

const event = {
  title: "${{ github.event.pull_request.title }}",
  body: "${{ github.event.pull_request.body }}",
  author: "${{ github.event.pull_request.user.login }}",
};

function runMetadata(
  name: string,
  env: Record<string, string>,
  directory: string,
  expectedBindings: Record<string, string>,
) {
  const selected = lookupWorkflowStep(validate, VALIDATE, METADATA, name);
  assert.deepEqual(selected.job.permissions, {});
  assert.equal(selected.job.if, "github.event_name == 'pull_request'");
  const actionSteps = (selected.job.steps as WorkflowStepData[]).flatMap(
    (step) => (step.uses ? [step.uses] : []),
  );
  assert.equal(actionSteps.length, 1);
  assert.match(actionSteps[0]!, /^actions\/setup-node@[0-9a-f]{40}$/);
  assert.deepEqual(selected.step.env, expectedBindings);
  assert.doesNotMatch(selected.step.run!, /\$\{\{|github\.event/);

  // GitHub's quoted heredoc starts Node with these exact bytes on stdin.
  const result = spawnSync(process.execPath, [], {
    cwd: directory,
    env,
    input: inlineNodeScript(selected) + "\n",
    encoding: "utf8",
  });
  assert.ifError(result.error);
  return {
    status: result.status,
    stdout: result.stdout,
    stderr: result.stderr,
  };
}

test("the checkout-free metadata job executes the inline rules with event data in env only", (t) => {
  const directory = mkdtempSync(
    path.join(tmpdir(), "agentics-metadata-rules-"),
  );
  t.after(() => rmSync(directory, { recursive: true, force: true }));
  const output = path.join(directory, "github-output");
  const title = 'fix!: handle $(exit 73) `exit 74` "quotes"';
  const body =
    "## What changed\r\n\r\nUse $(exit 73) as text.\r\n\r\nBREAKING CHANGE: data\r\n";
  assert.deepEqual(
    runMetadata(TITLE, { PR_TITLE: title }, directory, {
      PR_TITLE: event.title,
    }),
    {
      status: 0,
      stdout: "PR title: Conventional Commits format accepted.\n",
      stderr: "",
    },
  );
  assert.deepEqual(
    runMetadata(
      MARKERS,
      {
        PR_TITLE: title,
        PR_BODY: body,
        GITHUB_OUTPUT: output,
      },
      directory,
      { PR_TITLE: event.title, PR_BODY: event.body },
    ),
    {
      status: 0,
      stdout: "Release level: major\n",
      stderr: "",
    },
  );
  assert.equal(readFileSync(output, "utf8"), "level=major\n");
  assert.deepEqual(
    runMetadata(
      MARKERS,
      {
        PR_TITLE: "fix: handle input",
        PR_BODY: body,
        GITHUB_OUTPUT: output,
      },
      directory,
      { PR_TITLE: event.title, PR_BODY: event.body },
    ),
    {
      status: 1,
      stdout:
        "::error::A BREAKING CHANGE footer needs a ! in the title, so a major release is visible in review. See CONTRIBUTING.md: Commit Message Convention.\n",
      stderr: "",
    },
  );
  assert.equal(readFileSync(output, "utf8"), "level=major\n");

  const descriptionBindings = { PR_BODY: event.body, PR_AUTHOR: event.author };
  assert.deepEqual(
    runMetadata(
      DESCRIPTION,
      {
        PR_BODY: body,
        PR_AUTHOR: "fixture-author",
      },
      directory,
      descriptionBindings,
    ),
    {
      status: 0,
      stdout:
        "Change description: `## What changed` is present and not empty.\n",
      stderr: "",
    },
  );
  const empty = "## What changed\n\n<!-- template -->\n";
  assert.deepEqual(
    runMetadata(
      DESCRIPTION,
      {
        PR_BODY: empty,
        PR_AUTHOR: "fixture-author",
        GITHUB_ACTOR: "dependabot[bot]",
      },
      directory,
      descriptionBindings,
    ),
    {
      status: 1,
      stdout:
        "::error::`## What changed` is empty. The PR description is the changelog: it becomes the commit body on squash. See .github/PULL_REQUEST_TEMPLATE.md.\n",
      stderr: "",
    },
  );
  assert.deepEqual(
    runMetadata(
      DESCRIPTION,
      {
        PR_BODY: empty,
        PR_AUTHOR: "dependabot[bot]",
        GITHUB_ACTOR: "fixture-actor",
      },
      directory,
      descriptionBindings,
    ),
    {
      status: 0,
      stdout:
        "Change description: skipped for Dependabot, whose body is generated.\n",
      stderr: "",
    },
  );
});

const reminder = lookupWorkflowStep(
  readFileSync(path.join(ROOT, REMINDERS), "utf8"),
  REMINDERS,
  "open-review-issue",
  "Open monthly tracking issue",
);
const reminderBody = reminder.step.with?.script;
assert.equal(typeof reminderBody, "string");
const reminderScript = new Script(`(async () => {\n${reminderBody}\n})()`, {
  filename: REMINDERS,
});
const repository = { owner: "fixture-owner", repo: "fixture-repo" };
const title = "Third-party reference repos review — 2026-11";
const sha = "a".repeat(40);
const runId = 123;
const notices = "THIRD_PARTY_NOTICES.md";

interface Issue {
  title: string;
  number: number;
  state: string;
  pull_request?: object;
}

function reminderFixture(directory: string, issues: Issue[]) {
  const calls: string[] = [];
  const logs: string[] = [];
  const created: {
    owner: string;
    repo: string;
    title: string;
    body: string;
  }[] = [];
  const listForRepo = () => assert.fail("the script must paginate all issues");
  return {
    calls,
    logs,
    created,
    run: async () => {
      await reminderScript.runInNewContext({
        github: {
          paginate: async (method: unknown, options: object) => {
            calls.push("paginate");
            assert.equal(method, listForRepo);
            assert.deepEqual(
              { ...options },
              { ...repository, state: "all", per_page: 100 },
            );
            return issues;
          },
          rest: {
            issues: {
              listForRepo,
              create: async (request: (typeof created)[number]) => {
                calls.push("create");
                created.push({ ...request });
                return { data: { number: 57 } };
              },
            },
          },
        },
        context: { repo: repository, sha, runId },
        core: {
          info: (message: string) => {
            calls.push("info");
            logs.push(message);
          },
        },
        process: {
          env: { GITHUB_SERVER_URL: "https://github.example.invalid" },
        },
        Date: class extends Date {
          constructor() {
            super("2026-11-01T00:00:00Z");
          }
        },
        require: (module: string) => {
          assert.equal(module, "node:fs");
          return {
            readFileSync: (file: string, encoding: BufferEncoding) => {
              calls.push("read");
              assert.equal(file, notices);
              assert.equal(encoding, "utf8");
              return readFileSync(path.join(directory, file), encoding);
            },
          };
        },
      });
    },
  };
}

test("the checked reminder queries all issues and leaves an open or closed duplicate alone", async (t) => {
  assert.match(reminder.step.uses!, /^actions\/github-script@[0-9a-f]{40}$/);
  assert.deepEqual(reminder.workflow.permissions, {
    contents: "read",
    issues: "write",
  });
  const directory = mkdtempSync(
    path.join(tmpdir(), "agentics-reminder-duplicate-"),
  );
  t.after(() => rmSync(directory, { recursive: true, force: true }));
  for (const state of ["open", "closed"]) {
    const fixture = reminderFixture(directory, [{ title, number: 41, state }]);
    await fixture.run();
    assert.deepEqual(fixture.calls, ["paginate", "info"]);
    assert.deepEqual(fixture.logs, [
      `Monthly review already tracked in #41 (${state}).`,
    ]);
    assert.deepEqual(fixture.created, []);
  }
});

test("the checked reminder ignores a matching pull request and submits one exact issue", async (t) => {
  const directory = mkdtempSync(
    path.join(tmpdir(), "agentics-reminder-create-"),
  );
  t.after(() => rmSync(directory, { recursive: true, force: true }));
  writeFileSync(
    path.join(directory, notices),
    [
      "Source repository: <https://github.com/zeta/repo>",
      "**Note on `alpha/repo`**",
      "Source repository: <https://github.com/alpha/repo>",
      "Source repository: <https://example.invalid/absent/repo>",
    ].join("\n"),
  );
  const fixture = reminderFixture(directory, [
    { title, number: 42, state: "open", pull_request: {} },
    { title: "Unrelated reminder", number: 43, state: "closed" },
  ]);
  await fixture.run();
  assert.deepEqual(fixture.calls, ["paginate", "read", "create", "info"]);
  assert.deepEqual(fixture.logs, ["Created monthly review #57."]);
  assert.equal(fixture.created.length, 1);
  const { body, ...identity } = fixture.created[0]!;
  assert.deepEqual(identity, { ...repository, title });
  assert.deepEqual(
    body.split("\n").filter((line) => line.startsWith("- [ ]")),
    [
      "- [ ] [alpha/repo](https://github.com/alpha/repo)",
      "- [ ] [zeta/repo](https://github.com/zeta/repo)",
    ],
  );
  assert.equal(
    createHash("sha256").update(body).digest("hex"),
    "7fade0607c7aaeb3da5324843a73d1956a6a59c12d5671e46b795911c99cf312",
  );
});

test("the checked reminder reports no reference repositories without creating an issue", async (t) => {
  const directory = mkdtempSync(
    path.join(tmpdir(), "agentics-reminder-empty-"),
  );
  t.after(() => rmSync(directory, { recursive: true, force: true }));
  writeFileSync(path.join(directory, notices), "No sources.\n");
  const fixture = reminderFixture(directory, []);
  await assert.rejects(
    fixture.run(),
    /No reference repositories found in THIRD_PARTY_NOTICES\.md/,
  );
  assert.deepEqual(fixture.calls, ["paginate", "read"]);
  assert.deepEqual(fixture.created, []);
});
