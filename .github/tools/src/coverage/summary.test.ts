import assert from "node:assert/strict";
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { test } from "node:test";
import { main } from "./cli.ts";
import { parseLcov } from "./lcov.ts";
import { listSourceFiles, summarize } from "./summary.ts";

function tree(t: { after: (fn: () => void) => void }, files: string[]): string {
  const root = mkdtempSync(join(tmpdir(), "agentics-coverage-summary-"));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  for (const file of files) {
    mkdirSync(dirname(join(root, file)), { recursive: true });
    writeFileSync(join(root, file), "export {};\n");
  }
  return root;
}

const toolsLcov = [
  "SF:src/a.ts",
  "DA:1,1",
  "DA:2,0",
  "DA:3,4",
  "DA:4,1",
  "BRDA:1,0,0,1",
  "BRDA:1,0,1,0",
  "end_of_record",
  "",
].join("\n");
const secondLcov = [
  "SF:src/b.ts",
  "DA:1,0",
  "DA:2,1",
  "end_of_record",
  "",
].join("\n");

test("each workspace gets a row of line and branch coverage, and the total sums them", (t) => {
  const root = tree(t, [".github/tools/src/a.ts", ".github/tools/src/b.ts"]);
  const reports = [
    {
      workspace: ".github/tools",
      coverage: parseLcov(toolsLcov, {
        root,
        workspace: ".github/tools",
        label: "tools",
      }),
    },
    {
      workspace: "second",
      coverage: parseLcov(secondLcov, {
        root,
        workspace: ".github/tools",
        label: "second",
      }),
    },
  ];
  const text = summarize(reports, listSourceFiles(root));
  assert.match(text, /^## TypeScript coverage$/m);
  assert.match(
    text,
    /^\| \.github\/tools \| 75\.0% \(3\/4\) \| 50\.0% \(1\/2\) \|$/m,
  );
  assert.match(text, /^\| second \| 50\.0% \(1\/2\) \| n\/a \(0\/0\) \|$/m);
  assert.match(text, /^\| total \| 66\.7% \(4\/6\) \| 50\.0% \(1\/2\) \|$/m);
  assert.match(
    text,
    /Every source file under \.github\/tools\/src was loaded by a test\./,
  );
});

test("a source file that no report loaded is listed, and test and testing files are not", (t) => {
  const root = tree(t, [
    ".github/tools/src/a.ts",
    ".github/tools/src/b.ts",
    ".github/tools/src/never.ts",
    "lib/src/gone.ts",
    "evals/src/engine.ts",
    ".github/tools/src/a.test.ts",
    ".github/tools/src/testing/helper.ts",
    ".github/tools/src/notes.md",
  ]);
  const reports = [
    {
      workspace: ".github/tools",
      coverage: parseLcov(toolsLcov, {
        root,
        workspace: ".github/tools",
        label: "tools",
      }),
    },
    {
      workspace: "second",
      coverage: parseLcov(secondLcov, {
        root,
        workspace: ".github/tools",
        label: "second",
      }),
    },
  ];
  const text = summarize(reports, listSourceFiles(root));
  assert.match(text, /^### Source files no test loaded \(1\)$/m);
  assert.match(text, /^- `\.github\/tools\/src\/never\.ts`$/m);
  // Only .github/tools/src is in scope, so a source file elsewhere is never listed.
  assert.doesNotMatch(
    text,
    /a\.test\.ts|helper\.ts|notes\.md|engine\.ts|gone\.ts/,
  );
});

test("the summary command prints the markdown and does not fail on unloaded files", (t) => {
  const root = tree(t, [
    ".github/tools/src/a.ts",
    ".github/tools/src/never.ts",
  ]);
  const lcov = join(root, "tools.lcov");
  writeFileSync(lcov, toolsLcov);
  const printed: string[] = [];
  t.mock.method(console, "log", (line: string) => printed.push(line));
  main(["summary", "--root", root, "--input", `.github/tools:${lcov}`]);
  const text = printed.join("\n");
  assert.match(
    text,
    /^\| \.github\/tools \| 75\.0% \(3\/4\) \| 50\.0% \(1\/2\) \|$/m,
  );
  assert.match(text, /^- `\.github\/tools\/src\/never\.ts`$/m);
  assert.equal(process.exitCode ?? 0, 0);
});

test("the summary command needs a root and at least one input", () => {
  assert.throws(
    () => main(["summary", "--root", "."]),
    /summary requires --root and --input/,
  );
  assert.throws(
    () => main(["summary", "--input", ".github/tools:x.lcov"]),
    /summary requires --root and --input/,
  );
});
