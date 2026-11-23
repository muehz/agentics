import path from "node:path";
import type { MutationScope } from "./mutation-artifacts.ts";
import {
  GO_MODULE,
  isGremlinsSource,
  isProductionSource,
} from "./mutation-source.ts";

export function selectScope(paths: readonly string[]): MutationScope {
  const stryker = [
    ...new Set(paths.filter((file) => isProductionSource(file, "stryker"))),
  ].sort();
  const packages = new Map<string, Set<string>>();
  for (const file of paths.filter(isGremlinsSource)) {
    const rel = file.slice(GO_MODULE.length);
    const dir = path.posix.dirname(rel);
    const pkg = dir === "." ? "." : `./${dir}`;
    if (!packages.has(pkg)) packages.set(pkg, new Set());
    packages.get(pkg)!.add(path.posix.basename(rel));
  }
  const gremlins = [...packages.keys()]
    .sort()
    .map((pkg) => ({ pkg, changed: [...packages.get(pkg)!].sort() }));
  return { stryker, gremlins };
}

// The line ranges a zero-context diff adds or rewrites in each file, numbered as in the new version. A hunk that only
// deletes lines leaves nothing to mutate and adds no range.
export function changedLines(diff: string) {
  const lines: Record<string, [number, number][]> = {};
  let file: string | null = null;
  for (const line of diff.split("\n")) {
    if (line.startsWith("+++ ")) {
      file = line.startsWith("+++ b/") ? line.slice(6) : null;
      if (file) lines[file] ??= [];
      continue;
    }
    const hunk = /^@@ -\S+ \+(\d+)(?:,(\d+))? @@/.exec(line);
    if (!hunk || !file) continue;
    const first = Number(hunk[1]);
    const count = hunk[2] === undefined ? 1 : Number(hunk[2]);
    if (count > 0) lines[file]!.push([first, first + count - 1]);
  }
  return lines;
}
