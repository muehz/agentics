#!/usr/bin/env node
// Installs the pinned gh-aw binary and checks that the committed gh-aw locks match a fresh compile, for the validate
// workflow's gh-aw-lock-drift job.
//   GH_AW_VERSION=<tag> GH_AW_SHA256=<hex> RUNNER_TEMP=<dir> GITHUB_PATH=<file> node ci-gh-aw.ts install
//   GH_AW_VERSION=<tag> node ci-gh-aw.ts check
// `install` downloads the Linux release asset, verifies its SHA-256 before anything is installed, puts it at
// <RUNNER_TEMP>/bin/gh-aw (mode 0755) and appends that directory to GITHUB_PATH. `check` recompiles every gh-aw
// workflow and fails when the compile changes a tracked path or leaves an untracked one. The job runs no `npm ci`, so
// this module imports Node built-ins and the dependency-free plugin/common.ts only.

import { spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import { appendFileSync, chmodSync, mkdirSync, writeFileSync } from "node:fs";
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
  options: { capture?: boolean; env?: Env; platform?: NodeJS.Platform },
) => CommandResult;
export type Download = (url: string) => Promise<Uint8Array>;
export type Env = Readonly<Record<string, string | undefined>>;
export type Report = (message: string) => void;

// The gh-aw workflows whose locks are recompiled, in compile order.
export const LOCK_WORKFLOWS = ["release", "backlog-grooming"] as const;
// The paths a compile writes to, and so the paths that must stay unchanged.
export const LOCK_PATHS = [
  ".github/workflows",
  ".github/aw",
  ".gitattributes",
] as const;

const USAGE = "usage: ci-gh-aw.ts install|check";

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

async function nativeDownload(url: string): Promise<Uint8Array> {
  const response = await fetch(url);
  if (!response.ok) {
    throw new Error(`HTTP ${response.status} for ${url}`);
  }
  return new Uint8Array(await response.arrayBuffer());
}

export interface Options {
  env?: Env;
  platform?: NodeJS.Platform;
  download?: Download;
  run?: CommandRun;
  write?: (line: string) => void;
  report?: Report;
}

async function install(
  env: Env,
  download: Download,
  report: Report,
): Promise<number> {
  const values: Record<string, string> = {};
  for (const name of [
    "GH_AW_VERSION",
    "GH_AW_SHA256",
    "RUNNER_TEMP",
    "GITHUB_PATH",
  ]) {
    const value = env[name] ?? "";
    if (value === "") {
      report(`ci-gh-aw: ${name} is not set`);
      return 1;
    }
    values[name] = value;
  }
  const url = `https://github.com/github/gh-aw/releases/download/${values["GH_AW_VERSION"]}/linux-amd64`;
  let body: Uint8Array;
  try {
    body = await download(url);
  } catch (error) {
    report(`ci-gh-aw: download of ${url} failed: ${(error as Error).message}`);
    return 1;
  }
  const actual = createHash("sha256").update(body).digest("hex");
  if (actual !== values["GH_AW_SHA256"]!.toLowerCase()) {
    report(
      `ci-gh-aw: checksum mismatch for ${url}: expected ${values["GH_AW_SHA256"]}, got ${actual}`,
    );
    return 1;
  }
  const bin = path.join(values["RUNNER_TEMP"]!, "bin");
  mkdirSync(bin, { recursive: true });
  const target = path.join(bin, "gh-aw");
  writeFileSync(target, body);
  chmodSync(target, 0o755);
  appendFileSync(values["GITHUB_PATH"]!, `${bin}\n`);
  return 0;
}

// The exit status a failed command leaves for the step, reporting why when the status alone does not say.
function failure(name: string, result: CommandResult, report: Report): number {
  if (result.error) {
    report(`ci-gh-aw: cannot start ${name}: ${result.error.message}`);
    return 1;
  }
  if (result.signal) {
    report(`ci-gh-aw: ${name} was killed by ${result.signal}`);
    return 1;
  }
  return result.status && result.status !== 0 ? result.status : 1;
}

const succeeded = (result: CommandResult): boolean =>
  !result.error && !result.signal && result.status === 0;

function check(
  env: Env,
  runOptions: { env: Env; platform?: NodeJS.Platform },
  run: CommandRun,
  write: (line: string) => void,
  report: Report,
): number {
  for (const workflow of LOCK_WORKFLOWS) {
    const result = run("gh-aw", ["compile", workflow], runOptions);
    if (!succeeded(result)) return failure("gh-aw", result, report);
  }
  const porcelain = run("git", ["status", "--porcelain", "--", ...LOCK_PATHS], {
    ...runOptions,
    capture: true,
  });
  if (!succeeded(porcelain)) return failure("git", porcelain, report);
  if (porcelain.stdout.replace(/\n+$/, "") === "") return 0;
  for (const args of [
    ["status", "--short", "--", ...LOCK_PATHS],
    ["--no-pager", "diff", "--", ...LOCK_PATHS],
  ]) {
    run("git", args, runOptions);
  }
  write(
    `::error::A gh-aw lock is stale. Run 'gh aw compile release' and 'gh aw compile backlog-grooming' with gh-aw ${env["GH_AW_VERSION"] ?? ""} and commit the result alongside the source change.`,
  );
  return 1;
}

// Returns the exit status of the step.
export async function main(
  argv: string[],
  options: Options = {},
): Promise<number> {
  const env = options.env ?? process.env;
  const report = options.report ?? console.error;
  const mode = argv.length === 1 ? argv[0] : undefined;
  if (mode === "install") {
    return install(env, options.download ?? nativeDownload, report);
  }
  if (mode === "check") {
    return check(
      env,
      { env, platform: options.platform },
      options.run ?? nativeRun,
      options.write ?? console.log,
      report,
    );
  }
  report(USAGE);
  return 2;
}

if (import.meta.main) process.exitCode = await main(process.argv.slice(2));
