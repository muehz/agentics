import { createWriteStream } from "node:fs";
import { finished } from "node:stream/promises";
import {
  ALLOWED_TOOLS,
  EXIT_FAILURE,
  EXIT_OK,
  EXIT_TIMEOUT,
  EXIT_VALIDATION_FAILED,
  type Target,
} from "./constants.ts";
import type { ReviewContext } from "./context.ts";
import { childEnvironment } from "./env.ts";
import { resolveLauncher } from "./launcher.ts";
import { runWithTimeout } from "./process.ts";
import { buildPrompt } from "./prompt.ts";
import { checkReviewOutput } from "./review.ts";

export interface TargetResult {
  rc: number;
  seconds: number;
}

function suggestFallback(ctx: ReviewContext, target: Target): void {
  const other = target === "claude" ? "codex" : "claude";
  ctx.host.err(
    [
      `WARNING: ${target} is not installed. Cannot dispatch peer review.`,
      "",
      "Options:",
      `  1. Install ${target} and re-run`,
      `  2. Run a same-platform review instead: re-run with --target ${other}`,
      "  3. Skip the peer review entirely",
      "",
      `Current platform: ${ctx.platform.source}`,
      "",
    ].join("\n"),
  );
}

// Claude's review is its stdout, teed to the output file. Codex writes the review to the output file itself and
// its stdout carries the transcript as progress only. Neither target is given a flag that skips a safety
// boundary: Claude's tool list and Codex's read-only sandbox are the non-mutation controls.
async function dispatchReview(
  ctx: ReviewContext,
  target: Target,
  outputFile: string,
): Promise<number> {
  const { host, options, tunables } = ctx;
  const launcher = resolveLauncher(target, host.env);
  if (launcher === undefined) {
    suggestFallback(ctx, target);
    return EXIT_FAILURE;
  }
  const prompt = `${buildPrompt(ctx).replace(/\n+$/, "")}\n`;
  const common = {
    env: childEnvironment(target, host.env),
    stdin: prompt,
    seconds: options.timeout,
    report: (line: string) => host.err(`${line}\n`),
  };
  if (target === "claude") {
    const sink = createWriteStream(outputFile);
    // A failed write is a failed dispatch, as it is for `tee` under pipefail: the review file cannot be trusted.
    let sinkFailed = false;
    sink.on("error", (error) => {
      sinkFailed = true;
      host.err(`ERROR: cannot write ${outputFile}: ${error.message}\n`);
    });
    const rc = await runWithTimeout({
      ...common,
      argv: [
        launcher,
        "-p",
        "--model",
        tunables.claudeModel,
        "--fallback-model",
        tunables.claudeFallbackModel,
        "--output-format",
        "text",
        "--allowedTools",
        ALLOWED_TOOLS,
        "--no-session-persistence",
      ],
      onStdout: (chunk) => {
        sink.write(chunk);
        host.out(chunk);
      },
    });
    sink.end();
    await finished(sink).catch(() => undefined);
    return sinkFailed ? EXIT_FAILURE : rc;
  }
  return await runWithTimeout({
    ...common,
    argv: [
      launcher,
      "exec",
      "-m",
      tunables.codexModel,
      "--sandbox",
      "read-only",
      "--output-last-message",
      outputFile,
      "-",
    ],
    onStdout: (chunk) => host.out(chunk),
  });
}

async function dispatchAndValidate(
  ctx: ReviewContext,
  target: Target,
  outputFile: string,
): Promise<number> {
  const { host, options } = ctx;
  const rc = await dispatchReview(ctx, target, outputFile);
  if (rc !== EXIT_OK) {
    if (rc === EXIT_TIMEOUT)
      host.err(`ERROR: ${target} review timed out after ${options.timeout}s\n`);
    return rc;
  }
  const check = checkReviewOutput(outputFile);
  if (check.ok) return EXIT_OK;
  host.err(
    check.reason === "empty"
      ? `ERROR: review output missing or empty: ${outputFile}\n`
      : `ERROR: review output requires exactly one REVIEW-COMPLETE terminator on the final line: ${outputFile}\n`,
  );
  return EXIT_VALIDATION_FAILED;
}

// A fast check before the real, possibly multi-minute dispatch. The reviewer CLIs need network and credential
// access, and a sandboxed shell makes them hang instead of error. It runs the same vendor binary the dispatch
// runs, so it uses the same scrubbed environment.
async function preflightTarget(
  ctx: ReviewContext,
  target: Target,
): Promise<number> {
  const { host, tunables } = ctx;
  const launcher = resolveLauncher(target, host.env);
  if (launcher === undefined) return EXIT_OK; // dispatchReview prints the missing-binary fallback
  const rc = await runWithTimeout({
    argv: [launcher, "--version"],
    env: childEnvironment(target, host.env),
    seconds: tunables.preflightTimeout,
    stderr: "ignore",
    // Only "cannot run" is worth showing from the preflight output: it names a launcher path that does not start,
    // which the generic failure line below cannot say.
    report: (line) => {
      if (line.startsWith("ERROR: cannot run")) host.err(`${line}\n`);
    },
  });
  if (rc === EXIT_OK) return EXIT_OK;
  host.err(
    [
      `ERROR: preflight for ${target} failed: '${launcher} --version' exited ${rc} (124 means it hung until the ${tunables.preflightTimeout}s preflight timeout).`,
      "The reviewer CLIs need network and credential (keychain) access. If this launcher is running inside",
      "Claude Code's default Bash sandbox, that sandbox blocks both — re-run the launcher unsandboxed.",
      "",
    ].join("\n"),
  );
  return EXIT_FAILURE;
}

export async function runTarget(
  ctx: ReviewContext,
  target: Target,
  outputFile: string,
): Promise<TargetResult> {
  const started = Date.now();
  let rc = await preflightTarget(ctx, target);
  if (rc === EXIT_OK) rc = await dispatchAndValidate(ctx, target, outputFile);
  return { rc, seconds: Math.floor((Date.now() - started) / 1000) };
}
