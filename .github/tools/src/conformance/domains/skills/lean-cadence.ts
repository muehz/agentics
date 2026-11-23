import type { Failure } from "../../check.ts";
import { checkPinnedText } from "../../pinned-text.ts";
import { RepoFileError } from "../../repo-file-error.ts";
import {
  auditFileError,
  checkedRepoPath,
  isFileErrorOfKind,
  walkDir,
} from "./files.ts";
import type { Family } from "./index.ts";

/** The rule id of a present pin that is gone or an absent pin that returned. */
export const LEAN_CADENCE_RULE = "lean-cadence-pins";

/** The rule id of a pinned file or directory that does not exist. */
export const LEAN_CADENCE_SOURCE_RULE = "lean-cadence-source-present";

/**
 * One literal a file must carry, or must not carry. `rel` names a file, or a directory when `dir` is set; a directory
 * pin applies to every Markdown file below it and reports the file that carries the text.
 */
export type LeanCadencePin = {
  rel: string;
  literal: string;
  absent?: true;
  dir?: true;
};

/**
 * The lean review cadence pins in the order they are checked: five present pins, then seven retired sentences that must
 * not return. A failure follows this order, with a missing source reported where it is first met.
 */
export const LEAN_CADENCE_PINS: readonly LeanCadencePin[] = [
  {
    rel: "skills/decompose/references/plan-template.md",
    literal: "**Packet review:**",
  },
  {
    rel: "skills/execute/references/stage-1-dispatch-rule.md",
    literal: "**Packet review:**",
  },
  { rel: "skills/execute/SKILL.md", literal: "review group" },
  // The Events table rows. The Evidence-forms table has rows that start with the same event names, so the pins run on
  // into each row's description to anchor them on the Events table.
  {
    rel: "skills/execute/references/flow-state.md",
    literal: "| `review` | a reviewer returned",
  },
  {
    rel: "skills/execute/references/flow-state.md",
    literal: "| `escape` | a Critical or Important defect that passed a review",
  },
  {
    rel: "skills/execute/SKILL.md",
    literal: "Dispatch each task's Stage 2 reviewer as its implementer returns",
    absent: true,
  },
  {
    rel: "skills/execute/SKILL.md",
    literal: "Stage 2 is pipelined per task",
    absent: true,
  },
  {
    rel: "skills/execute",
    literal: "runs at every wave boundary",
    absent: true,
    dir: true,
  },
  {
    rel: "skills/execute",
    literal: "aggregate wave review at every wave boundary",
    absent: true,
    dir: true,
  },
  {
    rel: "skills/gh/SKILL.md",
    literal: "always allowed. That includes a conversion",
    absent: true,
  },
  {
    rel: "skills/gh/references/stack.md",
    literal: "Only ever propose to convert",
    absent: true,
  },
  {
    rel: "skills/execute",
    literal: "Per-dispatch timing is not logged",
    absent: true,
    dir: true,
  },
];

const NOT_FOUND = "not found: the lean-cadence pins cannot be checked";

/**
 * The repo-relative slash paths of every file below `dir` whose name ends in `.md` (case-sensitive), in default sort
 * order of the full paths. No directory name is skipped. A link of any kind or extension, at the root or anywhere below
 * it, throws a `RepoFileError` of kind `symlink` naming the link, because the walk never follows a link and a linked
 * subtree would otherwise let every retired phrase pass unchecked. A root that cannot be inspected throws the
 * `RepoFileError` of `checkedRepoPath`, of kind `missing` when it does not exist.
 */
export function leanCadenceMarkdown(repoRoot: string, dir: string): string[] {
  const root = checkedRepoPath(repoRoot, dir);
  const out: string[] = [];
  walkDir(
    root,
    (inner, entry) => {
      const rel = inner === "." ? dir : `${dir}/${inner}`;
      if (entry.isSymlink) throw new RepoFileError("symlink", rel);
      if (entry.isDirectory || !entry.name.endsWith(".md")) return;
      out.push(rel);
    },
    dir,
  );
  return out.sort();
}

/**
 * Requires every present pin and rejects every absent pin, comparing against whitespace-folded text. A pinned file or
 * directory that does not exist is one failure per path, reported when it is first met; any other read or scan failure
 * is an audit error.
 */
export function leanCadenceRules(repoRoot: string): Failure[] {
  return checkPinnedText(repoRoot, LEAN_CADENCE_PINS, {
    projection: "folded-source",
    check: (pin) => ({
      kind: pin.absent || pin.dir ? "forbidden" : "required",
    }),
    directory: {
      matches: (pin) => pin.dir === true,
      // The direct scan call lets the filesystem test seam distinguish it from file reads.
      files: (root, rel) => leanCadenceMarkdown(root, rel),
    },
    mismatch: (pin, rel, failure) => ({
      rule: LEAN_CADENCE_RULE,
      path: rel,
      message:
        failure.kind === "forbidden"
          ? `retired text present: ${pin.literal}`
          : `missing: ${pin.literal}`,
    }),
    sourceError: (_pin, rel, action, error) => {
      if (!isFileErrorOfKind(error, "missing"))
        throw auditFileError(action, rel, error);
      return { rule: LEAN_CADENCE_SOURCE_RULE, path: rel, message: NOT_FOUND };
    },
  });
}

export const FAMILY: Family = {
  name: "lean-cadence",
  rules: [LEAN_CADENCE_RULE, LEAN_CADENCE_SOURCE_RULE],
  run: (root) => leanCadenceRules(root),
};
