import { existsSync } from "node:fs";
import path from "node:path";

/** One rule violation found in the audited checkout. `path` is "" for a repo-level rule. */
export interface Failure {
  rule: string;
  path: string;
  message: string;
}

/**
 * A domain's outcome. `failures` blame the checkout. `error` blames the auditor: the audit could not run. The two
 * stay separate so an auditor that cannot run never reads as a pass.
 */
export interface AuditResult {
  failures: Failure[];
  error?: string;
}

export interface RunOptions {
  /** Skip the gates that spawn a full toolchain run (today only tools-module-test). */
  short: boolean;
  /** Implies short and also skips the hooks module tests. Only the --fast flag sets it, so a CI job cannot inherit a partial audit. */
  fast: boolean;
  /** The stderr sink. A domain writes every non-result line here, never to process.stderr. */
  diagnostics: (text: string) => void;
  /** The environment child commands receive. */
  env: Record<string, string | undefined>;
}

/** The text of a thrown value: an Error's message, or the value itself as a string. */
export function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

export type DomainRun = (
  repoRoot: string,
  options: RunOptions,
) => Promise<AuditResult>;

/** The repo-relative file that identifies an agentics checkout root. */
export const MARKER = ".claude-plugin/plugin.json";

/** Set to "1" to run in short mode. */
export const ENV_SHORT = "AGX_CONFORMANCE_SHORT";

/** Walks up from dir (default: the working directory) to the marker. Returns the absolute root, or "" when none. */
export function repoRoot(dir: string = process.cwd()): string {
  let current = path.resolve(dir);
  for (;;) {
    if (existsSync(path.join(current, ...MARKER.split("/")))) return current;
    const parent = path.dirname(current);
    if (parent === current) return "";
    current = parent;
  }
}
