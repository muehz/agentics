import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { test } from "node:test";

type PluginEntry = string | [string, Record<string, unknown>];

interface ReleaseConfig {
  plugins: PluginEntry[];
}

interface Package {
  dependencies?: Record<string, string>;
  devDependencies?: Record<string, string>;
  optionalDependencies?: Record<string, string>;
  peerDependencies?: Record<string, string>;
}

const releaseDir = join(import.meta.dirname, "../../../../.github/release");

function read<T>(name: string): T {
  return JSON.parse(readFileSync(join(releaseDir, name), "utf8")) as T;
}

// The third entry is the in-tree plugin, which semantic-release loads as the
// TypeScript source; no other path, including the retired CommonJS plugin, is
// accepted.
const EXPECTED_PLUGINS = [
  "@semantic-release/commit-analyzer",
  "@semantic-release/release-notes-generator",
  "./.github/tools/src/release/plugin.ts",
  "@semantic-release/git",
];
const RELEASE_COMMIT_MESSAGE =
  "chore(release): v${nextRelease.version} [skip ci]";
// Without this rule the analyzer's default rules release a patch for a commit
// whose body says `This reverts commit <sha>.`, so a revert would ship.
const REVERT_RELEASE_RULES = [{ revert: true, release: false }];
const VERSION_MANIFESTS = [
  ".claude-plugin/marketplace.json",
  ".claude-plugin/plugin.json",
  ".codex-plugin/plugin.json",
  ".github/plugin/plugin.json",
  ".antigravity-plugin/plugin.json",
];

const nameOf = (entry: PluginEntry): string =>
  Array.isArray(entry) ? entry[0] : entry;

function pluginViolations(config: ReleaseConfig): string[] {
  const names = config.plugins.map(nameOf);
  const violations: string[] = [];
  const listIsExpected =
    names.length === EXPECTED_PLUGINS.length &&
    EXPECTED_PLUGINS.every((expected, i) => names[i] === expected);
  if (!listIsExpected) {
    violations.push(`plugin list is ${JSON.stringify(names)}`);
  }
  const analyzer = config.plugins.find(
    (p) => Array.isArray(p) && p[0] === "@semantic-release/commit-analyzer",
  );
  if (
    !Array.isArray(analyzer) ||
    JSON.stringify(analyzer[1].releaseRules) !==
      JSON.stringify(REVERT_RELEASE_RULES)
  ) {
    violations.push(
      'the commit-analyzer entry must carry releaseRules [{ "revert": true, "release": false }]',
    );
  }
  const git = config.plugins.find(
    (p) => Array.isArray(p) && p[0] === "@semantic-release/git",
  );
  if (
    !Array.isArray(git) ||
    JSON.stringify(git[1].assets) !== JSON.stringify(VERSION_MANIFESTS)
  ) {
    violations.push("@semantic-release/git assets are not the five manifests");
  }
  // The subject is what resume-check and the loop guard recognize, and
  // [skip ci] is what keeps the release commit from re-triggering the workflow.
  if (!Array.isArray(git) || git[1].message !== RELEASE_COMMIT_MESSAGE) {
    violations.push(
      "@semantic-release/git message is not the release-commit subject",
    );
  }
  return violations;
}

const DEPENDENCY_FIELDS = [
  "dependencies",
  "devDependencies",
  "optionalDependencies",
  "peerDependencies",
] as const;

function pinViolations(pkg: Package): string[] {
  const violations: string[] = [];
  for (const field of DEPENDENCY_FIELDS) {
    for (const [name, range] of Object.entries(pkg[field] ?? {})) {
      if (!/^\d+\.\d+\.\d+$/u.test(range)) {
        violations.push(`${name} in ${field} is not an exact pin: ${range}`);
      }
    }
  }
  // Major 10 of the preset needs conventional-changelog-writer 9, and
  // release-notes-generator 14 still uses writer 8: 10.0.0 renders empty notes
  // and 10.4.1 throws. Move to 10 only with a generator release on writer 9.
  const preset =
    pkg.dependencies?.["conventional-changelog-conventionalcommits"];
  if (!preset || !preset.startsWith("9.")) {
    violations.push(`the preset must stay on major 9, got ${preset}`);
  }
  return violations;
}

function entryNamed(
  config: ReleaseConfig,
  name: string,
): [string, Record<string, unknown>] {
  const entry = config.plugins.find((p) => Array.isArray(p) && p[0] === name);
  assert.ok(Array.isArray(entry), `${name} entry with options`);
  return entry;
}

test("the plugin list is exactly the four named entries", () => {
  assert.deepEqual(pluginViolations(read(".releaserc.json")), []);
});

test("every dependency is an exact pin and the preset is major 9", () => {
  assert.deepEqual(pinViolations(read("package.json")), []);
});

test("the plugin-list test dies when a publishing plugin is added", () => {
  const config = read<ReleaseConfig>(".releaserc.json");
  config.plugins.push("@semantic-release/github");
  assert.notDeepEqual(pluginViolations(config), []);
});

test("the plugin-list test dies when the retired .cjs plugin returns", () => {
  const config = read<ReleaseConfig>(".releaserc.json");
  config.plugins[2] = "./.github/release/agentics-release.cjs";
  assert.match(String(pluginViolations(config)), /plugin list is/);
});

test("the plugin-list test dies when the in-tree plugin is not the TypeScript plugin", () => {
  for (const path of [
    "./.github/release/other.cjs",
    "./.github/tools/src/release/cli.ts",
    "@semantic-release/github",
  ]) {
    const config = read<ReleaseConfig>(".releaserc.json");
    config.plugins[2] = path;
    assert.match(String(pluginViolations(config)), /plugin list is/, path);
  }
});

test("the plugin-list test dies when an entry is dropped or reordered", () => {
  const dropped = read<ReleaseConfig>(".releaserc.json");
  dropped.plugins.splice(2, 1);
  assert.match(String(pluginViolations(dropped)), /plugin list is/);
  const swapped = read<ReleaseConfig>(".releaserc.json");
  const [first, second, ...rest] = swapped.plugins;
  assert.ok(first && second);
  swapped.plugins = [second, first, ...rest];
  assert.match(String(pluginViolations(swapped)), /plugin list is/);
});

test("the release-rule test dies when the revert rule is removed or releases", () => {
  const removed = read<ReleaseConfig>(".releaserc.json");
  delete entryNamed(removed, "@semantic-release/commit-analyzer")[1]
    .releaseRules;
  assert.match(String(pluginViolations(removed)), /releaseRules/);
  const released = read<ReleaseConfig>(".releaserc.json");
  entryNamed(released, "@semantic-release/commit-analyzer")[1].releaseRules = [
    { revert: true, release: "patch" },
  ];
  assert.match(String(pluginViolations(released)), /releaseRules/);
});

test("the message test dies when [skip ci] is dropped", () => {
  const config = read<ReleaseConfig>(".releaserc.json");
  entryNamed(config, "@semantic-release/git")[1].message =
    "chore(release): v${nextRelease.version}";
  assert.notDeepEqual(pluginViolations(config), []);
});

test("the assets test dies when a manifest leaves the git plugin's assets", () => {
  const config = read<ReleaseConfig>(".releaserc.json");
  entryNamed(config, "@semantic-release/git")[1].assets =
    VERSION_MANIFESTS.slice(1);
  assert.match(String(pluginViolations(config)), /assets are not the five/);
});

test("the pin test dies when a range hides in another dependency field", () => {
  const pkg = read<Package>("package.json");
  pkg.devDependencies = { prettier: "^3.0.0" };
  assert.notDeepEqual(pinViolations(pkg), []);
});

test("the pin test dies when a range is introduced", () => {
  const pkg = read<Package>("package.json");
  assert.ok(pkg.dependencies);
  pkg.dependencies["semantic-release"] = "^25.0.9";
  assert.notDeepEqual(pinViolations(pkg), []);
});

test("the pin test dies when the preset leaves major 9", () => {
  const pkg = read<Package>("package.json");
  assert.ok(pkg.dependencies);
  pkg.dependencies["conventional-changelog-conventionalcommits"] = "10.0.0";
  assert.match(String(pinViolations(pkg)), /the preset must stay on major 9/);
});

const validatePath = join(
  import.meta.dirname,
  "../../../../.github/workflows/validate.yml",
);
const RELEASE_INSTALL = "npm ci --ignore-scripts --prefix .github/release";

// One top-level job of a workflow: from its `  name:` line to the next line indented no deeper, comment lines dropped
// so prose cannot satisfy the check.
function workflowJob(source: string, name: string): string {
  const lines = source.split("\n").filter((line) => !/^\s*#/.test(line));
  const start = lines.indexOf(`  ${name}:`);
  assert.notEqual(start, -1, `validate.yml has no ${name} job`);
  const end = lines.findIndex(
    (line, i) => i > start && line.trim() !== "" && line.search(/\S/) <= 2,
  );
  return lines.slice(start, end === -1 ? lines.length : end).join("\n");
}

// The level-parity and plugin end-to-end tests load semantic-release from .github/release/node_modules, so the job that
// runs them installs it.
function releaseInstallViolations(validate: string): string[] {
  return workflowJob(validate, "ts-suite").includes(RELEASE_INSTALL)
    ? []
    : [`the ts-suite job must run ${RELEASE_INSTALL}`];
}

const readValidate = (): string =>
  readFileSync(validatePath, "utf8").replaceAll("\r\n", "\n");

test("the ts-suite job installs the release tooling the plugin tests import", () => {
  assert.deepEqual(releaseInstallViolations(readValidate()), []);
});

test("the release-install test dies when ts-suite stops installing .github/release", () => {
  const validate = readValidate();
  const from = `        run: ${RELEASE_INSTALL}\n`;
  assert.equal(validate.split(from).length - 1, 1, "the install line moved");
  const mutated = validate.replace(
    from,
    "        run: npm ci --ignore-scripts\n",
  );
  assert.match(String(releaseInstallViolations(mutated)), /must run npm ci/);
});
