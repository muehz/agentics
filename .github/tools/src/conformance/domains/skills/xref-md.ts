import { crlfToLf, frontmatterBlock } from "../../text.ts";
import { walkSkillText } from "./files.ts";
import type { SkillsTreeModel } from "./tree-model.ts";

/** A line of skill prose: its 1-based line number in the original file and its text after CRLF becomes LF. */
export interface ProseLine {
  number: number;
  text: string;
}

/** A Markdown file under `skills/<skill>/`, with its text as read and whether the cross-reference checks scan it. */
export interface SkillDoc {
  rel: string;
  skill: string;
  text: string;
  scanned: boolean;
}

// Any indent opens a fence, which is looser than CommonMark: a fence nested in a list item is indented four or more
// spaces, and its lines must still be dropped. The cost is that an indented code block whose first line starts with a
// fence marker also opens a fence.
const FENCE_OPEN = /^\s*(`{3,}|~{3,})/;

// The `>` markers of a blockquote, nested or not, come before a fence that sits inside the quote.
const QUOTE = /^(?:\s*>)+/;

const HEADING = /^ {0,3}#{1,6}[ \t]+(.*?)(?:[ \t]+#+)?[ \t]*$/;

const BOLD_LEAD = /^\s*(?:[-*+][ \t]+|\d+\.[ \t]+)?\*\*(.+?)\*\*/;

const SCANNED = /^skills\/[^/]+\/(?:SKILL\.md|references\/.+\.md)$/;

/** The number of leading lines that frontmatter takes: none without an opener, every line when it is not closed. */
function frontmatterLength(lines: string[], text: string): number {
  if (lines[0] !== "---") return 0;
  if (!frontmatterBlock(text).closed) return lines.length;
  return lines.indexOf("---", 1) + 1;
}

/**
 * The prose lines of a Markdown file: CRLF becomes LF, then the frontmatter and every fenced code block are dropped.
 * A fence closes on a line whose trimmed text is only a run of the opening character at least as long as the opener;
 * the fence lines go with their content. An unclosed fence drops the rest of the file. A fence that opens after
 * blockquote markers reads its closer after the same kind of markers, and it also closes on the first line without a
 * marker, which ends the quote; that line is prose.
 */
export function proseLines(text: string): ProseLine[] {
  const lf = crlfToLf(text);
  const lines = lf.split("\n");
  const out: ProseLine[] = [];
  let fence: string | undefined;
  let quoted = false;
  for (let i = frontmatterLength(lines, lf); i < lines.length; i++) {
    const line = lines[i] ?? "";
    const quote = QUOTE.exec(line);
    const body = quote ? line.slice(quote[0].length) : line;
    if (fence !== undefined) {
      if (!quoted || quote) {
        const trimmed = (quoted ? body : line).trim();
        const char = fence.charAt(0);
        if (
          trimmed.length >= fence.length &&
          [...trimmed].every((c) => c === char)
        )
          fence = undefined;
        continue;
      }
      fence = undefined;
    }
    const open = FENCE_OPEN.exec(body);
    if (open) {
      fence = open[1];
      quoted = quote !== null;
      continue;
    }
    out.push({ number: i + 1, text: line });
  }
  return out;
}

/** The heading on a line already selected as prose, without its opening or closing hashes. */
export function headingText(line: string): string | undefined {
  return HEADING.exec(line)?.[1];
}

/** The text of each ATX heading in the prose lines, without the hashes or a closing hash run, in document order. */
export function headings(text: string, lines = proseLines): string[] {
  const out: string[] = [];
  for (const { text: line } of lines(text)) {
    const heading = headingText(line);
    if (heading !== undefined) out.push(heading);
  }
  return out;
}

/**
 * The GitHub anchor slug of a heading: backticks and asterisks go, the text is lowercased, every character other than a
 * letter, a digit, `-`, `_` or a space goes, and each space becomes `-`. Known limit: a heading that contains a
 * Markdown link is slugged from its raw text, so the URL and brackets are not removed the way GitHub removes them.
 */
export function githubSlug(heading: string): string {
  return heading
    .replace(/[`*]/g, "")
    .toLowerCase()
    .replace(/[^\p{L}\p{N}\-_ ]/gu, "")
    .replace(/ /g, "-");
}

/** The anchors GitHub gives a file's headings: the first use of a slug is plain, repeats get `-1`, `-2`, … in order. */
export function anchors(text: string, lines = proseLines): Set<string> {
  const out = new Set<string>();
  const seen = new Map<string, number>();
  for (const heading of headings(text, lines)) {
    const slug = githubSlug(heading);
    const count = seen.get(slug) ?? 0;
    seen.set(slug, count + 1);
    out.add(count === 0 ? slug : `${slug}-${count}`);
  }
  return out;
}

/** The text inside a `**…**` that starts a prose line or follows a list marker, in document order. */
export function boldLeads(text: string, lines = proseLines): string[] {
  const out: string[] = [];
  for (const { text: line } of lines(text)) {
    const match = BOLD_LEAD.exec(line);
    if (match) out.push(match[1] ?? "");
  }
  return out;
}

/**
 * Every Markdown file at least one directory deep under `skills/`, keyed by its repo-relative path in walk order. The
 * skill is the directory under `skills/`. `SKILL.md` and files under `references/` are scanned; other Markdown files,
 * such as assets, are kept only as link targets. An error from the walk propagates.
 */
export function skillDocs(
  repoRoot: string,
  tree?: Pick<SkillsTreeModel, "walkText">,
): Map<string, SkillDoc> {
  const out = new Map<string, SkillDoc>();
  const select = (rel: string, text: string) => {
    if (!rel.endsWith(".md")) return;
    const parts = rel.split("/");
    const skill = parts[1];
    if (parts.length < 3 || skill === undefined) return;
    out.set(rel, { rel, skill, text, scanned: SCANNED.test(rel) });
  };
  if (tree) tree.walkText("skill standards", select);
  else walkSkillText(repoRoot, "skill standards", select);
  return out;
}
