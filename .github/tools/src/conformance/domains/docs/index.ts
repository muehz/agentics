import type { AuditResult, Failure, RunOptions } from "../../check.ts";
import { ledgerRules } from "./ledger.ts";
import { modelIdRules } from "./model-ids.ts";
import { rosterCountRules } from "./roster.ts";

/** The environment variable that arms the ledger's max-age check. */
export const ENV_MAX_AGE_DAYS = "AGX_LEDGER_MAX_AGE_DAYS";

/** Whether the documentation audit completed with only stale ledger reviews. */
export function staleReviewOnly(result: AuditResult): boolean {
  return (
    result.error === undefined &&
    result.failures.length > 0 &&
    result.failures.every(({ rule }) => rule === "review-age")
  );
}

/** `now` is a test seam for the clock the age check reads. */
export interface DocsOptions extends RunOptions {
  now?: () => Date;
}

/**
 * Parses the knob as a base-10 integer: an optional `+` or `-` and then decimal digits only, within the signed 64-bit
 * range. Whitespace, a decimal point, an exponent or an out-of-range value is rejected. A negative value is a valid
 * knob that disarms the check, as an unset one does.
 */
function parseMaxAge(raw: string): number | undefined {
  if (!/^[+-]?[0-9]+$/.test(raw)) return undefined;
  const value = BigInt(raw);
  return value >= -(2n ** 63n) && value < 2n ** 63n ? Number(value) : undefined;
}

/**
 * Audits the checkout's documentation: the roster claims in current-state prose, the upstream-review ledger and the
 * surface docs it covers, and the model identifiers in the copies that must agree with the ladder.
 *
 * The ledger's max-age check is armed only when AGX_LEDGER_MAX_AGE_DAYS is non-empty, so ordinary conformance runs
 * impose no age policy and stay independent of the clock. An audit error in one area ends the run; the failures the
 * earlier areas and that area found so far are still reported.
 */
export async function run(
  repoRoot: string,
  options: DocsOptions,
): Promise<AuditResult> {
  let maxAge = -1;
  const raw = options.env[ENV_MAX_AGE_DAYS] ?? "";
  if (raw !== "") {
    const parsed = parseMaxAge(raw);
    if (parsed === undefined)
      return {
        failures: [],
        error: `docs: ${ENV_MAX_AGE_DAYS}=${JSON.stringify(raw)} is not an integer`,
      };
    maxAge = parsed;
  }
  const now = options.now ?? (() => new Date());
  const areas: Array<() => AuditResult> = [
    () => rosterCountRules(repoRoot),
    () => ledgerRules(repoRoot, maxAge, now()),
    () => modelIdRules(repoRoot),
  ];
  const failures: Failure[] = [];
  for (const area of areas) {
    const found = area();
    failures.push(...found.failures);
    if (found.error !== undefined) return { failures, error: found.error };
  }
  return { failures };
}
