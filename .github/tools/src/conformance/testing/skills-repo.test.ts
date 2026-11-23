import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { existsSync, lstatSync, readdirSync, readFileSync } from "node:fs";
import path from "node:path";
import { test } from "node:test";
import {
  addSkill,
  auditSkills,
  BREAKERS,
  cloneArtifactRepo,
  cloneEvalMetadataRepo,
  cloneInlineRepo,
  cloneOrchestrationRepo,
  cloneReferenceRepo,
  cloneSkillsRepo,
  cloneTriageRepo,
  evalsCheck,
  link,
  mkdir,
  mutateFile,
  remove,
  skillMD,
  withSkillsRepo,
  writeFile,
} from "./skills-repo.ts";
import { writeStandardsBaseline } from "./standards-baseline.ts";
import { canSymlink, removeTree, SYMLINK_SKIP } from "./host.ts";

type Pin = readonly [rel: string, sha256: string];

// A snapshot of each builder's output: every file it writes, in listing order, with its SHA-256. A builder change that
// alters any fixture byte fails here, so a family test cannot lose its expected input unnoticed.
const PINS: Record<string, readonly Pin[]> = {
  green: [
    [
      "AGENTS.md",
      "a0d07f28ece7b227ffcb8984f8975e75f2ce86e35a79790a7f57b0dd8ffc1cb8",
    ],
    [
      "README.md",
      "a0d07f28ece7b227ffcb8984f8975e75f2ce86e35a79790a7f57b0dd8ffc1cb8",
    ],
    [
      "THIRD_PARTY_NOTICES.md",
      "7c37f7b4daf226450b6c9484598e588cbcdebe9b3136df2d3c56dd5c533aa031",
    ],
    [
      "docs/ARCHITECTURE.md",
      "a0d07f28ece7b227ffcb8984f8975e75f2ce86e35a79790a7f57b0dd8ffc1cb8",
    ],
    [
      "docs/surfaces/claude-code.md",
      "5c11921b8e39523b63424afd23da25fc4c9baf9d3df32224f15c9c3c60915cbf",
    ],
    [
      "docs/surfaces/codex.md",
      "5c11921b8e39523b63424afd23da25fc4c9baf9d3df32224f15c9c3c60915cbf",
    ],
    [
      "docs/surfaces/copilot-cli.md",
      "5c11921b8e39523b63424afd23da25fc4c9baf9d3df32224f15c9c3c60915cbf",
    ],
    [
      "docs/surfaces/cursor.md",
      "5c11921b8e39523b63424afd23da25fc4c9baf9d3df32224f15c9c3c60915cbf",
    ],
    [
      "docs/surfaces/vscode.md",
      "5c11921b8e39523b63424afd23da25fc4c9baf9d3df32224f15c9c3c60915cbf",
    ],
    [
      "skills/challenge/SKILL.md",
      "8cce13cf0c65518f28ac1089cd395faec8de6d159261bc6ae345c373e2431b6a",
    ],
    [
      "skills/decompose/SKILL.md",
      "e809338be4f8105e599d4e7cdf6b1b482108df714e15da254773f34f08be928b",
    ],
    [
      "skills/decompose/references/chain-artifacts.md",
      "c4984af3dff312c474af1910ce70edb45b757847f975041dfe651aeb5cea9b5f",
    ],
    [
      "skills/decompose/references/plan-template.md",
      "8789e560ea8f8a8de3035e170a36ed66a498b2361f1750b56377db22a7b59bf3",
    ],
    [
      "skills/docs/SKILL.md",
      "aed3f612807264f5d259353aa216eded10730b07b1133efb1873fdd86a2def24",
    ],
    [
      "skills/execute/SKILL.md",
      "8f493431ee64f745779fdffe067539ea8030e4a77e44382e3d2465073f649738",
    ],
    [
      "skills/execute/references/flow-state.md",
      "25d831c09a37b64d0e0ed57fbdba80d65710baf16f79904a0fcbd8f69acaf8db",
    ],
    [
      "skills/execute/references/stage-1-dispatch-rule.md",
      "2850ffbd481b18f7222a29a62a240d2ba85bf73c00365d4f416a0a2ae75a9598",
    ],
    [
      "skills/forge/SKILL.md",
      "9885b1a9854dc9881bbdbd9ae67fb67621f6cab728e8a11b9dcb36265b98f42e",
    ],
    [
      "skills/gate/SKILL.md",
      "6e1622b06ce33f8501ebae2cc254dc66b6b24e7186eac0e8e2faa4667e3c9110",
    ],
    [
      "skills/gate/references/branch/review-and-audit.md",
      "3f1d33d4825ff7ec6f48b03741fd1f59c6ae9f08c1d350cd028186568d5e73d3",
    ],
    [
      "skills/gate/references/branch/tidy.md",
      "11bb4d42eee5f4718217dee39dcebe378452a24dce8dc2c4219be491b806447b",
    ],
    [
      "skills/gh/SKILL.md",
      "eaebce10740450aeb29a753fbcd963b7df0ededfadce149ac8c6844a2f1dfa2d",
    ],
    [
      "skills/gh/references/stack.md",
      "d0c54e375daca6d67677d6c100d22e176c80883da709f0d92d1e1f6b7ef81626",
    ],
    [
      "skills/prototype/SKILL.md",
      "96a524222eb2b028e651502a0c1b11e5b568bcef973c8be855829f27849224b8",
    ],
    [
      "skills/review/SKILL.md",
      "1ba5bc64dba148f3938d8406579e402d80769da2382665ac0a5f9b6e707cdb1c",
    ],
    [
      "skills/review/references/axes/axes-mode.md",
      "7879512c9845dfd7512d6983e59d9b6d556b1c33170318c9f1ea162956217df4",
    ],
    [
      "skills/review/references/axes/spec-axis.md",
      "fb1008a04d24109854128735e75663c07fed99dbd758906e89bd8ba1e7f2ad56",
    ],
    [
      "skills/routing/SKILL.md",
      "c51a490aa00c7319da33774cea7951bf56e5ff030fc0ec57d29072cad6a5a49b",
    ],
    [
      "skills/routing/references/contracts/question-format.md",
      "19d7e7b8608a83a9a7578bf6f893a3e6cff1ec816c8be7ea79bfdbecfb116606",
    ],
    [
      "skills/routing/references/copilot-tools.md",
      "5c11921b8e39523b63424afd23da25fc4c9baf9d3df32224f15c9c3c60915cbf",
    ],
    [
      "skills/routing/references/model-ladders-examples.md",
      "5c11921b8e39523b63424afd23da25fc4c9baf9d3df32224f15c9c3c60915cbf",
    ],
    [
      "skills/routing/references/model-ladders.md",
      "5c11921b8e39523b63424afd23da25fc4c9baf9d3df32224f15c9c3c60915cbf",
    ],
    [
      "skills/routing/references/surface-dispatch.md",
      "5c11921b8e39523b63424afd23da25fc4c9baf9d3df32224f15c9c3c60915cbf",
    ],
    [
      "skills/triage/SKILL.md",
      "c0a9b6a484d1f75ec454d8d906ff1c881b8e5384164ec06fde16c8719720db65",
    ],
    [
      "skills/triage/references/checks/checks-triage.md",
      "cb771f13710239bb93f787636e651cf39083d8f29ff63397400f5de01cc5671d",
    ],
    [
      "skills/triage/references/issue-mode.md",
      "8bfcdf747d92a24cd941f39f0c5b5a3b1ad75d5baa4edce08e22cf7d51f77ac2",
    ],
    [
      "skills/triage/references/orchestration-lifecycle.md",
      "de1e7483cfb40ba9c884996f82171cae38afd28ff2985647ed132f49f524a4e5",
    ],
    [
      "skills/triage/references/pull-requests/copilot/copilot-triage.md",
      "8c925b826e7d21557cef87cb8a97475d70694c36bf2fe5724f2b6a60896c2b21",
    ],
    [
      "skills/triage/references/rollup-template.md",
      "b60177ef8ebb76dfb8c9b54518b06b5809affd8b8a37caa8cff7743308c3dbf9",
    ],
    [
      "skills/triage/references/source-detection.md",
      "655e39598a9188341f28f0053f6ebc9b8de076032f9beada6347c3314bd76718",
    ],
    [
      "skills/triage/references/subagent-contract.md",
      "20e9c220be64ae5f58aa71dd22918004a3498c2b7d69b12b8524c2f43097985a",
    ],
  ],
  inline: [
    [
      "skills/tdd/SKILL.md",
      "99230807612f8ac81b11c7a41050a91008149efd52dd29d1b3f94208f579dc4b",
    ],
  ],
  evalMetadata: [
    [
      "evals/skills.yaml",
      "ca7aa0bf6d46f8360f27f6ce490694321ee7ae3def19d9dc216c07e7d4e8084d",
    ],
    [
      "skills/tdd/SKILL.md",
      "99230807612f8ac81b11c7a41050a91008149efd52dd29d1b3f94208f579dc4b",
    ],
  ],
  reference: [
    [
      "THIRD_PARTY_NOTICES.md",
      "7c37f7b4daf226450b6c9484598e588cbcdebe9b3136df2d3c56dd5c533aa031",
    ],
    [
      "skills/docs/SKILL.md",
      "aed3f612807264f5d259353aa216eded10730b07b1133efb1873fdd86a2def24",
    ],
    [
      "skills/review/references/axes/axes-mode.md",
      "4bf0a6c2b3004b26baf01f6fde3a6dd44fc631766c768812d4f3d3d40aa81f3c",
    ],
    [
      "skills/tdd/SKILL.md",
      "99230807612f8ac81b11c7a41050a91008149efd52dd29d1b3f94208f579dc4b",
    ],
    [
      "skills/tdd/references/example.md",
      "8ef0b048ad51a9d2ccaf9177716d7c83159a1d8c22fd37aa2b7d03c11b21b40d",
    ],
  ],
  triage: [
    [
      "skills/triage/SKILL.md",
      "c0a9b6a484d1f75ec454d8d906ff1c881b8e5384164ec06fde16c8719720db65",
    ],
    [
      "skills/triage/references/checks/checks-triage.md",
      "cb771f13710239bb93f787636e651cf39083d8f29ff63397400f5de01cc5671d",
    ],
    [
      "skills/triage/references/issue-mode.md",
      "8bfcdf747d92a24cd941f39f0c5b5a3b1ad75d5baa4edce08e22cf7d51f77ac2",
    ],
    [
      "skills/triage/references/orchestration-lifecycle.md",
      "de1e7483cfb40ba9c884996f82171cae38afd28ff2985647ed132f49f524a4e5",
    ],
    [
      "skills/triage/references/pull-requests/copilot/copilot-triage.md",
      "8c925b826e7d21557cef87cb8a97475d70694c36bf2fe5724f2b6a60896c2b21",
    ],
    [
      "skills/triage/references/rollup-template.md",
      "b60177ef8ebb76dfb8c9b54518b06b5809affd8b8a37caa8cff7743308c3dbf9",
    ],
    [
      "skills/triage/references/source-detection.md",
      "655e39598a9188341f28f0053f6ebc9b8de076032f9beada6347c3314bd76718",
    ],
    [
      "skills/triage/references/subagent-contract.md",
      "20e9c220be64ae5f58aa71dd22918004a3498c2b7d69b12b8524c2f43097985a",
    ],
  ],
  orchestration: [
    [
      "AGENTS.md",
      "a8be4fd1112d51b2b2249ffeb1798a33cab71a9aacab353e65dea268d7641916",
    ],
    [
      "README.md",
      "a8be4fd1112d51b2b2249ffeb1798a33cab71a9aacab353e65dea268d7641916",
    ],
    [
      "docs/ARCHITECTURE.md",
      "a8be4fd1112d51b2b2249ffeb1798a33cab71a9aacab353e65dea268d7641916",
    ],
    [
      "docs/surfaces/claude-code.md",
      "5c11921b8e39523b63424afd23da25fc4c9baf9d3df32224f15c9c3c60915cbf",
    ],
    [
      "docs/surfaces/codex.md",
      "5c11921b8e39523b63424afd23da25fc4c9baf9d3df32224f15c9c3c60915cbf",
    ],
    [
      "docs/surfaces/copilot-cli.md",
      "5c11921b8e39523b63424afd23da25fc4c9baf9d3df32224f15c9c3c60915cbf",
    ],
    [
      "docs/surfaces/cursor.md",
      "5c11921b8e39523b63424afd23da25fc4c9baf9d3df32224f15c9c3c60915cbf",
    ],
    [
      "docs/surfaces/vscode.md",
      "5c11921b8e39523b63424afd23da25fc4c9baf9d3df32224f15c9c3c60915cbf",
    ],
    [
      "skills/routing/references/copilot-tools.md",
      "351fb55b7cb5e0ae66a77dc452e62d668f213b72c1cc71985db8493e007aae67",
    ],
    [
      "skills/routing/references/model-ladders-examples.md",
      "5c11921b8e39523b63424afd23da25fc4c9baf9d3df32224f15c9c3c60915cbf",
    ],
    [
      "skills/routing/references/model-ladders.md",
      "5c11921b8e39523b63424afd23da25fc4c9baf9d3df32224f15c9c3c60915cbf",
    ],
    [
      "skills/routing/references/surface-dispatch.md",
      "2e8843c7ebaab28c4a8c35e9c0f0033b22552410a2cd0899ec790714d66206a5",
    ],
    [
      "skills/test/SKILL.md",
      "c5a6a19a79154609f3f5fb925c8593c9436319c5a2c7a91c801973bc4f017648",
    ],
  ],
  artifact: [
    [
      "skills/decompose/SKILL.md",
      "558ba40e2f32aed3d5c9114b9c6e42e926255dfd4aa721437612dbaa21f34eb5",
    ],
    [
      "skills/intent/SKILL.md",
      "dd5416392de823cc34197522f5dbaee744aa01fcd671141cfc98cafa085c90ef",
    ],
    [
      "skills/spec/SKILL.md",
      "6d27fc240a83d2260048b908b5b69b6670ca137c8c4330edca8739ee6f32807a",
    ],
    [
      "skills/tdd/SKILL.md",
      "99230807612f8ac81b11c7a41050a91008149efd52dd29d1b3f94208f579dc4b",
    ],
  ],
};

/** The sorted repo-relative path and SHA-256 of every file below `root`. */
function listing(root: string): Pin[] {
  const found: Pin[] = [];
  for (const entry of readdirSync(root, {
    recursive: true,
    withFileTypes: true,
  })) {
    if (!entry.isFile()) continue;
    const full = path.join(entry.parentPath, entry.name);
    found.push([
      path.relative(root, full).split(path.sep).join("/"),
      createHash("sha256").update(readFileSync(full)).digest("hex"),
    ]);
  }
  return found.sort((a, b) => (a[0] < b[0] ? -1 : a[0] > b[0] ? 1 : 0));
}

/** The first way `actual` differs from `expected`, or an empty string when they are equal. */
function firstDifference(
  actual: readonly Pin[],
  expected: readonly Pin[],
): string {
  const have = new Map(actual);
  const want = new Map(expected);
  for (const [rel, sha] of expected) {
    const got = have.get(rel);
    if (got === undefined) return `missing file ${rel}`;
    if (got !== sha) return `${rel} differs: expected ${sha}, got ${got}`;
  }
  for (const [rel] of actual)
    if (!want.has(rel)) return `unexpected file ${rel}`;
  return "";
}

function cleanUp(root: string): void {
  removeTree(root);
}

function assertBuilder(
  name: string,
  build: () => string,
  expected: readonly Pin[],
): void {
  test(`the ${name} builder writes its snapshot, byte for byte`, () => {
    const root = build();
    try {
      assert.equal(firstDifference(listing(root), expected), "");
      assert.deepEqual(
        listing(root).map(([rel]) => rel),
        expected.map(([rel]) => rel),
      );
    } finally {
      cleanUp(root);
    }
  });
}

const pins = (name: string): readonly Pin[] => PINS[name] ?? [];

assertBuilder(
  "green (cloneSkillsRepo without the shared contract files)",
  () => cloneSkillsRepo({ sharedContracts: "absent" }),
  pins("green"),
);
assertBuilder("inline", cloneInlineRepo, pins("inline"));
assertBuilder("eval-metadata", cloneEvalMetadataRepo, pins("evalMetadata"));
assertBuilder("reference", cloneReferenceRepo, pins("reference"));
assertBuilder("triage", cloneTriageRepo, pins("triage"));
assertBuilder("orchestration", cloneOrchestrationRepo, pins("orchestration"));
assertBuilder("artifact", cloneArtifactRepo, pins("artifact"));

test("every pin table is present and the fixture source is ASCII", () => {
  for (const name of [
    "green",
    "inline",
    "evalMetadata",
    "reference",
    "triage",
    "orchestration",
    "artifact",
  ])
    assert.ok(pins(name).length > 0, name);
  const source = readFileSync(
    new URL("./skills-repo.ts", import.meta.url),
    "utf8",
  );
  assert.equal(/[^\x00-\x7f]/.test(source), false);
});

test("firstDifference names the first differing path", () => {
  const expected: Pin[] = [
    ["a", "1"],
    ["b", "2"],
    ["c", "3"],
  ];
  assert.equal(firstDifference(expected, expected), "");
  assert.match(
    firstDifference(
      [
        ["a", "1"],
        ["b", "x"],
        ["c", "y"],
      ],
      expected,
    ),
    /^b differs/,
  );
  assert.match(firstDifference([["a", "1"]], expected), /^missing file b/);
  assert.match(
    firstDifference([...expected, ["d", "4"]], expected),
    /^unexpected file d/,
  );
});

test("cloneSkillsRepo gives each call its own directory", () => {
  const first = cloneSkillsRepo();
  const second = cloneSkillsRepo();
  try {
    assert.notEqual(first, second);
    assert.ok(path.basename(first).startsWith("agentics-skillsrepo-"));
    writeFile(first, "only-in-first.md", "x");
    assert.equal(existsSync(path.join(second, "only-in-first.md")), false);
  } finally {
    cleanUp(first);
    cleanUp(second);
  }
});

test("withSkillsRepo removes the directory after the body, even when the body throws", async () => {
  let seen = "";
  await assert.rejects(
    withSkillsRepo((root) => {
      seen = root;
      assert.ok(existsSync(path.join(root, "skills", "triage", "SKILL.md")));
      throw new Error("body failed");
    }),
    /body failed/,
  );
  assert.equal(existsSync(seen), false);
  let kept = "";
  const result = await withSkillsRepo(async (root) => {
    kept = root;
    return "done";
  });
  assert.equal(result, "done");
  assert.equal(existsSync(kept), false);
});

test("addSkill writes a skill in the fixture's frontmatter shape", async () => {
  await withSkillsRepo((root) => {
    assert.equal(existsSync(path.join(root, "skills", "tdd")), false);
    addSkill(root, "tdd");
    assert.equal(
      readFileSync(path.join(root, "skills", "tdd", "SKILL.md"), "utf8"),
      skillMD("tdd"),
    );
    addSkill(root, "intent");
    assert.equal(
      readFileSync(path.join(root, "skills", "intent", "SKILL.md"), "utf8"),
      skillMD("intent"),
    );
    assert.match(
      readFileSync(path.join(root, "skills", "intent", "SKILL.md"), "utf8"),
      /\n {4}artifact: intent\.md\n/,
    );
  });
});

test("addSkill takes an artifact and a body", async () => {
  await withSkillsRepo((root) => {
    addSkill(root, "ops", { artifact: "runbook.md", body: "Custom body.\n" });
    assert.equal(
      readFileSync(path.join(root, "skills", "ops", "SKILL.md"), "utf8"),
      '---\nname: ops\ndescription: Test skill for ops.\nversion: "1.0.0"\nmetadata:\n    surfaces: [claude-code]\n    artifact: runbook.md\n---\n\n# ops\n\nCustom body.\n',
    );
    addSkill(root, "spec", { artifact: null });
    assert.equal(
      readFileSync(
        path.join(root, "skills", "spec", "SKILL.md"),
        "utf8",
      ).includes("artifact:"),
      false,
    );
  });
});

test("the fixture holds none of the skills addSkill exists for", async () => {
  await withSkillsRepo((root) => {
    const skills = readdirSync(path.join(root, "skills")).sort();
    assert.deepEqual(skills, [
      "challenge",
      "decompose",
      "docs",
      "execute",
      "forge",
      "gate",
      "gh",
      "prototype",
      "review",
      "routing",
      "triage",
    ]);
  });
});

test("mkdir creates parents, remove accepts a path that is gone, and mutateFile changes a file", async () => {
  await withSkillsRepo((root) => {
    mkdir(root, "a/b/c");
    assert.ok(existsSync(path.join(root, "a", "b", "c")));
    remove(root, "a");
    assert.equal(existsSync(path.join(root, "a")), false);
    remove(root, "a");
    mutateFile(root, "AGENTS.md", "Intent only.", "Changed.");
    assert.equal(
      readFileSync(path.join(root, "AGENTS.md"), "utf8"),
      "# Shared\n\nChanged.\n",
    );
    assert.throws(
      () => mutateFile(root, "AGENTS.md", "not present", "x"),
      /mutation target "not present" not found in AGENTS\.md/,
    );
  });
});

test("BREAKERS maps each rule id to a mutator of a fixture clone", () => {
  const typed: Readonly<Record<string, (root: string) => void>> = BREAKERS;
  assert.ok(Object.keys(typed).length > 0);
  for (const [rule, mutate] of Object.entries(typed)) {
    assert.match(rule, /^[a-z]+(-[a-z]+)*$/, rule);
    assert.equal(typeof mutate, "function", rule);
  }
});

test("link reports whether the host can create a symlink and leaves no stray file when it cannot", async () => {
  await withSkillsRepo((root) => {
    const made = link(root, "skills/linked", "docs");
    assert.equal(made, canSymlink());
    if (!made) {
      assert.throws(() => lstatSync(path.join(root, "skills", "linked")));
      return;
    }
    assert.ok(lstatSync(path.join(root, "skills", "linked")).isSymbolicLink());
  });
  if (!canSymlink())
    console.log(
      "link: this host cannot create a symlink, so the creating branch was not exercised",
    );
});

test(
  "link creates a symlink that points at its target",
  { skip: canSymlink() ? false : SYMLINK_SKIP },
  async () => {
    await withSkillsRepo((root) => {
      assert.equal(link(root, "docs/linked.md", "ARCHITECTURE.md"), true);
      const at = path.join(root, "docs", "linked.md");
      assert.ok(lstatSync(at).isSymbolicLink());
      assert.equal(readFileSync(at, "utf8"), "# Shared\n\nIntent only.\n");
    });
  },
);

test("the clean evalsCheck stub resolves to no failures", async () => {
  assert.deepEqual(await evalsCheck("/any/root"), []);
});

test("auditSkills finds no failures and no error on the clean fixture with the standards baseline written", async () => {
  await withSkillsRepo(async (root) => {
    writeStandardsBaseline(root);
    assert.deepEqual(await auditSkills(root), { failures: [] });
  });
});
