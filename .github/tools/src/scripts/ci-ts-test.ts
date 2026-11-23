#!/usr/bin/env node
// Runs the TypeScript workspace tests for one leg of the validate workflow's ts-suite job.
//   WORKSPACE=<name> RUNNER_LABEL=<os> TEST_SHARD=<n> TEST_CONCURRENCY=<n> node .github/tools/src/scripts/ci-ts-test.ts
// The Windows runner splits the workspace's test files into shards (ci-test-shard.ts); every other runner runs the
// workspace's own `npm test`. The exit status of that child is the step's. It imports Node built-ins and the
// dependency-free plugin/common.ts only, like the other CI modules.

import { spawnSync } from "node:child_process";
import { findCommand, spawnPlan } from "../plugin/common.ts";

export interface CommandResult {
  status: number | null;
  error?: Error;
  signal?: NodeJS.Signals | null;
}
export type Env = Readonly<Record<string, string | undefined>>;
export type CommandRun = (
  command: string,
  args: string[],
  options: { env: Env; platform: NodeJS.Platform },
) => CommandResult;
export type Report = (message: string) => void;

// Resolves the command through the shared PATH lookup and starts it without a shell, with output going straight to the
// log. `node` is the running interpreter, so no lookup is needed for it.
const nativeRun: CommandRun = (command, args, options) => {
  const file =
    command === process.execPath
      ? command
      : (findCommand(command, options.env, options.platform) ?? command);
  const plan = spawnPlan(file, args, options.platform, options.env);
  const result = spawnSync(plan.file, plan.args, {
    stdio: "inherit",
    windowsVerbatimArguments: plan.windowsVerbatimArguments,
  });
  return {
    status: result.status,
    error: result.error,
    signal: result.signal,
  };
};

export interface Options {
  env?: Env;
  platform?: NodeJS.Platform;
  run?: CommandRun;
  report?: Report;
}

// Returns the exit status of the step.
export function tsTest(options: Options = {}): number {
  const env = options.env ?? process.env;
  const platform = options.platform ?? process.platform;
  const run = options.run ?? nativeRun;
  const report = options.report ?? console.error;
  const workspace = env["WORKSPACE"] ?? "";
  if (workspace === "") {
    report("unknown TypeScript workspace");
    return 1;
  }
  const [command, args] =
    env["RUNNER_LABEL"] === "windows-latest"
      ? ([
          process.execPath,
          [
            ".github/tools/src/scripts/ci-test-shard.ts",
            `windows-${workspace}`,
            env["TEST_SHARD"] ?? "",
            env["TEST_CONCURRENCY"] ?? "",
          ],
        ] as const)
      : (["npm", ["--prefix", `.github/${workspace}`, "test"]] as const);
  const name = command === process.execPath ? "node" : command;
  const result = run(command, [...args], { env, platform });
  if (result.error) {
    report(`ci-ts-test: cannot start ${name}: ${result.error.message}`);
    return 1;
  }
  if (result.signal) {
    report(`ci-ts-test: ${name} was killed by ${result.signal}`);
    return 1;
  }
  return result.status ?? 1;
}

if (import.meta.main) process.exitCode = tsTest();
