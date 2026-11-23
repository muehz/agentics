import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
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
import {
  basename,
  delimiter,
  dirname,
  isAbsolute,
  join,
  relative,
  resolve,
} from "node:path";
import type { TestContext } from "node:test";
import { pathToFileURL } from "node:url";
import { parse } from "yaml";
import { fixtureGitEnv, git } from "../../conformance/testing/git-checkout.ts";
import { findCommand, type Env } from "../../plugin/common.ts";
import { quotePosix, withFixturePath } from "./shell-support.ts";

export const defaultCopilotListing = "• agentics@muehz (v1.0.0)";

const CLI = ".github/tools/src/plugin/cli.ts";
const PLUGIN_SOURCES = ".github/tools/src/plugin";

export function findRepoRoot(start = process.cwd()): string {
  let directory = start;
  while (!existsSync(join(directory, CLI))) {
    const parent = dirname(directory);
    if (parent === directory)
      throw new Error(`No checkout holding ${CLI} above ${start}`);
    directory = parent;
  }
  return directory;
}

export function loadConformanceAction(): ReturnType<typeof parse> {
  return parse(
    readFileSync(
      join(getSuite().repoRoot, ".github/actions/conformance/action.yml"),
      "utf8",
    ),
  );
}

interface Suite {
  repoRoot: string;
  /** The checkout's maintainer CLI, which a run starts unless it names another script. */
  cli: string;
  tmpRoot: string;
  fakeBin: string;
  toolBin: string;
  cacheDir: string;
  goShim: string;
  /** The Node scripts behind the launchers. */
  fakeScripts: string;
}
let suite: Suite | undefined;
// Captured at load, because some cases empty PATH before the suite is built.
const inheritedPath = process.env.PATH ?? "";

/**
 * The git on `searchPath`. POSIX falls back to /usr/bin/git; Windows has no
 * such default, so a missing git is an error there rather than a path that
 * later fails to start.
 */
export function gitExecutable(
  env: Env = process.env,
  platform: NodeJS.Platform = process.platform,
  searchPath: string = inheritedPath,
): string {
  const found = findCommand(
    "git",
    { PATH: searchPath, PATHEXT: env.PATHEXT },
    platform,
  );
  if (found !== undefined) return found;
  if (platform === "win32")
    throw new Error(
      "the entrypoint harness found no git on the PATH the tests started with",
    );
  return "/usr/bin/git";
}

/** The path a launcher named `name` has in `dir`: Windows finds a command only through a PATHEXT extension. */
export function launcherPath(
  dir: string,
  name: string,
  platform: NodeJS.Platform = process.platform,
): string {
  return join(dir, platform === "win32" ? `${name}.cmd` : name);
}

/**
 * Writes a command named `name` into `dir` that runs the Node `script` with
 * `fixedArgs` before its own arguments. On POSIX it is an executable file whose
 * interpreter line names this Node by absolute path, so it runs on a PATH
 * without node. On Windows it is a `.cmd` file, which callers start through
 * cmd.exe (spawnPlan), never directly.
 */
export function writeLauncher(
  dir: string,
  name: string,
  script: string,
  fixedArgs: string[] = [],
  platform: NodeJS.Platform = process.platform,
): string {
  const file = launcherPath(dir, name, platform);
  mkdirSync(dir, { recursive: true });
  if (platform === "win32") {
    const line = [`"${process.execPath}"`, `"${script}"`, ...fixedArgs, "%*"];
    writeFileSync(file, `@${line.join(" ")}\r\n`);
    return file;
  }
  // The kernel splits an interpreter line at whitespace.
  assert.ok(
    !/\s/.test(process.execPath),
    `cannot name ${process.execPath} in an interpreter line`,
  );
  const splice = fixedArgs.length
    ? `process.argv.splice(2, 0, ...${JSON.stringify(fixedArgs)});\n`
    : "";
  writeFileSync(
    file,
    `#!${process.execPath}\n${splice}import(${JSON.stringify(pathToFileURL(script).href)}).catch(error => { console.error(error); process.exitCode = 1; });\n`,
    { mode: 0o755 },
  );
  return file;
}

/** Writes an executable adapter to the public TypeScript checker, preserving its argv and exit status. */
export function writeConformanceLauncher(path: string): void {
  const cli = pathToFileURL(
    join(findRepoRoot(), ".github/tools/src/conformance/cli.ts"),
  ).href;
  writeFileSync(
    path,
    `#!${process.execPath}\nimport(${JSON.stringify(cli)}).catch(error => { console.error(error); process.exitCode = 1; });\n`,
    { mode: 0o755 },
  );
}

// Stands in for .github/tools/src/conformance/cli.ts, which every validate mode runs.
// It has no import statement and no TypeScript syntax, so Node runs it as
// CommonJS or as an ES module. Each call appends
// its argv and the AGX_CONFORMANCE_SHORT value it ran with to
// AGX_STUB_CONFORMANCE_LOG as one JSON line, so a run that calls it twice
// keeps both records. A hooks call checks and starts the trusted verifier the
// entrypoint exported, as the real hooks domain does.
const stubConformance = String.raw`const { appendFileSync } = process.getBuiltinModule("node:fs");
const { spawnSync } = process.getBuiltinModule("node:child_process");
const args = process.argv.slice(2);
const log = process.env.AGX_STUB_CONFORMANCE_LOG;
if (log) appendFileSync(log, JSON.stringify({ argv: args, short: process.env.AGX_CONFORMANCE_SHORT ?? "" }) + "\n");
if (args[0] === "hooks") {
  const verifier = process.env.AGX_HOOK_VERIFIER;
  if (!verifier || verifier !== process.env.AGX_STUB_EXPECT_VERIFIER) {
    console.error("unexpected trusted verifier path", verifier); process.exit(1);
  }
  const checked = spawnSync(process.execPath, [verifier], { encoding: "utf8" });
  if (checked.error || checked.status !== 0 || checked.stdout !== "trusted fixture verifier\n") {
    console.error("trusted verifier did not run", checked.error, checked.stdout); process.exit(1);
  }
  console.log("PASS hooks");
  if (process.env.AGX_STUB_HOOKS_FAIL) { console.log("FAIL stub hooks"); process.exitCode = 1; }
} else {
  console.log("PASS manifests");
  console.log("PASS tools");
  if (process.env.AGX_STUB_CONFORMANCE_FAIL) { console.log("FAIL stub"); process.exitCode = 1; }
}
`;

// Stands in for .github/tools/src/boundaries/cli.ts. It records that it ran and fails
// when AGX_STUB_BOUNDARIES_FAIL is set.
const stubBoundaries = String.raw`const { appendFileSync } = process.getBuiltinModule("node:fs");
if (process.env.AGX_STUB_STAGE_LOG) appendFileSync(process.env.AGX_STUB_STAGE_LOG, "boundaries\n");
console.log("boundaries: 0 failures");
if (process.env.AGX_STUB_BOUNDARIES_FAIL) process.exitCode = 1;
`;

// Stands in for the typecheck and the tools tests: it appends its first
// argument to the stage log and then fails when the variable its second
// argument names is set, so a test can see which later stages of strict
// --validate ran and can fail exactly one of them.
const stageMarker = String.raw`import { appendFileSync } from "node:fs";
const [name, failVar] = process.argv.slice(2);
if (process.env.AGX_STUB_STAGE_LOG) appendFileSync(process.env.AGX_STUB_STAGE_LOG, name + "\n");
if (process.env[failVar]) process.exitCode = 1;
`;

// A fake plugin CLI. It appends its name and argv, tab-separated, to
// AGX_FAKE_CLI_LOG and refuses every command; copilot also answers
// "plugin list" from AGX_FAKE_COPILOT_LIST_OUTPUT, or exits with
// AGX_FAKE_COPILOT_LIST_EXIT when that is set and not 0.
function fakeCLI(name: string): string {
  return String.raw`import { appendFileSync, writeSync } from "node:fs";
const name = ${JSON.stringify(name)};
const args = process.argv.slice(2);
if (process.env.AGX_FAKE_CLI_LOG) appendFileSync(process.env.AGX_FAKE_CLI_LOG, [name, ...args].join("\t") + "\n");
if (name === "copilot" && args[0] === "plugin" && args[1] === "list") {
  const exit = process.env.AGX_FAKE_COPILOT_LIST_EXIT || "0";
  if (exit !== "0") process.exit(Number(exit));
  writeSync(1, (process.env.AGX_FAKE_COPILOT_LIST_OUTPUT ?? "") + "\n");
} else {
  writeSync(2, "forbidden fake " + name + " command\n");
  process.exitCode = 97;
}
`;
}

// Stands in for npm on the tool PATH, so strict --validate runs the validate
// fixture's package scripts without a real npm install. It accepts only the
// two forms the validate stage uses, `npm --prefix <dir> run <name>` and
// `npm --prefix <dir> test`. It runs that script of <dir>/package.json from
// <dir> with <dir>/node_modules/.bin first on PATH, as npm does, but without a
// shell: the script is a command name and plain words.
function fakeNpm(commonURL: string): string {
  return String.raw`import { readFileSync, writeSync } from "node:fs";
import { spawnSync } from "node:child_process";
import path from "node:path";
import { findCommand, spawnPlan } from ${JSON.stringify(commonURL)};
import { withFixturePath } from ${JSON.stringify(new URL("./shell-support.ts", import.meta.url).href)};
const fail = (code, message) => { writeSync(2, "fixture npm: " + message + "\n"); process.exit(code); };
const args = process.argv.slice(2);
let prefix = ".", name;
for (let i = 0; i < args.length; ) {
  if (args[i] === "--prefix" && i + 1 < args.length) { prefix = args[i + 1]; i += 2; }
  else if (args[i] === "run" && i + 1 < args.length) { name = args[i + 1]; i += 2; }
  else if (args[i] === "test") { name = "test"; i += 1; }
  else fail(97, "unsupported argument " + args[i]);
}
let script;
try {
  const scripts = JSON.parse(readFileSync(path.resolve(prefix, "package.json"), "utf8")).scripts ?? {};
  if (Object.hasOwn(scripts, name)) script = scripts[name];
} catch {}
if (typeof script !== "string") fail(1, "no script " + name + " in " + prefix + "/package.json");
const dir = path.resolve(prefix);
const env = withFixturePath(process.env, [path.join(dir, "node_modules", ".bin"), process.env.PATH ?? ""].join(path.delimiter));
const [command, ...words] = script.trim().split(/\s+/);
const file = findCommand(command, env, process.platform);
if (file === undefined) fail(127, command + ": command not found");
let plan;
try { plan = spawnPlan(file, words, process.platform, env); } catch (error) { fail(1, error.message); }
const result = spawnSync(plan.file, plan.args, { cwd: dir, env, stdio: "inherit", windowsVerbatimArguments: plan.windowsVerbatimArguments });
if (result.error) fail(1, result.error.message);
process.exitCode = result.status ?? 1;
`;
}

/** The checkout's engines field. A fixture needs it, because the maintainer CLI checks engines.node before every action but help. */
function rootEngines(repoRoot: string): unknown {
  return (
    JSON.parse(readFileSync(join(repoRoot, "package.json"), "utf8")) as {
      engines?: unknown;
    }
  ).engines;
}

function fixturePackageJSON(
  scripts: Record<string, string>,
  engines?: unknown,
): string {
  return (
    JSON.stringify({
      private: true,
      scripts,
      ...(engines === undefined ? {} : { engines }),
    }) + "\n"
  );
}

// Stands in for go on the tool PATH. The entrypoint builds only the trusted
// hook verifier, so the shim builds that one target, and only in a module
// holding the .agentics-entrypoint-hooktool marker; removing the marker makes the
// build fail as a broken source would.
function writeBuildShim(path: string): void {
  const program = String.raw`const fs = require("node:fs");
const path = require("node:path");
const args = process.argv.slice(2);
if (args[0] !== "build") { console.error("unsupported fixture build command"); process.exit(7); }
const moduleIndex = args.indexOf("-C"), outputIndex = args.indexOf("-o");
if (moduleIndex < 0 || outputIndex < 0 || !args[moduleIndex + 1] || !args[outputIndex + 1]) process.exit(7);
const moduleRoot = path.resolve(args[moduleIndex + 1]), destination = args[outputIndex + 1], target = args.at(-1);
if (target !== "./cmd/hooktool" || !fs.existsSync(path.join(moduleRoot, ".agentics-entrypoint-hooktool"))) {
  console.error("no controlled build fixture for", moduleRoot, target); process.exit(1);
}
fs.mkdirSync(path.dirname(destination), { recursive: true });
fs.writeFileSync(destination, "#!" + node + "\nconsole.log(\"trusted fixture verifier\");\n", { mode: 0o755 });
`;
  writeFileSync(
    path,
    `const node = ${JSON.stringify(process.execPath)};\n${program}`,
    { mode: 0o755 },
  );
}

export function getSuite(): Suite {
  if (suite) return suite;
  const repoRoot = findRepoRoot();
  // Match the shell's physical cache paths on macOS, where /var is a symlink.
  const tmpRoot = realpathSync(
    mkdtempSync(join(tmpdir(), "agentics-entrypoint-")),
  );
  const fakeBin = join(tmpRoot, "fake-bin"),
    toolBin = join(tmpRoot, "tool-bin"),
    cacheDir = join(tmpRoot, "cache"),
    fakeScripts = join(tmpRoot, "fake-scripts");
  for (const dir of [fakeBin, toolBin, cacheDir, fakeScripts]) mkdirSync(dir);
  for (const name of ["claude", "copilot", "codex", "agy"]) {
    const script = join(fakeScripts, `${name}.mjs`);
    writeFileSync(script, fakeCLI(name));
    writeLauncher(fakeBin, name, script);
  }
  const npm = join(fakeScripts, "npm.mjs");
  writeFileSync(
    npm,
    fakeNpm(pathToFileURL(join(repoRoot, PLUGIN_SOURCES, "common.ts")).href),
  );
  writeLauncher(toolBin, "npm", npm);
  writeFileSync(join(fakeScripts, "stage-marker.mjs"), stageMarker);
  const goShim = join(tmpRoot, "go-shim.cjs");
  writeBuildShim(goShim);
  writeLauncher(toolBin, "go", goShim);
  // On POSIX a run that omits a tool finds git and node only in this
  // directory (pathEntries). On Windows a run starts Node by its own path, and
  // git stays in its install directory, which the PATH names (systemDirs),
  // because Git for Windows finds its runtime relative to the executable.
  if (process.platform !== "win32")
    for (const [name, executable] of [
      ["git", gitExecutable()],
      ["node", process.execPath],
    ])
      writeFileSync(
        join(toolBin, name!),
        `#!/bin/sh\nexec ${quotePosix(executable!)} "$@"\n`,
        { mode: 0o755 },
      );
  suite = {
    repoRoot,
    cli: join(repoRoot, CLI),
    tmpRoot,
    fakeBin,
    toolBin,
    cacheDir,
    goShim,
    fakeScripts,
  };
  return suite;
}

export function disposeSuite(): void {
  if (suite) rmSync(suite.tmpRoot, { recursive: true, force: true });
  suite = undefined;
}

/**
 * The system directories a profile's PATH ends with: /usr/bin and /bin on
 * POSIX, and on Windows the directories holding git and cmd.exe.
 * AGX_ENTRYPOINT_SYSTEM_DIRS, split on the platform's path delimiter, replaces
 * them, so a host whose system directories hold a go or node can point a run
 * elsewhere. `gitSearchPath` is where Windows looks for git.
 */
export function systemDirs(
  env: Env = process.env,
  platform: NodeJS.Platform = process.platform,
  gitSearchPath: string = inheritedPath,
): string[] {
  const override = env.AGX_ENTRYPOINT_SYSTEM_DIRS;
  if (override !== undefined && override !== "")
    return override.split(platform === "win32" ? ";" : ":");
  if (platform !== "win32") return ["/usr/bin", "/bin"];
  const comspec =
    env.ComSpec ||
    env.COMSPEC ||
    join(env.SystemRoot || "C:\\Windows", "System32", "cmd.exe");
  return [
    ...new Set([
      dirname(gitExecutable(env, platform, gitSearchPath)),
      dirname(comspec),
    ]),
  ];
}

// The variables a Windows child needs to start at all (SystemRoot), to find
// cmd.exe and commands by extension, and to place temporary files.
const windowsBaseVariables = [
  "SystemRoot",
  "ComSpec",
  "PATHEXT",
  "TEMP",
  "TMP",
];

function baseEnv(
  env: Env = process.env,
  platform: NodeJS.Platform = process.platform,
): NodeJS.ProcessEnv {
  const base: NodeJS.ProcessEnv = {};
  if (env.TMPDIR) base.TMPDIR = env.TMPDIR;
  if (platform === "win32")
    for (const name of windowsBaseVariables) {
      // process.env is case-insensitive on Windows; a plain record is not.
      const value =
        env[name] ??
        Object.entries(env).find(
          ([key]) => key.toUpperCase() === name.toUpperCase(),
        )?.[1];
      if (value !== undefined) base[name] = value;
    }
  return base;
}

export interface Invocation {
  args?: string[];
  copilotListing?: string;
  copilotListExit?: string;
  noCLIs?: boolean;
  extraPath?: string;
  /** Tools the run's tool directory leaves out, so the run finds none of them. */
  omitTools?: ("go" | "node")[];
  env?: NodeJS.ProcessEnv;
  /** A Node script to run instead of the maintainer CLI. */
  script?: string;
}
export interface Result {
  exit: number;
  stdout: string;
  stderr: string;
  log: string;
  /** The program the run started: this Node. */
  command: string;
  /** The child's whole environment. */
  env: NodeJS.ProcessEnv;
}

export class Profile {
  readonly dir: string;
  readonly home: string;
  readonly xdg: string;
  readonly appdata: string;
  readonly log: string;
  constructor(t: TestContext) {
    this.dir = realpathSync(
      mkdtempSync(join(tmpdir(), "agentics-entrypoint-case-")),
    );
    t.after(() => rmSync(this.dir, { recursive: true, force: true }));
    this.home = this.path("home");
    this.xdg = this.path("xdg");
    this.appdata = this.path("appdata");
    this.log = this.path("commands.log");
    for (const dir of [this.home, this.xdg, this.appdata]) mkdirSync(dir);
    writeFileSync(this.log, "");
  }
  path(...parts: string[]): string {
    return join(this.dir, ...parts);
  }
  read(path: string): string {
    return readFileSync(path, "utf8");
  }
  write(path: string, content: string): void {
    mkdirSync(dirname(path), { recursive: true });
    writeFileSync(path, content);
  }
  /** The PATH entries of a run, without the omitted tools. */
  private pathEntries(inv: Invocation): string[] {
    const s = getSuite();
    const omit = inv.omitTools ?? [];
    const head = [inv.extraPath, inv.noCLIs ? undefined : s.fakeBin];
    let tail: string[];
    if (!omit.length) tail = [s.toolBin, ...systemDirs()];
    // A run that omits a tool leaves out the POSIX system directories, which
    // may hold a host go or node that would stand in for the omitted one; the
    // tool directory holds git there. On Windows the system directories are
    // those of git and cmd.exe.
    else
      tail = [
        this.toolBinWithout(omit),
        ...(process.platform === "win32" ? systemDirs() : []),
      ];
    return [...head, ...tail].filter((entry): entry is string => !!entry);
  }
  run(inv: Invocation = {}): Result {
    const s = getSuite();
    const omit = inv.omitTools ?? [];
    const path = this.pathEntries(inv).join(delimiter);
    const childEnv: NodeJS.ProcessEnv = {
      ...fixtureGitEnv(baseEnv()),
      PATH: path,
      HOME: this.home,
      XDG_CONFIG_HOME: this.xdg,
      APPDATA: this.appdata,
      XDG_CACHE_HOME: s.cacheDir,
      AGX_FAKE_CLI_LOG: this.log,
      AGX_FAKE_COPILOT_LIST_OUTPUT: inv.copilotListing || defaultCopilotListing,
      AGX_FAKE_COPILOT_LIST_EXIT: inv.copilotListExit ?? "",
      ...inv.env,
    };
    const env = withFixturePath(childEnv, childEnv.PATH);
    for (const tool of omit) {
      // Kept although the private PATH should make it unreachable: a run that
      // found the omitted tool would test the wrong case.
      const found = findCommand(tool, env);
      assert.equal(
        found,
        undefined,
        `the run omits ${tool}, but the host supplies ${found}; this case cannot run here`,
      );
    }
    // Without the fakes no plugin CLI may be reachable at all: one found on
    // the system directories would be the host's real CLI.
    if (inv.noCLIs)
      for (const cli of ["claude", "copilot", "codex", "agy"]) {
        const found = findCommand(cli, env);
        assert.equal(
          found,
          undefined,
          `the run has no CLIs, but the host supplies ${found}; this case cannot run here`,
        );
      }
    const command = process.execPath;
    const entry = inv.script ?? s.cli;
    const result = spawnSync(command, [entry, ...(inv.args ?? [])], {
      cwd: this.dir,
      env,
      encoding: "utf8",
      timeout: 120_000,
      maxBuffer: 16 * 1024 * 1024,
    });
    assert.ifError(result.error);
    assert.notEqual(
      result.status,
      null,
      `${basename(entry)} terminated by ${result.signal}`,
    );
    return {
      exit: result.status!,
      stdout: result.stdout,
      stderr: result.stderr,
      log: this.read(this.log),
      command,
      env,
    };
  }
  /** A copy of the suite's tool directory without the named tools. */
  private toolBinWithout(omit: string[]): string {
    const source = getSuite().toolBin,
      dir = this.path(`tool-bin-without-${omit.join("-")}`);
    mkdirSync(dir, { recursive: true });
    for (const name of readdirSync(source)) {
      const command =
        process.platform === "win32" ? name.replace(/\.cmd$/i, "") : name;
      if (!omit.includes(command))
        writeFileSync(join(dir, name), readFileSync(join(source, name)), {
          mode: 0o755,
        });
    }
    return dir;
  }
  assertNoSettingsFileUnder(directory: string): void {
    for (const item of readdirSync(directory, { withFileTypes: true })) {
      const path = join(directory, item.name);
      if (item.isDirectory()) this.assertNoSettingsFileUnder(path);
      else
        assert.notEqual(
          item.name,
          "settings.json",
          `Unexpected settings file: ${path}`,
        );
    }
  }
}
export function createProfile(t: TestContext): Profile {
  return new Profile(t);
}

// Relative specifiers in static imports and re-exports (`from "./x.ts"`),
// side-effect imports and dynamic imports. A specifier that names no file,
// such as one inside a string a test writes, is ignored.
const relativeImport =
  /\bfrom\s*["'](\.{1,2}\/[^"']+)["']|\bimport\s*\(?\s*["'](\.{1,2}\/[^"']+)["']/g;

/**
 * The maintainer CLI's sources, relative to the checkout: every .ts file under
 * the plugin directory and every repository module they reach through relative
 * imports. Walking the imports keeps a fixture runnable when a module gains a
 * dependency, without a list to update.
 */
export function maintainerSources(repoRoot: string): string[] {
  const pluginDir = join(repoRoot, PLUGIN_SOURCES);
  const pending = readdirSync(pluginDir, { recursive: true })
    .map(String)
    .filter((name) => name.endsWith(".ts"))
    .map((name) => join(pluginDir, name));
  const seen = new Set<string>();
  while (pending.length) {
    const file = pending.pop()!;
    if (seen.has(file)) continue;
    seen.add(file);
    for (const match of readFileSync(file, "utf8").matchAll(relativeImport)) {
      const target = resolve(dirname(file), match[1] ?? match[2]!);
      const inside = relative(repoRoot, target);
      if (inside.startsWith("..") || isAbsolute(inside)) continue;
      if (statSync(target, { throwIfNoEntry: false })?.isFile())
        pending.push(target);
    }
  }
  return [...seen].map((file) => relative(repoRoot, file)).sort();
}

export interface ValidateFixture {
  root: string;
  /** The fixture's maintainer CLI. */
  cli: string;
  /**
   * Deletes one fixture file, named relative to the fixture root. A launcher
   * is named without its platform extension, so `node_modules/.bin/tsc` also
   * removes `tsc.cmd` on Windows.
   */
  remove(relative: string): void;
}

/**
 * Builds a checkout whose conformance CLI, trusted verifier, typecheck, tools
 * and evals npm tests, and boundary CLI are stubs. The conformance stub records
 * every call (see runValidateFixture); AGX_STUB_HOOKS_FAIL fails its hooks
 * call and AGX_STUB_CONFORMANCE_FAIL every other call. The typecheck, both
 * test stages and the boundary check record themselves in the stage log
 * (stageLogPath) and fail when their AGX_STUB_*_FAIL variable is set.
 * Removing hooks/go/.agentics-entrypoint-hooktool makes the verifier build fail.
 */
export function createValidateFixture(t: TestContext): ValidateFixture {
  const root = realpathSync(
    mkdtempSync(join(tmpdir(), "agentics-entrypoint-repo-")),
  );
  t.after(() => rmSync(root, { recursive: true, force: true }));
  function write(relative: string, body: string | Buffer): void {
    const path = join(root, relative);
    mkdirSync(dirname(path), { recursive: true });
    writeFileSync(path, body, { mode: 0o755 });
  }
  const s = getSuite(),
    source = s.repoRoot;
  for (const file of maintainerSources(source))
    write(file, readFileSync(join(source, file)));
  // The stubs come after the copied sources, so a copied original never
  // replaces one.
  write(".github/tools/src/conformance/cli.ts", stubConformance);
  // The entrypoint names this module's Go version when it cannot build the
  // verifier.
  write("hooks/go/go.mod", "module fixture\n\ngo 1.27.4\n");
  write(
    "hooks/go/.agentics-entrypoint-hooktool",
    "controlled trusted verifier\n",
  );
  write(
    "package.json",
    fixturePackageJSON({ "test:typecheck": "tsc" }, rootEngines(source)),
  );
  const marker = join(s.fakeScripts, "stage-marker.mjs");
  writeLauncher(join(root, "node_modules/.bin"), "tsc", marker, [
    "typecheck",
    "AGX_STUB_TYPECHECK_FAIL",
  ]);
  write(
    ".github/tools/package.json",
    fixturePackageJSON({
      test: "stage-marker tools-test AGX_STUB_TOOLS_TEST_FAIL",
    }),
  );
  writeLauncher(
    join(root, ".github/tools/node_modules/.bin"),
    "stage-marker",
    marker,
  );
  write(".github/tools/src/boundaries/cli.ts", stubBoundaries);
  return {
    root,
    cli: join(root, CLI),
    remove(relative) {
      const path = join(root, relative);
      const launcher = launcherPath(dirname(path), basename(path));
      rmSync(existsSync(path) ? path : launcher);
    },
  };
}

/** One call of the fixture's conformance CLI: its argv and its AGX_CONFORMANCE_SHORT value. */
export interface ConformanceCall {
  argv: string[];
  short: string;
}

/** The file the validate fixture's typecheck, test and boundary stubs append their names to. */
export function stageLogPath(p: Profile): string {
  return p.path("stages.log");
}

/**
 * Runs the fixture's maintainer CLI, or another script given in the
 * invocation, with a private cache and returns every call of the conformance
 * stub, in order. The rest of the invocation (noCLIs, extraPath, omitTools)
 * passes through, and extraEnv follows the fixture's own entries.
 */
export function runValidateFixture(
  p: Profile,
  f: ValidateFixture,
  args: string[],
  extraEnv: NodeJS.ProcessEnv = {},
  invocation: Pick<
    Invocation,
    "noCLIs" | "extraPath" | "omitTools" | "script"
  > = {},
): { result: Result; calls: ConformanceCall[] } {
  const log = p.path("conformance-calls.jsonl"),
    cache = p.path("cache");
  // The maintainer CLI gives the verifier the .exe name Go needs on Windows.
  const verifier = process.platform === "win32" ? "hooktool.exe" : "hooktool";
  const result = p.run({
    script: f.cli,
    ...invocation,
    args,
    env: {
      XDG_CACHE_HOME: cache,
      AGX_STUB_CONFORMANCE_LOG: log,
      AGX_STUB_STAGE_LOG: stageLogPath(p),
      AGX_STUB_EXPECT_VERIFIER: join(cache, "agentics", verifier),
      ...extraEnv,
    },
  });
  const calls = existsSync(log)
    ? p
        .read(log)
        .split("\n")
        .filter(Boolean)
        .map((line) => JSON.parse(line) as ConformanceCall)
    : [];
  return { result, calls };
}

/**
 * The fixture root as the entrypoint computes its repository root. The fixture
 * root is created physical (realpath), so a logical `cd` and `pwd` give the
 * same path.
 */
export function canonicalRepoRoot(f: ValidateFixture): string {
  return realpathSync(f.root);
}

/**
 * Compares the conformance calls' argv with the expected calls, each given as
 * its domain and the arguments after --repo <root>. It returns "" on a match.
 */
export function validateArgvMismatch(
  calls: ConformanceCall[],
  f: ValidateFixture,
  ...expected: [string, ...string[]][]
): string {
  const root = canonicalRepoRoot(f);
  const actual = calls.map((call) => call.argv),
    want = expected.map(([domain, ...rest]) => [
      domain,
      "--repo",
      root,
      ...rest,
    ]);
  return JSON.stringify(actual) === JSON.stringify(want)
    ? ""
    : `Conformance argv = ${JSON.stringify(actual)}, want ${JSON.stringify(want)}`;
}

export class StageFixture {
  root: string;
  constructor(t: TestContext) {
    const parent = realpathSync(
      mkdtempSync(join(tmpdir(), "agentics-stage-repo-")),
    );
    t.after(() => rmSync(parent, { recursive: true, force: true }));
    this.root = join(parent, "repo");
    mkdirSync(this.root);
  }
  path(relative: string): string {
    return join(this.root, relative);
  }
  /** The fixture's maintainer CLI. */
  cli(): string {
    return this.path(CLI);
  }
  write(relative: string, content: string | Buffer, mode = 0o644): void {
    const path = this.path(relative);
    mkdirSync(dirname(path), { recursive: true });
    writeFileSync(path, content, { mode });
  }
  git(...args: string[]): void {
    git(this.root, "-c", "maintenance.auto=false", "-c", "gc.auto=0", ...args);
  }
  /** Runs the fixture's maintainer CLI and requires exit 0. */
  run(profile: Profile, options: { args?: string[] } = {}): Result {
    const result = profile.run({ script: this.cli(), args: options.args });
    assert.equal(result.exit, 0, result.stdout + result.stderr);
    return result;
  }
}

export function createStageFixture(t: TestContext): StageFixture {
  const fixture = new StageFixture(t),
    source = getSuite().repoRoot;
  for (const file of maintainerSources(source))
    fixture.write(file, readFileSync(join(source, file)));
  for (const path of [
    ".claude-plugin/marketplace.json",
    ".agents/plugins/marketplace.json",
    ".antigravity-plugin/plugin.json",
  ])
    fixture.write(path, "{}\n");
  fixture.write("hooks/run-hook.cmd", "fixture\n", 0o755);
  fixture.write("hooks/fetch-hook.ps1", "fixture\n");
  const stub = "#!/bin/sh\n";
  const binaries = [
    "agx-darwin-amd64",
    "agx-darwin-arm64",
    "agx-linux-amd64",
    "agx-linux-arm64",
    "agx-windows-amd64.exe",
    "agx-windows-arm64.exe",
  ];
  for (const name of binaries) fixture.write(`hooks/bin/${name}`, stub, 0o755);
  fixture.write("bin/agx.exe", "fixture launcher");
  const hash = createHash("sha256").update(stub).digest("hex");
  fixture.write(
    "hooks/SHA256SUMS",
    binaries.map((name) => `${hash}  bin/${name}\n`).join(""),
  );
  // Setup mode runs the TypeScript conformance CLI on the manifests domain
  // before it stages, so the stage tests need one that passes.
  fixture.write(
    ".github/tools/src/conformance/cli.ts",
    'console.log("PASS manifests");\n',
  );
  fixture.write("package.json", fixturePackageJSON({}, rootEngines(source)));
  fixture.write(".gitignore", "node_modules/\n.claude/worktrees/\n");
  fixture.write("node_modules/dependency.js", "// ignored dependency\n");
  fixture.write(".claude/worktrees/wt/file.txt", "ignored worktree\n");
  fixture.write("skills/committed/SKILL.md", "committed\n");
  fixture.git("init");
  fixture.git("add", "-A");
  fixture.git(
    "-c",
    "user.email=fixture@agentics.test",
    "-c",
    "user.name=fixture",
    "commit",
    "-m",
    "fixture",
  );
  return fixture;
}
