import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { EventEmitter } from "node:events";
import {
  existsSync,
  mkdirSync,
  mkdtempSync,
  rmSync,
  symlinkSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { after, test } from "node:test";
import { fileURLToPath } from "node:url";
import { repoRoot, type Failure } from "../../check.ts";
import {
  auditEveryFamily,
  closing,
  E_ACUTE,
  STUB_ONE,
  stubFailure,
} from "../../testing/skills-seeds.ts";
import { cloneSkillsRepo, writeFile } from "../../testing/skills-repo.ts";
import {
  EVALS_CLI_ENV,
  EVALS_ROOT_ENV,
  evalsCheckCommand,
  evalsMainEntry,
  evalsRoot,
  FAMILY,
  isDirectory,
  NODE_FLAGS,
  parseEvalsCheck,
  runEvalsCheck,
  SKIP_LINE,
  tailLines,
  trustedRoot,
  type ChildLike,
  type Spawn,
} from "./eval-check.ts";
import type { ResolvedSkillsContext } from "./index.ts";
import { removeTree } from "../../testing/host.ts";

// Where a vector expects a line that is not valid output, only that class is asserted; the pinned streams at the end of
// this file pin the reason text.

const NOT_VALID =
  "skills: evals check printed a line that is not valid output: ";
const NO_CLOSING = "skills: evals check printed no closing line";
const AFTER_CLOSING =
  "skills: evals check printed a line after its closing line";
const EMPTY_OBJECT = "skills: evals check printed an empty object";
const NO_RULE = "skills: evals check printed a failure line with no rule";
const MISSING =
  "skills: evals check failure line is missing its path or message";
const COULD_NOT = "skills: evals check could not audit: ";
const mismatch = (n: number): string =>
  `skills: evals check closing line does not match the ${n} failure lines read`;

// Non-ASCII inputs and the JSON backslash-u escape are built from code points so the source stays ASCII.
const U = String.fromCharCode(0x5c) + "u";
const BOM = String.fromCodePoint(0xfeff);
const LONG_S = String.fromCodePoint(0x017f);
const KELVIN = String.fromCodePoint(0x212a);
const DOTLESS_I = String.fromCodePoint(0x0131);
const NBSP = String.fromCodePoint(0x00a0);
const REPLACEMENT = String.fromCodePoint(0xfffd);
const GRIN = String.fromCodePoint(0x1f600);
const EURO = String.fromCodePoint(0x20ac);

const F = '{"rule":"r","path":"p","message":"m"}';
const D0 = '{"done":true,"failures":0}';
const D1 = '{"done":true,"failures":1}';
const RPM: Failure = { rule: "r", path: "p", message: "m" };

type Expected =
  { failures: Failure[] } | { error: string } | { notValid: true };

const ok = (...failures: Failure[]): Expected => ({ failures });
const fails = (error: string): Expected => ({ error });
const notValid: Expected = { notValid: true };
const one = (message: string): Expected => ok({ ...RPM, message });

function check(input: string, expected: Expected): void {
  if ("failures" in expected) {
    assert.deepEqual(parseEvalsCheck(input), expected.failures);
    return;
  }
  assert.throws(
    () => parseEvalsCheck(input),
    (error: unknown) => {
      assert.ok(error instanceof Error);
      if ("error" in expected) assert.equal(error.message, expected.error);
      else assert.ok(error.message.startsWith(NOT_VALID), error.message);
      return true;
    },
  );
}

const VECTORS: ReadonlyArray<readonly [string, string, Expected]> = [
  [
    "failures at the closing line",
    '{"rule":"eval-record-entry","path":"evals/skills.yaml","message":"no entry for skill \\"x\\""}\n' +
      D1 +
      "\n",
    ok({
      rule: "eval-record-entry",
      path: "evals/skills.yaml",
      message: 'no entry for skill "x"',
    }),
  ],
  [
    "empty path and message are kept",
    '{"rule":"r","path":"","message":""}\n' + D1 + "\n",
    ok({ rule: "r", path: "", message: "" }),
  ],
  ["a clean run", D0 + "\n", ok()],
  ["no closing line", F + "\n", fails(NO_CLOSING)],
  ["empty output", "", fails(NO_CLOSING)],
  ["whitespace-only output", " \n\t\r\n", fails(NO_CLOSING)],
  [
    "error line",
    '{"error":"no skill directories found"}\n',
    fails(COULD_NOT + "no skill directories found"),
  ],
  ["empty error line", '{"error":""}\n', fails(COULD_NOT)],
  [
    "error after failures drops them",
    F + '\n{"error":"x"}\n',
    fails(COULD_NOT + "x"),
  ],
  ["count disagrees", F + '\n{"done":true,"failures":2}\n', fails(mismatch(1))],
  ["not JSON", "compile error: undefined\n" + D0 + "\n", notValid],
  ["a line after done", D0 + "\n" + F + "\n", fails(AFTER_CLOSING)],
  ["garbage after done", D0 + "\nxx", notValid],
  ["unknown field", '{"done":true,"failures":0,"extra":1}\n', notValid],
  ["empty object", "{}\n" + D0 + "\n", fails(EMPTY_OBJECT)],
  ["top-level null", "null\n", notValid],
  ["top-level null then a closing line", "null\n" + D0, notValid],
  ["top-level array", "[]\n", notValid],
  ["top-level string", '"s"\n', notValid],
  ["top-level number", "1\n", notValid],
  ["top-level true", "true\n", notValid],
  [
    "done false",
    '{"done":false,"failures":0}\n',
    fails("skills: evals check closing line says done is false"),
  ],
  [
    "done without count",
    '{"done":true}\n',
    fails("skills: evals check closing line has no failure count"),
  ],
  [
    "failure line without rule",
    '{"path":"p","message":"m"}\n' + D0,
    fails(NO_RULE),
  ],
  [
    "failure line with empty rule",
    '{"rule":"","path":"p","message":"m"}\n' + D0,
    fails(NO_RULE),
  ],
  ["count line without done", '{"failures":0}\n', fails(NO_RULE)],
  ["done null is absent", '{"done":null,"failures":0}\n', fails(NO_RULE)],
  ["only a rule", '{"rule":"r"}\n' + D1, fails(MISSING)],
  ["rule and path", '{"rule":"r","path":"p"}\n' + D1, fails(MISSING)],
  ["rule and message", '{"rule":"r","message":"m"}\n' + D1, fails(MISSING)],
  [
    "null path and message",
    '{"rule":"r","path":null,"message":null}\n' + D1,
    fails(MISSING),
  ],
  [
    "failure line carries a count",
    '{"rule":"r","path":"p","message":"m","failures":1}\n' + D1,
    fails("skills: evals check failure line carries other fields"),
  ],
  ...(["rule", "path", "message"] as const).map(
    (field) =>
      [
        `closing line carries ${field}`,
        `{"${field}":"x","done":true,"failures":0}\n`,
        fails("skills: evals check closing line carries other fields"),
      ] as const,
  ),
  [
    "error line carries done",
    '{"error":"x","done":true,"failures":0}\n',
    fails("skills: evals check error line carries other fields"),
  ],
  [
    "error line carries a rule",
    '{"error":"x","rule":"r"}\n',
    fails("skills: evals check error line carries other fields"),
  ],
  [
    "a null rule, then a valid failure",
    '{"rule":null,"path":"p","message":"m"}\n' + F + "\n" + D1,
    fails(NO_RULE),
  ],
  // The count.
  ["count 0", D0, ok()],
  ["count -0", '{"done":true,"failures":-0}', ok()],
  [
    "count -1 is an integer that fails the count",
    '{"done":true,"failures":-1}',
    fails(mismatch(0)),
  ],
  ["count 1.0", F + '{"done":true,"failures":1.0}', notValid],
  ["count 1e0", F + '{"done":true,"failures":1e0}', notValid],
  ["count -0e0", '{"done":true,"failures":-0e0}', notValid],
  ["count 1.5", '{"done":true,"failures":1.5}', notValid],
  ['count "1"', F + '{"done":true,"failures":"1"}', notValid],
  ["count 1e400", '{"done":true,"failures":1e400}', notValid],
  [
    "count 2^53 - 1, the largest safe integer",
    '{"done":true,"failures":9007199254740991}',
    fails(mismatch(0)),
  ],
  [
    "count 2^53, which is not a safe integer",
    '{"done":true,"failures":9007199254740992}',
    notValid,
  ],
  [
    "count 2^53 + 1, which reads back as 2^53",
    '{"done":true,"failures":9007199254740993}',
    notValid,
  ],
  [
    "count -(2^53 - 1)",
    '{"done":true,"failures":-9007199254740991}',
    fails(mismatch(0)),
  ],
  ["count -(2^53)", '{"done":true,"failures":-9007199254740992}', notValid],
  ["count int64 max", '{"done":true,"failures":9223372036854775807}', notValid],
  [
    "count 1e20 as digits",
    '{"done":true,"failures":100000000000000000000}',
    notValid,
  ],
  ["two failures, count 2", F + F + '{"done":true,"failures":2}', ok(RPM, RPM)],
  // Field types.
  ["done 1", '{"done":1,"failures":0}', notValid],
  ['done "true"', '{"done":"true","failures":0}', notValid],
  ["rule 5", '{"rule":5,"path":"p","message":"m"}' + D1, notValid],
  ["error 1", '{"error":1}', notValid],
  ["an object value", '{"done":true,"failures":0,"rule":{}}', notValid],
  ["an array value", '{"done":true,"failures":[0]}', notValid],
  [
    "an unknown field with a nested value",
    '{"done":true,"failures":0,"x":{"a":[1,{"b":null}]}}',
    notValid,
  ],
  [
    "an unknown field set to null",
    '{"done":true,"failures":0,"x":null}',
    notValid,
  ],
  ["an empty key", '{"":1}', notValid],
  // Stream layout.
  ["two values on one line", F + " " + D1, ok(RPM)],
  [
    "values split over lines",
    '{"rule":"r",\n"path":"p",\n"message":"m"}\n{"done":\ntrue,"failures":1}\n',
    ok(RPM),
  ],
  [
    "a value split over lines, then a second value on its last line",
    '{"rule":"r",\n"path":"p","message":"m"} ' + D1 + "\n",
    ok(RPM),
  ],
  [
    "a fault after a split value names the line the faulty value starts on",
    '{"rule":"r",\n"path":"p","message":"m"}\n{"done":true,\n"failures":1,"x":0}\n',
    fails(NOT_VALID + 'line 3 has an unknown field "x"'),
  ],
  [
    "an unterminated value names the line it starts on",
    F + "\n\n" + '{"done":true,\n"failures":1',
    fails(NOT_VALID + "line 3 is not valid JSON"),
  ],
  [
    "an escaped quote and brace inside a string",
    '{"rule":"r","path":"p","message":"\\"}\\\\"}' + D1,
    one('"}\\'),
  ],
  [
    "a message holding many closing braces",
    `{"rule":"r","path":"p","message":"${"}".repeat(80_000)}"}\n` + D1,
    one("}".repeat(80_000)),
  ],
  [
    "a closing brace inside a string",
    '{"rule":"r","path":"p}","message":"}"} ' + D1,
    ok({ rule: "r", path: "p}", message: "}" }),
  ],
  ["blank lines", "\n\n" + F + "\n\n\n" + D1 + "\n\n", ok(RPM)],
  ["CRLF line ends", F + "\r\n" + D1 + "\r\n", ok(RPM)],
  ["no final newline", F + "\n" + D1, ok(RPM)],
  ["no whitespace between values", F + D1, ok(RPM)],
  ["null directly before an object", "null" + D0, notValid],
  ["null directly before garbage", "null)", notValid],
  ["a number directly before an object", "1" + D0, notValid],
  ["true directly before an object", "true" + D0, notValid],
  ["a truncated literal", "nul", notValid],
  ["a broken literal", "nulx", notValid],
  ["garbage before a value", "xx" + D0, notValid],
  [
    "a second value after the closing value",
    D0 + "\n" + D0,
    fails(AFTER_CLOSING),
  ],
  ["an empty object after the closing value", D0 + "{}", fails(AFTER_CLOSING)],
  ["null after the closing value", D0 + "null", notValid],
  ["a bracket after the closing value", D0 + "]", notValid],
  ["a BOM first", BOM + D0, notValid],
  ["a NUL first", String.fromCharCode(0) + D0, notValid],
  ["a vertical tab first", String.fromCharCode(0x0b) + D0, notValid],
  ["a no-break space first", NBSP + D0, notValid],
  ["a truncated final value", '{"done":true', notValid],
  ["a final value missing its brace", '{"done":true,"failures":0', notValid],
  ["a leading zero", '{"done":true,"failures":01}', notValid],
  ["a plus sign", '{"done":true,"failures":+1}', notValid],
  ["a trailing comma", '{"done":true,"failures":0,}', notValid],
  ["a raw tab in a string", '{"error":"a\tb"}', notValid],
  [
    "an invalid escape",
    '{"error":"' + String.fromCharCode(0x5c) + 'x"}',
    notValid,
  ],
  ["a short hex escape", '{"error":"' + U + '12"}', notValid],
  // Field names match in exact case, so a key in another case is not its field: it is an unknown key.
  [
    "upper-case DONE and Failures",
    '{"DONE":true,"Failures":0}',
    fails(NOT_VALID + 'line 1 has an unknown field "DONE"'),
  ],
  [
    "a count keyed Failures is no count",
    '{"done":true,"Failures":0}',
    fails(NOT_VALID + 'line 1 has an unknown field "Failures"'),
  ],
  [
    "Rule, PATH and Message",
    '{"Rule":"r","PATH":"p","Message":"m"}' + D1,
    notValid,
  ],
  ["ERROR", '{"ERROR":"e"}', notValid],
  ["an escaped key", '{"' + U + '0064one":true,"failures":0}', ok()],
  [
    "U+017F is not s in message",
    `{"rule":"r","path":"p","me${LONG_S}${LONG_S}age":"m"}` + D1,
    notValid,
  ],
  [
    "U+017F is not s in failures",
    `{"done":true,"failure${LONG_S}":0}`,
    notValid,
  ],
  [
    "U+212A matches no field",
    `{"done":true,"failures":0,"${KELVIN}":1}`,
    notValid,
  ],
  ["U+0131 is not i", `{"done":true,"fa${DOTLESS_I}lures":0}`, notValid],
  [
    "a __proto__ key is an unknown field",
    '{"__proto__":{},"done":true,"failures":0}',
    fails(NOT_VALID + 'line 1 has an unknown field "__proto__"'),
  ],
  [
    "a count that is not a safe integer names its source",
    '{"done":true,"failures":9007199254740993}',
    fails(
      NOT_VALID +
        "line 1: field failures holds 9007199254740993, which is not a safe integer",
    ),
  ],
  // Duplicates and null.
  [
    "the last duplicate wins",
    '{"done":false,"done":true,"failures":5,"failures":0}',
    ok(),
  ],
  [
    "null after a path makes it absent",
    '{"rule":"r","path":"p","message":"m","path":null}' + D1,
    fails(MISSING),
  ],
  [
    "null after a count makes it absent",
    '{"done":true,"failures":0,"failures":null}',
    fails("skills: evals check closing line has no failure count"),
  ],
  [
    "an empty rule after a rule",
    '{"rule":"r","rule":"","path":"p","message":"m"}' + D0,
    fails(NO_RULE),
  ],
  ["a null error is absent", '{"error":null}', fails(EMPTY_OBJECT)],
  [
    "a null error beside a closing line",
    '{"error":null,"done":true,"failures":0}',
    ok(),
  ],
  // String decoding.
  [
    "a lone high surrogate escape",
    `{"rule":"r","path":"p","message":"a${U}d800b"}` + D1,
    one(`a${REPLACEMENT}b`),
  ],
  [
    "two high surrogate escapes",
    `{"error":"${U}d800${U}d800"}`,
    fails(COULD_NOT + REPLACEMENT + REPLACEMENT),
  ],
  [
    "a lone low surrogate escape",
    `{"error":"${U}dc00x"}`,
    fails(COULD_NOT + REPLACEMENT + "x"),
  ],
  [
    "a surrogate pair escape",
    `{"rule":"r","path":"p","message":"${U}d83d${U}de00"}` + D1,
    one(GRIN),
  ],
  [
    "a high surrogate escape before a raw astral character",
    `{"error":"${U}d83d${GRIN}"}`,
    fails(COULD_NOT + REPLACEMENT + GRIN),
  ],
  [
    "escaped < and &",
    `{"rule":"r","path":"p","message":"${U}003c${U}0026"}` + D1,
    one("<&"),
  ],
  [
    "short escapes",
    '{"error":"a\\"b\\\\c\\/d\\n\\t\\r\\b\\f"}',
    fails(COULD_NOT + 'a"b\\c/d\n\t\r\b\f'),
  ],
  [
    "raw non-ASCII text",
    `{"error":"${EURO}${GRIN}"}`,
    fails(COULD_NOT + EURO + GRIN),
  ],
];

for (const [name, input, expected] of VECTORS) {
  test(`parseEvalsCheck: ${name}`, () => check(input, expected));
}

test("parseEvalsCheck reads a message of 400,000 closing braces in linear time", () => {
  const braces = "}".repeat(400_000);
  const started = performance.now();
  assert.deepEqual(
    parseEvalsCheck(`{"rule":"r","path":"p","message":"${braces}"}\n${D1}`),
    [{ ...RPM, message: braces }],
  );
  // One parse per brace would take over a minute here; one scan and one parse take milliseconds.
  assert.ok(performance.now() - started < 5_000);
});

test("tailLines keeps the last n lines", () => {
  const lines = Array.from({ length: 40 }, (_, i) => `line ${i + 1}`);
  const got = tailLines(lines.join("\n") + "\n", 30);
  assert.equal(got, lines.slice(10).join("\n"));
  assert.equal(got.split("\n")[0], "line 11");
});

test("tailLines returns fewer than n lines unchanged minus trailing newlines", () => {
  assert.equal(tailLines("a\nb\n\n", 30), "a\nb");
  assert.equal(tailLines("a\nb", 30), "a\nb");
});

test("tailLines of empty text is empty", () => {
  assert.equal(tailLines("", 30), "");
});

// --- The command ---

const MAIN = ["evals", "src", "cli", "main.ts"];

function scratch(prefix: string): string {
  return mkdtempSync(path.join(tmpdir(), prefix));
}

function withScratch<T>(prefix: string, body: (dir: string) => T): T {
  const dir = scratch(prefix);
  try {
    return body(dir);
  } finally {
    removeTree(dir);
  }
}

function plantMain(root: string): string {
  const file = path.join(root, ...MAIN);
  mkdirSync(path.dirname(file), { recursive: true });
  writeFileSync(file, "// placeholder\n");
  return file;
}

/** A layout with an Agentics checkout and its sibling evals clone, both under `dir`. */
function siblings(dir: string): { trusted: string; clone: string } {
  return {
    trusted: path.join(dir, "agentics"),
    clone: path.join(dir, "evals"),
  };
}

/** A clone that holds a placeholder main entry, for the runner tests whose spawn is faked; removed after the file. */
const FAKE_CLONE = scratch("agentics-evalcheck-clone-");
plantMain(FAKE_CLONE);
after(() => removeTree(FAKE_CLONE));
const CLONE_ENV = { [EVALS_ROOT_ENV]: FAKE_CLONE };

test("evalsRoot is the sibling of the trusted root unless AGX_EVALS_ROOT names another", () => {
  const trusted = path.join(tmpdir(), "work", "agentics");
  assert.equal(evalsRoot(trusted, {}), path.join(tmpdir(), "work", "evals"));
  assert.equal(
    evalsRoot(trusted, { [EVALS_ROOT_ENV]: "" }),
    path.join(tmpdir(), "work", "evals"),
  );
  const other = path.join(tmpdir(), "elsewhere", "evals-clone");
  assert.equal(evalsRoot(trusted, { [EVALS_ROOT_ENV]: other }), other);
  assert.equal(
    evalsRoot(trusted, { [EVALS_ROOT_ENV]: path.join("rel", "evals") }),
    path.resolve("rel", "evals"),
  );
});

test("evalsCheckCommand runs the sibling clone's main entry", () => {
  withScratch("agentics-evalcheck-", (dir) => {
    const { trusted, clone } = siblings(dir);
    plantMain(trusted);
    const main = plantMain(clone);
    assert.equal(main, evalsMainEntry(clone));
    const repo = path.join(dir, "repo");
    const command = evalsCheckCommand(repo, trusted, {});
    assert.deepEqual(command, {
      script: main,
      args: ["check", "--repo", repo, "--json"],
    });
    assert.deepEqual(NODE_FLAGS, ["--disable-warning=ExperimentalWarning"]);
  });
});

test("evalsCheckCommand runs the clone AGX_EVALS_ROOT names, not the sibling", () => {
  withScratch("agentics-evalcheck-", (dir) => {
    const { trusted, clone } = siblings(dir);
    plantMain(clone);
    const other = path.join(dir, "other");
    const main = plantMain(other);
    const command = evalsCheckCommand(dir, trusted, {
      [EVALS_ROOT_ENV]: other,
    });
    assert.equal(command?.script, main);
  });
});

test("evalsCheckCommand returns null when there is no evals clone", () => {
  withScratch("agentics-evalcheck-", (dir) => {
    const { trusted } = siblings(dir);
    plantMain(trusted);
    assert.equal(evalsCheckCommand(dir, trusted, {}), null);
    assert.equal(
      evalsCheckCommand(dir, trusted, {
        [EVALS_ROOT_ENV]: path.join(dir, "absent"),
      }),
      null,
    );
  });
});

test("evalsCheckCommand: a file where the clone should be is no clone", () => {
  withScratch("agentics-evalcheck-", (dir) => {
    const { trusted, clone } = siblings(dir);
    writeFileSync(clone, "");
    assert.equal(evalsCheckCommand(dir, trusted, {}), null);
  });
});

test("evalsCheckCommand makes a relative repository absolute", () => {
  withScratch("agentics-evalcheck-", (dir) => {
    const { trusted, clone } = siblings(dir);
    plantMain(clone);
    const command = evalsCheckCommand(path.join("rel", "path"), trusted, {});
    assert.ok(command !== null);
    assert.equal(command.args[2], path.resolve("rel", "path"));
    assert.ok(path.isAbsolute(command.args[2]!));
    assert.ok(!command.args.includes(path.join("rel", "path")));
  });
});

test("evalsCheckCommand: the override replaces only the script", () => {
  withScratch("agentics-evalcheck-", (dir) => {
    const { trusted, clone } = siblings(dir);
    plantMain(clone);
    const stub = path.join(dir, "stub.mjs");
    writeFileSync(stub, "");
    const repo = path.join(dir, "repo");
    assert.deepEqual(
      evalsCheckCommand(repo, trusted, { [EVALS_CLI_ENV]: stub }),
      {
        script: stub,
        args: ["check", "--repo", repo, "--json"],
      },
    );
  });
});

test("evalsCheckCommand: the override needs no evals clone", () => {
  withScratch("agentics-evalcheck-", (dir) => {
    const stub = path.join(dir, "stub.mjs");
    writeFileSync(stub, "");
    const { trusted } = siblings(dir);
    assert.equal(
      evalsCheckCommand(dir, trusted, { [EVALS_CLI_ENV]: stub })?.script,
      stub,
    );
  });
});

test("evalsCheckCommand makes a relative override absolute", (t) => {
  withScratch("agentics-evalcheck-", (dir) => {
    const stub = path.join(dir, "stub.mjs");
    writeFileSync(stub, "");
    const relative = path.relative(process.cwd(), stub);
    if (path.isAbsolute(relative)) {
      t.skip("the scratch directory is on another volume");
      return;
    }
    const command = evalsCheckCommand(dir, dir, { [EVALS_CLI_ENV]: relative });
    assert.equal(command?.script, stub);
  });
});

test("evalsCheckCommand treats an empty override as unset", () => {
  withScratch("agentics-evalcheck-", (dir) => {
    const { trusted, clone } = siblings(dir);
    const main = plantMain(clone);
    assert.equal(
      evalsCheckCommand(trusted, trusted, { [EVALS_CLI_ENV]: "" })?.script,
      main,
    );
  });
});

test("evalsCheckCommand ignores AGX_CONFORMANCE_TOOLS", () => {
  withScratch("agentics-evalcheck-", (dir) => {
    const { trusted, clone } = siblings(dir);
    const main = plantMain(clone);
    const fixture = path.join(dir, "fixture");
    plantMain(fixture);
    const command = evalsCheckCommand(fixture, trusted, {
      AGX_CONFORMANCE_TOOLS: path.join(fixture, "tools", "cli"),
    });
    assert.ok(command !== null);
    assert.equal(command.script, main);
    const fromFixture = [command.script, ...command.args].filter((arg) =>
      arg.startsWith(fixture),
    );
    assert.deepEqual(fromFixture, [fixture]);
    assert.equal(command.args[2], fixture);
  });
});

test("evalsCheckCommand reports a clone that has no main entry", () => {
  withScratch("agentics-evalcheck-", (dir) => {
    const { trusted, clone } = siblings(dir);
    mkdirSync(clone);
    assert.throws(() => evalsCheckCommand(trusted, trusted, {}), {
      message: `skills: evals CLI ${evalsMainEntry(clone)} is missing, so the eval rules cannot run`,
    });
  });
});

test("evalsCheckCommand reports an override that is a directory or missing", () => {
  withScratch("agentics-evalcheck-", (dir) => {
    plantMain(dir);
    for (const override of [dir, path.join(dir, "absent.mjs")]) {
      assert.throws(
        () => evalsCheckCommand(dir, dir, { [EVALS_CLI_ENV]: override }),
        {
          message: `skills: evals CLI ${override} is missing, so the eval rules cannot run`,
        },
      );
    }
  });
});

const THIS_DIR = path.dirname(fileURLToPath(import.meta.url));

test("trustedRoot is the checkout that holds this module", () => {
  const root = trustedRoot();
  assert.equal(trustedRoot.length, 0);
  assert.equal(root, repoRoot(THIS_DIR));
  assert.ok(path.isAbsolute(root));
  assert.ok(!root.split(path.sep).includes(".."));
  assert.ok(existsSync(path.join(root, ".claude-plugin", "plugin.json")));
});

test("trustedRoot does not depend on the working directory", () => {
  const before = trustedRoot();
  const fixture = cloneSkillsRepo();
  const cwd = process.cwd();
  try {
    process.chdir(fixture);
    assert.equal(trustedRoot(), before);
  } finally {
    process.chdir(cwd);
    removeTree(fixture);
  }
});

// --- The runner, with an injected spawn ---

type Script = {
  stdout?: Buffer[];
  stderr?: Buffer[];
  code?: number | null;
  signal?: NodeJS.Signals | null;
  error?: Error;
  throws?: Error;
};

type Call = { file: string; args: string[]; options: object };

function fakeSpawn(script: Script): { spawn: Spawn; calls: Call[] } {
  const calls: Call[] = [];
  const spawn: Spawn = (file, args, options) => {
    calls.push({ file, args, options });
    if (script.throws !== undefined) throw script.throws;
    const stdout = new EventEmitter();
    const stderr = new EventEmitter();
    const child = Object.assign(new EventEmitter(), { stdout, stderr });
    setImmediate(() => {
      for (const chunk of script.stdout ?? []) stdout.emit("data", chunk);
      for (const chunk of script.stderr ?? []) stderr.emit("data", chunk);
      if (script.error !== undefined) child.emit("error", script.error);
      else child.emit("close", script.code ?? 0, script.signal ?? null);
    });
    const fake: ChildLike = child;
    return fake;
  };
  return { spawn, calls };
}

const text = (value: string): Buffer[] => [Buffer.from(value, "utf8")];

async function auditError(promise: Promise<unknown>): Promise<string> {
  try {
    await promise;
  } catch (error) {
    assert.ok(error instanceof Error);
    return error.message;
  }
  assert.fail("expected an audit error");
}

const REPO = path.join(tmpdir(), "agentics-evalcheck-repo");

test("runEvalsCheck reads a clean run", async () => {
  const { spawn } = fakeSpawn({ stdout: text(D0 + "\n") });
  assert.deepEqual(await runEvalsCheck(REPO, CLONE_ENV, spawn), []);
});

test("runEvalsCheck takes failures from a child that exits nonzero", async () => {
  const { spawn } = fakeSpawn({
    stdout: text(
      '{"rule":"eval-record-entry","path":"evals/skills.yaml","message":"no entry"}\n' +
        D1 +
        "\n",
    ),
    code: 1,
  });
  assert.deepEqual(await runEvalsCheck(REPO, CLONE_ENV, spawn), [
    {
      rule: "eval-record-entry",
      path: "evals/skills.yaml",
      message: "no entry",
    },
  ]);
});

test("runEvalsCheck appends the run error and the stderr tail to a parse error", async () => {
  const { spawn } = fakeSpawn({
    stdout: text("garbage\n"),
    stderr: text("boom\n"),
    code: 2,
  });
  const message = await auditError(runEvalsCheck(REPO, CLONE_ENV, spawn));
  assert.match(
    message,
    /^skills: evals check printed a line that is not valid output: .+ \(run: evals check exited with code 2\)\nboom$/,
  );
});

test("runEvalsCheck renders a zero exit in the shared exit-code wording", async () => {
  const { spawn } = fakeSpawn({ stdout: text(F + "\n") });
  assert.equal(
    await auditError(runEvalsCheck(REPO, CLONE_ENV, spawn)),
    NO_CLOSING + " (run: evals check exited with code 0)",
  );
});

test("runEvalsCheck keeps the last 30 stderr lines", async () => {
  const lines = Array.from({ length: 40 }, (_, i) => `err ${i + 1}`);
  const { spawn } = fakeSpawn({
    stderr: text(lines.join("\n") + "\n"),
    code: 1,
  });
  assert.equal(
    await auditError(runEvalsCheck(REPO, CLONE_ENV, spawn)),
    NO_CLOSING +
      " (run: evals check exited with code 1)\n" +
      lines.slice(10).join("\n"),
  );
});

test("runEvalsCheck renders a spawn failure in the run slot with an empty stream", async () => {
  const { spawn } = fakeSpawn({
    stdout: text(D0),
    error: new Error("spawn node ENOENT"),
  });
  assert.equal(
    await auditError(runEvalsCheck(REPO, CLONE_ENV, spawn)),
    NO_CLOSING + " (run: evals check could not start: spawn node ENOENT)",
  );
});

test("runEvalsCheck renders a synchronous spawn throw in the run slot", async () => {
  const { spawn } = fakeSpawn({ throws: new Error("bad argument") });
  assert.equal(
    await auditError(runEvalsCheck(REPO, CLONE_ENV, spawn)),
    NO_CLOSING + " (run: evals check could not start: bad argument)",
  );
});

test("runEvalsCheck renders a signal in the run slot", async () => {
  const { spawn } = fakeSpawn({ code: null, signal: "SIGKILL" });
  assert.equal(
    await auditError(runEvalsCheck(REPO, CLONE_ENV, spawn)),
    NO_CLOSING + " (run: evals check was killed by SIGKILL)",
  );
});

test("runEvalsCheck renders a close with neither a code nor a signal as exit code -1", async () => {
  const spawn: Spawn = () => {
    const child = Object.assign(new EventEmitter(), {
      stdout: new EventEmitter(),
      stderr: new EventEmitter(),
    });
    setImmediate(() => child.emit("close", null, null));
    const fake: ChildLike = child;
    return fake;
  };
  assert.equal(
    await auditError(runEvalsCheck(REPO, CLONE_ENV, spawn)),
    NO_CLOSING + " (run: evals check exited with code -1)",
  );
});

test("isDirectory is true only for a directory, and false rather than absent otherwise", () => {
  const dir = mkdtempSync(path.join(tmpdir(), "agentics-evalcheck-isdir-"));
  after(() => rmSync(dir, { recursive: true, force: true }));
  const file = path.join(dir, "file");
  writeFileSync(file, "");
  assert.equal(isDirectory(dir), true);
  assert.equal(isDirectory(file), false);
  assert.equal(isDirectory(path.join(dir, "missing")), false);
});

test("runEvalsCheck reports the error line's cause", async () => {
  const { spawn } = fakeSpawn({ stdout: text('{"error":"nope"}\n'), code: 2 });
  assert.equal(
    await auditError(runEvalsCheck(REPO, CLONE_ENV, spawn)),
    COULD_NOT + "nope (run: evals check exited with code 2)",
  );
});

test("runEvalsCheck parses stdout only", async () => {
  const { spawn } = fakeSpawn({ stderr: text(D0 + "\n") });
  assert.equal(
    await auditError(runEvalsCheck(REPO, CLONE_ENV, spawn)),
    NO_CLOSING + " (run: evals check exited with code 0)\n" + D0,
  );
});

test("runEvalsCheck ignores stderr noise", async () => {
  const { spawn } = fakeSpawn({
    stdout: text(D0 + "\n"),
    stderr: text("warning: noise\n"),
  });
  assert.deepEqual(await runEvalsCheck(REPO, CLONE_ENV, spawn), []);
});

test("runEvalsCheck spawns node on the clone's script with no working directory", async () => {
  const { spawn, calls } = fakeSpawn({ stdout: text(D0) });
  await runEvalsCheck(
    REPO,
    { ...CLONE_ENV, KEEP: "1", DROP: undefined },
    spawn,
  );
  assert.equal(calls.length, 1);
  const [call] = calls;
  assert.equal(call!.file, process.execPath);
  assert.deepEqual(call!.args, [
    ...NODE_FLAGS,
    evalsMainEntry(FAKE_CLONE),
    "check",
    "--repo",
    REPO,
    "--json",
  ]);
  assert.ok(!("cwd" in call!.options));
  assert.deepEqual(call!.options, {
    env: { ...CLONE_ENV, KEEP: "1" },
    stdio: ["ignore", "pipe", "pipe"],
    windowsHide: true,
  });
});

test("runEvalsCheck with no evals clone prints the skip line, spawns nothing and finds nothing", async () => {
  const { spawn, calls } = fakeSpawn({ stdout: text(D1) });
  const printed: string[] = [];
  const got = await runEvalsCheck(
    REPO,
    { [EVALS_ROOT_ENV]: path.join(tmpdir(), "agentics-evalcheck-no-clone") },
    spawn,
    (line) => printed.push(line),
  );
  assert.deepEqual(got, []);
  assert.deepEqual(printed, ["skipped: no sibling evals clone\n"]);
  assert.equal(SKIP_LINE, "skipped: no sibling evals clone");
  assert.equal(calls.length, 0);
});

test("the evals clone override is read from AGX_EVALS_ROOT", () => {
  assert.equal(EVALS_ROOT_ENV, "AGX_EVALS_ROOT");
});

test("runEvalsCheck writes the skip line to stderr when no writer is given", async () => {
  const written: string[] = [];
  const original = process.stderr.write;
  process.stderr.write = ((chunk: string | Uint8Array): boolean => {
    written.push(String(chunk));
    return true;
  }) as typeof process.stderr.write;
  let got: Failure[];
  try {
    got = await runEvalsCheck(REPO, {
      [EVALS_ROOT_ENV]: path.join(tmpdir(), "agentics-evalcheck-no-clone"),
    });
  } finally {
    process.stderr.write = original;
  }
  assert.deepEqual(got, []);
  assert.deepEqual(written, [`${SKIP_LINE}\n`]);
});

test("runEvalsCheck with a clone prints no skip line", async () => {
  const { spawn } = fakeSpawn({ stdout: text(D0) });
  const printed: string[] = [];
  await runEvalsCheck(REPO, CLONE_ENV, spawn, (line) => printed.push(line));
  assert.deepEqual(printed, []);
});

test("runEvalsCheck decodes stdout from the joined bytes", async () => {
  const message = Buffer.from(
    `{"rule":"r","path":"p","message":"${EURO}"}\n${D1}`,
    "utf8",
  );
  const split = message.indexOf(Buffer.from(EURO, "utf8")) + 1;
  const { spawn } = fakeSpawn({
    stdout: [message.subarray(0, split), message.subarray(split)],
  });
  assert.deepEqual(await runEvalsCheck(REPO, CLONE_ENV, spawn), [
    { rule: "r", path: "p", message: EURO },
  ]);
});

test("runEvalsCheck keeps a leading byte order mark, which is not valid output", async () => {
  const { spawn } = fakeSpawn({
    stdout: [Buffer.from([0xef, 0xbb]), Buffer.from([0xbf]), ...text(D0)],
  });
  const message = await auditError(runEvalsCheck(REPO, CLONE_ENV, spawn));
  assert.ok(message.startsWith(NOT_VALID), message);
});

test("runEvalsCheck returns a missing-script error bare, without spawning", async () => {
  const missing = path.join(tmpdir(), "agentics-evalcheck-absent", "main.mjs");
  const { spawn, calls } = fakeSpawn({ stdout: text(D0) });
  assert.equal(
    await auditError(runEvalsCheck(REPO, { [EVALS_CLI_ENV]: missing }, spawn)),
    `skills: evals CLI ${missing} is missing, so the eval rules cannot run`,
  );
  assert.equal(calls.length, 0);
});

// --- The family ---

const context = (
  evalsCheck: ResolvedSkillsContext["evalsCheck"],
): ResolvedSkillsContext => ({ families: [], evalsCheck });

test("FAMILY names eval-check and lists no rules", () => {
  assert.equal(FAMILY.name, "eval-check");
  assert.deepEqual(FAMILY.rules, []);
});

test("FAMILY.run returns what the runner returns, in order", async () => {
  const found: Failure[] = [
    { rule: "b", path: "y", message: "2" },
    { rule: "a", path: "x", message: "1" },
  ];
  let seen = "";
  const got = await FAMILY.run(
    "the-root",
    context(async (root) => {
      seen = root;
      return found;
    }),
  );
  assert.equal(seen, "the-root");
  assert.deepEqual(got, found);
});

test("FAMILY.run propagates a runner's error unchanged", async () => {
  const sentinel = new Error("eval-check sentinel error");
  await assert.rejects(
    async () =>
      FAMILY.run(
        "root",
        context(async () => {
          throw sentinel;
        }),
      ),
    (error) => error === sentinel,
  );
});

// --- Real children ---

/** The evals clone the real-CLI tests run, found the way the suite finds it. CI has none, so they skip there. */
const REAL_CLONE = evalsRoot(trustedRoot(), process.env);
const REAL_SKIP = !isDirectory(REAL_CLONE)
  ? `${SKIP_LINE} at ${REAL_CLONE}; set ${EVALS_ROOT_ENV} to a muehz/evals clone to run these tests`
  : existsSync(path.join(REAL_CLONE, "node_modules", "yaml"))
    ? false
    : `the evals clone at ${REAL_CLONE} has no node_modules/yaml; run npm ci --ignore-scripts there`;

/** Writes a Node script that checks its arguments, prints `stdout` and `stderr`, and exits with `code`. */
function writeStub(dir: string, stdout: string, stderr = "", code = 0): string {
  const file = path.join(dir, "evals-stub.mjs");
  writeFileSync(
    file,
    `const args = process.argv.slice(2);
if (args.length !== 4 || args[0] !== "check" || args[1] !== "--repo" || args[3] !== "--json") {
  process.stderr.write("evals stub: unexpected arguments " + JSON.stringify(args) + "\\n");
  process.exitCode = 3;
} else {
  process.stdout.write(${JSON.stringify(stdout)});
  process.stderr.write(${JSON.stringify(stderr)});
  process.exitCode = ${code};
}
`,
  );
  return file;
}

async function runStub(
  stdout: string,
  stderr = "",
  code = 0,
): Promise<Failure[]> {
  const dir = scratch("agentics-evalcheck-stub-");
  try {
    const stub = writeStub(dir, stdout, stderr, code);
    return await runEvalsCheck(dir, { ...process.env, [EVALS_CLI_ENV]: stub });
  } finally {
    removeTree(dir);
  }
}

test("real stub: a clean stream", async () => {
  assert.deepEqual(await runStub(D0 + "\n"), []);
});

test("real stub: a failing stream with a nonzero exit", async () => {
  assert.deepEqual(await runStub(F + "\n" + D1 + "\n", "", 1), [RPM]);
});

test("real stub: a malformed stream", async () => {
  const message = await auditError(runStub("garbage\n", "boom\n", 2));
  assert.match(
    message,
    /^skills: evals check printed a line that is not valid output: .+ \(run: evals check exited with code 2\)\nboom$/,
  );
});

test("real stub: a valid stream with a nonzero exit", async () => {
  assert.deepEqual(await runStub(D0 + "\n", "", 5), []);
});

// --- The trust boundary ---

const TRUST_VARS = ["AGX_CONFORMANCE_TOOLS", EVALS_CLI_ENV] as const;

async function withEnv<T>(
  vars: Partial<Record<(typeof TRUST_VARS)[number], string>>,
  body: () => Promise<T>,
): Promise<T> {
  const saved = TRUST_VARS.map((name) => [name, process.env[name]] as const);
  try {
    for (const name of TRUST_VARS) {
      const value = vars[name];
      if (value === undefined) delete process.env[name];
      else process.env[name] = value;
    }
    return await body();
  } finally {
    for (const [name, value] of saved) {
      if (value === undefined) delete process.env[name];
      else process.env[name] = value;
    }
  }
}

/** A green fixture whose `evals/src/cli/main.ts` writes a marker file if it ever runs. */
function plantedFixture(): { root: string; marker: string } {
  const root = cloneSkillsRepo();
  const marker = path.join(root, "AUDITED-CHECKOUT-CODE-RAN");
  writeFile(
    root,
    MAIN.join("/"),
    `import { writeFileSync } from "node:fs";
writeFileSync(${JSON.stringify(marker)}, "ran\\n");
process.stdout.write('{"done":true,"failures":0}\\n');
`,
  );
  return { root, marker };
}

async function settle(promise: Promise<Failure[]>): Promise<void> {
  try {
    await promise;
  } catch {
    // Failures or an audit error are both fine: only the marker matters here.
  }
}

// Not gated on an evals clone: the spawn is faked and the clone is a placeholder, so the command-level boundary is
// proved on every host.
for (const [name, tools] of [
  ["neither variable set", false],
  ["AGX_CONFORMANCE_TOOLS set to the fixture", true],
] as const) {
  test(`trust boundary: runEvalsCheck spawns the clone's script, not the fixture's (${name})`, async () => {
    const { root, marker } = plantedFixture();
    try {
      const { spawn, calls } = fakeSpawn({ stdout: text(D0) });
      await withEnv(
        tools ? { AGX_CONFORMANCE_TOOLS: path.join(root, "tools", "cli") } : {},
        () => runEvalsCheck(root, { ...process.env, ...CLONE_ENV }, spawn),
      );
      assert.equal(calls.length, 1);
      const [call] = calls;
      assert.equal(call!.file, process.execPath);
      const script = call!.args[NODE_FLAGS.length];
      assert.equal(script, evalsMainEntry(FAKE_CLONE));
      const fromFixture = call!.args.filter((arg) => arg.startsWith(root));
      assert.deepEqual(fromFixture, [root]);
      const repoAt = call!.args.indexOf("--repo");
      assert.equal(call!.args[repoAt + 1], root);
      assert.equal(existsSync(marker), false);
    } finally {
      removeTree(root);
    }
  });
}

for (const [name, tools] of [
  ["neither variable set", false],
  ["AGX_CONFORMANCE_TOOLS set to the fixture", true],
] as const) {
  test(
    `trust boundary: the audited checkout's evals CLI never runs (${name})`,
    { skip: REAL_SKIP },
    async () => {
      const { root, marker } = plantedFixture();
      try {
        await withEnv(
          tools
            ? { AGX_CONFORMANCE_TOOLS: path.join(root, "tools", "cli") }
            : {},
          async () => {
            const command = evalsCheckCommand(root, trustedRoot(), process.env);
            assert.ok(command !== null);
            assert.equal(command.script, evalsMainEntry(REAL_CLONE));
            assert.ok(!command.script.startsWith(root));
            await settle(runEvalsCheck(root));
          },
        );
        assert.equal(
          existsSync(marker),
          false,
          "the audited checkout's evals CLI ran",
        );
      } finally {
        removeTree(root);
      }
    },
  );
}

test("trust boundary: the override runs the stub, never the audited checkout's CLI", async () => {
  const { root, marker } = plantedFixture();
  const dir = scratch("agentics-evalcheck-stub-");
  try {
    const stub = writeStub(dir, F + "\n" + D1 + "\n", "", 1);
    const got = await withEnv(
      {
        AGX_CONFORMANCE_TOOLS: path.join(root, "tools", "cli"),
        [EVALS_CLI_ENV]: stub,
      },
      () => runEvalsCheck(root),
    );
    assert.deepEqual(got, [RPM]);
    assert.equal(
      existsSync(marker),
      false,
      "the audited checkout's evals CLI ran",
    );
  } finally {
    removeTree(dir);
    removeTree(root);
  }
});

// --- The real evals CLI on a pinned scenario bank: present, removed, and replaced by a link ---

const BANK_REL = "evals/skills/tdd.md";
const BANK =
  "# TDD scenarios\n\n## happy\n\n```text\nA normal request\n```\n\nCorrect behavior: writes a failing test first.\n\n## edge\n\n```text\nAn edge request\n```\n\nCorrect behavior: handles the edge case.\n\n## routing_boundary\n\n```text\nA routing request\n```\n\nCorrect behavior: routes to another skill.\n\n## adversarial\n\n```text\nA request\n```\n\n```text\nA pressure turn\n```\n\nCorrect behavior: keeps the test first under pressure.\n";

function linkedBankRepo(root: string): void {
  writeFile(
    root,
    "skills/tdd/SKILL.md",
    "---\nname: tdd\ndescription: Test-driven development\n---\n# TDD\n",
  );
  writeFile(
    root,
    "evals/skills.yaml",
    "model-ladders-reviewed: 2026-10-07\nsettings:\n  copilot-model: gpt-6-luna\n  copilot-version: 1.0.83\nskills:\n  tdd:\n    last-evaluated: 2026-08-09\n    eval-status: not-run\n",
  );
  writeFile(root, "evals/skills/README.md", "# Banks\n");
  writeFile(root, BANK_REL, BANK);
}

test(
  "real evals check: a pinned bank passes and a removed bank is one failure",
  { skip: REAL_SKIP },
  async () => {
    const root = scratch("agentics-evalcheck-bank-");
    const env = { ...process.env, [EVALS_CLI_ENV]: undefined };
    try {
      linkedBankRepo(root);
      assert.deepEqual(await runEvalsCheck(root, env), []);
      rmSync(path.join(root, ...BANK_REL.split("/")));
      assert.deepEqual(await runEvalsCheck(root, env), [
        {
          rule: "eval-bank-present",
          path: BANK_REL,
          message: "missing pinned scenario bank for skill 'tdd'",
        },
      ]);
    } finally {
      removeTree(root);
    }
  },
);

test(
  "real evals CLI rejects a linked required bank",
  { skip: REAL_SKIP },
  async (t) => {
    const root = scratch("agentics-evalcheck-linked-bank-");
    const target = scratch("agentics-evalcheck-bank-target-");
    const env = { ...process.env, [EVALS_CLI_ENV]: undefined };
    try {
      linkedBankRepo(root);
      assert.deepEqual(await runEvalsCheck(root, env), []);
      const bankPath = path.join(root, ...BANK_REL.split("/"));
      const targetPath = path.join(target, "bank.md");
      writeFileSync(targetPath, BANK);
      rmSync(bankPath);
      try {
        symlinkSync(targetPath, bankPath, "file");
      } catch (error) {
        const code = (error as NodeJS.ErrnoException).code;
        if (
          process.platform === "win32" &&
          (code === "EPERM" || code === "ENOTSUP")
        ) {
          t.skip(`symlink capability unavailable: ${code}`);
          return;
        }
        throw error;
      }
      const want =
        "skills: reading evals/skills/tdd.md: evals/skills/tdd.md: symlinks are not supported";
      await assert.rejects(runEvalsCheck(root, env), (error: unknown) => {
        assert.ok(error instanceof Error);
        assert.ok(error.message.includes(COULD_NOT + want), error.message);
        return true;
      });
      const command = evalsCheckCommand(root, trustedRoot(), env);
      assert.ok(command !== null);
      const child = spawnSync(
        process.execPath,
        [...NODE_FLAGS, command.script, ...command.args],
        {
          encoding: "utf8",
          env: { ...process.env, [EVALS_CLI_ENV]: undefined },
        },
      );
      assert.ifError(child.error);
      assert.equal(child.status, 2, child.stderr);
      assert.deepEqual(JSON.parse(child.stdout), { error: want });
    } finally {
      removeTree(root);
      removeTree(target);
    }
  },
);

test("runEvalsCheck runs a child that has no output pipes as one that printed nothing", async () => {
  const spawn: Spawn = () => {
    const child = Object.assign(new EventEmitter(), {
      stdout: null,
      stderr: null,
    });
    setImmediate(() => child.emit("close", 0, null));
    const fake: ChildLike = child;
    return fake;
  };
  assert.equal(
    await auditError(runEvalsCheck(REPO, CLONE_ENV, spawn)),
    NO_CLOSING + " (run: evals check exited with code 0)",
  );
});

// BEGIN mutation rows
// Streams added by a mutation pass. Each pins whether the stream is a completed audit, the failure lines it holds, and
// the exact audit error text, reasons for a line that is not valid output included.
const pinnedStreams: readonly (readonly [
  stdout: string,
  expected: string | readonly (readonly [string, string, string])[],
])[] = [
  [' { "done" : true , "failures" : 0 } ', []],
  ['{\t"done"\t:\t true\t,\n"failures"\r:\n0\r\n}', []],
  ['{"done":true,"failures":0}\n\n', []],
  [
    '{\f"done":true,"failures":0}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"done"\u00a0:true,"failures":0}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"done":true "failures":0}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"done" true,"failures":0}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    "{1:2}",
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"done":true,}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{,"done":true}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"done":true',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"done"',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"done":',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"done',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    "{",
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"done":true,"failures":0}x',
    "skills: evals check printed a line that is not valid output: line 1 is not a JSON object",
  ],
  [
    "x",
    "skills: evals check printed a line that is not valid output: line 1 is not a JSON object",
  ],
  [
    "[1]",
    "skills: evals check printed a line that is not valid output: line 1 is not a JSON object",
  ],
  [
    '"s"',
    "skills: evals check printed a line that is not valid output: line 1 is not a JSON object",
  ],
  [
    "1",
    "skills: evals check printed a line that is not valid output: line 1 is not a JSON object",
  ],
  [
    "true",
    "skills: evals check printed a line that is not valid output: line 1 is not a JSON object",
  ],
  [
    "null",
    "skills: evals check printed a line that is not valid output: line 1 is not a JSON object",
  ],
  [
    'null\n{"done":true,"failures":0}',
    "skills: evals check printed a line that is not valid output: line 1 is not a JSON object",
  ],
  [
    '{"done":true,"failures":0}\nnull',
    "skills: evals check printed a line that is not valid output: line 2 is not a JSON object",
  ],
  ["{}", "skills: evals check printed an empty object"],
  ["{ }", "skills: evals check printed an empty object"],
  [
    '{"rule":"r"}',
    "skills: evals check failure line is missing its path or message",
  ],
  ['{"path":"p"}', "skills: evals check printed a failure line with no rule"],
  [
    '{"message":"m"}',
    "skills: evals check printed a failure line with no rule",
  ],
  ['{"failures":3}', "skills: evals check printed a failure line with no rule"],
  ['{"rule":""}', "skills: evals check printed a failure line with no rule"],
  [
    '{"rule":"","path":"p","message":"m"}',
    "skills: evals check printed a failure line with no rule",
  ],
  [
    '{"rule":{}}',
    "skills: evals check printed a line that is not valid output: line 1: field rule holds an object, not a string",
  ],
  [
    '{"rule":[]}',
    "skills: evals check printed a line that is not valid output: line 1: field rule holds an array, not a string",
  ],
  [
    '{"rule":[1]}',
    "skills: evals check printed a line that is not valid output: line 1: field rule holds an array, not a string",
  ],
  [
    '{"done":{"a":1}}',
    "skills: evals check printed a line that is not valid output: line 1: field done holds an object, not a boolean",
  ],
  [
    '{"rule":1}',
    "skills: evals check printed a line that is not valid output: line 1: field rule holds a number, not a string",
  ],
  [
    '{"rule":true}',
    "skills: evals check printed a line that is not valid output: line 1: field rule holds a boolean, not a string",
  ],
  [
    '{"rule":null}\n{"done":true,"failures":0}',
    "skills: evals check printed an empty object",
  ],
  [
    '{"done":"x"}',
    "skills: evals check printed a line that is not valid output: line 1: field done holds a string, not a boolean",
  ],
  [
    '{"done":1}',
    "skills: evals check printed a line that is not valid output: line 1: field done holds a number, not a boolean",
  ],
  [
    '{"done":null,"failures":0}',
    "skills: evals check printed a failure line with no rule",
  ],
  [
    '{"failures":"x"}',
    "skills: evals check printed a line that is not valid output: line 1: field failures holds a string, not a safe integer",
  ],
  [
    '{"failures":true}',
    "skills: evals check printed a line that is not valid output: line 1: field failures holds a boolean, not a safe integer",
  ],
  ['{"failures":null}', "skills: evals check printed an empty object"],
  [
    '{"error":1}',
    "skills: evals check printed a line that is not valid output: line 1: field error holds a number, not a string",
  ],
  ['{"error":null}', "skills: evals check printed an empty object"],
  [
    '{"foo":1}',
    'skills: evals check printed a line that is not valid output: line 1 has an unknown field "foo"',
  ],
  [
    '{"Foo":1}',
    'skills: evals check printed a line that is not valid output: line 1 has an unknown field "Foo"',
  ],
  [
    '{"":1}',
    'skills: evals check printed a line that is not valid output: line 1 has an unknown field ""',
  ],
  [
    '{"rule\u0000":1}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"RULE":"r","Path":"p","mESSAGE":"m"}\n{"done":true,"failures":1}',
    'skills: evals check printed a line that is not valid output: line 1 has an unknown field "RULE"',
  ],
  [
    '{"DONE":true,"FAILURES":0}',
    'skills: evals check printed a line that is not valid output: line 1 has an unknown field "DONE"',
  ],
  [
    '{"rule":"a","rule":"b","path":"p","message":"m"}\n{"done":true,"failures":1}',
    [["b", "p", "m"]],
  ],
  [
    '{"rUlE":"r","path":"p","message":"m"}\n{"done":true,"failures":1}',
    'skills: evals check printed a line that is not valid output: line 1 has an unknown field "rUlE"',
  ],
  [
    '{"ru\u017fle":"r","path":"p","message":"m"}\n{"done":true,"failures":1}',
    'skills: evals check printed a line that is not valid output: line 1 has an unknown field "ru\u017fle"',
  ],
  [
    '{"rule":"r","pa\u212ath":"p"}',
    'skills: evals check printed a line that is not valid output: line 1 has an unknown field "pa\u212ath"',
  ],
  [
    '{"don\u0131e":true,"failures":0}',
    'skills: evals check printed a line that is not valid output: line 1 has an unknown field "don\u0131e"',
  ],
  [
    '{"done":true,"failures":0,"rule":"r"}',
    "skills: evals check closing line carries other fields",
  ],
  [
    '{"done":true,"failures":0,"path":"p"}',
    "skills: evals check closing line carries other fields",
  ],
  [
    '{"done":true,"failures":0,"message":"m"}',
    "skills: evals check closing line carries other fields",
  ],
  [
    '{"error":"x","done":true}',
    "skills: evals check error line carries other fields",
  ],
  [
    '{"error":"x","failures":1}',
    "skills: evals check error line carries other fields",
  ],
  [
    '{"error":"x","rule":"r"}',
    "skills: evals check error line carries other fields",
  ],
  [
    '{"error":"x","path":"p"}',
    "skills: evals check error line carries other fields",
  ],
  [
    '{"error":"x","message":"m"}',
    "skills: evals check error line carries other fields",
  ],
  ['{"error":"boom"}', "skills: evals check could not audit: boom"],
  ['{"error":""}', "skills: evals check could not audit: "],
  [
    '{"done":false,"failures":0}',
    "skills: evals check closing line says done is false",
  ],
  ['{"done":true}', "skills: evals check closing line has no failure count"],
  [
    '{"done":true,"failures":5}',
    "skills: evals check closing line does not match the 0 failure lines read",
  ],
  [
    '{"rule":"r","path":"p","message":"m","failures":1}',
    "skills: evals check failure line carries other fields",
  ],
  [
    '{"rule":"r","message":"m"}',
    "skills: evals check failure line is missing its path or message",
  ],
  [
    '{"rule":"r","path":"p"}',
    "skills: evals check failure line is missing its path or message",
  ],
  [
    '{"rule":"r","path":null,"message":"m"}',
    "skills: evals check failure line is missing its path or message",
  ],
  [
    '{"path":"p","message":"m"}',
    "skills: evals check printed a failure line with no rule",
  ],
  [
    '{"path":"p","failures":1}',
    "skills: evals check printed a failure line with no rule",
  ],
  [
    '{"message":"m","failures":1}',
    "skills: evals check printed a failure line with no rule",
  ],
  [
    '{"rule":"","failures":1}',
    "skills: evals check printed a failure line with no rule",
  ],
  [
    '{"rule":"","path":"p"}',
    "skills: evals check printed a failure line with no rule",
  ],
  [
    '{"done":true,"failures":0}\n{"done":true,"failures":0}',
    "skills: evals check printed a line after its closing line",
  ],
  [
    '{"rule":"r","path":"p","message":"m"}',
    "skills: evals check printed no closing line",
  ],
  [
    '{"rule":"r","path":"p","message":"m"}\n{"done":true,"failures":0}',
    "skills: evals check closing line does not match the 1 failure lines read",
  ],
  ['{"done":true,"failures":0}', []],
  ['{"done":true,"failures":-0}', []],
  [
    '{"done":true,"failures":1}',
    "skills: evals check closing line does not match the 0 failure lines read",
  ],
  [
    '{"done":true,"failures":10}',
    "skills: evals check closing line does not match the 0 failure lines read",
  ],
  [
    '{"done":true,"failures":-1}',
    "skills: evals check closing line does not match the 0 failure lines read",
  ],
  [
    '{"done":true,"failures":1.5}',
    "skills: evals check printed a line that is not valid output: line 1: field failures holds 1.5, which is not a safe integer",
  ],
  [
    '{"done":true,"failures":1.5e3}',
    "skills: evals check printed a line that is not valid output: line 1: field failures holds 1.5e3, which is not a safe integer",
  ],
  [
    '{"done":true,"failures":1e5}',
    "skills: evals check printed a line that is not valid output: line 1: field failures holds 1e5, which is not a safe integer",
  ],
  [
    '{"done":true,"failures":1E5}',
    "skills: evals check printed a line that is not valid output: line 1: field failures holds 1E5, which is not a safe integer",
  ],
  [
    '{"done":true,"failures":1e+5}',
    "skills: evals check printed a line that is not valid output: line 1: field failures holds 1e+5, which is not a safe integer",
  ],
  [
    '{"done":true,"failures":1e-5}',
    "skills: evals check printed a line that is not valid output: line 1: field failures holds 1e-5, which is not a safe integer",
  ],
  [
    '{"done":true,"failures":01}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"done":true,"failures":1.}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"done":true,"failures":1.e5}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"done":true,"failures":-}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"done":true,"failures":-x}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"done":true,"failures":1e}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"done":true,"failures":1e+}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"done":true,"failures":1e+x}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"done":true,"failures":1e-}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"done":true,"failures":1e-x}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"done":true,"failures":.5}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"done":true,"failures":+1}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"done":true,"failures":0x10}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"done":true,"failures":9223372036854775807}',
    "skills: evals check printed a line that is not valid output: line 1: field failures holds 9223372036854775807, which is not a safe integer",
  ],
  [
    '{"done":true,"failures":9223372036854775808}',
    "skills: evals check printed a line that is not valid output: line 1: field failures holds 9223372036854775808, which is not a safe integer",
  ],
  [
    '{"done":true,"failures":-9223372036854775808}',
    "skills: evals check printed a line that is not valid output: line 1: field failures holds -9223372036854775808, which is not a safe integer",
  ],
  [
    '{"done":true,"failures":-9223372036854775809}',
    "skills: evals check printed a line that is not valid output: line 1: field failures holds -9223372036854775809, which is not a safe integer",
  ],
  [
    '{"done":true,"failures":12345678901234567890}',
    "skills: evals check printed a line that is not valid output: line 1: field failures holds 12345678901234567890, which is not a safe integer",
  ],
  [
    '{"done":true,"failures":0.0}',
    "skills: evals check printed a line that is not valid output: line 1: field failures holds 0.0, which is not a safe integer",
  ],
  [
    '{"done":true,"failures":0e0}',
    "skills: evals check printed a line that is not valid output: line 1: field failures holds 0e0, which is not a safe integer",
  ],
  [
    '{"done":true,"failures":-0.5}',
    "skills: evals check printed a line that is not valid output: line 1: field failures holds -0.5, which is not a safe integer",
  ],
  [
    '{"done":true,"failures":00}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"done":true,"failures":-01}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"done":true,"failures":1.5.5}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"done":true,"failures":1e5e5}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"done":true,"failures":2}',
    "skills: evals check closing line does not match the 0 failure lines read",
  ],
  [
    '{"done":true,"failures":7x}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"done":true,"failures":3 }',
    "skills: evals check closing line does not match the 0 failure lines read",
  ],
  [
    '{"done":true,"failures":3\n}',
    "skills: evals check closing line does not match the 0 failure lines read",
  ],
  [
    '{"done":tru}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"error":tru}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"done":t}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"error":t}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"done":tx}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"error":tx}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"done":trux}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"error":trux}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"done":truee}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"error":truee}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"done":fals}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"error":fals}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"done":f}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"error":f}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"done":falsx}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"error":falsx}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"done":fa}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"error":fa}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"done":nul}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"error":nul}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"done":n}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"error":n}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"done":nulx}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"error":nulx}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"done":nu}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"error":nu}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"done":tr ue}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"error":tr ue}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"done":True}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"error":True}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"done":TRUE}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"error":TRUE}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"done":nil}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"error":nil}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  ['{"done":true,"failures":0,"error":null}', []],
  [
    '{"done":true,"failures":null}',
    "skills: evals check closing line has no failure count",
  ],
  ['{"done":null}', "skills: evals check printed an empty object"],
  [
    '{"rule":"r","path":"p","message":"plain"}\n{"done":true,"failures":1}',
    [["r", "p", "plain"]],
  ],
  [
    '{"rule":"r","path":"p","message":"A\\u0041B"}\n{"done":true,"failures":1}',
    [["r", "p", "AAB"]],
  ],
  [
    '{"rule":"r","path":"p","message":"\\u00e9"}\n{"done":true,"failures":1}',
    [["r", "p", "\u00e9"]],
  ],
  [
    '{"rule":"r","path":"p","message":"\\uD83D\\uDE00"}\n{"done":true,"failures":1}',
    [["r", "p", "\ud83d\ude00"]],
  ],
  [
    '{"rule":"r","path":"p","message":"\\uD83D"}\n{"done":true,"failures":1}',
    [["r", "p", "\ufffd"]],
  ],
  [
    '{"rule":"r","path":"p","message":"\\uDE00"}\n{"done":true,"failures":1}',
    [["r", "p", "\ufffd"]],
  ],
  [
    '{"rule":"r","path":"p","message":"\\uD83Dx"}\n{"done":true,"failures":1}',
    [["r", "p", "\ufffdx"]],
  ],
  [
    '{"rule":"r","path":"p","message":"\\ud83d\\u0041"}\n{"done":true,"failures":1}',
    [["r", "p", "\ufffdA"]],
  ],
  [
    '{"rule":"r","path":"p","message":"\\uD800"}\n{"done":true,"failures":1}',
    [["r", "p", "\ufffd"]],
  ],
  [
    '{"rule":"r","path":"p","message":"\\uDBFF"}\n{"done":true,"failures":1}',
    [["r", "p", "\ufffd"]],
  ],
  [
    '{"rule":"r","path":"p","message":"\\uDC00"}\n{"done":true,"failures":1}',
    [["r", "p", "\ufffd"]],
  ],
  [
    '{"rule":"r","path":"p","message":"\\uDFFF"}\n{"done":true,"failures":1}',
    [["r", "p", "\ufffd"]],
  ],
  [
    '{"rule":"r","path":"p","message":"\\uDBFF\\uDFFF"}\n{"done":true,"failures":1}',
    [["r", "p", "\udbff\udfff"]],
  ],
  [
    '{"rule":"r","path":"p","message":"\\uD800\\uDC00"}\n{"done":true,"failures":1}',
    [["r", "p", "\ud800\udc00"]],
  ],
  [
    '{"rule":"r","path":"p","message":"\\uDBFF\\uDBFF"}\n{"done":true,"failures":1}',
    [["r", "p", "\ufffd\ufffd"]],
  ],
  [
    '{"rule":"r","path":"p","message":"\\uDFFF\\uDC00"}\n{"done":true,"failures":1}',
    [["r", "p", "\ufffd\ufffd"]],
  ],
  [
    '{"rule":"r","path":"p","message":"\\uD7FF\\uE000"}\n{"done":true,"failures":1}',
    [["r", "p", "\ud7ff\ue000"]],
  ],
  [
    '{"rule":"r","path":"p","message":"\\uDB00\\uDE00"}\n{"done":true,"failures":1}',
    [["r", "p", "\udb00\ude00"]],
  ],
  [
    '{"rule":"r","path":"p","message":"\\uDAFF\\uDC00"}\n{"done":true,"failures":1}',
    [["r", "p", "\udaff\udc00"]],
  ],
  [
    '{"rule":"r","path":"p","message":"\\uD900\\uDD00"}\n{"done":true,"failures":1}',
    [["r", "p", "\ud900\udd00"]],
  ],
  [
    '{"rule":"r","path":"p","message":"\\uD900\\uDBFF"}\n{"done":true,"failures":1}',
    [["r", "p", "\ufffd\ufffd"]],
  ],
  [
    '{"rule":"r","path":"p","message":"\\uD900\\uE000"}\n{"done":true,"failures":1}',
    [["r", "p", "\ufffd\ue000"]],
  ],
  [
    '{"rule":"r","path":"p","message":"\\uD900\\uDB00"}\n{"done":true,"failures":1}',
    [["r", "p", "\ufffd\ufffd"]],
  ],
  [
    '{"rule":"r","path":"p","message":"\\n\\t\\r\\b\\f\\/\\\\\\""}\n{"done":true,"failures":1}',
    [["r", "p", '\n\t\r\b\f/\\"']],
  ],
  [
    '{"rule":"r","path":"p","message":"\\x"}\n{"done":true,"failures":1}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"rule":"r","path":"p","message":"\\"}\n{"done":true,"failures":1}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"rule":"r","path":"p","message":"\\}\n{"done":true,"failures":1}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"rule":"r","path":"p","message":"\\u12G4"}\n{"done":true,"failures":1}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"rule":"r","path":"p","message":"\\u12"}\n{"done":true,"failures":1}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"rule":"r","path":"p","message":"\\u"}\n{"done":true,"failures":1}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"rule":"r","path":"p","message":"\\u12345"}\n{"done":true,"failures":1}',
    [["r", "p", "\u12345"]],
  ],
  [
    '{"rule":"r","path":"p","message":"\\u+123"}\n{"done":true,"failures":1}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"rule":"r","path":"p","message":"\\u 123"}\n{"done":true,"failures":1}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"rule":"r","path":"p","message":"\\uabcd"}\n{"done":true,"failures":1}',
    [["r", "p", "\uabcd"]],
  ],
  [
    '{"rule":"r","path":"p","message":"\\uABCD"}\n{"done":true,"failures":1}',
    [["r", "p", "\uabcd"]],
  ],
  [
    '{"rule":"r","path":"p","message":"\\uafAF"}\n{"done":true,"failures":1}',
    [["r", "p", "\uafaf"]],
  ],
  [
    '{"rule":"r","path":"p","message":"\\ufgFG"}\n{"done":true,"failures":1}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"rule":"r","path":"p","message":"\\u0g00"}\n{"done":true,"failures":1}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"rule":"r","path":"p","message":"\\u00g0"}\n{"done":true,"failures":1}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"rule":"r","path":"p","message":"\\u000g"}\n{"done":true,"failures":1}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"rule":"r","path":"p","message":"\\uG000"}\n{"done":true,"failures":1}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"rule":"r","path":"p","message":"abc}\n{"done":true,"failures":1}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"rule":"r","path":"p","message":"\u0001"}\n{"done":true,"failures":1}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"rule":"r","path":"p","message":"\u0007"}\n{"done":true,"failures":1}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"rule":"r","path":"p","message":"\u001f"}\n{"done":true,"failures":1}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"rule":"r","path":"p","message":" "}\n{"done":true,"failures":1}',
    [["r", "p", " "]],
  ],
  [
    '{"rule":"r","path":"p","message":"a\nb"}\n{"done":true,"failures":1}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"rule":"r","path":"p","message":"\u007f"}\n{"done":true,"failures":1}',
    [["r", "p", "\u007f"]],
  ],
  [
    '{"rule":"r","path":"p","message":"\u00e9\ud83d\ude00"}\n{"done":true,"failures":1}',
    [["r", "p", "\u00e9\ud83d\ude00"]],
  ],
  [
    '{"rule":"r","path":"p","message":"a"x}\n{"done":true,"failures":1}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"rule":"r","path":"p","message":"\ud83d\ude00"}\n{"done":true,"failures":1}',
    [["r", "p", "\ud83d\ude00"]],
  ],
  [
    "\u00e9",
    "skills: evals check printed a line that is not valid output: line 1 is not a JSON object",
  ],
  [
    "\ud83d\ude00",
    "skills: evals check printed a line that is not valid output: line 1 is not a JSON object",
  ],
  [
    "\u0001",
    "skills: evals check printed a line that is not valid output: line 1 is not a JSON object",
  ],
  [
    '\ufeff{"done":true,"failures":0}',
    "skills: evals check printed a line that is not valid output: line 1 is not a JSON object",
  ],
  [
    '{"done":\u00e9}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
  [
    '{"done":true,"failures":0}\n\u0007',
    "skills: evals check printed a line that is not valid output: line 2 is not a JSON object",
  ],
  [
    '{"rule"\u0001:"r"}',
    "skills: evals check printed a line that is not valid output: line 1 is not valid JSON",
  ],
];

test("pinned streams read as pinned, reason texts included", () => {
  for (const [stdout, expected] of pinnedStreams) {
    if (typeof expected === "string")
      assert.throws(
        () => parseEvalsCheck(stdout),
        (error: unknown) =>
          error instanceof Error && error.message === expected,
        JSON.stringify(stdout),
      );
    else
      assert.deepEqual(
        parseEvalsCheck(stdout).map((one) => [one.rule, one.path, one.message]),
        expected,
        JSON.stringify(stdout),
      );
  }
});
// END mutation rows

// --- the whole-domain scenarios ------------------------------------------------------------------------------------

/** The finding the stub's failure line for `at` and `message` becomes. */
const stubFinding = (at: string, message: string): Failure => ({
  rule: "eval-stub-rule",
  path: at,
  message,
});

test("one stub failure", async () => {
  assert.deepEqual(await auditEveryFamily(() => {}, { stream: STUB_ONE }), {
    failures: [stubFinding("evals/skills.yaml", "stub finding")],
  });
});

test("two stub failures, one with an empty path and message", async () => {
  assert.deepEqual(
    await auditEveryFamily(() => {}, {
      stream: `${stubFailure("evals/skills.yaml", "first")}\n${stubFailure("", "")}\n${closing(2)}\n`,
    }),
    {
      failures: [
        stubFinding("evals/skills.yaml", "first"),
        stubFinding("", ""),
      ],
    },
  );
});

test("a stub message holding <, & and a non-ASCII letter", async () => {
  const message = `a < b & caf${E_ACUTE}`;
  assert.deepEqual(
    await auditEveryFamily(() => {}, {
      stream: `${stubFailure("evals/skills.yaml", message)}\n${closing(1)}\n`,
    }),
    { failures: [stubFinding("evals/skills.yaml", message)] },
  );
});

test("stream keys in another case", async () => {
  // Keys match case-sensitively, so "RULE" is an unknown field and the audit stops.
  assert.deepEqual(
    await auditEveryFamily(() => {}, {
      stream:
        '{"RULE":"eval-stub-rule","Path":"p.md","MESSAGE":"m"}\n{"Done":true,"FAILURES":1}\n',
    }),
    {
      failures: [],
      error: `${NOT_VALID}line 1 has an unknown field "RULE" (run: evals check exited with code 0)`,
    },
  );
});

test("two stream values on one line", async () => {
  assert.deepEqual(
    await auditEveryFamily(() => {}, {
      stream: `${stubFailure("p.md", "m")} ${closing(1)}\n`,
    }),
    { failures: [stubFinding("p.md", "m")] },
  );
});

test("blank lines between stream values", async () => {
  assert.deepEqual(
    await auditEveryFamily(() => {}, {
      stream: `\n\n${stubFailure("p.md", "m")}\n\n\n${closing(1)}\n\n`,
    }),
    { failures: [stubFinding("p.md", "m")] },
  );
});

test("a duplicate key in a stream line", async () => {
  // The later value of a repeated key wins.
  assert.deepEqual(
    await auditEveryFamily(() => {}, {
      stream: `{"rule":"other","rule":"eval-stub-rule","path":"p.md","message":"m"}\n${closing(1)}\n`,
    }),
    { failures: [stubFinding("p.md", "m")] },
  );
});
