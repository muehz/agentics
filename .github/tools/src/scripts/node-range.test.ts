import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import test from "node:test";

import { satisfies, parseRange } from "./node-range.ts";

const ROOT = path.resolve(import.meta.dirname, "../../../..");
const RANGE = "^22.23.5 || ^24.22.3 || >=26.12.1";

test("the repository range admits supported Node lines only", () => {
  for (const version of [
    "22.23.5",
    "22.99.0",
    "24.22.3",
    "24.99.1",
    "26.12.1",
    "27.0.0",
  ]) {
    assert.ok(satisfies(RANGE, version), `${version} must satisfy ${RANGE}`);
  }
  for (const version of ["22.23.4", "23.0.0", "24.22.2", "25.0.0", "26.12.0"]) {
    assert.ok(!satisfies(RANGE, version), `${version} must not satisfy`);
  }
});

test("a single >= range keeps its old meaning", () => {
  assert.ok(satisfies(">=24.22.0", "25.1.0"));
  assert.ok(!satisfies(">=24.22.0", "24.21.0"));
});

test("a caret range does not cross a major version", () => {
  assert.ok(!satisfies("^24.22.0", "25.0.0"));
  assert.ok(satisfies("^24.22.0", "24.30.0"));
});

test("unsupported ranges are rejected, not guessed", () => {
  for (const range of [
    "24.x",
    "~24.22.0",
    ">24.22.0",
    ">=24.22.0 <25",
    "^0.5.0",
    "",
  ]) {
    assert.throws(() => parseRange(range), /Unsupported Node range/, range);
  }
  assert.throws(() => satisfies(RANGE, "v26.12.1"), /Not a Node version/);
});

test("the command line exits 0, 1 or 2", () => {
  const run = (...args: string[]) =>
    spawnSync(
      process.execPath,
      [path.join(import.meta.dirname, "node-range.ts"), ...args],
      {
        encoding: "utf8",
      },
    );
  assert.equal(run(RANGE, "26.12.1").status, 0);
  assert.equal(run(RANGE, "25.1.0").status, 1);
  const bad = run("~24.22.0", "24.22.0");
  assert.equal(bad.status, 2);
  assert.match(bad.stderr, /Unsupported Node range/);
});

test("the manifests carry the range this helper understands", () => {
  const read = (file: string): string =>
    JSON.parse(fs.readFileSync(path.join(ROOT, file), "utf8")).engines.node;
  assert.equal(read("package.json"), RANGE);
  assert.equal(read(".github/tools/package.json"), RANGE);
  assert.equal(read(".github/release/package.json"), RANGE);
  // The pinned CI Node and the cloud-setup fallback must satisfy it.
  const workflow = fs.readFileSync(
    path.join(ROOT, ".github/workflows/validate.yml"),
    "utf8",
  );
  const ci = /node-version: (\d+\.\d+\.\d+)/.exec(workflow)?.[1];
  assert.ok(ci, "CI Node pin must be present");
  assert.ok(satisfies(RANGE, ci), `CI Node ${ci} must satisfy ${RANGE}`);
  const cloud = fs.readFileSync(
    path.join(ROOT, ".github/scripts/cloud-setup.sh"),
    "utf8",
  );
  const fallback = /NODE_FALLBACK_VERSION="(\d+\.\d+\.\d+)"/.exec(cloud)?.[1];
  assert.ok(fallback, "cloud fallback pin must be present");
  assert.ok(satisfies(RANGE, fallback), `fallback ${fallback} must satisfy`);
});
