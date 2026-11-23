import assert from "node:assert/strict";
import { mkdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { test } from "node:test";
import { git } from "./github.ts";
import {
  formatVersion,
  MANIFESTS,
  setVersion,
  verifyReleaseCommit,
} from "./version.ts";
import { commitFiles, initRepo, scratchDir, sh } from "./testing/support.ts";

const PATHS = MANIFESTS.map((manifest) => manifest.path);
const RELEASE_SUBJECT = "chore(release): v1.3.0 [skip ci]";

function put(root: string, path: string, contents: string): void {
  const full = join(root, path);
  mkdirSync(dirname(full), { recursive: true });
  writeFileSync(full, contents);
}

function writeManifestsForSet(root: string, version: string): void {
  const plugin = `{\n    "name": "agentics",\n    "version": "${version}"\n}\n`;
  const marketplace = `{\n    "metadata": {\n        "version": "0.0.0"\n    },\n    "plugins": [\n        {\n            "name": "agentics",\n            "version": "${version}"\n        }\n    ]\n}\n`;
  for (const path of PATHS) {
    put(root, path, path.endsWith("marketplace.json") ? marketplace : plugin);
  }
}

// A repository whose one commit carries the five manifests at 1.2.3 and a
// notes.txt outside them, and that commit's SHA.
function releaseRepo(): { root: string; parent: string } {
  const root = scratchDir("agentics-verify-");
  initRepo(root);
  writeManifestsForSet(root, "1.2.3");
  const parent = commitFiles(root, { "notes.txt": "notes\n" }, "base");
  return { root, parent };
}

// Commits the whole working tree as the release commit.
function commitAll(root: string): string {
  sh(root, "git", "add", "-A");
  return commitIndex(root);
}

// Commits the index as it stands. A case that edits an entry's mode in the
// index commits through this, because staging again would restore the mode the
// file system reports.
function commitIndex(root: string): string {
  sh(root, "git", "commit", "--quiet", "--allow-empty", "-m", RELEASE_SUBJECT);
  return sh(root, "git", "rev-parse", "HEAD").trim();
}

// A commit on parent whose tree is the working tree's plus entry at the root.
// The tree is built with mktree because a checkout cannot hold every name a
// tree can, a path made only of spaces among them on Windows.
async function commitWithRootEntry(
  root: string,
  parent: string,
  entry: string,
): Promise<string> {
  const commit = commitAll(root);
  const listing = (await git(root, ["ls-tree", commit])).trim();
  const tree = (
    await git(root, ["mktree"], { input: `${listing}\n${entry}\n` })
  ).trim();
  return (
    await git(root, ["commit-tree", tree, "-p", parent, "-m", RELEASE_SUBJECT])
  ).trim();
}

function replaceInFile(
  root: string,
  path: string,
  old: string,
  replacement: string,
): void {
  const text = readFileSync(join(root, path), "utf8");
  assert.ok(text.includes(old), `${path} does not contain ${old}`);
  writeFileSync(join(root, path), text.replace(old, replacement));
}

test("verifyReleaseCommit accepts a commit that changes only the version fields", async () => {
  const { root, parent } = releaseRepo();
  setVersion(root, "1.3.0");
  const commit = commitAll(root);
  assert.equal(
    formatVersion(await verifyReleaseCommit(root, parent, commit)),
    "1.3.0",
  );
});

// The mode rule covers the parent side too, so a release commit that repairs an
// executable manifest still changes more than the version.
test("verifyReleaseCommit rejects a commit that repairs a manifest mode", async () => {
  const { root } = releaseRepo();
  sh(root, "git", "update-index", "--chmod=+x", ".github/plugin/plugin.json");
  const parent = commitIndex(root);
  setVersion(root, "1.3.0");
  sh(root, "git", "add", "-A");
  sh(root, "git", "update-index", "--chmod=-x", ".github/plugin/plugin.json");
  const commit = commitIndex(root);
  await assert.rejects(verifyReleaseCommit(root, parent, commit), {
    message:
      'release commit changes ".github/plugin/plugin.json" from mode 100755 to 100644, want 100644 on both sides',
  });
});

type Case = {
  name: string;
  build: (root: string, parent: string) => string | Promise<string>;
  want: string;
};

// Every case starts from a tree already at 1.3.0 and breaks one rule, so
// removing that rule's check lets the case through.
const REJECTED: Case[] = [
  {
    name: "an added path",
    build: (root) => {
      put(root, "README.md", "x\n");
      return commitAll(root);
    },
    want: 'release commit adds "README.md"',
  },
  {
    name: "a blob named only by a space",
    build: async (root, parent) => {
      const blob = (
        await git(root, ["hash-object", "-w", "--stdin"], { input: "x\n" })
      ).trim();
      return commitWithRootEntry(root, parent, `100644 blob ${blob}\t `);
    },
    want: 'release commit adds " "',
  },
  {
    name: "a gitlink named only by a space",
    build: (root, parent) =>
      commitWithRootEntry(root, parent, `160000 commit ${parent}\t `),
    want: 'release commit has a gitlink at " "',
  },
  {
    name: "a manifest made executable",
    build: (root) => {
      sh(root, "git", "add", "-A");
      sh(
        root,
        "git",
        "update-index",
        "--chmod=+x",
        ".claude-plugin/plugin.json",
      );
      return commitIndex(root);
    },
    want: 'release commit changes ".claude-plugin/plugin.json" from mode 100644 to 100755, want 100644 on both sides',
  },
  {
    // The symlink blob holds the manifest text, so the version reads correctly
    // and only the entry mode is wrong.
    name: "a manifest turned into a symlink",
    build: (root) => {
      sh(root, "git", "add", "-A");
      const blob = sh(
        root,
        "git",
        "hash-object",
        "-w",
        ".claude-plugin/plugin.json",
      ).trim();
      sh(
        root,
        "git",
        "update-index",
        "--cacheinfo",
        `120000,${blob},.claude-plugin/plugin.json`,
      );
      return commitIndex(root);
    },
    want: 'release commit has a symlink at ".claude-plugin/plugin.json"',
  },
  {
    // A deleted manifest has no version to read, so the version read refuses it
    // before the entry check runs.
    name: "a deleted manifest",
    build: (root) => {
      rmSync(join(root, ".codex-plugin/plugin.json"));
      return commitAll(root);
    },
    want: "release commit: .codex-plugin/plugin.json: git show: git exited with code 128",
  },
  {
    name: "a deleted path",
    build: (root) => {
      rmSync(join(root, "notes.txt"));
      return commitAll(root);
    },
    want: 'release commit deletes "notes.txt"',
  },
  {
    name: "a modified path",
    build: (root) => {
      put(root, "notes.txt", "changed\n");
      return commitAll(root);
    },
    want: 'release commit changes "notes.txt", which is not one of the five manifests',
  },
  {
    name: "a non-version change",
    build: (root) => {
      replaceInFile(
        root,
        ".claude-plugin/plugin.json",
        '"name": "agentics"',
        '"name": "agentics-renamed"',
      );
      return commitAll(root);
    },
    want: ".claude-plugin/plugin.json changes more than the version field",
  },
  {
    name: "a non-version change to the antigravity manifest",
    build: (root) => {
      replaceInFile(
        root,
        ".antigravity-plugin/plugin.json",
        '"name": "agentics"',
        '"name": "agentics-renamed"',
      );
      return commitAll(root);
    },
    want: ".antigravity-plugin/plugin.json changes more than the version field",
  },
  {
    // The marketplace metadata.version is a version field too, just not the
    // Agentics one, so a rewrite that also moves it is a second change.
    name: "the marketplace metadata version",
    build: (root) => {
      replaceInFile(
        root,
        ".claude-plugin/marketplace.json",
        '"version": "0.0.0"',
        '"version": "1.3.0"',
      );
      return commitAll(root);
    },
    want: ".claude-plugin/marketplace.json changes more than the version field",
  },
  {
    name: "a lower version",
    build: (root) => {
      writeManifestsForSet(root, "1.2.2");
      return commitAll(root);
    },
    want: "release commit version 1.2.2 is not greater than parent version 1.2.3",
  },
  {
    name: "the same version",
    build: (root) => {
      writeManifestsForSet(root, "1.2.3");
      return commitAll(root);
    },
    want: "release commit version 1.2.3 is not greater than parent version 1.2.3",
  },
];

test("verifyReleaseCommit rejects a commit that breaks one rule", async (t) => {
  for (const { name, build, want } of REJECTED) {
    await t.test(name, async () => {
      const { root, parent } = releaseRepo();
      setVersion(root, "1.3.0");
      const commit = await build(root, parent);
      await assert.rejects(verifyReleaseCommit(root, parent, commit), {
        message: want,
      });
    });
  }
});

test("verifyReleaseCommit names the side whose version cannot be read", async () => {
  const { root, parent } = releaseRepo();
  setVersion(root, "1.3.0");
  const commit = commitAll(root);
  const bad = "0".repeat(40);
  await assert.rejects(verifyReleaseCommit(root, bad, commit), {
    message: `parent: ${PATHS[0]}: git show: git exited with code 128`,
  });
  await assert.rejects(verifyReleaseCommit(root, parent, bad), {
    message: `release commit: ${PATHS[0]}: git show: git exited with code 128`,
  });
  await assert.rejects(verifyReleaseCommit(root, "HEAD", commit), {
    message: 'parent: invalid commit SHA: "HEAD"',
  });
});

// A parent entry that is a gitlink or a symlink is refused by name even when the release commit makes it a regular
// file, rather than as a mode change.
test("verifyReleaseCommit names a gitlink or a symlink on the parent side", async (t) => {
  for (const [kind, entry] of [
    [
      "gitlink",
      (root: string, parent: string) => `160000 commit ${parent}\tlib`,
    ],
    [
      "symlink",
      async (root: string) => {
        const target = (
          await git(root, ["hash-object", "-w", "--stdin"], {
            input: "notes.txt",
          })
        ).trim();
        return `120000 blob ${target}\tlib`;
      },
    ],
  ] as const) {
    await t.test(kind, async () => {
      const { root, parent } = releaseRepo();
      const before = await commitWithRootEntry(
        root,
        parent,
        await entry(root, parent),
      );
      setVersion(root, "1.3.0");
      const blob = (
        await git(root, ["hash-object", "-w", "--stdin"], { input: "x\n" })
      ).trim();
      const commit = await commitWithRootEntry(
        root,
        before,
        `100644 blob ${blob}\tlib`,
      );
      await assert.rejects(verifyReleaseCommit(root, before, commit), {
        message: `release commit has a ${kind} at "lib"`,
      });
    });
  }
});
