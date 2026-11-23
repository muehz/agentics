import { spawnSync } from "node:child_process";
import { scale } from "./harness.ts";

// Interprets one `ps -o pid=,stat= -p <pid>` observation. Only a successful one-row answer for that pid, or an
// empty answer with exit 1, counts. Anything else is unreliable and throws, because a successful empty response
// is not proof that a process is gone.
export function pidState(
  pid: number,
  stdout: string,
  stderr: string,
  code: number,
): boolean {
  const invalid = () =>
    new Error(
      `unreliable observation pid=${pid} exit=${code} stdout=${JSON.stringify(stdout)} stderr=${JSON.stringify(stderr)}`,
    );
  if (pid <= 0 || stderr !== "") throw invalid();
  if (code === 1 && stdout.trim() === "") return false;
  if (code !== 0) throw invalid();
  const rows = stdout.trim().split("\n");
  if (rows.length !== 1) throw invalid();
  const fields = (rows[0] ?? "").trim().split(/\s+/);
  if (fields.length !== 2 || fields[0] !== String(pid)) throw invalid();
  const state = fields[1] ?? "";
  if (state === "" || !"RSDTtWXZIUKP".includes(state[0] ?? "!"))
    throw invalid();
  for (const modifier of state.slice(1)) {
    if (!"+<>AELNSsVWXl".includes(modifier)) throw invalid();
  }
  return state[0] !== "Z";
}

// True when the process exists and is not a zombie.
export function pidAlive(pid: number): boolean {
  if (pid <= 0) throw new Error(`invalid owned fixture pid ${pid}`);
  const run = spawnSync("ps", ["-o", "pid=,stat=", "-p", String(pid)], {
    encoding: "utf8",
    timeout: scale(30_000),
    env: { ...process.env, LC_ALL: "C", COMMAND_MODE: "unix2003" },
  });
  if (run.error) throw run.error;
  return pidState(pid, run.stdout, run.stderr, run.status ?? -1);
}
