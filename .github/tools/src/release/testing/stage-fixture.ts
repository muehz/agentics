import assert from "node:assert/strict";
import { existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { EOL } from "node:os";
import { join } from "node:path";
import type { TestContext } from "node:test";
import { run } from "../commands/stage.ts";
import type { RunOptions } from "../io.ts";
import {
  STAGE_REPO,
  STAGE_SHA,
  STAGE_TAG,
  startReleaseFake,
  writeSyntheticBundle,
  type FakeRelease,
  type ReleaseFake,
  type SeenRequest,
} from "./release-fake.ts";
import { captureIo, scratchDir } from "./support.ts";

/** The draft body. It reaches only the create request: stage never sends it in a PATCH. */
export const STAGE_NOTES = "## What's Changed\n\nThe reviewed baseline body.\n";

/**
 * The body a published release already carries. It differs from STAGE_NOTES on purpose: a rerun with a notes
 * file must leave the canonical body alone, and a seeded body equal to the notes file would prove nothing.
 */
export const PUBLISHED_BODY =
  "## Published release\n\nThe canonical notes from the first run.\n";

const PRIMARY = "primary-credential";
const OUTPUT_DELIMITER = "_GitHubActionsFileCommandDelimeter_";

export interface StageResult {
  code: number;
  stderr: string;
}

/** One prepared run: a nine-asset bundle, a notes file, and a fake server holding the release state. */
export interface StageFixture {
  assets: string;
  notes: string;
  sha: string;
  fake: ReleaseFake;
  /** The base URL of the fake server, as GITHUB_API_URL carries it. */
  url: string;
  /** The GITHUB_OUTPUT file the run writes to. It does not exist until a run writes an output. */
  output: string;
  /** The flags a workflow run passes. */
  argv(): string[];
  env(): Record<string, string>;
  /** Drives the command with the fixture's flags plus extra ones. retryDelayMs is 0 unless a test asks otherwise. */
  run(extra?: string[], options?: RunOptions): Promise<StageResult>;
  /** Drives the command with exactly this argument list. */
  runArgv(argv: string[], options?: RunOptions): Promise<StageResult>;
  seen(): SeenRequest[];
  /** Seeds one draft release for the tag at the fixture's target. */
  draft(): FakeRelease;
  /** Seeds one published release for the tag, with the tag present at the target, as a completed publish leaves it. */
  published(): FakeRelease;
  /** Puts the tag at the fixture's target, which is what every draft staged after the tag-first change (#620) has behind it. */
  tagged(): void;
}

/** Builds a fixture and closes its server when the test ends. */
export async function newStageFixture(t: TestContext): Promise<StageFixture> {
  const assets = join(scratchDir("agentics-stage-"), "assets");
  writeSyntheticBundle(assets, STAGE_TAG);
  const notesDir = scratchDir("agentics-stage-notes-");
  const notes = join(notesDir, "baseline-notes.md");
  writeFileSync(notes, STAGE_NOTES);
  const output = join(scratchDir("agentics-stage-out-"), "outputs");
  const started = await startReleaseFake(STAGE_REPO);
  t.after(() => started.close());
  const { fake } = started;

  const fixture: StageFixture = {
    assets,
    notes,
    sha: STAGE_SHA,
    fake,
    url: started.url,
    output,
    argv: () => [
      "--repo",
      STAGE_REPO,
      "--sha",
      STAGE_SHA,
      "--tag",
      STAGE_TAG,
      "--assets",
      assets,
      "--notes",
      notes,
    ],
    env: () => ({
      GH_TOKEN: PRIMARY,
      GITHUB_API_URL: started.url,
      GITHUB_OUTPUT: output,
    }),
    run: (extra = [], options = { retryDelayMs: 0 }) =>
      fixture.runArgv([...fixture.argv(), ...extra], options),
    runArgv: async (argv, options = { retryDelayMs: 0 }) => {
      const captured = captureIo();
      const code = await run(argv, captured.io, fixture.env(), options);
      assert.equal(captured.stdout(), "", "stage wrote to stdout, want empty");
      return { code, stderr: captured.stderr() };
    },
    seen: () => started.seen(),
    draft: () =>
      fake.addRelease({
        id: 42,
        tagName: STAGE_TAG,
        name: STAGE_TAG,
        body: STAGE_NOTES,
        draft: true,
        targetCommitish: STAGE_SHA,
      }),
    published: () => {
      fake.tagObject = { type: "commit", sha: STAGE_SHA };
      return fake.addRelease({
        id: 42,
        tagName: STAGE_TAG,
        name: STAGE_TAG,
        body: PUBLISHED_BODY,
        targetCommitish: STAGE_SHA,
      });
    },
    tagged: () => {
      fake.tagObject = { type: "commit", sha: STAGE_SHA };
    },
  };
  return fixture;
}

/**
 * Reads GITHUB_OUTPUT and matches the multi-line file command writeOutputs writes: key, delimiter, value,
 * delimiter, each followed by the platform line ending.
 */
export function assertOutputs(
  path: string,
  want: Readonly<Record<string, string>>,
): void {
  const got = readFileSync(path, "utf8");
  for (const [key, value] of Object.entries(want)) {
    const entry = `${key}<<${OUTPUT_DELIMITER}${EOL}${value}${EOL}${OUTPUT_DELIMITER}${EOL}`;
    assert.ok(
      got.includes(entry),
      `GITHUB_OUTPUT = ${JSON.stringify(got)}, want substring ${JSON.stringify(entry)}`,
    );
  }
}

/** A failed run publishes nothing: the file is created by the first write, so absence is the whole assertion. */
export function assertNoOutputs(path: string): void {
  assert.equal(
    existsSync(path),
    false,
    "GITHUB_OUTPUT exists, want no outputs from a failed run",
  );
}

export function assertNoMethod(
  seen: readonly SeenRequest[],
  method: string,
): void {
  for (const request of seen) {
    assert.notEqual(
      request.method,
      method,
      `stage sent ${request.method} ${request.path}, want no ${method} request`,
    );
  }
}

export function assertNoRemoteWrites(seen: readonly SeenRequest[]): void {
  for (const request of seen) {
    assert.ok(
      !["POST", "PATCH", "DELETE"].includes(request.method),
      `stage sent remote write ${request.method} ${request.path}?${request.query} before rejecting the local candidate`,
    );
  }
}

/**
 * Proves the diagnostics of one phase carry no line of any release body. It matches line by line, because a
 * leak of one line is still a leak and a whole-body match would miss it.
 */
export function assertNoBodyLeak(stderr: string, ...bodies: string[]): void {
  for (const body of bodies) {
    for (const raw of body.split("\n")) {
      const line = raw.trim();
      if (line === "") continue;
      assert.ok(
        !stderr.includes(line),
        `stderr=${JSON.stringify(stderr)}, want no release body line ${JSON.stringify(line)}`,
      );
    }
  }
}

/** Returns argv with the value of flag replaced. */
export function replaceFlag(
  argv: readonly string[],
  flag: string,
  value: string,
): string[] {
  const index = argv.indexOf(flag);
  assert.ok(
    index >= 0 && index + 1 < argv.length,
    `argv does not carry ${flag}`,
  );
  const replaced = [...argv];
  replaced[index + 1] = value;
  return replaced;
}

/** Writes a file into a bundle directory, creating it if needed. */
export function writeAsset(
  dir: string,
  name: string,
  contents: string | Buffer,
): void {
  mkdirSync(dir, { recursive: true });
  writeFileSync(join(dir, name), contents);
}

export function readAsset(dir: string, name: string): Buffer {
  return readFileSync(join(dir, name));
}
