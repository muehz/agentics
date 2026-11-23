import type {
  BacklogIssue,
  BacklogSnapshot,
  BacklogPlan,
  FeatureReference,
  NewFeature,
  FeatureUpdate,
  GroomingGap,
} from "./backlog-grooming-model.ts";
export interface PlannedLink {
  issue: number;
  feature: FeatureReference;
  setTask: boolean;
}
export interface PlannedMove {
  issue: number;
  from: number;
  to: FeatureReference | null;
  reason: string;
  forced?: boolean;
}
export interface PlannedClose {
  issue: number;
  reason: "not_planned" | "completed";
}
export interface BacklogOperations {
  creates: NewFeature[];
  links: PlannedLink[];
  moves: PlannedMove[];
  updates: FeatureUpdate[];
  closes: PlannedClose[];
  ungrouped: { issue: number; reason: string }[];
  gaps: GroomingGap[];
  drops: string[];
}
// planOperations turns a parsed plan into the operations the apply job may
// run against live state, and a drop line for everything it refuses. It is
// pure: the executor re-reads each target again before writing.

import {
  CAPS,
  LABEL_INDEX,
  STICKY_MS,
  isBotFeature,
  isFeatureType,
} from "./backlog-grooming-model.ts";
import { readMarker, renderGap } from "./backlog-grooming-render.ts";

function name(target: FeatureReference | null) {
  if (target === null) return "Ungrouped";
  return typeof target === "string" ? target : `#${target}`;
}

function planOperations(
  plan: BacklogPlan,
  live: BacklogSnapshot,
  now: number,
): BacklogOperations {
  const gaps: GroomingGap[] = [];
  const drop = (subject: string, reason: string) =>
    gaps.push({ subject, status: "dropped", reason });
  const dropPlacement = (
    issue: number,
    subject: string,
    reason: string,
    action: "record" | "retry" = "record",
  ) =>
    gaps.push({
      subject,
      status: "dropped",
      reason,
      placement: { issue, action },
    });
  const byNumber = new Map(live.issues.map((i) => [i.number, i]));
  const own = (n: number) => byNumber.has(n) && isBotFeature(byNumber.get(n)!);
  const linkedRecently = (issue: BacklogIssue) =>
    issue.parentAddedAt === null ||
    now - Date.parse(issue.parentAddedAt) < STICKY_MS;

  // Caps count entries in plan order as they pass their own checks; a slot
  // taken by an entry that a later cascade drops is not given back.

  const created = new Map<string, NewFeature>();
  // Refs dropped for the cap, so their links and moves say why they did not
  // land; a capacity drop is retried next run, not judged.
  const capped = new Set<FeatureReference | null>();
  for (const f of plan.features) {
    if (created.size >= CAPS.features) {
      drop(`feature ${f.ref}`, `over the cap of ${CAPS.features} new Features`);
      capped.add(f.ref);
      continue;
    }
    created.set(f.ref, f);
  }
  const isTarget = (t: FeatureReference) =>
    typeof t === "string" ? created.has(t) : own(t);

  // members tracks each Feature's children as the plan would leave them.
  // Targets enter members when the live Feature or new reference is established above.
  const members = new Map<FeatureReference, Set<number>>();
  for (const issue of live.issues) {
    if (isBotFeature(issue))
      members.set(issue.number, new Set(issue.children.map((c) => c.number)));
  }
  for (const ref of created.keys()) members.set(ref, new Set());

  let links: PlannedLink[] = [];
  // New Features that lost a link or a move to a cap: if one then falls below
  // two children, that is a capacity drop too, not a judgment on its issues.
  const starved = new Set<FeatureReference | null>();
  for (const l of plan.links) {
    const what = `link #${l.issue} to ${name(l.feature)}`;
    const issue = byNumber.get(l.issue);
    if (links.length >= CAPS.links) {
      dropPlacement(
        l.issue,
        what,
        `over the cap of ${CAPS.links} links`,
        "retry",
      );
      if (typeof l.feature === "string") starved.add(l.feature);
    } else if (capped.has(l.feature))
      dropPlacement(l.issue, what, "its Feature was over the cap", "retry");
    else if (!isTarget(l.feature))
      dropPlacement(
        l.issue,
        what,
        "the target is not a Feature this workflow created",
      );
    else if (issue === undefined)
      dropPlacement(l.issue, what, "the issue is not open");
    else if (issue.parent !== null)
      dropPlacement(
        l.issue,
        what,
        `the issue already has parent #${issue.parent}`,
      );
    else if (
      isFeatureType(issue) ||
      isBotFeature(issue) ||
      issue.labels.includes(LABEL_INDEX)
    ) {
      dropPlacement(l.issue, what, "the issue is a Feature or the index");
    } else {
      links.push({
        issue: l.issue,
        feature: l.feature,
        setTask: issue.type === null,
      });
      members.get(l.feature)!.add(l.issue);
    }
  }

  let moves: PlannedMove[] = [];
  for (const m of plan.moves) {
    const what = `move #${m.issue} to ${name(m.to)}`;
    const issue = byNumber.get(m.issue);
    if (moves.length >= CAPS.moves) {
      dropPlacement(
        m.issue,
        what,
        `over the cap of ${CAPS.moves} moves`,
        "retry",
      );
      if (typeof m.to === "string") starved.add(m.to);
    } else if (issue === undefined)
      dropPlacement(m.issue, what, "the issue is not open");
    else if (issue.parent === null || !own(issue.parent)) {
      dropPlacement(
        m.issue,
        what,
        "the issue is not under a Feature this workflow created",
      );
    } else if (capped.has(m.to)) {
      dropPlacement(m.issue, what, "its Feature was over the cap", "retry");
    } else if (m.to !== null && !isTarget(m.to)) {
      dropPlacement(
        m.issue,
        what,
        "the target is not a Feature this workflow created",
      );
    } else if (m.to === issue.parent)
      dropPlacement(m.issue, what, "the issue is already under that Feature");
    else if (issue.parentAddedAt === null)
      dropPlacement(m.issue, what, "its link date is unknown");
    else if (linkedRecently(issue)) {
      dropPlacement(m.issue, what, "it was linked less than 7 days ago");
    } else {
      moves.push({
        issue: m.issue,
        from: issue.parent,
        to: m.to,
        reason: m.reason,
      });
      members.get(issue.parent)!.delete(m.issue);
      if (m.to !== null) members.get(m.to)!.add(m.issue);
    }
  }

  const dropLink = (
    l: PlannedLink,
    why: string,
    action: "record" | "retry",
  ) => {
    dropPlacement(
      l.issue,
      `link #${l.issue} to ${name(l.feature)}`,
      why,
      action,
    );
    members.get(l.feature)!.delete(l.issue);
  };
  const dropMove = (
    m: PlannedMove,
    why: string,
    action: "record" | "retry",
  ) => {
    dropPlacement(m.issue, `move #${m.issue} to ${name(m.to)}`, why, action);
    members.get(m.from)!.add(m.issue);
    if (m.to !== null) members.get(m.to)!.delete(m.issue);
  };
  const placedHere = (n: number, target: FeatureReference) =>
    links.some((l) => l.issue === n && l.feature === target) ||
    moves.some((m) => m.issue === n && m.to === target);
  const stateOf = (feature: BacklogIssue, n: number) => {
    if (byNumber.has(n)) return byNumber.get(n)!.state;
    const child = feature.children.find((c) => c.number === n);
    return child === undefined ? "OPEN" : child.state;
  };
  const botFeatures = live.issues.filter(isBotFeature);
  const isSingle = (feature: BacklogIssue) =>
    readMarker(feature.body)?.single === true;

  // Settle the plan. A reshape that leaves a bot Feature with one open child
  // forces that child out to Ungrouped, and the forced move obeys the same
  // rules as any other: the child was not placed there this run, it is not
  // sticky, and it fits under the move cap. When it cannot, the moves that
  // shrank the Feature are dropped instead. Every drop can shrink another
  // Feature or a new one, so this repeats until nothing changes.
  let forced: PlannedMove[] = [];
  for (let settled = false; !settled;) {
    settled = true;
    for (const [ref, f] of created) {
      const count = members.get(ref)!.size;
      if (count >= 2 || (f.single && count === 1)) continue;
      drop(
        `feature ${ref}`,
        `${count} children; a Feature needs two, or one when marked single`,
      );
      created.delete(ref);
      const action = starved.has(ref) ? "retry" : "record";
      const cause =
        action === "retry"
          ? "its Feature was over the cap"
          : "its Feature was dropped";
      links = links.filter((l) => {
        if (l.feature !== ref) return true;
        dropLink(l, cause, action);
        return false;
      });
      moves = moves.filter((m) => {
        if (m.to !== ref) return true;
        dropMove(m, cause, action);
        return false;
      });
      settled = false;
    }
    // Features are checked in live order, so when the cap fires it is the
    // earliest Feature in that order whose shrinking moves are dropped.
    forced = [];
    for (const feature of botFeatures) {
      const number = feature.number;
      const shrinking = moves.filter((m) => m.from === number);
      const children = members.get(number)!;
      if (shrinking.length === 0 || children.size !== 1) continue;
      if (isSingle(feature)) continue;
      const last = [...children][0]!;
      if (stateOf(feature, last) === "CLOSED") continue;
      const lastIssue = byNumber.get(last);
      let blocker: { reason: string; atCap: boolean } | undefined;
      if (placedHere(last, number)) {
        blocker = {
          reason: `it would leave #${number} with only #${last}, placed there this run`,
          atCap: false,
        };
      } else if (lastIssue === undefined || linkedRecently(lastIssue)) {
        blocker = {
          reason: `it would leave #${number} with #${last}, which cannot move yet`,
          atCap: false,
        };
      } else if (moves.length + forced.length + 1 > CAPS.moves) {
        blocker = {
          reason: `moving #${last} out of #${number} would pass the cap of ${CAPS.moves} moves`,
          atCap: true,
        };
      }
      if (blocker === undefined) {
        forced.push({
          issue: last,
          from: number,
          to: null,
          reason: "its Feature was left with one child",
          forced: true,
        });
        continue;
      }
      for (const m of shrinking) {
        dropMove(m, blocker.reason, "record");
        if (blocker.atCap && typeof m.to === "string") {
          starved.add(m.to);
        }
      }
      moves = moves.filter((m) => m.from !== number);
      settled = false;
      break;
    }
  }

  const closes: PlannedClose[] = [];
  const ungrouped: { issue: number; reason: string }[] = [];
  const forcedFrom = new Map(forced.map((m) => [m.from, m]));
  for (const feature of botFeatures) {
    const number = feature.number;
    const children = members.get(number)!;
    const shrunk = moves.some((m) => m.from === number);
    // Only moves empty or shrink a Feature here. A Feature with no children
    // that this run did not shrink is one a partial earlier apply created
    // before its links landed; the rerun adopts it, so it stays open.
    if (children.size === 0) {
      if (shrunk) closes.push({ issue: number, reason: "not_planned" });
    } else if (forcedFrom.has(number)) {
      const m = forcedFrom.get(number)!;
      ungrouped.push({ issue: m.issue, reason: m.reason });
      closes.push({ issue: number, reason: "not_planned" });
    } else if ([...children].every((n) => stateOf(feature, n) === "CLOSED")) {
      closes.push({ issue: number, reason: "completed" });
    }
  }

  const closing = new Set(closes.map((c) => c.issue));
  const updates = plan.featureUpdates.filter((u) => {
    if (!own(u.feature)) {
      drop(
        `update #${u.feature}`,
        "the target is not a Feature this workflow created",
      );
      return false;
    }
    if (closing.has(u.feature)) {
      drop(`update #${u.feature}`, "the Feature closes this run");
      return false;
    }
    return true;
  });

  // parsePlan already rejects an issue that is both placed and ungrouped.
  for (const u of plan.ungrouped) {
    const issue = byNumber.get(u.issue);
    const what = `ungrouped #${u.issue}`;
    if (issue === undefined)
      dropPlacement(u.issue, what, "the issue is not open");
    else if (issue.parent !== null)
      dropPlacement(u.issue, what, "the issue has a parent");
    else if (
      isFeatureType(issue) ||
      isBotFeature(issue) ||
      issue.labels.includes(LABEL_INDEX)
    ) {
      dropPlacement(u.issue, what, "the issue is a Feature or the index");
    } else ungrouped.push(u);
  }

  return {
    creates: [...created.values()],
    links,
    moves: [...moves, ...forced],
    updates,
    closes,
    ungrouped,
    gaps,
    get drops() {
      return gaps.map(renderGap);
    },
  };
}

function describe(ops: BacklogOperations) {
  return [
    ...ops.creates.map((f) => `create ${f.ref} "${f.title}"`),
    ...ops.links.map((l) => `link #${l.issue} to ${name(l.feature)}`),
    ...ops.moves.map(
      (m) => `move #${m.issue} from #${m.from} to ${name(m.to)}: ${m.reason}`,
    ),
    ...ops.updates.map((u) => `update #${u.feature} "${u.title}"`),
    ...ops.closes.map((c) => `close #${c.issue} as ${c.reason}`),
    ...ops.ungrouped.map((u) => `leave #${u.issue} ungrouped: ${u.reason}`),
  ];
}

export { planOperations, describe };
