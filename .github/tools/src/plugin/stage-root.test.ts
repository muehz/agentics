import assert from "node:assert/strict";
import {
  existsSync,
  lstatSync,
  mkdirSync,
  mkdtempSync,
  readdirSync,
  readFileSync,
  realpathSync,
  rmSync,
  symlinkSync,
  writeFileSync,
} from "node:fs";
import os from "node:os";
import path from "node:path";
import { test, type TestContext } from "node:test";
import { cacheRoot } from "./common.ts";
import {
  INVALID_ROOT,
  LOCK_NAME,
  STAGE_NAME,
  UNOWNED_LEAF,
  checkOwnedLeaf,
  enterCache,
  pathKey,
  resolveStageRoot,
  type ResolvedStageRoot,
} from "./stage.ts";

const isWindows = process.platform === "win32";

function temporary(t: TestContext): string {
  const dir = realpathSync.native(
    mkdtempSync(path.join(os.tmpdir(), "agentics-stage-root-")),
  );
  t.after(() => rmSync(dir, { recursive: true, force: true }));
  return dir;
}

/** A plain directory standing in for the checkout, with a sentinel the rules must never touch. */
function fakeRepo(dir: string, sentinel = "sentinel"): string {
  mkdirSync(path.dirname(path.join(dir, sentinel)), { recursive: true });
  writeFileSync(path.join(dir, sentinel), "do not change");
  return dir;
}

// A junction needs no privilege on Windows, unlike a directory symlink.
function linkDirectory(target: string, link: string): void {
  symlinkSync(target, link, isWindows ? "junction" : "dir");
}

function createdPaths(
  cache: string,
  platform: NodeJS.Platform,
): Record<string, boolean> {
  // A UNC path cannot be created by a local mkdir, and probing one on Windows waits on a network lookup.
  if (isWindows && /^[\\/]{2}/.test(cache)) return {};
  const api = platform === "win32" ? path.win32 : path.posix;
  const paths = [
    cache,
    api.join(cache, "agentics"),
    api.join(cache, STAGE_NAME),
    api.join(cache, LOCK_NAME),
  ];
  return Object.fromEntries(paths.map((p) => [p, present(p)]));
}

// Below a regular file POSIX reports ENOTDIR instead of ENOENT; either way nothing is there.
function present(p: string): boolean {
  try {
    return lstatSync(p, { throwIfNoEntry: false }) !== undefined;
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code === "ENOTDIR") return false;
    throw error;
  }
}

/** Asserts the root is rejected with the shared message and that the cache, tool cache, stage and lock are unchanged. */
function assertRejected(
  cache: string,
  options: { repoRoot?: string; platform?: NodeJS.Platform } = {},
): void {
  const platform = options.platform ?? process.platform;
  const before = createdPaths(cache, platform);
  const result = resolveStageRoot({ cacheRoot: cache, ...options });
  assert.equal(result.ok, false, `accepted ${JSON.stringify(cache)}`);
  assert.equal(!result.ok && result.message, INVALID_ROOT);
  assert.deepEqual(createdPaths(cache, platform), before, cache);
}

function assertAbsent(...paths: string[]): void {
  for (const p of paths)
    assert.equal(lstatSync(p, { throwIfNoEntry: false }), undefined, p);
}

function resolved(cache: string, repoRoot: string): ResolvedStageRoot {
  const result = resolveStageRoot({ cacheRoot: cache, repoRoot });
  assert.ok(result.ok, `rejected ${cache}`);
  return result;
}

test("stage root: rejects an empty or relative root", (t) => {
  const repo = fakeRepo(temporary(t));
  for (const platform of ["linux", "win32"] as const)
    for (const cache of ["", ".", "relative-cache"])
      assertRejected(cache, { repoRoot: repo, platform });
  // An empty XDG_CACHE_HOME counts as unset, so HOME decides the root.
  const home = temporary(t);
  const result = resolveStageRoot({
    env: { XDG_CACHE_HOME: "", HOME: home },
    repoRoot: repo,
  });
  assert.ok(result.ok);
  assert.equal(result.physical, path.join(home, ".cache"));
});

test("stage root: rejects POSIX filesystem roots", () => {
  for (const cache of ["/", "/.cache", "/.cache/", "//", "/.cache//"])
    assertRejected(cache, { platform: "linux" });
});

test("stage root: rejects Windows roots and unanchored paths", () => {
  for (const cache of [
    "C:\\",
    "C:/",
    "c:\\.cache",
    "C:\\.CACHE\\",
    "\\\\server\\share",
    "\\\\server\\share\\",
    "\\\\server\\share\\.cache",
    "C:cache",
    "\\cache",
    "\\\\?\\C:\\cache",
  ])
    assertRejected(cache, { platform: "win32" });
});

test("stage root: rejects dot components without normalizing them", () => {
  for (const cache of ["/a/./b", "/a/../b", "/a/.", "/a/.."])
    assertRejected(cache, { platform: "linux" });
  for (const cache of ["C:\\a\\..\\b", "C:\\a\\.", "C:/a/../b"])
    assertRejected(cache, { platform: "win32" });
});

test("stage root: strips trailing separators before resolving", (t) => {
  const dir = temporary(t);
  const repo = fakeRepo(path.join(dir, "repo"));
  const result = resolved(`${dir}/cache///`, repo);
  assert.equal(result.physical, path.join(dir, "cache"));
  assert.deepEqual(result.missing, ["cache"]);
  assert.equal(result.stageDir, path.join(dir, "cache", STAGE_NAME));
});

test("stage root: path keys fold case on win32 and keep it on posix", () => {
  assert.equal(pathKey("C:\\Users\\Cache", "win32"), "c:\\users\\cache");
  assert.equal(pathKey("C:/Users/Cache", "win32"), "c:\\users\\cache");
  assert.equal(pathKey("/Users/Cache", "linux"), "/Users/Cache");
  assert.equal(pathKey("/Users/Cache", "darwin"), "/Users/Cache");
});

test("stage root: rejects a cache equal to or inside the repository", (t) => {
  const repo = fakeRepo(temporary(t));
  assertRejected(repo, { repoRoot: repo });
  assertRejected(path.join(repo, "cache"), { repoRoot: repo });
  assert.equal(
    readFileSync(path.join(repo, "sentinel"), "utf8"),
    "do not change",
  );
});

// oracle: TestStageRejectsDerivedStageContainingCheckout
test("stage root: rejects derived stage containing checkout", (t) => {
  for (const layout of [[STAGE_NAME], [STAGE_NAME, "checkout"]]) {
    const cache = temporary(t);
    const repo = fakeRepo(path.join(cache, ...layout));
    assertRejected(cache, { repoRoot: repo });
    assert.equal(
      readFileSync(path.join(repo, "sentinel"), "utf8"),
      "do not change",
    );
    assertAbsent(path.join(cache, LOCK_NAME));
  }
});

// oracle: TestStageRejectsUnsafeCacheWithoutMutation
test("stage root: rejects unsafe cache without mutation", (t) => {
  const repo = fakeRepo(temporary(t), path.join(STAGE_NAME, "sentinel"));
  const outside = temporary(t);
  const alias = path.join(outside, "alias");
  linkDirectory(repo, alias);
  for (const cache of [
    ".",
    "relative-cache",
    repo,
    path.join(repo, "cache"),
    alias,
    `${outside}/../${path.basename(outside)}/alias`,
  ]) {
    assertRejected(cache, { repoRoot: repo });
    assert.equal(
      readFileSync(path.join(repo, STAGE_NAME, "sentinel"), "utf8"),
      "do not change",
    );
  }
});

test("stage root: accepts a repository elsewhere under the cache", (t) => {
  const cache = temporary(t);
  const repo = fakeRepo(path.join(cache, "checkout"));
  const result = resolved(cache, repo);
  assert.equal(result.physical, cache);
  assert.deepEqual(result.missing, []);
  assert.equal(result.repo, repo);
});

test("stage root: resolves a linked directory ancestor to its physical path", (t) => {
  const dir = temporary(t);
  const repo = fakeRepo(path.join(dir, "repo"));
  mkdirSync(path.join(dir, "real"));
  linkDirectory(path.join(dir, "real"), path.join(dir, "link"));
  const result = resolved(path.join(dir, "link", "cache"), repo);
  assert.equal(result.ancestor, path.join(dir, "real"));
  assert.equal(result.physical, path.join(dir, "real", "cache"));
  assert.deepEqual(result.missing, ["cache"]);
});

test("stage root: rejects a path component that is a regular file", (t) => {
  const dir = temporary(t);
  const repo = fakeRepo(path.join(dir, "repo"));
  writeFileSync(path.join(dir, "file"), "x");
  assertRejected(path.join(dir, "file"), { repoRoot: repo });
  assertRejected(path.join(dir, "file", "cache"), { repoRoot: repo });
});

test(
  "stage root: rejects a dangling symlink component",
  { skip: isWindows ? "POSIX only: dangling symlinks need privilege" : false },
  (t) => {
    const dir = temporary(t);
    const repo = fakeRepo(path.join(dir, "repo"));
    symlinkSync(path.join(dir, "nowhere"), path.join(dir, "dangling"));
    assertRejected(path.join(dir, "dangling"), { repoRoot: repo });
    assertRejected(path.join(dir, "dangling", "cache"), { repoRoot: repo });
    assertAbsent(path.join(dir, "nowhere"));
  },
);

/**
 * Whether the temporary volume resolves a path spelled in another case, which Windows and macOS APFS do by default
 * and most Linux filesystems do not. The probe spells the whole path in upper case, as the test below does.
 */
function caseSensitiveSkip(): string | false {
  const probe = realpathSync.native(
    mkdtempSync(path.join(os.tmpdir(), "agentics-stage-case-")),
  );
  try {
    return existsSync(probe.toUpperCase())
      ? false
      : "case-sensitive volume: the temporary directory does not resolve in another case";
  } finally {
    rmSync(probe, { recursive: true, force: true });
  }
}

test(
  "stage root: rejects a differently cased alias of the repository",
  { skip: caseSensitiveSkip() },
  (t) => {
    const repo = fakeRepo(temporary(t));
    assertRejected(repo.toUpperCase(), { repoRoot: repo });
    assertRejected(path.join(repo.toLowerCase(), "cache"), { repoRoot: repo });
  },
);

function shortNameSkip(): string | false {
  if (!isWindows) return "Windows only: 8.3 short names";
  const probe = realpathSync.native(
    mkdtempSync(path.join(os.tmpdir(), "agentics-stage-83-")),
  );
  try {
    mkdirSync(path.join(probe, "longdirectoryname"));
    return existsSync(path.join(probe, "LONGDI~1"))
      ? false
      : "Windows only: no 8.3 short names on the temp volume";
  } finally {
    rmSync(probe, { recursive: true, force: true });
  }
}

test(
  "stage root: rejects an 8.3 short-name alias whose stage holds the repository",
  { skip: shortNameSkip() },
  (t) => {
    const dir = temporary(t);
    const long = path.join(dir, "longdirectoryname");
    const repo = fakeRepo(path.join(long, STAGE_NAME));
    const short = path.join(dir, "LONGDI~1");
    assert.ok(existsSync(short), short);
    assertRejected(short, { repoRoot: repo });
  },
);

test("stage root: rejects the root when the repository does not resolve", (t) => {
  const dir = temporary(t);
  assertRejected(path.join(dir, "cache"), {
    repoRoot: path.join(dir, "no-such-repo"),
  });
});

test("stage root: accepts the default root under HOME", (t) => {
  const home = temporary(t);
  const repo = fakeRepo(temporary(t));
  for (const result of [
    resolveStageRoot({ cacheRoot: cacheRoot({ HOME: home }), repoRoot: repo }),
    resolveStageRoot({ env: { HOME: home }, repoRoot: repo }),
  ]) {
    assert.ok(result.ok);
    assert.equal(result.stageDir, path.join(home, ".cache", STAGE_NAME));
    assert.equal(result.toolCacheDir, path.join(home, ".cache", "agentics"));
  }
  assertAbsent(path.join(home, ".cache"));
});

test("stage root: entry with create makes each missing component", (t) => {
  const dir = temporary(t);
  const repo = fakeRepo(path.join(dir, "repo"));
  const root = resolved(path.join(dir, "missing cache", "nested"), repo);
  assert.deepEqual(root.missing, ["missing cache", "nested"]);
  const entry = enterCache(root, { create: true });
  assert.deepEqual(entry, { status: "ok", path: root.physical });
  assert.ok(lstatSync(root.physical).isDirectory());
  assert.deepEqual(readdirSync(root.physical), []);
});

test("stage root: entry without create reports a missing cache", (t) => {
  const dir = temporary(t);
  const repo = fakeRepo(path.join(dir, "repo"));
  const root = resolved(path.join(dir, "missing", "nested"), repo);
  assert.deepEqual(enterCache(root, { create: false }), { status: "missing" });
  assertAbsent(path.join(dir, "missing"));
});

test("stage root: entry rejects a component that became a link", (t) => {
  const dir = temporary(t);
  const repo = fakeRepo(path.join(dir, "repo"));
  const target = path.join(dir, "target");
  mkdirSync(target);
  writeFileSync(path.join(target, "keep"), "keep");
  const root = resolved(path.join(dir, "cache", "nested"), repo);
  linkDirectory(target, path.join(dir, "cache"));
  for (const create of [true, false]) {
    const entry = enterCache(root, { create });
    assert.deepEqual(entry, { status: "failed", message: INVALID_ROOT });
  }
  assert.deepEqual(readdirSync(target), ["keep"]);
});

test("stage root: entry rejects a component that became a file", (t) => {
  // The file is an intermediate component in one layout and the cache itself in the other.
  for (const parts of [["cache", "nested"], ["cache"]]) {
    const dir = temporary(t);
    const repo = fakeRepo(path.join(dir, "repo"));
    const root = resolved(path.join(dir, ...parts), repo);
    writeFileSync(path.join(dir, "cache"), "file");
    for (const create of [true, false])
      assert.deepEqual(enterCache(root, { create }), {
        status: "failed",
        message: INVALID_ROOT,
      });
    assert.equal(readFileSync(path.join(dir, "cache"), "utf8"), "file");
  }
});

test("stage root: leaf check accepts a missing or directory leaf", (t) => {
  const cache = temporary(t);
  assert.deepEqual(checkOwnedLeaf(cache), { ok: true });
  mkdirSync(path.join(cache, STAGE_NAME));
  assert.deepEqual(checkOwnedLeaf(cache), { ok: true });
});

test("stage root: leaf check rejects a file or linked leaf", (t) => {
  const fileCache = temporary(t);
  writeFileSync(path.join(fileCache, STAGE_NAME), "file");
  assert.deepEqual(checkOwnedLeaf(fileCache), {
    ok: false,
    message: UNOWNED_LEAF,
  });

  const linkCache = temporary(t);
  const target = temporary(t);
  writeFileSync(path.join(target, "keep"), "keep");
  linkDirectory(target, path.join(linkCache, STAGE_NAME));
  assert.deepEqual(checkOwnedLeaf(linkCache), {
    ok: false,
    message: UNOWNED_LEAF,
  });
  assert.deepEqual(readdirSync(target), ["keep"]);
  assert.equal(readFileSync(path.join(target, "keep"), "utf8"), "keep");
});
