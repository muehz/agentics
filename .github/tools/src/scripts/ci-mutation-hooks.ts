#!/usr/bin/env node
// Runs gremlins mutation testing on the hooks/go packages the mutation workflow's gremlins legs name.
//   TARGET=<package|rest> node .github/tools/src/scripts/ci-mutation-hooks.ts
// A TARGET other than `rest` is the one package to mutate. `rest` mutates every package of the module that has Go
// files, except the four that mutation-gremlins.ts cannot run in isolation or that other legs cover. The gremlins leg
// runs no `npm ci`, so this module imports Node built-ins only.

import path from "node:path";
import { spawnSync } from "node:child_process";

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
  options: { cwd?: string; capture?: boolean },
) => CommandResult;
export type Report = (message: string) => void;

const ROOT = path.resolve(import.meta.dirname, "../../../..");
const MODULE_PREFIX = "github.com/muehz/agentics/hooks/go";
const EXCLUDED = new Set([
  "./internal/shellwords",
  "./internal/commandguard",
  "./internal/secretsguard",
  "./cmd/hook",
]);

function nativeRun(
  command: string,
  args: string[],
  options: { cwd?: string; capture?: boolean },
): CommandResult {
  const result = spawnSync(command, args, {
    cwd: options.cwd,
    encoding: "utf8",
    stdio: ["inherit", options.capture ? "pipe" : "inherit", "inherit"],
    maxBuffer: 64 * 1024 * 1024,
  });
  return {
    status: result.status,
    stdout: result.stdout ?? "",
    error: result.error,
    signal: result.signal,
  };
}

// The exit status a failed command leaves for the step, reporting why when the status alone does not say.
function failure(name: string, result: CommandResult, report: Report): number {
  if (result.error) {
    report(`ci-mutation-hooks: cannot start ${name}: ${result.error.message}`);
    return 1;
  }
  if (result.signal) {
    report(`ci-mutation-hooks: ${name} was killed by ${result.signal}`);
    return 1;
  }
  return result.status && result.status !== 0 ? result.status : 1;
}

// The packages `rest` covers, or the exit status of the failed listing.
function restPackages(run: CommandRun, report: Report): string[] | number {
  const listing = run(
    "go",
    ["list", "-f", "{{if .GoFiles}}{{.ImportPath}}{{end}}", "./..."],
    { cwd: path.join(ROOT, "hooks", "go"), capture: true },
  );
  if (listing.error || listing.signal || listing.status !== 0) {
    return failure("go", listing, report);
  }
  const packages = listing.stdout
    .split(/\r?\n/)
    .filter((line) => line !== "")
    .map((line) =>
      line.startsWith(MODULE_PREFIX)
        ? `.${line.slice(MODULE_PREFIX.length)}`
        : line,
    )
    .filter((line) => !EXCLUDED.has(line));
  if (packages.length === 0) {
    report(
      "ci-mutation-hooks: no packages left to mutate after the exclusions",
    );
    return 1;
  }
  return packages;
}

// Returns the exit status of the step.
export function mutateHooks(
  env: NodeJS.ProcessEnv = process.env,
  run: CommandRun = nativeRun,
  report: Report = console.error,
): number {
  const target = env.TARGET ?? "";
  if (target === "") {
    report("ci-mutation-hooks: TARGET is not set");
    return 1;
  }
  const packages = target === "rest" ? restPackages(run, report) : [target];
  if (typeof packages === "number") return packages;
  const result = run(
    "node",
    [
      ".github/tools/src/scripts/mutation-gremlins.ts",
      "--packages",
      ...packages,
    ],
    {},
  );
  if (result.error || result.signal || result.status !== 0) {
    return failure("node", result, report);
  }
  return 0;
}

if (import.meta.main) process.exitCode = mutateHooks();
