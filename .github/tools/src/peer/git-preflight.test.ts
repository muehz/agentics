import { execFileSync } from "node:child_process";
import { describe, test } from "node:test";
import { DEFAULT_EXCLUDES } from "./constants.ts";
import { mustContain, mustExit } from "./testing/assert.ts";
import { installFakes } from "./testing/fakes.ts";
import { newWorkRepo, posix, runPeer, scratchDir } from "./testing/harness.ts";

// The git preflight runs before any launcher is reached, and each refusal has its own exit code so a caller can
// tell "nothing to review" from "the range is wrong" from "the diff is too large".
describe("git preflight", { skip: !posix }, () => {
  const codexEnv = (
    launcher: string,
    extra: Record<string, string> = {},
  ): Record<string, string> => ({
    CLAUDECODE: "1",
    PEER_REVIEW_CODEX_LAUNCHER: launcher,
    ...extra,
  });

  test("an empty range has its own exit code", async () => {
    const f = installFakes();
    const res = await runPeer({
      dir: newWorkRepo(),
      args: [
        "--target",
        "codex",
        "--base",
        "HEAD",
        "--head",
        "HEAD",
        "--summary",
        "empty",
      ],
      env: codexEnv(f.path("codex")),
    });
    mustExit(res, 3, "empty range");
    mustContain("empty range", res.combined, "Nothing to review");
  });

  test("an unresolvable revision fails before dispatch", async () => {
    const f = installFakes();
    const res = await runPeer({
      dir: newWorkRepo(),
      args: [
        "--target",
        "codex",
        "--base",
        "no-such-rev",
        "--summary",
        "bad rev",
      ],
      env: codexEnv(f.path("codex")),
    });
    mustExit(res, 1, "unresolvable base revision");
    mustContain("unresolvable base revision", res.combined, "does not resolve");
  });

  test("a directory outside any repository fails before dispatch", async () => {
    const f = installFakes();
    const res = await runPeer({
      dir: scratchDir("agentics-peer-norepo-"),
      args: ["--target", "codex", "--summary", "no repo"],
      env: codexEnv(f.path("codex")),
    });
    mustExit(res, 1, "not a git repository");
    mustContain(
      "not a git repository",
      res.combined,
      "not inside a git repository",
    );
  });

  test("an unresolvable head names the head flag", async () => {
    const f = installFakes();
    const res = await runPeer({
      dir: newWorkRepo(),
      args: ["--target", "codex", "--head", "no-such-rev", "--summary", "bad"],
      env: codexEnv(f.path("codex")),
    });
    mustExit(res, 1, "unresolvable head revision");
    mustContain(
      "unresolvable head",
      res.combined,
      "--head 'no-such-rev' does not resolve",
    );
  });

  test("a range whose every change is excluded has the nothing-to-review exit code", async () => {
    const f = installFakes();
    const res = await runPeer({
      dir: newWorkRepo(),
      args: [
        "--target",
        "codex",
        "--exclude",
        "file.txt",
        "--summary",
        "all excluded",
      ],
      env: codexEnv(f.path("codex")),
    });
    mustExit(res, 3, "every change excluded");
    mustContain("every change excluded", res.combined, "after exclusions");
  });

  // The limit is a strict greater-than, and the count is taken with the exclude pathspecs applied.
  test("a diff of exactly the limit passes and one byte over is refused", async () => {
    const f = installFakes();
    const repo = newWorkRepo();
    const size = execFileSync(
      "git",
      [
        "-C",
        repo,
        "diff",
        "HEAD~1",
        "HEAD",
        "--",
        ".",
        ...DEFAULT_EXCLUDES.map((pattern) => `:(exclude,glob)${pattern}`),
      ],
      { maxBuffer: 1 << 20 },
    ).length;
    const run = (limit: number) =>
      runPeer({
        dir: repo,
        args: [
          "--target",
          "codex",
          "--output",
          f.path(`limit-${limit}.md`),
          "--summary",
          "limit",
        ],
        env: codexEnv(f.path("codex"), {
          PEER_MAX_DIFF_BYTES: String(limit),
          PEER_HEARTBEAT_INTERVAL: "0",
        }),
      });
    mustExit(await run(size), 0, "diff of exactly the limit");
    const over = await run(size - 1);
    mustExit(over, 1, "diff one byte over the limit");
    mustContain("one byte over", over.combined, `is ${size} bytes`);
  });

  // The size guard is measured after the exclusion pathspecs are applied, and --force is the explicit override.
  test("an oversized diff is refused without --force", async () => {
    const f = installFakes();
    const res = await runPeer({
      dir: newWorkRepo(),
      args: [
        "--target",
        "codex",
        "--output",
        f.path("big-out.md"),
        "--summary",
        "big",
      ],
      env: codexEnv(f.path("codex"), { PEER_MAX_DIFF_BYTES: "10" }),
    });
    mustExit(res, 1, "oversized diff without --force");
    mustContain("oversized diff", res.combined, "--force");
  });
});
