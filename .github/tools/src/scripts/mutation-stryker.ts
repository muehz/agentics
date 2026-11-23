#!/usr/bin/env node

// Runs Stryker from the repository root on named files, on one area, or on the config's full scope.
//   node .github/tools/src/scripts/mutation-stryker.ts [options] [file ...]
//   node .github/tools/src/scripts/mutation-stryker.ts [options] --area <dir|tools-rest|release-rest> [--incremental <file> [--force]]
//   node .github/tools/src/scripts/mutation-stryker.ts [options] --scope <scope.json>
// Options bound the cost of a run:
//   --max-minutes <n>       stop Stryker after n minutes; the run then has no result
//   --max-test-seconds <s>  time each test file first and leave out the ones slower than s seconds
//   --test-times <file>     reuse the timings of an earlier --max-test-seconds run instead of measuring again
//   --sample <n> --seed <s> mutate about n mutants chosen from the seed instead of all of them
//   --ignore-static         skip mutants that run only while a module loads; each one reruns every test

import fs from "node:fs";
import path from "node:path";
import { spawn, spawnSync, type ChildProcess } from "node:child_process";
import { createRequire } from "node:module";
import {
  encodeTestCost,
  readStrykerScope,
  readTestTimes,
  TEST_COST_FILE,
  testCostReport,
} from "./mutation-artifacts.ts";
import {
  MUTATION_CONFIG_FILES,
  isMutationConfigFile,
} from "./mutation-config-values.ts";
import { SOURCE_ROOTS, strykerSourceGlobs } from "./mutation-source.ts";
const require = createRequire(import.meta.url);

import type { Mutant } from "@stryker-mutator/api/core";
import type { Writable } from "node:stream";
export type LocatedMutant = Pick<Mutant, "fileName" | "location">;
export type LineRanges = Record<string, readonly (readonly [number, number])[]>;
export interface SelectionOptions {
  files?: unknown;
  area?: string;
  lines?: unknown;
  ranges?: readonly string[];
  ignoreStatic?: boolean;
  incremental?: string;
  force?: boolean;
}
export interface RunOptions extends SelectionOptions {
  files: unknown;
  maxMinutes?: number;
  maxTestSeconds?: number;
  testTimes?: string;
  sample?: number;
  seed?: string;
}
export interface TestTime {
  file: string;
  seconds: number;
  over: boolean;
}
export interface StrykerResult {
  status: number;
  output: string;
  error?: Error;
  timedOut: boolean;
}
function property(value: unknown, key: PropertyKey): unknown {
  if (value === null || value === undefined)
    throw new TypeError(
      `Cannot read properties of ${value} (reading '${String(key)}')`,
    );
  return Reflect.get(Object(value), key);
}
function length(value: unknown): number {
  return Number(property(value, "length"));
}
function list(value: unknown, label: string, method = "map"): unknown[] {
  if (Array.isArray(value)) {
    const entries: unknown[] = value;
    return entries;
  }
  if (value === null || value === undefined) property(value, method);
  throw new TypeError(`${label}.${method} is not a function`);
}
function iterable(value: unknown, label: string): Iterable<unknown> {
  if (
    value !== null &&
    value !== undefined &&
    typeof property(value, Symbol.iterator) === "function"
  )
    return Reflect.apply((input: Iterable<unknown>) => input, undefined, [
      value,
    ]);
  throw new TypeError(`${label} is not iterable`);
}
function pair(value: unknown): [unknown, unknown] {
  return Reflect.apply(
    ([first, last]: Iterable<unknown>) => [first, last],
    undefined,
    [value],
  );
}
function endsWith(
  value: unknown,
  suffix: string,
  label: string,
): value is string {
  if (typeof value === "string") return value.endsWith(suffix);
  if (value === null || value === undefined) property(value, "endsWith");
  throw new TypeError(`${label}.endsWith is not a function`);
}
function startsWith(value: unknown, prefix: string): value is string {
  if (typeof value === "string") return value.startsWith(prefix);
  if (value === null || value === undefined) property(value, "startsWith");
  throw new TypeError("glob.startsWith is not a function");
}
function normalizedTarget(value: string): string {
  const file = value.replace(/:\d+(?::\d+)?-\d+(?::\d+)?$/u, "");
  const relative = path
    .relative(ROOT, path.resolve(ROOT, file))
    .split("\\")
    .join("/");
  return process.platform === "win32" ? relative.toLowerCase() : relative;
}
function configTarget(value: unknown): boolean {
  return (
    typeof value === "string" && isMutationConfigFile(normalizedTarget(value))
  );
}
function configExclusions(patterns: readonly unknown[]): string[] {
  const matches = (file: string, value: unknown): boolean => {
    if (typeof value !== "string") return false;
    try {
      return path.posix.matchesGlob(file, normalizedTarget(value));
    } catch {
      return true;
    }
  };
  return MUTATION_CONFIG_FILES.filter(
    (file) =>
      patterns.some(
        (pattern) =>
          typeof pattern === "string" &&
          !pattern.startsWith("!") &&
          matches(file, pattern),
      ) &&
      !patterns.some(
        (pattern) =>
          typeof pattern === "string" &&
          pattern.startsWith("!") &&
          matches(file, pattern.slice(1)),
      ),
  ).map((file) => `!${file}`);
}
function selected(files: unknown): unknown {
  return Array.isArray(files) && list(files, "files").some(configTarget)
    ? list(files, "files").filter((file) => !configTarget(file))
    : files;
}
function loadConfig(): typeof import("./mutation-config.ts") {
  return require("./mutation-config.ts");
}
const JSON_REPORT = "reports/mutation/stryker.json";
const TEST_COST = `reports/mutation/${TEST_COST_FILE}`;
const SAMPLE = "reports/mutation/sample.json";
// A run stopped by --max-minutes exits like timeout(1).
const TIMED_OUT = 124;
// The failure summary needs only the last ERROR lines, so the wrapper keeps the end of the output.
const OUTPUT_TAIL = 256 * 1024;

const ROOT = path.resolve(import.meta.dirname, "../../../..");
const STRYKER_BIN = path.join(
  ROOT,
  "node_modules/@stryker-mutator/core/bin/stryker.js",
);
// A "rest" area is a folder minus the subfolders that run as areas of their own. Each area is one leg of the full
// run, and a leg that cannot finish inside the job time limit saves no result, so the large folders are split.
const REST: Record<string, { dir: string; without: string[] }> = {
  "tools-rest": {
    dir: SOURCE_ROOTS[0],
    without: [
      ".github/tools/src/conformance",
      ".github/tools/src/release",
      ".github/tools/src/peer",
    ],
  },
  "release-rest": {
    dir: ".github/tools/src/release",
    without: [".github/tools/src/release/commands"],
  },
};

function restArea(area: string) {
  return Object.hasOwn(REST, area) ? REST[area] : undefined;
}

// With `lines` (file -> [[first, last]]), a file is mutated only inside its ranges, and a file with no range is left
// out. Stryker reads `file:first-last` as a line range.
function mutateTargets(files: unknown, lines?: unknown): unknown {
  const targets = selected(files);
  if (!lines) return targets;
  return list(targets, "files", "flatMap").flatMap((file) =>
    list(property(lines, String(file)) ?? [], "(lines[file] ?? [])").map(
      (range) => {
        const [first, last] = pair(range);
        return `${file}:${first}-${last}`;
      },
    ),
  );
}

function areaGlob(area: string) {
  const rest = restArea(area);
  const patterns = rest
    ? [
        ...strykerSourceGlobs(rest.dir),
        ...rest.without.map((dir) => `!${dir}/**`),
      ]
    : strykerSourceGlobs(area);
  return [...patterns, ...configExclusions(patterns)];
}

// `incremental` names the file where Stryker keeps the last run's results. With it, Stryker reuses the result of a
// mutant whose file and covering test files are unchanged, and tests only the rest.
function strykerArgs(
  options: SelectionOptions = {},
  patterns = selectionPatterns(selectionRequest(options)),
) {
  const { ignoreStatic, incremental, force } = options;
  const args = ["run"];
  if (patterns !== null) args.push("--mutate", patterns.join(","));
  if (incremental) {
    args.push("--incremental", "--incrementalFile", incremental);
    if (force) args.push("--force");
  }
  if (ignoreStatic) args.push("--ignoreStatic");
  return args;
}

interface SelectedFile {
  file: string;
  ranges: LocatedMutant["location"][] | null;
}

interface EffectiveSelection {
  files: ReadonlyMap<string, SelectedFile>;
  /** Null leaves Stryker's configured patterns in place. */
  patterns: readonly string[] | null;
  explicitlyEmpty: boolean;
}

// Stryker accepts one-based lines with optional zero-based columns in a range.
const MUTATE_RANGE = /^(.*?):(\d+)(?::(\d+))?-(\d+)(?::(\d+))?$/u;

function selectionRequest(options: SelectionOptions, scopeRequested = false) {
  const { area, lines, ranges } = options;
  const files =
    options.files === undefined && !scopeRequested ? [] : options.files;
  const named = length(files) > 0;
  const narrowed = named || (scopeRequested && Array.isArray(files));
  const requested = ranges
    ? ranges.filter((range) => !configTarget(range))
    : narrowed
      ? mutateTargets(files, lines)
      : area
        ? areaGlob(area)
        : null;
  return {
    requested,
    namedFiles: named ? files : undefined,
    explicitlyEmpty: requested !== null && length(requested) === 0,
  };
}

type SelectionRequest = ReturnType<typeof selectionRequest>;

function selectionPatterns(
  { requested, explicitlyEmpty }: SelectionRequest,
  sampling = false,
): readonly string[] | null {
  if (explicitlyEmpty) return ["!**/*"];
  if (requested === null) return null;
  if (!Array.isArray(requested)) {
    if (sampling) throw new TypeError("globs.filter is not a function");
    throw new TypeError("mutate.join is not a function");
  }
  // Stryker splits its CLI override on commas before expanding the patterns.
  return [...requested, ...configExclusions(requested.join(",").split(","))];
}

function effectiveSelection(
  request: SelectionRequest,
  configured: readonly string[],
  {
    sampling = false,
    glob = (pattern: string) => fs.globSync(pattern, { cwd: ROOT }),
  } = {},
): EffectiveSelection {
  const { namedFiles, explicitlyEmpty } = request;
  if (explicitlyEmpty) {
    return {
      files: new Map(),
      patterns: selectionPatterns(request),
      explicitlyEmpty,
    };
  }
  // Explicit scope JSON can carry non-string file entries. Keep the related
  // graph's existing diagnostic before interpreting those entries as globs.
  if (namedFiles !== undefined) {
    for (const file of iterable(namedFiles, "files")) {
      if (typeof file !== "string") endsWith(file, ".test.ts", "file");
    }
  }
  const patterns = selectionPatterns(request, sampling);
  const active =
    patterns === null
      ? list(configured, "globs", "filter")
      : patterns.join(",").split(",").filter(Boolean);
  const matches = new Map<string, SelectedFile>();
  for (const pattern of active) {
    const exclude = startsWith(pattern, "!");
    const source = exclude ? pattern.slice(1) : String(pattern);
    const match = MUTATE_RANGE.exec(source);
    const range = match
      ? {
          start: {
            line: Number(match[2]) - 1,
            column: Number(match[3] ?? 0),
          },
          end: {
            line: Number(match[4]) - 1,
            column: Number(match[5] ?? Number.MAX_SAFE_INTEGER),
          },
        }
      : null;
    for (const name of glob(normalizedTarget(source))) {
      const key = normalizedTarget(name);
      if (configTarget(name)) continue;
      if (exclude) {
        matches.delete(key);
        continue;
      }
      const previous = matches.get(key);
      if (previous?.ranges === null) continue;
      matches.set(key, {
        file: name.split("\\").join("/"),
        ranges: range ? [...(previous?.ranges ?? []), range] : null,
      });
    }
  }
  return {
    files: new Map(
      [...matches].sort(([a], [b]) => (a < b ? -1 : a > b ? 1 : 0)),
    ),
    patterns,
    explicitlyEmpty,
  };
}

function matchesSelection(
  mutant: LocatedMutant,
  selection: EffectiveSelection,
) {
  const target = selection.files.get(normalizedTarget(mutant.fileName));
  return (
    target !== undefined &&
    (target.ranges === null ||
      target.ranges.some((range) => contains(range, mutant.location)))
  );
}

function selectedFiles(selection: EffectiveSelection): string[] {
  return [...selection.files.values()].map(({ file }) => file);
}

// The source files an area mutates: the files its glob names, minus the files its `!` patterns name.
function areaSources(area: string, glob?: (pattern: string) => string[]) {
  return selectedFiles(
    effectiveSelection(selectionRequest({ area }), [], { glob }),
  );
}

function areaTestFiles(
  area: string,
  glob: (pattern: string) => string[] = (pattern) =>
    fs.globSync(pattern, { cwd: ROOT }),
) {
  const dir = restArea(area)?.dir ?? area;
  return glob(`${dir}/**/*.test.ts`);
}

// Any quoted relative path to a TypeScript or JavaScript module counts as a reference: static and dynamic imports,
// and `new URL("./x.ts", import.meta.url)`. JavaScript modules are in the graph because some tests reach their source
// through a `.mjs` runner. Counting too much only adds a test file to the run.
const REFERENCE = /["'`](\.{1,2}\/[^"'`\n]+?\.[cm]?[jt]s)["'`]/g;

function areaSourceFiles() {
  return SOURCE_ROOTS.flatMap((area) =>
    fs.globSync(`${area}/**/*.{ts,mts,cts,js,mjs,cjs}`, { cwd: ROOT }),
  ).map((file) => file.split("\\").join("/"));
}

// The test files that reach one of `files` through relative references, directly or through other files. Stryker
// records coverage only inside the test file's own process, so a test file that reaches a mutated file no other way
// cannot kill its mutants, and leaving it out of the run loses no kill.
function relatedTestFiles(
  files: Iterable<unknown>,
  all = areaSourceFiles(),
  read = (file: string) => fs.readFileSync(path.join(ROOT, file), "utf8"),
) {
  const referrers = new Map<string, Set<string>>();
  for (const file of all) {
    for (const match of read(file).matchAll(REFERENCE)) {
      const target = path.posix.join(path.posix.dirname(file), match[1]!);
      if (!referrers.has(target)) referrers.set(target, new Set());
      referrers.get(target)!.add(file);
    }
  }
  const seen = new Set(files);
  const queue = [...files];
  while (queue.length > 0) {
    for (const referrer of referrers.get(String(queue.pop())) ?? []) {
      if (seen.has(referrer)) continue;
      seen.add(referrer);
      queue.push(referrer);
    }
  }
  return [...seen]
    .filter((file): file is string => endsWith(file, ".test.ts", "file"))
    .sort();
}

// The related test files Stryker can run: the config leaves out the files in EXCLUDED_TEST_FILES. Dropping them here
// keeps the full-list fallback when every related file is excluded, so the run never starts with no test files.
function eligibleTestFiles(
  related: readonly string[],
  excluded: Readonly<Record<string, string>> = loadConfig().EXCLUDED_TEST_FILES,
) {
  return related.filter((file) => !(file in excluded));
}

function number(flag: string, value: unknown, { integer = false } = {}) {
  const parsed = Number(value);
  if (
    value === undefined ||
    !Number.isFinite(parsed) ||
    parsed <= 0 ||
    (integer && !Number.isInteger(parsed))
  )
    throw new Error(
      `${flag} needs a positive ${integer ? "integer" : "number"}`,
    );
  return parsed;
}

function parseArgs(
  argv: string[],
  readScope = (file: string) => fs.readFileSync(file, "utf8"),
): RunOptions {
  const options: RunOptions = { files: [] };
  for (let i = 0; i < argv.length; i++) {
    const arg = argv[i]!;
    if (arg === "--area") options.area = argv[++i];
    else if (arg === "--incremental") options.incremental = argv[++i];
    else if (arg === "--force") options.force = true;
    else if (arg === "--scope") {
      const { files, lines } = readStrykerScope(argv[++i]!, readScope);
      options.files = files;
      options.lines = lines;
    } else if (arg === "--max-minutes")
      options.maxMinutes = number(arg, argv[++i]);
    else if (arg === "--max-test-seconds")
      options.maxTestSeconds = number(arg, argv[++i]);
    else if (arg === "--test-times") options.testTimes = argv[++i];
    else if (arg === "--sample")
      options.sample = number(arg, argv[++i], { integer: true });
    else if (arg === "--seed") options.seed = argv[++i];
    else if (arg === "--ignore-static") options.ignoreStatic = true;
    else if (arg.startsWith("--")) throw new Error(`unknown option ${arg}`);
    else {
      const push = property(options.files, "push");
      if (typeof push !== "function")
        throw new TypeError("options.files.push is not a function");
      Reflect.apply(push, options.files, [arg]);
    }
  }
  if (options.sample !== undefined && options.seed === undefined)
    throw new Error(
      "--sample needs --seed, so the same sample can be drawn again",
    );
  if (options.testTimes && options.maxTestSeconds === undefined)
    throw new Error("--test-times needs --max-test-seconds");
  return options;
}

// mulberry32 over a 32-bit FNV-1a hash of the seed text: the same seed draws the same sample on every host.
function random(seed: unknown) {
  let state = 0x811c9dc5;
  for (const char of String(seed)) {
    state ^= char.codePointAt(0)!;
    state = Math.imul(state, 0x01000193) >>> 0;
  }
  return () => {
    state = (state + 0x6d2b79f5) >>> 0;
    let t = state;
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

function contains(
  outer: LocatedMutant["location"],
  inner: LocatedMutant["location"],
) {
  const { start: a, end: b } = outer;
  const { start: c, end: d } = inner;
  const startsBefore =
    a.line < c.line || (a.line === c.line && a.column <= c.column);
  const endsAfter =
    b.line > d.line || (b.line === d.line && b.column >= d.column);
  return startsBefore && endsAfter;
}

// Stryker mutates every mutant inside a --mutate range, so a range around one mutant also takes the mutants nested in
// it. Each pick therefore counts its nested mutants against the budget, and a pick that would overshoot is skipped.
function sampleMutants<T extends LocatedMutant>(
  input: readonly T[],
  size: number,
  seed: unknown,
) {
  const mutants = input.filter((mutant) => !configTarget(mutant.fileName));
  const next = random(seed);
  const order = mutants.map((mutant, index) => ({ mutant, index }));
  for (let i = order.length - 1; i > 0; i--) {
    const j = Math.floor(next() * (i + 1));
    [order[i], order[j]] = [order[j]!, order[i]!];
  }
  const taken = new Set<number>();
  const ranges: string[] = [];
  for (const { mutant, index } of order) {
    if (taken.size >= size) break;
    if (taken.has(index)) continue;
    const nested = mutants
      .map((other, otherIndex) => ({ other, otherIndex }))
      .filter(
        ({ other, otherIndex }) =>
          other.fileName === mutant.fileName &&
          !taken.has(otherIndex) &&
          contains(mutant.location, other.location),
      );
    if (taken.size + nested.length > size) continue;
    for (const { otherIndex } of nested) taken.add(otherIndex);
    ranges.push(rangeOf(mutant));
  }
  return { ranges: ranges.sort(), selected: taken.size };
}

// Stryker's mutant locations count lines and columns from zero; a --mutate range counts lines from one.
function rangeOf({ fileName, location: { start, end } }: LocatedMutant) {
  return `${fileName}:${start.line + 1}:${start.column}-${end.line + 1}:${end.column}`;
}

async function listMutants(files: readonly string[]) {
  const { Instrumenter } = await import("@stryker-mutator/instrumenter");
  const quiet = () => {};
  const logger = {
    isTraceEnabled: () => false,
    isDebugEnabled: () => false,
    isInfoEnabled: () => false,
    isWarnEnabled: () => true,
    isErrorEnabled: () => true,
    isFatalEnabled: () => true,
    trace: quiet,
    debug: quiet,
    info: quiet,
    warn: (...args: unknown[]) => console.warn(...args),
    error: (...args: unknown[]) => console.error(...args),
    fatal: (...args: unknown[]) => console.error(...args),
  };
  const result = await new Instrumenter(logger).instrument(
    files.map((name) => ({
      name,
      content: fs.readFileSync(path.resolve(ROOT, name), "utf8"),
      mutate: true,
    })),
    { plugins: null, excludedMutations: [], ignorers: [] },
  );
  return result.mutants;
}

// Runs one test file the way Stryker's tap runner does and reports its wall time, or `over` when it passes the limit.
function timeTestFile(
  file: string,
  limitSeconds: number,
  node = process.execPath,
): Promise<TestTime> {
  return new Promise((resolve) => {
    const started = process.hrtime.bigint();
    const child = spawn(node, ["--test-reporter=tap", file], {
      cwd: ROOT,
      stdio: "ignore",
      ...(process.platform === "win32" ? {} : { detached: true }),
    });
    const timer = setTimeout(() => {
      stopTree(child);
    }, limitSeconds * 1000);
    child.on("exit", () => {
      clearTimeout(timer);
      const seconds = Number(process.hrtime.bigint() - started) / 1e9;
      resolve({
        file,
        seconds: Math.round(seconds * 10) / 10,
        over: seconds >= limitSeconds,
      });
    });
  });
}

async function timeTestFiles(
  files: readonly string[],
  limitSeconds: number,
  concurrency: number,
  time = timeTestFile,
) {
  const results: TestTime[] = [];
  let next = 0;
  async function worker() {
    while (next < files.length)
      results.push(await time(files[next++]!, limitSeconds));
  }
  await Promise.all(Array.from({ length: Math.max(1, concurrency) }, worker));
  return results.sort((a, b) => a.file.localeCompare(b.file));
}

function stopTree(child: ChildProcess) {
  if (child.exitCode !== null || child.signalCode !== null) return;
  if (process.platform === "win32") {
    spawnSync("taskkill", ["/pid", String(child.pid), "/T", "/F"], {
      stdio: "ignore",
    });
    return;
  }
  try {
    process.kill(-child.pid!, "SIGKILL");
  } catch {
    child.kill("SIGKILL");
  }
}

// Streams Stryker's output as it runs, so a long run shows its progress, and keeps the tail for the failure summary.
function runStryker(
  args: readonly string[],
  {
    maxMs,
    env = process.env,
    bin = STRYKER_BIN,
    write = (chunk, stream) => stream.write(chunk),
  }: {
    maxMs?: number;
    env?: NodeJS.ProcessEnv;
    bin?: string;
    write?: (chunk: Buffer, stream: Writable) => unknown;
  } = {},
): Promise<StrykerResult> {
  return new Promise((resolve) => {
    let tail = "";
    let timedOut = false;
    const child = spawn(process.execPath, [bin, ...args], {
      cwd: ROOT,
      env,
      stdio: ["inherit", "pipe", "pipe"],
      // A process group lets a timeout stop Stryker together with its workers and their test processes.
      ...(process.platform === "win32" ? {} : { detached: true }),
    });
    const collect = (stream: Writable) => (chunk: Buffer) => {
      write(chunk, stream);
      tail = (tail + chunk.toString("utf8")).slice(-OUTPUT_TAIL);
    };
    // The child runs in its own process group, so Ctrl-C has to be passed on.
    const interrupt = () => stopTree(child);
    process.once("SIGINT", interrupt);
    process.once("SIGTERM", interrupt);
    child.stdout!.on("data", collect(process.stdout));
    child.stderr!.on("data", collect(process.stderr));
    const timer =
      maxMs === undefined
        ? undefined
        : setTimeout(() => {
            timedOut = true;
            stopTree(child);
          }, maxMs);
    child.on("error", (error) => {
      clearTimeout(timer);
      process.off("SIGINT", interrupt);
      process.off("SIGTERM", interrupt);
      resolve({ status: 1, output: tail, error, timedOut });
    });
    child.on("close", (code) => {
      clearTimeout(timer);
      process.off("SIGINT", interrupt);
      process.off("SIGTERM", interrupt);
      resolve({
        status: timedOut ? TIMED_OUT : (code ?? 1),
        output: tail,
        timedOut,
      });
    });
  });
}

function writeReport(name: string, text: string) {
  fs.mkdirSync(path.join(ROOT, "reports/mutation"), { recursive: true });
  fs.writeFileSync(path.join(ROOT, name), text);
}

async function main(argv: string[]) {
  const options = parseArgs(argv);
  const request = selectionRequest(options, argv.includes("--scope"));
  if (request.explicitlyEmpty) {
    // An empty --mutate would fall back to the config's full scope.
    writeReport(
      "reports/mutation/stryker-skipped.txt",
      "the changed files only lost lines; nothing to mutate\n",
    );
    console.log("the changed files only lost lines; nothing to mutate");
    return 0;
  }
  if (options.area && areaTestFiles(options.area).length === 0) {
    writeReport(
      "reports/mutation/stryker-skipped.txt",
      `no tests in ${options.area}\n`,
    );
    console.log(`no tests in ${options.area}`);
    return 0;
  }
  if (!fs.existsSync(STRYKER_BIN)) {
    writeReport(
      "reports/mutation/stryker-error.txt",
      "Stryker is not installed. Run npm ci first.\n",
    );
    console.error("Stryker is not installed. Run npm ci first.");
    return 1;
  }
  for (const name of [
    "reports/mutation/stryker-error.txt",
    JSON_REPORT,
    TEST_COST,
    SAMPLE,
  ])
    fs.rmSync(path.join(ROOT, name), { force: true });
  const configModule = await import("./mutation-config.ts");
  const sampling = options.sample !== undefined;
  // A scalar scope has no related files. It historically reports its error
  // at sampling or launch, after optional test-cost processing.
  const resolved = () =>
    effectiveSelection(request, configModule.default.mutate, { sampling });
  const selection =
    typeof request.requested === "string" ? undefined : resolved();
  // With named files or an area, the initial run and every mutant run only the test files that reach the mutated
  // files. With none found, the config's full list stays, so the run still reports instead of failing on an empty
  // test set.
  const files = selection ? selectedFiles(selection) : [];
  const related =
    selection?.patterns && files.length > 0
      ? eligibleTestFiles(
          relatedTestFiles(files),
          configModule.EXCLUDED_TEST_FILES,
        )
      : [];
  const env = { ...process.env };
  if (related.length > 0) {
    env.AGX_MUTATION_TEST_FILES = JSON.stringify(related);
    console.log(`running ${related.length} related test file(s)`);
  }

  if (options.maxTestSeconds !== undefined) {
    const times = options.testTimes
      ? readTestTimes(options.testTimes)
      : await timeTestFiles(
          // Only the test files this run would execute are worth timing.
          configModule.testFiles(env.AGX_MUTATION_TEST_FILES, []),
          options.maxTestSeconds,
          configModule.default.concurrency,
        );
    const report = testCostReport(options.maxTestSeconds, times);
    writeReport(TEST_COST, encodeTestCost(report));
    console.log(
      `test-cost limit ${options.maxTestSeconds} s: ${report.skipped.length} of ${times.length} test files left out (${TEST_COST})`,
    );
    env[configModule.SKIP_TESTS_ENV] = path.join(ROOT, TEST_COST);
  }

  const effective = selection ?? resolved();
  let launchPatterns = effective.patterns;
  if (options.sample !== undefined) {
    const mutants = (await listMutants(files)).filter((mutant) =>
      matchesSelection(mutant, effective),
    );
    const { ranges, selected } = sampleMutants(
      mutants,
      options.sample,
      options.seed,
    );
    writeReport(
      SAMPLE,
      `${JSON.stringify({ seed: options.seed, requested: options.sample, selected, total: mutants.length, ranges }, null, 2)}\n`,
    );
    console.log(
      `sample seed ${options.seed}: ${selected} of ${mutants.length} mutants (${SAMPLE})`,
    );
    if (ranges.length === 0) {
      writeReport(
        "reports/mutation/stryker-error.txt",
        "The sample selected no mutants.\n",
      );
      return 1;
    }
    launchPatterns = ranges;
  }

  const result = await runStryker(strykerArgs(options, launchPatterns), {
    env,
    maxMs:
      options.maxMinutes === undefined
        ? undefined
        : options.maxMinutes * 60_000,
  });
  const status = result.status;
  if (status === 0 && !fs.existsSync(path.join(ROOT, JSON_REPORT))) {
    // Stryker exits 0 and writes nothing when the named files hold no mutable code.
    writeReport(
      "reports/mutation/stryker-error.txt",
      "Stryker exited 0 but wrote no result file. The named files may hold no mutable code.\n",
    );
  }
  if (status !== 0) {
    // A failed run leaves its sandbox copy behind. That copy holds every SKILL.md again, and the skills conformance
    // check counts each one as a duplicate.
    fs.rmSync(path.join(ROOT, ".stryker-tmp"), {
      recursive: true,
      force: true,
    });
    const reason = result.timedOut
      ? `Stryker stopped after ${options.maxMinutes} minutes (--max-minutes); the run wrote no result.`
      : failureText(result.output, status, result.error);
    writeReport("reports/mutation/stryker-error.txt", `${reason}\n`);
    if (result.timedOut) console.error(reason);
  }
  return status;
}

if (import.meta.main)
  main(process.argv.slice(2)).then(
    (status) => process.exit(status),
    (error) => {
      console.error(error instanceof Error ? error.message : String(error));
      process.exit(2);
    },
  );

// The summary shows why a failed run has no result: the ERROR lines of the run, or the exit status when it has none.
function failureText(output: string, status: number, error?: Error) {
  // eslint-disable-next-line no-control-regex
  const plain = output.replace(/\u001b\[[0-9;]*m/g, "");
  const lines = plain.split("\n").filter((line) => /\bERROR\b/.test(line));
  if (lines.length > 0) return lines.slice(-5).join("\n");
  return error ? error.message : `stryker exited ${status}`;
}

export {
  strykerArgs,
  areaTestFiles,
  failureText,
  relatedTestFiles,
  mutateTargets,
  areaSources,
  eligibleTestFiles,
  effectiveSelection,
  matchesSelection,
  selectionRequest,
  REST,
  parseArgs,
  sampleMutants,
  rangeOf,
  random,
  listMutants,
  timeTestFile,
  timeTestFiles,
  runStryker,
  TIMED_OUT,
};
