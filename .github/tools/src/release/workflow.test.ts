import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import {
  cpSync,
  mkdirSync,
  readFileSync,
  rmSync,
  symlinkSync,
  writeFileSync,
} from "node:fs";
import { join } from "node:path";
import { parse } from "yaml";
import {
  checksumAssetNames,
  expectedAssetNames,
  fileDigest,
  pluginTarballName,
  verifyChecksums,
} from "./assets.ts";
import {
  commitFiles,
  initRepo,
  posix,
  scratchDir,
  sh,
} from "./testing/support.ts";
import { STAGE_TAG, writeSyntheticBundle } from "./testing/release-fake.ts";
import { describe, test } from "node:test";
import { readRepoText, repoRoot } from "./testing/repo.ts";
import {
  compiledReleaseLockViolations,
  detectionBootstrapViolations,
  jobKeySection,
  lockSection,
  lockTriggerViolations,
  orgBillingViolations,
  releaseWorkflowContractViolations,
  replaceFirst,
  replaceOnce,
  stageDerivationViolations,
  stageIdentityViolations,
  withoutYAMLComments,
  workflowJobSection,
  workflowStepBody,
} from "./testing/workflow.ts";

const md = readRepoText(".github/workflows/release.md");
const lock = readRepoText(".github/workflows/release.lock.yml");

test("the compiled conclusion reports a writer failure after successful handoff", () => {
  const conclusion = jobKeySection(lock, "conclusion");
  const line = conclusion.match(/^      GH_AW_JOB_RESULTS: (.+)$/m);
  assert.ok(line);
  const template: string = JSON.parse(line[1]!);
  const results = JSON.parse(
    template.replace(/\$\{\{ needs\.([\w-]+)\.result \}\}/g, (_expr, job) =>
      job === "write-notes" ? "failure" : "success",
    ),
  );
  assert.deepEqual(results.update_release_notes, { result: "success" });
  assert.deepEqual(results["write-notes"], { result: "failure" });
  assert.match(conclusion, /- write-notes\n/);
});

for (const [file, prepare, stage] of [
  [
    "release.md",
    workflowJobSection(md, "prepare", "stage"),
    workflowJobSection(md, "stage", "agent"),
  ],
  [
    "release.lock.yml",
    jobKeySection(lock, "prepare"),
    jobKeySection(lock, "stage"),
  ],
] as const) {
  test(`${file} requires target-bound re-derivation and verification before attestation`, () => {
    assert.deepEqual(stageDerivationViolations(stage), []);
    for (const [old, replacement, want] of [
      [
        "TAG: ${{ needs.prepare.outputs.tag }}",
        "TAG: ${{ inputs.tag }}",
        /prepared target/,
      ],
      [
        "ref: ${{ needs.prepare.outputs.target_sha }}",
        "ref: ${{ github.sha }}",
        /prepared target/,
      ],
      ["node-version: 26.12.1", "node-version: latest", /pin maintainer Node/],
      ["command: verify", "command: context", /bundle verification/],
      ["assets: release-assets", "assets: rebuilt", /bundle verification/],
    ] as const) {
      assert.match(
        stageDerivationViolations(replaceFirst(stage, old, replacement)).join(
          "\n",
        ),
        want,
      );
    }
  });
  test(`${file} binds stage outputs to the verified target and uploaded identity`, () => {
    assert.deepEqual(stageIdentityViolations(prepare, stage), []);
  });
  test(`${file} rejects substituted identity producers and bytes`, () => {
    for (const [old, replacement, want] of [
      [
        "target_sha: ${{ needs.prepare.outputs.target_sha }}",
        "target_sha: ${{ inputs.target_sha }}",
        "stage output target_sha must come from needs.prepare.outputs.target_sha",
      ],
      [
        "state_artifact_id: ${{ steps.upload_state.outputs.artifact-id }}",
        "state_artifact_id: ${{ needs.agent.outputs.id }}",
        "stage output state_artifact_id must come from steps.upload_state.outputs.artifact-id",
      ],
      [
        "state_sha256: ${{ steps.record_state.outputs.sha256 }}",
        "state_sha256: ${{ steps.upload_state.outputs.artifact-digest }}",
        "stage output state_sha256 must come from steps.record_state.outputs.sha256",
      ],
      [
        "sha256sum < release-state/state.json",
        "sha256sum < release-context/baseline-notes.md",
        "stage must hash the identity file written after publication",
      ],
      [
        '--arg sha "$TARGET_SHA"',
        '--arg sha "$AGENT_SHA"',
        "stage must hash the identity file written after publication",
      ],
      [
        "path: release-state/state.json",
        "path: release-state/other.json",
        "stage must upload the hashed identity file and fail on upload errors",
      ],
      [
        "if-no-files-found: error",
        "if-no-files-found: warn",
        "stage must upload the hashed identity file and fail on upload errors",
      ],
      [
        'test -n "$ARTIFACT_ID"',
        "true",
        "stage must require the identity upload's artifact ID",
      ],
    ] as const) {
      assert.deepEqual(
        stageIdentityViolations(prepare, replaceOnce(stage, old, replacement)),
        [want],
      );
    }
    assert.deepEqual(
      stageIdentityViolations(
        replaceOnce(
          prepare,
          "target_sha: ${{ steps.target.outputs.sha }}",
          "target_sha: ${{ steps.shape.outputs.sha }}",
        ),
        stage,
      ),
      ["the recorded SHA must come from the verified prepare target"],
    );
  });
}

describe("only agent and detection bill inference to the organization", () => {
  for (const name of ["release", "backlog-grooming"]) {
    test(name, () => {
      const compiled = readRepoText(`.github/workflows/${name}.lock.yml`);
      assert.deepEqual(orgBillingViolations(compiled), []);
    });
  }
});

// Each mutation breaks one clause of the billing rule and must be named by exactly that clause.
test("a billing change that breaks the organization rule is reported", () => {
  const compiled = readRepoText(".github/workflows/release.lock.yml");
  const agentPermission = "      copilot-requests: write\n";
  const cases: [string, string, string[]][] = [
    [
      "a third job receives the permission",
      replaceOnce(
        compiled,
        "\n  prepare:\n",
        "\n  prepare:\n    permissions:\n" + agentPermission,
      ),
      [
        "only agent and detection must receive organization inference permission",
      ],
    ],
    [
      "the PAT secret returns",
      replaceFirst(
        compiled,
        "          COPILOT_GITHUB_TOKEN: ${{ github.token }}\n",
        "          COPILOT_GITHUB_TOKEN: ${{ secrets.COPILOT_GITHUB_TOKEN }}\n",
      ),
      [
        "workflow still depends on the inference PAT secret",
        "agent does not authenticate inference with the Actions token",
      ],
    ],
  ];
  for (const [label, mutated, want] of cases) {
    assert.notEqual(mutated, compiled, `${label}: the mutation had no effect`);
    assert.deepEqual(orgBillingViolations(mutated), want, label);
  }
  const detection = lockSection(compiled, "\n  detection:\n", "\n  level:\n");
  const withoutPermission = replaceFirst(detection, agentPermission, "");
  assert.notEqual(withoutPermission, detection);
  assert.deepEqual(
    orgBillingViolations(replaceFirst(compiled, detection, withoutPermission)),
    [
      "only agent and detection must receive organization inference permission",
      "detection lacks organization inference permission",
    ],
  );
});

describe("the release workflow contract", () => {
  test("release.md holds its contract", () => {
    assert.deepEqual(releaseWorkflowContractViolations(md), []);
  });

  test("release.lock.yml holds the compiled contract", () => {
    assert.deepEqual(compiledReleaseLockViolations(lock), []);
  });
});

// Each case pins one compiled check: removing the check lets its mutation through and fails the case.
test("a compiled lock change that breaks the contract is reported", () => {
  const updateAt = lock.indexOf("\n  update_release_notes:\n");
  const inUpdate = (old: string, replacement: string): string =>
    lock.slice(0, updateAt) +
    replaceFirst(lock.slice(updateAt), old, replacement);
  const cases: [string, string, string][] = [
    [
      "a shorter retention",
      replaceOnce(
        lock,
        "          retention-days: ${{ vars.GH_AW_DEFAULT_ARTIFACT_RETENTION_DAYS || '30' }}\n\n  release:\n",
        "          retention-days: 1\n\n  release:\n",
      ),
      "compiled release-candidate upload must retain the candidate for 30 days",
    ],
    [
      "an optional candidate download",
      replaceOnce(
        lock,
        "      - name: Download release candidate\n",
        "      - name: Download release candidate\n        if: false\n",
      ),
      "compiled release-candidate download must be unconditional",
    ],
    [
      "a candidate download that may fail",
      replaceOnce(
        lock,
        "      - name: Download release candidate\n",
        "      - name: Download release candidate\n        continue-on-error: true\n",
      ),
      "compiled release-candidate download failure must be fatal",
    ],
    [
      "a nested expression marker in the safe-output condition",
      inUpdate("      (!cancelled()) &&", "      (${{ !cancelled() }}) &&"),
      "compiled safe-output condition contains nested expression markers",
    ],
    [
      "no successful detection output required for the handoff",
      inUpdate(
        "needs.detection.outputs.detection_success == 'true'",
        "needs.detection.outputs.detection_success != 'false'",
      ),
      "compiled notes handoff lacks detection gate or safe-output transport",
    ],
    [
      "no safe-output transport",
      lock.slice(0, updateAt) +
        lock
          .slice(updateAt)
          .split("GH_AW_AGENT_OUTPUT:")
          .join("GH_AW_AGENT_OUT:"),
      "compiled notes handoff lacks detection gate or safe-output transport",
    ],
    [
      "a write permission on the agent",
      replaceFirst(
        lock,
        "\n  agent:\n",
        "\n  agent:\n    permissions:\n      contents: write\n",
      ),
      "compiled agent has write permission",
    ],
    [
      "a lost environment",
      replaceOnce(lock, "    environment: github-release\n", ""),
      "compiled workflow lost an environment",
    ],
  ];
  for (const [label, mutated, want] of cases) {
    assert.notEqual(mutated, lock, `${label}: the mutation had no effect`);
    assert.deepEqual(compiledReleaseLockViolations(mutated), [want], label);
  }
});

test("the compiled lock runs only on workflow_dispatch with the prerelease input", () => {
  assert.deepEqual(lockTriggerViolations(lock), []);
});

test("a compiled trigger change that breaks the dispatch contract is reported", () => {
  const cases: [string, string, RegExp][] = [
    [
      "a push trigger",
      replaceOnce(
        lock,
        "\non:\n  workflow_dispatch:\n",
        "\non:\n  push:\n    branches: [main]\n  workflow_dispatch:\n",
      ),
      /must run only on workflow_dispatch, got 2 triggers/,
    ],
    [
      "a third input",
      replaceOnce(
        lock,
        "    inputs:\n      aw_context:\n",
        "    inputs:\n      target_sha:\n        type: string\n      aw_context:\n",
      ),
      /compiled dispatch inputs = \[aw_context prerelease target_sha\]/,
    ],
    [
      "a string input",
      replaceOnce(lock, "        type: boolean\n", "        type: string\n"),
      /compiled prerelease input is type "string"/,
    ],
    [
      "a true default",
      replaceOnce(lock, "        default: false\n", "        default: true\n"),
      /want a boolean that defaults to false/,
    ],
  ];
  for (const [label, mutated, want] of cases) {
    assert.notEqual(mutated, lock, `${label}: the mutation had no effect`);
    const got = lockTriggerViolations(mutated);
    assert.equal(got.length, 1, label);
    assert.match(got[0] ?? "", want, label);
  }
});

test("the compiled detection job installs the detector on every path", () => {
  assert.deepEqual(detectionBootstrapViolations(lock), []);
});

// The mode check must accept either input order.
describe("the stage mode check accepts either input order", () => {
  const stage = workflowJobSection(md, "stage", "agent");
  const token = "                  token: ${{ github.token }}\n";
  const mode = "                  prerelease: ${{ inputs.prerelease }}\n";
  const cases: { name: string; fields: string; want: string[] }[] = [
    { name: "mode last", fields: token + mode, want: [] },
    { name: "mode before token", fields: mode + token, want: [] },
    {
      name: "mode removed",
      fields: token,
      want: ["the stage step must pass the dispatch input prerelease"],
    },
  ];
  for (const { name, fields, want } of cases) {
    test(name, () => {
      const modifiedStage = replaceOnce(stage, token + mode, fields);
      const modified = replaceOnce(md, stage, modifiedStage);
      assert.deepEqual(releaseWorkflowContractViolations(modified), want);
    });
  }
});

// A comment between steps falls inside the previous step's
// text, so the no-token check on the install steps must read the keys and not the comment.
test("a comment between steps does not trip the install-step token check", () => {
  let source = md;
  const comment =
    "            # The token and secrets. arrive in a later step, never here.\n";
  const edits = [
    {
      job: "level",
      nextJob: "approve-major",
      nextStep: "            - name: Put the head on a local main branch\n",
    },
    {
      job: "release",
      nextJob: "prepare",
      nextStep: "            - name: Mint the release App token\n",
    },
  ];
  for (const edit of edits) {
    const section = workflowJobSection(source, edit.job, edit.nextJob);
    assert.ok(
      section.includes(edit.nextStep),
      `${edit.job} has no step ${JSON.stringify(edit.nextStep)} after its install step; the fixture has drifted`,
    );
    source = replaceFirst(
      source,
      section,
      replaceFirst(section, edit.nextStep, comment + edit.nextStep),
    );
  }
  for (const edit of edits) {
    const step = workflowStepBody(
      workflowJobSection(source, edit.job, edit.nextJob),
      "Install release tooling",
    );
    assert.ok(
      step.includes(comment.trimEnd()),
      `${edit.job} install step does not carry the inserted comment; the check would pass vacuously`,
    );
  }
  assert.deepEqual(releaseWorkflowContractViolations(source), []);
});

// Pins the check itself: the same insertion with a real token key in the install step must still be reported, so the
// comment-stripping did not turn the no-token check into a no-op.
test("a token key in the install step is still reported next to the inserted comment", () => {
  const section = workflowJobSection(md, "release", "prepare");
  const mutated = replaceFirst(
    md,
    section,
    replaceFirst(
      section,
      "              working-directory: .github/release\n",
      "              working-directory: .github/release\n              env:\n                  GH_TOKEN: ${{ github.token }}\n",
    ),
  );
  assert.notEqual(mutated, md);
  assert.ok(
    releaseWorkflowContractViolations(mutated).includes(
      "release: the npm install step must exist and run with no token in its environment",
    ),
  );
});

// The job sections each Go setup check reads, in release.md (bounded by the next job) and in the compiled lock.
const GO_JOBS = {
  level: "approve-major",
  release: "prepare",
  prepare: "stage",
  stage: "agent",
} as const;

// Only prepare and stage build Go, so only they set Go up, pinned from hooks/go/go.mod. level and release run the
// TypeScript command on Node alone.
function goSetupViolations(source: string, compiled: string): string[] {
  const violations: string[] = [];
  const md = withoutYAMLComments(source);
  const lockText = withoutYAMLComments(compiled);
  const views = [
    {
      label: "release.md",
      job: (name: keyof typeof GO_JOBS) =>
        workflowJobSection(md, name, GO_JOBS[name]),
    },
    {
      label: "release.lock.yml",
      job: (name: keyof typeof GO_JOBS) => jobKeySection(lockText, name),
    },
  ];
  for (const { label, job } of views) {
    for (const name of ["level", "release"] as const) {
      if (/setup-go@|name: Set up Go/.test(job(name))) {
        violations.push(`${label}: the ${name} job sets up Go`);
      }
    }
    for (const name of ["prepare", "stage"] as const) {
      const body = job(name);
      if (
        !body.includes("uses: ./.github/actions/release") ||
        !body.includes("command: bundle")
      ) {
        violations.push(
          `${label}: the ${name} job must invoke the release bundle action`,
        );
      }
    }
  }
  return violations;
}

test("only prepare and stage build with the release action", () => {
  assert.deepEqual(goSetupViolations(md, lock), []);
});

test("a Go setup change in the release jobs is reported", () => {
  const goStepMd = [
    "            - name: Set up Go",
    "              uses: actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e # v7.0.0",
    "              with:",
    "                  go-version-file: hooks/go/go.mod",
    "                  cache: false",
    "",
  ].join("\n");
  const goStepLock = [
    "      - name: Set up Go",
    "        uses: actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e # v7.0.0",
    "        with:",
    "          cache: false",
    "          go-version-file: hooks/go/go.mod",
    "",
  ].join("\n");
  const inJob = (
    text: string,
    section: string,
    from: string,
    to: string,
  ): string => replaceOnce(text, section, replaceOnce(section, from, to));
  const cases: [string, "md" | "lock", string, RegExp][] = [
    [
      "release.md level sets up Go",
      "md",
      replaceOnce(
        md,
        "            - name: Decide resume or analyze\n",
        `${goStepMd}            - name: Decide resume or analyze\n`,
      ),
      /release\.md: the level job sets up Go/,
    ],
    [
      "release.md release sets up Go",
      "md",
      replaceOnce(
        md,
        "            - name: Confirm the mode\n",
        `${goStepMd}            - name: Confirm the mode\n`,
      ),
      /release\.md: the release job sets up Go/,
    ],
    [
      "release.lock.yml level sets up Go",
      "lock",
      replaceOnce(
        lock,
        "      - name: Decide resume or analyze\n",
        `${goStepLock}      - name: Decide resume or analyze\n`,
      ),
      /release\.lock\.yml: the level job sets up Go/,
    ],
    [
      "release.lock.yml release sets up Go",
      "lock",
      replaceOnce(
        lock,
        "      - name: Confirm the mode\n",
        `${goStepLock}      - name: Confirm the mode\n`,
      ),
      /release\.lock\.yml: the release job sets up Go/,
    ],
    [
      "release.md prepare omits the trusted build",
      "md",
      inJob(
        md,
        workflowJobSection(md, "prepare", "stage"),
        "command: bundle",
        "command: verify",
      ),
      /release\.md: the prepare job must invoke the release bundle action/,
    ],
    [
      "release.lock.yml stage omits the trusted build",
      "lock",
      inJob(
        lock,
        jobKeySection(lock, "stage"),
        "command: bundle",
        "command: verify",
      ),
      /release\.lock\.yml: the stage job must invoke the release bundle action/,
    ],
  ];
  for (const [label, which, mutated, expected] of cases) {
    const violations =
      which === "md"
        ? goSetupViolations(mutated, lock)
        : goSetupViolations(md, mutated);
    assert.match(violations.join("\n"), expected, label);
  }
});

// Execute the shell that Actions gets from the compiled workflow. A successful run with
// identical bytes guards against a missing utility or a broken fixture passing a rejection test.
function runReleaseStep(
  job: string,
  name: string,
  cwd: string,
  env: NodeJS.ProcessEnv,
) {
  const section = parse(jobKeySection(lock, job));
  const step = section.steps.find(
    (item: { name: string }) => item.name === name,
  );
  assert.equal(typeof step?.run, "string", name);
  return spawnSync("bash", ["-eo", "pipefail", "-c", step.run], {
    cwd,
    env: { ...process.env, ...env },
    encoding: "utf8",
  });
}

test(
  "stage compares every candidate byte with the independent bundle",
  { skip: !posix },
  () => {
    const root = scratchDir("agentics-stage-compare-");
    const rebuilt = join(root, "rebuilt");
    const candidate = join(root, "release-assets");
    writeSyntheticBundle(rebuilt, STAGE_TAG);
    cpSync(rebuilt, candidate, { recursive: true });
    const compare = () =>
      runReleaseStep(
        "stage",
        "Compare release candidate with the independent bundle",
        root,
        {},
      );
    assert.equal(compare().status, 0);
    const archive = pluginTarballName(STAGE_TAG);
    writeFileSync(join(candidate, archive), "substituted package\n");
    const sums = join(candidate, "SHA256SUMS");
    writeFileSync(
      sums,
      checksumAssetNames(STAGE_TAG)
        .map((name) => fileDigest(join(candidate, name)) + "  " + name + "\n")
        .join(""),
    );
    verifyChecksums(candidate, STAGE_TAG);
    assert.equal(compare().status, 1);
    cpSync(rebuilt, candidate, { recursive: true });
    writeFileSync(sums, readFileSync(sums).subarray(0, -1));
    verifyChecksums(candidate, STAGE_TAG);
    assert.equal(compare().status, 1);
    cpSync(rebuilt, candidate, { recursive: true });
    rmSync(join(candidate, archive));
    assert.equal(compare().status, 1);
    cpSync(rebuilt, candidate, { recursive: true });
    writeFileSync(join(candidate, ".extra"), "");
    assert.equal(compare().status, 1);
  },
);

test(
  "prepare and stage bind a resumed target to the dispatch commit before choosing its parent",
  { skip: !posix },
  () => {
    const root = scratchDir("agentics-release-parent-");
    initRepo(root);
    const parent = commitFiles(root, {}, "reviewed");
    const release = commitFiles(root, {}, "release");
    const other = commitFiles(root, {}, "other");
    sh(root, "git", "update-ref", "refs/remotes/origin/main", other);
    for (const [job, step] of [
      ["prepare", "Move the verifier off a resumed release commit"],
      ["stage", "Bind stage to the reviewed parent"],
    ] as const) {
      sh(root, "git", "checkout", "--detach", release);
      const resume = (target: string) =>
        runReleaseStep(job, step, root, {
          MODE: "resume",
          GITHUB_SHA: release,
          TARGET_SHA: target,
        });
      assert.equal(resume(other).status, 1);
      assert.equal(sh(root, "git", "rev-parse", "HEAD").trim(), release);
      assert.equal(resume(release).status, 0);
      assert.equal(sh(root, "git", "rev-parse", "HEAD").trim(), parent);
    }
    const analyze = (target: string) =>
      runReleaseStep("stage", "Bind stage to the reviewed parent", root, {
        MODE: "analyze",
        GITHUB_SHA: parent,
        TARGET_SHA: target,
      });
    assert.equal(analyze(release).status, 0);
    assert.equal(analyze(other).status, 1);
    sh(root, "git", "update-ref", "refs/remotes/origin/main", parent);
    assert.equal(analyze(release).status, 1);
    const merge = sh(
      root,
      "git",
      "commit-tree",
      sh(root, "git", "rev-parse", "HEAD^{tree}").trim(),
      "-p",
      parent,
      "-p",
      other,
      "-m",
      "merge",
    ).trim();
    sh(root, "git", "update-ref", "refs/remotes/origin/main", merge);
    assert.equal(analyze(merge).status, 1);
  },
);

test(
  "stage reconciles a single complete verified attestation before publishing",
  { skip: !posix },
  async (t) => {
    const root = scratchDir("agentics-stage-provenance-");
    const assets = join(root, "rebuilt");
    writeSyntheticBundle(assets, STAGE_TAG);
    symlinkSync(join(repoRoot, ".github"), join(root, ".github"), "dir");
    mkdirSync(join(root, "bin"));
    writeFileSync(
      join(root, "bin", "gh"),
      [
        "#!/bin/sh",
        'case "$1" in',
        '  api) [ "$2 $3" = "--paginate --slurp" ] && [ "$#" -eq 4 ] || exit 2;',
        '    if [ "$COUNT" = unreadable ]; then exit 1; fi;',
        '    case "$COUNT" in missing|later-404|malformed-404)',
        '      cat "$RUNNER_TEMP/pages.json"; echo "gh: Not Found (HTTP 404)" >&2; exit 1 ;; esac;',
        '    cat "$RUNNER_TEMP/pages.json" ;;',
        '  attestation) [ "$2 $3 $4 $5" = "verify rebuilt/agentics-$TAG.tar.gz --repo $REPOSITORY" ] || exit 2;',
        '    [ "$6 $7" = "--signer-workflow $REPOSITORY/.github/workflows/release.lock.yml" ] || exit 2;',
        "    shift 7",
        '    if [ "$1" = "--source-ref" ]; then [ "$2" = "$SIGNING_REF" ] || exit 1; shift 2; fi;',
        '    [ "$1 $2" = "--limit $COUNT" ] && [ "$3 $4" = "--format json" ] || exit 2;',
        '    cat "$RUNNER_TEMP/verified.json" ;;',
        "  *) exit 2 ;;",
        "esac",
      ].join("\n"),
      { mode: 0o755 },
    );
    const output = join(root, "output");
    const subjects = expectedAssetNames(STAGE_TAG).map((name) => ({
      name,
      digest: { sha256: fileDigest(join(assets, name)) },
    }));
    const record = (subject: unknown) => ({
      verificationResult: { statement: { subject } },
    });
    const old = [
      record(
        subjects.filter((s) => s.name === "agentics-" + STAGE_TAG + ".tar.gz"),
      ),
    ];
    for (const [name, count, records, want, signingRef] of [
      ["absent", "0", [], "false"],
      ["missing", "missing", [], "false"],
      ["a later page returns 404", "later-404", [], null],
      ["malformed error body", "malformed-404", [], null],
      [
        "complete but signed on a feature branch",
        "1",
        [record(subjects)],
        null,
        "refs/heads/feature",
      ],
      ["legacy", "1", old, "false"],
      [
        "separate statements",
        "2",
        [record(subjects.slice(0, 1)), record(subjects.slice(1))],
        "false",
      ],
      [
        "wrong digest",
        "1",
        [
          record(
            subjects.map((s, i) =>
              i === 1 ? { ...s, digest: { sha256: "0".repeat(64) } } : s,
            ),
          ),
        ],
        "false",
      ],
      [
        "complete after legacy",
        "2",
        [...old, record([...subjects].reverse())],
        "true",
      ],
      ["malformed record after match", "2", [record(subjects), {}], null],
      ["missing verification result", "1", [], null],
      ["invalid count", "malformed", [], null],
      ["unreadable", "unreadable", [], null],
    ] as const) {
      await t.test(name, () => {
        let pages: unknown = [
          { attestations: [] },
          {
            attestations: /^\d+$/.test(count)
              ? Array.from({ length: Number(count) }, () => ({
                  bundle_url: "https://example.invalid/fixture",
                }))
              : null,
          },
        ];
        // gh api --paginate --slurp emits the error response as the last array element,
        // including when it has already read a successful page with a rel="next" link.
        if (count === "missing") pages = [{ message: "Not Found" }];
        if (count === "later-404")
          pages = [{ attestations: [] }, { message: "Not Found" }];
        if (count === "malformed-404") pages = [{ attestations: [] }];
        writeFileSync(join(root, "pages.json"), JSON.stringify(pages));
        writeFileSync(join(root, "verified.json"), JSON.stringify(records));
        writeFileSync(output, "");
        const result = runReleaseStep(
          "stage",
          "Reconcile existing attestations",
          root,
          {
            PATH: join(root, "bin") + ":" + process.env.PATH,
            COUNT: count,
            SIGNING_REF: signingRef ?? "refs/heads/main",
            RUNNER_TEMP: root,
            GITHUB_OUTPUT: output,
            TAG: STAGE_TAG,
            REPOSITORY: "fixture/release",
          },
        );
        assert.equal(
          result.status,
          want === null ? 1 : 0,
          name + ": " + result.stderr,
        );
        assert.equal(
          readFileSync(output, "utf8"),
          want === null ? "" : "present=" + want + "\n",
          name,
        );
      });
    }
  },
);
