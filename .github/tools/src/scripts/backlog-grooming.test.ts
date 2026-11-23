import type { FetchMeta } from "./backlog-grooming.ts";
import type {
  BacklogIssue,
  BacklogSnapshot,
  GroomingGap,
} from "./backlog-grooming-model.ts";
import type { BacklogClient } from "./backlog-grooming-client.ts";
interface TestIssue extends Pick<
  BacklogIssue,
  | "number"
  | "title"
  | "body"
  | "state"
  | "labels"
  | "type"
  | "parent"
  | "parentAddedAt"
> {
  createdAt?: string;
  stateReason?: string;
}
type SeedIssue = Pick<TestIssue, "number" | "title"> & Partial<TestIssue>;
type Write =
  | ["label", string]
  | ["create", string]
  | ["update", number, string]
  | ["link", number, number, boolean]
  | ["unlink", number, number]
  | ["pin", string];
type GitHubFixture = {
  issues: Map<number, TestIssue>;
  writes: Write[];
  client: BacklogClient;
};
// End to end over an in-memory GitHub: the agent's plan goes in, and the
// issues, links, and index that come out are what the maintainer sees.
import test from "node:test";
import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import {
  applyRun,
  fetchRun,
  readAgentPlan,
  readFetchMeta,
  main,
} from "./backlog-grooming.ts";
import { batchIssueOutcome } from "./backlog-grooming-model.ts";
import { readState } from "./backlog-grooming-render.ts";

const NOW = Date.parse("2026-10-10T12:00:00Z");
const META = {
  fetchedAt: "2026-10-10T11:58:00.000Z",
  queued: 3,
  failedRuns: 1,
};

function fakeGitHub(
  seed: SeedIssue[],
  failOn: (
    op: Write[0],
    ...args: (string | number | boolean)[]
  ) => boolean = () => false,
): GitHubFixture {
  const issues = new Map<number, TestIssue>(
    seed.map((i) => [
      i.number,
      {
        labels: [],
        type: null,
        parent: null,
        parentAddedAt: null,
        body: "",
        state: "OPEN",
        ...i,
      },
    ]),
  );
  let next = 1000;
  const writes: Write[] = [];
  const log = (...entry: Write) => {
    const [op, ...args] = entry;
    if (failOn(op, ...args)) throw new Error(`${op} refused`);
    writes.push(entry);
  };
  const children = (n: number) =>
    [...issues.values()]
      .filter((c) => c.parent === n)
      .map((c) => ({
        id: c.number * 10,
        number: c.number,
        title: c.title,
        state: c.state,
        type: c.type,
      }));
  const snapshotIssue = (i: TestIssue): BacklogIssue => ({
    id: i.number * 10,
    number: i.number,
    title: i.title,
    body: i.body,
    state: "OPEN",
    labels: i.labels,
    type: i.type,
    parent: i.parent,
    parentAddedAt: i.parentAddedAt,
    createdAt:
      i.createdAt ??
      (i.labels.includes("agentics: feature")
        ? new Date(NOW).toISOString()
        : "2026-09-01T00:00:00Z"),
    updatedAt: "2026-09-02T00:00:00Z",
    comments: [],
    children: children(i.number),
  });
  // Each supplied id is constructed from the fixture issue number.
  const byId = (id: number) =>
    [...issues.values()].find((i) => i.number * 10 === id)!;
  return {
    issues,
    writes,
    client: {
      snapshot: (now) => ({
        now,
        issues: [...issues.values()]
          .filter((i) => i.state === "OPEN")
          .map(snapshotIssue),
        pulls: [],
      }),
      failedRunsSinceSuccess: () => 1,
      issue: (n) => ({
        state: issues.get(n)!.state,
        parent: issues.get(n)!.parent,
        type: issues.get(n)!.type,
      }),
      ensureLabel: (name) => log("label", name),
      createIssue: ({ title, body, labels }) => {
        log("create", title);
        next += 1;
        issues.set(next, {
          number: next,
          title,
          body,
          labels,
          type: null,
          parent: null,
          parentAddedAt: null,
          state: "OPEN",
        });
        return { number: next, id: next * 10, nodeId: `I_${next}` };
      },
      updateIssue: (n, fields) => {
        log("update", n, Object.keys(fields).sort().join(","));
        const issue = issues.get(n)!;
        if (fields.type) issue.type = fields.type;
        if (fields.body !== undefined) issue.body = fields.body;
        if (fields.title !== undefined) issue.title = fields.title;
        if (fields.state === "closed") {
          issue.state = "CLOSED";
          issue.stateReason = fields.state_reason;
        }
      },
      addSubIssue: (parent, id, replace) => {
        log("link", byId(id).number, parent, replace);
        byId(id).parent = parent;
      },
      pinIssue: (nodeId) => log("pin", nodeId),
      removeSubIssue: (parent, id) => {
        log("unlink", byId(id).number, parent);
        byId(id).parent = null;
      },
    },
  };
}

const PLAN = JSON.stringify({
  features: [
    {
      ref: "new-1",
      title: "Grooming",
      outcome: "Groups exist.",
      why: "Same files.",
      dependsOn: [],
    },
  ],
  links: [
    { issue: 1, feature: "new-1" },
    { issue: 2, feature: "new-1" },
  ],
  ungrouped: [{ issue: 3, reason: "standalone" }],
});

function run(github: GitHubFixture, mode: "apply" | "plan", rawPlan = PLAN) {
  return applyRun({
    mode,
    rawPlan,
    meta: META,
    client: github.client,
    runId: "77",
    runUrl: "https://github.com/muehz/agentics/actions/runs/77",
    now: NOW,
  });
}

const seed = () => [
  { number: 1, title: "One" },
  { number: 2, title: "Two", type: "Bug" },
  { number: 3, title: "Three" },
];

test("apply mode creates the Feature, links its children, and writes the index", () => {
  const github = fakeGitHub(seed());
  run(github, "apply");
  const feature = github.issues.get(1001)!;
  assert.equal(feature.type, "Feature");
  assert.deepEqual(feature.labels, ["agentics: feature"]);
  assert.equal(github.issues.get(1)!.parent, 1001);
  assert.equal(github.issues.get(1)!.type, "Task");
  assert.equal(github.issues.get(2)!.type, "Bug");
  const index = github.issues.get(1002)!;
  assert.deepEqual(index.labels, ["agentics: backlog-index"]);
  assert.ok(index.body.includes("### #1001 Grooming"));
  assert.ok(index.body.includes("- #3 Three: standalone"));
  assert.ok(index.body.includes("- Queued for the next run: 3"));
  assert.deepEqual(readState(index.body).ungrouped, { 3: "standalone" });
  assert.equal(readState(index.body).lastCheck, META.fetchedAt);
  assert.equal(readState(index.body).lastSuccess, new Date(NOW).toISOString());
  assert.ok(github.writes.some((w) => w[0] === "pin" && w[1] === "I_1002"));
});

test("an existing index is updated in place and not pinned again", () => {
  const github = fakeGitHub(seed());
  run(github, "apply");
  run(github, "apply");
  assert.equal(github.writes.filter((w) => w[0] === "pin").length, 1);
  assert.equal(
    [...github.issues.values()].filter((i) =>
      i.labels.includes("agentics: backlog-index"),
    ).length,
    1,
  );
});

test("a failed pin does not fail the run", () => {
  const github = fakeGitHub(seed(), (op) => op === "pin");
  assert.doesNotThrow(() => run(github, "apply"));
  assert.ok(github.issues.get(1002)!.body.includes("# Backlog index"));
});

test("plan mode writes only the index, with a Proposed section", () => {
  const github = fakeGitHub(seed());
  const { body } = run(github, "plan");
  assert.deepEqual(
    github.writes.map((w) => w[0]),
    ["label", "create", "pin"],
  );
  assert.ok(body.includes("## Proposed"));
  assert.ok(body.includes('- create new-1 "Grooming"'));
  assert.equal(readState(body).lastCheck, null);
});

test("a rerun after a partial apply adopts the Feature it already created", () => {
  let failLinks = true;
  const github = fakeGitHub(seed(), (op) => op === "link" && failLinks);
  run(github, "apply");
  assert.equal(github.issues.get(1)!.parent, null);
  failLinks = false;
  run(github, "apply");
  assert.equal(
    github.writes.filter((w) => w[0] === "create" && w[1] === "Grooming")
      .length,
    1,
  );
  assert.equal(github.issues.get(1)!.parent, 1001);
  assert.equal(github.issues.get(2)!.parent, 1001);
  assert.equal(github.issues.get(1001)!.state, "OPEN");
});

test("a Feature whose type fails to set still receives its links", () => {
  const github = fakeGitHub(
    seed(),
    (op, n, fields) => op === "update" && n === 1001 && fields === "type",
  );
  const { body } = run(github, "apply");
  assert.equal(github.issues.get(1)!.parent, 1001);
  assert.equal(github.issues.get(2)!.parent, 1001);
  assert.ok(body.includes("type #1001 as Feature failed: update refused"));
});

test("a rejected plan writes nothing but the index and reports why", () => {
  const github = fakeGitHub(seed());
  const { body } = run(github, "apply", "{");
  assert.deepEqual(
    github.writes.map((w) => w[0]),
    ["label", "create", "pin"],
  );
  assert.ok(body.includes("plan rejected: plan is not valid JSON"));
  assert.equal(readState(body).lastSuccess, null);
});

test("a failed write is reported and the rest of the plan still applies", () => {
  const github = fakeGitHub(seed(), (op, n) => op === "link" && n === 1);
  const { body } = run(github, "apply");
  assert.equal(github.issues.get(2)!.parent, 1001);
  assert.ok(body.includes("link #1 failed: link refused"));
});

test("a link whose issue gained a parent after the plan was made is skipped", () => {
  const github = fakeGitHub([
    ...seed(),
    { number: 880, title: "Human", type: "Feature" },
  ]);
  const original = github.client.issue;
  github.client.issue = (n) =>
    n === 1 ? { ...original(n), parent: 880 } : original(n);
  const { body } = run(github, "apply");
  assert.equal(github.issues.get(1)!.parent, null);
  assert.ok(body.includes("link #1 skipped: it changed since the fetch"));
});

test("two index issues stop the run before any write", () => {
  const github = fakeGitHub([
    { number: 1, title: "A", labels: ["agentics: backlog-index"] },
    { number: 2, title: "B", labels: ["agentics: backlog-index"] },
  ]);
  assert.throws(
    () => run(github, "apply"),
    /found 2 open issues labeled agentics: backlog-index/,
  );
  assert.deepEqual(github.writes, []);
});

test("fetch selects the batch against the index state", () => {
  const github = fakeGitHub(seed());
  const { input, meta } = fetchRun({ client: github.client, now: NOW });
  assert.deepEqual(
    input.batch.map((i) => i.number),
    [1, 2, 3],
  );
  assert.equal(meta.queued, 0);
  assert.equal(meta.failedRuns, 1);
});

test("the agent output yields the one apply_grooming plan", () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "bg-"));
  const file = path.join(dir, "out.json");
  fs.writeFileSync(
    file,
    JSON.stringify({
      items: [{ type: "noop" }, { type: "apply_grooming", plan: "{}" }],
    }),
  );
  assert.equal(readAgentPlan(file), "{}");
  fs.writeFileSync(file, JSON.stringify({ items: [] }));
  assert.equal(readAgentPlan(file), null);
});

import { renderFeatureBody } from "./backlog-grooming-render.ts";

const OLD = new Date(NOW - 8 * 24 * 60 * 60 * 1000).toISOString();

function botFeature(number: number, extra: Partial<SeedIssue> = {}): SeedIssue {
  return {
    number,
    title: `Feature ${number}`,
    labels: ["agentics: feature"],
    type: "Feature",
    body: renderFeatureBody({
      outcome: "o",
      why: "w",
      dependsOn: [],
      run: "5",
      ref: "new-3",
      single: false,
    }),
    ...extra,
  };
}

const child = (number: number, parent: number): SeedIssue => ({
  number,
  title: `Child ${number}`,
  parent,
  parentAddedAt: OLD,
  type: "Task",
});

test("a rerun after one failed link links the rest under the adopted Feature", () => {
  let failSecond = true;
  const github = fakeGitHub(
    seed(),
    (op, n) => op === "link" && n === 2 && failSecond,
  );
  run(github, "apply");
  assert.equal(github.issues.get(1)!.parent, 1001);
  assert.equal(github.issues.get(2)!.parent, null);
  failSecond = false;
  const { body } = run(github, "apply");
  assert.equal(github.issues.get(2)!.parent, 1001);
  assert.equal(github.issues.get(1001)!.state, "OPEN");
  assert.equal(
    github.writes.filter((w) => w[0] === "create" && w[1] === "Grooming")
      .length,
    1,
  );
  assert.ok(body.includes("adopted #1001 Grooming"));
});

test("a rerun whose plan names a different grouping does not adopt", () => {
  const github = fakeGitHub([
    ...seed(),
    { number: 4, title: "Four" },
    { number: 5, title: "Five" },
  ]);
  run(github, "apply");
  const other = JSON.stringify({
    features: [
      {
        ref: "new-1",
        title: "Something else",
        outcome: "Other.",
        why: "Other files.",
        dependsOn: [],
      },
    ],
    links: [
      { issue: 4, feature: "new-1" },
      { issue: 5, feature: "new-1" },
    ],
  });
  run(github, "apply", other);
  const created = github.writes.filter((w) => w[0] === "create");
  assert.ok(created.some((w) => w[1] === "Something else"));
  assert.notEqual(github.issues.get(4)!.parent, 1001);
  assert.equal(github.issues.get(4)!.parent, github.issues.get(5)!.parent);
});

test("a move to Ungrouped unlinks the child and records it", () => {
  const github = fakeGitHub([
    botFeature(500),
    child(1, 500),
    child(2, 500),
    child(3, 500),
  ]);
  const { body } = run(
    github,
    "apply",
    JSON.stringify({ moves: [{ issue: 1, to: null, reason: "fits nowhere" }] }),
  );
  assert.ok(github.writes.some((w) => w[0] === "unlink" && w[1] === 1));
  assert.equal(github.issues.get(1)!.parent, null);
  assert.deepEqual(readState(body).ungrouped, { 1: "fits nowhere" });
  assert.ok(body.includes("- #1 Child 1: fits nowhere"));
});

test("a move between bot Features replaces the parent", () => {
  const github = fakeGitHub([
    botFeature(500),
    botFeature(600),
    child(1, 500),
    child(2, 500),
    child(3, 500),
    child(4, 600),
    child(5, 600),
  ]);
  run(
    github,
    "apply",
    JSON.stringify({ moves: [{ issue: 1, to: 600, reason: "fits" }] }),
  );
  assert.ok(
    github.writes.some(
      (w) => w[0] === "link" && w[1] === 1 && w[2] === 600 && w[3] === true,
    ),
  );
  assert.equal(github.issues.get(1)!.parent, 600);
});

test("a move whose child a person reparented after the fetch is skipped", () => {
  const github = fakeGitHub([
    botFeature(500),
    child(1, 500),
    child(2, 500),
    child(3, 500),
    { number: 880, title: "Human", type: "Feature" },
  ]);
  const original = github.client.issue;
  github.client.issue = (n) =>
    n === 1 ? { ...original(n), parent: 880 } : original(n);
  const { body } = run(
    github,
    "apply",
    JSON.stringify({ moves: [{ issue: 1, to: null, reason: "x" }] }),
  );
  assert.ok(body.includes("move #1 skipped: it changed since the fetch"));
  assert.ok(!github.writes.some((w) => w[0] === "unlink" || w[0] === "link"));
});

test("a Feature update keeps the Feature's own marker", () => {
  const github = fakeGitHub([botFeature(500), child(1, 500), child(2, 500)]);
  run(
    github,
    "apply",
    JSON.stringify({
      featureUpdates: [
        {
          feature: 500,
          title: "Renamed",
          outcome: "New outcome.",
          why: "New why.",
          dependsOn: [],
        },
      ],
    }),
  );
  const feature = github.issues.get(500)!;
  assert.equal(feature.title, "Renamed");
  assert.ok(feature.body.includes("New outcome."));
  assert.ok(
    feature.body.includes(
      "<!-- agentics:backlog-grooming managed run=5 ref=new-3 single=false deps= -->",
    ),
  );
});

test("a finished bot Feature closes with its reason", () => {
  const github = fakeGitHub([
    botFeature(500),
    { ...child(1, 500), state: "CLOSED" },
    { ...child(2, 500), state: "CLOSED" },
  ]);
  const { body } = run(github, "apply", "{}");
  assert.equal(github.issues.get(500)!.state, "CLOSED");
  assert.equal(github.issues.get(500)!.stateReason, "completed");
  assert.ok(body.includes("closed #500 as completed"));
});

test("an issue linked this run leaves the carried-forward Ungrouped state", () => {
  const github = fakeGitHub([
    ...seed(),
    {
      number: 900,
      title: "Backlog index",
      labels: ["agentics: backlog-index"],
      body: '<!-- agentics:backlog-grooming state {"lastCheck":null,"lastSuccess":null,"ungrouped":{"1":"old","3":"old"}} -->',
    },
  ]);
  const { body } = run(github, "apply");
  assert.deepEqual(readState(body).ungrouped, { 3: "standalone" });
});

test("a link to a Feature whose creation failed is skipped and reported", () => {
  const github = fakeGitHub(
    seed(),
    (op, title) => op === "create" && title === "Grooming",
  );
  const { body } = run(github, "apply");
  assert.ok(body.includes("link #1 skipped: new-1 was not created"));
  assert.ok(!github.writes.some((w) => w[0] === "link"));
});

test("rule drops reach the index", () => {
  const github = fakeGitHub([
    ...seed(),
    { number: 880, title: "Human", type: "Feature" },
  ]);
  const { body } = run(
    github,
    "apply",
    JSON.stringify({ links: [{ issue: 3, feature: 880 }] }),
  );
  assert.ok(
    body.includes(
      "link #3 to #880: the target is not a Feature this workflow created",
    ),
  );
});

test("plan mode labels only the index", () => {
  const github = fakeGitHub(seed());
  run(github, "plan");
  assert.deepEqual(github.writes[0], ["label", "agentics: backlog-index"]);
});

test("staged safe outputs make apply a no-op before reading anything", () => {
  assert.doesNotThrow(() =>
    main(["node", "script", "apply"], {
      GH_REPO: "muehz/agentics",
      GH_AW_SAFE_OUTPUTS_STAGED: "true",
    }),
  );
});

test("an unknown mode is rejected", () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "bg-"));
  const file = path.join(dir, "out.json");
  fs.writeFileSync(
    file,
    JSON.stringify({ items: [{ type: "apply_grooming", plan: "{}" }] }),
  );
  assert.throws(
    () =>
      main(["node", "script", "apply"], {
        GH_REPO: "muehz/agentics",
        GH_AW_AGENT_OUTPUT: file,
        MODE: "bogus",
      }),
    /MODE must be plan or apply/,
  );
});

test("malformed agent output is rejected with a named reason", () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "bg-"));
  const file = path.join(dir, "out.json");
  const item = { type: "apply_grooming", plan: "{}" };
  fs.writeFileSync(file, JSON.stringify({ items: [item, item] }));
  assert.throws(() => readAgentPlan(file), /multiple apply_grooming items/);
  fs.writeFileSync(
    file,
    JSON.stringify({ items: [{ type: "apply_grooming", plan: {} }] }),
  );
  assert.throws(
    () => readAgentPlan(file),
    /apply_grooming plan must be a string/,
  );
  fs.writeFileSync(file, "null");
  assert.throws(
    () => readAgentPlan(file),
    /GH_AW_AGENT_OUTPUT is not a JSON object/,
  );
});

test("negative: fetch metadata whose batch is not issue numbers is rejected", () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "bg-"));
  const file = path.join(dir, "fetch.json");
  for (const batch of [
    ["1"],
    [".*"],
    ["__proto__"],
    [1.5],
    [0],
    [-3],
    [null],
  ]) {
    fs.writeFileSync(file, JSON.stringify({ ...META, batch }));
    assert.throws(
      () => readFetchMeta(file),
      /FETCH_META batch must hold issue numbers/,
      JSON.stringify(batch),
    );
  }
  fs.writeFileSync(file, JSON.stringify({ ...META, batch: "1" }));
  assert.throws(() => readFetchMeta(file), /FETCH_META batch must be an array/);
  for (const text of ["null", "[]", '"meta"']) {
    fs.writeFileSync(file, text);
    assert.throws(() => readFetchMeta(file), /FETCH_META is not a JSON object/);
  }
});

test("fetch metadata with issue numbers, or with no batch, is read as written", () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "bg-"));
  const file = path.join(dir, "fetch.json");
  fs.writeFileSync(file, JSON.stringify({ ...META, batch: [1, 22] }));
  assert.deepEqual(readFetchMeta(file), { ...META, batch: [1, 22] });
  fs.writeFileSync(file, JSON.stringify(META));
  assert.deepEqual(readFetchMeta(file), META);
});

test("negative: apply refuses malformed fetch metadata before any write", () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "bg-"));
  const output = path.join(dir, "out.json");
  const meta = path.join(dir, "fetch.json");
  fs.writeFileSync(
    output,
    JSON.stringify({ items: [{ type: "apply_grooming", plan: "{}" }] }),
  );
  fs.writeFileSync(meta, JSON.stringify({ ...META, batch: ["1) #(.*"] }));
  const github = fakeGitHub([]);
  assert.throws(
    () =>
      main(
        ["node", "script", "apply"],
        {
          GH_REPO: "muehz/agentics",
          GH_AW_AGENT_OUTPUT: output,
          MODE: "apply",
          FETCH_META: meta,
          GITHUB_RUN_ID: "1",
          GITHUB_SERVER_URL: "https://github.com",
        },
        () => github.client,
      ),
    /FETCH_META batch must hold issue numbers/,
  );
  assert.deepEqual(github.writes, []);
});

test("a Feature left by a different run is never adopted", () => {
  const github = fakeGitHub([
    ...seed(),
    {
      number: 700,
      title: "Grooming",
      labels: ["agentics: feature"],
      type: "Feature",
      body: renderFeatureBody({
        outcome: "o",
        why: "w",
        dependsOn: [],
        run: "4",
        ref: "new-1",
        single: false,
      }),
    },
  ]);
  run(github, "apply");
  assert.ok(
    github.writes.some((w) => w[0] === "create" && w[1] === "Grooming"),
  );
  assert.notEqual(github.issues.get(1)!.parent, 700);
});

test("a converged link on rerun is not reported as a gap", () => {
  let failSecond = true;
  const github = fakeGitHub(
    seed(),
    (op, n) => op === "link" && n === 2 && failSecond,
  );
  run(github, "apply");
  failSecond = false;
  const { body } = run(github, "apply");
  assert.ok(!body.includes("already has parent"));
});

test("a rerun's move into the adopted Feature is kept", () => {
  let failLinks = true;
  const github = fakeGitHub(
    [...seed(), botFeature(500), child(7, 500), child(8, 500), child(9, 500)],
    (op) => op === "link" && failLinks,
  );
  const planWithMove = JSON.stringify({
    features: [
      {
        ref: "new-1",
        title: "Grooming",
        outcome: "Groups exist.",
        why: "Same files.",
        dependsOn: [],
      },
    ],
    links: [
      { issue: 1, feature: "new-1" },
      { issue: 2, feature: "new-1" },
    ],
    moves: [{ issue: 7, to: "new-1", reason: "belongs with the new group" }],
  });
  run(github, "apply", planWithMove);
  failLinks = false;
  run(github, "apply", planWithMove);
  assert.equal(github.issues.get(7)!.parent, 1001);
});

test("an adopted Feature is retyped only when its type is missing", () => {
  const typeWrites = (github: GitHubFixture, n: number) =>
    github.writes.filter(
      (w) => w[0] === "update" && w[1] === n && w[2] === "type",
    ).length;
  let failAll = true;
  const github = fakeGitHub(
    seed(),
    (op, n, fields) =>
      failAll && (op === "link" || (op === "update" && fields === "type")),
  );
  run(github, "apply");
  assert.equal(github.issues.get(1001)!.type, null);
  failAll = false;
  run(github, "apply");
  assert.equal(github.issues.get(1001)!.type, "Feature");
  const before = typeWrites(github, 1001);
  run(github, "apply");
  assert.equal(typeWrites(github, 1001), before);
});

test("an Ungrouped issue that gained a parent is not listed as Ungrouped", () => {
  const github = fakeGitHub([
    botFeature(500),
    child(1, 500),
    child(2, 500),
    {
      number: 900,
      title: "Backlog index",
      labels: ["agentics: backlog-index"],
      body: '<!-- agentics:backlog-grooming state {"lastCheck":null,"lastSuccess":null,"ungrouped":{"1":"old"}} -->',
    },
  ]);
  const { body } = run(github, "plan", "{}");
  assert.ok(!body.includes("- #1 Child 1: old"));
});

test("a Feature whose move out failed is neither emptied nor closed", () => {
  const github = fakeGitHub(
    [botFeature(50), child(1, 50), child(2, 50)],
    (op, n) => op === "unlink" && n === 1,
  );
  const { body } = run(
    github,
    "apply",
    JSON.stringify({ moves: [{ issue: 1, to: null, reason: "fits nowhere" }] }),
  );
  assert.equal(github.issues.get(50)!.state, "OPEN");
  assert.equal(github.issues.get(2)!.parent, 50);
  assert.ok(!github.writes.some((w) => w[0] === "unlink" && w[1] === 2));
  assert.ok(
    body.includes("move #2 skipped: the move of #1 out of #50 did not land"),
  );
  assert.ok(
    body.includes("close #50 skipped: the move of #1 out of it did not land"),
  );
});

test("a merge whose move was skipped does not close the emptied Feature", () => {
  const github = fakeGitHub([
    botFeature(50),
    botFeature(60),
    child(1, 50),
    child(2, 50),
    child(3, 60),
    child(4, 60),
    { number: 880, title: "Human", type: "Feature" },
  ]);
  const original = github.client.issue;
  github.client.issue = (n) =>
    n === 2 ? { ...original(n), parent: 880 } : original(n);
  run(
    github,
    "apply",
    JSON.stringify({
      moves: [
        { issue: 1, to: 60, reason: "merge" },
        { issue: 2, to: 60, reason: "merge" },
      ],
    }),
  );
  assert.equal(github.issues.get(50)!.state, "OPEN");
});

test("a link whose Task typing fails is still recorded as linked", () => {
  const github = fakeGitHub(
    seed(),
    (op, n, fields) => op === "update" && n === 1 && fields === "type",
  );
  const { body } = run(github, "apply");
  assert.equal(github.issues.get(1)!.parent, 1001);
  assert.ok(body.includes("linked #1 under #1001"));
  assert.ok(body.includes("type #1 as Task failed: update refused"));
  assert.ok(!body.includes("link #1 failed"));
});

const PRIOR_INDEX = {
  number: 900,
  title: "Backlog index",
  labels: ["agentics: backlog-index"],
  body: '<!-- agentics:backlog-grooming state {"lastCheck":null,"lastSuccess":"2026-10-01T00:00:00.000Z","ungrouped":{}} -->',
};

for (const existing of [true, false]) {
  test(`irreducible size leaves ${existing ? "existing" : "missing"} index untouched after earlier apply writes`, () => {
    const hugeFeature = {
      number: 800,
      title: "x".repeat(65000),
      type: "Feature",
    };
    const github = fakeGitHub([
      ...seed(),
      hugeFeature,
      { number: 801, title: "child", parent: 800 },
      ...(existing ? [PRIOR_INDEX] : []),
    ]);
    assert.throws(
      () => run(github, "apply"),
      /index body exceeds 65000.*state marker/,
    );
    assert.ok(github.writes.some(([op, n]) => op === "link" && n === 1));
    assert.notEqual(github.issues.get(1)!.parent, null);
    assert.ok(
      !github.writes.some(
        ([op, n, fields]) => op === "update" && n === 900 && fields === "body",
      ),
    );
    assert.ok(
      !github.writes.some(
        ([op, title]) => op === "create" && title === "Backlog index",
      ),
    );
    if (existing) assert.equal(github.issues.get(900)!.body, PRIOR_INDEX.body);
    else
      assert.ok(
        ![...github.issues.values()].some((issue) =>
          issue.labels.includes("agentics: backlog-index"),
        ),
      );
  });
}

test("a run with a failed write keeps the previous successful refresh date", () => {
  const github = fakeGitHub(
    [...seed(), PRIOR_INDEX],
    (op, n) => op === "link" && n === 1,
  );
  const { body } = run(github, "apply");
  assert.ok(body.includes("link #1 failed: link refused"));
  assert.equal(readState(body).lastSuccess, "2026-10-01T00:00:00.000Z");
  assert.ok(
    body.includes("- Last successful refresh: 2026-10-01T00:00:00.000Z"),
  );
});

test("a rule drop alone still counts as a successful refresh", () => {
  const github = fakeGitHub([
    ...seed(),
    PRIOR_INDEX,
    { number: 880, title: "Human", type: "Feature" },
  ]);
  const { body } = run(
    github,
    "apply",
    JSON.stringify({ links: [{ issue: 3, feature: 880 }] }),
  );
  assert.equal(readState(body).lastSuccess, new Date(NOW).toISOString());
});

test("a failed Feature label is a gap, and the index is still rewritten", () => {
  const github = fakeGitHub(
    [...seed(), PRIOR_INDEX],
    (op, name) => op === "label" && name === "agentics: feature",
  );
  const { body } = run(github, "apply");
  assert.ok(body.includes("label agentics: feature failed: label refused"));
  assert.equal(github.issues.get(900)!.body, body);
  assert.equal(readState(body).lastSuccess, "2026-10-01T00:00:00.000Z");
});

test("a failed re-read after apply is a gap, and the index is still rewritten", () => {
  const github = fakeGitHub([...seed(), PRIOR_INDEX]);
  const snapshot = github.client.snapshot;
  let calls = 0;
  github.client.snapshot = (now) => {
    calls += 1;
    if (calls === 2) throw new Error("graphql down");
    return snapshot(now);
  };
  const { body } = run(github, "apply");
  assert.ok(body.includes("re-read after apply failed: graphql down"));
  assert.equal(github.issues.get(900)!.body, body);
  assert.equal(readState(body).lastSuccess, "2026-10-01T00:00:00.000Z");
});

test("labels are ensured before any other write", () => {
  const github = fakeGitHub(seed());
  run(github, "apply");
  assert.deepEqual(github.writes[0], ["label", "agentics: backlog-index"]);
  assert.deepEqual(github.writes[1], ["label", "agentics: feature"]);
});

const withBatch = (batch: number[]) => ({ ...META, batch });

function runWith(
  github: GitHubFixture,
  mode: "apply" | "plan",
  rawPlan: string,
  meta: FetchMeta,
) {
  return applyRun({
    mode,
    rawPlan,
    meta,
    client: github.client,
    runId: "77",
    runUrl: "https://github.com/muehz/agentics/actions/runs/77",
    now: NOW,
  });
}

function observeWriteAttempts(github: GitHubFixture) {
  const attempts: unknown[][] = [];
  function observe<Args extends unknown[], Result>(
    name: string,
    operation: (...args: Args) => Result,
  ): (...args: Args) => Result {
    return (...args) => {
      attempts.push([name, ...structuredClone(args)]);
      return operation(...args);
    };
  }
  const client = github.client;
  github.client = {
    ...client,
    ensureLabel: observe("ensureLabel", client.ensureLabel),
    createIssue: observe("createIssue", client.createIssue),
    updateIssue: observe("updateIssue", client.updateIssue),
    addSubIssue: observe("addSubIssue", client.addSubIssue),
    pinIssue: observe("pinIssue", client.pinIssue),
    removeSubIssue: observe("removeSubIssue", client.removeSubIssue),
  };
  return attempts;
}

test("stored placement outcomes preserve retry, record and omission independently of gap wording", () => {
  const capped: GroomingGap = {
    subject: "link #4 to new-2",
    status: "dropped",
    reason: "its Feature was over the cap",
    placement: { issue: 4, action: "retry" },
  };
  const rejected: GroomingGap = {
    subject: "move #4 to new-2",
    status: "dropped",
    reason: "moving #7 out of #500 would pass the cap of 5 moves",
    placement: { issue: 4, action: "record" },
  };
  const failed: GroomingGap = {
    subject: "type #4 as Task",
    status: "failed",
    reason: "type refused",
    placement: { issue: 4, action: "retry" },
  };
  const cases = [
    { gaps: [capped], expected: "retry" },
    { gaps: [rejected], expected: "record" },
    { gaps: [rejected, failed], expected: "retry" },
    { gaps: [failed], ungrouped: { 4: "explicit reason" }, expected: "omit" },
    { gaps: [failed], issue: 1, expected: "record" },
  ];
  for (const { gaps, issue = 4, ungrouped = {}, expected } of cases) {
    assert.equal(batchIssueOutcome(issue, ungrouped, gaps).action, expected);
    const reworded = gaps.map((gap) => ({
      ...gap,
      subject: "changed",
      reason: "new words",
    }));
    assert.equal(
      batchIssueOutcome(issue, ungrouped, reworded).action,
      expected,
    );
  }
  assert.equal(
    batchIssueOutcome(4, {}, [{ ...rejected, reason: capped.reason }]).action,
    "record",
  );
});

test("partial apply, retry and converged replay preserve reports and write payloads", () => {
  let failFirst = true;
  const github = fakeGitHub(
    seed(),
    (op, n) => op === "link" && n === 1 && failFirst,
  );
  const attempts = observeWriteAttempts(github);
  const digest = (text: string) =>
    createHash("sha256").update(text).digest("hex");
  const outcomes = [];
  for (const batch of [[1, 2, 3], [1], []]) {
    const result = runWith(github, "apply", PLAN, withBatch(batch));
    outcomes.push({
      body: digest(result.body),
      writes: digest(JSON.stringify(attempts.splice(0))),
      state: readState(result.body),
      batch: fetchRun({ client: github.client, now: NOW }).meta.batch,
      failures: result.record.failures,
    });
    failFirst = false;
  }
  const state = { lastCheck: META.fetchedAt, ungrouped: { 3: "standalone" } };
  // Recorded from the original applier, including the failed addSubIssue attempt.
  assert.deepEqual(outcomes, [
    {
      body: "d1a5564f9b27082cb7ba4b2a647bf723449b1c4debfba80fc1d560c64209eb83",
      writes:
        "7f88f5a47fefeddad6f964f2d2d1a307dffc43f1f7caa3c35e612314615c6955",
      state: { ...state, lastSuccess: null },
      batch: [1],
      failures: 1,
    },
    {
      body: "49cb379c0a2a9fdc5f6e1a058fe09b0d07b4a7b9f5d564dd12fbfead350b6c41",
      writes:
        "1224a94424555d00e30dd9172d8887d65b1ba0ae1957eb7d4c831859e46a4535",
      state: { ...state, lastSuccess: new Date(NOW).toISOString() },
      batch: [],
      failures: 0,
    },
    {
      body: "468c557d168d49caf97794150dec58df020f778b6cb5f3f8bfb9bd53a9cd5611",
      writes:
        "ff4062ecb67f11f6bd91851924f455b963509272f78505a0cab81ffdd6415509",
      state: { ...state, lastSuccess: new Date(NOW).toISOString() },
      batch: [],
      failures: 0,
    },
  ]);
});

test("a rejected forced move is recorded while its dependent capped Feature is retried", () => {
  const github = fakeGitHub([
    botFeature(500),
    botFeature(600),
    ...[1, 2].map((n) => child(n, 500)),
    ...[3, 4, 5, 6, 7, 8].map((n) => child(n, 600)),
    { number: 9, title: "Unplaced candidate" },
  ]);
  const plan = JSON.stringify({
    features: [{ ref: "new-1", title: "Group", outcome: "o", why: "w" }],
    moves: [
      { issue: 1, to: "new-1", reason: "fits" },
      ...[3, 4, 5, 6].map((n) => ({
        issue: n,
        to: null,
        reason: "stands alone",
      })),
    ],
    links: [{ issue: 9, feature: "new-1" }],
  });
  const result = runWith(github, "apply", plan, withBatch([1, 9]));
  assert.deepEqual(result.record.ungrouped, {
    1: "not placed: move #1 to new-1: moving #2 out of #500 would pass the cap of 5 moves",
    3: "stands alone",
    4: "stands alone",
    5: "stands alone",
    6: "stands alone",
  });
  assert.deepEqual(readState(result.body).ungrouped, {
    3: "stands alone",
    4: "stands alone",
    5: "stands alone",
    6: "stands alone",
  });
  assert.deepEqual(
    fetchRun({ client: github.client, now: NOW }).meta.batch,
    [9],
  );
});

test("fetch records which issues were in the batch", () => {
  const github = fakeGitHub(seed());
  const { meta } = fetchRun({ client: github.client, now: NOW });
  assert.deepEqual(meta.batch, [1, 2, 3]);
});

test("batch issues the plan did not place are recorded so the queue drains", () => {
  const github = fakeGitHub(seed());
  const { body } = runWith(github, "apply", "{}", withBatch([1, 2, 3]));
  assert.deepEqual(readState(body).ungrouped, {
    1: "the plan did not place it",
    2: "the plan did not place it",
    3: "the plan did not place it",
  });
  const { input, meta } = fetchRun({ client: github.client, now: NOW });
  assert.deepEqual(input.batch, []);
  assert.equal(meta.queued, 0);
});

test("a batch issue whose link the rules dropped is recorded with the drop", () => {
  const github = fakeGitHub([
    ...seed(),
    { number: 880, title: "Human", type: "Feature" },
  ]);
  const { body } = runWith(
    github,
    "apply",
    JSON.stringify({ links: [{ issue: 3, feature: 880 }] }),
    withBatch([3]),
  );
  assert.equal(
    readState(body).ungrouped[3],
    "not placed: link #3 to #880: the target is not a Feature this workflow created",
  );
});

test("a batch issue whose link failed is left for the next run to retry", () => {
  const github = fakeGitHub(seed(), (op, n) => op === "link" && n === 1);
  const { body } = runWith(github, "apply", PLAN, withBatch([1, 2, 3]));
  assert.equal(Object.hasOwn(readState(body).ungrouped, "1"), false);
  assert.equal(readState(body).ungrouped[3], "standalone");
  assert.equal(github.issues.get(2)!.parent, 1001);
});

test("plan mode records no unplaced batch issues", () => {
  const github = fakeGitHub(seed());
  const { body } = runWith(github, "plan", "{}", withBatch([1, 2, 3]));
  assert.deepEqual(readState(body).ungrouped, {});
});

test("nothing is recorded as unplaced when the re-read after apply fails", () => {
  const github = fakeGitHub(seed());
  const snapshot = github.client.snapshot;
  let calls = 0;
  github.client.snapshot = (now) => {
    calls += 1;
    if (calls === 2) throw new Error("graphql down");
    return snapshot(now);
  };
  const { body } = runWith(github, "apply", PLAN, withBatch([1, 2, 3]));
  assert.deepEqual(readState(body).ungrouped, { 3: "standalone" });
});

test("batch issues dropped for capacity are offered again next run", () => {
  const seedIssues = Array.from({ length: 12 }, (_, i) => ({
    number: i + 1,
    title: `Issue ${i + 1}`,
  }));
  const github = fakeGitHub(seedIssues);
  const features = Array.from({ length: 6 }, (_, i) => ({
    ref: `new-${i + 1}`,
    title: `Group ${i + 1}`,
    outcome: "o",
    why: "w",
    dependsOn: [],
  }));
  const links = Array.from({ length: 12 }, (_, i) => ({
    issue: i + 1,
    feature: `new-${Math.floor(i / 2) + 1}`,
  }));
  const { body } = runWith(
    github,
    "apply",
    JSON.stringify({ features, links }),
    withBatch(seedIssues.map((i) => i.number)),
  );
  assert.equal(Object.hasOwn(readState(body).ungrouped, "11"), false);
  assert.equal(Object.hasOwn(readState(body).ungrouped, "12"), false);
  const { meta } = fetchRun({ client: github.client, now: NOW });
  assert.deepEqual(meta.batch, [11, 12]);
});

test("a rejected plan records no unplaced batch issues", () => {
  const github = fakeGitHub(seed());
  const { body } = runWith(github, "apply", "not json", withBatch([1, 2, 3]));
  assert.deepEqual(readState(body).ungrouped, {});
});

test("issues of a Feature dropped for its own size are recorded with the drop", () => {
  const github = fakeGitHub(seed());
  const plan = JSON.stringify({
    features: [
      { ref: "new-1", title: "Lonely", outcome: "o", why: "w", dependsOn: [] },
    ],
    links: [{ issue: 1, feature: "new-1" }],
  });
  const { body } = runWith(github, "apply", plan, withBatch([1]));
  assert.equal(
    readState(body).ungrouped[1],
    "not placed: link #1 to new-1: its Feature was dropped",
  );
});

test("a drop line about #11 is never read as one about #1", () => {
  const seedIssues = Array.from({ length: 11 }, (_, i) => ({
    number: i + 1,
    title: `Issue ${i + 1}`,
  }));
  const github = fakeGitHub([
    ...seedIssues,
    { number: 880, title: "Human", type: "Feature" },
  ]);
  const { body } = runWith(
    github,
    "apply",
    JSON.stringify({ links: [{ issue: 11, feature: 880 }] }),
    withBatch([1, 11]),
  );
  assert.equal(readState(body).ungrouped[1], "the plan did not place it");
});

test("a backlog larger than one batch drains across runs", () => {
  const seedIssues = Array.from({ length: 45 }, (_, i) => ({
    number: i + 1,
    title: `Issue ${i + 1}`,
  }));
  const github = fakeGitHub(seedIssues);
  const first = fetchRun({ client: github.client, now: NOW });
  assert.equal(first.meta.batch.length, 40);
  assert.equal(first.meta.queued, 5);
  runWith(github, "apply", "{}", first.meta);
  const second = fetchRun({ client: github.client, now: NOW });
  assert.equal(second.meta.batch.length, 5);
  assert.equal(second.meta.queued, 0);
});

const STALE_AT = "2026-09-20T00:00:00Z"; // 20 days before NOW

const gapsLine = (body: string) =>
  body.split("\n").find((line) => line.startsWith("- Coverage gaps: "))!;

const singleFeatureBody = renderFeatureBody({
  outcome: "o",
  why: "w",
  dependsOn: [],
  run: "5",
  ref: "new-3",
  single: true,
});

test("an old bot Feature with no children is listed on the Coverage gaps line", () => {
  const github = fakeGitHub([botFeature(500, { createdAt: STALE_AT })]);
  const { body } = run(github, "apply", "{}");
  assert.equal(
    gapsLine(body),
    "- Coverage gaps: Feature #500 has no children (open 20 days)",
  );
});

test("an old bot Feature with one child and no single marker is listed", () => {
  const github = fakeGitHub([
    botFeature(500, { createdAt: STALE_AT }),
    child(1, 500),
  ]);
  const { body } = run(github, "apply", "{}");
  assert.equal(
    gapsLine(body),
    "- Coverage gaps: Feature #500 has one child and is not marked single (open 20 days)",
  );
});

test("an old single-marked Feature with one child is not listed", () => {
  const github = fakeGitHub([
    botFeature(500, { createdAt: STALE_AT, body: singleFeatureBody }),
    child(1, 500),
  ]);
  const { body } = run(github, "apply", "{}");
  assert.equal(gapsLine(body), "- Coverage gaps: none");
});

test("a fresh bot Feature with no children is not listed", () => {
  const github = fakeGitHub([botFeature(500)]);
  const { body } = run(github, "apply", "{}");
  assert.equal(gapsLine(body), "- Coverage gaps: none");
});

test("plan mode lists a stale Feature too", () => {
  const github = fakeGitHub([botFeature(500, { createdAt: STALE_AT })]);
  const { body } = run(github, "plan", "{}");
  assert.equal(
    gapsLine(body),
    "- Coverage gaps: Feature #500 has no children (open 20 days)",
  );
});

test("a stale Feature does not count as a failed run", () => {
  const github = fakeGitHub([botFeature(500, { createdAt: STALE_AT })]);
  const { body } = run(github, "apply", "{}");
  assert.equal(
    gapsLine(body),
    "- Coverage gaps: Feature #500 has no children (open 20 days)",
  );
  assert.ok(body.includes("- Failed runs since the last success: 1"));
  assert.equal(readState(body).lastSuccess, new Date(NOW).toISOString());
});

test("logical gap count includes ordinary and stale gaps even when capped", () => {
  const mixed = run(
    fakeGitHub([botFeature(500, { createdAt: STALE_AT })]),
    "plan",
    "invalid",
  );
  assert.equal(mixed.gapCount, 2);
  assert.match(gapsLine(mixed.body), /Feature #500 has no children/);
  assert.match(gapsLine(mixed.body), /plan/i);
  const many = Array.from({ length: 1000 }, (_, i) =>
    botFeature(2000 + i, { createdAt: STALE_AT }),
  );
  const capped = run(fakeGitHub(many), "plan", "invalid");
  assert.equal(capped.gapCount, 1001);
  assert.match(gapsLine(capped.body), /more stale Features not shown/);
});

test("plan suppresses stale flags for proposed closes without projecting the listing", () => {
  const github = fakeGitHub([
    botFeature(500, { createdAt: STALE_AT }),
    { ...child(1, 500), state: "CLOSED" },
    botFeature(501, { createdAt: STALE_AT }),
    child(2, 501),
    botFeature(502, { createdAt: STALE_AT }),
  ]);
  const plan = JSON.stringify({
    moves: [{ issue: 2, to: null, reason: "fits nowhere" }],
  });
  const { body } = run(github, "plan", plan);
  assert.ok(body.includes("close #500 as completed"));
  assert.ok(body.includes("close #501 as not_planned"));
  assert.equal(
    gapsLine(body),
    "- Coverage gaps: Feature #502 has no children (open 20 days)",
  );
  assert.ok(body.includes("### #500") && body.includes("### #501"));
});

test("invalid plans and failed apply close or reread retain stale flags", () => {
  const seed = [
    botFeature(500, { createdAt: STALE_AT }),
    { ...child(1, 500), state: "CLOSED" },
  ];
  const invalid = run(fakeGitHub(seed), "plan", "invalid");
  assert.match(gapsLine(invalid.body), /Feature #500 has one child/);
  const failed = run(
    fakeGitHub(
      seed,
      (op, n, fields) =>
        op === "update" && n === 500 && fields === "state,state_reason",
    ),
    "apply",
    "{}",
  );
  assert.match(gapsLine(failed.body), /Feature #500 has one child/);
  assert.match(gapsLine(failed.body), /close #500 failed/);
  const github = fakeGitHub(seed);
  const snapshot = github.client.snapshot;
  let calls = 0;
  github.client.snapshot = (now) => {
    if (++calls === 2) throw new Error("offline");
    return snapshot(now);
  };
  const reread = run(github, "apply", "{}");
  assert.match(gapsLine(reread.body), /Feature #500 has one child/);
  assert.match(gapsLine(reread.body), /re-read after apply failed: offline/);
});

test("main logs all logical gaps including capped stale entries", () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "grooming-main-"));
  const originalError = console.error;
  try {
    const output = path.join(dir, "output.json");
    const meta = path.join(dir, "meta.json");
    fs.writeFileSync(
      output,
      JSON.stringify({ items: [{ type: "apply_grooming", plan: "invalid" }] }),
    );
    fs.writeFileSync(meta, JSON.stringify(META));
    const env = {
      GH_REPO: "owner/repo",
      GH_AW_AGENT_OUTPUT: output,
      FETCH_META: meta,
      MODE: "plan",
      GITHUB_RUN_ID: "77",
      GITHUB_SERVER_URL: "https://github.com",
    };
    for (const count of [1, 1000]) {
      const github = fakeGitHub(
        Array.from({ length: count }, (_, i) =>
          botFeature(2000 + i, { createdAt: "2000-01-01T00:00:00Z" }),
        ),
      );
      const messages: unknown[] = [];
      console.error = (message) => messages.push(message);
      main(["node", "script", "apply"], env, () => github.client);
      assert.deepEqual(github.writes, [
        ["label", "agentics: backlog-index"],
        ["create", "Backlog index"],
        ["pin", "I_1001"],
      ]);
      assert.match(
        github.issues.get(1001)!.body,
        /plan rejected: plan is not valid JSON/,
      );
      assert.ok(
        messages.includes(
          `backlog-grooming: plan: 0 changes, ${count + 1} gaps`,
        ),
      );
      if (count > 1)
        assert.match(
          github.issues.get(1001)!.body,
          /more stale Features not shown/,
        );
    }
  } finally {
    console.error = originalError;
    fs.rmSync(dir, { recursive: true, force: true });
  }
});
