import { spawn } from "node:child_process";
import { appendFileSync } from "node:fs";
import { EOL } from "node:os";
import type { Env, Writer } from "./io.ts";
import { fileErrorOf } from "./files.ts";

export interface GitOptions {
  env?: Env;
  input?: string | Buffer;
}

export interface RunOptions extends GitOptions {
  cwd?: string;
  // The program name a failure sentence uses. It defaults to the command as
  // given; a caller that runs a program by its path passes a shorter name.
  name?: string;
}

// What one child process left behind. Exactly one of status, signal and
// spawnError describes how it ended. output holds stdout and stderr in the
// order they arrived.
export interface CommandResult {
  command: string;
  status: number | null;
  signal: NodeJS.Signals | null;
  spawnError: Error | undefined;
  stdout: Buffer;
  stderr: Buffer;
  output: Buffer;
}

function childEnv(env: Env | undefined): NodeJS.ProcessEnv {
  if (env === undefined) return process.env;
  const clean: NodeJS.ProcessEnv = {};
  for (const [key, value] of Object.entries(env)) {
    if (value !== undefined) clean[key] = value;
  }
  return clean;
}

// The one place release code starts a child process. Runs command with args,
// without a shell, and collects its streams. It never rejects: callers turn
// the result into their own message with describeExit.
export function runCommand(
  command: string,
  args: readonly string[],
  options: RunOptions = {},
): Promise<CommandResult> {
  return new Promise((resolve) => {
    const out: Buffer[] = [];
    const err: Buffer[] = [];
    const combined: Buffer[] = [];
    let spawnError: Error | undefined;
    let settled = false;
    const settle = (status: number | null, signal: NodeJS.Signals | null) => {
      if (settled) return;
      settled = true;
      resolve({
        command: options.name ?? command,
        status,
        signal,
        spawnError,
        stdout: Buffer.concat(out),
        stderr: Buffer.concat(err),
        output: Buffer.concat(combined),
      });
    };
    let child: ReturnType<typeof spawn>;
    try {
      child = spawn(command, [...args], {
        env: childEnv(options.env),
        cwd: options.cwd,
        stdio: ["pipe", "pipe", "pipe"],
        windowsHide: true,
      });
    } catch (error) {
      // An argument Node refuses (a NUL byte, for one) throws before any process exists.
      spawnError = error instanceof Error ? error : new Error(String(error));
      settle(null, null);
      return;
    }
    child.stdout?.on("data", (chunk: Buffer) => {
      out.push(chunk);
      combined.push(chunk);
    });
    child.stderr?.on("data", (chunk: Buffer) => {
      err.push(chunk);
      combined.push(chunk);
    });
    // A closed stdin (the child exited before reading its input) is not a
    // failure of its own; the exit code says what happened.
    child.stdin?.on("error", () => {});
    child.on("error", (error) => {
      spawnError = error;
      // A process that never started has no streams to wait for.
      if (child.pid === undefined) settle(null, null);
    });
    child.on("close", (status, signal) => settle(status, signal));
    child.stdin?.end(options.input);
  });
}

// Runs `git -C repoRoot ...args` through runCommand.
export function runGit(
  repoRoot: string,
  args: readonly string[],
  options: GitOptions = {},
): Promise<CommandResult> {
  return runCommand("git", ["-C", repoRoot, ...args], options);
}

// How a failed run reads: `<command> exited with code <n>`, `<command> was
// killed by <signal>`, or `<command> could not start: <reason>`, where command
// is the program name, never its arguments.
export function describeExit(result: CommandResult): string {
  if (result.spawnError !== undefined)
    return `${result.command} could not start: ${result.spawnError.message}`;
  if (result.signal !== null)
    return `${result.command} was killed by ${result.signal}`;
  return `${result.command} exited with code ${result.status}`;
}

export function succeeded(result: CommandResult): boolean {
  return (
    result.spawnError === undefined &&
    result.signal === null &&
    result.status === 0
  );
}

// Resolves git's standard output, untrimmed. A failure rejects with
// `git <args>: <failure sentence>: <trimmed stderr>`.
export async function git(
  repoRoot: string,
  args: readonly string[],
  options: GitOptions = {},
): Promise<string> {
  return (await gitBuffer(repoRoot, args, options)).toString("utf8");
}

export async function gitBuffer(
  repoRoot: string,
  args: readonly string[],
  options: GitOptions = {},
): Promise<Buffer> {
  const result = await runGit(repoRoot, args, options);
  if (!succeeded(result)) {
    const stderr = result.stderr.toString("utf8").trim();
    throw new Error(
      `git ${args.join(" ")}: ${describeExit(result)}: ${stderr}`,
    );
  }
  return result.stdout;
}

const OUTPUT_DELIMITER = "_GitHubActionsFileCommandDelimeter_";

// With GITHUB_OUTPUT set, appends each key in sorted order as a multi-line file
// command; with it unset, writes `key=value` lines to stdout. A file that
// cannot be written throws.
export function writeOutputs(
  outputs: Readonly<Record<string, string>>,
  env: Env,
  stdout: Writer = process.stdout,
): void {
  const keys = Object.keys(outputs).sort();
  const path = env.GITHUB_OUTPUT;
  if (path === undefined || path === "") {
    for (const key of keys) stdout.write(`${key}=${outputs[key]}\n`);
    return;
  }
  for (const key of keys) {
    const command = `${key}<<${OUTPUT_DELIMITER}${EOL}${outputs[key]}${EOL}${OUTPUT_DELIMITER}${EOL}`;
    try {
      appendFileSync(path, command, { mode: 0o644 });
    } catch (error) {
      throw new Error(
        `write GITHUB_OUTPUT: unable to write command to the environment file: ${fileErrorOf(error, path).message}`,
      );
    }
  }
}

// ---------------------------------------------------------------------------
// GitHub REST client for the calls the release commands make. Plain fetch, no
// retry, no auto-pagination: a caller that wants the next page asks for it.
// ---------------------------------------------------------------------------

/** Bounds one API call, body included. The release runs unattended, so a stalled connection has to fail. */
export const API_TIMEOUT_MS = 30_000;
/** Bounds the credential-free read of an asset from its download host. A release archive is several megabytes. */
export const DOWNLOAD_TIMEOUT_MS = 5 * 60_000;
/** The message when neither GH_TOKEN nor GITHUB_TOKEN holds a value. It names both. */
export const MISSING_CREDENTIAL =
  "no GitHub credential: set GH_TOKEN or GITHUB_TOKEN";

const PRODUCTION_API_URL = "https://api.github.com";
const PRODUCTION_UPLOAD_URL = "https://uploads.github.com";
const MEDIA_TYPE_JSON = "application/vnd.github.v3+json";
const MEDIA_TYPE_BYTES = "application/octet-stream";
const API_VERSION = "2022-11-28";
const USER_AGENT = "agentics-release";
const MAX_REDIRECTS = 10;

export interface ReleaseAsset {
  id: number;
  name: string;
}

/** One release as the API reports it. Absent or null fields read as "", false, 0 or an empty list. */
export interface Release {
  id: number;
  tag_name: string;
  name: string;
  body: string;
  draft: boolean;
  prerelease: boolean;
  target_commitish: string;
  assets: ReleaseAsset[];
}

export interface GitObject {
  type: string;
  sha: string;
}

export interface GitRef {
  ref: string;
  object: GitObject;
}

export interface GitTag {
  sha: string;
  object: GitObject;
}

export interface PullRequest {
  number: number;
  html_url: string;
  title: string;
  body: string;
}

export interface CreateReleaseRequest {
  tag_name: string;
  target_commitish: string;
  name: string;
  body: string;
  draft: boolean;
  prerelease: boolean;
}

export interface UpdateReleaseRequest {
  tag_name: string;
  target_commitish: string;
  draft: boolean;
  prerelease: boolean;
  make_latest: "true" | "false";
}

/** One page of a listing. nextPage is 0 when the response carried no rel="next" link. */
export interface Page<T> {
  items: T[];
  nextPage: number;
}

export interface GithubErrorDetail {
  resource: string;
  field: string;
  code: string;
  message: string;
}

/** A non-2xx API answer. The status decides what a caller does with it, never the message text. */
export class GithubError extends Error {
  readonly status: number;
  readonly apiMessage: string;
  readonly details: readonly GithubErrorDetail[];
  constructor(
    method: string,
    url: string,
    status: number,
    apiMessage: string,
    details: readonly GithubErrorDetail[],
  ) {
    super(
      `${method} ${url}: ${status} ${apiMessage} ${formatDetails(details)}`,
    );
    this.name = "GithubError";
    this.status = status;
    this.apiMessage = apiMessage;
    this.details = details;
  }
}

// Renders the detail list as [{Resource:r Field:f Code:c Message:m} ...], one brace group per detail.
function formatDetails(details: readonly GithubErrorDetail[]): string {
  const rendered = details.map(
    (d) =>
      `{Resource:${d.resource} Field:${d.field} Code:${d.code} Message:${d.message}}`,
  );
  return `[${rendered.join(" ")}]`;
}

/** True for a GitHub 404, false for a transport failure or any other status. */
export function isNotFound(error: unknown): boolean {
  return error instanceof GithubError && error.status === 404;
}

/**
 * Returns the number after `page=` of the rel="next" link in a Link header, or 0.
 * Only the first header value is read, and a link with no page parameter is ignored.
 */
export function parseNextPage(link: string | null | undefined): number {
  if (!link) return 0;
  for (const part of link.split(",")) {
    const segments = part.trim().split(";");
    if (segments.length < 2) continue;
    const target = (segments[0] ?? "").trim();
    if (!target.startsWith("<") || !target.endsWith(">")) continue;
    if (!segments.slice(1).some((segment) => segment.trim() === 'rel="next"'))
      continue;
    let page: string | null;
    try {
      page = new URL(
        target.slice(1, -1),
        "http://link.invalid/",
      ).searchParams.get("page");
    } catch {
      continue;
    }
    const number = Number(page);
    if (page !== null && Number.isSafeInteger(number) && number > 0)
      return number;
  }
  return 0;
}

export interface GithubClientOptions {
  token: string;
  /** The value of GITHUB_API_URL. Empty, absent or production keeps the default API and upload hosts. */
  apiUrl?: string | undefined;
  apiTimeoutMs?: number;
  downloadTimeoutMs?: number;
}

interface Answer {
  status: number;
  headers: Headers;
  bytes: Buffer;
}

export class GithubClient {
  readonly baseUrl: string;
  readonly uploadUrl: string;
  readonly apiTimeoutMs: number;
  readonly downloadTimeoutMs: number;
  readonly #token: string;

  /**
   * Builds a client from GH_TOKEN, or GITHUB_TOKEN when GH_TOKEN is empty, and from GITHUB_API_URL.
   * GITHUB_API_URL replaces the API host and the upload host alike, which is how a test points the
   * client at a local server. Throws Error(MISSING_CREDENTIAL) when no token is set.
   */
  static fromEnv(env: Env): GithubClient {
    const token =
      (env["GH_TOKEN"] ?? "").trim() || (env["GITHUB_TOKEN"] ?? "").trim();
    if (token === "") throw new Error(MISSING_CREDENTIAL);
    return new GithubClient({ token, apiUrl: env["GITHUB_API_URL"] });
  }

  constructor(options: GithubClientOptions) {
    this.#token = options.token;
    this.apiTimeoutMs = options.apiTimeoutMs ?? API_TIMEOUT_MS;
    this.downloadTimeoutMs = options.downloadTimeoutMs ?? DOWNLOAD_TIMEOUT_MS;
    const override = (options.apiUrl ?? "").trim();
    if (
      override === "" ||
      override === PRODUCTION_API_URL ||
      override === `${PRODUCTION_API_URL}/`
    ) {
      this.baseUrl = `${PRODUCTION_API_URL}/`;
      this.uploadUrl = `${PRODUCTION_UPLOAD_URL}/`;
    } else {
      this.baseUrl = override.endsWith("/") ? override : `${override}/`;
      this.uploadUrl = this.baseUrl;
    }
  }

  async getRef(owner: string, repo: string, ref: string): Promise<GitRef> {
    const escaped = ref
      .replace(/^refs\//, "")
      .split("/")
      .map(encodeURIComponent)
      .join("/");
    return toRef(
      await this.json(
        "GET",
        this.api(`repos/${owner}/${repo}/git/ref/${escaped}`),
      ),
    );
  }

  async getTag(owner: string, repo: string, sha: string): Promise<GitTag> {
    const raw = asRecord(
      await this.json(
        "GET",
        this.api(`repos/${owner}/${repo}/git/tags/${sha}`),
      ),
    );
    return { sha: str(raw["sha"]), object: toObject(raw["object"]) };
  }

  async createRef(
    owner: string,
    repo: string,
    ref: string,
    sha: string,
  ): Promise<GitRef> {
    const body = { ref: `refs/${ref.replace(/^refs\//, "")}`, sha };
    return toRef(
      await this.json(
        "POST",
        this.api(`repos/${owner}/${repo}/git/refs`),
        body,
      ),
    );
  }

  /** One call, no pagination: the matching-refs listing the placeholder check reads is a single page. */
  async listMatchingRefs(
    owner: string,
    repo: string,
    ref: string,
  ): Promise<GitRef[]> {
    const escaped = ref
      .replace(/^refs\//, "")
      .split("/")
      .map(encodeURIComponent)
      .join("/");
    const raw = await this.json(
      "GET",
      this.api(`repos/${owner}/${repo}/git/matching-refs/${escaped}`),
    );
    return asArray(raw).map(toRef);
  }

  /** One page of releases, 100 per page. Pass the previous page's nextPage to read the next one. */
  async listReleases(
    owner: string,
    repo: string,
    page = 0,
  ): Promise<Page<Release>> {
    const query = new URLSearchParams();
    if (page > 0) query.set("page", String(page));
    query.set("per_page", "100");
    const answer = await this.send(
      "GET",
      this.api(`repos/${owner}/${repo}/releases?${query}`),
    );
    const items = asArray(this.decode(answer)).map(toRelease);
    return { items, nextPage: parseNextPage(answer.headers.get("link")) };
  }

  async getRelease(owner: string, repo: string, id: number): Promise<Release> {
    return toRelease(
      await this.json("GET", this.api(`repos/${owner}/${repo}/releases/${id}`)),
    );
  }

  /** Published releases only: the API answers 404 for a draft. */
  async getReleaseByTag(
    owner: string,
    repo: string,
    tag: string,
  ): Promise<Release> {
    return toRelease(
      await this.json(
        "GET",
        this.api(`repos/${owner}/${repo}/releases/tags/${tag}`),
      ),
    );
  }

  async getLatestRelease(owner: string, repo: string): Promise<Release> {
    return toRelease(
      await this.json(
        "GET",
        this.api(`repos/${owner}/${repo}/releases/latest`),
      ),
    );
  }

  async createRelease(
    owner: string,
    repo: string,
    request: CreateReleaseRequest,
  ): Promise<Release> {
    return toRelease(
      await this.json(
        "POST",
        this.api(`repos/${owner}/${repo}/releases`),
        request,
      ),
    );
  }

  async updateRelease(
    owner: string,
    repo: string,
    id: number,
    request: UpdateReleaseRequest,
  ): Promise<Release> {
    return toRelease(
      await this.json(
        "PATCH",
        this.api(`repos/${owner}/${repo}/releases/${id}`),
        request,
      ),
    );
  }

  async getPullRequest(
    owner: string,
    repo: string,
    number: number,
  ): Promise<PullRequest> {
    const raw = asRecord(
      await this.json(
        "GET",
        this.api(`repos/${owner}/${repo}/pulls/${number}`),
      ),
    );
    return {
      number: num(raw["number"]),
      html_url: str(raw["html_url"]),
      title: str(raw["title"]),
      body: str(raw["body"]),
    };
  }

  /**
   * Uploads one asset. The name travels in the query, as the upload route requires.
   * It never replaces an asset: the caller reaches it only when the roster has no asset of that name.
   */
  async uploadReleaseAsset(
    owner: string,
    repo: string,
    id: number,
    name: string,
    contents: Buffer,
  ): Promise<ReleaseAsset> {
    const url = new URL(
      `repos/${owner}/${repo}/releases/${id}/assets?${new URLSearchParams({ name })}`,
      this.uploadUrl,
    );
    const answer = await this.send("POST", url, {
      body: contents,
      contentType: MEDIA_TYPE_BYTES,
    });
    const raw = asRecord(this.decode(answer));
    return { id: num(raw["id"]), name: str(raw["name"]) };
  }

  /**
   * Reads the published bytes of one asset.
   *
   * github.com answers this route with a 302 to a separate download host. The first request is sent with
   * redirect "manual" and the token; the Location is then fetched by a second request that carries no
   * credential and a five-minute bound. A 200 with no redirect is returned as it is. A 301 within the
   * API origin (scheme, host and port) is followed with the token; any other redirect is an error.
   */
  async downloadReleaseAsset(
    owner: string,
    repo: string,
    id: number,
  ): Promise<Buffer> {
    let url = this.api(`repos/${owner}/${repo}/releases/assets/${id}`);
    for (let hop = 0; hop <= MAX_REDIRECTS; hop++) {
      const answer = await this.send("GET", url, {
        accept: MEDIA_TYPE_BYTES,
        manual: true,
      });
      if (answer.status < 300 || answer.status > 399) return answer.bytes;
      const location = answer.headers.get("location");
      if (location === null || location === "")
        throw new Error(
          `GET ${url}: ${answer.status} redirect with no Location`,
        );
      const target = new URL(location, this.baseUrl);
      if (answer.status === 302) return this.fetchRedirectTarget(target);
      if (answer.status !== 301)
        throw new Error(
          `GET ${url}: unexpected redirection response ${answer.status}`,
        );
      // The next request carries the token, so the whole origin must match: a
      // same-host redirect from https to http would send it in cleartext.
      const origin = new URL(this.baseUrl).origin;
      if (target.origin !== origin) {
        throw new Error(
          `refusing to follow cross-origin redirect from "${origin}" to "${target.origin}"`,
        );
      }
      url = target;
    }
    throw new Error(`GET ${url}: reached the maximum amount of redirections`);
  }

  private async fetchRedirectTarget(target: URL): Promise<Buffer> {
    const headers = { Accept: MEDIA_TYPE_BYTES, "User-Agent": USER_AGENT };
    let answer: Answer;
    try {
      const response = await fetch(target, {
        method: "GET",
        headers,
        signal: AbortSignal.timeout(this.downloadTimeoutMs),
      });
      answer = {
        status: response.status,
        headers: response.headers,
        bytes: Buffer.from(await response.arrayBuffer()),
      };
    } catch (error) {
      throw transportError("GET", target, error);
    }
    if (answer.status < 200 || answer.status > 299)
      throw apiError("GET", target, answer);
    return answer.bytes;
  }

  private api(path: string): URL {
    return new URL(path, this.baseUrl);
  }

  private async json(
    method: string,
    url: URL,
    body?: unknown,
  ): Promise<unknown> {
    const answer = await this.send(
      method,
      url,
      body === undefined
        ? {}
        : { body: JSON.stringify(body), contentType: "application/json" },
    );
    return this.decode(answer);
  }

  private decode(answer: Answer): unknown {
    try {
      return JSON.parse(answer.bytes.toString("utf8"));
    } catch {
      throw new Error(`unreadable GitHub response (status ${answer.status})`);
    }
  }

  private async send(
    method: string,
    url: URL,
    options: {
      body?: string | Buffer;
      contentType?: string;
      accept?: string;
      manual?: boolean;
    } = {},
  ): Promise<Answer> {
    const headers: Record<string, string> = {
      Accept: options.accept ?? MEDIA_TYPE_JSON,
      Authorization: `Bearer ${this.#token}`,
      "User-Agent": USER_AGENT,
      "X-GitHub-Api-Version": API_VERSION,
    };
    if (options.contentType !== undefined)
      headers["Content-Type"] = options.contentType;
    let answer: Answer;
    try {
      const response = await fetch(url, {
        method,
        headers,
        body: options.body ?? null,
        redirect: options.manual ? "manual" : "follow",
        signal: AbortSignal.timeout(this.apiTimeoutMs),
      });
      answer = {
        status: response.status,
        headers: response.headers,
        bytes: Buffer.from(await response.arrayBuffer()),
      };
    } catch (error) {
      throw transportError(method, url, error);
    }
    if (options.manual && answer.status >= 300 && answer.status <= 399)
      return answer;
    if (answer.status < 200 || answer.status > 299)
      throw apiError(method, url, answer);
    return answer;
  }
}

function transportError(method: string, url: URL, error: unknown): Error {
  const cause = error instanceof Error ? error : new Error(String(error));
  const detail =
    cause.name === "TimeoutError"
      ? "request timed out"
      : cause.cause instanceof Error
        ? cause.cause.message
        : cause.message;
  return new Error(`${method} ${url}: ${detail}`, { cause });
}

function apiError(method: string, url: URL, answer: Answer): GithubError {
  let message = "";
  let details: GithubErrorDetail[] = [];
  try {
    const raw = asRecord(JSON.parse(answer.bytes.toString("utf8")));
    message = str(raw["message"]);
    details = (Array.isArray(raw["errors"]) ? raw["errors"] : []).map(
      (item: unknown) => {
        const entry =
          typeof item === "object" && item !== null
            ? (item as Record<string, unknown>)
            : {};
        return {
          resource: str(entry["resource"]),
          field: str(entry["field"]),
          code: str(entry["code"]),
          message: str(entry["message"]),
        };
      },
    );
  } catch {
    // An error body that is not JSON leaves the message empty.
  }
  return new GithubError(
    method,
    url.toString(),
    answer.status,
    message,
    details,
  );
}

function asRecord(value: unknown): Record<string, unknown> {
  if (typeof value !== "object" || value === null || Array.isArray(value))
    throw new Error("unexpected GitHub response shape");
  return value as Record<string, unknown>;
}

function asArray(value: unknown): unknown[] {
  if (!Array.isArray(value))
    throw new Error("unexpected GitHub response shape");
  return value;
}

const str = (value: unknown): string =>
  typeof value === "string" ? value : "";
const num = (value: unknown): number => (typeof value === "number" ? value : 0);

function toObject(value: unknown): GitObject {
  const raw =
    typeof value === "object" && value !== null
      ? (value as Record<string, unknown>)
      : {};
  return { type: str(raw["type"]), sha: str(raw["sha"]) };
}

function toRef(value: unknown): GitRef {
  const raw = asRecord(value);
  return { ref: str(raw["ref"]), object: toObject(raw["object"]) };
}

function toRelease(value: unknown): Release {
  const raw = asRecord(value);
  const assets = Array.isArray(raw["assets"]) ? raw["assets"] : [];
  return {
    id: num(raw["id"]),
    tag_name: str(raw["tag_name"]),
    name: str(raw["name"]),
    body: str(raw["body"]),
    draft: raw["draft"] === true,
    prerelease: raw["prerelease"] === true,
    target_commitish: str(raw["target_commitish"]),
    assets: assets.map((asset: unknown) => {
      const entry = asRecord(asset);
      return { id: num(entry["id"]), name: str(entry["name"]) };
    }),
  };
}
