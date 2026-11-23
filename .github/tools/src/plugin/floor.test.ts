import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import {
  copyFileSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  readdirSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import os from "node:os";
import path from "node:path";
import { after, test } from "node:test";
import { fileURLToPath } from "node:url";
import { checkNodeFloor, readEnginesNode } from "./floor.ts";
import { runValidate } from "./validate.ts";

const RANGE = "^22.23.5 || ^24.22.3 || >=26.12.1";
const here = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(here, "..", "..", "..", "..");

const roots: string[] = [];
function tempDir(): string {
  const dir = mkdtempSync(path.join(os.tmpdir(), "agentics-floor-"));
  roots.push(dir);
  return dir;
}
after(() => {
  for (const dir of roots) rmSync(dir, { recursive: true, force: true });
});

type Line = { stream: "stdout" | "stderr"; text: string };
function capture(): {
  lines: Line[];
  write: (stream: "stdout" | "stderr", text: string) => void;
} {
  const lines: Line[] = [];
  return { lines, write: (stream, text) => lines.push({ stream, text }) };
}

for (const version of ["26.12.1", "24.22.3", "22.23.5"]) {
  test(`Node ${version} is inside the range`, () => {
    const out = capture();
    assert.equal(
      checkNodeFloor({
        action: "status",
        nodeVersion: version,
        enginesNode: RANGE,
        write: out.write,
      }),
      true,
    );
    assert.deepEqual(out.lines, []);
  });
}

for (const version of ["26.12.0", "24.22.2", "22.23.4", "23.0.0"]) {
  test(`Node ${version} is outside the range`, () => {
    const out = capture();
    assert.equal(
      checkNodeFloor({
        action: "setup",
        nodeVersion: version,
        enginesNode: RANGE,
        write: out.write,
      }),
      false,
    );
    assert.equal(out.lines.length, 1);
    assert.equal(out.lines[0]?.stream, "stderr");
    assert.ok(out.lines[0]?.text.includes(RANGE));
  });
}

for (const action of ["setup", "status", "teardown", "validate"]) {
  test(`a failing floor for the ${action} action writes one range line`, () => {
    const out = capture();
    const ok = checkNodeFloor({
      action,
      nodeVersion: "26.12.0",
      enginesNode: RANGE,
      write: out.write,
    });
    assert.equal(ok, false);
    const expected =
      action === "validate"
        ? `❌ --validate needs Node ${RANGE} for the TypeScript conformance suite and the tools tests.\n`
        : `❌ node .github/tools/src/plugin/cli.ts needs Node ${RANGE}; this is Node 26.12.0.\n`;
    assert.deepEqual(out.lines, [{ stream: "stderr", text: expected }]);
    assert.ok(!expected.includes("Skipped validation"));
  });
}

test("the validate line equals the one runValidate prints", () => {
  const floor = capture();
  checkNodeFloor({
    action: "validate",
    nodeVersion: "26.12.0",
    enginesNode: RANGE,
    write: floor.write,
  });
  const stage = capture();
  const code = runValidate({
    mode: "strict",
    nodeVersion: "26.12.0",
    enginesNode: RANGE,
    write: stage.write,
    runner: () => {
      throw new Error("the runner must not start");
    },
  });
  assert.equal(code, 1);
  assert.deepEqual(floor.lines, stage.lines);
});

test("the range comes from package.json when none is injected", () => {
  const root = tempDir();
  writeFileSync(
    path.join(root, "package.json"),
    JSON.stringify({ engines: { node: ">=999.0.0" } }),
  );
  const out = capture();
  assert.equal(
    checkNodeFloor({
      action: "status",
      nodeVersion: "26.12.1",
      repoRoot: root,
      write: out.write,
    }),
    false,
  );
  assert.ok(out.lines[0]?.text.includes(">=999.0.0"));
});

test("readEnginesNode reads the range from the repository", () => {
  const result = readEnginesNode(repoRoot);
  assert.equal(result.ok, true);
});

const badManifests: [string, (root: string) => void][] = [
  ["a missing file", () => {}],
  [
    "invalid JSON",
    (root) => writeFileSync(path.join(root, "package.json"), "{"),
  ],
  [
    "a missing field",
    (root) => writeFileSync(path.join(root, "package.json"), "{}"),
  ],
  [
    "an unsupported range",
    (root) =>
      writeFileSync(
        path.join(root, "package.json"),
        JSON.stringify({ engines: { node: "~22.0.0" } }),
      ),
  ],
];
for (const [name, setup] of badManifests) {
  test(`${name} gives one line and a failure, with no stack`, () => {
    const root = tempDir();
    setup(root);
    const result = readEnginesNode(root);
    assert.equal(result.ok, false);
    const message = result.ok ? "" : result.message;
    assert.ok(
      message.startsWith("❌ Could not read the Node range from "),
      message,
    );
    assert.ok(message.includes(path.join(root, "package.json")));
    assert.ok(!message.includes("\n"));
    const out = capture();
    assert.equal(
      checkNodeFloor({ action: "status", repoRoot: root, write: out.write }),
      false,
    );
    assert.equal(out.lines.length, 1);
  });
}

/** Copies the plugin modules and everything they reach through relative imports into a fresh tree. */
function copyModules(destRoot: string): void {
  const queue = readdirSync(here)
    .filter((name) => name.endsWith(".ts") && !name.endsWith(".test.ts"))
    .map((name) => path.join(here, name));
  const seen = new Set<string>();
  while (queue.length > 0) {
    const file = queue.pop() as string;
    if (seen.has(file)) continue;
    seen.add(file);
    const target = path.join(destRoot, path.relative(repoRoot, file));
    mkdirSync(path.dirname(target), { recursive: true });
    copyFileSync(file, target);
    const text = readFileSync(file, "utf8");
    for (const match of text.matchAll(/from "(\.{1,2}\/[^"]+)"/g)) {
      queue.push(path.resolve(path.dirname(file), match[1] as string));
    }
  }
}

function cliFixture(): string {
  const root = tempDir();
  copyModules(root);
  writeFileSync(
    path.join(root, "package.json"),
    JSON.stringify({ engines: { node: ">=999.0.0" } }),
  );
  return path.join(root, ".github", "tools", "src", "plugin", "cli.ts");
}

function runCli(cli: string, args: string[]) {
  const env: NodeJS.ProcessEnv = { PATH: path.dirname(process.execPath) };
  if (process.platform === "win32") {
    for (const key of ["SystemRoot", "ComSpec", "PATHEXT", "TEMP", "TMP"]) {
      env[key] = process.env[key];
    }
  }
  return spawnSync(process.execPath, [cli, ...args], {
    env,
    encoding: "utf8",
  });
}

const floorCases: [string, string[]][] = [
  ["no arguments", []],
  ["--status", ["--status"]],
  ["--teardown", ["--teardown"]],
  ["--validate", ["--validate"]],
  ["--vscode", ["--vscode"]],
];
for (const [name, args] of floorCases) {
  test(`the CLI stops at the floor for ${name}`, () => {
    const result = runCli(cliFixture(), args);
    assert.equal(result.status, 1);
    assert.ok(result.stderr.includes(">=999.0.0"), result.stderr);
    const both = result.stdout + result.stderr;
    for (const text of [
      "not ported yet",
      "=== Validating manifests ===",
      "=== Timing summary ===",
    ]) {
      assert.ok(!both.includes(text), both);
    }
  });
}

test("--help still answers below the floor", () => {
  const result = runCli(cliFixture(), ["--help"]);
  assert.equal(result.status, 0);
  assert.ok(result.stdout.split("\n")[0]?.startsWith("Usage:"));
});

test("a usage error still answers below the floor", () => {
  const result = runCli(cliFixture(), ["--bogus"]);
  assert.equal(result.status, 1);
  assert.ok(result.stdout.includes("Unknown option: --bogus"));
});
