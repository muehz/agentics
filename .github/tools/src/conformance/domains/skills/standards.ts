import {
  applyBaseline,
  BASELINE_STALE_RULE,
  type KeyedFailure,
  readBaseline,
} from "./baseline.ts";
import { DUPLICATE_RULE, duplicateFindings } from "./duplication.ts";
import {
  frontmatterFindings,
  KEY_RULE,
  VALUE_RULE,
} from "./frontmatter-std.ts";
import type { Family } from "./index.ts";
import { METADATA_RULE, readDeclarations } from "./skill-meta.ts";
import {
  ARGUMENT_HINT_RULE,
  DESCRIPTION_LENGTH_RULE,
  NAME_FORMAT_RULE,
  REFERENCES_INDEX_RULE,
  ROUTING_SECTION_RULE,
  structureFindings,
  SUMMARY_BLOCK_RULE,
} from "./structure.ts";
import { ANCHOR_RULE, anchorFindings } from "./xref-anchor.ts";
import {
  BARE_COMMAND_RULE,
  CODEX_FORM_RULE,
  commandFormFindings,
} from "./xref-form.ts";
import { SkillsTreeModel, skillNamePattern } from "./tree-model.ts";
import { FLAG_RULE, MODE_RULE, modeFindings } from "./xref-mode.ts";
import { STEP_RULE, stepFindings } from "./xref-step.ts";

/** The rule ids the standards family reports, in the order its checks run. */
export const STANDARDS_RULES = [
  ANCHOR_RULE,
  MODE_RULE,
  FLAG_RULE,
  BARE_COMMAND_RULE,
  CODEX_FORM_RULE,
  STEP_RULE,
  KEY_RULE,
  VALUE_RULE,
  METADATA_RULE,
  DUPLICATE_RULE,
  SUMMARY_BLOCK_RULE,
  REFERENCES_INDEX_RULE,
  ROUTING_SECTION_RULE,
  NAME_FORMAT_RULE,
  DESCRIPTION_LENGTH_RULE,
  ARGUMENT_HINT_RULE,
  BASELINE_STALE_RULE,
] as const;

/** The rules a baseline entry may name. A stale entry is never itself baselined. */
export const BASELINED_RULES: ReadonlySet<string> = new Set(
  STANDARDS_RULES.filter((rule) => rule !== BASELINE_STALE_RULE),
);

/**
 * Every finding of the standards rules before the baseline applies. Findings print in this order: metadata,
 * frontmatter key and value, anchor, mode and flag, bare command and Codex form, step, duplicate block, then the structure rules (line limit,
 * summary block, references index and routing section for each `SKILL.md`, then name format, description length and
 * argument hint for each skill). The baseline writer calls it too, so the baseline accepts exactly what the family
 * would report.
 */
export function collectStandardFindings(
  repoRoot: string,
  tree = new SkillsTreeModel(repoRoot),
): KeyedFailure[] {
  const { declarations, findings } = readDeclarations(repoRoot, tree);
  const docs = tree.docs();
  const skills = new Set(declarations.keys());
  const names = skillNamePattern(skills);
  return [
    ...findings,
    ...frontmatterFindings(declarations.values()),
    ...anchorFindings(docs, tree.lines),
    ...modeFindings(docs, declarations, names, tree.lines),
    ...commandFormFindings(docs, skills, names, tree.lines),
    ...stepFindings(docs, skills, names, tree.lines),
    ...duplicateFindings(docs, tree.lines),
    ...structureFindings(docs, declarations, tree.lines),
  ];
}

/**
 * The skill standards family: the cross-reference, frontmatter, metadata, duplication and structure rules, with the
 * committed baseline applied. `FAMILIES` (the core families) leaves it out and `DOMAIN_FAMILIES` runs it
 * before `eval-check`.
 */
export const STANDARDS_FAMILY: Family = {
  name: "skill-standards",
  rules: STANDARDS_RULES,
  run: (root, context) =>
    applyBaseline(
      collectStandardFindings(root, context?.tree),
      readBaseline(root, BASELINED_RULES),
    ),
};
