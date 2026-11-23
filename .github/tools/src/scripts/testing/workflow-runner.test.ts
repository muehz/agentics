import assert from "node:assert/strict";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import test, { type TestContext } from "node:test";
import { requireInterpreter } from "../../entrypoint/interpreters.ts";
import {
  ACTION_JOB,
  lookupWorkflowStep,
  readWorkflowArgv,
  recordWorkflowArgv,
  runWorkflowStep,
  workflowShell,
  workflowShellSkip,
} from "./workflow-steps.ts";

const root = path.resolve(import.meta.dirname, "../../../../..");
const validate = ".github/workflows/validate.yml";
const manual = ".github/workflows/validate-manual.yml";
const action = ".github/actions/conformance/action.yml";

function checked(file: string, job: string, name: string) {
  return lookupWorkflowStep(
    readFileSync(path.join(root, file), "utf8"),
    file,
    job,
    name,
  );
}

function fixture(t: TestContext) {
  const cwd = mkdtempSync(path.join(tmpdir(), "agentics workflow 'fixture' "));
  t.after(() => rmSync(cwd, { recursive: true, force: true }));
  return cwd;
}

const title = checked(validate, "pr-metadata", "Check the PR title");
const record = checked(action, ACTION_JOB, "Record the Node setup");
const bash = { skip: requireInterpreter("bash") };
const explicitBash = {
  name: "bash",
  flags: ["--noprofile", "--norc", "-e", "-o", "pipefail"],
};

test("checked runner labels and shell precedence determine the interpreter and flags", () => {
  assert.deepEqual(workflowShell(title), { name: "bash", flags: ["-e"] });
  assert.deepEqual(workflowShell(record), explicitBash);
  const job = "validate-windows-dispatch";
  const jobDefault = checked(
    manual,
    job,
    "cmd dispatches the built binary and denies",
  );
  const overridden = {
    ...jobDefault,
    workflow: { ...jobDefault.workflow, defaults: { run: { shell: "sh" } } },
  };
  assert.deepEqual(workflowShell(overridden), {
    name: "pwsh",
    flags: ["-command"],
  });
  assert.deepEqual(
    workflowShell({
      ...overridden,
      job: { ...overridden.job, defaults: undefined },
    }),
    { name: "sh", flags: ["-e"] },
  );
  assert.deepEqual(
    workflowShell({
      ...jobDefault,
      job: { ...jobDefault.job, defaults: undefined },
    }),
    { name: "pwsh", flags: ["-command"] },
  );
  assert.deepEqual(
    workflowShell(checked(manual, job, "Build the hook binaries from source")),
    explicitBash,
  );
});

test("a missing optional shell yields its skip reason and a required shell fails", () => {
  const missing = { env: { PATH: "" } };
  assert.equal(
    workflowShellSkip(title, missing),
    "bash is not available on this host",
  );
  assert.throws(
    () =>
      workflowShellSkip(title, {
        env: { PATH: "", AGX_REQUIRED_INTERPRETERS: "bash" },
      }),
    /required interpreter "bash" is missing/,
  );
});

test(
  "the checked title preserves the accepted and rejected status and bytes",
  bash,
  (t) => {
    const cwd = fixture(t);
    for (const [value, status, stdout] of [
      [
        "fix: handle quoted input",
        0,
        "PR title: Conventional Commits format accepted.\n",
      ],
      [
        "invalid",
        1,
        "::error::PR title must use <type>(<scope>)<!>: <summary>, with optional scope and !. See CONTRIBUTING.md: Commit Message Convention.\n",
      ],
    ] as const) {
      const env = { ...process.env, PR_TITLE: value };
      const actual = runWorkflowStep(title, { cwd, env });
      assert.deepEqual(actual, { status, stdout, stderr: "" });
    }
  },
);

test(
  "the checked action writes the exact Node setup record with a restricted PATH",
  bash,
  (t) => {
    const cwd = fixture(t);
    const shared = path.join(cwd, "github env");
    const actual = runWorkflowStep(record, {
      cwd,
      env: { PATH: "", GITHUB_ENV: shared },
    });
    assert.deepEqual(actual, { status: 0, stdout: "", stderr: "" });
    assert.deepEqual(
      readFileSync(shared),
      Buffer.from("AGX_NODE_SETUP=26.12.1\n"),
    );
  },
);

test(
  "implicit Bash continues after a failing pipeline and explicit Bash exits at the pipeline",
  bash,
  (t) => {
    const cwd = fixture(t);
    for (const [checkedStep, expected] of [
      [title, 0],
      [record, 1],
    ] as const) {
      const step = {
        ...checkedStep.step,
        run: "false | true\nprintf 'continued\\n'\n",
      };
      const actual = runWorkflowStep(
        { ...checkedStep, step },
        { cwd, env: process.env },
      );
      assert.deepEqual(actual, {
        status: expected,
        stdout: expected ? "" : "continued\n",
        stderr: "",
      });
    }
  },
);

test(
  "the shell executes a script file and records each argument including empty fields",
  bash,
  (t) => {
    const cwd = fixture(t);
    const argv = path.join(cwd, "recorded args");
    const run = [
      'test -f "$0"',
      'test "$(pwd -P)" = "$(cd "$FIXTURE_CWD" && pwd -P)"',
      'set -- "" "two words" "$DATA" ""',
      recordWorkflowArgv,
    ].join("\n");
    const data = 'literal "quote"\nnext line';
    const actual = runWorkflowStep(
      { ...record, step: { ...record.step, run } },
      {
        cwd,
        env: {
          ...process.env,
          ACTION_ARGV: argv,
          FIXTURE_CWD: cwd,
          DATA: data,
        },
      },
    );
    assert.deepEqual(actual, { status: 0, stdout: "", stderr: "" });
    assert.deepEqual(
      readFileSync(argv),
      Buffer.from(["", "two words", data, "", ""].join("\0")),
    );
    assert.deepEqual(readWorkflowArgv(argv), ["", "two words", data, ""]);
  },
);

test("the argument recorder distinguishes zero arguments, an empty argument and truncated data", (t) => {
  const file = path.join(fixture(t), "arguments");
  writeFileSync(file, "");
  assert.deepEqual(readWorkflowArgv(file), []);
  writeFileSync(file, "\0");
  assert.deepEqual(readWorkflowArgv(file), [""]);
  writeFileSync(file, "first\0incomplete");
  assert.throws(
    () => readWorkflowArgv(file),
    /argument recorder emitted an unterminated argument/,
  );
});

const windowsStep = checked(
  manual,
  "validate-windows-dispatch",
  "cmd dispatches the built binary and denies",
);
test(
  "the job's PowerShell command executes the script file and reports native failure",
  { skip: workflowShellSkip(windowsStep) },
  (t) => {
    const run =
      'if (-not (Test-Path -LiteralPath $PSCommandPath)) { throw "missing script file" }\nnode -e "process.exit(13)"\n[Console]::Write($LASTEXITCODE)';
    const actual = runWorkflowStep(
      { ...windowsStep, step: { ...windowsStep.step, run } },
      { cwd: fixture(t), env: process.env },
    );
    assert.deepEqual(actual, {
      status: 1,
      stdout: "13",
      stderr: "",
    });
  },
);
