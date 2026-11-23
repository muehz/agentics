import { spawn } from "node:child_process";
import path from "node:path";
import {
  type AuditResult,
  errorMessage,
  type Failure,
  type RunOptions,
} from "../../check.ts";
import { describeChildFailure } from "../tools/exec.ts";

const AREA_NAMES = ["ascii", "schema", "module", "freshness"] as const;

function absolute(value: string): boolean {
  // Node accepts a Windows drive-relative root (\foo); the verifier needs a volume too.
  return (
    path.isAbsolute(value) &&
    (process.platform !== "win32" || path.parse(value).root.length > 1)
  );
}

const NUMBER = String.raw`-?(?:0|[1-9]\d*)(?:\.\d+)?(?:[eE][+-]?\d+)?`;
const TOKEN = new RegExp(
  String.raw`"(?:[^"\\]|\\[\s\S])*"|[{}\[\]]|${NUMBER}|[^\s]`,
  "g",
);
// Matches from the end of a string token when that string is an object key,
// and captures the value's spelling when the value is a number.
const MEMBER = new RegExp(String.raw`[\t\n\r ]*:[\t\n\r ]*(${NUMBER})?`, "y");

/** An object key as the document spells it, with its value's spelling when that value is a number. */
interface KeyToken {
  text: string;
  number?: string;
}

/**
 * Reads the verifier's result under this document's strictness rule, which is
 * stricter than JSON: no object may hold two keys that are equal, or that
 * become equal in upper case or in lower case, and nothing but JSON whitespace
 * may follow the document. A verdict must not depend on which of two
 * look-alike keys a reader picks.
 *
 * `JSON.parse` keeps only the last of two equal keys and reads `1.0` as `1`,
 * so a scan of the document's tokens records every object's keys and the
 * spelling of the top-level `version`. The scan is exact only for well-formed
 * JSON, so its records are used only after the parse has accepted the same text.
 */
function document(source: string): {
  value: unknown;
  versionLiteral?: string;
} {
  let end = source.length;
  const objects: KeyToken[][] = [];
  // One entry per open container: an object's keys, or undefined for an array.
  const open: (KeyToken[] | undefined)[] = [];
  for (const token of source.matchAll(TOKEN)) {
    const text = token[0];
    if (text === "{") {
      const keys: KeyToken[] = [];
      objects.push(keys);
      open.push(keys);
    } else if (text === "[") open.push(undefined);
    else if (text === "}" || text === "]") {
      open.pop();
      if (open.length === 0) {
        end = token.index + 1;
        break;
      }
    } else if (text.startsWith('"')) {
      MEMBER.lastIndex = token.index + text.length;
      const member = MEMBER.exec(source);
      if (member !== null)
        open.at(-1)?.push({
          text,
          ...(member[1] === undefined ? {} : { number: member[1] }),
        });
    }
  }
  let value: unknown;
  try {
    value = JSON.parse(source.slice(0, end));
  } catch (error) {
    throw new Error(`document: invalid JSON (${errorMessage(error)})`);
  }
  let versionLiteral: string | undefined;
  for (const [index, keys] of objects.entries()) {
    const seen: string[] = [];
    for (const { text, number } of keys) {
      // Keys are compared as the strings they decode to, so an escape cannot hide a repeat.
      const key: unknown = JSON.parse(text);
      if (typeof key !== "string") throw new Error("document: invalid key");
      if (
        seen.some(
          (previous) =>
            previous.toUpperCase() === key.toUpperCase() ||
            previous.toLowerCase() === key.toLowerCase(),
        )
      )
        throw new Error(
          `document: duplicate object key ${JSON.stringify(key)}`,
        );
      seen.push(key);
      // The first object opened is the document itself when the document is an object.
      if (index === 0 && key === "version") versionLiteral = number;
    }
  }
  if (!/^[\t\n\r ]*$/.test(source.slice(end)))
    throw new Error("trailing output");
  return {
    value,
    ...(versionLiteral === undefined ? {} : { versionLiteral }),
  };
}

type Members = Record<string, unknown>;

function object(value: unknown): Members {
  if (value === null || value === undefined) return {};
  if (typeof value !== "object" || Array.isArray(value))
    throw new Error("document: expected object");
  return value as Members;
}

/** Reads a key in its exact case, and only when the document itself holds it. */
function member(holder: Members, key: string): unknown {
  return Object.hasOwn(holder, key) ? holder[key] : undefined;
}

function optionalType(value: unknown, type: "string" | "array"): void {
  if (value === null || value === undefined) return;
  if (type === "array" ? !Array.isArray(value) : typeof value !== "string")
    throw new Error(`document: expected ${type}`);
}

function parse(source: string): { failures: Failure[]; areaErrors: string[] } {
  const { value, versionLiteral } = document(source);
  const doc = object(value);
  const version = member(doc, "version");
  if (
    version !== null &&
    version !== undefined &&
    (typeof version !== "number" ||
      versionLiteral === undefined ||
      !/^-?\d+$/.test(versionLiteral))
  )
    throw new Error("document: version must be an integer");
  const areas = member(doc, "areas");
  optionalType(areas, "array");
  // A wrongly typed field is reported before a missing required one.
  for (const value of (areas ?? []) as unknown[]) {
    const area = object(value);
    optionalType(member(area, "name"), "string");
    const failures = member(area, "failures");
    optionalType(failures, "array");
    for (const value of (failures ?? []) as unknown[]) {
      const failure = object(value);
      for (const key of ["rule", "path", "message"])
        optionalType(member(failure, key), "string");
    }
  }
  if (version !== 1) throw new Error("version must be 1");
  if (!Array.isArray(areas) || areas.length !== AREA_NAMES.length)
    throw new Error("expected exactly four areas");
  const failures: Failure[] = [];
  const areaErrors: string[] = [];
  for (const [index, name] of AREA_NAMES.entries()) {
    const area = object(areas[index]);
    if (member(area, "name") !== name)
      throw new Error(`area ${index} must be ${name}`);
    const findings = member(area, "failures");
    if (!Array.isArray(findings))
      throw new Error(`area ${name} missing failures array`);
    const error = member(area, "error");
    if (error === undefined) throw new Error(`area ${name} missing error`);
    if (error !== null) {
      if (typeof error !== "string")
        throw new Error(`area ${name} error must be string or null`);
      areaErrors.push(`${name}: ${error}`);
    }
    for (const value of findings) {
      const failure = object(value);
      const rule = member(failure, "rule");
      const file = member(failure, "path");
      const message = member(failure, "message");
      if (
        typeof rule !== "string" ||
        typeof file !== "string" ||
        typeof message !== "string"
      )
        throw new Error(`area ${name} failure missing string field`);
      failures.push({ rule, path: file, message });
    }
  }
  return { failures, areaErrors };
}

/** The name the audit errors give the verifier child process. */
const COMMAND = "hook audit";

/**
 * The run error for a verifier that closed with `code` and `signal`, or undefined for exit 0. A close with neither a
 * code nor a signal is a failure, so an unexplained exit never passes the audit.
 */
export function closeError(
  code: number | null,
  signal: NodeJS.Signals | null,
): string | undefined {
  if (signal !== null) return describeChildFailure(COMMAND, { signal });
  if (code === 0) return undefined;
  return describeChildFailure(COMMAND, { code: code ?? -1 });
}

interface ProcessResult {
  stdout: string;
  stderr: string;
  startError?: string;
  runError?: string;
}

async function audit(
  verifier: string,
  repoRoot: string,
  skipTests: boolean,
  env: RunOptions["env"],
): Promise<ProcessResult> {
  const args = ["audit", "--repo", repoRoot];
  if (skipTests) args.push("--skip-tests");
  try {
    return await new Promise((resolve) => {
      const stdout: Buffer[] = [];
      const stderr: Buffer[] = [];
      const child = spawn(verifier, args, {
        env,
        stdio: ["ignore", "pipe", "pipe"],
        windowsHide: true,
      });
      child.stdout.on("data", (data: Buffer) => stdout.push(data));
      child.stderr.on("data", (data: Buffer) => stderr.push(data));
      let startError: string | undefined;
      child.on("error", (error) => {
        startError = error.message;
      });
      child.on("close", (code, signal) => {
        const runError = closeError(code, signal);
        resolve({
          stdout: Buffer.concat(stdout).toString("utf8"),
          stderr: Buffer.concat(stderr).toString("utf8"),
          ...(startError === undefined ? {} : { startError }),
          ...(runError === undefined ? {} : { runError }),
        });
      });
    });
  } catch (error) {
    return { stdout: "", stderr: "", startError: errorMessage(error) };
  }
}

/** Runs only the environment's explicit trusted verifier and retains findings alongside audit errors. */
export async function run(
  repoRoot: string,
  options: RunOptions,
): Promise<AuditResult> {
  const verifier = options.env.AGX_HOOK_VERIFIER ?? "";
  if (!absolute(verifier) || !absolute(repoRoot))
    return {
      failures: [],
      error: "verifier and repository paths must be absolute",
    };
  const result = await audit(verifier, repoRoot, options.fast, options.env);
  if (result.startError !== undefined)
    return {
      failures: [],
      error: describeChildFailure(COMMAND, { startError: result.startError }),
    };
  let diagnosticError: string | undefined;
  if (result.runError === undefined && result.stderr !== "") {
    try {
      options.diagnostics(result.stderr);
    } catch (error) {
      diagnosticError = `write hook audit diagnostics: ${errorMessage(error)}`;
    }
  }
  const combine = (error?: string) =>
    [error, diagnosticError].filter((part) => part !== undefined).join("\n");
  let parsed: ReturnType<typeof parse>;
  try {
    parsed = parse(result.stdout);
  } catch (error) {
    const invalid = `invalid hook audit result: ${errorMessage(error)}`;
    return {
      failures: [],
      error:
        result.runError === undefined
          ? combine(invalid)
          : `${invalid}; ${result.runError}; ${result.stderr.trim()}`,
    };
  }
  const { failures, areaErrors } = parsed;
  if (result.runError !== undefined)
    return {
      failures,
      error: `${result.runError}; ${areaErrors.join("; ")}; ${result.stderr.trim()}`,
    };
  if (areaErrors.length > 0)
    return {
      failures,
      error: combine(
        `hook audit returned errors with exit 0: ${areaErrors.join("; ")}`,
      ),
    };
  return diagnosticError === undefined
    ? { failures }
    : { failures, error: diagnosticError };
}
