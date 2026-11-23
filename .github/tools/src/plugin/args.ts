/**
 * Argument parsing for the maintainer CLI. Pure: it reads no environment and
 * performs no I/O, so every branch is testable from an argument list.
 */

export const CLI_COMMAND = "node .github/tools/src/plugin/cli.ts";

export type Action = "setup" | "teardown" | "validate" | "status";
export type VscodeVariant = "" | "code" | "insiders";
export type ValidateMode = "full" | "fast";

export type ParsedArgs =
  | {
      kind: "run";
      action: Action;
      vscode: VscodeVariant;
      mode: ValidateMode;
    }
  | { kind: "exit"; code: number; lines: string[] };

/** The help text, one entry per output line. */
export function helpText(): string[] {
  return [
    `Usage: ${CLI_COMMAND} [--vscode|--vscode-insiders] [--teardown|--validate [--fast]|--status|--help]`,
    "",
    "  (no args)              Register local checkout as marketplace and install plugins (CLI)",
    "  --teardown             Remove local plugins, reinstall from remote",
    "  --validate             Run the TypeScript conformance suite and the tools tests",
    "  --fast                 With --validate: run only the checkout checks; skip the Go and TypeScript test suites",
    "  --status               Show installed plugins and marketplaces",
    "  --vscode               Print the VS Code settings to add, check, and remove",
    "  --vscode-insiders      Same, for VS Code Insiders",
    "  --help                 Show this help",
    "",
    "VS Code is configured by hand. Agentics prints the three settings and the",
    "path to write them to; it never edits your settings.json. The printed",
    "block covers setup, checking, and removal, so --vscode ignores",
    "--teardown and --status.",
    "",
    "Every mode needs Node in the engines.node range of package.json. --validate also needs Go (it builds only the trusted hook verifier).",
  ];
}

/**
 * Parse the CLI arguments. The last action flag wins, an unknown option stops
 * parsing at once, and `--fast` is valid only when the winning action is
 * `--validate`.
 */
export function parseArgs(argv: readonly string[]): ParsedArgs {
  let action: Action | "help" = "setup";
  let vscode: VscodeVariant = "";
  let mode: ValidateMode = "full";
  const usageHint = `Run ${CLI_COMMAND} --help for usage.`;

  for (const arg of argv) {
    switch (arg) {
      case "--vscode":
        vscode = "code";
        break;
      case "--vscode-insiders":
        vscode = "insiders";
        break;
      case "--teardown":
        action = "teardown";
        break;
      case "--validate":
        action = "validate";
        break;
      case "--fast":
        mode = "fast";
        break;
      case "--status":
        action = "status";
        break;
      case "--help":
      case "-h":
        action = "help";
        break;
      default:
        return {
          kind: "exit",
          code: 1,
          lines: [`Unknown option: ${arg}`, usageHint],
        };
    }
  }

  if (action === "help") {
    return { kind: "exit", code: 0, lines: helpText() };
  }
  if (mode === "fast" && action !== "validate") {
    return {
      kind: "exit",
      code: 1,
      lines: ["--fast needs --validate.", usageHint],
    };
  }
  return { kind: "run", action, vscode, mode };
}
