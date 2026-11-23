import { mkdtempSync, readFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { MARKER, type AuditResult } from "../check.ts";
import { run } from "../domains/docs/index.ts";
import { LADDER_PATH, PIN_PATH } from "../domains/docs/ladder.ts";
import { writeFile } from "./files.ts";
import { removeTree } from "./host.ts";

export { writeFile };

/** The ledger as the fixture writes it. Every row is four columns, reviewed on 2026-08-09 except codex (2026-08-01). */
export const LEDGER_HEADER = [
  "# Surface upstream review ledger",
  "",
  "| Surface | Agentics doc | Upstream requirement sources | Last reviewed |",
  "| ------- | --------- | ---------------------------- | ------------- |",
];

export function ledgerRow(
  name: string,
  date: string,
  source = `[docs](https://example.com/${name})`,
): string {
  return `| ${name} | [${name}.md](${name}.md) | ${source} | ${date} |`;
}

export const LADDER = [
  "| Surface | Lower to higher model position |",
  "| --- | --- |",
  "| Codex | `gpt-6-luna` → `gpt-6.1-sol` |",
  "| Cursor | `Composer 2.5` → `Grok 4.7` |",
  "",
].join("\n");

export const PIN_ROW = "| `gpt-5.6-luna` | `hooks/go/pin.go` | fixture pin |\n";

export const PINS = [
  "| Identifier | Used in | Reason |",
  "| --- | --- | --- |",
  PIN_ROW,
].join("\n");

/** Fixed model-id sites, independent of the production list. */
export const FIXED_FILES = [
  ".github/tools/src/peer/models.ts",
  "evals/README.md",
  "evals/skills.yaml",
  ".github/workflows/release.md",
  ".github/workflows/backlog-grooming.md",
] as const;

/** Go roots scanned for model IDs, independent of the production list. */
export const GO_ROOTS = ["hooks/go"] as const;

export interface AuditOptions {
  maxAge?: number;
  now?: Date;
  env?: Record<string, string | undefined>;
}

/** Runs the docs domain on a checkout and returns its result. */
export function auditDocs(
  root: string,
  options: AuditOptions = {},
): Promise<AuditResult> {
  const {
    maxAge = -1,
    now = new Date("2026-09-13T12:00:00Z"),
    env = {},
  } = options;
  return run(root, {
    short: false,
    fast: false,
    diagnostics: () => {},
    env: { AGX_LEDGER_MAX_AGE_DAYS: String(maxAge), ...env },
    now: () => now,
  });
}

/**
 * A stand-in for an agentics checkout that passes every `docs` rule: four skills (the ladder lives in one) of which two are registered with
 * Copilot, the roster claims that say so, a four-surface ledger with a stamped doc per row, a model ladder and pin
 * table, and a scanned copy of every identifier. A test that sees a rule fire therefore knows what it changed.
 */
export function cloneDocsRepo(): string {
  const root = mkdtempSync(path.join(tmpdir(), "agentics-docsrepo-"));
  writeFile(root, MARKER, "{}");
  for (const skill of ["alpha", "beta", "gamma"])
    writeFile(root, `skills/${skill}/SKILL.md`, `# ${skill}\n`);
  writeFile(
    root,
    ".github/plugin/plugin.json",
    JSON.stringify({ skills: ["skills/alpha", "skills/beta"] }),
  );
  writeFile(
    root,
    "docs/ARCHITECTURE.md",
    "The canonical 4-skill roster lives in skills/.\nThe canonical 4-skill tree is shared.\nCopilot CLI ships all 2 skills.\n",
  );
  writeFile(
    root,
    "docs/surfaces/README.md",
    [
      ...LEDGER_HEADER,
      ledgerRow("claude-code", "2026-08-09"),
      ledgerRow("codex", "2026-08-01"),
      ledgerRow("copilot-cli", "2026-08-09"),
      ledgerRow("cursor", "2026-08-09"),
      "",
    ].join("\n"),
  );
  writeFile(
    root,
    "docs/surfaces/claude-code.md",
    "# Claude Code\n\nThe manifest registers all 4 skills.\n\nVerified on Claude Code 2.1.220 (2026-08-09).\n",
  );
  writeFile(
    root,
    "docs/surfaces/codex.md",
    "# Codex\n\nRuns on `gpt-6-luna`; the release engine stays on gpt-5.6-luna.\n\nVerified on codex-cli 0.147.0 (2026-08-01).\n",
  );
  writeFile(
    root,
    "docs/surfaces/copilot-cli.md",
    "# Copilot CLI\n\nThe manifest registers all 2 Agentics skills.\n\nVerified on Copilot CLI 1.0.78 (2026-08-09).\n",
  );
  writeFile(
    root,
    "docs/surfaces/cursor.md",
    "# Cursor\n\nOrder: `Composer 2.5` → `Grok\n4.7`.\n\nVerified on Cursor 3.15.6 (2026-08-09).\n",
  );
  writeFile(root, LADDER_PATH, LADDER);
  writeFile(root, PIN_PATH, PINS);
  for (const rel of FIXED_FILES) writeFile(root, rel, "fixture\n");
  writeFile(
    root,
    "hooks/go/pin.go",
    'package tools\n\n// gpt-9-nova in a comment is history, not a default.\nconst pin = "gpt-5.6-luna"\n',
  );
  writeFile(root, "hooks/go/hook.go", "package hooks\n");
  return root;
}

/** Runs `body` on a fresh passing fixture and removes it afterwards, whatever happens. */
export async function withDocsRepo<T>(
  body: (root: string) => Promise<T> | T,
): Promise<T> {
  const root = cloneDocsRepo();
  try {
    return await body(root);
  } finally {
    removeTree(root);
  }
}

/** The text of a fixture file. */
export function readFixture(root: string, rel: string): string {
  return readFileSync(path.join(root, ...rel.split("/")), "utf8");
}
