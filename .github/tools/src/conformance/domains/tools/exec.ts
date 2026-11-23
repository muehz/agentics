import { spawn } from "node:child_process";
import {
  accessSync,
  closeSync,
  constants,
  mkdtempSync,
  openSync,
  readSync,
  realpathSync,
  rmSync,
  statSync,
} from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { errorMessage } from "../../check.ts";

export type Env = Record<string, string | undefined>;

export interface ExecSpec {
  dir: string;
  bin: string;
  args: string[];
  env: Env;
}

/** `code` is null when the process never started or was killed by a signal; `spawnError` then says why. */
export interface ExecResult {
  code: number | null;
  output: string;
  spawnError?: string;
}

/** The process-runner seam. Tests replace it with a recorder, so they run no real toolchain. */
export type Exec = (spec: ExecSpec) => Promise<ExecResult>;

interface Outcome {
  code: number | null;
  spawnError?: string;
}

/** How a child process failed: a nonzero exit code, a terminating signal, or a reason it never started. */
export type ChildFailure =
  { code: number } | { signal: string } | { startError: string };

/**
 * The one wording every conformance call site gives a failed child process. `command` names the program as the call
 * site does, never its full argument list.
 */
export function describeChildFailure(
  command: string,
  failure: ChildFailure,
): string {
  if ("code" in failure) return `${command} exited with code ${failure.code}`;
  if ("signal" in failure) return `${command} was killed by ${failure.signal}`;
  return `${command} could not start: ${failure.startError}`;
}

export function definedEnv(env: Env): NodeJS.ProcessEnv {
  return Object.fromEntries(
    Object.entries(env).filter(([, value]) => value !== undefined),
  );
}

/** Reads the descriptor from offset 0 to EOF, so the output is collected without opening its path a second time. */
function readAll(fd: number): Buffer {
  const chunks: Buffer[] = [];
  const chunk = Buffer.allocUnsafe(64 * 1024);
  let position = 0;
  for (;;) {
    const n = readSync(fd, chunk, 0, chunk.length, position);
    if (n === 0) break;
    chunks.push(Buffer.from(chunk.subarray(0, n)));
    position += n;
  }
  return Buffer.concat(chunks);
}

function run(spec: ExecSpec, fd: number): Promise<Outcome> {
  return new Promise((resolve) => {
    let settled = false;
    const finish = (outcome: Outcome) => {
      if (!settled) {
        settled = true;
        resolve(outcome);
      }
    };
    // A synchronous spawn throw (a NUL byte in an argument, a .cmd shim on Windows) rejects this promise;
    // realExec turns that rejection into a spawnError.
    const child = spawn(spec.bin, spec.args, {
      cwd: spec.dir,
      env: definedEnv(spec.env),
      stdio: ["ignore", fd, fd],
      windowsHide: true,
    });
    child.on("error", (error) =>
      finish({ code: null, spawnError: error.message }),
    );
    child.on("close", (code, signal) =>
      finish(
        signal === null
          ? { code }
          : { code: null, spawnError: `killed by ${signal}` },
      ),
    );
  });
}

/**
 * Spawns without a shell and never rejects. Stdout and stderr share one file descriptor, so the output keeps the
 * order in which the child wrote it; two Node pipes would lose it. Output a descendant writes after the direct child
 * has exited is not collected: with a file descriptor, close fires when the child itself exits.
 */
export const realExec: Exec = async (spec) => {
  let scratch: string | undefined;
  let fd: number | undefined;
  try {
    scratch = mkdtempSync(path.join(tmpdir(), "agentics-exec-"));
    const file = path.join(scratch, "output");
    fd = openSync(file, "w+");
    const outcome = await run(spec, fd);
    // Decoded once from the whole bytes so a multibyte character is never split.
    const output = readAll(fd).toString("utf8");
    closeSync(fd);
    fd = undefined;
    return outcome.spawnError === undefined
      ? { code: outcome.code, output }
      : { code: outcome.code, output, spawnError: outcome.spawnError };
  } catch (error) {
    return { code: null, output: "", spawnError: errorMessage(error) };
  } finally {
    // Cleanup must not replace the result; Windows scanners can briefly hold the file open.
    if (fd !== undefined) {
      try {
        closeSync(fd);
      } catch {
        // already closed or invalid
      }
    }
    if (scratch !== undefined) {
      try {
        rmSync(scratch, {
          recursive: true,
          force: true,
          maxRetries: 3,
          retryDelay: 50,
        });
      } catch {
        // a leftover temp directory is harmless
      }
    }
  }
};

/** The last n lines of text after trimming trailing newlines. */
export function tail(text: string, lines: number): string {
  const all = text.replace(/\n+$/, "").split("\n");
  return (all.length > lines ? all.slice(all.length - lines) : all).join("\n");
}

function isRunnable(candidate: string): boolean {
  try {
    if (!statSync(candidate).isFile()) return false;
    if (process.platform !== "win32") accessSync(candidate, constants.X_OK);
    return true;
  } catch {
    // Missing, unreadable or (on POSIX) not executable: not a match, keep searching.
    return false;
  }
}

function isAbsoluteCandidate(candidate: string): boolean {
  if (process.platform !== "win32") return path.isAbsolute(candidate);
  return /^(?:[A-Za-z]:[\\/]|[\\/]{2}[^\\/])/.test(candidate);
}

function sameFile(a: string, b: string): boolean {
  try {
    return realpathSync(a) === realpathSync(b);
  } catch {
    return false;
  }
}

/**
 * Splits a PATH value into its entries. On Windows a double quote toggles a quoted region in which
 * the ";" separator is literal, and every quote is then removed, so `"C:\Program Files\Tools\bin"` names a directory
 * without the quotes and an unbalanced quote runs to the end of the value. An empty value is an empty list. On POSIX a
 * quote is an ordinary character and the value splits on ":".
 */
export function splitPathList(list: string, windows: boolean): string[] {
  if (!windows) return list.split(":");
  if (list === "") return [];
  const entries: string[] = [];
  let start = 0;
  let quoted = false;
  for (let i = 0; i < list.length; i++) {
    const c = list[i];
    if (c === '"') {
      quoted = !quoted;
    } else if (c === ";" && !quoted) {
      entries.push(list.slice(start, i));
      start = i + 1;
    }
  }
  entries.push(list.slice(start));
  return entries.map((entry) => entry.replaceAll('"', ""));
}

/**
 * Resolves a binary name against env.PATH and refuses a result found relative to the working directory: the first
 * match decides, and a relative one means "not found", so a
 * caller-controlled PATH cannot make the suite run a repository-local binary. On POSIX an empty PATH entry means "."
 * and is therefore such a relative match. On Windows empty entries are skipped, the working directory is searched
 * implicitly first unless NoDefaultCurrentDirectoryInExePath is set, and a relative match fails the lookup unless a
 * later absolute match is the same file. On Windows the bare name is tried only when it already has an extension,
 * then each PATHEXT extension; on POSIX the file must be executable.
 */
export function findOnPath(name: string, env: Env): string | undefined {
  const windows = process.platform === "win32";
  const entries = splitPathList(env.PATH ?? env.Path ?? "", windows);
  const extensions = windows
    ? (env.PATHEXT ?? ".com;.exe;.bat;.cmd")
        .split(";")
        .filter((extension) => extension !== "")
    : [];
  const lookIn = (dir: string): string | undefined => {
    const base = path.join(dir, name);
    if ((!windows || path.extname(name) !== "") && isRunnable(base))
      return base;
    for (const extension of extensions) {
      if (isRunnable(base + extension)) return base + extension;
    }
    return undefined;
  };
  if (!windows) {
    for (const entry of entries) {
      const found = lookIn(entry === "" ? "." : entry);
      if (found !== undefined) {
        return isAbsoluteCandidate(found) ? found : undefined;
      }
    }
    return undefined;
  }
  let relative =
    "NoDefaultCurrentDirectoryInExePath" in env ? undefined : lookIn(".");
  for (const entry of entries) {
    if (entry === "") continue;
    const found = lookIn(entry);
    if (found === undefined) continue;
    if (relative !== undefined && !sameFile(relative, found)) return undefined;
    if (!isAbsoluteCandidate(found)) {
      relative ??= found;
      continue;
    }
    return found;
  }
  return undefined;
}
