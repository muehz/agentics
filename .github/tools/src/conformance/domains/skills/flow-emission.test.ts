import assert from "node:assert/strict";
import { mkdtempSync, renameSync, rmSync, symlinkSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { test } from "node:test";
import type { RunOptions } from "../../check.ts";
import { auditEveryFamily, breaker } from "../../testing/skills-seeds.ts";
import {
  addSkill,
  mkdir,
  mutateFile,
  remove,
  skillMD,
  withSkillsRepo,
  writeFile,
} from "../../testing/skills-repo.ts";
import { RepoFileError } from "../../repo-file-error.ts";
import { seams, withSeams, type Seams } from "./files.ts";
import { FAMILY } from "./flow-emission.ts";
import { FAMILY as INDEPENDENCE_FAMILY } from "./independence.ts";
import { run } from "./index.ts";
import { removeTree } from "../../testing/host.ts";

const options: RunOptions = {
  short: false,
  fast: false,
  diagnostics: () => {},
  env: process.env,
};

const RULE = "flow-emission-declared";

const HELPER = "agx flow";
// The old hook binary name, split so the repository-wide search for it finds it only in the independence rule.
const OLD_HELPER = "agentics-" + "hook";
const TARGET = "execute/references/flow-state.md";
const LINK = `[flow-state.md](../${TARGET})`;

const INTENT = "skills/intent/SKILL.md";
const DECOMPOSE = "skills/decompose/SKILL.md";
const GATE = "skills/gate/SKILL.md";
const TRIAGE = "skills/triage/SKILL.md";

const audit = (root: string) => run(root, options, { families: [FAMILY] });

/** The body every roster skill carries: the helper path and the link to the record contract. */
const EMISSION_BODY = `A current-state body.\n\nRecord rows through \`${HELPER} log\`, per\n${LINK}.\n`;

/** The green fixture plus intent and spec, so all six roster skills declare the helper and link the contract. */
async function withRoster<T>(body: (root: string) => Promise<T> | T) {
  return withSkillsRepo(async (root) => {
    addSkill(root, "intent", { body: EMISSION_BODY });
    addSkill(root, "spec", { body: EMISSION_BODY });
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

const helperMissing = (name: string): string[] => [
  `skills/${name}/SKILL.md`,
  `${name} does not name the flow helper call "${HELPER}"`,
];
const linkMissing = (name: string): string[] => [
  `skills/${name}/SKILL.md`,
  `${name} does not link the record contract "${TARGET}"`,
];

/** A gate skill file that names the helper and holds `rest`, so only the link varies. */
function gateWithLink(rest: string): string {
  return `${skillMD("gate")}\nRecord rows through \`${HELPER}\`.\n${rest}`;
}

test("the family is named flow-emission and reports its one rule", () => {
  assert.equal(FAMILY.name, "flow-emission");
  assert.deepEqual(FAMILY.rules, [RULE]);
});

test("a fixture whose six roster skills declare the helper and the link is clean", async () => {
  await withRoster(async (root) => {
    assert.deepEqual(await found(root), []);
  });
});

test("a skill that does not name the helper path is one failure at its own path", async () => {
  await withRoster(async (root) => {
    mutateFile(root, GATE, HELPER, "skills/execute/scripts/flow.sh");
    assert.deepEqual(await found(root), [helperMissing("gate")]);
  });
});

test("a skill that names the hook binary path instead of agx flow fails this rule and repository independence", async () => {
  await withRoster(async (root) => {
    mutateFile(root, GATE, HELPER, `hooks/bin/${OLD_HELPER}-linux-amd64 flow`);
    assert.deepEqual(await found(root), [helperMissing("gate")]);
    const independence = await run(root, options, {
      families: [INDEPENDENCE_FAMILY],
    });
    assert.equal(independence.error, undefined);
    assert.ok(
      independence.failures.some(
        (one) =>
          one.rule === "skill-repo-independence" &&
          one.path === GATE &&
          one.message.includes(`names "${OLD_HELPER}"`),
      ),
      JSON.stringify(independence.failures),
    );
  });
});

test("a skill that does not link the record contract is one failure at its own path", async () => {
  await withRoster(async (root) => {
    mutateFile(root, DECOMPOSE, TARGET, "execute/references/other.md");
    assert.deepEqual(await found(root), [linkMissing("decompose")]);
  });
});

test("triage is on the roster: missing both is two failures, the helper first", async () => {
  await withRoster(async (root) => {
    mutateFile(root, TRIAGE, HELPER, "skills/execute/scripts/flow.sh");
    mutateFile(root, TRIAGE, TARGET, "execute/references/other.md");
    assert.deepEqual(await found(root), [
      helperMissing("triage"),
      linkMissing("triage"),
    ]);
  });
  await withRoster(async (root) => {
    mutateFile(root, TRIAGE, HELPER, "skills/execute/scripts/flow.sh");
    assert.deepEqual(await found(root), [helperMissing("triage")]);
  });
  await withRoster(async (root) => {
    mutateFile(root, TRIAGE, TARGET, "execute/references/other.md");
    assert.deepEqual(await found(root), [linkMissing("triage")]);
  });
});

test("failures follow the roster order, not the file order", async () => {
  await withRoster(async (root) => {
    mutateFile(root, TRIAGE, HELPER, "skills/execute/scripts/flow.sh");
    mutateFile(root, INTENT, HELPER, "skills/execute/scripts/flow.sh");
    assert.deepEqual(await found(root), [
      helperMissing("intent"),
      helperMissing("triage"),
    ]);
  });
  // decompose sorts before intent as a file, and follows it on the roster.
  await withRoster(async (root) => {
    mutateFile(root, DECOMPOSE, HELPER, "skills/execute/scripts/flow.sh");
    mutateFile(root, INTENT, HELPER, "skills/execute/scripts/flow.sh");
    assert.deepEqual(await found(root), [
      helperMissing("intent"),
      helperMissing("decompose"),
    ]);
  });
});

test("a roster skill that is absent is not a finding", async () => {
  await withRoster(async (root) => {
    remove(root, GATE);
    assert.deepEqual(await found(root), []);
  });
});

test("a skill is matched by its name, and reported at the path the roster builds", async () => {
  await withRoster(async (root) => {
    remove(root, "skills/gate");
    writeFile(root, "skills/zzz/SKILL.md", `${skillMD("gate")}\n${LINK}\n`);
    assert.deepEqual(await found(root), [helperMissing("gate")]);
  });
});

test("quotes around the name are trimmed, and so is a trailing space", async () => {
  for (const written of ['"gate"', "'gate'", "''gate''", "gate "]) {
    await withRoster(async (root) => {
      mutateFile(root, GATE, "name: gate", `name: ${written}`);
      mutateFile(root, GATE, HELPER, "skills/execute/scripts/flow.sh");
      assert.deepEqual(await found(root), [helperMissing("gate")], written);
    });
  }
});

test("with two files of one name the later sorted file wins", async () => {
  await withRoster(async (root) => {
    // skills/gate/SKILL.md is compliant and sorts before skills/zzz/SKILL.md, which is not.
    writeFile(root, "skills/zzz/SKILL.md", skillMD("gate"));
    assert.deepEqual(await found(root), [
      helperMissing("gate"),
      linkMissing("gate"),
    ]);
  });
  await withRoster(async (root) => {
    // The non-compliant file now sorts first, so the compliant one replaces it.
    writeFile(root, "skills/aaa/SKILL.md", skillMD("gate"));
    assert.deepEqual(await found(root), []);
  });
});

test("the first name line anywhere in the file is the key, even in the body", async () => {
  await withRoster(async (root) => {
    writeFile(root, "skills/zzz/SKILL.md", "# notes\n\nname: triage\n");
    assert.deepEqual(await found(root), [
      helperMissing("triage"),
      linkMissing("triage"),
    ]);
  });
  await withRoster(async (root) => {
    // The frontmatter name comes first, so the body line is not the key.
    writeFile(
      root,
      "skills/zzz/SKILL.md",
      "---\nname: other\n---\n\nname: triage\n",
    );
    assert.deepEqual(await found(root), []);
  });
});

test("a name pattern whitespace run crosses a blank line", async () => {
  await withRoster(async (root) => {
    writeFile(root, "skills/zzz/SKILL.md", "name:\n\ngate\n");
    assert.deepEqual(await found(root), [
      helperMissing("gate"),
      linkMissing("gate"),
    ]);
  });
});

test("a carriage return or U+2028 inside the name line is part of the name, so the skill is not matched", async () => {
  const separator = String.fromCodePoint(0x2028);
  for (const text of [
    "name: gate\rmore text\n",
    `name: gate${separator}more text\n`,
  ]) {
    await withRoster(async (root) => {
      writeFile(root, "skills/zzz/SKILL.md", text);
      assert.deepEqual(await found(root), [], JSON.stringify(text));
    });
  }
});

test("a name on the last line with no newline, with a CRLF ending, or after a vertical tab is matched", async () => {
  for (const text of ["name: gate", "name: gate\r\n", "name:\vgate\n"]) {
    await withRoster(async (root) => {
      writeFile(root, "skills/zzz/SKILL.md", text);
      assert.deepEqual(
        await found(root),
        [helperMissing("gate"), linkMissing("gate")],
        JSON.stringify(text),
      );
    });
  }
});

test("a file with no name line is not keyed", async () => {
  await withRoster(async (root) => {
    writeFile(root, "skills/zzz/SKILL.md", "# nothing to see\n");
    assert.deepEqual(await found(root), []);
  });
});

test("the link scan has no fence awareness: a link inside a fenced block counts", async () => {
  await withRoster(async (root) => {
    writeFile(root, GATE, gateWithLink(`\n\`\`\`\n${LINK}\n\`\`\`\n`));
    assert.deepEqual(await found(root), []);
  });
});

test("a link destination that spans a newline in the target text does not count", async () => {
  await withRoster(async (root) => {
    writeFile(
      root,
      GATE,
      gateWithLink(
        "\n[flow-state.md](../execute/references/\nflow-state.md)\n",
      ),
    );
    assert.deepEqual(await found(root), [linkMissing("gate")]);
  });
});

test("an unclosed link destination does not count", async () => {
  await withRoster(async (root) => {
    writeFile(root, GATE, gateWithLink(`\n[flow-state.md](../${TARGET}\n`));
    assert.deepEqual(await found(root), [linkMissing("gate")]);
  });
});

test("a destination that merely contains the target counts: a URL, or a target with a title", async () => {
  for (const destination of [
    `https://example.invalid/${TARGET}`,
    `../${TARGET} "the contract"`,
    `../../skills/${TARGET}`,
  ]) {
    await withRoster(async (root) => {
      writeFile(root, GATE, gateWithLink(`\n[x](${destination})\n`));
      assert.deepEqual(await found(root), [], destination);
    });
  }
});

test("the target outside a link destination does not count", async () => {
  await withRoster(async (root) => {
    writeFile(root, GATE, gateWithLink(`\nSee ${TARGET} for the contract.\n`));
    assert.deepEqual(await found(root), [linkMissing("gate")]);
  });
});

test("a SKILL.md that is a directory is an audit error", async () => {
  await withRoster(async (root) => {
    remove(root, GATE);
    mkdir(root, GATE);
    const result = await audit(root);
    assert.deepEqual(result.failures, []);
    assert.ok(
      result.error?.startsWith("skills: reading skills/gate/SKILL.md: "),
      result.error ?? "no audit error",
    );
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

test("a SKILL.md that becomes a link after the asset listing is refused by the read", async (t) => {
  await withRoster(async (root) => {
    const outside = mkdtempSync(
      path.join(tmpdir(), "agentics-flowemission-out-"),
    );
    try {
      const target = path.join(outside, "SKILL.md");
      writeFile(outside, "SKILL.md", skillMD("gate"));
      const file = path.join(root, "skills", "gate", "SKILL.md");
      // The asset listing inspects the file first; the link appears before the read inspects it again.
      let inspections = 0;
      const lstat = linkOnInspection(
        file,
        target,
        "file",
        () => (inspections += 1) === 2,
      );
      if (lstat === undefined) {
        t.skip("this host cannot create a symlink");
        return;
      }
      assert.deepEqual(await withSeams({ lstat }, () => audit(root)), {
        failures: [],
        error: `skills: reading ${GATE}: ${GATE}: passes through a link`,
      });
    } finally {
      removeTree(outside);
    }
  });
});

// --- audit error texts and causes, rows added by the mutation pass -------------------------------------------------------

test("a skill file that cannot be read is an audit error naming it and why", async () => {
  await withRoster(async (root) => {
    const target = path.join(root, "skills", "challenge", "SKILL.md");
    const real = seams.readFile;
    const rows: [string, unknown, string][] = [
      [
        "a coded failure",
        Object.assign(new Error("EACCES: permission denied, open"), {
          code: "EACCES",
          syscall: "open",
          path: target,
        }),
        "skills/challenge/SKILL.md: EACCES",
      ],
      [
        "a failure with no code",
        new Error("odd failure"),
        "skills/challenge/SKILL.md: unreadable",
      ],
      [
        "a file error for another path",
        new RepoFileError("symlink", "somewhere"),
        "somewhere: passes through a link",
      ],
      ["no error object", null, "skills/challenge/SKILL.md: unreadable"],
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
        {
          failures: [],
          error: `skills: reading skills/challenge/SKILL.md: ${text}`,
        },
        label,
      );
    }
  });
});

test("an audit error keeps the failure that caused it as its cause", async () => {
  await withRoster(async (root) => {
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
            error.cause.cause === unread,
        ),
    );
  });
});

// --- the whole-domain scenarios ------------------------------------------------------------------------------------

test("flow-emission-declared", async () => {
  assert.deepEqual(await auditEveryFamily(breaker("flow-emission-declared")), {
    failures: [
      {
        rule: RULE,
        path: GATE,
        message: 'gate does not name the flow helper call "agx flow"',
      },
    ],
  });
});
