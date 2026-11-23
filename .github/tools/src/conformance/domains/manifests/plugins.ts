import { lstatSync, statSync, type Dirent } from "node:fs";
import path from "node:path";
import { errorMessage, type Failure } from "../../check.ts";
import { lazyTrackedLookup } from "../../work-folder-temp.ts";
import { trackedPaths, type Tracked } from "../repo/walk.ts";
import {
  containedManifestPath,
  ifPresent,
  isJsonObject,
  loadJson,
  member,
  PartialAuditError,
  readDirIfPresent,
  referenceRel,
  walkTree,
  type Contained,
  type JsonValue,
} from "./files.ts";

const CLAUDE_MANIFEST = ".claude-plugin/plugin.json";

/** The plugin manifests that install from the checkout root, in the order they are checked. */
const ROOT_MANIFESTS = [
  CLAUDE_MANIFEST,
  ".codex-plugin/plugin.json",
  ".github/plugin/plugin.json",
  ".antigravity-plugin/plugin.json",
] as const;

const abs = (root: string, rel: string): string =>
  path.join(root, ...rel.split("/"));

/** The default sort's order (UTF-16 code units) for two entries' names. */
const byName = (a: Dirent, b: Dirent): number =>
  a.name < b.name ? -1 : a.name > b.name ? 1 : 0;

/** The lazy tracked lookup a rule called on its own uses: git runs with this process's environment, if at all. */
const processTracked = (root: string): (() => Tracked) =>
  lazyTrackedLookup(() => trackedPaths(root, process.env));

/**
 * `plugin-json-valid`: every non-directory named plugin.json in the walk is listed (sorted), and one that is not valid
 * JSON of any kind fails. A file that cannot be found when read (a dangling link) fails too: the rule asks only
 * whether the file loaded.
 * `tracked` decides which files under a work folder's `temp` directory the walk reaches.
 */
export function pluginJsonRules(
  root: string,
  tracked: () => Tracked = processTracked(root),
): {
  found: string[];
  failures: Failure[];
} {
  const failures: Failure[] = [];
  const found: string[] = [];
  walkTree(
    root,
    (rel, entry) => {
      if (entry.isDirectory() || entry.name !== "plugin.json") return;
      found.push(rel);
      if (!loadJson(root, rel, "any").ok)
        failures.push({
          rule: "plugin-json-valid",
          path: rel,
          message: "invalid JSON",
        });
    },
    tracked,
  );
  return { found: found.sort(), failures };
}

/**
 * `plugin-path-references`: the skills, apps, hooks, mcpServers and logo paths the root-installing manifests name exist
 * and are the right kind. An audit error is thrown as a PartialAuditError carrying every failure found so far.
 */
export function pathReferenceRules(root: string): Failure[] {
  const failures: Failure[] = [];
  for (const rel of ROOT_MANIFESTS) {
    const { ok, value: data } = loadJson(root, rel, "object");
    if (!ok) continue; // a missing file is tolerated and a parse error is plugin-json-valid's; neither is ok
    const check = (
      field: string,
      reference: string,
      fileOK: boolean,
      dirOK: boolean,
      suffix: string,
    ): void => {
      const audit = (cause: unknown): PartialAuditError =>
        new PartialAuditError(
          `audit ${rel} ${field} reference: ${errorMessage(cause)}`,
          failures,
          { cause },
        );
      let contained: Contained;
      try {
        contained = containedManifestPath(root, reference);
      } catch (error) {
        throw audit(error);
      }
      if (contained.kind === "escape") {
        failures.push({
          rule: "plugin-path-references",
          path: rel,
          message: `${field} ${contained.message}`,
        });
        return;
      }
      let good = false;
      if (contained.kind === "ok") {
        let info;
        try {
          info = ifPresent(contained.target, referenceRel(reference), (file) =>
            statSync(file),
          );
        } catch (error) {
          throw audit(error);
        }
        if (info !== undefined) good = info.isDirectory() ? dirOK : fileOK;
      }
      if (!good)
        failures.push({
          rule: "plugin-path-references",
          path: rel,
          message: `${field} path not found → ${reference}${suffix}`,
        });
    };
    const text = (raw: JsonValue): string =>
      typeof raw === "string" ? raw : "";
    for (const field of ["skills", "apps"]) {
      const paths = member(data, field);
      if (Array.isArray(paths))
        for (const raw of paths) check(field, text(raw), true, true, "");
      else if (typeof paths === "string")
        check(field, paths, false, true, " (string form must be a directory)");
    }
    const hooks = member(data, "hooks");
    if (typeof hooks === "string") check("hooks", hooks, true, false, "");
    const mcp = member(data, "mcpServers");
    if (mcp !== undefined && mcp !== null)
      for (const raw of Array.isArray(mcp) ? mcp : [mcp])
        check("mcpServers", text(raw), true, false, "");
    const logo = member(data, "logo");
    if (typeof logo === "string") check("logo", logo, true, false, "");
  }
  return failures;
}

/** The last element of a slash path, with trailing slashes removed; "." for an empty path, "/" for slashes only. */
function baseName(name: string): string {
  if (name === "") return ".";
  const trimmed = name.replace(/\/+$/, "");
  if (trimmed === "") return "/";
  return trimmed.slice(trimmed.lastIndexOf("/") + 1);
}

/**
 * `plugin-registers-shipped`: every directory under skills/ is named by the Claude manifest's `skills` list, compared by
 * the last path element, so a new skill cannot ship unregistered.
 */
export function registrationRules(root: string): Failure[] {
  const { found, ok, value: data } = loadJson(root, CLAUDE_MANIFEST, "object");
  if (!found || !ok) return []; // reported by plugin-json-valid
  const skills = member(data, "skills");
  const registered = new Set<string>();
  if (Array.isArray(skills))
    for (const raw of skills)
      registered.add(baseName(typeof raw === "string" ? raw : ""));
  let entries: Dirent[];
  try {
    entries = readDirIfPresent(root, "skills") ?? [];
  } catch (error) {
    throw new Error(`enumerate skills: ${errorMessage(error)}`, {
      cause: error,
    });
  }
  entries.sort(byName);
  const failures: Failure[] = [];
  for (const entry of entries) {
    if (!entry.isDirectory() || registered.has(entry.name)) continue;
    failures.push({
      rule: "plugin-registers-shipped",
      path: CLAUDE_MANIFEST,
      message: `skills/${entry.name}/ ships but no skills entry registers it — consumers never load it`,
    });
  }
  return failures;
}

/** `claude-hooks-field`: the Claude manifest names its hook config explicitly. */
export function claudeProviderRules(root: string): Failure[] {
  const { found, ok, value: data } = loadJson(root, CLAUDE_MANIFEST, "object");
  if (!found || !ok) return [];
  if (member(data, "hooks") === "./hooks/hooks.claude.json") return [];
  return [
    {
      rule: "claude-hooks-field",
      path: CLAUDE_MANIFEST,
      message: "hooks must be ./hooks/hooks.claude.json",
    },
  ];
}

/**
 * `no-mcp-json` over the walk, and `no-mcp-servers` over the plugin.json files the walk listed. `tracked` decides which
 * files under a work folder's `temp` directory the walk reaches.
 */
export function noMcpRules(
  root: string,
  pluginJsons: readonly string[],
  tracked: () => Tracked = processTracked(root),
): Failure[] {
  const failures: Failure[] = [];
  walkTree(
    root,
    (rel, entry) => {
      if (
        !entry.isDirectory() &&
        (entry.name === ".mcp.json" || entry.name === "mcp_config.json")
      )
        failures.push({
          rule: "no-mcp-json",
          path: rel,
          message: "Agentics ships no MCP configuration — remove it",
        });
    },
    tracked,
  );
  for (const rel of pluginJsons) {
    const { ok, value: data } = loadJson(root, rel, "object");
    if (!ok) continue;
    if (truthy(member(data, "mcpServers")))
      failures.push({
        rule: "no-mcp-servers",
        path: rel,
        message: "mcpServers must not be registered in plugin.json",
      });
  }
  return failures;
}

/** Python truthiness: `null`, `false`, `0`, `""`, an empty array and an empty object are false. */
function truthy(value: JsonValue | undefined): boolean {
  if (value === undefined || value === null) return false;
  if (Array.isArray(value)) return value.length > 0;
  if (isJsonObject(value)) return Object.keys(value).length > 0;
  return value !== false && value !== 0 && value !== "";
}

/** `no-claude-md`: either path existing, a symlink included, hides AGENTS.md from Claude Code. */
export function claudeMdRule(root: string): Failure[] {
  const failures: Failure[] = [];
  for (const rel of ["CLAUDE.md", ".claude/CLAUDE.md"]) {
    try {
      lstatSync(abs(root, rel));
    } catch {
      continue; // any error reads as absent
    }
    failures.push({
      rule: "no-claude-md",
      path: rel,
      message:
        "AGENTS.md is canonical; Claude Code reads this file instead of AGENTS.md, so remove it",
    });
  }
  return failures;
}
