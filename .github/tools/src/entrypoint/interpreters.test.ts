import assert from "node:assert/strict";
import {
  chmodSync,
  copyFileSync,
  mkdirSync,
  mkdtempSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import os from "node:os";
import path from "node:path";
import { after, describe, test } from "node:test";
import {
  findInterpreter,
  requireInterpreter,
  twinPowerShell,
  type ProbeRunner,
} from "./interpreters.ts";

const isWindows = process.platform === "win32";
const winOnly = isWindows
  ? false
  : "Windows only: needs PATHEXT and a .exe copy of node";
const posixOnly = isWindows
  ? "POSIX only: needs a shebang script with the executable bit"
  : false;

const roots: string[] = [];
after(() => {
  for (const root of roots) rmSync(root, { recursive: true, force: true });
});

function tempDir(): string {
  const dir = mkdtempSync(path.join(os.tmpdir(), "interp-"));
  roots.push(dir);
  return dir;
}

function suffix(name: string): string {
  return isWindows ? `${name}.exe` : name;
}

// Fakes live in a directory that is the only PATH entry. Names carry .exe on
// Windows so PATHEXT lookup finds them.
function fakeDir(names: string[]): {
  dir: string;
  env: Record<string, string>;
} {
  const dir = tempDir();
  for (const name of names) {
    const file = path.join(dir, suffix(name));
    writeFileSync(file, "");
    chmodSync(file, 0o755);
  }
  return { dir, env: { PATH: dir, PATHEXT: ".EXE;.CMD;.BAT" } };
}

interface Call {
  file: string;
  args: string[];
  timeout: number;
}

function runnerWith(status: (file: string) => number | null): {
  run: ProbeRunner;
  calls: Call[];
} {
  const calls: Call[] = [];
  const run: ProbeRunner = (file, args, options) => {
    calls.push({ file, args, timeout: options.timeout });
    return { status: status(file) };
  };
  return { run, calls };
}

const ok = () => 0;

describe("requireInterpreter", () => {
  test("a present sh returns no skip", () => {
    const { env } = fakeDir(["sh"]);
    assert.equal(
      requireInterpreter("sh", { env, run: runnerWith(ok).run }),
      false,
    );
  });

  test("a missing pwsh returns a skip naming pwsh", () => {
    const { env } = fakeDir([]);
    const skip = requireInterpreter("pwsh", { env, run: runnerWith(ok).run });
    assert.equal(typeof skip, "string");
    assert.match(String(skip), /pwsh/);
  });

  test("a missing pwsh listed as required throws, with trimmed entries", () => {
    const { env } = fakeDir(["sh"]);
    for (const list of ["sh,pwsh", " sh , pwsh "]) {
      assert.throws(
        () =>
          requireInterpreter("pwsh", {
            env: { ...env, AGX_REQUIRED_INTERPRETERS: list },
            run: runnerWith(ok).run,
          }),
        /pwsh/,
      );
    }
  });

  test("CI alone never turns a missing bash into a failure", () => {
    const { env } = fakeDir([]);
    const skip = requireInterpreter("bash", {
      env: { ...env, CI: "true" },
      run: runnerWith(ok).run,
    });
    assert.match(String(skip), /bash/);
  });

  test("an unknown list entry throws naming it", () => {
    const { env } = fakeDir(["sh"]);
    assert.throws(
      () =>
        requireInterpreter("sh", {
          env: { ...env, AGX_REQUIRED_INTERPRETERS: "sh,bsh" },
          run: runnerWith(ok).run,
        }),
      /bsh/,
    );
  });

  test("a name outside the four interpreters throws", () => {
    assert.throws(() => requireInterpreter("zsh", { env: {} }), /zsh/);
    assert.throws(() => findInterpreter("zsh", { env: {} }), /zsh/);
  });
});

describe("findInterpreter", () => {
  test("a failing first candidate is passed over for the second", () => {
    const first = fakeDir(["sh"]);
    const second = fakeDir(["sh"]);
    const sep = isWindows ? ";" : ":";
    const env = { PATH: `${first.dir}${sep}${second.dir}`, PATHEXT: ".EXE" };
    const { run, calls } = runnerWith((file) =>
      file.startsWith(first.dir) ? 1 : 0,
    );
    assert.equal(
      findInterpreter("sh", { env, run }),
      path.join(second.dir, suffix("sh")),
    );
    assert.equal(calls.length, 2);
    assert.ok(calls[0]!.file.startsWith(first.dir));
  });

  test("probe argv per interpreter and a ten second timeout", () => {
    const names = ["sh", "bash", "pwsh", "powershell"];
    const { env } = fakeDir(names);
    for (const name of names) {
      const { run, calls } = runnerWith(ok);
      findInterpreter(name, { env, run });
      const expected =
        name === "sh" || name === "bash"
          ? ["-c", "exit 0"]
          : ["-NoProfile", "-Command", "exit 0"];
      assert.deepEqual(calls[0]!.args, expected, name);
      assert.equal(calls[0]!.timeout, 10_000);
    }
  });

  test("a probe that errors, as a timeout does, counts as failing", () => {
    const { env } = fakeDir(["sh"]);
    const run: ProbeRunner = () => ({
      status: null,
      error: new Error("ETIMEDOUT"),
    });
    assert.equal(findInterpreter("sh", { env, run }), undefined);
  });

  test("a .cmd candidate starts through ComSpec /d /s /c, never directly", () => {
    const dir = tempDir();
    writeFileSync(path.join(dir, "pwsh.cmd"), "");
    const comspec = path.join(dir, "cmd.exe");
    const env = { PATH: dir, PATHEXT: ".EXE;.CMD;.BAT", ComSpec: comspec };
    const { run, calls } = runnerWith(ok);
    findInterpreter("pwsh", { env, run, platform: "win32" });
    assert.equal(calls.length, 1);
    assert.equal(calls[0]!.file, comspec);
    assert.deepEqual(calls[0]!.args, [
      "/d",
      "/s",
      "/c",
      `""${path.join(dir, "pwsh.cmd")}" "-NoProfile" "-Command" "exit 0""`,
    ]);
  });

  test("a .cmd candidate cmd.exe cannot quote safely is passed over, not started", () => {
    const root = tempDir();
    const unsafe = path.join(root, "100%");
    const safe = path.join(root, "safe");
    for (const dir of [unsafe, safe]) {
      mkdirSync(dir);
      writeFileSync(path.join(dir, "pwsh.cmd"), "");
    }
    const env = {
      PATH: [unsafe, safe].join(";"),
      PATHEXT: ".EXE;.CMD;.BAT",
    };
    const { run, calls } = runnerWith(ok);
    assert.equal(
      findInterpreter("pwsh", { env, run, platform: "win32" }),
      path.join(safe, "pwsh.cmd"),
    );
    assert.equal(calls.length, 1);
    assert.equal(calls[0]!.file, "C:\\Windows\\System32\\cmd.exe");
  });
});

describe("twinPowerShell", () => {
  const both = () => fakeDir(["pwsh", "powershell"]);

  test("AGX_TWIN_POWERSHELL=powershell selects powershell", () => {
    const { env } = both();
    const twin = twinPowerShell({
      env: { ...env, AGX_TWIN_POWERSHELL: "powershell" },
      run: runnerWith(ok).run,
    });
    assert.equal(twin.name, "powershell");
    assert.equal(twin.skip, false);
    assert.ok(twin.path?.endsWith(suffix("powershell")));
  });

  test("unset or empty selects pwsh when it passes, else powershell", () => {
    const { env } = both();
    for (const value of [undefined, ""]) {
      const e: Record<string, string | undefined> = {
        ...env,
        AGX_TWIN_POWERSHELL: value,
      };
      assert.equal(
        twinPowerShell({ env: e, run: runnerWith(ok).run }).name,
        "pwsh",
      );
      const noPwsh = runnerWith((file) =>
        path.basename(file).startsWith("pwsh") ? 1 : 0,
      );
      assert.equal(
        twinPowerShell({ env: e, run: noPwsh.run }).name,
        "powershell",
      );
    }
  });

  test("an invalid value throws naming it", () => {
    const { env } = both();
    assert.throws(
      () =>
        twinPowerShell({
          env: { ...env, AGX_TWIN_POWERSHELL: "pwsh7" },
          run: runnerWith(ok).run,
        }),
      /pwsh7/,
    );
  });

  test("a missing selected PowerShell skips, or throws when required", () => {
    const { env } = fakeDir([]);
    const twin = twinPowerShell({ env, run: runnerWith(ok).run });
    assert.equal(twin.path, undefined);
    assert.match(String(twin.skip), /powershell/);
    assert.throws(
      () =>
        twinPowerShell({
          env: { ...env, AGX_REQUIRED_INTERPRETERS: "powershell" },
          run: runnerWith(ok).run,
        }),
      /powershell/,
    );
  });
});

describe("required list edge cases", () => {
  test("unset selection, both PowerShells missing, list naming pwsh throws", () => {
    const { env } = fakeDir([]);
    assert.throws(
      () =>
        twinPowerShell({
          env: { ...env, AGX_REQUIRED_INTERPRETERS: "sh,bash,pwsh" },
          run: runnerWith(ok).run,
        }),
      /pwsh/,
    );
  });

  test("a trailing comma or empty entry is ignored", () => {
    const { env } = fakeDir(["sh"]);
    for (const list of ["sh,", ",sh", "sh, ,"]) {
      assert.equal(
        requireInterpreter("sh", {
          env: { ...env, AGX_REQUIRED_INTERPRETERS: list },
          run: runnerWith(ok).run,
        }),
        false,
      );
    }
  });
});

describe("real spawn", () => {
  test(
    "a bash.exe that fails its probe counts as missing",
    { skip: winOnly },
    () => {
      const dir = tempDir();
      copyFileSync(process.execPath, path.join(dir, "bash.exe"));
      const env = { PATH: dir, PATHEXT: ".EXE" };
      assert.match(String(requireInterpreter("bash", { env })), /bash/);
    },
  );

  test(
    "a script named sh that exits 0 is present, exiting 1 is missing",
    { skip: posixOnly },
    () => {
      for (const code of [0, 1]) {
        const dir = tempDir();
        const file = path.join(dir, "sh");
        writeFileSync(file, `#!/usr/bin/env node\nprocess.exit(${code});\n`);
        chmodSync(file, 0o755);
        const env = { PATH: dir };
        if (code === 0) {
          assert.equal(requireInterpreter("sh", { env }), false);
          assert.equal(findInterpreter("sh", { env }), file);
        } else {
          assert.match(String(requireInterpreter("sh", { env })), /sh/);
        }
      }
    },
  );
});
