import assert from "node:assert/strict";
import {
  mkdirSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { describe, test } from "node:test";
import { releaseCall } from "../scripts/ci-release-run.ts";
import {
  withRunBody,
  readWorkflowArgv,
  runWorkflowStep,
  workflowShellSkip,
} from "../scripts/testing/workflow-steps.ts";
import {
  actionInputViolations,
  actionOutputViolations,
  buildStep,
  checkedReleaseStep,
  flagsFromSource,
  inputsWithoutFlag,
  loadReleaseAction,
  outputKeysFromTypeScript,
  outputsWrittenByCommands,
  outputWriters,
  parseReleaseAction,
  registeredFlags,
  releaseCommands,
  runStep,
  stepReadsInput,
  type OutputWriter,
} from "./testing/action.ts";
import {
  expectedArgv,
  installReleaseRun,
  releaseRunSource,
  runArgv,
  runNonBundleScenario,
  runRealBuildScenario,
  runTrustedVerifierScenario,
  trustedVerifierViolations,
} from "./testing/action-run.ts";
import { readRepoText } from "./testing/repo.ts";
import { platformSkip, posix, sh, tempGitRepo } from "./testing/support.ts";
import {
  replaceEvery,
  replaceOnce,
  withoutYAMLComments,
} from "./testing/workflow.ts";

const { action, source } = loadReleaseAction();
const commands = releaseCommands(action);

// The flags a command registers, for the negative cases below: every input that is not on the allow-list, registered
// by one command. A rename or a new input then breaks exactly the rule it should.
function flagsMatchingInputs(): Map<string, string[]> {
  return new Map(
    Object.keys(action.inputs)
      .filter((name) => !(name in inputsWithoutFlag))
      .map((name) => [name, ["stage"]]),
  );
}

test("the action's inputs equal the release command's flags", () => {
  const registered = registeredFlags(commands);
  assert.deepEqual(
    actionInputViolations(action, source, registered, commands),
    [],
  );
});

test("the action declares every supported workflow command", () => {
  assert.deepEqual(commands, [
    "bundle",
    "context",
    "verify",
    "stage",
    "resume-check",
    "check-commit",
    "asset-roster",
  ]);
});

test("the action forwards the tag to asset-roster in the selected checkout", () => {
  const { args } = releaseCall({
    CHECKOUT: "checkout",
    COMMAND: "asset-roster",
    TAG: "v9.8.7",
    MAIN_REF: "refs/remotes/origin/main",
  });
  assert.deepEqual(args, [
    "checkout/.github/tools/src/release/cli.ts",
    "asset-roster",
    "--tag",
    "v9.8.7",
  ]);
});

test("the action inputs hold with a flag set that mirrors them", () => {
  assert.deepEqual(
    actionInputViolations(action, source, flagsMatchingInputs(), commands),
    [],
  );
});

// Each case breaks one rule of the input check and must be named by it. Removing the rule makes its case pass silently.
test("a change that breaks the input rules is reported", () => {
  const registered = flagsMatchingInputs();
  const cases: [string, string, Map<string, string[]>, string[]][] = [
    [
      "a renamed input",
      replaceOnce(
        source,
        "  baseline:\n    description:",
        "  baseline-file:\n    description:",
      ),
      registered,
      [
        'action input "baseline-file" maps to no --baseline-file flag',
        "--baseline is a flag of release stage, and the action declares no input of that name",
        'action input "baseline-file" is declared and never read',
      ],
    ],
    [
      "a new input no flag takes",
      replaceOnce(
        source,
        "  out:\n    description:",
        '  extra:\n    description: Not a flag\n    required: false\n    default: ""\n  out:\n    description:',
      ),
      registered,
      [
        'action input "extra" maps to no --extra flag',
        'action input "extra" is declared and never read',
      ],
    ],
    [
      "a flag with no input",
      source,
      new Map([...registered, ["ghost", ["stage"]]]),
      [
        "--ghost is a flag of release stage, and the action declares no input of that name",
      ],
    ],
    [
      "a command that registers --token",
      source,
      new Map([...registered, ["token", ["context", "stage"]]]),
      [
        "release context and stage now registers --token, so the allowlist entry for the token input is stale",
      ],
    ],
    [
      "a run step that no longer passes the token",
      replaceOnce(source, "GH_TOKEN: ${{ inputs.token }}", "GH_TOKEN: ''"),
      registered,
      [
        'the run step passes GH_TOKEN as "", so the token input no longer reaches the command',
      ],
    ],
    [
      "a build step that no longer reads the checkout",
      replaceOnce(
        source,
        '      env:\n        CHECKOUT: ${{ inputs.checkout }}\n        COMMAND: ${{ inputs.command }}\n      run: |\n        if [ "$COMMAND" = bundle ]; then\n          go build',
        '      env:\n        CHECKOUT: /fixed\n        COMMAND: ${{ inputs.command }}\n      run: |\n        if [ "$COMMAND" = bundle ]; then\n          go build',
      ),
      registered,
      [
        "the build step no longer reads the checkout input, so that allowlist entry is stale",
      ],
    ],
  ];
  for (const [label, text, flags, wants] of cases) {
    const got = actionInputViolations(
      parseReleaseAction(text),
      text,
      flags,
      commands,
    );
    for (const want of wants) {
      assert.ok(
        got.some((violation) => violation.includes(want)),
        `${label}: ${JSON.stringify(got)} lacks ${JSON.stringify(want)}`,
      );
    }
  }
});

test("the three allow-listed inputs are exactly the ones with another route", () => {
  assert.deepEqual(Object.keys(inputsWithoutFlag).sort(), [
    "checkout",
    "command",
    "token",
  ]);
  assert.equal(stepReadsInput({ A: "${{ inputs.x }}" }, "", "x"), true);
  assert.equal(stepReadsInput({}, 'echo "${{ inputs.x }}"', "x"), true);
  assert.equal(stepReadsInput({ A: "${{ inputs.y }}" }, "echo", "x"), false);
});

test("the TypeScript flag reader finds the allow-list of a parseArgs call", () => {
  assert.deepEqual(
    flagsFromSource(
      'const p = parseArgs(argv, ["repo", "--sha", \'main-ref\']);',
    ),
    ["repo", "sha", "main-ref"],
  );
  assert.deepEqual(
    flagsFromSource(
      'const FLAGS = ["repo", "tag"] as const;\nexport const run = () => parseArgs(argv, FLAGS);',
    ),
    ["repo", "tag"],
  );
  assert.deepEqual(
    flagsFromSource(
      'const parsed = parseArgs(\n  argv.slice(1),\n  ["a", "b"],\n);',
    ),
    ["a", "b"],
  );
  assert.deepEqual(
    flagsFromSource(
      'const p = parseFlags(argv, ["checkout", "sha"], io, PREFIX);',
    ),
    ["checkout", "sha"],
  );
  // A module that parses a subset first, the way bundle reads the version inputs before its own flags: every call
  // counts, and a flag both calls name is listed once.
  assert.deepEqual(
    flagsFromSource(
      'const FLAGS = ["repo", "sha", "tag", "out"] as const;\nconst v = () => parseArgs(argv, ["repo", "sha", "main-ref"]);\nexport const run = () => parseArgs(argv, FLAGS);',
    ).sort(),
    ["main-ref", "out", "repo", "sha", "tag"],
  );
  assert.deepEqual(
    flagsFromSource(
      'parseFlags(argv, ["checkout"], io, PREFIX);\nparseArgs(argv, ["sha", "checkout"]);',
    ).sort(),
    ["checkout", "sha"],
  );
  assert.throws(
    () => flagsFromSource('parseArgs(argv, ["a"]);\nparseArgs(argv);'),
    /no allow-list/,
  );
  assert.throws(
    () => flagsFromSource("export const run = () => 1;"),
    /never calls parseArgs/,
  );
  assert.throws(() => flagsFromSource("parseArgs(argv);"), /no allow-list/);
  assert.throws(
    () => flagsFromSource("parseArgs(argv, MISSING);"),
    /cannot find the allow-list MISSING/,
  );
});

test("the TypeScript flag reader rejects an expression that resembles a declaration", () => {
  assert.throws(
    () =>
      flagsFromSource(
        'const ACTIVE_FLAGS = ["repo"] as const;\nparseArgs(argv, ACTIVE.FLAGS);',
      ),
    /cannot find the allow-list ACTIVE\.FLAGS/,
  );
});

test("the TypeScript output reader finds keys in a literal and in a variable", () => {
  assert.deepEqual(
    outputKeysFromTypeScript(
      'writeOutputs({ mode, resume_tag: tag, "quoted": 1, ...rest }, env);',
    ),
    ["mode", "resume_tag", "quoted"],
  );
  assert.deepEqual(
    outputKeysFromTypeScript(
      "const outputs: Record<string, string> = {\n  release_id: String(id),\n  published: String(done),\n};\nwriteOutputs(outputs, env, io.stdout);",
    ),
    ["release_id", "published"],
  );
});

test("the command sources write the four outputs the action declares", () => {
  const written = outputsWrittenByCommands();
  assert.deepEqual(
    [...written.keys()].sort(),
    Object.keys(action.outputs).sort(),
  );
});

test("every action output records its writer", () => {
  assert.deepEqual(
    actionOutputViolations(action, outputsWrittenByCommands()),
    [],
  );
});

test("a change that breaks the output rules is reported", () => {
  const written = new Map([
    ["release_id", ["stage"]],
    ["published", ["stage"]],
    ["mode", ["resume-check"]],
    ["resume_tag", ["resume-check"]],
  ]);
  assert.deepEqual(actionOutputViolations(action, written), []);
  const without = (name: string): Record<string, OutputWriter> =>
    Object.fromEntries(
      Object.entries(outputWriters).filter(([key]) => key !== name),
    );
  const shellPublished: Record<string, OutputWriter> = {
    ...outputWriters,
    published: "shell",
  };
  const cases: [string, string[]][] = [
    [
      "an output with no recorded writer",
      actionOutputViolations(action, written, without("mode")),
    ],
    [
      "an output that reads a drifted key",
      actionOutputViolations(
        parseReleaseAction(
          replaceOnce(
            source,
            "value: ${{ steps.run.outputs.mode }}",
            "value: ${{ steps.run.outputs.modes }}",
          ),
        ),
        written,
      ),
    ],
    [
      "an output no command writes",
      actionOutputViolations(
        action,
        new Map([...written].filter(([key]) => key !== "published")),
      ),
    ],
    [
      "a record for an output the action dropped",
      actionOutputViolations(action, written, {
        ...outputWriters,
        ghost: "command",
      }),
    ],
    [
      "a shell writer whose key a command now writes",
      actionOutputViolations(action, written, shellPublished),
    ],
  ];
  const wants = [
    'action output "mode" has no recorded writer',
    'action output "mode" reads "${{ steps.run.outputs.modes }}"',
    'no command source passes "published" to the output writer',
    'outputWriters records "ghost", which the action no longer declares',
    'stage now writes "published" through the output writer',
  ];
  cases.forEach(([label, got], index) => {
    const want = wants[index] ?? "";
    assert.ok(
      got.some((violation) => violation.includes(want)),
      `${label}: ${JSON.stringify(got)} lacks ${JSON.stringify(want)}`,
    );
  });
  assert.ok(
    actionOutputViolations(
      action,
      new Map([...written].filter(([key]) => key !== "published")),
      shellPublished,
    ).some((v) =>
      v.includes("the run step no longer appends published= to $GITHUB_OUTPUT"),
    ),
  );
});

test("the run step is the one with an id and the build step is found by its Go build", () => {
  assert.equal(runStep(action).id, "run");
  assert.ok(buildStep(action).run.includes("./cmd/hooktool"));
  const noGo = parseReleaseAction(
    replaceEvery(source, "go build", "echo build"),
  );
  assert.throws(() => buildStep(noGo), /no step builds \.\/cmd\/hooktool/);
});

const NODE_VERSION = "26.12.1";

// The action's steps by name, each with its text from its "- name:" line to the next step. Comment lines are dropped
// first, so prose cannot satisfy or trip a check.
function namedSteps(text: string): { name: string; text: string }[] {
  const body = withoutYAMLComments(text);
  return body
    .slice(body.indexOf("\nruns:\n"))
    .split(/^ {4}- name: /m)
    .slice(1)
    .map((part) => ({
      name: part.split("\n", 1)[0] ?? "",
      text: `- name: ${part}`,
    }));
}

// The runtime the action sets up: Node at the version and setup-node SHA release.md pins, installed before the Run step
// that executes cli.ts, and Go from hooks/go only for bundle, the one command that builds the hook verifier. These read
// text only, so they run on every platform.
function actionRuntimeViolations(text: string, workflow: string): string[] {
  const violations: string[] = [];
  const steps = namedSteps(text);
  const step = (name: string) => steps.find((s) => s.name === name);
  const at = (name: string) => steps.findIndex((s) => s.name === name);

  if (
    !step("Run")?.text.includes(
      'node "$CHECKOUT/.github/tools/src/scripts/ci-release-run.ts"',
    )
  ) {
    violations.push(
      "the Run step must execute node $CHECKOUT/.github/tools/src/scripts/ci-release-run.ts",
    );
  }

  const pin = /actions\/setup-node@([0-9a-f]{40})/.exec(workflow)?.[1];
  const workflowNode = /node-version: (\S+)/.exec(workflow)?.[1];
  assert.ok(pin, "release.md must pin setup-node by SHA");
  assert.equal(workflowNode, NODE_VERSION, "release.md pins another Node");
  const node = step("Set up Node");
  if (!node) {
    violations.push("the action has no Set up Node step");
  } else {
    if (!node.text.includes(`uses: actions/setup-node@${pin} `)) {
      violations.push(
        "Set up Node must use the SHA release.md pins for setup-node",
      );
    }
    if (!node.text.includes(`node-version: ${NODE_VERSION}\n`)) {
      violations.push(`Set up Node must install ${NODE_VERSION}`);
    }
    if (!node.text.includes(`if: env.AGX_NODE_SETUP != '${NODE_VERSION}'\n`)) {
      violations.push(
        "Set up Node must skip when the marker already names the version",
      );
    }
  }
  if (
    !step("Record the Node setup")?.text.includes(
      `run: printf 'AGX_NODE_SETUP=%s\\n' ${NODE_VERSION} >> "$GITHUB_ENV"\n`,
    )
  ) {
    violations.push("the Node marker must record the version in GITHUB_ENV");
  }
  if (!(
    at("Set up Node") >= 0 &&
    at("Set up Node") < at("Record the Node setup") &&
    at("Record the Node setup") < at("Run")
  )) {
    violations.push("Node must be set up and recorded before the Run step");
  }

  const go = step("Set up Go");
  if (
    !go?.text.includes(
      "if: inputs.command == 'bundle' && env.AGX_GO_CACHE_RESTORED != format(",
    )
  ) {
    violations.push("Set up Go must run for bundle only");
  }
  if (
    !go?.text.includes(
      "go-version-file: ${{ inputs.checkout }}/hooks/go/go.mod\n",
    )
  )
    violations.push("Set up Go must read go-version-file from hooks/go/go.mod");
  if (!go?.text.includes("cache: false\n"))
    violations.push(
      "the independent build must start without a restored Go cache",
    );
  const marker = step("Record the restored cache");
  if (
    !marker?.text.includes("GO_MOD: ${{ inputs.checkout }}/hooks/go/go.mod\n")
  ) {
    violations.push("the Go cache marker must name hooks/go/go.mod");
  }
  if (!marker?.text.includes("if: inputs.command == 'bundle'\n")) {
    violations.push("the Go cache marker must be recorded for bundle only");
  }
  if (
    !step("Build the hook verifier")?.text.includes(
      'if [ "$COMMAND" = bundle ]; then\n          go build -C "$CHECKOUT/hooks/go" -trimpath -o "${RUNNER_TEMP}/hooktool" ./cmd/hooktool\n        fi\n',
    )
  ) {
    violations.push("the hook verifier must build for bundle only");
  }
  return violations;
}

const releaseWorkflow = readRepoText(".github/workflows/release.md");

test("the action runs cli.ts on the pinned Node and sets up Go for bundle only", () => {
  assert.deepEqual(actionRuntimeViolations(source, releaseWorkflow), []);
});

test("a change that breaks the action's runtime setup is reported", () => {
  const cases: [string, string, string, RegExp][] = [
    [
      "the Run step executes another script",
      'node "$CHECKOUT/.github/tools/src/scripts/ci-release-run.ts"',
      'node "$CHECKOUT/.github/tools/src/release/cli.ts" "$COMMAND"',
      /Run step must execute node/,
    ],
    [
      "setup-node is pinned by tag",
      /actions\/setup-node@[0-9a-f]{40} # v7\.1\.0/.exec(source)?.[0] ?? "",
      "actions/setup-node@v7",
      /Set up Node must use the SHA/,
    ],
    [
      "the Node version drifts",
      "        node-version: 26.12.1\n",
      "        node-version: 26.6.0\n",
      /Set up Node must install 26\.12\.1/,
    ],
    [
      "the Node install loses its marker guard",
      "      if: env.AGX_NODE_SETUP != '26.12.1'\n",
      "",
      /must skip when the marker/,
    ],
    [
      "the marker records another version",
      '\' 26.12.1 >> "$GITHUB_ENV"',
      '\' 26.6.0 >> "$GITHUB_ENV"',
      /marker must record the version/,
    ],
    [
      "Go is set up for every command",
      "      if: inputs.command == 'bundle' && env.AGX_GO_CACHE_RESTORED",
      "      if: env.AGX_GO_CACHE_RESTORED",
      /Set up Go must run for bundle only/,
    ],
    [
      "Go is pinned from another module",
      "go-version-file: ${{ inputs.checkout }}/hooks/go/go.mod",
      "go-version-file: ${{ inputs.checkout }}/missing/go.mod",
      /must read go-version-file from hooks\/go\/go\.mod/,
    ],
    [
      "the independent build restores a shared cache",
      "cache: false",
      "cache: true",
      /without a restored Go cache/,
    ],
    [
      "the Go cache marker names another module",
      "GO_MOD: ${{ inputs.checkout }}/hooks/go/go.mod",
      "GO_MOD: ${{ inputs.checkout }}/missing/go.mod",
      /marker must name hooks\/go\/go\.mod/,
    ],
    [
      "a non-bundle call records the Go cache",
      "    - name: Record the restored cache\n      if: inputs.command == 'bundle'\n",
      "    - name: Record the restored cache\n",
      /cache marker must be recorded for bundle only/,
    ],
    [
      "the hook verifier builds for every command",
      '        if [ "$COMMAND" = bundle ]; then\n          go build -C "$CHECKOUT/hooks/go"',
      '        if true; then\n          go build -C "$CHECKOUT/hooks/go"',
      /hook verifier must build for bundle only/,
    ],
  ];
  for (const [label, from, to, expected] of cases) {
    const violations = actionRuntimeViolations(
      replaceOnce(source, from, to),
      releaseWorkflow,
    );
    assert.match(violations.join("\n"), expected, label);
  }
});

test("a Node setup that moves after the Run step is reported", () => {
  const start = source.indexOf("    - name: Set up Node\n");
  const end = source.indexOf("    # Go is needed only by bundle");
  assert.ok(start >= 0 && end > start, "the Node steps moved");
  const moved =
    source.slice(0, start) +
    source.slice(end) +
    "\n" +
    source.slice(start, end);
  assert.notEqual(moved, source);
  assert.match(
    actionRuntimeViolations(moved, releaseWorkflow).join("\n"),
    /Node must be set up and recorded before the Run step/,
  );
});

// The shell tests run the action's real YAML steps against shims, so they need bash. They skip on Windows with this
// reason, under CI=true as well as outside it. Nothing checks for bash on POSIX: if it were missing, the tests would
// fail when they run it.
const bashReason = platformSkip(
  "bash",
  posix,
  "run the action tests on Linux or macOS: the action steps are bash scripts",
);
const shell = { skip: bashReason };

const run = checkedReleaseStep(source, "Run");
const build = checkedReleaseStep(source, "Build the hook verifier");

test(
  "the checked release step delivers the original argv bytes through the selected shell",
  { skip: workflowShellSkip(run) },
  (t) => {
    const cwd = mkdtempSync(join(tmpdir(), "agentics release argv "));
    t.after(() => rmSync(cwd, { recursive: true, force: true }));
    const checkout = "checkout with spaces";
    installReleaseRun(join(cwd, checkout));
    const commandDir = join(cwd, checkout, ".github/tools/src/release");
    mkdirSync(commandDir, { recursive: true });
    writeFileSync(
      join(commandDir, "cli.ts"),
      `import {writeFileSync} from "node:fs";
writeFileSync(process.env.ACTION_ARGV, process.argv.slice(2).map(arg => arg + "\\0").join(""));
`,
    );
    const argvFile = join(cwd, "arguments");
    const inputEnv = Object.fromEntries(
      Object.keys(run.step.env!).map((key) => [key, ""]),
    );
    for (const value of ["", 'two words "quoted"\nnext line']) {
      const env = {
        ...process.env,
        ...inputEnv,
        ACTION_ARGV: argvFile,
        COMMAND: "stage",
        CHECKOUT: checkout,
        PRERELEASE: value,
      };
      const actual = runWorkflowStep(run, { cwd, env });
      assert.deepEqual(actual, { status: 0, stdout: "", stderr: "" });
      const expected = ["stage", "--prerelease", value];
      assert.deepEqual(
        readFileSync(argvFile),
        Buffer.from([...expected, ""].join("\0")),
      );
      assert.deepEqual(readWorkflowArgv(argvFile), expected);
      if (posix) assert.deepEqual(runArgv(run, "stage", value), expected);
    }
  },
);

test("the run step takes PRERELEASE from the prerelease input", () => {
  assert.equal(run.step.env?.["PRERELEASE"], "${{ inputs.prerelease }}");
});

describe("the action passes every command the right arguments", () => {
  for (const command of commands) {
    for (const value of ["", "true", "false", "yes"]) {
      test(`${command} with PRERELEASE ${JSON.stringify(value)}`, shell, () => {
        assert.deepEqual(
          runArgv(run, command, value),
          expectedArgv(command, value),
        );
      });
    }
  }
});

// The Run step is one line that starts ci-release-run.ts, so its mutants are patched copies of that module, run through
// the real step from the fixture checkout. Each must change what the scenario observes, so the matching check above
// fails for it.
const prereleaseFlag =
  'if (command === "stage") args.push("--prerelease", value("PRERELEASE"));';

test(
  "a run step that drops or fixes the prerelease flag is caught",
  shell,
  () => {
    const dropped = replaceOnce(releaseRunSource, prereleaseFlag, "");
    const fixed = replaceOnce(
      releaseRunSource,
      prereleaseFlag,
      'if (command === "stage") args.push("--prerelease", "false");',
    );
    assert.notDeepEqual(
      runArgv(run, "stage", "true", dropped),
      expectedArgv("stage", "true"),
    );
    assert.notDeepEqual(
      runArgv(run, "stage", "yes", fixed),
      expectedArgv("stage", "yes"),
    );
  },
);

const verifierCases: [
  string,
  "trusted" | "missing" | "incompatible",
  string,
][] = [
  ["trusted", "trusted", "trusted-verifier-called"],
  // The code Node reports for the verifier file that was never built.
  ["missing", "missing", "ENOENT"],
  ["incompatible", "incompatible", "incompatible-verifier"],
];
describe("bundle refuses a verifier that is not the trusted build", () => {
  for (const [name, mode, want] of verifierCases) {
    test(`the ${name} verifier`, shell, () => {
      const outcome = runTrustedVerifierScenario(build, run, mode);
      assert.deepEqual(trustedVerifierViolations(outcome, want), []);
    });
  }
});

test(
  "the trusted verifier runs against the candidate with hostile inherited Git settings",
  shell,
  async (t) => {
    const outside = tempGitRepo();
    const index = join(outside.root, ".git", "index");
    const workingFile = join(outside.root, "THIRD_PARTY_NOTICES.md");
    const edited = "uncommitted outside change\n";
    writeFileSync(workingFile, edited);
    const indexBefore = readFileSync(index);
    for (const [key, value] of [
      ["GIT_DIR", join(outside.root, ".git")],
      ["GIT_CONFIG_COUNT", "invalid"],
    ] as const) {
      await t.test(key, () => {
        const previous = process.env[key];
        try {
          process.env[key] = value;
          const outcome = runTrustedVerifierScenario(build, run, "trusted");
          assert.equal(
            sh(outside.root, "git", "rev-parse", "HEAD").trim(),
            outside.sha,
          );
          assert.deepEqual(readFileSync(index), indexBefore);
          assert.equal(readFileSync(workingFile, "utf8"), edited);
          assert.equal(outcome.status, 1, outcome.output);
          assert.deepEqual(
            trustedVerifierViolations(outcome, "trusted-verifier-called"),
            [],
          );
        } finally {
          if (previous === undefined) delete process.env[key];
          else process.env[key] = previous;
        }
      });
    }
  },
);

test("a run step that uses the candidate's verifier is caught", shell, () => {
  const mutated = replaceOnce(
    releaseRunSource,
    'childEnv["AGX_HOOK_VERIFIER"] = `${value("RUNNER_TEMP")}/hooktool`;',
    'childEnv["AGX_HOOK_VERIFIER"] = value("AGX_HOOK_VERIFIER");',
  );
  const outcome = runTrustedVerifierScenario(build, run, "trusted", mutated);
  assert.ok(
    trustedVerifierViolations(outcome, "trusted-verifier-called").includes(
      "candidate verifier ran",
    ),
  );
});

test(
  "a non-bundle command builds no verifier and passes only its own flags",
  shell,
  () => {
    assert.deepEqual(runNonBundleScenario(build, run), []);
  },
);

test("a run step that keeps the main ref for context is caught", shell, () => {
  const mutated = replaceOnce(
    releaseRunSource,
    'values["MAIN_REF"] = "";',
    'values["MAIN_REF"] = value("MAIN_REF");',
  );
  const violations = runNonBundleScenario(build, run, mutated);
  assert.equal(violations.length, 1);
  assert.match(violations[0] ?? "", /^non-bundle args: /);
});

test("a build step that always builds the verifier is caught", shell, () => {
  const mutated = replaceOnce(
    build.step.run!,
    'if [ "$COMMAND" = bundle ]; then',
    "if true; then",
  );
  const violations = runNonBundleScenario(withRunBody(build, mutated), run);
  assert.match(violations[0] ?? "", /^step: exit 42: .*unexpected-hooktool/s);
});

test("the build step leaves only the hook verifier", shell, () => {
  assert.deepEqual(runRealBuildScenario(build), []);
});

test(
  "the release build fixture honors the selected step's shell before building",
  shell,
  (t) => {
    const bash = withRunBody(build, "false | true\n" + build.step.run);
    const sh = { ...bash, step: { ...bash.step, shell: "sh" } };
    const unavailable = workflowShellSkip(sh);
    if (unavailable) return t.skip(unavailable);
    assert.deepEqual(runRealBuildScenario(bash), [
      "actual action build: exit 1: ",
    ]);
    assert.deepEqual(runRealBuildScenario(sh), []);
  },
);

test(
  "a build step that cannot build, or that builds more or less, is caught",
  shell,
  () => {
    const broken = replaceEvery(
      build.step.run!,
      "./cmd/hooktool",
      "./cmd/no-such-command",
    );
    assert.match(
      runRealBuildScenario(withRunBody(build, broken))[0] ?? "",
      /^actual action build: exit /,
    );
    const buildsRelease = `${build.step.run}\ntouch "\${RUNNER_TEMP}/release"\n`;
    assert.deepEqual(runRealBuildScenario(withRunBody(build, buildsRelease)), [
      "the build left release behind, but the action builds only hooktool",
    ]);
  },
);
