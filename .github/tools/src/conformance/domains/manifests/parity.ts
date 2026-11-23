import type { ValueResult } from "../../../release/version.ts";
import type { Failure } from "../../check.ts";
import {
  loadJson,
  MARKETPLACES,
  member,
  pluginList,
  type JsonObject,
} from "./files.ts";

const CLAUDE_MANIFEST = ".claude-plugin/plugin.json";

/** The provider manifests compared with the Claude one, in the order the report lists them. */
const PROVIDER_ORDER = [
  ".github/plugin/plugin.json",
  ".codex-plugin/plugin.json",
  ".antigravity-plugin/plugin.json",
];

/** The plugin names of a catalog, and the non-empty string version of each plugin that has one. */
function namesAndVersions(data: JsonObject): {
  names: Set<string>;
  versions: Map<string, string>;
} {
  const names = new Set<string>();
  const versions = new Map<string, string>();
  for (const plugin of pluginList(data)) {
    const rawName = member(plugin, "name");
    const name = typeof rawName === "string" ? rawName : "";
    names.add(name);
    const version = member(plugin, "version");
    if (typeof version === "string" && version !== "")
      versions.set(name, version);
  }
  return { names, versions };
}

/**
 * The versions of a catalog's plugins. The Claude catalog's `agentics` version is the one the version reader found there,
 * the first result for that path with a non-empty value, so it is the value the release tooling sees.
 */
function marketplaceVersions(
  data: JsonObject,
  rel: string,
  values: readonly ValueResult[],
): Map<string, string> {
  if (rel !== MARKETPLACES[0]) return namesAndVersions(data).versions;
  const versions = new Map<string, string>();
  for (const plugin of pluginList(data)) {
    const rawName = member(plugin, "name");
    const name = typeof rawName === "string" ? rawName : "";
    if (name === "agentics") continue;
    const version = member(plugin, "version");
    if (typeof version === "string" && version !== "")
      versions.set(name, version);
  }
  const read = values.find((value) => value.path === rel && value.raw !== "");
  if (read !== undefined) versions.set("agentics", read.raw);
  return versions;
}

/** Names as a comma-separated list of JSON strings, so an empty name stays visible. */
const list = (names: string[]): string =>
  names.map((name) => JSON.stringify(name)).join(", ");

const difference = (a: ReadonlySet<string>, b: ReadonlySet<string>): string[] =>
  [...a].filter((name) => !b.has(name)).sort();

/**
 * `marketplace-name-parity` and `marketplace-version-parity`: the second parsed catalog against the first. A differing
 * name set ends that comparison. Because a catalog is parsed only with one `agentics` entry, the name rule cannot fire
 * through `run`; it stays as a guard and is tested directly.
 */
export function marketplaceParity(
  parsed: ReadonlyMap<string, JsonObject>,
  values: readonly ValueResult[],
): Failure[] {
  const failures: Failure[] = [];
  const refRel = MARKETPLACES[0];
  const reference = parsed.get(refRel);
  if (reference === undefined) return failures;
  const refNames = namesAndVersions(reference).names;
  const refVersions = marketplaceVersions(reference, refRel, values);
  for (const rel of MARKETPLACES.slice(1)) {
    const data = parsed.get(rel);
    if (data === undefined) continue;
    const names = namesAndVersions(data).names;
    const versions = marketplaceVersions(data, rel, values);
    const missing = difference(refNames, names);
    const extra = difference(names, refNames);
    if (missing.length > 0 || extra.length > 0) {
      const parts: string[] = [];
      if (missing.length > 0) parts.push(`missing plugins: ${list(missing)}`);
      if (extra.length > 0) parts.push(`extra plugins: ${list(extra)}`);
      failures.push({
        rule: "marketplace-name-parity",
        path: rel,
        message: `${parts.join("; ")} (reference: ${refRel})`,
      });
      continue; // versions are compared only when the names match
    }
    // Sorted so the report does not depend on the order the catalog lists its plugins in.
    for (const name of [...refVersions.keys()].sort()) {
      const version = refVersions.get(name) ?? "";
      const other = versions.get(name);
      if (other !== undefined && other !== version)
        failures.push({
          rule: "marketplace-version-parity",
          path: rel,
          message: `${name} version ${other} != ${version} (reference: ${refRel})`,
        });
    }
  }
  return failures;
}

/**
 * `provider-manifest-present` and `provider-version-parity`: the Claude manifest exists and is the version the Codex,
 * Copilot and Antigravity manifests carry. The version comes from the version reader's result for each path.
 */
export function providerParity(
  root: string,
  values: readonly ValueResult[],
): Failure[] {
  const failures: Failure[] = [];
  const claude = loadJson(root, CLAUDE_MANIFEST, "object");
  if (!claude.found)
    failures.push({
      rule: "provider-manifest-present",
      path: CLAUDE_MANIFEST,
      message: "missing",
    });
  if (!claude.found || !claude.ok) return failures;
  const canonical =
    values.find((value) => value.path === CLAUDE_MANIFEST)?.raw ?? "";
  if (canonical === "")
    failures.push({
      rule: "provider-version-parity",
      path: CLAUDE_MANIFEST,
      message: "has no version",
    });
  for (const rel of PROVIDER_ORDER) {
    const value = values.find((candidate) => candidate.path === rel);
    if (value === undefined) continue;
    if (value.kind === "read") {
      failures.push({
        rule: "provider-version-parity",
        path: rel,
        message: "is missing",
      });
      continue;
    }
    if (value.kind === "json") continue; // plugin-json-valid already fired
    if (value.raw !== canonical)
      failures.push({
        rule: "provider-version-parity",
        path: rel,
        message: `version ${JSON.stringify(value.raw)} != ${CLAUDE_MANIFEST} version ${JSON.stringify(canonical)}`,
      });
  }
  return failures;
}

/** The parity rules in report order: the catalogs, then the provider manifests. */
export function parityRules(
  root: string,
  parsed: ReadonlyMap<string, JsonObject>,
  values: readonly ValueResult[],
): Failure[] {
  return [
    ...marketplaceParity(parsed, values),
    ...providerParity(root, values),
  ];
}
