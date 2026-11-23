import assert from "node:assert/strict";
import test from "node:test";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { spawnSync } from "node:child_process";
import {
  strykerSection,
  gremlinsSection,
  render,
  cell,
  collect,
  entriesMarkdown,
  applyLedger,
} from "./mutation-report.ts";

test("falsy report artifacts retain the existing no-result classification", (t) => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "mutation-falsy-"));
  t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
  const scope = path.join(dir, "scope.json");
  const result = path.join(dir, "stryker.json");
  fs.writeFileSync(
    scope,
    JSON.stringify({ stryker: [".github/tools/src/a.ts"], gremlins: [] }),
  );
  for (const value of [false, 0, "", null]) {
    fs.writeFileSync(result, JSON.stringify(value));
    const run = spawnSync(
      process.execPath,
      [
        path.join(import.meta.dirname, "mutation-report.ts"),
        "--scope",
        scope,
        "--stryker",
        result,
      ],
      { encoding: "utf8", timeout: 10000 },
    );
    assert.equal(run.status, 0, run.stderr);
    assert.match(run.stdout, /Stryker: no result/);
  }
});

const result = {
  files: {
    ".github/tools/src/a.ts": {
      source: "const x = 1;\nif (a || b) {\n",
      mutants: [
        {
          mutatorName: "LogicalOperator",
          status: "Survived",
          replacement: "a && b",
          location: { start: { line: 2 } },
        },
        {
          mutatorName: "BooleanLiteral",
          status: "Killed",
          replacement: "false",
          location: { start: { line: 1 } },
        },
        {
          mutatorName: "BlockStatement",
          status: "Timeout",
          replacement: "{}",
          location: { start: { line: 2 } },
        },
        {
          mutatorName: "StringLiteral",
          status: "NoCoverage",
          replacement: '""',
          location: { start: { line: 1 } },
        },
      ],
    },
  },
};

test("a survivor renders as one row with the original line and the replacement", () => {
  const section = strykerSection(result);
  assert.deepEqual(section.rows, [
    {
      tool: "stryker",
      file: ".github/tools/src/a.ts",
      line: 2,
      mutator: "LogicalOperator",
      original: "if (a || b) {",
      mutated: "a && b",
    },
  ]);
  assert.deepEqual(section.counts, {
    killed: 1,
    survived: 1,
    timeout: 1,
    noCoverage: 1,
    errors: 0,
  });
});

test("the Markdown escapes the pipe in code and counts timeouts apart from kills", () => {
  const markdown = render({ stryker: { inScope: true, results: [result] } });
  assert.match(
    markdown,
    /\| `\.github\/tools\/src\/a\.ts` \| 2 \| LogicalOperator \| `if \(a \\\|\\\| b\) \{` \| `a && b` \|/,
  );
  assert.match(
    markdown,
    /killed 1, survived 1, timed out 1, not covered 1, errors 0/,
  );
});

test("an in-scope tool with no result file says no result", () => {
  assert.match(
    render({ stryker: { inScope: true, results: [] } }),
    /Stryker: no result/,
  );
});

test("an empty scope says no in-scope change", () => {
  assert.match(
    render({ stryker: { inScope: false, results: [] } }),
    /Stryker: no in-scope change/,
  );
});

test("a missing scope file says no result, not no in-scope change", () => {
  const missing = path.join(os.tmpdir(), "mutation-no-such-scope.json");
  const run = spawnSync(
    process.execPath,
    [`${import.meta.dirname}/mutation-report.ts`, "--scope", missing],
    { encoding: "utf8" },
  );
  assert.equal(run.status, 0);
  assert.match(run.stdout, /Scope: no result/);
  assert.doesNotMatch(run.stdout, /no in-scope change/);
});

test("a cell keeps backticks and pipes safe and never spans lines", () => {
  assert.equal(cell("a|b"), "`a\\|b`");
  assert.equal(cell("x `y` z"), "``x `y` z``");
  assert.equal(cell("`edge`"), "`` `edge` ``");
  assert.equal(cell("a\nb"), "`a b`");
});

test("an unreadable result file counts as no result", () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "mutation-bad-"));
  const file = path.join(dir, "scope.json");
  fs.writeFileSync(file, "{ cut off");
  try {
    const run = spawnSync(
      process.execPath,
      [`${import.meta.dirname}/mutation-report.ts`, "--scope", file],
      {
        encoding: "utf8",
      },
    );
    assert.equal(run.status, 0);
    assert.match(run.stdout, /Scope: no result/);
  } finally {
    fs.rmSync(dir, { recursive: true, force: true });
  }
});

const combined = {
  packages: [
    {
      pkg: "./internal/toolname",
      baseline: { ok: true },
      result: {
        files: [
          {
            file_name: "toolname.go",
            mutations: [
              {
                type: "CONDITIONALS_BOUNDARY",
                status: "LIVED",
                line: 2,
                column: 5,
              },
              {
                type: "CONDITIONALS_NEGATION",
                status: "KILLED",
                line: 2,
                column: 5,
              },
              {
                type: "ARITHMETIC_BASE",
                status: "TIMED OUT",
                line: 1,
                column: 1,
              },
            ],
          },
        ],
      },
    },
    {
      pkg: "./cmd/hook",
      baseline: {
        ok: false,
        reason: "fails after a comment-only change: FAIL",
      },
    },
  ],
};

test("a lived gremlins mutant renders with the repository path and the source line", () => {
  const read = (file: string) =>
    file === "hooks/go/internal/toolname/toolname.go"
      ? "a\n\tif i >= 0 {\n"
      : "";
  const section = gremlinsSection(combined, read);
  assert.deepEqual(section.rows, [
    {
      tool: "gremlins",
      file: "hooks/go/internal/toolname/toolname.go",
      line: 2,
      mutator: "CONDITIONALS_BOUNDARY",
      original: "if i >= 0 {",
      mutated: "—",
    },
  ]);
  assert.deepEqual(section.counts, {
    killed: 1,
    survived: 1,
    timeout: 1,
    noCoverage: 0,
    errors: 0,
  });
  assert.deepEqual(section.baseline, [
    "`./internal/toolname`: passed both baseline phases",
    "`./cmd/hook`: excluded: fails after a comment-only change: FAIL; no result",
  ]);
});

test("the report lists the excluded Stryker test files with their reasons", () => {
  const markdown = render({ stryker: { inScope: true, results: [] } });
  assert.match(markdown, /### Excluded Stryker test files/);
  assert.match(markdown, /`\.github\/tools\/src\/release\/action\.test\.ts`: /);
});

test("the gremlins part renders counts, a survivor row, and the baseline list", () => {
  const markdown = render({
    stryker: { inScope: false, results: [] },
    gremlins: {
      inScope: true,
      results: [
        {
          packages: [
            {
              pkg: "./internal/toolname",
              baseline: { ok: true },
              result: {
                files: [
                  {
                    file_name: "toolname.go",
                    mutations: [
                      {
                        type: "CONDITIONALS_BOUNDARY",
                        status: "LIVED",
                        line: 1,
                      },
                      {
                        type: "ARITHMETIC_BASE",
                        status: "NOT COVERED",
                        line: 1,
                      },
                      { type: "ARITHMETIC_BASE", status: "SKIPPED", line: 1 },
                      {
                        type: "ARITHMETIC_BASE",
                        status: "NOT VIABLE",
                        line: 1,
                      },
                      { type: "ARITHMETIC_BASE", status: "TIMED OUT", line: 1 },
                    ],
                  },
                ],
              },
            },
            {
              pkg: "./cmd/hook",
              baseline: { ok: false, reason: "fails in phase 1" },
            },
          ],
        },
      ],
    },
  });
  assert.match(markdown, /### gremlins\n/);
  assert.match(
    markdown,
    /killed 0, survived 1, timed out 1, not covered 1, errors 1/,
  );
  assert.match(markdown, /hooks\/go\/internal\/toolname\/toolname\.go/);
  assert.match(
    markdown,
    /- `\.\/internal\/toolname`: passed both baseline phases/,
  );
  assert.match(markdown, /- `\.\/cmd\/hook`: excluded: fails in phase 1/);
  assert.doesNotMatch(markdown, /### Excluded Stryker test files/);
});

test("a gremlins run with no result file and an empty gremlins scope say so", () => {
  const none = { inScope: false, results: [] };
  assert.match(
    render({ stryker: none, gremlins: { inScope: true, results: [] } }),
    /### gremlins: no result/,
  );
  assert.match(
    render({ stryker: none, gremlins: none }),
    /### gremlins: no in-scope change/,
  );
});

test("main reads the file after --gremlins", () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "mutation-main-"));
  try {
    const scope = path.join(dir, "scope.json");
    const result = path.join(dir, "gremlins.json");
    fs.writeFileSync(
      scope,
      JSON.stringify({
        stryker: [],
        gremlins: [{ pkg: "./internal/toolname", changed: ["toolname.go"] }],
      }),
    );
    fs.writeFileSync(
      result,
      JSON.stringify({
        packages: [{ pkg: "./internal/toolname", baseline: { ok: true } }],
      }),
    );
    const run = spawnSync(
      process.execPath,
      [
        `${import.meta.dirname}/mutation-report.ts`,
        "--scope",
        scope,
        "--gremlins",
        result,
      ],
      { encoding: "utf8" },
    );
    assert.equal(run.status, 0);
    assert.match(run.stdout, /passed both baseline phases/);
    assert.doesNotMatch(run.stdout, /gremlins: no result/);
  } finally {
    fs.rmSync(dir, { recursive: true, force: true });
  }
});

test("a mutation on a line past the end of the file does not stop the report", () => {
  const section = gremlinsSection(
    {
      packages: [
        {
          pkg: "./internal/toolname",
          baseline: { ok: true },
          result: {
            files: [
              {
                file_name: "toolname.go",
                mutations: [{ type: "X", status: "LIVED", line: 99999 }],
              },
            ],
          },
        },
      ],
    },
    () => "one line",
  );
  assert.equal(section.rows[0]!.original, "");
});

test("a package with a baseline pass and a gremlins error lists the error", () => {
  const section = gremlinsSection(
    {
      packages: [
        {
          pkg: "./internal/toolname",
          baseline: { ok: true },
          error: "gremlins exited 1",
        },
      ],
    },
    () => "",
  );
  assert.deepEqual(section.baseline, [
    "`./internal/toolname`: passed both baseline phases; no result, gremlins exited 1",
  ]);
});

const GREMLINS_OK = JSON.stringify({
  packages: [
    {
      pkg: "./internal/shellwords",
      baseline: { ok: true },
      result: { files: [] },
    },
  ],
});

function entryFolder(
  t: import("node:test").TestContext,
  files: Record<string, Record<string, string>>,
) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "mutation-dir-"));
  t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
  for (const [folder, contents] of Object.entries(files)) {
    fs.mkdirSync(path.join(dir, folder));
    for (const [name, text] of Object.entries(contents))
      fs.writeFileSync(path.join(dir, folder, name), text);
  }
  return dir;
}

test("--dir collects every result file below the folder", (t) => {
  const dir = entryFolder(t, {
    "mutation-0": {
      "gremlins.json": GREMLINS_OK,
      "entry.txt": "gremlins ./internal/shellwords\n",
    },
    "mutation-5": { "stryker.json": "{}" },
    "mutation-9": {
      "stryker-skipped.txt": "no tests in .github/tools/src/peer\n",
      "entry.txt": "stryker .github/tools/src/peer\n",
    },
    "mutation-10": { "entry.txt": "stryker tools-rest\n" },
  });
  assert.deepEqual(collect(dir), {
    stryker: [path.join(dir, "mutation-5", "stryker.json")],
    gremlins: [path.join(dir, "mutation-0", "gremlins.json")],
    entries: [
      { name: "gremlins ./internal/shellwords", status: "result" },
      { name: "mutation-5", status: "result" },
      {
        name: "stryker .github/tools/src/peer",
        status: "no tests in .github/tools/src/peer",
      },
      { name: "stryker tools-rest", status: "no result" },
    ],
  });
});

test("a gremlins.json whose package has an error and no result is no result", (t) => {
  const dir = entryFolder(t, {
    "mutation-0": {
      "entry.txt": "gremlins ./cmd/hook\n",
      "gremlins.json": JSON.stringify({
        packages: [
          {
            pkg: "./cmd/hook",
            baseline: { ok: true },
            error: "gremlins exited 1",
          },
        ],
      }),
    },
  });
  assert.deepEqual(collect(dir).entries, [
    { name: "gremlins ./cmd/hook", status: "no result" },
  ]);
});

test("one failed package makes a gremlins entry no result", (t) => {
  const dir = entryFolder(t, {
    "mutation-0": {
      "entry.txt": "gremlins rest\n",
      "gremlins.json": JSON.stringify({
        packages: [
          { pkg: "./a", baseline: { ok: true }, result: { files: [] } },
          { pkg: "./b", baseline: { ok: false, reason: "fails" } },
        ],
      }),
    },
  });
  assert.equal(collect(dir).entries[0]!.status, "no result");
});

test("a result file that does not parse is no result", (t) => {
  const dir = entryFolder(t, {
    "mutation-0": { "entry.txt": "stryker tools-rest\n", "stryker.json": "{" },
    "mutation-1": { "entry.txt": "gremlins rest\n", "gremlins.json": "{" },
    "mutation-2": { "entry.txt": "gremlins rest\n", "gremlins.json": "{}" },
  });
  assert.deepEqual(
    collect(dir).entries.map((entry) => entry.status),
    ["no result", "no result", "no result"],
  );
});

test("the entry list names a missing entry as no result", () => {
  const markdown = entriesMarkdown(
    [{ name: "stryker tools-rest", status: "result" }],
    11,
  );
  assert.match(markdown, /- stryker tools-rest: result/);
  assert.match(markdown, /- 10 of 11 entries uploaded nothing: no result/);
});

test("--dir with --full merges the folder and lists the matrix entries", (t) => {
  const dir = entryFolder(t, {
    "mutation-0": {
      "entry.txt": "gremlins ./internal/shellwords\n",
      "gremlins.json": GREMLINS_OK,
    },
  });
  const run = spawnSync(
    process.execPath,
    [
      `${import.meta.dirname}/mutation-report.ts`,
      "--dir",
      dir,
      "--full",
      "--expect",
      "3",
    ],
    { encoding: "utf8" },
  );
  assert.equal(run.status, 0, run.stderr);
  assert.match(run.stdout, /### Matrix entries/);
  assert.match(run.stdout, /- gremlins \.\/internal\/shellwords: result/);
  assert.match(run.stdout, /- 2 of 3 entries uploaded nothing: no result/);
  assert.match(run.stdout, /passed both baseline phases/);
});

test("collect orders entries by number and accepts a missing folder", (t) => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "mutation-order-"));
  t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
  for (const name of ["mutation-10", "mutation-2", "mutation-1"]) {
    fs.mkdirSync(path.join(dir, name));
  }
  assert.deepEqual(
    collect(dir).entries.map((entry) => entry.name),
    ["mutation-1", "mutation-2", "mutation-10"],
  );
  assert.deepEqual(collect(path.join(dir, "absent")), {
    stryker: [],
    gremlins: [],
    entries: [],
  });
});

const strykerRows = [
  {
    tool: "stryker",
    file: ".github/tools/src/a.ts",
    line: 2,
    mutator: "LogicalOperator",
    original: "if (a || b) {",
    mutated: "a && b",
  },
  {
    tool: "stryker",
    file: ".github/tools/src/a.ts",
    line: 9,
    mutator: "BooleanLiteral",
    original: "return true;",
    mutated: "false",
  },
];

test("a ledger entry drops its survivor, and an unmatched entry is stale", () => {
  const ledger = [
    {
      tool: "stryker",
      file: ".github/tools/src/a.ts",
      mutator: "LogicalOperator",
      source: "if (a || b) {",
      reason: "r",
    },
    {
      tool: "gremlins",
      file: "hooks/go/x.go",
      mutator: "ARITHMETIC_BASE",
      source: "n + 1",
      reason: "r",
    },
  ];
  const out = applyLedger(strykerRows, ledger);
  assert.deepEqual(
    out.rows.map((r) => r.line),
    [9],
  );
  assert.equal(out.dropped, 1);
  assert.deepEqual(
    out.stale.map((e) => e.file),
    ["hooks/go/x.go"],
  );
});

test("a ledger entry for another tool or another source line drops nothing", () => {
  const base = {
    file: ".github/tools/src/a.ts",
    mutator: "LogicalOperator",
    source: "if (a || b) {",
    reason: "r",
  };
  for (const entry of [
    { ...base, tool: "gremlins" },
    { ...base, tool: "stryker", file: ".github/tools/src/b.ts" },
    { ...base, tool: "stryker", mutator: "BooleanLiteral" },
    { ...base, tool: "stryker", source: "if (a) {" },
  ]) {
    const out = applyLedger(strykerRows, [entry]);
    assert.equal(out.dropped, 0);
    assert.equal(out.rows.length, 2);
    assert.equal(out.stale.length, 1);
  }
});

test("a ledger source with surrounding whitespace still matches", () => {
  const out = applyLedger(strykerRows, [
    {
      tool: "stryker",
      file: ".github/tools/src/a.ts",
      mutator: "LogicalOperator",
      source: "  if (a || b) {  ",
      reason: "r",
    },
  ]);
  assert.equal(out.dropped, 1);
});

const equivalentLedger = [
  {
    tool: "stryker",
    file: ".github/tools/src/a.ts",
    mutator: "LogicalOperator",
    source: "if (a || b) {",
    reason: "both operands are always equal",
  },
];

test("the Stryker summary drops the recorded survivor and counts it", () => {
  const markdown = render({
    stryker: { inScope: true, results: [result] },
    ledger: equivalentLedger,
  });
  assert.match(markdown, /dropped as equivalent 1/);
  assert.match(markdown, /No surviving mutants\./);
  assert.doesNotMatch(markdown, /LogicalOperator \|/);
});

test("the summary says dropped as equivalent 0 when the ledger is empty", () => {
  const markdown = render({ stryker: { inScope: true, results: [result] } });
  assert.match(markdown, /dropped as equivalent 0/);
  assert.match(markdown, /LogicalOperator/);
});

test("the gremlins summary drops a recorded survivor", () => {
  // render reads the real source file, so the ledger takes its line from the same file.
  const source = fs
    .readFileSync(
      `${import.meta.dirname}/../../../../hooks/go/internal/toolname/toolname.go`,
      "utf8",
    )
    .split("\n")[1];
  const input = {
    stryker: { inScope: false, results: [] },
    gremlins: { inScope: true, results: [combined] },
    ledger: [
      {
        tool: "gremlins",
        file: "hooks/go/internal/toolname/toolname.go",
        mutator: "CONDITIONALS_BOUNDARY",
        source,
        reason: "r",
      },
    ],
  };
  assert.match(
    render(input),
    /dropped as equivalent 1\n\nNo surviving mutants\./,
  );
  assert.match(
    render({ ...input, ledger: [] }),
    /dropped as equivalent 0\n\n\| File/,
  );
});

test("a full run lists a ledger entry that matched nothing as stale", () => {
  const stale = {
    tool: "stryker",
    file: ".github/tools/src/gone.ts",
    mutator: "BooleanLiteral",
    source: "return true;",
    reason: "the code was removed",
  };
  const input = {
    stryker: { inScope: true, results: [result] },
    ledger: [...equivalentLedger, stale],
  };
  const full = render({ ...input, full: true });
  assert.match(full, /### Stale ledger entries/);
  assert.match(
    full,
    /- stryker \.github\/tools\/src\/gone\.ts BooleanLiteral: the code was removed/,
  );
  assert.doesNotMatch(full, /both operands are always equal/);
  assert.doesNotMatch(render(input), /Stale ledger entries/);
});

test("a full run with every entry matched has no stale section", () => {
  const markdown = render({
    stryker: { inScope: true, results: [result] },
    ledger: equivalentLedger,
    full: true,
  });
  assert.doesNotMatch(markdown, /Stale ledger entries/);
});

test("main reads a --ledger file and a missing or broken ledger is empty", (t) => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "mutation-ledger-"));
  t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
  const scope = path.join(dir, "scope.json");
  const stryker = path.join(dir, "stryker.json");
  fs.writeFileSync(scope, JSON.stringify({ stryker: ["x"], gremlins: [] }));
  fs.writeFileSync(stryker, JSON.stringify(result));
  const good = path.join(dir, "good.json");
  fs.writeFileSync(good, JSON.stringify(equivalentLedger));
  const broken = path.join(dir, "broken.json");
  fs.writeFileSync(broken, "{not json");
  const run = (ledger: string) =>
    spawnSync(
      process.execPath,
      [
        `${import.meta.dirname}/mutation-report.ts`,
        "--scope",
        scope,
        "--stryker",
        stryker,
        "--ledger",
        ledger,
      ],
      { encoding: "utf8" },
    );
  assert.match(run(good).stdout, /dropped as equivalent 1/);
  const notArray = path.join(dir, "object.json");
  fs.writeFileSync(notArray, "{}");
  for (const ledger of [broken, notArray, path.join(dir, "absent.json")]) {
    const out = run(ledger);
    assert.equal(out.status, 0, out.stderr);
    assert.match(out.stdout, /dropped as equivalent 0/);
  }
});

test("the committed ledger is a JSON array", () => {
  const ledger = JSON.parse(
    fs.readFileSync(
      `${import.meta.dirname}/../../../../.github/mutation/equivalent.json`,
      "utf8",
    ),
  );
  assert.ok(Array.isArray(ledger));
});

test("a full run does not call a matched gremlins ledger entry stale", () => {
  const file = "hooks/go/internal/toolname/toolname.go";
  const source = fs
    .readFileSync(path.join(import.meta.dirname, "../../../..", file), "utf8")
    .split("\n")[0];
  const gremlins = {
    inScope: true,
    results: [
      {
        packages: [
          {
            pkg: "./internal/toolname",
            baseline: { ok: true },
            result: {
              files: [
                {
                  file_name: "toolname.go",
                  mutations: [
                    { type: "LEDGER_PROBE", status: "LIVED", line: 1 },
                  ],
                },
              ],
            },
          },
        ],
      },
    ],
  };
  const stryker = { inScope: false, results: [] };
  const entry = {
    tool: "gremlins",
    file,
    mutator: "LEDGER_PROBE",
    source,
    reason: "r",
  };
  const matched = render({ full: true, stryker, gremlins, ledger: [entry] });
  assert.doesNotMatch(matched, /Stale ledger entries/);
  assert.match(matched, /dropped as equivalent 1/);
  const stale = render({
    full: true,
    stryker,
    gremlins,
    ledger: [{ ...entry, mutator: "OTHER" }],
  });
  assert.match(stale, /Stale ledger entries/);
  assert.match(stale, /1 of 1 ledger entries matched no survivor/);
});

test("a ledger entry without a source text does not stop the report", () => {
  const out = render({
    stryker: { inScope: true, results: [result] },
    ledger: [{ tool: "stryker", file: "x", mutator: "y", reason: "r" }],
  });
  assert.match(out, /dropped as equivalent 0/);
});

test("a failed Stryker run shows its error in the summary and in the entry list", (t) => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "mutation-err-"));
  t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
  fs.mkdirSync(path.join(dir, "mutation-0"));
  fs.writeFileSync(
    path.join(dir, "mutation-0", "stryker-error.txt"),
    "12:01 ERROR DryRunExecutor One or more tests failed\n",
  );
  assert.equal(
    collect(dir).entries[0]!.status,
    "no result: 12:01 ERROR DryRunExecutor One or more tests failed",
  );
  const scope = path.join(dir, "scope.json");
  fs.writeFileSync(scope, JSON.stringify({ stryker: ["x"], gremlins: [] }));
  const run = spawnSync(
    process.execPath,
    [
      `${import.meta.dirname}/mutation-report.ts`,
      "--scope",
      scope,
      "--stryker",
      path.join(dir, "mutation-0", "stryker.json"),
    ],
    { encoding: "utf8" },
  );
  assert.match(run.stdout, /### Stryker: no result/);
  assert.match(run.stdout, /ERROR DryRunExecutor One or more tests failed/);
});
