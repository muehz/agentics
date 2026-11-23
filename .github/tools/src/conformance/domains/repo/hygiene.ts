import { closeSync, openSync, readFileSync, readSync } from "node:fs";
import type { Failure } from "../../check.ts";
import { repoFileErrorOf } from "../../repo-file-error.ts";
import { isOtherSymbol } from "../../text.ts";
import { walkCheckout, type Tracked } from "./walk.ts";

/** How many leading bytes the binary test reads, as git does. */
const BINARY_WINDOW = 8000;

/**
 * A text file's whole contents, or undefined once a NUL byte in the first 8,000 bytes marks it binary (the rest of a
 * binary file is not read). A text file is read to its end: a cap would let a payload sit past it unscanned. A leading
 * byte order mark is kept, and an invalid byte decodes to U+FFFD, which is itself a symbol.
 */
export function readText(file: string): string | undefined {
  const fd = openSync(file, "r");
  try {
    const head = Buffer.alloc(BINARY_WINDOW);
    let read = 0;
    while (read < BINARY_WINDOW) {
      const n = readSync(fd, head, read, BINARY_WINDOW - read, null);
      if (n === 0) break;
      read += n;
    }
    const start = head.subarray(0, read);
    if (start.includes(0)) return undefined;
    return Buffer.concat([start, readFileSync(fd)]).toString("utf8");
  } finally {
    closeSync(fd);
  }
}

const START_MARKER = "<<<<<<<";
const END_MARKER = ">>>>>>>";

/**
 * Reports every line that opens or closes a conflict block. `=======` is not matched: it also underlines a markdown
 * heading, and every real block opens with one of the other two.
 */
export function conflictMarkers(rel: string, text: string): Failure[] {
  if (!text.includes(START_MARKER) && !text.includes(END_MARKER)) return [];
  const failures: Failure[] = [];
  // A CRLF file needs no stripping: the markers are matched at the start of the line, so a trailing "\r" never matters.
  text.split("\n").forEach((line, index) => {
    const marker = line.startsWith(START_MARKER)
      ? START_MARKER
      : line.startsWith(END_MARKER)
        ? END_MARKER
        : "";
    if (marker === "") return;
    failures.push({
      rule: "conflict-marker",
      path: rel,
      message: `line ${index + 1} begins with ${JSON.stringify(marker)} — an unresolved git conflict block. Resolve the merge and delete every marker line; one side of the merge is missing until you do.`,
    });
  });
  return failures;
}

const INVISIBLE_RANGES: ReadonlyArray<readonly [number, number, string]> = [
  [0x061c, 0x061c, "bidi mark"],
  [0x115f, 0x1160, "Hangul filler"],
  [0x180e, 0x180e, "Mongolian vowel separator"],
  [0x200b, 0x200d, "zero-width character"],
  [0x200e, 0x200f, "bidi mark"],
  [0x202a, 0x202e, "bidi control"],
  [0x2060, 0x2064, "invisible format character"],
  [0x2066, 0x2069, "bidi isolate"],
  [0x3164, 0x3164, "Hangul filler"],
  [0xfe00, 0xfe0f, "variation selector"],
  [0xfeff, 0xfeff, "zero-width no-break space"],
  [0xe0000, 0xe007f, "Unicode tag character"],
  [0xe0100, 0xe01ef, "variation selector"],
];

/** Built from the table so a file with none of these code points skips the per-character loop. */
const CANDIDATE = new RegExp(
  INVISIBLE_RANGES.map(
    ([lo, hi]) => `[${String.fromCodePoint(lo)}-${String.fromCodePoint(hi)}]`,
  ).join("|"),
  "u",
);

const EMOJI_PRESENTATION = 0xfe0f;

function invisibleClass(codePoint: number): string | undefined {
  for (const [lo, hi, klass] of INVISIBLE_RANGES)
    if (codePoint >= lo && codePoint <= hi) return klass;
  return undefined;
}

/**
 * Reports each line that holds a code point a reviewer cannot see but a model reads, naming the first and counting the
 * rest. U+FE0F passes only directly after a symbol (category So), so a run of selectors, the shape that smuggles bytes,
 * still fails after its first member. The first code point in a text or on a line has no predecessor.
 */
export function invisibleUnicode(rel: string, text: string): Failure[] {
  if (!CANDIDATE.test(text)) return [];
  const failures: Failure[] = [];
  let line = 1;
  let previous = -1;
  let first = 0;
  let firstClass = "";
  let count = 0;
  const flush = (): void => {
    if (count === 0) return;
    const hex = first.toString(16).toUpperCase().padStart(4, "0");
    const more = count > 1 ? ` and ${count - 1} more` : "";
    failures.push({
      rule: "invisible-unicode",
      path: rel,
      message: `line ${line} holds U+${hex} (${firstClass})${more}, invisible in review but read by a model; delete it, or write it as a \\u or \\U escape in code.`,
    });
    count = 0;
  };
  for (const char of text) {
    const point = char.codePointAt(0) ?? 0;
    if (point === 0x0a) {
      flush();
      line += 1;
      previous = point;
      continue;
    }
    let klass = invisibleClass(point);
    if (
      klass !== undefined &&
      point === EMOJI_PRESENTATION &&
      isOtherSymbol(previous)
    )
      klass = undefined;
    if (klass !== undefined) {
      if (count === 0) {
        first = point;
        firstClass = klass;
      }
      count += 1;
    }
    previous = point;
  }
  flush();
  return failures;
}

/**
 * Scans every text file the hygiene walk reaches. A file that cannot be read stops the audit with a `RepoFileError`. `read` lets
 * a test make a read fail, because an unreadable file cannot be built portably.
 */
export function hygieneWalk(
  root: string,
  tracked: Tracked,
  read: (file: string) => string | undefined = readText,
): { failures: Failure[]; scanned: number } {
  const failures: Failure[] = [];
  let scanned = 0;
  walkCheckout(root, tracked, "hygiene", (file) => {
    let text: string | undefined;
    try {
      text = read(file.absolute);
    } catch (error) {
      throw repoFileErrorOf(error, file.rel);
    }
    if (text === undefined) return;
    scanned += 1;
    failures.push(
      ...conflictMarkers(file.rel, text),
      ...invisibleUnicode(file.rel, text),
    );
  });
  return { failures, scanned };
}
