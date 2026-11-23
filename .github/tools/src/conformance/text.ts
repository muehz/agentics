/**
 * Text normalisation the conformance rules share. Whitespace is what JavaScript's `\s` matches (and what `trim()`
 * strips), so a rule that flattens or collapses prose agrees with every other JavaScript string operation it makes.
 */

/** The words of `text`, split on runs of `\s` with empty ends dropped, joined by one space. */
export function oneLine(text: string): string {
  return text.trim().split(/\s+/).join(" ");
}

/** Replaces every CR LF pair with LF and leaves a lone CR alone. */
export function crlfToLf(text: string): string {
  return text.replaceAll("\r\n", "\n");
}

/**
 * The lines between the first two bare `---` lines, the first being line 1 of the text after CRLF becomes LF. `closed`
 * says whether the closing line was found. An unterminated block is every line after the opener. A byte order mark, a
 * trailing space or a lone CR on the opener means no block: the block is empty and not closed.
 */
export function frontmatterBlock(text: string): {
  block: string;
  closed: boolean;
} {
  const lines = crlfToLf(text).split("\n");
  if (lines[0] !== "---") return { block: "", closed: false };
  const body = lines.slice(1);
  const end = body.indexOf("---");
  if (end === -1) return { block: body.join("\n"), closed: false };
  return { block: body.slice(0, end).join("\n"), closed: true };
}

/** Collapses every run of `\s` to one space, so prose a formatter wrapped across lines still matches. */
export function collapseWhitespace(text: string): string {
  return text.replace(/\s+/g, " ");
}

const OTHER_SYMBOL = /^\p{So}$/u;

/**
 * Whether the code point's Unicode general category is So (Other Symbol). A lone surrogate is category Cs, so it is
 * not a symbol; a value out of range is not one either, and is rejected first because String.fromCodePoint throws on it.
 */
export function isOtherSymbol(codePoint: number): boolean {
  if (codePoint < 0 || codePoint > 0x10ffff) return false;
  return OTHER_SYMBOL.test(String.fromCodePoint(codePoint));
}
