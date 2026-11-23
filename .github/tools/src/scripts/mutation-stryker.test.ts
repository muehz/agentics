import type { TestContext } from "node:test";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import { MUTATION_CONFIG_FILES } from "./mutation-config-values.ts";
import assert from "node:assert/strict";
import path from "node:path";
import test from "node:test";
import { inCoverageScope } from "../coverage/scope.ts";
import {
  strykerArgs,
  areaTestFiles,
  failureText,
  relatedTestFiles,
  mutateTargets,
  areaSources,
  eligibleTestFiles,
  effectiveSelection as resolveSelection,
  matchesSelection,
  selectionRequest,
  type SelectionOptions,
} from "./mutation-stryker.ts";

test("named files become one comma-separated --mutate value", () => {
  assert.deepEqual(
    strykerArgs({
      files: [".github/tools/src/a.ts", ".github/tools/src/b.ts"],
    }),
    ["run", "--mutate", ".github/tools/src/a.ts,.github/tools/src/b.ts"],
  );
});

test("an area becomes its source glob minus tests and testing folders", () => {
  assert.deepEqual(strykerArgs({ area: ".github/tools/src/peer" }), [
    "run",
    "--mutate",
    ".github/tools/src/peer/**/*.ts,!.github/tools/src/peer/**/*.test.ts,!.github/tools/src/peer/**/testing/**",
  ]);
});

test("tools-rest mutates .github/tools/src minus the three areas that run alone", () => {
  assert.deepEqual(strykerArgs({ area: "tools-rest" })[2]!.split(","), [
    ".github/tools/src/**/*.ts",
    "!.github/tools/src/**/*.test.ts",
    "!.github/tools/src/**/testing/**",
    "!.github/tools/src/conformance/**",
    "!.github/tools/src/release/**",
    "!.github/tools/src/peer/**",
    ...MUTATION_CONFIG_FILES.map((file) => "!" + file),
  ]);
});

test("release-rest mutates release sources outside the commands leg", () => {
  assert.deepEqual(strykerArgs({ area: "release-rest" })[2]!.split(","), [
    ".github/tools/src/release/**/*.ts",
    "!.github/tools/src/release/**/*.test.ts",
    "!.github/tools/src/release/**/testing/**",
    "!.github/tools/src/release/commands/**",
  ]);
});

test("no file and no area runs the config's full scope", () => {
  assert.deepEqual(strykerArgs({}), ["run"]);
});

test("tools-rest looks for test files in all of .github/tools/src, and an area looks only in itself", () => {
  const seen: string[] = [];
  const glob = (pattern: string) => (seen.push(pattern), []);
  areaTestFiles("tools-rest", glob);
  areaTestFiles(".github/tools/src/peer", glob);
  assert.deepEqual(seen, [
    ".github/tools/src/**/*.test.ts",
    ".github/tools/src/peer/**/*.test.ts",
  ]);
});

test("a failed run is explained by its ERROR lines", () => {
  const out =
    "\u001b[32m12:00 INFO Stryker\u001b[39m start\n\u001b[91m12:01 ERROR DryRunExecutor\u001b[39m One or more tests failed\n\tx.test.ts\n";
  assert.equal(
    failureText(out, 1),
    "12:01 ERROR DryRunExecutor One or more tests failed",
  );
});

test("a failed run with no ERROR line is explained by its exit status or spawn error", () => {
  assert.equal(failureText("nothing useful", 3), "stryker exited 3");
  assert.equal(
    failureText("", 1, new Error("spawn npx ENOENT")),
    "spawn npx ENOENT",
  );
});

test("the dry run may take longer than Stryker's default of 5 minutes", async () => {
  const config = (await import("./mutation-config.ts")).default;
  assert.ok(config.dryRunTimeoutMinutes >= 15);
});

import fs from "node:fs";
import os from "node:os";
import {
  parseArgs,
  sampleMutants,
  rangeOf,
  listMutants,
  timeTestFiles,
  runStryker,
  TIMED_OUT,
} from "./mutation-stryker.ts";

function mutant(
  fileName: string,
  startLine: number,
  startColumn: number,
  endLine: number,
  endColumn: number,
) {
  return {
    fileName,
    location: {
      start: { line: startLine, column: startColumn },
      end: { line: endLine, column: endColumn },
    },
  };
}

const scriptDir = ".github/tools/src/scripts";
const rangeSource = `${scriptDir}/node-range.ts`;
const launcherSource = `${scriptDir}/node-test.ts`;
const smallConfig = { mutate: [rangeSource, launcherSource] };

function effectiveSelection(
  options: SelectionOptions,
  config: { mutate: string[] },
  { scopeRequested = false, sampling = false } = {},
) {
  return resolveSelection(
    selectionRequest(options, scopeRequested),
    config.mutate,
    {
      sampling,
    },
  );
}

test("effective selection compares omitted, empty and configured area requests", () => {
  const fallback = effectiveSelection({}, smallConfig);
  assert.equal(fallback.patterns, null);
  assert.equal(fallback.explicitlyEmpty, false);
  assert.deepEqual([...fallback.files.keys()], [rangeSource, launcherSource]);
  assert.deepEqual(strykerArgs({}), ["run"]);

  const empty = effectiveSelection({ files: [] }, smallConfig, {
    scopeRequested: true,
  });
  assert.equal(empty.explicitlyEmpty, true);
  assert.deepEqual(empty.patterns, ["!**/*"]);
  assert.equal(empty.files.size, 0);
  assert.deepEqual(strykerArgs({ files: [] }), ["run"]);

  const excluded = effectiveSelection(
    { files: MUTATION_CONFIG_FILES },
    smallConfig,
  );
  assert.equal(excluded.explicitlyEmpty, true);
  assert.equal(excluded.files.size, 0);
  assert.deepEqual(
    excluded.patterns,
    strykerArgs({ files: MUTATION_CONFIG_FILES })[2]!.split(","),
  );

  const area = `${scriptDir}/../scripts`;
  const actual = effectiveSelection({ area }, smallConfig);
  assert.deepEqual(actual.patterns, strykerArgs({ area })[2]!.split(","));
  assert.deepEqual([...actual.files.keys()], areaSources(scriptDir));
  for (const file of MUTATION_CONFIG_FILES) {
    assert.equal(actual.files.has(file), false);
    assert.equal(fallback.files.has(file), false);
  }
});

test("effective glob and line selections expose legacy related-test drift", () => {
  const graph = [
    `${scriptDir}/range.consumer.test.ts`,
    `${scriptDir}/launcher.consumer.test.ts`,
  ];
  const read = (file: string) =>
    file === graph[0]
      ? 'import "./node-range.ts";'
      : 'import "./node-test.ts";';
  const testsFor = (files: readonly string[]) =>
    eligibleTestFiles(relatedTestFiles(files, graph, read), {});
  const probes = [
    mutant(rangeSource, 0, 0, 1, 2),
    mutant(rangeSource, 2, 0, 2, 2),
    mutant(launcherSource, 0, 0, 0, 2),
    mutant(MUTATION_CONFIG_FILES[0]!, 0, 0, 0, 2),
  ];

  const pattern = {
    files: [
      `${scriptDir}/node-*.ts`,
      `!${scriptDir}/node-*.test.[t]s`,
      `!${launcherSource}`,
    ],
  };
  const globbed = effectiveSelection(pattern, smallConfig);
  assert.deepEqual(globbed.patterns, strykerArgs(pattern)[2]!.split(","));
  assert.deepEqual([...globbed.files.keys()], [rangeSource]);
  assert.deepEqual(testsFor(pattern.files), []);
  assert.deepEqual(testsFor([...globbed.files.keys()]), [graph[0]]);

  const lineScope = {
    files: [rangeSource, launcherSource, ...MUTATION_CONFIG_FILES],
    lines: { [rangeSource]: [[1, 2]], [MUTATION_CONFIG_FILES[0]!]: [[1, 6]] },
  };
  const ranged = effectiveSelection(lineScope, smallConfig);
  assert.deepEqual(ranged.patterns, strykerArgs(lineScope)[2]!.split(","));
  assert.deepEqual([...ranged.files.keys()], [rangeSource]);
  assert.deepEqual(testsFor(lineScope.files), [graph[1], graph[0]]);
  assert.deepEqual(testsFor([...ranged.files.keys()]), [graph[0]]);
  assert.deepEqual(
    probes.filter((probe) => matchesSelection(probe, ranged)),
    [probes[0]],
  );
  const noLines = effectiveSelection({ ...lineScope, lines: {} }, smallConfig);
  assert.equal(noLines.explicitlyEmpty, true);
  assert.equal(noLines.files.size, 0);
});

test("effective sampled ranges retain columns, exclusions and filename identity", () => {
  const options = {
    ranges: [`./${rangeSource}:1:2-1:5`, `${MUTATION_CONFIG_FILES[0]}:1:0-2:2`],
    incremental: "cache.json",
    force: true,
    ignoreStatic: true,
  };
  const selection = effectiveSelection(options, smallConfig);
  assert.deepEqual(strykerArgs(options), [
    "run",
    "--mutate",
    selection.patterns!.join(","),
    "--incremental",
    "--incrementalFile",
    "cache.json",
    "--force",
    "--ignoreStatic",
  ]);
  assert.deepEqual([...selection.files.keys()], [rangeSource]);
  assert.equal(
    matchesSelection(mutant(`./${rangeSource}`, 0, 2, 0, 5), selection),
    true,
  );
  assert.equal(
    matchesSelection(mutant(rangeSource, 0, 1, 0, 5), selection),
    false,
  );
  assert.equal(
    matchesSelection(mutant(rangeSource, 0, 2, 0, 6), selection),
    false,
  );
  assert.equal(
    matchesSelection(mutant(launcherSource, 0, 2, 0, 5), selection),
    false,
  );
  if (process.platform === "win32") {
    const windows = rangeSource.toUpperCase().replaceAll("/", "\\");
    assert.equal(
      matchesSelection(mutant(windows, 0, 2, 0, 5), selection),
      true,
    );
  }
});

test("effective selection preserves stage-specific malformed scope diagnostics", () => {
  const nonscalar = { files: [rangeSource], lines: { [rangeSource]: false } };
  const badRange = /\(lines\[file\] \?\? \[\]\)\.map is not a function/;
  assert.throws(() => strykerArgs(nonscalar), badRange);
  assert.throws(() => effectiveSelection(nonscalar, smallConfig), badRange);
  const scalar = { files: rangeSource };
  assert.throws(() => strykerArgs(scalar), /mutate\.join is not a function/);
  assert.throws(
    () => effectiveSelection(scalar, smallConfig),
    /mutate\.join is not a function/,
  );
  assert.throws(
    () => effectiveSelection(scalar, smallConfig, { sampling: true }),
    /globs\.filter is not a function/,
  );
  const invalid = [1];
  assert.throws(
    () => relatedTestFiles(invalid, []),
    /file\.endsWith is not a function/,
  );
  assert.throws(
    () => effectiveSelection({ files: invalid }, smallConfig),
    /file\.endsWith is not a function/,
  );
});

test("the cost options parse, and a bad value or an unknown option is refused", () => {
  const options = parseArgs([
    "--max-minutes",
    "20",
    "--max-test-seconds",
    "30",
    "--sample",
    "150",
    "--seed",
    "2026-W46",
    "--ignore-static",
    ".github/tools/src/peer/a.ts",
  ]);
  assert.equal(options.maxMinutes, 20);
  assert.equal(options.maxTestSeconds, 30);
  assert.equal(options.sample, 150);
  assert.equal(options.seed, "2026-W46");
  assert.equal(options.ignoreStatic, true);
  assert.deepEqual(options.files, [".github/tools/src/peer/a.ts"]);
  assert.throws(
    () => parseArgs(["--max-minutes", "0"]),
    /--max-minutes needs a positive number/,
  );
  assert.throws(
    () => parseArgs(["--sample", "1.5", "--seed", "x"]),
    /--sample needs a positive integer/,
  );
  assert.throws(() => parseArgs(["--sample", "10"]), /--sample needs --seed/);
  assert.throws(
    () => parseArgs(["--test-times", "t.json"]),
    /--test-times needs --max-test-seconds/,
  );
  assert.throws(
    () => parseArgs(["--max-minuts", "5"]),
    /unknown option --max-minuts/,
  );
});

test("the incremental options and a scope's line ranges parse beside the cost options", () => {
  const scope = JSON.stringify({
    stryker: [".github/tools/src/peer/a.ts"],
    strykerLines: { ".github/tools/src/peer/a.ts": [[3, 4]] },
  });
  const options = parseArgs(
    [
      "--scope",
      "scope.json",
      "--incremental",
      "x.json",
      "--force",
      "--max-minutes",
      "5",
    ],
    () => scope,
  );
  assert.deepEqual(options.files, [".github/tools/src/peer/a.ts"]);
  assert.deepEqual(options.lines, { ".github/tools/src/peer/a.ts": [[3, 4]] });
  assert.equal(options.incremental, "x.json");
  assert.equal(options.force, true);
  assert.equal(options.maxMinutes, 5);
  assert.deepEqual(strykerArgs({ ...options, ignoreStatic: true }), [
    "run",
    "--mutate",
    ".github/tools/src/peer/a.ts:3-4",
    "--incremental",
    "--incrementalFile",
    "x.json",
    "--force",
    "--ignoreStatic",
  ]);
});

test("a sample under line ranges draws only from the mutants inside them", () => {
  const inside = mutant(rangeSource, 2, 0, 3, 5);
  const straddling = mutant(rangeSource, 3, 0, 4, 5);
  const elsewhere = mutant(launcherSource, 2, 0, 2, 5);
  const all = [inside, straddling, elsewhere];
  const ranged = effectiveSelection(
    {
      files: [rangeSource, launcherSource],
      lines: { [rangeSource]: [[3, 4]] },
    },
    smallConfig,
  );
  assert.deepEqual(
    all.filter((probe) => matchesSelection(probe, ranged)),
    [inside],
  );
  const full = effectiveSelection({}, smallConfig);
  assert.deepEqual(
    all.filter((probe) => matchesSelection(probe, full)),
    all,
  );
});

test("sampled ranges and --ignore-static reach Stryker's arguments", () => {
  assert.deepEqual(
    strykerArgs({
      files: ["x.ts"],
      ranges: ["x.ts:1:0-1:4"],
      ignoreStatic: true,
    }),
    ["run", "--mutate", "x.ts:1:0-1:4", "--ignoreStatic"],
  );
});

test("a mutant's range counts lines from one and keeps Stryker's columns", () => {
  assert.equal(rangeOf(mutant("a.ts", 55, 16, 55, 66)), "a.ts:56:16-56:66");
});

test("the same seed draws the same sample, and another seed draws a different one", () => {
  const mutants = Array.from({ length: 200 }, (_, i) =>
    mutant("a.ts", i, 0, i, 5),
  );
  const first = sampleMutants(mutants, 20, "seed-a");
  assert.equal(first.selected, 20);
  assert.deepEqual(sampleMutants(mutants, 20, "seed-a"), first);
  assert.notDeepEqual(
    sampleMutants(mutants, 20, "seed-b").ranges,
    first.ranges,
  );
});

test("a range around a mutant counts the mutants nested in it, and the sample never exceeds its budget", () => {
  // One outer mutant holds ten inner ones; picking it alone would take eleven mutants.
  const outer = mutant("a.ts", 0, 0, 20, 0);
  const inner = Array.from({ length: 10 }, (_, i) =>
    mutant("a.ts", i + 1, 2, i + 1, 8),
  );
  const other = mutant("b.ts", 0, 0, 20, 0);
  for (const seed of ["1", "2", "3", "4", "5", "6", "7", "8"]) {
    const { ranges, selected } = sampleMutants(
      [outer, ...inner, other],
      5,
      seed,
    );
    assert.equal(selected, 5, seed);
    assert.ok(!ranges.includes("a.ts:1:0-21:0"), seed);
  }
  const all = sampleMutants([outer, ...inner, other], 12, "1");
  assert.equal(all.selected, 12);
});

test("the instrumenter lists a file's mutants with zero-based locations", async (t) => {
  const file = path.join(tempDir(t), "sample.ts");
  fs.writeFileSync(
    file,
    "export function ready(): boolean {\n  return true;\n}\n",
  );
  const mutants = await listMutants([file]);
  const negation = mutants.find((m) => m.mutatorName === "BooleanLiteral");
  assert.ok(negation, JSON.stringify(mutants.map((m) => m.mutatorName)));
  assert.deepEqual(negation.location.start, { line: 1, column: 9 });
});

function tempDir(t: TestContext) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "mutation-stryker-"));
  t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
  return dir;
}

// A process that is never stopped would hang the test, so each one fails at a deadline instead.
test(
  "test files slower than the limit are marked over and stopped at the limit",
  { timeout: 30000 },
  async (t) => {
    const dir = tempDir(t);
    const fast = path.join(dir, "fast.test.cjs");
    const slow = path.join(dir, "slow.test.cjs");
    fs.writeFileSync(fast, 'require("node:test")("fast", () => {});\n');
    fs.writeFileSync(
      slow,
      'require("node:test")("slow", () => new Promise((r) => setTimeout(r, 60000)));\n',
    );
    const started = Date.now();
    const times = await timeTestFiles([fast, slow], 3, 2);
    assert.ok(Date.now() - started < 30000, "the slow file ran past its limit");
    assert.deepEqual(
      times.map((entry) => [path.basename(entry.file), entry.over]),
      [
        ["fast.test.cjs", false],
        ["slow.test.cjs", true],
      ],
    );
  },
);

test("the config leaves out the test files a test-cost report skipped", async () => {
  const { testFiles } = await import("./mutation-config.ts");
  const all = testFiles("", []);
  const skipped = all[0];
  assert.ok(skipped);
  assert.ok(!testFiles("", [skipped]).includes(skipped));
  assert.equal(testFiles("", [skipped]).length, all.length - 1);
  // A skipped file stays out even when the related-file list names it.
  assert.deepEqual(testFiles(JSON.stringify([skipped, all[1]]), [skipped]), [
    all[1],
  ]);
});

test(
  "Stryker's output streams while it runs",
  { timeout: 30000 },
  async (t) => {
    const bin = path.join(tempDir(t), "fake-stryker.cjs");
    fs.writeFileSync(
      bin,
      'console.log("progress 1/2"); setTimeout(() => console.log("done"), 1500);\n',
    );
    let firstChunkAt: number | undefined;
    const result = await runStryker([], {
      bin,
      write: () => (firstChunkAt ??= Date.now()),
    });
    const exitedAt = Date.now();
    assert.equal(result.status, 0);
    assert.match(result.output, /progress 1\/2[\s\S]*done/);
    assert.ok(
      exitedAt - firstChunkAt! >= 1000,
      "output arrived only when the process exited",
    );
  },
);

test(
  "--max-minutes stops Stryker and the processes it started",
  { timeout: 30000 },
  async (t) => {
    const dir = tempDir(t);
    const pidFile = path.join(dir, "worker.pid");
    const bin = path.join(dir, "fake-stryker.cjs");
    fs.writeFileSync(
      bin,
      `const { spawn } = require("node:child_process");
const worker = spawn(process.execPath, ["-e", "setInterval(() => {}, 1000)"], { stdio: "ignore" });
require("node:fs").writeFileSync(${JSON.stringify(pidFile)}, String(worker.pid));
setInterval(() => {}, 1000);
`,
    );
    const started = Date.now();
    const result = await runStryker([], { bin, maxMs: 2000, write: () => {} });
    assert.ok(Date.now() - started < 20000, "the run was not stopped");
    assert.equal(result.timedOut, true);
    assert.equal(result.status, TIMED_OUT);
    const pid = Number(fs.readFileSync(pidFile, "utf8"));
    let alive = true;
    for (let i = 0; i < 50 && alive; i++) {
      try {
        process.kill(pid, 0);
        await new Promise((r) => setTimeout(r, 100));
      } catch {
        alive = false;
      }
    }
    assert.equal(alive, false, "Stryker's worker outlived the timeout");
  },
);
test("negative: Stryker leaves HTML templates alone when it disables type checks", async () => {
  const { DISABLE_TYPE_CHECKS, default: config } =
    await import("./mutation-config.ts");
  assert.equal(config.disableTypeChecks, DISABLE_TYPE_CHECKS);
  const matches = (file: string) =>
    path.posix.matchesGlob(file, DISABLE_TYPE_CHECKS);
  assert.equal(
    matches("/w/tools/src/peer/report/html/templates/view.html"),
    false,
  );
  assert.equal(matches("/w/tools/src/peer/cli/verbs/view.ts"), true);
  assert.equal(matches("/w/tools/src/peer/a/b.ts"), true);
});

const GRAPH: Record<string, string> = {
  ".github/tools/src/peer/a.ts": "",
  ".github/tools/src/peer/b.ts": 'import { a } from "./a.ts";',
  ".github/tools/src/peer/b.test.ts": 'import { b } from "./b.ts";',
  ".github/tools/src/peer/sub/url.test.ts":
    'const mod = new URL("../a.ts", import.meta.url);',
  ".github/tools/src/peer/other.ts": "",
  ".github/tools/src/peer/other.test.ts": 'import { o } from "./other.ts";',
  ".github/tools/src/a.test.ts": 'import { a } from "./a.ts";',
};

test("a test file is related when it reaches a mutated file directly, through another file, or by a URL", () => {
  assert.deepEqual(
    relatedTestFiles(
      [".github/tools/src/peer/a.ts"],
      Object.keys(GRAPH),
      (file) => GRAPH[file]!,
    ),
    [
      ".github/tools/src/peer/b.test.ts",
      ".github/tools/src/peer/sub/url.test.ts",
    ],
  );
});

test("a test file that reaches no mutated file is left out, and so is a same-named file in another folder", () => {
  const related = relatedTestFiles(
    [".github/tools/src/peer/b.ts"],
    Object.keys(GRAPH),
    (file) => GRAPH[file]!,
  );
  assert.deepEqual(related, [".github/tools/src/peer/b.test.ts"]);
});

test("the config keeps only the named test files, and all of them when none is named", async () => {
  const { testFiles } = await import("./mutation-config.ts");
  const all = testFiles("");
  assert.ok(all.length > 100);
  assert.deepEqual(testFiles(JSON.stringify([all[3], "no/such.test.ts"])), [
    all[3],
  ]);
});

test("with line ranges, each range is one file:first-last target and a file with none is left out", () => {
  assert.deepEqual(
    strykerArgs({
      files: [".github/tools/src/peer/a.ts", ".github/tools/src/peer/b.ts"],
      lines: {
        ".github/tools/src/peer/a.ts": [
          [3, 3],
          [11, 14],
        ],
        ".github/tools/src/peer/b.ts": [],
      },
    }),
    [
      "run",
      "--mutate",
      ".github/tools/src/peer/a.ts:3-3,.github/tools/src/peer/a.ts:11-14",
    ],
  );
});

test("without line ranges, named files are mutated whole", () => {
  assert.deepEqual(mutateTargets([".github/tools/src/peer/a.ts"], undefined), [
    ".github/tools/src/peer/a.ts",
  ]);
  assert.deepEqual(strykerArgs({ files: ["a.ts"], lines: {} }), [
    "run",
    "--mutate",
    "!**/*",
  ]);
});

test("an area's sources are its glob minus the files its exclusions name", () => {
  const tree: Record<string, string[]> = {
    ".github/tools/src/**/*.ts": [
      ".github/tools/src/a.ts",
      ".github/tools/src/a.test.ts",
      ".github/tools/src/testing/h.ts",
      ".github/tools/src/peer/p.ts",
    ],
    ".github/tools/src/**/*.test.ts": [".github/tools/src/a.test.ts"],
    ".github/tools/src/**/testing/**": [".github/tools/src/testing/h.ts"],
    ".github/tools/src/peer/**": [".github/tools/src/peer/p.ts"],
  };
  assert.deepEqual(
    areaSources("tools-rest", (pattern) => tree[pattern] ?? []),
    [".github/tools/src/a.ts"],
  );
});

test("an incremental file adds the incremental flags, and --force only with it", () => {
  assert.deepEqual(
    strykerArgs({
      area: ".github/tools/src/peer",
      incremental: "x.json",
    }).slice(3),
    ["--incremental", "--incrementalFile", "x.json"],
  );
  assert.deepEqual(
    strykerArgs({
      area: ".github/tools/src/peer",
      incremental: "x.json",
      force: true,
    }).slice(3),
    ["--incremental", "--incrementalFile", "x.json", "--force"],
  );
  assert.deepEqual(
    strykerArgs({ area: ".github/tools/src/peer", force: true }),
    [
      "run",
      "--mutate",
      ".github/tools/src/peer/**/*.ts,!.github/tools/src/peer/**/*.test.ts,!.github/tools/src/peer/**/testing/**",
    ],
  );
});

test("a test file that reaches a mutated file only through a JavaScript runner module is related", () => {
  const graph: Record<string, string> = {
    ".github/tools/src/peer/process/windows-containment.ts": "",
    ".github/tools/src/peer/process/testing/worker.mjs":
      'import { run } from "../windows-containment.ts";',
    ".github/tools/src/peer/process/testing/runner.mjs":
      'spawn(new URL("./worker.mjs", import.meta.url));',
    ".github/tools/src/peer/process/testing/windows-containment.test.ts":
      'const runner = new URL("./runner.mjs", import.meta.url);',
    ".github/tools/src/peer/process/testing/helper.cjs":
      'require("./unrelated.cjs");',
    ".github/tools/src/peer/process/testing/other.test.ts":
      'require("./helper.cjs");',
  };
  assert.deepEqual(
    relatedTestFiles(
      [".github/tools/src/peer/process/windows-containment.ts"],
      Object.keys(graph),
      (file) => graph[file]!,
    ),
    [".github/tools/src/peer/process/testing/windows-containment.test.ts"],
  );
});

test("related test files the config excludes are dropped, so an all-excluded selection is empty", () => {
  const excluded = { ".github/tools/src/release/plugin.test.ts": "reason" };
  assert.deepEqual(
    eligibleTestFiles(
      [
        ".github/tools/src/release/plugin.test.ts",
        ".github/tools/src/release/args.test.ts",
      ],
      excluded,
    ),
    [".github/tools/src/release/args.test.ts"],
  );
  assert.deepEqual(
    eligibleTestFiles([".github/tools/src/release/plugin.test.ts"], excluded),
    [],
  );
});

test("every file the config excludes is dropped from a related selection by default", async () => {
  const { EXCLUDED_TEST_FILES, testFiles } =
    await import("./mutation-config.ts");
  const excluded = Object.keys(EXCLUDED_TEST_FILES);
  assert.ok(excluded.length > 0);
  assert.deepEqual(eligibleTestFiles(excluded), []);
  const kept = testFiles("")[0];
  assert.deepEqual(eligibleTestFiles([...excluded, kept!]), [kept!]);
});

test("configuration files are excluded from named and ranged mutation selections", () => {
  assert.deepEqual(mutateTargets(MUTATION_CONFIG_FILES), []);
  assert.deepEqual(
    mutateTargets([".github/tools/src/scripts/../scripts/mutation-config.ts"]),
    [],
  );
  if (process.platform === "win32")
    assert.deepEqual(
      mutateTargets(MUTATION_CONFIG_FILES.map((file) => file.toUpperCase())),
      [],
    );
  const broad = strykerArgs({ files: [".github/tools/src/**/*.ts"] })[2]!.split(
    ",",
  );
  for (const file of MUTATION_CONFIG_FILES)
    assert.ok(broad.includes(`!${file}`));
  assert.deepEqual(
    mutateTargets([...MUTATION_CONFIG_FILES, ".github/tools/src/real.ts"], {
      [MUTATION_CONFIG_FILES[0]!]: [[1, 9]],
      ".github/tools/src/real.ts": [[2, 3]],
    }),
    [".github/tools/src/real.ts:2-3"],
  );
  assert.deepEqual(strykerArgs({ files: [...MUTATION_CONFIG_FILES] }), [
    "run",
    "--mutate",
    "!**/*",
  ]);
  assert.deepEqual(
    strykerArgs({ ranges: [`${MUTATION_CONFIG_FILES[0]}:1:0-1:2`] }),
    ["run", "--mutate", "!**/*"],
  );
});

test("tools and tools-rest selections omit configuration sources", () => {
  for (const area of [
    ".github/tools/src",
    "tools-rest",
    ".github/tools/src/scripts",
  ]) {
    const prefix =
      area === ".github/tools/src/scripts" ? area : ".github/tools/src";
    const glob = (pattern: string) =>
      pattern === `${prefix}/**/*.ts`
        ? [...MUTATION_CONFIG_FILES, ".github/tools/src/scripts/actual.ts"]
        : [];
    assert.deepEqual(areaSources(area, glob), [
      ".github/tools/src/scripts/actual.ts",
    ]);
    for (const file of MUTATION_CONFIG_FILES)
      assert.ok(strykerArgs({ area })[2]!.split(",").includes(`!${file}`));
  }
});

test("Stryker excludes its source policy from every selection and coverage retains it", async () => {
  const owner = ".github/tools/src/scripts/mutation-source.ts";
  const artifact = ".github/tools/src/scripts/mutation-artifacts.ts";
  const { default: config } = await import("./mutation-config.ts");
  const relevant = (files: readonly string[]) =>
    files.filter((file) => file === owner || file === artifact);
  const area = "tools-rest";
  assert.deepEqual(
    {
      defaultExcludesOwner: config.mutate.includes(`!${owner}`),
      defaultFiles: relevant([...effectiveSelection({}, config).files.keys()]),
      namedArgs: strykerArgs({ files: [owner, artifact] }),
      areaExcludesOwner: strykerArgs({ area })[2]!
        .split(",")
        .includes(`!${owner}`),
      areaFiles: relevant(areaSources(area)),
      coverageIncludesOwner: inCoverageScope(owner),
    },
    {
      defaultExcludesOwner: true,
      defaultFiles: [artifact],
      namedArgs: ["run", "--mutate", artifact],
      areaExcludesOwner: true,
      areaFiles: [artifact],
      coverageIncludesOwner: true,
    },
  );
});

test("sampling and file enumeration cannot reintroduce configuration", () => {
  const excluded = MUTATION_CONFIG_FILES.map((file) =>
    mutant(file, 0, 0, 0, 2),
  );
  assert.deepEqual(sampleMutants(excluded, 2, "config"), {
    ranges: [],
    selected: 0,
  });
  const requested = effectiveSelection(
    { files: MUTATION_CONFIG_FILES },
    { mutate: [".github/tools/src/**/*.ts"] },
  );
  assert.equal(requested.explicitlyEmpty, true);
  assert.equal(requested.files.size, 0);
  const fallback = effectiveSelection(
    {},
    { mutate: [...MUTATION_CONFIG_FILES] },
  );
  assert.equal(fallback.files.size, 0);
  assert.deepEqual(
    excluded.filter((probe) => matchesSelection(probe, fallback)),
    [],
  );
});

test("sampled scalar scope CLI rejects before enumerating the configured scope", (t) => {
  const fixture = tempDir(t);
  const scripts = path.join(fixture, ".github/tools/src/scripts");
  fs.mkdirSync(scripts, { recursive: true });
  fs.writeFileSync(
    path.join(fixture, ".github/tools/package.json"),
    '{"type":"module"}',
  );
  for (const file of [
    "mutation-stryker.ts",
    "mutation-config-values.ts",
    "mutation-artifacts.ts",
    "mutation-source.ts",
  ])
    fs.copyFileSync(
      fileURLToPath(new URL(file, import.meta.url)),
      path.join(scripts, file),
    );
  fs.writeFileSync(
    path.join(scripts, "mutation-config.ts"),
    'export const EXCLUDED_TEST_FILES = {}; export function testFiles() { return []; } export default { mutate: ["target.ts"] };',
  );
  const stryker = path.join(
    fixture,
    "node_modules/@stryker-mutator/core/bin/stryker.js",
  );
  fs.mkdirSync(path.dirname(stryker), { recursive: true });
  fs.writeFileSync(stryker, "process.exit(0);");
  fs.writeFileSync(
    path.join(fixture, "target.ts"),
    "export const target = true;",
  );
  const marker = path.join(fixture, "enumerated.txt");
  const preload = path.join(fixture, "preload.cjs");
  fs.writeFileSync(
    preload,
    `const fs = require("node:fs");
const original = fs.globSync;
fs.globSync = (...args) => { if (Array.isArray(args[0]) && args[0].includes("target.ts")) fs.writeFileSync(${JSON.stringify(marker)}, "enumerated"); return original(...args); };
require("node:module").syncBuiltinESMExports();`,
  );
  const scope = path.join(fixture, "scope.json");
  fs.writeFileSync(scope, JSON.stringify({ stryker: "x.ts" }));
  const result = spawnSync(
    process.execPath,
    [
      "--require",
      preload,
      path.join(scripts, "mutation-stryker.ts"),
      "--scope",
      scope,
      "--sample",
      "1",
      "--seed",
      "demo",
    ],
    {
      encoding: "utf8",
      timeout: 10_000,
      env: {
        ...process.env,
        AGX_STRYKER_TEST_COST: "",
        AGX_MUTATION_TEST_FILES: "",
      },
    },
  );
  assert.equal(result.status, 2, result.stderr);
  assert.match(result.stderr, /globs\.filter is not a function/);
  assert.equal(fs.existsSync(marker), false);
});

test("the CLI launches Stryker with resolved test selection and samples only its selected ranges", (t) => {
  const root = tempDir(t);
  const scripts = path.join(root, scriptDir);
  fs.mkdirSync(scripts, { recursive: true });
  fs.writeFileSync(path.join(root, "package.json"), '{"type":"module"}');
  for (const file of [
    "mutation-stryker.ts",
    "mutation-config-values.ts",
    "mutation-artifacts.ts",
    "mutation-source.ts",
  ])
    fs.copyFileSync(
      fileURLToPath(new URL(file, import.meta.url)),
      path.join(scripts, file),
    );
  const targetDir = ".github/tools/src/fixture";
  fs.mkdirSync(path.join(root, targetDir));
  const first = `${targetDir}/first.ts`;
  const second = `${targetDir}/second.ts`;
  const firstTest = `${targetDir}/first.test.ts`;
  const secondTest = `${targetDir}/second.test.ts`;
  fs.writeFileSync(path.join(root, first), "export const first = true;");
  fs.writeFileSync(path.join(root, second), "export const second = true;");
  fs.writeFileSync(path.join(root, firstTest), 'import "./first.ts";');
  fs.writeFileSync(path.join(root, secondTest), 'import "./second.ts";');
  fs.writeFileSync(
    path.join(scripts, "mutation-config.ts"),
    `export const SKIP_TESTS_ENV = "AGX_STRYKER_TEST_COST";
export const EXCLUDED_TEST_FILES = { ${JSON.stringify(secondTest)}: "fixture exclusion" };
export default { mutate: ${JSON.stringify([first, second])} };`,
  );

  const instrumenterDir = path.join(
    root,
    "node_modules/@stryker-mutator/instrumenter",
  );
  fs.mkdirSync(instrumenterDir, { recursive: true });
  fs.writeFileSync(
    path.join(instrumenterDir, "package.json"),
    '{"type":"module","exports":"./index.js"}',
  );
  fs.writeFileSync(
    path.join(instrumenterDir, "index.js"),
    `import fs from "node:fs";
export class Instrumenter {
  instrument(files) {
    fs.writeFileSync("instrumented.json", JSON.stringify(files.map(({name}) => name)));
    return { mutants: files.flatMap(({name}) =>
      [0, 4].map(line => ({
        fileName: name,
        location: { start: { line, column: 0 }, end: { line, column: 2 } },
      }))) };
  }
}`,
  );
  const runner = path.join(
    root,
    "node_modules/@stryker-mutator/core/bin/stryker.js",
  );
  fs.mkdirSync(path.dirname(runner), { recursive: true });
  fs.writeFileSync(
    runner,
    `const fs = require("node:fs");
const costPath = process.env.AGX_STRYKER_TEST_COST;
fs.writeFileSync("launch.json", JSON.stringify({
  args: process.argv.slice(2),
  related: process.env.AGX_MUTATION_TEST_FILES,
  cost: costPath ? JSON.parse(fs.readFileSync(costPath, "utf8")) : null,
}));
fs.mkdirSync("reports/mutation", { recursive: true });
fs.writeFileSync("reports/mutation/stryker.json", "{}");`,
  );
  const launch = path.join(root, "launch.json");
  const invoke = (...args: string[]) => {
    fs.rmSync(launch, { force: true });
    const result = spawnSync(
      process.execPath,
      [path.join(scripts, "mutation-stryker.ts"), ...args],
      {
        cwd: root,
        env: {
          ...process.env,
          AGX_MUTATION_TEST_FILES: "[]",
          AGX_STRYKER_TEST_COST: "",
        },
        encoding: "utf8",
        timeout: 10_000,
      },
    );
    assert.ifError(result.error);
    return result;
  };
  const run = (...args: string[]) => {
    const result = invoke(...args);
    assert.equal(result.status, 0, result.stderr);
    return result.stdout;
  };
  const invoked = () => JSON.parse(fs.readFileSync(launch, "utf8"));

  const patterns = [
    `${targetDir}/*.ts`,
    `!${targetDir}/*.test.[t]s`,
    `!${second}`,
  ];
  run(...patterns);
  assert.deepEqual(invoked(), {
    args: ["run", "--mutate", patterns.join(",")],
    related: JSON.stringify([firstTest]),
    cost: null,
  });

  const scope = path.join(root, "scope.json");
  fs.writeFileSync(
    scope,
    JSON.stringify({
      stryker: [first, second, MUTATION_CONFIG_FILES[0]],
      strykerLines: {
        [first]: [[1, 1]],
        [MUTATION_CONFIG_FILES[0]!]: [[1, 6]],
      },
    }),
  );
  const times = [{ file: firstTest, seconds: 2, over: false }];
  const timesFile = path.join(root, "times.json");
  fs.writeFileSync(timesFile, JSON.stringify({ times }));
  assert.match(
    run(
      "--scope",
      scope,
      "--sample",
      "1",
      "--seed",
      "stable",
      "--max-test-seconds",
      "2",
      "--test-times",
      timesFile,
      "--incremental",
      "cache.json",
      "--force",
      "--ignore-static",
    ),
    /test-cost limit 2 s:[\s\S]*sample seed stable: 1 of 1 mutants/,
  );
  const sampledRange = `${first}:1:0-1:2`;
  assert.deepEqual(invoked(), {
    args: [
      "run",
      "--mutate",
      sampledRange,
      "--incremental",
      "--incrementalFile",
      "cache.json",
      "--force",
      "--ignoreStatic",
    ],
    related: JSON.stringify([firstTest]),
    cost: { limitSeconds: 2, skipped: [firstTest], times },
  });
  assert.deepEqual(
    JSON.parse(fs.readFileSync(path.join(root, "instrumented.json"), "utf8")),
    [first],
  );
  assert.deepEqual(
    JSON.parse(
      fs.readFileSync(path.join(root, "reports/mutation/sample.json"), "utf8"),
    ),
    {
      seed: "stable",
      requested: 1,
      selected: 1,
      total: 1,
      ranges: [sampledRange],
    },
  );

  run(second);
  assert.deepEqual(invoked(), {
    args: ["run", "--mutate", second],
    related: "[]",
    cost: null,
  });
  run();
  assert.deepEqual(invoked(), { args: ["run"], related: "[]", cost: null });
  const skipped = path.join(root, "reports/mutation/stryker-skipped.txt");
  const instrumented = path.join(root, "instrumented.json");
  fs.rmSync(instrumented);
  fs.mkdirSync(path.join(root, "toString"));
  for (const area of [`${targetDir}/empty`, "constructor", "toString"]) {
    fs.rmSync(skipped, { force: true });
    assert.equal(run("--area", area), `no tests in ${area}\n`);
    assert.deepEqual(
      fs.readFileSync(skipped),
      Buffer.from(`no tests in ${area}\n`),
    );
    assert.equal(fs.existsSync(launch), false);
    assert.equal(fs.existsSync(instrumented), false);
  }
  fs.writeFileSync(scope, '{"stryker":[]}');
  assert.match(run("--scope", scope), /nothing to mutate/);
  assert.equal(fs.existsSync(launch), false);
  fs.writeFileSync(scope, '{"stryker":"bad.ts"}');
  const malformed = invoke(
    "--scope",
    scope,
    "--max-test-seconds",
    "2",
    "--test-times",
    timesFile,
  );
  assert.equal(malformed.status, 2);
  assert.equal(malformed.stderr.trimEnd(), "mutate.join is not a function");
  assert.match(malformed.stdout, /test-cost limit 2 s:/);
  assert.equal(fs.existsSync(launch), false);
});

test("an excluded-only or empty scope CLI run skips before configuration or Stryker loads", (t) => {
  const fixture = tempDir(t);
  const scripts = path.join(fixture, ".github/tools/src/scripts");
  fs.mkdirSync(scripts, { recursive: true });
  fs.writeFileSync(
    path.join(fixture, ".github/tools/package.json"),
    '{"type":"module"}',
  );
  for (const file of [
    "mutation-stryker.ts",
    "mutation-config-values.ts",
    "mutation-artifacts.ts",
    "mutation-source.ts",
  ])
    fs.copyFileSync(
      fileURLToPath(new URL(file, import.meta.url)),
      path.join(scripts, file),
    );
  fs.writeFileSync(
    path.join(scripts, "mutation-config.ts"),
    'throw new Error("config must not load");',
  );
  const scope = path.join(fixture, "scope.json");
  const cases = [
    [...MUTATION_CONFIG_FILES],
    ["--sample", "1", "--seed", "config", ...MUTATION_CONFIG_FILES],
  ];
  for (const stryker of [[], [...MUTATION_CONFIG_FILES]]) {
    fs.writeFileSync(scope, JSON.stringify({ stryker }));
    cases.push(["--scope", scope]);
    const r = spawnSync(
      process.execPath,
      [path.join(scripts, "mutation-stryker.ts"), "--scope", scope],
      {
        encoding: "utf8",
        env: {
          ...process.env,
          AGX_MUTATION_TEST_FILES: "malformed",
          AGX_STRYKER_TEST_COST: path.join(fixture, "missing.json"),
        },
      },
    );
    assert.equal(r.status, 0, r.stderr);
    assert.match(r.stdout, /nothing to mutate/);
    assert.equal(r.stderr, "");
  }
  for (const args of cases) {
    const r = spawnSync(
      process.execPath,
      [path.join(scripts, "mutation-stryker.ts"), ...args],
      { encoding: "utf8" },
    );
    assert.equal(r.status, 0, r.stderr);
    assert.match(r.stdout, /nothing to mutate/);
    assert.equal(r.stderr, "");
  }
  fs.writeFileSync(scope, JSON.stringify({ stryker: "" }));
  const scalar = spawnSync(
    process.execPath,
    [path.join(scripts, "mutation-stryker.ts"), "--scope", scope],
    { encoding: "utf8" },
  );
  assert.equal(scalar.status, 1, scalar.stderr);
  assert.equal(scalar.stdout, "");
  assert.equal(scalar.stderr, "Stryker is not installed. Run npm ci first.\n");
});
