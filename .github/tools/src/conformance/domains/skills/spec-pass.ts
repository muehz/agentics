import type { Failure } from "../../check.ts";
import { checkPinnedText } from "../../pinned-text.ts";
import { repoFileErrorOf } from "../../repo-file-error.ts";
import { isFileErrorOfKind } from "./files.ts";
import type { Family } from "./index.ts";

/** The one rule id this family emits. */
export const SPEC_PASS_RULE = "spec-pass-pinned";

const SPEC_AXIS_PATH = "skills/review/references/axes/spec-axis.md";
const AXES_MODE_PATH = "skills/review/references/axes/axes-mode.md";
const BRANCH_WORKFLOW_PATH =
  "skills/gate/references/branch/review-and-audit.md";

/** One sentence a file must carry. */
export type SpecPassPin = { rel: string; literal: string };

/** The sentences that carry the spec-pass contract, in the order they are checked. */
export const SPEC_PASS_PINS: readonly SpecPassPin[] = [
  { rel: SPEC_AXIS_PATH, literal: "## Per-requirement table" },
  {
    rel: SPEC_AXIS_PATH,
    literal:
      "`unrequested` lists a scope addition and is never counted as a gap.",
  },
  {
    rel: AXES_MODE_PATH,
    literal:
      "Each spec path named in the invocation. They are read before any other source.",
  },
  {
    rel: AXES_MODE_PATH,
    literal: "End the aggregate with exactly one `Lanes:` line",
  },
  {
    rel: AXES_MODE_PATH,
    literal: "Axes mode accepts `--spec-only`, which runs the Spec lane alone",
  },
  {
    rel: BRANCH_WORKFLOW_PATH,
    literal:
      "Read the per-requirement table from the Step 8 run whose pinned SHA equals the SHA this step pinned.",
  },
  {
    rel: BRANCH_WORKFLOW_PATH,
    literal: "Review: incomplete (<lane> not run)",
  },
];

/** The legacy missing-file marker. A real file containing exactly this text is also skipped. */
export const UNREADABLE_PLACEHOLDER = "\0unreadable";

/**
 * Requires every pinned sentence in the three files that carry the spec-pass contract. A missing file is one failure,
 * and its remaining pins are skipped; any other read failure is an audit error that carries no `skills:` prefix. Both
 * the file text and the literal are whitespace-folded before the comparison.
 */
export function specPassRules(repoRoot: string): Failure[] {
  return checkPinnedText(repoRoot, SPEC_PASS_PINS, {
    projection: "folded",
    ignoreText: (text) => text === UNREADABLE_PLACEHOLDER,
    mismatch: (pin, rel) => ({
      rule: SPEC_PASS_RULE,
      path: rel,
      message: `${rel} no longer carries the pinned sentence ${JSON.stringify(pin.literal)}`,
    }),
    sourceError: (_pin, rel, _action, error) => {
      const fileError = repoFileErrorOf(error, rel);
      const message = `cannot read ${rel}, which carries the spec-pass contract: ${fileError.message}`;
      if (!isFileErrorOfKind(fileError, "missing"))
        throw new Error(message, { cause: fileError });
      return { rule: SPEC_PASS_RULE, path: rel, message };
    },
  });
}

export const FAMILY: Family = {
  name: "spec-pass",
  rules: [SPEC_PASS_RULE],
  run: (root) => specPassRules(root),
};
