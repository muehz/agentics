import { globSync } from "node:fs";
import { mergeLcov, type Coverage } from "./lcov.ts";
import { inCoverageScope, SOURCE_ROOTS } from "./scope.ts";

export interface WorkspaceCoverage {
  workspace: string;
  coverage: Coverage;
}

/** Every TypeScript source file under the coverage roots that the report counts, as repo-relative slash paths. */
export function listSourceFiles(root: string): string[] {
  return SOURCE_ROOTS.flatMap((dir) =>
    globSync(`${dir}/**/*.ts`, { cwd: root }),
  )
    .map((file) => file.replaceAll("\\", "/"))
    .filter(inCoverageScope)
    .sort();
}

function count(coverage: Coverage) {
  let lines = 0,
    linesHit = 0,
    branches = 0,
    branchesHit = 0;
  for (const file of coverage.values()) {
    for (const hits of file.lines.values()) {
      lines++;
      if (hits > 0) linesHit++;
    }
    for (const hits of file.branches.values()) {
      branches++;
      if (hits > 0) branchesHit++;
    }
  }
  return { lines, linesHit, branches, branchesHit };
}

function percent(hit: number, total: number): string {
  const share = total === 0 ? "n/a" : `${((hit / total) * 100).toFixed(1)}%`;
  return `${share} (${hit}/${total})`;
}

/**
 * A markdown job summary: line and branch coverage per workspace and in total, then the in-scope source files that no
 * workspace report loaded. Unloaded files are listed for reading, never as a failure.
 */
export function summarize(
  reports: readonly WorkspaceCoverage[],
  sourceFiles: readonly string[],
): string {
  const workspaces = new Map<string, Coverage[]>();
  for (const { workspace, coverage } of reports) {
    const shards = workspaces.get(workspace) ?? [];
    shards.push(coverage);
    workspaces.set(workspace, shards);
  }
  const totals = mergeLcov(reports.map(({ coverage }) => coverage));
  const rows = [
    ...[...workspaces].map(([name, shards]) => ({
      name,
      ...count(mergeLcov(shards)),
    })),
    { name: "total", ...count(totals) },
  ];
  const unloaded = sourceFiles.filter((file) => !totals.has(file));
  const out = [
    "## TypeScript coverage",
    "",
    "| Workspace | Lines | Branches |",
    "| --- | --- | --- |",
    ...rows.map(
      (row) =>
        `| ${row.name} | ${percent(row.linesHit, row.lines)} | ${percent(row.branchesHit, row.branches)} |`,
    ),
    "",
  ];
  if (unloaded.length === 0)
    out.push(
      `Every source file under ${SOURCE_ROOTS.join(" and ")} was loaded by a test.`,
    );
  else
    out.push(
      `### Source files no test loaded (${unloaded.length})`,
      "",
      ...unloaded.map((file) => `- \`${file}\``),
    );
  return `${out.join("\n")}\n`;
}
