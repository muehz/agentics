import assert from "node:assert/strict";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { test } from "node:test";
import { fileURLToPath } from "node:url";
import { repoRoot } from "../../check.ts";
import { writeFile } from "../../testing/files.ts";
import { applyBaseline } from "./baseline.ts";
import { parseFrontmatter } from "./frontmatter.ts";
import { readDeclarations, type SkillDeclaration } from "./skill-meta.ts";
import {
  ARGUMENT_HINT_RULE,
  DESCRIPTION_LENGTH_RULE,
  NAME_FORMAT_RULE,
  REFERENCES_INDEX_RULE,
  ROUTING_SECTION_RULE,
  structureFindings,
  SUMMARY_BLOCK_RULE,
} from "./structure.ts";
import { skillDocs, type SkillDoc } from "./xref-md.ts";

const SKILL = "skills/a/SKILL.md";

const RULES = new Set([
  SUMMARY_BLOCK_RULE,
  REFERENCES_INDEX_RULE,
  ROUTING_SECTION_RULE,
  NAME_FORMAT_RULE,
  DESCRIPTION_LENGTH_RULE,
  ARGUMENT_HINT_RULE,
]);

const HEAD =
  "---\nname: a\n---\n# A\n\nIntro.\n\n## Don't use when\n\n- Never.\n\n## Steps\n\nDo it.\n";
const TAIL = "\n## References\n\n- [r](references/r.md)\n";
const PASSING = HEAD + TAIL;

function doc(rel: string, text: string): SkillDoc {
  return { rel, skill: rel.split("/")[1] ?? "", text, scanned: true };
}

function findings(text: string, rel = SKILL) {
  return structureFindings(new Map([[rel, doc(rel, text)]]), new Map());
}

function only(text: string, rule: string) {
  return findings(text).filter((f) => f.rule === rule);
}

test("the rule ids are the published names", () => {
  assert.equal(SUMMARY_BLOCK_RULE, "skill-summary-block");
  assert.equal(REFERENCES_INDEX_RULE, "skill-references-index");
  assert.equal(ROUTING_SECTION_RULE, "skill-routing-section");
  assert.equal(NAME_FORMAT_RULE, "skill-name-format");
  assert.equal(DESCRIPTION_LENGTH_RULE, "skill-description-length");
  assert.equal(ARGUMENT_HINT_RULE, "skill-argument-hint");
});

test("the passing shape gives no finding", () => {
  assert.deepEqual(findings(PASSING), []);
});

test("a Capability summary heading gives one summary-block finding keyed by its text", () => {
  const got = findings(HEAD + "\n## Capability summary\n\nText.\n" + TAIL);
  assert.equal(got.length, 1);
  assert.equal(got[0]?.rule, SUMMARY_BLOCK_RULE);
  assert.equal(got[0]?.path, SKILL);
  assert.equal(got[0]?.key, "Capability summary");
});

test("the summary heading is found at any level and in any case", () => {
  const got = only(
    HEAD + "\n### capability summary\n\nText.\n" + TAIL,
    SUMMARY_BLOCK_RULE,
  );
  assert.equal(got.length, 1);
  assert.equal(got[0]?.key, "capability summary");
});

test("a repeated summary heading carries its occurrence number, so a baselined first copy hides no later one", () => {
  const got = only(
    HEAD +
      "\n## Capability summary\n\nA.\n\n## Capability summary\n\nB.\n\n### capability summary\n" +
      TAIL,
    SUMMARY_BLOCK_RULE,
  );
  assert.deepEqual(
    got.map((f) => f.key),
    ["Capability summary", "Capability summary (2)", "capability summary"],
  );
  const accepted = [
    { rule: SUMMARY_BLOCK_RULE, path: SKILL, key: "Capability summary" },
  ];
  assert.deepEqual(
    applyBaseline(got, new Map([["a.json", accepted]])).map((f) => f.message),
    [got[1]?.message, got[2]?.message],
  );
});

test("a summary heading inside a code fence is not reported", () => {
  const text = HEAD + "\n```\n## Capability summary\n```\n" + TAIL;
  assert.deepEqual(findings(text), []);
});

test("a body with no References heading gives one references-index finding", () => {
  const got = findings(HEAD);
  assert.equal(got.length, 1);
  assert.equal(got[0]?.rule, REFERENCES_INDEX_RULE);
  assert.equal(got[0]?.path, SKILL);
  assert.equal(got[0]?.key, "references");
});

test("two References headings give one finding", () => {
  const got = only(HEAD + TAIL + TAIL, REFERENCES_INDEX_RULE);
  assert.equal(got.length, 1);
  assert.equal(got[0]?.key, "references");
});

test("a level-two heading after References gives one finding", () => {
  const got = only(PASSING + "\n## After\n\nMore.\n", REFERENCES_INDEX_RULE);
  assert.equal(got.length, 1);
  assert.equal(got[0]?.key, "references");
});

test("a deeper heading after References still passes", () => {
  assert.deepEqual(findings(PASSING + "\n### Sub\n\nMore.\n"), []);
});

test("a References heading inside a code fence does not count as the index", () => {
  const fenced = only(
    HEAD + "\n```\n## References\n```\n",
    REFERENCES_INDEX_RULE,
  );
  assert.equal(fenced.length, 1);
  assert.deepEqual(
    only(PASSING + "\n```\n## References\n```\n", REFERENCES_INDEX_RULE),
    [],
  );
});

test("a body with no Don't use when heading gives one routing-section finding", () => {
  const body = PASSING.replace("## Don't use when\n", "");
  const got = findings(body);
  assert.equal(got.length, 1);
  assert.equal(got[0]?.rule, ROUTING_SECTION_RULE);
  assert.equal(got[0]?.path, SKILL);
  assert.equal(got[0]?.key, "routing");
});

test("a Don't use when heading inside a code fence does not satisfy the routing rule", () => {
  const body = PASSING.replace(
    "## Don't use when\n",
    "```\n## Don't use when\n```\n",
  );
  assert.equal(only(body, ROUTING_SECTION_RULE).length, 1);
});

test("a reference file is not checked", () => {
  assert.deepEqual(findings("# R\n", "skills/a/references/r.md"), []);
});

interface Declared {
  modes?: string[];
  aliases?: Record<string, string>;
  flags?: string[];
}

/** A declaration for the skill directory `dir`, as the metadata reader would return it for the frontmatter `yaml`. */
function declaration(
  dir: string,
  yaml: string | undefined,
  words: Declared = {},
): SkillDeclaration {
  return {
    skill: dir,
    rel: `skills/${dir}/SKILL.md`,
    frontmatter: yaml === undefined ? undefined : parseFrontmatter(yaml),
    surfaces: [],
    modes: words.modes === undefined ? undefined : new Set(words.modes),
    aliases: new Map(Object.entries(words.aliases ?? {})),
    flags: new Set(words.flags ?? []),
  };
}

/** The findings of one rule for a single declared skill whose body passes, so only the frontmatter rules can report. */
function declared(
  rule: string,
  dir: string,
  yaml: string | undefined,
  words: Declared = {},
) {
  const rel = `skills/${dir}/SKILL.md`;
  return structureFindings(
    new Map([[rel, doc(rel, PASSING)]]),
    new Map([[dir, declaration(dir, yaml, words)]]),
  ).filter((f) => f.rule === rule);
}

const DESCRIPTION = "description: A test skill.\n";

function nameFindings(dir: string, nameLine: string) {
  return declared(NAME_FORMAT_RULE, dir, nameLine + DESCRIPTION);
}

test("a lowercase kebab-case name equal to the directory name gives no name finding", () => {
  assert.deepEqual(nameFindings("my-skill2", "name: my-skill2\n"), []);
});

test("an uppercase letter, an underscore or a leading hyphen each give one name finding keyed name", () => {
  for (const name of ["My-skill", "my_skill", "-my-skill"]) {
    const got = nameFindings(name, `name: "${name}"\n`);
    assert.equal(got.length, 1, name);
    assert.equal(got[0]?.key, "name");
    assert.equal(got[0]?.path, `skills/${name}/SKILL.md`);
    assert.ok(got[0]?.message.includes(name), name);
  }
});

test("a name of 64 characters passes and 65 characters gives one name finding", () => {
  const ok = "a".repeat(64);
  assert.deepEqual(nameFindings(ok, `name: ${ok}\n`), []);
  const long = "a".repeat(65);
  const got = nameFindings(long, `name: ${long}\n`);
  assert.equal(got.length, 1);
  assert.equal(got[0]?.key, "name");
  assert.match(got[0]?.message ?? "", /\b65\b/);
  assert.match(got[0]?.message ?? "", /\b64\b/);
});

test("a valid name that differs from the directory name gives one name finding", () => {
  const got = nameFindings("dir", "name: other\n");
  assert.equal(got.length, 1);
  assert.equal(got[0]?.key, "name");
  assert.match(got[0]?.message ?? "", /"other"/);
  assert.match(got[0]?.message ?? "", /"dir"/);
});

test("a name that is a number or a list gives one name finding", () => {
  assert.equal(nameFindings("a", "name: 42\n").length, 1);
  assert.equal(nameFindings("a", "name: [a]\n").length, 1);
});

test("a description of 1024 characters passes and 1025 gives one description finding", () => {
  const at = (n: number) =>
    declared(
      DESCRIPTION_LENGTH_RULE,
      "a",
      `name: a\ndescription: ${"d".repeat(n)}\n`,
    );
  assert.deepEqual(at(1024), []);
  const got = at(1025);
  assert.equal(got.length, 1);
  assert.equal(got[0]?.key, "description");
  assert.equal(got[0]?.path, SKILL);
  assert.match(got[0]?.message ?? "", /\b1025\b/);
  assert.match(got[0]?.message ?? "", /\b1024\b/);
});

test("a description that is a list gives one description finding", () => {
  const got = declared(
    DESCRIPTION_LENGTH_RULE,
    "a",
    "name: a\ndescription: [one, two]\n",
  );
  assert.equal(got.length, 1);
  assert.equal(got[0]?.key, "description");
});

function hintFindings(hint: string | undefined, words: Declared) {
  const line =
    hint === undefined ? "" : `argument-hint: ${JSON.stringify(hint)}\n`;
  return declared(
    ARGUMENT_HINT_RULE,
    "a",
    "name: a\n" + DESCRIPTION + line,
    words,
  );
}

test("every declared mode, alias and flag that is a token of the hint gives no hint finding", () => {
  const words = {
    modes: ["branch", "release"],
    aliases: { pr: "branch" },
    flags: ["--no-docs", "--no-review"],
  };
  assert.deepEqual(
    hintFindings(
      "branch [name] | release; pr/x→branch [--no-docs|--no-review]",
      words,
    ),
    [],
  );
});

test("a flag in brackets is a token of the hint", () => {
  assert.deepEqual(
    hintFindings("[topic] [--composed]", { flags: ["--composed"] }),
    [],
  );
});

test("a declared word that is only part of a hyphenated hint token gives one hint finding keyed by the word", () => {
  const got = hintFindings("[audit | feedback-loops author]", {
    modes: ["audit", "loops"],
    aliases: { feedback: "audit" },
  });
  assert.deepEqual(got.map((f) => f.key).sort(), ["feedback", "loops"]);
  assert.equal(got[0]?.path, SKILL);
  assert.ok(got.every((f) => f.message.includes("feedback-loops author")));
});

test("a flag without its dashes in the hint is not a match", () => {
  const got = hintFindings("[composed]", { flags: ["--composed"] });
  assert.deepEqual(
    got.map((f) => f.key),
    ["--composed"],
  );
});

test("declared words and no argument-hint give one hint finding per word", () => {
  const got = hintFindings(undefined, {
    modes: ["comments"],
    flags: ["--all"],
  });
  assert.deepEqual(got.map((f) => f.key).sort(), ["--all", "comments"]);
});

test("no declared word gives no hint finding, with or without a hint", () => {
  assert.deepEqual(hintFindings(undefined, {}), []);
  assert.deepEqual(hintFindings("[anything]", {}), []);
  assert.deepEqual(hintFindings(undefined, { modes: [] }), []);
});

test("unreadable frontmatter gives no frontmatter structure finding", () => {
  const got = structureFindings(
    new Map([[SKILL, doc(SKILL, PASSING)]]),
    new Map([
      ["a", declaration("a", undefined, { modes: ["x"], flags: ["--y"] })],
    ]),
  );
  assert.deepEqual(got, []);
});

test("an absent name or description key gives no finding for that rule", () => {
  assert.deepEqual(declared(NAME_FORMAT_RULE, "a", DESCRIPTION), []);
  assert.deepEqual(declared(DESCRIPTION_LENGTH_RULE, "a", "name: a\n"), []);
});

const FRONTMATTER_RULES = new Set([
  NAME_FORMAT_RULE,
  DESCRIPTION_LENGTH_RULE,
  ARGUMENT_HINT_RULE,
]);

/**
 * Writes one `skills/<dir>/SKILL.md` per entry to a temporary repo and returns the frontmatter findings read back
 * through `skillDocs` and `readDeclarations`.
 */
function onDisk(skills: Record<string, string>) {
  const root = mkdtempSync(path.join(tmpdir(), "agentics-structure-"));
  try {
    for (const [dir, text] of Object.entries(skills))
      writeFile(root, `skills/${dir}/SKILL.md`, text);
    return structureFindings(
      skillDocs(root),
      readDeclarations(root).declarations,
    ).filter((f) => FRONTMATTER_RULES.has(f.rule));
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
}

test("skills read from disk give the frontmatter findings and unreadable frontmatter gives none", () => {
  const meta =
    "metadata:\n  surfaces: [claude-code]\n  modes: [audit]\n  mode-aliases:\n    feedback: audit\n  flags: [--composed]\n";
  const got = onDisk({
    good: `---\nname: good\ndescription: ok\nargument-hint: "[audit | feedback] [--composed]"\n${meta}---\n# Good\n`,
    bad: `---\nname: Bad\ndescription: ${"d".repeat(1025)}\nargument-hint: "[feedback-loops]"\n${meta}---\n# Bad\n`,
    missing: "# Missing\n",
    unclosed: `---\nname: Unclosed\n${meta}# Unclosed\n`,
    scalar: "---\njust text\n---\n# Scalar\n",
  });
  assert.deepEqual(got.map((f) => `${f.rule} ${f.path} ${f.key}`).sort(), [
    `${ARGUMENT_HINT_RULE} skills/bad/SKILL.md --composed`,
    `${ARGUMENT_HINT_RULE} skills/bad/SKILL.md audit`,
    `${ARGUMENT_HINT_RULE} skills/bad/SKILL.md feedback`,
    `${DESCRIPTION_LENGTH_RULE} skills/bad/SKILL.md description`,
    `${NAME_FORMAT_RULE} skills/bad/SKILL.md name`,
  ]);
});

test("every finding in the real checkout names one of the rules and a SKILL.md", () => {
  const root = repoRoot(path.dirname(fileURLToPath(import.meta.url)));
  const { declarations } = readDeclarations(root);
  for (const finding of structureFindings(skillDocs(root), declarations)) {
    assert.ok(RULES.has(finding.rule));
    assert.match(finding.path, /^skills\/[^/]+\/SKILL\.md$/);
  }
});

test("an escape character in a name or an argument-hint is printed as an escape", () => {
  const name = nameFindings("a", 'name: "a\\eb"\n');
  assert.equal(name.length, 1);
  assert.ok(
    name[0]?.message.startsWith(
      'name "a\\u001bb" is not lowercase kebab-case, differs from the directory name "a"',
    ),
  );
  const hint = hintFindings("x\u001b[31m audit-all", { modes: ["audit"] });
  assert.equal(hint.length, 1);
  assert.ok(
    hint[0]?.message.includes(
      'argument-hint "x\\u001b[31m audit-all" does not name it',
    ),
  );
});
