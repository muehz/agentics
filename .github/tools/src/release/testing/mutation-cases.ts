import { artifactCases } from "./mutation-cases-artifacts.ts";
import { gateCases } from "./mutation-cases-gates.ts";
import { prepareCases } from "./mutation-cases-prepare.ts";
import type { MutationCase } from "./mutation-support.ts";

// The whole mutation table. Each group file holds one run of consecutive cases, and the groups keep their order.
export const mutationCases: readonly MutationCase[] = [
  ...gateCases,
  ...prepareCases,
  ...artifactCases,
];
