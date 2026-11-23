import assert from "node:assert/strict";
import { spawn, spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import { EventEmitter } from "node:events";
import fs, {
  existsSync,
  lstatSync,
  mkdirSync,
  mkdtempSync,
  readdirSync,
  readFileSync,
  realpathSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { syncBuiltinESMExports } from "node:module";
import os from "node:os";
import path from "node:path";
import { test, type TestContext } from "node:test";
import { pathToFileURL } from "node:url";
import { fixtureGitEnv, git } from "../conformance/testing/git-checkout.ts";
import { findCommand, type Env } from "./common.ts";
import {
  LOCK_NAME,
  STAGE_NAME,
  refreshStage,
  removeStage,
  type SignalSource,
} from "./stage.ts";

const isWindows = process.platform === "win32";

function temporary(t: TestContext): string {
  const dir = realpathSync.native(
    mkdtempSync(path.join(os.tmpdir(), "agentics-stage-signals-")),
  );
  t.after(() => rmSync(dir, { recursive: true, force: true }));
  return dir;
}

interface Checkout {
  root: string;
  env: Env;
}

/**
 * A tracked-binary checkout with the marketplace manifests, hook dispatcher and matching local artifacts.
 */
function checkout(t: TestContext): Checkout {
  const parent = temporary(t);
  const root = path.join(parent, "repo");
  const env = fixtureGitEnv();
  const files: Record<string, string> = {
    ".claude-plugin/marketplace.json": "{}\n",
    ".agents/plugins/marketplace.json": "{}\n",
    ".antigravity-plugin/plugin.json": "{}\n",
    "hooks/run-hook.cmd": "fixture\n",
    "hooks/fetch-hook.ps1": "fixture\n",
    "bin/agx.exe": "fixture launcher",
  };
  const hash = createHash("sha256").update("runtime").digest("hex");
  files["hooks/SHA256SUMS"] = [
    "darwin-amd64",
    "darwin-arm64",
    "linux-amd64",
    "linux-arm64",
    "windows-amd64.exe",
    "windows-arm64.exe",
  ]
    .map((name) => {
      files[`hooks/bin/agx-${name}`] = "runtime";
      return `${hash}  bin/agx-${name}\n`;
    })
    .join("");
  files["skills/one/SKILL.md"] = "one\n";
  files["skills/two/SKILL.md"] = "two\n";
  for (const [rel, content] of Object.entries(files)) {
    mkdirSync(path.dirname(path.join(root, rel)), { recursive: true });
    writeFileSync(path.join(root, rel), content, {
      mode: rel.startsWith("hooks/bin/") ? 0o755 : 0o644,
    });
  }
  git(root, "init", "-q");
  git(root, "add", "-A");
  git(
    root,
    "-c",
    "user.email=fixture@agentics.test",
    "-c",
    "user.name=fixture",
    "commit",
    "-q",
    "-m",
    "fixture",
  );
  return { root, env };
}

/** A fake signal source that records exits instead of ending the test runner. */
function fakeSignals(platform: NodeJS.Platform) {
  const emitter = new EventEmitter();
  const exits: number[] = [];
  const source: SignalSource = {
    emitter,
    exit: (code) => void exits.push(code),
    platform,
  };
  const registered = () =>
    (emitter.eventNames() as string[])
      .filter((name) => emitter.listenerCount(name) > 0)
      .sort();
  return { emitter, exits, source, registered };
}

/** A promise with its resolver, so a hook can pause refresh until the test lets it go or forever. */
function gate() {
  let open!: () => void;
  const opened = new Promise<void>((resolve) => (open = resolve));
  let reached!: () => void;
  const arrived = new Promise<void>((resolve) => (reached = resolve));
  return { open, opened, reached, arrived };
}

const quiet = () => {};

function refresh(
  c: Checkout,
  cache: string,
  signals: SignalSource,
  hooks: Pick<
    Parameters<typeof refreshStage>[0] & object,
    "copyHook" | "lockHook" | "interruptHook"
  > = {},
): Promise<number> {
  return refreshStage({
    repoRoot: c.root,
    cacheRoot: cache,
    env: c.env,
    out: quiet,
    signals,
    ...hooks,
  });
}

function assertAbsent(value: string): void {
  assert.equal(lstatSync(value, { throwIfNoEntry: false }), undefined, value);
}

function assertEmptyLeaf(cache: string): void {
  const leaf = path.join(cache, STAGE_NAME);
  assert.ok(lstatSync(leaf).isDirectory(), leaf);
  assert.deepEqual(readdirSync(leaf), []);
}

const copyCases: [NodeJS.Platform, NodeJS.Signals, number][] = [
  ["win32", "SIGINT", 130],
  ["win32", "SIGBREAK", 130],
  ["win32", "SIGHUP", 129],
  ["linux", "SIGINT", 130],
  ["linux", "SIGTERM", 143],
  ["linux", "SIGHUP", 129],
];

test("stage signals: an interrupt during the copy empties the leaf and releases the lock", async (t) => {
  for (const [platform, signal, code] of copyCases)
    await t.test(`${signal} on ${platform} exits ${code}`, async (t) => {
      const c = checkout(t);
      const cache = path.join(temporary(t), "cache");
      const s = fakeSignals(platform);
      const paused = gate();
      const cwd = process.cwd();
      void refresh(c, cache, s.source, {
        copyHook: () => {
          // A file that has landed in the leaf when the interrupt arrives.
          writeFileSync("partial", "half a stage");
          paused.reached();
          return paused.opened;
        },
      });
      await paused.arrived;
      assert.ok(existsSync(path.join(cache, STAGE_NAME, "partial")));
      s.emitter.emit(signal);
      assert.deepEqual(s.exits, [code]);
      assertEmptyLeaf(cache);
      assertAbsent(path.join(cache, LOCK_NAME));
      assert.equal(process.cwd(), cwd);
      assert.deepEqual(s.registered(), []);
    });
});

test("stage signals: an interrupt under the lock before the copy keeps the previous stage", async (t) => {
  const c = checkout(t);
  const cache = path.join(temporary(t), "cache");
  mkdirSync(path.join(cache, STAGE_NAME), { recursive: true });
  writeFileSync(path.join(cache, STAGE_NAME, "sentinel"), "previous stage");
  const s = fakeSignals("linux");
  const paused = gate();
  const cwd = process.cwd();
  void refresh(c, cache, s.source, {
    lockHook: () => {
      paused.reached();
      return paused.opened;
    },
  });
  await paused.arrived;
  assert.ok(lstatSync(path.join(cache, LOCK_NAME)).isDirectory());
  s.emitter.emit("SIGINT");
  assert.deepEqual(s.exits, [130]);
  assertAbsent(path.join(cache, LOCK_NAME));
  assert.equal(
    readFileSync(path.join(cache, STAGE_NAME, "sentinel"), "utf8"),
    "previous stage",
  );
  assert.equal(process.cwd(), cwd);
});

test("stage signals: a failed cleanup exits 1 and still empties the leaf", async (t) => {
  const c = checkout(t);
  const cache = path.join(temporary(t), "cache");
  const s = fakeSignals("linux");
  const paused = gate();
  void refresh(c, cache, s.source, {
    copyHook: () => {
      writeFileSync("partial", "half a stage");
      paused.reached();
      return paused.opened;
    },
  });
  await paused.arrived;
  // rmdir refuses a directory that is not empty.
  writeFileSync(path.join(cache, LOCK_NAME, "blocker"), "");
  s.emitter.emit("SIGINT");
  assert.deepEqual(s.exits, [1]);
  assertEmptyLeaf(cache);
});

test("stage signals: a repeated signal during the cleanup is ignored", async (t) => {
  const c = checkout(t);
  const cache = path.join(temporary(t), "cache");
  const s = fakeSignals("linux");
  // Node runs a signal's default action, ending the process, when no listener is registered for it.
  const killed: string[] = [];
  const deliver = (signal: NodeJS.Signals) => {
    if (s.emitter.listenerCount(signal) === 0) killed.push(signal);
    else s.emitter.emit(signal);
  };
  const paused = gate();
  let cleanups = 0;
  const cwd = process.cwd();
  void refresh(c, cache, s.source, {
    copyHook: () => {
      writeFileSync("partial", "half a stage");
      paused.reached();
      return paused.opened;
    },
    interruptHook: () => {
      cleanups += 1;
      deliver("SIGINT");
      deliver("SIGTERM");
    },
  });
  await paused.arrived;
  deliver("SIGINT");
  assert.equal(cleanups, 1);
  assert.deepEqual(killed, []);
  assert.deepEqual(s.exits, [130]);
  assertEmptyLeaf(cache);
  assertAbsent(path.join(cache, LOCK_NAME));
  assert.equal(process.cwd(), cwd);
  assert.deepEqual(s.registered(), []);
});

test("stage signals: an interrupt during a remove releases the lock", async (t) => {
  const c = checkout(t);
  const cache = path.join(temporary(t), "cache");
  const s = fakeSignals("linux");
  assert.equal(await refresh(c, cache, s.source), 0);
  const paused = gate();
  const cwd = process.cwd();
  void removeStage({
    repoRoot: c.root,
    cacheRoot: cache,
    env: c.env,
    out: quiet,
    signals: s.source,
    lockHook: () => {
      paused.reached();
      return paused.opened;
    },
  });
  await paused.arrived;
  assert.ok(lstatSync(path.join(cache, LOCK_NAME)).isDirectory());
  s.emitter.emit("SIGINT");
  assert.deepEqual(s.exits, [130]);
  assertAbsent(path.join(cache, LOCK_NAME));
  // The interrupt came before the removal started, so the stage is still whole.
  assert.ok(existsSync(path.join(cache, STAGE_NAME, "skills/one/SKILL.md")));
  assert.equal(process.cwd(), cwd);
  assert.deepEqual(s.registered(), []);
});

test("stage signals: the cleanup empties the leaf before it releases the lock", async (t) => {
  const c = checkout(t);
  const cache = path.join(temporary(t), "cache");
  const s = fakeSignals("linux");
  const paused = gate();
  void refresh(c, cache, s.source, {
    copyHook: () => {
      writeFileSync("partial", "half a stage");
      paused.reached();
      return paused.opened;
    },
  });
  await paused.arrived;
  const calls = recordRemovals(t);
  s.emitter.emit("SIGINT");
  restoreRemovals(t);
  assert.deepEqual(s.exits, [130]);
  assert.deepEqual(calls, ["rm partial", `rmdir ${LOCK_NAME}`]);
});

/**
 * Records the stage's synchronous removals in order. The stage imports them by name, so the mocks are copied into
 * the module bindings; `restoreRemovals` undoes both.
 */
function recordRemovals(t: TestContext): string[] {
  const calls: string[] = [];
  const rm = fs.rmSync,
    rmdir = fs.rmdirSync;
  t.mock.method(fs, "rmSync", (...args: Parameters<typeof fs.rmSync>) => {
    calls.push(`rm ${String(args[0])}`);
    return rm(...args);
  });
  t.mock.method(fs, "rmdirSync", (...args: Parameters<typeof fs.rmdirSync>) => {
    calls.push(`rmdir ${String(args[0])}`);
    return rmdir(...args);
  });
  syncBuiltinESMExports();
  return calls;
}

function restoreRemovals(t: TestContext): void {
  t.mock.restoreAll();
  syncBuiltinESMExports();
}

test("stage signals: the handlers are removed on every release path", async (t) => {
  for (const platform of ["win32", "linux"] as const)
    await t.test(platform, async (t) => {
      const c = checkout(t);
      const cache = path.join(temporary(t), "cache");
      const check = (label: string, status: number, want: number) => {
        assert.equal(status, want, label);
        for (const signal of [...signalNames, ...s.registered()])
          assert.equal(s.emitter.listenerCount(signal), 0, label);
        assert.deepEqual(s.exits, [], label);
      };
      const signalNames = ["SIGINT", "SIGBREAK", "SIGTERM", "SIGHUP"];
      const s = fakeSignals(platform);

      check("successful refresh", await refresh(c, cache, s.source), 0);

      // A tracked runtime changed without refreshing the manifest fails verification.
      writeFileSync(path.join(c.root, "hooks/bin/agx-linux-amd64"), "x");
      check("failed refresh", await refresh(c, cache, s.source), 1);
      rmSync(path.join(c.root, "hooks/bin"), { recursive: true });

      mkdirSync(path.join(cache, LOCK_NAME));
      check("refusal under a held lock", await refresh(c, cache, s.source), 1);
      rmSync(path.join(cache, LOCK_NAME), { recursive: true });

      const remove = (root: string) =>
        removeStage({
          repoRoot: c.root,
          cacheRoot: root,
          env: c.env,
          out: quiet,
          signals: s.source,
        });
      check("remove", await remove(cache), 0);
      assertAbsent(path.join(cache, STAGE_NAME));
      check("remove without a stage", await remove(cache), 0);
      check(
        "remove without a cache",
        await remove(path.join(cache, "missing")),
        0,
      );
    });
});

test("stage signals: the registered set follows the platform", async (t) => {
  for (const [platform, want] of [
    ["win32", ["SIGBREAK", "SIGHUP", "SIGINT"]],
    ["linux", ["SIGHUP", "SIGINT", "SIGTERM"]],
  ] as const)
    await t.test(platform, async (t) => {
      const c = checkout(t);
      const cache = path.join(temporary(t), "cache");
      const s = fakeSignals(platform);
      const paused = gate();
      const done = refresh(c, cache, s.source, {
        copyHook: () => {
          paused.reached();
          return paused.opened;
        },
      });
      await paused.arrived;
      assert.deepEqual(s.registered(), want);
      paused.open();
      assert.equal(await done, 0);
      assert.deepEqual(s.registered(), []);
    });
});

/** Polls until `ready` holds, failing when it does not within the limit or the child exits first. */
async function waitFor(
  what: string,
  ready: () => boolean,
  exited: () => string | undefined,
  limit = 30_000,
): Promise<void> {
  const start = Date.now();
  while (!ready()) {
    const early = exited();
    if (early !== undefined) assert.fail(`exited before ${what}: ${early}`);
    if (Date.now() - start > limit)
      assert.fail(`timed out waiting for ${what}`);
    await new Promise((resolve) => setTimeout(resolve, 20));
  }
}

interface Driver {
  wait: Promise<{ code: number | null; signal: NodeJS.Signals | null }>;
  kill: (signal: NodeJS.Signals) => void;
  exited: () => string | undefined;
}

/**
 * Starts refresh in its own Node process with the real process as the signal source. With `copyReady`, the copy hook
 * writes that marker and then waits a minute, so the stage is paused with the lock held and the leaf created.
 */
function startDriver(
  dir: string,
  c: Checkout,
  cache: string,
  env: Env,
  copyReady?: string,
): Driver {
  const stageURL = pathToFileURL(
    path.join(import.meta.dirname, "stage.ts"),
  ).href;
  const config = JSON.stringify({
    repoRoot: c.root,
    cacheRoot: cache,
    copyReady,
  });
  const code = `
import { writeFileSync } from "node:fs";
const { refreshStage } = await import(${JSON.stringify(stageURL)});
const config = ${config};
const copyHook = config.copyReady === undefined ? undefined : () => {
  writeFileSync(config.copyReady, "");
  return new Promise((resolve) => setTimeout(resolve, 60_000));
};
process.exit(await refreshStage({ repoRoot: config.repoRoot, cacheRoot: config.cacheRoot, env: process.env, copyHook }));
`;
  const child = spawn(process.execPath, ["--input-type=module", "-e", code], {
    cwd: dir,
    env,
    stdio: ["ignore", "pipe", "pipe"],
  });
  let output = "";
  child.stdout.on("data", (chunk) => (output += chunk));
  child.stderr.on("data", (chunk) => (output += chunk));
  let done: string | undefined;
  const wait = new Promise<{
    code: number | null;
    signal: NodeJS.Signals | null;
  }>((resolve) =>
    child.on("exit", (code, signal) => {
      done = `status ${code} signal ${signal}: ${output}`;
      resolve({ code, signal });
    }),
  );
  return {
    wait,
    kill: (signal) => void child.kill(signal),
    exited: () => done,
  };
}

/**
 * A fake git, first on PATH, that blocks on `ls-files` after writing its pid to `ready`, writes `exited` when it ends,
 * and gives up after a minute so it never outlives the test. Every other call goes to the real git.
 */
function writeFakeGit(dir: string, ready: string, exited: string): string {
  const bin = path.join(dir, "fake-bin");
  mkdirSync(bin);
  const commonURL = pathToFileURL(
    path.join(import.meta.dirname, "common.ts"),
  ).href;
  const script = `#!/usr/bin/env node
const { spawnSync } = require("node:child_process");
const { writeFileSync } = require("node:fs");
const args = process.argv.slice(2);
const originalPath = ${JSON.stringify(process.env.PATH ?? "")};
if (args.includes("ls-files")) {
  process.on("exit", () => writeFileSync(${JSON.stringify(exited)}, ""));
  process.on("SIGTERM", () => process.exit(143));
  writeFileSync(${JSON.stringify(ready)}, String(process.pid));
  setTimeout(() => process.exit(1), 60_000);
} else {
  import(${JSON.stringify(commonURL)}).then(({ findCommand }) => {
    const git = findCommand("git", { ...process.env, PATH: originalPath });
    const result = spawnSync(git, args, { stdio: "inherit" });
    process.exit(result.status ?? 1);
  });
}
`;
  writeFileSync(path.join(bin, "git"), script, { mode: 0o755 });
  return bin;
}

const spawnedSignals: [NodeJS.Signals, number][] = [
  ["SIGINT", 130],
  ["SIGTERM", 143],
  ["SIGHUP", 129],
];

test(
  "interrupting the offline developer build preserves the old stage and removes private output",
  { skip: isWindows ? "POSIX process group and signal" : false },
  async (t) => {
    const dir = temporary(t);
    const c = checkout(t);
    const untrack = spawnSync(
      "git",
      ["-C", c.root, "rm", "-r", "--cached", "hooks/bin", "bin/agx.exe"],
      { env: c.env, encoding: "utf8" },
    );
    assert.equal(untrack.status, 0, untrack.stderr);
    mkdirSync(path.join(c.root, "hooks/go"));
    writeFileSync(
      path.join(c.root, "hooks/go/go.mod"),
      "module fixture\n\ngo 1.27.4\n",
    );
    const cache = path.join(dir, "cache");
    const leaf = path.join(cache, STAGE_NAME);
    mkdirSync(leaf, { recursive: true });
    writeFileSync(path.join(leaf, "sentinel"), "old stage");
    const ready = path.join(dir, "builder-env");
    const shim = path.join(dir, "bin");
    mkdirSync(shim);
    writeFileSync(
      path.join(shim, "go"),
      '#!/bin/sh\nprintf "%s %s %s %s" "$GOTOOLCHAIN" "$GOPROXY" "$GOSUMDB" "$GONOPROXY" > "$READY"\nexec sleep 60\n',
      { mode: 0o755 },
    );
    const driver = startDriver(dir, c, cache, {
      ...c.env,
      READY: ready,
      GOTOOLCHAIN: "auto",
      GOPROXY: "direct",
      PATH: shim + path.delimiter + c.env.PATH,
    });
    await waitFor("developer build", () => existsSync(ready), driver.exited);
    assert.equal(readFileSync(ready, "utf8"), "local off off none");
    driver.kill("SIGTERM");
    assert.deepEqual(await driver.wait, { code: 143, signal: null });
    assert.deepEqual(readdirSync(cache), [STAGE_NAME]);
    assert.equal(
      readFileSync(path.join(leaf, "sentinel"), "utf8"),
      "old stage",
    );
  },
);

test(
  "stage signals: spawned refresh exits with the signal status",
  { skip: isWindows ? "POSIX only: INT, TERM and HUP statuses" : false },
  async (t) => {
    for (const [signal, code] of spawnedSignals)
      await t.test(signal, async (t) => {
        await t.test("during the copy", async (t) => {
          const dir = temporary(t);
          const c = checkout(t);
          const cache = path.join(dir, "cache");
          const ready = path.join(dir, "copy-ready");
          const driver = startDriver(dir, c, cache, c.env, ready);
          await waitFor(
            "the paused copy",
            () => existsSync(ready) && existsSync(path.join(cache, LOCK_NAME)),
            driver.exited,
          );
          driver.kill(signal);
          assert.deepEqual(await driver.wait, { code, signal: null });
          assertAbsent(path.join(cache, LOCK_NAME));
          assertEmptyLeaf(cache);
          assert.equal(
            await refreshStage({
              repoRoot: c.root,
              cacheRoot: cache,
              env: c.env,
              out: quiet,
            }),
            0,
          );
        });

        await t.test("while git lists the files", async (t) => {
          const dir = temporary(t);
          const c = checkout(t);
          const cache = path.join(dir, "cache");
          const ready = path.join(dir, "git-ready");
          const exited = path.join(dir, "git-exited");
          const bin = writeFakeGit(dir, ready, exited);
          const env = {
            ...c.env,
            PATH: `${bin}${path.delimiter}${c.env.PATH ?? ""}`,
          };
          assert.equal(findCommand("git", env), path.join(bin, "git"));
          const driver = startDriver(dir, c, cache, env);
          await waitFor(
            "the blocked git",
            () => existsSync(ready),
            driver.exited,
          );
          driver.kill(signal);
          assert.deepEqual(await driver.wait, { code, signal: null });
          await waitFor(
            "the fake git to exit",
            () => existsSync(exited),
            () => undefined,
          );
          assertAbsent(cache);
          assert.equal(
            await refreshStage({
              repoRoot: c.root,
              cacheRoot: cache,
              env: c.env,
              out: quiet,
            }),
            0,
          );
        });
      });
  },
);
