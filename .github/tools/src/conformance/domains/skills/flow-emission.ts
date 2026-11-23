import type { Failure } from "../../check.ts";
import { assetFiles, auditFileError, readRepoFile } from "./files.ts";
import { skillName } from "./frontmatter.ts";
import type { Family } from "./index.ts";

const RULE = "flow-emission-declared";

/** The skills whose SKILL.md must declare flow-state emission, in report order. */
const FLOW_ROSTER: readonly string[] = [
  "intent",
  "spec",
  "decompose",
  "execute",
  "gate",
  "triage",
];

const FLOW_HELPER_CALL = "agx flow";
const FLOW_STATE_REF_TARGET = "execute/references/flow-state.md";

/**
 * Every markdown link destination `](dest)` in `text`, in order. The scan is plain: it knows nothing of fences or
 * nesting, and a `](` with no `)` after it ends the scan. Callers match a destination by substring.
 */
function linkDestinations(text: string): string[] {
  const out: string[] = [];
  let rest = text;
  for (;;) {
    const start = rest.indexOf("](");
    if (start < 0) return out;
    rest = rest.slice(start + 2);
    const end = rest.indexOf(")");
    if (end < 0) return out;
    out.push(rest.slice(0, end));
    rest = rest.slice(end + 1);
  }
}

/**
 * Each roster skill's SKILL.md names the flow helper by its call form and links the record contract. A skill is found
 * by the `name:` value its file declares, the later sorted file winning a duplicate, and reported at
 * `skills/<name>/SKILL.md` whatever file held it. A roster skill that is absent is not a finding.
 */
function flowEmissionRules(root: string): Failure[] {
  const byName = new Map<string, string>();
  for (const rel of assetFiles(root)) {
    if (!rel.endsWith("/SKILL.md")) continue;
    let text: string;
    try {
      text = readRepoFile(root, rel);
    } catch (error) {
      throw auditFileError("reading", rel, error);
    }
    const name = skillName(text);
    if (name === undefined) continue;
    byName.set(name, text);
  }
  const failures: Failure[] = [];
  for (const name of FLOW_ROSTER) {
    const text = byName.get(name);
    if (text === undefined) continue;
    const path = `skills/${name}/SKILL.md`;
    if (!text.includes(FLOW_HELPER_CALL))
      failures.push({
        rule: RULE,
        path,
        message: `${name} does not name the flow helper call ${JSON.stringify(FLOW_HELPER_CALL)}`,
      });
    if (
      !linkDestinations(text).some((dest) =>
        dest.includes(FLOW_STATE_REF_TARGET),
      )
    )
      failures.push({
        rule: RULE,
        path,
        message: `${name} does not link the record contract ${JSON.stringify(FLOW_STATE_REF_TARGET)}`,
      });
  }
  return failures;
}

/** The `flow-emission` family: the skills that write flow state declare the helper and link its contract. */
export const FAMILY: Family = {
  name: "flow-emission",
  rules: [RULE],
  run: flowEmissionRules,
};
