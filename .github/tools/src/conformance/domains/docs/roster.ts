import { readdirSync, readFileSync } from "node:fs";
import path from "node:path";
import { type AuditResult, errorMessage, type Failure } from "../../check.ts";
import { collapseWhitespace } from "../../text.ts";
import { repoFileErrorOf } from "../../repo-file-error.ts";

/** Runs `read` on the absolute path of the repo-relative `rel`, turning a failure into a `RepoFileError`. */
function onRel<T>(repoRoot: string, rel: string, read: (abs: string) => T): T {
  try {
    return read(path.join(repoRoot, ...rel.split("/")));
  } catch (error) {
    throw repoFileErrorOf(error, rel);
  }
}

/** The counts a prose claim can be checked against, derived from the checkout. */
export type RosterCount = "shipped skills" | "Copilot-registered skills";

export type RosterCounts = Readonly<Record<RosterCount, number>>;

export interface RosterClaim {
  path: string;
  label: string;
  /** Matches the claim in whitespace-collapsed text; its one capture is the number claimed. */
  pattern: RegExp;
  want: RosterCount;
}

/**
 * Limited to current-state prose. Dated probe evidence, plans, and rename history preserve the counts observed at the
 * time and must not be rewritten to today's roster.
 */
export const ROSTER_CLAIMS: readonly RosterClaim[] = [
  {
    path: "docs/ARCHITECTURE.md",
    label: "canonical skill roster",
    pattern: /canonical (\d+)-skill roster/g,
    want: "shipped skills",
  },
  {
    path: "docs/ARCHITECTURE.md",
    label: "canonical skill tree",
    pattern: /canonical (\d+)-skill tree/g,
    want: "shipped skills",
  },
  {
    path: "docs/ARCHITECTURE.md",
    label: "Copilot skill coverage",
    pattern: /Copilot CLI ships all (\d+) skills/g,
    want: "Copilot-registered skills",
  },
  {
    path: "docs/surfaces/claude-code.md",
    label: "Claude manifest registration",
    pattern: /registers all (\d+) skills/g,
    want: "shipped skills",
  },
  {
    path: "docs/surfaces/copilot-cli.md",
    label: "Copilot manifest registration",
    pattern: /registers all (\d+) Agentics skills/g,
    want: "Copilot-registered skills",
  },
];

/** Audits the roster claims against counts derived from the checkout. */
export function rosterCountRules(repoRoot: string): AuditResult {
  let counts: RosterCounts;
  try {
    counts = deriveRosterCounts(repoRoot);
  } catch (error) {
    return { failures: [], error: errorMessage(error) };
  }
  return checkRosterClaims(repoRoot, counts);
}

const MAX_INT = 2n ** 63n - 1n;

/** Thrown when the Copilot manifest is not valid JSON. */
class ManifestSyntaxError extends Error {}

/** The repo-relative home of the Copilot manifest, whose `skills` array is the registered roster. */
const COPILOT_MANIFEST = ".github/plugin/plugin.json";

/** Checks every claim against `counts`. A claim site that is missing or no longer matches is itself a failure. */
export function checkRosterClaims(
  repoRoot: string,
  counts: RosterCounts,
): AuditResult {
  const texts = new Map<string, string>();
  const failures: Failure[] = [];
  for (const claim of ROSTER_CLAIMS) {
    let text = texts.get(claim.path);
    if (text === undefined) {
      try {
        const data = onRel(repoRoot, claim.path, (abs) =>
          readFileSync(abs, "utf8"),
        );
        text = collapseWhitespace(data);
        texts.set(claim.path, text);
      } catch (error) {
        const fileError = repoFileErrorOf(error, claim.path);
        if (fileError.kind === "missing") {
          failures.push({
            rule: "roster-claim-site-present",
            path: claim.path,
            message: `${claim.label} cannot be audited because the file is missing`,
          });
          continue;
        }
        return { failures, error: `docs: ${fileError.message}` };
      }
    }
    const matches = [...text.matchAll(claim.pattern)];
    if (matches.length === 0) {
      failures.push({
        rule: "roster-claim-site-present",
        path: claim.path,
        message: `${claim.label} no longer matches its current-state claim`,
      });
      continue;
    }
    for (const match of matches) {
      const digits = match[1] ?? "";
      const claimed = BigInt(digits);
      if (claimed > MAX_INT)
        return {
          failures,
          error: `docs: parsing roster claim ${JSON.stringify(claim.label)}: ${JSON.stringify(digits)} is out of range`,
        };
      const want = counts[claim.want];
      if (claimed !== BigInt(want))
        failures.push({
          rule: "roster-count-drift",
          path: claim.path,
          message: `${claim.label} says ${claimed}; ${claim.want} is ${want}`,
        });
    }
  }
  return { failures };
}

/** The shipped-skill count (directories under skills/) and the Copilot manifest's registered-skill count. */
export function deriveRosterCounts(repoRoot: string): RosterCounts {
  let skills: number;
  let manifest: string;
  try {
    skills = onRel(repoRoot, "skills", (abs) =>
      readdirSync(abs, { withFileTypes: true }),
    ).filter((entry) => entry.isDirectory()).length;
    manifest = onRel(repoRoot, COPILOT_MANIFEST, (abs) =>
      readFileSync(abs, "utf8"),
    );
  } catch (error) {
    throw new Error(`docs: ${errorMessage(error)}`, { cause: error });
  }
  let registered: number;
  try {
    registered = registeredSkills(manifest);
  } catch (error) {
    if (error instanceof ManifestSyntaxError)
      throw new Error(`docs: ${COPILOT_MANIFEST}: is not valid JSON`);
    throw new Error(`docs: parsing Copilot manifest: ${errorMessage(error)}`);
  }
  if (skills === 0 || registered === 0)
    throw new Error("docs: cannot derive roster counts from an empty roster");
  return { "shipped skills": skills, "Copilot-registered skills": registered };
}

/**
 * The length of the manifest's `skills` array, read by its exact key; a duplicate key resolves to its last value. An
 * absent or null array is empty; any other shape is an error.
 */
function registeredSkills(source: string): number {
  let manifest: unknown;
  try {
    manifest = JSON.parse(source);
  } catch {
    // The engine's syntax message is left out: its wording varies by version and it quotes the audited input.
    throw new ManifestSyntaxError();
  }
  if (manifest === null) return 0;
  if (typeof manifest !== "object" || Array.isArray(manifest))
    throw new Error("the manifest is not a JSON object");
  const skills: unknown = Object.hasOwn(manifest, "skills")
    ? (manifest as { skills: unknown }).skills
    : null;
  if (skills === null) return 0;
  if (!Array.isArray(skills)) throw new Error("skills is not an array");
  if (skills.some((skill) => skill !== null && typeof skill !== "string"))
    throw new Error("skills holds a value that is not a string");
  return skills.length;
}
