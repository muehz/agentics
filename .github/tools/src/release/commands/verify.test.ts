import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { spawnSync } from "node:child_process";
import { mkdirSync, symlinkSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { test } from "node:test";
import { fileURLToPath } from "node:url";
import {
  ASSET_FIXTURES,
  ASSET_TAG,
  removeAsset,
  validChecksumLines,
  writeAsset,
  writeAssetDir,
} from "../testing/assets.ts";
import {
  captureIo,
  platformSkip,
  posix,
  scratchDir,
} from "../testing/support.ts";
import { run } from "./verify.ts";

const CLI = fileURLToPath(new URL("../cli.ts", import.meta.url));
const REQUIRED = "release: verify: --assets and --tag are required\n";

async function verify(
  ...argv: string[]
): Promise<{ code: number; stdout: string; stderr: string }> {
  const captured = captureIo();
  const code = await run(argv, captured.io, {});
  return { code, stdout: captured.stdout(), stderr: captured.stderr() };
}

test("verify requires --assets and --tag", async () => {
  const dir = writeAssetDir(validChecksumLines());
  const cases: [string, string[], string][] = [
    ["no flags", [], REQUIRED],
    ["missing tag", ["--assets", dir], REQUIRED],
    ["missing assets", ["--tag", ASSET_TAG], REQUIRED],
    ["empty assets", ["--assets", "", "--tag", ASSET_TAG], REQUIRED],
    ["empty tag", ["--assets", dir, "--tag="], REQUIRED],
    [
      "positional argument",
      ["--assets", dir, "--tag", ASSET_TAG, "extra"],
      `release: verify: unexpected argument extra\n${REQUIRED}`,
    ],
  ];
  for (const [name, argv, stderr] of cases) {
    assert.deepEqual(
      await verify(...argv),
      { code: 2, stdout: "", stderr },
      name,
    );
  }
});

test("verify rejects a malformed tag with exit 2 before it reads the directory", async () => {
  const dir = writeAssetDir(validChecksumLines());
  for (const tag of [
    "1.2.3",
    "v1.2",
    "v1.2.3-rc.1",
    "v1.2.3+build",
    "v1.2.3\n",
    "V1.2.3",
  ]) {
    assert.deepEqual(await verify("--assets", dir, "--tag", tag), {
      code: 2,
      stdout: "",
      stderr: `release: verify: --tag must be vMAJOR.MINOR.PATCH, not ${tag}\n`,
    });
  }
  const missing = join(scratchDir("agentics-verify-cmd-"), "absent");
  assert.equal((await verify("--assets", missing, "--tag", "1.2.3")).code, 2);
});

test("verify accepts a correct bundle and writes nothing", async () => {
  const dir = writeAssetDir(validChecksumLines());
  assert.deepEqual(await verify("--assets", dir, "--tag", ASSET_TAG), {
    code: 0,
    stdout: "",
    stderr: "",
  });
  assert.deepEqual(await verify(`--assets=${dir}`, `--tag=${ASSET_TAG}`), {
    code: 0,
    stdout: "",
    stderr: "",
  });
});

test("verify checks the exact plugin archive name and digest", async () => {
  const name = `agentics-plugin-${ASSET_TAG}.tar.gz`;
  const lines = validChecksumLines();
  const index = lines.findIndex((line) => line.endsWith(`  ${name}`));
  assert.notEqual(index, -1);
  const dir = writeAssetDir(lines);
  const check = () => verify("--assets", dir, "--tag", ASSET_TAG);
  removeAsset(dir, name);
  assert.match((await check()).stderr, /exactly the ten expected names/);
  writeAsset(dir, name, "different archive bytes");
  assert.equal(
    (await check()).stderr,
    `release: verify: SHA256SUMS digest mismatch for ${name}\n`,
  );
  // A self-consistent substitution passes local verification. Only an independent build
  // supplies the trusted digest for a provenance check.
  const replacement = createHash("sha256")
    .update("different archive bytes")
    .digest("hex");
  const substituted = lines.with(index, `${replacement}  ${name}`);
  writeAsset(dir, "SHA256SUMS", substituted.join("\n") + "\n");
  assert.equal((await check()).code, 0);
  writeAsset(
    dir,
    "SHA256SUMS",
    substituted.with(index + 1, substituted[index]!).join("\n") + "\n",
  );
  assert.match(
    (await check()).stderr,
    /cover every non-checksum asset exactly once/,
  );
});

test("verify catches a stray file and a corrupted SHA256SUMS", async () => {
  const stray = writeAssetDir(validChecksumLines());
  writeAsset(stray, "stray.txt", "stray\n");
  assert.deepEqual(await verify("--assets", stray, "--tag", ASSET_TAG), {
    code: 1,
    stdout: "",
    stderr: `release: verify: assets must contain exactly the ten expected names for ${ASSET_TAG}\n`,
  });

  const lines = validChecksumLines();
  lines[0] = "0".repeat(64) + (lines[0] ?? "").slice(64);
  const corrupt = writeAssetDir(lines);
  assert.deepEqual(await verify("--assets", corrupt, "--tag", ASSET_TAG), {
    code: 1,
    stdout: "",
    stderr:
      "release: verify: SHA256SUMS digest mismatch for THIRD_PARTY_NOTICES.md\n",
  });
});

test("verify requires SHA256SUMS after all nine derivable assets match", async () => {
  const dir = writeAssetDir(validChecksumLines());
  removeAsset(dir, "SHA256SUMS");
  assert.deepEqual(await verify("--assets", dir, "--tag", ASSET_TAG), {
    code: 1,
    stdout: "",
    stderr: `release: verify: assets must contain exactly the ten expected names for ${ASSET_TAG}\n`,
  });
});

test("verify checks the roster before the checksums", async () => {
  const lines = validChecksumLines();
  lines[0] = "0".repeat(64) + (lines[0] ?? "").slice(64);
  const dir = writeAssetDir(lines);
  writeAsset(dir, "stray.txt", "stray\n");
  const result = await verify("--assets", dir, "--tag", ASSET_TAG);
  assert.equal(result.code, 1);
  assert.match(
    result.stderr,
    /assets must contain exactly the ten expected names/,
  );
});

test("verify reports a missing directory and a directory entry", async () => {
  const missing = join(scratchDir("agentics-verify-cmd-"), "absent");
  assert.deepEqual(await verify("--assets", missing, "--tag", ASSET_TAG), {
    code: 1,
    stdout: "",
    stderr: `release: verify: ${missing}: does not exist\n`,
  });
  const dir = writeAssetDir(validChecksumLines());
  mkdirSync(join(dir, "nested"));
  assert.deepEqual(await verify("--assets", dir, "--tag", ASSET_TAG), {
    code: 1,
    stdout: "",
    stderr: "release: verify: asset entry is not a regular file: nested\n",
  });
});

test("verify rejects a bundle built for another tag", async () => {
  const dir = writeAssetDir(validChecksumLines());
  assert.deepEqual(await verify("--assets", dir, "--tag", "v9.9.9"), {
    code: 1,
    stdout: "",
    stderr:
      "release: verify: assets must contain exactly the ten expected names for v9.9.9\n",
  });
});

test(
  "verify follows a symbolic link to a regular file and counts a stray link",
  {
    skip: platformSkip("symlinks", posix, "creating links needs a POSIX host"),
  },
  async () => {
    const dir = writeAssetDir(validChecksumLines());
    const target = join(scratchDir("agentics-verify-cmd-"), "target");
    writeFileSync(target, ASSET_FIXTURES["THIRD_PARTY_NOTICES.md"] ?? "");
    removeAsset(dir, "THIRD_PARTY_NOTICES.md");
    symlinkSync(target, join(dir, "THIRD_PARTY_NOTICES.md"));
    assert.equal((await verify("--assets", dir, "--tag", ASSET_TAG)).code, 0);
    symlinkSync(target, join(dir, ".dotlink"));
    assert.deepEqual(await verify("--assets", dir, "--tag", ASSET_TAG), {
      code: 1,
      stdout: "",
      stderr: `release: verify: assets must contain exactly the ten expected names for ${ASSET_TAG}\n`,
    });
  },
);

// The process seam: the workflow runs `node cli.ts verify --assets ... --tag ...`.
test("the entry point runs verify and returns its exit code", () => {
  const dir = writeAssetDir(validChecksumLines());
  const ok = spawnSync(
    process.execPath,
    [CLI, "verify", "--assets", dir, "--tag", ASSET_TAG],
    { encoding: "utf8" },
  );
  assert.deepEqual(
    { status: ok.status, stdout: ok.stdout, stderr: ok.stderr },
    { status: 0, stdout: "", stderr: "" },
  );
  writeAsset(dir, "stray.txt", "x\n");
  const bad = spawnSync(
    process.execPath,
    [CLI, "verify", "--assets", dir, "--tag", ASSET_TAG],
    { encoding: "utf8" },
  );
  assert.equal(bad.status, 1);
  assert.match(bad.stderr, /^release: verify: assets must contain exactly/);
  const usage = spawnSync(process.execPath, [CLI, "verify"], {
    encoding: "utf8",
  });
  assert.equal(usage.status, 2);
  assert.equal(usage.stderr, REQUIRED);
});
