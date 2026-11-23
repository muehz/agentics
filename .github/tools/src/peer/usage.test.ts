import assert from "node:assert/strict";
import { describe, test } from "node:test";
import type { Host } from "./host.ts";
import { main } from "./run.ts";
import { mustContain, mustNotContain } from "./testing/assert.ts";
import { posix, runPeer } from "./testing/harness.ts";

describe("usage and argument handling", { skip: !posix }, () => {
  test("help describes the target sets and the exit codes", async () => {
    const res = await runPeer({ args: ["--help"] });
    assert.equal(res.code, 0);
    mustContain(
      "help",
      res.stdout,
      "Running in Claude Code → dispatches to Codex",
      "Running in Codex       → dispatches to Claude",
      "Running in Gemini/agy  → dispatches to Claude + Codex (parallel)",
      "all     Dispatch to all alternate Claude/Codex platforms",
      "claude  Dispatch via claude -p with read-only tool constraints",
      "codex   Dispatch via codex exec with --sandbox read-only",
      "0 success   1 dispatch/preflight failure   2 usage error   3 nothing to review",
      "124 review timed out",
    );
    mustNotContain(
      "help",
      res.stdout,
      "gemini",
      "--self-test",
      "--allow-advisory-agy-plan-mode",
      "agy --help",
      "dangerously",
    );
  });

  const usageErrors: Array<[string, string[]]> = [
    ["an invalid target", ["--target", "invalid", "--dry-run"]],
    ["the removed gemini target", ["--target", "gemini", "--dry-run"]],
    [
      "the removed gemini acknowledgement flag",
      ["--allow-advisory-agy-plan-mode", "--dry-run"],
    ],
    ["the removed self-test flag", ["--self-test"]],
    ["a value-taking flag without its value", ["--summary"]],
    ["a non-numeric timeout", ["--timeout", "soon"]],
    ["a zero timeout", ["--timeout", "0"]],
    ["an unknown argument", ["--frobnicate"]],
  ];
  for (const [name, args] of usageErrors) {
    test(`${name} is a usage error`, async () => {
      const res = await runPeer({ args, env: { CLAUDECODE: "1" } });
      assert.equal(res.code, 2, res.combined);
      mustContain(name, res.stderr, "ERROR:");
    });
  }

  test("an unknown argument prints the usage on stderr", async () => {
    const res = await runPeer({ args: ["--frobnicate"] });
    mustContain(
      "unknown argument",
      res.stderr,
      "ERROR: unknown argument: --frobnicate",
      "Usage: peer",
    );
  });
});

test("windows is refused with a clear error and a nonzero exit", async () => {
  let err = "";
  const host: Host = {
    env: {},
    cwd: process.cwd(),
    platform: "win32",
    out: () => undefined,
    err: (text) => void (err += text),
  };
  const code = await main(["--dry-run"], host);
  assert.equal(code, 1);
  mustContain("win32 refusal", err, "Windows");
});
