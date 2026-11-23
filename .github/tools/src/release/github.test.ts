import assert from "node:assert/strict";
import {
  existsSync,
  readFileSync,
  realpathSync,
  statSync,
  writeFileSync,
} from "node:fs";
import { EOL } from "node:os";
import { join } from "node:path";
import { test } from "node:test";
import {
  describeExit,
  git,
  gitBuffer,
  runCommand,
  runGit,
  succeeded,
  writeOutputs,
  type CommandResult,
} from "./github.ts";
import {
  captureIo,
  commitFiles,
  initRepo,
  scratchDir,
} from "./testing/support.ts";

function repo(): string {
  const dir = scratchDir("agentics-github-");
  initRepo(dir);
  return dir;
}

// The output is returned untrimmed.
test("git resolves standard output untrimmed", async () => {
  assert.equal(
    await git(repo(), ["rev-parse", "--is-inside-work-tree"]),
    "true\n",
  );
});

test("git rejects with the command, the exit and the trimmed stderr", async () => {
  const dir = repo();
  const zeros = "0".repeat(40);
  await assert.rejects(git(dir, ["cat-file", "-t", zeros]), (error: Error) => {
    assert.match(
      error.message,
      new RegExp(
        `^git cat-file -t ${zeros}: git exited with code 128: fatal: `,
      ),
    );
    assert.match(error.message, /could not get object info/);
    assert.equal(error.message.endsWith("\n"), false);
    return true;
  });
});

test("gitBuffer returns the exact bytes git wrote", async () => {
  const dir = repo();
  const bytes = Buffer.from([0, 1, 2, 255, 254, 13, 10, 0]);
  const sha = commitFiles(dir, { "blob.bin": bytes }, "bytes");
  assert.deepEqual(await gitBuffer(dir, ["show", `${sha}:blob.bin`]), bytes);
});

test("gitBuffer rejects like git does", async () => {
  await assert.rejects(
    gitBuffer(repo(), ["rev-parse", "--verify", "nope"]),
    /^Error: git rev-parse --verify nope: git exited with code 128: /,
  );
});

test("git writes options.input to standard input", async () => {
  assert.equal(
    await git(repo(), ["hash-object", "--stdin"], { input: "hello\n" }),
    "ce013625030ba8dba906f756967f9e9ca394464a\n",
  );
});

test("git runs with options.env instead of the process environment", async () => {
  const dir = repo();
  const identity = await git(dir, ["var", "GIT_AUTHOR_IDENT"], {
    env: { ...process.env, GIT_AUTHOR_NAME: "Env Author" },
  });
  assert.match(identity, /^Env Author </);
});

test("git says that a missing executable could not start", async () => {
  await assert.rejects(
    git(repo(), ["--version"], { env: { PATH: "" } }),
    /^Error: git --version: git could not start: spawn git ENOENT: $/,
  );
});

// A git that never started has no exit code of its own; the spawn error says what happened.
test("runGit settles a missing executable with no status and no signal", async () => {
  const result = await runGit(repo(), ["--version"], { env: { PATH: "" } });
  assert.equal(result.status, null);
  assert.equal(result.signal, null);
  assert.equal((result.spawnError as { code?: unknown }).code, "ENOENT");
});

const NODE = process.execPath;

test("runCommand runs any program and names a nonzero exit by its code", async () => {
  const result = await runCommand(NODE, ["-e", "process.exit(3)"], {
    name: "node",
  });
  assert.equal(result.status, 3);
  assert.equal(succeeded(result), false);
  assert.equal(describeExit(result), "node exited with code 3");
});

test(
  "runCommand names the signal that killed the program",
  { skip: process.platform === "win32" && "Windows has no POSIX signals" },
  async () => {
    const result = await runCommand(
      NODE,
      ["-e", "process.kill(process.pid, 'SIGKILL')"],
      { name: "node" },
    );
    assert.equal(succeeded(result), false);
    assert.equal(describeExit(result), "node was killed by SIGKILL");
  },
);

test("runCommand names a program that could not start by the name the call site gives, never its arguments", async () => {
  const result = await runCommand("agentics-no-such-program", ["--flag"], {
    env: { PATH: "" },
  });
  assert.equal(succeeded(result), false);
  assert.equal(
    describeExit(result),
    "agentics-no-such-program could not start: spawn agentics-no-such-program ENOENT",
  );
});

// Two pipes give no order between writes that land together, so the child
// pauses between them.
test("runCommand keeps stdout, stderr and their combined output in arrival order", async () => {
  const script = [
    "const fs = require('node:fs');",
    "fs.writeSync(1, 'one ');",
    "setTimeout(() => fs.writeSync(2, 'two '), 200);",
    "setTimeout(() => fs.writeSync(1, 'three'), 400);",
  ].join("");
  const result = await runCommand(NODE, ["-e", script]);
  assert.equal(succeeded(result), true);
  assert.equal(result.stdout.toString("utf8"), "one three");
  assert.equal(result.stderr.toString("utf8"), "two ");
  assert.equal(result.output.toString("utf8"), "one two three");
});

test("runCommand runs in options.cwd", async () => {
  const dir = scratchDir("agentics-github-");
  const result = await runCommand(
    NODE,
    ["-e", "process.stdout.write(process.cwd())"],
    { cwd: dir },
  );
  assert.equal(realpathSync(result.stdout.toString("utf8")), realpathSync(dir));
});

function ended(fields: Partial<CommandResult>): CommandResult {
  return {
    command: "git",
    status: null,
    signal: null,
    spawnError: undefined,
    stdout: Buffer.alloc(0),
    stderr: Buffer.alloc(0),
    output: Buffer.alloc(0),
    ...fields,
  };
}

test("describeExit renders the three failure sentences", () => {
  assert.equal(
    describeExit(ended({ status: 128 })),
    "git exited with code 128",
  );
  assert.equal(
    describeExit(ended({ signal: "SIGTERM" })),
    "git was killed by SIGTERM",
  );
  const denied = Object.assign(new Error("spawn git EACCES"), {
    code: "EACCES",
  });
  assert.equal(
    describeExit(ended({ spawnError: denied })),
    "git could not start: spawn git EACCES",
  );
});

const DELIMITER = "_GitHubActionsFileCommandDelimeter_";

// The whole file is compared, and the keys are given out of order on purpose.
test("writeOutputs appends sorted multi-line file commands to GITHUB_OUTPUT", () => {
  const path = join(scratchDir("agentics-github-"), "outputs");
  writeFileSync(path, `before=1${EOL}`);
  writeOutputs({ tag: "v1.2.3", sha: "abc" }, { GITHUB_OUTPUT: path });
  assert.equal(
    readFileSync(path, "utf8"),
    `before=1${EOL}` +
      `sha<<${DELIMITER}${EOL}abc${EOL}${DELIMITER}${EOL}` +
      `tag<<${DELIMITER}${EOL}v1.2.3${EOL}${DELIMITER}${EOL}`,
  );
});

test("writeOutputs writes an empty value and a multi-line value as given", () => {
  const path = join(scratchDir("agentics-github-"), "outputs");
  writeOutputs({ resume_tag: "", notes: "a\nb" }, { GITHUB_OUTPUT: path });
  assert.equal(
    readFileSync(path, "utf8"),
    `notes<<${DELIMITER}${EOL}a\nb${EOL}${DELIMITER}${EOL}` +
      `resume_tag<<${DELIMITER}${EOL}${EOL}${DELIMITER}${EOL}`,
  );
});

// Windows has no POSIX permission bits to compare.
test(
  "writeOutputs creates GITHUB_OUTPUT with mode 0644 even under an open umask",
  { skip: process.platform === "win32" },
  () => {
    const path = join(scratchDir("agentics-github-"), "outputs");
    const previous = process.umask(0);
    try {
      writeOutputs({ tag: "v1.2.3" }, { GITHUB_OUTPUT: path });
    } finally {
      process.umask(previous);
    }
    assert.equal(statSync(path).mode & 0o777, 0o644);
  },
);

test("writeOutputs creates no file when there is nothing to write", () => {
  const path = join(scratchDir("agentics-github-"), "outputs");
  writeOutputs({}, { GITHUB_OUTPUT: path });
  assert.equal(existsSync(path), false);
});

// An empty GITHUB_OUTPUT is unset.
test("writeOutputs writes sorted key=value lines when GITHUB_OUTPUT is unset or empty", () => {
  for (const env of [{}, { GITHUB_OUTPUT: undefined }, { GITHUB_OUTPUT: "" }]) {
    const captured = captureIo();
    writeOutputs(
      { tag: "v1.2.3", sha: "abc", bundle: "agentics.tar.gz" },
      env,
      captured.io.stdout,
    );
    assert.equal(
      captured.stdout(),
      "bundle=agentics.tar.gz\nsha=abc\ntag=v1.2.3\n",
    );
    assert.equal(captured.stderr(), "");
  }
});

test("writeOutputs throws a GITHUB_OUTPUT error for an unwritable path", () => {
  const path = join(scratchDir("agentics-github-"), "absent", "outputs");
  assert.throws(
    () => writeOutputs({ tag: "v1.2.3" }, { GITHUB_OUTPUT: path }),
    {
      message: `write GITHUB_OUTPUT: unable to write command to the environment file: ${path}: does not exist`,
    },
  );
});
