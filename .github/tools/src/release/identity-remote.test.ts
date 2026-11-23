import assert from "node:assert/strict";
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { test } from "node:test";
import { expectedAssetNames, tarballName } from "./assets.ts";
import { GithubError, type Release } from "./github.ts";
import {
  alreadyExists,
  PinnedRelease,
  discoverRelease,
  releaseByID,
  tagTarget,
  uploadAsset,
  verifyAllAssets,
  verifyRemoteAsset,
} from "./identity.ts";
import {
  OTHER_SHA,
  STAGE_REPO,
  STAGE_SHA,
  STAGE_TAG,
  countRequests,
  newFakeRelease,
  startReleaseFake,
  testClient,
  uploadedNames,
  writeSyntheticBundle,
} from "./testing/release-fake.ts";
import { scratchDir, startServer } from "./testing/support.ts";

const OWNER = "muehz";
const NAME = "agentics";
const ROUTE = `/repos/${STAGE_REPO}`;
const ANNOTATED = `beef${"0".repeat(36)}`;

function bundleDir(): string {
  const dir = join(scratchDir("agentics-identity-"), "assets");
  writeSyntheticBundle(dir, STAGE_TAG);
  return dir;
}

async function rejection(promise: Promise<unknown>): Promise<unknown> {
  return promise.then(
    () => undefined,
    (error: unknown) => error,
  );
}

function githubError(
  status: number,
  apiMessage: string,
  details: GithubError["details"] = [],
): GithubError {
  return new GithubError("POST", "http://x/y", status, apiMessage, details);
}

test("tagTarget reports absence, a lightweight tag, and an annotated tag peeled to its commit", async () => {
  const started = await startReleaseFake();
  try {
    const client = testClient(started.url);
    assert.deepEqual(await tagTarget(client, OWNER, NAME, STAGE_TAG), {
      sha: "",
      present: false,
    });

    started.fake.tagObject = { type: "commit", sha: STAGE_SHA };
    assert.deepEqual(await tagTarget(client, OWNER, NAME, STAGE_TAG), {
      sha: STAGE_SHA,
      present: true,
    });

    started.fake.tagObject = { type: "tag", sha: ANNOTATED };
    started.fake.annotatedTags.set(ANNOTATED, OTHER_SHA);
    assert.deepEqual(await tagTarget(client, OWNER, NAME, STAGE_TAG), {
      sha: OTHER_SHA,
      present: true,
    });
  } finally {
    await started.close();
  }
});

test("tagTarget fails with the peel message when the tag object cannot be read", async () => {
  const started = await startReleaseFake();
  try {
    started.fake.tagObject = { type: "tag", sha: ANNOTATED };
    await assert.rejects(
      tagTarget(testClient(started.url), OWNER, NAME, STAGE_TAG),
      { message: `cannot peel observed tag ${STAGE_TAG}` },
    );
  } finally {
    await started.close();
  }
});

test("tagTarget does not read a failing tag route as an absent tag", async () => {
  const started = await startReleaseFake();
  try {
    started.fake.tagRefStatus = 500;
    const error = await rejection(
      tagTarget(testClient(started.url), OWNER, NAME, STAGE_TAG),
    );
    assert.ok(
      error instanceof GithubError && error.status === 500,
      String(error),
    );
  } finally {
    await started.close();
  }
});

test("alreadyExists needs a 422 whose message or error code says the reference exists", () => {
  assert.equal(
    alreadyExists(githubError(422, "Reference already exists")),
    true,
  );
  assert.equal(
    alreadyExists(githubError(422, "REFERENCE ALREADY EXISTS")),
    true,
  );
  assert.equal(
    alreadyExists(
      githubError(422, "Validation Failed", [
        {
          resource: "Reference",
          field: "ref",
          code: "already_exists",
          message: "",
        },
      ]),
    ),
    true,
  );
  assert.equal(alreadyExists(githubError(422, "Validation Failed")), false);
  assert.equal(
    alreadyExists(githubError(409, "Reference already exists")),
    false,
  );
  assert.equal(
    alreadyExists(githubError(500, "Reference already exists")),
    false,
  );
  assert.equal(alreadyExists(new Error("422 Reference already exists")), false);
  assert.equal(alreadyExists(undefined), false);
});

test("releaseByID returns a release, undefined for a 404, and fails on any other status", async () => {
  const started = await startReleaseFake();
  try {
    const client = testClient(started.url);
    started.fake.addRelease({
      id: 42,
      tagName: STAGE_TAG,
      name: STAGE_TAG,
      draft: true,
    });
    assert.equal((await releaseByID(client, OWNER, NAME, 42))?.id, 42);
    assert.equal(await releaseByID(client, OWNER, NAME, 7), undefined);

    // A 500 whose body says 404 must stay fatal.
    started.fake.readByIdCodes = [500];
    const error = await rejection(releaseByID(client, OWNER, NAME, 42));
    assert.ok(
      error instanceof GithubError &&
        error.status === 500 &&
        error.message.includes("404"),
      String(error),
    );
  } finally {
    await started.close();
  }
});

test("a pinned upload reread uses the earlier tag facts and reads the same id once", async (t) => {
  const started = await startReleaseFake();
  t.after(() => started.close());
  const target = { owner: OWNER, name: NAME, tag: STAGE_TAG, sha: STAGE_SHA };
  const draft = started.fake.addRelease({
    id: 42,
    tagName: STAGE_TAG,
    name: STAGE_TAG,
    draft: true,
    targetCommitish: STAGE_SHA,
  });
  started.fake.tagObject = { type: "commit", sha: STAGE_SHA };
  const { found } = await PinnedRelease.discover(
    testClient(started.url),
    target,
  );
  assert.ok(found);
  const before = started.seen().length;
  started.fake.tagObject = null;
  draft.draft = false;
  started.fake.addAllAssets(draft, bundleDir(), STAGE_TAG);
  const next = await found.rereadAfterUpload();
  assert.equal(next.current.draft, false);
  assert.equal(next.id, 42);
  assert.deepEqual(next.tagReading, { present: true, sha: STAGE_SHA });
  started.fake.readByIdCodes = [404, 200];
  await assert.rejects(next.rereadAfterUpload(), {
    message: `release ${STAGE_TAG} disappeared after asset upload`,
  });
  assert.deepEqual(
    started.seen().slice(before),
    Array(2).fill({
      method: "GET",
      path: `${ROUTE}/releases/42`,
      query: "",
    }),
  );
});

test("discoverRelease finds a published release by tag without reading the listing", async () => {
  const started = await startReleaseFake();
  try {
    started.fake.addRelease({
      id: 42,
      tagName: STAGE_TAG,
      name: STAGE_TAG,
      targetCommitish: STAGE_SHA,
    });
    const found = await discoverRelease(
      testClient(started.url),
      OWNER,
      NAME,
      STAGE_TAG,
    );
    assert.equal(found?.id, 42);
    assert.deepEqual(
      started.seen().map((request) => `${request.method} ${request.path}`),
      [`GET ${ROUTE}/releases/tags/${STAGE_TAG}`],
    );
  } finally {
    await started.close();
  }
});

test("discoverRelease finds a draft through the listing and rereads it by the id the listing pins", async () => {
  const started = await startReleaseFake();
  try {
    started.fake.addRelease({
      id: 42,
      tagName: STAGE_TAG,
      name: STAGE_TAG,
      draft: true,
      targetCommitish: STAGE_SHA,
    });
    const found = await discoverRelease(
      testClient(started.url),
      OWNER,
      NAME,
      STAGE_TAG,
    );
    assert.equal(found?.id, 42);
    assert.deepEqual(started.seen(), [
      { method: "GET", path: `${ROUTE}/releases/tags/${STAGE_TAG}`, query: "" },
      { method: "GET", path: `${ROUTE}/releases`, query: "per_page=100" },
      { method: "GET", path: `${ROUTE}/releases/42`, query: "" },
    ]);
  } finally {
    await started.close();
  }
});

test("discoverRelease follows the Link header across pages and returns undefined when no page matches", async () => {
  const server = await startServer((req, _body, res) => {
    const url = new URL(req.url ?? "/", "http://x");
    res.setHeader("Content-Type", "application/json");
    if (url.pathname.includes("/releases/tags/")) {
      res.writeHead(404);
      res.end(JSON.stringify({ message: "Not Found" }));
    } else if (url.pathname.endsWith("/releases/42")) {
      res.end(
        JSON.stringify({
          id: 42,
          tag_name: STAGE_TAG,
          name: STAGE_TAG,
          draft: true,
        }),
      );
    } else if (url.searchParams.get("page") === "2") {
      res.end(
        JSON.stringify([
          { id: 42, tag_name: STAGE_TAG, name: STAGE_TAG, draft: true },
        ]),
      );
    } else if (url.searchParams.get("page") === "3") {
      res.end("[]");
    } else {
      res.setHeader(
        "Link",
        '<http://127.0.0.1/x?page=2&per_page=100>; rel="next"',
      );
      res.end(JSON.stringify([{ id: 1, tag_name: "v0.0.1", name: "v0.0.1" }]));
    }
  });
  try {
    const client = testClient(server.url);
    assert.equal(
      (await discoverRelease(client, OWNER, NAME, STAGE_TAG))?.id,
      42,
    );
    assert.deepEqual(
      server.requests.map((request) => request.url),
      [
        `${ROUTE}/releases/tags/${STAGE_TAG}`,
        `${ROUTE}/releases?per_page=100`,
        `${ROUTE}/releases?page=2&per_page=100`,
        `${ROUTE}/releases/42`,
      ],
    );
    assert.equal(
      await discoverRelease(client, OWNER, NAME, "v9.9.9"),
      undefined,
    );
  } finally {
    await server.close();
  }
});

test("discoverRelease does not fall back to the listing when the by-tag read fails for any reason but a 404", async () => {
  const server = await startServer((_req, _body, res) => {
    res.setHeader("Content-Type", "application/json");
    res.writeHead(500);
    res.end(JSON.stringify({ message: "tag read unavailable" }));
  });
  try {
    const error = await rejection(
      discoverRelease(testClient(server.url), OWNER, NAME, STAGE_TAG),
    );
    assert.ok(
      error instanceof GithubError && error.status === 500,
      String(error),
    );
    assert.deepEqual(
      server.requests.map((request) => request.url),
      [`${ROUTE}/releases/tags/${STAGE_TAG}`],
    );
  } finally {
    await server.close();
  }
});

test("discoverRelease treats a placeholder draft named for another tag as a stranger", async () => {
  const started = await startReleaseFake();
  try {
    started.fake.placeholderDraftTags = true;
    started.fake.addRelease({
      id: 42,
      tagName: "v9.9.9",
      name: "v9.9.9",
      draft: true,
    });
    assert.equal(
      await discoverRelease(testClient(started.url), OWNER, NAME, STAGE_TAG),
      undefined,
    );
    started.fake.addRelease({
      id: 43,
      tagName: STAGE_TAG,
      name: STAGE_TAG,
      draft: true,
    });
    assert.equal(
      (await discoverRelease(testClient(started.url), OWNER, NAME, STAGE_TAG))
        ?.id,
      43,
    );
  } finally {
    await started.close();
  }
});

test("discoverRelease refuses two releases that match the tag", async () => {
  const started = await startReleaseFake();
  try {
    started.fake.addRelease({
      id: 42,
      tagName: STAGE_TAG,
      name: STAGE_TAG,
      draft: true,
    });
    started.fake.addRelease({
      id: 43,
      tagName: STAGE_TAG,
      name: STAGE_TAG,
      draft: true,
    });
    await assert.rejects(
      discoverRelease(testClient(started.url), OWNER, NAME, STAGE_TAG),
      {
        message: `multiple releases match ${STAGE_TAG}; refusing ambiguous draft recovery`,
      },
    );
  } finally {
    await started.close();
  }
});

test("discoverRelease refuses a listed release that has no valid id", async () => {
  const started = await startReleaseFake();
  try {
    started.fake.releases.push(
      newFakeRelease({
        id: 0,
        tagName: STAGE_TAG,
        name: STAGE_TAG,
        draft: true,
        targetCommitish: STAGE_SHA,
      }),
    );
    await assert.rejects(
      discoverRelease(testClient(started.url), OWNER, NAME, STAGE_TAG),
      {
        message: `observed release ${STAGE_TAG} has no valid id`,
      },
    );
  } finally {
    await started.close();
  }
});

test("discoverRelease treats a listed release that 404s on the reread as a disappearance", async () => {
  const started = await startReleaseFake();
  try {
    started.fake.addRelease({
      id: 42,
      tagName: STAGE_TAG,
      name: STAGE_TAG,
      draft: true,
    });
    started.fake.readByIdCodes = [404];
    await assert.rejects(
      discoverRelease(testClient(started.url), OWNER, NAME, STAGE_TAG),
      {
        message: `release ${STAGE_TAG} (id 42) was listed but could not be reread`,
      },
    );
  } finally {
    await started.close();
  }
});

test("discoverRelease has no fallback when the listing fails", async () => {
  const started = await startReleaseFake();
  try {
    started.fake.listStatus = 500;
    const error = await rejection(
      discoverRelease(testClient(started.url), OWNER, NAME, STAGE_TAG),
    );
    assert.ok(
      error instanceof GithubError && error.status === 500,
      String(error),
    );
    assert.equal(
      started.seen().filter((request) => request.method !== "GET").length,
      0,
    );
  } finally {
    await started.close();
  }
});

test("verifyRemoteAsset passes matching bytes, fails other bytes, and follows a redirect to read them", async () => {
  const started = await startReleaseFake();
  try {
    const dir = bundleDir();
    const client = testClient(started.url);
    const release = started.fake.addRelease({
      id: 42,
      tagName: STAGE_TAG,
      name: STAGE_TAG,
      draft: true,
    });
    started.fake.addAsset(release, dir, "THIRD_PARTY_NOTICES.md");
    for (const redirect of [false, true]) {
      started.fake.redirectAssets = redirect;
      const current = await client.getRelease(OWNER, NAME, 42);
      await verifyRemoteAsset(
        client,
        OWNER,
        NAME,
        current,
        dir,
        STAGE_TAG,
        "THIRD_PARTY_NOTICES.md",
      );
    }
    assert.ok(
      countRequests(started.seen(), "GET", "/release-asset-bytes/1000") >= 1,
      "the redirect was never followed",
    );

    release.assets[0]!.contents = Buffer.from(
      "bytes that are not the local asset\n",
    );
    for (const redirect of [false, true]) {
      started.fake.redirectAssets = redirect;
      const current = await client.getRelease(OWNER, NAME, 42);
      await assert.rejects(
        verifyRemoteAsset(
          client,
          OWNER,
          NAME,
          current,
          dir,
          STAGE_TAG,
          "THIRD_PARTY_NOTICES.md",
        ),
        {
          message: `observed asset THIRD_PARTY_NOTICES.md on release ${STAGE_TAG} conflicts; publish a new patch release for immutable content`,
        },
      );
    }
  } finally {
    await started.close();
  }
});

test("verifyRemoteAsset fails without a download when the roster has no single asset of the name", async () => {
  const started = await startReleaseFake();
  try {
    const dir = bundleDir();
    const client = testClient(started.url);
    const empty: Release = {
      id: 42,
      tag_name: STAGE_TAG,
      name: STAGE_TAG,
      body: "",
      draft: true,
      prerelease: false,
      target_commitish: STAGE_SHA,
      assets: [],
    };
    await assert.rejects(
      verifyRemoteAsset(
        client,
        OWNER,
        NAME,
        empty,
        dir,
        STAGE_TAG,
        "SHA256SUMS",
      ),
      /observed asset SHA256SUMS on release v1.2.3 conflicts/,
    );
    assert.equal(started.seen().length, 0);
  } finally {
    await started.close();
  }
});

test("verifyAllAssets compares the ten assets in roster order and stops at the first conflict", async () => {
  const started = await startReleaseFake();
  try {
    const dir = bundleDir();
    const client = testClient(started.url);
    const release = started.fake.addRelease({
      id: 42,
      tagName: STAGE_TAG,
      name: STAGE_TAG,
    });
    started.fake.addAllAssets(release, dir, STAGE_TAG);
    assert.equal(release.assets.length, 10);
    await verifyAllAssets(
      client,
      OWNER,
      NAME,
      await client.getRelease(OWNER, NAME, 42),
      dir,
      STAGE_TAG,
    );

    release.assets[1]!.contents = Buffer.from(
      "bytes that are not the local asset\n",
    );
    const name = release.assets[1]!.name;
    const before = started.seen().length;
    await assert.rejects(
      verifyAllAssets(
        client,
        OWNER,
        NAME,
        await client.getRelease(OWNER, NAME, 42),
        dir,
        STAGE_TAG,
      ),
      {
        message: `observed asset ${name} on release ${STAGE_TAG} conflicts; publish a new patch release for immutable content`,
      },
    );
    const downloads = started
      .seen()
      .slice(before)
      .filter((request) =>
        request.path.startsWith(`${ROUTE}/releases/assets/`),
      );
    assert.equal(
      downloads.length,
      2,
      "the first asset is read, the second conflicts, the rest are never read",
    );
  } finally {
    await started.close();
  }
});

test("verifyAllAssets names the first expected asset the roster lacks, before any download", async () => {
  const started = await startReleaseFake();
  try {
    const dir = bundleDir();
    const client = testClient(started.url);
    const release = started.fake.addRelease({
      id: 42,
      tagName: STAGE_TAG,
      name: STAGE_TAG,
    });
    started.fake.addAllAssets(release, dir, STAGE_TAG);
    const [first, second] = expectedAssetNames(STAGE_TAG);
    release.assets = release.assets.filter((asset) => asset.name !== second);
    await assert.rejects(
      verifyAllAssets(
        client,
        OWNER,
        NAME,
        await client.getRelease(OWNER, NAME, 42),
        dir,
        STAGE_TAG,
      ),
      {
        message: `observed asset ${second} on release ${STAGE_TAG} conflicts; publish a new patch release for immutable content`,
      },
    );
    const downloads = started
      .seen()
      .filter((request) =>
        request.path.startsWith(`${ROUTE}/releases/assets/`),
      );
    assert.equal(
      downloads.length,
      1,
      `only ${first} is read before ${second} is found missing`,
    );
  } finally {
    await started.close();
  }
});

test("uploadAsset posts the local file's bytes under its name and never lists or replaces", async () => {
  const started = await startReleaseFake();
  try {
    const dir = bundleDir();
    const release = started.fake.addRelease({
      id: 42,
      tagName: STAGE_TAG,
      name: STAGE_TAG,
      draft: true,
    });
    const name = tarballName(STAGE_TAG);
    await uploadAsset(testClient(started.url), OWNER, NAME, 42, dir, name);
    assert.deepEqual(uploadedNames(started.seen()), [name]);
    assert.equal(release.assets.length, 1);
    assert.deepEqual(
      release.assets[0]?.contents,
      readFileSync(join(dir, name)),
    );
    assert.deepEqual(
      started.seen().map((request) => `${request.method} ${request.path}`),
      [`POST ${ROUTE}/releases/42/assets`],
    );
  } finally {
    await started.close();
  }
});

test("uploadAsset fails with the filesystem error and sends nothing when the local file is missing", async () => {
  const started = await startReleaseFake();
  try {
    const dir = join(scratchDir("agentics-identity-"), "empty");
    mkdirSync(dir);
    writeFileSync(join(dir, "other"), "x");
    const error = await rejection(
      uploadAsset(testClient(started.url), OWNER, NAME, 42, dir, "SHA256SUMS"),
    );
    assert.ok(
      error instanceof Error && /ENOENT/.test(error.message),
      String(error),
    );
    assert.equal(started.seen().length, 0);
  } finally {
    await started.close();
  }
});
