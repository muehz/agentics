import assert from "node:assert/strict";
import fs, { mkdtempSync, type Dirent } from "node:fs";
import { syncBuiltinESMExports } from "node:module";
import { tmpdir } from "node:os";
import path from "node:path";
import { test, type TestContext } from "node:test";
import * as manifests from "./domains/manifests/files.ts";
import * as repo from "./domains/repo/walk.ts";
import * as skills from "./domains/skills/files.ts";
import { run as runSkills } from "./domains/skills/index.ts";
import { FAMILY as stateFamily } from "./domains/skills/state-tsv.ts";
import { FAMILY as workFolderFamily } from "./domains/skills/work-folder.ts";
import { RepoFileError } from "./repo-file-error.ts";
import { writeFile } from "./testing/files.ts";
import { initCheckout } from "./testing/git-checkout.ts";
import {
  canLinkDirectory,
  DIRECTORY_LINK_SKIP,
  linkDirectory,
  removeTree,
} from "./testing/host.ts";

const modes = [
  "repo-hygiene",
  "repo-area",
  "manifests",
  "skills-tree",
  "skills-dir",
] as const;
type Mode = (typeof modes)[number];
const none: repo.Tracked = { files: new Set(), dirs: new Set() };

function fixture(t: TestContext, files: string[] = []): string {
  const root = mkdtempSync(path.join(tmpdir(), "agentics-walk-"));
  t.after(() => removeTree(root));
  for (const rel of files) writeFile(root, rel, "contents");
  return root;
}

type Options = {
  tracked?: () => repo.Tracked;
  base?: string;
  onVisit?: (rel: string) => "skip" | void;
};
type Event = {
  rel: string;
  name: string;
  absolute?: string;
  kind?: string;
  parentPath?: string;
};

function observe(mode: Mode, root: string, options: Options = {}) {
  const events: Event[] = [];
  let lookups = 0;
  const list = options.tracked;
  const tracked =
    list === undefined
      ? undefined
      : () => {
          lookups++;
          return list();
        };
  const repoVisit = (file: repo.WalkedFile) => {
    events.push(file);
    return options.onVisit?.(file.rel);
  };
  const manifestVisit = (rel: string, entry: Dirent) => {
    events.push({
      rel,
      name: entry.name,
      kind: entry.isDirectory()
        ? "dir"
        : entry.isSymbolicLink()
          ? "link"
          : entry.isFile()
            ? "file"
            : "other",
      parentPath: entry.parentPath,
    });
    return options.onVisit?.(rel);
  };
  const skillVisit = (rel: string, entry: skills.TreeEntry) => {
    events.push({
      rel,
      name: entry.name,
      kind: entry.isSymlink ? "link" : entry.isDirectory ? "dir" : "leaf",
    });
    return options.onVisit?.(rel);
  };
  let fault: unknown;
  try {
    switch (mode) {
      case "repo-hygiene":
      case "repo-area":
        repo.walkCheckout(
          root,
          tracked?.() ?? none,
          mode === "repo-area" ? "area" : "hygiene",
          repoVisit,
        );
        break;
      case "manifests":
        manifests.walkTree(root, manifestVisit, tracked);
        break;
      case "skills-tree":
        skills.walkTree(root, skillVisit, tracked);
        break;
      case "skills-dir":
        skills.walkDir(root, skillVisit, options.base);
    }
  } catch (error) {
    fault = error;
  }
  return { events, fault, lookups };
}

const folder = "docs/specs/2026-11/feature-walk";
const scratch = `${folder}/temp`;
const emptyScratch = "2026-11/fix-empty/temp";
const orderedNames = ["B.txt", "a.txt", "😀.txt", "Ａ.txt"];
const orderedNamePaths = orderedNames.map((name) => `names/${name}`);

function expectedPaths(mode: Mode): string[] {
  if (mode === "repo-area" || mode === "repo-hygiene")
    return [
      `${scratch}/keep/yes`,
      "evals/top",
      ...orderedNamePaths,
      "nested/evals/local",
      ...(mode === "repo-hygiene" ? ["nested/testdata/yes"] : []),
      "node_modules/pkg/yes",
      "ordinary/node_modules",
      "tests/fixture",
    ];

  const unfiltered = mode === "skills-dir";
  return [
    ...(unfiltered
      ? [".claude", ".claude/worktrees", ".claude/worktrees/hidden"]
      : []),
    "2026-11",
    "2026-11/fix-empty",
    ...(unfiltered ? [emptyScratch, `${emptyScratch}/no`] : []),
    "docs",
    "docs/specs",
    "docs/specs/2026-11",
    folder,
    scratch,
    `${scratch}/keep`,
    ...(unfiltered ? [`${scratch}/keep/no`] : []),
    `${scratch}/keep/yes`,
    ...(unfiltered ? [`${scratch}/other`, `${scratch}/other/no`] : []),
    ...(mode === "skills-tree" ? [] : ["evals", "evals/top"]),
    "names",
    ...orderedNamePaths,
    "nested",
    "nested/evals",
    "nested/evals/local",
    "nested/testdata",
    "nested/testdata/yes",
    ...(unfiltered
      ? [
          "node_modules",
          "node_modules/pkg",
          "node_modules/pkg/no",
          "node_modules/pkg/yes",
        ]
      : []),
    "ordinary",
    "ordinary/node_modules",
    ...(mode === "skills-tree" ? [] : ["tests", "tests/fixture"]),
  ];
}

for (const mode of modes) {
  test(`${mode} preserves depth-first order, exclusions and tracked scratch contents`, (t) => {
    const root = fixture(t, [
      ...orderedNames.toReversed().map((name) => `names/${name}`),
      `${scratch}/keep/yes`,
      `${scratch}/keep/no`,
      `${scratch}/other/no`,
      `${emptyScratch}/no`,
      "node_modules/pkg/yes",
      "node_modules/pkg/no",
      "nested/testdata/yes",
      ".claude/worktrees/hidden",
      "tests/fixture",
      "evals/top",
      "nested/evals/local",
      "ordinary/node_modules",
    ]);
    initCheckout(root, [
      `${scratch}/keep/yes`,
      "node_modules/pkg/yes",
      "nested/testdata/yes",
    ]);
    const inventory = repo.trackedPaths(root, process.env);
    assert.deepEqual([...inventory.files].sort(), [
      `${scratch}/keep/yes`,
      "nested/testdata/yes",
      "node_modules/pkg/yes",
    ]);
    const result = observe(mode, root, { tracked: () => inventory });
    assert.equal(result.fault, undefined);
    const paths = result.events.map((e) => e.rel);
    assert.equal(paths.includes(".git"), mode === "skills-dir");
    assert.deepEqual(
      paths.filter((rel) => rel !== ".git" && !rel.startsWith(".git/")),
      expectedPaths(mode),
    );
    const sample = result.events.find((e) => e.rel === "names/B.txt");
    const leaf = { rel: "names/B.txt", name: "B.txt" };
    if (mode.startsWith("repo-")) {
      assert.deepEqual(sample, {
        ...leaf,
        absolute: path.join(root, "names", "B.txt"),
      });
    } else if (mode === "manifests") {
      assert.deepEqual(sample, {
        ...leaf,
        kind: "file",
        parentPath: path.join(root, "names"),
      });
    } else {
      assert.deepEqual(sample, { ...leaf, kind: "leaf" });
    }
    assert.equal(
      result.lookups,
      mode === "skills-dir" ? 0 : mode.startsWith("repo-") ? 1 : 2,
    );
    if (mode === "manifests" || mode === "skills-tree") {
      const unfiltered = observe(mode, root);
      assert.ok(unfiltered.events.some((e) => e.rel === `${scratch}/keep/no`));
    }
  });

  test(`${mode} preserves file roots, absent roots and root-name exclusions`, (t) => {
    const parent = fixture(t, ["node_modules/child"]);
    const file = observe(mode, path.join(parent, "node_modules", "child"));
    const missing = observe(mode, path.join(parent, "missing"), {
      base: "skills/sub",
    });
    assert.ok(missing.fault instanceof RepoFileError);
    assert.equal(missing.fault.kind, "missing");
    assert.equal(
      missing.fault.path,
      mode === "skills-dir" ? "skills/sub" : ".",
    );
    assert.equal(
      missing.fault.message,
      `${missing.fault.path}: does not exist`,
    );
    if (mode === "manifests") {
      assert.ok(file.fault instanceof RepoFileError);
      assert.equal(file.fault.kind, "not-a-directory");
    } else {
      assert.equal(file.fault, undefined);
      assert.deepEqual(
        file.events.map((e) => e.rel),
        mode === "skills-tree" ? [] : ["."],
      );
    }
    const excluded = observe(mode, path.join(parent, "node_modules"));
    assert.equal(excluded.fault, undefined);
    assert.deepEqual(
      excluded.events.map((e) => e.rel),
      mode === "repo-hygiene" ? [] : ["child"],
    );
  });

  test(
    `${mode} preserves link roots and linked directory entries`,
    { skip: canLinkDirectory() ? false : DIRECTORY_LINK_SKIP },
    (t) => {
      const target = fixture(t, ["off-tree"]);
      const root = fixture(t, ["plain"]);
      const link = path.join(root, "linked");
      assert.ok(linkDirectory(target, link));
      const children = observe(mode, root);
      assert.equal(children.fault, undefined);
      const reportsLink = mode !== "repo-hygiene" && mode !== "skills-tree";
      assert.deepEqual(
        children.events.map((e) => e.rel),
        reportsLink ? ["linked", "plain"] : ["plain"],
      );
      if (mode === "manifests" || mode === "skills-dir")
        assert.equal(children.events[0]?.kind, "link");
      const linkedRoot = observe(mode, link);
      if (mode === "manifests") {
        assert.ok(linkedRoot.fault instanceof RepoFileError);
        assert.equal(linkedRoot.fault.kind, "symlink");
      } else {
        assert.equal(linkedRoot.fault, undefined);
        assert.deepEqual(
          linkedRoot.events.map((e) => e.rel),
          reportsLink ? ["."] : [],
        );
      }
    },
  );

  test(`${mode} preserves visitor pruning and propagates visitor errors`, (t) => {
    const root = fixture(t, ["a/pruned/child", "b/end"]);
    const pruned = observe(mode, root, {
      onVisit: (rel) => (rel === "a" ? "skip" : undefined),
    });
    assert.equal(pruned.fault, undefined);
    assert.equal(
      pruned.events.some((e) => e.rel === "a/pruned/child"),
      mode !== "skills-dir" && mode !== "skills-tree",
    );
    const fault = new Error("visitor failed to read the file");
    const stopped = observe(mode, root, {
      onVisit: (rel) => {
        if (rel === "b/end") throw fault;
      },
    });
    assert.equal(stopped.fault, fault);
    assert.equal(stopped.events.at(-1)?.rel, "b/end");
  });

  test(`${mode} retains entries delivered before a directory read fails`, (t) => {
    const root = fixture(t, ["a-first", "bad/unread", "z-last"]);
    const read = fs.readdirSync;
    const fault = Object.assign(new Error("directory read denied"), {
      code: "EACCES",
    });
    const listing = t.mock.method(
      fs,
      "readdirSync",
      (...args: Parameters<typeof fs.readdirSync>) => {
        if (args[0] === path.join(root, "bad")) throw fault;
        return read(...args);
      },
    );
    syncBuiltinESMExports();
    try {
      const result = observe(mode, root, { base: "skills/one" });
      assert.ok(result.fault instanceof RepoFileError);
      assert.equal(result.fault.kind, "unreadable");
      assert.equal(
        result.fault.path,
        mode === "skills-dir" ? "skills/one/bad" : "bad",
      );
      assert.equal(result.fault.message, `${result.fault.path}: EACCES`);
      assert.equal(result.fault.cause, fault);
      assert.deepEqual(
        result.events.map((e) => e.rel),
        mode.startsWith("repo-") ? ["a-first"] : ["a-first", "bad"],
      );
    } finally {
      listing.mock.restore();
      syncBuiltinESMExports();
    }
  });
}

for (const mode of ["manifests", "skills-tree"] as const) {
  test(`${mode} requests tracked paths only on reaching scratch and preserves lookup failures`, (t) => {
    const root = fixture(t, ["ordinary/read"]);
    const fault = new Error("tracked lookup failed");
    const options = {
      tracked: () => {
        throw fault;
      },
    };
    assert.equal(observe(mode, root, options).lookups, 0);
    writeFile(root, `${scratch}/file`, "data");
    const result = observe(mode, root, options);
    assert.equal(result.fault, fault);
    assert.equal(result.lookups, 1);
    assert.ok(!result.events.some((e) => e.rel === scratch));
  });
}

for (const mode of ["skills-tree", "skills-dir"] as const) {
  test(`${mode} uses the supplied filesystem's entry order`, (t) => {
    const root = fixture(t, ["a", "z"]);
    const read = skills.seams.readDir;
    skills.withSeams({ readDir: (dir) => read(dir).reverse() }, () => {
      const result = observe(mode, root);
      assert.equal(result.fault, undefined);
      assert.deepEqual(
        result.events.map((e) => e.rel),
        ["z", "a"],
      );
    });
  });
}

const runOptions = {
  short: false,
  fast: false,
  diagnostics: () => {},
  env: process.env,
};

test("the skills domain exempts tracked scratch state and reports the malformed sibling", async (t) => {
  const root = fixture(t);
  const inside = `${scratch}/state.tsv`;
  const outside = `${folder}/state.tsv`;
  const header =
    "ts\tsession\tphase\tevent\ttarget\tstatus\twhy\tevidence\tnext";
  writeFile(root, inside, `# flow-state scratch\n${header}\n`);
  writeFile(root, outside, `# flow-state outside\n${header}\n`);
  initCheckout(root, [inside, outside]);
  assert.ok(repo.trackedPaths(root, process.env).files.has(inside));

  assert.deepEqual(
    await runSkills(root, runOptions, { families: [stateFamily] }),
    {
      failures: [
        {
          rule: "state-tsv-shape",
          path: outside,
          message:
            'line 1 is "# flow-state outside", want the header row or the version line "# flow-state 1"',
        },
      ],
    },
  );
});

test(
  "the skills domain reports the full audit diagnostic for a required path through a link",
  { skip: canLinkDirectory() ? false : DIRECTORY_LINK_SKIP },
  async (t) => {
    const root = fixture(t, ["skills/README.md"]);
    const target = fixture(t, ["references/chain-artifacts.md"]);
    assert.ok(linkDirectory(target, path.join(root, "skills", "decompose")));

    assert.deepEqual(
      await runSkills(root, runOptions, { families: [workFolderFamily] }),
      {
        failures: [],
        error:
          "skills: reading skills/decompose/references/chain-artifacts.md: skills/decompose: passes through a link",
      },
    );
  },
);
