// The release tooling has no runtime dependencies: a production file under
// .github/tools/src/release imports only node:* modules and relative paths. Tests
// and the test helpers are exempt, because they never reach the token-holding
// process.
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import {
  cpSync,
  mkdirSync,
  mkdtempSync,
  readdirSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, sep } from "node:path";
import { test } from "node:test";

const releaseDir = import.meta.dirname;
const EXEMPT_DIRECTORIES = ["testing"];

// Paths use "/" whatever the host separator is.
function isProduction(path: string): boolean {
  return (
    path.endsWith(".ts") &&
    !path.endsWith(".test.ts") &&
    !EXEMPT_DIRECTORIES.includes(path.split("/")[0] ?? "")
  );
}

function typeScriptFiles(dir: string): string[] {
  return readdirSync(dir, { recursive: true, encoding: "utf8" })
    .map((path) => path.split(sep).join("/"))
    .filter((path) => path.endsWith(".ts"))
    .sort();
}

function allowed(specifier: string): boolean {
  return (
    specifier.startsWith("node:") ||
    specifier.startsWith("./") ||
    specifier.startsWith("../")
  );
}

interface Scanned {
  // The source with comments blanked and each string, template and regular
  // expression literal replaced: strings by "#<index>", regular expressions by 0.
  code: string;
  strings: string[];
}

// A small lexer, so an import-looking line inside a comment, a string or a
// regular expression is not read as an import, and a real import is not missed
// because a quote character sat in a literal before it.
function scan(source: string): Scanned {
  const strings: string[] = [];
  const length = source.length;
  let code = "";
  let i = 0;

  const skipQuoted = (start: number): number => {
    const quote = source.charAt(start);
    let j = start + 1;
    while (j < length && source.charAt(j) !== quote) {
      j += source.charAt(j) === "\\" ? 2 : 1;
    }
    return j + 1;
  };
  const skipTemplate = (start: number): number => {
    let j = start + 1;
    while (j < length) {
      const ch = source.charAt(j);
      if (ch === "\\") j += 2;
      else if (ch === "`") return j + 1;
      else if (ch === "$" && source.charAt(j + 1) === "{")
        j = skipBraces(j + 2);
      else j += 1;
    }
    return j;
  };
  const skipBraces = (start: number): number => {
    let depth = 1;
    let j = start;
    while (j < length && depth > 0) {
      const ch = source.charAt(j);
      if (ch === "`") j = skipTemplate(j);
      else if (ch === '"' || ch === "'") j = skipQuoted(j);
      else {
        if (ch === "{") depth += 1;
        if (ch === "}") depth -= 1;
        j += 1;
      }
    }
    return j;
  };
  const skipRegex = (start: number): number => {
    let j = start + 1;
    let inClass = false;
    while (j < length) {
      const ch = source.charAt(j);
      if (ch === "\\") j += 2;
      else if (ch === "/" && !inClass) break;
      else {
        if (ch === "[") inClass = true;
        if (ch === "]") inClass = false;
        j += 1;
      }
    }
    j += 1;
    while (/[a-z]/u.test(source.charAt(j))) j += 1;
    return j;
  };
  const regexAllowed = (): boolean => {
    const before = code.trimEnd();
    return (
      before === "" ||
      "(,=:[!&|?{};+-*%<>~^".includes(before.slice(-1)) ||
      /\b(?:return|typeof|case|void|delete|in|of)$/u.test(before)
    );
  };

  while (i < length) {
    const ch = source.charAt(i);
    const next = source.charAt(i + 1);
    if (ch === "/" && next === "/") {
      while (i < length && source.charAt(i) !== "\n") i += 1;
    } else if (ch === "/" && next === "*") {
      const end = source.indexOf("*/", i + 2);
      i = end === -1 ? length : end + 2;
      code += " ";
    } else if (ch === '"' || ch === "'") {
      const end = skipQuoted(i);
      strings.push(source.slice(i + 1, end - 1));
      code += `"#${strings.length - 1}"`;
      i = end;
    } else if (ch === "`") {
      const end = skipTemplate(i);
      strings.push(source.slice(i + 1, end - 1));
      code += `"#${strings.length - 1}"`;
      i = end;
    } else if (ch === "/" && regexAllowed()) {
      code += "0";
      i = skipRegex(i);
    } else {
      code += ch;
      i += 1;
    }
  }
  return { code, strings };
}

// Every import, export-from, dynamic import and require in one source whose
// specifier is not node:* or relative. A dynamic import whose argument is not a
// literal is a violation too, because nothing can say what it loads.
function importViolations(source: string): string[] {
  const { code, strings } = scan(source);
  const violations: string[] = [];
  const check = (how: string, index: string | undefined): void => {
    const specifier = strings[Number(index)] ?? "";
    if (!allowed(specifier)) violations.push(`${how} "${specifier}"`);
  };
  for (const match of code.matchAll(
    /\b(?:import|export)\b[^;"'`()]*?\bfrom\s*"#(\d+)"/gu,
  )) {
    check("imports", match[1]);
  }
  for (const match of code.matchAll(/\bimport\s*"#(\d+)"/gu)) {
    check("imports", match[1]);
  }
  for (const match of code.matchAll(/\bimport\s*\(\s*(?:"#(\d+)")?/gu)) {
    if (match[1] === undefined) {
      violations.push("dynamic import with a computed specifier");
    } else {
      check("dynamically imports", match[1]);
    }
  }
  for (const match of code.matchAll(/\brequire\s*\(\s*"#(\d+)"/gu)) {
    check("requires", match[1]);
  }
  return violations;
}

function treeViolations(dir: string): string[] {
  return typeScriptFiles(dir)
    .filter(isProduction)
    .flatMap((path) =>
      importViolations(readFileSync(join(dir, path), "utf8")).map(
        (violation) => `${path}: ${violation}`,
      ),
    );
}

test("no production file imports anything but node:* or a relative path", () => {
  const files = typeScriptFiles(releaseDir);
  assert.ok(
    files.includes("zero-deps.test.ts"),
    "the walk must find the files beside this one",
  );
  assert.deepEqual(treeViolations(releaseDir), []);
});

test("asset-roster runs from a cold checkout without installed packages", (t) => {
  const root = mkdtempSync(join(tmpdir(), "agentics-roster-cold-"));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  const coldRelease = join(root, "release");
  cpSync(releaseDir, coldRelease, { recursive: true });
  cpSync(
    join(releaseDir, "../plugin-manifests.ts"),
    join(root, "plugin-manifests.ts"),
  );
  writeFileSync(join(root, "package.json"), '{"type":"module"}\n');
  const result = spawnSync(
    process.execPath,
    [join(coldRelease, "cli.ts"), "asset-roster", "--tag", "v9.8.7"],
    { cwd: root, encoding: "utf8" },
  );
  assert.equal(result.status, 0, result.stderr);
  assert.equal(result.stderr, "");
  assert.match(result.stdout, /"agentics-v9\.8\.7\.tar\.gz"/);
});

test("production files are the .ts files outside tests and test helpers", () => {
  assert.equal(isProduction("cli.ts"), true);
  assert.equal(isProduction("commands/stage.ts"), true);
  assert.equal(isProduction("plugin.test.ts"), false);
  assert.equal(isProduction("commands/stage.test.ts"), false);
  assert.equal(isProduction("testing/support.ts"), false);
  assert.equal(isProduction("notes.txt"), false);
});

// Each snippet is a whole source file with exactly one violation.
const VIOLATIONS: readonly (readonly [string, string])[] = [
  ["a bare static import", 'import x from "left-pad";'],
  ["a scoped static import", 'import { a } from "@scope/pkg";'],
  ["a bare subpath import", 'import { a } from "pkg/sub";'],
  ["a side-effect import", 'import "side-effect";'],
  ["a type import", 'import type { T } from "pkg";'],
  ["an export-star from a package", 'export * from "pkg";'],
  ["a named export from a package", 'export { a } from "pkg";'],
  ["a multi-line import", 'import {\n  a,\n  b,\n} from "pkg";'],
  ["a built-in without the node: prefix", 'import fs from "fs";'],
  ["a package whose name starts with node", 'import x from "node-fetch";'],
  ["a dynamic import of a package", 'const m = await import("pkg");'],
  [
    "a dynamic import with a computed specifier",
    "const m = await import(name);",
  ],
  [
    "a template specifier that starts with an expression",
    "const m = await import(`${dir}/x.ts`);",
  ],
  ["a require of a package", 'const r = require("pkg");'],
  [
    "an import after a regular expression with a quote",
    'const re = /"/;\nimport x from "pkg";',
  ],
  [
    "an import after a string with a slash pair",
    'const u = "https://x";\nimport x from "pkg";',
  ],
];

const CLEAN: readonly (readonly [string, string])[] = [
  ["a node: import", 'import { a } from "node:fs";'],
  ["a node: subpath import", 'import assert from "node:assert/strict";'],
  ["a relative import", 'import b from "./b.ts";'],
  ["a parent-relative import", 'import c from "../c.ts";'],
  ["a relative export-from", 'export { a } from "./a.ts";'],
  ["a relative dynamic import", 'const m = await import("./commands/x.ts");'],
  [
    "a relative template dynamic import",
    "const m = await import(`./commands/${name}.ts`);",
  ],
  [
    "a line comment that looks like an import",
    '// import x from "pkg";\nconst a = 1;',
  ],
  [
    "a block comment that looks like a require",
    '/* require("pkg") */\nconst a = 1;',
  ],
  ["a string that looks like an import", "const s = 'import x from \"pkg\"';"],
  ["a template that looks like an import", 'const t = `import x from "pkg"`;'],
  [
    "a regular expression with a quote",
    'const re = /"/;\nimport x from "node:fs";',
  ],
  [
    "a division, not a regular expression",
    'const q = a / b;\nimport x from "node:fs";',
  ],
  ["import.meta", "const here = import.meta.url;"],
  ["an export without from", "export const answer = 42;"],
];

for (const [name, snippet] of VIOLATIONS) {
  test(`the scan flags ${name}`, () => {
    assert.equal(importViolations(snippet).length, 1, snippet);
  });
}

for (const [name, snippet] of CLEAN) {
  test(`the scan accepts ${name}`, () => {
    assert.deepEqual(importViolations(snippet), [], snippet);
  });
}

test("the tree scan fails on a fixture file that imports a bare specifier", () => {
  const dir = mkdtempSync(join(tmpdir(), "agentics-zero-deps-"));
  try {
    const files: Record<string, string> = {
      "good.ts":
        'import { readFileSync } from "node:fs";\nimport "./bad.ts";\n',
      "bad.ts": 'import leftPad from "left-pad";\n',
      "nested/deep.ts": 'export * from "@scope/pkg";\n',
      "ignored.test.ts": 'import x from "pkg";\n',
      "testing/helper.ts": 'import x from "pkg";\n',
    };
    for (const [path, text] of Object.entries(files)) {
      mkdirSync(dirname(join(dir, path)), { recursive: true });
      writeFileSync(join(dir, path), text);
    }
    assert.deepEqual(treeViolations(dir), [
      'bad.ts: imports "left-pad"',
      'nested/deep.ts: imports "@scope/pkg"',
    ]);
    writeFileSync(join(dir, "bad.ts"), 'import { x } from "./good.ts";\n');
    rmSync(join(dir, "nested"), { recursive: true });
    assert.deepEqual(treeViolations(dir), []);
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
});
