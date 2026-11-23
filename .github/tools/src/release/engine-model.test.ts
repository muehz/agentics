import assert from "node:assert/strict";
import { test } from "node:test";
import { parse } from "yaml";
import { ReleaseEngineModel } from "./engine-model.ts";
import { readRepoText } from "./testing/repo.ts";

test("the release engine model matches the first Copilot ladder model", () => {
  const row = readRepoText("skills/routing/references/model-ladders.md")
    .split("\n")
    .find((line) => line.startsWith("| Copilot CLI and VS Code |"));
  assert.ok(row, "Copilot model ladder row is missing");
  const first = /`([^`]+)`/.exec(row)?.[1];
  assert.ok(first, "Copilot ladder row has no model");
  assert.equal(ReleaseEngineModel, first);
});

interface ReleaseWorkflow {
  engine?: { id?: unknown; model?: unknown };
  "safe-outputs"?: {
    "threat-detection"?: { engine?: { model?: unknown } };
  };
  jobs?: Record<string, { env?: Record<string, unknown> }>;
}

function readReleaseWorkflow(): ReleaseWorkflow {
  const text = readRepoText(".github/workflows/release.md");
  assert.ok(text.startsWith("---\n"));
  const end = text.indexOf("\n---\n", 4);
  assert.ok(end > 0, "release workflow frontmatter is unclosed");
  return parse(text.slice(4, end)) as ReleaseWorkflow;
}

test("release workflow engine and detector pin the shared model", () => {
  const doc = readReleaseWorkflow();
  assert.equal(doc.engine?.model, ReleaseEngineModel);
  assert.equal(
    doc["safe-outputs"]?.["threat-detection"]?.engine?.model,
    ReleaseEngineModel,
  );
});

// The notes footer names the engine and model that drafted the notes. The writer cannot read the engine block,
// so it restates the two values as environment variables, and they must not drift from the block.
test("the notes job restates the workflow's engine and model", () => {
  const doc = readReleaseWorkflow();
  const env = doc.jobs?.["write-notes"]?.env;
  assert.ok(env, "the write-notes job has no env");
  assert.equal(typeof doc.engine?.id, "string");
  assert.equal(env["NOTES_ENGINE"], doc.engine?.id);
  assert.equal(env["NOTES_MODEL"], doc.engine?.model);
});
