import { posix } from "node:path";
import { inCoverageScope } from "./scope.ts";

export interface FileCoverage {
  lines: Map<number, number>;
  branches: Map<string, number>;
}
export type Coverage = Map<string, FileCoverage>;
export interface LcovOptions {
  root: string;
  workspace: string;
  label: string;
}

const slashes = (value: string) => value.replaceAll("\\", "/");
function sourcePath(source: string, options: LcovOptions): string {
  const root = posix.normalize(slashes(options.root));
  const workspace = slashes(options.workspace);
  const base = /^(?:[A-Za-z]:)?\//.test(workspace)
    ? workspace
    : posix.join(root, workspace);
  const input = slashes(source);
  const full = posix.normalize(
    /^(?:[A-Za-z]:)?\//.test(input) ? input : posix.join(base, input),
  );
  // A drive prefix must stay in the comparison so unrelated drives cannot enter the report.
  if (full === root || !full.startsWith(`${root.replace(/\/$/, "")}/`))
    return "";
  return full.slice(root.replace(/\/$/, "").length + 1);
}

export function parseLcov(text: string, options: LcovOptions): Coverage {
  const report: Coverage = new Map();
  let current: FileCoverage | undefined;
  let hasSource = false;
  const integer = (value: string | undefined, line: number): number => {
    if (
      value === undefined ||
      !/^\d+$/.test(value) ||
      !Number.isSafeInteger(Number(value))
    ) {
      throw new Error(`${options.label}:${line}: invalid lcov integer`);
    }
    return Number(value);
  };
  for (const [index, record] of text.split(/\r?\n/).entries()) {
    if (record.startsWith("SF:")) {
      hasSource = true;
      const filename = sourcePath(record.slice(3), options);
      current = undefined;
      if (inCoverageScope(filename)) {
        current = report.get(filename) ?? {
          lines: new Map(),
          branches: new Map(),
        };
        report.set(filename, current);
      }
    } else if (record === "end_of_record") {
      current = undefined;
    } else if (current && record.startsWith("DA:")) {
      const [line, hits] = record.slice(3).split(",");
      const number = integer(line, index + 1);
      current.lines.set(
        number,
        (current.lines.get(number) ?? 0) + integer(hits, index + 1),
      );
    } else if (current && record.startsWith("BRDA:")) {
      const fields = record.slice(5).split(",");
      if (fields.length !== 4)
        throw new Error(`${options.label}:${index + 1}: invalid BRDA record`);
      integer(fields[0], index + 1);
      integer(fields[1], index + 1);
      integer(fields[2], index + 1);
      const key = fields.slice(0, 3).join(",");
      const hits = fields[3] === "-" ? 0 : integer(fields[3], index + 1);
      current.branches.set(key, (current.branches.get(key) ?? 0) + hits);
    } else if (current && /^(LF|LH|BRF|BRH):/.test(record)) {
      integer(record.split(":")[1], index + 1);
    }
  }
  if (!hasSource) throw new Error(`${options.label}: no SF: record`);
  return report;
}

export function mergeLcov(inputs: Coverage[]): Coverage {
  const merged: Coverage = new Map();
  for (const input of inputs)
    for (const [filename, source] of input) {
      const target = merged.get(filename) ?? {
        lines: new Map(),
        branches: new Map(),
      };
      for (const [line, hits] of source.lines)
        target.lines.set(line, (target.lines.get(line) ?? 0) + hits);
      for (const [branch, hits] of source.branches)
        target.branches.set(branch, (target.branches.get(branch) ?? 0) + hits);
      merged.set(filename, target);
    }
  return merged;
}
