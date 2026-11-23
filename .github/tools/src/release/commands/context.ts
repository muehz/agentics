import { mkdirSync } from "node:fs";
import { join } from "node:path";
import { parseArgs } from "../args.ts";
import { GithubClient, git, type Release } from "../github.ts";
import {
  EXIT_FAILURE,
  EXIT_OK,
  EXIT_USAGE,
  report,
  type Env,
  type Io,
  type Run,
} from "../io.ts";
import {
  PLACEHOLDER_TAG_PREFIX,
  conflictPlaceholderTag,
  conflictStrayPlaceholderTag,
  isPlaceholderTag,
  splitRepo,
} from "../identity.ts";
import { fileErrorOf, readFile, writeFile } from "../files.ts";
import {
  FULL_SHA,
  STABLE_TAG,
  lessThan,
  tagVersion,
  type Version,
} from "../version.ts";

const PREFIX = "release: context";
const REQUIRED =
  "--repo, --checkout, --sha, --tag, --baseline, and --out are required";
const FLAGS = ["repo", "checkout", "sha", "tag", "baseline", "out"];

/** Holds the baseline file's notes, or the subject list that stands in for empty ones, plus the licenses paragraph. */
export const BASELINE_NOTES_NAME = "baseline-notes.md";
/** Holds the baseline plus one section for each pull request a squash subject in the range names. */
export const CONTEXT_NOTES_NAME = "context.md";
/** Closes every baseline. The notices asset carries the notices themselves; this paragraph points a reader at it. */
export const LICENSES_PARAGRAPH =
  "\n\n## Licenses\n\nThird-party license notices for the Go-built hook binaries are included in the `THIRD_PARTY_NOTICES.md` release asset.\n";

// A leading zero is rejected: GitHub never writes one and a padded number would reach a different route than the subject displays.
const SQUASH_PULL_NUMBER = /\(#([1-9][0-9]*)\)$/;

interface ContextOptions {
  owner: string;
  name: string;
  checkout: string;
  sha: string;
  tag: string;
  baseline: string;
  out: string;
}

/** Writes the release notes context. The baseline file owns the changelog; an empty one falls back to the squash subjects. */
export const run: Run = async (argv, io, env) => {
  const parsed = parseArgs(argv, FLAGS);
  if (!parsed.ok) {
    io.stderr.write(`${parsed.message}\n`);
    return report(io, PREFIX, REQUIRED, EXIT_USAGE);
  }
  const { repo, checkout, sha, tag, baseline, out } = parsed.values;
  if (!repo || !checkout || !sha || !tag || !baseline || !out)
    return report(io, PREFIX, REQUIRED, EXIT_USAGE);

  const split = splitRepo(repo);
  if (split === undefined)
    return report(
      io,
      PREFIX,
      `--repo must be OWNER/REPO, not ${repo}`,
      EXIT_USAGE,
    );
  if (!FULL_SHA.test(sha)) {
    return report(
      io,
      PREFIX,
      `--sha must be 40 lowercase hexadecimal characters, not ${sha}`,
      EXIT_USAGE,
    );
  }
  if (!STABLE_TAG.test(tag))
    return report(
      io,
      PREFIX,
      `--tag must be vMAJOR.MINOR.PATCH, not ${tag}`,
      EXIT_USAGE,
    );

  try {
    await writeContext(
      {
        owner: split.owner,
        name: split.name,
        checkout,
        sha,
        tag,
        baseline,
        out,
      },
      io,
      env,
    );
  } catch (error) {
    return report(
      io,
      PREFIX,
      error instanceof Error ? error.message : String(error),
      EXIT_FAILURE,
    );
  }
  return EXIT_OK;
};

// The output directory is created here. The baseline file is read first, so a missing or unreadable one fails
// before any API call and before the output directory exists.
async function writeContext(
  options: ContextOptions,
  io: Io,
  env: Env,
): Promise<void> {
  const text = readFile(options.baseline);
  try {
    mkdirSync(options.out, { recursive: true, mode: 0o755 });
  } catch (error) {
    throw fileErrorOf(error, options.out);
  }
  const client = GithubClient.fromEnv(env);

  const previous = await previousStableRelease(client, options);
  await rejectStrayPlaceholderTags(client, options);

  const subjects = await rangeSubjects(options, previous);
  let end = text.length;
  while (end > 0 && (text[end - 1] === 0x0a || text[end - 1] === 0x0d)) end--;
  const trimmed = text.subarray(0, end);
  // The baseline file's bytes are copied as they are; only the blank check decodes them.
  const body =
    trimmed.toString("utf8").trim() === ""
      ? Buffer.from(subjectListBaseline(subjects).replace(/\n+$/, ""), "utf8")
      : trimmed;

  const baseline = Buffer.concat([
    body,
    Buffer.from(LICENSES_PARAGRAPH, "utf8"),
  ]);
  writeFile(join(options.out, BASELINE_NOTES_NAME), baseline);

  const label = previous === "" ? "none (first release)" : previous;
  const parts: Buffer[] = [
    Buffer.from(
      `# Release context\n\nTarget: ${options.sha}\nRelease: ${options.tag}\nPrevious release: ${label}\n\n`,
      "utf8",
    ),
    baseline,
  ];
  for (const number of squashPullNumbers(subjects)) {
    try {
      parts.push(
        Buffer.from(await pullSection(client, options, number), "utf8"),
      );
    } catch {
      // One unreadable pull request must never fail a release. The baseline still lists the change.
      io.stderr.write(
        `${PREFIX}: PR ${number} context unavailable; retaining GitHub baseline evidence\n`,
      );
    }
  }
  writeFile(join(options.out, CONTEXT_NOTES_NAME), Buffer.concat(parts));
}

/**
 * Fails when any refs/tags/untagged-* exists on the remote with no release behind it. Such a ref is what a
 * placeholder release deleted without --cleanup-tag leaves behind (#620). It runs after previousStableRelease,
 * because a placeholder that still has a published release is reported there with the release name and the
 * full recovery. A clean remote answers an empty list.
 */
async function rejectStrayPlaceholderTags(
  client: GithubClient,
  options: ContextOptions,
): Promise<void> {
  let refs;
  try {
    refs = await client.listMatchingRefs(
      options.owner,
      options.name,
      `tags/${PLACEHOLDER_TAG_PREFIX}`,
    );
  } catch (error) {
    throw new Error(
      `cannot list placeholder tags: ${error instanceof Error ? error.message : String(error)}`,
      { cause: error },
    );
  }
  for (const ref of refs) {
    const name = ref.ref.replace(/^refs\/tags\//, "");
    if (isPlaceholderTag(name)) throw conflictStrayPlaceholderTag(name);
  }
}

/**
 * Returns the tag the notes range starts at, or "" when no release has been published yet. It fails when
 * releases exist and none is a readable ancestor, because the range would silently widen to the whole history
 * and repeat changes an earlier release already announced. It also fails, through publishedReleases, when any
 * published release still carries a placeholder tag (#609).
 */
async function previousStableRelease(
  client: GithubClient,
  options: ContextOptions,
): Promise<string> {
  const published = await publishedReleases(client, options);
  for (const release of published) {
    // A tag tagVersion cannot read must never win here even though its shape passes the stable pattern:
    // newerVersion already treats it as unorderable, so the sort and the candidate filter must agree.
    if (tagVersion(release.tag_name) === undefined) continue;
    if (await readableAncestor(options.checkout, release.tag_name, options.sha))
      return release.tag_name;
  }
  if (published.length > 0)
    throw new Error(
      "no published stable release is a readable ancestor of the target",
    );
  return "";
}

/**
 * Lists every release, fails on one published under a placeholder tag, drops the drafts and the tag under
 * release, and returns the rest highest version first. Pre-releases stay: a pre-release dispatch publishes
 * one that may never be promoted, so the range runs from the previous release of either kind (ADR 0010).
 */
async function publishedReleases(
  client: GithubClient,
  options: ContextOptions,
): Promise<Release[]> {
  const kept: Release[] = [];
  let page = 0;
  for (;;) {
    const listed = await client.listReleases(options.owner, options.name, page);
    for (const release of listed.items) {
      // A release published under the placeholder is the wrong-tag hazard. Under any other version it would
      // pass here as a non-stable name, be skipped by the selector, and start the range one release early
      // (#609). It fails the step wherever it sits, pre-release or not. A draft may still carry the placeholder.
      if (!release.draft && isPlaceholderTag(release.tag_name))
        throw conflictPlaceholderTag(release.name, release.tag_name);
      if (release.draft || release.tag_name === options.tag) continue;
      kept.push(release);
    }
    if (listed.nextPage === 0) break;
    page = listed.nextPage;
  }
  return kept.sort(newerVersion);
}

// The version a stable tag names, or undefined for any name tagVersion refuses.
function stableVersion(tag: string): Version | undefined {
  const numbers = tagVersion(tag);
  if (numbers === undefined) return undefined;
  const [major, minor, patch] = numbers;
  return { major, minor, patch };
}

/**
 * Orders candidates highest stable version first, and every other name after all of them in listing order.
 * Version order, not publish order: an older release can be published after a newer one (v1.104.0 went out
 * twelve minutes after v1.105.0, #609), and a range that starts at the most recently published tag then
 * repeats changes an earlier release already announced. Returns a negative number when a sorts first.
 */
export function newerVersion(
  a: Pick<Release, "tag_name">,
  b: Pick<Release, "tag_name">,
): number {
  const av = stableVersion(a.tag_name);
  const bv = stableVersion(b.tag_name);
  if (av !== undefined && bv !== undefined) {
    if (lessThan(bv, av)) return -1;
    if (lessThan(av, bv)) return 1;
    return 0;
  }
  if (av !== undefined) return -1;
  if (bv !== undefined) return 1;
  return 0;
}

/**
 * Reports whether tag names a commit in checkout that sha descends from. Both git calls fail on an ordinary
 * miss: the tag may never have been fetched, and a tag on another branch is no ancestor. Neither is an error
 * here, because the caller answers those by trying the next candidate.
 */
async function readableAncestor(
  checkout: string,
  tag: string,
  sha: string,
): Promise<boolean> {
  try {
    await git(checkout, ["rev-parse", "--verify", `${tag}^{commit}`]);
    await git(checkout, ["merge-base", "--is-ancestor", tag, sha]);
    return true;
  } catch {
    return false;
  }
}

/** The pull requests the squash subjects name, deduplicated and ascending. */
export function squashPullNumbers(subjects: readonly string[]): number[] {
  const seen = new Set<number>();
  for (const subject of subjects) {
    const match = SQUASH_PULL_NUMBER.exec(subject.trim());
    if (match === null) continue;
    const number = Number(match[1] ?? "");
    if (Number.isSafeInteger(number)) seen.add(number);
  }
  return [...seen].sort((a, b) => a - b);
}

/**
 * The baseline written when the baseline file is empty: a bullet per squash subject, because no other changelog
 * exists for that head. A release commit's own subject is not a change, so it is left out.
 */
export function subjectListBaseline(subjects: readonly string[]): string {
  let text = "## Changes\n\n";
  for (const subject of subjects) {
    if (subject.startsWith("chore(release): ")) continue;
    text += `* ${subject}\n`;
  }
  return text;
}

/** The squash subjects in previous..sha, oldest first, or every subject reachable from sha when previous is empty. */
async function rangeSubjects(
  options: ContextOptions,
  previous: string,
): Promise<string[]> {
  const range = previous !== "" ? `${previous}..${options.sha}` : options.sha;
  const out = await git(options.checkout, [
    "log",
    "--reverse",
    "--format=%s",
    range,
  ]);
  const trimmed = out.replace(/\n+$/, "");
  // An empty range has no subject; split would turn it into one empty one.
  return trimmed === "" ? [] : trimmed.split("\n");
}

/** One pull request rendered for the context file. The heading carries the number the subject held. */
async function pullSection(
  client: GithubClient,
  options: ContextOptions,
  number: number,
): Promise<string> {
  const pull = await client.getPullRequest(options.owner, options.name, number);
  return `\n## PR #${number}\n\n${pull.html_url}\n\n${pull.title}\n\n${pull.body}\n`;
}
