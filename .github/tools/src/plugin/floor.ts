/**
 * The Node floor for the maintainer CLI: the running Node must satisfy `engines.node` from the repository's
 * `package.json`, in every mode. A failure prints one line naming the range and never a stack trace.
 */
import { readFileSync, writeSync } from "node:fs";
import path from "node:path";
import { parseRange, satisfies } from "../scripts/node-range.ts";
import { CLI_COMMAND } from "./args.ts";
import { REPO_ROOT } from "./common.ts";

export type EnginesResult =
  { ok: true; range: string } | { ok: false; message: string };

/** Reads and validates `engines.node`; every failure is a one-line message, so the CLI can exit without a stack. */
export function readEnginesNode(repoRoot: string = REPO_ROOT): EnginesResult {
  const file = path.join(repoRoot, "package.json");
  const fail = (reason: string): EnginesResult => ({
    ok: false,
    message: `❌ Could not read the Node range from ${file}: ${reason}`,
  });
  let manifest: { engines?: { node?: unknown } } | null;
  try {
    manifest = JSON.parse(readFileSync(file, "utf8")) as typeof manifest;
  } catch (error) {
    return fail((error as Error).message);
  }
  const range = manifest?.engines?.node;
  if (typeof range !== "string") return fail("it has no engines.node range");
  try {
    parseRange(range);
  } catch (error) {
    return fail((error as Error).message);
  }
  return { ok: true, range };
}

export interface FloorOptions {
  action: string;
  nodeVersion?: string;
  enginesNode?: string;
  repoRoot?: string;
  write?: (stream: "stdout" | "stderr", text: string) => void;
}

const defaultWrite = (stream: "stdout" | "stderr", text: string): void => {
  // Synchronous writes keep the line ordered with anything a later stage prints on the same descriptor.
  writeSync(stream === "stdout" ? 1 : 2, text);
};

/** Returns true when the running Node is inside the range; otherwise writes one stderr line and returns false. */
export function checkNodeFloor(options: FloorOptions): boolean {
  const write = options.write ?? defaultWrite;
  const err = (text: string) => write("stderr", `${text}\n`);
  let range = options.enginesNode;
  if (range === undefined) {
    const engines = readEnginesNode(options.repoRoot);
    if (!engines.ok) {
      err(engines.message);
      return false;
    }
    range = engines.range;
  }
  const version = options.nodeVersion ?? process.versions.node;
  let inRange: boolean;
  try {
    inRange = satisfies(range, version);
  } catch (error) {
    err(
      `❌ Could not check Node ${version} against ${range}: ${(error as Error).message}`,
    );
    return false;
  }
  if (inRange) return true;
  // The validate line is the same text the validate stage prints, so a user sees one wording whichever check runs.
  err(
    options.action === "validate"
      ? `❌ --validate needs Node ${range} for the TypeScript conformance suite and the tools tests.`
      : `❌ ${CLI_COMMAND} needs Node ${range}; this is Node ${version}.`,
  );
  return false;
}
