import assert from "node:assert/strict";
import type { ServerResponse } from "node:http";
import { test } from "node:test";
import {
  API_TIMEOUT_MS,
  DOWNLOAD_TIMEOUT_MS,
  GithubClient,
  GithubError,
  MISSING_CREDENTIAL,
  isNotFound,
  parseNextPage,
} from "./github.ts";
import { startServer } from "./testing/support.ts";

// Fixed stand-ins; no test reaches the real API.
const PRIMARY = "primary-credential";
const SECOND = "second-credential";
const OTHER = "other-credential";

function sendJson(
  res: ServerResponse,
  status: number,
  value: unknown,
  headers: Record<string, string> = {},
): void {
  res.writeHead(status, { "Content-Type": "application/json", ...headers });
  res.end(JSON.stringify(value));
}

function holdOpen(res: ServerResponse): Promise<void> {
  return new Promise((resolve) => res.on("close", () => resolve()));
}

function header(value: string | string[] | undefined): string | undefined {
  return Array.isArray(value) ? value[0] : value;
}

test("fromEnv sends GH_TOKEN as a Bearer token and points both hosts at GITHUB_API_URL", async () => {
  const server = await startServer((_req, _body, res) =>
    sendJson(res, 200, {}),
  );
  try {
    const client = GithubClient.fromEnv({
      GITHUB_API_URL: server.url,
      GH_TOKEN: PRIMARY,
      GITHUB_TOKEN: SECOND,
    });
    await client.getLatestRelease("o", "r");
    assert.equal(
      header(server.requests[0]?.headers["authorization"]),
      `Bearer ${PRIMARY}`,
    );
    assert.equal(client.baseUrl, `${server.url}/`);
    assert.equal(client.uploadUrl, `${server.url}/`);
    assert.equal(client.apiTimeoutMs, 30_000);
    assert.equal(API_TIMEOUT_MS, 30_000);
    assert.equal(DOWNLOAD_TIMEOUT_MS, 300_000);
  } finally {
    await server.close();
  }
});

test("fromEnv falls back to GITHUB_TOKEN when GH_TOKEN is empty or blank", async () => {
  const server = await startServer((_req, _body, res) =>
    sendJson(res, 200, {}),
  );
  try {
    const padded = ` ${SECOND} `;
    for (const blank of ["", "   "]) {
      const client = GithubClient.fromEnv({
        GITHUB_API_URL: server.url,
        GH_TOKEN: blank,
        GITHUB_TOKEN: padded,
      });
      await client.getLatestRelease("o", "r");
    }
    assert.deepEqual(
      server.requests.map((request) =>
        header(request.headers["authorization"]),
      ),
      [`Bearer ${SECOND}`, `Bearer ${SECOND}`],
    );
  } finally {
    await server.close();
  }
});

test("fromEnv throws an error that names both variables when neither holds a token", () => {
  for (const env of [
    {},
    { GH_TOKEN: "", GITHUB_TOKEN: "" },
    { GH_TOKEN: " ", GITHUB_TOKEN: "\t" },
  ]) {
    assert.throws(
      () => GithubClient.fromEnv(env),
      (error: unknown) =>
        error instanceof Error &&
        error.message === MISSING_CREDENTIAL &&
        error.message.includes("GH_TOKEN") &&
        error.message.includes("GITHUB_TOKEN"),
    );
  }
});

test("fromEnv keeps the API and upload hosts apart for an unset or production GITHUB_API_URL", () => {
  const cases: Record<string, string | undefined> = {
    unset: undefined,
    empty: "",
    production: "https://api.github.com",
    "production with a trailing slash": "https://api.github.com/",
  };
  for (const [name, apiUrl] of Object.entries(cases)) {
    const client = GithubClient.fromEnv({
      GH_TOKEN: PRIMARY,
      GITHUB_API_URL: apiUrl,
    });
    assert.equal(client.baseUrl, "https://api.github.com/", name);
    assert.equal(client.uploadUrl, "https://uploads.github.com/", name);
  }
  const override = GithubClient.fromEnv({
    GH_TOKEN: PRIMARY,
    GITHUB_API_URL: "http://127.0.0.1:9/base",
  });
  assert.equal(override.baseUrl, "http://127.0.0.1:9/base/");
  assert.equal(override.uploadUrl, "http://127.0.0.1:9/base/");
});

test("an API call that gets no answer within apiTimeoutMs fails instead of holding the job", async () => {
  const server = await startServer((_req, _body, res) => holdOpen(res));
  try {
    const client = new GithubClient({
      token: OTHER,
      apiUrl: server.url,
      apiTimeoutMs: 100,
    });
    const started = Date.now();
    await assert.rejects(
      client.getRelease("o", "r", 1),
      (error: unknown) =>
        error instanceof Error &&
        !(error instanceof GithubError) &&
        /timed out/.test(error.message),
    );
    assert.ok(
      Date.now() - started < 5_000,
      "the call waited far longer than its bound",
    );
  } finally {
    await server.close();
  }
});

test("a failed call is not retried and surfaces the status", async () => {
  const server = await startServer((_req, _body, res) =>
    sendJson(res, 500, { message: "boom" }),
  );
  try {
    const client = new GithubClient({ token: OTHER, apiUrl: server.url });
    await assert.rejects(
      client.getRelease("o", "r", 7),
      (error: unknown) =>
        error instanceof GithubError &&
        error.status === 500 &&
        error.apiMessage === "boom",
    );
    assert.equal(server.requests.length, 1, "a 500 must not be retried");
  } finally {
    await server.close();
  }
});

test("isNotFound decides by status type, never by message text", async () => {
  const server = await startServer((req, _body, res) => {
    if (req.url?.endsWith("/releases/1"))
      sendJson(res, 404, { message: "Not Found" });
    else sendJson(res, 500, { message: "this body says 404 on purpose" });
  });
  try {
    const client = new GithubClient({ token: OTHER, apiUrl: server.url });
    const missing = await client
      .getRelease("o", "r", 1)
      .catch((error: unknown) => error);
    const broken = await client
      .getRelease("o", "r", 2)
      .catch((error: unknown) => error);
    assert.equal(isNotFound(missing), true);
    assert.equal(isNotFound(broken), false);
    assert.ok(broken instanceof GithubError && broken.message.includes("404"));
  } finally {
    await server.close();
  }
  assert.equal(isNotFound(new Error("404 Not Found")), false);
});

test("a transport failure is a plain error, not a GithubError", async () => {
  const server = await startServer((_req, _body, res) =>
    sendJson(res, 200, {}),
  );
  const url = server.url;
  await server.close();
  const client = new GithubClient({ token: OTHER, apiUrl: url });
  const error = await client
    .getRelease("o", "r", 1)
    .catch((caught: unknown) => caught);
  assert.ok(error instanceof Error);
  assert.ok(!(error instanceof GithubError));
  assert.equal(isNotFound(error), false);
  assert.ok(
    error.message.startsWith(`GET ${url}/repos/o/r/releases/1: `),
    error.message,
  );
});

test("a GithubError message renders the status, the API message and the detail list", async () => {
  const server = await startServer((_req, _body, res) =>
    sendJson(res, 422, {
      message: "Validation Failed",
      errors: [
        { resource: "Release", field: "tag_name", code: "already_exists" },
      ],
    }),
  );
  try {
    const client = new GithubClient({ token: OTHER, apiUrl: server.url });
    const error = await client
      .createRef("o", "r", "refs/tags/v1.2.3", "a".repeat(40))
      .catch((caught: unknown) => caught);
    assert.ok(error instanceof GithubError);
    assert.equal(error.status, 422);
    assert.deepEqual(error.details, [
      {
        resource: "Release",
        field: "tag_name",
        code: "already_exists",
        message: "",
      },
    ]);
    assert.equal(
      error.message,
      `POST ${server.url}/repos/o/r/git/refs: 422 Validation Failed [{Resource:Release Field:tag_name Code:already_exists Message:}]`,
    );
  } finally {
    await server.close();
  }
});

test("parseNextPage reads the page of the rel=next link and nothing else", () => {
  const base = "https://api.github.com/repos/o/r/releases";
  assert.equal(
    parseNextPage(
      `<${base}?per_page=100&page=2>; rel="next", <${base}?page=9>; rel="last"`,
    ),
    2,
  );
  assert.equal(
    parseNextPage(
      `<${base}?page=9>; rel="last", <${base}?page=3&per_page=100>; rel="next"`,
    ),
    3,
  );
  assert.equal(parseNextPage(`<${base}?page=1>; rel="prev"`), 0);
  assert.equal(parseNextPage(`<${base}?per_page=100>; rel="next"`), 0);
  assert.equal(parseNextPage(`${base}?page=2; rel="next"`), 0);
  assert.equal(parseNextPage(`<${base}?page=0>; rel="next"`), 0);
  assert.equal(parseNextPage(`<${base}?page=abc>; rel="next"`), 0);
  assert.equal(parseNextPage(""), 0);
  assert.equal(parseNextPage(null), 0);
  assert.equal(parseNextPage(undefined), 0);
});

test("listReleases asks for 100 per page, reports nextPage from the Link header and never pages on its own", async () => {
  const server = await startServer((req, _body, res) => {
    const page = new URL(req.url ?? "/", "http://x").searchParams.get("page");
    if (page === "2") {
      sendJson(res, 200, [
        { id: 3, tag_name: "v1.0.0", name: null, draft: false },
      ]);
      return;
    }
    const link = `<http://127.0.0.1/repos/o/r/releases?page=2&per_page=100>; rel="next"`;
    sendJson(
      res,
      200,
      [{ id: 1, tag_name: "v2.0.0", name: "v2.0.0", draft: true }],
      { Link: link },
    );
  });
  try {
    const client = new GithubClient({ token: OTHER, apiUrl: server.url });
    const first = await client.listReleases("o", "r");
    assert.equal(first.nextPage, 2);
    assert.deepEqual(
      first.items.map((release) => [
        release.id,
        release.tag_name,
        release.draft,
      ]),
      [[1, "v2.0.0", true]],
    );
    assert.equal(server.requests.length, 1, "one call reads one page");
    assert.equal(server.requests[0]?.url, "/repos/o/r/releases?per_page=100");

    const second = await client.listReleases("o", "r", first.nextPage);
    assert.equal(second.nextPage, 0);
    assert.equal(
      second.items[0]?.name,
      "",
      "a null name reads as an empty string",
    );
    assert.equal(
      server.requests[1]?.url,
      "/repos/o/r/releases?page=2&per_page=100",
    );
  } finally {
    await server.close();
  }
});

test("release JSON is normalized: absent fields read as empty values and assets keep id and name", async () => {
  const server = await startServer((_req, _body, res) =>
    sendJson(res, 200, {
      id: 42,
      tag_name: "v1.2.3",
      assets: [{ id: 1000, name: "SHA256SUMS", size: 3 }],
    }),
  );
  try {
    const client = new GithubClient({ token: OTHER, apiUrl: server.url });
    assert.deepEqual(await client.getReleaseByTag("o", "r", "v1.2.3"), {
      id: 42,
      tag_name: "v1.2.3",
      name: "",
      body: "",
      draft: false,
      prerelease: false,
      target_commitish: "",
      assets: [{ id: 1000, name: "SHA256SUMS" }],
    });
    assert.equal(server.requests[0]?.url, "/repos/o/r/releases/tags/v1.2.3");
  } finally {
    await server.close();
  }
});

test("write calls send the expected request bodies to the expected routes", async () => {
  const server = await startServer((req, _body, res) => {
    if (req.method === "POST" && req.url?.endsWith("/git/refs"))
      sendJson(res, 201, {
        ref: "refs/tags/v1.2.3",
        object: { type: "commit", sha: "a".repeat(40) },
      });
    else sendJson(res, 200, { id: 42 });
  });
  try {
    const client = new GithubClient({ token: OTHER, apiUrl: server.url });
    const ref = await client.createRef("o", "r", "tags/v1.2.3", "a".repeat(40));
    assert.deepEqual(ref, {
      ref: "refs/tags/v1.2.3",
      object: { type: "commit", sha: "a".repeat(40) },
    });
    await client.createRelease("o", "r", {
      tag_name: "v1.2.3",
      target_commitish: "b".repeat(40),
      name: "v1.2.3",
      body: "notes <&>",
      draft: true,
      prerelease: true,
    });
    await client.updateRelease("o", "r", 42, {
      tag_name: "v1.2.3",
      target_commitish: "b".repeat(40),
      draft: false,
      prerelease: false,
      make_latest: "true",
    });
    const [createRef, createRelease, updateRelease] = server.requests;
    assert.equal(
      `${createRef?.method} ${createRef?.url}`,
      "POST /repos/o/r/git/refs",
    );
    assert.deepEqual(JSON.parse(createRef?.body.toString("utf8") ?? ""), {
      ref: "refs/tags/v1.2.3",
      sha: "a".repeat(40),
    });
    assert.equal(
      `${createRelease?.method} ${createRelease?.url}`,
      "POST /repos/o/r/releases",
    );
    assert.deepEqual(JSON.parse(createRelease?.body.toString("utf8") ?? ""), {
      tag_name: "v1.2.3",
      target_commitish: "b".repeat(40),
      name: "v1.2.3",
      body: "notes <&>",
      draft: true,
      prerelease: true,
    });
    assert.equal(
      header(createRelease?.headers["content-type"]),
      "application/json",
    );
    assert.equal(
      `${updateRelease?.method} ${updateRelease?.url}`,
      "PATCH /repos/o/r/releases/42",
    );
    assert.deepEqual(JSON.parse(updateRelease?.body.toString("utf8") ?? ""), {
      tag_name: "v1.2.3",
      target_commitish: "b".repeat(40),
      draft: false,
      prerelease: false,
      make_latest: "true",
    });
  } finally {
    await server.close();
  }
});

test("uploadReleaseAsset posts the bytes to the upload route with the name in the query", async () => {
  const server = await startServer((_req, _body, res) =>
    sendJson(res, 201, { id: 1000, name: "agentics-v1.2.3.tar.gz" }),
  );
  try {
    const client = new GithubClient({ token: OTHER, apiUrl: server.url });
    const bytes = Buffer.from([0, 1, 2, 255]);
    const asset = await client.uploadReleaseAsset(
      "o",
      "r",
      42,
      "agentics-v1.2.3.tar.gz",
      bytes,
    );
    assert.deepEqual(asset, { id: 1000, name: "agentics-v1.2.3.tar.gz" });
    const request = server.requests[0];
    assert.equal(
      `${request?.method} ${request?.url}`,
      "POST /repos/o/r/releases/42/assets?name=agentics-v1.2.3.tar.gz",
    );
    assert.equal(
      header(request?.headers["content-type"]),
      "application/octet-stream",
    );
    assert.equal(header(request?.headers["authorization"]), `Bearer ${OTHER}`);
    assert.deepEqual(request?.body, bytes);
  } finally {
    await server.close();
  }
});

test("getRef escapes each path segment and listMatchingRefs sends one request with no query", async () => {
  const server = await startServer((req, _body, res) => {
    if (req.url?.includes("/matching-refs/"))
      sendJson(res, 200, [
        {
          ref: "refs/tags/untagged-abc",
          object: { type: "commit", sha: "c".repeat(40) },
        },
      ]);
    else
      sendJson(res, 200, {
        ref: "refs/tags/v1.2.3",
        object: { type: "tag", sha: "d".repeat(40) },
      });
  });
  try {
    const client = new GithubClient({ token: OTHER, apiUrl: server.url });
    const ref = await client.getRef("o", "r", "tags/v1.2.3");
    assert.deepEqual(ref.object, { type: "tag", sha: "d".repeat(40) });
    const matches = await client.listMatchingRefs("o", "r", "tags/untagged-");
    assert.deepEqual(
      matches.map((match) => match.ref),
      ["refs/tags/untagged-abc"],
    );
    assert.deepEqual(
      server.requests.map((request) => request.url),
      [
        "/repos/o/r/git/ref/tags/v1.2.3",
        "/repos/o/r/git/matching-refs/tags/untagged-",
      ],
    );
  } finally {
    await server.close();
  }
});

test("downloadReleaseAsset follows a 302 with a second request that carries no credential", async () => {
  const cdn = await startServer((_req, _body, res) => {
    res.writeHead(200, { "Content-Type": "application/octet-stream" });
    res.end(Buffer.from("asset bytes"));
  });
  const api = await startServer((_req, _body, res) => {
    res.writeHead(302, { Location: `${cdn.url}/signed/asset?sig=abc` });
    res.end();
  });
  try {
    const client = new GithubClient({ token: PRIMARY, apiUrl: api.url });
    assert.equal(
      (await client.downloadReleaseAsset("o", "r", 1000)).toString("utf8"),
      "asset bytes",
    );

    const first = api.requests[0];
    assert.equal(first?.url, "/repos/o/r/releases/assets/1000");
    assert.equal(header(first?.headers["authorization"]), `Bearer ${PRIMARY}`);
    assert.equal(header(first?.headers["accept"]), "application/octet-stream");

    const second = cdn.requests[0];
    assert.equal(second?.url, "/signed/asset?sig=abc");
    assert.equal(
      second?.headers["authorization"],
      undefined,
      "the download host must never see the credential",
    );
    assert.equal(header(second?.headers["accept"]), "application/octet-stream");
    assert.equal(api.requests.length, 1);
    assert.equal(cdn.requests.length, 1);
  } finally {
    await api.close();
    await cdn.close();
  }
});

test("downloadReleaseAsset resolves a relative Location against the API host and returns a direct 200 as it is", async () => {
  const api = await startServer((req, _body, res) => {
    if (req.url?.startsWith("/repos/o/r/releases/assets/")) {
      if (req.url.endsWith("/1")) {
        res.writeHead(302, { Location: "/release-asset-bytes/1" });
        res.end();
      } else {
        res.writeHead(200);
        res.end(Buffer.from("direct"));
      }
      return;
    }
    res.writeHead(200);
    res.end(Buffer.from("relative target"));
  });
  try {
    const client = new GithubClient({ token: OTHER, apiUrl: api.url });
    assert.equal(
      (await client.downloadReleaseAsset("o", "r", 1)).toString("utf8"),
      "relative target",
    );
    assert.equal(
      (await client.downloadReleaseAsset("o", "r", 2)).toString("utf8"),
      "direct",
    );
  } finally {
    await api.close();
  }
});

test("downloadReleaseAsset fails on a redirect it does not follow and on an error from the download host", async () => {
  const cdn = await startServer((_req, _body, res) =>
    sendJson(res, 403, { message: "expired" }),
  );
  const api = await startServer((req, _body, res) => {
    if (req.url?.endsWith("/1")) {
      res.writeHead(307, { Location: `${cdn.url}/x` });
    } else {
      res.writeHead(302, { Location: `${cdn.url}/x` });
    }
    res.end();
  });
  try {
    const client = new GithubClient({ token: OTHER, apiUrl: api.url });
    await assert.rejects(
      client.downloadReleaseAsset("o", "r", 1),
      /unexpected redirection response 307/,
    );
    await assert.rejects(
      client.downloadReleaseAsset("o", "r", 2),
      (error: unknown) =>
        error instanceof GithubError &&
        error.status === 403 &&
        error.apiMessage === "expired",
    );
  } finally {
    await api.close();
    await cdn.close();
  }
});

test("downloadReleaseAsset follows a 301 within the API origin with the credential", async () => {
  const api = await startServer((req, _body, res) => {
    if (req.url?.endsWith("/moved")) {
      res.writeHead(200);
      res.end(Buffer.from("moved bytes"));
    } else {
      res.writeHead(301, { Location: "/repos/o/r/releases/assets/moved" });
      res.end();
    }
  });
  try {
    const client = new GithubClient({ token: OTHER, apiUrl: api.url });
    assert.equal(
      (await client.downloadReleaseAsset("o", "r", 1)).toString("utf8"),
      "moved bytes",
    );
    assert.equal(api.requests.length, 2);
    assert.equal(
      header(api.requests[1]?.headers["authorization"]),
      `Bearer ${OTHER}`,
    );
  } finally {
    await api.close();
  }
});

// A redirect is followed with the credential only within the same origin. Comparing the host alone would follow a 301
// that only changes the scheme and send the token in cleartext; ADR 0011 records why the whole origin is compared.
test("downloadReleaseAsset refuses a 301 to another host before it sends the credential again", async () => {
  const elsewhere = await startServer((_req, _body, res) => {
    res.writeHead(200);
    res.end(Buffer.from("never read"));
  });
  const api = await startServer((_req, _body, res) => {
    res.writeHead(301, { Location: `${elsewhere.url}/stolen` });
    res.end();
  });
  try {
    const client = new GithubClient({ token: OTHER, apiUrl: api.url });
    await assert.rejects(
      client.downloadReleaseAsset("o", "r", 1),
      /refusing to follow cross-origin redirect/,
    );
    assert.equal(api.requests.length, 1);
    assert.equal(elsewhere.requests.length, 0);
  } finally {
    await api.close();
    await elsewhere.close();
  }
});

test("downloadReleaseAsset refuses a 301 to the same host over another scheme", async () => {
  const api = await startServer((_req, _body, res) => {
    const downgraded = new URL(api.url);
    downgraded.protocol = "https:";
    res.writeHead(301, { Location: `${downgraded.href}stolen` });
    res.end();
  });
  try {
    const client = new GithubClient({ token: OTHER, apiUrl: api.url });
    await assert.rejects(
      client.downloadReleaseAsset("o", "r", 1),
      (error: unknown) =>
        error instanceof Error &&
        /refusing to follow cross-origin redirect from "http:\/\/127\.0\.0\.1:\d+" to "https:\/\/127\.0\.0\.1:\d+"/.test(
          error.message,
        ),
    );
    assert.equal(api.requests.length, 1);
  } finally {
    await api.close();
  }
});

test("the redirect target is read under downloadTimeoutMs, not the API bound", async () => {
  const cdn = await startServer((_req, _body, res) => holdOpen(res));
  const api = await startServer((_req, _body, res) => {
    res.writeHead(302, { Location: `${cdn.url}/slow` });
    res.end();
  });
  try {
    const client = new GithubClient({
      token: OTHER,
      apiUrl: api.url,
      apiTimeoutMs: 20_000,
      downloadTimeoutMs: 100,
    });
    const started = Date.now();
    await assert.rejects(client.downloadReleaseAsset("o", "r", 1), /timed out/);
    assert.ok(
      Date.now() - started < 5_000,
      "the second request waited for the API bound",
    );
  } finally {
    await api.close();
    await cdn.close();
  }
});

test("a GithubError is named GithubError", async () => {
  const server = await startServer((_req, _body, res) =>
    sendJson(res, 404, { message: "Not Found" }),
  );
  try {
    const client = new GithubClient({ token: OTHER, apiUrl: server.url });
    const error = await client
      .getRelease("o", "r", 1)
      .catch((caught: unknown) => caught);
    assert.ok(error instanceof GithubError);
    assert.equal(error.name, "GithubError");
  } finally {
    await server.close();
  }
});

test("parseNextPage skips a rel=next page that is zero, fractional or past the safe integers", () => {
  const base = "https://api.github.com/repos/o/r/releases";
  const later = `<${base}?page=5>; rel="next"`;
  for (const page of ["0", "1.5", "9007199254740993", "1e400"]) {
    assert.equal(parseNextPage(`<${base}?page=${page}>; rel="next"`), 0, page);
    assert.equal(
      parseNextPage(`<${base}?page=${page}>; rel="next", ${later}`),
      5,
      `${page} then 5`,
    );
  }
  assert.equal(parseNextPage(`<${base}?page=1>; rel="next"`), 1);
});

test("getTag and getPullRequest read their routes and normalize the fields", async () => {
  const sha = "e".repeat(40);
  const server = await startServer((req, _body, res) => {
    if (req.url?.includes("/git/tags/"))
      sendJson(res, 200, {
        sha,
        object: { type: "commit", sha: "f".repeat(40) },
      });
    else
      sendJson(res, 200, {
        number: 12,
        html_url: "https://github.com/o/r/pull/12",
        title: "release v1.2.3",
        body: "the notes",
      });
  });
  try {
    const client = new GithubClient({ token: OTHER, apiUrl: server.url });
    assert.deepEqual(await client.getTag("o", "r", sha), {
      sha,
      object: { type: "commit", sha: "f".repeat(40) },
    });
    assert.deepEqual(await client.getPullRequest("o", "r", 12), {
      number: 12,
      html_url: "https://github.com/o/r/pull/12",
      title: "release v1.2.3",
      body: "the notes",
    });
    assert.deepEqual(
      server.requests.map((request) => `${request.method} ${request.url}`),
      [`GET /repos/o/r/git/tags/${sha}`, "GET /repos/o/r/pulls/12"],
    );
  } finally {
    await server.close();
  }
});

test("listMatchingRefs drops only a leading refs/ prefix", async () => {
  const server = await startServer((_req, _body, res) =>
    sendJson(res, 200, []),
  );
  try {
    const client = new GithubClient({ token: OTHER, apiUrl: server.url });
    await client.listMatchingRefs("o", "r", "refs/tags/untagged-");
    await client.listMatchingRefs("o", "r", "tags/refs/x");
    assert.deepEqual(
      server.requests.map((request) => request.url),
      [
        "/repos/o/r/git/matching-refs/tags/untagged-",
        "/repos/o/r/git/matching-refs/tags/refs/x",
      ],
    );
  } finally {
    await server.close();
  }
});

test("a request with no body sends no Content-Type", async () => {
  const server = await startServer((_req, _body, res) =>
    sendJson(res, 200, { id: 1 }),
  );
  try {
    const client = new GithubClient({ token: OTHER, apiUrl: server.url });
    await client.getRelease("o", "r", 1);
    assert.equal(server.requests[0]?.headers["content-type"], undefined);
  } finally {
    await server.close();
  }
});

test("an API answer is a success from 200 through 299 and an error from 300 up when redirects are followed", async () => {
  const server = await startServer((req, _body, res) => {
    const status = Number(req.url?.split("/").at(-1));
    sendJson(res, status, { id: status, message: `status ${status}` });
  });
  try {
    const client = new GithubClient({ token: OTHER, apiUrl: server.url });
    assert.equal((await client.getRelease("o", "r", 299)).id, 299);
    for (const status of [300, 399]) {
      await assert.rejects(
        client.getRelease("o", "r", status),
        (error: unknown) =>
          error instanceof GithubError &&
          error.status === status &&
          error.message ===
            `GET ${server.url}/repos/o/r/releases/${status}: ${status} status ${status} []`,
      );
    }
  } finally {
    await server.close();
  }
});

test("downloadReleaseAsset treats 300 through 399 as a redirect and fails one with no Location", async () => {
  const api = await startServer((req, _body, res) => {
    const id = Number(req.url?.split("/").at(-1));
    // id 3020 stands for a 302 whose Location header is present but empty.
    if (id === 3020) res.writeHead(302, { Location: "" });
    else res.writeHead(id);
    res.end(Buffer.from("raw"));
  });
  try {
    const client = new GithubClient({ token: OTHER, apiUrl: api.url });
    for (const [id, status] of [
      [300, 300],
      [399, 399],
      [3020, 302],
    ] as const) {
      await assert.rejects(client.downloadReleaseAsset("o", "r", id), {
        message: `GET ${api.url}/repos/o/r/releases/assets/${id}: ${status} redirect with no Location`,
      });
    }
    assert.equal(
      (await client.downloadReleaseAsset("o", "r", 299)).toString("utf8"),
      "raw",
    );
    await assert.rejects(
      client.downloadReleaseAsset("o", "r", 404),
      (error: unknown) => error instanceof GithubError && error.status === 404,
    );
    assert.equal(
      api.requests.length,
      5,
      "a redirect with no Location is not followed",
    );
  } finally {
    await api.close();
  }
});

test("an error from the download host names GET and the download URL", async () => {
  const cdn = await startServer((_req, _body, res) =>
    sendJson(res, 403, { message: "expired" }),
  );
  const api = await startServer((_req, _body, res) => {
    res.writeHead(302, { Location: `${cdn.url}/x` });
    res.end();
  });
  try {
    const client = new GithubClient({ token: OTHER, apiUrl: api.url });
    await assert.rejects(client.downloadReleaseAsset("o", "r", 2), {
      name: "GithubError",
      message: `GET ${cdn.url}/x: 403 expired []`,
    });
  } finally {
    await api.close();
    await cdn.close();
  }
});

test("an error detail list keeps each message and reads a null entry as empty fields", async () => {
  const server = await startServer((_req, _body, res) =>
    sendJson(res, 422, {
      message: "Validation Failed",
      errors: [null, { code: "custom", message: "tag is taken" }],
    }),
  );
  try {
    const client = new GithubClient({ token: OTHER, apiUrl: server.url });
    const error = await client
      .getRelease("o", "r", 1)
      .catch((caught: unknown) => caught);
    assert.ok(error instanceof GithubError);
    assert.deepEqual(error.details, [
      { resource: "", field: "", code: "", message: "" },
      { resource: "", field: "", code: "custom", message: "tag is taken" },
    ]);
  } finally {
    await server.close();
  }
});

test("a response that is not a JSON object fails with the response-shape error", async () => {
  for (const shape of [null, [], "text", 7]) {
    const server = await startServer((_req, _body, res) =>
      sendJson(res, 200, shape),
    );
    try {
      const client = new GithubClient({ token: OTHER, apiUrl: server.url });
      await assert.rejects(
        client.getRelease("o", "r", 1),
        { message: "unexpected GitHub response shape" },
        JSON.stringify(shape),
      );
    } finally {
      await server.close();
    }
  }
});

test("a non-number id, a null git object and a release body read as their JSON says", async () => {
  const server = await startServer((req, _body, res) => {
    if (req.url?.includes("/git/ref/"))
      sendJson(res, 200, { ref: "refs/tags/v1", object: null });
    else sendJson(res, 200, { id: "42", body: "release notes" });
  });
  try {
    const client = new GithubClient({ token: OTHER, apiUrl: server.url });
    const release = await client.getRelease("o", "r", 42);
    assert.equal(release.id, 0, "a string id is not a number");
    assert.equal(release.body, "release notes");
    assert.deepEqual(await client.getRef("o", "r", "tags/v1"), {
      ref: "refs/tags/v1",
      object: { type: "", sha: "" },
    });
  } finally {
    await server.close();
  }
});
