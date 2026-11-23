// The text seam for .github/workflows/release.md and release.lock.yml: section and step finders and the checkers built
// on them. A checker returns its violations instead of failing, so a mutation test can run the same checker on a broken
// copy and require a violation.

import { parse, YAMLError } from "yaml";

type Mapping = Readonly<Record<string, unknown>>;

function isMapping(value: unknown): value is Mapping {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

// Follows a path of mapping keys and returns undefined as soon as a step is missing or is not a mapping.
function lookup(value: unknown, ...path: string[]): unknown {
  let current = value;
  for (const key of path) {
    if (!isMapping(current) || !Object.hasOwn(current, key)) return undefined;
    current = current[key];
  }
  return current;
}

// The keys of a mapping, or none when the value is not one.
function keysOf(value: unknown): string[] {
  return isMapping(value) ? Object.keys(value) : [];
}

// A scalar as text, so a message can quote what a key held; a collection or a missing value reads as "".
function scalarText(value: unknown): string {
  if (value === undefined || typeof value === "object") return "";
  return String(value);
}

// Literal string edits. JavaScript's String.replace reads "$&" and "$'" in a replacement, which the workflow text can
// contain, so every edit here goes through indexOf.

// Replaces the first occurrence of old. Returns source unchanged when old is absent.
export function replaceFirst(
  source: string,
  old: string,
  replacement: string,
): string {
  const at = source.indexOf(old);
  if (at < 0) return source;
  return source.slice(0, at) + replacement + source.slice(at + old.length);
}

// Replaces every occurrence of old.
export function replaceEvery(
  source: string,
  old: string,
  replacement: string,
): string {
  return source.split(old).join(replacement);
}

// Counts the non-overlapping occurrences of sub, which must not be empty.
export function countOf(source: string, sub: string): number {
  return source.split(sub).length - 1;
}

// Replaces old with replacement; throws unless old occurs exactly once.
export function replaceOnce(
  source: string,
  old: string,
  replacement: string,
): string {
  const count = countOf(source, old);
  if (count !== 1) {
    throw new Error(
      `${JSON.stringify(old)} occurs ${count} times in the workflow, want 1`,
    );
  }
  return replaceFirst(source, old, replacement);
}

// The text between two markers; throws when a marker is missing.
export function workflowSection(
  source: string,
  startMarker: string,
  endMarker: string,
): string {
  const start = source.indexOf(startMarker);
  if (start < 0) {
    throw new Error(
      `workflow is missing section ${JSON.stringify(startMarker)}`,
    );
  }
  const from = start + startMarker.length;
  const end = source.indexOf(endMarker, from);
  if (end < 0) {
    throw new Error(
      `workflow section ${JSON.stringify(startMarker)} has no end marker ${JSON.stringify(endMarker)}`,
    );
  }
  return source.slice(from, end);
}

// The section of one job, up to nextJob. An empty nextJob takes the text up to the closing frontmatter fence.
export function workflowJobSection(
  source: string,
  job: string,
  nextJob: string,
): string {
  const endMarker = nextJob === "" ? "\n---\n" : `\n    ${nextJob}:\n`;
  return workflowSection(source, `\n    ${job}:\n`, endMarker);
}

const STEP_START = "            - name: ";

// The named step from its "- name:" line to the next step; "" when absent. Comment lines above the next step belong to
// the result, so a caller must not read comment wording as wiring.
export function workflowStepBody(section: string, step: string): string {
  const start = section.indexOf(`${STEP_START}${step}\n`);
  if (start < 0) return "";
  let body = section.slice(start + STEP_START.length);
  const end = body.indexOf(`\n${STEP_START}`);
  if (end >= 0) body = body.slice(0, end);
  return section.slice(start, start + STEP_START.length) + body;
}

// The step that holds marker, found by what it does and not by its name; "" when none.
export function workflowStepContaining(
  section: string,
  marker: string,
): string {
  const at = section.indexOf(marker);
  if (at < 0) return "";
  const start = section.slice(0, at).lastIndexOf(`\n${STEP_START}`);
  if (start < 0) return "";
  let body = section.slice(start + 1);
  const end = body.indexOf(`\n${STEP_START}`);
  if (end >= 0) body = body.slice(0, end);
  return body;
}

// From the named step to the end of the text; throws when the step is missing.
export function workflowStepToEnd(source: string, step: string): string {
  const start = source.indexOf(`${STEP_START}${step}\n`);
  if (start < 0)
    throw new Error(`workflow is missing step ${JSON.stringify(step)}`);
  return source.slice(start);
}

// The value of a job's own `if:` line (eight-space indent); "" when none.
export function workflowJobCondition(section: string): string {
  const match = /^ {8}if: (.*)$/m.exec(section);
  return match === null ? "" : (match[1] ?? "");
}

// The reviewed scripts rebuild the immutable target before the candidate can be attested.
export function stageDerivationViolations(stage: string): string[] {
  const job = parse(stage);
  const steps: Mapping[] = job.steps;
  const checkout = steps.findIndex(
    (step) => step.name === "Checkout immutable target",
  );
  const node = steps.findIndex(
    (step) => step.name === "Set up maintainer Node",
  );
  const rebuild = steps.findIndex(
    (step) => step.name === "Independently rebuild the release bundle",
  );
  const compare = steps.findIndex(
    (step) => step.name === "Compare assets in the release roster",
  );
  const verify = steps[compare + 1];
  const violations: string[] = [];
  if (
    !(
      checkout >= 0 &&
      checkout < node &&
      node < rebuild &&
      rebuild < compare
    ) ||
    lookup(job, "env", "TAG") !== "${{ needs.prepare.outputs.tag }}" ||
    lookup(steps[checkout], "with", "ref") !==
      "${{ needs.prepare.outputs.target_sha }}" ||
    !scalarText(steps[checkout]?.uses).startsWith("actions/checkout@") ||
    !scalarText(steps[node]?.uses).startsWith("actions/setup-node@") ||
    lookup(steps[node], "with", "node-version") !== "26.12.1"
  ) {
    violations.push(
      "stage must check out the prepared target and pin maintainer Node before re-derivation",
    );
  }
  if (
    steps[rebuild]?.uses !== "./.github/actions/release" ||
    lookup(steps[rebuild], "with", "command") !== "bundle" ||
    lookup(steps[rebuild], "with", "sha") !==
      "${{ needs.prepare.outputs.target_sha }}" ||
    lookup(steps[rebuild], "with", "repo") !==
      "${{ github.workspace }}/release-target" ||
    steps[rebuild + 1]?.run !==
      "diff --brief --recursive --no-dereference rebuilt release-assets" ||
    steps[compare]?.shell !== "bash" ||
    verify?.uses !== "./.github/actions/release" ||
    lookup(verify, "with", "command") !== "verify" ||
    lookup(verify, "with", "assets") !== "release-assets" ||
    lookup(verify, "with", "tag") !== "${{ needs.prepare.outputs.tag }}" ||
    steps[compare + 2]?.id !== "attest-check" ||
    [
      steps[checkout],
      steps[node],
      steps[rebuild],
      steps[rebuild + 1],
      steps[compare],
      verify,
    ].some(
      (step) =>
        step?.if !== undefined || step?.["continue-on-error"] !== undefined,
    )
  ) {
    violations.push(
      "stage must require re-derivation and bundle verification before reconciling attestations",
    );
  }
  return violations;
}

// Both the Markdown job and its compiled YAML parse as a mapping once their job key is removed.
export function stageIdentityViolations(
  prepare: string,
  stage: string,
): string[] {
  const job = parse(stage);
  const violations: string[] = [];
  for (const [name, producer] of [
    ["target_sha", "needs.prepare.outputs.target_sha"],
    ["state_artifact_id", "steps.upload_state.outputs.artifact-id"],
    ["state_sha256", "steps.record_state.outputs.sha256"],
  ] as const) {
    if (lookup(job, "outputs", name) !== "${{ " + producer + " }}") {
      violations.push(`stage output ${name} must come from ${producer}`);
    }
  }
  if (
    lookup(parse(prepare), "outputs", "target_sha") !==
      "${{ steps.target.outputs.sha }}" ||
    lookup(job, "env", "TARGET_SHA") !==
      "${{ needs.prepare.outputs.target_sha }}"
  ) {
    violations.push(
      "the recorded SHA must come from the verified prepare target",
    );
  }
  const steps: Mapping[] = job.steps;
  const index = steps.findIndex((step) => step.id === "record_state");
  const record = steps[index];
  const upload = steps[index + 1];
  const guard = steps[index + 2];
  const recordScript = scalarText(record?.run).trimEnd();
  const write =
    "'{sha:$sha, tag:$tag, release_id:$release_id, prerelease:$prerelease}' > release-state/state.json\n";
  const digest =
    "test -s release-state/state.json\n" +
    'digest="$(sha256sum < release-state/state.json)"\n' +
    'printf \'sha256=%s\\n\' "${digest%% *}" >> "$GITHUB_OUTPUT"';
  if (
    index < 1 ||
    steps[index - 1]?.id !== "publish" ||
    !recordScript.includes(
      'jq -n --arg sha "$TARGET_SHA" --arg tag "$TAG" --arg release_id "$RELEASE_ID" --argjson prerelease "$PRERELEASE"',
    ) ||
    !recordScript.endsWith(write + digest) ||
    keysOf(record?.env).sort().join(",") !== "PRERELEASE,RELEASE_ID" ||
    lookup(record, "env", "RELEASE_ID") !==
      "${{ steps.publish.outputs.release_id }}" ||
    lookup(record, "env", "PRERELEASE") !== "${{ inputs.prerelease }}" ||
    record?.if !== undefined ||
    record?.["continue-on-error"] !== undefined
  ) {
    violations.push(
      "stage must hash the identity file written after publication",
    );
  }
  if (
    upload?.id !== "upload_state" ||
    !/^actions\/upload-artifact@[0-9a-f]{40}$/.test(scalarText(upload?.uses)) ||
    lookup(upload, "with", "name") !== "release-state" ||
    lookup(upload, "with", "path") !== "release-state/state.json" ||
    lookup(upload, "with", "if-no-files-found") !== "error" ||
    upload?.if !== undefined ||
    upload?.["continue-on-error"] !== undefined
  ) {
    violations.push(
      "stage must upload the hashed identity file and fail on upload errors",
    );
  }
  if (
    lookup(guard, "env", "ARTIFACT_ID") !==
      "${{ steps.upload_state.outputs.artifact-id }}" ||
    scalarText(guard?.run).trim() !== 'test -n "$ARTIFACT_ID"' ||
    guard?.if !== undefined ||
    guard?.["continue-on-error"] !== undefined
  ) {
    violations.push("stage must require the identity upload's artifact ID");
  }
  return violations;
}

// Drops whole-line comments so prose above the next step cannot satisfy or trip a check.
export function withoutYAMLComments(text: string): string {
  return text
    .split("\n")
    .filter((line) => !line.trim().startsWith("#"))
    .join("\n");
}

// Text of the compiled lock between two markers; an empty endMarker takes the rest of the file.
export function lockSection(
  source: string,
  startMarker: string,
  endMarker: string,
): string {
  const start = source.indexOf(startMarker);
  if (start < 0)
    throw new Error(
      `release.lock.yml has no ${JSON.stringify(startMarker)} section`,
    );
  const from = start + startMarker.length;
  if (endMarker === "") return source.slice(from);
  const end = source.indexOf(endMarker, from);
  if (end < 0)
    throw new Error(
      `release.lock.yml has no ${JSON.stringify(endMarker)} section`,
    );
  return source.slice(from, end);
}

// A compiled top-level job bounded by the next top-level job key, not by a neighbour's name.
export function jobKeySection(source: string, job: string): string {
  const marker = `\n  ${job}:\n`;
  const start = source.indexOf(marker);
  if (start < 0)
    throw new Error(
      `release.lock.yml has no ${JSON.stringify(marker)} section`,
    );
  const rest = source.slice(start + marker.length);
  const next = /^ {2}[A-Za-z_][\w-]*:\r?\n/m.exec(rest);
  return next === null ? rest : rest.slice(0, next.index);
}

// The named compiled step, bounded by the next step.
export function compiledStepSection(
  source: string,
  step: string,
  nextStep: string,
): string {
  return lockSection(
    source,
    `\n      - name: ${step}\n`,
    `\n      - name: ${nextStep}\n`,
  );
}

// From the named compiled step to the end of the text.
export function compiledStepToEnd(source: string, step: string): string {
  const start = source.indexOf(`\n      - name: ${step}\n`);
  if (start < 0)
    throw new Error(
      `compiled workflow is missing step ${JSON.stringify(step)}`,
    );
  return source.slice(start);
}

// A compiled job section as its steps; null when no step matches, which a caller must read as "step not found" and not
// skip.
export function splitSteps(section: string): string[] | null {
  const starts = [...section.matchAll(/^ {6}- name: /gm)].map((m) => m.index);
  if (starts.length === 0) return null;
  return starts.map((start, i) =>
    section.slice(start, starts[i + 1] ?? section.length),
  );
}

// The pinned threat-detect installer command on a line, or "".
export function installerInvocation(line: string): string {
  const at = line.indexOf('bash "');
  if (at < 0 || !line.includes("install_threat_detect_binary.sh")) return "";
  return line.slice(at).trim();
}

// Each `uses:` must name a 40-hex commit; local actions carry no ref.
export function assertEveryActionIsSHAPinnedViolations(
  source: string,
): string[] {
  const violations: string[] = [];
  for (const line of source.split("\n")) {
    let trimmed = line.trim();
    if (trimmed.startsWith("- ")) trimmed = trimmed.slice(2);
    if (!trimmed.startsWith("uses:")) continue;
    let ref = trimmed.slice("uses:".length).trim();
    const comment = ref.indexOf(" #");
    if (comment >= 0) ref = ref.slice(0, comment);
    ref = ref.trim();
    if (ref.startsWith("./")) continue;
    const at = ref.indexOf("@");
    if (at < 0) {
      violations.push(
        `action ${JSON.stringify(ref)} has no ref; pin it to a 40-hex commit`,
      );
      continue;
    }
    const sha = ref.slice(at + 1);
    if (!/^[0-9a-f]{40}$/.test(sha)) {
      violations.push(
        `action ${JSON.stringify(ref)} is not pinned to a 40-hex commit`,
      );
    }
  }
  return violations;
}

// The workflow runs only on workflow_dispatch, and its one input, prerelease, is a boolean that defaults to false.
export function releaseTriggerViolations(triggers: string): string[] {
  let doc: unknown;
  try {
    doc = parse(`on:\n${triggers}`);
  } catch (error) {
    if (!(error instanceof YAMLError)) throw error;
    return [`release triggers are not valid YAML: ${error.message}`];
  }
  const on = lookup(doc, "on");
  const names = keysOf(on);
  if (!names.includes("workflow_dispatch") || names.length !== 1) {
    return ["release must run only on workflow_dispatch"];
  }
  const inputs = lookup(on, "workflow_dispatch", "inputs");
  const inputNames = keysOf(inputs);
  const type = lookup(inputs, "prerelease", "type");
  const fallback = lookup(inputs, "prerelease", "default");
  if (
    inputNames.length !== 1 ||
    inputNames[0] !== "prerelease" ||
    type !== "boolean" ||
    fallback !== false
  ) {
    return [
      "release must take exactly one dispatch input, prerelease, a boolean that defaults to false",
    ];
  }
  return [];
}

// The compiled lock's detection job must install the pinned detector on the skipped-run path and stay strict.
export function detectionBootstrapViolations(lockSource: string): string[] {
  const detection = jobKeySection(lockSource, "detection");
  const violations: string[] = [];

  const skipCondition =
    "!cancelled() && steps.detection_guard.outputs.run_detection != 'true'";
  const skippedRunStep =
    (splitSteps(detection) ?? []).find((step) =>
      step.includes(skipCondition),
    ) ?? "";
  if (skippedRunStep === "") {
    violations.push("detector bootstrap must cover the skipped-run path");
  }

  const installerLines: string[] = [];
  for (const line of detection.split("\n")) {
    const invocation = installerInvocation(line);
    if (invocation !== "") installerLines.push(invocation);
  }
  const [firstInstaller = "", ...otherInstallers] = installerLines;
  if (installerLines.length < 2) {
    violations.push(
      `compiled detection installs the threat-detect binary ${installerLines.length} time(s); want at least 2 (bootstrap + main install, so a version bump can't miss one)`,
    );
  } else {
    for (const line of otherInstallers) {
      if (line !== firstInstaller) {
        violations.push(
          `threat-detect binary installer lines disagree: ${JSON.stringify(firstInstaller)} vs ${JSON.stringify(line)}`,
        );
        break;
      }
    }
  }

  // "A skip-condition step exists" and "two identical installer calls exist" can both hold while the call sits outside
  // the skipped-run step, so the step's own body must carry the call.
  if (skippedRunStep !== "") {
    if (
      !installerLines.some((invocation) => skippedRunStep.includes(invocation))
    ) {
      violations.push(
        "the skipped-run step must itself invoke the pinned threat-detect installer, not just exist alongside one elsewhere in the job",
      );
    }
  }

  if (!detection.includes("if: always()")) {
    violations.push("detection must keep an unconditional step");
  }
  if (!detection.includes('GH_AW_DETECTION_CONTINUE_ON_ERROR: "false"')) {
    violations.push("detection must remain strict (continue-on-error: false)");
  }
  return violations;
}

const quote = (text: string): string => JSON.stringify(text);

// Checks the security and wiring invariants of release.md's source and returns every violation, instead of stopping at
// the first. The real file must return none; a mutated copy must return at least one, so a relaxed rewrite of this
// table cannot pass silently. Prose and generated step names stay out of the tables below: they break on a harmless
// rename.
export function releaseWorkflowContractViolations(source: string): string[] {
  const level = workflowJobSection(source, "level", "approve-major");
  const approve = workflowJobSection(source, "approve-major", "release");
  const releaseJob = workflowJobSection(source, "release", "prepare");
  const prepare = workflowJobSection(source, "prepare", "stage");
  const stage = workflowJobSection(source, "stage", "agent");
  const agent = workflowJobSection(source, "agent", "");
  const safe = workflowSection(source, "\nsafe-outputs:\n", "\njobs:\n");
  const writer = workflowJobSection(source, "write-notes", "level");
  const candidateUpload = workflowStepToEnd(
    prepare,
    "Upload release candidate",
  );
  const candidateDownload = workflowSection(
    stage,
    "\n            - name: Download release candidate\n",
    "\n            - name: Prepare independent bundle directory\n",
  );

  const violations: string[] = [];
  const tables: [string, string, string[]][] = [
    [
      "level",
      level,
      [
        "contents: read",
        "command: resume-check",
        "npm ci --ignore-scripts",
        "semantic-release.js --dry-run --no-ci -e ./.github/release/.releaserc.json",
        // A draft that a failed stage left behind is not a published release.
        "--limit 1000 --exclude-drafts",
      ],
    ],
    [
      "approve-major",
      approve,
      ["environment: github-release", "needs.level.outputs.level == 'major'"],
    ],
    [
      "release",
      releaseJob,
      [
        "environment: release-commit",
        "github.ref == 'refs/heads/main'",
        "npm ci --ignore-scripts",
        "actions/create-github-app-token@",
        "AGX_MAJOR_APPROVED",
        "persist-credentials: false",
        "semantic-release.js --no-ci -e ./.github/release/.releaserc.json",
        "--limit 1000 --exclude-drafts",
      ],
    ],
    [
      "target validation",
      prepare,
      [
        "needs: [release, level]",
        "TARGET_SHA: ${{ needs.release.outputs.target_sha }}",
        "tag: ${{ needs.release.outputs.tag }}",
        "^[0-9a-f]{40}$",
        "git merge-base --is-ancestor",
        "persist-credentials: false",
        "fetch-depth: 0",
        "credential.helper=!f()",
        "./.github/actions/conformance",
        "path: release-target",
        "uses: ./.github/actions/release",
        "command: check-commit",
        "command: bundle",
      ],
    ],
    [
      "draft staging",
      stage,
      [
        "needs: [prepare, level]",
        "pull-requests: read",
        "id-token: write",
        "attestations: write",
        "diff --brief --recursive --no-dereference rebuilt release-assets",
        "actions/attest-build-provenance@",
        "subject-path: rebuilt/*",
        "uses: ./.github/actions/release",
        "command: context",
        "command: stage",
        "notes: release-context/baseline-notes.md",
        "token: ${{ github.token }}",
        "release_id:",
        "published:",
        "release-state",
      ],
    ],
    [
      "agent",
      agent,
      [
        "needs: [prepare, stage]",
        "needs.stage.outputs.published == 'false'",
        "contents: read",
        "copilot-requests: write",
        "name: release-context",
      ],
    ],
    [
      "notes boundary",
      safe,
      [
        "update-release-notes:",
        "needs.detection.result == 'success'",
        "body:",
        "type: string",
        "permissions: {}",
      ],
    ],
    [
      "candidate retention",
      candidateUpload,
      [
        "name: release-candidate",
        "path: release-assets/",
        "retention-days: 30",
      ],
    ],
    [
      "candidate download",
      candidateDownload,
      ["name: release-candidate", "path: release-assets"],
    ],
  ];
  for (const [label, section, wants] of tables) {
    for (const want of wants) {
      if (!section.includes(want))
        violations.push(`${label} missing ${quote(want)}`);
    }
  }
  if (countOf(source, "retention-days: 30") !== 1) {
    violations.push(
      "only the release-candidate upload may set retention-days: 30",
    );
  }
  if (candidateDownload.includes("if:")) {
    violations.push("release-candidate download must be unconditional");
  }
  if (candidateDownload.includes("continue-on-error:")) {
    violations.push("release-candidate download failure must be fatal");
  }

  // Only stage and the notes writer can change repository content with their job token.
  const contentsWrite = /^\s*contents: write\s*$/m;
  for (const [name, section] of [
    ["level", level],
    ["approve-major", approve],
    ["release", releaseJob],
    ["prepare", prepare],
    ["agent", agent],
  ] as const) {
    if (contentsWrite.test(section))
      violations.push(`${name} must not hold contents: write`);
  }

  // A skipped approve-major counts as a skipped ancestor, and the implicit success() then skips every job after it.
  // Each downstream condition opts out with !cancelled() and tests its direct upstream instead.
  for (const [name, section, cond] of [
    ["prepare", prepare, "!cancelled() && needs.release.result == 'success'"],
    ["stage", stage, "!cancelled() && needs.prepare.result == 'success'"],
    [
      "agent",
      agent,
      "!cancelled() && needs.stage.result == 'success' && needs.stage.outputs.published == 'false'",
    ],
  ] as const) {
    if (workflowJobCondition(section) !== "${{ " + cond + " }}") {
      violations.push(
        `${name} must run after a skipped approve-major: want if: ` +
          "${{ " +
          cond +
          " }}",
      );
    }
  }

  // A major release runs only once a reviewer approved it: release waits for level and for approve-major to succeed
  // or be skipped, and tells the plugin whether the approval happened.
  const releaseCondition = workflowJobCondition(releaseJob);
  if (!releaseCondition.includes("needs.level.result == 'success'")) {
    violations.push("release must run only after level succeeded");
  }
  if (
    !releaseCondition.includes(
      'contains(fromJSON(\'["success","skipped"]\'), needs.approve-major.result)',
    )
  ) {
    violations.push(
      "release must run only after approve-major succeeded or was skipped",
    );
  }
  if (
    !workflowStepBody(releaseJob, "Run semantic-release").includes(
      "AGX_MAJOR_APPROVED: ${{ needs.approve-major.result == 'success' && 'true' || 'false' }}\n",
    )
  ) {
    violations.push(
      "release must derive AGX_MAJOR_APPROVED from the approve-major result",
    );
  }
  // A dispatch reruns the head: an operator cannot pick the target or the version. The one input chooses the
  // publication state, never the commit.
  const triggers = workflowSection(source, "\non:\n", "\nconcurrency:\n");
  violations.push(...releaseTriggerViolations(triggers));
  if (
    !source.includes("\nrun-name: Agentics Release #${{ github.run_number }}\n")
  ) {
    violations.push("run-name must identify the run");
  }
  if (prepare.includes("command: version")) {
    violations.push(
      "prepare must take the tag from the release job, not compute a version",
    );
  }

  // The release job pushes with a token that bypasses the main ruleset, so prepare, which holds no token, checks the
  // commit before it bundles it. Each input is pinned inside the check's own step, because the bundle step carries
  // identical sha and tag lines that would satisfy a search of the whole job.
  const commitCheck = withoutYAMLComments(
    workflowStepContaining(prepare, "command: check-commit"),
  );
  for (const want of [
    "uses: ./.github/actions/release\n",
    "sha: ${{ steps.target.outputs.sha }}\n",
    "tag: ${{ needs.release.outputs.tag }}\n",
    "parent: ${{ steps.target.outputs.parent }}\n",
    "author: ${{ needs.release.outputs.author }}\n",
  ]) {
    if (!commitCheck.includes(want)) {
      violations.push(
        `release commit check missing ${quote(want.replace(/\n$/, ""))}`,
      );
    }
  }
  for (const skip of ["if:", "continue-on-error:"]) {
    if (commitCheck.includes(skip)) {
      violations.push(`the release commit check must not carry ${quote(skip)}`);
    }
  }
  const ancestry = prepare.indexOf("git merge-base --is-ancestor");
  const commitCheckAt = prepare.indexOf("command: check-commit");
  const bundleAt = prepare.indexOf("command: bundle");
  if (
    ancestry < 0 ||
    commitCheckAt < 0 ||
    bundleAt < 0 ||
    !(ancestry < commitCheckAt && commitCheckAt < bundleAt)
  ) {
    violations.push(
      "prepare must verify the release commit after the main-ancestry check and before it bundles it",
    );
  }

  // The check runs its verifier from the workspace, so the workspace must be a reviewed commit when the check runs:
  // the pushed head, or its first parent when a resumed run's head is the release commit. A release job that edits
  // .github/tools/src/release or the local action would otherwise pass its own check. The target gets its own checkout after the check.
  const verifierRef = "ref: ${{ github.sha }}\n";
  const targetRef = "ref: ${{ steps.shape.outputs.sha }}\n";
  const firstParent = 'git checkout --detach "$GITHUB_SHA^1"';
  for (const [label, ref] of [
    ["verifier", verifierRef],
    ["target", targetRef],
  ] as const) {
    const step = withoutYAMLComments(workflowStepContaining(prepare, ref));
    for (const want of [
      "uses: actions/checkout@",
      "fetch-depth: 0\n",
      "persist-credentials: false\n",
    ]) {
      if (!step.includes(want)) {
        violations.push(
          `${label} checkout missing ${quote(want.replace(/\n$/, ""))}`,
        );
      }
    }
  }
  const verifierAt = prepare.indexOf(verifierRef);
  if (verifierAt < 0 || commitCheckAt < 0 || verifierAt > commitCheckAt) {
    violations.push(
      "prepare must check out the pushed head before the release commit check, so the verifier code is reviewed",
    );
  }
  const targetAt = prepare.indexOf(targetRef);
  if (
    targetAt < 0 ||
    commitCheckAt < 0 ||
    bundleAt < 0 ||
    !(commitCheckAt < targetAt && targetAt < bundleAt)
  ) {
    violations.push(
      "prepare must check out the release target only after the release commit check and before the bundle",
    );
  }
  const firstParentStep = withoutYAMLComments(
    workflowStepContaining(prepare, firstParent),
  );
  const firstParentAt = prepare.indexOf(firstParent);
  if (
    !firstParentStep.includes("if: needs.level.outputs.mode != 'analyze'\n") ||
    verifierAt < 0 ||
    commitCheckAt < 0 ||
    !(verifierAt < firstParentAt && firstParentAt < commitCheckAt)
  ) {
    violations.push(
      "prepare must move the verifier to the release commit's first parent on a resumed run, before the release commit check",
    );
  }

  for (const [name, job, target] of [
    ["prepare", prepare, targetRef],
    ["stage", stage, "ref: ${{ needs.prepare.outputs.target_sha }}\n"],
  ] as const) {
    const verifier = withoutYAMLComments(
      workflowStepContaining(job, verifierRef),
    );
    const data = withoutYAMLComments(workflowStepContaining(job, target));
    const bundle = withoutYAMLComments(
      workflowStepContaining(job, "command: bundle"),
    );
    if (
      !verifier.includes("uses: actions/checkout@") ||
      verifier.includes("path:") ||
      !verifier.includes("persist-credentials: false") ||
      !verifier.includes("fetch-depth: 0") ||
      !data.includes("path: release-target\n") ||
      !data.includes("persist-credentials: false") ||
      !data.includes("fetch-depth: 0") ||
      !bundle.includes("uses: ./.github/actions/release") ||
      bundle.includes("checkout:") ||
      !bundle.includes("repo: ${{ github.workspace }}/release-target\n")
    )
      violations.push(
        `${name} must build the target with the reviewed workspace tools`,
      );
  }
  const rebuild = stage.indexOf("command: bundle");
  if (!(
    stage.indexOf(verifierRef) >= 0 &&
    stage.indexOf(verifierRef) < stage.indexOf(firstParent) &&
    stage.indexOf(firstParent) < stage.indexOf("git rev-list --parents") &&
    stage.indexOf("git rev-list --parents") <
      stage.indexOf("git merge-base --is-ancestor") &&
    stage.indexOf("git merge-base --is-ancestor") <
      stage.indexOf("path: release-target") &&
    stage.indexOf("path: release-target") < rebuild &&
    rebuild < stage.indexOf("diff --brief")
  ))
    violations.push(
      "stage must bind the reviewed parent before rebuilding and comparing the target",
    );
  if (
    !workflowStepContaining(stage, "command: stage").includes(
      "assets: rebuilt\n",
    ) ||
    !workflowStepContaining(stage, "command: bundle").includes(
      "out: ${{ github.workspace }}/rebuilt\n",
    )
  )
    violations.push("stage must publish the independently rebuilt directory");

  // stage builds the notes from the baseline semantic-release wrote in the release job. The artifact lands under
  // release-baseline-in in the workspace root, where the context step resolves its relative input.
  const baselineDownload = withoutYAMLComments(
    workflowStepContaining(stage, "name: release-baseline\n"),
  );
  for (const want of [
    "uses: actions/download-artifact@",
    "name: release-baseline\n",
    "path: release-baseline-in\n",
  ]) {
    if (!baselineDownload.includes(want)) {
      violations.push(
        `baseline download missing ${quote(want.replace(/\n$/, ""))}`,
      );
    }
  }
  const contextStep = withoutYAMLComments(
    workflowStepContaining(stage, "command: context"),
  );
  if (!contextStep.includes("baseline: release-baseline-in/baseline.md\n")) {
    violations.push(
      'context step missing "baseline: release-baseline-in/baseline.md"',
    );
  }
  const baselineAt = stage.indexOf("name: release-baseline\n");
  const contextAt = stage.indexOf("command: context");
  if (baselineAt < 0 || contextAt < 0 || baselineAt > contextAt) {
    violations.push(
      "stage must download the baseline notes before it builds the context",
    );
  }

  // The dispatch input chooses the publication state, so stage must receive it, the prepared release identity must
  // record it, and the notes job must refuse a record that is not a boolean.
  const stageStep = withoutYAMLComments(
    workflowStepContaining(stage, "command: stage"),
  );
  if (
    !stageStep
      .split("\n")
      .includes("                  prerelease: ${{ inputs.prerelease }}")
  ) {
    violations.push("the stage step must pass the dispatch input prerelease");
  }
  const recordStep = withoutYAMLComments(
    workflowStepBody(stage, "Record prepared release identity"),
  );
  for (const want of [
    "PRERELEASE: ${{ inputs.prerelease }}\n",
    '--argjson prerelease "$PRERELEASE"',
    "prerelease:$prerelease",
  ]) {
    if (!recordStep.includes(want)) {
      violations.push(
        `the prepared release identity must record the mode: missing ${quote(want)}`,
      );
    }
  }
  if (
    !withoutYAMLComments(writer).includes(
      "RELEASE_STATE_FILE: release-state/state.json\n",
    )
  ) {
    violations.push("the notes writer must use the bound artifact input");
  }
  if (
    !withoutYAMLComments(safe).includes(
      'cp -- "$GH_AW_AGENT_OUTPUT" notes-handoff/agent_output.json',
    )
  ) {
    violations.push("the handoff must use the detector-approved envelope");
  }

  // The notes baseline comes from semantic-release, so no prose or input description may still attribute it to
  // GitHub's note generator.
  for (const stale of [
    "GitHub-generated notes",
    "GitHub's generated changelog",
    "GitHub's generated notes",
  ]) {
    if (source.includes(stale)) {
      violations.push(
        `release.md still names ${quote(stale)}, but semantic-release writes the baseline`,
      );
    }
  }

  // Environments gate two jobs and no others: approve-major waits for a reviewer, and release holds the App secrets.
  if (countOf(source, "environment:") !== 2) {
    violations.push(
      "exactly approve-major and release may name an environment",
    );
  }
  for (const section of [level, prepare, stage, agent]) {
    if (section.includes("environment:")) {
      violations.push("only approve-major and release may name an environment");
    }
  }
  for (const section of [level, approve, prepare, stage, agent]) {
    if (withoutYAMLComments(section).includes("secrets.")) {
      violations.push("secrets reach only the release job");
    }
  }
  // npm code runs before the token exists: the install step must precede the token mint, no token reaches the install
  // step, and the App token reaches exactly two steps.
  const install = releaseJob.indexOf("npm ci --ignore-scripts");
  const mint = releaseJob.indexOf("actions/create-github-app-token@");
  if (install < 0 || mint < 0 || install > mint) {
    violations.push(
      "release must install npm packages before it mints the App token",
    );
  }
  for (const [name, section] of [
    ["level", level],
    ["release", releaseJob],
  ] as const) {
    const step = withoutYAMLComments(
      workflowStepBody(section, "Install release tooling"),
    );
    if (step === "" || step.includes("token") || step.includes("secrets.")) {
      violations.push(
        `${name}: the npm install step must exist and run with no token in its environment`,
      );
    }
  }
  const minted = "steps.app-token.outputs.token";
  if (countOf(releaseJob, minted) !== 2) {
    violations.push(
      "the App token may reach only the semantic-release step and the result step",
    );
  }
  if (!workflowStepBody(releaseJob, "Run semantic-release").includes(minted)) {
    violations.push("the semantic-release step must receive the App token");
  }
  if (
    !workflowStepBody(releaseJob, "Record the release commit and tag").includes(
      minted,
    )
  ) {
    violations.push("the result step must receive the App token");
  }
  for (const retired of [
    "\n    publish:\n",
    "command: publish",
    "release-id:",
  ]) {
    if (source.includes(retired))
      violations.push(`retired publication path ${quote(retired)}`);
  }

  // The bytes are attested before the first one reaches GitHub: re-derive, verify, then attest, then stage. A consumer
  // verifies the attestation, so it must exist before the pre-release is published. Anchored on the re-derive step's
  // verification line rather than its step name, which renames more often than the invariant changes.
  const rederive = stage.indexOf(
    "diff --brief --recursive --no-dereference rebuilt release-assets",
  );
  const verified = stage.indexOf("command: verify");
  const attest = stage.indexOf("actions/attest-build-provenance@");
  const staged = stage.indexOf("command: stage");
  if (
    rederive < 0 ||
    attest < 0 ||
    staged < 0 ||
    !(rederive < attest && attest < staged)
  ) {
    violations.push("stage must re-derive, attest, then stage, in that order");
  }
  if (
    verified < 0 ||
    rederive < 0 ||
    attest < 0 ||
    !(rederive < verified && verified < attest)
  ) {
    violations.push("stage must verify the bundle before attestation");
  }

  for (const obsolete of [
    "select-notes:",
    "release-final-notes",
    "upload_artifact",
    "pr-allowlist.txt",
    "update-release:",
    "release-bundle.sh",
    "release-context.sh",
    "release-publish.sh",
  ]) {
    if (source.includes(obsolete))
      violations.push(`obsolete notes path ${quote(obsolete)}`);
  }
  if (
    !source.includes("group: agentics-release") ||
    !source.includes("cancel-in-progress: false")
  ) {
    violations.push(
      "draft assembly and publication must share workflow serialization",
    );
  }
  violations.push(...assertEveryActionIsSHAPinnedViolations(source));
  return violations;
}

// Inference is billed to the organization through the Actions token: only agent and detection receive copilot-requests:
// write, and neither depends on the inference PAT secret.
export function orgBillingViolations(compiled: string): string[] {
  const violations: string[] = [];
  if (countOf(compiled, "      copilot-requests: write\n") !== 2) {
    violations.push(
      "only agent and detection must receive organization inference permission",
    );
  }
  if (compiled.includes("secrets.COPILOT_GITHUB_TOKEN")) {
    violations.push("workflow still depends on the inference PAT secret");
  }
  for (const job of ["agent", "detection"]) {
    const section = jobKeySection(compiled, job);
    if (!section.includes("      copilot-requests: write\n")) {
      violations.push(`${job} lacks organization inference permission`);
    }
    if (
      !section.includes("          COPILOT_GITHUB_TOKEN: ${{ github.token }}\n")
    ) {
      violations.push(
        `${job} does not authenticate inference with the Actions token`,
      );
    }
  }
  return violations;
}

// Checks the compiled release.lock.yml.
export function compiledReleaseLockViolations(compiled: string): string[] {
  const violations: string[] = [];
  const prepare = jobKeySection(compiled, "prepare");
  const stage = jobKeySection(compiled, "stage");
  const upload = compiledStepToEnd(prepare, "Upload release candidate");
  const download = compiledStepSection(
    stage,
    "Download release candidate",
    "Prepare independent bundle directory",
  );
  if (
    !upload.includes("name: release-candidate") ||
    !upload.includes("path: release-assets/") ||
    !upload.includes(
      "retention-days: ${{ vars.GH_AW_DEFAULT_ARTIFACT_RETENTION_DAYS || '30' }}",
    )
  ) {
    violations.push(
      "compiled release-candidate upload must retain the candidate for 30 days",
    );
  }
  if (download.includes("if:")) {
    violations.push(
      "compiled release-candidate download must be unconditional",
    );
  }
  if (download.includes("continue-on-error:")) {
    violations.push(
      "compiled release-candidate download failure must be fatal",
    );
  }
  const update = jobKeySection(compiled, "update_release_notes");
  if (update.includes("(${{")) {
    violations.push(
      "compiled safe-output condition contains nested expression markers",
    );
  }
  if (
    !update.includes("needs.detection.result == 'success'") ||
    !update.includes("needs.detection.outputs.detection_success == 'true'") ||
    !update.includes("GH_AW_AGENT_OUTPUT:")
  ) {
    violations.push(
      "compiled notes handoff lacks detection gate or safe-output transport",
    );
  }
  if (
    lockSection(compiled, "\n  agent:\n", "\n  conclusion:\n").includes(
      "contents: write",
    )
  ) {
    violations.push("compiled agent has write permission");
  }
  if (
    !compiled.includes("environment: release-commit") ||
    !compiled.includes("environment: github-release")
  ) {
    violations.push("compiled workflow lost an environment");
  }
  return violations;
}

// The compiled lock, which is what GitHub runs, has no push trigger and carries the one prerelease input; gh-aw adds
// its own aw_context input to every compiled workflow_dispatch.
export function lockTriggerViolations(lock: string): string[] {
  const triggers = lockSection(lock, "\non:\n", "\npermissions:");
  let doc: unknown;
  try {
    doc = parse(`on:\n${triggers}`);
  } catch (error) {
    if (!(error instanceof YAMLError)) throw error;
    return [`parse the compiled triggers: ${error.message}`];
  }
  const on = lookup(doc, "on");
  const names = keysOf(on);
  if (!names.includes("workflow_dispatch") || names.length !== 1) {
    return [
      `compiled release.lock.yml must run only on workflow_dispatch, got ${names.length} triggers`,
    ];
  }
  const inputs = lookup(on, "workflow_dispatch", "inputs");
  const inputNames = keysOf(inputs).sort();
  if (inputNames.join(",") !== "aw_context,prerelease") {
    return [
      `compiled dispatch inputs = [${inputNames.join(" ")}], want aw_context and prerelease`,
    ];
  }
  const type = lookup(inputs, "prerelease", "type");
  const fallback = lookup(inputs, "prerelease", "default");
  if (type !== "boolean" || fallback !== false) {
    return [
      `compiled prerelease input is type ${quote(scalarText(type))} default ${scalarText(fallback)}, want a boolean that defaults to false`,
    ];
  }
  return [];
}
