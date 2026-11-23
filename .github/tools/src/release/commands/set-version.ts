import { parseFlags } from "../args.ts";
import { EXIT_FAILURE, EXIT_USAGE, report, type Run } from "../io.ts";
import { formatVersion, setVersion } from "../version.ts";

const PREFIX = "release: set-version";

// Writes one version into the five manifests. The release job calls it from the
// semantic-release plugin's prepare step, so the commit semantic-release makes
// holds exactly the five version fields it changed.
export const run: Run = async (argv, io) => {
  const values = parseFlags(argv, ["repo", "version"], io, PREFIX);
  if (values === undefined || !values.repo || !values.version) {
    return report(io, PREFIX, "--repo and --version are required", EXIT_USAGE);
  }
  try {
    const { from, to } = setVersion(values.repo, values.version);
    io.stdout.write(
      `Agentics version ${formatVersion(from)} -> ${formatVersion(to)} (5 manifests written)\n`,
    );
  } catch (error) {
    return report(io, PREFIX, (error as Error).message, EXIT_FAILURE);
  }
  return 0;
};
