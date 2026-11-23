import type { RunOptions } from "./check.ts";
import type { Host } from "./host.ts";
import type { DomainRegistration } from "./registry.ts";

/** One domain's rendered outcome: its exit status and whether it failed only on stale ledger reviews. */
export interface DomainReport {
  status: 0 | 1;
  /** True when the domain classifies its completed audit as stale-review-only. */
  staleReviewOnly: boolean;
}

/**
 * Runs one domain and renders its outcome: PASS on stdout; FAIL, ERROR and the count on stderr. Audit errors and
 * rule failures render distinctly, because an auditor that cannot run must never read as a passing checkout.
 */
export async function reportDomain(
  name: string,
  domain: DomainRegistration,
  repoRoot: string,
  options: RunOptions,
  host: Host,
): Promise<DomainReport> {
  const { run, staleReviewOnly } = domain;
  const result = await run(repoRoot, options);
  for (const failure of result.failures) {
    host.err(
      failure.path !== ""
        ? `FAIL ${name} [${failure.rule}] ${failure.path}: ${failure.message}\n`
        : `FAIL ${name} [${failure.rule}] ${failure.message}\n`,
    );
  }
  if (result.error !== undefined) {
    host.err(`ERROR ${name}: audit could not run: ${result.error}\n`);
    return { status: 1, staleReviewOnly: false };
  }
  if (result.failures.length > 0) {
    host.err(`${name}: ${result.failures.length} failure(s)\n`);
    return {
      status: 1,
      staleReviewOnly: staleReviewOnly?.(result) ?? false,
    };
  }
  host.out(`PASS ${name}\n`);
  return { status: 0, staleReviewOnly: false };
}
