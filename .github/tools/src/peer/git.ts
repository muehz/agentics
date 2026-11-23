import { spawn, spawnSync } from "node:child_process";
import { EXIT_FAILURE, EXIT_NOTHING_TO_REVIEW, EXIT_OK } from "./constants.ts";
import type { ReviewContext } from "./context.ts";
import type { Host } from "./host.ts";

function gitStatus(host: Host, args: string[]): number {
  return (
    spawnSync("git", args, { cwd: host.cwd, env: host.env, stdio: "ignore" })
      .status ?? EXIT_FAILURE
  );
}

// Counts the bytes of a diff as they stream, so a very large diff never has to fit in memory. Git's own error text
// reaches the user, and a git that fails or cannot start gives undefined, never a size of 0.
function countDiffBytes(
  host: Host,
  args: string[],
): Promise<number | undefined> {
  return new Promise((resolve) => {
    const child = spawn("git", args, {
      cwd: host.cwd,
      env: host.env,
      stdio: ["ignore", "pipe", "inherit"],
    });
    let total = 0;
    child.stdout.on("data", (chunk: Buffer) => {
      total += chunk.length;
    });
    child.once("error", (error) => {
      host.err(`ERROR: cannot run git: ${error.message}\n`);
      resolve(undefined);
    });
    child.once("close", (code) => resolve(code === 0 ? total : undefined));
  });
}

// Returns EXIT_OK when the range is reviewable, else the exit code to stop with.
export async function gitPreflight(ctx: ReviewContext): Promise<number> {
  const { host, options, range, tunables, exclude } = ctx;
  if (gitStatus(host, ["rev-parse", "--git-dir"]) !== 0) {
    host.err(
      "ERROR: not inside a git repository — peer reviews a committed git range\n",
    );
    return EXIT_FAILURE;
  }
  for (const [flag, rev] of [
    ["--base", options.base],
    ["--head", options.head],
  ] as const) {
    if (
      gitStatus(host, [
        "rev-parse",
        "--verify",
        "--quiet",
        `${rev}^{commit}`,
      ]) !== 0
    ) {
      host.err(
        `ERROR: ${flag} '${rev}' does not resolve to a commit (shallow clone or typo?). Pass an explicit ${flag}.\n`,
      );
      return EXIT_FAILURE;
    }
  }
  if (
    gitStatus(host, ["diff", "--quiet", options.base, options.head, "--"]) === 0
  ) {
    host.err(`Nothing to review: ${range} contains no changes.\n`);
    return EXIT_NOTHING_TO_REVIEW;
  }
  if (
    gitStatus(host, ["diff", "--quiet"]) !== 0 ||
    gitStatus(host, ["diff", "--cached", "--quiet"]) !== 0
  ) {
    host.err(
      `WARNING: the working tree has uncommitted changes; peer reviews only the committed range ${range} — uncommitted changes are NOT reviewed.\n`,
    );
  }
  const bytes = await countDiffBytes(host, [
    "diff",
    options.base,
    options.head,
    "--",
    ".",
    ...exclude.args,
  ]);
  if (bytes === undefined) return EXIT_FAILURE;
  if (bytes === 0) {
    host.err(
      `Nothing to review after exclusions: every change in ${range} matches an excluded pathspec.\n`,
    );
    return EXIT_NOTHING_TO_REVIEW;
  }
  if (bytes > tunables.maxDiffBytes && !options.force) {
    host.err(
      `ERROR: post-exclusion diff is ${bytes} bytes (limit ${tunables.maxDiffBytes}). Diffs this large stall cross-model reviewers. Narrow the range, add --exclude patterns, or re-run with --force.\n`,
    );
    return EXIT_FAILURE;
  }
  return EXIT_OK;
}
