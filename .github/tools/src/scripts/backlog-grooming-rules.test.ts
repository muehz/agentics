import type {
  BacklogIssue,
  BacklogPlan,
  NewFeature,
} from "./backlog-grooming-model.ts";
// Each rule has a test that names the drop line it produces. Delete the
// rule and its test fails on that line, not on an incidental error.
import test from "node:test";
import assert from "node:assert/strict";
import { planOperations, describe } from "./backlog-grooming-rules.ts";

const DAY = 24 * 60 * 60 * 1000;
const NOW = Date.parse("2026-10-10T12:00:00Z");
const OLD = new Date(NOW - 8 * DAY).toISOString();
const NEW = new Date(NOW - 2 * DAY).toISOString();

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
    createdAt: "2026-09-01T00:00:00Z",
    updatedAt: "2026-09-02T00:00:00Z",
    comments: [],
    children: [],
    ...extra,
  };
}

function plan(extra: Partial<BacklogPlan> = {}): BacklogPlan {
  return {
    features: [],
    links: [],
    moves: [],
    featureUpdates: [],
    ungrouped: [],
    ...extra,
  };
}

function feature(ref: string, extra: Partial<NewFeature> = {}): NewFeature {
  return {
    ref,
    title: `F ${ref}`,
    outcome: "o",
    why: "w",
    dependsOn: [],
    single: false,
    ...extra,
  };
}

const bot = (
  number: number,
  children: number[],
  extra: Partial<BacklogIssue> = {},
) =>
  issue(number, {
    labels: ["agentics: feature"],
    type: "Feature",
    children: children.map((n) => ({
      id: n * 10,
      number: n,
      title: `Issue ${n}`,
      state: "OPEN",
      type: "Task",
    })),
    ...extra,
  });

test("a new Feature with two links is created and its untyped children get Task", () => {
  const live = { issues: [issue(1), issue(2, { type: "Bug" })], pulls: [] };
  const ops = planOperations(
    plan({
      features: [feature("new-1")],
      links: [
        { issue: 1, feature: "new-1" },
        { issue: 2, feature: "new-1" },
      ],
    }),
    live,
    NOW,
  );
  assert.deepEqual(ops.drops, []);
  assert.equal(ops.creates.length, 1);
  assert.deepEqual(
    ops.links.map((l) => l.setTask),
    [true, false],
  );
});

test("new Features over the cap of 5 are dropped", () => {
  const issues = Array.from({ length: 12 }, (_, i) => issue(i + 1));
  const features = Array.from({ length: 6 }, (_, i) => feature(`new-${i + 1}`));
  const links = Array.from({ length: 12 }, (_, i) => ({
    issue: i + 1,
    feature: `new-${Math.floor(i / 2) + 1}`,
  }));
  const ops = planOperations(
    plan({ features, links }),
    { issues, pulls: [] },
    NOW,
  );
  assert.equal(ops.creates.length, 5);
  assert.ok(
    ops.drops.includes("feature new-6: over the cap of 5 new Features"),
  );
});

test("links over the cap of 40 are dropped", () => {
  const issues = [
    bot(500, [1000, 1001]),
    ...Array.from({ length: 41 }, (_, i) => issue(i + 1)),
  ];
  const links = Array.from({ length: 41 }, (_, i) => ({
    issue: i + 1,
    feature: 500,
  }));
  const ops = planOperations(plan({ links }), { issues, pulls: [] }, NOW);
  assert.equal(ops.links.length, 40);
  assert.ok(ops.drops.includes("link #41 to #500: over the cap of 40 links"));
});

test("a link into a parent this workflow did not create is dropped", () => {
  const live = {
    issues: [issue(880, { type: "Feature" }), issue(1)],
    pulls: [],
  };
  const ops = planOperations(
    plan({ links: [{ issue: 1, feature: 880 }] }),
    live,
    NOW,
  );
  assert.ok(
    ops.drops.includes(
      "link #1 to #880: the target is not a Feature this workflow created",
    ),
  );
});

test("a link for an issue that already has a parent is dropped", () => {
  const live = {
    issues: [bot(500, [3, 4]), issue(1, { parent: 880 })],
    pulls: [],
  };
  const ops = planOperations(
    plan({ links: [{ issue: 1, feature: 500 }] }),
    live,
    NOW,
  );
  assert.ok(
    ops.drops.includes("link #1 to #500: the issue already has parent #880"),
  );
});

test("a new Feature with one child is dropped unless marked single", () => {
  const live = { issues: [issue(1), issue(2)], pulls: [] };
  const ops = planOperations(
    plan({
      features: [feature("new-1"), feature("new-2", { single: true })],
      links: [
        { issue: 1, feature: "new-1" },
        { issue: 2, feature: "new-2" },
      ],
    }),
    live,
    NOW,
  );
  assert.deepEqual(
    ops.creates.map((f) => f.ref),
    ["new-2"],
  );
  assert.ok(
    ops.drops.includes(
      "feature new-1: 1 children; a Feature needs two, or one when marked single",
    ),
  );
  assert.ok(ops.drops.includes("link #1 to new-1: its Feature was dropped"));
});

test("a move of a child linked less than 7 days ago is dropped", () => {
  const live = {
    issues: [
      bot(500, [1, 2, 3]),
      bot(600, [4, 5]),
      issue(1, { parent: 500, parentAddedAt: NEW }),
    ],
    pulls: [],
  };
  const ops = planOperations(
    plan({ moves: [{ issue: 1, to: 600, reason: "fits" }] }),
    live,
    NOW,
  );
  assert.ok(
    ops.drops.includes("move #1 to #600: it was linked less than 7 days ago"),
  );
});

test("moves over the cap of 5 are dropped", () => {
  const kids = [1, 2, 3, 4, 5, 6];
  const issues = [
    bot(500, [...kids, 7, 8]),
    bot(600, [9, 10]),
    ...kids.map((n) => issue(n, { parent: 500, parentAddedAt: OLD })),
  ];
  const moves = kids.map((n) => ({ issue: n, to: 600, reason: "fits" }));
  const ops = planOperations(plan({ moves }), { issues, pulls: [] }, NOW);
  assert.equal(ops.moves.length, 5);
  assert.ok(ops.drops.includes("move #6 to #600: over the cap of 5 moves"));
});

test("a move out of a human parent is dropped", () => {
  const live = {
    issues: [
      issue(880, { type: "Feature" }),
      issue(1, { parent: 880, parentAddedAt: OLD }),
    ],
    pulls: [],
  };
  const ops = planOperations(
    plan({ moves: [{ issue: 1, to: null, reason: "x" }] }),
    live,
    NOW,
  );
  assert.ok(
    ops.drops.includes(
      "move #1 to Ungrouped: the issue is not under a Feature this workflow created",
    ),
  );
});

test("a bot Feature left with one child closes and sends the child to Ungrouped", () => {
  const live = {
    issues: [
      bot(500, [1, 2]),
      issue(1, { parent: 500, parentAddedAt: OLD }),
      issue(2, { parent: 500, parentAddedAt: OLD }),
    ],
    pulls: [],
  };
  const ops = planOperations(
    plan({ moves: [{ issue: 1, to: null, reason: "fits nowhere" }] }),
    live,
    NOW,
  );
  assert.deepEqual(
    ops.moves.map((m) => [m.issue, m.to]),
    [
      [1, null],
      [2, null],
    ],
  );
  assert.deepEqual(ops.closes, [{ issue: 500, reason: "not_planned" }]);
  assert.ok(ops.ungrouped.some((u) => u.issue === 2));
  assert.deepEqual(
    ops.moves.map((m) => m.forced === true),
    [false, true],
  );
});

test("an empty bot Feature not emptied by moves stays open", () => {
  const ops = planOperations(
    plan(),
    { issues: [bot(500, [])], pulls: [] },
    NOW,
  );
  assert.deepEqual(ops.closes, []);
});

test("a bot Feature left with one closed child closes as completed", () => {
  const feature = bot(500, [1]);
  feature.children.push({
    id: 20,
    number: 2,
    title: "t",
    state: "CLOSED",
    type: "Task",
  });
  const live = {
    issues: [feature, issue(1, { parent: 500, parentAddedAt: OLD })],
    pulls: [],
  };
  const ops = planOperations(
    plan({ moves: [{ issue: 1, to: null, reason: "fits nowhere" }] }),
    live,
    NOW,
  );
  assert.deepEqual(ops.closes, [{ issue: 500, reason: "completed" }]);
  assert.deepEqual(
    ops.moves.map((m) => m.issue),
    [1],
  );
});

test("a bot Feature whose children are all closed closes as completed", () => {
  const done = bot(500, []);
  done.children = [1, 2].map((n) => ({
    id: n * 10,
    number: n,
    title: "t",
    state: "CLOSED",
    type: "Task",
  }));
  const ops = planOperations(plan(), { issues: [done], pulls: [] }, NOW);
  assert.deepEqual(ops.closes, [{ issue: 500, reason: "completed" }]);
});

test("a human parent is never closed", () => {
  const human = issue(880, { type: "Feature" });
  human.children = [
    { id: 1, number: 1, title: "t", state: "CLOSED", type: "Task" },
  ];
  const ops = planOperations(plan(), { issues: [human], pulls: [] }, NOW);
  assert.deepEqual(ops.closes, []);
});

test("an update to a Feature this workflow did not create is dropped", () => {
  const live = { issues: [issue(880, { type: "Feature" })], pulls: [] };
  const update = {
    feature: 880,
    title: "t",
    outcome: "o",
    why: "w",
    dependsOn: [],
  };
  const ops = planOperations(plan({ featureUpdates: [update] }), live, NOW);
  assert.ok(
    ops.drops.includes(
      "update #880: the target is not a Feature this workflow created",
    ),
  );
});

test("describe lists every operation for the Proposed section", () => {
  const live = { issues: [issue(1), issue(2)], pulls: [] };
  const ops = planOperations(
    plan({
      features: [feature("new-1")],
      links: [
        { issue: 1, feature: "new-1" },
        { issue: 2, feature: "new-1" },
      ],
    }),
    live,
    NOW,
  );
  assert.deepEqual(describe(ops), [
    'create new-1 "F new-1"',
    "link #1 to new-1",
    "link #2 to new-1",
  ]);
});

// Settling: a reshape stands only when its forced consequence is allowed.

test("a dropped move does not shrink or close its source Feature", () => {
  const live = {
    issues: [bot(500, [1]), issue(1, { parent: 500, parentAddedAt: OLD })],
    pulls: [],
  };
  const ops = planOperations(
    plan({
      features: [feature("new-1")],
      moves: [{ issue: 1, to: "new-1", reason: "fits" }],
    }),
    live,
    NOW,
  );
  assert.deepEqual(ops.moves, []);
  assert.deepEqual(ops.closes, []);
  assert.ok(ops.drops.includes("move #1 to new-1: its Feature was dropped"));
});

test("a reshape that would force out a sticky last child is dropped", () => {
  const live = {
    issues: [
      bot(500, [1, 2]),
      issue(1, { parent: 500, parentAddedAt: OLD }),
      issue(2, { parent: 500, parentAddedAt: NEW }),
    ],
    pulls: [],
  };
  const ops = planOperations(
    plan({ moves: [{ issue: 1, to: null, reason: "fits nowhere" }] }),
    live,
    NOW,
  );
  assert.deepEqual(ops.moves, []);
  assert.deepEqual(ops.closes, []);
  assert.ok(
    ops.drops.includes(
      "move #1 to Ungrouped: it would leave #500 with #2, which cannot move yet",
    ),
  );
});

test("forced moves count against the cap of 5 moves", () => {
  const features = [500, 600, 700];
  const issues = features.flatMap((f, i) => [
    bot(f, [i * 2 + 1, i * 2 + 2]),
    issue(i * 2 + 1, { parent: f, parentAddedAt: OLD }),
    issue(i * 2 + 2, { parent: f, parentAddedAt: OLD }),
  ]);
  const moves = features.map((_, i) => ({
    issue: i * 2 + 1,
    to: null,
    reason: "fits nowhere",
  }));
  const ops = planOperations(plan({ moves }), { issues, pulls: [] }, NOW);
  assert.ok(ops.moves.length <= 5);
  assert.equal(ops.moves.length, 4);
  assert.ok(
    ops.drops.includes(
      "move #5 to Ungrouped: moving #6 out of #700 would pass the cap of 5 moves",
    ),
  );
});

test("a child linked into a Feature this run is never forced back out", () => {
  const live = {
    issues: [
      bot(500, [1]),
      issue(1, { parent: 500, parentAddedAt: OLD }),
      issue(3),
    ],
    pulls: [],
  };
  const ops = planOperations(
    plan({
      links: [{ issue: 3, feature: 500 }],
      moves: [{ issue: 1, to: null, reason: "fits nowhere" }],
    }),
    live,
    NOW,
  );
  assert.deepEqual(
    ops.links.map((l) => l.issue),
    [3],
  );
  assert.deepEqual(ops.moves, []);
  assert.deepEqual(ops.closes, []);
  assert.ok(
    ops.drops.includes(
      "move #1 to Ungrouped: it would leave #500 with only #3, placed there this run",
    ),
  );
});

test("an emptied bot Feature closes as not planned", () => {
  const live = {
    issues: [
      bot(500, [1, 2]),
      bot(600, [3, 4]),
      issue(1, { parent: 500, parentAddedAt: OLD }),
      issue(2, { parent: 500, parentAddedAt: OLD }),
    ],
    pulls: [],
  };
  const ops = planOperations(
    plan({
      moves: [
        { issue: 1, to: 600, reason: "merge" },
        { issue: 2, to: 600, reason: "merge" },
      ],
    }),
    live,
    NOW,
  );
  assert.deepEqual(ops.closes, [{ issue: 500, reason: "not_planned" }]);
});

test("a single-marked bot Feature left with one child stays open", () => {
  const marker =
    "<!-- agentics:backlog-grooming managed run=1 ref=new-1 single=true deps= -->";
  const live = {
    issues: [
      bot(500, [1, 2], { body: marker }),
      bot(600, [3, 4]),
      issue(1, { parent: 500, parentAddedAt: OLD }),
      issue(2, { parent: 500, parentAddedAt: OLD }),
    ],
    pulls: [],
  };
  const ops = planOperations(
    plan({ moves: [{ issue: 1, to: 600, reason: "fits" }] }),
    live,
    NOW,
  );
  assert.deepEqual(
    ops.moves.map((m) => m.issue),
    [1],
  );
  assert.deepEqual(ops.closes, []);
});

test("a bot Feature with one child that no move touched stays open", () => {
  const live = {
    issues: [bot(500, [1]), issue(1, { parent: 500, parentAddedAt: OLD })],
    pulls: [],
  };
  const ops = planOperations(plan(), live, NOW);
  assert.deepEqual(ops.moves, []);
  assert.deepEqual(ops.closes, []);
});

test("a move to something other than a bot Feature is dropped", () => {
  const live = {
    issues: [
      bot(500, [1, 2]),
      issue(880, { type: "Feature" }),
      issue(1, { parent: 500, parentAddedAt: OLD }),
    ],
    pulls: [],
  };
  const ops = planOperations(
    plan({ moves: [{ issue: 1, to: 880, reason: "fits" }] }),
    live,
    NOW,
  );
  assert.ok(
    ops.drops.includes(
      "move #1 to #880: the target is not a Feature this workflow created",
    ),
  );
});

test("a move to the child's current parent is dropped", () => {
  const live = {
    issues: [bot(500, [1, 2]), issue(1, { parent: 500, parentAddedAt: OLD })],
    pulls: [],
  };
  const ops = planOperations(
    plan({ moves: [{ issue: 1, to: 500, reason: "stay" }] }),
    live,
    NOW,
  );
  assert.ok(
    ops.drops.includes(
      "move #1 to #500: the issue is already under that Feature",
    ),
  );
});

test("a move of a child with no known link date is dropped", () => {
  const live = {
    issues: [bot(500, [1, 2]), issue(1, { parent: 500, parentAddedAt: null })],
    pulls: [],
  };
  const ops = planOperations(
    plan({ moves: [{ issue: 1, to: null, reason: "x" }] }),
    live,
    NOW,
  );
  assert.ok(
    ops.drops.includes("move #1 to Ungrouped: its link date is unknown"),
  );
});

for (const [name, extra, why] of [
  [
    "a Feature-type issue",
    { type: "Feature" },
    "the issue is a Feature or the index",
  ],
  [
    "a bot Feature",
    { labels: ["agentics: feature"] },
    "the issue is a Feature or the index",
  ],
  [
    "the index",
    { labels: ["agentics: backlog-index"] },
    "the issue is a Feature or the index",
  ],
] satisfies [string, Partial<BacklogIssue>, string][]) {
  test(`a link of ${name} is dropped`, () => {
    const live = { issues: [bot(500, [3, 4]), issue(1, extra)], pulls: [] };
    const ops = planOperations(
      plan({ links: [{ issue: 1, feature: 500 }] }),
      live,
      NOW,
    );
    assert.ok(ops.drops.includes(`link #1 to #500: ${why}`));
  });

  test(`${name} cannot be left ungrouped`, () => {
    const live = { issues: [issue(1, extra)], pulls: [] };
    const ops = planOperations(
      plan({ ungrouped: [{ issue: 1, reason: "r" }] }),
      live,
      NOW,
    );
    assert.ok(ops.drops.includes(`ungrouped #1: ${why}`));
  });
}

test("a link of an issue that is not open is dropped", () => {
  const live = { issues: [bot(500, [3, 4])], pulls: [] };
  const ops = planOperations(
    plan({ links: [{ issue: 1, feature: 500 }] }),
    live,
    NOW,
  );
  assert.ok(ops.drops.includes("link #1 to #500: the issue is not open"));
});

test("an update to a Feature that closes this run is dropped", () => {
  const done = bot(500, []);
  done.children = [1, 2].map((n) => ({
    id: n * 10,
    number: n,
    title: "t",
    state: "CLOSED",
    type: "Task",
  }));
  const update = {
    feature: 500,
    title: "t",
    outcome: "o",
    why: "w",
    dependsOn: [],
  };
  const ops = planOperations(
    plan({ featureUpdates: [update] }),
    { issues: [done], pulls: [] },
    NOW,
  );
  assert.deepEqual(ops.updates, []);
  assert.ok(ops.drops.includes("update #500: the Feature closes this run"));
});

test("ungrouped entries need an open issue with no parent", () => {
  const live = { issues: [issue(2, { parent: 880 })], pulls: [] };
  const ops = planOperations(
    plan({
      ungrouped: [
        { issue: 1, reason: "r" },
        { issue: 2, reason: "r" },
      ],
    }),
    live,
    NOW,
  );
  assert.ok(ops.drops.includes("ungrouped #1: the issue is not open"));
  assert.ok(ops.drops.includes("ungrouped #2: the issue has a parent"));
  assert.deepEqual(ops.ungrouped, []);
});

test("a dropped move gives its child back, so the source is not closed as done", () => {
  const source = bot(500, [1]);
  source.children.push({
    id: 20,
    number: 2,
    title: "t",
    state: "CLOSED",
    type: "Task",
  });
  const live = {
    issues: [source, issue(1, { parent: 500, parentAddedAt: OLD })],
    pulls: [],
  };
  const ops = planOperations(
    plan({
      features: [feature("new-1")],
      moves: [{ issue: 1, to: "new-1", reason: "fits" }],
    }),
    live,
    NOW,
  );
  assert.deepEqual(ops.closes, []);
});

test("after dropping one reshape, the settle pass still forces later ones", () => {
  const live = {
    issues: [
      bot(500, [1, 2]),
      bot(600, [4, 5]),
      issue(1, { parent: 500, parentAddedAt: OLD }),
      issue(2, { parent: 500, parentAddedAt: NEW }),
      issue(4, { parent: 600, parentAddedAt: OLD }),
      issue(5, { parent: 600, parentAddedAt: OLD }),
    ],
    pulls: [],
  };
  const ops = planOperations(
    plan({
      moves: [
        { issue: 1, to: null, reason: "fits nowhere" },
        { issue: 4, to: null, reason: "fits nowhere" },
      ],
    }),
    live,
    NOW,
  );
  assert.deepEqual(
    ops.moves.map((m) => [m.issue, m.from, m.to]),
    [
      [4, 600, null],
      [5, 600, null],
    ],
  );
  assert.deepEqual(ops.closes, [{ issue: 600, reason: "not_planned" }]);
});

test("a dropped move also leaves its target, so a new Feature can fall below two", () => {
  const live = {
    issues: [
      bot(500, [1, 2]),
      issue(1, { parent: 500, parentAddedAt: OLD }),
      issue(2, { parent: 500, parentAddedAt: NEW }),
      issue(3),
    ],
    pulls: [],
  };
  const ops = planOperations(
    plan({
      features: [feature("new-1")],
      moves: [{ issue: 1, to: "new-1", reason: "fits" }],
      links: [{ issue: 3, feature: "new-1" }],
    }),
    live,
    NOW,
  );
  assert.deepEqual(ops.creates, []);
  assert.deepEqual(ops.links, []);
  assert.ok(
    ops.drops.includes(
      "feature new-1: 1 children; a Feature needs two, or one when marked single",
    ),
  );
});

test("links into a Feature dropped for the cap say so", () => {
  const issues = Array.from({ length: 12 }, (_, i) => issue(i + 1));
  const features = Array.from({ length: 6 }, (_, i) => feature(`new-${i + 1}`));
  const links = Array.from({ length: 12 }, (_, i) => ({
    issue: i + 1,
    feature: `new-${Math.floor(i / 2) + 1}`,
  }));
  const ops = planOperations(
    plan({ features, links }),
    { issues, pulls: [] },
    NOW,
  );
  assert.ok(
    ops.drops.includes("link #11 to new-6: its Feature was over the cap"),
  );
  assert.ok(
    ops.drops.includes("link #12 to new-6: its Feature was over the cap"),
  );
});

test("a move into a Feature dropped for the cap says so", () => {
  const issues = [
    ...Array.from({ length: 10 }, (_, i) => issue(i + 1)),
    bot(500, [11, 12, 13]),
    issue(11, { parent: 500, parentAddedAt: OLD }),
  ];
  const features = Array.from({ length: 6 }, (_, i) => feature(`new-${i + 1}`));
  const links = Array.from({ length: 10 }, (_, i) => ({
    issue: i + 1,
    feature: `new-${Math.floor(i / 2) + 1}`,
  }));
  const ops = planOperations(
    plan({
      features,
      links,
      moves: [{ issue: 11, to: "new-6", reason: "fits" }],
    }),
    { issues, pulls: [] },
    NOW,
  );
  assert.ok(
    ops.drops.includes("move #11 to new-6: its Feature was over the cap"),
  );
});

test("a Feature starved by the link cap drops its other links as capacity", () => {
  const issues = Array.from({ length: 41 }, (_, i) => issue(i + 1));
  const features = [feature("new-1"), feature("new-2")];
  const links = [
    ...Array.from({ length: 39 }, (_, i) => ({
      issue: i + 1,
      feature: "new-1",
    })),
    { issue: 40, feature: "new-2" },
    { issue: 41, feature: "new-2" },
  ];
  const ops = planOperations(
    plan({ features, links }),
    { issues, pulls: [] },
    NOW,
  );
  assert.ok(ops.drops.includes("link #41 to new-2: over the cap of 40 links"));
  assert.ok(
    ops.drops.includes("link #40 to new-2: its Feature was over the cap"),
  );
});

test("a Feature starved by the move cap drops its link as capacity", () => {
  const children = [10, 11, 12, 13, 14, 15, 16];
  const issues = [
    bot(100, children),
    ...children.map((n) => issue(n, { parent: 100, parentAddedAt: OLD })),
    issue(1),
  ];
  const moves = [10, 11, 12, 13, 14].map((n) => ({
    issue: n,
    to: null,
    reason: "fits nowhere",
  }));
  const ops = planOperations(
    plan({
      features: [feature("new-1")],
      moves: [...moves, { issue: 16, to: "new-1", reason: "fits" }],
      links: [{ issue: 1, feature: "new-1" }],
    }),
    { issues, pulls: [] },
    NOW,
  );
  assert.ok(ops.drops.includes("move #16 to new-1: over the cap of 5 moves"));
  assert.ok(
    ops.drops.includes("link #1 to new-1: its Feature was over the cap"),
  );
});

test("a Feature starved when the settle pass drops a move for the cap counts as capacity", () => {
  const issues = [
    bot(500, [1, 2]),
    bot(600, [3, 4, 5, 6, 7, 8]),
    ...[1, 2].map((n) => issue(n, { parent: 500, parentAddedAt: OLD })),
    ...[3, 4, 5, 6, 7, 8].map((n) =>
      issue(n, { parent: 600, parentAddedAt: OLD }),
    ),
    issue(9),
  ];
  const ops = planOperations(
    plan({
      features: [feature("new-1")],
      moves: [
        { issue: 1, to: "new-1", reason: "fits" },
        ...[3, 4, 5, 6].map((n) => ({
          issue: n,
          to: null,
          reason: "fits nowhere",
        })),
      ],
      links: [{ issue: 9, feature: "new-1" }],
    }),
    { issues, pulls: [] },
    NOW,
  );
  assert.ok(
    ops.drops.includes(
      "move #1 to new-1: moving #2 out of #500 would pass the cap of 5 moves",
    ),
  );
  assert.ok(
    ops.drops.includes("link #9 to new-1: its Feature was over the cap"),
  );
});
