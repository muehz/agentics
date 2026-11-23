import { readFileSync } from "node:fs";
import { join } from "node:path";
import { expectedAssetNames } from "./assets.ts";
import {
  GithubError,
  isNotFound,
  type GitRef,
  type GithubClient,
  type Release,
} from "./github.ts";

// This module holds the repository, tag, release and remote-asset rules that stage and context apply.
// It has no command of its own. Every error message is the reason alone, without a prefix: each caller
// adds its own "release: <command>: " prefix, so one message serves both commands.

/** Stands in for a name the caller cannot know: a published release with fewer than ten assets is missing one. */
export const MISSING_ASSET_NAME = "<missing expected asset>";

/** Starts the tag_name GitHub reports for a draft whose tag does not exist. */
export const PLACEHOLDER_TAG_PREFIX = "untagged-";

/** Splits the OWNER/REPO form. Undefined unless there is exactly one slash with text on both sides. */
export function splitRepo(
  repo: string,
): { owner: string; name: string } | undefined {
  const cut = repo.indexOf("/");
  if (cut < 0) return undefined;
  const owner = repo.slice(0, cut);
  const name = repo.slice(cut + 1);
  if (owner === "" || name === "" || name.includes("/")) return undefined;
  return { owner, name };
}

/**
 * The error for a tag that already names another commit: an immutable tag cannot move, so only a new release can
 * carry this content.
 */
export function conflictTag(tag: string, sha: string): Error {
  return new Error(
    `observed tag ${tag} does not target ${sha}; publish a new patch release for immutable content`,
  );
}

/** The tag-first path's mismatch (#620): a tag elsewhere is most often what a discarded draft left behind. */
export function conflictOrphanTag(tag: string, sha: string): Error {
  return new Error(
    `observed tag ${tag} does not target ${sha}; a discarded draft leaves this behind: with the draft still present run gh release delete ${tag} --cleanup-tag --yes, with no draft run git push origin :refs/tags/${tag}, then rerun; for a published release, publish a new patch release for immutable content`,
  );
}

export function isPlaceholderTag(tagName: string): boolean {
  return tagName.startsWith(PLACEHOLDER_TAG_PREFIX);
}

/**
 * True when tag_name is the tag, or a placeholder standing in for it on a draft. A published release must carry the
 * real tag.
 */
export function tagNameMatches(release: Release, tag: string): boolean {
  if (release.tag_name === tag) return true;
  return release.draft && isPlaceholderTag(release.tag_name);
}

/**
 * True when a listed release is this tag's release. A real tag_name matches whatever the name says, so a
 * same-tag release with a wrong title reaches the identity check instead of being skipped and duplicated.
 * A placeholder tag_name matches only by name, draft or not.
 */
export function listingMatches(candidate: Release, tag: string): boolean {
  if (candidate.tag_name === tag) return true;
  return isPlaceholderTag(candidate.tag_name) && candidate.name === tag;
}

/**
 * The maintainer's way back from a release published under GitHub's placeholder. One string so the two errors that
 * print it cannot drift.
 */
export function placeholderRecovery(name: string, tagName: string): string {
  return (
    `recover with: (1) gh release delete ${tagName} --cleanup-tag --yes, which addresses the release by the placeholder tag it carries; ` +
    `(2) git push origin <target-sha>:refs/tags/${name} at the commit that release was built from; ` +
    `(3) dispatch the release workflow for that target SHA to rebuild and publish the release with its ten assets; ` +
    `(4) if it was published as a pre-release, promote it from the Releases page when it is ready`
  );
}

export function conflictPlaceholderTag(tag: string, tagName: string): Error {
  return new Error(
    `observed release ${tag} is published under placeholder tag ${tagName}; ${placeholderRecovery(tag, tagName)}`,
  );
}

/** The shorter way back from a placeholder tag with no release behind it. */
export function strayTagRecovery(tagName: string): string {
  return `recover with: git push origin :refs/tags/${tagName}, then rerun; if a published release still carries it, ${placeholderRecovery("<release name>", tagName)}`;
}

export function conflictStrayPlaceholderTag(tagName: string): Error {
  return new Error(
    `observed stray placeholder tag ${tagName} on the remote; ${strayTagRecovery(tagName)}`,
  );
}

export function conflictRelease(tag: string): Error {
  return new Error(
    `observed release ${tag} has conflicting identity; use a separately human-approved notes edit for prose-only correction or publish a new patch release for immutable content`,
  );
}

export function conflictAsset(assetName: string, tag: string): Error {
  return new Error(
    `observed asset ${assetName} on release ${tag} conflicts; publish a new patch release for immutable content`,
  );
}

export function conflictReleaseTarget(
  tag: string,
  observed: string,
  sha: string,
): Error {
  return new Error(
    `observed release ${tag} has targetCommitish ${observed}, expected ${sha}; publish a new patch release for immutable content`,
  );
}

/**
 * Throws unless the observed release is the one this run may write to. The checks run in this order:
 * a usable id, the pinned id, the title, the tag_name, the target, then the tag for a published release.
 *
 * pinned is the id an earlier read established, or 0 before one exists. tagPresent and tagSHA come from
 * the most recent tag read; a draft has no git tag, so only a published release is held to it. Neither
 * value of prerelease is a conflict: a pre-release can be promoted later (ADR 0010).
 */
export function validateReleaseIdentity(
  release: Release,
  tag: string,
  sha: string,
  pinned: number,
  tagPresent: boolean,
  tagSHA: string,
): void {
  if (release.id <= 0)
    throw new Error(`release ${tag} has no valid numeric id`);
  if (pinned !== 0 && release.id !== pinned) {
    throw new Error(
      `pinned release id ${pinned} returned release id ${release.id}; refusing a mismatched release ID`,
    );
  }
  if (release.name !== tag) throw conflictRelease(tag);
  if (!tagNameMatches(release, tag)) {
    if (!release.draft && isPlaceholderTag(release.tag_name))
      throw conflictPlaceholderTag(tag, release.tag_name);
    throw conflictRelease(tag);
  }
  if (release.target_commitish !== sha)
    throw conflictReleaseTarget(tag, release.target_commitish, sha);
  if (!release.draft && (!tagPresent || tagSHA !== sha))
    throw conflictTag(tag, sha);
}

/**
 * Throws unless the release's own asset list holds only expected names, each at most once. A published
 * release must already carry all ten; a draft is still being filled, so a short roster is normal there.
 */
export function validateRemoteRoster(release: Release, tag: string): void {
  const expected = expectedAssetNames(tag);
  const seen = new Set<string>();
  for (const asset of release.assets) {
    if (!expected.includes(asset.name) || seen.has(asset.name))
      throw conflictAsset(asset.name, tag);
    seen.add(asset.name);
  }
  if (!release.draft && release.assets.length !== expected.length)
    throw conflictAsset(MISSING_ASSET_NAME, tag);
}

/** True when the release carries exactly one asset named assetName. */
export function hasRemoteAsset(release: Release, assetName: string): boolean {
  return (
    release.assets.filter((asset) => asset.name === assetName).length === 1
  );
}

/** The id of the one asset named assetName. Throws the asset conflict for none, several, or a non-positive id. */
export function remoteAssetId(
  release: Release,
  assetName: string,
  tag: string,
): number {
  let found = 0;
  let count = 0;
  for (const asset of release.assets) {
    if (asset.name !== assetName) continue;
    count++;
    found = asset.id;
  }
  if (count !== 1 || found <= 0) throw conflictAsset(assetName, tag);
  return found;
}

// ---------------------------------------------------------------------------
// Network half: the reads and writes stage and context make against the remote.
// ---------------------------------------------------------------------------

/**
 * True for GitHub's 422 for a ref that exists. The create-ref endpoint answers 422 for validation failures
 * too, so the status alone is not enough: the message or an error code must say the reference exists.
 * The type decides, never the text of a transport error.
 */
export function alreadyExists(error: unknown): boolean {
  if (!(error instanceof GithubError) || error.status !== 422) return false;
  if (error.apiMessage.toLowerCase().includes("already exists")) return true;
  return error.details.some((detail) => detail.code === "already_exists");
}

/**
 * Returns the commit refs/tags/<tag> names, and whether the tag exists at all. An annotated tag is peeled
 * through its tag object, because the ref then names the tag object rather than the commit. A 404 is
 * absence; every other failure is fatal, so an unreadable tag is never read as "no tag".
 */
export async function tagTarget(
  client: GithubClient,
  owner: string,
  name: string,
  tag: string,
): Promise<{ sha: string; present: boolean }> {
  let ref: GitRef;
  try {
    ref = await client.getRef(owner, name, `tags/${tag}`);
  } catch (error) {
    if (isNotFound(error)) return { sha: "", present: false };
    throw error;
  }
  if (ref.object.type !== "tag") return { sha: ref.object.sha, present: true };
  try {
    const annotated = await client.getTag(owner, name, ref.object.sha);
    return { sha: annotated.object.sha, present: true };
  } catch {
    throw new Error(`cannot peel observed tag ${tag}`);
  }
}

/** Reads one release by its pinned id. Undefined only for a 404; every other error is fatal. */
export async function releaseByID(
  client: GithubClient,
  owner: string,
  name: string,
  id: number,
): Promise<Release | undefined> {
  try {
    return await client.getRelease(owner, name, id);
  } catch (error) {
    if (isNotFound(error)) return undefined;
    throw error;
  }
}

/**
 * Returns the release for tag, or undefined when none exists. The by-tag endpoint answers with published
 * releases only, so a draft is found by paging the whole list. A failure of that listing is fatal with no
 * fallback: a fallback would create a second release beside a draft the read failed to show.
 */
export async function discoverRelease(
  client: GithubClient,
  owner: string,
  name: string,
  tag: string,
): Promise<Release | undefined> {
  try {
    return await client.getReleaseByTag(owner, name, tag);
  } catch (error) {
    if (!isNotFound(error)) throw error;
  }

  const matches: Release[] = [];
  let page = 0;
  for (;;) {
    const listed = await client.listReleases(owner, name, page);
    for (const candidate of listed.items) {
      if (listingMatches(candidate, tag)) matches.push(candidate);
    }
    if (listed.nextPage === 0) break;
    page = listed.nextPage;
  }

  const [only] = matches;
  if (only === undefined) return undefined;
  if (matches.length > 1)
    throw new Error(
      `multiple releases match ${tag}; refusing ambiguous draft recovery`,
    );
  if (only.id <= 0) throw new Error(`observed release ${tag} has no valid id`);

  // A listing entry can lag the release itself, so reread by the id it pins rather than trust the listed fields.
  const found = await releaseByID(client, owner, name, only.id);
  if (found === undefined) {
    // The listing just observed this id, so a 404 here is a disappearance and not an absence. Answering
    // "no release" would send the caller into createDraft and build a second release beside the listed one.
    throw new Error(
      `release ${tag} (id ${only.id}) was listed but could not be reread`,
    );
  }
  return found;
}

type Target = Readonly<{
  owner: string;
  name: string;
  tag: string;
  sha: string;
}>;
type TagReading = Readonly<{ sha: string; present: boolean }>;

export interface ReleaseReadRetry {
  retries: number;
  wait(id: number, attempt: number): Promise<void>;
}

/** The release and tag readings used to check one pinned id against its expected repository and target. */
export class PinnedRelease {
  private readonly client: GithubClient;
  private readonly target: Target;
  readonly id: number;
  readonly current: Release;
  readonly tagReading: TagReading;

  private constructor(
    client: GithubClient,
    target: Target,
    id: number,
    current: Release,
    tagReading: TagReading,
  ) {
    this.client = client;
    this.target = target;
    this.id = id;
    this.current = current;
    this.tagReading = tagReading;
  }

  static async discover(client: GithubClient, target: Target) {
    const { owner, name, tag, sha } = target;
    const tagReading = await tagTarget(client, owner, name, tag);
    if (tagReading.present && tagReading.sha !== sha)
      throw conflictOrphanTag(tag, sha);
    const release = await discoverRelease(client, owner, name, tag);
    const found =
      release &&
      new PinnedRelease(client, target, release.id, release, tagReading);
    found?.validateForAssets();
    return { found, tagReading };
  }

  static fromCreate(
    client: GithubClient,
    target: Target,
    response: Release,
    tagReading: TagReading,
  ): PinnedRelease {
    validateReleaseIdentity(
      response,
      target.tag,
      target.sha,
      0,
      tagReading.present,
      tagReading.sha,
    );
    if (!response.draft) throw conflictRelease(target.tag);
    return new PinnedRelease(client, target, response.id, response, tagReading);
  }

  withTag(tagReading: TagReading): PinnedRelease {
    return this.observed(this.current, tagReading);
  }

  private observed(release: Release, tagReading: TagReading): PinnedRelease {
    return new PinnedRelease(
      this.client,
      this.target,
      this.id,
      release,
      tagReading,
    );
  }

  private read() {
    return releaseByID(
      this.client,
      this.target.owner,
      this.target.name,
      this.id,
    );
  }

  private readTag() {
    return tagTarget(
      this.client,
      this.target.owner,
      this.target.name,
      this.target.tag,
    );
  }

  private async readAfterWrite(retry: ReleaseReadRetry): Promise<Release> {
    for (let attempt = 0; attempt <= retry.retries; attempt++) {
      const release = await this.read();
      if (release !== undefined) return release;
      if (attempt < retry.retries) await retry.wait(this.id, attempt + 1);
    }
    throw new Error(
      `release ${this.target.tag} (id ${this.id}) was not readable after create`,
    );
  }

  private validateIdentity(): void {
    validateReleaseIdentity(
      this.current,
      this.target.tag,
      this.target.sha,
      this.id,
      this.tagReading.present,
      this.tagReading.sha,
    );
  }

  private validateForAssets(): void {
    this.validateIdentity();
    validateRemoteRoster(this.current, this.target.tag);
  }

  async rereadCreated(
    retry: ReleaseReadRetry,
    reportCreated: (id: number) => void,
  ): Promise<PinnedRelease> {
    const release = await this.readAfterWrite(retry);
    if (!release.draft) throw conflictRelease(this.target.tag);
    const tagReading = await this.readTag();
    if (!tagReading.present || tagReading.sha !== this.target.sha)
      throw conflictTag(this.target.tag, this.target.sha);
    const next = this.observed(release, tagReading);
    // The creation is reported once the draft and tag are present, even if its identity or roster conflicts.
    reportCreated(this.id);
    next.validateForAssets();
    return next;
  }

  async rereadAfterUpload(): Promise<PinnedRelease> {
    const release = await this.read();
    if (release === undefined)
      throw new Error(
        `release ${this.target.tag} disappeared after asset upload`,
      );
    const next = this.observed(release, this.tagReading);
    next.validateForAssets();
    return next;
  }

  async rereadPublished(
    retry: ReleaseReadRetry,
    prerelease: boolean,
  ): Promise<PinnedRelease> {
    const release = await this.readAfterWrite(retry);
    const next = this.observed(release, await this.readTag());
    next.validateIdentity();
    const { tag } = this.target;
    if (
      release.draft ||
      release.prerelease !== prerelease ||
      release.tag_name !== tag
    ) {
      const kind = prerelease ? "pre-release" : "release";
      throw new Error(
        `release ${tag} is not a published ${kind} under its tag after publication (draft=${release.draft} prerelease=${release.prerelease} tag_name=${release.tag_name})`,
      );
    }
    validateRemoteRoster(release, tag);
    return next;
  }
}

/** Compares the published bytes of one asset with the local file of the same name. */
export async function verifyRemoteAsset(
  client: GithubClient,
  owner: string,
  name: string,
  release: Release,
  dir: string,
  tag: string,
  assetName: string,
): Promise<void> {
  const id = remoteAssetId(release, assetName, tag);
  const published = await client.downloadReleaseAsset(owner, name, id);
  const local = readFileSync(join(dir, assetName));
  if (!local.equals(published)) throw conflictAsset(assetName, tag);
}

/** Compares every one of the ten published assets with its local file, in roster order. */
export async function verifyAllAssets(
  client: GithubClient,
  owner: string,
  name: string,
  release: Release,
  dir: string,
  tag: string,
): Promise<void> {
  for (const assetName of expectedAssetNames(tag)) {
    if (!hasRemoteAsset(release, assetName))
      throw conflictAsset(assetName, tag);
    await verifyRemoteAsset(client, owner, name, release, dir, tag, assetName);
  }
}

/**
 * Adds one missing asset. It never replaces an existing one: the caller reaches this only when the roster
 * carries no asset of that name.
 */
export async function uploadAsset(
  client: GithubClient,
  owner: string,
  name: string,
  id: number,
  dir: string,
  assetName: string,
): Promise<void> {
  const contents = readFileSync(join(dir, assetName));
  await client.uploadReleaseAsset(owner, name, id, assetName, contents);
}
