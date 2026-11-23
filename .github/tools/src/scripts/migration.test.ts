import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import { pathToFileURL } from "node:url";
import YAML from "yaml";
import {
  ACTION_JOB,
  jobSteps,
  workflowSources,
} from "./testing/workflow-steps.ts";

const root = path.resolve(import.meta.dirname, "../../../..");

test("the maintainer script directory contains no retired CommonJS files", () => {
  const remaining = fs
    .readdirSync(path.join(root, ".github/scripts"))
    .filter((name) => name.endsWith(".cjs"))
    .sort();
  assert.deepEqual(remaining, []);
});

// Where a cold-start module runs: a workflow or composite action step, or a
// line of a script or manifest. `via` names the module the step starts when
// that module, not the entry, appears in the step's command.
type ColdSite =
  | { file: string; job: string; step: string; via?: string }
  | { file: string; command: string };

interface ColdEntry {
  module: string;
  sites: ColdSite[];
}

const VALIDATE = ".github/workflows/validate.yml";
const COVERAGE = ".github/workflows/validate-coverage.yml";
const MANUAL = ".github/workflows/validate-manual.yml";
const CONFORMANCE_ACTION = ".github/actions/conformance/action.yml";
const RELEASE_ACTION = ".github/actions/release/action.yml";

// Every module that runs before any `npm ci` on its leg, so it and everything
// it imports must load from Node built-ins and relative files alone. Paths are
// relative to .github/tools/src.
const COLD_ENTRIES: ColdEntry[] = [
  {
    module: "scripts/ci-coverage-paths.ts",
    sites: [
      {
        file: VALIDATE,
        job: "scope",
        step: "Match the TypeScript coverage paths",
      },
    ],
  },
  {
    module: "scripts/ci-version-fields.ts",
    sites: [
      {
        file: VALIDATE,
        job: "pr-version-fields",
        step: "Reject a changed version field",
      },
    ],
  },
  {
    module: "scripts/ci-antigravity-native.ts",
    sites: [
      {
        file: VALIDATE,
        job: "validate-antigravity-native",
        step: "Install pinned agy",
      },
      {
        file: VALIDATE,
        job: "validate-antigravity-native",
        step: "Antigravity plugin validation",
      },
    ],
  },
  {
    module: "scripts/ci-hooks-test.ts",
    sites: [
      { file: VALIDATE, job: "go-suite", step: "Test the hooks module" },
      {
        file: VALIDATE,
        job: "go-suite",
        step: "Enforce the 90% coverage floor",
      },
    ],
  },
  {
    module: "scripts/ci-gh-aw.ts",
    sites: [
      { file: VALIDATE, job: "gh-aw-lock-drift", step: "Install pinned gh-aw" },
      {
        file: VALIDATE,
        job: "gh-aw-lock-drift",
        step: "Recompile and diff the locks",
      },
    ],
  },
  {
    module: "scripts/ci-shellcheck.ts",
    sites: [
      { file: VALIDATE, job: "shellcheck", step: "Install pinned shellcheck" },
      { file: VALIDATE, job: "shellcheck", step: "Check shell scripts" },
    ],
  },
  {
    module: "scripts/ci-coverage.ts",
    sites: [
      {
        file: COVERAGE,
        job: "coverage-report",
        step: "Require every shard profile",
      },
      {
        file: COVERAGE,
        job: "coverage-report",
        step: "Merge shard profiles per module",
      },
      {
        file: COVERAGE,
        job: "coverage-report",
        step: "Report module coverage totals",
      },
      {
        file: COVERAGE,
        job: "ts-coverage-report",
        step: "Combine complete TypeScript coverage",
      },
    ],
  },
  {
    module: "coverage/cli.ts",
    sites: [
      {
        file: COVERAGE,
        job: "coverage-report",
        step: "Convert profiles to Cobertura XML",
      },
      {
        file: COVERAGE,
        job: "ts-coverage-report",
        step: "Combine complete TypeScript coverage",
        via: "scripts/ci-coverage.ts",
      },
    ],
  },
  {
    module: "scripts/ci-native-smoke.ts",
    sites: [
      {
        file: MANUAL,
        job: "native-hook-smoke",
        step: "Build the hook binaries from source",
      },
      {
        file: MANUAL,
        job: "native-hook-smoke",
        step: "Confirm the runner is the target architecture (macOS)",
      },
      {
        file: MANUAL,
        job: "native-hook-smoke",
        step: "Confirm the runner is the target architecture (Windows)",
      },
    ],
  },
  {
    module: "scripts/ci-reminders.ts",
    sites: [
      {
        file: ".github/workflows/reminders.yml",
        job: "check-ledger-staleness",
        step: "Open or update tracking issue",
      },
    ],
  },
  {
    module: "scripts/ci-conformance.ts",
    sites: [
      {
        file: CONFORMANCE_ACTION,
        job: ACTION_JOB,
        step: "Install the auditor's dependencies",
      },
      { file: CONFORMANCE_ACTION, job: ACTION_JOB, step: "Run conformance" },
    ],
  },
  {
    module: "scripts/ci-release-run.ts",
    sites: [{ file: RELEASE_ACTION, job: ACTION_JOB, step: "Run" }],
  },
  {
    module: "scripts/ci-mutation-hooks.ts",
    sites: [
      {
        file: ".github/workflows/validate-mutation.yml",
        job: "mutation-full",
        step: "Mutate hooks/go packages",
      },
    ],
  },
  {
    module: "scripts/mutation-report.ts",
    sites: [
      {
        file: ".github/workflows/validate-mutation.yml",
        job: "mutation-summary",
        step: "Write the merged survivor report",
      },
    ],
  },
  {
    module: "scripts/cloud-setup.ts",
    sites: [
      {
        file: ".github/scripts/cloud-setup.sh",
        command: "node .github/tools/src/scripts/cloud-setup.ts",
      },
    ],
  },
  {
    module: "scripts/ci-areas.ts",
    sites: [
      {
        file: VALIDATE,
        job: "scope",
        step: "Classify immutable Git diff",
        via: "scripts/ci-scope.ts",
      },
      {
        file: VALIDATE,
        job: "scope",
        step: "Publish validated test selection",
        via: "scripts/ci-scope.ts",
      },
      {
        file: VALIDATE,
        job: "validation-summary",
        step: "Report expected and actual jobs",
        via: "scripts/ci-summary.ts",
      },
    ],
  },
  {
    module: "scripts/ci-last-green.ts",
    sites: [
      { file: VALIDATE, job: "scope", step: "Find the last validated commit" },
    ],
  },
  {
    module: "scripts/ci-scope.ts",
    sites: [
      { file: VALIDATE, job: "scope", step: "Classify immutable Git diff" },
      {
        file: VALIDATE,
        job: "scope",
        step: "Publish validated test selection",
      },
    ],
  },
  {
    module: "scripts/ci-summary.ts",
    sites: [
      {
        file: VALIDATE,
        job: "validation-summary",
        step: "Report expected and actual jobs",
      },
    ],
  },
  {
    // The workspace's own test launcher: it starts the suite that installs
    // nothing of its own, so it must load before any dependency resolves.
    module: "scripts/node-test.ts",
    sites: [
      {
        file: ".github/tools/package.json",
        command: "node src/scripts/node-test.ts",
      },
    ],
  },
];

const SRC = ".github/tools/src";
// Static `import … from`, `export … from`, bare `import "…"`, dynamic
// `import("…")`, and `require("…")` or `createRequire(…)("…")` with a literal.
// Type-only imports count too, so the rule needs no parser.
const SPECIFIER =
  /\bfrom\s*["']([^"']+)["']|\bimport\s*\(\s*["']([^"']+)["']\s*\)|^\s*import\s*["']([^"']+)["']|\brequire\s*\(\s*["']([^"']+)["']\s*\)|\bcreateRequire\s*\([^()]*\)\s*\(\s*["']([^"']+)["']\s*\)/gm;

interface Closure {
  files: string[];
  violations: string[];
}

// The entry's relative-import closure. A specifier that is neither `node:` nor
// relative is a violation naming the entry, the file and the specifier.
function importClosure(entry: string, read: (file: string) => string): Closure {
  const files: string[] = [];
  const violations: string[] = [];
  const pending = [entry];
  while (pending.length > 0) {
    const file = pending.pop()!;
    if (files.includes(file)) continue;
    files.push(file);
    for (const match of read(file).matchAll(SPECIFIER)) {
      const specifier = match.slice(1).find((group) => group !== undefined)!;
      if (specifier.startsWith("node:")) continue;
      if (specifier.startsWith("./") || specifier.startsWith("../")) {
        pending.push(
          path.posix.normalize(
            path.posix.join(path.posix.dirname(file), specifier),
          ),
        );
        continue;
      }
      violations.push(
        `${entry} reaches ${file}, which imports "${specifier}"; a module that runs before npm ci may import only node: built-ins and relative files`,
      );
    }
  }
  return { files: files.sort(), violations };
}

function readSource(file: string): string {
  return fs.readFileSync(path.join(root, SRC, file), "utf8");
}

interface ColdStep {
  name?: string;
  if?: string;
  run?: string;
  uses?: string;
}
type ColdDocs = Record<
  string,
  { jobs?: Record<string, { steps: ColdStep[] }>; runs?: { steps: ColdStep[] } }
>;

// Every hand-written workflow and composite action, so the completeness check
// below sees module steps that no entry lists yet.
function readColdDocs(): ColdDocs {
  const docs: ColdDocs = {};
  for (const [file, text] of workflowSources(root))
    docs[file] = YAML.parse(text);
  return docs;
}

// `npm ci` or a project `npm install`; installing npm itself globally fills no
// node_modules. A step that `uses:` the conformance action has no run: of its
// own and installs only inside the auditor checkout, so it never counts.
// Each command of a line is judged apart, so a global npm install chained
// before a project install does not hide the second.
const NPM_CLEAN_INSTALLS = new Set([
  "ci",
  "clean-install",
  "ic",
  "install-clean",
]);
const NPM_INSTALLS = new Set(["install", "i", "in", "add"]);
// Options whose value is the next word, so it is not read as the subcommand.
const NPM_VALUE_OPTIONS = new Set(["--prefix", "-C", "--workspace", "-w"]);

function installs(run: string | undefined): boolean {
  if (run === undefined) return false;
  return run.split(/&&|\|\||[;|\n]/).some((command) => {
    const words = command.trim().split(/\s+/);
    const npm = words.indexOf("npm");
    if (npm < 0) return false;
    let subcommand: string | undefined;
    for (let at = npm + 1; at < words.length; at++) {
      const word = words[at]!;
      if (!word.startsWith("-")) {
        subcommand = word;
        break;
      }
      if (NPM_VALUE_OPTIONS.has(word)) at++;
    }
    if (subcommand === undefined) return false;
    if (NPM_CLEAN_INSTALLS.has(subcommand)) return true;
    const global = words.some(
      (word) =>
        word === "-g" || word === "--global" || word === "--location=global",
    );
    return NPM_INSTALLS.has(subcommand) && !global;
  });
}

// The `matrix.<key> == '<value>'` and `runner.os == '<value>'` terms of an
// `if:` that is a pure conjunction, lower-cased because GitHub compares
// strings case-insensitively; undefined when it holds anything else.
function conjunction(
  condition: string | undefined,
): Map<string, string> | undefined {
  if (condition === undefined) return new Map();
  const text = condition.replace(/^\s*\$\{\{|\}\}\s*$/g, "");
  if (text.includes("||")) return undefined;
  const terms = new Map<string, string>();
  for (const part of text.split("&&")) {
    const term =
      /^\s*((?:matrix\.[\w-]+)|runner\.os)\s*==\s*'([^']*)'\s*$/.exec(part);
    if (term) terms.set(term[1]!.toLowerCase(), term[2]!.toLowerCase());
  }
  return terms;
}

// Two conditions that require different values of the same matrix key or
// runner OS never hold on one leg.
function exclusive(a: string | undefined, b: string | undefined): boolean {
  const left = conjunction(a);
  const right = conjunction(b);
  if (left === undefined || right === undefined) return false;
  for (const [key, value] of left)
    if (right.has(key) && right.get(key) !== value) return true;
  return false;
}

function assertColdSites(docs: ColdDocs, text: (file: string) => string): void {
  const problems: string[] = [];
  for (const { module, sites } of COLD_ENTRIES) {
    const command = `${SRC}/${module}`;
    for (const site of sites) {
      if (!("job" in site)) {
        const source = text(site.file);
        const at = source.indexOf(site.command);
        if (at < 0)
          problems.push(
            `${module}: ${site.file} no longer runs ${site.command}`,
          );
        else if (installs(source.slice(0, at)))
          problems.push(`${module} runs in ${site.file} after an npm install`);
        continue;
      }
      const where = `${site.file} job ${site.job} step "${site.step}"`;
      const doc = docs[site.file]!;
      const steps =
        site.job === ACTION_JOB ? doc.runs?.steps : doc.jobs?.[site.job]?.steps;
      const index = steps?.findIndex((step) => step.name === site.step) ?? -1;
      if (steps === undefined || index < 0) {
        problems.push(`${module}: ${where} is missing`);
        continue;
      }
      const step = steps[index]!;
      const started = site.via === undefined ? command : `${SRC}/${site.via}`;
      if (!(step.run ?? "").includes(started))
        problems.push(`${module}: ${where} no longer starts ${started}`);
      for (const earlier of steps.slice(0, index))
        if (installs(earlier.run) && !exclusive(earlier.if, step.if))
          problems.push(
            `${module} runs in ${where} after npm installs in step "${earlier.name ?? earlier.run}" on the same leg`,
          );
    }
  }
  if (problems.length > 0) throw new Error(problems.join("\n"));
}

// A step command that starts a workspace module, from the checkout or from the
// checkout variable a composite action uses for its own code.
const MODULE_RUN =
  /\bnode\s+"?(?:\$\{?\w+\}?\/)?\.github\/tools\/src\/([\w./-]+\.ts)\b/g;

// Derived, not kept by hand: every module a step starts with no install before
// it on its leg must be a cold-start entry listing that step.
function assertColdListComplete(docs: ColdDocs): void {
  const problems: string[] = [];
  for (const { file, job, steps } of jobSteps(new Map(Object.entries(docs))))
    steps.forEach((step, index) => {
      const warm = steps
        .slice(0, index)
        .some(
          (earlier) => installs(earlier.run) && !exclusive(earlier.if, step.if),
        );
      if (warm) return;
      for (const [, module] of (step.run ?? "").matchAll(MODULE_RUN)) {
        const listed = COLD_ENTRIES.some((entry) =>
          entry.sites.some(
            (site) =>
              "job" in site &&
              site.file === file &&
              site.job === job &&
              site.step === step.name &&
              (site.via ?? entry.module) === module,
          ),
        );
        if (!listed)
          problems.push(
            `${module} runs before npm ci in ${file} job ${job} step "${step.name ?? step.run}" but is not in the cold-start list`,
          );
      }
    });
  if (problems.length > 0) throw new Error(problems.join("\n"));
}

test("every module a step starts before npm ci is in the cold-start list", () => {
  assertColdListComplete(readColdDocs());
});

test("negative: an unlisted module step before npm ci is rejected, one after it is not", () => {
  const added = (job: string, before: string, step: ColdStep) => {
    const docs = readColdDocs();
    const steps = docs[VALIDATE]!.jobs![job]!.steps;
    steps.splice(
      steps.findIndex((s) => s.name === before),
      0,
      step,
    );
    return docs;
  };
  assert.throws(
    () =>
      assertColdListComplete(
        added("go-suite", "Test the hooks module", {
          name: "Run a new module",
          run: "node .github/tools/src/scripts/ci-new.ts check",
        }),
      ),
    /scripts\/ci-new\.ts runs before npm ci in \.github\/workflows\/validate\.yml job go-suite step "Run a new module" but is not in the cold-start list/,
  );
  const docs = readColdDocs();
  docs[CONFORMANCE_ACTION]!.runs!.steps.unshift({
    name: "Probe the auditor",
    run: 'node "$AGX_CONFORMANCE_AUDITOR/.github/tools/src/scripts/ci-probe.ts"',
  });
  assert.throws(
    () => assertColdListComplete(docs),
    /scripts\/ci-probe\.ts runs before npm ci in \.github\/actions\/conformance\/action\.yml job runs step "Probe the auditor"/,
    "an action step from its checkout variable counts",
  );
  assert.doesNotThrow(
    () =>
      assertColdListComplete(
        added("ts-suite", "Test the TypeScript workspace", {
          name: "Run a new module",
          run: "node .github/tools/src/scripts/ci-new.ts check",
        }),
      ),
    "a module after the job's npm ci needs no entry",
  );
});

function readRepoText(file: string): string {
  return fs.readFileSync(path.join(root, file), "utf8");
}

test("every cold-start module runs before npm ci on its leg", () => {
  assertColdSites(readColdDocs(), readRepoText);
});

test("negative: an npm install before a cold-start step is rejected unless its if: excludes the leg", () => {
  const insert = (condition: string, run: string) => {
    const docs = readColdDocs();
    const steps = docs[VALIDATE]!.jobs!["go-suite"]!.steps;
    const at = steps.findIndex((step) => step.name === "Test the hooks module");
    steps.splice(at, 0, { name: "Install the workspace", if: condition, run });
    return docs;
  };
  assert.throws(
    () =>
      assertColdSites(
        insert("matrix.check == 'tests'", "npm ci --ignore-scripts"),
        readRepoText,
      ),
    /scripts\/ci-hooks-test\.ts runs in \.github\/workflows\/validate\.yml job go-suite step "Test the hooks module" after npm installs in step "Install the workspace"/,
  );
  assert.throws(
    () =>
      assertColdSites(
        insert("matrix.os == 'ubuntu-latest'", "npm install --ignore-scripts"),
        readRepoText,
      ),
    /ci-hooks-test\.ts runs in .* after npm installs in step "Install the workspace"/,
    "a project npm install counts, and a condition on another key can hold on the same leg",
  );
  assert.doesNotThrow(
    () =>
      assertColdSites(
        insert("matrix.check == 'race'", "npm ci --ignore-scripts"),
        readRepoText,
      ),
    "an install only on another leg does not count",
  );
  assert.doesNotThrow(
    () =>
      assertColdSites(
        insert(
          "matrix.check == 'tests'",
          "npm install --global --ignore-scripts npm@12.2.1",
        ),
        readRepoText,
      ),
    "installing npm itself does not count",
  );
  for (const [name, condition, run] of [
    [
      "an install through --prefix",
      "matrix.check == 'tests'",
      "npm --prefix .github/release ci --ignore-scripts",
    ],
    ["npm clean-install", "matrix.check == 'tests'", "npm clean-install"],
    [
      "a line that installs npm globally and then the project",
      "matrix.check == 'tests'",
      "npm install --global npm@12.2.1 && npm install --ignore-scripts",
    ],
    [
      "an || condition, which is never exclusive",
      "matrix.check == 'race' || matrix.os == 'windows-latest'",
      "npm ci --ignore-scripts",
    ],
    [
      "a value that differs only in case, since GitHub compares case-insensitively",
      "matrix.check == 'TESTS'",
      "npm ci --ignore-scripts",
    ],
  ] as const)
    assert.throws(
      () => assertColdSites(insert(condition, run), readRepoText),
      /ci-hooks-test\.ts runs in .* after npm installs in step "Install the workspace"/,
      name,
    );

  // The stryker-only npm ci passes only because its if: excludes the gremlins leg.
  const docs = readColdDocs();
  const install = docs[".github/workflows/validate-mutation.yml"]!.jobs![
    "mutation-full"
  ]!.steps.find(
    (step) => step.name === "Install the pinned Stryker toolchain",
  )!;
  assert.equal(install.if, "matrix.tool == 'stryker'");
  delete install.if;
  assert.throws(
    () => assertColdSites(docs, readRepoText),
    /scripts\/ci-mutation-hooks\.ts runs in .* step "Mutate hooks\/go packages" after npm installs in step "Install the pinned Stryker toolchain"/,
  );

  assert.throws(
    () =>
      assertColdSites(readColdDocs(), (file) =>
        file === ".github/scripts/cloud-setup.sh"
          ? "npm ci\n" + readRepoText(file)
          : readRepoText(file),
      ),
    /scripts\/cloud-setup\.ts runs in \.github\/scripts\/cloud-setup\.sh after an npm install/,
  );
});

test("every cold-start module's import closure is built-ins and relative files", () => {
  for (const { module } of COLD_ENTRIES)
    assert.deepEqual(importClosure(module, readSource).violations, [], module);
  assert.deepEqual(importClosure("scripts/cloud-setup.ts", readSource).files, [
    "plugin/common.ts",
    "scripts/cloud-setup-steps.ts",
    "scripts/cloud-setup.ts",
    "scripts/node-range.ts",
  ]);
  assert.deepEqual(
    importClosure("scripts/ci-conformance.ts", readSource).files,
    ["plugin/common.ts", "scripts/ci-conformance.ts"],
  );
  // mutation-report.ts loads mutation-config.ts through createRequire.
  assert.deepEqual(
    importClosure("scripts/mutation-report.ts", readSource).files,
    [
      "scripts/mutation-artifacts.ts",
      "scripts/mutation-config-values.ts",
      "scripts/mutation-config.ts",
      "scripts/mutation-report.ts",
      "scripts/mutation-source.ts",
    ],
  );
});

test("negative: a package import in a shared file is rejected naming each entry that reaches it", () => {
  const read = (file: string) =>
    file === "plugin/common.ts"
      ? 'import YAML from "yaml";\n' + readSource(file)
      : readSource(file);
  const reaching = COLD_ENTRIES.filter(({ module }) =>
    importClosure(module, readSource).files.includes("plugin/common.ts"),
  ).map(({ module }) => module);
  assert.ok(reaching.includes("scripts/cloud-setup.ts"));
  assert.ok(reaching.includes("scripts/ci-conformance.ts"));
  for (const { module } of COLD_ENTRIES) {
    const { violations } = importClosure(module, read);
    if (reaching.includes(module))
      assert.deepEqual(
        violations,
        [
          `${module} reaches plugin/common.ts, which imports "yaml"; a module that runs before npm ci may import only node: built-ins and relative files`,
        ],
        module,
      );
    else assert.deepEqual(violations, [], module);
  }
  const converter = "coverage/cli.ts";
  for (const [form, line] of [
    ["dynamic import", 'const later = await import("yaml");\n'],
    ["require", 'const later = require("yaml");\n'],
    ["createRequire", 'createRequire(import.meta.url)("yaml");\n'],
  ] as const)
    assert.deepEqual(
      importClosure(
        converter,
        (file) => (file === converter ? line : "") + readSource(file),
      ).violations,
      [
        `${converter} reaches ${converter}, which imports "yaml"; a module that runs before npm ci may import only node: built-ins and relative files`,
      ],
      `a ${form} call counts`,
    );
});

test("early CI helpers and the test launcher load without installed dependencies", (t) => {
  const cold = fs.mkdtempSync(
    path.join(os.tmpdir(), "agentics-cold-entrypoints-"),
  );
  t.after(() => fs.rmSync(cold, { recursive: true, force: true }));
  const copied = new Set<string>();
  for (const { module } of COLD_ENTRIES)
    for (const file of importClosure(module, readSource).files)
      copied.add(file);
  for (const file of copied) {
    fs.mkdirSync(path.dirname(path.join(cold, SRC, file)), { recursive: true });
    fs.copyFileSync(path.join(root, SRC, file), path.join(cold, SRC, file));
  }
  fs.writeFileSync(
    path.join(cold, ".github/tools/package.json"),
    '{"type":"module"}',
  );
  const scripts = path.join(cold, SRC, "scripts");
  // A tripwire gh first on PATH, and no token: nothing here may reach GitHub.
  const fakeBin = path.join(cold, "fake-bin");
  fs.mkdirSync(fakeBin);
  const ghMarker = path.join(cold, "gh-called");
  fs.writeFileSync(
    path.join(fakeBin, "gh"),
    `#!/bin/sh\necho called > '${ghMarker}'\nexit 97\n`,
    { mode: 0o755 },
  );
  fs.writeFileSync(
    path.join(fakeBin, "gh.cmd"),
    `@echo called> "${ghMarker}"\r\n@exit /b 97\r\n`,
  );
  fs.writeFileSync(path.join(cold, "event.json"), "{}");
  fs.writeFileSync(
    path.join(cold, "fixture.test.mjs"),
    "import test from 'node:test'; test('cold fixture', () => {});\n",
  );
  const env: NodeJS.ProcessEnv = {
    ...process.env,
    PATH:
      fakeBin + path.delimiter + (process.env.PATH ?? process.env.Path ?? ""),
    GITHUB_REPOSITORY: "",
    GITHUB_REF_NAME: "",
    GITHUB_EVENT_NAME: "workflow_dispatch",
    GITHUB_OUTPUT: "",
    CASE_EVENT: "unsupported",
  };
  delete env.Path;
  for (const name of [
    "NODE_TEST_CONTEXT",
    "GH_TOKEN",
    "GITHUB_TOKEN",
    "AGX_TOOLS_DIR",
    "AGX_BIN_DIR",
    "AGX_ORIGINAL_PATH",
  ])
    delete env[name];

  // Importing loads the whole static closure without running a gated CLI.
  for (const { module } of COLD_ENTRIES) {
    if (module === "scripts/cloud-setup.ts") continue;
    const loaded = spawnSync(
      process.execPath,
      [
        "--input-type=module",
        "-e",
        "await import(process.argv[1]);",
        pathToFileURL(path.join(cold, SRC, module)).href,
      ],
      { cwd: cold, env, encoding: "utf8", timeout: 10_000 },
    );
    assert.ifError(loaded.error);
    assert.equal(loaded.status, 0, `${module}: ${loaded.stderr}`);
  }
  const run = (
    name: string,
    args: string[] = [],
    extraEnv: NodeJS.ProcessEnv = {},
  ) =>
    spawnSync(process.execPath, [path.join(scripts, name), ...args], {
      cwd: cold,
      env: { ...env, ...extraEnv },
      encoding: "utf8",
      timeout: 10_000,
    });
  // cloud-setup.ts has no CLI gate; an unknown option stops it before any step.
  const setup = run("cloud-setup.ts", ["--cold-start-probe"]);
  assert.ifError(setup.error);
  assert.notEqual(setup.status, 0);
  assert.match(setup.stderr, /unknown option: --cold-start-probe/);
  assert.doesNotMatch(setup.stderr, /ERR_MODULE_NOT_FOUND/);
  const lookup = run("ci-last-green.ts");
  assert.ifError(lookup.error);
  assert.equal(lookup.status, 0, lookup.stderr);
  assert.equal(lookup.stdout, "sha=\n");
  const rawOutput = path.join(cold, "classify.out");
  const scope = run(
    "ci-scope.ts",
    ["--repo", cold, "--event", path.join(cold, "event.json")],
    { GITHUB_OUTPUT: rawOutput },
  );
  assert.ifError(scope.error);
  assert.equal(scope.status, 0, scope.stderr);
  const selection: unknown = JSON.parse(scope.stdout);
  assert.ok(selection && typeof selection === "object");
  assert.equal(Reflect.get(selection, "reason"), "event-full");
  const scheduledOutput = path.join(cold, "schedule.out");
  const schedule = run("ci-scope.ts", ["--schedule"], {
    AGX_CI_CLASSIFY_OUTCOME: "success",
    AGX_CI_CLASSIFY_OUTPUTS: JSON.stringify(
      Object.fromEntries(
        fs
          .readFileSync(rawOutput, "utf8")
          .trimEnd()
          .split("\n")
          .map((line) => line.split("=")),
      ),
    ),
    GITHUB_OUTPUT: scheduledOutput,
  });
  assert.ifError(schedule.error);
  assert.equal(schedule.status, 0, schedule.stderr);
  assert.equal(
    fs.readFileSync(scheduledOutput, "utf8"),
    'effective_selection=full|false||||event-full||["hooks"]|["ts-tools"]|true\n',
  );
  const summary = run("ci-summary.ts", [
    "--needs-env",
    "CASE_NEEDS",
    "--event-name-env",
    "CASE_EVENT",
    "--summary-env",
    "CASE_SUMMARY",
  ]);
  assert.ifError(summary.error);
  assert.equal(summary.status, 1);
  assert.equal(summary.stderr, "ci-summary: event-input\n");
  const launcher = run("node-test.ts", [path.join(cold, "fixture.test.mjs")]);
  assert.ifError(launcher.error);
  assert.equal(launcher.status, 0, launcher.stdout + launcher.stderr);
  assert.equal(fs.existsSync(path.join(cold, "node_modules")), false);
  assert.equal(fs.existsSync(ghMarker), false, "nothing may start gh");
});
