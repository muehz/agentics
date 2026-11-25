import { readdirSync, statSync } from "node:fs";
import type { Failure } from "../../check.ts";
import { FenceContainer } from "../../markdown-fence.ts";
import {
  auditFileError,
  checkedRepoPath,
  isFileErrorOfKind,
  readRepoFile,
} from "./files.ts";
import type { Family } from "./index.ts";

/** The rule id reported when a scenario bank criterion or assertion requires a placeholder with no concrete fixture value. */
export const BANK_FIXTURE_COMPLETENESS_RULE = "eval-bank-fixture-completeness";

const BANKS_DIR = "evals/skills";

const CATEGORY_HEADING = /^##\s+(happy|edge|routing_boundary|adversarial)\s*$/;
const SUB_HEADING = /^###\s+(.*)$/;
const CRITERION_START =
  /^(?:[-*+]\s+)?(?:\*\*)?(?:Correct behaviou?r|Expected|Pass)[^:\n]{0,40}:/;
const FAIL_START =
  /^(?:[-*+]\s+)?(?:\*\*)?(?:Fail|Failure|Failures)[^:\n]{0,40}:/;
const FIXTURE_START =
  /^(?:[-*+]\s+)?(?:\*\*)?(?:Fixture|Starting|Setup|Context|User|Prompt|Turn\s+\d)/i;
const TOKEN_RE =
  /(<[A-Za-z][A-Za-z0-9 _./:-]{0,30}>|\{[A-Za-z][A-Za-z0-9 _./:-]{0,30}\})/g;
const NEGATION_BEFORE =
  /\b(?:never|not|avoid|forbidden|literal|placeholder|unresolved|without|forbids?)\b/i;
const NEGATION_AFTER = /^\s*(?:placeholder|token)\b/i;

type FixtureSlot = {
  kind: string;
  token: RegExp;
  satisfied: (fixtureText: string) => boolean;
};

/**
 * Concrete fixture slots whose placeholder tokens in `Correct behavior:` or `Assertions:` require a concrete value
 * in the scenario's fixture text. Purely syntactic format slots (such as `Q<n>`, `<title>`, `<reason>`) are excluded.
 */
const FIXTURE_SLOTS: readonly FixtureSlot[] = [
  {
    kind: "hash",
    token:
      /^(?:<(?:current-hash|that hash|hash(?: [A-Za-z0-9_-]+)?)>|\{(?:current-hash|hash(?: [A-Za-z0-9_-]+)?)\})$/i,
    satisfied: (fx) =>
      /\btext=[0-9a-f]{7,40}\b/i.test(fx) ||
      /\bhash\b[^.\n]{0,40}\b[0-9a-f]{7,40}\b/i.test(fx) ||
      /\b[0-9a-f]{12,40}\b/.test(fx),
  },
  {
    kind: "sha",
    token:
      /^(?:<(?:pushed sha|full-sha|merge-base sha|red-sha|integrated sha|plan-commit sha|commit-sha|sha)>|\{(?:pushed sha|full-sha|merge-base sha|red-sha|integrated sha|plan-commit sha|commit-sha|sha)\})$/i,
    satisfied: (fx) => /\b[0-9a-f]{7,40}\b/.test(fx),
  },
  {
    kind: "thread-id",
    token: /^(?:<thread[-_ ]?id>|\{thread[-_ ]?id\})$/i,
    satisfied: (fx) =>
      /\bPRRT_[A-Za-z0-9_-]+\b/.test(fx) ||
      /\bthread[-_ ]?id(?:\s*[:=]\s*|\s+)`?[A-Za-z0-9_-]{4,}`?/i.test(fx),
  },
  {
    kind: "check-run-id",
    token: /^(?:<check[-_ ]?run[-_ ]?id>|\{check[-_ ]?run[-_ ]?id\})$/i,
    satisfied: (fx) =>
      /\bcheck[-_ ]?runs?\/\d+\b/i.test(fx) ||
      /\bcheck[-_ ]?run[-_ ]?id\b\s*[:=]\s*`?\d+`?/i.test(fx) ||
      /\bid\s*[:=]\s*`?\d{5,}`?/i.test(fx),
  },
  {
    kind: "worktree-parent",
    token:
      /^(?:<(?:wt[-_ ]?parent|worktree[-_ ]?parent)>|\{(?:wt[-_ ]?parent|worktree[-_ ]?parent)\})$/i,
    satisfied: (fx) =>
      /(?:worktree parent|WT_PARENT|parent directory)[^.\n]{0,40}(?:\/[\w.-]+|[A-Z]:\\[\w\\.-]+)/i.test(
        fx,
      ),
  },
  {
    kind: "pr-branch",
    token:
      /^(?:<(?:pr[-_ ]?branch|work[-_ ]?branch)>|\{(?:pr[-_ ]?branch|work[-_ ]?branch)\})$/i,
    satisfied: (fx) =>
      /\b(?:feat|fix|chore|docs|refactor|codex|release|hotfix)\/[a-z0-9._/-]+/i.test(
        fx,
      ) || /\bbranch\s+`[a-z0-9._/-]+`/i.test(fx),
  },
  {
    kind: "pr-author",
    token: /^(?:<pr[-_ ]?author>|\{pr[-_ ]?author\})$/i,
    satisfied: (fx) =>
      /@[a-z0-9][a-z0-9-]{1,38}\b/i.test(fx) ||
      /\bauthor\b[^.\n]{0,30}`[a-z0-9-]+`/i.test(fx),
  },
  {
    kind: "new-pr",
    token: /^(?:<new[-_ ]?pr>|\{new[-_ ]?pr\})$/i,
    satisfied: (fx) =>
      /\bnew PR\b[^.\n]{0,30}#?\d+/i.test(fx) ||
      /\bPR\s+#?\d+\b[^.\n]{0,40}\b(?:created|opened|returns?)\b/i.test(fx),
  },
  {
    kind: "work-root",
    token: /^(?:<work[-_ ]?root>|\{work[-_ ]?root\})$/i,
    satisfied: (fx) =>
      /\b(?:docs\/specs|planning\/|work[- ]?root\s*[:=]\s*`?[\w./-]+`?)\b/i.test(
        fx,
      ),
  },
  {
    kind: "folder",
    token: /^(?:<(?:work[-_ ]?)?folder>|\{(?:work[-_ ]?)?folder\})$/i,
    satisfied: (fx) =>
      /\b(?:docs\/specs|planning)\/\d{4}-\d{2}\/[a-z0-9-]+/i.test(fx),
  },
  {
    kind: "repo",
    token: /^(?:<repo>|\{repo\})$/i,
    satisfied: (fx) =>
      /(?:\/[\w.-]+\/[\w.-]+|[a-z0-9-]+\/[a-z0-9._-]+)/i.test(fx),
  },
];

type NumberedLine = { num: number; text: string };
type SubScenario = { title: string; lines: NumberedLine[] };

function hasCompletedFence(lines: readonly NumberedLine[]): boolean {
  const fence = new FenceContainer();
  let wasInside = false;
  for (const { text } of lines) {
    const inside = fence.step(text);
    if (wasInside && !inside) return true;
    wasInside = inside;
  }
  return false;
}

function checkSubScenario(
  rel: string,
  category: string,
  sub: SubScenario,
  sharedSetup: string,
): Failure[] {
  let mode: "fixture" | "expected" | "assert" | "fail" = "fixture";
  const fixtureLines: string[] = sharedSetup === "" ? [] : [sharedSetup];
  const expectedLines: NumberedLine[] = [];
  const fence = new FenceContainer();

  for (const item of sub.lines) {
    if (fence.step(item.text)) {
      fixtureLines.push(item.text);
      continue;
    }
    const trimmed = item.text.trim();
    if (trimmed === "Assertions:") {
      mode = "assert";
      continue;
    }
    if (trimmed === "Artifacts:") {
      mode = "fixture";
      continue;
    }
    if (FAIL_START.test(trimmed)) {
      mode = "fail";
      continue;
    }
    if (CRITERION_START.test(trimmed)) {
      mode = "expected";
      expectedLines.push(item);
      continue;
    }
    if (FIXTURE_START.test(trimmed)) {
      mode = "fixture";
      fixtureLines.push(item.text);
      continue;
    }
    if (mode === "expected" || mode === "assert") expectedLines.push(item);
    else if (mode === "fixture") fixtureLines.push(item.text);
  }

  const fixtureText = fixtureLines.join("\n");
  const normalizedFixture = fixtureText.replace(/\s+/g, " ");
  const failures: Failure[] = [];
  const reported = new Set<string>();

  for (const item of expectedLines) {
    for (const match of item.text.matchAll(TOKEN_RE)) {
      const token = match[1] ?? "";
      const index = match.index ?? 0;
      const before = item.text.slice(Math.max(0, index - 45), index);
      const after = item.text.slice(
        index + token.length,
        index + token.length + 25,
      );
      if (NEGATION_BEFORE.test(before) || NEGATION_AFTER.test(after)) continue;

      for (const slot of FIXTURE_SLOTS) {
        if (!slot.token.test(token)) continue;
        const key = `${slot.kind}:${token}`;
        if (reported.has(key)) continue;
        if (fixtureText.includes(token) || !slot.satisfied(normalizedFixture)) {
          reported.add(key);
          failures.push({
            rule: BANK_FIXTURE_COMPLETENESS_RULE,
            path: rel,
            message: `line ${item.num}: [${category} / ${sub.title}] unresolved ${slot.kind} placeholder ${token} has no concrete value in fixture text`,
          });
        }
      }
    }
  }
  return failures;
}

/** Audits one scenario bank's text for unresolved concrete fixture placeholders in criteria and assertions. */
export function auditBankText(rel: string, text: string): Failure[] {
  const failures: Failure[] = [];
  const lines = text.split(/\r?\n/);
  const fence = new FenceContainer();
  let category: string | undefined;
  let leadLines: NumberedLine[] = [];
  let subs: SubScenario[] = [];
  let currentSub: SubScenario | undefined;

  const flushCategory = () => {
    if (category === undefined) return;
    if (currentSub !== undefined) subs.push(currentSub);
    const leadIsScenario = subs.length === 0 || hasCompletedFence(leadLines);
    const sharedSetup = leadIsScenario
      ? ""
      : leadLines.map((item) => item.text).join("\n");
    if (leadIsScenario && leadLines.length > 0)
      failures.push(
        ...checkSubScenario(
          rel,
          category,
          { title: "(lead)", lines: leadLines },
          "",
        ),
      );
    for (const sub of subs)
      failures.push(...checkSubScenario(rel, category, sub, sharedSetup));
  };

  for (let i = 0; i < lines.length; i += 1) {
    const line = lines[i] ?? "";
    const inFence = fence.step(line);
    if (!inFence) {
      const catMatch = CATEGORY_HEADING.exec(line);
      if (catMatch !== null) {
        flushCategory();
        category = catMatch[1];
        leadLines = [];
        subs = [];
        currentSub = undefined;
        continue;
      }
      if (category !== undefined) {
        const subMatch = SUB_HEADING.exec(line);
        if (subMatch !== null) {
          if (currentSub !== undefined) subs.push(currentSub);
          currentSub = {
            title: (subMatch[1] ?? "").trim() || "(untitled)",
            lines: [],
          };
          continue;
        }
      }
    }
    if (category !== undefined) {
      const numbered: NumberedLine = { num: i + 1, text: line };
      if (currentSub !== undefined) currentSub.lines.push(numbered);
      else leadLines.push(numbered);
    }
  }
  flushCategory();
  return failures;
}

/** Lists sorted `evals/skills/*.md` bank paths (excluding `README.md`), or `[]` when `evals/skills` does not exist. */
function bankFiles(repoRoot: string): string[] {
  let banksRoot: string;
  try {
    banksRoot = checkedRepoPath(repoRoot, BANKS_DIR);
    if (!statSync(banksRoot).isDirectory()) return [];
  } catch (error) {
    if (isFileErrorOfKind(error, "missing")) return [];
    throw auditFileError("checking", BANKS_DIR, error);
  }
  let entries: string[];
  try {
    entries = readdirSync(banksRoot);
  } catch (error) {
    throw auditFileError("reading", BANKS_DIR, error);
  }
  return entries
    .filter((name) => name.endsWith(".md") && name !== "README.md")
    .sort()
    .map((name) => `${BANKS_DIR}/${name}`);
}

/** Checks every scenario bank under `evals/skills/` for fixture completeness against its criteria and assertions. */
export function bankFixtureCompletenessRules(repoRoot: string): Failure[] {
  const failures: Failure[] = [];
  for (const rel of bankFiles(repoRoot)) {
    let text: string;
    try {
      text = readRepoFile(repoRoot, rel);
    } catch (error) {
      throw auditFileError("reading", rel, error);
    }
    failures.push(...auditBankText(rel, text));
  }
  return failures;
}

export const FAMILY: Family = {
  name: BANK_FIXTURE_COMPLETENESS_RULE,
  rules: [BANK_FIXTURE_COMPLETENESS_RULE],
  run: (root) => bankFixtureCompletenessRules(root),
};
