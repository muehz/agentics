import assert from "node:assert/strict";
import path from "node:path";
import { test, type TestContext } from "node:test";
import type { AuditResult, Failure, RunOptions } from "../../check.ts";
import {
  auditEveryFamily,
  breaker,
  staleBaseline,
} from "../../testing/skills-seeds.ts";
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
import { FAMILY, WORK_FOLDER_PINS, workFolderRules } from "./work-folder.ts";
import {
  linkDirectory,
  reportsNotADirectory,
  withOutside,
} from "../../testing/host.ts";

// The pin sentences and their messages are read from the exported table; the source-present message is written out.

const options: RunOptions = {
  short: false,
  fast: false,
  diagnostics: () => {},
  env: process.env,
};

const CONTRACT = "skills/decompose/references/chain-artifacts.md";
const GATE = "skills/gate/references/branch/tidy.md";
const PIN_RULE = "work-folder-pin";
const SOURCE_RULE = "work-folder-source-present";
const NOT_FOUND = "not found: the work-folder contract pins cannot be checked";

const NBSP = String.fromCodePoint(0x00a0);
const BOM = String.fromCodePoint(0xfeff);

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

const pinFailure = (index: number): string[] => {
  const pin = WORK_FOLDER_PINS[index];
  assert.ok(pin !== undefined);
  return [PIN_RULE, pin.rel, pin.message];
};

function skipForLink(t: TestContext): void {
  t.skip("this host cannot create a symlink");
}

test("the family declares its name and both rules", () => {
  assert.equal(FAMILY.name, "workfolder");
  assert.deepEqual(FAMILY.rules, [SOURCE_RULE, PIN_RULE]);
});

test("the pin table keeps all twelve pins in check order", () => {
  assert.equal(WORK_FOLDER_PINS.length, 12);
  assert.deepEqual(
    WORK_FOLDER_PINS.map((pin) => pin.rel),
    [
      CONTRACT,
      CONTRACT,
      CONTRACT,
      GATE,
      GATE,
      GATE,
      GATE,
      GATE,
      GATE,
      GATE,
      CONTRACT,
      CONTRACT,
    ],
  );
});

test("a green fixture has no finding", () =>
  withSkillsRepo(async (root) => {
    assert.deepEqual(await failuresOf(root), []);
  }));

test("each pin bites once, on its own file, with its own message", async () => {
  assert.equal(WORK_FOLDER_PINS.length, 12);
  for (const [index, pin] of WORK_FOLDER_PINS.entries()) {
    await withSkillsRepo(async (root) => {
      mutateFile(root, pin.rel, pin.literal, "");
      assert.deepEqual(
        brief(await failuresOf(root)),
        [pinFailure(index)],
        `pin ${index + 1}`,
      );
    });
  }
});

test("pin failures follow table order across the two files", () =>
  withSkillsRepo(async (root) => {
    // Pins 2 and 11 live in the contract file and pin 5 in the gate file: file order would give 2, 11, 5.
    for (const index of [1, 4, 10]) {
      const pin = WORK_FOLDER_PINS[index];
      assert.ok(pin !== undefined);
      mutateFile(root, pin.rel, pin.literal, "");
    }
    assert.deepEqual(brief(await failuresOf(root)), [1, 4, 10].map(pinFailure));
  }));

test("a missing file is reported once and its pins are not", () =>
  withSkillsRepo(async (root) => {
    remove(root, CONTRACT);
    assert.deepEqual(brief(await failuresOf(root)), [
      [SOURCE_RULE, CONTRACT, NOT_FOUND],
    ]);
  }));

test("two missing files are reported in the order their pins first appear", () =>
  withSkillsRepo(async (root) => {
    remove(root, GATE);
    remove(root, CONTRACT);
    assert.deepEqual(brief(await failuresOf(root)), [
      [SOURCE_RULE, CONTRACT, NOT_FOUND],
      [SOURCE_RULE, GATE, NOT_FOUND],
    ]);
  }));

test("source-present failures come before pin failures, whatever the table order", () =>
  withSkillsRepo(async (root) => {
    remove(root, GATE);
    const pin = WORK_FOLDER_PINS[0];
    assert.ok(pin !== undefined);
    mutateFile(root, CONTRACT, pin.literal, "");
    assert.deepEqual(brief(await failuresOf(root)), [
      [SOURCE_RULE, GATE, NOT_FOUND],
      pinFailure(0),
    ]);
  }));

test("a pin wrapped across lines still matches", () =>
  withSkillsRepo(async (root) => {
    for (const pin of WORK_FOLDER_PINS)
      mutateFile(
        root,
        pin.rel,
        pin.literal,
        pin.literal.replaceAll(" ", "\n  "),
      );
    assert.deepEqual(await failuresOf(root), []);
  }));

test("a no-break space folds like a space", () =>
  withSkillsRepo(async (root) => {
    const pin = WORK_FOLDER_PINS[4];
    assert.ok(pin !== undefined);
    mutateFile(root, pin.rel, pin.literal, pin.literal.replace(" ", NBSP));
    assert.deepEqual(await failuresOf(root), []);
  }));

test("a byte order mark inside a pin folds like a space", () =>
  withSkillsRepo(async (root) => {
    const pin = WORK_FOLDER_PINS[4];
    assert.ok(pin !== undefined);
    mutateFile(root, pin.rel, pin.literal, pin.literal.replace(" ", BOM));
    assert.deepEqual(await failuresOf(root), []);
  }));

test("a symlinked pin file is an audit error", (t) =>
  withSkillsRepo(async (root) => {
    remove(root, CONTRACT);
    if (!link(root, CONTRACT, path.join(root, GATE), "file"))
      return skipForLink(t);
    const message = await errorOf(root);
    assert.ok(
      message.startsWith(`skills: reading ${CONTRACT}: `),
      `unexpected message: ${message}`,
    );
    assert.ok(message.endsWith(": passes through a link"), message);
  }));

test("a symlinked ancestor of a pin file is an audit error", (t) =>
  withSkillsRepo((root) =>
    withOutside(async (outside) => {
      remove(root, "skills/decompose/references");
      if (
        linkDirectory(
          outside,
          path.join(root, "skills/decompose/references"),
        ) === undefined
      )
        return skipForLink(t);
      const message = await errorOf(root);
      assert.ok(
        message.startsWith(`skills: reading ${CONTRACT}: `),
        `unexpected message: ${message}`,
      );
      assert.ok(message.endsWith(": passes through a link"), message);
    }),
  ));

test("a pin file below a regular file is an audit error where the host says not a directory, and absent where it does not", () =>
  withSkillsRepo(async (root) => {
    remove(root, "skills/decompose/references");
    writeFile(root, "skills/decompose/references", "not a directory");
    if (reportsNotADirectory()) {
      const message = await errorOf(root);
      assert.ok(
        message.startsWith(`skills: reading ${CONTRACT}: `),
        `unexpected message: ${message}`,
      );
    } else {
      assert.deepEqual(brief(await failuresOf(root)), [
        [SOURCE_RULE, CONTRACT, NOT_FOUND],
      ]);
    }
  }));

test("a pin file that is a directory is an audit error", () =>
  withSkillsRepo(async (root) => {
    remove(root, GATE);
    mkdir(root, GATE);
    const message = await errorOf(root);
    assert.ok(
      message.startsWith(`skills: reading ${GATE}: `),
      `unexpected message: ${message}`,
    );
  }));

test("the audit error drops the failure found before it", () =>
  withSkillsRepo(async (root) => {
    remove(root, CONTRACT);
    remove(root, GATE);
    mkdir(root, GATE);
    // The contract file's source-present failure came first; run-level dropping returns no failure with the error.
    const result = await audit(root);
    assert.deepEqual(result.failures, []);
    assert.ok(result.error?.startsWith(`skills: reading ${GATE}: `));
  }));

// --- audit error texts and causes, rows added by the mutation pass -------------------------------------------------------

/** Runs the audit with the read of the first pinned file failing with `error`. */
function auditFailingRead(root: string, file: string, error: unknown) {
  const real = seams.readFile;
  return withSeams(
    {
      readFile: (candidate) => {
        if (candidate === file) throw error;
        return real(candidate);
      },
    },
    () => audit(root),
  );
}

const firstPinned = (): string => {
  const pin = WORK_FOLDER_PINS[0];
  assert.ok(pin !== undefined);
  return pin.rel;
};

test("a pinned file that cannot be read is an audit error naming it and why", async () => {
  await withSkillsRepo(async (root) => {
    const rel = firstPinned();
    const target = path.join(root, ...rel.split("/"));
    const rows: [string, unknown, string][] = [
      [
        "a coded failure",
        Object.assign(new Error("EACCES: permission denied, lstat"), {
          code: "EACCES",
          syscall: "lstat",
          path: target,
        }),
        `${rel}: EACCES`,
      ],
      [
        "a failure with no code",
        new Error("odd failure"),
        `${rel}: unreadable`,
      ],
      [
        "a file error for another path",
        new RepoFileError("symlink", "somewhere"),
        "somewhere: passes through a link",
      ],
      ["no error object", null, `${rel}: unreadable`],
    ];
    for (const [label, error, text] of rows) {
      const result = await auditFailingRead(root, target, error);
      assert.deepEqual(
        result,
        { failures: [], error: `skills: reading ${rel}: ${text}` },
        label,
      );
    }
  });
});

test("a pinned file whose read fails as not existing is a source-present failure, from a code or a file error", async () => {
  await withSkillsRepo(async (root) => {
    const rel = firstPinned();
    const target = path.join(root, ...rel.split("/"));
    const gone = (): Error =>
      Object.assign(new Error("gone"), { code: "ENOENT" });
    for (const [label, error] of [
      ["the not-exist code", gone()],
      ["a missing file error", new RepoFileError("missing", "elsewhere")],
    ] as const) {
      const result = await auditFailingRead(root, target, error);
      assert.equal(result.error, undefined, label);
      assert.deepEqual(
        brief(result.failures).filter((one) => one[0] === SOURCE_RULE),
        [[SOURCE_RULE, rel, NOT_FOUND]],
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
          () => workFolderRules(root),
          (error: unknown) =>
            error instanceof Error &&
            error.cause instanceof RepoFileError &&
            error.cause.cause === unread,
        ),
    );
  });
});

// --- the whole-domain scenarios ------------------------------------------------------------------------------------

test("work-folder-pin", async () => {
  assert.deepEqual(await auditEveryFamily(breaker("work-folder-pin")), {
    failures: [
      {
        rule: PIN_RULE,
        path: GATE,
        message: "tidy.md must state that a name collision keeps both copies",
      },
    ],
  });
});

test("work-folder-source-present", async () => {
  // The baseline accepted a skill-xref-mode finding in the removed file, so that entry is now stale.
  assert.deepEqual(
    await auditEveryFamily(breaker("work-folder-source-present")),
    {
      failures: [
        {
          rule: SOURCE_RULE,
          path: CONTRACT,
          message: NOT_FOUND,
        },
        staleBaseline(
          "decompose",
          "skill-xref-mode",
          "skills/decompose/references/chain-artifacts.md",
          "/agentics:gate branch",
        ),
      ],
    },
  );
});
