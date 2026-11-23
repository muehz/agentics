// parsePlan is the only gate between agent text and the rules, so each
// rejection test asserts its distinctive message.
import test from "node:test";
import assert from "node:assert/strict";
import { parsePlan } from "./backlog-grooming-model.ts";

const feature = {
  ref: "new-1",
  title: "Grooming",
  outcome: "Groups exist.",
  why: "Shared files.",
  dependsOn: [],
};

test("an empty object is an empty plan", () => {
  const { plan, errors } = parsePlan("{}");
  assert.deepEqual(errors, []);
  assert.deepEqual(plan, {
    features: [],
    links: [],
    moves: [],
    featureUpdates: [],
    ungrouped: [],
  });
});

test("a valid plan parses with refs resolved", () => {
  const raw = JSON.stringify({
    features: [feature],
    links: [{ issue: 12, feature: "new-1" }],
    moves: [{ issue: 13, to: null, reason: "fits nowhere" }],
    ungrouped: [{ issue: 14, reason: "standalone" }],
    featureUpdates: [
      { feature: 900, title: "T", outcome: "O", why: "W", dependsOn: [5, 5] },
      { feature: 901, title: "T", outcome: "O", why: "W" },
    ],
  });
  const { plan, errors } = parsePlan(raw);
  assert.deepEqual(errors, []);
  assert.ok(plan);
  assert.ok(plan.links[0]);
  assert.ok(plan.moves[0]);
  assert.ok(plan.features[0]);
  assert.equal(plan.links[0].feature, "new-1");
  assert.equal(plan.moves[0].to, null);
  assert.equal(plan.features[0].single, false);
  assert.deepEqual(plan.featureUpdates, [
    { feature: 900, title: "T", outcome: "O", why: "W", dependsOn: [5] },
    { feature: 901, title: "T", outcome: "O", why: "W", dependsOn: [] },
  ]);
});

// Each case names one malformed input and the one message it must produce.
const rejections: [string, string, string][] = [
  ["a non-object plan", "[]", "plan must be a JSON object"],
  ["a non-array section", '{"features":{}}', "features must be an array"],
  ["a non-object entry", '{"features":[5]}', "features[0] must be an object"],
  [
    "a malformed ref",
    JSON.stringify({ features: [{ ...feature, ref: "new-0" }] }),
    "features[0].ref must match new-N",
  ],
  [
    "a non-boolean single",
    JSON.stringify({ features: [{ ...feature, single: "yes" }] }),
    "features[0].single must be a boolean",
  ],
  [
    "an overlong title",
    JSON.stringify({ features: [{ ...feature, title: "t".repeat(201) }] }),
    "features[0].title exceeds 200 characters",
  ],
  [
    "a fractional dependency",
    JSON.stringify({ features: [{ ...feature, dependsOn: [1.5] }] }),
    "features[0].dependsOn must be an array of issue numbers",
  ],
  [
    "a move to a string number",
    JSON.stringify({ moves: [{ issue: 13, to: "12", reason: "r" }] }),
    "moves[0].to must be an issue number, null, or a declared new-N ref",
  ],
  [
    "an update naming a ref",
    JSON.stringify({
      features: [feature],
      featureUpdates: [
        { feature: "new-1", title: "T", outcome: "O", why: "W" },
      ],
    }),
    "featureUpdates[0].feature must be an issue number",
  ],
  [
    "a Feature updated twice",
    JSON.stringify({
      featureUpdates: [
        { feature: 900, title: "T", outcome: "O", why: "W" },
        { feature: 900, title: "T", outcome: "O", why: "W" },
      ],
    }),
    "featureUpdates[1].feature #900 is updated twice",
  ],
  [
    "an unsafe issue number",
    JSON.stringify({ ungrouped: [{ issue: 1e300, reason: "r" }] }),
    "ungrouped[0].issue must be an issue number",
  ],
];

for (const [name, raw, message] of rejections) {
  test(`${name} is rejected`, () => {
    const { plan, errors } = parsePlan(raw);
    assert.equal(plan, null);
    assert.ok(errors.includes(message), `missing "${message}" in ${errors}`);
  });
}

test("a non-object entry reports one error, not one per field", () => {
  assert.deepEqual(parsePlan('{"features":[5]}').errors, [
    "features[0] must be an object",
  ]);
});

test("entries without valid issue numbers are not reported as duplicates", () => {
  const { errors } = parsePlan(
    JSON.stringify({ ungrouped: [{ reason: "r" }, { reason: "r" }] }),
  );
  assert.ok(!errors.some((e) => e.includes("#undefined")));
});

test("invalid JSON is rejected", () => {
  assert.deepEqual(parsePlan("{").errors, ["plan is not valid JSON"]);
});

test("an unknown key is rejected", () => {
  assert.ok(parsePlan('{"closes":[]}').errors.includes("unknown key closes"));
});

test("an undeclared ref is rejected", () => {
  const { errors } = parsePlan(
    JSON.stringify({ links: [{ issue: 12, feature: "new-9" }] }),
  );
  assert.ok(
    errors.includes(
      "links[0].feature must be an issue number or a declared new-N ref",
    ),
  );
});

test("a duplicate ref is rejected", () => {
  const { errors } = parsePlan(
    JSON.stringify({ features: [feature, feature] }),
  );
  assert.ok(errors.includes("features[1].ref new-1 is declared twice"));
});

test("a move without a reason is rejected", () => {
  const { errors } = parsePlan(
    JSON.stringify({ moves: [{ issue: 13, to: null, reason: " " }] }),
  );
  assert.ok(errors.includes("moves[0].reason must be a nonempty string"));
});

test("a multi-line title is rejected", () => {
  const { errors } = parsePlan(
    JSON.stringify({ features: [{ ...feature, title: "a\nb" }] }),
  );
  assert.ok(errors.includes("features[0].title must be one line"));
});

test("an issue placed twice is rejected", () => {
  const raw = JSON.stringify({
    features: [feature],
    links: [{ issue: 12, feature: "new-1" }],
    ungrouped: [{ issue: 12, reason: "standalone" }],
  });
  assert.ok(
    parsePlan(raw).errors.includes(
      "issue #12 appears in more than one of links, moves, and ungrouped",
    ),
  );
});

test("one bad entry rejects the whole plan", () => {
  const raw = JSON.stringify({
    features: [feature],
    links: [{ issue: 0, feature: "new-1" }],
  });
  const { plan, errors } = parsePlan(raw);
  assert.equal(plan, null);
  assert.ok(errors.includes("links[0].issue must be an issue number"));
});
