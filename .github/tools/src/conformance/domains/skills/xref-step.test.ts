import assert from "node:assert/strict";
import path from "node:path";
import { test } from "node:test";
import { fileURLToPath } from "node:url";
import { repoRoot } from "../../check.ts";
import { STEP_RULE, stepFindings } from "./xref-step.ts";
import { type SkillDoc, skillDocs } from "./xref-md.ts";

const X_SKILL = [
  "# x",
  "",
  "## Step 2: Plan",
  "",
  "### 3. Interview",
  "",
  "**Plan artifact identity.** text",
  "",
  "## Step 2.45 Narrow",
  "",
  "## Step 7.1 Sub",
  "",
  "**Step 0. Preflight.** text",
].join("\n");

const X_REFERENCE = "# W\n\n**Step 5.** text\n";

const SKILLS: ReadonlySet<string> = new Set(["x", "y"]);

function doc(rel: string, text: string, scanned = true): SkillDoc {
  return { rel, skill: rel.split("/")[1] ?? "", text, scanned };
}

/** The keys of the findings for one line of prose in skill `y`, with skill `x` declaring the fixture labels. */
function keysFor(line: string): string[] {
  const docs = new Map<string, SkillDoc>();
  for (const d of [
    doc("skills/x/SKILL.md", X_SKILL),
    doc("skills/x/references/w.md", X_REFERENCE),
    doc("skills/y/SKILL.md", `# y\n\n${line}\n`),
  ])
    docs.set(d.rel, d);
  const findings = stepFindings(docs, SKILLS);
  for (const f of findings) {
    assert.equal(f.rule, STEP_RULE);
    assert.equal(f.path, "skills/y/SKILL.md");
  }
  return findings.map((f) => f.key);
}

const CASES: [string, string[]][] = [
  ["see `x` Step 2", []],
  ["see `x` Step 3", []],
  ["shaped per x Step 3", []],
  ["see `x` Step 9", ["`x` Step 9"]],
  ["shaped per x Step 9", ["x Step 9"]],
  ["Step 9 of `x`", ["Step 9 of `x`"]],
  ["Step 2 of `skills/x/SKILL.md`", []],
  ["Step 9 of `skills/x/SKILL.md`", ["Step 9 of `skills/x/SKILL.md`"]],
  ["see `x` Step 5", []],
  ["the **Plan artifact identity** rule in `skills/x/SKILL.md`", []],
  ["the **Plan artifact identity** rule in `skills/x/references/w.md`", []],
  [
    "the **Missing thing** rule in `skills/x/SKILL.md`",
    ["**Missing thing** rule in `skills/x/SKILL.md`"],
  ],
  ["x's **Missing thing** step", ["x's **Missing thing** step"]],
  ["`x`'s **Missing thing** rule", ["`x`'s **Missing thing** rule"]],
  ["`x`'s **plan ARTIFACT** rule", []],
  ["`x`'s Step 9", ["`x`'s Step 9"]],
  ["`x`'s Step 2", []],
  ["spec's **Policy skills**", []],
  ["see `x` Step 2.4", ["`x` Step 2.4"]],
  ["see `x` Step 2.45", []],
  ["see `x` Step 7", ["`x` Step 7"]],
  ["see `x` Step 0", []],
  ["/x Step 9 and a/x Step 9", []],
  ["unknown `zz` Step 1", []],
];

for (const [line, keys] of CASES)
  test(`stepFindings reads ${JSON.stringify(line)}`, () => {
    assert.deepEqual(keysFor(line), keys);
  });

test("stepFindings gives one finding per occurrence with the line number and the missing label", () => {
  const docs = new Map<string, SkillDoc>([
    ["skills/x/SKILL.md", doc("skills/x/SKILL.md", X_SKILL)],
    [
      "skills/y/SKILL.md",
      doc(
        "skills/y/SKILL.md",
        "# y\n\nsee `x` Step 9 and `x` Step 9\n\nx's **Gone** rule\n",
      ),
    ],
  ]);
  assert.deepEqual(stepFindings(docs, SKILLS), [
    {
      rule: STEP_RULE,
      path: "skills/y/SKILL.md",
      key: "`x` Step 9",
      message:
        "line 3: `x` Step 9: x has no heading or bold lead-in for Step 9",
    },
    {
      rule: STEP_RULE,
      path: "skills/y/SKILL.md",
      key: "`x` Step 9",
      message:
        "line 3: `x` Step 9: x has no heading or bold lead-in for Step 9",
    },
    {
      rule: STEP_RULE,
      path: "skills/y/SKILL.md",
      key: "x's **Gone** rule",
      message:
        'line 5: x\'s **Gone** rule: x has no heading or bold lead-in for "Gone"',
    },
  ]);
});

test("stepFindings ignores a reference inside a fence", () => {
  const docs = new Map<string, SkillDoc>([
    ["skills/x/SKILL.md", doc("skills/x/SKILL.md", X_SKILL)],
    [
      "skills/y/SKILL.md",
      doc("skills/y/SKILL.md", "# y\n\n```\nsee `x` Step 9\n```\n"),
    ],
  ]);
  assert.deepEqual(stepFindings(docs, SKILLS), []);
});

test("stepFindings reads only scanned docs but takes labels from every doc of the skill", () => {
  const docs = new Map<string, SkillDoc>([
    ["skills/x/SKILL.md", doc("skills/x/SKILL.md", "# x\n")],
    [
      "skills/x/assets/a.md",
      doc("skills/x/assets/a.md", "## Step 7\n\n`x` Step 9\n", false),
    ],
    ["skills/y/SKILL.md", doc("skills/y/SKILL.md", "`x` Step 7\n")],
  ]);
  assert.deepEqual(stepFindings(docs, SKILLS), []);
});

test("this checkout has no unresolved step or named-rule reference", () => {
  const root = repoRoot(path.dirname(fileURLToPath(import.meta.url)));
  assert.notEqual(root, "");
  const docs = skillDocs(root);
  const skills = new Set<string>();
  for (const d of docs.values())
    if (d.rel === `skills/${d.skill}/SKILL.md`) skills.add(d.skill);
  assert.ok(skills.has("challenge"));
  assert.deepEqual(stepFindings(docs, skills), []);
});
