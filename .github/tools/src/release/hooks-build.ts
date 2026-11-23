import { createHash } from "node:crypto";
import {
  closeSync,
  constants,
  fstatSync,
  lstatSync,
  mkdirSync,
  mkdtempSync,
  openSync,
  readdirSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { basename, dirname, isAbsolute, join } from "node:path";
import { HOOK_BINARIES } from "./assets.ts";
import {
  describeExit,
  git,
  gitBuffer,
  runCommand,
  succeeded,
} from "./github.ts";
import type { Env } from "./io.ts";
import { extractTar } from "./tar.ts";

interface Completed {
  // stdout and stderr in arrival order.
  output: string;
  // The failure sentence, or undefined when the command exited 0.
  failure: string | undefined;
}

async function runCombined(
  command: string,
  args: readonly string[],
  env: Env,
  cwd?: string,
): Promise<Completed> {
  // A program run by its path is named by its file name, never its directory or arguments.
  const result = await runCommand(command, args, {
    env,
    cwd,
    name: basename(command),
  });
  return {
    output: result.output.toString("utf8"),
    failure: succeeded(result) ? undefined : describeExit(result),
  };
}

const LAUNCHER_PATH = "bin/agx.exe";
const MANIFEST_PATH = "hooks/SHA256SUMS";
const runtimePaths = HOOK_BINARIES.map((name) => `hooks/bin/${name}`);

// An absent optional executable has no entry in a successful tree listing. A link,
// submodule or directory at the same path is a different input and must be refused.
async function selectedBlobs(
  repo: string,
  sha: string,
  env: Env,
): Promise<Map<string, string>> {
  const listing = await git(
    repo,
    ["ls-tree", "-rtz", sha, "--", "bin", "hooks"],
    { env },
  );
  const blobs = new Map<string, string>();
  for (const entry of listing.split("\0").filter(Boolean)) {
    const tab = entry.indexOf("\t");
    const [mode, , oid] = entry.slice(0, tab).split(" ");
    const path = entry.slice(tab + 1);
    if (["bin", "hooks", "hooks/bin", "hooks/go"].includes(path)) {
      if (mode !== "040000") throw new Error(`expected a Git tree at ${path}`);
      continue;
    }
    if (path.startsWith("hooks/go/")) {
      if (!["100644", "100755", "040000"].includes(mode ?? ""))
        throw new Error(`unexpected Git mode ${mode} at ${path}`);
      continue;
    }
    if (
      path !== MANIFEST_PATH &&
      !path.startsWith("hooks/bin/") &&
      !path.startsWith("bin/")
    )
      continue;
    if (
      path !== MANIFEST_PATH &&
      path !== "bin/agx" &&
      path !== LAUNCHER_PATH &&
      !runtimePaths.includes(path)
    )
      throw new Error(
        `unexpected executable path in the selected tree: ${path}`,
      );
    if ((mode !== "100644" && mode !== "100755") || !oid)
      throw new Error(`expected a regular Git blob at ${path}`);
    blobs.set(path, oid);
  }
  return blobs;
}

// Writes the six verified runtimes to out and returns the launcher for the installable archive.
// Tracked executables, when present at sha, must match the independently built bytes.
export async function buildCommittedHooks(
  repo: string,
  sha: string,
  out: string,
  env: Env,
): Promise<Buffer> {
  const src = mkdtempSync(join(tmpdir(), "agentics-release-hooks."));
  try {
    return await buildInto(src, repo, sha, out, env);
  } finally {
    rmSync(src, { recursive: true, force: true });
  }
}

async function buildInto(
  src: string,
  repo: string,
  sha: string,
  out: string,
  env: Env,
): Promise<Buffer> {
  const blobs = await selectedBlobs(repo, sha, env);
  const manifestOid = blobs.get(MANIFEST_PATH);
  if (manifestOid === undefined)
    throw new Error(`missing ${MANIFEST_PATH} in the selected tree`);
  const tarPath = join(src, "src.tar");
  // A CRLF export would change the source digest, the stamp, and the bytes (a Windows host has
  // core.autocrlf=true and .gitattributes `* text=auto`), so the archive is pinned to no conversion
  // at all.
  await git(
    repo,
    [
      "-c",
      "core.autocrlf=false",
      "-c",
      "core.eol=lf",
      "archive",
      "--format=tar",
      `--output=${tarPath}`,
      sha,
      "hooks/go",
    ],
    { env },
  );

  extractTar(tarPath, src);

  const verifier = env.AGX_HOOK_VERIFIER ?? "";
  if (!isAbsolute(verifier))
    throw new Error("AGX_HOOK_VERIFIER must name an absolute trusted verifier");
  const buildOut = mkdtempSync(join(src, "build-"));
  const manifestPath = join(src, "hooks", "SHA256SUMS");
  writeFileSync(
    manifestPath,
    await gitBuffer(repo, ["cat-file", "blob", manifestOid], { env }),
  );
  const launcherOid = blobs.get(LAUNCHER_PATH);
  const launcherReference =
    launcherOid === undefined
      ? undefined
      : await gitBuffer(repo, ["cat-file", "blob", launcherOid], { env });
  let launcherArgs = ["--build-launcher"];
  if (launcherReference !== undefined) {
    const referencePath = join(src, LAUNCHER_PATH);
    mkdirSync(dirname(referencePath));
    writeFileSync(referencePath, launcherReference);
    launcherArgs = ["--launcher", referencePath];
  }
  const built = await runCombined(
    verifier,
    [
      "build",
      "--source",
      join(src, "hooks", "go"),
      "--manifest",
      manifestPath,
      "--out",
      buildOut,
      ...launcherArgs,
    ],
    env,
  );
  if (built.failure !== undefined) {
    const diagnostic = built.output.replaceAll(
      "but go.mod pins ",
      "but hooks/go/go.mod pins ",
    );
    throw new Error(`trusted hook verifier: ${built.failure}: ${diagnostic}`);
  }

  const manifest = readHookManifest(manifestPath);
  const runtimes = new Map<string, Buffer>();
  for (const name of HOOK_BINARIES) {
    const want = manifest.get(`bin/${name}`);
    if (want === undefined)
      throw new Error(`hooks/SHA256SUMS has no entry for bin/${name}`);
    const bytes = readBuiltFile(buildOut, name);
    const got = createHash("sha256").update(bytes).digest("hex");
    if (got !== want)
      throw new Error(
        `${name} built as sha256 ${got}, but hooks/SHA256SUMS records ${want}`,
      );
    const path = `hooks/bin/${name}`;
    const oid = blobs.get(path);
    if (
      oid !== undefined &&
      !bytes.equals(await gitBuffer(repo, ["cat-file", "blob", oid], { env }))
    )
      throw new Error(
        `${name} built from hooks/go does not match the committed ${path}`,
      );
    runtimes.set(name, bytes);
  }
  const rebuilt = readBuiltFile(buildOut, LAUNCHER_PATH);
  if (launcherReference !== undefined && !rebuilt.equals(launcherReference))
    throw new Error(
      `${LAUNCHER_PATH} built from hooks/go does not match the committed ${LAUNCHER_PATH}`,
    );
  if (
    readdirSync(buildOut).length !== HOOK_BINARIES.length + 1 ||
    readdirSync(join(buildOut, "bin")).length !== 1
  )
    throw new Error("unexpected file in trusted verifier output");
  const pin = pinnedHookToolchain(join(src, "hooks", "go"));
  await checkBuildToolchain(
    join(buildOut, HOOK_BINARIES[0] as string),
    pin,
    env,
  );
  for (const [name, bytes] of runtimes) {
    writeFileSync(join(out, name), bytes, {
      mode: 0o644,
    });
  }
  return rebuilt;
}

function readBuiltFile(dir: string, name: string): Buffer {
  const path = join(dir, name);
  let fd: number | undefined;
  try {
    if (!lstatSync(dirname(path)).isDirectory())
      throw new Error("expected a regular file");
    fd = openSync(
      path,
      constants.O_RDONLY |
        (constants.O_NOFOLLOW ?? 0) |
        (constants.O_NONBLOCK ?? 0),
    );
    // Windows lacks O_NOFOLLOW. Require the name to identify the opened regular file.
    const opened = fstatSync(fd, { bigint: true });
    const named = lstatSync(path, { bigint: true });
    if (
      !opened.isFile() ||
      !named.isFile() ||
      opened.dev !== named.dev ||
      opened.ino !== named.ino
    )
      throw new Error("expected a regular file");
    return readFileSync(fd);
  } catch (error) {
    throw new Error(
      `reading built ${name}: ${error instanceof Error ? error.message : String(error)}`,
    );
  } finally {
    if (fd !== undefined) closeSync(fd);
  }
}

// Returns "go" plus the first `go X.Y[.Z]` directive of the module's go.mod, for example "go1.27.3".
// The pattern is anchored on "\n" and the end of the text, not on every line terminator JavaScript
// knows, so a CRLF go.mod has no directive.
export function pinnedHookToolchain(source: string): string {
  let data: string;
  try {
    data = readFileSync(join(source, "go.mod"), "utf8");
  } catch (error) {
    throw new Error(
      `reading hooks/go/go.mod for the toolchain pin: ${error instanceof Error ? error.message : String(error)}`,
    );
  }
  const match = /(?:^|\n)go[ \t]+([0-9]+(?:\.[0-9]+){1,2})[ \t]*(?=\n|$)/.exec(
    data,
  );
  if (match === null) {
    throw new Error(
      "hooks/go/go.mod carries no `go <version>` directive — the toolchain check has nothing to compare against",
    );
  }
  return `go${match[1]}`;
}

// Reads hooks/SHA256SUMS into a map from the recorded path to its lowercase digest. The first
// entry for a path wins.
export function readHookManifest(path: string): Map<string, string> {
  let data: string;
  try {
    data = readFileSync(path, "utf8");
  } catch (error) {
    throw new Error(
      `reading the checksum manifest: ${error instanceof Error ? error.message : String(error)}`,
    );
  }
  const entries = new Map<string, string>();
  const lines = data.split("\n");
  for (const [index, line] of lines.entries()) {
    if (line.includes("\r"))
      throw new Error(
        `the checksum manifest line ${index + 1} carries a carriage return`,
      );
    const fields = line.split(/[ \t]+/).filter((field) => field !== "");
    const [digest, name] = fields;
    if (digest !== undefined && name !== undefined && !entries.has(name))
      entries.set(name, digest.toLowerCase());
  }
  if (entries.size === 0)
    throw new Error("the checksum manifest has no entries");
  return entries;
}

// Requires the Go version `go version <binary>` reports to equal the go.mod pin, exactly or as the
// pin followed by ".". Node has no equivalent of debug/buildinfo, and the bundle job always has Go.
export async function checkBuildToolchain(
  binary: string,
  pin: string,
  env: Env,
): Promise<void> {
  const name = basename(binary);
  // `go version` prints "<binary>: go1.27.3", with " X:<experiment>" appended for an experiment
  // build. It runs beside the binary so no go.mod in the caller's directory can select a toolchain.
  const result = await runCombined(
    "go",
    ["version", binary],
    env,
    dirname(binary),
  );
  const line = result.output.trim();
  if (result.failure !== undefined) {
    throw new Error(
      `reading the Go version of ${name}: ${[result.failure, line].filter((part) => part !== "").join(": ")}`,
    );
  }
  const separator = line.lastIndexOf(": ");
  const version =
    separator < 0 ? "" : (line.slice(separator + 2).split(/\s+/)[0] ?? "");
  if (version === "")
    throw new Error(
      `reading the Go version of ${name}: unexpected output from go version: ${line}`,
    );
  if (version !== pin && !version.startsWith(`${pin}.`)) {
    throw new Error(
      `the hook binaries were built with ${version}, but hooks/go/go.mod pins ${pin}; build with that toolchain`,
    );
  }
}
