import assert from "node:assert/strict";
import { writeFileSync } from "node:fs";
import { join } from "node:path";
import { test } from "node:test";
import {
  captureIo,
  commitFiles,
  initRepo,
  scratchDir,
  sh,
  writeManifests,
} from "../testing/support.ts";
import { setVersion } from "../version.ts";
import { run } from "./check-commit.ts";

// The author and committer name the release job gives the release commit.
const BOT = "agentics-release[bot]";
const REQUIRED =
  "release: check-commit: --checkout, --sha, --tag, and --author are required\n";

// A repository, the parent SHA, and the SHA of a release commit that raises the
// four manifests to version, made with the given author and committer names.
function releaseCommitAs(
  version: string,
  authorName: string,
  committerName: string,
): { root: string; parent: string; commit: string } {
  const root = scratchDir("agentics-check-commit-");
  initRepo(root);
  writeManifests(root, "1.2.3");
  const parent = commitFiles(
    root,
    { "THIRD_PARTY_NOTICES.md": "notices\n" },
    "initial",
  );
  setVersion(root, version);
  sh(root, "git", "add", "-A");
  sh(
    root,
    "git",
    "-c",
    `author.name=${authorName}`,
    "-c",
    `committer.name=${committerName}`,
    "-c",
    "user.email=1+agentics-release[bot]@users.noreply.github.com",
    "commit",
    "--quiet",
    "-m",
    `chore(release): v${version} [skip ci]`,
  );
  return { root, parent, commit: sh(root, "git", "rev-parse", "HEAD").trim() };
}

// A valid release commit for v1.3.0 authored and committed by the bot.
function releaseCommitFixture(): {
  root: string;
  parent: string;
  commit: string;
} {
  return releaseCommitAs("1.3.0", BOT, BOT);
}

async function checkCommit(
  root: string,
  ...args: string[]
): Promise<{ code: number; stdout: string; stderr: string }> {
  const captured = captureIo();
  const code = await run(["--checkout", root, ...args], captured.io, {});
  return { code, stdout: captured.stdout(), stderr: captured.stderr() };
}

function failure(message: string): {
  code: number;
  stdout: string;
  stderr: string;
} {
  return { code: 1, stdout: "", stderr: `release: check-commit: ${message}\n` };
}

test("check-commit accepts a release commit with and without --parent", async () => {
  const { root, parent, commit } = releaseCommitFixture();
  sh(root, "git", "tag", "v1.2.3", parent);
  for (const args of [
    ["--sha", commit, "--parent", parent, "--tag", "v1.3.0", "--author", BOT],
    ["--sha", commit, "--tag", "v1.3.0", "--author", BOT],
    [
      `--sha=${commit}`,
      `--parent=${parent}`,
      "--tag=v1.3.0",
      `--author=${BOT}`,
    ],
  ]) {
    assert.deepEqual(await checkCommit(root, ...args), {
      code: 0,
      stdout: "",
      stderr: "",
    });
  }
});

test("check-commit accepts a tag of the release name that already points at the commit", async () => {
  const { root, parent, commit } = releaseCommitFixture();
  sh(root, "git", "tag", "v1.3.0", commit);
  const result = await checkCommit(
    root,
    "--sha",
    commit,
    "--parent",
    parent,
    "--tag",
    "v1.3.0",
    "--author",
    BOT,
  );
  assert.equal(result.code, 0, result.stderr);
});

// An annotated tag names a tag object, not the commit, so only a peeled read
// sees that the release tag already points at the release commit.
test("check-commit accepts an annotated tag at the commit", async () => {
  const { root, parent, commit } = releaseCommitFixture();
  sh(root, "git", "tag", "-a", "v1.3.0", "-m", "v1.3.0", commit);
  const result = await checkCommit(
    root,
    "--sha",
    commit,
    "--parent",
    parent,
    "--tag",
    "v1.3.0",
    "--author",
    BOT,
  );
  assert.equal(result.code, 0, result.stderr);
});

// A prerelease or malformed v* tag is outside the release order, so a higher
// one must not block the release. The last name overflows a machine int in
// its minor part.
test("check-commit ignores tags outside the release order", async () => {
  const { root, parent, commit } = releaseCommitFixture();
  for (const name of [
    "v1.4.0-rc.1",
    "v9.0.0-rc.1",
    "vfoo",
    "v2.0",
    "v2.99999999999999999999.0",
  ]) {
    sh(root, "git", "tag", name, parent);
  }
  const result = await checkCommit(
    root,
    "--sha",
    commit,
    "--parent",
    parent,
    "--tag",
    "v1.3.0",
    "--author",
    BOT,
  );
  assert.equal(result.code, 0, result.stderr);
});

test("check-commit rejects a wrong parent, author, tag or tag order", async () => {
  const { root, parent, commit } = releaseCommitFixture();
  sh(root, "git", "tag", "v1.9.0", parent);
  const cases: [string, string[], string][] = [
    [
      "a wrong parent",
      ["--sha", commit, "--parent", commit, "--tag", "v1.3.0", "--author", BOT],
      `release commit parent is ${parent}, want ${commit}`,
    ],
    [
      "a different expected author",
      [
        "--sha",
        commit,
        "--parent",
        parent,
        "--tag",
        "v1.3.0",
        "--author",
        "someone-else",
      ],
      'release commit author is "agentics-release[bot]", want "someone-else"',
    ],
    [
      "a tag that is not the version",
      ["--sha", commit, "--parent", parent, "--tag", "v1.3.1", "--author", BOT],
      "tag v1.3.1 does not match version-at-commit v1.3.0",
    ],
    [
      "a tag not above the highest",
      ["--sha", commit, "--parent", parent, "--tag", "v1.3.0", "--author", BOT],
      "tag v1.3.0 is not greater than existing tag v1.9.0",
    ],
  ];
  for (const [name, args, message] of cases) {
    assert.deepEqual(await checkCommit(root, ...args), failure(message), name);
  }
});

// Each fixture gets exactly one name wrong, so removing either name check lets
// its commit through.
test("check-commit rejects a wrong author name and a wrong committer name", async () => {
  const cases: [string, string, string, string][] = [
    [
      "author",
      "someone-else",
      BOT,
      'release commit author is "someone-else", want "agentics-release[bot]"',
    ],
    [
      "committer",
      BOT,
      "someone-else",
      'release commit committer is "someone-else", want "agentics-release[bot]"',
    ],
  ];
  for (const [name, authorName, committerName, message] of cases) {
    const { root, parent, commit } = releaseCommitAs(
      "1.3.0",
      authorName,
      committerName,
    );
    assert.deepEqual(
      await checkCommit(
        root,
        "--sha",
        commit,
        "--parent",
        parent,
        "--tag",
        "v1.3.0",
        "--author",
        BOT,
      ),
      failure(message),
      name,
    );
  }
});

// v1.03.0 is a stable tag whose numbers equal the release's, so it is not
// lower and must block the release just as an exact duplicate would.
test("check-commit rejects a stable tag equal in version", async () => {
  const { root, parent, commit } = releaseCommitFixture();
  sh(root, "git", "tag", "v1.03.0", parent);
  assert.deepEqual(
    await checkCommit(
      root,
      "--sha",
      commit,
      "--parent",
      parent,
      "--tag",
      "v1.3.0",
      "--author",
      BOT,
    ),
    failure("tag v1.3.0 is not greater than existing tag v1.03.0"),
  );
});

test("check-commit compares tags numerically, not as text", async () => {
  const { root, parent, commit } = releaseCommitFixture();
  sh(root, "git", "tag", "v1.20.0", parent);
  sh(root, "git", "tag", "v1.2.99", parent);
  sh(root, "git", "tag", "v0.99.99", parent);
  assert.deepEqual(
    await checkCommit(
      root,
      "--sha",
      commit,
      "--parent",
      parent,
      "--tag",
      "v1.3.0",
      "--author",
      BOT,
    ),
    failure("tag v1.3.0 is not greater than existing tag v1.20.0"),
  );
  sh(root, "git", "tag", "-d", "v1.20.0");
  assert.equal(
    (
      await checkCommit(
        root,
        "--sha",
        commit,
        "--parent",
        parent,
        "--tag",
        "v1.3.0",
        "--author",
        BOT,
      )
    ).code,
    0,
  );
});

// rev-list peels a tag object to its commit, so the SHA the caller passed
// has to be compared with the commit rev-list reports.
test("check-commit rejects the SHA of a tag object", async () => {
  const { root, parent, commit } = releaseCommitFixture();
  sh(
    root,
    "git",
    "tag",
    "-a",
    "release-pointer",
    "-m",
    "release pointer",
    commit,
  );
  const object = sh(root, "git", "rev-parse", "release-pointer").trim();
  assert.notEqual(object, commit);
  assert.deepEqual(
    await checkCommit(
      root,
      "--sha",
      object,
      "--parent",
      parent,
      "--tag",
      "v1.3.0",
      "--author",
      BOT,
    ),
    failure(`${object} is not a commit; it peels to commit ${commit}`),
  );
});

// A bare name resolves refs/<name> before refs/tags/<name>, so only the full
// tag ref finds the tag that really exists.
test("check-commit resolves the tag ref exactly", async () => {
  const { root, parent, commit } = releaseCommitFixture();
  sh(root, "git", "tag", "v1.3.0", parent);
  sh(root, "git", "update-ref", "refs/v1.3.0", commit);
  assert.deepEqual(
    await checkCommit(
      root,
      "--sha",
      commit,
      "--parent",
      parent,
      "--tag",
      "v1.3.0",
      "--author",
      BOT,
    ),
    failure(
      `tag v1.3.0 already points at ${parent}, not the release commit ${commit}`,
    ),
  );
});

test("check-commit rejects the release tag when it points at another commit", async () => {
  const { root, parent, commit } = releaseCommitFixture();
  sh(root, "git", "tag", "v1.3.0", parent);
  assert.deepEqual(
    await checkCommit(
      root,
      "--sha",
      commit,
      "--parent",
      parent,
      "--tag",
      "v1.3.0",
      "--author",
      BOT,
    ),
    failure(
      `tag v1.3.0 already points at ${parent}, not the release commit ${commit}`,
    ),
  );
});

// The release commit's own diff is the one verifyReleaseCommit checks, so the
// command must reach it.
test("check-commit rejects a release commit that touches another path", async () => {
  const root = scratchDir("agentics-check-commit-");
  initRepo(root);
  writeManifests(root, "1.2.3");
  const parent = commitFiles(root, {}, "initial");
  setVersion(root, "1.3.0");
  writeFileSync(join(root, "extra.txt"), "x\n");
  sh(root, "git", "add", "-A");
  sh(
    root,
    "git",
    "-c",
    `user.name=${BOT}`,
    "commit",
    "--quiet",
    "-m",
    "chore(release): v1.3.0 [skip ci]",
  );
  const commit = sh(root, "git", "rev-parse", "HEAD").trim();
  assert.deepEqual(
    await checkCommit(
      root,
      "--sha",
      commit,
      "--parent",
      parent,
      "--tag",
      "v1.3.0",
      "--author",
      BOT,
    ),
    failure('release commit adds "extra.txt"'),
  );
});

// A manifest version past the largest signed 64-bit integer names a tag no
// ordering can place.
test("check-commit rejects a tag whose numbers do not fit a signed 64-bit integer", async () => {
  const huge = "9223372036854775808.0.0";
  const { root, parent, commit } = releaseCommitAs(huge, BOT, BOT);
  assert.deepEqual(
    await checkCommit(
      root,
      "--sha",
      commit,
      "--parent",
      parent,
      "--tag",
      `v${huge}`,
      "--author",
      BOT,
    ),
    failure(`tag v${huge} is not an orderable stable version`),
  );
});

test("check-commit orders tags at the largest signed 64-bit integer", async () => {
  const largest = "9223372036854775807.0.0";
  const { root, parent, commit } = releaseCommitAs(largest, BOT, BOT);
  sh(root, "git", "tag", "v9223372036854775807.0.1", parent);
  assert.deepEqual(
    await checkCommit(
      root,
      "--sha",
      commit,
      "--parent",
      parent,
      "--tag",
      `v${largest}`,
      "--author",
      BOT,
    ),
    failure(
      `tag v${largest} is not greater than existing tag v9223372036854775807.0.1`,
    ),
  );
});

test("check-commit rejects a merge commit", async () => {
  const { root, parent } = releaseCommitFixture();
  sh(root, "git", "checkout", "--quiet", "-b", "side", parent);
  commitFiles(root, { "side.txt": "x\n" }, "side");
  sh(root, "git", "checkout", "--quiet", "-");
  sh(
    root,
    "git",
    "-c",
    `user.name=${BOT}`,
    "merge",
    "--quiet",
    "--no-ff",
    "-m",
    "chore(release): v1.3.0 [skip ci]",
    "side",
  );
  const merge = sh(root, "git", "rev-parse", "HEAD").trim();
  assert.deepEqual(
    await checkCommit(root, "--sha", merge, "--tag", "v1.3.0", "--author", BOT),
    failure(`release commit ${merge} must have exactly one parent, has 2`),
  );
});

test("check-commit rejects a root commit", async () => {
  const root = scratchDir("agentics-check-commit-");
  initRepo(root);
  const first = commitFiles(root, {}, "initial");
  assert.deepEqual(
    await checkCommit(root, "--sha", first, "--tag", "v1.3.0", "--author", BOT),
    failure(`release commit ${first} must have exactly one parent, has 0`),
  );
});

// The first failure wins, so each pair below breaks two rules and must report
// the one that is checked first.
test("check-commit reports the first failing check", async () => {
  const wrongNames = releaseCommitAs("1.3.0", "x", "y");
  sh(wrongNames.root, "git", "tag", "v1.9.0", wrongNames.parent);
  const base = ["--sha", wrongNames.commit, "--tag", "v1.4.0"];
  const parent = ["--parent", wrongNames.commit];

  // parent before author, committer, version and tag
  assert.deepEqual(
    await checkCommit(wrongNames.root, ...base, ...parent, "--author", BOT),
    failure(
      `release commit parent is ${wrongNames.parent}, want ${wrongNames.commit}`,
    ),
  );
  // author before committer, version and tag
  assert.deepEqual(
    await checkCommit(wrongNames.root, ...base, "--author", BOT),
    failure('release commit author is "x", want "agentics-release[bot]"'),
  );
  // committer before version and tag
  assert.deepEqual(
    await checkCommit(wrongNames.root, ...base, "--author", "x"),
    failure('release commit committer is "y", want "x"'),
  );

  const good = releaseCommitFixture();
  sh(good.root, "git", "tag", "v1.9.0", good.parent);
  // the tag/version match before the tag order
  assert.deepEqual(
    await checkCommit(
      good.root,
      "--sha",
      good.commit,
      "--tag",
      "v1.4.0",
      "--author",
      BOT,
    ),
    failure("tag v1.4.0 does not match version-at-commit v1.3.0"),
  );
});

test("check-commit rejects bad arguments before it touches git", async () => {
  const sha = "a".repeat(40);
  const cases: [string, string[], string][] = [
    ["no --sha", ["--tag", "v1.3.0", "--author", BOT], REQUIRED],
    ["no --tag", ["--sha", sha, "--author", BOT], REQUIRED],
    ["no --author", ["--sha", sha, "--tag", "v1.3.0"], REQUIRED],
    [
      "an empty --author",
      ["--sha", sha, "--tag", "v1.3.0", "--author", ""],
      REQUIRED,
    ],
    [
      "a positional argument",
      ["--sha", sha, "--tag", "v1.3.0", "--author", BOT, "extra"],
      `release: check-commit: unexpected argument extra\n${REQUIRED}`,
    ],
    [
      "a short sha",
      ["--sha", "abc123", "--tag", "v1.3.0", "--author", BOT],
      "release: check-commit: --sha must be 40 lowercase hexadecimal characters, not abc123\n",
    ],
    [
      "an uppercase parent",
      [
        "--sha",
        sha,
        "--parent",
        "A".repeat(40),
        "--tag",
        "v1.3.0",
        "--author",
        BOT,
      ],
      `release: check-commit: --parent must be 40 lowercase hexadecimal characters, not ${"A".repeat(40)}\n`,
    ],
    [
      "a bad sha and a bad parent",
      ["--sha", "abc", "--parent", "def", "--tag", "v1.3.0", "--author", BOT],
      "release: check-commit: --sha must be 40 lowercase hexadecimal characters, not abc\n",
    ],
  ];
  for (const [name, args, stderr] of cases) {
    assert.deepEqual(
      await checkCommit(scratchDir("agentics-check-commit-"), ...args),
      { code: 2, stdout: "", stderr },
      name,
    );
  }
});

test("check-commit requires a checkout", async () => {
  const captured = captureIo();
  const code = await run(
    ["--sha", "a".repeat(40), "--tag", "v1.3.0", "--author", BOT],
    captured.io,
    {},
  );
  assert.equal(code, 2);
  assert.equal(captured.stderr(), REQUIRED);
});

test("check-commit reports git's reason when the checkout is not a repository", async () => {
  const sha = "a".repeat(40);
  const result = await checkCommit(
    scratchDir("agentics-check-commit-"),
    "--sha",
    sha,
    "--tag",
    "v1.3.0",
    "--author",
    BOT,
  );
  assert.equal(result.code, 1);
  assert.match(
    result.stderr,
    new RegExp(
      `^release: check-commit: git rev-list --parents -n 1 ${sha}: git exited with code 128: fatal: `,
    ),
  );
});
