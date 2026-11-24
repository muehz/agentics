import { lstatSync } from "node:fs";
import path from "node:path";
import type { Failure } from "../../check.ts";
import { repoFileErrorOf } from "../../repo-file-error.ts";
import {
  isFileErrorOfKind,
  readRepoFile,
  skillsTrackedLookup,
} from "./files.ts";
import type { Family, ResolvedSkillsContext } from "./index.ts";

/**
 * The byte limit of a skill's `SKILL.md`. Antigravity loads only the first 24,000 bytes of each `SKILL.md` body and
 * warns on longer files, so capping the whole `SKILL.md` at 24,000 bytes guarantees the body fits with zero truncation
 * on Antigravity (as well as Claude Code and Codex).
 */
export const SKILL_MD_BYTE_LIMIT = 24000;
/**
 * The byte limit of every other tracked file under `skills/`. Codex estimates a tool output's tokens as
 * `ceil(UTF-8 bytes / 4)` and cuts any single output over 10,000 tokens (40,000 bytes). Capping raw files at 30,000
 * bytes leaves headroom for line-number prefixes and tool framing and fits in a single Codex eval reader page.
 */
export const SKILL_FILE_BYTE_LIMIT = 30000;

export const FILE_SIZE_RULE = "skill-file-size";
export const FILE_SIZE_STALE_RULE = "skill-file-size-stale";
const ALLOWLIST = ".github/tools/src/conformance/skill-size-allowlist.json";
const SPLIT_GUIDE = "skills/forge/references/forging/forging-skills.md";
const SKILL_MD = /^skills\/[^/]+\/SKILL\.md$/;

function limitFor(rel: string): number {
  return SKILL_MD.test(rel) ? SKILL_MD_BYTE_LIMIT : SKILL_FILE_BYTE_LIMIT;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

/** The allowlist's ceilings by path. A missing file is an empty allowlist; a malformed one throws. */
function readAllowlist(repoRoot: string): Map<string, number> {
  let text: string;
  try {
    text = readRepoFile(repoRoot, ALLOWLIST);
  } catch (error) {
    if (isFileErrorOfKind(error, "missing")) return new Map();
    throw error;
  }
  let entries: unknown;
  try {
    entries = JSON.parse(text);
  } catch (error) {
    throw new Error(`skills: ${ALLOWLIST}: invalid JSON`, { cause: error });
  }
  if (!Array.isArray(entries))
    throw new Error(`skills: ${ALLOWLIST}: expected a JSON array`);
  const ceilings = new Map<string, number>();
  for (const [index, entry] of entries.entries()) {
    const prefix = `skills: ${ALLOWLIST}: entry ${index}`;
    if (
      !isRecord(entry) ||
      typeof entry.path !== "string" ||
      typeof entry.ceiling !== "number" ||
      !Number.isSafeInteger(entry.ceiling)
    )
      throw new Error(`${prefix} needs a string path and an integer ceiling`);
    const { path: rel, ceiling } = entry;
    if (!rel.startsWith("skills/"))
      throw new Error(`${prefix} names a path outside skills/`);
    if (ceilings.has(rel))
      throw new Error(`${prefix} duplicates an earlier entry for ${rel}`);
    const limit = limitFor(rel);
    if (ceiling <= limit)
      throw new Error(
        `${prefix} has ceiling ${ceiling}, at or below the limit of ${limit}`,
      );
    ceilings.set(rel, ceiling);
  }
  return ceilings;
}

/** The byte size of a regular file, or undefined when the path is missing or not a regular file. */
function sizeOf(repoRoot: string, rel: string): number | undefined {
  try {
    const info = lstatSync(path.join(repoRoot, ...rel.split("/")));
    return info.isFile() ? info.size : undefined;
  } catch (error) {
    if (repoFileErrorOf(error, rel).kind === "missing") return undefined;
    throw new Error(`skills: lstat ${rel}: ${(error as Error).message}`, {
      cause: error,
    });
  }
}

/**
 * Caps every tracked file under `skills/` at its byte limit. A file listed in the allowlist may instead reach its
 * ceiling, and an entry whose file is untracked or back within its limit is stale. The findings never pass through
 * the standards baseline, so no baseline entry can accept them.
 */
export function fileSizeRules(
  repoRoot: string,
  context: Partial<ResolvedSkillsContext> = {},
): Failure[] {
  const ceilings = readAllowlist(repoRoot);
  const tracked = (
    context.tracked ?? skillsTrackedLookup(repoRoot, process.env)
  )().files;
  const failures: Failure[] = [];
  for (const rel of tracked) {
    if (!rel.startsWith("skills/")) continue;
    const bytes = sizeOf(repoRoot, rel);
    if (bytes === undefined) continue;
    const limit = limitFor(rel);
    const ceiling = ceilings.get(rel);
    if (ceiling === undefined) {
      if (bytes > limit)
        failures.push({
          rule: FILE_SIZE_RULE,
          path: rel,
          message: `${rel} is ${bytes} bytes; the limit is ${limit}. Split it along its sections; see ${SPLIT_GUIDE}`,
        });
    } else if (bytes > ceiling) {
      failures.push({
        rule: FILE_SIZE_RULE,
        path: rel,
        message: `${rel} is ${bytes} bytes; its allowlist ceiling is ${ceiling} and the limit is ${limit}. A listed file may shrink but not grow`,
      });
    } else if (bytes <= limit) {
      failures.push({
        rule: FILE_SIZE_STALE_RULE,
        path: ALLOWLIST,
        message: `allowlist entry for ${rel} is stale: the file is ${bytes} bytes, within the limit of ${limit}. Remove the entry`,
      });
    }
  }
  for (const rel of ceilings.keys())
    if (!tracked.has(rel))
      failures.push({
        rule: FILE_SIZE_STALE_RULE,
        path: ALLOWLIST,
        message: `allowlist entry for ${rel} names no tracked file`,
      });
  return failures.sort(byPathThenMessage);
}

function byPathThenMessage(a: Failure, b: Failure): number {
  if (a.path !== b.path) return a.path < b.path ? -1 : 1;
  if (a.message !== b.message) return a.message < b.message ? -1 : 1;
  return 0;
}

export const FAMILY: Family = {
  name: "skill-file-size",
  rules: [FILE_SIZE_RULE, FILE_SIZE_STALE_RULE],
  run: fileSizeRules,
};
