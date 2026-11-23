import { spawnSync } from "node:child_process";
import {
  commandCandidates,
  spawnPlan,
  type Env,
  type SpawnPlan,
} from "../plugin/common.ts";

const INTERPRETERS = ["sh", "bash", "pwsh", "powershell"] as const;
export type Interpreter = (typeof INTERPRETERS)[number];

const PROBE_TIMEOUT_MS = 10_000;

/** Result of one probe run; only the exit status and a spawn or timeout error matter. */
export interface ProbeResult {
  status: number | null;
  error?: Error;
}

export type ProbeRunner = (
  file: string,
  args: string[],
  options: { timeout: number; windowsVerbatimArguments?: boolean },
) => ProbeResult;

export interface InterpreterOptions {
  env?: Env;
  platform?: NodeJS.Platform;
  run?: ProbeRunner;
}

export interface TwinPowerShell {
  name: "pwsh" | "powershell";
  path: string | undefined;
  skip: false | string;
}

const defaultRunner: ProbeRunner = (file, args, options) => {
  const result = spawnSync(file, args, {
    ...options,
    stdio: "ignore",
    windowsHide: true,
  });
  return { status: result.status, error: result.error };
};

function assertInterpreter(name: string): asserts name is Interpreter {
  if (!(INTERPRETERS as readonly string[]).includes(name)) {
    throw new Error(
      `unknown interpreter "${name}"; expected one of ${INTERPRETERS.join(", ")}`,
    );
  }
}

function nonEmpty(value: string | undefined): string | undefined {
  return value === undefined || value === "" ? undefined : value;
}

function probeArgs(name: Interpreter): string[] {
  return name === "sh" || name === "bash"
    ? ["-c", "exit 0"]
    : ["-NoProfile", "-Command", "exit 0"];
}

function probe(
  candidate: string,
  name: Interpreter,
  platform: NodeJS.Platform,
  env: Env,
  run: ProbeRunner,
): boolean {
  let plan: SpawnPlan;
  try {
    plan = spawnPlan(candidate, probeArgs(name), platform, env);
  } catch {
    // A .cmd whose path cmd.exe cannot quote safely is never started, so it cannot be a working interpreter.
    return false;
  }
  const result = run(plan.file, plan.args, {
    timeout: PROBE_TIMEOUT_MS,
    windowsVerbatimArguments: plan.windowsVerbatimArguments,
  });
  return result.error === undefined && result.status === 0;
}

const memo = new Map<string, string | undefined>();

/**
 * The absolute path of the first `PATH` candidate for `name` whose probe exits 0
 * within ten seconds, or `undefined`. A candidate that is present but broken is
 * passed over: `System32\bash.exe` is the WSL launcher and fails without an
 * installed distribution, yet can precede a working Git bash.
 */
export function findInterpreter(
  name: string,
  options: InterpreterOptions = {},
): string | undefined {
  assertInterpreter(name);
  const env = options.env ?? process.env;
  const platform = options.platform ?? process.platform;
  const useMemo = options.run === undefined;
  const key = JSON.stringify([
    name,
    env.PATH ?? env.Path ?? "",
    env.PATHEXT ?? "",
    platform,
  ]);
  if (useMemo && memo.has(key)) return memo.get(key);
  const run = options.run ?? defaultRunner;
  const found = commandCandidates(name, env, platform).find((candidate) =>
    probe(candidate, name, platform, env, run),
  );
  if (useMemo) memo.set(key, found);
  return found;
}

function requiredList(env: Env): Interpreter[] {
  const raw = nonEmpty(env.AGX_REQUIRED_INTERPRETERS);
  if (raw === undefined) return [];
  return raw
    .split(",")
    .map((entry) => entry.trim())
    .filter((entry) => entry !== "")
    .map((entry) => {
      assertInterpreter(entry);
      return entry;
    });
}

/**
 * For the `skip` option of `node:test`: `false` when the interpreter works, a skip
 * reason when it is missing, and a thrown error when it is missing but listed in
 * `AGX_REQUIRED_INTERPRETERS`. Whether the run is on CI is deliberately not
 * consulted, so a runner that lacks an interpreter skips unless it opted in to
 * requiring it.
 */
export function requireInterpreter(
  name: string,
  options: InterpreterOptions = {},
): false | string {
  assertInterpreter(name);
  const env = options.env ?? process.env;
  const required = requiredList(env);
  if (findInterpreter(name, options) !== undefined) return false;
  if (required.includes(name)) {
    throw new Error(
      `required interpreter "${name}" is missing or fails its probe (listed in AGX_REQUIRED_INTERPRETERS)`,
    );
  }
  return `${name} is not available on this host`;
}

/**
 * The PowerShell that runs the `.ps1` leg: `AGX_TWIN_POWERSHELL` when set and
 * non-empty, otherwise `pwsh` when it works, else `powershell`.
 */
export function twinPowerShell(
  options: InterpreterOptions = {},
): TwinPowerShell {
  const env = options.env ?? process.env;
  const chosen = nonEmpty(env.AGX_TWIN_POWERSHELL);
  let name: "pwsh" | "powershell";
  if (chosen === undefined) {
    name =
      findInterpreter("pwsh", options) !== undefined ? "pwsh" : "powershell";
    // Neither works: a list naming either one must fail, not skip, so a leg
    // that requires pwsh cannot slip through on the powershell fallback.
    if (
      name === "powershell" &&
      findInterpreter("powershell", options) === undefined &&
      requiredList(env).includes("pwsh")
    ) {
      requireInterpreter("pwsh", options);
    }
  } else if (chosen === "pwsh" || chosen === "powershell") {
    name = chosen;
  } else {
    throw new Error(
      `AGX_TWIN_POWERSHELL must be "pwsh" or "powershell", got "${chosen}"`,
    );
  }
  return {
    name,
    path: findInterpreter(name, options),
    skip: requireInterpreter(name, options),
  };
}
