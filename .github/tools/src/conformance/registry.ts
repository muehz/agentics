import type { AuditResult, DomainRun, RunOptions } from "./check.ts";
import { run as runDocs, staleReviewOnly } from "./domains/docs/index.ts";
import { run as runHooks } from "./domains/hooks/index.ts";
import { run as runManifests } from "./domains/manifests/index.ts";
import { run as runProbes } from "./domains/probes/index.ts";
import { run as runRepo } from "./domains/repo/index.ts";

/** The audit error the skills domain reports when the `yaml` package cannot be resolved. */
export const YAML_MISSING =
  "the yaml package is not installed; run npm ci --ignore-scripts at the repository root";

/** The part of the skills domain module that the registry calls. */
type SkillsDomain = Pick<typeof import("./domains/skills/index.ts"), "run">;
type ImportSkillsDomain = () => Promise<SkillsDomain>;

/**
 * Whether the error is Node's report that the bare specifier `yaml` did not resolve. The quoted package name keeps a
 * missing relative file, or another package whose name starts with `yaml`, out of the match.
 */
function isMissingYaml(error: unknown): boolean {
  return (
    error instanceof Error &&
    (error as NodeJS.ErrnoException).code === "ERR_MODULE_NOT_FOUND" &&
    error.message.includes("Cannot find package 'yaml'")
  );
}

/**
 * Runs the skills domain, importing its module only now: it is the one domain that needs an installed package
 * (`yaml`), and the plugin install runs `manifests` on hosts with no packages. A missing `yaml` is the audit error
 * {@link YAML_MISSING}; any other import failure is thrown. The evals runner gets the run's own environment, as every
 * other child process of the suite gets it, rather than `process.env`. `importDomain` is a test seam.
 */
export async function runSkillsDomain(
  repoRoot: string,
  options: RunOptions,
  importDomain: ImportSkillsDomain = () => import("./domains/skills/index.ts"),
): Promise<AuditResult> {
  let domain: SkillsDomain;
  try {
    domain = await importDomain();
  } catch (error) {
    if (isMissingYaml(error)) return { failures: [], error: YAML_MISSING };
    throw error;
  }
  return domain.run(repoRoot, options, { evalsInvocation: options });
}

/** A domain's invoker, selection policy, and domain-specific outcome classification. */
export interface DomainRegistration {
  run: DomainRun;
  /** Excluded from `all`; it runs only by name. */
  optIn?: true;
  /** Supplies the classifier for domains that support `--stale-review-exit-code`. */
  staleReviewOnly?: (result: AuditResult) => boolean;
}

/** The facts the CLI needs to run and render each domain. */
export const DOMAIN_REGISTRATIONS: Readonly<
  Record<string, DomainRegistration>
> = {
  docs: { run: runDocs, staleReviewOnly },
  hooks: { run: runHooks },
  manifests: { run: runManifests },
  repo: { run: runRepo },
  skills: { run: runSkillsDomain },
  probes: { run: runProbes, optIn: true },
};

/** Looks up a registered domain by its exact name. */
export function registeredDomain(name: string): DomainRegistration | undefined {
  return Object.hasOwn(DOMAIN_REGISTRATIONS, name)
    ? DOMAIN_REGISTRATIONS[name]
    : undefined;
}

/** The sorted names that `all` can select, including invokers injected by CLI tests. */
export function ordinaryDomainNames(
  domains?: Readonly<Record<string, DomainRun>>,
): string[] {
  return Object.keys(domains ?? DOMAIN_REGISTRATIONS)
    .filter((name) => !registeredDomain(name)?.optIn)
    .sort();
}

const registrations = Object.entries(DOMAIN_REGISTRATIONS);
const optInNames = registrations
  .filter(([, domain]) => domain.optIn)
  .map(([name]) => `${name} (opt-in)`)
  .sort()
  .join(", ");

/** The domain names that accept the stale-review exit option. */
export const STALE_REVIEW_DOMAINS = registrations
  .filter(([, domain]) => domain.staleReviewOnly !== undefined)
  .map(([name]) => name)
  .sort()
  .join(", ");

/** The CLI usage text, listing ordinary domains, `all`, and then opt-in domains. */
export function usage(domains?: Readonly<Record<string, DomainRun>>): string {
  return [
    "usage: conformance <domain> [--repo <path-to-agentics-checkout>] [--allow-missing] [--short] [--fast] [--exclude <domain>]... [--stale-review-exit-code]",
    `domains: ${ordinaryDomainNames(domains).join(", ")}, all; ${optInNames}`,
    "--fast: implies --short; also skips the hooks module tests",
    "--exclude: all only; skip the named domain",
    `--stale-review-exit-code: ${STALE_REVIEW_DOMAINS} only; exit 3 for exclusively stale reviews`,
  ].join("\n");
}
