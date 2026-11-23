import assert from "node:assert/strict";
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
import { test, type TestContext } from "node:test";
import { RepoFileError } from "../../repo-file-error.ts";
import { writeFile } from "../../testing/files.ts";
import {
  assetFiles,
  auditFileError,
  checkedRepoPath,
  globSkillLeaf,
  INDEPENDENCE_EXEMPT_FILE,
  isFileErrorOfKind,
  isValidGlobPattern,
  readRepoFile,
  seams,
  skillDirectories,
  SKIPPED_DIRS,
  walkDir,
  walkSkillText,
  walkTree,
  withSeams,
  type FileInfo,
  type TreeEntry,
} from "./files.ts";
import {
  canLinkDirectory,
  DIRECTORY_LINK_SKIP,
  linkDirectory,
  removeTree,
} from "../../testing/host.ts";

function withDir<T>(body: (root: string) => T): T {
  const root = mkdtempSync(path.join(tmpdir(), "agentics-skills-files-"));
  try {
    return body(root);
  } finally {
    removeTree(root);
  }
}

/** The same for a body that awaits: the directory goes away only after the body settles. */
async function withDirAsync<T>(body: (root: string) => Promise<T>): Promise<T> {
  const root = mkdtempSync(path.join(tmpdir(), "agentics-skills-files-"));
  try {
    return await body(root);
  } finally {
    removeTree(root);
  }
}

function linkFile(target: string, link: string): boolean {
  try {
    symlinkSync(target, link, "file");
    return true;
  } catch {
    return false;
  }
}

/** Skips the running test, with a logged reason, when the host cannot create a directory link. */
function canLinkDirectories(t: TestContext): boolean {
  if (canLinkDirectory()) return true;
  t.skip(DIRECTORY_LINK_SKIP);
  return false;
}

const at = (root: string, rel: string): string =>
  path.join(root, ...rel.split("/"));

/** The `code` property of a thrown value, if it has one. */
const codeOf = (error: unknown): unknown =>
  (error as { code?: unknown } | null)?.code;

/**
 * An `assert.throws` validator for a file error of `kind` at `shown` with no `code` of its own, whose cause carries
 * `code` when one is given and none otherwise.
 */
function fileErrorOf(
  kind: string,
  shown: string,
  code?: string,
): (error: unknown) => boolean {
  return (error) => {
    assert.ok(error instanceof RepoFileError, String(error));
    assert.equal(error.kind, kind);
    assert.equal(error.path, shown);
    assert.equal(Object.hasOwn(error, "code"), false);
    assert.equal(codeOf(error.cause), code);
    return true;
  };
}

const file = (): FileInfo => ({
  isSymbolicLink: () => false,
  isDirectory: () => false,
});
const directory = (): FileInfo => ({
  isSymbolicLink: () => false,
  isDirectory: () => true,
});

function walkedTree(root: string): string[] {
  const seen: string[] = [];
  walkTree(root, (rel) => {
    seen.push(rel);
  });
  return seen;
}

function walkedDir(root: string): string[] {
  const seen: string[] = [];
  walkDir(root, (rel) => {
    seen.push(rel);
  });
  return seen;
}

/** A `readDir` seam that fails for one directory and reads every other one for real; call it outside `withSeams`. */
function denyDirectory(denied: string): (dir: string) => TreeEntry[] {
  const real = seams.readDir;
  return (dir: string) => {
    if (dir === denied)
      throw Object.assign(new Error("denied"), { code: "EACCES", path: dir });
    return real(dir);
  };
}

test("the skipped directories and the exempt file", () => {
  assert.deepEqual([...SKIPPED_DIRS].sort(), [
    ".claude",
    ".git",
    "node_modules",
    "tests",
  ]);
  assert.equal(INDEPENDENCE_EXEMPT_FILE, "skills/README.md");
});

// --- seams -------------------------------------------------------------------

test("withSeams replaces the named seams for the body and restores them after it", () => {
  const originals = { ...seams };
  const lstat = (): FileInfo => file();
  const result = withSeams({ lstat }, () => {
    assert.equal(seams.lstat, lstat);
    assert.equal(seams.readFile, originals.readFile);
    assert.equal(seams.readSkillDirectory, originals.readSkillDirectory);
    assert.equal(seams.readDir, originals.readDir);
    return "result";
  });
  assert.equal(result, "result");
  assert.deepEqual({ ...seams }, originals);
});

test("withSeams restores the seams when the body throws", () => {
  const originals = { ...seams };
  assert.throws(
    () =>
      withSeams({ lstat: () => file(), readFile: () => "" }, () => {
        throw new Error("boom");
      }),
    { message: "boom" },
  );
  assert.deepEqual({ ...seams }, originals);
});

test("withSeams keeps the seams replaced across an await and restores them when the body resolves", async () => {
  const originals = { ...seams };
  const lstat = (): FileInfo => file();
  const result = await withSeams({ lstat }, async () => {
    assert.equal(seams.lstat, lstat);
    await Promise.resolve();
    await new Promise((resolve) => setImmediate(resolve));
    assert.equal(seams.lstat, lstat);
    return 42;
  });
  assert.equal(result, 42);
  assert.deepEqual({ ...seams }, originals);
});

test("withSeams restores the seams after an asynchronous body rejects, and passes the rejection on", async () => {
  const originals = { ...seams };
  await assert.rejects(
    withSeams({ lstat: () => file() }, async () => {
      await Promise.resolve();
      assert.notEqual(seams.lstat, originals.lstat);
      throw new Error("late boom");
    }),
    { message: "late boom" },
  );
  assert.deepEqual({ ...seams }, originals);
});

test("a helper that reads the directory after an await sees the replaced seam", async () => {
  await withDirAsync(async (root) => {
    writeFile(root, "skills/real/SKILL.md", "x");
    const names = await withSeams(
      {
        readSkillDirectory: () => ({ names: ["fake"] }),
        lstat: () => directory(),
      },
      async () => {
        await Promise.resolve();
        return skillDirectories(root);
      },
    );
    assert.deepEqual(names, ["skills/fake"]);
    assert.deepEqual(skillDirectories(root), ["skills/real"]);
  });
});

test("every helper reads its seam at call time", () => {
  withDir((root) => {
    const calls: string[] = [];
    withSeams(
      {
        lstat: (target) => {
          calls.push(`lstat ${target}`);
          return file();
        },
        readFile: (target) => `text of ${target}`,
      },
      () => {
        assert.equal(
          readRepoFile(root, "x/y"),
          `text of ${path.join(root, "x", "y")}`,
        );
      },
    );
    assert.deepEqual(calls, [
      `lstat ${path.join(root, "x")}`,
      `lstat ${path.join(root, "x", "y")}`,
    ]);
  });
});

// --- walkTree ----------------------------------------------------------------

test("walkTree visits depth first in byte order, with each directory right before its children", () => {
  withDir((root) => {
    for (const rel of ["a/b.md", "a-x/c.md", "a.md"]) writeFile(root, rel, "x");
    assert.deepEqual(walkedTree(root), [
      "a",
      "a/b.md",
      "a-x",
      "a-x/c.md",
      "a.md",
    ]);
  });
});

test("walkTree skips the top-level evals directory but not a deeper one, and delivers a file named evals", () => {
  withDir((root) => {
    writeFile(root, "evals/x.md", "x");
    writeFile(root, "deep/evals/x.md", "x");
    assert.deepEqual(walkedTree(root), [
      "deep",
      "deep/evals",
      "deep/evals/x.md",
    ]);
  });
  withDir((root) => {
    writeFile(root, "evals", "not a directory");
    assert.deepEqual(walkedTree(root), ["evals"]);
  });
});

test("walkTree skips .git, node_modules, .claude and tests at the top and at depth, and delivers files with those names", () => {
  withDir((root) => {
    for (const name of [".git", "node_modules", ".claude", "tests"]) {
      writeFile(root, `${name}/x.md`, "x");
      writeFile(root, `deep/${name}/x.md`, "x");
    }
    assert.deepEqual(walkedTree(root), ["deep"]);
  });
  withDir((root) => {
    writeFile(root, "tests", "a file");
    writeFile(root, "sub/tests", "a file");
    writeFile(root, "sub/.git", "gitdir: ../elsewhere");
    assert.deepEqual(walkedTree(root), [
      "sub",
      "sub/.git",
      "sub/tests",
      "tests",
    ]);
  });
});

test("walkTree delivers nothing for a symlink, enters none, and never delivers the root", (t) => {
  withDir((root) => {
    if (!canLinkDirectories(t)) return;
    mkdirSync(at(root, "real"));
    writeFile(root, "real/inside.md", "x");
    writeFile(root, "plain.md", "x");
    for (const name of ["link", ".git", "tests", "evals"])
      assert.ok(linkDirectory(at(root, "real"), at(root, name)) !== undefined);
    const fileLinked = linkFile(at(root, "plain.md"), at(root, "flink"));
    assert.deepEqual(walkedTree(root), ["plain.md", "real", "real/inside.md"]);
    if (!fileLinked)
      t.diagnostic("file symlink: skipped, this host cannot create one");
  });
});

test("walkTree skips a junction entirely on Windows and does not enter it", (t) => {
  if (process.platform !== "win32") {
    t.skip("junctions exist on Windows only");
    return;
  }
  withDir((root) => {
    mkdirSync(at(root, "real"));
    writeFile(root, "real/inside.md", "x");
    try {
      symlinkSync(at(root, "real"), at(root, "junction"), "junction");
    } catch {
      t.skip("this host cannot create a junction");
      return;
    }
    assert.deepEqual(walkedTree(root), ["real", "real/inside.md"]);
  });
});

test("walkTree delivers nothing and does not throw for a root that is a regular file or a symlink", (t) => {
  withDir((root) => {
    writeFile(root, "plain.md", "x");
    assert.deepEqual(walkedTree(at(root, "plain.md")), []);
    if (!canLinkDirectories(t)) return;
    mkdirSync(at(root, "real"));
    writeFile(root, "real/inside.md", "x");
    assert.ok(linkDirectory(at(root, "real"), at(root, "link")) !== undefined);
    assert.deepEqual(walkedTree(at(root, "link")), []);
  });
});

test("walkTree throws a missing file error naming the root as dot, with the not-exist code", () => {
  withDir((root) => {
    const missing = at(root, "gone");
    assert.throws(
      () => walkTree(missing, () => {}),
      fileErrorOf("missing", ".", "ENOENT"),
    );
    assert.throws(() => walkTree(missing, () => {}), {
      message: ".: does not exist",
    });
  });
});

test("walkTree throws an unreadable file error naming a directory that cannot be read, after delivering it", () => {
  withDir((root) => {
    writeFile(root, "a/x.md", "x");
    writeFile(root, "b/y.md", "x");
    const denied = at(root, "b");
    const seen: string[] = [];
    withSeams({ readDir: denyDirectory(denied) }, () => {
      assert.throws(
        () =>
          walkTree(root, (rel) => {
            seen.push(rel);
          }),
        (error: unknown) => {
          fileErrorOf("unreadable", "b", "EACCES")(error);
          assert.equal((error as Error).message, "b: EACCES");
          return true;
        },
      );
    });
    assert.deepEqual(seen, ["a", "a/x.md", "b"]);
  });
});

test("walkTree lets a visitor skip a directory and reads the directory entries only through the seam", () => {
  withDir((root) => {
    writeFile(root, "a/x.md", "x");
    writeFile(root, "b/y.md", "x");
    const seen: string[] = [];
    walkTree(root, (rel) => {
      seen.push(rel);
      return rel === "a" ? "skip" : undefined;
    });
    assert.deepEqual(seen, ["a", "b", "b/y.md"]);
    const fake: TreeEntry[] = [
      { name: "z", isDirectory: false, isSymlink: false },
      { name: "link", isDirectory: false, isSymlink: true },
    ];
    withSeams({ readDir: () => fake }, () => {
      assert.deepEqual(walkedTree(root), ["z"]);
    });
  });
});

test("walkTree orders names by the default sort, by UTF-16 code units", () => {
  withDir((root) => {
    // U+1F600 is a surrogate pair whose first unit, 0xD83D, sorts below the one unit of U+FF21, so the emoji comes
    // first, where an order by UTF-8 bytes would put the fullwidth letter first.
    const wide = `${String.fromCodePoint(0xff21)}.md`;
    const emoji = `${String.fromCodePoint(0x1f600)}.md`;
    writeFile(root, emoji, "x");
    writeFile(root, wide, "x");
    assert.deepEqual(walkedTree(root), [emoji, wide]);
  });
});

// --- walkDir -----------------------------------------------------------------

test("walkDir visits siblings in byte order with each subtree right after its directory", () => {
  withDir((root) => {
    for (const rel of ["a-x/c.md", "a.md", "a_b"]) writeFile(root, rel, "x");
    assert.deepEqual(walkedDir(root), ["a-x", "a-x/c.md", "a.md", "a_b"]);
  });
});

test("walkDir orders per directory, not by one global sort of the full paths", () => {
  withDir((root) => {
    writeFile(root, "a/z.md", "x");
    writeFile(root, "a-x", "x");
    const seen = walkedDir(root);
    assert.deepEqual(seen, ["a", "a/z.md", "a-x"]);
    // A global sort puts "a-x" before "a/z.md" because "-" (0x2d) sorts before "/" (0x2f).
    assert.notDeepEqual(seen, [...seen].sort());
  });
});

test("walkDir delivers files, directories and symlinks with their flags, and enters no symlink", (t) => {
  withDir((root) => {
    if (!canLinkDirectories(t)) return;
    writeFile(root, "real/inside.md", "x");
    writeFile(root, "plain.md", "x");
    assert.ok(linkDirectory(at(root, "real"), at(root, "dlink")) !== undefined);
    const fileLinked = linkFile(at(root, "plain.md"), at(root, "flink"));
    const seen = new Map<string, TreeEntry>();
    walkDir(root, (rel, entry) => {
      assert.ok(!seen.has(rel), `${rel} is delivered once`);
      seen.set(rel, entry);
    });
    assert.deepEqual(
      [...seen.keys()].filter((rel) => rel.startsWith("dlink")),
      ["dlink"],
    );
    assert.equal(seen.get("dlink")?.isSymlink, true);
    assert.equal(seen.get("dlink")?.isDirectory, false);
    assert.equal(seen.get("real")?.isDirectory, true);
    assert.equal(seen.get("real")?.isSymlink, false);
    assert.equal(seen.get("real/inside.md")?.isDirectory, false);
    assert.equal(seen.get("plain.md")?.isSymlink, false);
    if (fileLinked) {
      assert.equal(seen.get("flink")?.isSymlink, true);
      assert.equal(seen.get("flink")?.isDirectory, false);
    } else {
      t.diagnostic("file symlink: skipped, this host cannot create one");
    }
  });
});

test("walkDir lets a visitor skip a directory, and skipping anything else changes nothing", (t) => {
  withDir((root) => {
    writeFile(root, "a/x.md", "x");
    writeFile(root, "b.md", "x");
    writeFile(root, "c/y.md", "x");
    const seen: string[] = [];
    walkDir(root, (rel) => {
      seen.push(rel);
      return rel === "a" || rel === "b.md" ? "skip" : undefined;
    });
    assert.deepEqual(seen, ["a", "b.md", "c", "c/y.md"]);
    if (canLinkDirectories(t)) {
      assert.ok(linkDirectory(at(root, "c"), at(root, "link")) !== undefined);
      const again: string[] = [];
      walkDir(root, (rel) => {
        again.push(rel);
        return rel === "link" ? "skip" : undefined;
      });
      assert.deepEqual(again, ["a", "a/x.md", "b.md", "c", "c/y.md", "link"]);
    }
  });
});

test("walkDir throws for a directory that cannot be read, after delivering what came before", () => {
  withDir((root) => {
    writeFile(root, "a/x.md", "x");
    writeFile(root, "b/y.md", "x");
    const denied = at(root, "b");
    const seen: string[] = [];
    withSeams({ readDir: denyDirectory(denied) }, () => {
      assert.throws(
        () =>
          walkDir(
            root,
            (rel) => {
              seen.push(rel);
            },
            "base",
          ),
        fileErrorOf("unreadable", "base/b", "EACCES"),
      );
      assert.throws(() => walkDir(root, () => {}, "."), {
        message: "b: EACCES",
      });
      assert.throws(() => walkDir(root, () => {}), {
        message: `${denied.replaceAll("\\", "/")}: EACCES`,
      });
    });
    assert.deepEqual(seen, ["a", "a/x.md", "b"]);
  });
});

test("walkDir throws a missing file error naming the root by its base, or as given without one", () => {
  withDir((root) => {
    const missing = at(root, "gone");
    assert.throws(
      () => walkDir(missing, () => {}, "skills/gone"),
      fileErrorOf("missing", "skills/gone", "ENOENT"),
    );
    assert.throws(() => walkDir(missing, () => {}), {
      message: `${missing.replaceAll("\\", "/")}: does not exist`,
    });
  });
});

test("walkDir never delivers a directory root and delivers a regular-file root once as rel dot", () => {
  withDir((root) => {
    assert.deepEqual(walkedDir(root), []);
    writeFile(root, "plain.md", "x");
    const delivered: Array<[string, TreeEntry]> = [];
    walkDir(at(root, "plain.md"), (rel, entry) => {
      delivered.push([rel, entry]);
    });
    assert.deepEqual(delivered, [
      [".", { name: "plain.md", isDirectory: false, isSymlink: false }],
    ]);
  });
});

test("walkDir delivers a symlink root once as rel dot, flagged, and does not enter it", (t) => {
  withDir((root) => {
    if (!canLinkDirectories(t)) return;
    writeFile(root, "real/inside.md", "x");
    assert.ok(linkDirectory(at(root, "real"), at(root, "link")) !== undefined);
    const delivered: Array<[string, TreeEntry]> = [];
    walkDir(at(root, "link"), (rel, entry) => {
      delivered.push([rel, entry]);
    });
    assert.deepEqual(delivered, [
      [".", { name: "link", isDirectory: false, isSymlink: true }],
    ]);
  });
});

test("walkDir paths use slashes on every platform", () => {
  withDir((root) => {
    writeFile(root, "a/b/c.md", "x");
    assert.deepEqual(walkedDir(root), ["a", "a/b", "a/b/c.md"]);
  });
});

// --- checkedRepoPath ---------------------------------------------------------

test("checkedRepoPath rejects a non-local path as outside the checkout and never touches the file system", () => {
  withDir((root) => {
    let calls = 0;
    const lstat = (): FileInfo => {
      calls += 1;
      return file();
    };
    const absolute = process.platform === "win32" ? "/abs" : "/abs/x";
    for (const rel of ["../outside", absolute, "", "a/../../b"])
      assert.throws(
        () => checkedRepoPath(root, rel, { lstat }),
        (error: unknown) => {
          fileErrorOf("outside-repo", rel)(error);
          assert.equal(
            (error as Error).message,
            `${rel}: is outside the checkout`,
          );
          return true;
        },
        rel,
      );
    assert.equal(calls, 0);
  });
});

test("checkedRepoPath names the path as given and applies the rules of the platform it is given", () => {
  withDir((root) => {
    let calls = 0;
    const lstat = (): FileInfo => {
      calls += 1;
      return file();
    };
    const backslash = String.fromCharCode(92);
    const escaping = `..${backslash}outside`;
    assert.throws(
      () => checkedRepoPath(root, escaping, { lstat, platform: "win32" }),
      fileErrorOf("outside-repo", "../outside"),
    );
    assert.equal(calls, 0);
    // On Linux a backslash is an ordinary character, so the same text is a local file name. The components are joined
    // with the host's path module, so only the absence of the non-local error is asserted here.
    assert.doesNotThrow(() =>
      checkedRepoPath(root, escaping, { lstat, platform: "linux" }),
    );
    assert.equal(calls, 1);
    for (const rel of ["nul", "a/com1"]) {
      assert.throws(
        () => checkedRepoPath(root, rel, { lstat, platform: "win32" }),
        fileErrorOf("outside-repo", rel),
        rel,
      );
      assert.doesNotThrow(
        () => checkedRepoPath(root, rel, { lstat, platform: "linux" }),
        rel,
      );
    }
  });
});

test("checkedRepoPath returns the joined path with dot segments resolved", () => {
  withDir((root) => {
    writeFile(root, "a/b", "x");
    const expected = path.join(root, "a", "b");
    for (const rel of ["a/b", "a/./b", "a//b", "a/c/../b", "./a/b"])
      assert.equal(checkedRepoPath(root, rel), expected, rel);
  });
});

test("checkedRepoPath inspects every component from the first below the root to the leaf, once, in order", () => {
  withDir((root) => {
    writeFile(root, "a/b/c", "x");
    const seen: string[] = [];
    const lstat = (target: string): FileInfo => {
      seen.push(target);
      return lstatSync(target);
    };
    checkedRepoPath(root, "a/b/c", { lstat });
    assert.deepEqual(seen, [
      path.join(root, "a"),
      path.join(root, "a", "b"),
      path.join(root, "a", "b", "c"),
    ]);
  });
});

test("checkedRepoPath for dot inspects the root itself, and a linked root then fails", (t) => {
  withDir((base) => {
    const seen: string[] = [];
    const lstat = (target: string): FileInfo => {
      seen.push(target);
      return lstatSync(target);
    };
    assert.equal(checkedRepoPath(base, ".", { lstat }), path.join(base));
    assert.deepEqual(seen, [path.join(base)]);
    if (!canLinkDirectories(t)) return;
    mkdirSync(at(base, "real"));
    assert.ok(linkDirectory(at(base, "real"), at(base, "link")) !== undefined);
    assert.throws(
      () => checkedRepoPath(at(base, "link"), "."),
      (error: unknown) => {
        fileErrorOf("symlink", ".")(error);
        assert.equal((error as Error).message, ".: passes through a link");
        return true;
      },
    );
  });
});

test("checkedRepoPath throws a symlink file error naming the path up to a link at the leaf, the parent or the grandparent", (t) => {
  withDir((root) => {
    if (!canLinkDirectories(t)) return;
    const outside = path.join(root, "outside");
    mkdirSync(outside);
    writeFile(outside, "deep/leaf.md", "x");
    const checkout = at(root, "checkout");
    mkdirSync(at(checkout, "a/b"), { recursive: true });
    assert.ok(
      linkDirectory(outside, at(checkout, "grand")) !== undefined,
      "grandparent link",
    );
    assert.ok(
      linkDirectory(outside, at(checkout, "a/parent")) !== undefined,
      "parent link",
    );
    assert.ok(
      linkDirectory(outside, at(checkout, "a/b/leaf")) !== undefined,
      "leaf link",
    );
    const cases: Array<[string, string]> = [
      ["grand/deep/leaf.md", "grand"],
      ["a/parent/deep", "a/parent"],
      ["a/b/leaf", "a/b/leaf"],
    ];
    for (const [rel, linked] of cases)
      assert.throws(
        () => checkedRepoPath(checkout, rel),
        (error: unknown) => {
          fileErrorOf("symlink", linked)(error);
          assert.equal(
            (error as Error).message,
            `${linked}: passes through a link`,
          );
          return true;
        },
        rel,
      );
  });
});

test("checkedRepoPath throws a missing file error naming the first missing component, with the lstat error as its cause", () => {
  withDir((root) => {
    writeFile(root, "a/x", "x");
    let thrown: unknown;
    const lstat = (target: string): FileInfo => {
      try {
        return lstatSync(target);
      } catch (error) {
        thrown = error;
        throw error;
      }
    };
    const cases: Array<[string, string]> = [
      ["a/missing", "a/missing"],
      ["missing/deeper", "missing"],
    ];
    for (const [rel, shown] of cases) {
      thrown = undefined;
      assert.throws(
        () => checkedRepoPath(root, rel, { lstat }),
        (error: unknown) => {
          fileErrorOf("missing", shown, "ENOENT")(error);
          assert.equal(
            (error as Error).cause,
            thrown,
            "the original error object",
          );
          assert.equal((error as Error).message, `${shown}: does not exist`);
          return true;
        },
        rel,
      );
    }
  });
});

test("checkedRepoPath reports a path below a regular file as the host does", (t) => {
  withDir((root) => {
    writeFile(root, "plain.md", "x");
    let probed: unknown;
    try {
      lstatSync(at(root, "plain.md/x"));
    } catch (error) {
      probed = codeOf(error);
    }
    const expected = process.platform === "win32" ? "ENOENT" : "ENOTDIR";
    assert.equal(probed, expected);
    t.diagnostic(
      `a path below a regular file is ${expected} on ${process.platform} (ENOTDIR on POSIX, ENOENT on Windows)`,
    );
    assert.throws(
      () => checkedRepoPath(root, "plain.md/x"),
      fileErrorOf(
        expected === "ENOENT" ? "missing" : "not-a-directory",
        "plain.md/x",
        expected,
      ),
    );
  });
});

// --- readRepoFile ------------------------------------------------------------

test("readRepoFile returns the text with the byte order mark and CR kept", () => {
  withDir((root) => {
    const bom = String.fromCharCode(0xfeff);
    writeFileSync(at(root, "a.md"), Buffer.from(`${bom}one\r\ntwo\rthree\n`));
    assert.equal(readRepoFile(root, "a.md"), `${bom}one\r\ntwo\rthree\n`);
  });
});

test("readRepoFile decodes an invalid UTF-8 byte as U+FFFD", () => {
  withDir((root) => {
    writeFileSync(at(root, "a.md"), Buffer.from([0x61, 0xff, 0x62]));
    assert.equal(
      readRepoFile(root, "a.md"),
      `a${String.fromCharCode(0xfffd)}b`,
    );
  });
});

test("readRepoFile throws a not-a-file error for a directory target", () => {
  withDir((root) => {
    mkdirSync(at(root, "dir"));
    assert.throws(
      () => readRepoFile(root, "./dir"),
      fileErrorOf("not-a-file", "dir", "EISDIR"),
    );
    assert.throws(() => readRepoFile(root, "dir"), {
      message: "dir: is not a regular file",
    });
  });
});

test("readRepoFile throws the file error of each kind: missing, not a file, through a link, outside the checkout", (t) => {
  withDir((base) => {
    const root = at(base, "checkout");
    writeFile(root, "plain.md", "x");
    mkdirSync(at(root, "dir"));
    writeFile(base, "outside/target.md", "x");
    assert.throws(
      () => readRepoFile(root, "gone.md"),
      fileErrorOf("missing", "gone.md", "ENOENT"),
    );
    assert.throws(
      () => readRepoFile(root, "dir"),
      fileErrorOf("not-a-file", "dir", "EISDIR"),
    );
    assert.throws(
      () => readRepoFile(root, "../outside/target.md"),
      fileErrorOf("outside-repo", "../outside/target.md"),
    );
    if (!canLinkDirectories(t)) return;
    assert.ok(
      linkDirectory(at(base, "outside"), at(root, "linked")) !== undefined,
    );
    assert.throws(
      () => readRepoFile(root, "linked/target.md"),
      fileErrorOf("symlink", "linked"),
    );
  });
});

test("readRepoFile names a read failure by the cleaned path and keeps it as the cause", () => {
  withDir((root) => {
    writeFile(root, "a/b.md", "x");
    const failure = Object.assign(new Error("denied"), { code: "EACCES" });
    const read = (thrown: unknown) => () =>
      readRepoFile(root, "a/./b.md", {
        readFile: () => {
          throw thrown;
        },
      });
    assert.throws(read(failure), (error: unknown) => {
      fileErrorOf("unreadable", "a/b.md", "EACCES")(error);
      assert.equal((error as Error).cause, failure);
      assert.equal((error as Error).message, "a/b.md: EACCES");
      return true;
    });
    assert.throws(read(null), (error: unknown) => {
      fileErrorOf("unreadable", "a/b.md")(error);
      assert.equal(Object.hasOwn(error as object, "code"), false);
      return true;
    });
    const own = new RepoFileError("symlink", "elsewhere");
    assert.throws(read(own), (error: unknown) => error === own);
  });
});

test("readRepoFile stops on an inspection error without reading, and throws it as the cause of a file error", () => {
  withDir((root) => {
    writeFile(root, "sentinel", "readable content");
    const inspection = new Error("inspection failed");
    const inspected: string[] = [];
    let reads = 0;
    assert.throws(
      () =>
        readRepoFile(root, "sentinel", {
          lstat: (target) => {
            inspected.push(target);
            throw inspection;
          },
          readFile: () => {
            reads += 1;
            return "content";
          },
        }),
      (error: unknown) => {
        fileErrorOf("unreadable", "sentinel")(error);
        assert.equal((error as Error).cause, inspection);
        return true;
      },
    );
    assert.deepEqual(inspected, [path.join(root, "sentinel")]);
    assert.equal(reads, 0);
  });
});

test("readRepoFile rejects a non-local path before any read and refuses a link at the leaf", (t) => {
  withDir((root) => {
    assert.throws(() => readRepoFile(root, "../x"), {
      message: "../x: is outside the checkout",
    });
    writeFile(root, "target.md", "x");
    if (!linkFile(at(root, "target.md"), at(root, "linked.md"))) {
      t.skip("this host cannot create a file symlink");
      return;
    }
    assert.throws(
      () => readRepoFile(root, "linked.md"),
      fileErrorOf("symlink", "linked.md"),
    );
  });
});

// --- glob syntax -------------------------------------------------------------

test("isValidGlobPattern rejects the malformed classes on both platforms and accepts the well-formed ones", () => {
  for (const platform of ["linux", "win32"]) {
    for (const bad of ["[a", "[]a]", "[a-]", "[^]", "x[", "[", "[a-"])
      assert.equal(
        isValidGlobPattern(`/r/${bad}`, platform),
        false,
        `${bad} on ${platform}`,
      );
    for (const good of ["[x]", "[a-c]", "[^a]", "[*]", "x?y", "plain"])
      assert.equal(
        isValidGlobPattern(`/r/${good}`, platform),
        true,
        `${good} on ${platform}`,
      );
  }
});

test("isValidGlobPattern treats a backslash as an escape on linux and as a plain character on win32", () => {
  const trailing = `/r/x${String.fromCharCode(92)}`;
  assert.equal(isValidGlobPattern(trailing, "linux"), false);
  assert.equal(isValidGlobPattern(trailing, "win32"), true);
  const escapedBracket = `/r/[${String.fromCharCode(92)}]`;
  assert.equal(isValidGlobPattern(escapedBracket, "linux"), false);
  assert.equal(isValidGlobPattern(escapedBracket, "win32"), true);
  // An escaped closing bracket inside a class is valid on linux, and a class holding a backslash plus a literal
  // bracket on win32.
  const classWithEscape = `/r/[${String.fromCharCode(92)}]]`;
  assert.equal(isValidGlobPattern(classWithEscape, "linux"), true);
  assert.equal(isValidGlobPattern(classWithEscape, "win32"), true);
});

test("isValidGlobPattern looks only at the text before the first star outside a class, as Match against an empty name does", () => {
  assert.equal(isValidGlobPattern("/r/a*[b", "linux"), true);
  assert.equal(isValidGlobPattern("/r/[a*", "linux"), false);
  assert.equal(isValidGlobPattern("/r/[*]*[", "linux"), true);
  assert.equal(isValidGlobPattern("", "linux"), true);
  assert.equal(isValidGlobPattern("***", "linux"), true);
});

test("isValidGlobPattern rejects a class that holds a malformed character and accepts a multibyte one", () => {
  const eAcute = String.fromCharCode(0xe9);
  assert.equal(isValidGlobPattern(`/r/[${eAcute}]`, "linux"), true);
  assert.equal(isValidGlobPattern(`/r/[${eAcute}-z]`, "linux"), true);
  assert.equal(isValidGlobPattern(`/r/[${eAcute}-]`, "linux"), false);
});

// --- skillDirectories, globSkillLeaf, assetFiles ------------------------------

test("assetFiles selects skills/*/SKILL.md in sort order, not a lower-case skill.md, and a directory named SKILL.md", () => {
  withDir((root) => {
    writeFile(root, "skills/z/SKILL.md", "z");
    writeFile(root, "skills/a/SKILL.md", "a");
    writeFile(root, "skills/case/skill.md", "wrong case");
    mkdirSync(at(root, "skills/dir/SKILL.md"), { recursive: true });
    assert.deepEqual(skillDirectories(root), [
      "skills/a",
      "skills/case",
      "skills/dir",
      "skills/z",
    ]);
    assert.deepEqual(assetFiles(root), [
      "skills/a/SKILL.md",
      "skills/dir/SKILL.md",
      "skills/z/SKILL.md",
    ]);
  });
});

test("globSkillLeaf returns the joined path of a selected leaf and nothing for a missing name or parent", () => {
  withDir((root) => {
    writeFile(root, "skills/a/SKILL.md", "a");
    assert.deepEqual(globSkillLeaf(root, "skills/a", "SKILL.md"), {
      path: at(root, "skills/a/SKILL.md"),
      selected: true,
    });
    assert.deepEqual(globSkillLeaf(root, "skills/a", "OTHER.md"), {
      path: "",
      selected: false,
    });
    assert.deepEqual(globSkillLeaf(root, "skills/none", "SKILL.md"), {
      path: "",
      selected: false,
    });
    assert.deepEqual(globSkillLeaf(root, "../outside", "SKILL.md"), {
      path: "",
      selected: false,
    });
  });
});

test("assetFiles skips a skill whose directory cannot be enumerated, quietly", () => {
  withDir((root) => {
    writeFile(root, "skills/blocked/SKILL.md", "blocked");
    writeFile(root, "skills/z/SKILL.md", "z");
    writeFile(root, "skills/a/SKILL.md", "a");
    const real = seams.readSkillDirectory;
    const blocked = at(root, "skills/blocked");
    const readSkillDirectory = (dir: string) =>
      dir === blocked
        ? { names: [], error: new Error("permission denied") }
        : real(dir);
    assert.deepEqual(assetFiles(root, { readSkillDirectory }), [
      "skills/a/SKILL.md",
      "skills/z/SKILL.md",
    ]);
    withSeams({ readSkillDirectory }, () => {
      assert.deepEqual(assetFiles(root), [
        "skills/a/SKILL.md",
        "skills/z/SKILL.md",
      ]);
    });
  });
});

test("assetFiles keeps the names an enumeration returned together with an error", () => {
  withDir((root) => {
    writeFile(root, "skills/a/SKILL.md", "ok");
    writeFile(root, "skills/z/SKILL.md", "ok");
    const real = seams.readSkillDirectory;
    const failing = new Error("permission denied");
    const skills = at(root, "skills");
    const readSkillDirectory = (dir: string) => ({
      names: real(dir).names,
      error:
        dir === skills || dir === at(root, "skills/a") ? failing : undefined,
    });
    assert.deepEqual(assetFiles(root, { readSkillDirectory }), [
      "skills/a/SKILL.md",
      "skills/z/SKILL.md",
    ]);
  });
});

test("assetFiles throws an audit error with a missing cause when an enumerated leaf is missing from the file system", () => {
  withDir((root) => {
    writeFile(root, "skills/a/SKILL.md", "ok");
    mkdirSync(at(root, "skills/missing"));
    const real = seams.readSkillDirectory;
    const missing = at(root, "skills/missing");
    const readSkillDirectory = (dir: string) =>
      dir === missing ? real(at(root, "skills/a")) : real(dir);
    assert.throws(
      () => assetFiles(root, { readSkillDirectory }),
      (error: Error) => {
        assert.equal(
          error.message,
          "skills: checking skills/missing/SKILL.md: skills/missing/SKILL.md: does not exist",
        );
        fileErrorOf(
          "missing",
          "skills/missing/SKILL.md",
          "ENOENT",
        )(error.cause);
        return true;
      },
    );
  });
});

test("a linked skills root gives no directories and no assets", (t) => {
  withDir((base) => {
    if (!canLinkDirectories(t)) return;
    const root = at(base, "checkout");
    const outside = at(base, "outside");
    mkdirSync(root);
    writeFile(outside, "evil/SKILL.md", "SENTINEL");
    assert.ok(linkDirectory(outside, at(root, "skills")) !== undefined);
    assert.deepEqual(skillDirectories(root), []);
    assert.deepEqual(assetFiles(root), []);
  });
});

test("a linked child directory and a linked SKILL.md leaf are silently not selected", (t) => {
  withDir((base) => {
    if (!canLinkDirectories(t)) return;
    const root = at(base, "checkout");
    const outside = at(base, "outside");
    writeFile(outside, "SKILL.md", "SENTINEL");
    writeFile(root, "skills/good/SKILL.md", "ok");
    assert.ok(
      linkDirectory(outside, at(root, "skills/parent")) !== undefined,
      "linked child",
    );
    mkdirSync(at(root, "skills/leaf"));
    const leafLinked = linkFile(
      at(outside, "SKILL.md"),
      at(root, "skills/leaf/SKILL.md"),
    );
    // skills/leaf is a real directory whether or not its SKILL.md could be linked.
    assert.deepEqual(skillDirectories(root), ["skills/good", "skills/leaf"]);
    assert.deepEqual(assetFiles(root), ["skills/good/SKILL.md"]);
    if (!leafLinked)
      t.diagnostic(
        "linked leaf: skipped, this host cannot create a file symlink",
      );
  });
});

test("globSkillLeaf neither lists nor selects from a linked parent", (t) => {
  withDir((base) => {
    if (!canLinkDirectories(t)) return;
    const root = at(base, "checkout");
    const outside = at(base, "outside");
    writeFile(outside, "SKILL.md", "SENTINEL");
    mkdirSync(at(root, "skills"), { recursive: true });
    assert.ok(linkDirectory(outside, at(root, "skills/linked")) !== undefined);
    // The leaf check would also refuse the link, so only the listing shows that the parent check stopped first.
    const listed: string[] = [];
    const readSkillDirectory = (dir: string) => {
      listed.push(dir);
      return seams.readSkillDirectory(dir);
    };
    assert.deepEqual(
      globSkillLeaf(root, "skills/linked", "SKILL.md", { readSkillDirectory }),
      { path: "", selected: false },
    );
    assert.deepEqual(listed, []);
  });
});

test("a skills entry that is a regular file or is missing gives no directories", () => {
  withDir((root) => {
    assert.deepEqual(skillDirectories(root), []);
    assert.deepEqual(assetFiles(root), []);
    writeFile(root, "skills", "not a directory");
    assert.deepEqual(skillDirectories(root), []);
    assert.deepEqual(assetFiles(root), []);
  });
});

test("a repository root ending in a bracket throws the pattern error from skillDirectories and assetFiles", () => {
  withDir((base) => {
    const root = path.join(base, "[");
    assert.throws(() => skillDirectories(root), {
      message: "syntax error in pattern",
    });
    assert.throws(() => assetFiles(root), {
      message: "syntax error in pattern",
    });
  });
});

test("a root named with a valid class is a literal root and a sibling directory is ignored", () => {
  withDir((base) => {
    const root = path.join(base, "[x]");
    writeFile(root, "skills/a/SKILL.md", "literal");
    writeFile(path.join(base, "x"), "skills/other/SKILL.md", "other");
    assert.deepEqual(assetFiles(root), ["skills/a/SKILL.md"]);
    assert.deepEqual(skillDirectories(root), ["skills/a"]);
  });
});

test("a root path with a NUL byte gives an empty list, not an error", () => {
  withDir((base) => {
    const root = path.join(base, "a\0b");
    assert.deepEqual(skillDirectories(root), []);
    assert.deepEqual(assetFiles(root), []);
  });
});

// --- walkSkillText -----------------------------------------------------------

function scannedText(
  root: string,
  audit = "an audit",
): Array<[string, string]> {
  const seen: Array<[string, string]> = [];
  walkSkillText(root, audit, (rel, text) => {
    seen.push([rel, text]);
  });
  return seen;
}

const noFiles = (audit: string): { message: string } => ({
  message: `skills: no files under skills/ to audit for ${audit}`,
});

test("walkSkillText says there is nothing to audit when skills is a link, empty, or holds only binary files", (t) => {
  withDir((base) => {
    const root = at(base, "checkout");
    mkdirSync(at(root, "skills"), { recursive: true });
    assert.throws(
      () => scannedText(root, "repository independence"),
      noFiles("repository independence"),
    );
    writeFile(root, "skills/binary", `\0text`);
    assert.throws(
      () => scannedText(root, "legacy names"),
      noFiles("legacy names"),
    );
    rmSync(at(root, "skills"), { recursive: true });
    if (!canLinkDirectories(t)) return;
    writeFile(at(base, "outside"), "bad.md", "x");
    assert.ok(
      linkDirectory(at(base, "outside"), at(root, "skills")) !== undefined,
    );
    assert.throws(
      () => scannedText(root, "repository independence"),
      noFiles("repository independence"),
    );
  });
});

test("walkSkillText throws the missing file error for a missing skills directory", () => {
  withDir((root) => {
    assert.throws(
      () => scannedText(root),
      (error: unknown) => {
        fileErrorOf("missing", "skills", "ENOENT")(error);
        assert.ok(
          !(error as Error).message.includes("no files under skills/"),
          (error as Error).message,
        );
        return true;
      },
    );
  });
});

test("walkSkillText visits eligible files in sort order with slash paths, and skips the ineligible ones", (t) => {
  withDir((root) => {
    writeFile(root, "skills/z.md", "z");
    writeFile(root, "skills/a.md", "a");
    writeFile(root, "skills/README.md", "exempt");
    writeFile(root, "skills/sub/README.md", "nested readme");
    writeFile(root, "skills/binary", "\0hidden");
    writeFile(root, "skills/empty", "");
    writeFile(root, "outside.md", "x");
    if (canLinkDirectories(t)) {
      assert.ok(
        linkDirectory(at(root, "skills/sub"), at(root, "skills/dlink")) !==
          undefined,
      );
      linkFile(at(root, "outside.md"), at(root, "skills/flink.md"));
    }
    assert.deepEqual(scannedText(root), [
      ["skills/a.md", "a"],
      ["skills/empty", ""],
      ["skills/sub/README.md", "nested readme"],
      ["skills/z.md", "z"],
    ]);
  });
});

test("walkSkillText passes the text with the byte order mark and CR kept", () => {
  withDir((root) => {
    const bom = String.fromCharCode(0xfeff);
    mkdirSync(at(root, "skills"));
    writeFileSync(at(root, "skills/a.md"), Buffer.from(`${bom}one\r\ntwo\r`));
    assert.deepEqual(scannedText(root), [
      ["skills/a.md", `${bom}one\r\ntwo\r`],
    ]);
  });
});

test("walkSkillText visits a skills entry that is a regular file once, as skills", () => {
  withDir((root) => {
    writeFile(root, "skills", "plain file");
    assert.deepEqual(scannedText(root), [["skills", "plain file"]]);
  });
});

test("walkSkillText throws for a file or a directory that cannot be read", () => {
  withDir((root) => {
    writeFile(root, "skills/a.md", "a");
    writeFile(root, "skills/b/c.md", "c");
    const failing = at(root, "skills/a.md");
    const readError = Object.assign(new Error("denied"), { code: "EACCES" });
    assert.throws(
      () =>
        walkSkillText(root, "x", () => {}, {
          readFile: (target) => {
            if (target === failing) throw readError;
            return "text";
          },
        }),
      (error: unknown) => {
        fileErrorOf("unreadable", "skills/a.md", "EACCES")(error);
        assert.equal((error as Error).cause, readError);
        return true;
      },
    );
    const denied = at(root, "skills/b");
    withSeams({ readDir: denyDirectory(denied) }, () => {
      assert.throws(
        () => walkSkillText(root, "x", () => {}),
        (error: unknown) => {
          fileErrorOf("unreadable", "skills/b", "EACCES")(error);
          assert.equal((error as Error).message, "skills/b: EACCES");
          return true;
        },
      );
    });
  });
});

test("walkSkillText reads through the seams and counts an injected empty file as scanned", () => {
  withDir((root) => {
    mkdirSync(at(root, "skills"));
    const text = withSeams(
      {
        readDir: () => [{ name: "only", isDirectory: false, isSymlink: false }],
        readFile: () => "",
      },
      () => scannedText(root),
    );
    assert.deepEqual(text, [["skills/only", ""]]);
  });
});

// --- glob syntax and leaf selection, rows added by the mutation pass --------------------------------------------------

// Each row holds a class edge case: a dash or closing bracket in the wrong place, or a multibyte class character.
test("isValidGlobPattern rejects malformed class edges and accepts classes of one to four byte characters", () => {
  const bs = String.fromCharCode(92);
  const char = (point: number): string => String.fromCodePoint(point);
  const invalid = [
    "[-a]",
    "[^-a]",
    "[^]a]",
    "[a-]]",
    `[${char(0x4e2d)}-]`,
    `[${char(0x1f600)}-]`,
  ];
  const valid = [
    "[[]",
    "[[]x",
    "abc",
    "*abc",
    "**abc",
    `${bs}a`,
    `a${bs}a`,
    `[${bs}]]`,
    `[a${bs}]]`,
    `[${char(0xe9)}]`,
    `[${char(0x80)}]`,
    `[${char(0xbf)}]`,
    `[${char(0x7ff)}]`,
    `[${char(0x800)}]`,
    `[${char(0x4e2d)}]`,
    `[${char(0xf000)}]`,
    `[${char(0xffff)}]`,
    `[${char(0x10000)}]`,
    `[${char(0x1f600)}]`,
    `[${char(0x10ffff)}]`,
    `[a-${char(0x4e2d)}]`,
    `[${char(0x80)}-${char(0x7ff)}]`,
    `[${char(0x4e2d)}-${char(0x4e2e)}]`,
    `[${char(0x1f600)}-${char(0x1f64f)}]`,
  ];
  for (const platform of ["linux", "win32"]) {
    for (const pattern of invalid)
      assert.equal(
        isValidGlobPattern(`/r/${pattern}`, platform),
        false,
        `${JSON.stringify(pattern)} on ${platform}`,
      );
    for (const pattern of valid)
      assert.equal(
        isValidGlobPattern(`/r/${pattern}`, platform),
        true,
        `${JSON.stringify(pattern)} on ${platform}`,
      );
  }
  // A backslash before a star escapes it on linux, so the class after it is still in the first chunk; on win32 the
  // star ends the chunk first.
  const escapedStar = `/r/a${bs}*[`;
  assert.equal(isValidGlobPattern(escapedStar, "linux"), false);
  assert.equal(isValidGlobPattern(escapedStar, "win32"), true);
  // An escaped star is a literal one on linux, and a leading star is skipped before the first chunk is read.
  assert.equal(isValidGlobPattern(`/r/a${bs}*b`, "linux"), true);
  assert.equal(isValidGlobPattern(`/r/a${bs}*b`, "win32"), true);
  assert.equal(isValidGlobPattern("*[a", "linux"), false);
  assert.equal(isValidGlobPattern("*[a", "win32"), false);
  // A class cut off by a backslash is bad on both.
  assert.equal(isValidGlobPattern(`/r/[${bs}`, "linux"), false);
  assert.equal(isValidGlobPattern(`/r/[a${bs}`, "win32"), false);
});

test("globSkillLeaf throws an audit error naming the leaf, with the file error of the failed inspection as its cause", () => {
  withDir((root) => {
    writeFile(root, "skills/a/SKILL.md", "a");
    const leaf = at(root, "skills/a/SKILL.md");
    const rows: [string, unknown, string, string | undefined][] = [
      [
        "a coded failure",
        Object.assign(new Error("denied"), { code: "EACCES", path: "/else" }),
        "EACCES",
        "EACCES",
      ],
      ["a failure with no code", new Error("denied"), "unreadable", undefined],
      ["no error object", null, "unreadable", undefined],
    ];
    for (const [label, failure, reason, code] of rows) {
      assert.throws(
        () =>
          globSkillLeaf(root, "skills/a", "SKILL.md", {
            lstat: (candidate) => {
              if (candidate === leaf) throw failure;
              return lstatSync(candidate);
            },
          }),
        (error: Error) => {
          assert.equal(
            error.message,
            `skills: checking skills/a/SKILL.md: skills/a/SKILL.md: ${reason}`,
            label,
          );
          assert.equal(Object.hasOwn(error, "code"), false, label);
          fileErrorOf("unreadable", "skills/a/SKILL.md", code)(error.cause);
          assert.equal((error.cause as Error).cause, failure, label);
          return true;
        },
        label,
      );
    }
  });
});

test("auditFileError names the action and the path, then the message of the failure, and keeps the failure as its cause", () => {
  const cause = new RepoFileError("missing", "skills/a/SKILL.md");
  const same = auditFileError("reading", "skills/a/SKILL.md", cause);
  assert.equal(
    same.message,
    "skills: reading skills/a/SKILL.md: skills/a/SKILL.md: does not exist",
  );
  assert.equal(same.cause, cause);
  assert.equal(
    auditFileError(
      "reading",
      "skills/a/SKILL.md",
      new RepoFileError("symlink", "skills/a"),
    ).message,
    "skills: reading skills/a/SKILL.md: skills/a: passes through a link",
  );
  assert.equal(
    auditFileError("checking", "x", new Error("plain")).message,
    "skills: checking x: plain",
  );
  assert.equal(
    auditFileError("checking", "x", null).message,
    "skills: checking x: null",
  );
});

test("isFileErrorOfKind tells a file error by its kind and nothing else", () => {
  assert.equal(
    isFileErrorOfKind(new RepoFileError("missing", "a"), "missing"),
    true,
  );
  assert.equal(
    isFileErrorOfKind(new RepoFileError("missing", "a"), "symlink"),
    false,
  );
  assert.equal(
    isFileErrorOfKind(new RepoFileError("symlink", "a"), "symlink"),
    true,
  );
  assert.equal(
    isFileErrorOfKind(
      Object.assign(new Error("x"), { kind: "missing" }),
      "missing",
    ),
    false,
  );
  assert.equal(isFileErrorOfKind(null, "missing"), false);
});

test("the default directory enumeration returns no names and the error for a directory that cannot be read", () => {
  withDir((root) => {
    const result = seams.readSkillDirectory(at(root, "missing"));
    assert.deepEqual(result.names, []);
    assert.equal(codeOf(result.error), "ENOENT");
  });
});

// --- cases that pin behaviour a surviving mutant changed ---------------------------------------------------------------

test("walkTree enters a work folder's temp directory in full without a tracked lister", () => {
  withDir((root) => {
    writeFile(root, "w/2026-11/feature-x/temp/scratch.md", "x");
    assert.deepEqual(walkedTree(root), [
      "w",
      "w/2026-11",
      "w/2026-11/feature-x",
      "w/2026-11/feature-x/temp",
      "w/2026-11/feature-x/temp/scratch.md",
    ]);
  });
});

test("walkTree with a tracked lister delivers only the tracked part of a work folder's temp directory", () => {
  withDir((root) => {
    writeFile(root, "w/2026-11/feature-x/temp/kept.md", "x");
    writeFile(root, "w/2026-11/feature-x/temp/scratch.md", "x");
    writeFile(root, "w/2026-11/feature-y/temp/scratch.md", "x");
    // A regular file named temp is not a scratch directory, so it is delivered whether or not git tracks it.
    writeFile(root, "w/2026-11/feature-z/temp", "x");
    let calls = 0;
    const seen: string[] = [];
    walkTree(
      root,
      (rel) => {
        seen.push(rel);
      },
      () => {
        calls += 1;
        return {
          files: new Set(["w/2026-11/feature-x/temp/kept.md"]),
          dirs: new Set([
            "w",
            "w/2026-11",
            "w/2026-11/feature-x",
            "w/2026-11/feature-x/temp",
          ]),
        };
      },
    );
    assert.deepEqual(seen, [
      "w",
      "w/2026-11",
      "w/2026-11/feature-x",
      "w/2026-11/feature-x/temp",
      "w/2026-11/feature-x/temp/kept.md",
      "w/2026-11/feature-y",
      "w/2026-11/feature-z",
      "w/2026-11/feature-z/temp",
    ]);
    assert.equal(calls, 2);
  });
});

test("checkedRepoPath does not inspect a dot or empty segment as a component", () => {
  withDir((root) => {
    writeFile(root, "a/b", "x");
    const seen: string[] = [];
    const lstat = (target: string): FileInfo => {
      seen.push(target);
      return lstatSync(target);
    };
    checkedRepoPath(root, "./a//b", { lstat });
    assert.deepEqual(seen, [path.join(root, "a"), path.join(root, "a", "b")]);
  });
});

test("checkedRepoPath splits on a backslash only on win32", () => {
  const lstat = (): FileInfo => {
    throw Object.assign(new Error("gone"), { code: "ENOENT" });
  };
  const rel = `xy${String.fromCharCode(92)}z`;
  // On Linux the backslash is part of one name, so the first inspection is of the whole name.
  assert.throws(
    () => checkedRepoPath("root", rel, { lstat, platform: "linux" }),
    fileErrorOf("missing", "xy/z", "ENOENT"),
  );
  assert.throws(
    () => checkedRepoPath("root", rel, { lstat, platform: "win32" }),
    fileErrorOf("missing", "xy", "ENOENT"),
  );
});

test("skillDirectories checks the glob pattern under the platform it is given", () => {
  withDir((base) => {
    // On win32 the bracket opens a class that never closes; on Linux the backslash before it escapes it.
    const root = `${path.join(base, "x")}${String.fromCharCode(92)}[`;
    assert.deepEqual(skillDirectories(root, { platform: "linux" }), []);
    assert.throws(() => skillDirectories(root, { platform: "win32" }), {
      message: "syntax error in pattern",
    });
  });
});

test("skillDirectories lists only directories, not a file directly under skills", () => {
  withDir((root) => {
    writeFile(root, "skills/a/SKILL.md", "a");
    writeFile(root, "skills/README.md", "readme");
    assert.deepEqual(skillDirectories(root), ["skills/a"]);
  });
});

test("withSeams returns a null or undefined body result as it is", () => {
  assert.equal(
    withSeams({}, () => null),
    null,
  );
  assert.equal(
    withSeams({}, () => undefined),
    undefined,
  );
});

test("assetFiles orders by the whole path, so skills/a-x comes before skills/a", () => {
  withDir((root) => {
    writeFile(root, "skills/a/SKILL.md", "a");
    writeFile(root, "skills/a-x/SKILL.md", "a-x");
    assert.deepEqual(assetFiles(root), [
      "skills/a-x/SKILL.md",
      "skills/a/SKILL.md",
    ]);
  });
});
