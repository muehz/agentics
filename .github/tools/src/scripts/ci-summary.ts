#!/usr/bin/env node

import fs from "node:fs";

import {
  JOB_ORDER,
  SUMMARY_JOBS,
  assessJobs,
  expectedJobResult,
  isSelectionEvent,
  type Job,
  type JobId,
  type Selection,
} from "./ci-areas.ts";

type Needs = Record<string, unknown> & Record<JobId, Job>;
function record(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

const LEVELS = new Set(["none", "patch", "minor", "major"]);
const RESULTS = new Set(["success", "failure", "cancelled", "skipped"]);

function error(code: string): Error {
  return new Error(code);
}

function parseArgs(argv: string[]): {
  needsEnv: string;
  eventEnv: string;
  summaryEnv: string;
} {
  const values: Record<string, string | undefined> = {};
  for (let index = 0; index < argv.length; index += 1) {
    const flag = argv[index];
    if (
      flag !== "--needs-env" &&
      flag !== "--event-name-env" &&
      flag !== "--summary-env"
    )
      throw error("cli-input");
    if (values[flag] !== undefined || index + 1 >= argv.length)
      throw error("cli-input");
    const value = argv[++index];
    if (typeof value !== "string" || value.length === 0)
      throw error("cli-input");
    values[flag] = value;
  }
  if (
    values["--needs-env"] === undefined ||
    values["--event-name-env"] === undefined ||
    values["--summary-env"] === undefined
  )
    throw error("cli-input");
  return {
    needsEnv: values["--needs-env"],
    eventEnv: values["--event-name-env"],
    summaryEnv: values["--summary-env"],
  };
}

function readNeeds(name: string): Record<string, unknown> {
  const raw = process.env[name];
  if (typeof raw !== "string" || raw.length === 0) throw error("needs-input");
  let value: unknown;
  try {
    value = JSON.parse(raw);
  } catch {
    throw error("invalid-json");
  }
  if (!record(value)) throw error("invalid-needs");
  return value;
}

function checkJobs(needs: Record<string, unknown>): asserts needs is Needs {
  for (const id of Object.keys(needs)) {
    if (!Object.hasOwn(SUMMARY_JOBS, id)) throw error(`unknown-job:${id}`);
  }
  for (const id of JOB_ORDER) {
    if (!Object.hasOwn(needs, id)) throw error(`missing-job:${id}`);
    const value = needs[id];
    if (!record(value)) throw error(`invalid-job:${id}`);
    if (typeof value.result !== "string" || !RESULTS.has(value.result))
      throw error(`invalid-job:${id}`);
  }
}

function markdownCell(value: unknown): string {
  return String(value)
    .replaceAll("\\", "\\\\")
    .replaceAll("|", "\\|")
    .replaceAll("`", "\\`");
}

// The release level the PR metadata job previews. It is shown only when it is
// one of the four levels, so a forged output cannot inject markdown.
function releaseLevel(needs: Needs): string | undefined {
  const outputs = needs["pr-metadata"].outputs;
  const level = record(outputs) ? outputs.level : undefined;
  return typeof level === "string" && LEVELS.has(level) ? level : undefined;
}

function render(
  needs: Needs,
  event: string,
  selection: Selection | null,
  errors: string[],
): string {
  const rows = JOB_ORDER.map((id) => {
    const expected = expectedJobResult(id, event, selection);
    const actual = needs[id].result;
    let icon = "❌";
    if (actual === expected) icon = actual === "success" ? "✅" : "➖";
    return `| ${icon} | ${SUMMARY_JOBS[id][0]} | ${expected} | ${actual} |`;
  });
  const level = event === "pull_request" ? releaseLevel(needs) : undefined;
  return [
    errors.length ? "## ❌ Validation failed" : "## ✅ Validation passed",
    "",
    ...(errors.length
      ? [
          "Problems: " +
            errors.map((code) => `\`${markdownCell(code)}\``).join(", "),
          "",
        ]
      : []),
    ...(level ? [`Release level: **${level}**`, ""] : []),
    "| | Check | Expected | Actual |",
    "| --- | --- | --- | --- |",
    ...rows,
    "",
    "<details><summary>Change selection</summary>",
    "",
    `Event: \`${markdownCell(event)}\`  `,
    `Mode: \`${markdownCell(selection?.mode || "invalid")}\`  `,
    `Reason: \`${markdownCell(selection?.reason || "invalid")}\`  `,
    `Areas: \`${markdownCell(selection?.areas ?? "")}\`  `,
    `Base: \`${markdownCell(selection?.base ?? "")}\`  `,
    `Head: \`${markdownCell(selection?.head ?? "")}\`  `,
    `Merge: \`${markdownCell(selection?.merge ?? "")}\``,
    "",
    "</details>",
    "",
  ].join("\n");
}

function main() {
  const options = parseArgs(process.argv.slice(2));
  const event = process.env[options.eventEnv];
  if (!isSelectionEvent(event)) throw error("event-input");
  const needs = readNeeds(options.needsEnv);
  checkJobs(needs);
  const result = assessJobs(needs, event);
  const summary = render(needs, event, result.selection, result.errors);
  try {
    fs.appendFileSync(process.env[options.summaryEnv] || "", summary, "utf8");
  } catch {
    throw error("summary-write");
  }
  if (result.errors.length > 0) throw error(result.errors.join(","));
  process.stdout.write("ci-summary: success\n");
}

if (import.meta.main) {
  try {
    main();
  } catch (failure) {
    process.stderr.write(
      `ci-summary: ${failure instanceof Error ? failure.message : "failure"}\n`,
    );
    process.exitCode = 1;
  }
}
