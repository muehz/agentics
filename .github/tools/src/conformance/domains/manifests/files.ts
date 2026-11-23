import {
  readdirSync,
  readFileSync,
  realpathSync,
  statSync,
  type Dirent,
} from "node:fs";
import path from "node:path";
import type { ValueResult } from "../../../release/version.ts";
import type { Failure } from "../../check.ts";
import { isLocal } from "../../local-path.ts";
import { RepoFileError, repoFileErrorOf } from "../../repo-file-error.ts";
import { walkCheckout, type Tracked } from "../../checkout-walk.ts";

/** The two marketplace catalogs, in parity-reference order: the first is the reference. */
export const MARKETPLACES = [
  ".claude-plugin/marketplace.json",
  ".agents/plugins/marketplace.json",
] as const;

/**
 * Directory names every walk in this domain skips, at any depth. `.claude` is skipped because its gitignored worktrees
 * can hold stale copies of every manifest.
 */
export const SKIPPED_DIRS: ReadonlySet<string> = new Set([
  ".git",
  "node_modules",
  ".claude",
]);

/** The repo-relative path a file error gives the checkout root. */
const ROOT_REL = ".";

/**
 * An audit error that arrives with the failures a step had found before it. The two rules that report a partial list
 * with their error (`marketplace-source-dir` and `plugin-path-references`) throw this.
 */
export class PartialAuditError extends Error {
  readonly failures: Failure[];

  constructor(message: string, failures: Failure[], options?: ErrorOptions) {
    super(message, options);
    this.name = "PartialAuditError";
    this.failures = failures;
  }
}

/**
 * `op(file)`, or undefined when nothing exists at `file`. Any other failure throws a `RepoFileError` naming `rel`,
 * the repo-relative path that `file` stands for.
 */
export function ifPresent<T>(
  file: string,
  rel: string,
  op: (file: string) => T,
): T | undefined {
  try {
    return op(file);
  } catch (error) {
    const fileError = repoFileErrorOf(error, rel);
    if (fileError.kind === "missing") return undefined;
    throw fileError;
  }
}

/**
 * The entries of the repo-relative directory `rel`, or undefined when it does not exist. Any other failure throws a
 * `RepoFileError`; a file at `rel` is `not-a-directory`.
 */
export function readDirIfPresent(
  root: string,
  rel: string,
): Dirent[] | undefined {
  const dir = path.join(root, ...rel.split("/"));
  try {
    return readdirSync(dir, { withFileTypes: true });
  } catch (error) {
    const fileError = repoFileErrorOf(error, rel);
    if (fileError.kind !== "missing") throw fileError;
    // Windows can report "does not exist" when the directory read is given an existing file.
    if (ifPresent(dir, rel, (file) => statSync(file)) === undefined)
      return undefined;
    throw new RepoFileError("not-a-directory", rel, { cause: error });
  }
}

/** A value `JSON.parse` returns. A number that overflows is `Infinity` or `-Infinity`. */
export type JsonValue =
  null | boolean | number | string | JsonValue[] | JsonObject;

/** A parsed JSON object. A repeated key holds its last value. Read members with `member`. */
export type JsonObject = { readonly [key: string]: JsonValue };

/** Whether `value` is a JSON object: not `null` and not an array. */
export function isJsonObject(
  value: JsonValue | undefined,
): value is JsonObject {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

/**
 * The member of `object` named exactly `key`, or undefined when the document wrote no such member. A key in another
 * case is another key, and nothing inherited from `Object.prototype` counts.
 */
export function member(object: JsonObject, key: string): JsonValue | undefined {
  return Object.hasOwn(object, key) ? object[key] : undefined;
}

export type LoadedObject = { found: boolean; ok: boolean; value: JsonObject };
export type LoadedAny = {
  found: boolean;
  ok: boolean;
  value: JsonValue | undefined;
};

/**
 * Reads the repo-relative JSON file `rel` below `root` with `JSON.parse`. A file that does not exist is
 * `found: false`; any other read failure throws a `RepoFileError`. Otherwise `ok` holds when the text parses and, for
 * `"object"`, its root is an object or `null`; a `null` root reads as an empty object. For `"any"` the root may be any
 * JSON value. Whatever the parser throws makes the file not `ok`, never a pass, and the caller reports it.
 */
export function loadJson(
  root: string,
  rel: string,
  shape: "object",
): LoadedObject;
export function loadJson(root: string, rel: string, shape: "any"): LoadedAny;
export function loadJson(
  root: string,
  rel: string,
  shape: "object" | "any",
): LoadedObject | LoadedAny {
  const empty = shape === "object" ? {} : undefined;
  const data = ifPresent(path.join(root, ...rel.split("/")), rel, (file) =>
    readFileSync(file),
  );
  if (data === undefined) return { found: false, ok: false, value: empty };
  let parsed: JsonValue;
  try {
    parsed = JSON.parse(data.toString("utf8")) as JsonValue;
  } catch {
    return { found: true, ok: false, value: empty };
  }
  if (shape === "any") return { found: true, ok: true, value: parsed };
  if (parsed === null) return { found: true, ok: true, value: {} };
  if (isJsonObject(parsed)) return { found: true, ok: true, value: parsed };
  return { found: true, ok: false, value: empty };
}

/**
 * Visits the entries below `root` depth first, in the default sort order of their names, never
 * following a symlink, and calls `fn` with the slash-separated path from the root. A directory named in `SKIPPED_DIRS`
 * is not entered and not reported; a file or a symlink with that name is reported. A root that is not a directory (a
 * symlink included) and a directory that cannot be read throw a `RepoFileError`; the root's path is `.`.
 *
 * Given `tracked`, a work folder's `temp` directory is scratch: one that holds no tracked file is neither reported nor
 * entered, and one that holds a tracked file is reported and entered, but below it only the files git tracks and the
 * directories that hold one are visited. `tracked` is called only on reaching such a directory, so a tree without one
 * never runs git; its failure propagates. Without `tracked` the walk enters every directory not in `SKIPPED_DIRS`.
 */
export function walkTree(
  root: string,
  fn: (rel: string, entry: Dirent) => void,
  tracked?: () => Tracked,
): void {
  walkCheckout(root, {
    mode: "manifests",
    skipDirs: SKIPPED_DIRS,
    tracked,
    visit: fn,
  });
}

/**
 * The repo-relative path a file error gives a local manifest reference: slash-separated (a backslash is a separator only
 * on Windows), normalized, and with no trailing slash.
 */
export function referenceRel(
  reference: string,
  platform: string = process.platform,
): string {
  const slashed =
    platform === "win32" ? reference.replaceAll("\\", "/") : reference;
  return path.posix.normalize(slashed).replace(/(.)\/$/, "$1");
}

export type Contained =
  | { kind: "ok"; target: string }
  | { kind: "escape"; message: string }
  | { kind: "missing" };

/**
 * Resolves `reference` below `root` and requires the result to stay inside the
 * resolved root. `reference` must itself be a local path; a symlink that leaves the checkout is an escape, whose
 * message quotes the reference as JSON. A path that does not exist is `missing`. Any other resolution failure (a link
 * loop, a permission error) throws a `RepoFileError` naming the normalized reference, or `.` when the root itself
 * fails; the caller names what it was auditing. `platform` selects the rules for the local-path test and defaults to
 * the host's.
 */
export function containedManifestPath(
  root: string,
  reference: string,
  platform: string = process.platform,
): Contained {
  const escape: Contained = {
    kind: "escape",
    message: `path escapes checkout: ${JSON.stringify(reference)}`,
  };
  const name =
    platform === "win32" ? reference.replaceAll("/", "\\") : reference;
  if (!isLocal(name, platform)) return escape;
  const canonicalRoot = ifPresent(root, ROOT_REL, (file) => realpathSync(file));
  if (canonicalRoot === undefined) return { kind: "missing" };
  const target = ifPresent(
    path.join(canonicalRoot, name),
    referenceRel(reference, platform),
    (file) => realpathSync(file),
  );
  if (target === undefined) return { kind: "missing" };
  const relative = path.relative(canonicalRoot, target);
  return isLocal(relative === "" ? "." : relative, platform)
    ? { kind: "ok", target }
    : escape;
}

/** The entries of a catalog's `plugins` array that are objects; none when `plugins` is not an array. */
export function pluginList(data: JsonObject): JsonObject[] {
  const plugins = member(data, "plugins");
  return Array.isArray(plugins) ? plugins.filter(isJsonObject) : [];
}

/**
 * Throws when a version manifest exists but cannot be read: that is an audit error. A file that does not exist is not;
 * the rules that need it report it.
 */
export function assertVersionsReadable(values: readonly ValueResult[]): void {
  for (const value of values)
    if (value.kind === "read" && !value.missing && value.error !== undefined)
      throw new Error(value.error);
}
