import { parseFlags } from "../args.ts";
import { derivableAssetNames, HOOK_BINARY_PATHS } from "../assets.ts";
import { EXIT_OK, EXIT_USAGE, report, type Run } from "../io.ts";
import { STABLE_TAG } from "../version.ts";

const PREFIX = "release: asset-roster";

/** Emits the read-only stage roster as one JSON line for the requested tag. */
export const run: Run = async (argv, io) => {
  const values = parseFlags(argv, ["tag"], io, PREFIX);
  if (values === undefined) return EXIT_USAGE;
  const tag = values.tag;
  if (!tag) return report(io, PREFIX, "--tag is required", EXIT_USAGE);
  if (!STABLE_TAG.test(tag)) {
    return report(
      io,
      PREFIX,
      `--tag must be vMAJOR.MINOR.PATCH, not ${tag}`,
      EXIT_USAGE,
    );
  }

  const roster = {
    hookBinaryPaths: HOOK_BINARY_PATHS,
    derivableAssetNames: derivableAssetNames(tag),
  };
  io.stdout.write(`${JSON.stringify(roster)}\n`);
  return EXIT_OK;
};
