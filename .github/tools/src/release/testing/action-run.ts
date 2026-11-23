// Runs the real YAML steps of the release action in bash, against shims, and reports what happened. Each scenario
// returns its observations and a checker turns them into violations, so a mutation test can run a broken step and
// require a violation.

import {
  chmodSync,
  cpSync,
  existsSync,
  mkdirSync,
  readFileSync,
  statSync,
  writeFileSync,
} from "node:fs";
import { delimiter, join } from "node:path";
import { fixtureGitEnv } from "../../conformance/testing/git-checkout.ts";
import {
  readWorkflowArgv,
  recordWorkflowArgv,
  runWorkflowStep,
  type CheckedWorkflowStep,
} from "../../scripts/testing/workflow-steps.ts";
import { repoRoot } from "./repo.ts";
import { commitFiles, initRepo, manifestFiles, scratchDir } from "./support.ts";

function runActionStep(
  checked: CheckedWorkflowStep,
  env: Readonly<Record<string, string | undefined>>,
  cwd = process.cwd(),
) {
  const result = runWorkflowStep(checked, { env, cwd });
  return { status: result.status, output: result.stdout + result.stderr };
}

export function writeExecutable(path: string, body: string): void {
  mkdirSync(join(path, ".."), { recursive: true });
  writeFileSync(path, body);
  chmodSync(path, 0o755);
}

// The module the Run step starts, as the action runs it from the checkout. A mutation test passes a patched copy of its
// source in place of the real one.
export const releaseRunPath = ".github/tools/src/scripts/ci-release-run.ts";
export const releaseRunSource = readFileSync(
  join(repoRoot, releaseRunPath),
  "utf8",
);

// Writes the module into a fixture checkout, where the one-line Run step finds it through $CHECKOUT.
export function installReleaseRun(
  checkout: string,
  source: string = releaseRunSource,
): void {
  const path = join(checkout, releaseRunPath);
  mkdirSync(join(path, ".."), { recursive: true });
  writeFileSync(path, source);
}

// The Node that runs these tests, captured at load so a shim named `node` first on PATH can still reach it.
const realNode = process.execPath;

// A stand-in for the command the Run step starts. `body` sees the command's own arguments as "$@". The step runs
// `node <ci-release-run.ts>`, which starts `node <cli.ts> <args>` from PATH, so the shim is a `node` first on PATH: it
// runs the module on the real Node, and for the command it drops the script path before it runs `body`.
export function installCliShim(
  root: string,
  body: string,
): { env: Record<string, string> } {
  writeExecutable(
    join(root, "bin", "node"),
    `#!/bin/bash\ncase "\${1-}" in *ci-release-run.ts) exec ${JSON.stringify(realNode)} "$@";; esac\nshift\n${body}\n`,
  );
  return {
    env: {
      PATH: `${join(root, "bin")}${delimiter}${process.env["PATH"] ?? ""}`,
    },
  };
}

// The arguments the Run step passes for one command.
export function expectedArgv(command: string, prerelease: string): string[] {
  const want = [command];
  switch (command) {
    case "bundle":
      want.push("--repo", "checkout");
      break;
    case "context":
    case "resume-check":
    case "check-commit":
      want.push("--checkout", "checkout");
      break;
    case "stage":
      want.push("--prerelease", prerelease);
      break;
  }
  return want;
}

// Runs the Run step with every input empty except the command, the checkout and the publication mode, and returns the
// arguments the command received. `module` is the source of the module the step starts.
export function runArgv(
  run: CheckedWorkflowStep,
  command: string,
  prerelease: string,
  module: string = releaseRunSource,
): string[] {
  const root = scratchDir("agentics-action-argv-");
  installReleaseRun(join(root, "checkout"), module);
  const argvFile = join(root, "argv");
  const shim = installCliShim(root, recordWorkflowArgv);
  const env: Record<string, string | undefined> = {
    ...process.env,
    RUNNER_TEMP: root,
    ACTION_ARGV: argvFile,
  };
  for (const key of [
    "COMMAND",
    "CHECKOUT",
    "REPO",
    "SHA",
    "TAG",
    "MAIN_REF",
    "ASSETS",
    "NOTES",
    "BASELINE",
    "OUT",
    "PUBLISHED",
    "PARENT",
    "AUTHOR",
    "PRERELEASE",
  ]) {
    env[key] = "";
  }
  Object.assign(
    env,
    { COMMAND: command, CHECKOUT: "checkout", PRERELEASE: prerelease },
    shim.env,
  );
  const result = runActionStep(run, env, root);
  if (result.status !== 0)
    throw new Error(`action shell: exit ${result.status}\n${result.output}`);
  const argumentsSeen = readWorkflowArgv(argvFile);
  if (!argumentsSeen.length)
    throw new Error('argument recorder emitted no terminated arguments: ""');
  return argumentsSeen;
}

export interface TrustedOutcome {
  readonly status: number | null;
  readonly output: string;
  readonly forgedRan: boolean;
  readonly checksumsExist: boolean;
  readonly buildLog: string;
  readonly trusted: string;
}

/**
 * Builds a fixture repository whose candidate checkout holds a forged verifier, runs the Build step and the Run step
 * with the trusted checkout elsewhere, and records what ran. The trusted checkout holds the release code and the Run
 * step's module, so both load from $CHECKOUT and neither from the workspace.
 */
export function runTrustedVerifierScenario(
  build: CheckedWorkflowStep,
  run: CheckedWorkflowStep,
  mode: "trusted" | "missing" | "incompatible",
  module: string = releaseRunSource,
): TrustedOutcome {
  const root = scratchDir("agentics-action-trusted-");
  const [trusted, candidate, runner, bin, outdir] = [
    "trusted",
    "candidate",
    "runner",
    "bin",
    "out",
  ].map((name) => join(root, name)) as [string, string, string, string, string];
  for (const dir of [trusted, candidate, runner, bin, outdir])
    mkdirSync(dir, { recursive: true });
  initRepo(candidate);
  const sha = commitFiles(
    candidate,
    {
      ...manifestFiles("1.2.3"),
      "hooks/go/go.mod": "module fixture\n",
      "hooks/SHA256SUMS": "fixture\n",
      "bin/agx.exe": "fixture launcher\n",
      "hooks/go/cmd/hooktool/main.go": "forged candidate",
      hooktool:
        '#!/bin/sh\ntouch "$FORGED_MARKER"\necho forged-candidate >&2\nexit 23\n',
    },
    "fixture",
  );
  chmodSync(join(candidate, "hooktool"), 0o755);

  cpSync(
    join(repoRoot, ".github", "tools", "src", "release"),
    join(trusted, ".github", "tools", "src", "release"),
    { recursive: true },
  );
  cpSync(
    join(repoRoot, ".github/tools/src/plugin-manifests.ts"),
    join(trusted, ".github/tools/src/plugin-manifests.ts"),
  );
  installReleaseRun(trusted, module);
  writeExecutable(
    join(bin, "go"),
    `#!/bin/bash
set -eu
module=; target=; output=
while [ "$#" -gt 0 ]; do case "$1" in -C) shift; module=$1;; -o) shift; output=$1;; ./cmd/*) target=$1;; esac; shift; done
printf '%s %s\\n' "$module" "$target" >> "$BUILD_LOG"
case "$target" in
 ./cmd/hooktool) [ "$module" = "$CHECKOUT/hooks/go" ]; case "$BUILD_MODE" in missing) exit 0;; trusted) msg=trusted-verifier-called;; incompatible) msg=incompatible-verifier;; esac; printf '#!/bin/sh\\necho %s >&2\\nexit 23\\n' "$msg" > "$output"; chmod +x "$output";;
 *) exit 9;;
esac
`,
  );
  const env = {
    ...fixtureGitEnv(),
    PATH: `${bin}${delimiter}${process.env["PATH"] ?? ""}`,
    CHECKOUT: trusted,
    REPO: candidate,
    RUNNER_TEMP: runner,
    COMMAND: "bundle",
    SHA: sha,
    TAG: "v1.2.3",
    MAIN_REF: "HEAD",
    OUT: outdir,
    BUILD_MODE: mode,
    BUILD_LOG: join(root, "build.log"),
    FORGED_MARKER: join(root, "forged"),
    AGX_HOOK_VERIFIER: join(candidate, "hooktool"),
  };
  const built = runActionStep(build, env);
  if (built.status !== 0)
    throw new Error(`build step: exit ${built.status}\n${built.output}`);
  const ran = runActionStep(run, env);
  return {
    status: ran.status,
    output: ran.output,
    forgedRan: existsSync(join(root, "forged")),
    checksumsExist: existsSync(join(outdir, "SHA256SUMS")),
    buildLog: existsSync(join(root, "build.log"))
      ? readFileSync(join(root, "build.log"), "utf8")
      : "",
    trusted,
  };
}

export function trustedVerifierViolations(
  outcome: TrustedOutcome,
  want: string,
): string[] {
  const violations: string[] = [];
  if (outcome.status === 0 || !outcome.output.includes(want)) {
    violations.push(
      `bundle must refuse with ${JSON.stringify(want)}: exit ${outcome.status}: ${outcome.output}`,
    );
  }
  if (outcome.forgedRan) violations.push("candidate verifier ran");
  if (outcome.checksumsExist) violations.push("checksum asset exists");
  if (
    !outcome.buildLog.includes(
      `${join(outcome.trusted, "hooks/go")} ./cmd/hooktool`,
    )
  ) {
    violations.push(`trusted build missing: ${outcome.buildLog}`);
  }
  return violations;
}

/**
 * Runs the Build and Run steps for a non-bundle command and returns the violations of its contract, empty when there
 * are none: it builds no hook verifier, passes only its own flags, and leaves the caller's AGX_HOOK_VERIFIER alone.
 */
export function runNonBundleScenario(
  build: CheckedWorkflowStep,
  run: CheckedWorkflowStep,
  module: string = releaseRunSource,
): string[] {
  const buildEnv = build.step.env ?? {};
  const runEnv = run.step.env ?? {};
  if (
    buildEnv["COMMAND"] !== "${{ inputs.command }}" ||
    buildEnv["CHECKOUT"] !== "${{ inputs.checkout }}" ||
    runEnv["REPO"] !== "${{ inputs.repo }}" ||
    runEnv["CHECKOUT"] !== "${{ inputs.checkout }}"
  ) {
    return ["action input environment mapping changed"];
  }
  const root = scratchDir("agentics-action-nonbundle-");
  const bin = join(root, "bin");
  const runner = join(root, "runner");
  mkdirSync(bin, { recursive: true });
  mkdirSync(runner, { recursive: true });
  const body =
    'printf \'%s\\n\' "$@" > "$ARGS_LOG"\nprintf \'%s\' "${AGX_HOOK_VERIFIER-unset}" > "$ENV_LOG"';
  writeExecutable(
    join(bin, "go"),
    `#!/bin/bash
set -eu
case "$*" in *./cmd/hooktool*) echo unexpected-hooktool >&2; exit 42;; esac
`,
  );
  installCliShim(root, body);
  installReleaseRun(join(root, "trusted"), module);
  const env = {
    ...process.env,
    PATH: `${bin}${delimiter}${process.env["PATH"] ?? ""}`,
    RUNNER_TEMP: runner,
    CHECKOUT: join(root, "trusted"),
    REPO: "owner/repo",
    COMMAND: "context",
    MAIN_REF: "refs/remotes/origin/main",
    ARGS_LOG: join(root, "args"),
    ENV_LOG: join(root, "env"),
    AGX_HOOK_VERIFIER: "original",
  };
  for (const script of [build, run]) {
    const result = runActionStep(script, env);
    if (result.status !== 0)
      return [`step: exit ${result.status}: ${result.output}`];
  }
  const violations: string[] = [];
  const args = existsSync(join(root, "args"))
    ? readFileSync(join(root, "args"), "utf8")
    : undefined;
  const wantArgs = `context\n--repo\nowner/repo\n--checkout\n${join(root, "trusted")}\n`;
  if (args !== wantArgs)
    violations.push(`non-bundle args: ${JSON.stringify(args)}`);
  const value = existsSync(join(root, "env"))
    ? readFileSync(join(root, "env"), "utf8")
    : undefined;
  if (value !== "original")
    violations.push(`non-bundle verifier env: ${JSON.stringify(value)}`);
  return violations;
}

// Runs the Build step as written against a strict compiler stand-in, checking hooktool output without requiring a
// host Go installation.
export function runRealBuildScenario(build: CheckedWorkflowStep): string[] {
  const root = scratchDir("agentics-action-build-");
  const runner = join(root, "runner");
  const bin = join(root, "bin");
  mkdirSync(runner);
  writeExecutable(
    join(bin, "go"),
    `#!/bin/bash
set -eu
if [ "$#" -ne 7 ] || [ "$1" != build ] || [ "$2" != -C ] ||
   [ "$3" != "$CHECKOUT/hooks/go" ] || [ "$4" != -trimpath ] ||
   [ "$5" != -o ] || [ "$6" != "$RUNNER_TEMP/hooktool" ] ||
   [ "$7" != ./cmd/hooktool ]; then
  echo "fake go: unexpected build arguments: $*" >&2
  exit 42
fi
printf '#!/bin/sh\\nexit 0\\n' > "$6"
chmod +x "$6"
`,
  );
  const result = runActionStep(build, {
    ...process.env,
    PATH: `${bin}${delimiter}${process.env.PATH ?? ""}`,
    CHECKOUT: repoRoot,
    COMMAND: "bundle",
    RUNNER_TEMP: runner,
  });
  if (result.status !== 0)
    return [`actual action build: exit ${result.status}: ${result.output}`];
  const violations: string[] = [];
  for (const name of ["release", "hooktool"]) {
    const path = join(runner, name);
    const expected = name === "hooktool";
    if (
      expected &&
      (!existsSync(path) || (statSync(path).mode & 0o111) === 0)
    ) {
      violations.push(`built ${name} is not executable`);
    }
    if (!expected && existsSync(path))
      violations.push(
        `the build left ${name} behind, but the action builds only hooktool`,
      );
  }
  return violations;
}
