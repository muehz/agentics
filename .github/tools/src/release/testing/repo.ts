import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

// The checkout root, found from this file's own location (.github/tools/src/release/testing/), so a test does not depend on
// the directory it runs from.
export const repoRoot: string = fileURLToPath(
  new URL("../../../../../", import.meta.url),
);

// Reads a tracked text file by repository-relative path and normalizes CRLF to LF, so a checkout with
// core.autocrlf cannot change what a text mutation matches.
export function readRepoText(relative: string): string {
  return readFileSync(`${repoRoot}${relative}`, "utf8").replace(/\r\n/g, "\n");
}
