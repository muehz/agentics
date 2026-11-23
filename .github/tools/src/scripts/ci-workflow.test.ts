import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import test from "node:test";
import os from "node:os";
import { spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import YAML from "yaml";
import { GO_CHECKS, JOB_ORDER, TS_SHARDS, SHARDS } from "./ci-areas.ts";
import { main as hooksTestMain } from "./ci-hooks-test.ts";
import { tsTest as tsTestMain } from "./ci-ts-test.ts";
import { LOCK_PATHS, LOCK_WORKFLOWS } from "./ci-gh-aw.ts";
import { SHELLCHECK_PATHSPECS } from "./ci-shellcheck.ts";
import { type Host, HOST_TABLE, makeArgs } from "./ci-native-smoke.ts";
import { MODULE_SHARDS, main as coverageMain } from "./ci-coverage.ts";
import {
  findInterpreter,
  requireInterpreter,
} from "../entrypoint/interpreters.ts";
import {
  ACTION_JOB,
  duplicateStepNames,
  jobSteps,
  workflowSources,
} from "./testing/workflow-steps.ts";

interface WorkflowWith {
  [key: string]: unknown;
  name: string;
  path: string;
  pattern: string;
}
interface WorkflowStep {
  name?: string;
  id?: string;
  uses?: string;
  run: string;
  if?: string;
  with: WorkflowWith;
  env: Record<string, string>;
  [key: string]: unknown;
}
interface WorkflowJob {
  steps: WorkflowStep[];
  if: string;
  env: Record<string, string>;
  defaults: { run: { shell: string } };
  strategy: {
    matrix: { workspace: string; [key: string]: unknown };
    [key: string]: unknown;
  };
  permissions?: "write-all" | Record<string, string>;
  needs: string[];
  [key: string]: unknown;
}
interface WorkflowDocument {
  jobs: Record<string, WorkflowJob>;
  permissions?: Record<string, string>;
}
interface CoverageArtifact {
  name: string;
  file: string;
  contents: string;
}
interface EngineManifestFixture {
  engines: { npm: string };
}
// These repository-owned YAML fixtures are checked by the assertions below; the
// type names the expected fields without treating arbitrary parsed YAML as validated.
function parseWorkflow(source: string): WorkflowDocument {
  return YAML.parse(source) as WorkflowDocument;
}
const ROOT = path.resolve(import.meta.dirname, "../../../..");
const WORKFLOW = path.join(ROOT, ".github/workflows/validate.yml");
const CHECKOUT = "actions/checkout@v7.0.1";
const SETUP_NODE = "actions/setup-node@v7.1.0";
const SETUP_GO = "actions/setup-go@v7.0.0";
const FORMAT_GO_MESSAGE = "format must not set up Go";
const FORMAT_TESTS_MESSAGE = "format must not run node --test";
const SCOPED =
  "needs.scope.result == 'success' && needs.scope.outputs.complete == 'true'";
const SELECTOR_CONDITION =
  "${{ !cancelled() && !(" +
  SCOPED +
  " && needs.scope.outputs.mode == 'asset-only') }}";
// The selector applied only to pull requests before pushes were scoped.
const PR_ONLY_SELECTOR_CONDITION =
  "${{ !cancelled() && !(github.event_name == 'pull_request' && needs.scope.result == 'success' && needs.scope.outputs.complete == 'true' && needs.scope.outputs.mode == 'asset-only') }}";
const OLD_SELECTOR_CONDITION =
  "${{ always() && !(github.event_name == 'pull_request' && needs.scope.result == 'success' && needs.scope.outputs.complete == 'true' && needs.scope.outputs.mode == 'asset-only') }}";
// The macOS and Windows jobs cost several times an Ubuntu minute, so only a manual dispatch runs them. A dispatch is
// never scoped, so no scope output narrows them.
const POST_MERGE_CONDITION =
  "${{ !cancelled() && github.event_name == 'workflow_dispatch' }}";
const PR_RUNNING_POST_MERGE_CONDITION =
  "${{ !cancelled() && github.event_name != 'pull_request' }}";
const SHARD_FALLBACK = '["hooks"]';
const GO_CHECKS_FALLBACK = '["tests", "race", "fuzz"]';
const CLASSIFY_FIELDS = [
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
] as const;
const OPTIONAL_BLANK_FIELDS = new Set(["base", "head", "merge", "areas"]);
const SCHEDULE_STEP = "Publish validated test selection";
const SCHEDULE_RUN = "node .github/tools/src/scripts/ci-scope.ts --schedule";
const PROOF_FORMAT = CLASSIFY_FIELDS.map((_, index) => `{${index}}`).join("|");
const PROOF_VALUES = CLASSIFY_FIELDS.map(
  (name) =>
    `needs.scope.outputs.${name}${OPTIONAL_BLANK_FIELDS.has(name) ? " || ''" : ""}`,
).join(", ");
const EFFECTIVE_SCOPE = `needs.scope.result == 'success' && needs.scope.outputs.effective_selection == format('${PROOF_FORMAT}', ${PROOF_VALUES})`;
const GO_SUITE_MATRIX_LINE = `        check: \${{ fromJSON(${EFFECTIVE_SCOPE} && needs.scope.outputs.race == 'false' && '["tests"]' || '${GO_CHECKS_FALLBACK}') }}`;
// The Go suite runs on every event, pull requests included, whenever a change reaches the hooks module.
const GO_SUITE_CONDITION = `\${{ !cancelled() && !(${EFFECTIVE_SCOPE} && needs.scope.outputs.shards == '[]') }}`;
// A go-suite condition that skips pull requests; the workflow must reject it.
const PR_SKIPPING_GO_SUITE_CONDITION = GO_SUITE_CONDITION.replace(
  "!cancelled() && ",
  "!cancelled() && github.event_name != 'pull_request' && ",
);
// Only the Ubuntu leg races and fuzzes: macOS and Windows legs on manual dispatch run the tests.
const GO_SUITE_EXCLUDES = [
  "          - { os: xcode-27, check: race }\n",
  "          - { os: xcode-27, check: fuzz }\n",
  "          - { os: windows-latest, check: race }\n",
  "          - { os: windows-latest, check: fuzz }\n",
];
const GO_RACE_COMMAND = "go test -C hooks/go -count=1 -race ./...";
const GO_FUZZ_COMMANDS = [
  "go test -C hooks/go -count=1 ./internal/shellwords -run '^$' -fuzz '^FuzzParse$' -fuzztime=30s",
  "go test -C hooks/go -count=1 ./internal/provider -run '^$' -fuzz '^FuzzNormalize$' -fuzztime=30s",
];
const TS_SHARD_FALLBACK = '["ts-tools"]';
const TS_SUITE_CONDITION = `\${{ !cancelled() && !(${EFFECTIVE_SCOPE} && needs.scope.outputs.ts_shards == '[]') }}`;
const TS_WORKSPACE_MATRIX = `\${{ fromJSON(${EFFECTIVE_SCOPE} && needs.scope.outputs.ts_shards || '${TS_SHARD_FALLBACK}') }}`;
const GO_COVERAGE_SHARDS = `\${{ ${EFFECTIVE_SCOPE} && needs.scope.outputs.shards || '${SHARD_FALLBACK}' }}`;
const TS_WORKSPACE = "${{ matrix.workspace == 'ts-tools' && 'tools' || '' }}";
// The coverage call job skips pull requests; inside it, a push runs TypeScript coverage only on a path match.
const COVERAGE_CALL_CONDITION =
  "${{ !cancelled() && github.event_name != 'pull_request' }}";
const TS_COVERAGE_CONDITION =
  "${{ github.event_name != 'push' || inputs.ts-coverage != 'false' }}";
const TS_COVERAGE_REPORT_CONDITION =
  "${{ !cancelled() && needs.ts-coverage.result == 'success' }}";
const TS_COVERAGE_SHARDS = ["tools-1"];
const COVERAGE_REPORT_CONDITION_AREAS =
  "${{ inputs.go-suite-result != 'skipped' && (inputs.shards == '' || inputs.shards == '" +
  SHARD_FALLBACK +
  "') }}";
const NATIVE_FAMILIES = ["go-suite"] as const;
const NATIVE_CONDITIONS = {
  "go-suite": GO_SUITE_CONDITION,
};
const POST_MERGE_FAMILIES = [
  "validate-windows-dispatch",
  "native-hook-smoke",
] as const;
const FULL_OS = ["ubuntu-latest", "xcode-27", "windows-latest"];
function osList(list: readonly string[]): string {
  return `'[${list.map((os) => `"${os}"`).join(", ")}]'`;
}
const GO_SUITE_OS_LINE = `        os: \${{ fromJSON(github.event_name == 'workflow_dispatch' && ${osList(FULL_OS)} || ${osList(["ubuntu-latest"])}) }}`;
const CHEAP_FAMILIES = [
  "validate-claude-native",
  "validate-antigravity-native",
  "conformance",
  "format",
  "shellcheck",
  "gh-aw-lock-drift",
];
// One conformance leg per rule area; the hooks leg leaves the module tests to go-suite.
// The hooks leg cross-builds every target, so it needs the larger runner.
const CONFORMANCE_LEGS = [
  { domain: "skills", label: "Skills", runner: "ubuntu-slim" },
  { domain: "manifests", label: "Manifests", runner: "ubuntu-slim" },
  { domain: "docs", label: "Docs", runner: "ubuntu-slim" },
  { domain: "repo", label: "Repo", runner: "ubuntu-slim" },
  { domain: "hooks", label: "Hooks", runner: "ubuntu-latest" },
];
const CONFORMANCE_LEGS_MESSAGE =
  "conformance must run one named leg per rule area";
const SKIP_MODULE_TESTS_MESSAGE =
  "the hooks leg must leave the module tests to go-suite, and no other leg may pass --fast";
const MCP_EXAMPLES_MESSAGE =
  "the skills leg must run the forge MCP safe-write examples";
const WINDOWS_HOOK_STEPS = [
  "Build the hook binaries from source",
  "cmd dispatches the built binary and denies",
  "benign payload passes silently through the binary",
  "missing binary fails closed for the guard",
  "cmd fetches a missing binary through fetch-hook.ps1",
];
const SUMMARY_JOB = "validation-summary";
const SUMMARY_CONDITION = "    if: ${{ !cancelled() }}";
const EXPRESSION_CACHE_MODE =
  "cache-mode: ${{ inputs.cache == 'cold' && 'none' || github.ref == 'refs/heads/main' && (github.event_name == 'schedule' || github.event_name == 'workflow_dispatch') && 'write' || 'read' }}";
const CACHE_INPUT = "cache: ${{ inputs.cache != 'cold' }}";
// validate.yml calls these reusable workflows, one per group of jobs that most
// events skip, so each group shows as one entry on a pull request.
const CALLED_WORKFLOWS = {
  coverage: "validate-coverage.yml",
  manual: "validate-manual.yml",
  mutation: "validate-mutation.yml",
} as const;

function readFile(name: string): string {
  return fs
    .readFileSync(path.join(ROOT, ".github/workflows", name), "utf8")
    .replaceAll("\r\n", "\n");
}

// validate.yml with the jobs of the workflows it calls appended to its jobs
// block. The job guards below read one job namespace, wherever a job is
// defined; the call jobs and the called files' own headers are checked apart.
function readWorkflow() {
  // The line-oriented guards mutate complete expressions. Decode the folded
  // scheduling scalars before those mutations, with their semantics intact.
  let source = readFile("validate.yml").replace(
    /^([ ]+)(if|check|workspace|shards): >-\n(?:\1 {2}\S.*\n)+/gm,
    (block, indent: string, key: string) => {
      const value = (YAML.parse(block) as Record<string, string>)[key];
      return `${indent}${key}: ${value}\n`;
    },
  );
  for (const name of Object.values(CALLED_WORKFLOWS)) {
    const called = readFile(name);
    source += "\n" + called.slice(called.indexOf("\njobs:\n") + 7);
  }
  return source;
}

function jobsText(source: string): string {
  const start = source.indexOf("jobs:\n");
  assert.notEqual(start, -1, "jobs block is missing");
  return source.slice(start + "jobs:\n".length);
}

function jobBlock(source: string, id: string): string {
  const jobs = jobsText(source);
  const header = `  ${id}:\n`;
  const start = jobs.indexOf(header);
  assert.notEqual(start, -1, `${id} job is missing`);
  const body = jobs.slice(start + header.length);
  const nextJob = body.search(/^  [A-Za-z0-9_-]+:\s*$/m);
  return body.slice(0, nextJob < 0 ? body.length : nextJob);
}

function replaceInJob(
  source: string,
  id: string,
  old: string,
  replacement: string,
): string {
  const header = `  ${id}:\n`;
  const start = source.indexOf(header);
  assert.notEqual(start, -1, `${id} job is missing`);
  const job = jobBlock(source, id);
  assert.equal(
    job.split(old).length - 1,
    1,
    `${id} fixture target must occur exactly once`,
  );
  const bodyStart = start + header.length;
  return (
    source.slice(0, bodyStart) +
    job.replace(old, replacement) +
    source.slice(bodyStart + job.length)
  );
}

// The named step's lines, up to the next step, step comment or job comment; null when the job has no such step.
function stepBlock(job: string, name: string): string | null {
  const header = `      - name: ${name}\n`;
  const start = job.indexOf(header);
  if (start < 0) return null;
  const body = job.slice(start + header.length);
  const next = body.search(/^(?:      [-#]|  #)/m);
  return body.slice(0, next < 0 ? body.length : next);
}

function assertTriggersAndConcurrency(source: string): void {
  assert.match(source, /^on:\n/m, "workflow triggers are missing");
  const pullRequest = source.match(
    /^  pull_request:\n((?:^    .*\n|^\s*$\n)*)/m,
  );
  assert.ok(pullRequest, "pull_request trigger is missing");
  assert.match(
    pullRequest[1]!,
    /^    branches: \[main\]$/m,
    "pull_request must target main",
  );
  assert.doesNotMatch(
    pullRequest[1]!,
    /^    paths(?:-ignore)?:/m,
    "pull_request must not use path filters",
  );
  assert.match(
    source,
    /^  push:\n    branches: \[main\]$/m,
    "push trigger must validate main after a merge",
  );
  assert.match(
    source,
    /^  schedule:\n    - cron: "17 6 \* \* 1"$/m,
    "weekly schedule must use the approved UTC entrypoint",
  );
  assert.match(
    source,
    /^  workflow_dispatch:\n/m,
    "manual entrypoint is missing",
  );
  const dispatch = source.match(
    /^  workflow_dispatch:\n((?:^    .*\n|^\s*$\n)*)/m,
  );
  assert.ok(dispatch, "workflow_dispatch block is missing");
  assert.match(
    dispatch[1]!,
    /^    inputs:\n/m,
    "manual cache input is missing",
  );
  assert.match(
    dispatch[1]!,
    /^      cache:\n/m,
    "manual cache input is missing",
  );
  assert.match(dispatch[1]!, /^        type: choice$/m);
  assert.match(dispatch[1]!, /^        options: \[normal, cold\]$/m);
  assert.match(dispatch[1]!, /^        default: normal$/m);
  assert.ok(
    source.includes(
      "  group: ${{ github.workflow }}-${{ github.event_name == 'pull_request' && format('pr-{0}', github.event.pull_request.number) || github.event_name == 'push' && format('push-{0}', github.ref) || format('run-{0}-{1}', github.event_name, github.run_id) }}",
    ),
    "concurrency group must isolate each PR, each pushed ref, and every schedule or manual run",
  );
  assert.ok(
    source.includes(
      "  cancel-in-progress: ${{ github.event_name == 'pull_request' || github.event_name == 'push' }}",
    ),
    "only a newer run of the same PR or pushed ref may cancel a predecessor",
  );
  assert.doesNotMatch(
    source,
    /^\s*cache-mode:.*\$\{\{/m,
    "cache-mode must be a literal; GitHub rejects the workflow file for an expression",
  );
}

function assertCacheContract(source: string): void {
  const setupJobs = {
    "go-suite": ["hooks/go/go.mod"],
    "validate-windows-dispatch": ["hooks/go/go.mod"],
    "native-hook-smoke": ["hooks/go/go.mod"],
  };
  for (const [id, dependencies] of Object.entries(setupJobs)) {
    const job = jobBlock(source, id);
    assert.ok(
      job.includes(`          ${CACHE_INPUT}\n`),
      `${id} must disable setup-go on cold runs`,
    );
    for (const dependency of dependencies) {
      assert.ok(
        job.includes(`            ${dependency}\n`) ||
          job.includes(`          cache-dependency-path: ${dependency}\n`),
        `${id} cache dependency ${dependency} is missing`,
      );
    }
  }
  const conformance = jobBlock(source, "conformance");
  assert.match(
    conformance,
    /^          cache: \$\{\{ inputs\.cache != 'cold' \}\}$/m,
    "validation conformance must pass its cold cache switch",
  );
  assert.doesNotMatch(
    source,
    /cache-hit/,
    "selected tests must never skip on a cache-hit output",
  );
  for (const line of source.split("\n")) {
    if (line.includes("go test -C ")) {
      assert.match(
        line,
        /-count=1(?:\s|$)/,
        `selected Go test must be uncached: ${line.trim()}`,
      );
    }
  }
}

// A push is scoped from the last validated commit. The scope job must look it up first and pass it to the classifier,
// and the lookup needs actions: read.
function assertLastGreenLookup(
  source: string,
  id: string,
  classifyStep: string,
): void {
  const job = jobBlock(source, id);
  assert.ok(
    job.includes(
      "    permissions:\n      contents: read\n      actions: read\n",
    ),
    `${id} must hold actions: read for the lookup and nothing wider`,
  );
  const lookup = job.indexOf(
    "        id: last\n        if: github.event_name == 'push'\n        env:\n          GH_TOKEN: ${{ github.token }}\n        run: node .github/tools/src/scripts/ci-last-green.ts >> \"$GITHUB_OUTPUT\"\n",
  );
  const classify = job.indexOf(
    `${classifyStep}\n        env:\n          AGX_CI_LAST_GREEN: \${{ steps.last.outputs.sha }}\n        run: node .github/tools/src/scripts/ci-scope.ts `,
  );
  assert.ok(
    lookup >= 0 && classify > lookup,
    `${id} must look up the last validated commit and pass it to the classifier`,
  );
}

const COVERAGE_PATHS_RUN =
  "        run: node .github/tools/src/scripts/ci-coverage-paths.ts\n";

// The step hands the push to the module through its env; the module owns the path match.
function assertCoveragePathsStep(source: string): void {
  const message =
    "the Match the TypeScript coverage paths step must run only on a push, pass BEFORE, and run the module";
  const job = jobBlock(source, "scope");
  const step = job.indexOf(
    "      - name: Match the TypeScript coverage paths\n",
  );
  const expected =
    "      - name: Match the TypeScript coverage paths\n" +
    "        id: ts-coverage\n" +
    "        if: github.event_name == 'push'\n" +
    "        env:\n" +
    "          BEFORE: ${{ github.event.before }}\n" +
    COVERAGE_PATHS_RUN;
  assert.ok(step >= 0 && job.slice(step).startsWith(expected), message);
}

function assertScopeJob(source: string): void {
  assertLastGreenLookup(source, "scope", "        id: classify");
  const job = jobBlock(source, "scope");
  assert.match(job, /^    runs-on: ubuntu-slim$/m);
  assert.doesNotMatch(job, /^    (?:needs|if):/m, "scope must be independent");
  assert.match(job, /^    outputs:\n/m, "scope outputs are missing");
  for (const output of [
    "mode",
    "complete",
    "base",
    "head",
    "merge",
    "reason",
  ]) {
    assert.ok(
      job.includes(
        "      " + output + ": ${{ steps.classify.outputs." + output + " }}",
      ),
      `scope output ${output} must bind the classifier step`,
    );
  }
  const checkout = job.indexOf(`uses: ${CHECKOUT}`);
  const setupNode = job.indexOf(`uses: ${SETUP_NODE}`);
  const classify = job.indexOf("      - name: Classify immutable Git diff");
  assert.ok(checkout >= 0, "scope checkout pin is missing");
  assert.ok(setupNode > checkout, "scope must set up Node after checkout");
  assert.ok(classify > setupNode, "scope classifier must follow setup-node");
  assert.match(job, /^          fetch-depth: 0$/m);
  assert.match(job, /^          persist-credentials: false$/m);
  assert.match(job, /^          node-version: 26\.12\.1$/m);
  assert.match(job, /^        id: classify$/m);
  assert.match(
    job,
    /^        run: node \.github\/tools\/src\/scripts\/ci-scope\.ts --repo "\$\{GITHUB_WORKSPACE\}" --event "\$\{GITHUB_EVENT_PATH\}"$/m,
    "scope must invoke the frozen classifier command",
  );
  const steps = (YAML.parse("scope:\n" + job) as { scope: WorkflowJob }).scope
    .steps;
  const publish = steps.find((step) => step.name === SCHEDULE_STEP);
  assert.ok(publish, "scope must publish validated test selection");
  assert.ok(
    steps.indexOf(publish) > steps.findIndex((step) => step.id === "classify"),
  );
  assert.equal(publish.id, "selection");
  assert.equal(
    publish.if,
    undefined,
    "publication must require earlier steps to succeed",
  );
  assert.equal(publish.run, SCHEDULE_RUN);
  assert.deepEqual(publish.env, {
    AGX_CI_CLASSIFY_OUTPUTS: "${{ toJSON(steps.classify.outputs) }}",
    AGX_CI_CLASSIFY_OUTCOME: "${{ steps.classify.outcome }}",
  });
  assert.ok(
    job.includes(
      "      effective_selection: ${{ steps.selection.outputs.effective_selection }}\n",
    ),
    "scope must publish one proof of the complete validated selection",
  );
  assertCoveragePathsStep(source);
}

// A failing test must not report as a formatting failure, so the format job checks formatting and nothing else.
function assertFormatJob(source: string): void {
  // jobBlock runs to the next job header, so it carries the comment above the next job; only the steps count here.
  const format = jobBlock(source, "format")
    .split("\n")
    .filter((line) => !/^\s*#/.test(line))
    .join("\n");
  assert.ok(!format.includes("actions/setup-go@"), FORMAT_GO_MESSAGE);
  assert.ok(!format.includes("node --test"), FORMAT_TESTS_MESSAGE);
}

// The conformance job runs one leg per rule area. The hooks leg alone passes --fast, because go-suite runs the hooks
// module tests and their coverage floor; the skills leg runs the forge MCP examples even when ts-suite is skipped.
function assertConformanceLegs(source: string): void {
  const job = jobBlock(source, "conformance");
  // Only this job is parsed, so a mutation elsewhere reaches the guard that owns it.
  const parsed = YAML.parse(job) as WorkflowJob;
  const include = (parsed.strategy.matrix as unknown as { include?: unknown[] })
    .include;
  assert.ok(
    JSON.stringify(include) === JSON.stringify(CONFORMANCE_LEGS) &&
      parsed.name === "Check / ${{ matrix.label }}" &&
      parsed["runs-on"] === "${{ matrix.runner }}" &&
      job.includes("          domain: ${{ matrix.domain }}\n"),
    CONFORMANCE_LEGS_MESSAGE,
  );
  assert.ok(
    job.split("skip-module-tests:").length === 2 &&
      job.includes(
        "          skip-module-tests: ${{ matrix.domain == 'hooks' }}\n",
      ),
    SKIP_MODULE_TESTS_MESSAGE,
  );
  const examples = stepBlock(job, "Test the forge MCP safe-write examples");
  assert.ok(
    examples !== null &&
      examples.includes("        if: matrix.domain == 'skills'\n") &&
      examples.includes(
        "        run: node --test .github/tools/src/skill-examples/mcp-safe-write.test.ts\n",
      ),
    MCP_EXAMPLES_MESSAGE,
  );
}

const GO_CHECKS_MESSAGE =
  "go-suite must run the tests on every OS and race and fuzz on Ubuntu only";
const GO_RACE_MESSAGE =
  "go-suite must race every hook package with CGO enabled on the race leg";
const GO_FUZZ_MESSAGE = "go-suite must run each fuzz target on the fuzz leg";
const GO_CRASHER_MESSAGE =
  "go-suite must upload fuzz crashers when a fuzz leg fails";
const GO_FLOOR_MESSAGE =
  "go-suite must enforce the 90% coverage floor on the Ubuntu test leg";
// The module owns the go arguments, the profile path and the floor value; the workflow only names the subcommand.
const HOOKS_TEST_COMMAND =
  "node .github/tools/src/scripts/ci-hooks-test.ts test";
const HOOKS_FLOOR_COMMAND =
  "node .github/tools/src/scripts/ci-hooks-test.ts floor";

// One job holds every Go test: the module tests with their coverage floor, the race run, and the fuzz targets.
function assertGoChecks(source: string): void {
  // assertCoverageReport checks that the check list comes from the scope job.
  const job = jobBlock(source, "go-suite");
  assert.ok(
    GO_SUITE_EXCLUDES.every((line) => job.includes(line)),
    GO_CHECKS_MESSAGE,
  );
  const race = stepBlock(job, "Race hook packages");
  assert.ok(
    race !== null &&
      race.includes("        if: matrix.check == 'race'\n") &&
      race.includes('          CGO_ENABLED: "1"\n') &&
      race.includes(`        run: ${GO_RACE_COMMAND}\n`),
    GO_RACE_MESSAGE,
  );
  for (const [name, command] of [
    ["Fuzz shell parsing", GO_FUZZ_COMMANDS[0]!],
    ["Fuzz provider normalization", GO_FUZZ_COMMANDS[1]!],
  ] as const) {
    const step = stepBlock(job, name);
    assert.ok(
      step !== null &&
        step.includes("        if: matrix.check == 'fuzz'\n") &&
        step.includes(`        run: ${command}\n`),
      GO_FUZZ_MESSAGE,
    );
  }
  const crashers = stepBlock(job, "Upload fuzz crashers");
  assert.ok(
    crashers !== null &&
      crashers.includes("        if: failure() && matrix.check == 'fuzz'\n") &&
      crashers.includes("          path: hooks/go/**/testdata/fuzz/**\n") &&
      crashers.includes("          if-no-files-found: ignore\n"),
    GO_CRASHER_MESSAGE,
  );
  const floor = stepBlock(job, "Enforce the 90% coverage floor");
  assert.ok(
    floor !== null &&
      floor.includes(
        "        if: matrix.check == 'tests' && matrix.os == 'ubuntu-latest'\n",
      ) &&
      floor.includes(`        run: ${HOOKS_FLOOR_COMMAND}\n`),
    GO_FLOOR_MESSAGE,
  );
}

const TS_CHECKS_MESSAGE =
  "ts-suite must typecheck every workspace and check the area boundaries on Linux";

function assertTypeScriptChecks(source: string): void {
  const job = jobBlock(source, "ts-suite");
  const typecheck = stepBlock(job, "Typecheck every npm workspace");
  const boundaries = stepBlock(job, "Check the area boundaries");
  assert.ok(
    typecheck !== null &&
      boundaries !== null &&
      typecheck.includes("        if: runner.os == 'Linux'\n") &&
      typecheck.includes("        run: npm run test:typecheck\n") &&
      boundaries.includes("        if: runner.os == 'Linux'\n") &&
      boundaries.includes("        run: npm run test:boundaries\n"),
    TS_CHECKS_MESSAGE,
  );
}

function assertNativeFamilies(source: string): void {
  for (const id of NATIVE_FAMILIES) {
    const job = jobBlock(source, id);
    assert.match(job, /^    needs: \[scope\]$/m, `${id} must depend on scope`);
    assert.match(
      job,
      new RegExp(
        `^    if: ${NATIVE_CONDITIONS[id].replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}$`,
        "m",
      ),
      `${id} must use the common selector fallback condition`,
    );
  }
  const manual = jobBlock(source, "manual");
  assert.match(
    manual,
    /^    needs: \[scope\]$/m,
    "manual must depend on scope",
  );
  assert.ok(
    manual.includes(`\n    if: ${POST_MERGE_CONDITION}\n`) &&
      manual.includes(
        `\n    uses: ./.github/workflows/${CALLED_WORKFLOWS.manual}\n`,
      ),
    "manual must run only on manual dispatch, never on a pull request, a push or the schedule",
  );
  // The call job's condition is the only gate; a condition inside would hide a skipped job from validation-summary.
  for (const id of POST_MERGE_FAMILIES) {
    assert.doesNotMatch(
      jobBlock(source, id),
      /^    (?:needs|if):/m,
      `${id} must run only on manual dispatch, never on a pull request, a push or the schedule`,
    );
  }
}

function assertCheapFamilies(source: string): void {
  for (const id of CHEAP_FAMILIES) {
    const job = jobBlock(source, id);
    assert.doesNotMatch(
      job,
      /^    (?:needs|if):/m,
      `${id} must remain unconditionally independent of selector eligibility`,
    );
  }
}

function assertSummaryJob(source: string): void {
  const job = jobBlock(source, SUMMARY_JOB);
  assert.match(
    job,
    /^    runs-on: ubuntu-slim$/m,
    "summary runner must be ubuntu-slim",
  );
  assert.match(
    job,
    /^    if: \$\{\{ !cancelled\(\) \}\}$/m,
    "summary must use cancellation-aware reporting semantics",
  );
  const needsStart = job.indexOf("    needs:");
  const conditionStart = job.indexOf("    if:", needsStart);
  assert.ok(
    needsStart >= 0 && conditionStart > needsStart,
    "summary needs are missing",
  );
  const actualNeeds = job
    .slice(needsStart, conditionStart)
    .replace(/^    needs:\s*/, "")
    .match(/[a-z][a-z0-9-]*/g);
  assert.deepEqual(
    actualNeeds,
    JOB_ORDER,
    "validation-summary must depend on every selected job in source order",
  );
  const checkout = job.indexOf(`uses: ${CHECKOUT}`);
  const setupNode = job.indexOf(`uses: ${SETUP_NODE}`);
  const report = job.indexOf("      - name: Report expected and actual jobs");
  assert.ok(checkout >= 0, "validation-summary checkout pin is missing");
  assert.ok(
    setupNode > checkout,
    "validation-summary must set up Node after checkout",
  );
  assert.ok(
    report > setupNode,
    "validation-summary report must follow setup-node",
  );
  assert.match(
    job,
    /^          persist-credentials: false$/m,
    "summary checkout must disable persisted credentials",
  );
  assert.match(
    job,
    /^          node-version: 26\.12\.1$/m,
    "summary must use Node 26.12.1",
  );
  assert.doesNotMatch(
    job,
    /^          (?:GITHUB_EVENT_NAME|GITHUB_STEP_SUMMARY):/m,
    "summary must use GitHub-provided reserved environment variables",
  );
  assert.match(
    job,
    /^          AGX_CI_NEEDS_JSON: \$\{\{ toJSON\(needs\) \}\}$/m,
    "summary must bind toJSON(needs) through its custom environment variable",
  );
  assert.match(
    job,
    /^        run: node \.github\/tools\/src\/scripts\/ci-summary\.ts --needs-env AGX_CI_NEEDS_JSON --event-name-env GITHUB_EVENT_NAME --summary-env GITHUB_STEP_SUMMARY$/m,
    "validation-summary must pass needs through the fixed environment interface",
  );
  assert.doesNotMatch(
    job,
    /run:.*\$\{\{\s*toJSON\(needs\)/,
    "needs JSON must not be interpolated into shell code",
  );
}

const SMOKE_BUILD_RUN =
  "        run: node .github/tools/src/scripts/ci-native-smoke.ts build\n";
const SMOKE_BINARIES = [
  ["Linux/X64", "agx-linux-amd64"],
  ["Linux/ARM64", "agx-linux-arm64"],
  ["macOS/X64", "agx-darwin-amd64"],
  ["macOS/ARM64", "agx-darwin-arm64"],
  ["Windows/X64", "agx-windows-amd64.exe"],
  ["Windows/ARM64", "agx-windows-arm64.exe"],
] as const;

// The step hands the runner to the module through the environment GitHub sets; the module owns the host table, the
// single-binary make, the manifest hash check and the tracked-file diff, which its own tests pin.
function assertHostTargetBuild(
  source: string,
  id: string,
  table: Readonly<Record<string, Host>> = HOST_TABLE,
  makeArguments: (name: string) => string[] = makeArgs,
): void {
  const job = jobBlock(source, id);
  assert.ok(job.length > 0, `${id} host build step is missing`);
  assert.doesNotMatch(
    job,
    /make -B -C hooks\/go build-all/,
    `${id} must not rebuild every hook binary`,
  );
  assert.ok(
    !makeArguments("agx-x").includes("build-all") &&
      makeArguments("agx-x").at(-1) === "../bin/agx-x",
    `${id} must not rebuild every hook binary`,
  );
  const step = stepBlock(job, "Build the hook binaries from source");
  assert.ok(
    step !== null &&
      step.includes("        shell: bash\n") &&
      step.includes(SMOKE_BUILD_RUN),
    `${id} host build proof is missing: the module build step`,
  );
  for (const [key, name] of SMOKE_BINARIES) {
    const host = table[key];
    assert.ok(
      host !== undefined && `agx-${host.os}-${host.arch}${host.ext}` === name,
      `${id} host build proof is missing: ${key}`,
    );
  }
}

const TRACKED_NATIVE_STEP = "Reject tracked hook binaries and Windows launcher";
const TRACKED_NATIVE_RUN =
  'tracked=$(git ls-files -- hooks/bin bin/agx.exe) && test -z "$tracked"';

function assertSourceOnlyGuard(source: string): void {
  const job = jobBlock(source, "conformance");
  const step = stepBlock(job, TRACKED_NATIVE_STEP);
  assert.ok(
    step,
    "the always-running hooks conformance job needs a tracked-native guard",
  );
  assert.ok(
    step.includes("        if: matrix.domain == 'hooks'\n") &&
      step.includes("        shell: bash\n"),
    "the tracked-native guard must run on the hooks conformance leg",
  );
  assert.ok(
    step.includes(`        run: '${TRACKED_NATIVE_RUN}'\n`),
    "the tracked-native guard must fail on tracked paths or Git errors",
  );
  assert.ok(
    job.indexOf(TRACKED_NATIVE_STEP) <
      job.indexOf("uses: $/.github/actions/conformance"),
    "the tracked-native guard must run before conformance generates binaries",
  );
}

function assertHookBuild(source: string): void {
  const job = jobBlock(source, "go-suite");
  const buildStart = job.indexOf(
    "      - name: Build hook binaries and check reproducibility\n        if: matrix.check == 'tests'\n",
  );
  const buildEnd = job.indexOf(
    "      - name: Test the hooks module\n",
    buildStart,
  );
  const build = job.slice(buildStart, buildEnd);
  assert.ok(
    buildStart >= 0 && buildEnd > buildStart,
    "hooks build step is missing",
  );
  assert.ok(
    job.includes(`${GO_SUITE_OS_LINE}\n`),
    "hooks must retain every supported OS on manual dispatch and only Ubuntu otherwise",
  );
  assert.match(
    build,
    /make -B -C hooks\/go build-all/,
    "hooks must retain the complete rebuild",
  );
  assert.match(
    build,
    /^ {10}git diff --exit-code -- hooks\/ bin\/$/m,
    "hooks must retain the complete hooks diff",
  );
}

function assertNativeSmoke(source: string): void {
  const job = jobBlock(source, "native-hook-smoke");
  assert.match(
    job,
    /^        os: \[macos-15-intel, windows-11-arm\]$/m,
    "native smoke must retain both native host identities",
  );
  for (const [name, condition, shell, command] of [
    [
      "Confirm the runner is the target architecture (macOS)",
      "runner.os == 'macOS'",
      "bash",
      "arch macos",
    ],
    [
      "Confirm the runner is the target architecture (Windows)",
      "runner.os == 'Windows'",
      "pwsh",
      "arch windows",
    ],
  ] as const) {
    const step = stepBlock(job, name);
    assert.ok(
      step !== null &&
        step.includes(`        if: ${condition}\n`) &&
        step.includes(`        shell: ${shell}\n`) &&
        step.includes(
          `        run: node .github/tools/src/scripts/ci-native-smoke.ts ${command}\n`,
        ),
      `native smoke proof is missing: ${name}`,
    );
  }
  for (const line of [
    "Run the native binary directly (macOS)",
    "AGX_SOURCE_DIGEST",
    "hooks/bin/agx-darwin-amd64",
    "Run the native binary directly (Windows)",
    "hooks\\bin\\agx-windows-arm64.exe",
    "command-guardian dispatch smoke (macOS)",
    "hooks/bin/agx-darwin-arm64",
    "command-guardian dispatch smoke (Windows)",
    "hooks\\bin\\agx-windows-amd64.exe",
    "AGX_NO_FETCH",
  ]) {
    assert.ok(job.includes(line), `native smoke proof is missing: ${line}`);
  }
  const windowsDispatch = stepBlock(
    job,
    "command-guardian dispatch smoke (Windows)",
  );
  assert.ok(
    windowsDispatch?.includes(
      'if (Test-Path -LiteralPath "hooks\\bin\\agx-windows-amd64.exe") {',
    ),
    "the Windows ARM64 smoke must allow the other architecture's binary to be absent",
  );
}

function assertWindowsDispatchBuild(source: string): void {
  const job = jobBlock(source, "validate-windows-dispatch");
  for (const name of WINDOWS_HOOK_STEPS) {
    assert.ok(
      stepBlock(job, name) !== null,
      `Windows dispatcher proof is missing the step: ${name}`,
    );
  }
  for (const line of [
    "runs-on: windows-latest",
    "shell: bash",
    "rm -f hooks/bin/agx-windows-amd64.exe",
    "make -B -C hooks/go build-all",
    "git diff --exit-code -- hooks/ bin/",
  ]) {
    assert.ok(
      job.includes(line),
      `Windows dispatcher proof is missing: ${line}`,
    );
  }
  assertGhAwDrift(source);
}

const GH_AW_INSTALL_RUN =
  "        run: node .github/tools/src/scripts/ci-gh-aw.ts install\n";
const GH_AW_CHECK_RUN =
  "        run: node .github/tools/src/scripts/ci-gh-aw.ts check\n";

// The two steps hand the pinned release to the module through their env. The module owns the download, the checksum,
// the compile order and the porcelain check; its exported lists name the workflows and paths it covers.
function assertGhAwDrift(
  source: string,
  workflows: readonly string[] = LOCK_WORKFLOWS,
  paths: readonly string[] = LOCK_PATHS,
): void {
  const job = jobBlock(source, "gh-aw-lock-drift");
  const install = stepBlock(job, "Install pinned gh-aw");
  assert.ok(
    install !== null &&
      install.includes(GH_AW_INSTALL_RUN) &&
      install.includes("          GH_AW_VERSION: v1.0.1\n") &&
      /^ {10}GH_AW_SHA256: [0-9a-f]{64}$/m.test(install),
    "gh-aw must be installed from the pinned release and checksum by the module",
  );
  assert.ok(
    workflows.includes("release"),
    "release lock must be rederived in validation",
  );
  assert.ok(
    workflows.includes("backlog-grooming"),
    "backlog grooming lock must be rederived in validation",
  );
  const check = stepBlock(job, "Recompile and diff the locks");
  assert.ok(
    check !== null &&
      check.includes(GH_AW_CHECK_RUN) &&
      check.includes("          GH_AW_VERSION: v1.0.1\n") &&
      [".github/workflows", ".github/aw", ".gitattributes"].every((p) =>
        paths.includes(p),
      ),
    "gh-aw lock rederivation must catch stale and uncommitted locks",
  );
}

const COVERAGE_SHARDS = ["hooks"];
const FORK_GUARD =
  "github.event_name != 'pull_request' || github.event.pull_request.head.repo.full_name == github.repository";

// The Go coverage report is wired across three jobs and an artifact name. A
// broken link leaves every check green and silently uploads nothing, so each
// link is pinned.
function assertCoverageReport(source: string): void {
  const suite = jobBlock(source, "go-suite");
  assert.ok(
    suite.includes(
      "          COVERAGE: ${{ matrix.os == 'ubuntu-latest' && '1' || '' }}\n",
    ),
    "go-suite must instrument only its Ubuntu legs",
  );
  // The module writes the profile under RUNNER_TEMP/coverage when COVERAGE is set; its own tests pin the arguments.
  assert.ok(
    stepBlock(suite, "Test the hooks module")?.includes(
      `        run: ${HOOKS_TEST_COMMAND}\n`,
    ),
    "go-suite must write one coverage profile per shard",
  );
  assert.equal(
    suite.split(`        run: ${HOOKS_TEST_COMMAND}\n`).length - 1,
    COVERAGE_SHARDS.length,
    "every shard must record a coverage profile",
  );
  const profileUpload = stepBlock(suite, "Upload the coverage profile");
  for (const line of [
    "        if: matrix.check == 'tests' && matrix.os == 'ubuntu-latest'\n",
    "          name: go-coverage-profile-hooks\n",
    "          path: ${{ runner.temp }}/coverage/hooks.out\n",
    "          if-no-files-found: error\n",
  ]) {
    assert.ok(
      profileUpload?.includes(line),
      `go-suite must upload each shard profile: ${line.trim()}`,
    );
  }
  assert.ok(
    suite.includes(GO_SUITE_MATRIX_LINE + "\n"),
    "go-suite must read its checks from the scope job",
  );
  assert.deepEqual(
    JSON.parse(SHARD_FALLBACK),
    COVERAGE_SHARDS,
    "coverage must cover every go-suite shard",
  );

  const call = jobBlock(source, "coverage");
  for (const line of [
    "    needs: [scope, go-suite]\n",
    "    if: ${{ !cancelled() && github.event_name != 'pull_request' }}\n",
    "      go-suite-result: ${{ needs.go-suite.result }}\n",
    `      shards: ${GO_COVERAGE_SHARDS}\n`,
  ])
    assert.ok(call.includes(line), "coverage-report must depend on go-suite");
  const report = jobBlock(source, "coverage-report");
  assert.ok(
    report.includes(`    if: ${COVERAGE_REPORT_CONDITION_AREAS}\n`),
    "coverage-report must still run when an advisory leg fails",
  );
  assert.doesNotMatch(
    report,
    /^ {4}permissions:/m,
    "coverage-report runs repository code and must keep the read-only token",
  );
  const downloads = parseWorkflow(source).jobs["coverage-report"]!.steps.filter(
    (step) => step.uses?.startsWith("actions/download-artifact@"),
  );
  assert.deepEqual(
    downloads.map((step) => step.with),
    COVERAGE_SHARDS.map((shard) => ({
      name: `go-coverage-profile-${shard}`,
      path: `\${{ runner.temp }}/profiles/go-coverage-profile-${shard}`,
    })),
    "coverage-report must download every shard profile into its named directory",
  );
  for (const [name, command] of COVERAGE_STEP_COMMANDS) {
    const step = stepBlock(report, name);
    assert.ok(
      step !== null &&
        step.includes(`        run: ${COVERAGE_MODULE} ${command}`),
      `coverage-report's ${name} step must run the module's ${command}`,
    );
  }
  const required = report.match(
    /^ {8}run: node \.github\/tools\/src\/scripts\/ci-coverage\.ts require-profiles (.+)$/m,
  );
  assert.ok(required, "coverage-report must check every shard profile");
  assert.deepEqual(
    required[1]!.split(" "),
    COVERAGE_SHARDS,
    "the profile check must list the go-suite shards",
  );
  assert.deepEqual(
    MODULE_SHARDS,
    COVERAGE_SHARDS,
    "the profile merge must join the go-suite shards",
  );
  assert.doesNotMatch(
    report,
    /merge-cobertura|cobertura-hooks\.xml/,
    "coverage-report must write the final XML directly",
  );

  assert.ok(
    report.includes("          name: coverage-report-go\n"),
    "coverage-report must publish the coverage-report-go artifact",
  );
  for (const file of [
    "cobertura.xml",
    "coverage-hooks.out",
    "coverage-hooks.txt",
  ]) {
    assert.ok(
      report.includes(`            ${file}\n`),
      `coverage-report must upload ${file} in its artifact`,
    );
  }
  assert.ok(
    report.includes(`        if: ${FORK_GUARD}\n`),
    "coverage-report must not publish fork artifacts",
  );

  const upload = jobBlock(source, "coverage-upload");
  assert.match(
    upload,
    /^ {4}needs: \[coverage-report\]$/m,
    "coverage-upload must depend on coverage-report",
  );
  assert.ok(
    upload.includes("needs.coverage-report.result == 'success'") &&
      upload.includes(FORK_GUARD),
    "coverage-upload must wait for the report and skip fork pull requests",
  );
  assert.ok(
    upload.includes("      code-quality: write\n"),
    "coverage-upload must hold code-quality: write",
  );
  assert.ok(
    upload.includes("          name: coverage-report-go\n") &&
      upload.includes("          file: cobertura.xml\n"),
    "coverage-upload must upload the report coverage-report publishes",
  );
  assert.doesNotMatch(
    upload,
    /actions\/checkout/,
    "the job that can write code-quality must not check out repository code",
  );
  assertCoverageWriters(source);
  assertGoProfileConversion(source);
}

function assertGoProfileConversion(source: string): void {
  const report = parseWorkflow(source).jobs["coverage-report"]!;
  const nodeIndex = report.steps.findIndex((step) => step.uses === SETUP_NODE);
  const convertIndex = report.steps.findIndex(
    (step) => step.name === "Convert profiles to Cobertura XML",
  );
  assert.ok(
    nodeIndex >= 0 && nodeIndex < convertIndex,
    "Go profile conversion needs pinned Node before conversion",
  );
  assert.equal(report.steps[nodeIndex]!.with["node-version"], "26.12.1");
  assert.doesNotMatch(
    source,
    /gocover-cobertura/,
    "Go coverage must convert with .github/tools/src/coverage, not gocover-cobertura",
  );
  assert.equal(
    convertIndex >= 0,
    true,
    "Go profile conversion step is missing",
  );
  assert.equal(
    report.steps[convertIndex]!.run.trim(),
    "node .github/tools/src/coverage/cli.ts goprofile --source-root . --module hooks/go --profile coverage-hooks.out --out cobertura.xml",
    "Go profile conversion must preserve hooks/go source and profile mapping and write cobertura.xml directly",
  );
}

function assertCoverageWriters(source: string): void {
  {
    const workflow = parseWorkflow(source);
    // The coverage call job runs no steps: it only caps the called jobs' tokens.
    const expected = ["coverage", "coverage-upload", "ts-coverage-upload"];
    assert.ok(
      jobBlock(source, "coverage").includes(
        `\n    uses: ./.github/workflows/${CALLED_WORKFLOWS.coverage}\n`,
      ) && !jobBlock(source, "coverage").includes("\n    steps:"),
      "only the two named coverage upload jobs may write code-quality",
    );
    assert.deepEqual(
      workflow.permissions,
      { contents: "read" },
      "the workflow token must remain read-only by default",
    );
    const writers = Object.entries(workflow.jobs)
      .filter(
        ([, job]) =>
          job.permissions === "write-all" ||
          job.permissions?.["code-quality"] === "write",
      )
      .map(([id]) => id);
    assert.deepEqual(
      writers,
      expected,
      "only the two named coverage upload jobs may write code-quality",
    );
  }
}

function assertTypeScriptSuite(source: string): void {
  const jobs = parseWorkflow(source).jobs;
  const job = jobs["ts-suite"]!;
  assert.ok(job, "ts-suite job is missing");
  assert.deepEqual(job.needs, ["scope"], "ts-suite must depend on scope");
  assert.equal(
    job.if,
    TS_SUITE_CONDITION,
    "ts-suite must guard empty TS selections and preserve selector fallback",
  );
  assert.equal(
    job["timeout-minutes"],
    30,
    "ts-suite gives every leg 30 minutes",
  );
  assert.deepEqual(
    job.permissions,
    { contents: "read" },
    "ts-suite must remain read-only",
  );
  assert.equal(job["runs-on"], "${{ matrix.os }}");
  assert.equal(job.strategy["fail-fast"], false);
  assert.equal(
    job.strategy.matrix.os,
    jobs["go-suite"]!.strategy.matrix.os,
    "ts-suite must cover the go-suite operating systems",
  );
  assert.equal(
    job.strategy.matrix.workspace,
    TS_WORKSPACE_MATRIX,
    "ts-suite must read TS selections with full fallback",
  );
  assert.equal(
    job.env.WORKSPACE!,
    TS_WORKSPACE,
    "ts-suite must map TS shards to npm workspaces",
  );
  assert.equal(job.defaults.run.shell, "bash");
  const checkout = job.steps.findIndex((step) => step.uses === CHECKOUT);
  const node = job.steps.findIndex((step) => step.uses === SETUP_NODE);
  assert.ok(checkout >= 0 && node > checkout);
  assert.equal(job.steps[checkout]!.with["persist-credentials"], false);
  assert.equal(job.steps[checkout]!.with["fetch-depth"], 0);
  assert.equal(job.steps[node]!.with["node-version"], "26.12.1");
  assertFixtureToolchain(job);
  assert.equal(
    job.env.GOTOOLCHAIN,
    undefined,
    "ts-suite must not pin a Go toolchain",
  );
  const pin = job.steps.findIndex(
    (step) => step.run === "npm install --global --ignore-scripts npm@12.2.1",
  );
  const install = job.steps.findIndex(
    (step) => step.run === "npm ci --ignore-scripts",
  );
  const release = job.steps.findIndex(
    (step) => step.run === "npm ci --ignore-scripts --prefix .github/release",
  );
  const testStep = job.steps.findIndex(
    (step) => step.name === "Test the TypeScript workspace",
  );
  assert.ok(
    pin > node && install > pin && release > install && testStep > release,
    "ts-suite must install pinned Node/npm and both dependency sets before tests",
  );
  assert.equal(
    job.strategy["max-parallel"],
    3,
    "the suite matrix must cap concurrent legs",
  );
  assert.deepEqual(job.strategy.matrix.shard, [1, 2, 3, 4]);
  assert.deepEqual(job.strategy.matrix.exclude, [
    { os: "ubuntu-latest", shard: 2 },
    { os: "ubuntu-latest", shard: 3 },
    { os: "ubuntu-latest", shard: 4 },
    { os: "xcode-27", shard: 2 },
    { os: "xcode-27", shard: 3 },
    { os: "xcode-27", shard: 4 },
    { workspace: "ts-tools", shard: 4 },
  ]);
  assert.deepEqual(job.steps[testStep]!.env, {
    RUNNER_LABEL: "${{ matrix.os }}",
    TEST_SHARD: "${{ matrix.shard }}",
    TEST_CONCURRENCY: "${{ inputs.windows-test-concurrency || '2' }}",
  });
  assert.doesNotMatch(job.steps[testStep]!.run, /test:coverage/);
  assert.equal(
    job.steps.some((step) => step.uses?.startsWith("actions/upload-artifact@")),
    false,
    "required tests must not upload coverage",
  );
  assert.ok(
    jobs[SUMMARY_JOB]!.needs.includes("ts-suite"),
    "validation summary must depend on ts-suite",
  );
}

function actionsExpression<T = unknown>(
  expression: string,
  parameters: string[],
  values: unknown[],
): T {
  const text = expression.startsWith("${{")
    ? expression.slice(3, -2)
    : expression;
  return new Function(...parameters, `return (${text});`)(...values);
}

function scheduleFor(
  jobs: Record<string, WorkflowJob>,
  scope: { result: string; outputs: Record<string, unknown> },
) {
  const needs = { scope };
  const format = (template: string, ...values: unknown[]) =>
    template.replace(/\{(\d+)\}/g, (_, index: string) =>
      String(values[Number(index)] ?? ""),
    );
  const go = jobs["go-suite"]!;
  const ts = jobs["ts-suite"]!;
  const coverage = jobs.coverage!.with as { shards: string };
  const enabled = (job: WorkflowJob) =>
    actionsExpression<boolean>(
      job.if,
      ["needs", "cancelled", "format"],
      [needs, () => false, format],
    );
  const matrix = (expression: string) =>
    actionsExpression<string[]>(
      expression,
      ["needs", "fromJSON", "format"],
      [needs, JSON.parse, format],
    );
  return {
    go: enabled(go),
    checks: matrix(go.strategy.matrix.check as string),
    ts: enabled(ts),
    workspaces: matrix(ts.strategy.matrix.workspace),
    coverageShards: actionsExpression<string>(
      coverage.shards,
      ["needs", "format"],
      [needs, format],
    ),
  };
}

function publishSelection(
  jobs: Record<string, WorkflowJob>,
  raw: Record<string, unknown>,
  event: string,
  output: string,
) {
  const publish = jobs.scope!.steps.find(
    (step) => step.name === SCHEDULE_STEP,
  )!;
  assert.equal(publish.run, SCHEDULE_RUN);
  const [, script, flag] = SCHEDULE_RUN.split(" ");
  const steps = { classify: { outcome: "success", outputs: raw } };
  const env = Object.fromEntries(
    Object.entries(publish.env).map(([key, expression]) => [
      key,
      actionsExpression<string>(
        expression,
        ["steps", "toJSON"],
        [steps, JSON.stringify],
      ),
    ]),
  );
  const result = spawnSync(
    process.execPath,
    [path.resolve(ROOT, script!), flag!],
    {
      cwd: ROOT,
      encoding: "utf8",
      env: {
        ...process.env,
        ...env,
        GITHUB_EVENT_NAME: event,
        GITHUB_OUTPUT: output,
      },
    },
  );
  assert.ifError(result.error);
  return result;
}

const FULL_SCHEDULE = {
  go: true,
  checks: GO_CHECKS,
  ts: true,
  workspaces: TS_SHARDS,
  coverageShards: SHARD_FALLBACK,
};

const SCOPE_IDS = {
  base: "a".repeat(40),
  head: "b".repeat(40),
  merge: "c".repeat(40),
};

function expectedProof(raw: Record<string, string>): string {
  return CLASSIFY_FIELDS.map((field) => raw[field] ?? "").join("|");
}

test("stale, incomplete, or disagreeing selection cannot narrow the deployed jobs", () => {
  const jobs = parseWorkflow(readFile("validate.yml")).jobs;
  const reduced = {
    ...SCOPE_IDS,
    mode: "asset-only",
    complete: "true",
    reason: "eligible",
    shards: "[]",
    ts_shards: "[]",
    race: "false",
  };
  const proof = expectedProof(reduced);
  const published = { ...reduced, effective_selection: proof };
  const cases = [
    { result: "failure", outputs: published },
    { result: "success", outputs: reduced },
    {
      result: "success",
      outputs: { ...reduced, effective_selection: proof.slice(0, -3) },
    },
    {
      result: "success",
      outputs: { ...published, ts_shards: "not-json" },
    },
    {
      result: "success",
      outputs: { ...published, race: undefined },
    },
  ];
  for (const scope of cases) {
    assert.deepEqual(
      scheduleFor(jobs, scope),
      FULL_SCHEDULE,
      `${scope.result}: ${JSON.stringify(scope.outputs)}`,
    );
  }
});

test("an output write failure on the publication step keeps the full workflow schedule", (t) => {
  const jobs = parseWorkflow(readFile("validate.yml")).jobs;
  const directory = fs.mkdtempSync(
    path.join(os.tmpdir(), "agentics-ci-schedule-write-"),
  );
  t.after(() => fs.rmSync(directory, { recursive: true, force: true }));
  const raw = {
    ...SCOPE_IDS,
    mode: "asset-only",
    complete: "true",
    reason: "eligible",
    shards: "[]",
    ts_shards: "[]",
    race: "false",
  };
  const publication = publishSelection(jobs, raw, "pull_request", directory);
  assert.equal(publication.status, 1);
  assert.equal(publication.stderr, "ci-scope: schedule output-write\n");
  assert.deepEqual(
    scheduleFor(jobs, { result: "failure", outputs: raw }),
    FULL_SCHEDULE,
  );
});

test("losing raw evidence after valid publication forces the full deployed schedule", (t) => {
  const jobs = parseWorkflow(readFile("validate.yml")).jobs;
  const directory = fs.mkdtempSync(
    path.join(os.tmpdir(), "agentics-ci-schedule-transport-"),
  );
  t.after(() => fs.rmSync(directory, { recursive: true, force: true }));
  const asset = {
    ...SCOPE_IDS,
    mode: "asset-only",
    complete: "true",
    reason: "eligible",
    shards: "[]",
    ts_shards: "[]",
    race: "false",
  };
  const tools = {
    ...asset,
    mode: "area-scoped",
    reason: "area-scoped",
    areas: "tools",
    ts_shards: '["ts-tools"]',
  };
  for (const raw of [asset, tools]) {
    const output = path.join(directory, raw.mode);
    const result = publishSelection(jobs, raw, "pull_request", output);
    assert.equal(result.status, 0, result.stderr);
    const bytes = fs.readFileSync(output, "utf8");
    const proof = expectedProof(raw);
    assert.equal(bytes, `effective_selection=${proof}\n`);
    const outputs: Record<string, string> = {
      ...raw,
      effective_selection: proof,
    };
    assert.equal(scheduleFor(jobs, { result: "success", outputs }).go, false);
    assert.equal(
      scheduleFor(jobs, { result: "success", outputs }).ts,
      raw.mode !== "asset-only",
    );
    for (const field of Object.keys(raw)) {
      const omitted = { ...outputs };
      delete omitted[field];
      assert.deepEqual(
        scheduleFor(jobs, { result: "success", outputs: omitted }),
        FULL_SCHEDULE,
        `${raw.mode}: omitted ${field}`,
      );
    }
    const differentHead = { ...outputs, head: "d".repeat(40) };
    assert.deepEqual(
      scheduleFor(jobs, { result: "success", outputs: differentHead }),
      FULL_SCHEDULE,
    );
  }
});

test("the deployed publication step supplies the actual job conditions and matrices", (t) => {
  const jobs = parseWorkflow(readFile("validate.yml")).jobs;
  const directory = fs.mkdtempSync(
    path.join(os.tmpdir(), "agentics-ci-schedule-"),
  );
  t.after(() => fs.rmSync(directory, { recursive: true, force: true }));
  for (const [event, mode, areas, goShards, tsShards, race] of [
    ["pull_request", "asset-only", "", [], [], false],
    ["pull_request", "area-scoped", "tools", [], ["ts-tools"], false],
    ["pull_request", "area-scoped", "skills", ["hooks"], [], false],
    ["push", "area-scoped", "skills,tools", ["hooks"], ["ts-tools"], false],
    ["pull_request", "area-scoped", "hooks", ["hooks"], ["ts-tools"], true],
    ["workflow_dispatch", "full", "", ["hooks"], ["ts-tools"], true],
  ] as const) {
    const output = path.join(directory, event + "-" + mode + "-" + areas);
    const raw = {
      ...(event === "pull_request"
        ? SCOPE_IDS
        : event === "push"
          ? { base: SCOPE_IDS.base, head: SCOPE_IDS.head }
          : {}),
      mode,
      complete: String(mode !== "full"),
      reason:
        mode === "full"
          ? "event-full"
          : mode === "asset-only"
            ? "eligible"
            : "area-scoped",
      ...(areas ? { areas } : {}),
      shards: JSON.stringify(goShards),
      ts_shards: JSON.stringify(tsShards),
      race: String(race),
    };
    const result = publishSelection(jobs, raw, event, output);
    assert.equal(result.status, 0, result.stderr);
    assert.equal(result.stdout, "");
    assert.equal(result.stderr, "");
    const bytes = fs.readFileSync(output, "utf8");
    const effective_selection = expectedProof(raw);
    assert.equal(bytes, `effective_selection=${effective_selection}\n`);
    assert.deepEqual(
      scheduleFor(jobs, {
        result: "success",
        outputs: { ...raw, effective_selection },
      }),
      {
        go: goShards.length > 0,
        checks: race ? GO_CHECKS : ["tests"],
        ts: tsShards.length > 0,
        workspaces: tsShards,
        coverageShards: raw.shards,
      },
    );
  }
  const job = jobs["ts-suite"]!;
  assert.equal(
    actionsExpression(
      job.env.WORKSPACE!,
      ["matrix"],
      [{ workspace: "ts-tools" }],
    ),
    "tools",
  );
  assert.equal(
    actionsExpression(
      job.env.WORKSPACE!,
      ["matrix"],
      [{ workspace: "unknown" }],
    ),
    "",
  );
  assert.equal(TS_SHARD_FALLBACK, JSON.stringify(TS_SHARDS));
  assert.deepEqual(JSON.parse(GO_CHECKS_FALLBACK), GO_CHECKS);
});

const TS_TEST_COMMAND = "node .github/tools/src/scripts/ci-ts-test.ts";

// The step hands the leg to the module through its env; the module owns the command it starts.
function assertTsTestStep(source: string): void {
  const step = parseWorkflow(source).jobs["ts-suite"]!.steps.find(
    (step) => step.name === "Test the TypeScript workspace",
  );
  assert.ok(step, "the Test the TypeScript workspace step is missing");
  assert.equal(
    step.run?.trim(),
    TS_TEST_COMMAND,
    "the Test the TypeScript workspace step must run the module",
  );
  assert.deepEqual(
    step.env,
    {
      RUNNER_LABEL: "${{ matrix.os }}",
      TEST_SHARD: "${{ matrix.shard }}",
      TEST_CONCURRENCY: "${{ inputs.windows-test-concurrency || '2' }}",
    },
    "the Test the TypeScript workspace step must pass the leg through its env",
  );
}

test("TypeScript workspace tests shard only on Windows and reject unmapped workspaces", () => {
  const source = readWorkflow();
  assertTsTestStep(source);
  const job = parseWorkflow(source).jobs["ts-suite"]!;
  assert.match(job.env.WORKSPACE!, /matrix\.workspace == 'ts-tools'/);
  const calls: string[][] = [];
  for (const [workspace, runner, expected] of [
    ["tools", "ubuntu-latest", ["npm", "--prefix", ".github/tools", "test"]],
    ["tools", "xcode-27", ["npm", "--prefix", ".github/tools", "test"]],
    [
      "tools",
      "windows-latest",
      [
        "node",
        ".github/tools/src/scripts/ci-test-shard.ts",
        "windows-tools",
        "4",
        "2",
      ],
    ],
    ["", "windows-latest", []],
  ] as const) {
    calls.length = 0;
    const status = tsTestMain({
      env: {
        WORKSPACE: workspace,
        RUNNER_LABEL: runner,
        TEST_SHARD: "4",
        TEST_CONCURRENCY: "2",
      },
      run: (command, args) => {
        calls.push([command === process.execPath ? "node" : command, ...args]);
        return { status: 0 };
      },
      report: () => {},
    });
    assert.equal(
      status,
      expected.length === 0 ? 1 : 0,
      `${workspace}/${runner}`,
    );
    assert.deepEqual(calls.length === 0 ? [] : calls[0], expected);
  }
});

test("negative: the workspace test guard rejects the old script and a changed module path", () => {
  const source = readWorkflow();
  const swap = (from: string, to: string) => source.replace(from, () => to);
  for (const [name, mutated] of [
    [
      "the old inline script restored",
      swap(
        `        run: ${TS_TEST_COMMAND}\n`,
        '        run: |\n          npm --prefix ".github/$WORKSPACE" test\n',
      ),
    ],
    [
      "a changed module path",
      swap(TS_TEST_COMMAND, "node .github/tools/src/scripts/ci-ts-tests.ts"),
    ],
  ] as const) {
    assert.notEqual(mutated, source, `${name}: the mutation changed nothing`);
    assert.throws(
      () => assertTsTestStep(mutated),
      /Test the TypeScript workspace step must run the module/,
      name,
    );
  }
});

test("TypeScript guards reject missing fallback, empty-selection protection, and write permissions", () => {
  const source = readWorkflow();
  for (const [from, to, message] of [
    [
      "    if: " + TS_SUITE_CONDITION,
      "    if: " + SELECTOR_CONDITION,
      /guard empty TS selections/,
    ],
    [
      TS_WORKSPACE_MATRIX,
      "${{ fromJSON(needs.scope.outputs.ts_shards) }}",
      /read TS selections with full fallback/,
    ],
    ["      contents: read", "      contents: write", /must remain read-only/],
  ] as const)
    assert.throws(
      () => assertTypeScriptSuite(replaceInJob(source, "ts-suite", from, to)),
      message,
    );
});

function assertShardModules(source: string): void {
  const suite = parseWorkflow(source).jobs["go-suite"]!;
  const setup = suite.steps.find((step) =>
    step.uses?.startsWith("actions/setup-go@"),
  )!;
  assert.equal(
    setup!.with["go-version-file"],
    "hooks/go/go.mod",
    "hooks must select its own Go module",
  );
}

test("each Go shard sets up the toolchain hooks/go/go.mod names", () => {
  assertShardModules(readWorkflow());
});

test("shard commands select each package exactly once", () => {
  const suite = parseWorkflow(readWorkflow()).jobs["go-suite"]!;
  const step = suite.steps.find((s) => s.name === "Test the hooks module")!;
  assert.equal(step.run, HOOKS_TEST_COMMAND);
  const calls: string[][] = [];
  const status = hooksTestMain(["test"], {
    env: { COVERAGE: "" },
    run: (_command, args) => {
      calls.push(args);
      return { status: 0, stdout: "" };
    },
  });
  assert.equal(status, 0);
  assert.deepEqual(
    calls.map((args) => args.join(" ")),
    ["test -C hooks/go -count=1 ./..."],
    "hooks must select only its own packages",
  );
});

test("the shard module guard rejects a different toolchain file", () => {
  assert.throws(
    () =>
      assertShardModules(
        replaceInJob(
          readWorkflow(),
          "go-suite",
          "go-version-file: hooks/go/go.mod",
          "go-version-file: wrong/go.mod",
        ),
      ),
    /hooks must select its own Go module/,
  )!;
});

const CORPUS_EXCLUDE = ":!:hooks/go/cmd/hook/testdata/corpus/scripts/plugin.sh";
// The frozen verify-stage.sh sources a stage library that no longer exists,
// so shellcheck cannot see where its variables are read.
const CORPUS_STAGE_EXCLUDE =
  ":!:hooks/go/cmd/hook/testdata/corpus/scripts/verify-stage.sh";
const CORPUS_WILDCARD_EXCLUDE = "corpus/scripts/*";
const SHELLCHECK_INSTALL_RUN =
  "        run: node .github/tools/src/scripts/ci-shellcheck.ts install\n";
const SHELLCHECK_CHECK_RUN =
  "        run: node .github/tools/src/scripts/ci-shellcheck.ts check\n";

// The steps hand the pinned release to the module through their env. The module owns the download, the checksum, the
// extraction and the lint; its exported pathspecs name the scripts it lints.
function assertShellcheckCorpusExclusion(
  source: string,
  specs: readonly string[] = SHELLCHECK_PATHSPECS,
): void {
  const job = jobBlock(source, "shellcheck");
  const install = stepBlock(job, "Install pinned shellcheck");
  assert.ok(
    install !== null &&
      install.includes(SHELLCHECK_INSTALL_RUN) &&
      install.includes("          SHELLCHECK_VERSION: v0.11.0\n") &&
      /^ {10}SHELLCHECK_SHA256: [0-9a-f]{64}$/m.test(install),
    "shellcheck must be installed from the pinned release and checksum by the module",
  );
  const step = stepBlock(job, "Check shell scripts");
  assert.ok(step, "shellcheck job must have a Check shell scripts step");
  assert.ok(
    step.includes(SHELLCHECK_CHECK_RUN),
    "shellcheck's Check shell scripts step must run the module",
  );
  assert.ok(
    specs.includes(CORPUS_EXCLUDE),
    "shellcheck's glob pass must exclude the frozen corpus plugin.sh copy",
  );
  assert.ok(
    specs.includes(CORPUS_STAGE_EXCLUDE),
    "shellcheck's glob pass must exclude the frozen corpus verify-stage.sh copy",
  );
  assert.ok(
    !specs.some((spec) => spec.includes(CORPUS_WILDCARD_EXCLUDE)),
    "shellcheck's glob pass must not exclude the other frozen corpus scripts",
  );
}

function assertWorkflow(source: string): void {
  assertTriggersAndConcurrency(source);
  assertCacheContract(source);
  assertScopeJob(source);
  assertFormatJob(source);
  assertAntigravityNativeJob(source);
  assertNativeFamilies(source);
  assertCheapFamilies(source);
  assertConformanceLegs(source);
  assertSourceOnlyGuard(source);
  assertGoChecks(source);
  assertShellcheckCorpusExclusion(source);
  assertTypeScriptChecks(source);
  assertSummaryJob(source);
  assertHostTargetBuild(source, "native-hook-smoke");
  assertHookBuild(source);
  assertNativeSmoke(source);
  assertWindowsDispatchBuild(source);
  assertShardModules(source);
  assertCoverageReport(source);
  assertTypeScriptSuite(source);
  assertTypeScriptCoverageReport(source);
  assertTypeScriptCoverageUpload(source);
}

// The pinned download action extracts one artifact directly into its configured
// path, even when a pattern was used and merge-multiple is false.
function extractCoverageDownloads(
  job: WorkflowJob,
  artifacts: readonly CoverageArtifact[],
  root: string,
): void {
  for (const step of job.steps.filter((step) =>
    step.uses?.startsWith("actions/download-artifact@"),
  )) {
    const selected = artifacts.filter((artifact) =>
      step.with.name
        ? artifact.name === step.with.name
        : artifact.name.startsWith(step.with.pattern.slice(0, -1)),
    );
    const base = step.with.path.replace("${{ runner.temp }}", root);
    for (const artifact of selected) {
      const dir =
        step.with.name || step.with["merge-multiple"] || selected.length === 1
          ? base
          : path.join(base, artifact.name);
      fs.mkdirSync(dir, { recursive: true });
      fs.writeFileSync(path.join(dir, artifact.file), artifact.contents);
    }
  }
}

const COVERAGE_STEP_COMMANDS = [
  ["Require every shard profile", "require-profiles"],
  ["Merge shard profiles per module", "merge-profiles"],
  ["Report module coverage totals", "totals"],
] as const;
const COVERAGE_MODULE = "node .github/tools/src/scripts/ci-coverage.ts";

test("Go coverage download paths handle a single artifact without an extra directory", (t) => {
  const job = parseWorkflow(readWorkflow()).jobs["coverage-report"]!;
  const guard = job.steps.find(
    (step) => step.name === "Require every shard profile",
  );
  const prefix = `${COVERAGE_MODULE} require-profiles `;
  assert.ok(
    guard?.run?.startsWith(prefix),
    "the coverage-report profile check must run the module",
  );
  const root = fs.mkdtempSync(
    path.join(os.tmpdir(), "agentics-artifact-layout-"),
  );
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  extractCoverageDownloads(
    job,
    [
      {
        name: "go-coverage-profile-hooks",
        file: "hooks.out",
        contents: "mode: count\n",
      },
    ],
    root,
  );
  const errors: string[] = [];
  const status = coverageMain(
    ["require-profiles", ...guard!.run!.slice(prefix.length).split(" ")],
    {
      env: { RUNNER_TEMP: root },
      write: (line) => errors.push(line),
      report: (line) => errors.push(line),
    },
  );
  assert.equal(status, 0, errors.join("\n"));
});

// The tools script tests build Go fixtures, so every tools leg needs Go.
function assertFixtureToolchain(job: WorkflowJob): void {
  const setup = job.steps.filter((step) =>
    step.uses?.startsWith("actions/setup-go@"),
  )!;
  assert.equal(setup.length, 1, "tools needs the native fixture toolchain");
  assert.equal(setup[0]!.uses, SETUP_GO);
  assert.equal(
    setup[0]!.if,
    undefined,
    "fixture Go setup must run for every tools leg",
  );
  assert.deepEqual(setup[0]!.with, {
    "go-version-file": "hooks/go/go.mod",
    cache: "${{ inputs.cache != 'cold' }}",
    "cache-dependency-path": "hooks/go/go.mod",
  });
}

test("TypeScript fixture Go setup runs for the tools suite", () => {
  assertFixtureToolchain(parseWorkflow(readWorkflow()).jobs["ts-suite"]!);
});

test("TypeScript fixture toolchain guard rejects a missing or conditional Go setup", () => {
  const job = parseWorkflow(readWorkflow()).jobs["ts-suite"]!;
  const setup = job.steps.find((step) =>
    step.uses?.startsWith("actions/setup-go@"),
  )!;
  assert.ok(setup, "fixture toolchain is missing");
  setup.if = "matrix.os == 'ubuntu-latest'";
  assert.throws(
    () => assertFixtureToolchain(job),
    /must run for every tools leg/,
  );
  job.steps = job.steps.filter((step) => step !== setup);
  assert.throws(
    () => assertFixtureToolchain(job),
    /tools needs the native fixture toolchain/,
  );
});

function assertTypeScriptCoverageUpload(source: string): void {
  assertCoverageWriters(source);
  const jobs = parseWorkflow(source).jobs;
  for (const [id, language, artifact, file, label] of [
    [
      "coverage-upload",
      "Go",
      "coverage-report-go",
      "cobertura.xml",
      "code-coverage-agent",
    ],
    [
      "ts-coverage-upload",
      "TypeScript",
      "coverage-report-typescript",
      "cobertura-typescript.xml",
      "code-coverage-agent-typescript",
    ],
  ] as const) {
    const report = id.replace("upload", "report");
    assert.deepEqual(
      jobs[id]!,
      {
        name: `${language} upload`,
        "runs-on": "ubuntu-latest",
        needs: [report],
        if: `\${{ !cancelled() && needs.${report}.result == 'success' && (${FORK_GUARD}) }}`,
        permissions: { contents: "read", "code-quality": "write" },
        steps: [
          {
            name: "Download coverage artifact",
            uses: "actions/download-artifact@v8.0.2",
            with: { name: artifact },
          },
          {
            name: "Select supported coverage upload context",
            id: "context",
            shell: "bash",
            env: {
              DEFAULT_BRANCH: "${{ github.event.repository.default_branch }}",
            },
            run:
              'if [[ "$GITHUB_EVENT_NAME" == workflow_dispatch && "$GITHUB_REF" != "refs/heads/$DEFAULT_BRANCH" ]]; then\n' +
              '  echo "should_upload=false" >> "$GITHUB_OUTPUT"\n' +
              '  echo "::notice::Coverage artifact retained; manual non-default refs have no supported code-quality upload context."\n' +
              'else\n  echo "should_upload=true" >> "$GITHUB_OUTPUT"\nfi\n',
          },
          {
            uses: "actions/upload-code-coverage@v1.4.4",
            if: "steps.context.outputs.should_upload == 'true'",
            with: { file, language, label },
          },
        ],
      },
      `${id} must keep the pinned artifact-only upload and inline context guards`,
    );
  }
}

test("TypeScript coverage upload is artifact-only with exactly the allowed writers", () => {
  assertTypeScriptCoverageUpload(readWorkflow());
});

test("coverage writer allowlist rejects an added writer and an identity swap", () => {
  const source = readWorkflow();
  const added = replaceInJob(
    source,
    "ts-coverage-report",
    "      contents: read",
    "      contents: read\n      code-quality: write",
  );
  assert.throws(
    () => assertCoverageWriters(added),
    /two named coverage upload jobs/,
  );
  const swapped = replaceInJob(
    added,
    "ts-coverage-upload",
    "      code-quality: write\n",
    "",
  );
  assert.throws(
    () => assertCoverageWriters(swapped),
    /two named coverage upload jobs/,
  );
});

test("TypeScript upload rejects repository execution, changed pins, and weakened context guards", () => {
  for (const [from, to] of [
    ["    steps:\n", `    steps:\n      - uses: ${CHECKOUT}\n`],
    [
      "    steps:\n",
      "    steps:\n      - run: node .github/tools/src/coverage/cli.ts\n",
    ],
    ["      contents: read", "      contents: write"],
    ["    needs: [ts-coverage-report]", "    needs: [ts-suite]"],
    [
      "needs.ts-coverage-report.result == 'success'",
      "needs.ts-coverage-report.result != 'skipped'",
    ],
    [` && (${FORK_GUARD})`, ""],
    [
      '"$GITHUB_EVENT_NAME" == workflow_dispatch',
      '"$GITHUB_EVENT_NAME" == push',
    ],
    ['"refs/heads/$DEFAULT_BRANCH"', '"refs/heads/main"'],
    [
      "          DEFAULT_BRANCH: ${{ github.event.repository.default_branch }}",
      "          DEFAULT_BRANCH: main",
    ],
    ["        if: steps.context.outputs.should_upload == 'true'\n", ""],
    ["actions/upload-code-coverage@v1.4.4", "actions/upload-code-coverage@v1"],
    ["          language: TypeScript", "          language: Go"],
    [
      "          name: coverage-report-typescript",
      "          name: coverage-report-go",
    ],
    [
      "          file: cobertura-typescript.xml",
      "          file: cobertura.xml",
    ],
  ] as const) {
    assert.throws(
      () =>
        assertTypeScriptCoverageUpload(
          replaceInJob(readWorkflow(), "ts-coverage-upload", from, to),
        ),
      /artifact-only upload and inline context guards/,
    );
  }
});

function assertTypeScriptCoverageReport(source: string): void {
  const jobs = parseWorkflow(source).jobs;
  const suite = jobs["ts-coverage"]!;
  const report = jobs["ts-coverage-report"]!;
  assert.ok(suite && report, "TypeScript coverage jobs are missing");
  const call = jobs["coverage"]!;
  assert.equal(suite.needs, undefined);
  assert.ok(
    suite.if === TS_COVERAGE_CONDITION &&
      call.if === COVERAGE_CALL_CONDITION &&
      (call.with as unknown as Record<string, string>)["ts-coverage"] ===
        "${{ needs.scope.outputs.ts_coverage }}",
    "coverage must skip pull requests and pushes outside its paths",
  );
  assert.deepEqual(suite.permissions, { contents: "read" });
  assert.deepEqual(report.permissions, { contents: "read" });
  assert.deepEqual(report.needs, ["ts-coverage"]);
  assert.equal(
    report.if,
    TS_COVERAGE_REPORT_CONDITION,
    "coverage requires every shard to pass",
  );
  assert.equal(suite["runs-on"], "ubuntu-latest");
  assert.equal(suite.strategy["fail-fast"], false);
  assert.equal(suite.strategy["max-parallel"], 3);
  assert.deepEqual(
    suite.strategy.matrix.include,
    TS_COVERAGE_SHARDS.map((id) => {
      const [workspace, shard] = id.split("-");
      return { workspace, shard: Number(shard) };
    }),
  );
  assertFixtureToolchain(suite);
  assert.equal(
    suite.steps.find((step) => step.name === "Run the instrumented test shard")
      ?.run,
    'node .github/tools/src/scripts/ci-test-shard.ts "coverage-$WORKSPACE" "$TEST_SHARD" 1',
  );
  assert.deepEqual(suite.steps.at(-1)?.with, {
    name: "ts-coverage-lcov-${{ matrix.workspace }}-${{ matrix.shard }}",
    path: ".github/${{ matrix.workspace }}/coverage/lcov.info",
    "if-no-files-found": "error",
    "retention-days": 1,
  });
  assert.equal(report.steps[0]?.uses, CHECKOUT);
  assert.equal(report.steps[0]?.with["persist-credentials"], false);
  assert.equal(report.steps[1]?.uses, SETUP_NODE);
  assert.equal(report.steps[1]?.with["node-version"], "26.12.1");
  const download = report.steps.find(
    (step) => step.name === "Download shard LCOV reports",
  )!;
  assert.equal(download.uses, "actions/download-artifact@v8.0.2");
  assert.deepEqual(download.with, {
    name: "ts-coverage-lcov-tools-1",
    path: "${{ runner.temp }}/lcov/ts-coverage-lcov-tools-1",
  });
  const upload = report.steps.find(
    (step) => step.name === "Upload TypeScript coverage artifact",
  )!;
  assert.equal(upload.uses, "actions/upload-artifact@v7.0.2");
  assert.equal(upload.if, FORK_GUARD);
  assert.deepEqual(upload.with, {
    name: "coverage-report-typescript",
    path: "cobertura-typescript.xml",
    "if-no-files-found": "error",
  });
  const combine = report.steps.find(
    (step) => step.name === "Combine complete TypeScript coverage",
  );
  assert.equal(combine?.if, undefined);
  // The shard arguments must equal the ts-coverage matrix, so a leg that stops delivering cannot be skipped.
  assert.equal(
    combine?.run?.trim(),
    `${TS_COMBINE_PREFIX}${TS_COVERAGE_SHARDS.join(" ")}`,
    "the TypeScript report must combine every ts-coverage shard through the module",
  );
}

const TS_COMBINE_PREFIX =
  "node .github/tools/src/scripts/ci-coverage.ts combine-ts ";
const TS_COMBINE_COMMAND = `${TS_COMBINE_PREFIX}tools-1`;

function copyCoverageSourcePolicy(root: string): void {
  const source = ".github/tools/src/scripts/mutation-source.ts";
  fs.cpSync(path.join(ROOT, source), path.join(root, source));
}

test("TypeScript coverage is read-only, skips pull requests, and requires every shard", () => {
  assertTypeScriptCoverageReport(readWorkflow());
});

test("TypeScript report guards reject weak prerequisites and merged artifact paths", () => {
  const source = readWorkflow();
  for (const [from, to] of [
    ["      contents: read", "      contents: read\n      code-quality: write"],
    [
      "          path: ${{ runner.temp }}/lcov",
      "          path: ${{ runner.temp }}",
    ],
    [
      "needs.ts-coverage.result == 'success'",
      "needs.ts-coverage.result != 'skipped'",
    ],
  ] as const) {
    assert.throws(() =>
      assertTypeScriptCoverageReport(
        replaceInJob(source, "ts-coverage-report", from, to),
      ),
    );
  }
});

test("TypeScript report requires every nonempty shard", async (t) => {
  const report = parseWorkflow(readWorkflow()).jobs["ts-coverage-report"]!;
  const combine = report.steps.find(
    (step) => step.name === "Combine complete TypeScript coverage",
  )!;
  assert.ok(combine.run?.startsWith(TS_COMBINE_PREFIX));
  const shards = combine.run!.slice(TS_COMBINE_PREFIX.length).trim().split(" ");
  for (const missing of [
    null,
    ...TS_COVERAGE_SHARDS,
    "empty-tools-1",
  ] as const) {
    await t.test(missing || "all shards", (t) => {
      const root = fs.mkdtempSync(
        path.join(os.tmpdir(), "agentics-ts-report-"),
      );
      t.after(() => fs.rmSync(root, { recursive: true, force: true }));
      fs.cpSync(
        path.join(ROOT, ".github/tools/src/coverage"),
        path.join(root, ".github/tools/src/coverage"),
        {
          recursive: true,
        },
      );
      copyCoverageSourcePolicy(root);
      extractCoverageDownloads(
        report,
        TS_COVERAGE_SHARDS.filter((shard) => shard !== missing).map(
          (shard) => ({
            name: "ts-coverage-lcov-" + shard,
            file: "lcov.info",
            contents:
              missing === "empty-" + shard
                ? ""
                : "SF:src/sample.ts\nDA:1,1\nDA:2,1\nend_of_record\n",
          }),
        ),
        root,
      );
      const summary = path.join(root, "summary.md");
      const out: string[] = [];
      const status = coverageMain(["combine-ts", ...shards], {
        env: {
          RUNNER_TEMP: root,
          GITHUB_WORKSPACE: root,
          GITHUB_STEP_SUMMARY: summary,
        },
        cwd: root,
        write: (line) => out.push(line),
        report: (line) => out.push(line),
      });
      assert.equal(status, missing ? 1 : 0, out.join("\n"));
      if (missing) {
        assert.match(
          out.join("\n"),
          new RegExp(
            "Missing or empty LCOV report for " + missing.replace("empty-", ""),
          ),
        );
        assert.equal(
          fs.existsSync(path.join(root, "cobertura-typescript.xml")),
          false,
        );
        return;
      }
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
  }
});

test("negative: the TypeScript report guard rejects a wrong module call and a missing shard argument", () => {
  const source = readWorkflow();
  const cases = [
    [
      "a wrong module call",
      TS_COMBINE_COMMAND,
      "node ci-coverage.ts combine-ts tools-1",
    ],
    [
      "the old inline script restored",
      `        run: ${TS_COMBINE_COMMAND}\n`,
      "        run: |\n          for shard in tools-1; do\n          done\n",
    ],
    ["a missing shard argument", TS_COMBINE_COMMAND, TS_COMBINE_PREFIX.trim()],
    [
      "an extra shard argument",
      TS_COMBINE_COMMAND,
      `${TS_COMBINE_COMMAND} tools-2`,
    ],
  ] as const;
  for (const [name, from, to] of cases) {
    const mutated = replaceInJob(source, "ts-coverage-report", from, to);
    assert.notEqual(mutated, source, `${name}: the mutation changed nothing`);
    assert.throws(
      () => assertTypeScriptCoverageReport(mutated),
      /must combine every ts-coverage shard through the module/,
      name,
    );
  }
});

test("validate workflow wires selector and entrypoints", () => {
  assertWorkflow(readWorkflow());
});

test("the source-only guard rejects allowing tracked outputs or Git failure", () => {
  const source = readWorkflow();
  const step = stepBlock(jobBlock(source, "conformance"), TRACKED_NATIVE_STEP)!;
  for (const [from, to] of [
    [" && ", "; "],
    ['test -z "$tracked"', "true"],
  ] as const) {
    const mutated = source.replace(step, step.replace(from, to));
    assert.notEqual(mutated, source);
    assert.throws(
      () => assertSourceOnlyGuard(mutated),
      /tracked-native guard must fail on tracked paths or Git errors/,
    );
  }
});

test("summary condition guard rejects removed and legacy always condition", () => {
  const source = readWorkflow();
  const removed = replaceInJob(
    source,
    SUMMARY_JOB,
    `${SUMMARY_CONDITION}\n`,
    "",
  );
  assert.notEqual(
    removed,
    source,
    "summary condition fixture target must exist",
  );
  assert.throws(
    () => assertSummaryJob(removed),
    /summary must use cancellation-aware reporting semantics/,
  );

  const legacy = replaceInJob(
    source,
    SUMMARY_JOB,
    SUMMARY_CONDITION,
    "    if: always()",
  );
  assert.notEqual(
    legacy,
    source,
    "summary condition fixture target must exist",
  );
  assert.throws(
    () => assertSummaryJob(legacy),
    /summary must use cancellation-aware reporting semantics/,
  );
});

test("summary dependency guard rejects an extra dependency", () => {
  const source = readWorkflow();
  const mutated = source.replace(
    "        manual,\n      ]",
    "        manual,\n        untrusted-job,\n      ]",
  );
  assert.notEqual(mutated, source, "summary fixture target must exist");
  assert.throws(
    () => assertSummaryJob(mutated),
    /validation-summary must depend on every selected job in source order/,
  );
});

test("summary dependency guard rejects a missing dependency", () => {
  const source = readWorkflow();
  const mutated = source.replace("        manual,\n", "");
  assert.notEqual(mutated, source, "summary fixture target must exist");
  assert.throws(
    () => assertSummaryJob(mutated),
    /validation-summary must depend on every selected job in source order/,
  );
});

test("summary dependency guard rejects a duplicate dependency", () => {
  const source = readWorkflow();
  const mutated = source.replace(
    "        manual,\n      ]",
    "        manual,\n        scope,\n      ]",
  );
  assert.notEqual(mutated, source, "summary fixture target must exist");
  assert.throws(
    () => assertSummaryJob(mutated),
    /validation-summary must depend on every selected job in source order/,
  );
});

test("selector dependency guard rejects removed needs", () => {
  const source = readWorkflow();
  assert.throws(
    () => assertWorkflow(source.replace("    needs: [scope]\n", "")),
    /depend on scope/,
  );
});

test("selector condition guard rejects removed and legacy predicates", () => {
  const source = readWorkflow();
  for (const id of NATIVE_FAMILIES) {
    const line = "    if: " + NATIVE_CONDITIONS[id] + "\n";
    assert.throws(
      () => assertWorkflow(replaceInJob(source, id, line, "")),
      /common selector fallback condition/,
    );
    assert.throws(
      () =>
        assertWorkflow(
          replaceInJob(
            source,
            id,
            line,
            "    if: " + OLD_SELECTOR_CONDITION + "\n",
          ),
        ),
      /common selector fallback condition/,
    );
  }
});

test("area guard rejects jobs that ignore the area outputs", () => {
  const source = readWorkflow();
  const fixedMatrix = '        check: ["tests", "race", "fuzz"]\n';
  // Without the race output, a skills-only change would race and fuzz the unchanged hooks.
  const ignoresRace =
    GO_SUITE_MATRIX_LINE.replace(
      "needs.scope.outputs.race == 'false'",
      "false",
    ) + "\n";
  const cases = [
    [
      "go-suite",
      "    if: " + GO_SUITE_CONDITION + "\n",
      "    if: " + SELECTOR_CONDITION + "\n",
      /common selector fallback condition/,
    ],
    [
      "go-suite",
      GO_SUITE_MATRIX_LINE + "\n",
      fixedMatrix,
      /go-suite must read its checks from the scope job/,
    ],
    [
      "go-suite",
      GO_SUITE_MATRIX_LINE + "\n",
      ignoresRace,
      /go-suite must read its checks from the scope job/,
    ],
  ] as const;
  for (const [id, from, to, expected] of cases) {
    assert.throws(
      () => assertWorkflow(replaceInJob(source, id, from, to)),
      expected,
    );
  }
});

test("the scope job exports area outputs and no path filter selects jobs", () => {
  const source = readWorkflow();
  const scope = jobBlock(source, "scope");
  for (const name of ["areas", "shards", "race", "ts_shards"]) {
    assert.ok(
      scope.includes(`      ${name}: \${{ steps.classify.outputs.${name} }}\n`),
      `scope must export ${name}`,
    );
  }
  assert.doesNotMatch(source, /^\s+paths(?:-ignore)?:/m);
});

test("the workflow shard literals equal the shard table", () => {
  const table = JSON.stringify(SHARDS);
  assert.equal(SHARD_FALLBACK, table);
  assert.equal(JSON.stringify(COVERAGE_SHARDS), table);
});

test("negative: format guard rejects Go setup or script tests in the format job", () => {
  const source = readWorkflow();
  const check = "      - name: Check formatting\n";
  const withGo = replaceInJob(
    source,
    "format",
    check,
    `      - uses: ${SETUP_GO} # v7.0.0\n        with:\n          go-version-file: hooks/go/go.mod\n\n${check}`,
  );
  assert.throws(() => assertWorkflow(withGo), new RegExp(FORMAT_GO_MESSAGE));
  const withTests = replaceInJob(
    source,
    "format",
    check,
    `      - name: Run the maintainer script tests\n        run: node --test ".github/tools/src/scripts/*.test.ts"\n\n${check}`,
  );
  assert.throws(
    () => assertWorkflow(withTests),
    new RegExp(FORMAT_TESTS_MESSAGE),
  );
});

test("negative: conformance guard rejects a dropped leg, a shallow history leg, and a misplaced --fast", () => {
  const source = readWorkflow();
  const cases: [string, string, string, RegExp][] = [
    [
      "a dropped leg",
      "          - { domain: repo, label: Repo, runner: ubuntu-slim }\n",
      "",
      new RegExp(CONFORMANCE_LEGS_MESSAGE),
    ],
    [
      "the hooks leg on the small runner",
      "          - { domain: hooks, label: Hooks, runner: ubuntu-latest }\n",
      "          - { domain: hooks, label: Hooks, runner: ubuntu-slim }\n",
      new RegExp(CONFORMANCE_LEGS_MESSAGE),
    ],
    [
      "a fixed domain",
      "          domain: ${{ matrix.domain }}\n",
      "          domain: all\n",
      new RegExp(CONFORMANCE_LEGS_MESSAGE),
    ],
    [
      "module tests skipped on every leg",
      "          skip-module-tests: ${{ matrix.domain == 'hooks' }}\n",
      "          skip-module-tests: true\n",
      new RegExp(SKIP_MODULE_TESTS_MESSAGE),
    ],
    [
      "no --fast on the hooks leg",
      "          skip-module-tests: ${{ matrix.domain == 'hooks' }}\n",
      "",
      new RegExp(SKIP_MODULE_TESTS_MESSAGE),
    ],
    [
      "MCP examples dropped",
      "        run: node --test .github/tools/src/skill-examples/mcp-safe-write.test.ts\n",
      "        run: true\n",
      new RegExp(MCP_EXAMPLES_MESSAGE),
    ],
    [
      "MCP examples on another leg",
      "        if: matrix.domain == 'skills'\n",
      "        if: matrix.domain == 'docs'\n",
      new RegExp(MCP_EXAMPLES_MESSAGE),
    ],
    [
      "a conditional leg",
      "    runs-on: ${{ matrix.runner }}\n",
      "    runs-on: ${{ matrix.runner }}\n    if: github.event_name == 'pull_request'\n",
      /conformance must remain unconditionally independent/,
    ],
  ];
  for (const [name, from, to, expected] of cases) {
    assert.throws(
      () => assertWorkflow(replaceInJob(source, "conformance", from, to)),
      expected,
      name,
    );
  }
});

test("negative: Go guard rejects a dropped race, fuzz, crasher or floor step and widened race legs", () => {
  const source = readWorkflow();
  const cases: [string, string, string, RegExp][] = [
    ["race on macOS", GO_SUITE_EXCLUDES[0]!, "", new RegExp(GO_CHECKS_MESSAGE)],
    [
      "fuzz on Windows",
      GO_SUITE_EXCLUDES[3]!,
      "",
      new RegExp(GO_CHECKS_MESSAGE),
    ],
    [
      "race without CGO",
      '          CGO_ENABLED: "1"\n',
      "",
      new RegExp(GO_RACE_MESSAGE),
    ],
    [
      "race without -race",
      `        run: ${GO_RACE_COMMAND}\n`,
      "        run: go test -C hooks/go -count=1 ./...\n",
      new RegExp(GO_RACE_MESSAGE),
    ],
    [
      "race on the tests leg",
      "        if: matrix.check == 'race'\n",
      "        if: matrix.check == 'tests'\n",
      new RegExp(GO_RACE_MESSAGE),
    ],
    [
      "shorter fuzzing",
      // String.replace reads "$'" in a replacement as a pattern, so the mutation targets text without one.
      "-fuzztime=30s\n\n      - name: Fuzz provider normalization",
      "-fuzztime=1s\n\n      - name: Fuzz provider normalization",
      new RegExp(GO_FUZZ_MESSAGE),
    ],
    [
      "crashers kept on success only",
      "        if: failure() && matrix.check == 'fuzz'\n",
      "        if: matrix.check == 'fuzz'\n",
      new RegExp(GO_CRASHER_MESSAGE),
    ],
    [
      "the old floor script restored",
      `        run: ${HOOKS_FLOOR_COMMAND}\n`,
      "        run: |\n          total=$(go -C hooks/go tool cover)\n",
      new RegExp(GO_FLOOR_MESSAGE),
    ],
    [
      "a wrong floor subcommand",
      `        run: ${HOOKS_FLOOR_COMMAND}\n`,
      `        run: ${HOOKS_TEST_COMMAND}\n`,
      new RegExp(GO_FLOOR_MESSAGE),
    ],
    [
      "a floor on the race leg",
      `        if: matrix.check == 'tests' && matrix.os == 'ubuntu-latest'\n        run: ${HOOKS_FLOOR_COMMAND}\n`,
      `        if: matrix.check == 'race' && matrix.os == 'ubuntu-latest'\n        run: ${HOOKS_FLOOR_COMMAND}\n`,
      new RegExp(GO_FLOOR_MESSAGE),
    ],
  ];
  for (const [name, from, to, expected] of cases) {
    assert.throws(
      () => assertWorkflow(replaceInJob(source, "go-suite", from, to)),
      expected,
      name,
    );
  }
});

test("negative: TypeScript guard rejects a dropped typecheck or boundary check", () => {
  const source = readWorkflow();
  for (const [from, to] of [
    ["        run: npm run test:typecheck\n", "        run: true\n"],
    ["        run: npm run test:boundaries\n", "        run: true\n"],
    [
      "      - name: Typecheck every npm workspace\n        if: runner.os == 'Linux'\n",
      "      - name: Typecheck every npm workspace\n        if: runner.os == 'Windows'\n",
    ],
  ] as const) {
    assert.throws(
      () => assertWorkflow(replaceInJob(source, "ts-suite", from, to)),
      new RegExp(TS_CHECKS_MESSAGE),
    );
  }
});

test("weekly schedule guard rejects removed entrypoint", () => {
  const source = readWorkflow();
  assert.throws(
    () => assertWorkflow(source.replace('    - cron: "17 6 * * 1"\n', "")),
    /weekly schedule/,
  );
});

test("push trigger guard rejects removed post-merge entrypoint", () => {
  const source = readWorkflow();
  const mutated = source.replace("  push:\n    branches: [main]\n", "");
  assert.notEqual(mutated, source, "push fixture target must exist");
  assert.throws(
    () => assertWorkflow(mutated),
    /push trigger must validate main after a merge/,
  );
});

test("post-merge guard rejects a run on a pull request, a push or the schedule", () => {
  const source = readWorkflow();
  // The selector alone would run on every event; the old condition ran on every event but a pull request.
  for (const condition of [
    SELECTOR_CONDITION,
    PR_RUNNING_POST_MERGE_CONDITION,
  ]) {
    const mutated = replaceInJob(
      source,
      "manual",
      `    if: ${POST_MERGE_CONDITION}\n`,
      `    if: ${condition}\n`,
    );
    assert.throws(
      () => assertWorkflow(mutated),
      /manual must run only on manual dispatch/,
    );
  }
  // A condition inside the called workflow would hide a skipped job from validation-summary.
  for (const id of POST_MERGE_FAMILIES) {
    const mutated = replaceInJob(
      source,
      id,
      "    runs-on: ",
      `    if: ${POST_MERGE_CONDITION}\n    runs-on: `,
    );
    assert.throws(
      () => assertWorkflow(mutated),
      new RegExp(`${id} must run only on manual dispatch`),
    );
  }
});

test("push guards reject the pull-request-only selector, a push that does not cancel, and a dropped lookup", () => {
  const source = readWorkflow();
  for (const id of NATIVE_FAMILIES) {
    const line = "    if: " + NATIVE_CONDITIONS[id] + "\n";
    const prOnly = line.replace(
      "!(needs.scope.result",
      "!(github.event_name == 'pull_request' && needs.scope.result",
    );
    assert.notEqual(prOnly, line);
    assert.throws(
      () => assertWorkflow(replaceInJob(source, id, line, prOnly)),
      /common selector fallback condition/,
    );
  }
  assert.ok(PR_ONLY_SELECTOR_CONDITION.includes("pull_request"));
  const noCancel = source.replace(
    "  cancel-in-progress: ${{ github.event_name == 'pull_request' || github.event_name == 'push' }}",
    "  cancel-in-progress: ${{ github.event_name == 'pull_request' }}",
  );
  assert.notEqual(noCancel, source);
  assert.throws(
    () => assertWorkflow(noCancel),
    /only a newer run of the same PR or pushed ref may cancel a predecessor/,
  );
  for (const id of ["scope"]) {
    assert.throws(
      () =>
        assertWorkflow(
          replaceInJob(
            source,
            id,
            "          AGX_CI_LAST_GREEN: ${{ steps.last.outputs.sha }}\n",
            "",
          ),
        ),
      new RegExp(`${id} must look up the last validated commit`),
    );
    assert.throws(
      () =>
        assertWorkflow(replaceInJob(source, id, "      actions: read\n", "")),
      new RegExp(`${id} must hold actions: read`),
    );
  }
});

test("go-suite-dispatch-only-for-other-oses", () => {
  const source = readWorkflow();
  const widened = replaceInJob(
    source,
    "go-suite",
    osList(["ubuntu-latest"]),
    osList(FULL_OS),
  );
  assert.throws(
    () => assertWorkflow(widened),
    /hooks must retain every supported OS on manual dispatch and only Ubuntu otherwise/,
  );
});

// go-suite is the only job that runs the Go tests, so it must run on pull requests too.
test("go-suite runs on a pull request", () => {
  const source = readWorkflow();
  const line = "    if: " + GO_SUITE_CONDITION + "\n";
  assert.throws(
    () =>
      assertWorkflow(
        replaceInJob(
          source,
          "go-suite",
          line,
          "    if: " + PR_SKIPPING_GO_SUITE_CONDITION + "\n",
        ),
      ),
    /go-suite must use the common selector fallback condition/,
  );
});

test("cache-mode guard rejects an expression value", () => {
  const source = readWorkflow();
  const mutated = source.replace(
    "\njobs:\n",
    `\n${EXPRESSION_CACHE_MODE}\n\njobs:\n`,
  );
  assert.notEqual(mutated, source, "cache mode fixture target must exist");
  assert.throws(
    () => assertWorkflow(mutated),
    /cache-mode must be a literal; GitHub rejects the workflow file/,
  );
});

test("cache-hit guard rejects a conditional test skip", () => {
  const source = readWorkflow();
  const mutated = source.replace(
    "      - name: Race hook packages\n        if: matrix.check == 'race'\n",
    "      - name: Race hook packages\n        if: matrix.check == 'race' && steps.go-cache.outputs.cache-hit != 'true'\n",
  );
  assert.notEqual(mutated, source, "cache-hit fixture target must exist");
  assert.throws(() => assertWorkflow(mutated), /cache-hit/);
});

test("fresh-test guard rejects removed count=1", () => {
  const source = readWorkflow();
  const mutated = source.replace(
    "go test -C hooks/go -count=1 -race ./...",
    "go test -C hooks/go -race ./...",
  );
  assert.notEqual(mutated, source, "count fixture target must exist");
  assert.throws(
    () => assertWorkflow(mutated),
    /selected Go test must be uncached/,
  );
});

function assertHostBuildNegative(id: string): void {
  const source = readWorkflow();
  const broad = replaceInJob(
    source,
    id,
    SMOKE_BUILD_RUN,
    "        run: make -B -C hooks/go build-all\n",
  );
  assert.throws(
    () => assertWorkflow(broad),
    new RegExp(`${id} must not rebuild every hook binary`),
  );
  // The module's own arguments stand in for a module that rebuilds everything.
  assert.throws(
    () =>
      assertHostTargetBuild(source, id, HOST_TABLE, () => [
        "-B",
        "-C",
        "hooks/go",
        "build-all",
      ]),
    new RegExp(`${id} must not rebuild every hook binary`),
  );
  const oldScript = replaceInJob(
    source,
    id,
    SMOKE_BUILD_RUN,
    '        run: |\n          set -euo pipefail\n          make -B -C hooks/go "../bin/$name"\n',
  );
  assert.throws(
    () => assertWorkflow(oldScript),
    new RegExp(`${id} host build proof is missing`),
  );
  const wrongShell = replaceInJob(
    source,
    id,
    "        shell: bash\n        run: node .github/tools/src/scripts/ci-native-smoke.ts build\n",
    "        shell: pwsh\n        run: node .github/tools/src/scripts/ci-native-smoke.ts build\n",
  );
  assert.throws(
    () => assertWorkflow(wrongShell),
    new RegExp(`${id} host build proof is missing`),
  );
  const { "Windows/ARM64": _dropped, ...fewer } = HOST_TABLE;
  assert.throws(
    () => assertHostTargetBuild(source, id, fewer),
    new RegExp(`${id} host build proof is missing: Windows/ARM64`),
  );
}

test("broad-native-smoke", () => {
  assertHostBuildNegative("native-hook-smoke");
});

test("checksum-native-smoke", () => {
  assertHostBuildNegative("native-hook-smoke");
});

test("manifest-native-smoke", () => {
  assertHostBuildNegative("native-hook-smoke");
});

test("conformance and native smoke guards reject removed coverage", () => {
  const source = readWorkflow();
  const conformanceBuild = replaceInJob(
    source,
    "go-suite",
    "          make -B -C hooks/go build-all\n",
    "",
  );
  assert.throws(
    () => assertWorkflow(conformanceBuild),
    /hooks must retain the complete rebuild/,
  );
  const conformanceDiff = replaceInJob(
    source,
    "go-suite",
    "          git diff --exit-code -- hooks/ bin/\n",
    "",
  );
  assert.throws(
    () => assertWorkflow(conformanceDiff),
    /hooks must retain the complete hooks diff/,
  );
  const nativeIdentity = replaceInJob(
    source,
    "native-hook-smoke",
    "        os: [macos-15-intel, windows-11-arm]\n",
    "        os: [macos-15, windows-latest]\n",
  );
  assert.throws(
    () => assertWorkflow(nativeIdentity),
    /native smoke must retain both native host identities/,
  );
  const nativeDirect = replaceInJob(
    source,
    "native-hook-smoke",
    "      - name: Run the native binary directly (macOS)\n",
    "",
  );
  assert.throws(
    () => assertWorkflow(nativeDirect),
    /native smoke proof is missing: Run the native binary directly \(macOS\)/,
  );
  const nativeDispatch = replaceInJob(
    source,
    "native-hook-smoke",
    "      - name: command-guardian dispatch smoke (Windows)\n",
    "",
  );
  assert.throws(
    () => assertWorkflow(nativeDispatch),
    /native smoke proof is missing: command-guardian dispatch smoke \(Windows\)/,
  );
});

function assertConformanceOsNegative(os: string): void {
  const source = readWorkflow();
  const missingOs = replaceInJob(
    source,
    "go-suite",
    osList(FULL_OS),
    osList(FULL_OS.filter((candidate) => candidate !== os)),
  );
  assert.throws(
    () => assertWorkflow(missingOs),
    /hooks must retain every supported OS/,
  );
}

test("conformance-os-ubuntu", () => {
  assertConformanceOsNegative("ubuntu-latest");
});

test("conformance-os-macos", () => {
  assertConformanceOsNegative("xcode-27");
});

test("conformance-os-windows", () => {
  assertConformanceOsNegative("windows-latest");
});

test("Windows dispatch and release rederivation guards reject drift", () => {
  const source = readWorkflow();
  const windowsBuild = replaceInJob(
    source,
    "validate-windows-dispatch",
    "          make -B -C hooks/go build-all\n",
    "",
  );
  assert.throws(
    () => assertWorkflow(windowsBuild),
    /Windows dispatcher proof is missing: make -B -C hooks\/go build-all/,
  );
  const windowsDiff = replaceInJob(
    source,
    "validate-windows-dispatch",
    "          git diff --exit-code -- hooks/ bin/\n",
    "          git diff --exit-code -- hooks/\n",
  );
  assert.throws(
    () => assertWorkflow(windowsDiff),
    /Windows dispatcher proof is missing: git diff --exit-code -- hooks\/ bin\//,
  );
  const driftCheck = replaceInJob(
    source,
    "gh-aw-lock-drift",
    GH_AW_CHECK_RUN,
    "        run: node .github/tools/src/scripts/ci-gh-aw.ts install\n",
  );
  assert.throws(
    () => assertWorkflow(driftCheck),
    /gh-aw lock rederivation must catch stale and uncommitted locks/,
  );
  const oldCheck = replaceInJob(
    source,
    "gh-aw-lock-drift",
    GH_AW_CHECK_RUN,
    "        run: |\n          gh-aw compile release\n",
  );
  assert.throws(
    () => assertWorkflow(oldCheck),
    /gh-aw lock rederivation must catch stale and uncommitted locks/,
  );
  const oldInstall = replaceInJob(
    source,
    "gh-aw-lock-drift",
    GH_AW_INSTALL_RUN,
    "        run: |\n          curl -fsSL -o gh-aw url\n",
  );
  assert.throws(
    () => assertWorkflow(oldInstall),
    /gh-aw must be installed from the pinned release/,
  );
  const unpinned = replaceInJob(
    source,
    "gh-aw-lock-drift",
    "          GH_AW_VERSION: v1.0.1\n          GH_AW_SHA256:",
    "          GH_AW_SHA256:",
  );
  assert.throws(
    () => assertWorkflow(unpinned),
    /gh-aw must be installed from the pinned release/,
  );
  // The module's lists stand in for a module that stops compiling a workflow or watching a path.
  assert.throws(
    () => assertGhAwDrift(source, ["release"]),
    /backlog grooming lock must be rederived in validation/,
  );
  assert.throws(
    () => assertGhAwDrift(source, ["backlog-grooming"]),
    /release lock must be rederived in validation/,
  );
  assert.throws(
    () => assertGhAwDrift(source, LOCK_WORKFLOWS, [".github/workflows"]),
    /gh-aw lock rederivation must catch stale and uncommitted locks/,
  );
});

test("negative: Windows dispatcher guard rejects a dropped hook step", () => {
  const source = readWorkflow();
  const job = jobBlock(source, "validate-windows-dispatch");
  for (const name of WINDOWS_HOOK_STEPS) {
    const header = `      - name: ${name}\n`;
    const mutated = replaceInJob(
      source,
      "validate-windows-dispatch",
      header + stepBlock(job, name)!,
      "",
    );
    assert.notEqual(mutated, source, name);
    assert.throws(
      () => assertWindowsDispatchBuild(mutated),
      { message: `Windows dispatcher proof is missing the step: ${name}` },
      name,
    );
  }
});

// The in-repo eval engine is frozen and its tests moved with the engine to its own repository, so neither
// TypeScript workflow may run them again.
function assertNoEvalEngineRun(source: string, file: string): void {
  for (const needle of ["evals/src/", "ts-evals", "windows-evals"]) {
    assert.ok(!source.includes(needle), `${file} must not run ${needle}`);
  }
}

test("validate no longer runs the in-repo eval engine", () => {
  assertNoEvalEngineRun(readWorkflow(), "validate.yml");
  assert.throws(
    () =>
      assertNoEvalEngineRun(
        readWorkflow().replace(
          "'[\"ts-tools\"]'",
          '\'["ts-tools","ts-evals"]\'',
        ),
        "validate.yml",
      ),
    { message: "validate.yml must not run ts-evals" },
  );
  assert.throws(
    () =>
      assertNoEvalEngineRun(
        readWorkflow().replace(
          TS_COMBINE_COMMAND,
          `node --test "evals/src/process/**/*.test.ts"\n          ${TS_COMBINE_COMMAND}`,
        ),
        "validate.yml",
      ),
    { message: "validate.yml must not run evals/src/" },
  );
});

function readRelease(file: string): string {
  return fs
    .readFileSync(path.join(ROOT, ".github/workflows", file), "utf8")
    .replaceAll("\r\n", "\n");
}

// The trusted action rebuilds the selected commit in a separate checkout. Its
// output is the comparison source for the two archives and all other assets.
function assertReleaseRederivation(source: string, file: string): void {
  const workflow = parseWorkflow(
    file.endsWith(".md") ? source.split("---\n")[1]! : source,
  );
  const steps = workflow.jobs.stage!.steps;
  const rebuild = steps.find(
    (step) => step.name === "Independently rebuild the release bundle",
  );
  assert.equal(rebuild?.uses, "./.github/actions/release");
  assert.equal(rebuild?.with?.command, "bundle");
  assert.equal(rebuild?.with?.repo, "${{ github.workspace }}/release-target");
  assert.equal(rebuild?.with?.sha, "${{ needs.prepare.outputs.target_sha }}");
  assert.equal(rebuild?.with?.out, "${{ github.workspace }}/rebuilt");
  const comparison = steps.find(
    (step) => step.name === "Compare assets in the release roster",
  )?.run;
  assert.ok(comparison, `${file} must compare the release assets`);
  assert.match(
    comparison,
    /node \.github\/tools\/src\/release\/cli\.ts asset-roster --tag "\$TAG"/,
  );
  assert.match(
    comparison,
    /deepStrictEqual\(roster\.hookBinaryPaths, HOOK_BINARY_PATHS,/,
  );
  assert.match(comparison, /cmp "rebuilt\/\$name" "release-assets\/\$name"/);
  assert.doesNotMatch(
    comparison.replace(/^\s*#.*$/gm, ""),
    /bin\/agx\.exe/,
    `${file} must exclude the dispatcher from release assets`,
  );
}

test("the release stage rebuilds the selected target and compares the release roster", () => {
  for (const file of ["release.md", "release.lock.yml"]) {
    const source = readRelease(file);
    assertReleaseRederivation(source, file);
    const wrongSource = source.replaceAll(
      "repo: ${{ github.workspace }}/release-target",
      "repo: ${{ github.workspace }}",
    );
    assert.notEqual(wrongSource, source);
    assert.throws(() => assertReleaseRederivation(wrongSource, file));
    const launcherAsset = source.replace(
      /^( +)cmp "rebuilt\/\$name"/m,
      "$1cp bin/agx.exe rebuilt/agx.exe\n$&",
    );
    assert.notEqual(launcherAsset, source);
    assert.throws(
      () => assertReleaseRederivation(launcherAsset, file),
      /must exclude the dispatcher from release assets/,
    );
  }
});

const NPM_WORKFLOWS = [
  ".github/workflows/validate.yml",
  ...Object.values(CALLED_WORKFLOWS).map((name) => `.github/workflows/${name}`),
  ".github/workflows/release.md",
  ".github/workflows/release.lock.yml",
];
const NPM_USE = /\b(npm (ci|install)|npx)\b/;

function npmFloor(manifest: string): string {
  const file = path.join(ROOT, manifest);
  const document: EngineManifestFixture = JSON.parse(
    fs.readFileSync(file, "utf8"),
  );
  const range = document.engines.npm;
  const match = /^>=(\d+\.\d+\.\d+)$/.exec(range);
  assert.ok(match, `${manifest} engines.npm must read >=X.Y.Z, got ${range}`);
  return match[1]!;
}

// Node 26 bundles npm 11, so each job that runs npm must install the pinned
// npm 12 after setup-node and before its first npm command.
function npmUsesWithoutPin(source: string, version: string): string[] {
  const pin = `npm install --global --ignore-scripts npm@${version}`;
  const missing: string[] = [];
  let pinned = false;
  source.split("\n").forEach((line, index) => {
    const text = line.trim();
    if (text.startsWith("#")) return;
    if (text.includes("actions/setup-node@")) pinned = false;
    if (text.endsWith(pin)) {
      pinned = true;
    } else if (NPM_USE.test(text) && !pinned) {
      missing.push(`line ${index + 1}: ${text}`);
    }
  });
  return missing;
}

test("npm 12 is installed before every npm command in CI", () => {
  const version = npmFloor("package.json");
  assert.equal(
    npmFloor(".github/release/package.json"),
    version,
    "the release tooling must require the same npm as the root",
  );
  for (const workflow of NPM_WORKFLOWS) {
    const source = fs
      .readFileSync(path.join(ROOT, workflow), "utf8")
      .replaceAll("\r\n", "\n");
    assert.deepEqual(npmUsesWithoutPin(source, version), [], workflow);
  }
});

test("npm pin guard rejects a job that drops the install step", () => {
  const version = npmFloor("package.json");
  const source = readWorkflow();
  const pin = `        run: npm install --global --ignore-scripts npm@${version}\n`;
  assert.ok(source.includes(pin), "validate.yml must carry the npm pin step");
  const dropped = source.replace(`      - name: Install npm 12\n${pin}\n`, "");
  assert.notEqual(dropped, source);
  assert.notDeepEqual(npmUsesWithoutPin(dropped, version), []);
  const stale = source.replaceAll(`npm@${version}`, "npm@11.21.1");
  assert.notDeepEqual(npmUsesWithoutPin(stale, version), []);
});

const PR_TITLE_TEST = "node --test .github/tools/src/scripts/pr-title.test.ts";
const LOCKED_TOOLCHAIN = "npm ci --ignore-scripts";

function assertClaudeTitleDependencies(source: string): void {
  const job = parseWorkflow(source).jobs["validate-claude-native"]!;
  const npmPin = `npm install --global --ignore-scripts npm@${npmFloor("package.json")}`;
  const npm = job.steps.findIndex((step) => step.run === npmPin);
  const install = job.steps.findIndex((step) => step.run === LOCKED_TOOLCHAIN);
  const title = job.steps.findIndex((step) => step.run === PR_TITLE_TEST);
  assert.ok(
    npm >= 0 && install > npm && title > install,
    "Claude title regressions must install locked root dependencies after pinning npm and before running the title suite",
  );
}

test("Claude title regressions install the locked toolchain before running", () => {
  const source = readFile("validate.yml");
  assertClaudeTitleDependencies(source);
  const job = "validate-claude-native";
  const install = `      - name: Install pinned toolchain\n        run: ${LOCKED_TOOLCHAIN}\n\n`;
  const titleRun = `        run: ${PR_TITLE_TEST}\n\n`;
  const withoutInstall = replaceInJob(source, job, install, "");
  for (const candidate of [
    withoutInstall,
    replaceInJob(withoutInstall, job, titleRun, titleRun + install),
    replaceInJob(source, job, LOCKED_TOOLCHAIN, "npm ci"),
  ]) {
    assert.throws(
      () => assertClaudeTitleDependencies(candidate),
      /Claude title regressions must install locked root dependencies/,
    );
  }
});

function assertCoverageRejects(
  job: string,
  old: string,
  replacement: string,
  pattern: RegExp,
): void {
  const mutated = replaceInJob(readWorkflow(), job, old, replacement);
  assert.throws(() => assertCoverageReport(mutated), pattern);
}

test("coverage guard accepts the shipped wiring", () => {
  assertCoverageReport(readWorkflow());
});

test("Go profile conversion guard rejects missing Node and module mappings", () => {
  const source = readWorkflow();
  for (const [from, to, expected] of [
    [SETUP_NODE, "actions/setup-node@untrusted", /needs pinned Node/],
    [
      "--source-root . --module hooks/go",
      "--module hooks/go",
      /preserve hooks\/go/,
    ],
  ] as const) {
    assert.throws(
      () =>
        assertGoProfileConversion(
          replaceInJob(source, "coverage-report", from, to),
        ),
      expected,
    );
  }
});

test(
  "Go workflow conversion writes the final XML directly without invoking Go",
  { skip: requireInterpreter("bash") },
  (t) => {
    const report = parseWorkflow(readWorkflow()).jobs["coverage-report"]!;
    const conversion = report.steps.find(
      (step) => step.name === "Convert profiles to Cobertura XML",
    )!.run;
    const root = fs.mkdtempSync(
      path.join(os.tmpdir(), "agentics-go-coverage-conversion-"),
    );
    t.after(() => fs.rmSync(root, { recursive: true, force: true }));
    const coverage = path.join(root, ".github/tools/src/coverage");
    fs.mkdirSync(coverage, { recursive: true });
    for (const file of fs.readdirSync(
      path.join(ROOT, ".github/tools/src/coverage"),
    )) {
      if (file.endsWith(".ts") && !file.endsWith(".test.ts"))
        fs.copyFileSync(
          path.join(ROOT, ".github/tools/src/coverage", file),
          path.join(coverage, file),
        );
    }
    fs.mkdirSync(path.join(root, ".github/tools/src/scripts"), {
      recursive: true,
    });
    copyCoverageSourcePolicy(root);
    const bin = path.join(root, "bin");
    fs.mkdirSync(bin);
    fs.writeFileSync(
      path.join(bin, "go"),
      '#!/bin/sh\necho called > "$GO_MARKER"\nexit 97\n',
      { mode: 0o755 },
    );
    const moduleRoot = path.join(root, "hooks/go");
    fs.mkdirSync(path.join(moduleRoot, "sub", "path"), { recursive: true });
    fs.mkdirSync(path.join(moduleRoot, "second"), { recursive: true });
    fs.writeFileSync(
      path.join(moduleRoot, "go.mod"),
      "module example.test/sample\n",
    );
    for (const file of ["sub/path/a&b'c.go", "sub/path/z.go", "second/b.go"])
      fs.writeFileSync(
        path.join(moduleRoot, file),
        "package sample\nfunc Hit() int {\n  return 1\n}\n",
      );
    const profile = [
      "mode: atomic",
      "example.test/sample/sub\\path/z.go:2.16,4.2 1 0",
      "example.test/sample/second/b.go:2.16,4.2 1 2",
      "example.test/sample/sub\\path/a&b'c.go:2.16,4.2 1 3",
      "",
    ].join("\n");
    const bash = findInterpreter("bash")!;
    const marker = path.join(root, "go-called");
    const reportPath = path.join(root, "cobertura.xml");
    const runConversion = (input: string) => {
      fs.writeFileSync(path.join(root, "coverage-hooks.out"), input);
      const result = spawnSync(bash, ["-eo", "pipefail", "-c", conversion], {
        cwd: root,
        encoding: "utf8",
        env: {
          ...process.env,
          PATH: bin + path.delimiter + process.env.PATH,
          GO_MARKER: marker,
          NODE_OPTIONS:
            "--import=data:text/javascript,Date.now%20=%20()%20=>%20123456789",
        },
        timeout: 30_000,
      });
      assert.ifError(result.error);
      assert.equal(result.status, 0, result.stdout + result.stderr);
      assert.equal(
        fs.existsSync(marker),
        false,
        "conversion must not launch Go",
      );
      assert.equal(
        fs.existsSync(path.join(root, "cobertura-hooks.xml")),
        false,
        "conversion must write the final report directly",
      );
      const xml = fs.readFileSync(reportPath);
      fs.rmSync(reportPath);
      return xml;
    };
    // The module converter and prefix step at the same timestamp produced these bytes.
    const xml = runConversion(profile);
    assert.equal(
      createHash("sha256").update(xml).digest("hex"),
      "709a6985d13282d0074bfdb3dbb5db05cc3cf4e018db0410a22baf5916408f23",
    );
    assert.match(
      xml.toString("utf8"),
      /filename="hooks\/go\/sub\/path\/a&amp;b'c\.go"/,
    );
    assert.equal(
      createHash("sha256")
        .update(runConversion("mode: atomic\n"))
        .digest("hex"),
      "e6160905b15dd7eb98becff6129ade8c0c9deaccc42840ca45bff53ffd29f51f",
    );
  },
);

test(
  "coverage upload eligibility follows supported provider contexts",
  { skip: requireInterpreter("bash") },
  async (t) => {
    for (const id of ["coverage-upload", "ts-coverage-upload"]) {
      await t.test(id, async (t) => {
        const upload = parseWorkflow(readWorkflow()).jobs[id]!;
        assert.ok(upload, `${id} job is missing`);
        const context = upload.steps.find((step) => step.id === "context");
        const action = upload.steps.find((step) =>
          step.uses?.startsWith("actions/upload-code-coverage@"),
        )!;
        const bash = findInterpreter("bash")!;
        const cases = [
          [
            "manual feature branch",
            "workflow_dispatch",
            "refs/heads/perf/ci-under-five-minutes",
            "main",
            false,
          ],
          [
            "manual default branch",
            "workflow_dispatch",
            "refs/heads/main",
            "main",
            true,
          ],
          [
            "renamed default branch",
            "workflow_dispatch",
            "refs/heads/trunk",
            "trunk",
            true,
          ],
          [
            "case-distinct branch",
            "workflow_dispatch",
            "refs/heads/Main",
            "main",
            false,
          ],
          ["manual tag", "workflow_dispatch", "refs/tags/v1", "main", false],
          ["default branch push", "push", "refs/heads/main", "main", true],
          [
            "scheduled default branch",
            "schedule",
            "refs/heads/main",
            "main",
            true,
          ],
          ["pull request", "pull_request", "refs/pull/1/merge", "main", true],
        ] as const;
        for (const [name, event, ref, defaultBranch, expected] of cases) {
          await t.test(name, (t) => {
            const dir = fs.mkdtempSync(
              path.join(os.tmpdir(), "agentics-coverage-context-"),
            );
            t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
            let shouldUpload;
            if (context) {
              assert.equal(
                context.env.DEFAULT_BRANCH,
                "${{ github.event.repository.default_branch }}",
              );
              const result = spawnSync(bash, ["-c", context.run], {
                cwd: dir,
                env: {
                  ...process.env,
                  GITHUB_EVENT_NAME: event,
                  GITHUB_REF: ref,
                  DEFAULT_BRANCH: defaultBranch,
                  GITHUB_OUTPUT: "github-output",
                },
                encoding: "utf8",
                timeout: 10000,
              });
              assert.equal(result.status, 0, result.stdout + result.stderr);
              const output = fs.readFileSync(
                path.join(dir, "github-output"),
                "utf8",
              );
              const value = output.match(/^should_upload=(true|false)$/m);
              assert.ok(value, "eligibility step must write its decision");
              shouldUpload = value[1] === "true";
            }
            let enabled = true;
            if (action.if) {
              assert.equal(
                action.if,
                "steps.context.outputs.should_upload == 'true'",
              );
              enabled = shouldUpload === true;
            }
            assert.equal(
              enabled,
              expected,
              `${name}: provider upload eligibility`,
            );
          });
        }
      });
    }
  },
);

test("TypeScript upload job skips fork PRs, failed reports, and cancelled runs", () => {
  const upload = parseWorkflow(readWorkflow()).jobs["ts-coverage-upload"]!;
  assert.ok(upload, "ts-coverage-upload job is missing");
  for (const [event, head, result, cancelled, expected] of [
    ["pull_request", "owner/repo", "success", false, true],
    ["pull_request", "fork/repo", "success", false, false],
    ["push", undefined, "success", false, true],
    ["schedule", undefined, "success", false, true],
    ["workflow_dispatch", undefined, "success", false, true],
    ["push", undefined, "failure", false, false],
    ["push", undefined, "skipped", false, false],
    ["push", undefined, "success", true, false],
  ] as const) {
    assert.equal(
      actionsExpression(
        upload.if.replaceAll("needs.ts-coverage-report", "needs.report"),
        ["github", "needs", "cancelled"],
        [
          {
            event_name: event,
            repository: "owner/repo",
            event: head
              ? { pull_request: { head: { repo: { full_name: head } } } }
              : {},
          },
          { report: { result } },
          () => cancelled,
        ],
      ),
      expected,
      `${event}: ${head}, ${result}, cancelled=${cancelled}`,
    );
  }
});

test("coverage guard rejects an intermediate Cobertura output", () => {
  assertCoverageRejects(
    "coverage-report",
    "--out cobertura.xml",
    "--out cobertura-hooks.xml",
    /coverage-report must write the final XML directly/,
  );
});

test("coverage guard rejects a shard that stops recording a profile", () => {
  assertCoverageRejects(
    "go-suite",
    `        run: ${HOOKS_TEST_COMMAND}\n`,
    "        run: go test -C hooks/go -count=1 ./...\n",
    /(?:every shard must record|go-suite must write one) a? ?coverage profile/,
  );
});

test("coverage guard rejects a test step that runs the wrong subcommand", () => {
  assertCoverageRejects(
    "go-suite",
    `        run: ${HOOKS_TEST_COMMAND}\n`,
    `        run: ${HOOKS_FLOOR_COMMAND}\n`,
    /go-suite must write one coverage profile per shard/,
  );
});

test("coverage guard rejects a dropped profile upload", () => {
  assertCoverageRejects(
    "go-suite",
    "          if-no-files-found: error\n          retention-days: 1\n",
    "          retention-days: 1\n",
    /go-suite must upload each shard profile/,
  );
});

test("coverage guard rejects a shard the profile check does not list", () => {
  assertCoverageRejects(
    "coverage-report",
    "require-profiles hooks",
    "require-profiles none",
    /the profile check must list the go-suite shards/,
  );
});

test("coverage guard rejects a profile step that is not the module call", () => {
  for (const [name, command] of COVERAGE_STEP_COMMANDS) {
    assertCoverageRejects(
      "coverage-report",
      `${COVERAGE_MODULE} ${command}`,
      "echo skipped",
      new RegExp(`${name} step must run the module's ${command}`),
    );
  }
  assertCoverageRejects(
    "coverage-report",
    "require-profiles hooks",
    "merge-profiles hooks",
    /Require every shard profile step must run the module's require-profiles/,
  );
});

test("coverage guard rejects a changed download name", () => {
  assertCoverageRejects(
    "coverage-report",
    "          name: go-coverage-profile-hooks\n",
    "          name: ignored-hooks\n",
    /coverage-report must download every shard profile/,
  );
});

test("coverage guard rejects a report that waits on a green go-suite", () => {
  assertCoverageRejects(
    "coverage-report",
    "inputs.go-suite-result != 'skipped'",
    "inputs.go-suite-result == 'success'",
    /coverage-report must still run when an advisory leg fails/,
  );
});

test("coverage guard rejects a report that runs on a partial shard set", () => {
  assertCoverageRejects(
    "coverage-report",
    " && (inputs.shards == '' || inputs.shards == '" + SHARD_FALLBACK + "')",
    "",
    /coverage-report must still run when an advisory leg fails/,
  );
  assertCoverageRejects(
    "coverage",
    "    needs: [scope, go-suite]\n",
    "    needs: [scope]\n",
    /coverage-report must depend on go-suite/,
  );
});

test("coverage guard rejects a report that can write code-quality", () => {
  assertCoverageRejects(
    "coverage-report",
    "    timeout-minutes: 10\n",
    "    timeout-minutes: 10\n    permissions:\n      code-quality: write\n",
    /coverage-report runs repository code and must keep the read-only token/,
  );
});

test("coverage guard rejects an upload that stops waiting for the report", () => {
  assertCoverageRejects(
    "coverage-upload",
    "    needs: [coverage-report]\n",
    "    needs: [go-suite]\n",
    /coverage-upload must depend on coverage-report/,
  );
});

test("coverage guard rejects an upload without the fork guard", () => {
  assertCoverageRejects(
    "coverage-upload",
    " && (github.event_name != 'pull_request' || github.event.pull_request.head.repo.full_name == github.repository)",
    "",
    /coverage-upload must wait for the report and skip fork pull requests/,
  );
});

test("coverage guard rejects a write job that checks out code", () => {
  assertCoverageRejects(
    "coverage-upload",
    "    steps:\n",
    `    steps:\n      - uses: ${CHECKOUT}\n`,
    /must not check out repository code/,
  );
});

test("missing-binary proof succeeds under the Actions PowerShell exit wrapper", (t) => {
  if (process.platform !== "win32") {
    t.skip("requires native Windows cmd and PowerShell");
    return;
  }
  const parent = fs.realpathSync(os.tmpdir());
  const cwd = fs.mkdtempSync(path.join(parent, "agentics-dispatch-ci-"));
  t.after(() => {
    assert.equal(path.dirname(fs.realpathSync(cwd)), parent);
    fs.rmSync(cwd, { recursive: true });
  });
  fs.mkdirSync(path.join(cwd, "hooks/bin"), { recursive: true });
  for (const file of ["run-hook.cmd", "fetch-hook.ps1"])
    fs.copyFileSync(
      path.join(ROOT, "hooks", file),
      path.join(cwd, "hooks", file),
    );
  const binary = path.join(cwd, "hooks/bin/agx-windows-amd64.exe");
  fs.writeFileSync(binary, "placeholder hidden before dispatch");
  const fixture = "hooks/go/surfaces/claude-code/payloads/pre-tool-use.json";
  fs.mkdirSync(path.dirname(path.join(cwd, fixture)), { recursive: true });
  fs.writeFileSync(path.join(cwd, fixture), "{}\n");
  const run = parseWorkflow(readFile(CALLED_WORKFLOWS.manual)).jobs[
    "validate-windows-dispatch"
  ]!.steps.find(
    (step) => step.name === "missing binary fails closed for the guard",
  )!.run;
  const invoke = (body: string) => {
    const script = path.join(cwd, "step.ps1");
    fs.writeFileSync(
      script,
      "$ErrorActionPreference = 'Stop'\n" +
        body +
        "\nif (Test-Path -LiteralPath variable:\\LASTEXITCODE) { exit $LASTEXITCODE }\n",
    );
    return spawnSync(
      "pwsh",
      ["-NoProfile", "-NonInteractive", "-File", script],
      {
        cwd,
        env: {
          ...process.env,
          RUNNER_TEMP: cwd,
          CLAUDE_PLUGIN_ROOT: cwd,
          CLAUDE_PLUGIN_DATA: path.join(cwd, "data"),
          AGX_NO_FETCH: "1",
        },
        encoding: "utf8",
        timeout: 60000,
      },
    );
  };
  const result = invoke(run);
  assert.equal(result.status, 0, result.stdout + result.stderr);
  assert.match(result.stdout, /PASS: guard fails closed/);
  assert.ok(fs.existsSync(binary), "finally must restore the executable");
  const wrongExpectation = run.replace(
    "$LASTEXITCODE -ne 2",
    "$LASTEXITCODE -ne 0",
  );
  assert.notEqual(wrongExpectation, run);
  const rejected = invoke(wrongExpectation);
  assert.notEqual(
    rejected.status,
    0,
    "an incorrect guard status must still fail CI",
  );
  assert.match(rejected.stderr, /guard should fail closed/);
  assert.ok(
    fs.existsSync(binary),
    "finally must restore after an assertion failure",
  );
});

const RELEASE_INSTALL =
  "        run: npm ci --ignore-scripts --prefix .github/release\n";
function withoutComments(text: string): string {
  return text
    .split("\n")
    .filter((line) => !/^\s*#/.test(line))
    .join("\n");
}
function releaseWiringViolations(source: string): string[] {
  const violations = [];
  const suites = withoutComments(jobBlock(source, "ts-suite"));
  const install = suites.indexOf(RELEASE_INSTALL);
  const run = suites.indexOf("      - name: Test the TypeScript workspace\n");
  if (install === -1)
    violations.push("ts-suite must install the pinned release tooling");
  else if (run !== -1 && install > run)
    violations.push("the release tooling install must precede the suites");
  return violations;
}

test("the TypeScript suite installs the pinned release tooling", () => {
  assert.deepEqual(releaseWiringViolations(readWorkflow()), []);
});

for (const [name, mutate, expected] of [
  [
    "missing install",
    (source: string) =>
      replaceInJob(source, "ts-suite", RELEASE_INSTALL, "        run: true\n"),
    /must install the pinned release tooling/,
  ],
  [
    "late install",
    (source: string) =>
      replaceInJob(
        replaceInJob(
          source,
          "ts-suite",
          RELEASE_INSTALL,
          "        run: true\n",
        ),
        "ts-suite",
        `        run: ${TS_TEST_COMMAND}\n`,
        `        run: ${TS_TEST_COMMAND}\n\n      - name: Late install\n${RELEASE_INSTALL}`,
      ),
    /must precede the suites/,
  ],
  [
    "commented-out install",
    (source: string) =>
      replaceInJob(
        source,
        "ts-suite",
        RELEASE_INSTALL,
        `#${RELEASE_INSTALL}        run: true\n`,
      ),
    /must install the pinned release tooling/,
  ],
] as const) {
  test(`release install guard rejects ${name}`, () => {
    const source = readWorkflow(),
      changed = mutate(source);
    assert.notEqual(changed, source);
    assert.match(releaseWiringViolations(changed).join("\n"), expected);
  });
}

const MODULE_TESTS = `        run: ${HOOKS_TEST_COMMAND}\n`;
const TS_SUITE_COMMANDS = [`        run: ${TS_TEST_COMMAND}\n`];

// No conformance leg runs the hooks module tests, the tools node:test suite or the typecheck. Each is left out only
// because another job runs it on a pull request: go-suite runs the module tests whenever a change reaches the hooks
// module, and ts-suite runs the tools suite, the typecheck and the area boundaries, scoped by area.
function assertFastConformance(source: string): void {
  const conformance = withoutComments(jobBlock(source, "conformance"));
  assert.doesNotMatch(
    conformance,
    /plugin\.sh --validate|plugin\/cli\.ts --validate/,
    "conformance must run each rule area, not the maintainer entrypoint",
  );
  const suite = withoutComments(jobBlock(source, "go-suite"));
  const tests = stepBlock(suite, "Test the hooks module");
  assert.ok(
    tests !== null &&
      tests.includes("        if: matrix.check == 'tests'\n") &&
      tests.includes(MODULE_TESTS),
    "go-suite must run the hooks module tests that the hooks leg leaves out",
  );
  const suites = withoutComments(jobBlock(source, "ts-suite"));
  assert.ok(
    suites.includes(TS_WORKSPACE_MATRIX),
    "ts-suite must run the tools suite when the scope names none",
  );
  for (const command of TS_SUITE_COMMANDS) {
    assert.ok(suites.includes(command), `ts-suite must run: ${command.trim()}`);
  }
}

test("no conformance leg runs a suite that another job does not also run", () => {
  assertFastConformance(readWorkflow());
});

test("skill examples run even when scope omits the tools suite", () => {
  const job = (YAML.parse(readWorkflow()) as WorkflowDocument).jobs
    .conformance!;
  const example = job.steps.find(
    (step) =>
      step.run ===
      "node --test .github/tools/src/skill-examples/mcp-safe-write.test.ts",
  );
  assert.ok(example, "conformance must test the executable forge examples");
  assert.equal(job.if, undefined);
  assert.equal(example.if, "matrix.domain == 'skills'");
});

test("the fast conformance guard rejects each way of dropping a suite", () => {
  const source = readWorkflow();
  const cases = [
    [
      "a conformance leg that runs the maintainer entrypoint instead",
      replaceInJob(
        source,
        "conformance",
        "      - uses: $/.github/actions/conformance\n",
        "      - run: .github/scripts/plugin.sh --validate --fast\n\n      - uses: $/.github/actions/conformance\n",
      ),
      /must run each rule area, not the maintainer entrypoint/,
    ],
    [
      "a conformance leg that runs the maintainer CLI instead",
      replaceInJob(
        source,
        "conformance",
        "      - uses: $/.github/actions/conformance\n",
        "      - run: node .github/tools/src/plugin/cli.ts --validate --fast\n\n      - uses: $/.github/actions/conformance\n",
      ),
      /must run each rule area, not the maintainer entrypoint/,
    ],
    [
      "a go-suite without the module tests",
      replaceInJob(source, "go-suite", MODULE_TESTS, "        run: true\n"),
      /go-suite must run the hooks module tests/,
    ],
    [
      "module tests moved to the race leg",
      replaceInJob(
        source,
        "go-suite",
        "      - name: Test the hooks module\n        if: matrix.check == 'tests'\n",
        "      - name: Test the hooks module\n        if: matrix.check == 'race'\n",
      ),
      /go-suite must run the hooks module tests/,
    ],
    ...TS_SUITE_COMMANDS.map(
      (command) =>
        [
          `a ts-suite job without ${command.trim()}`,
          replaceInJob(source, "ts-suite", command, ""),
          new RegExp(
            `must run: ${command.trim().replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}`,
          ),
        ] as const,
    ),
    [
      "a ts-suite fallback that drops the tools suite",
      replaceInJob(source, "ts-suite", "|| '[\"ts-tools\"]'", "|| '[]'"),
      /must run the tools suite when the scope names none/,
    ],
  ] as const;
  for (const [name, mutated, expected] of cases) {
    assert.notEqual(mutated, source, `${name}: the mutation changed nothing`);
    assert.throws(() => assertFastConformance(mutated), expected, name);
  }
});

test("TypeScript coverage runs on push, schedule, and dispatch, never on a pull request", () => {
  const jobs = parseWorkflow(readWorkflow()).jobs;
  // The call job gates the event and the called job the paths; JavaScript reads a hyphen as minus.
  const condition =
    `\${{ (${jobs["coverage"]!.if.slice(3, -2)}) && (` +
    jobs["ts-coverage"]!.if.slice(3, -2).replace(
      "inputs.ts-coverage",
      "needs.scope.outputs.ts_coverage",
    ) +
    ") }}";
  const job = { if: condition };
  for (const [event, paths, cancelled, expected] of [
    ["pull_request", "", false, false],
    ["push", "true", false, true],
    ["push", "false", false, false],
    // A failed scope job leaves the output empty, and coverage runs.
    ["push", "", false, true],
    ["schedule", "", false, true],
    ["workflow_dispatch", "", false, true],
    ["push", "true", true, false],
  ] as const) {
    assert.equal(
      actionsExpression(
        job.if,
        ["github", "needs", "cancelled"],
        [
          { event_name: event },
          { scope: { outputs: { ts_coverage: paths } } },
          () => cancelled,
        ],
      ),
      expected,
      `${event}: paths=${paths}, cancelled=${cancelled}`,
    );
  }
});

test("the scope job passes the push to the coverage paths module", () => {
  const scope = parseWorkflow(readWorkflow()).jobs["scope"]!;
  assert.equal(
    (scope.outputs as Record<string, string>).ts_coverage,
    "${{ steps.ts-coverage.outputs.run }}",
  );
  assertCoveragePathsStep(readWorkflow());
});

test("negative: the coverage paths guard rejects a step that is not the module call", () => {
  const source = readWorkflow();
  const oldRun =
    "        run: |\n" +
    '          if ! git cat-file -e "${BEFORE}^{commit}" 2>/dev/null; then\n' +
    '            echo "run=true" >> "$GITHUB_OUTPUT"\n' +
    "          fi\n";
  const swap = (from: string, to: string) => source.replace(from, () => to);
  const cases = [
    ["the old multi-line run restored", swap(COVERAGE_PATHS_RUN, oldRun)],
    [
      "a changed module path",
      swap(COVERAGE_PATHS_RUN, "        run: node ci-coverage-paths.ts\n"),
    ],
    [
      "a dropped before input",
      swap("          BEFORE: ${{ github.event.before }}\n", ""),
    ],
    [
      "a dropped push condition",
      swap(
        "        id: ts-coverage\n        if: github.event_name == 'push'\n",
        "        id: ts-coverage\n",
      ),
    ],
  ] as const;
  for (const [name, mutated] of cases) {
    assert.notEqual(mutated, source, `${name}: the mutation changed nothing`);
    assert.throws(
      () => assertCoveragePathsStep(mutated),
      /Match the TypeScript coverage paths/,
      name,
    );
  }
});

test("the PR metadata jobs run only on pull requests and hold no wider token", () => {
  const jobs = parseWorkflow(readWorkflow()).jobs;
  assert.deepEqual(jobs["pr-metadata"]!.permissions, {
    "pull-requests": "read",
  });
  assert.deepEqual(jobs["pr-version-fields"]!.permissions, {
    contents: "read",
  });
  for (const id of ["pr-metadata", "pr-version-fields"]) {
    assert.equal(jobs[id]!.if, "github.event_name == 'pull_request'", id);
  }
  // Keep the standard PR events, including drafts, and validate metadata edits.
  const on = YAML.parse(readWorkflow()).on;
  assert.deepEqual(on.pull_request, {
    branches: ["main"],
    types: ["opened", "synchronize", "reopened", "edited"],
  });
});

test("each called workflow runs only when validate.yml calls it, with a read-only default token", () => {
  const jobs = parseWorkflow(fs.readFileSync(WORKFLOW, "utf8")).jobs;
  for (const [caller, name] of Object.entries(CALLED_WORKFLOWS)) {
    const called = YAML.parse(readFile(name));
    assert.deepEqual(Object.keys(called.on), ["workflow_call"], name);
    assert.deepEqual(called.permissions, { contents: "read" }, name);
    assert.equal(jobs[caller]?.uses, `./.github/workflows/${name}`, caller);
  }
});

const AGY_TARBALL_URL =
  "https://storage.googleapis.com/antigravity-public/antigravity-cli/1.5.13-5999302598852608/linux-x64/cli_linux_x64.tar.gz";
const AGY_TARBALL_SHA512 =
  "014b37387703acab1f47cd532c8bb60167a6a3537b0f9891de03f1b1ffa9614307bc83c4b2f7551273b0877d722716b0cd9e26782f6b2a50705db4f05e18c68f";
const AGY_INSTALL_RUN =
  "node .github/tools/src/scripts/ci-antigravity-native.ts install";
const AGY_CHECK_RUN =
  "node .github/tools/src/scripts/ci-antigravity-native.ts check";

function assertAntigravityNativeJob(source: string): void {
  const rawJob = jobBlock(source, "validate-antigravity-native");
  const activeJob = withoutComments(rawJob);
  // Only this job and validation-summary are parsed, so a mutation elsewhere reaches the guard that owns it.
  const job = YAML.parse(rawJob) as WorkflowJob;
  assert.ok(job, "validate-antigravity-native job is missing");
  assert.equal(
    job.name,
    "Check / Antigravity plugin",
    "validate-antigravity-native must use the expected display name",
  );
  assert.equal(
    job["runs-on"],
    "ubuntu-slim",
    "validate-antigravity-native must run on ubuntu-slim",
  );
  assert.deepEqual(
    job.permissions,
    { contents: "read" },
    "validate-antigravity-native must hold read-only contents permission",
  );
  const checkout = job.steps.findIndex((step) => step.uses === CHECKOUT);
  const setupNode = job.steps.findIndex((step) => step.uses === SETUP_NODE);
  const install = job.steps.findIndex(
    (step) => step.name === "Install pinned agy",
  );
  const validate = job.steps.findIndex(
    (step) => step.name === "Antigravity plugin validation",
  );
  assert.ok(
    checkout >= 0 &&
      setupNode > checkout &&
      install > setupNode &&
      validate > install,
    "validate-antigravity-native must pin checkout and setup-node before installing and running agy",
  );
  assert.equal(
    job.steps[checkout]!.with["persist-credentials"],
    false,
    "validate-antigravity-native checkout must disable persisted credentials",
  );
  assert.equal(
    job.steps[setupNode]!.with["node-version"],
    "26.12.1",
    "validate-antigravity-native must use Node 26.12.1",
  );
  assert.doesNotMatch(
    activeJob,
    /curl\b[^|\n]*\|\s*(?:\/\S+\/)?(?:ba)?sh\b/,
    "validate-antigravity-native must never pipe curl to sh or bash",
  );
  const installStep = job.steps[install]!;
  assert.equal(
    installStep.env?.AGY_URL,
    AGY_TARBALL_URL,
    "validate-antigravity-native must pin the agy 1.5.13 tarball URL",
  );
  assert.equal(
    installStep.env?.AGY_SHA512,
    AGY_TARBALL_SHA512,
    "validate-antigravity-native must pin the agy 1.5.13 SHA-512 checksum",
  );
  assert.equal(
    installStep.run.trim(),
    AGY_INSTALL_RUN,
    "validate-antigravity-native must install agy through ci-antigravity-native.ts install",
  );
  const validateStep = job.steps[validate]!;
  assert.equal(
    validateStep.run.trim(),
    AGY_CHECK_RUN,
    "validate-antigravity-native must validate through ci-antigravity-native.ts check",
  );
  const summaryJob = YAML.parse(jobBlock(source, SUMMARY_JOB)) as WorkflowJob;
  assert.ok(
    Array.isArray(summaryJob.needs) &&
      summaryJob.needs.includes("validate-antigravity-native"),
    "validation-summary must depend on validate-antigravity-native",
  );
}

test("negative: validate-antigravity-native guard rejects checksum, URL, curl-pipe, subcommand, runner, permission, and summary drift", () => {
  const source = readWorkflow();
  const cases: [string, string, string, string, RegExp][] = [
    [
      "altered SHA-512 checksum",
      "validate-antigravity-native",
      AGY_TARBALL_SHA512,
      "0".repeat(128),
      /pin the agy 1\.5\.13 SHA-512 checksum/,
    ],
    [
      "altered tarball URL",
      "validate-antigravity-native",
      AGY_TARBALL_URL,
      "https://example.invalid/agy.tar.gz",
      /pin the agy 1\.5\.13 tarball URL/,
    ],
    [
      "curl piped to bash",
      "validate-antigravity-native",
      `        run: ${AGY_INSTALL_RUN}\n`,
      '        run: curl -fsSL "$AGY_URL" | bash\n',
      /never pipe curl to sh or bash/,
    ],
    [
      "wrong install subcommand",
      "validate-antigravity-native",
      `        run: ${AGY_INSTALL_RUN}\n`,
      `        run: ${AGY_CHECK_RUN}\n`,
      /install agy through ci-antigravity-native\.ts install/,
    ],
    [
      "wrong validate subcommand",
      "validate-antigravity-native",
      `        run: ${AGY_CHECK_RUN}\n`,
      `        run: ${AGY_INSTALL_RUN}\n`,
      /validate through ci-antigravity-native\.ts check/,
    ],
    [
      "runner drift",
      "validate-antigravity-native",
      "    runs-on: ubuntu-slim\n",
      "    runs-on: ubuntu-latest\n",
      /must run on ubuntu-slim/,
    ],
    [
      "permissions drift",
      "validate-antigravity-native",
      "      contents: read\n",
      "      contents: write\n",
      /read-only contents permission/,
    ],
    [
      "dropped summary dependency",
      SUMMARY_JOB,
      "        validate-antigravity-native,\n",
      "",
      /validation-summary must depend on validate-antigravity-native/,
    ],
  ];
  for (const [label, jobId, from, to, expected] of cases) {
    assert.throws(
      () => assertAntigravityNativeJob(replaceInJob(source, jobId, from, to)),
      expected,
      label,
    );
  }
});

// Jobs that run repository modules without a Node setup of their own. Each
// needs Node 26.12.1 for the TypeScript modules the later steps start.
const MODULE_NODE_JOBS = [
  "pr-version-fields",
  "validate-antigravity-native",
  "go-suite",
  "shellcheck",
  "gh-aw-lock-drift",
  "validate-windows-dispatch",
  "native-hook-smoke",
] as const;
const MODULE_NODE_VERSION = "26.12.1";
const NATIVE_NODE_ARCHITECTURE =
  "${{ matrix.os == 'windows-11-arm' && 'arm64' || '' }}";
function moduleNodeStep(id: string): string {
  const architecture =
    id === "native-hook-smoke"
      ? `          architecture: ${NATIVE_NODE_ARCHITECTURE}\n`
      : "";
  return `      - uses: ${SETUP_NODE}\n        with:\n          node-version: ${MODULE_NODE_VERSION}\n${architecture}\n`;
}

// The setup step follows checkout, precedes the job's first run, and pins the
// version. Unnamed, like the other setup-node steps, so named-step guards hold.
function assertModuleNodeSetup(source: string): void {
  for (const id of MODULE_NODE_JOBS) {
    const job = jobBlock(source, id);
    const setup = job.indexOf(`uses: ${SETUP_NODE}`);
    assert.notEqual(setup, -1, `${id} must set up Node with the pinned action`);
    const checkout = job.indexOf("uses: actions/checkout@");
    assert.ok(
      checkout >= 0 && checkout < setup,
      `${id} must set up Node after checkout`,
    );
    const firstRun = job.search(/^ {8}run:/m);
    assert.ok(
      firstRun < 0 || setup < firstRun,
      `${id} must set up Node before its first run:`,
    );
    const step = job.slice(setup).split("\n\n", 1)[0]!;
    assert.ok(
      step.includes(`node-version: ${MODULE_NODE_VERSION}`),
      `${id} must pin Node ${MODULE_NODE_VERSION}`,
    );
  }
}

function replaceModuleJob(
  source: string,
  id: string,
  edit: (job: string) => string,
): string {
  const job = jobBlock(source, id);
  assert.equal(
    job.split(moduleNodeStep(id)).length - 1,
    1,
    `${id} must carry exactly one module Node setup step`,
  );
  return source.replace(job, () => edit(job));
}

test("module jobs set up Node 26.12.1 after checkout and before their first run", () => {
  const source = readWorkflow();
  assertModuleNodeSetup(source);
  for (const id of MODULE_NODE_JOBS) {
    const removed = replaceModuleJob(source, id, (job) =>
      job.replace(moduleNodeStep(id), ""),
    );
    assert.throws(
      () => assertModuleNodeSetup(removed),
      new RegExp(`${id} must set up Node with the pinned action`),
      `removing the setup step from ${id} must fail`,
    );
    const moved = replaceModuleJob(source, id, (job) => {
      const without = job.replace(moduleNodeStep(id), "");
      const run = without.search(/^ {8}run:/m);
      const next = without.indexOf("\n      - ", run);
      const at = next < 0 ? without.length : next + 1;
      return without.slice(0, at) + moduleNodeStep(id) + without.slice(at);
    });
    assert.notEqual(moved, source, `${id} move fixture must change the job`);
    assert.throws(
      () => assertModuleNodeSetup(moved),
      new RegExp(`${id} must set up Node before its first run`),
      `moving the setup step after the first run in ${id} must fail`,
    );
  }
});

for (const [name, exclude, message] of [
  ["plugin.sh", CORPUS_EXCLUDE, /frozen corpus plugin\.sh copy/],
  [
    "verify-stage.sh",
    CORPUS_STAGE_EXCLUDE,
    /frozen corpus verify-stage\.sh copy/,
  ],
] as const) {
  test(`the shellcheck guard rejects a glob pass that lints the corpus ${name}`, () => {
    const source = readWorkflow();
    assertShellcheckCorpusExclusion(source);
    assert.throws(
      () =>
        assertShellcheckCorpusExclusion(
          source,
          SHELLCHECK_PATHSPECS.filter((spec) => spec !== exclude),
        ),
      message,
    );
  });
}

test("the shellcheck guard rejects a corpus wildcard exclusion", () => {
  assert.throws(
    () =>
      assertShellcheckCorpusExclusion(readWorkflow(), [
        ...SHELLCHECK_PATHSPECS,
        ":!:hooks/go/cmd/hook/testdata/corpus/scripts/*",
      ]),
    /must not exclude the other frozen corpus scripts/,
  );
});

test("negative: the shellcheck guard rejects the old scripts, a wrong subcommand and an unpinned install", () => {
  const source = readWorkflow();
  const cases = [
    [
      "the old install script restored",
      SHELLCHECK_INSTALL_RUN,
      '        run: |\n          cd "$RUNNER_TEMP"\n',
      /installed from the pinned release/,
    ],
    [
      "the old check script restored",
      SHELLCHECK_CHECK_RUN,
      "        run: |\n          shellcheck --version\n",
      /Check shell scripts step must run the module/,
    ],
    [
      "a wrong check subcommand",
      SHELLCHECK_CHECK_RUN,
      SHELLCHECK_INSTALL_RUN,
      /Check shell scripts step must run the module/,
    ],
    [
      "a dropped pinned version",
      "          SHELLCHECK_VERSION: v0.11.0\n",
      "",
      /installed from the pinned release/,
    ],
  ] as const;
  for (const [name, from, to, expected] of cases) {
    const mutated = replaceInJob(source, "shellcheck", from, to);
    assert.notEqual(mutated, source, `${name}: the mutation changed nothing`);
    assert.throws(
      () => assertShellcheckCorpusExclusion(mutated),
      expected,
      name,
    );
  }
});

// The run: blocks that stay inline, keyed by file, job and step (never by line, which moves). A multi-line block is LOGIC
// and belongs in a ci-*.ts module unless it is one of these: the conformance auditor check, the launcher tests, the
// short plain sequences, and the blocks in jobs that check out no repository code.
const INLINE_BLOCKS = [
  [".github/actions/conformance/action.yml", ACTION_JOB, "Check the auditor"],
  [".github/actions/release/action.yml", ACTION_JOB, "Build the hook verifier"],
  [
    ".github/workflows/validate-coverage.yml",
    "coverage-report",
    "Convert profiles to Cobertura XML",
  ],
  [
    ".github/workflows/validate-coverage.yml",
    "coverage-upload",
    "Select supported coverage upload context",
  ],
  [
    ".github/workflows/validate-coverage.yml",
    "ts-coverage-upload",
    "Select supported coverage upload context",
  ],
  [
    ".github/workflows/validate.yml",
    "pr-metadata",
    "Fetch current pull request metadata",
  ],
  [".github/workflows/validate.yml", "pr-metadata", "Check the PR title"],
  [
    ".github/workflows/validate.yml",
    "pr-metadata",
    "Check the release markers",
  ],
  [
    ".github/workflows/validate.yml",
    "pr-metadata",
    "Check the change description",
  ],
  [
    ".github/workflows/validate.yml",
    "go-suite",
    "Build hook binaries and check reproducibility",
  ],
  [".github/workflows/validate.yml", "format", "Check formatting"],
  ...[
    "Build the hook binaries from source",
    "cmd dispatches the built binary and denies",
    "benign payload passes silently through the binary",
    "missing binary fails closed for the guard",
    "cmd fetches a missing binary through fetch-hook.ps1",
  ].map(
    (step) =>
      [
        ".github/workflows/validate-manual.yml",
        "validate-windows-dispatch",
        step,
      ] as const,
  ),
  ...[
    "Run the native binary directly (macOS)",
    "Run the native binary directly (Windows)",
    "command-guardian dispatch smoke (macOS)",
    "command-guardian dispatch smoke (Windows)",
  ].map(
    (step) =>
      [
        ".github/workflows/validate-manual.yml",
        "native-hook-smoke",
        step,
      ] as const,
  ),
] as const;

// The steps whose block moved into a module: each is now one line that starts the module, from the checkout that
// supplies the action's own code in the two composite actions.
const MODULE_CALL = "node .github/tools/src/scripts/";
const AUDITOR_CALL =
  'node "$AGX_CONFORMANCE_AUDITOR/.github/tools/src/scripts/ci-conformance.ts"';
const MOVED_STEPS = [
  [
    ".github/workflows/validate.yml",
    "scope",
    "Match the TypeScript coverage paths",
    `${MODULE_CALL}ci-coverage-paths.ts`,
  ],
  [
    ".github/workflows/validate.yml",
    "pr-version-fields",
    "Reject a changed version field",
    `${MODULE_CALL}ci-version-fields.ts`,
  ],
  [
    ".github/workflows/validate.yml",
    "validate-antigravity-native",
    "Install pinned agy",
    `${MODULE_CALL}ci-antigravity-native.ts install`,
  ],
  [
    ".github/workflows/validate.yml",
    "validate-antigravity-native",
    "Antigravity plugin validation",
    `${MODULE_CALL}ci-antigravity-native.ts check`,
  ],
  [
    ".github/workflows/validate.yml",
    "go-suite",
    "Test the hooks module",
    `${MODULE_CALL}ci-hooks-test.ts test`,
  ],
  [
    ".github/workflows/validate.yml",
    "go-suite",
    "Enforce the 90% coverage floor",
    `${MODULE_CALL}ci-hooks-test.ts floor`,
  ],
  [
    ".github/workflows/validate.yml",
    "ts-suite",
    "Test the TypeScript workspace",
    `${MODULE_CALL}ci-ts-test.ts`,
  ],
  [
    ".github/workflows/validate.yml",
    "shellcheck",
    "Install pinned shellcheck",
    `${MODULE_CALL}ci-shellcheck.ts install`,
  ],
  [
    ".github/workflows/validate.yml",
    "shellcheck",
    "Check shell scripts",
    `${MODULE_CALL}ci-shellcheck.ts check`,
  ],
  [
    ".github/workflows/validate.yml",
    "gh-aw-lock-drift",
    "Install pinned gh-aw",
    `${MODULE_CALL}ci-gh-aw.ts install`,
  ],
  [
    ".github/workflows/validate.yml",
    "gh-aw-lock-drift",
    "Recompile and diff the locks",
    `${MODULE_CALL}ci-gh-aw.ts check`,
  ],
  [
    ".github/workflows/validate-coverage.yml",
    "coverage-report",
    "Require every shard profile",
    `${MODULE_CALL}ci-coverage.ts require-profiles`,
  ],
  [
    ".github/workflows/validate-coverage.yml",
    "coverage-report",
    "Merge shard profiles per module",
    `${MODULE_CALL}ci-coverage.ts merge-profiles`,
  ],
  [
    ".github/workflows/validate-coverage.yml",
    "coverage-report",
    "Report module coverage totals",
    `${MODULE_CALL}ci-coverage.ts totals`,
  ],
  [
    ".github/workflows/validate-coverage.yml",
    "ts-coverage-report",
    "Combine complete TypeScript coverage",
    `${MODULE_CALL}ci-coverage.ts combine-ts`,
  ],
  [
    ".github/workflows/validate-manual.yml",
    "native-hook-smoke",
    "Build the hook binaries from source",
    `${MODULE_CALL}ci-native-smoke.ts build`,
  ],
  [
    ".github/workflows/validate-manual.yml",
    "native-hook-smoke",
    "Confirm the runner is the target architecture (macOS)",
    `${MODULE_CALL}ci-native-smoke.ts arch macos`,
  ],
  [
    ".github/workflows/validate-manual.yml",
    "native-hook-smoke",
    "Confirm the runner is the target architecture (Windows)",
    `${MODULE_CALL}ci-native-smoke.ts arch windows`,
  ],
  [
    ".github/workflows/validate-mutation.yml",
    "mutation-full",
    "Mutate hooks/go packages",
    `${MODULE_CALL}ci-mutation-hooks.ts`,
  ],
  [
    ".github/workflows/reminders.yml",
    "check-ledger-staleness",
    "Open or update tracking issue",
    `${MODULE_CALL}ci-reminders.ts`,
  ],
  [
    ".github/actions/conformance/action.yml",
    ACTION_JOB,
    "Install the auditor's dependencies",
    `${AUDITOR_CALL} install`,
  ],
  [
    ".github/actions/conformance/action.yml",
    ACTION_JOB,
    "Run conformance",
    `${AUDITOR_CALL} run`,
  ],
  [
    ".github/actions/release/action.yml",
    ACTION_JOB,
    "Run",
    'node "$CHECKOUT/.github/tools/src/scripts/ci-release-run.ts"',
  ],
] as const;

interface RunStep {
  file: string;
  job: string;
  step: string;
  multiline: boolean;
  run: string;
}

function stepKey(file: string, job: string, step: string): string {
  return `${file} job ${job} step "${step}"`;
}

// A run: is multi-line when its value spans more than one source line: any block scalar, even one holding a single
// command, or a flow scalar continued on the next line.
function runSteps(sources: Map<string, string>): RunStep[] {
  const steps: RunStep[] = [];
  for (const [file, text] of sources) {
    const doc = YAML.parseDocument(text);
    const jobs = doc.get("jobs");
    const lists: [string, unknown][] = YAML.isMap(jobs)
      ? jobs.items.map((pair) => [
          String(YAML.isScalar(pair.key) ? pair.key.value : pair.key),
          YAML.isMap(pair.value) ? pair.value.get("steps") : undefined,
        ])
      : [[ACTION_JOB, doc.getIn(["runs", "steps"])]];
    for (const [job, list] of lists) {
      if (!YAML.isSeq(list)) continue;
      list.items.forEach((item, index) => {
        if (!YAML.isMap(item)) return;
        const run = item.get("run", true);
        if (!YAML.isScalar(run)) return;
        const [start, end] = run.range ?? [0, 0];
        steps.push({
          file,
          job,
          step: String(item.get("name") ?? item.get("id") ?? `#${index + 1}`),
          multiline:
            run.type === "BLOCK_LITERAL" ||
            run.type === "BLOCK_FOLDED" ||
            text.slice(start, end).trimEnd().includes("\n"),
          run: String(run.value),
        });
      });
    }
  }
  return steps;
}

function assertRunInventory(steps: RunStep[]): void {
  const problems: string[] = [];
  const inline = new Set(INLINE_BLOCKS.map(([f, j, s]) => stepKey(f, j, s)));
  const multiline = new Set<string>();
  for (const step of steps) {
    if (!step.multiline) continue;
    const key = stepKey(step.file, step.job, step.step);
    multiline.add(key);
    if (!inline.has(key))
      problems.push(
        `new multi-line run: block in ${key}: move it into a ci-*.ts module, or list it as inline when it is not LOGIC`,
      );
  }
  for (const key of inline)
    if (!multiline.has(key))
      problems.push(
        `stale inline entry: ${key} no longer has a multi-line run: block`,
      );
  for (const [file, job, name, command] of MOVED_STEPS) {
    const step = steps.find(
      (s) => s.file === file && s.job === job && s.step === name,
    );
    const run = step?.run.trim() ?? "";
    if (
      step === undefined ||
      step.multiline ||
      !(run === command || run.startsWith(command + " "))
    )
      problems.push(
        `moved step ${stepKey(file, job, name)} must be a one-line run: that calls ${command}`,
      );
  }
  if (problems.length > 0) throw new Error(problems.join("\n"));
}

test("exactly the 20 inline run: blocks stay multi-line and the 23 moved steps call their modules", () => {
  assert.equal(INLINE_BLOCKS.length, 20);
  assert.equal(MOVED_STEPS.length, 23);
  const modules = new Set(
    MOVED_STEPS.map(([, , , command]) => /ci-[a-z-]+\.ts/.exec(command)![0]),
  );
  assert.equal(
    modules.size,
    13,
    "the moved blocks live in thirteen ci-*.ts modules",
  );
  const steps = runSteps(workflowSources(ROOT));
  assertRunInventory(steps);
  const counts: Record<string, number> = {};
  for (const step of steps.filter((s) => s.multiline))
    counts[step.file] = (counts[step.file] ?? 0) + 1;
  assert.deepEqual(counts, {
    ".github/actions/conformance/action.yml": 1,
    ".github/actions/release/action.yml": 1,
    ".github/workflows/validate-coverage.yml": 3,
    ".github/workflows/validate-manual.yml": 9,
    ".github/workflows/validate.yml": 6,
  });
});

test("negative: the inventory names each new, stale or regrown multi-line run: block", (t) => {
  const sources = workflowSources(ROOT);
  const edited = (file: string, from: string, to: string) => {
    const text = sources.get(file)!;
    assert.equal(
      text.split(from).length - 1,
      1,
      `${file} fixture target must occur once`,
    );
    return runSteps(new Map(sources).set(file, text.replace(from, to)));
  };
  const cases = [
    [
      "a new block in a workflow",
      edited(
        ".github/workflows/validate.yml",
        "      - name: Check formatting\n",
        '      - name: Loop over the files\n        run: |\n          for f in *; do echo "$f"; done\n\n      - name: Check formatting\n',
      ),
      /new multi-line run: block in \.github\/workflows\/validate\.yml job format step "Loop over the files"/,
    ],
    [
      "a new block in an action",
      edited(
        ".github/actions/conformance/action.yml",
        "    - name: Install the auditor's dependencies\n",
        "    - name: Extra action logic\n      shell: bash\n      run: |\n        echo one\n        echo two\n    - name: Install the auditor's dependencies\n",
      ),
      /new multi-line run: block in \.github\/actions\/conformance\/action\.yml job runs step "Extra action logic"/,
    ],
    [
      "a removed inline block",
      edited(
        ".github/workflows/validate-coverage.yml",
        "        run: |\n          node .github/tools/src/coverage/cli.ts goprofile",
        "        run: node .github/tools/src/coverage/cli.ts goprofile",
      ),
      /stale inline entry: \.github\/workflows\/validate-coverage\.yml job coverage-report step "Convert profiles to Cobertura XML"/,
    ],
    [
      "a moved step given a multi-line run:",
      edited(
        ".github/workflows/validate.yml",
        "        run: node .github/tools/src/scripts/ci-hooks-test.ts floor\n",
        "        run: |\n          node .github/tools/src/scripts/ci-hooks-test.ts floor\n",
      ),
      /moved step \.github\/workflows\/validate\.yml job go-suite step "Enforce the 90% coverage floor" must be a one-line run:/,
    ],
    [
      "a moved step that stops calling its module from its own checkout",
      edited(
        ".github/actions/release/action.yml",
        'run: node "$CHECKOUT/.github/tools/src/scripts/ci-release-run.ts"',
        'run: node "$GITHUB_WORKSPACE/.github/tools/src/scripts/ci-release-run.ts"',
      ),
      /moved step \.github\/actions\/release\/action\.yml job runs step "Run" must be a one-line run:/,
    ],
  ] as const;
  for (const [name, steps, expected] of cases)
    assert.throws(() => assertRunInventory(steps), expected, name);

  const root = fs.mkdtempSync(
    path.join(os.tmpdir(), "agentics-run-inventory-"),
  );
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  for (const dir of [".github/workflows", ".github/actions"])
    fs.cpSync(path.join(ROOT, dir), path.join(root, dir), { recursive: true });
  const extra =
    "jobs:\n  extra:\n    runs-on: ubuntu-latest\n    steps:\n      - name: Extra logic\n        run: |\n          echo one\n          echo two\n";
  fs.writeFileSync(path.join(root, ".github/workflows/extra.yml"), extra);
  fs.writeFileSync(
    path.join(root, ".github/workflows/extra.lock.yml"),
    extra.replace("Extra logic", "Compiled logic"),
  );
  assert.throws(
    () => assertRunInventory(runSteps(workflowSources(root))),
    (error: Error) =>
      /new multi-line run: block in \.github\/workflows\/extra\.yml job extra step "Extra logic"/.test(
        error.message,
      ) && !error.message.includes("Compiled logic"),
    "a new workflow file is found by listing the directory; a .lock.yml is not",
  );
});

// The interpreters each CI leg must have: a test whose interpreter is listed fails instead of skipping when it is
// missing. The macOS image has no pwsh.
const INTERPRETER_LEGS = [
  ["validate.yml", "ts-suite", { os: "ubuntu-latest" }, "sh,bash,pwsh"],
  ["validate.yml", "ts-suite", { os: "windows-latest" }, "sh,bash,pwsh"],
  ["validate.yml", "ts-suite", { os: "xcode-27" }, "sh,bash"],
  ...(
    [
      ["ubuntu-latest", "tests"],
      ["ubuntu-latest", "race"],
      ["ubuntu-latest", "fuzz"],
      ["xcode-27", "tests"],
      ["windows-latest", "tests"],
    ] as const
  ).map(
    ([os, check]) =>
      ["validate.yml", "go-suite", { os, check }, "sh,bash"] as const,
  ),
  [
    "validate-coverage.yml",
    "ts-coverage",
    { workspace: "tools", shard: 1 },
    "sh,bash,pwsh",
  ],
  [
    "validate-mutation.yml",
    "mutation-full",
    { tool: "gremlins", target: "rest" },
    "sh,bash,pwsh",
  ],
  [
    "validate-mutation.yml",
    "mutation-full",
    { tool: "stryker", target: "tools-rest" },
    "sh,bash,pwsh",
  ],
  ["validate-manual.yml", "validate-windows-dispatch", {}, "powershell"],
] as const;

type InterpreterDocs = Record<string, WorkflowDocument>;

function readInterpreterDocs(): InterpreterDocs {
  return Object.fromEntries(
    [...new Set(INTERPRETER_LEGS.map(([file]) => file))].map((file) => [
      file,
      parseWorkflow(readFile(file)),
    ]),
  );
}

function assertInterpreterLists(docs: InterpreterDocs): void {
  for (const [file, id, matrix, expected] of INTERPRETER_LEGS) {
    const value = docs[file]!.jobs[id]?.env?.AGX_REQUIRED_INTERPRETERS;
    assert.ok(
      typeof value === "string",
      `${id} must set AGX_REQUIRED_INTERPRETERS in its job env`,
    );
    const actual = value.startsWith("${{")
      ? actionsExpression(value, ["matrix"], [matrix])
      : value;
    assert.equal(
      actual,
      expected,
      `${id} leg ${JSON.stringify(matrix)} must require ${expected}`,
    );
    // A step-level value would replace the job's list for that step alone.
    for (const step of docs[file]!.jobs[id]!.steps)
      assert.ok(
        step.env?.AGX_REQUIRED_INTERPRETERS === undefined,
        `${id} step "${step.name ?? step.uses}" must not override AGX_REQUIRED_INTERPRETERS`,
      );
  }
}

test("each CI leg requires the interpreters its runner has", () => {
  assertInterpreterLists(readInterpreterDocs());
});

test("negative: the interpreter guard rejects a dropped or changed list, naming the job", () => {
  const jobs = [
    ...new Set(INTERPRETER_LEGS.map(([file, id]) => `${file}:${id}`)),
  ];
  assert.equal(jobs.length, 5);
  for (const entry of jobs) {
    const [file, id] = entry.split(":") as [string, string];
    for (const [change, value, expected] of [
      [
        "dropped",
        undefined,
        new RegExp(`: ${id} must set AGX_REQUIRED_INTERPRETERS`),
      ],
      ["narrowed to sh", "sh", new RegExp(`: ${id} leg .* must require`)],
      [
        "given a step-level list only",
        "step",
        new RegExp(`: ${id} must set AGX_REQUIRED_INTERPRETERS`),
      ],
    ] as const) {
      const docs = readInterpreterDocs();
      const job = docs[file]!.jobs[id]!;
      const original = job.env.AGX_REQUIRED_INTERPRETERS!;
      delete job.env.AGX_REQUIRED_INTERPRETERS;
      if (value === "sh") job.env.AGX_REQUIRED_INTERPRETERS = "sh";
      if (value === "step")
        job.steps.at(-1)!.env = { AGX_REQUIRED_INTERPRETERS: original };
      assert.throws(
        () => assertInterpreterLists(docs),
        expected,
        `${id}: ${change}`,
      );
    }
    // The job list stays; a test step overrides it for itself.
    for (const [change, value] of [
      ["narrowed", "sh"],
      ["emptied", ""],
    ] as const) {
      const docs = readInterpreterDocs();
      const steps = docs[file]!.jobs[id]!.steps;
      const step = steps.find((s) => /\bnode\b/.test(s.run ?? ""))!;
      step.env = { ...step.env, AGX_REQUIRED_INTERPRETERS: value };
      assert.throws(
        () => assertInterpreterLists(docs),
        new RegExp(
          `: ${id} step "${step.name}" must not override AGX_REQUIRED_INTERPRETERS`,
        ),
        `${id}: a test step ${change} the list`,
      );
    }
  }
  const docs = readInterpreterDocs();
  docs["validate.yml"]!.jobs["ts-suite"]!.env.AGX_REQUIRED_INTERPRETERS =
    "sh,bash,pwsh";
  assert.throws(
    () => assertInterpreterLists(docs),
    /: ts-suite leg \{"os":"xcode-27"\} must require sh,bash\n/,
    "the macOS leg must not require pwsh",
  );
});

// The twin tables spawn the PowerShell that AGX_TWIN_POWERSHELL names; a step's shell: picks only the shell that
// starts node. The run is one line, so it needs no module, and it follows the hook proof so a twin failure cannot
// mask it.
const WINDOWS_TWIN_STEP = "Twin tables under Windows PowerShell 5.1";
const WINDOWS_TWIN_TESTS = [
  ".github/tools/src/entrypoint/polluter.test.ts",
  ".github/tools/src/scripts/scaffold-protection.test.ts",
];

function assertWindowsTwinStep(doc: WorkflowDocument): void {
  const steps = doc.jobs["validate-windows-dispatch"]!.steps;
  const index = steps.findIndex((step) => step.name === WINDOWS_TWIN_STEP);
  assert.ok(
    index >= 0,
    `validate-windows-dispatch must run the twin tables in a step named "${WINDOWS_TWIN_STEP}"`,
  );
  const step = steps[index]!;
  assert.ok(
    /^node --test( \S+)+$/.test(step.run),
    "the twin step must be a one-line node --test run",
  );
  const files = step.run.split(" ").slice(2);
  for (const file of WINDOWS_TWIN_TESTS)
    assert.ok(files.includes(file), `the twin step must run ${file}`);
  assert.equal(
    step.env?.AGX_TWIN_POWERSHELL,
    "powershell",
    "the twin step must set AGX_TWIN_POWERSHELL to powershell",
  );
  for (const name of WINDOWS_HOOK_STEPS)
    assert.ok(
      steps.findIndex((s) => s.name === name) < index,
      `the twin step must follow the hook proof step "${name}"`,
    );
}

test("the Windows dispatch job runs the twin tables under Windows PowerShell 5.1", () => {
  assertWindowsTwinStep(parseWorkflow(readFile("validate-manual.yml")));
});

test("negative: the twin step guard rejects a dropped step, test file or interpreter", () => {
  const source = readFile("validate-manual.yml");
  const twinStep = (doc: WorkflowDocument) =>
    doc.jobs["validate-windows-dispatch"]!.steps.find(
      (step) => step.name === WINDOWS_TWIN_STEP,
    )!;
  type Mutation = [string, (doc: WorkflowDocument) => void, RegExp];
  const cases: Mutation[] = [
    [
      "step dropped",
      (doc) => {
        const job = doc.jobs["validate-windows-dispatch"]!;
        const twin = twinStep(doc);
        job.steps = job.steps.filter((step) => step !== twin);
      },
      /must run the twin tables in a step named/,
    ],
    [
      "step renamed",
      (doc) => {
        twinStep(doc).name = "Twin tables";
      },
      /must run the twin tables in a step named/,
    ],
    ...WINDOWS_TWIN_TESTS.map((file): Mutation => [
      `${file} dropped`,
      (doc) => {
        const step = twinStep(doc);
        step.run = step.run.replace(` ${file}`, "");
      },
      new RegExp(`must run ${file.replaceAll(".", "\\.")}`),
    ]),
    [
      "interpreter dropped",
      (doc) => {
        delete twinStep(doc).env.AGX_TWIN_POWERSHELL;
      },
      /must set AGX_TWIN_POWERSHELL to powershell/,
    ],
    [
      "interpreter changed to pwsh",
      (doc) => {
        twinStep(doc).env.AGX_TWIN_POWERSHELL = "pwsh";
      },
      /must set AGX_TWIN_POWERSHELL to powershell/,
    ],
    [
      "run made multi-line",
      (doc) => {
        twinStep(doc).run += "\necho done";
      },
      /must be a one-line node --test run/,
    ],
    [
      "step moved before the hook proof",
      (doc) => {
        const job = doc.jobs["validate-windows-dispatch"]!;
        const twin = twinStep(doc);
        job.steps = [twin, ...job.steps.filter((step) => step !== twin)];
      },
      /must follow the hook proof step/,
    ],
  ];
  for (const [name, mutate, expected] of cases) {
    const doc = parseWorkflow(source);
    mutate(doc);
    assert.throws(() => assertWindowsTwinStep(doc), expected, name);
  }
});

// The Windows architecture check reads its own Node process, so it proves the runner only when that Node is ARM64.
function assertNativeNodeArchitecture(doc: WorkflowDocument): void {
  const job = doc.jobs["native-hook-smoke"]!;
  assert.deepEqual(job.strategy.matrix.os, [
    "macos-15-intel",
    "windows-11-arm",
  ]);
  const setup = job.steps.find((step) => step.uses?.startsWith(SETUP_NODE));
  const value = setup?.with.architecture;
  assert.ok(
    typeof value === "string",
    "native-hook-smoke must choose the Node architecture for each leg",
  );
  for (const [os, architecture] of [
    ["macos-15-intel", ""],
    ["windows-11-arm", "arm64"],
  ] as const)
    assert.equal(
      actionsExpression(value, ["matrix"], [{ os }]),
      architecture,
      `native-hook-smoke must install ${architecture || "the runner's own"} Node on ${os}`,
    );
}

test("the Windows native smoke leg runs its architecture check on ARM64 Node", () => {
  const source = readFile("validate-manual.yml");
  assertNativeNodeArchitecture(parseWorkflow(source));
  assert.ok(source.includes(`architecture: ${NATIVE_NODE_ARCHITECTURE}\n`));
  for (const [name, replacement, expected] of [
    ["dropped", "", /must choose the Node architecture/],
    [
      "x64",
      "          architecture: ${{ 'x64' }}\n",
      /must install arm64 Node on windows-11-arm|must install the runner's own Node on macos-15-intel/,
    ],
  ] as const) {
    const mutated = source.replace(
      `          architecture: ${NATIVE_NODE_ARCHITECTURE}\n`,
      replacement,
    );
    assert.notEqual(mutated, source, name);
    assert.throws(
      () => assertNativeNodeArchitecture(parseWorkflow(mutated)),
      expected,
      name,
    );
  }
});

// The inventory and the cold-start list key steps by file, job and name, so a repeated name would hide a step.
function assertUniqueStepNames(sources: Map<string, string>): void {
  const docs = new Map(
    [...sources].map(([file, text]) => [file, YAML.parse(text) as unknown]),
  );
  const duplicates = duplicateStepNames(jobSteps(docs));
  assert.deepEqual(
    duplicates,
    [],
    `step names repeat within a job:\n${duplicates.join("\n")}`,
  );
}

test("step names are unique within each job", () => {
  assertUniqueStepNames(workflowSources(ROOT));
});

test("negative: a repeated step name within a job is rejected naming it", () => {
  const sources = workflowSources(ROOT);
  const file = ".github/workflows/validate.yml";
  const text = sources.get(file)!;
  const from = "      - name: Check formatting\n";
  assert.equal(text.split(from).length - 1, 1);
  assert.throws(
    () =>
      assertUniqueStepNames(
        new Map(sources).set(
          file,
          text.replace(
            from,
            "      - name: Check formatting\n        run: echo again\n\n" +
              from,
          ),
        ),
      ),
    /\.github\/workflows\/validate\.yml job format step "Check formatting"/,
  );
  const action = ".github/actions/release/action.yml";
  const actionText = sources.get(action)!;
  const run = "    - name: Run\n";
  assert.equal(actionText.split(run).length - 1, 1);
  assert.throws(
    () =>
      assertUniqueStepNames(
        new Map(sources).set(
          action,
          actionText.replace(
            run,
            "    - name: Run\n      shell: bash\n      run: echo again\n" + run,
          ),
        ),
      ),
    /\.github\/actions\/release\/action\.yml job runs step "Run"/,
  );
});
