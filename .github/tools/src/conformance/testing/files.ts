import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import path from "node:path";

export function writeFile(root: string, rel: string, content: string): void {
  const target = path.join(root, ...rel.split("/"));
  mkdirSync(path.dirname(target), { recursive: true });
  writeFileSync(target, content);
}

/** Rewrites one file by replacement, throwing when the target text is absent: a silent no-op would fake a bite check. */
export function mutateFile(
  root: string,
  rel: string,
  from: string,
  to: string,
): void {
  const target = path.join(root, ...rel.split("/"));
  const data = readFileSync(target, "utf8");
  if (!data.includes(from))
    throw new Error(
      `mutation target ${JSON.stringify(from)} not found in ${rel}`,
    );
  writeFileSync(target, data.split(from).join(to));
}
