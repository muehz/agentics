import type { Failure } from "../../check.ts";
import { SkillsTreeModel } from "./tree-model.ts";
import type { Family } from "./index.ts";

/**
 * The one rule id this family emits. A skill states flow state through `state.tsv` and the flow helper; the names of the
 * retired mechanisms must not reappear in skill text.
 */
export const LEGACY_NAMES_RULE = "skill-legacy-names";

// A word boundary here is ASCII-word based, so the pattern spells it as explicit ASCII-word lookarounds. Every
// alternative starts and ends with a word character, so a lookbehind before and a lookahead after are the whole of
// `\b`. The pattern uses the `u` flag and no `i` flag, so no case folding widens the word class.
const WORD_START = "(?<![A-Za-z0-9_])";
const WORD_END = "(?![A-Za-z0-9_])";

/**
 * Exact text with word boundaries. The bare word "chain" is deliberately not a token: "supply chain", "chain of tasks"
 * and "Keychain state" are ordinary text.
 */
export const LEGACY_NAME_TOKENS = new RegExp(
  `${WORD_START}(?:chain state|internal/chain|events\\.tsv|log-row)${WORD_END}`,
  "gu",
);

/** Orders failures by path, then message, as the default sort orders strings. */
function byPathThenMessage(a: Failure, b: Failure): number {
  if (a.path !== b.path) return a.path < b.path ? -1 : 1;
  if (a.message !== b.message) return a.message < b.message ? -1 : 1;
  return 0;
}

/**
 * Audits every eligible file under `skills/`: one finding per match, sorted by path then message in default sort order.
 * Throws the no-files audit error when nothing was scanned or `skills` is a link.
 */
export function legacyNamesRules(
  repoRoot: string,
  tree = new SkillsTreeModel(repoRoot),
): Failure[] {
  const failures: Failure[] = [];
  tree.walkText("legacy names", (rel, text) => {
    for (const match of text.matchAll(LEGACY_NAME_TOKENS)) {
      const start = match.index;
      failures.push({
        rule: LEGACY_NAMES_RULE,
        path: rel,
        message: `line ${text.slice(0, start).split("\n").length} names ${JSON.stringify(match[0])}: this state mechanism is retired; use state.tsv and the flow helper`,
      });
    }
  });
  return failures.sort(byPathThenMessage);
}

export const FAMILY: Family = {
  name: "legacy-names",
  rules: [LEGACY_NAMES_RULE],
  run: (root, context) => legacyNamesRules(root, context?.tree),
};
