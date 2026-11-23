import { readFileSync } from "node:fs";
import path from "node:path";
import { repoFileErrorOf } from "../../repo-file-error.ts";

/** The repo-relative home of the order table. */
export const LADDER_PATH = "skills/routing/references/model-ladders.md";

/** The repo-relative home of the pin table: identifiers Agentics binds or cites deliberately off the ladder. */
export const PIN_PATH = "docs/maintainers/model-ids.md";

/** One row of the pin table. */
export interface Pin {
  usedIn: string;
  reason: string;
}

/** The parsed order table and pin table. */
export interface Ladder {
  /** Maps a surface cell to its identifiers, lower to higher. */
  surfaces: Map<string, string[]>;
  /** Maps an off-ladder identifier to its row. */
  pins: Map<string, Pin>;
}

const ORDER_HEADER = ["Surface", "Lower to higher model position"];
const PIN_HEADER = ["Identifier", "Used in", "Reason"];

/** Whether an identifier is on any ladder row or has a pin row. */
export function isKnown(ladder: Ladder, id: string): boolean {
  if (ladder.pins.has(id)) return true;
  for (const ids of ladder.surfaces.values()) if (ids.includes(id)) return true;
  return false;
}

/** Reads the order table from LADDER_PATH and the pin table from PIN_PATH under the checkout. */
export function loadLadder(repoRoot: string): Ladder {
  const texts = [LADDER_PATH, PIN_PATH].map((rel) => {
    try {
      return readFileSync(path.join(repoRoot, ...rel.split("/")), "utf8");
    } catch (error) {
      const fileError = repoFileErrorOf(error, rel);
      throw new Error(`modelids: ${fileError.message}`, { cause: fileError });
    }
  });
  return parseLadder(texts[0] ?? "", texts[1] ?? "");
}

const splitLines = (text: string) => text.replaceAll("\r\n", "\n").split("\n");

const same = (a: string[] | null, b: string[]) =>
  a !== null && a.length === b.length && a.every((cell, i) => cell === b[i]);

/**
 * Reads the order table from ladderText, the text of LADDER_PATH, and the pin table from pinText, the text of
 * PIN_PATH. Each table appears exactly once in its own text, and neither text may hold the other's table: a copy left
 * behind would look authoritative to a reader of that file while the loader ignores it. The order table needs at
 * least one row, and each row names at least one backticked identifier. The pin table may be empty; each of its rows
 * names one identifier, where it is used, and why.
 */
export function parseLadder(ladderText: string, pinText: string): Ladder {
  const ladderLines = splitLines(ladderText);
  const pinLines = splitLines(pinText);
  const strays = countTables(ladderLines, PIN_HEADER);
  if (strays !== 0)
    throw new Error(
      `modelids: ${LADDER_PATH} has ${strays} tables headed ${JSON.stringify(PIN_HEADER.join(" | "))}, want 0; the pin table belongs in ${PIN_PATH}`,
    );
  const orderStrays = countTables(pinLines, ORDER_HEADER);
  if (orderStrays !== 0)
    throw new Error(
      `modelids: ${PIN_PATH} has ${orderStrays} tables headed ${JSON.stringify(ORDER_HEADER.join(" | "))}, want 0; the order table belongs in ${LADDER_PATH}`,
    );
  const order = tableRows(ladderLines, ORDER_HEADER, LADDER_PATH);
  if (order.length === 0)
    throw new Error(`modelids: ${LADDER_PATH} order table has no rows`);
  const pins = tableRows(pinLines, PIN_HEADER, PIN_PATH);
  const ladder: Ladder = { surfaces: new Map(), pins: new Map() };
  for (const row of order) {
    const surface = row[0] ?? "";
    const ids = backtickedIds(row[1] ?? "");
    if (surface === "" || ids.length === 0)
      throw new Error(
        `modelids: order row ${JSON.stringify(surface)} names no backticked identifier`,
      );
    ladder.surfaces.set(surface, ids);
  }
  for (const row of pins) {
    const ids = backtickedIds(row[0] ?? "");
    const usedIn = row[1] ?? "";
    const reason = row[2] ?? "";
    const id = ids[0];
    if (ids.length !== 1 || id === undefined || usedIn === "" || reason === "")
      throw new Error(
        `modelids: pin row ${JSON.stringify(row[0] ?? "")} must name one backticked identifier, where it is used, and a reason`,
      );
    if (ladder.pins.has(id))
      throw new Error(
        `modelids: identifier ${JSON.stringify(id)} is pinned twice`,
      );
    ladder.pins.set(id, { usedIn, reason });
  }
  return ladder;
}

/** How many lines are a table header equal to header. */
function countTables(lines: string[], header: string[]): number {
  return lines.filter((line) => same(cells(line), header)).length;
}

/**
 * The body rows of the one table whose header cells equal header. The line after the header is its separator row and
 * is skipped. `source` names the file the lines came from in an error.
 */
function tableRows(
  lines: string[],
  header: string[],
  source: string,
): string[][] {
  const rows: string[][] = [];
  let found = 0;
  lines.forEach((line, i) => {
    if (!same(cells(line), header)) return;
    found += 1;
    for (const body of lines.slice(Math.min(i + 2, lines.length))) {
      const row = cells(body);
      if (row === null) break;
      if (row.length !== header.length)
        throw new Error(
          `modelids: row ${JSON.stringify(body.trim())} has ${row.length} cells, want ${header.length}`,
        );
      rows.push(row);
    }
  });
  if (found !== 1)
    throw new Error(
      `modelids: ${source} has ${found} tables headed ${JSON.stringify(header.join(" | "))}, want 1`,
    );
  return rows;
}

/** One Markdown table row split into trimmed cells, or null for a line that is not a table row. */
function cells(line: string): string[] | null {
  const trimmed = line.trim();
  if (trimmed.length < 2 || !trimmed.startsWith("|") || !trimmed.endsWith("|"))
    return null;
  return trimmed
    .slice(1, -1)
    .split("|")
    .map((cell) => cell.trim());
}

function backtickedIds(cell: string): string[] {
  return [...cell.matchAll(/`([^`]+)`/g)].map((match) => match[1] ?? "");
}
