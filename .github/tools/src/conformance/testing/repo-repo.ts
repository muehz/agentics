import { mkdtempSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { MARKER, type AuditResult } from "../check.ts";
import { run } from "../domains/repo/index.ts";
import { writeFile } from "./files.ts";
import { fixtureGitEnv } from "./git-checkout.ts";
import { removeTree } from "./host.ts";

export { writeFile };

export const HOOKS_MOD =
  "module github.com/muehz/agentics/hooks/go\n\ngo 1.27.3\n";

/** What `auditRepo` can vary. */
export interface RepoAuditOptions {
  /** The inherited environment for the fixture's child commands. */
  env?: NodeJS.ProcessEnv;
}

/** Runs the repo domain on a checkout as the CLI does without `--short` or `--fast`, and returns its result. */
export function auditRepo(
  root: string,
  options: RepoAuditOptions = {},
): Promise<AuditResult> {
  return run(root, {
    short: false,
    fast: false,
    diagnostics: () => {},
    env: fixtureGitEnv(options.env),
  });
}

/** A checkout that passes every `repo` rule, for tests to change once. */
export function cloneRepoRepo(): string {
  const root = mkdtempSync(path.join(tmpdir(), "agentics-reporepo-"));
  writeFile(root, MARKER, "{}");
  writeFile(root, "README.md", "# Agentics\n\nA marketplace.\n");
  writeFile(root, "hooks/go/go.mod", HOOKS_MOD);
  return root;
}

/** Runs `body` on a fresh passing fixture and removes it afterwards, whatever happens. */
export async function withRepoRepo<T>(
  body: (root: string) => Promise<T> | T,
): Promise<T> {
  const root = cloneRepoRepo();
  try {
    return await body(root);
  } finally {
    removeTree(root);
  }
}
