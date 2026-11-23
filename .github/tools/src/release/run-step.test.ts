// Pins the release action's Run step: one line that starts ci-release-run.ts from the checkout that supplies the
// release code, with an env block that carries every input and nothing else. Text seam: the checker reads the action's
// text and returns violations, and each mutation test edits that text in memory and requires the checker to report it.
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import test from "node:test";

const root = join(import.meta.dirname, "..", "..", "..", "..");
const action = readFileSync(
  join(root, ".github/actions/release/action.yml"),
  "utf8",
).replaceAll("\r\n", "\n");

// The Run step starts this one module from the checkout that supplies the release code, never from the workspace.
const RUN_LINE =
  '      run: node "$CHECKOUT/.github/tools/src/scripts/ci-release-run.ts"';

// The Run step's env block. Every input reaches the module through it, so it must not change while the module reads
// these names.
const RUN_ENV = [
  "      env:",
  "        COMMAND: ${{ inputs.command }}",
  "        CHECKOUT: ${{ inputs.checkout }}",
  "        REPO: ${{ inputs.repo }}",
  "        SHA: ${{ inputs.sha }}",
  "        TAG: ${{ inputs.tag }}",
  "        MAIN_REF: ${{ inputs.main-ref }}",
  "        ASSETS: ${{ inputs.assets }}",
  "        NOTES: ${{ inputs.notes }}",
  "        BASELINE: ${{ inputs.baseline }}",
  "        OUT: ${{ inputs.out }}",
  "        PUBLISHED: ${{ inputs.published }}",
  "        PARENT: ${{ inputs.parent }}",
  "        AUTHOR: ${{ inputs.author }}",
  "        PRERELEASE: ${{ inputs.prerelease }}",
  "        # An input, not an inherited variable. GitHub does not document",
  "        # whether a run step inside a composite action sees the env of the",
  "        # step that called it, so the two API commands must not rely on it.",
  "        GH_TOKEN: ${{ inputs.token }}",
].join("\n");

// Replaces the one occurrence of `from` and fails if there is not exactly one, or if the edit changed nothing, so a
// stale anchor cannot make a mutation test pass without exercising its check.
function mutate(source: string, from: string, to: string): string {
  assert.equal(
    source.split(from).length - 1,
    1,
    `mutation anchor must occur once: ${from}`,
  );
  const mutated = source.replace(from, () => to);
  assert.notEqual(mutated, source, "the mutation changed nothing");
  return mutated;
}

// The lines of one mapping entry: from `${indent}${key}:` to the next non-blank line indented no deeper than the key.
function block(source: string, key: string, indent: number): string {
  const lines = source.split("\n");
  const pad = " ".repeat(indent);
  const start = lines.indexOf(`${pad}${key}:`);
  assert.notEqual(start, -1, `no ${key}: entry at indent ${indent}`);
  let end = lines.findIndex(
    (line, i) => i > start && line.trim() !== "" && line.search(/\S/) <= indent,
  );
  if (end === -1) end = lines.length;
  return lines.slice(start, end).join("\n");
}

// One step's own lines with their comments: from its `- name:` line to the next step, the comment above the next step,
// or the end of the file.
function rawStep(source: string, name: string): string | undefined {
  const start = source.indexOf(`    - name: ${name}\n`);
  if (start === -1) return undefined;
  const body = source.slice(start);
  const first = body.indexOf("\n") + 1;
  const next = body.slice(first).search(/^ {4}(?:- |#)/m);
  return next === -1 ? body : body.slice(0, first + next);
}

// Whether the step's run: is the one line RUN_LINE, with no block scalar or continuation under it.
function runIsOneLine(step: string): boolean {
  const lines = step.split("\n");
  const runs = lines.filter((line) => /^ {6}run:/.test(line));
  if (runs.length !== 1 || runs[0] !== RUN_LINE) return false;
  const after = lines.slice(lines.indexOf(RUN_LINE) + 1);
  return after.every((line) => line.trim() === "" || line.search(/\S/) <= 6);
}

export function runStepViolations(text: string): string[] {
  const violations: string[] = [];
  const run = rawStep(text, "Run");
  if (run === undefined || !runIsOneLine(run)) {
    violations.push(
      `the Run step's run: must be the single line ${RUN_LINE.trim()}`,
    );
  }
  if (run === undefined || block(run, "env", 6) !== RUN_ENV) {
    violations.push(
      "the Run step's env block changed: every input reaches ci-release-run.ts through it",
    );
  }
  return violations;
}

test("the action's Run step is the one-line module call with its exact env", () => {
  assert.deepEqual(runStepViolations(action), []);
});

test("the Run step checker reports each way the step can change", () => {
  const cases: [string, string, string, RegExp][] = [
    [
      "the Run step loads the module from the workspace",
      'run: node "$CHECKOUT/.github/tools/src/scripts/ci-release-run.ts"',
      "run: node .github/tools/src/scripts/ci-release-run.ts",
      /Run step's run: must be the single line/,
    ],
    [
      "the Run step grows a script around the module",
      '      run: node "$CHECKOUT/.github/tools/src/scripts/ci-release-run.ts"',
      '      run: |\n        node "$CHECKOUT/.github/tools/src/scripts/ci-release-run.ts"\n        echo done',
      /Run step's run: must be the single line/,
    ],
    [
      "the Run step runs cli.ts directly",
      'node "$CHECKOUT/.github/tools/src/scripts/ci-release-run.ts"',
      'node "$CHECKOUT/.github/tools/src/release/cli.ts" "$COMMAND"',
      /Run step's run: must be the single line/,
    ],
    [
      "the Run step's env loses an input",
      "        PARENT: ${{ inputs.parent }}\n",
      "",
      /Run step's env block changed/,
    ],
    [
      "the Run step's env gains a variable",
      "        GH_TOKEN: ${{ inputs.token }}\n",
      "        GH_TOKEN: ${{ inputs.token }}\n        NODE_OPTIONS: --require=./hook.js\n",
      /Run step's env block changed/,
    ],
    [
      "the Run step's token comment drifts",
      "        # step that called it, so the two API commands must not rely on it.\n",
      "        # step that called it.\n",
      /Run step's env block changed/,
    ],
  ];
  for (const [label, from, to, expected] of cases) {
    const violations = runStepViolations(mutate(action, from, to));
    assert.match(violations.join("\n"), expected, label);
  }
});
