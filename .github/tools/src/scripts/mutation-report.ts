#!/usr/bin/env node

// Turns mutation results into the job summary.
//   node .github/tools/src/scripts/mutation-report.ts --scope <scope.json> [--stryker <stryker.json> ...] [--gremlins <gremlins.json> ...]
//   node .github/tools/src/scripts/mutation-report.ts --dir <folder> --full [--expect <entries>]

import fs from "node:fs";
import path from "node:path";
import {
  GREMLINS_FILE,
  gremlinsPackages,
  hasGremlinsResult,
  readGremlinsReport,
  readReportScope,
  SCOPE_FILE,
} from "./mutation-artifacts.ts";
import { EXCLUDED_TEST_FILES } from "./mutation-config-values.ts";
import { createRequire } from "node:module";
const require = createRequire(import.meta.url);
const ROOT = path.resolve(import.meta.dirname, "../../../..");

export interface Survivor {
  tool: string;
  file: string;
  line: unknown;
  mutator: unknown;
  original: string;
  mutated?: unknown;
}
interface Section {
  rows: Survivor[];
  counts: Record<string, number>;
}
interface ToolInput<T> {
  inScope: boolean;
  results: T[];
  error?: string;
}
export interface ReportInput {
  ledger?: readonly unknown[];
  stryker: ToolInput<unknown>;
  gremlins?: ToolInput<unknown>;
  full?: boolean;
}
interface MatrixEntry {
  name: string;
  status: string;
}
interface Collected {
  stryker: string[];
  gremlins: string[];
  entries: MatrixEntry[];
}

type GremlinsReport = NonNullable<ReturnType<typeof readGremlinsReport>>;

// JSON inputs retain JavaScript's existing property boxing and operation errors; these helpers do not validate a schema.
function property(value: unknown, key: string): unknown {
  if (value === null || value === undefined)
    throw new TypeError(
      `Cannot read properties of ${value} (reading '${key}')`,
    );
  return Reflect.get(Object(value), key);
}
function entries(value: unknown): [string, unknown][] {
  return Reflect.apply(Object.entries, undefined, [value]);
}
function* items(value: unknown, label: string): Generator<unknown> {
  if (typeof value === "string") {
    yield* value;
    return;
  }
  if (Array.isArray(value)) {
    const array: unknown[] = value;
    yield* array;
    return;
  }
  throw new TypeError(`${label} is not iterable`);
}
function splitSource(value: unknown): string[] {
  if (value === null || value === undefined)
    throw new TypeError(`Cannot read properties of ${value} (reading 'split')`);
  if (typeof value !== "string")
    throw new TypeError("entry.source.split is not a function");
  return value.split("\n");
}
function joinedReportPath(pkg: unknown, file: unknown): string {
  return Reflect.apply(path.posix.join, undefined, ["hooks/go", pkg, file]);
}

// A table cell holds code. A fence longer than any backtick run in the text keeps template literals intact, and a
// newline would split the row, so it becomes a space.
function cell(text: unknown) {
  const flat = String(text).replaceAll(/\r?\n/g, " ").replaceAll("|", "\\|");
  const longest = Math.max(
    0,
    ...(flat.match(/`+/g) ?? []).map((run) => run.length),
  );
  const fence = "`".repeat(longest + 1);
  const pad = flat.startsWith("`") || flat.endsWith("`") ? " " : "";
  return `${fence}${pad}${flat}${pad}${fence}`;
}

function strykerSection(result: unknown) {
  const rows: Survivor[] = [];
  const counts = {
    killed: 0,
    survived: 0,
    timeout: 0,
    noCoverage: 0,
    errors: 0,
  };
  for (const [file, entry] of entries(property(result, "files"))) {
    const lines = splitSource(property(entry, "source"));
    for (const mutant of items(property(entry, "mutants"), "entry.mutants")) {
      if (property(mutant, "status") === "Killed") counts.killed++;
      else if (property(mutant, "status") === "Survived") {
        counts.survived++;
        const line = property(
          property(property(mutant, "location"), "start"),
          "line",
        );
        rows.push({
          tool: "stryker",
          file,
          line,
          mutator: property(mutant, "mutatorName"),
          original: lines[Number(line) - 1]!.trim(),
          mutated: property(mutant, "replacement"),
        });
      } else if (property(mutant, "status") === "Timeout") counts.timeout++;
      else if (property(mutant, "status") === "NoCoverage") counts.noCoverage++;
      else counts.errors++;
    }
  }
  rows.sort(
    (a, b) => a.file.localeCompare(b.file) || Number(a.line) - Number(b.line),
  );
  return { rows, counts };
}

function merge(sections: readonly Section[]) {
  const rows = sections.flatMap((section) => section.rows);
  const counts: Record<string, number> = {};
  for (const section of sections) {
    for (const [key, value] of Object.entries(section.counts))
      counts[key] = (counts[key] ?? 0) + value;
  }
  return { rows, counts };
}

const LEDGER = path.join(ROOT, ".github/mutation/equivalent.json");

// A ledger entry records a survivor that no test can kill. It matches on tool, file, mutator, and the trimmed source
// line, so an edit to that line makes the entry stale instead of hiding a new survivor.
function applyLedger<T>(rows: readonly Survivor[], ledger: readonly T[]) {
  const used = new Set<number>();
  const kept = rows.filter((row) => {
    const index = ledger.findIndex((e) => {
      if (
        property(e, "tool") !== row.tool ||
        property(e, "file") !== row.file ||
        property(e, "mutator") !== row.mutator
      )
        return false;
      const source = property(e, "source");
      return typeof source === "string" && source.trim() === row.original;
    });
    if (index === -1) return true;
    used.add(index);
    return false;
  });
  return {
    rows: kept,
    dropped: rows.length - kept.length,
    stale: ledger.filter((_, i) => !used.has(i)),
  };
}

function table(rows: readonly Survivor[]) {
  if (rows.length === 0) return "No surviving mutants.\n";
  const head =
    "| File | Line | Mutator | Original | Mutated |\n| --- | --- | --- | --- | --- |\n";
  return (
    head +
    rows
      .map(
        (r) =>
          `| ${cell(r.file)} | ${r.line} | ${r.mutator} | ${cell(r.original)} | ${cell(r.mutated)} |`,
      )
      .join("\n") +
    "\n"
  );
}

function strykerMarkdown(
  { inScope, results, error }: ToolInput<unknown>,
  ledger: readonly unknown[],
) {
  if (!inScope) return "### Stryker: no in-scope change\n";
  if (results.length === 0)
    return [
      "### Stryker: no result",
      "",
      "The Stryker run wrote no result file. See the step log.",
      ...(error ? ["", "```text", error, "```"] : []),
      "",
    ].join("\n");
  const merged = merge(results.map(strykerSection));
  const { rows, dropped } = applyLedger(merged.rows, ledger);
  const counts = merged.counts;
  return [
    "### Stryker",
    "",
    `killed ${counts.killed}, survived ${counts.survived}, timed out ${counts.timeout}, not covered ${counts.noCoverage}, errors ${counts.errors}, dropped as equivalent ${dropped}`,
    "",
    table(rows),
    "Test files in `EXCLUDED_TEST_FILES` (`stryker.config.mjs`) did not run, so a mutant that only they cover shows as a survivor or as not covered.",
    "",
  ].join("\n");
}

function gremlinsSection(
  combined: unknown,
  read = (file: string) => fs.readFileSync(path.join(ROOT, file), "utf8"),
) {
  const rows: Survivor[] = [];
  const counts = {
    killed: 0,
    survived: 0,
    timeout: 0,
    noCoverage: 0,
    errors: 0,
  };
  const baseline = [];
  const report = {
    packages: property(combined, "packages") as GremlinsReport["packages"],
  };
  for (const { pkg, ok, reason, error, result } of gremlinsPackages(report)) {
    baseline.push(
      !ok
        ? `\`${pkg}\`: excluded: ${reason}; no result`
        : error
          ? `\`${pkg}\`: passed both baseline phases; no result, ${error}`
          : `\`${pkg}\`: passed both baseline phases`,
    );
    if (!result) continue;
    for (const file of items(property(result, "files"), "entry.result.files")) {
      const repoPath = joinedReportPath(pkg, property(file, "file_name"));
      const lines = read(repoPath).split("\n");
      for (const m of items(property(file, "mutations"), "file.mutations")) {
        if (property(m, "status") === "KILLED") counts.killed++;
        else if (property(m, "status") === "LIVED") {
          counts.survived++;
          // gremlins v0.6.0 writes no replacement text in its JSON, so the mutated column has none to show.
          rows.push({
            tool: "gremlins",
            file: repoPath,
            line: property(m, "line"),
            mutator: property(m, "type"),
            original: (lines[Number(property(m, "line")) - 1] ?? "").trim(),
            mutated: "—",
          });
        } else if (property(m, "status") === "TIMED OUT") counts.timeout++;
        else if (property(m, "status") === "NOT COVERED") counts.noCoverage++;
        else if (property(m, "status") !== "SKIPPED") counts.errors++;
      }
    }
  }
  rows.sort(
    (a, b) => a.file.localeCompare(b.file) || Number(a.line) - Number(b.line),
  );
  return { rows, counts, baseline };
}

function gremlinsMarkdown(
  { inScope, results }: ToolInput<unknown>,
  ledger: readonly unknown[],
) {
  if (!inScope) return "### gremlins: no in-scope change\n";
  if (results.length === 0)
    return "### gremlins: no result\n\nThe gremlins run wrote no result file. See the step log.\n";
  const sections = results.map((r) => gremlinsSection(r));
  const merged = merge(sections);
  const { rows, dropped } = applyLedger(merged.rows, ledger);
  const counts = merged.counts;
  return [
    "### gremlins",
    "",
    `killed ${counts.killed}, survived ${counts.survived}, timed out ${counts.timeout}, not covered ${counts.noCoverage}, errors ${counts.errors}, dropped as equivalent ${dropped}`,
    "",
    table(rows),
    "#### Baseline (isolated copy, then a comment-only change)",
    "",
    ...sections.flatMap((s) => s.baseline).map((line) => `- ${line}`),
    "",
  ].join("\n");
}

function excludedTestsMarkdown() {
  // The old report loaded the configuration only when it rendered this section.
  require("./mutation-config.ts");
  const lines = Object.entries(EXCLUDED_TEST_FILES).map(
    ([file, reason]) => `- \`${file}\`: ${reason}`,
  );
  return ["### Excluded Stryker test files", "", ...lines, ""].join("\n");
}

function render(input: ReportInput) {
  const ledger = input.ledger ?? [];
  const parts = [
    "## Mutation testing",
    "",
    strykerMarkdown(input.stryker, ledger),
    gremlinsMarkdown(input.gremlins ?? { inScope: false, results: [] }, ledger),
    ...(input.stryker.inScope ? [excludedTestsMarkdown()] : []),
  ];
  if (input.full) {
    // An entry is stale only when no survivor of the whole run matched it, so this needs every row of the run.
    const all = [
      ...merge(input.stryker.results.map(strykerSection)).rows,
      ...merge((input.gremlins?.results ?? []).map((r) => gremlinsSection(r)))
        .rows,
    ];
    const { stale } = applyLedger(all, ledger);
    if (stale.length > 0) {
      parts.push(
        "### Stale ledger entries",
        "",
        `${stale.length} of ${ledger.length} ledger entries matched no survivor in this run.`,
        "",
        ...stale.map(
          (e) =>
            `- ${property(e, "tool")} ${property(e, "file")} ${property(e, "mutator")}: ${property(e, "reason")}`,
        ),
        "",
      );
    }
  }
  return parts.join("\n");
}

function readJson(file: string | undefined): unknown {
  if (file === undefined || !fs.existsSync(file)) return null;
  try {
    return JSON.parse(fs.readFileSync(file, "utf8"));
  } catch {
    return null;
  }
}

function filesAfter(argv: readonly string[], flag: string) {
  return argv.flatMap((arg, i) => (arg === flag ? [argv[i + 1]!] : []));
}

// A result can be named by both the matrix directory and a command-line flag.
// Share its decoded value while preserving how many times it contributes.
function createGremlinsReader() {
  const results = new Map<string, GremlinsReport | null>();
  return (file: string) => {
    const key = path.resolve(file);
    const previous = results.get(key);
    if (previous !== undefined) return previous;
    const result = readGremlinsReport(file);
    results.set(key, result);
    return result;
  };
}

function collect(
  dir: string,
  readGremlins: (file: string) => GremlinsReport | null = readGremlinsReport,
) {
  const found: Collected = { stryker: [], gremlins: [], entries: [] };
  if (!fs.existsSync(dir)) return found;
  for (const rel of fs
    .readdirSync(dir, { recursive: true, encoding: "utf8" })
    .sort()) {
    const base = path.basename(rel);
    if (base === "stryker.json") found.stryker.push(path.join(dir, rel));
    if (base === GREMLINS_FILE) found.gremlins.push(path.join(dir, rel));
  }
  // One artifact folder per matrix entry. entry.txt names the entry, so a failed entry is still named.
  for (const name of fs
    .readdirSync(dir)
    .sort((a, b) => a.localeCompare(b, undefined, { numeric: true }))) {
    const folder = path.join(dir, name);
    if (!fs.statSync(folder).isDirectory()) continue;
    const text = (file: string) =>
      fs.existsSync(path.join(folder, file))
        ? fs.readFileSync(path.join(folder, file), "utf8").trim()
        : null;
    const files = ["stryker.json", GREMLINS_FILE]
      .map((file) => path.join(folder, file))
      .filter((file) => fs.existsSync(file));
    const status =
      files.length > 0
        ? files.every((file) =>
            path.basename(file) === GREMLINS_FILE
              ? hasGremlinsResult(readGremlins(file))
              : Boolean(readJson(file)),
          )
          ? "result"
          : "no result"
        : (text("stryker-skipped.txt") ??
          (text("stryker-error.txt")
            ? `no result: ${text("stryker-error.txt")!.split("\n")[0]}`
            : "no result"));
    found.entries.push({ name: text("entry.txt") ?? name, status });
  }
  return found;
}

function entriesMarkdown(entries: readonly MatrixEntry[], expected: number) {
  const lines = [
    "### Matrix entries",
    "",
    ...entries.map((entry) => `- ${entry.name}: ${entry.status}`),
  ];
  if (expected > entries.length) {
    lines.push(
      `- ${expected - entries.length} of ${expected} entries uploaded nothing: no result`,
    );
  }
  return `${lines.join("\n")}\n`;
}

function main(argv: string[]) {
  const full = argv.includes("--full");
  const scope = full
    ? { stryker: true, gremlins: true }
    : readReportScope(argv[argv.indexOf("--scope") + 1]);
  if (!scope) {
    process.stdout.write(
      `## Mutation testing\n\n### Scope: no result\n\nThe scope step wrote no \`${SCOPE_FILE}\`. See the step log.\n`,
    );
    return 0;
  }
  const readGremlins = createGremlinsReader();
  const found = argv.includes("--dir")
    ? collect(argv[argv.indexOf("--dir") + 1]!, readGremlins)
    : { stryker: [], gremlins: [], entries: [] };
  const strykerFiles = [...filesAfter(argv, "--stryker"), ...found.stryker];
  const gremlinsFiles = [...filesAfter(argv, "--gremlins"), ...found.gremlins];
  const ledgerFile = argv.includes("--ledger")
    ? argv[argv.indexOf("--ledger") + 1]
    : LEDGER;
  const ledger = readJson(ledgerFile);
  const ledgerEntries: unknown[] = Array.isArray(ledger) ? ledger : [];
  const input = {
    full,
    ledger: ledgerEntries,
    stryker: {
      inScope: scope.stryker,
      results: strykerFiles.map(readJson).filter(Boolean),
      error: strykerFiles
        .map((file) => path.join(path.dirname(file), "stryker-error.txt"))
        .filter((file) => fs.existsSync(file))
        .map((file) => fs.readFileSync(file, "utf8").trim())
        .join("\n"),
    },
    gremlins: {
      inScope: scope.gremlins,
      results: gremlinsFiles.map(readGremlins).filter(Boolean),
    },
  };
  process.stdout.write(render(input));
  if (full) {
    const expected = argv.includes("--expect")
      ? Number(argv[argv.indexOf("--expect") + 1])
      : 0;
    process.stdout.write(`\n${entriesMarkdown(found.entries, expected)}`);
  }
  return 0;
}

if (import.meta.main) process.exit(main(process.argv.slice(2)));

export {
  strykerSection,
  gremlinsSection,
  merge,
  table,
  render,
  cell,
  filesAfter,
  collect,
  entriesMarkdown,
  applyLedger,
};
