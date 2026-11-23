export { AREAS, AREA_ROOTS, type Area } from "../repository-areas.ts";

/** The areas that are npm workspaces and have a TypeScript `src/`. */
export const TS_AREAS = ["tools"] as const;
export type TsArea = (typeof TS_AREAS)[number];

export interface Violation {
  rule: string;
  path: string;
  message: string;
}
