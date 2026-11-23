#!/usr/bin/env node
// The coverage steps of the validate-coverage workflow's two report jobs.
//   RUNNER_TEMP=<dir> node .github/tools/src/scripts/ci-coverage.ts require-profiles <shard>...
//   RUNNER_TEMP=<dir> node .github/tools/src/scripts/ci-coverage.ts merge-profiles
//   GITHUB_STEP_SUMMARY=<file> node .github/tools/src/scripts/ci-coverage.ts totals
//   RUNNER_TEMP=<dir> GITHUB_WORKSPACE=<dir> GITHUB_STEP_SUMMARY=<file> node .github/tools/src/scripts/ci-coverage.ts combine-ts <shard>...
// The coverage-report job (Go): the download step leaves each shard's profile at
// <RUNNER_TEMP>/profiles/go-coverage-profile-<shard>/<shard>.out. `require-profiles` fails unless every named shard
// delivered a non-empty one, `merge-profiles` joins them into coverage-hooks.out in the working directory, and
// `totals` writes coverage-hooks.txt and the job summary from it.
// The ts-coverage-report job (TypeScript): `combine-ts` takes each shard `<workspace>-<n>`, checks its LCOV report at
// <RUNNER_TEMP>/lcov/ts-coverage-lcov-<shard>/lcov.info, converts the reports to cobertura-typescript.xml with the
// coverage CLI and appends the per-workspace summary to the job summary.
// Neither job runs `npm ci`, so this module imports Node built-ins and the dependency-free plugin/common.ts only.

import { spawnSync } from "node:child_process";
import { appendFileSync, readFileSync, statSync, writeFileSync } from "node:fs";
import path from "node:path";
import { findCommand, spawnPlan } from "../plugin/common.ts";

export interface CommandResult {
  status: number | null;
  stdout: string;
  error?: Error;
  signal?: NodeJS.Signals | null;
}
export type CommandRun = (
  command: string,
  args: string[],
  options: {
    cwd?: string;
    env?: Env;
    platform?: NodeJS.Platform;
    // Pipes stdout into the result; without it the child writes straight to this process's output.
    capture?: boolean;
  },
) => CommandResult;
export type Env = Readonly<Record<string, string | undefined>>;
export type Report = (message: string) => void;

// The shards `merge-profiles` joins, in order. They are the go-suite matrix's shards.
export const MODULE_SHARDS = ["hooks"];

const USAGE =
  "usage: ci-coverage.ts require-profiles|merge-profiles|totals|combine-ts";

const nativeRun: CommandRun = (command, args, options) => {
  const env = options.env ?? process.env;
  const platform = options.platform ?? process.platform;
  const file = findCommand(command, env, platform) ?? command;
  const plan = spawnPlan(file, args, platform, env);
  const result = spawnSync(plan.file, plan.args, {
    cwd: options.cwd,
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
  cwd?: string;
  // The shards `merge-profiles` joins; the workflow's go-suite matrix by default.
  shards?: readonly string[];
  run?: CommandRun;
  write?: (line: string) => void;
  report?: Report;
}

const profilePath = (temp: string, shard: string): string =>
  path.join(temp, "profiles", `go-coverage-profile-${shard}`, `${shard}.out`);

function requireProfiles(
  shards: string[],
  temp: string,
  write: (line: string) => void,
): number {
  let missing = false;
  for (const shard of shards) {
    let size = 0;
    try {
      size = statSync(profilePath(temp, shard)).size;
    } catch {
      size = 0;
    }
    if (size === 0) {
      write(
        `::error::No coverage profile from the ${shard} shard; its tests failed or did not run.`,
      );
      missing = true;
    }
  }
  return missing ? 1 : 0;
}

// The first line of the first profile, then every profile's lines after its first.
function mergeProfiles(
  shards: readonly string[],
  temp: string,
  cwd: string,
  report: Report,
): number {
  let header = "";
  let body = "";
  for (const [index, shard] of shards.entries()) {
    const file = profilePath(temp, shard);
    let text: string;
    try {
      text = readFileSync(file, "utf8");
    } catch (error) {
      report(`ci-coverage: cannot read ${file}: ${(error as Error).message}`);
      return 1;
    }
    const newline = text.indexOf("\n");
    if (index === 0) header = newline < 0 ? text : text.slice(0, newline + 1);
    body += newline < 0 ? "" : text.slice(newline + 1);
  }
  writeFileSync(path.join(cwd, "coverage-hooks.out"), header + body);
  return 0;
}

function totals(
  options: { env: Env; platform?: NodeJS.Platform },
  cwd: string,
  run: CommandRun,
  report: Report,
): number {
  const summary = options.env["GITHUB_STEP_SUMMARY"] ?? "";
  if (summary === "") {
    report("ci-coverage: GITHUB_STEP_SUMMARY is not set");
    return 1;
  }
  const result = run(
    "go",
    ["-C", "hooks/go", "tool", "cover", "-func=../../coverage-hooks.out"],
    { cwd, capture: true, ...options },
  );
  // The upload step needs the file even when the listing failed, as a shell redirect would leave it.
  writeFileSync(path.join(cwd, "coverage-hooks.txt"), result.stdout);
  if (result.error) {
    report(`ci-coverage: cannot start go: ${result.error.message}`);
    return 1;
  }
  if (result.signal) {
    report(`ci-coverage: go was killed by ${result.signal}`);
    return 1;
  }
  if (result.status !== 0) return result.status ?? 1;
  const lines = result.stdout.split("\n");
  while (lines.length > 0 && lines[lines.length - 1] === "") lines.pop();
  const last = lines[lines.length - 1] ?? "";
  const total = last.trim().split(/\s+/).pop() ?? "";
  appendFileSync(
    summary,
    [
      "### Go statement coverage",
      "",
      "These totals combine all package tests. Individual test log percentages use the whole module as their denominator.",
      "The uploaded Cobertura report measures line coverage, so its percentage can differ.",
      "",
      "| Module | Statement coverage |",
      "| --- | --- |",
      `| \`hooks/go\` | ${total} |`,
      "",
    ].join("\n"),
  );
  return 0;
}

const succeeded = (result: CommandResult): boolean =>
  !result.error && !result.signal && result.status === 0;

// The exit status a failed command leaves for the step, reporting why when the status alone does not say.
function failure(name: string, result: CommandResult, report: Report): number {
  if (result.error) {
    report(`ci-coverage: cannot start ${name}: ${result.error.message}`);
    return 1;
  }
  if (result.signal) {
    report(`ci-coverage: ${name} was killed by ${result.signal}`);
    return 1;
  }
  return result.status && result.status !== 0 ? result.status : 1;
}

// Converts every shard's LCOV report to one Cobertura file and appends the per-workspace summary. Each shard is named
// `<workspace>-<n>`; its report is at <RUNNER_TEMP>/lcov/ts-coverage-lcov-<shard>/lcov.info and covers `.github/<workspace>`.
function combineTs(
  shards: string[],
  env: Env,
  cwd: string,
  run: CommandRun,
  write: (line: string) => void,
  report: Report,
): number {
  const temp = env["RUNNER_TEMP"] ?? "";
  const workspace = env["GITHUB_WORKSPACE"] ?? "";
  const summary = env["GITHUB_STEP_SUMMARY"] ?? "";
  for (const [name, value] of [
    ["RUNNER_TEMP", temp],
    ["GITHUB_WORKSPACE", workspace],
    ["GITHUB_STEP_SUMMARY", summary],
  ] as const) {
    if (value === "") {
      report(`ci-coverage: ${name} is not set`);
      return 1;
    }
  }
  const inputs: string[] = [];
  for (const shard of shards) {
    const file = path.join(
      temp,
      "lcov",
      `ts-coverage-lcov-${shard}`,
      "lcov.info",
    );
    let size = 0;
    try {
      size = statSync(file).size;
    } catch {
      size = 0;
    }
    if (size === 0) {
      write(`::error::Missing or empty LCOV report for ${shard}.`);
      return 1;
    }
    const dash = shard.lastIndexOf("-");
    inputs.push(
      "--input",
      `.github/${dash < 0 ? shard : shard.slice(0, dash)}:${file}`,
    );
  }
  const cli = ".github/tools/src/coverage/cli.ts";
  const options = { cwd, env };
  const lcov = run(
    process.execPath,
    [
      cli,
      "lcov",
      "--root",
      workspace,
      ...inputs,
      "--out",
      "cobertura-typescript.xml",
    ],
    options,
  );
  if (!succeeded(lcov)) return failure("node", lcov, report);
  const listing = run(
    process.execPath,
    [cli, "summary", "--root", workspace, ...inputs],
    { ...options, capture: true },
  );
  // The summary is appended even when the command failed, as a shell redirect would leave it.
  appendFileSync(summary, listing.stdout);
  return succeeded(listing) ? 0 : failure("node", listing, report);
}

// Returns the exit status of the step.
export function main(argv: string[], options: Options = {}): number {
  const env = options.env ?? process.env;
  const cwd = options.cwd ?? process.cwd();
  const report = options.report ?? console.error;
  const write = options.write ?? console.log;
  const [mode, ...rest] = argv;
  if (
    (mode !== "require-profiles" &&
      mode !== "merge-profiles" &&
      mode !== "totals" &&
      mode !== "combine-ts") ||
    (mode === "require-profiles" || mode === "combine-ts"
      ? rest.length === 0
      : rest.length > 0)
  ) {
    report(USAGE);
    return 2;
  }
  if (mode === "combine-ts") {
    return combineTs(rest, env, cwd, options.run ?? nativeRun, write, report);
  }
  if (mode === "totals") {
    return totals(
      { env, platform: options.platform },
      cwd,
      options.run ?? nativeRun,
      report,
    );
  }
  const temp = env["RUNNER_TEMP"] ?? "";
  if (temp === "") {
    report("ci-coverage: RUNNER_TEMP is not set");
    return 1;
  }
  if (mode === "require-profiles") return requireProfiles(rest, temp, write);
  return mergeProfiles(options.shards ?? MODULE_SHARDS, temp, cwd, report);
}

if (import.meta.main) process.exitCode = main(process.argv.slice(2));
