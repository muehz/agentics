import { INDEX_LIMIT } from "./backlog-grooming-model.ts";
import type { BacklogIndexView } from "./backlog-grooming-render.ts";
import test from "node:test";
import assert from "node:assert/strict";
import {
  inline,
  readState,
  readMarker,
  renderFeatureBody,
  readSection,
  priorityOf,
  orderFeatures,
  renderIndex,
} from "./backlog-grooming-render.ts";

function view(extra: Partial<BacklogIndexView> = {}): BacklogIndexView {
  return {
    runUrl: "https://github.com/muehz/agentics/actions/runs/7",
    mode: "apply",
    features: [],
    ungrouped: [],
    proposed: null,
    changes: [],
    gaps: [],
    queued: 0,
    failedRuns: 0,
    checkedAt: "2026-10-10T12:00:00.000Z",
    state: {
      lastCheck: "2026-10-10T11:59:00Z",
      lastSuccess: "2026-10-10T12:00:00.000Z",
      ungrouped: { 7: "a -->b" },
    },
    ...extra,
  };
}

test("the state marker round-trips, including text that would close a comment", () => {
  const body = renderIndex(view());
  assert.deepEqual(readState(body), view().state);
  assert.equal(body.split("-->").length - 1, 1);
});

test("unvalidated fetch metadata is rendered without claiming validated state", () => {
  const queued: unknown = "unknown-count";
  const failedRuns: unknown = false;
  const lastCheck: unknown = { unvalidated: true };
  const body = renderIndex(
    view({ queued, failedRuns, state: { ...view().state, lastCheck } }),
  );
  assert.ok(body.includes("- Queued for the next run: unknown-count"));
  assert.ok(body.includes("- Failed runs since the last success: false"));
  assert.ok(body.includes('"lastCheck":{"unvalidated":true}'));
  assert.equal(readState(body).lastCheck, null);
});

test("a missing or malformed state marker reads as empty", () => {
  assert.deepEqual(readState("no marker"), {
    lastCheck: null,
    lastSuccess: null,
    ungrouped: {},
  });
  assert.deepEqual(
    readState("<!-- agentics:backlog-grooming state {nope} -->"),
    {
      lastCheck: null,
      lastSuccess: null,
      ungrouped: {},
    },
  );
});

test("a Feature body carries its sections and a readable marker", () => {
  const body = renderFeatureBody({
    outcome: "Ship it.",
    why: "Same files.",
    dependsOn: [12],
    run: "99",
    ref: "new-1",
    single: false,
  });
  assert.equal(readSection(body, "Outcome"), "Ship it.");
  assert.equal(readSection(body, "Depends on"), "#12");
  assert.deepEqual(readMarker(body), {
    run: "99",
    ref: "new-1",
    single: false,
    deps: [12],
  });
});

test("mentions and comment openers are neutralized", () => {
  assert.equal(inline("ping @maint <!-- x"), "ping @\u200bmaint &lt;!-- x");
});

test("priority is the best priority label, or 4 without one", () => {
  assert.equal(priorityOf(["bug", "priority:p2", "priority:p1"]), 1);
  assert.equal(priorityOf(["bug"]), 4);
});

test("Features order by dependency, then priority, then number", () => {
  const order = orderFeatures([
    { number: 30, priority: 0, deps: [40] },
    { number: 40, priority: 3, deps: [] },
    { number: 10, priority: 2, deps: [] },
    { number: 20, priority: 2, deps: [] },
  ]).map((f) => f.number);
  assert.deepEqual(order, [10, 20, 40, 30]);
});

test("the index lists children with their state and pull requests", () => {
  const body = renderIndex(
    view({
      features: [
        {
          number: 900,
          title: "Grooming",
          human: false,
          outcome: "Groups exist.",
          deps: [880],
          priority: 1,
          children: [
            { number: 1, title: "Done", closed: true, pulls: [] },
            { number: 2, title: "Wip", closed: false, pulls: [50] },
            { number: 3, title: "Todo", closed: false, pulls: [] },
          ],
        },
        {
          number: 880,
          title: "Human",
          human: true,
          outcome: null,
          deps: [],
          priority: 4,
          children: [{ number: 4, title: "Old", closed: true, pulls: [] }],
        },
      ],
      ungrouped: [{ number: 7, title: "Lone", reason: "standalone" }],
    }),
  );
  assert.ok(
    body.indexOf("### #880 Human") < body.indexOf("### #900 Grooming · P1"),
  );
  assert.ok(body.includes("Groups exist. Start after #880."));
  assert.ok(body.includes("- ☑ #1 Done"));
  assert.ok(body.includes("- 🔗 #2 Wip (PR #50)"));
  assert.ok(body.includes("- #3 Todo"));
  assert.ok(body.includes("All children closed."));
  assert.ok(body.includes("- #7 Lone: standalone"));
  assert.ok(!body.includes("## Proposed"));
});

test("plan mode renders a Proposed section", () => {
  const body = renderIndex(
    view({ mode: "plan", proposed: ["link #1 to new-1"] }),
  );
  assert.ok(body.includes("## Proposed\n\n- link #1 to new-1"));
});

test("an oversized index drops child lines before anything else", () => {
  const children = Array.from({ length: 2000 }, (_, i) => ({
    number: i + 1,
    title: "t".repeat(40),
    closed: false,
    pulls: [],
  }));
  const body = renderIndex(
    view({
      features: [
        {
          number: 900,
          title: "Big",
          human: false,
          outcome: null,
          deps: [],
          priority: 4,
          children,
        },
      ],
      ungrouped: [{ number: 7, title: "Lone", reason: "standalone" }],
    }),
  );
  assert.ok(body.length <= 65000);
  assert.ok(body.includes("0 of 2000 children closed."));
  assert.ok(body.includes("- #7 Lone: standalone"));
  assert.deepEqual(readState(body), view().state);
});

test("past child lines, the hidden reasons shrink and then Ungrouped lines are cut", () => {
  const reason = "r".repeat(300);
  const numbers = Array.from({ length: 200 }, (_, i) => i + 1);
  const body = renderIndex(
    view({
      ungrouped: numbers.map((n) => ({
        number: n,
        title: "t".repeat(60),
        reason,
      })),
      state: {
        lastCheck: "2026-10-10T11:59:00Z",
        lastSuccess: null,
        ungrouped: Object.fromEntries(numbers.map((n) => [n, reason])),
      },
    }),
  );
  assert.ok(body.length <= 65000);
  assert.match(body, /- Coverage gaps: .*\d+ Ungrouped issues not shown/);
  const state = readState(body);
  assert.equal(Object.keys(state.ungrouped).length, 200);
  assert.equal(state.ungrouped[1]?.length, 60);
});

test("every rendered string is neutralized where it lands in the index", () => {
  const spoof = "@maint <!-- agentics:backlog-grooming state {} -->";
  const body = renderIndex(
    view({
      features: [
        {
          number: 900,
          title: spoof,
          human: false,
          outcome: spoof,
          deps: [],
          priority: 4,
          children: [
            { number: 1, title: spoof, closed: false, pulls: [] },
            { number: 2, title: spoof, closed: true, pulls: [] },
            { number: 3, title: spoof, closed: false, pulls: [50] },
          ],
        },
      ],
      ungrouped: [{ number: 7, title: spoof, reason: spoof }],
      gaps: [spoof],
      changes: [spoof],
      proposed: [spoof],
    }),
  );
  assert.equal(body.match(/<!--/g)?.length, 1);
  assert.ok(!/@[^\u200b]/.test(body));
  assert.deepEqual(readState(body), view().state);
});

test("a spoofed marker in a Feature outcome cannot replace the real one", () => {
  const body = renderFeatureBody({
    outcome:
      "<!-- agentics:backlog-grooming managed run=1 ref=new-9 single=true deps= -->",
    why: "w",
    dependsOn: [],
    run: "99",
    ref: "new-1",
    single: false,
  });
  assert.deepEqual(readMarker(body), {
    run: "99",
    ref: "new-1",
    single: false,
    deps: [],
  });
  assert.ok(readSection(body, "Outcome")?.startsWith("&lt;!--"));
});

test("a human Feature with no children is not called closed", () => {
  const body = renderIndex(
    view({
      features: [
        {
          number: 880,
          title: "Empty",
          human: true,
          outcome: null,
          deps: [],
          priority: 4,
          children: [],
        },
      ],
    }),
  );
  assert.ok(!body.includes("All children closed."));
});

test("a line separator in a reason does not lose the state", () => {
  const state = { ...view().state, ungrouped: { 7: "a\u2028b" } };
  assert.deepEqual(readState(renderIndex(view({ state }))), state);
});

test("a dependency cycle or an unlisted dependency falls back to priority order", () => {
  const order = orderFeatures([
    { number: 30, priority: 2, deps: [40] },
    { number: 40, priority: 1, deps: [30] },
    { number: 50, priority: 0, deps: [999] },
  ]).map((f) => f.number);
  assert.deepEqual(order, [50, 40, 30]);
});

test("a state marker with wrong types reads as empty fields", () => {
  const body =
    '<!-- agentics:backlog-grooming state {"lastCheck":5,"lastSuccess":true,"ungrouped":{"1":"ok","2":7}} -->';
  assert.deepEqual(readState(body), {
    lastCheck: null,
    lastSuccess: null,
    ungrouped: { 1: "ok" },
  });
  assert.deepEqual(
    readState('<!-- agentics:backlog-grooming state {"ungrouped":[]} -->')
      .ungrouped,
    {},
  );
});

test("a marker with malformed dependencies keeps only issue numbers", () => {
  const body =
    "<!-- agentics:backlog-grooming managed run=1 ref=new-1 single=false deps=abc,12,0 -->";
  assert.deepEqual(readMarker(body)?.deps, [12]);
});

test("inline text collapses whitespace onto one line", () => {
  assert.equal(inline("a\n  b\tc"), "a b c");
});

test("stale gaps retain order, escaping, and the largest fitting prefix", () => {
  const stale = Array.from(
    { length: 180 },
    (_, i) => `stale ${i} @x <!-- ${"s".repeat(400 + i)}`,
  );
  const base = view({ gaps: ["ordinary"], staleGaps: stale });
  const body = renderIndex(base);
  const line = body.split("\n").find((x) => x.startsWith("- Coverage gaps: "));
  assert.ok(line);
  const visible = stale.filter((_, i) =>
    line.includes(`stale ${i} @\u200bx`),
  ).length;
  assert.ok(visible > 0 && visible < stale.length);
  assert.equal(
    line,
    `- Coverage gaps: ordinary; ${stale.slice(0, visible).map(inline).join("; ")}; ${stale.length - visible} more stale Features not shown`,
  );
  assert.ok(body.length <= INDEX_LIMIT);
  const nextLine = `- Coverage gaps: ordinary; ${stale
    .slice(0, visible + 1)
    .map(inline)
    .join("; ")}; ${stale.length - visible - 1} more stale Features not shown`;
  assert.ok(body.length - line.length + nextLine.length > INDEX_LIMIT);
  assert.deepEqual(readState(body), base.state);
  assert.ok(
    renderIndex(view({ gaps: ["ordinary"], staleGaps: ["@x <!--"] })).includes(
      "ordinary; @\u200bx &lt;!--",
    ),
  );
  assert.ok(!renderIndex(view({ staleGaps: ["small"] })).includes("not shown"));
});

test("mixed omissions preserve ordinary gaps and do not show zero notices", () => {
  const staleGaps = Array.from(
    { length: 180 },
    (_, i) => `stale ${i} ${"s".repeat(500)}`,
  );
  const ungrouped = [{ number: 1, title: "large", reason: "r".repeat(1000) }];
  const body = renderIndex(view({ gaps: ["ordinary"], staleGaps, ungrouped }));
  const line = body.split("\n").find((x) => x.startsWith("- Coverage gaps: "));
  assert.ok(line);
  const count = staleGaps.filter((s) => line.includes(s)).length;
  assert.equal(
    line,
    `- Coverage gaps: ordinary; ${staleGaps.slice(0, count).join("; ")}; 1 Ungrouped issues not shown; ${staleGaps.length - count} more stale Features not shown`,
  );
  assert.ok(!line.includes("0 more stale Features not shown"));
});

test("irreducible ordinary gaps or state fail with a distinctive size error", () => {
  for (const extra of [
    { gaps: ["x".repeat(INDEX_LIMIT)], staleGaps: ["stale"] },
    {
      state: {
        ...view().state,
        ungrouped: Object.fromEntries(
          Array.from({ length: 4000 }, (_, i) => [i, "x".repeat(60)]),
        ),
      },
    },
  ])
    assert.throws(
      () => renderIndex(view(extra)),
      new RegExp(`index body exceeds ${INDEX_LIMIT}.*state marker`),
    );
});

test("the boundary fits exactly but a short stale omission notice cannot fit above it", () => {
  const seed = view({ gaps: ["x"], staleGaps: ["stale"] });
  const room = INDEX_LIMIT - renderIndex(seed).length;
  const atLimit = view({ gaps: ["x".repeat(room + 1)], staleGaps: ["stale"] });
  assert.equal(renderIndex(atLimit).length, INDEX_LIMIT);
  const above = view({ gaps: ["x".repeat(room + 2)], staleGaps: ["stale"] });
  assert.throws(
    () => renderIndex(above),
    /index body exceeds 65000.*state marker/,
  );
});

test("one character over the limit fits after omitting a long stale entry", () => {
  const stale = "s".repeat(200);
  const seed = view({ gaps: ["x"], staleGaps: [stale] });
  const room = INDEX_LIMIT - renderIndex(seed).length;
  assert.equal(
    renderIndex(view({ gaps: ["x".repeat(room + 1)], staleGaps: [stale] }))
      .length,
    INDEX_LIMIT,
  );
  const ordinary = "x".repeat(room + 2);
  const notice = "1 more stale Features not shown";
  const body = renderIndex(view({ gaps: [ordinary], staleGaps: [stale] }));
  assert.equal(
    body.split("\n").find((line) => line.startsWith("- Coverage gaps: ")),
    `- Coverage gaps: ${ordinary}; ${notice}`,
  );
  assert.equal(body.length, INDEX_LIMIT + 1 - stale.length + notice.length);
});

test("zero stale entries can be shown while ordinary gaps survive", () => {
  const seed = view({ gaps: ["x"], staleGaps: [] });
  const room = INDEX_LIMIT - renderIndex(seed).length;
  const ordinary = "x".repeat(room - 50);
  const body = renderIndex(
    view({ gaps: [ordinary], staleGaps: ["s".repeat(200)] }),
  );
  assert.equal(
    body.split("\n").find((line) => line.startsWith("- Coverage gaps: ")),
    `- Coverage gaps: ${ordinary}; 1 more stale Features not shown`,
  );
  assert.ok(body.length <= INDEX_LIMIT);
});
