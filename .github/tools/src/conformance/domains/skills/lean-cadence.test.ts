import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import path from "node:path";
import { test, type TestContext } from "node:test";
import type { AuditResult, Failure, RunOptions } from "../../check.ts";
import { auditEveryFamily, breaker, put } from "../../testing/skills-seeds.ts";
import {
  link,
  mkdir,
  mutateFile,
  remove,
  withSkillsRepo,
  writeFile,
} from "../../testing/skills-repo.ts";
import { RepoFileError } from "../../repo-file-error.ts";
import { seams, withSeams } from "./files.ts";
import { run } from "./index.ts";
import {
  FAMILY,
  LEAN_CADENCE_PINS,
  leanCadenceMarkdown,
  leanCadenceRules,
  type LeanCadencePin,
} from "./lean-cadence.ts";
import { linkDirectory, withOutside } from "../../testing/host.ts";

// The pin literals are read from the exported table; the messages are written out.

const options: RunOptions = {
  short: false,
  fast: false,
  diagnostics: () => {},
  env: process.env,
};

const PINS_RULE = "lean-cadence-pins";
const SOURCE_RULE = "lean-cadence-source-present";
const NOT_FOUND = "not found: the lean-cadence pins cannot be checked";
const EXECUTE = "skills/execute";
const EXECUTE_SKILL = "skills/execute/SKILL.md";
const STAGE_1 = "skills/execute/references/stage-1-dispatch-rule.md";
const FLOW_STATE = "skills/execute/references/flow-state.md";
const PLAN_TEMPLATE = "skills/decompose/references/plan-template.md";
const GH_SKILL = "skills/gh/SKILL.md";
const GH_STACK = "skills/gh/references/stack.md";

const audit = (root: string): Promise<AuditResult> =>
  run(root, options, { families: [FAMILY] });

async function failuresOf(root: string): Promise<Failure[]> {
  const result = await audit(root);
  assert.equal(result.error, undefined);
  return result.failures;
}

async function errorOf(root: string): Promise<string> {
  const result = await audit(root);
  assert.deepEqual(result.failures, []);
  assert.notEqual(result.error, undefined, "expected an audit error");
  return result.error ?? "";
}

const brief = (found: Failure[]): string[][] =>
  found.map((one) => [one.rule, one.path, one.message]);

const retired = (rel: string, literal: string): string[] => [
  PINS_RULE,
  rel,
  `retired text present: ${literal}`,
];

function pinAt(index: number): LeanCadencePin {
  const pin = LEAN_CADENCE_PINS[index];
  assert.ok(pin !== undefined);
  return pin;
}

const presentPins = LEAN_CADENCE_PINS.filter((pin) => pin.absent !== true);
const absentPins = LEAN_CADENCE_PINS.filter((pin) => pin.absent === true);
const directoryPins = LEAN_CADENCE_PINS.filter((pin) => pin.dir === true);

/** The distinct files the file pins name, in table order. */
const pinnedFiles = [
  ...new Set(
    LEAN_CADENCE_PINS.filter((pin) => pin.dir !== true).map((pin) => pin.rel),
  ),
];

/** The text of `rel` with its first line break replaced by `insert` and a line break, as an injected retired phrase. */
function plantAtFirstLineBreak(
  root: string,
  rel: string,
  insert: string,
): void {
  const target = path.join(root, ...rel.split("/"));
  const text = readFileSync(target, "utf8");
  const lineBreak = text.indexOf("\n");
  assert.ok(lineBreak >= 0, `${rel} has no line break`);
  writeFile(
    root,
    rel,
    `${text.slice(0, lineBreak)} ${insert}${text.slice(lineBreak)}`,
  );
}

function skipForLink(t: TestContext): void {
  t.skip("this host cannot create a symlink");
}

test("the family declares its name and both rules", () => {
  assert.equal(FAMILY.name, "lean-cadence");
  assert.deepEqual(FAMILY.rules, [PINS_RULE, SOURCE_RULE]);
});

test("the pin table keeps twelve pins: five present and seven absent, three of them directory pins", () => {
  assert.equal(LEAN_CADENCE_PINS.length, 12);
  assert.equal(presentPins.length, 5);
  assert.equal(absentPins.length, 7);
  assert.equal(directoryPins.length, 3);
  assert.deepEqual(
    LEAN_CADENCE_PINS.map((pin) => pin.absent === true),
    [
      false,
      false,
      false,
      false,
      false,
      true,
      true,
      true,
      true,
      true,
      true,
      true,
    ],
  );
  assert.equal(pinnedFiles.length, 6);
});

test("a green fixture has no finding", () =>
  withSkillsRepo(async (root) => {
    assert.deepEqual(await failuresOf(root), []);
  }));

test("each present pin bites once, on its own file", async () => {
  assert.equal(presentPins.length, 5);
  for (const pin of presentPins) {
    await withSkillsRepo(async (root) => {
      mutateFile(root, pin.rel, pin.literal, "");
      assert.deepEqual(
        brief(await failuresOf(root)),
        [[PINS_RULE, pin.rel, `missing: ${pin.literal}`]],
        pin.literal,
      );
    });
  }
});

test("each absent pin bites once, on the file that carries the retired text", async () => {
  assert.equal(absentPins.length, 7);
  for (const pin of absentPins) {
    await withSkillsRepo(async (root) => {
      // A directory pin is planted in the execute skill file, which the directory scan reads.
      const rel = pin.dir === true ? EXECUTE_SKILL : pin.rel;
      plantAtFirstLineBreak(root, rel, pin.literal);
      assert.deepEqual(
        brief(await failuresOf(root)),
        [retired(rel, pin.literal)],
        pin.literal,
      );
    });
  }
});

test("a present pin wrapped across lines still matches", () =>
  withSkillsRepo(async (root) => {
    mutateFile(
      root,
      PLAN_TEMPLATE,
      "**Packet review:**",
      "**Packet\nreview:**",
    );
    assert.deepEqual(await failuresOf(root), []);
  }));

test("table rows padded with runs of spaces still match", () =>
  withSkillsRepo(async (root) => {
    mutateFile(
      root,
      FLOW_STATE,
      "| `review` | a reviewer returned",
      "|   `review`   |   a reviewer     returned",
    );
    mutateFile(
      root,
      FLOW_STATE,
      "| `escape` | a Critical or Important defect that passed a review",
      "| `escape`  |  a Critical or Important\tdefect that passed a  review",
    );
    assert.deepEqual(await failuresOf(root), []);
  }));

test("a retired phrase wrapped across lines still fires once", () =>
  withSkillsRepo(async (root) => {
    writeFile(root, GH_STACK, "Only ever propose\nto convert a stack.\n");
    assert.deepEqual(brief(await failuresOf(root)), [
      retired(GH_STACK, "Only ever propose to convert"),
    ]);
  }));

test("each directory phrase, one word per line in a nested file, fires once on that file", async () => {
  assert.equal(directoryPins.length, 3);
  const nested = "skills/execute/references/nested/extra.md";
  for (const pin of directoryPins) {
    await withSkillsRepo(async (root) => {
      writeFile(root, nested, `Notes.\n${pin.literal.replaceAll(" ", "\n")}\n`);
      assert.deepEqual(
        brief(await failuresOf(root)),
        [retired(nested, pin.literal)],
        pin.literal,
      );
    });
  }
});

test("the directory scan checks the suffix case-sensitively", async () => {
  const phrase = pinAt(7).literal;
  for (const name of ["extra.txt", "extra.MD"]) {
    await withSkillsRepo(async (root) => {
      writeFile(root, `skills/execute/references/${name}`, `${phrase}\n`);
      assert.deepEqual(await failuresOf(root), [], name);
    });
  }
});

test("a file named exactly .md is scanned", () =>
  withSkillsRepo(async (root) => {
    const phrase = pinAt(7).literal;
    const bare = "skills/execute/references/.md";
    writeFile(root, bare, `${phrase}\n`);
    assert.deepEqual(brief(await failuresOf(root)), [retired(bare, phrase)]);
  }));

test("each pinned file, deleted in turn, is one source-present failure and no error", async () => {
  assert.equal(pinnedFiles.length, 6);
  for (const rel of pinnedFiles) {
    await withSkillsRepo(async (root) => {
      remove(root, rel);
      assert.deepEqual(
        brief(await failuresOf(root)),
        [[SOURCE_RULE, rel, NOT_FOUND]],
        rel,
      );
    });
  }
});

test("a missing execute skill file is reported before the directory scan, which still reads the other files", () =>
  withSkillsRepo(async (root) => {
    remove(root, EXECUTE_SKILL);
    const phrase = pinAt(7).literal;
    const nested = "skills/execute/references/nested/extra.md";
    writeFile(root, nested, `${phrase}\n`);
    assert.deepEqual(brief(await failuresOf(root)), [
      [SOURCE_RULE, EXECUTE_SKILL, NOT_FOUND],
      retired(nested, phrase),
    ]);
  }));

test("a missing directory is reported once, in the order each source is first met", () =>
  withSkillsRepo(async (root) => {
    remove(root, EXECUTE);
    assert.deepEqual(brief(await failuresOf(root)), [
      [SOURCE_RULE, STAGE_1, NOT_FOUND],
      [SOURCE_RULE, EXECUTE_SKILL, NOT_FOUND],
      [SOURCE_RULE, FLOW_STATE, NOT_FOUND],
      [SOURCE_RULE, EXECUTE, NOT_FOUND],
    ]);
  }));

test("pin failures follow table order, whichever file or directory carries them", () =>
  withSkillsRepo(async (root) => {
    const first = pinAt(0);
    mutateFile(root, first.rel, first.literal, "");
    const nested = "skills/execute/references/nested/extra.md";
    const phrase = pinAt(7).literal;
    writeFile(root, nested, `${phrase}\n`);
    assert.deepEqual(brief(await failuresOf(root)), [
      [PINS_RULE, first.rel, `missing: ${first.literal}`],
      retired(nested, phrase),
    ]);
  }));

test("the directory scan sorts by full path, not by walk order", () =>
  withSkillsRepo(async (root) => {
    const phrase = pinAt(7).literal;
    writeFile(root, "skills/execute/a-b.md", `${phrase}\n`);
    writeFile(root, "skills/execute/a/z.md", `${phrase}\n`);
    // `-` sorts before `/`, so a-b.md comes first; a directory-first walk would visit a/z.md first.
    assert.deepEqual(brief(await failuresOf(root)), [
      retired("skills/execute/a-b.md", phrase),
      retired("skills/execute/a/z.md", phrase),
    ]);
    assert.deepEqual(leanCadenceMarkdown(root, EXECUTE), [
      "skills/execute/SKILL.md",
      "skills/execute/a-b.md",
      "skills/execute/a/z.md",
      "skills/execute/references/fix-entry.md",
      "skills/execute/references/flow-state.md",
      "skills/execute/references/stage-1-dispatch-rule.md",
    ]);
  }));

test("the directory scan orders names by the default sort, by UTF-16 code units", () =>
  withSkillsRepo((root) => {
    // U+1F600 is a surrogate pair, whose first unit sorts below the one unit of U+FF5E.
    const wide = `${String.fromCodePoint(0xff5e)}.md`;
    const astral = `${String.fromCodePoint(0x1f600)}.md`;
    writeFile(root, `skills/execute/${astral}`, "x\n");
    writeFile(root, `skills/execute/${wide}`, "x\n");
    const listed = leanCadenceMarkdown(root, EXECUTE);
    assert.ok(
      listed.indexOf(`skills/execute/${astral}`) <
        listed.indexOf(`skills/execute/${wide}`),
      listed.join("\n"),
    );
  }));

test("a file pin that is a directory is an audit error", () =>
  withSkillsRepo(async (root) => {
    remove(root, GH_STACK);
    mkdir(root, GH_STACK);
    const message = await errorOf(root);
    assert.ok(
      message.startsWith(`skills: reading ${GH_STACK}: `),
      `unexpected message: ${message}`,
    );
  }));

test("a symlinked execute directory is an audit error at the first file pin under it", (t) =>
  withSkillsRepo((root) =>
    withOutside(async (outside) => {
      remove(root, EXECUTE);
      if (linkDirectory(outside, path.join(root, EXECUTE)) === undefined)
        return skipForLink(t);
      const message = await errorOf(root);
      assert.equal(
        message,
        `skills: reading ${EXECUTE}/references/stage-1-dispatch-rule.md: ${EXECUTE}: passes through a link`,
      );
    }),
  ));

test("a link below the execute directory is an audit error from the scan, and drops the earlier failure", async (t) => {
  const cases: Array<{
    name: string;
    plant: (root: string, outside: string) => boolean;
  }> = [
    {
      name: "a directory link",
      plant: (root, outside) =>
        linkDirectory(
          outside,
          path.join(root, "skills/execute/references/linked"),
        ) !== undefined,
    },
    {
      name: "a file link named .md",
      plant: (root) =>
        link(
          root,
          "skills/execute/references/extra.md",
          path.join(root, ...EXECUTE_SKILL.split("/")),
          "file",
        ),
    },
    {
      name: "a file link named .txt",
      plant: (root) =>
        link(
          root,
          "skills/execute/x.txt",
          path.join(root, ...EXECUTE_SKILL.split("/")),
          "file",
        ),
    },
  ];
  for (const one of cases) {
    await t.test(one.name, (st) =>
      withSkillsRepo((root) =>
        withOutside(async (outside) => {
          // The first pin's file is missing, which the family finds before it reaches the scan.
          remove(root, PLAN_TEMPLATE);
          if (!one.plant(root, outside)) return skipForLink(st);
          const message = await errorOf(root);
          assert.ok(
            message.startsWith(`skills: scanning ${EXECUTE}: `),
            `unexpected message: ${message}`,
          );
          assert.ok(message.endsWith(": passes through a link"), message);
          assert.throws(
            () => leanCadenceRules(root),
            (error: unknown) =>
              error instanceof Error &&
              error.message.startsWith(`skills: scanning ${EXECUTE}: `),
          );
        }),
      ),
    );
  }
});

test("leanCadenceMarkdown refuses a linked root and reports a missing one as missing", (t) =>
  withSkillsRepo((root) =>
    withOutside((outside) => {
      assert.throws(
        () => leanCadenceMarkdown(root, "skills/absent"),
        (error: unknown) =>
          error instanceof RepoFileError &&
          error.kind === "missing" &&
          error.path === "skills/absent",
      );
      remove(root, EXECUTE);
      if (linkDirectory(outside, path.join(root, EXECUTE)) === undefined)
        return skipForLink(t);
      assert.throws(
        () => leanCadenceMarkdown(root, EXECUTE),
        (error: unknown) =>
          error instanceof RepoFileError &&
          error.kind === "symlink" &&
          error.message === `${EXECUTE}: passes through a link`,
      );
    }),
  ));

// --- audit error texts and causes, rows added by the mutation pass -------------------------------------------------------

/** The inspection and read errors a file-system call can raise, with the reason the audit error gives each. */
function failureRows(
  target: string,
  shown: string,
): [string, unknown, string][] {
  return [
    [
      "a coded failure",
      Object.assign(new Error("EACCES: permission denied"), {
        code: "EACCES",
        path: target,
      }),
      `${shown}: EACCES`,
    ],
    [
      "a failure with no code",
      new Error("odd failure"),
      `${shown}: unreadable`,
    ],
    [
      "a file error for another path",
      new RepoFileError("symlink", "somewhere"),
      "somewhere: passes through a link",
    ],
    ["no error object", null, `${shown}: unreadable`],
  ];
}

const calledFrom = (name: string): boolean =>
  new Error("probe").stack?.includes(`at ${name} (`) ?? false;

/** Runs the audit with the read of `target` failing with `error`. */
function auditFailingRead(root: string, target: string, error: unknown) {
  const real = seams.readFile;
  return withSeams(
    {
      readFile: (candidate) => {
        if (candidate === target) throw error;
        return real(candidate);
      },
    },
    () => audit(root),
  );
}

/** Runs the audit with the inspection of the directory scan's root failing with `error`. */
function auditFailingScan(root: string, target: string, error: unknown) {
  const real = seams.lstat;
  return withSeams(
    {
      lstat: (candidate) => {
        if (candidate === target && calledFrom("leanCadenceMarkdown"))
          throw error;
        return real(candidate);
      },
    },
    () => audit(root),
  );
}

const gone = (): Error => Object.assign(new Error("gone"), { code: "ENOENT" });

test("a pinned file that cannot be read is an audit error naming it and why", async () => {
  await withSkillsRepo(async (root) => {
    const target = path.join(root, ...GH_SKILL.split("/"));
    for (const [label, error, text] of failureRows(target, GH_SKILL)) {
      const result = await auditFailingRead(root, target, error);
      assert.deepEqual(
        result,
        {
          failures: [],
          error: `skills: reading ${GH_SKILL}: ${text}`,
        },
        label,
      );
    }
  });
});

test("a pinned directory that cannot be scanned is an audit error naming it and why", async () => {
  await withSkillsRepo(async (root) => {
    const target = path.join(root, "skills", "execute");
    for (const [label, error, text] of failureRows(target, EXECUTE)) {
      const result = await auditFailingScan(root, target, error);
      assert.deepEqual(
        result,
        {
          failures: [],
          error: `skills: scanning ${EXECUTE}: ${text}`,
        },
        label,
      );
    }
  });
});

test("a pinned file or directory whose failure is missing is a source failure, from a code or a file error", async () => {
  await withSkillsRepo(async (root) => {
    const file = path.join(root, ...GH_SKILL.split("/"));
    const dir = path.join(root, "skills", "execute");
    for (const [label, error] of [
      ["the not-exist code", gone()],
      ["a missing file error", new RepoFileError("missing", "elsewhere")],
    ] as const) {
      const read = await auditFailingRead(root, file, error);
      assert.equal(read.error, undefined, label);
      assert.deepEqual(
        brief(read.failures).filter((one) => one[1] === GH_SKILL),
        [[SOURCE_RULE, GH_SKILL, NOT_FOUND]],
        label,
      );
      const scan = await auditFailingScan(root, dir, error);
      assert.equal(scan.error, undefined, label);
      assert.deepEqual(
        brief(scan.failures).filter((one) => one[1] === EXECUTE),
        [[SOURCE_RULE, EXECUTE, NOT_FOUND]],
        label,
      );
    }
  });
});

test("an audit error keeps the failure that caused it as its cause", async () => {
  await withSkillsRepo(async (root) => {
    const unread = new Error("read failed");
    withSeams(
      {
        readFile: () => {
          throw unread;
        },
      },
      () =>
        assert.throws(
          () => leanCadenceRules(root),
          (error: unknown) =>
            error instanceof Error &&
            error.cause instanceof RepoFileError &&
            error.cause.cause === unread,
          "a read",
        ),
    );
    const denied = Object.assign(new Error("denied"), { code: "EACCES" });
    const real = seams.lstat;
    withSeams(
      {
        lstat: (candidate) => {
          if (calledFrom("leanCadenceMarkdown")) throw denied;
          return real(candidate);
        },
      },
      () =>
        assert.throws(
          () => leanCadenceRules(root),
          (error: unknown) =>
            error instanceof Error &&
            error.cause instanceof RepoFileError &&
            error.cause.cause === denied,
          "a scan",
        ),
    );
  });
});

// --- the whole-domain scenarios ------------------------------------------------------------------------------------

test("lean-cadence-pins", async () => {
  assert.deepEqual(await auditEveryFamily(breaker("lean-cadence-pins")), {
    failures: [
      {
        rule: PINS_RULE,
        path: PLAN_TEMPLATE,
        message: "missing: **Packet review:**",
      },
    ],
  });
});

test("retired lean-cadence text wrapped across lines in a nested execute file", async () => {
  assert.deepEqual(
    await auditEveryFamily(
      put(
        "skills/execute/references/nested/x.md",
        "The review runs at every\nwave boundary.\n",
      ),
    ),
    {
      failures: [
        {
          rule: PINS_RULE,
          path: "skills/execute/references/nested/x.md",
          message: "retired text present: runs at every wave boundary",
        },
      ],
    },
  );
});

test("lean-cadence-source-present", async () => {
  assert.deepEqual(
    await auditEveryFamily(breaker("lean-cadence-source-present")),
    {
      failures: [
        {
          rule: SOURCE_RULE,
          path: GH_STACK,
          message: NOT_FOUND,
        },
      ],
    },
  );
});
