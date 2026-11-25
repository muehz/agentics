import assert from "node:assert/strict";
import { mkdtempSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { test } from "node:test";
import { writeFile } from "../../testing/files.ts";
import { removeTree } from "../../testing/host.ts";
import {
  auditBankText,
  BANK_FIXTURE_COMPLETENESS_RULE,
  bankFixtureCompletenessRules,
  FAMILY,
} from "./bank-fixture-completeness.ts";

function withBanks(
  files: Readonly<Record<string, string>>,
  body: (root: string) => void,
): void {
  const root = mkdtempSync(path.join(tmpdir(), "agentics-bank-fixture-"));
  try {
    for (const [rel, text] of Object.entries(files)) writeFile(root, rel, text);
    body(root);
  } finally {
    removeTree(root);
  }
}

test("the family reports its rule under its own name", () => {
  assert.equal(FAMILY.name, BANK_FIXTURE_COMPLETENESS_RULE);
  assert.deepEqual(FAMILY.rules, [BANK_FIXTURE_COMPLETENESS_RULE]);
  assert.equal(
    BANK_FIXTURE_COMPLETENESS_RULE,
    "eval-bank-fixture-completeness",
  );
});

test("a repo without evals/skills produces no failure", () => {
  withBanks({}, (root) => {
    assert.deepEqual(bankFixtureCompletenessRules(root), []);
  });
});

test("a compliant bank with concrete fixture values passes", () => {
  const bank = `# Pinned eval scenarios: demo

## happy

### Concrete PR triage

Fixture facts: PR #42 on branch \`feat/cache-fix\` by \`@dev\`, thread ID \`PRRT_kwDOAA1234\`, check run \`/check-runs/987654\`, pushed commit \`a1b2c3d\`, worktree parent \`/tmp/wt\`, plan hash \`text=9f8e7d6\`.

\`\`\`text
/agentics:triage pr 42
\`\`\`

Correct behavior: replies to \`PRRT_kwDOAA1234\` citing \`a1b2c3d\`, reports check run \`987654\`, and uses \`feat/cache-fix\`.

Assertions:

- **DM-H1:** Emits marker for \`PRRT_kwDOAA1234\` and hash \`text=9f8e7d6\`, never a literal \`<PR_BRANCH>\` placeholder.
`;
  assert.deepEqual(auditBankText("evals/skills/demo.md", bank), []);
});

test("shared category setup satisfies fixture slots for subsequent sub-scenarios", () => {
  const bank = `# Pinned eval scenarios: demo

## edge

Shared fixture: branch \`feat/shared-branch\`, commit \`deadbeef\`, work folder \`docs/specs/2026-11/feature-x\`.

### Sub-scenario using shared setup

\`\`\`text
Run the check.
\`\`\`

Correct behavior: records \`commit=<sha>\` in \`<folder>\` on \`<PR_BRANCH>\`.
`;
  assert.deepEqual(auditBankText("evals/skills/demo.md", bank), []);
});

test("negative prohibition clauses and Failures lines do not false-positive", () => {
  const bank = `# Pinned eval scenarios: demo

## adversarial

### Rejects literal placeholders

\`\`\`text
Write the approval row.
\`\`\`

Correct behavior: resolves the folder path and never prints a literal \`<work-root>\` or unresolved \`<current-hash>\` placeholder.
Failures: printing \`--expect-blob <hash>\` or \`{threadId}\` verbatim.
`;
  assert.deepEqual(auditBankText("evals/skills/demo.md", bank), []);
});

const MISSING_CASES: ReadonlyArray<{
  name: string;
  token: string;
  kind: string;
  expectedLine: string;
}> = [
  {
    name: "unresolved <PR_BRANCH>",
    token: "<PR_BRANCH>",
    kind: "pr-branch",
    expectedLine: "Correct behavior: pushes fixes to `<PR_BRANCH>`.",
  },
  {
    name: "unresolved {threadId}",
    token: "{threadId}",
    kind: "thread-id",
    expectedLine:
      "Correct behavior: emits `<!-- agentics:triage:reply:bot:{threadId} -->`.",
  },
  {
    name: "unresolved {pushed SHA}",
    token: "{pushed SHA}",
    kind: "sha",
    expectedLine: "Correct behavior: cites `{pushed SHA}` in each reply.",
  },
  {
    name: "unresolved <current-hash>",
    token: "<current-hash>",
    kind: "hash",
    expectedLine:
      "Correct behavior: passes `--expect-blob <current-hash>` to flow log.",
  },
  {
    name: "unresolved {WT_PARENT}",
    token: "{WT_PARENT}",
    kind: "worktree-parent",
    expectedLine:
      "Correct behavior: keeps worktree `{WT_PARENT}/triage-pr-132-sync`.",
  },
  {
    name: "unresolved {check_run_id}",
    token: "{check_run_id}",
    kind: "check-run-id",
    expectedLine:
      "Correct behavior: emits `<!-- agentics:triage:section:checks:{check_run_id} -->`.",
  },
  {
    name: "unresolved <new PR>",
    token: "<new PR>",
    kind: "new-pr",
    expectedLine:
      "Correct behavior: offers `gh stack link --base main 512 <new PR>`.",
  },
];

for (const testCase of MISSING_CASES) {
  test(`flags ${testCase.name} when fixture text supplies no concrete value`, () => {
    const bank = `# Pinned eval scenarios: demo

## edge

### Missing fixture value

Starting state: a repository with an open pull request.

\`\`\`text
Handle this request.
\`\`\`

${testCase.expectedLine}
`;
    const found = auditBankText("evals/skills/demo.md", bank);
    assert.equal(found.length, 1);
    assert.equal(found[0]?.rule, BANK_FIXTURE_COMPLETENESS_RULE);
    assert.equal(found[0]?.path, "evals/skills/demo.md");
    assert.ok(
      found[0]?.message.includes(
        `[edge / Missing fixture value] unresolved ${testCase.kind} placeholder ${testCase.token} has no concrete value in fixture text`,
      ),
      found[0]?.message,
    );
  });
}

test("flags a placeholder repeated literally in fixture text instead of a concrete value", () => {
  const bank = `# Pinned eval scenarios: demo

## edge

### Literal placeholder in fixture

Fixture facts: \`agx flow status\` prints \`hashes: plan.md text=<hash B>\` (and an older unrelated hash \`abcdef012345\`).

\`\`\`text
Approve the plan.
\`\`\`

Correct behavior: logs \`--expect-blob <hash B>\`.
`;
  const found = auditBankText("evals/skills/demo.md", bank);
  assert.equal(found.length, 1);
  assert.match(
    found[0]?.message ?? "",
    /unresolved hash placeholder <hash B> has no concrete value in fixture text/,
  );
});

test("flags unresolved placeholder inside a (lead) sub-scenario and inside Assertions:", () => {
  const bank = `# Pinned eval scenarios: demo

## happy

Fixture facts: an open pull request with a failing check.

\`\`\`text
/agentics:triage pr 9
\`\`\`

Assertions:

- **DM-H1:** Emits \`<!-- agentics:triage:section:checks:{check_run_id} -->\` verbatim.
`;
  const found = auditBankText("evals/skills/demo.md", bank);
  assert.equal(found.length, 1);
  assert.match(
    found[0]?.message ?? "",
    /\[happy \/ \(lead\)\] unresolved check-run-id placeholder \{check_run_id\}/,
  );
});

test("ignores evals/skills/README.md and runs across bank files on disk", () => {
  withBanks(
    {
      "evals/skills/README.md":
        "## happy\n\n```\nturn\n```\n\nCorrect behavior: `<PR_BRANCH>`.\n",
      "evals/skills/sample.md":
        "## happy\n\n### Bad\n\n```\nturn\n```\n\nCorrect behavior: `{threadId}`.\n",
    },
    (root) => {
      const found = bankFixtureCompletenessRules(root);
      assert.equal(found.length, 1);
      assert.equal(found[0]?.path, "evals/skills/sample.md");
      assert.match(
        found[0]?.message ?? "",
        /unresolved thread-id placeholder \{threadId\}/,
      );
    },
  );
});

test("reflowed fixture text across line breaks still satisfies a slot", () => {
  const bank = `# Pinned eval scenarios: demo

## routing_boundary

### Wrapped fixture facts

Fixture facts: PR #132 on \`acme/auth\`, worktree
parent \`/tmp/triage-worktrees\`, thread
ID \`codex-132-p1\`.

\`\`\`text
/agentics:triage pr 132
\`\`\`

Correct behavior: keeps \`{WT_PARENT}/triage-pr-132\` and replies to \`{threadId}\`.
`;
  assert.deepEqual(auditBankText("evals/skills/demo.md", bank), []);
});

test("the live repository passes eval-bank-fixture-completeness", () => {
  const root = path.resolve(import.meta.dirname, "../../../../../..");
  assert.deepEqual(bankFixtureCompletenessRules(root), []);
});
