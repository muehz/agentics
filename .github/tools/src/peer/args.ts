import { DEFAULT_EXCLUDES, type Target } from "./constants.ts";

export type TargetChoice = "auto" | "all" | Target;

export interface Options {
  target: TargetChoice;
  base: string;
  head: string;
  planPath: string;
  summary: string;
  timeout: number;
  output: string;
  excludes: string[];
  force: boolean;
  dryRun: boolean;
}

export type Parsed = { kind: "help" } | { kind: "run"; options: Options };

export class UsageError extends Error {
  readonly showUsage: boolean;
  constructor(message: string, showUsage = false) {
    super(message);
    this.showUsage = showUsage;
  }
}

export const USAGE = `Usage: peer [--target auto|all|claude|codex] [--base <rev>] [--head <rev>]
           [--plan <path>] [--summary <text>] [--timeout <seconds>] [--output <path>]
           [--exclude <glob>] [--force] [--dry-run]

Requests a peer review from an alternate model family. Claude and Codex
use strict non-mutation controls.

Auto-detection (default):
  Running in Claude Code → dispatches to Codex
  Running in Codex       → dispatches to Claude
  Running in Gemini/agy  → dispatches to Claude + Codex (parallel)

Explicit targets:
  all     Dispatch to all alternate Claude/Codex platforms
  claude  Dispatch via claude -p with read-only tool constraints
  codex   Dispatch via codex exec with --sandbox read-only

Options:
  --timeout <seconds>  Max time per review (default 300)
  --output <path>      Findings file (single-target) or directory (multi-target).
                       Default: <cwd>/docs/reviews/peer/<utc-timestamp>-<pid>-<range>/<target>.md
  --exclude <glob>     Add a diff exclusion (repeatable; defaults cover lockfiles and generated dirs)
  --force              Dispatch even when the post-exclusion diff exceeds the size threshold

Exit codes:
  0 success   1 dispatch/preflight failure   2 usage error   3 nothing to review
  4 output validation failed (empty/truncated/missing REVIEW-COMPLETE)
  124 review timed out
`;

const TARGET_CHOICES: string[] = ["auto", "all", "claude", "codex"];

export function parseArgs(argv: string[]): Parsed {
  const options: Options = {
    target: "auto",
    base: "HEAD~1",
    head: "HEAD",
    planPath: "",
    summary: "",
    timeout: 300,
    output: "",
    excludes: [...DEFAULT_EXCLUDES],
    force: false,
    dryRun: false,
  };
  let index = 0;
  while (index < argv.length) {
    const flag = argv[index] ?? "";
    // A value-taking flag needs one more argument after it.
    const value = (): string => {
      if (argv.length - index < 2)
        throw new UsageError(`${flag} requires a value`);
      return argv[index + 1] ?? "";
    };
    switch (flag) {
      case "--base":
        options.base = value();
        index += 2;
        break;
      case "--head":
        options.head = value();
        index += 2;
        break;
      case "--plan":
        options.planPath = value();
        index += 2;
        break;
      case "--summary":
        options.summary = value();
        index += 2;
        break;
      case "--output":
        options.output = value();
        index += 2;
        break;
      case "--exclude":
        options.excludes.push(value());
        index += 2;
        break;
      case "--timeout": {
        const seconds = value();
        // Spelling alone is not enough: a long digit string converts to Infinity or a rounded value, and an
        // infinite timeout would let a hung reviewer run forever.
        const parsed = Number(seconds);
        if (!/^[1-9][0-9]*$/.test(seconds) || !Number.isSafeInteger(parsed))
          throw new UsageError(
            "--timeout must be a positive integer of seconds",
          );
        options.timeout = parsed;
        index += 2;
        break;
      }
      case "--target": {
        const choice = value();
        if (!TARGET_CHOICES.includes(choice))
          throw new UsageError(
            "--target must be 'auto', 'all', 'claude', or 'codex'",
          );
        options.target = choice as TargetChoice;
        index += 2;
        break;
      }
      case "--force":
        options.force = true;
        index += 1;
        break;
      case "--dry-run":
        options.dryRun = true;
        index += 1;
        break;
      case "-h":
      case "--help":
        return { kind: "help" };
      default:
        throw new UsageError(`unknown argument: ${flag}`, true);
    }
  }
  return { kind: "run", options };
}
