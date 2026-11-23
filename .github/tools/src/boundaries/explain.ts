export interface Reason {
  specifier: string;
  importer: string;
}

export interface ResolvedFile {
  file: string;
  reasons: Reason[];
}

const IMPORT_REASON =
  /^\s+Imported via "(.*)" from file '([^']*)'(?: with packageId '.*')?$/;
const DIAGNOSTIC = /\berror TS\d+:/;

/** Repo-relative, forward-slash form of a path tsc printed; paths outside the root stay as printed. */
export function toRepoPath(raw: string, root: string): string {
  const slashed = raw.replaceAll("\\", "/");
  const prefix = root.replaceAll("\\", "/").replace(/\/$/, "") + "/";
  return slashed.startsWith(prefix) ? slashed.slice(prefix.length) : slashed;
}

/** Parses `tsc --explainFiles` output. The tsc exit code is not an input: type errors do not stop the list. */
export function parseExplainFiles(
  output: string,
  root: string,
): ResolvedFile[] {
  const files: ResolvedFile[] = [];
  let current: ResolvedFile | null = null;
  for (const line of output.split(/\r?\n/)) {
    if (line.trim() === "") continue;
    if (/^\s/.test(line)) {
      const match = IMPORT_REASON.exec(line);
      if (current && match) {
        current.reasons.push({
          specifier: match[1]!,
          importer: toRepoPath(match[2]!, root),
        });
      }
      continue;
    }
    if (DIAGNOSTIC.test(line)) {
      current = null;
      continue;
    }
    current = { file: toRepoPath(line, root), reasons: [] };
    files.push(current);
  }
  return files;
}

const MISSING_MODULE =
  /^(.+?)\(\d+,\d+\): error TS2307: Cannot find module '([^']+)'/;

/**
 * Parses the module-not-found diagnostics of a `--checkJs` run. `--explainFiles` lists only imports that resolved, so an
 * import of a package that is not installed appears here and nowhere else. Relative and absolute specifiers are skipped:
 * a missing file is not a dependency.
 */
export function parseUnresolvedImports(output: string, root: string): Reason[] {
  const out: Reason[] = [];
  for (const line of output.split(/\r?\n/)) {
    const match = MISSING_MODULE.exec(line);
    if (!match) continue;
    const specifier = match[2]!;
    if (specifier.startsWith(".") || specifier.startsWith("/")) continue;
    out.push({ specifier, importer: toRepoPath(match[1]!, root) });
  }
  return out;
}
