// The in-tree semantic-release plugin. It runs in the release job with the
// App token in its environment, so it stays small and only does two things:
// record the plan and refuse an unapproved major in verifyRelease, and in
// prepare write the four manifest version fields through the set-version
// command and save the generated notes as the baseline. It imports nothing
// but node:* modules. In particular it never imports the writer: set-version
// runs in a child process, so the writer never shares a process with the token.
import { execFileSync } from "node:child_process";
import { mkdirSync, writeFileSync } from "node:fs";
import { isAbsolute, join } from "node:path";
import { fileURLToPath } from "node:url";

type Env = Readonly<Record<string, string | undefined>>;

interface Context {
  readonly nextRelease: {
    readonly version: string;
    readonly gitTag: string;
    readonly type: string;
    readonly notes?: string;
  };
  readonly options: { readonly dryRun?: boolean };
  readonly env: Env;
  readonly logger: { log(message: string): void };
  readonly cwd: string;
}

const SEMVER = /^\d+\.\d+\.\d+$/u;

// The command line entry point, found beside this file so the writer that runs
// is the one that shipped with the plugin.
const CLI = fileURLToPath(new URL("./cli.ts", import.meta.url));

// The writer needs only enough environment to start a process. Everything else
// stays behind, above all the App token and the GITHUB_* and AGX_*
// variables. Keys compare case-insensitively because Windows spells PATH as
// Path in a copied environment object.
const CHILD_ENV_KEYS: ReadonlySet<string> = new Set([
  "PATH",
  "HOME",
  "USERPROFILE",
  "SYSTEMROOT",
  "TMPDIR",
  "TEMP",
  "TMP",
]);

function childEnv(env: Env): Record<string, string> {
  const kept: Record<string, string> = {};
  for (const [key, value] of Object.entries(env)) {
    if (value !== undefined && CHILD_ENV_KEYS.has(key.toUpperCase())) {
      kept[key] = value;
    }
  }
  return kept;
}

function outDir(env: Env): string {
  const dir = env.AGX_RELEASE_OUT;
  if (!dir || !isAbsolute(dir)) {
    throw new Error(
      "agentics-release: AGX_RELEASE_OUT must be an absolute path",
    );
  }
  mkdirSync(dir, { recursive: true });
  return dir;
}

export async function verifyRelease(
  _pluginConfig: unknown,
  context: Context,
): Promise<void> {
  const { nextRelease, options, env, logger } = context;
  if (!SEMVER.test(nextRelease.version)) {
    throw new Error(
      `agentics-release: unexpected version ${nextRelease.version}`,
    );
  }
  if (
    nextRelease.type === "major" &&
    !options.dryRun &&
    env.AGX_MAJOR_APPROVED !== "true"
  ) {
    throw new Error(
      "agentics-release: a major release needs approval, and AGX_MAJOR_APPROVED is not true",
    );
  }
  writeFileSync(
    join(outDir(env), "plan.json"),
    `${JSON.stringify({ version: nextRelease.version, tag: nextRelease.gitTag, type: nextRelease.type })}\n`,
  );
  logger.log(
    `agentics-release: planned ${nextRelease.gitTag} (${nextRelease.type})`,
  );
}

export async function prepare(
  _pluginConfig: unknown,
  context: Context,
): Promise<void> {
  const { nextRelease, cwd, env } = context;
  if (!SEMVER.test(nextRelease.version)) {
    throw new Error(
      `agentics-release: unexpected version ${nextRelease.version}`,
    );
  }
  const { version } = nextRelease;
  execFileSync(
    process.execPath,
    [CLI, "set-version", "--repo", cwd, "--version", version],
    { stdio: "inherit", env: childEnv(env) },
  );
  writeFileSync(join(outDir(env), "baseline.md"), nextRelease.notes ?? "");
}
