import assert from "node:assert/strict";
import { mkdirSync, symlinkSync, writeFileSync } from "node:fs";
import { join, resolve } from "node:path";
import { test } from "node:test";
import {
  checksumAssetNames,
  CHECKSUMS_NAME,
  expectedAssetNames,
  fileDigest,
  HOOK_BINARIES,
  NOTICES_NAME,
  parseChecksumLine,
  tarballName,
  verifyChecksums,
  verifyLocalAssets,
} from "./assets.ts";
import { readHookManifest } from "./hooks-build.ts";
import {
  ASSET_FIXTURES,
  ASSET_TAG,
  assetDigest,
  removeAsset,
  validChecksumLines,
  writeAsset,
  writeAssetDir,
} from "./testing/assets.ts";
import { platformSkip, posix, scratchDir } from "./testing/support.ts";

const TARBALL = `agentics-${ASSET_TAG}.tar.gz`;
const ROSTER = `assets must contain exactly the ten expected names for ${ASSET_TAG}`;
const COUNT = "SHA256SUMS must contain exactly nine sorted asset lines";

function messageOf(body: () => unknown): string {
  try {
    body();
  } catch (error) {
    return (error as Error).message;
  }
  assert.fail("expected an error");
}

test("expectedAssetNames lists the ten names in byte order", () => {
  assert.deepEqual(expectedAssetNames(ASSET_TAG), [
    "SHA256SUMS",
    "THIRD_PARTY_NOTICES.md",
    `agentics-plugin-${ASSET_TAG}.tar.gz`,
    TARBALL,
    "agx-darwin-amd64",
    "agx-darwin-arm64",
    "agx-linux-amd64",
    "agx-linux-arm64",
    "agx-windows-amd64.exe",
    "agx-windows-arm64.exe",
  ]);
});

test("checksumAssetNames omits the checksum file", () => {
  assert.deepEqual(checksumAssetNames(ASSET_TAG), [
    "THIRD_PARTY_NOTICES.md",
    `agentics-plugin-${ASSET_TAG}.tar.gz`,
    TARBALL,
    "agx-darwin-amd64",
    "agx-darwin-arm64",
    "agx-linux-amd64",
    "agx-linux-arm64",
    "agx-windows-amd64.exe",
    "agx-windows-arm64.exe",
  ]);
  assert.equal(tarballName("v9.9.9"), "agentics-v9.9.9.tar.gz");
  assert.equal(NOTICES_NAME, "THIRD_PARTY_NOTICES.md");
  assert.equal(CHECKSUMS_NAME, "SHA256SUMS");
  assert.equal(HOOK_BINARIES.length, 6);
});

// The committed hooks/SHA256SUMS is what a release builds against, so the roster must name exactly
// the binaries it lists.
test("HOOK_BINARIES names exactly the binaries hooks/SHA256SUMS lists", () => {
  const manifest = readHookManifest(
    resolve(import.meta.dirname, "../../../../hooks/SHA256SUMS"),
  );
  assert.deepEqual(
    [...manifest.keys()].sort(),
    HOOK_BINARIES.map((name) => `bin/${name}`).sort(),
  );
  assert.equal(HOOK_BINARIES.length + 4, expectedAssetNames(ASSET_TAG).length);
});

test("a correct directory passes both checks", () => {
  const dir = writeAssetDir(validChecksumLines());
  verifyLocalAssets(dir, ASSET_TAG);
  verifyChecksums(dir, ASSET_TAG);
});

test("verifyLocalAssets rejects a directory that is not exactly the ten assets", () => {
  const cases: [string, (dir: string) => void, string][] = [
    ["missing asset", (dir) => removeAsset(dir, TARBALL), ROSTER],
    ["extra asset", (dir) => writeAsset(dir, "EXTRA.md", "extra\n"), ROSTER],
    ["extra dot file", (dir) => writeAsset(dir, ".hidden", "hidden\n"), ROSTER],
    [
      "tarball built for another tag",
      (dir) => {
        removeAsset(dir, TARBALL);
        writeAsset(dir, "agentics-v9.9.9.tar.gz", "tarball fixture\n");
      },
      ROSTER,
    ],
    [
      "directory entry",
      (dir) => mkdirSync(join(dir, "nested")),
      "asset entry is not a regular file: nested",
    ],
  ];
  for (const [name, prepare, want] of cases) {
    const dir = writeAssetDir(validChecksumLines());
    prepare(dir);
    assert.equal(
      messageOf(() => verifyLocalAssets(dir, ASSET_TAG)),
      want,
      name,
    );
  }
});

test("verifyLocalAssets rejects an existing empty directory with the roster message", () => {
  assert.equal(
    messageOf(() =>
      verifyLocalAssets(scratchDir("agentics-assets-"), ASSET_TAG),
    ),
    ROSTER,
  );
});

test("verifyLocalAssets says a missing directory does not exist", () => {
  const missing = join(scratchDir("agentics-assets-"), "absent");
  assert.equal(
    messageOf(() => verifyLocalAssets(missing, ASSET_TAG)),
    `${missing}: does not exist`,
  );
});

test("verifyLocalAssets names the first entry in byte order that is not a regular file", () => {
  const dir = writeAssetDir(validChecksumLines());
  mkdirSync(join(dir, "zzz-dir"));
  mkdirSync(join(dir, "aaa-dir"));
  assert.equal(
    messageOf(() => verifyLocalAssets(dir, ASSET_TAG)),
    "asset entry is not a regular file: aaa-dir",
  );
});

// The check follows a link, as the shell's `test -f` did.
test(
  "verifyLocalAssets accepts a symbolic link to a regular file",
  {
    skip: platformSkip("symlinks", posix, "creating links needs a POSIX host"),
  },
  () => {
    const dir = writeAssetDir(validChecksumLines());
    const target = join(scratchDir("agentics-assets-"), "notices-target");
    writeFileSync(target, ASSET_FIXTURES[NOTICES_NAME] ?? "");
    removeAsset(dir, NOTICES_NAME);
    symlinkSync(target, join(dir, NOTICES_NAME));
    verifyLocalAssets(dir, ASSET_TAG);
    verifyChecksums(dir, ASSET_TAG);
  },
);

test(
  "verifyLocalAssets rejects a broken symbolic link, which the shell skipped",
  {
    skip: platformSkip("symlinks", posix, "creating links needs a POSIX host"),
  },
  () => {
    const dir = writeAssetDir(validChecksumLines());
    symlinkSync(join(dir, "nowhere"), join(dir, "stray-link"));
    assert.equal(
      messageOf(() => verifyLocalAssets(dir, ASSET_TAG)),
      "asset entry is not a regular file: stray-link",
    );
  },
);

type Mutation = [string, (lines: string[]) => string[], string];

const first = (lines: string[], index: number): string => lines[index] ?? "";

// Each case breaks one rule of the nine correct lines, so removing that rule's
// check lets the case through.
const REJECTED: Mutation[] = [
  ["ten lines", (lines) => [...lines, first(lines, 7)], COUNT],
  // The trim before the count drops one trailing empty element, and only one.
  // A blank line after the ninth asset is a tenth line.
  ["trailing blank line", (lines) => [...lines, ""], COUNT],
  ["seven lines", (lines) => lines.slice(0, 7), COUNT],
  [
    "unsorted pair",
    (lines) => {
      [lines[1], lines[2]] = [first(lines, 2), first(lines, 1)];
      return lines;
    },
    "SHA256SUMS names must be filename-sorted",
  ],
  [
    "duplicate name",
    (lines) => {
      lines[7] = first(lines, 6);
      return lines;
    },
    "SHA256SUMS must cover every non-checksum asset exactly once",
  ],
  [
    "short hash",
    (lines) => {
      lines[0] = first(lines, 0).slice(1);
      return lines;
    },
    "malformed SHA256SUMS line 1",
  ],
  [
    "uppercase hash",
    (lines) => {
      lines[0] =
        first(lines, 0).slice(0, 64).toUpperCase() + first(lines, 0).slice(64);
      return lines;
    },
    "malformed SHA256SUMS line 1",
  ],
  [
    "single space separator",
    (lines) => {
      lines[1] = first(lines, 1).replace("  ", " ");
      return lines;
    },
    "malformed SHA256SUMS line 2",
  ],
  [
    "empty name",
    (lines) => {
      lines[2] = `${first(lines, 2).slice(0, 64)}  `;
      return lines;
    },
    "malformed SHA256SUMS line 3",
  ],
  [
    "name holding a space",
    (lines) => {
      lines[3] = `${first(lines, 3)} tail`;
      return lines;
    },
    "malformed SHA256SUMS line 4",
  ],
  [
    "empty line",
    (lines) => {
      lines[4] = "";
      return lines;
    },
    "malformed SHA256SUMS line 5",
  ],
  [
    "unexpected name",
    (lines) => {
      lines[0] = `${first(lines, 0).slice(0, 64)}  UNEXPECTED.md`;
      return lines;
    },
    "SHA256SUMS names an unexpected asset: UNEXPECTED.md",
  ],
  [
    "digest mismatch",
    (lines) => {
      lines[0] = "0".repeat(64) + first(lines, 0).slice(64);
      return lines;
    },
    "SHA256SUMS digest mismatch for THIRD_PARTY_NOTICES.md",
  ],
];

test("verifyChecksums rejects a file that breaks one rule", async (t) => {
  for (const [name, mutate, want] of REJECTED) {
    await t.test(name, () => {
      const dir = writeAssetDir(mutate(validChecksumLines()));
      assert.equal(
        messageOf(() => verifyChecksums(dir, ASSET_TAG)),
        want,
      );
    });
  }
});

// The split on newlines reduces a file with no lines to zero lines rather than
// one.
test("verifyChecksums rejects an empty file with the nine-line message", () => {
  const dir = writeAssetDir([]);
  assert.equal(
    messageOf(() => verifyChecksums(dir, ASSET_TAG)),
    COUNT,
  );
});

test("verifyChecksums accepts a last line with no newline after it", () => {
  const dir = writeAssetDir(validChecksumLines());
  writeFileSync(join(dir, CHECKSUMS_NAME), validChecksumLines().join("\n"));
  verifyChecksums(dir, ASSET_TAG);
});

test("verifyChecksums says a missing SHA256SUMS does not exist", () => {
  const dir = scratchDir("agentics-assets-");
  assert.equal(
    messageOf(() => verifyChecksums(dir, ASSET_TAG)),
    `${join(dir, "SHA256SUMS")}: does not exist`,
  );
});

test("verifyChecksums rejects a name made of white space that is not an ASCII space", () => {
  const nbsp = String.fromCharCode(0xa0);
  for (const tail of ["\t", "\r", nbsp]) {
    const lines = validChecksumLines();
    lines[1] = `${first(lines, 1)}${tail}`;
    assert.equal(
      messageOf(() => verifyChecksums(writeAssetDir(lines), ASSET_TAG)),
      "malformed SHA256SUMS line 2",
      JSON.stringify(tail),
    );
  }
});

test("verifyChecksums says a listed file that is missing does not exist", () => {
  const dir = writeAssetDir(validChecksumLines());
  removeAsset(dir, "agx-linux-amd64");
  assert.equal(
    messageOf(() => verifyChecksums(dir, ASSET_TAG)),
    `${join(dir, "agx-linux-amd64")}: does not exist`,
  );
});

test("parseChecksumLine splits the digest and the name", () => {
  const digest = "a".repeat(64);
  assert.deepEqual(parseChecksumLine(`${digest}  name`), {
    digest,
    name: "name",
  });
  for (const line of [
    "",
    digest,
    `${digest} name`,
    `${digest}   name`,
    `${digest}  `,
    `${digest}  a b`,
    `${"A".repeat(64)}  name`,
    `${"a".repeat(63)}  name`,
    `${digest}\t name`,
  ]) {
    assert.equal(parseChecksumLine(line), undefined, JSON.stringify(line));
  }
});

test("fileDigest is the lowercase hex SHA-256 of the bytes", () => {
  const dir = writeAssetDir(validChecksumLines());
  assert.equal(fileDigest(join(dir, NOTICES_NAME)), assetDigest(NOTICES_NAME));
  assert.equal(
    fileDigest(join(dir, NOTICES_NAME)),
    "8a52e1dc9a6f514d6a2ddfc2f72ff372507021c89130a23eeb59b64971753342",
  );
  assert.throws(() => fileDigest(join(dir, "absent")), {
    message: `${join(dir, "absent")}: does not exist`,
  });
});
