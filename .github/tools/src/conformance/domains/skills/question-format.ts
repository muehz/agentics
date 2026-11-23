import path from "node:path";
import type { Failure } from "../../check.ts";
import { oneLine } from "../../text.ts";
import { auditFileError } from "./files.ts";
import type { Family } from "./index.ts";
import { policyLinks } from "./shared-contracts.ts";
import { headings, headingText } from "./xref-md.ts";
import { SkillsTreeModel } from "./tree-model.ts";

const RULE = "question-format";
const CONTRACT = "skills/routing/references/contracts/question-format.md";
const MARKER = "Format each question exactly like so:";
const ANSWERS_HEADING = "Collecting answers";

/**
 * Sentences the Collecting answers section must keep: a round in the visible chat can be reread after the ask dialog
 * closes and copied for someone else to answer, which an ask dialog alone does not allow.
 */
const POSTED_ROUND_PINS: readonly string[] = [
  "posted in the visible chat first, as one message, before any ask-tool call",
  "The whole round sits in one blockquote",
];

/** The fixed roster still requires a link after the inline-copy option has been removed. */
const REQUIRED = new Set(
  [
    "challenge",
    "deepen",
    "docs",
    "execute",
    "forge",
    "gate",
    "gh",
    "git",
    "hillclimb",
    "ops",
    "prototype",
    "review",
    "scaffold",
    "triage",
  ].map((skill) => `skills/${skill}/SKILL.md`),
);

const TITLE = /<question title>[^\n]*<question body\b[^>]*>/i;
const RECOMMENDATION = /<your recommended answer>/i;

function hasTemplate(text: string): boolean {
  const lines = text.split(/\r?\n/);
  return lines.some(
    (line, index) =>
      TITLE.test(line) &&
      lines
        .slice(index + 1, index + 5)
        .some((next) => RECOMMENDATION.test(next)),
  );
}

function isLocalCopy(text: string, prose: string): boolean {
  // Filled examples can quote real questions. The paired placeholders identify a reusable template instead.
  if (hasTemplate(text)) return true;
  return (
    /\bdecision questions\.\s+to ask the user to pick an option or set a direction\b/i.test(
      prose.replace(/[*_]/g, ""),
    ) || /\bformat each question exactly like so\s*:/i.test(prose)
  );
}

function linksToContract(rel: string, prose: string): boolean {
  return policyLinks(prose).some(
    (target) =>
      !target.split("/").includes("") &&
      path.posix.join(path.posix.dirname(rel), target) === CONTRACT,
  );
}

function readContract(tree: SkillsTreeModel): string {
  try {
    return tree.read(CONTRACT);
  } catch (error) {
    throw auditFileError("reading", CONTRACT, error);
  }
}

function checkContract(tree: SkillsTreeModel): Failure[] {
  const { visible: text } = tree.questionProse(readContract(tree));
  const marker = tree.lines(text).find(({ text }) => text.trim() === MARKER);
  // Keep the canonical example directly beneath its marker, inside its own closed fence.
  const afterMarker = marker ? text.split(/\r?\n/).slice(marker.number) : [];
  const start = afterMarker.findIndex((line) => line.trim() !== "");
  const fence = /^[ \t]*(`{3,})[^`]*$/.exec(afterMarker[start] ?? "")?.[1];
  const end =
    fence &&
    afterMarker.findIndex(
      (line, index) =>
        index > start &&
        line.trimStart().startsWith(fence) &&
        /^`+$/.test(line.trim()),
    );
  const body =
    typeof end === "number" &&
    end > start &&
    afterMarker.slice(start + 1, end).join("\n");
  if (!body || !hasTemplate(body))
    throw new Error(
      `skills: ${CONTRACT}: no fenced block found after "${MARKER}"`,
    );
  if (!headings(text, tree.lines).includes(ANSWERS_HEADING))
    return [
      {
        rule: RULE,
        path: CONTRACT,
        message:
          'must carry a "## Collecting answers" section outside any code fence: skills link to this file for how answers are collected',
      },
    ];
  const section = oneLine(answersSection(text, tree));
  return POSTED_ROUND_PINS.filter(
    (literal) => !section.includes(oneLine(literal)),
  ).map((literal) => ({
    rule: RULE,
    path: CONTRACT,
    message: `"## ${ANSWERS_HEADING}" must carry the sentence ${JSON.stringify(literal)}: skills link to this file for how a round is posted`,
  }));
}

/** The prose from the Collecting answers heading to the next heading, with fenced and blockquoted lines gone. */
function answersSection(text: string, tree: SkillsTreeModel): string {
  const lines = tree.lines(text);
  const start = lines.findIndex(
    ({ text }) => headingText(text) === ANSWERS_HEADING,
  );
  const rest = lines.slice(start + 1);
  const end = rest.findIndex(({ text }) => headingText(text) !== undefined);
  return (end < 0 ? rest : rest.slice(0, end))
    .map(({ text }) => text)
    .join("\n");
}

export function questionFormatRules(
  root: string,
  tree = new SkillsTreeModel(root),
): Failure[] {
  const failures = checkContract(tree);
  for (const { rel, text, scanned } of tree.docs().values()) {
    if (!scanned || rel === CONTRACT) continue;
    const { visible, prose } = tree.questionProse(text);
    if (isLocalCopy(visible, prose)) {
      failures.push({
        rule: RULE,
        path: rel,
        message: `replace the local decision-question format with a link to ${CONTRACT}`,
      });
    } else if (REQUIRED.has(rel) && !linksToContract(rel, prose)) {
      failures.push({
        rule: RULE,
        path: rel,
        message: `must link to ${CONTRACT}`,
      });
    }
  }
  return failures;
}

export const FAMILY: Family = {
  name: RULE,
  rules: [RULE],
  run: (root, context) => questionFormatRules(root, context?.tree),
};
