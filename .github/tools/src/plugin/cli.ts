import { parseArgs } from "./args.ts";
import { checkNodeFloor } from "./floor.ts";
import { setupLocal, showStatus, teardownLocal } from "./marketplace.ts";
import { runValidate } from "./validate.ts";
import { showVscode } from "./vscode.ts";

/**
 * Entry file for the maintainer CLI. It runs unconditionally, with no
 * `import.meta.main` gate: that property is missing on some supported Node
 * releases, and a gated entry would exit 0 without doing anything there.
 */
async function main(argv: readonly string[]): Promise<number> {
  const parsed = parseArgs(argv);
  if (parsed.kind === "exit") {
    for (const line of parsed.lines) {
      console.log(line);
    }
    return parsed.code;
  }
  if (!checkNodeFloor({ action: parsed.action })) {
    return 1;
  }
  // --validate acts on the repository with or without --vscode.
  if (parsed.action === "validate") {
    return runValidate({ mode: "strict", speed: parsed.mode });
  }
  // Setup, status and teardown have the same answer for VS Code, so one block serves all three.
  if (parsed.vscode !== "") {
    return showVscode(parsed.vscode);
  }
  switch (parsed.action) {
    case "setup":
      return setupLocal();
    case "teardown":
      return teardownLocal();
    case "status":
      return showStatus();
  }
}

// The exit code is set rather than exiting, so buffered output is flushed before the process ends.
process.exitCode = await main(process.argv.slice(2));
