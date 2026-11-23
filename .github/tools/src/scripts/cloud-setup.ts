// Runs the cloud setup steps that follow .github/scripts/cloud-setup.sh: the
// npm floor, optional tools, worktree setup, the smoke test and the summary.
// The bootstrap exports AGX_TOOLS_DIR, AGX_BIN_DIR and AGX_ORIGINAL_PATH.
// Imports only Node built-ins, because it runs before npm ci.
import path from "node:path";
import { runCloudSetup, systemDeps } from "./cloud-setup-steps.ts";

const repoRoot = path.resolve(import.meta.dirname, "../../../..");
process.exitCode = runCloudSetup(
  process.argv.slice(2),
  systemDeps(process.env, repoRoot),
);
