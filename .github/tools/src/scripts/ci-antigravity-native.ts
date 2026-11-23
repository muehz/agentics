#!/usr/bin/env node
// Installs the pinned agy CLI and validates the repository as an Antigravity plugin, for the validate workflow's
// validate-antigravity-native job.
//   AGY_URL=<url> AGY_SHA512=<hex> RUNNER_TEMP=<dir> GITHUB_PATH=<file> node ci-antigravity-native.ts install
//   node ci-antigravity-native.ts check
// `install` checks that AGY_URL matches the compile-time pinned Linux x64 release tarball, downloads it with `curl`
// into a private temporary directory, verifies its SHA-512 before extracting anything, extracts only the `antigravity`
// binary with `tar -xzf`, installs it at <RUNNER_TEMP>/bin/agy (mode 0755), removes the temporary directory, and
// appends <RUNNER_TEMP>/bin to GITHUB_PATH. `check` runs `agy plugin validate --json .` and rejects any error or
// unexpected warning while allowing notes and the intentional root AGENTS.md warning. The job runs no `npm ci`, so
// this module imports Node built-ins only.

import { spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import {
  appendFileSync,
  chmodSync,
  copyFileSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  rmSync,
} from "node:fs";
import os from "node:os";
import path from "node:path";

export type AllowedCommand = "curl" | "tar" | "agy";

export interface CommandResult {
  status: number | null;
  stdout: string;
  error?: Error;
  signal?: NodeJS.Signals | null;
}
// `capture` pipes stdout into the result; without it the child writes straight to this process's output.
export type CommandRun = (
  command: AllowedCommand,
  args: readonly string[],
  options: {
    capture?: boolean;
    cwd?: string;
    env?: Env;
    platform?: NodeJS.Platform;
  },
) => CommandResult;
export type Env = Readonly<Record<string, string | undefined>>;
export type Report = (message: string) => void;

export const PINNED_AGY_URL =
  "https://storage.googleapis.com/antigravity-public/antigravity-cli/1.5.13-5999302598852608/linux-x64/cli_linux_x64.tar.gz";
export const AGY_ARCHIVE_MEMBER = "antigravity";

const ARCHIVE_FILE = "agy.tar.gz";
const EXTRACT_DIR = "extract";
const CURL_ARGS: readonly string[] = [
  "-fsSL",
  "-o",
  ARCHIVE_FILE,
  PINNED_AGY_URL,
];
const TAR_ARGS: readonly string[] = [
  "-xzf",
  ARCHIVE_FILE,
  "-C",
  EXTRACT_DIR,
  AGY_ARCHIVE_MEMBER,
];
const CHECK_ARGS: readonly string[] = ["plugin", "validate", "--json", "."];
const USAGE = "usage: ci-antigravity-native.ts install|check";

const nativeRun: CommandRun = (command, args, options) => {
  const result = spawnSync(command, args, {
    cwd: options.cwd,
    env: options.env ?? process.env,
    encoding: "utf8",
    shell: false,
    stdio: ["inherit", options.capture ? "pipe" : "inherit", "inherit"],
    maxBuffer: 64 * 1024 * 1024,
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

// The exit status a failed command leaves for the step, reporting why when the status alone does not say.
function failure(name: string, result: CommandResult, report: Report): number {
  if (result.error) {
    report(
      `ci-antigravity-native: cannot start ${name}: ${result.error.message}`,
    );
    return 1;
  }
  if (result.signal) {
    report(`ci-antigravity-native: ${name} was killed by ${result.signal}`);
    return 1;
  }
  return result.status && result.status !== 0 ? result.status : 1;
}

const succeeded = (result: CommandResult): boolean =>
  !result.error && !result.signal && result.status === 0;

function install(
  env: Env,
  runOptions: { env: Env; platform?: NodeJS.Platform },
  run: CommandRun,
  report: Report,
): number {
  const values: Record<string, string> = {};
  for (const name of ["AGY_URL", "AGY_SHA512", "RUNNER_TEMP", "GITHUB_PATH"]) {
    const value = env[name] ?? "";
    if (value === "") {
      report(`ci-antigravity-native: ${name} is not set`);
      return 1;
    }
    values[name] = value;
  }
  if (values["AGY_URL"] !== PINNED_AGY_URL) {
    report(
      `ci-antigravity-native: AGY_URL does not match pinned release URL ${PINNED_AGY_URL}`,
    );
    return 1;
  }
  const workDir = mkdtempSync(path.join(os.tmpdir(), "agentics-agy-"));
  const extractDir = path.join(workDir, EXTRACT_DIR);
  const archive = path.join(workDir, ARCHIVE_FILE);
  const bin = path.join(values["RUNNER_TEMP"]!, "bin");
  try {
    mkdirSync(extractDir, { recursive: true });
    const downloaded = run("curl", CURL_ARGS, { ...runOptions, cwd: workDir });
    if (!succeeded(downloaded)) return failure("curl", downloaded, report);
    let body: Buffer;
    try {
      body = readFileSync(archive);
    } catch (error) {
      report(
        `ci-antigravity-native: failed to read downloaded archive: ${(error as Error).message}`,
      );
      return 1;
    }
    const actual = createHash("sha512").update(body).digest("hex");
    if (actual !== values["AGY_SHA512"]!.toLowerCase()) {
      report(
        `ci-antigravity-native: checksum mismatch for ${PINNED_AGY_URL}: expected ${values["AGY_SHA512"]}, got ${actual}`,
      );
      return 1;
    }
    const extracted = run("tar", TAR_ARGS, { ...runOptions, cwd: workDir });
    if (!succeeded(extracted)) return failure("tar", extracted, report);
    mkdirSync(bin, { recursive: true });
    const target = path.join(bin, "agy");
    copyFileSync(path.join(extractDir, AGY_ARCHIVE_MEMBER), target);
    chmodSync(target, 0o755);
  } finally {
    rmSync(workDir, { recursive: true, force: true });
  }
  appendFileSync(values["GITHUB_PATH"]!, `${bin}\n`);
  return 0;
}

export type ValidationResult =
  { ok: true; summary: string } | { ok: false; errors: string[] };

function isRecord(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function isAllowedFinding(finding: Record<string, unknown>): boolean {
  if (finding.severity === "note") return true;
  // Root AGENTS.md is the repository-level instruction file for Codex, Claude Code, and Copilot.
  if (finding.severity === "warning" && finding.file === "AGENTS.md")
    return true;
  return false;
}

/** Validates the parsed JSON output of `agy plugin validate --json .`. */
export function validateReport(payload: unknown): ValidationResult {
  if (!isRecord(payload) || !Array.isArray(payload.findings)) {
    return {
      ok: false,
      errors: ["agy plugin validate output is missing a findings array."],
    };
  }
  const components = payload.components;
  if (
    !isRecord(components) ||
    typeof components.skills !== "number" ||
    !(components.skills >= 1)
  ) {
    return {
      ok: false,
      errors: ["agy plugin validate reported no skills."],
    };
  }
  const unexpected = payload.findings.filter(
    (item) => !isRecord(item) || !isAllowedFinding(item),
  );
  if (unexpected.length > 0) {
    const errors = ["Unexpected agy plugin validate findings:"];
    for (const item of unexpected) {
      const f = isRecord(item) ? item : undefined;
      errors.push(`  [${f?.severity}] ${f?.file}: ${f?.message}`);
    }
    return { ok: false, errors };
  }
  return {
    ok: true,
    summary: `Antigravity plugin valid: ${components.skills} skills, ${payload.findings.length} allowed finding(s).`,
  };
}

function check(
  runOptions: { env: Env; platform?: NodeJS.Platform },
  run: CommandRun,
  write: (line: string) => void,
  report: Report,
): number {
  const result = run("agy", CHECK_ARGS, {
    ...runOptions,
    capture: true,
  });
  if (!succeeded(result)) {
    if (!result.error && !result.signal && result.stdout !== "") {
      report(result.stdout.replace(/\n$/, ""));
    }
    return failure("agy", result, report);
  }
  let parsed: unknown;
  try {
    parsed = JSON.parse(result.stdout);
  } catch (error) {
    report(
      `ci-antigravity-native: invalid JSON from agy plugin validate: ${(error as Error).message}`,
    );
    return 1;
  }
  const outcome = validateReport(parsed);
  if (!outcome.ok) {
    for (const line of outcome.errors) report(line);
    return 1;
  }
  write(outcome.summary);
  return 0;
}

// Returns the exit status of the step.
export function main(argv: string[], options: Options = {}): number {
  const env = options.env ?? process.env;
  const report = options.report ?? console.error;
  const run = options.run ?? nativeRun;
  const runOptions = { env, platform: options.platform };
  const mode = argv.length === 1 ? argv[0] : undefined;
  if (mode === "install") {
    return install(env, runOptions, run, report);
  }
  if (mode === "check") {
    return check(runOptions, run, options.write ?? console.log, report);
  }
  report(USAGE);
  return 2;
}

if (import.meta.main) process.exitCode = main(process.argv.slice(2));
