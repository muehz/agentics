#!/usr/bin/env node
// Verifies .github/workflows/actions.lock via `gh actions-lock --verify --ignore-local-actions` and rejects any
// workspace-local (`./`) action finding outside the two intentional verifier calls in release.lock.yml.
//
// release.md intentionally checks out $GITHUB_SHA^1 on resumed releases and runs ./.github/actions/release and
// ./.github/actions/conformance from that reviewed parent workspace so a release commit cannot verify itself.
// `--ignore-local-actions` downgrades `./` globally, so this check restores strictness for every other workflow and
// path. The job runs no `npm ci`, so this module imports Node built-ins and dependency-free plugin/common.ts only.

import { spawnSync } from "node:child_process";
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
  options: { capture?: boolean; env?: Env; platform?: NodeJS.Platform },
) => CommandResult;
export type Env = Readonly<Record<string, string | undefined>>;
export type Report = (message: string) => void;

export const ALLOWED_LOCAL_ACTIONS: ReadonlySet<string> = new Set([
  ".github/workflows/release.lock.yml::cannot follow workspace-local action ./.github/actions/release",
  ".github/workflows/release.lock.yml::cannot follow workspace-local action ./.github/actions/conformance",
]);

export const VERIFY_ARGS = [
  "actions-lock",
  "--verify",
  "--ignore-local-actions",
  "--json=valid,findings",
] as const;

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

function record(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function normalizePath(value: string): string {
  return value.replaceAll("\\", "/");
}

function failure(result: CommandResult, report: Report): number {
  if (result.error) {
    report(`ci-actions-lock: cannot start gh: ${result.error.message}`);
    return 1;
  }
  if (result.signal) {
    report(`ci-actions-lock: gh was killed by ${result.signal}`);
    return 1;
  }
  return result.status && result.status !== 0 ? result.status : 1;
}

/**
 * Validates parsed `--json=valid,findings` output from `gh actions-lock`.
 * Returns human-readable violation messages; an empty array means the report passed.
 */
export function evaluateReport(raw: unknown): string[] {
  if (!record(raw)) {
    return ["gh actions-lock output is not a JSON object"];
  }
  if (typeof raw["valid"] !== "boolean") {
    return ["gh actions-lock output is missing boolean field 'valid'"];
  }
  if (!Array.isArray(raw["findings"])) {
    return ["gh actions-lock output is missing array field 'findings'"];
  }

  const violations: string[] = [];
  if (!raw["valid"]) {
    violations.push("gh actions-lock reported valid=false");
  }

  for (const item of raw["findings"]) {
    if (!record(item)) {
      violations.push("gh actions-lock finding is not a JSON object");
      continue;
    }
    const workflow =
      typeof item["workflow"] === "string"
        ? normalizePath(item["workflow"])
        : "";
    const category =
      typeof item["category"] === "string" ? item["category"] : "";
    const severity =
      typeof item["severity"] === "string" ? item["severity"] : "";
    const detail = typeof item["detail"] === "string" ? item["detail"] : "";

    if (!workflow || !category || !severity || !detail) {
      violations.push(
        `gh actions-lock finding is missing required fields: ${JSON.stringify(item)}`,
      );
      continue;
    }

    if (severity === "error") {
      violations.push(`${workflow}: [${category}] ${detail}`);
      continue;
    }

    if (category === "local-action") {
      const key = `${workflow}::${detail}`;
      if (!ALLOWED_LOCAL_ACTIONS.has(key)) {
        violations.push(
          `unexpected local action in ${workflow}: ${detail} (use $/... instead of ./...)`,
        );
      }
    }
  }

  return violations;
}

export function main(options: Options = {}): number {
  const env = options.env ?? process.env;
  const run = options.run ?? nativeRun;
  const write = options.write ?? console.log;
  const report = options.report ?? console.error;

  const result = run("gh", [...VERIFY_ARGS], {
    capture: true,
    env,
    platform: options.platform,
  });
  if (result.error || result.signal || result.status !== 0) {
    return failure(result, report);
  }

  let parsed: unknown;
  try {
    parsed = JSON.parse(result.stdout);
  } catch (error) {
    report(
      `ci-actions-lock: invalid JSON from gh actions-lock: ${(error as Error).message}`,
    );
    return 1;
  }

  const violations = evaluateReport(parsed);
  if (violations.length > 0) {
    for (const violation of violations) {
      write(`::error::${violation}`);
    }
    return 1;
  }

  write(
    "ci-actions-lock: actions.lock verified with no unexpected local actions.",
  );
  return 0;
}

if (import.meta.main) process.exitCode = main();
