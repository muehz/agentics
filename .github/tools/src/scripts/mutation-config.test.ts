import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import path from "node:path";
import { pathToFileURL } from "node:url";
import test from "node:test";
import config, {
  AREAS,
  DISABLE_TYPE_CHECKS,
  EXCLUDED_TEST_FILES,
  MUTATION_CONFIG_FILE,
  SKIP_TESTS_ENV,
  testFiles,
} from "./mutation-config.ts";

test("Stryker's root entrypoint preserves the canonical typed module exports", () => {
  const rootURL = pathToFileURL(
    path.resolve(import.meta.dirname, "../../../../stryker.config.mjs"),
  ).href;
  const run = spawnSync(
    process.execPath,
    [
      "--input-type=module",
      "-e",
      `const m=await import(process.argv[1]);const {testFiles,...tap}=m.default.tap;console.log(JSON.stringify({areas:m.AREAS,disable:m.DISABLE_TYPE_CHECKS,excluded:m.EXCLUDED_TEST_FILES,skip:m.SKIP_TESTS_ENV,config:{...m.default,tap},selected:m.testFiles(JSON.stringify(['.github/tools/src/scripts/mutation-config.test.ts']),[])}));`,
      rootURL,
    ],
    { encoding: "utf8", timeout: 10000 },
  );
  assert.equal(run.status, 0, run.stderr);
  const { testFiles: unused, ...tap } = config.tap;
  assert.deepEqual(JSON.parse(run.stdout), {
    areas: AREAS,
    disable: DISABLE_TYPE_CHECKS,
    excluded: EXCLUDED_TEST_FILES,
    skip: SKIP_TESTS_ENV,
    config: { ...config, tap },
    selected: [".github/tools/src/scripts/mutation-config.test.ts"],
  });
});

test("the default mutator targets exclude the moved configuration but retain source files", () => {
  const selected = (file: string) => {
    let included = false;
    for (const pattern of config.mutate) {
      const negative = pattern.startsWith("!");
      if (path.posix.matchesGlob(file, negative ? pattern.slice(1) : pattern))
        included = !negative;
    }
    return included;
  };
  assert.equal(selected(MUTATION_CONFIG_FILE), false);
  assert.equal(selected(".github/tools/src/scripts/mutation-scope.ts"), true);
  assert.equal(
    selected(".github/tools/src/scripts/mutation-scope.test.ts"),
    false,
  );
  assert.equal(selected(".github/tools/src/coverage/scope.ts"), true);
  // The eval engine is mutation-tested in its own repository.
  assert.equal(selected("evals/src/cli/main.ts"), false);
});

test("test discovery still applies exclusion, explicit selection, and cost filtering", () => {
  const one = ".github/tools/src/scripts/mutation-config.test.ts";
  assert.deepEqual(testFiles(JSON.stringify([one]), []), [one]);
  assert.deepEqual(testFiles(JSON.stringify([one]), [one]), []);
  assert.deepEqual(testFiles("[]", []), []);
  const excluded = Object.keys(EXCLUDED_TEST_FILES)[0]!;
  assert.deepEqual(testFiles(JSON.stringify([excluded]), []), []);
});
