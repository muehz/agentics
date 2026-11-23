import type { Failure } from "../../check.ts";
import { FenceState } from "../../markdown-fence.ts";
import { isFileErrorOfKind } from "./files.ts";
import { SkillsTreeModel } from "./tree-model.ts";
import type { Family } from "./index.ts";

export const SHELL_DATA_RULE = "skill-shell-data";
export const SHELL_DATA_STALE_RULE = "skill-shell-data-stale";
const REVIEWED_LIST = ".github/tools/src/conformance/skill-shell-reviewed.json";

const HUMAN_OPERATOR_FILES = new Map([
  [
    "skills/ops/references/runbook-template.md",
    "An operator fills out the runbook.",
  ],
  [
    "skills/ops/references/communication-templates.md",
    "An operator fills out the incident messages.",
  ],
  [
    "skills/gate/references/change/change-record-template.md",
    "An operator fills out the change record.",
  ],
]);
const HUMAN_OPERATOR_EXAMPLE_TREES = new Map([
  [
    "skills/ops/references/examples/",
    "Operators consult these completed runbook examples.",
  ],
]);

export const DATA_PLACEHOLDERS = [
  "branch",
  "base-branch",
  "head-branch",
  "layer-branch",
  "recovery-branch",
  "base-ref",
  "ref",
  "remote",
  "push-url",
  "tag",
  "artifact-tag",
  "name",
  "file",
  "path",
  "staged-path",
  "other-path",
  "title",
  "subject",
  "body",
  "message",
] as const;

const PLACEHOLDER = new RegExp(`<(${DATA_PLACEHOLDERS.join("|")})>`, "g");
const SHELL_LABELS = new Set([
  "",
  "bash",
  "sh",
  "shell",
  "zsh",
  "powershell",
  "pwsh",
  "console",
]);
const LIST_MARKER = /^(?:[-*+]|[0-9]{1,9}[.)])(?: {1,4}|$)/;
const PROSE_BREAK =
  /^(?:=+|-+|(?:\*[ \t]*){3,}|(?:-[ \t]*){3,}|(?:_[ \t]*){3,})$/;
const INLINE = /(?<![`\\])(`+)(?!`)(.+?)(?<!`)\1(?!`)/gs;
const INLINE_COMMAND = /^(?:git|gh|agx) /;

interface Fence {
  state: FenceState;
  base: number;
  indent: number;
  label: string;
  quoteDepth: number;
}

interface Heredoc {
  delimiter: string;
  quoted: boolean;
  stripTabs: boolean;
}

// Recognize redirects outside ordinary shell quotes and escapes. The command text itself is never evaluated.
const SHELL_TOKENS =
  /(?<!<)<<(?!<)(-?)[ \t]*(?:'([^']+)'|"([^"\\]+)"|([A-Za-z0-9_][A-Za-z0-9_.-]*))|\\.|'[^']*'|"(?:\\.|[^"\\])*"|(?<!\S)#.*$/g;

function heredocs(line: string): Heredoc[] {
  const result: Heredoc[] = [];
  for (const match of line.matchAll(SHELL_TOKENS)) {
    const [, strip, single, double, bare] = match;
    const delimiter = single ?? double ?? bare;
    if (delimiter !== undefined)
      result.push({
        delimiter,
        quoted: bare === undefined,
        stripTabs: strip === "-",
      });
  }
  return result;
}

interface SourceLine {
  number: number;
  key: string;
  heading: string;
}

interface CommandLine extends SourceLine {
  commands: string[];
}

interface ProseLine extends SourceLine {
  content: string;
}

function* flushInlineCommands(buffer: ProseLine[]): Iterable<CommandLine> {
  if (buffer.length === 0) return;
  const lines = buffer.splice(0);
  const text = lines.map((line) => line.content).join("\n");
  const byLine = lines.map((): string[] => []);
  let firstLine = 0;
  let lineOffset = 0;
  for (const match of text.matchAll(INLINE)) {
    const pieces = match[2]!.split("\n");
    if (!INLINE_COMMAND.test(pieces.join(" ").trim())) continue;
    const bodyStart = match.index + match[1]!.length;
    // Locate the opening line by its offset in the paragraph, then keep each piece on its source line.
    while (bodyStart > lineOffset + lines[firstLine]!.content.length) {
      lineOffset += lines[firstLine]!.content.length + 1;
      firstLine++;
    }
    for (const [offset, piece] of pieces.entries()) {
      byLine[firstLine + offset]!.push(piece);
    }
  }
  for (const [index, { number, key, heading }] of lines.entries()) {
    const commands = byLine[index]!;
    if (commands.length > 0) yield { number, key, heading, commands };
  }
}

function quoteBody(line: string, limit = Infinity): [string, number] {
  if (limit === 0) return [line, 0];
  let body = line.trimStart();
  let depth = 0;
  while (depth < limit && body.startsWith(">")) {
    depth++;
    body = body.slice(body[1] === " " ? 2 : 1);
    if (depth < limit && /^ {0,3}>/.test(body)) body = body.trimStart();
  }
  return [depth === 0 ? line : body, depth];
}

function indentColumns(line: string): number {
  let column = 0;
  for (const char of line) {
    if (char === " ") column++;
    else if (char === "\t") column += 4 - (column % 4);
    else break;
  }
  return column;
}

function dedent(line: string, columns: number): string {
  let column = 0;
  let index = 0;
  for (; column < columns; index++) {
    if (line[index] === " ") column++;
    else if (line[index] === "\t") column += 4 - (column % 4);
    else break;
  }
  return " ".repeat(Math.max(0, column - columns)) + line.slice(index);
}

function* commandLines(text: string): Iterable<CommandLine> {
  let heading = "";
  let fence: Fence | undefined;
  let pending: Heredoc[] = [];
  let previousQuoteDepth = 0;
  const listColumns: number[] = [];
  const prose: ProseLine[] = [];
  for (const [index, original] of text.split(/\r?\n/).entries()) {
    let [line, quoteDepth] = quoteBody(original, fence?.quoteDepth);
    if (
      fence &&
      (quoteDepth < fence.quoteDepth ||
        (line.trim() !== "" && indentColumns(line) < fence.base))
    ) {
      fence = undefined;
      pending = [];
      [line, quoteDepth] = quoteBody(original);
    }
    const key = original.trim();
    if (fence === undefined) {
      if (quoteDepth !== previousQuoteDepth) {
        yield* flushInlineCommands(prose);
        listColumns.length = 0;
      }
      previousQuoteDepth = quoteDepth;
      if (line.trim() === "") {
        yield* flushInlineCommands(prose);
        continue;
      }

      const indent = indentColumns(line);
      if ((listColumns.at(-1) ?? 0) > indent) yield* flushInlineCommands(prose);
      while ((listColumns.at(-1) ?? 0) > indent) listColumns.pop();
      let base = listColumns.at(-1) ?? 0;
      if (indent - base <= 3) {
        if (PROSE_BREAK.test(line.trim())) {
          yield* flushInlineCommands(prose);
          continue;
        }
        const marker = LIST_MARKER.exec(line.trimStart())?.[0];
        if (marker) {
          yield* flushInlineCommands(prose);
          base = indent + marker.length;
          listColumns.push(base);
        }
      }
      // A list opener's marker occupies non-whitespace columns on this line only.
      const relative =
        base > indent
          ? line.trimStart().slice(base - indent)
          : dedent(line, base);
      const state = new FenceState();
      const offset = indentColumns(relative);
      if (offset <= 3 && state.step(relative)) {
        yield* flushInlineCommands(prose);
        const info = relative.trim().replace(/^(?:`{3,}|~{3,})/, "");
        fence = {
          state,
          base,
          indent: base + offset,
          label: info.trim().split(/\s+/, 1)[0]!.toLowerCase(),
          quoteDepth,
        };
        continue;
      }
      // An indented line can continue a paragraph, but cannot start one.
      if (offset > 3 && prose.length === 0) continue;
      const title = /^ {0,3}#{1,6}(?:[ \t]+(.*)|$)/.exec(relative);
      if (title !== null) {
        yield* flushInlineCommands(prose);
        heading = (title[1] ?? "").replace(/[ \t]+#+[ \t]*$/, "").trim();
      }
      prose.push({ number: index + 1, key, heading, content: relative });
      if (title !== null) yield* flushInlineCommands(prose);
      continue;
    }
    const relative = dedent(line, fence.base);
    if (indentColumns(relative) <= 3) fence.state.step(relative);
    if (!fence.state.open()) {
      fence = undefined;
      pending = [];
      continue;
    }
    if (!SHELL_LABELS.has(fence.label)) continue;
    const command = dedent(line, fence.indent);
    const current = pending[0];
    if (current) {
      const candidate = current.stripTabs
        ? command.replace(/^\t+/, "")
        : command;
      if (candidate === current.delimiter) {
        pending.shift();
        continue;
      }
      if (current.quoted) continue;
    } else if (fence.label !== "powershell" && fence.label !== "pwsh") {
      pending = heredocs(command);
    }
    yield { number: index + 1, key, heading, commands: [command] };
  }
  yield* flushInlineCommands(prose);
}

function lineFindings(path: string, line: CommandLine): Failure[] {
  return line.commands.flatMap((command) =>
    Array.from(command.matchAll(PLACEHOLDER), (match) => ({
      rule: SHELL_DATA_RULE,
      path,
      message: `line ${line.number} writes ${match[0]} into command text: take it from a producer in the same command, or add a reviewed entry with a reason`,
    })),
  );
}

interface ReviewedLine {
  path: string;
  heading: string;
  key: string;
  count: number;
  reason: string;
}

function identity(path: string, heading: string, key: string): string {
  return JSON.stringify([path, heading, key]);
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function readReviewed(tree: SkillsTreeModel): Map<string, ReviewedLine> {
  let text: string;
  try {
    text = tree.read(REVIEWED_LIST);
  } catch (error) {
    if (isFileErrorOfKind(error, "missing")) return new Map();
    throw error;
  }
  let entries: unknown;
  try {
    entries = JSON.parse(text);
  } catch (error) {
    throw new Error(`skills: ${REVIEWED_LIST}: invalid JSON`, { cause: error });
  }
  if (!Array.isArray(entries))
    throw new Error(`skills: ${REVIEWED_LIST}: expected a JSON array`);
  const reviewed = new Map<string, ReviewedLine>();
  for (const [index, entry] of entries.entries()) {
    const prefix = `skills: ${REVIEWED_LIST}: entry ${index}`;
    if (!isRecord(entry)) throw new Error(`${prefix} must be an object`);
    const { path, heading, key, count, reason } = entry;
    if (typeof reason !== "string" || reason.trim() === "")
      throw new Error(`${prefix} needs a non-empty reason`);
    if (
      typeof path !== "string" ||
      !path.startsWith("skills/") ||
      !path.endsWith(".md") ||
      typeof heading !== "string" ||
      heading !== heading.trim() ||
      typeof key !== "string" ||
      key === "" ||
      key !== key.trim() ||
      typeof count !== "number" ||
      !Number.isSafeInteger(count) ||
      count < 1
    ) {
      throw new Error(
        `${prefix} needs a skill Markdown path, heading, trimmed key and positive count`,
      );
    }
    const id = identity(path, heading, key);
    if (reviewed.has(id))
      throw new Error(`${prefix} duplicates an earlier reviewed line`);
    reviewed.set(id, { path, heading, key, count, reason });
  }
  return reviewed;
}

/** Orders failures by path, then message, as the default sort orders strings. */
function byPathThenMessage(a: Failure, b: Failure): number {
  if (a.path !== b.path) return a.path < b.path ? -1 : 1;
  if (a.message !== b.message) return a.message < b.message ? -1 : 1;
  return 0;
}

/** Reports named data placeholders in skill command templates without invoking a shell. */
export function shellDataRules(
  repoRoot: string,
  tree = new SkillsTreeModel(repoRoot),
): Failure[] {
  const reviewed = readReviewed(tree);
  const failures: Failure[] = [];
  const candidates = new Map<string, Failure[][]>();
  tree.walkText("shell data", (rel, text) => {
    if (
      !rel.endsWith(".md") ||
      HUMAN_OPERATOR_FILES.has(rel) ||
      [...HUMAN_OPERATOR_EXAMPLE_TREES.keys()].some((prefix) =>
        rel.startsWith(prefix),
      )
    )
      return;
    for (const line of commandLines(text)) {
      const found = lineFindings(rel, line);
      if (found.length === 0) continue;
      const id = identity(rel, line.heading, line.key);
      if (!reviewed.has(id)) {
        failures.push(...found);
        continue;
      }
      const lines = candidates.get(id) ?? [];
      lines.push(found);
      candidates.set(id, lines);
    }
  });
  for (const [id, entry] of reviewed) {
    const lines = candidates.get(id) ?? [];
    if (lines.length === entry.count) continue;
    for (const line of lines) failures.push(...line);
    failures.push({
      path: entry.path,
      rule: SHELL_DATA_STALE_RULE,
      message: `reviewed entry for ${JSON.stringify(entry.key)} under ${JSON.stringify(entry.heading)} expects ${entry.count} command line${entry.count === 1 ? "" : "s"}, found ${lines.length}`,
    });
  }
  return failures.sort(byPathThenMessage);
}

export const FAMILY: Family = {
  name: "skill-shell-data",
  rules: [SHELL_DATA_RULE, SHELL_DATA_STALE_RULE],
  run: (root, context) => shellDataRules(root, context?.tree),
};
