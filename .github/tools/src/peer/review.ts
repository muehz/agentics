import { readRegularFile } from "./regular-file.ts";

export type ReviewCheck =
  { ok: true } | { ok: false; reason: "empty" | "terminator" };

const MARKER = "REVIEW-COMPLETE:";
const TERMINATOR = /^REVIEW-COMPLETE: (no-findings|findings=[0-9]+)$/;

// Exactly one marker in the whole file, and it is the entire final line. A trailing CR on a line is ignored, so
// CRLF output passes. The final newline is not a record of its own.
function terminatorHolds(text: string): boolean {
  const lines = text.split("\n");
  if (text.endsWith("\n")) lines.pop();
  let count = 0;
  let last = "";
  for (const raw of lines) {
    const line = raw.endsWith("\r") ? raw.slice(0, -1) : raw;
    let rest = line;
    for (let at = rest.indexOf(MARKER); at !== -1; at = rest.indexOf(MARKER)) {
      count++;
      rest = rest.slice(at + MARKER.length);
    }
    last = line;
  }
  return count === 1 && TERMINATOR.test(last);
}

export function checkReviewOutput(file: string): ReviewCheck {
  const read = readRegularFile(file);
  if (read.kind === "missing") return { ok: false, reason: "empty" };
  // A path that exists but is not a readable file (a directory, for one) holds no terminator, so it fails the
  // terminator check rather than crashing the run.
  if (read.kind === "unreadable") return { ok: false, reason: "terminator" };
  const { text } = read;
  if (text === "") return { ok: false, reason: "empty" };
  return terminatorHolds(text)
    ? { ok: true }
    : { ok: false, reason: "terminator" };
}
