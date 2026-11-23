import { readFileSync } from "node:fs";
import { join } from "node:path";
import { VERSIONED_PLUGIN_MANIFESTS } from "../plugin-manifests.ts";
import { FileError, fileErrorOf, writeFile } from "./files.ts";
import { describeExit, runGit, succeeded } from "./github.ts";

export type Version = {
  readonly major: bigint;
  readonly minor: bigint;
  readonly patch: bigint;
};

export const MANIFESTS = VERSIONED_PLUGIN_MANIFESTS;

export type Snapshot = { version: Version; raw: string };

// A full, lowercase commit SHA. A short one would resolve locally and end
// a range at a target the publish job cannot compare.
export const FULL_SHA = /^[0-9a-f]{40}$/;

// A released version tag; a prerelease or build-metadata tag does not match.
export const STABLE_TAG = /^v([0-9]+)\.([0-9]+)\.([0-9]+)$/;

const CANONICAL = /^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$/;
const UINT64_MAX = (1n << 64n) - 1n;
const INT64_MAX = (1n << 63n) - 1n;
const INVALID_VERSION = "invalid agentics version";

// A canonical version: three dot-separated decimal numbers with no leading zero, each at most 2^64 - 1.
export function parseVersion(input: string): Version {
  const parts = CANONICAL.exec(input);
  if (parts === null) throw new Error(INVALID_VERSION);
  const numbers = parts.slice(1, 4).map((part) => BigInt(part));
  const [major, minor, patch] = numbers;
  if (major === undefined || minor === undefined || patch === undefined) {
    throw new Error(INVALID_VERSION);
  }
  if ([major, minor, patch].some((part) => part > UINT64_MAX)) {
    throw new Error(INVALID_VERSION);
  }
  return { major, minor, patch };
}

export function formatVersion(v: Version): string {
  return `${v.major}.${v.minor}.${v.patch}`;
}

// True when a sorts strictly before b, comparing major, minor, then patch.
export function lessThan(a: Version, b: Version): boolean {
  if (a.major !== b.major) return a.major < b.major;
  if (a.minor !== b.minor) return a.minor < b.minor;
  return a.patch < b.patch;
}

// The three numbers of a stable tag, or undefined for any other name,
// including a number that does not fit a signed 64-bit integer.
export function tagVersion(
  tag: string,
): readonly [bigint, bigint, bigint] | undefined {
  const match = STABLE_TAG.exec(tag);
  if (match === null) return undefined;
  const numbers = match.slice(1, 4).map((part) => BigInt(part));
  const [major, minor, patch] = numbers;
  if (major === undefined || minor === undefined || patch === undefined) {
    return undefined;
  }
  if ([major, minor, patch].some((part) => part > INT64_MAX)) return undefined;
  return [major, minor, patch];
}

export type ErrorKind = "none" | "read" | "json" | "shape" | "version";

// The independently read and parsed value at one manifest location.
export type ValueResult = {
  path: string;
  raw: string;
  kind: ErrorKind;
  error: string | undefined;
  // The comparable value of the version field: unset for a missing field, for null, and for every read or syntax
  // failure.
  value: ValueToken;
  // True when kind is "read" because the file does not exist.
  missing: boolean;
};

type ManifestRead = {
  raw: string;
  kind: "none" | "json" | "shape";
  message: string;
  value: ValueToken;
};

/**
 * The comparable value of a version field. A missing field and `null` are both unset. Two set tokens hold the same
 * value exactly when their `json` is equal.
 */
export type ValueToken = { readonly set: boolean; readonly json: string };

/** The token of a field that is missing or `null`. */
export const UNSET: ValueToken = { set: false, json: "" };

type JsonObject = Readonly<Record<string, unknown>>;

const isObject = (value: unknown): value is JsonObject =>
  typeof value === "object" && value !== null && !Array.isArray(value);

// Only a member the document itself wrote counts, never one inherited from Object.prototype.
const own = (object: JsonObject, key: string): unknown =>
  Object.hasOwn(object, key) ? object[key] : undefined;

// String.prototype.toWellFormed is newer than this project's library target.
const LONE_SURROGATE =
  /[\ud800-\udbff](?![\udc00-\udfff])|(?<![\ud800-\udbff])[\udc00-\udfff]/g;
const wellFormed = (text: string): string =>
  text.replace(LONE_SURROGATE, "\ufffd");

// The canonical text of a parsed value, so that equal text means equal values: object keys are sorted, `-0` is `0`,
// and a lone surrogate is U+FFFD in strings and keys. A number that overflowed is written `Infinity` or `-Infinity`,
// which is not JSON, so it never equals `null` or a string.
function canonicalJson(value: unknown): string {
  if (typeof value === "string") return JSON.stringify(wellFormed(value));
  if (typeof value === "number")
    return Number.isFinite(value) ? JSON.stringify(value) : String(value);
  if (Array.isArray(value)) return `[${value.map(canonicalJson).join(",")}]`;
  if (isObject(value)) {
    const members = new Map<string, string>();
    for (const [key, member] of Object.entries(value))
      members.set(wellFormed(key), canonicalJson(member));
    const sorted = [...members].sort(([a], [b]) => (a < b ? -1 : 1));
    return `{${sorted.map(([key, member]) => `${JSON.stringify(key)}:${member}`).join(",")}}`;
  }
  return JSON.stringify(value);
}

function valueToken(value: unknown): ValueToken {
  if (value === undefined || value === null) return UNSET;
  return { set: true, json: canonicalJson(value) };
}

// A version that is not a string keeps its token, so a caller can still compare what two manifests hold.
function versionOfField(path: string, field: unknown): ManifestRead {
  if (field === undefined) {
    return {
      raw: "",
      kind: "shape",
      message: `${path}: has no "version" field`,
      value: UNSET,
    };
  }
  if (typeof field !== "string") {
    return {
      raw: "",
      kind: "shape",
      message: `${path}: "version" is not a string`,
      value: valueToken(field),
    };
  }
  const raw = wellFormed(field);
  return { raw, kind: "none", message: "", value: valueToken(raw) };
}

// The Agentics version in a marketplace is the `version` of the one `plugins` entry named `agentics`. Keys match in exact
// case. A `null` plugins list is an empty one, a `null` entry is skipped, and a `null` name is no name; any other value
// of the wrong type fails the whole file.
function readMarketplace(path: string, document: unknown): ManifestRead {
  const fail = (
    kind: "json" | "shape",
    message: string,
    raw = "",
    value: ValueToken = UNSET,
  ): ManifestRead => ({ raw, kind, message: `${path}: ${message}`, value });
  if (document === null) return fail("shape", "missing plugins array");
  if (!isObject(document)) return fail("json", "is not a JSON object");
  const plugins = own(document, "plugins");
  if (plugins === undefined) return fail("shape", "missing plugins array");
  const invalid = (detail: string): ManifestRead =>
    fail("shape", `invalid plugins array: ${detail}`);
  if (plugins !== null && !Array.isArray(plugins)) {
    return invalid('"plugins" is not an array');
  }
  const matches: unknown[] = [];
  for (const [index, entry] of ((plugins ?? []) as unknown[]).entries()) {
    if (entry === null) continue;
    if (!isObject(entry)) return invalid(`entry ${index} is not an object`);
    const name = own(entry, "name") ?? "";
    if (typeof name !== "string") {
      return invalid(`entry ${index} "name" is not a string`);
    }
    if (name === "agentics") matches.push(own(entry, "version"));
  }
  if (matches.length !== 1) {
    // The last entry whose version is a non-empty string is kept as the raw value, so a duplicate still reports a
    // version. The token is the last entry's, whatever it holds.
    let legacy = "";
    let legacyValue: ValueToken = UNSET;
    for (const version of matches) {
      legacyValue = valueToken(version);
      if (typeof version === "string" && version !== "")
        legacy = wellFormed(version);
    }
    return fail(
      "shape",
      `expected exactly one Agentics marketplace entry, found ${matches.length}`,
      legacy,
      legacyValue,
    );
  }
  return versionOfField(path, matches[0]);
}

/**
 * Reads the Agentics version out of one manifest's bytes and never throws. A plugin manifest carries it in its own
 * `version` member; a marketplace carries it in its `agentics` plugin entry. Keys match in exact case, and a repeated key
 * takes its last value. Any failure is a result whose `kind` is not `"none"` and whose message names `path`: text that
 * is not JSON and a document that is not an object are `"json"`, a missing or non-string version is `"shape"`.
 */
export function readManifestVersion(path: string, data: Buffer): ManifestRead {
  const unreadable = (message: string): ManifestRead => ({
    raw: "",
    kind: "json",
    message: `${path}: ${message}`,
    value: UNSET,
  });
  let parsed = false;
  // One guard around decoding, parsing and reading: whatever throws, the manifest is reported as unreadable. The
  // engine's own syntax message is left out, because its wording varies by version and it quotes the input.
  try {
    const document: unknown = JSON.parse(data.toString("utf8"));
    parsed = true;
    if (path.endsWith("marketplace.json"))
      return readMarketplace(path, document);
    if (document === null) return versionOfField(path, undefined);
    if (!isObject(document)) return unreadable("is not a JSON object");
    return versionOfField(path, own(document, "version"));
  } catch {
    return unreadable(parsed ? "could not be read" : "is not valid JSON");
  }
}

// A failed outcome's error already names the manifest.
type Outcome = { data: Buffer } | { error: string; missing: boolean };

// The bytes of the manifest at the repo-relative `path` below `root`. A failure throws a `FileError` naming `path`,
// so the message never shows the absolute checkout path.
function readManifest(root: string, path: string): Buffer {
  try {
    return readFileSync(join(root, path));
  } catch (error) {
    throw fileErrorOf(error, path);
  }
}

// One result per manifest: a read failure, a manifest that does not yield a version string, or a version that is
// not canonical each make a result with an error.
function evaluate(outcomes: readonly Outcome[]): ValueResult[] {
  return MANIFESTS.map(({ path }, index): ValueResult => {
    const outcome = outcomes[index];
    if (outcome === undefined || "error" in outcome) {
      return {
        path,
        raw: "",
        kind: "read",
        error: outcome === undefined ? `${path}: not read` : outcome.error,
        value: UNSET,
        missing: outcome?.missing ?? false,
      };
    }
    const read = readManifestVersion(path, outcome.data);
    const { value } = read;
    if (read.kind !== "none") {
      return {
        path,
        raw: read.raw,
        kind: read.kind,
        error: read.message,
        value,
        missing: false,
      };
    }
    try {
      parseVersion(read.raw);
    } catch (error) {
      return {
        path,
        raw: read.raw,
        kind: "version",
        error: `${path}: version ${JSON.stringify(read.raw)}: ${(error as Error).message}`,
        value,
        missing: false,
      };
    }
    return {
      path,
      raw: read.raw,
      kind: "none",
      error: undefined,
      value,
      missing: false,
    };
  });
}

// The first result's error, a disagreement between two manifests, or the one version they all hold.
function snapshotOf(results: readonly ValueResult[]): Snapshot {
  const failed = results.find((result) => result.error !== undefined);
  if (failed?.error !== undefined) throw new Error(failed.error);
  const [common, ...others] = results;
  if (common === undefined) throw new Error(INVALID_VERSION);
  for (const other of others) {
    if (other.raw !== common.raw) {
      throw new Error(
        `version mismatch: ${common.path}=${JSON.stringify(common.raw)} conflicts with ${other.path}=${JSON.stringify(other.raw)}`,
      );
    }
  }
  try {
    return { version: parseVersion(common.raw), raw: common.raw };
  } catch (error) {
    throw new Error(
      `${common.path}: version ${JSON.stringify(common.raw)}: ${(error as Error).message}`,
    );
  }
}

// One result per manifest, in manifest order, whatever its neighbors hold.
export function readValues(root: string): ValueResult[] {
  return evaluate(
    MANIFESTS.map(({ path }): Outcome => {
      try {
        return { data: readManifest(root, path) };
      } catch (error) {
        return {
          error: (error as Error).message,
          missing: error instanceof FileError && error.kind === "missing",
        };
      }
    }),
  );
}

// Reads the five manifests beneath root and requires one canonical version.
export function readSnapshot(root: string): Snapshot {
  return snapshotOf(readValues(root));
}

// The same read from an immutable commit, through `git show`.
export async function readSnapshotAt(
  root: string,
  commit: string,
): Promise<Snapshot> {
  if (!FULL_SHA.test(commit)) {
    throw new Error(`invalid commit SHA: ${JSON.stringify(commit)}`);
  }
  const outcomes: Outcome[] = [];
  for (const { path } of MANIFESTS) {
    const result = await runGit(root, ["show", `${commit}:${path}`]);
    outcomes.push(
      succeeded(result)
        ? { data: result.stdout }
        : {
            error: `${path}: git show: ${describeExit(result)}`,
            missing: false,
          },
    );
  }
  return snapshotOf(evaluate(outcomes));
}

// A "version" member with any JSON white space around its colon; group 1 is the
// string between the quotes. The class is spelled out rather than \s, which
// would also match characters JSON does not count as white space. The text is
// matched as bytes (latin1) so that offsets are byte offsets and the bytes
// around the match are never re-encoded.
const VERSION_FIELD = /"version"[\t\n\f\r ]*:[\t\n\f\r ]*"([^"\\]*)"/dg;

// Replaces the one version string that readManifestVersion reads as the Agentics
// version. A text match alone cannot tell a marketplace's plugin entry from its
// own metadata.version, so each candidate is replaced in turn and kept only
// when the manifest reader then sees the new version. Nothing else in the file
// changes: not indentation, key order, line endings or trailing bytes.
export function rewriteVersion(
  path: string,
  data: Buffer,
  from: string,
  to: string,
): Buffer {
  for (const match of data.toString("latin1").matchAll(VERSION_FIELD)) {
    const span = match.indices?.[1];
    if (span === undefined || match[1] !== from) continue;
    const [start, end] = span;
    const candidate = Buffer.concat([
      data.subarray(0, start),
      Buffer.from(to, "latin1"),
      data.subarray(end),
    ]);
    const read = readManifestVersion(path, candidate);
    if (read.kind === "none" && read.raw === to) return candidate;
  }
  throw new Error(
    `${path}: no "version": ${JSON.stringify(from)} field reads as the Agentics version`,
  );
}

// Every rewrite is computed before any file is written, so a refusal on the
// last manifest leaves the first four as they were.
function writeAll(root: string, current: Snapshot, next: Version): void {
  const rewritten = MANIFESTS.map(({ path }) => {
    try {
      const data = readManifest(root, path);
      return {
        path,
        data: rewriteVersion(path, data, current.raw, formatVersion(next)),
      };
    } catch (error) {
      throw new Error(`refusing to set version: ${(error as Error).message}`);
    }
  });
  for (const { path, data } of rewritten) {
    try {
      writeFile(join(root, path), data);
    } catch (error) {
      throw new Error(
        `${(error as Error).message}; ${path} and the manifests before it may already be rewritten to ${formatVersion(next)}, so restore them from git`,
      );
    }
  }
}

// Writes target into the five manifests beneath root and returns the version
// before and after. It refuses, writing nothing, when target is not a
// canonical version strictly greater than the current one, when the five
// disagree, or when any one of them cannot be rewritten.
export function setVersion(
  root: string,
  target: string,
): { from: Version; to: Version } {
  let next: Version;
  try {
    next = parseVersion(target);
  } catch (error) {
    throw new Error(
      `refusing to set version: ${JSON.stringify(target)}: ${(error as Error).message}`,
    );
  }
  let current: Snapshot;
  try {
    current = readSnapshot(root);
  } catch (error) {
    throw new Error(`refusing to set version: ${(error as Error).message}`);
  }
  if (!lessThan(current.version, next)) {
    throw new Error(
      `refusing to set version: ${formatVersion(next)} is not greater than ${formatVersion(current.version)}`,
    );
  }
  writeAll(root, current, next);
  return { from: current.version, to: next };
}

// Tree entry modes `git diff-tree --raw` reports.
const MODE_REGULAR = "100644";
const MODE_SYMLINK = "120000";
const MODE_GITLINK = "160000";

// The bytes committed at commit:path. cat-file applies no textconv or other
// display configuration.
async function blobAt(
  root: string,
  commit: string,
  path: string,
): Promise<Buffer> {
  const result = await runGit(root, ["cat-file", "blob", `${commit}:${path}`]);
  if (!succeeded(result)) {
    throw new Error(
      `git cat-file blob ${commit}:${path}: ${describeExit(result)}`,
    );
  }
  return result.stdout;
}

// Requires every entry that differs between parent and commit to be one of the
// five manifests, modified in place as a regular non-executable file. It reads
// the plumbing diff-tree with NUL-delimited records, so no pager, porcelain
// configuration, path quoting or white space in a path name can hide an entry.
async function checkChangedEntries(
  root: string,
  parent: string,
  commit: string,
): Promise<void> {
  const result = await runGit(root, [
    "diff-tree",
    "-r",
    "-z",
    "--no-renames",
    "--raw",
    parent,
    commit,
  ]);
  if (!succeeded(result)) {
    throw new Error(`git diff-tree: ${describeExit(result)}`);
  }
  const out = result.stdout.toString("utf8");
  const manifests = new Set(MANIFESTS.map((manifest) => manifest.path));

  // Each record is ":<srcmode> <dstmode> <srcsha> <dstsha> <status>" and then
  // the path, each terminated by NUL.
  const parts = (out.endsWith("\0") ? out.slice(0, -1) : out).split("\0");
  if (parts.length === 1 && parts[0] === "") return;
  if (parts.length % 2 !== 0) {
    throw new Error(`git diff-tree: unexpected output ${JSON.stringify(out)}`);
  }
  for (let i = 0; i < parts.length; i += 2) {
    const record = parts[i] ?? "";
    const path = parts[i + 1] ?? "";
    const meta = (record.startsWith(":") ? record.slice(1) : record)
      .split(/\s+/)
      .filter((word) => word !== "");
    if (meta.length !== 5) {
      throw new Error(
        `git diff-tree: unexpected record ${JSON.stringify(record)}`,
      );
    }
    const [srcMode, dstMode, , , status] = meta;
    if (srcMode === MODE_GITLINK || dstMode === MODE_GITLINK) {
      throw new Error(
        `release commit has a gitlink at ${JSON.stringify(path)}`,
      );
    }
    if (srcMode === MODE_SYMLINK || dstMode === MODE_SYMLINK) {
      throw new Error(
        `release commit has a symlink at ${JSON.stringify(path)}`,
      );
    }
    if (status === "A") {
      throw new Error(`release commit adds ${JSON.stringify(path)}`);
    }
    if (status === "D") {
      throw new Error(`release commit deletes ${JSON.stringify(path)}`);
    }
    if (srcMode !== MODE_REGULAR || dstMode !== MODE_REGULAR) {
      throw new Error(
        `release commit changes ${JSON.stringify(path)} from mode ${srcMode} to ${dstMode}, want ${MODE_REGULAR} on both sides`,
      );
    }
    if (!manifests.has(path)) {
      throw new Error(
        `release commit changes ${JSON.stringify(path)}, which is not one of the five manifests`,
      );
    }
  }
}

// Checks that commit raises the Agentics version above parent's and differs from
// parent in exactly the five manifest paths, each a regular non-executable file
// on both sides that changes only in the Agentics version value, and returns the
// version at commit. The release job pushes the commit with a token that
// bypasses review, so this check is what makes "only the five version fields
// changed" true rather than conventional.
export async function verifyReleaseCommit(
  root: string,
  parent: string,
  commit: string,
): Promise<Version> {
  let before: Snapshot;
  try {
    before = await readSnapshotAt(root, parent);
  } catch (error) {
    throw new Error(`parent: ${(error as Error).message}`);
  }
  let after: Snapshot;
  try {
    after = await readSnapshotAt(root, commit);
  } catch (error) {
    throw new Error(`release commit: ${(error as Error).message}`);
  }
  if (!lessThan(before.version, after.version)) {
    throw new Error(
      `release commit version ${formatVersion(after.version)} is not greater than parent version ${formatVersion(before.version)}`,
    );
  }
  await checkChangedEntries(root, parent, commit);

  for (const { path } of MANIFESTS) {
    const parentBytes = await blobAt(root, parent, path);
    const commitBytes = await blobAt(root, commit, path);
    const expected = rewriteVersion(path, parentBytes, before.raw, after.raw);
    if (!expected.equals(commitBytes)) {
      throw new Error(`${path} changes more than the version field`);
    }
  }
  return after.version;
}
