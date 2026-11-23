import assert from "node:assert/strict";
import test from "node:test";
import { AREAS as repositoryAreas, AREA_ROOTS } from "../repository-areas.ts";
import {
  AREAS,
  SHARDS,
  SHARD_INPUTS,
  TS_SHARDS,
  TS_SHARD_INPUTS,
  RACE_INPUTS,
  areaOf,
  planFor,
} from "./ci-areas.ts";

test("the four areas and hook shard are fixed and ordered", () => {
  assert.equal(AREAS, repositoryAreas);
  assert.deepEqual(AREAS, ["hooks", "evals", "tools", "skills"]);
  assert.deepEqual(AREA_ROOTS, {
    hooks: "hooks",
    evals: "evals",
    tools: ".github/tools",
    skills: "skills",
  });
  assert.deepEqual(SHARDS, ["hooks"]);
});

test("TypeScript shard names and inputs are fixed and ordered", () => {
  assert.deepEqual(TS_SHARDS, ["ts-tools"]);
  assert.deepEqual(TS_SHARD_INPUTS, {
    "ts-tools": ["tools", "hooks", "evals"],
  });
});

test("a skills-only change runs the hooks shard but no TypeScript suite and no race job", () => {
  // The tools tests that read the real skills tree repeat the conformance job, which always runs. Only the hook guard sweep over the skill commands depends on skills.
  assert.deepEqual(planFor(["skills"]), {
    shards: ["hooks"],
    tsShards: [],
    race: false,
  });
  assert.ok(!RACE_INPUTS.includes("skills"));
});

test("canonical and legacy directory paths map to their area", () => {
  assert.equal(areaOf("hooks/go/cmd/hook/main.go"), "hooks");
  assert.equal(areaOf("evals/references/example.md"), "evals");
  assert.equal(areaOf("skills/diagnose/references/find-polluter.sh"), "skills");
  assert.equal(areaOf(".github/tools/src/file.ts"), "tools");
  assert.equal(areaOf("tools/src/file.ts"), "tools");
  assert.equal(areaOf(".github/tools"), null);
  assert.equal(areaOf(".github/tools-extra/src/file.ts"), null);
  assert.equal(areaOf("lib/src/index.ts"), null);
  assert.equal(areaOf("hooks"), null);
  assert.equal(areaOf("docs/x.md"), null);
  assert.equal(areaOf(".github/workflows/validate.yml"), null);
  assert.equal(areaOf("package.json"), null);
});

test("scripts and the two exact inventory paths retain CI infrastructure scope", () => {
  assert.equal(areaOf(".github/tools/src/scripts/check.ts"), null);
  assert.equal(areaOf(".github/tools/src/plugin-manifests.ts"), null);
  assert.equal(areaOf(".github/tools/src/repository-areas.ts"), null);
  assert.equal(areaOf(".github/tools/src/plugin-manifests.test.ts"), "tools");
  assert.equal(areaOf(".github/tools/src/repository-areas.test.ts"), "tools");
});

test("the plan matches the spec table for single-area changes", () => {
  const rows = [
    [["hooks"], ["hooks"], true],
    [["evals"], [], false],
    [["tools"], [], false],
    [["skills"], [...SHARDS], false],
  ] as const;
  const tsByArea: Record<string, readonly string[]> = {
    hooks: ["ts-tools"],
    evals: ["ts-tools"],
    tools: ["ts-tools"],
    skills: [],
  };
  for (const [areas, shards, race] of rows) {
    assert.deepEqual(
      planFor(areas),
      { shards, tsShards: tsByArea[areas[0]!], race },
      areas.join(),
    );
  }
});

test("every subset of areas runs exactly the shards whose inputs changed", () => {
  for (let mask = 0; mask < 1 << AREAS.length; mask += 1) {
    const subset: readonly string[] = AREAS.filter((_, i) => mask & (1 << i));
    const plan = planFor(subset);
    for (const shard of SHARDS) {
      const needed = SHARD_INPUTS[shard].some((a) => subset.includes(a));
      assert.equal(
        plan.shards.includes(shard),
        needed,
        `${shard} for ${subset.join()}`,
      );
    }
    for (const shard of TS_SHARDS) {
      assert.equal(
        plan.tsShards.includes(shard),
        TS_SHARD_INPUTS[shard].some((area) => subset.includes(area)),
        `${shard} for ${subset.join()}`,
      );
    }
    assert.equal(
      plan.race,
      RACE_INPUTS.some((a) => subset.includes(a)),
      `race for ${subset.join()}`,
    );
  }
});

test("no area is left out of every shard, so a change always runs something", () => {
  for (const area of AREAS)
    assert.ok(
      planFor([area]).shards.length + planFor([area]).tsShards.length > 0,
      area,
    );
});
