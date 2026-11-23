import type { Failure } from "../../check.ts";
import { checkPinnedText } from "../../pinned-text.ts";
import { auditFileError, isFileErrorOfKind } from "./files.ts";
import type { Family } from "./index.ts";

const CONTRACT_FILE = "skills/decompose/references/chain-artifacts.md";
const GATE_FILE = "skills/gate/references/branch/tidy.md";

/** One sentence a file must carry. The literal is compared with the file's text after whitespace folding. */
export type WorkFolderPin = {
  rel: string;
  literal: string;
  message: string;
};

/**
 * The sentences the work-folder layout rests on, in the order they are checked. The two files interleave, so a report
 * of several dropped pins follows this order and not file order.
 */
export const WORK_FOLDER_PINS: readonly WorkFolderPin[] = [
  {
    rel: CONTRACT_FILE,
    literal:
      "`intent.md` and `plan.md` are the required pair when `/agentics:gate branch` hands a branch off to a pull request",
    message:
      "chain-artifacts.md must state that intent.md and plan.md are the required pair at the gate branch hand-off",
  },
  {
    rel: CONTRACT_FILE,
    literal:
      "`intent.md` is required only when a present `plan.md` or `spec.md` names `[intent.md]` in its `derived-from` field",
    message:
      "chain-artifacts.md must make intent.md required only when a present plan.md or spec.md derives from it",
  },
  {
    rel: CONTRACT_FILE,
    literal:
      "The folder's scratch and evidence directory: optional, ignored by Git, with unrestricted contents, including nested folders",
    message:
      "chain-artifacts.md must define temp/ as the optional, Git-ignored scratch directory with unrestricted contents",
  },
  {
    rel: GATE_FILE,
    literal:
      "The tidy step never deletes content: it moves, renames, and untracks",
    message: "tidy.md must state that the tidy step never deletes content",
  },
  {
    rel: GATE_FILE,
    literal: "A name collision keeps both copies",
    message: "tidy.md must state that a name collision keeps both copies",
  },
  {
    rel: GATE_FILE,
    literal:
      "Tracked or force-added paths leave the index with `git rm -r --cached --ignore-unmatch`",
    message:
      "tidy.md must untrack tracked or force-added paths with git rm -r --cached --ignore-unmatch",
  },
  {
    rel: GATE_FILE,
    literal:
      "A conforming folder changes nothing and makes no commit, so a second run of the step is a no-op",
    message:
      "tidy.md must state that a conforming folder changes nothing and makes no commit",
  },
  {
    rel: GATE_FILE,
    literal:
      "Only a genuinely missing required file stops the gate: the stop names the file and every place searched",
    message:
      "tidy.md must stop the gate only for a genuinely missing required file, naming the file and every place searched",
  },
  {
    rel: GATE_FILE,
    literal:
      "The gate never fabricates a placeholder for a missing required file",
    message:
      "tidy.md must state that the gate never fabricates a placeholder for a missing required file",
  },
  {
    rel: GATE_FILE,
    literal:
      "The step does not run for `/agentics:gate check` or for a discard named in advance",
    message:
      "tidy.md must state that the tidy step does not run for gate check or a discard named in advance",
  },
  {
    rel: CONTRACT_FILE,
    literal:
      "A small fix that `execute fix` ran has no work folder and needs neither file at the hand-off",
    message:
      "chain-artifacts.md must state the small-fix exception: a small fix has no work folder and needs neither required file at the hand-off",
  },
  {
    rel: CONTRACT_FILE,
    literal:
      "`/agentics:gate branch` accepts that hand-off only when `<work-root>/unassigned/state.tsv` holds a `skip chain <branch>` row with no later `pivot fix <branch>` row",
    message:
      "chain-artifacts.md must state that the gate accepts a small-fix hand-off only when unassigned/state.tsv holds a skip chain row with no later pivot fix row",
  },
];

/**
 * Requires the two prose files that define and enforce the work-folder layout to keep every pinned sentence. The files
 * are read in two passes: the first reads each distinct file once, in first-use order, and reports a missing one once
 * as a source-present failure; the second reports the dropped sentences in table order. Any other read failure is an
 * audit error. The literals are compared as written, against the whitespace-folded text.
 */
export function workFolderRules(repoRoot: string): Failure[] {
  return checkPinnedText(repoRoot, WORK_FOLDER_PINS, {
    projection: "folded-source",
    readOrder: "before-pins",
    mismatch: (pin, rel) => ({
      rule: "work-folder-pin",
      path: rel,
      message: pin.message,
    }),
    sourceError: (_pin, rel, action, error) => {
      if (!isFileErrorOfKind(error, "missing"))
        throw auditFileError(action, rel, error);
      return {
        rule: "work-folder-source-present",
        path: rel,
        message: "not found: the work-folder contract pins cannot be checked",
      };
    },
  });
}

export const FAMILY: Family = {
  name: "workfolder",
  rules: ["work-folder-source-present", "work-folder-pin"],
  run: (root) => workFolderRules(root),
};
