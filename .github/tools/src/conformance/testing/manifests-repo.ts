import {
  mkdirSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import type { AuditResult } from "../check.ts";
import { run } from "../domains/manifests/index.ts";
import { writeFile } from "./files.ts";
import { fixtureGitEnv } from "./git-checkout.ts";
import { removeTree } from "./host.ts";

export { writeFile };

/** What `auditManifests` can vary. */
export interface ManifestsAuditOptions {
  /** The inherited environment for the fixture's tracked-files lookup. */
  env?: NodeJS.ProcessEnv;
}

/**
 * Runs the manifests domain on a checkout with its production steps, as the CLI does without `--short` or `--fast`,
 * and returns its result.
 */
export function auditManifests(
  root: string,
  options: ManifestsAuditOptions = {},
): Promise<AuditResult> {
  return run(root, {
    short: false,
    fast: false,
    diagnostics: () => {},
    env: fixtureGitEnv(options.env),
  });
}

/** The repo-relative path of each file the fixture writes, so a test names one without retyping it. */
export const CLAUDE_CATALOG = ".claude-plugin/marketplace.json";
export const AGENTS_CATALOG = ".agents/plugins/marketplace.json";
export const CLAUDE_PLUGIN = ".claude-plugin/plugin.json";
export const COPILOT_PLUGIN = ".github/plugin/plugin.json";
export const CODEX_PLUGIN = ".codex-plugin/plugin.json";
export const ANTIGRAVITY_PLUGIN = ".antigravity-plugin/plugin.json";
export const COPILOT_HOOKS = "hooks/hooks.copilot.json";

const json = (value: unknown): string => JSON.stringify(value);

/** A fresh checkout that passes every `manifests` rule, for a test to change once and audit. */
export function cloneManifestsRepo(): string {
  const root = mkdtempSync(path.join(tmpdir(), "agentics-manifestsrepo-"));
  writeFile(
    root,
    CLAUDE_CATALOG,
    json({
      name: "muehz",
      metadata: { version: "0.0.0" },
      plugins: [{ name: "agentics", version: "1.0.0", source: "./" }],
    }),
  );
  writeFile(
    root,
    AGENTS_CATALOG,
    json({ plugins: [{ name: "agentics", source: { path: "." } }] }),
  );
  writeFile(
    root,
    CLAUDE_PLUGIN,
    json({
      name: "agentics",
      version: "1.0.0",
      skills: ["./skills/tdd"],
      hooks: "./hooks/hooks.claude.json",
    }),
  );
  writeFile(
    root,
    COPILOT_PLUGIN,
    json({
      name: "agentics",
      version: "1.0.0",
      skills: ["./skills/tdd"],
      hooks: "./hooks/hooks.copilot.json",
    }),
  );
  writeFile(
    root,
    CODEX_PLUGIN,
    json({
      name: "agentics",
      version: "1.0.0",
      skills: "./skills/",
      hooks: "./hooks/hooks.codex.json",
    }),
  );
  writeFile(
    root,
    ANTIGRAVITY_PLUGIN,
    json({
      name: "agentics",
      version: "1.0.0",
      logo: "assets/logo.svg",
    }),
  );
  writeFile(root, "assets/logo.svg", "<svg></svg>\n");
  mkdir(root, "skills/tdd");
  writeFile(root, "hooks/run-hook.cmd", "REM polyglot dispatcher\n");
  writeFile(
    root,
    "hooks/hooks.claude.json",
    json({
      hooks: {
        PreToolUse: [
          {
            hooks: [
              {
                type: "command",
                command:
                  '"${CLAUDE_PLUGIN_ROOT}/hooks/run-hook.cmd" claude-code command-guardian --mode block',
                timeout: 10,
              },
            ],
          },
        ],
      },
    }),
  );
  writeFile(
    root,
    COPILOT_HOOKS,
    json({
      version: 1,
      hooks: {
        preToolUse: [
          {
            type: "command",
            bash: '"${COPILOT_PLUGIN_ROOT}/hooks/run-hook.cmd" copilot command-guardian --mode block',
            powershell:
              '& "$env:COPILOT_PLUGIN_ROOT\\hooks\\run-hook.cmd" copilot command-guardian --mode block',
          },
        ],
      },
    }),
  );
  writeFile(
    root,
    "hooks/hooks.codex.json",
    json({
      hooks: {
        preToolUse: [
          {
            type: "command",
            command: '"${PLUGIN_ROOT}/hooks/run-hook.cmd" codex pre-tool',
            commandWindows:
              'cmd /c "%PLUGIN_ROOT%\\hooks\\run-hook.cmd" codex pre-tool',
          },
        ],
      },
    }),
  );
  writeFile(root, "AGENTS.md", "# Agentics\n");
  mkdir(root, ".claude");
  return root;
}

const absolute = (root: string, rel: string): string =>
  path.join(root, ...rel.split("/"));

/** Creates a directory (and its parents) inside a fixture. */
export function mkdir(root: string, rel: string): void {
  mkdirSync(absolute(root, rel), { recursive: true });
}

/** Removes a file or a directory tree from a fixture; nothing happens when it is already gone. */
export function remove(root: string, rel: string): void {
  rmSync(absolute(root, rel), { recursive: true, force: true });
}

/** Changes one JSON file of a fixture: parses it, lets `edit` mutate the value, and writes it back. */
export function editJson(
  root: string,
  rel: string,
  edit: (value: Record<string, unknown>) => void,
): void {
  const value = JSON.parse(readFileSync(absolute(root, rel), "utf8")) as Record<
    string,
    unknown
  >;
  edit(value);
  writeFileSync(absolute(root, rel), json(value));
}

const firstEntry = (value: Record<string, unknown>): Record<string, unknown> =>
  (value.plugins as Record<string, unknown>[])[0] ?? {};

/**
 * For each rule the CLI can report, one change to a passing fixture that makes it fire. Some changes fire a second
 * rule as well (removing the Claude catalog also fails the catalog's presence rule); the tests that use this table
 * say which.
 */
export const BREAKERS: Readonly<Record<string, (root: string) => void>> = {
  "marketplace-present": (root) => remove(root, AGENTS_CATALOG),
  "marketplace-json-valid": (root) => writeFile(root, AGENTS_CATALOG, "{"),
  "marketplace-entry": (root) =>
    writeFile(root, AGENTS_CATALOG, '{"plugins":[]}'),
  "plugin-json-valid": (root) => writeFile(root, "docs/plugin.json", "{"),
  "marketplace-source-dir": (root) =>
    editJson(root, AGENTS_CATALOG, (value) => {
      firstEntry(value).source = { path: "missing" };
    }),
  "plugin-path-references": (root) =>
    editJson(root, CODEX_PLUGIN, (value) => {
      value.skills = "./nope/";
    }),
  "plugin-registers-shipped": (root) => mkdir(root, "skills/extra"),
  "claude-hooks-field": (root) =>
    editJson(root, CLAUDE_PLUGIN, (value) => {
      value.hooks = "./hooks/hooks.codex.json";
    }),
  "no-mcp-json": (root) => writeFile(root, ".mcp.json", "{}"),
  "no-mcp-servers": (root) =>
    writeFile(root, "docs/plugin.json", '{"mcpServers":{"a":1}}'),
  "marketplace-version-parity": (root) =>
    editJson(root, AGENTS_CATALOG, (value) => {
      firstEntry(value).version = "2.0.0";
    }),
  "provider-version-parity": (root) =>
    editJson(root, CODEX_PLUGIN, (value) => {
      value.version = "2.0.0";
    }),
  "no-claude-md": (root) => writeFile(root, "CLAUDE.md", "# x\n"),
  "copilot-shadow-manifest": (root) => writeFile(root, "plugin.json", "{}"),
  "copilot-effective-manifest": (root) => remove(root, COPILOT_PLUGIN),
  "copilot-marketplace-present": (root) => remove(root, CLAUDE_CATALOG),
  "copilot-marketplace-source": (root) =>
    editJson(root, CLAUDE_CATALOG, (value) => {
      firstEntry(value).source = ".";
    }),
  "copilot-marketplace-name-parity": (root) =>
    editJson(root, COPILOT_PLUGIN, (value) => {
      value.name = "other";
    }),
  "copilot-marketplace-version-parity": (root) =>
    editJson(root, COPILOT_PLUGIN, (value) => {
      value.version = "2.0.0";
    }),
  "copilot-hooks-field": (root) =>
    editJson(root, COPILOT_PLUGIN, (value) => {
      value.hooks = "./hooks/hooks.codex.json";
    }),
  "copilot-hooks-target": (root) => remove(root, COPILOT_HOOKS),
  "copilot-skills-parity": (root) =>
    editJson(root, COPILOT_PLUGIN, (value) => {
      value.skills = [];
    }),
};

/** Runs `body` on a fresh passing fixture and removes it afterwards, whatever happens. */
export async function withManifestsRepo<T>(
  body: (root: string) => Promise<T> | T,
): Promise<T> {
  const root = cloneManifestsRepo();
  try {
    return await body(root);
  } finally {
    removeTree(root);
  }
}
