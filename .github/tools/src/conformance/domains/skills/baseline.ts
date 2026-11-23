import {
  mkdirSync,
  readdirSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import path from "node:path";
import type { Failure } from "../../check.ts";
import { repoFileErrorOf } from "../../repo-file-error.ts";
import { auditFileError } from "./files.ts";

/** A finding with the stable key that a baseline entry matches. The key never reaches the printed output. */
export interface KeyedFailure extends Failure {
  key: string;
}

/** One accepted finding, recorded in the baseline file of the skill that owns its path. */
export interface BaselineEntry {
  rule: string;
  path: string;
  key: string;
}

/** The repo-relative directory that holds one `<skill>.json` baseline file per skill. */
export const BASELINE_DIR = ".github/tools/src/conformance/skill-baseline";

/** The rule reported for a baseline entry that covers no finding. It is never accepted in a baseline. */
export const BASELINE_STALE_RULE = "skill-baseline-stale";

const SUFFIX = ".json";

function identity(item: { rule: string; path: string; key: string }): string {
  return `${item.rule}\0${item.path}\0${item.key}`;
}

function isEntry(value: unknown): value is BaselineEntry {
  if (typeof value !== "object" || value === null) return false;
  const record = value as Record<string, unknown>;
  return (
    typeof record.rule === "string" &&
    typeof record.path === "string" &&
    typeof record.key === "string"
  );
}

function parseFile(
  repoRoot: string,
  rel: string,
  skill: string,
  rules: ReadonlySet<string>,
): BaselineEntry[] {
  let text: string;
  try {
    text = readFileSync(path.join(repoRoot, ...rel.split("/")), "utf8");
  } catch (error) {
    throw auditFileError("reading", rel, repoFileErrorOf(error, rel));
  }
  let data: unknown;
  try {
    data = JSON.parse(text);
  } catch (error) {
    throw new Error(`skills: ${rel}: is not valid JSON`, { cause: error });
  }
  if (!Array.isArray(data))
    throw new Error(`skills: ${rel}: want a JSON array of entries`);
  const prefix = `skills/${skill}/`;
  return data.map((value: unknown, i) => {
    if (!isEntry(value))
      throw new Error(
        `skills: ${rel}: entry ${i} needs string rule, path and key`,
      );
    if (!rules.has(value.rule))
      throw new Error(
        `skills: ${rel}: entry ${i} names unknown rule ${value.rule}`,
      );
    if (!value.path.startsWith(prefix))
      throw new Error(
        `skills: ${rel}: entry ${i} path ${value.path} is not under ${prefix}`,
      );
    return { rule: value.rule, path: value.path, key: value.key };
  });
}

/**
 * Reads every baseline file, keyed by its repo-relative path, in default sort order. A missing directory is an empty
 * baseline, and any other failure to list it throws `skills: reading <dir>: <file error>`. Throws on any entry that is
 * not a `<skill>.json` file and on any malformed file or entry; `rules` lists the rules a baseline may accept.
 */
export function readBaseline(
  repoRoot: string,
  rules: ReadonlySet<string>,
): Map<string, BaselineEntry[]> {
  const baseline = new Map<string, BaselineEntry[]>();
  let names: string[];
  try {
    names = readdirSync(path.join(repoRoot, ...BASELINE_DIR.split("/")));
  } catch (error) {
    const fileError = repoFileErrorOf(error, BASELINE_DIR);
    if (fileError.kind === "missing") return baseline;
    throw auditFileError("reading", BASELINE_DIR, fileError);
  }
  names.sort();
  for (const name of names) {
    const rel = `${BASELINE_DIR}/${name}`;
    // A directory whose name ends in the suffix fails as a read error in parseFile.
    if (!name.endsWith(SUFFIX))
      throw new Error(`skills: reading ${rel}: want <skill>${SUFFIX}`);
    const skill = name.slice(0, -SUFFIX.length);
    baseline.set(rel, parseFile(repoRoot, rel, skill, rules));
  }
  return baseline;
}

/**
 * Drops each finding a baseline entry covers and strips the key from the rest. Then it adds one stale finding, at
 * the baseline file's path, for each distinct entry that covered nothing. Kept findings stay in input order; stale
 * findings follow in baseline file default sort order, then entry order.
 */
export function applyBaseline(
  findings: readonly KeyedFailure[],
  baseline: ReadonlyMap<string, readonly BaselineEntry[]>,
): Failure[] {
  const accepted = new Set<string>();
  for (const entries of baseline.values())
    for (const entry of entries) accepted.add(identity(entry));
  const used = new Set<string>();
  const out: Failure[] = [];
  for (const finding of findings) {
    const id = identity(finding);
    if (accepted.has(id)) {
      used.add(id);
      continue;
    }
    out.push({
      rule: finding.rule,
      path: finding.path,
      message: finding.message,
    });
  }
  const files = [...baseline.keys()].sort();
  const reported = new Set<string>();
  for (const file of files) {
    for (const entry of baseline.get(file) ?? []) {
      const id = identity(entry);
      if (used.has(id) || reported.has(id)) continue;
      reported.add(id);
      out.push({
        rule: BASELINE_STALE_RULE,
        path: file,
        message: `baseline entry matches no finding: ${JSON.stringify({
          rule: entry.rule,
          path: entry.path,
          key: entry.key,
        })}`,
      });
    }
  }
  return out;
}

function compareText(a: string, b: string): number {
  if (a < b) return -1;
  return a > b ? 1 : 0;
}

function compareEntries(a: BaselineEntry, b: BaselineEntry): number {
  return (
    compareText(a.path, b.path) ||
    compareText(a.rule, b.rule) ||
    compareText(a.key, b.key)
  );
}

/** Replaces BASELINE_DIR with one sorted, de-duplicated <skill>.json per skill that has findings. */
export function writeBaselineFiles(
  repoRoot: string,
  findings: readonly KeyedFailure[],
): void {
  const dir = path.join(repoRoot, ...BASELINE_DIR.split("/"));
  rmSync(dir, { recursive: true, force: true });
  mkdirSync(dir, { recursive: true });
  const bySkill = new Map<string, Map<string, BaselineEntry>>();
  for (const finding of findings) {
    const skill = finding.path.split("/")[1] ?? "";
    let entries = bySkill.get(skill);
    if (entries === undefined) {
      entries = new Map();
      bySkill.set(skill, entries);
    }
    entries.set(identity(finding), {
      rule: finding.rule,
      path: finding.path,
      key: finding.key,
    });
  }
  for (const [skill, entries] of bySkill) {
    const sorted = [...entries.values()].sort(compareEntries);
    writeFileSync(
      path.join(dir, `${skill}${SUFFIX}`),
      JSON.stringify(sorted, null, 4) + "\n",
    );
  }
}
