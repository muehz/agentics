import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import {
  chmodSync,
  existsSync,
  mkdtempSync,
  readdirSync,
  renameSync,
  rmSync,
} from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { test } from "node:test";
import { fileURLToPath } from "node:url";
import {
  ENV_SHORT,
  type AuditResult,
  type DomainRun,
  type Failure,
  type RunOptions,
} from "../../check.ts";
import {
  addX,
  all,
  asDirectory,
  auditEveryFamily,
  auditSlice,
  CLEAN_RUN,
  EMPTY_SKILL_FINDING,
  NEEDS_SYMLINK,
  sliceNamed,
  AXES_MODE,
  BAD_STATE,
  BOM,
  breaker,
  CHALLENGE,
  challengeStepThree,
  CHECKS_LINK_REASON,
  closing,
  CONTRACT_LINK,
  crlf,
  D0,
  D1,
  DIR_SKILL_REASON,
  E_ACUTE,
  EMOJI,
  EMPTY_SKILL,
  F,
  FENCE,
  FENCE4,
  full,
  GARBAGE,
  GATE_SKILL,
  GIT_COMMIT,
  gitSkill,
  LINE_SEP,
  NBSP,
  NO_CLOSING,
  NO_SKILL_ASSETS,
  NOT_VALID,
  NUL,
  nulInEverySkillsFile,
  PLAN_TEMPLATE,
  PROHIBITION,
  PROTOTYPE_REFS,
  PROTOTYPE_SKILL,
  put,
  QUESTION_CONTRACT,
  ref,
  renameSkills,
  REQUIRED,
  ROLLUP_TEMPLATE,
  skillWithMetadata,
  SPEC_AXIS,
  symlinked,
  TRIAGE_REFS,
  triageLink,
  triageRef,
  versionAtLine,
  WORK_FOLDER_GATE,
  X_SKILL,
  xBody,
  xFields,
  xText,
  type Mutation,
} from "../../testing/skills-seeds.ts";
import {
  BREAKERS,
  contractFixtureMD,
  evalsCheck as cleanEvalsCheck,
  FOOTER,
  link,
  mkdir,
  mutateFile,
  remove,
  skillMD,
  withSkillsRepo,
  writeFile,
} from "../../testing/skills-repo.ts";
import { writeStandardsBaseline } from "../../testing/standards-baseline.ts";
import { definedEnv } from "../tools/exec.ts";
import {
  EVALS_ROOT_ENV,
  evalsRoot,
  isDirectory,
  runEvalsCheck,
  SKIP_LINE,
} from "./eval-check.ts";
import { INLINE_FAMILY } from "./frontmatter.ts";
import {
  FAMILIES,
  RULE_FAMILY,
  ruleFamilyOf,
  run,
  type EvalsCheck,
  type Family,
  type ResolvedSkillsContext,
} from "./index.ts";
import { makesUnreadableDirs, removeTree } from "../../testing/host.ts";

const options: RunOptions = {
  short: false,
  fast: false,
  diagnostics: () => {},
  env: process.env,
};

/** The context of a run through the core families with an evals runner that finds nothing and starts no process. */
const CLEAN = { evalsCheck: cleanEvalsCheck, families: FAMILIES };

const failure = (rule: string, at = ""): Failure => ({
  rule,
  path: at,
  message: rule,
});

/** A family that returns the given failures. */
const returning = (name: string, found: Failure[]): Family => ({
  name,
  rules: found.map((one) => one.rule),
  run: () => found,
});

/** A family that throws `thrown`. */
const throwing = (name: string, thrown: unknown): Family => ({
  name,
  rules: [],
  run: () => {
    throw thrown;
  },
});

test("run is assignable to DomainRun", () => {
  const asDomain: DomainRun = run;
  assert.equal(typeof asDomain, "function");
});

test("the failures come back in the order the families ran, not re-sorted", async () => {
  const families = [
    returning("one", [failure("z-rule", "z.md"), failure("y-rule", "y.md")]),
    returning("two", [failure("a-rule", "a.md")]),
    returning("three", [failure("m-rule", "m.md")]),
  ];
  assert.deepEqual(await run("/r", options, { families }), {
    failures: [
      failure("z-rule", "z.md"),
      failure("y-rule", "y.md"),
      failure("a-rule", "a.md"),
      failure("m-rule", "m.md"),
    ],
  });
});

test("a family that throws stops the run: earlier failures stay, the failing family adds none, later families never run", async () => {
  let laterRuns = 0;
  const families: Family[] = [
    returning("one", [failure("first")]),
    {
      name: "two",
      rules: [],
      run: () => {
        throw new Error("two blew up");
      },
    },
    {
      name: "three",
      rules: [],
      run: () => {
        laterRuns += 1;
        return [failure("third")];
      },
    },
  ];
  assert.deepEqual(await run("/r", options, { families }), {
    failures: [failure("first")],
    error: "two blew up",
  });
  assert.equal(laterRuns, 0);
});

test("an error that carries failures contributes none of them", async () => {
  const partial = Object.assign(new Error("partial audit"), {
    failures: [failure("attached")],
  });
  const families = [
    returning("one", [failure("first")]),
    throwing("two", partial),
  ];
  assert.deepEqual(await run("/r", options, { families }), {
    failures: [failure("first")],
    error: "partial audit",
  });
});

test("a family that returns a rejected promise behaves like one that throws", async () => {
  let laterRuns = 0;
  const families: Family[] = [
    returning("one", [failure("first")]),
    {
      name: "two",
      rules: [],
      run: () => Promise.reject(new Error("async failure")),
    },
    {
      name: "three",
      rules: [],
      run: () => {
        laterRuns += 1;
        return [];
      },
    },
  ];
  assert.deepEqual(await run("/r", options, { families }), {
    failures: [failure("first")],
    error: "async failure",
  });
  assert.equal(laterRuns, 0);
});

test("a family that returns a resolved promise is awaited", async () => {
  const families: Family[] = [
    {
      name: "one",
      rules: [],
      run: async () => {
        await Promise.resolve();
        return [failure("late")];
      },
    },
    returning("two", [failure("after")]),
  ];
  assert.deepEqual(await run("/r", options, { families }), {
    failures: [failure("late"), failure("after")],
  });
});

test("a value thrown that is not an Error becomes the error text", async () => {
  const families = [
    returning("one", [failure("first")]),
    throwing("two", "plain string"),
  ];
  assert.deepEqual(await run("/r", options, { families }), {
    failures: [failure("first")],
    error: "plain string",
  });
});

test("with no error the result has no error key at all", async () => {
  const result = await run("/r", options, {
    families: [returning("one", [failure("only")])],
  });
  assert.deepEqual(result, { failures: [failure("only")] });
  assert.equal("error" in result, false);
});

test("an empty list of families runs nothing", async () => {
  assert.deepEqual(await run("/r", options, { families: [] }), {
    failures: [],
  });
});

test("every family receives the same fully resolved context, carrying the injected runner", async () => {
  const seen: ResolvedSkillsContext[] = [];
  const record = (name: string): Family => ({
    name,
    rules: [],
    run: (_root, context) => {
      seen.push(context);
      return [];
    },
  });
  const evalsCheck: EvalsCheck = () => [];
  const families = [record("one"), record("two"), record("three")];
  await run("/r", options, { families, evalsCheck });
  assert.equal(seen.length, 3);
  assert.ok(seen.every((context) => context === seen[0]));
  assert.equal(seen[0]?.evalsCheck, evalsCheck);
  assert.equal(seen[0]?.families, families);
});

test("every family receives the repository root", async () => {
  const roots: string[] = [];
  const families: Family[] = ["one", "two"].map((name) => ({
    name,
    rules: [],
    run: (root: string) => {
      roots.push(root);
      return [];
    },
  }));
  await run("/the/root", options, { families });
  assert.deepEqual(roots, ["/the/root", "/the/root"]);
});

test("with no runner given, every family receives the exported runEvalsCheck as the evals runner", async () => {
  const seen: ResolvedSkillsContext[] = [];
  const recording: Family = {
    name: "recording",
    rules: [],
    run: (_root, context) => {
      seen.push(context);
      return [];
    },
  };
  assert.deepEqual(await run("/r", options, { families: [recording] }), {
    failures: [],
  });
  assert.equal(seen.length, 1);
  assert.equal(seen[0]?.evalsCheck, runEvalsCheck);
});

test("with no families given, the run uses DOMAIN_FAMILIES: the baselined green fixture passes and the injected runner runs once, last", async () => {
  await withSkillsRepo(async (root) => {
    writeStandardsBaseline(root);
    const calls: string[] = [];
    const evalsCheck: EvalsCheck = (at) => {
      calls.push(at);
      return [];
    };
    assert.deepEqual(await run(root, options, { evalsCheck }), {
      failures: [],
    });
    assert.deepEqual(calls, [root]);
  });
});

test("options are accepted and ignored", async () => {
  const families = [returning("one", [failure("only")])];
  const quiet = await run("/r", { ...options, short: false }, { families });
  const short = await run("/r", { ...options, short: true }, { families });
  assert.deepEqual(quiet, short);
});

// --- the wired roster -------------------------------------------------------------------------------------------

const EM_DASH = String.fromCodePoint(0x2014);

/** The names of the core families, in audit order. */
const ROSTER = [
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

/** The rule ids of the core families, each with its family, in family order. */
const PINNED_RULE_FAMILY: Record<string, string> = {
  "skill-md-present": "inline",
  "skill-frontmatter": "inline",
  "skill-name-unique": "inline",
  "frontmatter-yaml": "frontmatter",
  "eval-frontmatter-present": "eval-metadata",
  "eval-history-footer": "eval-metadata",
  "eval-frontmatter-clean": "eval-frontmatter-clean",
  "orchestration-file-present": "orchestration",
  "orchestration-stale-phrase": "orchestration",
  "orchestration-native-identifier": "orchestration",
  "reference-link": "reference-link",
  "attribution-centralized": "reference-link",
  "reference-attribution": "reference-link",
  "notices-manifest": "reference-link",
  "reference-naming": "reference-naming",
  "triage-file-present": "triage",
  "triage-detection-routing": "triage",
  "triage-enum-mapping": "triage",
  "triage-rollup-shape": "triage",
  "triage-attestation-footer": "triage",
  "triage-attestation-prefix": "triage",
  "triage-attestation-legacy": "triage",
  "triage-attestation-fence": "triage",
  "triage-attestation-issue-mode": "triage",
  "triage-attestation-contract-example": "triage",
  "skill-artifact-declared": "artifact",
  "skill-artifact-expected": "artifact",
  "skill-artifact-unique": "artifact",
  "retired-skill-reference": "retired-skill",
  "ai-attribution-trailer": "ai-attribution",
  "skill-repo-independence": "independence",
  "work-folder-source-present": "workfolder",
  "work-folder-pin": "workfolder",
  "spec-pass-pinned": "spec-pass",
  "skill-legacy-names": "legacy-names",
  "state-tsv-shape": "state-tsv",
  "flow-emission-declared": "flow-emission",
  "lean-cadence-pins": "lean-cadence",
  "lean-cadence-source-present": "lean-cadence",
};

/** The core rule ids, pinned on their own so the map above cannot shrink unnoticed. */
const RULE_IDS = Object.keys(PINNED_RULE_FAMILY);

test("FAMILIES holds the 19 core families in audit order, each name once", () => {
  assert.deepEqual(
    FAMILIES.map((family) => family.name),
    ROSTER,
  );
  assert.equal(new Set(FAMILIES.map((family) => family.name)).size, 19);
});

test("RULE_FAMILY maps the 39 core rule ids to their families in family order", () => {
  assert.equal(RULE_IDS.length, 39);
  assert.deepEqual(RULE_FAMILY, PINNED_RULE_FAMILY);
  assert.deepEqual(Object.keys(RULE_FAMILY), RULE_IDS);
});

test("the rule map keys are the union of every family's rules, no rule id is in two families, and eval-check lists none", () => {
  const listed = FAMILIES.flatMap((family) => [...family.rules]);
  assert.equal(new Set(listed).size, listed.length);
  assert.deepEqual(listed, RULE_IDS);
  assert.deepEqual(Object.keys(RULE_FAMILY), listed);
  for (const family of FAMILIES)
    for (const rule of family.rules)
      assert.equal(RULE_FAMILY[rule], family.name, rule);
  const evalCheck = FAMILIES.find((family) => family.name === "eval-check");
  assert.deepEqual(evalCheck?.rules, []);
});

test("the rule map builder rejects a rule id owned by two families and one repeated inside a family", () => {
  const fake = (name: string, rules: string[]): Family => ({
    name,
    rules,
    run: () => [],
  });
  assert.throws(
    () => ruleFamilyOf([fake("one", ["shared"]), fake("two", ["shared"])]),
    /shared/,
  );
  assert.throws(() => ruleFamilyOf([fake("one", ["twice", "twice"])]), /twice/);
  assert.deepEqual(ruleFamilyOf([fake("one", ["a"]), fake("two", ["b"])]), {
    a: "one",
    b: "two",
  });
});

// --- the real families through run ------------------------------------------------------------------------------

const brief = (failures: Failure[]): string[] =>
  failures.map((one) => one.rule);

const NOTICES = "THIRD_PARTY_NOTICES.md";
const NOTICES_HEADING = "## Per-file provenance manifest";
const VERSION_LINE = 'version: "1.0.0"\n';

/** Breaks skill-frontmatter, orchestration-stale-phrase and notices-manifest at once. */
function breakThree(root: string): void {
  mutateFile(root, "skills/docs/SKILL.md", VERSION_LINE, "");
  writeFile(
    root,
    "docs/surfaces/codex.md",
    "# Adapter\n\nNote that subagents are stateless here.\n",
  );
  mutateFile(root, NOTICES, NOTICES_HEADING, "## Something else");
}

const THREE_FAILURES: Failure[] = [
  {
    rule: "skill-frontmatter",
    path: "skills/docs/SKILL.md",
    message:
      "missing required frontmatter (name/description/version) in the first 20 lines",
  },
  {
    rule: "orchestration-stale-phrase",
    path: "docs/surfaces/codex.md",
    message: 'line 3: stale universal claim: "subagents are stateless"',
  },
  {
    rule: "notices-manifest",
    path: NOTICES,
    message: 'missing "## Per-file provenance manifest" section',
  },
];

test("the default families pass the green fixture with no failures and no error member", async () => {
  await withSkillsRepo(async (root) => {
    const result = await run(root, options, CLEAN);
    assert.deepEqual(result, { failures: [] });
    assert.equal("error" in result, false);
  });
});

test("failures from different families come out in family order with each family's messages", async () => {
  await withSkillsRepo(async (root) => {
    breakThree(root);
    assert.deepEqual(await run(root, options, CLEAN), {
      failures: THREE_FAILURES,
    });
  });
});

test("an inline failure precedes a reference-naming failure", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(root, "skills/docs/references/Bad_Name.md", "x\n");
    mutateFile(root, "skills/docs/SKILL.md", VERSION_LINE, "");
    assert.deepEqual(brief((await run(root, options, CLEAN)).failures), [
      "skill-frontmatter",
      "reference-naming",
    ]);
  });
});

test("a subset of one family runs only that family", async () => {
  await withSkillsRepo(async (root) => {
    breakThree(root);
    assert.deepEqual(await run(root, options, { families: [INLINE_FAMILY] }), {
      failures: [THREE_FAILURES[0] as Failure],
    });
  });
});

/** The skills directories that hold a references directory. */
function referenceOwners(root: string): string[] {
  return readdirSync(path.join(root, "skills")).filter((name) =>
    readdirSync(path.join(root, "skills", name)).includes("references"),
  );
}

test("the first family that raises an audit error stops the run: earlier failures stay, its own are dropped", async () => {
  await withSkillsRepo(
    async (root) => {
      mkdir(root, "skills/empty");
      for (const name of referenceOwners(root))
        remove(root, `skills/${name}/references`);
      const result = await run(root, options, CLEAN);
      const sixFamilies = await run(root, options, {
        families: FAMILIES.slice(0, 6),
      });
      assert.equal(sixFamilies.error, undefined);
      assert.deepEqual(result, {
        failures: sixFamilies.failures,
        error: `skills: no reference files found ${EM_DASH} the reference-naming glob is wrong`,
      });
      assert.deepEqual(brief(result.failures), [
        "skill-md-present",
        "orchestration-file-present",
        "orchestration-file-present",
        "orchestration-file-present",
        "orchestration-file-present",
        ...Array<string>(4).fill("reference-link"),
      ]);
    },
    { sharedContracts: "absent" },
  );
});

test("a family with failures that then raises an audit error loses its own failures only", async (t) => {
  await withSkillsRepo(async (root) => {
    mutateFile(root, "skills/docs/SKILL.md", VERSION_LINE, "");
    writeFile(
      root,
      "skills/prototype/references/notes.md",
      "[gone](./missing-doc.md)\n",
    );
    const before = await run(root, options, CLEAN);
    assert.equal(before.error, undefined);
    assert.deepEqual(brief(before.failures), [
      "skill-frontmatter",
      "reference-link",
    ]);

    const outside = mkdtempSync(path.join(tmpdir(), "agentics-skills-index-"));
    try {
      writeFile(outside, NOTICES, NOTICES_HEADING + "\n");
      remove(root, NOTICES);
      if (!link(root, NOTICES, path.join(outside, NOTICES), "file"))
        return t.skip("the host refuses to create a file symlink");
      const after: AuditResult = await run(root, options, CLEAN);
      assert.deepEqual(brief(after.failures), ["skill-frontmatter"]);
      assert.ok(
        after.error?.startsWith(
          `skills: reading ${NOTICES}: ${NOTICES}: passes through a link`,
        ),
        String(after.error),
      );
    } finally {
      removeTree(outside);
    }
  });
});

// --- the content slice's families through run -----------------------------------------------------------------------

const DETECTION = "skills/triage/references/source-detection.md";
const CHECKS_DIR = "skills/triage/references/checks";
const LEAK = "skills/prototype/references/leak.md";

/** The (rule, path) pairs of a result. */
const pairs = (failures: Failure[]): string[][] =>
  failures.map((one) => [one.rule, one.path]);

/** One mutation per content-slice family, each with the one (rule, path) it must produce. */
const CONTENT_SLICE_MUTATIONS: ReadonlyArray<{
  family: string;
  mutate: (root: string) => void;
  expected: string[];
}> = [
  {
    family: "artifact",
    mutate: (root) =>
      mutateFile(
        root,
        "skills/decompose/SKILL.md",
        "    artifact: plan.md\n",
        "",
      ),
    expected: ["skill-artifact-declared", "skills/decompose/SKILL.md"],
  },
  {
    family: "triage",
    mutate: (root) => remove(root, DETECTION),
    expected: ["triage-file-present", DETECTION],
  },
  {
    family: "retired-skill",
    mutate: (root) => writeFile(root, LEAK, "The planning owns the fork.\n"),
    expected: ["retired-skill-reference", LEAK],
  },
  {
    family: "ai-attribution",
    mutate: (root) => writeFile(root, LEAK, "Generated-by: codex\n"),
    expected: ["ai-attribution-trailer", LEAK],
  },
  {
    family: "independence",
    mutate: (root) => writeFile(root, LEAK, "See docs/CHARTER.md.\n"),
    expected: ["skill-repo-independence", LEAK],
  },
  {
    family: "legacy-names",
    mutate: (root) => writeFile(root, LEAK, "Call the chain state verb.\n"),
    expected: ["skill-legacy-names", LEAK],
  },
];

for (const { family, mutate, expected } of CONTENT_SLICE_MUTATIONS) {
  test(`the default list reports one ${family} failure for one ${family} mutation and nothing else`, async () => {
    await withSkillsRepo(async (root) => {
      mutate(root);
      const result = await run(root, options, CLEAN);
      assert.equal(result.error, undefined);
      assert.deepEqual(pairs(result.failures), [expected]);
    });
  });
}

test("a triage failure prints before a legacy-names failure, in family order", async () => {
  await withSkillsRepo(async (root) => {
    remove(root, DETECTION);
    writeFile(root, LEAK, "Call the chain state verb.\n");
    const result = await run(root, options, CLEAN);
    assert.equal(result.error, undefined);
    assert.deepEqual(pairs(result.failures), [
      ["triage-file-present", DETECTION],
      ["skill-legacy-names", LEAK],
    ]);
  });
});

/** Replaces `references/checks` with a link to a directory outside the fixture; false when the host cannot link. */
function linkChecks(root: string, outside: string): boolean {
  remove(root, CHECKS_DIR);
  return (
    link(root, CHECKS_DIR, outside, "dir") ||
    link(root, CHECKS_DIR, outside, "junction")
  );
}

test("the triage audit error stops the run: an earlier family's failure stays, later families never run", async (t) => {
  await withSkillsRepo(async (root) => {
    const outside = mkdtempSync(path.join(tmpdir(), "agentics-skills-index-"));
    try {
      if (!linkChecks(root, outside))
        return t.skip("the host refuses to create a directory link");
      mutateFile(root, "skills/docs/SKILL.md", VERSION_LINE, "");
      mutateFile(
        root,
        "skills/decompose/SKILL.md",
        "    artifact: plan.md\n",
        "",
      );
      writeFile(root, LEAK, "Call the chain state verb.\n");
      const result = await run(root, options, CLEAN);
      assert.deepEqual(brief(result.failures), ["skill-frontmatter"]);
      assert.equal(
        result.error,
        `skills: reading ${CHECKS_DIR}: ${CHECKS_DIR}: passes through a link`,
      );
    } finally {
      removeTree(outside);
    }
  });
});

test("the triage audit error drops the failures triage found before it raised", async (t) => {
  await withSkillsRepo(async (root) => {
    const outside = mkdtempSync(path.join(tmpdir(), "agentics-skills-index-"));
    try {
      const triage = FAMILIES.filter((family) => family.name === "triage");
      // Control: without the link, the same mutation is a triage finding, so triage does find it.
      remove(root, DETECTION);
      assert.deepEqual(
        pairs((await run(root, options, { families: triage })).failures),
        [["triage-file-present", DETECTION]],
      );
      // With the link, triage reports the missing file first and then raises on the linked directory.
      if (!linkChecks(root, outside))
        return t.skip("the host refuses to create a directory link");
      const triageOnly = await run(root, options, { families: triage });
      assert.deepEqual(triageOnly.failures, []);
      assert.ok(
        triageOnly.error?.startsWith(`skills: reading ${CHECKS_DIR}: `),
      );
      const result = await run(root, options, CLEAN);
      assert.deepEqual(result.failures, []);
      assert.equal(result.error, triageOnly.error);
    } finally {
      removeTree(outside);
    }
  });
});

test("a families list holding only artifact reports only artifact findings", async () => {
  await withSkillsRepo(async (root) => {
    remove(root, DETECTION);
    mutateFile(
      root,
      "skills/decompose/SKILL.md",
      "    artifact: plan.md\n",
      "",
    );
    const artifact = FAMILIES.filter((family) => family.name === "artifact");
    assert.equal(artifact.length, 1);
    const result = await run(root, options, { families: artifact });
    assert.equal(result.error, undefined);
    assert.deepEqual(pairs(result.failures), [
      ["skill-artifact-declared", "skills/decompose/SKILL.md"],
    ]);
  });
});

// --- the full roster through run: one breaker per rule -------------------------------------------------------------

// The non-ASCII characters of the expected messages, built from code points so the source stays ASCII.
const EM = EM_DASH;
const MIDDLE_DOT = String.fromCodePoint(0x00b7);

/**
 * The exact failures of each breaker through the full registered order with a clean evals runner, including every other
 * rule the breaker's change trips on this fixture.
 */
const EXPECTED: Readonly<Record<string, Failure[]>> = {
  "skill-md-present": [
    {
      rule: "skill-md-present",
      path: "skills/empty",
      message: "missing SKILL.md",
    },
  ],
  "skill-frontmatter": [
    {
      rule: "skill-frontmatter",
      path: "skills/docs/SKILL.md",
      message:
        "missing required frontmatter (name/description/version) in the first 20 lines",
    },
  ],
  "skill-name-unique": [
    {
      rule: "skill-name-unique",
      path: "skills/forge/SKILL.md",
      message:
        'duplicate skill name "docs": skills/docs/SKILL.md and skills/forge/SKILL.md',
    },
  ],
  "frontmatter-yaml": [
    {
      rule: "frontmatter-yaml",
      path: "skills/prototype/SKILL.md",
      message:
        "frontmatter is not a YAML mapping: Map keys must be unique (block line 2)",
    },
  ],
  "eval-frontmatter-present": [
    {
      rule: "skill-frontmatter",
      path: "skills/prototype/SKILL.md",
      message:
        "missing required frontmatter (name/description/version) in the first 20 lines",
    },
    {
      rule: "eval-frontmatter-present",
      path: "skills/prototype/SKILL.md",
      message: "empty or missing frontmatter",
    },
  ],
  "eval-history-footer": [
    {
      rule: "eval-history-footer",
      path: "skills/prototype/SKILL.md",
      message: `retired '## Validation history' footer found ${EM} record evals in evals/skills.yaml instead`,
    },
  ],
  "eval-frontmatter-clean": [
    {
      rule: "eval-frontmatter-clean",
      path: "skills/prototype/SKILL.md",
      message:
        "frontmatter holds metadata.eval-status; eval results belong in evals/skills.yaml, not in the skill file",
    },
  ],
  "orchestration-file-present": [
    {
      rule: "orchestration-file-present",
      path: "skills/routing/references/model-ladders.md",
      message: "exempted in the contract arrays but does not exist",
    },
  ],
  "orchestration-stale-phrase": [
    {
      rule: "orchestration-stale-phrase",
      path: "docs/surfaces/codex.md",
      message: 'line 3: stale universal claim: "subagents are stateless"',
    },
  ],
  "orchestration-native-identifier": [
    {
      rule: "orchestration-native-identifier",
      path: "AGENTS.md",
      message: "line 3: native orchestration identifier in a shared file",
    },
  ],
  "reference-link": [
    {
      rule: "reference-link",
      path: "skills/prototype/references/notes.md",
      message: "dangling link -> ./missing.md",
    },
  ],
  "attribution-centralized": [
    {
      rule: "attribution-centralized",
      path: "skills/prototype/references/origin.md",
      message:
        'in-file third-party attribution ("Adapted from example/project (MIT)"); record it in THIRD_PARTY_NOTICES.md instead',
    },
  ],
  "reference-attribution": [
    {
      rule: "reference-attribution",
      path: "skills/docs/SKILL.md",
      message:
        "pinned Apache-2.0 attribution notice (source + license) missing",
    },
  ],
  "notices-manifest": [
    {
      rule: "notices-manifest",
      path: "THIRD_PARTY_NOTICES.md",
      message: 'missing "## Per-file provenance manifest" section',
    },
  ],
  "reference-naming": [
    {
      rule: "reference-naming",
      path: "skills/prototype/references/Bad_Name.md",
      message: `reference file name is not lowercase kebab-case (exempt: README.md, LICENSE) ${EM} rename with 'git mv'`,
    },
  ],
  "triage-file-present": [
    {
      rule: "triage-file-present",
      path: "skills/triage/references/source-detection.md",
      message: "required triage contract file is missing",
    },
  ],
  "triage-detection-routing": [
    {
      rule: "triage-detection-routing",
      path: "skills/triage/references/source-detection.md",
      message: `line 10: bot routing table must not carry a checkmarx login row ${EM} detection is marker-keyed`,
    },
  ],
  "triage-enum-mapping": [
    {
      rule: "triage-enum-mapping",
      path: "skills/triage/references/rollup-template.md",
      message: "dispositions with no rollup mapping rule: deferred",
    },
  ],
  "triage-rollup-shape": [
    {
      rule: "triage-rollup-shape",
      path: "skills/triage/references/rollup-template.md",
      message: `example 1: header verdict is not \`N fixed ${MIDDLE_DOT} M needs you\` or \`Nothing needs you\`: @jdoe ${EM} all done ${MIDDLE_DOT} 1 needs you`,
    },
  ],
  "triage-attestation-footer": [
    {
      rule: "triage-attestation-footer",
      path: "skills/triage/references/pull-requests/copilot/copilot-triage.md",
      message: "missing the exact attestation footer",
    },
  ],
  "triage-attestation-prefix": [
    {
      rule: "triage-attestation-prefix",
      path: "skills/triage/references/notes.md",
      message:
        'line 3: footer missing its `> ` blockquote prefix: "_Triaged by an AI agent_"',
    },
  ],
  "triage-attestation-legacy": [
    {
      rule: "triage-attestation-legacy",
      path: "skills/triage/references/source-detection.md",
      message: "retains the superseded leading disclaimer",
    },
  ],
  "triage-attestation-fence": [
    {
      rule: "triage-attestation-fence",
      path: "skills/triage/references/rollup-template.md",
      message:
        'markered fence must close with the footer (ends "Closing line.")',
    },
  ],
  "triage-attestation-issue-mode": [
    {
      rule: "triage-attestation-issue-mode",
      path: "skills/triage/references/issue-mode.md",
      message:
        'disposition template must close with the footer (ends "Disposition template: fix.")',
    },
  ],
  "triage-attestation-contract-example": [
    {
      rule: "triage-attestation-contract-example",
      path: "skills/triage/references/subagent-contract.md",
      message:
        "JSON example body must open with the marker and close with the footer",
    },
  ],
  "skill-artifact-declared": [
    {
      rule: "skill-artifact-declared",
      path: "skills/decompose/SKILL.md",
      message:
        'chain skill "decompose" owns ["plan.md"] but declares no metadata.artifact',
    },
  ],
  "skill-artifact-expected": [
    {
      rule: "skill-artifact-expected",
      path: "skills/prototype/SKILL.md",
      message:
        'skill "prototype" declares metadata.artifact ["notes.md"] but owns no chain artifact; the field is absent, not empty, on a non-owner',
    },
  ],
  "skill-artifact-unique": [
    {
      rule: "skill-artifact-unique",
      path: "skills/decompose/SKILL.md",
      message:
        'artifact "plan.md" is declared by 2 skills: skills/decompose/SKILL.md, skills/prototype/SKILL.md',
    },
    {
      rule: "skill-artifact-expected",
      path: "skills/prototype/SKILL.md",
      message:
        'skill "prototype" declares metadata.artifact ["plan.md"] but owns no chain artifact; the field is absent, not empty, on a non-owner',
    },
  ],
  "retired-skill-reference": [
    {
      rule: "retired-skill-reference",
      path: "docs/guide/overview.md",
      message:
        "use intent and decompose; retired skill names remain in an active path or reference",
    },
  ],
  "ai-attribution-trailer": [
    {
      rule: "ai-attribution-trailer",
      path: "skills/prototype/references/commit-notes.md",
      message:
        "names an AI attribution trailer outside the prohibition in skills/git; commits Agentics writes carry no AI attribution",
    },
  ],
  "skill-repo-independence": [
    {
      rule: "skill-repo-independence",
      path: "skills/prototype/references/layout.md",
      message:
        'line 3 names "hooks/": a skill must not depend on an Agentics hook or on a file outside skills/',
    },
  ],
  "skill-legacy-names": [
    {
      rule: "skill-legacy-names",
      path: "skills/prototype/references/state-notes.md",
      message:
        'line 3 names "events.tsv": this state mechanism is retired; use state.tsv and the flow helper',
    },
  ],
  "work-folder-source-present": [
    {
      rule: "work-folder-source-present",
      path: "skills/decompose/references/chain-artifacts.md",
      message: "not found: the work-folder contract pins cannot be checked",
    },
  ],
  "work-folder-pin": [
    {
      rule: "work-folder-pin",
      path: "skills/gate/references/branch/tidy.md",
      message: "tidy.md must state that a name collision keeps both copies",
    },
  ],
  "spec-pass-pinned": [
    {
      rule: "spec-pass-pinned",
      path: "skills/review/references/axes/spec-axis.md",
      message:
        'skills/review/references/axes/spec-axis.md no longer carries the pinned sentence "## Per-requirement table"',
    },
  ],
  "state-tsv-shape": [
    {
      rule: "state-tsv-shape",
      path: "docs/specs/2026-01/feature-x/state.tsv",
      message:
        'line 1 is "# flow-state 2", want the header row or the version line "# flow-state 1"',
    },
  ],
  "flow-emission-declared": [
    {
      rule: "flow-emission-declared",
      path: "skills/gate/SKILL.md",
      message: 'gate does not name the flow helper call "agx flow"',
    },
  ],
  "lean-cadence-pins": [
    {
      rule: "lean-cadence-pins",
      path: "skills/decompose/references/plan-template.md",
      message: "missing: **Packet review:**",
    },
  ],
  "lean-cadence-source-present": [
    {
      rule: "lean-cadence-source-present",
      path: "skills/gh/references/stack.md",
      message: "not found: the lean-cadence pins cannot be checked",
    },
  ],
};

test("EXPECTED pins the breaker of every core rule, and BREAKERS adds only the question-format breaker", () => {
  const coreRules = Object.keys(BREAKERS).filter(
    (rule) => rule !== "question-format",
  );
  assert.deepEqual(Object.keys(EXPECTED).sort(), coreRules.sort());
  assert.deepEqual(coreRules, [...RULE_IDS].sort());
  assert.equal(Object.keys(BREAKERS).length, 40);
});

test("the baselined clean fixture yields no failures through every family, and the injected runner is called once with the root", async () => {
  await withSkillsRepo(async (root) => {
    writeStandardsBaseline(root);
    const calls: string[] = [];
    const result = await run(root, options, {
      evalsCheck: (at) => {
        calls.push(at);
        return [];
      },
    });
    assert.deepEqual(result, { failures: [] });
    assert.deepEqual(calls, [root]);
  });
});

for (const [rule, mutate] of Object.entries(BREAKERS)) {
  if (rule === "question-format") continue;
  test(`the ${rule} breaker gives exactly its pinned failures through the full family order`, async () => {
    await withSkillsRepo(async (root) => {
      mutate(root);
      const expected = EXPECTED[rule];
      assert.ok(expected !== undefined, rule);
      assert.ok(
        expected.some((one) => one.rule === rule),
        `${rule}: the pinned failures do not hold the breaker's own rule`,
      );
      assert.deepEqual(await run(root, options, CLEAN), {
        failures: expected,
      });
    });
  });
}

// --- the eval-check seam -----------------------------------------------------------------------------------------

const RUNNER_FAILURE: Failure = {
  rule: "eval-stub-rule",
  path: "evals/skills.yaml",
  message: "from the runner",
};

/** Applies the ai-attribution-trailer breaker: an earlier family's failure for the eval-check seam tests. */
function breakAttribution(root: string): void {
  const breaker = BREAKERS["ai-attribution-trailer"];
  assert.ok(breaker !== undefined);
  breaker(root);
}

test("the runner's failures come once, after every earlier family's failures", async () => {
  await withSkillsRepo(async (root) => {
    breakAttribution(root);
    let calls = 0;
    const result = await run(root, options, {
      families: FAMILIES,
      evalsCheck: () => {
        calls += 1;
        return [RUNNER_FAILURE];
      },
    });
    assert.equal(calls, 1);
    assert.deepEqual(result, {
      failures: [...(EXPECTED["ai-attribution-trailer"] ?? []), RUNNER_FAILURE],
    });
  });
});

test("a runner that throws gives its message as the error and keeps the earlier families' failures", async () => {
  await withSkillsRepo(async (root) => {
    breakAttribution(root);
    const result = await run(root, options, {
      families: FAMILIES,
      evalsCheck: () => {
        throw new Error("skills: evals check printed no closing line");
      },
    });
    assert.deepEqual(result, {
      failures: EXPECTED["ai-attribution-trailer"],
      error: "skills: evals check printed no closing line",
    });
  });
});

test("an audit error in an earlier family never calls the runner", async () => {
  await withSkillsRepo(async (root) => {
    for (const name of referenceOwners(root))
      remove(root, `skills/${name}/references`);
    let calls = 0;
    const result = await run(root, options, {
      families: FAMILIES,
      evalsCheck: () => {
        calls += 1;
        return [];
      },
    });
    assert.equal(
      result.error,
      `skills: no reference files found ${EM} the reference-naming glob is wrong`,
    );
    assert.equal(calls, 0);
  });
});

// --- a family that stops the run drops its own failures ----------------------------------------------------------

test("a spec-pass audit error after a missing spec-axis file drops spec-pass's own failure and keeps an earlier family's", async () => {
  await withSkillsRepo(async (root) => {
    breakAttribution(root);
    remove(root, "skills/review/references/axes/spec-axis.md");
    // Control: without the directory, the missing file is a spec-pass finding.
    const control = await run(root, options, CLEAN);
    assert.equal(control.error, undefined);
    assert.deepEqual(brief(control.failures), [
      "ai-attribution-trailer",
      "spec-pass-pinned",
    ]);
    remove(root, "skills/review/references/axes/axes-mode.md");
    mkdir(root, "skills/review/references/axes/axes-mode.md");
    const result = await run(root, options, CLEAN);
    // reference-naming reads the directory's .md name as a badly named reference directory.
    assert.deepEqual(brief(result.failures), [
      "reference-naming",
      "ai-attribution-trailer",
    ]);
    assert.ok(
      result.error?.startsWith(
        "cannot read skills/review/references/axes/axes-mode.md, which carries the spec-pass contract: ",
      ),
      String(result.error),
    );
  });
});

test("a lean-cadence audit error after a missing plan template drops lean-cadence's own failure and keeps an earlier family's", async (t) => {
  await withSkillsRepo(async (root) => {
    breakAttribution(root);
    const template = "skills/decompose/references/plan-template.md";
    remove(root, template);
    // Control: without the link, the missing template is a lean-cadence finding.
    const control = await run(root, options, CLEAN);
    assert.equal(control.error, undefined);
    assert.deepEqual(pairs(control.failures), [
      ["ai-attribution-trailer", "skills/prototype/references/commit-notes.md"],
      ["lean-cadence-source-present", template],
    ]);
    // The template is pin 1, read before the skills/execute directory pins, so its failure exists when the scan raises.
    if (
      !link(
        root,
        "skills/execute/references/linked.md",
        "flow-state.md",
        "file",
      )
    )
      return t.skip("the host refuses to create a file symlink");
    const result = await run(root, options, CLEAN);
    assert.deepEqual(result.failures, EXPECTED["ai-attribution-trailer"]);
    assert.equal(
      result.error,
      "skills: scanning skills/execute: skills/execute/references/linked.md: passes through a link",
    );
  });
});

// --- the trust boundary through the CLI --------------------------------------------------------------------------

const CHECKOUT = fileURLToPath(new URL("../../../../../../", import.meta.url));
const CLI = path.join(
  CHECKOUT,
  ".github",
  "tools",
  "src",
  "conformance",
  "cli.ts",
);
const HAVE_DEPS = existsSync(path.join(CHECKOUT, "node_modules", "yaml"));
const DEPS_SKIP =
  HAVE_DEPS || process.env["CI"] === "true"
    ? false
    : "this checkout has no node_modules/yaml; run npm ci --ignore-scripts";
/** The evals clone the suite runs the eval rules from. CI has none, so there the eval rules are skipped. */
const CLONE = evalsRoot(CHECKOUT, process.env);
const HAVE_CLONE = isDirectory(CLONE);
const CLONE_SKIP = HAVE_CLONE
  ? DEPS_SKIP
  : `${SKIP_LINE} at ${CLONE}; set ${EVALS_ROOT_ENV} to a muehz/evals clone to run this test`;

/** Runs the registered skills domain on `repo` from `cwd` with `env`, as the CLI does. */
function runSkillsCli(
  repo: string,
  cwd: string,
  env: Record<string, string | undefined>,
) {
  return spawnSync(
    process.execPath,
    ["--disable-warning=ExperimentalWarning", CLI, "skills", "--repo", repo],
    { cwd, env: definedEnv(env), encoding: "utf8" },
  );
}

test(
  "the registered skills domain passes this checkout through every family with the evals clone: PASS on stdout, nothing on stderr, exit 0",
  { skip: CLONE_SKIP },
  () => {
    const env: Record<string, string | undefined> = { ...process.env };
    delete env["AGX_CONFORMANCE_EVALS_CLI"];
    delete env[ENV_SHORT];
    const child = runSkillsCli(CHECKOUT, CHECKOUT, env);
    assert.equal(child.stdout, "PASS skills\n");
    assert.equal(child.stderr, "");
    assert.equal(child.status, 0);
  },
);

test(
  "the registered skills domain passes this checkout with no evals clone: PASS on stdout, only the skip line on stderr, exit 0",
  { skip: DEPS_SKIP },
  () => {
    assert.ok(HAVE_DEPS, "CI must install node_modules before this test runs");
    const missing = path.join(
      mkdtempSync(path.join(tmpdir(), "agentics-no-evals-")),
      "evals",
    );
    try {
      const env: Record<string, string | undefined> = {
        ...process.env,
        [EVALS_ROOT_ENV]: missing,
      };
      delete env["AGX_CONFORMANCE_EVALS_CLI"];
      delete env[ENV_SHORT];
      const child = runSkillsCli(CHECKOUT, CHECKOUT, env);
      assert.equal(child.stdout, "PASS skills\n");
      assert.equal(child.stderr, "skipped: no sibling evals clone\n");
      assert.equal(child.status, 0);
    } finally {
      rmSync(path.dirname(missing), { recursive: true, force: true });
    }
  },
);

test(
  "the registered skills domain never runs an evals CLI planted in the audited checkout, its working directory or the retired AGX_CONFORMANCE_TOOLS variable",
  { skip: DEPS_SKIP },
  async () => {
    assert.ok(HAVE_DEPS, "CI must install node_modules before this test runs");
    await withSkillsRepo(async (root) => {
      const marker = path.join(root, "planted-ran.txt");
      writeFile(
        root,
        "evals/src/cli/main.ts",
        `import { writeFileSync } from "node:fs";\nwriteFileSync(${JSON.stringify(marker)}, "ran");\nprocess.stdout.write('{"done":true,"failures":0}\\n');\n`,
      );
      writeFile(root, ".claude-plugin/plugin.json", "{}");
      mkdir(root, "tools/cli");
      const env: Record<string, string | undefined> = {
        ...process.env,
        AGX_CONFORMANCE_TOOLS: path.join(root, "tools", "cli"),
      };
      delete env["AGX_CONFORMANCE_EVALS_CLI"];
      const child = runSkillsCli(root, root, env);
      const output = `${child.stdout}${child.stderr}`;
      assert.match(output, /^(PASS|FAIL|ERROR) skills/m, output);
      // With the clone, its evals CLI audited the fixture, which holds no eval record, so eval-check reports eval
      // rules. Without it, the eval rules are skipped.
      if (HAVE_CLONE) assert.match(output, /^FAIL skills \[eval-/m, output);
      else assert.match(child.stderr, /^skipped: no sibling evals clone$/m);
      assert.equal(existsSync(marker), false, "the planted evals CLI ran");
    });
  },
);

// --- the clean-run scenarios ---------------------------------------------------------------------------------------

const METADATA_SLICE = sliceNamed("metadata");
const CONTENT_SLICE = sliceNamed("content");

/** The frontmatter-yaml finding on `skills/x/SKILL.md` with the reader's `reason`. */
const xNotYaml = (reason: string): AuditResult => ({
  failures: [
    {
      rule: "frontmatter-yaml",
      path: X_SKILL,
      message: `frontmatter is not a YAML mapping: ${reason}`,
    },
  ],
});

// Every family, on the green fixture.

test("a clean fixture", async () => {
  assert.deepEqual(await auditEveryFamily(() => {}), CLEAN_RUN);
});

test("a bad state file below a work folder's temp directory", async () => {
  assert.deepEqual(
    await auditEveryFamily(
      put("docs/specs/2026-01/feature-x/temp/state.tsv", BAD_STATE),
    ),
    CLEAN_RUN,
  );
});

test("a flow skill whose name: is followed by a blank line", async () => {
  // Only the standards family reports the null name.
  assert.deepEqual(
    await auditEveryFamily((r) =>
      mutateFile(r, GATE_SKILL, "name: gate\n", "name:\n\n"),
    ),
    {
      failures: [
        {
          rule: "skill-name-format",
          path: GATE_SKILL,
          message:
            'name null is not a string; expected lowercase letters and digits in hyphen-separated groups, at most 64 characters, equal to the directory name "gate"',
        },
      ],
    },
  );
});

test("a required skill that links to the contract file and carries no inline copy", async () => {
  // The rewritten skill also drops the fixture's link to another shared contract, which shared-contracts reports.
  assert.deepEqual(
    await auditEveryFamily(
      put(PROTOTYPE_SKILL, `${skillMD("prototype")}\nSee ${CONTRACT_LINK}.\n`),
    ),
    {
      failures: [
        {
          rule: "shared-contracts",
          path: PROTOTYPE_SKILL,
          message:
            "must link to skills/routing/references/contracts/empirical-questions.md",
        },
      ],
    },
  );
});

test("a spec-axis file holding only the unreadable placeholder", async () => {
  assert.deepEqual(
    await auditEveryFamily(put(SPEC_AXIS, `${NUL}unreadable`)),
    CLEAN_RUN,
  );
});

test("a symlinked state file", NEEDS_SYMLINK, async () => {
  assert.deepEqual(
    await auditEveryFamily(
      all(
        put("bad-state.tsv", BAD_STATE),
        symlinked(
          "docs/specs/2026-01/feature-x/state.tsv",
          "../../../../bad-state.tsv",
          "file",
        ),
      ),
    ),
    CLEAN_RUN,
  );
});

test("challenge links to the contract file and holds neither the marker nor the Collecting answers paragraph", async () => {
  // The rewritten skill also drops the fixture's link to another shared contract, which shared-contracts reports.
  assert.deepEqual(
    await auditEveryFamily(
      put(CHALLENGE, challengeStepThree(`Ask as ${CONTRACT_LINK} says.`)),
    ),
    {
      failures: [
        {
          rule: "shared-contracts",
          path: CHALLENGE,
          message:
            "must link to skills/routing/references/contracts/principles-check.md",
        },
      ],
    },
  );
});

test(
  "docs/specs is a symlink to a tree with a bad state file",
  NEEDS_SYMLINK,
  async () => {
    assert.deepEqual(
      await auditEveryFamily(
        all(
          put("specs-target/2026-01/feature-x/state.tsv", BAD_STATE),
          symlinked("docs/specs", "../specs-target", "dir"),
        ),
      ),
      CLEAN_RUN,
    );
  },
);

test("the Collecting answers paragraph under a level-4 heading inside Step 3", async () => {
  const mutate: Mutation = (r) =>
    mutateFile(
      r,
      CHALLENGE,
      "**Collecting answers.**",
      "#### Answers\n\n**Collecting answers.**",
    );
  const sharedContracts: Failure[] = [
    ["challenge", "principles-check"],
    ["decompose", "workflow-steps"],
    ["decompose", "principles-check"],
    ["decompose", "empirical-questions"],
    ["execute", "workflow-steps"],
    ["execute", "principles-check"],
    ["gate", "workflow-steps"],
    ["gate", "principles-check"],
    ["prototype", "empirical-questions"],
    ["review", "workflow-steps"],
    ["routing", "workflow-steps"],
    ["routing", "empirical-questions"],
  ].map(([skill, contract]) => ({
    rule: "shared-contracts",
    path: `skills/${skill}/SKILL.md`,
    message: `must link to skills/routing/references/contracts/${contract}.md`,
  }));
  assert.deepEqual(await auditEveryFamily(mutate, { pastedFormat: true }), {
    failures: [...QUESTION_FORMAT_FINDINGS, ...sharedContracts],
  });
  assert.deepEqual(await auditSlice(sliceNamed("workflow"), mutate), {
    failures: QUESTION_FORMAT_FINDINGS,
  });
});

// The metadata slice families, on the pasted-format fixture.

test("a CRLF copy of a whole skill", async () => {
  assert.deepEqual(
    await auditSlice(METADATA_SLICE, xText(crlf(skillMD("x")))),
    CLEAN_RUN,
  );
});

test("a document end marker followed by invalid text", async () => {
  // Text after a document end marker is read as a second document, which is refused.
  assert.deepEqual(
    await auditSlice(METADATA_SLICE, xFields(`${REQUIRED}...\n: bad [\n`)),
    xNotYaml(
      "Source contains multiple documents; please use YAML.parseAllDocuments() (block line 5)",
    ),
  );
});

test("a duplicate name with a leading BOM", async () => {
  // A leading byte order mark is not part of the name, so it collides with forge.
  assert.deepEqual(
    await auditSlice(
      METADATA_SLICE,
      xFields(`name: ${BOM}forge\ndescription: d\nversion: "1"\n`),
    ),
    {
      failures: [
        {
          rule: "skill-name-unique",
          path: X_SKILL,
          message:
            'duplicate skill name "forge": skills/forge/SKILL.md and skills/x/SKILL.md',
        },
      ],
    },
  );
});

test("a folded block scalar", async () => {
  assert.deepEqual(
    await auditSlice(
      METADATA_SLICE,
      xFields('name: x\ndescription: >\n  folded\n  text\nversion: "1"\n'),
    ),
    CLEAN_RUN,
  );
});

test("a garbage SKILL.md below .claude", async () => {
  assert.deepEqual(
    await auditSlice(METADATA_SLICE, put(".claude/skills/x/SKILL.md", GARBAGE)),
    CLEAN_RUN,
  );
});

test("a garbage SKILL.md below .git", async () => {
  assert.deepEqual(
    await auditSlice(METADATA_SLICE, put(".git/skills/x/SKILL.md", GARBAGE)),
    CLEAN_RUN,
  );
});

test("a garbage SKILL.md below node_modules", async () => {
  assert.deepEqual(
    await auditSlice(
      METADATA_SLICE,
      put("node_modules/skills/x/SKILL.md", GARBAGE),
    ),
    CLEAN_RUN,
  );
});

test("a garbage SKILL.md below tests", async () => {
  assert.deepEqual(
    await auditSlice(METADATA_SLICE, put("tests/skills/x/SKILL.md", GARBAGE)),
    CLEAN_RUN,
  );
});

test("a link inside a plain fence", async () => {
  assert.deepEqual(
    await auditSlice(METADATA_SLICE, ref("a.md", "```\n[x](./gone.md)\n```\n")),
    CLEAN_RUN,
  );
});

test("a link inside inline code", async () => {
  assert.deepEqual(
    await auditSlice(METADATA_SLICE, ref("a.md", "`[x](./gone.md)`\n")),
    CLEAN_RUN,
  );
});

test("a link with a title", async () => {
  assert.deepEqual(
    await auditSlice(METADATA_SLICE, ref("a.md", '[x](b.md "title")\n')),
    CLEAN_RUN,
  );
});

test("a linked entry in a references directory", NEEDS_SYMLINK, async () => {
  assert.deepEqual(
    await auditSlice(
      METADATA_SLICE,
      all(
        ref("real.md", "# Real\n"),
        symlinked(`${PROTOTYPE_REFS}/Linked_Entry.md`, "real.md", "file"),
      ),
    ),
    CLEAN_RUN,
  );
});

test("a linked references root", NEEDS_SYMLINK, async () => {
  assert.deepEqual(
    await auditSlice(
      METADATA_SLICE,
      symlinked("skills/prototype/references", "../docs", "dir"),
    ),
    CLEAN_RUN,
  );
});

test("a literal block scalar", async () => {
  assert.deepEqual(
    await auditSlice(
      METADATA_SLICE,
      xFields('name: x\ndescription: |\n  literal\n  text\nversion: "1"\n'),
    ),
    CLEAN_RUN,
  );
});

test("a merge key", async () => {
  assert.deepEqual(
    await auditSlice(
      METADATA_SLICE,
      xFields(`${REQUIRED}base: &a\n  k: v\nother:\n  <<: *a\n`),
    ),
    xNotYaml("anchors are not allowed (block line 5)"),
  );
});

test("a missing extra shared file", async () => {
  assert.deepEqual(
    await auditSlice(METADATA_SLICE, (r) => remove(r, "docs/ARCHITECTURE.md")),
    CLEAN_RUN,
  );
});

test("a multi-line flow sequence", async () => {
  assert.deepEqual(
    await auditSlice(
      METADATA_SLICE,
      xFields(`${REQUIRED}metadata:\n  surfaces: [a,\n    b]\n`),
    ),
    CLEAN_RUN,
  );
});

test("a multi-line plain scalar description", async () => {
  assert.deepEqual(
    await auditSlice(
      METADATA_SLICE,
      xFields('name: x\ndescription: first\n  second\nversion: "1"\n'),
    ),
    CLEAN_RUN,
  );
});

test("a symlinked child directory under skills", NEEDS_SYMLINK, async () => {
  assert.deepEqual(
    await auditSlice(
      METADATA_SLICE,
      symlinked("skills/linkeddir", "docs", "dir"),
    ),
    CLEAN_RUN,
  );
});

test(
  "a symlinked SKILL.md under a directory that is not a skills directory",
  NEEDS_SYMLINK,
  async () => {
    assert.deepEqual(
      await auditSlice(
        METADATA_SLICE,
        symlinked(
          "ordinary/linked/SKILL.md",
          "../../skills/docs/SKILL.md",
          "file",
        ),
      ),
      CLEAN_RUN,
    );
  },
);

test("a symlinked target that exists", NEEDS_SYMLINK, async () => {
  assert.deepEqual(
    await auditSlice(
      METADATA_SLICE,
      all(
        ref("real.md", "# Real\n"),
        ref("a.md", "[x](./ln.md)\n"),
        symlinked(`${PROTOTYPE_REFS}/ln.md`, "real.md", "file"),
      ),
    ),
    CLEAN_RUN,
  );
});

test("a target with a space", async () => {
  assert.deepEqual(
    await auditSlice(METADATA_SLICE, ref("a.md", "[x](space name.md)\n")),
    CLEAN_RUN,
  );
});

test("a target with a vertical tab", async () => {
  assert.deepEqual(
    await auditSlice(METADATA_SLICE, ref("a.md", "[x](a\x0bb.md)\n")),
    CLEAN_RUN,
  );
});

test("a top-level evals tree holding a garbage SKILL.md", async () => {
  assert.deepEqual(
    await auditSlice(METADATA_SLICE, put("evals/a/skills/x/SKILL.md", GARBAGE)),
    CLEAN_RUN,
  );
});

test("Agent, an NBSP and tool", async () => {
  assert.deepEqual(
    await auditSlice(
      METADATA_SLICE,
      put("AGENTS.md", `# Shared\n\nAgent${NBSP}tool\n`),
    ),
    CLEAN_RUN,
  );
});

test("an anchor and an alias", async () => {
  assert.deepEqual(
    await auditSlice(
      METADATA_SLICE,
      xFields(`${REQUIRED}base: &a\n  k: v\nother: *a\n`),
    ),
    xNotYaml("anchors are not allowed (block line 5)"),
  );
});

test("an attribution marker 201 characters wide", async () => {
  assert.deepEqual(
    await auditSlice(
      METADATA_SLICE,
      ref("a.md", `adapted from${EMOJI.repeat(201)}(MIT)\n`),
    ),
    CLEAN_RUN,
  );
});

test("an attribution marker split by a newline", async () => {
  assert.deepEqual(
    await auditSlice(METADATA_SLICE, ref("a.md", "adapted from x\n(MIT)\n")),
    CLEAN_RUN,
  );
});

test("an eval key under a top-level null key", async () => {
  assert.deepEqual(
    await auditSlice(
      METADATA_SLICE,
      xFields(`${REQUIRED}~:\n  eval-status: passing\n`),
    ),
    xNotYaml("mapping keys must be strings (block line 4)"),
  );
});

test("an identifier in a surface file", async () => {
  assert.deepEqual(
    await auditSlice(
      METADATA_SLICE,
      put("docs/surfaces/vscode.md", "run_in_background\n"),
    ),
    CLEAN_RUN,
  );
});

test("an identifier in an uppercase .MD file", async () => {
  assert.deepEqual(
    await auditSlice(
      METADATA_SLICE,
      all(addX, put("skills/x/NOTES.MD", "run_in_background\n")),
    ),
    CLEAN_RUN,
  );
});

test("my_spawn_agent", async () => {
  assert.deepEqual(
    await auditSlice(
      METADATA_SLICE,
      put("AGENTS.md", "# Shared\n\nmy_spawn_agent\n"),
    ),
    CLEAN_RUN,
  );
});

test("the exempt names", async () => {
  assert.deepEqual(
    await auditSlice(
      METADATA_SLICE,
      all(ref("README.md", "# R\n"), ref("LICENSE", "text\n")),
    ),
    CLEAN_RUN,
  );
});

test("the footer heading after a lone CR", async () => {
  assert.deepEqual(
    await auditSlice(METADATA_SLICE, xBody("intro\r## Validation history\n")),
    CLEAN_RUN,
  );
});

test("the footer heading after U+2028", async () => {
  assert.deepEqual(
    await auditSlice(
      METADATA_SLICE,
      xBody(`intro${LINE_SEP}## Validation history\n`),
    ),
    CLEAN_RUN,
  );
});

test("the keys 0x10 and 16", async () => {
  assert.deepEqual(
    await auditSlice(METADATA_SLICE, xFields(`${REQUIRED}0x10: a\n16: b\n`)),
    xNotYaml("Map keys must be unique (block line 5)"),
  );
});

test("the keys true and True", async () => {
  assert.deepEqual(
    await auditSlice(METADATA_SLICE, xFields(`${REQUIRED}true: a\nTrue: b\n`)),
    xNotYaml("Map keys must be unique (block line 5)"),
  );
});

test(
  "the repository root reached through a symlink",
  NEEDS_SYMLINK,
  async () => {
    assert.deepEqual(
      await auditSlice(METADATA_SLICE, (r) => mkdir(r, "skills/empty"), {
        throughLink: true,
      }),
      CLEAN_RUN,
    );
  },
);

test("the required version field on document line 20 with CRLF", async () => {
  assert.deepEqual(
    await auditSlice(METADATA_SLICE, xText(versionAtLine(20, "\r\n"))),
    CLEAN_RUN,
  );
});

test("the required version field on document line 20 with LF", async () => {
  assert.deepEqual(
    await auditSlice(METADATA_SLICE, xText(versionAtLine(20, "\n"))),
    CLEAN_RUN,
  );
});

test("x/fleet", async () => {
  assert.deepEqual(
    await auditSlice(
      METADATA_SLICE,
      put("AGENTS.md", "# Shared\n\nrun x/fleet now\n"),
    ),
    CLEAN_RUN,
  );
});

// The content slice families, on the pasted-format fixture.

test("a checkmarx row below the routing table end marker", async () => {
  assert.deepEqual(
    await auditSlice(CONTENT_SLICE, (r) =>
      mutateFile(
        r,
        `${TRIAGE_REFS}/source-detection.md`,
        "**Marker-keyed sources bypass this table.**\n",
        "**Marker-keyed sources bypass this table.**\n\n| checkmarx | Code Security |\n",
      ),
    ),
    CLEAN_RUN,
  );
});

test("a co-author trailer naming a human", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      ref("origin-note.md", "Co-authored-by: Ada Lovelace\n"),
    ),
    CLEAN_RUN,
  );
});

test("a co-author trailer with an AI identity 81 bytes after it", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      ref("origin-note.md", `Co-authored-by: ${E_ACUTE.repeat(38)} bot\n`),
    ),
    CLEAN_RUN,
  );
});

test("a file holding a NUL byte and a hook path", async () => {
  assert.deepEqual(
    await auditSlice(CONTENT_SLICE, ref("bin.md", `${NUL}\nSee hooks/x.md\n`)),
    CLEAN_RUN,
  );
});

test("a footer behind a doubled quote prefix", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      triageRef("notes.md", ">> _Triaged by an AI agent_\n"),
    ),
    CLEAN_RUN,
  );
});

test("a markered fence closed by a four-backtick line", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      triageRef(
        "notes.md",
        `# Notes\n\n${FENCE}text\n<!-- agentics:triage:reply:x -->\n\nBody.\n\n${FOOTER}\n${FENCE4}\n`,
      ),
    ),
    CLEAN_RUN,
  );
});

test("a markered markdown fence ending in a footer", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      triageRef(
        "notes.md",
        `# Notes\n\n${FENCE}markdown\n<!-- agentics:triage:reply:x -->\n\nBody.\n\n${FOOTER}\n${FENCE}\n`,
      ),
    ),
    CLEAN_RUN,
  );
});

test("a metadata value that is a list", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      xText(
        '---\nname: x\ndescription: d\nversion: "1"\nmetadata:\n  - artifact\n---\n',
      ),
    ),
    CLEAN_RUN,
  );
});

test("a new source directory with an emitter that carries the footer", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      triageRef(
        "pull-requests/newbot/newbot-triage.md",
        `# Emitter\n\nEvery posted body closes with ${FOOTER}\n`,
      ),
    ),
    CLEAN_RUN,
  );
});

test("a non-string key under metadata on a non-owner", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      skillWithMetadata("x", "    1: a\n    artifact: x.md\n"),
    ),
    CLEAN_RUN,
  );
});

test("a relative path reaching the root", async () => {
  assert.deepEqual(
    await auditSlice(CONTENT_SLICE, ref("a.md", "See ../../notes.md here.\n")),
    CLEAN_RUN,
  );
});

test("a retired name in a file holding a NUL byte", async () => {
  assert.deepEqual(
    await auditSlice(CONTENT_SLICE, ref("bin.md", `${NUL}\nevents.tsv\n`)),
    CLEAN_RUN,
  );
});

test("a retired name outside the skills tree", async () => {
  assert.deepEqual(
    await auditSlice(CONTENT_SLICE, put("docs/guide/state.md", "events.tsv\n")),
    CLEAN_RUN,
  );
});

test("a retired name with a word character before it", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      ref("a.md", "myevents.tsv and events.tsvx and log-rows\n"),
    ),
    CLEAN_RUN,
  );
});

test("a retired reference before the happy heading of a bank file", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      put(
        "evals/skills/bank.md",
        "Old `/ideation` runs.\n\n## happy\n\nFine.\n",
      ),
    ),
    CLEAN_RUN,
  );
});

test("a section fence without the footer", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      triageRef(
        "notes.md",
        `# Notes\n\n${FENCE}text\n<!-- agentics:triage:section:x -->\n\nBody.\n${FENCE}\n`,
      ),
    ),
    CLEAN_RUN,
  );
});

test(
  "a symlink to a legacy disclaimer is not walked",
  NEEDS_SYMLINK,
  async () => {
    assert.deepEqual(
      await auditSlice(
        CONTENT_SLICE,
        all(
          put("legacy-target.md", "This was generated by AI during triage\n"),
          symlinked(
            `${TRIAGE_REFS}/legacy-link.md`,
            "../../../legacy-target.md",
            "file",
          ),
        ),
      ),
      CLEAN_RUN,
    );
  },
);

test(
  "a symlinked file naming a hook path is not read",
  NEEDS_SYMLINK,
  async () => {
    assert.deepEqual(
      await auditSlice(
        CONTENT_SLICE,
        all(
          put("hook-target.md", "See hooks/x.md\n"),
          symlinked(
            `${PROTOTYPE_REFS}/ln.md`,
            "../../../hook-target.md",
            "file",
          ),
        ),
      ),
      CLEAN_RUN,
    );
  },
);

test(
  "a symlinked file naming a retired name is not read",
  NEEDS_SYMLINK,
  async () => {
    assert.deepEqual(
      await auditSlice(
        CONTENT_SLICE,
        all(
          put("legacy-state.md", "events.tsv\n"),
          symlinked(
            `${PROTOTYPE_REFS}/ln.md`,
            "../../../legacy-state.md",
            "file",
          ),
        ),
      ),
      CLEAN_RUN,
    );
  },
);

test(
  "a symlinked file naming a trailer is not read",
  NEEDS_SYMLINK,
  async () => {
    assert.deepEqual(
      await auditSlice(
        CONTENT_SLICE,
        all(
          put("trailer-target.md", "Assisted-by: Claude\n"),
          symlinked(
            `${PROTOTYPE_REFS}/ln.md`,
            "../../../trailer-target.md",
            "file",
          ),
        ),
      ),
      CLEAN_RUN,
    );
  },
);

test("a symlinked guide directory is not walked", NEEDS_SYMLINK, async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      all(
        put("guide-target/overview.md", "Run `/ideation` first.\n"),
        (r) => remove(r, "docs/guide"),
        symlinked("docs/guide", "../guide-target", "dir"),
      ),
    ),
    CLEAN_RUN,
  );
});

test(
  "a symlinked source directory is not a source",
  NEEDS_SYMLINK,
  async () => {
    assert.deepEqual(
      await auditSlice(
        CONTENT_SLICE,
        symlinked(`${TRIAGE_REFS}/pull-requests/linkedbot`, "copilot", "dir"),
      ),
      CLEAN_RUN,
    );
  },
);

test("a token after an ASCII letter", async () => {
  assert.deepEqual(
    await auditSlice(CONTENT_SLICE, ref("a.md", "ahooks/x.md\n")),
    CLEAN_RUN,
  );
});

test("a trailer outside every scanned root", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      put("docs/notes.md", "Assisted-by: Claude\n"),
    ),
    CLEAN_RUN,
  );
});

test("a URL naming a skill-external path", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      ref("a.md", "See https://example.invalid/hooks/x.md.\n"),
    ),
    CLEAN_RUN,
  );
});

test("an AI identity 81 bytes before a co-author trailer", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      ref("origin-note.md", `bot ${E_ACUTE.repeat(38)} Co-authored-by: x\n`),
    ),
    CLEAN_RUN,
  );
});

test("an artifact key at the top level instead of under metadata", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      xText(
        '---\nname: x\ndescription: d\nversion: "1"\nartifact: x.md\n---\n',
      ),
    ),
    CLEAN_RUN,
  );
});

test("Keychain state", async () => {
  assert.deepEqual(
    await auditSlice(CONTENT_SLICE, ref("a.md", "Keychain state is fine.\n")),
    CLEAN_RUN,
  );
});

test("sprint planning in ordinary prose", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      put("docs/guide/sprint.md", "Run a sprint planning meeting.\n"),
    ),
    CLEAN_RUN,
  );
});

test("the skills index naming a hook path", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      put("skills/README.md", "See hooks/x.md\n"),
    ),
    CLEAN_RUN,
  );
});

test("the skills index naming a retired name", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      put("skills/README.md", "Rows go to events.tsv.\n"),
    ),
    CLEAN_RUN,
  );
});

// --- the audit-error scenarios -------------------------------------------------------------------------------------

/** The test options of a scenario that needs a directory the audit cannot read: skipped where a test cannot make one. */
const NEEDS_UNREADABLE = {
  skip: makesUnreadableDirs()
    ? false
    : "this host lets the test read a directory it made unreadable (Windows, or running as root)",
};

/**
 * Asserts that `audit` stopped with the audit error `error`, after reporting exactly `failures` from the families that
 * ran before the one that raised it.
 */
async function assertAuditError(
  audit: Promise<AuditResult>,
  error: string,
  failures: Failure[] = [],
): Promise<void> {
  const result = await audit;
  assert.deepEqual(result, { failures, error });
}

/**
 * The question-format findings of the pasted-format fixture after the question-format breaker: the breaker drops the challenge
 * skill's Collecting answers paragraph, and every skill that pastes the decision-question format is told to link to it.
 */
const QUESTION_FORMAT_FINDINGS: Failure[] = [
  "challenge",
  "docs",
  "execute",
  "forge",
  "gate",
  "gh",
  "prototype",
  "review",
  "triage",
].map((name) => ({
  rule: "question-format",
  path: `skills/${name}/SKILL.md`,
  message:
    "replace the local decision-question format with a link to skills/routing/references/contracts/question-format.md",
}));

/** The ai-attribution-trailer finding its breaker seeds, which the families before eval-check report. */
const TRAILER_FINDING: Failure = {
  rule: "ai-attribution-trailer",
  path: "skills/prototype/references/commit-notes.md",
  message:
    "names an AI attribution trailer outside the prohibition in skills/git; commits Agentics writes carry no AI attribution",
};

/**
 * Asserts that every family, after the ai-attribution-trailer breaker and with the evals stub printing `stream`, keeps
 * that finding and stops with `reason`. The stub writes nothing to stderr, so no stderr tail follows the reason.
 */
async function assertStreamError(
  stream: string,
  reason: string,
): Promise<void> {
  await assertAuditError(
    auditEveryFamily(breaker("ai-attribution-trailer"), { stream }),
    reason,
    [TRAILER_FINDING],
  );
}

// Every family, on the green fixture.

test(
  "AGENTS.md is a symlink next to an empty skill directory",
  NEEDS_SYMLINK,
  async () => {
    await assertAuditError(
      auditEveryFamily(
        all(
          (r) => remove(r, "AGENTS.md"),
          symlinked("AGENTS.md", "README.md", "file"),
          EMPTY_SKILL,
        ),
      ),
      "skills: checking AGENTS.md: AGENTS.md: passes through a link",
      [EMPTY_SKILL_FINDING],
    );
  },
);

test(
  "a directory symlink below skills/execute after a question-format failure and a missing plan template",
  NEEDS_SYMLINK,
  async () => {
    await assertAuditError(
      auditEveryFamily(
        all(
          breaker("question-format"),
          (r) => remove(r, PLAN_TEMPLATE),
          symlinked("skills/execute/references/linked", "../../gh", "dir"),
        ),
        { pastedFormat: true },
      ),
      "skills: scanning skills/execute: skills/execute/references/linked: passes through a link",
      QUESTION_FORMAT_FINDINGS,
    );
  },
);

test("an evals stream with no output at all", async () => {
  await assertStreamError("", NO_CLOSING);
});

test("an evals stream with a line of garbage", async () => {
  await assertStreamError(
    "garbage\n",
    `${NOT_VALID}line 1 is not a JSON object (run: evals check exited with code 0)`,
  );
});

test("an evals stream with failure lines and no closing line", async () => {
  await assertStreamError(`${F}\n${F}\n`, NO_CLOSING);
});

test("an evals stream with a count that does not match the failure lines", async () => {
  await assertStreamError(
    `${F}\n${closing(2)}\n`,
    "skills: evals check closing line does not match the 1 failure lines read (run: evals check exited with code 0)",
  );
});

test("an evals stream with a value after the closing line", async () => {
  await assertStreamError(
    `${D0}\n${F}\n`,
    "skills: evals check printed a line after its closing line (run: evals check exited with code 0)",
  );
});

test("an evals stream with an error line", async () => {
  await assertStreamError(
    '{"error":"nope"}\n',
    "skills: evals check could not audit: nope (run: evals check exited with code 0)",
  );
});

test("an evals stream with an error line carrying other fields", async () => {
  await assertStreamError(
    '{"error":"nope","done":true}\n',
    "skills: evals check error line carries other fields (run: evals check exited with code 0)",
  );
});

test("an evals stream with done false", async () => {
  await assertStreamError(
    '{"done":false,"failures":0}\n',
    "skills: evals check closing line says done is false (run: evals check exited with code 0)",
  );
});

test("an evals stream with a closing line with no count", async () => {
  await assertStreamError(
    '{"done":true}\n',
    "skills: evals check closing line has no failure count (run: evals check exited with code 0)",
  );
});

test("an evals stream with a failure with no path", async () => {
  await assertStreamError(
    `{"rule":"eval-stub-rule","message":"m"}\n${D1}\n`,
    "skills: evals check failure line is missing its path or message (run: evals check exited with code 0)",
  );
});

test("an evals stream with a failure with no rule", async () => {
  await assertStreamError(
    `{"path":"p.md","message":"m"}\n${D1}\n`,
    "skills: evals check printed a failure line with no rule (run: evals check exited with code 0)",
  );
});

test("an evals stream with an empty object", async () => {
  await assertStreamError(
    "{}\n",
    "skills: evals check printed an empty object (run: evals check exited with code 0)",
  );
});

test("an evals stream with an unknown field", async () => {
  await assertStreamError(
    `{"rule":"eval-stub-rule","path":"p.md","message":"m","extra":1}\n${D1}\n`,
    `${NOT_VALID}line 1 has an unknown field "extra" (run: evals check exited with code 0)`,
  );
});

test("an evals stream with a top-level null", async () => {
  await assertStreamError(
    "null\n",
    `${NOT_VALID}line 1 is not a JSON object (run: evals check exited with code 0)`,
  );
});

test("an evals stream with a top-level array", async () => {
  await assertStreamError(
    "[]\n",
    `${NOT_VALID}line 1 is not a JSON object (run: evals check exited with code 0)`,
  );
});

test("an evals stream with a count of 1.0", async () => {
  await assertStreamError(
    `${F}\n{"done":true,"failures":1.0}\n`,
    `${NOT_VALID}line 2: field failures holds 1.0, which is not a safe integer (run: evals check exited with code 0)`,
  );
});

test("an unreadable directory below docs/specs", NEEDS_UNREADABLE, async () => {
  const locked = (r: string): string => full(r, "docs/specs/locked");
  await assertAuditError(
    auditEveryFamily(
      all(put("docs/specs/locked/state.tsv", BAD_STATE), (r) =>
        chmodSync(locked(r), 0o000),
      ),
      { restore: (r) => chmodSync(locked(r), 0o755) },
    ),
    "docs/specs/locked: EACCES",
  );
});

test("axes-mode.md is a directory after a question-format failure and a missing spec-axis file", async () => {
  await assertAuditError(
    auditEveryFamily(
      all(
        breaker("question-format"),
        (r) => remove(r, SPEC_AXIS),
        asDirectory(AXES_MODE),
      ),
      { pastedFormat: true },
    ),
    "cannot read skills/review/references/axes/axes-mode.md, which carries the spec-pass contract: skills/review/references/axes/axes-mode.md: is not a regular file",
    [
      // reference-naming reads the directory's .md name as a badly named reference directory.
      {
        rule: "reference-naming",
        path: AXES_MODE,
        message:
          "reference directory name is not lowercase kebab-case — rename with 'git mv'",
      },
      ...QUESTION_FORMAT_FINDINGS,
    ],
  );
});

test("every references directory is removed", async () => {
  await assertAuditError(
    auditEveryFamily(
      all(
        ...[
          "triage",
          "forge",
          "gate",
          "review",
          "execute",
          "decompose",
          "gh",
          "routing",
        ].map(
          (name): Mutation =>
            (r) =>
              remove(r, `skills/${name}/references`),
        ),
      ),
      { pastedFormat: true },
    ),
    "skills: no reference files found — the reference-naming glob is wrong",
    [
      ...[
        "skills/routing/references/copilot-tools.md",
        "skills/routing/references/surface-dispatch.md",
        "skills/routing/references/model-ladders.md",
        "skills/routing/references/model-ladders-examples.md",
      ].map((at) => ({
        rule: "orchestration-file-present",
        path: at,
        message: "exempted in the contract arrays but does not exist",
      })),
      ...["decompose", "execute", "gate", "triage"].map((name) => ({
        rule: "reference-link",
        path: `skills/${name}/SKILL.md`,
        message: "dangling link -> ../execute/references/flow-state.md",
      })),
    ],
  );
});

test(
  "references/checks is a symlink after a skill-frontmatter failure",
  NEEDS_SYMLINK,
  async () => {
    await assertAuditError(
      auditEveryFamily(
        all(
          breaker("skill-frontmatter"),
          triageLink(`${TRIAGE_REFS}/checks`, "pull-requests", "dir"),
        ),
      ),
      CHECKS_LINK_REASON,
      [
        {
          rule: "skill-frontmatter",
          path: "skills/docs/SKILL.md",
          message:
            "missing required frontmatter (name/description/version) in the first 20 lines",
        },
      ],
    );
  },
);

test("skills is removed", async () => {
  await assertAuditError(
    auditEveryFamily((r) => remove(r, "skills")),
    NO_SKILL_ASSETS,
  );
});

test("skills/a/SKILL.md is a symlink", NEEDS_SYMLINK, async () => {
  await assertAuditError(
    auditEveryFamily(
      symlinked("skills/a/SKILL.md", "../docs/SKILL.md", "file"),
    ),
    "skills: checking skills/a/SKILL.md: skills/a/SKILL.md: passes through a link",
  );
});

test("skills/dirskill/SKILL.md is a directory", async () => {
  await assertAuditError(
    auditEveryFamily((r) => mkdir(r, "skills/dirskill/SKILL.md")),
    DIR_SKILL_REASON,
  );
});

test("skills/gh/references/stack.md is a symlink", NEEDS_SYMLINK, async () => {
  await assertAuditError(
    auditEveryFamily(
      all(
        (r) =>
          renameSync(
            full(r, "skills/gh/references/stack.md"),
            full(r, "stack-copy.md"),
          ),
        symlinked(
          "skills/gh/references/stack.md",
          "../../../stack-copy.md",
          "file",
        ),
      ),
    ),
    "skills: reading skills/gh/references/stack.md: skills/gh/references/stack.md: passes through a link",
  );
});

test("the challenge skill file is a symlink", NEEDS_SYMLINK, async () => {
  await assertAuditError(
    auditEveryFamily(
      all(
        (r) => remove(r, CHALLENGE),
        symlinked(CHALLENGE, "../docs/SKILL.md", "file"),
      ),
    ),
    "skills: checking skills/challenge/SKILL.md: skills/challenge/SKILL.md: passes through a link",
  );
});

test("the contract file holds no canonical question block", async () => {
  await assertAuditError(
    auditEveryFamily(
      put(
        QUESTION_CONTRACT,
        "# Question format\n\n## Collecting answers\n\nCollect the answers.\n",
      ),
    ),
    'skills: skills/routing/references/contracts/question-format.md: no fenced block found after "Format each question exactly like so:"',
  );
});

test("the contract file is a symlink", NEEDS_SYMLINK, async () => {
  // The link points at a regular copy outside skills/, so the link itself is the only thing wrong with the path.
  await assertAuditError(
    auditEveryFamily(
      all(
        (r) => remove(r, QUESTION_CONTRACT),
        put("question-format-copy.md", contractFixtureMD()),
        symlinked(
          QUESTION_CONTRACT,
          "../../../../question-format-copy.md",
          "file",
        ),
      ),
    ),
    "skills: reading skills/routing/references/contracts/question-format.md: skills/routing/references/contracts/question-format.md: passes through a link",
  );
});

test("the contract file is missing", async () => {
  await assertAuditError(
    auditEveryFamily((r) => remove(r, QUESTION_CONTRACT), {
      pastedFormat: true,
    }),
    "skills: reading skills/routing/references/contracts/question-format.md: skills/routing/references/contracts/question-format.md: does not exist",
  );
});

test("the repository sits below a directory named with an unclosed [", async () => {
  await assertAuditError(
    auditEveryFamily(() => {}, { placeAt: "a[b/repo" }),
    "syntax error in pattern",
  );
});

test("the skills directory holds no skill", async () => {
  await assertAuditError(
    auditEveryFamily(
      all(
        (r) => remove(r, "skills"),
        (r) => mkdir(r, "skills"),
      ),
    ),
    NO_SKILL_ASSETS,
  );
});

test("the spec-axis file is a symlink", NEEDS_SYMLINK, async () => {
  await assertAuditError(
    auditEveryFamily(
      all(
        (r) => renameSync(full(r, SPEC_AXIS), full(r, "spec-axis-copy.md")),
        symlinked(SPEC_AXIS, "../../../../spec-axis-copy.md", "file"),
      ),
    ),
    "cannot read skills/review/references/axes/spec-axis.md, which carries the spec-pass contract: skills/review/references/axes/spec-axis.md: passes through a link",
  );
});

test("the work-folder gate file is a symlink", NEEDS_SYMLINK, async () => {
  await assertAuditError(
    auditEveryFamily(
      all(
        (r) => remove(r, WORK_FOLDER_GATE),
        symlinked(
          WORK_FOLDER_GATE,
          "../../../decompose/references/chain-artifacts.md",
          "file",
        ),
      ),
    ),
    "skills: reading skills/gate/references/branch/tidy.md: skills/gate/references/branch/tidy.md: passes through a link",
  );
});

// The metadata slice families, on the pasted-format fixture.

test("THIRD_PARTY_NOTICES.md is a directory next to an empty skill directory", async () => {
  await assertAuditError(
    auditSlice(
      METADATA_SLICE,
      all(asDirectory("THIRD_PARTY_NOTICES.md"), EMPTY_SKILL),
    ),
    "skills: reading THIRD_PARTY_NOTICES.md: THIRD_PARTY_NOTICES.md: is not a regular file",
    [EMPTY_SKILL_FINDING],
  );
});

test(
  "a dangling link and a symlinked notices file next to an empty skill directory",
  NEEDS_SYMLINK,
  async () => {
    await assertAuditError(
      auditSlice(
        METADATA_SLICE,
        all(
          ref("a.md", "[x](./gone.md)\n"),
          (r) => remove(r, "THIRD_PARTY_NOTICES.md"),
          symlinked("THIRD_PARTY_NOTICES.md", "README.md", "file"),
          EMPTY_SKILL,
        ),
      ),
      "skills: reading THIRD_PARTY_NOTICES.md: THIRD_PARTY_NOTICES.md: passes through a link",
      [EMPTY_SKILL_FINDING],
    );
  },
);

test("an adapter is a dangling symlink", NEEDS_SYMLINK, async () => {
  await assertAuditError(
    auditSlice(
      METADATA_SLICE,
      all(
        (r) => remove(r, "skills/routing/references/model-ladders.md"),
        symlinked(
          "skills/routing/references/model-ladders.md",
          "nowhere.md",
          "file",
        ),
      ),
    ),
    "skills: checking skills/routing/references/model-ladders.md: skills/routing/references/model-ladders.md: passes through a link",
  );
});

test("skills is a symlink", NEEDS_SYMLINK, async () => {
  await assertAuditError(
    auditSlice(
      METADATA_SLICE,
      all((r) => renameSkills(r), symlinked("skills", "skills-real", "dir")),
    ),
    NO_SKILL_ASSETS,
  );
});

test("skills/dirskill/SKILL.md is a directory next to an empty skill directory", async () => {
  await assertAuditError(
    auditSlice(
      METADATA_SLICE,
      all((r) => mkdir(r, "skills/dirskill/SKILL.md"), EMPTY_SKILL),
    ),
    DIR_SKILL_REASON,
    [EMPTY_SKILL_FINDING],
  );
});

// The content slice families, on the pasted-format fixture.

test("every file under skills/ holds a NUL byte", async () => {
  await assertAuditError(
    auditSlice(CONTENT_SLICE, nulInEverySkillsFile),
    "skills: no files under skills/ to audit for repository independence",
  );
});

test("references/checks is a symlink", NEEDS_SYMLINK, async () => {
  await assertAuditError(
    auditSlice(
      CONTENT_SLICE,
      triageLink(`${TRIAGE_REFS}/checks`, "pull-requests", "dir"),
    ),
    CHECKS_LINK_REASON,
  );
});

test("references/checks is removed", async () => {
  await assertAuditError(
    auditSlice(CONTENT_SLICE, (r) => remove(r, `${TRIAGE_REFS}/checks`)),
    "skills: reading skills/triage/references/checks: skills/triage/references/checks: does not exist",
  );
});

test("references/issue-mode.md is a directory", async () => {
  await assertAuditError(
    auditSlice(CONTENT_SLICE, asDirectory(`${TRIAGE_REFS}/issue-mode.md`)),
    "skills/triage/references/issue-mode.md: is not a regular file",
  );
});

test("references/issue-mode.md is a symlink", NEEDS_SYMLINK, async () => {
  await assertAuditError(
    auditSlice(
      CONTENT_SLICE,
      triageLink(`${TRIAGE_REFS}/issue-mode.md`, "source-detection.md", "file"),
    ),
    "skills/triage/references/issue-mode.md: passes through a link",
  );
});

test(
  "references/orchestration-lifecycle.md is a symlink",
  NEEDS_SYMLINK,
  async () => {
    await assertAuditError(
      auditSlice(
        CONTENT_SLICE,
        triageLink(
          `${TRIAGE_REFS}/orchestration-lifecycle.md`,
          "source-detection.md",
          "file",
        ),
      ),
      "skills/triage/references/orchestration-lifecycle.md: passes through a link",
    );
  },
);

test("references/pull-requests is a file", async () => {
  await assertAuditError(
    auditSlice(
      CONTENT_SLICE,
      all(
        (r) => remove(r, `${TRIAGE_REFS}/pull-requests`),
        put(`${TRIAGE_REFS}/pull-requests`, "not a directory\n"),
      ),
    ),
    "skills: reading skills/triage/references/pull-requests: skills/triage/references/pull-requests: is not a directory",
  );
});

test("references/pull-requests is a symlink", NEEDS_SYMLINK, async () => {
  await assertAuditError(
    auditSlice(
      CONTENT_SLICE,
      triageLink(`${TRIAGE_REFS}/pull-requests`, "checks", "dir"),
    ),
    "skills: reading skills/triage/references/pull-requests: skills/triage/references/pull-requests: passes through a link",
  );
});

test("references/pull-requests is removed", async () => {
  await assertAuditError(
    auditSlice(CONTENT_SLICE, (r) => remove(r, `${TRIAGE_REFS}/pull-requests`)),
    "skills: reading skills/triage/references/pull-requests: skills/triage/references/pull-requests: does not exist",
  );
});

test("references/rollup-template.md is a directory", async () => {
  await assertAuditError(
    auditSlice(CONTENT_SLICE, asDirectory(ROLLUP_TEMPLATE)),
    "skills/triage/references/rollup-template.md: is not a regular file",
  );
});

test("references/rollup-template.md is a symlink", NEEDS_SYMLINK, async () => {
  await assertAuditError(
    auditSlice(
      CONTENT_SLICE,
      triageLink(ROLLUP_TEMPLATE, "source-detection.md", "file"),
    ),
    "skills: checking skills/triage/references/rollup-template.md: skills/triage/references/rollup-template.md: passes through a link",
  );
});

test("references/source-detection.md is a symlink", NEEDS_SYMLINK, async () => {
  await assertAuditError(
    auditSlice(
      CONTENT_SLICE,
      triageLink(
        `${TRIAGE_REFS}/source-detection.md`,
        "subagent-contract.md",
        "file",
      ),
    ),
    "skills: checking skills/triage/references/source-detection.md: skills/triage/references/source-detection.md: passes through a link",
  );
});

test(
  "references/subagent-contract.md is a symlink",
  NEEDS_SYMLINK,
  async () => {
    await assertAuditError(
      auditSlice(
        CONTENT_SLICE,
        triageLink(
          `${TRIAGE_REFS}/subagent-contract.md`,
          "source-detection.md",
          "file",
        ),
      ),
      "skills: checking skills/triage/references/subagent-contract.md: skills/triage/references/subagent-contract.md: passes through a link",
    );
  },
);

test(
  "skills/git/references/commit.md is a symlink",
  NEEDS_SYMLINK,
  async () => {
    // No metadata slice family names the commit reference, and the walks that pass it skip a link, so attribution is
    // the first to read it and refuses the link.
    await assertAuditError(
      auditSlice(
        CONTENT_SLICE,
        all(
          gitSkill(`${PROHIBITION}\n`),
          put("commit-copy.md", `${PROHIBITION}\n`),
          symlinked(GIT_COMMIT, "../../../commit-copy.md", "file"),
        ),
      ),
      "skills: reading skills/git/references/commit.md: skills/git/references/commit.md: passes through a link",
    );
  },
);

test("skills/triage is a symlink", NEEDS_SYMLINK, async () => {
  await assertAuditError(
    auditSlice(
      CONTENT_SLICE,
      all(
        (r) => renameSync(full(r, "skills/triage"), full(r, "triage-real")),
        symlinked("skills/triage", "../triage-real", "dir"),
      ),
    ),
    "skills: checking skills/triage/references/rollup-template.md: skills/triage: passes through a link",
  );
});
