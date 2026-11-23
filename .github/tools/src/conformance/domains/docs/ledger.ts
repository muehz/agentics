import { readdirSync, readFileSync, statSync } from "node:fs";
import path from "node:path";
import { type AuditResult, errorMessage, type Failure } from "../../check.ts";
import { repoFileErrorOf } from "../../repo-file-error.ts";

export const LEDGER_REL = "docs/surfaces/README.md";

const PROBE_STAMP = /Verified on [^\n]+ \([0-9]{4}-[0-9]{2}-[0-9]{2}\)/;
const ROW_TARGET = /\]\(([a-z0-9-]+\.md)\)/g;
const DAY_MS = 86_400_000;

/**
 * The day a ledger row was last reviewed, as UTC midnight in ms, and the text it was written as. The date is the
 * fifth `|`-separated field: the "Last reviewed" column of a four-column row, and of a six-column one. A date in a
 * link or in another column never counts.
 */
function ledgerReviewDate(row: string): { ms: number; text: string } {
  const cells = row.trim().split("|");
  if (
    (cells.length !== 6 && cells.length !== 8) ||
    (cells[0] ?? "").trim() !== "" ||
    (cells[cells.length - 1] ?? "").trim() !== ""
  )
    throw new Error("ledger row must have four or six columns");
  const text = (cells[4] ?? "").trim();
  const ms = parseDay(text);
  if (ms === undefined)
    throw new Error(`invalid Last reviewed value ${JSON.stringify(text)}`);
  return { ms, text };
}

/** A YYYY-MM-DD calendar day as UTC midnight in ms, or undefined for any other text, including an impossible date. */
function parseDay(text: string): number | undefined {
  const match = /^([0-9]{4})-([0-9]{2})-([0-9]{2})$/.exec(text);
  if (match === null) return undefined;
  const [year, month, day] = [1, 2, 3].map((i) => Number(match[i]));
  if (year === undefined || month === undefined || day === undefined)
    return undefined;
  // Date.UTC treats years 0 to 99 as 1900 to 1999, so the year is set on its own.
  const date = new Date(0);
  date.setUTCFullYear(year, month - 1, day);
  // An out-of-range month or day rolls the date into another month.
  return date.getUTCMonth() === month - 1 ? date.getTime() : undefined;
}

/**
 * Whether a six-column row's Result cell opens with "Retired:". The colon is required because a live row's Result can
 * start with the verb ("Retired the stale probe claim") and must stay age-checked. A four-column row has no Result
 * cell, so it is never retired.
 */
function ledgerRowRetired(row: string): boolean {
  const cells = row.trim().split("|");
  return cells.length === 8 && (cells[6] ?? "").trim().startsWith("Retired:");
}

const fail = (rule: string, filePath: string, message: string): Failure => ({
  rule,
  path: filePath,
  message,
});

/** The text of the repo-relative file `rel`. A failure to read it throws a `RepoFileError`. */
function readRel(repoRoot: string, rel: string): string {
  try {
    return readFileSync(path.join(repoRoot, ...rel.split("/")), "utf8");
  } catch (error) {
    throw repoFileErrorOf(error, rel);
  }
}

/** The text of the repo-relative file `rel`, or undefined when it is missing. Any other failure to read it throws. */
function readIfPresent(repoRoot: string, rel: string): string | undefined {
  try {
    return readRel(repoRoot, rel);
  } catch (error) {
    if (repoFileErrorOf(error, rel).kind === "missing") return undefined;
    throw error;
  }
}

/** Whether a path can be stat-ed, following symlinks. Any stat error counts as absent. */
function exists(file: string): boolean {
  try {
    statSync(file);
    return true;
  } catch {
    return false;
  }
}

/**
 * Audits the upstream-review ledger, docs/surfaces/README.md: it exists; every surface doc has a row with a valid
 * review date and a pinned https:// source and carries an inline probe stamp; every row points at a doc that exists.
 * `maxAge` below 0 disarms the age check, which runs only after the structural checks pass.
 */
export function ledgerRules(
  repoRoot: string,
  maxAge: number,
  now: Date,
): AuditResult {
  try {
    return { failures: audit(repoRoot, maxAge, now) };
  } catch (error) {
    // An audit error drops the failures found so far.
    return { failures: [], error: errorMessage(error) };
  }
}

function audit(repoRoot: string, maxAge: number, now: Date): Failure[] {
  const failures: Failure[] = [];
  const surfaces = path.join(repoRoot, "docs", "surfaces");
  let ledger: string | undefined;
  try {
    ledger = readIfPresent(repoRoot, LEDGER_REL);
  } catch (error) {
    throw new Error(`docs: ${errorMessage(error)}`, { cause: error });
  }
  if (ledger === undefined)
    // A missing ledger violates the contract that the ledger exists.
    return [
      fail(
        "ledger-present",
        LEDGER_REL,
        "docs/surfaces/README.md (upstream review ledger) is missing",
      ),
    ];

  let entries;
  try {
    entries = readdirSync(surfaces, { withFileTypes: true });
  } catch (error) {
    const fileError = repoFileErrorOf(error, "docs/surfaces");
    throw new Error(`docs: ${fileError.message}`, { cause: fileError });
  }
  const surfaceDocs = entries
    .filter(
      (entry) =>
        !entry.isDirectory() &&
        entry.name.endsWith(".md") &&
        entry.name !== "README.md",
    )
    .map((entry) => entry.name)
    .sort();
  // Zero rules selected is never a pass.
  if (surfaceDocs.length === 0)
    throw new Error(
      "docs: no surface docs found under docs/surfaces — nothing to audit",
    );

  const ledgerLines = ledger.split("\n");
  for (const name of surfaceDocs) {
    const rel = `docs/surfaces/${name}`;
    // Every ledger row whose Agentics-doc column links to this file; the date and source checks apply to all of them.
    const link = `](${name})`;
    const rows = ledgerLines.filter(
      (line) => line.startsWith("|") && line.includes(link),
    );
    if (rows.length === 0) {
      failures.push(
        fail("ledger-row-present", LEDGER_REL, `no ledger row for ${rel}`),
      );
    } else {
      for (const row of rows) {
        try {
          ledgerReviewDate(row);
        } catch (error) {
          failures.push(
            fail(
              "ledger-row-date",
              LEDGER_REL,
              `ledger row for ${name} has invalid review date: ${errorMessage(error)}`,
            ),
          );
        }
      }
      if (!rows.join("\n").includes("https://"))
        failures.push(
          fail(
            "ledger-row-source",
            LEDGER_REL,
            `ledger row for ${name} pins no upstream source (https:// link)`,
          ),
        );
    }
    let doc: string;
    try {
      doc = readRel(repoRoot, rel);
    } catch (error) {
      throw new Error(`docs: ${errorMessage(error)}`, { cause: error });
    }
    if (!PROBE_STAMP.test(doc))
      failures.push(
        fail(
          "probe-stamp-present",
          rel,
          "carries no inline 'Verified on <CLI> <version> (YYYY-MM-DD)' probe stamp",
        ),
      );
  }

  // Reverse direction: every ledger row target must be a doc that exists (catches renames the forward loop cannot
  // see).
  const seen = new Set<string>();
  for (const line of ledgerLines) {
    if (!line.startsWith("|")) continue;
    for (const match of line.matchAll(ROW_TARGET)) {
      const target = match[1] ?? "";
      if (seen.has(target)) continue;
      seen.add(target);
      if (!exists(path.join(surfaces, target)))
        failures.push(
          fail(
            "ledger-doc-exists",
            LEDGER_REL,
            `ledger row references missing docs/surfaces/${target} (renamed or removed doc?)`,
          ),
        );
    }
  }

  // The opt-in age check runs over every table line carrying both a doc link and a date. One day of grace for
  // timezone skew; anything further in the future is a bad stamp. A retired surface gets no further reviews, so its
  // date can only grow older and the age budget would fail it forever; that date is still a stamp, so the validity
  // checks keep applying to it.
  if (maxAge >= 0 && failures.length === 0) {
    const today = Math.floor(now.getTime() / DAY_MS) * DAY_MS;
    for (const line of ledgerLines) {
      if (!line.startsWith("|")) continue;
      const doc = [...line.matchAll(ROW_TARGET)][0]?.[1];
      if (doc === undefined) continue;
      let stamp;
      try {
        stamp = ledgerReviewDate(line);
      } catch (error) {
        failures.push(
          fail(
            "ledger-row-date",
            LEDGER_REL,
            `ledger row for ${doc} has invalid review date: ${errorMessage(error)}`,
          ),
        );
        continue;
      }
      const ageDays = Math.trunc((today - stamp.ms) / DAY_MS);
      if (ageDays < -1)
        failures.push(
          fail(
            "review-date-future",
            LEDGER_REL,
            `${doc}: review date ${stamp.text} is in the future (invalid stamp)`,
          ),
        );
      else if (ageDays > maxAge && !ledgerRowRetired(line))
        failures.push(
          fail(
            "review-age",
            LEDGER_REL,
            `${doc}: last reviewed ${stamp.text} (${ageDays} days ago; budget ${maxAge})`,
          ),
        );
    }
  }
  return failures;
}
