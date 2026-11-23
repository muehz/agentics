// The text seam for .github/actions/release/action.yml: readers for the action file and the rules that tie its inputs
// and outputs to the release command modules. The rules read the release action only: the conformance action has a
// different shape (repo maps to a flag, domain is positional, tools is a path the action builds from).

import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { parse } from "yaml";
import {
  ACTION_JOB,
  lookupWorkflowStep,
} from "../../scripts/testing/workflow-steps.ts";
import { readRepoText, repoRoot } from "./repo.ts";

// The action inputs that reach the command by another route. Each entry carries the route, and the checks below verify
// that route, so an entry cannot stay here after the reason for it is gone.
export const inputsWithoutFlag: Readonly<Record<string, string>> = {
  command: "selects the subcommand",
  checkout: "locates ci-release-run.ts, cli.ts and the hook module",
  token: "reaches the command as the GH_TOKEN environment variable",
};

// What fills an action output: a release subcommand (it passes the key to the step-output writer), or the action's own
// run step (it appends the key to $GITHUB_OUTPUT, so no subcommand writes it). No output uses the second today; the
// check keeps the rule for one that might.
export type OutputWriter = "command" | "shell";

export const outputWriters: Readonly<Record<string, OutputWriter>> = {
  release_id: "command",
  published: "command",
  mode: "command",
  resume_tag: "command",
};

export interface ActionStep {
  readonly id: string;
  readonly env: Readonly<Record<string, string>>;
  readonly run: string;
}

export interface ReleaseAction {
  readonly inputs: Readonly<Record<string, string>>; // name -> description
  readonly outputs: Readonly<Record<string, string>>; // name -> value
  readonly steps: readonly ActionStep[];
}

type Mapping = Readonly<Record<string, unknown>>;

function isMapping(value: unknown): value is Mapping {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

// The value under key when parent is a mapping, so a missing or misplaced step reads as absent.
function valueAt(parent: unknown, key: string): unknown {
  return isMapping(parent) && Object.hasOwn(parent, key)
    ? parent[key]
    : undefined;
}

// A scalar as text; a null, a missing value or a collection reads as "".
function textOf(value: unknown): string {
  if (value === null || value === undefined || typeof value === "object")
    return "";
  return String(value);
}

// Parses the part of the action file these checks read.
export function parseReleaseAction(text: string): ReleaseAction {
  const doc: unknown = parse(text);
  const inputs: Record<string, string> = {};
  const outputs: Record<string, string> = {};
  const inputNode = valueAt(doc, "inputs");
  const outputNode = valueAt(doc, "outputs");
  if (isMapping(inputNode)) {
    for (const [name, node] of Object.entries(inputNode)) {
      inputs[name] = textOf(valueAt(node, "description"));
    }
  }
  if (isMapping(outputNode)) {
    for (const [name, node] of Object.entries(outputNode)) {
      outputs[name] = textOf(valueAt(node, "value"));
    }
  }
  if (Object.keys(inputs).length === 0 || Object.keys(outputs).length === 0) {
    throw new Error("the action declares no inputs or no outputs");
  }
  const stepNode = valueAt(valueAt(doc, "runs"), "steps");
  const steps: ActionStep[] = [];
  if (Array.isArray(stepNode)) {
    for (const item of stepNode as unknown[]) {
      const env: Record<string, string> = {};
      const envNode = valueAt(item, "env");
      if (isMapping(envNode)) {
        for (const [key, value] of Object.entries(envNode))
          env[key] = textOf(value);
      }
      steps.push({
        id: textOf(valueAt(item, "id")),
        env,
        run: textOf(valueAt(item, "run")),
      });
    }
  }
  return { inputs, outputs, steps };
}

const actionFile = ".github/actions/release/action.yml";

export function loadReleaseAction(): { action: ReleaseAction; source: string } {
  const source = readRepoText(actionFile);
  return { action: parseReleaseAction(source), source };
}

/** Resolve the executable step together with its declared shell. */
export function checkedReleaseStep(source: string, name: string) {
  return lookupWorkflowStep(source, actionFile, ACTION_JOB, name);
}

// The subcommands the action dispatches, read from the description of its command input. A
// subcommand the action never dispatches, such as set-version, is out of scope.
export function releaseCommands(action: ReleaseAction): string[] {
  const commands = (action.inputs["command"] ?? "")
    .split("|")
    .map((field) => field.trim())
    .filter((field) => field !== "");
  if (commands.length === 0)
    throw new Error("the command input names no subcommand");
  return commands;
}

// The step the action's outputs read: the one step with an id.
export function runStep(action: ReleaseAction): ActionStep {
  const step = action.steps.find((candidate) => candidate.id !== "");
  if (step === undefined) {
    throw new Error(
      "the action has no step with an id, so no step can carry an output",
    );
  }
  return step;
}

// The step that builds the one Go binary the action still needs: ./cmd/hooktool, the hook verifier
// bundle runs.
export function buildStep(action: ReleaseAction): ActionStep {
  const target = "./cmd/hooktool";
  const step = action.steps.find(
    (candidate) =>
      candidate.run.includes("go build") && candidate.run.includes(target),
  );
  if (step === undefined) throw new Error(`no step builds ${target}`);
  return step;
}

// Whether a step reads one action input, through its environment or directly in its script.
export function stepReadsInput(
  env: Readonly<Record<string, string>>,
  script: string,
  input: string,
): boolean {
  const reference = `inputs.${input}`;
  return (
    script.includes(reference) ||
    Object.values(env).some((value) => value.includes(reference))
  );
}

// Source scanning shared by the flag and output readers. Both walk text that is code, so they skip string contents when
// they count brackets.

// Returns the text strictly inside the bracket pair that opens at text[open].
export function sliceBalanced(text: string, open: number): string {
  const opener = text.charAt(open);
  const closer = ({ "(": ")", "[": "]", "{": "}" } as Record<string, string>)[
    opener
  ];
  if (closer === undefined) throw new Error(`no bracket at index ${open}`);
  let depth = 0;
  let quote = "";
  for (let i = open; i < text.length; i++) {
    const ch = text.charAt(i);
    if (quote !== "") {
      if (ch === "\\" && quote !== "`") i++;
      else if (ch === quote) quote = "";
      continue;
    }
    if (ch === '"' || ch === "'" || ch === "`") quote = ch;
    else if (ch === opener) depth++;
    else if (ch === closer && --depth === 0) return text.slice(open + 1, i);
  }
  throw new Error(`unbalanced ${opener} at index ${open}`);
}

// Splits text on commas that sit outside every bracket pair and string.
export function splitTopLevel(text: string): string[] {
  const parts: string[] = [];
  let depth = 0;
  let quote = "";
  let start = 0;
  for (let i = 0; i < text.length; i++) {
    const ch = text.charAt(i);
    if (quote !== "") {
      if (ch === "\\" && quote !== "`") i++;
      else if (ch === quote) quote = "";
      continue;
    }
    if (ch === '"' || ch === "'" || ch === "`") quote = ch;
    else if ("([{".includes(ch)) depth++;
    else if (")]}".includes(ch)) depth--;
    else if (ch === "," && depth === 0) {
      parts.push(text.slice(start, i));
      start = i + 1;
    }
  }
  parts.push(text.slice(start));
  return parts.map((part) => part.trim()).filter((part) => part !== "");
}

// The literal that identifier `name` is assigned in `text`, as the text inside its brackets. `opener` is "[" or "{";
function assignedLiteral(
  text: string,
  name: string,
  opener: string,
): string | undefined {
  const escaped = name.replace(/[$]/g, "\\$");
  const pattern = new RegExp(`\\b${escaped}\\b[^=\\n]*:?=\\s*\\${opener}`);
  const match = pattern.exec(text);
  if (match === null) return undefined;
  return sliceBalanced(text, match.index + match[0].length - 1);
}

// The flags one TypeScript command module accepts: the union of the arrays that its parseArgs and parseFlags calls take
// as the allow-list, each written inline or through a const declared in the same file. Both take the allow-list second;
// parseFlags is the wrapper the commands that read flags through io call. Every call counts, because a module can parse
// a subset first (bundle reads the version inputs before its own flags), and reading only that call would miss flags
// the command accepts. A leading "--" is dropped, and a flag named by more than one call is listed once.
export function flagsFromSource(text: string): string[] {
  const calls = [...text.matchAll(/\bparse(?:Args|Flags)\s*\(/g)];
  if (calls.length === 0)
    throw new Error("the command module never calls parseArgs or parseFlags");
  const flags = new Set<string>();
  for (const call of calls) {
    const open = call.index + call[0].length - 1;
    const args = splitTopLevel(sliceBalanced(text, open));
    const allowed = args[1];
    if (allowed === undefined)
      throw new Error("parseArgs takes no allow-list argument");
    let literal: string | undefined;
    if (allowed.startsWith("[")) literal = sliceBalanced(allowed, 0);
    else
      literal = assignedLiteral(
        text,
        allowed.replace(/\s+as\s+const$/, ""),
        "[",
      );
    if (literal === undefined)
      throw new Error(`cannot find the allow-list ${allowed}`);
    for (const match of literal.matchAll(/(["'])((?:(?!\1).)+)\1/g))
      flags.add((match[2] ?? "").replace(/^--/, ""));
  }
  return [...flags];
}

function outputKeys(literal: string): string[] {
  const keys: string[] = [];
  for (const entry of splitTopLevel(literal)) {
    if (entry.startsWith("...")) continue;
    const match = /^(?:"([^"]+)"|'([^']+)'|([A-Za-z_$][\w$]*))\s*(?::|$)/.exec(
      entry,
    );
    const key = match?.[1] ?? match?.[2] ?? match?.[3];
    if (key !== undefined) keys.push(key);
  }
  return keys;
}

// The output keys one command module writes: the keys of the object that each writeOutputs call takes first, written
// inline or through a variable the same file assigns.
export function outputKeysFromTypeScript(text: string): string[] {
  const keys: string[] = [];
  for (const call of text.matchAll(/\bwriteOutputs\(/g)) {
    const args = splitTopLevel(
      sliceBalanced(text, call.index + call[0].length - 1),
    );
    const arg = args[0] ?? "";
    const literal = arg.startsWith("{")
      ? sliceBalanced(arg, 0)
      : /^[A-Za-z_$][\w$]*$/.test(arg)
        ? assignedLiteral(text, arg, "{")
        : undefined;
    if (literal !== undefined) keys.push(...outputKeys(literal));
  }
  return keys;
}

// Each output key a command source writes, with the files that write it.
export function outputsWrittenByCommands(): Map<string, string[]> {
  const written = new Map<string, string[]>();
  const dir = ".github/tools/src/release/commands";
  const files = readdirSync(join(repoRoot, dir))
    .filter((name) => name.endsWith(".ts") && !name.endsWith(".test.ts"))
    .sort();
  for (const file of files) {
    const text = readFileSync(join(repoRoot, dir, file), "utf8");
    for (const key of outputKeysFromTypeScript(text))
      written.set(key, [...(written.get(key) ?? []), file]);
  }
  return written;
}

// The flags each action command registers, keyed by flag name, read from the command modules' parseArgs allow-lists.
export function registeredFlags(
  commands: readonly string[],
): Map<string, string[]> {
  const registered = new Map<string, string[]>();
  for (const command of commands) {
    const flags = flagsFromSource(
      readRepoText(`.github/tools/src/release/commands/${command}.ts`),
    );
    for (const flag of flags)
      registered.set(flag, [...(registered.get(flag) ?? []), command]);
  }
  return registered;
}

// Every input maps to a flag or carries an allow-list entry, every flag has an input, every input is read, and the
// three allow-list entries keep earning their place.
export function actionInputViolations(
  action: ReleaseAction,
  source: string,
  registered: ReadonlyMap<string, readonly string[]>,
  commands: readonly string[],
): string[] {
  const violations: string[] = [];
  const inputs = Object.keys(action.inputs).sort();
  for (const input of inputs) {
    if (input in inputsWithoutFlag) continue;
    if ((registered.get(input) ?? []).length === 0) {
      violations.push(
        `action input ${JSON.stringify(input)} maps to no --${input} flag of ${commands.join(", ")}: restore the flag name, or drop the input`,
      );
    }
  }
  for (const name of [...registered.keys()].sort()) {
    if (!(name in action.inputs)) {
      violations.push(
        `--${name} is a flag of release ${(registered.get(name) ?? []).join(" and ")}, and the action declares no input of that name`,
      );
    }
  }
  // An input that no step reads is dead weight, and it hides a rename that broke only the call site.
  for (const input of inputs) {
    if (!source.includes(`inputs.${input}`)) {
      violations.push(
        `action input ${JSON.stringify(input)} is declared and never read`,
      );
    }
  }
  const owners = registered.get("token") ?? [];
  if (owners.length !== 0) {
    violations.push(
      `release ${owners.join(" and ")} now registers --token, so the allowlist entry for the token input is stale`,
    );
  }
  const token = runStep(action).env["GH_TOKEN"];
  if (token !== "${{ inputs.token }}") {
    violations.push(
      `the run step passes GH_TOKEN as ${JSON.stringify(token ?? "")}, so the token input no longer reaches the command`,
    );
  }
  const build = buildStep(action);
  if (!stepReadsInput(build.env, build.run, "checkout")) {
    violations.push(
      "the build step no longer reads the checkout input, so that allowlist entry is stale",
    );
  }
  return violations;
}

// Every output records its writer, reads the run step's output of the same name, and has that writer; every record
// names a declared output.
export function actionOutputViolations(
  action: ReleaseAction,
  written: ReadonlyMap<string, readonly string[]>,
  writers: Readonly<Record<string, OutputWriter>> = outputWriters,
): string[] {
  const violations: string[] = [];
  const run = runStep(action);
  for (const name of Object.keys(action.outputs).sort()) {
    const writer = writers[name];
    if (writer === undefined) {
      violations.push(
        `action output ${JSON.stringify(name)} has no recorded writer: add it to outputWriters with the writer that fills it`,
      );
      continue;
    }
    // Every output reads the step that runs the command, so the output name and the step's key cannot drift apart.
    const want = "${{ steps." + run.id + ".outputs." + name + " }}";
    const got = (action.outputs[name] ?? "").trim();
    if (got !== want) {
      violations.push(
        `action output ${JSON.stringify(name)} reads ${JSON.stringify(got)}, want ${JSON.stringify(want)}`,
      );
    }
    const files = written.get(name) ?? [];
    if (writer === "command" && files.length === 0) {
      violations.push(
        `no command source passes ${JSON.stringify(name)} to the output writer, so a release subcommand writes nothing for that output`,
      );
    }
    if (writer === "shell") {
      if (files.length !== 0) {
        violations.push(
          `${files.join(" and ")} now writes ${JSON.stringify(name)} through the output writer, so the action's run step must stop writing it`,
        );
      }
      if (
        !run.run.includes(`${name}=%s`) ||
        !run.run.includes("$GITHUB_OUTPUT")
      ) {
        violations.push(
          `the run step no longer appends ${name}= to $GITHUB_OUTPUT, so that output has no writer`,
        );
      }
    }
  }
  for (const name of Object.keys(writers).sort()) {
    if (!(name in action.outputs)) {
      violations.push(
        `outputWriters records ${JSON.stringify(name)}, which the action no longer declares`,
      );
    }
  }
  return violations;
}
