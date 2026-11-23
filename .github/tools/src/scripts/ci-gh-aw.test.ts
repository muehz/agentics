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
  LOCK_PATHS,
  LOCK_WORKFLOWS,
  main,
} from "./ci-gh-aw.ts";

const MODULE = path.join(import.meta.dirname, "ci-gh-aw.ts");
const BINARY = Buffer.from("not really gh-aw");
const SHA = createHash("sha256").update(BINARY).digest("hex");
const URL =
  "https://github.com/github/gh-aw/releases/download/v1.0.1/linux-amd64";

function tempDir(t: test.TestContext): string {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "agentics-gh-aw-"));
  t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
  return dir;
}

interface Harness {
  temp: string;
  githubPath: string;
  env: Record<string, string>;
  urls: string[];
  errors: string[];
  out: string[];
  download: Download;
}

function harness(
  t: test.TestContext,
  download?: (url: string) => Promise<Uint8Array>,
  sha = SHA,
): Harness {
  const temp = tempDir(t);
  const githubPath = path.join(temp, "github-path");
  const urls: string[] = [];
  return {
    temp,
    githubPath,
    env: {
      GH_AW_VERSION: "v1.0.1",
      GH_AW_SHA256: sha,
      RUNNER_TEMP: temp,
      GITHUB_PATH: githubPath,
    },
    urls,
    errors: [],
    out: [],
    download: (url) => {
      urls.push(url);
      return (download ?? (() => Promise.resolve(BINARY)))(url);
    },
  };
}

const installed = (h: Harness) => path.join(h.temp, "bin", "gh-aw");

test("install downloads the pinned release, verifies it, installs it and extends PATH", async (t) => {
  const h = harness(t);
  const status = await main(["install"], {
    env: h.env,
    download: h.download,
    report: (line) => h.errors.push(line),
  });
  assert.equal(status, 0, h.errors.join("\n"));
  assert.deepEqual(h.urls, [URL]);
  assert.deepEqual(fs.readFileSync(installed(h)), BINARY);
  assert.equal(
    fs.readFileSync(h.githubPath, "utf8"),
    `${path.join(h.temp, "bin")}\n`,
  );
});

test("install appends to an existing GITHUB_PATH", async (t) => {
  const h = harness(t);
  fs.writeFileSync(h.githubPath, "/earlier\n");
  await main(["install"], { env: h.env, download: h.download });
  assert.equal(
    fs.readFileSync(h.githubPath, "utf8"),
    `/earlier\n${path.join(h.temp, "bin")}\n`,
  );
});

test("install accepts an upper-case checksum", async (t) => {
  const h = harness(t, undefined, SHA.toUpperCase());
  assert.equal(
    await main(["install"], { env: h.env, download: h.download }),
    0,
  );
});

test("install sets mode 0755", async (t) => {
  const h = harness(t);
  await main(["install"], { env: h.env, download: h.download });
  // POSIX only: Windows file modes carry no execute bit, so only the file's presence is checked there.
  if (process.platform === "win32") {
    assert.ok(fs.existsSync(installed(h)));
    return;
  }
  assert.equal(fs.statSync(installed(h)).mode & 0o777, 0o755);
});

test("a checksum mismatch installs nothing and leaves GITHUB_PATH untouched", async (t) => {
  const h = harness(t, undefined, "0".repeat(64));
  const status = await main(["install"], {
    env: h.env,
    download: h.download,
    report: (line) => h.errors.push(line),
  });
  assert.notEqual(status, 0);
  assert.equal(fs.existsSync(installed(h)), false);
  assert.equal(fs.existsSync(h.githubPath), false);
  assert.match(h.errors.join("\n"), /^ci-gh-aw: .*checksum/);
});

test("a failed download installs nothing and leaves GITHUB_PATH untouched", async (t) => {
  const h = harness(t, () => Promise.reject(new Error("HTTP 404")));
  const status = await main(["install"], {
    env: h.env,
    download: h.download,
    report: (line) => h.errors.push(line),
  });
  assert.notEqual(status, 0);
  assert.equal(fs.existsSync(installed(h)), false);
  assert.equal(fs.existsSync(h.githubPath), false);
  assert.match(h.errors.join("\n"), /^ci-gh-aw: .*download.*HTTP 404/);
});

test("install without its pinned inputs exits nonzero and downloads nothing", async (t) => {
  for (const missing of [
    "GH_AW_VERSION",
    "GH_AW_SHA256",
    "RUNNER_TEMP",
    "GITHUB_PATH",
  ]) {
    const h = harness(t);
    delete h.env[missing];
    const status = await main(["install"], {
      env: h.env,
      download: h.download,
      report: (line) => h.errors.push(line),
    });
    assert.notEqual(status, 0, missing);
    assert.deepEqual(h.urls, [], missing);
    assert.match(h.errors.join("\n"), new RegExp(`^ci-gh-aw: ${missing}`));
  }
});

interface Call {
  command: string;
  args: string[];
}

// Stands in for gh-aw and git: `porcelain` is what `git status --porcelain` prints, and `failing` names a command
// (by its joined arguments) that fails.
function fakes(porcelain: string, failing?: string) {
  const calls: Call[] = [];
  const run: CommandRun = (command, args, options) => {
    calls.push({ command, args });
    if (failing !== undefined && `${command} ${args.join(" ")}` === failing) {
      return { status: 4, stdout: "" };
    }
    return {
      status: 0,
      stdout: options.capture && args.includes("--porcelain") ? porcelain : "",
    } satisfies CommandResult;
  };
  return { calls, run };
}

const paths = [".github/workflows", ".github/aw", ".gitattributes"];

test("the exported lock list and paths are the two workflows and three paths", () => {
  assert.deepEqual([...LOCK_WORKFLOWS], ["release", "backlog-grooming"]);
  assert.deepEqual([...LOCK_PATHS], paths);
});

test("check compiles release then backlog-grooming then looks at the porcelain status", () => {
  const f = fakes("");
  const out: string[] = [];
  const status = main(["check"], {
    env: { GH_AW_VERSION: "v1.0.1" },
    run: f.run,
    write: (line) => out.push(line),
  });
  return Promise.resolve(status).then((code) => {
    assert.equal(code, 0);
    assert.deepEqual(
      f.calls.map((c) => `${c.command} ${c.args.join(" ")}`),
      [
        "gh-aw compile release",
        "gh-aw compile backlog-grooming",
        `git status --porcelain -- ${paths.join(" ")}`,
      ],
    );
    assert.deepEqual(out, []);
  });
});

test("check stops at a failing compile and returns its status", async () => {
  for (const [failing, calls] of [
    ["gh-aw compile release", 1],
    ["gh-aw compile backlog-grooming", 2],
  ] as const) {
    const f = fakes("", failing);
    const status = await main(["check"], { env: {}, run: f.run });
    assert.equal(status, 4, failing);
    assert.equal(f.calls.length, calls, failing);
  }
});

test("check on a dirty tree prints the status, the diff and the exact error and exits 1", async () => {
  const f = fakes(" M .github/workflows/release.lock.yml\n");
  const out: string[] = [];
  const status = await main(["check"], {
    env: { GH_AW_VERSION: "v1.0.1" },
    run: f.run,
    write: (line) => out.push(line),
  });
  assert.equal(status, 1);
  assert.deepEqual(
    f.calls.slice(3).map((c) => `${c.command} ${c.args.join(" ")}`),
    [
      `git status --short -- ${paths.join(" ")}`,
      `git --no-pager diff -- ${paths.join(" ")}`,
    ],
  );
  assert.deepEqual(out, [
    "::error::A gh-aw lock is stale. Run 'gh aw compile release' and 'gh aw compile backlog-grooming' with gh-aw v1.0.1 and commit the result alongside the source change.",
  ]);
});

test("check fails when git status cannot run", async () => {
  const f = fakes("", `git status --porcelain -- ${paths.join(" ")}`);
  const status = await main(["check"], { env: {}, run: f.run });
  assert.equal(status, 4);
});

test("a command that cannot start is reported and exits nonzero", async () => {
  const errors: string[] = [];
  const status = await main(["check"], {
    env: {},
    run: () => ({ status: null, stdout: "", error: new Error("spawn ENOENT") }),
    report: (line) => errors.push(line),
  });
  assert.notEqual(status, 0);
  assert.match(errors.join("\n"), /^ci-gh-aw: cannot start gh-aw/);
});

test("a missing or unknown subcommand prints the usage and exits 2", async () => {
  for (const argv of [[], ["bogus"], ["install", "extra"]]) {
    const errors: string[] = [];
    const status = await main(argv, {
      env: {},
      run: () => assert.fail("nothing may start"),
      download: () => assert.fail("nothing may download"),
      report: (line) => errors.push(line),
    });
    assert.equal(status, 2);
    assert.deepEqual(errors, ["usage: ci-gh-aw.ts install|check"]);
  }
});

test("run as a script, the module sets the exit status", () => {
  const result = spawnSync(process.execPath, [MODULE], { encoding: "utf8" });
  assert.equal(result.status, 2);
  assert.match(result.stderr, /usage: ci-gh-aw\.ts install\|check/);
});
