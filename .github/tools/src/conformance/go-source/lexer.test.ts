import assert from "node:assert/strict";
import { test } from "node:test";
import { goStringLiterals, goTokens } from "./lexer.ts";

test("interpreted and raw literals are returned in order, unquoted", () => {
  assert.deepEqual(
    goStringLiterals(
      'package x\n\nvar a, b = "one", `two\\n"three"`\nconst c = "four"\n',
    ),
    ["one", 'two\\n"three"', "four"],
  );
});

test("comments never contribute literals, and a comment marker inside a literal is not a comment", () => {
  assert.deepEqual(
    goStringLiterals(
      '// "no"\n/* "no" `no` */\nvar a = "http://x // y /* z */"\n// "no"\nvar b = "yes"\n',
    ),
    ["http://x // y /* z */", "yes"],
  );
});

test("a rune literal holding a quote does not open a string", () => {
  assert.deepEqual(
    goStringLiterals(
      "var q = '\"'\nvar b = '`'\nvar s = \"after\"\nvar e = '\\''\nvar t = \"end\"\n",
    ),
    ["after", "end"],
  );
});

test("every escape a rune literal may hold is accepted", () => {
  assert.deepEqual(
    goStringLiterals(
      "var r = []rune{'\\'', '\\n', '\\\\', '\\x41', '\\101', '\\u00e9', '\\U0001F600', '\\a'}\nvar s = \"after\"\n",
    ),
    ["after"],
  );
});

test("escapes are resolved", () => {
  assert.deepEqual(
    goStringLiterals(
      'var a = "a\\tb\\n\\\\ \\"q\\" \\x41 \\101 \\u00e9 \\U0001F600 \\a\\b\\f\\r\\v"\n',
    ),
    ['a\tb\n\\ "q" A A é 😀 \x07\b\f\r\v'],
  );
});

test("carriage returns are dropped from a raw literal, as the Go scanner does", () => {
  assert.deepEqual(goStringLiterals("var a = `x\r\ny`\n"), ["x\ny"]);
});

test("a raw literal may span lines and hold an escape-looking backslash", () => {
  assert.deepEqual(goStringLiterals("var a = `one\n\\ntwo`\n"), [
    "one\n\\ntwo",
  ]);
});

const malformed: Array<[string, string, RegExp]> = [
  [
    "an unterminated string",
    'var a = "open\n',
    /string literal not terminated/,
  ],
  [
    "a string open at the end of the file",
    'var a = "open',
    /string literal not terminated/,
  ],
  [
    "an unterminated raw string",
    "var a = `open\n",
    /raw string literal not terminated/,
  ],
  ["an unterminated rune", "var a = 'x\n", /rune literal not terminated/],
  [
    "an unterminated block comment",
    "/* open\nvar a = 1\n",
    /comment not terminated/,
  ],
  ["an unknown escape", 'var a = "\\q"\n', /unknown escape sequence/],
  [
    "an escaped single quote in a string",
    'var a = "\\\'"\n',
    /unknown escape sequence/,
  ],
  ["an unknown escape in a rune", "var a = '\\q'\n", /unknown escape sequence/],
  [
    "an escaped double quote in a rune",
    "var a = '\\\"'\n",
    /unknown escape sequence/,
  ],
  [
    "a short hex escape in a rune",
    "var a = '\\x4'\n",
    /escape sequence is incomplete/,
  ],
  ["a surrogate escape in a rune", "var a = '\\ud800'\n", /invalid Unicode/],
  ["a short hex escape", 'var a = "\\x4"\n', /escape sequence is incomplete/],
  [
    "a non-hex unicode escape",
    'var a = "\\u12g4"\n',
    /escape sequence is incomplete/,
  ],
  ["a surrogate escape", 'var a = "\\ud800"\n', /invalid Unicode code point/],
  [
    "a code point past the maximum",
    'var a = "\\U00110000"\n',
    /invalid Unicode code point/,
  ],
  [
    "an octal escape over 255",
    'var a = "\\400"\n',
    /octal escape value is invalid/,
  ],
  ["a short octal escape", 'var a = "\\12"\n', /octal escape value is invalid/],
];

for (const [name, source, error] of malformed) {
  test(`${name} is an error`, () => {
    assert.throws(() => goStringLiterals(source), error);
  });
}

test("goTokens yields strings, runes, words and punctuation in order and skips comments", () => {
  const tokens = [
    ...goTokens(
      "import x \"a/b\" // c\n/* d */ import (\n\t. `r`\n)\nvar q = 'q'\n",
    ),
  ];
  assert.deepEqual(tokens, [
    { kind: "word", text: "import" },
    { kind: "word", text: "x" },
    { kind: "string", value: "a/b", text: '"a/b"' },
    { kind: "word", text: "import" },
    { kind: "punct", text: "(" },
    { kind: "punct", text: "." },
    { kind: "string", value: "r", text: "`r`" },
    { kind: "punct", text: ")" },
    { kind: "word", text: "var" },
    { kind: "word", text: "q" },
    { kind: "word", text: "=" },
    { kind: "rune" },
  ]);
});

test("a raw literal's text keeps its carriage returns while its value drops them", () => {
  assert.deepEqual(
    [...goTokens("`a\r\nb`")],
    [{ kind: "string", value: "a\nb", text: "`a\r\nb`" }],
  );
});

test("a form feed and a vertical tab separate words, which Go's scanner would reject", () => {
  assert.deepEqual(
    [...goTokens("a\fb\vc")],
    [
      { kind: "word", text: "a" },
      { kind: "word", text: "b" },
      { kind: "word", text: "c" },
    ],
  );
});

test("a word ends at a quote, a backquote, a comment opener or punctuation", () => {
  assert.deepEqual(
    [...goTokens("a\"b\" c'd' e`f` g//h\ni/*j*/k(l);m.n")],
    [
      { kind: "word", text: "a" },
      { kind: "string", value: "b", text: '"b"' },
      { kind: "word", text: "c" },
      { kind: "rune" },
      { kind: "word", text: "e" },
      { kind: "string", value: "f", text: "`f`" },
      { kind: "word", text: "g" },
      { kind: "word", text: "i" },
      { kind: "word", text: "k" },
      { kind: "punct", text: "(" },
      { kind: "word", text: "l" },
      { kind: "punct", text: ")" },
      { kind: "punct", text: ";" },
      { kind: "word", text: "m" },
      { kind: "punct", text: "." },
      { kind: "word", text: "n" },
    ],
  );
});

test("goTokens is lazy: a malformed literal after the last token read is never reached", () => {
  const iterator = goTokens('package x\n\nvar a = "open\n');
  assert.deepEqual(iterator.next().value, { kind: "word", text: "package" });
  assert.deepEqual(iterator.next().value, { kind: "word", text: "x" });
  assert.throws(() => {
    for (;;) if (iterator.next().done) break;
  }, /string literal not terminated/);
});
