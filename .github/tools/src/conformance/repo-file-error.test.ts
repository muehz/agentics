import assert from "node:assert/strict";
import { test } from "node:test";
import {
  RepoFileError,
  repoFileErrorOf,
  type RepoFileErrorKind,
} from "./repo-file-error.ts";

function fsError(code: string): NodeJS.ErrnoException {
  const error: NodeJS.ErrnoException = new Error(
    `${code}: something failed, open 'x'`,
  );
  error.code = code;
  return error;
}

const SENTENCES: ReadonlyArray<[RepoFileErrorKind, string]> = [
  ["missing", "skills/a/SKILL.md: does not exist"],
  ["not-a-file", "skills/a/SKILL.md: is not a regular file"],
  ["not-a-directory", "skills/a/SKILL.md: is not a directory"],
  ["symlink", "skills/a/SKILL.md: passes through a link"],
  ["outside-repo", "skills/a/SKILL.md: is outside the checkout"],
  ["unreadable", "skills/a/SKILL.md: unreadable"],
];

for (const [kind, message] of SENTENCES) {
  test(`a ${kind} error carries its kind, path and sentence`, () => {
    const error = new RepoFileError(kind, "skills/a/SKILL.md");
    assert.ok(error instanceof Error);
    assert.equal(error.name, "RepoFileError");
    assert.equal(error.kind, kind);
    assert.equal(error.path, "skills/a/SKILL.md");
    assert.equal(error.message, message);
    assert.equal(Object.hasOwn(error, "cause"), false);
    assert.equal("code" in error, false);
  });
}

test("an unreadable error shows its detail in place of the default word", () => {
  const error = new RepoFileError("unreadable", "docs/a.md", {
    detail: "EACCES",
  });
  assert.equal(error.kind, "unreadable");
  assert.equal(error.message, "docs/a.md: EACCES");
});

test("an empty detail falls back to the default word", () => {
  const error = new RepoFileError("unreadable", "docs/a.md", { detail: "" });
  assert.equal(error.message, "docs/a.md: unreadable");
});

test("detail is ignored for every kind but unreadable", () => {
  for (const [kind, message] of SENTENCES) {
    if (kind === "unreadable") continue;
    const error = new RepoFileError(kind, "skills/a/SKILL.md", {
      detail: "EACCES",
    });
    assert.equal(error.message, message);
  }
});

test("the constructor writes the path with forward slashes", () => {
  const error = new RepoFileError("missing", "skills\\a\\SKILL.md");
  assert.equal(error.path, "skills/a/SKILL.md");
  assert.equal(error.message, "skills/a/SKILL.md: does not exist");
});

test("an outside-repo error keeps an absolute path as given", () => {
  const error = new RepoFileError("outside-repo", "/etc/passwd");
  assert.equal(error.path, "/etc/passwd");
  assert.equal(error.message, "/etc/passwd: is outside the checkout");
});

test("the constructor keeps a given cause", () => {
  const cause = new Error("lstat said so");
  const error = new RepoFileError("symlink", "skills/link", { cause });
  assert.equal(error.cause, cause);
});

const MAPPED: ReadonlyArray<[string, RepoFileErrorKind, string]> = [
  ["ENOENT", "missing", "docs/a.md: does not exist"],
  ["EISDIR", "not-a-file", "docs/a.md: is not a regular file"],
  ["ENOTDIR", "not-a-directory", "docs/a.md: is not a directory"],
];

for (const [code, kind, message] of MAPPED) {
  test(`repoFileErrorOf maps ${code} to ${kind}`, () => {
    const cause = fsError(code);
    const error = repoFileErrorOf(cause, "docs\\a.md");
    assert.ok(error instanceof RepoFileError);
    assert.equal(error.kind, kind);
    assert.equal(error.path, "docs/a.md");
    assert.equal(error.message, message);
    assert.equal(error.cause, cause);
  });
}

test("repoFileErrorOf maps any other code to unreadable and shows the code", () => {
  for (const code of ["EACCES", "EPERM", "ELOOP", "EMFILE"]) {
    const cause = fsError(code);
    const error = repoFileErrorOf(cause, "docs/a.md");
    assert.equal(error.kind, "unreadable");
    assert.equal(error.message, `docs/a.md: ${code}`);
    assert.equal(error.cause, cause);
  }
});

test("repoFileErrorOf maps an error without a string code to plain unreadable", () => {
  const numeric = Object.assign(new Error("odd"), { code: 13 });
  for (const cause of [new Error("boom"), numeric, "boom", null, undefined]) {
    const error = repoFileErrorOf(cause, "docs/a.md");
    assert.equal(error.kind, "unreadable");
    assert.equal(error.message, "docs/a.md: unreadable");
    assert.equal(Object.hasOwn(error, "cause"), true);
    assert.equal(error.cause, cause);
  }
});

test("repoFileErrorOf returns a RepoFileError unchanged", () => {
  const original = new RepoFileError("symlink", "skills/link");
  assert.equal(repoFileErrorOf(original, "some/other/path"), original);
  assert.equal(original.path, "skills/link");
});

test("a subclass is still returned unchanged and keeps the base name", () => {
  class Temporary extends RepoFileError {}
  const original = new Temporary("missing", "docs/a.md");
  assert.equal(repoFileErrorOf(original, "x"), original);
  assert.equal(original.name, "RepoFileError");
});
