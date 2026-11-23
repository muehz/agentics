import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import { pathToFileURL } from "node:url";
import {
  fixtureGitEnv,
  git as fixtureGit,
} from "../conformance/testing/git-checkout.ts";
import {
  lookupWorkflowStep,
  type WorkflowStepData,
} from "./testing/workflow-steps.ts";

const ROOT = path.resolve(import.meta.dirname, "../../../..");
const WORKFLOW_FILE = ".github/workflows/validate.yml";
const WORKFLOW = path.join(ROOT, WORKFLOW_FILE);
const RUN = "node .github/tools/src/scripts/ci-version-fields.ts";
const STEP_NAME = "Reject a changed version field";

// The step hands the base commit to the module through BASE_SHA and nothing else.
function assertWiring(source: string) {
  const selected = lookupWorkflowStep(
    source,
    WORKFLOW_FILE,
    "pr-version-fields",
    STEP_NAME,
  );
  const { step } = selected;
  assert.equal(step.run?.trim(), RUN, `${STEP_NAME}: run must be the module`);
  assert.deepEqual(
    step.env,
    { BASE_SHA: "${{ github.event.pull_request.base.sha }}" },
    `${STEP_NAME}: env must be BASE_SHA only`,
  );
  return selected;
}

test("the step runs the module with BASE_SHA as its only input", () => {
  assertWiring(fs.readFileSync(WORKFLOW, "utf8"));
});

test("negative: the wiring check rejects another command and a changed env", () => {
  const source = fs.readFileSync(WORKFLOW, "utf8");
  const swap = (from: string, to: string) => source.replace(from, () => to);
  const cases = [
    [
      "the old inline script restored",
      swap(
        `        run: ${RUN}\n`,
        "        run: |\n          node <<'NODE'\n          NODE\n",
      ),
    ],
    [
      "a changed module path",
      swap(RUN, "node .github/tools/src/scripts/ci-version-field.ts"),
    ],
    [
      "a dropped BASE_SHA",
      swap(
        "          BASE_SHA: ${{ github.event.pull_request.base.sha }}\n        run: node .github/tools/src/scripts/ci-version-fields.ts",
        "        run: node .github/tools/src/scripts/ci-version-fields.ts",
      ),
    ],
    [
      "an extra env value",
      swap(
        "          BASE_SHA: ${{ github.event.pull_request.base.sha }}\n        run: node .github/tools/src/scripts/ci-version-fields.ts",
        "          BASE_SHA: ${{ github.event.pull_request.base.sha }}\n          OTHER: x\n        run: node .github/tools/src/scripts/ci-version-fields.ts",
      ),
    ],
  ] as const;
  for (const [name, mutated] of cases) {
    assert.notEqual(mutated, source, `${name}: the mutation changed nothing`);
    assert.throws(
      () => assertWiring(mutated),
      /Reject a changed version/,
      name,
    );
  }
});

const MANIFESTS = [
  ".claude-plugin/marketplace.json",
  ".claude-plugin/plugin.json",
  ".codex-plugin/plugin.json",
  ".github/plugin/plugin.json",
  ".antigravity-plugin/plugin.json",
];

// A repository whose one commit holds the four manifests at version 1.2.3.
function fixture(t: test.TestContext, name = "agentics") {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "agentics-vf-unit-"));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const git = (...args: string[]) =>
    fixtureGit(
      root,
      "-c",
      "user.email=t@example.invalid",
      "-c",
      "user.name=t",
      ...args,
    ).trim();
  git("init", "--quiet", "-b", "main");
  for (const rel of MANIFESTS) {
    fs.mkdirSync(path.dirname(path.join(root, rel)), { recursive: true });
    const doc = rel.endsWith("marketplace.json")
      ? { plugins: [{ name, version: "1.2.3" }] }
      : { name, version: "1.2.3" };
    fs.writeFileSync(path.join(root, rel), JSON.stringify(doc));
  }
  git("add", "-A");
  git("commit", "--quiet", "-m", "base");
  return { root, git, base: git("rev-parse", "HEAD") };
}

function check(root: string, baseSha: string | undefined) {
  // The exported check launches Git with its process environment. Give that
  // process the fixture environment while retaining the injected writer.
  const module = pathToFileURL(
    path.join(import.meta.dirname, "ci-version-fields.ts"),
  ).href;
  const source = `
const { checkVersionFields } = await import(${JSON.stringify(module)});
const lines = [];
const status = checkVersionFields({
  env: process.env,
  cwd: process.cwd(),
  write: (line) => lines.push(line),
});
process.stdout.write(JSON.stringify({ status, lines }));`;
  const result = spawnSync(
    process.execPath,
    ["--input-type=module", "--eval", source],
    {
      cwd: root,
      env: { ...fixtureGitEnv(), BASE_SHA: baseSha },
      encoding: "utf8",
    },
  );
  assert.ifError(result.error);
  assert.equal(result.status, 0, result.stderr);
  return JSON.parse(result.stdout) as { status: number; lines: string[] };
}

test("the read-only version job runs the version CLI in its checkout", (t) => {
  const selected = assertWiring(fs.readFileSync(WORKFLOW, "utf8"));
  assert.deepEqual(selected.job.permissions, { contents: "read" });
  assert.equal(selected.job.if, "github.event_name == 'pull_request'");
  const steps = selected.job.steps as WorkflowStepData[];
  const checkout = steps.find((step) =>
    step.uses?.startsWith("actions/checkout@"),
  );
  assert.ok(checkout);
  assert.ok(steps.indexOf(checkout) < steps.indexOf(selected.step));
  assert.deepEqual(checkout.with, {
    "persist-credentials": false,
    "fetch-depth": 0,
  });

  const { root, base, git } = fixture(t);
  const [, module] = selected.step.run!.trim().split(" ");
  const files = [
    module!,
    ".github/tools/src/plugin-manifests.ts",
    ".github/tools/package.json",
  ];
  for (const file of files)
    fs.cpSync(path.join(ROOT, file), path.join(root, file));
  git("add", "--", ...files);
  git("commit", "--quiet", "-m", "checked-out version command");
  assert.equal(
    fs.existsSync(path.join(root, ".github/tools/node_modules")),
    false,
  );
  const run = (status: number) => {
    const expected = check(root, base);
    const invoked = spawnSync(process.execPath, [module!], {
      cwd: root,
      env: { ...fixtureGitEnv(), BASE_SHA: base },
      encoding: "utf8",
    });
    assert.ifError(invoked.error);
    assert.equal(invoked.status, status);
    assert.equal(invoked.status, expected.status);
    assert.equal(invoked.stdout, expected.lines.join("\n") + "\n");
    assert.equal(invoked.stderr, "");
  };
  run(0);
  fs.writeFileSync(
    path.join(root, ".codex-plugin/plugin.json"),
    JSON.stringify({ name: "agentics", version: "1.3.0" }),
  );
  git("commit", "--quiet", "-am", "change a version");
  run(1);
});

test("an unchanged checkout exits 0 and says so", (t) => {
  const { root, base } = fixture(t);
  assert.deepEqual(check(root, base), {
    status: 0,
    lines: ["Version fields: unchanged."],
  });
});

test("a rename preserves the marketplace version check across revisions", (t) => {
  const { root, base, git } = fixture(t, "previous-plugin");
  const writeMarketplace = (version: string) => {
    fs.writeFileSync(
      path.join(root, ".claude-plugin/marketplace.json"),
      JSON.stringify({
        plugins: [
          { name: "previous-plugin", version: "1.2.3" },
          { name: "agentics", version },
        ],
      }),
    );
  };
  fs.writeFileSync(
    path.join(root, ".claude-plugin/plugin.json"),
    JSON.stringify({ name: "agentics", version: "1.2.3" }),
  );
  writeMarketplace("1.2.3");
  git("commit", "--quiet", "-am", "rename the plugin");
  assert.equal(check(root, base).status, 0);

  writeMarketplace("1.2.4");
  git("commit", "--quiet", "-am", "change the renamed marketplace version");
  const result = check(root, base);
  assert.equal(result.status, 1);
  assert.match(
    result.lines[0]!,
    /changes the plugin version from "1\.2\.3" to "1\.2\.4"/,
  );
});

test("a BASE_SHA that is unset or the wrong shape exits 1 as an error line", (t) => {
  const { root } = fixture(t);
  for (const value of [undefined, "", "main", "A".repeat(40), "a".repeat(39)]) {
    const { status, lines } = check(root, value);
    assert.equal(status, 1, String(value));
    assert.deepEqual(lines, [
      "::error::BASE_SHA must be a 40-character lowercase hex commit id.",
    ]);
  }
});

test("a changed version exits 1 and names the file and both versions", (t) => {
  const { root, git, base } = fixture(t);
  fs.writeFileSync(
    path.join(root, ".codex-plugin/plugin.json"),
    JSON.stringify({ name: "agentics", version: "1.3.0" }),
  );
  git("commit", "--quiet", "-am", "bump");
  const { status, lines } = check(root, base);
  assert.equal(status, 1);
  assert.equal(lines.length, 1);
  assert.match(
    lines[0]!,
    /^::error::\.codex-plugin\/plugin\.json changes the plugin version from "1\.2\.3" to "1\.3\.0"; only the release commit/,
  );
});

test("a base commit missing from a non-merge checkout exits 1", (t) => {
  const { root } = fixture(t);
  const { status, lines } = check(root, "a".repeat(40));
  assert.equal(status, 1);
  assert.match(lines[0]!, /^::error::The base commit a{40} is not in this/);
});

test("on a merge commit the baseline is the first parent, not BASE_SHA", (t) => {
  const { root, git } = fixture(t);
  git("checkout", "--quiet", "-b", "pr");
  fs.writeFileSync(path.join(root, "notes.txt"), "x");
  git("add", "-A");
  git("commit", "--quiet", "-m", "pull request");
  git("checkout", "--quiet", "main");
  fs.writeFileSync(
    path.join(root, ".claude-plugin/plugin.json"),
    JSON.stringify({ name: "agentics", version: "1.4.0" }),
  );
  git("commit", "--quiet", "-am", "main moves on");
  git("checkout", "--quiet", "-b", "merge-ref");
  git("merge", "--quiet", "--no-ff", "-m", "merge", "pr");
  // A BASE_SHA absent from the checkout would fail a non-merge run, so passing proves it is not consulted.
  assert.deepEqual(check(root, "a".repeat(40)), {
    status: 0,
    lines: ["Version fields: unchanged."],
  });
});
