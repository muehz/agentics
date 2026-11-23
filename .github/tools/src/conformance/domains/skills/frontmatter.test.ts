import assert from "node:assert/strict";
import {
  mkdirSync,
  renameSync,
  rmSync,
  symlinkSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { test, type TestContext } from "node:test";
import { fileURLToPath } from "node:url";
import {
  repoRoot,
  type AuditResult,
  type Failure,
  type RunOptions,
} from "../../check.ts";
import {
  initCheckout,
  tempIsInsideRepository,
} from "../../testing/git-checkout.ts";
import {
  auditEveryFamily,
  auditSlice,
  EMPTY_SKILL_FINDING,
  sliceNamed,
  staleBaseline,
  all,
  breaker,
  CHALLENGE,
  crlf,
  GARBAGE,
  GATE_SKILL,
  GIT_COMMIT,
  put,
  REQUIRED,
  skillWithMetadata,
  versionAtLine,
  X_SKILL,
  xBody,
  xFields,
  xText,
} from "../../testing/skills-seeds.ts";
import {
  addSkill,
  link,
  mkdir,
  mutateFile,
  remove,
  skillMD,
  withSkillsRepo,
  writeFile,
} from "../../testing/skills-repo.ts";
import { RepoFileError } from "../../repo-file-error.ts";
import { seams, withSeams, type Seams } from "./files.ts";
import {
  EVAL_FRONTMATTER_CLEAN_FAMILY,
  EVAL_METADATA_FAMILY,
  frontmatter,
  frontmatterBlock,
  FRONTMATTER_FAMILY,
  INLINE_FAMILY,
  FrontmatterError,
  isBoolean,
  isMap,
  isScalar,
  isSeq,
  lookup,
  parseFrontmatter,
  printable,
  render,
  skillName,
  stringText,
} from "./frontmatter.ts";
import { run } from "./index.ts";
import { withOutside } from "../../testing/host.ts";

const options: RunOptions = {
  short: false,
  fast: false,
  diagnostics: () => {},
  env: process.env,
};

const NBSP = String.fromCharCode(0xa0);
const BOM = String.fromCharCode(0xfeff);
const LINE_SEPARATOR = String.fromCharCode(0x2028);
const NEL = String.fromCharCode(0x85);

const FRONTMATTER_MESSAGE =
  "missing required frontmatter (name/description/version) in the first 20 lines";

const finding = (rule: string, at: string, message: string): Failure => ({
  rule,
  path: at,
  message,
});

const missingFrontmatter = (at: string): Failure =>
  finding("skill-frontmatter", at, FRONTMATTER_MESSAGE);

const duplicate = (name: string, first: string, second: string): Failure =>
  finding(
    "skill-name-unique",
    second,
    `duplicate skill name "${name}": ${first} and ${second}`,
  );

const inline = (root: string) =>
  run(root, options, { families: [INLINE_FAMILY] });

/** A SKILL.md whose block holds the given lines, then `description` and `version`, and closes. */
const skillFile = (...lines: string[]): string =>
  ["---", ...lines, "description: d", 'version: "1"', "---", "Body.", ""].join(
    "\n",
  );

/** Creates a link, or skips the calling test with a logged reason when the host refuses. */
function linkOrSkip(
  t: TestContext,
  root: string,
  rel: string,
  target: string,
  type?: "file" | "dir",
): boolean {
  if (link(root, rel, target, type)) return true;
  t.skip("the host cannot create a symbolic link");
  return false;
}

// --- frontmatterBlock and frontmatter --------------------------------------

const BLOCK_ROWS: ReadonlyArray<{
  name: string;
  text: string;
  block: string;
  closed: boolean;
}> = [
  { name: "opener only", text: "---", block: "", closed: false },
  { name: "empty block", text: "---\n---", block: "", closed: true },
  { name: "blank line block", text: "---\n\n---", block: "", closed: true },
  { name: "no closer", text: "---\na: 1", block: "a: 1", closed: false },
  {
    name: "closer with a trailing space is body",
    text: "---\na: 1\n--- \nb\n---\n",
    block: "a: 1\n--- \nb",
    closed: true,
  },
  {
    name: "no closer, final newline",
    text: "---\na: 1\n",
    block: "a: 1\n",
    closed: false,
  },
  { name: "empty text", text: "", block: "", closed: false },
  {
    name: "BOM before the opener",
    text: BOM + "---\na: 1\n---\n",
    block: "",
    closed: false,
  },
  {
    name: "opener with a trailing space",
    text: "--- \na: 1\n---\n",
    block: "",
    closed: false,
  },
  {
    name: "opener followed by a lone CR",
    text: "---\ra: 1\n---\n",
    block: "",
    closed: false,
  },
  {
    name: "opener followed by CR CR LF",
    text: "---\r\r\na: 1\n---\n",
    block: "",
    closed: false,
  },
  {
    name: "CRLF file",
    text: "---\r\na: 1\r\n---\r\n",
    block: "a: 1",
    closed: true,
  },
  {
    name: "lone CR inside the block",
    text: "---\na\rb\n---\n",
    block: "a\rb",
    closed: true,
  },
  {
    name: "the opener must be the first line",
    text: "a: 1\n---\n",
    block: "",
    closed: false,
  },
];

for (const row of BLOCK_ROWS) {
  test(`frontmatterBlock: ${row.name}`, () => {
    assert.deepEqual(frontmatterBlock(row.text), {
      block: row.block,
      closed: row.closed,
    });
  });
  test(`frontmatter: ${row.name}`, () => {
    assert.equal(frontmatter(row.text), row.block);
  });
}

// --- skillName -------------------------------------------------------------

const NAME_ROWS: ReadonlyArray<{
  name: string;
  text: string;
  expected: string | undefined;
}> = [
  { name: "plain", text: "name: tdd", expected: "tdd" },
  { name: "padded", text: "name:   tdd  ", expected: "tdd" },
  { name: "no space", text: "name:x", expected: "x" },
  {
    name: "the whitespace run crosses a newline",
    text: "name:\ndescription: d",
    expected: "description: d",
  },
  {
    name: "an empty name at the end of the text is one space, trimmed",
    text: "name:   ",
    expected: "",
  },
  { name: "names: is not name:", text: "names: x", expected: undefined },
  { name: "the key is lower case", text: "Name: x", expected: undefined },
  { name: "the key starts the line", text: "  name: x", expected: undefined },
  { name: "no space before the colon", text: "name :x", expected: undefined },
  { name: "empty text", text: "", expected: undefined },
  { name: "CRLF line ending", text: "name: tdd\r\n", expected: "tdd" },
  {
    name: "CRLF crosses into the next line",
    text: "name:\r\ndescription: d\r\n",
    expected: "description: d",
  },
  { name: "double quotes", text: 'name: "tdd"', expected: "tdd" },
  { name: "single quotes", text: "name: 'tdd'", expected: "tdd" },
  { name: "mixed quote run", text: "name: '\"tdd\"'", expected: "tdd" },
  { name: "doubled quotes", text: 'name: ""tdd""', expected: "tdd" },
  { name: "inner quote stays", text: 'name: "a\'b"', expected: "a'b" },
  { name: "only quotes", text: 'name: ""', expected: "" },
  { name: "NBSP is trimmed", text: "name: " + NBSP + "tdd", expected: "tdd" },
  { name: "BOM is trimmed", text: "name: " + BOM + "tdd", expected: "tdd" },
  {
    name: "NEL is not trimmed",
    text: "name: " + NEL + "tdd",
    expected: NEL + "tdd",
  },
  {
    name: "a lone CR is not a line start",
    text: "x: y\rname: dup",
    expected: undefined,
  },
  {
    name: "an LF is a line start",
    text: "x: y\nname: dup",
    expected: "dup",
  },
  {
    name: "U+2028 stays in the name",
    text: "name: a" + LINE_SEPARATOR + "b",
    expected: "a" + LINE_SEPARATOR + "b",
  },
  {
    name: "the first match wins",
    text: "name: one\nname: two\n",
    expected: "one",
  },
];

for (const row of NAME_ROWS)
  test(`skillName: ${row.name}`, () => {
    assert.equal(skillName(row.text), row.expected);
  });

// --- the family ------------------------------------------------------------

test("the inline family names its rules in report order", () => {
  assert.equal(INLINE_FAMILY.name, "inline");
  assert.deepEqual(INLINE_FAMILY.rules, [
    "skill-md-present",
    "skill-frontmatter",
    "skill-name-unique",
  ]);
});

test("a clean clone reports nothing", async () => {
  await withSkillsRepo(async (root) => {
    assert.deepEqual(await inline(root), { failures: [] });
  });
});

// skill-md-present

test("an empty directory under skills has no SKILL.md", async () => {
  await withSkillsRepo(async (root) => {
    mkdir(root, "skills/empty");
    assert.deepEqual(await inline(root), {
      failures: [
        finding("skill-md-present", "skills/empty", "missing SKILL.md"),
      ],
    });
  });
});

test("every directory named skills is a parent, at any depth", async () => {
  await withSkillsRepo(async (root) => {
    mkdir(root, "deep/skills/x");
    assert.deepEqual(await inline(root), {
      failures: [
        finding("skill-md-present", "deep/skills/x", "missing SKILL.md"),
      ],
    });
  });
});

test("a child of skills without SKILL.md is reported, even when it holds skills", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(root, "skills/surfaces/codex/tdd/SKILL.md", skillMD("tdd"));
    assert.deepEqual(await inline(root), {
      failures: [
        finding("skill-md-present", "skills/surfaces", "missing SKILL.md"),
      ],
    });
  });
});

test("a directory named SKILL.md passes and is not read", async () => {
  await withSkillsRepo(async (root) => {
    mkdir(root, "skills/dirskill/SKILL.md");
    assert.deepEqual(await inline(root), { failures: [] });
  });
});

test("a regular file under skills is not a skill directory", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(root, "skills/note.txt", "x");
    assert.deepEqual(await inline(root), { failures: [] });
  });
});

test("a linked directory under skills is not a skill directory", async (t) => {
  await withSkillsRepo(async (root) => {
    await withOutside(async (outside) => {
      if (!linkOrSkip(t, root, "skills/linked", outside, "dir")) return;
      assert.deepEqual(await inline(root), { failures: [] });
    });
  });
});

for (const skipped of [".git", "node_modules", ".claude", "tests"])
  test(`a skills directory under ${skipped} is skipped and one under other is not`, async () => {
    await withSkillsRepo(async (root) => {
      mkdir(root, `${skipped}/skills/a`);
      assert.deepEqual(await inline(root), { failures: [] });
      mkdir(root, "other/skills/a");
      assert.deepEqual(await inline(root), {
        failures: [
          finding("skill-md-present", "other/skills/a", "missing SKILL.md"),
        ],
      });
    });
  });

test("eval snapshots at the top level are not plugin content", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(
      root,
      "evals/baselines/run/skills/tdd/SKILL.md",
      "not current skill metadata",
    );
    assert.deepEqual(await inline(root), { failures: [] });
  });
});

test("only the top-level evals directory is skipped", async () => {
  await withSkillsRepo(async (root) => {
    const rel = "deep/evals/baselines/run/skills/tdd/SKILL.md";
    writeFile(root, rel, "not current skill metadata");
    assert.deepEqual(await inline(root), {
      failures: [missingFrontmatter(rel)],
    });
  });
});

// skill-frontmatter

// The walk visits a directory's children before its next sibling, so `a/...` comes before `a-x/...`; the default sort
// puts `a-x` first because `-` sorts before `/`.
test("findings follow the default sort of their paths, not the walk order", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(root, "skills/a/SKILL.md", "---\nname: a\n---\n");
    writeFile(root, "skills/a-x/SKILL.md", "---\nname: a-x\n---\n");
    mkdirSync(path.join(root, "p", "skills", "empty"), { recursive: true });
    mkdirSync(path.join(root, "p-x", "skills", "empty"), { recursive: true });
    const { failures } = await inline(root);
    assert.deepEqual(
      failures.filter((failure) => failure.rule === "skill-frontmatter"),
      [
        missingFrontmatter("skills/a-x/SKILL.md"),
        missingFrontmatter("skills/a/SKILL.md"),
      ],
    );
    assert.deepEqual(
      failures
        .filter((failure) => failure.rule === "skill-md-present")
        .map((failure) => failure.path),
      ["p-x/skills/empty", "p/skills/empty"],
    );
  });
});

test("a FrontmatterError names itself in its string form", () => {
  assert.equal(String(new FrontmatterError("bad")), "FrontmatterError: bad");
});

test("a skill without a version line is reported", async () => {
  await withSkillsRepo(async (root) => {
    mutateFile(root, "skills/docs/SKILL.md", 'version: "1.0.0"\n', "");
    assert.deepEqual(await inline(root), {
      failures: [missingFrontmatter("skills/docs/SKILL.md")],
    });
  });
});

for (const newline of ["\n", "\r\n"])
  for (const field of ["name", "description", "version"])
    for (const lineNumber of [20, 21])
      test(`${field} on document line ${lineNumber} with ${JSON.stringify(newline)}`, async () => {
        await withSkillsRepo(async (root) => {
          const values: Record<string, string> = {
            name: "probe",
            description: "A probe.",
            version: "1.0.0",
          };
          const lines = ["---"];
          for (const other of ["name", "description", "version"])
            if (other !== field) lines.push(`${other}: ${values[other]}`);
          while (lines.length < lineNumber - 1) lines.push("# Padding");
          lines.push(`${field}: ${values[field]}`, "---", "Body.");
          writeFile(root, "skills/probe/SKILL.md", lines.join(newline));
          const expected =
            lineNumber > 20
              ? [missingFrontmatter("skills/probe/SKILL.md")]
              : [];
          assert.deepEqual(await inline(root), { failures: expected });
        });
      });

test("required fields in the body do not count", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(
      root,
      "skills/probe/SKILL.md",
      [
        "---",
        "metadata: {}",
        "---",
        "name: probe",
        "description: d",
        "version: 1",
        "",
      ].join("\n"),
    );
    assert.deepEqual(await inline(root), {
      failures: [missingFrontmatter("skills/probe/SKILL.md")],
    });
  });
});

for (const [label, nameLine, fires] of [
  ["an indented name", "  name: x", true],
  ["a space before the colon", "name : x", true],
  ["a capital N", "Name: x", true],
  ["names: alone", "names: x", true],
  ["no space after the colon", "name:x", false],
] as const)
  test(`frontmatter prefix: ${label}`, async () => {
    await withSkillsRepo(async (root) => {
      writeFile(root, "skills/probe/SKILL.md", skillFile(nameLine));
      assert.deepEqual(await inline(root), {
        failures: fires ? [missingFrontmatter("skills/probe/SKILL.md")] : [],
      });
    });
  });

test("an unclosed block counts as empty", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(
      root,
      "skills/probe/SKILL.md",
      "---\nname: probe\ndescription: d\nversion: 1\n",
    );
    assert.deepEqual(await inline(root), {
      failures: [missingFrontmatter("skills/probe/SKILL.md")],
    });
  });
});

test("a BOM before the opener is reported", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(root, "skills/probe/SKILL.md", BOM + skillFile("name: probe"));
    assert.deepEqual(await inline(root), {
      failures: [missingFrontmatter("skills/probe/SKILL.md")],
    });
  });
});

// skill-name-unique

test("a second skill with the same name is reported against the first", async () => {
  await withSkillsRepo(async (root) => {
    addSkill(root, "tdd");
    writeFile(root, "skills/tdd2/SKILL.md", skillFile("name: tdd"));
    assert.deepEqual(await inline(root), {
      failures: [
        duplicate("tdd", "skills/tdd/SKILL.md", "skills/tdd2/SKILL.md"),
      ],
    });
  });
});

test("the second and third skill both name the first", async () => {
  await withSkillsRepo(async (root) => {
    for (const dir of ["one", "two", "three"])
      writeFile(root, `skills/${dir}/SKILL.md`, skillFile("name: same"));
    assert.deepEqual(await inline(root), {
      failures: [
        duplicate("same", "skills/one/SKILL.md", "skills/three/SKILL.md"),
        duplicate("same", "skills/one/SKILL.md", "skills/two/SKILL.md"),
      ],
    });
  });
});

test("a surfaces mirror is exempt from uniqueness but not from frontmatter", async () => {
  await withSkillsRepo(async (root) => {
    addSkill(root, "tdd");
    const mirror = "skills/surfaces/codex/tdd/SKILL.md";
    writeFile(root, mirror, skillMD("tdd"));
    const parent = finding(
      "skill-md-present",
      "skills/surfaces",
      "missing SKILL.md",
    );
    assert.deepEqual(await inline(root), { failures: [parent] });
    mutateFile(root, mirror, 'version: "1.0.0"\n', "");
    assert.deepEqual(await inline(root), {
      failures: [parent, missingFrontmatter(mirror)],
    });
  });
});

test("quotes and padding do not hide a collision", async () => {
  await withSkillsRepo(async (root) => {
    addSkill(root, "tdd");
    writeFile(root, "skills/tdd2/SKILL.md", skillFile("name: '\"tdd\"'"));
    writeFile(root, "skills/tdd3/SKILL.md", skillFile("name: " + NBSP + "tdd"));
    assert.deepEqual(await inline(root), {
      failures: [
        duplicate("tdd", "skills/tdd/SKILL.md", "skills/tdd2/SKILL.md"),
        duplicate("tdd", "skills/tdd/SKILL.md", "skills/tdd3/SKILL.md"),
      ],
    });
  });
});

test("a leading BOM does not hide a collision", async () => {
  await withSkillsRepo(async (root) => {
    addSkill(root, "tdd");
    writeFile(root, "skills/tdd2/SKILL.md", skillFile("name: " + BOM + "tdd"));
    assert.deepEqual(await inline(root), {
      failures: [
        duplicate("tdd", "skills/tdd/SKILL.md", "skills/tdd2/SKILL.md"),
      ],
    });
  });
});

test("a leading NEL makes a different name", async () => {
  await withSkillsRepo(async (root) => {
    addSkill(root, "tdd");
    writeFile(root, "skills/tdd2/SKILL.md", skillFile("name: " + NEL + "tdd"));
    assert.deepEqual(await inline(root), { failures: [] });
  });
});

test("a bare name line captures the next line", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(
      root,
      "other/skills/c/SKILL.md",
      skillFile('name: "description: d"'),
    );
    writeFile(
      root,
      "skills/a/SKILL.md",
      ["---", "name:", "description: d", 'version: "1"', "---", ""].join("\n"),
    );
    assert.deepEqual(await inline(root), {
      failures: [
        duplicate(
          "description: d",
          "other/skills/c/SKILL.md",
          "skills/a/SKILL.md",
        ),
      ],
    });
  });
});

test("two empty names collide", async () => {
  await withSkillsRepo(async (root) => {
    for (const dir of ["e1", "e2"])
      writeFile(
        root,
        `skills/${dir}/SKILL.md`,
        ["---", "description: d", 'version: "1"', "name:   ", "---", ""].join(
          "\n",
        ),
      );
    assert.deepEqual(await inline(root), {
      failures: [duplicate("", "skills/e1/SKILL.md", "skills/e2/SKILL.md")],
    });
  });
});

test("a lone CR before name: is not a line start", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(
      root,
      "skills/cr1/SKILL.md",
      [
        "---",
        "x: y\rname: dup",
        "name: own",
        "description: d",
        "version: 1",
        "---",
        "",
      ].join("\n"),
    );
    writeFile(root, "skills/cr2/SKILL.md", skillFile("name: dup"));
    assert.deepEqual(await inline(root), { failures: [] });
  });
});

test("a name with U+2028 collides and is quoted as JSON quotes it", async () => {
  await withSkillsRepo(async (root) => {
    for (const dir of ["ls1", "ls2"])
      writeFile(
        root,
        `skills/${dir}/SKILL.md`,
        skillFile("name: a" + LINE_SEPARATOR + "b"),
      );
    assert.deepEqual(await inline(root), {
      failures: [
        finding(
          "skill-name-unique",
          "skills/ls2/SKILL.md",
          `duplicate skill name "a${LINE_SEPARATOR}b": skills/ls1/SKILL.md and skills/ls2/SKILL.md`,
        ),
      ],
    });
  });
});

test("an unclosed block gives no name, only the frontmatter finding", async () => {
  await withSkillsRepo(async (root) => {
    for (const dir of ["u1", "u2"])
      writeFile(
        root,
        `skills/${dir}/SKILL.md`,
        "---\nname: same\ndescription: d\nversion: 1\n",
      );
    assert.deepEqual(await inline(root), {
      failures: [
        missingFrontmatter("skills/u1/SKILL.md"),
        missingFrontmatter("skills/u2/SKILL.md"),
      ],
    });
  });
});

test("the findings come as presence, then frontmatter by path, then duplicate names", async () => {
  await withSkillsRepo(async (root) => {
    mkdir(root, "skills/empty");
    const noVersion = ["---", "name: dup", "description: d", "---", ""].join(
      "\n",
    );
    writeFile(root, "skills/b1/SKILL.md", noVersion);
    writeFile(root, "skills/b2/SKILL.md", noVersion);
    assert.deepEqual(await inline(root), {
      failures: [
        finding("skill-md-present", "skills/empty", "missing SKILL.md"),
        missingFrontmatter("skills/b1/SKILL.md"),
        missingFrontmatter("skills/b2/SKILL.md"),
        duplicate("dup", "skills/b1/SKILL.md", "skills/b2/SKILL.md"),
      ],
    });
  });
});

// audit errors

test("a linked SKILL.md is an audit error", async (t) => {
  await withSkillsRepo(async (root) => {
    await withOutside(async (outside) => {
      const target = path.join(outside, "SKILL.md");
      writeFileSync(target, skillFile("name: outside"));
      if (!linkOrSkip(t, root, "skills/a/SKILL.md", target, "file")) return;
      mkdir(root, "skills/empty");
      assert.deepEqual(await inline(root), {
        failures: [],
        error:
          "skills: checking skills/a/SKILL.md: skills/a/SKILL.md: passes through a link",
      });
    });
  });
});

test("the walk never delivers a linked SKILL.md", async (t) => {
  await withSkillsRepo(async (root) => {
    await withOutside(async (outside) => {
      const target = path.join(outside, "SKILL.md");
      writeFileSync(target, skillFile("name: outside"));
      writeFile(root, "ordinary/real/SKILL.md", skillFile("name: real"));
      if (!linkOrSkip(t, root, "ordinary/linked/SKILL.md", target, "file"))
        return;
      assert.deepEqual(await inline(root), { failures: [] });
    });
  });
});

/**
 * An lstat seam that replaces `at` with a real link to `target` the first time `when` holds for an inspection of `at`,
 * just before that inspection runs. It stands in for a link that appears after an earlier check vetted the path, so
 * only the guarded read's own check can refuse it. Undefined when the host cannot create the link.
 */
function linkOnInspection(
  at: string,
  target: string,
  type: "file" | "dir",
  when: () => boolean,
): Seams["lstat"] | undefined {
  const staged = `${target}.staged-link`;
  try {
    symlinkSync(target, staged, type);
  } catch {
    return undefined;
  }
  const real = seams.lstat;
  let swapped = false;
  return (candidate) => {
    if (!swapped && candidate === at && when()) {
      swapped = true;
      rmSync(at, { recursive: true, force: true });
      renameSync(staged, at);
    }
    return real(candidate);
  };
}

test("a skills directory that becomes a link after the walk is refused before it is read", async (t) => {
  await withSkillsRepo(async (root) => {
    await withOutside(async (outside) => {
      writeFile(root, "extra/skills/a/SKILL.md", skillFile("name: extra"));
      mkdirSync(path.join(outside, "b"));
      const parent = path.join(root, "extra", "skills");
      // The walk lists the directory without inspecting it; the link appears before the first inspection.
      const lstat = linkOnInspection(parent, outside, "dir", () => true);
      if (lstat === undefined) {
        t.skip("the host cannot create a symbolic link");
        return;
      }
      assert.deepEqual(await withSeams({ lstat }, () => inline(root)), {
        failures: [],
        error:
          "skills: reading extra/skills: extra/skills: passes through a link",
      });
    });
  });
});

test("a SKILL.md that becomes a link after the presence check is refused by the inline read", async (t) => {
  await withSkillsRepo(async (root) => {
    await withOutside(async (outside) => {
      writeFile(root, "skills/a/SKILL.md", skillFile("name: a"));
      const target = path.join(outside, "SKILL.md");
      writeFileSync(target, "No frontmatter.\n");
      const file = path.join(root, "skills", "a", "SKILL.md");
      // The presence check inspects the file first; the link appears before the read inspects it again.
      let inspections = 0;
      const lstat = linkOnInspection(
        file,
        target,
        "file",
        () => (inspections += 1) === 2,
      );
      if (lstat === undefined) {
        t.skip("the host cannot create a symbolic link");
        return;
      }
      assert.deepEqual(await withSeams({ lstat }, () => inline(root)), {
        failures: [],
        error:
          "skills: reading skills/a/SKILL.md: skills/a/SKILL.md: passes through a link",
      });
    });
  });
});

test("a repository path that does not exist is the walk error", async () => {
  const missing = path.join(
    tmpdir(),
    `agentics-inline-missing-${process.pid}-${Date.now()}`,
  );
  const result = await inline(missing);
  assert.deepEqual(result.failures, []);
  assert.equal(result.error, ".: does not exist");
});

test("an unreadable SKILL.md is an audit error naming it and why", async () => {
  await withSkillsRepo(async (root) => {
    const result = await withSeams(
      {
        readFile: () => {
          throw Object.assign(new Error("denied"), { code: "EACCES" });
        },
      },
      () => inline(root),
    );
    assert.equal(
      String(result.error),
      "skills: reading skills/challenge/SKILL.md: skills/challenge/SKILL.md: EACCES",
    );
    assert.deepEqual(result.failures, []);
  });
});

test("an unreadable skills directory is an audit error", async () => {
  await withSkillsRepo(async (root) => {
    const skillsDir = path.join(root, "skills");
    const real = seams.readDir;
    let calls = 0;
    const result = await withSeams(
      {
        readDir: (dir) => {
          if (dir === skillsDir) {
            calls += 1;
            if (calls === 2)
              throw Object.assign(new Error("denied"), { code: "EACCES" });
          }
          return real(dir);
        },
      },
      () => inline(root),
    );
    assert.equal(
      String(result.error),
      "skills: reading skills: skills: EACCES",
    );
    assert.deepEqual(result.failures, []);
  });
});

// --- parseFrontmatter and the frontmatter family ----------------------------

const DOCS = "skills/docs/SKILL.md";
const EM_DASH = String.fromCodePoint(0x2014);
const BAD_SCALAR = "the top level is a scalar, not a mapping";

const yamlFinding = (at: string, message: string): Failure =>
  finding(
    "frontmatter-yaml",
    at,
    `frontmatter is not a YAML mapping: ${message}`,
  );

const frontmatterOnly = (root: string) =>
  run(root, options, { families: [FRONTMATTER_FAMILY] });
const evalMetadata = (root: string) =>
  run(root, options, { families: [EVAL_METADATA_FAMILY] });
const evalClean = (root: string) =>
  run(root, options, { families: [EVAL_FRONTMATTER_CLEAN_FAMILY] });

const FOOTER_MESSAGE =
  "retired '## Validation history' footer found " +
  EM_DASH +
  " record evals in evals/skills.yaml instead";
const ZERO_ASSETS_ERROR =
  "skills: no SKILL.md assets found " + EM_DASH + " nothing to audit";

test("the frontmatter families name their rules in report order", () => {
  assert.equal(FRONTMATTER_FAMILY.name, "frontmatter");
  assert.deepEqual(FRONTMATTER_FAMILY.rules, ["frontmatter-yaml"]);
  assert.equal(EVAL_METADATA_FAMILY.name, "eval-metadata");
  assert.deepEqual(EVAL_METADATA_FAMILY.rules, [
    "eval-frontmatter-present",
    "eval-history-footer",
  ]);
  assert.equal(EVAL_FRONTMATTER_CLEAN_FAMILY.name, "eval-frontmatter-clean");
  assert.deepEqual(EVAL_FRONTMATTER_CLEAN_FAMILY.rules, [
    "eval-frontmatter-clean",
  ]);
});

const rejects = (block: string, message: string | RegExp): void => {
  assert.throws(
    () => parseFrontmatter(block),
    (error: unknown) => {
      assert.ok(error instanceof FrontmatterError, String(error));
      if (typeof message === "string") assert.equal(error.message, message);
      else assert.match(error.message, message);
      assert.ok(!error.message.includes("\n"), error.message);
      return true;
    },
  );
};

test("parseFrontmatter reads a plain mapping of strings", () => {
  const doc = parseFrontmatter("name: docs\ndescription: Does x.\n");
  assert.equal(doc.kind, "map");
  assert.deepEqual(
    doc.pairs.map((pair) => [pair.key, stringText(pair.value)]),
    [
      ["name", "docs"],
      ["description", "Does x."],
    ],
  );
  assert.deepEqual([...doc.entries.keys()], ["name", "description"]);
  assert.equal(doc.entries.get("name"), doc.pairs[0]?.value);
});

test("parseFrontmatter reads quoted keys, an empty key and every plain scalar type", () => {
  const doc = parseFrontmatter(
    '"name": x\n\'other\': y\n"": z\n"1": one\nn: 3\nb: true\nz: ~\ne:\n',
  );
  assert.deepEqual(
    doc.pairs.map((pair) => pair.key),
    ["name", "other", "", "1", "n", "b", "z", "e"],
  );
  assert.equal(stringText(doc.entries.get("name")), "x");
  const values = ["n", "b", "z", "e"].map((key) => {
    const node = doc.entries.get(key);
    assert.ok(isScalar(node));
    return [node.value, node.text];
  });
  assert.deepEqual(values, [
    [3, "3"],
    [true, "true"],
    [null, "~"],
    [null, ""],
  ]);
});

test("parseFrontmatter reads an explicit key with no value as an empty value, like a bare key", () => {
  const doc = parseFrontmatter("? name\nbare:\n");
  const empty = { kind: "scalar", value: null, text: "" };
  assert.deepEqual(doc.entries.get("name"), empty);
  assert.deepEqual(doc.entries.get("bare"), empty);
});

test("parseFrontmatter reads nested mappings, sequences and block scalars", () => {
  const doc = parseFrontmatter(
    "metadata:\n  surfaces: [a, b]\n  deep:\n    - k: v\ntext: |\n  one\n  two\n",
  );
  const surfaces = lookup(doc, "metadata", "surfaces");
  assert.ok(isSeq(surfaces));
  assert.deepEqual(surfaces.items.map(stringText), ["a", "b"]);
  const deep = lookup(doc, "metadata", "deep");
  assert.ok(isSeq(deep));
  assert.ok(isMap(deep.items[0]));
  assert.equal(stringText(lookup(deep.items[0], "k")), "v");
  assert.equal(stringText(lookup(doc, "text")), "one\ntwo\n");
});

test("the node helpers tell strings, booleans and collections apart", () => {
  const doc = parseFrontmatter(
    'text: "true"\nflag: false\ncount: 0x10\nnone: null\nlist: [a, 1, [b], {k: ~}]\n',
  );
  const at = (key: string) => doc.entries.get(key);
  assert.equal(stringText(at("text")), "true");
  for (const key of ["flag", "count", "none", "list", "missing"])
    assert.equal(stringText(at(key)), undefined, key);
  assert.equal(stringText(doc), undefined);
  assert.deepEqual(
    ["text", "flag", "count", "none", "list", "missing"].map((key) =>
      isBoolean(at(key)),
    ),
    [false, true, false, false, false, false],
  );
  assert.deepEqual(
    ["text", "list", "missing"].map((key) => [
      isScalar(at(key)),
      isSeq(at(key)),
      isMap(at(key)),
    ]),
    [
      [true, false, false],
      [false, true, false],
      [false, false, false],
    ],
  );
  assert.equal(isMap(doc), true);
  const count = at("count");
  assert.ok(isScalar(count));
  assert.deepEqual([count.value, count.text], [16, "0x10"]);
});

test("lookup follows mapping keys and stops at anything else", () => {
  const doc = parseFrontmatter("a:\n  b:\n    c: x\nlist: [1]\n");
  assert.equal(stringText(lookup(doc, "a", "b", "c")), "x");
  assert.equal(lookup(doc), doc);
  assert.equal(lookup(doc, "a", "missing", "c"), undefined);
  assert.equal(lookup(doc, "list", "0"), undefined);
  assert.equal(lookup(doc, "a", "b", "c", "d"), undefined);
  assert.equal(lookup(undefined, "a"), undefined);
});

test("printable escapes control characters and keeps tabs and other text", () => {
  assert.equal(
    printable(
      "a\u0000\u0008\tb\n\r\u001b\u007f\u0080\u009f\u00a0\u2028\u2029\u{1f600}é",
    ),
    "a\\u0000\\u0008\tb\\u000a\\u000d\\u001b\\u007f\\u0080\\u009f\u00a0\\u2028\\u2029\u{1f600}é",
  );
  assert.equal(printable(""), "");
});

test("render prints a value for a message", () => {
  const doc = parseFrontmatter(
    'a: text\nb: 0x10\nc: 1.50\nd: true\ne: ~\nf: [x, 1, [y]]\ng: {k: v, n: {m: 2}}\nh: ""\ni: .inf\nj: -0\n',
  );
  assert.deepEqual(
    doc.pairs.map((pair) => render(pair.value)),
    [
      "text",
      "16",
      "1.5",
      "true",
      "null",
      "[x, 1, [y]]",
      "{k: v, n: {m: 2}}",
      "",
      "Infinity",
      "0",
    ],
  );
  assert.equal(render(doc).startsWith("{a: text, b: 16, "), true);
});

test("render escapes non-printable characters and leaves the value itself alone", () => {
  const doc = parseFrontmatter(
    'a: \u001b[31mred\nb: "x\\ny\\tz\\x7f\\x85\\u2028\\u2029"\nc: ["\\e"]\nd: {"k\\e": "\\0"}\n',
  );
  assert.deepEqual(
    doc.pairs.map((pair) => render(pair.value)),
    [
      "\\u001b[31mred",
      "x\\u000ay\tz\\u007f\\u0085\\u2028\\u2029",
      "[\\u001b]",
      "{k\\u001b: \\u0000}",
    ],
  );
  assert.equal(stringText(doc.entries.get("a")), "\u001b[31mred");
  assert.equal(
    stringText(doc.entries.get("b")),
    "x\ny\tz\u007f\u0085\u2028\u2029",
  );
});

test("a refusal escapes non-printable characters from the block", () => {
  rejects(
    "a: !<x\u001b[31mred> 1\n",
    "Unresolved tag: x\\u001b[31mred (block line 1)",
  );
  rejects(
    "a: !<x\u0085\u009b\u007f\u0000> 1\n",
    "Unresolved tag: x\\u0085\\u009b\\u007f\\u0000 (block line 1)",
  );
});

test("frontmatter-yaml prints an escape character in a tag name as an escape", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(root, DOCS, "---\na: !<x\u001b[2J> 1\n---\nBody.\n");
    assert.deepEqual(await frontmatterOnly(root), {
      failures: [
        yamlFinding(DOCS, "Unresolved tag: x\\u001b[2J (block line 1)"),
      ],
    });
  });
});

const MULTIPLE_DOCS =
  "Source contains multiple documents; please use YAML.parseAllDocuments()";

const REJECTED: ReadonlyArray<[string, string, string]> = [
  [
    "a duplicate key",
    "name: a\nmetadata: {}\nname: b\n",
    "Map keys must be unique (block line 3)",
  ],
  [
    "a nested duplicate key",
    "m:\n  k: 1\n  k: 2\n",
    "Map keys must be unique (block line 3)",
  ],
  ["an anchor", "a: &x 1\n", "anchors are not allowed (block line 1)"],
  [
    "an anchor on a collection",
    "a: 1\nb: &x [1]\n",
    "anchors are not allowed (block line 2)",
  ],
  ["an anchor on a key", "&k a: 1\n", "anchors are not allowed (block line 1)"],
  ["an alias", "a:\n  - *x\n", "aliases are not allowed (block line 2)"],
  [
    "an alias as a key",
    "*x : 1\n",
    "mapping keys must be strings (block line 1)",
  ],
  ["a custom tag", "a: !foo 1\n", "Unresolved tag: !foo (block line 1)"],
  [
    "a standard explicit tag",
    "a: !!str 1\n",
    "explicit tags are not allowed (block line 1)",
  ],
  [
    "the non-specific tag",
    "a: 1\nb: ! 2\n",
    "explicit tags are not allowed (block line 2)",
  ],
  [
    "a binary tag",
    "a: !!binary aGk=\n",
    "explicit tags are not allowed (block line 1)",
  ],
  [
    "a set tag",
    "a: !!set {x}\n",
    "explicit tags are not allowed (block line 1)",
  ],
  [
    "a tag on the top-level mapping",
    "!!map {a: 1}\n",
    "explicit tags are not allowed (block line 1)",
  ],
  [
    "a tag on a key",
    "!!str a: 1\n",
    "explicit tags are not allowed (block line 1)",
  ],
  [
    "a top-level integer key",
    "name: x\n1: a\n",
    "mapping keys must be strings (block line 2)",
  ],
  [
    "a top-level bool key",
    "true: a\n",
    "mapping keys must be strings (block line 1)",
  ],
  [
    "a top-level null key",
    "~: a\n",
    "mapping keys must be strings (block line 1)",
  ],
  [
    "a top-level missing key",
    ": a\n",
    "mapping keys must be strings (block line 1)",
  ],
  [
    "a nested integer key",
    "m:\n  k:\n    1: b\n",
    "mapping keys must be strings (block line 3)",
  ],
  [
    "a non-string key inside a sequence",
    "m:\n  - {1.5: b}\n",
    "mapping keys must be strings (block line 2)",
  ],
  [
    "a complex key",
    "? [a]\n: b\n",
    "mapping keys must be strings (block line 1)",
  ],
  ["a second document", "a: 1\n...\nb: 2\n", `${MULTIPLE_DOCS} (block line 3)`],
  [
    "a document start marker with a trailing space",
    "a: 1\n--- \nb: 2\n",
    `${MULTIPLE_DOCS} (block line 2)`,
  ],
  [
    "an unclosed flow sequence",
    "a: 1\nb: [1, 2\nc: 3\n",
    "Flow sequence in block collection must be sufficiently indented and end with a ] (block line 3)",
  ],
  [
    "a tab as indentation",
    "a:\n\tb: 1\n",
    "Tabs are not allowed as indentation (block line 2)",
  ],
  [
    "a directive",
    "%TAG !e! tag:example.com,2000:\na: !e!foo 1\n",
    "Missing directives-end/doc-start indicator line (block line 2)",
  ],
];

for (const [label, block, message] of REJECTED)
  test(`parseFrontmatter rejects ${label}`, () => {
    rejects(block, message);
  });

for (const [label, block, message] of REJECTED)
  test(`frontmatter-yaml reports ${label}`, async () => {
    await withSkillsRepo(async (root) => {
      writeFile(root, DOCS, "---\n" + block + "---\nBody.\n");
      assert.deepEqual(await frontmatterOnly(root), {
        failures: [yamlFinding(DOCS, message)],
      });
    });
  });

test("a parser warning alone makes the block unparseable", () => {
  rejects("a: !!float x\n", /^Unresolved tag: tag:yaml\.org,2002:float /);
});

test("parseFrontmatter refuses a top level that is no mapping", () => {
  for (const block of ["", " ", "\n\n", "# only a comment", "~", "null"])
    rejects(block, "frontmatter holds no mapping");
  rejects("foo", "the top level is a scalar, not a mapping");
  rejects("12", "the top level is a scalar, not a mapping");
  rejects("- a\n- b", "the top level is a sequence, not a mapping");
});

test("parseFrontmatter fails closed on nesting too deep to read", () => {
  assert.throws(() => parseFrontmatter("a: " + "[".repeat(20000)));
});

test("plain scalars resolve by the YAML 1.2 core schema", () => {
  const doc = parseFrontmatter(
    "date: 2026-11-20\nunderscore: 1_000\nbinary: 0b101\nyes: yes\ninf: .InF\n" +
      "legacy: 0777\noctal: 0o17\nhex: 0x10\nzero: 08\nbig: 9007199254740993\nmerge: {<<: {k: v}}\n",
  );
  const values = Object.fromEntries(
    doc.pairs.map((pair) => [
      pair.key,
      isScalar(pair.value) ? pair.value.value : undefined,
    ]),
  );
  assert.deepEqual(values, {
    date: "2026-11-20",
    underscore: "1_000",
    binary: "0b101",
    yes: "yes",
    inf: ".InF",
    legacy: 777,
    octal: 15,
    hex: 16,
    zero: 8,
    big: 9007199254740992,
    merge: undefined,
  });
  assert.equal(stringText(lookup(doc, "merge", "<<", "k")), "v");
});

test("frontmatter-yaml: a clean clone reports nothing", async () => {
  await withSkillsRepo(async (root) => {
    assert.deepEqual(await frontmatterOnly(root), { failures: [] });
  });
});

test("frontmatter-yaml: a YAML syntax error fires on the file", async () => {
  await withSkillsRepo(async (root) => {
    mutateFile(
      root,
      DOCS,
      "description: Test skill for docs.",
      "description: [unclosed",
    );
    const result = await frontmatterOnly(root);
    assert.equal(result.error, undefined);
    assert.equal(result.failures.length, 1);
    const [only] = result.failures;
    assert.equal(only?.rule, "frontmatter-yaml");
    assert.equal(only?.path, DOCS);
    assert.ok(
      only?.message.startsWith("frontmatter is not a YAML mapping: "),
      only?.message,
    );
    assert.ok(
      only.message.length > "frontmatter is not a YAML mapping: ".length,
    );
  });
});

test("frontmatter-yaml: a duplicate key is folded to one line", async () => {
  await withSkillsRepo(async (root) => {
    mutateFile(root, DOCS, "metadata:", "name: docs\nmetadata:");
    assert.deepEqual(await frontmatterOnly(root), {
      failures: [yamlFinding(DOCS, "Map keys must be unique (block line 4)")],
    });
  });
});

for (const [label, block] of [
  ["one space", " "],
  ["a comment", "# only a comment"],
  ["a tilde", "~"],
  ["null", "null"],
] as const)
  test(`frontmatter-yaml: a block holding ${label} holds no mapping`, async () => {
    await withSkillsRepo(async (root) => {
      writeFile(root, DOCS, `---\n${block}\n---\nBody.\n`);
      assert.deepEqual(await frontmatterOnly(root), {
        failures: [yamlFinding(DOCS, "frontmatter holds no mapping")],
      });
    });
  });

for (const [label, block, tail] of [
  ["a short scalar", "foo", BAD_SCALAR],
  ["a long scalar", "abcdefghijklmnop", BAD_SCALAR],
  ["a number", "12", BAD_SCALAR],
  ["a sequence", "- a\n- b", "the top level is a sequence, not a mapping"],
] as const)
  test(`frontmatter-yaml: ${label} at the top level is no mapping`, async () => {
    await withSkillsRepo(async (root) => {
      writeFile(root, DOCS, `---\n${block}\n---\nBody.\n`);
      assert.deepEqual(await frontmatterOnly(root), {
        failures: [yamlFinding(DOCS, tail)],
      });
    });
  });

for (const text of [
  "---\n# Heading\n\n## Another\n",
  "---\nname: docs\n",
  "---\n\n",
])
  test(`frontmatter-yaml: an unclosed block reports once (${JSON.stringify(text)})`, async () => {
    await withSkillsRepo(async (root) => {
      writeFile(root, DOCS, text);
      assert.deepEqual(await frontmatterOnly(root), {
        failures: [
          finding(
            "frontmatter-yaml",
            DOCS,
            "frontmatter block is never closed by a bare ---",
          ),
        ],
      });
    });
  });

for (const [label, text] of [
  ["no block", "# Docs\n\nBody.\n"],
  ["a BOM before the opener", BOM + "---\na: 1\n---\n"],
  ["an empty closed block", "---\n---\nBody.\n"],
] as const)
  test(`frontmatter-yaml: ${label} is silent`, async () => {
    await withSkillsRepo(async (root) => {
      writeFile(root, DOCS, text);
      assert.deepEqual(await frontmatterOnly(root), { failures: [] });
    });
  });

test("frontmatter-yaml: findings follow the default sort order of the paths", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(root, "skills/zzz/SKILL.md", "---\nfoo\n---\n");
    writeFile(root, "skills/aaa/SKILL.md", "---\nfoo\n---\n");
    const result = await frontmatterOnly(root);
    assert.deepEqual(
      result.failures.map((failure) => failure.path),
      ["skills/aaa/SKILL.md", "skills/zzz/SKILL.md"],
    );
  });
});

test("frontmatter-yaml: a directory SKILL.md is an audit error", async () => {
  await withSkillsRepo(async (root) => {
    mkdir(root, "skills/a/SKILL.md");
    const result = await frontmatterOnly(root);
    assert.deepEqual(result.failures, []);
    assert.ok(
      result.error?.startsWith("skills: reading skills/a/SKILL.md: "),
      String(result.error),
    );
  });
});

test("frontmatter-yaml: a linked leaf SKILL.md is not selected", async (t) => {
  await withSkillsRepo(async (root) => {
    await withOutside(async (outside) => {
      const target = path.join(outside, "SKILL.md");
      writeFileSync(target, "---\nfoo\n---\n");
      if (!linkOrSkip(t, root, "skills/a/SKILL.md", target, "file")) return;
      assert.deepEqual(await frontmatterOnly(root), { failures: [] });
    });
  });
});

test("frontmatter-yaml: a SKILL.md that becomes a link after the asset listing is refused by the read", async (t) => {
  await withSkillsRepo(async (root) => {
    await withOutside(async (outside) => {
      writeFile(root, "skills/a/SKILL.md", skillFile("name: a"));
      const target = path.join(outside, "SKILL.md");
      writeFileSync(target, "---\nfoo\n---\n");
      const file = path.join(root, "skills", "a", "SKILL.md");
      // The asset listing inspects the file first; the link appears before the read inspects it again.
      let inspections = 0;
      const lstat = linkOnInspection(
        file,
        target,
        "file",
        () => (inspections += 1) === 2,
      );
      if (lstat === undefined) {
        t.skip("the host cannot create a symbolic link");
        return;
      }
      assert.deepEqual(
        await withSeams({ lstat }, () => frontmatterOnly(root)),
        {
          failures: [],
          error:
            "skills: reading skills/a/SKILL.md: skills/a/SKILL.md: passes through a link",
        },
      );
    });
  });
});

test("frontmatter-yaml: a linked child directory is not selected", async (t) => {
  await withSkillsRepo(async (root) => {
    await withOutside(async (outside) => {
      writeFileSync(path.join(outside, "SKILL.md"), "---\nfoo\n---\n");
      if (!linkOrSkip(t, root, "skills/linked", outside, "dir")) return;
      assert.deepEqual(await frontmatterOnly(root), { failures: [] });
    });
  });
});

test("frontmatter-yaml: a linked skills directory is not selected", async (t) => {
  await withSkillsRepo(async (root) => {
    await withOutside(async (outside) => {
      mkdirSync(path.join(outside, "x"));
      writeFileSync(path.join(outside, "x", "SKILL.md"), "---\nfoo\n---\n");
      rmSync(path.join(root, "skills"), { recursive: true, force: true });
      if (!linkOrSkip(t, root, "skills", outside, "dir")) return;
      assert.deepEqual(await frontmatterOnly(root), { failures: [] });
    });
  });
});

// --- eval-metadata ------------------------------------------------------------

const presentFinding = finding(
  "eval-frontmatter-present",
  DOCS,
  "empty or missing frontmatter",
);

for (const [label, text] of [
  ["no frontmatter", "# Docs\n\nBody.\n"],
  ["an empty closed block", "---\n---\nBody.\n"],
  ["a block holding one blank line", "---\n\n---\nBody.\n"],
  ["a BOM before the opener", BOM + "---\na: 1\n---\nBody.\n"],
] as const)
  test(`eval-frontmatter-present: ${label} fires`, async () => {
    await withSkillsRepo(async (root) => {
      writeFile(root, DOCS, text);
      assert.deepEqual(await evalMetadata(root), {
        failures: [presentFinding],
      });
    });
  });

test("eval-frontmatter-present: an unclosed block with content does not fire", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(root, DOCS, "---\nname: docs\n");
    assert.deepEqual(await evalMetadata(root), { failures: [] });
  });
});

test("eval-metadata: a clean clone reports nothing", async () => {
  await withSkillsRepo(async (root) => {
    assert.deepEqual(await evalMetadata(root), { failures: [] });
  });
});

test("eval-history-footer: a footer in the body fires once with the footer message", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(
      root,
      DOCS,
      skillMD("docs") + "\n## Validation history\n\n- 2026-08-09 passed\n",
    );
    assert.deepEqual(await evalMetadata(root), {
      failures: [finding("eval-history-footer", DOCS, FOOTER_MESSAGE)],
    });
  });
});

const FOOTER_ROWS: ReadonlyArray<{
  name: string;
  text: string;
  fires: boolean;
}> = [
  {
    name: "a suffix after the heading",
    text: skillMD("docs") + "\n## Validation historyX\n",
    fires: true,
  },
  {
    name: "the heading inside a code fence",
    text: skillMD("docs") + "\n```\n## Validation history\n```\n",
    fires: true,
  },
  {
    name: "the heading at the very start of the file",
    text: "## Validation history\n---\nname: docs\n---\n",
    fires: true,
  },
  {
    name: "a CRLF file",
    text:
      skillMD("docs").replaceAll("\n", "\r\n") +
      "\r\n## Validation history\r\n",
    fires: true,
  },
  {
    name: "two spaces before the heading",
    text: skillMD("docs") + "\n  ## Validation history\n",
    fires: false,
  },
  {
    name: "a lower case heading",
    text: skillMD("docs") + "\n## validation history\n",
    fires: false,
  },
  {
    name: "a lone CR before the heading",
    text: skillMD("docs") + "\r## Validation history\n",
    fires: false,
  },
  {
    name: "U+2028 before the heading",
    text: skillMD("docs") + LINE_SEPARATOR + "## Validation history\n",
    fires: false,
  },
];

for (const row of FOOTER_ROWS)
  test(`eval-history-footer: ${row.name} ${row.fires ? "fires" : "does not fire"}`, async () => {
    await withSkillsRepo(async (root) => {
      writeFile(root, DOCS, row.text);
      const failures = (await evalMetadata(root)).failures.filter(
        (failure) => failure.rule === "eval-history-footer",
      );
      assert.equal(failures.length, row.fires ? 1 : 0);
    });
  });

test("eval-metadata: per file the order is present then footer, files in default sort order", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(root, "skills/zzz/SKILL.md", "## Validation history\n");
    writeFile(root, "skills/aaa/SKILL.md", "## Validation history\n");
    assert.deepEqual(await evalMetadata(root), {
      failures: [
        finding(
          "eval-frontmatter-present",
          "skills/aaa/SKILL.md",
          "empty or missing frontmatter",
        ),
        finding("eval-history-footer", "skills/aaa/SKILL.md", FOOTER_MESSAGE),
        finding(
          "eval-frontmatter-present",
          "skills/zzz/SKILL.md",
          "empty or missing frontmatter",
        ),
        finding("eval-history-footer", "skills/zzz/SKILL.md", FOOTER_MESSAGE),
      ],
    });
  });
});

test("eval-metadata: zero assets is the only audit error of the three families", async () => {
  await withSkillsRepo(async (root) => {
    rmSync(path.join(root, "skills"), { recursive: true, force: true });
    assert.deepEqual(await evalMetadata(root), {
      failures: [],
      error: ZERO_ASSETS_ERROR,
    });
    assert.deepEqual(await frontmatterOnly(root), { failures: [] });
    assert.deepEqual(await evalClean(root), { failures: [] });
  });
});

test("eval-metadata: a linked skills directory has zero assets", async (t) => {
  await withSkillsRepo(async (root) => {
    await withOutside(async (outside) => {
      mkdirSync(path.join(outside, "x"));
      writeFileSync(path.join(outside, "x", "SKILL.md"), skillMD("x"));
      rmSync(path.join(root, "skills"), { recursive: true, force: true });
      if (!linkOrSkip(t, root, "skills", outside, "dir")) return;
      assert.deepEqual(await evalMetadata(root), {
        failures: [],
        error: ZERO_ASSETS_ERROR,
      });
      assert.deepEqual(await frontmatterOnly(root), { failures: [] });
      assert.deepEqual(await evalClean(root), { failures: [] });
    });
  });
});

test("eval-metadata: a directory SKILL.md is an audit error", async () => {
  await withSkillsRepo(async (root) => {
    mkdir(root, "skills/a/SKILL.md");
    const result = await evalMetadata(root);
    assert.deepEqual(result.failures, []);
    assert.ok(
      result.error?.startsWith("skills: reading skills/a/SKILL.md: "),
      String(result.error),
    );
  });
});

// --- eval-frontmatter-clean ---------------------------------------------------

const cleanFinding = (key: string, at: string = DOCS): Failure =>
  finding(
    "eval-frontmatter-clean",
    at,
    `frontmatter holds ${key}; eval results belong in evals/skills.yaml, not in the skill file`,
  );

/** Replaces the docs skill with a closed block holding the three required keys, then `extra`, and a body. */
function withFrontmatter(root: string, extra: string): void {
  writeFile(
    root,
    DOCS,
    '---\nname: docs\ndescription: Test skill for docs.\nversion: "1.0.0"\n' +
      extra +
      "---\n\n# docs\n\nBody.\n",
  );
}

const CLEAN_MESSAGE =
  /^frontmatter holds (.*); eval results belong in evals\/skills\.yaml, not in the skill file$/s;

/** The paths `eval-frontmatter-clean` reports for the docs skill given these frontmatter lines. */
async function cleanPaths(extra: string): Promise<string[]> {
  return await withSkillsRepo(async (root) => {
    withFrontmatter(root, extra);
    const result = await evalClean(root);
    assert.equal(result.error, undefined);
    return result.failures.map((failure) => {
      assert.equal(failure.rule, "eval-frontmatter-clean");
      assert.equal(failure.path, DOCS);
      const match = CLEAN_MESSAGE.exec(failure.message);
      assert.ok(match, failure.message);
      return match[1] ?? "";
    });
  });
}

const MOVED_KEY_CASES: ReadonlyArray<[string, string]> = [
  [
    "metadata.last-evaluated",
    "metadata:\n    surfaces: [claude-code]\n    last-evaluated: 2026-08-09\n",
  ],
  [
    "metadata.eval-status",
    "metadata:\n    surfaces: [claude-code]\n    eval-status: passing\n",
  ],
  [
    "metadata.eval-digest",
    "metadata:\n    surfaces: [claude-code]\n    eval-digest: abc\n",
  ],
  [
    "metadata.model-ladders-reviewed",
    "metadata:\n    surfaces: [claude-code]\n    model-ladders-reviewed: 2026-08-09\n",
  ],
  ["eval-status", "eval-status: passing\n"],
  [
    "metadata.evals.eval-digest",
    "metadata:\n    surfaces: [claude-code]\n    evals:\n        eval-digest: abc\n",
  ],
  ["history[0].last-evaluated", "history:\n    - last-evaluated: 2026-08-09\n"],
];

for (const [key, extra] of MOVED_KEY_CASES)
  test(`eval-frontmatter-clean: ${key}`, async () => {
    await withSkillsRepo(async (root) => {
      withFrontmatter(root, extra);
      assert.deepEqual(await evalClean(root), {
        failures: [cleanFinding(key)],
      });
    });
  });

test("eval-frontmatter-clean: a clean clone reports nothing", async () => {
  await withSkillsRepo(async (root) => {
    assert.deepEqual(await evalClean(root), { failures: [] });
  });
});

test("eval-frontmatter-clean reads keys only", async () => {
  assert.deepEqual(
    await cleanPaths(
      "x: eval-status last-evaluated eval-digest model-ladders-reviewed\n",
    ),
    [],
  );
  await withSkillsRepo(async (root) => {
    writeFile(
      root,
      DOCS,
      skillMD("docs") + "\n    eval-status: passing\neval-status: passing\n",
    );
    assert.deepEqual(await evalClean(root), { failures: [] });
  });
  for (const text of [
    "---\ndescription: [unclosed\neval-status: passing\n---\n",
    "---\nname: docs\neval-status: passing\n",
    "# no block\neval-status: passing\n",
  ])
    await withSkillsRepo(async (root) => {
      writeFile(root, DOCS, text);
      assert.deepEqual(await evalClean(root), { failures: [] });
    });
});

const KEY_PATH_ROWS: ReadonlyArray<[string, string, string[]]> = [
  [
    "nested sequences",
    "x:\n  - - last-evaluated: 1\n  - - last-evaluated: 2\n",
    ["x[0][0].last-evaluated", "x[1][0].last-evaluated"],
  ],
  [
    "three levels of sequence",
    "a: [[[{last-evaluated: 1}]]]\n",
    ["a[0][0][0].last-evaluated"],
  ],
  ["a plain string item in a list", "x:\n  - eval-status\n", []],
  [
    "a moved key holding another",
    "metadata:\n  eval-status:\n    eval-digest: 1\n",
    ["metadata.eval-status", "metadata.eval-status.eval-digest"],
  ],
  ["a different case", "Eval-Status: 1\n", []],
];

for (const [label, extra, expected] of KEY_PATH_ROWS)
  test(`eval-frontmatter-clean paths: ${label}`, async () => {
    assert.deepEqual(await cleanPaths(extra), expected);
  });

const UNUSUAL_KEY_ROWS: ReadonlyArray<[string, string, string[]]> = [
  [
    "an empty string key",
    'm:\n  "":\n    eval-status: x\n',
    ["m..eval-status"],
  ],
  ["a top-level empty string key", '"":\n  eval-status: x\n', ["eval-status"]],
  [
    "a quoted number key",
    'm:\n  "1":\n    eval-status: x\n',
    ["m.1.eval-status"],
  ],
  [
    "a date key, which is a string",
    "m:\n  2026-01-01:\n    eval-status: x\n",
    ["m.2026-01-01.eval-status"],
  ],
  [
    "a merge key, which is an ordinary key",
    "<<:\n  eval-status: 1\n",
    ["<<.eval-status"],
  ],
];

for (const [label, extra, expected] of UNUSUAL_KEY_ROWS)
  test(`eval-frontmatter-clean unusual keys: ${label}`, async () => {
    assert.deepEqual(await cleanPaths(extra), expected);
  });

test("eval-frontmatter-clean prints a key holding a newline and an escape character on one printable line", async () => {
  await withSkillsRepo(async (root) => {
    withFrontmatter(
      root,
      '"a\\nFAIL skills [fake] p: m\\e[2K":\n  eval-status: x\n',
    );
    const result = await evalClean(root);
    assert.deepEqual(result.failures, [
      cleanFinding("a\\u000aFAIL skills [fake] p: m\\u001b[2K.eval-status"),
    ]);
  });
});

const REFUSED_ROWS: ReadonlyArray<[string, string]> = [
  ["an integer key", "m:\n  1:\n    eval-status: x\n"],
  ["a bool key", "m:\n  true:\n    eval-status: x\n"],
  ["a null key", "m:\n  ~:\n    eval-status: x\n"],
  ["a hex key", "m:\n  0x10:\n    eval-status: x\n"],
  ["a float key", "m:\n  1.5:\n    eval-status: x\n"],
  ["a top-level integer key", "1:\n  eval-status: x\n"],
  ["a top-level bool key", "true:\n  eval-digest: x\n"],
  ["a top-level null key", "~:\n  eval-status: x\n"],
  ["an anchored map reused", "a: &x\n  eval-digest: 1\nb: *x\n"],
  ["an anchored merge value", "m:\n  <<: &x\n    eval-digest: 1\n"],
];

for (const [label, extra] of REFUSED_ROWS)
  test(`eval-frontmatter-clean skips a block that frontmatter-yaml reports: ${label}`, async () => {
    assert.deepEqual(await cleanPaths(extra), []);
    await withSkillsRepo(async (root) => {
      withFrontmatter(root, extra);
      const result = await frontmatterOnly(root);
      assert.deepEqual(
        result.failures.map((failure) => [failure.rule, failure.path]),
        [["frontmatter-yaml", DOCS]],
      );
    });
  });

test("eval-frontmatter-clean reports duplicates, once per occurrence", async () => {
  assert.deepEqual(
    await cleanPaths("a.b:\n  eval-status: 1\na:\n  b:\n    eval-status: 2\n"),
    ["a.b.eval-status", "a.b.eval-status"],
  );
});

test("eval-frontmatter-clean sorts the paths in string order", async () => {
  const items = Array.from(
    { length: 11 },
    (_, i) => `  - eval-status: ${i}\n`,
  ).join("");
  assert.deepEqual(
    await cleanPaths(`x:\n${items}`),
    // "]" sorts after "0", so "x[10]" comes before "x[1]" in byte order.
    [0, 10, 1, 2, 3, 4, 5, 6, 7, 8, 9].map((i) => `x[${i}].eval-status`),
  );
  assert.deepEqual(
    await cleanPaths(
      "metadata:\n  eval-digest: 1\nlast-evaluated: 1\nhistory:\n  - eval-status: 1\n",
    ),
    ["history[0].eval-status", "last-evaluated", "metadata.eval-digest"],
  );
});

test("eval-frontmatter-clean joins a run with the first three families", async () => {
  await withSkillsRepo(async (root) => {
    mutateFile(
      root,
      "skills/routing/SKILL.md",
      "metadata:\n",
      "metadata:\n    model-ladders-reviewed: 2026-08-09\n",
    );
    const result = await run(root, options, {
      families: [
        FRONTMATTER_FAMILY,
        EVAL_METADATA_FAMILY,
        EVAL_FRONTMATTER_CLEAN_FAMILY,
      ],
    });
    assert.deepEqual(result, {
      failures: [
        cleanFinding(
          "metadata.model-ladders-reviewed",
          "skills/routing/SKILL.md",
        ),
      ],
    });
  });
});

// --- the real tree ------------------------------------------------------------

test("the three families pass on the repository", async () => {
  const root = repoRoot(path.dirname(fileURLToPath(import.meta.url)));
  const result = await run(root, options, {
    families: [
      FRONTMATTER_FAMILY,
      EVAL_METADATA_FAMILY,
      EVAL_FRONTMATTER_CLEAN_FAMILY,
    ],
  });
  assert.deepEqual(result, { failures: [] });
});

// --- audit error texts and causes, rows added by the mutation pass -----------------------------------------------------

/** Runs the inline family with the inspection of `target` failing with `error`; every other path is inspected as usual. */
function inlineFailingLstat(root: string, target: string, error: unknown) {
  const real = seams.lstat;
  return withSeams(
    {
      lstat: (candidate) => {
        if (candidate === target) throw error;
        return real(candidate);
      },
    },
    () => inline(root),
  );
}

test("a skills directory whose inspection fails is an audit error naming it and why", async () => {
  await withSkillsRepo(async (root) => {
    const target = path.join(root, "skills");
    const rows: [string, unknown, string][] = [
      [
        "a coded failure",
        Object.assign(new Error("EACCES: permission denied, lstat"), {
          code: "EACCES",
          syscall: "lstat",
          path: target,
        }),
        "skills: EACCES",
      ],
      [
        "a failure with no code",
        new Error("odd failure"),
        "skills: unreadable",
      ],
      [
        "a file error for another path",
        new RepoFileError("symlink", "somewhere"),
        "somewhere: passes through a link",
      ],
      ["no error object", null, "skills: unreadable"],
    ];
    for (const [label, error, text] of rows) {
      const result = await inlineFailingLstat(root, target, error);
      assert.deepEqual(result.failures, [], label);
      assert.equal(result.error, `skills: reading skills: ${text}`, label);
    }
  });
});

test("an audit error from a read keeps the failure that caused it as its cause", async () => {
  await withSkillsRepo(async (root) => {
    const context = { families: [], evalsCheck: () => [] };
    const readFailure = new Error("open somewhere: permission denied");
    const readDirFailure = Object.assign(new Error("denied"), {
      code: "EACCES",
    });
    const skillsDir = path.join(root, "skills");
    const realReadDir = seams.readDir;
    // A failed read reaches the audit error as the cause of the file error; a failed directory read is its cause.
    const causeOf = (error: unknown): unknown =>
      error instanceof Error && error.cause instanceof RepoFileError
        ? error.cause.cause
        : (error as Error).cause;
    const expectCause = (body: () => unknown, cause: unknown, label: string) =>
      assert.throws(
        body,
        (error: unknown) => error instanceof Error && causeOf(error) === cause,
        label,
      );
    withSeams(
      {
        readFile: () => {
          throw readFailure;
        },
      },
      () => {
        expectCause(
          () => INLINE_FAMILY.run(root, context),
          readFailure,
          "a SKILL.md that cannot be read in the inline family",
        );
        expectCause(
          () => FRONTMATTER_FAMILY.run(root, context),
          readFailure,
          "a SKILL.md that cannot be read in the frontmatter family",
        );
      },
    );
    let calls = 0;
    withSeams(
      {
        readDir: (dir) => {
          if (dir === skillsDir) {
            calls += 1;
            if (calls === 2) throw readDirFailure;
          }
          return realReadDir(dir);
        },
      },
      () =>
        expectCause(
          () => INLINE_FAMILY.run(root, context),
          readDirFailure,
          "a skills directory that cannot be read",
        ),
    );
  });
});

// A work folder's temp directory is scratch: only what git tracks there is checked.

const TEMP_COPY = "docs/specs/2026-11/feature-x/temp/copy/skills";
const OUTSIDE_COPY = "docs/specs/2026-11/feature-x/copy/skills";

/** A duplicate `gate` skill and an empty skill directory under `parent`. */
function copySkills(root: string, parent: string): void {
  writeFile(root, `${parent}/gate/SKILL.md`, skillMD("gate"));
  mkdir(root, `${parent}/empty`);
}

const rulesOf = (result: { failures: Failure[]; error?: string }) =>
  result.failures.map((failure) => `${failure.rule}|${failure.path}`);

test("an untracked copy of the skills under a work folder's temp is skipped without git", async () => {
  await withSkillsRepo(async (root) => {
    copySkills(root, TEMP_COPY);
    assert.deepEqual(await inline(root), { failures: [] });
  });
});

test("the same copy outside a work folder's temp is checked", async () => {
  await withSkillsRepo(async (root) => {
    copySkills(root, OUTSIDE_COPY);
    assert.deepEqual(rulesOf(await inline(root)), [
      `skill-md-present|${OUTSIDE_COPY}/empty`,
      "skill-name-unique|skills/gate/SKILL.md",
    ]);
  });
});

test("in a checkout a tracked SKILL.md under a work folder's temp is checked and an untracked sibling directory is not", async () => {
  await withSkillsRepo(async (root) => {
    copySkills(root, TEMP_COPY);
    initCheckout(root, [`${TEMP_COPY}/gate/SKILL.md`]);
    assert.deepEqual(rulesOf(await inline(root)), [
      "skill-name-unique|skills/gate/SKILL.md",
    ]);
  });
});

test("in a checkout a tracked skill directory under a work folder's temp that lacks a SKILL.md is reported", async () => {
  await withSkillsRepo(async (root) => {
    const readme = `${TEMP_COPY}/nodoc/README.md`;
    writeFile(root, readme, "No skill here.\n");
    initCheckout(root, [readme]);
    assert.deepEqual(await inline(root), {
      failures: [
        finding("skill-md-present", `${TEMP_COPY}/nodoc`, "missing SKILL.md"),
      ],
    });
  });
});

test("in a checkout an untracked SKILL.md in a tracked skill directory under a work folder's temp does not count as present", async () => {
  await withSkillsRepo(async (root) => {
    const readme = `${TEMP_COPY}/nodoc/README.md`;
    writeFile(root, readme, "No skill here.\n");
    writeFile(root, `${TEMP_COPY}/nodoc/SKILL.md`, skillMD("scratch"));
    initCheckout(root, [readme]);
    assert.deepEqual(await inline(root), {
      failures: [
        finding("skill-md-present", `${TEMP_COPY}/nodoc`, "missing SKILL.md"),
      ],
    });
  });
});

test("in a checkout an untracked linked SKILL.md in a tracked skill directory under a work folder's temp is not an audit error", async (t) => {
  await withSkillsRepo(async (root) => {
    await withOutside(async (outside) => {
      const target = path.join(outside, "SKILL.md");
      writeFileSync(target, skillFile("name: outside"));
      const readme = `${TEMP_COPY}/nodoc/README.md`;
      writeFile(root, readme, "No skill here.\n");
      if (!linkOrSkip(t, root, `${TEMP_COPY}/nodoc/SKILL.md`, target, "file"))
        return;
      initCheckout(root, [readme]);
      assert.deepEqual(await inline(root), {
        failures: [
          finding("skill-md-present", `${TEMP_COPY}/nodoc`, "missing SKILL.md"),
        ],
      });
    });
  });
});

test("in a checkout a tracked SKILL.md in a skill directory under a work folder's temp counts as present", async () => {
  await withSkillsRepo(async (root) => {
    const skill = `${TEMP_COPY}/scratch/SKILL.md`;
    writeFile(root, skill, skillMD("scratch"));
    initCheckout(root, [skill]);
    assert.deepEqual(await inline(root), { failures: [] });
  });
});

test("a .git that is not a repository, with no work folder temp, never runs git", async () => {
  await withSkillsRepo(async (root) => {
    mkdir(root, ".git");
    assert.deepEqual(await inline(root), { failures: [] });
  });
});

test(
  "a .git that is not a repository, with a work folder temp, is an audit error",
  {
    skip: tempIsInsideRepository()
      ? "the temp directory is inside a git repository"
      : false,
  },
  async () => {
    await withSkillsRepo(async (root) => {
      copySkills(root, TEMP_COPY);
      mkdir(root, ".git");
      const result = await inline(root);
      assert.deepEqual(result.failures, []);
      assert.match(result.error ?? "", /^skills: listing tracked files: /);
    });
  },
);

test("the inline family called with a context that carries no lookup still skips an untracked temp copy", async () => {
  await withSkillsRepo(async (root) => {
    copySkills(root, TEMP_COPY);
    assert.deepEqual(
      await INLINE_FAMILY.run(root, { families: [], evalsCheck: () => [] }),
      [],
    );
  });
});

// --- the frontmatter scenarios through the whole domain ------------------------

const METADATA_SLICE = sliceNamed("metadata");
const CONTENT_SLICE = sliceNamed("content");
const WORKFLOW_SLICE = sliceNamed("workflow");

const PROTOTYPE = "skills/prototype/SKILL.md";
const MISSING_SKILL_MD = "missing SKILL.md";
/** The finding the standards family gives for the skill file `at` whose name, printed as `printed`, has `problem`. */
const nameFormat = (at: string, printed: string, problem: string): Failure =>
  finding(
    "skill-name-format",
    at,
    `name ${printed} ${problem}; expected lowercase letters and digits in hyphen-separated groups, at most 64 characters, equal to the directory name ${JSON.stringify(path.posix.basename(path.posix.dirname(at)))}`,
  );

// Every family, on the green fixture.

test("a flow skill whose name line holds a carriage return", async () => {
  // A lone carriage return does not end a line, so the name is one scalar and the block parses; only the standards
  // family objects to the name, which it prints with the carriage return escaped.
  assert.deepEqual(
    await auditEveryFamily((r) =>
      mutateFile(r, GATE_SKILL, "name: gate\n", "name: ga\rte\n"),
    ),
    {
      failures: [
        nameFormat(
          GATE_SKILL,
          '"ga\\u000dte"',
          'is not lowercase kebab-case, differs from the directory name "gate"',
        ),
      ],
    },
  );
});

test("eval-frontmatter-clean", async () => {
  assert.deepEqual(await auditEveryFamily(breaker("eval-frontmatter-clean")), {
    failures: [cleanFinding("metadata.eval-status", PROTOTYPE)],
  });
});

test("eval-frontmatter-present", async () => {
  // The rewritten skill also loses its required fields and its links to the shared contracts.
  assert.deepEqual(
    await auditEveryFamily(breaker("eval-frontmatter-present")),
    {
      failures: [
        missingFrontmatter(PROTOTYPE),
        finding(
          "eval-frontmatter-present",
          PROTOTYPE,
          "empty or missing frontmatter",
        ),
        finding(
          "question-format",
          PROTOTYPE,
          "must link to skills/routing/references/contracts/question-format.md",
        ),
        staleBaseline(
          "prototype",
          "skill-frontmatter-key",
          "skills/prototype/SKILL.md",
          "version",
        ),
        finding(
          "shared-contracts",
          PROTOTYPE,
          "must link to skills/routing/references/contracts/empirical-questions.md",
        ),
      ],
    },
  );
});

test("eval-history-footer", async () => {
  assert.deepEqual(await auditEveryFamily(breaker("eval-history-footer")), {
    failures: [finding("eval-history-footer", PROTOTYPE, FOOTER_MESSAGE)],
  });
});

test("frontmatter-yaml", async () => {
  // The block no longer parses, so the standards family no longer reads the version it baselined.
  assert.deepEqual(await auditEveryFamily(breaker("frontmatter-yaml")), {
    failures: [
      yamlFinding(PROTOTYPE, "Map keys must be unique (block line 2)"),
      staleBaseline(
        "prototype",
        "skill-frontmatter-key",
        "skills/prototype/SKILL.md",
        "version",
      ),
    ],
  });
});

test("skill-frontmatter", async () => {
  assert.deepEqual(await auditEveryFamily(breaker("skill-frontmatter")), {
    failures: [
      missingFrontmatter(DOCS),
      staleBaseline(
        "docs",
        "skill-frontmatter-key",
        "skills/docs/SKILL.md",
        "version",
      ),
    ],
  });
});

test("skill-md-present", async () => {
  assert.deepEqual(await auditEveryFamily(breaker("skill-md-present")), {
    failures: [EMPTY_SKILL_FINDING],
  });
});

test("skill-name-unique", async () => {
  const forge = "skills/forge/SKILL.md";
  assert.deepEqual(await auditEveryFamily(breaker("skill-name-unique")), {
    failures: [
      duplicate("docs", DOCS, forge),
      nameFormat(forge, '"docs"', 'differs from the directory name "forge"'),
    ],
  });
});

test("the challenge skill file is missing", async () => {
  // question-format takes challenge from the skill walk, so an absent file adds nothing there.
  assert.deepEqual(await auditEveryFamily((r) => remove(r, CHALLENGE)), {
    failures: [
      finding("skill-md-present", "skills/challenge", MISSING_SKILL_MD),
      staleBaseline(
        "challenge",
        "skill-frontmatter-key",
        "skills/challenge/SKILL.md",
        "version",
      ),
      staleBaseline(
        "challenge",
        "skill-references-index",
        "skills/challenge/SKILL.md",
        "references",
      ),
      staleBaseline(
        "challenge",
        "skill-routing-section",
        "skills/challenge/SKILL.md",
        "routing",
      ),
    ],
  });
  // The workflow slice families leave the presence check to the metadata slice. The pasted-format fixture's skills
  // paste the decision-question format, which question-format reports on every skill but the missing challenge.
  assert.deepEqual(
    await auditSlice(WORKFLOW_SLICE, (r) => remove(r, CHALLENGE)),
    {
      failures: [
        "docs",
        "execute",
        "forge",
        "gate",
        "gh",
        "prototype",
        "review",
        "triage",
      ].map((name) =>
        finding(
          "question-format",
          `skills/${name}/SKILL.md`,
          "replace the local decision-question format with a link to skills/routing/references/contracts/question-format.md",
        ),
      ),
    },
  );
});

// The metadata slice families, on the pasted-format fixture.

/** The skill-frontmatter finding on `skills/x/SKILL.md`, then its frontmatter-yaml finding with `reason`. */
const xUnreadBlock = (reason: string): AuditResult => ({
  failures: [missingFrontmatter(X_SKILL), yamlFinding(X_SKILL, reason)],
});

/** The frontmatter-yaml finding on `skills/x/SKILL.md` with the reader's `reason`. */
const xNotYaml = (reason: string): AuditResult => ({
  failures: [yamlFinding(X_SKILL, reason)],
});

/** The eval-frontmatter-clean finding on `skills/x/SKILL.md` for the key path `key`. */
const xEvalKey = (key: string): AuditResult => ({
  failures: [cleanFinding(key, X_SKILL)],
});

const X_FOOTER: AuditResult = {
  failures: [finding("eval-history-footer", X_SKILL, FOOTER_MESSAGE)],
};

const NO_MAPPING = "frontmatter holds no mapping";
const NON_STRING_KEY = "mapping keys must be strings (block line 5)";
const ANCHOR = "anchors are not allowed (block line 5)";

test("a block of one space", async () => {
  assert.deepEqual(
    await auditSlice(METADATA_SLICE, xFields(" \n")),
    xUnreadBlock(NO_MAPPING),
  );
});

test("a BOM before the opener", async () => {
  assert.deepEqual(
    await auditSlice(METADATA_SLICE, xText(BOM + skillMD("x"))),
    {
      failures: [
        missingFrontmatter(X_SKILL),
        finding(
          "eval-frontmatter-present",
          X_SKILL,
          "empty or missing frontmatter",
        ),
      ],
    },
  );
});

test("a comment-only block", async () => {
  assert.deepEqual(
    await auditSlice(METADATA_SLICE, xFields("# only a comment\n")),
    xUnreadBlock(NO_MAPPING),
  );
});

test("a duplicate key at the top level", async () => {
  assert.deepEqual(
    await auditSlice(METADATA_SLICE, xFields(`${REQUIRED}version: "2"\n`)),
    xNotYaml("Map keys must be unique (block line 4)"),
  );
});

test("a duplicate name with a leading NBSP", async () => {
  // A leading no-break space is trimmed from the name, so it collides with forge.
  assert.deepEqual(
    await auditSlice(
      METADATA_SLICE,
      xFields(`name: ${NBSP}forge\ndescription: d\nversion: "1"\n`),
    ),
    { failures: [duplicate("forge", "skills/forge/SKILL.md", X_SKILL)] },
  );
});

test("a duplicate nested key", async () => {
  assert.deepEqual(
    await auditSlice(
      METADATA_SLICE,
      xFields(`${REQUIRED}metadata:\n  a: 1\n  a: 2\n`),
    ),
    xNotYaml("Map keys must be unique (block line 6)"),
  );
});

test("a heading that continues the footer text", async () => {
  assert.deepEqual(
    await auditSlice(METADATA_SLICE, xBody("## Validation historyX\n")),
    X_FOOTER,
  );
});

test("a lone CR before the name field", async () => {
  // A lone carriage return does not end a line, so name: sits inside the description's value.
  assert.deepEqual(
    await auditSlice(
      METADATA_SLICE,
      xText('---\ndescription: d\rname: x\nversion: "1"\n---\n'),
    ),
    xUnreadBlock(
      "Nested mappings are not allowed in compact mappings (block line 1)",
    ),
  );
});

test("a name on the next line colliding with a quoted name", async () => {
  // The bare name line captures the quoted line after it, which the YAML reader refuses.
  assert.deepEqual(
    await auditSlice(
      METADATA_SLICE,
      all(
        xText(`---\nname:\n"forge"\ndescription: d\nversion: "1"\n---\n`),
        put(
          "skills/y/SKILL.md",
          `---\nname: "forge"\ndescription: d\nversion: "1"\n---\n`,
        ),
      ),
    ),
    {
      failures: [
        duplicate("forge", "skills/forge/SKILL.md", X_SKILL),
        duplicate("forge", "skills/forge/SKILL.md", "skills/y/SKILL.md"),
        yamlFinding(
          X_SKILL,
          "Implicit map keys need to be followed by map values (block line 2)",
        ),
      ],
    },
  );
});

test("a nested evals tree holding a garbage SKILL.md", async () => {
  // Only the top-level evals directory is skipped.
  assert.deepEqual(
    await auditSlice(
      METADATA_SLICE,
      put("deep/evals/a/skills/x/SKILL.md", GARBAGE),
    ),
    { failures: [missingFrontmatter("deep/evals/a/skills/x/SKILL.md")] },
  );
});

test("a surfaces mirror with a repeated name", async () => {
  assert.deepEqual(
    await auditSlice(
      METADATA_SLICE,
      put("skills/surfaces/docs/SKILL.md", skillMD("docs")),
    ),
    {
      failures: [
        finding("skill-md-present", "skills/surfaces", MISSING_SKILL_MD),
      ],
    },
  );
});

test("a top-level scalar block", async () => {
  assert.deepEqual(
    await auditSlice(METADATA_SLICE, xFields("just a scalar\n")),
    xUnreadBlock(BAD_SCALAR),
  );
});

test("a top-level sequence block", async () => {
  assert.deepEqual(
    await auditSlice(METADATA_SLICE, xFields("- a\n- b\n")),
    xUnreadBlock("the top level is a sequence, not a mapping"),
  );
});

test("an empty directory under a nested skills directory", async () => {
  assert.deepEqual(
    await auditSlice(METADATA_SLICE, (r) => mkdir(r, "deep/skills/empty")),
    {
      failures: [
        finding("skill-md-present", "deep/skills/empty", MISSING_SKILL_MD),
      ],
    },
  );
});

test("an eval key in a list nest", async () => {
  assert.deepEqual(
    await auditSlice(
      METADATA_SLICE,
      xFields(`${REQUIRED}m:\n  - eval-status: passing\n`),
    ),
    xEvalKey("m[0].eval-status"),
  );
});

test("an eval key in the eleventh list item", async () => {
  const items = Array.from({ length: 10 }, (_, i) => `  - k${i}: 1\n`).join("");
  assert.deepEqual(
    await auditSlice(
      METADATA_SLICE,
      xFields(`${REQUIRED}m:\n${items}  - eval-status: passing\n`),
    ),
    xEvalKey("m[10].eval-status"),
  );
});

test("an eval key reached through a merge", async () => {
  // The reader refuses anchors, so the key behind the merge is never reached.
  assert.deepEqual(
    await auditSlice(
      METADATA_SLICE,
      xFields(`${REQUIRED}base: &a\n  eval-status: passing\nm:\n  <<: *a\n`),
    ),
    xNotYaml(ANCHOR),
  );
});

test("an eval key reached through an alias", async () => {
  assert.deepEqual(
    await auditSlice(
      METADATA_SLICE,
      xFields(`${REQUIRED}m: &a\n  eval-status: passing\nn: *a\n`),
    ),
    xNotYaml(ANCHOR),
  );
});

test("an eval key under a nested date key", async () => {
  // A date key is a plain string under the core schema.
  assert.deepEqual(
    await auditSlice(
      METADATA_SLICE,
      xFields(`${REQUIRED}m:\n  2026-01-02:\n    eval-status: passing\n`),
    ),
    xEvalKey("m.2026-01-02.eval-status"),
  );
});

test("an eval key under a nested empty key", async () => {
  assert.deepEqual(
    await auditSlice(
      METADATA_SLICE,
      xFields(`${REQUIRED}m:\n  "":\n    eval-status: passing\n`),
    ),
    xEvalKey("m..eval-status"),
  );
});

test("an eval key under a nested hexadecimal key", async () => {
  assert.deepEqual(
    await auditSlice(
      METADATA_SLICE,
      xFields(`${REQUIRED}m:\n  0x10:\n    eval-status: passing\n`),
    ),
    xNotYaml(NON_STRING_KEY),
  );
});

test("an eval key under a nested integer key", async () => {
  assert.deepEqual(
    await auditSlice(
      METADATA_SLICE,
      xFields(`${REQUIRED}m:\n  1:\n    eval-status: passing\n`),
    ),
    xNotYaml(NON_STRING_KEY),
  );
});

test("an eval key under a nested null key", async () => {
  assert.deepEqual(
    await auditSlice(
      METADATA_SLICE,
      xFields(`${REQUIRED}m:\n  ~:\n    eval-status: passing\n`),
    ),
    xNotYaml(NON_STRING_KEY),
  );
});

test("an eval key under a nested true key", async () => {
  assert.deepEqual(
    await auditSlice(
      METADATA_SLICE,
      xFields(`${REQUIRED}m:\n  true:\n    eval-status: passing\n`),
    ),
    xNotYaml(NON_STRING_KEY),
  );
});

test("an unclosed block holding all fields", async () => {
  assert.deepEqual(
    await auditSlice(
      METADATA_SLICE,
      xText('---\nname: x\ndescription: d\nversion: "1"\n'),
    ),
    {
      failures: [
        missingFrontmatter(X_SKILL),
        finding(
          "frontmatter-yaml",
          X_SKILL,
          "frontmatter block is never closed by a bare ---",
        ),
      ],
    },
  );
});

test("an unclosed flow sequence in the description", async () => {
  assert.deepEqual(
    await auditSlice(
      METADATA_SLICE,
      xFields('name: x\ndescription: [unclosed\nversion: "1"\n'),
    ),
    xNotYaml(
      "Flow sequence in block collection must be sufficiently indented and end with a ] (block line 3)",
    ),
  );
});

test("the Apache-attributed skills/docs/SKILL.md is removed", async () => {
  assert.deepEqual(await auditSlice(METADATA_SLICE, (r) => remove(r, DOCS)), {
    failures: [
      finding("skill-md-present", "skills/docs", MISSING_SKILL_MD),
      finding(
        "reference-attribution",
        DOCS,
        "attributed third-party file missing",
      ),
    ],
  });
});

test("the footer heading in a CRLF file", async () => {
  assert.deepEqual(
    await auditSlice(
      METADATA_SLICE,
      xText(crlf(skillMD("x") + "\n## Validation history\n")),
    ),
    X_FOOTER,
  );
});

test("the footer heading inside a fence", async () => {
  // A fence does not hide the heading.
  assert.deepEqual(
    await auditSlice(
      METADATA_SLICE,
      xBody("```\n## Validation history\n```\n"),
    ),
    X_FOOTER,
  );
});

test("the keys 1 and quoted 1", async () => {
  // The unquoted key is an integer, which the reader refuses before it compares the two keys.
  assert.deepEqual(
    await auditSlice(METADATA_SLICE, xFields(`${REQUIRED}1: a\n"1": b\n`)),
    xNotYaml("mapping keys must be strings (block line 4)"),
  );
});

test("the required version field on document line 21 with CRLF", async () => {
  assert.deepEqual(
    await auditSlice(METADATA_SLICE, xText(versionAtLine(21, "\r\n"))),
    { failures: [missingFrontmatter(X_SKILL)] },
  );
});

test("the required version field on document line 21 with LF", async () => {
  assert.deepEqual(
    await auditSlice(METADATA_SLICE, xText(versionAtLine(21, "\n"))),
    { failures: [missingFrontmatter(X_SKILL)] },
  );
});

test("two skills with an empty name", async () => {
  // Each bare name line captures the line after it, so both skills are named "description: d".
  const empty = '---\nname:\ndescription: d\nversion: "1"\n---\n';
  assert.deepEqual(
    await auditSlice(
      METADATA_SLICE,
      all(xText(empty), put("skills/y/SKILL.md", empty)),
    ),
    { failures: [duplicate("description: d", X_SKILL, "skills/y/SKILL.md")] },
  );
});

// The content slice families, on the pasted-format fixture: none of them reports the frontmatter rules these changes
// break.

test("a BOM before the frontmatter of an owner", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      put(
        "skills/intent/SKILL.md",
        `${BOM}---\nname: intent\ndescription: d\nversion: "1"\nmetadata:\n  artifact: intent.md\n---\n`,
      ),
    ),
    { failures: [] },
  );
});

test("a commit reference without the git skill", async () => {
  assert.deepEqual(
    await auditSlice(CONTENT_SLICE, put(GIT_COMMIT, "Commit with care.\n")),
    { failures: [] },
  );
});

test("a duplicate artifact key on a non-owner", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      skillWithMetadata("x", "    artifact: a.md\n    artifact: b.md\n"),
    ),
    { failures: [] },
  );
});

test("a duplicate artifact key on an owner", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      skillWithMetadata(
        "intent",
        "    artifact: intent.md\n    artifact: other.md\n",
      ),
    ),
    { failures: [] },
  );
});
