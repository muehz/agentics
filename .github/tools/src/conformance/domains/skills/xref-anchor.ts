/**
 * Checks that each relative Markdown link with a `#anchor` in a scanned skill doc names a heading in its target file.
 * Fenced code and frontmatter are not read; inline code spans are read, so a bad anchor inside backticks is reported.
 * Known limit: a target outside the skill docs, such as `skills/README.md`, is never checked, and a target file that
 * does not exist is left to the reference-link rule.
 */
import path from "node:path";
import type { KeyedFailure } from "./baseline.ts";
import { anchors, proseLines, type SkillDoc } from "./xref-md.ts";

/** The rule name of a link whose anchor matches no heading in its target file. */
export const ANCHOR_RULE = "skill-xref-anchor";

const LINK = /\[[^\]]*\]\(([^)\s]+)(?:[ \t]+"[^"]*")?\)/g;

const SCHEME = /^[a-z][a-z0-9+.-]*:/i;

/** The resolved target file of a link from source, where an empty file part names the source itself. */
function resolveTarget(source: string, file: string): string {
  if (file === "") return source;
  return path.posix.normalize(
    path.posix.join(path.posix.dirname(source), file),
  );
}

/** One finding for each occurrence of an anchored link in a scanned doc whose anchor names no heading in the target. */
export function anchorFindings(
  docs: ReadonlyMap<string, SkillDoc>,
  lines = proseLines,
): KeyedFailure[] {
  const cache = new Map<string, Set<string>>();
  const out: KeyedFailure[] = [];
  for (const source of docs.values()) {
    if (!source.scanned) continue;
    for (const line of lines(source.text)) {
      for (const match of line.text.matchAll(LINK)) {
        const target = match[1] ?? "";
        const hash = target.indexOf("#");
        if (hash < 0 || SCHEME.test(target)) continue;
        const anchor = target.slice(hash + 1);
        if (anchor === "") continue;
        const resolved = resolveTarget(source.rel, target.slice(0, hash));
        const targetDoc = docs.get(resolved);
        if (targetDoc === undefined) continue;
        let known = cache.get(resolved);
        if (known === undefined) {
          known = anchors(targetDoc.text, lines);
          cache.set(resolved, known);
        }
        if (known.has(anchor)) continue;
        out.push({
          rule: ANCHOR_RULE,
          path: source.rel,
          key: target,
          message: `line ${line.number}: link ${target} names #${anchor}, which matches no heading in ${resolved}`,
        });
      }
    }
  }
  return out;
}
