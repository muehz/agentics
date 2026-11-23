import assert from "node:assert/strict";
import {
  detectionBootstrapViolations,
  releaseWorkflowContractViolations,
  replaceFirst,
  workflowJobSection,
} from "./workflow.ts";

// One entry of the mutation table. The mutation edits the workflow text in memory, and the checker must report at least
// one violation of the edited text. When `want` is set, one reported violation must contain it, so the case fails for
// the check it targets and not for a side effect.
export interface MutationCase {
  readonly name: string;
  readonly base: "md" | "lock";
  readonly mutate: (source: string) => string;
  readonly check: (source: string) => string[];
  readonly want?: string;
}

// A case over release.md, checked by the contract checker.
export function contractCase(
  name: string,
  mutate: (source: string) => string,
  want?: string,
): MutationCase {
  return {
    name,
    base: "md",
    mutate,
    check: releaseWorkflowContractViolations,
    want,
  };
}

// A case over release.lock.yml, checked by the detection bootstrap checker.
export function detectionCase(
  name: string,
  mutate: (source: string) => string,
  want?: string,
): MutationCase {
  return {
    name,
    base: "lock",
    mutate,
    check: detectionBootstrapViolations,
    want,
  };
}

// Throws when the mutation changed nothing, when the checker found nothing, or when no violation names the targeted check.
export function assertMutationCaught(
  tc: MutationCase,
  sources: { readonly md: string; readonly lock: string },
): void {
  const base = sources[tc.base];
  const mutated = tc.mutate(base);
  assert.notEqual(
    mutated,
    base,
    "mutation had no effect; the fixture it targets has drifted",
  );
  const violations = tc.check(mutated);
  assert.ok(
    violations.length > 0,
    "mutated text should have produced at least one contract violation",
  );
  if (tc.want === undefined) return;
  const want = tc.want;
  assert.ok(
    violations.some((violation) => violation.includes(want)),
    `violations ${JSON.stringify(violations)} lack the targeted one ${JSON.stringify(want)}`,
  );
}

// Rewrites old to replacement inside one release.md job, so a mutation cannot land on an identical
// line in a neighbouring job.
export function replaceIn(
  job: string,
  nextJob: string,
  old: string,
  replacement: string,
): (source: string) => string {
  return (source) => {
    const section = workflowJobSection(source, job, nextJob);
    return replaceFirst(
      source,
      section,
      replaceFirst(section, old, replacement),
    );
  };
}

// The release commit check and the baseline wiring sit beside identical sha, tag, and path lines in other steps, so
// each mutation rewrites its own step's text and cannot land on a neighbour.
export const commitCheckStep =
  "            - name: Verify the release commit\n" +
  "              uses: ./.github/actions/release\n" +
  "              with:\n" +
  "                  command: check-commit\n" +
  "                  sha: ${{ steps.target.outputs.sha }}\n" +
  "                  tag: ${{ needs.release.outputs.tag }}\n" +
  "                  parent: ${{ steps.target.outputs.parent }}\n" +
  "                  author: ${{ needs.release.outputs.author }}\n";

export const targetCheckoutStep =
  "            - name: Checkout immutable target\n" +
  "              uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1\n" +
  "              with:\n" +
  "                  ref: ${{ steps.shape.outputs.sha }}\n" +
  "                  path: release-target\n" +
  "                  fetch-depth: 0\n" +
  "                  persist-credentials: false\n";

export const firstParentStep =
  "            - name: Move the verifier off a resumed release commit\n" +
  "              if: needs.level.outputs.mode != 'analyze'\n" +
  "              env:\n" +
  "                  MODE: ${{ needs.level.outputs.mode }}\n" +
  "              run: |\n" +
  '                  test "$MODE" = resume && test "$TARGET_SHA" = "$GITHUB_SHA" || exit 1\n' +
  '                  git checkout --detach "$GITHUB_SHA^1"\n';

export const baselineDownloadStep =
  "            - name: Download the baseline notes\n" +
  "              uses: actions/download-artifact@9000827ccba6bdab643e8b6fd33ac0654aef8333 # v8.0.2\n" +
  "              with:\n" +
  "                  name: release-baseline\n" +
  "                  path: release-baseline-in\n";

// Edits the check-commit step's own text; a source without that step is returned unchanged.
export function editCommitCheck(
  old: string,
  replacement: string,
): (source: string) => string {
  return (source) => {
    if (!source.includes(commitCheckStep) || !commitCheckStep.includes(old)) {
      return source;
    }
    return replaceFirst(
      source,
      commitCheckStep,
      replaceFirst(commitCheckStep, old, replacement),
    );
  };
}

// Moves the whole check-commit step before or after a line that occurs exactly once.
export function moveCommitCheck(
  anchor: string,
  after: boolean,
): (source: string) => string {
  return (source) => {
    if (
      !source.includes(commitCheckStep) ||
      source.split(anchor).length - 1 !== 1
    ) {
      return source;
    }
    const removed = replaceFirst(source, commitCheckStep, "");
    return after
      ? replaceFirst(removed, anchor, anchor + commitCheckStep)
      : replaceFirst(removed, anchor, commitCheckStep + anchor);
  };
}
