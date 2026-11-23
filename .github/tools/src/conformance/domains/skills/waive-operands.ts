import type { Failure } from "../../check.ts";
import { isFileErrorOfKind } from "./files.ts";
import { SkillsTreeModel } from "./tree-model.ts";
import type { Family } from "./index.ts";

export const WAIVE_OPERANDS_RULE = "flow-waive-operands";

/** The record contract whose `## Operands` table lists every `waive` head and the file that emits it. */
export const OPERANDS_FILE = "skills/execute/references/flow-state.md";

/** One listed head and the repository path its **Emitted in** cell names first. */
export type Operand = { head: string; emittedIn: string };

/** A finding with the 1-based line it sorts by; 0 for a finding about the whole file. */
type Located = { failure: Failure; line: number };

/** One `waive` or `demote` target a skill file writes, as the scan found it. */
type Emission = {
  verb: "waive" | "demote";
  target: string;
  line: number;
};

const CODE_SPAN = /`([^`\n]+)`/g;
const SPAN_FORM = /^(waive|demote) (.+)$/;
// The target is either one double-quoted run or one run that ends at a space or a closing backtick.
const COMMAND_FORM = /agx flow log [^\s`]+ (waive|demote) ("[^"\n]*"|[^\s`]+)/g;
const FLOW_LOG_CALL = "agx flow log";
const PLACEHOLDER = /^<[^>]*>$/;
// The grammar of demoteTarget in hooks/go/internal/flow/operands.go, so a target that passes here also passes the writer.
const DEMOTE_TARGET = /^wave-[A-Za-z0-9-]+$/;
// A documented form such as `demote wave-<N>` names no wave yet, so the grammar does not apply to it.
const DEMOTE_PLACEHOLDER = /^wave-<[^>]*>$/;

/** The text of each cell of a Markdown table row, trimmed; the outer pipes are dropped. */
function cells(row: string): string[] {
  const inner = row.trim().replace(/^\|/, "").replace(/\|$/, "");
  return inner.split("|").map((cell) => cell.trim());
}

/** The contents of each backticked span in `text`, in order. */
function codeSpans(text: string): string[] {
  return [...text.matchAll(CODE_SPAN)].map((match) => match[1] ?? "");
}

/**
 * The rows of the `## Operands` table in `text`: each head with the first backticked path of its **Emitted in** cell.
 * A file without the section, or a section without a table, yields no row, so every emitted head then fails.
 */
export function parseOperands(text: string): Operand[] {
  const lines = text.split(/\r?\n/);
  const start = lines.findIndex((line) => line.trim() === "## Operands");
  if (start < 0) return [];
  const rows: string[] = [];
  for (const line of lines.slice(start + 1)) {
    if (line.startsWith("## ")) break;
    if (line.trimStart().startsWith("|")) rows.push(line);
    else if (rows.length > 0) break;
  }
  const header = rows[0];
  if (header === undefined) return [];
  const names = cells(header);
  const headColumn = names.indexOf("Head");
  const emittedColumn = names.indexOf("Emitted in");
  if (headColumn < 0 || emittedColumn < 0) return [];
  const operands: Operand[] = [];
  // rows[1] is the delimiter row.
  for (const row of rows.slice(2)) {
    const rowCells = cells(row);
    const emittedIn = codeSpans(rowCells[emittedColumn] ?? "")[0] ?? "";
    for (const head of codeSpans(rowCells[headColumn] ?? ""))
      operands.push({ head, emittedIn });
  }
  return operands;
}

/** Every explicit `waive` or `demote` target in `text`: backticked mentions and `agx flow log` commands. */
function emissions(text: string): Emission[] {
  const out: Emission[] = [];
  // Spans are matched one line at a time, so a backticked target wrapped across a line break is not read.
  text.split(/\r?\n/).forEach((content, index) => {
    const line = index + 1;
    for (const span of codeSpans(content)) {
      const match = SPAN_FORM.exec(span);
      if (match === null) continue;
      out.push({
        verb: match[1] as Emission["verb"],
        target: match[2] ?? "",
        line,
      });
    }
    for (const match of content.matchAll(COMMAND_FORM))
      out.push({
        verb: match[1] as Emission["verb"],
        target: match[2] ?? "",
        line,
      });
  });
  return out;
}

/** The target without one pair of surrounding double quotes. */
function unquote(target: string): string {
  return target.length >= 2 && target.startsWith('"') && target.endsWith('"')
    ? target.slice(1, -1)
    : target;
}

/** The head of a target: its first word, without a release item's `:<check>`. */
function headOf(target: string): string {
  const word = target.split(" ")[0] ?? "";
  return PLACEHOLDER.test(word) ? word : (word.split(":")[0] ?? "");
}

/**
 * The places of `text` where a head counts as emitted: each backticked span, which covers a mention and a backticked
 * item list, and each line that holds an `agx flow log` command. Ordinary prose is left out, so a head that is also a
 * common word, such as `review` or `refactor`, is not found there by accident.
 */
function emittingPlaces(text: string): string[] {
  const places: string[] = [];
  for (const line of text.split(/\r?\n/)) {
    places.push(...codeSpans(line));
    if (line.includes(FLOW_LOG_CALL)) places.push(line);
  }
  return places;
}

/** Whether `head` appears in one of `places` bounded by characters that cannot continue a hyphenated word. */
function hasWholeWord(places: readonly string[], head: string): boolean {
  const escaped = head.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  const word = new RegExp(`(?<![A-Za-z0-9_-])${escaped}(?![A-Za-z0-9_-])`);
  return places.some((place) => word.test(place));
}

/** The findings of one skill file against the listed heads: an unlisted head, or a `demote` target that is not a wave. */
function emittedFailures(
  rel: string,
  text: string,
  listed: ReadonlySet<string>,
): Located[] {
  const failures: Located[] = [];
  const add = (line: number, message: string): void => {
    failures.push({
      failure: {
        rule: WAIVE_OPERANDS_RULE,
        path: rel,
        message: `line ${line}: ${message}`,
      },
      line,
    });
  };
  for (const { verb, target: raw, line } of emissions(text)) {
    const target = unquote(raw);
    const head = headOf(target);
    if (PLACEHOLDER.test(head)) continue;
    if (verb === "demote") {
      if (!DEMOTE_TARGET.test(target) && !DEMOTE_PLACEHOLDER.test(target))
        add(line, `demote target ${JSON.stringify(target)} is not wave-<id>`);
      continue;
    }
    if (!listed.has(head))
      add(
        line,
        `waive head ${JSON.stringify(head)} is not in the Operands table of ${OPERANDS_FILE}`,
      );
  }
  return failures;
}

/** Orders two strings as the default sort orders them. */
function compareText(a: string, b: string): number {
  if (a < b) return -1;
  return a > b ? 1 : 0;
}

/**
 * Ties the `waive` and `demote` targets skills write to the Operands table in both directions. Every explicit target
 * a skill file writes names a listed head, or a `wave-<id>` for `demote`; and every listed head appears as a whole word
 * in a backticked span or an `agx flow log` line of the file its **Emitted in** cell names. The second direction also
 * accepts a backticked item list; the first reads only explicit `waive` forms so an ordinary word is never taken for a
 * head. The scan covers the record contract too, so its examples stay listed heads. Findings sort by path, then line.
 */
export function waiveOperandsRules(
  repoRoot: string,
  tree = new SkillsTreeModel(repoRoot),
): Failure[] {
  let contract = "";
  try {
    contract = tree.read(OPERANDS_FILE);
  } catch (error) {
    if (!isFileErrorOfKind(error, "missing")) throw error;
  }
  const operands = parseOperands(contract);
  const listed = new Set(operands.map((operand) => operand.head));
  const failures: Located[] = [];
  const fileFailure = (path: string, message: string): void => {
    failures.push({
      failure: { rule: WAIVE_OPERANDS_RULE, path, message },
      line: 0,
    });
  };
  tree.walkText("waive operands", (rel, text) => {
    failures.push(...emittedFailures(rel, text, listed));
  });
  const emitters = new Map<string, string[] | undefined>();
  for (const { head, emittedIn } of operands) {
    if (emittedIn === "") {
      fileFailure(
        OPERANDS_FILE,
        `head ${JSON.stringify(head)} names no Emitted in path`,
      );
      continue;
    }
    if (!emitters.has(emittedIn)) {
      try {
        emitters.set(emittedIn, emittingPlaces(tree.read(emittedIn)));
      } catch (error) {
        if (!isFileErrorOfKind(error, "missing")) throw error;
        emitters.set(emittedIn, undefined);
      }
    }
    const places = emitters.get(emittedIn);
    if (places === undefined)
      fileFailure(
        OPERANDS_FILE,
        `head ${JSON.stringify(head)} is emitted in ${emittedIn}, which does not exist`,
      );
    else if (!hasWholeWord(places, head))
      fileFailure(
        emittedIn,
        `head ${JSON.stringify(head)} is listed as emitted here but appears in no backticked span or agx flow log line`,
      );
  }
  return failures
    .sort(
      (a, b) =>
        compareText(a.failure.path, b.failure.path) ||
        a.line - b.line ||
        compareText(a.failure.message, b.failure.message),
    )
    .map((located) => located.failure);
}

/** The `flow-waive-operands` family: `waive` and `demote` targets match the record contract's Operands table. */
export const FAMILY: Family = {
  name: "flow-waive-operands",
  rules: [WAIVE_OPERANDS_RULE],
  run: (root, context) => waiveOperandsRules(root, context?.tree),
};
