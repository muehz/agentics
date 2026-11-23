/**
 * The `--validate` stages: build the trusted hook verifier, run the conformance suite, typecheck, run the tools tests
 * and check the area boundaries, then print a timing table. Setup mode runs the manifests domain only.
 *
 * Every child starts through an injectable {@link Runner}, so the stage logic is testable without Go, npm or a real
 * conformance run.
 */
import { spawnSync } from "node:child_process";
import {
  closeSync,
  mkdirSync,
  mkdtempSync,
  openSync,
  readFileSync,
  rmSync,
  statSync,
  writeSync,
} from "node:fs";
import os from "node:os";
import path from "node:path";
import { performance } from "node:perf_hooks";
import { checkNodeFloor } from "./floor.ts";
import { CLI_COMMAND } from "./args.ts";
import {
  REPO_ROOT,
  findCommand,
  spawnPlan,
  type SpawnPlan,
  toolCacheDir as defaultToolCacheDir,
} from "./common.ts";

export type StageKey =
  | "builds"
  | "hooks_conformance"
  | "ts_conformance"
  | "typecheck"
  | "tools_tests"
  | "boundaries";

/** The stages in run order. The timing table uses its own order, which puts the builds last. */
export const VALIDATE_STAGES: readonly { key: StageKey; label: string }[] = [
  { key: "builds", label: "Prerequisite builds" },
  { key: "hooks_conformance", label: "Hooks conformance" },
  { key: "ts_conformance", label: "TypeScript conformance" },
  { key: "typecheck", label: "Typecheck" },
  { key: "tools_tests", label: "tools tests" },
  { key: "boundaries", label: "Area boundaries" },
];

const SUMMARY_ORDER: readonly StageKey[] = [
  "hooks_conformance",
  "ts_conformance",
  "typecheck",
  "tools_tests",
  "boundaries",
  "builds",
];

/** `tool` is a logical name, so callers and tests stay independent of how each platform starts it. */
export type Command = {
  tool: "node" | "npm" | "go";
  args: string[];
  env: NodeJS.ProcessEnv;
  capture?: boolean;
};

/** `found: false` means the tool is not on PATH. A signal death reads as status 1. */
export type CommandResult = { found: boolean; status: number; output: string };

export type Runner = (command: Command) => CommandResult;

export { spawnPlan, type SpawnPlan } from "./common.ts";

function writeError(text: string): void {
  writeSync(2, `${text}\n`);
}

/**
 * Starts `node` as `process.execPath`, the Node the floor check accepted, never a `node` from PATH. `npm` and `go`
 * resolve on the command's PATH. Captured output goes to one file for both streams, so their lines keep the order the
 * child wrote them in.
 */
export const defaultRunner: Runner = (command) => {
  const file =
    command.tool === "node"
      ? process.execPath
      : findCommand(command.tool, command.env, process.platform);
  if (file === undefined) return { found: false, status: 1, output: "" };
  let plan: SpawnPlan;
  try {
    plan = spawnPlan(file, command.args, process.platform, command.env);
  } catch (error) {
    writeError(`❌ ${(error as Error).message}`);
    return { found: true, status: 1, output: "" };
  }
  let captureDir: string | undefined;
  let fd: number | undefined;
  try {
    if (command.capture === true) {
      captureDir = mkdtempSync(path.join(os.tmpdir(), "agentics-validate-"));
      fd = openSync(path.join(captureDir, "output"), "w");
    }
    const result = spawnSync(plan.file, plan.args, {
      env: command.env,
      stdio: fd === undefined ? "inherit" : ["inherit", fd, fd],
      windowsVerbatimArguments: plan.windowsVerbatimArguments,
    });
    let output = "";
    if (fd !== undefined && captureDir !== undefined) {
      closeSync(fd);
      fd = undefined;
      output = readFileSync(path.join(captureDir, "output"), "utf8");
    }
    if (result.error !== undefined) {
      writeError(`❌ Could not start ${file}: ${result.error.message}`);
      return { found: true, status: 1, output };
    }
    return { found: true, status: result.status ?? 1, output };
  } finally {
    if (fd !== undefined) closeSync(fd);
    if (captureDir !== undefined)
      rmSync(captureDir, { recursive: true, force: true });
  }
};

export type StageRecord = {
  result: "passed" | "failed" | "skipped";
  seconds?: number;
};
export type StageRecords = Partial<Record<StageKey, StageRecord>>;

/** The table printed on every strict exit after the Node check. A stage with no record did not run. */
export function formatTimingSummary(
  records: StageRecords,
  totalSeconds: number,
): string {
  const label = (key: StageKey) =>
    VALIDATE_STAGES.find((stage) => stage.key === key)?.label ?? key;
  const rows = SUMMARY_ORDER.map((key) => {
    const record = records[key];
    if (record === undefined) return `${label(key).padEnd(24)} not run`;
    if (record.result === "skipped") return `${label(key).padEnd(24)} skipped`;
    return `${label(key).padEnd(24)} ${record.result.padEnd(8)} ${record.seconds ?? 0}s`;
  });
  return [
    "",
    "=== Timing summary ===",
    ...rows,
    `${"Total".padEnd(24)} ${"".padEnd(8)} ${totalSeconds}s`,
    "",
  ].join("\n");
}

export interface ValidateOptions {
  mode: "strict" | "setup";
  speed?: "full" | "fast";
  repoRoot?: string;
  toolCacheDir?: string;
  env?: NodeJS.ProcessEnv;
  platform?: NodeJS.Platform;
  runner?: Runner;
  clock?: () => number;
  nodeVersion?: string;
  enginesNode?: string;
  write?: (stream: "stdout" | "stderr", text: string) => void;
}

/** The strict run covers every domain; the hooks domain runs first as its own stage and is excluded from this one. */
const STRICT_DOMAIN = "all";
const SETUP_DOMAIN = "manifests";

function goFloor(repoRoot: string): string {
  try {
    const text = readFileSync(
      path.join(repoRoot, "hooks", "go", "go.mod"),
      "utf8",
    );
    for (const line of text.split(/\r?\n/)) {
      const fields = line.trim().split(/\s+/);
      if (fields[0] === "go" && fields[1] !== undefined) return fields[1];
    }
  } catch {
    // An unreadable go.mod leaves the version blank, as the awk lookup did.
  }
  return "";
}

const defaultWrite = (stream: "stdout" | "stderr", text: string): void => {
  // Synchronous writes to the descriptors keep these lines in order with output of children that inherit them.
  writeSync(stream === "stdout" ? 1 : 2, text);
};

const defaultClock = (): number => Math.floor(performance.now() / 1000);

/** Runs `--validate` (strict) or the install-time manifests check (setup) and returns the exit code, 0 or 1. */
export function runValidate(options: ValidateOptions): number {
  const repoRoot = options.repoRoot ?? REPO_ROOT;
  const env = options.env ?? process.env;
  const cacheDir = options.toolCacheDir ?? defaultToolCacheDir(env);
  const platform = options.platform ?? process.platform;
  const runner = options.runner ?? defaultRunner;
  const clock = options.clock ?? defaultClock;
  const write = options.write ?? defaultWrite;
  const speed = options.speed ?? "full";
  const out = (text: string) => write("stdout", `${text}\n`);
  const err = (text: string) => write("stderr", `${text}\n`);

  const conformance = (
    domain: string,
    extra: string[],
    childEnv: NodeJS.ProcessEnv,
  ): CommandResult =>
    runner({
      tool: "node",
      args: [
        path.join(repoRoot, ".github", "tools", "src", "conformance", "cli.ts"),
        domain,
        "--repo",
        repoRoot,
        ...extra,
      ],
      env: childEnv,
    });

  if (options.mode === "setup") {
    out("=== Validating manifests ===");
    const result = conformance(SETUP_DOMAIN, [], env);
    out("");
    if (result.status !== 0) {
      out(
        `❌ Validation failed (conformance ${SETUP_DOMAIN}) — see the FAIL/ERROR lines above.`,
      );
      return 1;
    }
    out(`✅ All manifests valid (conformance ${SETUP_DOMAIN}).`);
    return 0;
  }

  // The floor check reports an unreadable or out-of-range engines.node as one line, never a stack trace.
  const nodeOk = checkNodeFloor({
    action: "validate",
    repoRoot,
    write,
    nodeVersion: options.nodeVersion,
    enginesNode: options.enginesNode,
  });
  if (!nodeOk) return 1;

  const records: StageRecords = {};
  const started = clock();
  const finish = (code: number): number => {
    out(formatTimingSummary(records, clock() - started).replace(/\n$/, ""));
    return code;
  };

  /** A stage returns its exit status, or "skipped" when the checkout lacks its input. */
  const runStage = (key: StageKey, stage: () => number | "skipped"): number => {
    const start = clock();
    const outcome = stage();
    const seconds = clock() - start;
    if (outcome === "skipped") {
      records[key] = { result: "skipped", seconds };
      return 0;
    }
    records[key] = { result: outcome === 0 ? "passed" : "failed", seconds };
    return outcome;
  };

  // The verifier lives outside the checkout, because a directory-mode plugin install copies the working tree
  // including ignored files. It is rebuilt on every run: Go's build cache keeps a warm rebuild fast and a stale
  // verifier impossible. Go writes the -o name verbatim, so Windows needs the extension.
  const verifier = path.join(
    cacheDir,
    platform === "win32" ? "hooktool.exe" : "hooktool",
  );
  const built = runStage("builds", () => {
    let result: CommandResult | undefined;
    try {
      mkdirSync(cacheDir, { recursive: true });
    } catch {
      // Whether Go is installed is unknown here; the build cannot run either way.
      result = { found: true, status: 1, output: "" };
    }
    result ??= runner({
      tool: "go",
      args: [
        "build",
        "-C",
        path.join(repoRoot, "hooks", "go"),
        "-o",
        verifier,
        "./cmd/hooktool",
      ],
      env,
    });
    if (result.found && result.status === 0) return 0;
    if (result.found) err("❌ Could not build the trusted hook verifier.");
    err(
      `❌ --validate needs Go ${goFloor(repoRoot)} to build the hook verifier`,
    );
    return 1;
  });
  if (built !== 0) return finish(1);

  const childEnv: NodeJS.ProcessEnv = { ...env, AGX_HOOK_VERIFIER: verifier };

  out("=== Validating manifests ===");
  // A hooks failure does not stop the other domains, so one run reports the findings of both.
  const hooksStatus = runStage(
    "hooks_conformance",
    () =>
      conformance("hooks", speed === "fast" ? ["--fast"] : [], childEnv).status,
  );
  const tsStatus = runStage(
    "ts_conformance",
    () =>
      conformance(
        STRICT_DOMAIN,
        ["--exclude", "hooks"],
        speed === "fast"
          ? { ...childEnv, AGX_CONFORMANCE_SHORT: "1" }
          : childEnv,
      ).status,
  );
  out("");
  if (hooksStatus !== 0 || tsStatus !== 0) {
    out(
      `❌ Validation failed (conformance ${STRICT_DOMAIN}) — see the FAIL/ERROR lines above.`,
    );
    return finish(1);
  }
  out(`✅ All manifests valid (conformance ${STRICT_DOMAIN}).`);

  // The typecheck, tools tests and boundaries stop at their first failure; the summary still prints either way.
  const toolsStages = (): number => {
    const toolsPackage = path.join(
      repoRoot,
      ".github",
      "tools",
      "package.json",
    );
    if (!isFile(toolsPackage)) {
      records.typecheck = { result: "skipped" };
      records.boundaries = { result: "skipped" };
      if (speed !== "fast") records.tools_tests = { result: "skipped" };
      return 0;
    }

    out(
      speed === "fast"
        ? "=== Typechecking TypeScript workspaces (node:test skipped) ==="
        : "=== Testing TypeScript workspaces (typecheck and node:test) ===",
    );

    const typecheck = runStage("typecheck", () => {
      const tsc = findCommand(
        "tsc",
        {
          PATH: path.join(repoRoot, "node_modules", ".bin"),
          PATHEXT: env.PATHEXT,
        },
        platform,
      );
      if (tsc === undefined) {
        out(
          "ℹ Typecheck skipped: TypeScript is not installed. Run npm ci --ignore-scripts to enable it.",
        );
        return "skipped";
      }
      const result = runner({
        tool: "npm",
        args: ["--prefix", repoRoot, "run", "test:typecheck"],
        env: childEnv,
      });
      if (result.status !== 0) {
        err("❌ Workspace typecheck failed — see the output above.");
        return 1;
      }
      return 0;
    });
    if (typecheck !== 0) return 1;

    if (speed !== "fast") {
      const tools = runStage("tools_tests", () => {
        const result = runner({
          tool: "npm",
          args: ["--prefix", path.join(repoRoot, ".github", "tools"), "test"],
          env: childEnv,
        });
        if (result.status !== 0) {
          err("❌ Tools tests failed — see the output above.");
          return 1;
        }
        out("✅ Tools tests passed.");
        return 0;
      });
      if (tools !== 0) return 1;
    }

    const boundaries = runStage("boundaries", () => {
      out("=== Checking area boundaries ===");
      const result = runner({
        tool: "node",
        args: [
          path.join(
            repoRoot,
            ".github",
            "tools",
            "src",
            "boundaries",
            "cli.ts",
          ),
          "--repo",
          repoRoot,
        ],
        env: childEnv,
        capture: true,
      });
      // The CLI's own output decides the closing line, so a rule it skipped is reported as skipped, never as held.
      out(result.output.replace(/\n+$/, ""));
      if (result.status !== 0) {
        err("❌ Area boundary check failed — see the output above.");
        return 1;
      }
      if (/^SKIP /m.test(result.output)) {
        out(
          "ℹ Some area boundary rules were skipped (see the SKIP lines above). Run npm ci --ignore-scripts to enable them.",
        );
      } else {
        out("✅ Area boundaries hold.");
      }
      return 0;
    });
    if (boundaries !== 0) return 1;
    return 0;
  };
  const toolsStatus = toolsStages();
  finish(toolsStatus);
  if (toolsStatus !== 0) return 1;
  if (speed === "fast") {
    out("");
    out(
      `✅ Fast validation passed. The Go and TypeScript test suites were not run; ${CLI_COMMAND} --validate must pass before a pull request.`,
    );
  }
  return 0;
}

function isFile(file: string): boolean {
  try {
    return statSync(file).isFile();
  } catch {
    return false;
  }
}
