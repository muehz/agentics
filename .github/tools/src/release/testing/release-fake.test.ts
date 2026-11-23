import assert from "node:assert/strict";
import { writeFileSync } from "node:fs";
import { join } from "node:path";
import { test } from "node:test";
import {
  expectedAssetNames,
  verifyChecksums,
  verifyLocalAssets,
} from "../assets.ts";
import {
  OTHER_SHA,
  STAGE_REPO,
  STAGE_TAG,
  placeholderTagFor,
  startReleaseFake,
  wireTagName,
  writeSyntheticBundle,
} from "./release-fake.ts";
import { scratchDir } from "./support.ts";

const ROUTE = `/repos/${STAGE_REPO}`;

async function call(
  url: string,
  method: string,
  body?: string,
): Promise<{ status: number; json: unknown }> {
  const init: RequestInit = body === undefined ? { method } : { method, body };
  const response = await fetch(url, init);
  const text = await response.text();
  let json: unknown = null;
  try {
    json = JSON.parse(text);
  } catch {
    json = text;
  }
  return { status: response.status, json };
}

test("the fake serves tag ref creation and the matching-refs query", async () => {
  const started = await startReleaseFake();
  try {
    const created = await call(
      `${started.url}${ROUTE}/git/refs`,
      "POST",
      JSON.stringify({ ref: `refs/tags/${STAGE_TAG}`, sha: OTHER_SHA }),
    );
    assert.equal(created.status, 201);
    assert.deepEqual(started.fake.tagObject, {
      type: "commit",
      sha: OTHER_SHA,
    });
    assert.equal(started.fake.tagCreated.length, 1);
    assert.equal(started.fake.tagCreated[0]?.["ref"], `refs/tags/${STAGE_TAG}`);

    started.fake.placeholderRefs = ["untagged-e40f9231e54c9c966daa"];
    const matching = await call(
      `${started.url}${ROUTE}/git/matching-refs/tags/untagged-`,
      "GET",
    );
    assert.equal(matching.status, 200);
    assert.deepEqual(matching.json, [
      {
        ref: "refs/tags/untagged-e40f9231e54c9c966daa",
        object: { type: "commit", sha: "a".repeat(40) },
      },
    ]);
    assert.equal(
      started
        .seen()
        .filter(
          (request) =>
            request.method === "GET" &&
            request.path === `${ROUTE}/git/matching-refs/tags/untagged-`,
        ).length,
      1,
    );

    const tag = await call(
      `${started.url}${ROUTE}/git/ref/tags/${STAGE_TAG}`,
      "GET",
    );
    assert.equal(tag.status, 200);
    assert.deepEqual(tag.json, {
      ref: `refs/tags/${STAGE_TAG}`,
      object: { type: "commit", sha: OTHER_SHA },
    });
  } finally {
    await started.close();
  }
});

test("the fake runs the PATCH hook after the promotion is applied and records the body", async () => {
  const started = await startReleaseFake();
  try {
    const changed = "a body somebody edited during the promotion";
    const release = started.fake.addRelease({
      id: 42,
      tagName: STAGE_TAG,
      name: STAGE_TAG,
      body: "published body",
      draft: true,
    });
    started.fake.afterPatch = () => {
      release.body = changed;
    };
    const patched = await call(
      `${started.url}${ROUTE}/releases/42`,
      "PATCH",
      JSON.stringify({ draft: false, make_latest: "true" }),
    );
    assert.equal(patched.status, 200);
    assert.equal(started.fake.release(42)?.body, changed);
    assert.equal((patched.json as { body: string }).body, changed);
    assert.equal(started.fake.release(42)?.draft, false);
    assert.equal(started.fake.patched.length, 1);
    assert.equal(started.fake.patched[0]?.["make_latest"], "true");
    assert.deepEqual(started.seen(), [
      { method: "PATCH", path: `${ROUTE}/releases/42`, query: "" },
    ]);
  } finally {
    await started.close();
  }
});

test("the fake models make_latest: only a published non-pre-release PATCH makes latest, and latestScript overrides", async () => {
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
      tagName: "v1.2.4",
      name: "v1.2.4",
      draft: true,
    });
    const latest = async (): Promise<[number, number]> => {
      const answer = await call(
        `${started.url}${ROUTE}/releases/latest`,
        "GET",
      );
      return [answer.status, (answer.json as { id?: number }).id ?? 0];
    };
    const patch = (id: number, body: object) =>
      call(
        `${started.url}${ROUTE}/releases/${id}`,
        "PATCH",
        JSON.stringify(body),
      );

    assert.deepEqual(await latest(), [404, 0], "latest before any publish");
    await patch(42, { draft: false, prerelease: false, make_latest: "true" });
    assert.deepEqual(await latest(), [200, 42]);
    await patch(43, { draft: false, prerelease: true, make_latest: "false" });
    assert.deepEqual(
      await latest(),
      [200, 42],
      "a pre-release publish must not move latest",
    );

    started.fake.latestScript = [0, -1, 43];
    assert.deepEqual(await latest(), [404, 0]);
    assert.deepEqual(await latest(), [500, 0]);
    assert.deepEqual(await latest(), [200, 43]);
    assert.deepEqual(await latest(), [200, 42]);
  } finally {
    await started.close();
  }
});

test("the placeholder model reports untagged-<hex id> for a draft, keeps the real tag after a PATCH that carries it, and publishes under the placeholder when the PATCH omits it", async () => {
  const started = await startReleaseFake();
  try {
    started.fake.placeholderDraftTags = true;
    const withTag = started.fake.addRelease({
      id: 42,
      tagName: STAGE_TAG,
      name: STAGE_TAG,
      draft: true,
    });
    const without = started.fake.addRelease({
      id: 255,
      tagName: STAGE_TAG,
      name: STAGE_TAG,
      draft: true,
    });
    assert.equal(wireTagName(withTag), "untagged-2a");
    assert.equal(placeholderTagFor(without), "untagged-ff");

    await call(
      `${started.url}${ROUTE}/releases/42`,
      "PATCH",
      JSON.stringify({ draft: false, tag_name: STAGE_TAG }),
    );
    await call(
      `${started.url}${ROUTE}/releases/255`,
      "PATCH",
      JSON.stringify({ draft: false }),
    );
    assert.equal(wireTagName(withTag), STAGE_TAG);
    assert.equal(wireTagName(without), "untagged-ff");
  } finally {
    await started.close();
  }
});

test("scripted reads answer the scripted status with a body that says 404, then answer normally", async () => {
  const started = await startReleaseFake();
  try {
    started.fake.addRelease({
      id: 42,
      tagName: STAGE_TAG,
      name: STAGE_TAG,
      draft: true,
    });
    started.fake.readByIdCodes = [500, 404];
    const first = await call(`${started.url}${ROUTE}/releases/42`, "GET");
    assert.equal(first.status, 500);
    assert.ok(JSON.stringify(first.json).includes("404"));
    assert.equal(
      (await call(`${started.url}${ROUTE}/releases/42`, "GET")).status,
      404,
    );
    assert.equal(
      (await call(`${started.url}${ROUTE}/releases/42`, "GET")).status,
      200,
    );
  } finally {
    await started.close();
  }
});

test("a scripted patchStatus answers that status, records the body and leaves the release alone", async () => {
  const started = await startReleaseFake();
  try {
    const release = started.fake.addRelease({
      id: 42,
      tagName: STAGE_TAG,
      name: STAGE_TAG,
      draft: true,
    });
    started.fake.patchStatus = 500;
    const patched = await call(
      `${started.url}${ROUTE}/releases/42`,
      "PATCH",
      JSON.stringify({ draft: false }),
    );
    assert.equal(patched.status, 500);
    assert.equal(started.fake.patched.length, 1);
    assert.equal(release.draft, true);
    assert.deepEqual(started.fake.latestReadTimes, []);
    await call(`${started.url}${ROUTE}/releases/latest`, "GET");
    assert.equal(started.fake.latestReadTimes.length, 1);
  } finally {
    await started.close();
  }
});

test("uploads number assets from 1000 and the asset route redirects when redirectAssets is on", async () => {
  const started = await startReleaseFake();
  try {
    const release = started.fake.addRelease({
      id: 42,
      tagName: STAGE_TAG,
      name: STAGE_TAG,
      draft: true,
    });
    const upload = await fetch(
      `${started.url}${ROUTE}/releases/42/assets?name=SHA256SUMS`,
      { method: "POST", body: Buffer.from("sums") },
    );
    assert.equal(upload.status, 201);
    assert.equal(release.assets[0]?.id, 1000);
    assert.equal(release.assets[0]?.contents.toString("utf8"), "sums");

    const direct = await fetch(`${started.url}${ROUTE}/releases/assets/1000`);
    assert.equal(await direct.text(), "sums");

    started.fake.redirectAssets = true;
    const redirected = await fetch(
      `${started.url}${ROUTE}/releases/assets/1000`,
      { redirect: "manual" },
    );
    assert.equal(redirected.status, 302);
    assert.equal(
      redirected.headers.get("location"),
      "/release-asset-bytes/1000",
    );
    assert.equal(
      await (await fetch(`${started.url}/release-asset-bytes/1000`)).text(),
      "sums",
    );
  } finally {
    await started.close();
  }
});

test("writeSyntheticBundle writes a bundle that the local verifiers accept and a flipped byte breaks", () => {
  const dir = join(scratchDir("agentics-bundle-"), "assets");
  writeSyntheticBundle(dir, STAGE_TAG);
  assert.doesNotThrow(() => verifyLocalAssets(dir, STAGE_TAG));
  assert.doesNotThrow(() => verifyChecksums(dir, STAGE_TAG));
  assert.equal(expectedAssetNames(STAGE_TAG).length, 10);

  writeFileSync(join(dir, "THIRD_PARTY_NOTICES.md"), "changed\n");
  assert.throws(
    () => verifyChecksums(dir, STAGE_TAG),
    /SHA256SUMS digest mismatch for THIRD_PARTY_NOTICES.md/,
  );
});
