import assert from "node:assert/strict";
import { test } from "node:test";
import { isLocal } from "./local-path.ts";

// Paths every platform answers alike.
const EVERYWHERE: Array<[string, boolean]> = [
  ["", false],
  [".", true],
  ["..", false],
  ["../a", false],
  ["/", false],
  ["/a", false],
  ["/a/../..", false],
  ["a", true],
  ["a/../a", true],
  ["a/", true],
  ["a/.", true],
  ["a/./b/./c", true],
  ["a/../b:/../../c", false],
  // A "." or empty component must not count as a directory entered.
  ["./..", false],
  ["a/./../..", false],
  ["a//../..", false],
  ["a//..", true],
  ["a//b/../..", true],
];

// Device names, drive letters and backslash paths, which only `win32` rejects.
const WINDOWS: Array<[string, boolean]> = [
  ["NUL", false],
  ["nul", false],
  ["nul ", false],
  ["nul.", false],
  ["a/nul:", false],
  ["a/nul : a", false],
  ["com0", true],
  ["com1", false],
  ["com2", false],
  ["com3", false],
  ["com4", false],
  ["com5", false],
  ["com6", false],
  ["com7", false],
  ["com8", false],
  ["com9", false],
  ["com\u00b9", false],
  ["com\u00b2", false],
  ["com\u00b3", false],
  ["com\u00b9 : a", false],
  ["cOm1", false],
  ["lpt1", false],
  ["LPT1", false],
  ["lpt\u00b3", false],
  ["./nul", false],
  ["\\", false],
  ["\\a", false],
  ["C:", false],
  ["C:\\a", false],
  ["..\\a", false],
  ["a/../c:", false],
  ["CONIN$", false],
  ["conin$", false],
  ["CONOUT$", false],
  ["conout$", false],
  ["dollar$", true],
];

// The edges of each Windows rule, each answered as Windows 11 answered it when this table was written.
const WINDOWS_EXTRA: Array<[string, boolean]> = [
  // A colon anywhere, not only at a drive letter.
  ["a:b", false],
  ["a/b:c/d", false],
  ["a/b:", false],
  // Rooted by either separator.
  ["/a", false],
  ["\\\\host\\share", false],
  // Reserved names in any component, with the base ending at the first dot.
  ["a/nul/b", false],
  ["a\\con\\b", false],
  ["aux", false],
  ["PRN", false],
  ["lpt9", false],
  ["lpt0", true],
  ["com10", true],
  ["con1", true],
  ["null", true],
  ["conin", true],
  ["conin$x", true],
  ["nul ..", false],
  ["com1.", false],
  ["aux. ", false],
  // Windows 11 does not reserve a device name that has an extension.
  ["nul.txt", true],
  ["a/CON.txt", true],
  ["nul .txt", true],
  ["conout$.x", true],
  // Not device names.
  ["nul\t", true],
  ["a/b.", true],
  // Separators are interchangeable, and cleaning applies to both.
  ["a\\..\\b", true],
  ["a/..\\..", false],
  ["a\\b\\..\\..\\c", true],
  ["a\\b\\..\\..\\..\\c", false],
  ["a//b", true],
  ["a\\.\\..\\..", false],
  [".\\..", false],
  ["a\\.\\..", true],
];

// Unix has none of the Windows rules: a colon, a backslash and a device name are ordinary characters.
const UNIX_EXTRA: Array<[string, boolean]> = [
  ["a:b", true],
  ["nul", true],
  ["con.txt", true],
  ["a\\..\\..", true],
  ["a/b/../../..", false],
  ["a/b/../..", true],
  ["a//b", true],
  ["./a", true],
  ["a/..", true],
  ["..a", true],
  ["a/../b/../..", false],
];

test("isLocal answers the shared cases alike on every platform", () => {
  for (const platform of ["linux", "darwin", "win32"])
    for (const [name, want] of EVERYWHERE)
      assert.equal(
        isLocal(name, platform),
        want,
        `${platform}: IsLocal(${JSON.stringify(name)})`,
      );
});

test("isLocal rejects device names, drive letters and backslash roots on win32", () => {
  for (const [name, want] of WINDOWS)
    assert.equal(
      isLocal(name, "win32"),
      want,
      `IsLocal(${JSON.stringify(name)})`,
    );
});

test("isLocal answers the edges of each Windows rule on win32", () => {
  for (const [name, want] of WINDOWS_EXTRA)
    assert.equal(
      isLocal(name, "win32"),
      want,
      `IsLocal(${JSON.stringify(name)})`,
    );
});

test("isLocal applies none of the Windows rules on Unix", () => {
  for (const [name, want] of UNIX_EXTRA)
    assert.equal(
      isLocal(name, "linux"),
      want,
      `IsLocal(${JSON.stringify(name)})`,
    );
});

test("isLocal defaults to the host platform", () => {
  assert.equal(isLocal("a/b"), true);
  assert.equal(isLocal("../a"), false);
  assert.equal(isLocal("a:b"), process.platform !== "win32");
});
