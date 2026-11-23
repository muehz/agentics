import type { GhCommand } from "./backlog-grooming-client.ts";
// The client is the only code that knows gh arguments and GraphQL shapes.
// These tests pin both against a recording stand-in for gh.
import test from "node:test";
import assert from "node:assert/strict";
import {
  createGh,
  createClient,
  normalizeIssue,
  referencedIssues,
} from "./backlog-grooming-client.ts";

function recorder(responses: unknown[]) {
  const calls: { args: string[]; input?: string }[] = [];
  const gh: GhCommand = (args, input) => {
    calls.push({ args, input });
    const next = responses.shift();
    if (next instanceof Error) throw next;
    return next;
  };
  return { gh, calls };
}

const node = {
  databaseId: 111,
  number: 1,
  title: "One",
  body: null,
  createdAt: "2026-09-01T00:00:00Z",
  updatedAt: "2026-09-02T00:00:00Z",
  issueType: null,
  labels: { nodes: [{ name: "bug" }] },
  parent: { number: 500 },
  timelineItems: {
    nodes: [
      { createdAt: "2026-09-05T00:00:00Z", parent: { number: 400 } },
      { createdAt: "2026-09-04T00:00:00Z", parent: { number: 500 } },
      {},
    ],
  },
  comments: { nodes: [{ author: null, body: "hi" }] },
  subIssues: {
    nodes: [
      {
        databaseId: 222,
        number: 2,
        title: "Two",
        state: "CLOSED",
        issueType: { name: "Task" },
      },
    ],
  },
};

test("an issue node normalizes to the snapshot shape", () => {
  const issue = normalizeIssue(node);
  assert.equal(issue.id, 111);
  assert.equal(issue.body, "");
  assert.equal(issue.type, null);
  assert.equal(issue.parent, 500);
  assert.equal(issue.parentAddedAt, "2026-09-04T00:00:00Z");
  assert.deepEqual(issue.comments, [{ author: "ghost", body: "hi" }]);
  assert.deepEqual(issue.children, [
    { id: 222, number: 2, title: "Two", state: "CLOSED", type: "Task" },
  ]);
});

test("pull request references join closing references and body mentions", () => {
  const refs = referencedIssues({
    title: "Fix #3",
    body: "Also #4, not #0",
    closingIssuesReferences: { nodes: [{ number: 5 }] },
  });
  assert.deepEqual(refs, [3, 4, 5]);
});

test("snapshot pages issues until the last page", () => {
  const page = (nodes: unknown[], next: string | null) => ({
    data: {
      repository: {
        issues: {
          pageInfo: { hasNextPage: next !== null, endCursor: next },
          nodes,
        },
      },
    },
  });
  const pulls = {
    data: {
      repository: {
        pullRequests: {
          pageInfo: { hasNextPage: false, endCursor: null },
          nodes: [],
        },
      },
    },
  };
  const { gh, calls } = recorder([
    page([node], "c1"),
    page([{ ...node, number: 2 }], null),
    pulls,
  ]);
  const snapshot = createClient(gh, "muehz/agentics").snapshot(
    "2026-10-10T00:00:00Z",
  );
  assert.deepEqual(
    snapshot.issues.map((i) => i.number),
    [1, 2],
  );
  assert.ok(calls[1]!.args.includes("cursor=c1"));
  assert.ok(!calls[0]!.args.some((a) => a.startsWith("cursor=")));
});

test("the provider scalar bridge preserves malformed values without validating the response", () => {
  const connection = (nodes: unknown[]) => ({
    pageInfo: { hasNextPage: false, endCursor: null },
    nodes,
  });
  const { gh } = recorder([
    {
      data: {
        repository: {
          issues: connection([{ ...node, number: "x", title: 13 }]),
        },
      },
    },
    {
      data: {
        repository: {
          pullRequests: connection([
            {
              number: "pull",
              title: 23,
              body: null,
              isDraft: "truthy",
              closingIssuesReferences: { nodes: [] },
            },
          ]),
        },
      },
    },
  ]);
  const snapshot = createClient(gh, "muehz/agentics").snapshot(
    "2026-10-10T00:00:00Z",
  );
  assert.equal(snapshot.issues[0]!.number, "x");
  assert.equal(snapshot.issues[0]!.title, 13);
  assert.equal(snapshot.pulls[0]!.number, "pull");
  assert.equal(snapshot.pulls[0]!.title, 23);
  assert.equal(snapshot.pulls[0]!.draft, "truthy");
});

test("writes use the REST endpoints with JSON on stdin", () => {
  const { gh, calls } = recorder([
    { number: 9, id: 99, node_id: "I_9" },
    null,
    null,
    null,
  ]);
  const client = createClient(gh, "muehz/agentics");
  assert.deepEqual(
    client.createIssue({
      title: "t",
      body: "b",
      labels: ["agentics: feature"],
    }),
    { number: 9, id: 99, nodeId: "I_9" },
  );
  client.updateIssue(9, { type: "Feature" });
  client.addSubIssue(9, 111, true);
  client.removeSubIssue(9, 111);
  assert.deepEqual(
    calls.map((c) => c.args.slice(2, 4)),
    [
      ["POST", "repos/muehz/agentics/issues"],
      ["PATCH", "repos/muehz/agentics/issues/9"],
      ["POST", "repos/muehz/agentics/issues/9/sub_issues"],
      ["DELETE", "repos/muehz/agentics/issues/9/sub_issue"],
    ],
  );
  assert.deepEqual(
    calls.map((c) => c.args.slice(4)),
    [
      ["--input", "-"],
      ["--input", "-"],
      ["--input", "-"],
      ["--input", "-"],
    ],
  );
  assert.deepEqual(JSON.parse(calls[0]!.input!), {
    title: "t",
    body: "b",
    labels: ["agentics: feature"],
  });
  assert.deepEqual(JSON.parse(calls[1]!.input!), { type: "Feature" });
  assert.deepEqual(JSON.parse(calls[2]!.input!), {
    sub_issue_id: 111,
    replace_parent: true,
  });
  assert.deepEqual(JSON.parse(calls[3]!.input!), { sub_issue_id: 111 });
});

test("issue reads state, parent, and type by number", () => {
  const { gh, calls } = recorder([
    {
      data: {
        repository: {
          issue: {
            state: "OPEN",
            parent: { number: 500 },
            issueType: null,
          },
        },
      },
    },
  ]);
  assert.deepEqual(createClient(gh, "muehz/agentics").issue(12), {
    state: "OPEN",
    parent: 500,
    type: null,
  });
  // -F lets gh send 12 as the Int the query declares.
  assert.ok(calls[0]!.args.includes("number=12"));
  assert.equal(calls[0]!.input, undefined);
});

test("pinIssue sends the pinIssue mutation for the node ID", () => {
  const { gh, calls } = recorder([null]);
  createClient(gh, "muehz/agentics").pinIssue("I_kw123");
  assert.ok(calls[0]!.args.includes("issueId=I_kw123"));
  assert.ok(
    calls[0]!.args.some((a) =>
      a.includes("pinIssue(input: {issueId: $issueId})"),
    ),
  );
});

test("ensureLabel surfaces a lookup failure that is not a 404", () => {
  const { gh, calls } = recorder([new Error("gh api failed: HTTP 403")]);
  assert.throws(
    () =>
      createClient(gh, "muehz/agentics").ensureLabel(
        "agentics: feature",
        "5319e7",
        "d",
      ),
    /HTTP 403/,
  );
  assert.equal(calls.length, 1);
});

test("gh output that is not JSON raises a named error", () => {
  const gh = createGh(() => ({ status: 0, stdout: "<html>", stderr: "" }));
  assert.throws(() => gh(["api", "x"]), {
    message: "gh api returned invalid JSON",
  });
});

test("ensureLabel creates a label only when the lookup returns 404", () => {
  const { gh, calls } = recorder([new Error("gh api failed: HTTP 404"), null]);
  createClient(gh, "muehz/agentics").ensureLabel(
    "agentics: feature",
    "5319e7",
    "d",
  );
  assert.equal(
    calls[0]!.args[3],
    "repos/muehz/agentics/labels/agentics%3A%20feature",
  );
  assert.deepEqual(JSON.parse(calls[1]!.input!), {
    name: "agentics: feature",
    color: "5319e7",
    description: "d",
  });
});

test("failed runs are counted back to the last success", () => {
  const runs = {
    workflow_runs: [
      { conclusion: "failure" },
      { conclusion: "cancelled" },
      { conclusion: "failure" },
      { conclusion: "success" },
      { conclusion: "failure" },
    ],
  };
  const { gh } = recorder([runs]);
  assert.equal(
    createClient(gh, "muehz/agentics").failedRunsSinceSuccess(
      "backlog-grooming.lock.yml",
    ),
    2,
  );
});

test("a failing gh process raises with its HTTP status", () => {
  const gh = createGh(() => ({ status: 1, stdout: "", stderr: "HTTP 422" }));
  assert.throws(() => gh(["api", "x"]), { message: "gh api failed: HTTP 422" });
});
