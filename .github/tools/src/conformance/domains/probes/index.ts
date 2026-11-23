import {
  mkdirSync,
  mkdtempSync,
  readFileSync,
  realpathSync,
  rmSync,
  statSync,
} from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import {
  errorMessage,
  type AuditResult,
  type Failure,
  type RunOptions,
} from "../../check.ts";
import { copilotProviderFailures } from "../manifests/copilot.ts";
import { findOnPath, realExec } from "../tools/exec.ts";

/** Options for the opt-in provider install audit. */
export interface ProbeOptions extends RunOptions {
  allowMissing?: boolean;
}

/** Process boundary injected by tests so they never install or invoke a real provider. */
export interface ProbeContext {
  lookPath(
    command: string,
    env: Record<string, string | undefined>,
  ): string | undefined;
  execute(
    command: string,
    args: string[],
    options: { env: Record<string, string>; cwd: string },
  ):
    | { status: number | null; output: string }
    | Promise<{ status: number | null; output: string }>;
}

const CONTEXT: ProbeContext = {
  lookPath: findOnPath,
  async execute(command, args, options) {
    const result = await realExec({
      bin: command,
      args,
      dir: options.cwd,
      env: options.env,
    });
    return {
      status: result.code,
      output: result.output + (result.spawnError ?? ""),
    };
  },
};

function probeEnvironment(
  env: Record<string, string | undefined>,
  home: string,
  cache: string,
): Record<string, string> {
  const result: Record<string, string> = {
    COPILOT_HOME: home,
    COPILOT_CACHE_HOME: cache,
  };
  for (const name of [
    "HOME",
    "LANG",
    "LC_ALL",
    "LC_CTYPE",
    "LOGNAME",
    "PATH",
    "SHELL",
    "TERM",
    "TMPDIR",
    "TZ",
    "USER",
    "__CF_USER_TEXT_ENCODING",
    "GH_TOKEN",
    "GITHUB_TOKEN",
    "XDG_CONFIG_HOME",
    "APPDATA",
    "LOCALAPPDATA",
  ])
    if (env[name] !== undefined) result[name] = env[name];
  return result;
}

function canonical(file: string): string {
  const absolute = path.resolve(file);
  try {
    return realpathSync(absolute);
  } catch {
    try {
      return path.join(
        realpathSync(path.dirname(absolute)),
        path.basename(absolute),
      );
    } catch {
      return absolute;
    }
  }
}

function installRoot(output: string, home: string, root: string): string {
  let entries: {
    name?: string;
    marketplace?: string;
    source?: string;
    installedFrom?: string;
  }[];
  try {
    let value: unknown;
    try {
      value = JSON.parse(output);
    } catch (error) {
      throw new Error(`invalid JSON (${errorMessage(error)})`);
    }
    if (!Array.isArray(value) && value !== null)
      throw new Error("expected an array");
    entries = ((value ?? []) as unknown[]).map((entry) => {
      if (entry !== null && (typeof entry !== "object" || Array.isArray(entry)))
        throw new Error("expected plugin objects");
      const fields = {
        name: "",
        marketplace: "",
        source: "",
        installedFrom: "",
      };
      if (entry === null) return fields;
      for (const field of Object.keys(fields) as (keyof typeof fields)[]) {
        if (!Object.hasOwn(entry, field)) continue;
        const member: unknown = (entry as Record<string, unknown>)[field];
        if (member === null) continue;
        if (typeof member !== "string")
          throw new Error(`plugin field ${field} must be a string`);
        fields[field] = member;
      }
      return fields;
    });
  } catch (error) {
    throw new Error(
      `copilot plugin list --json output unparseable: ${errorMessage(error)}`,
    );
  }
  for (const entry of entries) {
    if (entry.name !== "agentics" || entry.marketplace !== "muehz") continue;
    if (entry.source !== "live")
      return path.join(home, "installed-plugins", "muehz", "agentics");
    const installedFrom =
      typeof entry.installedFrom === "string" ? entry.installedFrom : "";
    let same = false;
    try {
      const a = statSync(installedFrom),
        b = statSync(root);
      same = a.dev === b.dev && a.ino === b.ino;
    } catch {
      /* An absent live root cannot identify the audited checkout. */
    }
    if (!installedFrom || !same)
      throw new Error(
        `live install loads ${JSON.stringify(installedFrom)}, not the audited checkout ${root}`,
      );
    return installedFrom;
  }
  throw new Error("copilot plugin list --json has no agentics@muehz entry");
}

function readMap(file: string): Record<string, unknown> {
  const value: unknown = JSON.parse(readFileSync(file, "utf8"));
  if (value === null) return {};
  if (typeof value !== "object" || Array.isArray(value))
    throw new Error("expected a JSON object");
  return value as Record<string, unknown>;
}

function skillSet(manifest: Record<string, unknown>): Set<string> {
  return new Set(
    Array.isArray(manifest.skills)
      ? manifest.skills
          .filter((skill): skill is string => typeof skill === "string")
          .map((skill) => skill.replace(/^\.\//, ""))
      : [],
  );
}
function difference(a: Set<string>, b: Set<string>): string[] {
  return [...a].filter((value) => !b.has(value)).sort();
}
function equal(a: Set<string>, b: Set<string>): boolean {
  return a.size === b.size && difference(a, b).length === 0;
}

function installedTree(
  installed: string,
  output: string,
  root: string,
): Failure[] {
  const failures: Failure[] = [];
  const fail = (rule: string, rel: string, message: string) =>
    failures.push({ rule, path: rel, message });
  for (const rel of [
    ".github/plugin/plugin.json",
    ".claude-plugin/marketplace.json",
    ".claude-plugin/plugin.json",
  ]) {
    let text: string;
    try {
      text = readFileSync(path.join(installed, rel), "utf8");
    } catch (error) {
      if (rel === ".claude-plugin/plugin.json")
        fail(
          "probe-copilot-invariants",
          rel,
          `installed ${rel} is missing or unreadable: ${errorMessage(error)}`,
        );
      continue;
    }
    try {
      JSON.parse(text);
    } catch (error) {
      fail(
        "probe-copilot-invariants",
        rel,
        `installed ${rel} is not valid JSON: ${errorMessage(error)}`,
      );
    }
  }
  for (const failure of copilotProviderFailures(installed))
    fail(
      "probe-copilot-invariants",
      path.isAbsolute(failure.path) ? "" : failure.path,
      `installed tree violates ${failure.rule}: ${failure.message}`,
    );
  const rel = ".github/plugin/plugin.json";
  const skillFail = (message: string) =>
    fail("probe-copilot-skills", rel, message);
  let manifest: Record<string, unknown>, source: Record<string, unknown>;
  try {
    manifest = readMap(path.join(installed, rel));
  } catch (error) {
    skillFail(`installed ${rel} unreadable: ${errorMessage(error)}`);
    return failures;
  }
  try {
    source = readMap(path.join(root, rel));
  } catch (error) {
    skillFail(`source ${rel} unreadable: ${errorMessage(error)}`);
    return failures;
  }
  let listed: Record<string, unknown>[];
  try {
    const value: unknown = JSON.parse(output);
    if (
      value !== null &&
      (!Array.isArray(value) ||
        value.some(
          (entry) =>
            entry === null || typeof entry !== "object" || Array.isArray(entry),
        ))
    )
      throw new Error("expected an array of skill objects");
    listed = (value ?? []) as Record<string, unknown>[];
  } catch (error) {
    skillFail(
      `copilot skill list --json output unparseable: ${errorMessage(error)}`,
    );
    return failures;
  }
  const expectedSkills = skillSet(source),
    installedSkills = skillSet(manifest);
  if (!expectedSkills.size) {
    skillFail(`${root}/${rel} declares no skills`);
    return failures;
  }
  if (!equal(expectedSkills, installedSkills))
    skillFail(
      `installed manifest does not match source manifest; missing=${JSON.stringify(difference(expectedSkills, installedSkills))}, extra=${JSON.stringify(difference(installedSkills, expectedSkills))}`,
    );
  const expectedPaths = new Set(
    [...installedSkills].map((skill) => canonical(path.join(installed, skill))),
  );
  if (expectedPaths.size !== expectedSkills.size)
    skillFail(
      `expected effective installed manifest to declare exactly ${expectedSkills.size} skills, got ${expectedPaths.size}`,
    );
  const agenticsSkills = listed.filter(
    (skill) =>
      skill.source === "plugin" &&
      skill.enabled === true &&
      path.dirname(
        canonical(typeof skill.path === "string" ? skill.path : ""),
      ) === canonical(path.join(installed, "skills")),
  );
  const actualPaths = new Set(
    agenticsSkills.map((skill) => canonical(skill.path as string)),
  );
  if (agenticsSkills.length !== expectedSkills.size)
    skillFail(
      `expected exactly ${expectedSkills.size} enabled installed Agentics plugin skills, got ${agenticsSkills.length}`,
    );
  if (!equal(actualPaths, expectedPaths))
    skillFail(
      `installed skill paths do not match effective manifest; missing=${JSON.stringify(difference(expectedPaths, actualPaths))}, extra=${JSON.stringify(difference(actualPaths, expectedPaths))}`,
    );
  if (manifest.hooks !== "./hooks/hooks.copilot.json") {
    fail(
      "probe-copilot-hooks",
      rel,
      `effective installed hooks must be ./hooks/hooks.copilot.json, got ${JSON.stringify(manifest.hooks ?? "")}`,
    );
  } else {
    try {
      if (statSync(path.join(installed, manifest.hooks)).isDirectory())
        throw new Error("directory");
    } catch {
      fail(
        "probe-copilot-hooks",
        rel,
        "effective installed hooks target does not exist",
      );
    }
  }
  return failures;
}

/** Audits a provider install in isolated homes. Missing CLIs are errors unless explicitly allowed. */
export async function run(
  repoRoot: string,
  options: ProbeOptions,
  context: ProbeContext = CONTEXT,
): Promise<AuditResult> {
  let temporary: string | undefined;
  try {
    const executable = context.lookPath("copilot", options.env);
    if (!executable) {
      if (options.allowMissing) {
        options.diagnostics("SKIP: copilot is not installed (allow-missing)\n");
        return { failures: [] };
      }
      return {
        failures: [],
        error:
          "probes: copilot is required; pass allow-missing to skip unavailable provider CLIs",
      };
    }
    temporary = mkdtempSync(
      path.join(tmpdir(), "agentics-provider-install-probes."),
    );
    const home = path.join(temporary, "copilot-home"),
      cache = path.join(temporary, "copilot-cache");
    mkdirSync(home, { recursive: true });
    mkdirSync(cache, { recursive: true });
    const env = probeEnvironment(options.env, home, cache);
    let pluginOutput = "",
      skillsOutput = "",
      installed = "";
    const steps = [
      ["plugin", "marketplace", "add", repoRoot],
      ["plugin", "install", "agentics@muehz"],
      ["plugin", "list", "--json"],
      ["skill", "list", "--json"],
    ];
    for (const args of steps) {
      const result = await context.execute(executable, args, {
        env,
        cwd: process.cwd(),
      });
      if (result.status !== 0) {
        const command =
          args[0] === "plugin" && args[1] === "marketplace"
            ? "plugin marketplace add"
            : args.join(" ");
        return {
          failures: [
            {
              rule:
                args[0] === "skill"
                  ? "probe-copilot-skills"
                  : "probe-copilot-install",
              path: "",
              message: `copilot ${command} failed:\n${result.output.replace(/\n+$/, "").split("\n").slice(-20).join("\n")}`,
            },
          ],
        };
      }
      if (args[0] === "plugin" && args[1] === "list") {
        pluginOutput = result.output;
        try {
          installed = installRoot(pluginOutput, home, repoRoot);
        } catch (error) {
          return {
            failures: [
              {
                rule: "probe-copilot-install",
                path: "",
                message: errorMessage(error),
              },
            ],
          };
        }
        try {
          if (!statSync(installed).isDirectory())
            throw new Error("not a directory");
        } catch {
          return {
            failures: [
              {
                rule: "probe-copilot-install",
                path: "",
                message: `Copilot installed root not found: ${installed}`,
              },
            ],
          };
        }
      }
      if (args[0] === "skill") skillsOutput = result.output;
    }
    return { failures: installedTree(installed, skillsOutput, repoRoot) };
  } catch (error) {
    return { failures: [], error: `probes: ${errorMessage(error)}` };
  } finally {
    if (temporary) rmSync(temporary, { recursive: true, force: true });
  }
}
