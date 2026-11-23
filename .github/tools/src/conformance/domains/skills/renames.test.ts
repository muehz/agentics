import assert from "node:assert/strict";
import path from "node:path";
import { test } from "node:test";
import type { Failure, RunOptions } from "../../check.ts";
import {
  auditEveryFamily,
  auditSlice,
  sliceNamed,
  ARROW_RIGHT,
  breaker,
  NUL,
  put,
} from "../../testing/skills-seeds.ts";
import {
  addSkill,
  link,
  mkdir,
  remove,
  withSkillsRepo,
  writeFile,
} from "../../testing/skills-repo.ts";
import { RepoFileError } from "../../repo-file-error.ts";
import { seams, withSeams } from "./files.ts";
import { run } from "./index.ts";
import {
  FAMILY,
  owns,
  referencedIn,
  RETIRED_SKILLS,
  ROOTS,
  retiredSkillRules,
} from "./renames.ts";
import { canSymlink, SYMLINK_SKIP, withOutside } from "../../testing/host.ts";

const options: RunOptions = {
  short: false,
  fast: false,
  diagnostics: () => {},
  env: process.env,
};

const RULE = "retired-skill-reference";
const PLANNING_MESSAGE =
  "use intent and decompose; retired skill names remain in an active path or reference";
const WORK_MESSAGE =
  "routing is the automatic chain entrance and phase skills are the explicit ones; the retired work skill remains in an active path or reference";

const ARROW = String.fromCodePoint(0x2192);

/** The later skill retirements, in entry order from the third entry. */
const LATER_SKILLS: ReadonlyArray<{ names: string[]; message: string }> = [
  {
    names: ["understand", "teach"],
    message:
      "use explain and its how, why and teach modes; retired skill names remain in an active path or reference",
  },
  {
    names: ["incident", "runbook"],
    message:
      "use ops and its modes; retired skill names remain in an active path or reference",
  },
  {
    names: ["blast-radius"],
    message:
      "use review impact; the retired blast-radius skill remains in an active path or reference",
  },
  {
    names: ["uiux"],
    message:
      "adopt an outside UI/UX skill; the retired uiux skill remains in an active path or reference",
  },
  {
    names: ["rig"],
    message:
      "use scaffold feedback-loops; the retired rig skill remains in an active path or reference",
  },
  {
    names: ["bro"],
    message:
      "bro has no replacement; the retired bro skill remains in an active path or reference",
  },
  {
    names: ["peer"],
    message:
      "the maintainer dev tool is .github/tools/src/peer; the retired peer skill remains in an active path or reference",
  },
];
const HOOK_MESSAGE =
  "the hook has no replacement; the retired test-advisor hook remains in an active path or reference";

const laterMessage = (index: number): string => {
  const found = LATER_SKILLS[index];
  assert.ok(found, `later entry ${index}`);
  return found.message;
};

const audit = (root: string) => run(root, options, { families: [FAMILY] });

const brief = (found: Failure[]): string[][] =>
  found.map((one) => [one.rule, one.path]);

/** The failures of a run that must not have raised an audit error. */
async function findings(root: string): Promise<Failure[]> {
  const result = await audit(root);
  assert.equal(result.error, undefined);
  return result.failures;
}

const planning = (path: string): Failure => ({
  rule: RULE,
  path,
  message: PLANNING_MESSAGE,
});
const work = (path: string): Failure => ({
  rule: RULE,
  path,
  message: WORK_MESSAGE,
});

const NO_LINKS = canSymlink() ? false : SYMLINK_SKIP;

// --- the table ---------------------------------------------------------------------------------

test("the family is named retired-skill and reports its one rule", () => {
  assert.equal(FAMILY.name, "retired-skill");
  assert.deepEqual(FAMILY.rules, [RULE]);
});

test("the roots and the ten entries equal the expected values", () => {
  assert.deepEqual(ROOTS, [
    "skills",
    "docs/guide",
    "docs/surfaces",
    "evals/skills",
    ".github/scripts",
    ".claude-plugin",
    ".codex-plugin",
    ".github/plugin",
  ]);
  assert.deepEqual(
    RETIRED_SKILLS.map((one) => [
      one.paths,
      one.references.length,
      one.message,
    ]),
    [
      [
        [
          "skills/ideation/",
          "skills/planning/",
          "evals/skills/ideation.md",
          "evals/skills/planning.md",
        ],
        2,
        PLANNING_MESSAGE,
      ],
      [["skills/work/", "evals/skills/work.md"], 1, WORK_MESSAGE],
      [
        [
          "skills/understand/",
          "skills/teach/",
          "evals/skills/understand.md",
          "evals/skills/teach.md",
        ],
        1,
        laterMessage(0),
      ],
      [
        [
          "skills/incident/",
          "skills/runbook/",
          "evals/skills/incident.md",
          "evals/skills/runbook.md",
        ],
        1,
        laterMessage(1),
      ],
      [
        ["skills/blast-radius/", "evals/skills/blast-radius.md"],
        1,
        laterMessage(2),
      ],
      [["skills/uiux/", "evals/skills/uiux.md"], 1, laterMessage(3)],
      [["skills/rig/", "evals/skills/rig.md"], 1, laterMessage(4)],
      [["skills/bro/", "evals/skills/bro.md"], 1, laterMessage(5)],
      [["skills/peer/", "evals/skills/peer.md"], 1, laterMessage(6)],
      [[], 1, HOOK_MESSAGE],
    ],
  );
});

test("owns: an exact path or anything below a path that ends in a slash", () => {
  const [planningSkill] = RETIRED_SKILLS;
  assert.ok(planningSkill);
  assert.equal(owns(planningSkill, "skills/ideation/SKILL.md"), true);
  assert.equal(owns(planningSkill, "skills/ideation/deep/x.md"), true);
  assert.equal(owns(planningSkill, "skills/ideation"), false);
  assert.equal(owns(planningSkill, "skills/ideation2/SKILL.md"), false);
  assert.equal(owns(planningSkill, "evals/skills/ideation.md"), true);
  assert.equal(owns(planningSkill, "evals/skills/ideation.md/x"), false);
  assert.equal(owns(planningSkill, "evals/skills/ideation.mdx"), false);
  assert.equal(owns(planningSkill, "skills/work/SKILL.md"), false);
});

test("referencedIn: any pattern of the entry, and nothing for empty text", () => {
  const [planningSkill, workSkill] = RETIRED_SKILLS;
  assert.ok(planningSkill && workSkill);
  assert.equal(referencedIn(planningSkill, "`/ideation`"), true);
  assert.equal(referencedIn(planningSkill, "the ideation workflow"), true);
  assert.equal(referencedIn(planningSkill, ""), false);
  assert.equal(referencedIn(workSkill, "`/ideation`"), false);
});

// --- the retired-name patterns, on what they match and where the match starts and ends ----------------
// A token in an input or an expected match stands for a character that is not typed into this source:
// {ARROW} U+2192, {VT} U+000B, {NBSP} U+00A0.

const TOKENS: ReadonlyArray<[string, string]> = [
  ["{ARROW}", ARROW],
  ["{VT}", "\v"],
  ["{NBSP}", String.fromCodePoint(0xa0)],
];

function expand(text: string): string {
  let out = text;
  for (const [token, char] of TOKENS) out = out.replaceAll(token, char);
  return out;
}

type Vector = readonly [input: string, match: string | null];

function checkVectors(pattern: RegExp, vectors: readonly Vector[]): void {
  for (const [input, expected] of vectors) {
    const found = pattern.exec(expand(input));
    assert.equal(
      found === null ? null : found[0],
      expected === null ? null : expand(expected),
      `input ${JSON.stringify(input)}`,
    );
  }
}

function pattern(entry: number, index: number): RegExp {
  const found = RETIRED_SKILLS[entry]?.references[index];
  assert.ok(found, `pattern ${entry}.${index}`);
  return found;
}

test("the planning path-or-name pattern matches a skills path, a relative path or a quoted name", () => {
  checkVectors(pattern(0, 0), [
    ["Use `/ideation` now.", "`/ideation`"],
    ["Route to `planning`.", "`planning`"],
    ["[schema](../planning/references/chain-artifacts.md)", "../planning/"],
    ['"./skills/ideation"', 'skills/ideation"'],
    ["`ideation`", "`ideation`"],
    ["`agentics:planning` and", "`agentics:planning`"],
    ["`/planning:", "`/planning:"],
    ["`planning ", "`planning "],
    ["`planning`", "`planning`"],
    ["`planningx`", null],
    ["`Planning`", null],
    ["skills/planning/", "skills/planning/"],
    ["skills/planning'", "skills/planning'"],
    ['skills/planning"', 'skills/planning"'],
    ["skills/planning`", "skills/planning`"],
    ["skills/planning.md", null],
    ["skills/planner/", null],
    ["../../ideation/x", "../../ideation/"],
    ["..ideation/", null],
    ["./ideation/", null],
    ["xskills/ideation/", "skills/ideation/"],
    ["Sprint planning skill", null],
    ["planning workflow", null],
    ["the ideation workflow", null],
    ["/2026-10/work/plan.md", null],
    ["`work_folder`", null],
  ]);
});

test("every ../ run in a reference pattern can start only at the run's first segment", () => {
  const run = /\(\?:\\\.\\\.\\?\/\)\+/gu;
  let runs = 0;
  for (const skill of RETIRED_SKILLS)
    for (const expression of skill.references)
      for (const found of expression.source.matchAll(run)) {
        runs += 1;
        assert.match(
          expression.source.slice(0, found.index),
          /\(\?<!\\\.\\\.\\?\/\)$/u,
          `unanchored ../ run in ${expression.source}`,
        );
      }
  assert.ok(runs > 0, "no ../ run found; the run pattern is stale");
  checkVectors(pattern(0, 0), [
    ["x../../../ideation/", "../../../ideation/"],
    ["...../ideation/", "../ideation/"],
    [`${"../".repeat(4)}other/`, null],
  ]);
});

test("the planning prose pattern matches an arrow, a role word or a table cell next to a retired name", () => {
  checkVectors(pattern(0, 1), [
    ["ideation {ARROW} planning", "ideation {ARROW}"],
    ["ideation{ARROW}planning", "ideation{ARROW}"],
    ["ideation {ARROW}", "ideation {ARROW}"],
    ["ideation\n{ARROW}", "ideation\n{ARROW}"],
    ["ideation{VT}{ARROW}", null],
    ["{ARROW} planning", "{ARROW} planning"],
    ["{ARROW}planning", "{ARROW}planning"],
    ["{ARROW}\tideation", "{ARROW}\tideation"],
    ["{ARROW} ideation2", null],
    ["{ARROW} ideationx", null],
    ["planning owns the fork", "planning owns"],
    ["planning owns", "planning owns"],
    ["planning ownsx", null],
    ["planning workflow", null],
    ["planning skill", "planning skill"],
    ["planning skills", null],
    ["Sprint planning skill", "planning skill"],
    ["ideation owns", "ideation owns"],
    ["ideation workflow", "ideation workflow"],
    ["ideation skill", "ideation skill"],
    ["ideation skills", null],
    ["ideation workflows", null],
    ["the ideation workflow", "ideation workflow"],
    ["ideation  skill", "ideation  skill"],
    ["ideation{NBSP}skill", null],
    ["ideation{VT}skill", null],
    ["xideation skill", "ideation skill"],
    ["xplanning skill", "planning skill"],
    ["<diagnose | planning>", "| planning>"],
    ["|ideation|", "|ideation|"],
    ["< planning >", "< planning >"],
    ["<planning", null],
    ["planning>", null],
    ["| planning", null],
    ["<\nideation\n>", "<\nideation\n>"],
    ["/2026-10/work/plan.md", null],
    ["`work_folder`", null],
    ["ideation{ARROW}", "ideation{ARROW}"],
    ["planning{ARROW}", "planning{ARROW}"],
  ]);
});

test("the work pattern matches a skills path, a relative path or a quoted name", () => {
  checkVectors(pattern(1, 0), [
    ["Type `/work` to enter the chain.", "`/work`"],
    ["the `work` entrance", "`work`"],
    ["[work](../work/SKILL.md)", "../work/"],
    ['"./skills/work"', 'skills/work"'],
    ["invoke `agentics:work` here", "`agentics:work`"],
    ["`work_folder`", null],
    ["`/2026-10/work/plan.md`", null],
    ["<plan-parent>/2026-10/work/plan.md", null],
    ["Start or continue work.", null],
    ["`work`", "`work`"],
    ["`worker` ", null],
    ["skills/work/", "skills/work/"],
    ["skills/workflow/", null],
    ["`Work` ", null],
    ["`work:", "`work:"],
    ["`/work`", "`/work`"],
  ]);
});

test("each later skill pattern matches every form and terminator of each of its names, and no bare name", () => {
  LATER_SKILLS.forEach(({ names }, index) => {
    const expression = pattern(index + 2, 0);
    for (const name of names) {
      const vectors: Vector[] = [];
      for (const end of ["/", "'", '"', "`"]) {
        vectors.push([`see skills/${name}${end}x`, `skills/${name}${end}`]);
        vectors.push([`see ../${name}${end}x`, `../${name}${end}`]);
        vectors.push([`see ../../${name}${end}x`, `../../${name}${end}`]);
      }
      for (const lead of ["/", "agentics:", "/agentics:"])
        for (const end of ["`", " ", ":"])
          vectors.push([
            `run \`${lead}${name}${end}x`,
            `\`${lead}${name}${end}`,
          ]);
      vectors.push(
        [`the \`${name}\` mode`, null],
        [`skills/${name}.md`, null],
        [`skills/${name}x/`, null],
        [`./${name}/`, null],
        [`\`/${name}x\``, null],
        [`\`agentics:${name}x\``, null],
        [`\`/agentics:${name}x\``, null],
        [`\`/${name}`, null],
        [`${name} skill`, null],
      );
      checkVectors(expression, vectors);
    }
  });
});

test("the test-advisor pattern needs no word character or hyphen on either side", () => {
  checkVectors(pattern(9, 0), [
    ["the test-advisor hook", "test-advisor"],
    ["test-advisor", "test-advisor"],
    ["hooks/test-advisor/", "test-advisor"],
    ["`test-advisor`", "test-advisor"],
    ["test-advisory", null],
    ["test-advisor2", null],
    ["test-advisor_x", null],
    ["test-advisor-x", null],
    ["xtest-advisor", null],
    ["9test-advisor", null],
    ["_test-advisor", null],
    ["-test-advisor", null],
    ["Test-Advisor", null],
    ["test advisor", null],
  ]);
});

// --- the family ----------------------------------------------------------------------------------

test("the green fixture passes", async () => {
  await withSkillsRepo(async (root) => {
    assert.deepEqual(await audit(root), { failures: [] });
  });
});

test("the findings come in path order and, within a file, in entry order", async () => {
  await withSkillsRepo(async (root) => {
    const want: Failure[] = [];
    for (const dir of [
      ".github/scripts",
      "docs/guide",
      "evals/skills",
      "skills",
    ]) {
      for (let i = 0; i < 6; i += 1) {
        const rel = `${dir}/example${String(i).padStart(2, "0")}.md`;
        writeFile(root, rel, "Use `/planning` and `/work`.\n");
        want.push(planning(rel), work(rel));
      }
    }
    const found = await findings(root);
    assert.equal(found.length, 48);
    assert.deepEqual(found, want);
  });
});

test("findings from a later root sort ahead of an earlier root by path, with the entry order kept inside one path", async () => {
  await withSkillsRepo(async (root) => {
    for (const rel of [
      "skills/zz.md",
      "docs/surfaces/zz.md",
      ".github/plugin/zz.json",
    ])
      writeFile(root, rel, "`/work` and `/planning`\n");
    assert.deepEqual(await findings(root), [
      planning(".github/plugin/zz.json"),
      work(".github/plugin/zz.json"),
      planning("docs/surfaces/zz.md"),
      work("docs/surfaces/zz.md"),
      planning("skills/zz.md"),
      work("skills/zz.md"),
    ]);
  });
});

for (const stale of [
  "Use `/ideation` now.",
  "Route to `planning`.",
  "[schema](../planning/references/chain-artifacts.md)",
  '"./skills/ideation"',
  `ideation ${ARROW} planning ${ARROW} execute`,
  "planning owns the fork",
  "<diagnose | planning>",
  "the ideation workflow",
]) {
  test(`a retired planning reference is one finding: ${JSON.stringify(stale)}`, async () => {
    await withSkillsRepo(async (root) => {
      writeFile(root, "skills/example/SKILL.md", stale);
      assert.deepEqual(await findings(root), [
        planning("skills/example/SKILL.md"),
      ]);
    });
  });
}

const FORMS: ReadonlyArray<[label: string, text: (name: string) => string]> = [
  ["skills-path", (name) => `See skills/${name}/SKILL.md.`],
  ["relative", (name) => `[old](../${name}/SKILL.md)`],
  ["slash", (name) => `Type \`/${name}\` now.`],
  ["agentics", (name) => `Invoke \`agentics:${name}\` here.`],
  ["slash-agentics", (name) => `Type \`/agentics:${name}\` now.`],
];

LATER_SKILLS.forEach(({ names, message }) => {
  for (const name of names) {
    test(`each form of a reference to ${name} is one finding with its entry's message, and history is allowed`, async () => {
      await withSkillsRepo(async (root) => {
        const want: Failure[] = [];
        for (const [label, text] of FORMS) {
          const rel = `skills/x/${label}.md`;
          writeFile(root, rel, text(name));
          want.push({ rule: RULE, path: rel, message });
          writeFile(
            root,
            `docs/specs/2026-11/feature-x/${label}.md`,
            text(name),
          );
        }
        writeFile(
          root,
          "docs/RENAMES.md",
          FORMS.map(([, text]) => text(name)).join("\n"),
        );
        want.sort((a, b) => (a.path < b.path ? -1 : 1));
        assert.deepEqual(await findings(root), want);
      });
    });

    test(`skills/${name}/SKILL.md and evals/skills/${name}.md are rejected by their paths alone`, async () => {
      await withSkillsRepo(async (root) => {
        writeFile(root, `skills/${name}/SKILL.md`, "obsolete");
        writeFile(root, `evals/skills/${name}.md`, "obsolete");
        assert.deepEqual(await findings(root), [
          { rule: RULE, path: `evals/skills/${name}.md`, message },
          { rule: RULE, path: `skills/${name}/SKILL.md`, message },
        ]);
      });
    });
  }
});

test("a test-advisor reference is one finding with the hook message, and history is allowed", async () => {
  await withSkillsRepo(async (root) => {
    const text = "The test-advisor hook nudges for tests.";
    writeFile(root, "skills/x/SKILL.md", text);
    writeFile(root, "docs/RENAMES.md", text);
    writeFile(root, "docs/specs/2026-11/feature-x/plan.md", text);
    assert.deepEqual(await findings(root), [
      { rule: RULE, path: "skills/x/SKILL.md", message: HOOK_MESSAGE },
    ]);
  });
});

test("current text that shares a later retired name is allowed", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(
      root,
      "skills/x/SKILL.md",
      [
        "Use the `teach` mode of explain, or type `/agentics:explain teach`.",
        "Apply the `incident` label.",
        "The dev tool lives in .github/tools/src/peer and runs a peer review.",
        "When you run the incident drill, first understand the code.",
        "Read [the guide](../../docs/guide/understand.md).",
        "The test-advisory note is history.",
      ].join("\n"),
    );
    assert.deepEqual(await findings(root), []);
  });
});

test("one file naming both skills of one entry gets one finding", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(root, "skills/x/SKILL.md", "`/incident` and `/runbook`");
    assert.deepEqual(await findings(root), [
      { rule: RULE, path: "skills/x/SKILL.md", message: laterMessage(1) },
    ]);
  });
});

test("evidence and ordinary planning prose are allowed", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(
      root,
      "skills/example/SKILL.md",
      "Sprint planning workflows and native plan mode; use `/decompose` and `/intent`.",
    );
    writeFile(
      root,
      "docs/RENAMES.md",
      "`ideation` became `intent`; `planning` became `decompose`.",
    );
    writeFile(
      root,
      "docs/specs/2026-09-05-feature-record/plan.md",
      "Previously used `/planning`.",
    );
    writeFile(
      root,
      "evals/baselines/old/skills/planning/SKILL.md",
      "name: planning\n",
    );
    assert.deepEqual(await findings(root), []);
  });
});

for (const owned of [
  "skills/ideation/SKILL.md",
  "skills/planning/SKILL.md",
  "evals/skills/ideation.md",
  "evals/skills/planning.md",
]) {
  test(`${owned} is rejected by its path alone`, async () => {
    await withSkillsRepo(async (root) => {
      writeFile(root, owned, "obsolete");
      assert.deepEqual(await findings(root), [planning(owned)]);
    });
  });
}

for (const owned of ["skills/work/SKILL.md", "evals/skills/work.md"]) {
  test(`${owned} is rejected by its path alone`, async () => {
    await withSkillsRepo(async (root) => {
      writeFile(root, owned, "obsolete");
      assert.deepEqual(await findings(root), [work(owned)]);
    });
  });
}

test("a bank's preamble is history, but its scenarios are active", async () => {
  await withSkillsRepo(async (root) => {
    const rel = "evals/skills/example.md";
    writeFile(
      root,
      rel,
      "The planning skill scored 4/5 historically.\n## happy\nUse decompose.\n",
    );
    assert.deepEqual(await findings(root), []);
    writeFile(
      root,
      rel,
      `History.\n## happy\nideation ${ARROW} planning ${ARROW} execute\n`,
    );
    assert.deepEqual(await findings(root), [planning(rel)]);
  });
});

test("a retired reference only before the happy heading of an evals bank is history", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(root, "evals/skills/git.md", "Old: `/planning`.\n## happy\nok\n");
    assert.deepEqual(await findings(root), []);
    writeFile(
      root,
      "evals/skills/git.md",
      "Old.\n## happy\nNew: `/planning`.\n",
    );
    assert.deepEqual(await findings(root), [planning("evals/skills/git.md")]);
  });
});

test("a bank without a happy heading is scanned whole", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(root, "evals/skills/git.md", "Old: `/planning`.\n## sad\nok\n");
    assert.deepEqual(await findings(root), [planning("evals/skills/git.md")]);
  });
});

test("the heading must follow a newline: a heading on the first line does not cut the file", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(root, "evals/skills/git.md", "## happy `/planning`\n");
    assert.deepEqual(await findings(root), [planning("evals/skills/git.md")]);
  });
});

test("outside evals nothing is cut at the happy heading", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(root, "docs/guide/x.md", "Old: `/planning`.\n## happy\nok\n");
    assert.deepEqual(await findings(root), [planning("docs/guide/x.md")]);
  });
});

for (const rel of ["docs/guide/04-plan.md", "docs/surfaces/codex.md"]) {
  test(`a retired reference in ${rel} is one finding at that path`, async () => {
    await withSkillsRepo(async (root) => {
      writeFile(root, rel, "Use `/planning` after `/ideation`.");
      assert.deepEqual(await findings(root), [planning(rel)]);
      writeFile(root, rel, "Sprint planning uses `/intent` and `/decompose`.");
      assert.deepEqual(await findings(root), []);
    });
  });
}

for (const stale of [
  "Type `/work` to enter the chain.",
  "the `work` entrance",
  "[work](../work/SKILL.md)",
  '"./skills/work"',
  "invoke `agentics:work` here",
]) {
  test(`a retired work reference is one finding: ${JSON.stringify(stale)}`, async () => {
    await withSkillsRepo(async (root) => {
      writeFile(root, "skills/example/SKILL.md", stale);
      assert.deepEqual(await findings(root), [work("skills/example/SKILL.md")]);
    });
  });
}

test("ordinary prose about work is allowed", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(
      root,
      "skills/example/SKILL.md",
      "Start or continue work. The work folder holds plan.md; `work_folder` is reported; see `<plan-parent>/2026-10/work/plan.md`.",
    );
    writeFile(
      root,
      "docs/RENAMES.md",
      "`work` skill retired; `/work` is gone.",
    );
    writeFile(
      root,
      "docs/specs/2026-10/feature-x/plan.md",
      "Previously typed `/work`.",
    );
    assert.deepEqual(await findings(root), []);
  });
});

test("the second planning pattern has no leading boundary and the first has none to give", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(root, "skills/a.md", "Sprint planning skill");
    writeFile(root, "skills/b.md", "planning workflow");
    assert.deepEqual(await findings(root), [planning("skills/a.md")]);
  });
});

test("an arrow with no space around it fires", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(root, "skills/a.md", `ideation${ARROW}`);
    writeFile(root, "skills/b.md", `${ARROW}planning`);
    assert.deepEqual(await findings(root), [
      planning("skills/a.md"),
      planning("skills/b.md"),
    ]);
  });
});

test("one file with both a planning and a work reference gets two findings, planning first", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(root, "skills/example/SKILL.md", "`/work` then `/planning`");
    assert.deepEqual(await findings(root), [
      planning("skills/example/SKILL.md"),
      work("skills/example/SKILL.md"),
    ]);
  });
});

test("a file under a tests directory and one under node_modules are scanned", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(root, "skills/zealot/references/tests/x.md", "`/planning`");
    writeFile(root, "skills/node_modules/x.md", "`/planning`");
    assert.deepEqual(brief(await findings(root)), [
      [RULE, "skills/node_modules/x.md"],
      [RULE, "skills/zealot/references/tests/x.md"],
    ]);
  });
});

test("a file holding a NUL byte is scanned (no binary filter)", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(root, "skills/example/bin.md", "a\0b `/planning`");
    assert.deepEqual(await findings(root), [planning("skills/example/bin.md")]);
  });
});

test("each of the eight roots is scanned", async () => {
  await withSkillsRepo(async (root) => {
    const rels = [
      "docs/guide/x.md",
      "docs/surfaces/x.md",
      "evals/skills/x.md",
      ".github/scripts/x.sh",
      ".claude-plugin/x.json",
      ".codex-plugin/x.json",
      ".github/plugin/x.json",
      "skills/x/SKILL.md",
    ];
    for (const rel of rels) writeFile(root, rel, "`/planning`");
    assert.deepEqual(
      (await findings(root)).map((one) => one.path),
      [...rels].sort(),
    );
  });
});

test("a root that is missing is skipped without a finding or an error", async () => {
  await withSkillsRepo(async (root) => {
    remove(root, "skills");
    assert.deepEqual(await audit(root), { failures: [] });
  });
});

test(
  "a root that is a symlink is skipped even when its target holds a reference",
  { skip: NO_LINKS },
  async () => {
    await withOutside(async (outside) => {
      writeFile(outside, "x.md", "`/planning`");
      await withSkillsRepo(async (root) => {
        remove(root, "docs/guide");
        assert.equal(link(root, "docs/guide", outside, "dir"), true);
        assert.deepEqual(await audit(root), { failures: [] });
      });
    });
  },
);

test("a root that is a file is walked as that one file and named by the root's own path", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(root, "docs/guide", "`/planning`");
    assert.deepEqual(await findings(root), [planning("docs/guide")]);
  });
});

test(
  "a path below a regular file is an audit error on a host that reports ENOTDIR",
  {
    skip:
      process.platform === "win32"
        ? "Windows reports a path below a file as not existing, so the root is skipped"
        : false,
  },
  async () => {
    await withSkillsRepo(async (root) => {
      remove(root, "docs");
      writeFile(root, "docs", "a file where a directory should be");
      const result = await audit(root);
      assert.deepEqual(result.failures, []);
      assert.equal(result.error, "docs/guide: is not a directory");
    });
  },
);

test(
  "a path below a regular file is skipped on a host that reports it as not existing",
  {
    skip:
      process.platform === "win32"
        ? false
        : "POSIX hosts report ENOTDIR, an audit error (covered above)",
  },
  async () => {
    await withSkillsRepo(async (root) => {
      remove(root, "docs");
      writeFile(root, "docs", "a file where a directory should be");
      assert.deepEqual(await audit(root), { failures: [] });
    });
  },
);

test(
  "a symlinked file inside a root is not reported",
  { skip: NO_LINKS },
  async () => {
    await withOutside(async (outside) => {
      writeFile(outside, "x.md", "`/planning`");
      await withSkillsRepo(async (root) => {
        assert.equal(
          link(
            root,
            "skills/example/link.md",
            path.join(outside, "x.md"),
            "file",
          ),
          true,
        );
        assert.deepEqual(await audit(root), { failures: [] });
      });
    });
  },
);

test("a file that cannot be read is an audit error naming it and the error code", async () => {
  await withSkillsRepo(async (root) => {
    const result = await withSeams(
      {
        readFile: () => {
          throw Object.assign(new Error("denied"), { code: "EACCES" });
        },
      },
      () => audit(root),
    );
    assert.deepEqual(result.failures, []);
    assert.equal(result.error, "skills/challenge/SKILL.md: EACCES");
  });
});

test("a file read that fails with no code is unreadable", async () => {
  await withSkillsRepo(async (root) => {
    const result = await withSeams(
      {
        readFile: () => {
          throw new Error("odd failure");
        },
      },
      () => audit(root),
    );
    assert.equal(result.error, "skills/challenge/SKILL.md: unreadable");
  });
});

test("a failing directory read is an audit error naming the directory and drops the findings so far", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(root, "skills/aaa.md", "`/planning`");
    mkdir(root, "docs/guide");
    const real = seams.readDir;
    const result = await withSeams(
      {
        readDir: (dir) => {
          if (dir === path.join(root, "docs", "guide"))
            throw Object.assign(new Error("denied"), { code: "EACCES" });
          return real(dir);
        },
      },
      () => audit(root),
    );
    assert.deepEqual(result.failures, []);
    assert.equal(result.error, "docs/guide: EACCES");
  });
});

/** Runs the audit with the inspection of `target` failing with `error`, and every other path inspected as usual. */
function auditFailingLstat(
  root: string,
  target: string,
  error: unknown,
): Promise<{ failures: Failure[]; error?: string }> {
  const real = seams.lstat;
  return withSeams(
    {
      lstat: (candidate) => {
        if (candidate === target) throw error;
        return real(candidate);
      },
    },
    () => audit(root),
  );
}

const guideRoot = (root: string): string => path.join(root, "docs", "guide");

test("a root whose inspection fails is an audit error naming it and the error code", async () => {
  await withSkillsRepo(async (root) => {
    mkdir(root, "docs/guide");
    const target = guideRoot(root);
    const result = await auditFailingLstat(
      root,
      target,
      Object.assign(new Error("EACCES: permission denied, lstat"), {
        code: "EACCES",
        syscall: "lstat",
        path: target,
      }),
    );
    assert.deepEqual(result.failures, []);
    assert.equal(result.error, "docs/guide: EACCES");
  });
});

test("an inspection failure from another call is named by its code", async () => {
  await withSkillsRepo(async (root) => {
    mkdir(root, "docs/guide");
    const target = guideRoot(root);
    const result = await auditFailingLstat(
      root,
      target,
      Object.assign(new Error("open failed"), {
        code: "EACCES",
        syscall: "open",
        path: target,
      }),
    );
    assert.equal(result.error, "docs/guide: EACCES");
  });
});

test("an inspection failure with no code is unreadable", async () => {
  await withSkillsRepo(async (root) => {
    mkdir(root, "docs/guide");
    const result = await auditFailingLstat(
      root,
      guideRoot(root),
      new Error("odd failure"),
    );
    assert.equal(result.error, "docs/guide: unreadable");
  });
});

test("an inspection that throws no error object is unreadable", async () => {
  await withSkillsRepo(async (root) => {
    mkdir(root, "docs/guide");
    const result = await auditFailingLstat(root, guideRoot(root), null);
    assert.equal(result.error, "docs/guide: unreadable");
  });
});

test("a root whose inspection fails as missing is skipped", async () => {
  await withSkillsRepo(async (root) => {
    mkdir(root, "docs/guide");
    const result = await auditFailingLstat(
      root,
      guideRoot(root),
      Object.assign(new Error("gone"), { code: "ENOENT" }),
    );
    assert.deepEqual(result, { failures: [] });
  });
});

test("an audit error keeps the failure that caused it as its cause", async () => {
  await withSkillsRepo(async (root) => {
    mkdir(root, "docs/guide");
    const target = guideRoot(root);
    const cause = Object.assign(new Error("denied"), {
      code: "EACCES",
      syscall: "lstat",
      path: target,
    });
    const real = seams.lstat;
    assert.throws(
      () =>
        withSeams(
          {
            lstat: (candidate) => {
              if (candidate === target) throw cause;
              return real(candidate);
            },
          },
          () => retiredSkillRules(root),
        ),
      (error: unknown) =>
        error instanceof Error &&
        error.cause instanceof RepoFileError &&
        error.cause.cause === cause,
    );
  });
});

// --- the whole-domain scenarios ------------------------------------------------------------------------------------

const CONTENT_SLICE = sliceNamed("content");

test("retired-skill-reference", async () => {
  assert.deepEqual(await auditEveryFamily(breaker(RULE)), {
    failures: [planning("docs/guide/overview.md")],
  });
});

test("a retired reference after the happy heading of a bank file", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      put("evals/skills/bank.md", "Intro.\n\n## happy\n\nRun `/ideation`.\n"),
    ),
    { failures: [planning("evals/skills/bank.md")] },
  );
});

test("a retired reference in a file outside the evals tree before a happy heading", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      put("docs/guide/bank.md", "Old `/ideation` runs.\n\n## happy\n\nFine.\n"),
    ),
    { failures: [planning("docs/guide/bank.md")] },
  );
});

test("both retired entries in one file", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      put("docs/guide/both.md", "Use `/ideation` and `/work` here.\n"),
    ),
    { failures: [planning("docs/guide/both.md"), work("docs/guide/both.md")] },
  );
});

test("Sprint planning skill", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      put("docs/guide/sprint.md", "Our Sprint planning skill grows.\n"),
    ),
    { failures: [planning("docs/guide/sprint.md")] },
  );
});

test("a retired reference in a file holding a NUL byte", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      put("docs/guide/bin.md", `${NUL}\nRun \`/ideation\` first.\n`),
    ),
    { failures: [planning("docs/guide/bin.md")] },
  );
});

test("a retired skill path", async () => {
  assert.deepEqual(
    await auditSlice(CONTENT_SLICE, put("evals/skills/work.md", "# Work\n")),
    { failures: [work("evals/skills/work.md")] },
  );
});

test("a retired reference with an arrow", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      put("docs/guide/arrow.md", `planning ${ARROW_RIGHT} spec\n`),
    ),
    { failures: [planning("docs/guide/arrow.md")] },
  );
});

test("a retired reference in the github scripts directory", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      put(".github/scripts/run.sh", "echo `/ideation`\n"),
    ),
    { failures: [planning(".github/scripts/run.sh")] },
  );
});

test("a skill directory under a retired path", async () => {
  assert.deepEqual(
    await auditSlice(CONTENT_SLICE, (r) => addSkill(r, "ideation")),
    { failures: [planning("skills/ideation/SKILL.md")] },
  );
});

test("a guide path that is a file naming a retired skill", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      put("docs/guide", "Run `/ideation` first.\n"),
    ),
    { failures: [planning("docs/guide")] },
  );
});
