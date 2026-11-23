import assert from "node:assert/strict";
import { test } from "node:test";
import { readGoMod } from "./gomod.ts";

test("line and block forms, quoting, comments and several blocks read in file order", () => {
  const source = [
    "module example.com/agentics/hooks // c",
    "",
    "go 1.27.3",
    "",
    "require github.com/a/one v1.0.0",
    "require (",
    "\tgithub.com/a/two v1.0.0 // indirect",
    '\t"github.com/a/three" v1.0.0',
    "\t// a comment line",
    ")",
    "require ( )",
    "require (",
    "\tgithub.com/a/four v1.0.0",
    ")",
    "replace github.com/a/one => ../local",
    "replace (",
    "\tgithub.com/a/two v1.0.0 => github.com/b/two v1.2.3",
    '\t"github.com/a/three" => "./three"',
    ")",
    "tool (",
    "\tgithub.com/a/cmd",
    ")",
    "",
  ].join("\n");
  assert.deepEqual(readGoMod(source), {
    requires: [
      "github.com/a/one",
      "github.com/a/two",
      "github.com/a/three",
      "github.com/a/four",
    ],
    replaces: [
      { old: "github.com/a/one", new: "../local" },
      { old: "github.com/a/two", new: "github.com/b/two" },
      { old: "github.com/a/three", new: "./three" },
    ],
  });
});

test("a double slash ends a token, and carriage returns are white space", () => {
  assert.deepEqual(readGoMod("require a v1//c\r\nrequire b v2\r\n").requires, [
    "a",
    "b",
  ]);
  assert.deepEqual(readGoMod("replace a => ./b//c\n").replaces, [
    { old: "a", new: "./b" },
  ]);
  // A carriage return ends the token before it, so it never reaches a path.
  assert.deepEqual(readGoMod("replace a => ./b\r\n").replaces, [
    { old: "a", new: "./b" },
  ]);
});

test("a bracket, a brace or a comma ends a token and is a token itself", () => {
  for (const separator of ["[", "]", "{", "}", ","])
    assert.throws(
      () => readGoMod(`require a${separator}b v1\n`),
      /usage: require/,
      separator,
    );
});

test("a raw-quoted token ends at the first backquote, whatever precedes it, and may hold spaces", () => {
  assert.throws(
    () => readGoMod("require `a\\` v1\n"),
    /unquoted string cannot contain quote/,
  );
  assert.throws(
    () => readGoMod("require `a b` v1\n"),
    /unquoted string cannot contain quote/,
  );
});

test("an escaped quote in a quoted path is read", () => {
  assert.deepEqual(readGoMod('require "a\\"b" v1\n').requires, ['a"b']);
});

test("every replace shape with versions on either side is read", () => {
  const read = (line: string) => readGoMod(`${line}\n`).replaces;
  assert.deepEqual(read("replace a => b v1"), [{ old: "a", new: "b" }]);
  assert.deepEqual(read("replace a v1 => b v2"), [{ old: "a", new: "b" }]);
  assert.deepEqual(read("replace a v1 => ./b"), [{ old: "a", new: "./b" }]);
});

const malformed: Array<[string, string, RegExp]> = [
  ["a block comment", "require a v1 /* c */\n", /\/\* \*\//],
  ["an unterminated quote", 'require "a v1\n', /unterminated/],
  ["a bad escape", 'require "a\\q" v1\n', /unknown escape sequence/],
  ["an unterminated block", "require (\n\ta v1\n", /unterminated block/],
  ["text after a closing paren", "require (\n\ta v1\n) x\n", /closing paren/],
  [
    "a block header of two tokens",
    "require x (\n\ta v1\n)\n",
    /unknown block type/,
  ],
  ["a require with one operand", "require a\n", /usage: require/],
  ["a require with three operands", "require a v1 v2\n", /usage: require/],
  ["a replace with no arrow", "replace a b\n", /usage: replace/],
  ["a replace with a glued arrow", "replace a v1=>b\n", /usage: replace/],
  [
    "a replace with a glued arrow and spaces",
    "replace a =>b\n",
    /usage: replace/,
  ],
  [
    "a raw-quoted path",
    "require `a` v1\n",
    /unquoted string cannot contain quote/,
  ],
  [
    "a single-quoted path",
    "require 'a' v1\n",
    /unquoted string cannot contain quote/,
  ],
  ["an inline parenthesis", "require (a v1)\n", /usage: require/],
  [
    "a replace with a glued arrow after a version",
    "replace a v1 =>b v2\n",
    /usage: replace/,
  ],
  [
    "a replace with too many operands",
    "replace a => b v1 x\n",
    /usage: replace/,
  ],
  [
    "a quote-holding require version",
    "require a 'v1'\n",
    /unquoted string cannot contain quote/,
  ],
  [
    "a quote-holding old replace version",
    "replace a 'v1' => b v2\n",
    /unquoted string cannot contain quote/,
  ],
  [
    "a quote-holding new replace version",
    "replace a => b 'v2'\n",
    /unquoted string cannot contain quote/,
  ],
  ["a block comment inside a token", "require a v1/*c*/\n", /\/\* \*\//],
];
for (const [name, source, error] of malformed)
  test(`${name} is an error`, () => {
    assert.throws(() => readGoMod(source), error);
  });
