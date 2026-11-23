import assert from "node:assert/strict";
import { describe, test } from "node:test";
import { parse } from "yaml";
import {
  releaseEngineModel,
  releaseFrontmatterPolicyViolations,
} from "./testing/frontmatter.ts";
import { readRepoText } from "./testing/repo.ts";
import { replaceOnce } from "./testing/workflow.ts";

const workflow = readRepoText(".github/workflows/release.md");
const model = releaseEngineModel();

for (const [name, doc] of [
  ["source", parse(workflow.split("\n---\n")[0]!)],
  ["lock", parse(readRepoText(".github/workflows/release.lock.yml"))],
] as const) {
  test(`${name} gives the notes writer stage authority and only the approved content`, () => {
    const handoff =
      name === "source"
        ? doc["safe-outputs"].jobs["update-release-notes"]
        : doc.jobs.update_release_notes;
    const writer = doc.jobs["write-notes"];
    assert.deepEqual(handoff.permissions, {});
    assert.match(handoff.if, /needs\.detection\.result == 'success'/);
    assert.match(
      handoff.if,
      /needs\.detection\.outputs\.detection_success == 'true'/,
    );
    assert.deepEqual(writer.permissions, { contents: "write" });
    const dependencies = ["detection", "stage", "update_release_notes"];
    assert.deepEqual([...writer.needs].sort(), dependencies);
    for (const job of dependencies)
      assert.ok(writer.if.includes(`needs.${job}.result == 'success'`));
    assert.deepEqual(writer.env, {
      TARGET_SHA: "${{ needs.stage.outputs.target_sha }}",
      STATE_ARTIFACT_ID: "${{ needs.stage.outputs.state_artifact_id }}",
      STATE_SHA256: "${{ needs.stage.outputs.state_sha256 }}",
      RELEASE_STATE_FILE: "release-state/state.json",
      GH_AW_AGENT_OUTPUT: "notes-handoff/agent_output.json",
      BASELINE_NOTES: "release-context/baseline-notes.md",
      NOTES_ENGINE: doc.engine?.id ?? "copilot",
      NOTES_MODEL: model,
    });
    const step = (job: typeof writer, stepName: string) => {
      const step = job.steps.find((s: { name: string }) => s.name === stepName);
      assert.ok(step, stepName);
      assert.equal(step.if, undefined);
      assert.equal(step["continue-on-error"], undefined);
      return step;
    };
    const upload = step(handoff, "Upload detector-approved notes handoff");
    assert.match(upload.uses, /^actions\/upload-artifact@[0-9a-f]{40}$/);
    assert.equal(upload.with.name, "release-notes-handoff");
    assert.equal(upload.with.path, "notes-handoff/");
    assert.equal(upload.with.overwrite, false);
    assert.equal(upload.with["if-no-files-found"], "error");
    const download = step(writer, "Download detector-approved notes handoff");
    assert.deepEqual(download.with, {
      name: upload.with.name,
      path: "notes-handoff",
    });
    const identity = step(writer, "Download bound release identity");
    assert.match(identity.uses, /^actions\/download-artifact@[0-9a-f]{40}$/);
    assert.equal(download.uses, identity.uses);
    assert.deepEqual(identity.with, {
      "artifact-ids": "${{ needs.stage.outputs.state_artifact_id }}",
      "merge-multiple": true,
      path: "release-state",
    });
    const checkout = step(writer, "Checkout immutable target");
    assert.deepEqual(checkout.with, {
      path: "agentics-source",
      "persist-credentials": false,
      ref: "${{ needs.stage.outputs.target_sha }}",
    });
    const adapter = step(writer, "Update only the release notes");
    assert.deepEqual(adapter.env, {
      GH_REPO: "${{ github.repository }}",
      GH_TOKEN: "${{ github.token }}",
    });
    assert.equal(
      adapter.run,
      "node agentics-source/.github/tools/src/scripts/release-notes.ts",
    );
    const order = [
      "Require prepared release binding",
      "Download bound release identity",
      "Verify bound release identity",
      "Download detector-approved notes handoff",
      "Read safe-output staged flag",
      "Checkout immutable target",
      "Update only the release notes",
    ].map((name) => writer.steps.indexOf(step(writer, name)));
    assert.deepEqual(
      order,
      [...order].sort((a, b) => a - b),
    );
    if (name === "source")
      assert.deepEqual(Object.keys(handoff.inputs), ["body"]);
    else
      assert.equal(
        step(handoff, "Prepare detector-approved notes handoff").env
          .GH_AW_AGENT_OUTPUT,
        "${{ runner.temp }}/gh-aw/safe-jobs/agent_output.json",
      );
  });
}

// Pins the model and credit caps in release.md's frontmatter.
test("release.md frontmatter holds the model and credit policy", () => {
  assert.deepEqual(releaseFrontmatterPolicyViolations(workflow), []);
});

test("the shared engine model is a model name", () => {
  assert.match(model, /^[a-z0-9][a-z0-9.-]*$/);
});

const policy =
  "engine:\n  model: " +
  model +
  "\nmax-ai-credits: 100\nsafe-outputs:\n  threat-detection:\n" +
  "    enabled: true\n    engine:\n      model: " +
  model +
  "\n    max-ai-credits: 50\n";
const front = (body: string): string => "---\n" + body + "---\nWorkflow body\n";
const commented =
  "# " + policy.replace(/\n$/, "").split("\n").join("\n# ") + "\n";
const nested =
  "unrelated:\n  " + policy.replace(/\n$/, "").split("\n").join("\n  ") + "\n";

const rootModel = "engine.model must be " + model;
const rootCredits = "max-ai-credits must be 100";
const detectorOn = "safe-outputs.threat-detection.enabled must be true";
const detectorModel =
  "safe-outputs.threat-detection.engine.model must be " + model;
const detectorCredits =
  "safe-outputs.threat-detection.max-ai-credits must be 50";
const everyRule = [
  rootModel,
  rootCredits,
  detectorOn,
  detectorModel,
  detectorCredits,
];

function swapCredits(text: string): string {
  return text
    .replace("max-ai-credits: 100", "max-ai-credits: @@")
    .replace("max-ai-credits: 50", "max-ai-credits: 100")
    .replace("max-ai-credits: @@", "max-ai-credits: 50");
}

// Each case breaks one rule and requires exactly the
// violations that name it, so a check that is removed or loosened fails its own case rather than hiding behind another.
const named: [string, string, string[]][] = [
  [
    "real root credits 101",
    replaceOnce(workflow, "\nmax-ai-credits: 100\n", "\nmax-ai-credits: 101\n"),
    [rootCredits],
  ],
  [
    "real detector disabled",
    replaceOnce(
      workflow,
      "        enabled: true\n",
      "        enabled: false\n",
    ),
    [detectorOn],
  ],
  [
    "root model",
    front(policy.replace("  model: " + model, "  model: wrong")),
    [rootModel],
  ],
  [
    "root credits 99",
    front(policy.replace("max-ai-credits: 100", "max-ai-credits: 99")),
    [rootCredits],
  ],
  [
    "fractional root credits",
    front(policy.replace("max-ai-credits: 100", "max-ai-credits: 100.5")),
    [rootCredits],
  ],
  [
    "quoted root credits",
    front(policy.replace("max-ai-credits: 100", 'max-ai-credits: "100"')),
    [rootCredits],
  ],
  [
    "detector disabled",
    front(policy.replace("enabled: true", "enabled: false")),
    [detectorOn],
  ],
  [
    "detector model",
    front(policy.replace("      model: " + model, "      model: wrong")),
    [detectorModel],
  ],
  [
    "detector credits 49",
    front(policy.replace("max-ai-credits: 50", "max-ai-credits: 49")),
    [detectorCredits],
  ],
  [
    "fractional detector credits",
    front(policy.replace("max-ai-credits: 50", "max-ai-credits: 50.5")),
    [detectorCredits],
  ],
  [
    "swapped credits",
    front(swapCredits(policy)),
    [rootCredits, detectorCredits],
  ],
  ["comments only", front(commented), everyRule],
  ["wrong nesting", front(nested), everyRule],
  ["policy in the body only", "---\nname: release\n---\n" + policy, everyRule],
  ["missing frontmatter", policy, ["missing workflow frontmatter"]],
  ["unclosed frontmatter", "---\n" + policy, ["unclosed workflow frontmatter"]],
];

describe("frontmatter policy mutations are caught", () => {
  for (const [name, source, want] of named) {
    test(name, () => {
      assert.deepEqual(releaseFrontmatterPolicyViolations(source), want);
    });
  }
});

describe("frontmatter policy rejects malformed frontmatter", () => {
  for (const [name, source] of [
    ["duplicate key", front(policy + "max-ai-credits: 100\n")],
    ["malformed YAML", front(policy + "broken: [\n")],
  ] as const) {
    test(name, () => {
      const got = releaseFrontmatterPolicyViolations(source);
      assert.equal(got.length, 1);
      assert.ok(
        (got[0] ?? "").startsWith("invalid workflow frontmatter: "),
        got.join(),
      );
    });
  }
});

const valid: [string, string][] = [
  ["synthetic", front(policy)],
  ["CRLF", front(policy).replace(/\n/g, "\r\n")],
  [
    "hexadecimal cap",
    front(
      policy
        .replace("max-ai-credits: 100", "max-ai-credits: 0x64")
        .replace("max-ai-credits: 50", "max-ai-credits: 0x32"),
    ),
  ],
];
describe("frontmatter policy accepts valid frontmatter", () => {
  for (const [name, source] of valid) {
    test(name, () => {
      assert.deepEqual(releaseFrontmatterPolicyViolations(source), []);
    });
  }
});

// A different model must change every model rule.
test("a policy written for another model is rejected against the shared constant", () => {
  const other = front(policy.split(model).join("another-model"));
  assert.deepEqual(releaseFrontmatterPolicyViolations(other), [
    rootModel,
    detectorModel,
  ]);
  assert.deepEqual(
    releaseFrontmatterPolicyViolations(other, "another-model"),
    [],
  );
});
