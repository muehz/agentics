import { mkdirSync, readFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { parseArgs, UsageError, USAGE, type Parsed } from "./args.ts";
import { detectPlatform, readTunables, resolveTargets } from "./config.ts";
import {
  EXIT_FAILURE,
  EXIT_OK,
  EXIT_TIMEOUT,
  EXIT_USAGE,
  EXIT_VALIDATION_FAILED,
  type Target,
} from "./constants.ts";
import type { ReviewContext } from "./context.ts";
import { runTarget } from "./dispatch.ts";
import { renderDryRun } from "./dry-run.ts";
import { gitPreflight } from "./git.ts";
import type { Host } from "./host.ts";
import { renderManifest, writeManifest, type TargetState } from "./manifest.ts";
import { defaultOutputDir, isoStamp } from "./output.ts";
import { buildExcludes } from "./prompt.ts";

// Prints a progress line on a timer, because a reviewer that buffers its whole reply until exit is otherwise
// indistinguishable from a hang. An interval of 0 turns the ticker off. Returns the function that stops it.
function startHeartbeat(ctx: ReviewContext): () => void {
  const interval = ctx.tunables.heartbeatInterval;
  if (interval <= 0) return () => undefined;
  let elapsed = 0;
  const timer = setInterval(() => {
    elapsed += interval;
    ctx.host.err(
      `peer: reviews still running (${elapsed}s elapsed; timeout ${ctx.options.timeout}s per target)\n`,
    );
  }, interval * 1000);
  return () => clearInterval(timer);
}

// Creates an output directory, reporting a failure as a one-line error instead of letting it escape as a stack trace.
function ensureDir(host: Host, dir: string): boolean {
  try {
    mkdirSync(dir, { recursive: true });
    return true;
  } catch (error) {
    host.err(`ERROR: cannot create ${dir}: ${(error as Error).message}\n`);
    return false;
  }
}

async function runReviews(
  ctx: ReviewContext,
  targets: Target[],
): Promise<number> {
  const { host, options } = ctx;
  const startedUtc = isoStamp();
  const single = targets.length === 1;
  let manifestFile: string | undefined;
  let states: TargetState[];
  if (single && options.output !== "") {
    // An explicit single file has no directory to own, so no manifest.
    const file = resolve(host.cwd, options.output);
    if (!ensureDir(host, dirname(file))) return EXIT_FAILURE;
    states = targets.map((target) => ({ target, file }));
  } else {
    const dir =
      options.output !== ""
        ? resolve(host.cwd, options.output)
        : defaultOutputDir(ctx);
    if (!ensureDir(host, dir)) return EXIT_FAILURE;
    manifestFile = join(dir, "manifest.json");
    host.err(
      `peer: output dir ${dir} (manifest.json tracks per-target status; a restarted session can read it instead of relaunching)\n`,
    );
    states = targets.map((target) => ({
      target,
      file: join(dir, `${target}.md`),
    }));
  }
  // A manifest that cannot be written is one clear error, not a stack trace. Before any reviewer starts it stops the
  // run; after that it is reported once and the reviewers already running finish, so none is left without a watchdog.
  let manifestFailed = false;
  const writeNow = (): boolean => {
    if (manifestFile === undefined) return true;
    try {
      writeManifest(manifestFile, renderManifest(ctx, startedUtc, states));
      return true;
    } catch (error) {
      if (!manifestFailed) {
        host.err(
          `ERROR: cannot write ${manifestFile}: ${(error as Error).message}\n`,
        );
      }
      manifestFailed = true;
      return false;
    }
  };
  if (!writeNow()) return EXIT_FAILURE;

  const stopHeartbeat = startHeartbeat(ctx);
  const results = await Promise.all(
    states.map(async (state) => {
      const result = await runTarget(ctx, state.target, state.file);
      state.exit = result.rc;
      state.duration = result.seconds;
      writeNow();
      return result.rc;
    }),
  );
  stopHeartbeat();

  const failures = results.filter((rc) => rc !== EXIT_OK).length;
  if (single)
    return manifestFailed ? EXIT_FAILURE : (results[0] ?? EXIT_FAILURE);

  const lines = [
    "",
    "=== Peer review complete ===",
    `Platform: ${ctx.platform.source}`,
    `Targets: ${targets.join(" ")}`,
  ];
  for (const state of states) {
    switch (state.exit) {
      case EXIT_OK:
        // The line count is the number of newline characters, as `wc -l` reports it.
        lines.push(
          `  ${state.target} → ${state.file} (${readFileSync(state.file, "utf8").split("\n").length - 1} lines)`,
        );
        break;
      case EXIT_TIMEOUT:
        lines.push(
          `  ${state.target} → FAILED (timed out after ${options.timeout}s)`,
        );
        break;
      case EXIT_VALIDATION_FAILED:
        lines.push(
          `  ${state.target} → FAILED (output validation: empty, truncated, or missing REVIEW-COMPLETE)`,
        );
        break;
      default:
        lines.push(
          `  ${state.target} → FAILED (exit ${state.exit ?? "unknown"})`,
        );
    }
  }
  if (manifestFile !== undefined) lines.push(`Manifest: ${manifestFile}`);
  if (failures > 0) lines.push(`WARNING: ${failures} target(s) failed`);
  host.out(`${lines.join("\n")}\n`);
  return failures > 0 || manifestFailed ? EXIT_FAILURE : EXIT_OK;
}

export async function main(argv: string[], host: Host): Promise<number> {
  if (host.platform === "win32") {
    host.err(
      "ERROR: peer runs on Linux and macOS only (it needs POSIX process groups and ps); it does not support Windows\n",
    );
    return EXIT_FAILURE;
  }
  let parsed: Parsed;
  try {
    parsed = parseArgs(argv);
  } catch (error) {
    if (!(error instanceof UsageError)) throw error;
    host.err(`ERROR: ${error.message}\n`);
    if (error.showUsage) host.err(USAGE);
    return EXIT_USAGE;
  }
  if (parsed.kind === "help") {
    host.out(USAGE);
    return EXIT_OK;
  }
  const { options } = parsed;
  const platform = detectPlatform(host.env);
  const resolved = resolveTargets(options.target, platform);
  if ("error" in resolved) {
    host.err(`ERROR: ${resolved.error}\n`);
    return EXIT_FAILURE;
  }
  const ctx: ReviewContext = {
    host,
    options,
    tunables: readTunables(host),
    platform,
    range: `${options.base}..${options.head}`,
    exclude: buildExcludes(options.excludes),
  };
  if (options.dryRun) {
    host.out(renderDryRun(ctx, resolved.targets));
    return EXIT_OK;
  }
  const preflight = await gitPreflight(ctx);
  if (preflight !== EXIT_OK) return preflight;
  return await runReviews(ctx, resolved.targets);
}
