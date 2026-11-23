// The four areas of the repository, and which go-suite shards, TypeScript suite
// workspaces and race job each area's change feeds. ci-scope.ts decides what
// runs from this table, and ci-summary.ts recomputes what must have run from
// the same table, so the two cannot drift apart.
//
// Hook tests also read skills: the skillcompat sweep runs every command in a
// skill through the hook guard, so a skill change feeds the hooks shard.
// Skill-only changes receive the always-on conformance and skill example checks
// rather than the whole TypeScript suites. Real-tree integration tests run on full
// backstops. The evals area is eval data (banks, the eval record, fixtures),
// which the tools tests read; the eval engine itself lives in muehz/evals and
// Agentics runs none of its tests. The race job needs only a change to the hooks
// themselves: the sweep is deterministic, so a skill change adds no race risk.
//
// A pull request that changes only the Markdown under docs/ or skills/ never
// reaches this table: ci-scope.ts classifies it as asset-only.

import { CI_ASSET_PLUGIN_MANIFEST_PATHS } from "../plugin-manifests.ts";
import { AREA_ROOTS, AREAS, type Area } from "../repository-areas.ts";

const SHARDS = ["hooks"] as const;

const SHARD_INPUTS = {
  hooks: ["hooks", "skills"],
};

const TS_SHARDS = ["ts-tools"] as const;
export const GO_CHECKS = ["tests", "race", "fuzz"] as const;

const TS_SHARD_INPUTS = {
  "ts-tools": ["tools", "hooks", "evals"],
};

const RACE_INPUTS = ["hooks"];

const TOOLS_SOURCE = `${AREA_ROOTS.tools}/src/`;

function areaOf(value: string): Area | null {
  // Maintainer infrastructure affected every area before moving into tools.
  if (value.startsWith(`${TOOLS_SOURCE}scripts/`)) return null;
  // Both inventories decide when CI can narrow verification.
  if (
    value === `${TOOLS_SOURCE}plugin-manifests.ts` ||
    value === `${TOOLS_SOURCE}repository-areas.ts`
  )
    return null;
  if (value.startsWith("tools/")) return "tools";
  return AREAS.find((area) => value.startsWith(`${AREA_ROOTS[area]}/`)) ?? null;
}

function planFor(areas: readonly string[]) {
  const changed = new Set(areas);
  return {
    shards: SHARDS.filter((shard) =>
      SHARD_INPUTS[shard].some((area) => changed.has(area)),
    ),
    tsShards: TS_SHARDS.filter((shard) =>
      TS_SHARD_INPUTS[shard].some((area) => changed.has(area)),
    ),
    race: RACE_INPUTS.some((area) => changed.has(area)),
  };
}

const ROOT_PROSE = new Set([
  "README.md",
  "AGENTS.md",
  "CONTRIBUTING.md",
  "SECURITY.md",
  "THIRD_PARTY_NOTICES.md",
]);
const MANIFESTS = new Set(CI_ASSET_PLUGIN_MANIFEST_PATHS);
export function assetPath(value: string): boolean {
  return (
    ROOT_PROSE.has(value) ||
    MANIFESTS.has(value) ||
    /^(?:docs|skills)\/.+\.md$/.test(value)
  );
}

const FULL_REASONS = [
  "merge-input-mismatch",
  "incomplete-history",
  "unknown-path",
  "other-path",
  "empty-diff",
  "non-regular-mode",
  "unsupported-status",
  "malformed-diff",
  "base-not-ancestor",
] as const;
const BACKSTOP_REASONS = [
  "event-full",
  "cli-input",
  "event-input",
  "git-error",
  "output-write",
] as const;
export type FullReason =
  (typeof FULL_REASONS)[number] | (typeof BACKSTOP_REASONS)[number];
type Mode = "full" | "area-scoped" | "asset-only";
type Reason = FullReason | "eligible" | "area-scoped";
export interface Identity {
  base: string;
  head: string;
  merge: string;
}
export interface Selection extends Identity {
  mode: Mode;
  complete: boolean;
  reason: Reason;
  areas: string;
  shards: readonly string[];
  tsShards: readonly string[];
  race: boolean;
}

export function fullSelection(
  reason: FullReason,
  identity: Identity = { base: "", head: "", merge: "" },
): Selection {
  return {
    mode: "full",
    complete: false,
    base: identity.base,
    head: identity.head,
    merge: identity.merge,
    reason,
    areas: "",
    shards: [...SHARDS],
    tsShards: [...TS_SHARDS],
    race: true,
  };
}

function reducedSelection(identity: Identity, areas: string[]): Selection {
  const mode = areas.length ? "area-scoped" : "asset-only";
  return {
    mode,
    complete: true,
    base: identity.base,
    head: identity.head,
    merge: identity.merge,
    reason: areas.length ? "area-scoped" : "eligible",
    areas: areas.join(","),
    ...planFor(areas),
  };
}

// The caller checks each immutable diff's record shape, paths and history before
// combining the changed paths. A rename or copy contributes both sides.
export function selectPaths(
  identity: Identity,
  paths: readonly string[],
): Selection {
  if (!paths.length) return fullSelection("empty-diff", identity);
  const areas = new Set<string>();
  let assetsOnly = true;
  for (const path of paths) {
    const asset = assetPath(path);
    const area = areaOf(path);
    if (!area && !asset) return fullSelection("other-path", identity);
    if (area) areas.add(area);
    if (!asset) assetsOnly = false;
  }
  return reducedSelection(identity, assetsOnly ? [] : [...areas].sort());
}

function selectionOutputs(selection: Selection) {
  return {
    mode: selection.mode,
    complete: String(selection.complete),
    base: selection.base,
    head: selection.head,
    merge: selection.merge,
    reason: selection.reason,
    areas: selection.areas,
    shards: JSON.stringify(selection.shards),
    ts_shards: JSON.stringify(selection.tsShards),
    race: String(selection.race),
  };
}
export function encodeSelection(selection: Selection): string {
  return Object.entries(selectionOutputs(selection))
    .map(([key, value]) => `${key}=${value}\n`)
    .join("");
}

function selectionProof(selection: Selection): string {
  // Every canonical output is ASCII and excludes '|'. The separator keeps
  // absent values in their own positions when the workflow rebuilds the proof.
  return Object.values(selectionOutputs(selection)).join("|");
}

const EVENTS = new Set([
  "pull_request",
  "push",
  "schedule",
  "workflow_dispatch",
]);

export function isSelectionEvent(event: string | undefined): event is string {
  return event !== undefined && EVENTS.has(event);
}

export const SUMMARY_JOBS = {
  scope: ["Setup / Classify changes", "scope"],
  "pr-metadata": ["PR / Title and description", "pr-only"],
  "pr-version-fields": ["PR / Version fields", "pr-only"],
  "validate-claude-native": ["Check / Claude plugin (strict)", "required"],
  "validate-antigravity-native": ["Check / Antigravity plugin", "required"],
  conformance: ["Check / Skills, Manifests, Docs, Repo, Hooks", "required"],
  "go-suite": ["Test / Go", "native"],
  "ts-suite": ["Test / TypeScript", "native"],
  format: ["Check / Format", "required"],
  shellcheck: ["Check / Shellcheck", "required"],
  "gh-aw-lock-drift": ["Check / gh-aw lock drift", "required"],
  manual: ["Manual", "post-merge"],
} as const;
export type JobId = keyof typeof SUMMARY_JOBS;
export const JOB_ORDER = Object.keys(SUMMARY_JOBS) as JobId[];
export interface Job {
  result: string;
  outputs?: unknown;
}

export function expectedJobResult(
  id: JobId,
  event: string,
  selection: Selection | null,
): "success" | "skipped" {
  switch (SUMMARY_JOBS[id][1]) {
    case "pr-only":
      return event === "pull_request" ? "success" : "skipped";
    case "post-merge":
      return event === "workflow_dispatch" ? "success" : "skipped";
    case "native":
      if (
        selection &&
        !(id === "go-suite" ? selection.shards : selection.tsShards).length
      )
        return "skipped";
  }
  return "success";
}

function member<T extends string>(
  values: readonly T[],
  value: string,
): value is T {
  return (values as readonly string[]).includes(value);
}

export function isFullReason(value: string): value is FullReason {
  return member(FULL_REASONS, value) || member(BACKSTOP_REASONS, value);
}

// GitHub omits empty output strings from toJSON(needs). Only the fields whose
// canonical value can be empty for this event and mode may be absent.
export function validateSelection(scope: Job, event: string): Selection {
  const outputs = scope.outputs;
  if (!outputs || typeof outputs !== "object" || Array.isArray(outputs))
    throw new Error("selector-invalid-output");
  function field(
    key: string,
    diagnostic = "output",
    optionalEmpty = false,
  ): string {
    const value: unknown = Reflect.get(outputs as object, key);
    if (typeof value === "string") return value;
    if (optionalEmpty && value === undefined) return "";
    throw new Error("selector-invalid-" + diagnostic);
  }
  const mode = field("mode", "mode");
  const complete = field("complete", "complete");
  const offPr = event !== "pull_request";
  const identity = {
    base: field("base", "identity", offPr),
    head: field("head", "identity", offPr),
    merge: field("merge", "identity", offPr),
  };
  const { base, head, merge } = identity;
  const reason = field("reason", "reason");
  if (!member(["full", "asset-only", "area-scoped"], mode))
    throw new Error("selector-invalid-mode");
  if (!member(["true", "false"], complete))
    throw new Error("selector-invalid-output");
  let selection: Selection;
  if (scope.result !== "success") {
    if (mode !== "full" || complete !== "false")
      throw new Error("selector-invalid-output");
    if (
      !member(FULL_REASONS, reason) &&
      !member(BACKSTOP_REASONS, reason) &&
      reason !== "eligible"
    )
      throw new Error("selector-invalid-reason");
    if (offPr && (base || head || merge))
      throw new Error("selector-invalid-output");
    // A failed scope always requires every suite. Preserve its recorded reason
    // in the summary even if the legacy selector reported eligibility.
    return { ...fullSelection("git-error", identity), reason };
  }
  if (offPr && (event !== "push" || reason === "event-full")) {
    if (mode === "area-scoped") throw new Error("selector-forbidden-area-mode");
    if (mode === "asset-only") throw new Error("selector-forbidden-asset-mode");
    if (
      complete !== "false" ||
      base ||
      head ||
      merge ||
      reason !== "event-full"
    )
      throw new Error("selector-invalid-output");
    selection = fullSelection(reason);
  } else {
    const sha = /^[0-9a-f]{40}$/;
    if (
      !sha.test(base) ||
      !sha.test(head) ||
      (offPr ? merge !== "" : !sha.test(merge))
    )
      throw new Error("selector-invalid-identity");
    if (mode === "full") {
      if (complete !== "false") throw new Error("selector-invalid-output");
      if (!member(FULL_REASONS, reason))
        throw new Error("selector-invalid-reason");
      selection = fullSelection(reason, identity);
    } else {
      if (reason !== (mode === "area-scoped" ? "area-scoped" : "eligible"))
        throw new Error("selector-invalid-reason");
      if (complete !== "true") throw new Error("selector-invalid-output");
      const ids = offPr ? [base, head] : [base, head, merge];
      if (ids.includes("0".repeat(40)) || new Set(ids).size !== ids.length)
        throw new Error("selector-invalid-identity");
      const areas = field("areas", "output", mode === "asset-only");
      const list = areas ? areas.split(",") : [];
      if (
        (mode === "asset-only") !== (list.length === 0) ||
        list.some((area) => !member(AREAS, area)) ||
        [...new Set(list)].sort().join(",") !== areas
      )
        throw new Error("selector-invalid-output");
      selection = reducedSelection(identity, list);
    }
  }
  const canonical = selectionOutputs(selection);
  if (field("areas", "output", !canonical.areas) !== canonical.areas)
    throw new Error("selector-invalid-output");
  if (field("shards") !== canonical.shards)
    throw new Error("selector-invalid-shards");
  if (field("ts_shards") !== canonical.ts_shards)
    throw new Error("selector-invalid-ts-shards");
  if (field("race") !== canonical.race)
    throw new Error("selector-invalid-race");
  return selection;
}

// Publish one proof of the validated selection. Job consumers compare it with
// the actual job outputs, where even validated step outputs can be absent.
export function encodeSchedule(scope: Job, event: string | undefined): string {
  if (scope.result !== "success" || !isSelectionEvent(event)) return "";
  try {
    const selection = validateSelection(scope, event);
    return `effective_selection=${selectionProof(selection)}\n`;
  } catch {
    return "";
  }
}

function matchesSchedule(rawOutputs: unknown, selection: Selection): boolean {
  if (typeof rawOutputs !== "object" || rawOutputs === null) return false;
  const proof: unknown = Reflect.get(rawOutputs, "effective_selection");
  // Actions compares strings case-insensitively. The raw record has already
  // passed validation, including its lowercase commit and scheduling fields.
  return (
    typeof proof === "string" &&
    proof.toLowerCase() === selectionProof(selection)
  );
}

// The scope diagnostic comes first, then job violations in the summary CLI's
// established order. An invalid selector gives every native suite a full run.
export function assessJobs(needs: Record<JobId, Job>, event: string) {
  const errors = [];
  if (needs.scope.result !== "success") errors.push("selector-failed");
  let selection: Selection | null = null;
  try {
    selection = validateSelection(needs.scope, event);
  } catch (error) {
    errors.push(error instanceof Error ? error.message : String(error));
  }
  if (selection?.complete && !matchesSchedule(needs.scope.outputs, selection)) {
    errors.push("selector-invalid-schedule");
    selection = null;
  }
  for (const group of ["required", "native", "post-merge", "pr-only"]) {
    for (const id of JOB_ORDER) {
      if (
        SUMMARY_JOBS[id][1] === group &&
        needs[id].result !== expectedJobResult(id, event, selection)
      )
        errors.push(`${group}-job:${id}`);
    }
  }
  return { selection, errors };
}

export {
  AREAS,
  SHARDS,
  SHARD_INPUTS,
  TS_SHARDS,
  TS_SHARD_INPUTS,
  RACE_INPUTS,
  areaOf,
  planFor,
};
