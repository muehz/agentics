import { isProductionSource } from "../scripts/mutation-source.ts";

export {
  EXCLUDE_PATTERNS,
  SOURCE_ROOTS,
} from "../scripts/mutation-source.ts";

export function inCoverageScope(filename: string): boolean {
  return isProductionSource(filename, "coverage");
}
