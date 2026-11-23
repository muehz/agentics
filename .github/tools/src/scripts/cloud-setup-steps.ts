import { spawnSync } from "node:child_process";
import {
  mkdirSync,
  readFileSync,
  rmSync,
  symlinkSync,
  writeSync,
} from "node:fs";
import path from "node:path";
import { findCommand } from "../plugin/common.ts";
import { satisfies } from "./node-range.ts";

export interface RunResult {
  status: number;
  stdout: string;
}

export interface RunOptions {
  cwd: string;
  env: NodeJS.ProcessEnv;
  /** Capture stdout and discard stderr. Otherwise the child inherits both. */
  capture?: boolean;
}

export type Runner = (
  command: string,
  args: string[],
  options: RunOptions,
) => RunResult;

export interface CloudSetupDeps {
  env: NodeJS.ProcessEnv;
  repoRoot: string;
  run: Runner;
  /** Resolves a command on the PATH in `env`; undefined when it is absent. */
  which: (name: string, env: NodeJS.ProcessEnv) => string | undefined;
  /** Replaces `linkPath` with a symbolic link to `target`, like `ln -sf`. */
  link: (target: string, linkPath: string) => void;
  /** The effective user id, or undefined where the platform has none. */
  uid: () => number | undefined;
  out: (text: string) => void;
  err: (text: string) => void;
}

class SetupFailure extends Error {}

/** True when `have` is a plain X.Y.Z version inside `>=floor`; anything else counts as too old. */
function meetsFloor(have: string, floor: string): boolean {
  try {
    return satisfies(`>=${floor}`, have);
  } catch {
    return false;
  }
}

export function npmFloor(packageJson: string): string {
  let range: unknown;
  try {
    range = (JSON.parse(packageJson) as { engines?: { npm?: unknown } }).engines
      ?.npm;
  } catch {
    range = undefined;
  }
  const match =
    typeof range === "string" ? /^>=(\d+\.\d+\.\d+)$/.exec(range) : null;
  if (!match?.[1])
    throw new SetupFailure(
      'package.json engines.npm is not in the form ">=X.Y.Z". Update npmFloor in .github/tools/src/scripts/cloud-setup-steps.ts.',
    );
  return match[1];
}

const OPTIONAL_TOOLS = ["make", "shellcheck", "jq", "gh"];

/**
 * Runs the setup steps that follow the bootstrap and returns the exit status.
 * `argv` holds only the arguments the bootstrap forwards.
 */
export function runCloudSetup(argv: string[], deps: CloudSetupDeps): number {
  const { env, repoRoot, run, which, out, err } = deps;
  const fail = (message: string): never => {
    throw new SetupFailure(message);
  };
  const warn = (message: string): void =>
    err(`cloud-setup: WARNING: ${message}\n`);
  const step = (message: string): void => out(`==> ${message}\n`);
  const exec = (command: string, args: string[], capture = false) =>
    run(command, args, { cwd: repoRoot, env, capture });
  const version = (command: string, args: string[]): string => {
    if (!which(command, env)) return "";
    const result = exec(command, args, true);
    return result.status === 0 ? result.stdout.trim() : "";
  };

  try {
    let runTests = true;
    for (const arg of argv) {
      if (arg === "--skip-tests") runTests = false;
      else fail(`unknown option: ${arg}`);
    }
    const required = (name: string): string => {
      const value = env[name];
      return value
        ? value
        : fail(`${name} is not set. Run .github/scripts/cloud-setup.sh.`);
    };
    const toolsDir = required("AGX_TOOLS_DIR");
    const binDir = required("AGX_BIN_DIR");
    const originalPath = required("AGX_ORIGINAL_PATH");

    step("Checking npm");
    const floor = npmFloor(
      readFileSync(path.join(repoRoot, "package.json"), "utf8"),
    );
    let have = version("npm", ["--version"]);
    if (have && meetsFloor(have, floor)) {
      out(`npm ${have} satisfies engines.npm (>=${floor}).\n`);
    } else {
      if (have) warn(`npm ${have} is older than the required ${floor}.`);
      step(`Installing npm ${floor}`);
      const install = exec("npm", [
        "install",
        "--prefix",
        `${toolsDir}/npm`,
        "--ignore-scripts",
        "--no-audit",
        "--no-fund",
        `npm@${floor}`,
      ]);
      if (install.status !== 0)
        fail(
          `cannot install npm ${floor}. Check access to registry.npmjs.org.`,
        );
      deps.link(`${toolsDir}/npm/node_modules/.bin/npm`, `${binDir}/npm`);
      deps.link(`${toolsDir}/npm/node_modules/.bin/npx`, `${binDir}/npx`);
    }
    have = version("npm", ["--version"]);
    if (!have || !meetsFloor(have, floor))
      fail(
        `npm ${have || "is missing"} is on PATH but package.json requires >=${floor}. Put ${binDir} first on PATH.`,
      );

    const missing = OPTIONAL_TOOLS.filter((tool) => !which(tool, env));
    if (missing.length > 0) {
      if (deps.uid() === 0 && which("apt-get", env)) {
        step(`Installing optional tools with apt-get: ${missing.join(" ")}`);
        if (exec("apt-get", ["update", "-qq"], true).status !== 0)
          warn("apt-get update failed.");
        for (const tool of missing)
          if (
            exec("apt-get", ["install", "-y", "-qq", tool], true).status !== 0
          )
            warn(`could not install ${tool} with apt-get.`);
      }
      for (const tool of OPTIONAL_TOOLS)
        if (!which(tool, env))
          warn(
            `${tool} is not installed. Some tasks need it: make (hook rebuild), shellcheck (CI repro), jq (evals workflow), gh (pull requests).`,
          );
    }

    step("Running setup-worktree.ts (npm ci, Go modules, Git hooks)");
    if (
      exec("node", [".github/tools/src/scripts/setup-worktree.ts"]).status !== 0
    )
      fail("setup-worktree.ts failed. Read its message above.");

    if (runTests) {
      step("Smoke test: go test -C hooks/go ./...");
      if (exec("go", ["test", "-C", "hooks/go", "./..."]).status !== 0)
        fail("hooks/go tests failed. Read the output above.");
    } else {
      out("Skipped the smoke test (--skip-tests).\n");
    }

    step("Ready");
    out(`go:        ${version("go", ["version"])}\n`);
    out(`node:      ${version("node", ["--version"])}\n`);
    out(`npm:       ${version("npm", ["--version"])}\n`);
    out(`tool dir:  ${binDir}\n`);
    if (!originalPath.split(":").includes(binDir))
      warn(
        `${binDir} was not on PATH when this script started. Add it to PATH in each new shell.`,
      );
    return 0;
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    err(`cloud-setup: ERROR: ${message}\n`);
    return 1;
  }
}

export function systemDeps(
  env: NodeJS.ProcessEnv,
  repoRoot: string,
): CloudSetupDeps {
  return {
    env,
    repoRoot,
    run: (command, args, options) => {
      const result = spawnSync(command, args, {
        cwd: options.cwd,
        env: options.env,
        encoding: "utf8",
        stdio: options.capture ? ["inherit", "pipe", "ignore"] : "inherit",
      });
      return { status: result.status ?? 1, stdout: result.stdout ?? "" };
    },
    which: (name, env) => findCommand(name, env),
    link: (target, linkPath) => {
      mkdirSync(path.dirname(linkPath), { recursive: true });
      rmSync(linkPath, { force: true });
      symlinkSync(target, linkPath);
    },
    uid: () => process.geteuid?.(),
    out: (text) => writeSync(1, text),
    err: (text) => writeSync(2, text),
  };
}
