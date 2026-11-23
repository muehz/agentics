import { createHash } from "node:crypto";
import { mkdirSync, rmSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { CHECKSUMS_NAME } from "../assets.ts";
import { scratchDir } from "./support.ts";

// Fixtures for the asset roster, shared by the tests of every command that
// verifies an asset directory.

// The tag the asset fixtures are built for.
export const ASSET_TAG = "v1.2.3";

// The content of the nine checksummed assets. The bytes differ per name, so a
// digest copied from another asset cannot pass.
export const ASSET_FIXTURES: Readonly<Record<string, string>> = {
  [`agentics-${ASSET_TAG}.tar.gz`]: "tarball fixture\n",
  [`agentics-plugin-${ASSET_TAG}.tar.gz`]: "installable plugin fixture\n",
  "agx-darwin-amd64": "darwin-amd64 fixture\n",
  "agx-darwin-arm64": "darwin-arm64 fixture\n",
  "agx-linux-amd64": "linux-amd64 fixture\n",
  "agx-linux-arm64": "linux-arm64 fixture\n",
  "agx-windows-amd64.exe": "windows-amd64 fixture\n",
  "agx-windows-arm64.exe": "windows-arm64 fixture\n",
  "THIRD_PARTY_NOTICES.md": "notices fixture\n",
};

// The digest of one fixture asset, in the lowercase hexadecimal form
// SHA256SUMS carries.
export function assetDigest(name: string): string {
  const contents = ASSET_FIXTURES[name];
  if (contents === undefined) throw new Error(`no fixture for asset ${name}`);
  return createHash("sha256").update(contents).digest("hex");
}

// The nine correct SHA256SUMS lines, in the order a correct file lists them.
export function validChecksumLines(): string[] {
  return [
    "THIRD_PARTY_NOTICES.md",
    `agentics-plugin-${ASSET_TAG}.tar.gz`,
    `agentics-${ASSET_TAG}.tar.gz`,
    "agx-darwin-amd64",
    "agx-darwin-arm64",
    "agx-linux-amd64",
    "agx-linux-arm64",
    "agx-windows-amd64.exe",
    "agx-windows-arm64.exe",
  ].map((name) => `${assetDigest(name)}  ${name}`);
}

export function writeAsset(dir: string, name: string, contents: string): void {
  mkdirSync(dir, { recursive: true });
  writeFileSync(join(dir, name), contents);
}

export function removeAsset(dir: string, name: string): void {
  rmSync(join(dir, name));
}

// Writes the nine fixture assets and a SHA256SUMS built from lines, and
// returns the directory.
export function writeAssetDir(lines: readonly string[]): string {
  const dir = scratchDir("agentics-assets-");
  for (const [name, contents] of Object.entries(ASSET_FIXTURES)) {
    writeAsset(dir, name, contents);
  }
  const body = lines.length > 0 ? `${lines.join("\n")}\n` : "";
  writeAsset(dir, CHECKSUMS_NAME, body);
  return dir;
}
