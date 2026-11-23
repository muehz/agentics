import path from "node:path";
import { errorMessage, type Failure } from "../../check.ts";
import type { Family } from "./index.ts";
import { extractRelativeLinks } from "./references.ts";
import { SkillsTreeModel } from "./tree-model.ts";

const RULE = "shared-contracts";
const CONTRACT_DIR = "skills/routing/references/contracts";

interface Contract {
  filename: string;
  consumers: readonly string[];
  copies: readonly RegExp[];
}

// These patterns capture clauses from migrated definitions. A local step list, an applicability note, or a reference
// to the policy by name can stand on its own without repeating a shared definition.
const CONTRACTS: readonly Contract[] = [
  {
    filename: "workflow-steps.md",
    consumers: [
      "skills/decompose/SKILL.md",
      "skills/diagnose/SKILL.md",
      "skills/execute/SKILL.md",
      "skills/execute/references/arena-mode.md",
      "skills/forge/references/quality/advanced-patterns.md",
      "skills/forge/references/quality/skill-hardening-checklist.md",
      "skills/gate/SKILL.md",
      "skills/intent/SKILL.md",
      "skills/intent/references/brief.md",
      "skills/review/SKILL.md",
      "skills/routing/SKILL.md",
      "skills/tdd/SKILL.md",
    ],
    copies: [
      /add (?:its|the mode's) steps to the (?:session's )?todo list,? after any items already/,
      /add this run's steps to the todo list/,
      /a phase that runs later adds its own steps instead of replacing them/,
      /once step 0 knows what this run does, add to the todo list/,
      /before phase 1, add the four phases and the cleanup checklist to the todo list/,
      /once the entry row is known, add step 0, phases 1 to 3, and the verification checklist to the todo list/,
      /open the todo list with one entry per phase before you launch anything/,
      /copy them into the todo list verbatim, before task-specific items/,
      /add each named step and checklist item verbatim, before that workflow's task-specific items/,
    ],
  },
  {
    filename: "principles-check.md",
    consumers: [
      "skills/challenge/SKILL.md",
      "skills/decompose/SKILL.md",
      "skills/deepen/SKILL.md",
      "skills/diagnose/SKILL.md",
      "skills/execute/SKILL.md",
      "skills/gate/SKILL.md",
      "skills/hillclimb/SKILL.md",
      "skills/intent/SKILL.md",
      "skills/principles/SKILL.md",
      "skills/spec/references/draft.md",
      "skills/tdd/SKILL.md",
    ],
    copies: [
      /shortlist the principles whose applies when [^.]+\. read each shortlisted leaf/,
      /scan the principles index for principles whose applies when [^.]+read each one you shortlist/,
      /scan principles for decisions (?:this work|this solution) could change.{0,30}read each shortlisted leaf/,
      /shortlist a principle only when its applies when cell matches the work and a real decision in this task could go differently/,
      /run this check when the current phase can inspect a real artifact.{0,125}does not count as evidence/,
    ],
  },
  {
    filename: "empirical-questions.md",
    consumers: [
      "skills/challenge/references/discern.md",
      "skills/decompose/SKILL.md",
      "skills/intent/SKILL.md",
      "skills/prototype/SKILL.md",
      "skills/prototype/references/probe.md",
      "skills/routing/SKILL.md",
      "skills/spec/SKILL.md",
      "skills/spec/references/draft.md",
    ],
    copies: [
      /a question whose answer a run can observe.{0,140}is settled by (?:an experiment|prototype)/,
      /a fact that a run can observe.{0,140}is not a judgment call, and no one should be asked/,
      /before asking the user or taking a question to challenge, separate facts an observation can settle from product or preference choices/,
    ],
  },
];

function checkCanonicalFile(tree: SkillsTreeModel, rel: string): void {
  try {
    const text = tree.read(rel);
    if (!text.trim() || text.includes("\0"))
      throw new Error("expected nonempty Markdown text");
  } catch (cause) {
    throw new Error(
      "skills: shared-contracts: reading " + rel + ": " + errorMessage(cause),
      { cause },
    );
  }
}

function policyParagraphs(prose: string): string[] {
  return prose.split(/\n[ \t]*\n/).map((paragraph) =>
    paragraph
      .replace(/\[([^\]\n]+)\]\([^)\n]+\)/g, "$1")
      .replace(/(\x60+)([^\x60]+)\1/g, (_span, _ticks, content: string) =>
        /^(?:\/?principles|prototype|applies when)$/i.test(content.trim())
          ? content
          : " ",
      )
      .replace(/[*_"“”]/g, "")
      .replace(/’/g, "'")
      .replace(/\s+/g, " ")
      .trim()
      .toLowerCase(),
  );
}

/** Returns the targets of navigable inline citations in active skill prose. */
export function policyLinks(prose: string): string[] {
  // Skip code spans before looking for inline links. The historic link audit only needs to recognize the target
  // suffix, but a required citation must have a label and must be navigable rather than an image.
  const tokens =
    /(\x60+)(?!\x60)[^\r\n]*?(?<!\x60)\1(?!\x60)|(?<![!\\])\[[^\[\]\r\n]+(?<!\\)\]\(([^ \t\r\n)]+)(?:[ \t]+(?:"[^"\r\n]*"|'[^'\r\n]*'|\([^\r\n)]*\)))?\)/g;
  const citations = Array.from(prose.matchAll(tokens), (match) =>
    match[2] === undefined ? "" : "](" + match[2] + ")",
  ).join("\n");
  return extractRelativeLinks(citations);
}

function audit(root: string, tree = new SkillsTreeModel(root)): Failure[] {
  for (const { filename } of CONTRACTS)
    checkCanonicalFile(tree, CONTRACT_DIR + "/" + filename);

  const failures: Failure[] = [];
  for (const { rel, text, scanned } of tree.docs().values()) {
    if (!scanned) continue;
    const prose = tree.contractProse(text);
    const paragraphs = policyParagraphs(prose);
    const directory = path.posix.dirname(rel);
    const links = new Set(
      policyLinks(prose).map((target) => path.posix.join(directory, target)),
    );
    for (const { filename, consumers, copies } of CONTRACTS) {
      const canonical = CONTRACT_DIR + "/" + filename;
      if (consumers.includes(rel) && !links.has(canonical))
        failures.push({
          rule: RULE,
          path: rel,
          message: "must link to " + canonical,
        });
      if (
        rel !== canonical &&
        paragraphs.some((paragraph) =>
          copies.some((copy) => copy.test(paragraph)),
        )
      )
        failures.push({
          rule: RULE,
          path: rel,
          message: "replace the local policy copy with a link to " + canonical,
        });
    }
  }
  return failures;
}

export const FAMILY: Family = {
  name: RULE,
  rules: [RULE],
  run: (root, context) => audit(root, context?.tree),
};
