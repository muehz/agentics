import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import fs, {
  chmodSync,
  cpSync,
  existsSync,
  lstatSync,
  mkdirSync,
  mkdtempSync,
  readdirSync,
  readFileSync,
  readlinkSync,
  realpathSync,
  renameSync,
  rmSync,
  statSync,
  symlinkSync,
  writeFileSync,
} from "node:fs";
import fsp from "node:fs/promises";
import { syncBuiltinESMExports } from "node:module";
import { tmpdir } from "node:os";
import { delimiter, dirname, join, relative } from "node:path";
import { after, test, type TestContext } from "node:test";
import { fixtureGitEnv, git } from "../conformance/testing/git-checkout.ts";
import {
  createProfile,
  createStageFixture,
  disposeSuite,
  getSuite,
  gitExecutable,
  systemDirs,
  writeLauncher,
  type StageFixture,
} from "./testing/harness.ts";
import { findCommand, spawnPlan } from "../plugin/common.ts";
import { refreshStage, removeStage, verifyStage } from "../plugin/stage.ts";

after(disposeSuite);
const stageName = "agentics-local-marketplace";
const bloatPaths = [".git", "node_modules", ".claude/worktrees"];
// The maintainer CLI names the stage by its physical path, which on Windows can differ in spelling from the suite's.
const stagePath = () =>
  join(realpathSync.native(getSuite().cacheDir), stageName);
const posixOnly = (reason: string) =>
  process.platform === "win32" ? `POSIX only: ${reason}` : false;
function temporary(t: TestContext): string {
  const path = realpathSync(mkdtempSync(join(tmpdir(), "agentics-stage-")));
  t.after(() => rmSync(path, { recursive: true, force: true }));
  return path;
}
function overlapFixture(t: TestContext, layout: string) {
  const f = createStageFixture(t),
    cache = temporary(t);
  const target = join(
    cache,
    stageName,
    ...(layout === "stage-ancestor" ? ["checkout"] : []),
  );
  mkdirSync(dirname(target), { recursive: true });
  renameSync(f.root, target);
  f.root = target;
  return { f, cache };
}
/** Runs the legacy setup contract from a checkout fixture with tracked native outputs. */
function setup(t: TestContext) {
  const p = createProfile(t),
    f = createStageFixture(t);
  const source = getSuite().repoRoot;
  f.write(
    ".claude-plugin/plugin.json",
    readFileSync(join(source, ".claude-plugin/plugin.json")),
  );
  f.write("bin/agx", readFileSync(join(source, "bin/agx")), 0o755);
  commitAll(f);
  return { p, f, result: f.run(p) };
}
function assertAbsent(path: string) {
  assert.equal(lstatSync(path, { throwIfNoEntry: false }), undefined, path);
}
/**
 * Reports how stage fails to carry the agx launchers from the fixture. On POSIX
 * bin/agx must arrive executable, since the shell runs it directly; any
 * execute bit counts, because umask decides the rest. Windows has no execute
 * bits. bin/agx.exe must arrive byte-identical, since it is a committed build
 * output the stage must not alter.
 */
function launcherProblems(
  stage: string,
  windowsLauncher: Buffer,
  platform: NodeJS.Platform = process.platform,
): string[] {
  const problems: string[] = [];
  const launcher = statSync(join(stage, "bin/agx"), { throwIfNoEntry: false });
  if (!launcher) problems.push("bin/agx is missing from the stage");
  else if (platform !== "win32" && (launcher.mode & 0o111) === 0)
    problems.push(
      `bin/agx lost its executable bit in the stage (mode ${(launcher.mode & 0o777).toString(8)})`,
    );
  if (!existsSync(join(stage, "bin/agx.exe")))
    problems.push("bin/agx.exe is missing from the stage");
  else if (!readFileSync(join(stage, "bin/agx.exe")).equals(windowsLauncher))
    problems.push("bin/agx.exe in the stage differs from the checkout's copy");
  return problems;
}

// A Node script standing in for go and npm: it logs its name to the fake CLI
// log and fails, so any build or validation step that starts it shows there.
const loggingShim = String.raw`import { appendFileSync } from "node:fs";
const [name] = process.argv.slice(2);
if (process.env.AGX_FAKE_CLI_LOG) appendFileSync(process.env.AGX_FAKE_CLI_LOG, name + " invoked\n");
process.exitCode = 97;
`;

test("stage entrypoint preflight precedes build and CLI", async (t) => {
  for (const action of ["setup", "teardown"])
    for (const rootKind of [
      "checkout",
      "relative",
      "stage-equals-checkout",
      "stage-ancestor",
    ])
      await t.test(`${action}/${rootKind}`, (t) => {
        const overlapping = rootKind.startsWith("stage-")
          ? overlapFixture(t, rootKind)
          : undefined;
        const f = overlapping?.f ?? createStageFixture(t),
          p = createProfile(t);
        const cache =
          rootKind === "relative"
            ? "relative-cache"
            : (overlapping?.cache ?? f.root);
        f.write(`${stageName}/sentinel`, "preserve", 0o600);
        const shims = p.path("shims"),
          script = p.path("logging-shim.mjs");
        p.write(script, loggingShim);
        for (const tool of ["go", "npm"])
          writeLauncher(shims, tool, script, [tool]);
        const r = p.run({
          script: f.cli(),
          args: action === "teardown" ? ["--teardown"] : [],
          extraPath: shims,
          env: { XDG_CACHE_HOME: cache },
        });
        assert.notEqual(r.exit, 0, r.stdout + r.stderr);
        assert.ok(
          r.stderr.includes("ERROR: invalid marketplace cache root"),
          r.stdout + r.stderr,
        );
        assert.equal(r.log, "", "build/CLI ran before preflight");
        assert.ok(!r.stdout.includes("manifests"), r.stdout);
        assert.equal(
          readFileSync(f.path(`${stageName}/sentinel`), "utf8"),
          "preserve",
        );
        for (const name of ["agentics", "relative-cache"]) {
          assertAbsent(f.path(name));
          assertAbsent(p.path(name));
        }
        if (overlapping) assertAbsent(join(overlapping.cache, "agentics"));
      });
});

test("stage is registered instead of checkout", (t) => {
  const { f, result } = setup(t);
  for (const cli of ["claude", "copilot", "codex"])
    assert.ok(
      result.log.includes(`${cli}\tplugin\tmarketplace\tadd\t${stagePath()}`),
      result.log,
    );
  assert.ok(
    result.log.includes(`agy\tplugin\tinstall\t${stagePath()}`),
    result.log,
  );
  for (const line of result.log.split("\n"))
    if (
      line.includes("marketplace\tadd\t") ||
      line.startsWith("agy\tplugin\tinstall\t")
    )
      assert.ok(!line.endsWith(`\t${f.root}`), line);
});

test("marketplace is removed before being repointed", (t) => {
  const { result } = setup(t);
  for (const cli of ["claude", "copilot", "codex"]) {
    const removal = `${cli}\tplugin\tmarketplace\tremove\tmuehz${cli === "copilot" ? "\t--force" : ""}`;
    assert.ok(result.log.includes(removal), result.log);
    const remove = result.log.indexOf(`${cli}\tplugin\tmarketplace\tremove\t`),
      add = result.log.indexOf(`${cli}\tplugin\tmarketplace\tadd\t`);
    assert.ok(add >= 0 && remove < add, result.log);
  }
  const agyUninstall = result.log.indexOf("agy\tplugin\tuninstall\tagentics");
  const agyInstall = result.log.indexOf("agy\tplugin\tinstall\t");
  assert.ok(
    agyInstall >= 0 && agyUninstall >= 0 && agyUninstall < agyInstall,
    result.log,
  );
});

test("stage excludes working-tree bloat", (t) => {
  setup(t);
  for (const path of bloatPaths) assertAbsent(join(stagePath(), path));
  for (const path of [
    ".claude-plugin/marketplace.json",
    ".claude-plugin/plugin.json",
    ".agents/plugins/marketplace.json",
    ".antigravity-plugin/plugin.json",
    "skills",
  ])
    assert.ok(existsSync(join(stagePath(), path)), path);
});

function hookBinaries(f: StageFixture): string[] {
  const names = readdirSync(f.path("hooks/bin"), { withFileTypes: true })
    .filter((entry) => !entry.isDirectory())
    .map((entry) => entry.name);
  assert.ok(names.length > 0, "binary roster must not be empty");
  return names;
}

test("stage carries hook binaries and launchers", (t) => {
  const { f } = setup(t);
  for (const name of hookBinaries(f)) {
    assert.deepEqual(
      readFileSync(join(stagePath(), "hooks/bin", name)),
      readFileSync(f.path(`hooks/bin/${name}`)),
      name,
    );
  }
  assert.deepEqual(
    readFileSync(join(stagePath(), "hooks/SHA256SUMS")),
    readFileSync(f.path("hooks/SHA256SUMS")),
  );
  assert.ok(existsSync(join(stagePath(), "hooks/bin/agx-windows-amd64.exe")));
  assert.deepEqual(
    launcherProblems(stagePath(), readFileSync(f.path("bin/agx.exe")), "win32"),
    [],
  );
});

test(
  "stage keeps hook binaries executable",
  { skip: posixOnly("execute bits and symlinks") },
  (t) => {
    const { f } = setup(t);
    for (const name of hookBinaries(f))
      assert.notEqual(
        statSync(join(stagePath(), "hooks/bin", name)).mode & 0o111,
        0,
        name,
      );
    assert.deepEqual(
      launcherProblems(stagePath(), readFileSync(f.path("bin/agx.exe"))),
      [],
    );
  },
);

test("stage carries uncommitted edits and removes deleted files", (t) => {
  const p = createProfile(t),
    f = createStageFixture(t);
  f.write("skills/committed/SKILL.md", "uncommitted-edit-marker\n");
  f.write("skills/fresh/SKILL.md", "never-committed-marker\n");
  f.run(p);
  for (const [path, marker] of [
    ["skills/committed/SKILL.md", "uncommitted-edit-marker"],
    ["skills/fresh/SKILL.md", "never-committed-marker"],
  ])
    assert.ok(readFileSync(join(stagePath(), path!), "utf8").includes(marker!));
  rmSync(f.path("skills/fresh"), { recursive: true });
  f.run(p);
  assertAbsent(join(stagePath(), "skills/fresh"));
});

test("stage excludes ignored paths and a real git directory", (t) => {
  const p = createProfile(t),
    f = createStageFixture(t);
  assert.ok(statSync(f.path(".git")).isDirectory());
  f.run(p);
  for (const path of bloatPaths) assertAbsent(join(stagePath(), path));
  assert.ok(existsSync(join(stagePath(), "skills/committed/SKILL.md")));
});

test("teardown removes stage", (t) => {
  const { f, p } = setup(t);
  assert.ok(existsSync(stagePath()));
  const result = f.run(p, { args: ["--teardown"] });
  assert.ok(
    result.log.includes("agy\tplugin\tuninstall\tagentics"),
    result.log,
  );
  assert.ok(
    result.log.includes(
      "agy\tplugin\tinstall\thttps://github.com/muehz/agentics",
    ),
    result.log,
  );
  assertAbsent(stagePath());
});

const zeroHash = "0".repeat(64);

test("npm setup installs unpublished source for offline agx help and preserves a good stage on build failure", (t) => {
  const f = createStageFixture(t);
  const p = createProfile(t);
  const source = getSuite().repoRoot;
  const npm = findCommand("npm");
  const go = findCommand("go");
  assert.ok(npm && go, "source-only setup requires npm and Go");
  f.git("rm", "-r", "hooks/bin");
  if (existsSync(f.path("bin/agx.exe"))) f.git("rm", "bin/agx.exe");
  cpSync(join(source, "hooks/go"), f.path("hooks/go"), { recursive: true });
  for (const rel of [
    "package.json",
    "bin/agx",
    "hooks/run-hook.cmd",
    "hooks/fetch-hook.ps1",
    ".claude-plugin/plugin.json",
    "hooks/SHA256SUMS",
  ])
    f.write(rel, readFileSync(join(source, rel)), 0o755);
  commitAll(f);
  const trackedManifest = readFileSync(f.path("hooks/SHA256SUMS"));
  const main = "hooks/go/cmd/hook/main.go";
  const oldHelp = "agx: the Agentics command-line tool";
  const newHelp = "agx: unpublished developer checkout";
  const original = readFileSync(f.path(main), "utf8");
  assert.ok(original.includes(oldHelp));
  f.write(main, original.replace(oldHelp, newHelp));
  const decoys = [
    "bin/AGX.EXE",
    "hooks/bin/agx-linux-amd64",
    "hooks/BiN/local-only/unpublished",
  ];
  // A case-sensitive volume can hold this directory alongside the tracked manifest.
  if (!existsSync(f.path("hooks/sHa256SuMs")))
    decoys.push("hooks/sHa256SuMs/local-only");
  for (const rel of decoys) f.write(rel, `local decoy ${rel}`);
  const installed = p.path("installed plugin");
  const bin = p.path("cli-bin");
  const installer = p.path("local-install.mjs");
  p.write(
    installer,
    `import { cpSync } from "node:fs";
const [group, action, stage] = process.argv.slice(2);
if (group === "plugin" && action === "install") cpSync(stage, process.env.INSTALL_DIR, { recursive: true });
`,
  );
  writeLauncher(bin, "agy", installer);
  const cache = p.path("cache");
  const env = {
    ...fixtureGitEnv({}),
    PATH: [
      bin,
      dirname(process.execPath),
      dirname(go),
      dirname(gitExecutable()),
      ...systemDirs(),
    ].join(delimiter),
    HOME: p.home,
    USERPROFILE: p.home,
    APPDATA: p.appdata,
    LOCALAPPDATA: p.path("local"),
    XDG_CONFIG_HOME: p.xdg,
    XDG_CACHE_HOME: cache,
    INSTALL_DIR: installed,
    AGX_NO_FETCH: "offline",
    npm_config_offline: "true",
    npm_config_update_notifier: "false",
    npm_config_cache: p.path("npm-cache"),
    GOCACHE:
      process.env.GOCACHE ||
      spawnSync(go, ["env", "GOCACHE"], {
        env: { ...process.env, GOTOOLCHAIN: "local", GOENV: "off" },
        encoding: "utf8",
      }).stdout.trim(),
    ...Object.fromEntries(
      [
        "SystemRoot",
        "ComSpec",
        "PATHEXT",
        "TEMP",
        "TMP",
        "TMPDIR",
        "PROCESSOR_ARCHITECTURE",
        "PROCESSOR_IDENTIFIER",
      ].flatMap((key) => (process.env[key] ? [[key, process.env[key]]] : [])),
    ),
  };
  const plan = spawnPlan(npm, ["run", "plugin:setup"], process.platform, env);
  const setup = () =>
    spawnSync(plan.file, plan.args, {
      cwd: f.root,
      env,
      windowsVerbatimArguments: plan.windowsVerbatimArguments,
      encoding: "utf8",
    });
  const result = setup();
  assert.ifError(result.error);
  assert.equal(result.status, 0, result.stdout + result.stderr);
  const manifest = readFileSync(join(installed, "hooks/SHA256SUMS"), "utf8");
  const lines = manifest.trimEnd().split("\n");
  assert.equal(lines.length, 6);
  assert.notEqual(manifest, trackedManifest.toString());
  for (const line of lines) {
    const [, digest, rel] =
      /^([a-f0-9]{64})  (bin\/agx-[\w.-]+)$/.exec(line) ?? [];
    assert.ok(rel, line);
    assert.equal(
      createHash("sha256")
        .update(readFileSync(join(installed, "hooks", rel)))
        .digest("hex"),
      digest,
    );
  }
  const launcher = join(
    installed,
    process.platform === "win32" ? "bin/agx.exe" : "bin/agx",
  );
  const help = spawnSync(launcher, ["help"], {
    cwd: p.dir,
    env,
    encoding: "utf8",
  });
  assert.ifError(help.error);
  assert.equal(help.status, 0, help.stderr);
  assert.ok(help.stdout.includes(newHelp), help.stdout);
  assert.deepEqual(readFileSync(f.path("hooks/SHA256SUMS")), trackedManifest);
  for (const rel of decoys)
    assert.equal(readFileSync(f.path(rel), "utf8"), `local decoy ${rel}`);
  f.git("diff", "--cached", "--exit-code");
  const stage = join(cache, stageName);
  for (const root of [stage, installed]) {
    const files = regularFiles(root);
    assert.ok(
      !files.includes("bin/AGX.EXE"),
      `copied decoy bin/AGX.EXE into ${root}`,
    );
    assert.ok(
      !files.some((rel) => rel.split("/").includes("local-only")),
      `copied decoy subtree into ${root}`,
    );
  }
  writeFileSync(join(stage, "sentinel"), "keep");
  f.write(main, "package main\ninvalid_go_source\n");
  const failed = setup();
  assert.ifError(failed.error);
  assert.notEqual(failed.status, 0, failed.stdout + failed.stderr);
  assert.match(failed.stderr, /invalid_go_source|syntax error/);
  assert.equal(readFileSync(join(stage, "sentinel"), "utf8"), "keep");
  assert.equal(readFileSync(join(stage, "hooks/SHA256SUMS"), "utf8"), manifest);
  assert.deepEqual(readdirSync(cache), [stageName]);

  const otherGo = p.path("other-go.mjs");
  p.write(
    otherGo,
    `const { GOTOOLCHAIN, GOPROXY, GOSUMDB } = process.env;
if (process.argv.slice(2).join(" ") !== "env GOVERSION" || [GOTOOLCHAIN,GOPROXY,GOSUMDB].join(" ") !== "local off off") process.exit(91);
console.log("go1.27.5");
`,
  );
  writeLauncher(bin, "go", otherGo);
  const wrongVersion = setup();
  assert.notEqual(wrongVersion.status, 0);
  assert.match(
    wrongVersion.stderr,
    /source-only setup requires local go1\.27\.4, found go1\.27\.5/,
  );
  assert.equal(readFileSync(join(stage, "sentinel"), "utf8"), "keep");
  assert.deepEqual(readdirSync(cache), [stageName]);
});
const closingLines = [
  "   run-hook.cmd needs the dispatcher, the manifest, and any",
  "   staged binary to agree; both guardian hooks fail closed",
  "   otherwise. Refusing to register a broken stage.",
];

/**
 * A checkout and a stage that verify cleanly: the stage holds the marketplace
 * manifests, the Antigravity plugin manifest, and the two hook scripts, and
 * both trees hold two of the six agx binaries (the Unix one mode 644 in the
 * stage), which SHA256SUMS lists by their real hashes beside zero hashes for
 * the other four.
 */
function verifyFixture(t: TestContext) {
  const root = temporary(t),
    repo = join(root, "repo"),
    stage = join(root, "stage");
  mkdirSync(join(repo, "hooks/bin"), { recursive: true });
  mkdirSync(join(stage, "hooks/bin"), { recursive: true });
  for (const manifest of [
    ".claude-plugin/marketplace.json",
    ".agents/plugins/marketplace.json",
    ".antigravity-plugin/plugin.json",
  ]) {
    mkdirSync(dirname(join(stage, manifest)), { recursive: true });
    writeFileSync(join(stage, manifest), "{}\n");
  }
  for (const file of ["run-hook.cmd", "fetch-hook.ps1"])
    writeFileSync(join(stage, "hooks", file), "fixture\n");
  const hashes: Record<string, string> = {};
  for (const binary of ["agx-linux-amd64", "agx-windows-amd64.exe"]) {
    const body = `fixture ${binary}\n`;
    writeFileSync(join(repo, "hooks/bin", binary), body);
    writeFileSync(join(stage, "hooks/bin", binary), body);
    hashes[binary] = createHash("sha256").update(body).digest("hex");
  }
  chmodSync(join(stage, "hooks/bin/agx-linux-amd64"), 0o644);
  chmodSync(join(stage, "hooks/bin/agx-windows-amd64.exe"), 0o755);
  const manifest = [
    `${zeroHash}  bin/agx-darwin-amd64`,
    `${zeroHash}  bin/agx-darwin-arm64`,
    `${hashes["agx-linux-amd64"]}  bin/agx-linux-amd64`,
    `${zeroHash}  bin/agx-linux-arm64`,
    `${hashes["agx-windows-amd64.exe"]}  bin/agx-windows-amd64.exe`,
    `${zeroHash}  bin/agx-windows-arm64.exe`,
  ];
  const writeManifest = (lines: string[]) =>
    writeFileSync(
      join(stage, "hooks/SHA256SUMS"),
      lines.map((line) => line + "\n").join(""),
    );
  writeManifest(manifest);
  const verify = (platform: NodeJS.Platform) => {
    const lines: string[] = [];
    const ok = verifyStage(repo, stage, {
      platform,
      write: (text) => lines.push(...text.split("\n").filter(Boolean)),
    });
    return { ok, lines };
  };
  return { repo, stage, manifest, writeManifest, verify };
}

/** Asserts a rejection whose failure lines are exactly `want`, in order. */
function assertRejected(
  result: { ok: boolean; lines: string[] },
  want: string[],
) {
  assert.equal(result.ok, false, result.lines.join("\n"));
  assert.deepEqual(
    result.lines.filter((line) => line.startsWith("❌")),
    want,
  );
  assert.deepEqual(result.lines.slice(-3), closingLines);
}

function assertAccepted(result: { ok: boolean; lines: string[] }) {
  assert.equal(result.ok, true, result.lines.join("\n"));
  assert.deepEqual(result.lines, []);
}

test("stage module: verify accepts a non-executable foreign binary on win32", (t) => {
  const f = verifyFixture(t);
  assertAccepted(f.verify("win32"));
});

test("stage module: verify rejects a binary missing from the stage", (t) => {
  const f = verifyFixture(t);
  rmSync(join(f.stage, "hooks/bin/agx-linux-amd64"));
  assertRejected(f.verify("win32"), [
    "❌ Marketplace stage: hooks/bin/agx-linux-amd64 is missing.",
  ]);
});

test(
  "stage module: verify rejects a non-executable Unix binary",
  {
    skip: posixOnly("execute bits"),
  },
  (t) => {
    const f = verifyFixture(t);
    assertRejected(f.verify("linux"), [
      "❌ Marketplace stage: hooks/bin/agx-linux-amd64 is not executable.",
    ]);
  },
);

test("stage module: verify rejects a binary that disagrees with SHA256SUMS", (t) => {
  const f = verifyFixture(t);
  writeFileSync(
    join(f.stage, "hooks/bin/agx-linux-amd64"),
    "fixture agx-linux-amd64\ntampered\n",
  );
  assertRejected(f.verify("win32"), [
    "❌ Marketplace stage: hooks/bin/agx-linux-amd64 does not match hooks/SHA256SUMS.",
  ]);
});

test("stage module: verify accepts a checkout and stage without hooks/bin", (t) => {
  const f = verifyFixture(t);
  rmSync(join(f.repo, "hooks/bin"), { recursive: true });
  rmSync(join(f.stage, "hooks/bin"), { recursive: true });
  assertAccepted(f.verify("win32"));
});

test("stage module: verify rejects a manifest missing an entry", (t) => {
  const f = verifyFixture(t);
  f.writeManifest(f.manifest.filter((line) => !line.includes("linux-arm64")));
  assertRejected(f.verify("win32"), [
    "❌ Marketplace stage: hooks/SHA256SUMS has no entry for bin/agx-linux-arm64.",
  ]);
});

// run-hook.cmd and fetch-hook.ps1 never match a binary-mode "*bin/" line, so
// the stage check must not count it as the entry either.
test("stage module: verify rejects a binary-mode manifest entry", (t) => {
  const f = verifyFixture(t);
  f.writeManifest(
    f.manifest.map((line) =>
      line.replace("  bin/agx-linux-arm64", " *bin/agx-linux-arm64"),
    ),
  );
  assertRejected(f.verify("win32"), [
    "❌ Marketplace stage: hooks/SHA256SUMS is not the six contract entries: unexpected entry *bin/agx-linux-arm64",
    "❌ Marketplace stage: hooks/SHA256SUMS has no entry for bin/agx-linux-arm64.",
  ]);
});

test("stage module: verify rejects an extra manifest entry", (t) => {
  const f = verifyFixture(t);
  f.writeManifest([...f.manifest, `${zeroHash}  bin/agx-plan9-amd64`]);
  assertRejected(f.verify("win32"), [
    "❌ Marketplace stage: hooks/SHA256SUMS is not the six contract entries: unexpected entry bin/agx-plan9-amd64",
  ]);
});

test("stage module: verify rejects a duplicate manifest entry", (t) => {
  const f = verifyFixture(t);
  f.writeManifest([
    ...f.manifest,
    f.manifest.find((line) => line.endsWith("linux-amd64"))!,
  ]);
  assertRejected(f.verify("win32"), [
    "❌ Marketplace stage: hooks/SHA256SUMS is not the six contract entries: duplicate entry bin/agx-linux-amd64",
  ]);
});

test("stage module: verify rejects a malformed manifest line", (t) => {
  const f = verifyFixture(t);
  f.writeManifest([...f.manifest, "not-a-manifest-line"]);
  assertRejected(f.verify("win32"), [
    "❌ Marketplace stage: hooks/SHA256SUMS is not the six contract entries: malformed line 7",
  ]);
});

// The marketplace files, the Antigravity manifest, both hook scripts and the
// manifest share one presence check.
test("stage module: verify rejects a stage missing a required file", (t) => {
  const f = verifyFixture(t);
  rmSync(join(f.stage, "hooks/run-hook.cmd"));
  assertRejected(f.verify("win32"), [
    "❌ Marketplace stage is missing hooks/run-hook.cmd.",
  ]);
});

test("stage module: verify rejects a stage missing .antigravity-plugin/plugin.json", (t) => {
  const f = verifyFixture(t);
  rmSync(join(f.stage, ".antigravity-plugin/plugin.json"));
  assertRejected(f.verify("win32"), [
    "❌ Marketplace stage is missing .antigravity-plugin/plugin.json.",
  ]);
});

// The launcher assertions in "stage keeps hook binaries executable" can fail:
// a stage without bin/ must report both launchers missing.
test("stage module: verify launcher check rejects a stage without bin", (t) => {
  const f = createStageFixture(t);
  const stage = temporary(t);
  mkdirSync(join(stage, "hooks"));
  writeFileSync(join(stage, "hooks/run-hook.cmd"), "fixture\n");
  const problems = launcherProblems(stage, readFileSync(f.path("bin/agx.exe")));
  for (const want of [
    "bin/agx is missing from the stage",
    "bin/agx.exe is missing from the stage",
  ])
    assert.ok(problems.includes(want), problems.join("\n"));
});

const lockName = ".agentics-local-marketplace.lock";

function gitEnv(extra: NodeJS.ProcessEnv = {}) {
  return { ...fixtureGitEnv(), ...extra };
}

/** Collects what the stage module writes, by stream. */
function sink() {
  const lines: { stream: "stdout" | "stderr"; text: string }[] = [];
  const text = (stream?: "stdout" | "stderr") =>
    lines
      .filter((line) => stream === undefined || line.stream === stream)
      .map((line) => line.text)
      .join("");
  return {
    out: (stream: "stdout" | "stderr", text: string) =>
      void lines.push({ stream, text }),
    text,
  };
}

type StageCall = { code: number; output: string; stdout: string };

async function refresh(
  f: StageFixture,
  cache: string,
  options: { copyHook?: () => void } = {},
): Promise<StageCall> {
  const s = sink();
  const code = await refreshStage({
    repoRoot: f.root,
    cacheRoot: cache,
    env: gitEnv(),
    out: s.out,
    copyHook: options.copyHook,
  });
  return { code, output: s.text(), stdout: s.text("stdout") };
}

async function remove(f: StageFixture, cache: string): Promise<StageCall> {
  const s = sink();
  const code = await removeStage({
    repoRoot: f.root,
    cacheRoot: cache,
    env: gitEnv(),
    out: s.out,
  });
  return { code, output: s.text(), stdout: s.text("stdout") };
}

function commitAll(f: StageFixture) {
  f.git("add", "-A");
  f.git(
    "-c",
    "user.email=fixture@agentics.test",
    "-c",
    "user.name=fixture",
    "commit",
    "-q",
    "-m",
    "change",
  );
}

/** The regular files below `root`, relative and with `/` separators. */
function regularFiles(root: string): string[] {
  return readdirSync(root, { recursive: true, withFileTypes: true })
    .filter((entry) => entry.isFile())
    .map((entry) =>
      relative(root, join(entry.parentPath, entry.name)).replaceAll("\\", "/"),
    )
    .sort();
}

test("stage module: refresh copies the tracked file list", async (t) => {
  const f = createStageFixture(t),
    cache = temporary(t);
  f.write("skills/deleted/SKILL.md", "committed then deleted\n");
  commitAll(f);
  rmSync(f.path("skills/deleted"), { recursive: true });
  f.write(".gitignore", "node_modules/\n.claude/worktrees/\n*.ignored\n");
  f.write("notes.ignored", "ignored\n");
  f.write("skills/untracked/SKILL.md", "untracked\n");
  const want = git(
    f.root,
    "ls-files",
    "-z",
    "--cached",
    "--others",
    "--exclude-standard",
  )
    .split("\0")
    .filter((path) => path !== "" && path !== "skills/deleted/SKILL.md")
    .sort();
  assert.ok(want.includes("skills/untracked/SKILL.md"));

  const result = await refresh(f, cache);
  assert.equal(result.code, 0, result.output);
  const stage = join(cache, stageName);
  assert.deepEqual(regularFiles(stage), want);
  assert.ok(existsSync(join(stage, "skills/untracked/SKILL.md")));
  for (const path of [
    "notes.ignored",
    ".git",
    "skills/deleted",
    "node_modules",
  ])
    assertAbsent(join(stage, path));
  assert.match(result.stdout, /^Staged \S+ of tracked plugin content\.\n$/);
  assertAbsent(join(cache, lockName));
});

test("stage fixture directs real Git commands away from an inherited checkout", async (t) => {
  const outside = temporary(t);
  const workingFile = join(outside, "outside.txt");
  writeFileSync(workingFile, "outside commit\n");
  git(outside, "init", "-q");
  git(outside, "config", "--local", "core.hooksPath", "/dev/null");
  git(outside, "add", "--", "outside.txt");
  git(outside, "commit", "-qm", "outside");
  writeFileSync(workingFile, "outside working-tree edit\n");
  const gitDir = join(outside, ".git");
  const index = join(gitDir, "index");
  const snapshot = () => ({
    head: git(outside, "rev-parse", "HEAD"),
    index: readFileSync(index),
    bytes: readFileSync(workingFile),
  });
  const before = snapshot();
  const hostile = {
    GIT_DIR: gitDir,
    GIT_WORK_TREE: outside,
    GIT_INDEX_FILE: index,
    GIT_AUTHOR_NAME: "Outside",
    GIT_AUTHOR_EMAIL: "outside@example.invalid",
    GIT_CONFIG_COUNT: "1",
    GIT_CONFIG_KEY_0: "core.autocrlf",
    GIT_CONFIG_VALUE_0: "true",
  };
  const saved = Object.entries(hostile).map(([key, value]) => {
    const previous = process.env[key];
    process.env[key] = value;
    return [key, previous] as const;
  });
  try {
    const f = createStageFixture(t);
    assert.deepEqual(
      snapshot(),
      before,
      "fixture setup changed the outside checkout",
    );
    assert.equal(
      git(f.root, "log", "-1", "--format=%an <%ae>").trim(),
      "fixture <fixture@agentics.test>",
    );
    const cache = temporary(t);
    const trace = join(cache, "git.trace");
    const s = sink();
    const result = await refreshStage({
      repoRoot: f.root,
      cacheRoot: cache,
      env: gitEnv({ GIT_TRACE: trace }),
      out: s.out,
    });
    assert.equal(result, 0, s.text());
    assert.match(
      readFileSync(trace, "utf8"),
      /built-in: git ls-files -z -t --cached --others --exclude-standard/,
    );
    const staged = join(cache, stageName);
    assert.deepEqual(
      readFileSync(join(staged, "skills/committed/SKILL.md")),
      Buffer.from("committed\n"),
    );
    assertAbsent(join(staged, "outside.txt"));
    assert.deepEqual(
      snapshot(),
      before,
      "stage refresh changed the outside checkout",
    );
  } finally {
    for (const [key, value] of saved) {
      if (value === undefined) delete process.env[key];
      else process.env[key] = value;
    }
  }
});

test("stage module: refresh with a bad agx checksum empties the leaf and releases the lock", async (t) => {
  const f = createStageFixture(t),
    cache = temporary(t);
  let result = await refresh(f, cache);
  assert.equal(result.code, 0, result.output);
  f.write("hooks/bin/agx-darwin-arm64", "#!/bin/sh\ntampered\n", 0o755);
  result = await refresh(f, cache);
  assert.notEqual(result.code, 0);
  assert.ok(
    result.output.includes(
      "❌ Marketplace stage: hooks/bin/agx-darwin-arm64 does not match hooks/SHA256SUMS.\n",
    ),
    result.output,
  );
  const leaf = join(cache, stageName);
  assert.ok(lstatSync(leaf).isDirectory());
  assert.deepEqual(readdirSync(leaf), []);
  assertAbsent(join(cache, lockName));
});

test("stage module: refresh refuses while the lock is held", async (t) => {
  const f = createStageFixture(t),
    cache = temporary(t),
    lock = join(cache, lockName),
    leaf = join(cache, stageName);
  mkdirSync(lock);
  mkdirSync(leaf);
  writeFileSync(join(leaf, "sentinel"), "previous stage");
  for (const action of [refresh, remove]) {
    const result = await action(f, cache);
    assert.notEqual(result.code, 0);
    assert.ok(
      result.output.includes(
        "ERROR: marketplace stage is locked or its lock cannot be created\n",
      ),
      result.output,
    );
    assert.ok(statSync(lock).isDirectory());
    assert.deepEqual(readdirSync(leaf), ["sentinel"]);
    assert.equal(
      readFileSync(join(leaf, "sentinel"), "utf8"),
      "previous stage",
    );
  }
});

test(
  "stage module: refresh keeps modes and symlinks",
  { skip: posixOnly("execute bits and symlinks") },
  async (t) => {
    const f = createStageFixture(t),
      cache = temporary(t),
      outside = temporary(t);
    const target = join(outside, "target.txt");
    writeFileSync(target, "outside target\n");
    f.write("tools/run-script", "#!/bin/sh\n");
    chmodSync(f.path("tools/run-script"), 0o755);
    f.write("tools/plain.txt", "plain\n");
    chmodSync(f.path("tools/plain.txt"), 0o644);
    mkdirSync(f.path("links"));
    symlinkSync("../tools/plain.txt", f.path("links/relative"));
    symlinkSync(target, f.path("links/absolute"));
    commitAll(f);

    const result = await refresh(f, cache);
    assert.equal(result.code, 0, result.output);
    const stage = join(cache, stageName);
    assert.equal(
      lstatSync(join(stage, "tools/run-script")).mode & 0o7777,
      0o755,
    );
    assert.equal(
      lstatSync(join(stage, "tools/plain.txt")).mode & 0o7777,
      0o644,
    );
    for (const link of ["links/relative", "links/absolute"]) {
      assert.ok(lstatSync(join(stage, link)).isSymbolicLink(), link);
      assert.equal(
        readlinkSync(join(stage, link)),
        readlinkSync(f.path(link)),
        link,
      );
    }
    assert.equal(readFileSync(target, "utf8"), "outside target\n");
    assert.deepEqual(readdirSync(outside), ["target.txt"]);
  },
);

test("stage module: refresh refuses a source that is not a Git checkout", async (t) => {
  const f = createStageFixture(t),
    plain = temporary(t),
    cache = join(temporary(t), "cache");
  writeFileSync(join(plain, "file.txt"), "not a checkout\n");
  const s = sink();
  const code = await refreshStage({
    repoRoot: plain,
    cacheRoot: cache,
    // Git must not find a repository above the plain directory.
    env: gitEnv({ GIT_CEILING_DIRECTORIES: dirname(plain) }),
    out: s.out,
  });
  assert.notEqual(code, 0);
  assert.ok(
    s.text().includes("ERROR: marketplace source is not a Git checkout\n"),
    s.text(),
  );
  assertAbsent(cache);
});

test("stage module: refresh and remove reject an unowned leaf and preserve an existing lock", async (t) => {
  const f = createStageFixture(t);
  for (const kind of ["symlink", "file", "lock"]) {
    const cache = temporary(t),
      outside = temporary(t);
    const sentinel = join(outside, "sentinel"),
      leaf = join(cache, stageName),
      lock = join(cache, lockName);
    writeFileSync(sentinel, "preserve", { mode: 0o600 });
    let diagnostic = "marketplace stage leaf is not an owned directory";
    // A junction needs no privilege on Windows; POSIX ignores the type.
    if (kind === "symlink") symlinkSync(outside, leaf, "junction");
    else if (kind === "file")
      writeFileSync(leaf, "preserve leaf", { mode: 0o600 });
    else {
      mkdirSync(lock, { mode: 0o700 });
      diagnostic = "marketplace stage is locked";
    }
    for (const action of [refresh, remove]) {
      const result = await action(f, cache);
      assert.notEqual(result.code, 0, `${kind}: ${result.output}`);
      assert.ok(
        result.output.includes(diagnostic),
        `${kind}: ${result.output}`,
      );
      assert.equal(readFileSync(sentinel, "utf8"), "preserve");
      assert.deepEqual(readdirSync(outside), ["sentinel"]);
      if (kind === "lock") assert.ok(statSync(lock).isDirectory());
      else assertAbsent(lock);
      if (kind === "file")
        assert.equal(readFileSync(leaf, "utf8"), "preserve leaf");
    }
  }
});

test("stage module: refresh and remove follow the canonical cycle", async (t) => {
  const f = createStageFixture(t),
    base = temporary(t),
    cache = join(base, "missing cache", "nested");
  let result = await refresh(f, cache);
  assert.equal(result.code, 0, result.output);
  assert.ok(statSync(cache).isDirectory());
  const stage = join(cache, stageName),
    stale = join(stage, "old-only");
  writeFileSync(stale, "old");
  result = await refresh(f, cache);
  assert.equal(result.code, 0, result.output);
  assertAbsent(stale);

  const physicalStage = join(realpathSync.native(cache), stageName);
  result = await remove(f, cache);
  assert.equal(result.code, 0, result.output);
  assert.equal(
    result.stdout,
    `Removing local marketplace stage (${physicalStage})...\n`,
  );
  assertAbsent(stage);
  assertAbsent(join(cache, lockName));
  result = await remove(f, cache);
  assert.equal(result.code, 0, result.output);
  assert.equal(result.output, "");

  const absent = join(base, "absent cache", "nested");
  result = await remove(f, absent);
  assert.equal(result.code, 0, result.output);
  assertAbsent(join(base, "absent cache"));
});

test(
  "stage module: refresh stays anchored after leaf replacement",
  { skip: posixOnly("a working directory cannot be renamed on Windows") },
  async (t) => {
    const f = createStageFixture(t),
      cache = temporary(t),
      outside = temporary(t);
    writeFileSync(join(outside, "sentinel"), "outside unchanged", {
      mode: 0o600,
    });
    const leaf = join(cache, stageName);
    const result = await refresh(f, cache, {
      copyHook: () => {
        renameSync(leaf, join(cache, "held-original"));
        symlinkSync(outside, leaf);
      },
    });
    assert.notEqual(result.code, 0);
    assert.ok(
      result.output.includes(
        "ERROR: marketplace stage leaf changed during refresh\n",
      ),
      result.output,
    );
    assert.equal(
      readFileSync(join(outside, "sentinel"), "utf8"),
      "outside unchanged",
    );
    assert.deepEqual(readdirSync(outside), ["sentinel"]);
    assert.ok(
      existsSync(join(cache, "held-original/.claude-plugin/marketplace.json")),
    );
    assertAbsent(join(cache, lockName));
  },
);

// The root check runs before any lock, copy or removal: a cache root inside
// the checkout must leave an existing leaf, the lock and the tool cache alone.
test("stage module: an invalid cache root stops refresh and remove before any change", async (t) => {
  const f = createStageFixture(t);
  f.write(`${stageName}/sentinel`, "preserve");
  let copied = false;
  const s = sink();
  const refreshed = await refreshStage({
    repoRoot: f.root,
    cacheRoot: f.root,
    env: gitEnv(),
    out: s.out,
    copyHook: () => {
      copied = true;
    },
  });
  const removed = await removeStage({
    repoRoot: f.root,
    cacheRoot: f.root,
    env: gitEnv(),
    out: s.out,
  });
  assert.notEqual(refreshed, 0);
  assert.notEqual(removed, 0);
  assert.equal(s.text(), "ERROR: invalid marketplace cache root\n".repeat(2));
  assert.equal(copied, false);
  assert.deepEqual(readdirSync(f.path(stageName)), ["sentinel"]);
  assert.equal(
    readFileSync(f.path(`${stageName}/sentinel`), "utf8"),
    "preserve",
  );
  for (const name of [lockName, "agentics"]) assertAbsent(f.path(name));
});

// The index still lists a/b after a was replaced in the working tree by a
// link, and git lists the untracked link before the index entries. The stage
// recreates a as a link first, so writing a/b next would land in the link's
// target, outside the leaf. The copy must refuse instead.
test(
  "stage module: refresh refuses to write below a parent that became a link",
  { skip: posixOnly("a tracked file below a symlinked directory") },
  async (t) => {
    const f = createStageFixture(t),
      cache = temporary(t);
    f.write("a/b", "tracked\n");
    commitAll(f);
    rmSync(f.path("a"), { recursive: true });
    // The same relative text names a directory beside the checkout as the
    // source and a directory beside the leaf in the stage.
    const source = join(dirname(f.root), "outside");
    mkdirSync(source);
    writeFileSync(join(source, "b"), "read through the link\n");
    symlinkSync("../outside", f.path("a"));
    const target = join(cache, "outside");
    mkdirSync(target);

    const result = await refresh(f, cache);
    assert.notEqual(result.code, 0, result.output);
    assert.ok(
      result.output.includes("ERROR: marketplace stage copy failed:"),
      result.output,
    );
    assert.deepEqual(readdirSync(target), []);
    assert.deepEqual(readdirSync(source), ["b"]);
    assert.equal(
      readFileSync(join(source, "b"), "utf8"),
      "read through the link\n",
    );
    assert.deepEqual(readdirSync(join(cache, stageName)), []);
    assertAbsent(join(cache, lockName));
  },
);

test("stage module: the caller's working directory is restored", async (t) => {
  const f = createStageFixture(t),
    cache = temporary(t),
    caller = temporary(t);
  const lock = join(cache, lockName);
  const before = process.cwd();
  process.chdir(caller);
  try {
    const expect = (label: string, call: StageCall, ok: boolean) => {
      assert.equal(call.code === 0, ok, `${label}: ${call.output}`);
      assert.equal(process.cwd(), caller, label);
    };
    expect("refresh", await refresh(f, cache), true);
    mkdirSync(lock);
    expect("refused refresh", await refresh(f, cache), false);
    expect("refused remove", await remove(f, cache), false);
    rmSync(lock, { recursive: true });
    f.write("hooks/bin/agx-darwin-arm64", "#!/bin/sh\ntampered\n", 0o755);
    expect("failed refresh", await refresh(f, cache), false);
    expect("remove", await remove(f, cache), true);
  } finally {
    process.chdir(before);
  }
});

test("stage module: a failed refresh empties the leaf before it releases the lock", async (t) => {
  const f = createStageFixture(t),
    cache = temporary(t);
  f.write("hooks/bin/agx-darwin-arm64", "#!/bin/sh\ntampered\n", 0o755);
  const calls: string[] = [];
  const rm = fsp.rm,
    rmdir = fs.rmdirSync;
  t.mock.method(fsp, "rm", (...args: Parameters<typeof fsp.rm>) => {
    calls.push(`rm ${String(args[0])}`);
    return rm(...args);
  });
  // The stage imports rmdirSync by name, so the mock is copied into the binding.
  t.mock.method(fs, "rmdirSync", (...args: Parameters<typeof fs.rmdirSync>) => {
    calls.push(`rmdir ${String(args[0])}`);
    return rmdir(...args);
  });
  syncBuiltinESMExports();
  let result: StageCall;
  try {
    result = await refresh(f, cache);
  } finally {
    t.mock.restoreAll();
    syncBuiltinESMExports();
  }
  assert.notEqual(result.code, 0);
  const release = calls.indexOf(`rmdir ${lockName}`);
  assert.equal(release, calls.length - 1, calls.join("\n"));
  // The leaf is replaced first, then emptied entry by entry from inside it.
  assert.equal(calls[0], `rm ${stageName}`);
  for (const entry of [".claude-plugin", "hooks", "skills"])
    assert.ok(calls.indexOf(`rm ${entry}`) > 0, calls.join("\n"));
  assert.deepEqual(readdirSync(join(cache, stageName)), []);
});
