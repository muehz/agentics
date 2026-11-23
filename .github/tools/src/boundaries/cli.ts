import { existsSync, readFileSync, readdirSync } from "node:fs";
import path from "node:path";
import { AREA_ROOTS, TS_AREAS, type TsArea, type Violation } from "./areas.ts";
import { codeownersViolations } from "./codeowners.ts";
import {
  collectCompilerEvidence,
  type TscRunner,
} from "./compiler-evidence.ts";
import { areaImportCheck, skillImportViolations } from "./imports.ts";
import { packageViolations } from "./packages.ts";

export {
  defaultTsc,
  type TscResult,
  type TscRunner,
} from "./compiler-evidence.ts";

export interface Options {
  root: string;
  tsc?: TscRunner;
  /** Overrides the node_modules/typescript/bin/tsc probe. */
  hasTsc?: boolean;
}

export interface Report {
  lines: string[];
  code: 0 | 1 | 2;
}

type Json = Record<string, unknown>;

function readJson(file: string): Json | null {
  try {
    return JSON.parse(readFileSync(file, "utf8")) as Json;
  } catch {
    return null;
  }
}

const SCRIPT = /\.(?:js|cjs|mjs|ts|cts|mts)$/;

function scriptFiles(dir: string, root: string): string[] {
  const out: string[] = [];
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    if (entry.name === "node_modules") continue;
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) out.push(...scriptFiles(full, root));
    else if (SCRIPT.test(entry.name))
      out.push(path.relative(root, full).replaceAll("\\", "/"));
  }
  return out;
}

export function run(options: Options): Report {
  const { root, tsc } = options;
  const hasTsc =
    options.hasTsc ??
    existsSync(path.join(root, "node_modules", "typescript", "bin", "tsc"));
  const violations: Violation[] = [];
  const lines: string[] = [];
  let errors = 0;

  const packages = Object.fromEntries(
    TS_AREAS.map((area) => [
      area,
      readJson(path.join(root, AREA_ROOTS[area], "package.json")),
    ]),
  ) as Record<TsArea, Json | null>;
  violations.push(
    ...packageViolations(
      readJson(path.join(root, "package.json")) ?? {},
      packages,
    ),
  );
  const owners = existsSync(path.join(root, "CODEOWNERS"))
    ? readFileSync(path.join(root, "CODEOWNERS"), "utf8")
    : "";
  violations.push(...codeownersViolations(owners));

  if (!hasTsc) {
    lines.push("SKIP area-ts-import: TypeScript is not installed");
    lines.push("SKIP area-skill-import: TypeScript is not installed");
  } else {
    for (const area of TS_AREAS) {
      const directory = AREA_ROOTS[area];
      const evidence = collectCompilerEvidence({
        root,
        args: [
          "--noEmit",
          "--explainFiles",
          "-p",
          `${directory}/tsconfig.json`,
        ],
        directory,
        tsc,
      });
      if (!evidence.ok) {
        errors += 1;
        lines.push(
          `ERROR area-ts-import: ${area}: tsc did not list any file of ${area}: ${evidence.detail}`,
        );
        continue;
      }
      violations.push(...areaImportCheck(area, evidence.files));
    }
    const skillsDir = path.join(root, "skills");
    const skills = existsSync(skillsDir)
      ? readdirSync(skillsDir, { withFileTypes: true }).filter((e) =>
          e.isDirectory(),
        )
      : [];
    for (const skill of skills) {
      const files = scriptFiles(path.join(skillsDir, skill.name), root);
      if (files.length === 0) continue;
      const directory = `skills/${skill.name}`;
      const evidence = collectCompilerEvidence({
        root,
        args: [
          "--noEmit",
          "--allowJs",
          "--checkJs",
          "--resolveJsonModule",
          "--explainFiles",
          "--module",
          "nodenext",
          "--moduleResolution",
          "nodenext",
          "--skipLibCheck",
          "--allowImportingTsExtensions",
          "--types",
          "node",
          ...files,
        ],
        directory,
        includeUnresolved: true,
        tsc,
      });
      if (!evidence.ok) {
        errors += 1;
        lines.push(
          `ERROR area-skill-import: ${skill.name}: tsc did not list any file of ${directory}: ${evidence.detail}`,
        );
        continue;
      }
      violations.push(
        ...skillImportViolations(
          skill.name,
          evidence.files,
          evidence.unresolved,
        ),
      );
    }
  }

  const sorted = [...violations].sort((a, b) =>
    `${a.rule}${a.path}${a.message}`.localeCompare(
      `${b.rule}${b.path}${b.message}`,
    ),
  );
  const fails = sorted.map((v) => `FAIL ${v.rule}: ${v.path}: ${v.message}`);
  lines.unshift(...fails);
  lines.push(`boundaries: ${violations.length} failures`);
  return { lines, code: errors > 0 ? 2 : violations.length > 0 ? 1 : 0 };
}

function main(argv: string[]) {
  let repo = process.cwd();
  for (let i = 0; i < argv.length; i += 1) {
    if (argv[i] === "--repo" && argv[i + 1] !== undefined)
      repo = path.resolve(argv[++i]!);
    else {
      console.error(
        "usage: node .github/tools/src/boundaries/cli.ts [--repo <root>]",
      );
      process.exitCode = 2;
      return;
    }
  }
  const report = run({ root: repo });
  console.log(report.lines.join("\n"));
  process.exitCode = report.code;
}

if (import.meta.main) main(process.argv.slice(2));
