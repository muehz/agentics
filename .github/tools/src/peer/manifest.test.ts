import assert from "node:assert/strict";
import { test } from "node:test";
import type { ReviewContext } from "./context.ts";
import { renderManifest } from "./manifest.ts";

// renderManifest reads only the range, the platform's source and the timeout from the context.
const ctx = {
  range: 'a b/"x"..HEAD',
  platform: { source: "claude-code" },
  options: { timeout: 45 },
} as unknown as ReviewContext;

// The layout is a durable interface: a restarted session reads this file instead of relaunching reviews.
test("the manifest text is exact for every status, with null for what has not finished", () => {
  const text = renderManifest(ctx, "2026-10-21T04:00:00Z", [
    { target: "claude", file: "/out/claude.md" },
    { target: "codex", file: "/out/codex.md", exit: 0, duration: 3 },
  ]);
  assert.equal(
    text,
    [
      "{",
      '  "range": "a b/\\"x\\"..HEAD",',
      '  "platform": "claude-code",',
      '  "started_utc": "2026-10-21T04:00:00Z",',
      '  "timeout_seconds": 45,',
      '  "targets": {',
      '    "claude": { "status": "running", "exit_code": null, "duration_seconds": null, "output": "/out/claude.md" },',
      '    "codex": { "status": "complete", "exit_code": 0, "duration_seconds": 3, "output": "/out/codex.md" }',
      "  }",
      "}",
      "",
    ].join("\n"),
  );
});

test("each exit code maps to its status", () => {
  const status = (exit: number) =>
    renderManifest(ctx, "t", [
      { target: "claude", file: "f", exit, duration: 1 },
    ]).match(/"status": "([a-z-]+)"/)?.[1];
  assert.equal(status(0), "complete");
  assert.equal(status(124), "timeout");
  assert.equal(status(4), "invalid-output");
  assert.equal(status(1), "failed");
  assert.equal(status(7), "failed");
});
