import {
  lstatSync,
  mkdirSync,
  mkdtempSync,
  rmSync,
  symlinkSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";

/** The skip reason a test passes when `canSymlink()` is false. */
export const SYMLINK_SKIP = "this host cannot create a symlink";

/** The skip reason a test passes when `canLinkDirectory()` is false. */
export const DIRECTORY_LINK_SKIP = "this host cannot create a directory link";

/**
 * Removes `target` and everything below it, retrying a few times so a file another process still holds open on Windows
 * does not fail the test. A missing path is not an error.
 */
export function removeTree(target: string): void {
  rmSync(target, {
    recursive: true,
    force: true,
    maxRetries: 5,
    retryDelay: 100,
  });
}

function withProbeDir<T>(body: (dir: string) => T): T {
  const dir = mkdtempSync(path.join(tmpdir(), "agentics-host-probe-"));
  try {
    return body(dir);
  } finally {
    removeTree(dir);
  }
}

let symlinkAnswer: boolean | undefined;

/**
 * Whether this host can create a symbolic link to a file. Windows without the symlink privilege cannot. The first call
 * probes by linking a file in its own temporary directory; later calls return the cached answer.
 */
export function canSymlink(): boolean {
  symlinkAnswer ??= withProbeDir((dir) => {
    const target = path.join(dir, "target");
    writeFileSync(target, "x");
    try {
      symlinkSync(target, path.join(dir, "link"));
      return true;
    } catch {
      return false;
    }
  });
  return symlinkAnswer;
}

/**
 * Makes `link` a link to the directory `target`, creating the link's missing parent directories first. It tries a
 * directory symlink and, on Windows, falls back to a junction, which needs no privilege. `target` is passed to the host
 * unchanged, so a relative target resolves against the link's directory. Returns the kind of link made, or undefined
 * when neither can be made; it never throws.
 */
export function linkDirectory(
  target: string,
  link: string,
): "dir" | "junction" | undefined {
  try {
    mkdirSync(path.dirname(link), { recursive: true });
  } catch {
    return undefined;
  }
  const kinds =
    process.platform === "win32"
      ? (["dir", "junction"] as const)
      : (["dir"] as const);
  for (const kind of kinds) {
    try {
      symlinkSync(target, link, kind);
      return kind;
    } catch {
      // The next kind may still succeed.
    }
  }
  return undefined;
}

let directoryLinkAnswer: boolean | undefined;

/**
 * Whether `linkDirectory` can make a directory link on this host. On Windows without the symlink privilege it is true
 * while `canSymlink()` is false, because a junction needs no privilege. The first call probes in its own temporary
 * directory; later calls return the cached answer.
 */
export function canLinkDirectory(): boolean {
  directoryLinkAnswer ??= withProbeDir((dir) => {
    const target = path.join(dir, "target");
    mkdirSync(target);
    return linkDirectory(target, path.join(dir, "link")) !== undefined;
  });
  return directoryLinkAnswer;
}

/**
 * Calls `fn` with a fresh directory outside any fixture, for a link to point at, and returns `fn`'s result. The
 * directory is removed right after `fn` returns or throws; when `fn` returns a promise, it is removed once the promise
 * settles, and the promise returned carries the same outcome.
 */
export function withOutside<T>(fn: (dir: string) => T): T {
  const dir = mkdtempSync(path.join(tmpdir(), "agentics-outside-"));
  let result: T;
  try {
    result = fn(dir);
  } catch (error) {
    removeTree(dir);
    throw error;
  }
  if (result instanceof Promise)
    return result.finally(() => removeTree(dir)) as T;
  removeTree(dir);
  return result;
}

/**
 * Whether the host reports a path below a regular file as "not a directory" (`ENOTDIR` from `lstat`). POSIX hosts do;
 * Windows reports the path as absent instead.
 */
export function reportsNotADirectory(): boolean {
  return withProbeDir((dir) => {
    writeFileSync(path.join(dir, "file"), "x");
    try {
      lstatSync(path.join(dir, "file", "child"));
      return false;
    } catch (error) {
      return (error as NodeJS.ErrnoException).code === "ENOTDIR";
    }
  });
}

/**
 * Whether a test can make a directory the process cannot read by removing its permissions: not on Windows, which
 * ignores them, and not as root, which reads it anyway.
 */
export function makesUnreadableDirs(): boolean {
  return process.platform !== "win32" && process.getuid?.() !== 0;
}

/**
 * Whether a test can make a regular file the audit cannot read: on Linux and not as root. Root reads a file whose
 * permissions are removed and Windows ignores them; macOS could make one, but the tests that use this pin their
 * expectation on Linux only.
 */
export function makesUnreadableFilesOnLinux(): boolean {
  return process.platform === "linux" && process.getuid?.() !== 0;
}
