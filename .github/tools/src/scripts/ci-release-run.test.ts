import assert from "node:assert/strict";
import { spawnSync, type SpawnSyncOptions } from "node:child_process";
import {
  existsSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { describe, test } from "node:test";
import { releaseCall, runRelease, type Spawn } from "./ci-release-run.ts";

const CLI = "/co/.github/tools/src/release/cli.ts";

// The environment the action gives the Run step, with every input empty unless a case sets it.
function stepEnv(
  values: Record<string, string | undefined>,
): Record<string, string | undefined> {
  return {
    COMMAND: "",
    CHECKOUT: "/co",
    REPO: "",
    SHA: "",
    TAG: "",
    MAIN_REF: "",
    ASSETS: "",
    NOTES: "",
    BASELINE: "",
    OUT: "",
    PUBLISHED: "",
    PARENT: "",
    AUTHOR: "",
    PRERELEASE: "",
    RUNNER_TEMP: "/rt",
    ...values,
  };
}

// The value flags in the order the command receives them, with the variable each one reads.
const FLAGS: [string, string][] = [
  ["REPO", "--repo"],
  ["SHA", "--sha"],
  ["TAG", "--tag"],
  ["MAIN_REF", "--main-ref"],
  ["ASSETS", "--assets"],
  ["NOTES", "--notes"],
  ["BASELINE", "--baseline"],
  ["OUT", "--out"],
  ["PUBLISHED", "--published"],
  ["PARENT", "--parent"],
  ["AUTHOR", "--author"],
];

const everyFlag = Object.fromEntries(
  FLAGS.map(([name]) => [name, name.toLowerCase()]),
);

describe("the argument builder", () => {
  test("passes every set flag in the order the action passed them", () => {
    assert.deepEqual(
      releaseCall(stepEnv({ ...everyFlag, COMMAND: "bundle" })).args,
      [
        CLI,
        "bundle",
        ...FLAGS.flatMap(([name, flag]) => [flag, name.toLowerCase()]),
      ],
    );
    assert.deepEqual(
      releaseCall(
        stepEnv({ ...everyFlag, COMMAND: "stage", PRERELEASE: "true" }),
      ).args,
      [
        CLI,
        "stage",
        ...FLAGS.filter(([name]) => name !== "MAIN_REF").flatMap(
          ([name, flag]) => [flag, name.toLowerCase()],
        ),
        "--prerelease",
        "true",
      ],
    );
    assert.deepEqual(
      releaseCall(stepEnv({ ...everyFlag, COMMAND: "context" })).args,
      [
        CLI,
        "context",
        ...FLAGS.filter(([name]) => name !== "MAIN_REF").flatMap(
          ([name, flag]) => [flag, name.toLowerCase()],
        ),
        "--checkout",
        "/co",
      ],
    );
  });

  for (const [name, flag] of FLAGS) {
    test(`leaves ${flag} out when ${name} is empty or unset`, () => {
      // bundle is the one command that passes every flag, and its repo fallback needs a set repo to stay out of the way.
      const base = { ...everyFlag, COMMAND: "bundle" };
      const set = releaseCall(stepEnv(base)).args;
      assert.ok(set.includes(flag), `${flag} missing when ${name} is set`);
      const empty = releaseCall(stepEnv({ ...base, [name]: "" })).args;
      const unset = stepEnv(base);
      delete unset[name];
      const absent = releaseCall(unset).args;
      if (name === "REPO") {
        // The bundle fallback fills an empty repo with the checkout instead.
        assert.deepEqual(empty, absent);
        assert.equal(empty[empty.indexOf("--repo") + 1], "/co");
      } else {
        assert.equal(empty.includes(flag), false, `${flag} passed when empty`);
        assert.deepEqual(empty, absent);
      }
    });
  }

  for (const value of ["", "true", "false", "yes"]) {
    test(`stage passes --prerelease ${JSON.stringify(value)} as it is`, () => {
      assert.deepEqual(
        releaseCall(stepEnv({ COMMAND: "stage", PRERELEASE: value })).args,
        [CLI, "stage", "--prerelease", value],
      );
    });
  }

  test("stage passes an empty --prerelease when PRERELEASE is unset", () => {
    const env = stepEnv({ COMMAND: "stage" });
    delete env["PRERELEASE"];
    assert.deepEqual(releaseCall(env).args, [CLI, "stage", "--prerelease", ""]);
  });

  test("only stage takes --prerelease and only three commands take --checkout", () => {
    const want: Record<string, string[]> = {
      bundle: [CLI, "bundle", "--repo", "/co"],
      context: [CLI, "context", "--checkout", "/co"],
      verify: [CLI, "verify"],
      stage: [CLI, "stage", "--prerelease", "yes"],
      "resume-check": [CLI, "resume-check", "--checkout", "/co"],
      "check-commit": [CLI, "check-commit", "--checkout", "/co"],
    };
    for (const [command, args] of Object.entries(want)) {
      assert.deepEqual(
        releaseCall(stepEnv({ COMMAND: command, PRERELEASE: "yes" })).args,
        args,
        command,
      );
    }
  });

  test("bundle falls back to the checkout only when no repo is given", () => {
    const given = releaseCall(
      stepEnv({ COMMAND: "bundle", REPO: "/candidate" }),
    );
    assert.deepEqual(given.args, [CLI, "bundle", "--repo", "/candidate"]);
    const unset = stepEnv({ COMMAND: "bundle" });
    delete unset["REPO"];
    assert.deepEqual(releaseCall(unset).args, [CLI, "bundle", "--repo", "/co"]);
    // The fallback is bundle's alone: an API command without a repo gets no --repo.
    assert.deepEqual(releaseCall(stepEnv({ COMMAND: "context" })).args, [
      CLI,
      "context",
      "--checkout",
      "/co",
    ]);
  });

  test("only bundle passes --main-ref", () => {
    assert.deepEqual(
      releaseCall(stepEnv({ COMMAND: "bundle", REPO: "r", MAIN_REF: "HEAD" }))
        .args,
      [CLI, "bundle", "--repo", "r", "--main-ref", "HEAD"],
    );
    for (const command of [
      "context",
      "verify",
      "stage",
      "resume-check",
      "check-commit",
    ]) {
      const args = releaseCall(
        stepEnv({ COMMAND: command, MAIN_REF: "refs/remotes/origin/main" }),
      ).args;
      assert.equal(args.includes("--main-ref"), false, command);
      assert.equal(args.includes("refs/remotes/origin/main"), false, command);
    }
  });

  test("bundle runs the verifier the action built, and every other command keeps the caller's", () => {
    const caller = { AGX_HOOK_VERIFIER: "/candidate/hooktool" };
    assert.equal(
      releaseCall(stepEnv({ ...caller, COMMAND: "bundle" })).env[
        "AGX_HOOK_VERIFIER"
      ],
      "/rt/hooktool",
    );
    const unsetRunner = stepEnv({ COMMAND: "bundle" });
    delete unsetRunner["RUNNER_TEMP"];
    assert.equal(
      releaseCall(unsetRunner).env["AGX_HOOK_VERIFIER"],
      "/hooktool",
    );
    for (const command of [
      "context",
      "verify",
      "stage",
      "resume-check",
      "check-commit",
    ]) {
      assert.equal(
        releaseCall(stepEnv({ ...caller, COMMAND: command })).env[
          "AGX_HOOK_VERIFIER"
        ],
        "/candidate/hooktool",
        command,
      );
      const env = releaseCall(stepEnv({ COMMAND: command })).env;
      assert.equal("AGX_HOOK_VERIFIER" in env, false, command);
    }
  });

  test("passes an unknown command and its flags through for cli.ts to reject", () => {
    assert.deepEqual(
      releaseCall(stepEnv({ COMMAND: "set-version", SHA: "abc" })).args,
      [CLI, "set-version", "--sha", "abc"],
    );
    const unset = stepEnv({});
    delete unset["COMMAND"];
    assert.deepEqual(releaseCall(unset).args, [CLI, ""]);
  });

  test("finds cli.ts under the checkout it is given", () => {
    assert.equal(
      releaseCall(stepEnv({ CHECKOUT: "/elsewhere", COMMAND: "verify" }))
        .args[0],
      "/elsewhere/.github/tools/src/release/cli.ts",
    );
  });

  test("passes the whole environment on to the command", () => {
    const env = stepEnv({
      COMMAND: "stage",
      GH_TOKEN: "token",
      GITHUB_OUTPUT: "/out",
      PATH: "/bin",
    });
    const call = releaseCall(env);
    assert.deepEqual(call.env, env);
    assert.notEqual(call.env, env, "the caller's environment was modified");
  });
});

interface Recorded {
  command: string;
  args: readonly string[];
  options: SpawnSyncOptions;
}

function recordingSpawn(result: ReturnType<Spawn>): {
  spawn: Spawn;
  calls: Recorded[];
} {
  const calls: Recorded[] = [];
  return {
    calls,
    spawn: (command, args, options) => {
      calls.push({ command, args, options });
      return result;
    },
  };
}

describe("the runner", () => {
  test("starts node by name with the arguments as an array, no shell, and inherited stdio", () => {
    const { spawn, calls } = recordingSpawn({ status: 0, signal: null });
    const env = stepEnv({
      COMMAND: "bundle",
      GH_TOKEN: "token",
      GITHUB_OUTPUT: "/out",
    });
    assert.equal(
      runRelease(env, spawn, () => {}),
      0,
    );
    assert.equal(calls.length, 1);
    const [call] = calls;
    assert.equal(call?.command, "node");
    assert.deepEqual(call?.args, [CLI, "bundle", "--repo", "/co"]);
    assert.equal(call?.options.shell, undefined);
    assert.equal(call?.options.stdio, "inherit");
    assert.deepEqual(call?.options.env, {
      ...env,
      AGX_HOOK_VERIFIER: "/rt/hooktool",
    });
  });

  for (const status of [0, 1, 2]) {
    test(`returns the command's exit status ${status}`, () => {
      const { spawn } = recordingSpawn({ status, signal: null });
      assert.equal(
        runRelease(stepEnv({ COMMAND: "verify" }), spawn, () => {}),
        status,
      );
    });
  }

  test("fails when the command is killed by a signal", () => {
    const lines: string[] = [];
    const { spawn } = recordingSpawn({ status: null, signal: "SIGTERM" });
    const code = runRelease(stepEnv({ COMMAND: "verify" }), spawn, (line) =>
      lines.push(line),
    );
    assert.notEqual(code, 0);
    assert.match(lines.join("\n"), /SIGTERM/);
  });

  test("fails when node cannot be started", () => {
    const lines: string[] = [];
    const error = Object.assign(new Error("spawnSync node ENOENT"), {
      code: "ENOENT",
    });
    const { spawn } = recordingSpawn({ status: null, signal: null, error });
    const code = runRelease(stepEnv({ COMMAND: "verify" }), spawn, (line) =>
      lines.push(line),
    );
    assert.notEqual(code, 0);
    assert.match(lines.join("\n"), /cannot start node: spawnSync node ENOENT/);
  });
});

// The module run as the action runs it: from its own path, with the workspace as the working directory. Only the cli.ts
// under CHECKOUT may run, never one the workspace or the module's own checkout holds.
test("the module runs cli.ts from CHECKOUT and nowhere else", () => {
  const root = mkdtempSync(join(tmpdir(), "agentics-ci-release-run-"));
  const record = (marker: string, status: number) =>
    `require("node:fs").writeFileSync(${JSON.stringify(marker)}, JSON.stringify(process.argv.slice(2)));\nprocess.exit(${status});\n`;
  const checkout = join(root, "checkout");
  const workspace = join(root, "workspace");
  for (const [dir, marker, status] of [
    [checkout, join(root, "trusted"), 3],
    [workspace, join(root, "forged"), 0],
  ] as const) {
    const cli = join(dir, ".github", "tools", "src", "release");
    mkdirSync(cli, { recursive: true });
    // A CommonJS body under a .ts name: Node strips no types from it and runs it as it is.
    writeFileSync(join(cli, "cli.ts"), record(marker, status));
    writeFileSync(join(dir, "package.json"), '{"type":"commonjs"}\n');
  }
  const result = spawnSync(
    process.execPath,
    [join(import.meta.dirname, "ci-release-run.ts")],
    {
      cwd: workspace,
      encoding: "utf8",
      env: {
        ...Object.fromEntries(
          Object.entries(process.env).filter(
            ([name]) => !(name in stepEnv({})),
          ),
        ),
        CHECKOUT: checkout,
        COMMAND: "verify",
        TAG: "v1.2.3",
      },
    },
  );
  assert.equal(result.status, 3, `${result.stdout}${result.stderr}`);
  assert.equal(existsSync(join(root, "forged")), false);
  assert.deepEqual(JSON.parse(readFileSync(join(root, "trusted"), "utf8")), [
    "verify",
    "--tag",
    "v1.2.3",
  ]);
});
