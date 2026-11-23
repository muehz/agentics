import assert from "node:assert/strict";
import { mkdirSync, readFileSync, statSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { test } from "node:test";
import {
  FileError,
  fileErrorOf,
  readDirectory,
  readFile,
  writeFile,
} from "./files.ts";
import { posix, scratchDir } from "./testing/support.ts";

function fails(
  run: () => unknown,
  kind: FileError["kind"],
  message: string,
): void {
  assert.throws(run, (error: unknown) => {
    assert.ok(error instanceof FileError);
    assert.equal(error.kind, kind);
    assert.equal(error.message, message);
    return true;
  });
}

test("readFile returns the bytes of a regular file", () => {
  const dir = scratchDir("agentics-release-files-");
  const path = join(dir, "a.txt");
  writeFileSync(path, "hello\n");
  assert.equal(readFile(path).toString("utf8"), "hello\n");
});

test("readFile says a missing file does not exist", () => {
  const path = join(scratchDir("agentics-release-files-"), "absent");
  fails(() => readFile(path), "missing", `${path}: does not exist`);
});

test("readFile says a directory is not a regular file", () => {
  const dir = scratchDir("agentics-release-files-");
  fails(() => readFile(dir), "not-a-file", `${dir}: is not a regular file`);
});

test("readDirectory returns the entry names in default sort order", () => {
  const dir = scratchDir("agentics-release-files-");
  // The names differ in more than case, so a case-insensitive file system keeps all of them.
  for (const name of ["b", "a", "Z", ".hidden"]) {
    writeFileSync(join(dir, name), "");
  }
  mkdirSync(join(dir, "c"));
  assert.deepEqual(readDirectory(dir), [".hidden", "Z", "a", "b", "c"]);
});

test("readDirectory says a regular file is not a directory", () => {
  const path = join(scratchDir("agentics-release-files-"), "file");
  writeFileSync(path, "");
  fails(
    () => readDirectory(path),
    "not-a-directory",
    `${path}: is not a directory`,
  );
});

test("readDirectory says a missing directory does not exist", () => {
  const path = join(scratchDir("agentics-release-files-"), "absent");
  fails(() => readDirectory(path), "missing", `${path}: does not exist`);
});

test("writeFile writes the data and a write below a regular file fails", () => {
  const dir = scratchDir("agentics-release-files-");
  const path = join(dir, "out");
  writeFile(path, "data");
  assert.equal(readFileSync(path, "utf8"), "data");

  const below = join(path, "child");
  // Linux reports a path below a regular file as ENOTDIR; Windows reports it as absent.
  if (posix) {
    fails(
      () => writeFile(below, "x"),
      "not-a-directory",
      `${below}: is not a directory`,
    );
  } else {
    fails(() => writeFile(below, "x"), "missing", `${below}: does not exist`);
  }
});

test("fileErrorOf names any other code, or says unreadable without one", () => {
  const cause = Object.assign(new Error("EACCES: permission denied"), {
    code: "EACCES",
  });
  const coded = fileErrorOf(cause, "x");
  assert.equal(coded.kind, "unreadable");
  assert.equal(coded.message, "x: EACCES");
  assert.equal(coded.cause, cause);

  const plain = fileErrorOf(new Error("boom"), "y");
  assert.equal(plain.kind, "unreadable");
  assert.equal(plain.message, "y: unreadable");

  assert.equal(fileErrorOf(coded, "z"), coded);
});

test("fileErrorOf says unreadable for a thrown value with no string code and keeps it as the cause", () => {
  const numeric = Object.assign(new Error("odd"), { code: 5 });
  for (const thrown of [new Error("boom"), numeric, "boom", null, undefined]) {
    const error = fileErrorOf(thrown, "p");
    assert.equal(error.kind, "unreadable");
    assert.equal(error.message, "p: unreadable");
    assert.equal(Object.hasOwn(error, "cause"), true);
    assert.equal(error.cause, thrown);
  }
});

test("a FileError names itself in its string form", () => {
  assert.equal(
    String(new FileError("missing", "p")),
    "FileError: p: does not exist",
  );
});

// Windows has no POSIX permission bits to compare.
test(
  "writeFile creates the file with mode 0644 even under an open umask",
  { skip: process.platform === "win32" },
  () => {
    const path = join(scratchDir("agentics-release-files-"), "out");
    const previous = process.umask(0);
    try {
      writeFile(path, "data");
    } finally {
      process.umask(previous);
    }
    assert.equal(statSync(path).mode & 0o777, 0o644);
  },
);

test("each kind has its own sentence", () => {
  const cases: [FileError["kind"], string][] = [
    ["missing", "p: does not exist"],
    ["not-a-file", "p: is not a regular file"],
    ["not-a-directory", "p: is not a directory"],
    ["symlink", "p: passes through a link"],
    ["outside-repo", "p: is outside the checkout"],
    ["unreadable", "p: unreadable"],
  ];
  for (const [kind, message] of cases) {
    assert.equal(new FileError(kind, "p").message, message);
  }
});
