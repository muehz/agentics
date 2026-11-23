#!/usr/bin/env node
// Runs the hooks module's Go tests and enforces its statement-coverage floor, for the validate workflow's go-suite job.
//   node .github/tools/src/scripts/ci-hooks-test.ts test    (COVERAGE=1 records the profile under RUNNER_TEMP)
//   node .github/tools/src/scripts/ci-hooks-test.ts floor   (reads that profile)
// The macOS and Windows legs run `test` uninstrumented. The go-suite job runs no `npm ci`, so this module imports Node
// built-ins and the dependency-free plugin/common.ts only.

import { spawnSync } from "node:child_process";
import { mkdirSync } from "node:fs";
import path from "node:path";
import { findCommand, spawnPlan } from "../plugin/common.ts";

export interface CommandResult {
  status: number | null;
  stdout: string;
  error?: Error;
  signal?: NodeJS.Signals | null;
}
// `capture` pipes stdout into the result; without it the child writes straight to this process's output.
export type CommandRun = (
  command: string,
  args: string[],
  options: {
    capture?: boolean;
    env?: Env;
    platform?: NodeJS.Platform;
  },
) => CommandResult;
export type Env = Readonly<Record<string, string | undefined>>;
export type Report = (message: string) => void;

const FLOOR = 90;
const USAGE = "usage: ci-hooks-test.ts test|floor";

// Resolves go through the shared PATH lookup and starts it without a shell, so a Windows shim is never spawned directly.
const nativeRun: CommandRun = (command, args, options) => {
  const env = options.env ?? process.env;
  const platform = options.platform ?? process.platform;
  const file = findCommand(command, env, platform) ?? command;
  const plan = spawnPlan(file, args, platform, env);
  const result = spawnSync(plan.file, plan.args, {
    encoding: "utf8",
    stdio: ["inherit", options.capture ? "pipe" : "inherit", "inherit"],
    maxBuffer: 64 * 1024 * 1024,
    windowsVerbatimArguments: plan.windowsVerbatimArguments,
  });
  return {
    status: result.status,
    stdout: result.stdout ?? "",
    error: result.error,
    signal: result.signal,
  };
};

export interface Options {
  env?: Env;
  platform?: NodeJS.Platform;
  run?: CommandRun;
  write?: (line: string) => void;
  report?: Report;
}

// The exit status a failed go leaves for the step, reporting why when the status alone does not say.
function failure(result: CommandResult, report: Report): number {
  if (result.error) {
    report(`ci-hooks-test: cannot start go: ${result.error.message}`);
    return 1;
  }
  if (result.signal) {
    report(`ci-hooks-test: go was killed by ${result.signal}`);
    return 1;
  }
  return result.status && result.status !== 0 ? result.status : 1;
}

const succeeded = (result: CommandResult): boolean =>
  !result.error && !result.signal && result.status === 0;

// Returns the exit status of the step.
export function main(argv: string[], options: Options = {}): number {
  const env = options.env ?? process.env;
  const run = options.run ?? nativeRun;
  const write = options.write ?? console.log;
  const report = options.report ?? console.error;
  const runOptions = { env, platform: options.platform };
  const mode = argv.length === 1 ? argv[0] : undefined;
  if (mode !== "test" && mode !== "floor") {
    report(USAGE);
    return 2;
  }

  const temp = env["RUNNER_TEMP"] ?? "";
  const coverage = mode === "floor" || (env["COVERAGE"] ?? "") !== "";
  if (coverage && temp === "") {
    report("ci-hooks-test: RUNNER_TEMP is not set");
    return 1;
  }
  const profile = path.join(temp, "coverage", "hooks.out");

  if (mode === "test") {
    const args = ["test", "-C", "hooks/go", "-count=1", "./..."];
    if (coverage) {
      mkdirSync(path.join(temp, "coverage"), { recursive: true });
      args.push("-coverpkg=./...", `-coverprofile=${profile}`);
    }
    const result = run("go", args, runOptions);
    return succeeded(result) ? 0 : failure(result, report);
  }

  const listing = run(
    "go",
    ["-C", "hooks/go", "tool", "cover", `-func=${profile}`],
    { ...runOptions, capture: true },
  );
  if (!succeeded(listing)) return failure(listing, report);
  const line = listing.stdout
    .split(/\r?\n/)
    .map((text) => text.trim().split(/\s+/))
    .find((fields) => fields[0] === "total:");
  const total = line?.[line.length - 1]?.replace("%", "") ?? "";
  if (total === "") {
    write("::error::The coverage profile has no total line.");
    return 1;
  }
  write(`hooks/go statement coverage: ${total}%`);
  if (!(Number(total) >= FLOOR)) {
    write(
      `::error::Statement coverage is ${total}%, under the ${FLOOR}% floor. Add cases for the uncovered branches (go tool cover -func) rather than lowering the floor.`,
    );
    return 1;
  }
  return 0;
}

if (import.meta.main) process.exitCode = main(process.argv.slice(2));
