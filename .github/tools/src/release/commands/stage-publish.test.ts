import assert from "node:assert/strict";
import { writeFileSync } from "node:fs";
import { test } from "node:test";
import { expectedAssetNames, tarballName } from "../assets.ts";
import {
  ASSET_BYTES_ROUTE,
  OTHER_SHA,
  STAGE_REPO,
  STAGE_TAG,
  countRequests,
  countUploads,
  placeholderTagFor,
  uploadedNames,
  wireTagName,
  type SeenRequest,
} from "../testing/release-fake.ts";
import {
  PUBLISHED_BODY,
  STAGE_NOTES,
  assertNoBodyLeak,
  assertNoMethod,
  assertNoOutputs,
  assertOutputs,
  newStageFixture,
  type StageFixture,
} from "../testing/stage-fixture.ts";
test("stage preserves failure precedence and stops at the rejected observation", async (t) => {
  const createRequests = wantStageCreateRequests(100);
  const beforePublishedAssetReads = createRequests.slice(
    0,
    -expectedAssetNames(STAGE_TAG).length,
  );
  const cases: {
    name: string;
    setup(fixture: StageFixture): void;
    diagnostic: RegExp;
    requests: SeenRequest[];
    created?: true;
  }[] = [
    {
      name: "published release without tag or assets",
      requests: createRequests.slice(0, 2),
      diagnostic: /observed tag v1\.2\.3 does not target/,
      setup: (fixture) => {
        fixture.published();
        fixture.fake.tagObject = null;
      },
    },
    {
      name: "create response has wrong target and draft state",
      requests: createRequests.slice(0, 6),
      diagnostic: /has targetCommitish/,
      setup: ({ fake }) => {
        fake.createReturns = { target_commitish: OTHER_SHA, draft: false };
      },
    },
    {
      name: "created reread has wrong draft state and moved tag",
      requests: createRequests.slice(0, 7),
      diagnostic: /observed release v1\.2\.3 has conflicting identity/,
      setup: ({ fake }) => {
        fake.afterCreate = (state) => {
          state.releases[0]!.draft = false;
          state.tagObject = { type: "commit", sha: OTHER_SHA };
        };
        fake.createReturns = { draft: true };
      },
    },
    {
      name: "created reread has wrong id and roster",
      requests: createRequests.slice(0, 8),
      created: true,
      diagnostic: /pinned release id 100 returned release id 101/,
      setup: ({ fake }) => {
        fake.afterCreate = (state) => {
          const other = state.addRelease({
            id: 101,
            draft: true,
            name: STAGE_TAG,
            tagName: STAGE_TAG,
            targetCommitish: OTHER_SHA,
          });
          other.assets.push({
            id: 200,
            name: "unexpected",
            contents: Buffer.alloc(0),
          });
          state.readByIdReturns = other.id;
        };
      },
    },
    {
      name: "roster changes after upload",
      requests: createRequests.slice(0, 10),
      created: true,
      diagnostic: /observed asset unexpected on release/,
      setup: ({ fake }) => {
        fake.afterUpload = (state) => {
          state.releases[0]!.assets[0]!.name = "unexpected";
        };
      },
    },
    {
      name: "tag moves and published identity and kind conflict",
      requests: beforePublishedAssetReads,
      diagnostic: /published under placeholder tag untagged-123/,
      setup: ({ fake }) => {
        fake.afterPatch = (state) => {
          const release = state.releases[0]!;
          release.tagName = "untagged-123";
          release.prerelease = true;
          release.assets = [];
          state.tagObject = null;
        };
      },
    },
    {
      name: "published kind and roster both conflict",
      requests: beforePublishedAssetReads,
      diagnostic: /is not a published release under its tag/,
      setup: ({ fake }) => {
        fake.afterPatch = (state) => {
          state.releases[0]!.prerelease = true;
          state.releases[0]!.assets = [];
        };
      },
    },
  ];
  for (const item of cases) {
    await t.test(item.name, async (t) => {
      const fixture = await newStageFixture(t);
      item.setup(fixture);
      const result = await fixture.run();

      assert.equal(result.code, 1, result.stderr);
      assert.match(result.stderr, item.diagnostic);
      assert.deepEqual(fixture.seen(), item.requests);
      assertNoOutputs(fixture.output);
      if (item.created) {
        assert.match(
          result.stderr,
          /created draft 100 for v1\.2\.3\nrelease: stage:/,
        );
      }
    });
  }
});

const ROUTE = `/repos/${STAGE_REPO}`;
const NOTICES = "THIRD_PARTY_NOTICES.md";
const CHECKSUMS = "SHA256SUMS";
const ANNOTATED = `beef${"0".repeat(36)}`;
const LATEST: SeenRequest = {
  method: "GET",
  path: `${ROUTE}/releases/latest`,
  query: "",
};

/**
 * The request sequence a stage run sends when the repository holds no release for the tag yet. The reads are as
 * much of the contract as the writes. The tag is created before the draft (#620), and that create is followed
 * by a fresh tag read because the first tag read is now older than it. The draft create is followed by a read of
 * the returned id and another fresh tag read. Each upload is followed by a read of the release and a read of
 * the asset it just wrote, so a release that vanished mid-run stops the loop. The final read of fillDraft is
 * followed by a comparison of all ten published assets, then the publishing PATCH, its own reread, its own tag
 * read, and its own comparison of all ten assets.
 */
function wantStageCreateRequests(id: number): SeenRequest[] {
  const release = `${ROUTE}/releases/${id}`;
  const readTag: SeenRequest = {
    method: "GET",
    path: `${ROUTE}/git/ref/tags/${STAGE_TAG}`,
    query: "",
  };
  const readRelease: SeenRequest = { method: "GET", path: release, query: "" };
  const want: SeenRequest[] = [
    readTag,
    // Discovery asks by tag first, and a draft never answers there, so the listing is the read that finds one.
    { method: "GET", path: `${ROUTE}/releases/tags/${STAGE_TAG}`, query: "" },
    { method: "GET", path: `${ROUTE}/releases`, query: "per_page=100" },
    { method: "POST", path: `${ROUTE}/git/refs`, query: "" },
    readTag,
    { method: "POST", path: `${ROUTE}/releases`, query: "" },
    readRelease,
    readTag,
  ];
  // The fake numbers each new asset from 1000, so the assets carry the upload order.
  let assetId = 1000;
  const readAssets: SeenRequest[] = [];
  for (const name of expectedAssetNames(STAGE_TAG)) {
    const readAsset: SeenRequest = {
      method: "GET",
      path: `${ROUTE}/releases/assets/${assetId}`,
      query: "",
    };
    want.push(
      { method: "POST", path: `${release}/assets`, query: `name=${name}` },
      readRelease,
      readAsset,
    );
    readAssets.push(readAsset);
    assetId++;
  }
  want.push(readRelease, ...readAssets);
  want.push(
    { method: "PATCH", path: release, query: "" },
    readRelease,
    readTag,
    ...readAssets,
  );
  return want;
}

/** Proves a run sent exactly one PATCH, the publication, and that it never carried a body. */
function assertOnePublishingPatch(fixture: StageFixture): void {
  assert.equal(
    fixture.fake.patched.length,
    1,
    "stage must send exactly one PATCH",
  );
  assert.equal(
    "body" in (fixture.fake.patched[0] ?? {}),
    false,
    "the publishing PATCH must not send body",
  );
}

test("stage retries the read after a create and stops retrying once it succeeds", async (t) => {
  const fixture = await newStageFixture(t);
  // The create answers with id 100. The first two reads of it answer 404, which is the lag that failed the
  // v1.95.0 and v1.96.0 runs.
  fixture.fake.readByIdCodes = [404, 404];
  const result = await fixture.run();
  assert.equal(result.code, 0, result.stderr);
  for (const attempt of [1, 2]) {
    assert.ok(
      result.stderr.includes(
        `release: stage: draft 100 not readable yet, attempt ${attempt} of 3`,
      ),
      result.stderr,
    );
  }
  assert.equal(result.stderr.split("not readable yet").length - 1, 2);
  assert.ok(
    !result.stderr.includes("attempt 3 of 3"),
    "stage retried after the read succeeded",
  );
  assertOutputs(fixture.output, { release_id: "100", published: "false" });
});

test("stage accepts --prerelease true and --prerelease false", async (t) => {
  for (const value of ["true", "false"]) {
    const fixture = await newStageFixture(t);
    const result = await fixture.run(["--prerelease", value]);
    assert.equal(result.code, 0, `${value}: ${result.stderr}`);
  }
});

test("stage creates the draft from the notes file and sends the whole request sequence in order", async (t) => {
  const fixture = await newStageFixture(t);
  const result = await fixture.run();
  assert.equal(result.code, 0, result.stderr);
  assert.deepEqual(fixture.fake.created, [
    {
      tag_name: STAGE_TAG,
      name: STAGE_TAG,
      target_commitish: fixture.sha,
      body: STAGE_NOTES,
      draft: true,
      prerelease: true,
    },
  ]);
  // The order is the guarantee: a create is read back before anything is uploaded onto it, and every upload is
  // read back before the next one. A test that only counted requests would pass on a run that uploaded first.
  assert.deepEqual(fixture.seen(), [...wantStageCreateRequests(100), LATEST]);
  assertOnePublishingPatch(fixture);
  assertOutputs(fixture.output, { release_id: "100", published: "false" });
});

test("stage sends the notes file's text unchanged, a byte-order mark included", async (t) => {
  const fixture = await newStageFixture(t);
  // U+FEFF, an accented letter and a snowman: the body must survive the decode and the JSON round trip.
  const text = `${String.fromCodePoint(0xfeff)}## Notes\n\nA caf${String.fromCodePoint(0xe9)} ${String.fromCodePoint(0x2603)}\n`;
  writeFileSync(fixture.notes, text, "utf8");
  const result = await fixture.run();
  assert.equal(result.code, 0, result.stderr);
  assert.equal(fixture.fake.created[0]?.["body"], text);
});

test("stage leaves the body of an existing draft alone and publishes it", async (t) => {
  const fixture = await newStageFixture(t);
  // The draft carries a body a human edited after staging. The publishing PATCH never carries body.
  const reviewed = "## Reviewed\n\nA human rewrote these notes.\n";
  fixture.draft().body = reviewed;
  const result = await fixture.run();
  assert.equal(result.code, 0, result.stderr);
  assertOnePublishingPatch(fixture);
  assert.equal(
    fixture.fake.created.length,
    0,
    "stage created a release beside the existing draft",
  );
  assert.equal(fixture.fake.release(42)?.body, reviewed);
  assertOutputs(fixture.output, { release_id: "42", published: "false" });
});

test("stage accepts an annotated tag that peels to the target", async (t) => {
  const fixture = await newStageFixture(t);
  fixture.fake.tagObject = { type: "tag", sha: ANNOTATED };
  fixture.fake.annotatedTags.set(ANNOTATED, fixture.sha);
  const result = await fixture.run();
  assert.equal(result.code, 0, result.stderr);
});

test("stage finds an existing draft through the listing and does not create another", async (t) => {
  const fixture = await newStageFixture(t);
  fixture.draft();
  const result = await fixture.run();
  assert.equal(result.code, 0, result.stderr);
  // The by-tag endpoint answers with published releases only, so the listing is the only read that can find a draft.
  assert.ok(
    countRequests(fixture.seen(), "GET", `${ROUTE}/releases`) > 0,
    "stage never read the release listing",
  );
  assert.equal(
    fixture.fake.created.length,
    0,
    "stage created a second release beside the discovered draft",
  );
  assertOutputs(fixture.output, { release_id: "42", published: "false" });
});

test("stage keeps writing to the pinned release when a duplicate appears mid-run", async (t) => {
  const fixture = await newStageFixture(t);
  fixture.draft();
  // The duplicate appears after discovery, in the middle of the upload loop. Discovery pinned id 42, and every
  // read after it names that id, so the run must ignore the second release.
  const duplicateId = 43;
  fixture.fake.afterUpload = (fake) => {
    if (fake.releases.some((release) => release.id === duplicateId)) return;
    fake.addRelease({
      id: duplicateId,
      tagName: STAGE_TAG,
      name: STAGE_TAG,
      draft: true,
      targetCommitish: fixture.sha,
    });
  };
  const result = await fixture.run();
  assert.equal(result.code, 0, result.stderr);
  assertOutputs(fixture.output, { release_id: "42", published: "false" });
  const duplicate = fixture.fake.release(duplicateId);
  assert.ok(
    duplicate !== undefined,
    "the duplicate never appeared, so this run proved nothing",
  );
  assert.equal(duplicate.assets.length, 0);
  assert.equal(
    countRequests(fixture.seen(), "POST", `${ROUTE}/releases/42/assets`),
    expectedAssetNames(STAGE_TAG).length,
  );
  assert.equal(fixture.fake.created.length, 0);
});

test("stage uploads only the assets a draft is missing and never replaces one", async (t) => {
  const fixture = await newStageFixture(t);
  const draft = fixture.draft();
  const names = expectedAssetNames(STAGE_TAG);
  for (const name of names.slice(0, 3))
    fixture.fake.addAsset(draft, fixture.assets, name);
  const before = new Map(draft.assets.map((asset) => [asset.name, asset.id]));
  const result = await fixture.run();
  assert.equal(result.code, 0, result.stderr);
  assert.deepEqual(uploadedNames(fixture.seen()), names.slice(3));
  assert.ok(names.slice(3).includes(tarballName(STAGE_TAG)));
  // An existing asset keeps its identity: stage never replaces one.
  const after = new Map(
    fixture.fake.release(42)?.assets.map((asset) => [asset.name, asset.id]),
  );
  for (const [name, id] of before)
    assert.equal(after.get(name), id, `asset ${name} was replaced`);
  assert.equal(after.size, names.length);
  // One reread after each upload, plus the final read.
  assert.ok(countRequests(fixture.seen(), "GET", `${ROUTE}/releases/42`) >= 5);
  assert.equal(
    fixture.fake.created.length,
    0,
    "stage created a release beside the partial draft",
  );
  assertOnePublishingPatch(fixture);
});

test("stage stops when the release disappears after an upload", async (t) => {
  const fixture = await newStageFixture(t);
  fixture.draft();
  fixture.fake.afterUpload = (fake) => {
    fake.releases = [];
  };
  const result = await fixture.run();
  assert.equal(result.code, 1, result.stderr);
  assert.ok(
    result.stderr.includes(
      `release: stage: release ${STAGE_TAG} disappeared after asset upload`,
    ),
    result.stderr,
  );
  assert.equal(
    countUploads(fixture.seen()),
    1,
    "stage uploaded more than one asset before it stopped",
  );
  // The listing is read once, for discovery. A second read would be a rediscovery of a release this run had
  // already pinned, and it could pin a different one.
  assert.equal(countRequests(fixture.seen(), "GET", `${ROUTE}/releases`), 1);
  assertNoMethod(fixture.seen(), "PATCH");
});

test("stage rejects an existing asset whose bytes differ, direct or redirected", async (t) => {
  for (const redirect of [false, true]) {
    const fixture = await newStageFixture(t);
    fixture.fake.redirectAssets = redirect;
    const draft = fixture.draft();
    fixture.fake.addAsset(draft, fixture.assets, NOTICES);
    const [asset] = draft.assets;
    assert.ok(asset !== undefined);
    asset.contents = Buffer.from("bytes that are not the local asset\n");
    const result = await fixture.run();
    assert.equal(result.code, 1, `redirect=${redirect}: ${result.stderr}`);
    assert.ok(
      result.stderr.includes(
        `release: stage: observed asset ${NOTICES} on release ${STAGE_TAG} conflicts`,
      ),
      result.stderr,
    );
    // One upload is correct here, and it is SHA256SUMS: the run walks the roster in order and uploads a missing
    // asset as it reaches it. SHA256SUMS is first and absent; the conflicting asset is second, so the run stops there.
    assert.deepEqual(uploadedNames(fixture.seen()), [CHECKSUMS]);
    assertNoMethod(fixture.seen(), "PATCH");
    assertNoOutputs(fixture.output);
  }
});

test("stage refuses a draft carrying the checksum roster from before the plugin archive", async (t) => {
  const fixture = await newStageFixture(t);
  const draft = fixture.draft();
  fixture.fake.addAsset(draft, fixture.assets, CHECKSUMS);
  const [asset] = draft.assets;
  assert.ok(asset !== undefined);
  asset.contents = Buffer.from(
    asset.contents
      .toString("utf8")
      .split("\n")
      .filter((line) => !line.endsWith(`  agentics-plugin-${STAGE_TAG}.tar.gz`))
      .join("\n"),
  );
  const result = await fixture.run();
  assert.equal(result.code, 1, result.stderr);
  assert.ok(
    result.stderr.includes(
      `release: stage: observed asset ${CHECKSUMS} on release ${STAGE_TAG} conflicts; publish a new patch release for immutable content`,
    ),
    result.stderr,
  );
  assert.equal(countUploads(fixture.seen()), 0);
  assertNoMethod(fixture.seen(), "PATCH");
  assert.equal(fixture.fake.created.length, 0);
  assert.equal(draft.assets.length, 1);
  assertNoOutputs(fixture.output);
});

test("stage follows the asset redirect to compare bytes", async (t) => {
  const fixture = await newStageFixture(t);
  // github.com answers an asset read with a 302 to a separate download host. A client that does not follow
  // would compare nothing, so a fake that never redirects would pass every comparison and prove nothing.
  fixture.fake.redirectAssets = true;
  const draft = fixture.draft();
  fixture.tagged();
  fixture.fake.addAllAssets(draft, fixture.assets, STAGE_TAG);
  const result = await fixture.run();
  assert.equal(result.code, 0, result.stderr);
  assert.ok(
    countRequests(fixture.seen(), "GET", `${ASSET_BYTES_ROUTE}1000`) > 0,
    "stage never followed the asset redirect",
  );
  assertNoMethod(fixture.seen(), "POST");
  assertOutputs(fixture.output, { release_id: "42", published: "false" });
});

test("stage creates the tag before the draft and the draft reports the real tag", async (t) => {
  const fixture = await newStageFixture(t);
  fixture.fake.placeholderDraftTags = true;
  const result = await fixture.run();
  assert.equal(result.code, 0, result.stderr);
  assert.deepEqual(fixture.fake.tagCreated, [
    { ref: `refs/tags/${STAGE_TAG}`, sha: fixture.sha },
  ]);
  const seen = fixture.seen();
  const refIndex = seen.findIndex(
    (request) =>
      request.method === "POST" && request.path === `${ROUTE}/git/refs`,
  );
  const createIndex = seen.findIndex(
    (request) =>
      request.method === "POST" && request.path === `${ROUTE}/releases`,
  );
  assert.ok(
    refIndex >= 0 && createIndex >= 0 && refIndex < createIndex,
    `tag create at ${refIndex}, draft create at ${createIndex}; want the tag first`,
  );
  const [published] = fixture.fake.releases;
  assert.ok(published !== undefined);
  assert.equal(wireTagName(published), STAGE_TAG);
});

test("stage skips the tag write when the tag already exists", async (t) => {
  const fixture = await newStageFixture(t);
  fixture.tagged();
  fixture.draft();
  const result = await fixture.run();
  assert.equal(result.code, 0, result.stderr);
  assert.equal(
    fixture.fake.tagCreated.length,
    0,
    "stage created a tag that already existed",
  );
  assert.ok(result.stderr.includes("adopted draft 42"), result.stderr);
});

test("stage creates the tag at the target when it adopts a draft that has none", async (t) => {
  const fixture = await newStageFixture(t);
  const draft = fixture.draft();
  fixture.fake.addAllAssets(draft, fixture.assets, STAGE_TAG);
  const result = await fixture.run();
  assert.equal(result.code, 0, result.stderr);
  assert.equal(fixture.fake.tagCreated.length, 1);
  assert.equal(fixture.fake.tagObject?.sha, fixture.sha);
  assert.ok(
    result.stderr.includes(`created tag ${STAGE_TAG}`) &&
      result.stderr.includes("adopted draft 42"),
    result.stderr,
  );
});

test("stage accepts a lost race on the tag only when the winner tagged the same commit", async (t) => {
  const cases: { name: string; winner: string; code: number }[] = [
    { name: "the winner tagged the same commit", winner: "", code: 0 },
    { name: "the winner tagged another commit", winner: OTHER_SHA, code: 1 },
  ];
  for (const item of cases) {
    const fixture = await newStageFixture(t);
    fixture.fake.tagCreateStatus = 422;
    const winner = item.winner === "" ? fixture.sha : item.winner;
    // The first read says absent; the create loses; the reread sees the winner's tag. The hook flips the object after read one.
    fixture.fake.tagReadHook = (fake, reads) => {
      if (reads === 1) fake.tagObject = { type: "commit", sha: winner };
    };
    const result = await fixture.run();
    assert.equal(result.code, item.code, `${item.name}: ${result.stderr}`);
    if (item.code === 1) {
      assert.ok(
        result.stderr.includes("does not target"),
        `${item.name}: ${result.stderr}`,
      );
      assert.equal(fixture.fake.created.length, 0, item.name);
    }
  }
});

test("stage publishes a real release in one PATCH that pins the tag and the target and makes it latest", async (t) => {
  const fixture = await newStageFixture(t);
  const result = await fixture.run();
  assert.equal(result.code, 0, result.stderr);
  assertOutputs(fixture.output, { release_id: "100", published: "false" });
  assert.equal(
    fixture.fake.created[0]?.["prerelease"],
    true,
    "the draft must be a pre-release so it is never latest",
  );
  assert.deepEqual(fixture.fake.patched, [
    {
      tag_name: STAGE_TAG,
      target_commitish: fixture.sha,
      draft: false,
      prerelease: false,
      make_latest: "true",
    },
  ]);
  const release = fixture.fake.releases[0];
  assert.ok(release !== undefined);
  assert.equal(release.draft, false);
  assert.equal(release.prerelease, false);
  assert.equal(wireTagName(release), STAGE_TAG);
  assert.equal(release.assets.length, 10);
  assert.ok(
    result.stderr.includes(
      `release: stage: published release 100 for ${STAGE_TAG}`,
    ),
    result.stderr,
  );
});

test("stage publishes a pre-release in one PATCH that does not make it latest", async (t) => {
  const fixture = await newStageFixture(t);
  const result = await fixture.run(["--prerelease", "true"]);
  assert.equal(result.code, 0, result.stderr);
  assertOutputs(fixture.output, { release_id: "100", published: "false" });
  assert.deepEqual(fixture.fake.patched, [
    {
      tag_name: STAGE_TAG,
      target_commitish: fixture.sha,
      draft: false,
      prerelease: true,
      make_latest: "false",
    },
  ]);
  const release = fixture.fake.releases[0];
  assert.ok(release !== undefined);
  assert.equal(release.draft, false);
  assert.equal(release.prerelease, true);
  assert.equal(wireTagName(release), STAGE_TAG);
  assert.equal(release.assets.length, 10);
  assert.ok(
    result.stderr.includes(
      `release: stage: published pre-release 100 for ${STAGE_TAG}`,
    ),
    result.stderr,
  );
});

test("stage publishes an adopted placeholder draft under its real tag because the PATCH carries tag_name", async (t) => {
  const fixture = await newStageFixture(t);
  fixture.fake.placeholderDraftTags = true;
  const draft = fixture.draft();
  fixture.fake.addAllAssets(draft, fixture.assets, STAGE_TAG);
  const result = await fixture.run();
  assert.equal(result.code, 0, result.stderr);
  assert.equal(fixture.fake.patched[0]?.["tag_name"], STAGE_TAG);
  const [release] = fixture.fake.releases;
  assert.ok(release !== undefined);
  assert.equal(wireTagName(release), STAGE_TAG);
});

test("a pre-release run fails by name when the release reads back as promoted", async (t) => {
  const fixture = await newStageFixture(t);
  fixture.fake.afterPatch = (fake) => {
    const [release] = fake.releases;
    if (release !== undefined) release.prerelease = false;
  };
  const result = await fixture.run(["--prerelease", "true"]);
  assert.equal(result.code, 1, result.stderr);
  assert.ok(
    result.stderr.includes("is not a published pre-release under its tag"),
    result.stderr,
  );
  assertNoOutputs(fixture.output);
});

test("a release run fails by name when the release still reads back as a pre-release", async (t) => {
  const fixture = await newStageFixture(t);
  fixture.fake.afterPatch = (fake) => {
    const [release] = fake.releases;
    if (release !== undefined) release.prerelease = true;
  };
  const result = await fixture.run();
  assert.equal(result.code, 1, result.stderr);
  assert.ok(
    result.stderr.includes("is not a published release under its tag"),
    result.stderr,
  );
  assertNoOutputs(fixture.output);
});

test("stage checks identity first after the PATCH, so a release published under a placeholder is named with its recovery", async (t) => {
  const fixture = await newStageFixture(t);
  fixture.fake.afterPatch = (fake) => {
    const [release] = fake.releases;
    if (release !== undefined) release.tagName = placeholderTagFor(release);
  };
  const result = await fixture.run();
  assert.equal(result.code, 1, result.stderr);
  assert.ok(
    result.stderr.includes(
      `observed release ${STAGE_TAG} is published under placeholder tag untagged-64`,
    ),
    result.stderr,
  );
  assertNoOutputs(fixture.output);
});

test("stage compares all ten assets again after the PATCH and revalidates the roster", async (t) => {
  const bytes = await newStageFixture(t);
  bytes.fake.afterPatch = (fake) => {
    const asset = fake.releases[0]?.assets[2];
    if (asset !== undefined)
      asset.contents = Buffer.from("changed after publication\n");
  };
  const changed = await bytes.run();
  assert.equal(changed.code, 1, changed.stderr);
  assert.ok(
    changed.stderr.includes(
      `release: stage: observed asset ${expectedAssetNames(STAGE_TAG)[2]} on release ${STAGE_TAG} conflicts`,
    ),
    changed.stderr,
  );
  assertNoOutputs(bytes.output);

  const roster = await newStageFixture(t);
  roster.fake.afterPatch = (fake) => {
    fake.releases[0]?.assets.push({
      id: 5000,
      name: "stray.txt",
      contents: Buffer.from("x"),
    });
  };
  const stray = await roster.run();
  assert.equal(stray.code, 1, stray.stderr);
  assert.ok(
    stray.stderr.includes(
      `release: stage: observed asset stray.txt on release ${STAGE_TAG} conflicts`,
    ),
    stray.stderr,
  );
  assertNoOutputs(roster.output);
});

test("stage fails when an asset goes missing between the last upload and the final reread", async (t) => {
  const fixture = await newStageFixture(t);
  // Every uploaded asset is present when it is checked. Only the final reread sees nine, and a draft may
  // legitimately hold a short roster, so only the final count can catch it.
  fixture.fake.afterUpload = (fake) => {
    const [release] = fake.releases;
    if (release !== undefined && release.assets.length === 10)
      release.assets.splice(0, 1);
  };
  const result = await fixture.run();
  assert.equal(result.code, 1, result.stderr);
  assert.ok(
    result.stderr.includes(
      `release: stage: observed asset <missing expected asset> on release ${STAGE_TAG} conflicts`,
    ),
    result.stderr,
  );
  assertNoMethod(fixture.seen(), "PATCH");
});

test("stage retries a latest read that has not caught up", async (t) => {
  const fixture = await newStageFixture(t);
  fixture.fake.addRelease({
    id: 7,
    tagName: "v1.2.2",
    name: "v1.2.2",
    targetCommitish: OTHER_SHA,
  });
  // The first read finds no latest release, the second still names the previous one, and the third names the new one.
  fixture.fake.latestScript = [0, 7, 100];
  const result = await fixture.run();
  assert.equal(result.code, 0, result.stderr);
  for (const attempt of [1, 2]) {
    assert.ok(
      result.stderr.includes(
        `release: stage: latest release is not 100 yet, attempt ${attempt} of 3`,
      ),
      result.stderr,
    );
  }
  assert.ok(
    !result.stderr.includes("attempt 3 of 3"),
    "stage retried after latest named the release",
  );
  assert.equal(countRequests(fixture.seen(), "GET", LATEST.path), 3);
  assertOutputs(fixture.output, { release_id: "100", published: "false" });
});

test("stage fails after four latest reads that never name the release", async (t) => {
  const scripts: Record<string, number[]> = {
    "not found": [0, 0, 0, 0],
    "another release": [7, 7, 7, 7],
  };
  for (const [name, script] of Object.entries(scripts)) {
    const fixture = await newStageFixture(t);
    fixture.fake.addRelease({
      id: 7,
      tagName: "v1.2.2",
      name: "v1.2.2",
      targetCommitish: OTHER_SHA,
    });
    fixture.fake.latestScript = script;
    const result = await fixture.run();
    assert.equal(result.code, 1, `${name}: ${result.stderr}`);
    assert.ok(
      result.stderr.includes(
        `release: stage: release ${STAGE_TAG} (id 100) was not the latest release after publication`,
      ),
      result.stderr,
    );
    assert.equal(countRequests(fixture.seen(), "GET", LATEST.path), 4, name);
    assertNoOutputs(fixture.output);
  }
});

test("stage reads a 500 from releases/latest as fatal after one read", async (t) => {
  const fixture = await newStageFixture(t);
  fixture.fake.latestScript = [-1];
  const result = await fixture.run();
  assert.equal(result.code, 1, result.stderr);
  assert.equal(countRequests(fixture.seen(), "GET", LATEST.path), 1);
  assertNoOutputs(fixture.output);
});

test("a pre-release run never reads releases/latest", async (t) => {
  const fixture = await newStageFixture(t);
  const result = await fixture.run(["--prerelease", "true"]);
  assert.equal(result.code, 0, result.stderr);
  assert.equal(countRequests(fixture.seen(), "GET", LATEST.path), 0);
});

test("stage adopts a placeholder draft on a rerun instead of creating a second release", async (t) => {
  const fixture = await newStageFixture(t);
  fixture.fake.placeholderDraftTags = true;
  const draft = fixture.draft();
  fixture.fake.addAllAssets(draft, fixture.assets, STAGE_TAG);
  const result = await fixture.run();
  assert.equal(result.code, 0, result.stderr);
  assertOutputs(fixture.output, { release_id: "42", published: "false" });
  // The rerun must find the draft by name and reuse it, and it creates the tag this pre-change draft never had (#620).
  assert.equal(countRequests(fixture.seen(), "POST", `${ROUTE}/releases`), 0);
  assert.equal(fixture.fake.tagCreated.length, 1);
});

test("stage creates under the placeholder model and ends with the real tag", async (t) => {
  const fixture = await newStageFixture(t);
  fixture.fake.placeholderDraftTags = true;
  const result = await fixture.run();
  assert.equal(result.code, 0, result.stderr);
  assert.equal(fixture.fake.created[0]?.["tag_name"], STAGE_TAG);
  assertOutputs(fixture.output, { release_id: "100", published: "false" });
  assert.equal(fixture.fake.tagCreated.length, 1);
  const [release] = fixture.fake.releases;
  assert.ok(release !== undefined);
  assert.equal(wireTagName(release), STAGE_TAG);
});

test("stage resolves a placeholder draft by its name only", async (t) => {
  // A placeholder draft named for another tag is a stranger, so stage creates this tag's draft beside it.
  const stranger = await newStageFixture(t);
  stranger.fake.placeholderDraftTags = true;
  const other = stranger.draft();
  other.name = "v9.9.9";
  other.tagName = "v9.9.9";
  const created = await stranger.run();
  assert.equal(created.code, 0, created.stderr);
  assert.equal(
    stranger.fake.created.length,
    1,
    "stage must create exactly one release for this tag",
  );

  // A placeholder draft named for this tag is this tag's draft, so its wrong target is a conflict and not a
  // reason to create a second one.
  const owned = await newStageFixture(t);
  owned.fake.placeholderDraftTags = true;
  owned.draft().targetCommitish = OTHER_SHA;
  const conflict = await owned.run();
  assert.equal(conflict.code, 1, conflict.stderr);
  assert.ok(
    conflict.stderr.includes(`has targetCommitish ${OTHER_SHA}`),
    conflict.stderr,
  );
  assert.equal(
    owned.fake.created.length,
    0,
    "stage created a release beside this tag's draft",
  );
});

test("stage names the release it adopted, created or found published, and never one it rejected", async (t) => {
  const adopted = await newStageFixture(t);
  adopted.fake.addAllAssets(adopted.draft(), adopted.assets, STAGE_TAG);
  const adoptedResult = await adopted.run();
  assert.equal(adoptedResult.code, 0, adoptedResult.stderr);
  assert.ok(
    adoptedResult.stderr.includes(
      `release: stage: adopted draft 42 for ${STAGE_TAG}`,
    ),
    adoptedResult.stderr,
  );

  const created = await newStageFixture(t);
  const createdResult = await created.run();
  assert.equal(createdResult.code, 0, createdResult.stderr);
  assert.ok(
    createdResult.stderr.includes(
      `release: stage: created draft 100 for ${STAGE_TAG}`,
    ),
    createdResult.stderr,
  );

  const published = await newStageFixture(t);
  published.fake.addAllAssets(
    published.published(),
    published.assets,
    STAGE_TAG,
  );
  const publishedResult = await published.run();
  assert.equal(publishedResult.code, 0, publishedResult.stderr);
  assert.ok(
    publishedResult.stderr.includes(
      `release: stage: found published release 42 for ${STAGE_TAG}`,
    ),
    publishedResult.stderr,
  );
  assert.ok(
    !publishedResult.stderr.includes("adopted draft"),
    "a published release is not an adopted draft",
  );

  const conflict = await newStageFixture(t);
  conflict.draft().targetCommitish = OTHER_SHA;
  const conflictResult = await conflict.run();
  assert.equal(conflictResult.code, 1, conflictResult.stderr);
  assert.ok(
    !conflictResult.stderr.includes("adopted draft"),
    "a rejected release must not be named as adopted",
  );
});

test("stage never prints the body of a draft it publishes", async (t) => {
  const fixture = await newStageFixture(t);
  fixture.draft().body = PUBLISHED_BODY;
  const result = await fixture.run();
  assert.equal(result.code, 0, result.stderr);
  assertNoBodyLeak(result.stderr, STAGE_NOTES, PUBLISHED_BODY);
});

test("stage revalidates the identity of the pinned release after an upload", async (t) => {
  const fixture = await newStageFixture(t);
  fixture.draft();
  fixture.fake.afterUpload = (fake) => {
    const release = fake.release(42);
    if (release !== undefined) release.targetCommitish = OTHER_SHA;
  };
  const result = await fixture.run();
  assert.equal(result.code, 1, result.stderr);
  assert.ok(
    result.stderr.includes(
      `release: stage: observed release ${STAGE_TAG} has targetCommitish ${OTHER_SHA}`,
    ),
    result.stderr,
  );
  assertNoMethod(fixture.seen(), "PATCH");
  assertNoOutputs(fixture.output);
});

test("stage revalidates the roster of the pinned release after an upload", async (t) => {
  const fixture = await newStageFixture(t);
  fixture.draft();
  fixture.fake.afterUpload = (fake) => {
    const release = fake.release(42);
    if (
      release !== undefined &&
      !release.assets.some((asset) => asset.name === "stray.txt")
    ) {
      release.assets.push({
        id: 5000,
        name: "stray.txt",
        contents: Buffer.from("x"),
      });
    }
  };
  const result = await fixture.run();
  assert.equal(result.code, 1, result.stderr);
  assert.ok(
    result.stderr.includes(
      `release: stage: observed asset stray.txt on release ${STAGE_TAG} conflicts`,
    ),
    result.stderr,
  );
  assertNoMethod(fixture.seen(), "PATCH");
});

test("stage names the kind in the error when the publishing PATCH is refused", async (t) => {
  for (const [flag, kind] of [
    ["false", "release"],
    ["true", "pre-release"],
  ] as const) {
    const fixture = await newStageFixture(t);
    fixture.fake.patchStatus = 500;
    const result = await fixture.run(["--prerelease", flag]);
    assert.equal(result.code, 1, result.stderr);
    assert.ok(
      result.stderr.includes(
        `release: stage: cannot publish ${STAGE_TAG} as a ${kind}: PATCH `,
      ),
      result.stderr,
    );
    assert.equal(
      fixture.fake.patched.length,
      1,
      "a refused PATCH must not be retried",
    );
    assertNoOutputs(fixture.output);
  }
});

test("stage fails by name when the release still reads back as a draft after the PATCH", async (t) => {
  const fixture = await newStageFixture(t);
  fixture.fake.afterPatch = (fake) => {
    const [release] = fake.releases;
    if (release !== undefined) release.draft = true;
  };
  const result = await fixture.run();
  assert.equal(result.code, 1, result.stderr);
  assert.ok(
    result.stderr.includes(
      `release: stage: release ${STAGE_TAG} is not a published release under its tag after publication (draft=true prerelease=false tag_name=${STAGE_TAG})`,
    ),
    result.stderr,
  );
  assertNoOutputs(fixture.output);
});

test("stage retries a 404 on the read that follows the publishing PATCH", async (t) => {
  const fixture = await newStageFixture(t);
  fixture.tagged();
  fixture.fake.addAllAssets(fixture.draft(), fixture.assets, STAGE_TAG);
  // Two id reads precede the PATCH (discovery, then the final reread of the fill); the next two answer 404.
  fixture.fake.readByIdCodes = [200, 200, 404, 404];
  const result = await fixture.run();
  assert.equal(result.code, 0, result.stderr);
  assert.equal(
    result.stderr.split("draft 42 not readable yet").length - 1,
    2,
    result.stderr,
  );
  assertOutputs(fixture.output, { release_id: "42", published: "false" });
});

test("stage waits retryDelayMs between latest reads", async (t) => {
  const fixture = await newStageFixture(t);
  fixture.fake.addRelease({
    id: 7,
    tagName: "v1.2.2",
    name: "v1.2.2",
    targetCommitish: OTHER_SHA,
  });
  fixture.fake.latestScript = [0, 7, 100];
  const result = await fixture.run([], { retryDelayMs: 60 });
  assert.equal(result.code, 0, result.stderr);
  const times = fixture.fake.latestReadTimes;
  assert.equal(times.length, 3);
  for (let index = 1; index < times.length; index++) {
    assert.ok(
      (times[index] ?? 0) - (times[index - 1] ?? 0) >= 50,
      `gap ${index} was ${(times[index] ?? 0) - (times[index - 1] ?? 0)} ms, want the 60 ms delay`,
    );
  }
});
