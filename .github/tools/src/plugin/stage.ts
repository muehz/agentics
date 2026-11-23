import { spawn, spawnSync, type ChildProcess } from "node:child_process";
import { createHash } from "node:crypto";
import {
  constants as fsConstants,
  lstatSync,
  mkdirSync,
  mkdtempSync,
  readdirSync,
  readFileSync,
  realpathSync,
  rmdirSync,
  rmSync,
  statSync,
} from "node:fs";
import fsp from "node:fs/promises";
import path from "node:path";
import {
  REPO_ROOT,
  cacheRoot as defaultCacheRoot,
  findCommand,
  spawnPlan,
  type Env,
  type SpawnPlan,
} from "./common.ts";

/**
 * The safety rules for the clean marketplace stage. Every CLI installs the local marketplace by copying a directory,
 * so the stage lives outside the checkout, under the cache root. Refresh and teardown delete and recreate the stage
 * leaf, so the cache root is checked before anything is created, copied or removed: a root that could alias the
 * checkout, a filesystem root, or a path whose meaning depends on normalization is rejected rather than repaired.
 */

export const STAGE_NAME = "agentics-local-marketplace";
export const LOCK_NAME = ".agentics-local-marketplace.lock";
export const INVALID_ROOT = "ERROR: invalid marketplace cache root";
export const UNOWNED_LEAF =
  "ERROR: marketplace stage leaf is not an owned directory";

export interface StageFailure {
  ok: false;
  message: string;
}

export interface ResolvedStageRoot {
  ok: true;
  platform: NodeJS.Platform;
  /** The nearest existing directory of the cache root, physical (symlinks and 8.3 names resolved). */
  ancestor: string;
  /** The cache root's components below `ancestor` that do not exist yet, outermost first. */
  missing: string[];
  /** The physical cache root: `ancestor` joined with `missing`. */
  physical: string;
  /** The physical checkout. */
  repo: string;
  stageDir: string;
  toolCacheDir: string;
}

export interface ResolveStageRootOptions {
  /** The cache root to check; defaults to `cacheRoot(env)`. */
  cacheRoot?: string;
  repoRoot?: string;
  platform?: NodeJS.Platform;
  env?: Env;
}

function invalid(): StageFailure {
  return { ok: false, message: INVALID_ROOT };
}

function pathApi(platform: NodeJS.Platform): typeof path.posix {
  return platform === "win32" ? path.win32 : path.posix;
}

/**
 * The comparison key for a path: Windows paths compare case-insensitively and with either separator, POSIX paths
 * compare exactly.
 */
export function pathKey(
  value: string,
  platform: NodeJS.Platform = process.platform,
): string {
  return platform === "win32"
    ? value.replaceAll("/", "\\").toLowerCase()
    : value;
}

/** Whether `child` is `parent` or below it, comparing keys. */
function within(child: string, parent: string, sep: string): boolean {
  if (child === parent) return true;
  const prefix = parent.endsWith(sep) ? parent : parent + sep;
  return child.startsWith(prefix);
}

/**
 * The absolute root of `value`, or undefined when the path is not anchored. On Windows only a drive with a separator
 * (`C:\`) or a UNC share counts: a drive-relative `C:cache` depends on the drive's current directory and a drive-less
 * `\cache` on the current drive, and device paths (`\\?\`, `\\.\`) bypass the normalization the other rules rely on.
 */
function absoluteRoot(
  value: string,
  platform: NodeJS.Platform,
): string | undefined {
  const api = pathApi(platform);
  const root = api.parse(value).root;
  if (platform !== "win32") return root === "/" ? root : undefined;
  if (/^[A-Za-z]:[\\/]$/.test(root)) return root;
  if (/^[\\/]{2}[?.][\\/]/.test(value)) return undefined;
  if (/^[\\/]{2}[^\\/]+[\\/][^\\/]+[\\/]?$/.test(root)) return root;
  return undefined;
}

/** Whether `value` is a filesystem root or the `.cache` directory directly under one. */
function isRootOrRootCache(value: string, platform: NodeJS.Platform): boolean {
  const root = absoluteRoot(value, platform);
  if (root === undefined) return true;
  const rest = pathKey(value.slice(root.length), platform).replace(
    /[\\/]+$/,
    "",
  );
  return rest === "" || rest === ".cache";
}

function hasDotComponent(value: string, platform: NodeJS.Platform): boolean {
  const separators = platform === "win32" ? /[\\/]/ : /\//;
  return value.split(separators).some((part) => part === "." || part === "..");
}

function isDirectory(value: string): boolean {
  try {
    return statSync(value).isDirectory();
  } catch {
    return false;
  }
}

/**
 * Whether anything, including a dangling link, occupies `value`. A path below a regular file is absent (POSIX reports
 * ENOTDIR, Windows ENOENT), as `[ -e ]` and `[ -L ]` both say; the walk then rejects the file itself. Any other error
 * counts as present, so a path that cannot be inspected is rejected rather than assumed free.
 */
function exists(value: string): boolean {
  try {
    return lstatSync(value, { throwIfNoEntry: false }) !== undefined;
  } catch (error) {
    return (error as NodeJS.ErrnoException).code !== "ENOTDIR";
  }
}

/**
 * The physical path, from the native call on every platform. The JavaScript one keeps the caller's spelling of each
 * component: 8.3 short names on Windows, and the caller's letter case on a case-insensitive volume (macOS APFS),
 * where keys keep case. Either would let an alias of the checkout pass the overlap rule. On Windows the long-path
 * prefix is removed so keys compare.
 */
function physicalPath(value: string, platform: NodeJS.Platform): string {
  const real = realpathSync.native(value);
  if (platform !== "win32") return real;
  if (/^\\\\\?\\UNC\\/i.test(real)) return "\\\\" + real.slice(8);
  if (real.startsWith("\\\\?\\")) return real.slice(4);
  return real;
}

/**
 * Checks a cache root before any mutation. Unsafe input is rejected, never normalized.
 * The checkout may sit elsewhere under the cache; it may not contain the cache or sit inside the stage leaf.
 */
export function resolveStageRoot(
  options: ResolveStageRootOptions = {},
): ResolvedStageRoot | StageFailure {
  const platform = options.platform ?? process.platform;
  const env = options.env ?? process.env;
  const input = options.cacheRoot ?? defaultCacheRoot(env);
  const repoRoot = options.repoRoot ?? REPO_ROOT;
  const api = pathApi(platform);

  if (input === "") return invalid();
  const root = absoluteRoot(input, platform);
  if (root === undefined) return invalid();
  if (hasDotComponent(input, platform)) return invalid();
  let trimmed = input;
  while (trimmed.length > root.length && /[\\/]$/.test(trimmed))
    trimmed = trimmed.slice(0, -1);
  if (isRootOrRootCache(trimmed, platform)) return invalid();

  // Walk up to the nearest directory, following directory symlinks as `[ -d ]` does. Anything else that exists in
  // the way, including a dangling link, is rejected: creating through it later would land somewhere unchecked.
  const parts = trimmed
    .slice(root.length)
    .split(platform === "win32" ? /[\\/]/ : /\//)
    .filter((part) => part !== "");
  const missing: string[] = [];
  let depth = parts.length;
  for (;;) {
    const candidate = api.join(root, ...parts.slice(0, depth));
    if (isDirectory(candidate)) break;
    if (exists(candidate) || depth === 0) return invalid();
    missing.unshift(parts[depth - 1]!);
    depth -= 1;
  }

  let ancestor: string;
  let repo: string;
  try {
    ancestor = physicalPath(api.join(root, ...parts.slice(0, depth)), platform);
    repo = physicalPath(repoRoot, platform);
  } catch {
    return invalid();
  }
  const physical = api.join(ancestor, ...missing);
  if (isRootOrRootCache(physical, platform)) return invalid();

  const key = pathKey(physical, platform);
  const repoKey = pathKey(repo, platform);
  if (within(key, repoKey, api.sep)) return invalid();
  if (within(repoKey, api.join(key, STAGE_NAME), api.sep)) return invalid();

  return {
    ok: true,
    platform,
    ancestor,
    missing,
    physical,
    repo,
    stageDir: api.join(physical, STAGE_NAME),
    toolCacheDir: api.join(physical, "agentics"),
  };
}

export type EnterCacheResult =
  | { status: "ok"; path: string }
  | { status: "missing" }
  | { status: "failed"; message: string };

/**
 * Re-verifies the resolved cache root and, with `create`, creates its missing components. A Node process cannot hold
 * a directory handle and create relative to it, so each component is created on its own
 * (never recursively), checked with `lstat` first so a symlink or file that appeared since resolution is refused, and
 * compared with its physical path after creation. The returned path is the verified physical cache root callers build
 * on. Without `create`, a missing component returns "missing" and nothing is created; teardown treats that as done.
 */
export function enterCache(
  resolved: ResolvedStageRoot,
  { create }: { create: boolean },
): EnterCacheResult {
  const { platform } = resolved;
  const api = pathApi(platform);
  const failed: EnterCacheResult = { status: "failed", message: INVALID_ROOT };
  const same = (a: string, b: string) =>
    pathKey(a, platform) === pathKey(b, platform);
  try {
    if (!isDirectory(resolved.ancestor)) return failed;
    let current = physicalPath(resolved.ancestor, platform);
    if (!same(current, resolved.ancestor)) return failed;
    for (const part of resolved.missing) {
      const next = api.join(current, part);
      let info = lstatSync(next, { throwIfNoEntry: false });
      if (info === undefined) {
        if (!create) return { status: "missing" };
        mkdirSync(next);
        info = lstatSync(next);
      }
      if (info.isSymbolicLink() || !info.isDirectory()) return failed;
      const actual = physicalPath(next, platform);
      if (!same(actual, next)) return failed;
      current = actual;
    }
    if (!same(current, resolved.physical)) return failed;
    return { status: "ok", path: current };
  } catch {
    return failed;
  }
}

/**
 * Whether the stage leaf under the verified cache path is safe to replace: missing or a
 * real directory. A symlink or junction is refused so removal never reaches its target.
 */
export function checkOwnedLeaf(cachePath: string): { ok: true } | StageFailure {
  const info = lstatSync(path.join(cachePath, STAGE_NAME), {
    throwIfNoEntry: false,
  });
  if (info === undefined) return { ok: true };
  if (info.isSymbolicLink() || !info.isDirectory())
    return { ok: false, message: UNOWNED_LEAF };
  return { ok: true };
}

/** The six hook binaries `run-hook.cmd` resolves; `hooks/SHA256SUMS` lists exactly these. */
const CONTRACT_BINARIES = [
  "agx-darwin-amd64",
  "agx-darwin-arm64",
  "agx-linux-amd64",
  "agx-linux-arm64",
  "agx-windows-amd64.exe",
  "agx-windows-arm64.exe",
] as const;
const CONTRACT_ENTRIES = new Set<string>(
  CONTRACT_BINARIES.map((name) => `bin/${name}`),
);
const NATIVE_OUTPUTS = new Set([
  ...CONTRACT_BINARIES.map((name) => `hooks/bin/${name}`),
  "bin/agx.exe",
]);
const DEVELOPMENT_FILES = [...NATIVE_OUTPUTS, "hooks/SHA256SUMS"];
const GENERATED_PATH_KEYS = ["hooks/bin", "hooks/sha256sums", "bin/agx.exe"];

export interface VerifyStageOptions {
  platform?: NodeJS.Platform;
  /** Receives each output line, newline included. */
  write?: (line: string) => void;
}

/**
 * Splits the manifest as awk's default field splitting does, which the shell check this replaces relied on: fields
 * are runs of anything but spaces and tabs, the empty remainder after a final newline is not a line, an empty line is
 * a record with no fields, and a carriage return stays inside its field, so a CRLF manifest is rejected.
 */
function manifestRecords(text: string): string[][] {
  const lines = text.split("\n");
  if (lines.at(-1) === "") lines.pop();
  return lines.map((line) =>
    line.split(/[ \t]+/).filter((field) => field !== ""),
  );
}

/** Whether `value` is a regular file after following links, as `[ -f ]` asks. */
function isFile(value: string): boolean {
  try {
    return statSync(value).isFile();
  } catch {
    return false;
  }
}

/** Whether `value` exists after following links, as `[ -e ]` asks. */
function existsFollowing(value: string): boolean {
  try {
    return statSync(value, { throwIfNoEntry: false }) !== undefined;
  } catch {
    return false;
  }
}

/** The lowercase hex SHA-256 of `file`, or undefined when it cannot be read, which then never matches an entry. */
function sha256(file: string): string | undefined {
  try {
    return createHash("sha256").update(readFileSync(file)).digest("hex");
  } catch {
    return undefined;
  }
}

/**
 * Checks the two ways a stage can be broken without looking broken: a file `run-hook.cmd`
 * needs is missing, or the hook roster disagrees with `hooks/SHA256SUMS`. The roster is the six contract names,
 * anchored on the manifest rather than on whatever `hooks/bin` holds. Every failure is written, then three closing
 * lines; the result says whether the stage is good.
 */
export function verifyStage(
  repoRoot: string,
  stageDir: string,
  {
    platform = process.platform,
    write = (line) => process.stderr.write(line),
  }: VerifyStageOptions = {},
): boolean {
  let ok = true;
  const fail = (message: string) => {
    write(`❌ Marketplace stage${message}\n`);
    ok = false;
  };

  for (const rel of [
    ".claude-plugin/marketplace.json",
    ".agents/plugins/marketplace.json",
    ".antigravity-plugin/plugin.json",
    "hooks/run-hook.cmd",
    "hooks/fetch-hook.ps1",
    "hooks/SHA256SUMS",
  ])
    if (!isFile(path.join(stageDir, rel))) fail(` is missing ${rel}.`);

  const manifestPath = path.join(stageDir, "hooks/SHA256SUMS");
  if (isFile(manifestPath)) {
    // latin1 maps every byte to one character, so a non-UTF-8 byte stays a mismatch instead of a decoding error.
    const records = manifestRecords(readFileSync(manifestPath, "latin1"));

    // Exactly the six contract entries, one "<hex>  bin/<name>" line each: build-all never writes anything else,
    // so a malformed, duplicate or extra line is a hand edit nothing verifies.
    const problems: string[] = [];
    const seen = new Set<string>();
    records.forEach((fields, index) => {
      const entry = fields[1];
      if (fields.length !== 2 || entry === undefined)
        problems.push(`malformed line ${index + 1}`);
      else if (!CONTRACT_ENTRIES.has(entry))
        problems.push(`unexpected entry ${entry}`);
      else if (seen.has(entry)) problems.push(`duplicate entry ${entry}`);
      else seen.add(entry);
    });
    if (problems.length > 0)
      fail(
        `: hooks/SHA256SUMS is not the six contract entries: ${problems.join(";")}`,
      );

    for (const name of CONTRACT_BINARIES) {
      const rel = `hooks/bin/${name}`;
      // The first line naming the entry supplies the hash even when it is malformed; that line is already reported.
      const want = records
        .find((fields) => fields[1] === `bin/${name}`)?.[0]
        ?.toLowerCase();
      if (want === undefined) {
        fail(`: hooks/SHA256SUMS has no entry for bin/${name}.`);
        continue;
      }
      const staged = path.join(stageDir, rel);
      if (!existsFollowing(staged)) {
        // Absent from the checkout too: run-hook.cmd fetches it and verifies it against the manifest. Present there
        // but not here: the copy dropped it.
        if (existsFollowing(path.join(repoRoot, rel)))
          fail(`: ${rel} is missing.`);
        continue;
      }
      if (sha256(staged) !== want) {
        fail(`: ${rel} does not match hooks/SHA256SUMS.`);
        continue;
      }
      // Windows has no execute bits, and the copy there keeps file content only.
      if (platform === "win32") continue;
      if ((statSync(staged).mode & 0o111) === 0)
        fail(`: ${rel} is not executable.`);
    }
  }

  if (!ok) {
    write("   run-hook.cmd needs the dispatcher, the manifest, and any\n");
    write("   staged binary to agree; both guardian hooks fail closed\n");
    write("   otherwise. Refusing to register a broken stage.\n");
  }
  return ok;
}

export const LOCKED =
  "ERROR: marketplace stage is locked or its lock cannot be created";
export const NOT_A_CHECKOUT = "ERROR: marketplace source is not a Git checkout";
export const LEAF_CHANGED =
  "ERROR: marketplace stage leaf changed during refresh";

export type StageOutput = (stream: "stdout" | "stderr", text: string) => void;

export interface RemoveStageOptions {
  repoRoot?: string;
  /** The cache root; defaults to `cacheRoot(env)`. */
  cacheRoot?: string;
  /** The environment for the cache root default and the git child. */
  env?: Env;
  platform?: NodeJS.Platform;
  /** Receives each output line, newline included. */
  out?: StageOutput;
  /** Where interrupts arrive and how the process ends after cleaning up; defaults to the real process. */
  signals?: SignalSource;
  /** Test seam: runs once the lock is held, before the leaf is checked again, replaced or removed. */
  lockHook?: () => void | Promise<void>;
  /** Test seam: runs synchronously when an interrupt's cleanup starts, before anything is undone. */
  interruptHook?: () => void;
}

export interface RefreshStageOptions extends RemoveStageOptions {
  /** Test seam: runs once the leaf is the working directory, before the first file is written. */
  copyHook?: () => void | Promise<void>;
}

/** The interrupt seam: an emitter of signal events, the process exit, and the platform that picks the signals. */
export interface SignalSource {
  emitter: {
    on(signal: NodeJS.Signals, listener: () => void): unknown;
    off(signal: NodeJS.Signals, listener: () => void): unknown;
  };
  exit: (code: number) => void;
  platform: NodeJS.Platform;
}

const processSignals: SignalSource = {
  emitter: process,
  exit: (code) => process.exit(code),
  platform: process.platform,
};

/**
 * The interrupts refresh and remove clean up after, with the exit status each one ends with. A Windows console
 * process receives Ctrl+C and Ctrl+Break, which mean the user stopped it, and a hangup when its console window closes,
 * after which Windows ends it within seconds. Each status is 128 plus the signal number, as a shell reports it, except
 * that Ctrl+Break ends with the status of Ctrl+C.
 */
export function signalCodes(
  platform: NodeJS.Platform,
): ReadonlyMap<NodeJS.Signals, number> {
  return platform === "win32"
    ? new Map([
        ["SIGINT", 130],
        ["SIGBREAK", 130],
        ["SIGHUP", 129],
      ])
    : new Map([
        ["SIGINT", 130],
        ["SIGTERM", 143],
        ["SIGHUP", 129],
      ]);
}

/**
 * What an interrupt must undo, which changes as refresh or remove advances. Before the lock only the git child needs
 * stopping. "locked" and "removing" hold the lock. "copying" also owns the leaf, which is then the working directory
 * and is emptied, because a partial stage must never be registered.
 */
type CleanupScope = "unlocked" | "locked" | "copying" | "removing";

/**
 * The interrupt handlers for one refresh or remove, registered on creation and removed by `disarm`. The handler is
 * synchronous: the copy is asynchronous, so any await here would let it write another file after the cleanup. It
 * kills the git child and empties the leaf before it releases the lock, because a lock released first could let a
 * concurrent setup take it and then have its new stage emptied here. It then restores the caller's directory and
 * exits with the signal's status, or 1 when a cleanup step failed. It never re-raises the signal, so the caller sees
 * an exit status rather than death by signal. The listeners stay registered until just before the exit: removing the
 * last one restores the signal's default action, so a second Ctrl+C during the cleanup would kill the process with
 * the lock and a partial stage left behind. A repeated signal is ignored instead.
 */
class InterruptCleanup {
  child: ChildProcess | undefined;
  building = false;
  privateDir: string | undefined;
  #scope: CleanupScope = "unlocked";
  #cache: string | undefined;
  #interrupted = false;
  readonly #listeners = new Map<NodeJS.Signals, () => void>();
  readonly #source: SignalSource;
  readonly #platform: NodeJS.Platform;
  readonly #previous: string;
  readonly #hook: (() => void) | undefined;

  constructor(
    source: SignalSource,
    platform: NodeJS.Platform,
    previous: string,
    hook?: () => void,
  ) {
    this.#source = source;
    this.#platform = platform;
    this.#previous = previous;
    this.#hook = hook;
    for (const [signal, code] of signalCodes(source.platform)) {
      const listener = () => this.#interrupt(code);
      this.#listeners.set(signal, listener);
      source.emitter.on(signal, listener);
    }
  }

  enter(scope: CleanupScope, cache?: string): void {
    this.#scope = scope;
    if (cache !== undefined) this.#cache = cache;
  }

  disarm(): void {
    for (const [signal, listener] of this.#listeners)
      this.#source.emitter.off(signal, listener);
    this.#listeners.clear();
  }

  removePrivateDir(): void {
    if (this.privateDir === undefined) return;
    // Its name is relative to the verified cache. A replacement cache path
    // must never redirect a recursive removal elsewhere.
    process.chdir(this.#cache!);
    if (!cwdIs(this.#cache!, this.#platform)) throw new Error(INVALID_ROOT);
    rmSync(this.privateDir, { recursive: true, force: true });
    this.privateDir = undefined;
  }

  #interrupt(code: number): void {
    if (this.#interrupted) return;
    this.#interrupted = true;
    this.#hook?.();
    let status = code;
    try {
      if (this.building && this.child?.pid) {
        // The Go driver starts hooktool, which starts cross-compilers. Stop
        // the tree before removing its private output or releasing the lock.
        if (process.platform === "win32") {
          const taskkill = path.join(
            process.env.SystemRoot!,
            "System32",
            "taskkill.exe",
          );
          spawnSync(taskkill, ["/T", "/F", "/PID", String(this.child.pid)], {
            windowsHide: true,
          });
        } else process.kill(-this.child.pid, "SIGKILL");
      } else this.child?.kill();
    } catch {
      // A child that already exited has nothing left to stop.
    }
    if (this.#scope === "copying") {
      try {
        // The same anchored cleanup as a failed copy: relative to the entered leaf, removing links as links.
        for (const entry of readdirSync("."))
          rmSync(entry, { recursive: true, force: true });
      } catch {
        status = 1;
      }
    }
    try {
      this.removePrivateDir();
    } catch {
      status = 1;
    }
    if (this.#scope !== "unlocked" && this.#cache !== undefined)
      if (!releaseLock(this.#cache, this.#platform)) status = 1;
    restoreCwd(this.#previous);
    this.disarm();
    this.#source.exit(status);
  }
}

const defaultOutput: StageOutput = (stream, text) => {
  (stream === "stdout" ? process.stdout : process.stderr).write(text);
};

/**
 * The paths to copy, repository-relative with `/` separators: tracked plus untracked-but-not-ignored, read from the
 * working tree rather than HEAD so the maintainer's uncommitted edits reach the install, while `.gitignore` and `.git`
 * stay out by definition. Undefined when git is missing or the source is not a checkout. The child is asynchronous so
 * a signal handler can run while it lists.
 */
async function listStagePaths(
  repo: string,
  env: Env,
  platform: NodeJS.Platform,
  cleanup: InterruptCleanup,
): Promise<{ paths: string[]; trackedNative: Set<string> } | undefined> {
  const git = findCommand("git", env, platform);
  if (git === undefined) return undefined;
  let plan: SpawnPlan;
  try {
    plan = spawnPlan(
      git,
      [
        "-C",
        repo,
        "--git-dir",
        path.join(repo, ".git"),
        "--work-tree",
        repo,
        "ls-files",
        "-z",
        "-t",
        "--cached",
        "--others",
        "--exclude-standard",
      ],
      platform,
      env,
    );
  } catch {
    return undefined;
  }
  return new Promise((resolve) => {
    const chunks: Buffer[] = [];
    // Git hooks can export the invoking checkout's temporary index and common
    // directory. The selected plugin checkout owns this roster.
    const gitEnv = Object.fromEntries(
      Object.entries(env).filter(
        ([key]) =>
          ![
            "GIT_INDEX_FILE",
            "GIT_COMMON_DIR",
            "GIT_OBJECT_DIRECTORY",
            "GIT_ALTERNATE_OBJECT_DIRECTORIES",
          ].includes(key.toUpperCase()),
      ),
    );
    const child = spawn(plan.file, plan.args, {
      env: gitEnv,
      stdio: ["ignore", "pipe", "ignore"],
      windowsVerbatimArguments: plan.windowsVerbatimArguments,
      windowsHide: true,
    });
    cleanup.child = child;
    child.stdout.on("data", (chunk: Buffer) => chunks.push(chunk));
    child.on("error", () => {
      cleanup.child = undefined;
      resolve(undefined);
    });
    child.on("close", (code) => {
      cleanup.child = undefined;
      if (code !== 0) return resolve(undefined);
      const text = Buffer.concat(chunks).toString("utf8");
      const paths: string[] = [];
      const trackedNative = new Set<string>();
      for (const entry of text.split("\0")) {
        if (!entry) continue;
        const rel = entry.slice(2);
        paths.push(rel);
        if (!entry.startsWith("? ") && NATIVE_OUTPUTS.has(rel))
          trackedNative.add(rel);
      }
      resolve({ paths, trackedNative });
    });
  });
}

/** Whether the working directory's physical path is `expected`. */
function cwdIs(expected: string, platform: NodeJS.Platform): boolean {
  try {
    return (
      pathKey(physicalPath(".", platform), platform) ===
      pathKey(expected, platform)
    );
  } catch {
    return false;
  }
}

/**
 * Creates the parent directories of `rel` below the working directory, one component at a time. A component that
 * already exists must be a real directory, never a link: the file list can name a path below a link (a stale index
 * entry under a directory the maintainer replaced with a symlink), and writing through it would leave the stage.
 */
async function makeParents(rel: string, made: Set<string>): Promise<void> {
  const parts = rel.split("/").slice(0, -1);
  let current = "";
  for (const part of parts) {
    current = current === "" ? part : `${current}/${part}`;
    if (made.has(current)) continue;
    const info = await fsp.lstat(current).catch((error: unknown) => {
      if ((error as NodeJS.ErrnoException).code === "ENOENT") return undefined;
      throw error;
    });
    if (info === undefined) await fsp.mkdir(current);
    else if (info.isSymbolicLink() || !info.isDirectory())
      throw new Error(`${current} is not a directory in the stage`);
    made.add(current);
  }
}

/**
 * Copies the listed paths from the checkout into the working directory, which is the stage leaf. Every destination is
 * relative, so the copy stays in the directory that was entered even if the leaf's name is moved meanwhile. Regular
 * files keep their permission bits on POSIX. A symlink is recreated from its text and never followed; Windows gets a
 * regular file holding the text, as Git writes a link with `core.symlinks=false`. A path that vanished since it was
 * listed (a tracked file deleted without staging) is skipped. Returns the bytes written.
 */
async function copyStage(
  repo: string,
  paths: readonly string[],
  platform: NodeJS.Platform,
): Promise<number> {
  const made = new Set<string>();
  let size = 0;
  for (const rel of paths) {
    const source = path.join(repo, rel);
    const info = await fsp.lstat(source).catch(() => undefined);
    if (info === undefined) continue;
    if (info.isSymbolicLink()) {
      const target = await fsp.readlink(source);
      await makeParents(rel, made);
      if (platform === "win32")
        await fsp.writeFile(rel, target, { flag: "wx" });
      else await fsp.symlink(target, rel);
      size += Buffer.byteLength(target);
    } else if (info.isFile()) {
      await makeParents(rel, made);
      // EXCL refuses an existing destination, so a copy never writes through a link already in the stage.
      await fsp.copyFile(source, rel, fsConstants.COPYFILE_EXCL);
      if (platform !== "win32") await fsp.chmod(rel, info.mode & 0o7777);
      size += info.size;
    }
    // A directory entry is a nested repository or a submodule, which the stage does not carry.
  }
  return size;
}

/** Removes everything inside the working directory, never following a link. */
async function emptyWorkingDirectory(): Promise<void> {
  for (const entry of await fsp.readdir("."))
    await fsp.rm(entry, { recursive: true, force: true });
}

/**
 * A byte count the way `du -h` prints it: 1024-based `K`, `M` or `G`, rounded up, with one decimal below 10.
 */
export function formatSize(bytes: number): string {
  if (bytes <= 0) return "0";
  const units = ["K", "M", "G"];
  let value = bytes / 1024;
  let unit = 0;
  for (;;) {
    const shown = value < 10 ? Math.ceil(value * 10) / 10 : Math.ceil(value);
    if (shown >= 1024 && unit < units.length - 1) {
      value /= 1024;
      unit += 1;
      continue;
    }
    return `${shown < 10 ? shown.toFixed(1) : String(shown)}${units[unit]}`;
  }
}

/**
 * Enters the verified cache path and takes the lock, as the shared opening of refresh and remove. The process working
 * directory is the cache on success, so every later lock and leaf path is relative to it. Returns the cache path, or
 * an exit status after writing the reason.
 */
function enterAndLock(
  resolved: ResolvedStageRoot,
  create: boolean,
  err: (line: string) => void,
): { cache: string } | { status: number } {
  const entered = enterCache(resolved, { create });
  if (entered.status === "missing") return { status: 0 };
  if (entered.status === "failed") {
    err(entered.message);
    return { status: 1 };
  }
  try {
    process.chdir(entered.path);
  } catch {
    err(INVALID_ROOT);
    return { status: 1 };
  }
  if (!cwdIs(entered.path, resolved.platform)) {
    err(INVALID_ROOT);
    return { status: 1 };
  }
  const owned = checkOwnedLeaf(".");
  if (!owned.ok) {
    err(owned.message);
    return { status: 1 };
  }
  try {
    mkdirSync(LOCK_NAME);
  } catch {
    err(LOCKED);
    return { status: 1 };
  }
  return { cache: entered.path };
}

/**
 * Returns to the cache and releases the lock there. False when the cache is no longer the verified path, so a lock of
 * the same name elsewhere is never removed, or when the lock could not be removed.
 */
function releaseLock(cache: string, platform: NodeJS.Platform): boolean {
  try {
    process.chdir(cache);
    if (!cwdIs(cache, platform)) return false;
    rmdirSync(LOCK_NAME);
    return true;
  } catch {
    return false;
  }
}

function reason(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

function restoreCwd(previous: string): void {
  try {
    process.chdir(previous);
  } catch {
    // The caller's directory is gone; the stage has nothing to return to and nothing left to undo.
  }
}

/** Builds the install overlay under the lock, before replacing the current marketplace. */
async function buildDevelopmentFiles(
  resolved: ResolvedStageRoot,
  env: Env,
  cleanup: InterruptCleanup,
  out: StageOutput,
): Promise<{ root: string; digests: Map<string, string> }> {
  const go = findCommand("go", env, resolved.platform);
  if (!go)
    throw new Error(
      "source-only setup requires the local Go toolchain pinned in hooks/go/go.mod",
    );
  cleanup.privateDir = mkdtempSync(".agentics-dev-");
  const overlay = path.join(resolved.physical, cleanup.privateDir, "plugin");
  const buildEnv = {
    ...env,
    GOTOOLCHAIN: "local",
    GOPROXY: "off",
    GOSUMDB: "off",
    GOVCS: "*:off",
    GONOPROXY: "none",
    GOWORK: "off",
    GOENV: "off",
    GOFLAGS: "-mod=readonly",
    GOOS: "",
    GOARCH: "",
    CGO_ENABLED: "0",
  };
  const module = path.join(resolved.repo, "hooks/go");
  const runGo = (args: string[]) =>
    new Promise<string>((resolve, reject) => {
      const plan = spawnPlan(go, args, resolved.platform, buildEnv);
      const child = spawn(plan.file, plan.args, {
        cwd: module,
        env: buildEnv,
        stdio: ["ignore", "pipe", "pipe"],
        detached: process.platform !== "win32",
        windowsVerbatimArguments: plan.windowsVerbatimArguments,
        windowsHide: true,
      });
      cleanup.child = child;
      cleanup.building = true;
      let stdout = "";
      child.stdout.setEncoding("utf8").on("data", (chunk: string) => {
        stdout += chunk;
      });
      child.stderr
        .setEncoding("utf8")
        .on("data", (message: string) => out("stderr", message));
      child.on("error", reject);
      child.on("close", (code) => {
        cleanup.child = undefined;
        cleanup.building = false;
        if (code === 0) resolve(stdout.trim());
        else reject(new Error(`development build exited with ${code}`));
      });
    });
  const pin = /^go[ \t]+(\d+\.\d+\.\d+)[ \t]*\r?$/m.exec(
    readFileSync(path.join(module, "go.mod"), "utf8"),
  )?.[1];
  if (!pin)
    throw new Error(
      "hooks/go/go.mod requires an exact Go version for developer setup",
    );
  const version = await runGo(["env", "GOVERSION"]);
  if (version !== `go${pin}`)
    throw new Error(
      `source-only setup requires local go${pin}, found ${version}`,
    );
  await runGo([
    "run",
    "./cmd/hooktool",
    "stage-dev",
    "--repo",
    resolved.repo,
    "--out",
    overlay,
  ]);
  const digests = new Map<string, string>();
  for (const rel of DEVELOPMENT_FILES) {
    const file = path.join(overlay, rel);
    const digest = lstatSync(file).isFile() && sha256(file);
    if (!digest) throw new Error(`unreadable development artifact: ${rel}`);
    digests.set(rel, digest);
  }
  const expected = CONTRACT_BINARIES.map(
    (name) => `${digests.get(`hooks/bin/${name}`)}  bin/${name}\n`,
  ).join("");
  if (readFileSync(path.join(overlay, "hooks/SHA256SUMS"), "utf8") !== expected)
    throw new Error("development manifest does not match the built runtimes");
  return { root: overlay, digests };
}

/**
 * Rebuilds the stage leaf from the checkout's file list. The root is checked and the list
 * read before the cache is touched. Under the lock the leaf is replaced, entered, filled through relative paths and
 * verified; any failure there empties the leaf (no previous stage is kept). The leaf must still be the owned stage
 * directory afterwards. The lock is released however refresh ends, and a failed release fails the refresh.
 */
export async function refreshStage(
  options: RefreshStageOptions = {},
): Promise<number> {
  const env = options.env ?? process.env;
  const platform = options.platform ?? process.platform;
  const out = options.out ?? defaultOutput;
  const err = (line: string) => out("stderr", `${line}\n`);

  const resolved = resolveStageRoot({
    cacheRoot: options.cacheRoot,
    repoRoot: options.repoRoot,
    platform,
    env,
  });
  if (!resolved.ok) {
    err(resolved.message);
    return 1;
  }
  const previous = process.cwd();
  // Armed before git starts, so an interrupt while it lists stops it too.
  const cleanup = new InterruptCleanup(
    options.signals ?? processSignals,
    platform,
    previous,
    options.interruptHook,
  );
  try {
    const listed = await listStagePaths(resolved.repo, env, platform, cleanup);
    if (listed === undefined) {
      err(NOT_A_CHECKOUT);
      return 1;
    }
    if (
      listed.trackedNative.size !== 0 &&
      listed.trackedNative.size !== NATIVE_OUTPUTS.size
    ) {
      err("ERROR: incomplete tracked marketplace native artifact roster");
      return 1;
    }
    // Refresh creates a missing cache, so entering it never reports it missing.
    const locked = enterAndLock(resolved, true, err);
    if ("status" in locked) return locked.status;
    cleanup.enter("locked", locked.cache);
    let status = 1;
    try {
      await options.lockHook?.();
      const overlay =
        listed.trackedNative.size === 0
          ? await buildDevelopmentFiles(resolved, env, cleanup, out)
          : undefined;
      status = await fillLeaf(
        resolved,
        locked.cache,
        listed.paths,
        overlay,
        options,
        out,
        cleanup,
      );
    } catch (error) {
      err(`ERROR: marketplace stage refresh failed: ${reason(error)}`);
      status = 1;
    } finally {
      try {
        cleanup.removePrivateDir();
      } catch (error) {
        err(`ERROR: marketplace build cleanup failed: ${reason(error)}`);
        status = 1;
      }
      if (!releaseLock(locked.cache, platform)) status = 1;
      // Removed with the lock, so a later interrupt in the caller cannot clean up a finished stage.
      cleanup.disarm();
    }
    return status;
  } finally {
    cleanup.disarm();
    restoreCwd(previous);
  }
}

/** The locked part of refresh; the working directory is the cache on entry and may be anywhere on return. */
async function fillLeaf(
  resolved: ResolvedStageRoot,
  cache: string,
  paths: readonly string[],
  overlay: Awaited<ReturnType<typeof buildDevelopmentFiles>> | undefined,
  options: RefreshStageOptions,
  out: StageOutput,
  cleanup: InterruptCleanup,
): Promise<number> {
  const { platform } = resolved;
  const err = (line: string) => out("stderr", `${line}\n`);
  const owned = checkOwnedLeaf(".");
  if (!owned.ok) {
    err(owned.message);
    return 1;
  }
  // rm removes a leaf that became a link as the link itself, never its target.
  await fsp.rm(STAGE_NAME, { recursive: true, force: true });
  await fsp.mkdir(STAGE_NAME);
  process.chdir(STAGE_NAME);
  if (!cwdIs(resolved.stageDir, platform)) {
    err(LEAF_CHANGED);
    return 1;
  }
  cleanup.enter("copying");

  let size: number;
  try {
    await options.copyHook?.();
    const sourcePaths = overlay
      ? paths.filter((rel) => {
          // Git lists slash-separated paths on every host. Reserve the same ASCII
          // spellings for generated files on case-sensitive and case-insensitive volumes.
          const key = rel.replace(/[A-Z]/g, (letter) => letter.toLowerCase());
          return !GENERATED_PATH_KEYS.some((generated) =>
            within(key, generated, "/"),
          );
        })
      : paths;
    size = await copyStage(resolved.repo, sourcePaths, platform);
    if (overlay) {
      size += await copyStage(overlay.root, DEVELOPMENT_FILES, platform);
      for (const [rel, digest] of overlay.digests) {
        if (sha256(rel) !== digest)
          throw new Error(`development artifact differs after copy: ${rel}`);
      }
      if (
        !isFile("bin/agx") ||
        (platform !== "win32" && !(statSync("bin/agx").mode & 0o111))
      )
        throw new Error("missing executable POSIX launcher bin/agx");
    }
    const verified = verifyStage(overlay?.root ?? resolved.repo, ".", {
      platform,
      write: (line) => out("stderr", line),
    });
    if (!verified) throw new VerifyFailed();
    // A complete, verified stage is kept if an interrupt arrives from here on.
    cleanup.enter("locked");
  } catch (error) {
    if (!(error instanceof VerifyFailed))
      err(`ERROR: marketplace stage copy failed: ${reason(error)}`);
    await emptyWorkingDirectory();
    return 1;
  }
  out("stdout", `Staged ${formatSize(size)} of tracked plugin content.\n`);

  process.chdir(cache);
  const leaf = checkOwnedLeaf(".");
  let stays = leaf.ok && cwdIs(cache, platform);
  if (stays) {
    try {
      stays =
        lstatSync(STAGE_NAME).isDirectory() &&
        pathKey(physicalPath(STAGE_NAME, platform), platform) ===
          pathKey(resolved.stageDir, platform);
    } catch {
      stays = false;
    }
  }
  if (!stays) {
    err(LEAF_CHANGED);
    return 1;
  }
  return 0;
}

class VerifyFailed extends Error {}

/**
 * Removes the stage leaf. A missing cache or leaf is already removed and returns 0 without
 * creating anything. The leaf is checked before and after the lock is taken, and removed as a link if it became one.
 */
export async function removeStage(
  options: RemoveStageOptions = {},
): Promise<number> {
  const env = options.env ?? process.env;
  const platform = options.platform ?? process.platform;
  const out = options.out ?? defaultOutput;
  const err = (line: string) => out("stderr", `${line}\n`);

  const resolved = resolveStageRoot({
    cacheRoot: options.cacheRoot,
    repoRoot: options.repoRoot,
    platform,
    env,
  });
  if (!resolved.ok) {
    err(resolved.message);
    return 1;
  }

  const previous = process.cwd();
  let cleanup: InterruptCleanup | undefined;
  try {
    const locked = enterAndLock(resolved, false, err);
    if ("status" in locked) return locked.status;
    cleanup = new InterruptCleanup(
      options.signals ?? processSignals,
      platform,
      previous,
      options.interruptHook,
    );
    cleanup.enter("removing", locked.cache);
    let status = 1;
    try {
      await options.lockHook?.();
      const owned = checkOwnedLeaf(".");
      if (!owned.ok) err(owned.message);
      else if (lstatSync(STAGE_NAME, { throwIfNoEntry: false }) === undefined)
        status = 0;
      else {
        out(
          "stdout",
          `Removing local marketplace stage (${resolved.stageDir})...\n`,
        );
        await fsp.rm(STAGE_NAME, { recursive: true, force: true });
        status = 0;
      }
    } catch (error) {
      err(`ERROR: marketplace stage removal failed: ${reason(error)}`);
      status = 1;
    } finally {
      if (!releaseLock(locked.cache, platform)) status = 1;
      cleanup.disarm();
    }
    return status;
  } finally {
    cleanup?.disarm();
    restoreCwd(previous);
  }
}
