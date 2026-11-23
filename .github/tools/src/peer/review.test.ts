import assert from "node:assert/strict";
import { chmodSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { test } from "node:test";
import { checkReviewOutput } from "./review.ts";
import { scratchDir } from "./testing/harness.ts";

const dir = scratchDir("agentics-peer-review-");
const check = (name: string, body: string) => {
  const file = join(dir, name);
  writeFileSync(file, body);
  return checkReviewOutput(file);
};

const valid: Array<[string, string]> = [
  ["no findings", "body\nREVIEW-COMPLETE: no-findings\n"],
  ["CRLF zero findings", "body\r\nREVIEW-COMPLETE: findings=0\r\n"],
  ["no final newline", "REVIEW-COMPLETE: findings=12"],
  ["two findings", "body\nREVIEW-COMPLETE: findings=2\n"],
];
for (const [name, body] of valid) {
  test(`a valid terminator is accepted: ${name}`, () =>
    assert.deepEqual(check(`v-${name}`, body), { ok: true }));
}

const invalid: Array<[string, string]> = [
  ["missing", "unfinished review\n"],
  ["extra blank line", "REVIEW-COMPLETE: no-findings\n\n"],
  [
    "two markers one line",
    "REVIEW-COMPLETE: no-findings REVIEW-COMPLETE: no-findings\n",
  ],
  [
    "quoted then valid",
    "quoted REVIEW-COMPLETE: no-findings\nREVIEW-COMPLETE: no-findings\n",
  ],
  [
    "duplicate valid lines",
    "REVIEW-COMPLETE: no-findings\nREVIEW-COMPLETE: no-findings\n",
  ],
  ["early marker", "REVIEW-COMPLETE: no-findings\nafter\n"],
  ["blockquoted", "> REVIEW-COMPLETE: no-findings\n"],
  ["fenced", "```\nREVIEW-COMPLETE: no-findings\n```\n"],
  ["leading space", " REVIEW-COMPLETE: no-findings\n"],
  ["trailing space", "REVIEW-COMPLETE: no-findings \n"],
  ["negative count", "REVIEW-COMPLETE: findings=-1\n"],
  ["nonnumeric count", "REVIEW-COMPLETE: findings=many\n"],
  ["raw envelope", '{"text":"REVIEW-COMPLETE: no-findings"}\n'],
  ["escaped envelope", '{"text":"body\\nREVIEW-COMPLETE: no-findings\\n"}\n'],
  ["whitespace", " \t\n"],
];
for (const [name, body] of invalid) {
  test(`an invalid terminator is rejected: ${name}`, () =>
    assert.deepEqual(check(`i-${name}`, body), {
      ok: false,
      reason: "terminator",
    }));
}

test("a path that is not a readable file fails the terminator check instead of throwing", () => {
  assert.deepEqual(checkReviewOutput(dir), {
    ok: false,
    reason: "terminator",
  });
});

test("an empty file and a missing file are reported as empty", () => {
  assert.deepEqual(check("empty", ""), { ok: false, reason: "empty" });
  assert.deepEqual(checkReviewOutput(join(dir, "does-not-exist")), {
    ok: false,
    reason: "empty",
  });
});

test(
  "a file that denies reading fails the terminator check instead of throwing",
  {
    skip: process.platform === "win32" || process.getuid?.() === 0,
  },
  () => {
    const locked = join(dir, "locked");
    writeFileSync(locked, "body\nREVIEW-COMPLETE: no-findings\n");
    chmodSync(locked, 0o000);
    assert.deepEqual(checkReviewOutput(locked), {
      ok: false,
      reason: "terminator",
    });
  },
);
