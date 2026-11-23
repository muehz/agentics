import assert from "node:assert/strict";
import { existsSync, readFileSync, readdirSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { test } from "node:test";
import { placeholderRecovery, strayTagRecovery } from "../identity.ts";
import { toSeen } from "../testing/release-fake.ts";
import {
  captureIo,
  commitFiles,
  initRepo,
  scratchDir,
  sh,
  startServer,
} from "../testing/support.ts";
import {
  LICENSES_PARAGRAPH,
  newerVersion,
  run,
  squashPullNumbers,
  subjectListBaseline,
} from "./context.ts";
import { tagVersion } from "../version.ts";

const REPO = "muehz/agentics";
const TAG = "v2.0.0";
const PRIMARY = "primary-credential";
const DEFAULT_BASELINE = "### Changes\n\n* a change\n";
const REQUIRED =
  "release: context: --repo, --checkout, --sha, --tag, --baseline, and --out are required";
const PLACEHOLDER_REF =
  '[{"ref":"refs/tags/untagged-e40f9231e54c9c966daa","object":{"type":"commit","sha":"129c131cf4711758408ca44bce3e522acf8260d7"}}]';

function git(root: string, ...args: string[]): string {
  return sh(root, "git", ...args).trim();
}

function release(published: string, tag: string): string {
  return `{"tag_name":"${tag}","draft":false,"prerelease":false,"published_at":"${published}"}`;
}

// The first commit carries v1.0.0, v1.8.0 and v1.9.0; the second carries
// v1.1.0; a side branch off the second carries v1.3.0, which the target never reaches; the target carries v2.0.0.
function contextRepo(): { root: string; sha: string } {
  const root = scratchDir("agentics-context-");
  initRepo(root);
  commitFiles(root, { "README.md": "fixture\n" }, "initial");
  for (const tag of ["v1.0.0", "v1.8.0", "v1.9.0"]) git(root, "tag", tag);
  git(root, "commit", "--quiet", "--allow-empty", "-m", "second");
  git(root, "tag", "v1.1.0");
  git(root, "checkout", "--quiet", "-b", "side");
  git(root, "commit", "--quiet", "--allow-empty", "-m", "side");
  git(root, "tag", "v1.3.0");
  git(root, "checkout", "--quiet", "main");
  git(root, "commit", "--quiet", "--allow-empty", "-m", "third");
  git(root, "tag", TAG);
  return { root, sha: git(root, "rev-parse", "HEAD") };
}

function emptyCommit(root: string, subject: string): void {
  git(root, "commit", "--quiet", "--allow-empty", "-m", subject);
}

function argv(
  repo: string,
  checkout: string,
  sha: string,
  tag: string,
  out: string,
  baseline: string | Buffer = DEFAULT_BASELINE,
): string[] {
  const file = join(scratchDir("agentics-baseline-"), "baseline.md");
  writeFileSync(file, baseline);
  return [
    "--repo",
    repo,
    "--checkout",
    checkout,
    "--sha",
    sha,
    "--tag",
    tag,
    "--baseline",
    file,
    "--out",
    out,
  ];
}

interface ServerPlan {
  /** The JSON text of each listing page; a Link header points at the next one. */
  pages?: string[];
  /** The matching-refs answer. */
  refs?: string;
  /** Pull request numbers that answer 404. */
  unreadable?: number[];
}

async function startGithub(plan: ServerPlan = {}) {
  const pages = plan.pages ?? ["[]"];
  const unreadable = new Set(plan.unreadable ?? []);
  const server = await startServer((req, _body, res) => {
    const url = new URL(req.url ?? "/", "http://fake.invalid");
    res.setHeader("Content-Type", "application/json");
    if (url.pathname === `/repos/${REPO}/releases`) {
      const page = Number(url.searchParams.get("page") ?? "1");
      if (page < pages.length) {
        res.setHeader(
          "Link",
          `<https://api.github.com/repos/${REPO}/releases?per_page=100&page=${page + 1}>; rel="next"`,
        );
      }
      res.end(pages[page - 1] ?? "[]");
    } else if (
      url.pathname === `/repos/${REPO}/git/matching-refs/tags/untagged-`
    ) {
      res.end(plan.refs ?? "[]");
    } else if (url.pathname.startsWith(`/repos/${REPO}/pulls/`)) {
      const number = Number(url.pathname.slice(`/repos/${REPO}/pulls/`.length));
      if (unreadable.has(number)) {
        res.writeHead(404);
        res.end('{"message":"Not Found"}');
      } else {
        res.end(
          JSON.stringify({
            number,
            html_url: `https://github.com/${REPO}/pull/${number}`,
            title: `Title ${number}`,
            body: `Body ${number}`,
          }),
        );
      }
    } else {
      res.writeHead(404);
      res.end('{"message":"Not Found"}');
    }
  });
  return { server, env: { GH_TOKEN: PRIMARY, GITHUB_API_URL: server.url } };
}

async function execute(args: string[], env: Record<string, string>) {
  const captured = captureIo();
  const code = await run(args, captured.io, env);
  return { code, stdout: captured.stdout(), stderr: captured.stderr() };
}

function names(dir: string): string[] {
  return readdirSync(dir).sort();
}

function pullPaths(server: {
  requests: readonly { method: string; url: string }[];
}): string[] {
  return toSeen(server.requests)
    .map((request) => request.path)
    .filter((path) => path.includes("/pulls/"));
}

/** A failure path must have asked for the listing and the placeholder scan and for nothing else. */
function assertOnlyListingRequests(server: {
  requests: readonly { method: string; url: string }[];
}): void {
  const allowed = new Set([
    `/repos/${REPO}/releases`,
    `/repos/${REPO}/git/matching-refs/tags/untagged-`,
  ]);
  const seen = toSeen(server.requests);
  assert.ok(
    seen.length > 0,
    "the command made no API request, want at least the release listing",
  );
  for (const request of seen) {
    assert.ok(
      allowed.has(request.path),
      `the command requested ${request.method} ${request.path} on a failure path`,
    );
  }
}

function assertNoOutputFiles(out: string): void {
  for (const name of ["baseline-notes.md", "context.md"]) {
    assert.equal(
      existsSync(join(out, name)),
      false,
      `the command wrote ${name} on the failure path`,
    );
  }
}

test("context picks the highest-version readable ancestor across two listing pages", async () => {
  const { root, sha } = contextRepo();
  const out = join(scratchDir("agentics-out-"), "absent", "context");
  // The first page carries the two newest releases, each rejected by a different filter. v1.5.0 has no tag in
  // the checkout and v1.3.0 sits off the target's history. v1.0.0 is listed ahead of v1.1.0 and was published
  // after it (#609), so only the version order moves v1.1.0 to the front.
  const first = `[${release("2026-06-01T00:00:00Z", "v2.0.0")},{"tag_name":"v1.9.0","draft":true,"prerelease":false,"published_at":"2026-05-01T00:00:00Z"}]`;
  const second = `[${release("2026-03-01T00:00:00Z", "v1.5.0")},${release("2026-02-01T00:00:00Z", "v1.3.0")},${release("2026-01-03T00:00:00Z", "v1.0.0")},${release("2026-01-02T00:00:00Z", "v1.1.0")}]`;
  const { server, env } = await startGithub({ pages: [first, second] });
  try {
    const result = await execute(
      argv(REPO, root, sha, TAG, out, "### Changes\n\n* Change\n"),
      env,
    );
    assert.equal(result.code, 0, result.stderr);
    assert.equal(result.stdout, "");
    assert.equal(result.stderr, "");
    // The two files are the whole artifact; any third name is a regression.
    assert.deepEqual(names(out), ["baseline-notes.md", "context.md"]);
    const wantBaseline = `### Changes\n\n* Change${LICENSES_PARAGRAPH}`;
    assert.equal(
      readFileSync(join(out, "baseline-notes.md"), "utf8"),
      wantBaseline,
    );
    const wantHeader = `# Release context\n\nTarget: ${sha}\nRelease: ${TAG}\nPrevious release: v1.1.0\n\n`;
    assert.ok(
      readFileSync(join(out, "context.md"), "utf8").startsWith(
        wantHeader + wantBaseline,
      ),
    );
    assert.deepEqual(
      toSeen(server.requests)
        .filter((request) => request.path.endsWith("/releases"))
        .map((request) => request.query),
      ["per_page=100", "page=2&per_page=100"],
    );
  } finally {
    await server.close();
  }
});

test("context counts a published pre-release as the start of the range", async () => {
  const { root, sha } = contextRepo();
  const out = join(scratchDir("agentics-out-"), "absent", "context");
  const page = `[${release("2026-06-01T00:00:00Z", "v2.0.0")},{"tag_name":"v1.8.0","draft":false,"prerelease":true,"published_at":"2026-04-01T00:00:00Z"},${release("2026-01-02T00:00:00Z", "v1.1.0")}]`;
  const { server, env } = await startGithub({ pages: [page] });
  try {
    // A CRLF line end and a blank trailing line are trimmed alike.
    const result = await execute(
      argv(REPO, root, sha, TAG, out, "### Changes\n\n* Change\r\n\n"),
      env,
    );
    assert.equal(result.code, 0, result.stderr);
    assert.equal(result.stdout, "");
    assert.equal(result.stderr, "");
    assert.deepEqual(names(out), ["baseline-notes.md", "context.md"]);
    const wantBaseline = `### Changes\n\n* Change${LICENSES_PARAGRAPH}`;
    assert.equal(
      readFileSync(join(out, "baseline-notes.md"), "utf8"),
      wantBaseline,
    );
    const wantHeader = `# Release context\n\nTarget: ${sha}\nRelease: ${TAG}\nPrevious release: v1.8.0\n\n`;
    assert.ok(
      readFileSync(join(out, "context.md"), "utf8").startsWith(
        wantHeader + wantBaseline,
      ),
    );
  } finally {
    await server.close();
  }
});

test("newerVersion orders numerically, highest first, and puts every other name after them", () => {
  const cases: [string, string, number][] = [
    ["v1.10.0", "v1.9.0", -1],
    ["v1.9.0", "v1.10.0", 1],
    ["v2.0.0", "v1.99.99", -1],
    ["v1.2.3", "v1.2.3", 0],
    ["v0.1.0", "v1.2.3-rc1", -1],
    ["v1.2.3-rc1", "v0.1.0", 1],
    ["v1.2.3-rc1", "untagged-abc", 0],
    // 2^63 overflows a machine int, so tagVersion fails even though the tag's shape matches the stable pattern.
    ["v1.0.0", "v9223372036854775808.0.0", -1],
    ["v9223372036854775808.0.0", "v1.0.0", 1],
  ];
  for (const [a, b, want] of cases) {
    assert.equal(
      Math.sign(newerVersion({ tag_name: a }, { tag_name: b })),
      want,
      `newerVersion(${a}, ${b})`,
    );
  }
  assert.deepEqual(tagVersion("v9223372036854775807.0.1"), [
    9223372036854775807n,
    0n,
    1n,
  ]);
  assert.equal(tagVersion("v9223372036854775808.0.0"), undefined);
  assert.equal(tagVersion("v1.2.3-rc1"), undefined);
});

test("context names no previous release for a first release", async () => {
  const { root, sha } = contextRepo();
  const out = join(scratchDir("agentics-out-"), "context");
  const { server, env } = await startGithub({ pages: ["[]"] });
  try {
    const result = await execute(
      argv(REPO, root, sha, TAG, out, "### Changes\n\n* First release\n"),
      env,
    );
    assert.equal(result.code, 0, result.stderr);
    assert.ok(
      readFileSync(join(out, "context.md"), "utf8").includes(
        "Previous release: none (first release)\n",
      ),
    );
    assert.equal(
      readFileSync(join(out, "baseline-notes.md"), "utf8"),
      `### Changes\n\n* First release${LICENSES_PARAGRAPH}`,
    );
  } finally {
    await server.close();
  }
});

test("context fails when no published release is a readable ancestor and writes no file", async () => {
  const { root, sha } = contextRepo();
  const out = join(scratchDir("agentics-out-"), "context");
  const { server, env } = await startGithub({
    pages: [
      `[${release("2026-03-01T00:00:00Z", "v1.5.0")},${release("2026-02-01T00:00:00Z", "v1.3.0")}]`,
    ],
  });
  try {
    const result = await execute(argv(REPO, root, sha, TAG, out), env);
    assert.equal(result.code, 1);
    assert.equal(result.stdout, "");
    assert.equal(
      result.stderr,
      "release: context: no published stable release is a readable ancestor of the target\n",
    );
    // An unverified range must not be enriched, and it must not leave a baseline the publish job could pick up.
    assertOnlyListingRequests(server);
    assertNoOutputFiles(out);
  } finally {
    await server.close();
  }
});

test("context rejects an overflowing version as the previous release even when it is the only readable ancestor", async () => {
  const { root, sha } = contextRepo();
  const out = join(scratchDir("agentics-out-"), "context");
  git(root, "tag", "v9223372036854775808.0.0");
  const { server, env } = await startGithub({
    pages: [`[${release("2026-01-01T00:00:00Z", "v9223372036854775808.0.0")}]`],
  });
  try {
    const result = await execute(argv(REPO, root, sha, TAG, out), env);
    assert.equal(result.code, 1);
    assert.equal(result.stdout, "");
    assert.equal(
      result.stderr,
      "release: context: no published stable release is a readable ancestor of the target\n",
    );
    assertOnlyListingRequests(server);
    assertNoOutputFiles(out);
  } finally {
    await server.close();
  }
});

test("context fails on a release published under a placeholder tag wherever it sits in the listing", async () => {
  const placeholder = (prerelease: boolean) =>
    `{"tag_name":"untagged-e40f9231e54c9c966daa","name":"v1.106.0","draft":false,"prerelease":${prerelease},"published_at":"2026-09-20T17:20:03Z"}`;
  const ordinary = (tag: string, published: string) =>
    `{"tag_name":"${tag}","name":"${tag}","draft":false,"prerelease":false,"published_at":"${published}"}`;
  const cases: Record<string, string> = {
    "placeholder first": `[${placeholder(false)},${ordinary("v1.105.0", "2026-09-19T17:45:14Z")},${ordinary("v1.104.0", "2026-09-19T17:57:04Z")}]`,
    "placeholder after ordinary releases": `[${ordinary("v1.105.0", "2026-09-19T17:45:14Z")},${ordinary("v1.104.0", "2026-09-19T17:57:04Z")},${placeholder(false)}]`,
    "placeholder is a prerelease": `[${placeholder(true)},${ordinary("v1.105.0", "2026-09-19T17:45:14Z")}]`,
  };
  for (const [name, listing] of Object.entries(cases)) {
    const { root, sha } = contextRepo();
    const out = join(scratchDir("agentics-out-"), "context");
    // The fixture also serves the untagged-* ref the placeholder release really has, so the published-release
    // check must be what reports it and not the later, ref-only stray-tag scan.
    const { server, env } = await startGithub({
      pages: [listing],
      refs: PLACEHOLDER_REF,
    });
    try {
      const result = await execute(argv(REPO, root, sha, TAG, out), env);
      assert.equal(result.code, 1, name);
      assert.equal(result.stdout, "", name);
      assert.equal(
        result.stderr,
        `release: context: observed release v1.106.0 is published under placeholder tag untagged-e40f9231e54c9c966daa; ${placeholderRecovery("v1.106.0", "untagged-e40f9231e54c9c966daa")}\n`,
        name,
      );
      assertOnlyListingRequests(server);
      assertNoOutputFiles(out);
    } finally {
      await server.close();
    }
  }
});

test("context ignores a draft that still carries the placeholder tag", async () => {
  const { root, sha } = contextRepo();
  const out = join(scratchDir("agentics-out-"), "context");
  const draft =
    '{"tag_name":"untagged-e40f9231e54c9c966daa","name":"v2.0.0","draft":true,"prerelease":false,"published_at":null}';
  const { server, env } = await startGithub({
    pages: [`[${draft},${release("2026-01-02T00:00:00Z", "v1.1.0")}]`],
  });
  try {
    const result = await execute(argv(REPO, root, sha, TAG, out), env);
    assert.equal(result.code, 0, result.stderr);
    assert.ok(
      readFileSync(join(out, "context.md"), "utf8").includes(
        "Previous release: v1.1.0\n",
      ),
    );
  } finally {
    await server.close();
  }
});

test("context fails on an untagged ref that has no release behind it", async () => {
  const { root, sha } = contextRepo();
  const out = join(scratchDir("agentics-out-"), "context");
  const { server, env } = await startGithub({
    pages: [`[${release("2026-01-02T00:00:00Z", "v1.1.0")}]`],
    refs: PLACEHOLDER_REF,
  });
  try {
    const result = await execute(argv(REPO, root, sha, TAG, out), env);
    assert.equal(result.code, 1);
    assert.equal(
      result.stderr,
      `release: context: observed stray placeholder tag untagged-e40f9231e54c9c966daa on the remote; ${strayTagRecovery("untagged-e40f9231e54c9c966daa")}\n`,
    );
  } finally {
    await server.close();
  }
});

test("context skips a pull request that cannot be read and says so", async () => {
  const { root } = contextRepo();
  for (const subject of ["Earlier change (#41)", "Final change (#42)"])
    emptyCommit(root, subject);
  const sha = git(root, "rev-parse", "HEAD");
  const out = join(scratchDir("agentics-out-"), "context");
  const { server, env } = await startGithub({ unreadable: [41] });
  try {
    const result = await execute(argv(REPO, root, sha, TAG, out), env);
    assert.equal(result.code, 0, result.stderr);
    assert.equal(result.stdout, "");
    assert.equal(
      result.stderr,
      "release: context: PR 41 context unavailable; retaining GitHub baseline evidence\n",
    );
    const text = readFileSync(join(out, "context.md"), "utf8");
    assert.ok(!text.includes("## PR #41"), text);
    for (const part of [
      "## PR #42",
      "Title 42",
      "Body 42",
      "https://github.com/muehz/agentics/pull/42",
    ]) {
      assert.ok(text.includes(part), `context.md lacks ${part}`);
    }
  } finally {
    await server.close();
  }
});

test("context asks for each trailer pull request once, in numeric order, and skips padded and mid-subject numbers", async () => {
  const { root } = contextRepo();
  for (const subject of [
    "Repeated (#41)",
    "Repeated again (#41)",
    "Hundredth (#100)",
    "Ninth (#9)",
    "Mid-subject (#7) is not a trailer",
    "A padded number (#052)",
    "No number at all",
  ]) {
    emptyCommit(root, subject);
  }
  const sha = git(root, "rev-parse", "HEAD");
  const out = join(scratchDir("agentics-out-"), "context");
  const { server, env } = await startGithub();
  try {
    const result = await execute(argv(REPO, root, sha, TAG, out), env);
    assert.equal(result.code, 0, result.stderr);
    assert.equal(result.stderr, "");
    // Numeric order, not commit order: 100 and 9 were committed after 41. 7 is absent because its number sits
    // mid-subject. 52 is absent because the padded number fails the leading-zero rule.
    assert.deepEqual(pullPaths(server), [
      `/repos/${REPO}/pulls/9`,
      `/repos/${REPO}/pulls/41`,
      `/repos/${REPO}/pulls/100`,
    ]);
    const text = readFileSync(join(out, "context.md"), "utf8");
    assert.ok(!text.includes("## PR #52"));
    const indexes = ["## PR #9", "## PR #41", "## PR #100"].map((heading) =>
      text.indexOf(heading),
    );
    assert.ok(
      indexes.every((index) => index >= 0),
      text,
    );
    assert.deepEqual(
      [...indexes].sort((a, b) => a - b),
      indexes,
    );
    assert.equal(text.split("## PR #41").length - 1, 1);
  } finally {
    await server.close();
  }
});

test("context rejects bad arguments with exit 2 before any API request or file", async () => {
  const { root, sha } = contextRepo();
  const { server, env } = await startGithub();
  try {
    const complete = (out: string) => argv(REPO, root, sha, TAG, out);
    const cases: {
      name: string;
      argv: (out: string) => string[];
      want: string;
    }[] = [
      { name: "no flags", argv: () => [], want: REQUIRED },
      {
        name: "missing one flag",
        argv: (out) => [
          "--repo",
          REPO,
          "--sha",
          sha,
          "--tag",
          TAG,
          "--out",
          out,
        ],
        want: REQUIRED,
      },
      {
        name: "missing baseline",
        argv: (out) => [
          "--repo",
          REPO,
          "--checkout",
          root,
          "--sha",
          sha,
          "--tag",
          TAG,
          "--out",
          out,
        ],
        want: REQUIRED,
      },
      {
        name: "empty value counts as missing",
        argv: (out) => argv("", root, sha, TAG, out),
        want: REQUIRED,
      },
      {
        name: "extra positional argument",
        argv: (out) => [...complete(out), "extra"],
        want: REQUIRED,
      },
      {
        name: "flag with no value",
        argv: (out) => complete(out).slice(0, -1),
        want: REQUIRED,
      },
      {
        name: "unknown flag",
        argv: (out) => [...complete(out), "--unknown", "x"],
        want: REQUIRED,
      },
      {
        name: "repository with no owner",
        argv: (out) => argv("agentics", root, sha, TAG, out),
        want: "release: context: --repo must be OWNER/REPO, not agentics",
      },
      {
        name: "repository with an empty part",
        argv: (out) => argv("muehz/", root, sha, TAG, out),
        want: "release: context: --repo must be OWNER/REPO, not muehz/",
      },
      {
        name: "repository with a third part",
        argv: (out) => argv("muehz/agentics/go", root, sha, TAG, out),
        want: "release: context: --repo must be OWNER/REPO, not muehz/agentics/go",
      },
      // A short SHA still resolves locally, so only the shape guard stops it from ending the subject range.
      {
        name: "short SHA",
        argv: (out) => argv(REPO, root, sha.slice(0, 7), TAG, out),
        want: "release: context: --sha must be 40 lowercase hexadecimal characters",
      },
      {
        name: "uppercase SHA",
        argv: (out) => argv(REPO, root, sha.toUpperCase(), TAG, out),
        want: "release: context: --sha must be 40 lowercase hexadecimal characters",
      },
      {
        name: "tag with no v prefix",
        argv: (out) => argv(REPO, root, sha, "2.0.0", out),
        want: "release: context: --tag must be vMAJOR.MINOR.PATCH, not 2.0.0",
      },
      {
        name: "prerelease tag",
        argv: (out) => argv(REPO, root, sha, "v2.0.0-rc.1", out),
        want: "release: context: --tag must be vMAJOR.MINOR.PATCH, not v2.0.0-rc.1",
      },
    ];
    for (const item of cases) {
      const out = scratchDir("agentics-out-");
      const result = await execute(item.argv(out), env);
      assert.equal(result.code, 2, `${item.name}: ${result.stderr}`);
      assert.equal(result.stdout, "", item.name);
      assert.ok(
        result.stderr.includes(item.want),
        `${item.name}: stderr=${result.stderr}`,
      );
      assert.deepEqual(
        names(out),
        [],
        `${item.name}: a rejected argument list wrote a file`,
      );
    }
    assert.equal(
      server.requests.length,
      0,
      "a rejected argument list reached the API",
    );
  } finally {
    await server.close();
  }
});

test("context lists the squash subjects for an empty baseline and leaves the release commit out", async () => {
  const { root } = contextRepo();
  emptyCommit(root, "chore(release): v2.0.0 [skip ci]");
  const sha = git(root, "rev-parse", "HEAD");
  const out = join(scratchDir("agentics-out-"), "context");
  const { server, env } = await startGithub();
  try {
    const result = await execute(argv(REPO, root, sha, TAG, out, ""), env);
    assert.equal(result.code, 0, result.stderr);
    assert.equal(
      readFileSync(join(out, "baseline-notes.md"), "utf8"),
      `## Changes\n\n* initial\n* second\n* third${LICENSES_PARAGRAPH}`,
    );
  } finally {
    await server.close();
  }
});

test("context trims only trailing newlines from the baseline", async () => {
  const { root, sha } = contextRepo();
  const out = join(scratchDir("agentics-out-"), "context");
  const { server, env } = await startGithub();
  try {
    const result = await execute(
      argv(REPO, root, sha, TAG, out, "  ### Changes\n\n* Change  \n\n"),
      env,
    );
    assert.equal(result.code, 0, result.stderr);
    assert.equal(
      readFileSync(join(out, "baseline-notes.md"), "utf8"),
      `  ### Changes\n\n* Change  ${LICENSES_PARAGRAPH}`,
    );
  } finally {
    await server.close();
  }
});

test("context copies the baseline file's bytes verbatim, including bytes that are not UTF-8", async () => {
  const { root, sha } = contextRepo();
  const out = join(scratchDir("agentics-out-"), "context");
  const { server, env } = await startGithub();
  try {
    const baseline = Buffer.from([0x23, 0x20, 0xff, 0xfe, 0x0a, 0x0a]);
    const result = await execute(
      argv(REPO, root, sha, TAG, out, baseline),
      env,
    );
    assert.equal(result.code, 0, result.stderr);
    const written = readFileSync(join(out, "baseline-notes.md"));
    assert.deepEqual(
      written,
      Buffer.concat([
        Buffer.from([0x23, 0x20, 0xff, 0xfe]),
        Buffer.from(LICENSES_PARAGRAPH),
      ]),
    );
  } finally {
    await server.close();
  }
});

test("context fails on an unreadable baseline before any API call or output directory", async () => {
  const { root, sha } = contextRepo();
  const out = join(scratchDir("agentics-out-"), "context");
  const missing = join(scratchDir("agentics-missing-"), "absent.md");
  const { server, env } = await startGithub();
  try {
    const args = [
      "--repo",
      REPO,
      "--checkout",
      root,
      "--sha",
      sha,
      "--tag",
      TAG,
      "--baseline",
      missing,
      "--out",
      out,
    ];
    const result = await execute(args, env);
    assert.equal(result.code, 1);
    assert.equal(result.stdout, "");
    assert.equal(
      result.stderr,
      `release: context: ${missing}: does not exist\n`,
    );
    assert.equal(existsSync(join(out, "baseline-notes.md")), false);
    assert.equal(
      existsSync(out),
      false,
      "the command created the output directory before it read the baseline",
    );
    assert.equal(
      server.requests.length,
      0,
      "the command called the API before it read the baseline",
    );
  } finally {
    await server.close();
  }
});

test("context treats a baseline of blank lines as empty", async () => {
  const { root, sha } = contextRepo();
  const out = join(scratchDir("agentics-out-"), "context");
  const { server, env } = await startGithub({
    pages: [`[${release("2026-01-02T00:00:00Z", "v1.1.0")}]`],
  });
  try {
    const result = await execute(
      argv(REPO, root, sha, TAG, out, " \n\t\n"),
      env,
    );
    assert.equal(result.code, 0, result.stderr);
    assert.equal(
      readFileSync(join(out, "baseline-notes.md"), "utf8"),
      `## Changes\n\n* third${LICENSES_PARAGRAPH}`,
    );
  } finally {
    await server.close();
  }
});

test("context starts the range at the previous release and asks only for later pull requests", async () => {
  const { root } = contextRepo();
  emptyCommit(root, "Shipped change (#5)");
  git(root, "tag", "v1.9.5");
  emptyCommit(root, "New change (#6)");
  const sha = git(root, "rev-parse", "HEAD");
  const out = join(scratchDir("agentics-out-"), "context");
  const { server, env } = await startGithub({
    pages: [`[${release("2026-05-02T00:00:00Z", "v1.9.5")}]`],
  });
  try {
    const result = await execute(argv(REPO, root, sha, TAG, out, ""), env);
    assert.equal(result.code, 0, result.stderr);
    assert.equal(
      readFileSync(join(out, "baseline-notes.md"), "utf8"),
      `## Changes\n\n* New change (#6)${LICENSES_PARAGRAPH}`,
    );
    assert.deepEqual(pullPaths(server), [`/repos/${REPO}/pulls/6`]);
    const text = readFileSync(join(out, "context.md"), "utf8");
    assert.ok(
      text.includes("Previous release: v1.9.5\n") &&
        text.includes("## PR #6\n"),
      text,
    );
  } finally {
    await server.close();
  }
});

test("context writes the heading alone for an empty range", async () => {
  const { root } = contextRepo();
  const out = join(scratchDir("agentics-out-"), "context");
  const second = git(root, "rev-parse", "v1.1.0");
  const { server, env } = await startGithub({
    pages: [`[${release("2026-01-02T00:00:00Z", "v1.1.0")}]`],
  });
  try {
    const result = await execute(argv(REPO, root, second, TAG, out, ""), env);
    assert.equal(result.code, 0, result.stderr);
    assert.equal(
      readFileSync(join(out, "baseline-notes.md"), "utf8"),
      `## Changes${LICENSES_PARAGRAPH}`,
    );
  } finally {
    await server.close();
  }
});

test("squashPullNumbers returns the trailer numbers, deduplicated and ascending", () => {
  assert.deepEqual(
    squashPullNumbers([
      "feat(skills): add peer mode (#12)",
      "fix(hooks): stop a false deny (#9)",
      "docs: clarify (#12)",
      "chore: no number",
      "fix: a (#3) in the middle is not a trailer",
      "padded (#012)",
      "overflow (#9223372036854775808)",
      "trailing space (#4)  ",
    ]),
    [4, 9, 12],
  );
});

test("subjectListBaseline drops only the release commit", () => {
  assert.equal(
    subjectListBaseline([
      "feat: a (#1)",
      "chore(release): v1.3.0 [skip ci]",
      "fix: b (#2)",
      "chore(deps): x (#3)",
    ]),
    "## Changes\n\n* feat: a (#1)\n* fix: b (#2)\n* chore(deps): x (#3)\n",
  );
  assert.equal(subjectListBaseline([]), "## Changes\n\n");
});

test("context stops with the credential message when neither token is set", async () => {
  const { root, sha } = contextRepo();
  const out = join(scratchDir("agentics-out-"), "context");
  const result = await execute(argv(REPO, root, sha, TAG, out), {});
  assert.equal(result.code, 1);
  assert.equal(
    result.stderr,
    "release: context: no GitHub credential: set GH_TOKEN or GITHUB_TOKEN\n",
  );
});
