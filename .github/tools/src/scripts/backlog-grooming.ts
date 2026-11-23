#!/usr/bin/env node
import type {
  BacklogSnapshot,
  BacklogIssue,
  BacklogPlan,
  BacklogState,
  FeatureReference,
  GroomingGap,
} from "./backlog-grooming-model.ts";
import type { BacklogOperations } from "./backlog-grooming-rules.ts";
import type { BacklogClient } from "./backlog-grooming-client.ts";
export interface FetchMeta {
  [field: string]: unknown;
  batch?: number[];
}
export interface RunRecord {
  changes: string[];
  gaps: string[];
  ungrouped: Record<string, string>;
  failures: number;
}
interface RenderState {
  lastCheck: unknown;
  lastSuccess: string | null;
  ungrouped: Record<string, string>;
}
interface ViewMeta {
  mode: string;
  runUrl: string;
  proposed: string[] | null;
  state: RenderState;
  queued: unknown;
  failedRuns: unknown;
  checkedAt: string;
}
interface ApplyOptions {
  mode: "apply" | "plan";
  rawPlan: string;
  meta: FetchMeta;
  client: BacklogClient;
  runId: string;
  runUrl: string;
  now: number;
}
function property(value: unknown, key: string): unknown {
  // This is an operational property view, not a schema claim; native boxing/null errors are preserved.
  return (value as Record<string, unknown>)[key];
}

// Entry point for both deterministic stages of backlog grooming:
//   fetch  writes the agent's input and the run metadata the apply job reads
//   apply  validates the agent's plan against live state, applies it within
//          the caps (apply mode) or only proposes it (plan mode), and
//          rewrites the index last
// Neither stage trusts the other's view of GitHub: apply re-reads.

import fs from "node:fs";
import path from "node:path";
import {
  LABEL_FEATURE,
  LABEL_INDEX,
  batchIssueOutcome,
  isBotFeature,
  parsePlan,
} from "./backlog-grooming-model.ts";
import {
  selectBatch,
  movableChildren,
  pullsFor,
  isListedFeature,
  agentInput,
  staleFeatures,
} from "./backlog-grooming-select.ts";
import { planOperations, describe } from "./backlog-grooming-rules.ts";
import {
  readState,
  readMarker,
  readSection,
  renderFeatureBody,
  priorityOf,
  renderIndex,
  renderGap,
} from "./backlog-grooming-render.ts";
import { readSafeOutput, safeOutputRepo } from "./safe-output.ts";
import { createGh, createClient } from "./backlog-grooming-client.ts";

const WORKFLOW_FILE = "backlog-grooming.lock.yml";
const INDEX_TITLE = "Backlog index";

function findIndex(snapshot: BacklogSnapshot) {
  const found = snapshot.issues.filter((i) => i.labels.includes(LABEL_INDEX));
  if (found.length > 1) {
    throw new Error(
      `found ${found.length} open issues labeled ${LABEL_INDEX}; close all but one`,
    );
  }
  return found.length === 1 ? found[0]! : null;
}

function fetchRun({ client, now }: { client: BacklogClient; now: number }) {
  const snapshot = client.snapshot(new Date(now).toISOString());
  const index = findIndex(snapshot);
  const state = readState(index ? index.body : "");
  const selection = selectBatch(snapshot, state);
  return {
    input: agentInput(
      snapshot,
      state,
      selection,
      movableChildren(snapshot, now),
    ),
    meta: {
      fetchedAt: snapshot.now,
      queued: selection.queued,
      batch: selection.batch.map((issue) => issue.number),
      failedRuns: client.failedRunsSinceSuccess(WORKFLOW_FILE),
    },
  };
}

function childId(live: BacklogSnapshot, number: number) {
  for (const issue of live.issues) {
    if (issue.number === number) return issue.id;
    const child = issue.children.find((c) => c.number === number);
    if (child) return child.id;
  }
  throw new Error(`no id known for #${number}`);
}

// A Feature this run already created (a rerun of the same run after a
// partial apply) is adopted before the rules see the plan: its ref becomes
// the issue number, and links already in place drop out as done. The title
// must match too, because re-running all jobs reruns the agent under the
// same run id, and the same ref can then name a different grouping.
function adoptCreated(plan: BacklogPlan, live: BacklogSnapshot, runId: string) {
  const adopted = new Map<string, BacklogIssue>();
  for (const f of plan.features) {
    const existing = live.issues.find((i) => {
      const marker = isBotFeature(i) ? readMarker(i.body) : null;
      return (
        marker !== null &&
        marker.run === String(runId) &&
        marker.ref === f.ref &&
        i.title === f.title
      );
    });
    if (existing) adopted.set(f.ref, existing);
  }
  if (adopted.size === 0) return { plan, adopted };
  const parentOf = new Map(live.issues.map((i) => [i.number, i.parent]));
  const to = (t: FeatureReference) =>
    typeof t === "string" && adopted.has(t) ? adopted.get(t)!.number : t;
  return {
    adopted,
    plan: {
      ...plan,
      features: plan.features.filter((f) => !adopted.has(f.ref)),
      links: plan.links
        .map((l) => ({ ...l, feature: to(l.feature) }))
        .filter((l) => parentOf.get(l.issue) !== l.feature),
      moves: plan.moves.map((m) => ({
        ...m,
        to: m.to === null ? null : to(m.to),
      })),
    },
  };
}

// Each write re-reads its target first, so a write that already happened,
// or that someone else made, is skipped rather than repeated or forced.
function execute(
  ops: BacklogOperations,
  live: BacklogSnapshot,
  client: BacklogClient,
  runId: string,
  record: RunRecord,
  gaps: GroomingGap[],
  adopted: Map<string, BacklogIssue>,
) {
  const { changes, ungrouped } = record;
  const writeGap = (
    status: "failed" | "skipped",
    subject: string,
    reason: string,
    issue?: number,
  ) => {
    const placement =
      issue === undefined ? undefined : { issue, action: "retry" as const };
    gaps.push({ subject, status, reason, placement });
  };
  const attempt = (subject: string, fn: () => void, issue?: number) => {
    try {
      fn();
    } catch (error) {
      writeGap("failed", subject, `${property(error, "message")}`, issue);
      record.failures += 1;
    }
  };
  for (const existing of adopted.values()) {
    changes.push(`adopted #${existing.number} ${existing.title}`);
    if (existing.type !== "Feature") {
      attempt(
        `type #${existing.number} as Feature`,
        () => client.updateIssue(existing.number, { type: "Feature" }),
        existing.number,
      );
    }
  }

  if (ops.creates.length > 0) {
    attempt(`label ${LABEL_FEATURE}`, () =>
      client.ensureLabel(
        LABEL_FEATURE,
        "5319e7",
        "Deliverable grouped by backlog grooming",
      ),
    );
  }
  const numbers = new Map<string, number>();
  for (const f of ops.creates) {
    attempt(`create ${f.ref}`, () => {
      const { number } = client.createIssue({
        title: f.title,
        body: renderFeatureBody({ ...f, run: runId }),
        labels: [LABEL_FEATURE],
      });
      // Recorded before typing: a Feature whose type fails to set still
      // receives its links, and the typing failure is its own gap.
      numbers.set(f.ref, number);
      changes.push(`created #${number} ${f.title}`);
      attempt(
        `type #${number} as Feature`,
        () => client.updateIssue(number, { type: "Feature" }),
        number,
      );
    });
  }
  const resolve = (t: FeatureReference) =>
    typeof t === "string" ? numbers.get(t) : t;

  for (const l of ops.links) {
    const parent = resolve(l.feature);
    if (parent === undefined) {
      writeGap(
        "skipped",
        `link #${l.issue}`,
        `${l.feature} was not created`,
        l.issue,
      );
      continue;
    }
    attempt(
      `link #${l.issue}`,
      () => {
        const current = client.issue(l.issue);
        if (current.parent === parent) return;
        if (current.state !== "OPEN" || current.parent !== null) {
          writeGap(
            "skipped",
            `link #${l.issue}`,
            "it changed since the fetch",
            l.issue,
          );
          return;
        }
        client.addSubIssue(parent, childId(live, l.issue), false);
        changes.push(`linked #${l.issue} under #${parent}`);
        // The link has landed; a typing failure is its own gap.
        if (current.type === null) {
          attempt(
            `type #${l.issue} as Task`,
            () => client.updateIssue(l.issue, { type: "Task" }),
            l.issue,
          );
        }
      },
      l.issue,
    );
  }

  // A Feature whose moves did not all land keeps its remaining children:
  // its forced move-out and its close both depended on those moves, and
  // closing it anyway would strand a child under a closed Feature.
  const unsettled = new Map<number, number>();
  for (const m of ops.moves) {
    if (m.forced && unsettled.has(m.from)) {
      writeGap(
        "skipped",
        `move #${m.issue}`,
        `the move of #${unsettled.get(m.from)} out of #${m.from} did not land`,
        m.issue,
      );
      continue;
    }
    const to = m.to === null ? null : resolve(m.to);
    if (to === undefined) {
      writeGap(
        "skipped",
        `move #${m.issue}`,
        `${m.to} was not created`,
        m.issue,
      );
      unsettled.set(m.from, m.issue);
      continue;
    }
    let landed = false;
    attempt(
      `move #${m.issue}`,
      () => {
        const current = client.issue(m.issue);
        if (current.parent === to) {
          landed = true;
          return;
        }
        if (current.parent !== m.from) {
          writeGap(
            "skipped",
            `move #${m.issue}`,
            "it changed since the fetch",
            m.issue,
          );
          return;
        }
        if (to === null) {
          client.removeSubIssue(m.from, childId(live, m.issue));
          ungrouped[m.issue] = m.reason;
        } else {
          client.addSubIssue(to, childId(live, m.issue), true);
        }
        landed = true;
        changes.push(
          `moved #${m.issue} from #${m.from} to ${to === null ? "Ungrouped" : `#${to}`}: ${m.reason}`,
        );
      },
      m.issue,
    );
    if (!landed && !unsettled.has(m.from)) unsettled.set(m.from, m.issue);
  }

  for (const u of ops.updates) {
    attempt(`update #${u.feature}`, () => {
      const feature = live.issues.find((i) => i.number === u.feature)!;
      const marker = readMarker(feature.body) || {
        run: runId,
        ref: "",
        single: false,
      };
      client.updateIssue(u.feature, {
        title: u.title,
        body: renderFeatureBody({
          ...u,
          run: marker.run,
          ref: marker.ref,
          single: marker.single,
        }),
      });
      changes.push(`updated #${u.feature} ${u.title}`);
    });
  }

  for (const c of ops.closes) {
    if (c.reason === "not_planned" && unsettled.has(c.issue)) {
      writeGap(
        "skipped",
        `close #${c.issue}`,
        `the move of #${unsettled.get(c.issue)} out of it did not land`,
      );
      continue;
    }
    attempt(`close #${c.issue}`, () => {
      client.updateIssue(c.issue, { state: "closed", state_reason: c.reason });
      changes.push(`closed #${c.issue} as ${c.reason}`);
    });
  }

  for (const u of ops.ungrouped) ungrouped[u.issue] = u.reason;
}

function buildView(
  after: BacklogSnapshot,
  state: RenderState,
  record: Pick<RunRecord, "changes" | "gaps">,
  meta: ViewMeta,
  closing = new Set<number>(),
) {
  const byNumber = new Map(after.issues.map((i) => [i.number, i]));
  const features = after.issues.filter(isListedFeature).map((f) => {
    const own = isBotFeature(f);
    return {
      number: f.number,
      title: f.title,
      human: !own,
      outcome: own ? readSection(f.body, "Outcome") : null,
      deps: own ? (readMarker(f.body) || { deps: [] }).deps : [],
      priority: priorityOf(
        f.children.flatMap(
          (c) => (byNumber.get(c.number) || { labels: [] }).labels,
        ),
      ),
      children: f.children.map((c) => ({
        number: c.number,
        title: c.title,
        closed: c.state === "CLOSED",
        pulls: c.state === "CLOSED" ? [] : pullsFor(after, c.number),
      })),
    };
  });
  const ungrouped = Object.entries(state.ungrouped)
    .map(([number, reason]) => ({
      issue: byNumber.get(Number(number)),
      reason,
    }))
    .filter(({ issue }) => issue !== undefined && issue.parent === null)
    .map(({ issue, reason }) => ({
      number: issue!.number,
      title: issue!.title,
      reason,
    }));
  const stale = staleFeatures(after, Date.parse(meta.checkedAt))
    .filter((f) => !closing.has(f.number))
    .map((f) => `Feature #${f.number} ${f.reason} (open ${f.days} days)`);
  return {
    ...meta,
    features,
    ungrouped,
    ...record,
    gaps: record.gaps,
    staleGaps: stale,
  };
}

function applyRun({
  mode,
  rawPlan,
  meta,
  client,
  runId,
  runUrl,
  now,
}: ApplyOptions) {
  const live = client.snapshot(new Date(now).toISOString());
  const index = findIndex(live);
  const previous = readState(index ? index.body : "");
  // failures counts writes that threw; a rule drop or a skipped write is a
  // gap but not a failure.
  const record: RunRecord = {
    changes: [],
    gaps: [],
    ungrouped: {},
    failures: 0,
  };
  const gaps: GroomingGap[] = [];
  // Labels come first, the index label included: a Feature or the index
  // cannot carry a label that does not exist yet, and ensuring them before any
  // other write keeps a label failure from leaving half a run behind.
  client.ensureLabel(
    LABEL_INDEX,
    "0e8a16",
    "Rolling index written by backlog grooming",
  );

  const parsed = parsePlan(rawPlan);
  let ops: BacklogOperations | null = null;
  let adopted = new Map<string, BacklogIssue>();
  if (parsed.plan === null) {
    gaps.push({
      subject: "plan rejected",
      status: "dropped",
      reason: parsed.errors.slice(0, 5).join("; "),
    });
  } else {
    const adoption = adoptCreated(parsed.plan, live, runId);
    adopted = adoption.adopted;
    ops = planOperations(adoption.plan, live, now);
    gaps.push(...ops.gaps);
  }

  let proposed: string[] | null = null;
  let after = live;
  let state: RenderState = previous;
  if (ops !== null && mode === "apply") {
    execute(ops, live, client, runId, record, gaps, adopted);
    // The index is rewritten even when this re-read fails; it then shows the
    // state as fetched, with the failure in its gap line.
    let reread = true;
    try {
      after = client.snapshot(new Date(now).toISOString());
    } catch (error) {
      gaps.push({
        subject: "re-read after apply",
        status: "failed",
        reason: `${property(error, "message")}`,
      });
      record.failures += 1;
      reread = false;
    }
    // A batch issue this run neither placed nor recorded would come back at
    // the front of every batch, so the queue would never drain. Record it as
    // Ungrouped with why; the usual rule offers it again once it changes. An
    // issue whose write failed or was skipped is left out so the next run
    // retries it, and nothing is recorded without a fresh re-read. Issues now
    // closed or parented fall out in the merge below.
    if (reread) {
      for (const n of meta.batch || []) {
        const outcome = batchIssueOutcome(n, record.ungrouped, gaps);
        if (outcome.action !== "record") continue;
        record.ungrouped[n] = outcome.drop
          ? `not placed: ${renderGap(outcome.drop)}`
          : "the plan did not place it";
      }
    }
    state = {
      lastCheck: meta.fetchedAt,
      // A run with a failed write is not a successful refresh, so the date
      // of the last one carries forward.
      lastSuccess:
        record.failures === 0
          ? new Date(now).toISOString()
          : previous.lastSuccess,
      ungrouped: Object.fromEntries(
        Object.entries({ ...previous.ungrouped, ...record.ungrouped }).filter(
          ([n]) => {
            const issue = after.issues.find((i) => i.number === Number(n));
            return issue !== undefined && issue.parent === null;
          },
        ),
      ),
    };
  } else if (ops !== null) {
    proposed = describe(ops);
  }

  const closing = new Set(
    mode === "plan" && ops !== null ? ops.closes.map((c) => c.issue) : [],
  );
  record.gaps = gaps.map(renderGap);
  const view = buildView(
    after,
    state,
    { changes: record.changes, gaps: record.gaps },
    {
      mode,
      runUrl,
      proposed,
      state,
      queued: meta.queued,
      failedRuns: meta.failedRuns,
      checkedAt: new Date(now).toISOString(),
    },
    closing,
  );
  const gapCount = view.gaps.length + view.staleGaps.length;
  const body = renderIndex(view);
  if (index === null) {
    const created = client.createIssue({
      title: INDEX_TITLE,
      body,
      labels: [LABEL_INDEX],
    });
    // Pinning is a convenience; the index is found by its label, so a
    // failed pin is logged and the run still succeeds.
    try {
      client.pinIssue(created.nodeId);
    } catch (error) {
      console.error(
        `backlog-grooming: pinning the index failed: ${property(error, "message")}`,
      );
    }
  } else {
    client.updateIssue(index.number, { body });
  }
  return { body, record, gapCount };
}

function readAgentPlan(outputPath: string): string | null {
  const output = readSafeOutput(
    { GH_AW_AGENT_OUTPUT: outputPath },
    "apply_grooming",
  );
  return output.kind === "matched" ? output.value : null;
}

// The metadata crosses a job boundary as an artifact, and its batch numbers
// are later built into a regular expression and used as record keys, so
// anything but an issue number is refused here.
function readFetchMeta(metaPath: string): FetchMeta {
  const meta: unknown = JSON.parse(fs.readFileSync(metaPath, "utf8"));
  if (meta === null || typeof meta !== "object" || Array.isArray(meta)) {
    throw new Error("FETCH_META is not a JSON object");
  }
  const fields = meta as Record<string, unknown>;
  if (fields.batch !== undefined) {
    if (!Array.isArray(fields.batch)) {
      throw new Error("FETCH_META batch must be an array");
    }
    for (const n of fields.batch) {
      if (typeof n !== "number" || !Number.isSafeInteger(n) || n <= 0) {
        throw new Error("FETCH_META batch must hold issue numbers");
      }
    }
  }
  // Object shape and every supplied batch item were checked above; all other fields remain unknown.
  return fields as FetchMeta;
}

function required(env: NodeJS.ProcessEnv, name: string): string {
  if (!env[name]) throw new Error(`${name} is required`);
  return env[name]!;
}

function main(
  argv: string[],
  env: NodeJS.ProcessEnv,
  makeClient: (repo: string) => BacklogClient = (repo) =>
    createClient(createGh(), repo),
) {
  const command = argv[2];
  const repo = safeOutputRepo(env);
  const client = makeClient(repo);
  const now = Date.now();

  if (command === "fetch") {
    const out = required(env, "OUT_DIR");
    const { input, meta } = fetchRun({ client, now });
    fs.mkdirSync(out, { recursive: true });
    fs.writeFileSync(
      path.join(out, "agent-input.json"),
      `${JSON.stringify(input, null, 2)}\n`,
    );
    fs.writeFileSync(path.join(out, "fetch.json"), `${JSON.stringify(meta)}\n`);
    console.error(
      `backlog-grooming: batch of ${input.batch.length}, ${meta.queued} queued`,
    );
    return;
  }
  if (command === "apply") {
    const output = readSafeOutput(env, "apply_grooming");
    if (output.kind !== "matched") {
      console.error(
        output.kind === "staged"
          ? "backlog-grooming: safe outputs are staged; no API calls made"
          : "backlog-grooming: no apply_grooming item; no-op",
      );
      return;
    }
    const rawPlan = output.value;
    const mode = required(env, "MODE");
    if (mode !== "plan" && mode !== "apply")
      throw new Error("MODE must be plan or apply");
    const meta = readFetchMeta(required(env, "FETCH_META"));
    const runId = required(env, "GITHUB_RUN_ID");
    const runUrl = `${required(env, "GITHUB_SERVER_URL")}/${repo}/actions/runs/${runId}`;
    const { record, gapCount } = applyRun({
      mode,
      rawPlan,
      meta,
      client,
      runId,
      runUrl,
      now,
    });
    console.error(
      `backlog-grooming: ${mode}: ${record.changes.length} changes, ${gapCount} gaps`,
    );
    return;
  }
  throw new Error("usage: backlog-grooming.ts fetch|apply");
}

if (import.meta.main) {
  try {
    main(process.argv, process.env);
  } catch (error) {
    console.error(
      `backlog-grooming: ${error instanceof Error ? error.message : "operation failed"}`,
    );
    process.exitCode = 1;
  }
}
export {
  fetchRun,
  applyRun,
  execute,
  buildView,
  readAgentPlan,
  readFetchMeta,
  findIndex,
  main,
  adoptCreated,
};
