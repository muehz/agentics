import {
  closeSync,
  lstatSync,
  openSync,
  readFileSync,
  readSync,
} from "node:fs";
import path from "node:path";
import type { Failure } from "../../check.ts";
import { repoFileErrorOf } from "../../repo-file-error.ts";
import type { Tracked } from "./walk.ts";

/** Where the allowlist of shell and PowerShell files lives, relative to the repository root. */
export const SHELL_ALLOWLIST =
  ".github/tools/src/conformance/shell-allowlist.json";
/** The rule id of a tracked shell or PowerShell file the allowlist does not list. */
export const SHELL_RULE = "shell-allowlist";
/** The rule id of an allowlist entry that no tracked file matches. */
export const STALE_RULE = "shell-allowlist-stale";

/** Extensions that mark a file as shell or PowerShell, compared lower-cased. */
const SHELL_EXTENSIONS: ReadonlySet<string> = new Set([
  ".sh",
  ".bash",
  ".ps1",
  ".psm1",
  ".cmd",
  ".bat",
]);

/** Interpreter names a `#!` line may name for an extensionless file to count. */
const SHELL_INTERPRETERS: ReadonlySet<string> = new Set([
  "sh",
  "bash",
  "zsh",
  "pwsh",
  "powershell",
]);

/** How many leading bytes of an extensionless file are read; a shebang line longer than this is cut off. */
const PREFIX_BYTES = 256;

const BYTE_ORDER_MARK = "\uFEFF";

/** The first bytes of a file, at most `PREFIX_BYTES` of them. */
function readPrefix(file: string): Buffer {
  const fd = openSync(file, "r");
  try {
    const buffer = Buffer.alloc(PREFIX_BYTES);
    let filled = 0;
    while (filled < PREFIX_BYTES) {
      const n = readSync(fd, buffer, filled, PREFIX_BYTES - filled, null);
      if (n === 0) break;
      filled += n;
    }
    return buffer.subarray(0, filled);
  } finally {
    closeSync(fd);
  }
}

/** Whether a file's leading bytes open with a `#!` line that names a shell or PowerShell interpreter. */
function hasShellShebang(prefix: Buffer): boolean {
  if (prefix.includes(0)) return false;
  let text = prefix.toString("utf8");
  // A byte order mark before the marker still counts: the fail-closed choice.
  if (text.startsWith(BYTE_ORDER_MARK)) text = text.slice(1);
  if (!text.startsWith("#!")) return false;
  const newline = text.indexOf("\n");
  let line = newline === -1 ? text : text.slice(0, newline);
  if (line.endsWith("\r")) line = line.slice(0, -1);
  const tokens = splitWords(line.slice(2));
  let interpreter = path.posix.basename(tokens[0] ?? "");
  if (interpreter === "env")
    interpreter = path.posix.basename(envCommand(tokens.slice(1)) ?? "");
  return SHELL_INTERPRETERS.has(interpreter);
}

/** The `env` options that take an operand, as one letter. The operand is the rest of the token or the next token. */
const ENV_SHORT_OPERAND = "uCPa";
/** The long `env` options that take an operand, which is the text after `=` or the next token. */
const ENV_LONG_OPERAND: ReadonlySet<string> = new Set([
  "unset",
  "chdir",
  "argv0",
]);
const ENV_ASSIGNMENT = /^[A-Za-z_][A-Za-z0-9_]*=/;

/**
 * The command word of an `env` invocation, given the tokens after `env`: options come first (with the operands of
 * the options that take one, and a `-S` string already split into tokens), then `NAME=value` assignments, then the
 * command. A bare `--` ends the options. The result is undefined when no command follows.
 */
function envCommand(args: string[]): string | undefined {
  const rest = [...args];
  while (rest.length > 0) {
    const token = rest[0] ?? "";
    if (token === "--") {
      rest.shift();
      break;
    }
    if (token === "-") {
      rest.shift();
      continue;
    }
    if (!token.startsWith("-")) break;
    rest.shift();
    if (token.startsWith("--")) {
      const [name = "", ...value] = token.slice(2).split("=");
      if (name === "split-string" && value.length > 0)
        rest.unshift(...splitWords(value.join("=")));
      else if (ENV_LONG_OPERAND.has(name) && value.length === 0) rest.shift();
      continue;
    }
    for (let i = 1; i < token.length; i++) {
      const letter = token[i] ?? "";
      const attached = token.slice(i + 1);
      if (letter === "S") {
        rest.unshift(...splitWords(attached));
        break;
      }
      if (ENV_SHORT_OPERAND.includes(letter)) {
        if (attached === "") rest.shift();
        break;
      }
    }
  }
  while (ENV_ASSIGNMENT.test(rest[0] ?? "")) rest.shift();
  return rest[0];
}

function splitWords(text: string): string[] {
  return text.split(/[ \t]+/).filter((word) => word !== "");
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

/** The allowlisted paths. A missing file is an empty allowlist; a malformed one throws. */
function readAllowlist(root: string): Set<string> {
  let text: string;
  try {
    text = readFileSync(path.join(root, ...SHELL_ALLOWLIST.split("/")), "utf8");
  } catch (error) {
    const fileError = repoFileErrorOf(error, SHELL_ALLOWLIST);
    if (fileError.kind === "missing") return new Set();
    throw fileError;
  }
  const fail = (message: string, cause?: unknown): Error =>
    new Error(`${SHELL_ALLOWLIST}: ${message}`, { cause });
  let entries: unknown;
  try {
    entries = JSON.parse(text);
  } catch (error) {
    throw fail("invalid JSON", error);
  }
  if (!Array.isArray(entries)) throw fail("expected a JSON array");
  const listed = new Set<string>();
  for (const [index, entry] of entries.entries()) {
    const prefix = `entry ${index}`;
    if (!isRecord(entry)) throw fail(`${prefix} is not an object`);
    const { path: rel, reason } = entry;
    if (typeof rel !== "string" || rel === "")
      throw fail(`${prefix} needs a non-empty string path`);
    if (typeof reason !== "string" || reason.trim() === "")
      throw fail(`${prefix} needs a non-empty reason`);
    if (listed.has(rel))
      throw fail(`${prefix} duplicates an earlier entry for ${rel}`);
    listed.add(rel);
  }
  return listed;
}

/**
 * Whether a tracked path is shell or PowerShell. An extension decides it by the name alone, so a symlink or a file
 * missing from the tree still counts. An extensionless regular file counts when its first line is a shell `#!`; a
 * symlink, a directory, a missing path and a binary file do not, and only a bounded prefix is ever read.
 */
function isShellFile(
  root: string,
  rel: string,
  read: (file: string) => Buffer,
): boolean {
  const name = path.posix.basename(rel);
  const extension = path.extname(name);
  if (extension !== "") return SHELL_EXTENSIONS.has(extension.toLowerCase());
  const absolute = path.join(root, ...rel.split("/"));
  let isRegular: boolean;
  try {
    isRegular = lstatSync(absolute).isFile();
  } catch (error) {
    const fileError = repoFileErrorOf(error, rel);
    if (fileError.kind === "missing") return false;
    throw fileError;
  }
  if (!isRegular) return false;
  let prefix: Buffer;
  try {
    prefix = read(absolute);
  } catch (error) {
    throw repoFileErrorOf(error, rel);
  }
  return hasShellShebang(prefix);
}

function byPathThenMessage(a: Failure, b: Failure): number {
  if (a.path !== b.path) return a.path < b.path ? -1 : 1;
  if (a.message !== b.message) return a.message < b.message ? -1 : 1;
  return 0;
}

/**
 * Fails each tracked shell or PowerShell file that the allowlist does not list, and each allowlist entry that no
 * tracked file matches. Shell is an extension (`.sh`, `.bash`, `.ps1`, `.psm1`, `.cmd`, `.bat`) or an extensionless file
 * whose first line is a `#!` naming `sh`, `bash`, `zsh`, `pwsh` or `powershell`. The tracked list decides what is
 * visited, so a tracked file under a skipped directory counts and an untracked file never does. Paths are compared
 * exactly as git spells them. A missing allowlist is an empty one, so removing the file cannot silence the rule, and a
 * malformed allowlist throws. `read` lets a test make a read fail, because an unreadable file cannot be built portably.
 */
export function shellAllowlist(
  root: string,
  tracked: Tracked,
  options: { read?: (file: string) => Buffer } = {},
): Failure[] {
  const read = options.read ?? readPrefix;
  const listed = readAllowlist(root);
  const failures: Failure[] = [];
  for (const rel of tracked.files)
    if (!listed.has(rel) && isShellFile(root, rel, read))
      failures.push({
        rule: SHELL_RULE,
        path: rel,
        message: `${rel} is a shell or PowerShell file and is not listed in ${SHELL_ALLOWLIST}. Port it to TypeScript, or add an entry with the reason it must stay`,
      });
  for (const rel of listed)
    if (!tracked.files.has(rel))
      failures.push({
        rule: STALE_RULE,
        path: SHELL_ALLOWLIST,
        message: `allowlist entry for ${rel} names no tracked file. Remove the entry`,
      });
  return failures.sort(byPathThenMessage);
}
