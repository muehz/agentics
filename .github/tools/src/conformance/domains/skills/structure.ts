import type { KeyedFailure } from "./baseline.ts";
import { lookup, printable, render, stringText } from "./frontmatter.ts";
import type { SkillDeclaration } from "./skill-meta.ts";
import { proseLines, type SkillDoc } from "./xref-md.ts";

/** The rule name for a `Capability summary` heading, at any level and in any case, in a `SKILL.md`. */
export const SUMMARY_BLOCK_RULE = "skill-summary-block";

/** The rule name for a `SKILL.md` without exactly one `## References` heading as its last level-two heading. */
export const REFERENCES_INDEX_RULE = "skill-references-index";

/** The rule name for a `SKILL.md` without a `## Don't use when` heading. */
export const ROUTING_SECTION_RULE = "skill-routing-section";

/**
 * The rule name for a frontmatter `name` that is not lowercase kebab-case, exceeds 64 characters or differs from the
 * skill's directory name.
 */
export const NAME_FORMAT_RULE = "skill-name-format";

/** The rule name for a frontmatter `description` that is not a string of at most 1,024 characters. */
export const DESCRIPTION_LENGTH_RULE = "skill-description-length";

/**
 * The rule name for a declared mode, mode alias or flag that the frontmatter `argument-hint` does not name as a whole
 * word.
 */
export const ARGUMENT_HINT_RULE = "skill-argument-hint";

// The limits that skill hosts enforce when they load a skill's frontmatter.
const MAX_NAME = 64;
const MAX_DESCRIPTION = 1024;

const NAME_SHAPE = /^[a-z0-9]+(?:-[a-z0-9]+)*$/;

// Hyphens stay inside a token so a flag keeps its leading `--` and an alias does not match part of a hyphenated word.
const HINT_SEPARATOR = /[^a-z0-9-]+/;

const SKILL_FILE = /^skills\/[^/]+\/SKILL\.md$/;

// The same ATX shape that `headings()` in xref-md.ts reads, with the hash run captured because these rules need the level.
const HEADING = /^ {0,3}(#{1,6})[ \t]+(.*?)(?:[ \t]+#+)?[ \t]*$/;

const SUMMARY = "capability summary";
const REFERENCES = "References";
const ROUTING = "Don't use when";

interface Heading {
  level: number;
  text: string;
}

function headingsWithLevel(text: string, lines: typeof proseLines): Heading[] {
  const out: Heading[] = [];
  for (const { text: line } of lines(text)) {
    const match = HEADING.exec(line);
    if (match) out.push({ level: match[1]?.length ?? 0, text: match[2] ?? "" });
  }
  return out;
}

function bodyFindings(
  rel: string,
  text: string,
  lines: typeof proseLines,
): KeyedFailure[] {
  const out: KeyedFailure[] = [];
  const heads = headingsWithLevel(text, lines);
  // A baseline entry accepts every finding with its key, so a repeat of the same heading text gets its occurrence
  // number in the key; otherwise one accepted heading would hide each later copy.
  const seen = new Map<string, number>();
  for (const h of heads) {
    if (h.text.toLowerCase() !== SUMMARY) continue;
    const n = (seen.get(h.text) ?? 0) + 1;
    seen.set(h.text, n);
    out.push({
      rule: SUMMARY_BLOCK_RULE,
      path: rel,
      key: n === 1 ? h.text : `${h.text} (${n})`,
      message: `heading "${h.text}" at level ${h.level}; expected no Capability summary heading at any level`,
    });
  }
  const second = heads.filter((h) => h.level === 2);
  const indexes = second.filter((h) => h.text === REFERENCES).length;
  const last = second.at(-1)?.text;
  if (indexes !== 1 || last !== REFERENCES) {
    out.push({
      rule: REFERENCES_INDEX_RULE,
      path: rel,
      key: "references",
      message: `found ${indexes} "## ${REFERENCES}" heading(s) and the last level-two heading is ${last === undefined ? "missing" : `"## ${last}"`}; expected exactly one "## ${REFERENCES}" as the last level-two heading`,
    });
  }
  if (!second.some((h) => h.text === ROUTING)) {
    out.push({
      rule: ROUTING_SECTION_RULE,
      path: rel,
      key: "routing",
      message: `found no "## ${ROUTING}" heading; expected one that names the requests this skill does not handle`,
    });
  }
  return out;
}

function frontmatterFindings(declaration: SkillDeclaration): KeyedFailure[] {
  const { frontmatter, rel, skill } = declaration;
  // The frontmatter rules already report a block that is missing, unclosed or not a mapping.
  if (frontmatter === undefined) return [];
  const out: KeyedFailure[] = [];

  const name = lookup(frontmatter, "name");
  if (name !== undefined) {
    const value = stringText(name);
    const problems: string[] = [];
    if (value === undefined) {
      problems.push("is not a string");
    } else {
      const length = [...value].length;
      if (!NAME_SHAPE.test(value)) problems.push("is not lowercase kebab-case");
      if (length > MAX_NAME)
        problems.push(`has ${length} characters; the limit is ${MAX_NAME}`);
      if (value !== skill)
        problems.push(`differs from the directory name "${skill}"`);
    }
    if (problems.length > 0) {
      const shown =
        value === undefined ? render(name) : `"${printable(value)}"`;
      out.push({
        rule: NAME_FORMAT_RULE,
        path: rel,
        key: "name",
        message: `name ${shown} ${problems.join(", ")}; expected lowercase letters and digits in hyphen-separated groups, at most ${MAX_NAME} characters, equal to the directory name "${skill}"`,
      });
    }
  }

  const description = lookup(frontmatter, "description");
  if (description !== undefined) {
    const value = stringText(description);
    const length = value === undefined ? 0 : [...value].length;
    if (value === undefined || length > MAX_DESCRIPTION) {
      out.push({
        rule: DESCRIPTION_LENGTH_RULE,
        path: rel,
        key: "description",
        message:
          value === undefined
            ? `description is not a string; expected a string of at most ${MAX_DESCRIPTION} characters`
            : `description has ${length} characters; the limit is ${MAX_DESCRIPTION}`,
      });
    }
  }

  const words = [
    ...(declaration.modes ?? []),
    ...declaration.aliases.keys(),
    ...declaration.flags,
  ];
  if (words.length === 0) return out;
  const hint = stringText(lookup(frontmatter, "argument-hint"));
  const tokens = new Set((hint ?? "").split(HINT_SEPARATOR));
  for (const word of words) {
    if (tokens.has(word)) continue;
    out.push({
      rule: ARGUMENT_HINT_RULE,
      path: rel,
      key: word,
      message:
        hint === undefined
          ? `${word} is declared but the skill has no string argument-hint; expected an argument-hint that names ${word} as a whole word`
          : `${word} is declared but argument-hint "${printable(hint)}" does not name it as a whole word; expected ${word} between characters other than lowercase letters, digits and hyphens`,
    });
  }
  return out;
}

/**
 * The structure findings for the skills: first the body rules for every `skills/<name>/SKILL.md` in the documents, in
 * document order, then the frontmatter rules for every declaration, in map order. Other Markdown files under a skill
 * are not checked. Headings are read from the prose lines, so a heading inside a fenced code block counts for none of the
 * rules. The frontmatter rules use the declarations as `readDeclarations` returns them and report nothing for a skill
 * whose frontmatter it could not read.
 */
export function structureFindings(
  docs: ReadonlyMap<string, SkillDoc>,
  declarations: ReadonlyMap<string, SkillDeclaration>,
  lines = proseLines,
): KeyedFailure[] {
  const out: KeyedFailure[] = [];
  for (const [rel, doc] of docs) {
    if (!SKILL_FILE.test(rel)) continue;
    out.push(...bodyFindings(rel, doc.text, lines));
  }
  for (const declaration of declarations.values())
    out.push(...frontmatterFindings(declaration));
  return out;
}
