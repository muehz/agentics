import assert from "node:assert/strict";
import { test } from "node:test";
import {
  collapseWhitespace,
  crlfToLf,
  isOtherSymbol,
  oneLine,
} from "./text.ts";

// Builds a string from code points, so this file stays ASCII and each vector names the characters it means.
const chars = (...codePoints: number[]): string =>
  String.fromCodePoint(...codePoints);

test("collapseWhitespace collapses every run of \\s", () => {
  assert.equal(collapseWhitespace("a \t\r\n\f b"), "a b");
  assert.equal(
    collapseWhitespace(chars(0x61, 0x0b, 0x62, 0xa0, 0x63, 0xfeff, 0x64)),
    "a b c d",
  );
  // U+0085 and U+200B are not \s, so they survive.
  assert.equal(
    collapseWhitespace(chars(0x61, 0x85, 0x62, 0x200b, 0x63)),
    chars(0x61, 0x85, 0x62, 0x200b, 0x63),
  );
  assert.equal(collapseWhitespace("  a  "), " a ");
  assert.equal(collapseWhitespace(""), "");
});

test("isOtherSymbol is the So general category", () => {
  for (const cp of [0x27a1, 0x26a0, 0x1f642, 0xfffd])
    assert.equal(isOtherSymbol(cp), true, cp.toString(16));
  // U+203C is punctuation (Po) and U+FE0F is a mark: neither is So, so neither exempts a selector.
  for (const cp of [
    0x61, 0x30, 0x203c, 0x0a, 0xfe0f, 0xe0001, 0xd800, 0xdfff, -1, 0x110000,
  ])
    assert.equal(isOtherSymbol(cp), false, cp.toString(16));
});

test("oneLine joins the \\s-separated words with one space", () => {
  assert.equal(oneLine("  a \t\r\n  b  "), "a b");
  assert.equal(oneLine(chars(0x61, 0xfeff, 0x62)), "a b");
  assert.equal(
    oneLine(
      chars(0xa0, 0x20, 0x61, 0xa0, 0x2028, 0x3000, 0x0b, 0x62, 0x20, 0xa0),
    ),
    "a b",
  );
  // U+0085 and U+200B are not \s, so they stay inside a word.
  assert.equal(oneLine(chars(0x61, 0x85, 0x62)), chars(0x61, 0x85, 0x62));
  assert.equal(oneLine(chars(0x61, 0x200b, 0x62)), chars(0x61, 0x200b, 0x62));
  assert.equal(oneLine("a"), "a");
  assert.equal(oneLine(""), "");
  assert.equal(oneLine(chars(0x20, 0xa0, 0x3000, 0x0a)), "");
});

test("crlfToLf replaces only CR LF pairs", () => {
  assert.equal(crlfToLf("a\r\nb"), "a\nb");
  assert.equal(crlfToLf("a\rb"), "a\rb");
  assert.equal(crlfToLf("a\r\r\nb"), "a\r\nb");
  assert.equal(crlfToLf("a\r\nb\r\r\nc\rd"), "a\nb\r\nc\rd");
  assert.equal(crlfToLf("a\nb"), "a\nb");
  assert.equal(crlfToLf(""), "");
});
