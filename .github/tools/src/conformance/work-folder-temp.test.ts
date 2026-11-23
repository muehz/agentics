import assert from "node:assert/strict";
import { mkdirSync, mkdtempSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { test } from "node:test";
import { trackedPaths, type Tracked } from "./domains/repo/walk.ts";
import { writeFile } from "./testing/files.ts";
import {
  initCheckout,
  tempIsInsideRepository,
} from "./testing/git-checkout.ts";
import {
  inWorkFolderTemp,
  isWorkFolderTempDir,
  lazyTrackedLookup,
} from "./work-folder-temp.ts";
import { removeTree } from "./testing/host.ts";

function withTree(body: (root: string) => void): void {
  const root = mkdtempSync(path.join(tmpdir(), "agentics-workfoldertemp-"));
  try {
    body(root);
  } finally {
    removeTree(root);
  }
}

// Each directory with whether it is a work folder's temp directory; the near misses mirror the walk's hygiene table.
const DIRS: ReadonlyArray<[string, boolean]> = [
  ["docs/specs/2026-10/feature-x/temp", true],
  ["2026-10/feature-x/temp", true],
  ["docs/temp", false],
  ["docs/specs/2026-10/Feature_X/temp", false],
  ["docs/specs/2026-10/feature-x/temp2", false],
  ["2026-10/temp", false],
  ["docs/specs/2026-10/Feature-Y/temp", false],
  ["a2026-10/feature-x/temp", false],
  ["docs/specs/2026-1/feature-x/temp", false],
  ["docs/specs/2026-100/feature-x/temp", false],
];

test("the directory form matches only a work folder's temp directory", () => {
  for (const [dir, want] of DIRS) {
    assert.equal(isWorkFolderTempDir(dir), want, dir);
    assert.equal(isWorkFolderTempDir(`${dir}/x`), false, `${dir}/x`);
  }
});

test("the file form matches only a path below a work folder's temp directory", () => {
  for (const [dir, want] of DIRS) {
    assert.equal(inWorkFolderTemp(`${dir}/x`), want, `${dir}/x`);
    assert.equal(inWorkFolderTemp(`${dir}/sub/x`), want, `${dir}/sub/x`);
    assert.equal(inWorkFolderTemp(dir), false, dir);
  }
});

function counting(root: string): { list: () => Tracked; calls: () => number } {
  let calls = 0;
  return {
    list: () => {
      calls++;
      return trackedPaths(root, process.env);
    },
    calls: () => calls,
  };
}

test("the lookup over a tree without .git is empty", () => {
  withTree((root) => {
    writeFile(root, "a.txt", "x");
    const lookup = lazyTrackedLookup(() => trackedPaths(root, process.env));
    assert.deepEqual(lookup(), { files: new Set(), dirs: new Set() });
  });
});

test("the lookup lists once, on first use, and returns the same result after", () => {
  withTree((root) => {
    writeFile(root, "a.txt", "x");
    initCheckout(root, ["a.txt"]);
    const lister = counting(root);
    const lookup = lazyTrackedLookup(lister.list);
    assert.equal(lister.calls(), 0);
    const first = lookup();
    assert.deepEqual([...first.files], ["a.txt"]);
    assert.equal(lookup(), first);
    assert.equal(lister.calls(), 1);
  });
});

test(
  "the lookup rethrows the first failure without listing again",
  {
    skip: tempIsInsideRepository()
      ? "the temp directory is inside a git repository"
      : false,
  },
  () => {
    withTree((root) => {
      mkdirSync(path.join(root, ".git"));
      const lister = counting(root);
      const lookup = lazyTrackedLookup(lister.list);
      const caught = (): unknown => {
        try {
          lookup();
        } catch (error) {
          return error;
        }
        assert.fail("the lookup did not throw");
      };
      const first = caught();
      assert.match(String(first), /^Error: listing tracked files: /);
      assert.equal(caught(), first);
      assert.equal(lister.calls(), 1);
    });
  },
);
