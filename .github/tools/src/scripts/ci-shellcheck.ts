#!/usr/bin/env node
// Installs the pinned shellcheck and lints the repository's shell scripts, for the validate workflow's shellcheck job.
//   SHELLCHECK_VERSION=<tag> SHELLCHECK_SHA256=<hex> RUNNER_TEMP=<dir> GITHUB_PATH=<file> node ci-shellcheck.ts install
//   node ci-shellcheck.ts check
// `install` downloads the Linux release tarball, verifies its SHA-256 before extracting anything, extracts only the
// shellcheck binary with the system `tar -xJf` (Node has no xz decoder), puts it at <RUNNER_TEMP>/bin/shellcheck (mode
// 0755) and appends that directory to GITHUB_PATH. `check` lints every tracked `*.sh` except the two frozen copies of
// the retired Bash maintainer entrypoint and its stage check in the hook corpus: they source libraries that no longer
// exist, so -x cannot follow them. The other corpus scripts are still checked. The job runs no `npm ci`, so this module
// imports Node built-ins and the dependency-free plugin/common.ts only.

import { spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import {
  appendFileSync,
  chmodSync,
  copyFileSync,
  mkdirSync,
  writeFileSync,
} from "node:fs";
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
    cwd?: string;
    env?: Env;
    platform?: NodeJS.Platform;
  },
) => CommandResult;
export type Download = (url: string) => Promise<Uint8Array>;
export type Env = Readonly<Record<string, string | undefined>>;
export type Report = (message: string) => void;

// The pathspecs `git ls-files` takes to list the scripts to lint: every shell script except the two frozen copies.
export const SHELLCHECK_PATHSPECS = [
  "*.sh",
  ":!:hooks/go/cmd/hook/testdata/corpus/scripts/plugin.sh",
  ":!:hooks/go/cmd/hook/testdata/corpus/scripts/verify-stage.sh",
] as const;

const USAGE = "usage: ci-shellcheck.ts install|check";

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

// The exit status a failed command leaves for the step, reporting why when the status alone does not say.
function failure(name: string, result: CommandResult, report: Report): number {
  if (result.error) {
    report(`ci-shellcheck: cannot start ${name}: ${result.error.message}`);
    return 1;
  }
  if (result.signal) {
    report(`ci-shellcheck: ${name} was killed by ${result.signal}`);
    return 1;
  }
  return result.status && result.status !== 0 ? result.status : 1;
}

const succeeded = (result: CommandResult): boolean =>
  !result.error && !result.signal && result.status === 0;

async function install(
  env: Env,
  runOptions: { env: Env; platform?: NodeJS.Platform },
  download: Download,
  run: CommandRun,
  report: Report,
): Promise<number> {
  const values: Record<string, string> = {};
  for (const name of [
    "SHELLCHECK_VERSION",
    "SHELLCHECK_SHA256",
    "RUNNER_TEMP",
    "GITHUB_PATH",
  ]) {
    const value = env[name] ?? "";
    if (value === "") {
      report(`ci-shellcheck: ${name} is not set`);
      return 1;
    }
    values[name] = value;
  }
  const version = values["SHELLCHECK_VERSION"]!;
  const temp = values["RUNNER_TEMP"]!;
  const url = `https://github.com/koalaman/shellcheck/releases/download/${version}/shellcheck-${version}.linux.x86_64.tar.xz`;
  let body: Uint8Array;
  try {
    body = await download(url);
  } catch (error) {
    report(
      `ci-shellcheck: download of ${url} failed: ${(error as Error).message}`,
    );
    return 1;
  }
  const actual = createHash("sha256").update(body).digest("hex");
  if (actual !== values["SHELLCHECK_SHA256"]!.toLowerCase()) {
    report(
      `ci-shellcheck: checksum mismatch for ${url}: expected ${values["SHELLCHECK_SHA256"]}, got ${actual}`,
    );
    return 1;
  }
  const archive = path.join(temp, "shellcheck.tar.xz");
  mkdirSync(temp, { recursive: true });
  writeFileSync(archive, body);
  const member = `shellcheck-${version}/shellcheck`;
  const extracted = run("tar", ["-xJf", archive, member], {
    ...runOptions,
    cwd: temp,
  });
  if (!succeeded(extracted)) return failure("tar", extracted, report);
  const bin = path.join(temp, "bin");
  mkdirSync(bin, { recursive: true });
  const target = path.join(bin, "shellcheck");
  copyFileSync(path.join(temp, member), target);
  chmodSync(target, 0o755);
  appendFileSync(values["GITHUB_PATH"]!, `${bin}\n`);
  return 0;
}

function check(
  runOptions: { env: Env; platform?: NodeJS.Platform },
  run: CommandRun,
  write: (line: string) => void,
  report: Report,
): number {
  const version = run("shellcheck", ["--version"], runOptions);
  if (!succeeded(version)) return failure("shellcheck", version, report);
  const listing = run("git", ["ls-files", ...SHELLCHECK_PATHSPECS], {
    ...runOptions,
    capture: true,
  });
  if (!succeeded(listing)) return failure("git", listing, report);
  const scripts = listing.stdout.split(/\r?\n/).filter((line) => line !== "");
  const result = run(
    "shellcheck",
    ["-x", "-S", "style", ...scripts],
    runOptions,
  );
  if (result.error || result.signal)
    return failure("shellcheck", result, report);
  if (result.status === 0) return 0;
  const specs = SHELLCHECK_PATHSPECS.map((spec) => `'${spec}'`).join(" ");
  write(
    `::error::Shellcheck findings. Local repro: docker run --rm -v "$PWD:/src" -w /src koalaman/shellcheck:v0.11.0 -x -S style $(git ls-files ${specs})`,
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
  const run = options.run ?? nativeRun;
  const runOptions = { env, platform: options.platform };
  const mode = argv.length === 1 ? argv[0] : undefined;
  if (mode === "install") {
    return install(
      env,
      runOptions,
      options.download ?? nativeDownload,
      run,
      report,
    );
  }
  if (mode === "check") {
    return check(runOptions, run, options.write ?? console.log, report);
  }
  report(USAGE);
  return 2;
}

if (import.meta.main) process.exitCode = await main(process.argv.slice(2));
