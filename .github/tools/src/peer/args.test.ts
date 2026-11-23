import assert from "node:assert/strict";
import { test } from "node:test";
import { parseArgs, UsageError } from "./args.ts";

function timeoutOf(seconds: string): number {
  const parsed = parseArgs(["--timeout", seconds]);
  assert.equal(parsed.kind, "run");
  return parsed.kind === "run" ? parsed.options.timeout : -1;
}

test("--timeout accepts a positive integer of seconds", () => {
  assert.equal(timeoutOf("90"), 90);
  assert.equal(timeoutOf("9007199254740991"), 9007199254740991);
});

// A digit string passes the spelling check yet converts to Infinity or a rounded value; an infinite timeout would
// let a hung reviewer run forever.
test("--timeout rejects a value that is not a positive safe integer", () => {
  for (const value of [
    "0",
    "-1",
    "1.5",
    "soon",
    "9007199254740993",
    "9".repeat(400),
  ]) {
    assert.throws(
      () => parseArgs(["--timeout", value]),
      (error: unknown) =>
        error instanceof UsageError &&
        error.message === "--timeout must be a positive integer of seconds",
      value,
    );
  }
});
