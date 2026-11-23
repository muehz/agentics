import { parseFlags } from "../args.ts";
import { git, writeOutputs } from "../github.ts";
import { readFile } from "../files.ts";
import { EXIT_FAILURE, EXIT_USAGE, report, type Run } from "../io.ts";
import { FULL_SHA, STABLE_TAG } from "../version.ts";

const PREFIX = "release: resume-check";

// The subject @semantic-release/git gives the release commit. Group 1 is the
// version the commit carries.
const RELEASE_COMMIT_SUBJECT =
  /^chore\(release\): v([0-9]+\.[0-9]+\.[0-9]+) \[skip ci\]$/;

// Decides what the release workflow does with a head.
//
//   analyze   the head is an ordinary commit; semantic-release decides.
//   resume    the head is a release commit whose tag has no published release.
//   released  the head is a release commit and its release is published.
//
// It writes mode and resume_tag. An older stable tag merged into the head with
// no published release only warns: the run proceeds and the maintainer settles
// that tag by hand.
export const run: Run = async (argv, io, env) => {
  const values = parseFlags(argv, ["checkout", "sha", "published"], io, PREFIX);
  const checkout = values?.checkout;
  const sha = values?.sha;
  const published = values?.published;
  if (!checkout || !sha || !published) {
    return report(
      io,
      PREFIX,
      "--checkout, --sha, and --published are required",
      EXIT_USAGE,
    );
  }
  if (!FULL_SHA.test(sha)) {
    return report(
      io,
      PREFIX,
      `--sha must be 40 lowercase hexadecimal characters, not ${sha}`,
      EXIT_USAGE,
    );
  }

  try {
    const publishedTags = new Set(
      readFile(published)
        .toString("utf8")
        .split(/\s+/)
        .filter((word) => word !== ""),
    );

    const subject = await git(checkout, ["log", "-1", "--format=%s", sha]);
    let mode = "analyze";
    let tag = "";
    const match = RELEASE_COMMIT_SUBJECT.exec(subject.trim());
    if (match !== null) {
      tag = `v${match[1]}`;
      mode = publishedTags.has(tag) ? "released" : "resume";
    }

    const merged = await git(checkout, [
      "tag",
      "--merged",
      sha,
      "--list",
      "v*",
    ]);
    const stranded = merged
      .split(/\s+/)
      .filter(
        (name) =>
          name !== tag && STABLE_TAG.test(name) && !publishedTags.has(name),
      )
      .sort();
    for (const name of stranded) {
      io.stderr.write(
        `::warning::${name} has no published release; settle it by hand\n`,
      );
    }

    writeOutputs({ mode, resume_tag: tag }, env);
  } catch (error) {
    return report(io, PREFIX, (error as Error).message, EXIT_FAILURE);
  }
  return 0;
};
