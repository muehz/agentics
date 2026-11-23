import { mkdirSync, writeFileSync } from "node:fs";
import path from "node:path";
import { HOOKS_MOD } from "./repo-repo.ts";

/** A conflict block: an opening, a middle and a closing marker line between ordinary lines. */
export const CONFLICT_BLOCK =
  "before\n<<<<<<< HEAD\nmine\n=======\ntheirs\n>>>>>>> other\nafter\n";

/** Writes `data` to the slash-separated `rel` inside `root` as raw bytes, creating its parent directory first. */
export const writeBytes = (root: string, rel: string, data: Buffer): void => {
  const target = path.join(root, ...rel.split("/"));
  mkdirSync(path.dirname(target), { recursive: true });
  writeFileSync(target, data);
};

/** Every invisible range, written out literally so a changed bound in the production table is noticed. */
const INVISIBLE_RANGES: ReadonlyArray<readonly [number, number]> = [
  [0x061c, 0x061c],
  [0x115f, 0x1160],
  [0x180e, 0x180e],
  [0x200b, 0x200d],
  [0x200e, 0x200f],
  [0x202a, 0x202e],
  [0x2060, 0x2064],
  [0x2066, 0x2069],
  [0x3164, 0x3164],
  [0xfe00, 0xfe0f],
  [0xfeff, 0xfeff],
  [0xe0000, 0xe007f],
  [0xe0100, 0xe01ef],
];

const inRange = (point: number): boolean =>
  INVISIBLE_RANGES.some(([lo, hi]) => point >= lo && point <= hi);

/** The first and last code point of every invisible range. */
export const INVISIBLE_EDGES = INVISIBLE_RANGES.flatMap(([lo, hi]) =>
  lo === hi ? [lo] : [lo, hi],
);

/** The code point just below and just above every invisible range, except where that lands inside another range. */
export const OUTSIDE_INVISIBLE_EDGES = [
  ...new Set(
    INVISIBLE_RANGES.flatMap(([lo, hi]) => [lo - 1, hi + 1]).filter(
      (p) => !inRange(p),
    ),
  ),
];

/** The paths the hygiene scan skips, written out literally so a dropped entry in the production list is noticed. */
export const PATH_SKIPPED = [
  ".claude/worktrees",
  "node_modules",
  ".worktrees",
  "evals/runs",
  "docs/handoffs",
  "docs/probes",
  "docs/reviews/peer",
  ".agentics",
  ".audit",
];

/** A work folder's scratch directory, which the hygiene scan skips unless a file under it is tracked. */
export const SCRATCH = "docs/specs/2026-10/feature-x/temp";

/** A go.mod text: `base` (the hooks module by default) followed by `body` and a newline. */
export const WORK_MOD = (body: string, base: string = HOOKS_MOD): string =>
  `${base}${body}\n`;
