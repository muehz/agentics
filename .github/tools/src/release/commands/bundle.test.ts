import assert from "node:assert/strict";
import { execFileSync, spawnSync } from "node:child_process";
import {
  chmodSync,
  existsSync,
  mkdirSync,
  readdirSync,
  readFileSync,
  statSync,
  symlinkSync,
  writeFileSync,
} from "node:fs";
import { delimiter, join } from "node:path";
import { test } from "node:test";
import { fileURLToPath } from "node:url";
import { fixtureGitEnv } from "../../conformance/testing/git-checkout.ts";
import {
  HOOK_BINARIES,
  tarballName,
  verifyChecksums,
  verifyLocalAssets,
} from "../assets.ts";
import type { Env, Io, Run } from "../io.ts";
import { MANIFESTS } from "../version.ts";
import {
  binaryContent,
  createHookFixture,
  hookEnv,
  LAUNCHER,
  NOTICES,
  sha256,
  TAG,
  type HookFixture,
} from "../testing/hooks-fixture.ts";
import {
  captureIo,
  commitFiles,
  platformSkip,
  posix,
  sh,
  tempGitRepo,
} from "../testing/support.ts";
import { writeBundle } from "./bundle.ts";

const needsPosix = platformSkip(
  "a POSIX shell for the stand-in verifier",
  posix,
  "run these tests on Linux or macOS",
);

const REQUIRED =
  "release: bundle: --repo, --sha, --main-ref, --tag, and --out are required";

// The version snapshot starts Git using the process environment. Run the entry
// points in a child so its ambient Git settings are the same as its explicit inputs.
const bundleChild = `
import { guardBundleTarget, run } from ${JSON.stringify(new URL("./bundle.ts", import.meta.url).href)};
import { main } from ${JSON.stringify(new URL("../cli.ts", import.meta.url).href)};
const [entry, input] = JSON.parse(process.argv[1]);
if (entry === "guard") {
  try {
    process.stdout.write(await guardBundleTarget(input, process.env));
  } catch (error) {
    process.stderr.write(error.message);
    process.exitCode = 1;
  }
} else {
  const io = { stdout: process.stdout, stderr: process.stderr };
  process.exitCode = await (entry === "run" ? run : main)(input, io, process.env);
}
`;

function runBundleChild(
  entry: "run" | "main" | "guard",
  input: readonly string[] | { repo: string; sha: string; mainRef: string },
  env: Env,
) {
  const result = spawnSync(
    process.execPath,
    [
      "--input-type=module",
      "--eval",
      bundleChild,
      JSON.stringify([entry, input]),
    ],
    { encoding: "utf8", env },
  );
  assert.ifError(result.error);
  assert.ok(
    result.status !== null,
    `bundle fixture stopped by ${result.signal}`,
  );
  return { code: result.status, stdout: result.stdout, stderr: result.stderr };
}

function runBundleCommand(
  entry: "run" | "main",
  argv: readonly string[],
  io: Io,
  env: Env,
): number {
  const result = runBundleChild(entry, argv, env);
  io.stdout.write(result.stdout);
  io.stderr.write(result.stderr);
  return result.code;
}

const run: Run = async (argv, io, env) =>
  runBundleCommand("run", argv, io, env);
const main: Run = async (argv, io, env) =>
  runBundleCommand("main", argv, io, env);

async function guardBundleTarget(
  target: { repo: string; sha: string; mainRef: string },
  env: Env,
): Promise<string> {
  const result = runBundleChild("guard", target, env);
  if (result.code !== 0) throw new Error(result.stderr);
  return result.stdout;
}

function emptyOut(fixture: HookFixture): string {
  const out = join(fixture.root, "assets");
  mkdirSync(out);
  return out;
}

function commitWithoutCodexManifest(fixture: HookFixture): string {
  sh(fixture.repo, "git", "rm", "--", ".codex-plugin/plugin.json");
  return commitFiles(fixture.repo, {}, "remove a version manifest");
}

function bundleArgv(
  fixture: HookFixture,
  out: string,
  overrides: Readonly<Record<string, string>> = {},
): string[] {
  const flags: Record<string, string> = {
    repo: fixture.repo,
    sha: fixture.sha,
    "main-ref": "HEAD",
    tag: TAG,
    out,
    ...overrides,
  };
  return Object.entries(flags).flatMap(([name, value]) => [`--${name}`, value]);
}

function without(argv: readonly string[], name: string): string[] {
  const index = argv.indexOf(`--${name}`);
  return [...argv.slice(0, index), ...argv.slice(index + 2)];
}

function gitBytes(repo: string, ...args: string[]): Buffer {
  return execFileSync("git", ["-C", repo, ...args], {
    env: fixtureGitEnv(),
    encoding: "buffer",
    maxBuffer: 1 << 28,
  });
}

function mode(out: string, name: string): number {
  return statSync(join(out, name)).mode & 0o777;
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

test(
  "bundle produces a deterministic installable plugin archive",
  { skip: needsPosix },
  async () => {
    const fixture = createHookFixture();
    const reference = "skills/example/references/Café notes.md";
    fixture.sha = commitFiles(
      fixture.repo,
      { [reference]: "portable reference\n" },
      "portable Unicode filename",
    );
    const out = emptyOut(fixture);
    const rebuilt = join(fixture.root, "independent");
    mkdirSync(rebuilt);
    const name = `agentics-plugin-${TAG}.tar.gz`;
    const clone = join(fixture.root, "clone");
    sh(fixture.root, "git", "clone", "--no-local", fixture.repo, clone);
    sh(clone, "git", "config", "tar.umask", "0077");
    for (const [dir, repo] of [
      [out, fixture.repo],
      [rebuilt, clone],
    ] as const) {
      const { io, stderr } = captureIo();
      assert.equal(
        await run(bundleArgv(fixture, dir, { repo }), io, hookEnv(fixture)),
        0,
        stderr(),
      );
      assert.ok(
        existsSync(join(dir, name)),
        `missing installable archive ${name}`,
      );
    }
    assert.deepEqual(
      readFileSync(join(out, name)),
      readFileSync(join(rebuilt, name)),
    );
    assert.equal(
      sh(
        fixture.root,
        "tar",
        "-xzOf",
        join(rebuilt, name),
        `agentics-plugin-${TAG}/${reference}`,
      ),
      "portable reference\n",
    );
  },
);

for (const sourceOnly of [false, true]) {
  test(
    `installable archive matches the exact ${sourceOnly ? "source-only" : "legacy"} tree and generated launcher`,
    { skip: needsPosix },
    async () => {
      const fixture = createHookFixture();
      const { repo } = fixture;
      const extra: Record<string, string | Buffer> = {
        "assets/dev.svg": "development logo",
        "hooks/notes.md": "development notes",
        ".github/plugin/dev.md": "development notes",
        "docs/manual.md": "private manual",
      };
      if (sourceOnly) sh(repo, "git", "rm", "bin/agx.exe", "assets/logo.svg");
      else
        for (const name of HOOK_BINARIES)
          extra[`hooks/bin/${name}`] = binaryContent(name);
      commitFiles(repo, extra, "selected plugin version");
      // Make a scratch-file timestamp observably different from the selected commit's.
      execFileSync("git", ["commit", "--allow-empty", "-qm", "dated release"], {
        cwd: repo,
        env: { ...fixtureGitEnv(), GIT_COMMITTER_DATE: "2002-01-01T00:00:00Z" },
      });
      fixture.sha = sh(repo, "git", "rev-parse", "HEAD").trim();
      writeFileSync(join(repo, "bin/agx.exe"), "dirty launcher");
      writeFileSync(join(repo, "assets/logo.svg"), "dirty logo");
      writeFileSync(
        join(repo, ".antigravity-plugin/plugin.json"),
        JSON.stringify({ version: "1.2.3", logo: "assets/logo.svg" }),
      );
      writeFileSync(join(repo, "skills/example/SKILL.md"), "dirty skill");
      writeFileSync(
        join(repo, "skills/example/untracked.md"),
        "working tree only",
      );
      const out = emptyOut(fixture);
      const { io, stderr } = captureIo();
      assert.equal(
        await run(bundleArgv(fixture, out), io, hookEnv(fixture)),
        0,
        stderr(),
      );
      const archive = join(out, `agentics-plugin-${TAG}.tar.gz`);
      const prefix = `agentics-plugin-${TAG}/`;
      const paths = [
        ".agents/plugins/marketplace.json",
        ".antigravity-plugin/plugin.json",
        ".claude-plugin/marketplace.json",
        ".claude-plugin/plugin.json",
        ".codex-plugin/plugin.json",
        ".github/plugin/plugin.json",
        "LICENSE",
        "THIRD_PARTY_NOTICES.md",
        ...(sourceOnly ? [] : ["assets/logo.svg"]),
        "bin/agx",
        "bin/agx.exe",
        "hooks.json",
        "hooks/SHA256SUMS",
        "hooks/fetch-hook.ps1",
        "hooks/hooks.claude.json",
        "hooks/hooks.codex.json",
        "hooks/hooks.copilot.json",
        "hooks/run-hook.cmd",
        "skills/example/SKILL.md",
        "skills/example/references/example.md",
        "skills/example/scripts/tool.sh",
      ];
      const entries = sh(repo, "tar", "-tzf", archive).trimEnd().split("\n");
      const parents = new Set<string>([prefix]);
      for (const path of paths) {
        const parts = path.split("/");
        for (let depth = 1; depth < parts.length; depth++)
          parents.add(prefix + parts.slice(0, depth).join("/") + "/");
      }
      assert.deepEqual(
        entries.sort(),
        [...parents, ...paths.map((path) => prefix + path)].sort(),
      );
      const extracted = join(fixture.root, "installed");
      mkdirSync(extracted);
      sh(extracted, "tar", "-xzpf", archive);
      const executable = new Set([
        "bin/agx",
        "hooks/run-hook.cmd",
        "skills/example/scripts/tool.sh",
      ]);
      const stamp = Number(
        sh(repo, "git", "show", "-s", "--format=%ct", fixture.sha),
      );
      for (const path of paths) {
        const installed = join(extracted, prefix, path);
        assert.deepEqual(
          readFileSync(installed),
          path === "bin/agx.exe"
            ? Buffer.from(LAUNCHER)
            : gitBytes(repo, "show", `${fixture.sha}:${path}`),
          path,
        );
        assert.equal(
          mode(extracted, prefix + path),
          executable.has(path) ? 0o755 : 0o644,
          path,
        );
        assert.equal(statSync(installed).mtimeMs, stamp * 1000, path);
      }
    },
  );
}

test(
  "bundle refuses plugin names that cannot be extracted portably",
  { skip: needsPosix },
  async (t) => {
    const cases: [string, string[], RegExp][] = [
      ["Windows punctuation", ["question?.md"], /invalid plugin archive path/],
      [
        "control character",
        ["control\u0001.txt"],
        /invalid plugin archive path/,
      ],
      [
        "trailing space in a parent",
        ["manual /notes.md"],
        /invalid plugin archive path/,
      ],
      ["trailing dot", ["manual."], /invalid plugin archive path/],
      [
        "superscript device number",
        ["COM¹.txt"],
        /invalid plugin archive path/,
      ],
      [
        "case-equivalent files",
        ["notes.md", "NOTES.md"],
        /plugin archive paths collide/,
      ],
      [
        "case-equivalent parents",
        ["Manual/a.md", "manual/b.md"],
        /plugin archive paths collide/,
      ],
      [
        "equivalent Unicode names",
        ["caf\u00e9.md", "cafe\u0301.md"],
        /plugin archive paths collide/,
      ],
    ];
    for (const [label, paths, reason] of cases) {
      await t.test(label, async () => {
        const fixture = createHookFixture();
        const { repo } = fixture;
        const oid = sh(
          repo,
          "git",
          "rev-parse",
          "HEAD:skills/example/SKILL.md",
        ).trim();
        // Build the selected tree in the index so the fixture also works on case-insensitive hosts.
        sh(repo, "git", "config", "core.ignorecase", "false");
        sh(repo, "git", "config", "core.precomposeunicode", "false");
        for (const path of paths)
          sh(
            repo,
            "git",
            "update-index",
            "--add",
            "--cacheinfo",
            `100644,${oid},skills/example/${path}`,
          );
        sh(repo, "git", "commit", "--quiet", "-m", "plugin member names");
        fixture.sha = sh(repo, "git", "rev-parse", "HEAD").trim();
        const out = emptyOut(fixture);
        const { io, stderr } = captureIo();
        assert.equal(
          await run(bundleArgv(fixture, out), io, hookEnv(fixture)),
          1,
          label,
        );
        assert.match(stderr(), reason);
        assert.equal(existsSync(join(out, "SHA256SUMS")), false);
      });
    }
  },
);

test(
  "bundle refuses a missing logo declared by the selected manifest",
  { skip: needsPosix },
  async () => {
    const fixture = createHookFixture();
    const { repo } = fixture;
    const path = ".antigravity-plugin/plugin.json";
    const original = readFileSync(join(repo, path), "utf8");
    sh(repo, "git", "rm", "assets/logo.svg");
    fixture.sha = commitFiles(
      repo,
      {
        [path]: JSON.stringify({
          ...JSON.parse(original),
          logo: "assets/logo.svg",
        }),
      },
      "declare missing logo",
    );
    writeFileSync(join(repo, path), original);
    const out = emptyOut(fixture);
    const { io, stderr } = captureIo();
    assert.equal(await run(bundleArgv(fixture, out), io, hookEnv(fixture)), 1);
    assert.match(stderr(), /missing plugin file: assets\/logo\.svg/);
    assert.equal(existsSync(join(out, "SHA256SUMS")), false);
  },
);

test(
  "bundle refuses invalid committed plugin inputs before checksumming",
  { skip: needsPosix },
  async (t) => {
    const cases: [string, (repo: string) => void, RegExp][] = [
      [
        "directory in place of a required file",
        (repo) => {
          sh(repo, "git", "rm", "LICENSE");
          mkdirSync(join(repo, "LICENSE"));
          writeFileSync(join(repo, "LICENSE/nested"), "fixture");
        },
        /unexpected plugin path: LICENSE\/nested/,
      ],
      [
        "linked skill reference",
        (repo) => {
          symlinkSync(
            "example.md",
            join(repo, "skills/example/references/alias"),
          );
        },
        /expected a regular plugin file at skills\/example\/references\/alias/,
      ],
      [
        "nonportable skill filename",
        (repo) => writeFileSync(join(repo, "skills/example/NUL.md"), "fixture"),
        /invalid plugin archive path/,
      ],
      [
        "absent skills",
        (repo) => {
          sh(repo, "git", "rm", "-r", "skills");
        },
        /missing plugin skills/,
      ],
      ...["bin/agx", "hooks/run-hook.cmd"].map(
        (path): [string, (repo: string) => void, RegExp] => [
          `non-executable ${path}`,
          (repo) => chmodSync(join(repo, path), 0o644),
          /expected an executable plugin file/,
        ],
      ),
      [
        "exported resource missing from tar",
        (repo) => {
          writeFileSync(
            join(repo, ".gitattributes"),
            "assets/logo.svg export-ignore\n",
          );
        },
        /missing plugin archive entry/,
      ],
      [
        "resource transformed by Git",
        (repo) => {
          writeFileSync(join(repo, ".gitattributes"), "LICENSE export-subst\n");
          writeFileSync(join(repo, "LICENSE"), "$Format:%H$\n");
        },
        /plugin archive differs from the selected tree at LICENSE/,
      ],
    ];
    for (const [label, change, reason] of cases) {
      await t.test(label, async () => {
        const fixture = createHookFixture();
        change(fixture.repo);
        fixture.sha = commitFiles(fixture.repo, {}, label);
        const out = emptyOut(fixture);
        const { io, stderr } = captureIo();
        assert.equal(
          await run(bundleArgv(fixture, out), io, hookEnv(fixture)),
          1,
        );
        assert.match(stderr(), reason);
        assert.equal(existsSync(join(out, "SHA256SUMS")), false);
      });
    }
  },
);

test(
  "bundle writes the ten assets, each equal to what git and the committed manifest say",
  { skip: needsPosix },
  async () => {
    const fixture = createHookFixture();
    const out = emptyOut(fixture);
    const { io, stdout, stderr } = captureIo();
    const code = await run(bundleArgv(fixture, out), io, hookEnv(fixture));
    assert.equal(code, 0, stderr());
    assert.equal(stdout(), "");
    assert.equal(stderr(), "");

    assert.deepEqual(readdirSync(out), [
      "SHA256SUMS",
      "THIRD_PARTY_NOTICES.md",
      `agentics-plugin-${TAG}.tar.gz`,
      `agentics-${TAG}.tar.gz`,
      "agx-darwin-amd64",
      "agx-darwin-arm64",
      "agx-linux-amd64",
      "agx-linux-arm64",
      "agx-windows-amd64.exe",
      "agx-windows-arm64.exe",
    ]);
    verifyLocalAssets(out, TAG);
    verifyChecksums(out, TAG);

    // The publish job re-derives every asset from the same commit and compares the bytes.
    const wantArchive = gitBytes(
      fixture.repo,
      "archive",
      "--format=tar.gz",
      `--prefix=agentics-${TAG}/`,
      fixture.sha,
    );
    assert.deepEqual(readFileSync(join(out, tarballName(TAG))), wantArchive);
    assert.equal(
      readFileSync(join(out, "THIRD_PARTY_NOTICES.md"), "utf8"),
      NOTICES,
    );
    const manifest = sh(
      fixture.repo,
      "git",
      "show",
      `${fixture.sha}:hooks/SHA256SUMS`,
    );
    for (const name of HOOK_BINARIES) {
      assert.deepEqual(
        readFileSync(join(out, name)),
        binaryContent(name),
        name,
      );
      assert.ok(
        manifest.includes(
          `${sha256(readFileSync(join(out, name)))}  bin/${name}\n`,
        ),
        `${name} matches hooks/SHA256SUMS`,
      );
    }
  },
);

test(
  "bundle gives the Unix hook binaries 0755 and the Windows ones no executable bit",
  { skip: needsPosix },
  async () => {
    const fixture = createHookFixture();
    const out = emptyOut(fixture);
    assert.equal(
      await run(bundleArgv(fixture, out), captureIo().io, hookEnv(fixture)),
      0,
    );
    for (const name of [
      "agx-darwin-amd64",
      "agx-darwin-arm64",
      "agx-linux-amd64",
      "agx-linux-arm64",
    ]) {
      assert.equal(mode(out, name), 0o755, name);
    }
    for (const name of ["agx-windows-amd64.exe", "agx-windows-arm64.exe"]) {
      assert.equal(mode(out, name), 0o644, name);
    }
    assert.equal(mode(out, "SHA256SUMS"), 0o644 & ~process.umask());
    assert.equal(mode(out, "THIRD_PARTY_NOTICES.md"), 0o644 & ~process.umask());
  },
);

test(
  "bundle writes the notices from bytes, not from a decoded string",
  { skip: needsPosix },
  async () => {
    const fixture = createHookFixture();
    const notices = Buffer.from([
      0x23, 0x20, 0xff, 0xfe, 0x0d, 0x0a, 0x80, 0x00, 0x0a,
    ]);
    const sha = commitFiles(
      fixture.repo,
      { "THIRD_PARTY_NOTICES.md": notices },
      "binary notices",
    );
    const out = emptyOut(fixture);
    const { io, stderr } = captureIo();
    assert.equal(
      await run(bundleArgv(fixture, out, { sha }), io, hookEnv(fixture)),
      0,
      stderr(),
    );
    assert.deepEqual(
      readFileSync(join(out, "THIRD_PARTY_NOTICES.md")),
      notices,
    );
    verifyChecksums(out, TAG);
  },
);

test(
  "bundle reads the commit, not the working tree",
  { skip: needsPosix },
  async () => {
    const fixture = createHookFixture();
    const dirty = "dirty working tree\n";
    writeFileSync(join(fixture.repo, "THIRD_PARTY_NOTICES.md"), dirty);
    writeFileSync(join(fixture.repo, "hooks/go/cmd/hook/main.go"), "not go\n");
    writeFileSync(join(fixture.repo, "hooks/SHA256SUMS"), "junk\n");
    writeFileSync(
      join(fixture.repo, ".claude-plugin/plugin.json"),
      '{"name": "agentics", "version": "9.9.9"}\n',
    );
    writeFileSync(join(fixture.repo, "untracked.txt"), "untracked\n");
    const out = emptyOut(fixture);
    const { io, stderr } = captureIo();
    assert.equal(
      await run(bundleArgv(fixture, out), io, hookEnv(fixture)),
      0,
      stderr(),
    );

    assert.equal(
      readFileSync(join(out, "THIRD_PARTY_NOTICES.md"), "utf8"),
      NOTICES,
    );
    for (const name of HOOK_BINARIES)
      assert.deepEqual(
        readFileSync(join(out, name)),
        binaryContent(name),
        name,
      );
    const archive = join(out, tarballName(TAG));
    const listing = execFileSync("tar", ["-tzf", archive], {
      encoding: "utf8",
    }).split("\n");
    assert.ok(
      !listing.includes(`agentics-${TAG}/untracked.txt`),
      "the archive holds only committed paths",
    );
    const archivedNotices = execFileSync(
      "tar",
      ["-xzOf", archive, `agentics-${TAG}/THIRD_PARTY_NOTICES.md`],
      { encoding: "utf8" },
    );
    assert.equal(archivedNotices, NOTICES);
  },
);

test(
  "bundle refuses a verifier that is missing",
  { skip: needsPosix },
  async () => {
    const fixture = createHookFixture();
    const out = emptyOut(fixture);
    const env = {
      ...hookEnv(fixture),
      AGX_HOOK_VERIFIER: join(fixture.root, "absent-verifier"),
    };
    const { io, stderr } = captureIo();
    assert.equal(await run(bundleArgv(fixture, out), io, env), 1);
    assert.match(stderr(), /^release: bundle: trusted hook verifier: /);
    assert.equal(existsSync(join(out, "SHA256SUMS")), false);
  },
);

const badArguments: readonly (readonly [
  string,
  (fixture: HookFixture, out: string) => string[],
])[] = [
  ["no flags", () => []],
  [
    "a missing flag",
    (fixture, out) => without(bundleArgv(fixture, out), "tag"),
  ],
  ["an empty value", (fixture, out) => bundleArgv(fixture, out, { tag: "" })],
  [
    "an extra positional argument",
    (fixture, out) => [...bundleArgv(fixture, out), "extra"],
  ],
  [
    "a flag with no value",
    (fixture, out) => bundleArgv(fixture, out).slice(0, -1),
  ],
  [
    "an unknown flag",
    (fixture, out) => [...bundleArgv(fixture, out), "--unknown", "x"],
  ],
  [
    "a single-dash flag",
    (fixture, out) => [
      "-repo",
      fixture.repo,
      ...bundleArgv(fixture, out).slice(2),
    ],
  ],
];

for (const [label, argv] of badArguments) {
  test(`bundle rejects ${label} with exit 2 and writes nothing`, async () => {
    const fixture = createHookFixture();
    const out = emptyOut(fixture);
    const { io, stdout, stderr } = captureIo();
    const code = await run(argv(fixture, out), io, hookEnv(fixture));
    assert.equal(code, 2, stderr());
    assert.equal(stdout(), "");
    assert.ok(stderr().includes(`${REQUIRED}\n`), stderr());
    assert.deepEqual(readdirSync(out), []);
  });
}

test("bundle rejects a bad repository or output directory with exit 1 before it writes", async (t) => {
  const absentSha = "a".repeat(40);
  const cases: readonly (readonly [
    string,
    (fixture: HookFixture) => { repo: string; out: string },
    string,
  ])[] = [
    [
      "an output directory that does not exist",
      (f) => ({ repo: f.repo, out: join(f.root, "absent") }),
      "release: bundle: output directory does not exist: ",
    ],
    [
      "an output path that is a file",
      (f) => {
        const out = join(f.root, "assets-file");
        writeFileSync(out, "not a directory\n");
        return { repo: f.repo, out };
      },
      "release: bundle: output directory does not exist: ",
    ],
    [
      "an output directory that holds a file",
      (f) => {
        const out = emptyOut(f);
        writeFileSync(join(out, "leftover"), "stale\n");
        return { repo: f.repo, out };
      },
      "release: bundle: output directory must be empty: ",
    ],
    [
      "an output directory that holds a directory",
      (f) => {
        const out = emptyOut(f);
        mkdirSync(join(out, "leftover"));
        return { repo: f.repo, out };
      },
      "release: bundle: output directory must be empty: ",
    ],
    // The repository guard shares this failure path. Without it the run reaches the version step and
    // reports an unreadable object instead.
    [
      "a repository that does not exist",
      (f) => ({ repo: join(f.root, "absent-repo"), out: emptyOut(f) }),
      "release: bundle: repository does not exist: ",
    ],
  ];
  for (const [label, setup, want] of cases) {
    await t.test(label, async () => {
      const fixture = createHookFixture();
      const { repo, out } = setup(fixture);
      const { io, stdout, stderr } = captureIo();
      const code = await run(
        bundleArgv(fixture, out, { repo, sha: absentSha }),
        io,
        hookEnv(fixture),
      );
      assert.equal(code, 1, stderr());
      assert.equal(stdout(), "");
      assert.ok(stderr().includes(want), stderr());
    });
  }
});

test("bundle dispatcher preserves exact rejections before writing assets", async (t) => {
  const fixture = createHookFixture();
  const out = emptyOut(fixture);
  const absentSha = "0".repeat(40);
  const cases: {
    name: string;
    flags: Record<string, string>;
    diagnostic: string;
  }[] = [
    {
      name: "tag mismatch",
      flags: { tag: "v9.9.9" },
      diagnostic: `release: bundle: tag v9.9.9 does not match version-at-commit ${TAG}\n`,
    },
    {
      name: "unreadable target",
      flags: { sha: absentSha },
      diagnostic: `release bundle: target ${absentSha} is not a readable Git object: git exited with code 128\n`,
    },
    {
      name: "manifest read failure",
      flags: { sha: commitWithoutCodexManifest(fixture) },
      diagnostic:
        "release bundle: .codex-plugin/plugin.json: git show: git exited with code 128\n",
    },
  ];
  for (const { name, flags, diagnostic } of cases) {
    await t.test(name, async () => {
      const { io, stdout, stderr } = captureIo();
      const exit = await main(
        ["bundle", ...bundleArgv(fixture, out, flags)],
        io,
        hookEnv(fixture),
      );
      assert.deepEqual(
        { exit, stdout: stdout(), stderr: stderr(), entries: readdirSync(out) },
        { exit: 1, stdout: "", stderr: diagnostic, entries: [] },
      );
    });
  }
});

test(
  "bundle is reachable through the dispatcher",
  { skip: needsPosix },
  async () => {
    const fixture = createHookFixture();
    const refused = captureIo();
    assert.equal(await main(["bundle"], refused.io, hookEnv(fixture)), 2);
    assert.ok(refused.stderr().includes(REQUIRED));
    const out = emptyOut(fixture);
    const ok = captureIo();
    assert.equal(
      await main(
        ["bundle", ...bundleArgv(fixture, out)],
        ok.io,
        hookEnv(fixture),
      ),
      0,
      ok.stderr(),
    );
    verifyLocalAssets(out, TAG);
  },
);

test(
  "bundle runs its git commands in the documented order",
  { skip: needsPosix },
  async () => {
    const fixture = createHookFixture();
    const trackedRuntime = "hooks/bin/agx-linux-amd64";
    fixture.sha = commitFiles(
      fixture.repo,
      { [trackedRuntime]: binaryContent("agx-linux-amd64") },
      "track one legacy runtime",
    );
    const blobCommand = (path: string) =>
      "cat-file blob " +
      sh(fixture.repo, "git", "rev-parse", `${fixture.sha}:${path}`).trim();
    const manifestBlob = blobCommand("hooks/SHA256SUMS");
    const launcherBlob = blobCommand("bin/agx.exe");
    const runtimeBlob = blobCommand(trackedRuntime);
    const shim = join(fixture.root, "shim");
    mkdirSync(shim);
    const log = join(fixture.root, "git.log");
    const realGit = execFileSync("sh", ["-c", "command -v git"], {
      encoding: "utf8",
    }).trim();
    writeFileSync(
      join(shim, "git"),
      '#!/bin/sh\nprintf \'%s\\n\' "$*" >> "$GIT_LOG"\nexec "$REAL_GIT" "$@"\n',
    );
    chmodSync(join(shim, "git"), 0o755);
    const out = emptyOut(fixture);

    const env = hookEnv(fixture, {
      PATH: [fixture.binDir, shim, process.env.PATH ?? ""].join(delimiter),
      GIT_LOG: log,
      REAL_GIT: realGit,
    });
    const { io, stderr } = captureIo();
    const code = await run(bundleArgv(fixture, out), io, env);
    assert.equal(code, 0, stderr());

    const prefix = `-C ${fixture.repo} `;
    const kinds = readFileSync(log, "utf8")
      .split("\n")
      .filter((line) => line !== "")
      .map((line) => {
        assert.ok(
          line.startsWith(prefix),
          `every git call targets the repository: ${line}`,
        );
        const args = line.slice(prefix.length);
        const sha = fixture.sha;
        if (args === `cat-file -t ${sha}`) return "cat-file -t";
        if (args === `merge-base --is-ancestor ${sha} HEAD`)
          return "merge-base --is-ancestor";
        if (args === `show ${sha}:THIRD_PARTY_NOTICES.md`)
          return "show notices";
        if (args.startsWith(`show ${sha}:`)) return "show manifest";
        if (args === `ls-tree -rtz ${sha} -- bin hooks`)
          return "inspect hook inputs";
        if (
          args.startsWith(`ls-tree -rz ${sha} -- `) &&
          args.endsWith(" skills")
        )
          return "inspect plugin inputs";
        if (
          args.includes(`--prefix=agentics-plugin-${TAG}/bin/ --add-file=`) &&
          args.includes(`--prefix=agentics-plugin-${TAG}/ ${sha} -- `)
        )
          return "archive plugin";
        if (args === manifestBlob) return "read hook manifest";
        if (args === launcherBlob) return "read tracked launcher";
        if (args === runtimeBlob) return "read tracked runtime";
        if (
          args ===
          `archive --format=tar.gz --prefix=agentics-${TAG}/ --output=${join(out, tarballName(TAG))} ${sha}`
        )
          return "archive tar.gz";
        if (
          args.startsWith(
            "-c core.autocrlf=false -c core.eol=lf archive --format=tar --output=",
          ) &&
          args.endsWith(` ${sha} hooks/go`)
        ) {
          return "archive hook source";
        }
        return `unexpected: ${args}`;
      });
    assert.equal(
      kinds.filter((kind) => kind === "show manifest").length,
      MANIFESTS.length,
      "one git show per manifest",
    );
    assert.deepEqual(
      kinds.filter((kind, index) => kind !== kinds[index - 1]),
      [
        "cat-file -t",
        "merge-base --is-ancestor",
        "show manifest",
        "archive tar.gz",
        "inspect hook inputs",
        "archive hook source",
        "read hook manifest",
        "read tracked launcher",
        "read tracked runtime",
        "inspect plugin inputs",
        "archive plugin",
        "show notices",
      ],
    );
  },
);

test(
  "bundle refuses a stale committed manifest, names the binary, and writes no SHA256SUMS",
  { skip: needsPosix },
  async () => {
    const fixture = createHookFixture();
    const manifest = sh(
      fixture.repo,
      "git",
      "show",
      `${fixture.sha}:hooks/SHA256SUMS`,
    );
    const sha = commitFiles(
      fixture.repo,
      { "hooks/SHA256SUMS": "0".repeat(64) + manifest.slice(64) },
      "stale manifest",
    );
    const out = emptyOut(fixture);
    await rejectsMatching(
      writeBundle(fixture.repo, sha, TAG, out, hookEnv(fixture)),
      /agx-darwin-amd64/,
      /hooks\/SHA256SUMS records/,
    );
    assert.equal(existsSync(join(out, "SHA256SUMS")), false);
  },
);

test(
  "bundle refuses a toolchain that is not the go.mod pin, and writes no SHA256SUMS",
  { skip: needsPosix },
  async () => {
    const fixture = createHookFixture();
    const sha = commitFiles(
      fixture.repo,
      { "hooks/go/go.mod": "module example.com/tinyhook\n\ngo 1.24\n" },
      "pin an older toolchain",
    );
    const out = emptyOut(fixture);
    await rejectsMatching(
      writeBundle(fixture.repo, sha, TAG, out, hookEnv(fixture)),
      /hooks\/go\/go\.mod pins go1\.24/,
    );
    assert.equal(existsSync(join(out, "SHA256SUMS")), false);
  },
);

test(
  "bundle refuses a verifier that succeeds and produces no binaries",
  { skip: needsPosix },
  async () => {
    const fixture = createHookFixture();
    const noop = join(fixture.root, "noop-verifier");
    writeFileSync(noop, "#!/bin/sh\nexit 0\n");
    chmodSync(noop, 0o755);
    const out = emptyOut(fixture);
    const { io, stderr } = captureIo();
    const code = await run(bundleArgv(fixture, out), io, {
      ...hookEnv(fixture),
      AGX_HOOK_VERIFIER: noop,
    });
    assert.equal(code, 1);
    assert.match(stderr(), /^release: bundle: reading built agx-darwin-amd64/);
    assert.equal(existsSync(join(out, "SHA256SUMS")), false);
  },
);

for (const verifierMode of [
  "correct",
  "wrong",
  "missing",
  "first-good",
  "first-bad",
] as const) {
  test(
    `bundle checks the verifier's output against the committed manifest: ${verifierMode}`,
    { skip: needsPosix },
    async () => {
      const fixture = createHookFixture();
      let sha = fixture.sha;
      if (verifierMode === "first-good" || verifierMode === "first-bad") {
        const manifest = sh(
          fixture.repo,
          "git",
          "show",
          `${fixture.sha}:hooks/SHA256SUMS`,
        );
        const bad = `${"0".repeat(64)}  bin/${HOOK_BINARIES[0]}\n`;
        sha = commitFiles(
          fixture.repo,
          {
            "hooks/SHA256SUMS":
              verifierMode === "first-good" ? manifest + bad : bad + manifest,
          },
          "duplicate manifest",
        );
      }
      const out = emptyOut(fixture);
      const { io, stderr } = captureIo();
      const env = hookEnv(fixture, {
        MODE:
          verifierMode === "wrong" || verifierMode === "missing"
            ? verifierMode
            : "correct",
      });
      const code = await run(bundleArgv(fixture, out, { sha }), io, env);
      const wantSuccess =
        verifierMode === "correct" || verifierMode === "first-good";
      assert.equal(code === 0, wantSuccess, `code=${code} stderr=${stderr()}`);
      if (!wantSuccess) {
        assert.ok(
          stderr().includes(
            verifierMode === "missing" ? "reading built" : "built as sha256",
          ),
          stderr(),
        );
        assert.equal(existsSync(join(out, "SHA256SUMS")), false);
      }
    },
  );
}

test(
  "SHA256SUMS is sorted by name, excludes itself, and is the digest, two spaces, and the name",
  { skip: needsPosix },
  async () => {
    const fixture = createHookFixture();
    const out = emptyOut(fixture);
    assert.equal(
      await run(bundleArgv(fixture, out), captureIo().io, hookEnv(fixture)),
      0,
    );
    const contents = readFileSync(join(out, "SHA256SUMS"), "utf8");
    assert.ok(contents.endsWith("\n"), "SHA256SUMS ends with a newline");
    const lines = contents.slice(0, -1).split("\n");
    assert.equal(lines.length, 9);
    let previous = "";
    for (const line of lines) {
      assert.match(line, /^[0-9a-f]{64} {2}\S+$/);
      const digest = line.slice(0, 64);
      const name = line.slice(66);
      assert.notEqual(name, "SHA256SUMS");
      assert.ok(name > previous, `${name} follows ${previous} in byte order`);
      previous = name;
      assert.equal(digest, sha256(readFileSync(join(out, name))), name);
    }
    assert.equal(lines.at(0)?.endsWith("  THIRD_PARTY_NOTICES.md"), true);
    assert.equal(lines.at(-1)?.endsWith("  agx-windows-arm64.exe"), true);
  },
);

test("the parsed guard returns the version tag at the commit", async () => {
  const fixture = createHookFixture();
  const guardedTag = await guardBundleTarget(
    { repo: fixture.repo, sha: fixture.sha, mainRef: "HEAD" },
    hookEnv(fixture),
  );
  assert.equal(guardedTag, TAG);
});

test("the parsed guard rejects an annotated tag object", async () => {
  const fixture = createHookFixture();
  sh(
    fixture.repo,
    "git",
    "tag",
    "-a",
    "annotated-release",
    "-m",
    "annotated release",
  );
  const tagObject = sh(
    fixture.repo,
    "git",
    "rev-parse",
    "annotated-release^{tag}",
  ).trim();
  assert.match(tagObject, /^[0-9a-f]{40}$/);
  await assert.rejects(
    guardBundleTarget(
      { repo: fixture.repo, sha: tagObject, mainRef: "HEAD" },
      hookEnv(fixture),
    ),
    { message: `target ${tagObject} is not a commit (object type tag)` },
  );
});

test("release fixture Git stays in its checkout with hostile inherited settings", () => {
  const fixture = createHookFixture();
  const outside = tempGitRepo();
  const outsideIndex = join(outside.root, ".git", "index");
  const outsideFile = join(outside.root, "staged.txt");
  writeFileSync(outsideFile, "outside bytes\n");
  sh(outside.root, "git", "add", "staged.txt");
  const indexBefore = readFileSync(outsideIndex);
  const hostileConfig = join(fixture.root, "hostile.gitconfig");
  writeFileSync(hostileConfig, "[invalid\n");
  const hostileEnv: NodeJS.ProcessEnv = {
    ...process.env,
    GIT_DIR: join(outside.root, ".git"),
    GIT_WORK_TREE: outside.root,
    GIT_INDEX_FILE: outsideIndex,
    GIT_CONFIG_GLOBAL: hostileConfig,
    GIT_CONFIG_SYSTEM: hostileConfig,
    GIT_CONFIG_NOSYSTEM: "0",
    GIT_CONFIG_COUNT: "1",
    GIT_CONFIG_KEY_0: "user.name",
    GIT_CONFIG_VALUE_0: "Outside Fixture",
    GIT_AUTHOR_NAME: "Outside Author",
  };
  const { NODE_TEST_CONTEXT: _, ...nestedEnv } = hostileEnv;
  const positive = spawnSync(
    process.execPath,
    [
      "--test",
      "--test-concurrency=1",
      "--test-reporter=tap",
      "--test-name-pattern=^the parsed guard returns the version tag at the commit$",
      fileURLToPath(import.meta.url),
    ],
    { encoding: "utf8", env: nestedEnv },
  );

  const result = spawnSync(
    process.execPath,
    [
      "--input-type=module",
      "--eval",
      `
import { commitFiles, sh } from ${JSON.stringify(new URL("../testing/support.ts", import.meta.url).href)};
import { hookEnv } from ${JSON.stringify(new URL("../testing/hooks-fixture.ts", import.meta.url).href)};
import { guardBundleTarget } from ${JSON.stringify(new URL("./bundle.ts", import.meta.url).href)};
const fixture = JSON.parse(process.argv[1]);
const sha = commitFiles(fixture.repo, { "owned.txt": "fixture bytes\\n" }, "fixture commit");
sh(fixture.repo, "git", "tag", "-a", "proof", "-m", "proof");
const tag = sh(fixture.repo, "git", "rev-parse", "proof^{tag}").trim();
try {
  await guardBundleTarget({ repo: fixture.repo, sha: tag, mainRef: "HEAD" }, hookEnv(fixture));
  throw new Error("expected the annotated tag check to fail");
} catch (error) {
  if (error.message !== \`target \${tag} is not a commit (object type tag)\`) throw error;
}
console.log(sha);
`,
      JSON.stringify(fixture),
    ],
    { encoding: "utf8", env: hostileEnv },
  );

  assert.equal(
    sh(outside.root, "git", "rev-parse", "HEAD").trim(),
    outside.sha,
  );
  assert.deepEqual(readFileSync(outsideIndex), indexBefore);
  assert.equal(readFileSync(outsideFile, "utf8"), "outside bytes\n");
  assert.ifError(positive.error);
  assert.equal(positive.status, 0, positive.stdout + positive.stderr);
  assert.match(
    positive.stdout,
    /^ok 1 - the parsed guard returns the version tag at the commit$/m,
  );
  assert.ifError(result.error);
  assert.equal(result.status, 0, result.stderr);
  const sha = result.stdout.trim();
  assert.match(sha, /^[0-9a-f]{40}$/);
  assert.notEqual(sha, fixture.sha);
  assert.equal(
    sh(fixture.repo, "git", "log", "-1", "--format=%an").trim(),
    "Agentics Test",
  );
  assert.deepEqual(
    gitBytes(fixture.repo, "show", `${sha}:owned.txt`),
    Buffer.from("fixture bytes\n"),
  );
});

test("the parsed guard preserves target and manifest diagnostics", async (t) => {
  const cases: readonly (readonly [
    string,
    (fixture: HookFixture) => { sha: string; mainRef: string },
    RegExp,
  ])[] = [
    [
      "a malformed SHA",
      () => ({ sha: "A".repeat(40), mainRef: "HEAD" }),
      /^target A{40} is not a readable Git object: git exited with code 128$/,
    ],
    [
      "an unknown commit",
      () => ({ sha: "0".repeat(40), mainRef: "HEAD" }),
      /^target 0{40} is not a readable Git object: git exited with code 128$/,
    ],
    [
      "a target that is not an ancestor",
      (f) => {
        const second = commitFiles(
          f.repo,
          { "change.txt": "second commit\n" },
          "second",
        );
        return { sha: second, mainRef: "HEAD^" };
      },
      /^target [0-9a-f]{40} is not on HEAD\^: git exited with code 1$/,
    ],
    [
      "a manifest that cannot be read",
      (f) => ({ sha: commitWithoutCodexManifest(f), mainRef: "HEAD" }),
      /^\.codex-plugin\/plugin\.json: git show: git exited with code 128$/,
    ],
    [
      "mismatched manifest fields",
      (f) => {
        const body = readFileSync(
          join(f.repo, ".codex-plugin/plugin.json"),
          "utf8",
        ).replace('"version": "1.2.3"', '"version": "9.9.9"');
        return {
          sha: commitFiles(
            f.repo,
            { ".codex-plugin/plugin.json": body },
            "mismatch",
          ),
          mainRef: "HEAD",
        };
      },
      /^.*version mismatch/,
    ],
  ];
  for (const [label, setup, want] of cases) {
    await t.test(label, async () => {
      const fixture = createHookFixture();
      const { sha, mainRef } = setup(fixture);
      await assert.rejects(
        guardBundleTarget(
          { repo: fixture.repo, sha, mainRef },
          hookEnv(fixture),
        ),
        { message: want },
      );
    });
  }
});
