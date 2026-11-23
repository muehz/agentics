import { execFile, spawn } from "node:child_process";
import { constants as osConstants } from "node:os";
import { setTimeout as sleep } from "node:timers/promises";
import { EXIT_FAILURE, EXIT_TIMEOUT } from "./constants.ts";
import { isInterrupted, trackGroup } from "./interrupt.ts";

export type GroupState = "live" | "gone" | "unknown";

// Interprets `ps -axo pgid=,stat=`. A group is live while any member is not a zombie.
export function groupStateFromRows(rows: string, group: number): GroupState {
  for (const row of rows.split("\n")) {
    const [pgid, stat] = row.trim().split(/\s+/);
    if (pgid === String(group) && stat !== undefined && !stat.startsWith("Z"))
      return "live";
  }
  return "gone";
}

// A ps that hangs would stall the watchdog that polls it, so the timeout it enforces would never fire. The wait is
// bounded, and an expired one reads as a failed observation.
const PS_TIMEOUT_MS = 10_000;

function psRows(timeoutMs: number): Promise<string | undefined> {
  return new Promise((resolve) => {
    execFile(
      "ps",
      ["-axo", "pgid=,stat="],
      {
        maxBuffer: 64 * 1024 * 1024,
        timeout: timeoutMs,
        killSignal: "SIGKILL",
      },
      (error, stdout) => {
        resolve(error === null ? stdout : undefined);
      },
    );
  });
}

// "unknown" means ps itself failed or did not answer in time. Never read it as "gone": an unobservable group is not a
// dead group.
export async function groupState(
  group: number,
  psTimeoutMs = PS_TIMEOUT_MS,
): Promise<GroupState> {
  const rows = await psRows(psTimeoutMs);
  return rows === undefined ? "unknown" : groupStateFromRows(rows, group);
}

function killGroup(group: number, signal: NodeJS.Signals): void {
  try {
    process.kill(-group, signal);
  } catch {
    // the group is already gone
  }
}

// TERM, then KILL, each followed by up to five seconds of polling for the group to disappear.
export async function stopGroup(
  group: number,
  report: (line: string) => void,
): Promise<boolean> {
  for (const signal of ["SIGTERM", "SIGKILL"] as const) {
    killGroup(group, signal);
    for (let tick = 0; tick < 50; tick++) {
      const state = await groupState(group);
      if (state === "gone") return true;
      if (state === "unknown") {
        killGroup(group, "SIGKILL");
        report(`ERROR: peer could not observe owned process group ${group}`);
        return false;
      }
      await sleep(100);
    }
  }
  report(`ERROR: peer process group ${group} remained active after TERM/KILL`);
  return false;
}

export interface RunOptions {
  argv: string[];
  env: Record<string, string>;
  stdin?: string;
  seconds: number;
  stderr?: "inherit" | "ignore";
  onStdout?: (chunk: Buffer) => void;
  report: (line: string) => void;
}

type Marker = "" | "timeout" | "visibility";

// Runs a command in a process group of its own, under a timeout that always holds. The status is the command's
// exit status, 124 when the timeout fired and the whole group was stopped, and 1 when cleanup or observation
// failed. Descendants a successful leader leaves behind are stopped too.
export async function runWithTimeout(options: RunOptions): Promise<number> {
  const [command, ...args] = options.argv;
  if (command === undefined) throw new Error("runWithTimeout needs a command");
  if (isInterrupted()) {
    options.report("ERROR: peer was interrupted; not starting a reviewer");
    return EXIT_FAILURE;
  }
  const child = spawn(command, args, {
    env: options.env,
    detached: true,
    stdio: ["pipe", "pipe", options.stderr ?? "inherit"],
  });
  let spawnError: Error | undefined;
  child.once("error", (error) => {
    spawnError = error;
  });
  const exited = new Promise<number>((resolve) => {
    child.once("exit", (code, signal) =>
      resolve(
        code ??
          128 + (signal === null ? 0 : (osConstants.signals[signal] ?? 0)),
      ),
    );
    child.once("error", () => resolve(127));
  });
  const closed = new Promise<void>((resolve) =>
    child.once("close", () => resolve()),
  );
  const group = child.pid;
  if (group === undefined || child.stdin === null || child.stdout === null) {
    await exited;
    options.report(
      `ERROR: cannot run ${command}: ${spawnError?.message ?? "spawn failed"}`,
    );
    return 127;
  }
  const release = trackGroup(group);
  child.stdin.on("error", () => undefined); // the reviewer may exit before it reads its whole prompt
  child.stdin.end(options.stdin ?? "");
  child.stdout.on("data", (chunk: Buffer) => options.onStdout?.(chunk));

  // The marker is read after the leader exits. `as Marker` keeps TypeScript from narrowing it to "" here.
  let marker = "" as Marker;
  const stopWatching = new AbortController();
  const watchdog = (async (): Promise<boolean> => {
    try {
      for (let waited = 0; waited < options.seconds; waited++) {
        await sleep(1000, undefined, { signal: stopWatching.signal });
        const state = await groupState(group);
        if (state === "gone") return true;
        if (state === "unknown") {
          marker = "visibility";
          killGroup(group, "SIGKILL");
          return false;
        }
      }
    } catch (error) {
      if ((error as Error).name === "AbortError") return true;
      throw error;
    }
    marker = "timeout";
    return await stopGroup(group, options.report);
  })();

  let status = await exited;
  if (marker !== "") {
    const clean = await watchdog;
    if (marker === "timeout" && clean) {
      status = EXIT_TIMEOUT;
    } else {
      options.report(
        "ERROR: peer timeout cleanup or process visibility failed",
      );
      status = EXIT_FAILURE;
    }
  } else {
    stopWatching.abort();
    await watchdog;
  }
  // Handles a successful leader that leaves an owned background process too.
  if (!(await stopGroup(group, options.report))) status = EXIT_FAILURE;
  release();
  await Promise.race([closed, sleep(5000, undefined, { ref: false })]);
  return status;
}
