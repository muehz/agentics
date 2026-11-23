import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import {
  existsSync,
  mkdirSync,
  readFileSync,
  realpathSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { join, resolve } from "node:path";
import { test } from "node:test";
import { pathToFileURL } from "node:url";
import { posix, scratchDir } from "./testing/support.ts";

const source = readFileSync(new URL("./plugin.ts", import.meta.url), "utf8");

interface Release {
  version: string;
  gitTag: string;
  type: string;
  notes?: string;
}

interface Ctx {
  nextRelease: Release;
  options: { dryRun: boolean };
  env: Record<string, string>;
  logger: { log(message: string): void };
  cwd: string;
}

interface Plugin {
  verifyRelease(config: unknown, context: Ctx): Promise<void>;
  prepare(config: unknown, context: Ctx): Promise<void>;
}

interface Call {
  script: string;
  execPath: string;
  args: string[];
  env: Record<string, string>;
  baselineExisted: boolean;
}

const major: Release = {
  version: "2.0.0",
  gitTag: "v2.0.0",
  type: "major",
  notes: "",
};
const prerelease: Release = {
  version: "1.3.0-rc.1",
  gitTag: "v1.3.0-rc.1",
  type: "minor",
  notes: "",
};

// The plugin spawns the cli.ts that sits beside it. The tests give each loaded
// copy a sibling cli.ts that records how it was started, so a test sees the
// real child process: the interpreter, the script, the arguments, and the whole
// environment it received. The output directory sits beside the checkout, so
// the child can tell whether baseline.md already existed without being told
// where AGX_RELEASE_OUT is. A file named "fail" beside it makes it exit 3.
const FAKE_CLI = `import { appendFileSync, existsSync, realpathSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const args = process.argv.slice(2);
const repo = args[args.indexOf("--repo") + 1] ?? "";
const entry = {
  script: realpathSync(process.argv[1] ?? ""),
  execPath: realpathSync(process.execPath),
  args,
  env: process.env,
  baselineExisted: existsSync(join(dirname(repo), "out", "baseline.md")),
};
appendFileSync(join(here, "calls.jsonl"), JSON.stringify(entry) + "\\n");
if (existsSync(join(here, "fail"))) process.exit(3);
`;

// Builds a plugin context over a fresh scratch directory. load() writes the
// given plugin text beside the fake cli.ts and imports it, so the real source
// and a mutant run the same way.
function harness(overrides: Partial<Ctx> = {}) {
  const dir = scratchDir("agentics-plugin-");
  const out = join(dir, "out");
  const pluginDir = join(dir, "plugin");
  mkdirSync(pluginDir);
  writeFileSync(join(pluginDir, "package.json"), '{"type":"module"}\n');
  writeFileSync(join(pluginDir, "cli.ts"), FAKE_CLI);
  writeFileSync(
    join(pluginDir, "child-process.mjs"),
    `import { execFileSync as run } from "node:child_process";
import { appendFileSync } from "node:fs";
export function execFileSync(...args) {
  appendFileSync(new URL("./requested-envs.jsonl", import.meta.url), JSON.stringify(args[2]?.env) + "\\n");
  return run(...args);
}
`,
  );
  let loads = 0;
  const load = async (text: string): Promise<Plugin> => {
    loads += 1;
    const file = join(pluginDir, `plugin${loads}.ts`);
    writeFileSync(
      file,
      text.replace('from "node:child_process"', 'from "./child-process.mjs"'),
    );
    return (await import(pathToFileURL(file).href)) as Plugin;
  };
  const log = join(pluginDir, "calls.jsonl");
  const calls = (): Call[] =>
    existsSync(log)
      ? readFileSync(log, "utf8")
          .split("\n")
          .filter((line) => line !== "")
          .map((line) => JSON.parse(line) as Call)
      : [];
  const requestedEnvs = (): Record<string, string>[] => {
    const log = join(pluginDir, "requested-envs.jsonl");
    return existsSync(log)
      ? readFileSync(log, "utf8")
          .trim()
          .split("\n")
          .map((line) => JSON.parse(line))
      : [];
  };
  const ctx: Ctx = {
    nextRelease: {
      version: "1.3.0",
      gitTag: "v1.3.0",
      type: "minor",
      notes: "### Features\n",
    },
    options: { dryRun: false },
    env: { AGX_RELEASE_OUT: out },
    logger: { log() {} },
    // Never created and distinct from the output directory, so a test can tell
    // the checkout the writer is pointed at from where the plugin writes.
    cwd: join(dir, "checkout"),
    ...overrides,
  };
  const failChild = (): void => writeFileSync(join(pluginDir, "fail"), "");
  return { dir, out, pluginDir, ctx, calls, requestedEnvs, load, failChild };
}

// Replaces the first occurrence of `from` inside the named function, so two
// identical checks in different functions can be mutated independently. The
// function ends at the first closing brace in column zero. With no function
// name the search covers the whole file.
function mutate(fn: string | undefined, from: string, to: string): string {
  const start = fn === undefined ? 0 : source.indexOf(`function ${fn}(`);
  const end = fn === undefined ? source.length : source.indexOf("\n}\n", start);
  const at = start < 0 ? -1 : source.indexOf(from, start);
  assert.ok(at >= 0 && at < end, `${from} not found in ${fn ?? "the plugin"}`);
  const mutated = source.slice(0, at) + to + source.slice(at + from.length);
  assert.notEqual(mutated, source);
  return mutated;
}

// Each guard's expectation is a function so the same assertion can run
// against the real source and against a mutant with the guard removed.
async function majorNeedsApproval(text: string): Promise<void> {
  const { out, ctx, load } = harness();
  ctx.nextRelease = major;
  const plugin = await load(text);
  await assert.rejects(plugin.verifyRelease({}, ctx), /needs approval/);
  assert.ok(
    !existsSync(join(out, "plan.json")),
    "plan.json was written before the refusal",
  );
}

async function dryRunMajorPasses(text: string): Promise<void> {
  const { ctx, load } = harness({ options: { dryRun: true } });
  ctx.nextRelease = major;
  const plugin = await load(text);
  await plugin.verifyRelease({}, ctx);
}

async function verifyRefusesPrerelease(text: string): Promise<void> {
  const { out, ctx, load } = harness();
  ctx.nextRelease = prerelease;
  const plugin = await load(text);
  await assert.rejects(
    plugin.verifyRelease({}, ctx),
    /unexpected version 1\.3\.0-rc\.1/,
  );
  assert.ok(
    !existsSync(join(out, "plan.json")),
    "plan.json was written before the refusal",
  );
}

async function prepareRefusesPrerelease(text: string): Promise<void> {
  const { ctx, calls, load } = harness();
  ctx.nextRelease = prerelease;
  const plugin = await load(text);
  await assert.rejects(
    plugin.prepare({}, ctx),
    /unexpected version 1\.3\.0-rc\.1/,
  );
  assert.equal(calls().length, 0);
}

async function relativeOutRefused(text: string): Promise<void> {
  const { dir, ctx, load } = harness();
  // A bare name resolves against the working directory, which a mutant that
  // accepts it would write into; the test removes whatever it created.
  const relative = `agentics-release-relative-${dir.split(/[\\/]/u).pop()}`;
  ctx.env.AGX_RELEASE_OUT = relative;
  const plugin = await load(text);
  try {
    await assert.rejects(
      plugin.verifyRelease({}, ctx),
      /AGX_RELEASE_OUT must be an absolute path/,
    );
  } finally {
    rmSync(resolve(relative), { recursive: true, force: true });
  }
}

// The environment the writer is allowed to see. PATH points at a directory that
// does not exist, so a plugin that starts the interpreter by name instead of
// by path cannot find it.
function keptEnv(dir: string): Record<string, string> {
  return {
    PATH: join(dir, "no-such-bin"),
    HOME: "/home/runner",
    USERPROFILE: "C:\\Users\\runner",
    SystemRoot: process.env.SystemRoot ?? "C:\\Windows",
    TMPDIR: "/tmp",
    TEMP: "C:\\Temp",
    TMP: "C:\\Temp",
  };
}

// Darwin can add this field after launch. Measure it independently, without
// taking any expected values from the plugin child's observed environment.
function assertObservedEnv(
  actual: Record<string, string>,
  kept: Record<string, string>,
  baseline: Record<string, string>,
  platform: NodeJS.Platform,
): void {
  const expected = { ...kept };
  if (
    platform === "darwin" &&
    Object.hasOwn(baseline, "__CF_USER_TEXT_ENCODING")
  ) {
    assert.equal(typeof baseline.__CF_USER_TEXT_ENCODING, "string");
    expected.__CF_USER_TEXT_ENCODING = baseline.__CF_USER_TEXT_ENCODING!;
  }
  assert.deepEqual(
    baseline,
    expected,
    "the baseline child added an unexpected environment field",
  );
  assert.deepEqual(actual, expected);
}

// Windows adds several fields of its own. Everywhere, retained values must
// arrive and withheld ones must remain absent; POSIX also compares every key.
function assertChildEnv(
  actual: Record<string, string>,
  kept: Record<string, string>,
  withheld: readonly string[],
): void {
  const find = (key: string): string | undefined =>
    Object.entries(actual).find(
      ([name]) => name.toUpperCase() === key.toUpperCase(),
    )?.[1];
  for (const [key, value] of Object.entries(kept)) {
    assert.equal(find(key), value, `${key} did not reach the child`);
  }
  for (const key of withheld) {
    assert.equal(find(key), undefined, `${key} reached the child`);
  }
  if (posix) {
    const baseline = JSON.parse(
      execFileSync(
        process.execPath,
        ["-e", "process.stdout.write(JSON.stringify(process.env))"],
        { env: kept, encoding: "utf8" },
      ),
    ) as Record<string, string>;
    assertObservedEnv(actual, kept, baseline, process.platform);
  }
}

test("the child environment observer accepts only independently measured Darwin additions", () => {
  const kept = { PATH: "/controlled/bin" };
  const baseline = { ...kept, __CF_USER_TEXT_ENCODING: "baseline-value" };
  assertObservedEnv({ ...baseline }, kept, baseline, "darwin");
  assertObservedEnv({ ...kept }, kept, kept, "darwin");
  assertObservedEnv({ ...kept }, kept, kept, "linux");
  for (const key of [
    "GITHUB_TOKEN",
    "GH_TOKEN",
    "AGX_RELEASE_OUT",
    "UNEXPECTED",
  ]) {
    assert.throws(() =>
      assertObservedEnv(
        { ...baseline, [key]: "leak" },
        kept,
        baseline,
        "darwin",
      ),
    );
    assert.throws(() =>
      assertObservedEnv(
        { ...baseline, [key]: "leak" },
        kept,
        { ...baseline, [key]: "leak" },
        "darwin",
      ),
    );
  }
  const counterexamples: Record<string, string>[] = [
    kept,
    { ...baseline, __CF_USER_TEXT_ENCODING: "different" },
    { ...kept, __cf_user_text_encoding: baseline.__CF_USER_TEXT_ENCODING },
    { __CF_USER_TEXT_ENCODING: baseline.__CF_USER_TEXT_ENCODING },
    { ...baseline, PATH: "/different/bin" },
  ];
  for (const actual of counterexamples)
    assert.throws(() => assertObservedEnv(actual, kept, baseline, "darwin"));
  assert.throws(() => assertObservedEnv(baseline, kept, baseline, "linux"));
  assert.throws(() => assertObservedEnv(baseline, kept, kept, "darwin"));
});

async function childGetsMinimalEnv(text: string): Promise<void> {
  const { dir, pluginDir, ctx, calls, requestedEnvs, load } = harness();
  const kept = keptEnv(dir);
  const withheld = {
    GITHUB_TOKEN: "placeholder",
    GH_TOKEN: "placeholder",
    AGX_MAJOR_APPROVED: "true",
    AGX_RELEASE_BIN: "agentics-tools",
    RUNNER_TEMP: "/runner",
  };
  Object.assign(ctx.env, kept, withheld);
  ctx.env.__CF_USER_TEXT_ENCODING = "context-only-sentinel";
  const plugin = await load(text);
  await plugin.prepare({}, ctx);
  const requested = requestedEnvs();
  assert.equal(requested.length, 1);
  const recorded = calls();
  assert.equal(recorded.length, 1);
  const call = recorded[0];
  assert.ok(call);
  assert.equal(call.execPath, realpathSync(process.execPath));
  assert.equal(call.script, realpathSync(join(pluginDir, "cli.ts")));
  assert.deepEqual(call.args, [
    "set-version",
    "--repo",
    ctx.cwd,
    "--version",
    "1.3.0",
  ]);
  assertChildEnv(call.env, kept, [...Object.keys(withheld), "AGX_RELEASE_OUT"]);
  assert.equal(
    requested[0]?.__CF_USER_TEXT_ENCODING,
    undefined,
    "__CF_USER_TEXT_ENCODING from the context was requested before launch",
  );
}

// Windows spells the variable Path; the allowlist must still keep it.
async function childKeepsTheWindowsPathSpelling(text: string): Promise<void> {
  const { ctx, calls, load } = harness();
  ctx.env.Path = "C:\\Windows\\System32";
  const plugin = await load(text);
  await plugin.prepare({}, ctx);
  const call = calls()[0];
  assert.ok(call);
  assertChildEnv(call.env, { Path: "C:\\Windows\\System32" }, [
    "AGX_RELEASE_OUT",
  ]);
}

async function prepareStopsWhenSetVersionFails(text: string): Promise<void> {
  const { out, ctx, calls, load, failChild } = harness();
  failChild();
  const plugin = await load(text);
  await assert.rejects(plugin.prepare({}, ctx), /Command failed/);
  assert.equal(calls().length, 1);
  assert.ok(!existsSync(join(out, "baseline.md")));
}

const missingRejection = {
  name: "AssertionError",
  message: /Missing expected rejection/,
};

test("verifyRelease records the plan", async () => {
  const { out, ctx, load } = harness();
  const plugin = await load(source);
  await plugin.verifyRelease({}, ctx);
  const plan: unknown = JSON.parse(
    readFileSync(join(out, "plan.json"), "utf8"),
  );
  assert.deepEqual(plan, { version: "1.3.0", tag: "v1.3.0", type: "minor" });
  assert.equal(
    readFileSync(join(out, "plan.json"), "utf8"),
    '{"version":"1.3.0","tag":"v1.3.0","type":"minor"}\n',
  );
});

test("verifyRelease refuses a major without approval", () =>
  majorNeedsApproval(source));

test("verifyRelease lets an approved major through", async () => {
  const { ctx, load } = harness();
  ctx.nextRelease = major;
  ctx.env.AGX_MAJOR_APPROVED = "true";
  const plugin = await load(source);
  await plugin.verifyRelease({}, ctx);
});

test("verifyRelease lets a dry-run major through", () =>
  dryRunMajorPasses(source));

test("verifyRelease refuses a version that is not MAJOR.MINOR.PATCH", () =>
  verifyRefusesPrerelease(source));

test("verifyRelease refuses a relative AGX_RELEASE_OUT", () =>
  relativeOutRefused(source));

test("prepare runs set-version first, then writes the notes as the baseline", async () => {
  const { out, ctx, calls, load } = harness();
  const plugin = await load(source);
  await plugin.prepare({}, ctx);
  assert.equal(calls().length, 1);
  assert.equal(calls()[0]?.baselineExisted, false);
  assert.equal(
    readFileSync(join(out, "baseline.md"), "utf8"),
    "### Features\n",
  );
});

test("prepare writes an empty baseline when there are no notes", async () => {
  const { out, ctx, load } = harness();
  delete ctx.nextRelease.notes;
  const plugin = await load(source);
  await plugin.prepare({}, ctx);
  assert.equal(readFileSync(join(out, "baseline.md"), "utf8"), "");
});

test("prepare refuses a version that is not MAJOR.MINOR.PATCH", () =>
  prepareRefusesPrerelease(source));

test("prepare refuses a relative AGX_RELEASE_OUT", async () => {
  const { out, ctx, load } = harness();
  ctx.env.AGX_RELEASE_OUT = "relative-out";
  const plugin = await load(source);
  await assert.rejects(
    plugin.prepare({}, ctx),
    /AGX_RELEASE_OUT must be an absolute path/,
  );
  assert.ok(!existsSync(join(out, "baseline.md")));
  assert.ok(!existsSync(resolve("relative-out")));
});

test("prepare stops, and writes no baseline, when set-version fails", () =>
  prepareStopsWhenSetVersionFails(source));

test("prepare passes set-version its arguments and a minimal environment", () =>
  childGetsMinimalEnv(source));

test("prepare keeps the Windows spelling of Path", () =>
  childKeepsTheWindowsPathSpelling(source));

test("prepare no longer reads AGX_RELEASE_BIN", async () => {
  assert.doesNotMatch(source, /AGX_RELEASE_BIN/u);
  const { ctx, calls, load } = harness();
  ctx.env.AGX_RELEASE_BIN = "not-an-absolute-path";
  const plugin = await load(source);
  await plugin.prepare({}, ctx);
  assert.equal(calls().length, 1);
});

test("the plugin exports the two steps semantic-release calls and nothing else", async () => {
  const { load } = harness();
  const plugin = await load(source);
  assert.deepEqual(Object.keys(plugin).sort(), ["prepare", "verifyRelease"]);
});

test("the mutate helper reports a needle that is not in the function", () => {
  assert.throws(
    () => mutate("prepare", "no such text", "x"),
    /no such text not found in prepare/,
  );
  assert.throws(
    () => mutate("verifyRelease", "const { version } = nextRelease;", "x"),
    /not found in verifyRelease/,
  );
});

test("the major guard test dies when the guard is removed", async () => {
  const mutated = mutate(
    "verifyRelease",
    'nextRelease.type === "major" &&',
    "false &&",
  );
  await assert.rejects(majorNeedsApproval(mutated), missingRejection);
});

test("the major test dies when plan.json is written before the guard", async () => {
  const mutated = mutate(
    "verifyRelease",
    "const { nextRelease, options, env, logger } = context;",
    'const { nextRelease, options, env, logger } = context;\n  writeFileSync(join(outDir(env), "plan.json"), "{}");',
  );
  await assert.rejects(majorNeedsApproval(mutated), {
    name: "AssertionError",
    message: /plan\.json was written before the refusal/,
  });
});

test("the repo test dies when set-version is pointed at the output directory", async () => {
  const mutated = mutate(
    "prepare",
    '"--repo", cwd,',
    '"--repo", env.AGX_RELEASE_OUT,',
  );
  await assert.rejects(childGetsMinimalEnv(mutated), {
    name: "AssertionError",
  });
});

test("the Path test dies when the allowlist compares keys case-sensitively", async () => {
  const mutated = mutate("childEnv", "key.toUpperCase()", "key");
  await assert.rejects(childKeepsTheWindowsPathSpelling(mutated), {
    name: "AssertionError",
  });
});

test("the dry-run test dies when the dry-run exemption is removed", async () => {
  const mutated = mutate("verifyRelease", "!options.dryRun &&", "");
  await assert.rejects(dryRunMajorPasses(mutated), /needs approval/);
});

test("the verifyRelease version test dies when its check is removed", async () => {
  const mutated = mutate(
    "verifyRelease",
    "!SEMVER.test(nextRelease.version)",
    "false",
  );
  await assert.rejects(verifyRefusesPrerelease(mutated), missingRejection);
});

test("the prepare version test dies when its check is removed", async () => {
  const mutated = mutate(
    "prepare",
    "!SEMVER.test(nextRelease.version)",
    "false",
  );
  await assert.rejects(prepareRefusesPrerelease(mutated), missingRejection);
});

test("the AGX_RELEASE_OUT test dies when the absolute-path check is removed", async () => {
  const mutated = mutate("outDir", "!dir || !isAbsolute(dir)", "!dir");
  await assert.rejects(relativeOutRefused(mutated), missingRejection);
});

test("the environment test dies when prepare passes the whole environment", async () => {
  const mutated = mutate("prepare", "env: childEnv(env)", "env");
  await assert.rejects(childGetsMinimalEnv(mutated), {
    name: "AssertionError",
    message: /GITHUB_TOKEN/,
  });
});

test("the environment test dies when the allowlist admits the token", async () => {
  const mutated = mutate(
    undefined,
    '"USERPROFILE",',
    '"USERPROFILE",\n  "GITHUB_TOKEN",',
  );
  await assert.rejects(childGetsMinimalEnv(mutated), {
    name: "AssertionError",
    message: /GITHUB_TOKEN/,
  });
});

test("the environment test dies when the allowlist admits a context encoding value", async () => {
  const mutated = mutate(
    undefined,
    '"USERPROFILE",',
    '"USERPROFILE",\n  "__CF_USER_TEXT_ENCODING",',
  );
  await assert.rejects(childGetsMinimalEnv(mutated), {
    name: "AssertionError",
    message: /__CF_USER_TEXT_ENCODING/,
  });
});

test("the interpreter test dies when the child is started by name, not by process.execPath", async () => {
  const mutated = mutate("prepare", "process.execPath,", '"node",');
  await assert.rejects(childGetsMinimalEnv(mutated));
});

test("the script test dies when set-version runs a script that is not the sibling cli.ts", async () => {
  const mutated = mutate(
    undefined,
    'new URL("./cli.ts", import.meta.url)',
    'new URL("./other.ts", import.meta.url)',
  );
  await assert.rejects(childGetsMinimalEnv(mutated));
});

test("the failure test dies when prepare ignores a failing set-version", async () => {
  const mutated = mutate("prepare", "execFileSync(", "(() => {})(");
  await assert.rejects(
    prepareStopsWhenSetVersionFails(mutated),
    missingRejection,
  );
});
