import { spawn as nodeSpawn, type SpawnOptions } from "node:child_process";
import { statSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import type { Failure } from "../../check.ts";
import {
  definedEnv,
  describeChildFailure,
  tail,
  type Env,
} from "../tools/exec.ts";
import type { Family } from "./index.ts";

// Trust rule: the checkout being audited never supplies the code that audits it. The evals CLI this family runs is
// the engine in the muehz/evals clone beside the suite's own checkout root (`<root>/../evals`), or in the directory
// `AGX_EVALS_ROOT` names. The suite's root is computed from this module's location and from nothing else: never
// from `--repo`, a manifest, or any file read from the audited checkout, and the audited checkout cannot set an
// environment variable. The retired `AGX_CONFORMANCE_TOOLS` variable is not honoured as a trust source: setting it
// changes nothing. `AGX_CONFORMANCE_EVALS_CLI` is a test seam: an absolute script path run in place of the engine's
// main entry, so a test can feed the family a canned stream. CI never sets it and has no evals clone, so CI always
// takes the skip.

/** The environment variable that replaces the evals CLI script; a test seam, never set by CI. */
export const EVALS_CLI_ENV = "AGX_CONFORMANCE_EVALS_CLI";

/** The environment variable that names the muehz/evals clone in place of the sibling `../evals`. */
export const EVALS_ROOT_ENV = "AGX_EVALS_ROOT";

/** The one diagnostic line printed when there is no evals clone, so the eval rules did not run. */
export const SKIP_LINE = "skipped: no sibling evals clone";

/** The flags node runs the evals CLI with, before the script. */
export const NODE_FLAGS: readonly string[] = [
  "--disable-warning=ExperimentalWarning",
];

/** The engine's main entry, relative to the root of a muehz/evals clone. */
const MAIN_ENTRY = ["evals", "src", "cli", "main.ts"];

/** How many trailing stderr lines an audit error carries. */
const STDERR_TAIL_LINES = 30;

/**
 * The suite's own checkout root: a fixed climb from this module, which sits in `.github/tools/src/conformance/domains/skills/`.
 * It takes no input and searches for no marker, so neither the audited checkout nor the working directory can move it.
 */
export function trustedRoot(): string {
  return path.resolve(
    fileURLToPath(new URL("../../../../../../", import.meta.url)),
  );
}

/**
 * The muehz/evals clone the eval rules run from: `AGX_EVALS_ROOT` when it is set and not empty, made absolute, and
 * the sibling of the trusted root otherwise. Whether the directory exists is the caller's question.
 */
export function evalsRoot(trusted: string, env: Env): string {
  const override = env[EVALS_ROOT_ENV];
  return override === undefined || override === ""
    ? path.resolve(trusted, "..", "evals")
    : path.resolve(override);
}

/** The engine's main entry inside the evals clone at `root`. */
export function evalsMainEntry(root: string): string {
  return path.join(root, ...MAIN_ENTRY);
}

/** Whether `dir` exists, after following links, as a directory. */
export function isDirectory(dir: string): boolean {
  try {
    return statSync(dir).isDirectory();
  } catch {
    return false;
  }
}

/** The program and arguments of one `evals check` run; node and `NODE_FLAGS` come before the script. */
export type EvalsCommand = { script: string; args: string[] };

/**
 * Builds the `evals check` invocation for `repoRoot`, or returns null when there is no evals clone to run it from. The
 * repository path is made absolute, because the child sets no working directory. The script is
 * `AGX_CONFORMANCE_EVALS_CLI` when it is set and not empty, made absolute, and the main entry of the evals clone
 * (`evalsRoot`) otherwise. A missing clone directory means the eval rules are skipped. A clone that exists is expected
 * to be whole, so the script must exist after following links and be a file; otherwise this throws the audit error.
 */
export function evalsCheckCommand(
  repoRoot: string,
  trusted: string,
  env: Env,
): EvalsCommand | null {
  const repo = path.resolve(repoRoot);
  const override = env[EVALS_CLI_ENV];
  let script: string;
  if (override === undefined || override === "") {
    const root = evalsRoot(trusted, env);
    if (!isDirectory(root)) return null;
    script = evalsMainEntry(root);
  } else script = path.resolve(override);
  let isFile = false;
  try {
    isFile = statSync(script).isFile();
  } catch {
    // Missing or unreadable: reported below as a missing CLI.
  }
  if (!isFile)
    throw new Error(
      `skills: evals CLI ${script} is missing, so the eval rules cannot run`,
    );
  return { script, args: ["check", "--repo", repo, "--json"] };
}

// The result stream. Stdout holds zero or more JSON objects separated by optional space, tab, LF or CR, and each
// object is read with `JSON.parse`. Keys match in exact case, an unknown key fails the line, a duplicate key
// resolves to its last value, and `null` leaves a field absent. The failure count must be written as an integer and be
// a safe integer, because a count above 2^53 cannot be compared with the failure lines exactly.

type Line = {
  rule?: string;
  path?: string;
  message?: string;
  done?: boolean;
  failures?: number;
  error?: string;
};

type FieldName = keyof Line;

/** Each field with the JSON type it accepts besides `null`. */
const FIELDS: Readonly<Record<FieldName, "string" | "boolean" | "count">> = {
  rule: "string",
  path: "string",
  message: "string",
  done: "boolean",
  failures: "count",
  error: "string",
};

const INTEGER_LITERAL = /^-?(?:0|[1-9][0-9]*)$/;

const STREAM_SPACE = /[ \t\n\r]*/y;

/** A stream fault; `parseEvalsCheck` reports it as a line that is not valid output. */
class NotValid extends Error {}

function kindOf(value: unknown): string {
  if (Array.isArray(value)) return "an array";
  if (typeof value === "object") return "an object";
  return `a ${typeof value}`;
}

/**
 * Parses `text` as one JSON object and records, for each object in it, the source text of its `failures` member when
 * that is a number. Throws whatever `JSON.parse` throws.
 */
function parseObject(
  text: string,
  countSources: WeakMap<object, string>,
): Record<string, unknown> {
  return JSON.parse(
    text,
    function (
      this: unknown,
      key: string,
      value: unknown,
      context?: { source?: string },
    ) {
      if (
        key === "failures" &&
        typeof value === "number" &&
        typeof this === "object" &&
        this !== null
      )
        countSources.set(this, context?.source ?? "");
      return value;
    },
  ) as Record<string, unknown>;
}

/** Reads the fields of one parsed object, or throws `NotValid` for an unknown key or a value of the wrong type. */
function toLine(
  object: Record<string, unknown>,
  number: number,
  countSources: WeakMap<object, string>,
): Line {
  const line: Line = {};
  for (const key of Object.keys(object)) {
    if (!Object.hasOwn(FIELDS, key))
      throw new NotValid(
        `line ${number} has an unknown field ${JSON.stringify(key)}`,
      );
    const name = key as FieldName;
    const value = object[name];
    if (value === null) continue;
    const kind = FIELDS[name];
    if (name === "failures") {
      const source = countSources.get(object) ?? "";
      if (
        typeof value !== "number" ||
        !INTEGER_LITERAL.test(source) ||
        !Number.isSafeInteger(value)
      )
        throw new NotValid(
          typeof value === "number"
            ? `line ${number}: field failures holds ${source}, which is not a safe integer`
            : `line ${number}: field failures holds ${kindOf(value)}, not a safe integer`,
        );
      line.failures = value;
    } else if (kind === "boolean") {
      if (typeof value !== "boolean")
        throw new NotValid(
          `line ${number}: field ${name} holds ${kindOf(value)}, not a boolean`,
        );
      line.done = value;
    } else {
      if (typeof value !== "string")
        throw new NotValid(
          `line ${number}: field ${name} holds ${kindOf(value)}, not a string`,
        );
      // An escaped lone surrogate becomes U+FFFD, so every string the audit prints is well-formed UTF-16.
      Object.assign(line, {
        [name]: Buffer.from(value, "utf8").toString("utf8"),
      });
    }
  }
  return line;
}

/**
 * The offset just past the `}` that closes the object opening at `start`, or -1 when the input ends first. Braces
 * inside strings do not count, and a backslash in a string escapes the character after it. Whether the object is valid
 * JSON is left to `JSON.parse`.
 */
function objectEnd(text: string, start: number): number {
  let depth = 0;
  let inString = false;
  for (let at = start; at < text.length; at += 1) {
    const char = text[at];
    if (inString) {
      if (char === "\\") at += 1;
      else if (char === '"') inString = false;
    } else if (char === '"') inString = true;
    else if (char === "{") depth += 1;
    else if (char === "}") {
      depth -= 1;
      if (depth === 0) return at + 1;
    }
  }
  return -1;
}

/**
 * The objects of the stream in order, read lazily so a fault is reported only once every earlier object has been
 * judged. An object may span lines; a reason names the line it starts on.
 */
function* readLines(stdout: string): Generator<Line> {
  let at = 0;
  let number = 1;
  let counted = 0;
  for (;;) {
    STREAM_SPACE.lastIndex = at;
    STREAM_SPACE.test(stdout);
    at = STREAM_SPACE.lastIndex;
    if (at >= stdout.length) return;
    for (; counted < at; counted += 1)
      if (stdout[counted] === "\n") number += 1;
    if (stdout[at] !== "{")
      throw new NotValid(`line ${number} is not a JSON object`);
    const end = objectEnd(stdout, at);
    const countSources = new WeakMap<object, string>();
    let object: Record<string, unknown>;
    try {
      if (end === -1) throw new SyntaxError("unterminated object");
      object = parseObject(stdout.slice(at, end), countSources);
    } catch {
      throw new NotValid(`line ${number} is not valid JSON`);
    }
    yield toLine(object, number, countSources);
    at = end;
  }
}

/**
 * Judges one `evals check --json` run by its closing line. A done line whose count equals the failure lines read is a
 * completed audit. Anything else, a line that is not valid output included, throws the audit error; the exit code is
 * never the verdict. The input is the child's whole stdout, decoded once.
 */
export function parseEvalsCheck(stdout: string): Failure[] {
  const reader = readLines(stdout);
  const failures: Failure[] = [];
  let closed = false;
  for (;;) {
    let next: IteratorResult<Line>;
    try {
      next = reader.next();
    } catch (cause) {
      if (cause instanceof NotValid)
        throw new Error(
          `skills: evals check printed a line that is not valid output: ${cause.message}`,
        );
      throw cause;
    }
    if (next.done === true) break;
    const line = next.value;
    if (closed)
      throw new Error(
        "skills: evals check printed a line after its closing line",
      );
    if (line.error !== undefined) {
      if (
        line.done !== undefined ||
        line.failures !== undefined ||
        line.rule !== undefined ||
        line.path !== undefined ||
        line.message !== undefined
      )
        throw new Error("skills: evals check error line carries other fields");
      throw new Error(`skills: evals check could not audit: ${line.error}`);
    }
    if (line.done !== undefined) {
      if (
        line.rule !== undefined ||
        line.path !== undefined ||
        line.message !== undefined
      )
        throw new Error(
          "skills: evals check closing line carries other fields",
        );
      if (!line.done)
        throw new Error("skills: evals check closing line says done is false");
      if (line.failures === undefined)
        throw new Error(
          "skills: evals check closing line has no failure count",
        );
      if (line.failures !== failures.length)
        throw new Error(
          `skills: evals check closing line does not match the ${failures.length} failure lines read`,
        );
      closed = true;
    } else if (line.rule !== undefined && line.rule !== "") {
      if (line.failures !== undefined)
        throw new Error(
          "skills: evals check failure line carries other fields",
        );
      if (line.path === undefined || line.message === undefined)
        throw new Error(
          "skills: evals check failure line is missing its path or message",
        );
      failures.push({
        rule: line.rule,
        path: line.path,
        message: line.message,
      });
    } else if (
      line.rule !== undefined ||
      line.path !== undefined ||
      line.message !== undefined ||
      line.failures !== undefined
    ) {
      throw new Error(
        "skills: evals check printed a failure line with no rule",
      );
    } else {
      throw new Error("skills: evals check printed an empty object");
    }
  }
  if (!closed) throw new Error("skills: evals check printed no closing line");
  return failures;
}

/** The last `n` lines of `text` after trailing newlines are trimmed, joined by newlines. */
export function tailLines(text: string, n: number): string {
  return tail(text, n);
}

/** The readable side of a child's output pipe, as far as the runner uses it. */
export interface OutputStream {
  on(event: "data", listener: (chunk: Buffer) => void): unknown;
}

/** The child process, as far as the runner uses it: Node's `ChildProcess` satisfies it, and tests fake it. */
export interface ChildLike {
  stdout: OutputStream | null;
  stderr: OutputStream | null;
  on(event: "error", listener: (error: Error) => void): unknown;
  on(
    event: "close",
    listener: (code: number | null, signal: NodeJS.Signals | null) => void,
  ): unknown;
}

/** The process-spawn seam, shaped like Node's `spawn`. */
export type Spawn = (
  file: string,
  args: string[],
  options: SpawnOptions,
) => ChildLike;

const realSpawn: Spawn = (file, args, options) =>
  nodeSpawn(file, args, options);

type Outcome = { stdout: string; stderr: string; runError: string };

/** The name the run slot of an audit error gives the child process. */
const COMMAND = "evals check";

function collect(
  spawn: Spawn,
  file: string,
  args: string[],
  options: SpawnOptions,
): Promise<Outcome> {
  return new Promise((resolve) => {
    const out: Buffer[] = [];
    const err: Buffer[] = [];
    // Decoded once from the joined bytes, so a character split across chunks survives and a leading BOM is kept.
    // A child that failed to start counts as having printed nothing.
    const finish = (runError: string, started = true) => {
      resolve({
        stdout: started ? Buffer.concat(out).toString("utf8") : "",
        stderr: started ? Buffer.concat(err).toString("utf8") : "",
        runError,
      });
    };
    let child: ChildLike;
    try {
      child = spawn(file, args, options);
    } catch (cause) {
      const startError = cause instanceof Error ? cause.message : String(cause);
      finish(describeChildFailure(COMMAND, { startError }), false);
      return;
    }
    child.stdout?.on("data", (chunk) => out.push(chunk));
    child.stderr?.on("data", (chunk) => err.push(chunk));
    // The runner never kills the child or opens IPC, so "error" means the spawn itself failed.
    child.on("error", (error) =>
      finish(
        describeChildFailure(COMMAND, { startError: error.message }),
        false,
      ),
    );
    child.on("close", (code, signal) => {
      if (signal !== null) finish(describeChildFailure(COMMAND, { signal }));
      else finish(describeChildFailure(COMMAND, { code: code ?? -1 }));
    });
  });
}

/**
 * Runs `evals check --json` on `repoRoot` with the evals clone's CLI and returns its failures. With no evals clone it
 * writes `SKIP_LINE` to `diagnostics` and returns no failures, so a checkout without the clone still passes. The child
 * is `node` with `NODE_FLAGS`, gets `env` with undefined values dropped, sets no working directory, ignores stdin, and
 * has no timeout. Stdout and stderr are captured apart and only stdout is parsed. A parse error becomes the audit error
 * `<parse error> (run: <run state>)`, followed by a newline and the last 30 stderr lines when stderr holds any text.
 * The run state is the shared child-process text, a clean exit included: `evals check exited with code N`,
 * `evals check was killed by SIGNAL`, or `evals check could not start: REASON`.
 */
export async function runEvalsCheck(
  repoRoot: string,
  env: Env = process.env,
  spawn: Spawn = realSpawn,
  diagnostics: (text: string) => void = (text) => process.stderr.write(text),
): Promise<Failure[]> {
  const command = evalsCheckCommand(repoRoot, trustedRoot(), env);
  if (command === null) {
    diagnostics(`${SKIP_LINE}\n`);
    return [];
  }
  const { stdout, stderr, runError } = await collect(
    spawn,
    process.execPath,
    [...NODE_FLAGS, command.script, ...command.args],
    {
      env: definedEnv(env),
      stdio: ["ignore", "pipe", "pipe"],
      windowsHide: true,
    },
  );
  try {
    return parseEvalsCheck(stdout);
  } catch (cause) {
    const message = cause instanceof Error ? cause.message : String(cause);
    const tailText = tailLines(stderr, STDERR_TAIL_LINES);
    throw new Error(
      `${message} (run: ${runError})${tailText === "" ? "" : `\n${tailText}`}`,
    );
  }
}

/** The `eval-check` family: the eval record, digest and bank rules, reported under the rule ids `evals check` prints. */
export const FAMILY: Family = {
  name: "eval-check",
  rules: [],
  run: (root, context) => context.evalsCheck(root),
};
