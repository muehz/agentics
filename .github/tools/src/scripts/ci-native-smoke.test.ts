import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import {
  type CommandResult,
  type CommandRun,
  HOST_TABLE,
  main,
  makeArgs,
} from "./ci-native-smoke.ts";

const MODULE = path.join(import.meta.dirname, "ci-native-smoke.ts");
const BINARY = Buffer.from("not really a hook binary");
const SHA = createHash("sha256").update(BINARY).digest("hex");

const HOSTS = [
  ["Linux", "X64", "agx-linux-amd64"],
  ["Linux", "ARM64", "agx-linux-arm64"],
  ["macOS", "X64", "agx-darwin-amd64"],
  ["macOS", "ARM64", "agx-darwin-arm64"],
  ["Windows", "X64", "agx-windows-amd64.exe"],
  ["Windows", "ARM64", "agx-windows-arm64.exe"],
] as const;

function tempDir(t: test.TestContext): string {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "agentics-native-smoke-"));
  t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
  return dir;
}

interface Call {
  command: string;
  args: string[];
  cwd: string | undefined;
}

// A checkout with the manifest line for `name`; the fake make writes the binary it would have built.
function checkout(
  t: test.TestContext,
  name: string,
  manifest = `${SHA}  bin/${name}\n${"0".repeat(64)}  bin/other\n`,
) {
  const root = tempDir(t);
  fs.mkdirSync(path.join(root, "hooks", "bin"), { recursive: true });
  fs.writeFileSync(path.join(root, "hooks", "SHA256SUMS"), manifest);
  return root;
}

function fakes(
  root: string,
  name: string,
  options: {
    make?: Partial<CommandResult>;
    git?: Partial<CommandResult>;
    uname?: string;
    binary?: Buffer;
  } = {},
) {
  const calls: Call[] = [];
  const run: CommandRun = (command, args, runOptions) => {
    calls.push({ command, args, cwd: runOptions.cwd });
    if (command === "make") {
      fs.writeFileSync(
        path.join(root, "hooks", "bin", name),
        options.binary ?? BINARY,
      );
      return { status: 0, stdout: "", ...options.make };
    }
    if (command === "uname") {
      return { status: 0, stdout: `${options.uname ?? "x86_64"}\n` };
    }
    return { status: 0, stdout: "", ...options.git };
  };
  return { calls, run };
}

function build(
  root: string,
  run: CommandRun,
  env: Record<string, string | undefined>,
) {
  const errors: string[] = [];
  const out: string[] = [];
  const status = main(["build"], {
    env,
    cwd: root,
    run,
    write: (line) => out.push(line),
    report: (line) => errors.push(line),
  });
  return { status, errors, out };
}

for (const [runnerOs, arch, name] of HOSTS) {
  test(`build on ${runnerOs}/${arch} builds, verifies and diffs ${name}`, (t) => {
    const root = checkout(t, name);
    const f = fakes(root, name);
    const r = build(root, f.run, { RUNNER_OS: runnerOs, RUNNER_ARCH: arch });
    assert.equal(r.status, 0, r.errors.join("\n"));
    assert.deepEqual(
      f.calls.map((c) => [c.command, ...c.args]),
      [
        ["make", "-B", "-C", "hooks/go", `../bin/${name}`],
        [
          "git",
          "diff",
          "--exit-code",
          "--",
          "hooks/SHA256SUMS",
          `hooks/bin/${name}`,
        ],
      ],
    );
    assert.equal(f.calls[0]!.cwd, root);
  });
}

test("the exported host table is the six supported hosts and the make arguments name one binary", () => {
  assert.deepEqual(
    Object.keys(HOST_TABLE).sort(),
    HOSTS.map(([o, a]) => `${o}/${a}`).sort(),
  );
  for (const [runnerOs, arch, name] of HOSTS) {
    const host = HOST_TABLE[`${runnerOs}/${arch}`]!;
    assert.equal(`agx-${host.os}-${host.arch}${host.ext}`, name);
  }
  assert.deepEqual(makeArgs("agx-x"), ["-B", "-C", "hooks/go", "../bin/agx-x"]);
});

test("an unsupported host prints the exact line and runs nothing", (t) => {
  for (const env of [
    { RUNNER_OS: "FreeBSD", RUNNER_ARCH: "X64" },
    { RUNNER_OS: "Linux", RUNNER_ARCH: "X86" },
    {},
  ]) {
    const root = checkout(t, "agx-linux-amd64");
    const f = fakes(root, "agx-linux-amd64");
    const r = build(root, f.run, env);
    assert.equal(r.status, 1);
    assert.deepEqual(r.errors, [
      `unsupported host ${env.RUNNER_OS ?? ""}/${env.RUNNER_ARCH ?? ""}`,
    ]);
    assert.equal(f.calls.length, 0);
  }
});

const LINUX = { RUNNER_OS: "Linux", RUNNER_ARCH: "X64" };
const LINUX_NAME = "agx-linux-amd64";

test("a failing make fails the build and checks nothing further", (t) => {
  const root = checkout(t, LINUX_NAME);
  const f = fakes(root, LINUX_NAME, { make: { status: 2 } });
  const r = build(root, f.run, LINUX);
  assert.equal(r.status, 2);
  assert.deepEqual(
    f.calls.map((c) => c.command),
    ["make"],
  );
});

test("a make that cannot start is reported and fails", (t) => {
  const root = checkout(t, LINUX_NAME);
  const errors: string[] = [];
  const status = main(["build"], {
    env: LINUX,
    cwd: root,
    run: () => ({
      status: null,
      stdout: "",
      error: new Error("spawn make ENOENT"),
    }),
    report: (line) => errors.push(line),
  });
  assert.notEqual(status, 0);
  assert.match(errors.join("\n"), /^ci-native-smoke: cannot start make/);
});

test("a manifest without an entry for the binary fails", (t) => {
  for (const manifest of [
    `${SHA}  bin/other\n`,
    "",
    `${SHA}  hooks/bin/${LINUX_NAME}\n`,
  ]) {
    const root = checkout(t, LINUX_NAME, manifest);
    const f = fakes(root, LINUX_NAME);
    const r = build(root, f.run, LINUX);
    assert.equal(r.status, 1, manifest);
    assert.match(
      r.errors.join("\n"),
      /^ci-native-smoke: .*SHA256SUMS.*bin\/agx-linux-amd64/,
    );
    assert.deepEqual(
      f.calls.map((c) => c.command),
      ["make"],
    );
  }
});

test("a binary whose hash differs from the manifest fails before the diff", (t) => {
  const root = checkout(t, LINUX_NAME);
  const f = fakes(root, LINUX_NAME, { binary: Buffer.from("tampered") });
  const r = build(root, f.run, LINUX);
  assert.equal(r.status, 1);
  assert.match(r.errors.join("\n"), /^ci-native-smoke: .*hash/);
  assert.deepEqual(
    f.calls.map((c) => c.command),
    ["make"],
  );
});

test("a missing built binary fails", (t) => {
  const root = checkout(t, LINUX_NAME);
  const run: CommandRun = () => ({ status: 0, stdout: "" });
  const r = build(root, run, LINUX);
  assert.equal(r.status, 1);
  assert.match(r.errors.join("\n"), /^ci-native-smoke: cannot read/);
});

test("a non-zero git diff fails the build with its status", (t) => {
  const root = checkout(t, LINUX_NAME);
  const f = fakes(root, LINUX_NAME, { git: { status: 1 } });
  const r = build(root, f.run, LINUX);
  assert.equal(r.status, 1);
  const killed = fakes(root, LINUX_NAME, {
    git: { status: null, signal: "SIGKILL" },
  });
  const k = build(root, killed.run, LINUX);
  assert.equal(k.status, 1);
  assert.match(
    k.errors.join("\n"),
    /^ci-native-smoke: git was killed by SIGKILL/,
  );
});

test("arch macos passes on x86_64 and prints the architecture", () => {
  const f = fakes("", "", { uname: "x86_64" });
  const out: string[] = [];
  const status = main(["arch", "macos"], {
    env: {},
    run: f.run,
    write: (line) => out.push(line),
  });
  assert.equal(status, 0);
  assert.deepEqual(out, ["uname -m: x86_64"]);
  assert.deepEqual(
    f.calls.map((c) => [c.command, ...c.args]),
    [["uname", "-m"]],
  );
});

test("arch macos fails on arm64 with the exact line", () => {
  const f = fakes("", "", { uname: "arm64" });
  const out: string[] = [];
  const status = main(["arch", "macos"], {
    env: {},
    run: f.run,
    write: (line) => out.push(line),
  });
  assert.equal(status, 1);
  assert.deepEqual(out, [
    "uname -m: arm64",
    "::error::expected x86_64 on macos-15-intel, got arm64",
  ]);
});

test("arch macos reports a uname that cannot run", () => {
  const errors: string[] = [];
  const status = main(["arch", "macos"], {
    env: {},
    run: () => ({ status: 1, stdout: "" }),
    report: (line) => errors.push(line),
  });
  assert.equal(status, 1);
  const spawnFail = main(["arch", "macos"], {
    env: {},
    run: () => ({ status: null, stdout: "", error: new Error("spawn failed") }),
    report: (line) => errors.push(line),
  });
  assert.equal(spawnFail, 1);
  assert.match(errors.join("\n"), /^ci-native-smoke: cannot start uname/m);
});

test("arch windows reads PROCESSOR_ARCHITECTURE from the environment", () => {
  const ok: string[] = [];
  assert.equal(
    main(["arch", "windows"], {
      env: { PROCESSOR_ARCHITECTURE: "ARM64" },
      write: (line) => ok.push(line),
    }),
    0,
  );
  assert.deepEqual(ok, ["PROCESSOR_ARCHITECTURE: ARM64"]);
  for (const env of [{ PROCESSOR_ARCHITECTURE: "AMD64" }, {}]) {
    const out: string[] = [];
    const errors: string[] = [];
    const status = main(["arch", "windows"], {
      env,
      write: (line) => out.push(line),
      report: (line) => errors.push(line),
    });
    assert.equal(status, 1);
    assert.deepEqual(errors, [
      `expected ARM64 on windows-11-arm, got ${env.PROCESSOR_ARCHITECTURE ?? ""}`,
    ]);
  }
});

test("a missing or unknown subcommand prints the usage and exits 2", () => {
  for (const argv of [
    [],
    ["bogus"],
    ["arch"],
    ["arch", "linux"],
    ["build", "extra"],
  ]) {
    const errors: string[] = [];
    const status = main(argv, {
      env: {},
      run: () => assert.fail("nothing may start"),
      report: (line) => errors.push(line),
    });
    assert.equal(status, 2, argv.join(" "));
    assert.deepEqual(errors, [
      "usage: ci-native-smoke.ts build|arch macos|arch windows",
    ]);
  }
});

test("run as a script, the module sets the exit status", () => {
  const result = spawnSync(process.execPath, [MODULE], { encoding: "utf8" });
  assert.equal(result.status, 2);
  assert.match(
    result.stderr,
    /usage: ci-native-smoke\.ts build\|arch macos\|arch windows/,
  );
});
