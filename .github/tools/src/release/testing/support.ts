import { execFileSync } from "node:child_process";
import {
  existsSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import {
  createServer,
  type IncomingMessage,
  type ServerResponse,
} from "node:http";
import type { AddressInfo } from "node:net";
import { EOL, tmpdir } from "node:os";
import { delimiter, dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { fixtureGitEnv } from "../../conformance/testing/git-checkout.ts";
import { VERSIONED_PLUGIN_MANIFESTS } from "../../plugin-manifests.ts";
import type { Io } from "../io.ts";

export const posix = process.platform !== "win32";

// The checkout these tests run from: .github/tools/src/release/testing is four levels
// below it.
export const repoRoot = fileURLToPath(
  new URL("../../../../../", import.meta.url),
);

// Whether an executable of this name is on PATH.
export function commandAvailable(name: string): boolean {
  const extensions = posix
    ? [""]
    : ["", ...(process.env.PATHEXT ?? ".EXE;.CMD").split(";")];
  for (const dir of (process.env.PATH ?? "").split(delimiter)) {
    for (const extension of extensions) {
      if (dir !== "" && existsSync(join(dir, name + extension))) return true;
    }
  }
  return false;
}

const scratch: string[] = [];
process.on("exit", () => {
  for (const dir of scratch) rmSync(dir, { recursive: true, force: true });
});

// A fresh directory under the system temp directory, removed when the process
// exits.
export function scratchDir(prefix: string): string {
  const dir = mkdtempSync(join(tmpdir(), prefix));
  scratch.push(dir);
  return dir;
}

// Checks for a tool or dependency the test runner is expected to provide.
// Pass the result as the test's `skip` option. False runs the test and a string
// gives the skip reason. Under CI=true a missing prerequisite throws.
export function prerequisite(
  name: string,
  available: boolean,
  hint: string,
): string | false {
  if (available) return false;
  if (process.env.CI === "true") {
    throw new Error(`${name} is required when CI=true: ${hint}`);
  }
  return `${name} is not available: ${hint}`;
}

// Checks whether the current platform can run the test. Pass the result as
// the test's `skip` option. False runs the test and a string gives the skip
// reason. An unsupported platform skips even under CI=true.
export function platformSkip(
  name: string,
  supported: boolean,
  hint: string,
): string | false {
  if (supported) return false;
  return `${name} is not available: ${hint}`;
}

export function captureIo(): {
  io: Io;
  stdout(): string;
  stderr(): string;
} {
  let out = "";
  let err = "";
  return {
    io: {
      stdout: {
        write(chunk: string) {
          out += chunk;
          return true;
        },
      },
      stderr: {
        write(chunk: string) {
          err += chunk;
          return true;
        },
      },
    },
    stdout: () => out,
    stderr: () => err,
  };
}

// Runs a command in cwd and returns its standard output. A nonzero exit throws
// an error whose message carries the command and its standard error.
export function sh(cwd: string, command: string, ...args: string[]): string {
  try {
    return execFileSync(command, args, {
      cwd,
      env: command === "git" ? fixtureGitEnv() : undefined,
      encoding: "utf8",
      stdio: ["ignore", "pipe", "pipe"],
    });
  } catch (error) {
    const stderr = (error as { stderr?: unknown }).stderr;
    throw new Error(
      `${command} ${args.join(" ")} failed in ${cwd}: ${String(stderr ?? error)}`,
    );
  }
}

// A repository on branch main that signs nothing, converts no line endings and
// runs no hooks, whatever the host's git configuration says. A Windows
// checkout usually carries core.autocrlf=true, and git archive applies it, so
// the release fixtures pin the Linux behavior the release runs under.
export function initRepo(dir: string): void {
  mkdirSync(dir, { recursive: true });
  sh(dir, "git", "init", "--quiet", "-b", "main");
  sh(dir, "git", "config", "user.name", "Agentics Test");
  sh(dir, "git", "config", "user.email", "test@example.com");
  sh(dir, "git", "config", "commit.gpgsign", "false");
  sh(dir, "git", "config", "tag.gpgsign", "false");
  sh(dir, "git", "config", "core.autocrlf", "false");
  sh(dir, "git", "config", "core.hooksPath", join(dir, ".git", "no-hooks"));
}

// Writes the files, commits everything (an empty commit when there is nothing
// to stage) and returns the new commit's SHA.
export function commitFiles(
  dir: string,
  files: Readonly<Record<string, string | Buffer>>,
  message: string,
): string {
  for (const [path, contents] of Object.entries(files)) {
    const full = join(dir, path);
    mkdirSync(dirname(full), { recursive: true });
    writeFileSync(full, contents);
  }
  sh(dir, "git", "add", "-A");
  sh(dir, "git", "commit", "--quiet", "--allow-empty", "-m", message);
  return sh(dir, "git", "rev-parse", "HEAD").trim();
}

export const MANIFEST_PATHS: readonly string[] = VERSIONED_PLUGIN_MANIFESTS.map(
  ({ path }) => path,
);

// The text of each of the five manifests at the given Agentics version. They are
// valid JSON with two-space indentation and LF endings; the marketplace
// carries a metadata.version of 0.0.0, which is not the Agentics version.
export function manifestFiles(version: string): Record<string, string> {
  const plugin = `${JSON.stringify({ name: "agentics", version }, null, 2)}\n`;
  const marketplace = `${JSON.stringify(
    {
      name: "muehz",
      metadata: { version: "0.0.0" },
      plugins: [{ name: "agentics", source: "./", version }],
    },
    null,
    2,
  )}\n`;
  return Object.fromEntries(
    VERSIONED_PLUGIN_MANIFESTS.map(({ path, kind }) => [
      path,
      kind === "marketplace" ? marketplace : plugin,
    ]),
  );
}

export function writeManifests(dir: string, version: string): void {
  for (const [path, contents] of Object.entries(manifestFiles(version))) {
    const full = join(dir, path);
    mkdirSync(dirname(full), { recursive: true });
    writeFileSync(full, contents);
  }
}

// A repository whose one commit carries the five manifests at 1.2.3 and a
// THIRD_PARTY_NOTICES.md, and that commit's SHA, so the release tag for it is
// v1.2.3. The version is a fixed literal on purpose: a fixture that copied the
// checkout's own manifests would change its answer at every version bump. The
// bundle tests add the hooks module on top.
export function tempGitRepo(): { root: string; sha: string } {
  const root = scratchDir("agentics-repo-");
  initRepo(root);
  writeManifests(root, "1.2.3");
  const sha = commitFiles(
    root,
    { "THIRD_PARTY_NOTICES.md": "# Third party notices\n\nfixture\n" },
    "initial",
  );
  return { root, sha };
}

const OUTPUT_DELIMITER = "_GitHubActionsFileCommandDelimeter_";

// Reads a GITHUB_OUTPUT file the way the runner does: each output is
// `key<<DELIMITER`, the value, then the delimiter, every line ended with the
// platform's line break. A later write of the same key wins.
export function readOutputs(path: string): Record<string, string> {
  const open = `<<${OUTPUT_DELIMITER}${EOL}`;
  const close = `${EOL}${OUTPUT_DELIMITER}${EOL}`;
  const outputs: Record<string, string> = {};
  let rest = readFileSync(path, "utf8");
  while (rest !== "") {
    const header = rest.indexOf(open);
    if (header < 0) throw new Error(`malformed output file: ${rest}`);
    const start = header + open.length;
    const end = rest.indexOf(close, start);
    if (end < 0) throw new Error(`unterminated output: ${rest}`);
    outputs[rest.slice(0, header)] = rest.slice(start, end);
    rest = rest.slice(end + close.length);
  }
  return outputs;
}

export interface RecordedRequest {
  method: string;
  url: string;
  headers: Record<string, string | string[] | undefined>;
  body: Buffer;
}

// A local HTTP server on an ephemeral port. Every request is recorded, with
// its body read in full, before the handler answers it. A handler that throws
// answers 500.
export async function startServer(
  handler: (
    req: IncomingMessage,
    body: Buffer,
    res: ServerResponse,
  ) => void | Promise<void>,
): Promise<{
  url: string;
  requests: RecordedRequest[];
  close(): Promise<void>;
}> {
  const requests: RecordedRequest[] = [];
  const server = createServer((req, res) => {
    const chunks: Buffer[] = [];
    req.on("data", (chunk: Buffer) => chunks.push(chunk));
    req.on("end", () => {
      const body = Buffer.concat(chunks);
      requests.push({
        method: req.method ?? "",
        url: req.url ?? "",
        headers: { ...req.headers },
        body,
      });
      Promise.resolve()
        .then(() => handler(req, body, res))
        .catch((error: unknown) => {
          res.statusCode = 500;
          res.end(String(error));
        });
    });
  });
  await new Promise<void>((resolve) =>
    server.listen(0, "127.0.0.1", () => resolve()),
  );
  const { port } = server.address() as AddressInfo;
  return {
    url: `http://127.0.0.1:${port}`,
    requests,
    close: () =>
      new Promise<void>((resolve) => {
        server.close(() => resolve());
        server.closeAllConnections();
      }),
  };
}
