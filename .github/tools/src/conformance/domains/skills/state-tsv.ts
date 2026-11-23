import type { Failure } from "../../check.ts";
import { repoFileErrorOf } from "../../repo-file-error.ts";
import { checkedRepoPath, readRepoFile, walkDir } from "./files.ts";
import { inWorkFolderTemp } from "../../work-folder-temp.ts";
import type { Family } from "./index.ts";

/** Whether a path lies under a work folder's `temp/`; the state check skips such files. */
export { inWorkFolderTemp };

const RULE = "state-tsv-shape";

/** The directory the state files live under. */
const SPECS_ROOT = "docs/specs";

// A new file starts with the header row, which names the columns; a file from before carries the version line above
// it. The helper reads each row by its own content and never rewrites that line, so no other version line is valid.
const VERSION_ONE = "# flow-state 1";
const HEADER_ROW =
  "ts\tsession\tphase\tevent\ttarget\tstatus\twhy\tevidence\tnext";

const PHASES: ReadonlySet<string> = new Set([
  "intent",
  "spec",
  "decompose",
  "execute",
  "gate",
]);
const STATUSES: ReadonlySet<string> = new Set([
  "ok",
  "failed",
  "blocked",
  "pending",
  "declined",
  "cancelled",
]);
const EVENTS: ReadonlySet<string> = new Set([
  "create",
  "challenge",
  "approve",
  "revoke",
  "diverge",
  "verify",
  "revert",
  "demote",
  "block",
  "unblock",
  "pivot",
  "waive",
  "branch",
  "release",
  "change",
  "skip",
  "na",
  "enter",
  "bind",
  "pause",
  "done",
  "review",
  "escape",
  "order",
  "commit",
  "drop",
  "note",
]);
/** The kinds of change a `commit` row records as its target. */
const COMMIT_KINDS: ReadonlySet<string> = new Set([
  "fix",
  "triage",
  "user",
  "housekeeping",
]);

// No multi-line flag: `$` is the end of the text only.
const TS_SHAPE = /^20[0-9]{2}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$/;

/**
 * Whether a stored cell looks like a raw value that should have carried the writer's
 * formula-injection guard. A cell starting with `=`, `+`, `-` or `@` is unguarded; one starting with an apostrophe is
 * guarded only when the next character is a trigger or a second apostrophe. Only the first characters are read.
 */
function needsGuard(cell: string): boolean {
  switch (cell[0]) {
    case "=":
    case "+":
    case "-":
    case "@":
      return true;
    case "'":
      switch (cell[1]) {
        case "=":
        case "+":
        case "-":
        case "@":
        case "'":
          return false;
        default:
          return true;
      }
    default:
      return false;
  }
}

/** The zero-based index of the session cell in a row. */
const SESSION_COLUMN = 1;

/** A lone `-` in the session column is what the writer stores when the host supplies no session id. */
function isSessionPlaceholder(column: number, cell: string): boolean {
  return column === SESSION_COLUMN && cell === "-";
}

/**
 * The shape failures of one state.tsv, in check order. Per data row: a carriage return, the cell
 * count, then for each cell the whitespace and guard checks, then the ts, phase, event and status cells, the status of
 * a `drop` row and the target of a `commit` row, the empty cells, and the `next` cell. Only newlines are trimmed from the
 * end of the body, never a carriage return.
 */
export function checkStateFile(rel: string, body: string): Failure[] {
  const out: Failure[] = [];
  const add = (message: string): void => {
    out.push({ rule: RULE, path: rel, message });
  };
  const lines = body.replace(/\n+$/, "").split("\n");
  const firstLine = lines[0] ?? "";
  // The index of the first line after the header row.
  let start = 1;
  if (firstLine !== HEADER_ROW) {
    if (firstLine !== VERSION_ONE)
      add(
        `line 1 is ${JSON.stringify(firstLine)}, want the header row or the version line ${JSON.stringify(VERSION_ONE)}`,
      );
    // A line 1 that reads as a version line, valid or not, stands above the header row.
    if (firstLine.startsWith("#")) {
      if (lines[1] !== HEADER_ROW) add("line 2 is not the header row");
      start = 2;
    }
  }
  lines.slice(start).forEach((line, index) => {
    const n = index + start + 1;
    // A merge resolved by hand can repeat the version line and the header row; that is a valid merged file.
    if (line === VERSION_ONE || line === HEADER_ROW) return;
    if (line.includes("\r")) {
      add(`line ${n} contains a carriage return`);
      return;
    }
    const cells = line.split("\t");
    if (cells.length !== 9) {
      add(`line ${n} has ${cells.length} cells, want 9`);
      return;
    }
    cells.forEach((cell, column) => {
      // Whitespace is checked before the guard because it hides a trigger character from the guard.
      if (cell.trim() !== cell) {
        add(
          `line ${n}: cell ${column + 1} (${JSON.stringify(cell)}) carries leading or trailing whitespace; the writer trims before it applies the formula-injection guard, so whitespace here means the guard was bypassed`,
        );
        return;
      }
      if (needsGuard(cell) && !isSessionPlaceholder(column, cell))
        add(
          `line ${n}: cell ${column + 1} (${JSON.stringify(cell)}) begins with =, +, -, @, or a quote but carries no guard prefix`,
        );
    });
    const [
      ts = "",
      ,
      phase = "",
      event = "",
      target = "",
      status = "",
      ,
      ,
      next = "",
    ] = cells;
    if (!TS_SHAPE.test(ts))
      add(`line ${n}: ts ${JSON.stringify(ts)} is not ISO 8601 UTC`);
    if (!PHASES.has(phase))
      add(
        `line ${n}: phase ${JSON.stringify(phase)} is outside the closed set`,
      );
    if (!EVENTS.has(event))
      add(
        `line ${n}: event ${JSON.stringify(event)} is outside the closed set`,
      );
    if (!STATUSES.has(status))
      add(
        `line ${n}: status ${JSON.stringify(status)} is outside the closed set`,
      );
    if (event === "drop" && status !== "cancelled")
      add(
        `line ${n}: a drop row has status ${JSON.stringify(status)}, want cancelled`,
      );
    if (event === "commit" && !COMMIT_KINDS.has(target))
      add(
        `line ${n}: a commit row has target ${JSON.stringify(target)}, want fix, triage, user or housekeeping`,
      );
    for (const column of [4, 6, 7, 8])
      if ((cells[column] ?? "").trim() === "")
        add(`line ${n}: cell ${column + 1} is empty`);
    const first = next.split(/\s+/).find((field) => field !== "");
    if (first === undefined || (!PHASES.has(first) && first !== "done"))
      add(
        `line ${n}: next ${JSON.stringify(next)} must start with a phase name or done`,
      );
  });
  return out;
}

/**
 * Checks every committed `state.tsv` under `docs/specs`, found by a depth-first walk with each
 * directory's entries in default sort order and no directory skipped by name. A symlink is never entered and a symlinked
 * `state.tsv` is never read. A file under a work folder's `temp/` is skipped. A `docs/specs` that cannot be resolved,
 * for any reason, passes silently; any other walk or read failure is an audit error.
 */
function stateTSVShapeRules(root: string): Failure[] {
  let base: string;
  try {
    base = checkedRepoPath(root, SPECS_ROOT);
  } catch {
    return [];
  }
  const failures: Failure[] = [];
  try {
    walkDir(
      base,
      (inner, entry) => {
        if (inner === "." || entry.isDirectory || entry.isSymlink) return;
        if (entry.name !== "state.tsv") return;
        const rel = `${SPECS_ROOT}/${inner}`;
        if (inWorkFolderTemp(rel)) return;
        failures.push(...checkStateFile(rel, readRepoFile(root, rel)));
      },
      SPECS_ROOT,
    );
  } catch (error) {
    const fileError = repoFileErrorOf(error, SPECS_ROOT);
    throw new Error(fileError.message, { cause: fileError });
  }
  return failures;
}

/** The `state-tsv` family: the shape of every committed state.tsv under `docs/specs`. */
export const FAMILY: Family = {
  name: "state-tsv",
  rules: [RULE],
  run: stateTSVShapeRules,
};
