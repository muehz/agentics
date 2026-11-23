import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import os from "node:os";
import { spawnSync } from "node:child_process";
import test from "node:test";
import { concurrency } from "./node-test.ts";

test("a two-CPU runner still runs four test files at once", () => {
  assert.equal(concurrency(1), 4);
  assert.equal(concurrency(2), 4);
  assert.equal(concurrency(5), 4);
});

test("a larger machine keeps Node's default of one fewer than its CPUs", () => {
  assert.equal(concurrency(6), 5);
  assert.equal(concurrency(24), 23);
});

test("an explicit concurrency limit can run fewer than four files", () => {
  assert.equal(concurrency(24, "2"), 2);
  assert.equal(concurrency(24, "1"), 1);
});

test("invalid concurrency limits are rejected", () => {
  for (const value of ["", "0", "-1", "1.5", "two", "9007199254740992"])
    assert.throws(() => concurrency(24, value), /positive safe integer/);
});

test("the launcher honors a serial limit across real test files", (t) => {
  const dir = fs.mkdtempSync(
    path.join(os.tmpdir(), "agentics-test-concurrency-"),
  );
  t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
  const events = path.join(dir, "events.txt");
  const files = ["a", "b"].map((name) => {
    const file = path.join(dir, `${name}.test.cjs`);
    fs.writeFileSync(
      file,
      `const fs = require('node:fs');
const test = require('node:test');
test('${name}', async () => {
  fs.appendFileSync(${JSON.stringify(events)}, '${name}:start\\n');
  await new Promise(resolve => setTimeout(resolve, 200));
  fs.appendFileSync(${JSON.stringify(events)}, '${name}:end\\n');
});
`,
    );
    return file;
  });
  const env: NodeJS.ProcessEnv = {
    ...process.env,
    AGX_TEST_CONCURRENCY: "1",
  };
  delete env.NODE_TEST_CONTEXT;
  const result = spawnSync(
    process.execPath,
    [path.join(import.meta.dirname, "node-test.ts"), ...files],
    {
      env,
      encoding: "utf8",
      timeout: 15000,
    },
  );
  assert.equal(result.status, 0, result.stderr);
  const rows = fs.readFileSync(events, "utf8").trim().split("\n");
  let running = 0;
  let peak = 0;
  for (const row of rows) {
    running += row.endsWith(":start") ? 1 : -1;
    peak = Math.max(peak, running);
  }
  assert.equal(rows.length, 4);
  assert.equal(running, 0);
  assert.equal(peak, 1, `test files overlapped: ${rows.join(", ")}`);
});

test("the tools test script runs through the launcher", () => {
  const { scripts } = JSON.parse(
    fs.readFileSync(
      path.join(import.meta.dirname, "../../../../.github/tools/package.json"),
      "utf8",
    ),
  );
  assert.equal(
    scripts.test,
    'node src/scripts/node-test.ts "src/**/*.test.ts"',
  );
});

test("importing the launcher from evaluated code does not run it", () => {
  const script = path.join(import.meta.dirname, "node-test.ts");
  const source = `
    import { pathToFileURL } from "node:url";
    const launcher = await import(pathToFileURL(process.argv[1]).href);
    console.log(launcher.concurrency(2));
  `;
  const result = spawnSync(
    process.execPath,
    ["--input-type=module", "--eval", source, script, "missing.test.ts"],
    { encoding: "utf8", timeout: 10_000 },
  );
  assert.ifError(result.error);
  assert.equal(result.status, 0, result.stderr);
  assert.equal(result.stdout.trim(), "4");
});

test("the launcher discovers tests by glob and propagates child exits", (t) => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "agentics-node-test-"));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const script = path.join(import.meta.dirname, "node-test.ts");
  const fixtureDir = path.join(root, "src", "nested");
  fs.mkdirSync(fixtureDir, { recursive: true });
  const fixture = path.join(fixtureDir, "fixture.test.ts");
  // The subprocess is a new test runner, not a child file of this runner.
  const env = { ...process.env };
  delete env.NODE_TEST_CONTEXT;
  for (const fail of [false, true]) {
    fs.writeFileSync(
      fixture,
      `import test from 'node:test'; test('nested fixture ran', () => { ${fail ? "throw new Error('expected failure');" : ""} });\n`,
    );
    const result = spawnSync(process.execPath, [script, "src/**/*.test.ts"], {
      cwd: root,
      encoding: "utf8",
      timeout: 10_000,
      env,
    });
    assert.ifError(result.error);
    assert.equal(result.status, fail ? 1 : 0, result.stdout + result.stderr);
    assert.match(result.stdout, /nested fixture ran/);
  }
});
