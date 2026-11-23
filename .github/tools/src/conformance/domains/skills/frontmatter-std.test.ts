import assert from "node:assert/strict";
import path from "node:path";
import { test } from "node:test";
import { fileURLToPath } from "node:url";
import { repoRoot } from "../../check.ts";
import type { KeyedFailure } from "./baseline.ts";
import { parseFrontmatter } from "./frontmatter.ts";
import {
  frontmatterFindings,
  KEY_RULE,
  SURFACE_DOCS,
  VALUE_RULE,
} from "./frontmatter-std.ts";
import {
  readDeclarations,
  SHIPPED_SURFACES,
  type SkillDeclaration,
  type Surface,
} from "./skill-meta.ts";

const REL = "skills/x/SKILL.md";

function declaration(
  extra: string,
  surfaces: Surface[] = ["claude-code"],
): SkillDeclaration {
  return {
    skill: "x",
    rel: REL,
    frontmatter: parseFrontmatter(`name: x\ndescription: Does x.\n${extra}`),
    surfaces,
    modes: undefined,
    aliases: new Map(),
    flags: new Set(),
  };
}

function findings(extra: string, surfaces?: Surface[]): KeyedFailure[] {
  const found = frontmatterFindings([declaration(extra, surfaces)]);
  for (const f of found) assert.equal(f.path, REL);
  return found;
}

function keys(extra: string, surfaces?: Surface[]): string[] {
  return findings(extra, surfaces).map((f) => `${f.rule} ${f.key}`);
}

const CASES: [string, string, Surface[] | undefined, string[]][] = [
  ["unknown key", 'version: "1.0.0"', undefined, [`${KEY_RULE} version`]],
  [
    "key no listed surface documents",
    "context: fork",
    ["codex"],
    [`${KEY_RULE} context`],
  ],
  [
    "bad value on two documenting surfaces",
    "context: inline",
    ["claude-code", "vscode"],
    [`${VALUE_RULE} context: claude-code`, `${VALUE_RULE} context: vscode`],
  ],
  [
    "string where a boolean belongs",
    'user-invocable: "yes"',
    undefined,
    [`${VALUE_RULE} user-invocable: claude-code`],
  ],
  [
    "allowed-tools list",
    "allowed-tools:\n  - Bash(gh pr view:*)\n  - Read",
    undefined,
    [],
  ],
  [
    "allowed-tools string split outside parentheses",
    'allowed-tools: "Bash(git add *) Read, Grep"',
    undefined,
    [],
  ],
  [
    "allowed-tools bad name",
    'allowed-tools: "1bad"',
    undefined,
    [`${VALUE_RULE} allowed-tools: claude-code`],
  ],
  [
    "allowed-tools empty",
    'allowed-tools: ""',
    undefined,
    [`${VALUE_RULE} allowed-tools: claude-code`],
  ],
  [
    "effort out of range",
    "effort: extreme",
    undefined,
    [`${VALUE_RULE} effort: claude-code`],
  ],
  ["license on copilot-cli", "license: MIT", ["copilot-cli"], []],
  [
    "unknown shell",
    "shell: zsh",
    undefined,
    [`${VALUE_RULE} shell: claude-code`],
  ],
  [
    "compatibility too long",
    `compatibility: ${"a".repeat(501)}`,
    undefined,
    [`${VALUE_RULE} compatibility: claude-code`],
  ],
  [
    "compatibility at the limit",
    `compatibility: ${"a".repeat(500)}`,
    undefined,
    [],
  ],
  [
    "disallowed-tools bad name",
    'disallowed-tools: "1bad"',
    undefined,
    [`${VALUE_RULE} disallowed-tools: claude-code`],
  ],
  [
    "metadata not a map",
    "metadata: x",
    undefined,
    [`${VALUE_RULE} metadata: claude-code`],
  ],
  [
    "empty argument-hint",
    'argument-hint: ""',
    undefined,
    [`${VALUE_RULE} argument-hint: claude-code`],
  ],
  [
    "folded argument-hint",
    "argument-hint: >-\n  first line\n  second line",
    undefined,
    [],
  ],
  [
    "valid claude-code keys",
    [
      "user-invocable: false",
      "disable-model-invocation: true",
      "effort: xhigh",
      "shell: powershell",
      "context: fork",
      "metadata:\n  surfaces: [claude-code]",
    ].join("\n"),
    undefined,
    [],
  ],
];

for (const [name, extra, surfaces, want] of CASES) {
  test(`frontmatterFindings: ${name}`, () => {
    assert.deepEqual(keys(extra, surfaces), want);
  });
}

test("frontmatterFindings: the key message names the listed surfaces", () => {
  assert.deepEqual(findings("context: fork", ["codex", "copilot-cli"]), [
    {
      rule: KEY_RULE,
      path: REL,
      key: "context",
      message:
        'top-level key "context" is documented by none of this skill\'s surfaces (codex, copilot-cli)',
    },
  ]);
});

test("frontmatterFindings: the value message names the rule and source", () => {
  assert.deepEqual(findings("context: inline", ["vscode"]), [
    {
      rule: VALUE_RULE,
      path: REL,
      key: "context: vscode",
      message: `context must be exactly fork on vscode (${SURFACE_DOCS.vscode.source})`,
    },
  ]);
});

test("frontmatterFindings: skips a skill with no frontmatter or no surface", () => {
  const none: SkillDeclaration = {
    ...declaration('version: "1"'),
    frontmatter: undefined,
  };
  assert.deepEqual(frontmatterFindings([none]), []);
  assert.deepEqual(frontmatterFindings([declaration('version: "1"', [])]), []);
});

test("SURFACE_DOCS: one row for each shipped surface with recorded read dates", () => {
  assert.deepEqual(Object.keys(SURFACE_DOCS).sort(), [...SHIPPED_SURFACES]);
  assert.ok(SHIPPED_SURFACES.includes("antigravity"));
  const expectedReads: Record<Surface, string> = {
    antigravity: "2026-11-20",
    "claude-code": "2026-11-10",
    codex: "2026-11-10",
    "copilot-cli": "2026-11-10",
    vscode: "2026-11-10",
  };
  for (const surface of SHIPPED_SURFACES) {
    const row = SURFACE_DOCS[surface];
    assert.equal(row.read, expectedReads[surface]);
    assert.match(row.source, /^https:\/\//);
    assert.ok(row.keys.includes("name"));
    assert.ok(row.keys.includes("description"));
  }
  assert.deepEqual(SURFACE_DOCS.antigravity, {
    source: "https://antigravity.google/docs/build-plugins",
    read: "2026-11-20",
    keys: ["name", "description"],
  });
});

test("frontmatterFindings: the checkout gives only frontmatter findings", () => {
  const root = repoRoot(path.dirname(fileURLToPath(import.meta.url)));
  const { declarations } = readDeclarations(root);
  for (const f of frontmatterFindings(declarations.values()))
    assert.ok(f.rule === KEY_RULE || f.rule === VALUE_RULE, f.rule);
});

test("frontmatterFindings: a key holding a newline and an escape character is printed on one printable line", () => {
  const found = findings('"a\\nFAIL skills [fake] p: m\\e[2K": 1\n');
  assert.deepEqual(
    found.map((f) => [f.rule, f.key]),
    [[KEY_RULE, "a\\u000aFAIL skills [fake] p: m\\u001b[2K"]],
  );
  assert.equal(
    found[0]?.message,
    'top-level key "a\\u000aFAIL skills [fake] p: m\\u001b[2K" is documented by none of this skill\'s surfaces (claude-code)',
  );
});
