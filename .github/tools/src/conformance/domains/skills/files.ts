import { lstatSync, readdirSync, readFileSync } from "node:fs";
import path from "node:path";
import { errorMessage } from "../../check.ts";
import { isLocal } from "../../local-path.ts";
import {
  RepoFileError,
  repoFileErrorOf,
  type RepoFileErrorKind,
} from "../../repo-file-error.ts";
import {
  readTreeEntries,
  trackedPaths,
  walkCheckout,
  type FileInfo,
  type Tracked,
  type TreeEntry,
} from "../../checkout-walk.ts";
import { lazyTrackedLookup } from "../../work-folder-temp.ts";

export type { FileInfo, TreeEntry } from "../../checkout-walk.ts";

/**
 * Directory names the skills tree walk skips at any depth. `.git` holds repository metadata, `tests` keeps test
 * fixture trees, whose `skills` directories and `SKILL.md` files are not real skills, out of the skill audits, and
 * `.claude` and `node_modules` guard local runs against gitignored worktrees.
 */
export const SKIPPED_DIRS: ReadonlySet<string> = new Set([
  ".git",
  "node_modules",
  ".claude",
  "tests",
]);

/** The one file under `skills/` the text audits skip. */
export const INDEPENDENCE_EXEMPT_FILE = "skills/README.md";

/** What a visitor returns to keep a directory from being entered; any other result continues the walk. */
export type VisitResult = "skip" | void;

/** The names a directory read produced, in default sort order, together with any error that cut it short. */
export type DirectoryNames = { names: string[]; error?: unknown };

/**
 * The replaceable file-system operations. Every helper reads them at call time, so a test replaces members through
 * `withSeams`.
 */
export type Seams = {
  /** Inspects a path without following a link; throws the Node error when the path cannot be inspected. */
  lstat(path: string): FileInfo;
  /** A file's text: UTF-8 decoded, byte order mark and CR kept; throws the Node error. */
  readFile(path: string): string;
  /** The names in a skill directory in default sort order, kept even when an error comes with them. */
  readSkillDirectory(dir: string): DirectoryNames;
  /** The entries of a directory in default sort order of their names; throws the Node error. */
  readDir(dir: string): TreeEntry[];
};

/** Test seams for the helpers that take them directly, plus the platform whose path and glob rules apply. */
export type FileOptions = {
  lstat?: Seams["lstat"];
  readFile?: Seams["readFile"];
  readSkillDirectory?: Seams["readSkillDirectory"];
  platform?: string;
};

export const seams: Seams = {
  lstat: (target) => lstatSync(target),
  readFile: (target) => readFileSync(target, "utf8"),
  readSkillDirectory: (dir) => {
    try {
      return { names: readdirSync(dir).sort() };
    } catch (error) {
      return { names: [], error };
    }
  },
  readDir: readTreeEntries,
};

function isPromiseLike(value: unknown): value is Promise<unknown> {
  return (
    typeof value === "object" &&
    value !== null &&
    typeof (value as { then?: unknown }).then === "function"
  );
}

/**
 * Replaces the named seams, runs `body` and returns its result. A synchronous body gets the seams back in a `finally`.
 * When `body` returns a Promise the seams come back only after it settles, on success and on rejection, so code that
 * reads the file system after an `await` still sees the replacement. Overlapping asynchronous calls would restore in
 * the wrong order, so tests that use it run one at a time.
 */
export function withSeams<T>(overrides: Partial<Seams>, body: () => T): T {
  const saved: Partial<Seams> = {};
  for (const key of Object.keys(overrides) as Array<keyof Seams>)
    (saved as Record<string, unknown>)[key] = seams[key];
  Object.assign(seams, overrides);
  const restore = (): void => {
    Object.assign(seams, saved);
  };
  let result: T;
  try {
    result = body();
  } catch (error) {
    restore();
    throw error;
  }
  if (isPromiseLike(result)) {
    return Promise.resolve(result).finally(restore) as T;
  }
  restore();
  return result;
}

/**
 * The lazy tracked lookup the skills walk uses: git runs with `env` on first use, if at all. Its failure carries the
 * domain's `skills: ` prefix like the families' other audit errors; the prefix is added here rather than around the
 * walk so the walk's own errors keep their text.
 */
export function skillsTrackedLookup(
  root: string,
  env: Record<string, string | undefined>,
): () => Tracked {
  return lazyTrackedLookup(() => {
    try {
      return trackedPaths(root, env);
    } catch (error) {
      throw new Error(`skills: ${errorMessage(error)}`, { cause: error });
    }
  });
}

/**
 * The skills tree walk: visits the entries below `root`, the checkout, depth first in default sort order of their
 * names, with slash paths relative to the root. A symlink is neither delivered nor entered, whatever its name. The
 * top-level `evals` directory and a directory named in `SKIPPED_DIRS` at any depth are neither
 * delivered nor entered; a file with such a name is delivered. The root is never delivered, so a root that is a regular
 * file or a symlink delivers nothing. A root that cannot be inspected and a directory that cannot be read each throw a
 * `RepoFileError` naming it, the root as `.`. A visitor may return `"skip"` for a directory to keep it from being
 * entered.
 *
 * Given `tracked`, a work folder's `temp` directory is scratch: only the files git tracks below it and the directories
 * that hold one are delivered and entered, and one that holds no tracked file is neither delivered nor entered.
 * `tracked` is called only on reaching such a directory, so a tree without one never runs git; its failure propagates.
 * Without `tracked` the walk enters `temp` like any other directory.
 *
 * On Windows a junction is reported as a symlink and so skipped entirely.
 */
export function walkTree(
  root: string,
  visit: (rel: string, entry: TreeEntry) => VisitResult,
  tracked?: () => Tracked,
): void {
  walkCheckout(root, {
    mode: "skills-tree",
    skipDirs: SKIPPED_DIRS,
    tracked,
    visit,
    source: seams,
  });
}

/**
 * A depth-first, pre-order walk of `root`: each directory's entries in default sort order of their names, every entry
 * delivered (directories, files and symlinks) with its slash path relative to the root. A symlink is flagged and never
 * entered. A directory root is never delivered; a root that is a regular file or a symlink is delivered once with the
 * relative path `.` and not entered, so callers that must refuse a linked root check it first with `checkedRepoPath`.
 * A visitor returning `"skip"` for a directory keeps its children out; for anything else the result is ignored.
 *
 * `base` is the root's repo-relative path. A root that cannot be inspected throws a `RepoFileError` naming `base`,
 * and a directory that cannot be read, after the entries before it were delivered, one naming the directory below
 * `base`. Without `base` the error names the root as given, with forward slashes.
 *
 * On Windows a junction is flagged as a symlink.
 */
export function walkDir(
  root: string,
  visit: (rel: string, entry: TreeEntry) => VisitResult,
  base: string = root.replaceAll("\\", "/"),
): void {
  walkCheckout(root, { mode: "skills-dir", base, visit, source: seams });
}

/** The path components of `name` with empty and `.` components dropped and each `..` removing the one before it. */
function cleanComponents(name: string, separator: string): string[] {
  const parts: string[] = [];
  for (const part of name.split(separator)) {
    if (part === "" || part === ".") continue;
    if (part === "..") parts.pop();
    else parts.push(part);
  }
  return parts.length === 0 ? ["."] : parts;
}

/** A path below the checkout that `resolveRepoPath` has inspected: the joined path and its cleaned repo-relative form. */
type ResolvedPath = { full: string; rel: string };

function resolveRepoPath(
  repoRoot: string,
  rel: string,
  options: FileOptions,
): ResolvedPath {
  const platform = options.platform ?? process.platform;
  const lstat = options.lstat ?? seams.lstat;
  const windows = platform === "win32";
  const name = windows ? rel.replaceAll("/", "\\") : rel;
  if (!isLocal(name, platform)) throw new RepoFileError("outside-repo", rel);
  const parts = cleanComponents(name, windows ? "\\" : "/");
  let current = repoRoot;
  let shown = "";
  for (const part of parts) {
    current = path.join(current, part);
    shown = shown === "" ? part : `${shown}/${part}`;
    let info: FileInfo;
    try {
      info = lstat(current);
    } catch (error) {
      throw repoFileErrorOf(error, shown);
    }
    if (info.isSymbolicLink()) throw new RepoFileError("symlink", shown);
  }
  return { full: current, rel: shown };
}

/**
 * Resolves `rel` below `repoRoot` and refuses a link on the way. `rel` must be a local path (`platform` selects the
 * rules, the host's by default) or the call throws a `RepoFileError` of kind `outside-repo` naming `rel` as given,
 * without touching the file system. Each component from the root down to and including the leaf is inspected once, in
 * order, with `lstat`; the root itself is not, except for `.`. A link at any component throws a `RepoFileError` of kind
 * `symlink` naming the path up to that component. Any other inspection failure throws a `RepoFileError` naming the path up
 * to the component that failed, with the failure as its cause. The check and a following read are not atomic. Returns
 * the joined path.
 *
 * The components are joined with the host's `path` module, whatever `platform` says.
 */
export function checkedRepoPath(
  repoRoot: string,
  rel: string,
  options: FileOptions = {},
): string {
  return resolveRepoPath(repoRoot, rel, options).full;
}

/**
 * The text of a repository file read through `checkedRepoPath`, UTF-8 decoded with the byte order mark and CR kept;
 * an invalid UTF-8 byte decodes to U+FFFD. An inspection failure is thrown as `checkedRepoPath` throws it and the file
 * is not read. A read failure throws a `RepoFileError` naming the cleaned `rel`, or the `RepoFileError` the read
 * threw.
 */
export function readRepoFile(
  repoRoot: string,
  rel: string,
  options: FileOptions = {},
): string {
  const readFile = options.readFile ?? seams.readFile;
  const resolved = resolveRepoPath(repoRoot, rel, options);
  try {
    return readFile(resolved.full);
  } catch (error) {
    throw repoFileErrorOf(error, resolved.rel);
  }
}

const BAD_PATTERN = "syntax error in pattern";

const BACKSLASH = 0x5c;
const STAR = 0x2a;
const LEFT_BRACKET = 0x5b;
const RIGHT_BRACKET = 0x5d;
const QUESTION = 0x3f;
const CARET = 0x5e;
const DASH = 0x2d;

/** The length of the UTF-8 sequence at `at`, or undefined when it is not a valid one. */
function runeWidth(bytes: Buffer, at: number): number | undefined {
  const lead = bytes[at] ?? 0;
  let width: number;
  if (lead < 0x80) return 1;
  else if (lead >= 0xc2 && lead <= 0xdf) width = 2;
  else if (lead >= 0xe0 && lead <= 0xef) width = 3;
  else if (lead >= 0xf0 && lead <= 0xf4) width = 4;
  else return undefined;
  if (at + width > bytes.length) return undefined;
  for (let i = 1; i < width; i += 1) {
    const next = bytes[at + i] ?? 0;
    if (next < 0x80 || next > 0xbf) return undefined;
  }
  return width;
}

/** The index after one possibly escaped character of a class, or undefined for a bad pattern. */
function classCharacter(
  chunk: Buffer,
  from: number,
  windows: boolean,
): number | undefined {
  let at = from;
  if (at >= chunk.length || chunk[at] === DASH || chunk[at] === RIGHT_BRACKET)
    return undefined;
  if (chunk[at] === BACKSLASH && !windows) {
    at += 1;
    if (at >= chunk.length) return undefined;
  }
  const width = runeWidth(chunk, at);
  if (width === undefined) return undefined;
  at += width;
  return at >= chunk.length ? undefined : at;
}

/** Whether a pattern chunk is well formed: every class closes and holds valid ranges, and no escape ends the chunk. */
function chunkIsWellFormed(chunk: Buffer, windows: boolean): boolean {
  let at = 0;
  while (at < chunk.length) {
    const byte = chunk[at];
    if (byte === LEFT_BRACKET) {
      at += 1;
      if (chunk[at] === CARET) at += 1;
      let ranges = 0;
      for (;;) {
        if (at < chunk.length && chunk[at] === RIGHT_BRACKET && ranges > 0) {
          at += 1;
          break;
        }
        const afterLow = classCharacter(chunk, at, windows);
        if (afterLow === undefined) return false;
        at = afterLow;
        if (chunk[at] === DASH) {
          const afterHigh = classCharacter(chunk, at + 1, windows);
          if (afterHigh === undefined) return false;
          at = afterHigh;
        }
        ranges += 1;
      }
    } else if (byte === QUESTION) {
      at += 1;
    } else if (byte === BACKSLASH && !windows) {
      at += 1;
      if (at >= chunk.length) return false;
      at += 1;
    } else {
      at += 1;
    }
  }
  return true;
}

/**
 * Whether `pattern` is a well-formed glob when matched against an empty name, which is the check `skillDirectories`
 * makes on the pattern it would glob. Only the first chunk is inspected, the text after any leading stars up to the
 * first `*` outside a class, so a malformed class after that `*` is not seen. On `win32` a backslash is a separator,
 * not an escape. The pattern is read as its UTF-8 bytes.
 */
export function isValidGlobPattern(
  pattern: string,
  platform: string = process.platform,
): boolean {
  const windows = platform === "win32";
  const bytes = Buffer.from(pattern);
  let start = 0;
  while (start < bytes.length && bytes[start] === STAR) start += 1;
  let inClass = false;
  let end = bytes.length;
  for (let i = start; i < bytes.length; i += 1) {
    const byte = bytes[i];
    if (byte === BACKSLASH) {
      if (!windows && i + 1 < bytes.length) i += 1;
    } else if (byte === LEFT_BRACKET) {
      inClass = true;
    } else if (byte === RIGHT_BRACKET) {
      inClass = false;
    } else if (byte === STAR && !inClass) {
      end = i;
      break;
    }
  }
  return chunkIsWellFormed(bytes.subarray(start, end), windows);
}

/** The names in the skill directory `dir`, read by the seam or by the replacement a test passed. */
function enumerate(dir: string, options: FileOptions): DirectoryNames {
  return (options.readSkillDirectory ?? seams.readSkillDirectory)(dir);
}

/**
 * The immediate real directories of `skills/` as `skills/<name>` in default sort order, never following a link. Throws
 * `syntax error in pattern` when the repository root makes the pattern `<root>/skills/*\/SKILL.md` malformed (a root
 * ending in `[`). Every other failure is swallowed, so the listing is best effort: a linked, missing or non-directory
 * `skills`, an unreadable directory (the names read before the error are kept) and a path with a NUL byte all give no
 * entry for what they cover.
 */
export function skillDirectories(
  repoRoot: string,
  options: FileOptions = {},
): string[] {
  const platform = options.platform ?? process.platform;
  const lstat = options.lstat ?? seams.lstat;
  if (
    !isValidGlobPattern(
      path.join(repoRoot, "skills", "*", "SKILL.md"),
      platform,
    )
  )
    throw new Error(BAD_PATTERN);
  let root: string;
  try {
    root = checkedRepoPath(repoRoot, "skills", options);
    if (!lstat(root).isDirectory()) return [];
  } catch {
    return [];
  }
  const dirs: string[] = [];
  for (const entry of enumerate(root, options).names) {
    try {
      const child = checkedRepoPath(repoRoot, `skills/${entry}`, options);
      if (lstat(child).isDirectory()) dirs.push(`skills/${entry}`);
    } catch {
      // A link, a vanished entry or a non-local name is not a skill directory.
    }
  }
  return dirs;
}

/** The outcome of `globSkillLeaf`: the joined path of the leaf, and whether it was selected. */
export type SelectedLeaf = { path: string; selected: boolean };

/** Whether `error` is a `RepoFileError` of kind `kind`. */
export function isFileErrorOfKind(
  error: unknown,
  kind: RepoFileErrorKind,
): error is RepoFileError {
  return error instanceof RepoFileError && error.kind === kind;
}

/**
 * The audit error for a failure while `action` (such as `reading`) the repo-relative `rel`, with `error` as its cause:
 * `skills: <action> <rel>: <message>`. The message of a `RepoFileError` names its own path, which may be `rel` again,
 * an ancestor holding a link, or a directory below `rel` that a walk could not read.
 */
export function auditFileError(
  action: string,
  rel: string,
  error: unknown,
): Error {
  return new Error(`skills: ${action} ${rel}: ${errorMessage(error)}`, {
    cause: error,
  });
}

/**
 * Selects the literal child `leaf` of `parent` by enumerating the parent. An unusable parent, an unreadable directory
 * (names read before the error still count), a missing name and a link at the leaf are quietly not selected. Any other
 * failure to inspect a listed leaf throws the `auditFileError` for `checking <parent>/<leaf>`.
 */
export function globSkillLeaf(
  repoRoot: string,
  parent: string,
  leaf: string,
  options: FileOptions = {},
): SelectedLeaf {
  const none: SelectedLeaf = { path: "", selected: false };
  let dir: string;
  try {
    dir = checkedRepoPath(repoRoot, parent, options);
  } catch {
    return none;
  }
  for (const entry of enumerate(dir, options).names) {
    if (entry !== leaf) continue;
    const rel = `${parent}/${leaf}`;
    try {
      return { path: checkedRepoPath(repoRoot, rel, options), selected: true };
    } catch (error) {
      if (isFileErrorOfKind(error, "symlink")) return none;
      throw auditFileError("checking", rel, error);
    }
  }
  return none;
}

/** `skills/*\/SKILL.md` as repo-relative slash paths in default sort order. */
export function assetFiles(
  repoRoot: string,
  options: FileOptions = {},
): string[] {
  const out: string[] = [];
  for (const parent of skillDirectories(repoRoot, options))
    if (globSkillLeaf(repoRoot, parent, "SKILL.md", options).selected)
      out.push(`${parent}/SKILL.md`);
  return out.sort();
}

/**
 * Calls `visit` with the repo-relative slash path and the text of each eligible file under `skills/`, in walk order.
 * Directories, symlinks, `skills/README.md` and files holding a NUL byte are skipped; an empty file counts as scanned.
 * The text is decoded as UTF-8 with the byte order mark and CR kept. Returns the count visited, zero for a linked
 * root or an empty selection. A missing `skills` throws the `RepoFileError` of its inspection. If `skills` is a
 * regular file it is visited once as `skills`. A file or directory that cannot be read throws a `RepoFileError` naming it.
 */
export function visitSkillText(
  repoRoot: string,
  visit: (rel: string, text: string) => void,
  options: FileOptions = {},
): number {
  const readFile = options.readFile ?? seams.readFile;
  let base: string;
  try {
    base = checkedRepoPath(repoRoot, "skills", options);
  } catch (error) {
    if (isFileErrorOfKind(error, "symlink")) return 0;
    throw error;
  }
  let scanned = 0;
  walkDir(
    base,
    (inner, entry) => {
      if (entry.isDirectory || entry.isSymlink) return;
      const rel = inner === "." ? "skills" : `skills/${inner}`;
      if (rel === INDEPENDENCE_EXEMPT_FILE) return;
      let text: string;
      try {
        text = readFile(inner === "." ? base : path.join(base, inner));
      } catch (error) {
        throw repoFileErrorOf(error, rel);
      }
      if (text.includes("\0")) return;
      scanned += 1;
      visit(rel, text);
    },
    "skills",
  );
  return scanned;
}

/** The caller supplies the audit name, including when another audit already observed the empty selection. */
export function noSkillTextError(audit: string): Error {
  return new Error(`skills: no files under skills/ to audit for ${audit}`);
}

/** Visits eligible skill text in walk order and reports an empty selection under the given audit name. */
export function walkSkillText(
  repoRoot: string,
  audit: string,
  visit: (rel: string, text: string) => void,
  options: FileOptions = {},
): void {
  if (visitSkillText(repoRoot, visit, options) === 0)
    throw noSkillTextError(audit);
}
