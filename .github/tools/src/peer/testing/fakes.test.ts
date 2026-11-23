import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { describe, test } from "node:test";
import { mustLog } from "./assert.ts";
import { installFakes } from "./fakes.ts";
import { newWorkRepo, posix } from "./harness.ts";

describe("test harness", { skip: !posix }, () => {
  test("the fakes record invocations and review output", () => {
    const fakes = installFakes();
    assert.match(
      execFileSync(fakes.path("codex"), ["--version"], { encoding: "utf8" }),
      /fake 1\.0/,
    );
    const out = `${fakes.dir}/out.md`;
    execFileSync(
      fakes.path("codex"),
      ["exec", "--output-last-message", out, "-"],
      { input: "Range: A..B\n" },
    );
    mustLog(fakes.path("codex-invocations"), 2, "version", "dispatch");
    assert.match(
      readFileSync(out, "utf8"),
      /^Range: A\.\.B\n.*\nREVIEW-COMPLETE: no-findings\n$/s,
    );
  });

  test("newWorkRepo builds a base commit and a head commit", () => {
    const repo = newWorkRepo();
    const log = execFileSync("git", ["-C", repo, "log", "--format=%s"], {
      encoding: "utf8",
    });
    assert.equal(log, "head\nbase\n");
  });
});
