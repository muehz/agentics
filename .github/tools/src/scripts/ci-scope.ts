#!/usr/bin/env node

import fs from "node:fs";
import path from "node:path";
import { spawnSync, type SpawnSyncReturns } from "node:child_process";
import {
  encodeSchedule,
  encodeSelection,
  fullSelection,
  isFullReason,
  selectPaths,
  type FullReason,
  type Identity,
  type Selection,
} from "./ci-areas.ts";

interface RawRecord {
  oldMode: string;
  newMode: string;
  oldOid: string;
  newOid: string;
  status: string;
  paths: string[];
}
function property(value: unknown, key: string): unknown {
  return value !== null && typeof value === "object"
    ? Reflect.get(value, key)
    : undefined;
}
const ZERO = "0".repeat(40);

function sha(value: unknown): value is string {
  return typeof value === "string" && /^[0-9a-f]{40}$/.test(value);
}

function safePath(value: unknown): value is string {
  if (
    typeof value !== "string" ||
    value.length === 0 ||
    value.includes("\\") ||
    /["\u0000-\u001f\u007f]/.test(value)
  )
    return false;
  if (value.split("/").some((part) => part === "." || part === ".."))
    return false;
  return !value
    .split("/")
    .some(
      (part) =>
        /[<>:"|?*]/.test(part) ||
        /[. ]$/.test(part) ||
        /^(?:CON|PRN|AUX|NUL|COM[1-9¹²³]|LPT[0-9¹²³]|CONIN\$|CONOUT\$)(?:\..*)?$/i.test(
          part,
        ),
    );
}

function regularSide(mode: string): boolean {
  return mode === "100644" || mode === "000000";
}

function writeOutputs(text: string): void {
  const output = process.env.GITHUB_OUTPUT;
  if (output && text) fs.appendFileSync(output, text);
}

function stdoutResult(result: Selection): void {
  process.stdout.write(`${JSON.stringify(result)}\n`);
}

function finish(result: Selection, exitCode = 0): void {
  try {
    writeOutputs(encodeSelection(result));
    stdoutResult(result);
  } catch {
    // A failed output write is selector infrastructure failure. Emit a fixed
    // fallback result so the workflow cannot mistake a partial selector run
    // for an eligibility decision.
    stdoutResult(fullSelection("output-write"));
    process.exitCode = 1;
    return;
  }
  process.exitCode = exitCode;
}

function parseArgs(argv: string[]): { repo: string; event: string } {
  const values: Record<"repo" | "event", string | null | undefined> = {
    repo: null,
    event: null,
  };
  for (let index = 0; index < argv.length; index += 1) {
    const flag = argv[index];
    if (flag !== "--repo" && flag !== "--event") throw new Error("cli-input");
    const key = flag === "--repo" ? "repo" : "event";
    if (values[key] !== null || index + 1 >= argv.length)
      throw new Error("cli-input");
    values[key] = argv[++index];
  }
  if (!values.repo || !values.event) throw new Error("cli-input");
  return { repo: path.resolve(values.repo), event: path.resolve(values.event) };
}

function readEvent(file: string): unknown {
  try {
    return JSON.parse(fs.readFileSync(file, "utf8"));
  } catch {
    throw new Error("event-input");
  }
}

function git(
  repo: string,
  args: string[],
  encoding?: "utf8",
): SpawnSyncReturns<string>;
function git(
  repo: string,
  args: string[],
  encoding: null,
): SpawnSyncReturns<Buffer>;
function git(
  repo: string,
  args: string[],
  encoding: "utf8" | null = "utf8",
): SpawnSyncReturns<string | Buffer> {
  const result = spawnSync("git", ["-C", repo, ...args], {
    cwd: repo,
    encoding,
    shell: false,
    windowsHide: true,
    maxBuffer: 64 * 1024 * 1024,
  });
  return result;
}

function commitExists(repo: string, value: string): boolean {
  const result = git(repo, ["cat-file", "-e", `${value}^{commit}`]);
  if (result.error) throw new Error("git-error");
  if (result.status !== 0) {
    const diagnostic = String(result.stderr || "");
    if (
      !/not a valid object name|could not get object info|bad object/i.test(
        diagnostic,
      )
    )
      throw new Error("git-error");
    return false;
  }
  return result.status === 0;
}

function parents(repo: string, value: string): string[] {
  const result = git(repo, ["cat-file", "-p", value]);
  if (result.error || result.status !== 0) throw new Error("git-error");
  const headers = String(result.stdout).split(/\r?\n\r?\n/, 1)[0]!;
  const values = headers
    .split(/\r?\n/)
    .filter((line) => line.startsWith("parent "))
    .map((line) => line.slice("parent ".length));
  if (values.some((parent) => !sha(parent))) throw new Error("git-error");
  return values;
}

function rawDiff(
  repo: string,
  base: string,
  target: string,
): SpawnSyncReturns<Buffer> {
  return git(
    repo,
    [
      "diff",
      "--raw",
      "-z",
      "--no-abbrev",
      "-M",
      "-C",
      "--find-copies-harder",
      base,
      target,
      "--",
    ],
    null,
  );
}

function parseRaw(buffer: unknown): RawRecord[] {
  if (!Buffer.isBuffer(buffer)) throw new Error("malformed-diff");
  const text = buffer.toString("utf8");
  if (!Buffer.from(text, "utf8").equals(buffer))
    throw new Error("malformed-diff");
  if (buffer.length === 0) return [];
  if (!text.endsWith("\0")) throw new Error("malformed-diff");
  const tokens = text.slice(0, -1).split("\0");
  const records = [];
  for (let index = 0; index < tokens.length;) {
    const header = tokens[index++]!;
    const match =
      /^:([0-7]{6}) ([0-7]{6}) ([0-9a-f]{40}) ([0-9a-f]{40}) ((?:[ADMTU]|[RC][0-9]{1,3}))$/.exec(
        header,
      );
    if (!match) throw new Error("malformed-diff");
    const status = match[5]!;
    if (
      (status[0] === "R" || status[0] === "C") &&
      Number(status.slice(1)) > 100
    )
      throw new Error("malformed-diff");
    const count = /^[RC]/.test(status) ? 2 : 1;
    if (index + count > tokens.length) throw new Error("malformed-diff");
    const paths = tokens.slice(index, index + count);
    if (paths.some((value) => value.length === 0))
      throw new Error("malformed-diff");
    index += count;
    records.push({
      oldMode: match[1]!,
      newMode: match[2]!,
      oldOid: match[3]!,
      newOid: match[4]!,
      status,
      paths,
    });
  }
  return records;
}

function recordReason(record: RawRecord): FullReason | null {
  if (!/^(?:A|D|M|R[0-9]{1,3}|C[0-9]{1,3})$/.test(record.status))
    return "unsupported-status";
  if (!regularSide(record.oldMode) || !regularSide(record.newMode))
    return "non-regular-mode";
  if (
    record.paths.length === 0 ||
    record.paths.some((value) => !safePath(value))
  )
    return "unknown-path";
  if (record.status === "A") {
    if (record.oldMode !== "000000" || record.newMode !== "100644")
      return "non-regular-mode";
    if (record.oldOid !== ZERO || record.newOid === ZERO)
      return "malformed-diff";
  } else if (record.status === "D") {
    if (record.oldMode !== "100644" || record.newMode !== "000000")
      return "non-regular-mode";
    if (record.oldOid === ZERO || record.newOid !== ZERO)
      return "malformed-diff";
  } else {
    if (record.oldMode !== "100644" || record.newMode !== "100644")
      return "non-regular-mode";
    if (record.oldOid === ZERO || record.newOid === ZERO)
      return "malformed-diff";
  }
  return null;
}

function recordsReason(records: RawRecord[]): FullReason | null {
  if (records.length === 0) return "empty-diff";
  for (const record of records) {
    const reason = recordReason(record);
    if (reason) return reason;
  }
  return null;
}

function headCommit(repo: string): string {
  const result = git(repo, ["rev-parse", "HEAD"]);
  if (result.status !== 0) throw new Error("git-error");
  const value = String(result.stdout).trim();
  if (!sha(value)) throw new Error("git-error");
  return value;
}

function isAncestor(
  repo: string,
  ancestor: string,
  descendant: string,
): boolean {
  const result = git(repo, [
    "merge-base",
    "--is-ancestor",
    ancestor,
    descendant,
  ]);
  if (result.error || (result.status !== 0 && result.status !== 1))
    throw new Error("git-error");
  return result.status === 0;
}

// A push to main is scoped against the last commit whose validation succeeded, not against the commit before the
// push. A newer push cancels an older run, so the commits of a cancelled run must stay inside the next diff until
// some run validates them.
function classifyPush(repo: string, base: string): Selection {
  const head = headCommit(repo);
  if (!commitExists(repo, base))
    return fullSelection("incomplete-history", { base, head, merge: "" });
  if (!isAncestor(repo, base, head))
    return fullSelection("base-not-ancestor", { base, head, merge: "" });
  return classifyDiffs(repo, { base, head, merge: "" }, [head]);
}

function classify(repo: string, event: unknown): Selection {
  const request = property(event, "pull_request");
  const base = property(property(request, "base"), "sha");
  const head = property(property(request, "head"), "sha");
  if (!sha(base) || !sha(head)) throw new Error("event-input");

  const merge = headCommit(repo);
  const mergeParents = parents(repo, merge);
  if (
    !mergeParents ||
    mergeParents.length !== 2 ||
    mergeParents[0] !== base ||
    mergeParents[1] !== head
  )
    return fullSelection("merge-input-mismatch", { base, head, merge });
  if (
    !commitExists(repo, base) ||
    !commitExists(repo, head) ||
    !commitExists(repo, merge)
  )
    return fullSelection("incomplete-history", { base, head, merge });
  return classifyDiffs(repo, { base, head, merge }, [head, merge]);
}

// Classifies the diffs from base to each target. Every diff must qualify for a scoped mode.
function classifyDiffs(
  repo: string,
  identity: Identity,
  targets: string[],
): Selection {
  const diffs = targets.map((target) => rawDiff(repo, identity.base, target));
  if (diffs.some((result) => result.status !== 0)) {
    throw new Error("git-error");
  }

  const parsed = diffs.map((result) => parseRaw(result.stdout));
  const reason = parsed.map(recordsReason).find((reason) => reason !== null);
  if (reason === "malformed-diff") throw new Error(reason);
  if (reason) return fullSelection(reason, identity);

  // Both sides of a rename or copy are in record.paths, so a move between
  // areas selects the shards of both.
  return selectPaths(
    identity,
    parsed.flatMap((records) => records.flatMap((record) => record.paths)),
  );
}

function schedule(): void {
  const raw = process.env.AGX_CI_CLASSIFY_OUTPUTS;
  const result = process.env.AGX_CI_CLASSIFY_OUTCOME;
  if (!raw || result !== "success") return;
  let outputs: unknown;
  try {
    outputs = JSON.parse(raw);
  } catch {
    return;
  }
  try {
    writeOutputs(
      encodeSchedule({ result, outputs }, process.env.GITHUB_EVENT_NAME),
    );
  } catch {
    process.stderr.write("ci-scope: schedule output-write\n");
    process.exitCode = 1;
  }
}

function main(argv: string[]) {
  if (argv.length === 1 && argv[0] === "--schedule") {
    schedule();
    return;
  }
  let options;
  try {
    options = parseArgs(argv);
  } catch {
    finish(fullSelection("cli-input"), 1);
    return;
  }

  let event;
  try {
    event = readEvent(options.event);
  } catch {
    finish(fullSelection("event-input"), 1);
    return;
  }

  // The workflow passes the last validated commit of a push through this variable. Without one, a push runs in full,
  // as the schedule and manual runs always do.
  const eventName = process.env.GITHUB_EVENT_NAME;
  const lastGreen = process.env.AGX_CI_LAST_GREEN || "";
  const push = eventName === "push" && lastGreen !== "";
  if (eventName !== "pull_request" && !push) {
    finish(fullSelection("event-full"));
    return;
  }

  try {
    if (push && !sha(lastGreen)) throw new Error("event-input");
    finish(
      push
        ? classifyPush(options.repo, lastGreen)
        : classify(options.repo, event),
    );
  } catch (error) {
    const reason = error instanceof Error ? error.message : "";
    finish(fullSelection(isFullReason(reason) ? reason : "git-error"), 1);
  }
}

if (import.meta.main) main(process.argv.slice(2));
