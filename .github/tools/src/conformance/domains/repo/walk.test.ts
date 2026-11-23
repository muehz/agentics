import assert from "node:assert/strict";
import fs, { mkdirSync, mkdtempSync, symlinkSync } from "node:fs";
import { syncBuiltinESMExports } from "node:module";
import { tmpdir } from "node:os";
import path from "node:path";
import { test } from "node:test";
import { RepoFileError } from "../../repo-file-error.ts";
import {
  initCheckout,
  tempIsInsideRepository,
} from "../../testing/git-checkout.ts";
import { writeFile } from "../../testing/files.ts";
import {
  trackedPaths,
  walkCheckout,
  withoutGitLocation,
  type Tracked,
} from "./walk.ts";
import { removeTree } from "../../testing/host.ts";

const NONE: Tracked = { files: new Set(), dirs: new Set() };

function withTree(body: (root: string) => void): void {
  const root = mkdtempSync(path.join(tmpdir(), "agentics-repowalk-"));
  try {
    body(root);
  } finally {
    removeTree(root);
  }
}

const visited = (
  root: string,
  tracked: Tracked,
  mode: "hygiene" | "area",
): string[] => {
  const seen: string[] = [];
  walkCheckout(root, tracked, mode, (file) => seen.push(file.rel));
  return seen;
};

test("a tree without .git has no tracked paths", () => {
  withTree((root) => {
    writeFile(root, "a.txt", "x");
    assert.deepEqual(trackedPaths(root, process.env), NONE);
  });
});

test("a checkout lists tracked files and the directories that hold one", () => {
  withTree((root) => {
    writeFile(root, "a/b/c.txt", "x");
    writeFile(root, "d.txt", "x");
    writeFile(root, "untracked.txt", "x");
    initCheckout(root, ["a/b/c.txt", "d.txt"]);
    const tracked = trackedPaths(root, process.env);
    assert.deepEqual([...tracked.files].sort(), ["a/b/c.txt", "d.txt"]);
    assert.deepEqual([...tracked.dirs].sort(), ["a", "a/b"]);
  });
});

test("a caller's GIT_DIR does not redirect the listing", () => {
  withTree((root) => {
    writeFile(root, "a.txt", "x");
    initCheckout(root, ["a.txt"]);
    const elsewhere = path.join(root, "nowhere");
    const tracked = trackedPaths(root, { ...process.env, GIT_DIR: elsewhere });
    assert.deepEqual([...tracked.files], ["a.txt"]);
  });
});

test("a caller's other git location variables do not redirect the listing", () => {
  withTree((root) => {
    writeFile(root, "a.txt", "x");
    initCheckout(root, ["a.txt"]);
    const elsewhere = path.join(root, "nowhere");
    // A work tree above the root turns the root into a subdirectory of it, so an unstripped call lists nothing.
    // The lowercase name bites only where environment variable names are case-insensitive (Windows); on Linux git
    // ignores it and the assertion passes either way.
    const redirects: Record<string, string> = {
      GIT_WORK_TREE: path.dirname(root),
      GIT_INDEX_FILE: elsewhere,
      GIT_COMMON_DIR: elsewhere,
      git_dir: elsewhere,
    };
    for (const [key, value] of Object.entries(redirects)) {
      const tracked = trackedPaths(root, { ...process.env, [key]: value });
      assert.deepEqual([...tracked.files], ["a.txt"], key);
    }
  });
});

test(
  "a .git that is not a repository is an error",
  {
    skip: tempIsInsideRepository()
      ? "the temp directory is inside a git repository"
      : false,
  },
  () => {
    withTree((root) => {
      mkdirSync(path.join(root, ".git"));
      assert.throws(
        () => trackedPaths(root, process.env),
        /^Error: listing tracked files: git exited with code 128: /,
      );
    });
  },
);

test("a git that cannot start is an error that says so", () => {
  withTree((root) => {
    mkdirSync(path.join(root, ".git"));
    const env = { ...process.env, PATH: root, Path: undefined };
    assert.throws(
      () => trackedPaths(root, env),
      /^Error: listing tracked files: git could not start: /,
    );
  });
});

test("hygiene mode skips each skip-list entry and visits the rest in sorted order", () => {
  withTree((root) => {
    // Skipped by name at any depth, and by path only at that path.
    for (const rel of [
      "sub/.git/x",
      "node_modules/x",
      "a/node_modules/x",
      ".worktrees/x",
      ".agentics/x",
      ".audit/x",
      ".claude/worktrees/x",
      "evals/runs/x",
      "docs/handoffs/x",
      "docs/probes/x",
      "docs/reviews/peer/x",
    ])
      writeFile(root, rel, "skipped");
    // Not skipped: the entries with a slash match only at their own path, and no entry matches as a prefix.
    for (const rel of [
      "x/evals/runs/y",
      "docs/handoffs.md",
      "worktrees/x",
      "c.txt",
      "a.txt",
      "B.txt",
      ".claude/other/x",
      ".claude/worktrees-old/x",
      ".github/x",
      "docs/handoffs-old/x",
      "docs/probes-old/x",
      "docs/reviews/other/x",
      "evals/runs-old/x",
      "node_modules-old/x",
    ])
      writeFile(root, rel, "kept");
    assert.deepEqual(visited(root, NONE, "hygiene"), [
      ".claude/other/x",
      ".claude/worktrees-old/x",
      ".github/x",
      "B.txt",
      "a.txt",
      "c.txt",
      "docs/handoffs-old/x",
      "docs/handoffs.md",
      "docs/probes-old/x",
      "docs/reviews/other/x",
      "evals/runs-old/x",
      "node_modules-old/x",
      "worktrees/x",
      "x/evals/runs/y",
    ]);
  });
});

test("a work folder's temp directory is skipped in both modes only when it matches the pattern", () => {
  withTree((root) => {
    writeFile(root, "docs/specs/2026-10/feature-x/temp/a", "skipped");
    writeFile(root, "2026-10/feature-x/temp/b", "skipped");
    writeFile(root, "docs/temp/c", "kept");
    writeFile(root, "docs/specs/2026-10/Feature_X/temp/d", "kept");
    writeFile(root, "docs/specs/2026-10/feature-x/temp2/e", "kept");
    // Near misses: no purpose folder, an uppercase purpose, a prefix before the date, a short or long month.
    writeFile(root, "2026-10/temp/f", "kept");
    writeFile(root, "docs/specs/2026-10/Feature-Y/temp/g", "kept");
    writeFile(root, "a2026-10/feature-x/temp/h", "kept");
    writeFile(root, "docs/specs/2026-1/feature-x/temp/i", "kept");
    writeFile(root, "docs/specs/2026-100/feature-x/temp/j", "kept");
    assert.deepEqual(visited(root, NONE, "hygiene"), [
      "2026-10/temp/f",
      "a2026-10/feature-x/temp/h",
      "docs/specs/2026-1/feature-x/temp/i",
      "docs/specs/2026-10/Feature-Y/temp/g",
      "docs/specs/2026-10/Feature_X/temp/d",
      "docs/specs/2026-10/feature-x/temp2/e",
      "docs/specs/2026-100/feature-x/temp/j",
      "docs/temp/c",
    ]);
    assert.deepEqual(visited(root, NONE, "area").sort(), [
      "2026-10/temp/f",
      "a2026-10/feature-x/temp/h",
      "docs/specs/2026-1/feature-x/temp/i",
      "docs/specs/2026-10/Feature-Y/temp/g",
      "docs/specs/2026-10/Feature_X/temp/d",
      "docs/specs/2026-10/feature-x/temp2/e",
      "docs/specs/2026-100/feature-x/temp/j",
      "docs/temp/c",
    ]);
  });
});

test("a skipped directory with a tracked file is entered as a partial directory", () => {
  withTree((root) => {
    writeFile(root, "node_modules/pkg/forced.txt", "x");
    writeFile(root, "node_modules/pkg/other.txt", "x");
    writeFile(root, "node_modules/loose/y.txt", "x");
    writeFile(root, "docs/specs/2026-10/feature-x/temp/forced.txt", "x");
    writeFile(root, "docs/specs/2026-10/feature-x/temp/scratch.txt", "x");
    writeFile(root, "plain.txt", "x");
    // A sibling whose name only begins with the partial directory's is not under it.
    writeFile(root, "node_modules_extra/y.txt", "x");
    initCheckout(root, [
      "node_modules/pkg/forced.txt",
      "docs/specs/2026-10/feature-x/temp/forced.txt",
    ]);
    const tracked = trackedPaths(root, process.env);
    assert.deepEqual(visited(root, tracked, "hygiene"), [
      "docs/specs/2026-10/feature-x/temp/forced.txt",
      "node_modules/pkg/forced.txt",
      "node_modules_extra/y.txt",
      "plain.txt",
    ]);
  });
});

test("area mode skips testdata even when it holds a tracked file, ignores the root's own name, and visits symlinks", (t) => {
  withTree((root) => {
    writeFile(root, "a/testdata/go.mod", "x");
    writeFile(root, "a/go.mod", "x");
    initCheckout(root, ["a/testdata/go.mod"]);
    const tracked = trackedPaths(root, process.env);
    assert.deepEqual(visited(root, tracked, "area"), ["a/go.mod"]);
    try {
      symlinkSync(path.join(root, "a", "go.mod"), path.join(root, "link"));
    } catch {
      t.diagnostic(
        "symlinks are not available on this host: the symlink half is skipped",
      );
      return;
    }
    assert.deepEqual(visited(root, tracked, "area"), ["a/go.mod", "link"]);
    // Only the area walk skips testdata; the hygiene walk reads a tracked file there.
    assert.deepEqual(visited(root, tracked, "hygiene"), [
      "a/go.mod",
      "a/testdata/go.mod",
    ]);
  });
});

test("a link to a directory is a leaf in both modes and is never entered", (t) => {
  withTree((root) => {
    writeFile(root, "checkout/a.txt", "x");
    writeFile(root, "outside/x.txt", "x");
    const checkout = path.join(root, "checkout");
    try {
      symlinkSync(
        path.join(root, "outside"),
        path.join(checkout, "linked"),
        "dir",
      );
    } catch {
      t.skip("symlinks are not available on this host");
      return;
    }
    assert.deepEqual(visited(checkout, NONE, "area"), ["a.txt", "linked"]);
    assert.deepEqual(visited(checkout, NONE, "hygiene"), ["a.txt"]);
  });
});

test("a root whose own name is on the skip list scans nothing in hygiene mode", () => {
  withTree((parent) => {
    const root = path.join(parent, "node_modules");
    writeFile(root, "a.txt", "x");
    assert.deepEqual(visited(root, NONE, "hygiene"), []);
    assert.deepEqual(visited(root, NONE, "area"), ["a.txt"]);
  });
});

test("entries are visited in the default sort order of their names, which compares UTF-16 code units", () => {
  withTree((root) => {
    const fullwidth = `${String.fromCodePoint(0xff21)}.txt`;
    const emoji = `${String.fromCodePoint(0x1f600)}.txt`;
    for (const name of [fullwidth, emoji, "a.txt", "B.txt"])
      writeFile(root, name, "x");
    // U+1F600 is the surrogate pair D83D DE00, which sorts before U+FF21.
    assert.deepEqual(visited(root, NONE, "hygiene"), [
      "B.txt",
      "a.txt",
      emoji,
      fullwidth,
    ]);
  });
});

test("a directory the walk cannot list is a file error naming it", (t) => {
  withTree((root) => {
    writeFile(root, "sub/a.txt", "x");
    const target = path.join(root, "sub");
    const readdir = fs.readdirSync;
    const listing = t.mock.method(
      fs,
      "readdirSync",
      (...args: Parameters<typeof fs.readdirSync>) => {
        if (args[0] === target)
          throw Object.assign(new Error(`EACCES: scandir ${target}`), {
            code: "EACCES",
          });
        return readdir(...args);
      },
    );
    syncBuiltinESMExports();
    try {
      assert.throws(
        () => visited(root, NONE, "hygiene"),
        (error) =>
          error instanceof RepoFileError &&
          error.kind === "unreadable" &&
          error.message === "sub: EACCES",
      );
    } finally {
      listing.mock.restore();
      syncBuiltinESMExports();
    }
  });
});

test("a root that does not exist is a missing file error", () => {
  withTree((parent) => {
    assert.throws(
      () => visited(path.join(parent, "gone"), NONE, "hygiene"),
      (error) => error instanceof RepoFileError && error.kind === "missing",
    );
  });
});

test("a root that is a file is visited as the root", () => {
  withTree((root) => {
    writeFile(root, "a.txt", "x");
    const seen: string[] = [];
    walkCheckout(path.join(root, "a.txt"), NONE, "hygiene", (file) =>
      seen.push(`${file.rel}|${file.name}`),
    );
    assert.deepEqual(seen, [".|a.txt"]);
  });
});

test("withoutGitLocation drops the git location variables by upper-cased name and keeps the rest", () => {
  assert.deepEqual(
    withoutGitLocation({
      GIT_DIR: "a",
      git_work_tree: "b",
      Git_Index_File: "c",
      GIT_COMMON_DIR: "d",
      GIT_AUTHOR_NAME: "kept",
      GIT_DIRECTORY: "kept too",
      PATH: "p",
      UNSET: undefined,
    }),
    { GIT_AUTHOR_NAME: "kept", GIT_DIRECTORY: "kept too", PATH: "p" },
  );
});

test("withoutGitLocation keeps a __proto__ key as an own property and leaves the prototype alone", () => {
  const env = JSON.parse('{"__proto__": "x", "PATH": "p"}') as Record<
    string,
    string
  >;
  const clean = withoutGitLocation(env);
  assert.equal(Object.getOwnPropertyDescriptor(clean, "__proto__")?.value, "x");
  assert.equal(clean.PATH, "p");
  assert.equal(Object.getPrototypeOf(clean), Object.prototype);
  assert.equal(Object.getPrototypeOf({}), Object.prototype);
  assert.equal(Object.hasOwn(Object.prototype, "PATH"), false);
});

test(
  "the temp directory probe ignores a caller's GIT_DIR",
  {
    skip: tempIsInsideRepository()
      ? "the temp directory is inside a git repository"
      : false,
  },
  () => {
    withTree((root) => {
      writeFile(root, "a.txt", "x");
      initCheckout(root, ["a.txt"]);
      // An unstripped GIT_DIR names a real repository, so git would answer for it whatever the directory it is run in.
      assert.equal(
        tempIsInsideRepository({
          ...process.env,
          GIT_DIR: path.join(root, ".git"),
        }),
        false,
      );
    });
  },
);
