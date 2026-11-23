import test, { type TestContext } from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { spawnSync } from "node:child_process";
import { fixtureGitEnv, git } from "../conformance/testing/git-checkout.ts";

interface CommandCall {
  command: string;
  args: string[];
  cwd: string;
}

const source = path.resolve(import.meta.dirname, "../../../..");

// The exact Husky launcher text, without its optional trailing newline.
const launcher = '#!/usr/bin/env sh\n. "$(dirname "$0")/h"';

// Shaped like the wrapper Entire CLI installs over a launcher it moves aside.
const entireWrapper = `#!/bin/sh
# Entire CLI hooks
entire hooks git post-commit "$@" 2>/dev/null || true
if [ -x "$(dirname "$0")/post-commit.pre-entire" ]; then
  "$(dirname "$0")/post-commit.pre-entire" "$@"
fi
`;

function hooksEnv() {
  const env = fixtureGitEnv();
  // These scenarios read and run the hook path recorded in the checkout.
  const key = Object.keys(env).find(
    (name) =>
      /^GIT_CONFIG_KEY_\d+$/.test(name) && env[name] === "core.hooksPath",
  );
  if (key === undefined) return env;
  const last = Number(env.GIT_CONFIG_COUNT) - 1;
  const lastKey = `GIT_CONFIG_KEY_${last}`;
  const lastValue = `GIT_CONFIG_VALUE_${last}`;
  env[key] = env[lastKey]!;
  env[key.replace("_KEY_", "_VALUE_")] = env[lastValue]!;
  delete env[lastKey];
  delete env[lastValue];
  env.GIT_CONFIG_COUNT = String(last);
  return env;
}

// Runs setup against the real npm. npm exports npm_config_prefix and
// npm_config_global_prefix to the scripts it launches, as
// `npm --prefix .github/tools test` does under --validate. Node's Windows
// npm.cmd shim reads that prefix to locate the global npm, so a child that
// inherits them runs Node's bundled npm rather than the one on the user's PATH.
function runSetup(root: string, probe: string, husky = "") {
  const env: NodeJS.ProcessEnv = {
    ...hooksEnv(),
    HUSKY: husky,
    NODE_OPTIONS: `--require=${JSON.stringify(probe)}`,
  };
  for (const key of Object.keys(env))
    if (/^npm_config_(?:global_)?prefix$/i.test(key)) delete env[key];
  return spawnSync(
    process.execPath,
    [".github/tools/src/scripts/setup-worktree.ts"],
    { cwd: root, encoding: "utf8", env },
  );
}

// Reads through one descriptor, as the installer does. Only the errors that
// mark a symbolic link or a directory mean "not a launcher"; any other error
// fails the test instead of passing a negative assertion.
function isPlainLauncher(file: string): boolean {
  let fd: number;
  try {
    fd = fs.openSync(
      file,
      fs.constants.O_RDONLY |
        (fs.constants.O_NOFOLLOW ?? 0) |
        (fs.constants.O_NONBLOCK ?? 0),
    );
  } catch (error) {
    const code = (error as NodeJS.ErrnoException).code;
    if (code === "ELOOP" || code === "EISDIR") return false;
    throw error;
  }
  try {
    const opened = fs.fstatSync(fd);
    if (!opened.isFile()) return false;
    const text = fs.readFileSync(fd, "utf8");
    const named = fs.lstatSync(file);
    if (
      named.isSymbolicLink() ||
      named.dev !== opened.dev ||
      named.ino !== opened.ino
    )
      return false;
    return text === launcher || text === `${launcher}\n`;
  } finally {
    fs.closeSync(fd);
  }
}

test("hook installer handles HUSKY=0 without installed dependencies", (t) => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "agentics-hook-skip-"));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  for (const name of [
    ".github/tools/package.json",
    ".github/tools/src/scripts/install-hooks.ts",
  ]) {
    const target = path.join(root, name);
    fs.mkdirSync(path.dirname(target), { recursive: true });
    fs.copyFileSync(path.join(source, name), target);
  }
  assert.equal(fs.existsSync(path.join(root, "node_modules")), false);
  for (const required of [false, true]) {
    const result = spawnSync(
      process.execPath,
      [
        ".github/tools/src/scripts/install-hooks.ts",
        ...(required ? ["--required"] : []),
      ],
      {
        cwd: root,
        encoding: "utf8",
        timeout: 10_000,
        env: { ...hooksEnv(), HUSKY: "0" },
      },
    );
    assert.ifError(result.error);
    assert.equal(
      result.status,
      required ? 1 : 0,
      result.stdout + result.stderr,
    );
    if (required) assert.match(result.stderr, /HUSKY=0 disables Git hooks/);
    else assert.match(result.stdout, /HUSKY=0 skip install/);
  }
});

test("installer leaves a foreign pre-commit in place and warns", (t) => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "agentics-hook-foreign-"));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  for (const name of [
    ".github/tools/package.json",
    ".github/tools/src/scripts/install-hooks.ts",
  ]) {
    const target = path.join(root, name);
    fs.mkdirSync(path.dirname(target), { recursive: true });
    fs.copyFileSync(path.join(source, name), target);
  }
  git(root, "init", "--quiet");
  git(root, "config", "--local", "core.hooksPath", ".github/husky/_");
  // The installer copies Husky's runtime from wherever `husky` resolves.
  const husky = path.join(root, "node_modules/husky");
  fs.mkdirSync(husky, { recursive: true });
  fs.writeFileSync(
    path.join(husky, "package.json"),
    JSON.stringify({ name: "husky", main: "index.js" }),
  );
  fs.writeFileSync(path.join(husky, "index.js"), "");
  fs.writeFileSync(path.join(husky, "husky"), "#!/usr/bin/env sh\n");
  const generated = path.join(root, ".github/husky/_");
  fs.mkdirSync(generated, { recursive: true });
  const foreign = "#!/bin/sh\n# other tool\nexit 0\n";
  const seeded: Record<string, string> = {
    "pre-commit": foreign,
    "post-checkout": launcher,
    "post-commit": entireWrapper,
  };
  for (const [name, text] of Object.entries(seeded))
    fs.writeFileSync(path.join(generated, name), text, { mode: 0o755 });
  // A named pipe with no writer blocks a plain read-only open, so the scan
  // would hang setup before the timeout below ends it. Windows filesystems
  // hold no named pipes.
  const pipe = path.join(generated, "other-tool.pipe");
  if (process.platform !== "win32") {
    const made = spawnSync("mkfifo", [pipe], { encoding: "utf8" });
    assert.ifError(made.error);
    assert.equal(made.status, 0, made.stderr);
  }

  for (const required of [true, false]) {
    const result = spawnSync(
      process.execPath,
      [
        ".github/tools/src/scripts/install-hooks.ts",
        ...(required ? ["--required"] : []),
      ],
      {
        cwd: root,
        encoding: "utf8",
        timeout: 10_000,
        env: { ...hooksEnv(), HUSKY: "" },
      },
    );
    assert.ifError(result.error);
    assert.equal(result.status, 0, result.stdout + result.stderr);
    const warnings = result.stderr
      .split("\n")
      .filter((line) => line.includes("lint-staged"));
    assert.equal(warnings.length, 1, result.stderr);
    assert.match(warnings.join("\n"), /\.github\/husky\/_\/pre-commit/);
    assert.doesNotMatch(result.stdout, /lint-staged/);
    assert.equal(
      fs.readFileSync(path.join(generated, "pre-commit"), "utf8"),
      foreign,
    );
    assert.equal(
      fs.readFileSync(path.join(generated, "post-commit"), "utf8"),
      entireWrapper,
    );
    assert.equal(fs.existsSync(path.join(generated, "post-checkout")), false);
    if (process.platform !== "win32")
      assert.equal(fs.lstatSync(pipe).isFIFO(), true);
  }
});

test("setup downloads modules from relocated working directories", (t) => {
  const root = fs.mkdtempSync(
    path.join(fs.realpathSync(os.tmpdir()), "agentics-setup-paths-"),
  );
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  for (const name of [
    "package.json",
    ".github/tools/package.json",
    ".github/tools/src/scripts/setup-worktree.ts",
    ".github/tools/src/scripts/node-range.ts",
  ]) {
    const dest = path.join(root, name);
    fs.mkdirSync(path.dirname(dest), { recursive: true });
    fs.copyFileSync(path.join(source, name), dest);
  }
  const hook = path.join(root, ".github/husky/_/pre-commit");
  fs.mkdirSync(path.dirname(hook), { recursive: true });
  fs.writeFileSync(hook, "fixture", { mode: 0o755 });
  const log = path.join(root, "commands.jsonl");
  const probe = path.join(root, "intercept.cjs");
  fs.writeFileSync(
    probe,
    `
const fs = require('node:fs');
require('node:child_process').spawnSync = (command, args, options) => {
  fs.appendFileSync(${JSON.stringify(log)}, JSON.stringify({ command, args, cwd: options.cwd }) + '\\n');
  const npm = command === 'npm' || (command.toLowerCase() === 'cmd.exe' && args[0] === '/d' && args[1] === '/s' && args[2] === '/c' && /^npm\\.cmd (?:--version|ci --include=dev --ignore-scripts)$/.test(args[3]));
  const gitVersion = command === 'git' && JSON.stringify(args) === JSON.stringify(['--version']);
  const gitRead = command === 'git' && JSON.stringify(args) === JSON.stringify(['config', '--get', 'core.hooksPath']);
  const go = command === 'go' && JSON.stringify(args) === JSON.stringify(['mod', 'download']);
  const installer = command === process.execPath && JSON.stringify(args) === JSON.stringify(['.github/tools/src/scripts/install-hooks.ts', '--required']);
  if (!(npm || gitVersion || gitRead || go || installer)) throw new Error('Unexpected child command: ' + command + ' ' + args.join(' '));
  const npmVersion = npm && args[args.length - 1].endsWith('--version');
  return { status: 0, stderr: '', stdout: gitRead ? '.github/husky/_\\n' : npmVersion ? '12.2.2\\n' : '' };
};
require("node:module").syncBuiltinESMExports();
`,
  );
  const result = spawnSync(
    process.execPath,
    [".github/tools/src/scripts/setup-worktree.ts"],
    {
      cwd: root,
      encoding: "utf8",
      env: {
        ...fixtureGitEnv(),
        NODE_OPTIONS: `--require=${JSON.stringify(probe)}`,
      },
    },
  );
  assert.equal(result.status, 0, result.stdout + result.stderr);
  const commands = fs
    .readFileSync(log, "utf8")
    .trim()
    .split("\n")
    // Every line was produced by the preload above with this exact shape.
    .map((line): CommandCall => JSON.parse(line));
  assert.deepEqual(
    commands.filter((call) => call.command === "go").map((call) => call.cwd),
    [path.join(root, "hooks/go")],
    "go mod download working directories",
  );
  assert.equal(
    commands.filter((call) => call.command === process.execPath).length,
    1,
  );
  assert.equal(
    commands.filter(
      (call) =>
        call.command === "npm" || call.command.toLowerCase() === "cmd.exe",
    ).length,
    2,
  );
});

// Runs setup with a fake npm first on PATH. Only npm reaches a real process;
// the preload stubs git, go, and the hook installer so the run isolates the
// npm floor check. The fake records every non-version call in the log.
function runWithFakeNpm(t: TestContext, version: string) {
  const root = fs.mkdtempSync(
    path.join(fs.realpathSync(os.tmpdir()), "agentics-setup-npm-"),
  );
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  for (const name of [
    "package.json",
    ".github/tools/src/scripts/setup-worktree.ts",
    ".github/tools/src/scripts/node-range.ts",
  ]) {
    const dest = path.join(root, name);
    fs.mkdirSync(path.dirname(dest), { recursive: true });
    fs.copyFileSync(path.join(source, name), dest);
  }
  const hook = path.join(root, ".github/husky/_/pre-commit");
  fs.mkdirSync(path.dirname(hook), { recursive: true });
  fs.writeFileSync(hook, "fixture", { mode: 0o755 });
  const bin = path.join(root, "fake-bin");
  fs.mkdirSync(bin);
  const log = path.join(root, "npm-calls.log");
  fs.writeFileSync(
    path.join(bin, "npm"),
    `#!/bin/sh\nif [ "$1" = "--version" ]; then echo '${version}'; exit 0; fi\necho "$*" >> '${log}'\n`,
    { mode: 0o755 },
  );
  fs.writeFileSync(
    path.join(bin, "npm.cmd"),
    `@echo off\r\nif "%1"=="--version" (\r\n  echo ${version}\r\n  exit /b 0\r\n)\r\necho %*>>"${log}"\r\n`,
  );
  const probe = path.join(root, "stub-others.cjs");
  fs.writeFileSync(
    probe,
    `
const cp = require('node:child_process');
const original = cp.spawnSync;
cp.spawnSync = function(command, args, options) {
  if (command === 'npm' || command.toLowerCase() === 'cmd.exe') return original.call(this, command, args, options);
  const gitRead = command === 'git' && args[0] === 'config';
  return { status: 0, stderr: '', stdout: gitRead ? '.github/husky/_\\n' : '' };
};
require("node:module").syncBuiltinESMExports();
`,
  );
  const env = fixtureGitEnv();
  // Windows spells the key Path; replace it rather than add a second entry.
  const pathKey = Object.keys(env).find((key) => /^path$/i.test(key)) ?? "PATH";
  env[pathKey] = `${bin}${path.delimiter}${env[pathKey] ?? ""}`;
  env.NODE_OPTIONS = `--require=${JSON.stringify(probe)}`;
  const result = spawnSync(
    process.execPath,
    [".github/tools/src/scripts/setup-worktree.ts"],
    { cwd: root, encoding: "utf8", env, timeout: 30_000 },
  );
  const calls = fs.existsSync(log)
    ? fs.readFileSync(log, "utf8").trim().split(/\r?\n/)
    : [];
  return { result, calls };
}

test("setup refuses an npm older than engines.npm before installing", (t) => {
  const { result, calls } = runWithFakeNpm(t, "12.2.1");
  assert.equal(result.status, 1, result.stdout + result.stderr);
  assert.match(result.stderr, /npm >=12\.2\.2 required; found 12\.2\.1/);
  assert.match(result.stderr, /npm install --global npm@12\.2\.2/);
  assert.deepEqual(calls, [], "no npm install ran");
  assert.doesNotMatch(result.stdout, /Installing pinned development tools/);
});

test("setup refuses npm --version output that is not a version", (t) => {
  const { result, calls } = runWithFakeNpm(t, "garbage");
  assert.equal(result.status, 1, result.stdout + result.stderr);
  assert.match(result.stderr, /npm --version printed "garbage"/);
  assert.deepEqual(calls, []);
});

for (const version of ["12.2.2", "12.3.0"]) {
  test(`setup proceeds with npm ${version}`, (t) => {
    const { result, calls } = runWithFakeNpm(t, version);
    assert.equal(result.status, 0, result.stdout + result.stderr);
    assert.deepEqual(calls, ["ci --include=dev --ignore-scripts"]);
  });
}

function fixture(t: TestContext, denyConfig = true) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "agentics-setup-"));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  for (const name of [
    "package.json",
    ".github/tools/package.json",
    "package-lock.json",
    ".github/tools/src/scripts/setup-worktree.ts",
    ".github/tools/src/scripts/node-range.ts",
    ".github/husky/pre-commit",
  ]) {
    const dest = path.join(root, name);
    fs.mkdirSync(path.dirname(dest), { recursive: true });
    fs.copyFileSync(path.join(source, name), dest);
  }
  const installer = ".github/tools/src/scripts/install-hooks.ts";
  if (fs.existsSync(path.join(source, installer))) {
    fs.copyFileSync(path.join(source, installer), path.join(root, installer));
  }
  git(root, "init", "--quiet");
  git(root, "config", "--local", "core.hooksPath", ".github/husky/_");
  const probe = path.join(root, "deny-config.cjs");
  fs.writeFileSync(
    probe,
    `
const cp = require('node:child_process');
const original = cp.spawnSync;
cp.spawnSync = function(command, args, options) {
  // Go bootstrap is independent of hook setup and absent from the formatting CI job.
  if (command === 'go') return { status: 0, stderr: '', stdout: '' };
  if (${denyConfig} && command === 'git' && args[0] === 'config' && args[1] === 'core.hooksPath') {
    return { status: 128, stderr: 'error: could not lock config file: Permission denied', stdout: '' };
  }
  return original.call(this, command, args, options);
};
require("node:module").syncBuiltinESMExports();
`,
  );
  return { root, probe };
}

test("setup generates fresh worktree hooks without rewriting an active shared hooksPath", (t) => {
  const { root, probe } = fixture(t);
  const result = runSetup(root, probe);
  assert.equal(result.status, 0, result.stdout + result.stderr);
  assert.doesNotMatch(result.stdout + result.stderr, /Permission denied/);
  assert.doesNotMatch(result.stdout + result.stderr, /lint-staged/);
  assert.match(result.stdout, /dependencies and Git hooks are ready/);
  assert.ok(fs.existsSync(path.join(root, ".github/husky/_/pre-commit")));
  assert.ok(fs.existsSync(path.join(root, ".github/husky/_/h")));

  fs.writeFileSync(
    path.join(root, ".github/husky/pre-commit"),
    'command -v prettier > hook-ran\nprintf "%s\\n" "$@" >> hook-ran\nexit 7\n',
  );
  const hook = spawnSync(
    "git",
    ["hook", "run", "pre-commit", "--", "first", "second"],
    {
      cwd: root,
      encoding: "utf8",
      env: { ...hooksEnv(), HUSKY: "" },
    },
  );
  assert.equal(hook.status, 7, hook.stdout + hook.stderr);
  assert.match(
    fs.readFileSync(path.join(root, "hook-ran"), "utf8"),
    /prettier[^\n]*\nfirst\nsecond\n/,
  );
});

test("hooks added after setup do not run", (t) => {
  const { root, probe } = fixture(t);
  const result = runSetup(root, probe);
  assert.equal(result.status, 0, result.stdout + result.stderr);
  const generated = path.join(root, ".github/husky/_");
  assert.deepEqual(fs.readdirSync(generated).sort(), [
    ".gitignore",
    "h",
    "pre-commit",
  ]);
  for (const name of ["commit-msg", "post-checkout"]) {
    assert.equal(fs.existsSync(path.join(generated, name)), false, name);
    fs.writeFileSync(
      path.join(root, ".github/husky", name),
      `: > added-${name}-ran\nexit 9\n`,
    );
    const hook = spawnSync("git", ["hook", "run", "--ignore-missing", name], {
      cwd: root,
      encoding: "utf8",
      env: { ...hooksEnv(), HUSKY: "" },
    });
    assert.equal(hook.status, 0, hook.stdout + hook.stderr);
    assert.equal(fs.existsSync(path.join(root, `added-${name}-ran`)), false);
  }
});

test("setup removes stale Husky launchers and keeps other tools' files", (t) => {
  const { root, probe } = fixture(t);
  const generated = path.join(root, ".github/husky/_");
  fs.mkdirSync(generated, { recursive: true });
  const seed = (name: string, text: string) =>
    fs.writeFileSync(path.join(generated, name), text, { mode: 0o755 });
  const removed = [
    "post-checkout",
    "post-merge",
    "post-commit.pre-entire",
    "pre-commit.pre-entire",
  ];
  seed("post-checkout", launcher);
  seed("post-merge", `${launcher}\n`);
  seed("post-commit.pre-entire", launcher);
  seed("pre-commit.pre-entire", launcher);
  [
    "pre-merge-commit",
    "prepare-commit-msg",
    "commit-msg",
    "applypatch-msg",
    "pre-applypatch",
    "post-applypatch",
    "post-rewrite",
    "pre-auto-gc",
  ].forEach((name, index) => {
    seed(name, index % 2 === 0 ? launcher : `${launcher}\n`);
    removed.push(name);
  });
  const kept: Record<string, string> = {
    "post-commit": entireWrapper,
    notes: "kept by another tool\n",
    "pre-rebase": `${launcher}\necho extra\n`,
    "pre-push": `${launcher.replaceAll("\n", "\r\n")}\r\n`,
  };
  for (const [name, text] of Object.entries(kept)) seed(name, text);
  seed("pre-commit", launcher);
  const directory = path.join(generated, "pre-auto-gc.d");
  fs.mkdirSync(directory);
  fs.writeFileSync(path.join(directory, "inner"), launcher);
  const link = path.join(generated, "stale-link");
  const linkTarget = path.join(root, "link-target");
  const linked = process.platform !== "win32";
  if (linked) {
    fs.writeFileSync(linkTarget, launcher);
    fs.symlinkSync(linkTarget, link);
  }

  const result = runSetup(root, probe);
  assert.equal(result.status, 0, result.stdout + result.stderr);
  assert.doesNotMatch(result.stdout + result.stderr, /lint-staged/);
  for (const name of removed)
    assert.equal(fs.existsSync(path.join(generated, name)), false, name);
  for (const entry of fs.readdirSync(generated)) {
    if (entry === "pre-commit") continue;
    assert.equal(isPlainLauncher(path.join(generated, entry)), false, entry);
  }
  for (const [name, text] of Object.entries(kept))
    assert.equal(fs.readFileSync(path.join(generated, name), "utf8"), text);
  assert.ok(fs.lstatSync(directory).isDirectory());
  assert.deepEqual(fs.readdirSync(directory), ["inner"]);
  assert.equal(
    fs.readFileSync(path.join(directory, "inner"), "utf8"),
    launcher,
  );
  if (linked) {
    assert.ok(fs.lstatSync(link).isSymbolicLink());
    assert.equal(fs.readlinkSync(link), linkTarget);
    assert.equal(fs.readFileSync(linkTarget, "utf8"), launcher);
  }
  assert.equal(
    fs.readFileSync(path.join(generated, "pre-commit"), "utf8"),
    `${launcher}\n`,
  );
});

test("setup fails actionably when shared hook activation is denied", (t) => {
  const { root, probe } = fixture(t);
  git(root, "config", "--local", "core.hooksPath", "other-hooks");
  const result = runSetup(root, probe);
  assert.equal(result.status, 1);
  assert.match(
    result.stdout + result.stderr,
    /Cannot activate Git hooks: error: could not lock config file: Permission denied/,
  );
  assert.match(
    result.stdout + result.stderr,
    /Run npm run prepare from a terminal with write access/,
  );
  assert.doesNotMatch(result.stdout, /dependencies and Git hooks are ready/);
});

test("setup rejects HUSKY=0 even when old hook files exist", (t) => {
  const { root, probe } = fixture(t);
  fs.mkdirSync(path.join(root, ".github/husky/_"));
  fs.writeFileSync(path.join(root, ".github/husky/_/pre-commit"), "stale", {
    mode: 0o755,
  });
  const result = runSetup(root, probe, "0");
  assert.equal(result.status, 1);
  assert.match(
    result.stdout + result.stderr,
    /HUSKY=0 disables Git hooks; unset it before worktree setup/,
  );
  assert.doesNotMatch(result.stdout, /dependencies and Git hooks are ready/);
});

test("setup activates a missing hook path when shared configuration is writable", (t) => {
  const { root, probe } = fixture(t, false);
  git(root, "config", "--local", "--unset", "core.hooksPath");
  const result = runSetup(root, probe);
  assert.equal(result.status, 0, result.stdout + result.stderr);
  assert.equal(
    git(root, "config", "--local", "--get", "core.hooksPath").trim(),
    ".github/husky/_",
  );
  assert.ok(fs.existsSync(path.join(root, ".github/husky/_/pre-commit")));
});
