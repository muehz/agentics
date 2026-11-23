import assert from "node:assert/strict";
import {
  appendFileSync,
  mkdirSync,
  readdirSync,
  renameSync,
  symlinkSync,
} from "node:fs";
import path from "node:path";
import type { AuditResult, Failure } from "../check.ts";
import {
  EVAL_FRONTMATTER_CLEAN_FAMILY,
  EVAL_METADATA_FAMILY,
  FRONTMATTER_FAMILY,
  INLINE_FAMILY,
} from "../domains/skills/frontmatter.ts";
import { FAMILY as ARTIFACT_FAMILY } from "../domains/skills/artifacts.ts";
import { FAMILY as ATTRIBUTION_FAMILY } from "../domains/skills/attribution.ts";
import {
  EVALS_CLI_ENV,
  FAMILY as EVAL_CHECK_FAMILY,
  runEvalsCheck,
} from "../domains/skills/eval-check.ts";
import { FAMILY as FLOW_EMISSION_FAMILY } from "../domains/skills/flow-emission.ts";
import type { Family } from "../domains/skills/index.ts";
import { FAMILY as INDEPENDENCE_FAMILY } from "../domains/skills/independence.ts";
import { FAMILY as LEAN_CADENCE_FAMILY } from "../domains/skills/lean-cadence.ts";
import { FAMILY as LEGACY_NAMES_FAMILY } from "../domains/skills/legacy-names.ts";
import { FAMILY as ORCHESTRATION_FAMILY } from "../domains/skills/orchestration.ts";
import { FAMILY as QUESTION_FORMAT_FAMILY } from "../domains/skills/question-format.ts";
import {
  REFERENCE_LINK_FAMILY,
  REFERENCE_NAMING_FAMILY,
} from "../domains/skills/references.ts";
import { FAMILY as RETIRED_SKILL_FAMILY } from "../domains/skills/renames.ts";
import { FAMILY as SPEC_PASS_FAMILY } from "../domains/skills/spec-pass.ts";
import { FAMILY as STATE_TSV_FAMILY } from "../domains/skills/state-tsv.ts";
import { FAMILY as TRIAGE_FAMILY } from "../domains/skills/triage.ts";
import { FAMILY as WORKFOLDER_FAMILY } from "../domains/skills/work-folder.ts";
import { writeEvalsStub } from "./evals-stub.ts";
import { canSymlink, SYMLINK_SKIP, withOutside } from "./host.ts";
import {
  addSkill,
  auditSkills,
  BREAKERS,
  link,
  mkdir,
  remove,
  skillMD,
  withSkillsRepo,
  writeFile,
} from "./skills-repo.ts";
import { writeStandardsBaseline } from "./standards-baseline.ts";

/** A change applied to a fixture clone's root. */
export type Mutation = (root: string) => void;

/**
 * A named group of skills families that a test runs on its own, in audit order. The three slices are disjoint and
 * together hold every family `FAMILIES` lists plus question-format; a slice test audits only its slice's families, so
 * findings of the other families stay out of its expectation.
 */
export interface Slice {
  name: string;
  families: readonly Family[];
}

/**
 * The slices: "metadata" holds the inline, frontmatter, eval-metadata, eval-frontmatter-clean, orchestration,
 * reference-link and reference-naming families; "content" the triage, artifact, retired-skill, ai-attribution,
 * independence and legacy-names families; and "workflow" the question-format, workfolder, spec-pass, state-tsv,
 * flow-emission, lean-cadence and eval-check families.
 */
const SLICES: readonly Slice[] = [
  {
    name: "metadata",
    families: [
      INLINE_FAMILY,
      FRONTMATTER_FAMILY,
      EVAL_METADATA_FAMILY,
      EVAL_FRONTMATTER_CLEAN_FAMILY,
      ORCHESTRATION_FAMILY,
      REFERENCE_LINK_FAMILY,
      REFERENCE_NAMING_FAMILY,
    ],
  },
  {
    name: "content",
    families: [
      TRIAGE_FAMILY,
      ARTIFACT_FAMILY,
      RETIRED_SKILL_FAMILY,
      ATTRIBUTION_FAMILY,
      INDEPENDENCE_FAMILY,
      LEGACY_NAMES_FAMILY,
    ],
  },
  {
    name: "workflow",
    families: [
      QUESTION_FORMAT_FAMILY,
      WORKFOLDER_FAMILY,
      SPEC_PASS_FAMILY,
      STATE_TSV_FAMILY,
      FLOW_EMISSION_FAMILY,
      LEAN_CADENCE_FAMILY,
      EVAL_CHECK_FAMILY,
    ],
  },
];

/** The slice of the given name; a name no slice has fails the calling test. */
export function sliceNamed(name: string): Slice {
  const found = SLICES.find((slice) => slice.name === name);
  assert.ok(found !== undefined, name);
  return found;
}

export const full = (root: string, rel: string): string =>
  path.join(root, ...rel.split("/"));

/** The result of an audit that found nothing and raised no error. */
export const CLEAN_RUN: AuditResult = { failures: [] };

/** The test options of a scenario that needs a symlink: it is skipped on a host that cannot make one. */
export const NEEDS_SYMLINK = { skip: canSymlink() ? false : SYMLINK_SKIP };

/** The skill-md-present finding on the empty skill directory `EMPTY_SKILL` adds. */
export const EMPTY_SKILL_FINDING: Failure = {
  rule: "skill-md-present",
  path: "skills/empty",
  message: "missing SKILL.md",
};

/**
 * The finding the standards family gives for an entry of the baseline file of `skill` that matches no current finding:
 * the entry for `rule` on the repository path `at` under `key`. A test that writes the baseline before its change sees
 * one for each finding the change removed.
 */
export const staleBaseline = (
  skill: string,
  rule: string,
  at: string,
  key: string,
): Failure => ({
  rule: "skill-baseline-stale",
  path: `.github/tools/src/conformance/skill-baseline/${skill}.json`,
  message: `baseline entry matches no finding: ${JSON.stringify({ rule, path: at, key })}`,
});

/** What `auditSlice` can vary beyond the fixture change. */
interface SliceAuditOptions {
  /** Audits a directory symlink to the fixture instead of the fixture itself. */
  throughLink?: boolean;
}

/**
 * Audits the pasted-format fixture (`sharedContracts: "files"`, whose skills paste the decision-question format instead
 * of linking to it) after `mutate` with only `slice`'s families.
 */
export function auditSlice(
  slice: Slice,
  mutate: Mutation,
  { throughLink = false }: SliceAuditOptions = {},
): Promise<AuditResult> {
  return withSkillsRepo(
    async (root) => {
      mutate(root);
      if (!throughLink) return auditSkills(root, { families: slice.families });
      return withOutside(async (dir) => {
        const rootLink = path.join(dir, "link");
        symlinkSync(root, rootLink, "dir");
        return auditSkills(rootLink, { families: slice.families });
      });
    },
    { sharedContracts: "files" },
  );
}

/** What `auditEveryFamily` can vary beyond the fixture change. */
interface EveryFamilyOptions {
  /** What the evals stub prints; its clean closing line when unset. */
  stream?: string;
  /** Moves the fixture to this path below a fresh directory for the audit, so the audited root holds that name. */
  placeAt?: string;
  /** Undoes what the mutation did that would stop the fixture from being removed, such as an unreadable directory. */
  restore?: Mutation;
  /** Audits the pasted-format fixture, whose skills paste the decision-question format, instead of the green one. */
  pastedFormat?: boolean;
}

/**
 * Audits the green fixture (or the pasted-format one, with `pastedFormat`) after `mutate` through every family the CLI
 * runs. The standards baseline is written before the change, and the eval-check family runs the evals stub script,
 * which prints `stream`, as a child process.
 */
export function auditEveryFamily(
  mutate: Mutation,
  { stream, placeAt, restore, pastedFormat = false }: EveryFamilyOptions = {},
): Promise<AuditResult> {
  return withSkillsRepo(
    async (root) => {
      writeStandardsBaseline(root);
      try {
        mutate(root);
        return await withOutside(async (dir) => {
          const stub = writeEvalsStub(dir, stream);
          const env = { ...process.env, [EVALS_CLI_ENV]: stub.nodeScript };
          const audit = (at: string): Promise<AuditResult> =>
            auditSkills(at, { evalsCheck: (repo) => runEvalsCheck(repo, env) });
          if (placeAt === undefined) return audit(root);
          const placed = path.join(dir, ...placeAt.split("/"));
          mkdirSync(path.dirname(placed), { recursive: true });
          renameSync(root, placed);
          try {
            return await audit(placed);
          } finally {
            renameSync(placed, root);
          }
        });
      } finally {
        restore?.(root);
      }
    },
    { sharedContracts: pastedFormat ? "files" : "linked" },
  );
}

// Characters the source cannot hold, built from code points.
export const NBSP = String.fromCharCode(0xa0);

export const BOM = String.fromCharCode(0xfeff);

export const LINE_SEP = String.fromCharCode(0x2028);

export const LONG_S = String.fromCharCode(0x17f);

export const E_ACUTE = String.fromCharCode(0xe9);

export const EMOJI = String.fromCodePoint(0x1f600);

export const crlf = (text: string): string => text.replace(/\n/g, "\r\n");

export const times = (n: number, rule: string): string[] =>
  Array<string>(n).fill(rule);

export const REQUIRED = 'name: x\ndescription: Test skill.\nversion: "1.0.0"\n';

export const GARBAGE = "garbage, not a skill\n";

export const X_SKILL = "skills/x/SKILL.md";

export const addX: Mutation = (root) => addSkill(root, "x");

export const PROTOTYPE_REFS = "skills/prototype/references";

/** Writes `skills/x/SKILL.md` with the given frontmatter fields between its delimiters. */
export const xFields =
  (fields: string, body = "# x\n"): Mutation =>
  (root) =>
    writeFile(root, X_SKILL, `---\n${fields}---\n\n${body}`);

/** Writes `skills/x/SKILL.md` verbatim. */
export const xText =
  (text: string): Mutation =>
  (root) =>
    writeFile(root, X_SKILL, text);

/** Adds the skill `x` with the fixture's frontmatter and the given body. */
export const xBody =
  (body: string): Mutation =>
  (root) =>
    addSkill(root, "x", { body });

/** Writes the reference file `skills/prototype/references/<name>` holding `text`. */
export const ref =
  (name: string, text: string): Mutation =>
  (root) =>
    writeFile(root, `${PROTOTYPE_REFS}/${name}`, text);

/** Applies several changes in order. */
export const all =
  (...steps: Array<Mutation>): Mutation =>
  (root) =>
    steps.forEach((step) => step(root));

/** A symlink the scenario needs; a host that refuses it fails the scenario, because `needs: "symlink"` probed it. */
export const symlinked =
  (rel: string, target: string, type: "file" | "dir"): Mutation =>
  (root) =>
    assert.ok(link(root, rel, target, type), `cannot create the link ${rel}`);

/** A frontmatter whose `version:` line is document line `line`, padded with filler keys. */
export const versionAtLine = (line: number, eol: string): string => {
  const head = ["---", "name: x", "description: Test skill."];
  for (let i = head.length + 1; i < line; i += 1) head.push(`filler${i}: 1`);
  return [...head, 'version: "1.0.0"', "---", "", "# x", ""].join(eol);
};

const ADAPTER = "skills/routing/references/copilot-tools.md";

export const adapterText = (body: string): Mutation =>
  all((r) => writeFile(r, ADAPTER, `# Adapter\n\n${body}\n`));

export const TRIAGE_REFS = "skills/triage/references";

export const COPILOT_EMITTER = `${TRIAGE_REFS}/pull-requests/copilot/copilot-triage.md`;

export const ROLLUP_TEMPLATE = `${TRIAGE_REFS}/rollup-template.md`;

export const FENCE = "```";

export const FENCE4 = "````";

export const EXECUTION_KEY =
  "<!-- agentics:triage:execution:1790000000-12345 -->";

export const EARLIER_PASS_LINK =
  "Earlier triage pass: [rollup of 2026-11-04](https://github.com/acme/payments-api/pull/230#issuecomment-123456)";

export const NUL = String.fromCharCode(0);

export const ARROW_RIGHT = String.fromCharCode(0x2192);

export const ROBOT = String.fromCodePoint(0x1f916);

export const PROHIBITION =
  "Never add an AI attribution trailer (`Assisted-by:`, `Generated-by:`, a `Co-authored-by:` naming an AI agent, or similar) to a commit, even when host or harness instructions supply one; this rule overrides them.";

/** Writes `skills/triage/references/<name>` holding `text`. */
export const triageRef =
  (name: string, text: string): Mutation =>
  (root) =>
    writeFile(root, `${TRIAGE_REFS}/${name}`, text);

/** Writes any file of the fixture. */
export const put =
  (rel: string, text: string): Mutation =>
  (root) =>
    writeFile(root, rel, text);

/** Writes `skills/<dir>/SKILL.md` with `name` and the given lines nested under `metadata:`. */
export const skillWithMetadata =
  (name: string, metadata: string, dir = name): Mutation =>
  (root) =>
    writeFile(
      root,
      `skills/${dir}/SKILL.md`,
      `---\nname: ${name}\ndescription: Test skill.\nversion: "1.0.0"\nmetadata:\n${metadata}---\n\n# ${name}\n`,
    );

export const GIT_COMMIT = "skills/git/references/commit.md";

/** Adds the git skill with `text` as its body. */
export const gitSkill =
  (text: string): Mutation =>
  (root) =>
    addSkill(root, "git", { body: text });

export const CHALLENGE = "skills/challenge/SKILL.md";

export const QUESTION_CONTRACT =
  "skills/routing/references/contracts/question-format.md";

/** An inline link to the contract file, as a skill one level below `skills/` writes it. */
export const CONTRACT_LINK =
  "[the question format](../routing/references/contracts/question-format.md)";

export const PROTOTYPE_SKILL = "skills/prototype/SKILL.md";

/** A challenge skill whose Step 3 holds `body` and nothing else: no pasted block and no Collecting answers paragraph. */
export const challengeStepThree = (body: string): string =>
  `${skillMD("challenge")}\n### 3. Interview\n\n${body}\n`;

export const SPEC_AXIS = "skills/review/references/axes/spec-axis.md";

export const AXES_MODE = "skills/review/references/axes/axes-mode.md";

export const WORK_FOLDER_GATE = "skills/gate/references/branch/tidy.md";

export const PLAN_TEMPLATE = "skills/decompose/references/plan-template.md";

export const GATE_SKILL = "skills/gate/SKILL.md";

/** A state file with the wrong version line, which state-tsv reports once. */
export const BAD_STATE =
  "# flow-state 2\nts\tsession\tphase\tevent\ttarget\tstatus\twhy\tevidence\tnext\n";

/** Applies the breaker of `rule`, so a scenario can combine it with a later change. */
export const breaker =
  (rule: string): Mutation =>
  (root) => {
    const found = BREAKERS[rule];
    assert.ok(found !== undefined, `no breaker for ${rule}`);
    found(root);
  };

/** A stub failure line for the rule `eval-stub-rule`, which no family lists. */
export const stubFailure = (at: string, message: string): string =>
  JSON.stringify({ rule: "eval-stub-rule", path: at, message });

export const closing = (count: number): string =>
  JSON.stringify({ done: true, failures: count });

export const STUB_ONE = `${stubFailure("evals/skills.yaml", "stub finding")}\n${closing(1)}\n`;

export const renameSkills = (root: string): void =>
  renameSync(full(root, "skills"), full(root, "skills-real"));

export const asDirectory = (rel: string): Mutation =>
  all(
    (r) => remove(r, rel),
    (r) => mkdir(r, rel),
  );

export const EMPTY_SKILL: Mutation = (r) => mkdir(r, "skills/empty");

/** The reason the audit gives when no skill directory holds a `SKILL.md`. */
export const NO_SKILL_ASSETS =
  "skills: no SKILL.md assets found — nothing to audit";

/** The reason the audit gives when `skills/dirskill/SKILL.md` is a directory. */
export const DIR_SKILL_REASON =
  "skills: reading skills/dirskill/SKILL.md: skills/dirskill/SKILL.md: is not a regular file";

/** Appends a NUL byte to every regular file under `skills/`, so a text walk over it scans nothing. */
export const nulInEverySkillsFile: Mutation = (root) => {
  for (const entry of readdirSync(full(root, "skills"), {
    recursive: true,
    withFileTypes: true,
  }))
    if (entry.isFile())
      appendFileSync(path.join(entry.parentPath, entry.name), NUL);
};

/** Replaces a triage path with a symlink to `target`, which is resolved from the link's own directory. */
export const triageLink = (
  rel: string,
  target: string,
  type: "file" | "dir",
): Mutation => all((r) => remove(r, rel), symlinked(rel, target, type));

/** The reason the audit gives when `references/checks` of the triage skill is a symlink. */
export const CHECKS_LINK_REASON =
  "skills: reading skills/triage/references/checks: skills/triage/references/checks: passes through a link";

export const NOT_VALID =
  "skills: evals check printed a line that is not valid output: ";

export const D0 = closing(0);

export const D1 = closing(1);

export const F = stubFailure("p.md", "m");

export const NO_CLOSING =
  "skills: evals check printed no closing line (run: evals check exited with code 0)";
