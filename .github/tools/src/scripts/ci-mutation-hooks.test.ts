import assert from "node:assert/strict";
import path from "node:path";
import fs from "node:fs";
import test from "node:test";
import YAML from "yaml";
import { type CommandRun, mutateHooks } from "./ci-mutation-hooks.ts";

const MODULE = "github.com/muehz/agentics/hooks/go";
const LISTING = [
  `${MODULE}/internal/shellwords`,
  `${MODULE}/internal/commandguard`,
  `${MODULE}/internal/secretsguard`,
  `${MODULE}/cmd/hook`,
  `${MODULE}/internal/alpha`,
  `${MODULE}/cmd/sourcedigest`,
  `${MODULE}/internal/commandguardian`,
  "",
].join("\n");

interface Call {
  command: string;
  args: string[];
  cwd: string | undefined;
  capture: boolean | undefined;
}

function fakes(listing: { status: number | null; stdout: string }) {
  const calls: Call[] = [];
  const run: CommandRun = (command, args, options) => {
    calls.push({
      command,
      args,
      cwd: options.cwd,
      capture: options.capture,
    });
    if (command === "go") return listing;
    return { status: 0, stdout: "" };
  };
  return { calls, run };
}

test("rest lists the module's packages with go list in hooks/go", () => {
  const { calls, run } = fakes({ status: 0, stdout: LISTING });
  mutateHooks({ TARGET: "rest" }, run);
  assert.equal(calls[0]!.command, "go");
  assert.deepEqual(calls[0]!.args, [
    "list",
    "-f",
    "{{if .GoFiles}}{{.ImportPath}}{{end}}",
    "./...",
  ]);
  assert.equal(path.basename(path.dirname(calls[0]!.cwd ?? "")), "hooks");
  assert.equal(path.basename(calls[0]!.cwd ?? ""), "go");
});

test("rest rewrites the module prefix and drops the four excluded packages, and nothing else", () => {
  const { calls, run } = fakes({ status: 0, stdout: LISTING });
  assert.equal(mutateHooks({ TARGET: "rest" }, run), 0);
  assert.equal(calls.length, 2);
  assert.equal(calls[1]!.command, "node");
  assert.deepEqual(calls[1]!.args, [
    ".github/tools/src/scripts/mutation-gremlins.ts",
    "--packages",
    "./internal/alpha",
    "./cmd/sourcedigest",
    "./internal/commandguardian",
  ]);
  assert.equal(calls[1]!.cwd, undefined);
});

test("any other target is the one package and go list is not run", () => {
  const { calls, run } = fakes({ status: 0, stdout: LISTING });
  assert.equal(mutateHooks({ TARGET: "./internal/hooktool" }, run), 0);
  assert.equal(calls.length, 1);
  assert.equal(calls[0]!.command, "node");
  assert.deepEqual(calls[0]!.args.slice(1), [
    "--packages",
    "./internal/hooktool",
  ]);
});

test("a failing go list exits nonzero and starts no mutation run", () => {
  const { calls, run } = fakes({ status: 3, stdout: "" });
  assert.equal(mutateHooks({ TARGET: "rest" }, run), 3);
  assert.deepEqual(
    calls.map((c) => c.command),
    ["go"],
  );
});

test("a go list that cannot start exits nonzero", () => {
  const run: CommandRun = () => ({
    status: null,
    stdout: "",
    error: new Error("spawn go ENOENT"),
  });
  assert.notEqual(mutateHooks({ TARGET: "rest" }, run), 0);
});

test("a listing with nothing left after the exclusions exits nonzero and starts no mutation run", () => {
  const { calls, run } = fakes({
    status: 0,
    stdout: `${MODULE}/cmd/hook\n`,
  });
  assert.notEqual(mutateHooks({ TARGET: "rest" }, run), 0);
  assert.equal(calls.length, 1);
});

test("the exit status of the mutation run is the exit status of the step", () => {
  const run: CommandRun = (command) =>
    command === "go"
      ? { status: 0, stdout: `${MODULE}/internal/alpha\n` }
      : { status: 7, stdout: "" };
  assert.equal(mutateHooks({ TARGET: "rest" }, run), 7);
});

test("a missing or empty TARGET exits nonzero and runs nothing", () => {
  for (const env of [{}, { TARGET: "" }]) {
    const { calls, run } = fakes({ status: 0, stdout: LISTING });
    assert.notEqual(mutateHooks(env, run), 0);
    assert.equal(calls.length, 0);
  }
});

test("only go list is captured; the mutation run's output goes straight to the log", () => {
  const { calls, run } = fakes({ status: 0, stdout: LISTING });
  mutateHooks({ TARGET: "rest" }, run);
  assert.equal(calls[0]!.capture, true);
  assert.notEqual(calls[1]!.capture, true);
  const single = fakes({ status: 0, stdout: "" });
  mutateHooks({ TARGET: "./internal/alpha" }, single.run);
  assert.notEqual(single.calls[0]!.capture, true);
});

test("a command that cannot start is reported on stderr", () => {
  const messages: string[] = [];
  const run: CommandRun = () => ({
    status: null,
    stdout: "",
    error: new Error("spawn go ENOENT"),
  });
  assert.equal(
    mutateHooks({ TARGET: "rest" }, run, (m) => messages.push(m)),
    1,
  );
  assert.deepEqual(messages, [
    "ci-mutation-hooks: cannot start go: spawn go ENOENT",
  ]);
});

test("a child killed by a signal is reported on stderr", () => {
  const messages: string[] = [];
  const run: CommandRun = () => ({
    status: null,
    stdout: "",
    signal: "SIGKILL",
  });
  assert.equal(
    mutateHooks({ TARGET: "./internal/alpha" }, run, (m) => messages.push(m)),
    1,
  );
  assert.deepEqual(messages, ["ci-mutation-hooks: node was killed by SIGKILL"]);
});

test("an empty rest listing says so", () => {
  const messages: string[] = [];
  const { run } = fakes({ status: 0, stdout: `${MODULE}/cmd/hook\n` });
  assert.equal(
    mutateHooks({ TARGET: "rest" }, run, (m) => messages.push(m)),
    1,
  );
  assert.deepEqual(messages, [
    "ci-mutation-hooks: no packages left to mutate after the exclusions",
  ]);
});

test("the workflow step runs this module with TARGET in its env", () => {
  const file = path.resolve(
    import.meta.dirname,
    "../../../../.github/workflows/validate-mutation.yml",
  );
  const workflow = YAML.parse(fs.readFileSync(file, "utf8"));
  const steps: { name?: string; run?: string; env?: Record<string, string> }[] =
    workflow.jobs["mutation-full"].steps;
  const step = steps.find((s) => s.name === "Mutate hooks/go packages");
  assert.ok(step);
  assert.equal(
    step.run?.trim(),
    "node .github/tools/src/scripts/ci-mutation-hooks.ts",
  );
  assert.ok(step.env && "TARGET" in step.env);
});
