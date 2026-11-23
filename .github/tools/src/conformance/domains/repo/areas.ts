import { readFileSync } from "node:fs";
import path from "node:path";
import { AREA_ROOTS } from "../../../repository-areas.ts";
import { errorMessage, type Failure } from "../../check.ts";
import { readGoMod } from "../../go-source/gomod.ts";
import { repoFileErrorOf } from "../../repo-file-error.ts";
import { walkCheckout, type Tracked } from "./walk.ts";

/** The go.mod of the only Go module the repository holds. */
const MODULE_FILE = `${AREA_ROOTS.hooks}/go/go.mod`;

// The Go audit also covers the legacy top-level tools directory.
const COVERED = [...Object.values(AREA_ROOTS), "tools"].map((dir) => `${dir}/`);

function requireFailures(source: string): Failure[] {
  const mod = readGoMod(source);
  const fail = (message: string): Failure => ({
    rule: "area-go-require",
    path: MODULE_FILE,
    message,
  });
  return [
    ...mod.requires.map((required) =>
      fail(`requires ${required}; hooks/go requires no module`),
    ),
    ...mod.replaces.map((replace) =>
      fail(`replaces ${replace.old}; hooks/go requires no module`),
    ),
  ];
}

function moduleSetFailures(root: string, tracked: Tracked): Failure[] {
  const failures: Failure[] = [];
  walkCheckout(root, tracked, "area", (file) => {
    if (file.name !== "go.mod" && file.name !== "go.work") return;
    if (file.rel === MODULE_FILE) return;
    const covered =
      !file.rel.includes("/") ||
      COVERED.some((prefix) => file.rel.startsWith(prefix));
    if (!covered) return;
    failures.push({
      rule: "area-go-module-set",
      path: file.rel,
      message:
        file.name === "go.mod"
          ? "a Go module outside hooks/go; hooks/go is the only module root"
          : "a go.work file joins modules into one build; hooks/go is the only module root",
    });
  });
  return failures;
}

/**
 * Enforces the Go half of the area boundaries: `hooks/go/go.mod` has no `require` and no `replace`, and it is the only
 * `go.mod` or `go.work` at the repository root or under `hooks`, `evals`, `tools`, `skills` and `.github/tools`. Module
 * files anywhere else are not checked. A missing `hooks/go/go.mod` is not a failure. A skipped directory is still
 * entered for the files git tracks in it, so a force-added `go.mod` there is checked.
 */
export function areaGoModules(root: string, tracked: Tracked): Failure[] {
  const failures: Failure[] = [];
  let source: string | undefined;
  try {
    source = readFileSync(path.join(root, ...MODULE_FILE.split("/")), "utf8");
  } catch (error) {
    const fileError = repoFileErrorOf(error, MODULE_FILE);
    if (fileError.kind !== "missing") throw fileError;
  }
  if (source !== undefined) {
    try {
      failures.push(...requireFailures(source));
    } catch (error) {
      throw new Error(`${MODULE_FILE}: ${errorMessage(error)}`);
    }
  }
  return [...failures, ...moduleSetFailures(root, tracked)];
}
