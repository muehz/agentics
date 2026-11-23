import {
  appendFileSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  symlinkSync,
} from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import type { AuditResult } from "../check.ts";
import {
  DOMAIN_FAMILIES,
  run,
  type EvalsCheck,
  type Family,
} from "../domains/skills/index.ts";
import { mutateFile, writeFile } from "./files.ts";
import { fixtureGitEnv } from "./git-checkout.ts";
import { removeTree } from "./host.ts";

export { mutateFile, writeFile };

// The skills domain's fixture builders. They copy the literals the family modules pin instead of importing them, so a
// pin changed in a family module fails a test rather than moving the fixture along with it.

// The non-ASCII characters of the fixture text, built from code points so the source stays ASCII.
const EM = String.fromCodePoint(0x2014);
const MIDDLE_DOT = String.fromCodePoint(0x00b7);
const QUESTION_MARK = String.fromCodePoint(0x2753);
const ARROW = String.fromCodePoint(0x27a1, 0xfe0f);

/** The footer every triage emission carries. */
export const FOOTER = "> _Triaged by an AI agent_";

/** The repo-relative path of the eval record the eval-metadata fixture holds. */
export const EVAL_RECORD_PATH = "evals/skills.yaml";

const TRIAGE_ROOT = "skills/triage";
const MIN_ROLLUP_EXAMPLES = 3;

// Copies of the shared, adapter and surface file lists in domains/skills/orchestration.ts.
const SHARED_FILES_EXTRA = ["AGENTS.md", "README.md", "docs/ARCHITECTURE.md"];
const ADAPTER_FILES = [
  "skills/routing/references/copilot-tools.md",
  "skills/routing/references/surface-dispatch.md",
  "skills/routing/references/model-ladders.md",
  "skills/routing/references/model-ladders-examples.md",
];
const SURFACE_FILES = [
  "docs/surfaces/claude-code.md",
  "docs/surfaces/codex.md",
  "docs/surfaces/copilot-cli.md",
  "docs/surfaces/cursor.md",
  "docs/surfaces/vscode.md",
];

// Copies of the notices file and the Apache-attributed files in domains/skills/references.ts.
const NOTICES_FILE = "THIRD_PARTY_NOTICES.md";
const APACHE_ATTRIBUTED_FILES: ReadonlyArray<{ rel: string; notice: string }> =
  [
    {
      rel: "skills/docs/SKILL.md",
      notice:
        "<!-- Adapted from anthropics/skills doc-coauthoring (Apache-2.0); modified. -->",
    },
  ];

// The skills that ask decision questions, and the line that opens each pasted copy of the question format.
const REQUIRED_DECISION_QUESTION_SKILLS: ReadonlySet<string> = new Set([
  "skills/deepen/SKILL.md",
  "skills/docs/SKILL.md",
  "skills/execute/SKILL.md",
  "skills/forge/SKILL.md",
  "skills/gate/SKILL.md",
  "skills/gh/SKILL.md",
  "skills/git/SKILL.md",
  "skills/hillclimb/SKILL.md",
  "skills/ops/SKILL.md",
  "skills/prototype/SKILL.md",
  "skills/review/SKILL.md",
  "skills/scaffold/SKILL.md",
  "skills/triage/SKILL.md",
]);

const CANONICAL_DECISION_QUESTIONS_LINE =
  "**Decision questions.** To ask the user to pick an option or set a direction, use this format, numbered from Q1 per round, trailing asks too. " +
  ARROW +
  " always proposes an answer or default. Exempt: one-line clarifiers, fixed yes/no confirmations. Two or more: post the whole round as text, then one ask-tool call for the answers, shaped per challenge Step 3. One with 2+ choices: may use that tool directly. No such tool: post and wait.";

/** The metadata.artifact line the three chain skills carry in the fixture; every other skill declares none. */
export function artifactLine(name: string): string {
  switch (name) {
    case "intent":
      return "    artifact: intent.md\n";
    case "spec":
      return "    artifact: spec.md\n";
    case "decompose":
      return "    artifact: plan.md\n";
    default:
      return "";
  }
}

function skillText(name: string, artifact: string, body: string): string {
  return (
    "---\nname: " +
    name +
    "\ndescription: Test skill for " +
    name +
    '.\nversion: "1.0.0"\nmetadata:\n    surfaces: [claude-code]\n' +
    artifact +
    "---\n\n# " +
    name +
    "\n\n" +
    body
  );
}

/** A SKILL.md with the fixture's frontmatter, the three chain skills carrying their artifact. */
export function skillMD(name: string): string {
  return skillText(name, artifactLine(name), "A current-state body.\n");
}

/** A text fence holding a triage marker comment, `body` and the footer. */
export function markeredFence(marker: string, body: string): string {
  return (
    "```text\n<!-- agentics:triage:" +
    marker +
    " -->\n\n" +
    body +
    "\n\n" +
    FOOTER +
    "\n```\n"
  );
}

/** A markdown fence holding `body` and the footer. */
export function issueFence(body: string): string {
  return "```markdown\n" + body + "\n\n" + FOOTER + "\n```\n";
}

/** The triage subagent contract: the disposition enum, the response shape and the check-verification heading. */
export function subagentContractMD(): string {
  return (
    "# Subagent contract\n\nThe table sums for every counted source. Human Feedback stays drafts-only.\n\n## Disposition enum\n\n| Value | Meaning | Used by |\n| ----- | ------- | ------- |\n| `remediated` | fixed | PR sources |\n| `accept_intentional` | deliberate | PR sources |\n| `dismissed` | false | PR sources |\n| `escalate` | human | PR sources |\n| `deferred` | later | PR sources |\n| `conflict` | conflict | Merge Conflicts |\n| `needs_human` | blocked | Checks |\n| `drafted` | reply drafted | Human Feedback |\n\n## Response shape\n\n```json\n" +
    '{"body": "<!-- agentics:triage:reply:{source}:{targetId} -->\\n\\nFixed in {SHA} ' +
    EM +
    " description.\\n\\n" +
    FOOTER +
    '"}\n```\n\n## Verifying that a check fails as intended\n'
  );
}

/** The rollup template without examples: the column rules, the per-source mapping and one rendered body per marker. */
export function rollupMappingMD(): string {
  let out =
    '# Rollup template\n\n| Source | Status | Found | Fixed | Accepted | Dismissed | Needs you |\n| ------ | ------ | ----- | ----- | -------- | --------- | --------- |\n\n**Accepted and Dismissed are separate columns on purpose.** Use `disposition === "accept_intentional"`.\n\n**Exactly one column per finding.**\n\n**The mapping is total.**\n\n**Check the balance before posting.**\n\n**Flag partially-correct findings explicitly.**\n\n## Per-source mapping\n\n| Source          | Fixed | Accepted | Dismissed | Needs you |\n| --------------- | ----- | -------- | --------- | --------- |\n| PR sources | `remediated` | `accept_intentional` | `dismissed` | `escalate`, `deferred` |\n| Human Feedback | Decision `accepted` or `accepted-edited` with a commit | `accepted` or `accepted-edited`, no commit | Decision `skipped` | (none) |\n| Merge Conflicts | `conflict` | ' +
    EM +
    " | " +
    EM +
    " | `needs_human` |\n\n## Rendered rollup bodies\n\n";
  for (const marker of [
    "rollup:pr",
    "reply:example",
    "rollup:conflicts",
    "rollup:security",
  ])
    out += markeredFence(marker, "Body for " + marker + ".") + "\n";
  return out;
}

/** A valid triage rollup in the mixed configuration: two sources, one item needing you. */
export const VALID_ROLLUP = [
  "<!-- agentics:triage:rollup -->",
  "<!-- agentics:triage:execution:1790000000-12345 -->",
  "",
  "@jdoe " + EM + " 6 fixed " + MIDDLE_DOT + " 1 needs you",
  "",
  "### Needs you",
  "",
  "- **Checks " +
    EM +
    " Test / Go (xcode-27, tests):** fails non-deterministically. [Failed job](https://example.invalid/job/1)",
  "",
  "| Source | Status | Found | Fixed | Accepted | Dismissed | Needs you |",
  "| ------ | ------ | ----- | ----- | -------- | --------- | --------- |",
  "| Copilot Review | Completed | 6 | 5 | 1 | 0 | 0 |",
  "| Checks | Partial | 2 | 1 | " + EM + " | " + EM + " | 1 |",
  "| **Total** | | **8** | **6** | **1** | **0** | **1** |",
  "",
  "<details><summary>Details</summary>",
  "",
  "#### Base sync",
  "",
  "<!-- agentics:triage:section:base-sync -->",
  "Up to date " + EM + " `main` was already an ancestor of the PR head.",
  "<!-- agentics:triage:section-end:base-sync -->",
  "",
  "#### Copilot Review",
  "",
  "- **Fixed (5):** `8bf374e5`",
  "- **Accepted (1):** `api/cors.go:14` " + EM + " deliberate.",
  "",
  "<sub>6 replies posted</sub>",
  "",
  "#### Checks",
  "",
  "- **Fixed (1):** Format drift " + EM + " `a1b2c3d4`",
  "- **Needs you (1):** Test / Go (xcode-27, tests)",
  "",
  "</details>",
  "",
  FOOTER,
].join("\n");

/** The rollup template: the mapping followed by the minimum number of valid rollup examples. */
export function rollupTemplateMD(): string {
  let out = rollupMappingMD() + "## Examples\n\n";
  for (let i = 0; i < MIN_ROLLUP_EXAMPLES; i += 1)
    out += "```text\n" + VALID_ROLLUP + "\n```\n\n";
  return out;
}

/** The triage source-detection reference: the Checkmarx marker rule and the bot routing table. */
export function sourceDetectionMD(): string {
  return (
    "# Source detection\n\nCheckmarx detection keys on the body marker " +
    EM +
    " never the comment author.\n\n### Bot routing table\n\n| login | source |\n| ----- | ------ |\n| copilot | Copilot |\n| github-advanced-security | Code Security |\n\n**Marker-keyed sources bypass this table.**\n"
  );
}

/** The orchestration lifecycle reference, which names the footer every posted body closes with. */
export function lifecycleMD(): string {
  return (
    "# Orchestration lifecycle\n\nEvery posted body closes with " +
    FOOTER +
    "\n"
  );
}

const CANONICAL_QUESTION_BLOCK_LINE_1 =
  QUESTION_MARK +
  " **Q1** - **<question title>**: <question body, might be multiple paragraphs, including multiple choices>";
const CANONICAL_QUESTION_BLOCK_LINE_3 = ARROW + " <your recommended answer>";

/** The paragraph challenge's Step 3 carries that the decision-question copies point to. */
const COLLECTING_ANSWERS_PARAGRAPH =
  "**Collecting answers.** Post the whole round as text, then make one ask-tool call.\n";

/** Where the fixture keeps the contract file that owns the canonical question-format block. */
const QUESTION_FORMAT_CONTRACT_FILE =
  "skills/routing/references/contracts/question-format.md";

/**
 * The contract file a fixture checkout carries, with the marker line, the canonical block unindented, and the
 * Collecting answers section skills link to, carrying the posted-round sentences question-format pins.
 */
export function contractFixtureMD(): string {
  return (
    "# Question format\n\n" +
    "Format each question exactly like so:\n\n" +
    "```\n" +
    CANONICAL_QUESTION_BLOCK_LINE_1 +
    "\n" +
    "\n" +
    CANONICAL_QUESTION_BLOCK_LINE_3 +
    "\n" +
    "```\n\n" +
    "## Collecting answers\n\n" +
    "Collect the answers.\n\n" +
    "Every round is posted in the visible chat first, as one message, before any ask-tool call.\n" +
    "The whole round sits in one blockquote.\n"
  );
}

/**
 * The challenge skill: a pasted copy of the contract file's block inside a numbered list item, fence-indented four
 * spaces, plus the Collecting answers paragraph, both inside Step 3 and ahead of the Step 4 heading.
 */
export function challengeFixtureMD(): string {
  return (
    skillMD("challenge") +
    "\n" +
    "### 3. Interview in frontier rounds\n\n" +
    "2. Ask the whole frontier in one round. Format each question exactly like so:\n\n" +
    "    ```\n" +
    "    " +
    CANONICAL_QUESTION_BLOCK_LINE_1 +
    "\n" +
    "\n" +
    "    " +
    CANONICAL_QUESTION_BLOCK_LINE_3 +
    "\n" +
    "    ```\n" +
    "\n" +
    COLLECTING_ANSWERS_PARAGRAPH +
    "\n### 4. Challenge weak spots\n"
  );
}

/**
 * An inline, unindented copy of the block, the shape every decision-asking skill carries, optionally changed by one
 * character (the first `Q1` becomes `Q2`).
 */
export function decisionQuestionsCopy(mutateOneChar = false): string {
  const line1 = mutateOneChar
    ? CANONICAL_QUESTION_BLOCK_LINE_1.replace("Q1", "Q2")
    : CANONICAL_QUESTION_BLOCK_LINE_1;
  return (
    CANONICAL_DECISION_QUESTIONS_LINE +
    "\n\n" +
    "```\n" +
    line1 +
    "\n" +
    "\n" +
    CANONICAL_QUESTION_BLOCK_LINE_3 +
    "\n" +
    "```\n"
  );
}

/** The shared skill body plus the two declarations flow-emission requires. */
export function emissionSkillMD(name: string): string {
  return (
    skillMD(name) +
    "\nRecord rows through `agx flow log`, per\n" +
    "[flow-state.md](../execute/references/flow-state.md).\n"
  );
}

const WORK_FOLDER_CONTRACT_FILE =
  "skills/decompose/references/chain-artifacts.md";
const WORK_FOLDER_GATE_FILE = "skills/gate/references/branch/tidy.md";

// Copies of the pins in domains/skills/work-folder.ts, in table order.
const WORK_FOLDER_PINS: ReadonlyArray<{ rel: string; literal: string }> = [
  {
    rel: WORK_FOLDER_CONTRACT_FILE,
    literal:
      "`intent.md` and `plan.md` are the required pair when `/agentics:gate branch` hands a branch off to a pull request",
  },
  {
    rel: WORK_FOLDER_CONTRACT_FILE,
    literal:
      "`intent.md` is required only when a present `plan.md` or `spec.md` names `[intent.md]` in its `derived-from` field",
  },
  {
    rel: WORK_FOLDER_CONTRACT_FILE,
    literal:
      "The folder's scratch and evidence directory: optional, ignored by Git, with unrestricted contents, including nested folders",
  },
  {
    rel: WORK_FOLDER_GATE_FILE,
    literal:
      "The tidy step never deletes content: it moves, renames, and untracks",
  },
  {
    rel: WORK_FOLDER_GATE_FILE,
    literal: "A name collision keeps both copies",
  },
  {
    rel: WORK_FOLDER_GATE_FILE,
    literal:
      "Tracked or force-added paths leave the index with `git rm -r --cached --ignore-unmatch`",
  },
  {
    rel: WORK_FOLDER_GATE_FILE,
    literal:
      "A conforming folder changes nothing and makes no commit, so a second run of the step is a no-op",
  },
  {
    rel: WORK_FOLDER_GATE_FILE,
    literal:
      "Only a genuinely missing required file stops the gate: the stop names the file and every place searched",
  },
  {
    rel: WORK_FOLDER_GATE_FILE,
    literal:
      "The gate never fabricates a placeholder for a missing required file",
  },
  {
    rel: WORK_FOLDER_GATE_FILE,
    literal:
      "The step does not run for `/agentics:gate check` or for a discard named in advance",
  },
  {
    rel: WORK_FOLDER_CONTRACT_FILE,
    literal:
      "A small fix that `execute fix` ran has no work folder and needs neither file at the hand-off",
  },
  {
    rel: WORK_FOLDER_CONTRACT_FILE,
    literal:
      "`/agentics:gate branch` accepts that hand-off only when `<work-root>/unassigned/state.tsv` holds a `skip chain <branch>` row with no later `pivot fix <branch>` row",
  },
];

/** Every pinned sentence for `rel`, each on its own line. */
export function workFolderFixtureText(rel: string): string {
  const lines = WORK_FOLDER_PINS.filter((pin) => pin.rel === rel).map(
    (pin) => pin.literal,
  );
  return "# Pinned file\n\n" + lines.join("\n\n") + "\n";
}

/** Writes both files the work-folder family pins, each with every pin. */
export function writeWorkFolderPinnedFiles(root: string): void {
  for (const rel of [WORK_FOLDER_CONTRACT_FILE, WORK_FOLDER_GATE_FILE])
    writeFile(root, rel, workFolderFixtureText(rel));
}

const SPEC_AXIS_PATH = "skills/review/references/axes/spec-axis.md";
const AXES_MODE_PATH = "skills/review/references/axes/axes-mode.md";
const BRANCH_WORKFLOW_PATH =
  "skills/gate/references/branch/review-and-audit.md";

// Copies of the pins in domains/skills/spec-pass.ts, in table order.
const SPEC_PASS_PINS: ReadonlyArray<{ rel: string; literal: string }> = [
  { rel: SPEC_AXIS_PATH, literal: "## Per-requirement table" },
  {
    rel: SPEC_AXIS_PATH,
    literal:
      "`unrequested` lists a scope addition and is never counted as a gap.",
  },
  {
    rel: AXES_MODE_PATH,
    literal:
      "Each spec path named in the invocation. They are read before any other source.",
  },
  {
    rel: AXES_MODE_PATH,
    literal: "End the aggregate with exactly one `Lanes:` line",
  },
  {
    rel: AXES_MODE_PATH,
    literal: "Axes mode accepts `--spec-only`, which runs the Spec lane alone",
  },
  {
    rel: BRANCH_WORKFLOW_PATH,
    literal:
      "Read the per-requirement table from the Step 8 run whose pinned SHA equals the SHA this step pinned.",
  },
  {
    rel: BRANCH_WORKFLOW_PATH,
    literal: "Review: incomplete (<lane> not run)",
  },
];

/** Every pinned sentence for `rel`, each in its own paragraph. */
export function specPassFixtureText(rel: string): string {
  const parts = SPEC_PASS_PINS.filter((pin) => pin.rel === rel).map(
    (pin) => pin.literal,
  );
  return "# Pinned file\n\n" + parts.join("\n\n") + "\n";
}

const absolute = (root: string, rel: string): string =>
  path.join(root, ...rel.split("/"));

function readExisting(root: string, rel: string): string {
  try {
    return readFileSync(absolute(root, rel), "utf8");
  } catch {
    return "";
  }
}

/** Adds the pinned sentences and keeps what each file already carries. */
export function appendSpecPassPins(root: string): void {
  for (const rel of [SPEC_AXIS_PATH, AXES_MODE_PATH, BRANCH_WORKFLOW_PATH])
    writeFile(
      root,
      rel,
      readExisting(root, rel) + "\n" + specPassFixtureText(rel),
    );
}

/** The Events table rows the lean-cadence pins anchor on, then rows that only start alike. */
export const FLOW_STATE_FIXTURE_TEXT =
  "| `review` | a reviewer returned; `target` is `plan` |\n" +
  "| `escape` | a Critical or Important defect that passed a review and was found later |\n\n" +
  "| `review` | `kind=<plan> dispatched=<ts>` |\n" +
  "| `escape` | `found-by=<gate> severity=<critical>` |\n";

/** One prose body per pinned file, carrying every present pin and none of the retired text. */
export function leanCadenceFixtureFiles(): Record<string, string> {
  return {
    "skills/decompose/references/plan-template.md":
      "A task marks **Packet review:** no or yes.\n",
    "skills/execute/references/stage-1-dispatch-rule.md":
      "The task carries **Packet review:** no.\n",
    "skills/execute/SKILL.md": "Give each review group one quality review.\n",
    "skills/execute/references/flow-state.md": FLOW_STATE_FIXTURE_TEXT,
    "skills/gh/SKILL.md": "Convert only on request.\n",
    "skills/gh/references/stack.md": "Convert a stack when asked.\n",
  };
}

/** Adds the pinned text to a green fixture, keeping any content a file already carries. */
export function appendLeanCadencePins(root: string): void {
  writeFile(
    root,
    "skills/gh/SKILL.md",
    skillMD("gh") + "\n" + decisionQuestionsCopy(false),
  );
  for (const [rel, body] of Object.entries(leanCadenceFixtureFiles()))
    writeFile(root, rel, readExisting(root, rel) + "\n" + body);
}

/** Writes the notices file and every Apache-attributed file with its in-file notice. */
export function writeApacheFixture(root: string): void {
  const lines = [
    "# THIRD-PARTY NOTICES",
    "",
    "## Per-file provenance manifest",
    "",
    "| File | Upstream | License |",
    "| ---- | -------- | ------- |",
  ];
  for (const entry of APACHE_ATTRIBUTED_FILES) {
    let body = "# Apache-derived fixture\n\n" + entry.notice + "\n";
    if (entry.rel.endsWith("/SKILL.md")) {
      const name = entry.rel.split("/")[1] ?? "";
      body = skillMD(name) + "\n" + entry.notice + "\n";
      if (REQUIRED_DECISION_QUESTION_SKILLS.has(entry.rel))
        body += decisionQuestionsCopy(false);
    }
    writeFile(root, entry.rel, body);
    lines.push(
      "| `" + entry.rel.replace(/^skills\//, "") + "` | fixture | Apache-2.0 |",
    );
  }
  writeFile(root, NOTICES_FILE, [...lines, ""].join("\n"));
}

function newRoot(): string {
  return mkdtempSync(path.join(tmpdir(), "agentics-skillsrepo-"));
}

/** A checkout with one skill, tdd. */
export function cloneInlineRepo(): string {
  const root = newRoot();
  writeFile(root, "skills/tdd/SKILL.md", skillMD("tdd"));
  return root;
}

/** A checkout with tdd and a minimal eval record as plain text, with no digest binding. */
export function cloneEvalMetadataRepo(): string {
  const root = cloneInlineRepo();
  writeFile(
    root,
    EVAL_RECORD_PATH,
    "skills:\n  tdd:\n    last-evaluated: 2026-08-09\n    eval-status: passing\n",
  );
  return root;
}

/** A checkout with tdd and a reference, the review axes file and the Apache fixture. */
export function cloneReferenceRepo(): string {
  const root = newRoot();
  writeFile(root, "skills/tdd/SKILL.md", skillMD("tdd"));
  writeFile(root, "skills/tdd/references/example.md", "# Example\n");
  writeFile(
    root,
    "skills/review/references/axes/axes-mode.md",
    "# Axes\n\nIntent-level guidance.\n",
  );
  writeApacheFixture(root);
  return root;
}

/** A checkout with the triage skill and its references. */
export function cloneTriageRepo(): string {
  const root = newRoot();
  writeFile(
    root,
    "skills/triage/SKILL.md",
    emissionSkillMD("triage") +
      "\nEvery posted comment closes with " +
      FOOTER +
      "\n" +
      decisionQuestionsCopy(false),
  );
  writeFile(
    root,
    TRIAGE_ROOT + "/references/rollup-template.md",
    rollupTemplateMD(),
  );
  writeFile(
    root,
    TRIAGE_ROOT + "/references/subagent-contract.md",
    subagentContractMD(),
  );
  writeFile(
    root,
    TRIAGE_ROOT + "/references/source-detection.md",
    sourceDetectionMD(),
  );
  writeFile(
    root,
    TRIAGE_ROOT + "/references/orchestration-lifecycle.md",
    lifecycleMD(),
  );
  let issueMode = "# Issue mode\n\n";
  for (const disposition of ["fix", "wontfix", "duplicate", "question"])
    issueMode +=
      issueFence("Disposition template: " + disposition + ".") + "\n";
  writeFile(root, TRIAGE_ROOT + "/references/issue-mode.md", issueMode);
  writeFile(
    root,
    TRIAGE_ROOT + "/references/checks/checks-triage.md",
    "# Checks\n\nEmit a section framed by <!-- agentics:triage:section:checks:{check_run_id} -->.\n",
  );
  writeFile(
    root,
    TRIAGE_ROOT + "/references/pull-requests/copilot/copilot-triage.md",
    "# Emitter\n\nEvery posted body closes with " + FOOTER + "\n",
  );
  return root;
}

/** A checkout with the shared files, the adapters and the surface explainers. */
export function cloneOrchestrationRepo(): string {
  const root = newRoot();
  for (const rel of SHARED_FILES_EXTRA)
    writeFile(root, rel, "# Shared\n\nShared guidance stating intent only.\n");
  writeFile(root, "skills/test/SKILL.md", skillMD("test"));
  for (const rel of [...ADAPTER_FILES, ...SURFACE_FILES])
    writeFile(root, rel, "# Adapter\n\nSurface notes.\n");
  writeFile(
    root,
    "skills/routing/references/copilot-tools.md",
    "# Adapter\n\nNative names live here: the runSubagent tool.\n",
  );
  writeFile(
    root,
    "skills/routing/references/surface-dispatch.md",
    "# Adapter\n\nDispatch: spawn_agent.\n",
  );
  return root;
}

/** A checkout with the three chain artifact owners plus one skill that owns nothing. */
export function cloneArtifactRepo(): string {
  const root = newRoot();
  for (const name of ["intent", "spec", "decompose", "tdd"])
    writeFile(root, `skills/${name}/SKILL.md`, skillMD(name));
  return root;
}

/**
 * The skills fixture. `sharedContracts` picks how its skills meet the shared contracts: `"files"` (the default) writes
 * the three shared contract files and leaves the decision-question format pasted into each skill; `"linked"` also
 * replaces each pasted copy and adds each required link, so the default audit passes; `"absent"` leaves out the three
 * shared contract files and the record-keeping text.
 */
export function cloneSkillsRepo({
  sharedContracts = "files",
}: { sharedContracts?: "absent" | "files" | "linked" } = {}): string {
  const root = cloneTriageRepo();
  for (const name of ["docs", "forge", "prototype"])
    writeFile(
      root,
      `skills/${name}/SKILL.md`,
      skillMD(name) + "\n" + decisionQuestionsCopy(false),
    );
  // execute is on the flow roster, so its skill declares its flow-state emission like every other roster member.
  writeFile(
    root,
    "skills/execute/SKILL.md",
    emissionSkillMD("execute") + "\n" + decisionQuestionsCopy(false),
  );
  writeFile(
    root,
    "skills/execute/references/flow-state.md",
    "# Flow state\n\nThe contract.\n",
  );
  // gate and decompose own the two prose files the workfolder family pins.
  writeFile(
    root,
    "skills/gate/SKILL.md",
    emissionSkillMD("gate") + "\n" + decisionQuestionsCopy(false),
  );
  writeFile(root, "skills/decompose/SKILL.md", emissionSkillMD("decompose"));
  writeWorkFolderPinnedFiles(root);
  // The record-keeping family requires the helper's audit call in gate's branch workflow and in the fix entry, the fix
  // entry's audit heading, and execute's two links to it. The absent variant leaves them out.
  if (sharedContracts !== "absent") {
    writeFile(
      root,
      BRANCH_WORKFLOW_PATH,
      readExisting(root, BRANCH_WORKFLOW_PATH) +
        "\nRun `agx flow audit` and report each finding.\n",
    );
    writeFile(
      root,
      "skills/execute/references/fix-entry.md",
      "# Fix entry\n\n## Standalone fix commit audit\n\nAudit the fix with `agx flow audit`.\n",
    );
    // A code span, not a Markdown link, so a test that deletes every references directory sees only the broken links
    // of the other skills, not one more for this line.
    const auditLink =
      "Run the standalone fix commit audit, `references/fix-entry.md#standalone-fix-commit-audit`.\n";
    appendFileSync(
      absolute(root, "skills/execute/SKILL.md"),
      `\n${auditLink}\n${auditLink}`,
    );
  }
  appendLeanCadencePins(root);
  writeFile(
    root,
    "skills/review/SKILL.md",
    skillMD("review") + "\n" + decisionQuestionsCopy(false),
  );
  appendSpecPassPins(root);
  // The question-format family reads the contract file directly, so a checkout without it ends in an audit error.
  // challenge carries a pasted copy of the block, as on the real tree.
  writeFile(root, QUESTION_FORMAT_CONTRACT_FILE, contractFixtureMD());
  writeFile(root, "skills/challenge/SKILL.md", challengeFixtureMD());
  writeApacheFixture(root);
  writeFile(root, "skills/routing/SKILL.md", skillMD("routing"));
  for (const rel of SHARED_FILES_EXTRA)
    writeFile(root, rel, "# Shared\n\nIntent only.\n");
  for (const rel of [...ADAPTER_FILES, ...SURFACE_FILES])
    writeFile(root, rel, "# Adapter\n\nSurface notes.\n");
  if (sharedContracts !== "absent")
    writeSharedContractFixtures(root, sharedContracts === "linked");
  if (sharedContracts === "linked") linkFixtureDecisionQuestions(root);
  return root;
}

/** Replaces each skill's pasted decision-question format with a link to the contract file, as the real skills do. */
function linkFixtureDecisionQuestions(root: string): void {
  const relLink =
    "[Question format](../routing/references/contracts/question-format.md)";
  for (const name of [
    "docs",
    "execute",
    "forge",
    "gate",
    "gh",
    "prototype",
    "review",
    "triage",
  ])
    mutateFile(
      root,
      `skills/${name}/SKILL.md`,
      decisionQuestionsCopy(false),
      relLink + "\n",
    );
  writeFile(
    root,
    "skills/challenge/SKILL.md",
    skillMD("challenge") + "\n" + relLink + "\n",
  );
  appendFileSync(
    absolute(root, "skills/challenge/SKILL.md"),
    "\n[Principles check](../routing/references/contracts/principles-check.md).\n",
  );
}

/**
 * Writes the three shared contract files under `skills/routing/references/contracts/` and, with `linkConsumers`, a link
 * to each from every skill that must name it.
 */
function writeSharedContractFixtures(
  root: string,
  linkConsumers: boolean,
): void {
  const dir = "skills/routing/references/contracts";
  for (const [file, title, consumers] of [
    [
      "workflow-steps.md",
      "Workflow steps",
      ["decompose", "execute", "gate", "review", "routing"],
    ],
    [
      "principles-check.md",
      "Principles check",
      ["challenge", "decompose", "execute", "gate"],
    ],
    [
      "empirical-questions.md",
      "Empirical questions",
      ["decompose", "prototype", "routing"],
    ],
  ] as const) {
    const canonical = `${dir}/${file}`;
    writeFile(root, canonical, `# ${title}\n\nShared policy.\n`);
    if (!linkConsumers) continue;
    for (const consumer of consumers) {
      const rel = `skills/${consumer}/SKILL.md`;
      const link = path.posix.relative(path.posix.dirname(rel), canonical);
      appendFileSync(absolute(root, rel), `\n[${title}](${link}).\n`);
    }
  }
}

/** What `addSkill` can vary. */
export type AddSkillOptions = {
  /**
   * The `metadata.artifact` file name. Left out, the fixture's own choice applies (`intent.md`, `spec.md` or `plan.md`
   * for the three chain skills, none for any other); `null` declares none.
   */
  artifact?: string | null;
  /** The text after the `# <name>` heading, replacing the fixture's one-line current-state body. */
  body?: string;
};

/**
 * Writes `skills/<name>/SKILL.md` in the fixture's frontmatter shape, for a test that needs a skill the green fixture
 * lacks (intent, spec, tdd, git, ops, deepen, hillclimb, scaffold).
 */
export function addSkill(
  root: string,
  name: string,
  options: AddSkillOptions = {},
): void {
  const artifact =
    options.artifact === undefined
      ? artifactLine(name)
      : options.artifact === null
        ? ""
        : `    artifact: ${options.artifact}\n`;
  writeFile(
    root,
    `skills/${name}/SKILL.md`,
    skillText(name, artifact, options.body ?? "A current-state body.\n"),
  );
}

/** Creates a directory (and its parents) inside a fixture. */
export function mkdir(root: string, rel: string): void {
  mkdirSync(absolute(root, rel), { recursive: true });
}

/** Removes a file or a directory tree from a fixture; nothing happens when it is already gone. */
export function remove(root: string, rel: string): void {
  rmSync(absolute(root, rel), { recursive: true, force: true });
}

/**
 * Creates a symbolic link at `rel` inside the fixture that points at `target` (resolved as the host does, so a relative
 * target is relative to the link's directory). Returns false, with nothing left behind, when the host refuses to create
 * a link; a test then skips with a logged reason instead of weakening an assertion.
 */
export function link(
  root: string,
  rel: string,
  target: string,
  type?: "file" | "dir" | "junction",
): boolean {
  const at = absolute(root, rel);
  try {
    mkdirSync(path.dirname(at), { recursive: true });
    symlinkSync(target, at, type);
    return true;
  } catch {
    return false;
  }
}

/** A clean `evalsCheck` runner for the last family: it finds nothing and starts no process. */
export const evalsCheck: EvalsCheck = async () => [];

/** What `auditSkills` can vary. The tracked-files lookup is not among them: the domain's own default runs. */
export interface SkillsAuditOptions {
  /** The inherited environment for the fixture's tracked-files lookup. */
  env?: NodeJS.ProcessEnv;
  /** The families to run, in order. Defaults to `DOMAIN_FAMILIES`, the set the CLI runs. */
  families?: readonly Family[];
  /** The runner for the last family. Defaults to the clean `evalsCheck` stub, so no engine process starts. */
  evalsCheck?: EvalsCheck;
}

/**
 * Runs the skills domain on a checkout as the CLI does without `--short` or `--fast`, and returns its result. A test
 * that audits the default families on a passing fixture writes the standards baseline first, as the real checkout
 * commits one.
 */
export function auditSkills(
  root: string,
  options: SkillsAuditOptions = {},
): Promise<AuditResult> {
  return run(
    root,
    {
      short: false,
      fast: false,
      diagnostics: () => {},
      env: fixtureGitEnv(options.env),
    },
    {
      families: options.families ?? DOMAIN_FAMILIES,
      evalsCheck: options.evalsCheck ?? evalsCheck,
    },
  );
}

/**
 * For each rule the CLI can report, one change to a passing fixture that makes it fire, grouped by the slice that holds
 * the rule's family. Some breakers fire other rules too; the skills domain's index tests pin every failure each breaker
 * gives.
 */
export const BREAKERS: Readonly<Record<string, (root: string) => void>> = {
  // metadata slice: inline
  "skill-md-present": (root) => mkdir(root, "skills/empty"),
  "skill-frontmatter": (root) =>
    mutateFile(root, "skills/docs/SKILL.md", 'version: "1.0.0"\n', ""),
  "skill-name-unique": (root) =>
    mutateFile(root, "skills/forge/SKILL.md", "name: forge\n", "name: docs\n"),
  // metadata slice: frontmatter
  "frontmatter-yaml": (root) =>
    mutateFile(
      root,
      "skills/prototype/SKILL.md",
      "name: prototype\n",
      "name: prototype\nname: again\n",
    ),
  // metadata slice: eval-metadata
  "eval-frontmatter-present": (root) =>
    writeFile(
      root,
      "skills/prototype/SKILL.md",
      "# prototype\n\nNo frontmatter.\n",
    ),
  "eval-history-footer": (root) =>
    writeFile(
      root,
      "skills/prototype/SKILL.md",
      readExisting(root, "skills/prototype/SKILL.md") +
        "\n## Validation history\n\n- 2026-01-01 passing\n",
    ),
  // metadata slice: eval-frontmatter-clean
  "eval-frontmatter-clean": (root) =>
    mutateFile(
      root,
      "skills/prototype/SKILL.md",
      "    surfaces: [claude-code]\n",
      "    surfaces: [claude-code]\n    eval-status: passing\n",
    ),
  // metadata slice: orchestration
  "orchestration-file-present": (root) =>
    remove(root, "skills/routing/references/model-ladders.md"),
  "orchestration-stale-phrase": (root) =>
    writeFile(
      root,
      "docs/surfaces/codex.md",
      "# Adapter\n\nThe subagents are stateless here.\n",
    ),
  "orchestration-native-identifier": (root) =>
    mutateFile(
      root,
      "AGENTS.md",
      "Intent only.",
      "Intent only. Use the Agent tool to fan out.",
    ),
  // metadata slice: reference-link
  "reference-link": (root) =>
    writeFile(
      root,
      "skills/prototype/references/notes.md",
      "# Notes\n\nSee [the missing page](./missing.md).\n",
    ),
  "attribution-centralized": (root) =>
    writeFile(
      root,
      "skills/prototype/references/origin.md",
      "# Origin\n\nAdapted from example/project (MIT); modified.\n",
    ),
  "reference-attribution": (root) =>
    mutateFile(root, "skills/docs/SKILL.md", "Adapted from", "Based on"),
  "notices-manifest": (root) =>
    mutateFile(
      root,
      "THIRD_PARTY_NOTICES.md",
      "## Per-file provenance manifest",
      "## Provenance",
    ),
  // metadata slice: reference-naming
  "reference-naming": (root) =>
    writeFile(root, "skills/prototype/references/Bad_Name.md", "# Bad\n"),
  // content slice: triage
  "triage-file-present": (root) =>
    remove(root, "skills/triage/references/source-detection.md"),
  "triage-detection-routing": (root) =>
    mutateFile(
      root,
      "skills/triage/references/source-detection.md",
      "| copilot | Copilot |\n",
      "| copilot | Copilot |\n| checkmarx | Code Security |\n",
    ),
  "triage-enum-mapping": (root) =>
    mutateFile(
      root,
      "skills/triage/references/rollup-template.md",
      "`escalate`, `deferred`",
      "`escalate`",
    ),
  "triage-rollup-shape": (root) =>
    replaceFirst(
      root,
      "skills/triage/references/rollup-template.md",
      "@jdoe " + EM + " 6 fixed",
      "@jdoe " + EM + " all done",
    ),
  "triage-attestation-footer": (root) =>
    writeFile(
      root,
      "skills/triage/references/pull-requests/copilot/copilot-triage.md",
      "# Emitter\n\nEvery posted body closes with the attestation.\n",
    ),
  "triage-attestation-prefix": (root) =>
    writeFile(
      root,
      "skills/triage/references/notes.md",
      "# Notes\n\n_Triaged by an AI agent_\n",
    ),
  "triage-attestation-legacy": (root) =>
    mutateFile(
      root,
      "skills/triage/references/source-detection.md",
      "# Source detection\n",
      "# Source detection\n\nThis was generated by AI during triage.\n",
    ),
  "triage-attestation-fence": (root) =>
    mutateFile(
      root,
      "skills/triage/references/rollup-template.md",
      "Body for rollup:pr.\n\n" + FOOTER,
      "Body for rollup:pr.\n\nClosing line.",
    ),
  "triage-attestation-issue-mode": (root) =>
    mutateFile(
      root,
      "skills/triage/references/issue-mode.md",
      "Disposition template: fix.\n\n" + FOOTER,
      "Disposition template: fix.",
    ),
  "triage-attestation-contract-example": (root) =>
    mutateFile(
      root,
      "skills/triage/references/subagent-contract.md",
      "description.\\n\\n",
      "description.\\n",
    ),
  // content slice: artifact
  "skill-artifact-declared": (root) =>
    mutateFile(
      root,
      "skills/decompose/SKILL.md",
      "    artifact: plan.md\n",
      "",
    ),
  "skill-artifact-expected": (root) =>
    mutateFile(
      root,
      "skills/prototype/SKILL.md",
      "    surfaces: [claude-code]\n",
      "    surfaces: [claude-code]\n    artifact: notes.md\n",
    ),
  "skill-artifact-unique": (root) =>
    mutateFile(
      root,
      "skills/prototype/SKILL.md",
      "    surfaces: [claude-code]\n",
      "    surfaces: [claude-code]\n    artifact: plan.md\n",
    ),
  // content slice: retired-skill
  "retired-skill-reference": (root) =>
    writeFile(
      root,
      "docs/guide/overview.md",
      "# Overview\n\nRun `/ideation` first.\n",
    ),
  // content slice: ai-attribution
  "ai-attribution-trailer": (root) =>
    writeFile(
      root,
      "skills/prototype/references/commit-notes.md",
      "# Commit notes\n\nEnd every commit message with Assisted-by: Claude.\n",
    ),
  // content slice: independence
  "skill-repo-independence": (root) =>
    writeFile(
      root,
      "skills/prototype/references/layout.md",
      "# Layout\n\nSee hooks/README.md for the details.\n",
    ),
  // content slice: legacy-names
  "skill-legacy-names": (root) =>
    writeFile(
      root,
      "skills/prototype/references/state-notes.md",
      "# State notes\n\nRows go to events.tsv.\n",
    ),
  // workflow slice: question-format
  "question-format": (root) =>
    mutateFile(
      root,
      "skills/challenge/SKILL.md",
      COLLECTING_ANSWERS_PARAGRAPH,
      "",
    ),
  // workflow slice: workfolder
  "work-folder-source-present": (root) =>
    remove(root, WORK_FOLDER_CONTRACT_FILE),
  "work-folder-pin": (root) =>
    mutateFile(
      root,
      WORK_FOLDER_GATE_FILE,
      "A name collision keeps both copies",
      "",
    ),
  // workflow slice: spec-pass
  "spec-pass-pinned": (root) =>
    mutateFile(root, SPEC_AXIS_PATH, "## Per-requirement table", ""),
  // workflow slice: state-tsv
  "state-tsv-shape": (root) =>
    writeFile(
      root,
      "docs/specs/2026-01/feature-x/state.tsv",
      "# flow-state 2\nts\tsession\tphase\tevent\ttarget\tstatus\twhy\tevidence\tnext\n",
    ),
  // workflow slice: flow-emission
  "flow-emission-declared": (root) =>
    mutateFile(
      root,
      "skills/gate/SKILL.md",
      "agx flow",
      "skills/execute/scripts/flow.sh",
    ),
  // workflow slice: lean-cadence
  "lean-cadence-pins": (root) =>
    mutateFile(
      root,
      "skills/decompose/references/plan-template.md",
      "**Packet review:**",
      "",
    ),
  "lean-cadence-source-present": (root) =>
    remove(root, "skills/gh/references/stack.md"),
};

/** Replaces the first occurrence of `from` in a fixture file; throws when it is absent. */
function replaceFirst(
  root: string,
  rel: string,
  from: string,
  to: string,
): void {
  const text = readExisting(root, rel);
  if (!text.includes(from))
    throw new Error(
      `mutation target ${JSON.stringify(from)} not found in ${rel}`,
    );
  writeFile(
    root,
    rel,
    text.replace(from, () => to),
  );
}

/** Runs `body` on a fresh passing fixture and removes it afterwards, whatever happens. */
export async function withSkillsRepo<T>(
  body: (root: string) => Promise<T> | T,
  options?: Parameters<typeof cloneSkillsRepo>[0],
): Promise<T> {
  const root = cloneSkillsRepo({ sharedContracts: "linked", ...options });
  try {
    return await body(root);
  } finally {
    removeTree(root);
  }
}
