import type { KeyedFailure } from "./baseline.ts";
import {
  isBoolean,
  isMap,
  isSeq,
  printable,
  stringText,
  type FrontmatterNode,
} from "./frontmatter.ts";
import type { SkillDeclaration, Surface } from "./skill-meta.ts";

/** The rule reported for a top-level frontmatter key that none of the skill's surfaces documents. */
export const KEY_RULE = "skill-frontmatter-key";

/** The rule reported for a documented frontmatter key whose value breaks the documented rule. */
export const VALUE_RULE = "skill-frontmatter-value";

/**
 * The top-level skill frontmatter keys each surface documents, with the page that documents them and the date that page
 * was read. Harness documentation changes without notice, so update a row only after reading its source again, and set
 * `read` to the date of that reading.
 */
export const SURFACE_DOCS: Readonly<
  Record<Surface, { source: string; read: string; keys: readonly string[] }>
> = {
  antigravity: {
    source: "https://antigravity.google/docs/build-plugins",
    read: "2026-11-20",
    keys: ["name", "description"],
  },
  "claude-code": {
    source: "https://code.claude.com/docs/en/skills",
    read: "2026-11-10",
    keys: [
      "name",
      "description",
      "when_to_use",
      "argument-hint",
      "arguments",
      "disable-model-invocation",
      "user-invocable",
      "allowed-tools",
      "disallowed-tools",
      "model",
      "effort",
      "context",
      "agent",
      "background",
      "hooks",
      "paths",
      "shell",
      "metadata",
      "license",
      "compatibility",
    ],
  },
  vscode: {
    source:
      "https://code.visualstudio.com/docs/copilot/customization/agent-skills",
    read: "2026-11-10",
    keys: [
      "name",
      "description",
      "argument-hint",
      "user-invocable",
      "disable-model-invocation",
      "context",
    ],
  },
  "copilot-cli": {
    source:
      "https://docs.github.com/en/copilot/how-tos/copilot-cli/customize-copilot/add-skills",
    read: "2026-11-10",
    keys: ["name", "description", "license", "allowed-tools"],
  },
  codex: {
    source: "https://learn.chatgpt.com/docs/build-skills",
    read: "2026-11-10",
    keys: ["name", "description"],
  },
};

/** One tool entry: a tool name, optionally followed by a parenthesized pattern. */
const TOOL = /^[A-Za-z][A-Za-z0-9_]*(?:\(.*\))?$/s;

const EFFORTS: ReadonlySet<string> = new Set([
  "low",
  "medium",
  "high",
  "xhigh",
  "max",
]);

const SHELLS: ReadonlySet<string> = new Set(["bash", "powershell"]);

const COMPATIBILITY_MAX = 500;

/**
 * Splits a tool string on spaces and commas outside parentheses, because a pattern such as `Bash(git add *)` holds
 * spaces that belong to the entry.
 */
function splitTools(value: string): string[] {
  const entries: string[] = [];
  let depth = 0;
  let current = "";
  for (const char of value) {
    if (char === "(") depth++;
    else if (char === ")" && depth > 0) depth--;
    if (depth === 0 && (char === "," || /\s/.test(char))) {
      if (current !== "") entries.push(current);
      current = "";
    } else {
      current += char;
    }
  }
  if (current !== "") entries.push(current);
  return entries;
}

/** Whether a node is a tool string or list with at least one entry, each a valid tool entry. */
function toolList(node: FrontmatterNode): boolean {
  let entries: (string | undefined)[];
  const value = stringText(node);
  if (value !== undefined) entries = splitTools(value);
  else if (isSeq(node)) entries = node.items.map(stringText);
  else return false;
  return (
    entries.length > 0 &&
    entries.every((entry) => entry !== undefined && TOOL.test(entry))
  );
}

const TOOLS_RULE = {
  want: "a string or list of Name or Name(pattern) entries, with at least one entry",
  ok: toolList,
};

const BOOLEAN_RULE = { want: "a boolean", ok: isBoolean };

/**
 * The documented value rule for each key that has one. A documented key without a row here takes any value; the
 * frontmatter rules already check `name` and `description`.
 */
export const VALUE_RULES: Readonly<
  Record<string, { want: string; ok: (node: FrontmatterNode) => boolean }>
> = {
  "argument-hint": {
    want: "a non-empty string",
    ok: (node) => (stringText(node)?.trim() ?? "") !== "",
  },
  "user-invocable": BOOLEAN_RULE,
  "disable-model-invocation": BOOLEAN_RULE,
  context: { want: "exactly fork", ok: (node) => stringText(node) === "fork" },
  "allowed-tools": TOOLS_RULE,
  "disallowed-tools": TOOLS_RULE,
  effort: {
    want: `one of ${[...EFFORTS].join(", ")}`,
    ok: (node) => EFFORTS.has(stringText(node) ?? ""),
  },
  shell: {
    want: "bash or powershell",
    ok: (node) => SHELLS.has(stringText(node) ?? ""),
  },
  compatibility: {
    want: `a string of 1 to ${COMPATIBILITY_MAX} characters`,
    ok: (node) => {
      const value = stringText(node);
      if (value === undefined) return false;
      const length = [...value].length;
      return length >= 1 && length <= COMPATIBILITY_MAX;
    },
  },
  metadata: { want: "a map", ok: isMap },
};

/**
 * The findings for each skill's top-level frontmatter keys, checked against the documentation of the surfaces the skill
 * lists. A key that no listed surface documents gives one `KEY_RULE` finding. A documented key whose value breaks its
 * `VALUE_RULES` row gives one `VALUE_RULE` finding for each listed surface that documents it. A skill without parsed
 * frontmatter or without a valid surface gives none, because the frontmatter and metadata rules report it.
 */
export function frontmatterFindings(
  declarations: Iterable<SkillDeclaration>,
): KeyedFailure[] {
  const findings: KeyedFailure[] = [];
  for (const { rel, frontmatter, surfaces } of declarations) {
    if (frontmatter === undefined || surfaces.length === 0) continue;
    for (const { key, value } of frontmatter.pairs) {
      const documenting = surfaces.filter((surface) =>
        SURFACE_DOCS[surface].keys.includes(key),
      );
      if (documenting.length === 0) {
        findings.push({
          rule: KEY_RULE,
          path: rel,
          key: printable(key),
          message: `top-level key "${printable(key)}" is documented by none of this skill's surfaces (${surfaces.join(", ")})`,
        });
        continue;
      }
      const rule = Object.hasOwn(VALUE_RULES, key)
        ? VALUE_RULES[key]
        : undefined;
      if (rule === undefined || rule.ok(value)) continue;
      for (const surface of documenting)
        findings.push({
          rule: VALUE_RULE,
          path: rel,
          key: `${key}: ${surface}`,
          message: `${key} must be ${rule.want} on ${surface} (${SURFACE_DOCS[surface].source})`,
        });
    }
  }
  return findings;
}
