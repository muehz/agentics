const SIMPLE_ESCAPES: Readonly<Record<string, string>> = {
  a: "\x07",
  b: "\b",
  f: "\f",
  n: "\n",
  r: "\r",
  t: "\t",
  v: "\v",
  "\\": "\\",
};

const HEX = /^[0-9a-fA-F]+$/;
const OCTAL = /^[0-7]{3}$/;

/** The value of a code point written as `\u` or `\U`, or an error for a surrogate or a value past U+10FFFF. */
function codePoint(digits: string): string {
  const value = Number.parseInt(digits, 16);
  if (value > 0x10ffff || (value >= 0xd800 && value <= 0xdfff))
    throw new Error("escape is invalid Unicode code point");
  return String.fromCodePoint(value);
}

/**
 * The value of an interpreted Go string or rune literal's body (the text between the quotes). Only the literal's own
 * quote may be escaped: `\"` is an error in a rune and `\'` is an error in a string. `\x` and octal escapes name a
 * byte in Go; here they become the character of that code, which differs only for bytes above 0x7f and never matters
 * to a reader that looks for ASCII identifiers.
 */
export function unquoteInterpreted(body: string, quote: '"' | "'"): string {
  let value = "";
  for (let i = 0; i < body.length; i++) {
    const char = body[i] ?? "";
    if (char !== "\\") {
      value += char;
      continue;
    }
    const kind = body[i + 1] ?? "";
    const simple = SIMPLE_ESCAPES[kind];
    if (simple !== undefined || kind === quote) {
      value += simple ?? kind;
      i += 1;
    } else if (kind === "x" || kind === "u" || kind === "U") {
      const width = kind === "x" ? 2 : kind === "u" ? 4 : 8;
      const digits = body.slice(i + 2, i + 2 + width);
      if (digits.length !== width || !HEX.test(digits))
        throw new Error("escape sequence is incomplete");
      value +=
        kind === "x"
          ? String.fromCharCode(Number.parseInt(digits, 16))
          : codePoint(digits);
      i += 1 + width;
    } else if (kind >= "0" && kind <= "7") {
      const digits = body.slice(i + 1, i + 4);
      if (!OCTAL.test(digits) || Number.parseInt(digits, 8) > 255)
        throw new Error("octal escape value is invalid");
      value += String.fromCharCode(Number.parseInt(digits, 8));
      i += 3;
    } else {
      throw new Error("unknown escape sequence");
    }
  }
  return value;
}

export type GoToken =
  | { kind: "string"; value: string; text: string }
  | { kind: "rune" }
  | { kind: "word"; text: string }
  | { kind: "punct"; text: string };

const PUNCTUATION = "();.";
const WHITESPACE = " \t\n\r\f\v";

/**
 * The tokens of one Go file, lazily and in order: interpreted and raw strings (with their value and their source
 * text), runes, runs of other characters ("words") and the punctuation `(`, `)`, `;` and `.`. Comments and white space
 * are skipped. It tokenizes rather than parses, so comments and rune literals' values never reach the caller and build
 * constraints need no handling. It checks what the literals need to be read (termination, escapes, in runes as well as
 * strings) and nothing else about the file: it is not a Go compiler. A malformed literal, or an
 * unterminated comment, raises an error, as the Go scanner's error does.
 * The Go scanner also rejects a NUL byte, invalid UTF-8, a stray character such as `#` (a form feed or a vertical tab
 * is one: this scan treats both as white space), and a rune literal of more than one character. This scan does not
 * reject them: it reads literals correctly without them, and the `tools` domain's `go vet` and `go test` gates can
 * reject such a file when run. `.github/tools/README.md` lists this as a known difference.
 */
export function* goTokens(source: string): Generator<GoToken> {
  let i = 0;
  while (i < source.length) {
    const char = source[i] ?? "";
    const next = source[i + 1];
    if (char === "/" && next === "/") {
      const end = source.indexOf("\n", i);
      i = end === -1 ? source.length : end;
    } else if (char === "/" && next === "*") {
      const end = source.indexOf("*/", i + 2);
      if (end === -1) throw new Error("comment not terminated");
      i = end + 2;
    } else if (char === "`") {
      const end = source.indexOf("`", i + 1);
      if (end === -1) throw new Error("raw string literal not terminated");
      // The Go scanner discards carriage returns from a raw string's value.
      yield {
        kind: "string",
        value: source.slice(i + 1, end).replaceAll("\r", ""),
        text: source.slice(i, end + 1),
      };
      i = end + 1;
    } else if (char === '"' || char === "'") {
      let j = i + 1;
      for (;;) {
        const c = source[j];
        if (c === undefined || c === "\n")
          throw new Error(
            `${char === '"' ? "string" : "rune"} literal not terminated`,
          );
        if (c === "\\") j += 2;
        else if (c === char) break;
        else j += 1;
      }
      const value = unquoteInterpreted(source.slice(i + 1, j), char);
      yield char === '"'
        ? { kind: "string", value, text: source.slice(i, j + 1) }
        : { kind: "rune" };
      i = j + 1;
    } else if (PUNCTUATION.includes(char)) {
      yield { kind: "punct", text: char };
      i += 1;
    } else if (WHITESPACE.includes(char)) {
      i += 1;
    } else {
      let j = i + 1;
      while (j < source.length) {
        const c = source[j] ?? "";
        const after = source[j + 1];
        if (
          WHITESPACE.includes(c) ||
          PUNCTUATION.includes(c) ||
          c === '"' ||
          c === "'" ||
          c === "`" ||
          (c === "/" && (after === "/" || after === "*"))
        )
          break;
        j += 1;
      }
      yield { kind: "word", text: source.slice(i, j) };
      i = j;
    }
  }
}

/** The unquoted string literals of one Go file, in order. See `goTokens` for what is and is not checked. */
export function goStringLiterals(source: string): string[] {
  const literals: string[] = [];
  for (const token of goTokens(source))
    if (token.kind === "string") literals.push(token.value);
  return literals;
}
