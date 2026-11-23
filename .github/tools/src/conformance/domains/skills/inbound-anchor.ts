/**
 * Checks the relative Markdown links that point into `skills/` from tracked Markdown outside it: the target must be a
 * tracked file or a directory holding one, and an anchor must name a heading in a Markdown target. Work folders under
 * `docs/specs/` are not read, because they record history and may link to files that have since moved. Fenced code and
 * frontmatter are not read; inline code spans are, as in `skill-xref-anchor`. Targets and anchors are compared as
 * written, never percent-decoded.
 */
import path from "node:path";
import type { Failure } from "../../check.ts";
import { isFileErrorOfKind, skillsTrackedLookup } from "./files.ts";
import type { Family, ResolvedSkillsContext } from "./index.ts";
import { anchors } from "./xref-md.ts";
import { SkillsTreeModel } from "./tree-model.ts";

/** The rule name of an inbound link into `skills/` whose target or anchor does not exist. */
export const INBOUND_ANCHOR_RULE = "skill-inbound-anchor";

const LINK = /\[[^\]]*\]\(([^)\s]+)(?:[ \t]+"[^"]*")?\)/g;

const SCHEME = /^[a-z][a-z0-9+.-]*:/i;

/** The text of a repository file, or undefined when it is missing on disk. */
function readIfPresent(tree: SkillsTreeModel, rel: string): string | undefined {
  try {
    return tree.read(rel);
  } catch (error) {
    if (isFileErrorOfKind(error, "missing")) return undefined;
    throw error;
  }
}

/** Reports each inbound link into `skills/` whose target is not tracked or whose anchor names no heading there. */
export function inboundAnchorRules(
  root: string,
  context: Partial<ResolvedSkillsContext> = {},
): Failure[] {
  const tree = context.tree ?? new SkillsTreeModel(root);
  const tracked = (context.tracked ?? skillsTrackedLookup(root, process.env))();
  const cache = new Map<string, Set<string>>();
  const out: Failure[] = [];
  for (const source of tracked.files) {
    if (
      !source.endsWith(".md") ||
      source.startsWith("skills/") ||
      source.startsWith("docs/specs/")
    )
      continue;
    const text = readIfPresent(tree, source);
    if (text === undefined) continue;
    for (const line of tree.lines(text)) {
      for (const match of line.text.matchAll(LINK)) {
        const link = match[1] ?? "";
        if (link.startsWith("<") || SCHEME.test(link)) continue;
        const hash = link.indexOf("#");
        const file = hash < 0 ? link : link.slice(0, hash);
        const anchor = hash < 0 ? "" : link.slice(hash + 1);
        if (file === "") continue;
        const resolved = path.posix
          .normalize(path.posix.join(path.posix.dirname(source), file))
          .replace(/\/+$/, "");
        if (!resolved.startsWith("skills/")) continue;
        const isFile = tracked.files.has(resolved);
        if (!isFile && !tracked.dirs.has(resolved)) {
          out.push({
            rule: INBOUND_ANCHOR_RULE,
            path: source,
            message: `${source} links to ${file}, which is not a tracked file or directory`,
          });
          continue;
        }
        if (anchor === "") continue;
        let known: Set<string> | undefined;
        if (isFile && resolved.endsWith(".md")) {
          known = cache.get(resolved);
          if (known === undefined) {
            known = anchors(readIfPresent(tree, resolved) ?? "", tree.lines);
            cache.set(resolved, known);
          }
        }
        if (known?.has(anchor)) continue;
        out.push({
          rule: INBOUND_ANCHOR_RULE,
          path: source,
          message: `${source} links to ${file}#${anchor}, which names no heading there`,
        });
      }
    }
  }
  return out.sort(byPathThenMessage);
}

function byPathThenMessage(a: Failure, b: Failure): number {
  if (a.path !== b.path) return a.path < b.path ? -1 : 1;
  if (a.message !== b.message) return a.message < b.message ? -1 : 1;
  return 0;
}

export const FAMILY: Family = {
  name: "skill-inbound-anchor",
  rules: [INBOUND_ANCHOR_RULE],
  run: inboundAnchorRules,
};
