#!/usr/bin/env node
// Decides whether a push touches a path the TypeScript coverage job covers, for the validate workflow's scope job.
//   BEFORE=<sha> GITHUB_SHA=<sha> GITHUB_OUTPUT=<file> node .github/tools/src/scripts/ci-coverage-paths.ts
// Like a workflow paths filter, it compares the push with its `before` commit. When that commit is missing (a new
// branch, or a force push that dropped it) coverage runs. The scope job runs no `npm ci`, so this module imports Node
// built-ins and the dependency-free plugin/common.ts only.

import { spawnSync } from "node:child_process";
import { appendFileSync } from "node:fs";
import { findCommand, spawnPlan } from "../plugin/common.ts";

export interface CommandResult {
  status: number | null;
  stdout: string;
  error?: Error;
}
// `stderr` defaults to the log; the probe for a commit that may be missing ignores it, as the shell step did.
export interface RunOptions {
  stderr?: "inherit" | "ignore";
}
export type CommandRun = (
  command: string,
  args: string[],
  options?: RunOptions,
) => CommandResult;
export type Report = (message: string) => void;
export type Env = Readonly<Record<string, string | undefined>>;

const COVERED =
  /^(?:\.github\/tools|evals|hooks|skills|\.github\/workflows|\.github\/release)\/|^package[^/]*\.json$/;
const NULL_SHA = /^0+$/;

// Starts git without a shell, through the shared launch plan, so a Windows `.cmd` shim is never spawned directly.
function nativeRun(
  command: string,
  args: string[],
  options: RunOptions = {},
): CommandResult {
  const file = findCommand(command) ?? command;
  const plan = spawnPlan(file, args, process.platform);
  const result = spawnSync(plan.file, plan.args, {
    encoding: "utf8",
    stdio: ["ignore", "pipe", options.stderr ?? "inherit"],
    maxBuffer: 256 * 1024 * 1024,
    windowsVerbatimArguments: plan.windowsVerbatimArguments,
  });
  return {
    status: result.status,
    stdout: result.stdout ?? "",
    error: result.error,
  };
}

// Returns the exit status of the step.
export function coveragePaths(
  env: Env = process.env,
  run: CommandRun = nativeRun,
  report: Report = console.error,
): number {
  const output = env["GITHUB_OUTPUT"] ?? "";
  if (output === "") {
    report("ci-coverage-paths: GITHUB_OUTPUT is not set");
    return 1;
  }
  const emit = (value: boolean): number => {
    appendFileSync(output, `run=${value}\n`);
    return 0;
  };
  const before = env["BEFORE"] ?? "";
  if (before === "" || NULL_SHA.test(before)) return emit(true);
  const known = run("git", ["cat-file", "-e", `${before}^{commit}`], {
    stderr: "ignore",
  });
  if (known.error) {
    report(`ci-coverage-paths: cannot start git: ${known.error.message}`);
    return 1;
  }
  if (known.status !== 0) return emit(true);
  const sha = env["GITHUB_SHA"] ?? "";
  const diff = run("git", ["diff", "--name-only", before, sha]);
  if (diff.error) {
    report(`ci-coverage-paths: cannot start git: ${diff.error.message}`);
    return 1;
  }
  if (diff.status !== 0) {
    report(`ci-coverage-paths: git diff failed with status ${diff.status}`);
    return diff.status ?? 1;
  }
  return emit(diff.stdout.split(/\r?\n/).some((line) => COVERED.test(line)));
}

if (import.meta.main) process.exitCode = coveragePaths();
