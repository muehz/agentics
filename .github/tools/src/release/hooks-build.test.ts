import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import fs from "node:fs";
import {
  chmodSync,
  copyFileSync,
  existsSync,
  mkdirSync,
  readdirSync,
  readFileSync,
  renameSync,
  statSync,
  symlinkSync,
  writeFileSync,
} from "node:fs";
import { syncBuiltinESMExports } from "node:module";
import { basename, delimiter, dirname, isAbsolute, join } from "node:path";
import { pathToFileURL } from "node:url";
import { test } from "node:test";
import {
  checksumAssetNames,
  CHECKSUMS_NAME,
  expectedAssetNames,
  HOOK_BINARIES,
  NOTICES_NAME,
  tarballName,
  pluginTarballName,
} from "./assets.ts";
import {
  buildCommittedHooks,
  checkBuildToolchain,
  pinnedHookToolchain,
  readHookManifest,
} from "./hooks-build.ts";
import {
  binaryContent,
  createHookFixture,
  hookEnv,
  LAUNCHER,
  MAIN_GO,
  PINNED_GO,
  sha256,
  type HookFixture,
} from "./testing/hooks-fixture.ts";
import {
  commitFiles,
  platformSkip,
  posix,
  scratchDir,
  sh,
} from "./testing/support.ts";

const needsPosix = platformSkip(
  "a POSIX shell for the stand-in verifier",
  posix,
  "run these tests on Linux or macOS",
);

// A stand-in `go` made from the running Node, with a scoped preload that answers only the version
// query. Windows needs a native executable, so it gets a copy and the preload compares real paths.
// POSIX links to the interpreter instead: a copy cannot run when Node loads a shared library from
// beside its own binary, as the Homebrew build does. The link shares the real path with Node, so
// the preload tells the two apart by the name the process was started under.
function versionFixture(aliased = false) {
  const directory = scratchDir("agentics-controlled-go-version-");
  const root = aliased ? join(directory, "alias") : directory;
  if (aliased) {
    const physical = join(directory, "physical");
    mkdirSync(physical);
    symlinkSync(
      physical,
      root,
      process.platform === "win32" ? "junction" : "dir",
    );
  }
  const bin = join(root, "bin");
  mkdirSync(bin);
  const go = join(bin, process.platform === "win32" ? "go.exe" : "go");
  if (process.platform === "win32") {
    copyFileSync(process.execPath, go);
    chmodSync(go, 0o755);
  } else {
    symlinkSync(process.execPath, go);
  }
  const preload = join(root, "go-version.mjs");
  writeFileSync(
    preload,
    `import fs from "node:fs";
import path from "node:path";
const sameExecutable = process.platform === "win32"
  ? fs.realpathSync(process.execPath).toLowerCase() === fs.realpathSync(${JSON.stringify(go)}).toLowerCase()
  : path.basename(process.argv0) === "go";
if (sameExecutable) {
  const args = process.argv.slice(1);
  if (args.length !== 2 || path.basename(args[0]) !== "version") {
    console.error("controlled go: unexpected arguments"); process.exit(2);
  }
  if (fs.readFileSync(args[1], "utf8") !== "controlled Go binary\\n") {
    console.error("not a Go binary"); process.exit(1);
  }
  console.log(args[1] + ": go${PINNED_GO}"); process.exit(0);
}
`,
  );
  return {
    root,
    env: {
      ...process.env,
      PATH: `${bin}${delimiter}${process.env.PATH ?? ""}`,
      NODE_OPTIONS:
        `${process.env.NODE_OPTIONS ?? ""} --import=${pathToFileURL(preload).href}`.trim(),
    },
  };
}

async function rejectsMatching(
  promise: Promise<unknown>,
  ...patterns: RegExp[]
): Promise<void> {
  await assert.rejects(promise, (error: unknown) => {
    const message = error instanceof Error ? error.message : String(error);
    for (const pattern of patterns) assert.match(message, pattern);
    return true;
  });
}

function emptyOut(fixture: HookFixture): string {
  const out = join(fixture.root, "out");
  mkdirSync(out);
  return out;
}

function commitManifest(
  fixture: HookFixture,
  edit: (manifest: string) => string,
  message: string,
): string {
  const manifest = sh(
    fixture.repo,
    "git",
    "show",
    `${fixture.sha}:hooks/SHA256SUMS`,
  );
  return commitFiles(
    fixture.repo,
    { "hooks/SHA256SUMS": edit(manifest) },
    message,
  );
}

test("the roster contains the hook binaries, notices, source and plugin archives, and SHA256SUMS", () => {
  const tag = "v1.2.3";
  assert.equal(HOOK_BINARIES.length, 6);
  assert.deepEqual(
    checksumAssetNames(tag),
    [
      NOTICES_NAME,
      ...HOOK_BINARIES,
      tarballName(tag),
      pluginTarballName(tag),
    ].sort(),
  );
  assert.deepEqual(
    expectedAssetNames(tag),
    [
      CHECKSUMS_NAME,
      NOTICES_NAME,
      ...HOOK_BINARIES,
      tarballName(tag),
      pluginTarballName(tag),
    ].sort(),
  );
});

test(
  "buildCommittedHooks writes the six built binaries and nothing else",
  { skip: needsPosix },
  async () => {
    const fixture = createHookFixture();
    const out = emptyOut(fixture);
    await buildCommittedHooks(fixture.repo, fixture.sha, out, hookEnv(fixture));
    assert.deepEqual(readdirSync(out).sort(), [...HOOK_BINARIES].sort());
    for (const name of HOOK_BINARIES) {
      assert.deepEqual(
        readFileSync(join(out, name)),
        binaryContent(name),
        name,
      );
      assert.equal(
        statSync(join(out, name)).mode & 0o111,
        0,
        `${name} is written without an executable bit`,
      );
    }
  },
);

test(
  "buildCommittedHooks builds the commit, not the working tree",
  { skip: needsPosix },
  async () => {
    const fixture = createHookFixture();
    const committedManifest = sh(
      fixture.repo,
      "git",
      "show",
      `${fixture.sha}:hooks/SHA256SUMS`,
    );
    writeFileSync(join(fixture.repo, "hooks/go/cmd/hook/main.go"), "not go\n");
    writeFileSync(join(fixture.repo, "hooks/SHA256SUMS"), "junk\n");
    writeFileSync(join(fixture.repo, "untracked.txt"), "untracked\n");
    const out = emptyOut(fixture);
    await buildCommittedHooks(fixture.repo, fixture.sha, out, hookEnv(fixture));
    assert.equal(readFileSync(`${fixture.record}.main`, "utf8"), MAIN_GO);
    assert.equal(
      readFileSync(`${fixture.record}.manifest`, "utf8"),
      committedManifest,
    );
    assert.equal(readdirSync(out).length, HOOK_BINARIES.length);
  },
);

test(
  "buildCommittedHooks reads the inspected launcher after its name is replaced",
  { skip: needsPosix },
  async (t) => {
    const fixture = createHookFixture();
    const out = emptyOut(fixture);
    let replaced = false;
    const stat = fs.lstatSync;
    const inspect = t.mock.method(
      fs,
      "lstatSync",
      (...args: Parameters<typeof stat>) => {
        const info = stat(...args);
        const [file] = args;
        if (
          !replaced &&
          typeof file === "string" &&
          basename(file) === "agx.exe" &&
          basename(dirname(dirname(file))).startsWith("build-")
        ) {
          renameSync(file, join(fixture.root, "inspected-launcher"));
          writeFileSync(file, "replacement launcher");
          replaced = true;
        }
        return info;
      },
    );
    syncBuiltinESMExports();
    try {
      const launcher = await buildCommittedHooks(
        fixture.repo,
        fixture.sha,
        out,
        hookEnv(fixture),
      );
      assert.ok(replaced, "the fixture replaced the checked launcher");
      assert.deepEqual(launcher, Buffer.from(LAUNCHER));
    } finally {
      inspect.mock.restore();
      syncBuiltinESMExports();
    }
  },
);

test(
  "buildCommittedHooks exports the source without line-ending conversion",
  { skip: needsPosix },
  async () => {
    const fixture = createHookFixture();
    commitFiles(
      fixture.repo,
      { ".gitattributes": "* text=auto\n" },
      "normalize text",
    );
    const sha = sh(fixture.repo, "git", "rev-parse", "HEAD").trim();
    // The setting a Windows checkout carries: without the pin in the archive command, git writes CRLF.
    sh(fixture.repo, "git", "config", "core.autocrlf", "true");
    const unpinned = sh(
      fixture.repo,
      "git",
      "-c",
      "core.autocrlf=true",
      "archive",
      "--format=tar",
      sha,
      "hooks/go/cmd/hook/main.go",
    );
    assert.ok(
      unpinned.includes("\r\n"),
      "the fixture must make an unpinned export convert line endings",
    );
    await buildCommittedHooks(
      fixture.repo,
      sha,
      emptyOut(fixture),
      // Let this scenario's local autocrlf setting reach the production Git call.
      { ...hookEnv(fixture), GIT_CONFIG_COUNT: undefined },
    );
    assert.equal(readFileSync(`${fixture.record}.main`, "utf8"), MAIN_GO);
  },
);

for (const mode of [
  "correct",
  "wrong",
  "missing",
  "first-good",
  "first-bad",
] as const) {
  test(
    `buildCommittedHooks checks the built bytes against the committed manifest when the verifier reports success: ${mode}`,
    { skip: needsPosix },
    async () => {
      const fixture = createHookFixture();
      let sha = fixture.sha;
      if (mode === "first-good" || mode === "first-bad") {
        const bad = `${"0".repeat(64)}  bin/${HOOK_BINARIES[0]}\n`;
        sha = commitManifest(
          fixture,
          (manifest) =>
            mode === "first-good" ? manifest + bad : bad + manifest,
          "duplicate manifest",
        );
      }
      const out = emptyOut(fixture);
      const outcome = buildCommittedHooks(
        fixture.repo,
        sha,
        out,
        hookEnv(fixture, {
          MODE: mode === "wrong" || mode === "missing" ? mode : "correct",
        }),
      );
      if (mode === "correct" || mode === "first-good") {
        await outcome;
        assert.equal(readdirSync(out).length, HOOK_BINARIES.length);
      } else {
        await rejectsMatching(
          outcome,
          mode === "missing"
            ? /reading built agx-darwin-amd64/
            : /agx-darwin-amd64 built as sha256/,
        );
        assert.deepEqual(
          readdirSync(out),
          [],
          "a refusal must leave the output directory untouched",
        );
      }
      const recorded = readFileSync(fixture.record, "utf8");
      assert.ok(isAbsolute(recorded), "the verifier gets an absolute --out");
      assert.notEqual(
        recorded,
        out,
        "the verifier builds into a private directory, not the bundle",
      );
    },
  );
}

test(
  "buildCommittedHooks has the verifier rebuild the committed bin/agx.exe and ships no launcher",
  { skip: needsPosix },
  async () => {
    const fixture = createHookFixture();
    // A dirty working-tree launcher must not reach the verifier: it compares against the commit.
    writeFileSync(join(fixture.repo, "bin/agx.exe"), "working tree\n");
    const out = emptyOut(fixture);
    await buildCommittedHooks(fixture.repo, fixture.sha, out, hookEnv(fixture));
    const launcher = readFileSync(`${fixture.record}.launcher`, "utf8");
    assert.ok(isAbsolute(launcher), "the verifier gets an absolute --launcher");
    assert.ok(
      launcher.endsWith(join("bin", "agx.exe")),
      `the --launcher path names bin/agx.exe: ${launcher}`,
    );
    assert.equal(readFileSync(`${fixture.record}.committed`, "utf8"), LAUNCHER);
    assert.deepEqual(
      readdirSync(out).sort(),
      [...HOOK_BINARIES].sort(),
      "the launcher is verified, never shipped as an asset",
    );
    assert.ok(!existsSync(join(out, "bin")));
    assert.ok(!existsSync(join(out, "agx.exe")));
  },
);

for (const mode of ["launcher-wrong", "launcher-missing"] as const) {
  test(
    `buildCommittedHooks refuses a verifier whose rebuilt launcher is not the committed one: ${mode}`,
    { skip: needsPosix },
    async () => {
      const fixture = createHookFixture();
      const out = emptyOut(fixture);
      await rejectsMatching(
        buildCommittedHooks(
          fixture.repo,
          fixture.sha,
          out,
          hookEnv(fixture, { MODE: mode }),
        ),
        mode === "launcher-missing"
          ? /^reading built bin\/agx\.exe: /
          : /^bin\/agx\.exe built from hooks\/go does not match the committed bin\/agx\.exe$/,
      );
      assert.deepEqual(
        readdirSync(out),
        [],
        "a refusal must leave the output directory untouched",
      );
    },
  );
}

test(
  "buildCommittedHooks refuses a committed bin/agx.exe that cmd/launcher does not reproduce",
  { skip: needsPosix },
  async () => {
    const fixture = createHookFixture();
    const sha = commitFiles(
      fixture.repo,
      { "bin/agx.exe": "a stale launcher\n" },
      "stale launcher",
    );
    const out = emptyOut(fixture);
    await rejectsMatching(
      buildCommittedHooks(fixture.repo, sha, out, hookEnv(fixture)),
      /^bin\/agx\.exe built from hooks\/go does not match the committed bin\/agx\.exe$/,
    );
    assert.equal(
      readFileSync(`${fixture.record}.committed`, "utf8"),
      "a stale launcher\n",
      "the verifier compares against the launcher at the target commit",
    );
    assert.deepEqual(readdirSync(out), []);
  },
);

test(
  "buildCommittedHooks derives all seven executables from a source-only commit",
  { skip: needsPosix },
  async () => {
    const fixture = createHookFixture();
    sh(fixture.repo, "git", "rm", "--quiet", "bin/agx.exe");
    sh(fixture.repo, "git", "commit", "--quiet", "-m", "drop the launcher");
    const sha = sh(fixture.repo, "git", "rev-parse", "HEAD").trim();
    const out = emptyOut(fixture);
    const launcher = await buildCommittedHooks(
      fixture.repo,
      sha,
      out,
      hookEnv(fixture),
    );
    assert.deepEqual(launcher, Buffer.from(LAUNCHER));
    assert.deepEqual(readdirSync(out).sort(), [...HOOK_BINARIES].sort());
    for (const name of HOOK_BINARIES)
      assert.deepEqual(readFileSync(join(out, name)), binaryContent(name));
  },
);

test(
  "buildCommittedHooks refuses wrong tracked runtime bytes even when the build matches the manifest",
  { skip: needsPosix },
  async () => {
    const fixture = createHookFixture();
    const path = "hooks/bin/agx-linux-arm64";
    const sha = commitFiles(fixture.repo, { [path]: "wrong" }, "stale runtime");
    writeFileSync(join(fixture.repo, path), binaryContent("agx-linux-arm64"));
    const out = emptyOut(fixture);
    await rejectsMatching(
      buildCommittedHooks(fixture.repo, sha, out, hookEnv(fixture)),
      /does not match the committed hooks\/bin\/agx-linux-arm64/,
    );
    assert.deepEqual(readdirSync(out), []);
  },
);

test(
  "buildCommittedHooks refuses nonregular and unexpected entries in the selected artifact tree",
  { skip: needsPosix },
  async () => {
    const fixture = createHookFixture();
    const out = emptyOut(fixture);
    const path = "hooks/bin/agx-linux-amd64";
    mkdirSync(join(fixture.repo, "hooks/bin"));
    symlinkSync("../SHA256SUMS", join(fixture.repo, path));
    const linked = commitFiles(fixture.repo, {}, "linked runtime");
    await rejectsMatching(
      buildCommittedHooks(fixture.repo, linked, out, hookEnv(fixture)),
      /expected a regular Git blob at hooks\/bin\/agx-linux-amd64/,
    );
    sh(fixture.repo, "git", "rm", path);
    const extra = commitFiles(
      fixture.repo,
      { "hooks/bin/extra": "extra" },
      "extra runtime",
    );
    await rejectsMatching(
      buildCommittedHooks(fixture.repo, extra, out, hookEnv(fixture)),
      /unexpected executable path in the selected tree: hooks\/bin\/extra/,
    );
    assert.ok(!existsSync(fixture.record));
    assert.deepEqual(readdirSync(out), []);
  },
);

test(
  "buildCommittedHooks surfaces an incompatible trusted verifier for a source-only commit",
  { skip: needsPosix },
  async () => {
    const fixture = createHookFixture();
    sh(fixture.repo, "git", "rm", "bin/agx.exe");
    const sha = commitFiles(fixture.repo, {}, "source only");
    writeFileSync(
      fixture.verifier,
      '#!/bin/sh\necho "unsupported flag $8" >&2\nexit 64\n',
    );
    const out = emptyOut(fixture);
    await rejectsMatching(
      buildCommittedHooks(fixture.repo, sha, out, hookEnv(fixture)),
      /trusted hook verifier: verifier exited with code 64: unsupported flag --build-launcher/,
    );
    assert.deepEqual(readdirSync(out), []);
  },
);

test(
  "buildCommittedHooks refuses a stale committed manifest and names the binary and the manifest",
  { skip: needsPosix },
  async () => {
    const fixture = createHookFixture();
    const sha = commitManifest(
      fixture,
      (manifest) => "0".repeat(64) + manifest.slice(64),
      "stale manifest",
    );
    const out = emptyOut(fixture);
    await rejectsMatching(
      buildCommittedHooks(fixture.repo, sha, out, hookEnv(fixture)),
      /agx-darwin-amd64/,
      /hooks\/SHA256SUMS records/,
    );
    assert.deepEqual(readdirSync(out), []);
  },
);

test(
  "buildCommittedHooks refuses a manifest with no entry for a contract binary",
  { skip: needsPosix },
  async () => {
    const fixture = createHookFixture();
    const sha = commitManifest(
      fixture,
      (manifest) =>
        manifest
          .split("\n")
          .filter((line) => !line.endsWith("bin/agx-linux-amd64"))
          .join("\n"),
      "drop an entry",
    );
    await rejectsMatching(
      buildCommittedHooks(
        fixture.repo,
        sha,
        emptyOut(fixture),
        hookEnv(fixture),
      ),
      /hooks\/SHA256SUMS has no entry for bin\/agx-linux-amd64/,
    );
  },
);

test(
  "buildCommittedHooks forwards the verifier's diagnostic and names hooks/go/go.mod in it",
  { skip: needsPosix },
  async () => {
    const fixture = createHookFixture();
    const out = emptyOut(fixture);
    await rejectsMatching(
      buildCommittedHooks(
        fixture.repo,
        fixture.sha,
        out,
        hookEnv(fixture, { MODE: "fail" }),
      ),
      /^trusted hook verifier: verifier exited with code 3: /,
      /built with go1\.26\.0, but hooks\/go\/go\.mod pins go1\.27\.3/,
    );
    assert.deepEqual(readdirSync(out), []);
  },
);

test(
  "buildCommittedHooks refuses a verifier that is missing, relative or unset",
  { skip: needsPosix },
  async () => {
    const fixture = createHookFixture();
    const base = hookEnv(fixture);
    await rejectsMatching(
      buildCommittedHooks(fixture.repo, fixture.sha, emptyOut(fixture), {
        ...base,
        AGX_HOOK_VERIFIER: join(fixture.root, "absent-verifier"),
      }),
      /^trusted hook verifier: /,
    );
    for (const verifier of ["verifier", "", undefined]) {
      await rejectsMatching(
        buildCommittedHooks(
          fixture.repo,
          fixture.sha,
          join(fixture.root, "unused"),
          { ...base, AGX_HOOK_VERIFIER: verifier },
        ),
        /^AGX_HOOK_VERIFIER must name an absolute trusted verifier$/,
      );
    }
  },
);

test(
  "buildCommittedHooks reads git before it looks at the verifier",
  { skip: needsPosix },
  async () => {
    const fixture = createHookFixture();
    await rejectsMatching(
      buildCommittedHooks(
        fixture.repo,
        "0".repeat(40),
        join(fixture.root, "unused"),
        { ...hookEnv(fixture), AGX_HOOK_VERIFIER: undefined },
      ),
      /^(?!.*AGX_HOOK_VERIFIER)/s,
    );
  },
);

test(
  "buildCommittedHooks accepts a go.mod pin that is a prefix of the reported version, up to a dot",
  { skip: needsPosix },
  async () => {
    const fixture = createHookFixture();
    const sha = commitFiles(
      fixture.repo,
      { "hooks/go/go.mod": "module example.com/tinyhook\n\ngo 1.27\n" },
      "pin the minor",
    );
    const out = emptyOut(fixture);
    await buildCommittedHooks(fixture.repo, sha, out, hookEnv(fixture));
    assert.equal(readdirSync(out).length, HOOK_BINARIES.length);
  },
);

for (const [label, pin, reported] of [
  ["an older pin", "1.24", PINNED_GO],
  ["a pin that is a string prefix but not a dotted prefix", "1.2", PINNED_GO],
  ["a newer pin", "1.99.0", PINNED_GO],
] as const) {
  test(
    `buildCommittedHooks refuses a toolchain mismatch: ${label}`,
    { skip: needsPosix },
    async () => {
      const fixture = createHookFixture();
      const sha = commitFiles(
        fixture.repo,
        { "hooks/go/go.mod": `module example.com/tinyhook\n\ngo ${pin}\n` },
        `pin ${pin}`,
      );
      const out = emptyOut(fixture);
      await rejectsMatching(
        buildCommittedHooks(
          fixture.repo,
          sha,
          out,
          hookEnv(fixture, { FAKE_GO_VERSION: reported }),
        ),
        new RegExp(
          `built with go${reported.replaceAll(".", "\\.")}, but hooks/go/go\\.mod pins go${pin.replaceAll(".", "\\.")}`,
        ),
      );
      assert.deepEqual(
        readdirSync(out),
        [],
        "a toolchain refusal must not copy the binaries",
      );
    },
  );
}

test(
  "buildCommittedHooks asks go for the version from the verifier output directory, away from any go.mod",
  { skip: needsPosix },
  async () => {
    const fixture = createHookFixture();
    await buildCommittedHooks(
      fixture.repo,
      fixture.sha,
      emptyOut(fixture),
      hookEnv(fixture),
    );
    assert.equal(
      readFileSync(`${fixture.record}.gocwd`, "utf8"),
      readFileSync(`${fixture.record}.real`, "utf8"),
    );
  },
);

test(
  "buildCommittedHooks reports a go command that fails",
  { skip: needsPosix },
  async () => {
    const fixture = createHookFixture();
    await rejectsMatching(
      buildCommittedHooks(
        fixture.repo,
        fixture.sha,
        emptyOut(fixture),
        hookEnv(fixture, { FAKE_GO_FAIL: "boom" }),
      ),
      /^reading the Go version of agx-darwin-amd64: go exited with code 1: fake go: boom$/,
    );
  },
);

for (const aliased of [false, true]) {
  test(`checkBuildToolchain reads a controlled binary version and names the pin on a mismatch${aliased ? " through a directory alias" : ""}`, async () => {
    const { root, env } = versionFixture(aliased);
    const reported = `go${PINNED_GO}`;
    const binary = join(root, "controlled-hook");
    writeFileSync(binary, "controlled Go binary\n");
    const realNode = spawnSync(
      process.execPath,
      ["-e", "process.stdout.write('real node')"],
      { env, encoding: "utf8" },
    );
    assert.equal(realNode.status, 0, realNode.stderr);
    assert.equal(
      realNode.stdout,
      "real node",
      "the preload must not replace the actual Node executable",
    );
    await checkBuildToolchain(binary, reported, env);
    await checkBuildToolchain(
      binary,
      reported.slice(0, reported.lastIndexOf(".")),
      env,
    );
    await rejectsMatching(
      checkBuildToolchain(binary, "go1.99.0", env),
      /hooks\/go\/go\.mod pins go1\.99\.0/,
      new RegExp(reported.replaceAll(".", "\\.")),
    );
  });
}

test("checkBuildToolchain forwards a controlled not-Go-binary refusal", async () => {
  const { root, env } = versionFixture();
  const notGo = join(root, "plain.txt");
  writeFileSync(notGo, "not a binary\n");
  await rejectsMatching(
    checkBuildToolchain(notGo, "go1.27.3", env),
    /^reading the Go version of plain\.txt/,
    /not a Go binary/,
  );
});

for (const [label, contents, write] of [
  ["a missing file", "", false],
  ["a go.mod with no directive", "module example.com/hook\n", true],
  ["an invalid directive", "module example.com/hook\n\ngo invalid\n", true],
  [
    "a CRLF go.mod, whose lines do not end in a bare newline",
    "module example.com/hook\r\n\r\ngo 1.27.3\r\n",
    true,
  ],
  ["an indented directive", "module example.com/hook\n\n  go 1.27.3\n", true],
  [
    "a directive with a trailing comment",
    "module example.com/hook\n\ngo 1.27.3 // pinned\n",
    true,
  ],
] as const) {
  test(`pinnedHookToolchain names hooks/go/go.mod when it finds no pin: ${label}`, () => {
    const dir = scratchDir("agentics-pin-");
    if (write) writeFileSync(join(dir, "go.mod"), contents);
    assert.throws(() => pinnedHookToolchain(dir), /hooks\/go\/go\.mod/);
  });
}

for (const [label, contents, want] of [
  ["a patch version", "module m\n\ngo 1.27.3\n", "go1.27.3"],
  ["a minor version", "module m\n\ngo 1.27\n", "go1.27"],
  [
    "the first directive when there are two",
    "module m\n\ngo 1.20\ngo 1.21\n",
    "go1.20",
  ],
  ["a directive at the start of the file", "go 1.27.3\n", "go1.27.3"],
  [
    "a directive at the end of the file with no newline",
    "module m\n\ngo 1.27.3",
    "go1.27.3",
  ],
  [
    "a tab separator and trailing blanks",
    "module m\n\ngo\t1.27.3 \t\n",
    "go1.27.3",
  ],
  [
    "a toolchain line, which is not the directive",
    "module m\n\ntoolchain go1.99.0\ngo 1.27.3\n",
    "go1.27.3",
  ],
] as const) {
  test(`pinnedHookToolchain returns the go directive of ${label}`, () => {
    const dir = scratchDir("agentics-pin-");
    writeFileSync(join(dir, "go.mod"), contents);
    assert.equal(pinnedHookToolchain(dir), want);
  });
}

test("readHookManifest maps recorded paths to lowercase digests and keeps the first entry for a path", () => {
  const dir = scratchDir("agentics-manifest-");
  const path = join(dir, "SHA256SUMS");
  writeFileSync(
    path,
    `${"AB".repeat(32)}  bin/one\n${"cd".repeat(32)}\tbin/two\n${"ef".repeat(32)}   bin/one\nshort-line\n\n`,
  );
  assert.deepEqual(
    [...readHookManifest(path)],
    [
      ["bin/one", "ab".repeat(32)],
      ["bin/two", "cd".repeat(32)],
    ],
  );
});

test("readHookManifest refuses a carriage return and names the line", () => {
  const dir = scratchDir("agentics-manifest-");
  const path = join(dir, "SHA256SUMS");
  writeFileSync(path, `${sha256("a")}  bin/one\n${sha256("b")}  bin/two\r\n`);
  assert.throws(
    () => readHookManifest(path),
    /^Error: the checksum manifest line 2 carries a carriage return$/,
  );
});

test("readHookManifest refuses a manifest with no entries and a file that cannot be read", () => {
  const dir = scratchDir("agentics-manifest-");
  const empty = join(dir, "empty");
  writeFileSync(empty, "\nonlyonefield\n");
  assert.throws(
    () => readHookManifest(empty),
    /^Error: the checksum manifest has no entries$/,
  );
  assert.throws(
    () => readHookManifest(join(dir, "absent")),
    /^Error: reading the checksum manifest: /,
  );
});
