import type {
  BacklogIssue,
  BacklogChild,
  BacklogState,
} from "./backlog-grooming-model.ts";
import test from "node:test";
import assert from "node:assert/strict";
import {
  selectBatch,
  movableChildren,
  agentInput,
  staleFeatures,
} from "./backlog-grooming-select.ts";
import { CAPS, STALE_FEATURE_MS } from "./backlog-grooming-model.ts";
import { renderFeatureBody } from "./backlog-grooming-render.ts";

const DAY = 24 * 60 * 60 * 1000;
const NOW = Date.parse("2026-10-10T12:00:00Z");
const BASE = Date.parse("2026-09-01T00:00:00Z");

function issue(
  number: number,
  extra: Partial<BacklogIssue> = {},
): BacklogIssue {
  return {
    id: number * 10,
    number,
    title: `Issue ${number}`,
    body: "",
    state: "OPEN",
    labels: [],
    type: null,
    parent: null,
    parentAddedAt: null,
    createdAt: new Date(BASE + number * 1000).toISOString(),
    updatedAt: "2026-09-02T00:00:00Z",
    comments: [],
    children: [],
    ...extra,
  };
}

const empty: BacklogState = {
  lastCheck: null,
  lastSuccess: null,
  ungrouped: {},
};

test("the batch holds the oldest issues by creation time and counts the rest as queued", () => {
  // Creation order is decoupled from issue number, so sorting by number
  // alone would pick the wrong three.
  const issues = Array.from({ length: 100 }, (_, i) =>
    issue(i + 1, {
      createdAt: new Date(BASE + ((i * 37) % 100) * 1000).toISOString(),
    }),
  );
  const { batch, queued } = selectBatch({ issues, pulls: [] }, empty);
  assert.equal(batch.length, 40);
  assert.equal(queued, 60);
  assert.deepEqual(
    batch.slice(0, 3).map((i) => i.number),
    [1, 74, 47],
  );
});

test("issues created at the same moment are ordered by number", () => {
  const at = "2026-09-01T00:00:00Z";
  const issues = [issue(9, { createdAt: at }), issue(4, { createdAt: at })];
  const { batch } = selectBatch({ issues, pulls: [] }, empty);
  assert.deepEqual(
    batch.map((i) => i.number),
    [4, 9],
  );
});

test("parented, Feature-type, Feature-labeled, and index issues are excluded", () => {
  const issues = [
    issue(1, { parent: 9 }),
    issue(2, { type: "Feature" }),
    issue(3, { labels: ["agentics: feature"] }),
    issue(4, { labels: ["agentics: backlog-index"] }),
    issue(5),
  ];
  const { batch, queued } = selectBatch({ issues, pulls: [] }, empty);
  assert.deepEqual(
    batch.map((i) => i.number),
    [5],
  );
  assert.equal(queued, 0);
});

test("an Ungrouped issue waits until it is updated after the last check", () => {
  const state = {
    ...empty,
    lastCheck: "2026-09-05T00:00:00Z",
    ungrouped: { 1: "standalone", 2: "standalone", 4: "standalone" },
  };
  const issues = [
    issue(1),
    issue(2, { updatedAt: "2026-09-06T00:00:00Z" }),
    // Not Ungrouped, so the last check does not apply to it.
    issue(3),
    // Updated exactly at the last check: not after it, so it still waits.
    issue(4, { updatedAt: "2026-09-05T00:00:00Z" }),
  ];
  const { batch } = selectBatch({ issues, pulls: [] }, state);
  assert.deepEqual(
    batch.map((i) => i.number),
    [2, 3],
  );
});

test("an unreadable last check offers every Ungrouped issue again", () => {
  const state = {
    ...empty,
    lastCheck: "not a date",
    ungrouped: { 1: "standalone" },
  };
  const { batch } = selectBatch({ issues: [issue(1)], pulls: [] }, state);
  assert.deepEqual(
    batch.map((i) => i.number),
    [1],
  );
});

test("only children of bot Features linked 7 or more days ago are movable", () => {
  const issues = [
    issue(10, { labels: ["agentics: feature"] }),
    issue(11, {
      parent: 10,
      parentAddedAt: new Date(NOW - 7 * DAY).toISOString(),
    }),
    issue(12, {
      parent: 10,
      parentAddedAt: new Date(NOW - 7 * DAY + 1000).toISOString(),
    }),
    issue(20),
    issue(21, {
      parent: 20,
      parentAddedAt: new Date(NOW - 30 * DAY).toISOString(),
    }),
  ];
  assert.deepEqual(movableChildren({ issues, pulls: [] }, NOW), [
    { issue: 11, parent: 10 },
  ]);
});

test("movable children default to the snapshot's own clock", () => {
  const issues = [
    issue(10, { labels: ["agentics: feature"] }),
    issue(11, {
      parent: 10,
      parentAddedAt: new Date(NOW - 8 * DAY).toISOString(),
    }),
  ];
  const snapshot = { now: new Date(NOW).toISOString(), issues, pulls: [] };
  assert.deepEqual(movableChildren(snapshot), [{ issue: 11, parent: 10 }]);
});

test("agent input clips text to its limits and carries Features, Ungrouped, caps, and queue", () => {
  const comments = Array.from({ length: 12 }, (_, i) => ({
    author: "a",
    body: i === 11 ? "y".repeat(1500) : `c${i}`,
  }));
  const child = {
    id: 70,
    number: 7,
    title: "Child",
    state: "OPEN",
    type: "Task",
  };
  const snapshot = {
    issues: [
      issue(1, { body: "x".repeat(5000), comments }),
      issue(500, {
        labels: ["agentics: feature"],
        type: "Feature",
        body: "b".repeat(5000),
        children: [child],
      }),
      issue(880, { type: "Feature", children: [child] }),
      issue(881, { type: "Feature" }),
    ],
    pulls: [{ number: 50, title: "PR", draft: true, refs: [1] }],
  };
  const state = { ...empty, ungrouped: { 9: "standalone" } };
  const selection = selectBatch(snapshot, state);
  const input = agentInput(snapshot, state, selection, []);
  assert.ok(input.batch[0]);
  assert.ok(input.batch[0].comments[0]);
  assert.ok(input.batch[0].comments[9]);
  assert.ok(input.features[0]);
  assert.equal(input.batch[0].body.length, 4000);
  assert.ok(input.batch[0].body.endsWith("…"));
  assert.equal(input.batch[0].comments.length, 10);
  assert.equal(input.batch[0].comments[0].body, "c2");
  assert.equal(input.batch[0].comments[9].body.length, 1000);
  assert.deepEqual(input.batch[0].pulls, [50]);
  assert.deepEqual(
    input.features.map((f) => [f.number, f.own, f.body.length]),
    [
      [500, true, 4000],
      [880, false, 0],
    ],
  );
  assert.deepEqual(input.features[0].children, [
    { number: 7, title: "Child", state: "OPEN" },
  ]);
  assert.deepEqual(input.ungrouped, [{ issue: 9, reason: "standalone" }]);
  assert.deepEqual(input.caps, CAPS);
  assert.equal(input.queued, selection.queued);
});

function botFeatureIssue(
  number: number,
  ageMs: number,
  {
    single = false,
    children = [],
    ...extra
  }: Partial<BacklogIssue> & { single?: boolean } = {},
) {
  return issue(number, {
    labels: ["agentics: feature"],
    type: "Feature",
    body: renderFeatureBody({
      outcome: "o",
      why: "w",
      dependsOn: [],
      run: "5",
      ref: "new-1",
      single,
    }),
    createdAt: new Date(NOW - ageMs).toISOString(),
    children,
    ...extra,
  });
}

const kid = (number: number, state = "OPEN"): BacklogChild => ({
  id: number * 10,
  number,
  title: `Kid ${number}`,
  state,
  type: null,
});

function flagged(issues: BacklogIssue[]) {
  return staleFeatures({ issues, pulls: [] }, NOW).map((f) => [
    f.number,
    f.reason,
    f.days,
  ]);
}

test("a bot Feature is stale only once it is older than the window", () => {
  assert.deepEqual(flagged([botFeatureIssue(1, STALE_FEATURE_MS)]), []);
  assert.deepEqual(flagged([botFeatureIssue(1, STALE_FEATURE_MS + 1)]), [
    [1, "has no children", 7],
  ]);
});

test("a Feature with no children is flagged even when marked single", () => {
  assert.deepEqual(flagged([botFeatureIssue(1, 9 * DAY, { single: true })]), [
    [1, "has no children", 9],
  ]);
});

test("a Feature with one child is flagged unless it is marked single", () => {
  assert.deepEqual(
    flagged([botFeatureIssue(1, 9 * DAY, { children: [kid(11)] })]),
    [[1, "has one child and is not marked single", 9]],
  );
  assert.deepEqual(
    flagged([
      botFeatureIssue(1, 9 * DAY, { single: true, children: [kid(11)] }),
    ]),
    [],
  );
});

test("closed children count toward the child total", () => {
  assert.deepEqual(
    flagged([
      botFeatureIssue(1, 9 * DAY, { children: [kid(11), kid(12, "CLOSED")] }),
    ]),
    [],
  );
  assert.deepEqual(
    flagged([botFeatureIssue(1, 9 * DAY, { children: [kid(11, "CLOSED")] })]),
    [[1, "has one child and is not marked single", 9]],
  );
});

test("human Features, closed Features, and Features with two children are never flagged", () => {
  const human = issue(1, {
    type: "Feature",
    createdAt: new Date(NOW - 9 * DAY).toISOString(),
  });
  const closed = botFeatureIssue(2, 9 * DAY, { state: "CLOSED" });
  const crowded = botFeatureIssue(3, 9 * DAY, {
    children: [kid(31), kid(32)],
  });
  assert.deepEqual(flagged([human, closed, crowded]), []);
});

test("stale Features come back in issue-number order with whole-day ages", () => {
  assert.deepEqual(
    flagged([
      botFeatureIssue(7, 9 * DAY + 5 * 60 * 60 * 1000),
      botFeatureIssue(3, 10 * DAY),
    ]),
    [
      [3, "has no children", 10],
      [7, "has no children", 9],
    ],
  );
});
