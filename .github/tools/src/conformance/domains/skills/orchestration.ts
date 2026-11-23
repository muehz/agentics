import { statSync } from "node:fs";
import type { Failure } from "../../check.ts";
import {
  auditFileError,
  checkedRepoPath,
  isFileErrorOfKind,
  readRepoFile,
  walkDir,
} from "./files.ts";
import type { Family } from "./index.ts";

/** The shared documents outside the `skills` tree. */
export const SHARED_FILES_EXTRA: readonly string[] = [
  "AGENTS.md",
  "README.md",
  "docs/ARCHITECTURE.md",
];

/** The trees whose markdown is in the shared set. */
const SHARED_ROOTS: readonly string[] = ["skills"];

/** The adapter files, which own native tool names by design. */
export const ADAPTER_FILES: readonly string[] = [
  "skills/routing/references/copilot-tools.md",
  "skills/routing/references/surface-dispatch.md",
  "skills/routing/references/model-ladders.md",
  "skills/routing/references/model-ladders-examples.md",
];

/** The per-surface explainers: exempt from the native names like the adapters, but never from a stale phrase. */
export const SURFACE_FILES: readonly string[] = [
  "docs/surfaces/claude-code.md",
  "docs/surfaces/codex.md",
  "docs/surfaces/copilot-cli.md",
  "docs/surfaces/cursor.md",
  "docs/surfaces/vscode.md",
];

/** Universal behavioral claims that are false on at least one shipped surface; rejected in every scanned file. */
export const STALE_PHRASES: readonly string[] = [
  "subagents are stateless",
  "one-shot stays the default everywhere",
  "Copilot CLI always uses one-shot dispatch",
  "waves " + String.fromCodePoint(0x2264) + "6, depth 1",
];

// POSIX [:space:] written out: the class includes the vertical tab and no Unicode space, so JavaScript's \s, which adds
// Unicode spaces, does not fit it. The ASCII alnum class is spelled out for the same reason, and ^ and $ keep their
// whole-text meaning because no pattern carries the m flag.
const SPACE = "\\t\\n\\v\\f\\r ";
const MARK = "`*_" + SPACE;
const WORD = "A-Za-z0-9_";
const NATIVE_NAMES =
  "run_in_background|spawn_agent|send_input|wait_agent|write_agent|SendMessage|runSubagent|invoke_subagent";

/**
 * Native orchestration identifiers, rejected in shared files only. In order: the Agent tool, the Task tool, the
 * emphasised lowercase task tool, the eight native names and the /fleet command.
 */
export const NATIVE_IDENT_RES: readonly RegExp[] = [
  new RegExp(`[${MARK}]*Agent[${MARK}]+[${SPACE}]*tool`, "u"),
  new RegExp(`[${MARK}]*Task[${MARK}]+[${SPACE}]*tool`, "u"),
  new RegExp(`[\`*_]+task[\`*_]+[${SPACE}]*tool`, "u"),
  new RegExp(`(^|[^${WORD}])(${NATIVE_NAMES})([^${WORD}]|$)`, "u"),
  new RegExp(`(^|[${SPACE}])/fleet([^${WORD}-]|$)`, "u"),
];

const ZERO_SELECTION =
  "skills: the orchestration scan selected 0 files " +
  String.fromCodePoint(0x2014) +
  " a guard that inspects nothing must never report success";

/** Whether `rel` resolves below the root without a link and `stat`s; a link is thrown, anything else is false. */
function present(repoRoot: string, rel: string): boolean {
  try {
    statSync(checkedRepoPath(repoRoot, rel));
    return true;
  } catch (error) {
    if (isFileErrorOfKind(error, "symlink"))
      throw auditFileError("checking", rel, error);
    return false;
  }
}

/**
 * Every shared markdown file, de-duplicated and in default sort order: the extras that exist plus every `.md` file under the
 * shared roots (a directory, a symlink and a name with another case of the suffix are not selected, and no directory is
 * skipped). A link on the way to an extra is an audit error; a linked root is skipped. Selecting nothing is an audit
 * error, never a pass.
 */
export function sharedSet(repoRoot: string): string[] {
  const seen = new Set<string>();
  for (const rel of SHARED_FILES_EXTRA)
    if (present(repoRoot, rel)) seen.add(rel);
  for (const root of SHARED_ROOTS) {
    let base: string;
    try {
      base = checkedRepoPath(repoRoot, root);
    } catch (error) {
      if (isFileErrorOfKind(error, "symlink")) continue;
      throw auditFileError("walking", root, error);
    }
    try {
      walkDir(
        base,
        (inner, entry) => {
          if (
            entry.isSymlink ||
            entry.isDirectory ||
            !entry.name.endsWith(".md")
          )
            return;
          seen.add(inner === "." ? root : `${root}/${inner}`);
        },
        root,
      );
    } catch (error) {
      throw auditFileError("walking", root, error);
    }
  }
  if (seen.size === 0) throw new Error(ZERO_SELECTION);
  return [...seen].sort();
}

/**
 * Applies the deny patterns to one file: every stale phrase first, line by line, then, for a file that is not an
 * adapter, one native-identifier finding per line at most.
 */
export function checkOrchestrationFile(
  rel: string,
  text: string,
  isAdapter: boolean,
): Failure[] {
  const failures: Failure[] = [];
  const lines = text.split("\n");
  lines.forEach((line, index) => {
    for (const phrase of STALE_PHRASES) {
      if (line.includes(phrase))
        failures.push({
          rule: "orchestration-stale-phrase",
          path: rel,
          message: `line ${index + 1}: stale universal claim: ${JSON.stringify(phrase)}`,
        });
    }
  });
  if (!isAdapter) {
    lines.forEach((line, index) => {
      if (NATIVE_IDENT_RES.some((pattern) => pattern.test(line)))
        failures.push({
          rule: "orchestration-native-identifier",
          path: rel,
          message: `line ${index + 1}: native orchestration identifier in a shared file`,
        });
    });
  }
  return failures;
}

/**
 * Validates the derived shared set against the checkout. The pinned adapter and surface files must exist; each that
 * does is appended to the scan list, which already holds the adapters (they are shared markdown), so an adapter is
 * scanned twice and a stale phrase in one is reported twice.
 */
export function orchestrationRules(repoRoot: string): Failure[] {
  const failures: Failure[] = [];
  const exempt = new Set([...ADAPTER_FILES, ...SURFACE_FILES]);
  const all = sharedSet(repoRoot);
  for (const rel of [...ADAPTER_FILES, ...SURFACE_FILES]) {
    if (present(repoRoot, rel)) {
      all.push(rel);
      continue;
    }
    failures.push({
      rule: "orchestration-file-present",
      path: rel,
      message: "exempted in the contract arrays but does not exist",
    });
  }
  all.sort();
  for (const rel of all) {
    let text: string;
    try {
      text = readRepoFile(repoRoot, rel);
    } catch (error) {
      if (isFileErrorOfKind(error, "missing")) continue;
      throw auditFileError("reading", rel, error);
    }
    failures.push(...checkOrchestrationFile(rel, text, exempt.has(rel)));
  }
  return failures;
}

export const FAMILY: Family = {
  name: "orchestration",
  rules: [
    "orchestration-file-present",
    "orchestration-stale-phrase",
    "orchestration-native-identifier",
  ],
  run: (root) => orchestrationRules(root),
};
