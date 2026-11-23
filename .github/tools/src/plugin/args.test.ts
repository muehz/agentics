import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { readFileSync } from "node:fs";
import path from "node:path";
import test from "node:test";

import { CLI_COMMAND, helpText, parseArgs } from "./args.ts";

const CLI = path.resolve(import.meta.dirname, "cli.ts");
const ROOT_PACKAGE = path.resolve(
  import.meta.dirname,
  "../../../../package.json",
);

test("no arguments set up the local checkout", () => {
  assert.deepEqual(parseArgs([]), {
    kind: "run",
    action: "setup",
    vscode: "",
    mode: "full",
  });
});

test("the last action wins", () => {
  const parsed = parseArgs(["--validate", "--status"]);
  assert.equal(parsed.kind === "run" && parsed.action, "status");
  const reversed = parseArgs(["--status", "--teardown"]);
  assert.equal(reversed.kind === "run" && reversed.action, "teardown");
});

test("--fast alone exits 1 with its own message", () => {
  assert.deepEqual(parseArgs(["--fast"]), {
    kind: "exit",
    code: 1,
    lines: ["--fast needs --validate.", `Run ${CLI_COMMAND} --help for usage.`],
  });
});

test("--fast is rejected when another action wins", () => {
  const parsed = parseArgs(["--validate", "--fast", "--status"]);
  assert.equal(parsed.kind, "exit");
  assert.equal(
    parsed.kind === "exit" && parsed.lines[0],
    "--fast needs --validate.",
  );
});

test("--validate --fast selects the fast mode", () => {
  assert.deepEqual(parseArgs(["--validate", "--fast"]), {
    kind: "run",
    action: "validate",
    vscode: "",
    mode: "fast",
  });
});

test("an unknown option prints it and exits 1", () => {
  assert.deepEqual(parseArgs(["--bogus"]), {
    kind: "exit",
    code: 1,
    lines: ["Unknown option: --bogus", `Run ${CLI_COMMAND} --help for usage.`],
  });
});

test("an unknown option wins over a help request before it", () => {
  const parsed = parseArgs(["--help", "--bogus"]);
  assert.equal(parsed.kind === "exit" && parsed.code, 1);
});

test("-h and --help print the help and exit 0", () => {
  for (const flag of ["-h", "--help"]) {
    assert.deepEqual(parseArgs([flag]), {
      kind: "exit",
      code: 0,
      lines: helpText(),
    });
  }
});

test("help states that every mode needs Node and --validate needs Go", () => {
  const text = helpText().join("\n");
  assert.match(text, /^Usage: node \.github\/tools\/src\/plugin\/cli\.ts /);
  assert.match(text, /Every mode needs Node in the engines\.node range/);
  assert.match(text, /--validate also needs Go/);
  assert.doesNotMatch(text, /Installing the plugin needs neither/);
  assert.doesNotMatch(text, /plugin\.sh/);
});

test("every usage line names the node entry", () => {
  for (const line of helpText().filter((l) => l.startsWith("Usage:"))) {
    assert.ok(line.includes(CLI_COMMAND));
  }
  assert.equal(CLI_COMMAND, "node .github/tools/src/plugin/cli.ts");
});

test("--vscode --validate validates", () => {
  const parsed = parseArgs(["--vscode", "--validate"]);
  assert.deepEqual(parsed, {
    kind: "run",
    action: "validate",
    vscode: "code",
    mode: "full",
  });
});

test("--vscode-insiders --teardown keeps the teardown action and the variant", () => {
  assert.deepEqual(parseArgs(["--vscode-insiders", "--teardown"]), {
    kind: "run",
    action: "teardown",
    vscode: "insiders",
    mode: "full",
  });
});

test("the last VS Code variant wins", () => {
  const parsed = parseArgs(["--vscode", "--vscode-insiders"]);
  assert.equal(parsed.kind === "run" && parsed.vscode, "insiders");
});

test("cli.ts --help prints usage and exits 0", () => {
  const result = spawnSync(process.execPath, [CLI, "--help"], {
    encoding: "utf8",
  });
  assert.equal(result.status, 0);
  assert.ok(result.stdout.split("\n")[0]?.startsWith("Usage:"));
});

test("cli.ts rejects an unknown option with exit 1", () => {
  const result = spawnSync(process.execPath, [CLI, "--bogus"], {
    encoding: "utf8",
  });
  assert.equal(result.status, 1);
  assert.match(result.stdout, /^Unknown option: --bogus\n/);
});

test("cli.ts --vscode prints the instructions and exits 0", () => {
  const result = spawnSync(process.execPath, [CLI, "--vscode"], {
    encoding: "utf8",
  });
  assert.equal(result.status, 0);
  assert.ok(
    result.stdout.startsWith("=== VS Code: local plugin registration ===\n"),
  );
});

test("root package.json defines the maintainer npm script shortcuts", () => {
  const pkg = JSON.parse(readFileSync(ROOT_PACKAGE, "utf8")) as {
    scripts?: Record<string, string>;
  };
  const expected: Record<string, string> = {
    prepare: "node .github/tools/src/scripts/install-hooks.ts",
    "worktree:setup": "node .github/tools/src/scripts/setup-worktree.ts",
    "plugin:setup": CLI_COMMAND,
    "plugin:status": `${CLI_COMMAND} --status`,
    "plugin:teardown": `${CLI_COMMAND} --teardown`,
    "plugin:validate": `${CLI_COMMAND} --validate`,
    "plugin:validate:fast": `${CLI_COMMAND} --validate --fast`,
    "plugin:vscode": `${CLI_COMMAND} --vscode`,
    "test:tools": "npm --prefix .github/tools test",
    "test:tools:coverage": "npm --prefix .github/tools run test:coverage",
    "test:hooks": "go test -C hooks/go ./...",
    "test:typecheck": "npm run typecheck --workspaces --if-present",
    "test:boundaries": "node .github/tools/src/boundaries/cli.ts --repo .",
    "test:mutation": "node .github/tools/src/scripts/mutation-stryker.ts",
    format: "prettier --write .",
    "format:check": "prettier --check .",
    typecheck: "npm run typecheck --workspaces --if-present",
  };
  for (const [name, command] of Object.entries(expected)) {
    assert.equal(pkg.scripts?.[name], command, `scripts.${name}`);
  }
});
