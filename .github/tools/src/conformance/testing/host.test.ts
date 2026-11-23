import assert from "node:assert/strict";
import {
  existsSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  readdirSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { test } from "node:test";
import {
  DIRECTORY_LINK_SKIP,
  SYMLINK_SKIP,
  canLinkDirectory,
  canSymlink,
  linkDirectory,
  makesUnreadableDirs,
  removeTree,
  reportsNotADirectory,
  makesUnreadableFilesOnLinux,
  withOutside,
} from "./host.ts";

const notRoot = process.getuid?.() !== 0;

function withScratch(body: (dir: string) => void): void {
  const dir = mkdtempSync(path.join(tmpdir(), "agentics-host-test-"));
  try {
    body(dir);
  } finally {
    removeTree(dir);
  }
}

test("the skip reasons name the missing capability", () => {
  assert.equal(SYMLINK_SKIP, "this host cannot create a symlink");
  assert.equal(DIRECTORY_LINK_SKIP, "this host cannot create a directory link");
});

test("canSymlink returns the same answer twice", () => {
  assert.equal(typeof canSymlink(), "boolean");
  assert.equal(canSymlink(), canSymlink());
});

test("canLinkDirectory returns the same answer twice, and is true wherever a file symlink can be made", () => {
  assert.equal(typeof canLinkDirectory(), "boolean");
  assert.equal(canLinkDirectory(), canLinkDirectory());
  if (canSymlink()) assert.equal(canLinkDirectory(), true);
});

test(
  "linkDirectory makes a link that reads through to the target, creating the link's missing parents",
  { skip: canLinkDirectory() ? false : DIRECTORY_LINK_SKIP },
  () => {
    withScratch((dir) => {
      const target = path.join(dir, "target");
      mkdirSync(target);
      writeFileSync(path.join(target, "file.txt"), "through\n");
      const link = path.join(dir, "missing", "parent", "link");
      const kind = linkDirectory(target, link);
      assert.ok(kind === "dir" || kind === "junction", `kind was ${kind}`);
      if (process.platform !== "win32") assert.equal(kind, "dir");
      assert.equal(
        readFileSync(path.join(link, "file.txt"), "utf8"),
        "through\n",
      );
    });
  },
);

test("linkDirectory returns undefined without throwing when the link path lies below a regular file", () => {
  withScratch((dir) => {
    const target = path.join(dir, "target");
    mkdirSync(target);
    writeFileSync(path.join(dir, "file"), "x");
    assert.equal(
      linkDirectory(target, path.join(dir, "file", "below", "link")),
      undefined,
    );
  });
});

test("removeTree removes a nested tree and is silent on a missing path", () => {
  const dir = mkdtempSync(path.join(tmpdir(), "agentics-host-test-"));
  mkdirSync(path.join(dir, "a", "b", "c"), { recursive: true });
  writeFileSync(path.join(dir, "a", "b", "c", "file"), "x");
  writeFileSync(path.join(dir, "a", "top"), "x");
  removeTree(dir);
  assert.equal(existsSync(dir), false);
  assert.doesNotThrow(() => removeTree(dir));
});

test("withOutside gives fn a fresh empty directory and removes it after fn returns", () => {
  let seen = "";
  const result = withOutside((dir) => {
    seen = dir;
    assert.deepEqual(readdirSync(dir), []);
    writeFileSync(path.join(dir, "file"), "x");
    return 42;
  });
  assert.equal(result, 42);
  assert.notEqual(seen, "");
  assert.equal(existsSync(seen), false);
});

test("withOutside removes its directory after fn throws, and rethrows", () => {
  let seen = "";
  assert.throws(
    () =>
      withOutside((dir) => {
        seen = dir;
        throw new Error("boom");
      }),
    /boom/,
  );
  assert.equal(existsSync(seen), false);
});

test("withOutside keeps the directory while a returned promise is pending and removes it once it resolves", async () => {
  let seen = "";
  let release: (value: string) => void = () => {};
  const pending = withOutside(async (dir) => {
    seen = dir;
    return new Promise<string>((resolve) => {
      release = resolve;
    });
  });
  await new Promise((resolve) => setImmediate(resolve));
  assert.equal(existsSync(seen), true);
  release("done");
  assert.equal(await pending, "done");
  assert.equal(existsSync(seen), false);
});

test("withOutside removes its directory after a returned promise rejects, and the rejection passes through", async () => {
  let seen = "";
  await assert.rejects(
    withOutside(async (dir) => {
      seen = dir;
      throw new Error("late boom");
    }),
    /late boom/,
  );
  assert.equal(existsSync(seen), false);
});

test("reportsNotADirectory is false on Windows and true on POSIX hosts", () => {
  assert.equal(reportsNotADirectory(), process.platform !== "win32");
});

test("makesUnreadableDirs is false on Windows and as root, and true otherwise", () => {
  assert.equal(
    makesUnreadableDirs(),
    process.platform === "win32" ? false : notRoot,
  );
});

test("makesUnreadableFilesOnLinux is true only on Linux and not as root", () => {
  assert.equal(
    makesUnreadableFilesOnLinux(),
    process.platform === "linux" ? notRoot : false,
  );
});
