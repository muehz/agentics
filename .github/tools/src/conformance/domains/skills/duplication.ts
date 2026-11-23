import type { KeyedFailure } from "./baseline.ts";
import { proseLines, type SkillDoc } from "./xref-md.ts";

/** The rule name for a paragraph or list item printed more than once in one `SKILL.md`. */
export const DUPLICATE_RULE = "skill-duplicate-block";

/** The fewest words a block needs before a repeat of it is reported; shorter repeats are usually labels or phrases. */
export const MIN_WORDS = 8;

const SKILL_FILE = /^skills\/[^/]+\/SKILL\.md$/;
const HEADING = /^ {0,3}#{1,6}(\s|$)/;
const LIST_ITEM = /^\s*(?:[-*+]|\d+\.)[ \t]+/;
const INDENTED = /^(?: {4}|\t)/;

function normalize(text: string): string {
  return text.replace(/\s+/g, " ").trim();
}

function words(text: string): string[] {
  return text === "" ? [] : text.split(" ");
}

/**
 * The paragraphs and list items of a Markdown file, each with the 1-based line it starts on and its text with white
 * space collapsed. Frontmatter, fenced code, headings, table rows and indented code are not blocks. A blank line, a
 * heading, a table row or a removed fence ends a block; a list item line, nested or not, starts a new one.
 */
export function blocks(
  text: string,
  lines = proseLines,
): { line: number; text: string }[] {
  const out: { line: number; text: string }[] = [];
  let open: { line: number; parts: string[] } | undefined;
  let previous: number | undefined;
  const close = (): void => {
    if (open === undefined) return;
    const joined = normalize(open.parts.join(" "));
    if (joined !== "") out.push({ line: open.line, text: joined });
    open = undefined;
  };
  for (const { number, text: line } of lines(text)) {
    if (previous !== undefined && number !== previous + 1) close();
    previous = number;
    if (
      line.trim() === "" ||
      HEADING.test(line) ||
      line.trim().startsWith("|")
    ) {
      close();
      continue;
    }
    const item = LIST_ITEM.exec(line);
    if (item) {
      close();
      open = { line: number, parts: [line.slice(item[0].length)] };
      continue;
    }
    if (open === undefined && INDENTED.test(line)) continue;
    if (open === undefined) open = { line: number, parts: [line] };
    else open.parts.push(line);
  }
  close();
  return out;
}

/**
 * One finding for each run of repeated blocks in a `skills/<name>/SKILL.md`. Only blocks of `MIN_WORDS` words or more
 * count. A block whose text equals an earlier long block is a repeat, and repeats next to each other in the list of
 * long blocks form one run, so a list printed twice gives one finding. Three copies of one block report as a run
 * printed twice, from the first copy to the second; the third copy starts its own run unless it is next to that one.
 * The key is the first `MIN_WORDS` words of the run's first repeated block.
 */
export function duplicateFindings(
  docs: ReadonlyMap<string, SkillDoc>,
  lines = proseLines,
): KeyedFailure[] {
  const out: KeyedFailure[] = [];
  for (const [rel, doc] of docs) {
    if (!SKILL_FILE.test(rel)) continue;
    const long = blocks(doc.text, lines).filter(
      (b) => words(b.text).length >= MIN_WORDS,
    );
    const first = new Map<string, number>();
    let run: { a: number; b: number; key: string; k: number } | undefined;
    const flush = (): void => {
      if (run === undefined) return;
      out.push({
        rule: DUPLICATE_RULE,
        path: rel,
        key: run.key,
        message: `lines ${run.a} and ${run.b} start a run of ${run.k} block(s) printed twice: "${run.key}"`,
      });
      run = undefined;
    };
    for (const block of long) {
      const earlier = first.get(block.text);
      if (earlier === undefined) {
        first.set(block.text, block.line);
        flush();
        continue;
      }
      if (run === undefined) {
        const key = words(block.text).slice(0, MIN_WORDS).join(" ");
        run = { a: earlier, b: block.line, key, k: 1 };
      } else run.k++;
    }
    flush();
  }
  return out;
}
