import assert from "node:assert/strict";
import { execFile, spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { createServer } from "node:http";
import { tmpdir } from "node:os";
import path from "node:path";
import test from "node:test";
import { promisify } from "node:util";
import { Script } from "node:vm";
import {
  inlineNodeScript,
  lookupWorkflowStep,
  type WorkflowStepData,
} from "./workflow-steps.ts";

const ROOT = path.resolve(import.meta.dirname, "../../../../..");
const VALIDATE = ".github/workflows/validate.yml";
const METADATA = "pr-metadata";
const FETCH = "Fetch current pull request metadata";
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

const metadata = {
  title: "${{ fromJSON(steps.metadata.outputs.snapshot).title }}",
  body: "${{ fromJSON(steps.metadata.outputs.snapshot).body }}",
  author: "${{ fromJSON(steps.metadata.outputs.snapshot).author }}",
};

test("each metadata run fetches the current snapshot for all three checks", async (t) => {
  const selected = lookupWorkflowStep(validate, VALIDATE, METADATA, FETCH);
  const events = selected.workflow.on as { pull_request: { types: string[] } };
  assert.deepEqual(events.pull_request.types, [
    "opened",
    "synchronize",
    "reopened",
    "edited",
  ]);
  assert.deepEqual(selected.step.env, {
    GH_TOKEN: "${{ github.token }}",
    PR_NUMBER: "${{ github.event.pull_request.number }}",
  });
  const directory = mkdtempSync(path.join(tmpdir(), "agentics-pr-metadata-"));
  t.after(() => rmSync(directory, { recursive: true, force: true }));
  const output = path.join(directory, "github-output");
  const calls: { url: string | undefined; method: string | undefined }[] = [];
  let current = {
    title: "Old title",
    body: null as string | null,
    user: { login: "fixture-author" },
  };
  let status = 200;
  const server = createServer((request, response) => {
    calls.push({ url: request.url, method: request.method });
    response.writeHead(status, { "Content-Type": "application/json" });
    response.end(JSON.stringify(current));
  });
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  t.after(() => server[Symbol.asyncDispose]());
  const address = server.address();
  assert.ok(address && typeof address === "object");
  const env = {
    ...process.env,
    GITHUB_API_URL: `http://127.0.0.1:${address.port}`,
    GITHUB_REPOSITORY: "example/agentics",
    GH_TOKEN: "fixture-token",
    PR_NUMBER: "2",
    GITHUB_OUTPUT: output,
    PR_TITLE: "Old title",
    PR_BODY: "Old description",
    PR_AUTHOR: "old-author",
  };
  const fetchMetadata = () =>
    promisify(execFile)(process.execPath, ["-e", inlineNodeScript(selected)], {
      env,
    });
  const fetchSnapshot = async () => {
    writeFileSync(output, "");
    await fetchMetadata();
    const line = readFileSync(output, "utf8");
    assert.match(line, /^snapshot=[^\r\n]+\n$/u);
    return JSON.parse(line.slice("snapshot=".length)) as Record<string, string>;
  };
  const check = (
    name: string,
    bindings: Record<string, string>,
    snapshot: Record<string, string>,
  ) =>
    runMetadata(
      name,
      {
        PR_TITLE: snapshot.title!,
        PR_BODY: snapshot.body!,
        PR_AUTHOR: snapshot.author!,
      },
      directory,
      bindings,
    );
  const titleBindings = { PR_TITLE: metadata.title };
  const markerBindings = { PR_TITLE: metadata.title, PR_BODY: metadata.body };
  const descriptionBindings = {
    PR_BODY: metadata.body,
    PR_AUTHOR: metadata.author,
  };

  const stale = await fetchSnapshot();
  assert.deepEqual(stale, {
    title: "Old title",
    body: "",
    author: "fixture-author",
  });
  assert.equal(check(TITLE, titleBindings, stale).status, 1);
  assert.equal(check(DESCRIPTION, descriptionBindings, stale).status, 1);

  current = {
    ...current,
    title: "fix: check current metadata",
    body: "## What changed\r\n\r\nTreat newlines and shell-shaped text $(exit 73) as data.\r\n",
  };
  const fresh = await fetchSnapshot();
  assert.deepEqual(fresh, {
    title: current.title,
    body: current.body,
    author: current.user.login,
  });
  assert.equal(check(TITLE, titleBindings, fresh).status, 0);
  assert.match(
    check(MARKERS, markerBindings, fresh).stdout,
    /Release level: patch/u,
  );
  assert.equal(check(DESCRIPTION, descriptionBindings, fresh).status, 0);

  status = 403;
  writeFileSync(output, "");
  await assert.rejects(fetchMetadata(), {
    stderr: /current pull request metadata.*HTTP 403/u,
  });
  assert.equal(readFileSync(output, "utf8"), "");
  assert.deepEqual(
    calls,
    Array.from({ length: 3 }, () => ({
      url: "/repos/example/agentics/pulls/2",
      method: "GET",
    })),
  );
});

function runMetadata(
  name: string,
  env: Record<string, string>,
  directory: string,
  expectedBindings: Record<string, string>,
) {
  const selected = lookupWorkflowStep(validate, VALIDATE, METADATA, name);
  assert.deepEqual(selected.job.permissions, { "pull-requests": "read" });
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

test("the checkout-free metadata job executes the inline rules with snapshot data in env only", (t) => {
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
      PR_TITLE: metadata.title,
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
      { PR_TITLE: metadata.title, PR_BODY: metadata.body },
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
      { PR_TITLE: metadata.title, PR_BODY: metadata.body },
    ),
    {
      status: 1,
      stdout:
        "::error::A BREAKING CHANGE footer needs a ! in the title, so a major release is visible in review. See CONTRIBUTING.md: Commit Message Convention.\n",
      stderr: "",
    },
  );
  assert.equal(readFileSync(output, "utf8"), "level=major\n");

  const descriptionBindings = {
    PR_BODY: metadata.body,
    PR_AUTHOR: metadata.author,
  };
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
