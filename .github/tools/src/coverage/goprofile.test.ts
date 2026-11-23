import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import {
  cpSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { test } from "node:test";
import type { TestContext } from "node:test";
import {
  convertGoProfile,
  parseGoProfile,
  sourceFunctions,
} from "./goprofile.ts";

const repo = fileURLToPath(new URL("../../../../", import.meta.url));
const fixture = (name: string) =>
  fileURLToPath(new URL(`./fixtures/${name}`, import.meta.url));
function summary(xml: string) {
  const root = /<coverage\b[^>]*>/.exec(xml)![0];
  const counts = [
    "lines-covered",
    "lines-valid",
    "branches-covered",
    "branches-valid",
  ].map((name) => Number(new RegExp(`${name}="(\\d+)"`).exec(root)![1]));
  const files = new Map<string, Set<number>>();
  for (const match of xml.matchAll(
    /<class\b[^>]*filename="([^"]+)"[^>]*>([\s\S]*?)<\/class>/g,
  )) {
    const lines = files.get(match[1]!) ?? new Set<number>();
    for (const line of match[2]!.matchAll(
      /<line\b[^>]*number="(\d+)"[^>]*hits="(\d+)"/g,
    ))
      if (Number(line[2]) > 0) lines.add(Number(line[1]));
    files.set(match[1]!, lines);
  }
  return { counts, files };
}

// The profile and the reference report are pinned against these frozen copies
// of the profiled sources, so the live production sources must not be read. The
// go.mod is written at run time because a checked-in one outside hooks/go
// fails the area-go-module-set conformance rule.
test("sample profile matches the pinned reference report's counts and covered lines", (t) => {
  const root = mkdtempSync(join(tmpdir(), "agentics-goprofile-sample-"));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  cpSync(fixture("hooks-sample-src"), root, { recursive: true });
  writeFileSync(
    join(root, "hooks", "go", "go.mod"),
    "module github.com/muehz/agentics/hooks/go\n\ngo 1.27.4\n",
  );
  const profile = readFileSync(fixture("hooks-sample.out"), "utf8");
  const reference = readFileSync(fixture("hooks-sample.reference.xml"), "utf8");
  const expected = summary(reference);
  const repositoryFiles = new Map(
    [...expected.files].map(([file, lines]) => [`hooks/go/${file}`, lines]),
  );
  assert.deepEqual(
    summary(
      convertGoProfile(profile, {
        module: "hooks/go",
        sourceRoot: root,
      }),
    ),
    { counts: expected.counts, files: repositoryFiles },
  );
  assert.ok(!reference.includes("Users"));
  assert.match(reference, /timestamp="0"/);
});

test("all profile modes parse and invalid ranges report their line", () => {
  for (const mode of ["set", "count", "atomic"]) {
    const parsed = parseGoProfile(
      `mode: ${mode}\nexample.test/p/a.go:2.1,2.9 1 3\n`,
      "mode.out",
    );
    assert.equal(parsed.mode, mode);
    assert.equal(parsed.files.get("example.test/p/a.go")![0]!.count, 3);
  }
  assert.throws(
    () =>
      parseGoProfile(
        "mode: count\nexample.test/p/a.go:3.9,2.1 1 3\n",
        "broken.out",
      ),
    /broken\.out:2:.*range/,
  );
  assert.throws(
    () => parseGoProfile("mode: bogus\n", "broken.out"),
    /broken\.out:1:.*mode/,
  );
  assert.throws(
    () => parseGoProfile("mode: count\nnot a block\n", "broken.out"),
    /broken\.out:2:/,
  );
});

const source = `package sample
var excluded = struct{ F func() int }{F: func() int { return 9 }}; func First() int { return 1 }; func Second() int { return 2 }
var Direct = func() int { return 3 }
var Grouped = (func() int { return 4 })
func Outer() int { local := func() int { return 5 }; return local() }
`;

function synthetic(t: TestContext) {
  const root = mkdtempSync(join(tmpdir(), "agentics-goprofile-"));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  const module = join(root, "module");
  mkdirSync(module);
  writeFileSync(
    join(module, "go.mod"),
    "module example.test/sample\n\ngo 1.27.3\n",
  );
  writeFileSync(join(module, "sample.go"), source);
  const second = source.split("\n")[1]!;
  const block = (line: number, needle: string, count: number) => {
    const text = source.split("\n")[line - 1]!;
    const start = text.indexOf(needle) + 1;
    return `example.test/sample/sample.go:${line}.${start},${line}.${start + needle.length} 1 ${count}`;
  };
  const profile =
    "mode: count\n" +
    [
      block(2, "return 9", 7),
      block(2, "return 1", 1),
      block(2, "return 2", 0),
      block(3, "return 3", 1),
      block(4, "return 4", 9),
      block(5, "return 5", 2),
    ].join("\n") +
    "\n";
  assert.ok(second.includes("func Second"));
  return { root, module, profile };
}

test("source positions exclude nested initializers and retain separate same-line functions", (t) => {
  const f = synthetic(t);
  const xml = convertGoProfile(f.profile, {
    module: "module",
    sourceRoot: f.root,
  });
  const report = summary(xml);
  // The report holds four methods: First, Second, Direct and Outer.
  assert.deepEqual(report.counts, [3, 4, 0, 0]);
  assert.deepEqual(
    report.files,
    new Map([["module/sample.go", new Set([2, 3, 5])]]),
  );
  assert.match(xml, /name="First"/);
  assert.match(xml, /name="Second"/);
  assert.ok(!xml.includes('name="excluded"'));
  assert.ok(!xml.includes('name="Grouped"'));
});

test("scanner ignores braces in literals/comments and uses byte columns", () => {
  const input =
    'package x\n/* Ω { */ func F[T any](x T) string { _ = `}`; _ = "{\\\"}"; _ = \'{\'; return "x" }\n';
  const found = sourceFunctions(input, "test.go");
  assert.equal(found.length, 1);
  assert.equal(found[0]!.name, "F");
  assert.equal(found[0]!.start.column, Buffer.byteLength("/* Ω { */ ") + 1);
  assert.throws(
    () => sourceFunctions("package x\nfunc F() {", "bad.go"),
    /bad\.go.*unclosed/,
  );
});

test("goprofile CLI accepts explicit repository root and defaults it to cwd", (t) => {
  const f = synthetic(t);
  const profile = join(f.root, "profile.out"),
    output = join(f.root, "coverage.xml");
  writeFileSync(profile, f.profile);
  for (const extra of [["--source-root", f.root], []]) {
    const result = spawnSync(
      process.execPath,
      [
        resolve(repo, ".github/tools/src/coverage/cli.ts"),
        "goprofile",
        "--module",
        "module",
        "--profile",
        profile,
        ...extra,
        "--out",
        output,
      ],
      { cwd: f.root, encoding: "utf8" },
    );
    assert.equal(result.status, 0, result.stderr);
    assert.deepEqual(summary(readFileSync(output, "utf8")), {
      counts: [3, 4, 0, 0],
      files: new Map([["module/sample.go", new Set([2, 3, 5])]]),
    });
  }
  writeFileSync(
    profile,
    "mode: set\nexample.test/sample/sample.go:5.9,2.1 1 1\n",
  );
  const result = spawnSync(
    process.execPath,
    [
      resolve(repo, ".github/tools/src/coverage/cli.ts"),
      "goprofile",
      "--module",
      "module",
      "--profile",
      profile,
      "--out",
      output,
    ],
    { cwd: f.root, encoding: "utf8" },
  );
  assert.equal(result.status, 1);
  assert.match(result.stderr, /profile\.out:2:.*range/);
});
