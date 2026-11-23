import { existsSync } from "node:fs";
import path from "node:path";
import {
  type DomainRun,
  ENV_SHORT,
  errorMessage,
  MARKER,
  repoRoot as findRepoRoot,
  type RunOptions,
} from "./check.ts";
import type { Host } from "./host.ts";
import {
  DOMAIN_REGISTRATIONS,
  type DomainRegistration,
  ordinaryDomainNames,
  registeredDomain,
  STALE_REVIEW_DOMAINS,
  usage,
} from "./registry.ts";
import { type DomainReport, reportDomain } from "./report.ts";

/** The stderr line acknowledging the accepted short-mode option. */
export const SHORT_ANNOUNCEMENT = `conformance: short mode (${ENV_SHORT}=1)\n`;

/** The stderr line that says fast mode skipped the module-test gates. It replaces the short-mode line, which fast implies. */
export const FAST_ANNOUNCEMENT =
  "conformance: fast mode (--fast) — the hooks module tests are NOT run; this is not a full audit\n";

/** The exit code with --stale-review-exit-code when `docs` failed only on stale ledger reviews. */
export const STALE_REVIEW_EXIT = 3;

/**
 * The conformance CLI. It runs one domain, or every registered domain for `all`, against the checkout named by
 * `--repo` or found above the working directory. Exit codes: 0 pass; 1 rule failures, an audit that could not run, or
 * bad arguments; and 3, only with --stale-review-exit-code, when `docs` failed solely on stale ledger reviews.
 * `domains` is a test seam; production passes the registry.
 */
export async function main(
  argv: string[],
  host: Host,
  domains?: Readonly<Record<string, DomainRun>>,
): Promise<number> {
  const help = () => `${usage(domains)}\n`;
  if (argv.length < 1) {
    host.err(help());
    return 1;
  }
  const domain = argv[0] ?? "";
  let repo = "";
  let staleReviewExit = false;
  let allowMissing = false;
  let short = host.env[ENV_SHORT] === "1";
  let fast = false;
  const excluded: string[] = [];
  for (let i = 1; i < argv.length; i++) {
    const arg = argv[i] ?? "";
    if (arg === "--repo") {
      if (i + 1 >= argv.length) {
        host.err(help());
        return 1;
      }
      repo = argv[++i] ?? "";
    } else if (arg === "--allow-missing") {
      allowMissing = true;
    } else if (arg === "--stale-review-exit-code") {
      if (staleReviewExit) {
        host.err("conformance: --stale-review-exit-code repeated\n");
        return 1;
      }
      staleReviewExit = true;
    } else if (arg === "--short") {
      short = true;
    } else if (arg === "--fast") {
      fast = true;
      short = true;
    } else if (arg === "--exclude") {
      if (i + 1 >= argv.length) {
        host.err(`conformance: --exclude needs a domain\n${help()}`);
        return 1;
      }
      excluded.push(argv[++i] ?? "");
    } else {
      host.err(
        `conformance: unknown argument ${JSON.stringify(arg)}\n${help()}`,
      );
      return 1;
    }
  }
  if (
    staleReviewExit &&
    registeredDomain(domain)?.staleReviewOnly === undefined
  ) {
    host.err(
      `conformance: --stale-review-exit-code requires ${STALE_REVIEW_DOMAINS}\n`,
    );
    return 1;
  }
  const exclusionError = checkExclusions(domain, excluded, domains);
  if (exclusionError !== "") {
    host.err(`conformance: ${exclusionError}\n${help()}`);
    return 1;
  }
  if (repo === "") {
    repo = findRepoRoot(host.cwd);
    if (repo === "") {
      host.err(
        `conformance: no agentics checkout found above the working directory (missing ${MARKER}); pass --repo\n${help()}`,
      );
      return 1;
    }
  }
  const absolute = path.resolve(host.cwd, repo);
  if (fast) host.err(FAST_ANNOUNCEMENT);
  else if (short) host.err(SHORT_ANNOUNCEMENT);
  const selected = selectDomain(domain, domains);
  const known = selected !== undefined || domain === "all";
  const marker = path.join(absolute, ...MARKER.split("/"));
  if (!existsSync(marker)) {
    host.err(
      `conformance: ${absolute} does not look like an agentics checkout (missing ${marker})\n${known ? "" : help()}`,
    );
    return 1;
  }
  const options: RunOptions & { allowMissing: boolean } = {
    short,
    fast,
    allowMissing,
    diagnostics: host.err,
    env: host.env,
  };
  const runOne = async (
    name: string,
    registration: DomainRegistration,
  ): Promise<DomainReport> => {
    try {
      return await reportDomain(name, registration, absolute, options, host);
    } catch (error) {
      // A domain that throws must never read as a pass.
      host.err(`ERROR ${name}: audit could not run: ${errorMessage(error)}\n`);
      return { status: 1, staleReviewOnly: false };
    }
  };
  const exitCode = (report: DomainReport): number =>
    staleReviewExit && report.status !== 0 && report.staleReviewOnly
      ? STALE_REVIEW_EXIT
      : report.status;
  if (domain === "all") {
    const names = ordinaryDomainNames(domains).filter(
      (name) => !excluded.includes(name),
    );
    let status = 0;
    for (const name of names) {
      const named = selectDomain(name, domains);
      if (named !== undefined && (await runOne(name, named)).status !== 0)
        status = 1;
    }
    return status;
  }
  if (selected === undefined) {
    host.err(
      `conformance: unknown domain ${JSON.stringify(domain)}\n${help()}`,
    );
    return 1;
  }
  return exitCode(await runOne(domain, selected));
}

/** Applies a test's live invoker map while retaining registered policy and named opt-in fallbacks. */
function selectDomain(
  name: string,
  domains?: Readonly<Record<string, DomainRun>>,
): DomainRegistration | undefined {
  const registered = registeredDomain(name);
  if (domains === undefined) return registered;
  if (!Object.hasOwn(domains, name))
    return registered?.optIn ? registered : undefined;
  const run = domains[name];
  return run === undefined ? undefined : { ...registered, run };
}

/**
 * The reason the --exclude names are invalid for this run, or "" when they are valid. Only `all` takes exclusions,
 * and each name must be a domain `all` would run, so a typo cannot silently leave a domain in the run.
 */
function checkExclusions(
  domain: string,
  excluded: readonly string[],
  domains?: Readonly<Record<string, DomainRun>>,
): string {
  if (excluded.length === 0) return "";
  if (domain !== "all") return "--exclude requires all";
  const seen = new Set<string>();
  for (const name of excluded) {
    if (registeredDomain(name)?.optIn)
      return `--exclude cannot name opt-in domain ${JSON.stringify(name)}; all never runs it`;
    if (!Object.hasOwn(domains ?? DOMAIN_REGISTRATIONS, name))
      return `--exclude names unknown domain ${JSON.stringify(name)}`;
    if (seen.has(name)) return `--exclude ${JSON.stringify(name)} repeated`;
    seen.add(name);
  }
  return "";
}
