import assert from "node:assert/strict";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import test from "node:test";
import {
  lookupWorkflowStep,
  runWorkflowStep,
  workflowShellSkip,
  withRunBody,
  type CheckedWorkflowStep,
  type WorkflowStepData,
} from "./testing/workflow-steps.ts";

const WORKFLOW = ".github/workflows/validate.yml";
const JOB = "pr-metadata";
const workflow = readFileSync(
  path.join(import.meta.dirname, "../../../..", WORKFLOW),
  "utf8",
);

function titleStepOf(source: string) {
  return lookupWorkflowStep(source, WORKFLOW, JOB, "Check the PR title");
}

const step = titleStepOf(workflow);
const script = step.step.run!;
function testsForStep(checked: CheckedWorkflowStep) {
  const options = { skip: workflowShellSkip(checked) };
  return (name: string, body: () => void) => test(name, options, body);
}
const titleTest = testsForStep(step);
const marker = lookupWorkflowStep(
  workflow,
  WORKFLOW,
  JOB,
  "Check the release markers",
);
const bodyScript = marker.step.run!;
const markerTest = testsForStep(marker);

test("the Ubuntu PR title regression step requires the shell that runs the checked title", () => {
  const regression = lookupWorkflowStep(
    workflow,
    WORKFLOW,
    "validate-claude-native",
    "PR title guard regressions",
  );
  const env = { ...regression.step.env, PATH: "" };
  assert.throws(
    () => workflowShellSkip(step, { env }),
    /required interpreter "bash" is missing/,
  );
  assert.equal(
    workflowShellSkip(step, { env: { PATH: "" } }),
    "bash is not available on this host",
  );
});

function run(title: string, selected = step) {
  return runWorkflowStep(selected, {
    cwd: process.cwd(),
    env: { ...process.env, PR_TITLE: title },
  });
}

titleTest(
  "the title fixture executes shell statements surrounding the checked heredoc",
  () => {
    const result = run(
      "fix: handle input",
      withRunBody(step, `${script}\nprintf 'after title\\n' >&2\nexit 23\n`),
    );
    assert.deepEqual(
      { status: result.status, stdout: result.stdout, stderr: result.stderr },
      {
        status: 23,
        stdout: "PR title: Conventional Commits format accepted.\n",
        stderr: "after title\n",
      },
    );
  },
);

titleTest(
  "the title fixture ignores a same-name step in an earlier job",
  () => {
    const source = workflow.replace(
      /^jobs:\r?$/m,
      [
        "jobs:",
        "  decoy:",
        "    steps:",
        "      - name: Check the PR title",
        "        run: |",
        "          node <<'NODE'",
        "          console.log('Earlier step ran.');",
        "          process.exitCode = 73;",
        "          NODE",
      ].join("\n"),
    );
    assert.notEqual(source, workflow);
    const result = run("fix: handle input", titleStepOf(source));
    assert.equal(result.status, 0, result.stdout + result.stderr);
    assert.equal(
      result.stdout,
      "PR title: Conventional Commits format accepted.\n",
    );
    assert.equal(result.stderr, "");
  },
);

for (const title of [
  "fix!: remove old behavior",
  "docs: describe Windows and Codex setup",
  "feat: élargir le support",
  "feat(hooks): add x (see 1.2.3 notes)",
  "chore(deps): bump x",
  "chore: tidy",
  "chore(releases): x",
  "docs(release): x",
  ...[
    "feat",
    "fix",
    "refactor",
    "docs",
    "test",
    "chore",
    "style",
    "ci",
    "perf",
    "build",
    "revert",
  ].map((type) => `${type}: update tooling`),
]) {
  titleTest(`accepts ${title}`, () => {
    const result = run(title);
    assert.equal(result.status, 0, result.stdout + result.stderr);
    assert.match(result.stdout, /Conventional Commits format accepted/);
  });
}

const RESERVED_RELEASE_TITLE =
  "The chore(release): title is reserved for the release commit that the release workflow makes.";

for (const [title, message] of [
  ["Sweep foo: #1", "must use"],
  ["sweep: update hooks", "unsupported Conventional Commits type"],
  ["research(evals): measure a cycle", "unsupported Conventional Commits type"],
  ["feat: Add x", "summary must start in lower case"],
  ["feat: 123 updates", "summary must start in lower case"],
  ["feat: # tidy metadata", "summary must start in lower case"],
  ["feat: 中文更新", "summary must start in lower case"],
  ["feat: 🚀 update tooling", "summary must start in lower case"],
  ["feat: add x.", "summary must start in lower case"],
  ["feat(hooks): add x (1.2.3)", "must not end with a version"],
  [
    "refactor(hooks)!: simplify dispatch (1.2.3-rc.1)",
    "must not end with a version",
  ],
  ["fix: stop y (1.2.3+b)", "must not end with a version"],
  ["feat: add x. (1.2.3)", "must not end with a version"],
  ["chore(release): v1.2.3 [skip ci]", RESERVED_RELEASE_TITLE],
  ["chore(release): anything", RESERVED_RELEASE_TITLE],
  ["chore(release)!: anything", RESERVED_RELEASE_TITLE],
  ["feat: ", "must use"],
  ["feat: (1.2.3)", "must not end with a version"],
  ["feat(): add x", "must use"],
  ["feat(two words): add x", "must use"],
  ["feat: add x\nfix: add y", "must use"],
  ["feat: add x\r", "must use"],
  ["feat: add x ", "must use"],
  ["feat:add x", "must use"],
  ["", "must use"],
] as const) {
  titleTest(`rejects ${JSON.stringify(title)}`, () => {
    const result = run(title);
    assert.equal(result.status, 1, result.stdout + result.stderr);
    assert.ok(result.stdout.includes(message), result.stdout);
  });
}

titleTest(
  "unsupported-type rejection dies when the production type guard is removed",
  () => {
    const mutated = script.replace(
      /if \(!types\.has\(match\[1\]\)\) \{[\s\S]*?\n\}/,
      "",
    );
    assert.notEqual(mutated, script);
    const result = run("sweep: update hooks", withRunBody(step, mutated));
    assert.equal(result.status, 0, result.stdout + result.stderr);
    assert.throws(() => assert.equal(result.status, 1), assert.AssertionError);
  },
);

titleTest(
  "reserved release-title rejection dies when the production check is removed",
  () => {
    const mutated = script.replace(
      /if \(match\[1\] === "chore" && match\[2\] === "release"\) \{[\s\S]*?\n\}/,
      "",
    );
    assert.notEqual(mutated, script);
    for (const title of [
      "chore(release): v1.2.3 [skip ci]",
      "chore(release)!: anything",
    ] as const) {
      assert.equal(run(title).status, 1);
      const result = run(title, withRunBody(step, mutated));
      assert.equal(result.status, 0, result.stdout + result.stderr);
      assert.throws(
        () => assert.equal(result.status, 1),
        assert.AssertionError,
      );
    }
  },
);

titleTest(
  "lowercase-start rejection dies when the production lowercase check is removed",
  () => {
    const mutated = script.replace("!/^\\p{Ll}/u.test(summary)", "false");
    assert.notEqual(mutated, script);
    const result = run("feat: 123 updates", withRunBody(step, mutated));
    assert.equal(result.status, 0, result.stdout + result.stderr);
    assert.throws(() => assert.equal(result.status, 1), assert.AssertionError);
  },
);

titleTest(
  "version-suffix rejection dies when the production version check is removed",
  () => {
    const mutated = script.replace(".test(match[4])", '.test("")');
    assert.notEqual(mutated, script);
    const result = run(
      "feat(hooks): add x (1.2.3)",
      withRunBody(step, mutated),
    );
    assert.equal(result.status, 0, result.stdout + result.stderr);
    assert.throws(() => assert.equal(result.status, 1), assert.AssertionError);
  },
);

test("title is passed through env and never interpolated in run", () => {
  assert.deepEqual(step.step.env, {
    PR_TITLE: "${{ fromJSON(steps.metadata.outputs.snapshot).title }}",
  });
  assert.doesNotMatch(step.step.run!, /\$\{\{/);
});

titleTest("shell-shaped title content stays inert data", () => {
  const result = run('fix: handle $(exit 73) `exit 74` "quotes"; exit 75');
  assert.equal(result.status, 0, result.stdout + result.stderr);
  assert.equal(
    result.stdout,
    "PR title: Conventional Commits format accepted.\n",
  );
});

function runMarkers(
  title: string,
  body: string,
  selected = marker,
  outputPath = "",
) {
  return runWorkflowStep(selected, {
    cwd: process.cwd(),
    env: {
      ...process.env,
      PR_TITLE: title,
      PR_BODY: body,
      GITHUB_OUTPUT: outputPath,
    },
  });
}

// Footer lines the commit parser reads as a breaking-change note: any case,
// a leading run of whitespace, | and *, and a colon or whitespace after the
// keyword.
const FOOTER_LINES = [
  "BREAKING CHANGE: none",
  "BREAKING-CHANGE: none",
  "Breaking change: none",
  "breaking change: x",
  "Breaking Change: x",
  "* BREAKING CHANGE: x",
  "  BREAKING CHANGE: x",
  "**BREAKING CHANGE:** x",
  "BREAKING CHANGE no colon",
];

// Lines the parser does not read as a note, so no ! is required.
const NOT_FOOTER_LINES = [
  "BREAKING CHANGES: x",
  "see BREAKING CHANGE: x",
  "BREAKING CHANGE\nmore text",
  "BREAKING_CHANGE: x",
  "> BREAKING CHANGE: x",
];

const withBody = (line: string, eol = "\n") =>
  `## What changed\n\nx\n\n${line}\n`.replace(/\n/g, eol);

markerTest("a body BREAKING CHANGE line without ! in the title fails", () => {
  for (const line of FOOTER_LINES) {
    const result = runMarkers("docs: clarify intent", withBody(line));
    assert.equal(result.status, 1, `${line}\n${result.stdout}${result.stderr}`);
    assert.ok(
      result.stdout.includes("BREAKING CHANGE footer needs a ! in the title"),
      result.stdout,
    );
  }
});

markerTest(
  "a BREAKING CHANGE line with ! in the title passes and reports a major",
  () => {
    for (const line of FOOTER_LINES) {
      const result = runMarkers("feat(skills)!: retire wrap", withBody(line));
      assert.equal(
        result.status,
        0,
        `${line}\n${result.stdout}${result.stderr}`,
      );
      assert.match(result.stdout, /Release level: major/);
    }
  },
);

markerTest("lines the parser does not read as a note need no !", () => {
  for (const line of NOT_FOOTER_LINES) {
    const result = runMarkers("docs: clarify intent", withBody(line));
    assert.equal(result.status, 0, `${line}\n${result.stdout}${result.stderr}`);
    assert.match(result.stdout, /Release level: none/);
  }
});

markerTest("a footer in a CRLF body is still found", () => {
  const result = runMarkers(
    "docs: clarify intent",
    withBody("BREAKING CHANGE: none", "\r\n"),
  );
  assert.equal(result.status, 1, result.stdout + result.stderr);
  assert.ok(
    result.stdout.includes("BREAKING CHANGE footer needs a ! in the title"),
    result.stdout,
  );
});

test("release-marker step reads title and body only through env", () => {
  const envOnly = (step: WorkflowStepData) => {
    assert.deepEqual(step.env, {
      PR_TITLE: "${{ fromJSON(steps.metadata.outputs.snapshot).title }}",
      PR_BODY: "${{ fromJSON(steps.metadata.outputs.snapshot).body }}",
    });
    assert.doesNotMatch(step.run!, /\$\{\{/);
    assert.doesNotMatch(step.run!, /github\.event/);
  };
  envOnly(marker.step);
  const run = marker.step.run!.replace(
    'process.env.PR_BODY || ""',
    '"${{ github.event.pull_request.body }}"',
  );
  assert.notEqual(run, marker.step.run);
  assert.throws(() => envOnly({ ...marker.step, run }), assert.AssertionError);
});

markerTest(
  "each change to the footer pattern is caught by a footer line or a non-footer line",
  () => {
    const mutants = [
      ["case-sensitive", "/im.test(body)", "/m.test(body)", FOOTER_LINES],
      ["no leading run", "^[\\s|*]*BREAKING", "^BREAKING", FOOTER_LINES],
      ["no hyphen form", "BREAKING[ -]CHANGE", "BREAKING CHANGE", FOOTER_LINES],
      ["colon only", "(?::|[^\\S\\r\\n])+", ":", FOOTER_LINES],
      ["no multiline flag", "/im.test(body)", "/i.test(body)", FOOTER_LINES],
      [
        "no line anchor",
        "/^[\\s|*]*BREAKING",
        "/[\\s|*]*BREAKING",
        NOT_FOOTER_LINES,
      ],
      [
        "crosses a line break",
        "(?::|[^\\S\\r\\n])+",
        "[:\\s]+",
        NOT_FOOTER_LINES,
      ],
    ] as const;
    for (const [name, from, to, lines] of mutants) {
      const mutated = bodyScript.replace(from, to);
      assert.notEqual(mutated, bodyScript, name);
      const wrong = lines.filter((line) => {
        const result = runMarkers(
          "docs: clarify intent",
          withBody(line),
          withRunBody(marker, mutated),
        );
        return lines === FOOTER_LINES
          ? result.status === 0
          : result.status === 1;
      });
      assert.ok(wrong.length > 0, `${name} was not caught`);
    }
  },
);

markerTest("the release level reaches stdout and the step output", () => {
  const dir = mkdtempSync(path.join(tmpdir(), "pr-title-"));
  try {
    const summary = path.join(dir, "output");
    for (const [title, level] of [
      ["feat: add x", "minor"],
      ["fix: stop y", "patch"],
      ["docs: clarify intent", "none"],
      ["fix!: drop a flag", "major"],
    ] as const) {
      writeFileSync(summary, "");
      const result = runMarkers(
        title,
        "## What changed\n\nx\n",
        marker,
        summary,
      );
      assert.equal(result.status, 0, result.stdout + result.stderr);
      assert.ok(
        result.stdout.split("\n").includes(`Release level: ${level}`),
        result.stdout,
      );
      assert.ok(
        readFileSync(summary, "utf8").split("\n").includes(`level=${level}`),
        readFileSync(summary, "utf8"),
      );
    }
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
});

markerTest(
  "the marker rejection dies when the production check is removed",
  () => {
    const mutated = bodyScript.replace(
      /if \(marker && !breaking\) \{[\s\S]*?\n\}/,
      "",
    );
    assert.notEqual(mutated, bodyScript);
    const result = runMarkers(
      "docs: clarify intent",
      "## What changed\n\nx\n\nBREAKING CHANGE: none\n",
      withRunBody(marker, mutated),
    );
    assert.equal(result.status, 0, result.stdout + result.stderr);
  },
);
