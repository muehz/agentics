import assert from "node:assert/strict";
import { existsSync, mkdirSync, readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { describe, test } from "node:test";
import { mustContain, mustExit, mustNotLog } from "./testing/assert.ts";
import { installFakes, type Fakes } from "./testing/fakes.ts";
import { newWorkRepo, posix, runPeer } from "./testing/harness.ts";

describe("multi-target runs", { skip: !posix }, () => {
  const both = (
    f: Fakes,
    extra: Record<string, string> = {},
  ): Record<string, string> => ({
    PEER_REVIEW_LAUNCHER: f.path("claude"),
    PEER_REVIEW_CODEX_LAUNCHER: f.path("codex"),
    PEER_HEARTBEAT_INTERVAL: "0",
    ...extra,
  });

  test("a durable manifest records both targets", async () => {
    const f = installFakes();
    const outDir = f.path("multi");
    const res = await runPeer({
      dir: newWorkRepo(),
      args: ["--target", "all", "--output", outDir, "--summary", "multi live"],
      env: both(f),
    });
    mustExit(res, 0, "multi-target run with valid fakes");
    mustContain(
      "codex output",
      readFileSync(join(outDir, "codex.md"), "utf8"),
      "REVIEW-COMPLETE:",
    );
    mustContain(
      "claude output",
      readFileSync(join(outDir, "claude.md"), "utf8"),
      "REVIEW-COMPLETE:",
    );
    mustContain(
      "summary",
      res.combined,
      "=== Peer review complete ===",
      "Manifest:",
      // The fakes' known output: claude prints 2 lines, codex writes 3 (the Range line, the review, the terminator).
      `claude → ${join(outDir, "claude.md")} (2 lines)`,
      `codex → ${join(outDir, "codex.md")} (3 lines)`,
    );
    const manifest = readFileSync(join(outDir, "manifest.json"), "utf8");
    assert.equal(
      manifest.split('"status": "complete"').length - 1,
      2,
      manifest,
    );
    const parsed = JSON.parse(manifest) as {
      range: string;
      timeout_seconds: number;
      targets: Record<string, { exit_code: number; output: string }>;
    };
    assert.equal(parsed.range, "HEAD~1..HEAD");
    assert.equal(parsed.timeout_seconds, 300);
    assert.equal(parsed.targets.codex?.output, join(outDir, "codex.md"));
    assert.equal(parsed.targets.codex?.exit_code, 0);
  });

  // One target's invalid output fails the run overall without discarding the other's completed review.
  test("failures stay isolated", async () => {
    const f = installFakes();
    const outDir = f.path("mixed");
    const res = await runPeer({
      dir: newWorkRepo(),
      args: ["--target", "all", "--output", outDir, "--summary", "mixed live"],
      env: both(f, { FAKE_CLAUDE_MODE: "truncated" }),
    });
    mustExit(res, 1, "one invalid target");
    mustContain(
      "codex output",
      readFileSync(join(outDir, "codex.md"), "utf8"),
      "REVIEW-COMPLETE:",
    );
    mustContain(
      "mixed summary",
      res.combined,
      "claude → FAILED (output validation",
      "WARNING: 1 target(s) failed",
    );
    mustContain(
      "mixed manifest",
      readFileSync(join(outDir, "manifest.json"), "utf8"),
      '"status": "complete"',
      '"status": "invalid-output"',
    );
  });

  // The manifest is the durable record, so a write that fails is one clear error, and no reviewer starts without it.
  test("a manifest that cannot be written is reported and nothing is dispatched", async () => {
    const f = installFakes();
    const out = f.path("blocked");
    mkdirSync(join(out, "manifest.json.tmp"), { recursive: true });
    const res = await runPeer({
      dir: newWorkRepo(),
      args: ["--target", "all", "--output", out, "--summary", "no manifest"],
      env: both(f),
    });
    mustExit(res, 1, "unwritable manifest");
    mustContain("unwritable manifest", res.stderr, "ERROR: cannot write");
    assert.ok(!res.stderr.includes("\n    at "), res.stderr);
    mustNotLog(f.path("claude-invocations"), "no reviewer starts");
    mustNotLog(f.path("codex-invocations"), "no reviewer starts");
  });

  // peer.sh runs the targets as parallel jobs. Two reviews that each take 3 seconds finish in about 3 seconds
  // together, and would take 6 or more one after the other.
  test("the targets run in parallel", async () => {
    const f = installFakes();
    const start = Date.now();
    const res = await runPeer({
      dir: newWorkRepo(),
      args: ["--target", "all", "--output", f.path("par"), "--summary", "par"],
      env: both(f, { FAKE_CLAUDE_MODE: "slow", FAKE_CODEX_MODE: "slow" }),
    });
    const elapsed = Date.now() - start;
    mustExit(res, 0, "two slow reviews");
    assert.ok(
      elapsed < 5_000,
      `two 3s reviews took ${elapsed}ms: not parallel`,
    );
  });

  test("a single explicit output file writes no manifest", async () => {
    const f = installFakes();
    const out = f.path("single.md");
    const res = await runPeer({
      dir: newWorkRepo(),
      args: ["--target", "codex", "--output", out, "--summary", "single"],
      env: both(f),
    });
    mustExit(res, 0, "single explicit output");
    assert.equal(existsSync(join(dirname(out), "manifest.json")), false);
  });

  test("the default output directory sits under the working directory", async () => {
    const f = installFakes();
    const res = await runPeer({
      dir: newWorkRepo(),
      args: ["--target", "all", "--summary", "default dir"],
      env: both(f),
    });
    mustExit(res, 0, "default output directory");
    const line = res.stderr
      .split("\n")
      .find((entry) => entry.startsWith("peer: output dir "));
    assert.ok(line, res.stderr);
    const dir = line.slice("peer: output dir ".length).split(" (")[0] ?? "";
    // The slug turns every byte outside A-Za-z0-9._- into a hyphen, so HEAD~1..HEAD becomes HEAD-1..HEAD.
    assert.match(
      dir,
      /\/docs\/reviews\/peer\/\d{8}T\d{6}Z-\d+-HEAD-1\.\.HEAD$/,
    );
    assert.ok(existsSync(join(dir, "manifest.json")), `no manifest in ${dir}`);
  });

  // A slow but valid review still reports progress, so a long run is distinguishable from a hang.
  test("the heartbeat ticks while a review runs", async () => {
    const f = installFakes();
    const res = await runPeer({
      dir: newWorkRepo(),
      args: [
        "--target",
        "codex",
        "--output",
        f.path("heartbeat-out.md"),
        "--summary",
        "heartbeat",
      ],
      env: both(f, { FAKE_CODEX_MODE: "slow", PEER_HEARTBEAT_INTERVAL: "1" }),
    });
    mustExit(res, 0, "slow but valid review");
    mustContain(
      "heartbeat",
      res.combined,
      "peer: reviews still running (1s elapsed; timeout 300s per target)",
    );
  });

  test("a non-integer heartbeat interval warns and falls back to the default", async () => {
    const f = installFakes();
    const res = await runPeer({
      dir: newWorkRepo(),
      args: [
        "--target",
        "codex",
        "--output",
        f.path("hb-out.md"),
        "--summary",
        "bad interval",
      ],
      env: both(f, { PEER_HEARTBEAT_INTERVAL: "often" }),
    });
    mustExit(res, 0, "bad heartbeat interval");
    mustContain(
      "warning",
      res.stderr,
      "WARNING: PEER_HEARTBEAT_INTERVAL='often' is not a non-negative integer; using 30",
    );
  });

  // The directory branch creates the output directory itself, so a path beneath a regular file must be reported.
  test("a multi-target output directory that cannot be created is reported", async () => {
    const f = installFakes();
    const res = await runPeer({
      dir: newWorkRepo(),
      args: [
        "--target",
        "all",
        "--output",
        `${f.path("codex")}/sub`,
        "--summary",
        "no dir",
      ],
      env: both(f),
    });
    mustExit(res, 1, "output directory under a regular file");
    mustContain("mkdir failure", res.stderr, "ERROR: cannot create");
    assert.equal(
      res.stderr.includes("    at "),
      false,
      `a stack trace leaked: ${res.stderr}`,
    );
  });
});
