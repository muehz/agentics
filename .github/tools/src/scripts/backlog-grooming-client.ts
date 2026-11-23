import type {
  BacklogSnapshot,
  BacklogIssue,
} from "./backlog-grooming-model.ts";
export interface CreatedIssue {
  number: number;
  id: number;
  nodeId: string;
}
export interface IssueFields {
  title?: string;
  body?: string;
  type?: string;
  state?: string;
  state_reason?: string;
}
export interface CreateIssueInput {
  title: string;
  body: string;
  labels: string[];
}
export interface BacklogClient {
  snapshot(now: string): BacklogSnapshot;
  failedRunsSinceSuccess(workflowFile: string): number;
  issue(number: number): Pick<BacklogIssue, "state" | "parent" | "type">;
  ensureLabel(label: string, color: string, description: string): void;
  createIssue(input: CreateIssueInput): CreatedIssue;
  updateIssue(number: number, fields: IssueFields): void;
  addSubIssue(parent: number, childId: number, replace: boolean): void;
  pinIssue(nodeId: string): void;
  removeSubIssue(parent: number, childId: number): void;
}
// The GitHub boundary. Every gh call and every GraphQL shape lives here and
// is normalized into the snapshot the pure modules read, so nothing past
// this file sees a wire type.

import { runSafeGh, type SafeGhRunner } from "./safe-output.ts";

export type GhCommand = (args: string[], input?: string) => unknown;
export type GhRunner = SafeGhRunner;

function property(value: unknown, key: string): unknown {
  // This view preserves native property access and boxing; it establishes no wire schema.
  return (value as Record<string, unknown>)[key];
}
function nodes(value: unknown): unknown[] {
  // Elements stay unknown until normalization; the existing map/filter operations remain authoritative.
  return value as unknown[];
}
// The existing client assumes selected GitHub scalars match the domain types.
// These assertions preserve that legacy assumption and malformed scalar values;
// they do not validate the complete provider response.
function providerNumber(value: unknown): number {
  return value as number;
}
function providerString(value: unknown): string {
  return value as string;
}
function providerBoolean(value: unknown): boolean {
  return value as boolean;
}

const ISSUES_QUERY = `query($owner: String!, $name: String!, $cursor: String) {
  repository(owner: $owner, name: $name) {
    issues(first: 50, states: OPEN, after: $cursor, orderBy: {field: CREATED_AT, direction: ASC}) {
      pageInfo { hasNextPage endCursor }
      nodes {
        databaseId number title body createdAt updatedAt
        issueType { name }
        labels(first: 30) { nodes { name } }
        parent { number }
        timelineItems(last: 10, itemTypes: [PARENT_ISSUE_ADDED_EVENT]) {
          nodes { ... on ParentIssueAddedEvent { createdAt parent { number } } }
        }
        comments(last: 10) { nodes { author { login } body } }
        subIssues(first: 100) { nodes { databaseId number title state issueType { name } } }
      }
    }
  }
}`;

const PULLS_QUERY = `query($owner: String!, $name: String!, $cursor: String) {
  repository(owner: $owner, name: $name) {
    pullRequests(first: 50, states: OPEN, after: $cursor) {
      pageInfo { hasNextPage endCursor }
      nodes {
        number title body isDraft
        closingIssuesReferences(first: 20) { nodes { number } }
      }
    }
  }
}`;

const PIN_MUTATION = `mutation($issueId: ID!) {
  pinIssue(input: {issueId: $issueId}) { issue { number } }
}`;

const ISSUE_QUERY = `query($owner: String!, $name: String!, $number: Int!) {
  repository(owner: $owner, name: $name) {
    issue(number: $number) { state parent { number } issueType { name } }
  }
}`;

function createGh(run?: GhRunner): GhCommand {
  return (args, input) => runSafeGh("grooming", args, input, run);
}

function normalizeIssue(node: unknown): BacklogIssue {
  const parentNode = property(node, "parent");
  const parent = parentNode
    ? providerNumber(property(parentNode, "number"))
    : null;
  const added = nodes(property(property(node, "timelineItems"), "nodes"))
    .filter(
      (e) =>
        e &&
        property(e, "parent") &&
        property(property(e, "parent"), "number") === parent,
    )
    .map((e) => providerString(property(e, "createdAt")))
    .sort();
  return {
    id: providerNumber(property(node, "databaseId")),
    number: providerNumber(property(node, "number")),
    title: providerString(property(node, "title")),
    body: providerString(property(node, "body") || ""),
    state: "OPEN",
    labels: nodes(property(property(node, "labels"), "nodes")).map((l) =>
      providerString(property(l, "name")),
    ),
    type: property(node, "issueType")
      ? providerString(property(property(node, "issueType"), "name"))
      : null,
    parent,
    parentAddedAt: added.length > 0 ? added[added.length - 1]! : null,
    createdAt: providerString(property(node, "createdAt")),
    updatedAt: providerString(property(node, "updatedAt")),
    comments: nodes(property(property(node, "comments"), "nodes")).map((c) => ({
      author: property(c, "author")
        ? providerString(property(property(c, "author"), "login"))
        : "ghost",
      body: providerString(property(c, "body") || ""),
    })),
    children: nodes(property(property(node, "subIssues"), "nodes")).map(
      (c) => ({
        id: providerNumber(property(c, "databaseId")),
        number: providerNumber(property(c, "number")),
        title: providerString(property(c, "title")),
        state: providerString(property(c, "state")),
        type: property(c, "issueType")
          ? providerString(property(property(c, "issueType"), "name"))
          : null,
      }),
    ),
  };
}

function referencedIssues(node: unknown) {
  const refs = new Set(
    nodes(property(property(node, "closingIssuesReferences"), "nodes")).map(
      (n) => providerNumber(property(n, "number")),
    ),
  );
  for (const match of `${property(node, "title")}\n${property(node, "body") || ""}`.matchAll(
    /#([1-9][0-9]*)\b/g,
  )) {
    refs.add(Number(match[1]));
  }
  return [...refs].sort((a, b) => a - b);
}

function createClient(gh: GhCommand, repo: string): BacklogClient {
  const [owner, name] = repo.split("/");
  const graphql = (query: string, vars: Record<string, unknown>): unknown => {
    const args = [
      "api",
      "graphql",
      "-f",
      `query=${query}`,
      "-F",
      `owner=${owner}`,
      "-F",
      `name=${name}`,
    ];
    for (const [key, value] of Object.entries(vars)) {
      if (value !== null && value !== undefined)
        args.push("-F", `${key}=${value}`);
    }
    return property(property(gh(args), "data"), "repository");
  };
  const pages = <T>(
    query: string,
    field: string,
    normalize: (node: unknown) => T,
  ): T[] => {
    const out: T[] = [];
    let cursor: unknown = null;
    do {
      const connection: unknown = property(graphql(query, { cursor }), field);
      out.push(...nodes(property(connection, "nodes")).map(normalize));
      const pageInfo = property(connection, "pageInfo");
      cursor = property(pageInfo, "hasNextPage")
        ? property(pageInfo, "endCursor")
        : null;
    } while (cursor !== null);
    return out;
  };
  const rest = (method: string, path: string, body?: unknown): unknown =>
    gh(
      [
        "api",
        "--method",
        method,
        `repos/${repo}/${path}`,
        ...(body === undefined ? [] : ["--input", "-"]),
      ],
      body === undefined ? undefined : JSON.stringify(body),
    );

  return {
    snapshot(now) {
      return {
        now,
        issues: pages(ISSUES_QUERY, "issues", normalizeIssue),
        pulls: pages(PULLS_QUERY, "pullRequests", (node) => ({
          number: providerNumber(property(node, "number")),
          title: providerString(property(node, "title")),
          draft: providerBoolean(property(node, "isDraft")),
          refs: referencedIssues(node),
        })),
      };
    },
    failedRunsSinceSuccess(workflowFile) {
      const runs = property(
        rest("GET", `actions/workflows/${workflowFile}/runs?per_page=30`),
        "workflow_runs",
      );
      let failed = 0;
      for (const run of nodes(runs)) {
        if (property(run, "conclusion") === "success") break;
        if (property(run, "conclusion") === "failure") failed += 1;
      }
      return failed;
    },
    issue(number) {
      const issue = property(graphql(ISSUE_QUERY, { number }), "issue");
      return {
        state: providerString(property(issue, "state")),
        parent: property(issue, "parent")
          ? providerNumber(property(property(issue, "parent"), "number"))
          : null,
        type: property(issue, "issueType")
          ? providerString(property(property(issue, "issueType"), "name"))
          : null,
      };
    },
    ensureLabel(label, color, description) {
      // Only a 404 means the label is missing. Any other failure (auth,
      // network) is surfaced rather than masked by a doomed create.
      try {
        rest("GET", `labels/${encodeURIComponent(label)}`);
      } catch (error) {
        if (!/HTTP 404/.test(`${property(error, "message")}`)) throw error;
        rest("POST", "labels", { name: label, color, description });
      }
    },
    createIssue({ title, body, labels }) {
      const issue = rest("POST", "issues", { title, body, labels });
      return {
        number: providerNumber(property(issue, "number")),
        id: providerNumber(property(issue, "id")),
        nodeId: providerString(property(issue, "node_id")),
      };
    },
    updateIssue(number, fields) {
      rest("PATCH", `issues/${number}`, fields);
    },
    addSubIssue(parent, childId, replace) {
      rest("POST", `issues/${parent}/sub_issues`, {
        sub_issue_id: childId,
        replace_parent: replace,
      });
    },
    pinIssue(nodeId) {
      gh([
        "api",
        "graphql",
        "-f",
        `query=${PIN_MUTATION}`,
        "-F",
        `issueId=${nodeId}`,
      ]);
    },
    removeSubIssue(parent, childId) {
      rest("DELETE", `issues/${parent}/sub_issue`, { sub_issue_id: childId });
    },
  };
}

export { createGh, createClient, normalizeIssue, referencedIssues };
