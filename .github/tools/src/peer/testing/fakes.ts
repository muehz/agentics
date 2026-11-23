import { readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { scratchDir } from "./harness.ts";

export interface Fakes {
  dir: string;
  path: (name: string) => string;
}

const templates: Record<string, string> = {
  claude: readFileSync(new URL("./fake-claude.sh", import.meta.url), "utf8"),
  codex: readFileSync(new URL("./fake-codex.sh", import.meta.url), "utf8"),
};

// Installs the fake launchers in a directory of their own. Every test gets its own, so no case resets another's
// state. The fakes write their invocation logs beside themselves.
export function installFakes(): Fakes {
  const dir = scratchDir("agentics-peer-fakes-");
  for (const [name, template] of Object.entries(templates)) {
    writeFileSync(join(dir, name), template.replaceAll("{{DIR}}", dir), {
      mode: 0o755,
    });
  }
  return { dir, path: (name) => join(dir, name) };
}
