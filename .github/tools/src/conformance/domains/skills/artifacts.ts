import type { Failure } from "../../check.ts";
import { assetFiles, auditFileError, readRepoFile } from "./files.ts";
import {
  frontmatter,
  isMap,
  isScalar,
  isSeq,
  parseFrontmatter,
  render,
  skillName,
  type FrontmatterMap,
  type FrontmatterNode,
} from "./frontmatter.ts";
import type { Family } from "./index.ts";

/**
 * The skills the chain boundary design assigns an artifact, keyed by the `name:` value the file declares. A skill
 * outside this set declares no `metadata.artifact`; the field is absent, not empty.
 */
const ARTIFACT_OWNERS: ReadonlyMap<string, readonly string[]> = new Map([
  ["intent", ["intent.md"]],
  ["spec", ["spec.md"]],
  ["decompose", ["plan.md"]],
]);

/** One declared value as text: a null reads as the empty string, anything else as `render` prints it. */
function scalarText(node: FrontmatterNode): string {
  return isScalar(node) && node.value === null ? "" : render(node);
}

/**
 * The rendered values of `metadata.artifact`, or undefined when the key is not declared. The key counts only inside a
 * `metadata` mapping; a present key with no value is a declaration of one empty item, not an absence. A list is sorted
 * in default sort order after rendering.
 */
function declaredArtifacts(doc: FrontmatterMap): string[] | undefined {
  const meta = doc.entries.get("metadata");
  if (!isMap(meta)) return undefined;
  const value = meta.entries.get("artifact");
  if (value === undefined) return undefined;
  if (!isSeq(value)) return [scalarText(value)];
  return value.items.map(scalarText).sort();
}

/** Orders failures as the default sort orders their paths; the sort is stable, so equal paths keep their order. */
function byPath(a: Failure, b: Failure): number {
  if (a.path < b.path) return -1;
  return a.path > b.path ? 1 : 0;
}

function equal(a: readonly string[], b: readonly string[]): boolean {
  return a.length === b.length && a.every((item, at) => item === b[at]);
}

function artifactRules(root: string): Failure[] {
  const failures: Failure[] = [];
  const seen = new Map<string, string[]>();
  for (const rel of assetFiles(root)) {
    let text: string;
    try {
      text = readRepoFile(root, rel);
    } catch (error) {
      throw auditFileError("reading", rel, error);
    }
    // A skill is identified by the name its frontmatter declares, not by its directory.
    const name = skillName(text);
    if (name === undefined) continue;
    const want = ARTIFACT_OWNERS.get(name);
    let doc: FrontmatterMap;
    try {
      doc = parseFrontmatter(frontmatter(text));
    } catch {
      continue;
    }
    const got = declaredArtifacts(doc);

    if (want !== undefined && got === undefined)
      failures.push({
        rule: "skill-artifact-declared",
        path: rel,
        message: `chain skill ${JSON.stringify(name)} owns ${JSON.stringify(want)} but declares no metadata.artifact`,
      });
    if (got === undefined) continue;
    if (want === undefined)
      failures.push({
        rule: "skill-artifact-expected",
        path: rel,
        message: `skill ${JSON.stringify(name)} declares metadata.artifact ${JSON.stringify(got)} but owns no chain artifact; the field is absent, not empty, on a non-owner`,
      });
    else if (!equal(got, want))
      failures.push({
        rule: "skill-artifact-expected",
        path: rel,
        message: `skill ${JSON.stringify(name)} declares metadata.artifact ${JSON.stringify(got)}; the chain boundary design assigns it ${JSON.stringify(want)}`,
      });
    for (const artifact of got) {
      if (artifact === "") continue;
      const paths = seen.get(artifact);
      if (paths === undefined) seen.set(artifact, [rel]);
      else paths.push(rel);
    }
  }
  for (const artifact of [...seen.keys()].sort()) {
    const paths = seen.get(artifact) ?? [];
    if (paths.length < 2) continue;
    paths.sort();
    failures.push({
      rule: "skill-artifact-unique",
      path: paths[0] ?? "",
      message: `artifact ${JSON.stringify(artifact)} is declared by ${paths.length} skills: ${paths.join(", ")}`,
    });
  }
  return failures.sort(byPath);
}

/**
 * The `artifact` family: each chain artifact has exactly one owning skill. `intent` owns `intent.md`, `spec` owns
 * `spec.md` and `decompose` owns `plan.md`, each declared as `metadata.artifact`, and no other skill declares one. A
 * file whose frontmatter does not parse is skipped, because the `frontmatter` family reports it. Findings sort by path,
 * keeping the order in which they were found for equal paths.
 */
export const FAMILY: Family = {
  name: "artifact",
  rules: [
    "skill-artifact-declared",
    "skill-artifact-expected",
    "skill-artifact-unique",
  ],
  run: artifactRules,
};
