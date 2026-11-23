import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import {
  type CommandResult,
  type CommandRun,
  type Download,
  SHELLCHECK_PATHSPECS,
  main,
} from "./ci-shellcheck.ts";

const MODULE = path.join(import.meta.dirname, "ci-shellcheck.ts");
const BINARY = Buffer.from("not really shellcheck");
const ARCHIVE = Buffer.from("not really an archive");
const SHA = createHash("sha256").update(ARCHIVE).digest("hex");
const VERSION = "v0.11.0";
const URL = `https://github.com/koalaman/shellcheck/releases/download/${VERSION}/shellcheck-${VERSION}.linux.x86_64.tar.xz`;
const PLUGIN_SH = ":!:hooks/go/cmd/hook/testdata/corpus/scripts/plugin.sh";
const STAGE_SH = ":!:hooks/go/cmd/hook/testdata/corpus/scripts/verify-stage.sh";

function tempDir(t: test.TestContext): string {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "agentics-shellcheck-"));
  t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
  return dir;
}

interface Call {
  command: string;
  args: string[];
  cwd: string | undefined;
}

interface Harness {
  temp: string;
  githubPath: string;
  env: Record<string, string>;
  urls: string[];
  calls: Call[];
  errors: string[];
  download: Download;
  run: CommandRun;
}

// The fake tar writes the member the real one would extract, so the install step has a file to put in place.
function harness(
  t: test.TestContext,
  options: {
    download?: (url: string) => Promise<Uint8Array>;
    sha?: string;
    tarStatus?: number;
  } = {},
): Harness {
  const temp = tempDir(t);
  const githubPath = path.join(temp, "github-path");
  const urls: string[] = [];
  const calls: Call[] = [];
  return {
    temp,
    githubPath,
    env: {
      SHELLCHECK_VERSION: VERSION,
      SHELLCHECK_SHA256: options.sha ?? SHA,
      RUNNER_TEMP: temp,
      GITHUB_PATH: githubPath,
    },
    urls,
    calls,
    errors: [],
    download: (url) => {
      urls.push(url);
      return (options.download ?? (() => Promise.resolve(ARCHIVE)))(url);
    },
    run: (command, args, runOptions) => {
      calls.push({ command, args, cwd: runOptions.cwd });
      if (command === "tar" && options.tarStatus === undefined) {
        const member = path.join(runOptions.cwd ?? "", args[2]!);
        fs.mkdirSync(path.dirname(member), { recursive: true });
        fs.writeFileSync(member, BINARY);
      }
      return { status: options.tarStatus ?? 0, stdout: "" };
    },
  };
}

const installed = (h: Harness) => path.join(h.temp, "bin", "shellcheck");

test("install downloads the pinned release, verifies it, extracts one member and extends PATH", async (t) => {
  const h = harness(t);
  const status = await main(["install"], {
    env: h.env,
    download: h.download,
    run: h.run,
    report: (line) => h.errors.push(line),
  });
  assert.equal(status, 0, h.errors.join("\n"));
  assert.deepEqual(h.urls, [URL]);
  assert.deepEqual(
    fs.readFileSync(path.join(h.temp, "shellcheck.tar.xz")),
    ARCHIVE,
  );
  assert.deepEqual(h.calls, [
    {
      command: "tar",
      args: [
        "-xJf",
        path.join(h.temp, "shellcheck.tar.xz"),
        `shellcheck-${VERSION}/shellcheck`,
      ],
      cwd: h.temp,
    },
  ]);
  assert.deepEqual(fs.readFileSync(installed(h)), BINARY);
  assert.equal(
    fs.readFileSync(h.githubPath, "utf8"),
    `${path.join(h.temp, "bin")}\n`,
  );
});

test("install sets mode 0755", async (t) => {
  const h = harness(t);
  await main(["install"], { env: h.env, download: h.download, run: h.run });
  // POSIX only: Windows file modes carry no execute bit, so only the file's presence is checked there.
  if (process.platform === "win32") {
    assert.ok(fs.existsSync(installed(h)));
    return;
  }
  assert.equal(fs.statSync(installed(h)).mode & 0o777, 0o755);
});

test("install accepts an upper-case checksum", async (t) => {
  const h = harness(t, { sha: SHA.toUpperCase() });
  assert.equal(
    await main(["install"], { env: h.env, download: h.download, run: h.run }),
    0,
  );
});

test("a checksum mismatch extracts and installs nothing", async (t) => {
  const h = harness(t, { sha: "0".repeat(64) });
  const status = await main(["install"], {
    env: h.env,
    download: h.download,
    run: h.run,
    report: (line) => h.errors.push(line),
  });
  assert.notEqual(status, 0);
  assert.deepEqual(h.calls, []);
  assert.equal(fs.existsSync(installed(h)), false);
  assert.equal(fs.existsSync(h.githubPath), false);
  assert.match(h.errors.join("\n"), /^ci-shellcheck: .*checksum/);
});

test("a failed download extracts and installs nothing", async (t) => {
  const h = harness(t, {
    download: () => Promise.reject(new Error("HTTP 404")),
  });
  const status = await main(["install"], {
    env: h.env,
    download: h.download,
    run: h.run,
    report: (line) => h.errors.push(line),
  });
  assert.notEqual(status, 0);
  assert.deepEqual(h.calls, []);
  assert.equal(fs.existsSync(installed(h)), false);
  assert.equal(fs.existsSync(h.githubPath), false);
  assert.match(h.errors.join("\n"), /^ci-shellcheck: .*download.*HTTP 404/);
});

test("a failing tar installs nothing and returns its status", async (t) => {
  const h = harness(t, { tarStatus: 2 });
  const status = await main(["install"], {
    env: h.env,
    download: h.download,
    run: h.run,
  });
  assert.equal(status, 2);
  assert.equal(fs.existsSync(installed(h)), false);
  assert.equal(fs.existsSync(h.githubPath), false);
});

test("a tar that cannot start is reported and exits nonzero", async (t) => {
  const h = harness(t);
  const errors: string[] = [];
  const status = await main(["install"], {
    env: h.env,
    download: h.download,
    run: () => ({
      status: null,
      stdout: "",
      error: new Error("spawn tar ENOENT"),
    }),
    report: (line) => errors.push(line),
  });
  assert.notEqual(status, 0);
  assert.match(errors.join("\n"), /^ci-shellcheck: cannot start tar/);
});

test("install without its pinned inputs exits nonzero and downloads nothing", async (t) => {
  for (const missing of [
    "SHELLCHECK_VERSION",
    "SHELLCHECK_SHA256",
    "RUNNER_TEMP",
    "GITHUB_PATH",
  ]) {
    const h = harness(t);
    delete h.env[missing];
    const status = await main(["install"], {
      env: h.env,
      download: h.download,
      run: h.run,
      report: (line) => h.errors.push(line),
    });
    assert.notEqual(status, 0, missing);
    assert.deepEqual(h.urls, [], missing);
    assert.match(h.errors.join("\n"), new RegExp(`^ci-shellcheck: ${missing}`));
  }
});

test("the pathspecs exclude exactly the two frozen corpus copies", () => {
  assert.deepEqual([...SHELLCHECK_PATHSPECS], ["*.sh", PLUGIN_SH, STAGE_SH]);
  assert.ok(
    !SHELLCHECK_PATHSPECS.some((spec) => spec.includes("corpus/scripts/*")),
  );
});

function checkFakes(files: string, shellcheck: Partial<CommandResult> = {}) {
  const calls: Call[] = [];
  const run: CommandRun = (command, args, options) => {
    calls.push({ command, args, cwd: options.cwd });
    if (command === "git") return { status: 0, stdout: files };
    if (command === "shellcheck" && args[0] !== "--version") {
      return { status: 0, stdout: "", ...shellcheck };
    }
    return { status: 0, stdout: "" };
  };
  return { calls, run };
}

test("check prints the version, lists files with the exact pathspecs and runs shellcheck at style severity", async () => {
  const f = checkFakes("a.sh\nb/c.sh\n");
  const out: string[] = [];
  const status = await main(["check"], {
    env: {},
    run: f.run,
    write: (line) => out.push(line),
  });
  assert.equal(status, 0);
  assert.deepEqual(
    f.calls.map((c) => [c.command, ...c.args]),
    [
      ["shellcheck", "--version"],
      ["git", "ls-files", "*.sh", PLUGIN_SH, STAGE_SH],
      ["shellcheck", "-x", "-S", "style", "a.sh", "b/c.sh"],
    ],
  );
  assert.deepEqual(out, []);
});

test("check on findings prints the exact repro line and exits 1", async () => {
  const f = checkFakes("a.sh\n", { status: 1 });
  const out: string[] = [];
  const status = await main(["check"], {
    env: {},
    run: f.run,
    write: (line) => out.push(line),
  });
  assert.equal(status, 1);
  assert.deepEqual(out, [
    `::error::Shellcheck findings. Local repro: docker run --rm -v "$PWD:/src" -w /src koalaman/shellcheck:v0.11.0 -x -S style $(git ls-files '*.sh' '${PLUGIN_SH}' '${STAGE_SH}')`,
  ]);
  assert.ok(!out[0]!.includes("corpus/scripts/*"));
});

test("check fails when git cannot list files, and when shellcheck cannot start", async () => {
  const failing: CommandRun = (command) =>
    command === "git" ? { status: 128, stdout: "" } : { status: 0, stdout: "" };
  assert.equal(await main(["check"], { env: {}, run: failing }), 128);
  const errors: string[] = [];
  const status = await main(["check"], {
    env: {},
    run: () => ({ status: null, stdout: "", error: new Error("spawn ENOENT") }),
    report: (line) => errors.push(line),
  });
  assert.notEqual(status, 0);
  assert.match(errors.join("\n"), /^ci-shellcheck: cannot start shellcheck/);
});

test("a shellcheck that dies during the lint is reported, not taken for findings", async () => {
  for (const [dying, expected] of [
    [
      { status: null, error: new Error("spawn failed") },
      /^ci-shellcheck: cannot start shellcheck/,
    ],
    [
      { status: null, signal: "SIGKILL" as const },
      /^ci-shellcheck: shellcheck was killed by SIGKILL/,
    ],
  ] as const) {
    const f = checkFakes("a.sh\n", dying);
    const out: string[] = [];
    const errors: string[] = [];
    const status = await main(["check"], {
      env: {},
      run: f.run,
      write: (line) => out.push(line),
      report: (line) => errors.push(line),
    });
    assert.equal(status, 1);
    assert.deepEqual(out, []);
    assert.match(errors.join("\n"), expected);
  }
});

test("a missing or unknown subcommand prints the usage and exits 2", async () => {
  for (const argv of [[], ["bogus"], ["check", "extra"]]) {
    const errors: string[] = [];
    const status = await main(argv, {
      env: {},
      run: () => assert.fail("nothing may start"),
      download: () => assert.fail("nothing may download"),
      report: (line) => errors.push(line),
    });
    assert.equal(status, 2);
    assert.deepEqual(errors, ["usage: ci-shellcheck.ts install|check"]);
  }
});

test("run as a script, the module sets the exit status", () => {
  const result = spawnSync(process.execPath, [MODULE], { encoding: "utf8" });
  assert.equal(result.status, 2);
  assert.match(result.stderr, /usage: ci-shellcheck\.ts install\|check/);
});
