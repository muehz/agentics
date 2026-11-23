import { readFileSync } from "node:fs";

export const SCOPE_FILE = "scope.json";
export const GREMLINS_FILE = "gremlins.json";
export const TEST_COST_FILE = "test-cost.json";

type Json = null | boolean | number | string | Json[] | { [key: string]: Json };
type Field = Json | undefined;

/** The scope produced from the changed repository paths. */
export interface MutationScope {
  stryker: string[];
  gremlins: { pkg: string; changed: string[] }[];
  strykerLines?: Record<string, [number, number][]>;
}

export interface GremlinsPackage {
  pkg: unknown;
  baseline:
    { pkg: unknown; ok: true } | { pkg: unknown; ok: false; reason: string };
  result?: unknown;
  error?: string;
}

const readText = (file: string) => readFileSync(file, "utf8");
function readJson(file: string, read = readText): Json {
  return JSON.parse(read(file));
}

// Report generation treats missing or unreadable JSON as a run with no result.
function readReportJson(file: string | undefined): Json {
  if (file === undefined) return null;
  try {
    return readJson(file);
  } catch {
    return null;
  }
}

// Existing files can contain scalar values. Preserve JS property access and the
// consuming operation's diagnostic instead of rejecting the entire artifact.
function field(value: unknown, name: string): Field {
  if (value === null || value === undefined)
    throw new TypeError(
      `Cannot read properties of ${value} (reading '${name}')`,
    );
  return Reflect.get(Object(value), name);
}

function encode(value: unknown): string {
  return `${JSON.stringify(value, null, 2)}\n`;
}

export function encodeScope(scope: MutationScope): string {
  return encode(scope);
}

export function readStrykerScope(file: string, read = readText) {
  const scope = readJson(file, read);
  return {
    files: field(scope, "stryker"),
    lines: field(scope, "strykerLines"),
  };
}

interface GremlinsTarget {
  pkg?: Field;
  changed?: Field;
}

export function readGremlinsTargets(file: string): Iterable<GremlinsTarget> {
  const parsed = readJson(file);
  // Destructuring a null scope has its own CLI diagnostic.
  const { gremlins } = parsed as { gremlins?: Json };
  if (!Array.isArray(gremlins) && typeof gremlins !== "string")
    throw new TypeError("targets is not iterable");
  // Preserve partial progress if a later target cannot be destructured.
  function* targets(entries: Iterable<Json>) {
    for (const { pkg, changed } of entries as Iterable<GremlinsTarget>)
      yield { pkg, changed };
  }
  return targets(gremlins);
}

export function readReportScope(file: string | undefined) {
  const scope = readReportJson(file);
  if (!scope) return null;
  const hasTargets = (name: string) =>
    Number(field(field(scope, name), "length")) > 0;
  return {
    get stryker() {
      return hasTargets("stryker");
    },
    get gremlins() {
      return hasTargets("gremlins");
    },
  };
}

export function encodeGremlinsReport(
  packages: readonly GremlinsPackage[],
): string {
  return encode({ packages });
}

export function readGremlinsReport(file: string | undefined) {
  const report = readReportJson(file);
  return report ? { packages: field(report, "packages") } : null;
}

type GremlinsReport = NonNullable<ReturnType<typeof readGremlinsReport>>;

export function hasGremlinsResult(report: GremlinsReport | null): boolean {
  const packages = report?.packages;
  return (
    Array.isArray(packages) &&
    packages.length > 0 &&
    packages.every((entry) => field(entry, "result"))
  );
}

// A matrix entry only checks for result presence. The summary consumes the
// baseline of each package, including those without a result.
export function* gremlinsPackages(report: GremlinsReport) {
  const packages = report.packages;
  if (!Array.isArray(packages) && typeof packages !== "string")
    throw new TypeError("combined.packages is not iterable");
  for (const entry of packages) {
    const baseline = field(entry, "baseline");
    yield {
      pkg: field(entry, "pkg"),
      ok: field(baseline, "ok"),
      reason: field(baseline, "reason"),
      error: field(entry, "error"),
      result: field(entry, "result"),
    };
  }
}

export function readTestTimes(file: string): Json[] {
  const times = field(readJson(file), "times");
  if (Array.isArray(times)) return times;
  if (times === undefined || times === null) field(times, "filter");
  throw new TypeError("times.filter is not a function");
}

export function testCostReport(
  limitSeconds: number,
  times: readonly unknown[],
) {
  const skipped = times
    .filter(
      (entry) =>
        field(entry, "over") || Number(field(entry, "seconds")) >= limitSeconds,
    )
    .map((entry) => field(entry, "file"));
  return { limitSeconds, skipped, times };
}

export function encodeTestCost(
  report: ReturnType<typeof testCostReport>,
): string {
  return encode(report);
}

export function readSkippedTests(file: string | undefined): Field {
  return file ? field(readJson(file), "skipped") : [];
}

export function isSkippedTest(skipped: unknown, file: string): boolean {
  if (typeof skipped === "string" || Array.isArray(skipped))
    return skipped.includes(file);
  if (skipped === undefined || skipped === null) field(skipped, "includes");
  throw new TypeError("skipped.includes is not a function");
}
