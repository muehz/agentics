import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import {
  copyFileSync,
  existsSync,
  lstatSync,
  mkdirSync,
  mkdtempSync,
  readdirSync,
  readFileSync,
  rmSync,
  symlinkSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { after, test, type TestContext } from "node:test";
import { fileURLToPath, pathToFileURL } from "node:url";
import {
  writeConformanceLauncher,
  writeLauncher,
} from "../entrypoint/testing/harness.ts";
import {
  defaultSpawn,
  install,
  main,
  run,
  verifierPath,
  type Spawn,
  type ToolCall,
} from "./ci-conformance.ts";

const moduleFile = fileURLToPath(
  new URL("./ci-conformance.ts", import.meta.url),
);
const auditorCli = ".github/tools/src/conformance/cli.ts";
const windows = process.platform === "win32";

// Fakes shared by every case. Each records its argv NUL-terminated, because an argument can hold a newline.
const shared = mkdtempSync(join(tmpdir(), "agentics-ci-conformance-"));
after(() => rmSync(shared, { recursive: true, force: true }));

const fakeNpm = join(shared, "npm.mjs");
writeFileSync(
  fakeNpm,
  `import { mkdirSync, readdirSync, writeFileSync } from "node:fs";
import { join } from "node:path";
const log = process.env.NPM_LOG, args = process.argv.slice(2);
mkdirSync(log, { recursive: true });
writeFileSync(join(log, String(readdirSync(log).length)), args.map((a) => a + "\\0").join(""));
if (args[0] === "--version") process.stdout.write(process.env.NPM_VERSION + "\\n");
`,
);

// Builds nothing: it records the call, then fails, writes nothing, or puts a Node executable at the -o path that the
// verifier preload turns into the fake verifier.
const fakeGo = join(shared, "go.mjs");
writeFileSync(
  fakeGo,
  `import { linkSync, symlinkSync, writeFileSync } from "node:fs";
const args = process.argv.slice(2);
writeFileSync(process.env.GO_ARG_LOG, args.map((a) => a + "\\0").join(""));
const mode = process.env.FAKE_GO_MODE || "ok";
if (mode === "failed") { process.stderr.write("trusted-build-failed\\n"); process.exit(7); }
const out = args[args.indexOf("-o") + 1];
if (mode === "ok" && process.env.VERIFIER_SOURCE) {
  if (process.platform === "win32") linkSync(process.env.VERIFIER_SOURCE, out);
  else symlinkSync(process.env.VERIFIER_SOURCE, out);
}
`,
);

// Stands in for the auditor's cli.ts. It has no import statement and no TypeScript syntax, so Node runs it either way.
const recordingCli = `const { writeFileSync } = process.getBuiltinModule("node:fs");
const record = (file, values) => { if (file) writeFileSync(file, values.map((v) => v + "\\0").join("")); };
record(process.env.NODE_ARG_LOG, process.argv.slice(2));
record(process.env.NODE_ENV_LOG, [process.env.AGX_CONFORMANCE_AUDITOR ?? "", process.env.AGX_HOOK_VERIFIER ?? ""]);
process.exitCode = Number(process.env.FAKE_CLI_EXIT || "0");
`;

// The real hooks domain starts the verifier directly, so the fake verifier must be an executable on every platform:
// a Node executable named hooktool. This preload, loaded by every Node in the chain through NODE_OPTIONS, acts only in
// the process started under that name, and exits before Node looks for the "audit" script.
const verifierPreload = join(shared, "verifier-preload.mjs");
writeFileSync(
  verifierPreload,
  `import { writeSync } from "node:fs";
import path from "node:path";
if (path.basename(process.argv0).replace(/\\.exe$/i, "") === "hooktool") {
  writeSync(2, process.env.FAKE_VERIFIER_NOTICE ?? "");
  writeSync(1, process.env.FAKE_VERIFIER_OUTPUT ?? "");
  process.exit(Number(process.env.FAKE_VERIFIER_EXIT || "0"));
}
`,
);

// Windows cannot symlink without a privilege, so the fake go hard-links a test-owned copy of this Node instead.
let nodeCopy: string | undefined;
function verifierSource(): string {
  if (!windows) return process.execPath;
  if (nodeCopy === undefined) {
    nodeCopy = join(shared, "node-copy.exe");
    copyFileSync(process.execPath, nodeCopy);
  }
  return nodeCopy;
}

const emptyAudit =
  '{"version":1,"areas":[{"name":"ascii","failures":[],"error":null},{"name":"schema","failures":[],"error":null},{"name":"module","failures":[],"error":null},{"name":"freshness","failures":[],"error":null}]}';

function temporary(t: TestContext): string {
  const dir = mkdtempSync(join(tmpdir(), "agentics-ci-conformance-case-"));
  t.after(() => rmSync(dir, { recursive: true, force: true }));
  return dir;
}

function write(path: string, body: string, mode = 0o644): void {
  mkdirSync(dirname(path), { recursive: true });
  writeFileSync(path, body, { mode });
}

// The host environment without PATH (any spelling) and without variables the step or its children read, so a case
// sets exactly what it needs. Windows children still need SystemRoot and ComSpec to start.
function baseEnv(path: string): NodeJS.ProcessEnv {
  const env: NodeJS.ProcessEnv = {};
  for (const [key, value] of Object.entries(process.env)) {
    const upper = key.toUpperCase();
    if (
      upper === "PATH" ||
      upper === "NODE_OPTIONS" ||
      upper === "RUNNER_TEMP" ||
      upper === "CI" ||
      upper.startsWith("AGX_") ||
      upper.startsWith("GITHUB_")
    )
      continue;
    env[key] = value;
  }
  env.PATH = path;
  return env;
}

function readArgs(file: string): string[] {
  const text = readFileSync(file, "utf8");
  assert.ok(
    text.endsWith("\0"),
    `argv is not NUL-terminated: ${JSON.stringify(text)}`,
  );
  return text.slice(0, -1).split("\0");
}

function npmCalls(log: string): string[][] {
  if (!existsSync(log)) return [];
  return readdirSync(log)
    .sort((a, b) => Number(a) - Number(b))
    .map((name) => readArgs(join(log, name)));
}

function absent(file: string): boolean {
  return lstatSync(file, { throwIfNoEntry: false }) === undefined;
}

interface Recorder {
  calls: ToolCall[];
  output: string;
  spawn: Spawn;
}

// Starts every tool as this Node: node with the call's own arguments, npm and go with their fake script first. No
// .cmd is involved, so hostile arguments reach the fakes on every platform. Child output lands in `output`.
function recorder(cwd?: string): Recorder {
  const calls: ToolCall[] = [];
  const r: Recorder = {
    calls,
    output: "",
    spawn: () => ({ status: 0, stdout: "" }),
  };
  r.spawn = (call) => {
    calls.push(call);
    const script =
      call.tool === "npm" ? [fakeNpm] : call.tool === "go" ? [fakeGo] : [];
    const result = spawnSync(process.execPath, [...script, ...call.args], {
      cwd,
      env: call.env,
      encoding: "utf8",
    });
    assert.ifError(result.error);
    if (!call.capture) r.output += result.stdout;
    r.output += result.stderr;
    return {
      status: result.status ?? 1,
      stdout: call.capture ? result.stdout : "",
    };
  };
  return r;
}

function reporter() {
  const lines: string[] = [];
  return { lines, report: (line: string) => void lines.push(line) };
}

test("install gates on the domain and the npm version", async (t) => {
  const ci = (auditor: string) => [
    "ci",
    "--ignore-scripts",
    "--prefix",
    auditor,
  ];
  const global = ["install", "--global", "--ignore-scripts", "npm@12.2.2"];
  const cases = [
    { domain: "all", version: "11.0.0", global: true },
    { domain: "all", version: "12.2.2", global: false },
    { domain: "all", version: "12.10.0", global: false },
    { domain: "all", version: "12.2.0", global: true },
    { domain: "all", version: "12.2.1", global: true },
    { domain: "all", version: "12.1.9", global: true },
    { domain: "all", version: "13.0.0", global: false },
    // Only the leading version is read, and in decimal.
    { domain: "all", version: "012.002.002-pre", global: false },
    { domain: "skills", version: "11.0.0", global: true },
  ];
  for (const c of cases)
    await t.test(`${c.domain}-${c.version}`, (t) => {
      const dir = temporary(t),
        auditor = join(dir, "auditor"),
        log = join(dir, "npm-calls"),
        r = recorder();
      mkdirSync(auditor);
      const status = install({
        env: {
          ...baseEnv(""),
          AGX_CONFORMANCE_AUDITOR: auditor,
          AGX_CONFORMANCE_DOMAIN: c.domain,
          NPM_LOG: log,
          NPM_VERSION: c.version,
        },
        spawn: r.spawn,
      });
      assert.equal(status, 0, r.output);
      assert.deepEqual(
        npmCalls(log),
        c.global
          ? [["--version"], global, ci(auditor)]
          : [["--version"], ci(auditor)],
      );
    });
});

test("install makes no npm call outside all and skills or with node_modules present", async (t) => {
  for (const name of ["docs", "directory"])
    await t.test(name, (t) => {
      const dir = temporary(t),
        auditor = join(dir, "auditor"),
        modules = join(auditor, "node_modules");
      mkdirSync(auditor);
      if (name === "directory") mkdirSync(modules);
      const calls: ToolCall[] = [];
      const status = install({
        env: {
          ...baseEnv(""),
          AGX_CONFORMANCE_AUDITOR: auditor,
          AGX_CONFORMANCE_DOMAIN: name === "docs" ? "docs" : "all",
        },
        spawn: (call) => {
          calls.push(call);
          return { status: 0, stdout: "11.0.0\n" };
        },
      });
      assert.equal(status, 0);
      assert.deepEqual(calls, []);
    });
});

// A shell `-e` test follows the link, so a dangling node_modules counts as no install and npm ci runs.
test("install follows a dangling node_modules link and installs", (t) => {
  const dir = temporary(t),
    auditor = join(dir, "auditor"),
    modules = join(auditor, "node_modules");
  mkdirSync(auditor);
  // A junction needs no privilege on Windows and dangles the same way.
  symlinkSync(join(dir, "missing"), modules, windows ? "junction" : "dir");
  assert.ok(!existsSync(modules));
  assert.ok(!absent(modules));
  const calls: string[][] = [];
  const status = install({
    env: {
      ...baseEnv(""),
      AGX_CONFORMANCE_AUDITOR: auditor,
      AGX_CONFORMANCE_DOMAIN: "all",
    },
    spawn: (call) => {
      calls.push(call.args);
      return { status: 0, stdout: "12.2.2\n" };
    },
  });
  assert.equal(status, 0);
  assert.deepEqual(calls, [
    ["--version"],
    ["ci", "--ignore-scripts", "--prefix", auditor],
  ]);
});

// An empty auditor would resolve every auditor path against the working directory, which is the audited workspace.
test("install and run refuse an empty or unset auditor", async (t) => {
  for (const [step, name] of [
    ["install", "empty"],
    ["install", "unset"],
    ["run", "empty"],
    ["run", "unset"],
  ] as const)
    await t.test(`${step}-${name}`, (t) => {
      const root = temporary(t),
        calls: ToolCall[] = [],
        { lines, report } = reporter();
      const env = runEnv(root, {
        AGX_CONFORMANCE_DOMAIN: "all",
        AGX_CONFORMANCE_AUDITOR: name === "empty" ? "" : undefined,
      });
      const status = (step === "install" ? install : run)({
        env,
        spawn: (call) => {
          calls.push(call);
          return { status: 0, stdout: "12.2.2\n" };
        },
        report,
      });
      assert.equal(status, 1);
      assert.deepEqual(lines, [
        "conformance action: AGX_CONFORMANCE_AUDITOR is empty or unset",
      ]);
      assert.deepEqual(calls, []);
      assert.ok(absent(join(root, "output")));
    });
});

test("install refuses an unexpected npm version after one call", (t) => {
  const dir = temporary(t),
    auditor = join(dir, "auditor"),
    log = join(dir, "npm-calls"),
    r = recorder(),
    { lines, report } = reporter();
  mkdirSync(auditor);
  const status = install({
    env: {
      ...baseEnv(""),
      AGX_CONFORMANCE_AUDITOR: auditor,
      AGX_CONFORMANCE_DOMAIN: "all",
      NPM_LOG: log,
      NPM_VERSION: "garbage",
    },
    spawn: r.spawn,
    report,
  });
  assert.equal(status, 1);
  assert.deepEqual(lines, [
    "conformance action: unexpected npm version: garbage",
  ]);
  assert.deepEqual(npmCalls(log), [["--version"]]);
});

test("install stops at the first failing npm call", (t) => {
  const calls: string[][] = [];
  const status = install({
    env: {
      ...baseEnv(""),
      AGX_CONFORMANCE_AUDITOR: join(temporary(t), "auditor"),
      AGX_CONFORMANCE_DOMAIN: "all",
    },
    spawn: (call) => {
      calls.push(call.args);
      return call.args[0] === "--version"
        ? { status: 0, stdout: "11.0.0\n" }
        : { status: 5, stdout: "" };
    },
  });
  assert.equal(status, 5);
  assert.deepEqual(calls, [
    ["--version"],
    ["install", "--global", "--ignore-scripts", "npm@12.2.2"],
  ]);
});

// The install step runs the module before any npm ci, as the action does: npm is found on PATH, and on Windows the
// fake is npm.cmd, found through PATHEXT and started through cmd.exe.
test("install finds npm on PATH and starts it", (t) => {
  const dir = temporary(t),
    bin = join(dir, "bin"),
    auditor = join(dir, "auditor"),
    log = join(dir, "npm-calls");
  mkdirSync(auditor);
  const launcher = writeLauncher(bin, "npm", fakeNpm);
  assert.equal(launcher.endsWith(".cmd"), windows);
  const r = spawnSync(process.execPath, [moduleFile, "install"], {
    env: {
      ...baseEnv(bin),
      ...(windows ? { PATHEXT: ".COM;.EXE;.BAT;.CMD" } : {}),
      NPM_LOG: log,
      NPM_VERSION: "11.0.0",
      AGX_CONFORMANCE_AUDITOR: auditor,
      AGX_CONFORMANCE_DOMAIN: "skills",
    },
    encoding: "utf8",
  });
  assert.equal(r.status, 0, r.stdout + r.stderr);
  assert.deepEqual(npmCalls(log), [
    ["--version"],
    ["install", "--global", "--ignore-scripts", "npm@12.2.2"],
    ["ci", "--ignore-scripts", "--prefix", auditor],
  ]);
});

test("the default spawn refuses an argument cmd.exe cannot quote", (t) => {
  const bin = temporary(t),
    { lines, report } = reporter();
  writeFileSync(join(bin, "npm.cmd"), "@exit /b 0\r\n");
  const result = defaultSpawn(
    {
      tool: "npm",
      args: ["ci", "--prefix", 'a"b'],
      env: { PATH: bin, PATHEXT: ".CMD" },
      capture: false,
    },
    { platform: "win32", report },
  );
  assert.equal(result.status, 1);
  assert.equal(lines.length, 1);
  assert.match(
    lines[0]!,
    /^conformance action: cannot pass "a\\"b" through cmd\.exe/,
  );
});

test("the default spawn reports a tool missing from PATH", (t) => {
  const { lines, report } = reporter();
  const result = defaultSpawn(
    {
      tool: "go",
      args: ["version"],
      env: { PATH: temporary(t) },
      capture: false,
    },
    { platform: process.platform, report },
  );
  assert.deepEqual(result, { status: 127, stdout: "" });
  assert.deepEqual(lines, ["conformance action: go: command not found"]);
});

function runEnv(root: string, values: NodeJS.ProcessEnv): NodeJS.ProcessEnv {
  return {
    ...baseEnv(""),
    RUNNER_TEMP: root,
    GITHUB_OUTPUT: join(root, "output"),
    AGX_CONFORMANCE_DOMAIN: "docs",
    AGX_CONFORMANCE_REPO: join(root, "repo"),
    AGX_CONFORMANCE_AUDITOR: join(root, "auditor"),
    AGX_ALLOW_STALE_REVIEW: "false",
    AGX_SKIP_MODULE_TESTS: "false",
    ...values,
  };
}

test("run validates its inputs before anything starts", async (t) => {
  for (const [name, values, message] of [
    [
      "allow-invalid",
      { AGX_ALLOW_STALE_REVIEW: "invalid" },
      "invalid allow-stale-review input",
    ],
    [
      "allow-empty",
      { AGX_ALLOW_STALE_REVIEW: "" },
      "invalid allow-stale-review input",
    ],
    [
      "allow-unset",
      { AGX_ALLOW_STALE_REVIEW: undefined },
      "invalid allow-stale-review input",
    ],
    [
      "skip-yes",
      { AGX_SKIP_MODULE_TESTS: "yes" },
      "invalid skip-module-tests input",
    ],
  ] as const)
    await t.test(name, (t) => {
      const root = temporary(t),
        calls: ToolCall[] = [],
        { lines, report } = reporter();
      const status = run({
        env: runEnv(root, values),
        spawn: (call) => {
          calls.push(call);
          return { status: 0, stdout: "" };
        },
        report,
      });
      assert.equal(status, 1);
      assert.deepEqual(lines, [message]);
      assert.deepEqual(calls, [], "go or the CLI ran");
      assert.ok(absent(join(root, "output")));
    });
});

test("run builds the verifier from the auditor, then starts its CLI", async (t) => {
  for (const [skip, allow, extra] of [
    ["false", "false", []],
    ["true", "false", ["--fast"]],
    ["", "false", []],
    [undefined, "false", []],
    ["false", "true", ["--stale-review-exit-code"]],
    ["true", "true", ["--stale-review-exit-code", "--fast"]],
  ] as const)
    await t.test(`skip=${skip ?? "unset"} allow=${allow}`, (t) => {
      const root = temporary(t),
        calls: ToolCall[] = [],
        env = runEnv(root, {
          AGX_SKIP_MODULE_TESTS: skip,
          AGX_ALLOW_STALE_REVIEW: allow,
        });
      const status = run({
        env,
        spawn: (call) => {
          calls.push(call);
          return { status: 0, stdout: "" };
        },
      });
      assert.equal(status, 0);
      const auditor = env.AGX_CONFORMANCE_AUDITOR!,
        verifier = verifierPath(root, process.platform);
      assert.deepEqual(
        calls.map((c) => [c.tool, c.args, c.capture]),
        [
          [
            "go",
            [
              "build",
              "-C",
              join(auditor, "hooks/go"),
              "-o",
              verifier,
              "./cmd/hooktool",
            ],
            false,
          ],
          [
            "node",
            [
              join(auditor, auditorCli),
              "docs",
              "--repo",
              env.AGX_CONFORMANCE_REPO,
              ...extra,
            ],
            false,
          ],
        ],
      );
      assert.equal(calls[0]!.env, env);
      assert.deepEqual(calls[1]!.env, {
        ...env,
        AGX_HOOK_VERIFIER: verifier,
      });
      assert.equal(
        readFileSync(join(root, "output"), "utf8"),
        "stale-review=false\n",
      );
    });
});

test("the verifier path carries the Windows extension", () => {
  assert.equal(verifierPath("rt", "linux"), join("rt", "hooktool"));
  assert.equal(verifierPath("rt", "win32"), join("rt", "hooktool.exe"));
});

test("a failed build stops before the CLI", (t) => {
  const root = temporary(t),
    r = recorder();
  const status = run({
    env: runEnv(root, {
      GO_ARG_LOG: join(root, "go-args"),
      FAKE_GO_MODE: "failed",
    }),
    spawn: r.spawn,
  });
  assert.equal(status, 7);
  assert.ok(r.output.includes("trusted-build-failed"), r.output);
  assert.deepEqual(
    r.calls.map((c) => c.tool),
    ["go"],
  );
  assert.ok(absent(join(root, "output")));
});

// oracle: TestConformanceActionStaleStatus
test("run maps the CLI status to the step status and output", async (t) => {
  for (const [code, allow, exit, output] of [
    [0, "true", 0, "stale-review=false\n"],
    [3, "true", 0, "stale-review=true\n"],
    [1, "true", 1, ""],
    [2, "true", 2, ""],
    [3, "false", 3, ""],
    [0, "invalid", 1, ""],
  ] as const)
    await t.test(`${code}-${allow}`, (t) => {
      const root = temporary(t),
        auditor = join(root, "auditor"),
        outputPath = join(root, "output"),
        r = recorder();
      write(join(auditor, auditorCli), recordingCli);
      writeFileSync(outputPath, "");
      const status = run({
        env: runEnv(root, {
          AGX_ALLOW_STALE_REVIEW: allow,
          GO_ARG_LOG: join(root, "go-args"),
          NODE_ARG_LOG: join(root, "node-args"),
          FAKE_CLI_EXIT: String(code),
        }),
        spawn: r.spawn,
        report: reporter().report,
      });
      assert.equal(status, exit, r.output);
      assert.equal(readFileSync(outputPath, "utf8"), output);
      if (allow === "invalid") assert.ok(absent(join(root, "node-args")));
      else
        assert.deepEqual(readArgs(join(root, "node-args")), [
          "docs",
          "--repo",
          join(root, "repo"),
          ...(allow === "true" ? ["--stale-review-exit-code"] : []),
        ]);
    });
});

// oracle: TestConformanceActionLiteralInputs
test("hostile paths and domains reach argv literally", async (t) => {
  // Windows forbids a quote and a line break in a file name, so its hostile path keeps the rest.
  const hostile = windows
    ? "space ' $(touch cq38-canary) & %PATH% !x"
    : "space ' \"\n$(touch cq38-canary)";
  for (const [name, domain, real] of [
    ["valid", "docs", false],
    ["install", "all", false],
    ["invalid-literal-domain", "unknown ' \"\n$(touch cq38-canary)", true],
  ] as const)
    await t.test(name, (t) => {
      const dir = temporary(t),
        runner = join(dir, "runner"),
        repo = join(dir, hostile + " repo"),
        auditor = join(dir, hostile + " auditor"),
        cli = join(auditor, auditorCli);
      mkdirSync(runner);
      write(join(repo, ".claude-plugin/plugin.json"), '{"name":"agentics"}');
      if (real) {
        mkdirSync(dirname(cli), { recursive: true });
        writeConformanceLauncher(cli);
      } else write(cli, recordingCli);
      const env = {
        ...baseEnv(""),
        RUNNER_TEMP: runner,
        GITHUB_OUTPUT: join(dir, "outputs"),
        GO_ARG_LOG: join(dir, "go-args"),
        NODE_ARG_LOG: join(dir, "node-args"),
        NODE_ENV_LOG: join(dir, "node-env"),
        NPM_LOG: join(dir, "npm-calls"),
        NPM_VERSION: "11.0.0",
        AGX_CONFORMANCE_AUDITOR: auditor,
        AGX_CONFORMANCE_REPO: repo,
        AGX_CONFORMANCE_DOMAIN: domain,
        AGX_ALLOW_STALE_REVIEW: "false",
        AGX_SKIP_MODULE_TESTS: "false",
      };
      const r = recorder(dir);
      assert.equal(install({ env, spawn: r.spawn }), 0, r.output);
      assert.deepEqual(
        npmCalls(env.NPM_LOG),
        domain === "all"
          ? [
              ["--version"],
              ["install", "--global", "--ignore-scripts", "npm@12.2.2"],
              ["ci", "--ignore-scripts", "--prefix", auditor],
            ]
          : [],
      );
      const status = run({ env, spawn: r.spawn });
      assert.deepEqual(readArgs(env.GO_ARG_LOG), [
        "build",
        "-C",
        join(auditor, "hooks/go"),
        "-o",
        verifierPath(runner, process.platform),
        "./cmd/hooktool",
      ]);
      const node = r.calls.filter((c) => c.tool === "node");
      assert.equal(node.length, 1);
      assert.deepEqual(node[0]!.args, [cli, domain, "--repo", repo]);
      if (real) {
        assert.equal(status, 1, r.output);
        assert.ok(r.output.includes("conformance: unknown domain"), r.output);
      } else {
        assert.equal(status, 0, r.output);
        assert.deepEqual(readArgs(env.NODE_ARG_LOG), [domain, "--repo", repo]);
        assert.deepEqual(readArgs(env.NODE_ENV_LOG), [
          auditor,
          verifierPath(runner, process.platform),
        ]);
        assert.equal(
          readFileSync(env.GITHUB_OUTPUT, "utf8"),
          "stale-review=false\n",
        );
      }
      assert.ok(absent(join(dir, "cq38-canary")), "input executed");
    });
});

// The module runs as the action runs it, as its own process: go is the fake on PATH (go.cmd on Windows), the auditor's
// cli.ts is the real suite, and the real hooks domain starts the verifier the fake build linked.
function runModule(
  t: TestContext,
  setup: { goMode?: string; verifier?: Record<string, string> } = {},
) {
  const dir = temporary(t),
    trusted = join(dir, "trusted"),
    candidate = join(dir, "candidate"),
    runner = join(dir, "runner"),
    bin = join(dir, "bin");
  for (const path of [join(trusted, "hooks/go"), runner, bin])
    mkdirSync(path, { recursive: true });
  writeLauncher(bin, "go", fakeGo);
  const cli = join(trusted, auditorCli);
  mkdirSync(dirname(cli), { recursive: true });
  writeConformanceLauncher(cli);
  write(join(candidate, ".claude-plugin/plugin.json"), "{}");
  // A candidate CLI or committed verifier that ran would leave its canary.
  write(
    join(candidate, auditorCli),
    `const { writeFileSync } = process.getBuiltinModule("node:fs");
writeFileSync(process.env.CLI_CANARY, "candidate cli ran");
`,
  );
  write(
    join(candidate, "hooktool"),
    '#!/bin/sh\ntouch "$HOOK_CANARY"\necho forged-candidate >&2\n',
    0o755,
  );
  const env = {
    ...baseEnv(bin),
    ...(windows ? { PATHEXT: ".COM;.EXE;.BAT;.CMD" } : {}),
    NODE_OPTIONS: `--import=${pathToFileURL(verifierPreload).href}`,
    RUNNER_TEMP: runner,
    GITHUB_OUTPUT: join(dir, "output"),
    GO_ARG_LOG: join(dir, "go-args"),
    FAKE_GO_MODE: setup.goMode ?? "ok",
    VERIFIER_SOURCE: verifierSource(),
    CLI_CANARY: join(dir, "cli-canary"),
    HOOK_CANARY: join(dir, "hook-canary"),
    AGX_CONFORMANCE_AUDITOR: trusted,
    AGX_CONFORMANCE_REPO: candidate,
    AGX_CONFORMANCE_DOMAIN: "hooks",
    AGX_ALLOW_STALE_REVIEW: "false",
    AGX_SKIP_MODULE_TESTS: "false",
    ...setup.verifier,
  };
  const r = spawnSync(process.execPath, [moduleFile, "run"], {
    env,
    encoding: "utf8",
    timeout: 60_000,
  });
  assert.ifError(r.error);
  const output = r.stdout + r.stderr;
  assert.ok(!output.includes("forged-candidate"), output);
  for (const canary of [env.CLI_CANARY, env.HOOK_CANARY])
    assert.ok(absent(canary), `${canary} ran`);
  assert.deepEqual(readArgs(env.GO_ARG_LOG), [
    "build",
    "-C",
    join(trusted, "hooks/go"),
    "-o",
    verifierPath(runner, process.platform),
    "./cmd/hooktool",
  ]);
  return { status: r.status, output, outputFile: env.GITHUB_OUTPUT };
}

// oracle: TestConformanceActionHookVerifierWiring
test("the auditor's CLI starts the verifier the run built", (t) => {
  const r = runModule(t, {
    verifier: {
      FAKE_VERIFIER_NOTICE: "action-trusted-notice\n",
      FAKE_VERIFIER_OUTPUT: `${emptyAudit}\n`,
    },
  });
  assert.equal(r.status, 0, r.output);
  for (const text of ["action-trusted-notice", "PASS hooks"])
    assert.ok(r.output.includes(text), r.output);
  assert.equal(readFileSync(r.outputFile, "utf8"), "stale-review=false\n");
});

// oracle: TestConformanceActionHookVerifierRefusal
test("hook verifier refusal and diagnostics", async (t) => {
  for (const [name, goMode, verifier, want] of [
    ["missing", "missing", {}, "hook audit could not start"],
    [
      "incompatible",
      "ok",
      {
        FAKE_VERIFIER_NOTICE: "action-incompatible\n",
        FAKE_VERIFIER_EXIT: "23",
      },
      "action-incompatible",
    ],
  ] as const)
    await t.test(name, (t) => {
      const r = runModule(t, { goMode, verifier });
      assert.equal(r.status, 1, r.output);
      assert.ok(r.output.includes(want), r.output);
      assert.ok(!r.output.includes("PASS hooks"), r.output);
      assert.ok(absent(r.outputFile));
    });
  await t.test("successful_diagnostics", (t) => {
    const r = runModule(t, {
      verifier: {
        FAKE_VERIFIER_NOTICE: "action-success-notice\n",
        FAKE_VERIFIER_OUTPUT: `${emptyAudit}\n`,
      },
    });
    assert.equal(r.status, 0, r.output);
    for (const text of ["action-success-notice", "PASS hooks"])
      assert.ok(r.output.includes(text), r.output);
  });
});

test("an unknown subcommand prints the usage and exits 2", () => {
  for (const argv of [[], ["build"], ["run", "extra"]]) {
    const { lines, report } = reporter();
    assert.equal(main(argv, { report }), 2);
    assert.deepEqual(lines, ["usage: ci-conformance.ts install|run"]);
  }
});

test("the module loads from the auditor path it is given", () => {
  // The action names the module by the auditor's path; nothing in it resolves files from the working directory.
  const source = readFileSync(moduleFile, "utf8");
  assert.doesNotMatch(source, /process\.cwd\(\)/);
  assert.doesNotMatch(source, /import\(/);
  const specifiers = [
    ...source.matchAll(/^import [^;]* from "([^"]+)";$/gms),
  ].map((match) => match[1] ?? "");
  assert.ok(specifiers.length > 0);
  for (const specifier of specifiers)
    assert.ok(
      specifier.startsWith("node:") || specifier === "../plugin/common.ts",
      specifier,
    );
});
