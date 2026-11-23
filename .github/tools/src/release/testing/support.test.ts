import assert from "node:assert/strict";
import {
  existsSync,
  readdirSync,
  readFileSync,
  statSync,
  writeFileSync,
} from "node:fs";
import { request } from "node:http";
import { EOL } from "node:os";
import { join, relative } from "node:path";
import { test } from "node:test";
import type { ParserOptions } from "prettier";
import { parsers } from "prettier/plugins/typescript";
import {
  captureIo,
  commandAvailable,
  commitFiles,
  initRepo,
  MANIFEST_PATHS,
  platformSkip,
  prerequisite,
  readOutputs,
  repoRoot,
  scratchDir,
  sh,
  startServer,
  tempGitRepo,
  writeManifests,
} from "./support.ts";

function withCi<T>(value: string | undefined, body: () => T): T {
  const saved = process.env.CI;
  if (value === undefined) delete process.env.CI;
  else process.env.CI = value;
  try {
    return body();
  } finally {
    if (saved === undefined) delete process.env.CI;
    else process.env.CI = saved;
  }
}

test("prerequisite lets a test run when the prerequisite is available", () => {
  assert.equal(
    withCi("true", () => prerequisite("bash", true, "install bash")),
    false,
  );
  assert.equal(
    withCi(undefined, () => prerequisite("bash", true, "install bash")),
    false,
  );
});

test("prerequisite returns a skip reason outside CI", () => {
  const reason = withCi(undefined, () =>
    prerequisite("go", false, "install Go 1.27.4"),
  );
  assert.equal(typeof reason, "string");
  assert.match(String(reason), /go/);
  assert.match(String(reason), /install Go 1\.27\.4/);
});

test("prerequisite fails under CI=true instead of skipping", () => {
  assert.throws(
    () => withCi("true", () => prerequisite("go", false, "install Go 1.27.4")),
    /go is required when CI=true: install Go 1\.27\.4/,
  );
});

test("platformSkip lets a test run on a supported platform under CI=true", () => {
  assert.equal(
    withCi("true", () => platformSkip("bash", true, "needs a POSIX host")),
    false,
  );
});

test("platformSkip lets a test run on a supported platform outside CI", () => {
  assert.equal(
    withCi(undefined, () => platformSkip("bash", true, "needs a POSIX host")),
    false,
  );
});

test("platformSkip returns a skip reason on an unsupported platform outside CI", () => {
  assert.equal(
    withCi(undefined, () => platformSkip("bash", false, "needs a POSIX host")),
    "bash is not available: needs a POSIX host",
  );
});

test("platformSkip skips instead of throwing on an unsupported platform under CI=true", () => {
  let reason: string | false = false;
  assert.doesNotThrow(() => {
    reason = withCi("true", () =>
      platformSkip("bash", false, "needs a POSIX host"),
    );
  });
  assert.equal(reason, "bash is not available: needs a POSIX host");
});

function isAstNode(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && "type" in value;
}

function isNamedIdentifier(node: unknown, name: string): boolean {
  if (!isAstNode(node)) return false;
  switch (node.type) {
    case "Identifier":
      return node.name === name;
    case "TSAsExpression":
    case "TSInstantiationExpression":
    case "TSNonNullExpression":
    case "TSSatisfiesExpression":
    case "TSTypeAssertion":
      return isNamedIdentifier(node.expression, name);
    default:
      return false;
  }
}

function containsPosixPrerequisite(node: unknown): boolean {
  if (Array.isArray(node)) return node.some(containsPosixPrerequisite);
  if (!isAstNode(node)) return false;
  if (
    node.type === "CallExpression" &&
    isNamedIdentifier(node.callee, "prerequisite") &&
    Array.isArray(node.arguments) &&
    isNamedIdentifier(node.arguments[1], "posix")
  ) {
    return true;
  }
  return Object.values(node).some(containsPosixPrerequisite);
}

async function passesPosixToPrerequisite(
  source: string,
  filepath = "fixture.ts",
): Promise<boolean> {
  // Only the path is needed to select TypeScript syntax for the AST.
  const ast: unknown = await parsers.typescript.parse(source, {
    filepath,
  } as ParserOptions);
  return containsPosixPrerequisite(ast);
}

test("the posix source guard checks call arguments across syntax variations", async () => {
  const cases = [
    ['prerequisite(label("bash", "tool"), posix, "hint");', true],
    [
      'prerequisite /* comment */ ("bash", /* comment */\n(posix), "hint");',
      true,
    ],
    ['(prerequisite)?.<boolean>("bash", posix as boolean, "hint");', true],
    ['prerequisite(`${label("bash", "tool")}`, posix, "hint");', true],
    ['platformSkip("bash", posix, "hint");', false],
    ['prerequisite(label(posix), installed, "hint");', false],
    ['// prerequisite("bash", posix, "hint");', false],
    ['const example = `prerequisite("bash", posix, "hint");`;', false],
  ] as const;
  for (const [source, expected] of cases) {
    assert.equal(await passesPosixToPrerequisite(source), expected, source);
  }
});

test("no release test passes posix to prerequisite", async () => {
  const releaseDir = join(repoRoot, ".github", "tools", "src", "release");
  const offenders: string[] = [];
  for (const dir of [releaseDir, join(releaseDir, "commands")]) {
    for (const name of readdirSync(dir)) {
      if (!name.endsWith(".test.ts")) continue;
      const path = join(dir, name);
      if (await passesPosixToPrerequisite(readFileSync(path, "utf8"), path)) {
        offenders.push(relative(repoRoot, path).replaceAll("\\", "/"));
      }
    }
  }
  assert.deepEqual(
    offenders,
    [],
    `prerequisite(...) throws under CI=true when its availability is false, so it must not take posix, which is false on Windows. Use platformSkip for a platform gap in: ${offenders.join(", ")}`,
  );
});

test("commandAvailable finds git and rejects a made-up command", () => {
  assert.equal(commandAvailable("git"), true);
  assert.equal(commandAvailable("agentics-no-such-command-9f3a"), false);
});

test("repoRoot is the checkout that holds the manifests", () => {
  assert.equal(
    existsSync(join(repoRoot, ".claude-plugin", "plugin.json")),
    true,
  );
});

test("scratchDir returns a new existing directory with the prefix", () => {
  const first = scratchDir("agentics-support-");
  const second = scratchDir("agentics-support-");
  assert.notEqual(first, second);
  assert.match(first, /agentics-support-/);
  assert.equal(statSync(first).isDirectory(), true);
});

test("captureIo keeps the two streams apart", () => {
  const captured = captureIo();
  captured.io.stdout.write("a");
  captured.io.stderr.write("b");
  captured.io.stdout.write("c");
  assert.equal(captured.stdout(), "ac");
  assert.equal(captured.stderr(), "b");
});

test("sh returns stdout and throws with the command and stderr on failure", () => {
  const dir = scratchDir("agentics-support-");
  assert.equal(sh(dir, "git", "--version").startsWith("git version"), true);
  assert.throws(
    () => sh(dir, "git", "cat-file", "-t", "0".repeat(40)),
    /git cat-file -t 0{40} failed in .*(not a git repository|Not a valid object name)/s,
  );
});

test("initRepo pins signing, line endings and hooks", () => {
  const dir = scratchDir("agentics-support-");
  initRepo(dir);
  const config = (key: string): string => sh(dir, "git", "config", key).trim();
  assert.equal(config("commit.gpgsign"), "false");
  assert.equal(config("core.autocrlf"), "false");
  assert.equal(config("user.name"), "Agentics Test");
  assert.equal(
    sh(dir, "git", "symbolic-ref", "HEAD").trim(),
    "refs/heads/main",
  );
});

test("commitFiles commits text and bytes and returns the commit SHA", () => {
  const dir = scratchDir("agentics-support-");
  initRepo(dir);
  const sha = commitFiles(
    dir,
    { "a/b.txt": "line\n", "bin.dat": Buffer.from([0, 255, 10]) },
    "first",
  );
  assert.match(sha, /^[0-9a-f]{40}$/);
  assert.equal(sh(dir, "git", "rev-parse", "HEAD").trim(), sha);
  assert.equal(sh(dir, "git", "show", `${sha}:a/b.txt`), "line\n");
  assert.deepEqual(
    readFileSync(join(dir, "bin.dat")),
    Buffer.from([0, 255, 10]),
  );
  const empty = commitFiles(dir, {}, "empty");
  assert.notEqual(empty, sha);
  assert.equal(
    sh(dir, "git", "log", "-1", "--format=%s", empty).trim(),
    "empty",
  );
});

test("writeManifests writes the five valid manifests with LF and two spaces", () => {
  const dir = scratchDir("agentics-support-");
  writeManifests(dir, "1.2.3");
  assert.deepEqual(MANIFEST_PATHS, [
    ".claude-plugin/marketplace.json",
    ".claude-plugin/plugin.json",
    ".codex-plugin/plugin.json",
    ".github/plugin/plugin.json",
    ".antigravity-plugin/plugin.json",
  ]);
  for (const path of MANIFEST_PATHS) {
    const text = readFileSync(join(dir, path), "utf8");
    assert.equal(text.includes("\r"), false, path);
    assert.equal(text.endsWith("}\n"), true, path);
    assert.match(text, /\n {2}"(name|plugins)"/, path);
    JSON.parse(text);
  }
  const marketplace = JSON.parse(
    readFileSync(join(dir, MANIFEST_PATHS[0] ?? ""), "utf8"),
  ) as { metadata: { version: string }; plugins: { version: string }[] };
  assert.equal(marketplace.metadata.version, "0.0.0");
  assert.equal(marketplace.plugins[0]?.version, "1.2.3");
});

test("startServer records every request with its query and body", async () => {
  const server = await startServer((req, body, res) => {
    res.setHeader("content-type", "application/json");
    res.end(JSON.stringify({ method: req.method, bytes: body.length }));
  });
  try {
    const send = (method: string, path: string, body?: string) =>
      new Promise<string>((resolve, reject) => {
        const req = request(server.url + path, { method }, (res) => {
          const chunks: Buffer[] = [];
          res.on("data", (chunk: Buffer) => chunks.push(chunk));
          res.on("end", () => resolve(Buffer.concat(chunks).toString()));
        });
        req.on("error", reject);
        req.end(body);
      });
    assert.equal(
      await send("GET", "/repos/muehz/agentics/releases"),
      '{"method":"GET","bytes":0}',
    );
    assert.equal(
      await send(
        "POST",
        "/repos/muehz/agentics/releases?name=SHA256SUMS",
        "xyz",
      ),
      '{"method":"POST","bytes":3}',
    );
    assert.deepEqual(
      server.requests.map(({ method, url }) => ({ method, url })),
      [
        { method: "GET", url: "/repos/muehz/agentics/releases" },
        {
          method: "POST",
          url: "/repos/muehz/agentics/releases?name=SHA256SUMS",
        },
      ],
    );
    assert.equal(server.requests[1]?.body.toString(), "xyz");
    assert.equal(typeof server.requests[0]?.headers.host, "string");
  } finally {
    await server.close();
  }
});

test("startServer answers 500 when the handler throws", async () => {
  const server = await startServer(() => {
    throw new Error("boom");
  });
  try {
    const response = await fetch(`${server.url}/x`);
    assert.equal(response.status, 500);
    assert.match(await response.text(), /boom/);
    assert.equal(server.requests.length, 1);
  } finally {
    await server.close();
  }
});

test("readOutputs parses the file commands the runner reads", () => {
  const dir = scratchDir("agentics-support-");
  const path = join(dir, "output");
  const d = "_GitHubActionsFileCommandDelimeter_";
  writeFileSync(
    path,
    `mode<<${d}${EOL}resume${EOL}${d}${EOL}` +
      `resume_tag<<${d}${EOL}${EOL}${d}${EOL}` +
      `notes<<${d}${EOL}a\nb${EOL}${d}${EOL}` +
      `mode<<${d}${EOL}released${EOL}${d}${EOL}`,
  );
  assert.deepEqual(readOutputs(path), {
    mode: "released",
    resume_tag: "",
    notes: "a\nb",
  });
  writeFileSync(path, "mode=analyze\n");
  assert.throws(() => readOutputs(path), /malformed output file/);
  writeFileSync(path, `mode<<${d}${EOL}resume${EOL}`);
  assert.throws(() => readOutputs(path), /unterminated output/);
  writeFileSync(path, "");
  assert.deepEqual(readOutputs(path), {});
});

test("tempGitRepo is a repository at v1.2.3 with the manifests and the notices committed", () => {
  const { root, sha } = tempGitRepo();
  assert.match(sha, /^[0-9a-f]{40}$/);
  assert.equal(sh(root, "git", "rev-parse", "HEAD").trim(), sha);
  for (const path of MANIFEST_PATHS) {
    assert.match(
      sh(root, "git", "show", `${sha}:${path}`),
      /"version": "1\.2\.3"/,
    );
  }
  assert.equal(
    sh(root, "git", "show", `${sha}:THIRD_PARTY_NOTICES.md`),
    "# Third party notices\n\nfixture\n",
  );
  assert.equal(sh(root, "git", "status", "--porcelain"), "");
  assert.notEqual(tempGitRepo().root, root);
});
