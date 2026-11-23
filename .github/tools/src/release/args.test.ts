import assert from "node:assert/strict";
import { test } from "node:test";
import { parseArgs, parseFlags } from "./args.ts";
import { EXIT_FAILURE, EXIT_OK, EXIT_USAGE, report, type Io } from "./io.ts";

const ALLOWED = ["repo", "version", "main-ref"];

// The streams are collected here rather than by testing/support.ts, which
// itself depends on io.ts.
function captureIo(): { io: Io; stdout(): string; stderr(): string } {
  let out = "";
  let err = "";
  return {
    io: {
      stdout: { write: (chunk: string) => (out += chunk) },
      stderr: { write: (chunk: string) => (err += chunk) },
    },
    stdout: () => out,
    stderr: () => err,
  };
}

function ok(argv: string[]): Record<string, string> {
  const parsed = parseArgs(argv, ALLOWED);
  assert.equal(parsed.ok, true, JSON.stringify(parsed));
  return parsed.ok ? { ...parsed.values } : {};
}

function message(argv: string[]): string {
  const parsed = parseArgs(argv, ALLOWED);
  assert.equal(parsed.ok, false, `${argv.join(" ")} should not parse`);
  return parsed.ok ? "" : parsed.message;
}

test("parseArgs reads --name value and --name=value", () => {
  assert.deepEqual(ok(["--repo", "/r", "--version", "1.2.3"]), {
    repo: "/r",
    version: "1.2.3",
  });
  assert.deepEqual(ok(["--repo=/r", "--main-ref=origin/main"]), {
    repo: "/r",
    "main-ref": "origin/main",
  });
  assert.deepEqual(ok(["--repo=/r=s", "--version", "a=b"]), {
    repo: "/r=s",
    version: "a=b",
  });
});

test("parseArgs returns no values for no arguments, and omits flags that never appear", () => {
  assert.deepEqual(ok([]), {});
  assert.deepEqual(ok(["--repo", "/r"]), { repo: "/r" });
});

test("parseArgs lets the last occurrence of a flag win, even an empty one", () => {
  assert.deepEqual(ok(["--repo", "a", "--repo", "b"]), { repo: "b" });
  assert.deepEqual(ok(["--repo=a", "--repo", "b", "--repo=c"]), { repo: "c" });
  assert.deepEqual(ok(["--repo", "a", "--repo="]), { repo: "" });
});

// An empty value is present, and the callers that test `!values.x` treat it as
// missing.
test("parseArgs keeps an empty value so a caller can treat it as missing", () => {
  assert.deepEqual(ok(["--repo", ""]), { repo: "" });
  assert.deepEqual(ok(["--repo="]), { repo: "" });
});

// The argument after a flag is its value, whatever it looks like.
test("parseArgs takes the next argument as a value even when it starts with a dash", () => {
  assert.deepEqual(ok(["--repo", "--version"]), { repo: "--version" });
  assert.deepEqual(ok(["--repo", "-x"]), { repo: "-x" });
});

test("parseArgs rejects a flag with no value", () => {
  assert.equal(message(["--repo"]), "flag --repo needs a value");
  assert.equal(
    message(["--version", "1", "--repo"]),
    "flag --repo needs a value",
  );
});

test("parseArgs rejects an unknown flag, naming it", () => {
  assert.equal(message(["--force"]), "unknown flag --force");
  assert.equal(message(["--force=1"]), "unknown flag --force");
  assert.equal(message(["--repo=/r", "--Repo", "x"]), "unknown flag --Repo");
  assert.equal(message(["--=x"]), "unknown flag --");
});

// No caller uses single-dash flags or -h, so they are usage errors.
test("parseArgs rejects single-dash flags and -h", () => {
  const reason = (flag: string) =>
    `unsupported flag ${flag} (flags take the form --name value)`;
  assert.equal(message(["-repo", "/r"]), reason("-repo"));
  assert.equal(message(["-repo=/r"]), reason("-repo=/r"));
  assert.equal(message(["-h"]), reason("-h"));
  assert.equal(message(["--repo", "/r", "-version", "1"]), reason("-version"));
});

test("parseArgs rejects a positional argument", () => {
  assert.equal(message(["extra"]), "unexpected argument extra");
  assert.equal(message(["--repo", "/r", "extra"]), "unexpected argument extra");
  assert.equal(message(["-"]), "unexpected argument -");
  assert.equal(message([""]), "unexpected argument ");
});

// A bare -- ends the flags; whatever follows is positional.
test("parseArgs ends the flags at a bare --", () => {
  assert.deepEqual(ok(["--repo", "/r", "--"]), { repo: "/r" });
  assert.equal(
    message(["--repo", "/r", "--", "--version", "1"]),
    "unexpected argument --version",
  );
  assert.equal(message(["--", "x"]), "unexpected argument x");
});

test("parseArgs does not read an inherited property as a flag name", () => {
  assert.equal(message(["--constructor", "x"]), "unknown flag --constructor");
  assert.equal(message(["--__proto__=x"]), "unknown flag --__proto__");
  assert.equal(Object.hasOwn(ok(["--repo", "r"]), "version"), false);
});

test("parseFlags writes the one diagnostic line and returns undefined on a bad argument", () => {
  const captured = captureIo();
  assert.equal(
    parseFlags(["--nope"], ALLOWED, captured.io, "release: set-version"),
    undefined,
  );
  assert.equal(
    captured.stderr(),
    "release: set-version: unknown flag --nope\n",
  );
  assert.equal(captured.stdout(), "");
});

test("parseFlags returns the values and writes nothing when the arguments parse", () => {
  const captured = captureIo();
  assert.deepEqual(
    parseFlags(["--repo", "/r"], ALLOWED, captured.io, "release: set-version"),
    { repo: "/r" },
  );
  assert.equal(captured.stderr(), "");
});

test("report writes `prefix: message` to stderr and returns the code", () => {
  const captured = captureIo();
  assert.equal(
    report(captured.io, "release: resume-check", "--sha is wrong", EXIT_USAGE),
    2,
  );
  assert.equal(
    report(
      captured.io,
      "release bundle",
      "target x is not a commit",
      EXIT_FAILURE,
    ),
    1,
  );
  assert.equal(
    captured.stderr(),
    "release: resume-check: --sha is wrong\nrelease bundle: target x is not a commit\n",
  );
  assert.equal(captured.stdout(), "");
  assert.deepEqual([EXIT_OK, EXIT_FAILURE, EXIT_USAGE], [0, 1, 2]);
});
