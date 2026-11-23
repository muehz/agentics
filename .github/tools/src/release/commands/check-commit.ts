import { parseFlags } from "../args.ts";
import { git } from "../github.ts";
import { EXIT_FAILURE, EXIT_USAGE, report, type Run } from "../io.ts";
import {
  formatVersion,
  FULL_SHA,
  tagVersion,
  verifyReleaseCommit,
} from "../version.ts";

const PREFIX = "release: check-commit";
const REQUIRED = "--checkout, --sha, --tag, and --author are required";

// Verifies the release commit before anything is built from it. The release job
// pushes with a token that bypasses the main ruleset, so the commit's shape is
// checked here, in a job that holds no token.
export const run: Run = async (argv, io) => {
  const values = parseFlags(
    argv,
    ["checkout", "sha", "parent", "tag", "author"],
    io,
    PREFIX,
  );
  const checkout = values?.checkout;
  const sha = values?.sha;
  const tag = values?.tag;
  const author = values?.author;
  const parent = values?.parent ?? "";
  if (!checkout || !sha || !tag || !author) {
    return report(io, PREFIX, REQUIRED, EXIT_USAGE);
  }
  if (!FULL_SHA.test(sha)) {
    return report(
      io,
      PREFIX,
      `--sha must be 40 lowercase hexadecimal characters, not ${sha}`,
      EXIT_USAGE,
    );
  }
  if (parent !== "" && !FULL_SHA.test(parent)) {
    return report(
      io,
      PREFIX,
      `--parent must be 40 lowercase hexadecimal characters, not ${parent}`,
      EXIT_USAGE,
    );
  }
  try {
    await checkReleaseCommit(checkout, sha, parent, tag, author);
  } catch (error) {
    return report(io, PREFIX, (error as Error).message, EXIT_FAILURE);
  }
  return 0;
};

// Requires sha to name a commit, not an object that peels to one, with exactly
// one parent (parent itself when that is non-empty), author and committer names
// equal to author, and a diff from that parent that only raises the four
// version fields. It then requires tag to name v<version-at-commit>, any
// existing tag of that name to point at sha, and every other stable v* tag to
// be lower than tag. The first failure wins, so the order below is the
// contract.
//
// The name comparison is a sanity check on the name the release job
// configures, not authentication: anyone who can make a commit can set any
// author or committer name.
async function checkReleaseCommit(
  checkout: string,
  sha: string,
  parent: string,
  tag: string,
  author: string,
): Promise<void> {
  const line = await git(checkout, ["rev-list", "--parents", "-n", "1", sha]);
  const parents = line.split(/\s+/).filter((word) => word !== "");
  if (parents.length > 0 && parents[0] !== sha) {
    throw new Error(`${sha} is not a commit; it peels to commit ${parents[0]}`);
  }
  const onlyParent = parents[1];
  if (parents.length !== 2 || onlyParent === undefined) {
    throw new Error(
      `release commit ${sha} must have exactly one parent, has ${parents.length - 1}`,
    );
  }
  if (parent !== "" && onlyParent !== parent) {
    throw new Error(`release commit parent is ${onlyParent}, want ${parent}`);
  }

  const names = await git(checkout, ["log", "-1", "--format=%an%n%cn", sha]);
  const trimmed = names.endsWith("\n") ? names.slice(0, -1) : names;
  const newline = trimmed.indexOf("\n");
  const authorName = newline < 0 ? trimmed : trimmed.slice(0, newline);
  const committerName = newline < 0 ? "" : trimmed.slice(newline + 1);
  if (authorName !== author) {
    throw new Error(
      `release commit author is ${JSON.stringify(authorName)}, want ${JSON.stringify(author)}`,
    );
  }
  if (committerName !== author) {
    throw new Error(
      `release commit committer is ${JSON.stringify(committerName)}, want ${JSON.stringify(author)}`,
    );
  }

  const version = await verifyReleaseCommit(checkout, onlyParent, sha);
  const want = `v${formatVersion(version)}`;
  if (tag !== want) {
    throw new Error(`tag ${tag} does not match version-at-commit ${want}`);
  }
  const current = tagVersion(tag);
  if (current === undefined) {
    throw new Error(`tag ${tag} is not an orderable stable version`);
  }

  const tags = await git(checkout, ["tag", "--list", "v*"]);
  for (const name of tags.split(/\s+/).filter((word) => word !== "")) {
    if (name === tag) {
      // The full ref name keeps a same-named ref elsewhere, such as
      // refs/<name>, from winning the lookup. An annotated tag names a tag
      // object, so peel it to the commit.
      const pointsAt = await git(checkout, [
        "rev-parse",
        "--verify",
        "--end-of-options",
        `refs/tags/${name}^{commit}`,
      ]);
      const target = pointsAt.trim();
      if (target !== sha) {
        throw new Error(
          `tag ${name} already points at ${target}, not the release commit ${sha}`,
        );
      }
      continue;
    }
    // A prerelease or any other non-stable name has no place in the release
    // order, so it can neither block nor permit a release.
    const existing = tagVersion(name);
    if (existing === undefined) continue;
    if (!lowerThan(existing, current)) {
      throw new Error(`tag ${tag} is not greater than existing tag ${name}`);
    }
  }
}

// True when a sorts strictly before b, comparing major, minor, then patch.
function lowerThan(
  a: readonly [bigint, bigint, bigint],
  b: readonly [bigint, bigint, bigint],
): boolean {
  for (let i = 0; i < 3; i += 1) {
    const x = a[i] ?? 0n;
    const y = b[i] ?? 0n;
    if (x !== y) return x < y;
  }
  return false;
}
