import type {
  BacklogIssue,
  BacklogSnapshot,
  BacklogState,
} from "./backlog-grooming-model.ts";
export interface BatchSelection {
  batch: BacklogIssue[];
  queued: number;
}
export interface MovableChild {
  issue: number;
  parent: number;
}
export interface StaleFeature {
  number: number;
  reason: string;
  days: number;
}
// Selection decides what the agent sees. Progress lives in GitHub: an issue
// with a parent is done, and an issue already judged Ungrouped waits until
// it changes. Children of human parents are excluded the same way; the
// agent still sees those parents in its Features list, marked own: false.
// Everything here is a pure function of the snapshot.

import {
  LABEL_FEATURE,
  LABEL_INDEX,
  BATCH_SIZE,
  STICKY_MS,
  STALE_FEATURE_MS,
  BODY_LIMIT,
  COMMENT_LIMIT,
  COMMENTS_PER_ISSUE,
  CAPS,
  isBotFeature,
  isFeatureType,
} from "./backlog-grooming-model.ts";
import { readMarker } from "./backlog-grooming-render.ts";

const DAY_MS = 24 * 60 * 60 * 1000;

// The ellipsis counts toward the limit, so a clipped value is exactly
// `limit` characters.
function clip(value: string, limit: number) {
  return value.length > limit ? `${value.slice(0, limit - 1)}…` : value;
}

function isCandidate(issue: BacklogIssue) {
  return (
    issue.state === "OPEN" &&
    issue.parent === null &&
    !isFeatureType(issue) &&
    !issue.labels.includes(LABEL_FEATURE) &&
    !issue.labels.includes(LABEL_INDEX)
  );
}

function selectBatch(
  snapshot: BacklogSnapshot,
  state: BacklogState,
  batchSize = BATCH_SIZE,
): BatchSelection {
  // An unreadable last-check time fails open: every Ungrouped issue is
  // offered again rather than skipped against a time nobody can trust.
  const parsed = state.lastCheck === null ? NaN : Date.parse(state.lastCheck);
  const lastCheck = Number.isNaN(parsed) ? null : parsed;
  const candidates = snapshot.issues
    .filter(isCandidate)
    .filter((issue) => {
      const judged = Object.hasOwn(state.ungrouped, String(issue.number));
      return !(
        judged &&
        lastCheck !== null &&
        Date.parse(issue.updatedAt) <= lastCheck
      );
    })
    .sort(
      (a, b) =>
        Date.parse(a.createdAt) - Date.parse(b.createdAt) ||
        a.number - b.number,
    );
  return {
    batch: candidates.slice(0, batchSize),
    queued: Math.max(0, candidates.length - batchSize),
  };
}

function movableChildren(
  snapshot: BacklogSnapshot,
  now = Date.parse(snapshot.now ?? ""),
): MovableChild[] {
  const own = new Set(
    snapshot.issues.filter(isBotFeature).map((i) => i.number),
  );
  return snapshot.issues
    .filter(
      (issue): issue is BacklogIssue & { parent: number } =>
        issue.parent !== null &&
        own.has(issue.parent) &&
        issue.parentAddedAt !== null &&
        now - Date.parse(issue.parentAddedAt) >= STICKY_MS,
    )
    .map((issue) => ({ issue: issue.number, parent: issue.parent }));
}

// A workflow-created Feature that stays empty, or holds one child without
// being marked single, is a grouping that did not land. Nothing closes it;
// the index lists it so a person can. Every sub-issue counts as a child,
// open or closed.
function staleFeatures(
  snapshot: BacklogSnapshot,
  now = Date.parse(snapshot.now ?? ""),
): StaleFeature[] {
  return snapshot.issues
    .filter(
      (issue) =>
        issue.state === "OPEN" &&
        isBotFeature(issue) &&
        now - Date.parse(issue.createdAt) > STALE_FEATURE_MS,
    )
    .flatMap((issue) => {
      const days = Math.floor((now - Date.parse(issue.createdAt)) / DAY_MS);
      if (issue.children.length === 0) {
        return [{ number: issue.number, reason: "has no children", days }];
      }
      const marker = readMarker(issue.body);
      if (issue.children.length === 1 && !(marker && marker.single)) {
        return [
          {
            number: issue.number,
            reason: "has one child and is not marked single",
            days,
          },
        ];
      }
      return [];
    })
    .sort((a, b) => a.number - b.number);
}

function pullsFor(snapshot: BacklogSnapshot, number: number): number[] {
  return snapshot.pulls
    .filter((p) => p.refs.includes(number))
    .map((p) => p.number);
}

function isListedFeature(issue: BacklogIssue) {
  return (
    isBotFeature(issue) || (isFeatureType(issue) && issue.children.length > 0)
  );
}

function agentInput(
  snapshot: BacklogSnapshot,
  state: BacklogState,
  selection: BatchSelection,
  movable: MovableChild[],
) {
  return {
    caps: CAPS,
    queued: selection.queued,
    batch: selection.batch.map((issue) => ({
      number: issue.number,
      title: issue.title,
      type: issue.type,
      labels: issue.labels,
      createdAt: issue.createdAt,
      body: clip(issue.body, BODY_LIMIT),
      comments: issue.comments.slice(-COMMENTS_PER_ISSUE).map((c) => ({
        author: c.author,
        body: clip(c.body, COMMENT_LIMIT),
      })),
      pulls: pullsFor(snapshot, issue.number),
    })),
    features: snapshot.issues.filter(isListedFeature).map((f) => ({
      number: f.number,
      title: f.title,
      own: isBotFeature(f),
      body: clip(f.body, BODY_LIMIT),
      children: f.children.map((c) => ({
        number: c.number,
        title: c.title,
        state: c.state,
      })),
    })),
    movable,
    ungrouped: Object.entries(state.ungrouped).map(([number, reason]) => ({
      issue: Number(number),
      reason,
    })),
  };
}

export {
  selectBatch,
  movableChildren,
  staleFeatures,
  pullsFor,
  isListedFeature,
  agentInput,
};
