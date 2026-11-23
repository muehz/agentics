import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import { partitionTests } from "./ci-test-shard.ts";

const repo = path.resolve(import.meta.dirname, "../../../..");
const launcher = path.join(import.meta.dirname, "ci-test-shard.ts");

test("each CI plan assigns every workspace test file once", () => {
  for (const [workspace, plan, count] of [
    [".github/tools", "windows-tools", 3],
    [".github/tools", "coverage-tools", 1],
  ] as const) {
    const files = fs
      .globSync("src/**/*.test.ts", { cwd: path.join(repo, workspace) })
      .map((file) => file.replaceAll("\\", "/"))
      .sort();
    const shards = partitionTests(plan, files);
    assert.equal(shards.length, count, plan);
    assert.deepEqual(shards.flat().sort(), files, plan);
    assert.deepEqual(partitionTests(plan, [...files].reverse()), shards, plan);
    assert.throws(
      () => partitionTests(plan, [...files, files[0]!]),
      /duplicate test file/,
    );
  }
});

test("the Windows tools plan splits scripts, conformance and boundaries, and the rest", () => {
  const shards = partitionTests("windows-tools", [
    "src/release/plugin.test.ts",
    "src/boundaries/rules.test.ts",
    "src/scripts/ci-test-shard.test.ts",
    "src/conformance/registry.test.ts",
  ]);
  assert.deepEqual(shards, [
    ["src/scripts/ci-test-shard.test.ts"],
    ["src/boundaries/rules.test.ts", "src/conformance/registry.test.ts"],
    ["src/release/plugin.test.ts"],
  ]);
});

test("the shard launcher rejects invalid selections and reports file starts and exits", (t) => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "agentics-ci-shard-"));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  for (const [area, body] of [
    ["scripts", "throw new Error('selected fixture failed');"],
    ["conformance", "throw new Error('wrong shard');"],
    ["release", "throw new Error('wrong shard');"],
  ] as const) {
    const dir = path.join(root, ".github", "tools", "src", area);
    fs.mkdirSync(dir, { recursive: true });
    fs.writeFileSync(
      path.join(dir, "fixture.test.ts"),
      `import test from 'node:test'; test('fixture', () => { ${body} });`,
    );
  }
  const env = { ...process.env };
  delete env.NODE_TEST_CONTEXT;
  const run = (...args: string[]) =>
    spawnSync(process.execPath, [launcher, ...args], {
      cwd: root,
      env,
      encoding: "utf8",
      timeout: 10_000,
    });
  const invalid = run("windows-tools", "4", "2");
  assert.equal(invalid.status, 1);
  assert.match(invalid.stderr, /invalid CI test shard/);
  // The eval engine is tested in its own repository, so no plan runs the evals workspace.
  for (const plan of ["windows-evals", "coverage-evals"]) {
    const unknown = run(plan, "1", plan.startsWith("coverage-") ? "1" : "2");
    assert.equal(unknown.status, 1);
    assert.match(unknown.stderr, /unknown CI test plan/);
  }
  const result = run("windows-tools", "1", "2");
  assert.ifError(result.error);
  assert.equal(result.status, 1, result.stderr);
  assert.match(result.stdout, /selected fixture failed/);
  assert.doesNotMatch(result.stdout, /wrong shard/);
  const file = JSON.stringify(path.join("src", "scripts", "fixture.test.ts"));
  assert.ok(result.stderr.includes(`START ${file}`), result.stderr);
  assert.ok(result.stderr.includes(`FINISH ${file}`), result.stderr);
  assert.match(result.stderr, /ms failed/);
});

test("the instrumented shard writes LCOV for the files it exercises", (t) => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "agentics-ci-coverage-"));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const src = path.join(root, ".github", "tools", "src");
  fs.mkdirSync(src, { recursive: true });
  fs.writeFileSync(path.join(src, "fixture.ts"), "export const value = 1;\n");
  fs.writeFileSync(
    path.join(src, "fixture.test.ts"),
    "import test from 'node:test'; import assert from 'node:assert/strict'; import { value } from './fixture.ts'; test('covered', () => assert.equal(value, 1));\n",
  );
  const env = { ...process.env };
  delete env.NODE_TEST_CONTEXT;
  const result = spawnSync(
    process.execPath,
    [launcher, "coverage-tools", "1", "1"],
    {
      cwd: root,
      env,
      encoding: "utf8",
      timeout: 10_000,
    },
  );
  assert.ifError(result.error);
  assert.equal(result.status, 0, result.stderr);
  const report = fs.readFileSync(
    path.join(root, ".github/tools/coverage/lcov.info"),
    "utf8",
  );
  assert.match(report, /^SF:.*fixture\.ts$/m);
  assert.doesNotMatch(report, /^SF:.*fixture\.test\.ts$/m);
});
