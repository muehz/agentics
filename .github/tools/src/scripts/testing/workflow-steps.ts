import { spawnSync } from "node:child_process";
import fs from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import YAML from "yaml";
import {
  findInterpreter,
  requireInterpreter,
  type Interpreter,
  type InterpreterOptions,
} from "../../entrypoint/interpreters.ts";
import { spawnPlan } from "../../plugin/common.ts";

/**
 * The job name the workflow tests give a composite action's `runs.steps`: an
 * action has no jobs, and this name keeps one file/job/step key for both kinds.
 */
export const ACTION_JOB = "runs";

/**
 * Every hand-written workflow, found by listing the directory (the gh-aw
 * `.lock.yml` files are compiled output), and every composite action, keyed by
 * repository-relative path with CRLF line endings normalized.
 */
export function workflowSources(root: string): Map<string, string> {
  const sources = new Map<string, string>();
  const read = (file: string) =>
    fs.readFileSync(path.join(root, file), "utf8").replaceAll("\r\n", "\n");
  for (const name of fs
    .readdirSync(path.join(root, ".github/workflows"))
    .sort())
    if (name.endsWith(".yml") && !name.endsWith(".lock.yml"))
      sources.set(
        `.github/workflows/${name}`,
        read(`.github/workflows/${name}`),
      );
  for (const name of fs
    .readdirSync(path.join(root, ".github/actions"))
    .sort()) {
    const file = `.github/actions/${name}/action.yml`;
    if (fs.existsSync(path.join(root, file))) sources.set(file, read(file));
  }
  return sources;
}

/** One step as the workflow tests read it; other keys are kept but untyped. */
export interface WorkflowStepData {
  name?: string;
  id?: string;
  if?: string;
  run?: string;
  uses?: string;
  shell?: string;
  env?: Record<string, string>;
  with?: Record<string, unknown>;
  [key: string]: unknown;
}

/** The steps of one job, or of a composite action under {@link ACTION_JOB}. */
export interface JobSteps {
  file: string;
  job: string;
  steps: WorkflowStepData[];
}

function jobBodies(doc: object): [string, unknown][] {
  const jobs: unknown = Reflect.get(doc, "jobs");
  return jobs !== null && typeof jobs === "object"
    ? Object.entries(jobs)
    : [[ACTION_JOB, Reflect.get(doc, "runs")]];
}

function mapping(value: unknown): Record<string, unknown> | undefined {
  if (value === null || typeof value !== "object" || Array.isArray(value))
    return undefined;
  return value as Record<string, unknown>;
}

/** Every job's steps in the parsed sources; a job without steps is left out. */
export function jobSteps(docs: Map<string, unknown>): JobSteps[] {
  const result: JobSteps[] = [];
  for (const [file, doc] of docs) {
    if (doc === null || typeof doc !== "object") continue;
    for (const [job, body] of jobBodies(doc)) {
      const steps = mapping(body)?.steps;
      if (Array.isArray(steps))
        result.push({ file, job, steps: steps as WorkflowStepData[] });
    }
  }
  return result;
}

export interface CheckedWorkflowStep {
  file: string;
  jobName: string;
  workflow: Record<string, unknown>;
  job: Record<string, unknown>;
  step: WorkflowStepData;
}

/** Replace the script for a mutation case while preserving its checked shell and job. */
export function withRunBody(
  checked: CheckedWorkflowStep,
  run: string,
): CheckedWorkflowStep {
  return { ...checked, step: { ...checked.step, run } };
}

/** Resolve one named step within its workflow job, or within a composite action's runs. */
export function lookupWorkflowStep(
  source: string | Record<string, unknown>,
  file: string,
  jobName: string,
  stepName: string,
): CheckedWorkflowStep {
  const workflow = mapping(
    typeof source === "string" ? YAML.parse(source) : source,
  );
  if (!workflow) throw new Error(`${file}: expected a workflow mapping`);
  const job = mapping(
    jobBodies(workflow).find(([name]) => name === jobName)?.[1],
  );
  if (!job) throw new Error(`${file}: job ${jobName} is missing`);
  const matches: unknown[] = Array.isArray(job.steps)
    ? job.steps.filter((step: unknown) => mapping(step)?.name === stepName)
    : [];
  if (matches.length !== 1)
    throw new Error(
      `${file} job ${jobName}: expected one step named "${stepName}", found ${matches.length}`,
    );
  return { file, jobName, workflow, job, step: matches[0] as WorkflowStepData };
}

function defaultShell(scope: Record<string, unknown>): unknown {
  return mapping(mapping(scope.defaults)?.run)?.shell;
}

/** Select the shell for this checked job even when the fixture runs on a different OS. */
export function workflowShell({
  file,
  jobName,
  workflow,
  job,
  step,
}: CheckedWorkflowStep): {
  name: Interpreter;
  flags: string[];
} {
  const declared = step.shell ?? defaultShell(job) ?? defaultShell(workflow);
  const runners = [job["runs-on"]]
    .flat()
    .filter((label): label is string => typeof label === "string");
  const strictBash = ["--noprofile", "--norc", "-e", "-o", "pipefail"];
  if (declared === undefined) {
    if (
      runners.some((label) =>
        /^(?:ubuntu|linux|macos|xcode)(?:-|$)/i.test(label),
      )
    )
      return { name: "bash", flags: ["-e"] };
    if (runners.some((label) => /^windows(?:-|$)/i.test(label)))
      return { name: "pwsh", flags: ["-command"] };
    throw new Error(
      `${file} job ${jobName}: cannot determine the shell for ${JSON.stringify(job["runs-on"])}`,
    );
  }
  if (typeof declared === "string") {
    switch (declared) {
      case "bash":
        return { name: declared, flags: strictBash };
      case "sh":
        return { name: declared, flags: ["-e"] };
      case "pwsh":
      case "powershell":
        return { name: declared, flags: ["-command"] };
    }
  }
  throw new Error(
    `${file} job ${jobName}: unsupported shell ${JSON.stringify(declared)}`,
  );
}

/** The node:test skip reason, or an error when the host requires this interpreter. */
export function workflowShellSkip(
  checked: CheckedWorkflowStep,
  host?: InterpreterOptions,
): false | string {
  return requireInterpreter(workflowShell(checked).name, host);
}

/** Runs the checked body as a script file. Shell discovery uses the host PATH before fixture overrides. */
export function runWorkflowStep(
  checked: CheckedWorkflowStep,
  options: {
    cwd: string;
    env: NodeJS.ProcessEnv;
    timeout?: number;
    maxBuffer?: number;
  },
): { status: number; stdout: string; stderr: string } {
  const { name, flags } = workflowShell(checked);
  const interpreter = findInterpreter(name);
  if (!interpreter)
    throw new Error(
      `${checked.file} job ${checked.jobName}: ${name} is not available on this host`,
    );
  const body = checked.step.run;
  if (typeof body !== "string")
    throw new Error(
      `${checked.file} job ${checked.jobName}: step "${checked.step.name}" has no run body`,
    );
  const powershell = name === "pwsh" || name === "powershell";
  const directory = fs.mkdtempSync(path.join(tmpdir(), "agentics-workflow-"));
  try {
    const script = path.join(
      directory,
      powershell ? "workflow step.ps1" : "workflow step.sh",
    );
    fs.writeFileSync(
      script,
      powershell
        ? `\uFEFF$ErrorActionPreference = 'stop'\n${body}\nif (Test-Path -LiteralPath variable:\\LASTEXITCODE) { exit $LASTEXITCODE }\n`
        : body,
    );
    const argument = powershell
      ? `. '${script.replaceAll("'", "''")}'`
      : script;
    const plan = spawnPlan(interpreter, [...flags, argument], process.platform);
    const result = spawnSync(plan.file, plan.args, {
      ...options,
      windowsVerbatimArguments: plan.windowsVerbatimArguments,
      windowsHide: true,
      encoding: "utf8",
    });
    if (result.error) throw result.error;
    if (result.status === null)
      throw new Error(`${name} terminated by ${result.signal}`);
    return {
      status: result.status,
      stdout: result.stdout,
      stderr: result.stderr,
    };
  } finally {
    fs.rmSync(directory, { recursive: true, force: true });
  }
}

/** A POSIX shell recorder for the command shim. An empty argument still gets its terminator. */
export const recordWorkflowArgv = `for arg do printf '%s\\0' "$arg"; done > "$ACTION_ARGV"`;

/** Decode the command's NUL-terminated arguments, including leading or trailing empty strings. */
export function readWorkflowArgv(file: string): string[] {
  const bytes = fs.readFileSync(file);
  if (!bytes.length) return [];
  if (bytes.at(-1) !== 0)
    throw new Error("argument recorder emitted an unterminated argument");
  return bytes.subarray(0, -1).toString("utf8").split("\0");
}

/** The exact JavaScript in a checked metadata step's quoted Node heredoc. */
export function inlineNodeScript({
  file,
  jobName,
  step,
}: CheckedWorkflowStep): string {
  const body = /^node <<'NODE'\n([\s\S]*?)\nNODE\n?$/.exec(step.run ?? "")?.[1];
  if (body === undefined)
    throw new Error(
      `${file} job ${jobName}: step "${step.name}" must be a quoted Node heredoc`,
    );
  return body;
}

/** Each step name used more than once within one job, as `file job step`. */
export function duplicateStepNames(jobs: JobSteps[]): string[] {
  const duplicates: string[] = [];
  for (const { file, job, steps } of jobs) {
    const seen = new Set<string>();
    for (const { name } of steps) {
      if (name === undefined) continue;
      if (seen.has(name)) duplicates.push(`${file} job ${job} step "${name}"`);
      seen.add(name);
    }
  }
  return duplicates;
}
