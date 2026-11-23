import { readFileSync } from "node:fs";
import { dirname, isAbsolute, relative, resolve, sep } from "node:path";
import {
  serializeCobertura,
  type CoberturaDocument,
  type CoberturaLine,
  type CoberturaMethod,
  type CoberturaPackage,
} from "./cobertura.ts";

interface Position {
  line: number;
  column: number;
}
interface GoFunction {
  name: string;
  start: Position;
  end: Position;
}
interface Token {
  text: string;
  start: Position;
  end: Position;
}
interface Block {
  start: Position;
  end: Position;
  count: number;
}
interface Profile {
  mode: "set" | "count" | "atomic";
  files: Map<string, Block[]>;
}
export interface GoProfileOptions {
  module: string;
  sourceRoot?: string;
  label?: string;
}

function compare(a: Position, b: Position): number {
  return a.line - b.line || a.column - b.column;
}

export function parseGoProfile(text: string, label = "profile"): Profile {
  const rows = text.trimEnd().split(/\r?\n/);
  const mode = /^mode: (set|count|atomic)$/.exec(rows[0] ?? "")?.[1] as
    Profile["mode"] | undefined;
  if (!mode) throw new Error(`${label}:1: invalid coverage mode`);
  const files = new Map<string, Block[]>();
  for (let index = 1; index < rows.length; index++) {
    const match = /^(.+):(\d+)\.(\d+),(\d+)\.(\d+) (\d+) (\d+)$/.exec(
      rows[index]!,
    );
    if (!match)
      throw new Error(`${label}:${index + 1}: invalid coverage block`);
    const numbers = match.slice(2).map(Number);
    if (numbers.some((n) => !Number.isSafeInteger(n)))
      throw new Error(`${label}:${index + 1}: invalid coverage integer`);
    const [startLine, startCol, endLine, endCol, , count] = numbers as [
      number,
      number,
      number,
      number,
      number,
      number,
    ];
    const start = { line: startLine, column: startCol },
      end = { line: endLine, column: endCol };
    if (
      Math.min(startLine, startCol, endLine, endCol) < 1 ||
      compare(start, end) > 0
    )
      throw new Error(`${label}:${index + 1}: invalid coverage range`);
    const blocks = files.get(match[1]!) ?? [];
    blocks.push({ start, end, count });
    files.set(match[1]!, blocks);
  }
  for (const blocks of files.values())
    blocks.sort((a, b) => compare(a.start, b.start));
  return { mode, files };
}

function tokens(source: string, label: string): Token[] {
  const result: Token[] = [];
  let index = 0,
    line = 1,
    lineStart = 0;
  const position = (): Position => ({
    line,
    column: Buffer.byteLength(source.slice(lineStart, index)) + 1,
  });
  const advance = () => {
    if (source[index++] === "\n") {
      line++;
      lineStart = index;
    }
  };
  const semicolon = () => {
    const previous = result.at(-1)?.text ?? "";
    if (
      (/^(?:[\p{L}\p{N}_]|literal)/u.test(previous) &&
        ![
          "func",
          "var",
          "const",
          "type",
          "package",
          "import",
          "struct",
          "interface",
          "map",
          "chan",
        ].includes(previous)) ||
      [")", "]", "}", "++", "--"].includes(previous)
    ) {
      const p = position();
      result.push({ text: ";", start: p, end: p });
    }
  };
  while (index < source.length) {
    const char = source[index]!;
    if (/\s/.test(char)) {
      if (char === "\n") semicolon();
      advance();
      continue;
    }
    if (source.startsWith("//", index)) {
      while (index < source.length && source[index] !== "\n") advance();
      continue;
    }
    if (source.startsWith("/*", index)) {
      advance();
      advance();
      let ended = false;
      while (index < source.length) {
        if (source.startsWith("*/", index)) {
          advance();
          advance();
          ended = true;
          break;
        }
        if (source[index] === "\n") semicolon();
        advance();
      }
      if (!ended) throw new Error(`${label}: unclosed block comment`);
      continue;
    }
    const start = position();
    if (['"', "'", "`"].includes(char)) {
      advance();
      let ended = false;
      while (index < source.length) {
        if (source[index] === char) {
          advance();
          ended = true;
          break;
        }
        if (source[index] === "\\" && char !== "`") {
          advance();
          if (index < source.length) advance();
        } else advance();
      }
      if (!ended) throw new Error(`${label}: unclosed literal`);
      result.push({ text: "literal", start, end: position() });
      continue;
    }
    const word = /^[\p{L}_][\p{L}\p{N}_]*/u.exec(source.slice(index))?.[0];
    const number = /^\d[\w.]*/.exec(source.slice(index))?.[0];
    const text =
      word ??
      number ??
      (["++", "--", ":="].includes(source.slice(index, index + 2))
        ? source.slice(index, index + 2)
        : char);
    for (let count = 0; count < text.length; count++) advance();
    result.push({ text, start, end: position() });
  }
  semicolon();
  return result;
}

export function sourceFunctions(
  source: string,
  label = "source",
): GoFunction[] {
  const input = tokens(source, label);
  const result: GoFunction[] = [];
  const pairs = new Map([
    ["(", ")"],
    ["[", "]"],
    ["{", "}"],
  ]);
  const matching = new Map<number, number>();
  const stack: number[] = [];
  for (let i = 0; i < input.length; i++) {
    const text = input[i]!.text;
    if (pairs.has(text)) stack.push(i);
    else if ([")", "]", "}"].includes(text)) {
      const open = stack.pop();
      if (open === undefined || pairs.get(input[open]!.text) !== text)
        throw new Error(`${label}: mismatched ${text}`);
      matching.set(open, i);
    }
  }
  if (stack.length)
    throw new Error(`${label}: unclosed ${input[stack[0]!]!.text}`);
  const text = (index: number) => input[index]?.text;
  function functionAt(
    start: number,
    name: string | undefined,
  ): { fn: GoFunction; after: number } | undefined {
    let cursor = start + 1;
    if (name === undefined) {
      if (text(cursor) === "(") cursor = matching.get(cursor)! + 1;
      name = text(cursor);
      if (!name || !/^[\p{L}_]/u.test(name)) return undefined;
      cursor++;
      if (text(cursor) === "[") cursor = matching.get(cursor)! + 1;
    }
    if (text(cursor) !== "(") return undefined;
    cursor = matching.get(cursor)! + 1;
    while (cursor < input.length && text(cursor) !== ";") {
      if (
        text(cursor) === "{" &&
        !["struct", "interface"].includes(text(cursor - 1) ?? "")
      ) {
        const end = matching.get(cursor)!;
        return {
          fn: { name, start: input[start]!.start, end: input[end]!.end },
          after: end + 1,
        };
      }
      if (pairs.has(text(cursor) ?? "")) cursor = matching.get(cursor)! + 1;
      else cursor++;
    }
    return undefined;
  }
  function variableSpec(start: number, end: number) {
    let equals = start;
    while (equals < end && text(equals) !== "=") {
      equals = pairs.has(text(equals) ?? "")
        ? matching.get(equals)! + 1
        : equals + 1;
    }
    if (equals === end) return;
    const names = input
      .slice(start, equals)
      .filter((token, i, all) => i === 0 || all[i - 1]?.text === ",")
      .map((token) => token.text);
    let cursor = equals + 1,
      value = 0;
    while (cursor < end) {
      let next = cursor;
      while (next < end && text(next) !== ",")
        next = pairs.has(text(next) ?? "") ? matching.get(next)! + 1 : next + 1;
      if (text(cursor) === "func") {
        const found = functionAt(cursor, names[value] ?? "-");
        if (found?.after === next) result.push(found.fn);
      }
      cursor = next + 1;
      value++;
    }
  }
  function specEnd(start: number, limit: number): number {
    let end = start;
    while (end < limit && text(end) !== ";")
      end = pairs.has(text(end) ?? "") ? matching.get(end)! + 1 : end + 1;
    return end;
  }
  for (let cursor = 0; cursor < input.length;) {
    if (text(cursor) === "func") {
      const found = functionAt(cursor, undefined);
      if (found) {
        result.push(found.fn);
        cursor = found.after;
        continue;
      }
    } else if (text(cursor) === "var") {
      cursor++;
      if (text(cursor) === "(") {
        const end = matching.get(cursor)!;
        for (let start = cursor + 1; start < end;) {
          if (text(start) === ";") {
            start++;
            continue;
          }
          const next = specEnd(start, end);
          variableSpec(start, next);
          start = next + 1;
        }
        cursor = end + 1;
      } else {
        const end = specEnd(cursor, input.length);
        variableSpec(cursor, end);
        cursor = end;
      }
      continue;
    }
    if (pairs.has(text(cursor) ?? "")) cursor = matching.get(cursor)! + 1;
    else cursor++;
  }
  return result;
}

function readGoProfile(
  text: string,
  moduleRoot: string,
  label: string | undefined,
): Omit<CoberturaDocument, "timestamp"> {
  const profile = parseGoProfile(text, label);
  const moduleName = /^module\s+(\S+)/m
    .exec(readFileSync(resolve(moduleRoot, "go.mod"), "utf8"))?.[1]
    ?.replace(/^"|"$/g, "");
  if (!moduleName) throw new Error(`${moduleRoot}: missing module directive`);
  const packages = new Map<string, CoberturaPackage>();
  let totalValid = 0,
    totalCovered = 0;
  for (const [filename, blocks] of [...profile.files].sort(([a], [b]) =>
    a.localeCompare(b),
  )) {
    if (!filename.startsWith(`${moduleName}/`))
      throw new Error(`${filename}: outside module ${moduleName}`);
    const relativeName = filename
      .slice(moduleName.length + 1)
      .replaceAll("\\", "/");
    const fullPath = resolve(moduleRoot, relativeName);
    const bounded = relative(moduleRoot, fullPath);
    if (
      isAbsolute(bounded) ||
      bounded === ".." ||
      bounded.startsWith(`..${sep}`)
    )
      throw new Error(`${filename}: outside module source root`);
    const functions = sourceFunctions(
      readFileSync(fullPath, "utf8"),
      relativeName,
    );
    const methods: CoberturaMethod[] = [];
    const fileLines: CoberturaLine[] = [];
    let fileValid = 0,
      fileCovered = 0;
    for (const fn of functions) {
      const lines = new Map<number, number>();
      for (const block of blocks) {
        if (compare(block.start, fn.end) >= 0) break;
        if (compare(block.end, fn.start) <= 0) continue;
        for (
          let number = block.start.line;
          number <= block.end.line;
          number++
        ) {
          const prior = lines.get(number) ?? 0;
          lines.set(
            number,
            profile.mode === "set"
              ? Number(BigInt(prior) | BigInt(block.count))
              : prior + block.count,
          );
        }
      }
      const hits = [...lines.values()].filter((n) => n > 0).length;
      fileValid += lines.size;
      fileCovered += hits;
      const sorted = [...lines]
        .sort(([a], [b]) => a - b)
        .map(([number, hits]) => ({ number, hits }));
      fileLines.push(...sorted);
      methods.push({
        name: fn.name,
        lineCount: { covered: hits, valid: lines.size },
        lines: sorted,
      });
    }
    if (!functions.length) continue;
    totalValid += fileValid;
    totalCovered += fileCovered;
    const name = dirname(filename).replaceAll("\\", "/");
    const pkg = packages.get(name) ?? {
      name,
      classes: [],
      lineCount: { covered: 0, valid: 0 },
    };
    pkg.lineCount.valid += fileValid;
    pkg.lineCount.covered += fileCovered;
    pkg.classes.push({
      name: relativeName,
      filename: relativeName,
      lineCount: { covered: fileCovered, valid: fileValid },
      methods,
      lines: fileLines,
    });
    packages.set(name, pkg);
  }
  return {
    lineCount: { covered: totalCovered, valid: totalValid },
    packages: [...packages.values()],
  };
}

export function convertGoProfile(
  text: string,
  options: GoProfileOptions,
  timestamp = Date.now(),
): string {
  const sourceRoot = resolve(options.sourceRoot ?? process.cwd());
  const moduleRoot = resolve(sourceRoot, options.module);
  const prefix = relative(sourceRoot, moduleRoot).split(sep).join("/");
  if (isAbsolute(prefix) || prefix === ".." || prefix.startsWith("../"))
    throw new Error(`${moduleRoot}: outside repository source root`);
  return serializeCobertura(
    { ...readGoProfile(text, moduleRoot, options.label), timestamp },
    "go",
    prefix,
  );
}
