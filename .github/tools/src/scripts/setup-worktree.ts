#!/usr/bin/env node
// Codex setup command on macOS, Linux, and Windows:
// node .github/tools/src/scripts/setup-worktree.ts
// Requires Node/npm, Git (Git for Windows on Windows), and Go on PATH.
// Plugin installs are machine-wide; run the maintainer CLI (node .github/tools/src/plugin/cli.ts) separately to select a checkout.
import { spawnSync } from "node:child_process";
import { accessSync, constants, readFileSync } from "node:fs";
import path from "node:path";
import { parseRange, satisfies } from "./node-range.ts";

const root = path.resolve(import.meta.dirname, "../../../..");

function run(
  command: string,
  args: string[],
  cwd = root,
  capture = false,
): string | undefined {
  const result = spawnSync(command, args, {
    cwd,
    stdio: capture ? "pipe" : "inherit",
    encoding: "utf8",
  });
  if (result.error)
    throw new Error(`Cannot run ${command}: ${result.error.message}`);
  if (result.status !== 0)
    throw new Error(
      `${command} failed (${result.signal || result.status}). ${result.stderr || ""}`,
    );
  return result.stdout?.trim();
}

function npm(args: string[], capture = false): string | undefined {
  // npm.cmd needs cmd.exe on Windows. Only fixed commands enter the shell;
  // the checkout path is passed as cwd, so spaces need no shell quoting.
  if (process.platform === "win32")
    return run(
      "cmd.exe",
      ["/d", "/s", "/c", `npm.cmd ${args.join(" ")}`],
      root,
      capture,
    );
  return run("npm", args, root, capture);
}

// The install command needs one concrete version, so engines.npm is limited
// to a single >=X.Y.Z floor.
function npmFloor(range: string): string {
  const parts = parseRange(range);
  const [only] = parts;
  if (parts.length !== 1 || only?.operator !== ">=")
    throw new Error(`Unsupported npm range "${range}": use >=X.Y.Z`);
  return only.minimum.join(".");
}

try {
  // This is the checked-in manifest, whose engine field is validated by the
  // repository conformance suite; it is not an external payload.
  const manifest: { engines: { node: string; npm: string } } = JSON.parse(
    readFileSync(path.join(root, "package.json"), "utf8"),
  );
  const required = manifest.engines.node;
  if (!satisfies(required, process.versions.node))
    throw new Error(
      `Node ${required} required; found ${process.versions.node}`,
    );
  run("git", ["--version"]);
  // Checked before any download or install so an old npm changes nothing.
  const npmVersion = npm(["--version"], true) ?? "";
  if (!/^\d+\.\d+\.\d+$/.test(npmVersion))
    throw new Error(`npm --version printed "${npmVersion}", not a version`);
  if (!satisfies(manifest.engines.npm, npmVersion))
    throw new Error(
      `npm ${manifest.engines.npm} required; found ${npmVersion}\n` +
        `Upgrade it with: npm install --global npm@${npmFloor(manifest.engines.npm)}`,
    );
  for (const module of ["hooks/go"]) {
    console.log(`Preparing ${module}`);
    // Run inside each module so Go selects its toolchain from go.mod.
    run("go", ["mod", "download"], path.join(root, module));
  }
  console.log("Installing pinned development tools and Git hooks");
  // Explicit installation prevents npm's ignore-scripts setting from silently
  // skipping hook activation and preserves configuration failures.
  npm(["ci", "--include=dev", "--ignore-scripts"]);
  run(process.execPath, [
    ".github/tools/src/scripts/install-hooks.ts",
    "--required",
  ]);
  accessSync(
    path.join(root, ".github/husky/_/pre-commit"),
    process.platform === "win32" ? constants.F_OK : constants.X_OK,
  );
  if (
    run("git", ["config", "--get", "core.hooksPath"], root, true) !==
    ".github/husky/_"
  )
    throw new Error(
      "Git hooks were not activated. Check HUSKY and npm ignore-scripts settings.",
    );
  console.log("Agentics worktree dependencies and Git hooks are ready.");
  console.log("For full conformance, run: npm run plugin:validate");
} catch (error) {
  console.error(
    `Setup failed: ${error instanceof Error ? error.message : String(error)}`,
  );
  console.error("Check the prerequisites in CONTRIBUTING.md, then retry.");
  process.exitCode = 1;
}
