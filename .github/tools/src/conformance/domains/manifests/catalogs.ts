import { statSync } from "node:fs";
import path from "node:path";
import { errorMessage, type Failure } from "../../check.ts";
import { isLocal } from "../../local-path.ts";
import {
  containedManifestPath,
  ifPresent,
  isJsonObject,
  loadJson,
  MARKETPLACES,
  member,
  PartialAuditError,
  pluginList,
  type Contained,
  type JsonObject,
} from "./files.ts";

const CLAUDE_CATALOG = MARKETPLACES[0];

/** Turns backslashes into slashes on a host whose separator is a backslash; a no-op elsewhere. */
const toSlash = (text: string): string =>
  path.sep === "\\" ? text.replaceAll("\\", "/") : text;

/** Joins two slash-separated elements and cleans the result, leaving no trailing slash. */
function joinClean(a: string, b: string): string {
  const joined = path.posix.join(toSlash(a), toSlash(b));
  return joined.endsWith("/") ? joined.slice(0, -1) : joined;
}

/**
 * `marketplace-present`, `marketplace-json-valid` and `marketplace-entry` over the two catalogs. Returns the catalogs
 * that hold exactly one object named `agentics`, keyed by their repo-relative path; the others are left out of every later
 * step, parity included.
 */
export function catalogRules(root: string): {
  parsed: Map<string, JsonObject>;
  failures: Failure[];
} {
  const failures: Failure[] = [];
  const parsed = new Map<string, JsonObject>();
  for (const rel of MARKETPLACES) {
    const { found, ok, value } = loadJson(root, rel, "object");
    if (!found) {
      failures.push({
        rule: "marketplace-present",
        path: rel,
        message: "missing",
      });
      continue;
    }
    if (!ok) {
      failures.push({
        rule: "marketplace-json-valid",
        path: rel,
        message: "invalid JSON",
      });
      continue;
    }
    const entries = member(value, "plugins");
    const entry =
      Array.isArray(entries) && entries.length === 1 ? entries[0] : undefined;
    if (!isJsonObject(entry) || member(entry, "name") !== "agentics") {
      failures.push({
        rule: "marketplace-entry",
        path: rel,
        message: "plugins must contain exactly one object named agentics",
      });
      continue; // an invalid list cannot provide parity evidence
    }
    parsed.set(rel, value);
  }
  return { parsed, failures };
}

/**
 * `marketplace-source-dir`: every entry's source resolves to an existing directory inside the checkout. Only the Claude
 * catalog honors `metadata.pluginRoot`. An audit error is thrown as a PartialAuditError that carries the failures found
 * so far, which survive it.
 */
export function marketplaceSourceRules(
  root: string,
  parsed: ReadonlyMap<string, JsonObject>,
): Failure[] {
  const failures: Failure[] = [];
  for (const rel of MARKETPLACES) {
    const data = parsed.get(rel);
    if (data === undefined) continue; // presence and validity are already reported
    let pluginRoot = ".";
    if (rel === CLAUDE_CATALOG) {
      const meta = member(data, "metadata");
      const configured = isJsonObject(meta)
        ? member(meta, "pluginRoot")
        : undefined;
      if (typeof configured === "string" && configured !== "")
        pluginRoot = configured;
    }
    for (const plugin of pluginList(data)) {
      const name = member(plugin, "name");
      const source = member(plugin, "source");
      let src = "";
      if (typeof source === "string") src = source;
      else if (isJsonObject(source)) {
        const inner = member(source, "path");
        if (typeof inner === "string") src = inner;
      }
      if (src === "") {
        failures.push({
          rule: "marketplace-source-dir",
          path: rel,
          message: `plugin ${JSON.stringify(typeof name === "string" ? name : "")} has no local source path`,
        });
        continue;
      }
      let contained: Contained;
      let isDirectory = false;
      try {
        contained = containedManifestPath(root, pluginRoot);
        if (contained.kind === "ok" && !isLocal(src))
          contained = {
            kind: "escape",
            message: `path escapes checkout: ${JSON.stringify(src)}`,
          };
        if (contained.kind === "ok") {
          // The containment check gets the joined path with the host separator, so an escape message quotes it that
          // way. The "directory not found" message below quotes it with slashes instead.
          const joined = joinClean(pluginRoot, src);
          contained = containedManifestPath(
            root,
            path.sep === "\\" ? joined.replaceAll("/", "\\") : joined,
          );
        }
        if (contained.kind === "escape") {
          failures.push({
            rule: "marketplace-source-dir",
            path: rel,
            message: contained.message,
          });
          continue;
        }
        if (contained.kind === "ok")
          isDirectory =
            ifPresent(contained.target, joinClean(pluginRoot, src), (file) =>
              statSync(file),
            )?.isDirectory() ?? false;
      } catch (error) {
        throw new PartialAuditError(
          `audit marketplace source ${rel}: ${errorMessage(error)}`,
          failures,
          { cause: error },
        );
      }
      if (!isDirectory)
        failures.push({
          rule: "marketplace-source-dir",
          path: rel,
          message: `${joinClean(pluginRoot, src)} — directory not found`,
        });
    }
  }
  return failures;
}
