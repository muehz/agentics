import {
  walkCheckout as walk,
  type Tracked,
  type WalkedFile,
} from "../../checkout-walk.ts";

export {
  trackedPaths,
  withoutGitLocation,
  type Tracked,
  type WalkedFile,
} from "../../checkout-walk.ts";

/**
 * Directories the walk does not enter. An entry matches a directory by its path from the root or by its own name, so
 * the entries without a slash match at any depth. Each is walk scope, not an exemption: in a checkout a tracked file
 * under one is still read.
 */
export const SKIP_DIRS: ReadonlySet<string> = new Set([
  ".git",
  ".claude/worktrees",
  "node_modules",
  ".worktrees",
  "evals/runs",
  "docs/handoffs",
  "docs/probes",
  "docs/reviews/peer",
  ".agentics",
  ".audit",
]);

/**
 * Both modes skip a work folder's scratch directory. `hygiene` reads regular files. `area` looks only at names, so a
 * symlink counts, and also skips every `testdata` directory and ignores the root's own name.
 */
export type WalkMode = "hygiene" | "area";

/**
 * Walks the checkout depth first, each directory's entries in the default sort order of their names, symlinks never
 * followed. A skipped directory that holds a tracked file is entered as a partial directory, where only tracked files
 * and directories that hold one are visited. `visit` is called for each file the mode cares about; an error it throws
 * propagates, and a directory that cannot be listed throws a `RepoFileError`.
 */
export function walkCheckout(
  root: string,
  tracked: Tracked,
  mode: WalkMode,
  visit: (file: WalkedFile) => void,
): void {
  walk(root, {
    mode: mode === "area" ? "repo-area" : "repo-hygiene",
    skipDirs: SKIP_DIRS,
    tracked,
    visit,
  });
}
