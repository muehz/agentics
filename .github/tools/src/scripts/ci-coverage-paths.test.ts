import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import { spawnSync } from "node:child_process";
import {
  fixtureGitEnv,
  git as fixtureGit,
} from "../conformance/testing/git-checkout.ts";
import {
  type CommandResult,
  type CommandRun,
  coveragePaths,
} from "./ci-coverage-paths.ts";

const MODULE = path.join(import.meta.dirname, "ci-coverage-paths.ts");

function fixture(t: test.TestContext) {
  const repo = fs.mkdtempSync(path.join(os.tmpdir(), "agentics-ts-paths-"));
  t.after(() => fs.rmSync(repo, { recursive: true, force: true }));
  const env = fixtureGitEnv();
  const git = (...args: string[]) => fixtureGit(repo, ...args).trim();
  git("init", "-q");
  const commit = (file: string) => {
    fs.mkdirSync(path.dirname(path.join(repo, file)), { recursive: true });
    fs.writeFileSync(path.join(repo, file), file + Math.random());
    git("add", "-A");
    git(
      "-c",
      "user.email=ci@example.com",
      "-c",
      "user.name=ci",
      "commit",
      "-q",
      "-m",
      file,
    );
    return git("rev-parse", "HEAD");
  };
  const outputFile = path.join(repo, ".git", "github-output");
  return { repo, env, git, commit, outputFile };
}

// Runs git in the fixture checkout.
function inRepo(repo: string, env: NodeJS.ProcessEnv): CommandRun {
  return (command, args): CommandResult => {
    const r = spawnSync(command, args, { cwd: repo, env, encoding: "utf8" });
    return { status: r.status, stdout: r.stdout ?? "", error: r.error };
  };
}

const SHA_A = "a".repeat(40);

test("each push is matched against the coverage paths", async (t) => {
  const { repo, env, git, commit, outputFile } = fixture(t);
  commit("README.md");
  for (const [name, file, before, expected] of [
    ["tools change", ".github/tools/src/a.ts", "base", "true"],
    ["root package manifest", "package-lock.json", "base", "true"],
    ["workflow change", ".github/workflows/x.yml", "base", "true"],
    ["evals change", "evals/src/a.ts", "base", "true"],
    ["hooks change", "hooks/go/a.go", "base", "true"],
    ["skills change", "skills/a/SKILL.md", "base", "true"],
    ["release tooling change", ".github/release/a.ts", "base", "true"],
    ["docs only", "docs/guide.md", "base", "false"],
    ["nested package manifest", "docs/package.json", "base", "false"],
    ["new branch", "docs/other.md", "0".repeat(40), "true"],
    ["empty before", "docs/more.md", "", "true"],
  ] as const) {
    await t.test(name, () => {
      const from = before === "base" ? git("rev-parse", "HEAD") : before;
      const head = commit(file);
      fs.rmSync(outputFile, { force: true });
      const status = coveragePaths(
        { BEFORE: from, GITHUB_SHA: head, GITHUB_OUTPUT: outputFile },
        inRepo(repo, env),
      );
      assert.equal(status, 0);
      assert.equal(fs.readFileSync(outputFile, "utf8"), `run=${expected}\n`);
    });
  }
});

test("a before commit missing from the checkout runs coverage and runs no diff", (t) => {
  const { repo, env, commit, outputFile } = fixture(t);
  const head = commit("docs/a.md");
  const calls: string[][] = [];
  const stderrByCall: (string | undefined)[] = [];
  const real = inRepo(repo, env);
  const status = coveragePaths(
    { BEFORE: "1".repeat(40), GITHUB_SHA: head, GITHUB_OUTPUT: outputFile },
    (command, args, options) => {
      calls.push(args);
      stderrByCall.push(options?.stderr);
      return real(command, args);
    },
  );
  // The probe for a missing commit is expected to fail, so its stderr must stay out of the log.
  assert.deepEqual(stderrByCall, ["ignore"]);
  assert.equal(status, 0);
  assert.equal(fs.readFileSync(outputFile, "utf8"), "run=true\n");
  assert.ok(calls.length > 0);
  assert.ok(!calls.some((args) => args[0] === "diff"));
});

test("an empty or all-zero before runs no git command", (t) => {
  const { outputFile } = fixture(t);
  for (const before of ["", "0".repeat(40), "0"]) {
    fs.rmSync(outputFile, { force: true });
    const status = coveragePaths(
      { BEFORE: before, GITHUB_SHA: "x", GITHUB_OUTPUT: outputFile },
      () => assert.fail("no git command may run"),
    );
    assert.equal(status, 0);
    assert.equal(fs.readFileSync(outputFile, "utf8"), "run=true\n");
  }
});

test("a failing diff exits nonzero and writes no output", (t) => {
  const { outputFile } = fixture(t);
  const messages: string[] = [];
  const run: CommandRun = (_command, args) =>
    args[0] === "cat-file"
      ? { status: 0, stdout: "" }
      : { status: 128, stdout: "docs/a.md\n.github/tools/x.ts\n" };
  const status = coveragePaths(
    { BEFORE: SHA_A, GITHUB_SHA: "b", GITHUB_OUTPUT: outputFile },
    run,
    (message) => messages.push(message),
  );
  assert.equal(status, 128);
  assert.equal(fs.existsSync(outputFile), false);
  assert.match(messages.join("\n"), /^ci-coverage-paths: /);
});

test("a git that cannot start exits nonzero and writes no output", (t) => {
  const { outputFile } = fixture(t);
  for (const failing of ["cat-file", "diff"]) {
    const messages: string[] = [];
    const run: CommandRun = (_command, args) =>
      args[0] === failing
        ? { status: null, stdout: "", error: new Error("spawn git ENOENT") }
        : { status: 0, stdout: "" };
    const status = coveragePaths(
      { BEFORE: SHA_A, GITHUB_SHA: "b", GITHUB_OUTPUT: outputFile },
      run,
      (message) => messages.push(message),
    );
    assert.notEqual(status, 0, failing);
    assert.equal(fs.existsSync(outputFile), false, failing);
    assert.match(messages.join("\n"), /^ci-coverage-paths: cannot start git/);
  }
});

test("exactly one line is appended, after existing output", (t) => {
  const { outputFile } = fixture(t);
  fs.writeFileSync(outputFile, "earlier=1\n");
  const run: CommandRun = (_command, args) =>
    args[0] === "cat-file"
      ? { status: 0, stdout: "" }
      : { status: 0, stdout: "docs/a.md\nhooks/go/x.go\n" };
  coveragePaths(
    { BEFORE: SHA_A, GITHUB_SHA: "b", GITHUB_OUTPUT: outputFile },
    run,
  );
  assert.equal(fs.readFileSync(outputFile, "utf8"), "earlier=1\nrun=true\n");
});

test("a missing GITHUB_OUTPUT exits nonzero", () => {
  const messages: string[] = [];
  const status = coveragePaths(
    { BEFORE: "" },
    () => assert.fail("no git command may run"),
    (message) => messages.push(message),
  );
  assert.notEqual(status, 0);
  assert.match(messages.join("\n"), /^ci-coverage-paths: GITHUB_OUTPUT/);
});

test("run as a script, the module reads its step environment and sets the exit status", (t) => {
  const { repo, env, commit, outputFile } = fixture(t);
  const base = commit("README.md");
  const head = commit("hooks/go/a.go");
  const ok = spawnSync(process.execPath, [MODULE], {
    cwd: repo,
    env: { ...env, BEFORE: base, GITHUB_SHA: head, GITHUB_OUTPUT: outputFile },
    encoding: "utf8",
    timeout: 20000,
  });
  assert.equal(ok.status, 0, ok.stdout + ok.stderr);
  assert.equal(fs.readFileSync(outputFile, "utf8"), "run=true\n");
  const bad = spawnSync(process.execPath, [MODULE], {
    cwd: repo,
    env: {
      ...env,
      BEFORE: base,
      GITHUB_SHA: "nope",
      GITHUB_OUTPUT: outputFile,
    },
    encoding: "utf8",
    timeout: 20000,
  });
  assert.notEqual(bad.status, 0);
});
