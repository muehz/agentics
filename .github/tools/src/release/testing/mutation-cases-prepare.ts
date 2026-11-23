import {
  contractCase,
  editCommitCheck,
  commitCheckStep,
  firstParentStep,
  moveCommitCheck,
  replaceIn,
  targetCheckoutStep,
  type MutationCase,
} from "./mutation-support.ts";
import { replaceFirst } from "./workflow.ts";

const parentLine = "parent: ${{ steps.target.outputs.parent }}";
const firstParentWant =
  "prepare must move the verifier to the release commit's first parent on a resumed run, before the release commit check";

// Mutation cases 22 to 50 of the table: the dispatch triggers and input, the publication-mode record, and the
// prepare job's release commit check and checkouts.
export const prepareCases: readonly MutationCase[] = [
  contractCase(
    "a push trigger lets every merge release",
    (s) =>
      replaceFirst(
        s,
        "\non:\n    workflow_dispatch:\n",
        "\non:\n    push:\n        branches: [main]\n    workflow_dispatch:\n",
      ),
    "release must run only on workflow_dispatch",
  ),
  contractCase(
    "a second dispatch input lets an operator choose the release target",
    (s) =>
      replaceFirst(
        s,
        "            prerelease:\n",
        "            target_sha:\n                type: string\n            prerelease:\n",
      ),
    "release must take exactly one dispatch input",
  ),
  contractCase(
    "a string prerelease input accepts any value",
    (s) =>
      replaceFirst(
        s,
        "                type: boolean\n",
        "                type: string\n",
      ),
    "release must take exactly one dispatch input",
  ),
  contractCase(
    "a prerelease input that defaults to true makes every dispatch a pre-release",
    (s) =>
      replaceFirst(
        s,
        "                default: false\n",
        "                default: true\n",
      ),
    "release must take exactly one dispatch input",
  ),
  contractCase(
    "removing the prerelease input leaves no way to choose a pre-release",
    (s) => {
      const start = s.indexOf("        inputs:\n");
      const end = s.indexOf("\nconcurrency:\n");
      return s.slice(0, start) + s.slice(end + 1);
    },
    "release must take exactly one dispatch input",
  ),
  contractCase(
    "a run-name without the run number cannot tell two runs apart",
    (s) =>
      replaceFirst(
        s,
        "run-name: Agentics Release #${{ github.run_number }}\n",
        "run-name: Agentics release\n",
      ),
    "run-name must identify the run",
  ),
  contractCase(
    "a stage step without the input always publishes a real release",
    replaceIn(
      "stage",
      "agent",
      "                  prerelease: ${{ inputs.prerelease }}\n",
      "",
    ),
    "the stage step must pass the dispatch input prerelease",
  ),
  contractCase(
    "a state record without the mode leaves the notes job guessing",
    replaceIn("stage", "agent", ", prerelease:$prerelease}", "}"),
    "the prepared release identity must record the mode",
  ),
  contractCase(
    "the notes writer passes a different artifact to the adapter",
    (s) =>
      replaceFirst(
        s,
        "RELEASE_STATE_FILE: release-state/state.json",
        "RELEASE_STATE_FILE: notes-handoff/state.json",
      ),
    "the notes writer must use the bound artifact input",
  ),
  contractCase(
    "the handoff reads an envelope outside the safe-output transport",
    (s) =>
      replaceFirst(s, 'cp -- "$GH_AW_AGENT_OUTPUT"', 'cp -- "$OTHER_FILE"'),
    "the handoff must use the detector-approved envelope",
  ),
  contractCase(
    "prepare computing its own version can disagree with the release commit",
    replaceIn(
      "prepare",
      "stage",
      "            # bundle requires an existing empty directory.\n",
      "            - name: Resolve version\n              id: version\n              uses: ./.github/actions/release\n              with:\n                  command: version\n                  sha: ${{ steps.target.outputs.sha }}\n\n            # bundle requires an existing empty directory.\n",
    ),
    "prepare must take the tag from the release job, not compute a version",
  ),
  contractCase(
    "prepare without the level job cannot tell a resumed run from a fresh one",
    replaceIn(
      "prepare",
      "stage",
      "needs: [release, level]",
      "needs: [release]",
    ),
    'target validation missing "needs: [release, level]"',
  ),
  contractCase(
    "dropping the commit check lets prepare bundle a commit nobody verified",
    replaceIn("prepare", "stage", commitCheckStep, ""),
    'target validation missing "command: check-commit"',
  ),
  contractCase(
    "checking out the target over the reviewed workspace lets it supply the verifier",
    replaceIn("prepare", "stage", "path: release-target", "path: ."),
    'target validation missing "path: release-target"',
  ),
  contractCase(
    "moving the commit check after the bundle verifies a commit that is already built",
    moveCommitCheck(
      "                  out: ${{ github.workspace }}/release-assets\n\n",
      true,
    ),
    "prepare must verify the release commit after the main-ancestry check and before it bundles it",
  ),
  contractCase(
    "moving the commit check before the ancestry check verifies a commit not yet known to be on main",
    moveCommitCheck("            - name: Establish main ancestry\n", false),
    "prepare must verify the release commit after the main-ancestry check and before it bundles it",
  ),
  contractCase(
    "the dispatch SHA as parent rejects every valid resumed release",
    editCommitCheck("${{ steps.target.outputs.parent }}", "${{ github.sha }}"),
    'release commit check missing "' + parentLine + '"',
  ),
  contractCase(
    "dropping the parent skips the check that the release was made on the pushed head",
    editCommitCheck("                  " + parentLine + "\n", ""),
    'release commit check missing "' + parentLine + '"',
  ),
  contractCase(
    "dropping the author lets any committer name pass the check",
    editCommitCheck(
      "                  author: ${{ needs.release.outputs.author }}\n",
      "",
    ),
    'release commit check missing "author: ${{ needs.release.outputs.author }}"',
  ),
  contractCase(
    "dropping the tag leaves the check without the version it must match",
    editCommitCheck(
      "                  tag: ${{ needs.release.outputs.tag }}\n",
      "",
    ),
    'release commit check missing "tag: ${{ needs.release.outputs.tag }}"',
  ),
  contractCase(
    "checking the pushed head instead of the target verifies the wrong commit",
    editCommitCheck(
      "sha: ${{ steps.target.outputs.sha }}",
      "sha: ${{ github.sha }}",
    ),
    'release commit check missing "sha: ${{ steps.target.outputs.sha }}"',
  ),
  contractCase(
    "a condition on the commit check lets prepare skip it",
    editCommitCheck(
      "              with:\n",
      "              if: false\n              with:\n",
    ),
    'the release commit check must not carry "if:"',
  ),
  contractCase(
    "continue-on-error on the commit check lets a failed check pass",
    editCommitCheck(
      "              with:\n",
      "              continue-on-error: true\n              with:\n",
    ),
    'the release commit check must not carry "continue-on-error:"',
  ),
  contractCase(
    "a checkout without full history cannot compare the release tag with the existing ones",
    replaceIn("prepare", "stage", "                  fetch-depth: 0\n", ""),
    'verifier checkout missing "fetch-depth: 0"',
  ),
  contractCase(
    "a target checkout without full history bundles from a repository that lacks the tags",
    replaceIn(
      "prepare",
      "stage",
      targetCheckoutStep,
      replaceFirst(
        targetCheckoutStep,
        "                  fetch-depth: 0\n",
        "",
      ),
    ),
    'target checkout missing "fetch-depth: 0"',
  ),
  contractCase(
    "checking out the target as the verifier lets the release commit vouch for itself",
    replaceIn(
      "prepare",
      "stage",
      "                  ref: ${{ github.sha }}\n",
      "                  ref: ${{ steps.shape.outputs.sha }}\n",
    ),
    "prepare must check out the pushed head before the release commit check, so the verifier code is reviewed",
  ),
  contractCase(
    "checking out the target before the check runs the verifier from the commit it verifies",
    (s) => {
      const checkStart = "            - name: Verify the release commit\n";
      s = replaceIn("prepare", "stage", targetCheckoutStep + "\n", "")(s);
      return replaceIn(
        "prepare",
        "stage",
        checkStart,
        targetCheckoutStep + "\n" + checkStart,
      )(s);
    },
    "prepare must check out the release target only after the release commit check and before the bundle",
  ),
  contractCase(
    "dropping the first-parent step runs a resumed run's verifier from the release commit",
    replaceIn("prepare", "stage", firstParentStep, ""),
    firstParentWant,
  ),
  contractCase(
    "flipping the first-parent condition moves the reviewed head and keeps the release commit",
    replaceIn(
      "prepare",
      "stage",
      "if: needs.level.outputs.mode != 'analyze'\n",
      "if: needs.level.outputs.mode == 'analyze'\n",
    ),
    firstParentWant,
  ),
];
