import path from "node:path";
import {
  AREA_ROOTS,
  AREAS,
  TS_AREAS,
  type TsArea,
  type Violation,
} from "./areas.ts";

type Json = Record<string, unknown>;
const FIELDS = [
  "dependencies",
  "devDependencies",
  "peerDependencies",
  "optionalDependencies",
] as const;
const rule = "area-ts-dependency";

function entries(json: Json, field: string): [string, string][] {
  const value = json[field];
  if (typeof value !== "object" || value === null) return [];
  return Object.entries(value as Record<string, unknown>).map(
    ([name, spec]) => [name, String(spec)],
  );
}

/**
 * The directory a `file:`, `link:`, or `workspace:` value points at, relative to the repository root, or null when
 * the value does not name a directory. A `workspace:*` or `workspace:^` range selects a package by name, and the
 * only workspace is the area itself, so a range names no other area.
 */
function localTarget(area: string, spec: string): string | null {
  const match = /^(file|link|workspace):(.*)$/.exec(spec);
  if (!match) return null;
  const value = match[2]!;
  if (match[1] === "workspace" && !/^\.{1,2}(?:\/|$)|^\//.test(value))
    return null;
  if (path.posix.isAbsolute(value)) return value;
  return path.posix.normalize(path.posix.join(area, value));
}

/** The area whose directory holds `target`, or its first path segment when no area does. */
function areaAt(target: string): string {
  for (const area of AREAS) {
    if (target.startsWith(`${AREA_ROOTS[area]}/`)) return area;
  }
  return target.split("/")[0]!;
}

/**
 * `root` is the root package.json; `packages` maps each TypeScript area to its package.json, or null if missing. No
 * area may depend on a local directory: the areas share no code, so such a value reaches into another area.
 */
export function packageViolations(
  root: Json,
  packages: Record<TsArea, Json | null>,
): Violation[] {
  const out: Violation[] = [];
  const roots = TS_AREAS.map((area) => AREA_ROOTS[area]);
  const workspaces = Array.isArray(root.workspaces)
    ? root.workspaces.map(String)
    : [];
  if ([...workspaces].sort().join(",") !== [...roots].sort().join(",")) {
    out.push({
      rule,
      path: "package.json",
      message: `workspaces must be exactly ${roots.join(", ")}; found ${workspaces.join(", ") || "none"}`,
    });
  }
  for (const area of TS_AREAS) {
    const home = AREA_ROOTS[area];
    const json = packages[area];
    if (json === null) {
      out.push({
        rule,
        path: `${home}/package.json`,
        message: `${area} is a workspace but has no package.json`,
      });
      continue;
    }
    for (const field of FIELDS) {
      for (const [name, spec] of entries(json, field)) {
        const resolved = localTarget(home, spec);
        if (resolved === null) continue;
        out.push({
          rule,
          path: `${home}/package.json`,
          message: `${field} entry ${name} uses ${spec}, which targets ${areaAt(resolved)}; ${area} depends on no local directory`,
        });
      }
    }
  }
  return out;
}
