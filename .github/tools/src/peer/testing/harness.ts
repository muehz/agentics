import { execFileSync, spawn, type ChildProcess } from "node:child_process";
import {
  closeSync,
  mkdtempSync,
  openSync,
  readFileSync,
  rmSync,
  symlinkSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { findOnPath } from "../launcher.ts";

export const posix = process.platform !== "win32";

// Lengthens a hang-guard duration for slow or instrumented runners. It never shortens one.
export function scale(milliseconds: number): number {
  const factor = Number(process.env.AGX_TEST_TIMEOUT_SCALE);
  return Number.isFinite(factor) && factor >= 1 && factor <= 1000
    ? milliseconds * factor
    : milliseconds;
}

const cli = fileURLToPath(new URL("../cli.ts", import.meta.url));
// The backstop for the case where the tool's own watchdog is the thing that broke; the hanging fakes sleep 30s.
const runTimeout = scale(120_000);

const scratch: string[] = [];
process.on("exit", () => {
  for (const dir of scratch) rmSync(dir, { recursive: true, force: true });
});

export function scratchDir(prefix: string): string {
  const dir = mkdtempSync(join(tmpdir(), prefix));
  scratch.push(dir);
  return dir;
}

// A disposable HOME: no run may read the maintainer's profile.
const suiteHome = scratchDir("agentics-peer-home-");

export interface Invocation {
  args?: string[];
  env?: Record<string, string>;
  dir?: string;
  path?: string;
  pipe?: boolean;
}

export interface Result {
  code: number;
  stdout: string;
  stderr: string;
  combined: string;
}

export interface Exit {
  // null when a signal killed the launcher, so a test can tell that apart from the launcher choosing a status.
  code: number | null;
  signal: NodeJS.Signals | null;
  stdout: string;
  stderr: string;
}

export interface Started {
  child: ChildProcess;
  finished: Promise<Exit>;
}

// Runs the real CLI as a child. The environment holds nothing but PATH, HOME, and TMPDIR plus the case's own
// variables, so platform detection fails unless a case sets a marker. Output goes to files by default. With
// pipe: true it goes through real pipes, so the run is not done until every holder of the write end has exited.
export async function runPeer(invocation: Invocation = {}): Promise<Result> {
  const { code, stdout, stderr } = await startPeer(invocation).finished;
  if (code === null) {
    throw new Error(
      `peer run [${(invocation.args ?? []).join(" ")}] did not finish
stdout: ${stdout}
stderr: ${stderr}`,
    );
  }
  return { code, stdout, stderr, combined: stdout + stderr };
}

// Starts the real CLI and returns at once, so a test can signal it while it runs. runPeer wraps it.
export function startPeer(invocation: Invocation = {}): Started {
  const env: Record<string, string> = {
    PATH: invocation.path ?? process.env.PATH ?? "",
    HOME: suiteHome,
    TMPDIR: tmpdir(),
    ...invocation.env,
  };
  const capture = scratchDir("agentics-peer-capture-");
  const outPath = join(capture, "stdout");
  const errPath = join(capture, "stderr");
  const piped = invocation.pipe === true;
  const outFd = piped ? undefined : openSync(outPath, "w");
  const errFd = piped ? undefined : openSync(errPath, "w");
  const child = spawn(process.execPath, [cli, ...(invocation.args ?? [])], {
    cwd: invocation.dir ?? process.cwd(),
    env,
    stdio: ["ignore", outFd ?? "pipe", errFd ?? "pipe"],
  });
  let pipedOut = "";
  let pipedErr = "";
  child.stdout?.on("data", (chunk: Buffer) => {
    pipedOut += chunk.toString();
  });
  child.stderr?.on("data", (chunk: Buffer) => {
    pipedErr += chunk.toString();
  });
  const timer = setTimeout(() => child.kill("SIGKILL"), runTimeout);
  const finished = new Promise<Exit>((resolve, reject) => {
    child.once("error", reject);
    child.once("close", (code, signal) => {
      clearTimeout(timer);
      if (outFd !== undefined) closeSync(outFd);
      if (errFd !== undefined) closeSync(errFd);
      resolve({
        code,
        signal,
        stdout: piped ? pipedOut : readFileSync(outPath, "utf8"),
        stderr: piped ? pipedErr : readFileSync(errPath, "utf8"),
      });
    });
  });
  return { child, finished };
}

// The git fixtures must not inherit GIT_DIR and its relatives from a caller that runs inside a git hook.
const gitEnv = Object.fromEntries(
  Object.entries(process.env).filter(([name]) => !name.startsWith("GIT_")),
);

function git(dir: string, ...args: string[]): void {
  execFileSync("git", ["-C", dir, ...args], { stdio: "pipe", env: gitEnv });
}

// A fixed identity and no signing or hooks, so the maintainer's git configuration cannot change the fixture.
function commit(dir: string, ...args: string[]): void {
  git(
    dir,
    "-c",
    "user.name=fixture",
    "-c",
    "user.email=fixture@localhost",
    "-c",
    "commit.gpgsign=false",
    "commit",
    "-q",
    "--no-verify",
    ...args,
  );
}

// Two commits: an empty base and a head that adds file.txt. The tool's git preflight resolves HEAD~1, which does
// not exist in a shallow checkout.
export function newWorkRepo(): string {
  const dir = scratchDir("agentics-peer-repo-");
  git(dir, "-c", "init.defaultBranch=main", "init", "-q");
  commit(dir, "--allow-empty", "-m", "base");
  writeFileSync(join(dir, "file.txt"), "one\n");
  git(dir, "add", "file.txt");
  commit(dir, "-m", "head");
  return dir;
}

// A PATH holding everything the tool needs EXCEPT timeout and gtimeout, so a dependency on coreutils timeout
// would show up as a failure on Linux too.
export function newShimPath(): string {
  const dir = scratchDir("agentics-peer-shim-");
  const names = [
    "bash",
    "sh",
    "env",
    "git",
    "date",
    "mktemp",
    "tee",
    "tr",
    "wc",
    "grep",
    "sed",
    "cat",
    "sleep",
    "rm",
    "mkdir",
    "mv",
    "head",
    "ls",
    "dirname",
    "basename",
    "ps",
    "awk",
  ];
  for (const name of names) {
    const target = findOnPath(name, process.env.PATH);
    if (target !== undefined) symlinkSync(target, join(dir, name));
  }
  for (const forbidden of ["timeout", "gtimeout"]) {
    if (findOnPath(forbidden, dir) !== undefined)
      throw new Error(`the shim PATH must not carry ${forbidden}`);
  }
  return dir;
}
