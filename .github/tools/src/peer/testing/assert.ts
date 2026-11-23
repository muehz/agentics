import assert from "node:assert/strict";
import { existsSync, readFileSync } from "node:fs";
import type { Result } from "./harness.ts";

export function mustContain(
  label: string,
  body: string,
  ...wants: string[]
): void {
  for (const want of wants) {
    assert.ok(
      body.includes(want),
      `${label}: missing ${JSON.stringify(want)}\n---\n${body}\n---`,
    );
  }
}

export function mustNotContain(
  label: string,
  body: string,
  ...unwanted: string[]
): void {
  for (const text of unwanted) {
    assert.ok(
      !body.includes(text),
      `${label}: unexpected ${JSON.stringify(text)}\n---\n${body}\n---`,
    );
  }
}

export function mustExit(result: Result, want: number, what: string): void {
  assert.equal(
    result.code,
    want,
    `${what}: exit ${result.code}, want ${want}\n---\n${result.combined}\n---`,
  );
}

// Checks a fake's append-only invocation log: the record count and, when records are given, each one in order.
export function mustLog(
  path: string,
  want: number,
  ...records: string[]
): void {
  const text = readFileSync(path, "utf8").replace(/\n$/, "");
  const got = text === "" ? [] : text.split("\n");
  assert.equal(
    got.length,
    want,
    `${path}: ${got.length} record(s), want ${want}\n${got.join("\n")}`,
  );
  records.forEach((record, index) =>
    assert.equal(got[index], record, `${path} line ${index + 1}`),
  );
}

// Proves a fake never ran. A missing log and an empty one mean the same thing.
export function mustNotLog(path: string, what: string): void {
  if (!existsSync(path)) return;
  assert.equal(
    readFileSync(path, "utf8"),
    "",
    `${what}: expected no invocation`,
  );
}
