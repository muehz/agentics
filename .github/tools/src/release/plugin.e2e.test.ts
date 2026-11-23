// The end-to-end proof of the semantic-release path. It runs the
// semantic-release command lines it reads out of .github/workflows/release.md,
// so it cannot pass when the workflow does not wire them. Each fixture
// repository has a local bare origin; nothing here touches GitHub.
//
// It needs the pinned semantic-release install, which the plain tools install
// does not provide: run `npm ci --ignore-scripts --prefix .github/release`.
import assert from "node:assert/strict";
import { execFileSync, spawnSync } from "node:child_process";
import {
  appendFileSync,
  copyFileSync,
  cpSync,
  existsSync,
  mkdirSync,
  readFileSync,
  rmSync,
  symlinkSync,
  unlinkSync,
  writeFileSync,
} from "node:fs";
import { basename, dirname, join, relative, sep } from "node:path";
import { after, test } from "node:test";
import { pathToFileURL } from "node:url";
import { fixtureGitEnv } from "../conformance/testing/git-checkout.ts";
import { initRepo, prerequisite, scratchDir } from "./testing/support.ts";

const repoRoot = join(import.meta.dirname, "../../../..");
const releaseDir = join(repoRoot, ".github/release");
const toolsRelease = join(repoRoot, ".github/tools/src/release");
const WORKFLOW = readFileSync(
  join(repoRoot, ".github/workflows/release.md"),
  "utf8",
);

const skip = prerequisite(
  ".github/release/node_modules",
  existsSync(join(releaseDir, "node_modules")),
  "run: npm ci --ignore-scripts --prefix .github/release",
);

const SEMANTIC_RELEASE =
  "node .github/release/node_modules/semantic-release/bin/semantic-release.js";
// The level job points its dry run at its own checkout through this word.
const WORKSPACE_URL_WORD = '"file://$GITHUB_WORKSPACE"';
const VERSION_MANIFESTS = [
  ".claude-plugin/marketplace.json",
  ".claude-plugin/plugin.json",
  ".codex-plugin/plugin.json",
  ".github/plugin/plugin.json",
  ".antigravity-plugin/plugin.json",
];
const PLUGIN_PATH = "./.github/tools/src/release/plugin.ts";
const EXPECTED_PLUGINS = [
  "@semantic-release/commit-analyzer",
  "@semantic-release/release-notes-generator",
  PLUGIN_PATH,
  "@semantic-release/git",
];

// A CI runner's environment would change what the children do: env-ci reads
// the runner's branch and pull-request state from GITHUB_* and CI, and
// set-version-style commands write to GITHUB_OUTPUT instead of standard output.
// The approval flag and tokens must come only from a scenario. GIT_* goes too,
// so a test run from inside a git hook cannot aim the fixture's git at the real
// repository. Everything else, including PATH and HOME, stays.
const DROPPED_KEYS = new Set([
  "AGX_MAJOR_APPROVED",
  "GH_TOKEN",
  "GITHUB_TOKEN",
]);
const DROPPED_PREFIXES = ["GITHUB_", "RUNNER_", "CI", "GIT_"];

type Env = Record<string, string | undefined>;

function scrubbedEnv(env: Env): Record<string, string> {
  const kept: Record<string, string> = {};
  for (const [key, value] of Object.entries(env)) {
    const upper = key.toUpperCase();
    if (
      value !== undefined &&
      !DROPPED_KEYS.has(upper) &&
      !DROPPED_PREFIXES.some((prefix) => upper.startsWith(prefix))
    ) {
      kept[key] = value;
    }
  }
  return kept;
}

// Returns the raw shell words of the one semantic-release command in a step.
// The step ends at the first later line indented no deeper than its own
// "- name:", which is the next step, a comment between steps, or the next job,
// so a later step's command can never be read as this one's.
function commandWords(text: string, job: string, stepName: string): string[] {
  const lines = text.split("\n");
  const jobsAt = lines.indexOf("jobs:");
  assert.notEqual(jobsAt, -1, "release.md has no top-level jobs key");
  const jobAt = lines.indexOf(`    ${job}:`, jobsAt);
  assert.notEqual(jobAt, -1, `release.md has no ${job} job`);
  let jobEnd = lines.findIndex(
    (line, i) => i > jobAt && /^ {0,4}\S/u.test(line),
  );
  if (jobEnd === -1) jobEnd = lines.length;
  const jobLines = lines.slice(jobAt + 1, jobEnd);
  const isStep = (line: string): boolean =>
    line.trim() === `- name: ${stepName}`;
  const stepAt = jobLines.findIndex(isStep);
  assert.notEqual(stepAt, -1, `${job} has no step "${stepName}"`);
  assert.equal(
    jobLines.filter(isStep).length,
    1,
    `${job} names more than one step "${stepName}"`,
  );
  const indent = (jobLines[stepAt] ?? "").indexOf("-");
  let stepEnd = jobLines.findIndex(
    (line, i) =>
      i > stepAt && line.trim() !== "" && line.search(/\S/u) <= indent,
  );
  if (stepEnd === -1) stepEnd = jobLines.length;
  const found = jobLines
    .slice(stepAt, stepEnd)
    .map((line) => line.trim())
    .filter(
      (line) =>
        line === SEMANTIC_RELEASE || line.startsWith(`${SEMANTIC_RELEASE} `),
    );
  assert.equal(
    found.length,
    1,
    `step "${stepName}" must run exactly one semantic-release command`,
  );
  return shellWords(found[0] ?? "");
}

// Splits a command line into raw shell words. Only plain words and whole
// quoted spans are understood; anything that would make a shell do more than
// split words is refused rather than guessed at.
function shellWords(line: string): string[] {
  const words = line.match(/(?:"[^"]*"|'[^']*'|[^\s"'])+/gu) ?? [];
  // A stray quote matches no word pattern, so the words then cover less of
  // the line than the line itself.
  assert.equal(
    words.join("").replace(/\s+/gu, ""),
    line.replace(/\s+/gu, ""),
    `the command line has an unterminated quote: ${line}`,
  );
  for (const word of words) {
    assert.doesNotMatch(
      word,
      /[\\|;&<>`#]/u,
      `the command line has unsupported shell syntax: ${word}`,
    );
  }
  return words;
}

// Turns raw shell words into arguments. A word listed in substitutions is
// replaced whole; any other word that would expand a variable is refused, so
// a shell expansion is never passed through as literal text.
function resolveWords(
  words: readonly string[],
  substitutions: Readonly<Record<string, string>> = {},
): string[] {
  return words.map((word) => {
    const substitute = substitutions[word];
    if (Object.hasOwn(substitutions, word) && substitute !== undefined) {
      return substitute;
    }
    assert.doesNotMatch(
      word.replace(/'[^']*'/gu, ""),
      /\$/u,
      `the command line has an unexpanded shell word: ${word}`,
    );
    return word.replace(/"([^"]*)"|'([^']*)'/gu, "$1$2");
  });
}

function releaseArgs(text: string = WORKFLOW): string[] {
  const words = commandWords(text, "release", "Run semantic-release");
  assert.equal(words[0], "node");
  return resolveWords(words).slice(1);
}

function levelArgs(fx: { work: string }, text: string = WORKFLOW): string[] {
  const words = commandWords(text, "level", "Compute the level");
  assert.equal(words[0], "node");
  assert.ok(
    words.includes("--dry-run"),
    "the level command line has no --dry-run",
  );
  const at = words.indexOf("--repository-url");
  assert.notEqual(at, -1, "the level command line has no --repository-url");
  assert.equal(
    words[at + 1],
    WORKSPACE_URL_WORD,
    "the level command line must point --repository-url at its checkout",
  );
  return resolveWords(words, {
    [WORKSPACE_URL_WORD]: pathToFileURL(fx.work).href,
  }).slice(1);
}

const links: string[] = [];
const binDir = scratchDir("agentics-release-bin-");

// Every git in a fixture, including the ones semantic-release starts, reads an
// empty global and system configuration, so a developer's hooksPath, template
// directory, or signing setting cannot reach a fixture commit. Git on some
// Windows hosts rejects the null device here, so this is a real empty file.
const emptyConfig = join(binDir, "empty.gitconfig");
writeFileSync(emptyConfig, "");
const isolatedEnv = (inherited: Env): Record<string, string> => ({
  ...fixtureGitEnv(scrubbedEnv(inherited)),
  GIT_CONFIG_GLOBAL: emptyConfig,
  GIT_CONFIG_SYSTEM: emptyConfig,
});

const git = (cwd: string, ...args: string[]): string =>
  execFileSync("git", args, {
    cwd,
    encoding: "utf8",
    env: {
      ...isolatedEnv(process.env),
      GIT_AUTHOR_NAME: "fixture",
      GIT_AUTHOR_EMAIL: "fixture@example.invalid",
      GIT_COMMITTER_NAME: "fixture",
      GIT_COMMITTER_EMAIL: "fixture@example.invalid",
    },
    stdio: ["ignore", "pipe", "pipe"],
  }).trim();

// Each fixture links node_modules to the real install. Unlinking it before the
// scratch cleanup removes the tree keeps the remove from reaching through the
// link.
after(() => {
  for (const link of links) {
    try {
      unlinkSync(link);
    } catch {
      // The link is absent when the fixture failed before making it.
    }
  }
});

function manifest(version: string, marketplace: boolean): string {
  if (marketplace) {
    return `${JSON.stringify(
      {
        metadata: { version: "0.0.0" },
        plugins: [{ name: "agentics", version }],
      },
      null,
      4,
    )}\n`;
  }
  return `${JSON.stringify({ name: "agentics", version }, null, 4)}\n`;
}

type Plugins = (string | [string, ...unknown[]])[];
interface Rc {
  plugins: Plugins;
  [key: string]: unknown;
}

// The configuration exactly as shipped, so a fixture runs whatever plugin
// list the release runs and assertShippedPlugins refuses any other.
function shippedRc(): Rc {
  return JSON.parse(
    readFileSync(join(releaseDir, ".releaserc.json"), "utf8"),
  ) as Rc;
}

const pluginName = (p: string | [string, ...unknown[]]): string =>
  Array.isArray(p) ? p[0] : p;

function analyzerEntry(config: Rc): [string, { releaseRules?: unknown }] {
  const entry = config.plugins.find(
    (p) => Array.isArray(p) && p[0] === "@semantic-release/commit-analyzer",
  );
  assert.ok(Array.isArray(entry), "the shipped analyzer entry has options");
  return entry as [string, { releaseRules?: unknown }];
}

// Refuses a fixture whose configuration loads anything beyond the four
// shipped plugins, so no scenario can pass on a publishing plugin.
function assertShippedPlugins(rcText: string): void {
  const names = (JSON.parse(rcText) as Rc).plugins.map(pluginName);
  assert.deepEqual(
    names,
    EXPECTED_PLUGINS,
    "the fixture configuration must load exactly the four shipped plugins",
  );
}

type Subject = string | ((work: string) => string);

function commit(work: string, message: Subject): void {
  appendFileSync(join(work, "README.md"), "change\n");
  git(work, "add", "README.md");
  git(
    work,
    "commit",
    "--quiet",
    "-m",
    typeof message === "function" ? message(work) : message,
  );
}

// Copies the production TypeScript the plugin starts, without the tests and
// test helpers, so the fixture runs the plugin and cli.ts that ship.
function copyTools(work: string): void {
  mkdirSync(join(work, ".github/tools/src"), { recursive: true });
  copyFileSync(
    join(repoRoot, ".github/tools/package.json"),
    join(work, ".github/tools/package.json"),
  );
  copyFileSync(
    join(repoRoot, ".github/tools/src/plugin-manifests.ts"),
    join(work, ".github/tools/src/plugin-manifests.ts"),
  );
  cpSync(toolsRelease, join(work, ".github/tools/src/release"), {
    recursive: true,
    filter: (source) => {
      const top = relative(toolsRelease, source).split(sep)[0];
      return !source.endsWith(".test.ts") && top !== "testing";
    },
  });
}

test("the copied release CLI writes exactly the versioned manifests without an install", (t) => {
  const work = scratchDir("agentics-release-cold-");
  t.after(() => rmSync(work, { recursive: true, force: true }));
  copyTools(work);
  const selectorOnly = ".agents/plugins/marketplace.json";
  const preserved = manifest("1.2.3", true);
  for (const rel of [...VERSION_MANIFESTS, selectorOnly]) {
    const file = join(work, rel);
    mkdirSync(dirname(file), { recursive: true });
    writeFileSync(
      file,
      manifest("1.2.3", basename(rel) === "marketplace.json"),
    );
  }
  const result = spawnSync(
    process.execPath,
    [
      ".github/tools/src/release/cli.ts",
      "set-version",
      "--repo",
      ".",
      "--version",
      "1.3.0",
    ],
    { cwd: work, encoding: "utf8" },
  );
  assert.ifError(result.error);
  assert.equal(result.status, 0, output(result));
  assert.equal(
    result.stdout,
    "Agentics version 1.2.3 -> 1.3.0 (5 manifests written)\n",
  );
  assert.equal(result.stderr, "");
  for (const rel of VERSION_MANIFESTS) {
    assert.match(readFileSync(join(work, rel), "utf8"), /"version": "1\.3\.0"/);
  }
  assert.equal(readFileSync(join(work, selectorOnly), "utf8"), preserved);
});

interface FixtureOptions {
  released?: Subject[];
  plugin?: string;
  rc?: string;
}

interface Fixture {
  root: string;
  bare: string;
  work: string;
  out: string;
}

// Builds a work tree on main whose origin is a local bare repository. The
// baseline and any `released` commits sit under the v1.2.3 tag; `subjects`
// follow it. A message may be a function of the work tree, so it can name an
// earlier commit. `plugin` and `rc` replace the shipped plugin source and
// configuration text in the fixture only.
function fixture(
  subjects: Subject[],
  { released = [], plugin, rc }: FixtureOptions = {},
): Fixture {
  const root = scratchDir("agentics-release-fx-");
  const bare = join(root, "origin.git");
  const work = join(root, "work");
  git(root, "init", "--quiet", "--bare", "-b", "main", bare);
  initRepo(work);
  git(work, "config", "user.email", "fixture@example.invalid");
  git(work, "config", "user.name", "fixture");
  // A URL rather than a bare path: the notes generator parses the repository
  // URL, and a POSIX path is no URL (a Windows one only passes as scheme c:).
  git(work, "remote", "add", "origin", pathToFileURL(bare).href);
  for (const rel of VERSION_MANIFESTS) {
    const file = join(work, rel);
    mkdirSync(dirname(file), { recursive: true });
    writeFileSync(
      file,
      manifest("1.2.3", basename(rel) === "marketplace.json"),
    );
  }
  const release = join(work, ".github/release");
  mkdirSync(release, { recursive: true });
  copyFileSync(join(releaseDir, "package.json"), join(release, "package.json"));
  writeFileSync(
    join(release, ".releaserc.json"),
    rc ?? `${JSON.stringify(shippedRc(), null, 4)}\n`,
  );
  copyTools(work);
  if (plugin !== undefined) {
    writeFileSync(join(work, ".github/tools/src/release/plugin.ts"), plugin);
  }
  assertShippedPlugins(readFileSync(join(release, ".releaserc.json"), "utf8"));
  const link = join(release, "node_modules");
  symlinkSync(
    join(releaseDir, "node_modules"),
    link,
    process.platform === "win32" ? "junction" : "dir",
  );
  links.push(link);
  writeFileSync(join(work, ".gitignore"), "node_modules/\n");
  writeFileSync(join(work, "README.md"), "fixture\n");
  git(work, "add", "-A");
  git(work, "commit", "--quiet", "-m", "chore: baseline (#0)");
  for (const message of released) commit(work, message);
  git(work, "tag", "v1.2.3");
  git(work, "push", "--quiet", "origin", "main", "--tags");
  for (const message of subjects) commit(work, message);
  git(work, "push", "--quiet", "origin", "main");
  return { root, bare, work, out: join(root, "out") };
}

// The origin is a local path, so semantic-release needs no credential.
// `inherited` stands in for the parent environment so a test can show what a
// CI runner's variables would do.
//
// Limits: only the step's command line is read from release.md, not its env:
// block, so the approval value and the bot identity here are the test's own.
// The `git checkout -B main` step is not exercised either; the fixture is
// already on main.
function runCommand(
  fx: Fixture,
  args: readonly string[],
  extraEnv: Record<string, string> = {},
  inherited: Env = process.env,
) {
  const result = spawnSync(process.execPath, args, {
    cwd: fx.work,
    encoding: "utf8",
    env: {
      ...isolatedEnv(inherited),
      GIT_AUTHOR_NAME: "agentics-release[bot]",
      GIT_AUTHOR_EMAIL: "1+agentics-release[bot]@users.noreply.github.com",
      GIT_COMMITTER_NAME: "agentics-release[bot]",
      GIT_COMMITTER_EMAIL: "1+agentics-release[bot]@users.noreply.github.com",
      AGX_RELEASE_OUT: fx.out,
      ...extraEnv,
    },
  });
  assert.ifError(result.error);
  return result;
}

const output = (result: { stdout: string; stderr: string }): string =>
  result.stdout + result.stderr;

const tags = (dir: string): string[] =>
  git(dir, "tag", "--list", "v*").split("\n").sort();

const readPlan = (fx: Fixture): unknown =>
  JSON.parse(readFileSync(join(fx.out, "plan.json"), "utf8"));

function assertNothingReleased(fx: Fixture, head: string): void {
  assert.equal(git(fx.work, "rev-parse", "HEAD"), head);
  assert.equal(git(fx.bare, "rev-parse", "main"), head);
  assert.deepEqual(tags(fx.work), ["v1.2.3"]);
  assert.deepEqual(tags(fx.bare), ["v1.2.3"]);
  assert.equal(existsSync(join(fx.out, "plan.json")), false);
}

function assertMinorRelease(fx: Fixture): void {
  assert.equal(
    git(fx.work, "log", "-1", "--format=%s"),
    "chore(release): v1.3.0 [skip ci]",
  );
  const changed = git(fx.work, "show", "--name-only", "--format=", "HEAD")
    .split("\n")
    .sort();
  assert.deepEqual(changed, [...VERSION_MANIFESTS].sort());
  const head = git(fx.work, "rev-parse", "HEAD");
  assert.equal(git(fx.work, "cat-file", "-t", "v1.3.0"), "commit");
  assert.equal(git(fx.work, "rev-parse", "v1.3.0"), head);
  assert.equal(git(fx.bare, "rev-parse", "main"), head);
  assert.equal(git(fx.bare, "cat-file", "-t", "v1.3.0"), "commit");
  assert.equal(git(fx.bare, "rev-parse", "v1.3.0"), head);
  assert.deepEqual(readPlan(fx), {
    version: "1.3.0",
    tag: "v1.3.0",
    type: "minor",
  });
  assert.match(
    readFileSync(join(fx.out, "baseline.md"), "utf8"),
    /add peer mode/,
  );
  for (const rel of VERSION_MANIFESTS) {
    const text = readFileSync(join(fx.work, rel), "utf8");
    assert.match(text, /"version": "1\.3\.0"/);
    if (basename(rel) === "marketplace.json") {
      assert.match(text, /"version": "0\.0\.0"/);
    }
  }
}

test(
  "a feat subject yields a minor release commit, a lightweight tag, and notes",
  { skip },
  () => {
    const fx = fixture(["feat(skills): add peer mode (#5)"]);
    const result = runCommand(fx, releaseArgs());
    assert.equal(result.status, 0, output(result));
    assertMinorRelease(fx);
  },
);

test(
  "docs, refactor, and subject-only revert subjects release nothing",
  { skip },
  () => {
    // A subject-only `revert:` parses as type revert, and no default release
    // rule releases that type.
    const fx = fixture([
      "docs(skills): clarify intent (#1)",
      "refactor(hooks): tidy parser (#2)",
      "revert: undo a change (#3)",
    ]);
    const head = git(fx.work, "rev-parse", "HEAD");
    const result = runCommand(fx, releaseArgs());
    assert.equal(result.status, 0, output(result));
    assertNothingReleased(fx, head);
  },
);

// GitHub's real revert shape: a `Revert "..."` subject with a
// `This reverts commit <sha>.` body, after the reverted commit was released.
function githubRevertFixture(options: FixtureOptions = {}): Fixture {
  return fixture(
    [
      (work) =>
        `Revert "feat(skills): add peer mode (#5)"

This reverts commit ${git(work, "rev-parse", "HEAD")}.
`,
    ],
    { released: ["feat(skills): add peer mode (#5)"], ...options },
  );
}

test("a GitHub revert of a released commit releases nothing", { skip }, () => {
  // The body line makes the conventionalcommits preset parse the commit as a
  // revert, and the analyzer's default rules would release a patch for that.
  // The release rule in the shipped configuration makes a parsed revert
  // release nothing; a subject-only `revert:` never needed it.
  const fx = githubRevertFixture();
  const head = git(fx.work, "rev-parse", "HEAD");
  const result = runCommand(fx, releaseArgs());
  assert.equal(result.status, 0, output(result));
  assertNothingReleased(fx, head);
});

test(
  "the GitHub revert scenario dies when the revert release rule is removed",
  { skip },
  () => {
    const config = shippedRc();
    assert.ok(
      analyzerEntry(config)[1].releaseRules,
      "the shipped analyzer entry has the rule",
    );
    delete analyzerEntry(config)[1].releaseRules;
    const fx = githubRevertFixture({ rc: JSON.stringify(config, null, 4) });
    const result = runCommand(fx, releaseArgs());
    assert.equal(result.status, 0, output(result));
    assert.equal(
      git(fx.work, "log", "-1", "--format=%s"),
      "chore(release): v1.2.4 [skip ci]",
    );
    assert.deepEqual(readPlan(fx), {
      version: "1.2.4",
      tag: "v1.2.4",
      type: "patch",
    });
  },
);

test("a major aborts without approval and releases with it", { skip }, () => {
  const fx = fixture(["feat(skills)!: retire the wrap skill (#6)"]);
  const head = git(fx.work, "rev-parse", "HEAD");
  const refused = runCommand(fx, releaseArgs());
  assert.notEqual(refused.status, 0);
  assert.match(output(refused), /needs approval/);
  assertNothingReleased(fx, head);

  const approved = runCommand(fx, releaseArgs(), {
    AGX_MAJOR_APPROVED: "true",
  });
  assert.equal(approved.status, 0, output(approved));
  assert.equal(
    git(fx.work, "log", "-1", "--format=%s"),
    "chore(release): v2.0.0 [skip ci]",
  );
  assert.equal(git(fx.work, "cat-file", "-t", "v2.0.0"), "commit");
});

test(
  "a runner's approval flag and branch never reach semantic-release",
  { skip },
  () => {
    // Unscrubbed, AGX_MAJOR_APPROVED would let the major through, and the
    // runner's GITHUB_REF would make semantic-release skip a non-main branch;
    // either way the run would exit 0.
    const fx = fixture(["feat(skills)!: retire the wrap skill (#6)"]);
    const head = git(fx.work, "rev-parse", "HEAD");
    const refused = runCommand(
      fx,
      releaseArgs(),
      {},
      {
        ...process.env,
        AGX_MAJOR_APPROVED: "true",
        CI: "true",
        GITHUB_ACTIONS: "true",
        GITHUB_EVENT_NAME: "push",
        GITHUB_REF: "refs/heads/elsewhere",
      },
    );
    assert.notEqual(refused.status, 0, output(refused));
    assert.match(output(refused), /needs approval/);
    assertNothingReleased(fx, head);
  },
);

test(
  "a global git configuration never reaches the release commit",
  { skip },
  () => {
    // The home below holds a global configuration whose hooksPath rejects every
    // commit; without the empty-configuration override, the release fails.
    const fx = fixture(["feat(skills): add peer mode (#5)"]);
    const home = join(fx.root, "home");
    const hooks = join(home, "hooks");
    mkdirSync(hooks, { recursive: true });
    writeFileSync(join(hooks, "pre-commit"), "#!/bin/sh\nexit 1\n", {
      mode: 0o755,
    });
    writeFileSync(
      join(home, ".gitconfig"),
      `[core]\n\thooksPath = ${hooks.replaceAll("\\", "/")}\n`,
    );
    const result = runCommand(
      fx,
      releaseArgs(),
      {},
      {
        ...process.env,
        HOME: home,
        USERPROFILE: home,
        XDG_CONFIG_HOME: home,
      },
    );
    assert.equal(result.status, 0, output(result));
    assertMinorRelease(fx);
  },
);

test("the level command plans and pushes nothing", { skip }, () => {
  const fx = fixture(["fix(hooks): stop a false deny (#8)"]);
  const head = git(fx.work, "rev-parse", "HEAD");
  const args = levelArgs(fx);
  assert.ok(args.includes("--dry-run"));
  assert.equal(
    args[args.indexOf("--repository-url") + 1],
    pathToFileURL(fx.work).href,
  );
  const result = runCommand(fx, args);
  assert.equal(result.status, 0, output(result));
  assert.deepEqual(readPlan(fx), {
    version: "1.2.4",
    tag: "v1.2.4",
    type: "patch",
  });
  rmSync(join(fx.out, "plan.json"));
  assertNothingReleased(fx, head);
});

function levelOnMajor(options?: FixtureOptions) {
  const fx = fixture(["feat(skills)!: retire the wrap skill (#6)"], options);
  const head = git(fx.work, "rev-parse", "HEAD");
  return { fx, head, result: runCommand(fx, levelArgs(fx)) };
}

test("the level command plans a major without approval", { skip }, () => {
  const { fx, head, result } = levelOnMajor();
  assert.equal(result.status, 0, output(result));
  assert.deepEqual(readPlan(fx), {
    version: "2.0.0",
    tag: "v2.0.0",
    type: "major",
  });
  rmSync(join(fx.out, "plan.json"));
  assertNothingReleased(fx, head);
});

test(
  "the major level scenario dies when the dry-run exemption is removed",
  { skip },
  () => {
    const { fx, head, result } = levelOnMajor({
      plugin: mutatedPlugin("!options.dryRun &&", ""),
    });
    assert.notEqual(result.status, 0, output(result));
    assert.match(output(result), /needs approval/);
    assertNothingReleased(fx, head);
  },
);

test("the release commit alone releases nothing", { skip }, () => {
  const fx = fixture(["feat(skills): add peer mode (#5)"]);
  const first = runCommand(fx, releaseArgs());
  assert.equal(first.status, 0, output(first));
  const head = git(fx.work, "rev-parse", "HEAD");
  rmSync(fx.out, { recursive: true, force: true });
  const second = runCommand(fx, releaseArgs());
  assert.equal(second.status, 0, output(second));
  assert.equal(git(fx.work, "rev-parse", "HEAD"), head);
  assert.equal(git(fx.bare, "rev-parse", "main"), head);
  assert.deepEqual(tags(fx.work), ["v1.2.3", "v1.3.0"]);
  assert.deepEqual(tags(fx.bare), ["v1.2.3", "v1.3.0"]);
  assert.equal(existsSync(join(fx.out, "plan.json")), false);
});

test(
  "resume-check reports resume, then released, for the release commit",
  { skip },
  () => {
    const fx = fixture(["feat(skills): add peer mode (#5)"]);
    const made = runCommand(fx, releaseArgs());
    assert.equal(made.status, 0, output(made));
    const head = git(fx.work, "rev-parse", "HEAD");

    // The scrubbed environment has no GITHUB_OUTPUT, so the command prints
    // key=value lines to standard output.
    function mode(published: string): string {
      const listFile = join(fx.root, "published.txt");
      writeFileSync(listFile, published);
      const result = spawnSync(
        process.execPath,
        [
          join(toolsRelease, "cli.ts"),
          "resume-check",
          "--checkout",
          fx.work,
          "--sha",
          head,
          "--published",
          listFile,
        ],
        { encoding: "utf8", env: isolatedEnv(process.env) },
      );
      assert.equal(result.status, 0, output(result));
      assert.match(result.stdout, /resume_tag=v1\.3\.0/u);
      const found = /mode=(\w+)/u.exec(result.stdout)?.[1];
      assert.ok(found, result.stdout);
      return found;
    }

    assert.equal(mode("v1.2.3\n"), "resume");
    assert.equal(mode("v1.2.3\nv1.3.0\n"), "released");
  },
);

// Returns the shipped plugin source with one replacement made, and fails when
// the replacement finds nothing, so a mutation test cannot pass vacuously.
function mutatedPlugin(from: string, to: string): string {
  const original = readFileSync(join(toolsRelease, "plugin.ts"), "utf8");
  const mutated = original.replace(from, to);
  assert.notEqual(mutated, original, `${from} not found in the plugin`);
  return mutated;
}

test(
  "the unapproved-major scenario dies when the plugin guard is removed",
  { skip },
  () => {
    const mutated = mutatedPlugin(
      'nextRelease.type === "major" &&',
      "false &&",
    );
    const fx = fixture(["feat!: drop an old behavior (#9)"], {
      plugin: mutated,
    });
    const result = runCommand(fx, releaseArgs());
    assert.equal(
      result.status,
      0,
      `the mutated plugin must let the unapproved major through\n${output(result)}`,
    );
    assert.equal(
      git(fx.work, "log", "-1", "--format=%s"),
      "chore(release): v2.0.0 [skip ci]",
    );
  },
);

test(
  "the feat scenario dies when the release message drops [skip ci]",
  { skip },
  () => {
    const original = `${JSON.stringify(shippedRc(), null, 4)}\n`;
    const mutated = original.replace(" [skip ci]", "");
    assert.notEqual(mutated, original);
    const fx = fixture(["feat(skills): add peer mode (#5)"], { rc: mutated });
    const result = runCommand(fx, releaseArgs());
    assert.equal(result.status, 0, output(result));
    assert.throws(
      () => assertMinorRelease(fx),
      (err: unknown) =>
        err instanceof assert.AssertionError &&
        err.actual === "chore(release): v1.3.0",
    );
  },
);

test("a fixture configuration with a publishing plugin is refused", () => {
  const config = shippedRc();
  config.plugins.push("@semantic-release/github");
  assert.throws(
    () =>
      fixture(["feat(skills): add peer mode (#5)"], {
        rc: JSON.stringify(config),
      }),
    /exactly the four shipped plugins/,
  );
});

test("the scrub drops runner, CI, git, token, and approval variables only", () => {
  assert.deepEqual(
    scrubbedEnv({
      AGX_MAJOR_APPROVED: "true",
      CI: "true",
      Ci_Name: "x",
      GH_TOKEN: "t",
      GITHUB_ACTIONS: "true",
      GITHUB_OUTPUT: "/tmp/out",
      GITHUB_TOKEN: "t",
      GIT_DIR: "/elsewhere/.git",
      RUNNER_TEMP: "/tmp",
      github_ref: "refs/heads/x",
      GOCACHE: "/cache",
      GOFLAGS: "-mod=mod",
      HOME: "/home/u",
      Path: "/bin",
      SystemRoot: "C:\\Windows",
    }),
    {
      GOCACHE: "/cache",
      GOFLAGS: "-mod=mod",
      HOME: "/home/u",
      Path: "/bin",
      SystemRoot: "C:\\Windows",
    },
  );
});

test("a step's command is never read from the step after it", () => {
  const text = [
    "jobs:",
    "    level:",
    "        steps:",
    "            - name: Compute the level",
    "              run: echo nothing",
    "            - name: Later",
    "              run: |",
    `                  ${SEMANTIC_RELEASE} --dry-run`,
    "",
  ].join("\n");
  assert.throws(
    () => commandWords(text, "level", "Compute the level"),
    /step "Compute the level" must run exactly one semantic-release command/,
  );
  assert.deepEqual(commandWords(text, "level", "Later").slice(-1), [
    "--dry-run",
  ]);
});

test("a step name that appears twice in its job is refused", () => {
  const text = [
    "jobs:",
    "    level:",
    "        steps:",
    "            - name: Compute the level",
    "              run: |",
    `                  ${SEMANTIC_RELEASE} --dry-run`,
    "            - name: Compute the level",
    "              run: echo nothing",
    "",
  ].join("\n");
  assert.throws(
    () => commandWords(text, "level", "Compute the level"),
    /level names more than one step "Compute the level"/,
  );
});

test("shell syntax beyond plain and quoted words is refused", () => {
  for (const word of [
    "a\\b",
    "a|b",
    "a;b",
    "a&b",
    "a>b",
    "a<b",
    "a`b`",
    "a#b",
  ]) {
    assert.throws(
      () => shellWords(`node x ${word}`),
      /unsupported shell syntax/,
      word,
    );
  }
  assert.throws(
    () => shellWords('node x "--repository-url'),
    /unterminated quote/,
  );
  assert.deepEqual(shellWords(`node x "a  b" 'c'`), [
    "node",
    "x",
    '"a  b"',
    "'c'",
  ]);
});

test("the level command must keep --dry-run and its repository URL", () => {
  const fx = { work: scratchDir("agentics-release-level-") };
  const noDryRun = WORKFLOW.replace(
    `${SEMANTIC_RELEASE} --dry-run`,
    SEMANTIC_RELEASE,
  );
  assert.notEqual(noDryRun, WORKFLOW);
  assert.throws(() => levelArgs(fx, noDryRun), /has no --dry-run/);
  const noUrl = WORKFLOW.replace(` --repository-url ${WORKSPACE_URL_WORD}`, "");
  assert.notEqual(noUrl, WORKFLOW);
  assert.throws(() => levelArgs(fx, noUrl), /has no --repository-url/);
});

test("an unexpanded shell word is refused", () => {
  assert.throws(
    () => resolveWords(["node", '"$HOME/x"']),
    /unexpanded shell word/,
  );
  assert.deepEqual(resolveWords(["'$literal'", '"a b"']), ["$literal", "a b"]);
});
