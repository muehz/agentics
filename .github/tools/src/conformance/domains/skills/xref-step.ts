import type { KeyedFailure } from "./baseline.ts";
import { skillNamePattern } from "./tree-model.ts";
import { boldLeads, headings, proseLines, type SkillDoc } from "./xref-md.ts";

/** The rule that reports a reference to a skill's step or named rule that the skill does not declare. */
export const STEP_RULE = "skill-xref-step";

const STEP_NUMBER = String.raw`\d+(?:\.\d+)?[a-z]?`;

// A skill name that a word character, a slash, a backtick or a hyphen comes before is part of a path, a command such
// as `/gate`, a longer name, or a backticked form that its own pattern reads.
const FREE = String.raw`(?<![\w/\`-])`;

/** What a reference points at: a numbered step or a named step or rule. */
type Target = { step: string } | { name: string };

interface Reference {
  index: number;
  text: string;
  skill: string;
  target: Target;
}

function escapeRegExp(text: string): string {
  return text.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

/** The reference patterns for the given name alternation; each names its `skill` and either `step` or `name` group. */
function patterns(names: string): RegExp[] {
  const step = `Step (?<step>${STEP_NUMBER})`;
  const named = String.raw`\*\*(?<name>[^*]+?)\*\* (?:step|rule)\b`;
  return [
    // Backticked: `skill` Step N
    new RegExp(`\`(?<skill>${names})\` ${step}`, "g"),
    // Bare: skill Step N
    new RegExp(`${FREE}(?<skill>${names}) ${step}`, "g"),
    // Step N of `skill`
    new RegExp(`${step} of \`(?<skill>${names})\``, "g"),
    // Step N of `skills/skill/path.md`
    new RegExp(`${step} of \`skills/(?<skill>${names})/[^\`]+\\.md\``, "g"),
    // **Name** step or rule in `skills/skill/path.md`
    new RegExp(`${named} in \`skills/(?<skill>${names})/[^\`]+\\.md\``, "g"),
    // Plain possessive: skill's **Name** step or rule
    new RegExp(`${FREE}(?<skill>${names})'s ${named}`, "g"),
    // Backticked possessive: `skill`'s **Name** step or rule
    new RegExp(`\`(?<skill>${names})\`'s ${named}`, "g"),
    // Backticked possessive: `skill`'s Step N
    new RegExp(`\`(?<skill>${names})\`'s ${step}`, "g"),
  ];
}

/** Every reference on one line, in the order it appears. */
function references(line: string, regexps: readonly RegExp[]): Reference[] {
  const out: Reference[] = [];
  for (const regexp of regexps) {
    for (const match of line.matchAll(regexp)) {
      const groups = match.groups ?? {};
      const skill = groups["skill"] ?? "";
      const step = groups["step"];
      const name = groups["name"];
      const target: Target =
        step !== undefined ? { step } : { name: name ?? "" };
      out.push({ index: match.index, text: match[0], skill, target });
    }
  }
  return out.sort((a, b) => a.index - b.index);
}

/** The headings and bold lead-ins of every doc of the skill, each with one trailing `.` or `:` removed. */
function skillLabels(
  docs: ReadonlyMap<string, SkillDoc>,
  skill: string,
  lines: typeof proseLines,
): string[] {
  const out: string[] = [];
  for (const doc of docs.values()) {
    if (doc.skill !== skill) continue;
    for (const label of [
      ...headings(doc.text, lines),
      ...boldLeads(doc.text, lines),
    ])
      out.push(label.replace(/[.:]$/, ""));
  }
  return out;
}

/**
 * Whether a label declares the target: by `Step N`, by a leading `N.` or `N:`, or for a name by containing it. A `.`
 * after `Step N` ends the number unless a digit follows it, so `Step 0. Preflight` declares Step 0 but `Step 2.45` does
 * not declare Step 2.4 or Step 2.
 */
function declares(label: string, target: Target): boolean {
  if ("name" in target)
    return label.toLowerCase().includes(target.name.toLowerCase());
  const number = escapeRegExp(target.step);
  return (
    new RegExp(`(?:^|\\W)Step ${number}(?!\\w|\\.\\d)`, "i").test(label) ||
    new RegExp(`^${number}[.:](?:\\s|$)`).test(label)
  );
}

function describe(target: Target): string {
  return "step" in target ? `Step ${target.step}` : `"${target.name}"`;
}

/**
 * Findings for each reference, in a scanned doc's prose, to a step or named rule of a skill in `skills` that no
 * heading or bold lead-in of that skill declares. One finding per occurrence; its key is the matched reference text.
 */
export function stepFindings(
  docs: ReadonlyMap<string, SkillDoc>,
  skills: ReadonlySet<string>,
  names = skillNamePattern(skills),
  lines = proseLines,
): KeyedFailure[] {
  if (skills.size === 0) return [];
  const regexps = patterns(names);
  const labels = new Map<string, string[]>();
  const out: KeyedFailure[] = [];
  for (const source of docs.values()) {
    if (!source.scanned) continue;
    for (const { number, text } of lines(source.text)) {
      for (const ref of references(text, regexps)) {
        let known = labels.get(ref.skill);
        if (known === undefined) {
          known = skillLabels(docs, ref.skill, lines);
          labels.set(ref.skill, known);
        }
        if (known.some((label) => declares(label, ref.target))) continue;
        out.push({
          rule: STEP_RULE,
          path: source.rel,
          key: ref.text,
          message: `line ${number}: ${ref.text}: ${ref.skill} has no heading or bold lead-in for ${describe(ref.target)}`,
        });
      }
    }
  }
  return out;
}
