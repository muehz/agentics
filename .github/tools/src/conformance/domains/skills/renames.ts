import path from "node:path";
import type { Failure } from "../../check.ts";
import { repoFileErrorOf } from "../../repo-file-error.ts";
import { checkedRepoPath, isFileErrorOfKind, seams, walkDir } from "./files.ts";
import type { Family } from "./index.ts";

const RULE = "retired-skill-reference";

/** The roots the scan covers, in scan order. None of them is filtered: no NUL check and no skipped directory. */
export const ROOTS: readonly string[] = [
  "skills",
  "docs/guide",
  "docs/surfaces",
  "evals/skills",
  ".github/scripts",
  ".claude-plugin",
  ".codex-plugin",
  ".github/plugin",
];

/**
 * A skill that no longer ships: the paths that must not exist, the skill-syntax references that must not appear in
 * active roots, and the message that names its replacement. A path ending in a slash owns everything below it.
 */
export type RetiredSkill = {
  paths: readonly string[];
  references: readonly RegExp[];
  message: string;
};

// The arrow is built from its code point so the source stays ASCII.
const ARROW = String.fromCodePoint(0x2192);

// White space is [\t\n\f\r ] only and a word boundary is ASCII-only, so both are written out rather than left to \s and
// \b, whose Unicode forms would match more; the patterns carry no flag that folds case.
const SPACE = "[\\t\\n\\f\\r ]";
const NOT_WORD_AFTER = "(?![A-Za-z0-9_])";

// Each `(?:\.\./)+` run below is preceded by the lookbehind `(?<!\.\./)`, which lets a run start only at its first
// segment. Without it, V8 retries the run from every segment when no retired name follows, which is quadratic in the
// run's length. A match still starts where the leftmost match without the lookbehind would.

const PLANNING_PATH_OR_NAME =
  /(?:skills\/|(?<!\.\.\/)(?:\.\.\/)+)(?:ideation|planning)(?:\/|["'`])|`(?:\/|agentics:)?(?:ideation|planning)(?:`|[ :])/u;

// Read with A standing for the arrow U+2192, \s for SPACE and \b for NOT_WORD_AFTER, the pattern is these four
// alternatives:
//   ideation(?:\s*A|\s+(?:owns|workflow|skill)\b)
//   planning(?:\s*A|\s+(?:owns|skill)\b)
//   A\s*(?:ideation|planning)\b
//   [|<]\s*(?:ideation|planning)\s*[|>]
// The first two alternatives have no leading boundary, so a name ending in ideation or planning matches too.
const PLANNING_PROSE = new RegExp(
  `ideation(?:${SPACE}*${ARROW}|${SPACE}+(?:owns|workflow|skill)${NOT_WORD_AFTER})` +
    `|planning(?:${SPACE}*${ARROW}|${SPACE}+(?:owns|skill)${NOT_WORD_AFTER})` +
    `|${ARROW}${SPACE}*(?:ideation|planning)${NOT_WORD_AFTER}` +
    `|[|<]${SPACE}*(?:ideation|planning)${SPACE}*[|>]`,
  "u",
);

const WORK_PATH_OR_NAME =
  /(?:skills\/|(?<!\.\.\/)(?:\.\.\/)+)work(?:\/|["'`])|`(?:\/|agentics:)?work(?:`|[ :])/u;

/**
 * The skill-syntax references to any of `names`: a skills path or a relative link to the old directory, or a backticked
 * invocation. A bare backticked name is left out because current text uses some of these names for a mode, a label or
 * the dev tool.
 */
function skillSyntax(...names: string[]): RegExp {
  const name = `(?:${names.join("|")})`;
  return new RegExp(
    `(?:skills/|(?<!\\.\\./)(?:\\.\\./)+)${name}(?:/|["'\`])|\`(?:/(?:agentics:)?|agentics:)${name}(?:\`|[ :])`,
    "u",
  );
}

// The hook name is hyphenated and unique, so any standalone occurrence is a reference; the boundaries are written out
// so they stay ASCII-only and also exclude a hyphen.
const TEST_ADVISOR = /(?<![A-Za-z0-9_-])test-advisor(?![A-Za-z0-9_-])/u;

/** The paths a retired skill owns: its skill directory and its evals bank, for each name. */
function skillPaths(...names: string[]): string[] {
  return [
    ...names.map((name) => `skills/${name}/`),
    ...names.map((name) => `evals/skills/${name}.md`),
  ];
}

/**
 * The retired skills and hooks: the ideation-and-planning and work entries first, then the later retirements grouped by
 * replacement. A file can produce one finding for each entry.
 */
export const RETIRED_SKILLS: readonly RetiredSkill[] = [
  {
    paths: [
      "skills/ideation/",
      "skills/planning/",
      "evals/skills/ideation.md",
      "evals/skills/planning.md",
    ],
    references: [PLANNING_PATH_OR_NAME, PLANNING_PROSE],
    message:
      "use intent and decompose; retired skill names remain in an active path or reference",
  },
  {
    paths: ["skills/work/", "evals/skills/work.md"],
    references: [WORK_PATH_OR_NAME],
    message:
      "routing is the automatic chain entrance and phase skills are the explicit ones; the retired work skill remains in an active path or reference",
  },
  {
    paths: skillPaths("understand", "teach"),
    references: [skillSyntax("understand", "teach")],
    message:
      "use explain and its how, why and teach modes; retired skill names remain in an active path or reference",
  },
  {
    paths: skillPaths("incident", "runbook"),
    references: [skillSyntax("incident", "runbook")],
    message:
      "use ops and its modes; retired skill names remain in an active path or reference",
  },
  {
    paths: skillPaths("blast-radius"),
    references: [skillSyntax("blast-radius")],
    message:
      "use review impact; the retired blast-radius skill remains in an active path or reference",
  },
  {
    paths: skillPaths("uiux"),
    references: [skillSyntax("uiux")],
    message:
      "adopt an outside UI/UX skill; the retired uiux skill remains in an active path or reference",
  },
  {
    paths: skillPaths("rig"),
    references: [skillSyntax("rig")],
    message:
      "use scaffold feedback-loops; the retired rig skill remains in an active path or reference",
  },
  {
    paths: skillPaths("bro"),
    references: [skillSyntax("bro")],
    message:
      "bro has no replacement; the retired bro skill remains in an active path or reference",
  },
  {
    paths: skillPaths("peer"),
    references: [skillSyntax("peer")],
    message:
      "the maintainer dev tool is .github/tools/src/peer; the retired peer skill remains in an active path or reference",
  },
  {
    paths: [],
    references: [TEST_ADVISOR],
    message:
      "the hook has no replacement; the retired test-advisor hook remains in an active path or reference",
  },
];

/** Whether `rel` is an owned path, or lies below an owned path that ends in a slash. */
export function owns(skill: RetiredSkill, rel: string): boolean {
  return skill.paths.some(
    (owned) => rel === owned || (owned.endsWith("/") && rel.startsWith(owned)),
  );
}

/** Whether any of the skill's patterns matches `text`. */
export function referencedIn(skill: RetiredSkill, text: string): boolean {
  return skill.references.some((pattern) => pattern.test(text));
}

/** The audit error for a failure to inspect, walk or read `rel`: the file error's own text, with no prefix. */
function auditError(error: unknown, rel: string): Error {
  const fileError = repoFileErrorOf(error, rel);
  return new Error(fileError.message, { cause: fileError });
}

/**
 * The retired-skill-reference rule: one finding per retired skill that a scanned file owns by path or mentions, in
 * default sort order of the paths (stable) with the entries in table order. A file below `evals/` is read from its
 * first `## happy` heading: the preamble before it records measurement history. A root that is a link or missing is
 * skipped; any other inspection, walk or read failure is an audit error.
 */
export function retiredSkillRules(repoRoot: string): Failure[] {
  const failures: Failure[] = [];
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
      throw auditError(error, root);
    }
    try {
      walkDir(
        dir,
        (inner, entry) => {
          if (entry.isDirectory || entry.isSymlink) return;
          // A file root is delivered as ".", which stands for the root's own path.
          const rel = inner === "." ? root : `${root}/${inner}`;
          let text: string;
          try {
            text = seams.readFile(
              inner === "." ? dir : path.join(dir, ...inner.split("/")),
            );
          } catch (error) {
            throw repoFileErrorOf(error, rel);
          }
          let active = text;
          if (rel.startsWith("evals/")) {
            const start = active.indexOf("\n## happy");
            if (start >= 0) active = active.slice(start);
          }
          for (const skill of RETIRED_SKILLS)
            if (owns(skill, rel) || referencedIn(skill, active))
              failures.push({ rule: RULE, path: rel, message: skill.message });
        },
        root,
      );
    } catch (error) {
      throw auditError(error, root);
    }
  }
  return failures.sort((a, b) =>
    a.path < b.path ? -1 : a.path > b.path ? 1 : 0,
  );
}

/** The `retired-skill` family. */
export const FAMILY: Family = {
  name: "retired-skill",
  rules: [RULE],
  run: (repoRoot) => retiredSkillRules(repoRoot),
};
