import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import path from "node:path";
import { test } from "node:test";
import type { RunOptions } from "../../check.ts";
import {
  evalsCheck as cleanEvalsCheck,
  skillMD,
  withSkillsRepo,
  writeFile,
} from "../../testing/skills-repo.ts";
import { writeStandardsBaseline } from "../../testing/standards-baseline.ts";
import { BASELINE_DIR, BASELINE_STALE_RULE } from "./baseline.ts";
import { DUPLICATE_RULE } from "./duplication.ts";
import { FAMILY as EVAL_CHECK_FAMILY } from "./eval-check.ts";
import { KEY_RULE, VALUE_RULE } from "./frontmatter-std.ts";
import {
  DOMAIN_FAMILIES,
  FAMILIES,
  RULE_FAMILY,
  ruleFamilyOf,
  run,
  type ResolvedSkillsContext,
} from "./index.ts";
import { METADATA_RULE } from "./skill-meta.ts";
import { FAMILY as SHARED_CONTRACTS_FAMILY } from "./shared-contracts.ts";
import { FAMILY as RECORD_KEEPING_FAMILY } from "./record-keeping.ts";
import { FAMILY as WAIVE_OPERANDS_FAMILY } from "./waive-operands.ts";
import { FAMILY as SHELL_DATA_FAMILY } from "./shell-data.ts";
import { FAMILY as FILE_SIZE_FAMILY } from "./file-size.ts";
import { FAMILY as INBOUND_ANCHOR_FAMILY } from "./inbound-anchor.ts";
import {
  BASELINED_RULES,
  collectStandardFindings,
  STANDARDS_FAMILY,
  STANDARDS_RULES,
} from "./standards.ts";
import {
  ARGUMENT_HINT_RULE,
  DESCRIPTION_LENGTH_RULE,
  NAME_FORMAT_RULE,
  REFERENCES_INDEX_RULE,
  ROUTING_SECTION_RULE,
  SUMMARY_BLOCK_RULE,
} from "./structure.ts";
import { ANCHOR_RULE } from "./xref-anchor.ts";
import { BARE_COMMAND_RULE, CODEX_FORM_RULE } from "./xref-form.ts";
import { FLAG_RULE, MODE_RULE } from "./xref-mode.ts";
import { STEP_RULE } from "./xref-step.ts";

const options: RunOptions = {
  short: false,
  fast: false,
  diagnostics: () => {},
  env: process.env,
};

const context: ResolvedSkillsContext = {
  families: DOMAIN_FAMILIES,
  evalsCheck: cleanEvalsCheck,
};

/** The core family names in `FAMILIES` order; question-format is not core, so it is not here. */
const CORE_NAMES = [
  "inline",
  "frontmatter",
  "eval-metadata",
  "eval-frontmatter-clean",
  "orchestration",
  "reference-link",
  "reference-naming",
  "triage",
  "artifact",
  "retired-skill",
  "ai-attribution",
  "independence",
  "workfolder",
  "spec-pass",
  "legacy-names",
  "state-tsv",
  "flow-emission",
  "lean-cadence",
  "eval-check",
];

/**
 * The rule ids the standards families report: the skill-standards family and the domain families that run beside it
 * outside the core families.
 */
const STANDARDS_FAMILIES_RULES: ReadonlySet<string> = new Set([
  "skill-xref-anchor",
  "skill-xref-mode",
  "skill-xref-flag",
  "skill-command-bare",
  "skill-command-codex-form",
  "skill-xref-step",
  "skill-frontmatter-key",
  "skill-frontmatter-value",
  "skill-metadata",
  "skill-duplicate-block",
  "skill-summary-block",
  "skill-references-index",
  "skill-routing-section",
  "skill-name-format",
  "skill-description-length",
  "skill-argument-hint",
  "skill-baseline-stale",
  "shared-contracts",
  "skill-shell-data",
  "skill-shell-data-stale",
  "skill-file-size",
  "skill-file-size-stale",
  "skill-inbound-anchor",
  "flow-waive-operands",
  "gate-record-keeping",
]);

/** The standards family's findings alone, through `run`, so the index's error handling is what a caller sees. */
async function standards(root: string) {
  return run(root, options, { families: [STANDARDS_FAMILY] });
}

/** The routing skill's frontmatter line that opens its metadata block, as the fixture writes it. */
const ROUTING_METADATA = "metadata:\n    surfaces: [claude-code]\n";

/** Rewrites the routing skill with a changed frontmatter and body; the fixture's own text is the start point. */
function rewriteRouting(
  root: string,
  edit: { frontmatter?: (text: string) => string; body?: string },
): void {
  let text = skillMD("routing");
  if (edit.frontmatter !== undefined) {
    assert.ok(text.includes(ROUTING_METADATA), "the fixture shape changed");
    text = edit.frontmatter(text);
  }
  if (edit.body !== undefined) text += edit.body;
  writeFile(root, "skills/routing/SKILL.md", text);
}

/**
 * Removes the routing skill's baseline entry for one rule. The fixture's routing skill already breaks the body rules
 * that need a heading, and those rules give one finding for a file, so dropping the accepted entry is the defect.
 */
function unbaselineRouting(root: string, rule: string): void {
  const rel = `${BASELINE_DIR}/routing.json`;
  const entries = JSON.parse(
    readFileSync(path.join(root, ...rel.split("/")), "utf8"),
  ) as Array<{ rule: string }>;
  const kept = entries.filter((entry) => entry.rule !== rule);
  assert.equal(kept.length, entries.length - 1, `no ${rule} entry to remove`);
  writeFile(root, rel, JSON.stringify(kept, null, 4) + "\n");
}

/** The rules that check a skill's file structure and frontmatter shape. */
const STRUCTURE_RULES: ReadonlySet<string> = new Set([
  SUMMARY_BLOCK_RULE,
  REFERENCES_INDEX_RULE,
  ROUTING_SECTION_RULE,
  NAME_FORMAT_RULE,
  DESCRIPTION_LENGTH_RULE,
  ARGUMENT_HINT_RULE,
]);

// --- the family's shape --------------------------------------------------------------------------------------------

test("the default audit's non-core rules are the question-format rule and the standards families' rules", () => {
  assert.deepEqual(
    new Set(
      DOMAIN_FAMILIES.filter((family) => !FAMILIES.includes(family)).flatMap(
        (family) => [...family.rules],
      ),
    ),
    new Set([...STANDARDS_FAMILIES_RULES, "question-format"]),
  );
  assert.deepEqual(
    STANDARDS_FAMILIES_RULES,
    new Set([
      ...STANDARDS_RULES,
      "skill-shell-data",
      "skill-shell-data-stale",
      "skill-file-size",
      "skill-file-size-stale",
      "skill-inbound-anchor",
      "shared-contracts",
      "flow-waive-operands",
      "gate-record-keeping",
    ]),
  );
  assert.equal(STANDARDS_FAMILY.name, "skill-standards");
  assert.deepEqual(STANDARDS_FAMILY.rules, STANDARDS_RULES);
});

test("a baseline may name every standards rule except the stale rule", () => {
  assert.equal(BASELINED_RULES.has(BASELINE_STALE_RULE), false);
  assert.deepEqual(
    new Set([...BASELINED_RULES, BASELINE_STALE_RULE]),
    new Set(STANDARDS_RULES),
  );
});

test("DOMAIN_FAMILIES adds question-format after independence and the other non-core families before eval-check", () => {
  assert.deepEqual(
    FAMILIES.map((family) => family.name),
    CORE_NAMES,
  );
  const expected = [...CORE_NAMES];
  expected.splice(expected.indexOf("workfolder"), 0, "question-format");
  expected.splice(
    expected.indexOf("eval-check"),
    0,
    "skill-standards",
    "skill-shell-data",
    "skill-file-size",
    "skill-inbound-anchor",
    "shared-contracts",
    "flow-waive-operands",
    "gate-record-keeping",
  );
  assert.deepEqual(
    DOMAIN_FAMILIES.map((family) => family.name),
    expected,
  );
  assert.equal(DOMAIN_FAMILIES.length, FAMILIES.length + 8);
  assert.equal(DOMAIN_FAMILIES.at(-1), EVAL_CHECK_FAMILY);
  assert.equal(DOMAIN_FAMILIES.at(-2), RECORD_KEEPING_FAMILY);
  assert.equal(DOMAIN_FAMILIES.at(-3), WAIVE_OPERANDS_FAMILY);
  assert.equal(DOMAIN_FAMILIES.at(-4), SHARED_CONTRACTS_FAMILY);
  assert.equal(DOMAIN_FAMILIES.at(-5), INBOUND_ANCHOR_FAMILY);
  assert.equal(DOMAIN_FAMILIES.at(-6), FILE_SIZE_FAMILY);
  assert.equal(DOMAIN_FAMILIES.at(-7), SHELL_DATA_FAMILY);
  assert.equal(DOMAIN_FAMILIES.at(-8), STANDARDS_FAMILY);
  for (const family of FAMILIES) assert.ok(DOMAIN_FAMILIES.includes(family));
});

test("RULE_FAMILY is the map of the core families alone, and DOMAIN_FAMILIES has no rule with two owners", () => {
  assert.deepEqual(RULE_FAMILY, ruleFamilyOf(FAMILIES));
  for (const rule of STANDARDS_FAMILIES_RULES)
    assert.equal(Object.hasOwn(RULE_FAMILY, rule), false, rule);
  const domainMap = ruleFamilyOf(DOMAIN_FAMILIES);
  for (const rule of STANDARDS_RULES)
    assert.equal(domainMap[rule], "skill-standards");
  assert.equal(domainMap["shared-contracts"], "shared-contracts");
  assert.equal(domainMap["gate-record-keeping"], "gate-record-keeping");
  assert.equal(domainMap["skill-shell-data"], "skill-shell-data");
  assert.equal(domainMap["skill-shell-data-stale"], "skill-shell-data");
  assert.equal(domainMap["skill-file-size"], "skill-file-size");
  assert.equal(domainMap["skill-file-size-stale"], "skill-file-size");
  assert.equal(domainMap["skill-inbound-anchor"], "skill-inbound-anchor");
  assert.equal(RULE_FAMILY["question-format"], undefined);
  assert.equal(domainMap["question-format"], "question-format");
});

// --- the family on the fixture -------------------------------------------------------------------------------------

test("the green fixture has standards findings until its baseline is written, then none", async () => {
  await withSkillsRepo(async (root) => {
    const before = await standards(root);
    assert.equal(before.error, undefined);
    assert.ok(
      before.failures.length > 0,
      "the fixture has no finding to baseline",
    );
    writeStandardsBaseline(root);
    assert.deepEqual(await standards(root), { failures: [] });
  });
});

/**
 * Each defect, with the rule it must add to a baselined fixture; every message names the rule once. A `single` defect
 * must give exactly one finding, so the rule is shown to report it once and nothing is left once it is taken away.
 */
const DEFECTS: ReadonlyArray<{
  rule: string;
  single?: boolean;
  inject: (root: string) => void;
}> = [
  {
    rule: ANCHOR_RULE,
    inject: (root) =>
      rewriteRouting(root, {
        body: "\nSee [the gate](../gate/SKILL.md#no-such-heading).\n",
      }),
  },
  {
    rule: MODE_RULE,
    inject: (root) =>
      rewriteRouting(root, {
        frontmatter: (text) =>
          text.replace(
            ROUTING_METADATA,
            `argument-hint: "[plan]"\n${ROUTING_METADATA}    modes: [plan]\n`,
          ),
        body: "\nRun $agentics:routing nosuch to begin.\n",
      }),
  },
  {
    rule: FLAG_RULE,
    inject: (root) =>
      rewriteRouting(root, {
        body: "\nRun `$agentics:gate --nosuch` to begin.\n",
      }),
  },
  {
    rule: BARE_COMMAND_RULE,
    inject: (root) => rewriteRouting(root, { body: "\nRun /gate to begin.\n" }),
  },
  {
    rule: CODEX_FORM_RULE,
    inject: (root) =>
      rewriteRouting(root, { body: "\nRun `/agentics:gate` to begin.\n" }),
  },
  {
    rule: STEP_RULE,
    inject: (root) =>
      rewriteRouting(root, { body: "\nThen follow `gate` Step 99.\n" }),
  },
  {
    rule: KEY_RULE,
    inject: (root) =>
      rewriteRouting(root, {
        frontmatter: (text) =>
          text.replace(ROUTING_METADATA, `bogus-key: yes\n${ROUTING_METADATA}`),
      }),
  },
  {
    rule: VALUE_RULE,
    inject: (root) =>
      rewriteRouting(root, {
        frontmatter: (text) =>
          text.replace(
            ROUTING_METADATA,
            `context: inline\n${ROUTING_METADATA}`,
          ),
      }),
  },
  {
    rule: METADATA_RULE,
    inject: (root) =>
      rewriteRouting(root, {
        frontmatter: (text) =>
          text.replace(
            ROUTING_METADATA,
            "metadata:\n    surfaces: [claude-code, cursor]\n",
          ),
      }),
  },
  {
    rule: DUPLICATE_RULE,
    inject: (root) => {
      const paragraph = "one two three four five six seven eight nine\n";
      rewriteRouting(root, { body: `\n${paragraph}\n${paragraph}` });
    },
  },
  {
    rule: SUMMARY_BLOCK_RULE,
    single: true,
    inject: (root) =>
      rewriteRouting(root, { body: "\n## Capability summary\n\nText.\n" }),
  },
  {
    rule: REFERENCES_INDEX_RULE,
    single: true,
    inject: (root) => unbaselineRouting(root, REFERENCES_INDEX_RULE),
  },
  {
    rule: ROUTING_SECTION_RULE,
    single: true,
    inject: (root) => unbaselineRouting(root, ROUTING_SECTION_RULE),
  },
  {
    rule: NAME_FORMAT_RULE,
    single: true,
    inject: (root) =>
      rewriteRouting(root, {
        frontmatter: (text) =>
          text.replace("name: routing\n", "name: Routing\n"),
      }),
  },
  {
    rule: DESCRIPTION_LENGTH_RULE,
    single: true,
    inject: (root) =>
      rewriteRouting(root, {
        frontmatter: (text) =>
          text.replace(
            "description: Test skill for routing.\n",
            `description: ${"d".repeat(1025)}\n`,
          ),
      }),
  },
  {
    rule: ARGUMENT_HINT_RULE,
    single: true,
    inject: (root) =>
      rewriteRouting(root, {
        frontmatter: (text) =>
          text.replace(
            ROUTING_METADATA,
            `argument-hint: "[audit]"\n${ROUTING_METADATA}    modes: [plan]\n`,
          ),
      }),
  },
  {
    rule: BASELINE_STALE_RULE,
    inject: (root) =>
      writeFile(
        root,
        `${BASELINE_DIR}/zzz.json`,
        JSON.stringify([
          { rule: ANCHOR_RULE, path: "skills/zzz/SKILL.md", key: "gone" },
        ]),
      ),
  },
];

test("the defects cover every standards rule once", () => {
  assert.deepEqual(
    DEFECTS.map((defect) => defect.rule),
    [...STANDARDS_RULES],
  );
});

for (const { rule, single, inject } of DEFECTS) {
  test(`one injected defect on the baselined fixture reports ${rule} and nothing else`, async () => {
    await withSkillsRepo(async (root) => {
      writeStandardsBaseline(root);
      inject(root);
      const result = await standards(root);
      assert.equal(result.error, undefined);
      assert.ok(result.failures.length > 0, `${rule}: no finding`);
      assert.deepEqual(
        [...new Set(result.failures.map((failure) => failure.rule))],
        [rule],
      );
      if (single === true) assert.equal(result.failures.length, 1, rule);
    });
  });
}

test("a skill with the passing structure and valid frontmatter gives no structure finding and the whole skills domain passes once the baseline is written", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(
      root,
      "skills/shaped/SKILL.md",
      [
        "---",
        "name: shaped",
        "description: A skill whose file has the expected structure.",
        'argument-hint: "[plan] [--fast]"',
        'version: "1.0.0"',
        "metadata:",
        "    surfaces: [claude-code]",
        "    modes: [plan]",
        "    flags: [--fast]",
        "---",
        "",
        "# shaped",
        "",
        "Intro.",
        "",
        "## Don't use when",
        "",
        "- Never.",
        "",
        "## Steps",
        "",
        "Do it.",
        "",
        "## References",
        "",
        "- None.",
        "",
      ].join("\n"),
    );
    const structural = collectStandardFindings(root).filter((finding) =>
      STRUCTURE_RULES.has(finding.rule),
    );
    // The fixture's own skills have no References heading, so an empty list here would mean the rules never ran.
    assert.ok(
      structural.some((finding) => finding.path === "skills/routing/SKILL.md"),
      "the collector reports no structure finding for the fixture",
    );
    assert.deepEqual(
      structural.filter((finding) => finding.path === "skills/shaped/SKILL.md"),
      [],
    );
    writeStandardsBaseline(root);
    assert.deepEqual(
      await run(root, options, { evalsCheck: cleanEvalsCheck }),
      {
        failures: [],
      },
    );
  });
});

test("the stale finding names the baseline file and quotes the entry", async () => {
  await withSkillsRepo(async (root) => {
    writeStandardsBaseline(root);
    const entry = {
      rule: ANCHOR_RULE,
      path: "skills/zzz/SKILL.md",
      key: "gone",
    };
    writeFile(root, `${BASELINE_DIR}/zzz.json`, JSON.stringify([entry]));
    assert.deepEqual(await standards(root), {
      failures: [
        {
          rule: BASELINE_STALE_RULE,
          path: `${BASELINE_DIR}/zzz.json`,
          message: `baseline entry matches no finding: ${JSON.stringify(entry)}`,
        },
      ],
    });
  });
});

test("the non-core findings reach the default run in family order before eval-check", async () => {
  await withSkillsRepo(async (root) => {
    writeStandardsBaseline(root);
    rewriteRouting(root, { body: "\nThen follow `gate` Step 99.\n" });
    const runner = {
      rule: "eval-stub",
      path: "evals/skills.yaml",
      message: "m",
    };
    const result = await run(root, options, { evalsCheck: () => [runner] });
    assert.deepEqual(
      result.failures.map((failure) => failure.rule),
      [STEP_RULE, "shared-contracts", "shared-contracts", "eval-stub"],
    );
  });
});

// --- audit errors --------------------------------------------------------------------------------------------------

test("a baseline entry that names the stale rule is an audit error: the family throws and the run records it", async () => {
  await withSkillsRepo(async (root) => {
    writeStandardsBaseline(root);
    writeFile(
      root,
      `${BASELINE_DIR}/routing.json`,
      JSON.stringify([
        {
          rule: BASELINE_STALE_RULE,
          path: "skills/routing/SKILL.md",
          key: "x",
        },
      ]),
    );
    const message = `skills: ${BASELINE_DIR}/routing.json: entry 0 names unknown rule ${BASELINE_STALE_RULE}`;
    assert.throws(() => STANDARDS_FAMILY.run(root, context), { message });
    assert.deepEqual(await standards(root), { failures: [], error: message });
  });
});
