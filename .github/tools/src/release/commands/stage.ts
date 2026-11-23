import { readFileSync } from "node:fs";
import { join } from "node:path";
import { parseArgs } from "../args.ts";
import {
  expectedAssetNames,
  fileDigest,
  tarballName,
  verifyChecksums,
  verifyLocalAssets,
} from "../assets.ts";
import {
  GithubClient,
  isNotFound,
  describeExit,
  runCommand,
  succeeded,
  writeOutputs,
  type Release,
} from "../github.ts";
import {
  MISSING_ASSET_NAME,
  PinnedRelease,
  type ReleaseReadRetry,
  alreadyExists,
  conflictAsset,
  conflictOrphanTag,
  hasRemoteAsset,
  splitRepo,
  tagTarget,
  uploadAsset,
  verifyAllAssets,
  verifyRemoteAsset,
} from "../identity.ts";
import { FULL_SHA, STABLE_TAG } from "../version.ts";
import {
  EXIT_FAILURE,
  EXIT_OK,
  EXIT_USAGE,
  report,
  type Run,
  type Writer,
} from "../io.ts";

const PREFIX = "release: stage";
const REQUIRED = "--repo, --sha, --tag, --assets, and --notes are required";
// --release-id is intentionally not a flag: the composite action passes no such input, and the action test holds
// the action's inputs and this command's flags to the same set.
const FLAGS = ["repo", "sha", "tag", "assets", "notes", "prerelease"];

/** A rerun skips provenance only when one verified statement covers the complete rebuilt roster. */
export async function hasReleaseAttestation(
  repo: string,
  assets: string,
  tag: string,
): Promise<boolean> {
  const source = join(assets, tarballName(tag));
  const digest = `sha256:${fileDigest(source)}`;
  const listing = await runCommand("gh", [
    "api",
    "--paginate",
    "--slurp",
    `repos/${repo}/attestations/${digest}?predicate_type=provenance`,
  ]);
  if (!succeeded(listing)) {
    if (
      listing.status === 1 &&
      /^gh: [^\r\n]+ \(HTTP 404\)\r?\n?$/.test(listing.stderr.toString("utf8"))
    ) {
      // gh --slurp includes the error body as a page. A preceding successful page
      // means pagination failed; it cannot establish absence for the whole listing.
      const pages: unknown = JSON.parse(listing.stdout.toString("utf8"));
      if (
        Array.isArray(pages) &&
        pages.length === 1 &&
        typeof pages[0]?.message === "string" &&
        !("attestations" in pages[0])
      )
        return false;
    }
    throw new Error(
      `cannot list attestations for ${digest}: ${describeExit(listing)}`,
    );
  }
  const pages: unknown = JSON.parse(listing.stdout.toString("utf8"));
  if (!Array.isArray(pages) || pages.length === 0)
    throw new Error("missing attestation response pages");
  let count = 0;
  for (const page of pages) {
    if (
      !Array.isArray(page?.attestations) ||
      page.attestations.some(
        (item: unknown) =>
          item === null || typeof item !== "object" || Array.isArray(item),
      )
    )
      throw new Error("invalid attestation listing");
    count += page.attestations.length;
  }
  if (count === 0) return false;
  const verified = await runCommand("gh", [
    "attestation",
    "verify",
    source,
    "--repo",
    repo,
    "--signer-workflow",
    `${repo}/.github/workflows/release.lock.yml`,
    "--source-ref",
    "refs/heads/main",
    "--limit",
    String(count),
    "--format",
    "json",
  ]);
  if (!succeeded(verified))
    throw new Error(
      `cannot verify attestations for ${digest}: ${describeExit(verified)}`,
    );
  const records: unknown = JSON.parse(verified.stdout.toString("utf8"));
  if (!Array.isArray(records) || records.length !== count)
    throw new Error("incomplete attestation verification results");
  const expected = new Map(
    expectedAssetNames(tag).map((name) => [
      name,
      fileDigest(join(assets, name)),
    ]),
  );
  let present = false;
  for (const record of records) {
    const subjects = record?.verificationResult?.statement?.subject;
    if (!Array.isArray(subjects) || subjects.length === 0)
      throw new Error("missing attestation subjects");
    const seen = new Set<string>();
    let matches = subjects.length === expected.size;
    for (const subject of subjects) {
      const name = subject?.name;
      const digest = subject?.digest?.sha256;
      if (
        typeof name !== "string" ||
        name === "" ||
        seen.has(name) ||
        typeof digest !== "string" ||
        !/^[0-9a-f]{64}$/.test(digest)
      )
        throw new Error("invalid attestation subject name or SHA-256");
      seen.add(name);
      if (expected.get(name) !== digest) matches = false;
    }
    if (matches) present = true;
  }
  return present;
}

/** The wait between the reads that follow a create. GitHub can answer 404 for a release it has just created (v1.95.0, v1.96.0). */
export const DEFAULT_RETRY_DELAY_MS = 2000;
/** The retries after the first read. Four reads in all, retryDelay apart, cover six seconds of lag. */
const REREAD_RETRIES = 3;

/** One validated invocation. */
export interface StageOptions {
  owner: string;
  name: string;
  sha: string;
  tag: string;
  assets: string;
  /** The draft body, sent once in the create request. An existing release keeps the body a human approved. */
  body: string;
  /** True publishes a non-latest pre-release, false a real release that is marked latest. */
  prerelease: boolean;
}

/** Names the publication state a run asks for, in messages. */
function kindOf(options: StageOptions): string {
  return options.prerelease ? "pre-release" : "release";
}

/** The make_latest value the publishing PATCH sends. GitHub never echoes it; a pre-release cannot be latest. */
function makeLatestOf(options: StageOptions): "true" | "false" {
  return options.prerelease ? "false" : "true";
}

function describe(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

function sleep(milliseconds: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, milliseconds));
}

/**
 * Creates or recovers the release and publishes it: a real, latest release by default, or a non-latest
 * pre-release for --prerelease true. A release that is already published is verified and left unchanged
 * (ADR 0010). It sends one PATCH, the publication, which never carries a body.
 */
export const run: Run = async (argv, io, env, runOptions) => {
  const parsed = parseArgs(argv, FLAGS);
  if (!parsed.ok) {
    io.stderr.write(`${parsed.message}\n`);
    return report(io, PREFIX, REQUIRED, EXIT_USAGE);
  }
  const { repo, sha, tag, assets, notes } = parsed.values;
  // An empty --prerelease is a value, not a missing one: only the other five count the empty string as absent.
  const prerelease = parsed.values.prerelease ?? "false";
  if (!repo || !sha || !tag || !assets || !notes)
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
  if (prerelease !== "true" && prerelease !== "false") {
    return report(
      io,
      PREFIX,
      `--prerelease must be true or false, not ${prerelease}`,
      EXIT_USAGE,
    );
  }
  let bytes: Buffer;
  try {
    bytes = readFileSync(notes);
  } catch {
    return report(
      io,
      PREFIX,
      `notes file does not exist: ${notes}`,
      EXIT_USAGE,
    );
  }
  let body: string;
  try {
    body = new TextDecoder("utf-8", { fatal: true, ignoreBOM: true }).decode(
      bytes,
    );
  } catch {
    return report(
      io,
      PREFIX,
      `notes file is not valid UTF-8: ${notes}`,
      EXIT_USAGE,
    );
  }

  // The bundle is verified before the first API call, so a broken bundle never reaches GitHub and never
  // creates a release it cannot fill.
  try {
    verifyLocalAssets(assets, tag);
    verifyChecksums(assets, tag);
  } catch (error) {
    return report(io, PREFIX, describe(error), EXIT_FAILURE);
  }

  const options: StageOptions = {
    owner: split.owner,
    name: split.name,
    sha,
    tag,
    assets,
    body,
    prerelease: prerelease === "true",
  };
  const retryDelayMs = runOptions?.retryDelayMs ?? DEFAULT_RETRY_DELAY_MS;
  try {
    const client = GithubClient.fromEnv(env);
    const progress = await stagePinnedRelease(
      client,
      options,
      io.stderr,
      retryDelayMs,
    );
    writeOutputs(
      {
        release_id: String(progress.id),
        published: String(progress.state === "published"),
      },
      env,
    );
  } catch (error) {
    return report(io, PREFIX, describe(error), EXIT_FAILURE);
  }
  return EXIT_OK;
};

/** Names the state a published release is in. */
function publishedKind(release: Release): string {
  return release.prerelease ? "pre-release" : "release";
}

async function requestDraft(
  client: GithubClient,
  options: StageOptions,
): Promise<Release> {
  // A draft starts as a pre-release so it cannot become latest while incomplete.
  return client.createRelease(options.owner, options.name, {
    tag_name: options.tag,
    target_commitish: options.sha,
    name: options.tag,
    body: options.body,
    draft: true,
    prerelease: true,
  });
}

async function requestPublication(
  client: GithubClient,
  options: StageOptions,
  pinned: number,
): Promise<void> {
  const { owner, name, tag, sha } = options;
  try {
    await client.updateRelease(owner, name, pinned, {
      tag_name: tag,
      target_commitish: sha,
      draft: false,
      prerelease: options.prerelease,
      make_latest: makeLatestOf(options),
    });
  } catch (error) {
    throw new Error(
      `cannot publish ${tag} as a ${kindOf(options)}: ${describe(error)}`,
      { cause: error },
    );
  }
}

async function verifyExistingPublished(
  client: GithubClient,
  options: StageOptions,
  release: Release,
  stderr: Writer,
): Promise<void> {
  const { owner, name, tag } = options;
  const pinned = release.id;
  stderr.write(
    `${PREFIX}: found published ${publishedKind(release)} ${pinned} for ${tag}\n`,
  );
  await verifyAllAssets(client, owner, name, release, options.assets, tag);
  const found = publishedKind(release);
  if (found !== kindOf(options)) {
    stderr.write(
      `${PREFIX}: ${tag} is already published as a ${found}, so the requested ${kindOf(options)} was not applied\n`,
    );
  }
}

function retryAfterWrite(stderr: Writer, delayMs: number): ReleaseReadRetry {
  return {
    retries: REREAD_RETRIES,
    wait: async (id, attempt) => {
      stderr.write(
        `${PREFIX}: draft ${id} not readable yet, attempt ${attempt} of ${REREAD_RETRIES}\n`,
      );
      await sleep(delayMs);
    },
  };
}

type PreparedRelease =
  | { state: "published"; id: number }
  | { state: "draft"; id: number; observation: PinnedRelease };

export async function prepareDraft(
  client: GithubClient,
  options: StageOptions,
  stderr: Writer,
  retryDelayMs: number,
): Promise<PreparedRelease> {
  const { tag } = options;
  const { found, tagReading } = await PinnedRelease.discover(client, options);
  if (found && !found.current.draft) {
    await verifyExistingPublished(client, options, found.current, stderr);
    return { state: "published", id: found.id };
  }

  const ensured = await ensureTag(
    client,
    options,
    tagReading.sha,
    tagReading.present,
    stderr,
  );
  if (found) {
    stderr.write(`${PREFIX}: adopted draft ${found.id} for ${tag}\n`);
    return {
      state: "draft",
      id: found.id,
      observation: found.withTag(ensured),
    };
  }

  const response = await requestDraft(client, options);
  const created = PinnedRelease.fromCreate(client, options, response, ensured);
  const observation = await created.rereadCreated(
    retryAfterWrite(stderr, retryDelayMs),
    (id) => stderr.write(`${PREFIX}: created draft ${id} for ${tag}\n`),
  );
  return { state: "draft", id: observation.id, observation };
}

async function stagePinnedRelease(
  client: GithubClient,
  options: StageOptions,
  stderr: Writer,
  retryDelayMs: number,
): Promise<{ state: "draft" | "published"; id: number }> {
  const progress = await prepareDraft(client, options, stderr, retryDelayMs);
  if (progress.state === "published") return progress;
  const observation = await fillDraft(client, options, progress.observation);
  await requestPublication(client, options, observation.id);
  const published = await observation.rereadPublished(
    retryAfterWrite(stderr, retryDelayMs),
    options.prerelease,
  );
  await verifyAllAssets(
    client,
    options.owner,
    options.name,
    published.current,
    options.assets,
    options.tag,
  );
  if (!options.prerelease)
    await confirmLatest(client, options, published.id, stderr, retryDelayMs);
  stderr.write(
    `${PREFIX}: published ${kindOf(options)} ${published.id} for ${options.tag}\n`,
  );
  return progress;
}

/**
 * Makes refs/tags/<tag> exist at the target before the first draft write, so GitHub binds the draft to the
 * real tag instead of reporting an untagged-<hash> placeholder that a manual publish can then mint (#620).
 * The caller has already read the tag once and rejected a tag elsewhere; present at the target is the
 * same-SHA rerun and is left alone. A create that loses to a concurrent run answers 422; the reread decides.
 */
async function ensureTag(
  client: GithubClient,
  options: StageOptions,
  sha: string,
  present: boolean,
  stderr: Writer,
): Promise<{ sha: string; present: boolean }> {
  if (present) return { sha, present: true };
  let created = false;
  try {
    await client.createRef(
      options.owner,
      options.name,
      `refs/tags/${options.tag}`,
      options.sha,
    );
    created = true;
  } catch (error) {
    if (!alreadyExists(error)) {
      throw new Error(
        `cannot create tag ${options.tag} at ${options.sha}: ${describe(error)}`,
        { cause: error },
      );
    }
  }
  if (created)
    stderr.write(`${PREFIX}: created tag ${options.tag} at ${options.sha}\n`);
  const reread = await tagTarget(
    client,
    options.owner,
    options.name,
    options.tag,
  );
  if (!reread.present || reread.sha !== options.sha)
    throw conflictOrphanTag(options.tag, options.sha);
  return { sha: reread.sha, present: true };
}

/** Uploads missing assets, revalidates the release after each upload, and verifies the final roster by bytes. */
async function fillDraft(
  client: GithubClient,
  options: StageOptions,
  initial: PinnedRelease,
): Promise<PinnedRelease> {
  const { owner, name, tag } = options;
  let observation = initial;
  for (const assetName of expectedAssetNames(tag)) {
    if (hasRemoteAsset(observation.current, assetName)) {
      await verifyRemoteAsset(
        client,
        owner,
        name,
        observation.current,
        options.assets,
        tag,
        assetName,
      );
      continue;
    }
    await uploadAsset(
      client,
      owner,
      name,
      observation.id,
      options.assets,
      assetName,
    );
    observation = await observation.rereadAfterUpload();
    if (!hasRemoteAsset(observation.current, assetName))
      throw conflictAsset(assetName, tag);
    await verifyRemoteAsset(
      client,
      owner,
      name,
      observation.current,
      options.assets,
      tag,
      assetName,
    );
  }

  observation = await observation.rereadAfterUpload();
  if (observation.current.assets.length !== expectedAssetNames(tag).length)
    throw conflictAsset(MISSING_ASSET_NAME, tag);
  await verifyAllAssets(
    client,
    owner,
    name,
    observation.current,
    options.assets,
    tag,
  );
  return observation;
}

/**
 * Reads releases/latest until it names the release this run just published. GitHub never echoes make_latest,
 * so this read is how a real release is known to be latest. A 404 (no non-pre-release exists yet, as on the
 * first real release) and a read that names another release (a latest that has not caught up) each use a
 * read, with the same budget as the reads after a release write. Any other error is fatal.
 */
async function confirmLatest(
  client: GithubClient,
  options: StageOptions,
  pinned: number,
  stderr: Writer,
  retryDelayMs: number,
): Promise<void> {
  for (let attempt = 0; attempt <= REREAD_RETRIES; attempt++) {
    let latest: Release | undefined;
    try {
      latest = await client.getLatestRelease(options.owner, options.name);
    } catch (error) {
      if (!isNotFound(error)) throw error;
    }
    if (latest !== undefined && latest.id === pinned) return;
    if (attempt === REREAD_RETRIES) break;
    stderr.write(
      `${PREFIX}: latest release is not ${pinned} yet, attempt ${attempt + 1} of ${REREAD_RETRIES}\n`,
    );
    await sleep(retryDelayMs);
  }
  throw new Error(
    `release ${options.tag} (id ${pinned}) was not the latest release after publication`,
  );
}
