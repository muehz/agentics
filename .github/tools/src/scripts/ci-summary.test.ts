import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { spawnSync } from "node:child_process";
import test from "node:test";
import YAML from "yaml";
import {
  SUMMARY_JOBS,
  JOB_ORDER,
  expectedJobResult,
  planFor,
  validateSelection,
} from "./ci-areas.ts";

interface TestJob {
  result: string;
  outputs?: Record<string, unknown>;
}
interface ScopeJob extends TestJob {
  outputs: Record<string, unknown>;
}
type TestJobId = (typeof ALL)[number];
type TestNeeds = Record<TestJobId, TestJob> & {
  scope: ScopeJob;
  [id: string]: TestJob;
};
interface NeedsOptions {
  event?: string;
  mode?: string;
  complete?: string;
  scopeResult?: string;
  reason?: string;
  nativeResult?: string;
  postMergeResult?: string;
}
const ROOT = path.resolve(import.meta.dirname, "../../../..");
const SCRIPT = path.join(import.meta.dirname, "ci-summary.ts");
const CHEAP = [
  "validate-claude-native",
  "validate-antigravity-native",
  "conformance",
  "format",
  "shellcheck",
  "gh-aw-lock-drift",
] as const;
const NATIVE = ["go-suite", "ts-suite"] as const;
const POST_MERGE = ["manual"] as const;
const PR_ONLY = ["pr-metadata", "pr-version-fields"] as const;
const ALL = ["scope", ...CHEAP, ...NATIVE, ...POST_MERGE, ...PR_ONLY] as const;
const IDS = {
  base: "a".repeat(40),
  head: "b".repeat(40),
  merge: "c".repeat(40),
};

// Matches one summary row by its display name, ignoring the status icon.
function row(id: TestJobId, expected: string, actual: string): RegExp {
  const label = SUMMARY_JOBS[id][0].replace(/[.*+?^${}()|[\]\\/]/g, "\\$&");
  return new RegExp(
    `^\\| [^|]+ \\| ${label} \\| ${expected} \\| ${actual} \\|$`,
    "m",
  );
}

function job(result: string, outputs: Record<string, unknown>): ScopeJob;
function job(result: string): TestJob;
function job(result: string, outputs?: Record<string, unknown>): TestJob {
  return { result, ...(outputs ? { outputs } : {}) };
}

function withSchedule(needs: TestNeeds): TestNeeds {
  const { outputs } = needs.scope;
  outputs.effective_selection = [
    "mode",
    "complete",
    "base",
    "head",
    "merge",
    "reason",
    "areas",
    "shards",
    "ts_shards",
    "race",
  ]
    .map((field) => outputs[field] ?? "")
    .join("|");
  return needs;
}

function makeNeeds({
  event = "pull_request",
  mode = "full",
  complete = "false",
  scopeResult = "success",
  reason = event === "pull_request" ? "unknown-path" : "event-full",
  nativeResult,
  postMergeResult,
}: NeedsOptions = {}): TestNeeds {
  const needs: Record<string, TestJob> & { scope: ScopeJob } = {
    scope: job(scopeResult, {
      mode,
      complete,
      ...(event === "pull_request" ? IDS : { base: "", head: "", merge: "" }),
      reason,
      // Empty areas are absent from toJSON(needs) on asset and full runs.
      shards: mode === "asset-only" ? "[]" : '["hooks"]',
      ts_shards: mode === "asset-only" ? "[]" : '["ts-tools"]',
      race: mode === "asset-only" ? "false" : "true",
      ts_coverage: "false",
    }),
  };
  for (const id of CHEAP) needs[id] = job("success");
  // A manual dispatch is the only event that runs the post-merge jobs.
  for (const id of NATIVE) needs[id] = job(nativeResult || "success");
  for (const id of POST_MERGE)
    needs[id] = job(
      postMergeResult ||
        (event === "workflow_dispatch" ? "success" : "skipped"),
    );
  for (const id of PR_ONLY)
    needs[id] = job(event === "pull_request" ? "success" : "skipped");
  // The loops above initialize every member of ALL; scope always carries outputs.
  return withSchedule(needs as TestNeeds);
}

// Reads the file in one call instead of checking it first, so nothing can change
// between the check and the read. A missing path or a directory yields "".
function readIfFile(file: string): string {
  try {
    return fs.readFileSync(file, "utf8");
  } catch (error) {
    if (
      error !== null &&
      typeof error === "object" &&
      "code" in error &&
      (error.code === "ENOENT" || error.code === "EISDIR")
    )
      return "";
    throw error;
  }
}

function invoke(
  needs: unknown,
  event = "pull_request",
  options: { summaryFile?: string } = {},
) {
  const directory = fs.mkdtempSync(
    path.join(os.tmpdir(), "agentics-ci-summary-"),
  );
  const needsFile = path.join(directory, "needs.json");
  const summaryFile = options.summaryFile || path.join(directory, "summary.md");
  fs.writeFileSync(needsFile, JSON.stringify(needs));
  const result = spawnSync(
    process.execPath,
    [
      SCRIPT,
      "--needs-env",
      "AGX_CI_NEEDS_JSON",
      "--event-name-env",
      "GITHUB_EVENT_NAME",
      "--summary-env",
      "GITHUB_STEP_SUMMARY",
    ],
    {
      cwd: ROOT,
      encoding: "utf8",
      env: {
        ...process.env,
        AGX_CI_NEEDS_JSON: fs.readFileSync(needsFile, "utf8"),
        GITHUB_EVENT_NAME: event,
        GITHUB_STEP_SUMMARY: summaryFile,
      },
    },
  );
  const summary = readIfFile(summaryFile);
  fs.rmSync(directory, { recursive: true, force: true });
  return { ...result, summary };
}

function assertRed(
  result: { status: number | null; stdout: string; stderr: string },
  pattern: RegExp,
): void {
  assert.equal(
    result.status,
    1,
    `expected exit 1, got ${result.status}: ${result.stderr}`,
  );
  assert.match(`${result.stdout}\n${result.stderr}`, pattern);
}

function areaNeeds({
  areas,
  goSuite = planFor(areas.split(",")).shards.length ? "success" : "skipped",
  tsSuite = planFor(areas.split(",")).tsShards.length ? "success" : "skipped",
}: {
  areas: string;
  goSuite?: string;
  tsSuite?: string;
}): TestNeeds {
  const needs = makeNeeds({
    mode: "area-scoped",
    complete: "true",
    reason: "area-scoped",
  });
  needs.scope.outputs.areas = areas;
  needs.scope.outputs.shards = JSON.stringify(planFor(areas.split(",")).shards);
  needs.scope.outputs.ts_shards = JSON.stringify(
    planFor(areas.split(",")).tsShards,
  );
  needs.scope.outputs.race = String(planFor(areas.split(",")).race);
  needs["go-suite"] = job(goSuite);
  needs["ts-suite"] = job(tsSuite);
  return withSchedule(needs);
}

test("a hooks change on a pull request expects the Go suite to run", () => {
  const result = invoke(areaNeeds({ areas: "hooks" }));
  assert.equal(result.status, 0, result.stderr);
  assert.match(result.summary, /Areas: `hooks`/);
  assert.match(result.summary, row("go-suite", "success", "success"));
  assertRed(
    invoke(areaNeeds({ areas: "hooks", goSuite: "skipped" })),
    /native-job:go-suite/,
  );
});

test("a skills-only pull request expects the Go suite and no TypeScript suite", () => {
  const ok = invoke(areaNeeds({ areas: "skills" }));
  assert.equal(ok.status, 0, ok.stderr);
  assert.match(ok.summary, row("ts-suite", "skipped", "skipped"));
  assert.match(ok.summary, row("go-suite", "success", "success"));
  // Each job that ran for a skills-only change is a violation, so the scope rule cannot drift back.
  assertRed(
    invoke(areaNeeds({ areas: "skills", tsSuite: "success" })),
    /native-job:ts-suite/,
  );
});

test("a tools-only change expects no Go suite, and a ran Go suite is a violation", () => {
  const ok = invoke(areaNeeds({ areas: "tools" }));
  assert.equal(ok.status, 0, ok.stderr);
  assert.match(ok.summary, row("go-suite", "skipped", "skipped"));
  assertRed(
    invoke(areaNeeds({ areas: "tools", goSuite: "success" })),
    /native-job:go-suite/,
  );
});

test("area-scoped requires the shards output to equal the recomputed plan", () => {
  const wrong: Record<string, [string, string]> = {
    "wrong shards for a tools-only plan": ["tools", '["hooks"]'],
    "an empty selection": ["hooks", "[]"],
    "whitespace in the serialization": ["hooks", '[ "hooks" ]'],
  };
  for (const [areas, shards] of Object.values(wrong)) {
    const needs = areaNeeds({
      areas,
    });
    needs.scope.outputs.shards = shards;
    assertRed(invoke(needs), /selector-invalid-shards/);
  }
  const absent = areaNeeds({ areas: "tools" });
  delete absent.scope.outputs.shards;
  assertRed(invoke(absent), /selector-invalid-output/);
});

test("TypeScript suite failures are required and missing jobs fail closed", () => {
  const failed = makeNeeds();
  failed["ts-suite"].result = "failure";
  assertRed(invoke(failed), /native-job:ts-suite/);
  const missing = makeNeeds();
  Reflect.deleteProperty(missing, "ts-suite");
  assertRed(invoke(missing), /missing-job:ts-suite/);
});

test("asset-only changes allow the TypeScript suite to skip", () => {
  const needs = makeNeeds({
    mode: "asset-only",
    complete: "true",
    reason: "eligible",
    nativeResult: "skipped",
  });
  const result = invoke(needs);
  assert.equal(result.status, 0, result.stderr);
  assert.match(result.summary, row("ts-suite", "skipped", "skipped"));
});

test("TypeScript shards are recomputed from validated areas", () => {
  for (const [areas, shards] of [
    ["tools", "[]"],
    // The retired eval engine shard is no longer a plan output.
    ["evals", '["ts-evals"]'],
    ["hooks", "[]"],
    ["evals", '["ts-tools","ts-tools"]'],
  ] as const) {
    const needs = areaNeeds({
      areas,
    });
    needs.scope.outputs.ts_shards = shards;
    assertRed(invoke(needs), /selector-invalid-ts-shards/);
  }
  const absent = areaNeeds({ areas: "tools" });
  delete absent.scope.outputs.ts_shards;
  assertRed(invoke(absent), /selector-invalid-output/);
});

test("area-scoped is rejected with an empty, unsorted, duplicated, or unknown areas value", () => {
  for (const areas of [
    "",
    "tools,hooks",
    "hooks,hooks",
    "docs",
    "hooks,",
    ",hooks",
  ]) {
    assertRed(invoke(areaNeeds({ areas })), /selector-invalid-output/);
  }
  const absent = areaNeeds({ areas: "hooks" });
  delete absent.scope.outputs.areas;
  assertRed(invoke(absent), /selector-invalid-output/);
});

test("area-scoped with a wrong reason or complete flag is rejected", () => {
  const reason = areaNeeds({ areas: "hooks" });
  reason.scope.outputs.reason = "other-path";
  assertRed(invoke(reason), /selector-invalid-reason/);
  const complete = areaNeeds({ areas: "hooks" });
  complete.scope.outputs.complete = "false";
  assertRed(invoke(complete), /selector-invalid-output/);
});

test("area-scoped from a failed scope job is rejected", () => {
  const needs = areaNeeds({ areas: "hooks" });
  needs.scope.result = "failure";
  assertRed(invoke(needs), /selector-invalid-output/);
});

test("scoped modes are rejected on schedule and manual runs, and on a push that reports no validated commit", () => {
  for (const event of ["schedule", "workflow_dispatch"]) {
    const scoped = makeNeeds({
      event,
      mode: "area-scoped",
      complete: "true",
      reason: "area-scoped",
    });
    assertRed(invoke(scoped, event), /selector-forbidden-area-mode/);
    const asset = makeNeeds({
      event,
      mode: "asset-only",
      complete: "true",
      reason: "eligible",
    });
    assertRed(invoke(asset, event), /selector-forbidden-asset-mode/);
  }
  const push = makeNeeds({
    event: "push",
    mode: "area-scoped",
    complete: "true",
    reason: "event-full",
  });
  assertRed(invoke(push, "push"), /selector-forbidden-area-mode/);
});

// A scoped push: base is the last validated commit, head is the pushed commit, and there is no merge commit.
function pushNeeds({
  areas,
  mode = "area-scoped",
}: {
  areas?: string;
  mode?: string;
}): TestNeeds {
  const list = areas ? areas.split(",") : [];
  const plan = planFor(list);
  const needs = makeNeeds({
    event: "push",
    mode,
    complete: "true",
    reason: mode === "asset-only" ? "eligible" : "area-scoped",
  });
  needs.scope.outputs.base = IDS.base;
  needs.scope.outputs.head = IDS.head;
  delete needs.scope.outputs.merge;
  if (mode === "area-scoped") {
    needs.scope.outputs.areas = areas;
    needs.scope.outputs.shards = JSON.stringify(plan.shards);
    needs.scope.outputs.ts_shards = JSON.stringify(plan.tsShards);
    needs.scope.outputs.race = String(plan.race);
  }
  needs["go-suite"] = job(plan.shards.length > 0 ? "success" : "skipped");
  needs["ts-suite"] = job(plan.tsShards.length > 0 ? "success" : "skipped");
  for (const id of POST_MERGE) needs[id] = job("skipped");
  return withSchedule(needs);
}

test("a scoped push never runs the macOS and Windows jobs, whatever its areas", () => {
  for (const areas of ["evals", "tools", "skills", "hooks", "evals,hooks"]) {
    const result = invoke(pushNeeds({ areas }), "push");
    assert.equal(result.status, 0, `${areas}: ${result.stderr}`);
    assert.match(result.summary, row("manual", "skipped", "skipped"));
  }
});

test("a scoped push that runs a macOS or Windows job is a violation", () => {
  for (const id of POST_MERGE) {
    for (const areas of ["hooks", "tools"]) {
      const ran = pushNeeds({ areas });
      ran[id] = job("success");
      assertRed(invoke(ran, "push"), new RegExp(`post-merge-job:${id}`));
    }
  }
});

test("an asset-only push skips every native and post-merge job", () => {
  const needs = pushNeeds({ mode: "asset-only" });
  const result = invoke(needs, "push");
  assert.equal(result.status, 0, result.stderr);
  for (const id of [...NATIVE, ...POST_MERGE]) {
    const ran = pushNeeds({ mode: "asset-only" });
    ran[id] = job("success");
    assertRed(invoke(ran, "push"), new RegExp(`job:${id}`));
  }
});

test("a scoped push must name its base and head and must not name a merge", () => {
  const withMerge = pushNeeds({ areas: "evals" });
  withMerge.scope.outputs.merge = IDS.merge;
  assertRed(invoke(withMerge, "push"), /selector-invalid-identity/);
  for (const name of ["base", "head"]) {
    const missing = pushNeeds({ areas: "evals" });
    delete missing.scope.outputs[name];
    assertRed(invoke(missing, "push"), /selector-invalid-identity/);
  }
});

test("complete provider selections preserve successful summary bytes and job order", () => {
  const fullPush = makeNeeds({ event: "push" });
  for (const key of ["base", "head", "merge"])
    delete fullPush.scope.outputs[key];
  const cases: [string, TestNeeds][] = [
    ["pull_request", makeNeeds()],
    [
      "pull_request",
      makeNeeds({
        mode: "asset-only",
        complete: "true",
        reason: "eligible",
        nativeResult: "skipped",
      }),
    ],
    ["pull_request", areaNeeds({ areas: "skills" })],
    ["pull_request", areaNeeds({ areas: "hooks,tools" })],
    ["push", pushNeeds({ areas: "evals" })],
    ["push", pushNeeds({ mode: "asset-only" })],
    ["push", fullPush],
    ["schedule", makeNeeds({ event: "schedule" })],
    ["workflow_dispatch", makeNeeds({ event: "workflow_dispatch" })],
  ];
  for (const [event, needs] of cases) {
    const selection = validateSelection(needs.scope, event);
    const result = invoke(needs, event);
    assert.equal(result.status, 0, result.stderr);
    assert.equal(result.stdout, "ci-summary: success\n");
    assert.equal(result.stderr, "");
    const rows = JOB_ORDER.map((id) => {
      const expected = expectedJobResult(id, event, selection);
      const actual = needs[id].result;
      assert.equal(actual, expected, id);
      return `| ${expected === "success" ? "✅" : "➖"} | ${SUMMARY_JOBS[id][0]} | ${expected} | ${actual} |`;
    });
    assert.equal(
      result.summary,
      [
        "## ✅ Validation passed",
        "",
        "| | Check | Expected | Actual |",
        "| --- | --- | --- | --- |",
        ...rows,
        "",
        "<details><summary>Change selection</summary>",
        "",
        `Event: \`${event}\`  `,
        `Mode: \`${selection.mode}\`  `,
        `Reason: \`${selection.reason}\`  `,
        `Areas: \`${selection.areas}\`  `,
        `Base: \`${selection.base}\`  `,
        `Head: \`${selection.head}\`  `,
        `Merge: \`${selection.merge}\``,
        "",
        "</details>",
        "",
      ].join("\n"),
    );
  }
});

test("missing or contradictory reduced evidence requires every native suite", () => {
  const asset = () =>
    makeNeeds({
      mode: "asset-only",
      complete: "true",
      reason: "eligible",
      nativeResult: "skipped",
    });
  const tools = () => areaNeeds({ areas: "tools" });
  const cases: [() => TestNeeds, string, unknown, string][] = [
    [tools, "race", undefined, "output"],
    [tools, "race", "true", "race"],
    [() => areaNeeds({ areas: "hooks" }), "race", "false", "race"],
    [asset, "race", undefined, "output"],
    [asset, "race", true, "output"],
    [asset, "race", "true", "race"],
    [asset, "areas", "tools", "output"],
    [asset, "shards", undefined, "output"],
    [asset, "shards", '["hooks"]', "shards"],
    [asset, "ts_shards", undefined, "output"],
    [asset, "ts_shards", "[ ]", "ts-shards"],
    [asset, "base", "0".repeat(40), "identity"],
    [tools, "head", IDS.merge, "identity"],
    // An aggregate go-suite success can still hide omitted race/fuzz legs.
    [() => makeNeeds(), "race", "false", "race"],
  ];
  for (const [fixture, key, value, diagnostic] of cases) {
    const needs = fixture();
    const outputs = needs.scope.outputs;
    if (value === undefined) delete outputs[key];
    else outputs[key] = value;
    const errors = [
      "selector-invalid-" + diagnostic,
      ...NATIVE.filter((id) => needs[id].result !== "success").map(
        (id) => `native-job:${id}`,
      ),
    ];
    const result = invoke(needs);
    assert.equal(result.status, 1);
    assert.equal(result.stdout, "");
    assert.equal(result.stderr, `ci-summary: ${errors.join(",")}\n`);
    for (const id of NATIVE)
      assert.match(result.summary, row(id, "success", needs[id].result));
  }
});

test("absent or disagreeing job publication requires full coverage after raw validation", () => {
  for (const change of ["missing", "truncated", "disagreeing"]) {
    const needs = areaNeeds({ areas: "tools" });
    const proof = String(needs.scope.outputs.effective_selection);
    if (change === "missing") delete needs.scope.outputs.effective_selection;
    else
      needs.scope.outputs.effective_selection =
        change === "truncated"
          ? proof.slice(0, -2)
          : proof.replace("|tools|", "|hooks|");
    const result = invoke(needs);
    assert.equal(result.status, 1);
    assert.equal(
      result.stderr,
      "ci-summary: selector-invalid-schedule,native-job:go-suite\n",
    );
    assert.match(result.summary, row("go-suite", "success", "skipped"));
    assert.match(result.summary, row("ts-suite", "success", "success"));
    needs["go-suite"].result = "success";
    const full = invoke(needs);
    assert.equal(full.stderr, "ci-summary: selector-invalid-schedule\n");
    assert.match(full.summary, row("go-suite", "success", "success"));
  }
});

test("raw diagnostics precede complete proof comparison", () => {
  const needs = areaNeeds({ areas: "tools" });
  delete needs.scope.outputs.head;
  const result = invoke(needs);
  assert.equal(result.status, 1);
  assert.equal(result.stdout, "");
  assert.equal(
    result.stderr,
    "ci-summary: selector-invalid-identity,native-job:go-suite\n",
  );
  assert.match(result.summary, row("go-suite", "success", "skipped"));
  assert.match(result.summary, row("ts-suite", "success", "success"));
});

test("proof matching follows Actions case folding after strict raw validation", () => {
  const needs = areaNeeds({ areas: "tools" });
  needs.scope.outputs.effective_selection = String(
    needs.scope.outputs.effective_selection,
  ).toUpperCase();
  const result = invoke(needs);
  assert.equal(result.status, 0);
  assert.equal(result.stdout, "ci-summary: success\n");
  assert.equal(result.stderr, "");
  needs.scope.outputs.head = String(needs.scope.outputs.head).toUpperCase();
  const invalid = invoke(needs);
  assert.equal(invalid.status, 1);
  assert.equal(
    invalid.stderr,
    "ci-summary: selector-invalid-identity,native-job:go-suite\n",
  );
});

test("existing rejection diagnostics precede full-coverage job violations", () => {
  const cases: [string, unknown, string][] = [
    ["mode", "partial", "mode"],
    ["complete", false, "complete"],
    ["complete", "false", "output"],
    ["head", undefined, "identity"],
    ["merge", "not-a-sha", "identity"],
    ["reason", "eligible", "reason"],
    ["areas", "tools,hooks", "output"],
    ["shards", '["hooks"]', "shards"],
    ["ts_shards", "[]", "ts-shards"],
  ];
  for (const [key, value, diagnostic] of cases) {
    const needs = areaNeeds({ areas: "tools" });
    if (value === undefined) delete needs.scope.outputs[key];
    else needs.scope.outputs[key] = value;
    const reason = "selector-invalid-" + diagnostic;
    const result = invoke(needs);
    assert.equal(result.status, 1);
    assert.equal(result.stdout, "");
    assert.equal(result.stderr, `ci-summary: ${reason},native-job:go-suite\n`);
    for (const id of NATIVE)
      assert.match(result.summary, row(id, "success", needs[id].result));
  }
});

test("a failed scope retains diagnostic precedence across all job families", () => {
  const needs = makeNeeds({ scopeResult: "failure" });
  needs.scope.outputs.complete = "true";
  for (const id of [
    "format",
    "gh-aw-lock-drift",
    "ts-suite",
    "go-suite",
    "manual",
    "pr-metadata",
  ])
    needs[id]!.result = "failure";
  const errors = [
    "selector-failed",
    "selector-invalid-output",
    "required-job:format",
    "required-job:gh-aw-lock-drift",
    "native-job:go-suite",
    "native-job:ts-suite",
    "post-merge-job:manual",
    "pr-only-job:pr-metadata",
  ];
  const result = invoke(needs);
  assert.equal(result.status, 1);
  assert.equal(result.stdout, "");
  assert.equal(result.stderr, `ci-summary: ${errors.join(",")}\n`);
});

test("a push whose validated commit is not an ancestor runs in full and passes", () => {
  const needs = makeNeeds({ event: "push", reason: "base-not-ancestor" });
  needs.scope.outputs.base = IDS.base;
  needs.scope.outputs.head = IDS.head;
  delete needs.scope.outputs.merge;
  const result = invoke(needs, "push");
  assert.equal(result.status, 0, result.stderr);
});

test("other-path is an accepted full reason and still expects the native families to run", () => {
  const result = invoke(makeNeeds({ reason: "other-path" }));
  assert.equal(result.status, 0, result.stderr);
});

test("asset-only PR requires exact selector outputs and skips every native family", () => {
  const result = invoke(
    makeNeeds({
      mode: "asset-only",
      complete: "true",
      reason: "eligible",
      nativeResult: "skipped",
    }),
  );
  assert.equal(result.status, 0, result.stderr);
  assert.match(result.summary, row("scope", "success", "success"));
  assert.match(result.summary, row("go-suite", "skipped", "skipped"));
  assert.match(result.summary, /Mode: `asset-only`/);
  assert.match(result.summary, /Reason: `eligible`/);
  for (const value of Object.values(IDS))
    assert.match(result.summary, new RegExp(value));
});

test("a run that is not a pull request has no identity outputs and still passes", () => {
  // GitHub omits empty step outputs from toJSON(needs), so base, head, and merge are absent.
  for (const event of ["push", "schedule", "workflow_dispatch"]) {
    const needs = makeNeeds({ event });
    for (const name of ["base", "head", "merge"])
      delete needs.scope.outputs[name];
    const result = invoke(needs, event);
    assert.equal(result.status, 0, `${event}: ${result.stderr}`);
  }
});

test("full PR and backstop runs require successful native families", () => {
  for (const [event, reason] of [
    ["pull_request", "unknown-path"],
    ["push", "event-full"],
    ["schedule", "event-full"],
    ["workflow_dispatch", "event-full"],
  ] as const) {
    const result = invoke(makeNeeds({ event, reason }), event);
    assert.equal(result.status, 0, `${event}: ${result.stderr}`);
    assert.match(result.summary, /Expected \| Actual/);
    // A full run tests Go on every event, and only a manual dispatch runs the macOS and Windows jobs.
    assert.match(result.summary, row("go-suite", "success", "success"));
    assert.match(
      result.summary,
      event === "workflow_dispatch"
        ? row("manual", "success", "success")
        : row("manual", "skipped", "skipped"),
    );
    for (const id of PR_ONLY) {
      const expected = event === "pull_request" ? "success" : "skipped";
      assert.match(
        result.summary,
        row(id, expected, expected),
        `${event}: ${id} row`,
      );
    }
  }
});

test("each cheap family failure is reported distinctly", () => {
  for (const id of CHEAP) {
    for (const status of ["failure", "skipped", "cancelled"]) {
      const needs = makeNeeds();
      needs[id].result = status;
      assertRed(invoke(needs), new RegExp(`required-job:${id}`));
    }
  }
});

test("each native family failure is reported distinctly", () => {
  for (const event of ["pull_request", "push"]) {
    for (const id of NATIVE) {
      for (const status of ["failure", "skipped", "cancelled"]) {
        const needs = makeNeeds({ event });
        needs[id].result = status;
        assertRed(invoke(needs, event), new RegExp(`native-job:${id}`));
      }
    }
  }
});

test("asset-only accepts only skipped native families", () => {
  for (const id of NATIVE) {
    const needs = makeNeeds({
      mode: "asset-only",
      complete: "true",
      reason: "eligible",
      nativeResult: "skipped",
    });
    needs[id].result = "success";
    assertRed(invoke(needs), new RegExp(`native-job:${id}`));
  }
});

test("post-merge families skip on pull requests, pushes and the schedule, and run on manual dispatch", () => {
  for (const id of POST_MERGE) {
    for (const status of ["success", "failure", "cancelled"]) {
      const needs = makeNeeds();
      needs[id].result = status;
      assertRed(invoke(needs), new RegExp(`post-merge-job:${id}`));
    }
    for (const event of ["push", "schedule"]) {
      const ran = makeNeeds({ event });
      ran[id].result = "success";
      assertRed(invoke(ran, event), new RegExp(`post-merge-job:${id}`));
    }
    const assetOnly = makeNeeds({
      mode: "asset-only",
      complete: "true",
      reason: "eligible",
      nativeResult: "skipped",
    });
    assetOnly[id].result = "success";
    assertRed(invoke(assetOnly), new RegExp(`post-merge-job:${id}`));
    for (const status of ["failure", "skipped", "cancelled"]) {
      const needs = makeNeeds({ event: "workflow_dispatch" });
      needs[id].result = status;
      assertRed(
        invoke(needs, "workflow_dispatch"),
        new RegExp(`post-merge-job:${id}`),
      );
    }
  }
});

test("an unknown event is rejected", () => {
  assertRed(invoke(makeNeeds({ event: "release" }), "release"), /event-input/);
});

test("selector failure remains red when native fallback succeeds", () => {
  for (const scopeResult of ["failure", "cancelled", "skipped"]) {
    const needs = makeNeeds({ scopeResult });
    assertRed(invoke(needs), /selector-failed/);
  }
});

test("scope output tampering cannot authorize asset-only skips", () => {
  const invalidMode = makeNeeds();
  invalidMode.scope.outputs.mode = "partial";
  assertRed(invoke(invalidMode), /selector-invalid-mode/);

  const invalidShape = makeNeeds();
  Object.assign(invalidShape.scope, { outputs: null });
  assertRed(invoke(invalidShape), /selector-invalid-output/);

  const nonStringComplete = makeNeeds();
  nonStringComplete.scope.outputs.complete = true;
  assertRed(invoke(nonStringComplete), /selector-invalid-complete/);

  const incomplete = makeNeeds({
    mode: "asset-only",
    complete: "false",
    reason: "eligible",
    nativeResult: "skipped",
  });
  assertRed(invoke(incomplete), /selector-invalid-output/);

  const missingIdentity = makeNeeds({ mode: "full", complete: "false" });
  missingIdentity.scope.outputs.base = "";
  assertRed(invoke(missingIdentity), /selector-invalid-identity/);

  const invalidIdentity = makeNeeds({ mode: "full", complete: "false" });
  invalidIdentity.scope.outputs.head = "Z".repeat(40);
  assertRed(invoke(invalidIdentity), /selector-invalid-identity/);

  const arrayIdentity = makeNeeds({ mode: "full", complete: "false" });
  arrayIdentity.scope.outputs.merge = [IDS.merge];
  assertRed(invoke(arrayIdentity), /selector-invalid-identity/);

  const arbitraryReason = makeNeeds({
    mode: "full",
    complete: "false",
    reason: "trust-me",
  });
  assertRed(invoke(arbitraryReason), /selector-invalid-reason/);

  const forbiddenBackstop = makeNeeds({
    event: "schedule",
    mode: "asset-only",
    complete: "true",
    reason: "eligible",
    nativeResult: "skipped",
  });
  assertRed(
    invoke(forbiddenBackstop, "schedule"),
    /selector-forbidden-asset-mode/,
  );
});

test("each PR-only job is required on PRs and skipped only on backstops", () => {
  for (const id of PR_ONLY) {
    for (const status of ["failure", "cancelled", "skipped"]) {
      const pr = makeNeeds();
      pr[id].result = status;
      assertRed(invoke(pr), new RegExp(`pr-only-job:${id}$`, "m"));
    }

    for (const event of ["push", "schedule", "workflow_dispatch"]) {
      const backstop = makeNeeds({ event });
      backstop[id].result = "success";
      assertRed(invoke(backstop, event), new RegExp(`pr-only-job:${id}$`, "m"));
    }
  }
});

test("missing, malformed, and incomplete needs data fails nonzero", () => {
  const unknown = makeNeeds();
  unknown.untrusted = { result: "failure" };
  assertRed(invoke(unknown), /unknown-job:untrusted/);

  const unknownSuccess = makeNeeds();
  unknownSuccess.untrusted = { result: "success" };
  assertRed(invoke(unknownSuccess), /unknown-job:untrusted/);

  const valid = makeNeeds();
  Reflect.deleteProperty(valid, "go-suite");
  assertRed(invoke(valid), /missing-job:go-suite/);

  const missingResult = makeNeeds();
  Reflect.deleteProperty(missingResult.format, "result");
  assertRed(invoke(missingResult), /invalid-job:format/);

  const malformed = makeNeeds();
  const directory = fs.mkdtempSync(
    path.join(os.tmpdir(), "agentics-ci-summary-malformed-"),
  );
  const summaryFile = path.join(directory, "summary.md");
  const result = spawnSync(
    process.execPath,
    [
      SCRIPT,
      "--needs-env",
      "AGX_CI_NEEDS_JSON",
      "--event-name-env",
      "GITHUB_EVENT_NAME",
      "--summary-env",
      "GITHUB_STEP_SUMMARY",
    ],
    {
      cwd: ROOT,
      encoding: "utf8",
      env: {
        ...process.env,
        AGX_CI_NEEDS_JSON: "{",
        GITHUB_EVENT_NAME: "pull_request",
        GITHUB_STEP_SUMMARY: summaryFile,
      },
    },
  );
  assertRed(result, /invalid-json/);
  fs.rmSync(directory, { recursive: true, force: true });
  assert.equal(Object.keys(malformed).length, ALL.length);
});

test("invalid CLI inputs and summary writes fail nonzero", () => {
  const directory = fs.mkdtempSync(
    path.join(os.tmpdir(), "agentics-ci-summary-cli-"),
  );
  const result = spawnSync(process.execPath, [SCRIPT], {
    cwd: ROOT,
    encoding: "utf8",
    env: process.env,
  });
  assertRed(result, /cli-input/);
  const writeFailure = invoke(makeNeeds(), "pull_request", {
    summaryFile: directory,
  });
  assertRed(writeFailure, /summary-write/);
  fs.rmSync(directory, { recursive: true, force: true });
});

test("the summary leads with the verdict and marks the job that differs", () => {
  const passed = invoke(makeNeeds());
  assert.equal(passed.status, 0, passed.stderr);
  assert.match(passed.summary, /^## ✅ Validation passed$/m);
  assert.doesNotMatch(passed.summary, /Problems:/);
  assert.match(passed.summary, /^\| ✅ \| PR \/ Version fields \|/m);
  assert.match(passed.summary, /^\| ➖ \| Manual \|/m);

  const failing = makeNeeds();
  failing.format = job("failure");
  const failed = invoke(failing);
  assertRed(failed, /required-job:format/);
  assert.match(failed.summary, /^## ❌ Validation failed$/m);
  assert.match(failed.summary, /^Problems: `required-job:format`$/m);
  assert.match(
    failed.summary,
    /^\| ❌ \| Check \/ Format \| success \| failure \|$/m,
  );
});

test("a pull request summary shows the release level the PR metadata job computed", () => {
  for (const level of ["none", "patch", "minor", "major"]) {
    const needs = makeNeeds();
    needs["pr-metadata"] = job("success", { level });
    const result = invoke(needs);
    assert.equal(result.status, 0, result.stderr);
    assert.ok(
      result.summary.split("\n").includes(`Release level: **${level}**`),
      result.summary,
    );
  }
});

test("the release level is left out off pull requests and when it is not a known level", () => {
  const push = makeNeeds({ event: "push" });
  push["pr-metadata"] = job("skipped", { level: "minor" });
  const pushed = invoke(push, "push");
  assert.equal(pushed.status, 0, pushed.stderr);
  assert.doesNotMatch(pushed.summary, /Release level/);
  for (const level of ["", "Minor", "minor\n## injected", 1]) {
    const needs = makeNeeds();
    needs["pr-metadata"] = job("success", { level });
    const result = invoke(needs);
    assert.equal(result.status, 0, result.stderr);
    assert.doesNotMatch(result.summary, /Release level|injected/);
  }
});

test("every summary label names the job's display name in validate.yml", () => {
  const workflow = fs.readFileSync(
    path.join(ROOT, ".github/workflows/validate.yml"),
    "utf8",
  );
  const jobs = (
    YAML.parse(workflow) as {
      jobs: Record<
        string,
        {
          name: string;
          strategy?: { matrix?: { include?: { label: string }[] } };
        }
      >;
    }
  ).jobs;
  for (const id of ALL) {
    const job = jobs[id];
    assert.ok(job, `${id} job is missing`);
    // A job with one leg per label lists every leg's name.
    const legs = job.strategy?.matrix?.include?.map((leg) => leg.label);
    const name = job.name.includes("${{ matrix.label }}")
      ? job.name.replace("${{ matrix.label }}", legs!.join(", "))
      : job.name;
    assert.equal(SUMMARY_JOBS[id][0], name, id);
  }
});

test("validate-antigravity-native uses the expected label and follows validate-claude-native in the summary", () => {
  assert.equal(
    SUMMARY_JOBS["validate-antigravity-native"][0],
    "Check / Antigravity plugin",
  );
  const passed = invoke(makeNeeds());
  assert.equal(passed.status, 0, passed.stderr);
  assert.match(
    passed.summary,
    row("validate-antigravity-native", "success", "success"),
  );
  const lines = passed.summary.split("\n");
  const claudeIndex = lines.findIndex((line) =>
    line.includes("| Check / Claude plugin (strict) |"),
  );
  const antigravityIndex = lines.findIndex((line) =>
    line.includes("| Check / Antigravity plugin |"),
  );
  assert.ok(claudeIndex >= 0, "Claude plugin row must appear in summary");
  assert.equal(
    antigravityIndex,
    claudeIndex + 1,
    "Antigravity plugin row must immediately follow Claude plugin row",
  );
});
