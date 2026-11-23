import { lstatSync, statSync } from "node:fs";
import path from "node:path";
import {
  readValues,
  type ValueResult,
  type ValueToken,
} from "../../../release/version.ts";
import { errorMessage, type Failure } from "../../check.ts";
import {
  assertVersionsReadable,
  ifPresent,
  isJsonObject,
  loadJson,
  member,
  pluginList,
  type JsonValue,
} from "./files.ts";

const isAbsent = (value: JsonValue | undefined): value is null | undefined =>
  value === undefined || value === null;

/**
 * Whether two parsed values are equal: a missing member and `null` are equal to each other and to nothing else, arrays
 * compare item by item, and objects by their key sets, whatever the key order. The walk keeps its own stack, so a
 * deeply nested value cannot overflow the call stack.
 */
function jsonEqual(
  left: JsonValue | undefined,
  right: JsonValue | undefined,
): boolean {
  const pairs: Array<[JsonValue | undefined, JsonValue | undefined]> = [
    [left, right],
  ];
  for (let pair = pairs.pop(); pair !== undefined; pair = pairs.pop()) {
    const [a, b] = pair;
    if (isAbsent(a) || isAbsent(b)) {
      if (isAbsent(a) && isAbsent(b)) continue;
      return false;
    }
    if (Array.isArray(a) || Array.isArray(b)) {
      if (!Array.isArray(a) || !Array.isArray(b) || a.length !== b.length)
        return false;
      a.forEach((item, index) => pairs.push([item, b[index]]));
      continue;
    }
    if (isJsonObject(a) || isJsonObject(b)) {
      if (!isJsonObject(a) || !isJsonObject(b)) return false;
      const keys = Object.keys(a);
      if (keys.length !== Object.keys(b).length) return false;
      for (const key of keys) {
        if (!Object.hasOwn(b, key)) return false;
        pairs.push([a[key], b[key]]);
      }
      continue;
    }
    if (a !== b) return false;
  }
  return true;
}

/** Copilot CLI's manifest resolution order; the first candidate that is not a directory is the effective manifest. */
export const COPILOT_MANIFEST_CANDIDATES: readonly string[] = [
  ".plugin/plugin.json",
  "plugin.json",
  ".github/plugin/plugin.json",
  ".claude-plugin/plugin.json",
];

/** The file-system call a discovery uses. Tests inject a failing one to make a candidate fail. */
export type StatFn = (file: string) => { isDirectory(): boolean };

const abs = (root: string, rel: string): string =>
  path.join(root, ...rel.split("/"));

/**
 * The repo-relative path of the first existing candidate, or "" when there is none. A candidate that cannot be examined
 * for any reason but "does not exist" is an error, caused by the candidate's `RepoFileError`.
 */
export function checkedEffectiveCopilotManifest(
  root: string,
  stat: StatFn = statSync,
): string {
  for (const rel of COPILOT_MANIFEST_CANDIDATES) {
    let info: { isDirectory(): boolean } | undefined;
    try {
      info = ifPresent(abs(root, rel), rel, stat);
    } catch (error) {
      throw new Error(`discover Copilot manifest: ${errorMessage(error)}`, {
        cause: error,
      });
    }
    if (info !== undefined && !info.isDirectory()) return rel;
  }
  return "";
}

/** The effective manifest with a discovery error read as none: the repo-relative path, or "" for no manifest. */
export function effectiveCopilotManifest(
  root: string,
  stat: StatFn = statSync,
): string {
  try {
    return checkedEffectiveCopilotManifest(root, stat);
  } catch {
    return "";
  }
}

/**
 * The version token at `file`, or undefined when that token means nothing. An unreadable file and a malformed one carry
 * the same zero token as a manifest whose version is simply missing, so returning the token alone would read two
 * unreadable files as in parity. A `shape` result stays comparable: a missing field and a non-string one are
 * observations.
 */
export function sharedValue(
  values: readonly ValueResult[],
  file: string,
): ValueToken | undefined {
  const found = values.find((value) => value.path === file);
  if (found === undefined || found.kind === "read" || found.kind === "json")
    return undefined;
  return found.value;
}

/**
 * The nine `copilot-*` rules, in their report order. Each failure but the last stops the rules: nothing after it is meaningful.
 * The rules stay silent on a missing or invalid file, which `plugin-json-valid` and `marketplace-json-valid` report, so a
 * caller outside `run` must supply its own presence and validity checks. An audit error is thrown. `lstat` is a parameter
 * only so a test can make the shadow check fail.
 */
export function copilotProviderRules(
  root: string,
  values: readonly ValueResult[],
  stat: StatFn = statSync,
  lstat: StatFn = lstatSync,
): Failure[] {
  assertVersionsReadable(values);
  const failures: Failure[] = [];
  const effective = checkedEffectiveCopilotManifest(root, stat);
  for (const rel of COPILOT_MANIFEST_CANDIDATES.slice(0, 2)) {
    let present: boolean;
    try {
      present = ifPresent(abs(root, rel), rel, lstat) !== undefined;
    } catch (error) {
      throw new Error(
        `inspect Copilot shadow manifest: ${errorMessage(error)}`,
        { cause: error },
      );
    }
    if (present) {
      failures.push({
        rule: "copilot-shadow-manifest",
        path: rel,
        message: `higher-precedence Copilot shadow manifest ${rel} must be absent`,
      });
      return failures;
    }
  }
  if (effective !== COPILOT_MANIFEST_CANDIDATES[2]) {
    failures.push({
      rule: "copilot-effective-manifest",
      path: effective,
      message: "effective Copilot manifest must be .github/plugin/plugin.json",
    });
    return failures;
  }

  const marketplaceRel = ".claude-plugin/marketplace.json";
  const marketplace = loadJson(root, marketplaceRel, "object");
  if (!marketplace.found) {
    failures.push({
      rule: "copilot-marketplace-present",
      path: marketplaceRel,
      message: `${marketplaceRel} must be present`,
    });
    return failures;
  }
  if (!marketplace.ok) return failures; // marketplace-json-valid already fired

  const agenticsEntries = pluginList(marketplace.value).filter(
    (plugin) => member(plugin, "name") === "agentics",
  );
  const entry = agenticsEntries.length === 1 ? agenticsEntries[0] : undefined;
  if (entry === undefined || member(entry, "source") !== "./") {
    failures.push({
      rule: "copilot-marketplace-source",
      path: marketplaceRel,
      message: "agentics marketplace source must be exactly './'",
    });
    return failures;
  }

  const loaded = loadJson(root, effective, "object");
  if (!loaded.found || !loaded.ok) return failures; // plugin-json-valid already fired
  const manifest = loaded.value;

  if (!jsonEqual(member(manifest, "name"), member(entry, "name"))) {
    failures.push({
      rule: "copilot-marketplace-name-parity",
      path: effective,
      message: "effective manifest name must match Copilot marketplace entry",
    });
    return failures;
  }
  const effectiveVersion = sharedValue(values, effective);
  const marketplaceVersion = sharedValue(values, marketplaceRel);
  if (
    effectiveVersion !== undefined &&
    marketplaceVersion !== undefined &&
    (effectiveVersion.set !== marketplaceVersion.set ||
      effectiveVersion.json !== marketplaceVersion.json)
  ) {
    failures.push({
      rule: "copilot-marketplace-version-parity",
      path: effective,
      message:
        "effective manifest version must match Copilot marketplace entry",
    });
    return failures;
  }

  if (member(manifest, "hooks") !== "./hooks/hooks.copilot.json") {
    failures.push({
      rule: "copilot-hooks-field",
      path: effective,
      message: "effective hooks must be exactly './hooks/hooks.copilot.json'",
    });
    return failures;
  }
  let hooksTarget = false;
  try {
    hooksTarget = !statSync(
      abs(root, "hooks/hooks.copilot.json"),
    ).isDirectory();
  } catch {
    // Any error is a failure here, never an audit error.
  }
  if (!hooksTarget) {
    failures.push({
      rule: "copilot-hooks-target",
      path: "hooks/hooks.copilot.json",
      message: "effective hooks target does not exist",
    });
    return failures;
  }

  const claude = loadJson(root, ".claude-plugin/plugin.json", "object");
  if (!claude.found || !claude.ok) return failures;
  // The expected skills are the Claude list, in order. A value that is not an array counts as absent.
  const asList = (raw: JsonValue | undefined): JsonValue[] | undefined =>
    Array.isArray(raw) ? raw : undefined;
  const expected = asList(member(claude.value, "skills"));
  const actual = asList(member(manifest, "skills"));
  const same = jsonEqual(actual, expected);
  if (!same)
    failures.push({
      rule: "copilot-skills-parity",
      path: effective,
      message: "effective skills must match Claude skills",
    });
  return failures;
}

/**
 * The Copilot rules against any root, the repository checkout or an installed plugin tree, reading the version values
 * itself. An audit error becomes the one failure `copilot-audit` whose path is the root. The `hooks` domain calls it.
 */
export function copilotProviderFailures(root: string): Failure[] {
  try {
    return copilotProviderRules(root, readValues(root));
  } catch (error) {
    return [
      { rule: "copilot-audit", path: root, message: errorMessage(error) },
    ];
  }
}
