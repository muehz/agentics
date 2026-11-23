import type { KeyedFailure } from "./baseline.ts";
import { SkillsTreeModel } from "./tree-model.ts";
import {
  frontmatterBlock,
  isMap,
  isScalar,
  isSeq,
  lookup,
  parseFrontmatter,
  printable,
  render as renderNode,
  stringText,
  type FrontmatterMap,
  type FrontmatterNode,
} from "./frontmatter.ts";

/** The surfaces a skill may declare in `metadata.surfaces`. A retired surface such as `cursor` is not one of them. */
export const SHIPPED_SURFACES = [
  "antigravity",
  "claude-code",
  "codex",
  "copilot-cli",
  "vscode",
] as const;

/** One shipped surface name. */
export type Surface = (typeof SHIPPED_SURFACES)[number];

/** The rule reported for a malformed value in a skill's `metadata` block. */
export const METADATA_RULE = "skill-metadata";

/**
 * What one skill declares in its frontmatter. Each set holds only the valid values: a malformed or repeated value is
 * reported and left out. `modes` is undefined when the skill declares no `metadata.modes`, so a reference check can tell
 * "no modes" from "an empty or broken list".
 */
export interface SkillDeclaration {
  /** The skill's directory name. */
  skill: string;
  /** The repo-relative path of the skill's `SKILL.md`. */
  rel: string;
  /** The parsed frontmatter, undefined when it is missing, never closed or not a YAML mapping. */
  frontmatter: FrontmatterMap | undefined;
  /** The valid, distinct surfaces in file order. */
  surfaces: Surface[];
  /** The valid modes, undefined when `metadata.modes` is absent. */
  modes: ReadonlySet<string> | undefined;
  /** Each valid alias mapped to the mode it stands for. */
  aliases: ReadonlyMap<string, string>;
  /** The valid flags, each with its leading `--`. */
  flags: ReadonlySet<string>;
}

const WORD = /^[a-z][a-z0-9-]*$/;
const FLAG = /^--[a-z][a-z0-9-]*$/;

const SHIPPED: ReadonlySet<string> = new Set(SHIPPED_SURFACES);

/**
 * How a list item appears in a finding key and message: a scalar as it is written, a collection rendered, both with
 * non-printable characters escaped.
 */
function render(node: FrontmatterNode): string {
  return isScalar(node) ? printable(node.text) : renderNode(node);
}

/** Reads one skill's declaration and appends its findings. */
function declare(
  skill: string,
  rel: string,
  text: string,
  findings: KeyedFailure[],
): SkillDeclaration {
  const declaration: SkillDeclaration = {
    skill,
    rel,
    frontmatter: undefined,
    surfaces: [],
    modes: undefined,
    aliases: new Map(),
    flags: new Set(),
  };
  const { block, closed } = frontmatterBlock(text);
  if (!closed) return declaration;
  let frontmatter: FrontmatterMap;
  try {
    frontmatter = parseFrontmatter(block);
  } catch {
    // The frontmatter rules already report a block that does not parse.
    return declaration;
  }
  declaration.frontmatter = frontmatter;

  const report = (key: string, message: string): void => {
    findings.push({ rule: METADATA_RULE, path: rel, key, message });
  };

  const surfaces = lookup(frontmatter, "metadata", "surfaces");
  if (!isSeq(surfaces) || surfaces.items.length === 0) {
    report(
      "metadata.surfaces",
      `metadata.surfaces must be a non-empty list of shipped surfaces (${SHIPPED_SURFACES.join(", ")}).`,
    );
  } else {
    for (const item of surfaces.items) {
      const value = stringText(item);
      if (value === undefined || !SHIPPED.has(value)) {
        report(
          `metadata.surfaces: ${render(item)}`,
          `metadata.surfaces lists ${render(item)}, which is not a shipped surface; use one of ${SHIPPED_SURFACES.join(", ")}.`,
        );
      } else if (declaration.surfaces.includes(value as Surface)) {
        report(
          `metadata.surfaces: ${value}`,
          `metadata.surfaces lists ${value} more than once; list each surface once.`,
        );
      } else {
        declaration.surfaces.push(value as Surface);
      }
    }
  }

  const modesNode = lookup(frontmatter, "metadata", "modes");
  if (modesNode !== undefined) {
    const modes = new Set<string>();
    declaration.modes = modes;
    if (!isSeq(modesNode)) {
      report(
        "metadata.modes",
        "metadata.modes must be a list of lowercase mode words such as [branch, release].",
      );
    } else {
      for (const item of modesNode.items) {
        const value = stringText(item);
        if (value === undefined || !WORD.test(value)) {
          report(
            `metadata.modes: ${render(item)}`,
            `metadata.modes lists ${render(item)}, which is not a mode word; use lowercase letters, digits and hyphens, starting with a letter.`,
          );
        } else if (modes.has(value)) {
          report(
            `metadata.modes: ${value}`,
            `metadata.modes lists ${value} more than once; list each mode once.`,
          );
        } else {
          modes.add(value);
        }
      }
    }
  }

  const aliasesNode = lookup(frontmatter, "metadata", "mode-aliases");
  if (aliasesNode !== undefined) {
    if (!isMap(aliasesNode)) {
      report(
        "metadata.mode-aliases",
        "metadata.mode-aliases must be a mapping from each alias to a declared mode, such as pr: branch.",
      );
    } else {
      const aliases = new Map<string, string>();
      declaration.aliases = aliases;
      const modes = declaration.modes ?? new Set<string>();
      for (const pair of aliasesNode.pairs) {
        const alias = pair.key;
        const target = stringText(pair.value);
        if (!WORD.test(alias)) {
          report(
            `metadata.mode-aliases: ${printable(alias)}`,
            `metadata.mode-aliases has the alias ${printable(alias)}, which is not a word; use lowercase letters, digits and hyphens, starting with a letter.`,
          );
        } else if (modes.has(alias)) {
          report(
            `metadata.mode-aliases: ${alias}`,
            `metadata.mode-aliases has the alias ${alias}, which is already a mode; an alias must be a different word.`,
          );
        } else if (target === undefined || !modes.has(target)) {
          report(
            `metadata.mode-aliases: ${alias}`,
            `metadata.mode-aliases maps ${alias} to ${render(pair.value)}, which is not a declared mode; map each alias to a value in metadata.modes.`,
          );
        } else {
          aliases.set(alias, target);
        }
      }
    }
  }

  const flagsNode = lookup(frontmatter, "metadata", "flags");
  if (flagsNode !== undefined) {
    if (!isSeq(flagsNode)) {
      report(
        "metadata.flags",
        "metadata.flags must be a list of flags such as [--no-docs].",
      );
    } else {
      const flags = new Set<string>();
      declaration.flags = flags;
      for (const item of flagsNode.items) {
        const value = stringText(item);
        if (value === undefined || !FLAG.test(value)) {
          report(
            `metadata.flags: ${render(item)}`,
            `metadata.flags lists ${render(item)}, which is not a flag; use -- followed by lowercase letters, digits and hyphens.`,
          );
        } else if (flags.has(value)) {
          report(
            `metadata.flags: ${value}`,
            `metadata.flags lists ${value} more than once; list each flag once.`,
          );
        } else {
          flags.add(value);
        }
      }
    }
  }

  return declaration;
}

/**
 * Reads the declaration of every `skills/<name>/SKILL.md`, keyed by skill directory name, and the `skill-metadata`
 * findings for malformed `metadata` values. A skill whose frontmatter is missing, never closed or does not parse gets
 * an empty declaration and no finding here, because the frontmatter rules report it.
 */
export function readDeclarations(
  repoRoot: string,
  tree = new SkillsTreeModel(repoRoot),
): {
  declarations: Map<string, SkillDeclaration>;
  findings: KeyedFailure[];
} {
  const declarations = new Map<string, SkillDeclaration>();
  const findings: KeyedFailure[] = [];
  for (const rel of tree.declarationFiles()) {
    const skill = rel.split("/")[1] ?? rel;
    const text = tree.read(rel);
    declarations.set(skill, declare(skill, rel, text, findings));
  }
  return { declarations, findings };
}
