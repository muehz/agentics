import {
  errorMessage,
  type AuditResult,
  type Failure,
  type RunOptions,
} from "../../check.ts";
import type { Tracked } from "../repo/walk.ts";
import { skillsTrackedLookup } from "./files.ts";
import {
  EVAL_FRONTMATTER_CLEAN_FAMILY,
  EVAL_METADATA_FAMILY,
  FRONTMATTER_FAMILY,
  INLINE_FAMILY,
} from "./frontmatter.ts";
import { FAMILY as ARTIFACT_FAMILY } from "./artifacts.ts";
import { FAMILY as ATTRIBUTION_FAMILY } from "./attribution.ts";
import { FAMILY as BANK_FIXTURE_FAMILY } from "./bank-fixture-completeness.ts";
import { FAMILY as EVAL_CHECK_FAMILY, runEvalsCheck } from "./eval-check.ts";
import { FAMILY as FILE_SIZE_FAMILY } from "./file-size.ts";
import { FAMILY as FLOW_EMISSION_FAMILY } from "./flow-emission.ts";
import { FAMILY as INBOUND_ANCHOR_FAMILY } from "./inbound-anchor.ts";
import { FAMILY as INDEPENDENCE_FAMILY } from "./independence.ts";
import { FAMILY as LEAN_CADENCE_FAMILY } from "./lean-cadence.ts";
import { FAMILY as LEGACY_NAMES_FAMILY } from "./legacy-names.ts";
import { FAMILY as ORCHESTRATION_FAMILY } from "./orchestration.ts";
import { FAMILY as QUESTION_FORMAT_FAMILY } from "./question-format.ts";
import { FAMILY as RECORD_KEEPING_FAMILY } from "./record-keeping.ts";
import {
  REFERENCE_LINK_FAMILY,
  REFERENCE_NAMING_FAMILY,
} from "./references.ts";
import { FAMILY as RETIRED_SKILL_FAMILY } from "./renames.ts";
import { FAMILY as SHARED_CONTRACTS_FAMILY } from "./shared-contracts.ts";
import { FAMILY as SHELL_DATA_FAMILY } from "./shell-data.ts";
import { FAMILY as SPEC_PASS_FAMILY } from "./spec-pass.ts";
import { STANDARDS_FAMILY } from "./standards.ts";
import { FAMILY as STATE_TSV_FAMILY } from "./state-tsv.ts";
import { FAMILY as TRIAGE_FAMILY } from "./triage.ts";
import { SkillsTreeModel } from "./tree-model.ts";
import type { proseLines } from "./xref-md.ts";
import { FAMILY as WAIVE_OPERANDS_FAMILY } from "./waive-operands.ts";
import { FAMILY as WORKFOLDER_FAMILY } from "./work-folder.ts";

// Every family module imports `Family` with `import type`, which erases at runtime, so the modules that wire the
// families into `FAMILIES` create no import cycle with this one.

/** The runner the last family calls to run the evals check; it returns the findings or throws an audit error. */
export type EvalsCheck = (repoRoot: string) => Failure[] | Promise<Failure[]>;

/** What the caller may replace. Every member is optional. */
export type SkillsContext = {
  /**
   * The families to run, in order. Defaults to `DOMAIN_FAMILIES`; a test that audits only the core families
   * passes `FAMILIES`.
   */
  families?: readonly Family[];
  /**
   * A replacement runner for the last family. Without one, the audit uses `runEvalsCheck`.
   */
  evalsCheck?: EvalsCheck;
  /** Explicit invocation inputs for the default evals runner. Direct callers may use its process defaults. */
  evalsInvocation?: Pick<RunOptions, "env" | "diagnostics">;
  /**
   * The lazy lookup of the files git tracks, which decides what the walk checks below a work folder's `temp`. Defaults
   * to one that runs git with the run's environment on first use.
   */
  tracked?: () => Tracked;
  /** Parser for common Markdown prose, cached for this invocation only. */
  parseLines?: typeof proseLines;
};

/**
 * The domain gives each family the same tree and tracked lookup. Both are optional for direct family callers,
 * which can supply a partial context and let the family construct the missing resource.
 */
export type ResolvedSkillsContext = Required<
  Pick<SkillsContext, "families" | "evalsCheck">
> &
  Pick<SkillsContext, "tracked"> & { tree?: SkillsTreeModel };

/** One family of rules, run as a unit. */
export type Family = {
  /** The family's name, which the report and the family filter use. */
  name: string;
  /** The rule ids the family can report. */
  rules: readonly string[];
  /** The family's failures, or a throw for an audit error. */
  run(
    root: string,
    context: ResolvedSkillsContext,
  ): Failure[] | Promise<Failure[]>;
};

/**
 * The core families, in audit order: the report prints failures in this order. `eval-check` runs last because its
 * subprocess can fail for environmental reasons and the run stops at the first family that fails to run, so an earlier
 * position would drop every later family from the report. The question-format, standards, shell-data,
 * shared-contracts, waive-operands and record-keeping families are not here; `DOMAIN_FAMILIES` adds them.
 */
export const FAMILIES: readonly Family[] = [
  INLINE_FAMILY,
  FRONTMATTER_FAMILY,
  EVAL_METADATA_FAMILY,
  EVAL_FRONTMATTER_CLEAN_FAMILY,
  ORCHESTRATION_FAMILY,
  REFERENCE_LINK_FAMILY,
  REFERENCE_NAMING_FAMILY,
  TRIAGE_FAMILY,
  ARTIFACT_FAMILY,
  RETIRED_SKILL_FAMILY,
  ATTRIBUTION_FAMILY,
  INDEPENDENCE_FAMILY,
  WORKFOLDER_FAMILY,
  SPEC_PASS_FAMILY,
  LEGACY_NAMES_FAMILY,
  STATE_TSV_FAMILY,
  FLOW_EMISSION_FAMILY,
  LEAN_CADENCE_FAMILY,
  EVAL_CHECK_FAMILY,
];

/**
 * Every family the registered domain runs: `FAMILIES` in their order, with question-format after independence, and
 * standards, shell-data, file size, inbound anchor, shared contracts, waive operands, record keeping, and bank fixture
 * completeness before eval-check.
 */
export const DOMAIN_FAMILIES: readonly Family[] = FAMILIES.flatMap((family) =>
  family === EVAL_CHECK_FAMILY
    ? [
        STANDARDS_FAMILY,
        SHELL_DATA_FAMILY,
        FILE_SIZE_FAMILY,
        INBOUND_ANCHOR_FAMILY,
        SHARED_CONTRACTS_FAMILY,
        WAIVE_OPERANDS_FAMILY,
        RECORD_KEEPING_FAMILY,
        BANK_FIXTURE_FAMILY,
        family,
      ]
    : family === INDEPENDENCE_FAMILY
      ? [family, QUESTION_FORMAT_FAMILY]
      : [family],
);

/**
 * Maps each rule id to the name of the one family that reports it, in the order the families list them. Throws when a
 * rule id appears in two families or twice in one family's `rules`, because a rule with two owners cannot be filtered
 * by family.
 */
export function ruleFamilyOf(
  families: readonly Family[],
): Record<string, string> {
  const map: Record<string, string> = {};
  for (const family of families) {
    for (const rule of family.rules) {
      if (Object.hasOwn(map, rule))
        throw new Error(
          `rule ${rule} is listed by family ${family.name} and by family ${map[rule]}`,
        );
      map[rule] = family.name;
    }
  }
  return map;
}

/** The rule-to-family map of the core `FAMILIES`; the rules of the families `DOMAIN_FAMILIES` adds are not in it. */
export const RULE_FAMILY: Readonly<Record<string, string>> =
  ruleFamilyOf(FAMILIES);

/**
 * Audits the skill layer. The families run in order and their failures are concatenated, never re-sorted. Without
 * `context.families` the run uses `DOMAIN_FAMILIES`, which the registry does. The first family that throws stops the
 * run: the failures of the families before it are returned with `error` set to the thrown message, and the failing
 * family's own failures, even ones attached to the thrown error, are dropped. This differs from `manifests`, where some
 * steps keep their partial failures. Without `context.tracked` the run lists tracked files with `options.env`, and only
 * once a walk reaches a work folder's `temp` directory; the other options are accepted for the domain signature and
 * ignored.
 */
export async function run(
  repoRoot: string,
  options: RunOptions,
  context: SkillsContext = {},
): Promise<AuditResult> {
  const { evalsInvocation } = context;
  const resolved: ResolvedSkillsContext = {
    families: context.families ?? DOMAIN_FAMILIES,
    evalsCheck:
      context.evalsCheck ??
      (evalsInvocation === undefined
        ? runEvalsCheck
        : (root) =>
            runEvalsCheck(
              root,
              evalsInvocation.env,
              undefined,
              evalsInvocation.diagnostics,
            )),
    tracked: context.tracked ?? skillsTrackedLookup(repoRoot, options.env),
    tree: new SkillsTreeModel(repoRoot, context.parseLines),
  };
  const failures: Failure[] = [];
  for (const family of resolved.families) {
    let found: Failure[];
    try {
      found = await family.run(repoRoot, resolved);
    } catch (cause) {
      return { failures, error: errorMessage(cause) };
    }
    failures.push(...found);
  }
  return { failures };
}
