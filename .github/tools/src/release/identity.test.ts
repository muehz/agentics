import assert from "node:assert/strict";
import { test } from "node:test";
import { expectedAssetNames } from "./assets.ts";
import type { Release } from "./github.ts";
import {
  MISSING_ASSET_NAME,
  conflictOrphanTag,
  conflictStrayPlaceholderTag,
  hasRemoteAsset,
  listingMatches,
  placeholderRecovery,
  remoteAssetId,
  splitRepo,
  strayTagRecovery,
  tagNameMatches,
  validateReleaseIdentity,
  validateRemoteRoster,
} from "./identity.ts";
import { FULL_SHA, STABLE_TAG } from "./version.ts";

const TAG = "v1.2.3";
const SHA = "2".repeat(40);
const OTHER_SHA = "a".repeat(40);

function release(overrides: Partial<Release> = {}): Release {
  return {
    id: 42,
    tag_name: TAG,
    name: TAG,
    body: "",
    draft: false,
    prerelease: false,
    target_commitish: SHA,
    assets: [],
    ...overrides,
  };
}

function fullRoster(): Release["assets"] {
  return expectedAssetNames(TAG).map((name, index) => ({
    id: 1000 + index,
    name,
  }));
}

const PLACEHOLDER_RECOVERY_2A =
  "recover with: (1) gh release delete untagged-2a --cleanup-tag --yes, which addresses the release by the placeholder tag it carries; " +
  "(2) git push origin <target-sha>:refs/tags/v1.2.3 at the commit that release was built from; " +
  "(3) dispatch the release workflow for that target SHA to rebuild and publish the release with its ten assets; " +
  "(4) if it was published as a pre-release, promote it from the Releases page when it is ready";
const PLACEHOLDER_RECOVERY_ABC = PLACEHOLDER_RECOVERY_2A.replace(
  "untagged-2a",
  "untagged-abc",
);

function messageOf(action: () => void): string {
  try {
    action();
  } catch (error) {
    return error instanceof Error ? error.message : String(error);
  }
  return "";
}

test("identity accepts a placeholder tag_name on a draft only", () => {
  assert.doesNotThrow(() =>
    validateReleaseIdentity(
      release({ draft: true, tag_name: "untagged-2a" }),
      TAG,
      SHA,
      42,
      false,
      "",
    ),
  );
  assert.throws(() =>
    validateReleaseIdentity(
      release({ draft: true, tag_name: "v9.9.9" }),
      TAG,
      SHA,
      42,
      false,
      "",
    ),
  );
  const published = messageOf(() =>
    validateReleaseIdentity(
      release({ tag_name: "untagged-2a" }),
      TAG,
      SHA,
      42,
      true,
      SHA,
    ),
  );
  assert.equal(
    published,
    `observed release ${TAG} is published under placeholder tag untagged-2a; ${PLACEHOLDER_RECOVERY_2A}`,
  );
  assert.doesNotThrow(() =>
    validateReleaseIdentity(release(), TAG, SHA, 42, true, SHA),
  );
});

test("placeholderRecovery covers a release and a pre-release", () => {
  assert.equal(
    placeholderRecovery("v1.2.3", "untagged-abc"),
    PLACEHOLDER_RECOVERY_ABC,
  );
});

test("identity accepts a pre-release, draft or published", () => {
  for (const draft of [true, false]) {
    assert.doesNotThrow(
      () =>
        validateReleaseIdentity(
          release({ draft, prerelease: true }),
          TAG,
          SHA,
          42,
          true,
          SHA,
        ),
      `draft=${draft}`,
    );
  }
});

test("identity rejects an unusable id before it looks at anything else", () => {
  for (const id of [0, -1]) {
    assert.equal(
      messageOf(() =>
        validateReleaseIdentity(
          release({ id, name: "wrong" }),
          TAG,
          SHA,
          0,
          true,
          SHA,
        ),
      ),
      `release ${TAG} has no valid numeric id`,
    );
  }
});

test("identity checks the title before the tag_name, and the tag_name before the target", () => {
  const conflict = `observed release ${TAG} has conflicting identity; use a separately human-approved notes edit for prose-only correction or publish a new patch release for immutable content`;
  assert.equal(
    messageOf(() =>
      validateReleaseIdentity(
        release({ tag_name: "untagged-2a", name: "wrong" }),
        TAG,
        SHA,
        42,
        true,
        SHA,
      ),
    ),
    conflict,
  );
  assert.equal(
    messageOf(() =>
      validateReleaseIdentity(
        release({
          draft: true,
          tag_name: "v9.9.9",
          target_commitish: OTHER_SHA,
        }),
        TAG,
        SHA,
        42,
        false,
        "",
      ),
    ),
    conflict,
  );
});

test("identity rejects a release whose id is not the pinned one, and accepts any id while unpinned", () => {
  assert.equal(
    messageOf(() =>
      validateReleaseIdentity(
        release({ id: 200, name: "wrong" }),
        TAG,
        SHA,
        100,
        true,
        SHA,
      ),
    ),
    "pinned release id 100 returned release id 200; refusing a mismatched release ID",
  );
  assert.doesNotThrow(() =>
    validateReleaseIdentity(release({ id: 200 }), TAG, SHA, 0, true, SHA),
  );
});

test("identity rejects a wrong title and a draft under another real tag with the identity conflict", () => {
  const want = `observed release ${TAG} has conflicting identity; use a separately human-approved notes edit for prose-only correction or publish a new patch release for immutable content`;
  assert.equal(
    messageOf(() =>
      validateReleaseIdentity(
        release({ name: `Release ${TAG}` }),
        TAG,
        SHA,
        42,
        true,
        SHA,
      ),
    ),
    want,
  );
  assert.equal(
    messageOf(() =>
      validateReleaseIdentity(
        release({ draft: true, tag_name: "v9.9.9" }),
        TAG,
        SHA,
        42,
        false,
        "",
      ),
    ),
    want,
  );
  assert.equal(
    messageOf(() =>
      validateReleaseIdentity(
        release({ tag_name: "v9.9.9" }),
        TAG,
        SHA,
        42,
        true,
        SHA,
      ),
    ),
    want,
  );
});

test("identity rejects a wrong target before it looks at the tag", () => {
  const want = `observed release ${TAG} has targetCommitish ${OTHER_SHA}, expected ${SHA}; publish a new patch release for immutable content`;
  assert.equal(
    messageOf(() =>
      validateReleaseIdentity(
        release({ target_commitish: OTHER_SHA }),
        TAG,
        SHA,
        42,
        false,
        "",
      ),
    ),
    want,
  );
  assert.equal(
    messageOf(() =>
      validateReleaseIdentity(
        release({ draft: true, target_commitish: OTHER_SHA }),
        TAG,
        SHA,
        42,
        false,
        "",
      ),
    ),
    want,
  );
});

test("identity holds a published release to the tag and a draft to nothing", () => {
  const want = `observed tag ${TAG} does not target ${SHA}; publish a new patch release for immutable content`;
  assert.equal(
    messageOf(() =>
      validateReleaseIdentity(release(), TAG, SHA, 42, false, ""),
    ),
    want,
  );
  assert.equal(
    messageOf(() =>
      validateReleaseIdentity(release(), TAG, SHA, 42, true, OTHER_SHA),
    ),
    want,
  );
  assert.doesNotThrow(() =>
    validateReleaseIdentity(release({ draft: true }), TAG, SHA, 42, false, ""),
  );
  assert.doesNotThrow(() =>
    validateReleaseIdentity(
      release({ draft: true }),
      TAG,
      SHA,
      42,
      true,
      OTHER_SHA,
    ),
  );
});

test("tagNameMatches and listingMatches resolve a placeholder by the rule each one owns", () => {
  assert.equal(tagNameMatches(release({ tag_name: TAG }), TAG), true);
  assert.equal(
    tagNameMatches(release({ draft: true, tag_name: "untagged-1" }), TAG),
    true,
  );
  assert.equal(
    tagNameMatches(release({ draft: false, tag_name: "untagged-1" }), TAG),
    false,
  );
  assert.equal(
    tagNameMatches(release({ draft: true, tag_name: "v9.9.9" }), TAG),
    false,
  );

  assert.equal(
    listingMatches(release({ tag_name: TAG, name: "Release title" }), TAG),
    true,
    "a real tag matches whatever the name says",
  );
  assert.equal(
    listingMatches(
      release({ draft: true, tag_name: "untagged-1", name: TAG }),
      TAG,
    ),
    true,
  );
  assert.equal(
    listingMatches(
      release({ draft: false, tag_name: "untagged-1", name: TAG }),
      TAG,
    ),
    true,
    "a published placeholder must reach the identity check",
  );
  assert.equal(
    listingMatches(
      release({ draft: true, tag_name: "untagged-1", name: "v9.9.9" }),
      TAG,
    ),
    false,
    "another name is a stranger",
  );
  assert.equal(
    listingMatches(release({ tag_name: "v9.9.9", name: TAG }), TAG),
    false,
  );
  assert.equal(
    listingMatches(release({ tag_name: "v1-untagged-1", name: TAG }), TAG),
    false,
    "a placeholder is a prefix, not a substring",
  );
});

test("the remote roster rejects a name outside the ten, a duplicate, and a short published roster", () => {
  const stray = release({
    draft: true,
    assets: [{ id: 1, name: "stray.txt" }],
  });
  assert.equal(
    messageOf(() => validateRemoteRoster(stray, TAG)),
    `observed asset stray.txt on release ${TAG} conflicts; publish a new patch release for immutable content`,
  );
  const duplicate = release({
    draft: true,
    assets: [
      { id: 1, name: "THIRD_PARTY_NOTICES.md" },
      { id: 2, name: "THIRD_PARTY_NOTICES.md" },
    ],
  });
  assert.equal(
    messageOf(() => validateRemoteRoster(duplicate, TAG)),
    `observed asset THIRD_PARTY_NOTICES.md on release ${TAG} conflicts; publish a new patch release for immutable content`,
  );
  const short = release({ draft: false, assets: fullRoster().slice(1) });
  assert.equal(
    messageOf(() => validateRemoteRoster(short, TAG)),
    `observed asset ${MISSING_ASSET_NAME} on release ${TAG} conflicts; publish a new patch release for immutable content`,
  );
  assert.equal(MISSING_ASSET_NAME, "<missing expected asset>");
});

test("the remote roster accepts a short draft and a complete published release", () => {
  assert.doesNotThrow(() =>
    validateRemoteRoster(
      release({ draft: true, assets: fullRoster().slice(0, 3) }),
      TAG,
    ),
  );
  assert.doesNotThrow(() =>
    validateRemoteRoster(release({ draft: false, assets: fullRoster() }), TAG),
  );
});

test("hasRemoteAsset and remoteAssetId need exactly one asset of the name and a positive id", () => {
  const roster = [
    { id: 7, name: "SHA256SUMS" },
    { id: 8, name: "THIRD_PARTY_NOTICES.md" },
    { id: 9, name: "THIRD_PARTY_NOTICES.md" },
    { id: 0, name: "agentics-v1.2.3.tar.gz" },
  ];
  const subject = release({ assets: roster });
  assert.equal(hasRemoteAsset(subject, "SHA256SUMS"), true);
  assert.equal(hasRemoteAsset(subject, "THIRD_PARTY_NOTICES.md"), false);
  assert.equal(hasRemoteAsset(subject, "missing"), false);
  assert.equal(remoteAssetId(subject, "SHA256SUMS", TAG), 7);
  for (const name of [
    "THIRD_PARTY_NOTICES.md",
    "missing",
    "agentics-v1.2.3.tar.gz",
  ]) {
    assert.equal(
      messageOf(() => remoteAssetId(subject, name, TAG)),
      `observed asset ${name} on release ${TAG} conflicts; publish a new patch release for immutable content`,
      name,
    );
  }
});

test("conflict messages name the recovery a maintainer runs", () => {
  const orphan = conflictOrphanTag(TAG, SHA).message;
  assert.equal(
    orphan,
    `observed tag ${TAG} does not target ${SHA}; a discarded draft leaves this behind: with the draft still present run gh release delete ${TAG} --cleanup-tag --yes, with no draft run git push origin :refs/tags/${TAG}, then rerun; for a published release, publish a new patch release for immutable content`,
  );
  const stray = conflictStrayPlaceholderTag("untagged-abc").message;
  assert.equal(
    stray,
    `observed stray placeholder tag untagged-abc on the remote; ${strayTagRecovery("untagged-abc")}`,
  );
  assert.ok(
    strayTagRecovery("untagged-abc").startsWith(
      "recover with: git push origin :refs/tags/untagged-abc, then rerun; ",
    ),
  );
  assert.ok(
    strayTagRecovery("untagged-abc").includes(
      placeholderRecovery("<release name>", "untagged-abc"),
    ),
  );
});

test("splitRepo accepts OWNER/REPO and rejects every other shape", () => {
  assert.deepEqual(splitRepo("muehz/agentics"), {
    owner: "muehz",
    name: "agentics",
  });
  for (const bad of [
    "agentics",
    "muehz/",
    "/agentics",
    "muehz/agentics/go",
    "",
    "/",
  ]) {
    assert.equal(splitRepo(bad), undefined, bad);
  }
});

test("the tag and SHA patterns accept only a stable tag and a full lowercase SHA", () => {
  for (const good of [
    "v0.0.0",
    "v1.2.3",
    "v10.20.30",
    "v9223372036854775808.0.0",
  ])
    assert.ok(STABLE_TAG.test(good), good);
  for (const bad of [
    "1.2.3",
    "v1.2",
    "v1.2.3-rc1",
    "v1.2.3+build",
    "v1.2.3\n",
    " v1.2.3",
    "v1.2.x",
  ])
    assert.ok(!STABLE_TAG.test(bad), bad);
  assert.ok(FULL_SHA.test(SHA));
  for (const bad of [
    SHA.slice(1),
    `${SHA}0`,
    "A".repeat(40),
    `${SHA}\n`,
    "g".repeat(40),
    "",
  ]) {
    assert.ok(!FULL_SHA.test(bad), bad);
  }
});
