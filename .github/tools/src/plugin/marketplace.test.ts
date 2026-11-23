import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import {
  lstatSync,
  mkdirSync,
  mkdtempSync,
  realpathSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import os from "node:os";
import path from "node:path";
import test, { type TestContext } from "node:test";

import { fixtureGitEnv, git } from "../conformance/testing/git-checkout.ts";
import type { Env } from "./common.ts";
import {
  defaultCliRunner,
  setupLocal,
  showStatus,
  teardownLocal,
  type CliRunner,
} from "./marketplace.ts";
import { formatSize, LOCK_NAME, STAGE_NAME } from "./stage.ts";

function temporary(t: TestContext): string {
  const dir = realpathSync.native(
    mkdtempSync(path.join(os.tmpdir(), "agentics-marketplace-")),
  );
  t.after(() => rmSync(dir, { recursive: true, force: true }));
  return dir;
}

/**
 * Writes a file for each plugin CLI into `dir` so detection finds all four. The runner is always injected or the
 * spawn is refused first, so none of them is ever started.
 */
function writeDetectableClis(dir: string): void {
  mkdirSync(dir, { recursive: true });
  for (const cli of ["claude", "copilot", "codex", "agy"])
    writeFileSync(
      path.join(dir, process.platform === "win32" ? `${cli}.cmd` : cli),
      "never run\n",
      { mode: 0o755 },
    );
}

interface Fixture {
  repo: string;
  cache: string;
  env: Env;
  stage: string;
}

/**
 * A tracked-binary checkout, a cache root, and a PATH that finds the fake CLIs before the host's Git.
 */
function fixture(t: TestContext, options: { git?: boolean } = {}): Fixture {
  const base = temporary(t);
  const repo = path.join(base, "repo"),
    cache = path.join(base, "cache"),
    bin = path.join(base, "bin");
  writeDetectableClis(bin);
  const env = fixtureGitEnv();
  for (const key of Object.keys(env)) if (/^path$/i.test(key)) delete env[key];
  env.PATH = [bin, process.env.PATH ?? process.env.Path ?? ""].join(
    path.delimiter,
  );
  env.XDG_CACHE_HOME = cache;
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

  for (const [rel, content] of Object.entries(files)) {
    mkdirSync(path.dirname(path.join(repo, rel)), { recursive: true });
    writeFileSync(path.join(repo, rel), content, {
      mode: rel.startsWith("hooks/bin/") ? 0o755 : 0o644,
    });
  }
  if (options.git !== false) {
    git(repo, "init", "-q");
    git(repo, "add", "-A");
  }
  mkdirSync(cache);
  return { repo, cache, env, stage: path.join(cache, STAGE_NAME) };
}

/** Collects output by stream. */
function sink() {
  const parts: { stream: "stdout" | "stderr"; text: string }[] = [];
  return {
    write: (stream: "stdout" | "stderr", text: string) =>
      void parts.push({ stream, text }),
    text: (stream: "stdout" | "stderr") =>
      parts
        .filter((part) => part.stream === stream)
        .map((part) => part.text)
        .join(""),
  };
}

/** A runner that records every call and fails the ones `fails` matches. */
function recorder(fails: (call: string) => boolean = () => false) {
  const calls: string[] = [];
  const runner: CliRunner = (cli, args) => {
    const call = [cli, ...args].join(" ");
    calls.push(call);
    return !fails(call);
  };
  return { calls, runner };
}

async function setup(f: Fixture, runner: CliRunner) {
  const s = sink();
  const code = await setupLocal({
    repoRoot: f.repo,
    env: f.env,
    runner,
    validate: () => 0,
    write: s.write,
  });
  return { code, stdout: s.text("stdout"), stderr: s.text("stderr") };
}

test("setup with succeeding CLIs never falls back to update and reports no failed install", async (t) => {
  const f = fixture(t);
  const { calls, runner } = recorder();
  const r = await setup(f, runner);
  assert.equal(r.code, 0, r.stdout + r.stderr);
  assert.deepEqual(calls, [
    "claude plugin marketplace remove muehz",
    `claude plugin marketplace add ${f.stage}`,
    "claude plugin uninstall agentics@muehz --keep-data --scope user",
    "claude plugin install agentics@muehz --scope user",
    "copilot plugin marketplace remove muehz --force",
    `copilot plugin marketplace add ${f.stage}`,
    "copilot plugin install agentics@muehz",
    "codex plugin marketplace remove muehz",
    `codex plugin marketplace add ${f.stage}`,
    "codex plugin add agentics@muehz",
    "agy plugin uninstall agentics",
    `agy plugin install ${f.stage}`,
  ]);
  assert.ok(!r.stdout.includes("⚠ Failed to install"), r.stdout);
  assert.ok(lstatSync(path.join(f.stage, "skills/one/SKILL.md")).isFile());
});

test("setup reports agy install failure without breaking the setup", async (t) => {
  const f = fixture(t);
  const { calls, runner } = recorder(
    (call) => call === `agy plugin install ${f.stage}`,
  );
  const r = await setup(f, runner);
  assert.equal(r.code, 0, r.stdout + r.stderr);
  assert.ok(calls.includes("agy plugin uninstall agentics"));
  assert.ok(calls.includes(`agy plugin install ${f.stage}`));
  assert.equal(r.stdout.split("⚠ Failed to install").length - 1, 1, r.stdout);
  assert.ok(r.stdout.includes("    ⚠ Failed to install agentics\n"), r.stdout);
});

test("setup rejects a partial tracked roster even when every binary is on disk", async (t) => {
  const f = fixture(t);
  const remove = spawnSync(
    "git",
    ["-C", f.repo, "rm", "--cached", "bin/agx.exe"],
    { env: f.env, encoding: "utf8" },
  );
  assert.equal(remove.status, 0, remove.stderr);
  const { calls, runner } = recorder();
  const result = await setup(f, runner);
  assert.equal(result.code, 1);
  assert.match(
    result.stderr,
    /incomplete tracked marketplace native artifact roster/,
  );
  assert.deepEqual(calls, []);
  assert.equal(lstatSync(f.stage, { throwIfNoEntry: false }), undefined);
});

test("setup falls back to update only when claude's marketplace add fails", async (t) => {
  const f = fixture(t);
  const { calls, runner } = recorder(
    (call) =>
      call === `claude plugin marketplace add ${f.stage}` ||
      call === "copilot plugin install agentics@muehz",
  );
  const r = await setup(f, runner);
  assert.equal(r.code, 0, r.stdout + r.stderr);
  const add = calls.indexOf(`claude plugin marketplace add ${f.stage}`);
  assert.equal(calls[add + 1], "claude plugin marketplace update muehz");
  assert.equal(
    calls.filter((call) => call.includes("marketplace update")).length,
    1,
  );
  // Only copilot's install failed, so only its warning prints.
  assert.equal(r.stdout.split("⚠ Failed to install").length - 1, 1, r.stdout);
  assert.ok(
    r.stdout.includes(
      "    ⚠ Failed to install agentics (may already be installed)\n",
    ),
    r.stdout,
  );
});

test("teardown returns the stage removal's status", async (t) => {
  for (const locked of [false, true]) {
    const f = fixture(t, { git: false });
    mkdirSync(f.stage);
    writeFileSync(path.join(f.stage, "file"), "staged\n");
    if (locked) mkdirSync(path.join(f.cache, LOCK_NAME));
    const { calls, runner } = recorder(() => true);
    const s = sink();
    const code = await teardownLocal({
      repoRoot: f.repo,
      env: f.env,
      runner,
      write: s.write,
    });
    // Every CLI call failed, which teardown ignores; only the removal decides.
    assert.equal(calls.length, 14);
    assert.deepEqual(calls.slice(-2), [
      "agy plugin uninstall agentics",
      "agy plugin install https://github.com/muehz/agentics",
    ]);
    if (locked) {
      assert.notEqual(code, 0, s.text("stdout"));
      assert.ok(
        s.text("stderr").includes("marketplace stage is locked"),
        s.text("stderr"),
      );
      assert.ok(lstatSync(f.stage).isDirectory());
    } else {
      assert.equal(code, 0, s.text("stderr"));
      assert.equal(lstatSync(f.stage, { throwIfNoEntry: false }), undefined);
    }
  }
});

test("a command cmd.exe cannot carry fails with a message naming the CLI", (t) => {
  const dir = temporary(t);
  writeFileSync(path.join(dir, "claude.cmd"), "never run\n");
  const s = sink();
  const run = defaultCliRunner(
    { PATH: dir, PATHEXT: ".CMD" },
    "win32",
    s.write,
  );
  assert.equal(run("claude", ["plugin", "install", "100%"]), false);
  assert.match(
    s.text("stderr"),
    /^❌ claude: cannot pass "100%" through cmd\.exe: .*\n$/,
  );
  assert.equal(s.text("stdout"), "");
});

test("status prints the stage path and the formatted sum of its file sizes", (t) => {
  const f = fixture(t, { git: false });
  mkdirSync(path.join(f.stage, "nested/deeper"), { recursive: true });
  writeFileSync(path.join(f.stage, "a"), Buffer.alloc(3000));
  writeFileSync(path.join(f.stage, "nested/b"), Buffer.alloc(1500));
  writeFileSync(path.join(f.stage, "nested/deeper/c"), Buffer.alloc(500));
  const { calls, runner } = recorder(
    (call) => call === "codex plugin marketplace list",
  );
  const s = sink();
  assert.equal(
    showStatus({ repoRoot: f.repo, env: f.env, runner, write: s.write }),
    0,
  );
  const stdout = s.text("stdout");
  const size = formatSize(5000);
  assert.equal(size, "4.9K");
  assert.ok(
    stdout.split("\n").includes(`Marketplace stage: ${f.stage} (${size})`),
    stdout,
  );
  assert.equal(calls.length, 7);
  assert.ok(stdout.split("\n").includes("--- Antigravity ---"), stdout);
  assert.ok(!calls.includes("agy plugin marketplace list"), calls.join("\n"));
  assert.equal(
    stdout
      .split("\n")
      .filter((line) => line === "  (unable to list marketplaces)").length,
    1,
    stdout,
  );
  assert.ok(!stdout.includes("(unable to list plugins)"), stdout);
});
