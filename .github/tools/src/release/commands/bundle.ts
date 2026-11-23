import { createHash } from "node:crypto";
import {
  chmodSync,
  mkdtempSync,
  readdirSync,
  rmSync,
  statSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { gzipSync } from "node:zlib";
import {
  CHECKSUMS_NAME,
  checksumAssetNames,
  fileDigest,
  HOOK_BINARIES,
  NOTICES_NAME,
  pluginTarballName,
  tarballName,
} from "../assets.ts";
import { parseArgs } from "../args.ts";
import { describeExit, git, gitBuffer, runGit, succeeded } from "../github.ts";
import { buildCommittedHooks } from "../hooks-build.ts";
import {
  EXIT_FAILURE,
  EXIT_OK,
  EXIT_USAGE,
  report,
  type Env,
  type Run,
} from "../io.ts";
import { isLocalTarPath, tarEntries } from "../tar.ts";
import { MANIFESTS, readSnapshotAt } from "../version.ts";

const FLAGS = ["repo", "sha", "main-ref", "tag", "out"] as const;

function describe(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

// One git call whose failure is the failure sentence alone, without git's stderr.
async function probe(
  repo: string,
  args: readonly string[],
  env: Env,
): Promise<{ stdout: string; failure: string | undefined }> {
  const result = await runGit(repo, args, { env });
  return {
    stdout: result.stdout.toString("utf8"),
    failure: succeeded(result) ? undefined : describeExit(result),
  };
}

/** Returns the version tag of a commit on mainRef. Failure messages are ready for the caller's diagnostic prefix. */
export async function guardBundleTarget(
  { repo, sha, mainRef }: { repo: string; sha: string; mainRef: string },
  env: Env,
): Promise<string> {
  const type = await probe(repo, ["cat-file", "-t", sha], env);
  if (type.failure !== undefined) {
    throw new Error(
      `target ${sha} is not a readable Git object: ${type.failure}`,
    );
  }
  const objectType = type.stdout.trim();
  if (objectType !== "commit") {
    throw new Error(
      `target ${sha} is not a commit (object type ${objectType})`,
    );
  }

  const ancestor = await probe(
    repo,
    ["merge-base", "--is-ancestor", sha, mainRef],
    env,
  );
  if (ancestor.failure !== undefined) {
    throw new Error(`target ${sha} is not on ${mainRef}: ${ancestor.failure}`);
  }

  const snapshot = await readSnapshotAt(repo, sha);
  return `v${snapshot.raw}`;
}

// Runs before the first write. A directory that already holds a file would leave the earlier file
// in the bundle, and the publish job verifies the whole directory.
function checkBundleDirectories(repo: string, out: string): string | undefined {
  let isDirectory = false;
  try {
    isDirectory = statSync(repo).isDirectory();
  } catch {
    isDirectory = false;
  }
  if (!isDirectory) return `repository does not exist: ${repo}`;
  let entries: string[];
  try {
    entries = readdirSync(out);
  } catch {
    return `output directory does not exist: ${out}`;
  }
  if (entries.length !== 0) return `output directory must be empty: ${out}`;
  return undefined;
}

const PLUGIN_LOGO = "assets/logo.svg";
const REQUIRED_PLUGIN_FILES = [
  ...MANIFESTS.map(({ path }) => path),
  ".agents/plugins/marketplace.json",
  "hooks.json",
  "hooks/hooks.claude.json",
  "hooks/hooks.codex.json",
  "hooks/hooks.copilot.json",
  "hooks/run-hook.cmd",
  "hooks/fetch-hook.ps1",
  "hooks/SHA256SUMS",
  "bin/agx",
  "LICENSE",
  NOTICES_NAME,
];
const PLUGIN_FILES = [...REQUIRED_PLUGIN_FILES, PLUGIN_LOGO];

interface PluginBlob {
  mode: number;
  oid: string;
}

function registerPluginPath(
  path: string,
  spellings: Map<string, string>,
): void {
  if (!isLocalTarPath(path))
    throw new Error(`invalid plugin archive path: ${path}`);
  let prefix = "";
  for (const component of path.split("/")) {
    if (
      /[<>:"|?*\p{Cc}]|[. ]$/u.test(component) ||
      /^(?:COM|LPT)[¹²³] *(?:\.|$)/i.test(component)
    )
      throw new Error(`invalid plugin archive path: ${path}`);
    prefix = prefix ? `${prefix}/${component}` : component;
    // Case-insensitive, normalization-insensitive filesystems can merge two directories
    // even when their children all have distinct names.
    const key = prefix
      .normalize("NFD")
      .toLowerCase()
      .toUpperCase()
      .normalize("NFD");
    const previous = spellings.get(key);
    if (previous !== undefined && previous !== prefix)
      throw new Error(
        `plugin archive paths collide: ${previous} and ${prefix}`,
      );
    spellings.set(key, prefix);
  }
}

// A pathspec for a directory also matches its descendants. Inspect the selected tree before
// archiving so a directory or link cannot stand in for a required regular file.
async function pluginBlobs(
  repo: string,
  sha: string,
  env: Env,
): Promise<Map<string, PluginBlob>> {
  const listing = await gitBuffer(
    repo,
    ["ls-tree", "-rz", sha, "--", ...PLUGIN_FILES, "skills"],
    { env },
  );
  const blobs = new Map<string, PluginBlob>();
  const spellings = new Map<string, string>();
  registerPluginPath("bin/agx.exe", spellings);
  let hasSkills = false;
  const entries = new TextDecoder("utf-8", { fatal: true }).decode(listing);
  for (const entry of entries.split("\0").filter(Boolean)) {
    const tab = entry.indexOf("\t");
    const [mode, type, oid] = entry.slice(0, tab).split(" ");
    const path = entry.slice(tab + 1);
    registerPluginPath(path, spellings);
    if (path.startsWith("skills/")) hasSkills = true;
    else if (!PLUGIN_FILES.includes(path))
      throw new Error(`unexpected plugin path: ${path}`);
    if (type !== "blob" || (mode !== "100644" && mode !== "100755") || !oid)
      throw new Error(`expected a regular plugin file at ${path}`);
    if (["bin/agx", "hooks/run-hook.cmd"].includes(path) && mode !== "100755")
      throw new Error(`expected an executable plugin file at ${path}`);
    blobs.set(path, { mode: mode === "100755" ? 0o755 : 0o644, oid });
  }
  for (const path of REQUIRED_PLUGIN_FILES) {
    if (!blobs.has(path)) throw new Error(`missing plugin file: ${path}`);
  }
  if (!blobs.has(PLUGIN_LOGO)) {
    const { oid } = blobs.get(".antigravity-plugin/plugin.json")!;
    const manifest = JSON.parse(
      await git(repo, ["cat-file", "blob", oid], { env }),
    );
    if (manifest.logo === PLUGIN_LOGO)
      throw new Error(`missing plugin file: ${PLUGIN_LOGO}`);
  }
  if (!hasSkills) throw new Error("missing plugin skills");
  return blobs;
}

// Git attributes can omit a path (export-ignore) or change its bytes (export-subst, eol).
// Check the emitted tar against the selected blob IDs before giving it a release checksum.
function verifyPluginTar(
  archive: Buffer,
  prefix: string,
  blobs: Map<string, PluginBlob>,
  launcher: Buffer,
): void {
  const directories = new Set<string>([prefix]);
  const remaining = new Set([...blobs.keys(), "bin/agx.exe"]);
  for (const path of remaining) {
    for (
      let slash = path.indexOf("/");
      slash >= 0;
      slash = path.indexOf("/", slash + 1)
    )
      directories.add(prefix + path.slice(0, slash + 1));
  }
  for (const { name, typeflag, mode, body } of tarEntries(archive)) {
    if (typeflag === "5" && mode === 0o755 && directories.delete(name))
      continue;
    const path = name.slice(prefix.length);
    if (!name.startsWith(prefix) || typeflag !== "0" || !remaining.delete(path))
      throw new Error(`unexpected plugin archive entry: ${name}`);
    if (path === "bin/agx.exe") {
      if (mode !== 0o644 || !body.equals(launcher))
        throw new Error(
          "plugin archive does not contain the verified launcher",
        );
    } else {
      const blob = blobs.get(path)!;
      const hash = createHash(blob.oid.length === 64 ? "sha256" : "sha1")
        .update(`blob ${body.length}\0`)
        .update(body)
        .digest("hex");
      if (hash !== blob.oid || mode !== blob.mode)
        throw new Error(
          `plugin archive differs from the selected tree at ${path}`,
        );
    }
  }
  for (const missing of [...remaining, ...directories])
    throw new Error(`missing plugin archive entry: ${missing}`);
}

async function writePluginArchive(
  repo: string,
  sha: string,
  tag: string,
  out: string,
  env: Env,
  launcher: Buffer,
): Promise<void> {
  const blobs = await pluginBlobs(repo, sha, env);
  const prefix = `agentics-plugin-${tag}/`;
  const scratch = mkdtempSync(join(tmpdir(), "agentics-plugin-"));
  try {
    const file = join(scratch, "agx.exe");
    writeFileSync(file, launcher, { flag: "wx", mode: 0o600 });
    chmodSync(file, 0o644);
    // --add-file uses the prefix in effect when the option is read. Tracked paths use the
    // final prefix. The commit supplies the timestamp for both.
    const archive = await gitBuffer(
      repo,
      [
        "-c",
        "tar.umask=0022",
        "-c",
        "core.autocrlf=false",
        "-c",
        "core.eol=lf",
        "archive",
        "--format=tar",
        `--prefix=${prefix}bin/`,
        `--add-file=${file}`,
        `--prefix=${prefix}`,
        sha,
        "--",
        ...PLUGIN_FILES.filter((path) => blobs.has(path)),
        "skills",
      ],
      { env },
    );
    verifyPluginTar(archive, prefix, blobs, launcher);
    writeFileSync(join(out, pluginTarballName(tag)), gzipSync(archive), {
      mode: 0o644,
    });
  } finally {
    rmSync(scratch, { recursive: true, force: true });
  }
}

// Write SHA256SUMS only after every asset has been built.
export async function writeBundle(
  repo: string,
  sha: string,
  tag: string,
  out: string,
  env: Env,
): Promise<void> {
  const archiveName = tarballName(tag);
  // --output makes git write the file, so the compressed bytes never pass through this process.
  await git(
    repo,
    [
      "archive",
      "--format=tar.gz",
      `--prefix=agentics-${tag}/`,
      `--output=${join(out, archiveName)}`,
      sha,
    ],
    { env },
  );

  const launcher = await buildCommittedHooks(repo, sha, out, env);
  await writePluginArchive(repo, sha, tag, out, env, launcher);
  await writeCommittedFile(
    repo,
    sha,
    NOTICES_NAME,
    join(out, NOTICES_NAME),
    env,
  );

  // The four Unix binaries run straight from the download, so they carry the executable bit.
  // Windows derives that from the file extension instead, so the two .exe binaries are chmod'd back
  // down to 0644: `go build` on a Linux runner writes every output 0755, and the published .exe
  // carries no executable bit.
  for (const name of HOOK_BINARIES) {
    chmodSync(join(out, name), name.endsWith(".exe") ? 0o644 : 0o755);
  }

  writeChecksums(out, checksumAssetNames(tag));
}

// Copies one path out of the target commit, never out of the working tree, so a dirty checkout
// cannot reach a release. The bytes go through as a Buffer: decoding them to a string would change
// any byte sequence that is not valid UTF-8.
//
// The 0o644 request is masked by the umask, so the result is 0644 under the umask 022 that Actions
// uses.
async function writeCommittedFile(
  repo: string,
  sha: string,
  path: string,
  destination: string,
  env: Env,
): Promise<void> {
  const contents = await gitBuffer(repo, ["show", `${sha}:${path}`], { env });
  writeFileSync(destination, contents, { mode: 0o644 });
}

// Writes each asset's digest, two spaces, and its name in the supplied C-locale order.
function writeChecksums(out: string, names: readonly string[]): void {
  const lines = names.map(
    (name) => `${fileDigest(join(out, name))}  ${name}\n`,
  );
  writeFileSync(join(out, CHECKSUMS_NAME), lines.join(""), { mode: 0o644 });
}

export const run: Run = async (argv, io, env) => {
  const prefix = "release: bundle";
  const parsed = parseArgs(argv, FLAGS);
  const values = parsed.ok ? parsed.values : {};
  const { repo, sha, "main-ref": mainRef, tag, out } = values;
  if (!parsed.ok) io.stderr.write(`${parsed.message}\n`);
  if (!repo || !sha || !mainRef || !tag || !out) {
    return report(
      io,
      prefix,
      "--repo, --sha, --main-ref, --tag, and --out are required",
      EXIT_USAGE,
    );
  }

  const problem = checkBundleDirectories(repo, out);
  if (problem !== undefined) return report(io, prefix, problem, EXIT_FAILURE);

  let expected: string;
  try {
    expected = (await guardBundleTarget({ repo, sha, mainRef }, env)).trim();
  } catch (error) {
    return report(io, "release bundle", describe(error), EXIT_FAILURE);
  }
  if (tag !== expected) {
    return report(
      io,
      prefix,
      `tag ${tag} does not match version-at-commit ${expected}`,
      EXIT_FAILURE,
    );
  }

  try {
    await writeBundle(repo, sha, tag, out, env);
  } catch (error) {
    return report(io, prefix, describe(error), EXIT_FAILURE);
  }
  return EXIT_OK;
};
