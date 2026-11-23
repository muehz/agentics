import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { writeFileSync } from "node:fs";
import { join } from "node:path";
import { test } from "node:test";
import { fileURLToPath } from "node:url";
import {
  captureIo,
  commitFiles,
  initRepo,
  readOutputs,
  scratchDir,
  sh,
  writeManifests,
} from "../testing/support.ts";
import { run } from "./resume-check.ts";

const CLI = fileURLToPath(new URL("../cli.ts", import.meta.url));
const REQUIRED =
  "release: resume-check: --checkout, --sha, and --published are required\n";

// A repository at release v1.2.3, with that tag on its one commit.
function releasedRepo(): string {
  const root = scratchDir("agentics-resume-");
  initRepo(root);
  writeManifests(root, "1.2.3");
  commitFiles(root, {}, "initial");
  sh(root, "git", "tag", "v1.2.3");
  return root;
}

function head(root: string): string {
  return sh(root, "git", "rev-parse", "HEAD").trim();
}

function commitEmpty(root: string, subject: string): void {
  commitFiles(root, {}, subject);
}

// Runs resume-check at HEAD of root with the published tags given, and returns
// the exit code, the parsed GITHUB_OUTPUT file and stderr.
async function resume(
  root: string,
  published: string,
): Promise<{
  code: number;
  outputs: Record<string, string>;
  stdout: string;
  stderr: string;
}> {
  const dir = scratchDir("agentics-resume-");
  const output = join(dir, "output");
  const file = join(dir, "published.txt");
  writeFileSync(file, published);
  writeFileSync(output, "");
  const captured = captureIo();
  const code = await run(
    ["--checkout", root, "--sha", head(root), "--published", file],
    captured.io,
    { GITHUB_OUTPUT: output },
  );
  return {
    code,
    outputs: readOutputs(output),
    stdout: captured.stdout(),
    stderr: captured.stderr(),
  };
}

test("resume-check derives analyze, resume and released from the head", async () => {
  const root = releasedRepo();

  const plain = await resume(root, "v1.2.3\n");
  assert.equal(plain.code, 0, plain.stderr);
  assert.deepEqual(plain.outputs, { mode: "analyze", resume_tag: "" });
  assert.equal(plain.stderr, "");

  // semantic-release pushed the commit and its tag before the publish failed,
  // so the tag being resumed exists and must not warn about itself.
  commitEmpty(root, "chore(release): v1.3.0 [skip ci]");
  sh(root, "git", "tag", "v1.3.0");
  const unreleased = await resume(root, "v1.2.3\n");
  assert.equal(unreleased.code, 0, unreleased.stderr);
  assert.deepEqual(unreleased.outputs, {
    mode: "resume",
    resume_tag: "v1.3.0",
  });
  assert.equal(unreleased.stderr.includes("::warning::"), false);

  const released = await resume(root, "v1.2.3\nv1.3.0\n");
  assert.equal(released.code, 0, released.stderr);
  assert.deepEqual(released.outputs, {
    mode: "released",
    resume_tag: "v1.3.0",
  });
  assert.equal(released.stdout, "");
});

test("resume-check reads a release subject only in its exact shape", async () => {
  for (const subject of [
    "chore(release): v1.3.0",
    "chore(release): v1.3.0 [skip ci] extra",
    "chore(release): v1.3 [skip ci]",
    "chore(release): v1.3.0-rc.1 [skip ci]",
    "Chore(release): v1.3.0 [skip ci]",
    "feat: chore(release): v1.3.0 [skip ci]",
  ]) {
    const root = releasedRepo();
    commitEmpty(root, subject);
    const result = await resume(root, "v1.2.3\n");
    assert.deepEqual(
      result.outputs,
      { mode: "analyze", resume_tag: "" },
      subject,
    );
  }
});

test("resume-check trims white space around the subject and splits the published list on any white space", async () => {
  const root = releasedRepo();
  commitEmpty(root, "chore(release): v1.3.0 [skip ci]");
  const result = await resume(root, "  v1.2.3\r\n\tv1.3.0  \n\n");
  assert.deepEqual(result.outputs, { mode: "released", resume_tag: "v1.3.0" });
});

test("resume-check warns about an older unpublished stable tag and keeps going", async () => {
  const root = releasedRepo();
  commitEmpty(root, "chore(release): v1.2.4 [skip ci]");
  sh(root, "git", "tag", "v1.2.4");
  commitEmpty(root, "feat(skills): add peer mode (#7)");
  // A prerelease tag never has a published stable release and is not a
  // stranded release.
  sh(root, "git", "tag", "v1.2.5-rc.1");

  const result = await resume(root, "v1.2.3\n");
  assert.equal(result.code, 0, result.stderr);
  assert.equal(result.outputs.mode, "analyze");
  assert.equal(
    result.stderr,
    "::warning::v1.2.4 has no published release; settle it by hand\n",
  );
});

test("resume-check lists stranded tags in byte order, once each", async () => {
  const root = releasedRepo();
  sh(root, "git", "tag", "v1.10.0");
  sh(root, "git", "tag", "v1.9.0");
  sh(root, "git", "tag", "v1.100.0");
  // git lists tags in its own order; version order puts v1.9.0 first, so the
  // command has to sort by byte order itself.
  sh(root, "git", "config", "tag.sort", "version:refname");
  const result = await resume(root, "v1.2.3\n");
  assert.equal(result.code, 0);
  assert.equal(
    result.stderr,
    "::warning::v1.10.0 has no published release; settle it by hand\n" +
      "::warning::v1.100.0 has no published release; settle it by hand\n" +
      "::warning::v1.9.0 has no published release; settle it by hand\n",
  );
});

test("resume-check ignores a tag the head does not contain", async () => {
  const root = releasedRepo();
  const base = head(root);
  commitEmpty(root, "feat: on a side line");
  sh(root, "git", "tag", "v1.9.0");
  sh(root, "git", "checkout", "--quiet", base);
  const result = await resume(root, "v1.2.3\n");
  assert.equal(result.stderr, "");
});

test("resume-check rejects bad arguments with exit 2", async () => {
  const sha = "a".repeat(40);
  const cases: [string, string[], string][] = [
    [
      "short sha",
      ["--checkout", ".", "--sha", "abc", "--published", "x"],
      "release: resume-check: --sha must be 40 lowercase hexadecimal characters, not abc\n",
    ],
    [
      "uppercase sha",
      ["--checkout", ".", "--sha", "A".repeat(40), "--published", "x"],
      `release: resume-check: --sha must be 40 lowercase hexadecimal characters, not ${"A".repeat(40)}\n`,
    ],
    ["missing published", ["--checkout", ".", "--sha", sha], REQUIRED],
    ["missing checkout", ["--sha", sha, "--published", "x"], REQUIRED],
    ["missing sha", ["--checkout", ".", "--published", "x"], REQUIRED],
    [
      "empty published",
      ["--checkout", ".", "--sha", sha, "--published", ""],
      REQUIRED,
    ],
    [
      "positional",
      ["--checkout", ".", "--sha", sha, "--published", "x", "extra"],
      `release: resume-check: unexpected argument extra\n${REQUIRED}`,
    ],
  ];
  for (const [name, argv, stderr] of cases) {
    const captured = captureIo();
    const code = await run(argv, captured.io, {});
    assert.equal(code, 2, name);
    assert.equal(captured.stdout(), "", name);
    assert.equal(captured.stderr(), stderr, name);
  }
});

test("resume-check checks the SHA shape before it reads anything else", async () => {
  const captured = captureIo();
  const code = await run(
    [
      "--checkout",
      "/nowhere",
      "--sha",
      "abc",
      "--published",
      "/nowhere/published",
    ],
    captured.io,
    {},
  );
  assert.equal(code, 2);
  assert.match(captured.stderr(), /--sha must be 40 lowercase/);
});

test("resume-check fails with exit 1 when the published list cannot be read", async () => {
  const root = releasedRepo();
  const missing = join(scratchDir("agentics-resume-"), "published.txt");
  const captured = captureIo();
  const code = await run(
    ["--checkout", root, "--sha", head(root), "--published", missing],
    captured.io,
    {},
  );
  assert.equal(code, 1);
  assert.equal(
    captured.stderr(),
    `release: resume-check: ${missing}: does not exist\n`,
  );
});

test("resume-check reads the published list before it runs git", async () => {
  const notARepo = scratchDir("agentics-resume-");
  const missing = join(notARepo, "published.txt");
  const captured = captureIo();
  const code = await run(
    ["--checkout", notARepo, "--sha", "a".repeat(40), "--published", missing],
    captured.io,
    {},
  );
  assert.equal(code, 1);
  assert.equal(
    captured.stderr(),
    `release: resume-check: ${missing}: does not exist\n`,
  );
});

test("resume-check fails with exit 1 and git's reason when the checkout is not a repository", async () => {
  const notARepo = scratchDir("agentics-resume-");
  const published = join(notARepo, "published.txt");
  writeFileSync(published, "");
  const sha = "a".repeat(40);
  const captured = captureIo();
  const code = await run(
    ["--checkout", notARepo, "--sha", sha, "--published", published],
    captured.io,
    {},
  );
  assert.equal(code, 1);
  assert.match(
    captured.stderr(),
    new RegExp(
      `^release: resume-check: git log -1 --format=%s ${sha}: git exited with code 128: fatal: `,
    ),
  );
});

test("resume-check fails with exit 1 when GITHUB_OUTPUT cannot be written", async () => {
  const root = releasedRepo();
  const dir = scratchDir("agentics-resume-");
  const published = join(dir, "published.txt");
  writeFileSync(published, "v1.2.3\n");
  const output = join(dir, "absent", "output");
  const captured = captureIo();
  const code = await run(
    ["--checkout", root, "--sha", head(root), "--published", published],
    captured.io,
    { GITHUB_OUTPUT: output },
  );
  assert.equal(code, 1);
  assert.equal(
    captured.stderr(),
    `release: resume-check: write GITHUB_OUTPUT: unable to write command to the environment file: ${output}: does not exist\n`,
  );
});

// Without GITHUB_OUTPUT the outputs go to the process's real stdout, not to the
// injected writer. Only the process seam can see that.
test("the entry point writes key=value lines to stdout when GITHUB_OUTPUT is unset", () => {
  const root = releasedRepo();
  const published = join(scratchDir("agentics-resume-"), "published.txt");
  writeFileSync(published, "v1.2.3\n");
  const env = { ...process.env };
  delete env.GITHUB_OUTPUT;
  const result = spawnSync(
    process.execPath,
    [
      CLI,
      "resume-check",
      "--checkout",
      root,
      "--sha",
      head(root),
      "--published",
      published,
    ],
    { encoding: "utf8", env },
  );
  assert.equal(result.status, 0, result.stderr);
  assert.equal(result.stdout, "mode=analyze\nresume_tag=\n");
  assert.equal(result.stderr, "");

  const usage = spawnSync(process.execPath, [CLI, "resume-check"], {
    encoding: "utf8",
    env,
  });
  assert.equal(usage.status, 2);
  assert.equal(usage.stderr, REQUIRED);
});
