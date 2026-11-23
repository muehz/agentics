import { accessSync, constants, statSync } from "node:fs";
import { delimiter, join } from "node:path";
import type { Target } from "./constants.ts";
import type { Host } from "./host.ts";

export function findOnPath(
  name: string,
  pathValue: string | undefined,
): string | undefined {
  for (const dir of (pathValue ?? "").split(delimiter)) {
    if (dir === "") continue;
    const candidate = join(dir, name);
    try {
      if (statSync(candidate).isFile()) {
        accessSync(candidate, constants.X_OK);
        return candidate;
      }
    } catch {
      // not here; keep looking
    }
  }
  return undefined;
}

const OVERRIDE: Record<Target, string> = {
  claude: "PEER_REVIEW_LAUNCHER",
  codex: "PEER_REVIEW_CODEX_LAUNCHER",
};

// An explicit PEER_REVIEW_* override wins, and setting it empty disables that target on purpose. Otherwise the
// launcher is whatever is on PATH.
export function resolveLauncher(
  target: Target,
  env: Host["env"],
): string | undefined {
  const override = env[OVERRIDE[target]];
  if (override !== undefined) return override === "" ? undefined : override;
  return findOnPath(target, env.PATH);
}
