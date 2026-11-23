import assert from "node:assert/strict";
import { readFileSync, rmSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { test } from "node:test";
import { expectedAssetNames, pluginTarballName } from "../assets.ts";
import {
  OTHER_SHA,
  STAGE_REPO,
  STAGE_SHA,
  STAGE_TAG,
  countRequests,
  countUploads,
  newFakeRelease,
  placeholderTagFor,
  testClient,
  type ReleaseFake,
} from "../testing/release-fake.ts";
import {
  PUBLISHED_BODY,
  STAGE_NOTES,
  assertNoBodyLeak,
  assertNoMethod,
  assertNoOutputs,
  assertNoRemoteWrites,
  assertOutputs,
  newStageFixture,
  readAsset,
  replaceFlag,
  writeAsset,
} from "../testing/stage-fixture.ts";
import { captureIo, scratchDir } from "../testing/support.ts";
import {
  DEFAULT_RETRY_DELAY_MS,
  prepareDraft,
  run,
  type StageOptions,
} from "./stage.ts";

const ROUTE = `/repos/${STAGE_REPO}`;
const NOTICES = "THIRD_PARTY_NOTICES.md";
const CHECKSUMS = "SHA256SUMS";
const REQUIRED =
  "release: stage: --repo, --sha, --tag, --assets, and --notes are required";
const ANNOTATED = `beef${"0".repeat(36)}`;

function stageOptions(assets: string): StageOptions {
  return {
    owner: "muehz",
    name: "agentics",
    sha: STAGE_SHA,
    tag: STAGE_TAG,
    assets,
    body: STAGE_NOTES,
    prerelease: false,
  };
}

test("each stage fixture owns its own bundle, so corrupting one cannot change another", async (t) => {
  const first = await newStageFixture(t);
  const second = await newStageFixture(t);
  assert.notEqual(first.assets, second.assets);
  for (const name of expectedAssetNames(STAGE_TAG)) {
    const before = readAsset(second.assets, name);
    writeAsset(first.assets, name, "corrupted independent copy");
    assert.deepEqual(readAsset(second.assets, name), before, name);
    rmSync(join(first.assets, name));
    assert.deepEqual(readAsset(second.assets, name), before, name);
  }
});

test("stage requires its five flags and rejects positional and unknown arguments with exit 2", async (t) => {
  const fixture = await newStageFixture(t);
  const argv = fixture.argv();
  const cases: { name: string; argv: string[]; want: string }[] = [
    { name: "no flags", argv: [], want: REQUIRED },
    { name: "missing notes", argv: argv.slice(0, -2), want: REQUIRED },
    {
      name: "empty value counts as missing",
      argv: replaceFlag(argv, "--assets", ""),
      want: REQUIRED,
    },
    { name: "positional argument", argv: [...argv, "extra"], want: REQUIRED },
    // --release-id is not a flag of stage: the action passes no such input, and the action test holds the
    // action's inputs and this command's flags to the same set.
    {
      name: "release id",
      argv: [...argv, "--release-id", "42"],
      want: "release-id",
    },
  ];
  for (const item of cases) {
    const result = await fixture.runArgv(item.argv);
    assert.equal(result.code, 2, `${item.name}: ${result.stderr}`);
    assert.ok(
      result.stderr.includes(item.want),
      `${item.name}: stderr=${result.stderr}`,
    );
    assert.ok(
      result.stderr.includes(REQUIRED),
      `${item.name}: stderr=${result.stderr}`,
    );
  }
  assert.equal(
    fixture.seen().length,
    0,
    "argument validation made API requests",
  );
});

test("stage rejects a malformed sha, tag or repo with exit 2 before any API request", async (t) => {
  const fixture = await newStageFixture(t);
  const cases: { name: string; flag: string; value: string; want: string }[] = [
    {
      name: "short sha",
      flag: "--sha",
      value: "a".repeat(39),
      want: "--sha must be 40 lowercase hexadecimal characters",
    },
    {
      name: "uppercase sha",
      flag: "--sha",
      value: "A".repeat(40),
      want: "--sha must be 40 lowercase hexadecimal characters",
    },
    {
      name: "prerelease tag",
      flag: "--tag",
      value: "v1.2.3-rc1",
      want: "--tag must be vMAJOR.MINOR.PATCH",
    },
    {
      name: "tag without prefix",
      flag: "--tag",
      value: "1.2.3",
      want: "--tag must be vMAJOR.MINOR.PATCH",
    },
    {
      name: "repo without owner",
      flag: "--repo",
      value: "agentics",
      want: "--repo must be OWNER/REPO",
    },
    {
      name: "repo with three parts",
      flag: "--repo",
      value: "muehz/agentics/extra",
      want: "--repo must be OWNER/REPO",
    },
  ];
  for (const item of cases) {
    const result = await fixture.runArgv(
      replaceFlag(fixture.argv(), item.flag, item.value),
    );
    assert.equal(result.code, 2, `${item.name}: ${result.stderr}`);
    assert.ok(
      result.stderr.includes(`release: stage: ${item.want}`),
      `${item.name}: stderr=${result.stderr}`,
    );
  }
  assert.equal(
    fixture.seen().length,
    0,
    "a malformed identifier reached the API",
  );
});

test("stage accepts only true or false for --prerelease, and an empty value is a bad value, not a missing one", async (t) => {
  const fixture = await newStageFixture(t);
  for (const value of ["", "yes", "TRUE", "1", "false "]) {
    const result = await fixture.run(["--prerelease", value]);
    assert.equal(result.code, 2, `${JSON.stringify(value)}: ${result.stderr}`);
    assert.ok(
      result.stderr.includes(
        `release: stage: --prerelease must be true or false, not ${value}`,
      ),
      result.stderr,
    );
  }
  assert.equal(
    fixture.seen().length,
    0,
    "a malformed --prerelease reached the API",
  );
});

test("stage verifies the local bundle before any API request", async (t) => {
  const ten = `release: stage: assets must contain exactly the ten expected names for ${STAGE_TAG}`;
  const cases: {
    name: string;
    corrupt: (assets: string) => void;
    want: string;
  }[] = [
    {
      name: "extra local asset",
      corrupt: (assets) => writeAsset(assets, "stray.txt", "stray\n"),
      want: ten,
    },
    {
      name: "missing local asset",
      corrupt: (assets) => rmSync(join(assets, NOTICES)),
      want: ten,
    },
    {
      name: "missing plugin archive",
      corrupt: (assets) => rmSync(join(assets, pluginTarballName(STAGE_TAG))),
      want: ten,
    },
    {
      name: "absent candidate assets",
      corrupt: (assets) => {
        for (const name of expectedAssetNames(STAGE_TAG))
          rmSync(join(assets, name));
      },
      want: ten,
    },
    {
      name: "corrupted asset byte",
      corrupt: (assets) => {
        const contents = readAsset(assets, NOTICES);
        contents[0] = (contents[0] ?? 0) ^ 0xff;
        writeAsset(assets, NOTICES, contents);
      },
      want: `release: stage: SHA256SUMS digest mismatch for ${NOTICES}`,
    },
    {
      name: "truncated SHA256SUMS",
      corrupt: (assets) => {
        const lines = readAsset(assets, CHECKSUMS)
          .toString("utf8")
          .split(/(?<=\n)/);
        writeAsset(assets, CHECKSUMS, lines.slice(0, 3).join(""));
      },
      want: "release: stage: SHA256SUMS must contain exactly nine sorted asset lines",
    },
  ];
  for (const item of cases) {
    const fixture = await newStageFixture(t);
    item.corrupt(fixture.assets);
    const result = await fixture.run();
    assert.equal(result.code, 1, `${item.name}: ${result.stderr}`);
    assert.ok(
      result.stderr.includes(item.want),
      `${item.name}: stderr=${result.stderr}`,
    );
    assertNoRemoteWrites(fixture.seen());
    assert.equal(
      fixture.seen().length,
      0,
      `${item.name}: a broken bundle reached the API`,
    );
    assertNoOutputs(fixture.output);
  }
});

test("stage rejects a missing notes file with exit 2 before any API request", async (t) => {
  const fixture = await newStageFixture(t);
  const missing = join(scratchDir("agentics-notes-"), "absent.md");
  const result = await fixture.runArgv(
    replaceFlag(fixture.argv(), "--notes", missing),
  );
  assert.equal(result.code, 2);
  assert.equal(
    result.stderr,
    `release: stage: notes file does not exist: ${missing}\n`,
  );
  assert.equal(fixture.seen().length, 0);
});

test("stage rejects a notes file that is not valid UTF-8 with exit 2 before any API request", async (t) => {
  const fixture = await newStageFixture(t);
  writeFileSync(fixture.notes, Buffer.from([0x23, 0x20, 0xc3, 0x28, 0x0a]));
  const result = await fixture.run();
  assert.equal(result.code, 2);
  assert.equal(
    result.stderr,
    `release: stage: notes file is not valid UTF-8: ${fixture.notes}\n`,
  );
  assert.equal(fixture.seen().length, 0);
});

test("stage stops with the credential message before any request when no token is set", async (t) => {
  const fixture = await newStageFixture(t);
  const captured = captureIo();
  const code = await run(fixture.argv(), captured.io, {
    GITHUB_API_URL: fixture.url,
  });
  assert.equal(code, 1);
  assert.equal(
    captured.stderr(),
    "release: stage: no GitHub credential: set GH_TOKEN or GITHUB_TOKEN\n",
  );
  assert.equal(fixture.seen().length, 0);
});

test("stage rejects a tag at another commit before any write", async (t) => {
  const cases: {
    name: string;
    plan: (fake: ReleaseFake) => void;
    want: string;
    cleanup: boolean;
  }[] = [
    {
      name: "lightweight tag elsewhere",
      plan: (fake) => {
        fake.tagObject = { type: "commit", sha: OTHER_SHA };
      },
      want: `release: stage: observed tag ${STAGE_TAG} does not target `,
      cleanup: true,
    },
    {
      name: "annotated tag peels elsewhere",
      plan: (fake) => {
        fake.tagObject = { type: "tag", sha: ANNOTATED };
        fake.annotatedTags.set(ANNOTATED, OTHER_SHA);
      },
      want: `release: stage: observed tag ${STAGE_TAG} does not target `,
      cleanup: true,
    },
    {
      name: "annotated tag cannot be peeled",
      plan: (fake) => {
        fake.tagObject = { type: "tag", sha: ANNOTATED };
      },
      want: `release: stage: cannot peel observed tag ${STAGE_TAG}`,
      cleanup: false,
    },
  ];
  for (const item of cases) {
    const fixture = await newStageFixture(t);
    item.plan(fixture.fake);
    const result = await fixture.run();
    assert.equal(result.code, 1, `${item.name}: ${result.stderr}`);
    assert.ok(
      result.stderr.includes(item.want),
      `${item.name}: stderr=${result.stderr}`,
    );
    assert.equal(
      result.stderr.includes("--cleanup-tag"),
      item.cleanup,
      `${item.name}: stderr=${result.stderr}`,
    );
    assert.equal(
      fixture.fake.created.length,
      0,
      `${item.name}: a release was created despite a tag conflict`,
    );
    assertNoMethod(fixture.seen(), "POST");
    assertNoMethod(fixture.seen(), "PATCH");
    assertNoOutputs(fixture.output);
  }
});

test("stage rereads the tag after the draft create and catches a tag that moved", async (t) => {
  const fixture = await newStageFixture(t);
  // This run created the tag itself, but the tag moves elsewhere between the draft create and the reread that
  // follows it. The reread is the only check that can still catch it.
  fixture.fake.afterCreate = (fake) => {
    fake.tagObject = { type: "commit", sha: OTHER_SHA };
  };
  const result = await fixture.run();
  assert.equal(result.code, 1, result.stderr);
  assert.ok(
    result.stderr.includes(
      `release: stage: observed tag ${STAGE_TAG} does not target ${fixture.sha}`,
    ),
    result.stderr,
  );
  assert.equal(
    countRequests(fixture.seen(), "GET", `${ROUTE}/git/ref/tags/${STAGE_TAG}`),
    3,
  );
});

test("stage reads a new draft four times, writes three retry lines, then fails naming the id", async (t) => {
  const fixture = await newStageFixture(t);
  fixture.fake.readByIdCodes = [404, 404, 404, 404];
  const result = await fixture.run();
  assert.equal(result.code, 1, result.stderr);
  for (const attempt of [1, 2, 3]) {
    assert.ok(
      result.stderr.includes(
        `release: stage: draft 100 not readable yet, attempt ${attempt} of 3`,
      ),
      result.stderr,
    );
  }
  assert.equal(result.stderr.split("not readable yet").length - 1, 3);
  assert.ok(
    result.stderr.includes(
      `release: stage: release ${STAGE_TAG} (id 100) was not readable after create`,
    ),
    result.stderr,
  );
  assert.equal(
    countRequests(fixture.seen(), "GET", `${ROUTE}/releases/100`),
    4,
  );
  assertNoOutputs(fixture.output);
});

test("the retry delay is a parameter of run: a nonzero delay is waited between reads and the default is two seconds", async (t) => {
  assert.equal(DEFAULT_RETRY_DELAY_MS, 2000);
  const fixture = await newStageFixture(t);
  fixture.fake.readByIdCodes = [404, 404, 404, 404];
  const started = Date.now();
  const result = await fixture.run([], { retryDelayMs: 60 });
  assert.equal(result.code, 1);
  assert.ok(
    Date.now() - started >= 150,
    "three waits of 60 ms were not observed",
  );
});

test("stage refuses two drafts for the tag before any write", async (t) => {
  const fixture = await newStageFixture(t);
  fixture.draft();
  fixture.fake.addRelease({
    id: 43,
    tagName: STAGE_TAG,
    name: STAGE_TAG,
    draft: true,
    targetCommitish: fixture.sha,
  });
  const result = await fixture.run();
  assert.equal(result.code, 1, result.stderr);
  assert.ok(
    result.stderr.includes(
      `release: stage: multiple releases match ${STAGE_TAG}; refusing ambiguous draft recovery`,
    ),
    result.stderr,
  );
  assertNoMethod(fixture.seen(), "POST");
});

test("stage diagnostics never carry a line of a release body", async (t) => {
  const fixture = await newStageFixture(t);
  // A published release with the wrong title, which stage rejects. Two bodies are in play: the notes file holds
  // STAGE_NOTES and the seeded release holds PUBLISHED_BODY.
  fixture.published().name = `Release ${STAGE_TAG}`;
  const result = await fixture.run();
  assert.equal(result.code, 1, result.stderr);
  assert.ok(result.stderr.length > 0);
  assertNoBodyLeak(result.stderr, STAGE_NOTES, PUBLISHED_BODY);
});

test("stage has no fallback when the release listing fails", async (t) => {
  const fixture = await newStageFixture(t);
  // A fallback here would create a second release beside a draft the listing failed to show.
  fixture.fake.listStatus = 500;
  const result = await fixture.run();
  assert.equal(result.code, 1, result.stderr);
  assert.ok(result.stderr.startsWith("release: stage: "), result.stderr);
  assertNoMethod(fixture.seen(), "POST");
});

test("stage reads a 500 whose body says 404 as fatal, not as an absent release", async (t) => {
  const fixture = await newStageFixture(t);
  fixture.fake.readByIdCodes = [500];
  fixture.draft();
  const result = await fixture.run();
  assert.equal(result.code, 1, result.stderr);
  assert.ok(
    result.stderr.includes("404"),
    `the 500 body never reached the diagnostic: ${result.stderr}`,
  );
  assert.ok(
    !result.stderr.includes("not readable after create"),
    result.stderr,
  );
  assertNoMethod(fixture.seen(), "POST");
});

test("stage refuses a read that returns another release than the pinned one", async (t) => {
  const fixture = await newStageFixture(t);
  // A release under another tag, so discovery misses it and the create still runs. The read-by-id route then
  // answers with it whatever id it is asked for, which is the only way a server can trip this guard.
  fixture.fake.addRelease({
    id: 200,
    tagName: "v9.9.9",
    name: "v9.9.9",
    draft: true,
    targetCommitish: fixture.sha,
  });
  fixture.fake.readByIdReturns = 200;
  const result = await fixture.run();
  assert.equal(result.code, 1, result.stderr);
  assert.ok(
    result.stderr.includes(
      "release: stage: pinned release id 100 returned release id 200; refusing a mismatched release ID",
    ),
    result.stderr,
  );
  // The mismatch stops the run before the upload loop, so release 200 keeps the assets it had.
  assert.equal(countUploads(fixture.seen()), 0);
  assertNoMethod(fixture.seen(), "PATCH");
  assertNoOutputs(fixture.output);
});

test("stage rejects a draft with a wrong title or a wrong target and never prints its body", async (t) => {
  const cases: {
    name: string;
    plan: (release: { name: string; targetCommitish: string }) => void;
    want: string;
    absent?: string;
  }[] = [
    {
      name: "wrong title",
      plan: (release) => {
        release.name = `Release ${STAGE_TAG}`;
      },
      want: `release: stage: observed release ${STAGE_TAG} has conflicting identity; use a separately human-approved notes edit for prose-only correction or publish a new patch release for immutable content`,
    },
    {
      name: "wrong target",
      plan: (release) => {
        release.targetCommitish = OTHER_SHA;
      },
      want: `release: stage: observed release ${STAGE_TAG} has targetCommitish ${OTHER_SHA}, expected `,
      // The fixture seeds no tag, so no tag was observed. A message that named an observed tag here would send a
      // maintainer to look for a tag that does not exist.
      absent: "observed tag",
    },
  ];
  for (const item of cases) {
    const fixture = await newStageFixture(t);
    item.plan(fixture.draft());
    const result = await fixture.run();
    assert.equal(result.code, 1, `${item.name}: ${result.stderr}`);
    assert.ok(
      result.stderr.includes(item.want),
      `${item.name}: stderr=${result.stderr}`,
    );
    if (item.absent !== undefined)
      assert.ok(
        !result.stderr.includes(item.absent),
        `${item.name}: stderr=${result.stderr}`,
      );
    assertNoMethod(fixture.seen(), "POST");
    assertNoMethod(fixture.seen(), "PATCH");
    assertNoOutputs(fixture.output);
    assertNoBodyLeak(result.stderr, STAGE_NOTES);
  }
});

test("stage rejects a draft whose roster has a name outside the ten or a duplicate", async (t) => {
  const cases: {
    name: string;
    plan: (fixture: Awaited<ReturnType<typeof newStageFixture>>) => void;
    asset: string;
  }[] = [
    {
      name: "name outside the ten",
      plan: (fixture) => {
        const release = fixture.draft();
        fixture.fake.addAsset(release, fixture.assets, NOTICES);
        const [asset] = release.assets;
        if (asset !== undefined) asset.name = "stray.txt";
      },
      asset: "stray.txt",
    },
    {
      name: "duplicate name",
      plan: (fixture) => {
        const release = fixture.draft();
        fixture.fake.addAsset(release, fixture.assets, NOTICES);
        fixture.fake.addAsset(release, fixture.assets, NOTICES);
      },
      asset: NOTICES,
    },
  ];
  for (const item of cases) {
    const fixture = await newStageFixture(t);
    item.plan(fixture);
    const result = await fixture.run();
    assert.equal(result.code, 1, `${item.name}: ${result.stderr}`);
    assert.ok(
      result.stderr.includes(
        `release: stage: observed asset ${item.asset} on release ${STAGE_TAG} conflicts; publish a new patch release for immutable content`,
      ),
      `${item.name}: stderr=${result.stderr}`,
    );
    assertNoMethod(fixture.seen(), "POST");
  }
});

test("stage verifies a matching published release, writes nothing and reports published=true", async (t) => {
  const fixture = await newStageFixture(t);
  const release = fixture.published();
  fixture.fake.addAllAssets(release, fixture.assets, STAGE_TAG);
  const result = await fixture.run();
  assert.equal(result.code, 0, result.stderr);
  assert.ok(
    !result.stderr.includes("was not applied"),
    `a run that met the state it asked for said its input did not apply: ${result.stderr}`,
  );
  assertNoMethod(fixture.seen(), "POST");
  assertNoMethod(fixture.seen(), "PATCH");
  // The seeded body differs from the notes file, so this proves the notes file never reaches a release that has a body.
  assert.equal(fixture.fake.release(42)?.body, PUBLISHED_BODY);
  assertOutputs(fixture.output, { release_id: "42", published: "true" });
});

test("stage reports a matching published pre-release by its kind and changes nothing", async (t) => {
  const fixture = await newStageFixture(t);
  const release = fixture.published();
  release.prerelease = true;
  fixture.fake.addAllAssets(release, fixture.assets, STAGE_TAG);
  const before = JSON.stringify(release);
  const result = await fixture.run(["--prerelease", "true"]);
  assert.equal(result.code, 0, result.stderr);
  assert.ok(
    result.stderr.includes(
      `release: stage: found published pre-release 42 for ${STAGE_TAG}\n`,
    ),
    result.stderr,
  );
  assert.ok(!result.stderr.includes("was not applied"), result.stderr);
  for (const method of ["POST", "PATCH", "DELETE"])
    assertNoMethod(fixture.seen(), method);
  assert.equal(
    JSON.stringify(fixture.fake.release(42)),
    before,
    "the published release or its assets changed",
  );
  assertOutputs(fixture.output, { release_id: "42", published: "true" });
  assertNoBodyLeak(result.stderr, STAGE_NOTES, PUBLISHED_BODY);
});

test("stage leaves a published pre-release alone for a release run and says its input did not apply", async (t) => {
  const fixture = await newStageFixture(t);
  const release = fixture.published();
  release.prerelease = true;
  fixture.fake.addAllAssets(release, fixture.assets, STAGE_TAG);
  const result = await fixture.run();
  assert.equal(result.code, 0, result.stderr);
  assertNoMethod(fixture.seen(), "PATCH");
  assert.ok(
    result.stderr.includes(
      `release: stage: ${STAGE_TAG} is already published as a pre-release, so the requested release was not applied`,
    ),
    result.stderr,
  );
  assertOutputs(fixture.output, { release_id: "42", published: "true" });
});

test("stage leaves a published release alone for a pre-release run and says its input did not apply", async (t) => {
  const fixture = await newStageFixture(t);
  const release = fixture.published();
  fixture.fake.addAllAssets(release, fixture.assets, STAGE_TAG);
  const result = await fixture.run(["--prerelease", "true"]);
  assert.equal(result.code, 0, result.stderr);
  assertNoMethod(fixture.seen(), "PATCH");
  assert.ok(
    result.stderr.includes(
      `release: stage: ${STAGE_TAG} is already published as a release, so the requested pre-release was not applied`,
    ),
    result.stderr,
  );
  assertOutputs(fixture.output, { release_id: "42", published: "true" });
});

test("stage rejects a published release with no assets, naming the missing one", async (t) => {
  // This is the case that fires first in practice: v1.92.0 through v1.96.0 are published with zero assets.
  const fixture = await newStageFixture(t);
  fixture.published();
  const result = await fixture.run();
  assert.equal(result.code, 1, result.stderr);
  assert.ok(
    result.stderr.includes(
      `release: stage: observed asset <missing expected asset> on release ${STAGE_TAG} conflicts; publish a new patch release for immutable content`,
    ),
    result.stderr,
  );
  assertNoMethod(fixture.seen(), "POST");
  assertNoMethod(fixture.seen(), "PATCH");
  assertNoOutputs(fixture.output);
  assertNoBodyLeak(result.stderr, STAGE_NOTES, PUBLISHED_BODY);
});

test("stage rejects a published release when the tag is not at the target", async (t) => {
  const fixture = await newStageFixture(t);
  const release = fixture.published();
  fixture.fake.addAllAssets(release, fixture.assets, STAGE_TAG);
  // A published release with no tag at the target cannot be the release this run is verifying.
  fixture.fake.tagObject = null;
  const result = await fixture.run();
  assert.equal(result.code, 1, result.stderr);
  assert.ok(
    result.stderr.includes(
      `release: stage: observed tag ${STAGE_TAG} does not target ${fixture.sha}`,
    ),
    result.stderr,
  );
});

test("stage rejects a published release whose roster is complete but one asset has other bytes", async (t) => {
  // The roster can be complete and still wrong. Nine correct names with one wrong body must never report published=true.
  const fixture = await newStageFixture(t);
  const release = fixture.published();
  fixture.fake.addAllAssets(release, fixture.assets, STAGE_TAG);
  const wrong = release.assets[1];
  assert.ok(wrong !== undefined);
  wrong.contents = Buffer.from("bytes that are not the local asset\n");
  const result = await fixture.run();
  assert.equal(result.code, 1, result.stderr);
  assert.ok(
    result.stderr.includes(
      `release: stage: observed asset ${wrong.name} on release ${STAGE_TAG} conflicts; publish a new patch release for immutable content`,
    ),
    result.stderr,
  );
  assertNoMethod(fixture.seen(), "POST");
  assertNoMethod(fixture.seen(), "PATCH");
  assertNoOutputs(fixture.output);
  assertNoBodyLeak(result.stderr, STAGE_NOTES, PUBLISHED_BODY);
});

test("stage checks every field of the create response before it pins the id", async (t) => {
  const cases: {
    name: string;
    returns: Record<string, unknown>;
    want: string;
  }[] = [
    {
      name: "answers with a published release",
      returns: { draft: false },
      want: `release: stage: observed release ${STAGE_TAG} has conflicting identity`,
    },
    {
      name: "answers with another tag",
      returns: { tag_name: "v9.9.9" },
      want: `release: stage: observed release ${STAGE_TAG} has conflicting identity`,
    },
    {
      name: "answers with no usable id",
      returns: { id: 0 },
      want: `release: stage: release ${STAGE_TAG} has no valid numeric id`,
    },
  ];
  for (const item of cases) {
    const fixture = await newStageFixture(t);
    // The tag already sits at the target, so a non-draft answer is rejected by the draft check and not by the tag check.
    fixture.tagged();
    fixture.fake.createReturns = item.returns;
    const result = await fixture.run();
    assert.equal(result.code, 1, `${item.name}: ${result.stderr}`);
    assert.ok(
      result.stderr.includes(item.want),
      `${item.name}: stderr=${result.stderr}`,
    );
    // A rejected create response never becomes a pinned id, so nothing is uploaded onto the release it named.
    assert.equal(countUploads(fixture.seen()), 0, item.name);
    assertNoOutputs(fixture.output);
  }
});

test("stage rejects a draft whose reread comes back published, before it compares ids", async (t) => {
  const fixture = await newStageFixture(t);
  // The create answers with a draft, but the read-back of the pinned id returns a published release.
  fixture.fake.addRelease({
    id: 200,
    tagName: "v9.9.9",
    name: "v9.9.9",
    draft: false,
    targetCommitish: fixture.sha,
  });
  fixture.fake.readByIdReturns = 200;
  const result = await fixture.run();
  assert.equal(result.code, 1, result.stderr);
  assert.ok(
    result.stderr.includes(
      `release: stage: observed release ${STAGE_TAG} has conflicting identity`,
    ),
    result.stderr,
  );
  assert.ok(!result.stderr.includes("pinned release id"), result.stderr);
  assert.equal(countUploads(fixture.seen()), 0);
  assertNoOutputs(fixture.output);
});

test("stage refuses a listed release that has no id and does not create another", async (t) => {
  const fixture = await newStageFixture(t);
  // addRelease assigns an id when the caller leaves it zero, so the release is appended directly. The listing is
  // the only read that can reach it.
  fixture.fake.releases.push(
    newFakeRelease({
      id: 0,
      tagName: STAGE_TAG,
      name: STAGE_TAG,
      draft: true,
      targetCommitish: fixture.sha,
    }),
  );
  const result = await fixture.run();
  assert.equal(result.code, 1, result.stderr);
  assert.ok(
    result.stderr.includes(
      `release: stage: observed release ${STAGE_TAG} has no valid id`,
    ),
    result.stderr,
  );
  assertNoMethod(fixture.seen(), "POST");
});

test("stage reads a failing tag route as fatal and creates nothing", async (t) => {
  const fixture = await newStageFixture(t);
  // A 500 on the tag route is not "no tag". Reading it as absence would let stage create a draft over a tag it never read.
  fixture.fake.tagRefStatus = 500;
  const result = await fixture.run();
  assert.equal(result.code, 1, result.stderr);
  assert.ok(result.stderr.startsWith("release: stage: "), result.stderr);
  assertNoMethod(fixture.seen(), "POST");
  assert.equal(fixture.fake.created.length, 0);
  assertNoOutputs(fixture.output);
});

test("stage treats a listed release that 404s on the reread as a disappearance, not as no release", async (t) => {
  const fixture = await newStageFixture(t);
  fixture.draft();
  fixture.fake.readByIdCodes = [404];
  const result = await fixture.run();
  assert.equal(result.code, 1, result.stderr);
  assert.ok(
    result.stderr.includes(
      `release: stage: release ${STAGE_TAG} (id 42) was listed but could not be reread`,
    ),
    result.stderr,
  );
  assertNoMethod(fixture.seen(), "POST");
  assertNoOutputs(fixture.output);
});

test("stage reads a 422 that does not say the reference exists as a failed tag create", async (t) => {
  const fixture = await newStageFixture(t);
  fixture.fake.tagCreateStatus = 422;
  fixture.fake.tagCreateMessage = "Validation Failed";
  const result = await fixture.run();
  assert.equal(result.code, 1, result.stderr);
  assert.ok(
    result.stderr.includes(`cannot create tag ${STAGE_TAG}`) &&
      result.stderr.includes("Validation Failed"),
    result.stderr,
  );
  assert.equal(
    fixture.fake.created.length,
    0,
    "a draft was created after a failed tag create",
  );
});

test("stage finds a release published under the placeholder and names the hazard instead of creating another", async (t) => {
  const fixture = await newStageFixture(t);
  fixture.fake.placeholderDraftTags = true;
  const published = fixture.published();
  published.tagName = placeholderTagFor(published);
  const result = await fixture.run();
  assert.equal(result.code, 1, result.stderr);
  assert.ok(
    result.stderr.includes("published under placeholder tag"),
    result.stderr,
  );
  assert.equal(
    fixture.fake.created.length,
    0,
    "a release was created beside the published one",
  );
});

test("stage logs a created draft before rejecting a mismatched reread id and roster", async (t) => {
  const fixture = await newStageFixture(t);
  fixture.tagged();
  fixture.fake.afterCreate = (fake) => {
    const other = fake.addRelease({ id: 101, draft: true });
    other.assets.push({ id: 1, name: "unexpected", contents: Buffer.alloc(0) });
    fake.readByIdReturns = other.id;
  };
  const result = await fixture.run();
  assert.deepEqual(result, {
    code: 1,
    stderr:
      `release: stage: created draft 100 for ${STAGE_TAG}\n` +
      "release: stage: pinned release id 100 returned release id 101; refusing a mismatched release ID\n",
  });
  assertNoOutputs(fixture.output);
  assert.equal(countUploads(fixture.seen()), 0);
});

test("prepareDraft writes the tag first, creates the draft from the notes and rereads both", async (t) => {
  const fixture = await newStageFixture(t);
  const lines: string[] = [];
  const progress = await prepareDraft(
    testClient(fixture.url),
    stageOptions(fixture.assets),
    { write: (chunk) => lines.push(chunk) },
    0,
  );
  assert.equal(progress.state, "draft");
  assert.equal(progress.id, 100);
  // The draft is created as a pre-release in both modes (ADR 0010), so it is never latest as a draft.
  assert.deepEqual(fixture.fake.created, [
    {
      tag_name: STAGE_TAG,
      target_commitish: fixture.sha,
      name: STAGE_TAG,
      body: STAGE_NOTES,
      draft: true,
      prerelease: true,
    },
  ]);
  assert.deepEqual(fixture.fake.tagCreated, [
    { ref: `refs/tags/${STAGE_TAG}`, sha: fixture.sha },
  ]);
  // The reads are as much of the contract as the writes: the tag is created before the draft, and each create
  // is followed by a fresh tag read because the earlier reading is now older than the write.
  const readTag = {
    method: "GET",
    path: `${ROUTE}/git/ref/tags/${STAGE_TAG}`,
    query: "",
  };
  assert.deepEqual(fixture.seen(), [
    readTag,
    { method: "GET", path: `${ROUTE}/releases/tags/${STAGE_TAG}`, query: "" },
    { method: "GET", path: `${ROUTE}/releases`, query: "per_page=100" },
    { method: "POST", path: `${ROUTE}/git/refs`, query: "" },
    readTag,
    { method: "POST", path: `${ROUTE}/releases`, query: "" },
    { method: "GET", path: `${ROUTE}/releases/100`, query: "" },
    readTag,
  ]);
  assert.deepEqual(lines, [
    `release: stage: created tag ${STAGE_TAG} at ${fixture.sha}\n`,
    `release: stage: created draft 100 for ${STAGE_TAG}\n`,
  ]);
});

test("prepareDraft adopts an existing draft, writing the tag only when it is missing", async (t) => {
  const tagless = await newStageFixture(t);
  tagless.draft();
  const first: string[] = [];
  const adopted = await prepareDraft(
    testClient(tagless.url),
    stageOptions(tagless.assets),
    { write: (chunk) => first.push(chunk) },
    0,
  );
  assert.equal(adopted.state === "draft" && adopted.id, 42);
  assert.equal(
    tagless.fake.created.length,
    0,
    "a second release was created beside the discovered draft",
  );
  assert.equal(
    tagless.fake.tagCreated.length,
    1,
    "adopting a tagless draft must create the tag at the target",
  );
  assert.deepEqual(tagless.fake.tagObject, {
    type: "commit",
    sha: tagless.sha,
  });
  assert.deepEqual(first, [
    `release: stage: created tag ${STAGE_TAG} at ${tagless.sha}\n`,
    `release: stage: adopted draft 42 for ${STAGE_TAG}\n`,
  ]);
  assert.ok(
    countRequests(tagless.seen(), "GET", `${ROUTE}/releases`) >= 1,
    "the listing is the only read that can find a draft",
  );

  const tagged = await newStageFixture(t);
  tagged.tagged();
  tagged.draft();
  const second: string[] = [];
  await prepareDraft(
    testClient(tagged.url),
    stageOptions(tagged.assets),
    { write: (chunk) => second.push(chunk) },
    0,
  );
  assert.equal(
    tagged.fake.tagCreated.length,
    0,
    "stage created a tag that already existed",
  );
  assert.deepEqual(second, [
    `release: stage: adopted draft 42 for ${STAGE_TAG}\n`,
  ]);
});

test("stage keeps the notes file out of a draft it adopts", async (t) => {
  const fixture = await newStageFixture(t);
  const draft = fixture.draft();
  draft.body = "## Reviewed\n\nA human rewrote these notes.\n";
  await prepareDraft(
    testClient(fixture.url),
    stageOptions(fixture.assets),
    { write: () => undefined },
    0,
  );
  assert.equal(readFileSync(fixture.notes, "utf8"), STAGE_NOTES);
  assert.equal(
    fixture.fake.release(42)?.body,
    "## Reviewed\n\nA human rewrote these notes.\n",
  );
  assertNoMethod(fixture.seen(), "PATCH");
});
