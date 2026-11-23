import assert from "node:assert/strict";
import { test } from "node:test";
import {
  parseExplainFiles,
  parseUnresolvedImports,
  toRepoPath,
} from "./explain.ts";

const SAMPLE = [
  "evals/src/index.ts",
  "   Matched by include pattern 'src/**/*.ts' in 'evals/tsconfig.json'",
  "tools/src/index.ts",
  "   Imported via \"../../tools/src/index.ts\" from file 'evals/src/bad-rel.ts'",
  "   Imported via \"agentics-tools\" from file 'evals/src/bad-pkg.ts'",
  '   File is ECMAScript module because \'tools/package.json\' has field "type" with value "module"',
  "",
].join("\n");

test("each file keeps its import reasons and drops the others", () => {
  const files = parseExplainFiles(SAMPLE, "/repo");
  assert.deepEqual(files, [
    { file: "evals/src/index.ts", reasons: [] },
    {
      file: "tools/src/index.ts",
      reasons: [
        {
          specifier: "../../tools/src/index.ts",
          importer: "evals/src/bad-rel.ts",
        },
        { specifier: "agentics-tools", importer: "evals/src/bad-pkg.ts" },
      ],
    },
  ]);
});

test("absolute paths under the root become repo-relative with forward slashes", () => {
  const text =
    "C:\\repo\\tools\\src\\index.ts\n   Imported via \"x\" from file 'C:/repo/evals/src/a.ts'\n";
  assert.deepEqual(parseExplainFiles(text, "C:/repo"), [
    {
      file: "tools/src/index.ts",
      reasons: [{ specifier: "x", importer: "evals/src/a.ts" }],
    },
  ]);
  assert.equal(toRepoPath("/elsewhere/x.ts", "/repo"), "/elsewhere/x.ts");
});

test("a packageId suffix is not part of the importer path", () => {
  const text = [
    "node_modules/yaml/dist/index.js",
    "   Imported via \"yaml\" from file 'evals/src/a.ts' with packageId 'yaml/dist/index.js@2.8.0'",
    "",
  ].join("\n");
  assert.deepEqual(parseExplainFiles(text, "/repo"), [
    {
      file: "node_modules/yaml/dist/index.js",
      reasons: [{ specifier: "yaml", importer: "evals/src/a.ts" }],
    },
  ]);
});

test("lines after a diagnostic are not reasons of the previous file", () => {
  const text = [
    "evals/src/a.ts",
    "evals/src/b.ts(1,1): error TS2307: Cannot find module 'agentics-x'.",
    "   Imported via \"agentics-x\" from file 'evals/src/b.ts'",
    "",
  ].join("\n");
  assert.deepEqual(parseExplainFiles(text, "/repo"), [
    { file: "evals/src/a.ts", reasons: [] },
  ]);
});

test("unresolved bare specifiers are read from module-not-found diagnostics, relative ones are not", () => {
  const text = [
    "skills/demo/scripts/a.cjs",
    "/repo/skills/demo/scripts/a.cjs(1,19): error TS2307: Cannot find module 'axios' or its corresponding type declarations.",
    "skills/demo/scripts/a.cjs(2,19): error TS2307: Cannot find module '@scope/pkg/sub' or its corresponding type declarations.",
    "skills/demo/scripts/a.cjs(3,19): error TS2307: Cannot find module './gone.cjs' or its corresponding type declarations.",
    "skills/demo/scripts/a.cjs(4,7): error TS7006: Parameter 'x' implicitly has an 'any' type.",
    "",
  ].join("\n");
  assert.deepEqual(parseUnresolvedImports(text, "/repo"), [
    { specifier: "axios", importer: "skills/demo/scripts/a.cjs" },
    { specifier: "@scope/pkg/sub", importer: "skills/demo/scripts/a.cjs" },
  ]);
});
