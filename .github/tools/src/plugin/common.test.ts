import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import {
  chmodSync,
  existsSync,
  mkdirSync,
  mkdtempSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import os from "node:os";
import path from "node:path";
import { after, describe, test } from "node:test";
import {
  MARKETPLACE_NAME,
  PLUGINS,
  REPO_ROOT,
  cacheRoot,
  commandCandidates,
  detectClis,
  findCommand,
  spawnPlan,
  toolCacheDir,
} from "./common.ts";

const isWindows = process.platform === "win32";
const winOnly = isWindows
  ? false
  : "Windows only: needs Windows path and PATHEXT semantics";
const posixOnly = isWindows ? "POSIX only: needs the executable bit" : false;

const roots: string[] = [];
function tempDir(): string {
  const dir = mkdtempSync(path.join(os.tmpdir(), "agentics-common-"));
  roots.push(dir);
  return dir;
}
after(() => {
  for (const dir of roots) rmSync(dir, { recursive: true, force: true });
});

function touch(file: string, executable = true): void {
  mkdirSync(path.dirname(file), { recursive: true });
  writeFileSync(file, "");
  if (!isWindows) chmodSync(file, executable ? 0o755 : 0o644);
}

describe("repository facts", () => {
  test("REPO_ROOT is the checkout root whatever the cwd", () => {
    const nested = path.join(REPO_ROOT, ".github", "tools", "src");
    const result = spawnSync(
      process.execPath,
      [
        "--input-type=module",
        "-e",
        `import { REPO_ROOT } from ${JSON.stringify(new URL("./common.ts", import.meta.url).href)}; process.stdout.write(REPO_ROOT);`,
      ],
      { cwd: nested, encoding: "utf8" },
    );
    assert.equal(result.status, 0, result.stderr);
    assert.equal(result.stdout, REPO_ROOT);
    assert.ok(path.isAbsolute(REPO_ROOT));
    assert.ok(
      existsSync(path.join(REPO_ROOT, ".github", "tools", "package.json")),
    );
  });

  test("constants", () => {
    assert.equal(MARKETPLACE_NAME, "muehz");
    assert.deepEqual(PLUGINS, ["agentics"]);
  });
});

describe("cache root", () => {
  test("XDG_CACHE_HOME wins when non-empty", () => {
    const xdg = path.resolve("xdg-cache");
    assert.equal(
      cacheRoot({ XDG_CACHE_HOME: xdg, HOME: path.resolve("h") }),
      xdg,
    );
    assert.equal(
      toolCacheDir({ XDG_CACHE_HOME: xdg }),
      path.join(xdg, "agentics"),
    );
  });

  test("HOME is used when XDG_CACHE_HOME is unset or empty", () => {
    const home = path.resolve("home-dir");
    assert.equal(cacheRoot({ HOME: home }), path.join(home, ".cache"));
    assert.equal(
      cacheRoot({ XDG_CACHE_HOME: "", HOME: home }),
      path.join(home, ".cache"),
    );
    assert.equal(
      toolCacheDir({ HOME: home }),
      path.join(home, ".cache", "agentics"),
    );
  });

  test("os.homedir is the fallback when HOME is unset or empty", () => {
    const expected = path.join(os.homedir(), ".cache");
    assert.equal(cacheRoot({}), expected);
    assert.equal(cacheRoot({ HOME: "", XDG_CACHE_HOME: "" }), expected);
  });

  test("HOME wins over USERPROFILE", { skip: winOnly }, () => {
    const home = path.resolve("home-wins");
    const profile = path.resolve("profile-loses");
    assert.equal(
      cacheRoot({ HOME: home, USERPROFILE: profile }),
      path.join(home, ".cache"),
    );
  });
});

describe("command lookup", () => {
  test("empty PATH entries are ignored and Path is read when PATH is absent", () => {
    const dir = tempDir();
    const bin = path.join(dir, isWindows ? "tool.exe" : "tool");
    touch(bin);
    const env = isWindows
      ? { Path: `;${dir};`, PATHEXT: ".EXE" }
      : { Path: `:${dir}:` };
    assert.deepEqual(commandCandidates("tool", env), [bin]);
    assert.equal(findCommand("tool", {}), undefined);
    assert.equal(findCommand("tool", { PATH: "" }), undefined);
  });

  test(
    "PATHEXT matching ignores case; extensionless files and directories never match",
    { skip: winOnly },
    () => {
      const one = tempDir();
      const two = tempDir();
      touch(path.join(one, "claude.cmd"));
      touch(path.join(two, "claude.CMD"));
      touch(path.join(one, "plain"));
      mkdirSync(path.join(two, "dirname.cmd"));
      touch(path.join(two, "claude"));
      const env = { PATH: `${one};${two}`, PATHEXT: ".cmd;.exe" };
      assert.deepEqual(commandCandidates("claude", env, "win32"), [
        path.join(one, "claude.cmd"),
        path.join(two, "claude.CMD"),
      ]);
      assert.equal(
        findCommand("claude", env, "win32"),
        path.join(one, "claude.cmd"),
      );
      assert.equal(findCommand("plain", env, "win32"), undefined);
      assert.equal(findCommand("dirname", env, "win32"), undefined);
      const upper = { PATH: one, PATHEXT: ".CMD;.EXE" };
      assert.equal(
        findCommand("claude", upper, "win32"),
        path.join(one, "claude.cmd"),
      );
    },
  );

  test(
    "an executable file matches and candidates keep PATH order",
    { skip: posixOnly },
    () => {
      const one = tempDir();
      const two = tempDir();
      touch(path.join(one, "claude"));
      touch(path.join(two, "claude"));
      const env = { PATH: `${one}${path.delimiter}${two}`, PATHEXT: ".EXE" };
      assert.deepEqual(commandCandidates("claude", env, "linux"), [
        path.join(one, "claude"),
        path.join(two, "claude"),
      ]);
      assert.equal(
        findCommand("claude", env, "linux"),
        path.join(one, "claude"),
      );
    },
  );

  test(
    "a non-executable file and a directory never match",
    { skip: posixOnly },
    () => {
      const dir = tempDir();
      touch(path.join(dir, "claude"), false);
      mkdirSync(path.join(dir, "codex"));
      const env = { PATH: dir };
      assert.equal(findCommand("claude", env, "linux"), undefined);
      assert.equal(findCommand("codex", env, "linux"), undefined);
    },
  );
});

describe("detectClis", () => {
  function fixture(names: string[]): { PATH: string; PATHEXT: string } {
    const dir = tempDir();
    for (const name of names)
      touch(path.join(dir, isWindows ? `${name}.cmd` : name));
    return { PATH: dir, PATHEXT: ".CMD;.EXE" };
  }

  test("lists claude, copilot, codex, agy in that order", () => {
    assert.deepEqual(
      detectClis(fixture(["agy", "codex", "claude", "copilot"])),
      ["claude", "copilot", "codex", "agy"],
    );
  });

  test("omits a missing CLI", () => {
    assert.deepEqual(detectClis(fixture(["codex"])), ["codex"]);
    assert.deepEqual(detectClis(fixture([])), []);
  });
});

describe("spawnPlan", () => {
  test("on Windows a .cmd starts through cmd.exe with every argument quoted", () => {
    const npm = "C:\\Program Files\\nodejs\\npm.cmd";
    const args = ["--prefix", "C:\\work\\a b&c\\", "run", "typecheck"];
    const expectedLine = `""${npm}" "--prefix" "C:\\work\\a b&c\\\\" "run" "typecheck""`;
    assert.deepEqual(
      spawnPlan(npm, args, "win32", {
        ComSpec: "C:\\Windows\\system32\\cmd.exe",
      }),
      {
        file: "C:\\Windows\\system32\\cmd.exe",
        args: ["/d", "/s", "/c", expectedLine],
        windowsVerbatimArguments: true,
      },
    );
    // A bare cmd.exe could be found in the working directory first, so the fallback is always absolute.
    const fallback = "C:\\Windows\\System32\\cmd.exe";
    assert.equal(spawnPlan(npm, args, "win32", {}).file, fallback);
    assert.equal(
      spawnPlan(npm, args, "win32", { ComSpec: "", SystemRoot: "" }).file,
      fallback,
    );
    assert.equal(
      spawnPlan(npm, args, "win32", { COMSPEC: "D:\\cmd.exe" }).file,
      "D:\\cmd.exe",
    );
    assert.equal(
      spawnPlan(npm, args, "win32", { SystemRoot: "D:\\Win" }).file,
      "D:\\Win\\System32\\cmd.exe",
    );
    assert.equal(
      spawnPlan(npm, args, "win32", { SYSTEMROOT: "E:\\W" }).file,
      "E:\\W\\System32\\cmd.exe",
    );
    assert.equal(spawnPlan("C:\\x\\TOOL.BAT", [], "win32", {}).file, fallback);
    for (const bad of ['a"b', "50%", "wow!", "a\nb", "a\rb"]) {
      assert.throws(
        () => spawnPlan(npm, ["run", bad], "win32", {}),
        /cannot pass/,
        bad,
      );
    }
    assert.throws(() => spawnPlan("C:\\100%\\npm.cmd", [], "win32", {}));
  });

  test("real executables and POSIX commands start directly", () => {
    const args = ["build", 'say "hi" 100%!'];
    assert.deepEqual(spawnPlan("C:\\Go\\bin\\go.exe", args, "win32", {}), {
      file: "C:\\Go\\bin\\go.exe",
      args,
      windowsVerbatimArguments: false,
    });
    assert.deepEqual(spawnPlan("/usr/bin/npm.cmd", args, "linux", {}), {
      file: "/usr/bin/npm.cmd",
      args,
      windowsVerbatimArguments: false,
    });
  });
});
