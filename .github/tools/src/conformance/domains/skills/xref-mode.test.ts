import assert from "node:assert/strict";
import path from "node:path";
import { test } from "node:test";
import { fileURLToPath } from "node:url";
import { repoRoot } from "../../check.ts";
import { readDeclarations, type SkillDeclaration } from "./skill-meta.ts";
import { FLAG_RULE, MODE_RULE, modeFindings } from "./xref-mode.ts";
import { type SkillDoc, skillDocs } from "./xref-md.ts";

function declaration(
  skill: string,
  modes: string[] | undefined,
  aliases: [string, string][],
  flags: string[],
): SkillDeclaration {
  return {
    skill,
    rel: `skills/${skill}/SKILL.md`,
    frontmatter: undefined,
    surfaces: [],
    modes: modes === undefined ? undefined : new Set(modes),
    aliases: new Map(aliases),
    flags: new Set(flags),
  };
}

const DECLARATIONS: ReadonlyMap<string, SkillDeclaration> = new Map([
  ["gate", declaration("gate", ["branch"], [["pr", "branch"]], ["--no-docs"])],
  ["intent", declaration("intent", undefined, [], [])],
]);

const REL = "skills/y/SKILL.md";

function docs(text: string, scanned = true): Map<string, SkillDoc> {
  return new Map([[REL, { rel: REL, skill: "y", text, scanned }]]);
}

/** The rule and key of each finding for one line of prose, which sits on line 3 of the file. */
function findingsFor(line: string): [string, string][] {
  const findings = modeFindings(docs(`# y\n\n${line}\n`), DECLARATIONS);
  for (const f of findings) assert.equal(f.path, REL);
  return findings.map((f) => [f.rule, f.key]);
}

const CASES: [string, [string, string][]][] = [
  ["run `/agentics:gate branch`", []],
  ["use /agentics:gate pr now", []],
  ["use /agentics:gate nosuch", [[MODE_RULE, "/agentics:gate nosuch"]]],
  [
    "then /agentics:intent captures",
    [[MODE_RULE, "/agentics:intent captures"]],
  ],
  ["`/agentics:intent` captures", []],
  ["intent/gate decisions", []],
  ["/agentics:gate nosuch", [[MODE_RULE, "/agentics:gate nosuch"]]],
  ["(/agentics:gate nosuch)", [[MODE_RULE, "/agentics:gate nosuch"]]],
  ["`/agentics:gate branch --no-docs`", []],
  [
    "`/agentics:gate branch --nosuch`",
    [[FLAG_RULE, "/agentics:gate --nosuch"]],
  ],
  ["/agentics:nosuch x", []],
  ["/agentics:gate branch --nosuch", []],
  ["`/agentics:gatekeeper --nosuch`", []],
  ["`/agentics:gate branch` then --nosuch", []],
  ["use $agentics:gate nosuch", [[MODE_RULE, "/agentics:gate nosuch"]]],
  [
    "`$agentics:gate branch --nosuch`",
    [[FLAG_RULE, "/agentics:gate --nosuch"]],
  ],
  ["use /gate nosuch", []],
  ["use agentics:gate nosuch", []],
  [
    "use /agentics:gate a and /agentics:gate b",
    [
      [MODE_RULE, "/agentics:gate a"],
      [MODE_RULE, "/agentics:gate b"],
    ],
  ],
];

for (const [line, want] of CASES) {
  test(`modeFindings: ${line}`, () => {
    assert.deepEqual(findingsFor(line), want);
  });
}

test("a mode the target does not declare gets the exact message", () => {
  assert.deepEqual(
    modeFindings(docs("# y\n\nuse /agentics:gate nosuch\n"), DECLARATIONS),
    [
      {
        rule: MODE_RULE,
        path: REL,
        key: "/agentics:gate nosuch",
        message:
          "line 3: /agentics:gate nosuch: nosuch is not in gate's metadata.modes or metadata.mode-aliases",
      },
    ],
  );
});

test("a target without metadata.modes gets the no-modes message", () => {
  assert.deepEqual(
    modeFindings(docs("# y\n\nthen /agentics:intent captures\n"), DECLARATIONS),
    [
      {
        rule: MODE_RULE,
        path: REL,
        key: "/agentics:intent captures",
        message:
          "line 3: /agentics:intent captures: intent declares no metadata.modes",
      },
    ],
  );
});

test("a flag the target does not declare gets the exact message", () => {
  assert.deepEqual(
    modeFindings(
      docs("# y\n\n`/agentics:gate branch --nosuch`\n"),
      DECLARATIONS,
    ),
    [
      {
        rule: FLAG_RULE,
        path: REL,
        key: "/agentics:gate --nosuch",
        message: "line 3: --nosuch is not in gate's metadata.flags",
      },
    ],
  );
});

test("no declarations give no findings", () => {
  assert.deepEqual(modeFindings(docs("# y\n\n/x y\n"), new Map()), []);
});

test("a reference inside a fence is not read", () => {
  const text = "# y\n\n```\n/gate nosuch\n```\n";
  assert.deepEqual(modeFindings(docs(text), DECLARATIONS), []);
});

test("a doc that is not scanned is not read", () => {
  assert.deepEqual(
    modeFindings(docs("# y\n\n/gate nosuch\n", false), DECLARATIONS),
    [],
  );
});

test("this checkout's findings name only the mode and flag rules under skills/", () => {
  const root = repoRoot(path.dirname(fileURLToPath(import.meta.url)));
  assert.notEqual(root, "");
  const { declarations } = readDeclarations(root);
  assert.ok(declarations.has("gate"));
  for (const f of modeFindings(skillDocs(root), declarations)) {
    assert.ok(f.rule === MODE_RULE || f.rule === FLAG_RULE, f.rule);
    assert.ok(f.path.startsWith("skills/"), f.path);
  }
});
