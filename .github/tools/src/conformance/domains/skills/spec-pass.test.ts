import assert from "node:assert/strict";
import path from "node:path";
import { test, type TestContext } from "node:test";
import type { AuditResult, Failure, RunOptions } from "../../check.ts";
import { auditEveryFamily, breaker } from "../../testing/skills-seeds.ts";
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
import { FAMILY, SPEC_PASS_PINS, specPassRules } from "./spec-pass.ts";
import { linkDirectory, withOutside } from "../../testing/host.ts";

// The messages are written out; each pin sentence is read from the exported table.

const options: RunOptions = {
  short: false,
  fast: false,
  diagnostics: () => {},
  env: process.env,
};

const SPEC_AXIS = "skills/review/references/axes/spec-axis.md";
const AXES_MODE = "skills/review/references/axes/axes-mode.md";
const BRANCH = "skills/gate/references/branch/review-and-audit.md";
const AXES_DIR = "skills/review/references/axes";
const RULE = "spec-pass-pinned";
const CONTRACT_TAIL = ", which carries the spec-pass contract: ";

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

/**
 * The missing-sentence message, with the pin in double quotes. No pin holds a quote, a backslash or a non-ASCII
 * character.
 */
const carriesNot = (rel: string, literal: string): string =>
  `${rel} no longer carries the pinned sentence "${literal}"`;

const absolute = (root: string, rel: string): string =>
  path.join(root, ...rel.split("/"));

function skipForLink(t: TestContext): void {
  t.skip("this host cannot create a symlink");
}

function pinAt(index: number): { rel: string; literal: string } {
  const pin = SPEC_PASS_PINS[index];
  assert.ok(pin !== undefined);
  return pin;
}

test("the family declares its name and its one rule", () => {
  assert.equal(FAMILY.name, "spec-pass");
  assert.deepEqual(FAMILY.rules, [RULE]);
});

test("the pin table keeps all seven pins in check order", () => {
  assert.equal(SPEC_PASS_PINS.length, 7);
  assert.deepEqual(
    SPEC_PASS_PINS.map((pin) => pin.rel),
    [SPEC_AXIS, SPEC_AXIS, AXES_MODE, AXES_MODE, AXES_MODE, BRANCH, BRANCH],
  );
});

test("a green fixture has no finding", () =>
  withSkillsRepo(async (root) => {
    assert.deepEqual(await failuresOf(root), []);
  }));

test("each pin bites once, on its own file", async () => {
  assert.equal(SPEC_PASS_PINS.length, 7);
  for (const [index, pin] of SPEC_PASS_PINS.entries()) {
    await withSkillsRepo(async (root) => {
      mutateFile(root, pin.rel, pin.literal, "");
      assert.deepEqual(
        brief(await failuresOf(root)),
        [[RULE, pin.rel, carriesNot(pin.rel, pin.literal)]],
        `pin ${index + 1}`,
      );
    });
  }
});

test("the pin and the file are both whitespace-folded", () =>
  withSkillsRepo(async (root) => {
    const pin = pinAt(3);
    mutateFile(
      root,
      pin.rel,
      pin.literal,
      "  End the   aggregate\nwith exactly one\n   `Lanes:`   line  ",
    );
    assert.deepEqual(await failuresOf(root), []);
  }));

test("a missing file is reported once and its other pins are not", () =>
  withSkillsRepo(async (root) => {
    remove(root, AXES_MODE);
    const found = await failuresOf(root);
    assert.equal(found.length, 1);
    assert.equal(found[0]?.rule, RULE);
    assert.equal(found[0]?.path, AXES_MODE);
    assert.ok(
      found[0]?.message.startsWith(`cannot read ${AXES_MODE}${CONTRACT_TAIL}`),
      found[0]?.message,
    );
  }));

test("a missing file leaves the pins of the other files checked", () =>
  withSkillsRepo(async (root) => {
    remove(root, AXES_MODE);
    const spec = pinAt(0);
    const branch = pinAt(6);
    mutateFile(root, spec.rel, spec.literal, "");
    mutateFile(root, branch.rel, branch.literal, "");
    const found = await failuresOf(root);
    assert.deepEqual(
      found.map((one) => [one.rule, one.path]),
      [
        [RULE, SPEC_AXIS],
        [RULE, AXES_MODE],
        [RULE, BRANCH],
      ],
    );
    assert.equal(found[0]?.message, carriesNot(spec.rel, spec.literal));
    assert.equal(found[2]?.message, carriesNot(branch.rel, branch.literal));
  }));

test("the missing-file message embeds the error naming the first missing component", () =>
  withSkillsRepo(async (root) => {
    remove(root, AXES_DIR);
    const found = await failuresOf(root);
    assert.deepEqual(
      found.map((one) => [one.rule, one.path]),
      [
        [RULE, SPEC_AXIS],
        [RULE, AXES_MODE],
      ],
    );
    // The error names the first component that does not exist, here the axes directory.
    for (const one of found)
      assert.equal(
        one.message,
        `cannot read ${one.path}${CONTRACT_TAIL}${AXES_DIR}: does not exist`,
      );
  }));

test("a file whose whole content is the internal placeholder is skipped", () =>
  withSkillsRepo(async (root) => {
    writeFile(root, AXES_MODE, "\0unreadable");
    assert.deepEqual(await failuresOf(root), []);
  }));

test("the placeholder skip is an exact match on the whole content", () =>
  withSkillsRepo(async (root) => {
    writeFile(root, AXES_MODE, "\0unreadable\n");
    assert.deepEqual(
      brief(await failuresOf(root)),
      [2, 3, 4].map((index) => {
        const pin = pinAt(index);
        return [RULE, pin.rel, carriesNot(pin.rel, pin.literal)];
      }),
    );
  }));

test("a pin file that is a directory is an audit error with no skills prefix", () =>
  withSkillsRepo(async (root) => {
    remove(root, AXES_MODE);
    mkdir(root, AXES_MODE);
    const message = await errorOf(root);
    assert.ok(
      message.startsWith(`cannot read ${AXES_MODE}${CONTRACT_TAIL}`),
      `unexpected message: ${message}`,
    );
    assert.ok(!message.startsWith("skills:"), message);
  }));

test("the family throws on an audit error, so failures found before it are not returned", () =>
  withSkillsRepo((root) => {
    const pin = pinAt(0);
    mutateFile(root, pin.rel, pin.literal, "");
    remove(root, AXES_MODE);
    mkdir(root, AXES_MODE);
    assert.throws(
      () => specPassRules(root),
      (error: unknown) =>
        error instanceof Error &&
        error.message.startsWith(`cannot read ${AXES_MODE}${CONTRACT_TAIL}`),
    );
  }));

test("a symlinked pin file is an audit error", (t) =>
  withSkillsRepo(async (root) => {
    remove(root, SPEC_AXIS);
    if (!link(root, SPEC_AXIS, absolute(root, AXES_MODE), "file"))
      return skipForLink(t);
    const message = await errorOf(root);
    assert.ok(
      message.startsWith(`cannot read ${SPEC_AXIS}${CONTRACT_TAIL}`),
      `unexpected message: ${message}`,
    );
    assert.ok(message.endsWith(": passes through a link"), message);
  }));

test("a symlinked ancestor of the pin files is an audit error", (t) =>
  withSkillsRepo((root) =>
    withOutside(async (outside) => {
      remove(root, "skills/review/references");
      if (
        linkDirectory(outside, path.join(root, "skills/review/references")) ===
        undefined
      )
        return skipForLink(t);
      const message = await errorOf(root);
      assert.ok(
        message.startsWith(`cannot read ${SPEC_AXIS}${CONTRACT_TAIL}`),
        `unexpected message: ${message}`,
      );
      assert.ok(message.endsWith(": passes through a link"), message);
    }),
  ));

test("the gate file is shared with the work-folder family and reported here on its own", () =>
  withSkillsRepo(async (root) => {
    remove(root, BRANCH);
    const found = await failuresOf(root);
    assert.equal(found.length, 1);
    assert.equal(found[0]?.rule, RULE);
    assert.equal(found[0]?.path, BRANCH);
    assert.equal(
      found[0]?.message,
      `cannot read ${BRANCH}${CONTRACT_TAIL}${BRANCH}: does not exist`,
    );
  }));

// --- audit error texts, causes and reads, rows added by the mutation pass ---------------------------------------------

const firstPinned = (): string => {
  const pin = SPEC_PASS_PINS[0];
  assert.ok(pin !== undefined);
  return pin.rel;
};

test("a pinned file that cannot be read is an audit error naming it and why", async () => {
  await withSkillsRepo(async (root) => {
    const rel = firstPinned();
    const target = absolute(root, rel);
    const real = seams.readFile;
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
      const result = await withSeams(
        {
          readFile: (candidate) => {
            if (candidate === target) throw error;
            return real(candidate);
          },
        },
        () => audit(root),
      );
      assert.deepEqual(
        result,
        { failures: [], error: `cannot read ${rel}${CONTRACT_TAIL}${text}` },
        label,
      );
    }
  });
});

test("a pinned file whose read fails as not existing is a failure, from a code or a file error", async () => {
  await withSkillsRepo(async (root) => {
    const rel = firstPinned();
    const target = absolute(root, rel);
    const real = seams.readFile;
    const gone = (): Error =>
      Object.assign(new Error("gone"), { code: "ENOENT" });
    const rows: [string, Error, string][] = [
      ["the not-exist code", gone(), `${rel}: does not exist`],
      [
        "a missing file error",
        new RepoFileError("missing", "elsewhere"),
        "elsewhere: does not exist",
      ],
    ];
    for (const [label, error, reason] of rows) {
      const result = await withSeams(
        {
          readFile: (candidate) => {
            if (candidate === target) throw error;
            return real(candidate);
          },
        },
        () => audit(root),
      );
      assert.equal(result.error, undefined, label);
      assert.deepEqual(
        brief(result.failures),
        [[RULE, rel, `cannot read ${rel}${CONTRACT_TAIL}${reason}`]],
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
          () => specPassRules(root),
          (error: unknown) =>
            error instanceof Error &&
            error.cause instanceof RepoFileError &&
            error.cause.cause === unread,
        ),
    );
  });
});

test("each pinned file is read once, however many sentences it pins", async () => {
  await withSkillsRepo(async (root) => {
    const reads = new Map<string, number>();
    const real = seams.readFile;
    await withSeams(
      {
        readFile: (candidate) => {
          reads.set(candidate, (reads.get(candidate) ?? 0) + 1);
          return real(candidate);
        },
      },
      () => audit(root),
    );
    const files = new Set(SPEC_PASS_PINS.map((pin) => pin.rel));
    assert.ok(files.size < SPEC_PASS_PINS.length, "a file pins twice");
    for (const rel of files)
      assert.equal(reads.get(absolute(root, rel)), 1, rel);
  });
});

// --- the whole-domain scenarios ------------------------------------------------------------------------------------

test("spec-pass-pinned", async () => {
  assert.deepEqual(await auditEveryFamily(breaker("spec-pass-pinned")), {
    failures: [
      {
        rule: RULE,
        path: SPEC_AXIS,
        message: `${SPEC_AXIS} no longer carries the pinned sentence "## Per-requirement table"`,
      },
    ],
  });
});
