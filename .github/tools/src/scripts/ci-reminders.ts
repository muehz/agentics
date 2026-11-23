#!/usr/bin/env node

// Opens the upstream-docs review tracking issue, or comments on the open one. The reminders workflow runs this when the
// staleness check reports a stale review. The token and the run URL come from the environment:
//   GH_TOKEN=<token> RUN_URL=<run url> node .github/tools/src/scripts/ci-reminders.ts
// A failed gh call exits 1 with the error on stderr, which fails the step and reaches its log.

import { spawnSync } from "node:child_process";

export const TITLE = "Surface upstream-docs review due";

export interface GhResult {
  status: number | null;
  stdout: string;
  stderr: string;
}
export type Gh = (args: string[], env: NodeJS.ProcessEnv) => GhResult;

export function reminderBody(runUrl: string): string {
  return `The upstream review ledger (docs/surfaces/README.md) has surfaces over the 30-day review budget.\n\nRun the review playbook in that file for each stale surface, then update its ledger row.\n\nFailure detail: ${runUrl} (see the "Run staleness check" step log).`;
}

function runGh(args: string[], env: NodeJS.ProcessEnv): GhResult {
  const result = spawnSync("gh", args, { env, encoding: "utf8" });
  if (result.error) {
    return { status: null, stdout: "", stderr: result.error.message };
  }
  return {
    status: result.status,
    stdout: result.stdout ?? "",
    stderr: result.stderr ?? "",
  };
}

function check(result: GhResult, args: string[]): string {
  if (result.status !== 0) {
    throw new Error(
      `ci-reminders: gh ${args.slice(0, 2).join(" ")} failed: ${result.stderr.trim()}`,
    );
  }
  return result.stdout;
}

export function openOrUpdate(
  env: NodeJS.ProcessEnv = process.env,
  gh: Gh = runGh,
): string {
  const body = reminderBody(env.RUN_URL ?? "");
  const listArgs = [
    "issue",
    "list",
    "--state",
    "open",
    "--search",
    `in:title "${TITLE}"`,
    "--json",
    "number",
    "--jq",
    ".[0].number // empty",
  ];
  const number = check(gh(listArgs, env), listArgs).trim();
  if (number) {
    const commentArgs = ["issue", "comment", number, "--body", body];
    check(gh(commentArgs, env), commentArgs);
    return `Commented on existing issue #${number}`;
  }
  const createArgs = ["issue", "create", "--title", TITLE, "--body", body];
  return check(gh(createArgs, env), createArgs).trim();
}

export interface ReminderOutcome {
  exitCode: number;
  stdout: string;
  stderr: string;
}

// The entry point's contract: the step output on stdout, and on failure the error on stderr with exit code 1.
export function reminder(
  env: NodeJS.ProcessEnv = process.env,
  gh: Gh = runGh,
): ReminderOutcome {
  try {
    return { exitCode: 0, stdout: `${openOrUpdate(env, gh)}\n`, stderr: "" };
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    return { exitCode: 1, stdout: "", stderr: `${message}\n` };
  }
}

if (import.meta.main) {
  const outcome = reminder();
  process.stdout.write(outcome.stdout);
  process.stderr.write(outcome.stderr);
  process.exitCode = outcome.exitCode;
}
