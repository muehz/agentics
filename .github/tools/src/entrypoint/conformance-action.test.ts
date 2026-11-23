import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import {
  copyFileSync,
  linkSync,
  lstatSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  symlinkSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { after, test, type TestContext } from "node:test";
import { pathToFileURL } from "node:url";
import { parse } from "yaml";
import { runValidate, type Runner } from "../plugin/validate.ts";
import {
  disposeSuite,
  getSuite,
  loadConformanceAction,
  writeConformanceLauncher,
  writeLauncher,
} from "./testing/harness.ts";
import {
  ACTION_JOB,
  lookupWorkflowStep,
  runWorkflowStep,
  workflowShellSkip,
  withRunBody,
  type CheckedWorkflowStep,
  type WorkflowStepData as Step,
} from "../scripts/testing/workflow-steps.ts";

after(disposeSuite);

const action = loadConformanceAction();

test("the built TypeScript checker runs with an empty PATH", (t) => {
  const checker = buildChecker(t),
    repo = temporary(t);
  write(join(repo, ".claude-plugin/plugin.json"), "{}", 0o600);
  const r = spawnSync(
    process.execPath,
    [checker, "not-a-domain", "--repo", repo],
    {
      encoding: "utf8",
      env: { PATH: "" },
    },
  );
  assert.ifError(r.error);
  assert.equal(r.status, 1, r.stdout + r.stderr);
  assert.ok(r.stderr.includes("conformance: unknown domain"), r.stderr);
});
function temporary(t: TestContext): string {
  const dir = mkdtempSync(join(tmpdir(), "agentics-action-"));
  t.after(() => rmSync(dir, { recursive: true, force: true }));
  return dir;
}
function write(path: string, body: string, mode = 0o755): void {
  mkdirSync(dirname(path), { recursive: true });
  writeFileSync(path, body, { mode });
}
function stepNamed(name: string) {
  return lookupWorkflowStep(
    action,
    ".github/actions/conformance/action.yml",
    ACTION_JOB,
    name,
  );
}
function shellOptions(names: string[]) {
  const reasons = names.map((name) => workflowShellSkip(stepNamed(name)));
  return { skip: reasons.find((reason) => reason !== false) ?? false };
}
// The host environment with `overrides` applied. Windows variable names are case-insensitive, so an override drops
// every other spelling of its name (PATH and Path).
function hostEnv(overrides: NodeJS.ProcessEnv = {}): NodeJS.ProcessEnv {
  const names = new Set(Object.keys(overrides).map((k) => k.toUpperCase()));
  const env: NodeJS.ProcessEnv = {};
  for (const [key, value] of Object.entries(process.env))
    if (!names.has(key.toUpperCase())) env[key] = value;
  return { ...env, ...overrides };
}
function execute(
  step: CheckedWorkflowStep,
  env: NodeJS.ProcessEnv,
  cwd = process.cwd(),
) {
  const result = runWorkflowStep(step, {
    cwd,
    env: hostEnv(env),
    timeout: 30_000,
    maxBuffer: 16 * 1024 * 1024,
  });
  return { exit: result.status, output: result.stdout + result.stderr };
}
function buildChecker(t: TestContext): string {
  const checker = join(temporary(t), "real-conformance.cjs");
  writeConformanceLauncher(checker);
  return checker;
}
function absent(path: string): boolean {
  return lstatSync(path, { throwIfNoEntry: false }) === undefined;
}

const auditorCli = ".github/tools/src/conformance/cli.ts";
const auditorModule =
  '"$AGX_CONFORMANCE_AUDITOR/.github/tools/src/scripts/ci-conformance.ts"';
const stepNames = [
  "Set up Node",
  "Record the Node setup",
  "Set up Go",
  "Record the restored cache",
  "Check the auditor",
  "Install the auditor's dependencies",
  "Run conformance",
];
// The launcher holds no TypeScript syntax, so node runs it as the auditor's
// cli.ts and it starts the real suite.
function writeAuditorLauncher(auditor: string): void {
  const cli = join(auditor, auditorCli);
  mkdirSync(dirname(cli), { recursive: true });
  writeConformanceLauncher(cli);
}

test("conformance action cache contract", () => {
  const node = stepNamed("Set up Node").step,
    setup = stepNamed("Set up Go").step;
  assert.deepEqual(Object.keys(action.inputs), [
    "repo",
    "auditor",
    "domain",
    "allow-stale-review",
    "cache",
    "skip-module-tests",
  ]);
  assert.equal(action.inputs.repo.default, "${{ github.workspace }}");
  assert.equal(action.inputs.auditor.default, "${{ github.workspace }}");
  assert.equal(action.inputs.cache.default, "true");
  assert.equal(action.inputs["allow-stale-review"].default, "false");
  assert.equal(action.inputs["skip-module-tests"].default, "false");
  assert.equal(
    action.outputs["stale-review"].value,
    "${{ steps.conformance.outputs.stale-review }}",
  );
  assert.deepEqual(
    action.runs.steps.map((s: Step) => s.name),
    stepNames,
  );
  assert.equal(
    node.uses,
    "actions/setup-node@949feb2413d6458794dcd2491c4babbbce0c15c1",
  );
  assert.equal(node.with?.["node-version"], "26.12.1");
  assert.equal(node.if, "env.AGX_NODE_SETUP != '26.12.1'");
  assert.equal(
    setup.uses,
    "actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e",
  );
  assert.equal(
    setup.with?.["go-version-file"],
    "${{ inputs.auditor }}/hooks/go/go.mod",
  );
  assert.equal(setup.with?.cache, "${{ inputs.cache }}");
  assert.equal(
    String(setup.with?.["cache-dependency-path"]).trim(),
    [
      "${{ inputs.repo }}/hooks/go/go.mod",
      "${{ inputs.auditor }}/hooks/go/go.mod",
    ].join("\n"),
  );
  assert.equal(
    setup.if,
    "env.AGX_GO_CACHE_RESTORED != format('{0}/hooks/go/go.mod', inputs.auditor)",
  );
});

// Each record step must write exactly the value its setup step's `if:`
// compares against, or a second call in the job would set up again.
test(
  "conformance action record steps",
  shellOptions(["Record the Node setup", "Record the restored cache"]),
  (t) => {
    const setup = stepNamed("Set up Go").step,
      dir = temporary(t),
      auditor = join(dir, "auditor"),
      githubEnv = join(dir, "github-env");
    write(githubEnv, "", 0o600);
    const nodeRecord = stepNamed("Record the Node setup");
    assert.equal(nodeRecord.step.env, undefined);
    let r = execute(nodeRecord, { GITHUB_ENV: githubEnv });
    assert.equal(r.exit, 0, r.output);
    const goRecord = stepNamed("Record the restored cache"),
      env: NodeJS.ProcessEnv = { GITHUB_ENV: githubEnv };
    for (const [key, expr] of Object.entries(goRecord.step.env!)) {
      assert.ok(expr.includes("${{ inputs.auditor }}"), expr);
      env[key] = expr.replaceAll("${{ inputs.auditor }}", auditor);
    }
    r = execute(goRecord, env);
    assert.equal(r.exit, 0, r.output);
    const format = /format\('([^']*)', inputs\.auditor\)/.exec(setup.if!);
    assert.ok(format, setup.if ?? "no if");
    assert.equal(
      readFileSync(githubEnv, "utf8"),
      `AGX_NODE_SETUP=26.12.1\nAGX_GO_CACHE_RESTORED=${format[1]!.replace("{0}", auditor)}\n`,
    );
  },
);

test("the conformance fixture takes pipefail from the selected action step's shell", (t) => {
  const record = stepNamed("Record the Node setup");
  const bash = withRunBody(record, "false | true\n" + record.step.run);
  const sh = { ...bash, step: { ...bash.step, shell: "sh" } };
  const unavailable = [bash, sh].map((s) => workflowShellSkip(s)).find(Boolean);
  if (unavailable) return t.skip(unavailable);
  const output = join(temporary(t), "github-env");
  assert.deepEqual(execute(bash, { GITHUB_ENV: output }), {
    exit: 1,
    output: "",
  });
  assert.ok(absent(output));
  assert.deepEqual(execute(sh, { GITHUB_ENV: output }), {
    exit: 0,
    output: "",
  });
  assert.equal(readFileSync(output, "utf8"), "AGX_NODE_SETUP=26.12.1\n");
});

test("reminder uses typed stale output", () => {
  const workflow = parse(
    readFileSync(
      join(getSuite().repoRoot, ".github/workflows/reminders.yml"),
      "utf8",
    ),
  );
  let sawCheck = false,
    sawIssue = false;
  for (const step of workflow.jobs["check-ledger-staleness"].steps) {
    if (step.id === "staleness") {
      sawCheck = true;
      assert.ok(!step["continue-on-error"]);
      assert.equal(step.with["allow-stale-review"], "true");
    }
    if (step.if) {
      sawIssue = true;
      assert.equal(step.if, "steps.staleness.outputs.stale-review == 'true'");
    }
  }
  assert.ok(sawCheck && sawIssue, "reminder check or issue gate missing");
});

test("conformance action literal inputs", () => {
  const steps = action.runs.steps.filter((s: Step) => s.run);
  assert.deepEqual(
    steps.map((s: Step) => s.name),
    [
      "Record the Node setup",
      "Record the restored cache",
      "Check the auditor",
      "Install the auditor's dependencies",
      "Run conformance",
    ],
  );
  for (const step of steps) {
    assert.equal(step.shell, "bash");
    assert.ok(
      !step.run!.includes("${{ inputs."),
      `${step.name} interpolates input in shell source`,
    );
  }
  // The auditor check guards the module's own checkout, so it is the only step whose logic stays in the shell.
  assert.deepEqual(
    steps
      .filter((s: Step) => s.run!.trim().includes("\n"))
      .map((s: Step) => s.name),
    ["Check the auditor"],
  );
  const check = stepNamed("Check the auditor").step,
    install = stepNamed("Install the auditor's dependencies").step,
    run = stepNamed("Run conformance").step;
  assert.equal(run.id, "conformance");
  assert.equal(
    install.if,
    undefined,
    "the install gate must live in the module",
  );
  assert.deepEqual(check.env, {
    AGX_CONFORMANCE_AUDITOR: "${{ inputs.auditor }}",
  });
  assert.deepEqual(install.env, {
    AGX_CONFORMANCE_AUDITOR: "${{ inputs.auditor }}",
    AGX_CONFORMANCE_DOMAIN: "${{ inputs.domain }}",
  });
  assert.deepEqual(run.env, {
    AGX_CONFORMANCE_DOMAIN: "${{ inputs.domain }}",
    AGX_CONFORMANCE_REPO: "${{ inputs.repo }}",
    AGX_CONFORMANCE_AUDITOR: "${{ inputs.auditor }}",
    AGX_ALLOW_STALE_REVIEW: "${{ inputs.allow-stale-review }}",
    AGX_SKIP_MODULE_TESTS: "${{ inputs.skip-module-tests }}",
  });
  // The module loads from the trusted auditor, never from the audited workspace.
  assert.equal(install.run!.trim(), `node ${auditorModule} install`);
  assert.equal(run.run!.trim(), `node ${auditorModule} run`);
  for (const step of steps)
    for (const match of step.run!.matchAll(/"([^"]*)ci-conformance\.ts"/g))
      assert.ok(match[1]!.startsWith("$AGX_CONFORMANCE_AUDITOR/"), step.name);
});

// The auditor check keeps its message and exit 1, and it is the first step that touches the auditor: a run stops
// there, so neither the module nor go ever starts.
test(
  "conformance action refuses an auditor without the CLI",
  shellOptions([
    "Check the auditor",
    "Install the auditor's dependencies",
    "Run conformance",
  ]),
  async (t) => {
    const all = action.runs.steps as Step[];
    const sequence = all
      .slice(all.findIndex((s) => s.name === "Check the auditor"))
      .map((step) => stepNamed(step.name!));
    assert.deepEqual(
      sequence.map(({ step }) => step.name),
      [
        "Check the auditor",
        "Install the auditor's dependencies",
        "Run conformance",
      ],
    );
    for (const name of ["missing", "directory"])
      await t.test(name, (t) => {
        const dir = temporary(t),
          bin = join(dir, "bin"),
          auditor = join(dir, "auditor"),
          goMarker = join(dir, "go-ran"),
          moduleMarker = join(dir, "module-ran");
        mkdirSync(join(auditor, "hooks/go"), { recursive: true });
        if (name === "directory")
          mkdirSync(join(auditor, auditorCli), { recursive: true });
        // Had a later step started, the module would leave its marker, and the build would leave the go marker.
        write(
          join(auditor, ".github/tools/src/scripts/ci-conformance.ts"),
          `const { writeFileSync } = process.getBuiltinModule("node:fs");
writeFileSync(process.env.MODULE_MARKER, process.argv.slice(2).join(" "));
`,
          0o644,
        );
        const goFake = join(dir, "go.mjs");
        write(
          goFake,
          'import { writeFileSync } from "node:fs";\nwriteFileSync(process.env.GO_MARKER, "");\n',
          0o644,
        );
        writeLauncher(bin, "go", goFake);
        const inputs: Record<string, string> = {
          "${{ inputs.auditor }}": auditor,
          "${{ inputs.repo }}": dir,
          "${{ inputs.domain }}": "all",
          "${{ inputs.allow-stale-review }}": "false",
          "${{ inputs.skip-module-tests }}": "false",
        };
        const ran: string[] = [];
        let last = { exit: 0, output: "" };
        for (const checked of sequence) {
          const step = checked.step;
          const env: NodeJS.ProcessEnv = {
            PATH:
              bin +
              (process.platform === "win32" ? ";" : ":") +
              (process.env.PATH ?? ""),
            RUNNER_TEMP: dir,
            GITHUB_OUTPUT: join(dir, "output"),
            GO_MARKER: goMarker,
            MODULE_MARKER: moduleMarker,
          };
          for (const [key, expr] of Object.entries(step.env ?? {})) {
            assert.ok(
              Object.hasOwn(inputs, expr),
              `unresolved expression ${expr}`,
            );
            env[key] = inputs[expr];
          }
          ran.push(step.name!);
          last = execute(checked, env, dir);
          if (last.exit !== 0) break;
        }
        assert.deepEqual(ran, ["Check the auditor"]);
        assert.equal(last.exit, 1, last.output);
        assert.ok(
          last.output.includes(
            `conformance action: ${auditor} has no .github/tools/src/conformance/cli.ts`,
          ),
          last.output,
        );
        assert.ok(absent(goMarker), "go ran");
        assert.ok(absent(moduleMarker), "the module ran");
      });
    await t.test("present", (t) => {
      const auditor = temporary(t);
      write(join(auditor, auditorCli), "", 0o644);
      const r = execute(sequence[0]!, {
        AGX_CONFORMANCE_AUDITOR: auditor,
      });
      assert.equal(r.exit, 0, r.output);
      assert.equal(r.output, "");
    });
  },
);

const emptyAudit =
  '{"version":1,"areas":[{"name":"ascii","failures":[],"error":null},{"name":"schema","failures":[],"error":null},{"name":"module","failures":[],"error":null},{"name":"freshness","failures":[],"error":null}]}';

// The real hooks domain starts the verifier directly, so the fake verifier must be an executable on every platform: a
// Node executable whose name is hooktool. This preload, loaded by every Node in the run through NODE_OPTIONS, acts only
// in the process started under that name, and exits before Node looks for the "audit" script.
const fakes = mkdtempSync(join(tmpdir(), "agentics-action-fakes-"));
after(() => rmSync(fakes, { recursive: true, force: true }));
const verifierPreload = join(fakes, "verifier-preload.mjs");
writeFileSync(
  verifierPreload,
  `import { writeSync } from "node:fs";
import path from "node:path";
if (path.basename(process.argv0).replace(/\\.exe$/i, "") === "hooktool") {
  writeSync(2, process.env.FAKE_VERIFIER_NOTICE ?? "");
  writeSync(1, process.env.FAKE_VERIFIER_OUTPUT ?? "");
  process.exit(Number(process.env.FAKE_VERIFIER_EXIT || "0"));
}
`,
);
let nodeCopy: string | undefined;
// Windows cannot symlink without a privilege, so the build hard-links a test-owned copy of this Node instead.
function linkVerifier(out: string): void {
  if (process.platform !== "win32") {
    symlinkSync(process.execPath, out);
    return;
  }
  if (nodeCopy === undefined) {
    nodeCopy = join(fakes, "node-copy.exe");
    copyFileSync(process.execPath, nodeCopy);
  }
  linkSync(nodeCopy, out);
}

// The local validator runs as --validate does, with only the conformance call for every other domain stubbed: the go
// build is a fake that puts the verifier in place (or fails, or writes nothing), and the hooks call reaches the real
// TypeScript suite in the trusted checkout, which starts the verifier the validator built. Each conformance call's
// domain and extra arguments land in `calls`.
function localFixture(
  t: TestContext,
  verifier: Record<string, string>,
  mode: "ok" | "failed" | "missing" = "ok",
) {
  const dir = temporary(t),
    trusted = join(dir, "trusted"),
    cache = join(dir, "cache");
  mkdirSync(trusted);
  write(join(trusted, ".claude-plugin/plugin.json"), "{}", 0o644);
  write(
    join(trusted, "hooks/go/go.mod"),
    "module fixture\n\ngo 1.27.4\n",
    0o644,
  );
  writeAuditorLauncher(trusted);
  const forgedMarker = join(dir, "forged-marker");
  write(
    join(trusted, "hooktool"),
    `#!/bin/sh\necho forged-candidate >&2\ntouch "${forgedMarker}"\nexit 0\n`,
  );
  const builds: string[][] = [],
    calls: string[] = [];
  let output = "";
  const runner: Runner = (command) => {
    if (command.tool === "go") {
      builds.push(command.args);
      const at = (flag: string) => command.args[command.args.indexOf(flag) + 1];
      if (
        at("-C") !== join(trusted, "hooks", "go") ||
        command.args.at(-1) !== "./cmd/hooktool"
      )
        return { found: true, status: 1, output: "" };
      if (mode === "failed") {
        output += "trusted-build-failed\n";
        return { found: true, status: 7, output: "" };
      }
      if (mode === "ok") linkVerifier(at("-o")!);
      return { found: true, status: 0, output: "" };
    }
    assert.equal(command.tool, "node", `unexpected ${command.tool} call`);
    assert.equal(command.args[0], join(trusted, auditorCli));
    assert.deepEqual(command.args.slice(2, 4), ["--repo", trusted]);
    const domain = command.args[1]!;
    calls.push([domain, ...command.args.slice(4)].join(" "));
    if (domain === "all") {
      output += `ts-conformance ${domain}\n`;
      return { found: true, status: 1, output: "" };
    }
    const r = spawnSync(process.execPath, command.args, {
      env: command.env,
      encoding: "utf8",
      timeout: 60_000,
    });
    assert.ifError(r.error);
    output += r.stdout + r.stderr;
    return { found: true, status: r.status ?? 1, output: "" };
  };
  const exit = runValidate({
    mode: "strict",
    repoRoot: trusted,
    toolCacheDir: cache,
    env: hostEnv({
      NODE_OPTIONS: `--import=${pathToFileURL(verifierPreload).href}`,
      AGX_CONFORMANCE_SHORT: "1",
      AGX_HOOK_VERIFIER: join(trusted, "hooktool"),
      ...verifier,
    }),
    platform: process.platform,
    runner,
    nodeVersion: "26.12.1",
    enginesNode: "^22.23.5 || ^24.22.3 || >=26.12.1",
    write: (_stream, text) => {
      output += text;
    },
  });
  return { trusted, exit, output, builds, calls, forgedMarker };
}
function assertTrustedBuild(f: ReturnType<typeof localFixture>): void {
  assert.equal(f.builds.length, 1, f.output);
  assert.equal(f.builds[0]![2], join(f.trusted, "hooks", "go"));
  assert.equal(f.builds[0]!.at(-1), "./cmd/hooktool");
}

test("local validator trusted hook verifier success", (t) => {
  const f = localFixture(t, {
    FAKE_VERIFIER_NOTICE: "local-trusted-notice\n",
    FAKE_VERIFIER_OUTPUT: `${emptyAudit}\n`,
  });
  assert.equal(f.exit, 1, f.output);
  for (const text of [
    "local-trusted-notice",
    "PASS hooks",
    "ts-conformance all",
    "Validation failed (conformance all)",
  ])
    assert.ok(f.output.includes(text), f.output);
  assertTrustedBuild(f);
  assert.deepEqual(f.calls, ["hooks", "all --exclude hooks"]);
});

test("local validator hook verifier outcomes", async (t) => {
  const finding = {
    rule: "local-rule",
    path: "local-path",
    message: "local-message",
  };
  const withFinding = JSON.parse(emptyAudit);
  withFinding.areas[0].failures.push(finding);
  const withError = structuredClone(withFinding);
  withError.areas[1].error = "local-operation";
  withError.areas[2].error = "not-run: prior operational error";
  withError.areas[3].error = "not-run: prior operational error";
  const cases = [
    {
      name: "rule_finding",
      result: JSON.stringify(withFinding),
      diagnostic: "local-rule-notice",
      mode: "ok",
      exit: 0,
      want: ["FAIL hooks [local-rule] local-path: local-message"],
    },
    {
      name: "exit1_finding_and_error",
      result: JSON.stringify(withError),
      diagnostic: "local-error-detail",
      mode: "ok",
      exit: 1,
      want: [
        "FAIL hooks [local-rule] local-path: local-message",
        "ERROR hooks: audit could not run",
        "local-operation",
        "local-error-detail",
        "hook audit exited with code 1",
      ],
    },
    {
      name: "malformed_nonzero",
      result: "broken-output",
      diagnostic: "local-crash-detail",
      mode: "ok",
      exit: 23,
      want: [
        "ERROR hooks: audit could not run",
        "invalid hook audit result",
        "hook audit exited with code 23",
        "local-crash-detail",
      ],
    },
    {
      name: "missing_trusted_build",
      result: "",
      diagnostic: "",
      mode: "missing",
      exit: 0,
      want: ["ERROR hooks: audit could not run", "hook audit could not start"],
    },
    {
      name: "failed_trusted_build",
      result: "",
      diagnostic: "",
      mode: "failed",
      exit: 0,
      want: [
        "trusted-build-failed",
        "Could not build the trusted hook verifier",
        "--validate needs Go 1.27.4 to build the hook verifier",
      ],
    },
  ] as const;
  for (const c of cases)
    await t.test(c.name, (t) => {
      const f = localFixture(
        t,
        {
          FAKE_VERIFIER_NOTICE: c.diagnostic === "" ? "" : `${c.diagnostic}\n`,
          FAKE_VERIFIER_OUTPUT: `${c.result}\n`,
          FAKE_VERIFIER_EXIT: String(c.exit),
        },
        c.mode,
      );
      assert.equal(f.exit, 1, f.output);
      for (const text of c.want) assert.ok(f.output.includes(text), f.output);
      assert.ok(!f.output.includes("PASS hooks"), f.output);
      assert.ok(!f.output.includes("forged-candidate"), f.output);
      assert.ok(absent(f.forgedMarker), "forged candidate ran");
      if (c.mode !== "failed")
        for (const text of [
          "ts-conformance all",
          "Validation failed (conformance all)",
        ])
          assert.ok(f.output.includes(text), f.output);
      else assert.ok(!f.output.includes("Validating manifests"), f.output);
      assert.deepEqual(
        f.calls,
        c.mode === "failed" ? [] : ["hooks", "all --exclude hooks"],
      );
      assertTrustedBuild(f);
    });
});
