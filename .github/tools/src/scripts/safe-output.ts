import {
  spawnSync,
  type SpawnSyncOptionsWithStringEncoding,
} from "node:child_process";
import { readFileSync } from "node:fs";

type ItemType = "update_release_notes" | "apply_grooming";
type SafeOutput =
  { kind: "staged" | "unmatched" } | { kind: "matched"; value: string };

function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

export function readSafeOutput(
  env: NodeJS.ProcessEnv,
  type: ItemType,
): SafeOutput {
  if (env.GH_AW_SAFE_OUTPUTS_STAGED === "true") return { kind: "staged" };
  if (!env.GH_AW_AGENT_OUTPUT)
    throw new Error("GH_AW_AGENT_OUTPUT is required");
  let raw: string;
  try {
    raw = readFileSync(env.GH_AW_AGENT_OUTPUT, "utf8");
  } catch {
    throw new Error("could not read GH_AW_AGENT_OUTPUT");
  }
  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch {
    throw new Error("GH_AW_AGENT_OUTPUT is not valid JSON");
  }
  const items: unknown = isObject(parsed) ? parsed.items : undefined;
  if (!Array.isArray(items)) {
    if (type === "update_release_notes")
      throw new Error("GH_AW_AGENT_OUTPUT.items must be an array");
    if (!isObject(parsed))
      throw new Error("GH_AW_AGENT_OUTPUT is not a JSON object");
    return { kind: "unmatched" };
  }
  const matches = (items as unknown[]).filter(
    (item): item is Record<string, unknown> =>
      isObject(item) && item.type === type,
  );
  if (matches.length > 1) throw new Error(`multiple ${type} items`);
  const item = matches[0];
  if (!item) return { kind: "unmatched" };
  if (type === "apply_grooming") {
    if (typeof item.plan !== "string")
      throw new Error("apply_grooming plan must be a string");
    return { kind: "matched", value: item.plan };
  }
  if (typeof item.body !== "string" || !item.body.trim())
    throw new Error("update_release_notes body must be a nonempty string");
  if (Buffer.byteLength(item.body, "utf8") > 12000)
    throw new Error("update_release_notes body exceeds 12000 bytes");
  return { kind: "matched", value: item.body };
}

export function safeOutputRepo(env: NodeJS.ProcessEnv): string {
  const repo = env.GH_REPO;
  if (!repo) throw new Error("GH_REPO is required");
  if (!/^[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+$/.test(repo))
    throw new Error("GH_REPO is invalid");
  if (env.GITHUB_REPOSITORY !== undefined && repo !== env.GITHUB_REPOSITORY)
    throw new Error("GH_REPO does not match GITHUB_REPOSITORY");
  return repo;
}

type GhOperation = "release GET" | "PATCH" | "verification GET" | "grooming";
export type SafeGhRunner = (
  command: string,
  args: string[],
  options: SpawnSyncOptionsWithStringEncoding,
) => {
  status: number | null;
  stdout?: string | Buffer | null;
  stderr?: string | Buffer | null;
  error?: Error;
};

// Read the status from gh's diagnostic line. Later lines can contain advice
// or server-supplied text, and must not change the status used for a retry.
function httpStatus(stderr: string | Buffer | null | undefined) {
  const line =
    String(stderr ?? "")
      .trim()
      .split(/\r?\n/, 1)[0] ?? "";
  const match =
    /^(?:(?:gh: )?HTTP ([1-5]\d\d)|gh: [^\r\n]* \(HTTP ([1-5]\d\d)\))$/.exec(
      line,
    );
  return match?.[1] ?? match?.[2];
}

export function runSafeGh(
  operation: GhOperation,
  args: string[],
  input?: string,
  run: SafeGhRunner = spawnSync,
): unknown {
  const grooming = operation === "grooming";
  const label = grooming ? "gh api" : operation;
  let result: ReturnType<SafeGhRunner>;
  try {
    result = run("gh", args, {
      input,
      encoding: "utf8",
      maxBuffer: (grooming ? 64 : 2) * 1024 * 1024,
      windowsHide: true,
    });
  } catch {
    throw new Error(`${label} failed`);
  }
  if (result.error || result.status !== 0) {
    const status =
      grooming && !result.error && result.status !== null
        ? httpStatus(result.stderr)
        : undefined;
    throw new Error(`${label} failed${status ? `: HTTP ${status}` : ""}`);
  }
  let output = String(result.stdout ?? "");
  if (grooming) {
    output = output.trim();
    if (output === "") return null;
  }
  try {
    const parsed: unknown = JSON.parse(output);
    return parsed;
  } catch {
    throw new Error(`${label} returned invalid JSON`);
  }
}
