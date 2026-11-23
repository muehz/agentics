import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import { type CommandResult, type CommandRun, main } from "./ci-hooks-test.ts";

const MODULE = path.join(import.meta.dirname, "ci-hooks-test.ts");

interface Call {
  command: string;
  args: string[];
  capture: boolean | undefined;
}

function fakes(result: CommandResult = { status: 0, stdout: "" }) {
  const calls: Call[] = [];
  const out: string[] = [];
  const errors: string[] = [];
  const run: CommandRun = (command, args, options) => {
    calls.push({ command, args, capture: options.capture });
    return result;
  };
  return {
    calls,
    out,
    errors,
    run,
    write: (line: string) => out.push(line),
    report: (line: string) => errors.push(line),
  };
}

function runnerTemp(t: test.TestContext): string {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "agentics-hooks-test-"));
  t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
  return dir;
}

const GO_TEST = ["test", "-C", "hooks/go", "-count=1", "./..."];

test("test with COVERAGE records one profile over every package and creates its directory", (t) => {
  const temp = runnerTemp(t);
  const f = fakes();
  const status = main(["test"], {
    env: { COVERAGE: "1", RUNNER_TEMP: temp },
    run: f.run,
    write: f.write,
    report: f.report,
  });
  assert.equal(status, 0);
  assert.equal(f.calls.length, 1);
  assert.equal(f.calls[0]!.command, "go");
  assert.deepEqual(f.calls[0]!.args, [
    ...GO_TEST,
    "-coverpkg=./...",
    `-coverprofile=${path.join(temp, "coverage", "hooks.out")}`,
  ]);
  assert.ok(fs.statSync(path.join(temp, "coverage")).isDirectory());
  assert.notEqual(f.calls[0]!.capture, true);
});

test("test without COVERAGE, or with an empty one, runs plain go test and makes no directory", (t) => {
  const temp = runnerTemp(t);
  for (const env of [
    { RUNNER_TEMP: temp },
    { COVERAGE: "", RUNNER_TEMP: temp },
  ]) {
    const f = fakes();
    assert.equal(main(["test"], { env, run: f.run, report: f.report }), 0);
    assert.deepEqual(f.calls[0]!.args, GO_TEST);
  }
  assert.equal(fs.existsSync(path.join(temp, "coverage")), false);
});

test("test exits with the status of go test", () => {
  const f = fakes({ status: 3, stdout: "" });
  assert.equal(main(["test"], { env: {}, run: f.run, report: f.report }), 3);
});

test("test reports a go that cannot start or was killed and exits nonzero", () => {
  const failed = fakes({
    status: null,
    stdout: "",
    error: new Error("spawn go ENOENT"),
  });
  assert.notEqual(
    main(["test"], { env: {}, run: failed.run, report: failed.report }),
    0,
  );
  assert.match(failed.errors.join("\n"), /^ci-hooks-test: cannot start go/);
  const killed = fakes({ status: null, stdout: "", signal: "SIGKILL" });
  assert.notEqual(
    main(["test"], { env: {}, run: killed.run, report: killed.report }),
    0,
  );
  assert.match(killed.errors.join("\n"), /^ci-hooks-test: go was killed by/);
});

test("test with COVERAGE but no RUNNER_TEMP exits nonzero and runs nothing", () => {
  const f = fakes();
  assert.notEqual(
    main(["test"], { env: { COVERAGE: "1" }, run: f.run, report: f.report }),
    0,
  );
  assert.equal(f.calls.length, 0);
  assert.match(f.errors.join("\n"), /^ci-hooks-test: RUNNER_TEMP is not set/);
});

function floor(total: string | null, runResult?: Partial<CommandResult>) {
  const listing =
    total === null
      ? "ok/a.go:1:\tf\t100.0%\n"
      : `ok/a.go:1:\tf\t100.0%\ntotal:\t\t(statements)\t${total}%\n`;
  const f = fakes({ status: 0, stdout: listing, ...runResult });
  const status = main(["floor"], {
    env: { RUNNER_TEMP: "/tmp/r" },
    run: f.run,
    write: f.write,
    report: f.report,
  });
  return { ...f, status };
}

test("floor reads the profile with go tool cover from hooks/go", () => {
  const f = floor("95.3");
  assert.equal(f.calls[0]!.command, "go");
  assert.deepEqual(f.calls[0]!.args, [
    "-C",
    "hooks/go",
    "tool",
    "cover",
    `-func=${path.join("/tmp/r", "coverage", "hooks.out")}`,
  ]);
  assert.equal(f.calls[0]!.capture, true);
});

test("floor passes at 95.3 and at exactly 90.0 and prints the total", () => {
  for (const total of ["95.3", "90.0"]) {
    const f = floor(total);
    assert.equal(f.status, 0, total);
    assert.deepEqual(f.out, [`hooks/go statement coverage: ${total}%`]);
  }
});

test("floor fails at 89.9 with the exact message", () => {
  const f = floor("89.9");
  assert.equal(f.status, 1);
  assert.deepEqual(f.out, [
    "hooks/go statement coverage: 89.9%",
    "::error::Statement coverage is 89.9%, under the 90% floor. Add cases for the uncovered branches (go tool cover -func) rather than lowering the floor.",
  ]);
});

test("floor fails when the listing has no total line", () => {
  const f = floor(null);
  assert.equal(f.status, 1);
  assert.deepEqual(f.out, ["::error::The coverage profile has no total line."]);
});

test("floor fails when go tool cover fails or cannot start", () => {
  const failed = floor("99.0", { status: 2 });
  assert.equal(failed.status, 2);
  assert.deepEqual(failed.out, []);
  const missing = floor("99.0", {
    status: null,
    error: new Error("spawn go ENOENT"),
  });
  assert.notEqual(missing.status, 0);
  assert.match(missing.errors.join("\n"), /^ci-hooks-test: cannot start go/);
});

test("floor without RUNNER_TEMP exits nonzero and runs nothing", () => {
  const f = fakes();
  assert.notEqual(
    main(["floor"], { env: {}, run: f.run, report: f.report }),
    0,
  );
  assert.equal(f.calls.length, 0);
});

test("a missing or unknown subcommand prints the usage and exits 2", () => {
  for (const argv of [[], ["bogus"], ["test", "extra"]]) {
    const f = fakes();
    assert.equal(main(argv, { env: {}, run: f.run, report: f.report }), 2);
    assert.deepEqual(f.errors, ["usage: ci-hooks-test.ts test|floor"]);
    assert.equal(f.calls.length, 0);
  }
});

test("run as a script, the module sets the exit status", () => {
  const result = spawnSync(process.execPath, [MODULE], { encoding: "utf8" });
  assert.equal(result.status, 2);
  assert.match(result.stderr, /usage: ci-hooks-test\.ts test\|floor/);
});
