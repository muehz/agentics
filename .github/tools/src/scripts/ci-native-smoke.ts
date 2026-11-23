#!/usr/bin/env node
// The build and architecture checks of the manual validation's native-hook-smoke job.
//   RUNNER_OS=<os> RUNNER_ARCH=<arch> node .github/tools/src/scripts/ci-native-smoke.ts build
//   node .github/tools/src/scripts/ci-native-smoke.ts arch macos
//   node .github/tools/src/scripts/ci-native-smoke.ts arch windows
// `build` compiles only the hook binary for the runner it runs on, checks it against hooks/SHA256SUMS and requires the
// rebuild to leave the tracked files unchanged. The job runs no `npm ci`, so this module imports Node built-ins and
// the dependency-free plugin/common.ts only.

import { spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
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
export type Env = Readonly<Record<string, string | undefined>>;
export type Report = (message: string) => void;

export interface Host {
  os: string;
  arch: string;
  ext: string;
}

// The runner identities the job builds on, as GitHub names them, and the binary each one runs.
export const HOST_TABLE: Readonly<Record<string, Host>> = {
  "Linux/X64": { os: "linux", arch: "amd64", ext: "" },
  "Linux/ARM64": { os: "linux", arch: "arm64", ext: "" },
  "macOS/X64": { os: "darwin", arch: "amd64", ext: "" },
  "macOS/ARM64": { os: "darwin", arch: "arm64", ext: "" },
  "Windows/X64": { os: "windows", arch: "amd64", ext: ".exe" },
  "Windows/ARM64": { os: "windows", arch: "arm64", ext: ".exe" },
};

// Forces the rebuild of the one named binary, never every binary.
export const makeArgs = (name: string): string[] => [
  "-B",
  "-C",
  "hooks/go",
  `../bin/${name}`,
];

const USAGE = "usage: ci-native-smoke.ts build|arch macos|arch windows";

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
  run?: CommandRun;
  write?: (line: string) => void;
  report?: Report;
}

const succeeded = (result: CommandResult): boolean =>
  !result.error && !result.signal && result.status === 0;

// The exit status a failed command leaves for the step, reporting why when the status alone does not say.
function failure(name: string, result: CommandResult, report: Report): number {
  if (result.error) {
    report(`ci-native-smoke: cannot start ${name}: ${result.error.message}`);
    return 1;
  }
  if (result.signal) {
    report(`ci-native-smoke: ${name} was killed by ${result.signal}`);
    return 1;
  }
  return result.status && result.status !== 0 ? result.status : 1;
}

function build(
  env: Env,
  cwd: string,
  run: CommandRun,
  report: Report,
  platform: NodeJS.Platform | undefined,
): number {
  const runnerOs = env["RUNNER_OS"] ?? "";
  const runnerArch = env["RUNNER_ARCH"] ?? "";
  const key = `${runnerOs}/${runnerArch}`;
  if (!Object.hasOwn(HOST_TABLE, key)) {
    report(`unsupported host ${key}`);
    return 1;
  }
  const host = HOST_TABLE[key]!;
  const name = `agx-${host.os}-${host.arch}${host.ext}`;
  const runOptions = { cwd, env, platform };

  const made = run("make", makeArgs(name), runOptions);
  if (!succeeded(made)) return failure("make", made, report);

  const manifest = path.join(cwd, "hooks", "SHA256SUMS");
  const binary = path.join(cwd, "hooks", "bin", name);
  let sums: string;
  let contents: Buffer;
  try {
    sums = readFileSync(manifest, "utf8");
    contents = readFileSync(binary);
  } catch (error) {
    report(`ci-native-smoke: cannot read ${(error as Error).message}`);
    return 1;
  }
  const want = sums
    .split(/\r?\n/)
    .map((line) => line.trim().split(/\s+/))
    .find((fields) => fields[1] === `bin/${name}`)?.[0];
  if (want === undefined || want === "") {
    report(`ci-native-smoke: hooks/SHA256SUMS has no entry for bin/${name}`);
    return 1;
  }
  const have = createHash("sha256").update(contents).digest("hex");
  if (have !== want) {
    report(
      `ci-native-smoke: hash of hooks/bin/${name} is ${have}, but hooks/SHA256SUMS records ${want}`,
    );
    return 1;
  }

  const diff = run(
    "git",
    ["diff", "--exit-code", "--", "hooks/SHA256SUMS", `hooks/bin/${name}`],
    runOptions,
  );
  return succeeded(diff) ? 0 : failure("git", diff, report);
}

function archMacos(
  env: Env,
  cwd: string,
  run: CommandRun,
  write: (line: string) => void,
  report: Report,
  platform: NodeJS.Platform | undefined,
): number {
  const result = run("uname", ["-m"], { cwd, env, platform, capture: true });
  if (!succeeded(result)) return failure("uname", result, report);
  const arch = result.stdout.trim();
  write(`uname -m: ${arch}`);
  if (arch !== "x86_64") {
    write(`::error::expected x86_64 on macos-15-intel, got ${arch}`);
    return 1;
  }
  return 0;
}

// The value is this Node process's own architecture, so the check proves the runner only when Node is native: the
// setup-node step installs the runner's native Node on windows-11-arm, whereas an emulated x64 Node would report AMD64.
function archWindows(
  env: Env,
  write: (line: string) => void,
  report: Report,
): number {
  const value = env["PROCESSOR_ARCHITECTURE"] ?? "";
  write(`PROCESSOR_ARCHITECTURE: ${value}`);
  if (value !== "ARM64") {
    report(`expected ARM64 on windows-11-arm, got ${value}`);
    return 1;
  }
  return 0;
}

// Returns the exit status of the step.
export function main(argv: string[], options: Options = {}): number {
  const env = options.env ?? process.env;
  const cwd = options.cwd ?? process.cwd();
  const run = options.run ?? nativeRun;
  const write = options.write ?? console.log;
  const report = options.report ?? console.error;
  const command = argv.join(" ");
  if (command === "build") {
    return build(env, cwd, run, report, options.platform);
  }
  if (command === "arch macos") {
    return archMacos(env, cwd, run, write, report, options.platform);
  }
  if (command === "arch windows") return archWindows(env, write, report);
  report(USAGE);
  return 2;
}

if (import.meta.main) process.exitCode = main(process.argv.slice(2));
