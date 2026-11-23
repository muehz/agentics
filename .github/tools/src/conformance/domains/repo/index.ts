import {
  errorMessage,
  type AuditResult,
  type Failure,
  type RunOptions,
} from "../../check.ts";
import { areaGoModules } from "./areas.ts";
import { hygieneWalk } from "./hygiene.ts";
import { shellAllowlist } from "./shell-allowlist.ts";
import { trackedPaths, type Tracked } from "./walk.ts";

const error = (prefix: string, cause: unknown): AuditResult => ({
  failures: [],
  error: `repo: ${prefix}${errorMessage(cause)}`,
});

/** Audits repository text hygiene, the Go area boundaries and the shell allowlist. */
export async function run(
  repoRoot: string,
  options: RunOptions,
): Promise<AuditResult> {
  let tracked: Tracked;
  try {
    tracked = trackedPaths(repoRoot, options.env);
  } catch (cause) {
    return error("", cause);
  }
  const failures: Failure[] = [];
  let scanned = 0;
  try {
    const walked = hygieneWalk(repoRoot, tracked);
    failures.push(...walked.failures);
    scanned = walked.scanned;
  } catch (cause) {
    return error("", cause);
  }
  try {
    failures.push(...areaGoModules(repoRoot, tracked));
  } catch (cause) {
    return error("checking area Go modules: ", cause);
  }
  try {
    failures.push(...shellAllowlist(repoRoot, tracked));
  } catch (cause) {
    return error("checking the shell allowlist: ", cause);
  }
  if (scanned === 0)
    return {
      failures: [],
      error:
        "repo: the hygiene scan read 0 files — a guard that inspects nothing must never report success",
    };
  return { failures };
}
