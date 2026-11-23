import { statSync } from "node:fs";
import { errorMessage, type Failure } from "../../check.ts";
import { frontmatterBlock, oneLine } from "../../text.ts";
import {
  isAlias,
  isMap as isYamlMap,
  isNode,
  isScalar as isYamlScalar,
  isSeq as isYamlSeq,
  parseDocument,
} from "yaml";
import { repoFileErrorOf } from "../../repo-file-error.ts";
import {
  assetFiles,
  auditFileError,
  checkedRepoPath,
  isFileErrorOfKind,
  readRepoFile,
  seams,
  skillsTrackedLookup,
  walkTree,
} from "./files.ts";
import type { Family } from "./index.ts";
import { inWorkFolderTemp } from "../../work-folder-temp.ts";
import type { Tracked } from "../repo/walk.ts";

export { frontmatterBlock };

/** The block text of `frontmatterBlock`, empty when there is none. */
export function frontmatter(text: string): string {
  return frontmatterBlock(text).block;
}

/**
 * The `name:` line: `name:`, any run of tab, LF, FF, CR and space, then the rest of a line, captured. The lookarounds
 * `(?<![^\n])` and `(?![^\n])` anchor at the text's ends and at LF only, where the `m` flag's `^` and `$` would also
 * break at CR, U+2028 and U+2029. The white-space run crosses a newline, so a bare `name:` line captures the next line.
 */
const SKILL_NAME = /(?<![^\n])name:[\t\n\f\r ]*([^\n]+)(?![^\n])/;

const QUOTE_RUN = /^["']+|["']+$/g;

/**
 * The skill name in `text`: the first match of `SKILL_NAME`, trimmed of white space, then any run of double and single
 * quotes taken off both ends. `undefined` when nothing matches. The input may be a frontmatter block or a whole file;
 * CR stays in the text and the trim strips it from the capture.
 */
export function skillName(text: string): string | undefined {
  const match = SKILL_NAME.exec(text);
  if (match === null) return undefined;
  return (match[1] ?? "").trim().replace(QUOTE_RUN, "");
}

/** The lines of the block that count as the head of the document: the opener holds line 1, so 19 more. */
const HEAD_LINES = 19;

const FRONTMATTER_MESSAGE =
  "missing required frontmatter (name/description/version) in the first 20 lines";

/** Every directory named `skills` that the tree walk delivers, in default sort order. */
function skillParents(root: string, tracked: () => Tracked): string[] {
  const parents: string[] = [];
  walkTree(
    root,
    (rel, entry) => {
      if (entry.isDirectory && entry.name === "skills") parents.push(rel);
    },
    tracked,
  );
  return parents.sort();
}

/** Every file named `SKILL.md` that the tree walk delivers, in default sort order. */
function skillFiles(root: string, tracked: () => Tracked): string[] {
  const files: string[] = [];
  walkTree(
    root,
    (rel, entry) => {
      if (!entry.isDirectory && entry.name === "SKILL.md") files.push(rel);
    },
    tracked,
  );
  return files.sort();
}

/** Whether `${rel}/SKILL.md` exists; a symbolic link on the way to it is an audit error. */
function skillMdOnDisk(root: string, rel: string): boolean {
  try {
    statSync(checkedRepoPath(root, `${rel}/SKILL.md`));
    return true;
  } catch (error) {
    if (isFileErrorOfKind(error, "symlink"))
      throw auditFileError("checking", `${rel}/SKILL.md`, error);
    return false;
  }
}

/**
 * `skill-md-present` over every child directory of a delivered `skills` directory. Below a work folder's `temp` only
 * the children that hold a tracked file count, and only a tracked `SKILL.md` makes one present: the disk is not
 * consulted there, so an untracked file or link can neither satisfy the rule nor raise an audit error, matching what
 * the walk delivers and what a checkout of the committed tree would hold.
 */
function missingSkillMd(root: string, tracked: () => Tracked): Failure[] {
  const failures: Failure[] = [];
  for (const parent of skillParents(root, tracked)) {
    const scratch = inWorkFolderTemp(parent);
    let dir: string;
    try {
      dir = checkedRepoPath(root, parent);
    } catch (error) {
      throw auditFileError("reading", parent, error);
    }
    let entries: ReturnType<typeof seams.readDir>;
    try {
      entries = seams.readDir(dir);
    } catch (error) {
      throw auditFileError("reading", parent, repoFileErrorOf(error, parent));
    }
    for (const entry of entries) {
      if (!entry.isDirectory) continue;
      const rel = `${parent}/${entry.name}`;
      if (scratch && !tracked().dirs.has(rel)) continue;
      const present = scratch
        ? tracked().files.has(`${rel}/SKILL.md`)
        : skillMdOnDisk(root, rel);
      if (!present)
        failures.push({
          rule: "skill-md-present",
          path: rel,
          message: "missing SKILL.md",
        });
    }
  }
  return failures;
}

function skillFindings(root: string, tracked: () => Tracked): Failure[] {
  const failures: Failure[] = [];
  const names = new Map<string, string>();
  for (const rel of skillFiles(root, tracked)) {
    let text: string;
    try {
      text = readRepoFile(root, rel);
    } catch (error) {
      throw auditFileError("reading", rel, error);
    }
    const found = frontmatterBlock(text);
    const block = found.closed ? found.block : "";
    const head = block.split("\n").slice(0, HEAD_LINES);
    const has = (prefix: string): boolean =>
      head.some((line) => line.startsWith(prefix));
    if (!has("name:") || !has("description:") || !has("version:"))
      failures.push({
        rule: "skill-frontmatter",
        path: rel,
        message: FRONTMATTER_MESSAGE,
      });
    if (rel.split("/").includes("surfaces")) continue;
    const name = skillName(block);
    if (name === undefined) continue;
    const previous = names.get(name);
    if (previous === undefined) names.set(name, rel);
    else
      failures.push({
        rule: "skill-name-unique",
        path: rel,
        message: `duplicate skill name ${JSON.stringify(name)}: ${previous} and ${rel}`,
      });
  }
  return failures;
}

/**
 * The `inline` family: every child of a `skills` directory holds a `SKILL.md`, every `SKILL.md` carries `name:`,
 * `description:` and `version:` lines inside a closed block in its first 20 lines, and skill names are unique outside
 * `surfaces`. The `skill-md-present` findings come first; then each `SKILL.md` in default sort order reports its
 * `skill-frontmatter` and then its `skill-name-unique` finding. A link at a child's `SKILL.md`, and any directory or
 * file that cannot be read, is an audit error. Below a work folder's `temp` only what git tracks is checked: the
 * context's lookup decides that, or one that runs git with `process.env` when the context carries none, and a git
 * failure is an audit error.
 */
export const INLINE_FAMILY: Family = {
  name: "inline",
  rules: ["skill-md-present", "skill-frontmatter", "skill-name-unique"],
  run: (root, context) => {
    const tracked = context.tracked ?? skillsTrackedLookup(root, process.env);
    return [...missingSkillMd(root, tracked), ...skillFindings(root, tracked)];
  },
};

/**
 * A scalar read from frontmatter. `value` is what the YAML 1.2 core schema resolves it to. `text` is the string itself
 * for a string, and the text as written for anything else (`0x10`, `~`, or nothing for a key with no value).
 */
export interface FrontmatterScalar {
  readonly kind: "scalar";
  readonly value: string | number | boolean | null;
  readonly text: string;
}

/** A sequence read from frontmatter, with its items in source order. */
export interface FrontmatterSeq {
  readonly kind: "seq";
  readonly items: readonly FrontmatterNode[];
}

/** One key of a mapping and its value. */
export interface FrontmatterEntry {
  readonly key: string;
  readonly value: FrontmatterNode;
}

/**
 * A mapping read from frontmatter. Every key is a string and appears once: `pairs` holds the keys in source order and
 * `entries` indexes the same values by key.
 */
export interface FrontmatterMap {
  readonly kind: "map";
  readonly pairs: readonly FrontmatterEntry[];
  readonly entries: ReadonlyMap<string, FrontmatterNode>;
}

/** Any value read from frontmatter. */
export type FrontmatterNode =
  FrontmatterScalar | FrontmatterSeq | FrontmatterMap;

// Frontmatter is untrusted text and finding text reaches terminals and CI logs, where a control character could
// rewrite the line it is printed on.
const NON_PRINTABLE = /[\u0000-\u0008\u000a-\u001f\u007f-\u009f\u2028\u2029]/g;

/**
 * `text` with every C0 control but tab, DEL, every C1 control, U+2028 and U+2029 written as a `\uXXXX` escape, so
 * it prints as it reads. Every piece of frontmatter text a finding shows goes through it; rules decide on the text
 * itself.
 */
export function printable(text: string): string {
  return text.replace(
    NON_PRINTABLE,
    (char) => `\\u${char.charCodeAt(0).toString(16).padStart(4, "0")}`,
  );
}

/** Why a frontmatter block was refused, as one printable line of text. */
export class FrontmatterError extends Error {
  constructor(message: string) {
    super(printable(oneLine(message)));
    this.name = "FrontmatterError";
  }
}

/** The refusal `reason`, located by the 1-based line of `offset` within `block`. */
function refusal(reason: string, block: string, offset: number | undefined) {
  if (offset === undefined) return new FrontmatterError(reason);
  const line = block.slice(0, offset).split("\n").length;
  return new FrontmatterError(`${reason} (block line ${line})`);
}

/**
 * Converts one node of the parsed document, refusing what frontmatter may not hold: an alias, an anchor, an explicit
 * tag, and a mapping key that is anything but a string scalar. `node.tag` is set only for a tag written in the source.
 */
function convert(node: unknown, block: string): FrontmatterNode {
  if (node === null || node === undefined)
    return { kind: "scalar", value: null, text: "" };
  if (isAlias(node))
    throw refusal("aliases are not allowed", block, node.range?.[0]);
  if (!isYamlMap(node) && !isYamlSeq(node) && !isYamlScalar(node))
    throw new FrontmatterError("frontmatter holds a value that cannot be read");
  const at = node.range?.[0];
  if (node.anchor !== undefined)
    throw refusal("anchors are not allowed", block, at);
  if (node.tag !== undefined)
    throw refusal("explicit tags are not allowed", block, at);
  if (isYamlMap(node)) {
    const pairs: FrontmatterEntry[] = [];
    for (const pair of node.items) {
      const key: unknown = pair.key;
      if (!isYamlScalar(key) || typeof key.value !== "string")
        throw refusal(
          "mapping keys must be strings",
          block,
          isNode(key) ? key.range?.[0] : at,
        );
      convert(key, block);
      pairs.push({ key: key.value, value: convert(pair.value, block) });
    }
    return {
      kind: "map",
      pairs,
      entries: new Map(pairs.map((pair) => [pair.key, pair.value])),
    };
  }
  if (isYamlSeq(node))
    return {
      kind: "seq",
      items: node.items.map((item) => convert(item, block)),
    };
  const value = node.value;
  if (typeof value === "string") return { kind: "scalar", value, text: value };
  if (value === null || typeof value === "number" || typeof value === "boolean")
    return { kind: "scalar", value, text: node.source ?? String(value) };
  throw refusal("frontmatter holds a value that cannot be read", block, at);
}

/**
 * Reads a frontmatter block as one YAML 1.2 document under the core schema and returns its top-level mapping. The block
 * is untrusted text, so it is refused, with a `FrontmatterError`, when the parser reports any error or warning (a
 * duplicate key and a second document are errors), when it holds an anchor, an alias, an explicit tag of any kind or a
 * mapping key that is not a string, and when its top level is not a mapping: `frontmatter holds no mapping` for a block
 * that is empty, whitespace, comment-only or null. `<<` is an ordinary key. A refusal names the line counted from the
 * first line of the block. Nesting too deep to read throws an error of another type; callers treat every throw alike.
 */
export function parseFrontmatter(block: string): FrontmatterMap {
  const document = parseDocument(block, {
    schema: "core",
    version: "1.2",
    merge: false,
    uniqueKeys: true,
    customTags: [],
    prettyErrors: false,
  });
  const problem = document.errors[0] ?? document.warnings[0];
  if (problem !== undefined)
    throw refusal(problem.message, block, problem.pos[0]);
  const root =
    document.contents === null ? undefined : convert(document.contents, block);
  if (root === undefined || (root.kind === "scalar" && root.value === null))
    throw new FrontmatterError("frontmatter holds no mapping");
  if (root.kind !== "map")
    throw new FrontmatterError(
      `the top level is a ${root.kind === "seq" ? "sequence" : "scalar"}, not a mapping`,
    );
  return root;
}

/** Whether `node` is a mapping. */
export function isMap(
  node: FrontmatterNode | undefined,
): node is FrontmatterMap {
  return node?.kind === "map";
}

/** Whether `node` is a sequence. */
export function isSeq(
  node: FrontmatterNode | undefined,
): node is FrontmatterSeq {
  return node?.kind === "seq";
}

/** Whether `node` is a scalar. */
export function isScalar(
  node: FrontmatterNode | undefined,
): node is FrontmatterScalar {
  return node?.kind === "scalar";
}

/** Whether `node` is a scalar that resolves to a boolean. */
export function isBoolean(node: FrontmatterNode | undefined): boolean {
  return isScalar(node) && typeof node.value === "boolean";
}

/** The string a string scalar holds, undefined for any other node and for no node. */
export function stringText(
  node: FrontmatterNode | undefined,
): string | undefined {
  return isScalar(node) && typeof node.value === "string"
    ? node.value
    : undefined;
}

/** Follows a path of mapping keys from `node`; undefined as soon as a step is missing or is not a mapping. */
export function lookup(
  node: FrontmatterNode | undefined,
  ...path: string[]
): FrontmatterNode | undefined {
  let current = node;
  for (const key of path) {
    if (!isMap(current)) return undefined;
    current = current.entries.get(key);
  }
  return current;
}

/**
 * A value as a finding message shows it: a string as itself, any other scalar as JavaScript prints its resolved value
 * (`16` for `0x10`, `null`), a sequence as `[a, b]` and a mapping as `{k: v}` in source order. Non-printable
 * characters in keys and strings are written as `\uXXXX` escapes, so the result is one printable line.
 */
export function render(node: FrontmatterNode): string {
  if (node.kind === "seq") return `[${node.items.map(render).join(", ")}]`;
  if (node.kind === "map")
    return `{${node.pairs.map((pair) => `${printable(pair.key)}: ${render(pair.value)}`).join(", ")}}`;
  return printable(String(node.value));
}

/** The text of the file at `rel`, or the audit error for a file that cannot be read. */
function readAsset(root: string, rel: string): string {
  try {
    return readRepoFile(root, rel);
  } catch (error) {
    throw auditFileError("reading", rel, error);
  }
}

const UNCLOSED_MESSAGE = "frontmatter block is never closed by a bare ---";

function frontmatterRules(root: string): Failure[] {
  const failures: Failure[] = [];
  for (const rel of assetFiles(root)) {
    const found = frontmatterBlock(readAsset(root, rel));
    if (found.block === "") continue;
    if (!found.closed) {
      failures.push({
        rule: "frontmatter-yaml",
        path: rel,
        message: UNCLOSED_MESSAGE,
      });
      continue;
    }
    try {
      parseFrontmatter(found.block);
    } catch (error) {
      failures.push({
        rule: "frontmatter-yaml",
        path: rel,
        message: `frontmatter is not a YAML mapping: ${oneLine(errorMessage(error))}`,
      });
    }
  }
  return failures;
}

/**
 * The `frontmatter` family: every `skills/<name>/SKILL.md` whose block is closed holds a YAML mapping, and a block that
 * opens and never closes is reported once. A missing or empty block is left to `eval-metadata`. Whatever
 * `parseFrontmatter` refuses is reported with its reason after the prefix, on one line.
 */
export const FRONTMATTER_FAMILY: Family = {
  name: "frontmatter",
  rules: ["frontmatter-yaml"],
  run: frontmatterRules,
};

const EM_DASH = String.fromCodePoint(0x2014);

/** The validation history heading at the text's start or after an LF, but not after the `m` flag's other breaks. */
const HISTORY_FOOTER = /(?<![^\n])## Validation history/;

const EVAL_RECORD_PATH = "evals/skills.yaml";

function evalMetadataRules(root: string): Failure[] {
  const files = assetFiles(root);
  if (files.length === 0)
    throw new Error(
      `skills: no SKILL.md assets found ${EM_DASH} nothing to audit`,
    );
  const failures: Failure[] = [];
  for (const rel of files) {
    const text = readAsset(root, rel);
    if (frontmatter(text) === "")
      failures.push({
        rule: "eval-frontmatter-present",
        path: rel,
        message: "empty or missing frontmatter",
      });
    if (HISTORY_FOOTER.test(text))
      failures.push({
        rule: "eval-history-footer",
        path: rel,
        message: `retired '## Validation history' footer found ${EM_DASH} record evals in ${EVAL_RECORD_PATH} instead`,
      });
  }
  return failures;
}

/**
 * The `eval-metadata` family: every `skills/<name>/SKILL.md` holds a frontmatter block, then no retired
 * `## Validation history` footer line. Zero assets is an audit error, and this is the only family that raises it.
 */
export const EVAL_METADATA_FAMILY: Family = {
  name: "eval-metadata",
  rules: ["eval-frontmatter-present", "eval-history-footer"],
  run: evalMetadataRules,
};

/** The eval keys a skill's frontmatter no longer holds. */
const EVAL_MOVED_KEYS: ReadonlySet<string> = new Set([
  "last-evaluated",
  "eval-status",
  "eval-digest",
  "model-ladders-reviewed",
]);

/**
 * Appends the dotted path of every moved key under `node`. A sequence item's path carries its index, a key path is the
 * key alone when the prefix is empty, and children are visited after the check so a moved key under a moved key is
 * found too. The path is text for a finding, so each key in it is made printable.
 */
function movedEvalKeys(
  node: FrontmatterNode,
  prefix: string,
  found: string[],
): void {
  if (node.kind === "map") {
    for (const pair of node.pairs) {
      const key = printable(pair.key);
      const at = prefix === "" ? key : `${prefix}.${key}`;
      if (EVAL_MOVED_KEYS.has(pair.key)) found.push(at);
      movedEvalKeys(pair.value, at, found);
    }
  } else if (node.kind === "seq") {
    node.items.forEach((child, i) => {
      movedEvalKeys(child, `${prefix}[${i}]`, found);
    });
  }
}

function evalFrontmatterCleanRules(root: string): Failure[] {
  const failures: Failure[] = [];
  for (const rel of assetFiles(root)) {
    const found = frontmatterBlock(readAsset(root, rel));
    if (found.block === "" || !found.closed) continue;
    let doc: FrontmatterMap;
    try {
      doc = parseFrontmatter(found.block);
    } catch {
      continue;
    }
    const keys: string[] = [];
    movedEvalKeys(doc, "", keys);
    keys.sort();
    for (const key of keys)
      failures.push({
        rule: "eval-frontmatter-clean",
        path: rel,
        message: `frontmatter holds ${key}; eval results belong in ${EVAL_RECORD_PATH}, not in the skill file`,
      });
  }
  return failures;
}

/**
 * The `eval-frontmatter-clean` family: no moved eval key appears in a closed, parseable frontmatter block at any depth,
 * one failure per occurrence in default sort order of its key path. A missing, unclosed or unparseable block is skipped.
 */
export const EVAL_FRONTMATTER_CLEAN_FAMILY: Family = {
  name: "eval-frontmatter-clean",
  rules: ["eval-frontmatter-clean"],
  run: evalFrontmatterCleanRules,
};
