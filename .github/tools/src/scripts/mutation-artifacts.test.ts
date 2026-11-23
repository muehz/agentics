import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test, { type TestContext } from "node:test";
import { pathToFileURL } from "node:url";
import * as artifacts from "./mutation-artifacts.ts";
import * as source from "./mutation-source.ts";
import { selectScope } from "./mutation-scope.ts";
import * as coverage from "../coverage/scope.ts";
import { collect, gremlinsSection } from "./mutation-report.ts";
import { runPackages } from "./mutation-gremlins.ts";
import { strykerArgs, parseArgs } from "./mutation-stryker.ts";
import {
  AREAS,
  MUTATION_CONFIG_FILES,
  SKIP_TESTS_ENV,
} from "./mutation-config-values.ts";

function tempDir(t: TestContext) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "mutation-contract-"));
  t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
  return dir;
}

function cli(file: string, ...args: string[]) {
  const run = spawnSync(process.execPath, [file, ...args], {
    encoding: "utf8",
    env: {
      ...process.env,
      GITHUB_OUTPUT: "",
      AGX_MUTATION_TEST_FILES: "",
      [SKIP_TESTS_ENV]: "",
    },
  });
  assert.ifError(run.error);
  return run;
}

function outcome<T>(operation: () => T) {
  try {
    return { value: operation() };
  } catch (error) {
    assert.ok(error instanceof Error);
    return { error: error.message, name: error.name };
  }
}

function fixtureScripts(dir: string) {
  const scripts = path.join(dir, ".github/tools/src/scripts");
  fs.mkdirSync(scripts, { recursive: true });
  fs.writeFileSync(path.join(dir, "package.json"), '{"type":"module"}');
  for (const name of [
    "mutation-config-values",
    "mutation-config",
    "mutation-stryker",
    "mutation-artifacts",
    "mutation-source",
  ])
    fs.copyFileSync(
      path.join(import.meta.dirname, name + ".ts"),
      path.join(scripts, name + ".ts"),
    );
  return scripts;
}

test("source policies preserve coverage input, Stryker globs and changed Go files", () => {
  assert.deepEqual(source.SOURCE_ROOTS, coverage.SOURCE_ROOTS);
  assert.deepEqual(source.SOURCE_ROOTS, AREAS);
  assert.deepEqual(source.EXCLUDE_PATTERNS, coverage.EXCLUDE_PATTERNS);
  const dir = ".github/tools/src/release/commands";
  assert.deepEqual(
    source.strykerSourceGlobs(dir),
    strykerArgs({ area: dir })[2]!.split(","),
  );
  const paths = [
    ".github/tools/src/a.ts",
    ".github/tools/src/a.test.ts",
    ".github/tools/src/a.mjs",
    ".github/tools/src/testing/a.ts",
    ".github/tools/src/testing",
    ".github/tools/src/testings/a.ts",
    ".github/tools/src/line\nbreak/a.test.ts",
    ...MUTATION_CONFIG_FILES,
    ".github/tools/src-other/a.ts",
    "evals/src/a.ts",
    "hooks/go/main.go",
    "hooks/go/a/a.go",
    "hooks/go/a/a_test.go",
    "hooks/go/testdata/a.go",
    "hooks/goose/a.go",
  ];
  const selected = (policy: "coverage" | "stryker") =>
    paths.filter((file) => source.isProductionSource(file, policy));
  assert.deepEqual(
    selected("coverage"),
    paths.filter(coverage.inCoverageScope),
  );
  assert.deepEqual(selected("stryker"), [
    ".github/tools/src/a.ts",
    ".github/tools/src/testings/a.ts",
  ]);
  assert.ok(selected("coverage").includes(".github/tools/src/a.mjs"));
  assert.ok(selected("coverage").includes(MUTATION_CONFIG_FILES[0]!));
  assert.deepEqual(paths.filter(source.isGremlinsSource), [
    "hooks/go/main.go",
    "hooks/go/a/a.go",
  ]);
  assert.deepEqual(selectScope(paths), {
    stryker: selected("stryker"),
    gremlins: [
      { pkg: ".", changed: ["main.go"] },
      { pkg: "./a", changed: ["a.go"] },
    ],
  });
});

test("the retired scope launcher and a cold selection import do not write output", (t) => {
  const dir = tempDir(t);
  assert.equal(artifacts.SCOPE_FILE, "scope.json");
  const file = path.join(dir, artifacts.SCOPE_FILE);
  const scopePath = path.join(import.meta.dirname, "mutation-scope.ts");
  const direct = cli(scopePath, "--base", "HEAD", "--out", file);
  assert.equal(direct.status, 0, direct.stderr);
  assert.equal(direct.stdout, "");
  assert.equal(direct.stderr, "");
  assert.equal(fs.existsSync(file), false);
  const imported = cli(
    "--input-type=module",
    "-e",
    "const { selectScope } = await import(process.argv[1]); console.log(JSON.stringify(selectScope([])))",
    pathToFileURL(scopePath).href,
    "--base",
    "HEAD",
    "--out",
    file,
  );
  assert.equal(imported.status, 0, imported.stderr);
  assert.equal(imported.stdout.trim(), '{"stryker":[],"gremlins":[]}');
  assert.equal(imported.stderr, "");
  assert.equal(fs.existsSync(file), false);
});

test("scope encoding and both strict consumers preserve the wire format", (t) => {
  const file = path.join(tempDir(t), artifacts.SCOPE_FILE);
  const empty =
    '{\n  "stryker": [],\n  "gremlins": [],\n  "strykerLines": {}\n}\n';
  assert.equal(
    artifacts.encodeScope({ ...selectScope([]), strykerLines: {} }),
    empty,
  );
  const target = ".github/tools/src/a.ts";
  const scope = selectScope([target, "hooks/go/a.go"]);
  scope.strykerLines = { [target]: [[3, 7]] };
  const bytes = `{
  "stryker": [
    ".github/tools/src/a.ts"
  ],
  "gremlins": [
    {
      "pkg": ".",
      "changed": [
        "a.go"
      ]
    }
  ],
  "strykerLines": {
    ".github/tools/src/a.ts": [
      [
        3,
        7
      ]
    ]
  }
}
`;
  assert.equal(artifacts.encodeScope(scope), bytes);
  fs.writeFileSync(file, bytes);
  assert.deepEqual([...artifacts.readGremlinsTargets(file)], scope.gremlins);
  for (const raw of [
    bytes,
    '{"stryker":"scalar","gremlins":false}',
    "{}",
    "null",
    "{",
    undefined,
  ]) {
    if (raw === undefined) fs.unlinkSync(file);
    else fs.writeFileSync(file, raw);
    assert.deepEqual(
      outcome(() => artifacts.readStrykerScope(file)),
      outcome(() => parseArgs(["--scope", file])),
    );
  }
});

test("the gremlins driver accepts empty iterable targets and rejects malformed scope", (t) => {
  const dir = tempDir(t);
  const file = path.join(dir, "scope.json");
  const output = path.join(dir, artifacts.GREMLINS_FILE);
  for (const raw of [
    '{"gremlins":[]}',
    '{"gremlins":""}',
    '{"gremlins":[null]}',
    "{}",
    "null",
    "{",
    undefined,
  ]) {
    if (raw === undefined) fs.unlinkSync(file);
    else fs.writeFileSync(file, raw);
    const old = cli(
      path.join(import.meta.dirname, "mutation-gremlins.ts"),
      "--scope",
      file,
      "--out",
      output,
    );
    const current = outcome(() => [...artifacts.readGremlinsTargets(file)]);
    if ("error" in current) {
      assert.equal(old.status, 1);
      assert.ok(old.stderr.includes(current.error!), old.stderr);
    } else {
      assert.equal(old.status, 0, old.stderr);
      assert.deepEqual(current.value, []);
      assert.equal(
        fs.readFileSync(output, "utf8"),
        artifacts.encodeGremlinsReport([]),
      );
    }
  }
});

test("the report scope uses length coercion and treats missing or unreadable files as no result", (t) => {
  const file = path.join(tempDir(t), "scope.json");
  for (const raw of [
    undefined,
    "{",
    "false",
    '{"stryker":{"length":"0"},"gremlins":"x"}',
    '{"stryker":[]}',
  ]) {
    if (raw !== undefined) fs.writeFileSync(file, raw);
    const old = cli(
      path.join(import.meta.dirname, "mutation-report.ts"),
      "--scope",
      file,
    );
    const current = outcome(() => {
      const scope = artifacts.readReportScope(file);
      return scope && { ...scope };
    });
    if ("error" in current) {
      assert.equal(old.status, 1);
      assert.ok(old.stderr.includes(current.error!), old.stderr);
    } else {
      assert.equal(old.status, 0, old.stderr);
      assert.equal(
        current.value === null,
        old.stdout.includes("### Scope: no result"),
      );
      if (current.value) {
        assert.equal(
          current.value.stryker,
          !old.stdout.includes("### Stryker: no in-scope change"),
        );
        assert.equal(
          current.value.gremlins,
          !old.stdout.includes("### gremlins: no in-scope change"),
        );
      }
    }
  }
});

test("the CLI decodes a repeated Gremlins artifact once and renders both contributions", (t) => {
  const dir = tempDir(t);
  const folder = path.join(dir, "leg");
  fs.mkdirSync(folder);
  const artifact = path.join(folder, artifacts.GREMLINS_FILE);
  fs.writeFileSync(
    artifact,
    '{"packages":[{"pkg":".","baseline":{"ok":true},"result":{"files":[]}}]}',
  );
  const preload = path.join(dir, "count-reads.cjs");
  const count = path.join(dir, "reads.txt");
  fs.writeFileSync(
    preload,
    `const fs = require("node:fs");
const path = require("node:path");
const originalRead = fs.readFileSync;
fs.readFileSync = function(file, ...args) {
  if (typeof file === "string" && path.resolve(file) === ${JSON.stringify(artifact)}) {
    fs.appendFileSync(${JSON.stringify(count)}, "read\\n");
  }
  return originalRead.call(this, file, ...args);
};`,
  );
  const run = cli(
    "--require",
    preload,
    path.join(import.meta.dirname, "mutation-report.ts"),
    "--full",
    "--dir",
    dir,
    "--gremlins",
    artifact,
  );
  assert.equal(run.status, 0, run.stderr);
  assert.match(run.stdout, /^- leg: result$/m);
  assert.equal(run.stdout.match(/passed both baseline phases/g)?.length, 2);
  assert.equal(fs.readFileSync(count, "utf8"), "read\n");
});

test(
  "CI and make write the canonical Gremlins bytes and explicit output stays independent",
  {
    skip:
      process.platform === "win32" &&
      "requires POSIX fixture executables and make",
  },
  (t) => {
    const dir = tempDir(t);
    const scripts = fixtureScripts(dir);
    for (const name of ["ci-mutation-hooks.ts", "mutation-gremlins.ts"])
      fs.copyFileSync(
        path.join(import.meta.dirname, name),
        path.join(scripts, name),
      );
    const tools = path.join(dir, "hooks/go");
    const pkg = "./internal/demo";
    fs.mkdirSync(path.join(tools, pkg), { recursive: true });
    fs.writeFileSync(path.join(tools, pkg, "demo.go"), "package demo\n");
    fs.copyFileSync(
      path.resolve(import.meta.dirname, "../../../../hooks/go/Makefile"),
      path.join(tools, "Makefile"),
    );

    const bin = path.join(dir, "bin");
    fs.mkdirSync(bin);
    fs.writeFileSync(
      path.join(bin, "go"),
      `#!/bin/sh
case "$*" in
  "env GOPATH") printf '%s\\n' "$FIXTURE_ROOT";;
  "run ./cmd/sourcedigest") printf '%s\\n' fixture-digest;;
  "run ./cmd/sourcedigest --list") printf '%s\\n' internal/demo/demo.go;;
  "install "*|"test -count=1 ./internal/demo") ;;
  *) exit 12;;
esac
`,
      { mode: 0o755 },
    );
    fs.writeFileSync(
      path.join(bin, "gremlins"),
      `#!/bin/sh
set -eu
[ "$1" = unleash ] && [ "$2" = --workers ] && [ "$3" = 4 ] && [ "$4" = --output ]
printf '%s\\n' '{"files":[]}' > "$5"
`,
      { mode: 0o755 },
    );
    const launched = path.join(dir, "node-args.txt");
    fs.writeFileSync(
      path.join(bin, "node"),
      `#!/bin/sh
printf '%s\\n' "$@" > "$FIXTURE_ARGS"
exec "$FIXTURE_NODE" "$@"
`,
      { mode: 0o755 },
    );
    const env = {
      ...process.env,
      PATH: `${bin}${path.delimiter}${process.env.PATH ?? ""}`,
      GO: "go",
      MAKEFLAGS: "",
      TARGET: pkg,
      FIXTURE_ROOT: dir,
      FIXTURE_NODE: process.execPath,
      FIXTURE_ARGS: launched,
    };
    const driver = path.join(scripts, "mutation-gremlins.ts");
    const goMutation = path.join(dir, "reports/mutation/gremlins.json");
    const expected = Buffer.from(`{
  "packages": [
    {
      "pkg": "./internal/demo",
      "baseline": {
        "pkg": "./internal/demo",
        "ok": true
      },
      "result": {
        "files": []
      }
    }
  ]
}
`);
    for (const [command, args] of [
      [process.execPath, [path.join(scripts, "ci-mutation-hooks.ts")]],
      ["make", ["-C", tools, "mutation", `PKG=${pkg}`]],
    ] as const) {
      assert.equal(fs.existsSync(goMutation), false);
      const run = spawnSync(command, args, { cwd: dir, encoding: "utf8", env });
      assert.ifError(run.error);
      assert.equal(run.status, 0, run.stderr);
      assert.deepEqual(
        fs.readFileSync(launched, "utf8").trimEnd().split("\n").slice(1),
        ["--packages", pkg],
      );
      assert.deepEqual(fs.readFileSync(goMutation), expected);
      fs.unlinkSync(goMutation);
    }

    const override = path.join(dir, "overridden.json");
    const explicit = spawnSync(
      process.execPath,
      [driver, "--packages", pkg, "--out", override],
      { cwd: dir, encoding: "utf8", env },
    );
    assert.ifError(explicit.error);
    assert.equal(explicit.status, 0, explicit.stderr);
    assert.deepEqual(fs.readFileSync(override), expected);
    assert.equal(fs.existsSync(goMutation), false);

    const scope = path.join(dir, "partial-scope.json");
    fs.writeFileSync(scope, JSON.stringify({ gremlins: [{ pkg }, null] }));
    const partial = spawnSync(process.execPath, [driver, "--scope", scope], {
      cwd: dir,
      encoding: "utf8",
      env,
    });
    assert.ifError(partial.error);
    assert.equal(partial.status, 1);
    assert.match(partial.stderr, /Cannot destructure property 'pkg'/);
    assert.deepEqual(fs.readFileSync(goMutation), expected);
  },
);

test("gremlins report decoding distinguishes a completed matrix entry from a renderable baseline", (t) => {
  const dir = tempDir(t);
  const folder = path.join(dir, "leg");
  fs.mkdirSync(folder);
  assert.equal(artifacts.GREMLINS_FILE, "gremlins.json");
  const file = path.join(folder, artifacts.GREMLINS_FILE);
  assert.equal(runPackages([], file), 0);
  assert.equal(fs.readFileSync(file, "utf8"), '{\n  "packages": []\n}\n');
  assert.equal(
    artifacts.encodeGremlinsReport([]),
    fs.readFileSync(file, "utf8"),
  );
  const packages = [
    {
      pkg: ".",
      baseline: { pkg: ".", ok: true as const },
      result: { files: [] },
      error: undefined,
    },
  ];
  const bytes = `{
  "packages": [
    {
      "pkg": ".",
      "baseline": {
        "pkg": ".",
        "ok": true
      },
      "result": {
        "files": []
      }
    }
  ]
}
`;
  assert.equal(artifacts.encodeGremlinsReport(packages), bytes);
  for (const raw of [
    bytes,
    '{"packages":[{"pkg":".","baseline":{"ok":false,"reason":"build failed"}}]}',
    '{"packages":[{"pkg":"./x","baseline":{"ok":true},"result":{"files":[{"file_name":"a.go","mutations":[{"status":"LIVED","line":1,"type":"ARITHMETIC_BASE"}]}]}},{"pkg":"./y","baseline":{"ok":true},"error":"stopped"}]}',
    '{"packages":[{"result":true}]}',
    '{"packages":[null]}',
    '{"packages":[]}',
    "{}",
    "false",
    "{",
    undefined,
  ]) {
    if (raw === undefined) fs.unlinkSync(file);
    else fs.writeFileSync(file, raw);
    const result = artifacts.readGremlinsReport(file);
    assert.deepEqual(
      outcome(() => artifacts.hasGremlinsResult(result)),
      outcome(() => collect(dir).entries[0]?.status === "result"),
    );
    if (!result) continue;
    const readSource = () => "a += b";
    const rendered = outcome(() =>
      gremlinsSection(JSON.parse(raw!), readSource),
    );
    const decoded = outcome(() =>
      gremlinsSection(
        {
          packages: [...artifacts.gremlinsPackages(result)].map((entry) => ({
            pkg: entry.pkg,
            baseline: { ok: entry.ok, reason: entry.reason },
            error: entry.error,
            result: entry.result,
          })),
        },
        readSource,
      ),
    );
    assert.deepEqual(decoded, rendered);
  }
});

test("Stryker reuses timing entries in input order and writes exact test-cost bytes", (t) => {
  const dir = tempDir(t);
  const scripts = fixtureScripts(dir);
  const bin = path.join(
    dir,
    "node_modules/@stryker-mutator/core/bin/stryker.js",
  );
  fs.mkdirSync(path.dirname(bin), { recursive: true });
  fs.writeFileSync(bin, "process.exit(0);");
  const input = path.join(dir, "previous.json");
  assert.equal(artifacts.TEST_COST_FILE, "test-cost.json");
  const output = path.join(
    dir,
    "reports",
    "mutation",
    artifacts.TEST_COST_FILE,
  );
  const times = [
    { file: "b", seconds: "2", extra: 7 },
    { file: "a", seconds: 0, over: "yes" },
    { file: "c", seconds: 1, over: false },
    { file: "b", seconds: 3 },
  ];
  const bytes = `{
  "limitSeconds": 2,
  "skipped": [
    "b",
    "a",
    "b"
  ],
  "times": [
    {
      "file": "b",
      "seconds": "2",
      "extra": 7
    },
    {
      "file": "a",
      "seconds": 0,
      "over": "yes"
    },
    {
      "file": "c",
      "seconds": 1,
      "over": false
    },
    {
      "file": "b",
      "seconds": 3
    }
  ]
}
`;
  for (const raw of [
    bytes,
    '{"times":[null]}',
    '{"times":1}',
    "{}",
    "null",
    "{",
    undefined,
  ]) {
    if (raw === undefined) fs.unlinkSync(input);
    else fs.writeFileSync(input, raw);
    const old = cli(
      path.join(scripts, "mutation-stryker.ts"),
      "--max-test-seconds",
      "2",
      "--test-times",
      input,
    );
    const current = outcome(() =>
      artifacts.encodeTestCost(
        artifacts.testCostReport(2, artifacts.readTestTimes(input)),
      ),
    );
    if ("error" in current) {
      assert.equal(old.status, 2, old.stderr);
      assert.equal(old.stderr.trimEnd(), current.error);
      assert.equal(fs.existsSync(output), false);
    } else {
      assert.equal(old.status, 0, old.stderr);
      assert.deepEqual(artifacts.readTestTimes(input), times);
      assert.equal(current.value, bytes);
      assert.deepEqual(fs.readFileSync(output), Buffer.from(bytes));
      assert.match(
        old.stdout,
        /3 of 4 test files left out \(reports\/mutation\/test-cost\.json\)/,
      );
    }
  }
});

test("test-cost config reads skipped before parsing the wanted list and checks membership afterward", async (t) => {
  const dir = tempDir(t);
  const scripts = fixtureScripts(dir);
  const files = ["a", "b"].map(
    (name) => `.github/tools/src/scripts/${name}.test.ts`,
  );
  for (const file of files) fs.writeFileSync(path.join(dir, file), "");
  const config: typeof import("./mutation-config.ts") = await import(
    pathToFileURL(path.join(scripts, "mutation-config.ts")).href
  );
  const report = path.join(dir, "test-cost.json");
  const previous = process.env[SKIP_TESTS_ENV];
  t.after(() => {
    if (previous === undefined) delete process.env[SKIP_TESTS_ENV];
    else process.env[SKIP_TESTS_ENV] = previous;
  });
  process.env[SKIP_TESTS_ENV] = report;
  for (const raw of [
    JSON.stringify({ skipped: [files[1], files[1], false] }),
    JSON.stringify({ skipped: `prefix:${files[0]}:suffix` }),
    '{"skipped":{}}',
    '{"skipped":null}',
    "{}",
    "null",
    "{",
    undefined,
  ]) {
    if (raw === undefined) fs.unlinkSync(report);
    else fs.writeFileSync(report, raw);
    for (const only of ["", "[", "[]"]) {
      const old = outcome(() => config.testFiles(only));
      const current = outcome(() => {
        const skipped = artifacts.readSkippedTests(report);
        const wanted = only ? new Set<string>(JSON.parse(only)) : undefined;
        return files.filter(
          (file) =>
            !artifacts.isSkippedTest(skipped, file) &&
            (!wanted || wanted.has(file)),
        );
      });
      assert.deepEqual(current, old, `report=${raw}, only=${only}`);
    }
  }
  delete process.env[SKIP_TESTS_ENV];
  assert.deepEqual(
    config.testFiles(""),
    files.filter(
      (file) =>
        !artifacts.isSkippedTest(artifacts.readSkippedTests(undefined), file),
    ),
  );
});
