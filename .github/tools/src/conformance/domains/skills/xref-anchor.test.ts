import assert from "node:assert/strict";
import path from "node:path";
import { test } from "node:test";
import { fileURLToPath } from "node:url";
import { repoRoot } from "../../check.ts";
import { ANCHOR_RULE, anchorFindings } from "./xref-anchor.ts";
import { skillDocs, type SkillDoc } from "./xref-md.ts";

const SOURCE = "skills/a/SKILL.md";
const TARGET = "skills/b/references/r.md";
const TARGET_TEXT = "# R\n\n## Step 0 — Preflight\n";

function doc(rel: string, text: string, scanned = true): SkillDoc {
  return { rel, skill: rel.split("/")[1] ?? "", text, scanned };
}

function findings(source: string, scanned = true) {
  const docs = new Map<string, SkillDoc>([
    [SOURCE, doc(SOURCE, source, scanned)],
    [TARGET, doc(TARGET, TARGET_TEXT)],
  ]);
  return anchorFindings(docs);
}

function keys(source: string, scanned = true): string[] {
  return findings(source, scanned).map((f) => f.key);
}

test("a link whose anchor matches a heading in the target passes", () => {
  assert.deepEqual(
    keys("# A\n\nSee [x](../b/references/r.md#step-0--preflight).\n"),
    [],
  );
});

test("a link whose anchor matches no heading in the target is reported", () => {
  assert.deepEqual(findings("# A\n\nSee [x](../b/references/r.md#gone).\n"), [
    {
      rule: ANCHOR_RULE,
      path: SOURCE,
      key: "../b/references/r.md#gone",
      message:
        "line 3: link ../b/references/r.md#gone names #gone, which matches no heading in skills/b/references/r.md",
    },
  ]);
});

test("a same-file anchor that matches a heading passes", () => {
  assert.deepEqual(keys("# A\n\n## Local\n\n[x](#local)\n"), []);
});

test("a same-file anchor that matches no heading is reported", () => {
  const out = findings("# A\n\n[x](#nope)\n");
  assert.deepEqual(
    out.map((f) => [f.key, f.message]),
    [
      [
        "#nope",
        "line 3: link #nope names #nope, which matches no heading in skills/a/SKILL.md",
      ],
    ],
  );
});

test("an empty anchor is skipped", () => {
  assert.deepEqual(keys("[x](#) and [y](../b/references/r.md#)\n"), []);
});

test("a link with a scheme is skipped", () => {
  assert.deepEqual(keys("[x](https://e.com/a.md#z)\n"), []);
});

test("a bad link inside a fenced block is not read", () => {
  assert.deepEqual(keys("# A\n\n```md\n[x](#nope)\n```\n"), []);
});

test("a bad link inside a fenced block in a blockquote is not read", () => {
  assert.deepEqual(keys("# A\n\n> ```md\n> [x](#nope)\n> ```\n"), []);
});

test("a bad link inside an inline code span is reported", () => {
  assert.deepEqual(keys("# A\n\nUse `[x](#nope)` here.\n"), ["#nope"]);
});

test("a doc that is not scanned is not read", () => {
  assert.deepEqual(keys("# A\n\n[x](#nope)\n", false), []);
});

test("a link to a file that is not in docs is skipped", () => {
  assert.deepEqual(keys("[x](../c/references/missing.md#gone)\n"), []);
});

test("a link with a title is read up to the target", () => {
  assert.deepEqual(keys('# A\n\n[x](#nope "t")\n'), ["#nope"]);
});

test("two bad links on one line give two findings", () => {
  assert.deepEqual(keys("[x](#nope) and [y](../b/references/r.md#gone)\n"), [
    "#nope",
    "../b/references/r.md#gone",
  ]);
});

test("every anchored link in the repository skills names a heading", () => {
  const root = repoRoot(path.dirname(fileURLToPath(import.meta.url)));
  assert.deepEqual(anchorFindings(skillDocs(root)), []);
});
