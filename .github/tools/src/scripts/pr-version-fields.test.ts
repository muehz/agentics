import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import {
  fixtureGitEnv,
  git as fixtureGit,
} from "../conformance/testing/git-checkout.ts";
import { lookupWorkflowStep } from "./testing/workflow-steps.ts";

interface ManifestFixture {
  name?: string;
  version?: string;
  metadata?: { version: string };
  plugins?: { name: string; version?: string; description?: string }[];
  description?: string;
}
interface RepoFixture {
  root: string;
  base: string;
  prTip?: string;
}
type Change = (root: string) => void;
interface RunResult {
  status: number | null;
  stdout: string;
  stderr: string;
}
const ROOT = path.resolve(import.meta.dirname, "../../../..");
const WORKFLOW = ".github/workflows/validate.yml";
const versionStep = lookupWorkflowStep(
  fs.readFileSync(path.join(ROOT, WORKFLOW), "utf8"),
  WORKFLOW,
  "pr-version-fields",
  "Reject a changed version field",
);
const runCommand =
  /^node (\.github\/tools\/src\/scripts\/ci-version-fields\.ts)$/.exec(
    versionStep.step.run?.trim() ?? "",
  );
assert.ok(
  runCommand?.[1],
  "the version-field job must run the checkout's version module",
);
const MODULE = path.join(ROOT, runCommand[1]);
const script = fs.readFileSync(MODULE, "utf8");

const MANIFESTS = [
  ".claude-plugin/marketplace.json",
  ".claude-plugin/plugin.json",
  ".codex-plugin/plugin.json",
  ".github/plugin/plugin.json",
  ".antigravity-plugin/plugin.json",
];
const ANTIGRAVITY = ".antigravity-plugin/plugin.json";
const BASE_MANIFESTS = MANIFESTS.slice(0, 4);

const REJECTED = "only the release commit changes the plugin version";
const DELETED = "is deleted by this pull request";
const ADDED = "is added by this pull request with plugin version";
const NOT_OBJECT = "is not a JSON object";
const BAD_SHA = "BASE_SHA must be a 40-character lowercase hex commit id";
const UNKNOWN_BASE = "is not in this checkout";

const git = (cwd: string, ...args: string[]): string =>
  fixtureGit(
    cwd,
    "-c",
    "user.email=t@example.invalid",
    "-c",
    "user.name=t",
    ...args,
  ).trim();

const roots: string[] = [];
test.after(() => {
  for (const root of roots) {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

const isMarketplace = (rel: string): boolean =>
  path.basename(rel) === "marketplace.json";

function manifest(rel: string): ManifestFixture {
  return isMarketplace(rel)
    ? {
        metadata: { version: "0.0.0" },
        plugins: [{ name: "agentics", version: "1.2.3" }],
      }
    : { name: "agentics", version: "1.2.3" };
}

const readJson = (root: string, rel: string): ManifestFixture =>
  JSON.parse(fs.readFileSync(path.join(root, rel), "utf8"));
const writeJson = (root: string, rel: string, doc: unknown): void =>
  fs.writeFileSync(path.join(root, rel), `${JSON.stringify(doc, null, 4)}\n`);

// Creates a repository whose base commit holds the given manifests (all five by default).
function initRepo(
  prepare?: Change,
  manifests: readonly string[] = MANIFESTS,
): RepoFixture {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "agentics-vf-"));
  roots.push(root);
  git(root, "init", "--quiet", "-b", "main");
  for (const rel of manifests) {
    fs.mkdirSync(path.dirname(path.join(root, rel)), { recursive: true });
    writeJson(root, rel, manifest(rel));
  }
  if (prepare) {
    prepare(root);
  }
  git(root, "add", "-A");
  git(root, "commit", "--quiet", "-m", "base");
  return { root, base: git(root, "rev-parse", "HEAD") };
}

function commitAll(root: string, message: string): void {
  git(root, "add", "-A");
  git(root, "commit", "--quiet", "--allow-empty", "-m", message);
}

// Applies `change` on top of the base commit, then each `then` step as its
// own commit, and returns the base commit id the check compares against.
function repo(
  change: Change,
  {
    prepare,
    then = [],
    manifests = MANIFESTS,
  }: {
    prepare?: Change;
    then?: Change[];
    manifests?: readonly string[];
  } = {},
): RepoFixture {
  const fx = initRepo(prepare, manifests);
  change(fx.root);
  commitAll(fx.root, "change");
  for (const step of then) {
    step(fx.root);
    commitAll(fx.root, "then");
  }
  return fx;
}

// Builds the shape of a merge ref: HEAD is a merge commit whose first parent
// is a base tip that moved past `base`, and whose second parent is the pull
// request. `base` stays the stale commit a re-run's payload would carry.
function mergeRepo({
  main,
  pr,
}: {
  main: Change;
  pr: Change;
}): RepoFixture & { prTip: string } {
  const fx = initRepo();
  git(fx.root, "checkout", "--quiet", "-b", "pr");
  pr(fx.root);
  commitAll(fx.root, "pull request");
  const prTip = git(fx.root, "rev-parse", "HEAD");
  git(fx.root, "checkout", "--quiet", "main");
  main(fx.root);
  commitAll(fx.root, "main moves on");
  git(fx.root, "checkout", "--quiet", "-b", "merge-ref");
  git(fx.root, "merge", "--quiet", "--no-ff", "-m", "merge", "pr");
  return { ...fx, prTip };
}

function run(
  fx: RepoFixture,
  {
    source = MODULE,
    base = fx.base,
    env = {},
  }: { source?: string; base?: string; env?: NodeJS.ProcessEnv } = {},
) {
  const result = spawnSync(process.execPath, [source], {
    cwd: fx.root,
    env: { ...fixtureGitEnv(), BASE_SHA: base, ...env },
    encoding: "utf8",
    shell: false,
  });
  assert.ifError(result.error);
  return result;
}

const assertRejects = (result: RunResult, message: string): void => {
  assert.equal(result.status, 1, result.stdout + result.stderr);
  assert.ok(result.stdout.includes(message), result.stdout + result.stderr);
};
const assertPasses = (result: RunResult): void => {
  assert.equal(result.status, 0, result.stdout + result.stderr);
  assert.ok(
    result.stdout.includes("Version fields: unchanged."),
    result.stdout,
  );
};

// Keep the actual relative import layout while leaving node_modules absent.
const copies = fs.mkdtempSync(path.join(os.tmpdir(), "agentics-vf-copy-"));
test.after(() => fs.rmSync(copies, { recursive: true, force: true }));
const copyScripts = path.join(copies, ".github", "tools", "src", "scripts");
fs.mkdirSync(copyScripts, { recursive: true });
for (const rel of ["../plugin-manifests.ts", "../../package.json"]) {
  fs.copyFileSync(
    path.join(import.meta.dirname, rel),
    path.join(copyScripts, rel),
  );
}
let copyCount = 0;
function writeScriptCopy(source: string): string {
  const file = path.join(copyScripts, `copy-${copyCount++}.ts`);
  fs.writeFileSync(file, source);
  return file;
}

// Replaces exactly one occurrence so a mutation that stops applying fails loudly.
function mutate(source: string, from: string, to: string): string {
  assert.equal(
    source.split(from).length,
    2,
    `expected exactly one occurrence of: ${from}`,
  );
  return writeScriptCopy(source.replace(from, () => to));
}

const bumpVersion =
  (rel: string): Change =>
  (root) => {
    const doc = readJson(root, rel);
    if (isMarketplace(rel)) {
      doc.plugins![0]!.version = "1.3.0";
    } else {
      doc.version = "1.3.0";
    }
    writeJson(root, rel, doc);
  };

const deleteManifest =
  (rel: string): Change =>
  (root) => {
    fs.rmSync(path.join(root, rel));
  };

const corrupt =
  (rel: string, text: string): Change =>
  (root) => {
    fs.writeFileSync(path.join(root, rel), text);
  };

test("the copied version check keeps selector-only changes outside its policy", () => {
  const selectorOnly = ".agents/plugins/marketplace.json";
  const source = writeScriptCopy(script);
  const options = { manifests: [...MANIFESTS, selectorOnly] };
  assertPasses(run(repo(bumpVersion(selectorOnly), options), { source }));
  assertRejects(
    run(repo(bumpVersion(ANTIGRAVITY), options), { source }),
    REJECTED,
  );
});

for (const rel of MANIFESTS) {
  test(`a changed version in ${rel} fails`, () => {
    const result = run(repo(bumpVersion(rel)));
    assertRejects(result, REJECTED);
    assert.ok(result.stdout.includes(rel), result.stdout);
  });
}

for (const rel of MANIFESTS) {
  test(`a changed description in ${rel} passes`, () => {
    assertPasses(
      run(
        repo((root) => {
          const doc = readJson(root, rel);
          doc.description = "new words";
          if (isMarketplace(rel)) {
            doc.plugins![0]!.description = "new words";
          }
          writeJson(root, rel, doc);
        }),
      ),
    );
  });
}

test("a pull request that touches no manifest passes", () => {
  const fx = repo((root) => {
    fs.writeFileSync(path.join(root, "package.json"), '{"version":"9.9.9"}\n');
  });
  assertPasses(run(fx));
});

test("an empty change passes", () => {
  assertPasses(run(repo(() => {})));
});

test("a changed marketplace metadata.version is not a plugin version change", () => {
  const rel = ".claude-plugin/marketplace.json";
  const fx = repo((root) => {
    const doc = readJson(root, rel);
    doc.metadata!.version = "0.1.0";
    writeJson(root, rel, doc);
  });
  assertPasses(run(fx));
});

const addOtherPlugin: Change = (root) => {
  const rel = ".claude-plugin/marketplace.json";
  const doc = readJson(root, rel);
  doc.plugins!.push({ name: "other", version: "9.9.9" });
  writeJson(root, rel, doc);
};

test("a new plugins entry other than agentics passes", () => {
  assertPasses(run(repo(addOtherPlugin)));
});

test("the agentics entry is selected by name, not by position", () => {
  const mutated = mutate(
    script,
    ".find(\n        (plugin) => isObject(plugin) && plugin.name === name,\n      )",
    ".at(-1)",
  );
  const fx = repo(addOtherPlugin);
  assertPasses(run(fx));
  assert.throws(
    () => assertPasses(run(fx, { source: mutated })),
    assert.AssertionError,
  );
});

for (const rel of MANIFESTS) {
  test(`a deleted ${rel} fails`, () => {
    assertRejects(run(repo(deleteManifest(rel))), DELETED);
  });
}

test("a deleted manifest fails on its own check, not only on the version comparison", () => {
  const mutated = mutate(
    script,
    "if (before.exists && !after.exists) {",
    "if (false) {",
  );
  const fx = repo(deleteManifest(".codex-plugin/plugin.json"));
  assertRejects(run(fx), DELETED);
  assert.throws(
    () => assertRejects(run(fx, { source: mutated }), DELETED),
    assert.AssertionError,
  );
});

for (const [label, text] of [
  ["malformed JSON", "{ not json"],
  ["an empty file", ""],
  ["a JSON array", "[]"],
  ["JSON null", "null"],
] as const) {
  test(`${label} in a manifest fails with a message, not a stack trace`, () => {
    const result = run(repo(corrupt(".codex-plugin/plugin.json", text)));
    assertRejects(result, NOT_OBJECT);
    assert.equal(result.stderr, "");
  });
}

test("a manifest that was already malformed at the base fails closed", () => {
  const fx = repo(() => {}, {
    prepare: corrupt(".github/plugin/plugin.json", "{ not json"),
  });
  assertRejects(run(fx), NOT_OBJECT);
});

test("the unreadable-manifest rejection dies when its check is removed", () => {
  const mutated = mutate(script, "if (!isObject(doc)) {", "if (false) {");
  const fx = repo(corrupt(".codex-plugin/plugin.json", "{ not json"));
  assertRejects(run(fx), NOT_OBJECT);
  assert.throws(
    () => assertRejects(run(fx, { source: mutated }), NOT_OBJECT),
    assert.AssertionError,
  );
});

for (const [label, base] of [
  ["a branch name", "main"],
  ["HEAD", "HEAD"],
  ["an empty value", ""],
  ["upper-case hex", "A".repeat(40)],
  ["a short id", "a".repeat(39)],
  ["a long id", "a".repeat(41)],
  ["an option", "--output=x"],
  ["a revision expression", `${"a".repeat(38)}~1`],
] as const) {
  test(`a BASE_SHA of the wrong shape fails (${label})`, () => {
    assertRejects(
      run(
        repo(() => {}),
        { base },
      ),
      BAD_SHA,
    );
  });
}

test("the BASE_SHA shape rejection dies when its check is removed", () => {
  const mutated = mutate(
    script,
    "if (!/^[0-9a-f]{40}$/.test(baseSha)) {",
    "if (false) {",
  );
  const fx = repo(() => {});
  assertRejects(run(fx, { base: "main" }), BAD_SHA);
  assert.throws(
    () => assertRejects(run(fx, { source: mutated, base: "main" }), BAD_SHA),
    assert.AssertionError,
  );
});

test("a well-formed BASE_SHA that is not in the checkout fails", () => {
  assertRejects(
    run(
      repo(() => {}),
      { base: "a".repeat(40) },
    ),
    UNKNOWN_BASE,
  );
});

test("the missing-base rejection dies when its check is removed", () => {
  const mutated = mutate(script, "if (!baseKnown) {", "if (false) {");
  const fx = repo(() => {});
  const base = "a".repeat(40);
  assertRejects(run(fx, { base }), UNKNOWN_BASE);
  assert.throws(
    () => assertRejects(run(fx, { source: mutated, base }), UNKNOWN_BASE),
    assert.AssertionError,
  );
});

test("the rejection dies when the comparison is removed", () => {
  const mutated = mutate(
    script,
    "if (JSON.stringify(before.version) !== JSON.stringify(after.version)) {",
    "if (false) {",
  );
  const fx = repo(bumpVersion(".claude-plugin/plugin.json"));
  assertRejects(run(fx), REJECTED);
  const result = run(fx, { source: mutated });
  assert.equal(result.status, 0, result.stdout + result.stderr);
});

// A merge ref carries the base tip as its first parent. The payload's base
// sha is older once the base branch has advanced, and must not be blamed on
// the pull request.
const mainBumpsClaude: Change = (root) =>
  bumpVersion(".claude-plugin/plugin.json")(root);
const prDescribesCodex: Change = (root) => {
  const doc = readJson(root, ".codex-plugin/plugin.json");
  doc.description = "new words";
  writeJson(root, ".codex-plugin/plugin.json", doc);
};
const FORCE_FALLBACK = [
  "const mergeBuilt = parentCount === 2;",
  "const mergeBuilt = false;",
] as const;

test("a merge ref is compared against its first parent, not a stale BASE_SHA", () => {
  const fx = mergeRepo({ main: mainBumpsClaude, pr: prDescribesCodex });
  assertPasses(run(fx));
});

test("the merge-ref baseline dies when the fallback is always used", () => {
  const fx = mergeRepo({ main: mainBumpsClaude, pr: prDescribesCodex });
  const mutated = mutate(script, ...FORCE_FALLBACK);
  assert.throws(
    () => assertPasses(run(fx, { source: mutated })),
    assert.AssertionError,
  );
});

test("a version change inside a merge ref fails against its first parent", () => {
  const rel = ".github/plugin/plugin.json";
  const fx = mergeRepo({ main: mainBumpsClaude, pr: bumpVersion(rel) });
  // The pull request's own tip makes the fallback comparison equal for this
  // file, so only the first-parent baseline attributes the change to it.
  assertRejects(
    run(fx, { base: fx.prTip }),
    `${rel} changes the plugin version`,
  );
});

test("the merge-ref rejection dies when the fallback is always used", () => {
  const rel = ".github/plugin/plugin.json";
  const fx = mergeRepo({ main: mainBumpsClaude, pr: bumpVersion(rel) });
  const mutated = mutate(script, ...FORCE_FALLBACK);
  assert.throws(
    () =>
      assertRejects(
        run(fx, { source: mutated, base: fx.prTip }),
        `${rel} changes the plugin version`,
      ),
    assert.AssertionError,
  );
});

test("without a merge commit the baseline is BASE_SHA, not HEAD^1", () => {
  const rel = ".claude-plugin/plugin.json";
  const fx = repo(bumpVersion(rel), {
    then: [
      (root) => {
        const doc = readJson(root, ".codex-plugin/plugin.json");
        doc.description = "new words";
        writeJson(root, ".codex-plugin/plugin.json", doc);
      },
    ],
  });
  const result = run(fx);
  assertRejects(result, REJECTED);
  assert.ok(result.stdout.includes(rel), result.stdout);
});

test("the non-merge baseline dies when HEAD^1 is always used", () => {
  const fx = repo(bumpVersion(".claude-plugin/plugin.json"), {
    then: [
      (root) => {
        fs.writeFileSync(path.join(root, "notes.txt"), "x");
      },
    ],
  });
  const mutated = mutate(
    script,
    'const baseRev = mergeBuilt ? "HEAD^1" : baseSha;',
    'const baseRev = "HEAD^1";',
  );
  assertRejects(run(fx), REJECTED);
  assert.equal(run(fx, { source: mutated }).status, 0);
});

test("a merge ref does not need the payload's base commit in the checkout", () => {
  const fx = mergeRepo({ main: mainBumpsClaude, pr: prDescribesCodex });
  assertPasses(run(fx, { base: "a".repeat(40) }));
});

// A git failure must stop the job. Reading it as "no such file" on both sides
// would let any version edit pass.
const SWALLOW_GIT_ERRORS = [
  "return fail(\n        `git ${args[0]} failed: ${String(failure.stderr || failure.message).trim()}`,\n      );",
  'return "";',
] as const;

// Removes the loose object a revision's path resolves to, leaving the commit
// readable so only the later git call can notice.
function dropObject(root: string, spec: string): void {
  const id = git(root, "rev-parse", spec);
  const file = path.join(root, ".git", "objects", id.slice(0, 2), id.slice(2));
  fs.chmodSync(file, 0o666);
  fs.rmSync(file);
}

test("a directory that is not a git repository fails with the git error", () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "agentics-vf-"));
  roots.push(root);
  const fx = { root, base: "a".repeat(40) };
  const env = { GIT_CEILING_DIRECTORIES: os.tmpdir() };
  const result = run(fx, { env });
  assertRejects(result, "git rev-list failed:");
  const mutated = mutate(script, ...SWALLOW_GIT_ERRORS);
  assert.throws(
    () =>
      assertRejects(run(fx, { source: mutated, env }), "git rev-list failed:"),
    assert.AssertionError,
  );
});

test("an unreadable base tree fails instead of reading as no manifests", () => {
  const fx = repo(() => {});
  dropObject(fx.root, `${fx.base}^{tree}`);
  assertRejects(run(fx), "git ls-tree failed:");
  const mutated = mutate(script, ...SWALLOW_GIT_ERRORS);
  const result = run(fx, { source: mutated });
  assert.equal(result.status, 0, "the swallowed error is fail-open");
  assert.throws(
    () => assertRejects(result, "git ls-tree failed:"),
    assert.AssertionError,
  );
});

test("an unreadable manifest object fails with the git error", () => {
  const fx = repo(() => {});
  dropObject(fx.root, `${fx.base}:.codex-plugin/plugin.json`);
  assertRejects(run(fx), "git show failed:");
  const mutated = mutate(script, ...SWALLOW_GIT_ERRORS);
  assert.throws(
    () => assertRejects(run(fx, { source: mutated }), "git show failed:"),
    assert.AssertionError,
  );
});

test("a base commit missing from the checkout also fails through the git helper", () => {
  const fx = repo(() => {});
  const noKnownCheck = mutate(script, "if (!baseKnown) {", "if (false) {");
  assertRejects(
    run(fx, { source: noKnownCheck, base: "a".repeat(40) }),
    "git ls-tree failed:",
  );
});

test("a pull request that removes the agentics entry reports the version as absent", () => {
  const rel = ".claude-plugin/marketplace.json";
  const fx = repo((root) => {
    const doc = readJson(root, rel);
    doc.plugins = [];
    writeJson(root, rel, doc);
  });
  const expected = `${rel} changes the plugin version from "1.2.3" to absent`;
  assertRejects(run(fx), expected);
  const mutated = mutate(
    script,
    "version: entry && entry.version !== undefined ? entry.version : null",
    "version: entry.version !== undefined ? entry.version : null",
  );
  assert.throws(
    () => assertRejects(run(fx, { source: mutated }), expected),
    assert.AssertionError,
  );
});

test("a pull request that removes the plugins list reports the version as absent", () => {
  const rel = ".claude-plugin/marketplace.json";
  const fx = repo((root) => {
    const doc = readJson(root, rel);
    delete doc.plugins;
    writeJson(root, rel, doc);
  });
  const expected = `${rel} changes the plugin version from "1.2.3" to absent`;
  assertRejects(run(fx), expected);
  const mutated = mutate(
    script,
    "(Array.isArray(doc.plugins) ? doc.plugins : [])",
    "(doc.plugins)",
  );
  assert.throws(
    () => assertRejects(run(fx, { source: mutated }), expected),
    assert.AssertionError,
  );
});

test("a pull request that removes the version key reports it as absent", () => {
  const rel = ".codex-plugin/plugin.json";
  const fx = repo((root) => {
    const doc = readJson(root, rel);
    delete doc.version;
    writeJson(root, rel, doc);
  });
  const expected = `${rel} changes the plugin version from "1.2.3" to absent`;
  assertRejects(run(fx), expected);
  const mutated = mutate(
    script,
    "version: entry && entry.version !== undefined ? entry.version : null",
    "version: entry ? entry.version : null",
  );
  assert.throws(
    () => assertRejects(run(fx, { source: mutated }), expected),
    assert.AssertionError,
  );
});

const createManifest =
  (rel: string, doc: unknown): Change =>
  (root) => {
    fs.mkdirSync(path.dirname(path.join(root, rel)), { recursive: true });
    writeJson(root, rel, doc);
  };

test("a manifest absent at both base and HEAD passes", () => {
  const fx = repo(
    (root) => {
      fs.writeFileSync(path.join(root, "README.md"), "change\n");
    },
    { manifests: BASE_MANIFESTS },
  );
  assertPasses(run(fx));
});

test("initial creation of a new manifest matching unchanged .claude-plugin/plugin.json passes", () => {
  const fx = repo(
    createManifest(ANTIGRAVITY, { name: "agentics", version: "1.2.3" }),
    { manifests: BASE_MANIFESTS },
  );
  assertPasses(run(fx));
});

test("initial creation of a new manifest with a mismatched version fails", () => {
  const fx = repo(
    createManifest(ANTIGRAVITY, { name: "agentics", version: "1.3.0" }),
    { manifests: BASE_MANIFESTS },
  );
  const result = run(fx);
  assertRejects(result, ADDED);
  assert.ok(result.stdout.includes(ANTIGRAVITY), result.stdout);
  assert.ok(result.stdout.includes('"1.3.0"'), result.stdout);
});

test("initial creation of a new manifest with an absent version fails", () => {
  const fx = repo(createManifest(ANTIGRAVITY, { name: "agentics" }), {
    manifests: BASE_MANIFESTS,
  });
  const result = run(fx);
  assertRejects(result, ADDED);
  assert.ok(result.stdout.includes(ANTIGRAVITY), result.stdout);
  assert.ok(result.stdout.includes("absent"), result.stdout);
});

for (const [label, version] of [
  ["a number", 123],
  ["a boolean", true],
  ["an array", ["1.2.3"]],
] as const) {
  test(`initial creation of a new manifest with ${label} version fails`, () => {
    const fx = repo(
      createManifest(ANTIGRAVITY, { name: "agentics", version }),
      {
        manifests: BASE_MANIFESTS,
      },
    );
    const result = run(fx);
    assertRejects(result, ADDED);
    assert.ok(result.stdout.includes(ANTIGRAVITY), result.stdout);
  });
}

test("initial creation of .antigravity-plugin/plugin.json fails when .claude-plugin/plugin.json also changes", () => {
  for (const newVersion of ["1.2.3", "1.3.0"]) {
    const fx = repo(
      (root) => {
        bumpVersion(".claude-plugin/plugin.json")(root);
        createManifest(ANTIGRAVITY, { name: "agentics", version: newVersion })(
          root,
        );
      },
      { manifests: BASE_MANIFESTS },
    );
    // Index 1 (.claude-plugin/plugin.json) precedes index 4 (.antigravity-plugin/plugin.json)
    // in MANIFESTS, so the canonical manifest edit rejects first.
    const result = run(fx);
    assertRejects(result, REJECTED);
    assert.ok(
      result.stdout.includes(".claude-plugin/plugin.json"),
      result.stdout,
    );
  }
});

test("initial creation before .claude-plugin/plugin.json in MANIFESTS fails when canonical version changes", () => {
  const marketplace = ".claude-plugin/marketplace.json";
  const fx = repo(
    (root) => {
      bumpVersion(".claude-plugin/plugin.json")(root);
      createManifest(marketplace, manifest(marketplace))(root);
    },
    { manifests: MANIFESTS.slice(1) },
  );
  const result = run(fx);
  assertRejects(result, ADDED);
  assert.ok(result.stdout.includes(marketplace), result.stdout);
  const mutated = mutate(
    script,
    "canonicalBefore.version === canonicalAfter.version",
    "true",
  );
  assert.throws(
    () => assertRejects(run(fx, { source: mutated }), ADDED),
    assert.AssertionError,
  );
});

test("the initial-creation check dies when its branch is removed", () => {
  const mutated = mutate(
    script,
    "if (!before.exists && after.exists) {",
    "if (false) {",
  );
  const validFx = repo(
    createManifest(ANTIGRAVITY, { name: "agentics", version: "1.2.3" }),
    { manifests: BASE_MANIFESTS },
  );
  assertPasses(run(validFx));
  assert.throws(
    () => assertPasses(run(validFx, { source: mutated })),
    assert.AssertionError,
  );

  // Without the explicit fail inside the initial-creation branch, an absent
  // version would compare null === null and pass fail-open.
  const absentFx = repo(createManifest(ANTIGRAVITY, { name: "agentics" }), {
    manifests: BASE_MANIFESTS,
  });
  assertRejects(run(absentFx), ADDED);
  assertPasses(run(absentFx, { source: mutated }));
});

test("the initial-creation version-match rejection dies when its equality check is removed", () => {
  const mutated = mutate(
    script,
    "after.version === canonicalBefore.version",
    "true",
  );
  const fx = repo(
    createManifest(ANTIGRAVITY, { name: "agentics", version: "1.3.0" }),
    { manifests: BASE_MANIFESTS },
  );
  assertRejects(run(fx), ADDED);
  assertPasses(run(fx, { source: mutated }));
});
