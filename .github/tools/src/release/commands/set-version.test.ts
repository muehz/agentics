import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { test } from "node:test";
import { fileURLToPath } from "node:url";
import { USAGE } from "../cli.ts";
import { MANIFESTS, readSnapshot } from "../version.ts";
import { captureIo, scratchDir, writeManifests } from "../testing/support.ts";
import { run } from "./set-version.ts";

const CLI = fileURLToPath(new URL("../cli.ts", import.meta.url));
const REQUIRED = "release: set-version: --repo and --version are required\n";

function fixture(version = "1.2.3"): string {
  const root = scratchDir("agentics-set-version-");
  writeManifests(root, version);
  return root;
}

function readAll(root: string): string[] {
  return MANIFESTS.map(({ path }) => readFileSync(join(root, path), "latin1"));
}

async function setVersionCommand(
  ...argv: string[]
): Promise<{ code: number; stdout: string; stderr: string }> {
  const captured = captureIo();
  const code = await run(argv, captured.io, {});
  return { code, stdout: captured.stdout(), stderr: captured.stderr() };
}

test("set-version writes the five fields and reports the change", async () => {
  const root = fixture();
  const result = await setVersionCommand("--repo", root, "--version", "1.3.0");
  assert.deepEqual(result, {
    code: 0,
    stdout: "Agentics version 1.2.3 -> 1.3.0 (5 manifests written)\n",
    stderr: "",
  });
  assert.equal(readSnapshot(root).raw, "1.3.0");
});

test("each set-version refusal exits with its code and message and writes nothing", async () => {
  const root = fixture();
  const before = readAll(root);
  const cases: {
    name: string;
    argv: string[];
    code: number;
    stderr: string;
  }[] = [
    {
      name: "missing version",
      argv: ["--repo", root],
      code: 2,
      stderr: REQUIRED,
    },
    {
      name: "missing repo",
      argv: ["--version", "1.3.0"],
      code: 2,
      stderr: REQUIRED,
    },
    {
      name: "malformed",
      argv: ["--repo", root, "--version", "1.3"],
      code: 1,
      stderr:
        'release: set-version: refusing to set version: "1.3": invalid agentics version\n',
    },
    {
      name: "not greater",
      argv: ["--repo", root, "--version", "1.2.3"],
      code: 1,
      stderr:
        "release: set-version: refusing to set version: 1.2.3 is not greater than 1.2.3\n",
    },
    {
      name: "lower",
      argv: ["--repo", root, "--version", "0.9.9"],
      code: 1,
      stderr:
        "release: set-version: refusing to set version: 0.9.9 is not greater than 1.2.3\n",
    },
  ];
  for (const { name, argv, code, stderr } of cases) {
    assert.deepEqual(
      await setVersionCommand(...argv),
      { code, stdout: "", stderr },
      name,
    );
  }
  assert.deepEqual(readAll(root), before);
});

test("set-version treats an empty value as missing", async () => {
  const root = fixture();
  for (const argv of [
    ["--repo", "", "--version", "1.3.0"],
    ["--repo", root, "--version", ""],
    ["--repo=", "--version=1.3.0"],
    ["--repo", root, "--version=1.3.0", "--version="],
  ]) {
    const result = await setVersionCommand(...argv);
    assert.deepEqual(
      result,
      { code: 2, stdout: "", stderr: REQUIRED },
      argv.join(" "),
    );
  }
});

// --name=value is accepted and the last occurrence wins. A single-dash flag,
// -h, a positional argument and an unknown flag are a usage error with a
// one-line diagnostic ahead of the required-flags line.
test("set-version accepts --name=value and takes the last occurrence", async () => {
  const root = fixture();
  const result = await setVersionCommand(
    `--repo=${root}`,
    "--version",
    "1.3.0",
    "--version=1.4.0",
  );
  assert.equal(result.code, 0);
  assert.equal(
    result.stdout,
    "Agentics version 1.2.3 -> 1.4.0 (5 manifests written)\n",
  );
});

test("set-version rejects positionals, single-dash flags, -h and unknown flags", async () => {
  const root = fixture();
  const before = readAll(root);
  const good = ["--repo", root, "--version", "1.3.0"];
  const cases: [string[], string][] = [
    [[...good, "extra"], "release: set-version: unexpected argument extra\n"],
    [
      ["-h"],
      "release: set-version: unsupported flag -h (flags take the form --name value)\n",
    ],
    [
      ["-repo", root, "--version", "1.3.0"],
      `release: set-version: unsupported flag -repo (flags take the form --name value)\n`,
    ],
    [[...good, "--force", "1"], "release: set-version: unknown flag --force\n"],
    [
      ["--repo", root, "--version"],
      "release: set-version: flag --version needs a value\n",
    ],
  ];
  for (const [argv, diagnostic] of cases) {
    const result = await setVersionCommand(...argv);
    assert.deepEqual(
      result,
      { code: 2, stdout: "", stderr: diagnostic + REQUIRED },
      argv.join(" "),
    );
  }
  assert.deepEqual(readAll(root), before);
});

// A bare -- ends the flags.
test("set-version treats a trailing -- as the end of the flags", async () => {
  const root = fixture();
  const result = await setVersionCommand(
    "--repo",
    root,
    "--version",
    "1.3.0",
    "--",
  );
  assert.equal(result.code, 0);
  const after = await setVersionCommand(
    "--repo",
    root,
    "--",
    "--version",
    "1.4.0",
  );
  assert.deepEqual(after, {
    code: 2,
    stdout: "",
    stderr: `release: set-version: unexpected argument --version\n${REQUIRED}`,
  });
});

// A byte-order mark is not part of JSON and the reader does not strip one, so
// set-version exits 1 and leaves every file alone.
test("set-version refuses a manifest with a byte-order mark", async () => {
  const root = fixture();
  const path = join(root, ".codex-plugin/plugin.json");
  writeFileSync(
    path,
    Buffer.concat([Buffer.from([0xef, 0xbb, 0xbf]), readFileSync(path)]),
  );
  const before = readAll(root);
  const result = await setVersionCommand("--repo", root, "--version", "1.3.0");
  assert.deepEqual(result, {
    code: 1,
    stdout: "",
    stderr:
      "release: set-version: refusing to set version: .codex-plugin/plugin.json: is not valid JSON\n",
  });
  assert.deepEqual(readAll(root), before);
});

test("set-version keeps CRLF endings and a four-space indent", async () => {
  const root = fixture();
  for (const { path } of MANIFESTS) {
    const full = join(root, path);
    writeFileSync(
      full,
      readFileSync(full, "utf8")
        .replace(/\n {2}/g, "\r\n    ")
        .replace(/\n/g, "\r\n"),
    );
  }
  const before = readAll(root);
  const result = await setVersionCommand("--repo", root, "--version", "1.3.0");
  assert.equal(result.code, 0);
  readAll(root).forEach((after, index) => {
    assert.equal(after, (before[index] ?? "").replace('"1.2.3"', '"1.3.0"'));
    assert.ok(after.includes("\r\n"));
  });
});

// The process seam: the plugin runs `node cli.ts set-version --repo ... --version ...`.
test("the entry point runs set-version and returns its exit code", () => {
  const root = fixture();
  const ok = spawnSync(
    process.execPath,
    [CLI, "set-version", "--repo", root, "--version", "1.3.0"],
    { encoding: "utf8" },
  );
  assert.equal(ok.status, 0);
  assert.equal(
    ok.stdout,
    "Agentics version 1.2.3 -> 1.3.0 (5 manifests written)\n",
  );
  assert.equal(ok.stderr, "");
  assert.equal(readSnapshot(root).raw, "1.3.0");

  const usage = spawnSync(process.execPath, [CLI, "set-version"], {
    encoding: "utf8",
  });
  assert.equal(usage.status, 2);
  assert.equal(usage.stderr, REQUIRED);

  const refused = spawnSync(
    process.execPath,
    [CLI, "set-version", "--repo", root, "--version", "1.3.0"],
    { encoding: "utf8" },
  );
  assert.equal(refused.status, 1);
  assert.equal(
    refused.stderr,
    "release: set-version: refusing to set version: 1.3.0 is not greater than 1.3.0\n",
  );
  assert.ok(!refused.stderr.includes(USAGE));
});
