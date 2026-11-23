import {
  isRecord,
  type BacklogState,
  type GroomingGap,
} from "./backlog-grooming-model.ts";
export interface FeatureMarker {
  run: string;
  ref: string;
  single: boolean;
  deps: number[];
}
export interface FeatureBody {
  outcome: string;
  why: string;
  dependsOn: number[];
  run: string;
  ref: string;
  single?: boolean;
}
export interface IndexChild {
  number: number;
  title: string;
  closed: boolean;
  pulls: number[];
}
export interface IndexFeature {
  number: number;
  title: string;
  human: boolean;
  outcome: string | null;
  deps: number[];
  priority: number;
  children: IndexChild[];
}
export type RenderedBacklogState = Omit<BacklogState, "lastCheck"> & {
  lastCheck: unknown;
};

export interface BacklogIndexView {
  runUrl: string;
  mode: string;
  features: IndexFeature[];
  ungrouped: { number: number; title: string; reason: string }[];
  proposed: string[] | null;
  changes: string[];
  gaps: string[];
  queued: unknown;
  failedRuns: unknown;
  checkedAt: string;
  state: RenderedBacklogState;
  staleGaps?: string[];
  omissionNotices?: string[];
}
// Rendering and reading the two bodies the workflow owns: a Feature issue
// and the index. Hidden markers carry the state the next run reads, so the
// parse and render of each marker live side by side.

import { MARKER, INDEX_LIMIT } from "./backlog-grooming-model.ts";

// [\s\S] rather than `.`: JSON.stringify leaves U+2028 and U+2029 unescaped,
// and `.` does not match them, which would silently drop the whole state.
const STATE_RE = new RegExp(`<!-- ${MARKER} state (\\{[\\s\\S]*?\\}) -->`);
const MANAGED_RE = new RegExp(`<!-- ${MARKER} managed ([^>]*?) -->`);
const PRIORITY_RE = /^priority:p([0-9])$/;

// Issue and agent text lands in bodies GitHub renders. A bare @ would
// notify someone, and an HTML comment opener could forge a marker.
function inline(value: unknown) {
  return safe(value).replace(/\s+/g, " ").trim();
}

function safe(value: unknown) {
  return String(value).replace(/@/g, "@\u200b").replace(/<!--/g, "&lt;!--");
}

export function renderGap({ subject, status, reason }: GroomingGap): string {
  const label = status === "dropped" ? subject : `${subject} ${status}`;
  return `${label}: ${reason}`;
}

function emptyState(): BacklogState {
  return { lastCheck: null, lastSuccess: null, ungrouped: {} };
}

function readState(body: string | null | undefined): BacklogState {
  const match = STATE_RE.exec(body || "");
  if (!match) return emptyState();
  try {
    const parsed: unknown = JSON.parse(match[1] ?? "");
    if (!isRecord(parsed)) return emptyState();
    const fields: Record<string, unknown> = parsed;
    return {
      lastCheck: typeof fields.lastCheck === "string" ? fields.lastCheck : null,
      lastSuccess:
        typeof fields.lastSuccess === "string" ? fields.lastSuccess : null,
      ungrouped:
        fields.ungrouped &&
        typeof fields.ungrouped === "object" &&
        !Array.isArray(fields.ungrouped)
          ? Object.fromEntries(
              Object.entries(fields.ungrouped).filter(
                (entry): entry is [string, string] =>
                  typeof entry[1] === "string",
              ),
            )
          : {},
    };
  } catch {
    return emptyState();
  }
}

function writeState(state: RenderedBacklogState) {
  return `<!-- ${MARKER} state ${JSON.stringify(state).replace(/--/g, "-\\u002d")} -->`;
}

function readMarker(body: string | null | undefined): FeatureMarker | null {
  const match = MANAGED_RE.exec(body || "");
  if (!match) return null;
  const fields: Record<string, string | undefined> = Object.fromEntries(
    (match[1] ?? "")
      .split(" ")
      .filter(Boolean)
      .map((pair) => pair.split("=")),
  );
  return {
    run: fields.run || "",
    ref: fields.ref || "",
    single: fields.single === "true",
    deps: (fields.deps || "")
      .split(",")
      .map(Number)
      .filter((n) => Number.isSafeInteger(n) && n > 0),
  };
}

function writeMarker({
  run,
  ref,
  single,
  dependsOn,
}: Pick<FeatureBody, "run" | "ref" | "single" | "dependsOn">) {
  return `<!-- ${MARKER} managed run=${run} ref=${ref} single=${single === true} deps=${dependsOn.join(",")} -->`;
}

function renderFeatureBody({
  outcome,
  why,
  dependsOn,
  run,
  ref,
  single,
}: FeatureBody) {
  const deps =
    dependsOn.length === 0 ? "none" : dependsOn.map((n) => `#${n}`).join(", ");
  return [
    "## Outcome",
    "",
    safe(outcome),
    "",
    "## Why these belong together",
    "",
    safe(why),
    "",
    "## Depends on",
    "",
    deps,
    "",
    writeMarker({ run, ref, single, dependsOn }),
    "",
  ].join("\n");
}

// readSection takes the first matching heading. A heading injected into an
// earlier section's text can shadow the real one; that text is the bot's
// own sanitized output, so this is accepted rather than parsed strictly.
function readSection(body: string | null | undefined, heading: string) {
  const start = (body || "").indexOf(`## ${heading}\n\n`);
  if (start === -1) return null;
  const rest = (body || "").slice(start + heading.length + 5);
  const ends = ["\n## ", "\n<!-- "]
    .map((s) => rest.indexOf(s))
    .filter((i) => i !== -1);
  return (
    rest.slice(0, ends.length === 0 ? rest.length : Math.min(...ends)).trim() ||
    null
  );
}

function priorityOf(labels: readonly string[]) {
  let best = 4;
  for (const label of labels) {
    const match = PRIORITY_RE.exec(label);
    if (match) best = Math.min(best, Number(match[1]));
  }
  return best;
}

// Dependencies first, then priority, then issue number. A cycle is broken
// by the same priority order rather than dropping the Features in it.
function orderFeatures<
  T extends { number: number; priority: number; deps: number[] },
>(features: T[]): T[] {
  const rank = (a: T, b: T) => a.priority - b.priority || a.number - b.number;
  const listed = new Set(features.map((f) => f.number));
  const pending = new Map(
    features.map((f) => [
      f.number,
      new Set(f.deps.filter((d) => listed.has(d) && d !== f.number)),
    ]),
  );
  const ordered: T[] = [];
  const remaining = [...features].sort(rank);
  while (remaining.length > 0) {
    let index = remaining.findIndex((f) => pending.get(f.number)?.size === 0);
    if (index === -1) index = 0;
    for (const next of remaining.splice(index, 1)) {
      ordered.push(next);
      for (const deps of pending.values()) deps.delete(next.number);
    }
  }
  return ordered;
}

function childLine(child: IndexChild, compact: boolean) {
  if (compact) return null;
  if (child.closed) return `- ☑ #${child.number} ${inline(child.title)}`;
  if (child.pulls.length > 0) {
    return `- 🔗 #${child.number} ${inline(child.title)} (${child.pulls.map((p) => `PR #${p}`).join(", ")})`;
  }
  return `- #${child.number} ${inline(child.title)}`;
}

function featureBlock(feature: IndexFeature, compact: boolean) {
  const priority = feature.priority < 4 ? ` · P${feature.priority}` : "";
  const lines = [
    `### #${feature.number} ${inline(feature.title)}${priority}`,
    "",
  ];
  const notes = [];
  if (feature.outcome) notes.push(inline(feature.outcome));
  if (feature.deps.length > 0)
    notes.push(`Start after ${feature.deps.map((d) => `#${d}`).join(", ")}.`);
  const closed = feature.children.filter((c) => c.closed).length;
  if (
    feature.human &&
    feature.children.length > 0 &&
    closed === feature.children.length
  )
    notes.push("All children closed.");
  if (compact)
    notes.push(`${closed} of ${feature.children.length} children closed.`);
  if (notes.length > 0) lines.push(notes.join(" "), "");
  for (const child of feature.children) {
    const line = childLine(child, compact);
    if (line !== null) lines.push(line);
  }
  if (!compact) lines.push("");
  return lines;
}

function renderIndexBody(view: BacklogIndexView, compact: boolean) {
  const lines = [
    "# Backlog index",
    "",
    `> Maintained by [backlog grooming](${view.runUrl}). Each run replaces this body; comment instead of editing.`,
    "",
    "## Features",
    "",
  ];
  const features = orderFeatures(view.features);
  if (features.length === 0) lines.push("None.", "");
  for (const feature of features) lines.push(...featureBlock(feature, compact));
  lines.push("## Ungrouped", "");
  if (view.ungrouped.length === 0) lines.push("None.");
  for (const u of view.ungrouped)
    lines.push(`- #${u.number} ${inline(u.title)}: ${inline(u.reason)}`);
  lines.push("");
  if (view.proposed !== null) {
    lines.push("## Proposed", "");
    if (view.proposed.length === 0) lines.push("No changes proposed.");
    for (const p of view.proposed) lines.push(`- ${inline(p)}`);
    lines.push("");
  }
  lines.push("## Changes this run", "");
  if (view.changes.length === 0) lines.push("None.");
  for (const c of view.changes) lines.push(`- ${inline(c)}`);
  const gaps = [
    ...view.gaps,
    ...(view.staleGaps || []),
    ...(view.omissionNotices || []),
  ];
  lines.push(
    "",
    "## Status",
    "",
    `- Mode: ${view.mode}`,
    `- Queued for the next run: ${view.queued}`,
    `- Last checked: ${view.checkedAt}`,
    `- Last successful refresh: ${view.state.lastSuccess ? inline(view.state.lastSuccess) : "never"}`,
    `- Failed runs since the last success: ${view.failedRuns}`,
    `- Coverage gaps: ${gaps.length === 0 ? "none" : gaps.map(inline).join("; ")}`,
    "",
    writeState(view.state),
    "",
  );
  return lines.join("\n");
}

// GitHub caps an issue body at 65,536 characters. Shrink in order of what a
// reader loses least: child lines, then the reasons copied into the hidden
// state (the visible Ungrouped lines keep theirs), then visible Ungrouped
// lines, found by binary search because each render is O(body).
function clipReasons(
  state: RenderedBacklogState,
  limit: number,
): RenderedBacklogState {
  return {
    ...state,
    ungrouped: Object.fromEntries(
      Object.entries(state.ungrouped).map(([n, reason]) => [
        n,
        reason.length > limit ? `${reason.slice(0, limit - 1)}…` : reason,
      ]),
    ),
  };
}

function renderIndex(view: BacklogIndexView): string {
  const full = renderIndexBody(view, false);
  if (full.length <= INDEX_LIMIT) return full;
  const compact = renderIndexBody(view, true);
  if (compact.length <= INDEX_LIMIT) return compact;
  const lean = { ...view, state: clipReasons(view.state, 60) };
  const leanBody = renderIndexBody(lean, true);
  if (leanBody.length <= INDEX_LIMIT) return leanBody;
  const stale = lean.staleGaps || [];
  const withShown = (ungroupedCount: number, staleCount: number) => {
    const omittedUngrouped = lean.ungrouped.length - ungroupedCount;
    const omittedStale = stale.length - staleCount;
    const omissionNotices = [
      ...(omittedUngrouped > 0
        ? [`${omittedUngrouped} Ungrouped issues not shown`]
        : []),
      ...(omittedStale > 0
        ? [`${omittedStale} more stale Features not shown`]
        : []),
    ];
    return renderIndexBody(
      {
        ...lean,
        ungrouped: lean.ungrouped.slice(0, ungroupedCount),
        staleGaps: stale.slice(0, staleCount),
        omissionNotices,
      },
      true,
    );
  };
  const search = (
    high: number,
    render: (n: number) => string,
  ): string | null => {
    let low = 0;
    let best: string | null = null;
    while (low <= high) {
      const mid = Math.floor((low + high) / 2);
      const body = render(mid);
      if (body.length <= INDEX_LIMIT) {
        best = body;
        low = mid + 1;
      } else {
        high = mid - 1;
      }
    }
    return best;
  };
  const ungroupedBody = search(lean.ungrouped.length - 1, (n) =>
    withShown(n, stale.length),
  );
  if (ungroupedBody !== null) return ungroupedBody;
  const staleBody = search(stale.length - 1, (n) => withShown(0, n));
  if (staleBody !== null) return staleBody;
  const marker = writeState(lean.state).length;
  throw new Error(
    `index body exceeds ${INDEX_LIMIT} characters with no Ungrouped lines shown; the state marker alone is ${marker} characters`,
  );
}

export {
  inline,
  readState,
  emptyState,
  readMarker,
  renderFeatureBody,
  readSection,
  priorityOf,
  orderFeatures,
  renderIndex,
};
