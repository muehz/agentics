import assert from "node:assert/strict";
import { mkdirSync, mkdtempSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { test } from "node:test";
import { writeFile } from "../../testing/files.ts";
import { isFileErrorOfKind } from "./files.ts";
import {
  BRANCH_WORKFLOW_PATH,
  EXECUTE_SKILL_PATH,
  FAMILY,
  FIX_ENTRY_PATH,
  FLOW_STATE_PATH,
  RECORD_KEEPING_FILES,
  RECORD_KEEPING_RULE,
  recordKeepingRules,
} from "./record-keeping.ts";
import { removeTree } from "../../testing/host.ts";

const AUDIT_CALL = "agx flow audit";
const QUESTION = "are these intended";
const STOP_QUESTION = "Stop for one question on any unrowed commit";
const CARRY_FORWARD = "carries that approval forward";
const AUDIT_LINK = "references/fix-entry.md#standalone-fix-commit-audit";
const AUDIT_HEADING = "## Standalone fix commit audit";
const LINK_LINE = `Run the [standalone fix commit audit](${AUDIT_LINK}).\n`;
const HAND_AUDIT = "gate calls the helper's audit instead of a hand procedure";
const NO_EXECUTE_ROW =
  "gate writes no `execute` row; a commit's row comes from the author who made it";
const LINKED =
  "`execute`'s boundary check and completion check each send a standalone fix to the fix entry's audit";
const HEADING_KEPT =
  "the fix entry keeps the audit section that `execute`'s SKILL.md links to";

const CLEAN: Readonly<Record<string, string>> = {
  [BRANCH_WORKFLOW_PATH]:
    "# Branch workflow\n\nRun `agx flow audit` and report each finding.\n",
  [FIX_ENTRY_PATH]: `# Fix entry\n\n${AUDIT_HEADING}\n\nAudit the fix with \`agx flow audit\`.\n`,
  [EXECUTE_SKILL_PATH]: `# Execute\n\nRecord what can be recorded and report the rest.\n\n${LINK_LINE}\n${LINK_LINE}`,
  [FLOW_STATE_PATH]:
    "# Flow state\n\nAn edit after approval needs a new one.\n",
};

/** A temporary repository holding the four files, with `overrides` replacing a file's text or, as null, removing it. */
function withRepo<T>(
  overrides: Readonly<Record<string, string | null>>,
  body: (root: string) => T,
): T {
  const root = mkdtempSync(path.join(tmpdir(), "agentics-recordkeeping-"));
  try {
    for (const rel of RECORD_KEEPING_FILES) {
      const text = Object.hasOwn(overrides, rel) ? overrides[rel] : CLEAN[rel];
      if (text === null || text === undefined)
        mkdirSync(path.dirname(path.join(root, ...rel.split("/"))), {
          recursive: true,
        });
      else writeFile(root, rel, text);
    }
    return body(root);
  } finally {
    removeTree(root);
  }
}

const brief = (root: string): Array<[string, string]> =>
  recordKeepingRules(root).map((one) => {
    assert.equal(one.rule, RECORD_KEEPING_RULE);
    return [one.path, one.message];
  });

test("the family reports its one rule under its own name", () => {
  assert.equal(FAMILY.name, "gate-record-keeping");
  assert.deepEqual(FAMILY.rules, [RECORD_KEEPING_RULE]);
  assert.equal(RECORD_KEEPING_RULE, "gate-record-keeping");
});

test("four compliant files give no failure", () => {
  withRepo({}, (root) => assert.deepEqual(brief(root), []));
});

test("the live repository passes the rule", () => {
  const root = path.resolve(import.meta.dirname, "../../../../../..");
  assert.deepEqual(recordKeepingRules(root), []);
});

for (const rel of [BRANCH_WORKFLOW_PATH, FIX_ENTRY_PATH]) {
  test(`${rel} without the audit call is one failure that names the helper's audit`, () => {
    const text = (CLEAN[rel] ?? "").replace(
      `\`${AUDIT_CALL}\``,
      "the commits by hand",
    );
    assert.notEqual(text, CLEAN[rel]);
    withRepo({ [rel]: text }, (root) =>
      assert.deepEqual(brief(root), [
        [
          rel,
          `${rel} no longer carries "${AUDIT_CALL}"; gate and \`execute fix\` account for the branch by calling the helper's audit instead of a hand procedure`,
        ],
      ]),
    );
  });
}

const NO_QUESTION =
  "gate and `execute fix` ask no record-keeping question; they record what can be recorded and report the rest";

const FORBIDDEN: ReadonlyArray<[string, string, string]> = [
  [BRANCH_WORKFLOW_PATH, QUESTION, NO_QUESTION],
  [BRANCH_WORKFLOW_PATH, "list the rest yourself", HAND_AUDIT],
  [BRANCH_WORKFLOW_PATH, "at most 20 ids", HAND_AUDIT],
  [BRANCH_WORKFLOW_PATH, "writes late", NO_EXECUTE_ROW],
  [BRANCH_WORKFLOW_PATH, "gate writes an `execute`", NO_EXECUTE_ROW],
  [BRANCH_WORKFLOW_PATH, "gate appends an `execute`", NO_EXECUTE_ROW],
  [FIX_ENTRY_PATH, QUESTION, NO_QUESTION],
  [EXECUTE_SKILL_PATH, QUESTION, NO_QUESTION],
  [EXECUTE_SKILL_PATH, STOP_QUESTION, NO_QUESTION],
  [
    FLOW_STATE_PATH,
    CARRY_FORWARD,
    "gate never carries an approval over its own edit",
  ],
];

for (const [rel, literal, decision] of FORBIDDEN) {
  test(`${rel} carrying "${literal}" is one failure that names the decision`, () => {
    withRepo({ [rel]: `${CLEAN[rel]}\nAsk: ${literal}?\n` }, (root) =>
      assert.deepEqual(brief(root), [
        [rel, `${rel} carries "${literal}" again; ${decision}`],
      ]),
    );
  });
}

test("a forbidden sentence still matches when it is reflowed or recapitalized", () => {
  withRepo(
    {
      [EXECUTE_SKILL_PATH]: `${CLEAN[EXECUTE_SKILL_PATH]}\nStop for one question\non any Unrowed commit.\n`,
    },
    (root) => assert.equal(brief(root).length, 1),
  );
});

for (const count of [0, 1, 3]) {
  test(`${EXECUTE_SKILL_PATH} linking the standalone audit ${count} times is one failure`, () => {
    withRepo(
      {
        [EXECUTE_SKILL_PATH]: `# Execute\n\nRecord what can be recorded.\n\n${LINK_LINE.repeat(count)}`,
      },
      (root) =>
        assert.deepEqual(brief(root), [
          [
            EXECUTE_SKILL_PATH,
            `${EXECUTE_SKILL_PATH} carries "${AUDIT_LINK}" ${count} times, not 2; ${LINKED}`,
          ],
        ]),
    );
  });
}

for (const heading of [
  "### Standalone fix commit audit",
  "## Standalone fix audit",
  "Run the standalone fix commit audit.",
]) {
  test(`${FIX_ENTRY_PATH} with "${heading}" instead of the audit heading is one failure`, () => {
    withRepo(
      {
        [FIX_ENTRY_PATH]: `# Fix entry\n\n${heading}\n\nAudit the fix with \`agx flow audit\`.\n`,
      },
      (root) =>
        assert.deepEqual(brief(root), [
          [
            FIX_ENTRY_PATH,
            `${FIX_ENTRY_PATH} no longer has the line "${AUDIT_HEADING}"; ${HEADING_KEPT}`,
          ],
        ]),
    );
  });
}

for (const rel of RECORD_KEEPING_FILES) {
  test(`a missing ${rel} is the one read-error failure for that file`, () => {
    withRepo({ [rel]: null }, (root) => {
      const found = brief(root);
      assert.equal(found.length, 1);
      const [where, message] = found[0] ?? ["", ""];
      assert.equal(where, rel);
      assert.ok(
        message.startsWith(`cannot read ${rel}, which carries the rule that `),
        message,
      );
      assert.ok(message.endsWith(`: ${rel}: does not exist`), message);
    });
  });
}

test("a read failure other than a missing file is an audit error", () => {
  withRepo({ [FLOW_STATE_PATH]: null }, (root) => {
    mkdirSync(path.join(root, ...FLOW_STATE_PATH.split("/")));
    assert.throws(
      () => recordKeepingRules(root),
      (error: unknown) => {
        assert.ok(error instanceof Error);
        assert.match(
          error.message,
          new RegExp(
            `^cannot read ${FLOW_STATE_PATH}.*: is not a regular file$`,
          ),
        );
        assert.ok(isFileErrorOfKind(error.cause, "not-a-file"));
        return true;
      },
    );
  });
});
