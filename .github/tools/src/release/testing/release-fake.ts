import { createHash } from "node:crypto";
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import type { IncomingMessage, ServerResponse } from "node:http";
import { join } from "node:path";
import { checksumAssetNames, expectedAssetNames } from "../assets.ts";
import { GithubClient } from "../github.ts";
import { startServer } from "./support.ts";

// A stateful stand-in for the GitHub release API of one repository. Set the plan fields before the command under
// test runs and read the state after it returns.

/** The repository every stage and context test names. */
export const STAGE_REPO = "muehz/agentics";
/** The tag of the bundle the stage tests verify. */
export const STAGE_TAG = "v1.2.3";
/** The target commit of the stage fixtures. */
export const STAGE_SHA = "1".repeat(40);
/** A valid target that is not the fixture's. */
export const OTHER_SHA = "a".repeat(40);
/** The second handler an asset redirect points at. It is not a GitHub route: github.com redirects to another host. */
export const ASSET_BYTES_ROUTE = "/release-asset-bytes/";
/**
 * The body a scripted read status answers with. It says 404 on purpose: a 500 whose body mentions 404 must stay
 * fatal, so the tests that script a 500 need a body that says 404 for that rule to be under test at all.
 */
export const SCRIPTED_READ_MESSAGE =
  "scripted read status; this body says 404 on purpose";

export interface FakeAsset {
  id: number;
  name: string;
  contents: Buffer;
}

export interface FakeRelease {
  id: number;
  tagName: string;
  /** Set by the fake when placeholderDraftTags is on: a read of this release while it is a draft reports the placeholder. */
  placeholder: boolean;
  name: string;
  body: string;
  draft: boolean;
  prerelease: boolean;
  targetCommitish: string;
  assets: FakeAsset[];
}

/** The object refs/tags/<tag> points at: type "commit" for a lightweight tag, "tag" for an annotated one. */
export interface FakeTagObject {
  type: string;
  sha: string;
}

export interface SeenRequest {
  method: string;
  path: string;
  query: string;
}

/** Builds a release with empty defaults and no id assignment. */
export function newFakeRelease(
  fields: Partial<Omit<FakeRelease, "placeholder">> = {},
): FakeRelease {
  return {
    id: 0,
    tagName: "",
    placeholder: false,
    name: "",
    body: "",
    draft: false,
    prerelease: false,
    targetCommitish: "",
    assets: [],
    ...fields,
  };
}

/** The tag_name github.com reports for a draft whose tag does not exist. */
export function placeholderTagFor(release: FakeRelease): string {
  return `untagged-${release.id.toString(16)}`;
}

/** The tag_name a read reports: the placeholder for a draft under the placeholder model, the stored value otherwise. */
export function wireTagName(release: FakeRelease): string {
  return release.draft && release.placeholder
    ? placeholderTagFor(release)
    : release.tagName;
}

function assetWire(asset: FakeAsset): Record<string, unknown> {
  return {
    id: asset.id,
    name: asset.name,
    size: asset.contents.length,
    state: "uploaded",
  };
}

function releaseWire(release: FakeRelease): Record<string, unknown> {
  return {
    id: release.id,
    tag_name: wireTagName(release),
    name: release.name,
    body: release.body,
    draft: release.draft,
    prerelease: release.prerelease,
    target_commitish: release.targetCommitish,
    assets: release.assets.map(assetWire),
  };
}

function sendJson(res: ServerResponse, status: number, value: unknown): void {
  res.writeHead(status, { "Content-Type": "application/json" });
  res.end(JSON.stringify(value));
}

function sendError(res: ServerResponse, status: number, message: string): void {
  sendJson(res, status, { message });
}

function sendBytes(res: ServerResponse, contents: Buffer): void {
  res.writeHead(200, { "Content-Type": "application/octet-stream" });
  res.end(contents);
}

function decodeBody(
  body: Buffer,
  res: ServerResponse,
): Record<string, unknown> | undefined {
  try {
    const value: unknown = JSON.parse(body.toString("utf8"));
    if (typeof value === "object" && value !== null && !Array.isArray(value))
      return value as Record<string, unknown>;
  } catch {
    // fall through to the 400 below
  }
  sendError(res, 400, "undecodable body");
  return undefined;
}

const stringField = (fields: Record<string, unknown>, key: string): string => {
  const value = fields[key];
  return typeof value === "string" ? value : "";
};

const boolField = (fields: Record<string, unknown>, key: string): boolean =>
  fields[key] === true;

export class ReleaseFake {
  readonly repo: string;

  /** The whole repository state. */
  releases: FakeRelease[] = [];

  /** What refs/tags/<tag> points at, or null when no tag exists. */
  tagObject: FakeTagObject | null = null;

  /** Maps a tag object SHA to the commit that tag peels to. An absent SHA answers 404: the unpeelable tag. */
  annotatedTags = new Map<string, string>();

  /** The body of every POST git/refs. tagCreateStatus, when non-zero, is the status the create answers instead of 201. */
  tagCreated: Record<string, unknown>[] = [];
  tagCreateStatus = 0;
  /** The message a scripted tagCreateStatus answers with; empty means "Reference already exists". */
  tagCreateMessage = "";

  /** Runs after each GET git/ref/tags read has captured its answer, with the 1-based count of reads so far. */
  tagReadHook: ((fake: ReleaseFake, reads: number) => void) | undefined;
  tagReads = 0;

  /** The untagged-* tag names the matching-refs route lists. Empty is a clean remote. */
  placeholderRefs: string[] = [];

  /** The HTTP status of each read by release id, in order. Answers normally once empty, so [404, 404] makes the third read succeed. */
  readByIdCodes: number[] = [];

  /** When non-zero, the release the read-by-id route answers with, whatever id the request named. */
  readByIdReturns = 0;

  /**
   * Scripts each read of releases/latest, in order: a positive value serves that release id, zero answers 404,
   * a negative value answers 500. Once empty the route serves the release the last publishing PATCH made latest.
   */
  latestScript: number[] = [];
  latestId = 0;
  /** The time of each read of releases/latest, in milliseconds, so a test can check the wait between reads. */
  latestReadTimes: number[] = [];

  /** When non-zero, the status the release listing (listStatus), the tag ref route (tagRefStatus) or a PATCH (patchStatus) answers with. */
  listStatus = 0;
  tagRefStatus = 0;
  patchStatus = 0;

  /** Merged over the create response before it is written; the stored release is unchanged. */
  createReturns: Record<string, unknown> = {};

  /** Makes the asset route answer 302 to a second route that serves the bytes, as github.com does. */
  redirectAssets = false;

  /** The decoded body of every create request and of every PATCH request. */
  created: Record<string, unknown>[] = [];
  patched: Record<string, unknown>[] = [];

  /** Run after each accepted create, upload and PATCH, with the change already applied. */
  afterCreate: ((fake: ReleaseFake) => void) | undefined;
  afterUpload: ((fake: ReleaseFake) => void) | undefined;
  afterPatch: ((fake: ReleaseFake) => void) | undefined;

  /**
   * Models github.com: every read of a draft reports tag_name as untagged-<hex id> while the stored value keeps
   * what the create asked for. A promotion PATCH that carries tag_name sets the stored value; one that omits it
   * publishes under the placeholder, which is the hazard the production code guards against.
   */
  placeholderDraftTags = false;

  nextReleaseId = 100;
  nextAssetId = 1000;

  constructor(repo: string) {
    this.repo = repo;
  }

  /** Appends one release and returns it. It assigns an id when the caller left it zero. */
  addRelease(
    fields: Partial<Omit<FakeRelease, "placeholder">> = {},
  ): FakeRelease {
    const release = newFakeRelease(fields);
    if (release.id === 0) release.id = this.nextReleaseId++;
    release.placeholder = this.placeholderDraftTags;
    this.releases.push(release);
    return release;
  }

  /** Publishes one asset on release with the bytes of dir/name, which is what a completed upload leaves behind. */
  addAsset(release: FakeRelease, dir: string, name: string): FakeAsset {
    const asset = {
      id: this.nextAssetId++,
      name,
      contents: readFileSync(join(dir, name)),
    };
    release.assets.push(asset);
    return asset;
  }

  /** Publishes all nine assets for tag out of dir. */
  addAllAssets(release: FakeRelease, dir: string, tag: string): void {
    for (const name of expectedAssetNames(tag))
      this.addAsset(release, dir, name);
  }

  release(id: number): FakeRelease | undefined {
    return this.releases.find((candidate) => candidate.id === id);
  }

  private findAsset(id: number): FakeAsset | undefined {
    for (const release of this.releases) {
      const asset = release.assets.find((candidate) => candidate.id === id);
      if (asset !== undefined) return asset;
    }
    return undefined;
  }

  /** The handler to give to startServer. */
  handle = (req: IncomingMessage, body: Buffer, res: ServerResponse): void => {
    const url = new URL(req.url ?? "/", "http://fake.invalid");
    const method = req.method ?? "GET";
    const path = url.pathname;
    const prefix = `/repos/${this.repo}`;

    if (method === "GET" && path.startsWith(ASSET_BYTES_ROUTE)) {
      const asset = this.findAsset(
        Number(path.slice(ASSET_BYTES_ROUTE.length)),
      );
      if (asset === undefined) sendError(res, 404, "Not Found");
      else sendBytes(res, asset.contents);
      return;
    }
    if (!path.startsWith(`${prefix}/`)) {
      sendError(res, 404, "Not Found");
      return;
    }
    const rest = path.slice(prefix.length);
    let match: RegExpExecArray | null;

    if (method === "GET" && (match = /^\/git\/ref\/tags\/([^/]+)$/.exec(rest)))
      return this.serveTagRef(res, decodeURIComponent(match[1] ?? ""));
    if (method === "GET" && (match = /^\/git\/tags\/([^/]+)$/.exec(rest)))
      return this.serveAnnotatedTag(res, decodeURIComponent(match[1] ?? ""));
    if (method === "POST" && rest === "/git/refs")
      return this.serveCreateRef(res, body);
    if (method === "GET" && rest === "/git/matching-refs/tags/untagged-")
      return this.serveMatchingPlaceholderRefs(res);
    if (method === "GET" && rest === "/releases") return this.serveList(res);
    if (method === "POST" && rest === "/releases")
      return this.serveCreate(res, body);
    if (method === "GET" && (match = /^\/releases\/tags\/([^/]+)$/.exec(rest)))
      return this.serveByTag(res, decodeURIComponent(match[1] ?? ""));
    if (method === "GET" && rest === "/releases/latest")
      return this.serveLatest(res);
    if (
      method === "GET" &&
      (match = /^\/releases\/assets\/([^/]+)$/.exec(rest))
    )
      return this.serveAsset(res, match[1] ?? "");
    if (
      method === "POST" &&
      (match = /^\/releases\/([^/]+)\/assets$/.exec(rest))
    )
      return this.serveUpload(
        res,
        body,
        match[1] ?? "",
        url.searchParams.get("name") ?? "",
      );
    if (method === "GET" && (match = /^\/releases\/([^/]+)$/.exec(rest)))
      return this.serveById(res, match[1] ?? "");
    if (method === "PATCH" && (match = /^\/releases\/([^/]+)$/.exec(rest)))
      return this.servePatch(res, body, match[1] ?? "");
    sendError(res, 404, "Not Found");
  };

  private serveTagRef(res: ServerResponse, tag: string): void {
    const object = this.tagObject;
    const status = this.tagRefStatus;
    this.tagReads++;
    this.tagReadHook?.(this, this.tagReads);
    if (status !== 0) return sendError(res, status, "tag ref unavailable");
    if (object === null) return sendError(res, 404, "Not Found");
    sendJson(res, 200, {
      ref: `refs/tags/${tag}`,
      object: { type: object.type, sha: object.sha },
    });
  }

  private serveAnnotatedTag(res: ServerResponse, sha: string): void {
    const commit = this.annotatedTags.get(sha);
    if (commit === undefined) return sendError(res, 404, "Not Found");
    sendJson(res, 200, { sha, object: { type: "commit", sha: commit } });
  }

  /** Models POST git/refs for a lightweight tag. GitHub answers 422 when the ref exists; tagCreateStatus scripts that. */
  private serveCreateRef(res: ServerResponse, body: Buffer): void {
    const fields = decodeBody(body, res);
    if (fields === undefined) return;
    this.tagCreated.push(fields);
    if (this.tagCreateStatus !== 0) {
      return sendError(
        res,
        this.tagCreateStatus,
        this.tagCreateMessage === ""
          ? "Reference already exists"
          : this.tagCreateMessage,
      );
    }
    const ref = stringField(fields, "ref");
    const sha = stringField(fields, "sha");
    if (ref.startsWith("refs/tags/")) this.tagObject = { type: "commit", sha };
    sendJson(res, 201, { ref, object: { type: "commit", sha } });
  }

  private serveMatchingPlaceholderRefs(res: ServerResponse): void {
    sendJson(
      res,
      200,
      this.placeholderRefs.map((name) => ({
        ref: `refs/tags/${name}`,
        object: { type: "commit", sha: "a".repeat(40) },
      })),
    );
  }

  private serveList(res: ServerResponse): void {
    if (this.listStatus !== 0)
      return sendError(res, this.listStatus, "listing unavailable");
    sendJson(res, 200, this.releases.map(releaseWire));
  }

  private serveCreate(res: ServerResponse, body: Buffer): void {
    const fields = decodeBody(body, res);
    if (fields === undefined) return;
    this.created.push(fields);
    const release = newFakeRelease({
      id: this.nextReleaseId++,
      tagName: stringField(fields, "tag_name"),
      name: stringField(fields, "name"),
      body: stringField(fields, "body"),
      draft: boolField(fields, "draft"),
      prerelease: boolField(fields, "prerelease"),
      targetCommitish: stringField(fields, "target_commitish"),
    });
    this.releases.push(release);
    this.afterCreate?.(this);
    // The answer comes from a merged copy, so the stored release still holds what the request asked for. That
    // split lets one test reject a create response without breaking every later read. The create response echoes
    // the requested tag_name, as github.com does; the placeholder appears on the reads after it, and only while
    // the tag is missing.
    const answer = releaseWire(release);
    release.placeholder = this.placeholderDraftTags && this.tagObject === null;
    for (const [key, value] of Object.entries(this.createReturns))
      answer[key] = value;
    sendJson(res, 201, answer);
  }

  private serveByTag(res: ServerResponse, tag: string): void {
    // The by-tag endpoint answers with published releases only, which is why a draft is found through the listing.
    const release = this.releases.find(
      (candidate) => candidate.tagName === tag && !candidate.draft,
    );
    if (release === undefined) return sendError(res, 404, "Not Found");
    sendJson(res, 200, releaseWire(release));
  }

  private serveById(res: ServerResponse, rawId: string): void {
    const id = Number(rawId);
    if (!Number.isInteger(id)) return sendError(res, 404, "Not Found");
    const code = this.readByIdCodes.shift();
    if (code !== undefined && code !== 200)
      return sendError(res, code, SCRIPTED_READ_MESSAGE);
    const release = this.release(
      this.readByIdReturns !== 0 ? this.readByIdReturns : id,
    );
    if (release === undefined) return sendError(res, 404, "Not Found");
    sendJson(res, 200, releaseWire(release));
  }

  private serveLatest(res: ServerResponse): void {
    this.latestReadTimes.push(Date.now());
    const scripted = this.latestScript.shift();
    const id = scripted !== undefined ? scripted : this.latestId;
    if (id < 0) return sendError(res, 500, "scripted latest failure");
    const release = this.release(id);
    if (id === 0 || release === undefined)
      return sendError(res, 404, "Not Found");
    sendJson(res, 200, releaseWire(release));
  }

  private servePatch(res: ServerResponse, body: Buffer, rawId: string): void {
    const id = Number(rawId);
    if (!Number.isInteger(id)) return sendError(res, 404, "Not Found");
    const fields = decodeBody(body, res);
    if (fields === undefined) return;
    this.patched.push(fields);
    if (this.patchStatus !== 0)
      return sendError(res, this.patchStatus, "patch unavailable");
    const release = this.release(id);
    if (release === undefined) return sendError(res, 404, "Not Found");
    let promoting = false;
    if ("draft" in fields) {
      promoting = release.draft && !boolField(fields, "draft");
      release.draft = boolField(fields, "draft");
    }
    if ("tag_name" in fields) release.tagName = stringField(fields, "tag_name");
    else if (promoting && this.placeholderDraftTags)
      release.tagName = placeholderTagFor(release);
    if ("target_commitish" in fields)
      release.targetCommitish = stringField(fields, "target_commitish");
    if ("prerelease" in fields)
      release.prerelease = boolField(fields, "prerelease");
    if ("body" in fields) release.body = stringField(fields, "body");
    if (
      stringField(fields, "make_latest") === "true" &&
      !release.draft &&
      !release.prerelease
    )
      this.latestId = release.id;
    this.afterPatch?.(this);
    sendJson(res, 200, releaseWire(release));
  }

  private serveUpload(
    res: ServerResponse,
    body: Buffer,
    rawId: string,
    name: string,
  ): void {
    const release = this.release(Number(rawId));
    if (release === undefined) return sendError(res, 404, "Not Found");
    const asset = { id: this.nextAssetId++, name, contents: body };
    release.assets.push(asset);
    this.afterUpload?.(this);
    sendJson(res, 201, assetWire(asset));
  }

  private serveAsset(res: ServerResponse, rawId: string): void {
    const id = Number(rawId);
    const asset = this.findAsset(id);
    if (asset === undefined) return sendError(res, 404, "Not Found");
    if (this.redirectAssets) {
      res.writeHead(302, { Location: `${ASSET_BYTES_ROUTE}${id}` });
      res.end();
      return;
    }
    sendBytes(res, asset.contents);
  }
}

export interface StartedFake {
  fake: ReleaseFake;
  /** The base URL to put in GITHUB_API_URL. */
  url: string;
  /** Every request received so far, as method, path and raw query. Read it after the command returns. */
  seen(): SeenRequest[];
  close(): Promise<void>;
}

/** Starts a local server running a ReleaseFake for repo. */
export async function startReleaseFake(
  repo: string = STAGE_REPO,
): Promise<StartedFake> {
  const fake = new ReleaseFake(repo);
  const server = await startServer(fake.handle);
  return {
    fake,
    url: server.url,
    seen: () => toSeen(server.requests),
    close: () => server.close(),
  };
}

/** Maps the requests startServer recorded to method, path and raw query. */
export function toSeen(
  requests: readonly { method: string; url: string }[],
): SeenRequest[] {
  return requests.map((request) => {
    const url = new URL(request.url, "http://fake.invalid");
    return {
      method: request.method,
      path: url.pathname,
      query: url.search.replace(/^\?/, ""),
    };
  });
}

export function countRequests(
  seen: readonly SeenRequest[],
  method: string,
  path: string,
): number {
  return seen.filter(
    (request) => request.method === method && request.path === path,
  ).length;
}

/** The asset names a run uploaded, in order. An upload names its asset in the query, which separates it from a create. */
export function uploadedNames(seen: readonly SeenRequest[]): string[] {
  return seen
    .filter(
      (request) =>
        request.method === "POST" && request.query.startsWith("name="),
    )
    .map((request) => request.query.slice("name=".length));
}

export function countUploads(seen: readonly SeenRequest[]): number {
  return uploadedNames(seen).length;
}

/** A client for the fake at url, with a fixed stand-in credential. */
export function testClient(url: string): GithubClient {
  const credential = "test-credential";
  return new GithubClient({ token: credential, apiUrl: url });
}

const SUMS_NAME = "SHA256SUMS";

/**
 * Writes a nine-asset bundle for tag into an existing directory: the eight covered assets with distinct
 * bytes, then a SHA256SUMS that verifyLocalAssets and verifyChecksums accept. The stage tests use it in place
 * of a real bundle, which needs git and the Go toolchain.
 */
export function writeSyntheticBundle(dir: string, tag: string): void {
  mkdirSync(dir, { recursive: true });
  const lines: string[] = [];
  for (const name of checksumAssetNames(tag)) {
    const contents = Buffer.from(`synthetic ${name} for ${tag}\n`, "utf8");
    writeFileSync(join(dir, name), contents);
    lines.push(
      `${createHash("sha256").update(contents).digest("hex")}  ${name}\n`,
    );
  }
  writeFileSync(join(dir, SUMS_NAME), lines.join(""));
}
