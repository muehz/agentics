import assert from "node:assert/strict";
import { readFileSync, writeFileSync } from "node:fs";
import { describe, test } from "node:test";
import {
  mustContain,
  mustExit,
  mustLog,
  mustNotContain,
} from "./testing/assert.ts";
import { installFakes } from "./testing/fakes.ts";
import { newWorkRepo, posix, runPeer, scale } from "./testing/harness.ts";

const payloads: Array<[string, string, boolean]> = [
  ["no findings", "body\nREVIEW-COMPLETE: no-findings\n", true],
  ["CRLF zero findings", "body\r\nREVIEW-COMPLETE: findings=0\r\n", true],
  ["no final newline", "REVIEW-COMPLETE: findings=12", true],
  ["two findings", "body\nREVIEW-COMPLETE: findings=2\n", true],
  ["missing", "unfinished review\n", false],
  ["extra blank line", "REVIEW-COMPLETE: no-findings\n\n", false],
  [
    "two markers one line",
    "REVIEW-COMPLETE: no-findings REVIEW-COMPLETE: no-findings\n",
    false,
  ],
  [
    "quoted then valid",
    "quoted REVIEW-COMPLETE: no-findings\nREVIEW-COMPLETE: no-findings\n",
    false,
  ],
  [
    "duplicate valid lines",
    "REVIEW-COMPLETE: no-findings\nREVIEW-COMPLETE: no-findings\n",
    false,
  ],
  ["early marker", "REVIEW-COMPLETE: no-findings\nafter\n", false],
  ["blockquoted", "> REVIEW-COMPLETE: no-findings\n", false],
  ["fenced", "```\nREVIEW-COMPLETE: no-findings\n```\n", false],
  ["leading space", " REVIEW-COMPLETE: no-findings\n", false],
  ["trailing space", "REVIEW-COMPLETE: no-findings \n", false],
  ["negative count", "REVIEW-COMPLETE: findings=-1\n", false],
  ["nonnumeric count", "REVIEW-COMPLETE: findings=many\n", false],
  ["raw envelope", '{"text":"REVIEW-COMPLETE: no-findings"}\n', false],
  [
    "escaped envelope",
    '{"text":"body\\nREVIEW-COMPLETE: no-findings\\n"}\n',
    false,
  ],
  ["whitespace", " \t\n", false],
  ["empty", "", false],
];

const codexEnv = (
  launcher: string,
  extra: Record<string, string> = {},
): Record<string, string> => ({
  CLAUDECODE: "1",
  PEER_REVIEW_CODEX_LAUNCHER: launcher,
  PEER_HEARTBEAT_INTERVAL: "0",
  ...extra,
});

describe("live dispatch", { skip: !posix }, () => {
  for (const target of ["claude", "codex"] as const) {
    describe(`the review terminator through ${target}`, () => {
      for (const [name, payload, valid] of payloads) {
        test(name, async () => {
          const f = installFakes();
          writeFileSync(f.path("review-payload"), payload);
          const env: Record<string, string> = {
            CLAUDECODE: "1",
            PEER_HEARTBEAT_INTERVAL: "0",
          };
          if (target === "claude") {
            env.PEER_REVIEW_LAUNCHER = f.path("claude");
            env.FAKE_CLAUDE_MODE = "payload";
          } else {
            env.PEER_REVIEW_CODEX_LAUNCHER = f.path("codex");
            env.FAKE_CODEX_MODE = "payload";
          }
          const res = await runPeer({
            dir: newWorkRepo(),
            args: [
              "--target",
              target,
              "--output",
              f.path("review.md"),
              "--summary",
              "literal review",
            ],
            env,
          });
          if (valid) {
            mustExit(res, 0, "valid final terminator");
          } else {
            mustExit(res, 4, "invalid final terminator");
            mustContain(
              "terminator rejection",
              res.combined,
              payload === ""
                ? "review output missing or empty"
                : "requires exactly one REVIEW-COMPLETE terminator on the final line",
            );
          }
          assert.equal(
            readFileSync(f.path("review.md"), "utf8"),
            payload,
            "the review file must equal the payload",
          );
        });
      }
    });
  }

  test("a live codex dispatch writes a validated output and passes exactly the pinned flags", async () => {
    const f = installFakes();
    const out = f.path("codex-out.md");
    const res = await runPeer({
      dir: newWorkRepo(),
      args: ["--target", "codex", "--output", out, "--summary", "live codex"],
      env: codexEnv(f.path("codex")),
    });
    mustExit(res, 0, "live codex dispatch");
    // The Range line went in over stdin and came back out through --output-last-message, so the prompt
    // survived the watchdog.
    mustContain(
      "codex output",
      readFileSync(out, "utf8"),
      "REVIEW-COMPLETE:",
      "Range: HEAD~1..HEAD",
    );
    assert.equal(
      readFileSync(f.path("codex-argv"), "utf8"),
      `exec -m gpt-6.1-sol --sandbox read-only --output-last-message ${out} -\n`,
    );
    mustLog(f.path("codex-invocations"), 2, "version", "dispatch");
  });

  test("a live claude dispatch passes the pinned flags and tees the review", async () => {
    const f = installFakes();
    const out = f.path("claude-out.md");
    const res = await runPeer({
      dir: newWorkRepo(),
      args: ["--target", "claude", "--output", out, "--summary", "live claude"],
      env: {
        CLAUDECODE: "1",
        PEER_REVIEW_LAUNCHER: f.path("claude"),
        PEER_HEARTBEAT_INTERVAL: "0",
      },
    });
    mustExit(res, 0, "live claude dispatch");
    mustContain("claude stdout", res.stdout, "Fake claude review.");
    assert.equal(
      readFileSync(out, "utf8"),
      "Fake claude review. No findings.\nREVIEW-COMPLETE: no-findings\n",
    );
    assert.equal(
      readFileSync(f.path("claude-argv"), "utf8"),
      "-p --model fable --fallback-model opus --output-format text --allowedTools Bash(git diff:*),Bash(git show:*),Bash(git status:*),Bash(git log:*),Bash(git blame:*),Read,Grep,Glob --no-session-persistence\n",
    );
  });

  // The fallback message names the missing CLI and points at the target that is available from this platform.
  test("a missing launcher suggests the other family", async () => {
    for (const [target, env, missing, other] of [
      ["codex", { PEER_REVIEW_CODEX_LAUNCHER: "" }, "codex", "claude"],
      ["claude", { PEER_REVIEW_LAUNCHER: "" }, "claude", "codex"],
    ] as const) {
      const res = await runPeer({
        dir: newWorkRepo(),
        args: ["--target", target, "--summary", "missing"],
        env: { CLAUDECODE: "1", ...env },
      });
      mustExit(res, 1, `missing ${missing} launcher`);
      mustContain(
        `missing ${missing}`,
        res.combined,
        `${missing} is not installed`,
        `--target ${other}`,
        "Current platform: claude-code",
      );
    }
  });

  test("truncated output fails validation", async () => {
    const f = installFakes();
    const res = await runPeer({
      dir: newWorkRepo(),
      args: [
        "--target",
        "claude",
        "--output",
        f.path("truncated-out.md"),
        "--summary",
        "truncated",
      ],
      env: {
        CLAUDECODE: "1",
        PEER_REVIEW_LAUNCHER: f.path("claude"),
        FAKE_CLAUDE_MODE: "truncated",
        PEER_HEARTBEAT_INTERVAL: "0",
      },
    });
    mustExit(res, 4, "output missing the terminator");
    mustContain("truncated output", res.combined, "REVIEW-COMPLETE terminator");
  });

  // The most common real cause of a launcher that cannot report its version is a sandboxed shell.
  test("a failed version preflight explains sandboxing and never dispatches", async () => {
    const f = installFakes();
    const res = await runPeer({
      dir: newWorkRepo(),
      args: [
        "--target",
        "codex",
        "--output",
        f.path("badver-out.md"),
        "--summary",
        "preflight",
      ],
      env: codexEnv(f.path("codex"), { FAKE_CODEX_MODE: "bad-version" }),
    });
    mustExit(res, 1, "failed --version preflight");
    mustContain("version preflight", res.combined, "unsandboxed");
    mustLog(f.path("codex-invocations"), 1, "version");
  });

  test("a hung reviewer is killed at the timeout and exits 124", async () => {
    const f = installFakes();
    const start = Date.now();
    const res = await runPeer({
      dir: newWorkRepo(),
      args: [
        "--target",
        "codex",
        "--timeout",
        "1",
        "--output",
        f.path("slow-out.md"),
        "--summary",
        "hang",
      ],
      env: codexEnv(f.path("codex"), { FAKE_CODEX_MODE: "hang" }),
    });
    mustExit(res, 124, "hung reviewer");
    assert.ok(
      Date.now() - start < scale(30_000),
      "the watchdog must kill the run promptly",
    );
    mustContain("watchdog", res.combined, "timed out after 1s");
  });

  // A caller that pipes the tool (`peer ... | tee log.txt`, `| less`, a CI step) must see the run end when the
  // review does. A leftover timer or child holding the pipe would withhold EOF for a full heartbeat interval.
  // The interval is pinned at 10s: the review takes well under a second, so the case separates cleanly.
  test("a piped run ends when the review does", async () => {
    const f = installFakes();
    const out = f.path("piped-out.md");
    const start = Date.now();
    const res = await runPeer({
      dir: newWorkRepo(),
      pipe: true,
      args: ["--target", "codex", "--output", out, "--summary", "piped"],
      env: codexEnv(f.path("codex"), { PEER_HEARTBEAT_INTERVAL: "10" }),
    });
    const elapsed = Date.now() - start;
    mustExit(res, 0, "piped codex dispatch");
    mustContain("piped output", readFileSync(out, "utf8"), "REVIEW-COMPLETE:");
    assert.ok(
      elapsed < 5_000,
      `the piped run took ${elapsed}ms; something holds the pipe open after the review finished`,
    );
  });

  // The target is another vendor's binary, so a host session token or a rival vendor's key reaching even its
  // version probe has already left the host. Every invocation of every target is inspected.
  test("each target receives only its own vendor's credentials, on the version probe and the dispatch", async () => {
    const f = installFakes();
    const res = await runPeer({
      dir: newWorkRepo(),
      args: [
        "--target",
        "all",
        "--output",
        f.path("multi"),
        "--summary",
        "scrub",
      ],
      env: {
        PEER_REVIEW_LAUNCHER: f.path("claude"),
        PEER_REVIEW_CODEX_LAUNCHER: f.path("codex"),
        PEER_HEARTBEAT_INTERVAL: "0",
        CLAUDE_CODE_SESSION_ACCESS_TOKEN: "host-x",
        ANTHROPIC_API_KEY: "anth-x",
        OPENAI_API_KEY: "oai-x",
        CODEX_HOME: "/inert/codex-home",
        PLUGIN_ROOT: "/inherited/PLUGIN_ROOT",
        CLAUDE_PLUGIN_ROOT: "/inherited/CLAUDE_PLUGIN_ROOT",
      },
    });
    mustExit(res, 0, "scrubbed multi-target run");
    const claude =
      "ANTHROPIC_API_KEY=anth-x|OPENAI_API_KEY=<unset>|CODEX_HOME=<unset>|HOST_SESSION=<unset>|PLUGIN_ROOT=<unset>|CLAUDE_PLUGIN_ROOT=<unset>";
    const codex =
      "ANTHROPIC_API_KEY=<unset>|OPENAI_API_KEY=oai-x|CODEX_HOME=/inert/codex-home|HOST_SESSION=<unset>|PLUGIN_ROOT=<unset>|CLAUDE_PLUGIN_ROOT=<unset>";
    mustLog(f.path("claude-environment"), 2, claude, claude);
    mustLog(f.path("codex-environment"), 2, codex, codex);
    mustLog(f.path("claude-invocations"), 2, "version", "dispatch");
  });

  // The review covers committed work only, so uncommitted changes must be called out and not silently dropped.
  test("a dirty tree warns without blocking", async () => {
    const f = installFakes();
    const repo = newWorkRepo();
    writeFileSync(`${repo}/file.txt`, "one\nuncommitted\n");
    const res = await runPeer({
      dir: repo,
      args: [
        "--target",
        "codex",
        "--output",
        f.path("dirty-out.md"),
        "--summary",
        "dirty",
      ],
      env: codexEnv(f.path("codex")),
    });
    mustExit(res, 0, "dirty work tree");
    mustContain("dirty work tree", res.combined, "NOT reviewed");
  });

  test("an oversized diff dispatches with --force", async () => {
    const f = installFakes();
    const res = await runPeer({
      dir: newWorkRepo(),
      args: [
        "--target",
        "codex",
        "--force",
        "--output",
        f.path("big-out.md"),
        "--summary",
        "big forced",
      ],
      env: codexEnv(f.path("codex"), { PEER_MAX_DIFF_BYTES: "10" }),
    });
    mustExit(res, 0, "oversized diff with --force");
    mustNotContain("oversized diff", res.combined, "ERROR:");
  });

  // `tee` under pipefail fails the run before validation when the review file cannot be written.
  test("an output path that cannot be written fails the claude dispatch cleanly", async () => {
    const f = installFakes();
    const res = await runPeer({
      dir: newWorkRepo(),
      args: [
        "--target",
        "claude",
        "--output",
        f.dir,
        "--summary",
        "unwritable",
      ],
      env: {
        CLAUDECODE: "1",
        PEER_REVIEW_LAUNCHER: f.path("claude"),
        PEER_HEARTBEAT_INTERVAL: "0",
      },
    });
    mustExit(res, 1, "unwritable claude output");
    mustContain("unwritable output", res.stderr, "ERROR: cannot write");
    mustNotContain("unwritable output", res.stderr, "at checkReviewOutput");
  });

  test("an output directory that cannot be created is reported", async () => {
    const f = installFakes();
    const res = await runPeer({
      dir: newWorkRepo(),
      args: [
        "--target",
        "codex",
        "--output",
        `${f.path("codex")}/sub/review.md`,
        "--summary",
        "no dir",
      ],
      env: codexEnv(f.path("codex")),
    });
    mustExit(res, 1, "output directory under a regular file");
    mustContain("mkdir failure", res.stderr, "ERROR: cannot create");
  });

  // The prompt reaches the reviewer's stdin with exactly one trailing newline, and Codex's transcript is shown.
  test("the prompt ends in one newline and the codex transcript reaches stdout", async () => {
    const f = installFakes();
    const out = f.path("transcript-out.md");
    const res = await runPeer({
      dir: newWorkRepo(),
      args: ["--target", "codex", "--output", out, "--summary", "stdin shape"],
      env: codexEnv(f.path("codex")),
    });
    mustExit(res, 0, "codex dispatch");
    mustContain("transcript", res.stdout, "fake codex transcript");
    const stdin = readFileSync(f.path("codex-stdin"), "utf8");
    assert.ok(
      stdin.endsWith("\n") && !stdin.endsWith("\n\n"),
      JSON.stringify(stdin.slice(-40)),
    );
    mustContain("prompt", stdin, "Peer review request", "Summary: stdin shape");
  });

  // The same unwritable path under codex: its review file is never written, so validation fails with exit 4 and no crash.
  test("a codex output path that is a directory fails validation without a crash", async () => {
    const f = installFakes();
    const res = await runPeer({
      dir: newWorkRepo(),
      args: ["--target", "codex", "--output", f.dir, "--summary", "dir output"],
      env: codexEnv(f.path("codex"), { FAKE_CODEX_MODE: "payload" }),
    });
    mustExit(res, 4, "codex output path is a directory");
    mustContain(
      "directory output",
      res.stderr,
      "requires exactly one REVIEW-COMPLETE terminator",
    );
    mustNotContain("directory output", res.stderr, "at checkReviewOutput");
  });

  test("a launcher that cannot be started is reported", async () => {
    const res = await runPeer({
      dir: newWorkRepo(),
      args: ["--target", "codex", "--summary", "unstartable"],
      env: codexEnv("/no/such/launcher"),
    });
    mustExit(res, 1, "launcher that cannot start");
    mustContain(
      "unstartable",
      res.combined,
      "ERROR: cannot run /no/such/launcher",
    );
  });
});
