import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { test } from "node:test";
import { fileURLToPath } from "node:url";
import { COMMANDS, dispatch, USAGE, type Loader } from "./cli.ts";
import type { Env, Run, RunOptions } from "./io.ts";
import { captureIo } from "./testing/support.ts";

const CLI = fileURLToPath(new URL("./cli.ts", import.meta.url));

// A loader that records the names it is asked for and serves one fake run.
function fakeLoader(run: Run): { load: Loader; asked: string[] } {
  const asked: string[] = [];
  return {
    asked,
    load: async (name) => {
      asked.push(name);
      return { run };
    },
  };
}

async function dispatchWith(
  argv: string[],
  run: Run = async () => 0,
): Promise<{ code: number; stdout: string; stderr: string; asked: string[] }> {
  const captured = captureIo();
  const { load, asked } = fakeLoader(run);
  const code = await dispatch(argv, captured.io, {}, undefined, load);
  return { code, stdout: captured.stdout(), stderr: captured.stderr(), asked };
}

test("no argument prints the usage and exits 2", async () => {
  const result = await dispatchWith([]);
  assert.equal(result.code, 2);
  assert.equal(result.stdout, "");
  assert.equal(result.stderr, `${USAGE}\n`);
  assert.match(result.stderr, /usage: release set-version/);
  assert.deepEqual(result.asked, []);
});

// Plausible names (`version`, `smoke`), help flags, an empty name and near-misses of `set-version` get no special case.
test("an unknown subcommand prints the usage alone and exits 2", async () => {
  for (const name of [
    "release",
    "version",
    "smoke",
    "",
    "--help",
    "-h",
    "Set-Version",
    "set-version ",
    "publish",
  ]) {
    const result = await dispatchWith([name, "--repo", ".", "--sha", "x"]);
    assert.equal(result.code, 2, name);
    assert.equal(result.stdout, "", name);
    assert.equal(result.stderr, `${USAGE}\n`, name);
    assert.deepEqual(result.asked, [], name);
  }
});

// The name reaches an import, so only the allowlist may pass.
test("a name that could reach another module or an object member is unknown", async () => {
  for (const name of [
    "constructor",
    "__proto__",
    "toString",
    "hasOwnProperty",
    "../peer/cli",
    "../../etc/passwd",
    "set-version/../../x",
    "commands/stage",
    "stage.ts",
    "stage\0",
  ]) {
    const result = await dispatchWith([name]);
    assert.equal(result.code, 2, name);
    assert.equal(result.stderr, `${USAGE}\n`, name);
    assert.deepEqual(result.asked, [], name);
  }
});

test("the usage names every subcommand and none that is not one", () => {
  assert.equal(
    USAGE,
    "usage: release set-version --repo ROOT --version MAJOR.MINOR.PATCH\n" +
      "subcommands: set-version, resume-check, check-commit, bundle, context, verify, stage, asset-roster",
  );
  for (const name of COMMANDS) assert.ok(USAGE.includes(name), name);
  const line = USAGE.split("\n").find((l) => l.startsWith("subcommands: "));
  // "version" is a substring of "set-version", so the loop above cannot see a
  // list that still names it.
  const listed = (line ?? "").slice("subcommands: ".length).split(", ");
  assert.deepEqual(listed, [...COMMANDS]);
  assert.equal(listed.includes("version"), false);
  assert.equal(listed.includes("smoke"), false);
});

test("each subcommand loads its own module and gets the rest of argv", async () => {
  for (const name of COMMANDS) {
    const calls: { argv: readonly string[]; env: Env; options?: RunOptions }[] =
      [];
    const captured = captureIo();
    const env: Env = { GH_TOKEN: "t" };
    const options: RunOptions = { retryDelayMs: 0 };
    const { load, asked } = fakeLoader(
      async (argv, io, passedEnv, passedOptions) => {
        assert.equal(io, captured.io);
        calls.push({ argv, env: passedEnv, options: passedOptions });
        return 7;
      },
    );
    const code = await dispatch(
      [name, "--repo", "r", "--flag=v"],
      captured.io,
      env,
      options,
      load,
    );
    assert.equal(code, 7, name);
    assert.deepEqual(asked, [name]);
    assert.equal(calls.length, 1);
    assert.deepEqual(calls[0]?.argv, ["--repo", "r", "--flag=v"]);
    assert.equal(calls[0]?.env, env);
    assert.equal(calls[0]?.options, options);
    assert.equal(captured.stderr(), "");
  }
});

test("a command's exit code is returned unchanged", async () => {
  for (const code of [0, 1, 2]) {
    const result = await dispatchWith(["verify"], async () => code);
    assert.equal(result.code, code);
  }
});

test("a command that throws is reported with its name and exits 1", async () => {
  const result = await dispatchWith(["verify"], async () => {
    throw new Error("boom");
  });
  assert.equal(result.code, 1);
  assert.equal(result.stdout, "");
  assert.equal(result.stderr, "release: verify: boom\n");
});

test("importing cli.ts runs nothing", () => {
  assert.equal(process.exitCode, undefined);
});

function runCli(...args: string[]): {
  status: number | null;
  stdout: string;
  stderr: string;
} {
  const result = spawnSync(process.execPath, [CLI, ...args], {
    encoding: "utf8",
  });
  return {
    status: result.status,
    stdout: result.stdout,
    stderr: result.stderr,
  };
}

// The process seam: `run` alone cannot prove the entry point, type stripping,
// or the exit code the shell sees.
test("the entry point exits 2 with the usage on no argument", () => {
  assert.deepEqual(runCli(), { status: 2, stdout: "", stderr: `${USAGE}\n` });
});

test("the entry point exits 2 with the usage on an unknown subcommand", () => {
  assert.deepEqual(runCli("smoke", "--repo", "muehz/agentics"), {
    status: 2,
    stdout: "",
    stderr: `${USAGE}\n`,
  });
});

test("asset-roster emits the stage workflow's derivable assets in comparison order", () => {
  // Keep the expected release roster independent of its production projection.
  const hookBinaryPaths = [
    "hooks/bin/agx-darwin-amd64",
    "hooks/bin/agx-darwin-arm64",
    "hooks/bin/agx-linux-amd64",
    "hooks/bin/agx-linux-arm64",
    "hooks/bin/agx-windows-amd64.exe",
    "hooks/bin/agx-windows-arm64.exe",
  ];
  assert.deepEqual(runCli("asset-roster", "--tag", "v9.8.7"), {
    status: 0,
    stdout: `${JSON.stringify({
      hookBinaryPaths,
      derivableAssetNames: [
        "agentics-v9.8.7.tar.gz",
        "agentics-plugin-v9.8.7.tar.gz",
        ...hookBinaryPaths.map((name) => name.slice("hooks/bin/".length)),
        "THIRD_PARTY_NOTICES.md",
      ],
    })}\n`,
    stderr: "",
  });
});

test("asset-roster rejects a missing or invalid tag before emitting data", () => {
  assert.deepEqual(runCli("asset-roster"), {
    status: 2,
    stdout: "",
    stderr: "release: asset-roster: --tag is required\n",
  });
  const invalid = "v9.8.7-extra";
  assert.deepEqual(runCli("asset-roster", "--tag", invalid), {
    status: 2,
    stdout: "",
    stderr: `release: asset-roster: --tag must be vMAJOR.MINOR.PATCH, not ${invalid}\n`,
  });
});
