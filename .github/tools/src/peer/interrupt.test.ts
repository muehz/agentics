import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { setTimeout as sleep } from "node:timers/promises";
import { describe, test } from "node:test";
import { mustContain } from "./testing/assert.ts";
import { installFakes } from "./testing/fakes.ts";
import { newWorkRepo, posix, scale, startPeer } from "./testing/harness.ts";
import { pidAlive } from "./testing/ps.ts";

interface Case {
  name: string;
  signal: NodeJS.Signals;
  status: number;
  target: string;
  // Fixture files that prove a reviewer is running, and the pids inside its group.
  proofs: string[];
}

const claudeProofs = ["claude-leader.pid"];
const codexProofs = ["leader.pid", "descendant.pid", "sleeper.pid"];

const cases: Case[] = [
  {
    name: "SIGTERM stops both reviewer groups of a multi-target run",
    signal: "SIGTERM",
    status: 143,
    target: "all",
    proofs: [...claudeProofs, ...codexProofs],
  },
  {
    name: "SIGINT stops the reviewer group of a single target",
    signal: "SIGINT",
    status: 130,
    target: "codex",
    proofs: codexProofs,
  },
  {
    name: "SIGHUP stops the reviewer group of a single target",
    signal: "SIGHUP",
    status: 129,
    target: "claude",
    proofs: claudeProofs,
  },
];

// The reviewers run detached, so a signal to the launcher never reaches them. Before the launcher stopped its
// groups on interrupt, they outlived it and the timeout that was meant to bound them died with it.
describe("interrupting the launcher", { skip: !posix }, () => {
  for (const tc of cases) {
    test(tc.name, async () => {
      const f = installFakes();
      const readPID = (name: string): number => {
        try {
          return Number(readFileSync(f.path(name), "utf8").trim()) || 0;
        } catch {
          return 0;
        }
      };
      const run = startPeer({
        dir: newWorkRepo(),
        args: [
          "--target",
          tc.target,
          "--timeout",
          "120",
          "--output",
          f.path("out"),
          "--summary",
          "interrupted",
        ],
        env: {
          CLAUDECODE: "1",
          PEER_REVIEW_LAUNCHER: f.path("claude"),
          PEER_REVIEW_CODEX_LAUNCHER: f.path("codex"),
          FAKE_CLAUDE_MODE: "hang",
          FAKE_CODEX_MODE: "hang-tree",
          PEER_HEARTBEAT_INTERVAL: "0",
        },
      });
      try {
        const deadline = Date.now() + scale(30_000);
        while (tc.proofs.some((name) => readPID(name) === 0)) {
          assert.ok(Date.now() < deadline, "the reviewers never started");
          await sleep(50);
        }
        const start = Date.now();
        assert.ok(run.child.kill(tc.signal), "the signal was delivered");
        const exit = await run.finished;
        assert.equal(
          exit.signal,
          null,
          `the launcher was killed by ${exit.signal} instead of cleaning up first`,
        );
        assert.equal(
          exit.code,
          tc.status,
          `exit=${exit.code} stdout=${exit.stdout} stderr=${exit.stderr}`,
        );
        assert.ok(
          Date.now() - start < scale(20_000),
          "cleanup exceeded the grace bound",
        );
        mustContain(
          "interrupt diagnostic",
          exit.stderr,
          `${tc.signal} received`,
        );
        for (const name of tc.proofs) {
          const pid = readPID(name);
          assert.equal(
            pidAlive(pid),
            false,
            `owned ${name} pid=${pid} outlived the launcher`,
          );
        }
      } finally {
        run.child.kill("SIGKILL");
        for (const name of [...claudeProofs, ...codexProofs]) {
          const pid = readPID(name);
          if (pid > 0) {
            try {
              process.kill(pid, "SIGKILL");
            } catch {
              // already gone
            }
          }
        }
      }
    });
  }
});
