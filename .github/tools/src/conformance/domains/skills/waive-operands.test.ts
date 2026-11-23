import assert from "node:assert/strict";
import { readFileSync, writeFileSync } from "node:fs";
import path from "node:path";
import { test, type TestContext } from "node:test";
import type { RunOptions } from "../../check.ts";
import {
  addSkill,
  link,
  remove,
  withSkillsRepo,
  writeFile,
} from "../../testing/skills-repo.ts";
import { run } from "./index.ts";
import { FAMILY, parseOperands } from "./waive-operands.ts";
import { withOutside } from "../../testing/host.ts";

const options: RunOptions = {
  short: false,
  fast: false,
  diagnostics: () => {},
  env: process.env,
};

const RULE = "flow-waive-operands";
const CONTRACT = "skills/execute/references/flow-state.md";
const EXECUTE = "skills/execute/SKILL.md";
const EMISSION = "skills/gate/references/emission.md";
const INTENT = "skills/intent/SKILL.md";

const audit = (root: string) => run(root, options, { families: [FAMILY] });

/** An Operands table whose rows list `quality-review`, `chain`, and the gate items `ci` and `tests`. */
const TABLE =
  "## Operands\n\n" +
  "A `waive` target is a head, then its arguments.\n\n" +
  "| Head | Arguments | Emitted in |\n" +
  "| --- | --- | --- |\n" +
  "| `quality-review` | 1 group | `skills/execute/SKILL.md` |\n" +
  "| `chain` | none | `skills/intent/SKILL.md` (also named in `skills/gate/references/missing.md`) |\n" +
  "| `ci`, `tests` | none; the head may carry `:<check>` | `skills/gate/references/emission.md` |\n\n" +
  "- A `demote` target is `wave-<id>`, as in `demote wave-2`.\n\n" +
  "## Next\n\n| `not-a-head` | x | `skills/none.md` |\n";

function append(root: string, rel: string, text: string): void {
  let before = "";
  try {
    before = readFileSync(path.join(root, rel), "utf8");
  } catch {
    // A file the fixture does not hold starts empty.
  }
  writeFile(root, rel, `${before}\n${text}`);
}

/** The green fixture plus a contract table and emitting files that satisfy it in both directions. */
async function withOperands<T>(body: (root: string) => Promise<T> | T) {
  return withSkillsRepo(async (root) => {
    append(root, CONTRACT, TABLE);
    append(
      root,
      EXECUTE,
      'Record `waive "quality-review <group>"` when the user skips the review, and `demote wave-<N>` to fold a wave.\n' +
        "The general form is `waive <what>`.\n",
    );
    addSkill(root, "intent", {
      body:
        "```bash\n" +
        'agx flow log intent waive chain ok "<reason>" none "next" --folder <work-folder>\n' +
        "```\n",
    });
    writeFile(
      root,
      EMISSION,
      "# Emission\n\nThe release items are `ci` and `tests`; write `waive ci:required-checks` for one check.\n",
    );
    return body(root);
  });
}

/** The (path, message) pairs of a run that ended without an audit error. */
async function found(root: string): Promise<string[][]> {
  const result = await audit(root);
  assert.equal(result.error, undefined);
  for (const one of result.failures) assert.equal(one.rule, RULE);
  return result.failures.map((one) => [one.path, one.message]);
}

const unlisted = (rel: string, line: number, head: string): string[] => [
  rel,
  `line ${line}: waive head "${head}" is not in the Operands table of ${CONTRACT}`,
];

/** The 1-based line of the first line in `rel` that contains `needle`. */
function lineOf(root: string, rel: string, needle: string): number {
  const lines = readFileSync(path.join(root, rel), "utf8").split("\n");
  const index = lines.findIndex((line) => line.includes(needle));
  assert.ok(index >= 0, `${rel} does not hold ${needle}`);
  return index + 1;
}

test("the family is named flow-waive-operands and reports its one rule", () => {
  assert.equal(FAMILY.name, "flow-waive-operands");
  assert.deepEqual(FAMILY.rules, [RULE]);
});

test("the table parser reads each backticked head with the first backticked path of its row", () => {
  assert.deepEqual(parseOperands(TABLE), [
    { head: "quality-review", emittedIn: "skills/execute/SKILL.md" },
    { head: "chain", emittedIn: "skills/intent/SKILL.md" },
    { head: "ci", emittedIn: "skills/gate/references/emission.md" },
    { head: "tests", emittedIn: "skills/gate/references/emission.md" },
  ]);
  assert.deepEqual(parseOperands("# Flow state\n\nNo table.\n"), []);
});

test("the green fixture, which writes no waive target, is clean", async () => {
  await withSkillsRepo(async (root) => {
    assert.deepEqual(await found(root), []);
  });
});

test("a fixture whose targets are all listed and whose heads are all emitted is clean", async () => {
  await withOperands(async (root) => {
    assert.deepEqual(await found(root), []);
  });
});

test("a backticked waive of an unlisted head fails", async () => {
  await withOperands(async (root) => {
    append(root, EXECUTE, "Then write `waive made-up-gate`.\n");
    const line = lineOf(root, EXECUTE, "made-up-gate");
    assert.deepEqual(await found(root), [
      unlisted(EXECUTE, line, "made-up-gate"),
    ]);
  });
});

test("an agx flow log command that waives an unlisted head fails, quoted or not", async () => {
  await withOperands(async (root) => {
    append(
      root,
      INTENT,
      "agx flow log intent waive invented ok none none next\n" +
        'Run `agx flow log intent waive "also-invented g1" ok none none next`.\n',
    );
    assert.deepEqual(await found(root), [
      unlisted(INTENT, lineOf(root, INTENT, "waive invented"), "invented"),
      unlisted(INTENT, lineOf(root, INTENT, "also-invented"), "also-invented"),
    ]);
  });
});

test("a demote target that is not a whole wave-<id> fails", async () => {
  await withOperands(async (root) => {
    append(
      root,
      EXECUTE,
      "Write `demote 2` to fold it.\n" +
        "Not `demote wave-2 extra` either.\n" +
        "Nor `demote my-wave-2`.\n",
    );
    const line = lineOf(root, EXECUTE, "demote 2");
    assert.deepEqual(await found(root), [
      [EXECUTE, `line ${line}: demote target "2" is not wave-<id>`],
      [
        EXECUTE,
        `line ${line + 1}: demote target "wave-2 extra" is not wave-<id>`,
      ],
      [EXECUTE, `line ${line + 2}: demote target "my-wave-2" is not wave-<id>`],
    ]);
  });
});

test("a demote target takes the writer's wave-<id> grammar", async () => {
  await withOperands(async (root) => {
    append(
      root,
      EXECUTE,
      "Write `demote wave-2` to fold it.\n" + "Not `demote wave-2/foo`.\n",
    );
    const line = lineOf(root, EXECUTE, "demote wave-2/foo");
    assert.deepEqual(await found(root), [
      [EXECUTE, `line ${line}: demote target "wave-2/foo" is not wave-<id>`],
    ]);
  });
});

test("a quoted target reads its first word as the head", async () => {
  await withOperands(async (root) => {
    writeFile(root, CONTRACT, TABLE.replace("`quality-review`", "`qr`"));
    append(root, EXECUTE, "The head `qr`.\n");
    const line = lineOf(root, EXECUTE, "quality-review <group>");
    assert.deepEqual(await found(root), [
      unlisted(EXECUTE, line, "quality-review"),
    ]);
  });
});

test("a placeholder head is skipped even when no table lists anything", async () => {
  await withSkillsRepo(async (root) => {
    append(
      root,
      EXECUTE,
      'Write `waive <what>`, `waive "<what> <arg>"`, or `demote <target>`.\n',
    );
    assert.deepEqual(await found(root), []);
  });
});

test("without the Operands table every explicit head fails", async () => {
  await withOperands(async (root) => {
    remove(root, CONTRACT);
    assert.deepEqual(await found(root), [
      unlisted(
        EXECUTE,
        lineOf(root, EXECUTE, "quality-review"),
        "quality-review",
      ),
      unlisted(EMISSION, 3, "ci"),
      unlisted(INTENT, lineOf(root, INTENT, "waive chain"), "chain"),
    ]);
  });
});

/** The finding for a listed head that its Emitted in file does not emit. */
const notEmitted = (rel: string, head: string): string[] => [
  rel,
  `head "${head}" is listed as emitted here but appears in no backticked span or agx flow log line`,
];

test("a listed head absent from its Emitted in file fails", async () => {
  await withOperands(async (root) => {
    writeFile(
      root,
      EMISSION,
      "# Emission\n\nThe release item is `ci`; write `waive ci:required-checks`.\n",
    );
    assert.deepEqual(await found(root), [notEmitted(EMISSION, "tests")]);
  });
});

test("a listed head that appears only in plain prose fails", async () => {
  await withOperands(async (root) => {
    writeFile(
      root,
      EMISSION,
      "# Emission\n\nThe release items are `ci` and tests; write `waive ci:required-checks`.\n",
    );
    assert.deepEqual(await found(root), [notEmitted(EMISSION, "tests")]);
  });
});

test("a backticked span counts the head only where no word character continues it", async () => {
  for (const span of [
    "all-tests",
    "tests-run",
    "tests_run",
    "_tests",
    "tests2",
    "Atests",
  ])
    await withOperands(async (root) => {
      writeFile(
        root,
        EMISSION,
        `# Emission\n\nThe release item is \`ci\`; not \`${span}\`.\n`,
      );
      assert.deepEqual(
        await found(root),
        [notEmitted(EMISSION, "tests")],
        span,
      );
    });
  await withOperands(async (root) => {
    writeFile(
      root,
      EMISSION,
      "# Emission\n\nThe items: `ci, tests:<check>`.\n",
    );
    assert.deepEqual(await found(root), []);
  });
});

test("a direction-1 finding sorts after the file-level finding in the same file", async () => {
  await withOperands(async (root) => {
    writeFile(
      root,
      EMISSION,
      "# Emission\n\nThe release item is `ci`; write `waive made-up`.\n",
    );
    assert.deepEqual(await found(root), [
      notEmitted(EMISSION, "tests"),
      unlisted(EMISSION, 3, "made-up"),
    ]);
  });
});

test("findings in one file sort by numeric line", async () => {
  await withOperands(async (root) => {
    const rel = "skills/gate/references/numbers.md";
    writeFile(
      root,
      rel,
      "\n".repeat(8) + "`waive nine`\n" + "\n".repeat(2) + "`waive twelve`\n",
    );
    assert.deepEqual(await found(root), [
      unlisted(rel, 9, "nine"),
      unlisted(rel, 12, "twelve"),
    ]);
  });
});

test("a listed head whose Emitted in file does not exist fails", async () => {
  await withOperands(async (root) => {
    remove(root, EMISSION);
    assert.deepEqual(await found(root), [
      [CONTRACT, `head "ci" is emitted in ${EMISSION}, which does not exist`],
      [
        CONTRACT,
        `head "tests" is emitted in ${EMISSION}, which does not exist`,
      ],
    ]);
  });
});

/** Replaces the fixture file `rel` with a link to `target`, or skips the calling test when the host refuses. */
function linkOrSkip(
  t: TestContext,
  root: string,
  rel: string,
  target: string,
): boolean {
  remove(root, rel);
  if (link(root, rel, target, "file")) return true;
  t.skip("this host cannot create a symlink");
  return false;
}

test("a contract file that is a link is a symlink file error and is not read", async (t) => {
  await withOperands((root) =>
    withOutside(async (outside) => {
      const target = path.join(outside, "flow-state.md");
      writeFileSync(target, "# Flow state\n\nNo table.\n");
      if (!linkOrSkip(t, root, CONTRACT, target)) return;
      assert.deepEqual(await audit(root), {
        failures: [],
        error: `${CONTRACT}: passes through a link`,
      });
    }),
  );
});

test("an Emitted in file that is a link is a symlink file error and is not read", async (t) => {
  await withOperands((root) =>
    withOutside(async (outside) => {
      const target = path.join(outside, "emission.md");
      writeFileSync(target, readFileSync(path.join(root, EMISSION), "utf8"));
      if (!linkOrSkip(t, root, EMISSION, target)) return;
      assert.deepEqual(await audit(root), {
        failures: [],
        error: `${EMISSION}: passes through a link`,
      });
    }),
  );
});

test("a linked skill file is not scanned for waive targets", async (t) => {
  await withOperands((root) =>
    withOutside(async (outside) => {
      const target = path.join(outside, "linked.md");
      writeFileSync(target, "Then write `waive made-up-gate`.\n");
      if (!linkOrSkip(t, root, "skills/gate/references/linked.md", target))
        return;
      assert.deepEqual(await found(root), []);
    }),
  );
});

test("a listed head whose Emitted in cell names no path fails", async () => {
  await withOperands(async (root) => {
    writeFile(
      root,
      CONTRACT,
      TABLE.replace(
        "`skills/intent/SKILL.md` (also named in `skills/gate/references/missing.md`)",
        "recorded by the chain",
      ),
    );
    assert.deepEqual(await found(root), [
      [CONTRACT, 'head "chain" names no Emitted in path'],
    ]);
  });
});

const ROW = "| `late` | none | `skills/late.md` |\n";

test("the parser reads only the first table inside the Operands section", () => {
  assert.deepEqual(
    parseOperands(
      "## Operands\n\nNo table here.\n\n## Next\n\n" +
        "| Head | Arguments | Emitted in |\n| --- | --- | --- |\n" +
        ROW,
    ),
    [],
  );
  assert.deepEqual(
    parseOperands(
      "## Operands\n\n" +
        "| Head | Arguments | Emitted in |\n| --- | --- | --- |\n" +
        "| `early` | none | `skills/early.md` |\n\nBetween the tables.\n\n" +
        "| Head | Arguments | Emitted in |\n| --- | --- | --- |\n" +
        ROW,
    ),
    [{ head: "early", emittedIn: "skills/early.md" }],
  );
});

test("the parser finds the Head and Emitted in columns by name", () => {
  assert.deepEqual(
    parseOperands(
      "## Operands\n\n" +
        "| Emitted in | Arguments | Head |\n| --- | --- | --- |\n" +
        "| `skills/a.md` then `skills/b.md` | `none` | `one`, `two` |\n",
    ),
    [
      { head: "one", emittedIn: "skills/a.md" },
      { head: "two", emittedIn: "skills/a.md" },
    ],
  );
});
