import type { KeyedFailure } from "./baseline.ts";
import { skillNamePattern } from "./tree-model.ts";
import { proseLines, type SkillDoc } from "./xref-md.ts";

/** The rule that reports a skill command written without the `agentics:` plugin prefix, such as `/gate`. */
export const BARE_COMMAND_RULE = "skill-command-bare";

/** The rule that reports a `SKILL.md` that prints `/agentics:<skill>` commands but never gives the Codex form. */
export const CODEX_FORM_RULE = "skill-command-codex-form";

/**
 * The bare-command and Codex-form findings for scanned skill docs. A bare command is `/skill` after the start of a
 * line, whitespace, or an opening backtick, parenthesis, bracket, quote, emphasis mark, blockquote marker or table
 * pipe, where the name does not continue as a path, an identifier or a hyphenated word. A bare name can resolve to a
 * built-in or project skill of the same name, so every printed command carries the plugin prefix. Fenced code is not
 * read. A `SKILL.md` that prints `/agentics:<skill>` in prose must also name `$agentics:`, the form Codex accepts, at least
 * once in prose; a `$agentics:` only in frontmatter or fenced code does not tell the reader.
 */
export function commandFormFindings(
  docs: ReadonlyMap<string, SkillDoc>,
  skills: ReadonlySet<string>,
  names = skillNamePattern(skills),
  lines = proseLines,
): KeyedFailure[] {
  if (skills.size === 0) return [];
  const bare = new RegExp(
    `(?<![^\\s\`(\\["'*>|])\\/(${names})(?![\\w/-]|\\.\\w)`,
    "g",
  );
  const qualified = new RegExp(`\\/agentics:(?:${names})(?![\\w-])`);
  const out: KeyedFailure[] = [];
  for (const doc of docs.values()) {
    if (!doc.scanned) continue;
    let printsQualified = false;
    let namesCodexForm = false;
    for (const { number, text } of lines(doc.text)) {
      if (qualified.test(text)) printsQualified = true;
      if (text.includes("$agentics:")) namesCodexForm = true;
      for (const match of text.matchAll(bare)) {
        const skill = match[1] ?? "";
        out.push({
          rule: BARE_COMMAND_RULE,
          path: doc.rel,
          key: `/${skill}`,
          message: `line ${number}: /${skill} is a bare skill command; write /agentics:${skill}`,
        });
      }
    }
    if (printsQualified && doc.rel.endsWith("/SKILL.md") && !namesCodexForm) {
      out.push({
        rule: CODEX_FORM_RULE,
        path: doc.rel,
        key: "codex-form",
        message:
          "SKILL.md prints /agentics:<skill> commands but never names the Codex form $agentics:<skill>",
      });
    }
  }
  return out;
}
