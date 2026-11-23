#!/usr/bin/env node

// Runs gremlins on hooks/go packages, each after a two-phase baseline in an isolated copy of the module.
// gremlins copies only the module into a temporary folder, so a test that reads outside the module, or that hashes
// the module's source, fails for every mutant and gremlins reports kills that no test made. The baseline finds and
// excludes such packages: phase 1 runs the tests unchanged in a copy, and phase 2 runs them again after a
// comment-only change, which alters the source digest but no behavior.
//   node .github/tools/src/scripts/mutation-gremlins.ts --baseline-only <pkg> ...
//   node .github/tools/src/scripts/mutation-gremlins.ts --scope <scope.json> --out <file>
//   node .github/tools/src/scripts/mutation-gremlins.ts --packages <pkg> ... --out <file>

import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { spawnSync, type SpawnSyncOptions } from "node:child_process";
import {
  encodeGremlinsReport,
  GREMLINS_FILE,
  readGremlinsTargets,
  type GremlinsPackage,
} from "./mutation-artifacts.ts";

export type BaselineResult = GremlinsPackage["baseline"];
interface CommandResult {
  status: number | null;
  error?: Error;
  signal?: NodeJS.Signals | null;
}
export type GremlinsRun = (
  command: string,
  args: unknown[],
  options: SpawnSyncOptions,
) => CommandResult;
function nativeJoin(...parts: unknown[]): string {
  return Reflect.apply(path.join, path, parts);
}
function nativeRun(
  command: string,
  args: unknown[],
  options: SpawnSyncOptions,
): CommandResult {
  return Reflect.apply(spawnSync, undefined, [command, args, options]);
}
function includes(changed: unknown, name: string): boolean {
  if (typeof changed === "string") return changed.includes(name);
  if (Array.isArray(changed)) return changed.includes(name);
  throw new TypeError("changed.includes is not a function");
}
const ROOT = path.resolve(import.meta.dirname, "../../../..");
const MODULE = path.join(ROOT, "hooks", "go");
const PROBE = "\n// mutation baseline probe\n";

function appendProbe(text: string) {
  return text + PROBE;
}

// gremlins mutates a package folder and every folder below it, and it matches --exclude-files against the path
// relative to the package. The "/" pattern drops every nested folder: those packages get their own run and baseline.
// With a changed list, each unchanged top-level file is excluded too.
function excludePatterns(allFiles: readonly string[], changed: unknown = null) {
  const escape = (name: string) => name.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  const unchanged = changed
    ? allFiles.filter((name) => !includes(changed, name))
    : [];
  return ["/", ...unchanged.map((name) => `^${escape(name)}$`)];
}

function sourceFiles(dir: string) {
  return fs
    .readdirSync(dir)
    .filter((name) => name.endsWith(".go") && !name.endsWith("_test.go"))
    .sort();
}

function goTest(dir: string, pkg: unknown, env: NodeJS.ProcessEnv) {
  const run = Reflect.apply(spawnSync, undefined, [
    "go",
    ["test", "-count=1", pkg],
    {
      cwd: dir,
      env: { ...process.env, ...env },
      encoding: "utf8",
      timeout: 15 * 60 * 1000,
    },
  ]) as ReturnType<typeof spawnSync>;
  const text = `${run.stdout ?? ""}${run.stderr ?? ""}`.trim();
  return {
    ok: run.status === 0,
    output: run.error
      ? `${run.error.message}`
      : text.split("\n").slice(-5).join(" "),
  };
}

function baseline(
  moduleDir: string,
  pkg: unknown,
  env: NodeJS.ProcessEnv,
): BaselineResult {
  const holder = fs.mkdtempSync(path.join(os.tmpdir(), "mutation-baseline-"));
  const copy = path.join(holder, "go");
  try {
    fs.cpSync(moduleDir, copy, { recursive: true });
    const first = goTest(copy, pkg, env);
    if (!first.ok)
      return {
        pkg,
        ok: false,
        reason: `fails in an isolated copy: ${first.output}`,
      };
    const dir = nativeJoin(copy, pkg);
    for (const name of sourceFiles(dir)) {
      const file = path.join(dir, name);
      fs.writeFileSync(file, appendProbe(fs.readFileSync(file, "utf8")));
    }
    const second = goTest(copy, pkg, env);
    if (!second.ok)
      return {
        pkg,
        ok: false,
        reason: `fails after a comment-only change: ${second.output}`,
      };
    return { pkg, ok: true };
  } finally {
    fs.rmSync(holder, { recursive: true, force: true });
  }
}

function rootEnv() {
  return {
    AGX_REPO_ROOT: ROOT,
    CONFORMANCE_VERIFY_REPO: ROOT,
    GOTOOLCHAIN: "local",
    CGO_ENABLED: "0",
  };
}

function gremlinsArgs(
  pkg: unknown,
  allFiles: readonly string[],
  changed: unknown,
  out: string,
) {
  const args: unknown[] = ["unleash", "--workers", "4", "--output", out];
  for (const pattern of excludePatterns(allFiles, changed))
    args.push("--exclude-files", pattern);
  args.push(pkg);
  return args;
}

// `run` runs one gremlins command; tests replace it.
function runPackages(
  targets: Iterable<unknown>,
  out: string,
  run: GremlinsRun = nativeRun,
) {
  const env = rootEnv();
  const work = fs.mkdtempSync(path.join(os.tmpdir(), "mutation-gremlins-"));
  const packages: GremlinsPackage[] = [];
  // The file is rewritten after each package, so a run that hits its time limit keeps the packages that finished.
  const save = () => {
    fs.mkdirSync(path.dirname(out), { recursive: true });
    fs.writeFileSync(out, encodeGremlinsReport(packages));
  };
  try {
    // This property view retains native destructuring errors; neither field is claimed validated.
    for (const { pkg, changed } of targets as Iterable<
      Record<string, unknown>
    >) {
      const check = baseline(MODULE, pkg, env);
      if (!check.ok) {
        packages.push({ pkg, baseline: check });
        save();
        continue;
      }
      const json = path.join(
        work,
        `${Reflect.apply(String.prototype.replaceAll, pkg, ["/", "_"])}.json`,
      );
      const done = run(
        "gremlins",
        gremlinsArgs(pkg, sourceFiles(nativeJoin(MODULE, pkg)), changed, json),
        {
          cwd: MODULE,
          // gremlins sets each mutant's time limit from its first (coverage) test run. A cached test result makes that run
          // take no time, and then every mutant times out. -count=1 turns the test cache off.
          env: {
            ...process.env,
            ...env,
            GOFLAGS: [process.env.GOFLAGS, "-count=1"]
              .filter(Boolean)
              .join(" "),
            TMPDIR: work,
          },
          stdio: "inherit",
        },
      );
      let result: unknown;
      let error: string | undefined;
      if (done.status !== 0) {
        error = `gremlins did not finish: ${done.error?.message ?? done.signal ?? `exit ${done.status}`}`;
      } else if (!fs.existsSync(json)) {
        error = "gremlins wrote no result file";
      } else {
        try {
          result = JSON.parse(fs.readFileSync(json, "utf8"));
        } catch {
          error = "gremlins wrote an unreadable result file";
        }
      }
      packages.push({ pkg, baseline: check, result, error });
      save();
    }
  } finally {
    fs.rmSync(work, { recursive: true, force: true });
  }
  save();
  // A package excluded by the baseline or a gremlins failure is a tool failure: the job goes red, never on a survivor.
  return packages.filter((entry) => !entry.baseline.ok || entry.error).length;
}

function main(argv: string[]) {
  const out = argv.includes("--out")
    ? argv[argv.indexOf("--out") + 1]!
    : path.join(ROOT, "reports/mutation", GREMLINS_FILE);
  if (argv[0] === "--baseline-only") {
    let status = 0;
    for (const pkg of argv.slice(1)) {
      const result = baseline(MODULE, pkg, rootEnv());
      console.log(
        result.ok
          ? `${pkg}: passed both baseline phases`
          : `${pkg}: excluded, ${result.reason}`,
      );
      if (!result.ok) status = 1;
    }
    return status;
  }
  if (argv[0] === "--scope") {
    return runPackages(readGremlinsTargets(argv[1]!), out) === 0 ? 0 : 1;
  }
  if (argv[0] === "--packages") {
    const end = argv.includes("--out") ? argv.indexOf("--out") : argv.length;
    const targets = argv.slice(1, end).map((pkg) => ({ pkg, changed: null }));
    return runPackages(targets, out) === 0 ? 0 : 1;
  }
  console.error(
    "usage: mutation-gremlins.ts --baseline-only <pkg> ... | --scope <scope.json> --out <file> | --packages <pkg> ... --out <file>",
  );
  return 2;
}

if (import.meta.main) process.exit(main(process.argv.slice(2)));

export {
  baseline,
  appendProbe,
  excludePatterns,
  gremlinsArgs,
  runPackages,
  sourceFiles,
  goTest,
  rootEnv,
  ROOT,
  MODULE,
};
