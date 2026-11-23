import { posix } from "node:path";
import type { Coverage } from "./lcov.ts";

export interface CoberturaCount {
  covered: number;
  valid: number;
}

interface CoberturaMetrics {
  lineCount: CoberturaCount;
  branchCount?: CoberturaCount;
}

export interface CoberturaLine {
  number: number;
  hits: number;
  branchCount?: CoberturaCount;
}

export interface CoberturaMethod extends CoberturaMetrics {
  name: string;
  lines: readonly CoberturaLine[];
}

export interface CoberturaClass extends CoberturaMetrics {
  name: string;
  filename: string;
  methods: readonly CoberturaMethod[];
  lines: readonly CoberturaLine[];
}

export interface CoberturaPackage extends CoberturaMetrics {
  name: string;
  classes: CoberturaClass[];
}

export interface CoberturaDocument extends CoberturaMetrics {
  timestamp: number;
  packages: readonly CoberturaPackage[];
}

type CoberturaLayout = "lcov" | "go";

function escape(value: string, escapeApostrophe: boolean): string {
  const xml = value
    .replaceAll("&", "&amp;")
    .replaceAll('"', "&quot;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;");
  return escapeApostrophe ? xml.replaceAll("'", "&apos;") : xml;
}

const zero: CoberturaCount = { covered: 0, valid: 0 };
const rate = ({ covered, valid }: CoberturaCount) =>
  (valid === 0 ? 0 : covered / valid).toFixed(8);

function rateAttributes(metrics: CoberturaMetrics, decimalBranches: boolean) {
  const branches = metrics.branchCount ?? zero;
  return `line-rate="${rate(metrics.lineCount)}" branch-rate="${decimalBranches ? rate(branches) : "0"}"`;
}

function lineXml({ number, hits, branchCount }: CoberturaLine, lcov: boolean) {
  let branches = "";
  if (lcov) {
    branches =
      branchCount === undefined
        ? ' branch="false"'
        : ` branch="true" condition-coverage="${Math.floor((100 * branchCount.covered) / branchCount.valid)}% (${branchCount.covered}/${branchCount.valid})"`;
  }
  return `<line number="${number}" hits="${hits}"${branches}/>`;
}

function classXml(entry: CoberturaClass, lcov: boolean, prefix: string) {
  const indent = lcov ? "        " : "      ";
  const child = indent + "  ";
  const renderedName = escape(entry.name, lcov);
  const renderedPath = escape(
    prefix ? `${prefix}/${entry.filename}` : entry.filename,
    lcov,
  );
  const methods = lcov
    ? [`${child}<methods/>`]
    : [
        `${child}<methods>`,
        ...entry.methods.map((method) => {
          const lines = method.lines
            .map((line) => lineXml(line, false))
            .join("");
          return `${child}  <method name="${escape(method.name, false)}" signature="" ${rateAttributes(method, false)} complexity="0"><lines>${lines}</lines></method>`;
        }),
        `${child}</methods>`,
      ];
  return [
    `${indent}<class name="${renderedName}" filename="${renderedPath}" ${rateAttributes(entry, lcov)} complexity="0">`,
    ...methods,
    `${child}<lines>`,
    ...entry.lines.map((line) => `${child}  ${lineXml(line, lcov)}`),
    `${child}</lines>`,
    `${indent}</class>`,
  ];
}

function packageXml(pkg: CoberturaPackage, lcov: boolean, prefix: string) {
  const tag = `    <package name="${escape(pkg.name, lcov)}" ${rateAttributes(pkg, lcov)} complexity="0">`;
  const classes = pkg.classes.flatMap((entry) => classXml(entry, lcov, prefix));
  if (lcov)
    return [
      tag,
      "      <classes>",
      ...classes,
      "      </classes>",
      "    </package>",
    ];
  return [tag + "<classes>", ...classes, "    </classes></package>"];
}

export function serializeCobertura(
  document: CoberturaDocument,
  layout: CoberturaLayout,
  prefix = "",
): string {
  const lcov = layout === "lcov";
  const { lineCount, branchCount = zero } = document;
  const packages = document.packages.flatMap((pkg) =>
    packageXml(pkg, lcov, prefix),
  );
  // The old prefix step emitted an empty package block as a blank line.
  if (!lcov && packages.length === 0) packages.push("");
  return [
    '<?xml version="1.0" encoding="UTF-8"?>',
    '<!DOCTYPE coverage SYSTEM "http://cobertura.sourceforge.net/xml/coverage-04.dtd">',
    `<coverage ${rateAttributes(document, true)} version="" timestamp="${document.timestamp}" lines-covered="${lineCount.covered}" lines-valid="${lineCount.valid}" branches-covered="${branchCount.covered}" branches-valid="${branchCount.valid}" complexity="0">`,
    "  <sources>",
    "    <source>.</source>",
    "  </sources>",
    "  <packages>",
    ...packages,
    "  </packages>",
    "</coverage>",
    "",
  ].join("\n");
}

export function toCobertura(coverage: Coverage): string {
  const lineCount: CoberturaCount = { covered: 0, valid: 0 };
  const branchCount: CoberturaCount = { covered: 0, valid: 0 };
  const classes: CoberturaClass[] = [];
  for (const [filename, file] of [...coverage].sort(([a], [b]) =>
    a.localeCompare(b),
  )) {
    const covered = [...file.lines.values()].filter((hits) => hits > 0).length;
    const branchCovered = [...file.branches.values()].filter(
      (hits) => hits > 0,
    ).length;
    lineCount.valid += file.lines.size;
    lineCount.covered += covered;
    branchCount.valid += file.branches.size;
    branchCount.covered += branchCovered;
    const lines = [...file.lines]
      .sort(([a], [b]) => a - b)
      .map(([number, hits]) => {
        const branches = [...file.branches].filter(
          ([key]) => key.split(",")[0] === String(number),
        );
        return {
          number,
          hits,
          branchCount: branches.length
            ? {
                covered: branches.filter(([, count]) => count > 0).length,
                valid: branches.length,
              }
            : undefined,
        };
      });
    classes.push({
      name: posix.basename(filename),
      filename,
      lineCount: { covered, valid: file.lines.size },
      branchCount: { covered: branchCovered, valid: file.branches.size },
      methods: [],
      lines,
    });
  }
  return serializeCobertura(
    {
      timestamp: 0,
      lineCount,
      branchCount,
      packages: [{ name: "typescript", classes, lineCount, branchCount }],
    },
    "lcov",
  );
}
