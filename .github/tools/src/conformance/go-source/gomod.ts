import { unquoteInterpreted } from "./lexer.ts";

/** The module paths a go.mod requires, and the old and new path of each replacement, each in file order. */
export interface GoMod {
  requires: string[];
  replaces: Array<{ old: string; new: string }>;
}

const SEPARATORS = "()[]{},";

/** The tokens of one physical line, comments dropped. A token that starts with a quote keeps its quotes. */
function lexLine(line: string): string[] {
  const tokens: string[] = [];
  let i = 0;
  while (i < line.length) {
    const char = line[i] ?? "";
    if (char === " " || char === "\t" || char === "\r") {
      i += 1;
    } else if (char === "/" && line[i + 1] === "/") {
      break;
    } else if (SEPARATORS.includes(char)) {
      tokens.push(char);
      i += 1;
    } else if (char === '"' || char === "`") {
      let j = i + 1;
      for (;;) {
        const c = line[j];
        if (c === undefined) throw new Error("unterminated quoted string");
        if (c === char) break;
        j += c === "\\" && char === '"' ? 2 : 1;
      }
      tokens.push(line.slice(i, j + 1));
      i = j + 1;
    } else {
      // A bare token. The `/*` test below also rejects a block comment that opens the token.
      let j = i;
      while (j < line.length) {
        const c = line[j] ?? "";
        if (c === " " || c === "\t" || c === "\r" || SEPARATORS.includes(c))
          break;
        if (c === "/" && line[j + 1] === "/") break;
        if (c === "/" && line[j + 1] === "*")
          throw new Error(
            "mod files must use // comments (not /* */ comments)",
          );
        j += 1;
      }
      tokens.push(line.slice(i, j));
      i = j;
    }
  }
  return tokens;
}

/** `strconv.Unquote` for a token that starts with `"`; any other token holding a quote character is an error. */
function operand(token: string | undefined): string {
  if (token === undefined) throw new Error("missing operand");
  if (token.startsWith('"')) return unquoteInterpreted(token.slice(1, -1), '"');
  if (/["'`]/.test(token))
    throw new Error("unquoted string cannot contain quote");
  return token;
}

/**
 * Records one `require` or `replace` entry from the tokens after its verb, whether it came from a line or a block.
 * It throws when the operand count or the `=>` position is wrong, or an operand is badly quoted, because Go's parser
 * rejects those files; any other verb is ignored, since reading needs only these two.
 */
function directive(verb: string, args: string[], into: GoMod): void {
  if (verb === "require") {
    if (args.length !== 2) throw new Error("usage: require module/path v1.2.3");
    into.requires.push(operand(args[0]));
    operand(args[1]);
  } else if (verb === "replace") {
    const arrow = args[1] === "=>" ? 1 : 2;
    const usage =
      "usage: replace module/path [v1.2.3] => other/module v1.4 or ../local/directory";
    if (
      args.length < arrow + 2 ||
      args.length > arrow + 3 ||
      args[arrow] !== "=>"
    )
      throw new Error(usage);
    const old = operand(args[0]);
    if (arrow === 2) operand(args[1]);
    const replacement = operand(args[arrow + 1]);
    if (args.length === arrow + 3) operand(args[arrow + 2]);
    into.replaces.push({ old, new: replacement });
  }
}

/**
 * The `require` and `replace` entries of one go.mod, in file order, written as lines or in blocks. It reads the file as
 * `modfile.Parse` tokenizes it and checks what reading needs: lexical form, block structure, the operand shape of the
 * two directives and the quoting of their operands. Other directives are tokenized and ignored. It does not check module
 * paths, versions, major-version suffixes, directive names, or whether a replacement is consistent; `.github/tools/README.md`
 * lists this as a known difference.
 */
export function readGoMod(source: string): GoMod {
  const result: GoMod = { requires: [], replaces: [] };
  const lines = source.split("\n").map(lexLine);
  for (let n = 0; n < lines.length; n++) {
    const tokens = lines[n] ?? [];
    const last = tokens.at(-1);
    const empty = last === ")" && tokens.at(-2) === "(";
    if (last === "(" || empty) {
      const header = tokens.slice(0, empty ? -2 : -1);
      if (header.length !== 1)
        throw new Error(`unknown block type: ${header.join(" ")}`);
      const verb = header[0] ?? "";
      if (empty) continue;
      for (n += 1; ; n++) {
        const inner = lines[n];
        if (inner === undefined) throw new Error("unterminated block");
        if (inner.length === 0) continue;
        if (inner[0] === ")") {
          if (inner.length > 1)
            throw new Error("expected newline after closing paren");
          break;
        }
        directive(verb, inner, result);
      }
    } else {
      directive(tokens[0] ?? "", tokens.slice(1), result);
    }
  }
  return result;
}
