import assert from "node:assert/strict";
import { test } from "node:test";
import { areaImportCheck, skillImportViolations } from "./imports.ts";
import type { ResolvedFile } from "./explain.ts";

const edge = (
  file: string,
  specifier: string,
  importer: string,
): ResolvedFile => ({
  file,
  reasons: [{ specifier, importer }],
});

test("an edge that leaves the area is one violation naming the importer and specifier", () => {
  const result = areaImportCheck("tools", [
    edge("hooks/x.ts", "../../hooks/x.ts", ".github/tools/src/a.ts"),
    edge("hooks/x.ts", "../../hooks/x.ts", ".github/tools/src/a.ts"),
  ]);
  assert.equal(result.length, 1);
  assert.deepEqual(result[0], {
    rule: "area-ts-import",
    path: ".github/tools/src/a.ts",
    message:
      'imports "../../hooks/x.ts", which resolves to hooks/x.ts outside tools',
  });
});

test("an edge that stays inside the area is allowed", () => {
  const result = areaImportCheck("tools", [
    edge(".github/tools/src/b.ts", "./b.ts", ".github/tools/src/a.ts"),
  ]);
  assert.deepEqual(result, []);
});

test("a path outside every area is a violation too", () => {
  const result = areaImportCheck("tools", [
    edge(
      "lib/src/index.ts",
      "../../lib/src/index.ts",
      ".github/tools/src/a.ts",
    ),
  ]);
  assert.equal(result.length, 1);
});

test("third-party files and edges whose importer is outside the area are ignored", () => {
  const result = areaImportCheck("tools", [
    edge("node_modules/yaml/dist/index.js", "yaml", ".github/tools/src/a.ts"),
    edge("hooks/b.ts", "./b.ts", "evals/x.ts"),
  ]);
  assert.deepEqual(result, []);
});

test("skill edges from another skill are ignored and repeated edges are reported once", () => {
  const violations = skillImportViolations("scaffold", [
    edge(
      "hooks/go/x.cjs",
      "../../../hooks/go/x.cjs",
      "skills/other/scripts/c.cjs",
    ),
    edge(
      "hooks/go/x.cjs",
      "../../../hooks/go/x.cjs",
      "skills/scaffold/scripts/a.cjs",
    ),
    edge(
      "hooks/go/x.cjs",
      "../../../hooks/go/x.cjs",
      "skills/scaffold/scripts/a.cjs",
    ),
  ]);
  assert.equal(violations.length, 1);
  assert.equal(violations[0]?.path, "skills/scaffold/scripts/a.cjs");
});

test("a skill script may use Node built-ins and its own files, and nothing else", () => {
  const violations = skillImportViolations("scaffold", [
    edge(
      "node_modules/@types/node/fs.d.ts",
      "node:fs",
      "skills/scaffold/scripts/a.cjs",
    ),
    edge(
      "skills/scaffold/scripts/b.cjs",
      "./b.cjs",
      "skills/scaffold/scripts/a.cjs",
    ),
    edge(
      "hooks/go/x.cjs",
      "../../../hooks/go/x.cjs",
      "skills/scaffold/scripts/a.cjs",
    ),
    edge(
      "node_modules/yaml/dist/index.js",
      "yaml",
      "skills/scaffold/scripts/a.cjs",
    ),
  ]);
  assert.deepEqual(
    violations.map((v) => [
      v.rule,
      v.path,
      v.message.includes("hooks/go/x.cjs") ||
        v.message.includes("node_modules/yaml"),
    ]),
    [
      ["area-skill-import", "skills/scaffold/scripts/a.cjs", true],
      ["area-skill-import", "skills/scaffold/scripts/a.cjs", true],
    ],
  );
});

test("a package the compiler could not resolve is a violation, a built-in or another skill's import is not", () => {
  const violations = skillImportViolations(
    "scaffold",
    [],
    [
      { specifier: "axios", importer: "skills/scaffold/scripts/a.cjs" },
      { specifier: "axios", importer: "skills/scaffold/scripts/a.cjs" },
      { specifier: "node:sqlite", importer: "skills/scaffold/scripts/a.cjs" },
      { specifier: "fs/promises", importer: "skills/scaffold/scripts/a.cjs" },
      { specifier: "axios", importer: "skills/other/scripts/b.cjs" },
    ],
  );
  assert.equal(violations.length, 1);
  assert.equal(violations[0]?.rule, "area-skill-import");
  assert.equal(violations[0]?.path, "skills/scaffold/scripts/a.cjs");
  assert.match(violations[0]!.message, /"axios"/);
});

test("a resolved import into the Node types is allowed only for a built-in specifier", () => {
  const violations = skillImportViolations("scaffold", [
    edge(
      "node_modules/@types/node/index.d.ts",
      "@types/node",
      "skills/scaffold/scripts/a.cjs",
    ),
    edge(
      "node_modules/@types/node/fs/promises.d.ts",
      "node:fs/promises",
      "skills/scaffold/scripts/a.cjs",
    ),
    edge(
      "node_modules/@types/node/path.d.ts",
      "path",
      "skills/scaffold/scripts/a.cjs",
    ),
  ]);
  assert.equal(violations.length, 1);
  assert.match(violations[0]!.message, /"@types\/node"/);
});
