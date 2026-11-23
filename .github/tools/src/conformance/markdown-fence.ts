/**
 * The container-aware fenced-code tracker the skills conformance rules use to skip fenced Markdown. The eval engine in
 * muehz/evals keeps copies of the same tracker in `evals/src/fence/state.ts` and `evals/src/fence/container.ts`, meant
 * to change together with this one. This file cannot import them because the engine lives in that separate repository.
 */

/**
 * Tracks fenced-code-block state across lines. A fence line has at most three leading spaces of indentation followed
 * by a run of three or more identical backticks or tildes; it opens when no fence is open (except a backtick run whose
 * info string holds a backtick, which CommonMark does not treat as a fence) and closes only on the same character with a
 * run at least as long as the opener and a blank info string. A new tracker is outside any fence.
 */
export class FenceState {
  private char = "";
  private length = 0;

  /** Whether a fence is currently open. */
  open(): boolean {
    return this.length > 0;
  }

  /**
   * Reports whether `line` is fence syntax (the marker itself) or content inside an open fence; either way the caller
   * skips the line. Indentation counts spaces only, so a tab-indented marker is still treated as a fence, and the
   * marker is read after `trim()`, so a trailing CR or a U+00A0 around it is stripped.
   */
  step(line: string): boolean {
    let indent = 0;
    while (line[indent] === " ") indent++;
    const trimmed = line.trim();
    if (
      indent <= 3 &&
      trimmed.length >= 3 &&
      (trimmed[0] === "`" || trimmed[0] === "~")
    ) {
      const char = trimmed[0];
      let length = 0;
      while (trimmed[length] === char) length++;
      if (length >= 3) {
        const info = trimmed.slice(length).trim();
        if (this.length === 0 && !(char === "`" && info.includes("`"))) {
          this.char = char;
          this.length = length;
          return true;
        }
        if (
          this.length > 0 &&
          char === this.char &&
          length >= this.length &&
          info === ""
        ) {
          this.length = 0;
          return true;
        }
      }
    }
    return this.open();
  }
}

/** A list item marker at the start of the text, then one to four spaces or the end of the text. */
const LIST_MARKER = /^(?:[-*+]|[0-9]{1,9}[.)])(?: {1,4}|$)/;

/**
 * Tracks fence state for Markdown whose fences may sit in list items or blockquotes. A fence's indentation is measured
 * from its container's content column, so the tracker follows the open list items and strips blockquote markers, and a
 * marker opens a fence only within three columns of that column. A marker four or more columns in is an indented code
 * line, not a fence. A tab advances to the next multiple of four columns. A fence opened inside a quote ends with the
 * quote, closer or not. A line at a list item's indentation or less ends that item and its fence, so a lazy
 * continuation line closes the list early. A new tracker is outside any fence.
 */
export class FenceContainer {
  private fence = new FenceState();
  private fenceQuoteDepth = 0;
  /** Content column the open fence is measured from. */
  private base = 0;
  /** Content columns of the open list items, innermost last. */
  private lists: number[] = [];
  /** Blockquote depth of the previous line. */
  private quoteDepth = 0;

  /** Reports whether `line` is fence syntax or fenced content. */
  step(line: string): boolean {
    const maxDepth = this.fence.open() ? this.fenceQuoteDepth : -1;
    let [body, quoteDepth] = unquote(line, maxDepth);
    if (this.fenceQuoteDepth > quoteDepth) {
      this.fence = new FenceState();
      this.fenceQuoteDepth = 0;
    }
    body = expandTabs(body);
    let indent = spaceIndent(body);
    const blank = body.trim() === "";
    if (this.fence.open()) {
      if (!blank && indent < this.base) {
        this.fence = new FenceState();
        [body, quoteDepth] = unquote(line, -1);
        body = expandTabs(body);
        indent = spaceIndent(body);
      } else {
        this.fence.step(dedent(body, this.base));
        return true;
      }
    }
    if (quoteDepth !== this.quoteDepth) this.lists = [];
    this.quoteDepth = quoteDepth;
    if (blank) return false;

    while (
      this.lists.length > 0 &&
      indent < this.lists[this.lists.length - 1]!
    ) {
      this.lists.pop();
    }
    let base = this.lists.length > 0 ? this.lists[this.lists.length - 1]! : 0;
    const rel = body.slice(base);
    if (this.fence.step(rel)) {
      this.base = base;
      this.fenceQuoteDepth = quoteDepth;
      return true;
    }
    const offset = indent - base;
    if (offset <= 3) {
      const marker = LIST_MARKER.exec(rel.slice(offset))?.[0];
      if (marker) {
        base = indent + marker.length;
        this.lists.push(base);
        if (this.fence.step(body.slice(base))) {
          this.base = base;
          this.fenceQuoteDepth = quoteDepth;
          return true;
        }
      }
    }
    return false;
  }
}

/**
 * Removes up to `maxDepth` leading blockquote markers, each with one optional space; a negative limit removes all.
 * Markers beyond an open fence's quote depth are literal code, including any spaces before them.
 */
function unquote(line: string, maxDepth: number): [string, number] {
  if (maxDepth === 0) return [line, 0];
  let rest = line.replace(/^[ \t]+/, "");
  let depth = 0;
  while (rest.startsWith(">")) {
    depth++;
    rest = rest.slice(1);
    if (rest.startsWith(" ")) rest = rest.slice(1);
    if (depth === maxDepth) break;
    const indent = spaceIndent(rest);
    if (indent <= 3 && rest[indent] === ">") rest = rest.slice(indent);
  }
  return depth === 0 ? [line, 0] : [rest, depth];
}

/** Replaces leading tabs with spaces to the next multiple of four columns; only indentation decides fences. */
function expandTabs(line: string): string {
  let col = 0;
  let index = 0;
  for (; index < line.length; index++) {
    if (line[index] === " ") col++;
    else if (line[index] === "\t") col += 4 - (col % 4);
    else break;
  }
  return " ".repeat(col) + line.slice(index);
}

function spaceIndent(line: string): number {
  let indent = 0;
  while (line[indent] === " ") indent++;
  return indent;
}

/** Removes up to `count` leading spaces. */
function dedent(line: string, count: number): string {
  return line.slice(Math.min(count, spaceIndent(line)));
}
