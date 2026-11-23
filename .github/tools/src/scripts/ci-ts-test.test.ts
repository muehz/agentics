import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import path from "node:path";
import test from "node:test";
import { type CommandResult, type CommandRun, tsTest } from "./ci-ts-test.ts";

const MODULE = path.join(import.meta.dirname, "ci-ts-test.ts");

interface Call {
  command: string;
  args: string[];
}

function fakes(result: CommandResult = { status: 0 }) {
  const calls: Call[] = [];
  const errors: string[] = [];
  const run: CommandRun = (command, args) => {
    calls.push({ command, args });
    return result;
  };
  return { calls, errors, run, report: (line: string) => errors.push(line) };
}

const STEP_ENV = { TEST_SHARD: "4", TEST_CONCURRENCY: "2" };

test("a non-Windows runner runs the workspace's own npm test", () => {
  for (const runner of ["ubuntu-latest", "xcode-27"]) {
    const f = fakes();
    const status = tsTest({
      env: { ...STEP_ENV, WORKSPACE: "tools", RUNNER_LABEL: runner },
      run: f.run,
      report: f.report,
    });
    assert.equal(status, 0, runner);
    assert.deepEqual(f.calls, [
      { command: "npm", args: ["--prefix", ".github/tools", "test"] },
    ]);
  }
});

test("the Windows runner runs the shard script with the shard and concurrency", () => {
  const f = fakes();
  const status = tsTest({
    env: { ...STEP_ENV, WORKSPACE: "tools", RUNNER_LABEL: "windows-latest" },
    run: f.run,
    report: f.report,
  });
  assert.equal(status, 0);
  assert.deepEqual(f.calls, [
    {
      command: process.execPath,
      args: [
        ".github/tools/src/scripts/ci-test-shard.ts",
        "windows-tools",
        "4",
        "2",
      ],
    },
  ]);
});

test("an empty or missing workspace prints the error and starts nothing", () => {
  for (const env of [
    { ...STEP_ENV, WORKSPACE: "", RUNNER_LABEL: "windows-latest" },
    { ...STEP_ENV, RUNNER_LABEL: "ubuntu-latest" },
  ]) {
    const f = fakes();
    assert.equal(tsTest({ env, run: f.run, report: f.report }), 1);
    assert.equal(f.calls.length, 0);
    assert.deepEqual(f.errors, ["unknown TypeScript workspace"]);
  }
});

test("the exit status of the child is the exit status of the step", () => {
  for (const status of [0, 1, 7]) {
    const f = fakes({ status });
    assert.equal(
      tsTest({
        env: { WORKSPACE: "tools", RUNNER_LABEL: "ubuntu-latest" },
        run: f.run,
        report: f.report,
      }),
      status,
    );
  }
});

test("a child that cannot start or was killed exits nonzero with a reason", () => {
  const missing = fakes({
    status: null,
    error: new Error("spawn npm ENOENT"),
  });
  assert.equal(
    tsTest({
      env: { WORKSPACE: "tools", RUNNER_LABEL: "ubuntu-latest" },
      run: missing.run,
      report: missing.report,
    }),
    1,
  );
  assert.match(missing.errors.join("\n"), /^ci-ts-test: cannot start npm/);
  const killed = fakes({ status: null, signal: "SIGTERM" });
  assert.equal(
    tsTest({
      env: { WORKSPACE: "tools", RUNNER_LABEL: "ubuntu-latest" },
      run: killed.run,
      report: killed.report,
    }),
    1,
  );
  assert.match(
    killed.errors.join("\n"),
    /^ci-ts-test: npm was killed by SIGTERM/,
  );
});

test("run as a script, the module prints the workspace error and sets the exit status", () => {
  const env = { ...process.env, WORKSPACE: "", RUNNER_LABEL: "ubuntu-latest" };
  const result = spawnSync(process.execPath, [MODULE], {
    env,
    encoding: "utf8",
  });
  assert.equal(result.status, 1);
  assert.match(result.stderr, /unknown TypeScript workspace/);
});
