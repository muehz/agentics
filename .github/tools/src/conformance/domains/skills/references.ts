import { statSync } from "node:fs";
import path from "node:path";
import type { Failure } from "../../check.ts";
import { FenceContainer } from "../../markdown-fence.ts";
import { repoFileErrorOf } from "../../repo-file-error.ts";
import {
  auditFileError,
  checkedRepoPath,
  globSkillLeaf,
  isFileErrorOfKind,
  skillDirectories,
  walkDir,
} from "./files.ts";
import type { Family } from "./index.ts";
import { SkillsTreeModel } from "./tree-model.ts";

/**
 * Template files whose body links target the destination repository they are copied into, so their links intentionally
 * do not resolve here.
 */
export const TEMPLATE_LINK_ALLOWLIST: readonly string[] = [
  "skills/scaffold/references/governance/constitution.md",
];

/**
 * The Apache-2.0 files vendored from anthropics/skills, each pinned to the notice it must keep (Apache-2.0 section 4(b)
 * requires a prominent notice on each modified file). The header must not point at `NOTICES_FILE`, because skills are
 * copied into other repositories; every other skills file carries no third-party attribution at all.
 */
export const APACHE_ATTRIBUTED_FILES: ReadonlyArray<{
  rel: string;
  notice: string;
}> = [
  {
    rel: "skills/docs/SKILL.md",
    notice:
      "<!-- Adapted from anthropics/skills doc-coauthoring (Apache-2.0); modified. -->",
  },
];

/** The single place upstream provenance is recorded. */
export const NOTICES_FILE = "THIRD_PARTY_NOTICES.md";

/** The per-file manifest that makes `NOTICES_FILE` a complete substitute for in-file headers. */
const NOTICES_MANIFEST_HEADING = "## Per-file provenance manifest";

// An inline code span. Applied to one line at a time.
const INLINE_CODE = /`[^`]*`/g;

// A markdown link to a `.md` target. The anchor group is matched and dropped; the target is group 1.
const MD_LINK = /\]\(([^)]+\.md)(#[^)]*)?\)/g;

// A target that is a URL or an absolute path. Case-sensitive, so HTTP://x.md is kept.
const LINK_SCHEME = /^(?:https?:|mailto:|\/)/;

// A templated placeholder target: one holding `<`, `>`, `{`, `}`, `$`, `*`, `|`, or ASCII whitespace (tab, LF, VT,
// FF, CR or space).
const TEMPLATED = /[<>{}$*|\t\n\v\f\r ]/;

// A third-party attribution marker, matched case-insensitively. The {0,200} count is in code points, which needs u.
const ATTRIBUTION_MARKER =
  /(?:adapted|vendored|adopted|ported|generalized) from[^\n]{0,200}?\((?:MIT|Apache-2\.0)\)/iu;

// A pointer to the notices file in any case. With iu the long s (U+017F) folds to S.
const NOTICES_POINTER = /THIRD_PARTY_NOTICES/iu;

/**
 * The relative markdown `.md` link targets of `text`, anchor removed, unique and in default
 * sort order. Fenced code (marker lines and content, as the container-aware tracker sees them) and inline code spans are
 * ignored, and so are `http(s):`, `mailto:` and absolute targets and templated placeholders.
 */
export function extractRelativeLinks(text: string): string[] {
  const seen = new Set<string>();
  const fence = new FenceContainer();
  for (const line of text.split("\n")) {
    if (fence.step(line)) continue;
    const stripped = line.replace(INLINE_CODE, "");
    for (const match of stripped.matchAll(MD_LINK)) {
      const target = match[1] ?? "";
      if (LINK_SCHEME.test(target) || TEMPLATED.test(target)) continue;
      seen.add(target);
    }
  }
  return [...seen].sort();
}

function readFileOrAudit(tree: SkillsTreeModel, rel: string): string {
  try {
    return tree.read(rel);
  } catch (error) {
    throw auditFileError("reading", rel, error);
  }
}

/** The `SKILL.md` files and `references/**.md` files under `skills`, in default sort order. */
function referenceFiles(root: string): string[] {
  let skillsRoot: string;
  try {
    skillsRoot = checkedRepoPath(root, "skills");
    if (!statSync(skillsRoot).isDirectory()) return [];
  } catch {
    return [];
  }
  const files: string[] = [];
  try {
    walkDir(
      skillsRoot,
      (inner, entry) => {
        if (entry.isDirectory || entry.isSymlink) return;
        const rel = `skills/${inner}`;
        if (
          entry.name === "SKILL.md" ||
          (rel.includes("/references/") && rel.endsWith(".md"))
        )
          files.push(rel);
      },
      "skills",
    );
  } catch (error) {
    throw auditFileError("walking", "skills", error);
  }
  return files.sort();
}

function danglingLinks(root: string, rel: string, text: string): Failure[] {
  const failures: Failure[] = [];
  const dir = path.dirname(path.join(root, ...rel.split("/")));
  for (const target of extractRelativeLinks(text)) {
    try {
      statSync(path.join(dir, ...target.split("/")));
    } catch {
      failures.push({
        rule: "reference-link",
        path: rel,
        message: `dangling link -> ${target}`,
      });
    }
  }
  return failures;
}

/**
 * Every relative `.md` link in a `SKILL.md` or `references/` file resolves on disk, no file
 * carries third-party attribution but the Apache files, and the notices file records each Apache file.
 */
function referenceLinkRules(
  root: string,
  tree = new SkillsTreeModel(root),
): Failure[] {
  const failures: Failure[] = [];
  const files = referenceFiles(root);
  const allowed = new Set(TEMPLATE_LINK_ALLOWLIST);

  for (const rel of files) {
    if (allowed.has(rel)) continue;
    failures.push(...danglingLinks(root, rel, readFileOrAudit(tree, rel)));
  }

  const apache = new Set(APACHE_ATTRIBUTED_FILES.map((entry) => entry.rel));
  for (const rel of files) {
    const text = readFileOrAudit(tree, rel);
    const pointer = NOTICES_POINTER.exec(text)?.[0];
    if (pointer !== undefined)
      failures.push({
        rule: "attribution-centralized",
        path: rel,
        message: `points into the Agentics repository (${JSON.stringify(pointer)}); a skill must stay self-contained`,
      });
    if (apache.has(rel)) continue;
    const marker = ATTRIBUTION_MARKER.exec(text)?.[0];
    if (marker !== undefined)
      failures.push({
        rule: "attribution-centralized",
        path: rel,
        message: `in-file third-party attribution (${JSON.stringify(marker)}); record it in ${NOTICES_FILE} instead`,
      });
  }

  for (const entry of APACHE_ATTRIBUTED_FILES) {
    let text: string;
    try {
      text = tree.read(entry.rel);
    } catch (error) {
      if (!isFileErrorOfKind(error, "missing"))
        throw auditFileError("reading", entry.rel, error);
      failures.push({
        rule: "reference-attribution",
        path: entry.rel,
        message: "attributed third-party file missing",
      });
      continue;
    }
    if (!text.includes(entry.notice))
      failures.push({
        rule: "reference-attribution",
        path: entry.rel,
        message:
          "pinned Apache-2.0 attribution notice (source + license) missing",
      });
  }

  let notices: string;
  try {
    notices = tree.read(NOTICES_FILE);
  } catch (error) {
    if (!isFileErrorOfKind(error, "missing"))
      throw auditFileError("reading", NOTICES_FILE, error);
    failures.push({
      rule: "notices-manifest",
      path: NOTICES_FILE,
      message: "third-party notices file missing",
    });
    return failures;
  }
  if (!notices.includes(NOTICES_MANIFEST_HEADING))
    failures.push({
      rule: "notices-manifest",
      path: NOTICES_FILE,
      message: `missing ${JSON.stringify(NOTICES_MANIFEST_HEADING)} section`,
    });
  for (const entry of APACHE_ATTRIBUTED_FILES)
    if (!notices.includes(entry.rel.replace(/^skills\//, "")))
      failures.push({
        rule: "notices-manifest",
        path: NOTICES_FILE,
        message: `no manifest row for ${entry.rel}`,
      });
  return failures;
}

/** The `reference-link` family: relative links resolve and third-party attribution stays in the notices file. */
export const REFERENCE_LINK_FAMILY: Family = {
  name: "reference-link",
  rules: [
    "reference-link",
    "attribution-centralized",
    "reference-attribution",
    "notices-manifest",
  ],
  run: (root, context) => referenceLinkRules(root, context?.tree),
};

// The em dash of the naming messages, built from its code point so the source stays ASCII.
const EM_DASH = String.fromCodePoint(0x2014);

// Kebab-case directory and file names, both ASCII only and flagless: `$` matches only at the end of the name.
const KEBAB_DIR = /^[a-z0-9]+(-[a-z0-9]+)*$/;
const KEBAB_FILE = /^[a-z0-9]+(-[a-z0-9]+)*\.[a-z0-9]+$/;

/** Whether `base` is a conventional file name that keeps its uppercase form. */
function namingExempt(base: string): boolean {
  return base === "README.md" || base === "LICENSE";
}

/**
 * Everything under `skills/<skill>/references/` is lowercase kebab-case. The walk is
 * pre-order with default sort order at each level, so a directory's own failure comes before its contents and no global sort
 * applies. Symlink entries are neither checked nor counted; every other entry below a references root counts, exempt
 * files included, and a checkout that counts none is an audit error.
 */
function referenceNamingRules(root: string): Failure[] {
  const failures: Failure[] = [];
  let checked = 0;

  for (const parent of skillDirectories(root)) {
    const leaf = globSkillLeaf(root, parent, "references");
    if (!leaf.selected) continue;
    const refsRoot = leaf.path;
    const refsRel = `${parent}/references`;
    let isDirectory: boolean;
    try {
      isDirectory = statSync(refsRoot).isDirectory();
    } catch (error) {
      // The selection inspected the path a moment ago, so only a change on disk in between reaches this.
      throw auditFileError(
        "checking",
        refsRel,
        repoFileErrorOf(error, refsRel),
      );
    }
    if (!isDirectory) continue;
    try {
      walkDir(
        refsRoot,
        (inner, entry) => {
          if (entry.isSymlink) return;
          checked += 1;
          const rel = `${refsRel}/${inner}`;
          if (entry.isDirectory) {
            if (!KEBAB_DIR.test(entry.name))
              failures.push({
                rule: "reference-naming",
                path: rel,
                message: `reference directory name is not lowercase kebab-case ${EM_DASH} rename with 'git mv'`,
              });
            return;
          }
          if (namingExempt(entry.name)) return;
          if (!KEBAB_FILE.test(entry.name))
            failures.push({
              rule: "reference-naming",
              path: rel,
              message: `reference file name is not lowercase kebab-case (exempt: README.md, LICENSE) ${EM_DASH} rename with 'git mv'`,
            });
        },
        refsRel,
      );
    } catch (error) {
      throw auditFileError("walking", refsRel, error);
    }
  }

  if (checked === 0)
    throw new Error(
      `skills: no reference files found ${EM_DASH} the reference-naming glob is wrong`,
    );
  return failures;
}

/** The `reference-naming` family: everything under `references/` is named in lowercase kebab-case. */
export const REFERENCE_NAMING_FAMILY: Family = {
  name: "reference-naming",
  rules: ["reference-naming"],
  run: (root) => referenceNamingRules(root),
};
