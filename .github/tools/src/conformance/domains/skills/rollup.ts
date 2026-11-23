import type { Failure } from "../../check.ts";
import { isFileErrorOfKind, readRepoFile } from "./files.ts";

// The rendered shape of a triage rollup: an execution key line as the second line and no other, at most one
// earlier-pass link line, a verdict header that agrees with the table and the needs-you list and mentions the author
// only when something needs them, a closed status vocabulary, balanced rows, a fixed block order (the link line
// directly under the header), a blocked rollup with no table, no drafts, and no emoji.

/** The rule id of the rollup shape check. */
export const ROLLUP_SHAPE_RULE = "triage-rollup-shape";

/** The footer every triage emission closes with. */
export const ATTESTATION_FOOTER = "> _Triaged by an AI agent_";

/** The template whose rendered examples the rule checks. */
const TEMPLATE_PATH = "skills/triage/references/rollup-template.md";

/** Sibling reference carrying rendered rollup examples when split out of the template. */
const EXAMPLES_PATH = "skills/triage/references/rollup-details-and-examples.md";

// The non-ASCII characters of the rollup text, built from code points so the source stays ASCII.
const EM = String.fromCodePoint(0x2014);
const MIDDLE_DOT = String.fromCodePoint(0x00b7);

const ROLLUP_MARKER_LINE = "<!-- agentics:triage:rollup -->";
const NEEDS_YOU_HEADING = "### Needs you";
const DETAILS_OPEN_LINE = "<details><summary>Details</summary>";
const BASE_SYNC_ANCHOR = "<!-- agentics:triage:section:base-sync -->";
const BASE_SYNC_END = "<!-- agentics:triage:section-end:base-sync -->";
const SUPERSEDES_PREFIX = "Earlier triage runs posted per-source comments";
const LINK_PREFIX = "Earlier triage pass:";
const NOTHING_NEEDS_YOU = "Nothing needs you";
/** The header lead when nothing needs the author, who is then not mentioned. */
const QUIET_LEAD = "Triage";
const MIN_ROLLUP_EXAMPLES = 3;
const PENDING_SENTENCE =
  "Not handled " +
  EM +
  " no one was present to approve replies, so nothing was drafted or posted.";

// White space in a rollup is tab, LF, FF, CR and space only, so SPACE spells it out where JavaScript's \s would add
// the vertical tab and Unicode spaces. A line ends only at LF, so NOT_LF stands where JavaScript's dot would also stop
// at CR, U+2028 and U+2029. No pattern carries the m flag, so ^ and $ keep their whole-text meaning; where a line start
// is meant, the lookbehind below puts it after LF only.
const SPACE = "\\t\\n\\f\\r ";
const NOT_SPACE = "[^" + SPACE + "]";
const NOT_LF = "[^\\n]";
const LINE_START = "(?<=^|\\n)";

/** The fixed needs-you bullet of a `Pending Review` row for `source`. */
function pendingBullet(source: string): string {
  return (
    "- **" +
    source +
    ":** not handled " +
    EM +
    " replies here need someone to approve them, and no one was present."
  );
}
const ROLLUP_FENCE_RE = new RegExp(
  LINE_START +
    "```text\\n(" +
    ROLLUP_MARKER_LINE +
    "\\n.*?)" +
    LINE_START +
    "```",
  "gsu",
);
const PLACEHOLDER_RE = /\{[A-Z][A-Z_]*\}/u;
const EXECUTION_KEY_RE = /^<!-- agentics:triage:execution:[0-9]+-[0-9]+ -->$/u;
const HEADER_LINE_RE = new RegExp(
  "^(@" + NOT_SPACE + "+|" + QUIET_LEAD + ") " + EM + " (" + NOT_LF + "+)$",
  "u",
);
/** A line that sets out to be the header: an `@` mention, or the quiet lead and its dash. */
const HEADER_START_RE = new RegExp("^(@|" + QUIET_LEAD + " " + EM + ")", "u");
const FIXED_PART_RE = /^([1-9][0-9]*) fixed$/u;
const NEEDS_PART_RE = /^([1-9][0-9]*) needs you$/u;
/**
 * Any emoji GitHub renders, which the posting envelope forbids: a pictographic character, a regional indicator (flags
 * are pairs of them and hold no pictographic code point), or the combining keycap that turns a digit, `#` or `*` into
 * a keycap emoji.
 */
const EMOJI_RE = /[\p{Extended_Pictographic}\p{Regional_Indicator}\u{20E3}]/u;
const BLOCKED_SYNC_RE = new RegExp("^\\*\\*Base sync " + EM + " blocked", "u");
const SEPARATOR_RE = new RegExp(
  "^\\|([" + SPACE + "]*:?-+:?[" + SPACE + "]*\\|){7}[" + SPACE + "]*$",
  "u",
);
const COUNT_CELL_RE = /^(\*\*)?(0|[1-9][0-9]*)(\*\*)?$/u;

const ROLLUP_COLUMNS: readonly string[] = [
  "Source",
  "Status",
  "Found",
  "Fixed",
  "Accepted",
  "Dismissed",
  "Needs you",
];
const ROLLUP_STATUSES: ReadonlySet<string> = new Set([
  "Completed",
  "Partial",
  "Failed",
  "Skipped",
  "No new findings",
  "Pending Review",
  "Push Blocked",
  "Push Skipped",
]);
const LANDED_STATUSES: ReadonlySet<string> = new Set(["Completed", "Partial"]);
const STALLED_STATUSES: ReadonlySet<string> = new Set([
  "Failed",
  "Pending Review",
]);
const PUSH_STATUSES: ReadonlySet<string> = new Set([
  "Push Blocked",
  "Push Skipped",
]);

// Counts are signed 64-bit integers: a count that overflows reads as the maximum, and sums wrap. BigInt carries both,
// and prints as plain decimal text.
const MAX_INT64 = (1n << 63n) - 1n;
const wrap = (value: bigint): bigint => BigInt.asIntN(64, value);

/** The value of a run of decimal digits; an overflow gives the maximum 64-bit value. */
function atoi(digits: string): bigint {
  const value = BigInt(digits);
  return value > MAX_INT64 ? MAX_INT64 : value;
}

/** One source row. `counts` are Found, Fixed, Accepted, Dismissed and Needs you; -1 is a dash cell. */
type RollupRow = { source: string; status: string; counts: bigint[] };

const counted = (row: RollupRow): boolean => (row.counts[0] ?? -1n) >= 0n;

/** The count at `index`, with a dash cell reading as 0. */
function countAt(row: RollupRow, index: number): bigint {
  const value = row.counts[index] ?? -1n;
  return value < 0n ? 0n : value;
}

/** A list as messages show it: the elements joined by one space, in brackets. */
const listText = (items: readonly string[]): string =>
  "[" + items.join(" ") + "]";

function indexWhere(
  lines: readonly string[],
  test: (line: string) => boolean,
): number {
  return lines.findIndex(test);
}

function indexOf(lines: readonly string[], want: string): number {
  return indexWhere(lines, (line) => line === want);
}

/** `text` with every trailing LF removed; other trailing whitespace stays. */
function trimTrailingLf(text: string): string {
  let end = text.length;
  while (end > 0 && text[end - 1] === "\n") end -= 1;
  return text.slice(0, end);
}

/** `text` with every leading and trailing pipe removed. */
function trimPipes(text: string): string {
  let start = 0;
  let end = text.length;
  while (start < end && text[start] === "|") start += 1;
  while (end > start && text[end - 1] === "|") end -= 1;
  return text.slice(start, end);
}

function splitRow(line: string): string[] {
  return trimPipes(line.trim())
    .split("|")
    .map((cell) => cell.trim());
}

/** A count cell: a whole number, bold in the Total row, or a dash (returned as -1). Anything else is malformed. */
function parseCell(text: string): { value: bigint; ok: boolean } {
  if (text === EM) return { value: -1n, ok: true };
  const match = COUNT_CELL_RE.exec(text);
  if (match === null || (match[1] ?? "") !== (match[3] ?? ""))
    return { value: -1n, ok: false };
  return { value: atoi(match[2] ?? ""), ok: true };
}

/** The header's counts, whether it parsed, and whether it leads with a mention. */
type Verdict = { fixed: bigint; needs: bigint; ok: boolean; mentions: boolean };

/** Reads the fixed and needs-you counts, and the lead, from the header line. */
function parseVerdict(line: string): Verdict {
  const match = HEADER_LINE_RE.exec(line);
  const bad: Verdict = { fixed: 0n, needs: 0n, ok: false, mentions: false };
  if (match === null) return bad;
  const mentions = (match[1] ?? "").startsWith("@");
  const verdict = match[2] ?? "";
  if (verdict === NOTHING_NEEDS_YOU)
    return { fixed: 0n, needs: 0n, ok: true, mentions };
  const parts = verdict.split(" " + MIDDLE_DOT + " ");
  if (parts.length > 2) return bad;
  let fixed = 0n;
  let needs = 0n;
  for (const [i, part] of parts.entries()) {
    const fixedMatch = FIXED_PART_RE.exec(part);
    if (fixedMatch !== null && i === 0) {
      fixed = atoi(fixedMatch[1] ?? "");
      continue;
    }
    const needsMatch = NEEDS_PART_RE.exec(part);
    if (needsMatch !== null && needs === 0n) {
      needs = atoi(needsMatch[1] ?? "");
      continue;
    }
    return bad;
  }
  return { fixed, needs, ok: true, mentions };
}

/** Reads the source rows from the table header at `at` and checks the columns, statuses, row balance and Total row. */
function parseRollupTable(
  lines: readonly string[],
  at: number,
): { rows: RollupRow[]; msgs: string[] } {
  if (at < 0) return { rows: [], msgs: [] };
  const msgs: string[] = [];
  const got = splitRow(lines[at] ?? "");
  if (got.join("|") !== ROLLUP_COLUMNS.join("|")) {
    msgs.push(
      `table columns are ${listText(got)}, want ${listText(ROLLUP_COLUMNS)}`,
    );
    return { rows: [], msgs };
  }
  // Without the separator row GitHub renders the lines as prose, not a table.
  if (at + 1 >= lines.length || !SEPARATOR_RE.test(lines[at + 1] ?? "")) {
    msgs.push("the table has no separator row under its header");
    return { rows: [], msgs };
  }
  const rows: RollupRow[] = [];
  let total: string[] | undefined;
  for (const line of lines.slice(at + 2)) {
    if (!line.startsWith("|")) break;
    const cells = splitRow(line);
    if (cells.length !== ROLLUP_COLUMNS.length) {
      msgs.push(
        `row has ${cells.length} cells, want ${ROLLUP_COLUMNS.length}: ${line}`,
      );
      continue;
    }
    if (cells[0] === "**Total**") {
      total = cells;
      continue;
    }
    const row: RollupRow = {
      source: cells[0] ?? "",
      status: cells[1] ?? "",
      counts: [-1n, -1n, -1n, -1n, -1n],
    };
    if (!ROLLUP_STATUSES.has(row.status))
      msgs.push(
        `row ${JSON.stringify(row.source)} has status ${JSON.stringify(row.status)}, outside the eight statuses`,
      );
    for (let i = 0; i < 5; i += 1) {
      const cell = cells[i + 2] ?? "";
      const { value, ok } = parseCell(cell);
      if (!ok)
        msgs.push(
          `row ${JSON.stringify(row.source)} has a malformed ${ROLLUP_COLUMNS[i + 2]} cell ${JSON.stringify(cell)}; a count is a whole number or ` +
            "`" +
            EM +
            "`",
        );
      row.counts[i] = value;
    }
    if (
      counted(row) &&
      wrap(
        countAt(row, 1) + countAt(row, 2) + countAt(row, 3) + countAt(row, 4),
      ) !== row.counts[0]
    )
      msgs.push(
        `row ${JSON.stringify(row.source)} does not balance: Fixed + Accepted + Dismissed + Needs you != Found`,
      );
    rows.push(row);
  }
  // A Total sums counts, so a table of dash rows (a blocked sync) has none.
  const anyCounted = rows.some(counted);
  const needTotal = rows.length >= 2 && anyCounted;
  if (needTotal && total === undefined)
    msgs.push("two or more rows need a Total row");
  else if (!needTotal && total !== undefined)
    msgs.push("a Total row needs two or more rows with counts");
  else if (total !== undefined) {
    for (let i = 0; i < 5; i += 1) {
      let sum = 0n;
      for (const row of rows) sum = wrap(sum + countAt(row, i));
      const { value, ok } = parseCell(total[i + 2] ?? "");
      if (!ok || value !== sum)
        msgs.push(
          `Total ${ROLLUP_COLUMNS[i + 2]} is ${value}, rows sum to ${sum}`,
        );
    }
  }
  return { rows, msgs };
}

/**
 * Checks the Details block: it opens with a `####` heading, base sync is its first block, and every source has a
 * heading inside the fold, after detection notes and in table order.
 */
function checkDetails(
  lines: readonly string[],
  at: number,
  rows: readonly RollupRow[],
): string[] {
  const msgs: string[] = [];
  // The fold ends at the `</details>` that matches its opening line, found by nesting depth: section folds nest inside
  // it, and a sibling fold after it must not extend it. Searching past it would accept a block moved out.
  let end = lines.length - 1;
  let depth = 0;
  for (let i = at; i < lines.length; i += 1) {
    const line = lines[i] ?? "";
    if (line.startsWith("<details")) depth += 1;
    else if (line === "</details>") depth -= 1;
    if (depth === 0) {
      end = i;
      break;
    }
  }
  const fold = lines.slice(at, end + 1);
  let first = -1;
  for (let i = at + 1; i < lines.length; i += 1) {
    if ((lines[i] ?? "").trim() !== "") {
      first = i;
      break;
    }
  }
  if (first < 0 || !(lines[first] ?? "").startsWith("#### "))
    msgs.push("Details does not open with a `####` heading");
  const sync = indexOf(lines, BASE_SYNC_ANCHOR);
  if (sync >= 0) {
    if (sync < at || sync > end)
      msgs.push("base sync renders outside Details; only a blocked sync may");
    else if (first < 0 || lines[first] !== "#### Base sync")
      msgs.push("base sync is not the first block in Details");
  }
  // Blocks run base sync, detection notes, then one per source in table order, so a reader scanning the fold meets them
  // in the table's order.
  const endAt = indexOf(fold, BASE_SYNC_END);
  if (endAt >= 0) {
    for (const line of fold.slice(endAt + 1)) {
      if (line.startsWith("#### ") || line === "</details>") break;
      if (line.trim() !== "") {
        msgs.push(
          "content after the base sync section has no heading; detection notes need a `#### Detection notes` heading: " +
            line,
        );
        break;
      }
    }
  }
  const notes = indexOf(fold, "#### Detection notes");
  let prev = -1;
  let prevSource = "";
  for (const row of rows) {
    const pos = indexOf(fold, "#### " + row.source);
    if (pos < 0) {
      msgs.push(
        `Details has no \`#### ${row.source}\` heading inside the fold`,
      );
      continue;
    }
    if (notes >= 0 && pos < notes)
      msgs.push(
        `the \`#### ${row.source}\` block comes before \`#### Detection notes\``,
      );
    if (pos < prev)
      msgs.push(
        `source blocks are out of table order: \`#### ${row.source}\` comes before \`#### ${prevSource}\``,
      );
    prev = pos;
    prevSource = row.source;
  }
  return msgs;
}

/** The source's needs-you bullet and every line of its Details block, the places an unattended run must not name a reviewer. */
function pendingReviewLines(
  lines: readonly string[],
  source: string,
): string[] {
  const out: string[] = [];
  let inside = false;
  for (const line of lines) {
    if (line.startsWith("- **" + source)) out.push(line);
    else if (line === "#### " + source) inside = true;
    else if (inside && (line.startsWith("#### ") || line === "</details>"))
      inside = false;
    else if (inside) out.push(line);
  }
  return out;
}

/** Counts the `- ` lines directly under the needs-you heading. */
function countBullets(lines: readonly string[], at: number): number {
  if (at < 0) return 0;
  let n = 0;
  for (const line of lines.slice(at + 1)) {
    if (line.startsWith("- ")) n += 1;
    else if (line.trim() === "" && n === 0) continue;
    else return n;
  }
  return n;
}

/** One message per shape violation in a rendered rollup, in the order the checks run. */
export function checkRollupBody(body: string): string[] {
  const lines = trimTrailingLf(body.replaceAll("\r\n", "\n")).split("\n");
  const msgs: string[] = [];

  if (
    body.includes("agentics:triage:drafts") ||
    body.includes("Drafted Responses")
  )
    msgs.push("carries a drafts marker or a Drafted Responses section");
  if (lines[lines.length - 1] !== ATTESTATION_FOOTER)
    msgs.push("the attestation footer is not the last line");
  const emojiAt = indexWhere(lines, (line) => EMOJI_RE.test(line));
  if (emojiAt >= 0)
    msgs.push(`carries an emoji, which no posted body may: ${lines[emojiAt]}`);

  // The execution key ties the rollup to the command that posted it. A create retry, or a follow-up that lost the
  // comment id, finds the comment by this key, so a second key line would make that lookup ambiguous.
  if (!EXECUTION_KEY_RE.test(lines[1] ?? ""))
    msgs.push("no execution key line directly after the rollup marker");
  if (lines.filter((line) => EXECUTION_KEY_RE.test(line)).length > 1)
    msgs.push("more than one execution key line");

  // The link line points at the earlier rollup of the same PR, and one rollup links one earlier pass.
  if (lines.filter((line) => line.startsWith(LINK_PREFIX)).length > 1)
    msgs.push("more than one earlier-pass link line");
  const linkAt = indexWhere(lines, (line) => line.startsWith(LINK_PREFIX));

  const header = indexWhere(lines, (line) => HEADER_START_RE.test(line));
  if (header < 0) {
    msgs.push("no header verdict line");
    return msgs;
  }
  const verdict = parseVerdict(lines[header] ?? "");
  const { fixed, needs, ok, mentions } = verdict;
  if (!ok)
    msgs.push(
      "header verdict is not `N fixed " +
        MIDDLE_DOT +
        " M needs you` or `" +
        NOTHING_NEEDS_YOU +
        "`: " +
        lines[header],
    );
  // The rollup mentions the author once, and only when it needs something from them.
  if (ok && needs > 0n && !mentions)
    msgs.push(
      "the header must lead with the author's mention when something needs them: " +
        lines[header],
    );
  if (ok && needs === 0n && mentions)
    msgs.push(
      "the header mentions the author although nothing needs them: " +
        lines[header],
    );

  const blocked = indexWhere(lines, (line) => BLOCKED_SYNC_RE.test(line));
  const needsAt = indexOf(lines, NEEDS_YOU_HEADING);
  const tableAt = indexWhere(lines, (line) => line.startsWith("| Source |"));
  const detailsAt = indexOf(lines, DETAILS_OPEN_LINE);
  const supersedesAt = indexWhere(lines, (line) =>
    line.startsWith(SUPERSEDES_PREFIX),
  );

  const table = parseRollupTable(lines, tableAt);
  const rows = table.rows;
  msgs.push(...table.msgs);

  // Block order: header, link line, blocked base sync, supersedes, needs-you, table, Details.
  const order: ReadonlyArray<{ name: string; at: number }> = [
    { name: "header", at: header },
    { name: "link line", at: linkAt },
    { name: "blocked base sync", at: blocked },
    { name: "supersedes line", at: supersedesAt },
    { name: "needs-you list", at: needsAt },
    { name: "table", at: tableAt },
    { name: "Details", at: detailsAt },
  ];
  let last = -1;
  let lastName = "";
  for (const block of order) {
    if (block.at < 0) continue;
    if (block.at < last)
      msgs.push(
        `${block.name} appears before ${lastName}; the layout order is header, link line, blocked base sync, supersedes, needs-you, table, Details`,
      );
    last = block.at;
    lastName = block.name;
  }

  // The order check only compares blocks it recognises, so prose it does not know could sit between the verdict and the
  // link line. When no recognised block stands between them, only blank lines may.
  if (
    linkAt > header &&
    order.every(
      (block) => block.at < 0 || block.at <= header || block.at >= linkAt,
    ) &&
    lines.slice(header + 1, linkAt).some((line) => line.trim() !== "")
  )
    msgs.push("the link line is not directly under the header");

  if (blocked >= 0) {
    if (detailsAt >= 0 || needsAt >= 0)
      msgs.push(
        "a blocked base sync renders alone: no needs-you list and no Details",
      );
    if (ok && (fixed !== 0n || needs !== 1n))
      msgs.push("a blocked base sync's header is `1 needs you`");
    // Carry-forward reads only the execution's own rollup, and a blocked command stops before any source runs, so no
    // row exists to list.
    if (tableAt >= 0) msgs.push("a blocked rollup renders no table");
    return msgs;
  }

  if (indexOf(lines, BASE_SYNC_ANCHOR) < 0)
    msgs.push(
      `no base sync block: every rollup that is not blocked carries ${BASE_SYNC_ANCHOR}`,
    );

  if (tableAt < 0) msgs.push("no source table");
  if (detailsAt < 0) msgs.push("no Details block");
  else msgs.push(...checkDetails(lines, detailsAt, rows));

  let wantFixed = 0n;
  let wantNeeds = 0n;
  let push = false;
  for (const row of rows) {
    if (LANDED_STATUSES.has(row.status))
      wantFixed = wrap(wantFixed + countAt(row, 1));
    wantNeeds = wrap(wantNeeds + countAt(row, 4));
    if (STALLED_STATUSES.has(row.status)) wantNeeds = wrap(wantNeeds + 1n);
    if (PUSH_STATUSES.has(row.status)) push = true;
  }
  if (push) wantNeeds = wrap(wantNeeds + 1n);
  const bullets = countBullets(lines, needsAt);
  if (ok && fixed !== wantFixed)
    msgs.push(
      `header says ${fixed} fixed, but the table's landed Fixed cells sum to ${wantFixed}`,
    );
  if (ok && needs !== wantNeeds)
    msgs.push(
      `header says ${needs} needs you, but the table implies ${wantNeeds}`,
    );
  if (ok && needs !== BigInt(bullets))
    msgs.push(
      `header says ${needs} needs you, but the needs-you list has ${bullets} bullets`,
    );

  for (const row of rows) {
    if (row.status !== "Pending Review") continue;
    if (counted(row))
      msgs.push(
        "a `Pending Review` row renders `" +
          EM +
          "` in every count cell; row " +
          JSON.stringify(row.source) +
          " has counts",
      );
    let bulletCount = 0;
    let sentenceCount = 0;
    for (const line of pendingReviewLines(lines, row.source)) {
      if (line.trim() === "") continue;
      if (line.startsWith("- **" + row.source)) {
        bulletCount += 1;
        if (line !== pendingBullet(row.source))
          msgs.push("pending review bullet is not the pinned text: " + line);
      } else if (line === PENDING_SENTENCE) sentenceCount += 1;
      else
        msgs.push("pending review Details is not the pinned sentence: " + line);
    }
    if (bulletCount === 0)
      msgs.push("pending review is missing the pinned needs-you bullet");
    if (bulletCount > 1)
      msgs.push("pending review repeats the pinned needs-you bullet");
    if (sentenceCount === 0)
      msgs.push("pending review is missing the pinned Details sentence");
    if (sentenceCount > 1)
      msgs.push("pending review repeats the pinned Details sentence");
  }
  return msgs;
}

/**
 * The triage-rollup-shape rule: checks every rendered rollup example in the template and, when present, its sibling
 * examples reference. A fence holding a `{PLACEHOLDER}` is the layout skeleton, not an example. A read error on the
 * template, or a non-missing read error on the sibling reference, is thrown unchanged.
 */
export function rollupShapeFailures(repoRoot: string): Failure[] {
  const sources: Array<{ rel: string; text: string }> = [
    { rel: TEMPLATE_PATH, text: readRepoFile(repoRoot, TEMPLATE_PATH) },
  ];
  try {
    sources.push({
      rel: EXAMPLES_PATH,
      text: readRepoFile(repoRoot, EXAMPLES_PATH),
    });
  } catch (error) {
    if (!isFileErrorOfKind(error, "missing")) throw error;
  }
  const failures: Failure[] = [];
  let n = 0;
  for (const { rel, text } of sources) {
    for (const match of text.matchAll(ROLLUP_FENCE_RE)) {
      const body = match[1] ?? "";
      if (PLACEHOLDER_RE.test(body)) continue;
      n += 1;
      for (const msg of checkRollupBody(body))
        failures.push({
          rule: ROLLUP_SHAPE_RULE,
          path: rel,
          message: `example ${n}: ${msg}`,
        });
    }
  }
  if (n < MIN_ROLLUP_EXAMPLES)
    failures.push({
      rule: ROLLUP_SHAPE_RULE,
      path: TEMPLATE_PATH,
      message: `found ${n} rendered rollup examples, want at least ${MIN_ROLLUP_EXAMPLES}`,
    });
  return failures;
}
