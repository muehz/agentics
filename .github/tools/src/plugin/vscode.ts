import path from "node:path";
import { CLI_COMMAND, type VscodeVariant as ArgsVariant } from "./args.ts";
import { MARKETPLACE_NAME, REPO_ROOT, type Env } from "./common.ts";

/**
 * VS Code is configured by hand: Agentics prints the three settings and the path to write them to, and the maintainer
 * edits the file. Printing replaced a JSONC settings engine, and Agentics never writes a maintainer's settings.json, so
 * it can never damage one.
 */

export type VscodeVariant = Exclude<ArgsVariant, "">;

/** An empty value counts as unset, as Bash `${VAR:-...}` does. */
function nonEmpty(value: string | undefined): string | undefined {
  return value === undefined || value === "" ? undefined : value;
}

/**
 * Resolves the user-level settings.json for the variant: `Code` or `Code - Insiders` under the platform's user
 * configuration directory. Returns `undefined` on any platform other than win32, darwin and linux.
 */
export function vscodeSettingsPath(
  variant: VscodeVariant,
  env: Env = process.env,
  platform: NodeJS.Platform = process.platform,
): string | undefined {
  const dirName = variant === "code" ? "Code" : "Code - Insiders";
  const home = nonEmpty(env["HOME"]) ?? "~";
  switch (platform) {
    case "darwin":
      return `${home}/Library/Application Support/${dirName}/User/settings.json`;
    case "linux":
      return `${nonEmpty(env["XDG_CONFIG_HOME"]) ?? `${home}/.config`}/${dirName}/User/settings.json`;
    case "win32":
      return path.win32.join(
        nonEmpty(env["APPDATA"]) ?? "%APPDATA%",
        dirName,
        "User",
        "settings.json",
      );
    default:
      return undefined;
  }
}

/** The repository path as it appears in the snippet: forward slashes on Windows, so the snippet is valid JSON. */
export function repositoryKey(
  root: string,
  platform: NodeJS.Platform = process.platform,
): string {
  return platform === "win32" ? root.replaceAll("\\", "/") : root;
}

/** The lines of the block that covers setup, status and teardown, which all have the same answer. */
export function vscodeInstructions(
  variant: VscodeVariant,
  env: Env = process.env,
  platform: NodeJS.Platform = process.platform,
  root: string = REPO_ROOT,
): string[] {
  const label = variant === "code" ? "VS Code" : "VS Code Insiders";
  const key = repositoryKey(root, platform);
  const settings = vscodeSettingsPath(variant, env, platform);
  return [
    `=== ${label}: local plugin registration ===`,
    "",
    ...(settings === undefined
      ? [
          `⚠ Cannot determine the ${label} settings path on this platform.`,
          "  Open the file with: Preferences: Open User Settings (JSON)",
        ]
      : ["Settings file:", `  ${settings}`]),
    "",
    "Add these three keys to that file:",
    "",
    '  "chat.pluginLocations": {',
    `    "${key}": true`,
    "  },",
    `  "chat.plugins.marketplaces": ["${MARKETPLACE_NAME}/agentics"],`,
    '  "chat.plugins.enabled": true',
    "",
    `To check the registration, confirm "${key}" is present under`,
    "chat.pluginLocations and set to true.",
    "",
    "To remove it, delete that one entry from chat.pluginLocations.",
    "Leave chat.plugins.marketplaces and chat.plugins.enabled in place —",
    "they are the remote configuration, and remote installs resolve",
    "through them.",
    "",
    "💡 Validate the checkout first with:",
    `   ${CLI_COMMAND} --validate`,
    "",
  ];
}

/** Prints the block and returns the exit code, always 0. */
export function showVscode(variant: VscodeVariant): number {
  for (const line of vscodeInstructions(variant)) {
    console.log(line);
  }
  return 0;
}
