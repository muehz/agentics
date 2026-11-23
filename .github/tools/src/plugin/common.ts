import { accessSync, constants, readdirSync, statSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

export type Env = Record<string, string | undefined>;

/** The checkout root, derived from this module's location (four levels up) so a copied checkout resolves to itself. */
export const REPO_ROOT: string = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
  "..",
  "..",
  "..",
);

export const MARKETPLACE_NAME = "muehz";

export const PLUGINS: readonly string[] = ["agentics"];

/** An empty value counts as unset, as Bash `${VAR:-...}` does. */
function nonEmpty(value: string | undefined): string | undefined {
  return value === undefined || value === "" ? undefined : value;
}

/**
 * The parent of everything the maintainer CLI writes outside the checkout: `XDG_CACHE_HOME`, else `<HOME>/.cache`,
 * else `<os.homedir()>/.cache`. The result is not validated; the stage code rejects unsafe roots.
 */
export function cacheRoot(env: Env = process.env): string {
  const xdg = nonEmpty(env.XDG_CACHE_HOME);
  if (xdg !== undefined) return xdg;
  return path.join(nonEmpty(env.HOME) ?? os.homedir(), ".cache");
}

export function toolCacheDir(env: Env = process.env): string {
  return path.join(cacheRoot(env), "agentics");
}

const DEFAULT_PATHEXT = ".COM;.EXE;.BAT;.CMD";

function isRegularFile(file: string): boolean {
  try {
    return statSync(file).isFile();
  } catch {
    return false;
  }
}

function isExecutableFile(file: string): boolean {
  if (!isRegularFile(file)) return false;
  try {
    accessSync(file, constants.X_OK);
    return true;
  } catch {
    return false;
  }
}

function windowsMatches(
  dir: string,
  name: string,
  extensions: string[],
): string[] {
  let entries: string[];
  try {
    entries = readdirSync(dir);
  } catch {
    return [];
  }
  const found: string[] = [];
  const wanted = name.toLowerCase();
  const withExtension = extensions.some((ext) =>
    wanted.endsWith(ext.toLowerCase()),
  );
  const targets = withExtension ? [wanted] : [];
  for (const ext of extensions) targets.push(wanted + ext.toLowerCase());
  for (const target of targets) {
    for (const entry of entries) {
      if (entry.toLowerCase() !== target) continue;
      const full = path.join(dir, entry);
      if (isRegularFile(full) && !found.includes(full)) found.push(full);
    }
  }
  return found;
}

/**
 * Every match for `name` on `PATH`, in `PATH` order, as absolute paths. This is `command -v` semantics: empty `PATH`
 * entries are skipped and the working directory is never searched. It does not reuse the Go `exec.LookPath` port in the
 * conformance tools, which searches the working directory implicitly on Windows and fails on a relative match.
 * On Windows a file matches only through a `PATHEXT` extension, case-insensitively; on POSIX it must be an executable
 * regular file. Nothing is spawned.
 */
export function commandCandidates(
  name: string,
  env: Env = process.env,
  platform: NodeJS.Platform = process.platform,
): string[] {
  const windows = platform === "win32";
  const pathValue = env.PATH ?? env.Path ?? "";
  const extensions = windows
    ? (nonEmpty(env.PATHEXT) ?? DEFAULT_PATHEXT)
        .split(";")
        .filter((ext) => ext !== "")
    : [];
  const matches: string[] = [];
  for (const entry of pathValue.split(windows ? ";" : ":")) {
    if (entry === "") continue;
    const dir = path.resolve(entry);
    if (windows) {
      matches.push(...windowsMatches(dir, name, extensions));
    } else {
      const candidate = path.join(dir, name);
      if (isExecutableFile(candidate)) matches.push(candidate);
    }
  }
  return matches;
}

export function findCommand(
  name: string,
  env: Env = process.env,
  platform: NodeJS.Platform = process.platform,
): string | undefined {
  return commandCandidates(name, env, platform)[0];
}

export interface SpawnPlan {
  file: string;
  args: string[];
  windowsVerbatimArguments: boolean;
}

// cmd.exe expands % and ! even inside double quotes, and a quote or line break would end the quoting, so such an
// argument cannot be passed through it safely.
const CMD_UNSAFE = /["%!\r\n]/;

/**
 * How to start the resolved `file` with `args`. On Windows a `.cmd` or `.bat` cannot be started directly (Node refuses
 * it without a shell), so it runs through `cmd.exe /d /s /c` with every argument quoted and the line passed verbatim;
 * `/s` strips the outer quotes and `/d` skips AutoRun commands. An argument that quoting cannot protect is refused.
 */
export function spawnPlan(
  file: string,
  args: readonly string[],
  platform: NodeJS.Platform,
  env: Env = process.env,
): SpawnPlan {
  if (platform !== "win32" || !/\.(?:cmd|bat)$/i.test(file)) {
    return { file, args: [...args], windowsVerbatimArguments: false };
  }
  for (const value of [file, ...args]) {
    if (CMD_UNSAFE.test(value)) {
      throw new Error(
        `cannot pass ${JSON.stringify(value)} through cmd.exe: it contains a quote, %, ! or a line break`,
      );
    }
  }
  // The batch file hands its arguments to a program that parses them with the C runtime rules, where backslashes
  // before a closing quote escape it, so trailing backslashes are doubled.
  const quoted = args.map((arg) => `"${arg.replace(/(\\+)$/, "$1$1")}"`);
  const line = [`"${file}"`, ...quoted].join(" ");
  // Without ComSpec the fallback is absolute, because a bare cmd.exe can be found in the working directory first.
  const comspec =
    nonEmpty(env.ComSpec) ??
    nonEmpty(env.COMSPEC) ??
    path.win32.join(
      nonEmpty(env.SystemRoot) ?? nonEmpty(env.SYSTEMROOT) ?? "C:\\Windows",
      "System32",
      "cmd.exe",
    );
  return {
    file: comspec,
    args: ["/d", "/s", "/c", `"${line}"`],
    windowsVerbatimArguments: true,
  };
}

const CLIS = ["claude", "copilot", "codex", "agy"] as const;

/** The available plugin CLIs in the order `claude`, `copilot`, `codex`, `agy`; a missing CLI is omitted. */
export function detectClis(
  env: Env = process.env,
  platform: NodeJS.Platform = process.platform,
): string[] {
  return CLIS.filter((cli) => findCommand(cli, env, platform) !== undefined);
}
