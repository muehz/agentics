import assert from "node:assert/strict";
import { chmodSync, readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { describe, test } from "node:test";
import { groupState, groupStateFromRows, runWithTimeout } from "./process.ts";
import { mustContain } from "./testing/assert.ts";
import { posix, scale, scratchDir } from "./testing/harness.ts";
import { pidAlive } from "./testing/ps.ts";

test("a group is live while any member is not a zombie", () => {
  assert.equal(groupStateFromRows("  123 Ss+\n  999 S\n", 123), "live");
  assert.equal(groupStateFromRows("  123 Z+\n  999 S\n", 123), "gone");
  assert.equal(groupStateFromRows("  123 Z\n  123 Sl\n", 123), "live");
  assert.equal(groupStateFromRows("  124 S\n", 123), "gone");
  assert.equal(groupStateFromRows("", 123), "gone");
});

// A ps that never answers must read as an unobservable group, not stall the watchdog that polls it.
test(
  "a ps that hangs reads as unknown within a bounded time",
  { skip: !posix },
  async () => {
    const dir = scratchDir("agentics-peer-ps-");
    const fake = join(dir, "ps");
    writeFileSync(fake, "#!/bin/sh\nexec sleep 20\n");
    chmodSync(fake, 0o755);
    const savedPath = process.env.PATH;
    process.env.PATH = `${dir}:${savedPath ?? ""}`;
    try {
      const start = Date.now();
      assert.equal(await groupState(1, 300), "unknown");
      assert.ok(
        Date.now() - start < scale(10_000),
        "the observation must be bounded",
      );
    } finally {
      process.env.PATH = savedPath;
    }
  },
);

describe("runWithTimeout", { skip: !posix }, () => {
  const env = { PATH: process.env.PATH ?? "" };
  const run = (
    script: string,
    seconds: number,
    extra: { stdin?: string } = {},
  ) => {
    const out: Buffer[] = [];
    const lines: string[] = [];
    const promise = runWithTimeout({
      argv: ["bash", "-c", script],
      env,
      seconds,
      stdin: extra.stdin,
      stderr: "ignore",
      onStdout: (chunk) => void out.push(chunk),
      report: (line) => void lines.push(line),
    });
    return { promise, stdout: () => Buffer.concat(out).toString(), lines };
  };

  test("delivers stdin, streams stdout, and returns the exit status", async () => {
    const r = run('read line; echo "got:$line"; exit 3', 10, {
      stdin: "hello\n",
    });
    assert.equal(await r.promise, 3);
    assert.equal(r.stdout(), "got:hello\n");
  });

  test("kills the whole group of a command that outlives the timeout", async () => {
    const dir = scratchDir("agentics-peer-proc-");
    const start = Date.now();
    const r = run(`sleep 30 & echo $! > ${dir}/child.pid; wait`, 1);
    assert.equal(await r.promise, 124);
    assert.ok(
      Date.now() - start < scale(15_000),
      "the watchdog must kill the run promptly",
    );
    assert.equal(
      pidAlive(Number(readFileSync(join(dir, "child.pid"), "utf8").trim())),
      false,
    );
  });

  test("leaves no descendant behind a leader that exits successfully", async () => {
    const dir = scratchDir("agentics-peer-proc-");
    const r = run(
      `bash -c 'trap "" TERM; echo $$ > ${dir}/d.pid; sleep 30' & while [ ! -s ${dir}/d.pid ]; do sleep 0.01; done; exit 0`,
      10,
    );
    assert.equal(await r.promise, 0);
    assert.equal(
      pidAlive(Number(readFileSync(join(dir, "d.pid"), "utf8").trim())),
      false,
    );
  });

  test("reports a command that cannot start", async () => {
    const lines: string[] = [];
    const status = await runWithTimeout({
      argv: ["/no/such/launcher"],
      env,
      seconds: 5,
      stderr: "ignore",
      report: (line) => void lines.push(line),
    });
    assert.equal(status, 127);
    mustContain(
      "spawn failure",
      lines.join("\n"),
      "ERROR: cannot run /no/such/launcher",
    );
  });
});
