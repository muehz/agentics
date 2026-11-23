import { spawnSync } from "node:child_process";
import { realpathSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";

const config = {
  "user.name": "Agentics Test",
  "user.email": "test@example.com",
  "commit.gpgsign": "false",
  "tag.gpgsign": "false",
  "core.hooksPath": "/dev/null",
  "core.attributesFile": "/dev/null",
  "core.excludesFile": "/dev/null",
  "core.autocrlf": "false",
  "core.eol": "lf",
};

/** The environment for Git and commands that launch Git in a fixture. Apply deliberate scenario overrides afterwards. */
export function fixtureGitEnv(
  inherited: NodeJS.ProcessEnv = process.env,
): Record<string, string> {
  const env = Object.fromEntries(
    Object.entries(inherited).filter(
      (entry): entry is [string, string] =>
        entry[1] !== undefined && !/^GIT_/i.test(entry[0]),
    ),
  );
  // Git for Windows also recognizes /dev/null. Suppress templates and attributes that live outside gitconfig.
  env.GIT_CONFIG_GLOBAL = "/dev/null";
  env.GIT_CONFIG_NOSYSTEM = "1";
  env.GIT_ATTR_NOSYSTEM = "1";
  env.GIT_TEMPLATE_DIR = "";
  const settings = Object.entries(config);
  env.GIT_CONFIG_COUNT = String(settings.length);
  for (const [i, [key, value]] of settings.entries()) {
    env[`GIT_CONFIG_KEY_${i}`] = key;
    env[`GIT_CONFIG_VALUE_${i}`] = value;
  }
  return env;
}

/** Runs Git in the fixture root and returns stdout. A missing checkout must not resolve to a parent repository. */
export function git(root: string, ...args: string[]): string {
  const result = spawnSync("git", ["-C", root, ...args], {
    env: {
      ...fixtureGitEnv(),
      GIT_CEILING_DIRECTORIES: path.dirname(realpathSync(root)),
    },
    encoding: "utf8",
  });
  if (result.status !== 0)
    throw new Error(
      `git ${args.join(" ")}: ${result.stderr || result.error?.message}`,
    );
  return result.stdout;
}

/**
 * Whether Git can discover a repository above or at the OS temp directory. The probe uses the audit fixture's
 * environment so it predicts the same parent search when a test creates an empty, invalid `.git` directory.
 */
export function tempIsInsideRepository(
  env: Record<string, string | undefined> = process.env,
): boolean {
  return (
    spawnSync("git", ["-C", tmpdir(), "rev-parse", "--git-dir"], {
      env: fixtureGitEnv(env),
      encoding: "utf8",
    }).status === 0
  );
}

/** Turns a fixture into a git checkout and force-adds `tracked` (paths relative to `root`). */
export function initCheckout(root: string, tracked: string[] = []): void {
  git(root, "init", "-q");
  if (tracked.length > 0) git(root, "add", "-f", "--", ...tracked);
}
