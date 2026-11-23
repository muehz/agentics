import assert from "node:assert/strict";
import {
  existsSync,
  lstatSync,
  mkdirSync,
  readdirSync,
  readFileSync,
  statSync,
  writeFileSync,
} from "node:fs";
import { join } from "node:path";
import { test } from "node:test";
import { extractTar } from "./tar.ts";
import {
  posix,
  scratchDir,
  sh,
  initRepo,
  commitFiles,
} from "./testing/support.ts";

interface Entry {
  name: string;
  type?: string;
  body?: Buffer | string;
  linkname?: string;
  prefix?: string;
  magic?: "ustar" | "gnu" | "v7";
  mode?: number;
  declaredSize?: number;
}

function paxRecord(key: string, value: string): Buffer {
  const rest = Buffer.from(` ${key}=${value}\n`, "utf8");
  let length = rest.length + 1;
  while (String(length).length + rest.length !== length)
    length = String(length).length + rest.length;
  return Buffer.concat([Buffer.from(String(length)), rest]);
}

function header(entry: Entry, size: number): Buffer {
  const block = Buffer.alloc(512);
  block.write(entry.name, 0, 100, "utf8");
  block.write(
    `${(entry.mode ?? 0o644).toString(8).padStart(7, "0")}\0`,
    100,
    "latin1",
  );
  block.write("0000000\0", 108, "latin1");
  block.write("0000000\0", 116, "latin1");
  block.write(`${size.toString(8).padStart(11, "0")}\0`, 124, "latin1");
  block.write("00000000000\0", 136, "latin1");
  block.fill(0x20, 148, 156);
  block.write(entry.type ?? "0", 156, "latin1");
  if (entry.linkname !== undefined)
    block.write(entry.linkname, 157, 100, "utf8");
  const magic = entry.magic ?? "ustar";
  if (magic === "ustar") {
    block.write("ustar\0", 257, "latin1");
    block.write("00", 263, "latin1");
  } else if (magic === "gnu") {
    block.write("ustar  \0", 257, "latin1");
  }
  if (entry.prefix !== undefined) block.write(entry.prefix, 345, 155, "utf8");
  let sum = 0;
  for (const byte of block) sum += byte;
  block.write(`${sum.toString(8).padStart(6, "0")}\0 `, 148, "latin1");
  return block;
}

function entryBytes(entry: Entry): Buffer {
  const body =
    typeof entry.body === "string"
      ? Buffer.from(entry.body)
      : (entry.body ?? Buffer.alloc(0));
  const padding = Buffer.alloc((512 - (body.length % 512)) % 512);
  return Buffer.concat([
    header(entry, entry.declaredSize ?? body.length),
    body,
    padding,
  ]);
}

function tar(entries: readonly Entry[], end = true): Buffer {
  return Buffer.concat([
    ...entries.map(entryBytes),
    ...(end ? [Buffer.alloc(1024)] : []),
  ]);
}

function extractFrom(archive: Buffer): { dest: string; parent: string } {
  const parent = scratchDir("agentics-tar-");
  writeFileSync(join(parent, "in.tar"), archive);
  const dest = join(parent, "dest");
  mkdirSync(dest);
  extractTar(join(parent, "in.tar"), dest);
  return { dest, parent };
}

function refuses(
  archive: Buffer,
  pattern: RegExp,
): { dest: string; parent: string } {
  const parent = scratchDir("agentics-tar-");
  writeFileSync(join(parent, "in.tar"), archive);
  const dest = join(parent, "dest");
  mkdirSync(dest);
  assert.throws(() => extractTar(join(parent, "in.tar"), dest), pattern);
  return { dest, parent };
}

test("extractTar writes regular files and directories, including a body that spans blocks", () => {
  const big = Buffer.alloc(1500, "x");
  const { dest } = extractFrom(
    tar([
      { name: "hooks/", type: "5", mode: 0o755 },
      { name: "hooks/go/main.go", body: "package main\n" },
      { name: "hooks/go/big.bin", body: big },
      { name: "hooks/SHA256SUMS", body: "digest  bin/x\n" },
    ]),
  );
  assert.equal(
    readFileSync(join(dest, "hooks/go/main.go"), "utf8"),
    "package main\n",
  );
  assert.deepEqual(readFileSync(join(dest, "hooks/go/big.bin")), big);
  assert.equal(
    readFileSync(join(dest, "hooks/SHA256SUMS"), "utf8"),
    "digest  bin/x\n",
  );
  assert.ok(statSync(join(dest, "hooks")).isDirectory());
});

test(
  "extractTar never gives an extracted file the executable bit the archive carried",
  { skip: posix ? false : "POSIX modes are not meaningful on Windows" },
  () => {
    const { dest } = extractFrom(
      tar([{ name: "run.sh", body: "#!/bin/sh\n", mode: 0o755 }]),
    );
    assert.equal(statSync(join(dest, "run.sh")).mode & 0o111, 0);
  },
);

test("extractTar treats an old-style regular file whose name ends in a slash as a directory", () => {
  const { dest } = extractFrom(
    tar([
      { name: "hooks/go/", type: "\0" },
      { name: "hooks/go/a.go", type: "\0", body: "a" },
    ]),
  );
  assert.ok(statSync(join(dest, "hooks/go")).isDirectory());
  assert.equal(readFileSync(join(dest, "hooks/go/a.go"), "utf8"), "a");
});

test("extractTar applies a pax path to the next entry only, and ignores a pax global header", () => {
  const longName = `${"d".repeat(60)}/${"e".repeat(60)}/file.txt`;
  const { dest } = extractFrom(
    tar([
      {
        name: "pax_global_header",
        type: "g",
        body: paxRecord("comment", "0123456789abcdef"),
      },
      {
        name: "PaxHeaders/ignored",
        type: "x",
        body: paxRecord("path", longName),
      },
      { name: "truncated-name-that-must-lose", body: "long" },
      { name: "short.txt", body: "short" },
    ]),
  );
  assert.equal(readFileSync(join(dest, longName), "utf8"), "long");
  assert.equal(readFileSync(join(dest, "short.txt"), "utf8"), "short");
  assert.deepEqual(readdirSync(dest).sort(), ["d".repeat(60), "short.txt"]);
});

test("extractTar honors a pax size record", () => {
  const body = Buffer.alloc(700, "y");
  const { dest } = extractFrom(
    tar([
      { name: "PaxHeaders/sized", type: "x", body: paxRecord("size", "700") },
      { name: "sized.bin", body, declaredSize: 0 },
    ]),
  );
  assert.deepEqual(readFileSync(join(dest, "sized.bin")), body);
});

test("extractTar joins a ustar prefix to the name and ignores the prefix field of a GNU header", () => {
  const { dest } = extractFrom(
    tar([
      { name: "inner.txt", prefix: "outer/dir", body: "p" },
      { name: "gnu.txt", prefix: "must/not/apply", magic: "gnu", body: "g" },
    ]),
  );
  assert.equal(readFileSync(join(dest, "outer/dir/inner.txt"), "utf8"), "p");
  assert.equal(readFileSync(join(dest, "gnu.txt"), "utf8"), "g");
});

test("extractTar applies a GNU long name to the next entry", () => {
  const longName = `${"g".repeat(120)}.txt`;
  const { dest } = extractFrom(
    tar([
      { name: "././@LongLink", type: "L", body: `${longName}\0` },
      { name: longName.slice(0, 99), body: "gnu long" },
      { name: "plain.txt", body: "plain" },
    ]),
  );
  assert.equal(readFileSync(join(dest, longName), "utf8"), "gnu long");
  assert.equal(readFileSync(join(dest, "plain.txt"), "utf8"), "plain");
});

test("extractTar skips symlinks, hard links, devices and fifos and keeps reading", () => {
  const { dest } = extractFrom(
    tar([
      { name: "link", type: "2", linkname: "/etc/passwd", declaredSize: 5 },
      { name: "after.txt", body: "kept" },
      { name: "hard", type: "1", linkname: "link" },
      { name: "char", type: "3" },
      { name: "fifo", type: "6" },
    ]),
  );
  for (const name of ["link", "hard", "char", "fifo"]) {
    assert.equal(
      existsSync(join(dest, name)),
      false,
      `${name} must not be created`,
    );
  }
  assert.equal(readFileSync(join(dest, "after.txt"), "utf8"), "kept");
});

const refusedNames: readonly (readonly [string, Entry])[] = [
  ["a parent segment", { name: "../evil.txt" }],
  ["a parent segment that climbs out", { name: "go/../../evil.txt" }],
  ["a parent segment that stays inside", { name: "a/../b.txt" }],
  ["an absolute path", { name: "/abs/evil.txt" }],
  ["a backslash", { name: "dir\\evil.txt" }],
  ["a drive letter", { name: "C:evil.txt" }],
  ["a reserved device name", { name: "hooks/NUL" }],
  ["a reserved device name with an extension", { name: "hooks/aux.go" }],
  ["a reserved device name in a directory position", { name: "com1/file.txt" }],
  [
    "a symlink type and a non-local name",
    { name: "/abs/link", type: "2", linkname: "x" },
  ],
];

for (const [label, entry] of refusedNames) {
  test(`extractTar refuses an entry with ${label}`, () => {
    const { dest, parent } = refuses(tar([entry]), /escapes/);
    assert.deepEqual(readdirSync(dest), []);
    assert.equal(existsSync(join(parent, "evil.txt")), false);
    assert.equal(existsSync(join(parent, "..", "evil.txt")), false);
  });
}

test("extractTar refuses a pax path with a NUL byte and a pax path that is empty", () => {
  for (const value of ["a\0b", ""]) {
    const archive = tar([
      { name: "x", type: "x", body: paxRecord("path", value) },
      { name: "innocent.txt", body: "no" },
    ]);
    const { dest } = refuses(archive, /escapes/);
    assert.deepEqual(readdirSync(dest), []);
  }
});

test("extractTar accepts names that merely look like a refused one", () => {
  const { dest } = extractFrom(
    tar([
      { name: "./hooks/go/console.go", body: "1" },
      { name: "hooks//go/nulled.txt", body: "2" },
      { name: "auxiliary/comfort.txt", body: "3" },
      { name: "hooks/go/a..b", body: "4" },
    ]),
  );
  assert.equal(readFileSync(join(dest, "hooks/go/console.go"), "utf8"), "1");
  assert.equal(readFileSync(join(dest, "hooks/go/nulled.txt"), "utf8"), "2");
  assert.equal(readFileSync(join(dest, "auxiliary/comfort.txt"), "utf8"), "3");
  assert.equal(readFileSync(join(dest, "hooks/go/a..b"), "utf8"), "4");
});

test("extractTar refuses a header whose checksum does not match", () => {
  const archive = tar([{ name: "a.txt", body: "a" }]);
  archive[0] = (archive[0] as number) ^ 0x01;
  const { dest } = refuses(archive, /invalid tar header/);
  assert.deepEqual(readdirSync(dest), []);
});

test("extractTar refuses an entry whose body is cut short", () => {
  const archive = tar([{ name: "a.bin", body: Buffer.alloc(600, "z") }], false);
  refuses(archive.subarray(0, 512 + 100), /unexpected EOF/);
});

test("extractTar refuses a header block that is cut short", () => {
  refuses(
    Buffer.concat([
      tar([{ name: "a.txt", body: "a" }], false),
      Buffer.alloc(100, 1),
    ]),
    /unexpected EOF/,
  );
});

test("extractTar stops at the end-of-archive marker", () => {
  const { dest } = extractFrom(
    Buffer.concat([
      tar([{ name: "kept.txt", body: "k" }]),
      tar([{ name: "after-end.txt", body: "no" }]),
    ]),
  );
  assert.deepEqual(readdirSync(dest), ["kept.txt"]);
});

test("extractTar extracts what git archive writes: pax paths, nested directories, binary bytes, and a skipped symlink", () => {
  const repo = scratchDir("agentics-tar-git-");
  initRepo(repo);
  const longDirectory = Array.from(
    { length: 5 },
    (_, i) => `directory-${i}-${"n".repeat(18)}`,
  ).join("/");
  const binary = Buffer.from([0, 255, 13, 10, 200, 7, 0, 1]);
  // One path segment over 100 characters cannot be split into a ustar prefix and name, so git must write a pax path record.
  const longName = `${"f".repeat(110)}.txt`;
  const deepPath = `hooks/go/${longDirectory}/deep.txt`;
  const files: Record<string, string | Buffer> = {
    "hooks/go/go.mod": "module example.com/hook\n\ngo 1.27.3\n",
    "hooks/go/cmd/hook/main.go": "package main\n\nfunc main() {}\n",
    "hooks/SHA256SUMS": "0000  bin/agx\n",
    "hooks/go/binary.dat": binary,
    [deepPath]: "deep\n",
    [`hooks/go/${longName}`]: "long\n",
  };
  const longestPath = Math.max(...Object.keys(files).map((p) => p.length));
  assert.ok(
    longestPath <= 180,
    `the longest fixture path must stay short enough for the Windows 259-character path limit once the scratch directory is added; it is ${longestPath} characters`,
  );
  assert.ok(
    deepPath.length > 100,
    `the nested path must exceed 100 characters because this test relies on git writing a ustar prefix field for it; it is ${deepPath.length} characters`,
  );
  commitFiles(repo, files, "fixture");
  assert.ok(longName.length > 100, "the fixture must force a pax path header");
  // A symlink tree entry needs no symlink support on the host: write the blob and index it as mode 120000.
  const blob = sh(repo, "git", "hash-object", "-w", "hooks/SHA256SUMS").trim();
  sh(
    repo,
    "git",
    "update-index",
    "--add",
    "--cacheinfo",
    `120000,${blob},hooks/go/evil-link`,
  );
  sh(repo, "git", "commit", "-q", "-m", "add a symlink entry");
  const tarPath = join(scratchDir("agentics-tar-git-out-"), "src.tar");
  sh(
    repo,
    "git",
    "archive",
    "--format=tar",
    `--output=${tarPath}`,
    "HEAD",
    "hooks/go",
    "hooks/SHA256SUMS",
  );

  const dest = join(scratchDir("agentics-tar-git-dest-"), "out");
  mkdirSync(dest);
  extractTar(tarPath, dest);
  for (const [path, contents] of Object.entries(files)) {
    assert.deepEqual(
      readFileSync(join(dest, path)),
      Buffer.from(contents),
      path,
    );
  }
  assert.throws(() => lstatSync(join(dest, "hooks/go/evil-link")), {
    code: "ENOENT",
  });
  assert.equal(existsSync(join(dest, "pax_global_header")), false);
});
