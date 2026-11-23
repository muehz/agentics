import assert from "node:assert/strict";
import test from "node:test";
import { selectScope, changedLines } from "./mutation-scope.ts";

test("the moved configuration stays excluded without broadening an empty scope", () => {
  assert.deepEqual(
    selectScope([".github/tools/src/scripts/mutation-config.ts"]),
    {
      stryker: [],
      gremlins: [],
    },
  );
  assert.deepEqual(
    selectScope([
      ".github/tools/src/scripts/mutation-config.ts",
      ".github/tools/src/a.ts",
    ]),
    { stryker: [".github/tools/src/a.ts"], gremlins: [] },
  );
});

test("TypeScript sources in .github/tools/src go to Stryker, sorted and unique, and nothing outside it does", () => {
  const scope = selectScope([
    ".github/tools/src/peer/config.ts",
    "lib/src/index.ts",
    ".github/tools/src/a.ts",
    ".github/tools/src/peer/config.ts",
  ]);
  assert.deepEqual(scope.stryker, [
    ".github/tools/src/a.ts",
    ".github/tools/src/peer/config.ts",
  ]);
});

test("Go sources under hooks/go group by package with their changed file names", () => {
  const scope = selectScope([
    "hooks/go/internal/toolname/toolname.go",
    "hooks/go/cmd/hook/main.go",
    "hooks/go/cmd/hook/event.go",
  ]);
  assert.deepEqual(scope.gremlins, [
    { pkg: "./cmd/hook", changed: ["event.go", "main.go"] },
    { pkg: "./internal/toolname", changed: ["toolname.go"] },
  ]);
});

test("tests, test helpers, test data, binaries, and docs are out of scope", () => {
  const scope = selectScope([
    ".github/tools/src/peer/config.test.ts",
    ".github/tools/src/release/testing/repo.ts",
    "evals/src/cli/main.ts",
    "hooks/go/internal/toolname/toolname_test.go",
    "hooks/go/cmd/hook/testdata/corpus/x.go",
    "hooks/bin/agx-linux-amd64",
    "docs/x.md",
  ]);
  assert.deepEqual(scope, { stryker: [], gremlins: [] });
});

const DIFF = [
  "diff --git a/.github/tools/src/x/a.ts b/.github/tools/src/x/a.ts",
  "--- a/.github/tools/src/x/a.ts",
  "+++ b/.github/tools/src/x/a.ts",
  "@@ -3 +3 @@ function a() {",
  "-old",
  "+new",
  "@@ -10,0 +11,4 @@",
  "+one",
  "+two",
  "+three",
  "+four",
  "@@ -20,2 +24,0 @@",
  "-gone",
  "-gone too",
  "diff --git a/.github/tools/src/x/b.ts b/.github/tools/src/x/b.ts",
  "--- a/.github/tools/src/x/b.ts",
  "+++ b/.github/tools/src/x/b.ts",
  "@@ -5,3 +4,0 @@",
  "-x",
  "diff --git a/.github/tools/src/x/new.ts b/.github/tools/src/x/new.ts",
  "--- /dev/null",
  "+++ b/.github/tools/src/x/new.ts",
  "@@ -0,0 +1,2 @@",
  "+first",
  "+second",
].join("\n");

test("changed lines are the added and rewritten ranges of each file, in new-file numbering", () => {
  assert.deepEqual(changedLines(DIFF), {
    ".github/tools/src/x/a.ts": [
      [3, 3],
      [11, 14],
    ],
    ".github/tools/src/x/b.ts": [],
    ".github/tools/src/x/new.ts": [[1, 2]],
  });
});

test("a hunk that only deletes lines adds no range, and its file still appears", () => {
  assert.deepEqual(changedLines(DIFF)[".github/tools/src/x/b.ts"], []);
});
