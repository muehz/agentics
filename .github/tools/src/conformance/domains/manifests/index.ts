import {
  errorMessage,
  type AuditResult,
  type Failure,
  type RunOptions,
} from "../../check.ts";
import { readValues } from "../../../release/version.ts";
import { lazyTrackedLookup } from "../../work-folder-temp.ts";
import { trackedPaths } from "../repo/walk.ts";
import { catalogRules, marketplaceSourceRules } from "./catalogs.ts";
import { copilotProviderRules } from "./copilot.ts";
import { assertVersionsReadable, PartialAuditError } from "./files.ts";
import { parityRules } from "./parity.ts";
import {
  claudeMdRule,
  claudeProviderRules,
  noMcpRules,
  pathReferenceRules,
  pluginJsonRules,
  registrationRules,
} from "./plugins.ts";

/** The steps `run` performs, as a test seam: a test replaces one to make a step fail where no file system can. */
export interface Steps {
  readValues: typeof readValues;
  catalogRules: typeof catalogRules;
  pluginJsonRules: typeof pluginJsonRules;
  marketplaceSourceRules: typeof marketplaceSourceRules;
  pathReferenceRules: typeof pathReferenceRules;
  registrationRules: typeof registrationRules;
  claudeProviderRules: typeof claudeProviderRules;
  copilotProviderRules: typeof copilotProviderRules;
  noMcpRules: typeof noMcpRules;
  parityRules: typeof parityRules;
  claudeMdRule: typeof claudeMdRule;
}

export const STEPS: Steps = {
  readValues,
  catalogRules,
  pluginJsonRules,
  marketplaceSourceRules,
  pathReferenceRules,
  registrationRules,
  claudeProviderRules,
  copilotProviderRules,
  noMcpRules,
  parityRules,
  claudeMdRule,
};

/** The failures an audit error carries with it: only a partial-list error has any. */
const partialOf = (cause: unknown): Failure[] =>
  cause instanceof PartialAuditError ? cause.failures : [];

/**
 * Audits the plugin and marketplace manifests. The steps run in a fixed order, and the report prints the failures that
 * survive an audit error before the ERROR line, so which ones survive is fixed too: an error in a catalog or
 * plugin.json step (2, 3), in the MCP rules (6) or in the parity rules (7) drops every failure; an error in the
 * source rule (4) or in the path, registration, Claude or Copilot rules (5) keeps the failures found so far, and the
 * source rule (4) and the path-reference rule each add their own partial list.
 *
 * The plugin.json and MCP walks share one tracked lookup that runs git with `options.env`, at most once and only when
 * a walk reaches a work folder's `temp` directory. A git failure there is an audit error of the step that met it.
 */
export async function run(
  repoRoot: string,
  options: RunOptions,
  steps: Steps = STEPS,
): Promise<AuditResult> {
  const values = steps.readValues(repoRoot);
  try {
    assertVersionsReadable(values);
  } catch (cause) {
    return { failures: [], error: errorMessage(cause) };
  }

  const tracked = lazyTrackedLookup(() => trackedPaths(repoRoot, options.env));
  const failures: Failure[] = [];
  let catalogs: ReturnType<typeof catalogRules>;
  try {
    catalogs = steps.catalogRules(repoRoot);
  } catch (cause) {
    return { failures: [], error: errorMessage(cause) };
  }
  failures.push(...catalogs.failures);

  let plugins: ReturnType<typeof pluginJsonRules>;
  try {
    plugins = steps.pluginJsonRules(repoRoot, tracked);
  } catch (cause) {
    return { failures: [], error: errorMessage(cause) };
  }
  failures.push(...plugins.failures);

  const later: Array<() => Failure[]> = [
    () => steps.marketplaceSourceRules(repoRoot, catalogs.parsed),
    () => steps.pathReferenceRules(repoRoot),
    () => steps.registrationRules(repoRoot),
    () => steps.claudeProviderRules(repoRoot),
    () => steps.copilotProviderRules(repoRoot, values),
  ];
  for (const rule of later) {
    try {
      failures.push(...rule());
    } catch (cause) {
      failures.push(...partialOf(cause));
      return { failures, error: errorMessage(cause) };
    }
  }

  try {
    failures.push(...steps.noMcpRules(repoRoot, plugins.found, tracked));
    failures.push(...steps.parityRules(repoRoot, catalogs.parsed, values));
  } catch (cause) {
    return { failures: [], error: errorMessage(cause) };
  }
  failures.push(...steps.claudeMdRule(repoRoot));
  return { failures };
}
