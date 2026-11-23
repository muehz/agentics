import { renameSync, writeFileSync } from "node:fs";
import {
  EXIT_OK,
  EXIT_TIMEOUT,
  EXIT_VALIDATION_FAILED,
  type Target,
} from "./constants.ts";
import type { ReviewContext } from "./context.ts";

export interface TargetState {
  target: Target;
  file: string;
  exit?: number;
  duration?: number;
}

function statusFor(exit: number | undefined): string {
  switch (exit) {
    case undefined:
      return "running";
    case EXIT_OK:
      return "complete";
    case EXIT_TIMEOUT:
      return "timeout";
    case EXIT_VALIDATION_FAILED:
      return "invalid-output";
    default:
      return "failed";
  }
}

// The layout is fixed so a restarted session, or a person, can read it with a line-oriented tool.
export function renderManifest(
  ctx: ReviewContext,
  startedUtc: string,
  states: TargetState[],
): string {
  const entries = states.map((state) => {
    const exit = state.exit === undefined ? "null" : String(state.exit);
    const duration =
      state.duration === undefined ? "null" : String(state.duration);
    return `    "${state.target}": { "status": "${statusFor(state.exit)}", "exit_code": ${exit}, "duration_seconds": ${duration}, "output": ${JSON.stringify(state.file)} }`;
  });
  return [
    "{",
    `  "range": ${JSON.stringify(ctx.range)},`,
    `  "platform": ${JSON.stringify(ctx.platform.source)},`,
    `  "started_utc": ${JSON.stringify(startedUtc)},`,
    `  "timeout_seconds": ${ctx.options.timeout},`,
    '  "targets": {',
    entries.join(",\n"),
    "  }",
    "}",
    "",
  ].join("\n");
}

// Written beside the destination and renamed over it, so a reader never sees half a manifest.
export function writeManifest(file: string, text: string): void {
  const temporary = `${file}.tmp`;
  writeFileSync(temporary, text);
  renameSync(temporary, file);
}
