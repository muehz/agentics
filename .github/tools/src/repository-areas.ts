/** Canonical repository-relative roots, in area order. */
export const AREA_ROOTS = {
  hooks: "hooks",
  evals: "evals",
  tools: ".github/tools",
  skills: "skills",
} as const;

export type Area = keyof typeof AREA_ROOTS;
export const AREAS = Object.keys(AREA_ROOTS) as readonly Area[];
