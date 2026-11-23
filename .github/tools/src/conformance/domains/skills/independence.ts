import path from "node:path";
import type { Failure } from "../../check.ts";
import { SkillsTreeModel } from "./tree-model.ts";
import type { Family } from "./index.ts";

/**
 * The one rule id this family emits. A skill calls the flow helper by its command name, so it must not depend on an Agentics
 * hook path or on repository files outside `skills/`.
 */
export const INDEPENDENCE_RULE = "skill-repo-independence";

// A line starts only after LF, so line starts are explicit lookarounds: the JavaScript `m` flag would also break at CR,
// U+2028 and U+2029. `^` is the lookbehind below. `\w` stays ASCII with the `u` flag and no `i`. The negated classes
// consume one character of any kind, a line break and a non-ASCII character included.
const START = "(?<![^\\n])";

/**
 * Agentics paths that sit outside `skills/`. A match needs a boundary before it: a character that is not a word character,
 * a dot, a slash or a hyphen. The first group also matches right after `../` or `./`, because those paths have no
 * target inside `skills/`. `evals/` and `hooks/` keep the plain boundary, because a skill may hold folders of those
 * names; `RELATIVE_ESCAPE_TOKENS` handles a relative link to them. A `./` link to `evals/` or `hooks/` stays inside the
 * skill, so only the `evals/skills/` bank layout is matched after `./`.
 */
export const PATH_TOKENS = new RegExp(
  `(?:${START}|[^\\w./-]|\\.\\.?/)(docs/CHARTER|docs/surfaces/|docs/RENAMES|docs/maintainers/)` +
    `|(?:${START}|[^\\w./-])(evals/|hooks/)` +
    `|(?:${START}|[^\\w./-])\\./(evals/skills/)`,
  "dgu",
);

/**
 * Relative links to `evals/` or `hooks/`. Such a link is a dependency only when it climbs out of `skills/`: a file N
 * directories below the repository root reaches the root after N `../` segments, while fewer leave it inside `skills/`.
 */
export const RELATIVE_ESCAPE_TOKENS = new RegExp(
  `(?:${START}|[^\\w./-])((?:\\.\\./)+)(evals/|hooks/)`,
  "dgu",
);

/**
 * Slash-bearing path tokens that may be relative links. The boundary excludes a colon and a slash, so the host and path
 * of a URL and the tail of an absolute or variable-rooted path never start a token.
 */
export const RELATIVE_PATH_TOKENS = new RegExp(
  `(?:${START}|[^\\w./:~-])((?:[\\w.@%+~-]+/)+[\\w.@%+~-]*)`,
  "dgu",
);

/**
 * Agentics-specific names that need no boundary: no consumer text contains them by accident. A policy key such as
 * `policy.draft_prs`, the settings file `.agentics/hooks.json` and the `AGX_RELEASE_RECORD` variable are deliberately
 * not reported, so that a skill can name the setting that governs a rule.
 */
export const NAME_TOKENS = new RegExp(
  "\\.claude-plugin|\\.github/plugin|plugin\\.sh|THIRD_PARTY_NOTICES|muehz/agentics|run-hook|agentics-hook|hooks\\.[a-z]+\\.json|command-guardian|release-guardian|secrets-guardian|secrets-scanner|\\.antigravity-plugin",
  "dgu",
);

type Span = readonly [number, number];

/** The number of directories between the repository root and a file at `rel`, plus one (POSIX semantics, `/` only). */
function depthOf(rel: string): number {
  return path.posix.dirname(rel).split("/").length;
}

/**
 * Whether following `target` from a file at `rel` reaches the repository root or climbs past it. Reaching the root
 * counts: the root holds no skill content, so a link that gets there depends on the checkout around the skills. A later
 * descent does not undo it, because the link has already left `skills/`.
 */
export function leavesSkills(rel: string, target: string): boolean {
  let depth = depthOf(rel);
  for (const segment of target.split("/")) {
    if (segment === "" || segment === ".") continue;
    if (segment === "..") {
      depth -= 1;
      if (depth <= 0) return true;
    } else {
      depth += 1;
    }
  }
  return false;
}

/** Whether `[start, end)` shares a position with any span. */
export function overlapsAny(
  spans: readonly Span[],
  start: number,
  end: number,
): boolean {
  return spans.some(([from, to]) => from < end && start < to);
}

/** The span of the first participating capture group of a match, or of the whole match when none participated. */
function tokenSpan(match: RegExpMatchArray): Span {
  const indices = match.indices;
  for (let i = 1; i < match.length; i += 1) {
    const span = indices?.[i];
    if (span !== undefined) return [span[0], span[1]];
  }
  const whole = indices?.[0];
  if (whole === undefined)
    throw new Error("independence: match without indices");
  return [whole[0], whole[1]];
}

/** The number of non-overlapping occurrences of `needle` in `text`, scanning left to right past each match. */
function countOf(text: string, needle: string): number {
  return text.split(needle).length - 1;
}

function finding(
  rel: string,
  text: string,
  start: number,
  end: number,
): Failure {
  return {
    rule: INDEPENDENCE_RULE,
    path: rel,
    message: `line ${1 + countOf(text.slice(0, start), "\n")} names ${JSON.stringify(text.slice(start, end))}: a skill must not depend on an Agentics hook or on a file outside skills/`,
  };
}

/**
 * The findings for one file, in the order they are found and before the final sort: the named spans of `PATH_TOKENS`
 * then `NAME_TOKENS`, the escaping `../` links to `evals/` and `hooks/`, then any other relative link that leaves
 * `skills/` and does not overlap a span already recorded.
 */
export function scanText(rel: string, text: string): Failure[] {
  const failures: Failure[] = [];
  const named: Span[] = [];
  for (const re of [PATH_TOKENS, NAME_TOKENS]) {
    for (const match of text.matchAll(re)) {
      const [start, end] = tokenSpan(match);
      named.push([start, end]);
      failures.push(finding(rel, text, start, end));
    }
  }
  const depth = depthOf(rel);
  for (const match of text.matchAll(RELATIVE_ESCAPE_TOKENS)) {
    const climb = match[1];
    const target = match.indices?.[2];
    if (climb === undefined || target === undefined) continue;
    if (countOf(climb, "../") >= depth) {
      named.push([target[0], target[1]]);
      failures.push(finding(rel, text, target[0], target[1]));
    }
  }
  // Any other relative link that leaves skills/ is a dependency too, whatever name follows the climb. A link already
  // reported through the name it ends in is not reported a second time.
  for (const match of text.matchAll(RELATIVE_PATH_TOKENS)) {
    const span = match.indices?.[1];
    if (span === undefined) continue;
    const [start, end] = span;
    if (
      !leavesSkills(rel, text.slice(start, end)) ||
      overlapsAny(named, start, end)
    )
      continue;
    failures.push(finding(rel, text, start, end));
  }
  return failures;
}

/** Orders failures by path, then message, as the default sort orders strings. */
function byPathThenMessage(a: Failure, b: Failure): number {
  if (a.path !== b.path) return a.path < b.path ? -1 : 1;
  if (a.message !== b.message) return a.message < b.message ? -1 : 1;
  return 0;
}

/**
 * Audits every eligible file under `skills/`: findings sort by path then message in default sort order, so
 * `line 10` comes before `line 2` and two identical findings are both kept. Throws the no-files audit error when nothing
 * was scanned or `skills` is a link.
 */
export function independenceRules(
  repoRoot: string,
  tree = new SkillsTreeModel(repoRoot),
): Failure[] {
  const failures: Failure[] = [];
  tree.walkText("repository independence", (rel, text) => {
    failures.push(...scanText(rel, text));
  });
  return failures.sort(byPathThenMessage);
}

export const FAMILY: Family = {
  name: "independence",
  rules: [INDEPENDENCE_RULE],
  run: (root, context) => independenceRules(root, context?.tree),
};
