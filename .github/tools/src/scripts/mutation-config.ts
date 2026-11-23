// Stryker runs from the repository root. Tests read repository files through root-relative paths, and a run started
// inside a workspace cannot see .github/ or the other areas in its temporary copy.
import { globSync } from "node:fs";

import path from "node:path";
import { isSkippedTest, readSkippedTests } from "./mutation-artifacts.ts";
import { strykerSourceGlobs } from "./mutation-source.ts";
const root = path.resolve(import.meta.dirname, "../../../..");
import {
  AREAS,
  DISABLE_TYPE_CHECKS,
  EXCLUDED_TEST_FILES,
  MUTATION_CONFIG_FILES,
  SKIP_TESTS_ENV,
} from "./mutation-config-values.ts";
export * from "./mutation-config-values.ts";

// `only` is the JSON list of test files that mutation-stryker.ts found to reach the mutated files. A run on named
// files sets it, so the initial run does not execute the test files of unrelated code.
export function testFiles(
  only = process.env.AGX_MUTATION_TEST_FILES,
  skipped: unknown = readSkippedTests(process.env[SKIP_TESTS_ENV]),
) {
  const wanted = only ? new Set<unknown>(JSON.parse(only)) : null;
  return AREAS.flatMap((area) =>
    globSync(`${area}/**/*.test.ts`, { cwd: root }),
  )
    .map((file) => file.split("\\").join("/"))
    .filter(
      (file) => !(file in EXCLUDED_TEST_FILES) && !isSkippedTest(skipped, file),
    )
    .filter((file) => !wanted || wanted.has(file))
    .sort();
}

export default {
  testRunner: "tap",
  // node:test prints its spec format when no terminal is attached, and the TAP runner then parses no tests.
  tap: { testFiles: testFiles(), nodeArgs: ["--test-reporter=tap"] },
  mutate: [
    ...AREAS.flatMap(strykerSourceGlobs),
    ...MUTATION_CONFIG_FILES.map((file) => `!${file}`),
  ],
  coverageAnalysis: "perTest",
  // Stryker's default also inserts `// @ts-nocheck` at the top of every .html and .vue file under a src directory. A
  // script in such a file then starts with a JavaScript comment, which the evals HTML renderer rejects for the bundled
  // viewer template. No checker runs here, so only the TypeScript and JavaScript files need the directive.
  disableTypeChecks: DISABLE_TYPE_CHECKS,
  // TypeScript 7 has no compiler API, and Stryker's tsconfig step calls ts.parseConfigFileTextToJson. A file name that
  // does not exist skips that step. Remove this line when Stryker supports TypeScript 7.
  tsconfigFile: "stryker.no-tsconfig.json",
  // progress-append-only prints the tested count and remaining time as plain lines, which a log file can show.
  reporters: ["clear-text", "progress-append-only", "json"],
  jsonReporter: { fileName: "reports/mutation/stryker.json" },
  tempDirName: ".stryker-tmp",
  concurrency: 4,
  // The dry run executes every test file once. Stryker's default limit of 5 minutes is shorter than that on a CI runner.
  dryRunTimeoutMinutes: 30,
};
