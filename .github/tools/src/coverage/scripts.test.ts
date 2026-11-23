import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import {
  mkdirSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";
import { fileURLToPath } from "node:url";
import { EXCLUDE_PATTERNS } from "./scope.ts";

const root = fileURLToPath(new URL("../../../../", import.meta.url));

test("coverage cli rejects compare as an unknown verb", () => {
  const result = spawnSync(
    process.execPath,
    [fileURLToPath(new URL("./cli.ts", import.meta.url)), "compare"],
    { encoding: "utf8" },
  );
  assert.equal(result.status, 1, `${result.stdout}\n${result.stderr}`);
  assert.match(result.stderr, /^usage: cli\.ts lcov /m);
});

for (const workspace of [".github/tools"] as const) {
  test(`${workspace} coverage script reports the same test files to workspace lcov`, () => {
    const pkg = JSON.parse(
      readFileSync(join(root, workspace, "package.json"), "utf8"),
    );
    assert.equal(
      pkg.scripts.test,
      'node src/scripts/node-test.ts "src/**/*.test.ts"',
    );
    const script: string = pkg.scripts["test:coverage"];
    assert.equal(typeof script, "string", "test:coverage script is missing");
    assert.ok(script.includes('"src/**/*.test.ts"'));
    for (const pattern of EXCLUDE_PATTERNS)
      assert.ok(script.includes(pattern), pattern);

    const fixture = mkdtempSync(join(tmpdir(), "agentics-coverage-script-"));
    try {
      mkdirSync(join(fixture, "src", "testing"), { recursive: true });
      writeFileSync(join(fixture, "package.json"), '{"type":"module"}');
      writeFileSync(
        join(fixture, "src", "subject.ts"),
        "export const answer = 42;\n",
      );
      writeFileSync(
        join(fixture, "src", "testing", "helper.ts"),
        "export const helper = 1;\n",
      );
      writeFileSync(
        join(fixture, "src", "subject.test.ts"),
        'import { test } from "node:test"; import { answer } from "./subject.ts"; import { helper } from "./testing/helper.ts"; test("fixture", () => { if (answer + helper !== 43) throw new Error("fixture"); });\n',
      );
      const env = { ...process.env };
      delete env.NODE_TEST_CONTEXT;
      delete env.NODE_V8_COVERAGE;
      const result = spawnSync(script, {
        cwd: fixture,
        shell: true,
        encoding: "utf8",
        env,
      });
      assert.equal(result.status, 0, `${result.stdout}\n${result.stderr}`);
      assert.match(result.stdout, /fixture/);
      const report = readFileSync(
        join(fixture, "coverage", "lcov.info"),
        "utf8",
      ).replaceAll("\\", "/");
      assert.match(report, /SF:(?:.*\/)?src\/subject\.ts/);
      assert.doesNotMatch(report, /SF:(?:.*\/)?src\/subject\.test\.ts/);
      assert.doesNotMatch(report, /SF:(?:.*\/)?src\/testing\/helper\.ts/);
    } finally {
      rmSync(fixture, { recursive: true, force: true });
    }
  });
}
