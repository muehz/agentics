import { resolve } from "node:path";
import type { Excludes, ReviewContext } from "./context.ts";
import { readRegularFile } from "./regular-file.ts";

// Single-quotes a string for the reviewer's copy-and-paste commands. An embedded single quote becomes '\'' so the
// suggested line stays valid shell.
export function shellSingleQuote(text: string): string {
  return `'${text.replaceAll("'", "'\\''")}'`;
}

export function buildExcludes(patterns: string[]): Excludes {
  const args = patterns.map((pattern) => `:(exclude,glob)${pattern}`);
  return { args, pathspecs: args.map(shellSingleQuote).join(" ") };
}

// The first 240 lines with their own terminators, as `sed -n 1,240p` prints them: a last line with no newline stays
// without one, and an empty file contributes nothing. A relative path is read from the host's working directory.
function planContents(planPath: string, cwd: string): string {
  const read = readRegularFile(resolve(cwd, planPath));
  if (read.kind !== "text") return "";
  const lines = read.text.match(/[^\n]*\n|[^\n]+$/g) ?? [];
  return `\nPlan contents:\n${lines.slice(0, 240).join("")}`;
}

// One prompt for every target.
export function buildPrompt(ctx: ReviewContext): string {
  const { options, range, exclude } = ctx;
  const summary =
    options.summary === ""
      ? "No summary provided. Infer intent from the diff and nearby documentation."
      : options.summary;
  const plan =
    options.planPath === "" ? "No plan path provided." : options.planPath;
  const body = [
    "Peer review request",
    "",
    "You are a second-model-family reviewer providing a peer review for Agentics.",
    "",
    `Range: ${range}`,
    `Summary: ${summary}`,
    `Plan: ${plan}`,
    "",
    "Inspect only; do not edit files, stage files, commit, post comments, install dependencies, or run mutating commands.",
    "Use only read-only shell inspection commands and the allowed tools provided by the launcher.",
    "",
    "Executing tests or builds is out of scope by design; the caller runs the validation suite. Do not attempt to run tests or builds, and do not report the inability to execute them as a finding.",
    "",
    "Review the selected git range against the summary or plan. Prioritize correctness, regressions, stale references, install or package behavior, and test gaps.",
    "",
    "Useful commands (the exclude pathspecs skip lockfiles and generated files — do not read or review excluded paths):",
    "git status --short",
    `git diff --stat ${range} -- . ${exclude.pathspecs}`,
    `git diff ${range} -- . ${exclude.pathspecs}`,
    `git show --stat ${options.head}`,
    "",
    "Findings first. For each finding include severity, file:line, impact, evidence, and suggested fix.",
    'If there are no substantiated findings, say "No findings" and list residual risks briefly.',
    'End your review with exactly one final line: "REVIEW-COMPLETE: findings=<n>" (n = number of findings) or "REVIEW-COMPLETE: no-findings". Output without this terminator is treated as truncated and rejected.',
  ].join("\n");
  return `${body}\n${options.planPath === "" ? "" : planContents(options.planPath, ctx.host.cwd)}`;
}
