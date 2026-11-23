import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { spawnSync } from "node:child_process";
import test, { type TestContext } from "node:test";
import {
  fixtureGitEnv,
  git as fixtureGit,
} from "../conformance/testing/git-checkout.ts";
import { CI_ASSET_PLUGIN_MANIFEST_PATHS } from "../plugin-manifests.ts";
import {
  encodeSelection,
  fullSelection,
  selectPaths,
  validateSelection,
  type Selection as ScopeResult,
} from "./ci-areas.ts";
interface Fixture {
  repo: string;
  event: string;
  base: string;
  head: string;
  merge: string;
}
interface CommandOptions {
  cwd?: string;
  env?: NodeJS.ProcessEnv;
  encoding?: BufferEncoding;
  input?: string | Buffer;
}

const ROOT = path.resolve(import.meta.dirname, "../../../..");
const SCRIPT = path.join(import.meta.dirname, "ci-scope.ts");
const ZERO = "0".repeat(40);
const MANIFESTS = [
  ".agents/plugins/marketplace.json",
  ".antigravity-plugin/plugin.json",
  ".claude-plugin/marketplace.json",
  ".claude-plugin/plugin.json",
  ".codex-plugin/plugin.json",
  ".github/plugin/plugin.json",
];
const ROOT_PROSE = [
  "README.md",
  "AGENTS.md",
  "CONTRIBUTING.md",
  "SECURITY.md",
  "THIRD_PARTY_NOTICES.md",
];

function command(
  executable: string,
  args: string[],
  options: CommandOptions = {},
) {
  return spawnSync(executable, args, {
    cwd: options.cwd || ROOT,
    env: { ...fixtureGitEnv(), ...options.env },
    encoding: options.encoding || "utf8",
    shell: false,
    windowsHide: true,
    maxBuffer: 8 * 1024 * 1024,
    ...(options.input === undefined ? {} : { input: options.input }),
  });
}

function makeGitShim(
  t: TestContext,
  simulatedPlatform: string = process.platform,
) {
  const lookup = command(process.platform === "win32" ? "where.exe" : "which", [
    "git",
  ]);
  assert.equal(lookup.status, 0, lookup.stderr);
  const realGit = String(lookup.stdout).split(/\r?\n/).find(Boolean);
  const dir = fs.mkdtempSync(
    path.join(os.tmpdir(), "agentics-ci-scope-git-shim-"),
  );
  const source = path.join(
    dir,
    simulatedPlatform === "win32" ? "shim.go" : "git-shim.cjs",
  );
  const binary = path.join(
    dir,
    simulatedPlatform === "win32" ? "git.exe" : "git",
  );
  const validOid = "1".repeat(40);
  const otherOid = "2".repeat(40);
  const validRecord = `:100644 100644 ${validOid} ${otherOid} M\\x00docs/file.md\\x00`;
  const goLiteral = (value: string) =>
    JSON.stringify(value).replaceAll("\\\\x00", "\\x00");
  if (simulatedPlatform === "win32") {
    const sourceText = [
      "package main",
      "import (",
      ' "fmt"',
      ' "os"',
      ")",
      `const realGit = ${JSON.stringify(realGit)}`,
      "func has(args []string, value string) bool { for _, arg := range args { if arg == value { return true } }; return false }",
      "func main() {",
      " args := os.Args[1:]",
      ' mode := os.Getenv("AGX_SCOPE_SHIM_MODE")',
      ' if has(args, "diff") && has(args, "--raw") {',
      `  switch mode { case "truncated": fmt.Print(${goLiteral(`:100644 100644 ${validOid} ${otherOid} M\\x00docs/file.md`)}); return`,
      `  case "missing-rename-path": fmt.Print(${goLiteral(`:100644 100644 ${validOid} ${otherOid} R100\\x00docs/original.md\\x00`)}); return`,
      `  case "empty-path": fmt.Print(${goLiteral(`:100644 100644 ${validOid} ${otherOid} M\\x00\\x00`)}); return`,
      `  case "invalid-header": fmt.Print(${goLiteral(`bad\\x00docs/file.md\\x00`)}); return`,
      `  case "invalid-oid": fmt.Print(${goLiteral(`:100644 100644 zzz ${otherOid} M\\x00docs/file.md\\x00`)}); return`,
      `  case "invalid-score": fmt.Print(${goLiteral(`:100644 100644 ${validOid} ${otherOid} R101\\x00docs/original.md\\x00docs/renamed.md\\x00`)}); return`,
      `  case "mode-mismatch": fmt.Print(${goLiteral(`:100755 100644 ${validOid} ${otherOid} M\\x00docs/file.md\\x00`)}); return`,
      `  case "status-T": fmt.Print(${goLiteral(`:100644 100644 ${validOid} ${otherOid} T\\x00docs/file.md\\x00`)}); return`,
      `  case "oid-mismatch": fmt.Print(${goLiteral(`:000000 100644 ${validOid} ${otherOid} A\\x00docs/file.md\\x00`)}); return`,
      `  case "invalid-utf8": fmt.Print(${goLiteral(`:100644 100644 ${validOid} ${otherOid} M`)}); os.Stdout.Write([]byte{255, 0}); return`,
      `  case "valid": fmt.Print(${goLiteral(validRecord)}); return }`,
      " }",
      ' if mode == "parent-failure" && has(args, "cat-file") && has(args, "-p") { fmt.Fprintln(os.Stderr, "shim parent failure"); os.Exit(23) }',
      ' if mode == "diff-failure" && has(args, "diff") && has(args, "--raw") { fmt.Fprintln(os.Stderr, "shim diff failure"); os.Exit(23) }',
      " proc, err := os.StartProcess(realGit, append([]string{realGit}, args...), &os.ProcAttr{Files: []*os.File{os.Stdin, os.Stdout, os.Stderr}, Env: os.Environ()}); if err != nil { os.Exit(24) }; state, err := proc.Wait(); if err != nil { os.Exit(24) }; if !state.Success() { os.Exit(state.ExitCode()) }",
      "}",
    ].join("\n");
    fs.writeFileSync(source, sourceText);
    const built = command("go", ["build", "-o", binary, source], { cwd: dir });
    assert.equal(built.status, 0, built.stderr);
  } else {
    const jsLiteral = (value: string) =>
      JSON.stringify(value).replaceAll("\\\\x00", "\\x00");
    const sourceText = `#!/usr/bin/env node
const { spawnSync } = require("node:child_process");
const args = process.argv.slice(2);
const mode = process.env.AGX_SCOPE_SHIM_MODE;
const has = (value) => args.includes(value);
  if (has("diff") && has("--raw")) {
  const outputs = {
    truncated: ${jsLiteral(`:100644 100644 ${validOid} ${otherOid} M\\x00docs/file.md`)},
    "missing-rename-path": ${jsLiteral(`:100644 100644 ${validOid} ${otherOid} R100\\x00docs/original.md\\x00`)},
    "empty-path": ${jsLiteral(`:100644 100644 ${validOid} ${otherOid} M\\x00\\x00`)},
    "invalid-header": ${jsLiteral(`bad\\x00docs/file.md\\x00`)},
    "invalid-oid": ${jsLiteral(`:100644 100644 zzz ${otherOid} M\\x00docs/file.md\\x00`)},
    "invalid-score": ${jsLiteral(`:100644 100644 ${validOid} ${otherOid} R101\\x00docs/original.md\\x00docs/renamed.md\\x00`)},
    "mode-mismatch": ${jsLiteral(`:100755 100644 ${validOid} ${otherOid} M\\x00docs/file.md\\x00`)},
    "status-T": ${jsLiteral(`:100644 100644 ${validOid} ${otherOid} T\\x00docs/file.md\\x00`)},
    "oid-mismatch": ${jsLiteral(`:000000 100644 ${validOid} ${otherOid} A\\x00docs/file.md\\x00`)},
    valid: ${jsLiteral(validRecord)},
  };
  if (mode === "invalid-utf8") {
    process.stdout.write(Buffer.from([...Buffer.from(${jsLiteral(`:100644 100644 ${validOid} ${otherOid} M`)}, "utf8"), 255, 0]));
    process.exit(0);
  }
  if (outputs[mode] !== undefined) {
    process.stdout.write(Buffer.from(outputs[mode], "binary"));
    process.exit(0);
  }
}
if (mode === "parent-failure" && has("cat-file") && has("-p")) { process.stderr.write("shim parent failure\\n"); process.exit(23); }
if (mode === "diff-failure" && has("diff") && has("--raw")) { process.stderr.write("shim diff failure\\n"); process.exit(23); }
const result = spawnSync(${JSON.stringify(realGit)}, args, { encoding: null, shell: false, windowsHide: true });
if (result.stdout) process.stdout.write(result.stdout);
if (result.stderr) process.stderr.write(result.stderr);
process.exit(result.error || result.status === null ? 24 : result.status);
`;
    fs.writeFileSync(source, sourceText, { mode: 0o755 });
    fs.copyFileSync(source, binary);
    fs.chmodSync(binary, 0o755);
  }
  t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
  return {
    env(mode: string) {
      return {
        PATH: `${dir}${path.delimiter}${process.env.PATH || ""}`,
        AGX_SCOPE_SHIM_MODE: mode,
      };
    },
    source,
    binary,
  };
}

function git(
  repo: string,
  args: string[],
  { input }: Pick<CommandOptions, "input"> = {},
): string {
  const identity = [
    "-c",
    "user.name=Agentics test",
    "-c",
    "user.email=agentics-test@example.invalid",
  ];
  if (input === undefined) return fixtureGit(repo, ...identity, ...args).trim();
  const result = command("git", ["-C", repo, ...identity, ...args], { input });
  assert.equal(
    result.status,
    0,
    `git ${args.join(" ")} failed:\n${result.stderr || ""}`,
  );
  return String(result.stdout || "").trim();
}

function write(repo: string, relative: string, contents: string): void {
  const file = path.join(repo, ...relative.split("/"));
  fs.mkdirSync(path.dirname(file), { recursive: true });
  fs.writeFileSync(file, contents);
}

function makeFixture(relative: string, kind = "add"): Fixture {
  const repo = fs.mkdtempSync(path.join(os.tmpdir(), "agentics-ci-scope-"));
  fs.mkdirSync(repo, { recursive: true });
  git(repo, ["init", "--initial-branch=main"]);

  write(repo, "base.txt", "fixture base\n");
  if (kind !== "add") write(repo, relative, "fixture base\n");
  git(repo, ["add", "--all"]);
  git(repo, ["commit", "--message", "base"]);
  const base = git(repo, ["rev-parse", "HEAD"]);

  git(repo, ["switch", "--create", "change"]);
  if (kind === "delete") fs.rmSync(path.join(repo, ...relative.split("/")));
  else if (kind !== "empty") write(repo, relative, `fixture ${kind}\n`);
  git(repo, ["add", "--all"]);
  git(repo, ["commit", "--allow-empty", "--message", "change"]);
  const head = git(repo, ["rev-parse", "HEAD"]);

  git(repo, ["switch", "main"]);
  git(repo, ["merge", "--no-ff", "change", "--message", "merge"]);
  const merge = git(repo, ["rev-parse", "HEAD"]);

  const event = path.join(repo, "event.json");
  fs.writeFileSync(
    event,
    JSON.stringify({
      pull_request: {
        base: { sha: base },
        head: { sha: head },
      },
    }),
  );
  return { repo, event, base, head, merge };
}

function makeMultiFixture(relatives: readonly string[]): Fixture {
  const repo = fs.mkdtempSync(
    path.join(os.tmpdir(), "agentics-ci-scope-multi-"),
  );
  git(repo, ["init", "--initial-branch=main"]);
  write(repo, "base.txt", "fixture base\n");
  git(repo, ["add", "--all"]);
  git(repo, ["commit", "--message", "base"]);
  const base = git(repo, ["rev-parse", "HEAD"]);
  git(repo, ["switch", "--create", "change"]);
  for (const relative of relatives)
    write(repo, relative, `fixture ${relative}\n`);
  git(repo, ["add", "--all"]);
  git(repo, ["commit", "--message", "change"]);
  const head = git(repo, ["rev-parse", "HEAD"]);
  git(repo, ["switch", "main"]);
  git(repo, ["merge", "--no-ff", "change", "--message", "merge"]);
  const merge = git(repo, ["rev-parse", "HEAD"]);
  const event = path.join(repo, "event.json");
  fs.writeFileSync(
    event,
    JSON.stringify({
      pull_request: { base: { sha: base }, head: { sha: head } },
    }),
  );
  return { repo, event, base, head, merge };
}

function makeModeFixture(mode: string): Fixture {
  const repo = fs.mkdtempSync(
    path.join(os.tmpdir(), "agentics-ci-scope-mode-"),
  );
  fs.mkdirSync(repo, { recursive: true });
  git(repo, ["init", "--initial-branch=main"]);
  write(repo, "docs/target.md", "target\n");
  git(repo, ["add", "--all"]);
  git(repo, ["commit", "--message", "base"]);
  const base = git(repo, ["rev-parse", "HEAD"]);
  git(repo, ["switch", "--create", "change"]);
  if (mode === "executable") {
    fs.chmodSync(path.join(repo, "docs", "target.md"), 0o755);
    git(repo, ["update-index", "--chmod=+x", "docs/target.md"]);
  } else {
    write(repo, "symlink-target", "target\n");
    const oid = git(repo, ["hash-object", "-w", "symlink-target"]);
    fs.rmSync(path.join(repo, "docs", "target.md"));
    git(repo, [
      "update-index",
      "--add",
      "--cacheinfo",
      `120000,${oid},docs/target.md`,
    ]);
  }
  git(repo, ["commit", "--message", mode]);
  const head = git(repo, ["rev-parse", "HEAD"]);
  git(repo, ["switch", "main"]);
  git(repo, ["merge", "--no-ff", "change", "--message", "merge"]);
  const merge = git(repo, ["rev-parse", "HEAD"]);
  const event = path.join(repo, "event.json");
  fs.writeFileSync(
    event,
    JSON.stringify({
      pull_request: { base: { sha: base }, head: { sha: head } },
    }),
  );
  return { repo, event, base, head, merge };
}

function makeMergeTreeDiscrepancyFixture() {
  const fixture = makeFixture("README.md", "modify");
  write(fixture.repo, "unknown.fixture", "merge-only\n");
  git(fixture.repo, ["add", "unknown.fixture"]);
  const tree = git(fixture.repo, ["write-tree"]);
  fixture.merge = git(fixture.repo, [
    "commit-tree",
    tree,
    "-p",
    fixture.base,
    "-p",
    fixture.head,
    "-m",
    "discrepant merge",
  ]);
  git(fixture.repo, ["reset", "--hard", fixture.merge]);
  return fixture;
}

function makeIndexPathFixture(relative: string): Fixture {
  const repo = fs.mkdtempSync(
    path.join(os.tmpdir(), "agentics-ci-scope-index-path-"),
  );
  fs.mkdirSync(repo, { recursive: true });
  git(repo, ["init", "--initial-branch=main"]);
  write(repo, "README.md", "fixture base\n");
  git(repo, ["add", "--all"]);
  git(repo, ["commit", "--message", "base"]);
  const base = git(repo, ["rev-parse", "HEAD"]);
  git(repo, ["switch", "--create", "change"]);
  write(repo, "index-content", "hostile path\n");
  const oid = git(repo, ["hash-object", "-w", "index-content"]);
  const parts = relative.split("/");
  let subtree = git(repo, ["mktree", "--missing", "-z"], {
    input: Buffer.concat([
      Buffer.from(`100644 blob ${oid}\t`, "utf8"),
      Buffer.from(parts.at(-1)!, "utf8"),
      Buffer.from([0]),
    ]),
  });
  for (let index = parts.length - 2; index >= 1; index -= 1) {
    subtree = git(repo, ["mktree", "--missing", "-z"], {
      input: Buffer.concat([
        Buffer.from(`040000 tree ${subtree}\t`, "utf8"),
        Buffer.from(parts[index]!, "utf8"),
        Buffer.from([0]),
      ]),
    });
  }
  const readmeOid = git(repo, ["rev-parse", `${base}:README.md`]);
  const treeInput = Buffer.concat([
    Buffer.from(`100644 blob ${readmeOid}\tREADME.md\0`, "utf8"),
    Buffer.concat([
      Buffer.from(`040000 tree ${subtree}\t`, "utf8"),
      Buffer.from(parts[0]!, "utf8"),
      Buffer.from([0]),
    ]),
  ]);
  const tree = git(repo, ["mktree", "--missing", "-z"], { input: treeInput });
  const head = git(repo, [
    "commit-tree",
    tree,
    "-p",
    base,
    "-m",
    "hostile path",
  ]);
  git(repo, ["update-ref", "refs/heads/change", head]);
  git(repo, ["switch", "main"]);
  const merge = git(repo, [
    "commit-tree",
    tree,
    "-p",
    base,
    "-p",
    head,
    "-m",
    "merge",
  ]);
  git(repo, ["update-ref", "refs/heads/main", merge]);
  const event = path.join(repo, "event.json");
  fs.writeFileSync(
    event,
    JSON.stringify({
      pull_request: { base: { sha: base }, head: { sha: head } },
    }),
  );
  return { repo, event, base, head, merge };
}

function makeOneParentMessageSpoofFixture() {
  const repo = fs.mkdtempSync(
    path.join(os.tmpdir(), "agentics-ci-scope-parent-"),
  );
  fs.mkdirSync(repo, { recursive: true });
  git(repo, ["init", "--initial-branch=main"]);

  write(repo, "base.txt", "fixture base\n");
  git(repo, ["add", "--all"]);
  git(repo, ["commit", "--message", "base"]);
  const base = git(repo, ["rev-parse", "HEAD"]);

  git(repo, ["switch", "--create", "change"]);
  write(repo, "docs/branch.md", "branch change\n");
  git(repo, ["add", "--all"]);
  git(repo, ["commit", "--message", "change"]);
  const head = git(repo, ["rev-parse", "HEAD"]);

  git(repo, ["switch", "main"]);
  write(repo, "docs/main.md", "main change\n");
  git(repo, ["add", "--all"]);
  git(repo, ["commit", "--message", `one-parent\n\nparent ${head}`]);
  const merge = git(repo, ["rev-parse", "HEAD"]);

  const event = path.join(repo, "event.json");
  fs.writeFileSync(
    event,
    JSON.stringify({
      pull_request: { base: { sha: base }, head: { sha: head } },
    }),
  );
  return { repo, event, base, head, merge };
}

function makePairFixture(
  kind: string,
  target: string,
  source = "docs/original.md",
): Fixture {
  const repo = fs.mkdtempSync(
    path.join(os.tmpdir(), "agentics-ci-scope-pair-"),
  );
  fs.mkdirSync(repo, { recursive: true });
  git(repo, ["init", "--initial-branch=main"]);

  write(repo, source, "copy source\n");
  write(repo, "base.txt", "fixture base\n");
  git(repo, ["add", "--all"]);
  git(repo, ["commit", "--message", "base"]);
  const base = git(repo, ["rev-parse", "HEAD"]);

  git(repo, ["switch", "--create", "change"]);
  if (kind === "rename") {
    fs.mkdirSync(path.dirname(path.join(repo, ...target.split("/"))), {
      recursive: true,
    });
    git(repo, ["mv", source, target]);
  } else {
    write(repo, target, "copy source\n");
    git(repo, ["add", "--all"]);
  }
  git(repo, ["commit", "--message", kind]);
  const head = git(repo, ["rev-parse", "HEAD"]);

  git(repo, ["switch", "main"]);
  git(repo, ["merge", "--no-ff", "change", "--message", "merge"]);
  const merge = git(repo, ["rev-parse", "HEAD"]);
  const event = path.join(repo, "event.json");
  fs.writeFileSync(
    event,
    JSON.stringify({
      pull_request: { base: { sha: base }, head: { sha: head } },
    }),
  );
  return { repo, event, base, head, merge };
}

// Reads the file in one call instead of checking it first, so nothing can change
// between the check and the read. A missing path or a directory yields "".
function readIfFile(file: string): string {
  try {
    return fs.readFileSync(file, "utf8");
  } catch (error) {
    if (
      error !== null &&
      typeof error === "object" &&
      "code" in error &&
      (error.code === "ENOENT" || error.code === "EISDIR")
    )
      return "";
    throw error;
  }
}

function invoke(
  fixture: Pick<Fixture, "repo" | "event">,
  eventName = "pull_request",
  output = path.join(fixture.repo, "github-output.txt"),
  env: NodeJS.ProcessEnv = {},
) {
  const previous = readIfFile(output);
  const result = command(
    process.execPath,
    [SCRIPT, "--repo", fixture.repo, "--event", fixture.event],
    {
      env: {
        GITHUB_EVENT_NAME: eventName,
        GITHUB_OUTPUT: output,
        ...env,
      },
    },
  );
  let json: ScopeResult | null = null;
  if (result.stdout) json = JSON.parse(String(result.stdout).trim());
  assert.ok(json, "selector emitted no JSON result");
  const outputText = readIfFile(output);
  if (json.reason === "output-write") {
    assert.equal(
      result.stdout,
      JSON.stringify(fullSelection("output-write")) + "\n",
    );
    assert.equal(outputText, previous);
  } else {
    const appended = outputText.slice(previous.length);
    const fields = Object.fromEntries(
      appended
        .trimEnd()
        .split("\n")
        .map((line) => line.split("=")),
    );
    const selection = validateSelection(
      { result: result.status === 0 ? "success" : "failure", outputs: fields },
      eventName,
    );
    assert.equal(appended, encodeSelection(selection));
    assert.equal(result.stdout, JSON.stringify(selection) + "\n");
  }
  return { ...result, json, outputText };
}

function assertAssetOnly(fixture: Fixture): ScopeResult {
  const result = invoke(fixture);
  assert.equal(result.status, 0, result.stderr);
  assert.deepEqual(result.json, {
    mode: "asset-only",
    complete: true,
    base: fixture.base,
    head: fixture.head,
    merge: fixture.merge,
    reason: "eligible",
    areas: "",
    shards: [],
    tsShards: [],
    race: false,
  });
  assert.match(
    result.outputText,
    new RegExp(
      `^mode=asset-only\\ncomplete=true\\nbase=${fixture.base}\\nhead=${fixture.head}\\nmerge=${fixture.merge}\\nreason=eligible\\nareas=\\nshards=\\[\\]\\nts_shards=\\[\\]\\nrace=false\\n$`,
    ),
  );
  return result.json;
}

function expectedFields(extra: Partial<ScopeResult>) {
  return {
    areas: "",
    shards: ["hooks"],
    tsShards: ["ts-tools"],
    race: true,
    ...extra,
  };
}

function assertAreaScoped(
  fixture: Fixture,
  areas: string,
  shards: readonly string[],
  race: boolean,
): ScopeResult {
  const result = invoke(fixture);
  const tsShards = ["tools", "hooks", "evals"].some((area) =>
    areas.split(",").includes(area),
  )
    ? ["ts-tools"]
    : [];
  assert.equal(result.status, 0, result.stderr);
  assert.deepEqual(result.json, {
    mode: "area-scoped",
    complete: true,
    base: fixture.base,
    head: fixture.head,
    merge: fixture.merge,
    reason: "area-scoped",
    areas,
    shards,
    tsShards,
    race,
  });
  assert.ok(
    result.outputText.endsWith(
      `reason=area-scoped\nareas=${areas}\nshards=${JSON.stringify(shards)}\nts_shards=${JSON.stringify(tsShards)}\nrace=${race}\n`,
    ),
    result.outputText,
  );
  return result.json;
}

test("relocated maintainer scripts retain full selection and summary expectations", (t) => {
  const fixture = makeFixture(".github/tools/src/scripts/ci-scope.ts");
  t.after(() => fs.rmSync(fixture.repo, { recursive: true, force: true }));
  const selected = invoke(fixture);
  assert.equal(selected.status, 0, selected.stderr);
  assert.equal(selected.json.mode, "full");
  assert.equal(selected.json.reason, "other-path");
  assert.deepEqual(selected.json.shards, ["hooks"]);
  assert.deepEqual(selected.json.tsShards, ["ts-tools"]);
  assert.equal(selected.json.race, true);
  const outputs = Object.fromEntries(
    selected.outputText
      .trim()
      .split("\n")
      .map((line) => {
        const split = line.indexOf("=");
        return [line.slice(0, split), line.slice(split + 1)];
      }),
  );
  const jobs = [
    "scope",
    "pr-metadata",
    "pr-version-fields",
    "validate-claude-native",
    "validate-antigravity-native",
    "conformance",
    "go-suite",
    "ts-suite",
    "format",
    "shellcheck",
    "gh-aw-lock-drift",
    "manual",
  ];
  const needs = Object.fromEntries(
    jobs.map((id) => [
      id,
      {
        result: id === "manual" ? "skipped" : "success",
        ...(id === "scope" ? { outputs } : {}),
      },
    ]),
  );
  const summaryFile = path.join(fixture.repo, "summary.md");
  const summary = command(
    process.execPath,
    [
      path.join(import.meta.dirname, "ci-summary.ts"),
      "--needs-env",
      "AGX_CI_NEEDS_JSON",
      "--event-name-env",
      "GITHUB_EVENT_NAME",
      "--summary-env",
      "GITHUB_STEP_SUMMARY",
    ],
    {
      env: {
        AGX_CI_NEEDS_JSON: JSON.stringify(needs),
        GITHUB_EVENT_NAME: "pull_request",
        GITHUB_STEP_SUMMARY: summaryFile,
      },
    },
  );
  assert.equal(summary.status, 0, summary.stderr);
  const markdown = fs.readFileSync(summaryFile, "utf8");
  assert.ok(markdown.includes("| ➖ | Manual | skipped | skipped |"));
  for (const label of ["Test / TypeScript", "Test / Go"])
    assert.ok(markdown.includes(`| ✅ | ${label} | success | success |`));
});

test("editing only the shared manifest inventory keeps full verification", (t) => {
  const fixture = makeFixture(
    ".github/tools/src/plugin-manifests.ts",
    "modify",
  );
  t.after(() => fs.rmSync(fixture.repo, { recursive: true, force: true }));
  const result = invoke(fixture);
  assert.deepEqual(
    selectPaths(fixture, [".github/tools/src/plugin-manifests.ts"]),
    result.json,
  );
  assert.equal(result.status, 0, result.stderr);
  assert.deepEqual(result.json, {
    mode: "full",
    complete: false,
    base: fixture.base,
    head: fixture.head,
    merge: fixture.merge,
    reason: "other-path",
    areas: "",
    shards: ["hooks"],
    tsShards: ["ts-tools"],
    race: true,
  });
  assert.equal(
    result.outputText,
    `mode=full\ncomplete=false\nbase=${fixture.base}\nhead=${fixture.head}\nmerge=${fixture.merge}\nreason=other-path\nareas=\nshards=["hooks"]\nts_shards=["ts-tools"]\nrace=true\n`,
  );
});

test("only the repository area inventory edit keeps full verification", (t) => {
  const inventory = makeFixture(
    ".github/tools/src/repository-areas.ts",
    "modify",
  );
  const neighbor = makeFixture(
    ".github/tools/src/repository-areas.test.ts",
    "modify",
  );
  t.after(() => {
    fs.rmSync(inventory.repo, { recursive: true, force: true });
    fs.rmSync(neighbor.repo, { recursive: true, force: true });
  });

  const result = invoke(inventory);
  assert.equal(result.status, 0, result.stderr);
  assert.deepEqual(result.json, {
    mode: "full",
    complete: false,
    base: inventory.base,
    head: inventory.head,
    merge: inventory.merge,
    reason: "other-path",
    areas: "",
    shards: ["hooks"],
    tsShards: ["ts-tools"],
    race: true,
  });
  assert.equal(
    result.outputText,
    `mode=full\ncomplete=false\nbase=${inventory.base}\nhead=${inventory.head}\nmerge=${inventory.merge}\nreason=other-path\nareas=\nshards=["hooks"]\nts_shards=["ts-tools"]\nrace=true\n`,
  );
  assertAreaScoped(neighbor, "tools", [], false);
});

test("single-area changes select exactly the shards the table names", (t) => {
  const all = ["hooks"];
  const rows = [
    ["hooks/go/cmd/hook/main.go", "hooks", ["hooks"], true],
    ["evals/src/cli/main.ts", "evals", [], false],
    [".github/tools/src/conformance/cli.ts", "tools", [], false],
    // A skills change feeds the hooks shard (the skillcompat sweep) but needs no race run.
    ["skills/forge/evals/cases.json", "skills", all, false],
    ["skills/diagnose/references/find-polluter.sh", "skills", all, false],
  ] as const;
  for (const [relative, areas, shards, race] of rows) {
    const fixture = makeFixture(relative, "add");
    t.after(() => fs.rmSync(fixture.repo, { recursive: true, force: true }));
    const selected = assertAreaScoped(fixture, areas, shards, race);
    assert.deepEqual(selectPaths(fixture, [relative]), selected);
  }
});

test("Markdown beside an area change adds no area, and two areas union their shards", (t) => {
  const mixed = makeMultiFixture(["docs/x.md", "hooks/go/cmd/hook/main.go"]);
  const two = makeMultiFixture(["hooks/go/a.go", ".github/tools/src/b.ts"]);
  t.after(() => {
    fs.rmSync(mixed.repo, { recursive: true, force: true });
    fs.rmSync(two.repo, { recursive: true, force: true });
  });
  assert.deepEqual(
    selectPaths(mixed, ["docs/x.md", "hooks/go/cmd/hook/main.go"]),
    assertAreaScoped(mixed, "hooks", ["hooks"], true),
  );
  assert.deepEqual(
    selectPaths(two, ["hooks/go/a.go", ".github/tools/src/b.ts"]),
    assertAreaScoped(two, "hooks,tools", ["hooks"], true),
  );
});

test("TypeScript outputs select the tools suite for tools and eval data, and all on unknown or failed scope", (t) => {
  for (const [relative, expected] of [
    [".github/tools/src/example.ts", ["ts-tools"]],
    ["evals/skills/example.md", ["ts-tools"]],
    ["unknown.fixture", ["ts-tools"]],
  ] as const) {
    const fixture = makeFixture(relative, "add");
    t.after(() => fs.rmSync(fixture.repo, { recursive: true, force: true }));
    const result = invoke(fixture);
    assert.equal(result.status, 0, result.stderr);
    assert.deepEqual(result.json.tsShards, expected, relative);
    assert.ok(
      result.outputText.includes(`ts_shards=${JSON.stringify(expected)}\n`),
      result.outputText,
    );
    fs.writeFileSync(fixture.event, "not-json\n");
    const failed = invoke(
      fixture,
      "pull_request",
      path.join(fixture.repo, "failed-output"),
    );
    assert.equal(failed.status, 1);
    assert.equal(failed.json.reason, "event-input");
    assert.deepEqual(failed.json.tsShards, ["ts-tools"]);
    assert.ok(
      failed.outputText.includes('ts_shards=["ts-tools"]\n'),
      failed.outputText,
    );
  }
});

test("a rename counts both sides, so moving a file between areas runs both", (t) => {
  const fixture = makePairFixture(
    "rename",
    "evals/src/moved.ts",
    "hooks/go/original.go",
  );
  t.after(() => fs.rmSync(fixture.repo, { recursive: true, force: true }));
  assertAreaScoped(fixture, "evals,hooks", ["hooks"], true);
});

test("a safe path outside every area and asset path is other-path, and an unsafe path stays unknown-path", (t) => {
  for (const relative of [
    ".github/workflows/validate.yml",
    "package.json",
    "docs/hostile name.txt",
    "unknown.fixture",
  ]) {
    const fixture = makeFixture(relative, "add");
    t.after(() => fs.rmSync(fixture.repo, { recursive: true, force: true }));
    const result = invoke(fixture);
    assert.equal(result.json.mode, "full", relative);
    assert.equal(result.json.reason, "other-path", relative);
    assert.deepEqual(result.json.shards, expectedFields({}).shards);
    assert.equal(result.json.race, true);
  }
  const unsafe = makeIndexPathFixture("hooks/CON.go");
  t.after(() => fs.rmSync(unsafe.repo, { recursive: true, force: true }));
  assert.equal(invoke(unsafe).json.reason, "unknown-path");
});

test("added, modified, and deleted root prose qualify for asset-only", (t) => {
  for (const [index, kind] of ["add", "modify", "delete"].entries()) {
    const fixture = makeFixture(ROOT_PROSE[index]!, kind);
    t.after(() => fs.rmSync(fixture.repo, { recursive: true, force: true }));
    assert.deepEqual(
      selectPaths(fixture, [ROOT_PROSE[index]!]),
      assertAssetOnly(fixture),
    );
  }
});

test("every reviewed provider manifest qualifies for asset-only", (t) => {
  assert.deepEqual(CI_ASSET_PLUGIN_MANIFEST_PATHS, MANIFESTS);
  for (const relative of MANIFESTS) {
    // The native probe directly reads .github/plugin/plugin.json,
    // .claude-plugin/marketplace.json, and .claude-plugin/plugin.json in
    // the newProbeFixture installed root of
    // the hook conformance tests, the files
    // manifests.CopilotProviderRules reads. The other provider manifests
    // remain explicit policy entries; expanding this set requires a fresh
    // consumer audit before CI scope allocates validation to it.
    const fixture = makeFixture(relative, "add");
    t.after(() => fs.rmSync(fixture.repo, { recursive: true, force: true }));
    assert.deepEqual(
      selectPaths(fixture, [relative]),
      assertAssetOnly(fixture),
    );
  }
});

test("added, modified, and deleted .antigravity-plugin/plugin.json qualify for asset-only", (t) => {
  for (const kind of ["add", "modify", "delete"] as const) {
    const fixture = makeFixture(".antigravity-plugin/plugin.json", kind);
    t.after(() => fs.rmSync(fixture.repo, { recursive: true, force: true }));
    assertAssetOnly(fixture);
  }
});

test("Markdown under the reviewed roots qualifies for asset-only", (t) => {
  for (const relative of ["docs/guide.md", "skills/execute/SKILL.md"]) {
    const fixture = makeFixture(relative, "add");
    t.after(() => fs.rmSync(fixture.repo, { recursive: true, force: true }));
    assertAssetOnly(fixture);
  }
});

test("Windows-invalid Markdown paths cannot qualify for asset-only", (t) => {
  const fixtures = [
    "docs/con.md",
    "docs/a:b.md",
    "docs/invalid./guide.md",
    "docs/invalid /guide.md",
    "docs/CON.md",
    "docs/LPT0.md",
  ].map(makeIndexPathFixture);
  t.after(() => {
    for (const fixture of fixtures)
      fs.rmSync(fixture.repo, { recursive: true, force: true });
  });
  for (const fixture of fixtures) {
    const result = invoke(fixture);
    assert.equal(result.status, 0, result.stderr);
    assert.equal(result.json.reason, "unknown-path");
  }
});

test("configuration, workflows, release inputs, and unknown files outside the areas stay full", (t) => {
  // These paths belong to no area, so a change to one cannot be scoped to a
  // shard and every shard runs.
  const paths: [string, string][] = [
    [".claude/settings.json", "config"],
    [".github/scripts/plugin.sh", "plugin-entrypoint"],
    [".github/actions/conformance/action.yml", "local-action"],
    [".github/workflows/release.md", "release-source"],
    [".github/workflows/release.lock.yml", "release-lock"],
    ["unknown.fixture", "unknown"],
  ];
  for (const [relative, label] of paths) {
    const fixture = makeFixture(relative, "add");
    t.after(() => fs.rmSync(fixture.repo, { recursive: true, force: true }));
    const result = invoke(fixture);
    assert.equal(result.status, 0, `${label}: ${result.stderr}`);
    assert.deepEqual(result.json, {
      mode: "full",
      complete: false,
      base: fixture.base,
      head: fixture.head,
      merge: fixture.merge,
      reason: "other-path",
      ...expectedFields({}),
    });
    assert.match(
      result.outputText,
      /^mode=full\ncomplete=false\n[\s\S]*\nreason=other-path\nareas=\nshards=\["hooks"\]\nts_shards=\["ts-tools"\]\nrace=true\n$/,
    );
  }
});

test("schedule and manual events conservatively select full without diff inputs", (t) => {
  const repo = fs.mkdtempSync(
    path.join(os.tmpdir(), "agentics-ci-scope-event-"),
  );
  t.after(() => fs.rmSync(repo, { recursive: true, force: true }));
  const event = path.join(repo, "event.json");
  fs.writeFileSync(event, "{}\n");
  for (const eventName of [
    "schedule",
    "workflow_dispatch",
    "push",
    "unknown-event",
  ] as const) {
    const result = invoke({ repo, event }, eventName);
    assert.equal(result.status, 0, `${eventName}: ${result.stderr}`);
    assert.deepEqual(result.json, {
      mode: "full",
      complete: false,
      base: "",
      head: "",
      merge: "",
      reason: "event-full",
      ...expectedFields({}),
    });
  }
});

test("the checked-out merge must have the event base and head as its two parents", (t) => {
  const fixture = makeFixture("README.md", "modify");
  t.after(() => fs.rmSync(fixture.repo, { recursive: true, force: true }));
  fs.writeFileSync(
    fixture.event,
    JSON.stringify({
      pull_request: {
        base: { sha: ZERO },
        head: { sha: fixture.head },
      },
    }),
  );
  const result = invoke(fixture);
  assert.equal(result.status, 0, result.stderr);
  assert.deepEqual(result.json, {
    mode: "full",
    complete: false,
    base: ZERO,
    head: fixture.head,
    merge: fixture.merge,
    reason: "merge-input-mismatch",
    ...expectedFields({}),
  });
});

test("a commit-message parent line cannot turn a one-parent checkout into a merge", (t) => {
  const fixture = makeOneParentMessageSpoofFixture();
  t.after(() => fs.rmSync(fixture.repo, { recursive: true, force: true }));
  const result = invoke(fixture);
  assert.equal(result.status, 0, result.stderr);
  assert.deepEqual(result.json, {
    mode: "full",
    complete: false,
    base: fixture.base,
    head: fixture.head,
    merge: fixture.merge,
    reason: "merge-input-mismatch",
    ...expectedFields({}),
  });
});

test("renames qualify only when both sides stay in the reviewed allowlist", (t) => {
  const allowed = makePairFixture("rename", "docs/renamed.md");
  const rejected = makePairFixture("rename", "unknown.txt");
  const reverse = makePairFixture("rename", "docs/reverse.md", "go/source.go");
  t.after(() => {
    fs.rmSync(allowed.repo, { recursive: true, force: true });
    fs.rmSync(rejected.repo, { recursive: true, force: true });
    fs.rmSync(reverse.repo, { recursive: true, force: true });
  });
  assertAssetOnly(allowed);
  assert.equal(invoke(rejected).json.reason, "other-path");
  assert.equal(invoke(reverse).json.reason, "other-path");
});

test("copies qualify only when both sides stay in the reviewed allowlist", (t) => {
  const allowed = makePairFixture("copy", "docs/copied.md");
  const rejected = makePairFixture("copy", "unknown.txt");
  const reverse = makePairFixture(
    "copy",
    "docs/reverse-copy.md",
    "go/source.go",
  );
  t.after(() => {
    fs.rmSync(allowed.repo, { recursive: true, force: true });
    fs.rmSync(rejected.repo, { recursive: true, force: true });
    fs.rmSync(reverse.repo, { recursive: true, force: true });
  });
  assertAssetOnly(allowed);
  assert.equal(invoke(rejected).json.reason, "other-path");
  assert.equal(invoke(reverse).json.reason, "other-path");
});

test("raw paths with spaces remain one field while hostile paths stay full", (t) => {
  const spaced = makeFixture("docs/space name.md", "add");
  const hostile = makeFixture("docs/hostile name.txt", "add");
  t.after(() => {
    fs.rmSync(spaced.repo, { recursive: true, force: true });
    fs.rmSync(hostile.repo, { recursive: true, force: true });
  });
  assertAssetOnly(spaced);
  const result = invoke(hostile);
  assert.equal(result.status, 0, result.stderr);
  assert.equal(result.json.reason, "other-path");
});

test("index paths with newline, backslash, and quote bytes stay full", (t) => {
  const newline = makeIndexPathFixture("docs/line\nname.md");
  const backslash = makeIndexPathFixture(String.raw`docs/back\slash.md`);
  const quote = makeIndexPathFixture('docs/quote"name.md');
  t.after(() => {
    fs.rmSync(newline.repo, { recursive: true, force: true });
    fs.rmSync(backslash.repo, { recursive: true, force: true });
    fs.rmSync(quote.repo, { recursive: true, force: true });
  });
  for (const fixture of [newline, backslash, quote]) {
    const result = invoke(fixture);
    assert.equal(result.status, 0, result.stderr);
    assert.equal(result.json.reason, "unknown-path");
  }
});

test("an empty immutable diff conservatively selects full", (t) => {
  const fixture = makeFixture("docs/empty.md", "empty");
  t.after(() => fs.rmSync(fixture.repo, { recursive: true, force: true }));
  const result = invoke(fixture);
  assert.equal(result.status, 0, result.stderr);
  assert.equal(result.json.reason, "empty-diff");
  assert.equal(result.json.complete, false);
  assert.equal(result.json.mode, "full");
});

test("executable and symlink modes conservatively select full", (t) => {
  const executable = makeModeFixture("executable");
  const symlink = makeModeFixture("symlink");
  t.after(() => {
    fs.rmSync(executable.repo, { recursive: true, force: true });
    fs.rmSync(symlink.repo, { recursive: true, force: true });
  });
  assert.equal(invoke(executable).json.reason, "non-regular-mode");
  assert.equal(invoke(symlink).json.reason, "unsupported-status");
});

test("malformed event IDs fail the selector instead of authorizing a skip", (t) => {
  const fixture = makeFixture("README.md", "modify");
  t.after(() => fs.rmSync(fixture.repo, { recursive: true, force: true }));
  fs.writeFileSync(
    fixture.event,
    JSON.stringify({
      pull_request: {
        base: { sha: fixture.base.toUpperCase() },
        head: { sha: fixture.head },
      },
    }),
  );
  const result = invoke(fixture);
  assert.equal(result.status, 1);
  assert.equal(result.json.reason, "event-input");
  assert.equal(result.json.mode, "full");
});

test("missing parent history stays full with an incomplete-history reason", (t) => {
  const fixture = makeFixture("README.md", "modify");
  t.after(() => fs.rmSync(fixture.repo, { recursive: true, force: true }));
  const objectPath = path.join(
    fixture.repo,
    ".git",
    "objects",
    fixture.base.slice(0, 2),
    fixture.base.slice(2),
  );
  assert.equal(fs.existsSync(objectPath), true);
  fs.rmSync(objectPath);
  const result = invoke(fixture);
  assert.equal(result.status, 0, result.stderr);
  assert.equal(result.json.reason, "incomplete-history");
});

test("a GITHUB_OUTPUT write failure is a selector failure with a fixed full result", (t) => {
  const fixture = makeFixture("README.md", "modify");
  t.after(() => fs.rmSync(fixture.repo, { recursive: true, force: true }));
  const result = invoke(fixture, "pull_request", fixture.repo);
  assert.equal(result.status, 1);
  assert.deepEqual(result.json, {
    mode: "full",
    complete: false,
    base: "",
    head: "",
    merge: "",
    reason: "output-write",
    ...expectedFields({}),
  });
});

test("a merge tree discrepancy is checked and selects full", (t) => {
  const fixture = makeMergeTreeDiscrepancyFixture();
  t.after(() => fs.rmSync(fixture.repo, { recursive: true, force: true }));
  const result = invoke(fixture);
  assert.equal(result.status, 0, result.stderr);
  assert.equal(result.json.reason, "other-path");
  assert.equal(result.json.complete, false);
});

test("malformed and truncated raw Git bytes fail closed at the public CLI", (t) => {
  const shim = makeGitShim(t);
  const fixture = makeFixture("README.md", "modify");
  t.after(() => fs.rmSync(fixture.repo, { recursive: true, force: true }));
  const cases = [
    ["truncated", 1, "malformed-diff"],
    ["missing-rename-path", 1, "malformed-diff"],
    ["empty-path", 1, "malformed-diff"],
    ["invalid-header", 1, "malformed-diff"],
    ["invalid-oid", 1, "malformed-diff"],
    ["invalid-score", 1, "malformed-diff"],
    ["invalid-utf8", 1, "malformed-diff"],
    ["mode-mismatch", 0, "non-regular-mode"],
    ["status-T", 0, "unsupported-status"],
    ["oid-mismatch", 1, "malformed-diff"],
  ] as const;
  for (const [mode, status, reason] of cases) {
    const result = invoke(
      fixture,
      "pull_request",
      path.join(fixture.repo, `${mode}.out`),
      shim.env(mode),
    );
    assert.equal(result.status, status, `${mode}: ${result.stderr}`);
    assert.equal(result.json.mode, "full", mode);
    assert.equal(result.json.complete, false, mode);
    assert.equal(result.json.reason, reason, mode);
  }
});

test("Git command failures while reading parents or diffs fail nonzero", (t) => {
  const shim = makeGitShim(t);
  const fixture = makeFixture("README.md", "modify");
  t.after(() => fs.rmSync(fixture.repo, { recursive: true, force: true }));
  for (const mode of ["parent-failure", "diff-failure"]) {
    const result = invoke(
      fixture,
      "pull_request",
      path.join(fixture.repo, `${mode}.out`),
      shim.env(mode),
    );
    assert.equal(result.status, 1, `${mode}: ${result.stderr}`);
    assert.deepEqual(result.json, {
      mode: "full",
      complete: false,
      base: "",
      head: "",
      merge: "",
      reason: "git-error",
      ...expectedFields({}),
    });
  }
});

test("missing and malformed event inputs fail with fixed reasons", (t) => {
  const fixture = makeFixture("README.md", "modify");
  t.after(() => fs.rmSync(fixture.repo, { recursive: true, force: true }));
  fs.writeFileSync(fixture.event, "not-json\n");
  const malformed = invoke(fixture);
  assert.equal(malformed.status, 1);
  assert.equal(malformed.json.reason, "event-input");
  const missing = path.join(fixture.repo, "missing-event.json");
  const result = command(
    process.execPath,
    [SCRIPT, "--repo", fixture.repo, "--event", missing],
    { env: { GITHUB_EVENT_NAME: "pull_request" } },
  );
  assert.equal(result.status, 1);
  assert.equal(JSON.parse(String(result.stdout).trim()).reason, "event-input");
});

test("argument errors emit the full fallback and output write errors take precedence", (t) => {
  const repo = fs.mkdtempSync(path.join(os.tmpdir(), "agentics-ci-scope-cli-"));
  t.after(() => fs.rmSync(repo, { recursive: true, force: true }));
  const output = path.join(repo, "github-output");
  for (const [destination, reason] of [
    [output, "cli-input"],
    [repo, "output-write"],
  ] as const) {
    const legacy = command(process.execPath, [SCRIPT], {
      env: { GITHUB_OUTPUT: destination },
    });
    const selected = fullSelection(reason);
    assert.equal(legacy.status, 1);
    assert.equal(legacy.stderr, "");
    assert.equal(legacy.stdout, JSON.stringify(selected) + "\n");
    if (reason === "cli-input")
      assert.equal(readIfFile(output), encodeSelection(selected));
  }
});

test("the scheduling command publishes no reductions from failed or invalid provider input", (t) => {
  const dir = fs.mkdtempSync(
    path.join(os.tmpdir(), "agentics-ci-schedule-invalid-"),
  );
  t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
  const output = path.join(dir, "github-output");
  const reduced = {
    mode: "asset-only",
    complete: "true",
    base: "a".repeat(40),
    head: "b".repeat(40),
    merge: "c".repeat(40),
    reason: "eligible",
    shards: "[]",
    ts_shards: "[]",
    race: "false",
  };
  for (const [outcome, raw] of [
    ["failure", JSON.stringify(reduced)],
    ["success", ""],
    ["success", "{"],
    ["success", "[]"],
    ["success", JSON.stringify({ ...reduced, race: undefined })],
    ["success", JSON.stringify({ ...reduced, race: "true" })],
  ]) {
    const result = command(process.execPath, [SCRIPT, "--schedule"], {
      env: {
        GITHUB_OUTPUT: output,
        GITHUB_EVENT_NAME: "pull_request",
        AGX_CI_CLASSIFY_OUTPUTS: raw,
        AGX_CI_CLASSIFY_OUTCOME: outcome,
      },
    });
    assert.equal(result.status, 0, result.stderr);
    assert.equal(result.stdout, "");
    assert.equal(result.stderr, "");
    assert.equal(fs.existsSync(output), false);
  }
});

test("generated POSIX shim directly exercises handled and delegated branches", (t) => {
  const shim = makeGitShim(t, "posix");
  assert.equal(fs.existsSync(shim.binary), true);
  assert.deepEqual(fs.readFileSync(shim.binary), fs.readFileSync(shim.source));
  const invokeShim = (mode: string, args: string[]) =>
    command(process.execPath, [shim.binary, ...args], {
      env: {
        ...process.env,
        PATH: `${path.dirname(shim.binary)}${path.delimiter}${process.env.PATH || ""}`,
        AGX_SCOPE_SHIM_MODE: mode,
      },
    });
  const valid = invokeShim("valid", ["diff", "--raw"]);
  assert.equal(valid.status, 0);
  assert.equal(String(valid.stdout).endsWith("\0"), true);
  const truncated = invokeShim("truncated", ["diff", "--raw"]);
  assert.equal(truncated.status, 0);
  assert.equal(String(truncated.stdout).endsWith("\0"), false);
  const diffFailure = invokeShim("diff-failure", ["diff", "--raw"]);
  assert.equal(diffFailure.status, 23);
  assert.match(String(diffFailure.stderr), /shim diff failure/);
  const parentFailure = invokeShim("parent-failure", ["cat-file", "-p"]);
  assert.equal(parentFailure.status, 23);
  assert.match(String(parentFailure.stderr), /shim parent failure/);
  const delegated = invokeShim("unknown-mode", ["--version"]);
  assert.equal(delegated.status, 0);
  assert.match(String(delegated.stdout), /^git version /);
  t.diagnostic(
    JSON.stringify({
      valid: {
        status: valid.status,
        endsNul: String(valid.stdout).endsWith("\0"),
      },
      truncated: {
        status: truncated.status,
        endsNul: String(truncated.stdout).endsWith("\0"),
      },
      diffFailure: {
        status: diffFailure.status,
        stderr: String(diffFailure.stderr).trim(),
      },
      parentFailure: {
        status: parentFailure.status,
        stderr: String(parentFailure.stderr).trim(),
      },
      delegated: {
        status: delegated.status,
        stdout: String(delegated.stdout).trim(),
      },
    }),
  );
});

function invokePush(fixture: Fixture, lastGreen: string) {
  return invoke(fixture, "push", undefined, {
    AGX_CI_LAST_GREEN: lastGreen,
  });
}

test("a push is scoped against the last validated commit, with no merge identity", (t) => {
  const fixture = makeFixture("evals/src/cli/main.ts", "add");
  t.after(() => fs.rmSync(fixture.repo, { recursive: true, force: true }));
  const result = invokePush(fixture, fixture.base);
  assert.equal(result.status, 0, result.stderr);
  assert.deepEqual(result.json, {
    mode: "area-scoped",
    complete: true,
    base: fixture.base,
    head: fixture.merge,
    merge: "",
    reason: "area-scoped",
    areas: "evals",
    shards: [],
    tsShards: ["ts-tools"],
    race: false,
  });
});

test("a Markdown-only push since the last validated commit is asset-only", (t) => {
  const fixture = makeFixture("docs/guide.md", "add");
  t.after(() => fs.rmSync(fixture.repo, { recursive: true, force: true }));
  const result = invokePush(fixture, fixture.base);
  assert.equal(result.status, 0, result.stderr);
  assert.equal(result.json.mode, "asset-only");
  assert.equal(result.json.head, fixture.merge);
  assert.equal(result.json.merge, "");
  assert.deepEqual(result.json.shards, []);
});

test("commits a cancelled push left unvalidated stay in the next push's scope", (t) => {
  const fixture = makeFixture("hooks/go/cmd/hook/main.go", "add");
  t.after(() => fs.rmSync(fixture.repo, { recursive: true, force: true }));
  write(fixture.repo, "docs/later.md", "a later Markdown-only push\n");
  git(fixture.repo, ["add", "docs/later.md"]);
  git(fixture.repo, ["commit", "--message", "docs"]);
  const previous = git(fixture.repo, ["rev-parse", "HEAD~1"]);

  const sinceGreen = invokePush(fixture, fixture.base);
  assert.equal(sinceGreen.json.mode, "area-scoped");
  assert.equal(sinceGreen.json.areas, "hooks");
  const sincePrevious = invokePush(fixture, previous);
  assert.equal(sincePrevious.json.mode, "asset-only");
});

test("a push with no validated commit runs in full", (t) => {
  const fixture = makeFixture("docs/guide.md", "add");
  t.after(() => fs.rmSync(fixture.repo, { recursive: true, force: true }));
  const result = invokePush(fixture, "");
  assert.equal(result.status, 0, result.stderr);
  assert.deepEqual(result.json, {
    mode: "full",
    complete: false,
    base: "",
    head: "",
    merge: "",
    reason: "event-full",
    ...expectedFields({}),
  });
});

test("a validated commit that is not an ancestor of the push, or not in the clone, runs in full", (t) => {
  const fixture = makeFixture("docs/guide.md", "add");
  t.after(() => fs.rmSync(fixture.repo, { recursive: true, force: true }));
  git(fixture.repo, ["switch", "--create", "side", fixture.base]);
  write(fixture.repo, "docs/side.md", "not on main\n");
  git(fixture.repo, ["add", "docs/side.md"]);
  git(fixture.repo, ["commit", "--message", "side"]);
  const side = git(fixture.repo, ["rev-parse", "HEAD"]);
  git(fixture.repo, ["switch", "main"]);

  const diverged = invokePush(fixture, side);
  assert.equal(diverged.status, 0, diverged.stderr);
  assert.equal(diverged.json.mode, "full");
  assert.equal(diverged.json.reason, "base-not-ancestor");
  assert.deepEqual(diverged.json.shards, expectedFields({}).shards);

  const missing = invokePush(fixture, "f".repeat(40));
  assert.equal(missing.status, 0, missing.stderr);
  assert.equal(missing.json.mode, "full");
  assert.equal(missing.json.reason, "incomplete-history");
});

test("a malformed validated commit fails the selector, and other events ignore the variable", (t) => {
  const fixture = makeFixture("docs/guide.md", "add");
  t.after(() => fs.rmSync(fixture.repo, { recursive: true, force: true }));
  const malformed = invokePush(fixture, "main");
  assert.equal(malformed.status, 1);
  assert.equal(malformed.json.mode, "full");
  assert.equal(malformed.json.reason, "event-input");

  const scheduled = invoke(fixture, "schedule", undefined, {
    AGX_CI_LAST_GREEN: fixture.base,
  });
  assert.equal(scheduled.json.reason, "event-full");
  const pull = invoke(fixture, "pull_request", undefined, {
    AGX_CI_LAST_GREEN: fixture.head,
  });
  assert.equal(pull.json.merge, fixture.merge);
  assert.equal(pull.json.base, fixture.base);
});
