import { spawnSync } from "node:child_process";
import { existsSync, lstatSync, readdirSync, type Dirent } from "node:fs";
import path from "node:path";
import { describeChildFailure } from "./domains/tools/exec.ts";
import { RepoFileError, repoFileErrorOf } from "./repo-file-error.ts";
import { isWorkFolderTempDir } from "./work-folder-temp.ts";

const GIT_LOCATION = new Set([
  "GIT_DIR",
  "GIT_WORK_TREE",
  "GIT_INDEX_FILE",
  "GIT_COMMON_DIR",
]);

/**
 * The environment without the variables that tell git where a repository is, matched by upper-cased name because
 * Windows variable names are case-insensitive. A caller's GIT_DIR or GIT_INDEX_FILE would point git at another
 * repository than the one it is asked about.
 */
export function withoutGitLocation(
  env: Record<string, string | undefined>,
): Record<string, string> {
  // Object.fromEntries defines each key as an own property, so a key such as __proto__ cannot reach the prototype the
  // way an assignment would.
  return Object.fromEntries(
    Object.entries(env).filter(
      (entry): entry is [string, string] =>
        entry[1] !== undefined && !GIT_LOCATION.has(entry[0].toUpperCase()),
    ),
  );
}

/** The files git tracks under the root and every directory that holds one. Both are empty outside a checkout. */
export interface Tracked {
  files: ReadonlySet<string>;
  dirs: ReadonlySet<string>;
}

/**
 * Lists the tracked files of a checkout. A tree without `.git` (a fixture) has none, and the walk then skips every
 * skipped directory whole. Inside a checkout a failed git is an error, never a silent fallback.
 */
export function trackedPaths(
  repoRoot: string,
  env: Record<string, string | undefined>,
): Tracked {
  if (!existsSync(path.join(repoRoot, ".git")))
    return { files: new Set(), dirs: new Set() };
  const result = spawnSync("git", ["-C", repoRoot, "ls-files", "-z"], {
    env: withoutGitLocation(env),
    maxBuffer: 1024 ** 3,
  });
  if (result.error)
    throw new Error(
      `listing tracked files: ${describeChildFailure("git", { startError: result.error.message })}`,
    );
  if (result.signal !== null)
    throw new Error(
      `listing tracked files: ${describeChildFailure("git", { signal: result.signal })}`,
    );
  if (result.status !== 0)
    throw new Error(
      `listing tracked files: ${describeChildFailure("git", { code: result.status ?? -1 })}: ${result.stderr.toString("utf8").trim()}`,
    );
  const files = new Set<string>();
  const dirs = new Set<string>();
  for (const tracked of result.stdout.toString("utf8").split("\0")) {
    if (tracked === "") continue;
    files.add(tracked);
    for (
      let dir = path.posix.dirname(tracked);
      dir !== ".";
      dir = path.posix.dirname(dir)
    )
      dirs.add(dir);
  }
  return { files, dirs };
}

export type TreeEntry = {
  name: string;
  isDirectory: boolean;
  isSymlink: boolean;
};
export type FileInfo = { isDirectory(): boolean; isSymbolicLink(): boolean };
export type WalkedFile = { absolute: string; rel: string; name: string };
type SkillVisit = (rel: string, entry: TreeEntry) => "skip" | void;
type SkillsSource = {
  lstat: (file: string) => FileInfo;
  /** Entries in the order the skills visitor should receive them. */
  readDir: (dir: string) => TreeEntry[];
};

type RepoWalk = {
  mode: "repo-hygiene" | "repo-area";
  skipDirs: ReadonlySet<string>;
  tracked: Tracked;
  visit: (file: WalkedFile) => void;
};
type ManifestWalk = {
  mode: "manifests";
  skipDirs: ReadonlySet<string>;
  tracked?: () => Tracked;
  visit: (rel: string, entry: Dirent) => void;
};
type SkillsWalk =
  | {
      mode: "skills-tree";
      skipDirs: ReadonlySet<string>;
      tracked?: () => Tracked;
    }
  | { mode: "skills-dir"; base?: string };
type Walk =
  | RepoWalk
  | ManifestWalk
  | (SkillsWalk & { source?: SkillsSource; visit: SkillVisit });

function nativeEntries(dir: string): Dirent[] {
  return readdirSync(dir, { withFileTypes: true }).sort((a, b) =>
    a.name < b.name ? -1 : a.name > b.name ? 1 : 0,
  );
}

/** The ordered, link-aware entries supplied by the default skills filesystem. */
export function readTreeEntries(dir: string): TreeEntry[] {
  return nativeEntries(dir).map((entry) => ({
    name: entry.name,
    isDirectory: entry.isDirectory(),
    isSymlink: entry.isSymbolicLink(),
  }));
}

function readAt<T>(rel: string, read: () => T): T {
  try {
    return read();
  } catch (error) {
    throw repoFileErrorOf(error, rel);
  }
}

type EntryKind = "directory" | "link" | "leaf";

function directoryScope(
  walk: Walk,
  rel: string,
  name: string,
): "all" | "tracked" | "skip" {
  switch (walk.mode) {
    case "skills-dir":
      return "all";
    case "repo-hygiene":
    case "repo-area":
      if (walk.mode === "repo-area" && name === "testdata") return "skip";
      if (walk.skipDirs.has(rel) || walk.skipDirs.has(name)) return "tracked";
      break;
    case "skills-tree":
    case "manifests":
      if (walk.mode === "skills-tree" && rel === "evals") return "skip";
      if (walk.skipDirs.has(name)) return "skip";
      break;
  }
  return isWorkFolderTempDir(rel) ? "tracked" : "all";
}

type Entries<E> = {
  read: (dir: string) => E[];
  kind: (entry: E) => EntryKind;
  visit: (
    rel: string,
    entry: E,
    absolute: string,
    kind: EntryKind,
  ) => "skip" | void;
};

/** Only source reads are translated to file errors. Tracked lookup and visitors own their errors. */
function walkChildren<E extends { name: string }>(
  root: string,
  base: string,
  walk: Walk,
  entries: Entries<E>,
): void {
  const tracked = "tracked" in walk ? walk.tracked : undefined;
  const descend = (
    dir: string,
    prefix: string,
    shown: string,
    partial?: Tracked,
  ): void => {
    for (const entry of readAt(shown, () => entries.read(dir))) {
      const kind = entries.kind(entry);
      if (kind === "link" && walk.mode === "skills-tree") continue;
      const rel = prefix === "" ? entry.name : `${prefix}/${entry.name}`;
      let inside = partial;
      if (kind === "directory") {
        const scope = directoryScope(walk, rel, entry.name);
        if (scope === "skip") continue;
        if (scope === "tracked" && inside === undefined) {
          inside = typeof tracked === "function" ? tracked() : tracked;
        }
      }
      if (
        inside &&
        !(kind === "directory" ? inside.dirs : inside.files).has(rel)
      )
        continue;
      const absolute = path.join(dir, entry.name);
      const result = entries.visit(rel, entry, absolute, kind);
      if (kind === "directory" && result !== "skip")
        descend(absolute, rel, base === "." ? rel : `${base}/${rel}`, inside);
    }
  };
  descend(root, "", base);
}

const nativeKind = (entry: Dirent): EntryKind =>
  entry.isSymbolicLink() ? "link" : entry.isDirectory() ? "directory" : "leaf";

/**
 * Visits entries in depth-first preorder. Repo modes deliver eligible leaves, manifests receives native dirents,
 * and skills visitors may prune directories. The skills directory mode reports errors below its supplied base.
 * Explicit-path checks belong to the caller before walking a selected path.
 */
export function walkCheckout(root: string, walk: Walk): void {
  switch (walk.mode) {
    case "skills-tree":
    case "skills-dir": {
      const base =
        walk.mode === "skills-dir"
          ? (walk.base ?? root.replaceAll("\\", "/"))
          : ".";
      const source = walk.source ?? {
        lstat: lstatSync,
        readDir: readTreeEntries,
      };
      const info = readAt(base, () => source.lstat(root));
      const isSymlink = info.isSymbolicLink();
      if (isSymlink || !info.isDirectory()) {
        if (walk.mode === "skills-dir")
          walk.visit(".", {
            name: path.basename(root),
            isDirectory: false,
            isSymlink,
          });
        return;
      }
      walkChildren(root, base, walk, {
        read: (dir) => source.readDir(dir),
        kind: (entry) =>
          entry.isSymlink ? "link" : entry.isDirectory ? "directory" : "leaf",
        visit: walk.visit,
      });
      return;
    }
    case "manifests": {
      const info = readAt(".", () => lstatSync(root));
      if (info.isSymbolicLink()) throw new RepoFileError("symlink", ".");
      if (!info.isDirectory()) throw new RepoFileError("not-a-directory", ".");
      walkChildren(root, ".", walk, {
        read: nativeEntries,
        kind: nativeKind,
        visit: (rel, entry) => {
          walk.visit(rel, entry);
        },
      });
      return;
    }
    case "repo-hygiene":
    case "repo-area": {
      const info = readAt(".", () => lstatSync(root));
      const name = path.basename(root);
      if (!info.isDirectory()) {
        if (walk.mode === "repo-area" || info.isFile())
          walk.visit({ absolute: root, rel: ".", name });
        return;
      }
      if (
        walk.mode === "repo-hygiene" &&
        directoryScope(walk, ".", name) === "tracked" &&
        !walk.tracked.dirs.has(".")
      )
        return;
      walkChildren(root, ".", walk, {
        read: nativeEntries,
        kind: nativeKind,
        visit: (rel, entry, absolute, kind) => {
          if (
            kind !== "directory" &&
            (walk.mode === "repo-area" || entry.isFile())
          )
            walk.visit({ absolute, rel, name: entry.name });
        },
      });
    }
  }
}
