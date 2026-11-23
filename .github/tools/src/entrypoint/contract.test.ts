import assert from "node:assert/strict";
import { after, test, type TestContext } from "node:test";
import {
  existsSync,
  lstatSync,
  readdirSync,
  readFileSync,
  realpathSync,
  writeFileSync,
} from "node:fs";
import { spawnSync } from "node:child_process";
import { join, sep } from "node:path";
import { REPO_ROOT } from "../plugin/common.ts";
import {
  createProfile,
  createStageFixture,
  createValidateFixture,
  defaultCopilotListing,
  disposeSuite,
  getSuite,
  runValidateFixture,
  stageLogPath,
  validateArgvMismatch,
  type ConformanceCall,
  type Invocation,
  type Profile,
  type Result,
  type StageFixture,
  type ValidateFixture,
} from "./testing/harness.ts";

after(disposeSuite);

test("entrypoint fixture setup and controlled builds need no host Go", (t) => {
  {
    const suite = getSuite();
    const fixture = createValidateFixture(t);
    // The shim builds only from a module holding its marker, so a test that
    // removes the marker sees a failed build.
    const build = spawnSync(
      process.execPath,
      [
        suite.goShim,
        "build",
        "-C",
        fixture.root + "/unmarked-module",
        "-o",
        fixture.root + "/unmarked-verifier.cjs",
        "./cmd/hooktool",
      ],
      { encoding: "utf8", env: { PATH: "" } },
    );
    assert.ifError(build.error);
    assert.notEqual(build.status, 0, build.stdout + build.stderr);
    const profile = createProfile(t);
    const verifier = fixture.root + "/verifier.cjs";
    const hookBuild = spawnSync(
      process.execPath,
      [
        suite.goShim,
        "build",
        "-C",
        fixture.root + "/hooks/go",
        "-o",
        verifier,
        "./cmd/hooktool",
      ],
      { encoding: "utf8", env: { PATH: "" } },
    );
    assert.ifError(hookBuild.error);
    assert.equal(hookBuild.status, 0, hookBuild.stdout + hookBuild.stderr);
    const log = profile.path("portable-calls.jsonl");
    const result = spawnSync(
      process.execPath,
      [
        fixture.root + "/.github/tools/src/conformance/cli.ts",
        "hooks",
        "--repo",
        fixture.root,
      ],
      {
        encoding: "utf8",
        env: {
          PATH: "",
          AGX_HOOK_VERIFIER: verifier,
          AGX_STUB_EXPECT_VERIFIER: verifier,
          AGX_STUB_CONFORMANCE_LOG: log,
        },
      },
    );
    assert.ifError(result.error);
    assert.equal(result.status, 0, result.stdout + result.stderr);
    assert.deepEqual(
      readFileSync(log, "utf8")
        .split("\n")
        .filter(Boolean)
        .map((line) => JSON.parse(line)),
      [{ argv: ["hooks", "--repo", fixture.root], short: "" }],
    );
    assert.ok(result.stdout.includes("PASS hooks"));
  }
});

const stageName = "agentics-local-marketplace";

/** Fails when a fake CLI refusal reached the output: the CLI's stderr must be discarded. */
function assertNoFakeOutput(r: Result): void {
  assert.ok(
    !(r.stdout + r.stderr).includes("forbidden fake"),
    r.stdout + r.stderr,
  );
}

/** The fake CLI log as lines, without the empty remainder after the last newline. */
function logLines(r: Result): string[] {
  return r.log.split("\n").filter((line) => line !== "");
}

/** Runs a stage fixture's maintainer CLI with the profile's own cache root. */
function runStage(
  p: Profile,
  f: StageFixture,
  invocation: Invocation = {},
): Result {
  return p.run({
    ...invocation,
    script: f.cli(),
    env: { XDG_CACHE_HOME: p.path("cache"), ...invocation.env },
  });
}

/** A stage left by an earlier setup, under the profile's cache root. */
function writeStage(p: Profile): string {
  const stage = p.path("cache", stageName);
  p.write(join(stage, "skills/old/SKILL.md"), "an earlier stage\n");
  return stage;
}

function assertAbsent(path: string): void {
  assert.equal(lstatSync(path, { throwIfNoEntry: false }), undefined, path);
}

/** The fake CLI calls setup makes, in order, when every call fails. */
function setupCalls(stage: string): string[] {
  return [
    "claude\tplugin\tmarketplace\tremove\tmuehz",
    `claude\tplugin\tmarketplace\tadd\t${stage}`,
    "claude\tplugin\tmarketplace\tupdate\tmuehz",
    "claude\tplugin\tuninstall\tagentics@muehz\t--keep-data\t--scope\tuser",
    "claude\tplugin\tinstall\tagentics@muehz\t--scope\tuser",
    "copilot\tplugin\tmarketplace\tremove\tmuehz\t--force",
    `copilot\tplugin\tmarketplace\tadd\t${stage}`,
    "copilot\tplugin\tinstall\tagentics@muehz",
    "codex\tplugin\tmarketplace\tremove\tmuehz",
    `codex\tplugin\tmarketplace\tadd\t${stage}`,
    "codex\tplugin\tadd\tagentics@muehz",
    "agy\tplugin\tuninstall\tagentics",
    `agy\tplugin\tinstall\t${stage}`,
  ];
}

const teardownCalls = [
  "claude\tplugin\tuninstall\tagentics@muehz",
  "claude\tplugin\tmarketplace\tremove\tmuehz",
  "claude\tplugin\tmarketplace\tadd\tmuehz/agentics",
  "claude\tplugin\tinstall\tagentics@muehz\t--scope\tuser",
  "copilot\tplugin\tuninstall\tagentics",
  "copilot\tplugin\tmarketplace\tremove\tmuehz\t--force",
  "copilot\tplugin\tmarketplace\tadd\tmuehz/agentics",
  "copilot\tplugin\tinstall\tagentics@muehz",
  "codex\tplugin\tremove\tagentics",
  "codex\tplugin\tmarketplace\tremove\tmuehz",
  "codex\tplugin\tmarketplace\tadd\tmuehz/agentics",
  "codex\tplugin\tadd\tagentics@muehz",
  "agy\tplugin\tuninstall\tagentics",
  "agy\tplugin\tinstall\thttps://github.com/muehz/agentics",
];

const noCliLine = "⚠ No supported plugin CLI found in PATH.";

/** The index of the first stdout line that contains `text`, failing when none does. */
function lineIndex(stdout: string, text: string): number {
  const index = stdout.split("\n").findIndex((line) => line.includes(text));
  assert.notEqual(
    index,
    -1,
    `stdout has no line containing ${text}:\n${stdout}`,
  );
  return index;
}

test("marketplace: setup without a CLI exits 1 after setup validation", (t) => {
  const r = createProfile(t).run({ noCLIs: true });
  assert.equal(r.exit, 1, r.stdout + r.stderr);
  for (const text of [
    "All manifests valid (conformance manifests)",
    noCliLine,
    "  node .github/tools/src/plugin/cli.ts --vscode",
  ])
    assert.ok(r.stdout.includes(text), r.stdout);
  assert.ok(!r.stdout.includes("plugin.sh"), r.stdout);
  assert.ok(!r.stdout.includes("PASS tools"), r.stdout);
});

test("marketplace: setup runs the built manifests domain only", (t) => {
  const p = createProfile(t),
    f = createValidateFixture(t);
  const { result: r, calls } = runValidateFixture(
    p,
    f,
    [],
    {},
    { noCLIs: true },
  );
  assert.equal(r.exit, 1, r.stdout + r.stderr);
  assert.ok(r.stdout.includes(noCliLine), r.stdout);
  assert.equal(validateArgvMismatch(calls, f, ["manifests"]), "");
  assert.ok(!r.stdout.includes("PASS hooks"), r.stdout);
  for (const verifier of ["hooktool", "hooktool.exe"])
    assert.ok(
      !existsSync(p.path("cache", "agentics", verifier)),
      "setup mode built the hook verifier",
    );
});

test("marketplace: setup validation passes before the CLI check", (t) => {
  const p = createProfile(t),
    f = createValidateFixture(t);
  const { result: r, calls } = runValidateFixture(
    p,
    f,
    [],
    {},
    { noCLIs: true },
  );
  assert.equal(r.exit, 1, r.stdout + r.stderr);
  assert.equal(validateArgvMismatch(calls, f, ["manifests"]), "");
  const noCli = lineIndex(r.stdout, noCliLine);
  for (const text of [
    "PASS manifests",
    "All manifests valid (conformance manifests)",
  ])
    assert.ok(lineIndex(r.stdout, text) < noCli, r.stdout);
});

test("marketplace: setup with failing manifests exits 1 before the CLI check", (t) => {
  const p = createProfile(t),
    f = createValidateFixture(t);
  const { result: r, calls } = runValidateFixture(p, f, [], {
    AGX_STUB_CONFORMANCE_FAIL: "1",
  });
  assert.equal(r.exit, 1, r.stdout + r.stderr);
  for (const text of ["FAIL stub", "Validation failed (conformance manifests)"])
    assert.ok(r.stdout.includes(text), r.stdout);
  assert.ok(!r.stdout.includes(noCliLine), r.stdout);
  assert.equal(validateArgvMismatch(calls, f, ["manifests"]), "");
  assert.equal(r.log, "", "a CLI ran after failed validation");
  assertAbsent(p.path("cache", stageName));
});

test("marketplace: setup registers the stage and installs on every CLI in order", (t) => {
  const p = createProfile(t),
    f = createStageFixture(t);
  const r = runStage(p, f);
  assert.equal(r.exit, 0, r.stdout + r.stderr);
  assertNoFakeOutput(r);
  const stage = join(realpathSync.native(p.path("cache")), stageName);
  assert.deepEqual(logLines(r), setupCalls(stage));
  assert.ok(existsSync(join(stage, "skills/committed/SKILL.md")));
  for (const text of [
    "    ⚠ Failed to install agentics\n",
    "    ⚠ Failed to install agentics (may already be installed)\n",
    "✅ Claude Code setup complete.",
    "✅ Copilot CLI setup complete.",
    "✅ Codex CLI setup complete.",
    "✅ Antigravity setup complete.",
  ])
    assert.ok(r.stdout.includes(text), r.stdout);
  assert.equal(
    r.stdout.split("    ⚠ Failed to install agentics\n").length - 1,
    2,
    r.stdout,
  );
  assert.equal(
    r.stdout.split("(may already be installed)").length - 1,
    2,
    r.stdout,
  );
});

test("marketplace: setup passes a stage path with spaces and an ampersand as one argument", (t) => {
  const p = createProfile(t),
    f = createStageFixture(t);
  const cache = p.path("cache dir & (x)", "cache");
  const r = runStage(p, f, { env: { XDG_CACHE_HOME: cache } });
  assert.equal(r.exit, 0, r.stdout + r.stderr);
  assertNoFakeOutput(r);
  const stage = join(realpathSync.native(cache), stageName);
  const lines = logLines(r);
  for (const cli of ["claude", "copilot", "codex"])
    assert.ok(
      lines.includes(`${cli}\tplugin\tmarketplace\tadd\t${stage}`),
      r.log,
    );
  assert.ok(lines.includes(`agy\tplugin\tinstall\t${stage}`), r.log);
});

test("marketplace: teardown without a CLI removes the stage and exits 1", (t) => {
  const p = createProfile(t),
    f = createStageFixture(t);
  const stage = writeStage(p);
  const r = runStage(p, f, { args: ["--teardown"], noCLIs: true });
  assert.equal(r.exit, 1, r.stdout + r.stderr);
  assert.ok(r.stdout.includes("No CLI found."), r.stdout);
  assertAbsent(stage);
});

test("marketplace: teardown restores the remote marketplace on every CLI in order", (t) => {
  const p = createProfile(t),
    f = createStageFixture(t);
  const stage = writeStage(p);
  const r = runStage(p, f, { args: ["--teardown"] });
  assert.equal(r.exit, 0, r.stdout + r.stderr);
  assertNoFakeOutput(r);
  assert.deepEqual(logLines(r), teardownCalls);
  assertAbsent(stage);
  for (const text of [
    "✅ Claude Code teardown complete.",
    "✅ Copilot CLI teardown complete.",
    "✅ Codex CLI teardown complete.",
    "✅ Antigravity teardown complete.",
  ])
    assert.ok(r.stdout.includes(text), r.stdout);
});

test("marketplace: status without a CLI exits 0", (t) => {
  const p = createProfile(t),
    f = createStageFixture(t);
  const r = runStage(p, f, { args: ["--status"], noCLIs: true });
  assert.equal(r.exit, 0, r.stdout + r.stderr);
  assert.ok(r.stdout.includes("=== Plugin Status ==="), r.stdout);
  assert.ok(r.stdout.includes("⚠ No CLI found."), r.stdout);
  assert.ok(r.stdout.includes("(not staged)"), r.stdout);
});

test("marketplace: status lists plugins and marketplaces on every CLI", (t) => {
  const p = createProfile(t),
    f = createStageFixture(t);
  const count = (stdout: string, text: string) =>
    stdout.split("\n").filter((line) => line === text).length;
  const r = runStage(p, f, { args: ["--status"] });
  assert.equal(r.exit, 0, r.stdout + r.stderr);
  assertNoFakeOutput(r);
  const headers = [
    "--- Claude Code ---",
    "--- Copilot CLI ---",
    "--- Codex CLI ---",
    "--- Antigravity ---",
  ].map((header) => lineIndex(r.stdout, header));
  assert.deepEqual(
    headers,
    [...headers].sort((a, b) => a - b),
    r.stdout,
  );
  const listing = lineIndex(r.stdout, defaultCopilotListing);
  assert.ok(headers[1]! < listing && listing < headers[2]!, r.stdout);
  assert.equal(count(r.stdout, "  (unable to list plugins)"), 3, r.stdout);
  assert.equal(count(r.stdout, "  (unable to list marketplaces)"), 3, r.stdout);

  const failing = runStage(p, f, { args: ["--status"], copilotListExit: "3" });
  assert.equal(failing.exit, 0, failing.stdout + failing.stderr);
  assertNoFakeOutput(failing);
  assert.ok(!failing.stdout.includes(defaultCopilotListing), failing.stdout);
  assert.equal(
    count(failing.stdout, "  (unable to list plugins)"),
    4,
    failing.stdout,
  );
});

test("marketplace: status reports an invalid cache root as unavailable", (t) => {
  const p = createProfile(t),
    f = createStageFixture(t);
  const r = runStage(p, f, {
    args: ["--status"],
    env: { XDG_CACHE_HOME: "relative-cache" },
  });
  assert.equal(r.exit, 0, r.stdout + r.stderr);
  assertNoFakeOutput(r);
  assert.ok(
    r.stdout.includes("Marketplace stage: unavailable (invalid cache root)"),
    r.stdout,
  );
  assertAbsent(p.path("relative-cache"));
});

test("last action flag wins", (t) => {
  const r = createProfile(t).run({
    args: ["--teardown", "--status"],
    noCLIs: true,
  });
  assert.equal(r.exit, 0, r.stdout + r.stderr);
  assert.ok(r.stdout.includes("=== Plugin Status ==="));
  assert.ok(!r.stdout.includes("Teardown"));
});

/** The settings path the instructions name, built from the profile's own directories for the host platform. */
function expectedSettingsPath(p: Profile, dirName: string): string {
  const tail = [dirName, "User", "settings.json"];
  if (process.platform === "win32") return join(p.appdata, ...tail);
  if (process.platform === "darwin")
    return [p.home, "Library", "Application Support", ...tail].join("/");
  return [p.xdg, ...tail].join("/");
}

/** The repository path in the form the snippet prints: forward slashes on every platform. */
const repoKey = REPO_ROOT.split(sep).join("/");

/** The whole block the VS Code modes print. */
function expectedVscodeBlock(
  p: Profile,
  label: string,
  dirName: string,
): string {
  return (
    [
      `=== ${label}: local plugin registration ===`,
      "",
      "Settings file:",
      `  ${expectedSettingsPath(p, dirName)}`,
      "",
      "Add these three keys to that file:",
      "",
      '  "chat.pluginLocations": {',
      `    "${repoKey}": true`,
      "  },",
      '  "chat.plugins.marketplaces": ["muehz/agentics"],',
      '  "chat.plugins.enabled": true',
      "",
      `To check the registration, confirm "${repoKey}" is present under`,
      "chat.pluginLocations and set to true.",
      "",
      "To remove it, delete that one entry from chat.pluginLocations.",
      "Leave chat.plugins.marketplaces and chat.plugins.enabled in place —",
      "they are the remote configuration, and remote installs resolve",
      "through them.",
      "",
      "\u{1F4A1} Validate the checkout first with:",
      "   node .github/tools/src/plugin/cli.ts --validate",
    ].join("\n") + "\n\n"
  );
}

test("vscode prints the whole block", (t) => {
  const p = createProfile(t);
  const r = p.run({ args: ["--vscode"] });
  assert.equal(r.exit, 0, r.stdout + r.stderr);
  assert.equal(r.stdout, expectedVscodeBlock(p, "VS Code", "Code"));
  assert.equal(r.stderr, "");
});

test("vscode snippet is valid JSON naming the repository", (t) => {
  const r = createProfile(t).run({ args: ["--vscode"] });
  const lines = r.stdout.split("\n");
  const first = lines.findIndex((line) =>
    line.includes('"chat.pluginLocations": {'),
  );
  const last = lines.findIndex((line) =>
    line.includes('"chat.plugins.enabled": true'),
  );
  assert.ok(first >= 0 && last > first, r.stdout);
  const parsed = JSON.parse(`{${lines.slice(first, last + 1).join("\n")}}`);
  assert.deepEqual(parsed["chat.pluginLocations"], { [repoKey]: true });
  assert.equal(parsed["chat.plugins.enabled"], true);
});

test("vscode insiders names the Insiders variant", (t) => {
  const p = createProfile(t);
  const r = p.run({ args: ["--vscode-insiders"] });
  assert.equal(r.exit, 0, r.stdout + r.stderr);
  assert.equal(
    r.stdout,
    expectedVscodeBlock(p, "VS Code Insiders", "Code - Insiders"),
  );
});

test("vscode prints instructions without writes or CLI calls", (t) => {
  const p = createProfile(t),
    r = p.run({ args: ["--vscode"] });
  assert.equal(r.exit, 0, r.stdout + r.stderr);
  for (const key of [
    "chat.pluginLocations",
    "chat.plugins.marketplaces",
    "chat.plugins.enabled",
  ])
    assert.ok(r.stdout.includes(key));
  assert.equal(r.log.trim(), "");
  for (const dir of [p.home, p.xdg, p.appdata])
    p.assertNoSettingsFileUnder(dir);
});

test("vscode teardown and status print the same instructions", (t) => {
  const p = createProfile(t);
  const base = p.run({ args: ["--vscode"] });
  for (const extra of ["--teardown", "--status"]) {
    const r = p.run({ args: ["--vscode", extra] });
    assert.equal(r.exit, 0, r.stdout + r.stderr);
    assert.equal(r.stdout, base.stdout);
    assert.ok(r.stdout.includes("To remove it, delete that one entry"));
  }
});

test("vscode modes exit 0 with nothing on stderr", (t) => {
  const p = createProfile(t);
  for (const args of [
    ["--vscode"],
    ["--vscode-insiders"],
    ["--vscode", "--teardown"],
    ["--vscode-insiders", "--status"],
  ]) {
    const r = p.run({ args });
    assert.equal(r.exit, 0, args.join(" ") + r.stdout + r.stderr);
    assert.equal(r.stderr, "", args.join(" "));
  }
});

test("entrypoint domain selections and retired markers", (t) => {
  const f = createValidateFixture(t);
  const dir = join(f.root, ".github/tools/src/plugin");
  const body = readdirSync(dir)
    .filter((name) => name.endsWith(".ts") && !name.endsWith(".test.ts"))
    .map((name) => readFileSync(join(dir, name), "utf8"))
    .join("\n");
  for (const marker of [
    "AGX_CONFORMANCE_PIN",
    "--pin",
    "AGX_VSCODE_TESTING",
    "AGX_VSCODE_SETTINGS_PATH",
    "agentics-vscode-state",
  ])
    assert.ok(!body.includes(marker), marker);
});

test("domain mutation is detected by argv assertion", (t) => {
  // The second call's argv carries the selected domain; the first call
  // always holds "hooks" in strict mode.
  const p = createProfile(t),
    f = createValidateFixture(t);
  const file = join(f.root, ".github/tools/src/plugin/validate.ts");
  const body = readFileSync(file, "utf8");
  const from = 'const STRICT_DOMAIN = "all";';
  assert.ok(body.includes(from), `Mutation target not found: ${from}`);
  writeFileSync(file, body.replace(from, 'const STRICT_DOMAIN = "manifests";'));
  const { result: r, calls } = runFixture(p, f, ["--validate"]);
  assert.equal(r.exit, 0, r.stdout + r.stderr);
  assert.equal(
    validateArgvMismatch(
      calls,
      f,
      ["hooks"],
      ["manifests", "--exclude", "hooks"],
    ),
    "",
  );
  assert.notEqual(
    validateArgvMismatch(calls, f, ["hooks"], ["all", "--exclude", "hooks"]),
    "",
    JSON.stringify(calls),
  );
});

/** Checks the observable contract of a passing fast run. */
function assertFastRun(
  p: Profile,
  f: ValidateFixture,
  r: Result,
  calls: ConformanceCall[],
): void {
  assert.equal(
    validateArgvMismatch(
      calls,
      f,
      ["hooks", "--fast"],
      ["all", "--exclude", "hooks"],
    ),
    "",
  );
  // The hooks call shortens through --fast, the other domains through the
  // environment.
  assert.deepEqual(
    calls.map((call) => call.short),
    ["", "1"],
  );
  assert.equal(
    readFileSync(stageLogPath(p), "utf8"),
    "typecheck\nboundaries\n",
  );
  assert.ok(!r.stdout.includes("Tools tests passed"), r.stdout);
  assert.ok(!r.stdout.includes("Evals tests passed"), r.stdout);
  const last = r.stdout.trimEnd().split("\n").at(-1) ?? "";
  for (const want of [
    "Fast validation passed",
    "Go and TypeScript test suites were not run",
    "node .github/tools/src/plugin/cli.ts --validate must pass before a pull request",
  ])
    assert.ok(last.includes(want), `closing line ${JSON.stringify(last)}`);
}

/** The summary's stage rows, in the order the stages run. */
const timingStages = [
  "Hooks conformance",
  "TypeScript conformance",
  "Typecheck",
  "tools tests",
  "Area boundaries",
] as const;
const timingHeader = "=== Timing summary ===",
  timingBuildsRow = "Prerequisite builds",
  timingTotalRow = "Total";
const wholeSeconds = /^[0-9]+s$/;

/**
 * Returns the fields after each row label of the one timing summary in
 * stdout, keyed by label, and the index of the summary's header line.
 */
function timingSummary(stdout: string): {
  rows: Map<string, string[]>;
  start: number;
} {
  const lines = stdout.split("\n");
  const headers = lines.flatMap((line, i) =>
    line === timingHeader ? [i] : [],
  );
  assert.equal(headers.length, 1, `want one timing summary:\n${stdout}`);
  const start = headers[0]!;
  const rows = new Map<string, string[]>();
  const labels = [...timingStages, timingBuildsRow, timingTotalRow];
  for (const line of lines.slice(start + 1))
    for (const label of labels)
      if (line.startsWith(`${label} `))
        rows.set(
          label,
          line
            .slice(label.length + 1)
            .split(/\s+/)
            .filter(Boolean),
        );
  for (const label of labels)
    assert.ok(
      rows.has(label),
      `timing summary has no ${label} row:\n${stdout}`,
    );
  return { rows, start };
}

/** Checks one row's result, and that a whole-second duration appears exactly when the stage ran. */
function assertTimingRow(
  rows: Map<string, string[]>,
  label: string,
  want: string,
): void {
  const got = rows.get(label)!,
    words = want.split(" "),
    ran = want === "passed" || want === "failed";
  const shown = `timing row ${label} = ${JSON.stringify(got)}, want ${want}`;
  assert.deepEqual(got.slice(0, words.length), words, shown);
  assert.equal(got.length, words.length + (ran ? 1 : 0), shown);
  if (ran) assert.match(got.at(-1)!, wholeSeconds, shown);
}

/** Checks the builds row's result and the total's duration. */
function assertTimingTotal(rows: Map<string, string[]>, builds: string): void {
  assertTimingRow(rows, timingBuildsRow, builds);
  const total = rows.get(timingTotalRow)!;
  assert.equal(total.length, 1, `timing total = ${JSON.stringify(total)}`);
  assert.match(total[0]!, wholeSeconds);
}

/** Fails unless the first stdout line containing want comes before the summary header. */
function assertBefore(stdout: string, want: string, start: number): void {
  const line = stdout.split("\n").findIndex((text) => text.includes(want));
  assert.notEqual(
    line,
    -1,
    `stdout has no line containing ${want}:\n${stdout}`,
  );
  assert.ok(
    line < start,
    `the timing summary (line ${start}) precedes ${want} (line ${line}):\n${stdout}`,
  );
}

/** Runs the maintainer CLI from the checkout. */
function runCli(t: TestContext, args: string[]): Result {
  return createProfile(t).run({ args });
}

/** Runs the fixture's CLI. */
function runFixture(
  p: Profile,
  f: ValidateFixture,
  args: string[],
  env: NodeJS.ProcessEnv = {},
  invocation: { omitTools?: "go"[] } = {},
): ReturnType<typeof runValidateFixture> {
  return runValidateFixture(p, f, args, env, invocation);
}

test("dispatch: unknown flag exits 1", (t) => {
  assert.equal(runCli(t, ["--nope"]).exit, 1);
});

test("dispatch: help exits 0 and documents supported options", (t) => {
  const r = runCli(t, ["--help"]);
  assert.equal(r.exit, 0, r.stderr);
  assert.ok(r.stdout.startsWith("Usage:"), r.stdout);
  assert.ok(
    r.stdout.split("\n")[0]!.includes("node .github/tools/src/plugin/cli.ts"),
    r.stdout,
  );
  assert.ok(!r.stdout.includes("plugin.sh"), r.stdout);
  for (const option of ["--vscode-insiders", "--validate"])
    assert.ok(r.stdout.includes(option));
  for (const option of [
    "--bump",
    "AGX_VALIDATE_VSCODE_LOCAL_SH_FIXTURES",
    "AGX_VALIDATE_VSCODE_SETTINGS_FIXTURES",
  ])
    assert.ok(!r.stdout.includes(option));
});

test("validate: strict validation delegates domain all", (t) => {
  const p = createProfile(t),
    f = createValidateFixture(t);
  const { result: r, calls } = runFixture(p, f, ["--validate"]);
  assert.equal(r.exit, 0, r.stdout + r.stderr);
  // The hooks stage runs the hooks domain and the next stage every other
  // domain, so each call must report and receive its own share.
  for (const text of [
    "PASS hooks",
    "PASS manifests",
    "PASS tools",
    "All manifests valid (conformance all)",
  ])
    assert.ok(r.stdout.includes(text), r.stdout);
  assert.equal(
    validateArgvMismatch(calls, f, ["hooks"], ["all", "--exclude", "hooks"]),
    "",
  );
});

// A Node outside the root engines range must stop strict --validate before
// either conformance stage runs, so no half-run report can look like a pass.
test("validate: strict validation outside the Node range stops before conformance", (t) => {
  const p = createProfile(t),
    f = createValidateFixture(t);
  const manifest = join(f.root, "package.json");
  const pkg = JSON.parse(readFileSync(manifest, "utf8"));
  pkg.engines = { ...pkg.engines, node: ">=999.0.0" };
  writeFileSync(manifest, JSON.stringify(pkg, null, 2));
  const { result: r, calls } = runFixture(p, f, ["--validate"]);
  assert.equal(r.exit, 1, r.stdout + r.stderr);
  assert.deepEqual(calls, [], "the conformance stub ran before the Node check");
  assert.ok(
    !existsSync(p.path("cache", "agentics", "hooktool")),
    "the hook verifier was built before the Node check",
  );
  assert.ok(!r.stdout.includes("=== Validating manifests ==="), r.stdout);
  assert.ok(!r.stdout.includes("=== Timing summary ==="), r.stdout);
  assert.ok(
    r.stderr.includes(
      "❌ --validate needs Node >=999.0.0 for the TypeScript conformance suite and the tools tests.",
    ),
    r.stderr,
  );
});

test("validate: vscode validate runs validation instead of instructions", (t) => {
  const p = createProfile(t),
    f = createValidateFixture(t);
  const { result: r, calls } = runFixture(p, f, ["--vscode", "--validate"]);
  assert.equal(r.exit, 0, r.stdout + r.stderr);
  assert.ok(r.stdout.includes("=== Validating manifests ==="));
  assert.ok(r.stdout.includes("All manifests valid (conformance all)"));
  for (const text of ["PASS hooks", "PASS manifests", "PASS tools"])
    assert.ok(r.stdout.includes(text), r.stdout);
  assert.ok(!r.stdout.includes("chat.pluginLocations"));
  assert.equal(
    validateArgvMismatch(calls, f, ["hooks"], ["all", "--exclude", "hooks"]),
    "",
  );
});

test("validate: vscode validate fast runs fast validation", (t) => {
  const p = createProfile(t),
    f = createValidateFixture(t);
  const { result: r, calls } = runFixture(p, f, [
    "--vscode",
    "--validate",
    "--fast",
  ]);
  assert.equal(r.exit, 0, r.stdout + r.stderr);
  assert.ok(!r.stdout.includes("chat.pluginLocations"));
  assertFastRun(p, f, r, calls);
});

// --fast modifies --validate only; on any other action, including the default
// install, it must stop before doing anything.
for (const args of [["--fast"], ["--fast", "--status"], ["--vscode", "--fast"]])
  test(`dispatch: fast without validate is a usage error: ${args.join(" ")}`, (t) => {
    const r = runCli(t, args);
    assert.equal(r.exit, 1, r.stdout + r.stderr);
    assert.ok(r.stdout.includes("--fast needs --validate"), r.stdout);
    assert.ok(r.stdout.includes("--help for usage"), r.stdout);
    assert.ok(!r.stdout.includes("chat.pluginLocations"));
    assert.ok(!r.stdout.includes("=== Plugin Status ==="));
    assert.equal(r.log.trim(), "", "--fast without --validate invoked a CLI");
  });

test("dispatch: unknown option diagnostic goes to stdout", (t) => {
  const r = runCli(t, ["--nope"]);
  assert.equal(r.exit, 1);
  assert.ok(r.stdout.includes("Unknown option: --nope"));
  assert.ok(!r.stderr.includes("Unknown option"));
});

test("dispatch: retired bump option is unknown", (t) => {
  const r = runCli(t, ["--bump", "minor"]);
  assert.equal(r.exit, 1);
  assert.ok(r.stdout.includes("Unknown option: --bump"));
});

test("validate: failing stub reports strict failure", (t) => {
  const p = createProfile(t),
    f = createValidateFixture(t);
  const { result: r } = runFixture(p, f, ["--validate"], {
    AGX_STUB_CONFORMANCE_FAIL: "1",
  });
  assert.equal(r.exit, 1, r.stdout + r.stderr);
  assert.ok(r.stdout.includes("Validation failed (conformance all)"));
});

test("validate: fast run skips the test suites", (t) => {
  const p = createProfile(t),
    f = createValidateFixture(t);
  const { result: r, calls } = runFixture(p, f, ["--validate", "--fast"]);
  assert.equal(r.exit, 0, r.stdout + r.stderr);
  assertFastRun(p, f, r, calls);
});

// The fixture's test stage fails when asked to, so a fast run that passes with
// the failure armed proves it never started it.
test("validate: fast run ignores failing test stages", (t) => {
  const p = createProfile(t),
    f = createValidateFixture(t);
  const { result: r, calls } = runFixture(p, f, ["--validate", "--fast"], {
    AGX_STUB_TOOLS_TEST_FAIL: "1",
  });
  assert.equal(r.exit, 0, r.stdout + r.stderr);
  assertFastRun(p, f, r, calls);
});

// Every stage a fast run keeps still fails it, and the fixture can fail each
// stage alone. The test stages fail only the full run.
for (const [failVar, fastRuns, message] of [
  ["AGX_STUB_HOOKS_FAIL", true, "Validation failed (conformance all)"],
  ["AGX_STUB_CONFORMANCE_FAIL", true, "Validation failed (conformance all)"],
  ["AGX_STUB_TYPECHECK_FAIL", true, "Workspace typecheck failed"],
  ["AGX_STUB_BOUNDARIES_FAIL", true, "Area boundary check failed"],
  ["AGX_STUB_TOOLS_TEST_FAIL", false, "Tools tests failed"],
] as const) {
  const modes = fastRuns
    ? [["--validate"], ["--validate", "--fast"]]
    : [["--validate"]];
  for (const args of modes)
    test(`validate: ${failVar} ${args.join(" ")}`, (t) => {
      const p = createProfile(t),
        f = createValidateFixture(t);
      const { result: r } = runFixture(p, f, args, { [failVar]: "1" });
      assert.equal(r.exit, 1, r.stdout + r.stderr);
      assert.ok((r.stdout + r.stderr).includes(message), r.stdout + r.stderr);
      assert.ok(!r.stdout.includes("Fast validation passed"), r.stdout);
    });
}

test("validate: full run times every stage", (t) => {
  const p = createProfile(t),
    f = createValidateFixture(t);
  const { result: r } = runFixture(p, f, ["--validate"]);
  assert.equal(r.exit, 0, r.stdout + r.stderr);
  const { rows, start } = timingSummary(r.stdout);
  for (const label of timingStages) assertTimingRow(rows, label, "passed");
  assertTimingTotal(rows, "passed");
  assertBefore(r.stdout, "Area boundaries hold", start);
  assertBefore(r.stdout, "All manifests valid", start);
});

test("validate: fast run leaves out the test stages", (t) => {
  const p = createProfile(t),
    f = createValidateFixture(t);
  const { result: r, calls } = runFixture(p, f, ["--validate", "--fast"]);
  assert.equal(r.exit, 0, r.stdout + r.stderr);
  // The closing line stays last, after the summary.
  assertFastRun(p, f, r, calls);
  const { rows, start } = timingSummary(r.stdout);
  for (const label of timingStages)
    assertTimingRow(
      rows,
      label,
      label === "tools tests" ? "not run" : "passed",
    );
  assertTimingTotal(rows, "passed");
  assertBefore(r.stdout, "All manifests valid", start);
});

// Both conformance stages run before the combined result, so failing both
// fails both rows; nothing after the combined result runs.
test("validate: conformance failure stops later stages", (t) => {
  const p = createProfile(t),
    f = createValidateFixture(t);
  const { result: r } = runFixture(p, f, ["--validate"], {
    AGX_STUB_HOOKS_FAIL: "1",
    AGX_STUB_CONFORMANCE_FAIL: "1",
  });
  assert.equal(r.exit, 1, r.stdout + r.stderr);
  const { rows, start } = timingSummary(r.stdout);
  assertTimingRow(rows, "Hooks conformance", "failed");
  assertTimingRow(rows, "TypeScript conformance", "failed");
  for (const label of timingStages.slice(2))
    assertTimingRow(rows, label, "not run");
  assertTimingTotal(rows, "passed");
  assertBefore(r.stdout, "Validation failed (conformance all)", start);
});

// A hooks failure does not stop the other domains.
test("validate: hooks failure still runs typescript conformance", (t) => {
  const p = createProfile(t),
    f = createValidateFixture(t);
  const { result: r } = runFixture(p, f, ["--validate"], {
    AGX_STUB_HOOKS_FAIL: "1",
  });
  assert.equal(r.exit, 1, r.stdout + r.stderr);
  const { rows } = timingSummary(r.stdout);
  assertTimingRow(rows, "Hooks conformance", "failed");
  assertTimingRow(rows, "TypeScript conformance", "passed");
  for (const label of timingStages.slice(2))
    assertTimingRow(rows, label, "not run");
});

test("validate: tools test failure stops later stages", (t) => {
  const p = createProfile(t),
    f = createValidateFixture(t);
  const { result: r } = runFixture(p, f, ["--validate"], {
    AGX_STUB_TOOLS_TEST_FAIL: "1",
  });
  assert.equal(r.exit, 1, r.stdout + r.stderr);
  const { rows } = timingSummary(r.stdout);
  for (const [label, want] of Object.entries({
    "Hooks conformance": "passed",
    "TypeScript conformance": "passed",
    Typecheck: "passed",
    "tools tests": "failed",
    "Area boundaries": "not run",
  }))
    assertTimingRow(rows, label, want);
  assertTimingTotal(rows, "passed");
});

test("validate: missing typescript skips the typecheck", (t) => {
  const p = createProfile(t),
    f = createValidateFixture(t);
  f.remove("node_modules/.bin/tsc");
  const { result: r } = runFixture(p, f, ["--validate"]);
  assert.equal(r.exit, 0, r.stdout + r.stderr);
  assert.ok(
    r.stdout.includes("Typecheck skipped: TypeScript is not installed"),
    r.stdout,
  );
  const { rows } = timingSummary(r.stdout);
  for (const label of timingStages)
    assertTimingRow(rows, label, label === "Typecheck" ? "skipped" : "passed");
  assertTimingTotal(rows, "passed");
});

// Without the tools package nothing in the TypeScript workspaces runs. A fast
// run still shows its left-out test stages as not run.
const packageCases: {
  name: string;
  remove: string[];
  args: string[];
  skipped: Record<string, string>;
}[] = [
  {
    name: "missing tools package",
    remove: [".github/tools"],
    args: ["--validate"],
    skipped: {
      Typecheck: "skipped",
      "tools tests": "skipped",
      "Area boundaries": "skipped",
    },
  },
  {
    name: "missing tools package fast",
    remove: [".github/tools"],
    args: ["--validate", "--fast"],
    skipped: {
      Typecheck: "skipped",
      "tools tests": "not run",
      "Area boundaries": "skipped",
    },
  },
];
for (const { name, remove, args, skipped } of packageCases)
  test(`validate: ${name}`, (t) => {
    const p = createProfile(t),
      f = createValidateFixture(t);
    for (const area of remove) f.remove(`${area}/package.json`);
    const { result: r } = runFixture(p, f, args);
    assert.equal(r.exit, 0, r.stdout + r.stderr);
    const { rows } = timingSummary(r.stdout);
    for (const label of timingStages)
      assertTimingRow(rows, label, skipped[label] ?? "passed");
    assertTimingTotal(rows, "passed");
  });

// A prerequisite build that fails stops the run before any stage, and the
// summary still prints, with the builds row failed. Without its marker the
// controlled build shim refuses the build, as a broken source would; without
// Go there is nothing to build with. Either way the run names the Go version
// the hooks module declares.
const needsGo = "❌ --validate needs Go 1.27.4 to build the hook verifier";
const buildCases: {
  name: string;
  marker?: string;
  omitTools?: "go"[];
  messages: string[];
  absent: string[];
}[] = [
  {
    name: "verifier build fails",
    marker: "hooks/go/.agentics-entrypoint-hooktool",
    messages: ["❌ Could not build the trusted hook verifier.", needsGo],
    absent: [],
  },
  {
    name: "strict without Go",
    omitTools: ["go"],
    messages: [needsGo],
    absent: ["Could not build the trusted hook verifier"],
  },
];
for (const { name, marker, omitTools, messages, absent } of buildCases)
  test(`validate: ${name}`, (t) => {
    const p = createProfile(t),
      f = createValidateFixture(t);
    if (marker) f.remove(marker);
    const { result: r, calls } = runFixture(
      p,
      f,
      ["--validate"],
      {},
      { omitTools },
    );
    assert.equal(r.exit, 1, r.stdout + r.stderr);
    const errors = r.stderr.split("\n");
    for (const message of messages)
      assert.ok(errors.includes(message), r.stderr);
    for (const text of absent) assert.ok(!r.stderr.includes(text), r.stderr);
    assert.deepEqual(calls, [], "a conformance stage ran");
    assert.ok(!r.stdout.includes("=== Validating manifests ==="), r.stdout);
    const { rows } = timingSummary(r.stdout);
    for (const label of timingStages) assertTimingRow(rows, label, "not run");
    assertTimingTotal(rows, "failed");
  });
