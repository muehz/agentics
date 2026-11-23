import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import test from "node:test";
import YAML from "yaml";
import {
  ALLOWED_LOCAL_ACTIONS,
  type CommandResult,
  type CommandRun,
  VERIFY_ARGS,
  evaluateReport,
  main,
} from "./ci-actions-lock.ts";

function cleanReport(sep: "/" | "\\" = "/") {
  const release = `.github${sep}workflows${sep}release.lock.yml`;
  const backlog = `.github${sep}workflows${sep}backlog-grooming.lock.yml`;
  return {
    valid: true,
    findings: [
      {
        workflow: release,
        category: "local-action",
        severity: "warning",
        detail:
          "cannot follow workspace-local action ./.github/actions/release",
      },
      {
        workflow: release,
        category: "local-action",
        severity: "warning",
        detail:
          "cannot follow workspace-local action ./.github/actions/conformance",
      },
      {
        workflow: backlog,
        category: "sha-as-ref",
        severity: "warning",
        detail:
          "pinned to a bare SHA without a symbolic ref — weakens supply-chain traceability",
      },
    ],
  };
}

function fakeRun(result: Partial<CommandResult>): {
  run: CommandRun;
  calls: { command: string; args: string[] }[];
} {
  const calls: { command: string; args: string[] }[] = [];
  const run: CommandRun = (command, args) => {
    calls.push({ command, args });
    return {
      status: 0,
      stdout: "",
      ...result,
    };
  };
  return { run, calls };
}

test("allowlisted release.lock.yml local actions pass on both POSIX and Windows paths", () => {
  assert.equal(ALLOWED_LOCAL_ACTIONS.size, 2);
  assert.deepEqual(evaluateReport(cleanReport("/")), []);
  assert.deepEqual(evaluateReport(cleanReport("\\")), []);
});

test("unexpected local action in another workflow is rejected", () => {
  const report = cleanReport("/");
  report.findings.push({
    workflow: ".github/workflows/validate.yml",
    category: "local-action",
    severity: "warning",
    detail:
      "cannot follow workspace-local action ./.github/actions/conformance",
  });
  const violations = evaluateReport(report);
  assert.equal(violations.length, 1);
  assert.match(
    violations[0]!,
    /unexpected local action in \.github\/workflows\/validate\.yml: cannot follow workspace-local action \.\/\.github\/actions\/conformance/,
  );
});

test("unexpected local action path in release.lock.yml is rejected", () => {
  const report = cleanReport("/");
  report.findings.push({
    workflow: ".github/workflows/release.lock.yml",
    category: "local-action",
    severity: "warning",
    detail: "cannot follow workspace-local action ./.github/actions/other",
  });
  const violations = evaluateReport(report);
  assert.equal(violations.length, 1);
  assert.match(
    violations[0]!,
    /unexpected local action in \.github\/workflows\/release\.lock\.yml: .*\/\.github\/actions\/other/,
  );
});

test("valid=false, error findings, and malformed payloads are rejected", () => {
  assert.deepEqual(evaluateReport(null), [
    "gh actions-lock output is not a JSON object",
  ]);
  assert.deepEqual(evaluateReport({ findings: [] }), [
    "gh actions-lock output is missing boolean field 'valid'",
  ]);
  assert.deepEqual(evaluateReport({ valid: true }), [
    "gh actions-lock output is missing array field 'findings'",
  ]);
  assert.deepEqual(
    evaluateReport({
      valid: false,
      findings: [
        {
          workflow: ".github/workflows/validate.yml",
          category: "not-pinned",
          severity: "error",
          detail: "used in workflow but not pinned in lockfile",
        },
      ],
    }),
    [
      "gh actions-lock reported valid=false",
      ".github/workflows/validate.yml: [not-pinned] used in workflow but not pinned in lockfile",
    ],
  );
  assert.equal(
    evaluateReport({ valid: true, findings: [{}] }).length,
    1,
    "finding missing fields must fail",
  );
});

test("main invokes gh actions-lock with --verify, --ignore-local-actions, and --json and reports failures", () => {
  const lines: string[] = [];
  const errors: string[] = [];
  const ok = fakeRun({ stdout: JSON.stringify(cleanReport("/")) });
  assert.equal(
    main({
      run: ok.run,
      write: (line) => lines.push(line),
      report: (msg) => errors.push(msg),
    }),
    0,
  );
  assert.deepEqual(ok.calls, [{ command: "gh", args: [...VERIFY_ARGS] }]);
  assert.equal(errors.length, 0);
  assert.match(lines[0] ?? "", /verified with no unexpected local actions/);

  const badJson = fakeRun({ stdout: "not json" });
  const jsonErrors: string[] = [];
  assert.equal(
    main({ run: badJson.run, report: (msg) => jsonErrors.push(msg) }),
    1,
  );
  assert.match(jsonErrors[0] ?? "", /invalid JSON from gh actions-lock/);

  const nonzero = fakeRun({ status: 2 });
  assert.equal(main({ run: nonzero.run }), 2);

  const spawnErr = fakeRun({
    status: null,
    error: new Error("ENOENT"),
  });
  const spawnErrors: string[] = [];
  assert.equal(
    main({ run: spawnErr.run, report: (msg) => spawnErrors.push(msg) }),
    1,
  );
  assert.match(spawnErrors[0] ?? "", /cannot start gh: ENOENT/);

  const badFinding = cleanReport("/");
  badFinding.findings.push({
    workflow: ".github/workflows/reminders.yml",
    category: "local-action",
    severity: "warning",
    detail:
      "cannot follow workspace-local action ./.github/actions/conformance",
  });
  const writtenErrors: string[] = [];
  assert.equal(
    main({
      run: fakeRun({ stdout: JSON.stringify(badFinding) }).run,
      write: (line) => writtenErrors.push(line),
    }),
    1,
  );
  assert.match(writtenErrors[0] ?? "", /^::error::unexpected local action/);
});

test("verify-lockfile.yml runs ci-actions-lock.ts on pinned Node after installing pinned gh-actions-lock", () => {
  const file = path.resolve(
    import.meta.dirname,
    "../../../../.github/workflows/verify-lockfile.yml",
  );
  const workflow = YAML.parse(fs.readFileSync(file, "utf8")) as {
    permissions: Record<string, string>;
    jobs: {
      verify: {
        name: string;
        steps: {
          name?: string;
          uses?: string;
          run?: string;
          with?: Record<string, unknown>;
        }[];
      };
    };
  };
  assert.deepEqual(workflow.permissions, { contents: "read" });
  assert.equal(workflow.jobs.verify.name, "Check / Actions lockfile");
  const steps = workflow.jobs.verify.steps;
  const checkout = steps.findIndex((s) => s.uses === "actions/checkout@v7.0.1");
  const node = steps.findIndex((s) => s.uses === "actions/setup-node@v7.1.0");
  const install = steps.findIndex((s) =>
    s.run?.includes("gh extension install github/gh-actions-lock --pin "),
  );
  const verify = steps.findIndex(
    (s) =>
      s.run?.trim() === "node .github/tools/src/scripts/ci-actions-lock.ts",
  );
  assert.ok(
    checkout >= 0 && node > checkout && install > node && verify > install,
  );
  assert.equal(steps[checkout]!.with?.["persist-credentials"], false);
  assert.equal(steps[node]!.with?.["node-version"], "26.12.1");
});
