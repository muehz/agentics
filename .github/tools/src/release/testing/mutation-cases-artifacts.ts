import {
  baselineDownloadStep,
  contractCase,
  detectionCase,
  replaceIn,
  type MutationCase,
} from "./mutation-support.ts";
import {
  replaceEvery,
  replaceFirst,
  workflowJobSection,
  workflowStepContaining,
} from "./workflow.ts";

// Relocates the literal "command: stage" text to just after the stage job's header,
// ahead of the rebuild-compare and attestation steps.
function moveCommandStageEarlier(source: string): string {
  const marker = "command: stage";
  const at = source.indexOf(marker);
  if (at < 0) return source;
  const removed = source.slice(0, at) + source.slice(at + marker.length);
  const stageJobMarker = "\n    stage:\n";
  const jobAt = removed.indexOf(stageJobMarker);
  if (jobAt < 0) return source;
  const insertAt = jobAt + stageJobMarker.length;
  return removed.slice(0, insertAt) + marker + "\n" + removed.slice(insertAt);
}

// Relocates the complete local verification step after attestation and keeps
// the re-derive, attest, stage order, so the verify-before-attest rule is the only one it breaks.
function moveCommandVerifyAfterAttestation(source: string): string {
  const verifyStep =
    "            - name: Verify local bundle roster and checksums\n" +
    "              uses: ./.github/actions/release\n" +
    "              with:\n" +
    "                  command: verify\n" +
    "                  assets: release-assets\n" +
    "                  tag: ${{ needs.prepare.outputs.tag }}\n";
  if (!source.includes(verifyStep)) return source;
  const removed = replaceFirst(source, verifyStep, "");
  const nextStep =
    "\n            - name: Build the baseline notes and release-range context\n";
  if (!removed.includes(nextStep)) return source;
  return replaceFirst(removed, nextStep, "\n" + verifyStep + nextStep);
}

// Moves the pinned threat-detect installer call out of the
// skipped-run step to a new step elsewhere in the detection job. The relocated call is identical, so the "installed at
// least twice, identically" check still passes and only the "the skipped-run step itself invokes it" check can catch it.
function moveBootstrapInstallerOutOfSkippedRunStep(source: string): string {
  const prefix =
    'run: bash "${RUNNER_TEMP}/gh-aw/actions/install_threat_detect_binary.sh" ';
  const start = source.indexOf(prefix);
  if (start < 0) return source;
  const line = source.slice(start).split("\n")[0] ?? "";
  const at = source.indexOf(line);
  if (at < 0) return source;
  const removed = source.slice(0, at) + source.slice(at + line.length);

  const detectionMarker = "\n  detection:\n";
  const detectionAt = removed.indexOf(detectionMarker);
  if (detectionAt < 0) return source;
  const jobBodyStart = detectionAt + detectionMarker.length;
  const rest = removed.slice(jobBodyStart);
  const next = /^ {2}[A-Za-z_]+:\r?\n/m.exec(rest);
  const insertAt = next === null ? rest.length : next.index;
  const relocated =
    "      - name: Relocated installer for mutation test\n        " +
    line +
    "\n";
  return (
    removed.slice(0, jobBodyStart) +
    rest.slice(0, insertAt) +
    relocated +
    rest.slice(insertAt)
  );
}

const retentionLine = "                  retention-days: 30\n";
const statePath = "                  path: release-state/state.json\n";

function moveCandidateRetentionToStateUpload(source: string): string {
  if (!source.includes(retentionLine) || !source.includes(statePath))
    return source;
  const moved = replaceFirst(source, retentionLine, "");
  return replaceFirst(moved, statePath, statePath + retentionLine);
}

function addStateUploadRetention(source: string): string {
  if (!source.includes(retentionLine) || !source.includes(statePath))
    return source;
  return replaceFirst(source, statePath, statePath + retentionLine);
}

const candidateDownloadHeader =
  "            - name: Download release candidate\n" +
  "              uses: actions/download-artifact@9000827ccba6bdab643e8b6fd33ac0654aef8333 # v8.0.2\n";

function makeCandidateDownloadOptional(source: string): string {
  return replaceFirst(
    source,
    candidateDownloadHeader,
    candidateDownloadHeader + "              if: false\n",
  );
}

function makeCandidateDownloadContinueOnError(source: string): string {
  return replaceFirst(
    source,
    candidateDownloadHeader,
    candidateDownloadHeader + "              continue-on-error: true\n",
  );
}

// Mutation cases 51 to 68 of the table: the baseline hop, stale prose, action pinning, candidate retention and
// download, credentials, the detection job, and the verify and stage ordering.
export const artifactCases: readonly MutationCase[] = [
  contractCase(
    "dropping the baseline input builds the notes from the squash subjects alone",
    replaceIn(
      "stage",
      "agent",
      "                  baseline: release-baseline-in/baseline.md\n",
      "",
    ),
    'context step missing "baseline: release-baseline-in/baseline.md"',
  ),
  contractCase(
    "dropping the baseline download leaves the context input pointing at nothing",
    replaceIn("stage", "agent", baselineDownloadStep + "\n", ""),
    'baseline download missing "name: release-baseline"',
  ),
  contractCase(
    "downloading the baseline to another path leaves the context input pointing at nothing",
    replaceIn(
      "stage",
      "agent",
      "path: release-baseline-in\n",
      "path: release-in\n",
    ),
    'baseline download missing "path: release-baseline-in"',
  ),
  contractCase(
    "downloading the baseline after the context step reads a file that is not there yet",
    (s) => {
      const contextStart =
        "            - name: Build the baseline notes and release-range context\n";
      const contextEnd = "                  token: ${{ github.token }}\n";
      const stage = workflowJobSection(s, "stage", "agent");
      const context = workflowStepContaining(stage, "command: context");
      if (
        !stage.includes(baselineDownloadStep + "\n") ||
        !context.startsWith(contextStart)
      ) {
        return s;
      }
      let moved = replaceFirst(stage, baselineDownloadStep + "\n", "");
      moved = replaceFirst(
        moved,
        contextEnd,
        contextEnd + "\n" + baselineDownloadStep,
      );
      return replaceFirst(s, stage, moved);
    },
    "stage must download the baseline notes before it builds the context",
  ),
  contractCase(
    "naming GitHub's note generator in the prompt misleads the agent about the baseline",
    (s) =>
      replaceFirst(
        s,
        "contains the release notes for the range",
        "contains GitHub-generated notes for the range",
      ),
    'still names "GitHub-generated notes"',
  ),
  contractCase(
    "naming GitHub's changelog in the prompt misleads the agent about what follows its text",
    (s) =>
      replaceFirst(
        s,
        "The generated changelog and the license section",
        "GitHub's generated changelog and the license section",
      ),
    'still names "GitHub\'s generated changelog"',
  ),
  contractCase(
    "naming GitHub's notes in the no-pull-request case misleads the agent about the baseline",
    (s) =>
      replaceFirst(
        s,
        "make no update: the generated notes stand alone.",
        "make no update: GitHub's generated notes stand alone.",
      ),
    'still names "GitHub\'s generated notes"',
  ),
  contractCase(
    "naming GitHub's generator in the update-release-notes input misleads the model that fills it",
    (s) =>
      replaceFirst(
        s,
        "highlights to place before the generated notes.",
        "highlights to place before GitHub-generated notes.",
      ),
    'still names "GitHub-generated notes"',
  ),
  contractCase(
    "an unpinned token action can change under the release job",
    // The pattern matches the pinned action whatever commit it pins, so a legitimate bump does not break this case.
    (s) =>
      s.replace(
        /actions\/create-github-app-token@[0-9a-f]{40}/g,
        () => "actions/create-github-app-token@v3",
      ),
    'action "actions/create-github-app-token@v3" is not pinned to a 40-hex commit',
  ),
  contractCase(
    "moving retention to another artifact shortens the candidate recovery window",
    moveCandidateRetentionToStateUpload,
  ),
  contractCase(
    "adding retention to another artifact violates the candidate-only policy",
    addStateUploadRetention,
  ),
  contractCase(
    "making the candidate download optional permits staging without the candidate",
    makeCandidateDownloadOptional,
  ),
  contractCase(
    "allowing candidate download failure permits staging without the candidate",
    makeCandidateDownloadContinueOnError,
  ),
  contractCase(
    "removing persist-credentials: false lets an untrusted checkout keep credentials",
    (s) => replaceEvery(s, "persist-credentials: false", ""),
  ),
  detectionCase(
    "removing the strict detection flag lets threat detection fail open",
    (s) => replaceEvery(s, 'GH_AW_DETECTION_CONTINUE_ON_ERROR: "false"\n', ""),
  ),
  contractCase(
    "moving stage ahead of its rebuild-compare breaks the attest-before-publish order",
    moveCommandStageEarlier,
  ),
  contractCase(
    "moving verification after attestation breaks the verify-before-attest order",
    moveCommandVerifyAfterAttestation,
  ),
  detectionCase(
    "relocating the bootstrap installer out of the skipped-run step leaves that path uninstalled",
    moveBootstrapInstallerOutOfSkippedRunStep,
  ),
];
