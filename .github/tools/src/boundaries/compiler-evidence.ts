import { spawnSync } from "node:child_process";
import path from "node:path";
import {
  parseExplainFiles,
  parseUnresolvedImports,
  type Reason,
  type ResolvedFile,
} from "./explain.ts";

export interface TscResult {
  stdout: string;
  error: string | null;
}

export type TscRunner = (args: string[], cwd: string) => TscResult;

/** Runs the local compiler through Node. Type errors can accompany a usable file listing. */
export const defaultTsc: TscRunner = (args, cwd) => {
  const result = spawnSync(
    process.execPath,
    [path.join(cwd, "node_modules", "typescript", "bin", "tsc"), ...args],
    { cwd, encoding: "utf8", maxBuffer: 64 * 1024 * 1024 },
  );
  if (result.error) return { stdout: "", error: result.error.message };
  return { stdout: `${result.stdout}\n${result.stderr}`, error: null };
};

interface EvidenceRequest {
  root: string;
  args: string[];
  directory: string;
  includeUnresolved?: boolean;
  tsc?: TscRunner;
}

type CompilerEvidence =
  | { ok: true; files: ResolvedFile[]; unresolved: Reason[] }
  | { ok: false; detail: string };

/** Collects compiler evidence for a repository-relative directory using the audit's arguments. */
export function collectCompilerEvidence({
  root,
  args,
  directory,
  includeUnresolved = false,
  tsc = defaultTsc,
}: EvidenceRequest): CompilerEvidence {
  const { stdout, error } = tsc(args, root);
  if (error !== null) return { ok: false, detail: error };

  const files = parseExplainFiles(stdout, root);
  // Crash output can look like file entries. A listing must reach the requested directory.
  if (!files.some(({ file }) => file.startsWith(`${directory}/`)))
    return {
      ok: false,
      detail: stdout.trim().split("\n").slice(-5).join(" | "),
    };

  return {
    ok: true,
    files,
    unresolved: includeUnresolved ? parseUnresolvedImports(stdout, root) : [],
  };
}
