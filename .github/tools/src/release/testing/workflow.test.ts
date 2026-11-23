import assert from "node:assert/strict";
import { test } from "node:test";
import { readRepoText } from "./repo.ts";
import {
  assertEveryActionIsSHAPinnedViolations,
  countOf,
  detectionBootstrapViolations,
  installerInvocation,
  jobKeySection,
  releaseTriggerViolations,
  releaseWorkflowContractViolations,
  replaceEvery,
  replaceFirst,
  replaceOnce,
  splitSteps,
  withoutYAMLComments,
  workflowJobCondition,
  workflowJobSection,
  workflowSection,
  workflowStepBody,
  workflowStepContaining,
  workflowStepToEnd,
} from "./workflow.ts";

const md = readRepoText(".github/workflows/release.md");
const lock = readRepoText(".github/workflows/release.lock.yml");

test("literal edits ignore the replacement patterns of String.replace", () => {
  assert.equal(replaceFirst("a-b-b", "b", "$&$'"), "a-$&$'-b");
  assert.equal(replaceFirst("abc", "x", "y"), "abc");
  assert.equal(replaceEvery("a.b.c", ".", "$$"), "a$$b$$c");
  assert.equal(countOf("aaaa", "aa"), 2);
  assert.equal(countOf("abc", "z"), 0);
});

test("replaceOnce refuses a text that holds the target zero or two times", () => {
  assert.equal(replaceOnce("a b", "a", "c"), "c b");
  assert.throws(() => replaceOnce("a b", "z", "c"), /occurs 0 times/);
  assert.throws(() => replaceOnce("a a", "a", "c"), /occurs 2 times/);
});

test("workflowSection and workflowJobSection bound a job by its neighbour", () => {
  const level = workflowJobSection(md, "level", "approve-major");
  assert.ok(level.includes("command: resume-check"));
  assert.ok(!level.includes("environment: github-release"));
  const agent = workflowJobSection(md, "agent", "");
  assert.ok(agent.includes("copilot-requests: write"));
});

test("a missing marker throws and names the marker", () => {
  assert.throws(
    () => workflowSection(md, "\nno-such-section:\n", "\n"),
    /no-such-section/,
  );
  assert.throws(
    () => workflowSection(md, "\non:\n", "\nno-such-end:\n"),
    /no end marker/,
  );
  assert.throws(
    () => workflowJobSection(md, "level", "no-such-job"),
    /no-such-job/,
  );
  assert.throws(() => workflowStepToEnd(md, "No such step"), /No such step/);
});

const sample = [
  "    job:",
  "        if: ${{ top }}",
  "        steps:",
  "            - name: One",
  "              if: step-level",
  "              run: echo one",
  "            # prose about two",
  "            - name: Two",
  "              run: echo two",
  "",
].join("\n");

test('workflowStepBody returns the step with the next step\'s comment and "" when absent', () => {
  const body = workflowStepBody(sample, "One");
  assert.ok(body.startsWith("            - name: One\n"));
  assert.ok(body.includes("# prose about two"));
  assert.ok(!body.includes("name: Two"));
  assert.equal(workflowStepBody(sample, "Three"), "");
  assert.equal(
    workflowStepBody(sample, "Two"),
    "            - name: Two\n              run: echo two\n",
  );
});

test("workflowStepContaining finds a step by what it does", () => {
  assert.ok(
    workflowStepContaining(sample, "echo two").startsWith(
      "            - name: Two",
    ),
  );
  assert.equal(workflowStepContaining(sample, "echo nine"), "");
  assert.equal(workflowStepContaining("echo two", "echo two"), "");
});

test("workflowJobCondition reads the job's own if and not a step's", () => {
  assert.equal(workflowJobCondition(sample), "${{ top }}");
  assert.equal(workflowJobCondition("        steps:\n"), "");
});

test("withoutYAMLComments drops whole-line comments only", () => {
  assert.equal(
    withoutYAMLComments("a: 1\n  # note\nb: 2 # kept\n"),
    "a: 1\nb: 2 # kept\n",
  );
});

test("jobKeySection ends at the next top-level job key whatever its name", () => {
  const compiled = "x\n  one:\n    a: 1\n  zz_renamed:\n    b: 2\n";
  assert.equal(jobKeySection(compiled, "one"), "    a: 1\n");
  assert.equal(jobKeySection(compiled, "zz_renamed"), "    b: 2\n");
  assert.throws(() => jobKeySection(compiled, "absent"), /absent/);
});

test("splitSteps returns null when no step matches", () => {
  assert.equal(splitSteps("    runs-on: x\n"), null);
  assert.deepEqual(
    splitSteps("      - name: a\n        run: 1\n      - name: b\n"),
    ["      - name: a\n        run: 1\n", "      - name: b\n"],
  );
  assert.equal(
    installerInvocation(
      '      run: bash "x/install_threat_detect_binary.sh" v1',
    ),
    'bash "x/install_threat_detect_binary.sh" v1',
  );
  assert.equal(installerInvocation("run: echo"), "");
});

test("every action in the real workflow is pinned to a commit", () => {
  assert.deepEqual(assertEveryActionIsSHAPinnedViolations(md), []);
});

// Pins the check: each line below must be reported, so deleting the 40-hex test or the missing-ref branch fails here.
test("an action without a commit pin is reported", () => {
  const sha = "0123456789abcdef0123456789abcdef01234567";
  const lines = [
    "- uses: actions/checkout@v7",
    "  uses: actions/checkout",
    `  uses: actions/checkout@${sha.toUpperCase()}`,
    `  uses: actions/checkout@${sha.slice(1)}`,
  ].join("\n");
  assert.deepEqual(assertEveryActionIsSHAPinnedViolations(lines), [
    'action "actions/checkout@v7" is not pinned to a 40-hex commit',
    'action "actions/checkout" has no ref; pin it to a 40-hex commit',
    `action "actions/checkout@${sha.toUpperCase()}" is not pinned to a 40-hex commit`,
    `action "actions/checkout@${sha.slice(1)}" is not pinned to a 40-hex commit`,
  ]);
  assert.deepEqual(
    assertEveryActionIsSHAPinnedViolations(
      `  uses: actions/checkout@${sha} # v7.0.1\n  uses: ./.github/actions/release\n`,
    ),
    [],
  );
});

const triggers = workflowSection(md, "\non:\n", "\nconcurrency:\n");

test("the real triggers hold the dispatch contract", () => {
  assert.deepEqual(releaseTriggerViolations(triggers), []);
});

// Each mutation pins one clause of the trigger rule, so removing a clause makes its case pass silently and fail here.
test("a trigger change that breaks the dispatch contract is reported", () => {
  const dispatchOnly = "release must run only on workflow_dispatch";
  const oneInput =
    "release must take exactly one dispatch input, prerelease, a boolean that defaults to false";
  const cases: [string, string, string][] = [
    [
      "a push trigger",
      replaceOnce(
        triggers,
        "    workflow_dispatch:\n",
        "    push:\n        branches: [main]\n    workflow_dispatch:\n",
      ),
      dispatchOnly,
    ],
    ["no dispatch", "    push:\n", dispatchOnly],
    [
      "a second input",
      replaceOnce(
        triggers,
        "            prerelease:\n",
        "            target_sha:\n                type: string\n            prerelease:\n",
      ),
      oneInput,
    ],
    [
      "a string input",
      replaceOnce(triggers, "type: boolean", "type: string"),
      oneInput,
    ],
    [
      "a true default",
      replaceOnce(triggers, "default: false", "default: true"),
      oneInput,
    ],
    [
      "a quoted default",
      replaceOnce(triggers, "default: false", 'default: "false"'),
      oneInput,
    ],
    ["no inputs", "    workflow_dispatch:\n", oneInput],
  ];
  for (const [label, mutated, want] of cases) {
    assert.deepEqual(releaseTriggerViolations(mutated), [want], label);
  }
  const broken = releaseTriggerViolations("    workflow_dispatch: [\n");
  assert.equal(broken.length, 1);
  assert.match(broken[0] ?? "", /^release triggers are not valid YAML: /);
});

test("the real lock keeps the detector bootstrap", () => {
  assert.deepEqual(detectionBootstrapViolations(lock), []);
});

test("a detection job without its guarantees is reported", () => {
  const strict = 'GH_AW_DETECTION_CONTINUE_ON_ERROR: "false"';
  assert.ok(
    detectionBootstrapViolations(replaceEvery(lock, strict, "")).includes(
      "detection must remain strict (continue-on-error: false)",
    ),
  );
  assert.ok(
    detectionBootstrapViolations(
      replaceEvery(lock, "if: always()", "if: never()"),
    ).includes("detection must keep an unconditional step"),
  );
  const guard =
    "!cancelled() && steps.detection_guard.outputs.run_detection != 'true'";
  assert.ok(
    detectionBootstrapViolations(
      replaceEvery(lock, guard, "!cancelled()"),
    ).includes("detector bootstrap must cover the skipped-run path"),
  );
});

test("the real release.md holds the whole contract", () => {
  assert.deepEqual(releaseWorkflowContractViolations(md), []);
});

// The full mutation table lives in workflow-mutations.test.ts. These cases show that the checker itself reports each
// family of rule, so a rule dropped from the checker fails here even before the table runs.
test("independent breakages of the contract are each reported", () => {
  const levelSection = workflowJobSection(md, "level", "approve-major");
  const cases: [string, string, string][] = [
    [
      "an environment on prepare",
      replaceFirst(
        md,
        "    prepare:\n",
        "    prepare:\n        environment: github-release\n",
      ),
      "only approve-major and release may name an environment",
    ],
    [
      "contents: write on level",
      replaceFirst(
        md,
        levelSection,
        replaceFirst(levelSection, "contents: read", "contents: write"),
      ),
      "level must not hold contents: write",
    ],
    [
      "a retired publish job",
      replaceFirst(
        md,
        "\n    approve-major:\n",
        "\n    publish:\n        runs-on: ubuntu-latest\n\n    approve-major:\n",
      ),
      'retired publication path "\\n    publish:\\n"',
    ],
    [
      "an obsolete notes path",
      replaceFirst(
        md,
        "# Agentics release notes agent",
        "# Agentics release notes agent release-bundle.sh",
      ),
      'obsolete notes path "release-bundle.sh"',
    ],
    [
      "no shared serialization",
      replaceFirst(md, "group: agentics-release", "group: other"),
      "draft assembly and publication must share workflow serialization",
    ],
    [
      "an action pinned to a tag",
      replaceFirst(
        md,
        "actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1",
        "actions/checkout@v7",
      ),
      'action "actions/checkout@v7" is not pinned to a 40-hex commit',
    ],
  ];
  for (const [label, mutated, want] of cases) {
    assert.notEqual(mutated, md, `${label}: the mutation had no effect`);
    assert.ok(
      releaseWorkflowContractViolations(mutated).includes(want),
      `${label}: lacks ${want}`,
    );
  }
});

test("a text without the workflow's jobs throws instead of passing", () => {
  assert.throws(
    () => releaseWorkflowContractViolations("name: not a workflow\n"),
    /workflow is missing section/,
  );
});
