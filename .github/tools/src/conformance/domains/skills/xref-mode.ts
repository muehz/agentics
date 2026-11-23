import type { KeyedFailure } from "./baseline.ts";
import { skillNamePattern } from "./tree-model.ts";
import type { SkillDeclaration } from "./skill-meta.ts";
import { proseLines, type SkillDoc } from "./xref-md.ts";

/** The rule that reports a `/agentics:skill mode` reference whose mode the skill does not declare. */
export const MODE_RULE = "skill-xref-mode";

/** The rule that reports a flag in a `` `/agentics:skill …` `` span that the skill does not declare. */
export const FLAG_RULE = "skill-xref-flag";

const FLAG = /--[a-z][a-z0-9-]*/g;

/** Whether a word names one of the skill's modes or aliases. */
function declaresMode(declaration: SkillDeclaration, word: string): boolean {
  return declaration.modes?.has(word) === true || declaration.aliases.has(word);
}

/**
 * The mode and flag references in scanned skill prose that the target skill does not declare, one finding per
 * occurrence. A mode reference is `/agentics:skill word`, or the Codex form `$agentics:skill word`, after whitespace, a
 * backtick or an open parenthesis. A flag is read only inside a backticked span that starts with either form. A name
 * without a declaration is not a skill, so a reference to it is not read. A bare `/skill` is the bare-command rule's
 * finding, not a reference this rule reads. Keys and messages spell both forms as `/agentics:skill`.
 */
export function modeFindings(
  docs: ReadonlyMap<string, SkillDoc>,
  declarations: ReadonlyMap<string, SkillDeclaration>,
  names = skillNamePattern(declarations.keys()),
  lines = proseLines,
): KeyedFailure[] {
  const mode = new RegExp(
    `(?<![^\\s\`(])[/$]agentics:(${names})[ \\t]+([a-z][a-z0-9-]*)`,
    "g",
  );
  const span = new RegExp(`\`[/$]agentics:(${names})(?![\\w-])([^\`]*)\``, "g");
  const out: KeyedFailure[] = [];
  for (const doc of docs.values()) {
    if (!doc.scanned) continue;
    for (const { number, text } of lines(doc.text)) {
      for (const match of text.matchAll(mode)) {
        const skill = match[1] ?? "";
        const word = match[2] ?? "";
        const declaration = declarations.get(skill);
        if (declaration === undefined || declaresMode(declaration, word))
          continue;
        const reason =
          declaration.modes === undefined
            ? `${skill} declares no metadata.modes`
            : `${word} is not in ${skill}'s metadata.modes or metadata.mode-aliases`;
        out.push({
          rule: MODE_RULE,
          path: doc.rel,
          key: `/agentics:${skill} ${word}`,
          message: `line ${number}: /agentics:${skill} ${word}: ${reason}`,
        });
      }
      for (const match of text.matchAll(span)) {
        const skill = match[1] ?? "";
        const declaration = declarations.get(skill);
        if (declaration === undefined) continue;
        for (const [flag] of (match[2] ?? "").matchAll(FLAG)) {
          if (declaration.flags.has(flag)) continue;
          out.push({
            rule: FLAG_RULE,
            path: doc.rel,
            key: `/agentics:${skill} ${flag}`,
            message: `line ${number}: ${flag} is not in ${skill}'s metadata.flags`,
          });
        }
      }
    }
  }
  return out;
}
