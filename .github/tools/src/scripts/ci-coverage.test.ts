import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import { type CommandResult, type CommandRun, main } from "./ci-coverage.ts";

const MODULE = path.join(import.meta.dirname, "ci-coverage.ts");

function tempDir(t: test.TestContext): string {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "agentics-ci-coverage-"));
  t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
  return dir;
}

// Writes the profile where the download step leaves it: one directory per shard artifact holding `<shard>.out`.
function profile(root: string, shard: string, contents: string): void {
  const dir = path.join(root, "profiles", `go-coverage-profile-${shard}`);
  fs.mkdirSync(dir, { recursive: true });
  fs.writeFileSync(path.join(dir, `${shard}.out`), contents);
}

function run(
  t: test.TestContext,
  argv: string[],
  extra: {
    shards?: string[];
    run?: CommandRun;
    setup?: (root: string) => void;
    summary?: boolean;
  } = {},
) {
  const root = tempDir(t);
  extra.setup?.(root);
  const out: string[] = [];
  const errors: string[] = [];
  const summary = path.join(root, "summary.md");
  const env: Record<string, string> = {
    RUNNER_TEMP: root,
    GITHUB_WORKSPACE: root,
  };
  if (extra.summary !== false) env["GITHUB_STEP_SUMMARY"] = summary;
  const status = main(argv, {
    env,
    cwd: root,
    shards: extra.shards,
    run: extra.run,
    write: (line) => out.push(line),
    report: (line) => errors.push(line),
  });
  return { root, status, out, errors, summary };
}

const ERROR = (shard: string) =>
  `::error::No coverage profile from the ${shard} shard; its tests failed or did not run.`;

test("require-profiles passes when every shard delivered a non-empty profile", (t) => {
  const r = run(t, ["require-profiles", "hooks"], {
    setup: (root) => profile(root, "hooks", "mode: count\n"),
  });
  assert.equal(r.status, 0);
  assert.deepEqual(r.out, []);
});

test("require-profiles names a missing shard and an empty one", (t) => {
  const missing = run(t, ["require-profiles", "hooks"]);
  assert.equal(missing.status, 1);
  assert.deepEqual(missing.out, [ERROR("hooks")]);
  const empty = run(t, ["require-profiles", "hooks"], {
    setup: (root) => profile(root, "hooks", ""),
  });
  assert.equal(empty.status, 1);
  assert.deepEqual(empty.out, [ERROR("hooks")]);
});

test("require-profiles checks every shard before failing", (t) => {
  const r = run(t, ["require-profiles", "a", "b", "c"], {
    setup: (root) => profile(root, "b", "mode: count\n"),
  });
  assert.equal(r.status, 1);
  assert.deepEqual(r.out, [ERROR("a"), ERROR("c")]);
});

test("require-profiles with no shard, or without RUNNER_TEMP, exits nonzero", (t) => {
  assert.equal(run(t, ["require-profiles"]).status, 2);
  const errors: string[] = [];
  const status = main(["require-profiles", "hooks"], {
    env: {},
    report: (line) => errors.push(line),
  });
  assert.equal(status, 1);
  assert.match(errors.join("\n"), /^ci-coverage: RUNNER_TEMP is not set/);
});

test("merge-profiles keeps one mode header and every block in shard order", (t) => {
  const r = run(t, ["merge-profiles"], {
    shards: ["a", "b", "c"],
    setup: (root) => {
      profile(root, "a", "mode: atomic\nexample/a.go:1.1,1.2 1 1\n");
      profile(root, "b", "mode: atomic\nexample/b.go:1.1,1.2 1 2\n");
      profile(
        root,
        "c",
        "mode: atomic\nexample/c.go:1.1,1.2 1 3\nexample/c.go:2.1,2.2 1 4\n",
      );
    },
  });
  assert.equal(r.status, 0, r.errors.join("\n"));
  assert.equal(
    fs.readFileSync(path.join(r.root, "coverage-hooks.out"), "utf8"),
    "mode: atomic\nexample/a.go:1.1,1.2 1 1\nexample/b.go:1.1,1.2 1 2\nexample/c.go:1.1,1.2 1 3\nexample/c.go:2.1,2.2 1 4\n",
  );
});

test("merge-profiles of the single hooks profile is that profile", (t) => {
  const r = run(t, ["merge-profiles"], {
    setup: (root) =>
      profile(root, "hooks", "mode: atomic\nexample/hooks.go:1.1,1.2 1 1\n"),
  });
  assert.equal(r.status, 0, r.errors.join("\n"));
  assert.equal(
    fs.readFileSync(path.join(r.root, "coverage-hooks.out"), "utf8"),
    "mode: atomic\nexample/hooks.go:1.1,1.2 1 1\n",
  );
});

test("merge-profiles fails on a missing profile and writes no merged file", (t) => {
  const r = run(t, ["merge-profiles"]);
  assert.equal(r.status, 1);
  assert.equal(fs.existsSync(path.join(r.root, "coverage-hooks.out")), false);
  assert.match(r.errors.join("\n"), /^ci-coverage: cannot read/);
});

const LISTING = "hooks/go/a.go:1:\tf\t100.0%\ntotal:\t\t(statements)\t93.5%\n";

function fakeGo(result: Partial<CommandResult> = {}) {
  const calls: { command: string; args: string[]; cwd: string | undefined }[] =
    [];
  const goRun: CommandRun = (command, args, options) => {
    calls.push({ command, args, cwd: options.cwd });
    return { status: 0, stdout: LISTING, ...result };
  };
  return { calls, goRun };
}

test("totals writes coverage-hooks.txt and the exact summary", (t) => {
  const f = fakeGo();
  const r = run(t, ["totals"], { run: f.goRun });
  assert.equal(r.status, 0, r.errors.join("\n"));
  assert.deepEqual(f.calls.length, 1);
  assert.equal(f.calls[0]!.command, "go");
  assert.deepEqual(f.calls[0]!.args, [
    "-C",
    "hooks/go",
    "tool",
    "cover",
    "-func=../../coverage-hooks.out",
  ]);
  assert.equal(f.calls[0]!.cwd, r.root);
  assert.equal(
    fs.readFileSync(path.join(r.root, "coverage-hooks.txt"), "utf8"),
    LISTING,
  );
  assert.equal(
    fs.readFileSync(r.summary, "utf8"),
    [
      "### Go statement coverage",
      "",
      "These totals combine all package tests. Individual test log percentages use the whole module as their denominator.",
      "The uploaded Cobertura report measures line coverage, so its percentage can differ.",
      "",
      "| Module | Statement coverage |",
      "| --- | --- |",
      "| `hooks/go` | 93.5% |",
      "",
    ].join("\n"),
  );
});

test("totals appends to an existing summary", (t) => {
  const f = fakeGo();
  const r = run(t, ["totals"], {
    run: f.goRun,
    setup: (root) =>
      fs.writeFileSync(path.join(root, "summary.md"), "earlier\n"),
  });
  assert.equal(r.status, 0);
  assert.ok(
    fs
      .readFileSync(r.summary, "utf8")
      .startsWith("earlier\n### Go statement coverage\n"),
  );
});

test("a failing go exits nonzero but still leaves coverage-hooks.txt for the upload", (t) => {
  const f = fakeGo({ status: 3, stdout: "partial\n" });
  const r = run(t, ["totals"], { run: f.goRun });
  assert.equal(r.status, 3);
  assert.equal(
    fs.readFileSync(path.join(r.root, "coverage-hooks.txt"), "utf8"),
    "partial\n",
  );
  assert.equal(fs.existsSync(r.summary), false);
});

test("a go that cannot start is reported and exits nonzero", (t) => {
  const f = fakeGo({ status: null, error: new Error("spawn go ENOENT") });
  const r = run(t, ["totals"], { run: f.goRun });
  assert.notEqual(r.status, 0);
  assert.match(r.errors.join("\n"), /^ci-coverage: cannot start go/);
});

test("a go that was killed is reported and exits nonzero", (t) => {
  const f = fakeGo({ status: null, signal: "SIGKILL" });
  const r = run(t, ["totals"], { run: f.goRun });
  assert.equal(r.status, 1);
  assert.match(r.errors.join("\n"), /^ci-coverage: go was killed by SIGKILL/);
});

test("totals without GITHUB_STEP_SUMMARY exits nonzero and runs nothing", (t) => {
  const f = fakeGo();
  const r = run(t, ["totals"], { run: f.goRun, summary: false });
  assert.equal(r.status, 1);
  assert.equal(f.calls.length, 0);
  assert.match(
    r.errors.join("\n"),
    /^ci-coverage: GITHUB_STEP_SUMMARY is not set/,
  );
});

test("a missing or unknown subcommand prints the usage and exits 2", (t) => {
  for (const argv of [
    [],
    ["bogus"],
    ["totals", "extra"],
    ["merge-profiles", "extra"],
  ]) {
    const r = run(t, argv, {
      run: () => assert.fail("nothing may start"),
    });
    assert.equal(r.status, 2, argv.join(" "));
    assert.deepEqual(r.errors, [
      "usage: ci-coverage.ts require-profiles|merge-profiles|totals|combine-ts",
    ]);
  }
});

test("run as a script, the module sets the exit status", () => {
  const result = spawnSync(process.execPath, [MODULE], { encoding: "utf8" });
  assert.equal(result.status, 2);
  assert.match(
    result.stderr,
    /usage: ci-coverage\.ts require-profiles\|merge-profiles\|totals/,
  );
});

// The coverage CLI's calls, as the injected runner sees them.
function fakeCli(results: Partial<CommandResult>[] = []) {
  const calls: { command: string; args: string[]; cwd: string | undefined }[] =
    [];
  const cliRun: CommandRun = (command, args, options) => {
    const result = results[calls.length] ?? {};
    calls.push({ command, args, cwd: options.cwd });
    return { status: 0, stdout: "", ...result };
  };
  return { calls, cliRun };
}

function lcov(root: string, shard: string, contents: string): void {
  const dir = path.join(root, "lcov", `ts-coverage-lcov-${shard}`);
  fs.mkdirSync(dir, { recursive: true });
  fs.writeFileSync(path.join(dir, "lcov.info"), contents);
}

const CLI = ".github/tools/src/coverage/cli.ts";

test("combine-ts converts every shard's report, then appends the summary", (t) => {
  const f = fakeCli([{}, { stdout: "| total | 100.0% |\n" }]);
  const r = run(t, ["combine-ts", "tools-1", "evals-2"], {
    run: f.cliRun,
    setup: (root) => {
      lcov(root, "tools-1", "SF:a\n");
      lcov(root, "evals-2", "SF:b\n");
    },
  });
  assert.equal(r.status, 0, r.errors.join("\n"));
  const inputs = [
    "--input",
    `.github/tools:${path.join(r.root, "lcov", "ts-coverage-lcov-tools-1", "lcov.info")}`,
    "--input",
    `.github/evals:${path.join(r.root, "lcov", "ts-coverage-lcov-evals-2", "lcov.info")}`,
  ];
  assert.deepEqual(
    f.calls.map((c) => [c.command, ...c.args]),
    [
      [
        process.execPath,
        CLI,
        "lcov",
        "--root",
        r.root,
        ...inputs,
        "--out",
        "cobertura-typescript.xml",
      ],
      [process.execPath, CLI, "summary", "--root", r.root, ...inputs],
    ],
  );
  assert.equal(f.calls[0]!.cwd, r.root);
  assert.equal(fs.readFileSync(r.summary, "utf8"), "| total | 100.0% |\n");
});

test("combine-ts names a missing or empty shard report and starts nothing", (t) => {
  for (const [label, setup] of [
    ["missing", () => {}],
    ["empty", (root: string) => lcov(root, "tools-1", "")],
  ] as const) {
    const f = fakeCli();
    const r = run(t, ["combine-ts", "tools-1"], { run: f.cliRun, setup });
    assert.equal(r.status, 1, label);
    assert.deepEqual(r.out, [
      "::error::Missing or empty LCOV report for tools-1.",
    ]);
    assert.equal(f.calls.length, 0, label);
  }
});

test("combine-ts stops at the first shard without a report", (t) => {
  const f = fakeCli();
  const r = run(t, ["combine-ts", "a-1", "b-1", "c-1"], {
    run: f.cliRun,
    setup: (root) => lcov(root, "c-1", "SF:c\n"),
  });
  assert.equal(r.status, 1);
  assert.deepEqual(r.out, ["::error::Missing or empty LCOV report for a-1."]);
});

test("combine-ts stops at a failing lcov call before summary", (t) => {
  const f = fakeCli([{ status: 5 }]);
  const r = run(t, ["combine-ts", "tools-1"], {
    run: f.cliRun,
    setup: (root) => lcov(root, "tools-1", "SF:a\n"),
  });
  assert.equal(r.status, 5);
  assert.equal(f.calls.length, 1);
  assert.equal(fs.existsSync(r.summary), false);
});

test("combine-ts returns the status of a failing summary call", (t) => {
  const f = fakeCli([{}, { status: 6, stdout: "| partial | 50.0% |\n" }]);
  const r = run(t, ["combine-ts", "tools-1"], {
    run: f.cliRun,
    setup: (root) => lcov(root, "tools-1", "SF:a\n"),
  });
  assert.equal(r.status, 6);
  assert.equal(f.calls.length, 2);
  // The summary the failing call printed is kept, as the shell redirect left it.
  assert.equal(fs.readFileSync(r.summary, "utf8"), "| partial | 50.0% |\n");
});

test("combine-ts reports a CLI that cannot start or was killed", (t) => {
  for (const [dying, expected] of [
    [
      { status: null, error: new Error("spawn failed") },
      /^ci-coverage: cannot start node/,
    ],
    [
      { status: null, signal: "SIGKILL" as const },
      /^ci-coverage: node was killed by SIGKILL/,
    ],
  ] as const) {
    const f = fakeCli([dying]);
    const r = run(t, ["combine-ts", "tools-1"], {
      run: f.cliRun,
      setup: (root) => lcov(root, "tools-1", "SF:a\n"),
    });
    assert.equal(r.status, 1);
    assert.match(r.errors.join("\n"), expected);
  }
});

test("combine-ts without its inputs exits nonzero and starts nothing", (t) => {
  const f = fakeCli();
  assert.equal(run(t, ["combine-ts"], { run: f.cliRun }).status, 2);
  assert.equal(
    run(t, ["combine-ts", "tools-1"], { run: f.cliRun, summary: false }).status,
    1,
  );
  const errors: string[] = [];
  assert.equal(
    main(["combine-ts", "tools-1"], {
      env: { GITHUB_STEP_SUMMARY: "x" },
      run: f.cliRun,
      report: (line) => errors.push(line),
    }),
    1,
  );
  assert.match(errors.join("\n"), /^ci-coverage: RUNNER_TEMP is not set/);
  assert.equal(f.calls.length, 0);
});

test("combine-ts proves its arguments against the real coverage CLI", (t) => {
  const root = tempDir(t);
  fs.cpSync(
    path.resolve(import.meta.dirname, "../coverage"),
    path.join(root, ".github/tools/src/coverage"),
    { recursive: true },
  );
  const scripts = path.join(root, ".github/tools/src/scripts");
  fs.mkdirSync(scripts, { recursive: true });
  fs.copyFileSync(
    path.join(import.meta.dirname, "mutation-source.ts"),
    path.join(scripts, "mutation-source.ts"),
  );
  lcov(root, "tools-1", "SF:src/sample.ts\nDA:1,1\nDA:2,1\nend_of_record\n");
  const summary = path.join(root, "summary.md");
  const errors: string[] = [];
  const status = main(["combine-ts", "tools-1"], {
    env: {
      RUNNER_TEMP: root,
      GITHUB_WORKSPACE: root,
      GITHUB_STEP_SUMMARY: summary,
    },
    cwd: root,
    report: (line) => errors.push(line),
  });
  assert.equal(status, 0, errors.join("\n"));
  const xml = fs.readFileSync(
    path.join(root, "cobertura-typescript.xml"),
    "utf8",
  );
  assert.match(xml, /lines-covered="2"/);
  assert.match(xml, /lines-valid="2"/);
  assert.match(xml, /filename="\.github\/tools\/src\/sample.ts"/);
  const markdown = fs.readFileSync(summary, "utf8");
  assert.match(markdown, /^\| \.github\/tools \| 100\.0% \(2\/2\) \|/m);
  assert.match(markdown, /^\| total \| 100\.0% \(2\/2\) \|/m);
});
