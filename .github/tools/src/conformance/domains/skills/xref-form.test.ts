import assert from "node:assert/strict";
import path from "node:path";
import { test } from "node:test";
import { fileURLToPath } from "node:url";
import { repoRoot } from "../../check.ts";
import { readDeclarations } from "./skill-meta.ts";
import {
  BARE_COMMAND_RULE,
  CODEX_FORM_RULE,
  commandFormFindings,
} from "./xref-form.ts";
import { type SkillDoc, skillDocs } from "./xref-md.ts";

const SKILLS: ReadonlySet<string> = new Set(["gate", "git", "gh"]);
const REF = "skills/y/references/x.md";
const SKILL = "skills/y/SKILL.md";

function docs(
  rel: string,
  text: string,
  scanned = true,
): Map<string, SkillDoc> {
  return new Map([[rel, { rel, skill: "y", text, scanned }]]);
}

/** The rule and key of each finding for one line of reference prose. */
function bareFor(line: string): [string, string][] {
  return commandFormFindings(docs(REF, `# x\n\n${line}\n`), SKILLS).map((f) => [
    f.rule,
    f.key,
  ]);
}

const BARE_CASES: [string, [string, string][]][] = [
  ["run `/gate branch`", [[BARE_COMMAND_RULE, "/gate"]]],
  ["/gate at line start", [[BARE_COMMAND_RULE, "/gate"]]],
  ["(/git push)", [[BARE_COMMAND_RULE, "/git"]]],
  ["[`/gh`](../gh/SKILL.md)", [[BARE_COMMAND_RULE, "/gh"]]],
  ["| /gate | x |", [[BARE_COMMAND_RULE, "/gate"]]],
  ["**`/gate change`**", [[BARE_COMMAND_RULE, "/gate"]]],
  ["ends with /gate.", [[BARE_COMMAND_RULE, "/gate"]]],
  ["run `/agentics:gate branch`", []],
  ["run `$agentics:gate branch`", []],
  ["intent/gate decisions", []],
  ["see skills/gate/SKILL.md", []],
  ["a /gate-branch identifier", []],
  ["the /gate/x path", []],
  ["the /gate.md file", []],
  ["a /gatekeeper command", []],
  ["a /nosuch command", []],
  [
    "use /gate and /git",
    [
      [BARE_COMMAND_RULE, "/gate"],
      [BARE_COMMAND_RULE, "/git"],
    ],
  ],
];

for (const [line, want] of BARE_CASES) {
  test(`commandFormFindings bare: ${line}`, () => {
    assert.deepEqual(bareFor(line), want);
  });
}

test("a bare command gets the exact message", () => {
  assert.deepEqual(
    commandFormFindings(docs(REF, "# x\n\nrun /gate now\n"), SKILLS),
    [
      {
        rule: BARE_COMMAND_RULE,
        path: REF,
        key: "/gate",
        message: "line 3: /gate is a bare skill command; write /agentics:gate",
      },
    ],
  );
});

test("a bare command inside a fence is not read", () => {
  assert.deepEqual(
    commandFormFindings(docs(REF, "# x\n\n```\n/gate\n```\n"), SKILLS),
    [],
  );
});

test("a doc that is not scanned is not read", () => {
  assert.deepEqual(
    commandFormFindings(docs(REF, "# x\n\n/gate\n", false), SKILLS),
    [],
  );
});

test("a SKILL.md with a qualified command and no Codex form is reported", () => {
  assert.deepEqual(
    commandFormFindings(docs(SKILL, "# y\n\nrun `/agentics:gate`\n"), SKILLS),
    [
      {
        rule: CODEX_FORM_RULE,
        path: SKILL,
        key: "codex-form",
        message:
          "SKILL.md prints /agentics:<skill> commands but never names the Codex form $agentics:<skill>",
      },
    ],
  );
});

test("a SKILL.md that names the Codex form passes", () => {
  const text =
    "# y\n\nrun `/agentics:gate`. On Codex, type `$agentics:<skill>`.\n";
  assert.deepEqual(commandFormFindings(docs(SKILL, text), SKILLS), []);
});

test("a SKILL.md without qualified commands needs no Codex form", () => {
  assert.deepEqual(
    commandFormFindings(docs(SKILL, "# y\n\nno commands\n"), SKILLS),
    [],
  );
});

test("a reference file needs no Codex form of its own", () => {
  assert.deepEqual(
    commandFormFindings(docs(REF, "# x\n\nrun `/agentics:gate`\n"), SKILLS),
    [],
  );
});

test("a qualified command only inside a fence needs no Codex form", () => {
  const text = "# y\n\n```\n/agentics:gate\n```\n";
  assert.deepEqual(commandFormFindings(docs(SKILL, text), SKILLS), []);
});

const CODEX_ELSEWHERE: [string, string][] = [
  ["fenced code", "# y\n\nrun `/agentics:gate`\n\n```\n$agentics:gate\n```\n"],
  [
    "frontmatter",
    "---\nname: y\ndescription: try $agentics:gate\n---\n# y\n\nrun `/agentics:gate`\n",
  ],
];

for (const [where, text] of CODEX_ELSEWHERE) {
  test(`a Codex form only in ${where} does not satisfy the rule`, () => {
    assert.deepEqual(
      commandFormFindings(docs(SKILL, text), SKILLS).map((f) => f.rule),
      [CODEX_FORM_RULE],
    );
  });
}

test("no skills give no findings", () => {
  assert.deepEqual(
    commandFormFindings(docs(REF, "# x\n\n/gate\n"), new Set()),
    [],
  );
});

test("this checkout prints no bare skill command and every SKILL.md names the Codex form", () => {
  const root = repoRoot(path.dirname(fileURLToPath(import.meta.url)));
  assert.notEqual(root, "");
  const { declarations } = readDeclarations(root);
  assert.ok(declarations.has("gate"));
  assert.deepEqual(
    commandFormFindings(skillDocs(root), new Set(declarations.keys())),
    [],
  );
});
