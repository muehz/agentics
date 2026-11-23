import { createHash } from "node:crypto";
import { statSync } from "node:fs";
import { join } from "node:path";
import { readDirectory, readFile } from "./files.ts";

// The asset roster and the checksum verification the stage and verify commands
// share. Every error here is the reason alone, without a prefix; each caller
// adds its own `release: <phase>: `.

// The contract hook binaries, in the order the release publishes them. Every
// one is built from hooks/go as it is at the target commit.
export const HOOK_BINARIES: readonly string[] = [
  "agx-darwin-amd64",
  "agx-darwin-arm64",
  "agx-linux-amd64",
  "agx-linux-arm64",
  "agx-windows-amd64.exe",
  "agx-windows-arm64.exe",
];

/** Checkout-relative Git paths, in the stage workflow's binary rebuild order. */
export const HOOK_BINARY_PATHS: readonly string[] = HOOK_BINARIES.map(
  (name) => `hooks/bin/${name}`,
);

// The notices asset's source path and published name.
export const NOTICES_NAME = "THIRD_PARTY_NOTICES.md";

// The digests of all other assets.
export const CHECKSUMS_NAME = "SHA256SUMS";

// The digest field of a SHA256SUMS line. The uppercase form is rejected,
// because sha256sum writes the lowercase form.
const DIGEST = /^[0-9a-f]{64}$/;

// Strings sort by UTF-16 code unit, which equals byte order for these ASCII
// names.
function byteOrder(names: string[]): string[] {
  return names.sort();
}

export function tarballName(tag: string): string {
  return `agentics-${tag}.tar.gz`;
}

export function pluginTarballName(tag: string): string {
  return `agentics-plugin-${tag}.tar.gz`;
}

/** The assets derived from the target commit, in stage comparison order. */
export function derivableAssetNames(tag: string): string[] {
  return [
    tarballName(tag),
    pluginTarballName(tag),
    ...HOOK_BINARIES,
    NOTICES_NAME,
  ];
}

// The assets SHA256SUMS covers, in the order it must list them. The
// order is the C-locale sort order the retired publish script used.
export function checksumAssetNames(tag: string): string[] {
  return byteOrder(derivableAssetNames(tag));
}

// The release asset names, in C-locale sort order.
export function expectedAssetNames(tag: string): string[] {
  return byteOrder([CHECKSUMS_NAME, ...checksumAssetNames(tag)]);
}

function sameNames(a: readonly string[], b: readonly string[]): boolean {
  return a.length === b.length && a.every((name, index) => name === b[index]);
}

// Throws unless dir holds exactly the assets for tag and nothing else. It
// reads every entry, including a name that starts with a dot. A symbolic link
// that resolves to a regular file is an asset, as `test -f` had it; a broken
// link or a directory is not.
export function verifyLocalAssets(dir: string, tag: string): void {
  const names = readDirectory(dir);
  for (const name of names) {
    let regular = false;
    try {
      regular = statSync(join(dir, name)).isFile();
    } catch {
      regular = false;
    }
    if (!regular) throw new Error(`asset entry is not a regular file: ${name}`);
  }
  if (!sameNames(names, expectedAssetNames(tag))) {
    throw new Error(
      `assets must contain exactly the ten expected names for ${tag}`,
    );
  }
}

// Throws unless dir's SHA256SUMS covers the non-checksum assets exactly
// once each, in strictly ascending name order, with a digest that matches the
// file. The line shape is the one `sha256sum -c` reads back: the digest, two
// spaces, then the name.
export function verifyChecksums(dir: string, tag: string): void {
  const contents = readFile(join(dir, CHECKSUMS_NAME)).toString("utf8");
  const lines = contents.split("\n");
  // A written file ends with a newline, and the split then leaves a final empty
  // element. A last line with no newline after it is still read.
  if (lines.length > 0 && lines[lines.length - 1] === "") lines.pop();
  const expected = checksumAssetNames(tag);
  if (lines.length !== expected.length) {
    throw new Error("SHA256SUMS must contain exactly nine sorted asset lines");
  }

  const listed: string[] = [];
  let previous = "";
  lines.forEach((line, index) => {
    const parsed = parseChecksumLine(line);
    if (parsed === undefined) {
      throw new Error(`malformed SHA256SUMS line ${index + 1}`);
    }
    const { digest, name } = parsed;
    if (!expected.includes(name)) {
      throw new Error(`SHA256SUMS names an unexpected asset: ${name}`);
    }
    if (name < previous) {
      throw new Error("SHA256SUMS names must be filename-sorted");
    }
    if (fileDigest(join(dir, name)) !== digest) {
      throw new Error(`SHA256SUMS digest mismatch for ${name}`);
    }
    listed.push(name);
    previous = name;
  });

  // A repeated name passes the order check above, which rejects only a
  // descending step. This comparison is what rejects it.
  if (!sameNames(listed, expected)) {
    throw new Error(
      "SHA256SUMS must cover every non-checksum asset exactly once",
    );
  }
}

// Splits one SHA256SUMS line into its digest and its name. The separator is two
// spaces, and a name that holds white space is rejected because the separator
// would then be ambiguous.
export function parseChecksumLine(
  line: string,
): { digest: string; name: string } | undefined {
  const first = line.indexOf(" ");
  if (first < 0) return undefined;
  const digest = line.slice(0, first);
  const remainder = line.slice(first + 1);
  if (!remainder.startsWith(" ")) return undefined;
  const name = remainder.slice(1);
  if (!DIGEST.test(digest)) return undefined;
  if (name === "" || /\s/.test(name)) return undefined;
  return { digest, name };
}

// The SHA-256 digest of one file, in the lowercase hexadecimal form SHA256SUMS
// carries. A file that cannot be read throws a FileError.
export function fileDigest(path: string): string {
  return createHash("sha256").update(readFile(path)).digest("hex");
}
