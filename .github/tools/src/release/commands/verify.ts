import { parseFlags } from "../args.ts";
import { verifyChecksums, verifyLocalAssets } from "../assets.ts";
import { EXIT_FAILURE, EXIT_USAGE, report, type Run } from "../io.ts";
import { STABLE_TAG } from "../version.ts";

const PREFIX = "release: verify";

// Checks the local roster and its checksum file. Proving asset provenance requires an
// independent rebuild from the selected commit and comparison with those trusted bytes.
export const run: Run = async (argv, io) => {
  const values = parseFlags(argv, ["assets", "tag"], io, PREFIX);
  const assets = values?.assets;
  const tag = values?.tag;
  if (!assets || !tag) {
    return report(io, PREFIX, "--assets and --tag are required", EXIT_USAGE);
  }
  if (!STABLE_TAG.test(tag)) {
    return report(
      io,
      PREFIX,
      `--tag must be vMAJOR.MINOR.PATCH, not ${tag}`,
      EXIT_USAGE,
    );
  }
  try {
    verifyLocalAssets(assets, tag);
    verifyChecksums(assets, tag);
  } catch (error) {
    return report(io, PREFIX, (error as Error).message, EXIT_FAILURE);
  }
  return 0;
};
