// The release action's Run step. The step hands every input to this module through its env, and the module turns that
// env into the arguments of cli.ts in the same checkout, so no input value is ever spliced into a shell as code. The
// action runs it as node "$CHECKOUT/.github/tools/src/scripts/ci-release-run.ts", so it loads from the checkout that
// supplies the release code and never from the workspace. It imports Node built-ins only, because the release jobs
// install no packages before they call the action.
//
// The module does not check the command or the flags: cli.ts rejects an unknown one with exit 2, and that status is the
// step's.

import { spawnSync, type SpawnSyncOptions } from "node:child_process";
import { constants } from "node:os";

export type Env = Readonly<Record<string, string | undefined>>;

// The value flags in the order the command receives them, with the step variable each one reads.
const VALUE_FLAGS = [
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
] as const;

// The commands that declare --checkout. Every command finds cli.ts through the checkout; these three also read it.
const CHECKOUT_COMMANDS = ["context", "resume-check", "check-commit"];

export interface ReleaseCall {
  // The arguments of `node`: the path of cli.ts, then the command and its flags.
  readonly args: string[];
  readonly env: Record<string, string | undefined>;
}

export function releaseCall(env: Env = process.env): ReleaseCall {
  // An unset variable and an empty one both leave their flag out.
  const value = (name: string): string => env[name] ?? "";
  const command = value("COMMAND");
  const checkout = value("CHECKOUT");
  const values: Record<string, string> = Object.fromEntries(
    VALUE_FLAGS.map(([name]) => [name, value(name)]),
  );
  const childEnv: Record<string, string | undefined> = { ...env };
  if (command === "bundle") {
    // bundle runs the hook verifier the action built from the trusted checkout, never one the caller names.
    childEnv["AGX_HOOK_VERIFIER"] = `${value("RUNNER_TEMP")}/hooktool`;
    // The release workflow supplies a separate target checkout as bundle's filesystem repo.
    // Other callers can build from the tool checkout by leaving REPO empty.
    if (values["REPO"] === "") values["REPO"] = checkout;
  } else {
    // Only bundle declares --main-ref. The input has a non-empty default, so clear it here. context and stage exit 2
    // with an unknown flag error if it reaches them.
    values["MAIN_REF"] = "";
  }
  const args = [`${checkout}/.github/tools/src/release/cli.ts`, command];
  for (const [name, flag] of VALUE_FLAGS) {
    const given = values[name] ?? "";
    if (given !== "") args.push(flag, given);
  }
  // Only stage declares this flag. Pass even an empty value so stage rejects it instead of silently choosing the default
  // publication state.
  if (command === "stage") args.push("--prerelease", value("PRERELEASE"));
  if (CHECKOUT_COMMANDS.includes(command)) args.push("--checkout", checkout);
  return { args, env: childEnv };
}

export type Spawn = (
  command: string,
  args: readonly string[],
  options: SpawnSyncOptions,
) => {
  readonly status: number | null;
  readonly signal: NodeJS.Signals | null;
  readonly error?: Error;
};

// Starts cli.ts and returns the status the step exits with. `node` is found on PATH, as the step's shell found it, and
// takes the whole environment, so GH_TOKEN reaches the API commands and cli.ts writes the step outputs to
// $GITHUB_OUTPUT itself.
export function runRelease(
  env: Env = process.env,
  spawn: Spawn = (command, args, options) => spawnSync(command, args, options),
  report: (line: string) => void = (line) => {
    process.stderr.write(`${line}\n`);
  },
): number {
  const call = releaseCall(env);
  const result = spawn("node", call.args, { stdio: "inherit", env: call.env });
  if (result.error !== undefined) {
    report(`ci-release-run: cannot start node: ${result.error.message}`);
    return 127;
  }
  if (result.signal !== null) {
    report(`ci-release-run: node was killed by ${result.signal}`);
    // The status a shell reports for a child killed by a signal.
    return 128 + (constants.signals[result.signal] ?? 0);
  }
  return result.status ?? 1;
}

if (import.meta.main) process.exitCode = runRelease();
