import assert from "node:assert/strict";
import { chmodSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { test } from "node:test";
import { readRegularFile } from "./regular-file.ts";
import { scratchDir } from "./testing/harness.ts";

const dir = scratchDir("agentics-peer-regularfile-");

// Permission bits do not stop root, and Windows ignores them.
const canDenyRead = process.platform !== "win32" && process.getuid?.() !== 0;

test("a regular file is read, empty or not", () => {
  const full = join(dir, "full");
  writeFileSync(full, "a\nb\n");
  assert.deepEqual(readRegularFile(full), { kind: "text", text: "a\nb\n" });
  const empty = join(dir, "empty");
  writeFileSync(empty, "");
  assert.deepEqual(readRegularFile(empty), { kind: "text", text: "" });
});

test("a missing path is missing, including under a path that is not a directory", () => {
  assert.deepEqual(readRegularFile(join(dir, "absent")), { kind: "missing" });
  const plain = join(dir, "plain");
  writeFileSync(plain, "x");
  assert.deepEqual(readRegularFile(join(plain, "below")), { kind: "missing" });
});

test("a directory is unreadable rather than a throw", () => {
  assert.deepEqual(readRegularFile(dir), { kind: "unreadable" });
});

test(
  "a file that denies reading is unreadable rather than a throw",
  {
    skip: !canDenyRead,
  },
  () => {
    const locked = join(dir, "locked");
    writeFileSync(locked, "secret\n");
    chmodSync(locked, 0o000);
    assert.deepEqual(readRegularFile(locked), { kind: "unreadable" });
  },
);
