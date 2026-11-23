import { AREA_ROOTS, AREAS, type Violation } from "./areas.ts";

export function codeownersViolations(text: string): Violation[] {
  const entries = new Map<string, string[]>();
  for (const raw of text.split(/\r?\n/)) {
    const line = raw.replace(/#.*$/, "").trim();
    if (line === "") continue;
    const [pattern, ...owners] = line.split(/\s+/);
    entries.set(pattern!, owners);
  }
  const out: Violation[] = [];
  for (const area of AREAS) {
    const pattern = `/${AREA_ROOTS[area]}/`;
    if ((entries.get(pattern)?.length ?? 0) === 0) {
      out.push({
        rule: "area-codeowners",
        path: "CODEOWNERS",
        message: `no owner entry for ${pattern}`,
      });
    }
  }
  return out;
}
