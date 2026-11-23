/**
 * A work folder's scratch directory: the shape `<YYYY-MM>/<purpose>-<topic>/temp` anywhere in a slash path, where the
 * purpose and topic are lowercase ASCII letters, digits and hyphens. The match does not depend on the configured work
 * root, so a work folder under any parent is recognized. That directory is scratch with no content rules.
 *
 * Both forms derive from this one source so the directory a walk skips and the files a check ignores cannot drift.
 * `\d` matches ASCII digits only, so a year written in other digits is not a work folder.
 */
const WORK_FOLDER_TEMP_SOURCE = String.raw`(?:^|\/)\d{4}-\d{2}\/[a-z0-9-]+\/temp`;

const TEMP_DIR = new RegExp(`${WORK_FOLDER_TEMP_SOURCE}$`);
const UNDER_TEMP = new RegExp(`${WORK_FOLDER_TEMP_SOURCE}\\/`);

/** Whether the slash path names a work folder's `temp` directory itself. */
export function isWorkFolderTempDir(rel: string): boolean {
  return TEMP_DIR.test(rel);
}

/** Whether the slash path lies below a work folder's `temp` directory. The directory itself is not below it. */
export function inWorkFolderTemp(rel: string): boolean {
  return UNDER_TEMP.test(rel);
}

/**
 * Wraps a lister in a getter that calls it on first use and returns that result on every later call. A failure is
 * cached too and the same error rethrown, so a broken checkout costs one git run and every caller sees one cause. The
 * lister is passed in, rather than this module calling `trackedPaths` itself, so the module depends on no domain and
 * the repo walk can import it without a cycle.
 */
export function lazyTrackedLookup<T>(list: () => T): () => T {
  let state:
    | { kind: "pending" }
    | { kind: "done"; value: T }
    | { kind: "failed"; error: unknown } = { kind: "pending" };
  return () => {
    if (state.kind === "pending") {
      try {
        state = { kind: "done", value: list() };
      } catch (error) {
        state = { kind: "failed", error };
      }
    }
    if (state.kind === "failed") throw state.error;
    return state.value;
  };
}
