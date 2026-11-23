import assert from "node:assert/strict";
import { mkdtempSync, renameSync, rmSync, symlinkSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { test } from "node:test";
import type { Failure, RunOptions } from "../../check.ts";
import {
  auditEveryFamily,
  auditSlice,
  sliceNamed,
  all,
  skillWithMetadata,
  xText,
  type Mutation,
} from "../../testing/skills-seeds.ts";
import {
  addSkill,
  BREAKERS,
  link,
  mkdir,
  mutateFile,
  remove,
  skillMD,
  withSkillsRepo,
  writeFile,
} from "../../testing/skills-repo.ts";
import { FAMILY } from "./artifacts.ts";
import { RepoFileError } from "../../repo-file-error.ts";
import { seams, withSeams, type Seams } from "./files.ts";
import { run } from "./index.ts";
import { canSymlink, SYMLINK_SKIP } from "../../testing/host.ts";

const options: RunOptions = {
  short: false,
  fast: false,
  diagnostics: () => {},
  env: process.env,
};

const DECLARED = "skill-artifact-declared";
const EXPECTED = "skill-artifact-expected";
const UNIQUE = "skill-artifact-unique";

const SPEC = "skills/spec/SKILL.md";
const TDD = "skills/tdd/SKILL.md";
const INTENT = "skills/intent/SKILL.md";
const DECOMPOSE = "skills/decompose/SKILL.md";

const audit = (root: string) => run(root, options, { families: [FAMILY] });

/** The green fixture plus the chain skills it lacks and one skill that owns nothing. */
async function withChain<T>(body: (root: string) => Promise<T> | T) {
  return withSkillsRepo(async (root) => {
    addSkill(root, "intent");
    addSkill(root, "spec");
    addSkill(root, "tdd");
    return body(root);
  });
}

/** The (rule, path) pairs of a run that ended without an audit error. */
async function found(root: string): Promise<string[][]> {
  const result = await audit(root);
  assert.equal(result.error, undefined);
  return result.failures.map((one) => [one.rule, one.path]);
}

async function messages(root: string): Promise<string[]> {
  const result = await audit(root);
  assert.equal(result.error, undefined);
  return result.failures.map((one) => one.message);
}

function rules(failures: string[][]): string[] {
  return failures.map((pair) => pair[0] ?? "");
}

/** Replaces the artifact line of a chain skill's frontmatter. */
function declare(root: string, rel: string, from: string, to: string): void {
  mutateFile(root, rel, `    artifact: ${from}`, to);
}

const NO_LINKS = canSymlink() ? false : SYMLINK_SKIP;

test("the family is named artifact and reports its three rules", () => {
  assert.equal(FAMILY.name, "artifact");
  assert.deepEqual(FAMILY.rules, [DECLARED, EXPECTED, UNIQUE]);
});

test("TestSkillArtifactRulesSilentOnCleanRepo", async () => {
  await withChain(async (root) => {
    assert.deepEqual(await found(root), []);
  });
});

test("TestSkillArtifactAbsentOnNonOwner", async () => {
  await withChain(async (root) => {
    assert.ok(!rules(await found(root)).includes(DECLARED));
  });
});

test("TestSkillArtifactDeclared", async () => {
  await withChain(async (root) => {
    mutateFile(root, SPEC, "    artifact: spec.md\n", "");
    assert.deepEqual(await found(root), [[DECLARED, SPEC]]);
    assert.deepEqual(await messages(root), [
      'chain skill "spec" owns ["spec.md"] but declares no metadata.artifact',
    ]);
  });
});

test("TestSkillArtifactExpectedOnNonOwner", async () => {
  await withChain(async (root) => {
    mutateFile(root, TDD, "metadata:\n", "metadata:\n    artifact: tests.md\n");
    assert.deepEqual(await found(root), [[EXPECTED, TDD]]);
    assert.deepEqual(await messages(root), [
      'skill "tdd" declares metadata.artifact ["tests.md"] but owns no chain artifact; the field is absent, not empty, on a non-owner',
    ]);
  });
});

test("TestSkillArtifactExpectedWrongValue", async () => {
  await withChain(async (root) => {
    declare(root, SPEC, "spec.md", "    artifact: requirements.md");
    assert.deepEqual(await found(root), [[EXPECTED, SPEC]]);
    assert.deepEqual(await messages(root), [
      'skill "spec" declares metadata.artifact ["requirements.md"]; the chain boundary design assigns it ["spec.md"]',
    ]);
  });
});

test("TestSkillArtifactUnique", async () => {
  await withChain(async (root) => {
    declare(root, INTENT, "intent.md", "    artifact: plan.md");
    const failures = await found(root);
    assert.deepEqual(failures, [
      [UNIQUE, DECOMPOSE],
      [EXPECTED, INTENT],
    ]);
    assert.deepEqual(await messages(root), [
      `artifact "plan.md" is declared by 2 skills: ${DECOMPOSE}, ${INTENT}`,
      'skill "intent" declares metadata.artifact ["plan.md"]; the chain boundary design assigns it ["intent.md"]',
    ]);
  });
});

test("TestPlanOwnershipCannotBeShared", async () => {
  await withChain(async (root) => {
    declare(root, INTENT, "intent.md", "    artifact: [intent.md, plan.md]");
    const failures = await found(root);
    assert.ok(rules(failures).includes(UNIQUE));
    assert.deepEqual(failures, [
      [UNIQUE, DECOMPOSE],
      [EXPECTED, INTENT],
    ]);
  });
});

test("TestSkillArtifactEmptyValueOnNonOwner", async () => {
  await withChain(async (root) => {
    mutateFile(root, TDD, "metadata:\n", "metadata:\n    artifact:\n");
    assert.deepEqual(await found(root), [[EXPECTED, TDD]]);
  });
});

test("TestSkillArtifactEmptyValueOnOwner", async () => {
  await withChain(async (root) => {
    declare(root, SPEC, "spec.md", "    artifact:");
    const failures = await found(root);
    assert.deepEqual(failures, [[EXPECTED, SPEC]]);
    assert.ok(!rules(failures).includes(DECLARED));
    assert.deepEqual(await messages(root), [
      'skill "spec" declares metadata.artifact [""]; the chain boundary design assigns it ["spec.md"]',
    ]);
  });
});

test("an empty value, a tilde and null each declare one empty item and are not recorded", async () => {
  for (const empty of ["", "~", "null"]) {
    await withChain(async (root) => {
      mutateFile(
        root,
        TDD,
        "metadata:\n",
        `metadata:\n    artifact: ${empty}\n`,
      );
      assert.deepEqual(await found(root), [[EXPECTED, TDD]]);
      assert.deepEqual(await messages(root), [
        'skill "tdd" declares metadata.artifact [""] but owns no chain artifact; the field is absent, not empty, on a non-owner',
      ]);
    });
  }
});

test("TestSkillArtifactListFormAccepted", async () => {
  await withChain(async (root) => {
    declare(root, DECOMPOSE, "plan.md", "    artifact: [plan.md]");
    assert.deepEqual(await found(root), []);
  });
});

test("TestSkillArtifactBlockSequenceAccepted", async () => {
  await withChain(async (root) => {
    declare(root, DECOMPOSE, "plan.md", "    artifact:\n        - plan.md");
    assert.deepEqual(await found(root), []);
  });
});

test("TestSkillArtifactReadsMetadataNestingOnly", async () => {
  await withChain(async (root) => {
    mutateFile(
      root,
      TDD,
      "metadata:\n",
      "other:\n    artifact: tests.md\nmetadata:\n",
    );
    assert.deepEqual(await found(root), []);
  });
});

test("an artifact key under another key is not a declaration, so the owner gets the declared failure", async () => {
  await withChain(async (root) => {
    mutateFile(root, SPEC, "metadata:\n", "other:\n");
    assert.deepEqual(await found(root), [[DECLARED, SPEC]]);
  });
});

test("a metadata list or scalar is not a declaration", async () => {
  for (const meta of [
    "metadata:\n  - artifact: spec.md\n",
    "metadata: spec.md\n",
    "metadata:\n",
  ]) {
    await withChain(async (root) => {
      writeFile(root, SPEC, `---\nname: spec\n${meta}---\n\n# spec\n`);
      assert.deepEqual(await found(root), [[DECLARED, SPEC]], meta);
    });
  }
});

test("a metadata mapping with a non-string key skips the skill, which frontmatter-yaml reports", async () => {
  await withChain(async (root) => {
    mutateFile(root, SPEC, "metadata:\n", "metadata:\n    1: x\n");
    assert.deepEqual(await found(root), []);
  });
});

test("TestSkillArtifactListOwnership", async () => {
  const cases: Array<[string, string, string[][]]> = [
    [
      "wrong scalar",
      "plan.md",
      [
        [UNIQUE, DECOMPOSE],
        [EXPECTED, SPEC],
      ],
    ],
    [
      "duplicate kind",
      "[spec.md, spec.md]",
      [
        [EXPECTED, SPEC],
        [UNIQUE, SPEC],
      ],
    ],
    ["unknown kind", "[spec.md, unknown.md]", [[EXPECTED, SPEC]]],
    ["empty list", "[]", [[EXPECTED, SPEC]]],
  ];
  for (const [name, declaration, want] of cases) {
    await withChain(async (root) => {
      declare(root, SPEC, "spec.md", `    artifact: ${declaration}`);
      assert.deepEqual(await found(root), want, name);
    });
  }
});

test("a repeated list item is a unique failure at the same path and an empty list shows []", async () => {
  await withChain(async (root) => {
    declare(root, SPEC, "spec.md", "    artifact: [spec.md, spec.md]");
    assert.deepEqual(await messages(root), [
      'skill "spec" declares metadata.artifact ["spec.md","spec.md"]; the chain boundary design assigns it ["spec.md"]',
      `artifact "spec.md" is declared by 2 skills: ${SPEC}, ${SPEC}`,
    ]);
  });
  await withChain(async (root) => {
    declare(root, SPEC, "spec.md", "    artifact: []");
    assert.deepEqual(await messages(root), [
      'skill "spec" declares metadata.artifact []; the chain boundary design assigns it ["spec.md"]',
    ]);
  });
});

test("list items are sorted after rendering", async () => {
  await withChain(async (root) => {
    mutateFile(
      root,
      TDD,
      "metadata:\n",
      "metadata:\n    artifact: [b.md, a.md, 10.md, 9.md]\n",
    );
    assert.deepEqual(await messages(root), [
      'skill "tdd" declares metadata.artifact ["10.md","9.md","a.md","b.md"] but owns no chain artifact; the field is absent, not empty, on a non-owner',
    ]);
  });
});

test("declared values render as their resolved values", async () => {
  const vectors: Array<[string, string]> = [
    ["1.0", "1"],
    ["1e3", "1000"],
    ["1.5e7", "15000000"],
    ["0x10", "16"],
    ["0o17", "15"],
    ["1_000", "1_000"],
    ["-0", "0"],
    ["99999999999999999999", "100000000000000000000"],
    [".inf", "Infinity"],
    ["true", "true"],
    ["2026-01-01", "2026-01-01"],
    ["{a: b}", "{a: b}"],
    ["[[b]]", "[b]"],
    ["yes", "yes"],
  ];
  for (const [written, shown] of vectors) {
    await withChain(async (root) => {
      mutateFile(
        root,
        TDD,
        "metadata:\n",
        `metadata:\n    artifact: ${written}\n`,
      );
      assert.deepEqual(
        await messages(root),
        [
          `skill "tdd" declares metadata.artifact ["${shown}"] but owns no chain artifact; the field is absent, not empty, on a non-owner`,
        ],
        written,
      );
    });
  }
});

test("a duplicate artifact key skips the skill silently", async () => {
  await withChain(async (root) => {
    declare(
      root,
      SPEC,
      "spec.md",
      "    artifact: requirements.md\n    artifact: other.md",
    );
    assert.deepEqual(await found(root), []);
  });
});

test("frontmatter that is no mapping, or a leading byte order mark, skips the skill", async () => {
  await withChain(async (root) => {
    writeFile(root, SPEC, "---\n- a\n---\nname: spec\n");
    assert.deepEqual(await found(root), []);
  });
  await withChain(async (root) => {
    const bom = String.fromCodePoint(0xfeff);
    writeFile(
      root,
      SPEC,
      bom + skillMD("spec").replace(/ {4}artifact:.*\n/, ""),
    );
    assert.deepEqual(await found(root), []);
  });
});

test("a block with no closing line is parsed as the rest of the file", async () => {
  await withChain(async (root) => {
    writeFile(
      root,
      TDD,
      "---\nname: tdd\nmetadata:\n  artifact: tests.md\n\n# tdd\n",
    );
    assert.deepEqual(await found(root), [[EXPECTED, TDD]]);
  });
});

test("the name pattern crosses a newline: a bare name line captures the next line", async () => {
  await withChain(async (root) => {
    writeFile(
      root,
      TDD,
      "---\nname:\ndescription: d\nmetadata:\n  artifact: x.md\n---\n",
    );
    assert.deepEqual(await messages(root), [
      'skill "description: d" declares metadata.artifact ["x.md"] but owns no chain artifact; the field is absent, not empty, on a non-owner',
    ]);
  });
  await withChain(async (root) => {
    writeFile(
      root,
      SPEC,
      "---\nname:\n  spec\nmetadata:\n  artifact: x.md\n---\n",
    );
    assert.deepEqual(await messages(root), [
      'skill "spec" declares metadata.artifact ["x.md"]; the chain boundary design assigns it ["spec.md"]',
    ]);
  });
  await withChain(async (root) => {
    writeFile(root, SPEC, "name:\ndescription: d\n");
    assert.deepEqual(await found(root), []);
  });
});

test("quotes around the name are cut by cutset, so ownership still applies", async () => {
  for (const written of ["'spec'", "\"'spec'\"", '"spec"']) {
    await withChain(async (root) => {
      mutateFile(root, SPEC, "name: spec", `name: ${written}`);
      mutateFile(root, SPEC, "    artifact: spec.md\n", "");
      assert.deepEqual(await found(root), [[DECLARED, SPEC]], written);
      assert.deepEqual(await messages(root), [
        'chain skill "spec" owns ["spec.md"] but declares no metadata.artifact',
      ]);
    });
  }
});

test("a CRLF file is read with the name free of its carriage return", async () => {
  await withChain(async (root) => {
    writeFile(root, SPEC, skillMD("spec").replace(/\n/g, "\r\n"));
    assert.deepEqual(await found(root), []);
    writeFile(
      root,
      SPEC,
      skillMD("spec")
        .replace("    artifact: spec.md\n", "")
        .replace(/\n/g, "\r\n"),
    );
    assert.deepEqual(await messages(root), [
      'chain skill "spec" owns ["spec.md"] but declares no metadata.artifact',
    ]);
  });
});

test("two shared artifacts declared by the same two skills are ordered by artifact name", async () => {
  await withChain(async (root) => {
    addSkill(root, "git", { artifact: "[b.md, a.md]" });
    addSkill(root, "tdd", { artifact: "[b.md, a.md]" });
    const failures = await found(root);
    assert.deepEqual(failures, [
      [EXPECTED, "skills/git/SKILL.md"],
      [UNIQUE, "skills/git/SKILL.md"],
      [UNIQUE, "skills/git/SKILL.md"],
      [EXPECTED, TDD],
    ]);
    const shown = await messages(root);
    assert.equal(
      shown[1],
      'artifact "a.md" is declared by 2 skills: skills/git/SKILL.md, skills/tdd/SKILL.md',
    );
    assert.equal(
      shown[2],
      'artifact "b.md" is declared by 2 skills: skills/git/SKILL.md, skills/tdd/SKILL.md',
    );
  });
});

test("ownership follows the name value, not the directory", async () => {
  await withChain(async (root) => {
    writeFile(
      root,
      "skills/plan/SKILL.md",
      skillMD("plan").replace(
        "metadata:\n",
        "metadata:\n    artifact: plan.md\n",
      ),
    );
    assert.deepEqual(await found(root), [
      [UNIQUE, DECOMPOSE],
      [EXPECTED, "skills/plan/SKILL.md"],
    ]);
    assert.deepEqual(
      (await messages(root))[1],
      'skill "plan" declares metadata.artifact ["plan.md"] but owns no chain artifact; the field is absent, not empty, on a non-owner',
    );
  });
  await withChain(async (root) => {
    mutateFile(root, SPEC, "name: spec", "name: other");
    assert.deepEqual(await found(root), [[EXPECTED, SPEC]]);
  });
});

test("a skill with no name is skipped", async () => {
  await withChain(async (root) => {
    writeFile(root, SPEC, "---\nmetadata:\n  artifact: x.md\n---\n");
    assert.deepEqual(await found(root), []);
  });
});

test(
  "a skills root that is a link selects no skills",
  { skip: NO_LINKS },
  async () => {
    await withChain(async (root) => {
      const outside = mkdtempSync(
        path.join(tmpdir(), "agentics-artifacts-out-"),
      );
      try {
        writeFile(
          outside,
          "spec/SKILL.md",
          skillMD("spec").replace("spec.md", "wrong.md"),
        );
        remove(root, "skills");
        assert.equal(link(root, "skills", outside, "dir"), true);
        assert.deepEqual(await found(root), []);
      } finally {
        rmSync(outside, { recursive: true, force: true });
      }
    });
  },
);

test(
  "a skill directory that is a link selects no asset",
  { skip: NO_LINKS },
  async () => {
    await withChain(async (root) => {
      const outside = mkdtempSync(
        path.join(tmpdir(), "agentics-artifacts-out-"),
      );
      try {
        writeFile(
          outside,
          "SKILL.md",
          skillMD("spec").replace("spec.md", "wrong.md"),
        );
        remove(root, "skills/spec");
        assert.equal(link(root, "skills/spec", outside, "dir"), true);
        assert.deepEqual(await found(root), []);
      } finally {
        rmSync(outside, { recursive: true, force: true });
      }
    });
  },
);

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

test(
  "a SKILL.md that becomes a link after the asset listing is refused by the read",
  { skip: NO_LINKS },
  async () => {
    await withChain(async (root) => {
      const outside = mkdtempSync(
        path.join(tmpdir(), "agentics-artifacts-out-"),
      );
      try {
        writeFile(
          outside,
          "SKILL.md",
          skillMD("spec").replace("spec.md", "wrong.md"),
        );
        const file = path.join(root, "skills", "spec", "SKILL.md");
        // The asset listing inspects the file first; the link appears before the read inspects it again.
        let inspections = 0;
        const lstat = linkOnInspection(
          file,
          path.join(outside, "SKILL.md"),
          "file",
          () => (inspections += 1) === 2,
        );
        assert.ok(lstat, "the host cannot create a file symlink");
        const result = await withSeams({ lstat }, () => audit(root));
        assert.deepEqual(result.failures, []);
        assert.equal(
          result.error,
          `skills: reading ${SPEC}: ${SPEC}: passes through a link`,
        );
      } finally {
        rmSync(outside, { recursive: true, force: true });
      }
    });
  },
);

test("a lower-case skill.md is not selected", async () => {
  await withChain(async (root) => {
    remove(root, SPEC);
    writeFile(
      root,
      "skills/spec/skill.md",
      skillMD("spec").replace("spec.md", "wrong.md"),
    );
    assert.deepEqual(await found(root), []);
  });
});

test("a SKILL.md that is a directory is an audit error", async () => {
  await withChain(async (root) => {
    remove(root, SPEC);
    mkdir(root, SPEC);
    const result = await audit(root);
    assert.deepEqual(result.failures, []);
    assert.equal(
      result.error,
      "skills: reading skills/spec/SKILL.md: skills/spec/SKILL.md: is not a regular file",
    );
  });
});

test("empty items are not recorded, so two skills declaring none never collide", async () => {
  await withChain(async (root) => {
    addSkill(root, "git");
    for (const rel of [TDD, "skills/git/SKILL.md"])
      mutateFile(root, rel, "metadata:\n", "metadata:\n    artifact:\n");
    assert.deepEqual(await found(root), [
      [EXPECTED, "skills/git/SKILL.md"],
      [EXPECTED, TDD],
    ]);
  });
});

// --- audit error texts and causes, rows added by the mutation pass -----------------------------------------------------

test("a SKILL.md whose inspection fails is an audit error naming the file and why", async () => {
  await withChain(async (root) => {
    const target = path.join(root, "skills", "spec", "SKILL.md");
    const real = seams.lstat;
    const rows: [string, unknown, string][] = [
      [
        "a coded failure",
        Object.assign(new Error("EACCES: permission denied, lstat"), {
          code: "EACCES",
          syscall: "lstat",
          path: target,
        }),
        `${SPEC}: EACCES`,
      ],
      [
        "a failure with no code",
        new Error("odd failure"),
        `${SPEC}: unreadable`,
      ],
      [
        "a file error for another path",
        new RepoFileError("symlink", "somewhere"),
        "somewhere: passes through a link",
      ],
      ["no error object", null, `${SPEC}: unreadable`],
    ];
    for (const [label, error, text] of rows) {
      // The asset listing inspects the file first; the read inspects it again, and that one fails.
      let inspections = 0;
      const result = await withSeams(
        {
          lstat: (candidate) => {
            if (candidate === target && (inspections += 1) === 2) throw error;
            return real(candidate);
          },
        },
        () => audit(root),
      );
      assert.equal(result.error, `skills: reading ${SPEC}: ${text}`, label);
    }
  });
});

test("an audit error keeps the failure that caused it as its cause", async () => {
  await withChain(async (root) => {
    const unread = new Error("read failed");
    withSeams(
      {
        readFile: () => {
          throw unread;
        },
      },
      () =>
        assert.throws(
          () => FAMILY.run(root, { families: [], evalsCheck: () => [] }),
          (error: unknown) =>
            error instanceof Error &&
            error.cause instanceof RepoFileError &&
            error.cause.kind === "unreadable" &&
            error.cause.cause === unread,
        ),
    );
  });
});

// --- artifact declarations: every family on the green fixture, or the content slice on the pasted-format one ---------

const CONTENT_SLICE = sliceNamed("content");

const breakerOf = (rule: string): Mutation => {
  const found = BREAKERS[rule];
  assert.ok(found !== undefined, `no breaker for ${rule}`);
  return found;
};

const skillPath = (skill: string): string => `skills/${skill}/SKILL.md`;

/** The expected failure on a skill that owns no chain artifact but declares `shown`. */
const onNonOwner = (skill: string, shown: string): Failure => ({
  rule: EXPECTED,
  path: skillPath(skill),
  message: `skill "${skill}" declares metadata.artifact ${shown} but owns no chain artifact; the field is absent, not empty, on a non-owner`,
});

/** The expected failure on an owner whose declaration `shown` differs from its `assigned` artifact. */
const onOwner = (skill: string, shown: string, assigned: string): Failure => ({
  rule: EXPECTED,
  path: skillPath(skill),
  message: `skill "${skill}" declares metadata.artifact ${shown}; the chain boundary design assigns it ["${assigned}"]`,
});

/** The declared failure at `at` for the chain skill `owner`, which owns `artifact` and declares none. */
const undeclared = (at: string, owner: string, artifact: string): Failure => ({
  rule: DECLARED,
  path: at,
  message: `chain skill "${owner}" owns ["${artifact}"] but declares no metadata.artifact`,
});

/** The unique failure at the first of `paths`, which all declare `artifact`. */
const shared = (artifact: string, paths: readonly string[]): Failure => ({
  rule: UNIQUE,
  path: paths[0] ?? "",
  message: `artifact "${artifact}" is declared by ${paths.length} skills: ${paths.join(", ")}`,
});

test("skill-artifact-declared", async () => {
  assert.deepEqual(await auditEveryFamily(breakerOf(DECLARED)), {
    failures: [undeclared(DECOMPOSE, "decompose", "plan.md")],
  });
});

test("skill-artifact-expected", async () => {
  assert.deepEqual(await auditEveryFamily(breakerOf(EXPECTED)), {
    failures: [onNonOwner("prototype", '["notes.md"]')],
  });
});

test("skill-artifact-unique", async () => {
  assert.deepEqual(await auditEveryFamily(breakerOf(UNIQUE)), {
    failures: [
      shared("plan.md", [DECOMPOSE, skillPath("prototype")]),
      onNonOwner("prototype", '["plan.md"]'),
    ],
  });
});

test("a list naming spec.md twice on the spec owner", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      skillWithMetadata("spec", "    artifact: [spec.md, spec.md]\n"),
    ),
    {
      failures: [
        onOwner("spec", '["spec.md","spec.md"]', "spec.md"),
        shared("spec.md", [SPEC, SPEC]),
      ],
    },
  );
});

test("a mapping as the artifact of the decompose owner", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      skillWithMetadata("decompose", "    artifact:\n      a: b\n"),
    ),
    { failures: [onOwner("decompose", '["{a: b}"]', "plan.md")] },
  );
});

test("a non-string key under metadata on an owner", async () => {
  // The frontmatter reader refuses a non-string key, so the artifact family skips the skill; frontmatter-yaml, which
  // reports the refusal, is not a content slice family.
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      skillWithMetadata("intent", "    1: a\n    artifact: intent.md\n"),
    ),
    { failures: [] },
  );
});

test("a null artifact on an owner", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      skillWithMetadata("intent", "    artifact:\n"),
    ),
    { failures: [onOwner("intent", '[""]', "intent.md")] },
  );
});

test("a null artifact on a non-owner", async () => {
  assert.deepEqual(
    await auditSlice(CONTENT_SLICE, skillWithMetadata("x", "    artifact:\n")),
    { failures: [onNonOwner("x", '[""]')] },
  );
});

test("a tilde artifact on a non-owner", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      skillWithMetadata("x", "    artifact: ~\n"),
    ),
    { failures: [onNonOwner("x", '[""]')] },
  );
});

test("an empty string artifact on a non-owner", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      skillWithMetadata("x", '    artifact: ""\n'),
    ),
    { failures: [onNonOwner("x", '[""]')] },
  );
});

test("the artifact 1.5e7 on a non-owner", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      skillWithMetadata("x", "    artifact: 1.5e7\n"),
    ),
    { failures: [onNonOwner("x", '["15000000"]')] },
  );
});

test("the artifact 0x10 on a non-owner", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      skillWithMetadata("x", "    artifact: 0x10\n"),
    ),
    { failures: [onNonOwner("x", '["16"]')] },
  );
});

test("an unquoted date as the artifact of a non-owner", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      skillWithMetadata("x", "    artifact: 2026-01-02\n"),
    ),
    { failures: [onNonOwner("x", '["2026-01-02"]')] },
  );
});

test("the artifact true on a non-owner", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      skillWithMetadata("x", "    artifact: true\n"),
    ),
    { failures: [onNonOwner("x", '["true"]')] },
  );
});

test("the artifact 12 on the spec owner", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      skillWithMetadata("spec", "    artifact: 12\n"),
    ),
    { failures: [onOwner("spec", '["12"]', "spec.md")] },
  );
});

test("an unsorted list on a non-owner", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      skillWithMetadata("x", "    artifact: [b.md, a.md]\n"),
    ),
    { failures: [onNonOwner("x", '["a.md","b.md"]')] },
  );
});

test("an empty list on a non-owner", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      skillWithMetadata("x", "    artifact: []\n"),
    ),
    { failures: [onNonOwner("x", "[]")] },
  );
});

test("two shared artifacts declared by the same two skills", async () => {
  const both = [skillPath("x"), skillPath("y")];
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      all(
        skillWithMetadata("x", "    artifact: [a.md, b.md]\n"),
        skillWithMetadata("y", "    artifact: [a.md, b.md]\n"),
      ),
    ),
    {
      failures: [
        onNonOwner("x", '["a.md","b.md"]'),
        shared("a.md", both),
        shared("b.md", both),
        onNonOwner("y", '["a.md","b.md"]'),
      ],
    },
  );
});

test("an artifact shared by three skills", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      all(
        skillWithMetadata("x", "    artifact: shared.md\n"),
        skillWithMetadata("y", "    artifact: shared.md\n"),
        skillWithMetadata("z", "    artifact: shared.md\n"),
      ),
    ),
    {
      failures: [
        onNonOwner("x", '["shared.md"]'),
        shared("shared.md", ["x", "y", "z"].map(skillPath)),
        onNonOwner("y", '["shared.md"]'),
        onNonOwner("z", '["shared.md"]'),
      ],
    },
  );
});

test("a skill whose name sits on the line after name:", async () => {
  // The name is read across the line break, so skills/x/SKILL.md is the intent skill and declares no artifact.
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      xText('---\nname:\n  intent\ndescription: d\nversion: "1"\n---\n'),
    ),
    { failures: [undeclared("skills/x/SKILL.md", "intent", "intent.md")] },
  );
});

test("a skill whose directory differs from its name", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      skillWithMetadata("intent", "    surfaces: [claude-code]\n", "other"),
    ),
    { failures: [undeclared("skills/other/SKILL.md", "intent", "intent.md")] },
  );
});

test("a quoted owner name without an artifact", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      xText('---\nname: "intent"\ndescription: d\nversion: "1"\n---\n'),
    ),
    { failures: [undeclared("skills/x/SKILL.md", "intent", "intent.md")] },
  );
});
