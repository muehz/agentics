// The conformance action's install and run steps. Each step hands its inputs to this module through its env, so no
// input value is ever spliced into a shell as code. The action runs it as
// node "$AGX_CONFORMANCE_AUDITOR/.github/tools/src/scripts/ci-conformance.ts" <install|run>, so it loads from the
// trusted auditor checkout and never from the audited workspace, and it finds every auditor file through
// AGX_CONFORMANCE_AUDITOR rather than the working directory. The install subcommand runs before any `npm ci`, so the
// module imports Node built-ins and the built-ins-only common.ts only. It starts the auditor's cli.ts as a child and
// never imports it.

import { spawnSync, type SpawnSyncReturns } from "node:child_process";
import { appendFileSync, existsSync, writeSync } from "node:fs";
import { constants } from "node:os";
import path from "node:path";
import {
  findCommand,
  spawnPlan,
  type Env,
  type SpawnPlan,
} from "../plugin/common.ts";

export type Tool = "node" | "npm" | "go";

export interface ToolCall {
  tool: Tool;
  args: string[];
  env: Env;
  /** Capture stdout instead of passing it through; stderr always passes through. */
  capture: boolean;
}

/**
 * `status` is the exit status a shell step would report: 127 when the tool cannot start, 128 plus the signal number
 * when a signal killed it.
 */
export interface ToolResult {
  status: number;
  stdout: string;
}

export interface SpawnContext {
  platform: NodeJS.Platform;
  report: (line: string) => void;
}

export type Spawn = (call: ToolCall, context: SpawnContext) => ToolResult;

export interface StepOptions {
  env?: Env;
  platform?: NodeJS.Platform;
  spawn?: Spawn;
  report?: (line: string) => void;
}

const PREFIX = "conformance action";

// The npm the repository requires (engines.npm). Node 26 bundles npm 11, so an older npm is replaced first.
const NPM_FLOOR = [12, 2, 2] as const;

const defaultReport = (line: string): void => {
  writeSync(2, `${line}\n`);
};

/**
 * Starts `node` as this process's own executable, never one from PATH. `npm` and `go` resolve on the call's PATH
 * through findCommand, and a `.cmd` or `.bat` starts only through spawnPlan's cmd.exe line.
 */
export const defaultSpawn: Spawn = (call, { platform, report }) => {
  const file =
    call.tool === "node"
      ? process.execPath
      : findCommand(call.tool, call.env, platform);
  if (file === undefined) {
    report(`${PREFIX}: ${call.tool}: command not found`);
    return { status: 127, stdout: "" };
  }
  let plan: SpawnPlan;
  try {
    plan = spawnPlan(file, call.args, platform, call.env);
  } catch (error) {
    report(`${PREFIX}: ${(error as Error).message}`);
    return { status: 1, stdout: "" };
  }
  const result: SpawnSyncReturns<string> = spawnSync(plan.file, plan.args, {
    env: call.env,
    encoding: "utf8",
    stdio: ["inherit", call.capture ? "pipe" : "inherit", "inherit"],
    windowsVerbatimArguments: plan.windowsVerbatimArguments,
    maxBuffer: 16 * 1024 * 1024,
  });
  const stdout = result.stdout ?? "";
  if (result.error !== undefined) {
    report(`${PREFIX}: cannot start ${call.tool}: ${result.error.message}`);
    return { status: 127, stdout };
  }
  if (result.signal !== null)
    return { status: 128 + (constants.signals[result.signal] ?? 0), stdout };
  return { status: result.status ?? 1, stdout };
};

function resolveOptions(options: StepOptions) {
  const platform = options.platform ?? process.platform;
  const report = options.report ?? defaultReport;
  const spawn = options.spawn ?? defaultSpawn;
  return {
    env: options.env ?? process.env,
    platform,
    report,
    start: (call: ToolCall) => spawn(call, { platform, report }),
  };
}

// An empty value would resolve every auditor path against the working directory, which is the audited workspace.
function auditorOf(
  env: Env,
  report: (line: string) => void,
): string | undefined {
  const auditor = env.AGX_CONFORMANCE_AUDITOR ?? "";
  if (auditor !== "") return auditor;
  report(`${PREFIX}: AGX_CONFORMANCE_AUDITOR is empty or unset`);
  return undefined;
}

function below(version: readonly number[], floor: readonly number[]): boolean {
  for (let i = 0; i < floor.length; i++) {
    if (version[i] !== floor[i]) return version[i]! < floor[i]!;
  }
  return false;
}

/**
 * Installs the auditor's npm dependencies for the domains that load them (all and skills); every other domain makes no
 * npm call at all. An auditor whose node_modules exists is used as it is; the check follows links, as a shell `-e` test
 * does, so a dangling link is installed over. Both installs pass
 * --ignore-scripts, and npm ci reads the auditor's lockfile, never the audited checkout's.
 */
export function install(options: StepOptions = {}): number {
  const { env, report, start } = resolveOptions(options);
  const auditor = auditorOf(env, report);
  if (auditor === undefined) return 1;
  const domain = env.AGX_CONFORMANCE_DOMAIN ?? "";
  if (domain !== "all" && domain !== "skills") return 0;
  if (existsSync(path.join(auditor, "node_modules"))) return 0;
  const npm = (args: string[], capture = false) =>
    start({ tool: "npm", args, env, capture });
  const probe = npm(["--version"], true);
  if (probe.status !== 0) return probe.status;
  const version = probe.stdout.replace(/[\r\n]+$/, "");
  const match = /^(\d+)\.(\d+)\.(\d+)/.exec(version);
  if (match === null) {
    report(`${PREFIX}: unexpected npm version: ${version}`);
    return 1;
  }
  const parts = match.slice(1, 4).map((part) => Number.parseInt(part, 10));
  if (below(parts, NPM_FLOOR)) {
    const global = npm([
      "install",
      "--global",
      "--ignore-scripts",
      `npm@${NPM_FLOOR.join(".")}`,
    ]);
    if (global.status !== 0) return global.status;
  }
  return npm(["ci", "--ignore-scripts", "--prefix", auditor]).status;
}

/** Where the run step builds the trusted verifier. Go writes the -o name verbatim, so Windows needs the extension. */
export function verifierPath(
  runnerTemp: string,
  platform: NodeJS.Platform,
): string {
  return path.join(
    runnerTemp,
    platform === "win32" ? "hooktool.exe" : "hooktool",
  );
}

/**
 * Validates the inputs, builds the hook verifier from the auditor's hooks module, and runs the auditor's conformance
 * CLI against the audited checkout. The verifier comes from the auditor, so a hooktool committed in the audited
 * checkout never runs as the verifier. With allow-stale-review, CLI exit 3 (only stale docs reviews) becomes
 * `stale-review=true` and exit 0.
 */
export function run(options: StepOptions = {}): number {
  const { env, platform, report, start } = resolveOptions(options);
  const auditor = auditorOf(env, report);
  if (auditor === undefined) return 1;
  const allowStale = env.AGX_ALLOW_STALE_REVIEW ?? "";
  if (allowStale !== "true" && allowStale !== "false") {
    report("invalid allow-stale-review input");
    return 1;
  }
  // An empty input means false; the input itself always defaults to "false".
  const skipModuleTests = env.AGX_SKIP_MODULE_TESTS || "false";
  if (skipModuleTests !== "true" && skipModuleTests !== "false") {
    report("invalid skip-module-tests input");
    return 1;
  }
  const verifier = verifierPath(env.RUNNER_TEMP ?? "", platform);
  const build = start({
    tool: "go",
    args: [
      "build",
      "-C",
      path.join(auditor, "hooks", "go"),
      "-o",
      verifier,
      "./cmd/hooktool",
    ],
    env,
    capture: false,
  });
  if (build.status !== 0) return build.status;
  const args = [
    path.join(auditor, ".github", "tools", "src", "conformance", "cli.ts"),
    env.AGX_CONFORMANCE_DOMAIN ?? "",
    "--repo",
    env.AGX_CONFORMANCE_REPO ?? "",
  ];
  if (allowStale === "true") args.push("--stale-review-exit-code");
  if (skipModuleTests === "true") args.push("--fast");
  const cli = start({
    tool: "node",
    args,
    env: { ...env, AGX_HOOK_VERIFIER: verifier },
    capture: false,
  });
  let output: string;
  if (cli.status === 0) output = "stale-review=false\n";
  else if (allowStale === "true" && cli.status === 3)
    output = "stale-review=true\n";
  else return cli.status;
  try {
    appendFileSync(env.GITHUB_OUTPUT ?? "", output);
  } catch (error) {
    report(
      `${PREFIX}: cannot write the step output: ${(error as Error).message}`,
    );
    return 1;
  }
  return 0;
}

export function main(
  argv: readonly string[],
  options: StepOptions = {},
): number {
  const [command, ...rest] = argv;
  if (rest.length === 0 && command === "install") return install(options);
  if (rest.length === 0 && command === "run") return run(options);
  (options.report ?? defaultReport)("usage: ci-conformance.ts install|run");
  return 2;
}

if (import.meta.main) process.exitCode = main(process.argv.slice(2));
