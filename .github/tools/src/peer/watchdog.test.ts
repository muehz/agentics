import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import {
  chmodSync,
  existsSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { describe, test } from "node:test";
import { findOnPath } from "./launcher.ts";
import { mustContain } from "./testing/assert.ts";
import { installFakes } from "./testing/fakes.ts";
import {
  newShimPath,
  newWorkRepo,
  posix,
  runPeer,
  scale,
} from "./testing/harness.ts";
import { pidAlive } from "./testing/ps.ts";

interface Case {
  name: string;
  mode: string;
  failPS: boolean;
  wantExit: number;
}

const cases: Case[] = [
  {
    name: "leader exits on TERM",
    mode: "hang-tree",
    failPS: false,
    wantExit: 124,
  },
  {
    name: "leader ignores TERM",
    mode: "hang-tree-ignore-leader",
    failPS: false,
    wantExit: 124,
  },
  {
    name: "visibility fails during dispatch",
    mode: "hang-tree-ignore-leader",
    failPS: true,
    wantExit: 1,
  },
  {
    name: "visibility fails after leader exit",
    mode: "exit-tree",
    failPS: true,
    wantExit: 1,
  },
  {
    name: "successful dispatch under the shim PATH",
    mode: "good",
    failPS: false,
    wantExit: 0,
  },
];

// The shim PATH carries no timeout or gtimeout, so a hidden dependency on coreutils would also fail here.
describe("the watchdog through the CLI", { skip: !posix }, () => {
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
      const shim = newShimPath();
      if (tc.failPS) {
        // Remove the symlink before writing: never overwrite its system target.
        rmSync(`${shim}/ps`);
        const realPS = findOnPath("ps", process.env.PATH);
        assert.ok(realPS, "ps must exist on the test host");
        writeFileSync(
          `${shim}/ps`,
          `#!/bin/bash\nif [ "\${1:-}" = -axo ] && [ -f '${f.path("sleeper.pid")}' ]; then\n  printf 'injected ps visibility failure\\n' >&2\n  exit 71\nfi\nexec '${realPS}' "$@"\n`,
        );
        chmodSync(`${shim}/ps`, 0o755);
      }
      const unrelated = spawn("sleep", ["30"], { stdio: "ignore" });
      try {
        const start = Date.now();
        const res = await runPeer({
          dir: newWorkRepo(),
          path: shim,
          pipe: true,
          args: [
            "--target",
            "codex",
            "--timeout",
            "1",
            "--output",
            f.path("tree.md"),
            "--summary",
            "owned descendants",
          ],
          env: {
            CLAUDECODE: "1",
            PEER_REVIEW_CODEX_LAUNCHER: f.path("codex"),
            FAKE_CODEX_MODE: tc.mode,
            PEER_HEARTBEAT_INTERVAL: "0",
          },
        });
        // The pipes reached EOF, which means no owned process kept a write end open.
        assert.equal(
          res.code,
          tc.wantExit,
          `exit=${res.code} stdout=${res.stdout} stderr=${res.stderr}`,
        );
        assert.ok(
          Date.now() - start < scale(15_000),
          "cleanup/EOF exceeded the fixture grace bound",
        );
        if (tc.mode !== "good") {
          const leader = readPID("leader.pid");
          const group = readPID("leader.pgid");
          assert.ok(
            leader > 0 && group === leader,
            `command not isolated: pid=${leader} pgid=${group}`,
          );
          for (const name of ["leader.pid", "descendant.pid", "sleeper.pid"]) {
            const pid = readPID(name);
            assert.ok(pid > 0, `missing readiness proof ${name}`);
            assert.equal(
              pidAlive(pid),
              false,
              `owned ${name} pid=${pid} remains live`,
            );
          }
        }
        // pid 0 would signal the test's own process group, so an unset pid must fail rather than pass vacuously.
        assert.ok(unrelated.pid !== undefined, "the unrelated process started");
        assert.doesNotThrow(
          () => process.kill(unrelated.pid as number, 0),
          "the unrelated process was killed",
        );
        if (tc.failPS) {
          mustContain(
            "visibility diagnostic",
            res.stderr,
            "could not observe owned process group",
          );
        } else if (tc.wantExit === 124) {
          mustContain("timeout diagnostic", res.stderr, "timed out after 1s");
        } else {
          assert.ok(existsSync(f.path("tree.md")));
          mustContain(
            "successful dispatch",
            readFileSync(f.path("tree.md"), "utf8"),
            "REVIEW-COMPLETE: no-findings",
          );
        }
      } finally {
        unrelated.kill("SIGKILL");
        for (const name of ["sleeper.pid", "descendant.pid", "leader.pid"]) {
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
