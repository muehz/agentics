import { ALLOWED_TOOLS, type Target } from "./constants.ts";
import type { ReviewContext } from "./context.ts";
import { resolveLauncher } from "./launcher.ts";
import { defaultOutputDir } from "./output.ts";
import { buildPrompt } from "./prompt.ts";

// One renderer for both the single-target and the multi-target dry run.
function dryRunTarget(ctx: ReviewContext, target: Target): string {
  const { tunables } = ctx;
  const launcher = resolveLauncher(target, ctx.host.env) ?? "NOT FOUND";
  const lines = [`Launcher: ${launcher}`];
  if (target === "claude") {
    lines.push(
      "Transport: stdin → claude -p",
      `Model: ${tunables.claudeModel} (fallback: ${tunables.claudeFallbackModel})`,
      "Flags: --output-format text --allowedTools --no-session-persistence",
      `Allowed tools: ${ALLOWED_TOOLS}`,
    );
  } else {
    lines.push(
      "Transport: stdin → codex exec (the range travels in the prompt body)",
      `Model: ${tunables.codexModel}`,
      `Flags: -m ${tunables.codexModel} --sandbox read-only --output-last-message <output-file> -`,
      "Sandbox: read-only",
    );
  }
  return `${lines.join("\n")}\n\n${buildPrompt(ctx)}`;
}

export function renderDryRun(ctx: ReviewContext, targets: Target[]): string {
  const { options, platform } = ctx;
  const first = targets[0];
  if (targets.length > 1 || first === undefined) {
    const head = [
      "Peer review request (multi-target)",
      `Detected platform: ${platform.source}`,
      `Targets: ${targets.join(" ")}`,
      "Execution: parallel",
      `Timeout: ${options.timeout}s per target`,
      `Output: ${options.output === "" ? defaultOutputDir(ctx) : options.output}/<target>.md`,
    ].join("\n");
    const sections = targets.map(
      (target) => `\n--- ${target} ---\n${dryRunTarget(ctx, target)}`,
    );
    return `${head}\n${sections.join("")}`;
  }
  const head = [
    `Peer review request (${first})`,
    `Detected platform: ${platform.source}`,
    `Timeout: ${options.timeout}s`,
    `Output: ${options.output === "" ? `${defaultOutputDir(ctx)}/${first}.md` : options.output}`,
  ].join("\n");
  return `${head}\n${dryRunTarget(ctx, first)}`;
}
