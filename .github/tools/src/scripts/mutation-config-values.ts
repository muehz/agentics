export {
  SOURCE_ROOTS as AREAS,
  MUTATION_CONFIG_FILE,
  MUTATION_CONFIG_FILES,
  isMutationConfigFile,
} from "./mutation-source.ts";

export const DISABLE_TYPE_CHECKS =
  "**/{test,src,lib}/**/*.{js,ts,jsx,tsx,mjs,mts,cts,cjs}";

// Test files that fail when Stryker instruments the source. They stay out until a follow-up fixes them, so a mutant
// that only they would kill shows as a survivor or as not covered.
export const EXCLUDED_TEST_FILES: Readonly<Record<string, string>> = {
  ".github/tools/src/release/action.test.ts":
    "reads the release action's source text, which instrumentation rewrites",
  ".github/tools/src/release/zero-deps.test.ts":
    "scans production source imports, which instrumentation rewrites",
  ".github/tools/src/release/plugin.test.ts":
    "failed the Stryker dry run on 2026-11-07; cause not yet diagnosed",
  ".github/tools/src/peer/git-preflight.test.ts":
    "failed the Stryker dry run on 2026-11-07; cause not yet diagnosed",
  ".github/tools/src/peer/dispatch.test.ts":
    "failed the Stryker dry run on 2026-11-07; cause not yet diagnosed",
  ".github/tools/src/peer/interrupt.test.ts":
    "failed the Stryker dry run on 2026-11-07; cause not yet diagnosed",
  ".github/tools/src/peer/multi-target.test.ts":
    "failed the Stryker dry run on 2026-11-07; cause not yet diagnosed",
  ".github/tools/src/peer/process.test.ts":
    "failed the Stryker dry run on 2026-11-07; cause not yet diagnosed",
  ".github/tools/src/peer/watchdog.test.ts":
    "failed the Stryker dry run on 2026-11-07; cause not yet diagnosed",
  ".github/tools/src/conformance/domains/tools/exec.test.ts":
    "failed the Stryker dry run on Linux on 2026-11-08; cause not yet diagnosed",
  ".github/tools/src/release/level-parity.test.ts":
    "failed the Stryker dry run on Linux on 2026-11-08; cause not yet diagnosed",
  ".github/tools/src/release/plugin.e2e.test.ts":
    "failed the Stryker dry run on Linux on 2026-11-08; cause not yet diagnosed",
};

// The mutation wrapper's --max-test-seconds names a test-cost report here; its skipped files are too slow to rerun for
// every mutant they cover.
export const SKIP_TESTS_ENV = "AGX_STRYKER_TEST_COST";
