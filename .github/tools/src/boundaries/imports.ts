import { builtinModules } from "node:module";
import { AREA_ROOTS, type TsArea, type Violation } from "./areas.ts";
import type { Reason, ResolvedFile } from "./explain.ts";

const THIRD_PARTY = /(^|\/)node_modules\//;

/**
 * Counts edges, not files: the importer is inside the area and the target is a repository path outside it.
 * A file reached only through an outside file is not reported again; that file's own area run covers it.
 */
export function areaImportCheck(
  area: TsArea,
  files: ResolvedFile[],
): Violation[] {
  const home = `${AREA_ROOTS[area]}/`;
  const violations: Violation[] = [];
  const seen = new Set<string>();
  for (const { file, reasons } of files) {
    if (THIRD_PARTY.test(file)) continue;
    for (const { specifier, importer } of reasons) {
      if (!importer.startsWith(home)) continue;
      if (file.startsWith(home)) continue;
      const key = `${importer}\u0000${specifier}`;
      if (seen.has(key)) continue;
      seen.add(key);
      violations.push({
        rule: "area-ts-import",
        path: importer,
        message: `imports "${specifier}", which resolves to ${file} outside ${area}`,
      });
    }
  }
  return violations;
}

const NODE_TYPES = "node_modules/@types/node/";

/** A Node built-in specifier, such as `node:fs`, `fs`, or `fs/promises`. */
export function isNodeBuiltin(specifier: string): boolean {
  return specifier.startsWith("node:") || builtinModules.includes(specifier);
}

/**
 * A shipped skill script has no node_modules on a plugin user's machine, so it may import its own files and Node
 * built-ins (whose declarations live in node_modules/@types/node) and nothing else. `unresolved` lists the imports
 * the compiler could not resolve: a package that is not installed is a dependency the script cannot rely on, so it
 * fails too unless it is a built-in.
 */
export function skillImportViolations(
  skill: string,
  files: ResolvedFile[],
  unresolved: Reason[] = [],
): Violation[] {
  const home = `skills/${skill}/`;
  const violations: Violation[] = [];
  const seen = new Set<string>();
  for (const { file, reasons } of files) {
    if (file.startsWith(home)) continue;
    for (const { specifier, importer } of reasons) {
      if (!importer.startsWith(home)) continue;
      if (file.startsWith(NODE_TYPES) && isNodeBuiltin(specifier)) continue;
      const key = `${importer}\u0000${specifier}`;
      if (seen.has(key)) continue;
      seen.add(key);
      violations.push({
        rule: "area-skill-import",
        path: importer,
        message: `imports "${specifier}", which resolves to ${file} outside ${home}; a skill script depends on nothing in the repository`,
      });
    }
  }
  for (const { specifier, importer } of unresolved) {
    if (!importer.startsWith(home) || isNodeBuiltin(specifier)) continue;
    const key = `${importer}\u0000${specifier}`;
    if (seen.has(key)) continue;
    seen.add(key);
    violations.push({
      rule: "area-skill-import",
      path: importer,
      message: `imports "${specifier}", which does not resolve to an installed package; a skill script may import only its own files and Node built-ins`,
    });
  }
  return violations;
}
