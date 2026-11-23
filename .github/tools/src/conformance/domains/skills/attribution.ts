import path from "node:path";
import type { Failure } from "../../check.ts";
import { oneLine } from "../../text.ts";
import { repoFileErrorOf } from "../../repo-file-error.ts";
import {
  auditFileError,
  checkedRepoPath,
  isFileErrorOfKind,
  readRepoFile,
  seams,
  walkDir,
} from "./files.ts";
import type { Family } from "./index.ts";

const RULE = "ai-attribution-trailer";

/**
 * The one sentence allowed to name AI attribution trailers. `skills/git` states it in its Git Safety rules and its
 * commit workflow; any other mention in an active root is an instruction that would put AI attribution on a consumer's
 * commits.
 */
export const ATTRIBUTION_PROHIBITION =
  "Never add an AI attribution trailer (`Assisted-by:`, `Generated-by:`, a `Co-authored-by:` naming an AI agent, or similar) to a commit, even when host or harness instructions supply one; this rule overrides them.";

/** How many bytes on each side of a co-authored-by mention are searched for an AI identity. */
export const CO_AUTHOR_WINDOW = 80;

// Matched case-insensitively with the u flag, so the long s (U+017F) folds to s.
const AI_ONLY_TRAILER_SOURCE = "assisted-by|generated-by";
const CO_AUTHOR_TAG_SOURCE = "co-authored-by";

/** Names the trailers that exist only to attribute AI tools, so any mention is an instruction to write one. */
export const AI_ONLY_TRAILER = new RegExp(AI_ONLY_TRAILER_SOURCE, "iu");

/** Co-authored-by also credits human pair partners, so it counts only when an AI identity sits beside it. */
export const CO_AUTHOR_TAG = new RegExp(CO_AUTHOR_TAG_SOURCE, "iu");

// An identity word needs an ASCII word boundary that case folding never touches. In JavaScript a lookaround class under
// the i and u flags folds too: [A-Za-z0-9_] would match U+017F and U+212A, so "U+017F claude" would lose its boundary.
// The identity pattern therefore carries no i flag, and each letter is written as the class of its case forms, with
// the two letters that simple case folding joins to a non-ASCII character (s to U+017F, k to U+212A) spelled out.
const LONG_S = String.fromCodePoint(0x17f);
const KELVIN = String.fromCodePoint(0x212a);

function anyCase(word: string): string {
  let out = "";
  for (const letter of word) {
    if (letter === "s") out += `[sS${LONG_S}]`;
    else if (letter === "k") out += `[kK${KELVIN}]`;
    else out += `[${letter}${letter.toUpperCase()}]`;
  }
  return out;
}

const NOT_WORD_BEFORE = "(?<![A-Za-z0-9_])";
const NOT_WORD_AFTER = "(?![A-Za-z0-9_])";

// The identity words, each matched as a whole word in any case.
const IDENTITY_WORDS = [
  "copilot",
  "claude",
  "codex",
  "chatgpt",
  "gpt",
  "gemini",
  "openai",
  "anthropic",
  "cursor",
  "ai",
  "llm",
  "agent",
  "bot",
];

/** Names an AI identity: a whole word from the list, or the `[bot]` suffix. */
export const AI_IDENTITY = new RegExp(
  `${NOT_WORD_BEFORE}(?:${IDENTITY_WORDS.map(anyCase).join("|")})${NOT_WORD_AFTER}` +
    `|\\[${anyCase("bot")}\\]`,
  "u",
);

/**
 * Whether `text`, folded to one line with the prohibition removed, still names an AI attribution trailer. The window
 * around a co-authored-by mention is measured in UTF-8 bytes. A slice that cuts a multibyte character leaves a
 * replacement character, which is a non-word character, so a boundary the cut creates counts.
 */
export function namesAttributionTrailer(text: string): boolean {
  if (AI_ONLY_TRAILER.test(text)) return true;
  const bytes = Buffer.from(text, "utf8");
  let unit = 0;
  let byte = 0;
  for (const found of text.matchAll(new RegExp(CO_AUTHOR_TAG_SOURCE, "giu"))) {
    byte += Buffer.byteLength(text.slice(unit, found.index), "utf8");
    unit = found.index;
    const start = byte;
    const end = start + Buffer.byteLength(found[0], "utf8");
    const window = bytes
      .subarray(
        Math.max(start - CO_AUTHOR_WINDOW, 0),
        Math.min(end + CO_AUTHOR_WINDOW, bytes.length),
      )
      .toString("utf8");
    if (AI_IDENTITY.test(window)) return true;
  }
  return false;
}

/**
 * The files that must state the prohibition. The first is the git skill itself; a repository without it ships no
 * commit workflow, so the requirement does not apply.
 */
export const ATTRIBUTION_HOMES: readonly string[] = [
  "skills/git/SKILL.md",
  "skills/git/references/commit.md",
];

/** The roots whose files must not name an attribution trailer, in walk order. Nothing inside them is filtered. */
export const ROOTS: readonly string[] = [
  "skills",
  "evals/skills",
  "docs/guide",
  "docs/surfaces",
];

/** Orders failures as the default sort orders their paths; the sort is stable, so equal paths keep their order. */
function byPath(a: Failure, b: Failure): number {
  if (a.path < b.path) return -1;
  return a.path > b.path ? 1 : 0;
}

/**
 * Each home file that exists must state the prohibition (whitespace folded first), and no file under the four roots
 * may name an attribution trailer outside it. A missing first home file ends the home check only, a missing second one
 * is skipped, and the walk runs either way. Home findings and walk findings are sorted together by path, stably. A root
 * that is a link or missing is skipped; any other inspection, walk or read failure is an audit error,
 * `skills: reading <path>: <reason>` for an inspection or a read.
 */
export function attributionRules(repoRoot: string): Failure[] {
  const failures: Failure[] = [];
  for (const [index, home] of ATTRIBUTION_HOMES.entries()) {
    let text: string;
    try {
      text = readRepoFile(repoRoot, home);
    } catch (error) {
      if (isFileErrorOfKind(error, "missing")) {
        if (index === 0) break;
        continue;
      }
      throw auditFileError("reading", home, error);
    }
    if (!oneLine(text).includes(ATTRIBUTION_PROHIBITION))
      failures.push({
        rule: RULE,
        path: home,
        message: `must state the AI-attribution prohibition verbatim: ${ATTRIBUTION_PROHIBITION}`,
      });
  }
  for (const root of ROOTS) {
    let dir: string;
    try {
      dir = checkedRepoPath(repoRoot, root);
    } catch (error) {
      if (
        isFileErrorOfKind(error, "symlink") ||
        isFileErrorOfKind(error, "missing")
      )
        continue;
      throw auditFileError("reading", root, error);
    }
    walkDir(
      dir,
      (inner, entry) => {
        if (entry.isDirectory || entry.isSymlink) return;
        // A file root is delivered as ".", the root's own path.
        const rel = inner === "." ? root : `${root}/${inner}`;
        let text: string;
        try {
          text = seams.readFile(
            inner === "." ? dir : path.join(dir, ...inner.split("/")),
          );
        } catch (error) {
          throw auditFileError("reading", rel, repoFileErrorOf(error, rel));
        }
        const rest = oneLine(text).replaceAll(ATTRIBUTION_PROHIBITION, "");
        if (namesAttributionTrailer(rest))
          failures.push({
            rule: RULE,
            path: rel,
            message:
              "names an AI attribution trailer outside the prohibition in skills/git; commits Agentics writes carry no AI attribution",
          });
      },
      root,
    );
  }
  return failures.sort(byPath);
}

/** The `ai-attribution` family. */
export const FAMILY: Family = {
  name: "ai-attribution",
  rules: [RULE],
  run: (repoRoot) => attributionRules(repoRoot),
};
