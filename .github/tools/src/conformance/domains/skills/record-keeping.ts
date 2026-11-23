import { errorMessage, type Failure } from "../../check.ts";
import { checkPinnedText, type PinMismatch } from "../../pinned-text.ts";
import { isFileErrorOfKind } from "./files.ts";
import type { Family } from "./index.ts";

/** The one rule id this family emits. */
export const RECORD_KEEPING_RULE = "gate-record-keeping";

export const BRANCH_WORKFLOW_PATH =
  "skills/gate/references/branch/review-and-audit.md";
export const FIX_ENTRY_PATH = "skills/execute/references/fix-entry.md";
export const EXECUTE_SKILL_PATH = "skills/execute/SKILL.md";
export const FLOW_STATE_PATH = "skills/execute/references/flow-state.md";

/** Every file the family reads, in the order it reads them. */
export const RECORD_KEEPING_FILES: readonly string[] = [
  BRANCH_WORKFLOW_PATH,
  FIX_ENTRY_PATH,
  EXECUTE_SKILL_PATH,
  FLOW_STATE_PATH,
];

const AUDIT_CALL =
  "gate and `execute fix` account for the branch by calling the helper's audit instead of a hand procedure";
const NO_QUESTION =
  "gate and `execute fix` ask no record-keeping question; they record what can be recorded and report the rest";
const NO_CARRY_FORWARD = "gate never carries an approval over its own edit";
const NO_HAND_AUDIT =
  "gate calls the helper's audit instead of a hand procedure";
const NO_EXECUTE_ROW =
  "gate writes no `execute` row; a commit's row comes from the author who made it";
const AUDIT_LINKED =
  "`execute`'s boundary check and completion check each send a standalone fix to the fix entry's audit";
const AUDIT_HEADING_KEPT =
  "the fix entry keeps the audit section that `execute`'s SKILL.md links to";

/**
 * What a pin checks: `required` and `forbidden` look for the literal in the whitespace-folded, lowercased text; `count`
 * requires exactly `times` occurrences there; `line` requires a line that equals the literal exactly.
 */
export type RecordKeepingCheck =
  | { kind: "required" }
  | { kind: "forbidden" }
  | { kind: "count"; times: number }
  | { kind: "line" };

/** One check on one file, with the decision the check guards. */
export type RecordKeepingPin = {
  rel: string;
  literal: string;
  check: RecordKeepingCheck;
  decision: string;
};

const REQUIRED: RecordKeepingCheck = { kind: "required" };
const FORBIDDEN: RecordKeepingCheck = { kind: "forbidden" };

/** The pins, in the order the family checks them. */
export const RECORD_KEEPING_PINS: readonly RecordKeepingPin[] = [
  {
    rel: BRANCH_WORKFLOW_PATH,
    literal: "agx flow audit",
    check: REQUIRED,
    decision: AUDIT_CALL,
  },
  {
    rel: BRANCH_WORKFLOW_PATH,
    literal: "are these intended",
    check: FORBIDDEN,
    decision: NO_QUESTION,
  },
  {
    rel: BRANCH_WORKFLOW_PATH,
    literal: "list the rest yourself",
    check: FORBIDDEN,
    decision: NO_HAND_AUDIT,
  },
  {
    rel: BRANCH_WORKFLOW_PATH,
    literal: "at most 20 ids",
    check: FORBIDDEN,
    decision: NO_HAND_AUDIT,
  },
  {
    rel: BRANCH_WORKFLOW_PATH,
    literal: "writes late",
    check: FORBIDDEN,
    decision: NO_EXECUTE_ROW,
  },
  {
    rel: BRANCH_WORKFLOW_PATH,
    literal: "gate writes an `execute`",
    check: FORBIDDEN,
    decision: NO_EXECUTE_ROW,
  },
  {
    rel: BRANCH_WORKFLOW_PATH,
    literal: "gate appends an `execute`",
    check: FORBIDDEN,
    decision: NO_EXECUTE_ROW,
  },
  {
    rel: FIX_ENTRY_PATH,
    literal: "agx flow audit",
    check: REQUIRED,
    decision: AUDIT_CALL,
  },
  {
    rel: FIX_ENTRY_PATH,
    literal: "## Standalone fix commit audit",
    check: { kind: "line" },
    decision: AUDIT_HEADING_KEPT,
  },
  {
    rel: FIX_ENTRY_PATH,
    literal: "are these intended",
    check: FORBIDDEN,
    decision: NO_QUESTION,
  },
  {
    rel: EXECUTE_SKILL_PATH,
    literal: "references/fix-entry.md#standalone-fix-commit-audit",
    check: { kind: "count", times: 2 },
    decision: AUDIT_LINKED,
  },
  {
    rel: EXECUTE_SKILL_PATH,
    literal: "are these intended",
    check: FORBIDDEN,
    decision: NO_QUESTION,
  },
  {
    rel: EXECUTE_SKILL_PATH,
    literal: "Stop for one question on any unrowed commit",
    check: FORBIDDEN,
    decision: NO_QUESTION,
  },
  {
    rel: FLOW_STATE_PATH,
    literal: "carries that approval forward",
    check: FORBIDDEN,
    decision: NO_CARRY_FORWARD,
  },
];

function violation(literal: string, rel: string, failure: PinMismatch): string {
  const quoted = JSON.stringify(literal);
  switch (failure.kind) {
    case "required":
      return `${rel} no longer carries ${quoted}`;
    case "forbidden":
      return `${rel} carries ${quoted} again`;
    case "count":
      return `${rel} carries ${quoted} ${failure.actual} times, not ${failure.expected}`;
    case "line":
      return `${rel} no longer has the line ${quoted}`;
  }
}

/**
 * Requires the helper's audit call in gate's branch workflow and in the fix entry, keeps `execute`'s two links to the
 * fix entry's audit section and that section's heading, and forbids the removed record-keeping question, gate's hand
 * audit and execute-row instructions, and the approval carry-forward. A missing file is one failure and its remaining
 * pins are skipped; any other read failure is an audit error.
 */
export function recordKeepingRules(
  repoRoot: string,
  read?: (rel: string) => string,
): Failure[] {
  return checkPinnedText(
    repoRoot,
    RECORD_KEEPING_PINS,
    {
      projection: "folded-insensitive",
      check: (pin) => pin.check,
      mismatch: (pin, rel, failure) => ({
        rule: RECORD_KEEPING_RULE,
        path: rel,
        message: `${violation(pin.literal, rel, failure)}; ${pin.decision}`,
      }),
      sourceError: (pin, rel, _action, error) => {
        const message = `cannot read ${rel}, which carries the rule that ${pin.decision}: ${errorMessage(error)}`;
        if (!isFileErrorOfKind(error, "missing"))
          throw new Error(message, { cause: error });
        return { rule: RECORD_KEEPING_RULE, path: rel, message };
      },
    },
    read,
  );
}

export const FAMILY: Family = {
  name: "gate-record-keeping",
  rules: [RECORD_KEEPING_RULE],
  run: (root, context) => recordKeepingRules(root, context?.tree?.read),
};
