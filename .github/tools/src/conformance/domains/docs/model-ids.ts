import { lstatSync, readdirSync, readFileSync } from "node:fs";
import path from "node:path";
import { type AuditResult, errorMessage, type Failure } from "../../check.ts";
import { goStringLiterals } from "../../go-source/lexer.ts";
import { collapseWhitespace } from "../../text.ts";
import { RepoFileError, repoFileErrorOf } from "../../repo-file-error.ts";
import { isKnown, type Ladder, loadLadder, PIN_PATH } from "./ladder.ts";

/**
 * The fixed copies the model-ids area reads beside docs/surfaces/*.md and the non-test Go source.
 * .github/tools/src/peer/models.ts is the peer dev tool's model-pin file. The docs/specs history, captured hook payloads, frozen
 * eval banks, and the generated *.lock.yml files stay out on purpose: none of them is a copy a rename should rewrite.
 */
export const MODEL_ID_FILES: readonly string[] = [
  ".github/tools/src/peer/models.ts",
  "evals/README.md",
  "evals/skills.yaml",
  ".github/workflows/release.md",
  ".github/workflows/backlog-grooming.md",
];

/** The Go module whose non-test string literals are read. Comments and _test.go files record history, not defaults. */
export const MODEL_ID_GO_ROOTS: readonly string[] = ["hooks/go"];

/**
 * The identifier families the ladder uses: API identifiers and display names. Claude Code's bare aliases are ordinary
 * words in prose and are not matched. The leading group stands in for a word boundary, which a hyphenated token has no
 * regex escape for. A hyphen counts as a boundary so a shell default such as ${MODEL:-gpt-9-nova} is read.
 */
const MODEL_ID =
  /(?:^|[^A-Za-z0-9_.])(gpt-\d+(?:\.\d+)*(?:-[a-z][a-z0-9]*)*|claude-(?:opus|sonnet|fable|haiku)-\d+(?:[.-]\d+)*|grok-\d+(?:\.\d+)*|gemini-\d+(?:\.\d+)*(?:-[a-z][a-z0-9]*)*|GPT-\d+(?:\.\d+)*|(?:Composer|Grok|Claude (?:Opus|Sonnet|Fable|Haiku)) \d+(?:\.\d+)*)/g;

/**
 * Each distinct identifier in text once, in order. Whitespace runs collapse first, so a display name a formatter
 * wrapped across lines still matches.
 */
export function findModelIds(text: string): string[] {
  const ids: string[] = [];
  for (const match of collapseWhitespace(text).matchAll(MODEL_ID)) {
    const id = match[1] ?? "";
    if (!ids.includes(id)) ids.push(id);
  }
  return ids;
}

/** Runs `read` on the absolute path of the repo-relative `rel`, turning a failure into a `RepoFileError`. */
function onRel<T>(repoRoot: string, rel: string, read: (abs: string) => T): T {
  try {
    return read(path.join(repoRoot, ...rel.split("/")));
  } catch (error) {
    throw repoFileErrorOf(error, rel);
  }
}

const listDir = (repoRoot: string, rel: string) =>
  onRel(repoRoot, rel, (dir) =>
    readdirSync(dir, { withFileTypes: true }).sort((a, b) =>
      a.name < b.name ? -1 : a.name > b.name ? 1 : 0,
    ),
  );

/**
 * Walks `rel` under the checkout in lexical order, skipping every `testdata` directory, and calls `visit` with the
 * repo-relative slash path of each file. Symlinks are files, never followed as directories. A failure to read the
 * tree throws a `RepoFileError`.
 */
function walk(
  repoRoot: string,
  rel: string,
  visit: (fileRel: string) => void,
): void {
  const stat = onRel(repoRoot, rel, (abs) => lstatSync(abs));
  if (!stat.isDirectory()) {
    visit(rel);
    return;
  }
  for (const entry of listDir(repoRoot, rel)) {
    if (entry.isDirectory()) {
      if (entry.name !== "testdata")
        walk(repoRoot, `${rel}/${entry.name}`, visit);
    } else visit(`${rel}/${entry.name}`);
  }
}

/** The surface docs: every `*.md` entry of docs/surfaces, the ledger included, as a repo-relative path. */
function surfaceDocPaths(repoRoot: string): string[] {
  try {
    return readdirSync(path.join(repoRoot, "docs", "surfaces"))
      .filter((name) => name.endsWith(".md"))
      .sort()
      .map((name) => `docs/surfaces/${name}`);
  } catch {
    // A directory that cannot be listed selects no doc, which the caller reports as a missing site.
    return [];
  }
}

/**
 * Reports every model identifier in the scanned copies that neither the ladder's order table nor the maintainers' pin
 * table names (model-ids-known), and every pin row whose identifier no scanned copy names (model-ids-pin-used).
 */
export function modelIdRules(repoRoot: string): AuditResult {
  let ladder: Ladder;
  try {
    ladder = loadLadder(repoRoot);
  } catch (error) {
    return { failures: [], error: `docs: ${errorMessage(error)}` };
  }
  const failures: Failure[] = [];
  const reported = new Set<string>();
  const seen = new Set<string>();
  const audit = (rel: string, text: string) => {
    for (const id of findModelIds(text)) {
      seen.add(id);
      if (!isKnown(ladder, id) && !reported.has(`${rel}\u0000${id}`)) {
        reported.add(`${rel}\u0000${id}`);
        failures.push({
          rule: "model-ids-known",
          path: rel,
          message: `names model identifier ${JSON.stringify(id)}, which is on no ladder row and has no pin row in ${PIN_PATH}; add a pin row with its reason, or correct this copy`,
        });
      }
    }
  };
  const missing = (rel: string, why: string) => {
    failures.push({
      rule: "model-ids-site-present",
      path: rel,
      message: `${why}; model-ids-known cannot audit it`,
    });
  };

  const surfaceDocs = surfaceDocPaths(repoRoot);
  if (surfaceDocs.length === 0)
    missing("docs/surfaces", "holds no surface doc");
  for (const rel of [...surfaceDocs, ...MODEL_ID_FILES]) {
    let text: string;
    try {
      text = onRel(repoRoot, rel, (abs) => readFileSync(abs, "utf8"));
    } catch (error) {
      const fileError = repoFileErrorOf(error, rel);
      if (fileError.kind === "missing") {
        missing(rel, "is missing");
        continue;
      }
      return { failures, error: `docs: ${fileError.message}` };
    }
    audit(rel, text);
  }

  for (const root of MODEL_ID_GO_ROOTS) {
    let sources = 0;
    try {
      walk(repoRoot, root, (rel) => {
        const name = rel.slice(rel.lastIndexOf("/") + 1);
        if (!name.endsWith(".go") || name.endsWith("_test.go")) return;
        const source = onRel(repoRoot, rel, (abs) => readFileSync(abs, "utf8"));
        let literals: string[];
        try {
          literals = goStringLiterals(source);
        } catch (error) {
          throw new Error(`${rel}: ${errorMessage(error)}`);
        }
        sources += 1;
        for (const literal of literals) audit(rel, literal);
      });
    } catch (error) {
      if (error instanceof RepoFileError && error.kind === "missing") {
        missing(root, "is missing");
        continue;
      }
      return {
        failures,
        error: `docs: scanning ${root}: ${errorMessage(error)}`,
      };
    }
    if (sources === 0) missing(root, "holds no non-test Go source");
  }

  // A pin allowlists its identifier everywhere, so a row whose last copy was renamed or removed would keep a stale
  // identifier green.
  for (const id of [...ladder.pins.keys()].sort()) {
    if (!seen.has(id))
      failures.push({
        rule: "model-ids-pin-used",
        path: PIN_PATH,
        message: `pins model identifier ${JSON.stringify(id)}, which no file model-ids-known reads names; delete the pin row, or restore the copy it records`,
      });
  }
  return { failures };
}
