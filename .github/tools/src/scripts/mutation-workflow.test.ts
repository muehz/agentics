interface Step {
  name?: string;
  uses?: string;
  run?: string;
  if?: string;
  id?: string;
  with?: Record<string, string | number | boolean>;
  env?: Record<string, string>;
}
interface Entry {
  tool: string;
  target: string;
}
interface Job {
  steps: Step[];
  if: string;
  env?: Record<string, string>;
  permissions?: unknown;
  needs?: string[];
  "timeout-minutes"?: number;
  strategy?: { "fail-fast": boolean; matrix: { include: Entry[] } };
}
interface Workflow {
  permissions: unknown;
  on: Record<string, { inputs: { fresh: { type: string } } }>;
  jobs: Record<string, Job>;
}
function readWorkflow(file = FILE): Workflow {
  return YAML.parse(fs.readFileSync(file, "utf8"));
}
import { areaSources } from "./mutation-stryker.ts";
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import test from "node:test";
import YAML from "yaml";

const FILE = path.resolve(
  import.meta.dirname,
  "../../../../.github/workflows/validate-mutation.yml",
);
const CALLER = path.resolve(
  import.meta.dirname,
  "../../../../.github/workflows/validate.yml",
);
interface CallerJob {
  if: string;
  uses: string;
  with: Record<string, string>;
  permissions?: unknown;
}
const MUTATION_JOBS = ["mutation-full", "mutation-summary"];

test("the mutation jobs are read-only, secret-free, and pinned", () => {
  const workflow = readWorkflow();
  assert.deepEqual(workflow.permissions, { contents: "read" });
  assert.deepEqual(Object.keys(workflow.on), ["workflow_call"]);
  const caller = (
    YAML.parse(fs.readFileSync(CALLER, "utf8")) as {
      jobs: Record<string, CallerJob>;
    }
  ).jobs["mutation"]!;
  assert.equal(
    caller.permissions,
    undefined,
    "the caller must not widen the token",
  );
  for (const id of MUTATION_JOBS) {
    const job = workflow.jobs[id]!;
    assert.equal(job.permissions, undefined, `${id} must not widen the token`);
    assert.doesNotMatch(YAML.stringify(job), /secrets\./, id);
    for (const step of job.steps) {
      if (step.uses === undefined) continue;
      assert.match(step.uses, /^[\w.-]+\/[\w.-]+@[0-9a-f]{40}$/, step.uses);
    }
  }
});

test("mutation Go caches use the existing hooks module definition", () => {
  const workflow = readWorkflow();
  for (const name of MUTATION_JOBS) {
    for (const step of workflow.jobs[name]!.steps) {
      if (!step.uses?.startsWith("actions/setup-go@")) continue;
      assert.equal(step.with!["go-version-file"], "hooks/go/go.mod", name);
      assert.equal(
        step.with!["cache-dependency-path"],
        "hooks/go/go.mod",
        name,
      );
      assert.ok(
        fs.existsSync(
          path.resolve(
            import.meta.dirname,
            "../../../..",
            step.with!["cache-dependency-path"],
          ),
        ),
        `${name} cache input must exist in the checkout`,
      );
    }
  }
});

test("gremlins is installed from one pinned version value", () => {
  const text = fs.readFileSync(FILE, "utf8");
  assert.equal(
    readWorkflow().jobs["mutation-full"]!.env?.GREMLINS_VERSION,
    "v0.6.0",
  );
  assert.equal(text.match(/GREMLINS_VERSION: /g)!.length, 1);
  assert.match(
    text,
    /go install "github\.com\/go-gremlins\/gremlins\/cmd\/gremlins@\$\{GREMLINS_VERSION\}"/,
  );
  const makefile = fs.readFileSync(
    path.join(import.meta.dirname, "../../../../hooks/go/Makefile"),
    "utf8",
  );
  assert.equal(makefile.match(/^GREMLINS_VERSION \?= (\S+)$/m)?.[1], "v0.6.0");
});

test("the mutation jobs start only by hand, and the full run has the 10 matrix entries and a 360-minute limit", () => {
  const workflow = readWorkflow();
  // A full run costs many runner hours, so no schedule, push, or pull request may start it.
  const caller = YAML.parse(fs.readFileSync(CALLER, "utf8")) as {
    on: Record<string, { inputs: Record<string, { type: string }> }>;
    jobs: Record<string, CallerJob>;
  };
  assert.deepEqual(caller.jobs["mutation"], {
    name: "Mutation",
    if: "github.event_name == 'workflow_dispatch'",
    uses: "./.github/workflows/validate-mutation.yml",
    with: { fresh: "${{ inputs.fresh }}" },
  });
  assert.equal(caller.on.workflow_dispatch!.inputs.fresh!.type, "boolean");
  const job = workflow.jobs["mutation-full"]!;
  assert.equal(job.if, undefined);
  assert.equal(job["timeout-minutes"], 360);
  assert.equal(job.strategy!["fail-fast"], false);
  const entries = job.strategy!.matrix.include.map(
    (e) => `${e.tool} ${e.target}`,
  );
  assert.deepEqual(entries, [
    "gremlins ./internal/shellwords",
    "gremlins ./internal/commandguard",
    "gremlins ./internal/secretsguard",
    "gremlins ./cmd/hook",
    "gremlins rest",
    "stryker .github/tools/src/conformance",
    "stryker .github/tools/src/release/commands",
    "stryker release-rest",
    "stryker .github/tools/src/peer",
    "stryker tools-rest",
  ]);
  const summary = workflow.jobs["mutation-summary"]!;
  assert.deepEqual(summary.needs, ["mutation-full"]);
  assert.equal(summary.if, "${{ !cancelled() }}");
  const report = summary.steps.find((step) =>
    /mutation-report\.ts/.test(step.run ?? ""),
  )!;
  assert.match(report.run!, new RegExp(`--expect ${entries.length} >>`));
});

test("every Stryker matrix leg selects existing source files", () => {
  const entries =
    readWorkflow().jobs["mutation-full"]!.strategy!.matrix.include;
  for (const entry of entries.filter((entry) => entry.tool === "stryker")) {
    assert.ok(areaSources(entry.target).length > 0, entry.target);
  }
});

test("the Stryker legs together mutate every source file exactly once", () => {
  const legs = readWorkflow()
    .jobs["mutation-full"]!.strategy!.matrix.include.filter(
      (entry) => entry.tool === "stryker",
    )
    .flatMap((entry) => areaSources(entry.target));
  const all = areaSources(".github/tools/src");
  assert.deepEqual([...legs].sort(), [...all].sort());
});

test("each Stryker leg restores and saves its own incremental results, and a manual run can ignore them", () => {
  const workflow = readWorkflow();
  assert.equal(workflow.on.workflow_call!.inputs.fresh.type, "boolean");
  const steps = workflow.jobs["mutation-full"]!.steps;
  const cache = steps.findIndex((step) =>
    String(step.uses).startsWith("actions/cache@"),
  );
  const mutate = steps.findIndex(
    (step) => step.name === "Mutate one TypeScript area",
  );
  assert.ok(cache >= 0 && mutate > cache);
  assert.equal(steps[cache]!.if, "matrix.tool == 'stryker'");
  assert.deepEqual(steps[cache]!.with, {
    path: "reports/mutation-incremental",
    key: "stryker-incremental-${{ matrix.target }}-${{ github.run_id }}",
    "restore-keys": "stryker-incremental-${{ matrix.target }}-",
  });
  assert.match(
    steps[mutate]!.run!,
    /--area "\$TARGET" --incremental reports\/mutation-incremental\/stryker\.json \$FRESH$/,
  );
  assert.equal(
    steps[mutate]!.env!.FRESH,
    "${{ inputs.fresh && '--force' || '' }}",
  );
});

test("Go is set up before Stryker runs, because some tests build the Go command line", () => {
  const workflow = readWorkflow();
  const steps = workflow.jobs["mutation-full"]!.steps;
  const go = steps.findIndex((step) =>
    /actions\/setup-go@/.test(step.uses ?? ""),
  );
  const stryker = steps.findIndex((step) =>
    /npm run test:mutation\b/.test(step.run ?? ""),
  );
  assert.ok(go >= 0 && go < stryker, "mutation-full sets up Go before Stryker");
});
