import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import {
  existsSync,
  mkdirSync,
  mkdtempSync,
  readdirSync,
  readFileSync,
  realpathSync,
  rmSync,
  statSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { delimiter, join, relative } from "node:path";
import { after, test, type TestContext } from "node:test";
import { fileURLToPath, pathToFileURL } from "node:url";
import { inCoverageScope } from "../coverage/scope.ts";
import { findCommand } from "../plugin/common.ts";
import { isProductionSource } from "../scripts/mutation-source.ts";
import {
  canonicalRepoRoot,
  createProfile,
  createStageFixture,
  createValidateFixture,
  disposeSuite,
  findRepoRoot,
  getSuite,
  gitExecutable,
  maintainerSources,
  systemDirs,
  runValidateFixture,
  stageLogPath,
  validateArgvMismatch,
  type Invocation,
  type Profile,
} from "./testing/harness.ts";
import { withFixturePath } from "./testing/shell-support.ts";

after(disposeSuite);

test("the launcher harness is test support for coverage and mutation", () => {
  const source = findRepoRoot();
  const harness = fileURLToPath(
    new URL("./testing/harness.ts", import.meta.url),
  );
  const support = fileURLToPath(
    new URL("./testing/shell-support.ts", import.meta.url),
  );
  assert.equal(
    existsSync(join(source, ".github/tools/src/entrypoint/harness.ts")),
    false,
  );
  const production = ".github/tools/src/entrypoint/interpreters.ts";
  for (const path of [harness, support]) {
    assert.ok(statSync(path).isFile());
    const file = relative(source, path).replaceAll("\\", "/");
    assert.equal(inCoverageScope(file), false, file);
    assert.equal(isProductionSource(file, "stryker"), false, file);
  }
  assert.equal(inCoverageScope(production), true);
  assert.equal(isProductionSource(production, "stryker"), true);
});

const commonURL = pathToFileURL(
  join(findRepoRoot(), ".github/tools/src/plugin/common.ts"),
).href;

interface Started {
  name: string;
  file: string | undefined;
  status: number | null;
  stdout: string;
  stderr: string;
}

/**
 * Starts each named command from inside the profile's environment, the way the
 * maintainer CLI does: resolved by bare name with findCommand, then started
 * through spawnPlan, which goes through cmd.exe for a Windows .cmd launcher.
 */
function startFromProfile(
  p: Profile,
  calls: { name: string; args: string[] }[],
  invocation: Pick<Invocation, "copilotListExit"> = {},
): Started[] {
  const probe = p.path("probe.mjs");
  p.write(
    probe,
    `import { spawnSync } from "node:child_process";
import { findCommand, spawnPlan } from ${JSON.stringify(commonURL)};
const started = [];
for (const { name, args } of JSON.parse(process.argv[2])) {
  const file = findCommand(name, process.env, process.platform);
  if (file === undefined) { started.push({ name, file, status: null, stdout: "", stderr: "" }); continue; }
  const plan = spawnPlan(file, args, process.platform, process.env);
  const r = spawnSync(plan.file, plan.args, { encoding: "utf8", windowsVerbatimArguments: plan.windowsVerbatimArguments });
  started.push({ name, file, status: r.status, stdout: r.stdout, stderr: r.stderr });
}
process.stdout.write(JSON.stringify(started));
`,
  );
  const r = p.run({
    ...invocation,
    script: probe,
    args: [JSON.stringify(calls)],
  });
  assert.equal(r.exit, 0, r.stdout + r.stderr);
  return JSON.parse(r.stdout) as Started[];
}

test("fakes on PATH resolve by bare name and record their argv", (t) => {
  const p = createProfile(t);
  const hostile = ["a b", "x&y"];
  const started = startFromProfile(p, [
    ...["claude", "copilot", "codex", "agy", "npm", "go"].map((name) => ({
      name,
      args: hostile,
    })),
    { name: "copilot", args: ["plugin", "list"] },
  ]);
  const suite = getSuite();
  for (const s of started) {
    assert.ok(s.file, `${s.name} is not on the profile's PATH`);
    if (process.platform === "win32")
      assert.match(s.file, /\.cmd$/, `${s.name} resolved to ${s.file}`);
    const dir = ["npm", "go"].includes(s.name) ? suite.toolBin : suite.fakeBin;
    assert.equal(relative(dir, s.file).startsWith(".."), false, s.file);
  }
  const byName = (name: string, index = 0) =>
    started.filter((s) => s.name === name)[index]!;
  for (const name of ["claude", "copilot", "codex", "agy"]) {
    const s = byName(name);
    assert.equal(s.status, 97, s.stdout + s.stderr);
    assert.equal(s.stderr, `forbidden fake ${name} command\n`);
  }
  // Each fake parses its arguments itself, so a mangled argument would reach
  // its error message.
  assert.equal(byName("npm").status, 97, byName("npm").stderr);
  assert.equal(byName("npm").stderr, "fixture npm: unsupported argument a b\n");
  assert.equal(byName("go").status, 7, byName("go").stderr);
  assert.equal(byName("go").stderr, "unsupported fixture build command\n");
  const listing = byName("copilot", 1);
  assert.equal(listing.status, 0, listing.stderr);
  assert.equal(listing.stdout, "• agentics@muehz (v1.0.0)\n");
  assert.equal(
    p.read(p.log),
    [
      "claude\ta b\tx&y",
      "copilot\ta b\tx&y",
      "codex\ta b\tx&y",
      "agy\ta b\tx&y",
      "copilot\tplugin\tlist",
      "",
    ].join("\n"),
  );
});

test("a failing copilot listing exits with the requested status", (t) => {
  const p = createProfile(t);
  const [listing] = startFromProfile(
    p,
    [{ name: "copilot", args: ["plugin", "list"] }],
    { copilotListExit: "3" },
  );
  assert.equal(listing!.status, 3, listing!.stderr);
  assert.equal(listing!.stdout, "");
  assert.equal(p.read(p.log), "copilot\tplugin\tlist\n");
});

test("the real git stays reachable from the profile's PATH", (t) => {
  const p = createProfile(t);
  const [git] = startFromProfile(p, [{ name: "git", args: ["--version"] }]);
  assert.ok(git!.file, "git is not on the profile's PATH");
  assert.equal(git!.status, 0, git!.stderr);
  assert.match(git!.stdout, /^git version /);
});

test("the profile keeps its explicit PATH override and removes case aliases", (t) => {
  const p = createProfile(t);
  const script = p.path("path-probe.mjs");
  p.write(
    script,
    'console.log(JSON.stringify(Object.entries(process.env).filter(([key]) => key.toUpperCase() === "PATH")));',
  );
  const result = p.run({
    script,
    env: { PATH: "", Path: "ignored", pAtH: "also ignored" },
  });
  assert.equal(result.exit, 0, result.stderr);
  assert.deepEqual(
    Object.entries(result.env).filter(([key]) => key.toUpperCase() === "PATH"),
    [["PATH", ""]],
  );
  const seen: [string, string][] = JSON.parse(result.stdout);
  assert.equal(seen.length, 1);
  assert.equal(seen[0]![1], "");
});

test(
  "the POSIX fixture Git launcher quotes an executable path with an apostrophe",
  {
    skip: process.platform === "win32" && "requires the POSIX launcher",
  },
  (t) => {
    const root = mkdtempSync(join(tmpdir(), "agentics-harness-quote-"));
    t.after(() => rmSync(root, { recursive: true, force: true }));
    const bin = join(root, "git's bin");
    mkdirSync(bin);
    writeFileSync(join(bin, "git"), '#!/bin/sh\nprintf "%s\\n" "$1"\n', {
      mode: 0o755,
    });
    const module = new URL("./testing/harness.ts", import.meta.url).href;
    const probe = `
import { execFileSync } from "node:child_process";
import { join } from "node:path";
import { disposeSuite, getSuite } from ${JSON.stringify(module)};
try {
  process.stdout.write(execFileSync(join(getSuite().toolBin, "git"), ["it's one argument"]));
} finally {
  disposeSuite();
}`;
    const result = spawnSync(
      process.execPath,
      ["--input-type=module", "--eval", probe],
      {
        encoding: "utf8",
        env: withFixturePath(
          process.env,
          [bin, process.env.PATH ?? ""].join(delimiter),
        ),
      },
    );
    assert.ifError(result.error);
    assert.equal(result.status, 0, result.stderr);
    assert.equal(result.stdout, "it's one argument\n");
  },
);

test("a run starts the checkout's cli.ts with this Node", (t) => {
  const r = createProfile(t).run({ args: ["--help"] });
  assert.equal(r.exit, 0, r.stdout + r.stderr);
  assert.equal(r.command, process.execPath);
  const first = r.stdout.split("\n")[0]!;
  assert.ok(first.startsWith("Usage:"), first);
  assert.ok(first.includes("node .github/tools/src/plugin/cli.ts"), first);
});

test("omitTools go leaves no go on PATH", (t) => {
  const p = createProfile(t);
  const full = p.run({ args: ["--help"] });
  assert.ok(
    findCommand("go", full.env, process.platform),
    "the fake go is missing from a run that omits nothing",
  );
  const r = p.run({ args: ["--help"], omitTools: ["go"] });
  assert.equal(r.exit, 0, r.stdout + r.stderr);
  assert.equal(r.command, process.execPath);
  assert.equal(findCommand("go", r.env, process.platform), undefined);
  for (const tool of ["git", "npm", "claude"])
    assert.ok(findCommand(tool, r.env, process.platform), `${tool} omitted`);
});

test("findRepoRoot finds the checkout by the maintainer CLI", (t) => {
  const root = findRepoRoot(import.meta.dirname);
  assert.ok(existsSync(join(root, ".github/tools/src/plugin/cli.ts")));
  const scratch = realpathSync(mkdtempSync(join(tmpdir(), "agentics-root-")));
  t.after(() => rmSync(scratch, { recursive: true, force: true }));
  mkdirSync(join(scratch, ".github/tools/src/plugin"), { recursive: true });
  writeFileSync(join(scratch, ".github/tools/src/plugin/cli.ts"), "");
  mkdirSync(join(scratch, "a/b"), { recursive: true });
  assert.equal(findRepoRoot(join(scratch, "a/b")), scratch);
  // plugin.sh alone no longer marks a checkout.
  const shellOnly = join(scratch, "shell-only");
  mkdirSync(join(shellOnly, ".github/scripts"), { recursive: true });
  writeFileSync(join(shellOnly, ".github/scripts/plugin.sh"), "");
  rmSync(join(scratch, ".github/tools"), { recursive: true });
  assert.throws(() => findRepoRoot(shellOnly), /plugin\/cli\.ts/);
});

/** Every .ts file under the plugin directory, relative to the checkout. */
function pluginSources(root: string): string[] {
  return readdirSync(join(root, ".github/tools/src/plugin"), {
    recursive: true,
  })
    .map(String)
    .filter((name) => name.endsWith(".ts"))
    .map((name) => join(".github/tools/src/plugin", name));
}

function assertCopiesPluginSources(fixtureRoot: string): void {
  const source = findRepoRoot();
  const expected = [
    ...pluginSources(source),
    // Reached only through an import of floor.ts.
    join(".github/tools/src/scripts/node-range.ts"),
  ];
  for (const file of expected)
    assert.ok(
      readFileSync(join(source, file)).equals(
        readFileSync(join(fixtureRoot, file)),
      ),
      `${file} differs from the checkout's copy`,
    );
}

test("the independent validate and stage fixtures run the CLI from one source inventory", (t) => {
  const p = createProfile(t);
  const validate = createValidateFixture(t);
  const stage = createStageFixture(t);
  assert.notEqual(realpathSync(validate.root), realpathSync(stage.root));
  const source = findRepoRoot();
  const inventory = maintainerSources(source);
  const dependency = join(".github/tools/src/scripts", "node-range.ts");
  assert.deepEqual(inventory, [...new Set(inventory)].sort());
  assert.ok(inventory.includes(dependency));
  for (const root of [validate.root, stage.root]) {
    assertCopiesPluginSources(root);
    for (const file of inventory) {
      if (file === join(".github/tools/src/conformance", "cli.ts")) continue;
      assert.deepEqual(
        readFileSync(join(root, file)),
        readFileSync(join(source, file)),
        file,
      );
    }
  }

  assert.match(
    readFileSync(
      join(validate.root, ".github/tools/src/conformance/cli.ts"),
      "utf8",
    ),
    /AGX_STUB_CONFORMANCE_LOG/,
  );
  assert.equal(
    readFileSync(stage.path(".github/tools/src/conformance/cli.ts"), "utf8"),
    'console.log("PASS manifests");\n',
  );
  assert.equal(
    readFileSync(stage.path("bin/agx.exe"), "utf8"),
    "fixture launcher",
  );
  if (process.platform !== "win32") {
    assert.notEqual(statSync(join(validate.root, dependency)).mode & 0o111, 0);
    assert.equal(statSync(stage.path(dependency)).mode & 0o111, 0);
    assert.notEqual(
      statSync(stage.path("hooks/bin/agx-linux-amd64")).mode & 0o111,
      0,
    );
  }

  const { result } = runValidateFixture(p, validate, ["--help"]);
  assert.equal(result.exit, 0, result.stdout + result.stderr);
  assert.ok(result.stdout.startsWith("Usage: node "), result.stdout);
  validate.remove(dependency);
  const missing = p.run({ script: validate.cli, args: ["--help"] });
  assert.notEqual(missing.exit, 0, missing.stdout);
  assert.match(missing.stderr, /ERR_MODULE_NOT_FOUND/);
  assert.match(missing.stderr, /node-range\.ts/);
  const other = stage.run(p, { args: ["--help"] });
  assert.ok(other.stdout.startsWith("Usage: node "), other.stdout);
});

test("a stage fixture run passes the Node floor check", (t) => {
  const p = createProfile(t),
    f = createStageFixture(t);
  // --vscode reaches its instructions only after the floor check, which runs first, passed.
  const r = p.run({ script: f.cli(), args: ["--vscode"] });
  assert.ok(!r.stderr.includes("Node range"), r.stderr);
  assert.equal(r.exit, 0, r.stdout + r.stderr);
  assert.ok(r.stdout.includes("local plugin registration"), r.stdout);
  const root = JSON.parse(
    readFileSync(join(findRepoRoot(), "package.json"), "utf8"),
  ) as { engines: unknown };
  const fixture = JSON.parse(
    readFileSync(join(f.root, "package.json"), "utf8"),
  ) as { engines: unknown };
  assert.deepEqual(fixture.engines, root.engines);
});

test("systemDirs splits its override on the platform's delimiter", () => {
  assert.deepEqual(
    systemDirs({ AGX_ENTRYPOINT_SYSTEM_DIRS: "/a:/b" }, "linux"),
    ["/a", "/b"],
  );
  assert.deepEqual(
    systemDirs({ AGX_ENTRYPOINT_SYSTEM_DIRS: "C:\\a;D:\\b" }, "win32"),
    ["C:\\a", "D:\\b"],
  );
  // An empty override counts as unset.
  assert.deepEqual(systemDirs({ AGX_ENTRYPOINT_SYSTEM_DIRS: "" }, "linux"), [
    "/usr/bin",
    "/bin",
  ]);
});

test("a Windows PATH without git is a clear error", () => {
  assert.throws(
    () => gitExecutable({ PATHEXT: ".EXE" }, "win32", ""),
    /no git on the PATH/,
  );
  assert.equal(gitExecutable({}, "linux", ""), "/usr/bin/git");
});

test("the validate fixture package.json keeps stubs and the engines", (t) => {
  const f = createValidateFixture(t);
  const root = JSON.parse(
    readFileSync(join(findRepoRoot(), "package.json"), "utf8"),
  ) as { engines: unknown };
  const fixture = JSON.parse(
    readFileSync(join(f.root, "package.json"), "utf8"),
  ) as { scripts: unknown; engines: unknown };
  assert.deepEqual(fixture.scripts, { "test:typecheck": "tsc" });
  assert.deepEqual(fixture.engines, root.engines);
});

function validateNode(t: TestContext, env: NodeJS.ProcessEnv = {}) {
  const p = createProfile(t),
    f = createValidateFixture(t);
  const { result, calls } = runValidateFixture(p, f, ["--validate"], env);
  const log = existsSync(stageLogPath(p)) ? p.read(stageLogPath(p)) : "";
  return { f, result, calls, log };
}

test("validate fixture stages run under the maintainer CLI", (t) => {
  const { f, result: r, calls, log } = validateNode(t);
  assert.equal(r.exit, 0, r.stdout + r.stderr);
  assert.equal(log, "typecheck\ntools-test\nboundaries\n");
  assert.equal(
    validateArgvMismatch(calls, f, ["hooks"], ["all", "--exclude", "hooks"]),
    "",
  );
  assert.equal(canonicalRepoRoot(f), realpathSync(f.root));
});

test("validate fixture typecheck stub fails on request", (t) => {
  const { result: r, log } = validateNode(t, {
    AGX_STUB_TYPECHECK_FAIL: "1",
  });
  assert.equal(r.exit, 1, r.stdout + r.stderr);
  assert.equal(log, "typecheck\n");
});

test("validate fixture tools-test stub fails on request", (t) => {
  const { result: r, log } = validateNode(t, {
    AGX_STUB_TOOLS_TEST_FAIL: "1",
  });
  assert.equal(r.exit, 1, r.stdout + r.stderr);
  assert.equal(log, "typecheck\ntools-test\n");
});

test("removing the typecheck launcher skips the typecheck", (t) => {
  const p = createProfile(t),
    f = createValidateFixture(t);
  f.remove("node_modules/.bin/tsc");
  const { result: r } = runValidateFixture(p, f, ["--validate"]);
  assert.equal(r.exit, 0, r.stdout + r.stderr);
  assert.ok(r.stdout.includes("Typecheck skipped"), r.stdout);
  assert.equal(p.read(stageLogPath(p)), "tools-test\nboundaries\n");
});
