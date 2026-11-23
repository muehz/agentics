import assert from "node:assert/strict";
import path from "node:path";
import { test } from "node:test";
import type { Failure } from "../../check.ts";
import {
  CLEAN_RUN,
  auditEveryFamily,
  auditSlice,
  sliceNamed,
  all,
  challengeStepThree,
  crlf,
  GIT_COMMIT,
  gitSkill,
  PROHIBITION,
  PROTOTYPE_SKILL,
  put,
} from "../../testing/skills-seeds.ts";
import {
  addSkill,
  BREAKERS,
  challengeFixtureMD,
  contractFixtureMD,
  decisionQuestionsCopy,
  link,
  mutateFile,
  remove,
  skillMD,
  withSkillsRepo,
  writeFile,
} from "../../testing/skills-repo.ts";
import { isFileErrorOfKind, seams, withSeams } from "./files.ts";
import { DOMAIN_FAMILIES, run } from "./index.ts";
import { questionFormatRules } from "./question-format.ts";

const CONTRACT = "skills/routing/references/contracts/question-format.md";
const GIT = "skills/git/SKILL.md";
const CHALLENGE = "skills/challenge/SKILL.md";
const REF = "skills/new-skill/references/questions.md";
const CANONICAL_LINK =
  "[Question format](../routing/references/contracts/question-format.md)";
const REQUIRED_LINK = "must link to " + CONTRACT;
const DUPLICATE =
  "replace the local decision-question format with a link to " + CONTRACT;
const NOT_COLLECTING = /must carry a .*Collecting answers/;
const FENCE = "```";
const TEMPLATE = [
  "❓ **Q1** - **<question title>**: <question body, might be multiple paragraphs, including multiple choices>",
  "",
  "➡️ <your recommended answer>",
].join("\n");

async function audit(root: string) {
  const families = DOMAIN_FAMILIES.filter(
    (family) => family.name === "question-format",
  );
  assert.equal(families.length, 1);
  return run(
    root,
    { short: false, fast: false, diagnostics: () => {}, env: process.env },
    { families },
  );
}

const finding = (path: string, message: string) => ({
  rule: "question-format",
  path,
  message,
});

test("rejects the historic inline policy even beside an active canonical link", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(
      root,
      GIT,
      skillMD("git") + "\n" + CANONICAL_LINK + "\n\n" + decisionQuestionsCopy(),
    );
    assert.deepEqual(await audit(root), {
      failures: [finding(GIT, DUPLICATE)],
    });
  });
});

test("challenge's older template does not exempt it from question-format", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(
      root,
      CHALLENGE,
      challengeFixtureMD() + "\n" + CANONICAL_LINK + "\n",
    );
    assert.deepEqual(await audit(root), {
      failures: [finding(CHALLENGE, DUPLICATE)],
    });
  });
});

test("rejects a generic template in a previously unregistered skill reference", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(
      root,
      REF,
      "# Questions\n\n" + FENCE + "text\n" + TEMPLATE + "\n" + FENCE + "\n",
    );
    assert.deepEqual(await audit(root), {
      failures: [finding(REF, DUPLICATE)],
    });
  });
});

test("allows a filled example that is not a generic template", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(
      root,
      REF,
      "# An example\n\n" +
        FENCE +
        "text\n❓ **Q1** - **Branch**: Use main or develop?\n\n➡️ Use main.\n" +
        FENCE +
        "\n",
    );
    assert.deepEqual(await audit(root), { failures: [] });
  });
});

test("removing the canonical link from an existing required consumer fails", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(root, GIT, skillMD("git") + "\n" + CANONICAL_LINK + "\n");
    assert.deepEqual(await audit(root), { failures: [] });
    writeFile(root, GIT, skillMD("git"));
    assert.deepEqual(await audit(root), {
      failures: [finding(GIT, REQUIRED_LINK)],
    });
  });
});

test("lookalikes, images, malformed links, and quoted links do not meet the required link", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(
      root,
      "skills/git/references/question-format.md",
      "# Lookalike\n",
    );
    writeFile(
      root,
      GIT,
      skillMD("git") +
        "\n[Lookalike](references/question-format.md)\n\n!" +
        CANONICAL_LINK +
        "\n\n" +
        CANONICAL_LINK.slice(1) +
        "\n\n> " +
        CANONICAL_LINK +
        "\n\n`" +
        CANONICAL_LINK +
        "`\n\n" +
        FENCE +
        "\n" +
        CANONICAL_LINK +
        "\n" +
        FENCE +
        "\n",
    );
    assert.deepEqual(await audit(root), {
      failures: [finding(GIT, REQUIRED_LINK)],
    });
  });
});

test("relative links with a dot segment and a canonical heading are valid", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(
      root,
      GIT,
      skillMD("git") +
        "\n[Collect answers](../routing/./references/contracts/question-format.md#collecting-answers)\n",
    );
    assert.deepEqual(await audit(root), { failures: [] });
  });
});

for (const [title, separator] of [
  ["a space", " "],
  ["a tab", "\t"],
] as const) {
  test("allows an inline link with a title after " + title, async () => {
    await withSkillsRepo(async (root) => {
      writeFile(
        root,
        GIT,
        skillMD("git") +
          "\n" +
          CANONICAL_LINK.slice(0, -1) +
          separator +
          '"Questions")\n',
      );
      assert.deepEqual(await audit(root), { failures: [] });
    });
  });
}

test("a generic template quoted as historical data is allowed", async () => {
  await withSkillsRepo(async (root) => {
    const archived = decisionQuestionsCopy()
      .trimEnd()
      .split("\n")
      .map((line) => "> " + line)
      .join("\n");
    writeFile(root, REF, "# Historical example\n\n" + archived + "\n");
    assert.deepEqual(await audit(root), { failures: [] });
  });
});

test("findings identify the canonical problem first and then consumer paths in audit order", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(
      root,
      CONTRACT,
      contractFixtureMD().replace("## Collecting answers", "## Other section"),
    );
    writeFile(root, GIT, skillMD("git"));
    writeFile(root, CHALLENGE, skillMD("challenge"));
    const result = await audit(root);
    assert.equal(result.error, undefined);
    assert.deepEqual(
      result.failures.map((f) => f.path),
      [CONTRACT, CHALLENGE, GIT],
    );
  });
});

test("a read failure aborts this family, including an earlier copy it could have found", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(root, CHALLENGE, challengeFixtureMD());
    const denied = path.join(root, ...GIT.split("/"));
    writeFile(root, GIT, skillMD("git"));
    const read = seams.readFile;
    const result = await withSeams(
      {
        readFile: (name) => {
          if (name === denied) throw new Error("fixture: cannot read consumer");
          return read(name);
        },
      },
      () => audit(root),
    );
    assert.deepEqual(result, {
      failures: [],
      error: `${GIT}: unreadable`,
    });
  });
});

test("a generic copy hidden in an HTML comment is data, not live guidance", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(
      root,
      REF,
      "# Questions\n\n<!--\n" + decisionQuestionsCopy() + "-->\n",
    );
    assert.deepEqual(await audit(root), { failures: [] });
  });
});

test("the canonical file still requires its collecting-answers heading outside fences", async () => {
  await withSkillsRepo(async (root) => {
    const body = contractFixtureMD().replace(
      "## Collecting answers",
      FENCE + "\n## Collecting answers\n" + FENCE,
    );
    writeFile(root, CONTRACT, body);
    const result = await audit(root);
    assert.equal(result.error, undefined);
    assert.equal(result.failures.length, 1);
    assert.equal(result.failures[0]?.path, CONTRACT);
    assert.match(result.failures[0]?.message ?? "", NOT_COLLECTING);
  });
});

test("the canonical file still requires its fenced template", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(root, CONTRACT, "# Question format\n\n## Collecting answers\n");
    const result = await audit(root);
    assert.match(result.error ?? "", /no fenced block found after/);
    assert.deepEqual(result.failures, []);
  });
});

test("a later unrelated code block cannot stand in for the canonical example", async () => {
  await withSkillsRepo(async (root) => {
    const text = contractFixtureMD().replace(
      "Format each question exactly like so:\n\n",
      "Format each question exactly like so:\n\nThe following is a separate example.\n\n",
    );
    writeFile(root, CONTRACT, text);
    const result = await audit(root);
    assert.match(result.error ?? "", /no fenced block found after/);
    assert.deepEqual(result.failures, []);
  });
});

test("a missing canonical file is an audit error", async () => {
  await withSkillsRepo(async (root) => {
    remove(root, CONTRACT);
    const result = await audit(root);
    assert.equal(
      result.error,
      `skills: reading ${CONTRACT}: ${CONTRACT}: does not exist`,
    );
    assert.deepEqual(result.failures, []);
    assert.throws(
      () => questionFormatRules(root),
      (error: unknown) =>
        error instanceof Error && isFileErrorOfKind(error.cause, "missing"),
    );
  });
});

test("a linked canonical directory is refused even if the link target has the expected file", async (t) => {
  await withSkillsRepo(async (root) => {
    writeFile(root, "alternate/question-format.md", contractFixtureMD());
    remove(root, "skills/routing/references/contracts");
    if (
      !link(
        root,
        "skills/routing/references/contracts",
        path.resolve(root, "alternate"),
        process.platform === "win32" ? "junction" : "dir",
      )
    )
      return t.skip("the host refuses directory links");
    const result = await audit(root);
    assert.equal(
      result.error,
      `skills: reading ${CONTRACT}: skills/routing/references/contracts: passes through a link`,
    );
    assert.deepEqual(result.failures, []);
  });
});

const POSTED =
  "posted in the visible chat first, as one message, before any ask-tool call";
const BLOCKQUOTE = "The whole round sits in one blockquote";
const missing = (literal: string) =>
  finding(
    CONTRACT,
    `"## Collecting answers" must carry the sentence "${literal}": skills link to this file for how a round is posted`,
  );

test("the canonical file's collecting-answers section carries both posted-round sentences", async () => {
  await withSkillsRepo(async (root) => {
    const text = contractFixtureMD();
    assert.ok(text.includes(POSTED) && text.includes(BLOCKQUOTE));
    writeFile(root, CONTRACT, text);
    assert.deepEqual(await audit(root), { failures: [] });
  });
});

test("a collecting-answers section without the posted-first sentence fails", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(
      root,
      CONTRACT,
      contractFixtureMD().replace(POSTED, "shown in the ask dialog"),
    );
    assert.deepEqual(await audit(root), { failures: [missing(POSTED)] });
  });
});

test("a collecting-answers section without the blockquote sentence fails", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(
      root,
      CONTRACT,
      contractFixtureMD().replace(BLOCKQUOTE, "The round is plain text"),
    );
    assert.deepEqual(await audit(root), { failures: [missing(BLOCKQUOTE)] });
  });
});

test("a posted-round sentence wrapped across lines still counts", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(
      root,
      CONTRACT,
      contractFixtureMD().replace(
        POSTED,
        POSTED.replace(" as one message,", "\n  as one  message,"),
      ),
    );
    assert.deepEqual(await audit(root), { failures: [] });
  });
});

test("a posted-round sentence only inside a blockquote example does not count", async () => {
  await withSkillsRepo(async (root) => {
    const text = contractFixtureMD();
    const line = text.split("\n").find((l) => l.includes(BLOCKQUOTE)) ?? "";
    assert.notEqual(line, "");
    writeFile(root, CONTRACT, text.replace(line, "> " + line));
    assert.deepEqual(await audit(root), { failures: [missing(BLOCKQUOTE)] });
  });
});

test("a posted-round sentence outside the collecting-answers section does not count", async () => {
  await withSkillsRepo(async (root) => {
    const text = contractFixtureMD();
    const line = text.split("\n").find((l) => l.includes(POSTED)) ?? "";
    assert.notEqual(line, "");
    writeFile(
      root,
      CONTRACT,
      text
        .replace(line + "\n", "")
        .replace(
          "# Question format\n\n",
          "# Question format\n\n" + line + "\n\n",
        ) +
        "\n## Later section\n\n" +
        line +
        "\n",
    );
    assert.deepEqual(await audit(root), { failures: [missing(POSTED)] });
  });
});

// --- question-format and prohibition changes: every family, or one slice on the pasted-format fixture ----------------

/** The shared-contracts failure of a rewritten skill that dropped its link to the contract file `contract`. */
const unlinkedContract = (at: string, contract: string): Failure => ({
  rule: "shared-contracts",
  path: at,
  message: `must link to skills/routing/references/contracts/${contract}`,
});

/** The skills of the pasted-format fixture that paste the decision-question format, each told to link to it instead. */
const PASTED_COPIES: Failure[] = [
  "challenge",
  "docs",
  "execute",
  "forge",
  "gate",
  "gh",
  "prototype",
  "review",
  "triage",
].map((name) => finding(`skills/${name}/SKILL.md`, DUPLICATE));

/** The pasted-format fixture's missing links to the other shared contracts, which only the every-family run reports. */
const PASTED_FORMAT_UNLINKED: Failure[] = (
  [
    ["challenge", "principles-check.md"],
    ["decompose", "workflow-steps.md"],
    ["decompose", "principles-check.md"],
    ["decompose", "empirical-questions.md"],
    ["execute", "workflow-steps.md"],
    ["execute", "principles-check.md"],
    ["gate", "workflow-steps.md"],
    ["gate", "principles-check.md"],
    ["prototype", "empirical-questions.md"],
    ["review", "workflow-steps.md"],
    ["routing", "workflow-steps.md"],
    ["routing", "empirical-questions.md"],
  ] as const
).map(([name, contract]) =>
  unlinkedContract(`skills/${name}/SKILL.md`, contract),
);

test("question-format", async () => {
  // Dropping challenge's Collecting answers paragraph adds no finding: question-format reports only the pasted copies.
  const breaker = BREAKERS["question-format"];
  assert.ok(breaker !== undefined);
  assert.deepEqual(await auditEveryFamily(breaker, { pastedFormat: true }), {
    failures: [...PASTED_COPIES, ...PASTED_FORMAT_UNLINKED],
  });
  assert.deepEqual(await auditSlice(sliceNamed("workflow"), breaker), {
    failures: PASTED_COPIES,
  });
});

test("a challenge file with CRLF line endings", async () => {
  // A rewritten challenge that pastes the block reads as a local copy whatever its line endings.
  assert.deepEqual(
    await auditEveryFamily(put(CHALLENGE, crlf(challengeFixtureMD()))),
    {
      failures: [
        finding(CHALLENGE, DUPLICATE),
        unlinkedContract(CHALLENGE, "principles-check.md"),
      ],
    },
  );
});

test("a contract file with CRLF line endings", async () => {
  // The heading, the block and the posted-round sentences are still found when every line ends in CRLF.
  assert.deepEqual(
    await auditEveryFamily(put(CONTRACT, crlf(contractFixtureMD()))),
    CLEAN_RUN,
  );
});

test("a required skill that carries neither a link to the contract file nor an inline copy", async () => {
  assert.deepEqual(
    await auditEveryFamily(put(PROTOTYPE_SKILL, skillMD("prototype"))),
    {
      failures: [
        finding(PROTOTYPE_SKILL, REQUIRED_LINK),
        unlinkedContract(PROTOTYPE_SKILL, "empirical-questions.md"),
      ],
    },
  );
});

test("a required skill whose inline copy ends its lines in carriage returns", async () => {
  // Trailing carriage returns do not hide the copy. The copy's reference to challenge's Step 3 finds no such step in the
  // green fixture's challenge, which links to the contract file instead.
  assert.deepEqual(
    await auditEveryFamily(
      put(
        PROTOTYPE_SKILL,
        skillMD("prototype") +
          "\n" +
          decisionQuestionsCopy(false).replace(/\n/g, "\r\r\n"),
      ),
    ),
    {
      failures: [
        finding(PROTOTYPE_SKILL, DUPLICATE),
        {
          rule: "skill-xref-step",
          path: PROTOTYPE_SKILL,
          message:
            "line 13: challenge Step 3: challenge has no heading or bold lead-in for Step 3",
        },
        unlinkedContract(PROTOTYPE_SKILL, "empirical-questions.md"),
      ],
    },
  );
});

test("challenge holds neither the marker nor a link", async () => {
  assert.deepEqual(
    await auditEveryFamily(put(CHALLENGE, challengeStepThree("No block."))),
    {
      failures: [
        finding(CHALLENGE, REQUIRED_LINK),
        unlinkedContract(CHALLENGE, "principles-check.md"),
      ],
    },
  );
});

test("challenge holds the marker and no closed block after it", async () => {
  // An unclosed fence after the marker still reads as a local copy of the format.
  assert.deepEqual(
    await auditEveryFamily(
      put(
        CHALLENGE,
        challengeStepThree(
          "Format each question exactly like so:\n\n```\nA fence that never closes.",
        ),
      ),
    ),
    {
      failures: [
        finding(CHALLENGE, DUPLICATE),
        unlinkedContract(CHALLENGE, "principles-check.md"),
      ],
    },
  );
});

test("the contract file without its Collecting answers heading", async () => {
  assert.deepEqual(
    await auditEveryFamily((r) =>
      mutateFile(
        r,
        CONTRACT,
        "## Collecting answers\n\nCollect the answers.\n",
        "Collect the answers.\n",
      ),
    ),
    {
      failures: [
        finding(
          CONTRACT,
          'must carry a "## Collecting answers" section outside any code fence: skills link to this file for how answers are collected',
        ),
      ],
    },
  );
});

test("the allowlisted constitution file with a dangling link", async () => {
  // The metadata slice runs no question-format family, and reference-link skips the allowlisted constitution file.
  assert.deepEqual(
    await auditSlice(
      sliceNamed("metadata"),
      all(
        (r) => addSkill(r, "scaffold"),
        put(
          "skills/scaffold/references/governance/constitution.md",
          "[x](./gone.md)\n",
        ),
      ),
    ),
    CLEAN_RUN,
  );
});

test("the git skill stating the prohibition without a commit reference", async () => {
  // The content slice runs no question-format family, and the git skill states the trailer prohibition.
  assert.deepEqual(
    await auditSlice(sliceNamed("content"), gitSkill(`${PROHIBITION}\n`)),
    CLEAN_RUN,
  );
});

test("a commit reference stating the prohibition with the git skill", async () => {
  assert.deepEqual(
    await auditSlice(
      sliceNamed("content"),
      all(gitSkill(`${PROHIBITION}\n`), put(GIT_COMMIT, `${PROHIBITION}\n`)),
    ),
    CLEAN_RUN,
  );
});

test("a wrapped prohibition", async () => {
  // The prohibition still counts when its words are wrapped across lines.
  assert.deepEqual(
    await auditSlice(
      sliceNamed("content"),
      all(
        gitSkill(`${PROHIBITION.replace(/ /g, "\n")}\n`),
        put(GIT_COMMIT, `${PROHIBITION.replace(/, /g, ",\n  ")}\n`),
      ),
    ),
    CLEAN_RUN,
  );
});
