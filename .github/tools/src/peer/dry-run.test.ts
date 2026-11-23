import assert from "node:assert/strict";
import { writeFileSync } from "node:fs";
import { join } from "node:path";
import { describe, test } from "node:test";
import { mustContain, mustNotContain } from "./testing/assert.ts";
import { newWorkRepo, posix, runPeer, scratchDir } from "./testing/harness.ts";

interface GateCase {
  name: string;
  args: string[];
  env?: Record<string, string>;
  code?: number;
  want?: string[];
  deny?: string[];
  stderr?: string[];
}

// The launcher-override environment the dry-run cases share. These fake paths are only printed, never executed.
const hostEnv = (
  extra: Record<string, string> = {},
): Record<string, string> => ({
  PEER_REVIEW_LAUNCHER: "test-claude",
  PEER_REVIEW_CODEX_LAUNCHER: "test-codex",
  ...extra,
});

const cases: GateCase[] = [
  {
    name: "claude dry run composes the prompt",
    args: [
      "--target",
      "claude",
      "--dry-run",
      "--base",
      "HEAD~1",
      "--head",
      "HEAD",
      "--summary",
      "smoke test",
    ],
    env: hostEnv({ CLAUDECODE: "1" }),
    want: [
      "Peer review request (claude)",
      "Detected platform: claude-code",
      "Launcher: test-claude",
      "Transport: stdin",
      "Model: fable (fallback: opus)",
      "Range: HEAD~1..HEAD",
      "Summary: smoke test",
      "Inspect only; do not edit files",
      "Findings first",
      "Bash(git diff:*)",
      "Read,Grep,Glob",
      ":(exclude,glob)**/package-lock.json",
      ":(exclude,glob)**/*.min.*",
      "do not read or review excluded paths",
      "out of scope by design",
      "REVIEW-COMPLETE: findings=<n>",
      "REVIEW-COMPLETE: no-findings",
    ],
    deny: ["/usr/bin/CC"],
  },
  {
    name: "codex dry run uses exec, carries the range in the prompt, and passes no hook-trust bypass",
    args: [
      "--target",
      "codex",
      "--dry-run",
      "--base",
      "HEAD~1",
      "--head",
      "HEAD",
      "--summary",
      "codex smoke test",
    ],
    env: hostEnv({ CLAUDECODE: "1" }),
    want: [
      "Peer review request (codex)",
      "Launcher: test-codex",
      "codex exec",
      "Model: gpt-6.1-sol",
      "Sandbox: read-only",
      "Range: HEAD~1..HEAD",
      "--sandbox read-only --output-last-message",
      "codex smoke test",
      "Findings first",
      "out of scope by design",
      "REVIEW-COMPLETE: findings=<n>",
    ],
    deny: [
      "codex review",
      "--ephemeral",
      "--commit",
      "--dangerously-bypass-hook-trust",
    ],
  },
  {
    name: "codex dry run in branch mode",
    args: [
      "--target",
      "codex",
      "--dry-run",
      "--base",
      "origin/main",
      "--head",
      "HEAD",
      "--summary",
      "branch review",
    ],
    env: hostEnv({ CLAUDECODE: "1" }),
    want: ["Range: origin/main..HEAD"],
    deny: ["--base origin/main", "--commit"],
  },
  {
    name: "exclude appends to the prompt pathspecs",
    args: [
      "--target",
      "codex",
      "--dry-run",
      "--exclude",
      "*.snap",
      "--summary",
      "exclude test",
    ],
    env: hostEnv({ CLAUDECODE: "1" }),
    want: [":(exclude,glob)*.snap", ":(exclude,glob)**/yarn.lock"],
  },
  {
    name: "timeout appears in the dry run",
    args: [
      "--target",
      "codex",
      "--dry-run",
      "--timeout",
      "60",
      "--summary",
      "timeout test",
    ],
    env: hostEnv({ CLAUDECODE: "1" }),
    want: ["Timeout: 60s"],
  },
  {
    name: "claude code auto-dispatches to codex",
    args: ["--dry-run", "--summary", "auto from claude"],
    env: hostEnv({ CLAUDECODE: "1" }),
    want: [
      "Model: gpt-6.1-sol",
      "Peer review request (codex)",
      "Detected platform: claude-code",
    ],
    deny: ["gemini"],
  },
  {
    name: "codex auto-dispatches to claude",
    args: ["--dry-run", "--summary", "auto from codex"],
    env: hostEnv({ CODEX_CI: "1" }),
    want: ["Peer review request (claude)", "Detected platform: codex"],
    deny: ["gemini"],
  },
  {
    name: "gemini as a source platform auto-dispatches to claude and codex",
    args: ["--dry-run", "--summary", "auto from gemini"],
    env: hostEnv({ ANTIGRAVITY_AGENT: "1" }),
    want: [
      "Peer review request (multi-target)",
      "Detected platform: gemini",
      "Targets: claude codex",
    ],
  },
  {
    // Detection priority: CODEX_CI beats ANTIGRAVITY_AGENT beats CLAUDECODE.
    name: "detection priority is codex then gemini then claude",
    args: ["--dry-run", "--summary", "priority test"],
    env: hostEnv({ CODEX_CI: "1", ANTIGRAVITY_AGENT: "1", CLAUDECODE: "1" }),
    want: ["Detected platform: codex", "Peer review request (claude)"],
    deny: ["gemini"],
  },
  {
    name: "explicit target overrides detection",
    args: ["--target", "claude", "--dry-run", "--summary", "forced claude"],
    env: hostEnv({ CLAUDECODE: "1" }),
    want: ["Model: fable (fallback: opus)", "Peer review request (claude)"],
  },
  {
    // Peer does not run on Copilot CLI, so its plugin root must not read as a detected platform.
    name: "copilot is not a platform",
    args: ["--dry-run", "--summary", "copilot not detected"],
    env: { COPILOT_PLUGIN_ROOT: "/fake/agentics" },
    code: 1,
    stderr: ["could not detect current platform"],
  },
  {
    name: "no platform is an error",
    args: ["--dry-run", "--summary", "no platform"],
    code: 1,
    stderr: ["could not detect current platform"],
  },
  {
    name: "target all dispatches to claude and codex from any shell",
    args: ["--target", "all", "--dry-run", "--summary", "all targets"],
    want: [
      "Peer review request (multi-target)",
      "Targets: claude codex",
      "--- claude ---",
      "--- codex ---",
    ],
    deny: ["--- gemini ---"],
  },
  {
    name: "target all from claude code",
    args: ["--target", "all", "--dry-run", "--summary", "all from claude"],
    env: hostEnv({ CLAUDECODE: "1" }),
    want: ["Targets: claude codex", "--- claude ---", "--- codex ---"],
  },
  {
    name: "target all from codex",
    args: ["--target", "all", "--dry-run", "--summary", "all from codex"],
    env: hostEnv({ CODEX_CI: "1" }),
    want: ["Targets: claude codex", "--- claude ---", "--- codex ---"],
  },
];

describe("everything decided before dispatch", { skip: !posix }, () => {
  const repo = newWorkRepo();
  for (const tc of cases) {
    test(tc.name, async () => {
      const res = await runPeer({ dir: repo, args: tc.args, env: tc.env });
      assert.equal(res.code, tc.code ?? 0, res.combined);
      mustContain(tc.name, res.stdout, ...(tc.want ?? []));
      mustNotContain(tc.name, res.stdout, ...(tc.deny ?? []));
      mustContain(tc.name, res.stderr, ...(tc.stderr ?? []));
    });
  }

  test("a missing launcher prints NOT FOUND in the dry run", async () => {
    const res = await runPeer({
      dir: repo,
      args: ["--target", "codex", "--dry-run"],
      env: { CLAUDECODE: "1", PEER_REVIEW_CODEX_LAUNCHER: "" },
    });
    assert.equal(res.code, 0);
    mustContain("not found", res.stdout, "Launcher: NOT FOUND");
  });

  test("the plan file's first 240 lines travel in the prompt", async () => {
    const res = await runPeer({
      dir: repo,
      args: ["--target", "codex", "--dry-run", "--plan", "file.txt"],
      env: hostEnv({ CLAUDECODE: "1" }),
    });
    mustContain(
      "plan contents",
      res.stdout,
      "Plan: file.txt",
      "Plan contents:\none\n",
    );
  });

  // `sed -n 1,240p` is the model: line 240 is the last one sent, and the file's own line endings are kept.
  test("a long plan file is cut at line 240 and a missing final newline stays missing", async () => {
    const dir = scratchDir("agentics-peer-plan-");
    const long = join(dir, "long.md");
    writeFileSync(
      long,
      Array.from({ length: 300 }, (_, index) => `line ${index + 1}`).join(
        "\n",
      ) + "\n",
    );
    const cut = await runPeer({
      dir: repo,
      args: ["--target", "codex", "--dry-run", "--plan", long],
      env: hostEnv({ CLAUDECODE: "1" }),
    });
    assert.equal(cut.code, 0, cut.combined);
    mustContain("long plan", cut.stdout, "line 240\n");
    mustNotContain("long plan", cut.stdout, "line 241");

    const bare = join(dir, "bare.md");
    writeFileSync(bare, "no final newline");
    const noNewline = await runPeer({
      dir: repo,
      args: ["--target", "codex", "--dry-run", "--plan", bare],
      env: hostEnv({ CLAUDECODE: "1" }),
    });
    assert.ok(
      noNewline.stdout.endsWith("Plan contents:\nno final newline"),
      noNewline.stdout.slice(-80),
    );
  });

  // A plan path that is not a readable file adds no contents section, and the run still succeeds.
  test("a missing plan path and a directory plan path add no plan contents", async () => {
    const dir = scratchDir("agentics-peer-plan-");
    for (const plan of [join(dir, "absent.md"), dir]) {
      const res = await runPeer({
        dir: repo,
        args: ["--target", "codex", "--dry-run", "--plan", plan],
        env: hostEnv({ CLAUDECODE: "1" }),
      });
      assert.equal(res.code, 0, res.combined);
      mustContain("plan path", res.stdout, `Plan: ${plan}`);
      mustNotContain("plan path", res.stdout, "Plan contents:");
    }
  });

  test("an empty plan file adds the heading and nothing after it", async () => {
    const empty = join(scratchDir("agentics-peer-plan-"), "empty.md");
    writeFileSync(empty, "");
    const res = await runPeer({
      dir: repo,
      args: ["--target", "codex", "--dry-run", "--plan", empty],
      env: hostEnv({ CLAUDECODE: "1" }),
    });
    assert.ok(res.stdout.endsWith("Plan contents:\n"), res.stdout.slice(-80));
  });
});
