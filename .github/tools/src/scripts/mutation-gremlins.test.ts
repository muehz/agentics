import type { TestContext } from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import {
  baseline,
  appendProbe,
  excludePatterns,
  gremlinsArgs,
  runPackages,
} from "./mutation-gremlins.ts";

function fixture(t: TestContext) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "mutation-fixture-"));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const mod = path.join(root, "repo", "hooks", "go");
  const write = (rel: string, text: string) => {
    fs.mkdirSync(path.dirname(path.join(mod, rel)), { recursive: true });
    fs.writeFileSync(path.join(mod, rel), text);
  };
  write("go.mod", "module example.com/m\n\ngo 1.27\n");
  fs.writeFileSync(path.join(root, "repo", "outside.txt"), "x");
  write("plain/plain.go", "package plain\n\nfunc One() int { return 1 }\n");
  write(
    "plain/plain_test.go",
    'package plain\n\nimport "testing"\n\nfunc TestOne(t *testing.T) { if One() != 1 { t.Fatal() } }\n',
  );
  write("outside/outside.go", "package outside\n");
  write(
    "outside/outside_test.go",
    'package outside\n\nimport ("os"; "testing")\n\nfunc TestRead(t *testing.T) { if _, err := os.ReadFile("../../../outside.txt"); err != nil { t.Fatal(err) } }\n',
  );
  write("hashed/hashed.go", "package hashed\n");
  write(
    "hashed/hashed_test.go",
    'package hashed\n\nimport ("os"; "testing")\n\nfunc TestHash(t *testing.T) { b, _ := os.ReadFile("hashed.go"); if string(b) != "package hashed\\n" { t.Fatal("source changed") } }\n',
  );
  return mod;
}

test("a plain package passes both phases", (t) => {
  assert.deepEqual(baseline(fixture(t), "./plain", {}), {
    pkg: "./plain",
    ok: true,
  });
});

test("a test that reads outside the module fails phase 1 in the copy", (t) => {
  const result = baseline(fixture(t), "./outside", {});
  assert.equal(result.ok, false);
  assert.match(result.reason, /^fails in an isolated copy/);
});

test("a test that reads its own source fails phase 2", (t) => {
  const result = baseline(fixture(t), "./hashed", {});
  assert.equal(result.ok, false);
  assert.match(result.reason, /^fails after a comment-only change/);
});

test("the probe adds one comment line and keeps the code", () => {
  assert.equal(
    appendProbe("package x\n"),
    "package x\n\n// mutation baseline probe\n",
  );
});

test("nested folders and unchanged top-level files become exclude patterns", () => {
  assert.deepEqual(excludePatterns(["a.go", "b.go", "c.go"], ["b.go"]), [
    "/",
    "^a\\.go$",
    "^c\\.go$",
  ]);
});

test("a full-package run excludes only nested folders", () => {
  assert.deepEqual(excludePatterns(["a.go", "b.go"]), ["/"]);
});

test("a file name with regex characters is escaped", () => {
  assert.deepEqual(excludePatterns(["a+b.go"], []), ["/", "^a\\+b\\.go$"]);
});

test("a changed-files run excludes the package's other files and writes JSON", () => {
  assert.deepEqual(
    gremlinsArgs("./internal/x", ["a.go", "b.go"], ["b.go"], "/out/x.json"),
    [
      "unleash",
      "--workers",
      "4",
      "--output",
      "/out/x.json",
      "--exclude-files",
      "/",
      "--exclude-files",
      "^a\\.go$",
      "./internal/x",
    ],
  );
});

test("a full-package gremlins run excludes only nested folders", () => {
  assert.deepEqual(gremlinsArgs("./internal/x", ["a.go"], null, "/o.json"), [
    "unleash",
    "--workers",
    "4",
    "--output",
    "/o.json",
    "--exclude-files",
    "/",
    "./internal/x",
  ]);
});

test("a package that fails the baseline is a tool failure", (t) => {
  const out = path.join(
    fs.mkdtempSync(path.join(os.tmpdir(), "mutation-out-")),
    "g.json",
  );
  t.after(() => fs.rmSync(path.dirname(out), { recursive: true, force: true }));
  const failures = runPackages(
    [{ pkg: "./internal/no-such-package", changed: null }],
    out,
    () => assert.fail("gremlins must not run for an excluded package"),
  );
  assert.equal(failures, 1);
  const entry = JSON.parse(fs.readFileSync(out, "utf8")).packages[0];
  assert.equal(entry.baseline.ok, false);
});

test("a gremlins run that fails or writes nothing is a tool failure", (t) => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "mutation-out-"));
  t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
  const target = [{ pkg: "./internal/toolname", changed: null }];
  const crash = runPackages(target, path.join(dir, "a.json"), () => ({
    status: null,
    error: new Error("spawn gremlins ENOENT"),
  }));
  assert.equal(crash, 1);
  assert.match(
    JSON.parse(fs.readFileSync(path.join(dir, "a.json"), "utf8")).packages[0]
      .error,
    /ENOENT/,
  );
  const silent = runPackages(target, path.join(dir, "b.json"), () => ({
    status: 0,
  }));
  assert.equal(silent, 1);
  assert.match(
    JSON.parse(fs.readFileSync(path.join(dir, "b.json"), "utf8")).packages[0]
      .error,
    /no result file/,
  );
});

test("a finished gremlins run is not a failure", (t) => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "mutation-out-"));
  t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
  const failures = runPackages(
    [{ pkg: "./internal/toolname", changed: null }],
    path.join(dir, "c.json"),
    (_cmd, args) => {
      fs.writeFileSync(outputArg(args), '{"files":[]}');
      return { status: 0 };
    },
  );
  assert.equal(failures, 0);
});

test("results already written survive a run that stops early", (t) => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "mutation-out-"));
  t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
  const out = path.join(dir, "d.json");
  let calls = 0;
  assert.throws(() =>
    runPackages(
      [
        { pkg: "./internal/toolname", changed: null },
        { pkg: "./internal/toolname", changed: null },
      ],
      out,
      (_cmd, args) => {
        if (++calls === 2) throw new Error("time limit");
        fs.writeFileSync(outputArg(args), '{"files":[]}');
        return { status: 0 };
      },
    ),
  );
  assert.equal(JSON.parse(fs.readFileSync(out, "utf8")).packages.length, 1);
});

test("gremlins runs with the Go test cache off", (t) => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "mutation-out-"));
  t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
  let seen: string | undefined;
  runPackages(
    [{ pkg: "./internal/toolname", changed: null }],
    path.join(dir, "e.json"),
    (_cmd, args, options) => {
      seen = options.env!.GOFLAGS;
      fs.writeFileSync(outputArg(args), '{"files":[]}');
      return { status: 0 };
    },
  );
  assert.match(seen!, /(^| )-count=1( |$)/);
});

test("a gremlins result file that does not parse is a tool failure", (t) => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "mutation-out-"));
  t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
  const out = path.join(dir, "f.json");
  const failures = runPackages(
    [{ pkg: "./internal/toolname", changed: null }],
    out,
    (_cmd, args) => {
      fs.writeFileSync(outputArg(args), "{ cut off");
      return { status: 0 };
    },
  );
  assert.equal(failures, 1);
  const entry = JSON.parse(fs.readFileSync(out, "utf8")).packages[0];
  assert.equal(entry.result, undefined);
  assert.match(entry.error, /unreadable result file/);
});

function outputArg(args: readonly unknown[]): string {
  const value = args[args.indexOf("--output") + 1];
  assert.equal(typeof value, "string");
  return value as string;
}
