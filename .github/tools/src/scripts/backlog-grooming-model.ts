export interface BacklogChild {
  id: number;
  number: number;
  title: string;
  state: string;
  type: string | null;
}
export interface BacklogIssue {
  id: number;
  number: number;
  title: string;
  body: string;
  state: string;
  labels: string[];
  type: string | null;
  parent: number | null;
  parentAddedAt: string | null;
  createdAt: string;
  updatedAt: string;
  comments: { author: string; body: string }[];
  children: BacklogChild[];
}
export interface BacklogPull {
  number: number;
  title: string;
  draft: boolean;
  refs: number[];
}
export interface BacklogSnapshot {
  now?: string;
  issues: BacklogIssue[];
  pulls: BacklogPull[];
}
export interface BacklogState {
  lastCheck: string | null;
  lastSuccess: string | null;
  ungrouped: Record<string, string>;
}
export interface FeatureContent {
  title: string;
  outcome: string;
  why: string;
  dependsOn: number[];
}
export interface NewFeature extends FeatureContent {
  ref: string;
  single: boolean;
}
export interface FeatureUpdate extends FeatureContent {
  feature: number;
}
export type FeatureReference = number | string;
export interface BacklogPlan {
  features: NewFeature[];
  links: { issue: number; feature: FeatureReference }[];
  moves: { issue: number; to: FeatureReference | null; reason: string }[];
  featureUpdates: FeatureUpdate[];
  ungrouped: { issue: number; reason: string }[];
}
export type ParsePlanResult = { plan: BacklogPlan | null; errors: string[] };

export type GroomingGap = {
  subject: string;
  reason: string;
} & (
  | {
      status: "dropped";
      placement?: { issue: number; action: "record" | "retry" };
    }
  | {
      status: "failed" | "skipped";
      placement?: { issue: number; action: "retry" };
    }
);

export type IssueOutcome =
  { action: "omit" | "retry" } | { action: "record"; drop?: GroomingGap };

export function batchIssueOutcome(
  issue: number,
  ungrouped: Readonly<Record<string, string>>,
  gaps: readonly GroomingGap[],
): IssueOutcome {
  if (Object.hasOwn(ungrouped, String(issue))) return { action: "omit" };
  let drop: GroomingGap | undefined;
  for (const gap of gaps) {
    if (gap.placement?.issue !== issue) continue;
    // A skipped or failed write takes precedence over a rule drop for the same issue.
    if (gap.status !== "dropped") return { action: "retry" };
    drop ??= gap;
  }
  return drop?.placement?.action === "retry"
    ? { action: "retry" }
    : { action: "record", drop };
}

export function isRecord(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

// Shared vocabulary for backlog grooming: the labels that mark ownership,
// the per-run caps, and parsePlan, the boundary every agent plan crosses
// before any rule reads it.

const LABEL_FEATURE = "agentics: feature";
const LABEL_INDEX = "agentics: backlog-index";
const CAPS = Object.freeze({ features: 5, links: 40, moves: 5 });
const BATCH_SIZE = 40;
const STICKY_MS = 7 * 24 * 60 * 60 * 1000;
const STALE_FEATURE_MS = 7 * 24 * 60 * 60 * 1000;
const BODY_LIMIT = 4000;
const COMMENT_LIMIT = 1000;
const COMMENTS_PER_ISSUE = 10;
const INDEX_LIMIT = 65000;
const MARKER = "agentics:backlog-grooming";
const LIMITS = Object.freeze({
  title: 200,
  outcome: 1000,
  why: 1000,
  reason: 300,
});
const REF = /^new-[1-9][0-9]*$/;
const PLAN_KEYS = ["features", "links", "moves", "featureUpdates", "ungrouped"];

function isBotFeature(issue: Pick<BacklogIssue, "labels">) {
  return issue.labels.includes(LABEL_FEATURE);
}

function isFeatureType(issue: Pick<BacklogIssue, "type">) {
  return issue.type === "Feature";
}

function isPositiveInt(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value > 0;
}

function text(
  errors: string[],
  where: string,
  value: unknown,
  limit: number,
  oneLine: boolean,
) {
  if (typeof value !== "string" || value.trim() === "") {
    errors.push(`${where} must be a nonempty string`);
    return "";
  }
  if (value.length > limit) errors.push(`${where} exceeds ${limit} characters`);
  if (oneLine && /[\r\n]/.test(value)) errors.push(`${where} must be one line`);
  return value.trim();
}

// A repeated dependency carries no extra meaning, so duplicates collapse
// rather than reject the plan.
function numbers(errors: string[], where: string, value: unknown): number[] {
  if (!Array.isArray(value) || !value.every(isPositiveInt)) {
    errors.push(`${where} must be an array of issue numbers`);
    return [];
  }
  return [...new Set(value)];
}

function issueNumber(errors: string[], where: string, value: unknown): number {
  if (isPositiveInt(value)) return value;
  errors.push(`${where} must be an issue number`);
  // Invalid fields cannot escape: any accumulated error rejects the entire plan.
  return 0;
}

function target(
  errors: string[],
  where: string,
  value: unknown,
  refs: Set<string>,
  allowNull: false,
): FeatureReference;
function target(
  errors: string[],
  where: string,
  value: unknown,
  refs: Set<string>,
  allowNull: true,
): FeatureReference | null;
function target(
  errors: string[],
  where: string,
  value: unknown,
  refs: Set<string>,
  allowNull: boolean,
): FeatureReference | null {
  if (allowNull && value === null) return null;
  if (isPositiveInt(value)) return value;
  if (typeof value === "string" && refs.has(value)) return value;
  errors.push(
    `${where} must be an issue number${allowNull ? ", null," : ""} or a declared new-N ref`,
  );
  return 0;
}

// entries maps each object entry with its original index. A non-object
// entry is reported once and skipped, so one fault yields one message.
function entries<T>(
  errors: string[],
  plan: Record<string, unknown>,
  key: string,
  map: (item: Record<string, unknown>, index: number) => T,
): T[] {
  const value = plan[key] === undefined ? [] : plan[key];
  if (!Array.isArray(value)) {
    errors.push(`${key} must be an array`);
    return [];
  }
  return value.flatMap((item, index) => {
    if (item === null || typeof item !== "object" || Array.isArray(item)) {
      errors.push(`${key}[${index}] must be an object`);
      return [];
    }
    return [map(item, index)];
  });
}

// Any error rejects the whole plan. Accepting the valid half of a malformed
// plan would let a broken entry decide which of its siblings survive.
function parsePlan(raw: string): ParsePlanResult {
  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch {
    return { plan: null, errors: ["plan is not valid JSON"] };
  }
  if (!isRecord(parsed)) {
    return { plan: null, errors: ["plan must be a JSON object"] };
  }
  const plan: Record<string, unknown> = parsed;
  const errors: string[] = [];
  for (const key of Object.keys(plan)) {
    if (!PLAN_KEYS.includes(key)) errors.push(`unknown key ${key}`);
  }

  const refs = new Set<string>();
  const features = entries(errors, plan, "features", (f, i) => {
    const where = `features[${i}]`;
    if (typeof f.ref !== "string" || !REF.test(f.ref)) {
      errors.push(`${where}.ref must match new-N`);
    } else if (refs.has(f.ref)) {
      errors.push(`${where}.ref ${f.ref} is declared twice`);
    } else {
      refs.add(f.ref);
    }
    if (f.single !== undefined && typeof f.single !== "boolean") {
      errors.push(`${where}.single must be a boolean`);
    }
    return {
      ref: typeof f.ref === "string" ? f.ref : "",
      title: text(errors, `${where}.title`, f.title, LIMITS.title, true),
      outcome: text(
        errors,
        `${where}.outcome`,
        f.outcome,
        LIMITS.outcome,
        false,
      ),
      why: text(errors, `${where}.why`, f.why, LIMITS.why, false),
      dependsOn: numbers(
        errors,
        `${where}.dependsOn`,
        f.dependsOn === undefined ? [] : f.dependsOn,
      ),
      single: f.single === true,
    };
  });

  const links = entries(errors, plan, "links", (l, i) => ({
    issue: issueNumber(errors, `links[${i}].issue`, l.issue),
    feature: target(errors, `links[${i}].feature`, l.feature, refs, false),
  }));

  const moves = entries(errors, plan, "moves", (m, i) => ({
    issue: issueNumber(errors, `moves[${i}].issue`, m.issue),
    to: target(errors, `moves[${i}].to`, m.to, refs, true),
    reason: text(errors, `moves[${i}].reason`, m.reason, LIMITS.reason, true),
  }));

  const updated = new Set<number>();
  const featureUpdates = entries(errors, plan, "featureUpdates", (u, i) => {
    const where = `featureUpdates[${i}]`;
    if (isPositiveInt(u.feature)) {
      if (updated.has(u.feature)) {
        errors.push(`${where}.feature #${u.feature} is updated twice`);
      }
      updated.add(u.feature);
    }
    return {
      feature: issueNumber(errors, `${where}.feature`, u.feature),
      title: text(errors, `${where}.title`, u.title, LIMITS.title, true),
      outcome: text(
        errors,
        `${where}.outcome`,
        u.outcome,
        LIMITS.outcome,
        false,
      ),
      why: text(errors, `${where}.why`, u.why, LIMITS.why, false),
      dependsOn: numbers(
        errors,
        `${where}.dependsOn`,
        u.dependsOn === undefined ? [] : u.dependsOn,
      ),
    };
  });

  const ungrouped = entries(errors, plan, "ungrouped", (u, i) => ({
    issue: issueNumber(errors, `ungrouped[${i}].issue`, u.issue),
    reason: text(
      errors,
      `ungrouped[${i}].reason`,
      u.reason,
      LIMITS.reason,
      true,
    ),
  }));

  const seen = new Set<number>();
  for (const item of [...links, ...moves, ...ungrouped]) {
    // An invalid issue number is already reported; it is not a duplicate.
    if (!isPositiveInt(item.issue)) continue;
    if (seen.has(item.issue)) {
      errors.push(
        `issue #${item.issue} appears in more than one of links, moves, and ungrouped`,
      );
    }
    seen.add(item.issue);
  }

  if (errors.length > 0) return { plan: null, errors };
  return {
    plan: { features, links, moves, featureUpdates, ungrouped },
    errors: [],
  };
}

export {
  LABEL_FEATURE,
  LABEL_INDEX,
  CAPS,
  BATCH_SIZE,
  STICKY_MS,
  STALE_FEATURE_MS,
  BODY_LIMIT,
  COMMENT_LIMIT,
  COMMENTS_PER_ISSUE,
  INDEX_LIMIT,
  MARKER,
  isBotFeature,
  isFeatureType,
  parsePlan,
};
