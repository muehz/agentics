import assert from "node:assert/strict";
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
import { after, test } from "node:test";
import {
  VALIDATE_STAGES,
  defaultRunner,
  formatTimingSummary,
  runValidate,
  type Command,
  type CommandResult,
  type StageKey,
  type ValidateOptions,
} from "./validate.ts";

const isWindows = process.platform === "win32";
const ENGINES = "^22.23.5 || ^24.22.3 || >=26.12.1";

const roots: string[] = [];
function tempDir(): string {
  const dir = mkdtempSync(path.join(os.tmpdir(), "agentics-validate-"));
  roots.push(dir);
  return dir;
}
after(() => {
  for (const dir of roots) rmSync(dir, { recursive: true, force: true });
});

function writeFile(file: string, text: string, executable = false): void {
  mkdirSync(path.dirname(file), { recursive: true });
  writeFileSync(file, text);
  if (executable && !isWindows) chmodSync(file, 0o755);
}

interface FixtureOptions {
  tsc?: boolean;
  toolsPackage?: boolean;
}

/** A checkout with only what the validate stages probe: go.mod, the tools package and the installed tsc. */
function fixture({ tsc = true, toolsPackage = true }: FixtureOptions = {}): {
  repoRoot: string;
  cacheDir: string;
} {
  const base = tempDir();
  const repoRoot = path.join(base, "repo");
  writeFile(
    path.join(repoRoot, "hooks", "go", "go.mod"),
    "module example\n\ngo 1.27.4\n",
  );
  if (toolsPackage) {
    writeFile(path.join(repoRoot, ".github", "tools", "package.json"), "{}\n");
  }
  if (tsc) {
    // Both names, so the lookup succeeds whichever platform a case injects.
    const bin = path.join(repoRoot, "node_modules", ".bin");
    writeFile(path.join(bin, "tsc"), "", true);
    writeFile(path.join(bin, "tsc.cmd"), "");
  }
  return { repoRoot, cacheDir: path.join(base, "cache", "agentics") };
}

type Rule = (command: Command) => Partial<CommandResult> | undefined;

interface Harness {
  calls: Command[];
  lines: { stream: "stdout" | "stderr"; text: string }[];
  stdout(): string;
  stderr(): string;
  all(): string;
  run(overrides?: Partial<ValidateOptions>): number;
  repoRoot: string;
  cacheDir: string;
  env: NodeJS.ProcessEnv;
}

function harness(
  rules: Rule[] = [],
  fixtureOptions: FixtureOptions = {},
): Harness {
  const { repoRoot, cacheDir } = fixture(fixtureOptions);
  const calls: Command[] = [];
  const lines: Harness["lines"] = [];
  let now = 100;
  const env: NodeJS.ProcessEnv = { PATH: "", PATHEXT: ".COM;.EXE;.CMD" };
  const h: Harness = {
    calls,
    lines,
    repoRoot,
    cacheDir,
    env,
    stdout: () =>
      lines
        .filter((l) => l.stream === "stdout")
        .map((l) => l.text)
        .join(""),
    stderr: () =>
      lines
        .filter((l) => l.stream === "stderr")
        .map((l) => l.text)
        .join(""),
    all: () => lines.map((l) => l.text).join(""),
    run: (overrides = {}) =>
      runValidate({
        mode: "strict",
        speed: "full",
        repoRoot,
        toolCacheDir: cacheDir,
        env,
        platform: "linux",
        nodeVersion: "26.12.1",
        enginesNode: ENGINES,
        // Every runner call advances the clock by two seconds, so each ran stage reads 2s.
        clock: () => now,
        runner: (command) => {
          calls.push(command);
          now += 2;
          for (const rule of rules) {
            const result = rule(command);
            if (result !== undefined)
              return { found: true, status: 0, output: "", ...result };
          }
          return { found: true, status: 0, output: "" };
        },
        write: (stream, text) => lines.push({ stream, text }),
        ...overrides,
      }),
  };
  return h;
}

const isBuild = (c: Command) => c.tool === "go";
const isConformance = (domain: string) => (c: Command) =>
  c.tool === "node" &&
  c.args[0]?.endsWith(path.join("conformance", "cli.ts")) === true &&
  c.args[1] === domain;
const isTypecheck = (c: Command) =>
  c.tool === "npm" && c.args.includes("test:typecheck");
const isToolsTests = (c: Command) =>
  c.tool === "npm" && c.args.at(-1) === "test";
const isBoundaries = (c: Command) =>
  c.tool === "node" &&
  c.args[0]?.endsWith(path.join("boundaries", "cli.ts")) === true;

function failing(match: (c: Command) => boolean): Rule {
  return (c) => (match(c) ? { status: 1 } : undefined);
}

function kinds(calls: Command[]): string[] {
  return calls.map((c) => {
    if (isBuild(c)) return "build";
    if (isConformance("hooks")(c)) return "hooks";
    if (isConformance("all")(c)) return "all";
    if (isConformance("manifests")(c)) return "manifests";
    if (isTypecheck(c)) return "typecheck";
    if (isToolsTests(c)) return "tools";
    if (isBoundaries(c)) return "boundaries";
    return `unknown:${c.tool} ${c.args.join(" ")}`;
  });
}

/** The summary rows by label, as printed. */
function summaryRows(stdout: string): Map<string, string> {
  const start = stdout.indexOf("=== Timing summary ===\n");
  assert.notEqual(start, -1, "timing summary printed");
  const rows = new Map<string, string>();
  for (const line of stdout.slice(start).split("\n").slice(1, 8)) {
    rows.set(line.slice(0, 24).trimEnd(), line.slice(25).trim());
  }
  return rows;
}

const ROW_LABELS = [
  "Hooks conformance",
  "TypeScript conformance",
  "Typecheck",
  "tools tests",
  "Area boundaries",
  "Prerequisite builds",
  "Total",
];

test("strict full validation runs the six stages in order and passes", () => {
  const h = harness();
  assert.equal(h.run(), 0);
  assert.deepEqual(kinds(h.calls), [
    "build",
    "hooks",
    "all",
    "typecheck",
    "tools",
    "boundaries",
  ]);
  assert.deepEqual(
    VALIDATE_STAGES.map((s) => s.key),
    [
      "builds",
      "hooks_conformance",
      "ts_conformance",
      "typecheck",
      "tools_tests",
      "boundaries",
    ],
  );
  const [build, hooks, all, typecheck, tools, boundaries] = h.calls;
  const verifier = path.join(h.cacheDir, "hooktool");
  assert.deepEqual(build?.args, [
    "build",
    "-C",
    path.join(h.repoRoot, "hooks", "go"),
    "-o",
    verifier,
    "./cmd/hooktool",
  ]);
  const conformanceCli = path.join(
    h.repoRoot,
    ".github",
    "tools",
    "src",
    "conformance",
    "cli.ts",
  );
  assert.deepEqual(hooks?.args, [
    conformanceCli,
    "hooks",
    "--repo",
    h.repoRoot,
  ]);
  assert.deepEqual(all?.args, [
    conformanceCli,
    "all",
    "--repo",
    h.repoRoot,
    "--exclude",
    "hooks",
  ]);
  assert.equal(all?.env.AGX_CONFORMANCE_SHORT, undefined);
  assert.deepEqual(typecheck?.args, [
    "--prefix",
    h.repoRoot,
    "run",
    "test:typecheck",
  ]);
  assert.deepEqual(tools?.args, [
    "--prefix",
    path.join(h.repoRoot, ".github", "tools"),
    "test",
  ]);
  assert.deepEqual(boundaries?.args, [
    path.join(h.repoRoot, ".github", "tools", "src", "boundaries", "cli.ts"),
    "--repo",
    h.repoRoot,
  ]);
  assert.equal(boundaries?.capture, true);
  const out = h.stdout();
  assert.match(
    out,
    /^=== Validating manifests ===\n\n✅ All manifests valid \(conformance all\)\.\n=== Testing TypeScript workspaces \(typecheck and node:test\) ===\n✅ Tools tests passed\.\n=== Checking area boundaries ===\n\n✅ Area boundaries hold\.\n\n=== Timing summary ===\n/,
  );
  assert.doesNotMatch(out, /Fast validation passed/);
  assert.equal(h.stderr(), "");
  const rows = summaryRows(out);
  assert.deepEqual([...rows.keys()], ROW_LABELS);
  for (const label of ROW_LABELS.slice(0, 6))
    assert.equal(rows.get(label), "passed   2s");
  assert.equal(rows.get("Total"), "12s");
});

test("fast validation passes --fast to hooks, shortens the rest and skips the tools tests", () => {
  const h = harness();
  assert.equal(h.run({ speed: "fast" }), 0);
  assert.deepEqual(kinds(h.calls), [
    "build",
    "hooks",
    "all",
    "typecheck",
    "boundaries",
  ]);
  assert.equal(h.calls[1]?.args.at(-1), "--fast");
  assert.equal(h.calls[1]?.env.AGX_CONFORMANCE_SHORT, undefined);
  assert.equal(h.calls[2]?.env.AGX_CONFORMANCE_SHORT, "1");
  assert.equal(h.calls[3]?.env.AGX_CONFORMANCE_SHORT, undefined);
  const out = h.stdout();
  assert.match(
    out,
    /=== Typechecking TypeScript workspaces \(node:test skipped\) ===\n/,
  );
  assert.equal(summaryRows(out).get("tools tests"), "not run");
  assert.ok(
    out.endsWith(
      "\n\n✅ Fast validation passed. The Go and TypeScript test suites were not run; node .github/tools/src/plugin/cli.ts --validate must pass before a pull request.\n",
    ),
  );
});

test("a missing go fails the builds stage with only the needs-Go line", () => {
  const h = harness([(c) => (isBuild(c) ? { found: false } : undefined)]);
  assert.equal(h.run(), 1);
  assert.deepEqual(kinds(h.calls), ["build"]);
  assert.equal(
    h.stderr(),
    "❌ --validate needs Go 1.27.4 to build the hook verifier\n",
  );
  const rows = summaryRows(h.stdout());
  assert.equal(rows.get("Prerequisite builds"), "failed   2s");
  for (const label of ROW_LABELS.slice(0, 5))
    assert.equal(rows.get(label), "not run");
  assert.doesNotMatch(h.stdout(), /Validating manifests/);
});

test("a failed go build prints both build lines and stops before conformance", () => {
  const h = harness([failing(isBuild)]);
  assert.equal(h.run(), 1);
  assert.deepEqual(kinds(h.calls), ["build"]);
  assert.equal(
    h.stderr(),
    "❌ Could not build the trusted hook verifier.\n❌ --validate needs Go 1.27.4 to build the hook verifier\n",
  );
  const rows = summaryRows(h.stdout());
  assert.equal(rows.get("Prerequisite builds"), "failed   2s");
  for (const label of ROW_LABELS.slice(0, 5))
    assert.equal(rows.get(label), "not run");
});

test("an uncreatable cache directory fails the build without starting go", () => {
  const h = harness();
  const blocker = path.join(h.repoRoot, "not-a-directory");
  writeFile(blocker, "");
  assert.equal(h.run({ toolCacheDir: path.join(blocker, "agentics") }), 1);
  assert.deepEqual(h.calls, []);
  assert.equal(
    h.stderr(),
    "❌ Could not build the trusted hook verifier.\n❌ --validate needs Go 1.27.4 to build the hook verifier\n",
  );
  assert.equal(
    summaryRows(h.stdout()).get("Prerequisite builds"),
    "failed   0s",
  );
});

test("a hooks conformance failure still runs the other domains, then stops", () => {
  const h = harness([failing(isConformance("hooks"))]);
  assert.equal(h.run(), 1);
  assert.deepEqual(kinds(h.calls), ["build", "hooks", "all"]);
  const out = h.stdout();
  assert.match(
    out,
    /\n\n❌ Validation failed \(conformance all\) — see the FAIL\/ERROR lines above\.\n\n=== Timing summary ===/,
  );
  const rows = summaryRows(out);
  assert.equal(rows.get("Hooks conformance"), "failed   2s");
  assert.equal(rows.get("TypeScript conformance"), "passed   2s");
  for (const label of ["Typecheck", "tools tests", "Area boundaries"])
    assert.equal(rows.get(label), "not run");
});

test("a TypeScript conformance failure alone stops before the tools stages", () => {
  const h = harness([failing(isConformance("all"))]);
  assert.equal(h.run(), 1);
  assert.deepEqual(kinds(h.calls), ["build", "hooks", "all"]);
  const rows = summaryRows(h.stdout());
  assert.equal(rows.get("Hooks conformance"), "passed   2s");
  assert.equal(rows.get("TypeScript conformance"), "failed   2s");
  for (const label of ["Typecheck", "tools tests", "Area boundaries"])
    assert.equal(rows.get(label), "not run");
  assert.match(h.stdout(), /❌ Validation failed \(conformance all\)/);
});

test("a typecheck failure stops before the tools tests and boundaries", () => {
  const h = harness([failing(isTypecheck)]);
  assert.equal(h.run(), 1);
  assert.deepEqual(kinds(h.calls), ["build", "hooks", "all", "typecheck"]);
  assert.equal(
    h.stderr(),
    "❌ Workspace typecheck failed — see the output above.\n",
  );
  const rows = summaryRows(h.stdout());
  assert.equal(rows.get("Typecheck"), "failed   2s");
  assert.equal(rows.get("tools tests"), "not run");
  assert.equal(rows.get("Area boundaries"), "not run");
});

test("a tools test failure stops before the boundaries", () => {
  const h = harness([failing(isToolsTests)]);
  assert.equal(h.run(), 1);
  assert.deepEqual(kinds(h.calls), [
    "build",
    "hooks",
    "all",
    "typecheck",
    "tools",
  ]);
  assert.equal(h.stderr(), "❌ Tools tests failed — see the output above.\n");
  assert.doesNotMatch(h.stdout(), /Tools tests passed/);
  const rows = summaryRows(h.stdout());
  assert.equal(rows.get("tools tests"), "failed   2s");
  assert.equal(rows.get("Area boundaries"), "not run");
});

test("a boundaries failure prints the captured output, then the failure line", () => {
  const h = harness([
    (c) =>
      isBoundaries(c)
        ? { status: 2, output: "FAIL some rule\nboundaries: 1 failures\n" }
        : undefined,
  ]);
  assert.equal(h.run(), 1);
  assert.ok(
    h
      .all()
      .includes(
        "=== Checking area boundaries ===\nFAIL some rule\nboundaries: 1 failures\n❌ Area boundary check failed — see the output above.\n",
      ),
  );
  assert.equal(
    h.stderr(),
    "❌ Area boundary check failed — see the output above.\n",
  );
  assert.equal(summaryRows(h.stdout()).get("Area boundaries"), "failed   2s");
});

test("a missing TypeScript install skips the typecheck without seconds", () => {
  const h = harness([], { tsc: false });
  assert.equal(h.run(), 0);
  assert.deepEqual(kinds(h.calls), [
    "build",
    "hooks",
    "all",
    "tools",
    "boundaries",
  ]);
  assert.match(
    h.stdout(),
    /\nℹ Typecheck skipped: TypeScript is not installed\. Run npm ci --ignore-scripts to enable it\.\n/,
  );
  assert.equal(summaryRows(h.stdout()).get("Typecheck"), "skipped");
});

test("a checkout without the tools package skips its stages and passes", () => {
  const full = harness([], { toolsPackage: false });
  assert.equal(full.run(), 0);
  assert.deepEqual(kinds(full.calls), ["build", "hooks", "all"]);
  let rows = summaryRows(full.stdout());
  assert.equal(rows.get("Typecheck"), "skipped");
  assert.equal(rows.get("tools tests"), "skipped");
  assert.equal(rows.get("Area boundaries"), "skipped");
  assert.doesNotMatch(full.stdout(), /TypeScript workspaces/);
  assert.ok(full.stdout().endsWith("\nTotal                             6s\n"));

  const fast = harness([], { toolsPackage: false });
  assert.equal(fast.run({ speed: "fast" }), 0);
  rows = summaryRows(fast.stdout());
  assert.equal(rows.get("Typecheck"), "skipped");
  assert.equal(rows.get("tools tests"), "not run");
  assert.equal(rows.get("Area boundaries"), "skipped");
  assert.ok(
    fast
      .stdout()
      .endsWith(
        "\nTotal                             6s\n\n✅ Fast validation passed. The Go and TypeScript test suites were not run; node .github/tools/src/plugin/cli.ts --validate must pass before a pull request.\n",
      ),
  );
});

test("a SKIP line in the boundary output reports skipped rules instead of holding", () => {
  const h = harness([
    (c) =>
      isBoundaries(c)
        ? { output: "SKIP imports: tsc missing\nboundaries: 0 failures\n" }
        : undefined,
  ]);
  assert.equal(h.run(), 0);
  assert.match(
    h.stdout(),
    /\nℹ Some area boundary rules were skipped \(see the SKIP lines above\)\. Run npm ci --ignore-scripts to enable them\.\n/,
  );
  assert.doesNotMatch(h.stdout(), /Area boundaries hold/);
});

test("a Node outside engines.node stops before any stage and prints no summary", () => {
  const h = harness();
  assert.equal(h.run({ nodeVersion: "26.12.0" }), 1);
  assert.deepEqual(h.calls, []);
  assert.equal(
    h.stderr(),
    `❌ --validate needs Node ${ENGINES} for the TypeScript conformance suite and the tools tests.\n`,
  );
  assert.equal(h.stdout(), "");
});

test("an unreadable engines.node range is one failure line, not a stack trace", () => {
  const h = harness();
  const manifest = path.join(h.repoRoot, "package.json");
  for (const text of [undefined, "{not json", '{"engines":{}}']) {
    if (text === undefined) rmSync(manifest, { force: true });
    else writeFile(manifest, text);
    h.lines.length = 0;
    assert.equal(h.run({ enginesNode: undefined }), 1);
    assert.deepEqual(h.calls, []);
    assert.equal(h.stdout(), "");
    assert.match(
      h.stderr(),
      /^❌ Could not read the Node range from .*package\.json: [^\n]+\n$/,
    );
  }
});

test("later stages get the built verifier in a copy of the caller's env", () => {
  for (const [platform, name] of [
    ["linux", "hooktool"],
    ["win32", "hooktool.exe"],
  ] as const) {
    const h = harness();
    const before = { ...h.env };
    assert.equal(h.run({ platform }), 0);
    const verifier = path.join(h.cacheDir, name);
    assert.equal(h.calls[0]?.args[4], verifier);
    assert.equal(h.calls[0]?.env.AGX_HOOK_VERIFIER, undefined);
    for (const call of h.calls.slice(1))
      assert.equal(call.env.AGX_HOOK_VERIFIER, verifier);
    assert.deepEqual(h.env, before);
    assert.ok(existsSync(h.cacheDir), "the cache directory is created");
  }
});

test("formatTimingSummary pads each row and omits seconds for rows that did not run", () => {
  const records: Partial<
    Record<
      StageKey,
      { result: "passed" | "failed" | "skipped"; seconds?: number }
    >
  > = {
    builds: { result: "passed", seconds: 3 },
    hooks_conformance: { result: "failed", seconds: 121 },
    ts_conformance: { result: "passed", seconds: 0 },
    typecheck: { result: "skipped", seconds: 0 },
  };
  assert.equal(
    formatTimingSummary(records, 450),
    [
      "",
      "=== Timing summary ===",
      "Hooks conformance        failed   121s",
      "TypeScript conformance   passed   0s",
      "Typecheck                skipped",
      "tools tests              not run",
      "Area boundaries          not run",
      "Prerequisite builds      passed   3s",
      "Total                             450s",
      "",
    ].join("\n"),
  );
});

test("setup mode runs only the manifests domain", () => {
  const pass = harness();
  assert.equal(
    pass.run({ mode: "setup", speed: undefined, nodeVersion: "1.0.0" }),
    0,
  );
  assert.deepEqual(kinds(pass.calls), ["manifests"]);
  assert.equal(pass.calls[0]?.env.AGX_HOOK_VERIFIER, undefined);
  assert.equal(
    pass.stdout(),
    "=== Validating manifests ===\n\n✅ All manifests valid (conformance manifests).\n",
  );
  assert.ok(!existsSync(pass.cacheDir), "setup builds nothing");

  const fail = harness([failing(isConformance("manifests"))]);
  assert.equal(fail.run({ mode: "setup" }), 1);
  assert.equal(
    fail.stdout(),
    "=== Validating manifests ===\n\n❌ Validation failed (conformance manifests) — see the FAIL/ERROR lines above.\n",
  );
});

test("the default runner starts node from process.execPath and captures both streams", () => {
  const result = defaultRunner({
    tool: "node",
    args: [
      "-e",
      "process.stdout.write('out\\n'); process.stderr.write('err\\n'); process.exit(3)",
    ],
    env: { ...process.env, PATH: "" },
    capture: true,
  });
  assert.equal(result.found, true);
  assert.equal(result.status, 3);
  assert.equal(result.output, "out\nerr\n");
});

test("the default runner reports a tool missing from PATH as not found", () => {
  const result = defaultRunner({
    tool: "go",
    args: ["version"],
    env: { PATH: tempDir(), PATHEXT: ".EXE;.CMD" },
    capture: true,
  });
  assert.deepEqual(result, { found: false, status: 1, output: "" });
});

test("the default runner starts an npm shim found on PATH with arguments intact", () => {
  const bin = tempDir();
  const echo = path.join(bin, "echo-args.js");
  writeFile(
    echo,
    "process.stdout.write(JSON.stringify(process.argv.slice(2)));\n",
  );
  if (isWindows) {
    writeFile(
      path.join(bin, "npm.cmd"),
      `@"${process.execPath}" "${echo}" %*\r\n`,
    );
  } else {
    writeFile(
      path.join(bin, "npm"),
      `#!/bin/sh\nexec "${process.execPath}" "${echo}" "$@"\n`,
      true,
    );
  }
  const args = ["--prefix", path.join(bin, "a b&c|d"), "run", "type check"];
  const result = defaultRunner({
    tool: "npm",
    args,
    env: { ...process.env, PATH: bin, Path: undefined },
    capture: true,
  });
  assert.equal(result.found, true);
  assert.equal(result.status, 0, result.output);
  assert.deepEqual(JSON.parse(result.output), args);
});

// On POSIX the child dies of the signal; on Windows the kill terminates it with exit code 1. Both must read 1.
test("the default runner counts a signal death as status 1", () => {
  const result = defaultRunner({
    tool: "node",
    args: ["-e", "process.kill(process.pid, 'SIGTERM')"],
    env: { ...process.env },
    capture: true,
  });
  assert.equal(result.found, true);
  assert.equal(result.status, 1);
});
