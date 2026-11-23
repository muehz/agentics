import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import {
  copyFileSync,
  existsSync,
  lstatSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  readlinkSync,
  rmSync,
  symlinkSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { delimiter, dirname, join } from "node:path";
import { test, type TestContext } from "node:test";
import {
  findInterpreter,
  requireInterpreter,
  twinPowerShell,
} from "./interpreters.ts";
import { quotePosix, withFixturePath } from "./testing/shell-support.ts";

// This file imports no package, so it runs in a job that never ran npm ci.
const REPO_ROOT = join(import.meta.dirname, "..", "..", "..", "..");
const SCRIPTS = join(REPO_ROOT, "skills", "diagnose", "references");

type Artifact = "absent" | { content: string } | { link: string };

/**
 * A fake test runner, described the same way for every leg. Each call logs
 * its arguments to `calls.log`, prints `stdout` and then `stderr`, creates
 * `artifact` when `creates` is set, and exits.
 */
interface Runner {
  stdout?: string[];
  stderr?: string[];
  creates?: boolean;
  /** The exit code when no `exitFor` key matches; 0 when omitted. */
  exit?: number;
  /** Exit codes for a test file whose path contains the key. */
  exitFor?: Record<string, number>;
  /**
   * The `.ps1` leg exits from the fake's own PowerShell instead of a native
   * child, as Node's `npm.ps1` does when Ctrl+C ends it. Such a fake only
   * logs and exits.
   */
  inProcess?: boolean;
  /**
   * Every leg runs a native program: a link to Node named for the runner,
   * which runs the script `test` in the working directory. The other fakes
   * are scripts, and PowerShell treats the arguments to a script differently
   * from the arguments to a native command.
   */
  native?: boolean;
}

/**
 * One find-polluter run. Paths in `args` and `files` are relative to the
 * working directory the case runs in, where every runner also runs.
 */
interface Case {
  name: string;
  args: string[];
  /** FIND_POLLUTER_RUNNER */
  runner?: string;
  files?: Record<string, string>;
  /** Symlinks to create, as path to target. */
  links?: Record<string, string>;
  /** Fake runners placed first on PATH, by command name. */
  runners?: Record<string, Runner>;
  /** Bodies of extra commands placed first on PATH, written for the `.sh` leg. */
  shims?: Record<string, string>;
  /** PATH holds the fake runners' directory alone. */
  pathAlone?: boolean;
  /** The inherited environment carries conflicting spellings of PATH. */
  pathAliases?: boolean;
  /** `<script>` stands for the path of the script under test. */
  stdout: string[];
  stderr: string[];
  exit: number;
  /** The runner's arguments, one entry per call. */
  calls: string[];
  artifact: Artifact;
  /** The legs that run the case; every leg when omitted. */
  only?: LegName[];
}

type LegName = ".sh" | ".ps1";

interface Leg {
  name: LegName;
  extension: string;
  /** For the `skip` option of node:test. */
  skip: false | string;
  /** The program and leading arguments that run the script at `script`. */
  command(script: string): [string, string[]];
  /** The selected shell's shipped tools, after the fixture's own shims. */
  toolsPath?: string;
  /** Writes the fake runner `name` into `bin`, the first PATH directory. */
  writeRunner(bin: string, name: string, runner: Runner): void;
}

const createArtifact = "printf new > artifact";

const TEST_ARGS = ["artifact", "./tests/*_test.js"];

function search(pattern: string): string[] {
  return [
    "🔍 Searching for test that creates: artifact",
    `Test pattern: ${pattern}`,
    "",
  ];
}

const SEARCH = search("./tests/*_test.js");

function foundOne(file: string): string[] {
  return [...SEARCH, "Found 1 test files", "", `[1/1] Testing: ${file}`];
}

/** The stdout of a run that tests `files` in order and finds them clean. */
function cleanRun(pattern: string, files: string[]): string[] {
  return [
    ...search(pattern),
    `Found ${files.length} test files`,
    "",
    ...files.map((file, i) => `[${i + 1}/${files.length}] Testing: ${file}`),
    "",
    "✅ No polluter found - all tests clean!",
  ];
}

const USAGE = [
  "Usage: <script> <file_to_check> <test_pattern>",
  "Example: <script> 'packages/core/leaked-state' 'src/**/*.test.ts'",
];

// The runner's own output must stay out of a completed run's report, and a
// native runner that writes to stderr must not stop the PowerShell twin.
const NOISY: Runner = { stdout: ["runner says hi"], stderr: ["runner warns"] };

const CASES: Case[] = [
  {
    name: "clean",
    args: TEST_ARGS,
    files: { "tests/clean_test.js": "" },
    runners: { npm: NOISY },
    stdout: [
      ...foundOne("./tests/clean_test.js"),
      "",
      "✅ No polluter found - all tests clean!",
    ],
    stderr: [],
    exit: 0,
    calls: ["test ./tests/clean_test.js"],
    artifact: "absent",
  },
  {
    name: "dirty",
    args: TEST_ARGS,
    files: { "tests/dirty_test.js": "" },
    runners: { npm: { ...NOISY, creates: true } },
    stdout: [
      ...foundOne("./tests/dirty_test.js"),
      "",
      "🎯 FOUND POLLUTER!",
      "   Test: ./tests/dirty_test.js",
      "   Created: artifact",
      "",
      "Pollution details:",
      "To investigate:",
      '  npm test "./tests/dirty_test.js"    # Run just this test',
      '  cat "./tests/dirty_test.js"         # Review test code',
    ],
    stderr: [],
    exit: 1,
    calls: ["test ./tests/dirty_test.js"],
    artifact: { content: "new" },
  },
  {
    name: "preexisting",
    args: TEST_ARGS,
    files: { artifact: "original", "tests/preexisting_test.js": "" },
    runners: { npm: {} },
    stdout: [],
    stderr: ["Cannot diagnose: pollution artifact already exists: artifact"],
    exit: 2,
    calls: [],
    artifact: { content: "original" },
  },
  {
    name: "dangling",
    args: TEST_ARGS,
    files: { "tests/dangling_test.js": "" },
    links: { artifact: "missing-target" },
    runners: { npm: {} },
    stdout: [],
    stderr: ["Cannot diagnose: pollution artifact already exists: artifact"],
    exit: 2,
    calls: [],
    artifact: { link: "missing-target" },
  },
  {
    name: "empty",
    args: TEST_ARGS,
    runners: { npm: {} },
    stdout: SEARCH,
    stderr: ["Cannot diagnose: no tests match: ./tests/*_test.js"],
    exit: 2,
    calls: [],
    artifact: "absent",
  },
  {
    name: "late",
    args: TEST_ARGS,
    files: { "tests/late_test.js": "" },
    runners: { npm: {} },
    // find stands in for the filesystem changing between the search and the run.
    shims: { find: `${createArtifact}\nprintf './tests/late_test.js\\n'` },
    stdout: [...SEARCH, "Found 1 test files", ""],
    stderr: [
      "Cannot diagnose: pollution artifact appeared before test: ./tests/late_test.js",
    ],
    exit: 2,
    calls: [],
    artifact: { content: "new" },
    only: [".sh"],
  },
  {
    name: "runner that is not npm, pnpm or yarn",
    args: TEST_ARGS,
    runner: "make",
    files: { "tests/a_test.js": "" },
    stdout: [],
    stderr: [
      "Cannot diagnose: FIND_POLLUTER_RUNNER must be npm, pnpm or yarn, not: make",
    ],
    exit: 2,
    calls: [],
    artifact: "absent",
  },
  {
    name: "pnpm runner",
    args: TEST_ARGS,
    runner: "pnpm",
    files: { "tests/p_test.js": "" },
    runners: { pnpm: { creates: true } },
    stdout: [
      ...foundOne("./tests/p_test.js"),
      "",
      "🎯 FOUND POLLUTER!",
      "   Test: ./tests/p_test.js",
      "   Created: artifact",
      "",
      "Pollution details:",
      "To investigate:",
      '  pnpm test "./tests/p_test.js"    # Run just this test',
      '  cat "./tests/p_test.js"         # Review test code',
    ],
    stderr: [],
    exit: 1,
    calls: ["test ./tests/p_test.js"],
    artifact: { content: "new" },
  },
  {
    name: "failing tests make the result not clean",
    args: TEST_ARGS,
    files: { "tests/a_test.js": "", "tests/b_test.js": "" },
    runners: { npm: { exitFor: { b_test: 1 } } },
    stdout: [
      ...SEARCH,
      "Found 2 test files",
      "",
      "[1/2] Testing: ./tests/a_test.js",
      "[2/2] Testing: ./tests/b_test.js",
      "",
    ],
    stderr: [
      "No polluter found, but 1 of 2 test runs exited non-zero:",
      "   ./tests/b_test.js (exit 1)",
      "A test that stops early may never reach the code that creates artifact.",
      "Run one of them directly to see why it failed: npm test <test file>",
    ],
    exit: 3,
    calls: ["test ./tests/a_test.js", "test ./tests/b_test.js"],
    artifact: "absent",
  },
  // PowerShell reads a native runner's two pipes on separate threads, so the
  // order of a tail mixing both streams is not fixed; each tail row uses one.
  {
    name: "runner that failed to run, with stdout",
    args: TEST_ARGS,
    files: { "tests/a_test.js": "" },
    runners: {
      npm: { stdout: ["can't start", "", "after a blank"], exit: 127 },
    },
    pathAliases: true,
    stdout: foundOne("./tests/a_test.js"),
    stderr: [
      "",
      "Cannot diagnose: test runner failed to run (exit 127): npm test ./tests/a_test.js",
      "can't start",
      "",
      "after a blank",
    ],
    exit: 2,
    calls: ["test ./tests/a_test.js"],
    artifact: "absent",
  },
  {
    name: "runner that failed to run, with stderr",
    args: TEST_ARGS,
    files: { "tests/a_test.js": "" },
    runners: { npm: { stderr: ["bang é", "", "after a blank"], exit: 127 } },
    stdout: foundOne("./tests/a_test.js"),
    stderr: [
      "",
      "Cannot diagnose: test runner failed to run (exit 127): npm test ./tests/a_test.js",
      "bang é",
      "",
      "after a blank",
    ],
    exit: 2,
    calls: ["test ./tests/a_test.js"],
    artifact: "absent",
  },
  {
    // A shell prints one empty line for a runner that printed nothing.
    name: "runner that could not execute",
    args: TEST_ARGS,
    files: { "tests/a_test.js": "" },
    runners: { npm: { exit: 126 } },
    stdout: foundOne("./tests/a_test.js"),
    stderr: [
      "",
      "Cannot diagnose: test runner failed to run (exit 126): npm test ./tests/a_test.js",
      "",
    ],
    exit: 2,
    calls: ["test ./tests/a_test.js"],
    artifact: "absent",
  },
  {
    name: "runner that was not found",
    args: TEST_ARGS,
    files: { "tests/a_test.js": "" },
    pathAlone: true,
    stdout: foundOne("./tests/a_test.js"),
    stderr: [
      "",
      "Cannot diagnose: test runner failed to run (exit 127): npm test ./tests/a_test.js",
    ],
    exit: 2,
    calls: [],
    artifact: "absent",
    only: [".ps1"],
  },
  {
    name: "interrupted run",
    args: TEST_ARGS,
    files: { "tests/a_test.js": "" },
    runners: { npm: { exit: 130 } },
    stdout: foundOne("./tests/a_test.js"),
    stderr: [
      "",
      "Cannot diagnose: interrupted (exit 130) during: npm test ./tests/a_test.js",
    ],
    exit: 2,
    calls: ["test ./tests/a_test.js"],
    artifact: "absent",
  },
  {
    name: "run interrupted by Ctrl+C on Windows",
    args: TEST_ARGS,
    files: { "tests/a_test.js": "" },
    runners: { npm: { exit: -1073741510, inProcess: true } },
    stdout: foundOne("./tests/a_test.js"),
    stderr: [
      "",
      "Cannot diagnose: interrupted (exit 130) during: npm test ./tests/a_test.js",
    ],
    exit: 2,
    calls: ["test ./tests/a_test.js"],
    artifact: "absent",
    only: [".ps1"],
  },
  // A PowerShell parameter binder would claim the dashed ones before the
  // count check.
  ...[
    [],
    ["artifact"],
    ["artifact", "./tests/*_test.js", "extra"],
    ["--"],
    ["-h"],
    ["-x"],
    ["-v"],
    ["-ea"],
    ["--", "artifact", "./tests/*_test.js"],
    ["-v", "artifact", "./tests/*_test.js"],
  ].map((args): Case => ({
    name: `${args.length} arguments: ${JSON.stringify(args)}`,
    args,
    files: { "tests/a_test.js": "" },
    runners: { npm: {} },
    stdout: USAGE,
    stderr: [],
    exit: 2,
    calls: [],
    artifact: "absent",
  })),
  {
    name: "**/ matches no directory or several",
    args: ["artifact", "tests/**/a_test.js"],
    files: {
      "tests/a_test.js": "",
      "tests/sub/deeper/a_test.js": "",
      "tests/sub/b_test.js": "",
    },
    runners: { npm: {} },
    stdout: cleanRun("tests/**/a_test.js", [
      "./tests/a_test.js",
      "./tests/sub/deeper/a_test.js",
    ]),
    stderr: [],
    exit: 0,
    calls: ["test ./tests/a_test.js", "test ./tests/sub/deeper/a_test.js"],
    artifact: "absent",
  },
  {
    name: "node_modules is skipped",
    args: ["artifact", "**/*_test.js"],
    files: {
      "node_modules/pkg/a_test.js": "",
      "lib/node_modules/b_test.js": "",
      "tests/c_test.js": "",
    },
    runners: { npm: {} },
    stdout: cleanRun("**/*_test.js", ["./tests/c_test.js"]),
    stderr: [],
    exit: 0,
    calls: ["test ./tests/c_test.js"],
    artifact: "absent",
  },
  {
    name: "files from several directories run sorted",
    args: ["artifact", "**/*_test.js"],
    // Written out of order; the order agrees under a byte and a locale sort.
    files: {
      "tests/c/y_test.js": "",
      "src/z_test.js": "",
      "tests/b_test.js": "",
      "tests/a/x_test.js": "",
    },
    runners: { npm: {} },
    stdout: cleanRun("**/*_test.js", [
      "./src/z_test.js",
      "./tests/a/x_test.js",
      "./tests/b_test.js",
      "./tests/c/y_test.js",
    ]),
    stderr: [],
    exit: 0,
    calls: [
      "test ./src/z_test.js",
      "test ./tests/a/x_test.js",
      "test ./tests/b_test.js",
      "test ./tests/c/y_test.js",
    ],
    artifact: "absent",
  },
  ...(
    [
      {
        rule: "* stays within one path segment",
        pattern: "tests/*_test.js",
        found: ["tests/a_test.js"],
        missed: ["tests/sub/b_test.js"],
      },
      {
        rule: "? matches one character within a segment",
        pattern: "tests?a_test.js",
        found: ["testsXa_test.js"],
        missed: ["tests/a_test.js", "testsXYa_test.js"],
      },
      {
        rule: ". matches itself",
        pattern: "tests/a.test.js",
        found: ["tests/a.test.js"],
        missed: ["tests/aXtest.js"],
      },
      {
        rule: "[ and ] match themselves",
        pattern: "tests/[ab]_test.js",
        found: ["tests/[ab]_test.js"],
        missed: ["tests/a_test.js"],
      },
      {
        rule: "{ and } match themselves",
        pattern: "tests/{a,b}_test.js",
        found: ["tests/{a,b}_test.js"],
        missed: ["tests/a_test.js"],
      },
    ] satisfies {
      rule: string;
      pattern: string;
      found: string[];
      missed: string[];
    }[]
  ).map(({ rule, pattern, found, missed }): Case => ({
    name: `glob: ${rule}`,
    args: ["artifact", pattern],
    files: Object.fromEntries([...found, ...missed].map((path) => [path, ""])),
    runners: { npm: {} },
    stdout: cleanRun(
      pattern,
      found.map((path) => `./${path}`),
    ),
    stderr: [],
    exit: 0,
    calls: found.map((path) => `test ./${path}`),
    artifact: "absent",
  })),
  {
    name: "a link to a directory is not followed",
    args: ["artifact", "**/*_test.js"],
    files: { "real/sub/a_test.js": "" },
    links: { linked: "real/sub" },
    runners: { npm: {} },
    stdout: cleanRun("**/*_test.js", ["./real/sub/a_test.js"]),
    stderr: [],
    exit: 0,
    calls: ["test ./real/sub/a_test.js"],
    artifact: "absent",
  },
  {
    name: "a link to a file is not a test file",
    args: ["artifact", "**/*_test.js"],
    files: { "tests/a_test.js": "" },
    links: { "tests/linked_test.js": "a_test.js" },
    runners: { npm: {} },
    stdout: cleanRun("**/*_test.js", ["./tests/a_test.js"]),
    stderr: [],
    exit: 0,
    calls: ["test ./tests/a_test.js"],
    artifact: "absent",
  },
  {
    // PowerShell on Unix expands wildcards in an unquoted argument to a native
    // command, which would hand the runner ./tests/a_test.js instead.
    name: "a native runner gets a test name with wildcard characters as written",
    args: ["artifact", "tests/[ab]_test.js"],
    files: { "tests/[ab]_test.js": "", "tests/a_test.js": "" },
    runners: { npm: { native: true } },
    stdout: cleanRun("tests/[ab]_test.js", ["./tests/[ab]_test.js"]),
    stderr: [],
    exit: 0,
    calls: ["test ./tests/[ab]_test.js"],
    artifact: "absent",
  },
];

/** The lines of `text` without the trailing newline. */
function lines(text: string): string[] {
  return text === "" ? [] : text.replace(/\n$/, "").split("\n");
}

/**
 * Drops the body of the pollution details block, which holds `ls -la` output
 * on the `.sh` leg and `Format-List` output on the `.ps1` leg: only its
 * heading is part of the shared contract. A missing `To investigate:` line
 * leaves the output whole, so the comparison fails.
 */
function withoutDetailsBody(output: string[]): string[] {
  const heading = output.indexOf("Pollution details:");
  if (heading < 0) return output;
  const end = output.indexOf("To investigate:", heading + 1);
  if (end < 0) return output;
  return [...output.slice(0, heading + 1), ...output.slice(end)];
}

function psQuote(text: string): string {
  return `'${text.replaceAll("'", "''")}'`;
}

function renderShRunner(runner: Runner): string {
  const body = [`printf '%s\\n' "$*" >> calls.log`];
  for (const line of runner.stdout ?? [])
    body.push(`printf '%s\\n' ${quotePosix(line)}`);
  for (const line of runner.stderr ?? [])
    body.push(`printf '%s\\n' ${quotePosix(line)} >&2`);
  if (runner.creates) body.push(createArtifact);
  for (const [key, code] of Object.entries(runner.exitFor ?? {}))
    body.push(`case "$2" in *${key}*) exit ${code} ;; esac`);
  body.push(`exit ${runner.exit ?? 0}`);
  return `#!/bin/sh\n${body.join("\n")}\n`;
}

/**
 * A CommonJS script that acts out `runner` as a native process. Run as the
 * script `test` by a native runner, it logs its own name as the first
 * argument, which is where the runner put it.
 */
function renderNodeRunner(runner: Runner): string {
  return `const { appendFileSync, writeFileSync, writeSync } = require("node:fs");
const { basename } = require("node:path");
const runner = ${JSON.stringify(runner)};
const args = runner.native
  ? [basename(process.argv[1]), ...process.argv.slice(2)]
  : process.argv.slice(2);
appendFileSync("calls.log", args.join(" ") + "\\n");
const stdout = runner.stdout ?? [];
const stderr = runner.stderr ?? [];
for (const line of stdout) writeSync(1, line + "\\n");
for (const line of stderr) writeSync(2, line + "\\n");
if (runner.creates) writeFileSync("artifact", "new");
const file = args[1] ?? "";
const match = Object.entries(runner.exitFor ?? {}).find(([key]) => file.includes(key));
process.exit(match === undefined ? (runner.exit ?? 0) : match[1]);
`;
}

function writePs1Runner(bin: string, name: string, runner: Runner): void {
  let body: string;
  if (runner.inProcess) {
    body = [
      `[System.IO.File]::AppendAllText((Join-Path (Get-Location).ProviderPath 'calls.log'), (($args -join ' ') + "\`n"))`,
      `exit ${runner.exit ?? 0}`,
    ].join("\n");
  } else {
    const script = join(bin, `${name}-fake.cjs`);
    writeFileSync(script, renderNodeRunner(runner));
    // PowerShell on Unix expands wildcards in a splatted or unquoted argument
    // to a native command, so the two arguments the twin passes go on as
    // quoted strings, and any other count fails the case.
    body = [
      "if ($args.Count -ne 2) { exit 99 }",
      `& ${psQuote(process.execPath)} ${psQuote(script)} "$($args[0])" "$($args[1])"`,
      "exit $LASTEXITCODE",
    ].join("\n");
  }
  writeFileSync(join(bin, `${name}.ps1`), `${body}\n`);
}

function runCase(leg: Leg, c: Case, t: TestContext): void {
  const root = mkdtempSync(join(tmpdir(), "agentics-polluter-"));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  const bin = join(root, "bin");
  mkdirSync(bin);
  for (const [name, runner] of Object.entries(c.runners ?? {})) {
    if (!runner.native) {
      leg.writeRunner(bin, name, runner);
      continue;
    }
    writeFileSync(join(root, "test"), renderNodeRunner(runner));
    const program = join(
      bin,
      process.platform === "win32" ? `${name}.exe` : name,
    );
    try {
      symlinkSync(process.execPath, program);
    } catch (error) {
      // Windows creates a symlink only with a privilege most accounts lack.
      if ((error as NodeJS.ErrnoException).code !== "EPERM") throw error;
      copyFileSync(process.execPath, program);
    }
  }
  for (const [name, body] of Object.entries(c.shims ?? {}))
    writeFileSync(join(bin, name), `#!/bin/sh\n${body}\n`, { mode: 0o755 });
  for (const [path, content] of Object.entries(c.files ?? {})) {
    mkdirSync(dirname(join(root, path)), { recursive: true });
    writeFileSync(join(root, path), content);
  }
  for (const [path, target] of Object.entries(c.links ?? {})) {
    try {
      symlinkSync(target, join(root, path));
    } catch (error) {
      // Windows creates a symlink only with a privilege most accounts lack.
      if ((error as NodeJS.ErrnoException).code !== "EPERM") throw error;
      t.skip("the account cannot create symlinks");
      return;
    }
  }
  const script = join(SCRIPTS, `find-polluter.${leg.extension}`);
  const [file, leading] = leg.command(script);
  const inherited = c.pathAliases
    ? { ...process.env, Path: "ignored", pAtH: "also ignored" }
    : process.env;
  const result = spawnSync(file, [...leading, ...c.args], {
    cwd: root,
    encoding: "utf8",
    timeout: 60_000,
    env: withFixturePath(
      {
        ...inherited,
        // Git for Windows' sh expands a wildcard argument that a native process
        // passes it unquoted; a shell would have kept the pattern whole.
        MSYS: "noglob",
        ...(c.runner === undefined ? {} : { FIND_POLLUTER_RUNNER: c.runner }),
      },
      c.pathAlone
        ? bin
        : [
            bin,
            ...(leg.toolsPath ? [leg.toolsPath] : []),
            process.env.PATH ?? "",
          ].join(delimiter),
    ),
  });
  assert.ifError(result.error);
  const output = result.stdout + result.stderr;
  assert.equal(result.status, c.exit, output);
  assert.deepEqual(
    withoutDetailsBody(lines(result.stdout.replaceAll(script, "<script>"))),
    c.stdout,
    output,
  );
  assert.deepEqual(lines(result.stderr), c.stderr, output);

  const log = join(root, "calls.log");
  assert.deepEqual(
    existsSync(log) ? lines(readFileSync(log, "utf8")) : [],
    c.calls,
    output,
  );
  const artifact = join(root, "artifact");
  if (c.artifact === "absent")
    assert.equal(lstatSync(artifact, { throwIfNoEntry: false }), undefined);
  else if ("link" in c.artifact)
    assert.equal(readlinkSync(artifact), c.artifact.link);
  else assert.equal(readFileSync(artifact, "utf8"), c.artifact.content);
}

const shPath = findInterpreter("sh");
// Git for Windows' bin/sh wrapper prepends its own utilities to PATH. Run
// its adjacent shell runtime so the fixture's commands keep precedence.
const gitShell =
  process.platform === "win32" && shPath !== undefined
    ? findInterpreter("sh", {
        env: withFixturePath(
          process.env,
          join(dirname(shPath), "..", "usr", "bin"),
        ),
      })
    : undefined;
const powerShell = twinPowerShell();
const LEGS: Leg[] = [
  {
    name: ".sh",
    extension: "sh",
    skip: requireInterpreter("sh"),
    command: (script) => [gitShell ?? shPath ?? "sh", [script]],
    toolsPath: gitShell === undefined ? undefined : dirname(gitShell),
    writeRunner: (bin, name, runner) =>
      writeFileSync(join(bin, name), renderShRunner(runner), { mode: 0o755 }),
  },
  {
    name: ".ps1",
    extension: "ps1",
    skip: powerShell.skip,
    // An absolute path, so a case whose PATH lacks PowerShell still starts it.
    command: (script) => [
      powerShell.path ?? powerShell.name,
      ["-NoProfile", "-ExecutionPolicy", "Bypass", "-File", script],
    ],
    writeRunner: writePs1Runner,
  },
];

for (const leg of LEGS)
  test(
    `find-polluter${leg.name} distinguishes unrun, clean and polluted`,
    { skip: leg.skip },
    async (t) => {
      for (const c of CASES) {
        if (c.only !== undefined && !c.only.includes(leg.name)) continue;
        await t.test(c.name, (t) => runCase(leg, c, t));
      }
    },
  );
