/**
 * The claude, copilot, codex, and agy CLIs: setup registers the staged copy of the checkout as a local marketplace on
 * every detected CLI (or installs directly from the stage on agy) and installs the plugins from it, teardown swaps
 * every CLI back to the remote source, and status lists what each CLI has. Every CLI uses the stage, never the
 * checkout; see `stage.ts` for why.
 */
import { spawnSync } from "node:child_process";
import { lstatSync, readdirSync, statSync, writeSync } from "node:fs";
import path from "node:path";
import { CLI_COMMAND } from "./args.ts";
import {
  MARKETPLACE_NAME,
  PLUGINS,
  REPO_ROOT,
  detectClis,
  findCommand,
  spawnPlan,
  type Env,
} from "./common.ts";
import {
  formatSize,
  refreshStage,
  removeStage,
  resolveStageRoot,
  type StageOutput,
} from "./stage.ts";
import { runValidate } from "./validate.ts";

const REMOTE_MARKETPLACE = "muehz/agentics";

/** Runs one CLI command and reports whether it succeeded. */
export type CliRunner = (cli: string, args: readonly string[]) => boolean;

export interface MarketplaceOptions {
  repoRoot?: string;
  env?: Env;
  platform?: NodeJS.Platform;
  /** Receives every line this module and the stage write, newline included. */
  write?: StageOutput;
  /** Runs a CLI command; defaults to resolving it on the environment's PATH and starting it. */
  runner?: CliRunner;
  /** Runs the install-time manifests check; defaults to `runValidate` in setup mode. */
  validate?: () => number;
}

const defaultWrite: StageOutput = (stream, text) => {
  // Synchronous writes keep these lines in order with the output of the CLIs and the validation children.
  writeSync(stream === "stdout" ? 1 : 2, text);
};

/**
 * Starts a CLI the way `cli args 2>/dev/null` does: stdin is inherited and stderr is discarded. Without `write`, stdout
 * is inherited too, so an install shows its progress and prompts as it runs; the module's own lines are synchronous
 * writes to the same descriptor, so the order holds. With `write` (tests), stdout is captured and passed to it before
 * the next message. A command that cannot be resolved, cannot be started, or exits non-zero has failed. On Windows a
 * resolved `.cmd` starts through cmd.exe (`spawnPlan`), because Node refuses to spawn one without a shell; an argument
 * cmd.exe cannot carry fails that command with a message naming it.
 */
export function defaultCliRunner(
  env: Env,
  platform: NodeJS.Platform,
  write?: StageOutput,
): CliRunner {
  return (cli, args) => {
    const file = findCommand(cli, env, platform);
    if (file === undefined) return false;
    let plan;
    try {
      plan = spawnPlan(file, args, platform, env);
    } catch (error) {
      (write ?? defaultWrite)(
        "stderr",
        `❌ ${cli}: ${(error as Error).message}\n`,
      );
      return false;
    }
    const result = spawnSync(plan.file, plan.args, {
      env,
      stdio: ["inherit", write === undefined ? "inherit" : "pipe", "ignore"],
      windowsVerbatimArguments: plan.windowsVerbatimArguments,
      maxBuffer: 64 * 1024 * 1024,
    });
    if (
      write !== undefined &&
      result.stdout != null &&
      result.stdout.length > 0
    )
      write("stdout", result.stdout.toString("utf8"));
    return result.error === undefined && result.status === 0;
  };
}

interface Context {
  repoRoot: string;
  env: Env;
  platform: NodeJS.Platform;
  write: StageOutput;
  out: (line?: string) => void;
  run: CliRunner;
}

function context(options: MarketplaceOptions): Context {
  const env = options.env ?? process.env;
  const platform = options.platform ?? process.platform;
  const write = options.write ?? defaultWrite;
  return {
    repoRoot: options.repoRoot ?? REPO_ROOT,
    env,
    platform,
    write,
    out: (line = "") => write("stdout", `${line}\n`),
    run: options.runner ?? defaultCliRunner(env, platform, options.write),
  };
}

function detected(clis: readonly string[]): string {
  return `CLI(s) detected: ${clis.length > 0 ? clis.join(" ") : "none"}`;
}

/** Registers the stage on every detected CLI and installs the plugins from it. Returns the exit status. */
export async function setupLocal(
  options: MarketplaceOptions = {},
): Promise<number> {
  const c = context(options);
  const { out, run } = c;
  const resolved = resolveStageRoot({
    repoRoot: c.repoRoot,
    env: c.env,
    platform: c.platform,
  });
  if (!resolved.ok) {
    c.write("stderr", `${resolved.message}\n`);
    return 1;
  }
  const clis = detectClis(c.env, c.platform);

  out("=== Local Development Setup ===");
  out(`Repository: ${c.repoRoot}`);
  out(detected(clis));
  out();

  const validate =
    options.validate ??
    (() =>
      runValidate({
        mode: "setup",
        repoRoot: c.repoRoot,
        env: c.env,
        platform: c.platform,
        write: c.write,
      }));
  if (validate() !== 0) return 1;
  out();

  if (clis.length === 0) {
    out("⚠ No supported plugin CLI found in PATH.");
    out();
    out("Install one of:");
    out("  • Claude Code:  brew install claude-code");
    out("  • Copilot CLI:  gh extension install github/gh-copilot");
    out("  • Codex CLI:    npm install -g @openai/codex");
    out("  • Antigravity:  install Antigravity (agy)");
    out();
    out("For VS Code, run:");
    out(`  ${CLI_COMMAND} --vscode`);
    return 1;
  }

  // The stage holds the working directory inside the cache across its awaits, so nothing runs beside it.
  const refreshed = await refreshStage({
    repoRoot: c.repoRoot,
    env: c.env,
    platform: c.platform,
    out: c.write,
  });
  if (refreshed !== 0) return 1;
  out();

  const stage = resolved.stageDir;
  for (const cli of clis) {
    switch (cli) {
      case "claude":
        out("=== Registering local marketplace (Claude Code) ===");
        // Remove before add. On every CLI here, `add` is a no-op when the marketplace name already exists: it does
        // not repoint an existing entry at a new source, so a machine that registered an older source would keep
        // installing from it, silently.
        run("claude", ["plugin", "marketplace", "remove", MARKETPLACE_NAME]);
        if (!run("claude", ["plugin", "marketplace", "add", stage]))
          run("claude", ["plugin", "marketplace", "update", MARKETPLACE_NAME]);
        out();

        out("=== Installing plugins from local checkout (Claude Code) ===");
        for (const plugin of PLUGINS) {
          out(`  Installing ${plugin}...`);
          // `install` no-ops when the plugin is already installed, and `update` acts only on a manifest version
          // bump, so neither refreshes the cache snapshot after a content-only change. Uninstalling first (keeping
          // the plugin's persistent data directory) forces a fresh snapshot.
          const ref = `${plugin}@${MARKETPLACE_NAME}`;
          run("claude", [
            "plugin",
            "uninstall",
            ref,
            "--keep-data",
            "--scope",
            "user",
          ]);
          if (!run("claude", ["plugin", "install", ref, "--scope", "user"]))
            out(`    ⚠ Failed to install ${plugin}`);
        }
        out();
        out(
          "✅ Claude Code setup complete. Run '/reload-plugins' to pick up changes.",
        );
        out();
        out("🔎 Inspect with /plugin or: claude plugin details agentics@muehz");
        out(
          "   /plugin's Errors pane is the first place to look if skills don't appear.",
        );
        out();
        out("💡 For single-plugin iteration without install:");
        out("   claude --plugin-dir .");
        out();
        break;

      case "copilot":
        out("=== Registering local marketplace (Copilot CLI) ===");
        // See the Claude branch: add never repoints an existing entry.
        run("copilot", [
          "plugin",
          "marketplace",
          "remove",
          MARKETPLACE_NAME,
          "--force",
        ]);
        run("copilot", ["plugin", "marketplace", "add", stage]);
        out();

        out("=== Installing plugins from local checkout (Copilot CLI) ===");
        for (const plugin of PLUGINS) {
          out(`  Installing ${plugin}...`);
          if (
            !run("copilot", [
              "plugin",
              "install",
              `${plugin}@${MARKETPLACE_NAME}`,
            ])
          )
            out(`    ⚠ Failed to install ${plugin} (may already be installed)`);
        }
        out();
        out(
          "✅ Copilot CLI setup complete. Restart your Copilot session to use local plugins.",
        );
        out();
        out("🔎 Inspect with: copilot plugin list (or /plugins in-session).");
        out(
          "   No working enable/disable on 1.0.78 — uninstall/reinstall is the management path.",
        );
        out();
        out("💡 For single-plugin direct install:");
        out("   copilot plugin install .");
        out();
        break;

      case "codex":
        // Codex installs Agentics from the repository root through the marketplace entry (source.path "."), which
        // needs Codex CLI 0.140 or later (0.142.2 or later recommended). There is no package sync step.
        out("=== Registering local marketplace (Codex CLI) ===");
        // See the Claude branch: add never repoints an existing entry.
        run("codex", ["plugin", "marketplace", "remove", MARKETPLACE_NAME]);
        run("codex", ["plugin", "marketplace", "add", stage]);
        out();

        out("=== Installing plugins from local checkout (Codex CLI) ===");
        for (const plugin of PLUGINS) {
          out(`  Installing ${plugin}...`);
          if (!run("codex", ["plugin", "add", `${plugin}@${MARKETPLACE_NAME}`]))
            out(`    ⚠ Failed to install ${plugin} (may already be installed)`);
        }
        out();
        out(
          "✅ Codex CLI setup complete. Restart your Codex session to use local plugins.",
        );
        out();
        out(
          "🔎 Inspect and toggle with /plugins (Codex); start a new session after install or re-enable.",
        );
        out(
          "   Skills resolve at session start, so a running session won't see this install.",
        );
        out();
        break;

      case "agy":
        out("=== Installing plugin from local checkout (Antigravity) ===");
        for (const plugin of PLUGINS) {
          out(`  Installing ${plugin}...`);
          run("agy", ["plugin", "uninstall", plugin]);
          if (!run("agy", ["plugin", "install", stage]))
            out(`    ⚠ Failed to install ${plugin}`);
        }
        out();
        out(
          "✅ Antigravity setup complete. Start a new session to use the local plugin.",
        );
        out();
        out("🔎 Inspect with: agy plugin list");
        out();
        break;
    }
  }
  return 0;
}

/** Uninstalls the local plugins, repoints every CLI at the remote marketplace and removes the stage. */
export async function teardownLocal(
  options: MarketplaceOptions = {},
): Promise<number> {
  const c = context(options);
  const { out, run } = c;
  const resolved = resolveStageRoot({
    repoRoot: c.repoRoot,
    env: c.env,
    platform: c.platform,
  });
  if (!resolved.ok) {
    c.write("stderr", `${resolved.message}\n`);
    return 1;
  }
  const clis = detectClis(c.env, c.platform);
  const remove = () =>
    removeStage({
      repoRoot: c.repoRoot,
      env: c.env,
      platform: c.platform,
      out: c.write,
    });

  out("=== Teardown: Removing local plugins ===");
  out(detected(clis));
  out();

  if (clis.length === 0) {
    // The stage is Agentics's own artifact, so it goes even when no CLI is left to unregister it from.
    await remove();
    out("⚠ No CLI found. Remove plugins manually in VS Code.");
    return 1;
  }

  for (const cli of clis) {
    switch (cli) {
      case "claude":
        for (const plugin of PLUGINS) {
          out(`  Uninstalling ${plugin} (Claude Code)...`);
          run("claude", [
            "plugin",
            "uninstall",
            `${plugin}@${MARKETPLACE_NAME}`,
          ]);
        }
        out();
        out("  Removing local marketplace (Claude Code)...");
        run("claude", ["plugin", "marketplace", "remove", MARKETPLACE_NAME]);
        out();

        out("=== Reinstalling from remote (Claude Code) ===");
        run("claude", ["plugin", "marketplace", "add", REMOTE_MARKETPLACE]);
        for (const plugin of PLUGINS) {
          out(`  Installing ${plugin} from remote...`);
          run("claude", [
            "plugin",
            "install",
            `${plugin}@${MARKETPLACE_NAME}`,
            "--scope",
            "user",
          ]);
        }
        out();
        out("✅ Claude Code teardown complete. Run '/reload-plugins'.");
        out();
        break;

      case "copilot":
        for (const plugin of PLUGINS) {
          out(`  Uninstalling ${plugin} (Copilot CLI)...`);
          run("copilot", ["plugin", "uninstall", plugin]);
        }
        out();
        out("  Removing local marketplace (Copilot CLI)...");
        run("copilot", [
          "plugin",
          "marketplace",
          "remove",
          MARKETPLACE_NAME,
          "--force",
        ]);
        out();

        out("=== Reinstalling from remote (Copilot CLI) ===");
        run("copilot", ["plugin", "marketplace", "add", REMOTE_MARKETPLACE]);
        for (const plugin of PLUGINS) {
          out(`  Installing ${plugin} from remote...`);
          run("copilot", [
            "plugin",
            "install",
            `${plugin}@${MARKETPLACE_NAME}`,
          ]);
        }
        out();
        out("✅ Copilot CLI teardown complete. Restart your Copilot session.");
        out();
        break;

      case "codex":
        for (const plugin of PLUGINS) {
          out(`  Uninstalling ${plugin} (Codex CLI)...`);
          run("codex", ["plugin", "remove", plugin]);
        }
        out();
        out("  Removing local marketplace (Codex CLI)...");
        run("codex", ["plugin", "marketplace", "remove", MARKETPLACE_NAME]);
        out();

        out("=== Reinstalling from remote (Codex CLI) ===");
        run("codex", ["plugin", "marketplace", "add", REMOTE_MARKETPLACE]);
        for (const plugin of PLUGINS) {
          out(`  Installing ${plugin} from remote...`);
          run("codex", ["plugin", "add", `${plugin}@${MARKETPLACE_NAME}`]);
        }
        out();
        out("✅ Codex CLI teardown complete. Restart your Codex session.");
        out();
        break;

      case "agy":
        for (const plugin of PLUGINS) {
          out(`  Uninstalling ${plugin} (Antigravity)...`);
          run("agy", ["plugin", "uninstall", plugin]);
        }
        out();

        out("=== Reinstalling from remote (Antigravity) ===");
        for (const plugin of PLUGINS) {
          out(`  Installing ${plugin} from remote...`);
          run("agy", [
            "plugin",
            "install",
            `https://github.com/${REMOTE_MARKETPLACE}`,
          ]);
        }
        out();
        out("✅ Antigravity teardown complete. Start a new session.");
        out();
        break;
    }
  }

  // Last: every CLI above now points at the remote marketplace, so nothing reads the stage any more.
  return remove();
}

/**
 * The bytes in the stage: regular files and links by their `lstat` size, never following a link, which is what the
 * refresh counts, so this agrees with its `Staged …` line. An entry that vanishes meanwhile counts as nothing.
 */
export function stageBytes(stageDir: string): number {
  let total = 0;
  let entries: string[];
  try {
    entries = readdirSync(stageDir, { recursive: true }).map(String);
  } catch {
    return 0;
  }
  for (const entry of entries) {
    try {
      const info = lstatSync(path.join(stageDir, entry));
      if (info.isFile() || info.isSymbolicLink()) total += info.size;
    } catch {
      // Removed while it was counted.
    }
  }
  return total;
}

function isDirectory(value: string): boolean {
  try {
    return statSync(value).isDirectory();
  } catch {
    return false;
  }
}

/** Prints the stage and what every detected CLI lists. Always returns 0: an unusable cache root is reported. */
export function showStatus(options: MarketplaceOptions = {}): number {
  const c = context(options);
  const { out, run } = c;
  const clis = detectClis(c.env, c.platform);

  out("=== Plugin Status ===");
  out(detected(clis));
  const resolved = resolveStageRoot({
    repoRoot: c.repoRoot,
    env: c.env,
    platform: c.platform,
  });
  if (!resolved.ok) out("Marketplace stage: unavailable (invalid cache root)");
  else if (isDirectory(resolved.stageDir))
    out(
      `Marketplace stage: ${resolved.stageDir} (${formatSize(stageBytes(resolved.stageDir))})`,
    );
  else out(`Marketplace stage: ${resolved.stageDir} (not staged)`);
  out();

  if (clis.length === 0) {
    out("⚠ No CLI found.");
    return 0;
  }

  const headers: Record<string, string> = {
    claude: "--- Claude Code ---",
    copilot: "--- Copilot CLI ---",
    codex: "--- Codex CLI ---",
    agy: "--- Antigravity ---",
  };
  for (const cli of clis) {
    out(headers[cli]);
    if (!run(cli, ["plugin", "list"])) out("  (unable to list plugins)");
    out();
    if (cli !== "agy") {
      out("Marketplaces:");
      if (!run(cli, ["plugin", "marketplace", "list"]))
        out("  (unable to list marketplaces)");
      out();
    }
  }
  return 0;
}
