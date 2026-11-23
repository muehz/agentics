import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import { pathToFileURL } from "node:url";
import { parse } from "yaml";
import {
  fixtureGitEnv,
  git as fixtureGit,
} from "../conformance/testing/git-checkout.ts";

const root = path.resolve(import.meta.dirname, "../../../..");
const nodeAction =
  "actions/setup-node@949feb2413d6458794dcd2491c4babbbce0c15c1";
const releaseStep = "Update only the release notes";
const backlogSteps = [
  "Fetch the backlog and select the batch",
  "Apply the plan and rewrite the index",
];

function object(value: unknown): Record<string, unknown> {
  assert.ok(value && typeof value === "object" && !Array.isArray(value));
  return value as Record<string, unknown>;
}

function jobs(file: string): Record<string, unknown>[][] {
  const text = fs.readFileSync(path.join(root, file), "utf8");
  const yaml = file.endsWith(".md")
    ? text.match(/^---\r?\n([\s\S]*?)\r?\n---/)?.[1]
    : text;
  assert.ok(yaml, file);
  const document: unknown = parse(yaml);
  const doc = object(document);
  const groups = [doc.jobs];
  if (doc["safe-outputs"] !== undefined)
    groups.push(object(doc["safe-outputs"]).jobs);
  return groups.flatMap((group) =>
    group === undefined
      ? []
      : Object.values(object(group)).flatMap((job) => {
          const steps = object(job).steps;
          if (steps === undefined) return [];
          assert.ok(Array.isArray(steps));
          const values: unknown[] = steps;
          return [values.map(object)];
        }),
  );
}

function jobFor(file: string, step: string): Record<string, unknown>[] {
  const matches = jobs(file).filter((steps) =>
    steps.some((entry) => entry.name === step),
  );
  assert.equal(matches.length, 1, `${file}: ${step}`);
  return matches[0]!;
}

test("each migrated workflow entrypoint has pinned Node in its own job", () => {
  for (const suffix of ["md", "lock.yml"]) {
    const workflows = [
      { file: `.github/workflows/release.${suffix}`, targets: [releaseStep] },
      {
        file: `.github/workflows/backlog-grooming.${suffix}`,
        targets: backlogSteps,
      },
    ];
    for (const { file, targets } of workflows)
      for (const target of targets) {
        const steps = jobFor(file, target);
        const runIndex = steps.findIndex((entry) => entry.name === target);
        const setup = steps
          .slice(0, runIndex)
          .filter(
            (entry) =>
              typeof entry.uses === "string" &&
              entry.uses.startsWith("actions/setup-node@"),
          )
          .at(-1);
        assert.ok(setup, `${file}: ${target} needs Node in this job`);
        assert.equal(setup.uses, nodeAction);
        assert.deepEqual(setup.with, { "node-version": "26.12.1" });
        assert.ok(
          setup.if === undefined || setup.if === steps[runIndex]!.if,
          `${file}: Node setup must run whenever the entrypoint runs`,
        );
      }
  }
});

test("the cold Node test launcher runs directly and stays inert on import", (t) => {
  const fixture = fs.mkdtempSync(
    path.join(os.tmpdir(), "agentics node entry "),
  );
  t.after(() => fs.rmSync(fixture, { recursive: true, force: true }));
  const launcherDir = path.join(fixture, "launcher files");
  const aliasDir = path.join(fixture, "launcher alias");
  const cases = path.join(fixture, "test cases");
  fs.mkdirSync(launcherDir);
  fs.mkdirSync(cases);
  fs.symlinkSync(
    launcherDir,
    aliasDir,
    process.platform === "win32" ? "junction" : "dir",
  );
  const launcher = path.join(launcherDir, "node-test.ts");
  const aliased = path.join(aliasDir, "node-test.ts");
  fs.copyFileSync(path.join(import.meta.dirname, "node-test.ts"), launcher);
  for (const name of ["package.json", "node_modules"])
    assert.equal(fs.existsSync(path.join(fixture, name)), false, name);

  const selected = path.join(cases, "selected child.test.ts");
  const marker = path.join(fixture, "child.json");
  const untargeted = path.join(fixture, "untargeted.txt");
  fs.writeFileSync(
    selected,
    `import fs from "node:fs";
import test from "node:test";
test("selected child ran", () => {
  fs.writeFileSync("child.json", JSON.stringify({ version: process.version, executable: process.execPath }));
  if (process.env.AGX_COLD_LAUNCH_FAIL === "1") throw new Error("fixture requested failure");
});`,
  );
  fs.writeFileSync(
    path.join(cases, "untargeted child.test.ts"),
    'import fs from "node:fs"; fs.writeFileSync("untargeted.txt", "ran");',
  );
  const env: NodeJS.ProcessEnv = {
    ...process.env,
    AGX_TEST_CONCURRENCY: "1",
  };
  delete env.NODE_TEST_CONTEXT;
  const run = (args: string[], fail = false) => {
    fs.rmSync(marker, { force: true });
    const result = spawnSync(process.execPath, args, {
      cwd: fixture,
      env: { ...env, AGX_COLD_LAUNCH_FAIL: fail ? "1" : "0" },
      encoding: "utf8",
      timeout: 15_000,
    });
    assert.ifError(result.error);
    assert.equal(result.signal, null);
    assert.equal(fs.existsSync(untargeted), false);
    return result;
  };

  for (const fail of [false, true]) {
    const args = fail
      ? [path.relative(fixture, aliased), path.relative(fixture, selected)]
      : [launcher, selected];
    const result = run(args, fail);
    assert.equal(result.status, fail ? 1 : 0, result.stdout + result.stderr);
    assert.match(result.stdout, /selected child ran/);
    if (fail) assert.match(result.stdout, /fixture requested failure/);
    assert.deepEqual(JSON.parse(fs.readFileSync(marker, "utf8")), {
      version: process.version,
      executable: process.execPath,
    });
  }

  const importer = path.join(fixture, "ordinary import.ts");
  fs.writeFileSync(
    importer,
    `const launcher = await import(${JSON.stringify(pathToFileURL(aliased).href)});
console.log("imported " + launcher.concurrency(2));`,
  );
  const evalImport = `
import { pathToFileURL } from "node:url";
const launcher = await import(pathToFileURL(process.argv[1]).href);
console.log("imported " + launcher.concurrency(2));`;
  for (const args of [
    [importer, selected],
    ["--input-type=module", "--eval", evalImport, aliased, selected],
  ]) {
    const result = run(args);
    assert.equal(result.status, 0, result.stderr);
    assert.equal(fs.existsSync(marker), false, "import launched a test");
    assert.equal(result.stdout.trim(), "imported 4");
  }
});

test("backlog sparse checkouts keep the cold module graph under hostile Git settings", (t) => {
  const fixture = fs.mkdtempSync(
    path.join(os.tmpdir(), "agentics-workflow-sparse-"),
  );
  t.after(() => fs.rmSync(fixture, { recursive: true, force: true }));
  const outside = path.join(fixture, "outside");
  fs.mkdirSync(outside);
  const outsideFile = path.join(outside, "outside.txt");
  fs.writeFileSync(outsideFile, "outside commit\n");
  fixtureGit(outside, "init", "-q");
  fixtureGit(outside, "config", "--local", "core.hooksPath", "/dev/null");
  fixtureGit(outside, "add", "--", "outside.txt");
  fixtureGit(outside, "commit", "-qm", "outside");
  fs.writeFileSync(outsideFile, "outside working-tree edit\n");
  const outsideGit = path.join(outside, ".git");
  const outsideIndex = path.join(outsideGit, "index");
  const snapshot = () => ({
    head: fixtureGit(outside, "rev-parse", "HEAD"),
    index: fs.readFileSync(outsideIndex),
    bytes: fs.readFileSync(outsideFile),
  });
  const before = snapshot();
  const hostile = {
    GIT_DIR: outsideGit,
    GIT_WORK_TREE: outside,
    GIT_INDEX_FILE: outsideIndex,
    GIT_CONFIG_COUNT: "2",
    GIT_CONFIG_KEY_0: "core.autocrlf",
    GIT_CONFIG_VALUE_0: "true",
    GIT_CONFIG_KEY_1: "fixture.config",
    GIT_CONFIG_VALUE_1: "outside",
    GIT_AUTHOR_NAME: "Outside",
  };
  const saved = Object.entries(hostile).map(([key, value]) => {
    const previous = process.env[key];
    process.env[key] = value;
    return [key, previous] as const;
  });
  t.after(() => {
    for (const [key, value] of saved) {
      if (value === undefined) delete process.env[key];
      else process.env[key] = value;
    }
  });
  const seed = path.join(fixture, "seed");
  const scripts = path.join(seed, ".github/tools/src/scripts");
  fs.mkdirSync(scripts, { recursive: true });
  fs.mkdirSync(path.join(seed, "outside-sparse"));
  fs.writeFileSync(
    path.join(seed, "outside-sparse/marker"),
    "excluded by cone\n",
  );
  for (const file of fs.readdirSync(
    path.join(root, ".github/tools/src/scripts"),
  ))
    if (file.endsWith(".ts") && !file.endsWith(".test.ts"))
      fs.copyFileSync(
        path.join(root, ".github/tools/src/scripts", file),
        path.join(scripts, file),
      );
  for (const file of ["package.json", ".github/tools/package.json"])
    fs.copyFileSync(path.join(root, file), path.join(seed, file));
  fs.mkdirSync(path.join(seed, ".github/scripts"), { recursive: true });
  fs.writeFileSync(path.join(seed, ".github/scripts/plugin.sh"), "# fixture\n");
  fixtureGit(seed, "init", "--quiet", "--initial-branch=main");
  fixtureGit(seed, "add", ".");
  fixtureGit(
    seed,
    "-c",
    "user.name=Migration fixture",
    "-c",
    "user.email=migration@example.invalid",
    "commit",
    "--quiet",
    "-m",
    "fixture",
  );
  assert.deepEqual(
    snapshot(),
    before,
    "the seed commands changed the outside checkout",
  );
  const seedHead = fixtureGit(seed, "rev-parse", "HEAD");
  assert.equal(
    fixtureGit(seed, "log", "-1", "--format=%an <%ae>").trim(),
    "Migration fixture <migration@example.invalid>",
  );
  assert.doesNotMatch(
    fixtureGit(seed, "config", "--list"),
    /fixture\.config=outside/,
  );
  let index = 0;
  for (const suffix of ["md", "lock.yml"])
    for (const target of backlogSteps) {
      const file = `.github/workflows/backlog-grooming.${suffix}`;
      const checkout = jobFor(file, target).find(
        (entry) => entry.name === "Checkout grooming scripts",
      );
      assert.ok(checkout, `${file}: ${target}`);
      const sparse = object(checkout.with)["sparse-checkout"];
      assert.equal(typeof sparse, "string");
      const clone = path.join(fixture, `checkout-${index++}`);
      fixtureGit(seed, "clone", "--quiet", "--no-checkout", seed, clone);
      fixtureGit(clone, "sparse-checkout", "init", "--cone");
      fixtureGit(
        clone,
        "sparse-checkout",
        "set",
        ...String(sparse).split(/\r?\n/),
      );
      fixtureGit(clone, "checkout", "--quiet", "main");
      assert.equal(fixtureGit(clone, "rev-parse", "HEAD"), seedHead);
      assert.equal(
        fs.existsSync(path.join(clone, "outside-sparse/marker")),
        false,
      );
      const cli = path.join(
        clone,
        ".github/tools/src/scripts/backlog-grooming.ts",
      );
      assert.ok(
        fs.existsSync(cli),
        `${file}: ${target} omitted its entrypoint`,
      );
      assert.ok(fs.existsSync(path.join(clone, ".github/tools/package.json")));
      assert.equal(fs.existsSync(path.join(clone, "node_modules")), false);
      const result = spawnSync(
        process.execPath,
        [
          "--input-type=module",
          "-e",
          `await import(${JSON.stringify(pathToFileURL(cli).href)});`,
        ],
        { encoding: "utf8", env: fixtureGitEnv(), timeout: 20_000 },
      );
      assert.equal(result.status, 0, result.stderr);
    }
  assert.deepEqual(
    snapshot(),
    before,
    "sparse checkout changed the outside checkout",
  );
});
