import assert from "node:assert/strict";
import path from "node:path";
import { test } from "node:test";
import { fileURLToPath } from "node:url";
import { repoRoot } from "../../check.ts";
import {
  blocks,
  DUPLICATE_RULE,
  duplicateFindings,
  MIN_WORDS,
} from "./duplication.ts";
import { skillDocs, type SkillDoc } from "./xref-md.ts";

const SKILL = "skills/a/SKILL.md";

const NINE_A = "alpha one two three four five six seven eight";
const NINE_B = "beta one two three four five six seven eight";
const NINE_C = "gamma one two three four five six seven eight";
const NINE_D = "delta one two three four five six seven eight";
const NINE_E = "epsilon one two three four five six seven eight";
const NINE_X = "unique one two three four five six seven eight";

function doc(rel: string, text: string): SkillDoc {
  return { rel, skill: rel.split("/")[1] ?? "", text, scanned: true };
}

function findings(text: string, rel = SKILL) {
  return duplicateFindings(new Map([[rel, doc(rel, text)]]));
}

test("MIN_WORDS is eight", () => {
  assert.equal(MIN_WORDS, 8);
});

test("blocks splits paragraphs, list items and nested items, and skips headings, tables and code", () => {
  const text = [
    "---",
    "name: a",
    "---",
    "# Title",
    "First   line",
    "continues\there.",
    "",
    "- item one",
    "  wraps on",
    "  1. nested item",
    "| a | b |",
    "    indented code",
    "```",
    "fenced",
    "```",
    "after fence",
  ].join("\n");
  assert.deepEqual(blocks(text), [
    { line: 5, text: "First line continues here." },
    { line: 8, text: "item one wraps on" },
    { line: 10, text: "nested item" },
    { line: 16, text: "after fence" },
  ]);
});

test("an indented line continues an open block", () => {
  assert.deepEqual(blocks("para\n    more\n"), [
    { line: 1, text: "para more" },
  ]);
});

test("a three-item list printed twice is one finding with a run of three", () => {
  const list = `- ${NINE_A}\n- ${NINE_B}\n- ${NINE_C}\n`;
  const got = findings(`# A\n\n${list}\nMiddle.\n\n${list}`);
  assert.equal(got.length, 1);
  assert.match(got[0]?.message ?? "", /a run of 3 block\(s\)/);
});

test("the same paragraph twice, apart, is one finding with exact key and message", () => {
  const got = findings(`${NINE_A}\n\n${NINE_X}\n\n${NINE_A}\n`);
  assert.deepEqual(got, [
    {
      rule: DUPLICATE_RULE,
      path: SKILL,
      key: "alpha one two three four five six seven",
      message:
        'lines 1 and 5 start a run of 1 block(s) printed twice: "alpha one two three four five six seven"',
    },
  ]);
});

test("a seven-word paragraph printed twice is not reported", () => {
  const seven = "one two three four five six seven";
  assert.deepEqual(findings(`${seven}\n\n${seven}\n`), []);
});

test("the same line twice inside fences is not reported", () => {
  assert.deepEqual(findings("```\n" + `${NINE_A}\n${NINE_A}\n` + "```\n"), []);
});

test("the same table row twice is not reported", () => {
  const row = `| ${NINE_A} |`;
  assert.deepEqual(findings(`${row}\n${row}\n`), []);
});

test("two runs separated by a non-repeated long paragraph are two findings", () => {
  const text = [
    NINE_A,
    NINE_B,
    NINE_C,
    NINE_D,
    NINE_E,
    NINE_A,
    NINE_B,
    NINE_X,
    NINE_C,
    NINE_D,
  ].join("\n\n");
  const got = findings(text);
  assert.deepEqual(
    got.map((f) => f.message),
    [
      'lines 1 and 11 start a run of 2 block(s) printed twice: "alpha one two three four five six seven"',
      'lines 5 and 17 start a run of 2 block(s) printed twice: "gamma one two three four five six seven"',
    ],
  );
});

test("a nested list item printed twice is one finding", () => {
  const text = `- parent\n  - ${NINE_A}\n\n${NINE_X}\n\n- other\n  - ${NINE_A}\n`;
  assert.equal(findings(text).length, 1);
});

test("a repeat in a reference file is not reported", () => {
  assert.deepEqual(
    findings(`${NINE_A}\n\n${NINE_A}\n`, "skills/a/references/r.md"),
    [],
  );
});

test("a removed fence ends a block even with no blank line around it", () => {
  const text = `${NINE_A}\n` + "```\ncode\n```\n" + `${NINE_A}\n`;
  assert.deepEqual(blocks(text), [
    { line: 1, text: NINE_A },
    { line: 5, text: NINE_A },
  ]);
  assert.equal(findings(text).length, 1);
});

test("an eight-word paragraph printed twice is reported", () => {
  const eight = "one two three four five six seven eight";
  assert.equal(findings(`${eight}\n\n${eight}\n`).length, 1);
});

test("every finding in the real checkout names the rule and a SKILL.md", () => {
  const root = repoRoot(path.dirname(fileURLToPath(import.meta.url)));
  for (const finding of duplicateFindings(skillDocs(root))) {
    assert.equal(finding.rule, DUPLICATE_RULE);
    assert.match(finding.path, /\/SKILL\.md$/);
  }
});
