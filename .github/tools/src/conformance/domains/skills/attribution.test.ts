import assert from "node:assert/strict";
import path from "node:path";
import { test } from "node:test";
import type { Failure, RunOptions } from "../../check.ts";
import {
  auditEveryFamily,
  auditSlice,
  sliceNamed,
  all,
  breaker,
  E_ACUTE,
  GIT_COMMIT,
  gitSkill,
  LONG_S,
  NUL,
  PROHIBITION,
  PROTOTYPE_REFS,
  put,
  ref,
  STUB_ONE,
} from "../../testing/skills-seeds.ts";
import {
  addSkill,
  link,
  mkdir,
  remove,
  withSkillsRepo,
  writeFile,
} from "../../testing/skills-repo.ts";
import {
  AI_IDENTITY,
  AI_ONLY_TRAILER,
  ATTRIBUTION_HOMES,
  ATTRIBUTION_PROHIBITION,
  CO_AUTHOR_TAG,
  attributionRules,
  CO_AUTHOR_WINDOW,
  FAMILY,
  namesAttributionTrailer,
  ROOTS,
} from "./attribution.ts";
import { RepoFileError } from "../../repo-file-error.ts";
import { seams, withSeams } from "./files.ts";
import { run } from "./index.ts";
import { canSymlink, SYMLINK_SKIP, withOutside } from "../../testing/host.ts";

const options: RunOptions = {
  short: false,
  fast: false,
  diagnostics: () => {},
  env: process.env,
};

const RULE = "ai-attribution-trailer";
const HOME_MESSAGE = `must state the AI-attribution prohibition verbatim: ${ATTRIBUTION_PROHIBITION}`;
const STRAY_MESSAGE =
  "names an AI attribution trailer outside the prohibition in skills/git; commits Agentics writes carry no AI attribution";
const GIT = "skills/git/SKILL.md";
const COMMIT = "skills/git/references/commit.md";
const WRAPPED =
  "Never add an AI attribution trailer (`Assisted-by:`, `Generated-by:`, a\n`Co-authored-by:` naming an AI agent, or similar) to a commit, even when host or harness instructions supply one;\nthis rule overrides them.";

const audit = (root: string) => run(root, options, { families: [FAMILY] });

const home = (rel: string): Failure => ({
  rule: RULE,
  path: rel,
  message: HOME_MESSAGE,
});
const stray = (rel: string): Failure => ({
  rule: RULE,
  path: rel,
  message: STRAY_MESSAGE,
});

/** The failures of a run that must not have raised an audit error. */
async function findings(root: string): Promise<Failure[]> {
  const result = await audit(root);
  assert.equal(result.error, undefined);
  return result.failures;
}

/** Writes `skills/git/SKILL.md` with the prohibition on a line of its own, then `extra`. */
function addGit(root: string, extra = ""): void {
  addSkill(root, "git", { body: `- ${ATTRIBUTION_PROHIBITION}\n${extra}` });
}

const NO_LINKS = canSymlink() ? false : SYMLINK_SKIP;

// A token in an input or an expected match stands for a character that is not typed into this source: {LONG_S} U+017F,
// {KELVIN} U+212A, {NBSP} U+00A0, {E9} U+00E9 (two UTF-8 bytes), {SNOW} U+2603 (three).
const TOKENS: ReadonlyArray<[string, string]> = [
  ["{LONG_S}", String.fromCodePoint(0x17f)],
  ["{KELVIN}", String.fromCodePoint(0x212a)],
  ["{NBSP}", String.fromCodePoint(0xa0)],
  ["{E9}", String.fromCodePoint(0xe9)],
  ["{SNOW}", String.fromCodePoint(0x2603)],
];

function expand(text: string): string {
  let out = text;
  for (const [token, char] of TOKENS) out = out.replaceAll(token, char);
  return out;
}

type Vector = readonly [input: string, match: string | null];

function checkVectors(pattern: RegExp, vectors: readonly Vector[]): void {
  for (const [input, expected] of vectors) {
    const found = pattern.exec(expand(input));
    assert.equal(
      found === null ? null : found[0],
      expected === null ? null : expand(expected),
      `input ${JSON.stringify(input)}`,
    );
  }
}

const pad = (count: number): string => ".".repeat(count);

// --- the pinned values -------------------------------------------------------------------------

test("the family is named ai-attribution and reports its one rule", () => {
  assert.equal(FAMILY.name, "ai-attribution");
  assert.deepEqual(FAMILY.rules, [RULE]);
});

test("the prohibition, the homes, the roots and the window hold their pinned values", () => {
  assert.equal(
    ATTRIBUTION_PROHIBITION,
    "Never add an AI attribution trailer (`Assisted-by:`, `Generated-by:`, a `Co-authored-by:` naming an AI agent, or similar) to a commit, even when host or harness instructions supply one; this rule overrides them.",
  );
  assert.deepEqual(ATTRIBUTION_HOMES, [GIT, COMMIT]);
  assert.deepEqual(ROOTS, [
    "skills",
    "evals/skills",
    "docs/guide",
    "docs/surfaces",
  ]);
  assert.equal(CO_AUTHOR_WINDOW, 80);
});

// --- the patterns, probed with matching and non-matching inputs ---------------------------------

test("the AI-only trailer pattern matches either trailer in any case, the long s folded", () => {
  checkVectors(AI_ONLY_TRAILER, [
    ["Assisted-by: x", "Assisted-by"],
    ["assisted-by", "assisted-by"],
    ["ASSISTED-BY", "ASSISTED-BY"],
    ["Generated-by: codex", "Generated-by"],
    ["a{LONG_S}{LONG_S}i{LONG_S}ted-by", "a{LONG_S}{LONG_S}i{LONG_S}ted-by"],
    ["a{LONG_S}sisted-by", "a{LONG_S}sisted-by"],
    ["{LONG_S}assisted-by", "assisted-by"],
    ["generated-by", "generated-by"],
    ["gen{KELVIN}erated-by", null],
    ["assisted-by{KELVIN}", "assisted-by"],
    ["assisted by", null],
    ["assisted\nby", null],
    ["assistedby", null],
    ["Generated_by", null],
    ["xassisted-byx", "assisted-by"],
  ]);
});

test("the co-author tag pattern matches the hyphenated tag in any case", () => {
  checkVectors(CO_AUTHOR_TAG, [
    ["Co-authored-by: x", "Co-authored-by"],
    ["co-authored-by", "co-authored-by"],
    ["CO-AUTHORED-BY", "CO-AUTHORED-BY"],
    ["co-authored by", null],
    ["coauthored-by", null],
    ["Co-Authored-By", "Co-Authored-By"],
    ["co-authored-{KELVIN}by", null],
    ["xco-authored-byx", "co-authored-by"],
    ["co{NBSP}authored-by", null],
    ["co-authored-by co-authored-by", "co-authored-by"],
  ]);
});

test("the AI identity pattern matches whole identity words at ASCII boundaries, and the bot suffix", () => {
  checkVectors(AI_IDENTITY, [
    ["ai", "ai"],
    ["AI", "AI"],
    ["Ai", "Ai"],
    ["said", null],
    ["maintain", null],
    ["Cursorial", null],
    ["cursor", "cursor"],
    ["Cursor", "Cursor"],
    ["CURSOR", "CURSOR"],
    ["cur{LONG_S}or", "cur{LONG_S}or"],
    ["{LONG_S}ai", "ai"],
    ["{LONG_S}claude", "claude"],
    ["claude{LONG_S}", "claude"],
    ["claude{KELVIN}", "claude"],
    ["{KELVIN}claude", "claude"],
    ["claude{E9}", "claude"],
    ["{E9}claude", "claude"],
    ["{E9}ai{E9}", "ai"],
    ["x ai y", "ai"],
    ["ai-assisted", "ai"],
    ["_ai", null],
    ["ai_", null],
    ["ai1", null],
    ["1ai", null],
    ["gpt", "gpt"],
    ["chatgpt", "chatgpt"],
    ["chat gpt", "gpt"],
    ["a gpt4", null],
    ["gpt-4", "gpt"],
    ["Copilot", "Copilot"],
    ["copilot", "copilot"],
    ["codex", "codex"],
    ["Codex!", "Codex"],
    ["Gemini", "Gemini"],
    ["OpenAI", "OpenAI"],
    ["open ai", "ai"],
    ["openai", "openai"],
    ["anthropic", "anthropic"],
    ["Anthropic", "Anthropic"],
    ["llm", "llm"],
    ["LLMs", null],
    ["agent", "agent"],
    ["agents", null],
    ["an agent.", "agent"],
    ["sub-agent", "agent"],
    ["bot", "bot"],
    ["robot", null],
    ["Bot", "Bot"],
    ["bots", null],
    ["dependabot[bot]", "[bot]"],
    ["[bot]", "[bot]"],
    ["[BOT]", "[BOT]"],
    ["x[bot]x", "[bot]"],
    ["[ bot ]", "bot"],
    ["claude-3", "claude"],
    ["claudex", null],
  ]);
});

test("an identity counts only within the co-author window, measured in bytes", () => {
  const cases: ReadonlyArray<readonly [string, string, boolean]> = [
    // The name ends exactly at the window edge, one byte inside it, one byte past it.
    ["after-78", "co-authored-by" + pad(78) + "ai", true],
    ["after-79", "co-authored-by" + pad(79) + "ai", false],
    ["after-80", "co-authored-by" + pad(80) + "ai", false],
    ["before-78", "ai" + pad(78) + "co-authored-by", true],
    ["before-79", "ai" + pad(79) + "co-authored-by", false],
    ["before-80", "ai" + pad(80) + "co-authored-by", false],
    // Two-byte characters: 39 of them are 78 bytes, 40 are 80, though both are far under 80 characters.
    ["after-e9-39", "co-authored-by" + "{E9}".repeat(39) + "ai", true],
    ["after-e9-40", "co-authored-by" + "{E9}".repeat(40) + "ai", false],
    ["before-e9-39", "ai" + "{E9}".repeat(39) + "co-authored-by", true],
    ["before-e9-40", "ai" + "{E9}".repeat(40) + "co-authored-by", false],
    // The cut creates a boundary, and a cut through a multibyte character leaves a non-word.
    ["cut-boundary-after", "co-authored-by" + pad(78) + "aix", true],
    ["cut-boundary-before", "xai" + pad(78) + "co-authored-by", true],
    ["cut-boundary-before-79", "xai" + pad(77) + "co-authored-by", false],
    ["cut-before-multibyte", "co-authored-by" + pad(77) + "ai{E9}", true],
    ["cut-inside-multibyte", "co-authored-by" + pad(78) + "ai{E9}", true],
    ["cut-inside-snow-2", "co-authored-by" + pad(78) + "ai{SNOW}", true],
    [
      "cut-inside-multibyte-before",
      "{E9}ai" + pad(77) + "co-authored-by",
      true,
    ],
    // The rest of the decision.
    ["agent-in-window", "Co-authored-by: Alice agent", true],
    ["agent-outside", "Co-authored-by: Alice" + pad(80) + "agent", false],
    [
      "agent-prefix-cut",
      "Co-authored-by: Alice" + pad(70) + "sub agent",
      false,
    ],
    ["alice", "Co-authored-by: Alice <alice@example.com>", false],
    [
      "bot-suffix",
      "Co-authored-by: dependabot[bot] <support@github.com>",
      true,
    ],
    ["identity-before-tag", "claude co-authored-by: Alice", true],
    ["long-s-trailer", "a{LONG_S}{LONG_S}i{LONG_S}ted-by: x", true],
    ["long-s-cursor", "co-authored-by: cur{LONG_S}or", true],
    ["long-s-before-claude", "co-authored-by: {LONG_S}claude", true],
    ["substring-cursorial", "co-authored-by: Cursorial", false],
    ["substring-maintain", "co-authored-by: maintain", false],
    [
      "two-tags-second",
      "co-authored-by: Alice" + pad(100) + "co-authored-by: ai",
      true,
    ],
    [
      "two-tags-neither",
      "co-authored-by: Alice" + pad(100) + "co-authored-by: Bob",
      false,
    ],
  ];
  for (const [name, input, expected] of cases)
    assert.equal(namesAttributionTrailer(expand(input)), expected, name);
});

// --- the family ----------------------------------------------------------------------------------

test("the green fixture passes", async () => {
  await withSkillsRepo(async (root) => {
    assert.deepEqual(await audit(root), { failures: [] });
  });
});

for (const text of [
  "Include the `Assisted-by: Claude:model` trailer.",
  "add `Co-authored-by: Copilot <1+Copilot@users.noreply.github.com>`",
  "Generated-by: codex",
  "Keep the Copilot co-authored-by trailer the triage subagent wrote.",
  "Co-authored-by: Claude <noreply@anthropic.com>",
  "Co-authored-by: dependabot[bot] <support@github.com>",
]) {
  test(`a trailer instruction is one finding: ${JSON.stringify(text)}`, async () => {
    await withSkillsRepo(async (root) => {
      addGit(root);
      writeFile(root, "skills/example/SKILL.md", text);
      assert.deepEqual(await findings(root), [
        stray("skills/example/SKILL.md"),
      ]);
    });
  });
}

for (const text of [
  "Credit a pair partner with `Co-authored-by: Alice <alice@example.com>`.",
  "Commit with the existing co-authored-by trailer convention.",
]) {
  test(`a human co-author trailer is allowed: ${JSON.stringify(text)}`, async () => {
    await withSkillsRepo(async (root) => {
      addGit(root);
      writeFile(root, "skills/example/SKILL.md", text);
      assert.deepEqual(await findings(root), []);
    });
  });
}

for (const rel of [
  "evals/skills/git.md",
  "docs/guide/05-build.md",
  "docs/surfaces/codex.md",
]) {
  test(`a trailer in ${rel} is one finding at that path`, async () => {
    await withSkillsRepo(async (root) => {
      addGit(root);
      writeFile(root, rel, "write the `Assisted-by:` trailer");
      assert.deepEqual(await findings(root), [stray(rel)]);
    });
  });
}

test("history and the rename record are not scanned", async () => {
  await withSkillsRepo(async (root) => {
    addGit(root);
    writeFile(
      root,
      "docs/specs/2026-10/feature-x/plan.md",
      "drop the `Assisted-by:` trailer",
    );
    writeFile(root, "docs/RENAMES.md", "Assisted-by was removed");
    assert.deepEqual(await findings(root), []);
  });
});

test("the prohibition is the only allowed mention, in both home files", async () => {
  await withSkillsRepo(async (root) => {
    addGit(root);
    writeFile(root, COMMIT, `#### 5. Execute commit\n\n${WRAPPED}\n`);
    assert.deepEqual(await findings(root), []);
  });
});

test("a git skill without the prohibition is a home finding", async () => {
  await withSkillsRepo(async (root) => {
    addSkill(root, "git", {
      body: "Commits follow the repository convention.\n",
    });
    assert.deepEqual(await findings(root), [home(GIT)]);
  });
});

test("the repository without a git skill is not held to the prohibition", async () => {
  await withSkillsRepo(async (root) => {
    assert.deepEqual(await findings(root), []);
  });
});

test("a stray mention beside the prohibition is one walk finding at the git skill", async () => {
  await withSkillsRepo(async (root) => {
    addGit(root, "- Add `Co-authored-by: Claude` when pairing.\n");
    assert.deepEqual(await findings(root), [stray(GIT)]);
  });
});

test("a commit reference without the prohibition is a home finding at its path", async () => {
  await withSkillsRepo(async (root) => {
    addGit(root);
    writeFile(
      root,
      COMMIT,
      "#### 5. Execute commit\n\nFollow the Git Safety Protocol.\n",
    );
    assert.deepEqual(await findings(root), [home(COMMIT)]);
  });
});

test("the commit reference is optional", async () => {
  await withSkillsRepo(async (root) => {
    addGit(root);
    assert.deepEqual(await findings(root), []);
  });
});

test("a home finding and a walk finding at one path come home first, and the rest sort by path", async () => {
  await withSkillsRepo(async (root) => {
    addSkill(root, "git", { body: "No rule here. Assisted-by: x\n" });
    writeFile(root, COMMIT, "No rule here either.\n");
    writeFile(root, "skills/aaa.md", "Generated-by: x");
    writeFile(root, ".unscanned/x.md", "Generated-by: x");
    assert.deepEqual(await findings(root), [
      stray("skills/aaa.md"),
      home(GIT),
      stray(GIT),
      home(COMMIT),
    ]);
  });
});

test("home findings sort together with walk findings from a later root, by path", async () => {
  await withSkillsRepo(async (root) => {
    addSkill(root, "git", { body: "No rule here.\n" });
    writeFile(root, "docs/guide/x.md", "Generated-by: x");
    writeFile(root, "evals/skills/y.md", "Generated-by: x");
    assert.deepEqual(await findings(root), [
      stray("docs/guide/x.md"),
      stray("evals/skills/y.md"),
      home(GIT),
    ]);
  });
});

test("a missing git skill ends the home check only: the commit reference is not checked and the walk still runs", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(root, COMMIT, "No prohibition in this file.\n");
    assert.deepEqual(await findings(root), []);
    writeFile(root, "skills/example/SKILL.md", "Assisted-by: x");
    assert.deepEqual(await findings(root), [stray("skills/example/SKILL.md")]);
  });
});

test("a git skill with no commit reference is no failure", async () => {
  await withSkillsRepo(async (root) => {
    addGit(root);
    remove(root, "skills/git/references");
    assert.deepEqual(await findings(root), []);
  });
});

test("the prohibition wrapped across lines with extra spaces and a tab is accepted", async () => {
  await withSkillsRepo(async (root) => {
    const wrapped = ATTRIBUTION_PROHIBITION.replace("Never add", "Never\n  add")
      .replace("trailer (", "trailer\t(")
      .replace(" to a commit", "   to a commit");
    assert.notEqual(wrapped, ATTRIBUTION_PROHIBITION);
    addSkill(root, "git", { body: `- ${wrapped}\n` });
    assert.deepEqual(await findings(root), []);
  });
});

test("a non-breaking space between the prohibition's words is folded too", async () => {
  await withSkillsRepo(async (root) => {
    const nbsp = String.fromCodePoint(0xa0);
    const spaced = ATTRIBUTION_PROHIBITION.replace(
      "attribution trailer",
      `attribution${nbsp}trailer`,
    );
    assert.notEqual(spaced, ATTRIBUTION_PROHIBITION);
    addSkill(root, "git", { body: `- ${spaced}\n` });
    assert.deepEqual(await findings(root), []);
  });
});

test("the prohibition plus a mention elsewhere in the file is one walk finding", async () => {
  await withSkillsRepo(async (root) => {
    addGit(root, "Also: Assisted-by: x\n");
    assert.deepEqual(await findings(root), [stray(GIT)]);
  });
});

test("a long-s spelling of the trailer fires", async () => {
  await withSkillsRepo(async (root) => {
    addGit(root);
    const long = String.fromCodePoint(0x17f);
    writeFile(
      root,
      "skills/example/SKILL.md",
      `a${long}${long}i${long}ted-by: x`,
    );
    assert.deepEqual(await findings(root), [stray("skills/example/SKILL.md")]);
  });
});

test("the byte window decides through the whole family", async () => {
  const e9 = String.fromCodePoint(0xe9);
  for (const [text, expected] of [
    ["co-authored-by" + pad(78) + "ai", true],
    ["co-authored-by" + pad(79) + "ai", false],
    ["co-authored-by" + e9.repeat(39) + "ai", true],
    ["co-authored-by" + e9.repeat(40) + "ai", false],
    ["ai" + e9.repeat(39) + "co-authored-by", true],
    ["ai" + e9.repeat(40) + "co-authored-by", false],
  ] as const) {
    await withSkillsRepo(async (root) => {
      addGit(root);
      writeFile(root, "skills/example/SKILL.md", text);
      assert.deepEqual(
        await findings(root),
        expected ? [stray("skills/example/SKILL.md")] : [],
        text,
      );
    });
  }
});

test("a file under a tests directory and a file holding a NUL byte are both scanned", async () => {
  await withSkillsRepo(async (root) => {
    addGit(root);
    writeFile(root, "skills/zealot/references/tests/x.md", "Assisted-by: x");
    writeFile(root, "skills/example/bin.md", "a\0b Generated-by: x");
    assert.deepEqual(await findings(root), [
      stray("skills/example/bin.md"),
      stray("skills/zealot/references/tests/x.md"),
    ]);
  });
});

test("a root that is missing is skipped", async () => {
  await withSkillsRepo(async (root) => {
    remove(root, "skills");
    remove(root, "docs");
    assert.deepEqual(await audit(root), { failures: [] });
  });
});

test(
  "a root that is a symlink is skipped even when its target holds a trailer",
  { skip: NO_LINKS },
  async () => {
    await withOutside(async (outside) => {
      writeFile(outside, "x.md", "Assisted-by: x");
      await withSkillsRepo(async (root) => {
        assert.equal(link(root, "docs/guide", outside, "dir"), true);
        assert.deepEqual(await audit(root), { failures: [] });
      });
    });
  },
);

test("a root that is a file is walked as that one file and named by the root's own path", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(root, "docs/guide", "Assisted-by: x");
    assert.deepEqual(await findings(root), [stray("docs/guide")]);
  });
});

test(
  "a symlinked file inside a root is not reported",
  { skip: NO_LINKS },
  async () => {
    await withOutside(async (outside) => {
      writeFile(outside, "x.md", "Assisted-by: x");
      await withSkillsRepo(async (root) => {
        assert.equal(
          link(
            root,
            "skills/example/link.md",
            path.join(outside, "x.md"),
            "file",
          ),
          true,
        );
        assert.deepEqual(await audit(root), { failures: [] });
      });
    });
  },
);

test("a git skill that is a directory is an audit error and the walk does not run", async () => {
  await withSkillsRepo(async (root) => {
    mkdir(root, GIT);
    writeFile(root, "skills/example/SKILL.md", "Assisted-by: x");
    const result = await audit(root);
    assert.deepEqual(result.failures, []);
    assert.equal(
      result.error,
      "skills: reading skills/git/SKILL.md: skills/git/SKILL.md: is not a regular file",
    );
  });
});

test(
  "a git skill directory that is a symlink is an audit error naming the link",
  { skip: NO_LINKS },
  async () => {
    await withOutside(async (outside) => {
      writeFile(outside, "SKILL.md", ATTRIBUTION_PROHIBITION);
      await withSkillsRepo(async (root) => {
        assert.equal(link(root, "skills/git", outside, "dir"), true);
        const result = await audit(root);
        assert.deepEqual(result.failures, []);
        assert.equal(
          result.error,
          "skills: reading skills/git/SKILL.md: skills/git: passes through a link",
        );
      });
    });
  },
);

test(
  "a path below a regular file is an audit error on a host that reports ENOTDIR",
  {
    skip:
      process.platform === "win32"
        ? "Windows reports a path below a file as not existing, so the home check ends and the root is skipped"
        : false,
  },
  async () => {
    await withSkillsRepo(async (root) => {
      remove(root, "skills");
      writeFile(root, "skills", "a file where a directory should be");
      const result = await audit(root);
      assert.deepEqual(result.failures, []);
      assert.equal(
        result.error,
        "skills: reading skills/git/SKILL.md: skills/git: is not a directory",
      );
    });
  },
);

test("a file that cannot be read is an audit error naming it and the error code", async () => {
  await withSkillsRepo(async (root) => {
    const result = await withSeams(
      {
        readFile: () => {
          throw Object.assign(new Error("denied"), { code: "EACCES" });
        },
      },
      () => audit(root),
    );
    assert.deepEqual(result.failures, []);
    assert.equal(
      result.error,
      "skills: reading skills/challenge/SKILL.md: skills/challenge/SKILL.md: EACCES",
    );
  });
});

test("a failing directory read is an audit error naming the directory and drops the findings so far", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(root, "skills/aaa.md", "Assisted-by: x");
    mkdir(root, "docs/guide");
    const real = seams.readDir;
    const result = await withSeams(
      {
        readDir: (dir) => {
          if (dir === path.join(root, "docs", "guide"))
            throw Object.assign(new Error("denied"), { code: "EACCES" });
          return real(dir);
        },
      },
      () => audit(root),
    );
    assert.deepEqual(result.failures, []);
    assert.equal(result.error, "docs/guide: EACCES");
  });
});

/** Runs the audit with the inspection of `target` failing with `error`, and every other path inspected as usual. */
function auditFailingLstat(
  root: string,
  target: string,
  error: unknown,
): Promise<{ failures: Failure[]; error?: string }> {
  const real = seams.lstat;
  return withSeams(
    {
      lstat: (candidate) => {
        if (candidate === target) throw error;
        return real(candidate);
      },
    },
    () => audit(root),
  );
}

const gitHome = (root: string): string =>
  path.join(root, "skills", "git", "SKILL.md");

test("an inspection that fails is an audit error naming the file and the error code", async () => {
  await withSkillsRepo(async (root) => {
    addGit(root);
    const target = gitHome(root);
    const result = await auditFailingLstat(
      root,
      target,
      Object.assign(new Error("EACCES: permission denied, lstat"), {
        code: "EACCES",
        syscall: "lstat",
        path: target,
      }),
    );
    assert.deepEqual(result.failures, []);
    assert.equal(result.error, `skills: reading ${GIT}: ${GIT}: EACCES`);
  });
});

test("an inspection failure with no code is unreadable", async () => {
  await withSkillsRepo(async (root) => {
    addGit(root);
    const result = await auditFailingLstat(
      root,
      gitHome(root),
      new Error("odd failure"),
    );
    assert.equal(result.error, `skills: reading ${GIT}: ${GIT}: unreadable`);
  });
});

test("a file error for another path keeps its own path and reason", async () => {
  await withSkillsRepo(async (root) => {
    addGit(root);
    const result = await auditFailingLstat(
      root,
      gitHome(root),
      new RepoFileError("symlink", "somewhere"),
    );
    assert.equal(
      result.error,
      `skills: reading ${GIT}: somewhere: passes through a link`,
    );
  });
});

test("an inspection that throws no error object is unreadable", async () => {
  await withSkillsRepo(async (root) => {
    addGit(root);
    const result = await auditFailingLstat(root, gitHome(root), null);
    assert.equal(result.error, `skills: reading ${GIT}: ${GIT}: unreadable`);
  });
});

test("a home file read that fails as missing ends the home check without an error", async () => {
  await withSkillsRepo(async (root) => {
    addGit(root);
    const target = gitHome(root);
    const real = seams.readFile;
    let first = true;
    const result = await withSeams(
      {
        readFile: (candidate) => {
          if (candidate === target && first) {
            first = false;
            throw new RepoFileError("missing", GIT);
          }
          return real(candidate);
        },
      },
      () => audit(root),
    );
    assert.deepEqual(result, { failures: [] });
  });
});

test("a home file read that throws no error object is unreadable", async () => {
  await withSkillsRepo(async (root) => {
    addGit(root);
    const target = gitHome(root);
    const real = seams.readFile;
    const result = await withSeams(
      {
        readFile: (candidate) => {
          if (candidate === target) throw null;
          return real(candidate);
        },
      },
      () => audit(root),
    );
    assert.equal(result.error, `skills: reading ${GIT}: ${GIT}: unreadable`);
  });
});

test("an audit error keeps the failure that caused it as its cause", async () => {
  await withSkillsRepo(async (root) => {
    addGit(root);
    mkdir(root, "docs/guide");
    for (const target of [gitHome(root), path.join(root, "docs", "guide")]) {
      const cause = Object.assign(new Error("denied"), {
        code: "EACCES",
        syscall: "lstat",
        path: target,
      });
      const real = seams.lstat;
      assert.throws(
        () =>
          withSeams(
            {
              lstat: (candidate) => {
                if (candidate === target) throw cause;
                return real(candidate);
              },
            },
            () => attributionRules(root),
          ),
        (error: unknown) =>
          error instanceof Error &&
          error.cause instanceof RepoFileError &&
          error.cause.cause === cause,
        target,
      );
    }
  });
});

// --- the whole-domain scenarios ------------------------------------------------------------------------------------

const CONTENT_SLICE = sliceNamed("content");

const COMMIT_NOTES = "skills/prototype/references/commit-notes.md";
const ORIGIN_NOTE = `${PROTOTYPE_REFS}/origin-note.md`;

test("ai-attribution-trailer", async () => {
  assert.deepEqual(await auditEveryFamily(breaker(RULE)), {
    failures: [stray(COMMIT_NOTES)],
  });
});

test("a stub failure after an ai-attribution-trailer failure", async () => {
  assert.deepEqual(
    await auditEveryFamily(breaker(RULE), { stream: STUB_ONE }),
    {
      failures: [
        stray(COMMIT_NOTES),
        {
          rule: "eval-stub-rule",
          path: "evals/skills.yaml",
          message: "stub finding",
        },
      ],
    },
  );
});

test("the git skill without the prohibition", async () => {
  assert.deepEqual(
    await auditSlice(CONTENT_SLICE, gitSkill("Commit with care.\n")),
    { failures: [home(GIT)] },
  );
});

test("a commit reference lacking the prohibition next to a good git skill", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      all(gitSkill(`${PROHIBITION}\n`), put(GIT_COMMIT, "Commit with care.\n")),
    ),
    { failures: [home(COMMIT)] },
  );
});

test("a prohibition followed by a stray trailer", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      gitSkill(`${PROHIBITION}\n\nAdd Generated-by: x.\n`),
    ),
    { failures: [stray(GIT)] },
  );
});

test("a co-author trailer with an AI identity 79 bytes after it", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      ref("origin-note.md", `Co-authored-by: ${E_ACUTE.repeat(37)} bot\n`),
    ),
    { failures: [stray(ORIGIN_NOTE)] },
  );
});

test("an AI identity 79 bytes before a co-author trailer", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      ref("origin-note.md", `bot ${E_ACUTE.repeat(37)} Co-authored-by: x\n`),
    ),
    { failures: [stray(ORIGIN_NOTE)] },
  );
});

test("a co-author trailer with an identity glued to a multibyte character", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      ref("origin-note.md", `Co-authored-by: x${E_ACUTE}ai\n`),
    ),
    { failures: [stray(ORIGIN_NOTE)] },
  );
});

test("an assisted-by trailer spelled with long s", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      ref("origin-note.md", `A${LONG_S}${LONG_S}i${LONG_S}ted-by: x\n`),
    ),
    { failures: [stray(ORIGIN_NOTE)] },
  );
});

test("a generated-by trailer in capitals", async () => {
  assert.deepEqual(
    await auditSlice(CONTENT_SLICE, ref("origin-note.md", "GENERATED-BY: x\n")),
    { failures: [stray(ORIGIN_NOTE)] },
  );
});

test("a trailer under the evals skills directory", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      put("evals/skills/bank.md", "Assisted-by: Claude\n"),
    ),
    { failures: [stray("evals/skills/bank.md")] },
  );
});

test("a trailer under the guide directory", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      put("docs/guide/commits.md", "Assisted-by: Claude\n"),
    ),
    { failures: [stray("docs/guide/commits.md")] },
  );
});

test("a trailer under the surfaces directory", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      put("docs/surfaces/extra.md", "Assisted-by: Claude\n"),
    ),
    { failures: [stray("docs/surfaces/extra.md")] },
  );
});

test("a trailer in a file holding a NUL byte", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      ref("bin.md", `${NUL}\nAssisted-by: Claude\n`),
    ),
    { failures: [stray(`${PROTOTYPE_REFS}/bin.md`)] },
  );
});

test("a trailer in a tests directory", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      put(`${PROTOTYPE_REFS}/tests/t.md`, "Assisted-by: Claude\n"),
    ),
    { failures: [stray(`${PROTOTYPE_REFS}/tests/t.md`)] },
  );
});
