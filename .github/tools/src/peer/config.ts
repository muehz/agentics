import type { TargetChoice } from "./args.ts";
import type { Target } from "./constants.ts";
import type { Host } from "./host.ts";
import {
  DEFAULT_CLAUDE_FALLBACK_MODEL,
  DEFAULT_CLAUDE_MODEL,
  DEFAULT_CODEX_MODEL,
} from "./models.ts";

export interface Tunables {
  maxDiffBytes: number;
  preflightTimeout: number;
  heartbeatInterval: number;
  claudeModel: string;
  claudeFallbackModel: string;
  codexModel: string;
}

export interface Platform {
  source: string;
  autoTargets: Target[];
}

// An unset or empty variable takes its default, as ${NAME:-default} does.
function textTunable(env: Host["env"], name: string, fallback: string): string {
  const value = env[name];
  return value === undefined || value === "" ? fallback : value;
}

// The most seconds a setInterval can wait: Node clamps a delay above 2^31-1 milliseconds to 1 ms.
const MAX_INTERVAL_SECONDS = Math.floor((2 ** 31 - 1) / 1000);

// A non-integer override, one below the minimum, or one that is not a safe integer or exceeds the maximum falls back to
// the default with a warning. A long digit string would otherwise convert to Infinity and make the preflight timeout
// unbounded. For the heartbeat, 0 disables the ticker, so its minimum is 0. A timeout needs a minimum of 1: the
// watchdog treats 0 seconds as an immediate timeout, not as no limit.
function integerTunable(
  host: Host,
  name: string,
  fallback: number,
  minimum = 0,
  maximum = Number.MAX_SAFE_INTEGER,
): number {
  const value = host.env[name];
  if (value === undefined || value === "") return fallback;
  const parsed = Number(value);
  if (
    !/^[0-9]+$/.test(value) ||
    !Number.isSafeInteger(parsed) ||
    parsed < minimum ||
    parsed > maximum
  ) {
    const kind = minimum > 0 ? "a positive integer" : "a non-negative integer";
    host.err(`WARNING: ${name}='${value}' is not ${kind}; using ${fallback}\n`);
    return fallback;
  }
  return parsed;
}

export function readTunables(host: Host): Tunables {
  return {
    maxDiffBytes: integerTunable(host, "PEER_MAX_DIFF_BYTES", 256000),
    preflightTimeout: integerTunable(host, "PEER_PREFLIGHT_TIMEOUT", 15, 1),
    heartbeatInterval: integerTunable(
      host,
      "PEER_HEARTBEAT_INTERVAL",
      30,
      0,
      MAX_INTERVAL_SECONDS,
    ),
    claudeModel: textTunable(
      host.env,
      "PEER_CLAUDE_MODEL",
      DEFAULT_CLAUDE_MODEL,
    ),
    claudeFallbackModel: textTunable(
      host.env,
      "PEER_CLAUDE_FALLBACK_MODEL",
      DEFAULT_CLAUDE_FALLBACK_MODEL,
    ),
    codexModel: textTunable(host.env, "PEER_CODEX_MODEL", DEFAULT_CODEX_MODEL),
  };
}

// Platform detection, in priority order. Copilot's plugin root is deliberately absent: peer does not run on
// Copilot CLI, so it is not a detected platform.
export function detectPlatform(env: Host["env"]): Platform {
  if (env.CODEX_CI === "1") return { source: "codex", autoTargets: ["claude"] };
  if (env.ANTIGRAVITY_AGENT === "1")
    return { source: "gemini", autoTargets: ["claude", "codex"] };
  if (env.CLAUDECODE === "1")
    return { source: "claude-code", autoTargets: ["codex"] };
  return { source: "unknown", autoTargets: [] };
}

export function resolveTargets(
  choice: TargetChoice,
  platform: Platform,
): { targets: Target[] } | { error: string } {
  if (choice === "all") return { targets: ["claude", "codex"] };
  if (choice === "auto") {
    if (platform.autoTargets.length === 0) {
      return {
        error:
          "could not detect current platform; pass --target claude or --target codex explicitly",
      };
    }
    return { targets: platform.autoTargets };
  }
  return { targets: [choice] };
}
