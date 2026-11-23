import assert from "node:assert/strict";
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { test } from "node:test";
import { ENV_SHORT, errorMessage, MARKER, repoRoot } from "./check.ts";

function scratch(prefix: string): string {
  return mkdtempSync(path.join(tmpdir(), prefix));
}

test("the marker and the short-mode variable keep their names", () => {
  assert.equal(MARKER, ".claude-plugin/plugin.json");
  assert.equal(ENV_SHORT, "AGX_CONFORMANCE_SHORT");
});

test("repoRoot walks up from a nested directory to the marker", () => {
  const root = scratch("agentics-check-");
  try {
    mkdirSync(path.join(root, ".claude-plugin"), { recursive: true });
    writeFileSync(path.join(root, ".claude-plugin", "plugin.json"), "{}");
    const nested = path.join(root, "a", "b");
    mkdirSync(nested, { recursive: true });
    assert.equal(repoRoot(nested), root);
    assert.equal(repoRoot(root), root);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("repoRoot returns an empty string when no marker is above", () => {
  const dir = scratch("agentics-nomarker-");
  try {
    assert.equal(repoRoot(dir), "");
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
});

test("errorMessage gives an Error its message and any other value its string form", () => {
  assert.equal(errorMessage(new Error("boom")), "boom");
  assert.equal(errorMessage("plain"), "plain");
  assert.equal(errorMessage(42), "42");
  assert.equal(errorMessage(undefined), "undefined");
});
