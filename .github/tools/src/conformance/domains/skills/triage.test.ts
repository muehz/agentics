import assert from "node:assert/strict";
import {
  readFileSync,
  renameSync,
  rmSync,
  symlinkSync,
  writeFileSync,
} from "node:fs";
import path from "node:path";
import { test, type TestContext } from "node:test";
import type { AuditResult, Failure, RunOptions } from "../../check.ts";
import {
  BREAKERS,
  cloneTriageRepo,
  link,
  mkdir,
  mutateFile,
  remove,
  rollupMappingMD,
  rollupTemplateMD,
  writeFile,
} from "../../testing/skills-repo.ts";
import * as seeds from "../../testing/skills-seeds.ts";
import {
  auditEveryFamily,
  auditSlice,
  NEEDS_SYMLINK,
  sliceNamed,
} from "../../testing/skills-seeds.ts";
import { seams, withSeams, type Seams } from "./files.ts";
import { RepoFileError } from "../../repo-file-error.ts";
import { run } from "./index.ts";
import { FAMILY } from "./triage.ts";
import { linkDirectory, removeTree, withOutside } from "../../testing/host.ts";

// The expected messages are written out in full, never built from the module's own constants.

const options: RunOptions = {
  short: false,
  fast: false,
  diagnostics: () => {},
  env: process.env,
};

const EM = String.fromCodePoint(0x2014);
const ROBOT = String.fromCodePoint(0x1f916);
const FOOTER = "> _Triaged by an AI agent_";
const BARE = "_Triaged by an AI agent_";

const TEMPLATE = "skills/triage/references/rollup-template.md";
const CONTRACT = "skills/triage/references/subagent-contract.md";
const DETECTION = "skills/triage/references/source-detection.md";
const ISSUE_MODE = "skills/triage/references/issue-mode.md";
const LIFECYCLE = "skills/triage/references/orchestration-lifecycle.md";
const COPILOT =
  "skills/triage/references/pull-requests/copilot/copilot-triage.md";
const CHECKS = "skills/triage/references/checks/checks-triage.md";
const NOTES = "skills/triage/references/notes.md";

const PRESENT = "triage-file-present";
const ROUTING = "triage-detection-routing";
const ENUM = "triage-enum-mapping";
const SHAPE = "triage-rollup-shape";
const FOOTER_RULE = "triage-attestation-footer";
const PREFIX = "triage-attestation-prefix";
const LEGACY = "triage-attestation-legacy";
const FENCE = "triage-attestation-fence";
const ISSUE_RULE = "triage-attestation-issue-mode";
const EXAMPLE = "triage-attestation-contract-example";

const MISSING_FILE = "required triage contract file is missing";
const NO_FOOTER = "missing the exact attestation footer";
const NO_EMITTER = "triaged source directory holds no *-triage.md emitter";
const NOT_EXIST = "emitter listed in the guard does not exist";
const NO_ANCHOR =
  "section emitter names no <!-- agentics:triage:section: anchor";
const NO_TABLE = "could not locate the rollup per-source mapping table";
const MISSING_MAPPING = "dispositions with no rollup mapping rule: ";
const EXAMPLE_MESSAGE =
  "JSON example body must open with the marker and close with the footer";
const FEW_FENCES = `parsed only 3 markered comment-body fences ${EM} shape changed?`;

const HUMAN_FEEDBACK_ROW =
  "| Human Feedback | Decision `accepted` or `accepted-edited` with a commit | `accepted` or `accepted-edited`, no commit | Decision `skipped` | (none) |";

const SECTION_FENCE =
  "```text\n<!-- agentics:triage:section:checks:{check_run_id} -->\n<details><summary>Checks</summary>\n\nBody.\n\n</details>\n<!-- agentics:triage:section-end:checks:{check_run_id} -->\n```\n";

const audit = (root: string): Promise<AuditResult> =>
  run(root, options, { families: [FAMILY] });

async function failuresOf(root: string): Promise<Failure[]> {
  const result = await audit(root);
  assert.equal(result.error, undefined);
  return result.failures;
}

async function errorOf(root: string): Promise<string | undefined> {
  const result = await audit(root);
  assert.deepEqual(result.failures, []);
  assert.notEqual(result.error, undefined, "expected an audit error");
  return result.error;
}

const brief = (found: Failure[]): string[][] =>
  found.map((one) => [one.rule, one.path, one.message]);

const only = (found: Failure[], rule: string): Failure[] =>
  found.filter((one) => one.rule === rule);

/** A fresh green triage fixture, removed once the (possibly asynchronous) body has settled. */
function withTriage<T>(body: (root: string) => T): T {
  const root = cloneTriageRepo();
  const cleanup = (): void => removeTree(root);
  let result: T;
  try {
    result = body(root);
  } catch (error) {
    cleanup();
    throw error;
  }
  if (result instanceof Promise) return result.finally(cleanup) as T;
  cleanup();
  return result;
}

function skipForLink(t: TestContext): void {
  t.skip("this host cannot create a symlink");
}

const abs = (root: string, rel: string): string =>
  path.join(root, ...rel.split("/"));

test("the family is named triage and lists its ten rules in report order", () => {
  assert.equal(FAMILY.name, "triage");
  assert.deepEqual(FAMILY.rules, [
    PRESENT,
    ROUTING,
    ENUM,
    SHAPE,
    FOOTER_RULE,
    PREFIX,
    LEGACY,
    FENCE,
    ISSUE_RULE,
    EXAMPLE,
  ]);
});

test("the green fixture passes", async () => {
  await withTriage(async (root) => {
    assert.deepEqual(await audit(root), { failures: [] });
  });
});

test("the source stays ASCII", () => {
  for (const file of ["triage.ts", "triage.test.ts"]) {
    const text = readFileSync(new URL(file, import.meta.url), "utf8");
    assert.equal(/[^\x00-\x7f]/.test(text), false, file);
  }
});

// --- presence and order ----------------------------------------------------

test("a missing source-detection file is one presence failure and no routing failure", async () => {
  await withTriage(async (root) => {
    remove(root, DETECTION);
    assert.deepEqual(brief(await failuresOf(root)), [
      [PRESENT, DETECTION, MISSING_FILE],
    ]);
  });
});

test("a directory in place of a contract file is an audit error, as any unreadable contract file is", async () => {
  await withTriage(async (root) => {
    remove(root, CONTRACT);
    mkdir(root, CONTRACT);
    assert.equal(await errorOf(root), `${CONTRACT}: is not a regular file`);
  });
});

test("a directory counts as a missing contract file, and the enum rule is not run", async () => {
  await withTriage(async (root) => {
    remove(root, CONTRACT);
    mkdir(root, CONTRACT);
    const original = seams.readFile;
    const dir = abs(root, CONTRACT);
    const found = await withSeams(
      { readFile: (file) => (file === dir ? FOOTER + "\n" : original(file)) },
      () => failuresOf(root),
    );
    assert.deepEqual(brief(found), [
      [PRESENT, CONTRACT, MISSING_FILE],
      [EXAMPLE, CONTRACT, EXAMPLE_MESSAGE],
    ]);
  });
});

test("a missing template is a presence failure, then an attestation failure, and no enum or shape failure", async () => {
  await withTriage(async (root) => {
    remove(root, TEMPLATE);
    assert.deepEqual(brief(await failuresOf(root)), [
      [PRESENT, TEMPLATE, MISSING_FILE],
      [FOOTER_RULE, TEMPLATE, NOT_EXIST],
      [
        FENCE,
        "skills/triage",
        `parsed only 0 markered comment-body fences ${EM} shape changed?`,
      ],
    ]);
  });
});

test("attestation runs when every contract file is missing, after the presence failures", async () => {
  await withTriage(async (root) => {
    remove(root, TEMPLATE);
    remove(root, CONTRACT);
    remove(root, DETECTION);
    assert.deepEqual(brief(await failuresOf(root)), [
      [PRESENT, TEMPLATE, MISSING_FILE],
      [PRESENT, CONTRACT, MISSING_FILE],
      [PRESENT, DETECTION, MISSING_FILE],
      [FOOTER_RULE, TEMPLATE, NOT_EXIST],
      [FOOTER_RULE, CONTRACT, NOT_EXIST],
      [
        FENCE,
        "skills/triage",
        `parsed only 0 markered comment-body fences ${EM} shape changed?`,
      ],
    ]);
  });
});

test("the findings come out in the order presence, enum, shape, attestation, routing", async () => {
  await withTriage(async (root) => {
    writeFile(
      root,
      TEMPLATE,
      rollupMappingMD().replace("`escalate`, `deferred`", "`deferred`"),
    );
    mutateFile(
      root,
      COPILOT,
      "Every posted body closes with " + FOOTER,
      "None.",
    );
    mutateFile(
      root,
      DETECTION,
      "| copilot | Copilot |",
      "| copilot | Copilot |\n| checkmarx | x |",
    );
    const found = await failuresOf(root);
    assert.deepEqual(
      found.map((one) => one.rule),
      [ENUM, SHAPE, FOOTER_RULE, ROUTING],
    );
    assert.equal(
      found[1]?.message,
      "found 0 rendered rollup examples, want at least 3",
    );
  });
});

test("a linked contract file is an audit error naming the path checked", async (t) => {
  await withTriage(async (root) => {
    await withOutside(async (outside) => {
      writeFileSync(path.join(outside, "target.md"), "# Contract\n");
      remove(root, CONTRACT);
      if (!link(root, CONTRACT, path.join(outside, "target.md"), "file"))
        return skipForLink(t);
      assert.equal(
        await errorOf(root),
        `skills: checking ${CONTRACT}: ${CONTRACT}: passes through a link`,
      );
    });
  });
});

test("a linked skills/triage is an audit error from the first presence check", async (t) => {
  await withTriage(async (root) => {
    await withOutside(async (outside) => {
      remove(root, "skills/triage");
      if (
        linkDirectory(outside, path.join(root, "skills/triage")) === undefined
      )
        return skipForLink(t);
      assert.equal(
        await errorOf(root),
        `skills: checking ${TEMPLATE}: skills/triage: passes through a link`,
      );
    });
  });
});

// --- enum mapping ----------------------------------------------------------

test("a disposition dropped from the rollup mapping is one failure naming it", async () => {
  await withTriage(async (root) => {
    mutateFile(root, TEMPLATE, "`escalate`, `deferred`", "`deferred`");
    assert.deepEqual(brief(await failuresOf(root)), [
      [ENUM, TEMPLATE, MISSING_MAPPING + "escalate"],
    ]);
  });
});

test("an enum table shrunk to seven values is a shape failure at the contract", async () => {
  await withTriage(async (root) => {
    mutateFile(root, CONTRACT, "| `deferred` | later | PR sources |\n", "");
    assert.deepEqual(brief(await failuresOf(root)), [
      [ENUM, CONTRACT, `parsed only 7 enum values ${EM} table shape changed?`],
    ]);
  });
});

test("a row with a fourth cell and a row whose first cell is not backticked are not enum rows", async () => {
  await withTriage(async (root) => {
    mutateFile(
      root,
      CONTRACT,
      "| `deferred` | later | PR sources |\n",
      "| `deferred` | later | PR sources | extra |\n| deferred | later | PR sources |\n",
    );
    assert.deepEqual(brief(await failuresOf(root)), [
      [ENUM, CONTRACT, `parsed only 7 enum values ${EM} table shape changed?`],
    ]);
  });
});

test("a renamed mapping header is a could-not-locate failure at the template", async () => {
  await withTriage(async (root) => {
    mutateFile(
      root,
      TEMPLATE,
      "| Source          | Fixed",
      "| Origin          | Fixed",
    );
    assert.deepEqual(brief(await failuresOf(root)), [
      [ENUM, TEMPLATE, NO_TABLE],
    ]);
  });
});

test("a mapping table that ends the file with one newline is not located", async () => {
  await withTriage(async (root) => {
    const text = rollupMappingMD();
    writeFile(
      root,
      TEMPLATE,
      text.slice(0, text.indexOf("\n\n## Rendered rollup bodies")) + "\n",
    );
    assert.deepEqual(brief(only(await failuresOf(root), ENUM)), [
      [ENUM, TEMPLATE, NO_TABLE],
    ]);
  });
});

test("a template with CRLF line endings is not located either", async () => {
  await withTriage(async (root) => {
    const file = abs(root, TEMPLATE);
    writeFileSync(file, readFileSync(file, "utf8").replaceAll("\n", "\r\n"));
    assert.deepEqual(brief(only(await failuresOf(root), ENUM)), [
      [ENUM, TEMPLATE, NO_TABLE],
    ]);
  });
});

for (const decision of ["accepted", "accepted-edited", "skipped"]) {
  test(`a Human Feedback row that drops decision ${decision} is one failure`, async () => {
    await withTriage(async (root) => {
      const file = abs(root, TEMPLATE);
      writeFileSync(
        file,
        readFileSync(file, "utf8").replaceAll("`" + decision + "`", "dropped"),
      );
      assert.deepEqual(brief(await failuresOf(root)), [
        [
          ENUM,
          TEMPLATE,
          "Human Feedback mapping row does not name decision `" +
            decision +
            "`",
        ],
      ]);
    });
  });
}

test("a missing Human Feedback row fails the three decisions in order", async () => {
  await withTriage(async (root) => {
    mutateFile(root, TEMPLATE, HUMAN_FEEDBACK_ROW + "\n", "");
    assert.deepEqual(brief(await failuresOf(root)), [
      [
        ENUM,
        TEMPLATE,
        "Human Feedback mapping row does not name decision `accepted`",
      ],
      [
        ENUM,
        TEMPLATE,
        "Human Feedback mapping row does not name decision `accepted-edited`",
      ],
      [
        ENUM,
        TEMPLATE,
        "Human Feedback mapping row does not name decision `skipped`",
      ],
    ]);
  });
});

test("a Human Feedback value is exempt, and a value used by PR sources is not", async () => {
  await withTriage(async (root) => {
    assert.deepEqual(await failuresOf(root), []);
    mutateFile(
      root,
      CONTRACT,
      "| `drafted` | reply drafted | Human Feedback |",
      "| `drafted` | reply drafted | PR sources |",
    );
    assert.deepEqual(brief(await failuresOf(root)), [
      [ENUM, TEMPLATE, MISSING_MAPPING + "drafted"],
    ]);
  });
});

test("a value listed twice takes the later used-by cell", async () => {
  await withTriage(async (root) => {
    mutateFile(root, TEMPLATE, "`escalate`, `deferred`", "`deferred`");
    mutateFile(
      root,
      CONTRACT,
      "| `deferred` | later | PR sources |\n",
      "| `deferred` | later | PR sources |\n| `escalate` | again | Human Feedback |\n",
    );
    assert.deepEqual(await failuresOf(root), []);
  });
  await withTriage(async (root) => {
    mutateFile(
      root,
      CONTRACT,
      "| `drafted` | reply drafted | Human Feedback |\n",
      "| `drafted` | reply drafted | Human Feedback |\n| `drafted` | again | PR sources |\n",
    );
    assert.deepEqual(brief(await failuresOf(root)), [
      [ENUM, TEMPLATE, MISSING_MAPPING + "drafted"],
    ]);
  });
});

test("unmapped values are listed once each in byte order, after any Human Feedback failure", async () => {
  await withTriage(async (root) => {
    mutateFile(
      root,
      CONTRACT,
      "| `drafted` | reply drafted | Human Feedback |\n",
      "| `drafted` | reply drafted | Human Feedback |\n| `zeta` | z | PR sources |\n| `alpha` | a | PR sources |\n| `zeta` | z | PR sources |\n",
    );
    assert.deepEqual(brief(await failuresOf(root)), [
      [ENUM, TEMPLATE, MISSING_MAPPING + "alpha, zeta"],
    ]);
    mutateFile(root, TEMPLATE, "Decision `skipped`", "Decision dropped");
    assert.deepEqual(brief(await failuresOf(root)), [
      [
        ENUM,
        TEMPLATE,
        "Human Feedback mapping row does not name decision `skipped`",
      ],
      [ENUM, TEMPLATE, MISSING_MAPPING + "alpha, zeta"],
    ]);
  });
});

// --- detection routing -----------------------------------------------------

const routingMessage = (line: number): string =>
  `line ${line}: bot routing table must not carry a checkmarx login row ${EM} detection is marker-keyed`;

test("a checkmarx row inside the routing table is one failure at its 1-based line", async () => {
  await withTriage(async (root) => {
    mutateFile(
      root,
      DETECTION,
      "| copilot | Copilot |",
      "| copilot | Copilot |\n| checkmarx | Checkmarx |",
    );
    const line =
      readFileSync(abs(root, DETECTION), "utf8")
        .split("\n")
        .findIndex((text) => text.startsWith("| checkmarx")) + 1;
    assert.equal(line, 10);
    assert.deepEqual(brief(await failuresOf(root)), [
      [ROUTING, DETECTION, routingMessage(10)],
    ]);
  });
});

test("every checkmarx row in the table is reported, in line order", async () => {
  await withTriage(async (root) => {
    mutateFile(
      root,
      DETECTION,
      "| copilot | Copilot |",
      "| checkmarx | one |\n| copilot | Copilot |\n| other | CHECKMARX |",
    );
    assert.deepEqual(brief(await failuresOf(root)), [
      [ROUTING, DETECTION, routingMessage(8 + 1)],
      [ROUTING, DETECTION, routingMessage(8 + 3)],
    ]);
  });
});

test("a checkmarx row above the heading or below the marker-keyed line is not checked", async () => {
  await withTriage(async (root) => {
    mutateFile(
      root,
      DETECTION,
      "### Bot routing table",
      "| checkmarx | above |\n\n### Bot routing table",
    );
    mutateFile(
      root,
      DETECTION,
      "**Marker-keyed sources bypass this table.**\n",
      "**Marker-keyed sources bypass this table.**\n| checkmarx | below |\n",
    );
    assert.deepEqual(await failuresOf(root), []);
  });
});

test("a capital-C Checkmarx row fires", async () => {
  await withTriage(async (root) => {
    mutateFile(
      root,
      DETECTION,
      "| copilot | Copilot |",
      "| Checkmarx | Checkmarx |\n| copilot | Copilot |",
    );
    assert.deepEqual(brief(await failuresOf(root)), [
      [ROUTING, DETECTION, routingMessage(8 + 1)],
    ]);
  });
});

test("a line inside the table that is not a table row is not checked", async () => {
  await withTriage(async (root) => {
    mutateFile(
      root,
      DETECTION,
      "| copilot | Copilot |",
      "A note that mentions checkmarx.\n| copilot | Copilot |",
    );
    assert.deepEqual(await failuresOf(root), []);
  });
});

test("a linked source-detection file is an audit error, not a failure", async (t) => {
  await withTriage(async (root) => {
    await withOutside(async (outside) => {
      const target = path.join(outside, "detection.md");
      writeFileSync(target, "# Detection\n");
      remove(root, DETECTION);
      if (!link(root, DETECTION, target, "file")) return skipForLink(t);
      assert.equal(
        await errorOf(root),
        `skills: checking ${DETECTION}: ${DETECTION}: passes through a link`,
      );
    });
  });
});

// --- attestation emitters --------------------------------------------------

test("a copilot emitter without the footer is a footer failure at its path", async () => {
  await withTriage(async (root) => {
    mutateFile(
      root,
      COPILOT,
      "Every posted body closes with " + FOOTER,
      "No footer here.",
    );
    assert.deepEqual(brief(await failuresOf(root)), [
      [FOOTER_RULE, COPILOT, NO_FOOTER],
    ]);
  });
});

test("a deleted emitter leaves its source directory reported", async () => {
  await withTriage(async (root) => {
    remove(root, COPILOT);
    assert.deepEqual(brief(await failuresOf(root)), [
      [
        FOOTER_RULE,
        "skills/triage/references/pull-requests/copilot",
        NO_EMITTER,
      ],
    ]);
  });
});

test("a new source directory is audited without any edit to the rule", async () => {
  const newbot =
    "skills/triage/references/pull-requests/newbot/newbot-triage.md";
  await withTriage(async (root) => {
    writeFile(root, newbot, "# Newbot\n\nPost a reply. No footer here.\n");
    assert.deepEqual(brief(await failuresOf(root)), [
      [FOOTER_RULE, newbot, NO_FOOTER],
    ]);
  });
  await withTriage(async (root) => {
    writeFile(
      root,
      newbot,
      "# Newbot\n\nEvery posted body closes with " + FOOTER + "\n",
    );
    assert.deepEqual(await failuresOf(root), []);
  });
  await withTriage(async (root) => {
    writeFile(
      root,
      "skills/triage/references/pull-requests/newbot/notes.md",
      "# Notes\n",
    );
    assert.deepEqual(brief(await failuresOf(root)), [
      [
        FOOTER_RULE,
        "skills/triage/references/pull-requests/newbot",
        NO_EMITTER,
      ],
    ]);
  });
});

test("an upper-case emitter name is not an emitter", async () => {
  await withTriage(async (root) => {
    writeFile(
      root,
      "skills/triage/references/pull-requests/newbot/X-TRIAGE.md",
      "# X\n",
    );
    assert.deepEqual(brief(await failuresOf(root)), [
      [
        FOOTER_RULE,
        "skills/triage/references/pull-requests/newbot",
        NO_EMITTER,
      ],
    ]);
  });
});

test("a linked directory under pull-requests is not a source", async (t) => {
  await withTriage(async (root) => {
    await withOutside(async (outside) => {
      if (
        linkDirectory(
          outside,
          path.join(root, "skills/triage/references/pull-requests/linked"),
        ) === undefined
      )
        return skipForLink(t);
      assert.deepEqual(await failuresOf(root), []);
    });
  });
});

test("the checks emitter passes with its anchor and no footer, and fails without the anchor", async () => {
  await withTriage(async (root) => {
    assert.deepEqual(await failuresOf(root), []);
    writeFile(root, CHECKS, "# Checks\n\nNo footer, no section.\n");
    assert.deepEqual(brief(await failuresOf(root)), [
      [FOOTER_RULE, CHECKS, NO_ANCHOR],
    ]);
  });
});

test("the checkmarx and merge-conflicts adapters are section emitters too", async () => {
  for (const source of ["checkmarx", "merge-conflicts"]) {
    const rel = `skills/triage/references/pull-requests/${source}/${source}-triage.md`;
    await withTriage(async (root) => {
      writeFile(
        root,
        rel,
        "# Section\n\nFramed by <!-- agentics:triage:section:x -->.\n",
      );
      assert.deepEqual(await failuresOf(root), []);
      writeFile(root, rel, "# Section\n\nNo anchor and no footer.\n");
      assert.deepEqual(brief(await failuresOf(root)), [
        [FOOTER_RULE, rel, NO_ANCHOR],
      ]);
    });
  }
});

test("a thread-reply adapter that only mentions an anchor still needs the footer", async () => {
  await withTriage(async (root) => {
    writeFile(
      root,
      COPILOT,
      "# Emitter\n\nSee <!-- agentics:triage:section:checkmarx --> in the rollup.\n",
    );
    assert.deepEqual(brief(await failuresOf(root)), [
      [FOOTER_RULE, COPILOT, NO_FOOTER],
    ]);
  });
});

test("a removed issue-mode file is a footer failure and no issue-mode failure", async () => {
  await withTriage(async (root) => {
    remove(root, ISSUE_MODE);
    assert.deepEqual(brief(await failuresOf(root)), [
      [FOOTER_RULE, ISSUE_MODE, NOT_EXIST],
    ]);
  });
});

test("an emitter that is a directory is an audit error with the read error's text", async () => {
  await withTriage(async (root) => {
    remove(root, ISSUE_MODE);
    mkdir(root, ISSUE_MODE);
    assert.equal(await errorOf(root), `${ISSUE_MODE}: is not a regular file`);
  });
});

test("a missing references/checks directory is an audit error", async () => {
  await withTriage(async (root) => {
    remove(root, "skills/triage/references/checks");
    assert.equal(
      await errorOf(root),
      `skills: reading skills/triage/references/checks: skills/triage/references/checks: does not exist`,
    );
  });
});

test("a linked references/checks directory is an audit error", async (t) => {
  await withTriage(async (root) => {
    await withOutside(async (outside) => {
      remove(root, "skills/triage/references/checks");
      if (
        linkDirectory(
          outside,
          path.join(root, "skills/triage/references/checks"),
        ) === undefined
      )
        return skipForLink(t);
      assert.equal(
        await errorOf(root),
        `skills: reading skills/triage/references/checks: skills/triage/references/checks: passes through a link`,
      );
    });
  });
});

test("a linked references/pull-requests is an audit error and an empty ordinary one is not", async (t) => {
  await withTriage(async (root) => {
    await withOutside(async (outside) => {
      remove(root, "skills/triage/references/pull-requests");
      mkdir(root, "skills/triage/references/pull-requests");
      assert.deepEqual(await failuresOf(root), []);
      remove(root, "skills/triage/references/pull-requests");
      if (
        linkDirectory(
          outside,
          path.join(root, "skills/triage/references/pull-requests"),
        ) === undefined
      )
        return skipForLink(t);
      assert.equal(
        await errorOf(root),
        `skills: reading skills/triage/references/pull-requests: skills/triage/references/pull-requests: passes through a link`,
      );
    });
  });
});

test("a removed skills/triage directory is an audit error, not three presence failures", async () => {
  await withTriage(async (root) => {
    remove(root, "skills/triage");
    assert.equal(
      await errorOf(root),
      `skills: reading skills/triage/references/pull-requests: skills/triage: does not exist`,
    );
  });
});

// --- per-file scan ---------------------------------------------------------

const prefixMessage = (line: number, quoted: string): string =>
  `line ${line}: footer missing its \`> \` blockquote prefix: ${quoted}`;

test("a bare footer line is a prefix failure showing the line quoted", async () => {
  await withTriage(async (root) => {
    writeFile(root, NOTES, "# Notes\n\n" + BARE + "\n");
    assert.deepEqual(brief(await failuresOf(root)), [
      [PREFIX, NOTES, prefixMessage(3, '"_Triaged by an AI agent_"')],
    ]);
  });
});

test("a bare footer in an emitter is a footer failure and then a prefix failure", async () => {
  await withTriage(async (root) => {
    mutateFile(root, COPILOT, "closes with " + FOOTER, "closes with " + BARE);
    assert.deepEqual(brief(await failuresOf(root)), [
      [FOOTER_RULE, COPILOT, NO_FOOTER],
      [
        PREFIX,
        COPILOT,
        prefixMessage(
          3,
          '"Every posted body closes with _Triaged by an AI agent_"',
        ),
      ],
    ]);
  });
});

test("a footer nested two blockquote levels deep is a prefix failure", async () => {
  await withTriage(async (root) => {
    writeFile(root, NOTES, "# Notes\n\n> > " + BARE + "\n");
    assert.deepEqual(brief(await failuresOf(root)), [
      [
        PREFIX,
        NOTES,
        'line 3: footer nested deeper than one blockquote level: "> > _Triaged by an AI agent_"',
      ],
    ]);
  });
});

test("a footer behind >> is not a prefix failure", async () => {
  await withTriage(async (root) => {
    writeFile(root, NOTES, "# Notes\n\n>> " + BARE + "\n");
    assert.deepEqual(await failuresOf(root), []);
  });
});

test("each footer occurrence on a line is judged", async () => {
  await withTriage(async (root) => {
    writeFile(root, NOTES, "# Notes\n\n" + BARE + " and " + FOOTER + "\n");
    assert.deepEqual(brief(await failuresOf(root)), [
      [
        PREFIX,
        NOTES,
        prefixMessage(
          3,
          '"_Triaged by an AI agent_ and > _Triaged by an AI agent_"',
        ),
      ],
    ]);
  });
});

test("a bare footer line ending in CRLF is reported with the carriage return shown", async () => {
  await withTriage(async (root) => {
    writeFile(root, NOTES, "# Notes\r\n\r\n" + BARE + "\r\n");
    assert.deepEqual(brief(await failuresOf(root)), [
      [PREFIX, NOTES, prefixMessage(3, '"_Triaged by an AI agent_\\r"')],
    ]);
  });
});

test("each superseded form is one legacy failure, in a fixed order", async () => {
  await withTriage(async (root) => {
    mutateFile(
      root,
      DETECTION,
      "# Source detection",
      "# Source detection\n\nThis was generated by AI during triage.",
    );
    assert.deepEqual(brief(await failuresOf(root)), [
      [LEGACY, DETECTION, "retains the superseded leading disclaimer"],
    ]);
  });
  await withTriage(async (root) => {
    mutateFile(
      root,
      DETECTION,
      "# Source detection",
      "# Source detection\n\n" + ROBOT + " _Triaged by someone else_.",
    );
    assert.deepEqual(brief(await failuresOf(root)), [
      [LEGACY, DETECTION, "retains the superseded emoji-prefixed footer"],
    ]);
  });
  await withTriage(async (root) => {
    mutateFile(
      root,
      DETECTION,
      "# Source detection",
      "# Source detection\n\n_Triaged by [Agentics](https://example.invalid/agentics)",
    );
    assert.deepEqual(brief(await failuresOf(root)), [
      [
        LEGACY,
        DETECTION,
        "retains the superseded footer that links to the Agentics repository",
      ],
    ]);
  });
  await withTriage(async (root) => {
    writeFile(
      root,
      NOTES,
      "_Triaged by [Agentics](https://example.invalid/agentics)\n" +
        ROBOT +
        " _Triaged by someone\nThis was generated by AI during triage.\n",
    );
    assert.deepEqual(brief(await failuresOf(root)), [
      [LEGACY, NOTES, "retains the superseded leading disclaimer"],
      [LEGACY, NOTES, "retains the superseded emoji-prefixed footer"],
      [
        LEGACY,
        NOTES,
        "retains the superseded footer that links to the Agentics repository",
      ],
    ]);
  });
});

test("a markered fence that does not close with the footer is a fence failure", async () => {
  await withTriage(async (root) => {
    mutateFile(
      root,
      TEMPLATE,
      "Body for rollup:pr.\n\n" + FOOTER,
      "Body for rollup:pr.\n\nNo footer.",
    );
    assert.deepEqual(brief(await failuresOf(root)), [
      [
        FENCE,
        TEMPLATE,
        'markered fence must close with the footer (ends "No footer.")',
      ],
    ]);
  });
});

test("a markered fence ending in the footer and a space still fires", async () => {
  await withTriage(async (root) => {
    mutateFile(
      root,
      TEMPLATE,
      "Body for rollup:pr.\n\n" + FOOTER,
      "Body for rollup:pr.\n\n" + FOOTER + " ",
    );
    assert.deepEqual(brief(await failuresOf(root)), [
      [
        FENCE,
        TEMPLATE,
        'markered fence must close with the footer (ends "> _Triaged by an AI agent_ ")',
      ],
    ]);
  });
});

test("fewer than four markered fences is one failure at skills/triage", async () => {
  await withTriage(async (root) => {
    writeFile(
      root,
      TEMPLATE,
      rollupMappingMD().replace(
        "<!-- agentics:triage:rollup:security -->",
        "no longer a marker",
      ),
    );
    assert.deepEqual(brief(only(await failuresOf(root), FENCE)), [
      [FENCE, "skills/triage", FEW_FENCES],
    ]);
  });
});

test("a section fence without the footer is no failure and is not counted", async () => {
  await withTriage(async (root) => {
    writeFile(
      root,
      TEMPLATE,
      rollupMappingMD().replace(
        "<!-- agentics:triage:rollup:security -->",
        "no longer a marker",
      ),
    );
    writeFile(root, CHECKS, "# Checks\n\n" + SECTION_FENCE);
    assert.deepEqual(brief(only(await failuresOf(root), FENCE)), [
      [FENCE, "skills/triage", FEW_FENCES],
    ]);
    assert.deepEqual(only(await failuresOf(root), FOOTER_RULE), []);
  });
  await withTriage(async (root) => {
    writeFile(root, CHECKS, "# Checks\n\n" + SECTION_FENCE);
    assert.deepEqual(await failuresOf(root), []);
  });
});

test("a section fence with the footer inside is a fence failure and is not counted", async () => {
  await withTriage(async (root) => {
    writeFile(
      root,
      TEMPLATE,
      rollupMappingMD().replace(
        "<!-- agentics:triage:rollup:security -->",
        "no longer a marker",
      ),
    );
    writeFile(
      root,
      CHECKS,
      "# Checks\n\n" +
        SECTION_FENCE.replace("</details>\n", "</details>\n\n" + FOOTER + "\n"),
    );
    assert.deepEqual(brief(only(await failuresOf(root), FENCE)), [
      [
        FENCE,
        CHECKS,
        "embedded section <!-- agentics:triage:section:checks:{check_run_id} --> must not carry its own footer",
      ],
      [FENCE, "skills/triage", FEW_FENCES],
    ]);
  });
});

test("a fence whose first non-blank line is not a triage marker is ignored", async () => {
  await withTriage(async (root) => {
    writeFile(
      root,
      NOTES,
      "# Notes\n\n```markdown\n\nplain text, no footer\n```\n\n```text\nmore plain text\n```\n",
    );
    assert.deepEqual(await failuresOf(root), []);
  });
});

test("a .md file in a deep directory is scanned and a .MD file is not", async () => {
  await withTriage(async (root) => {
    writeFile(
      root,
      "skills/triage/references/deep/er/notes.md",
      "This was generated by AI during triage.\n",
    );
    writeFile(
      root,
      "skills/triage/references/deep/NOTES.MD",
      "This was generated by AI during triage.\n",
    );
    assert.deepEqual(brief(await failuresOf(root)), [
      [
        LEGACY,
        "skills/triage/references/deep/er/notes.md",
        "retains the superseded leading disclaimer",
      ],
    ]);
  });
});

test("the files are scanned in byte order of their paths, not in walk order", async () => {
  await withTriage(async (root) => {
    const text = "This was generated by AI during triage.\n";
    writeFile(root, "skills/triage/references/a/b.md", text);
    writeFile(root, "skills/triage/references/a.md", text);
    assert.deepEqual(
      (await failuresOf(root)).map((one) => one.path),
      ["skills/triage/references/a.md", "skills/triage/references/a/b.md"],
    );
  });
});

test("a failed read of a scanned file is an audit error", async () => {
  await withTriage(async (root) => {
    writeFile(root, NOTES, "# Notes\n");
    const original = seams.readFile;
    const file = abs(root, NOTES);
    const message = `${NOTES}: EACCES`;
    const result = await withSeams(
      {
        readFile: (target) => {
          if (target === file)
            throw Object.assign(new Error("denied"), { code: "EACCES" });
          return original(target);
        },
      },
      () => audit(root),
    );
    assert.deepEqual(result, { failures: [], error: message });
  });
});

// --- issue mode and contract example --------------------------------------

test("an issue-mode template that does not close with the footer is a failure", async () => {
  await withTriage(async (root) => {
    mutateFile(
      root,
      ISSUE_MODE,
      "Disposition template: fix.\n\n" + FOOTER,
      "Disposition template: fix.\n\nNo footer.",
    );
    assert.deepEqual(brief(await failuresOf(root)), [
      [
        ISSUE_RULE,
        ISSUE_MODE,
        'disposition template must close with the footer (ends "No footer.")',
      ],
    ]);
  });
});

test("an empty markdown fence is an issue-mode failure with an empty last line", async () => {
  await withTriage(async (root) => {
    writeFile(
      root,
      ISSUE_MODE,
      readFileSync(abs(root, ISSUE_MODE), "utf8") + "\n```markdown\n```\n",
    );
    assert.deepEqual(brief(await failuresOf(root)), [
      [
        ISSUE_RULE,
        ISSUE_MODE,
        'disposition template must close with the footer (ends "")',
      ],
    ]);
  });
});

test("three issue-mode templates are a shape failure", async () => {
  await withTriage(async (root) => {
    mutateFile(
      root,
      ISSUE_MODE,
      "```markdown\nDisposition template: question.\n\n" + FOOTER + "\n```\n",
      "",
    );
    assert.deepEqual(brief(await failuresOf(root)), [
      [
        ISSUE_RULE,
        ISSUE_MODE,
        `parsed only 3 issue-mode disposition templates ${EM} shape changed?`,
      ],
    ]);
  });
});

test("a contract example that loses its escaped line breaks is a failure", async () => {
  await withTriage(async (root) => {
    mutateFile(root, CONTRACT, "description.\\n\\n", "description. ");
    assert.deepEqual(brief(await failuresOf(root)), [
      [EXAMPLE, CONTRACT, EXAMPLE_MESSAGE],
    ]);
  });
});

test("a contract example that closes with the footer but opens without the marker is a failure", async () => {
  await withTriage(async (root) => {
    mutateFile(
      root,
      CONTRACT,
      '"body": "<!-- agentics:triage:reply:',
      '"body": "reply:',
    );
    assert.deepEqual(brief(await failuresOf(root)), [
      [EXAMPLE, CONTRACT, EXAMPLE_MESSAGE],
    ]);
  });
});

test("a linked issue-mode file is an audit error from the emitter read", async (t) => {
  await withTriage(async (root) => {
    await withOutside(async (outside) => {
      const target = path.join(outside, "issue-mode.md");
      writeFileSync(target, "# Issue mode\n");
      remove(root, ISSUE_MODE);
      if (!link(root, ISSUE_MODE, target, "file")) return skipForLink(t);
      assert.equal(await errorOf(root), `${ISSUE_MODE}: passes through a link`);
    });
  });
});

test("the lifecycle emitter needs the footer", async () => {
  await withTriage(async (root) => {
    writeFile(root, LIFECYCLE, "# Orchestration lifecycle\n");
    assert.deepEqual(brief(await failuresOf(root)), [
      [FOOTER_RULE, LIFECYCLE, NO_FOOTER],
    ]);
  });
});

// --- white space, line starts and fence bodies in the triage patterns --------------------------------------
// White space is tab, LF, FF, CR and space only, so a vertical tab, a no-break space, U+2028 and U+0085 are not white
// space; a line starts only after LF, so a lone CR, U+2028, U+2029 and U+0085 do not start one; a fence body matches
// every character, a CR and the Unicode line separators included.

const VT = "\v";
const NBSP = String.fromCodePoint(0xa0);
const LS = String.fromCodePoint(0x2028);
const PS = String.fromCodePoint(0x2029);
const NEL = String.fromCodePoint(0x85);

/** Each character with whether the patterns count it as white space. */
const SPACE_CHARS: ReadonlyArray<readonly [string, string, boolean]> = [
  ["a tab", "\t", true],
  ["a form feed", "\f", true],
  ["a carriage return", "\r", true],
  ["a vertical tab", VT, false],
  ["a no-break space", NBSP, false],
  ["U+2028", LS, false],
  ["U+0085", NEL, false],
];

/** The characters after which no line starts for the patterns, even though some of them end one in JavaScript. */
const NOT_LINE_BREAKS: ReadonlyArray<readonly [string, string]> = [
  ["a lone carriage return", "\r"],
  ["U+2028", LS],
  ["U+2029", PS],
  ["U+0085", NEL],
];

const ENUM_ROW = "| `deferred` | later | PR sources |\n";

for (const [name, char, isSpace] of SPACE_CHARS) {
  test(`${name} around an enum row cell or after its last pipe ${isSpace ? "is" : "is not"} white space`, async () => {
    const rows = [
      `|${char}\`deferred\` | later | PR sources |\n`,
      `| \`deferred\`${char}| later | PR sources |\n`,
      `| \`deferred\` | later | PR sources |${char}\n`,
    ];
    for (const row of rows) {
      await withTriage(async (root) => {
        mutateFile(root, CONTRACT, ENUM_ROW, row);
        assert.deepEqual(
          brief(await failuresOf(root)),
          isSpace
            ? []
            : [
                [
                  ENUM,
                  CONTRACT,
                  `parsed only 7 enum values ${EM} table shape changed?`,
                ],
              ],
          JSON.stringify(row),
        );
      });
    }
  });

  test(`${name} between Source and Fixed in the mapping header ${isSpace ? "is" : "is not"} white space`, async () => {
    await withTriage(async (root) => {
      mutateFile(
        root,
        TEMPLATE,
        "| Source          | Fixed",
        `| Source${char}${char}| Fixed`,
      );
      assert.deepEqual(
        brief(await failuresOf(root)),
        isSpace ? [] : [[ENUM, TEMPLATE, NO_TABLE]],
      );
    });
  });
}

test("a mapping header after CRLF starts a line", async () => {
  await withTriage(async (root) => {
    mutateFile(
      root,
      TEMPLATE,
      "## Per-source mapping\n\n| Source          | Fixed",
      "## Per-source mapping\n\n\r\n| Source          | Fixed",
    );
    assert.deepEqual(await failuresOf(root), []);
  });
});

for (const [name, char] of NOT_LINE_BREAKS) {
  test(`a mapping header after ${name} does not start a line`, async () => {
    await withTriage(async (root) => {
      mutateFile(
        root,
        TEMPLATE,
        "## Per-source mapping\n\n| Source          | Fixed",
        `## Per-source mapping\n\n${char}| Source          | Fixed`,
      );
      assert.deepEqual(brief(await failuresOf(root)), [
        [ENUM, TEMPLATE, NO_TABLE],
      ]);
    });
  });
}

const NO_CLOSE =
  'markered fence must close with the footer (ends "No footer.")';
const NO_CLOSE_ISSUE =
  'disposition template must close with the footer (ends "No footer.")';

for (const [name, char] of [["an LF", "\n"], ...NOT_LINE_BREAKS] as const) {
  const lineStart = char === "\n";
  test(`a markered fence opened after ${name} ${lineStart ? "is" : "is not"} a fence`, async () => {
    await withTriage(async (root) => {
      writeFile(
        root,
        NOTES,
        `# Notes${char}\`\`\`text\n<!-- agentics:triage:rollup:x -->\n\nNo footer.\n\`\`\`\n`,
      );
      assert.deepEqual(
        brief(await failuresOf(root)),
        lineStart ? [[FENCE, NOTES, NO_CLOSE]] : [],
      );
    });
  });

  test(`a markered fence closed after ${name} ${lineStart ? "is" : "is not"} a fence`, async () => {
    await withTriage(async (root) => {
      writeFile(
        root,
        NOTES,
        `\`\`\`text\n<!-- agentics:triage:rollup:x -->\n\nNo footer.${char}\`\`\`\n`,
      );
      assert.deepEqual(
        brief(await failuresOf(root)),
        lineStart ? [[FENCE, NOTES, NO_CLOSE]] : [],
      );
    });
  });

  test(`an issue-mode template opened after ${name} ${lineStart ? "is" : "is not"} a fence`, async () => {
    await withTriage(async (root) => {
      writeFile(
        root,
        ISSUE_MODE,
        readFileSync(abs(root, ISSUE_MODE), "utf8") +
          `\n# Tail${char}\`\`\`markdown\nNo footer.\n\`\`\`\n`,
      );
      assert.deepEqual(
        brief(await failuresOf(root)),
        lineStart ? [[ISSUE_RULE, ISSUE_MODE, NO_CLOSE_ISSUE]] : [],
      );
    });
  });

  test(`an issue-mode template closed after ${name} ${lineStart ? "is" : "is not"} a fence`, async () => {
    await withTriage(async (root) => {
      writeFile(
        root,
        ISSUE_MODE,
        readFileSync(abs(root, ISSUE_MODE), "utf8") +
          `\n\`\`\`markdown\nNo footer.${char}\`\`\`\n`,
      );
      assert.deepEqual(
        brief(await failuresOf(root)),
        lineStart ? [[ISSUE_RULE, ISSUE_MODE, NO_CLOSE_ISSUE]] : [],
      );
    });
  });
}

for (const [name, char] of [
  ["a lone carriage return", "\r"],
  ["U+2028", LS],
  ["U+2029", PS],
] as const) {
  test(`${name} inside a fence body is part of the body, as the s flag's dot matches it`, async () => {
    await withTriage(async (root) => {
      writeFile(
        root,
        NOTES,
        `\`\`\`text\n<!-- agentics:triage:rollup:x -->${char}middle\nNo footer.\n\`\`\`\n`,
      );
      assert.deepEqual(brief(await failuresOf(root)), [
        [FENCE, NOTES, NO_CLOSE],
      ]);
    });
    await withTriage(async (root) => {
      writeFile(
        root,
        ISSUE_MODE,
        readFileSync(abs(root, ISSUE_MODE), "utf8") +
          `\n\`\`\`markdown\nA${char}B\nNo footer.\n\`\`\`\n`,
      );
      assert.deepEqual(brief(await failuresOf(root)), [
        [ISSUE_RULE, ISSUE_MODE, NO_CLOSE_ISSUE],
      ]);
    });
  });
}

// --- audit error texts, causes and path filters, rows added by the mutation pass ---------------------------------------

/** The inspection and read errors a file-system call can raise on `target`, shown as `shown`, with the reason each gives. */
function failureRows(
  target: string,
  shown: string,
): [string, unknown, string][] {
  return [
    [
      "a coded failure",
      Object.assign(new Error("EACCES: permission denied, lstat"), {
        code: "EACCES",
        syscall: "lstat",
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
      "a symlink error for another path",
      new RepoFileError("symlink", "somewhere"),
      "somewhere: passes through a link",
    ],
    ["no error object", null, `${shown}: unreadable`],
  ];
}

/** Runs the audit with the read of `file` failing with `error` on its first read only. */
function auditFailingRead(root: string, file: string, error: unknown) {
  const real = seams.readFile;
  let failed = false;
  return withSeams(
    {
      readFile: (target) => {
        if (target === file && !failed) {
          failed = true;
          throw error;
        }
        return real(target);
      },
    },
    () => audit(root),
  );
}

test("a contract file that cannot be read is an audit error naming it and why", async () => {
  await withTriage(async (root) => {
    const file = abs(root, CONTRACT);
    for (const [label, error, text] of failureRows(file, CONTRACT)) {
      const result = await auditFailingRead(root, file, error);
      assert.deepEqual(result, { failures: [], error: text }, label);
    }
  });
});

test("an emitter whose read fails as not existing is a footer failure, from a code or a file error", async () => {
  await withTriage(async (root) => {
    const file = abs(root, LIFECYCLE);
    const gone = (): Error =>
      Object.assign(new Error("gone"), { code: "ENOENT" });
    for (const [label, error] of [
      ["the not-exist code", gone()],
      ["a missing file error", new RepoFileError("missing", "elsewhere")],
    ] as const) {
      const result = await auditFailingRead(root, file, error);
      assert.deepEqual(
        brief(result.failures),
        [
          [
            FOOTER_RULE,
            LIFECYCLE,
            "emitter listed in the guard does not exist",
          ],
        ],
        label,
      );
      assert.equal(result.error, undefined, label);
    }
    const other = await auditFailingRead(
      root,
      file,
      new Error("read failed", {
        cause: Object.assign(new Error("denied"), { code: "EACCES" }),
      }),
    );
    assert.equal(other.error, `${LIFECYCLE}: unreadable`);
    const none = await auditFailingRead(root, file, null);
    assert.equal(none.error, `${LIFECYCLE}: unreadable`);
  });
});

/** Whether the call in progress was made from the named function, so a seam can fail one call site only. */
const calledFrom = (name: string): boolean =>
  new Error("probe").stack?.includes(`at ${name} (`) ?? false;

test("an audit error keeps the failure that caused it as its cause", async () => {
  await withTriage(async (root) => {
    const context = { families: [], evalsCheck: () => [] };
    // A failure the file reader wraps is the cause of the file error that the audit error carries.
    const causeOf = (error: Error): unknown =>
      error.cause instanceof RepoFileError && error.cause.cause !== undefined
        ? error.cause.cause
        : error.cause;
    const expectCause = (body: () => unknown, cause: unknown, label: string) =>
      assert.throws(
        body,
        (error: unknown) => error instanceof Error && causeOf(error) === cause,
        label,
      );
    const unread = new Error("read failed");
    const contract = abs(root, CONTRACT);
    const realRead = seams.readFile;
    withSeams(
      {
        readFile: (target) => {
          if (target === contract) throw unread;
          return realRead(target);
        },
      },
      () => expectCause(() => FAMILY.run(root, context), unread, "a read"),
    );
    const realLstat = seams.lstat;
    const template = abs(root, TEMPLATE);
    const linked = new RepoFileError("symlink", template);
    withSeams(
      {
        lstat: (target) => {
          if (target === template) throw linked;
          return realLstat(target);
        },
      },
      () =>
        expectCause(
          () => FAMILY.run(root, context),
          linked,
          "a presence check",
        ),
    );
    const checks = abs(root, "skills/triage/references/checks");
    const triageRoot = abs(root, "skills/triage");
    const realReadDir = seams.readDir;
    const rows: [string, string, string][] = [
      [
        checks,
        "a source directory",
        "skills: reading skills/triage/references/checks: skills/triage/references/checks: EACCES",
      ],
      [
        triageRoot,
        "the skill walk",
        "skills: walking skills/triage: skills/triage: EACCES",
      ],
    ];
    for (const [dir, label, message] of rows) {
      withSeams(
        {
          readDir: (target) => {
            if (target === dir)
              throw Object.assign(new Error("denied"), { code: "EACCES" });
            return realReadDir(target);
          },
        },
        () =>
          assert.throws(
            () => FAMILY.run(root, context),
            (error: unknown) =>
              error instanceof Error &&
              error.message === message &&
              error.cause instanceof Error,
            label,
          ),
      );
    }
  });
});

test("a link met only while walking skills/triage is an audit error naming the walk", async () => {
  await withTriage(async (root) => {
    const triageRoot = abs(root, "skills/triage");
    const linked = new RepoFileError("symlink", triageRoot);
    const real = seams.lstat;
    const result = await withSeams(
      {
        lstat: (target) => {
          if (target === triageRoot && calledFrom("markdownFiles"))
            throw linked;
          return real(target);
        },
      },
      () => audit(root),
    );
    assert.deepEqual(result, {
      failures: [],
      error: `skills: walking skills/triage: ${linked.message}`,
    });
  });
});

test("a link met only while walking skills/triage keeps the link as the cause", async () => {
  await withTriage(async (root) => {
    const triageRoot = abs(root, "skills/triage");
    const linked = new RepoFileError("symlink", triageRoot);
    const real = seams.lstat;
    withSeams(
      {
        lstat: (target) => {
          if (target === triageRoot && calledFrom("markdownFiles"))
            throw linked;
          return real(target);
        },
      },
      () =>
        assert.throws(
          () => FAMILY.run(root, { families: [], evalsCheck: () => [] }),
          (error: unknown) => error instanceof Error && error.cause === linked,
        ),
    );
  });
});

test("a skill directory that cannot be inspected for the walk, other than by a link, has no markdown files", async () => {
  await withTriage(async (root) => {
    const triageRoot = abs(root, "skills/triage");
    const real = seams.lstat;
    const result = await withSeams(
      {
        lstat: (target) => {
          if (target === triageRoot && calledFrom("markdownFiles"))
            throw Object.assign(new Error("denied"), { code: "EACCES" });
          return real(target);
        },
      },
      () => audit(root),
    );
    assert.equal(result.error, undefined);
    assert.deepEqual(brief(result.failures), [
      [
        FENCE,
        "skills/triage",
        `parsed only 0 markered comment-body fences ${EM} shape changed?`,
      ],
    ]);
  });
});

test("a link met only when the issue-mode and contract files are read after the emitter pass is an audit error", async () => {
  for (const rel of [ISSUE_MODE, CONTRACT]) {
    await withTriage(async (root) => {
      const file = abs(root, rel);
      const linked = new RepoFileError("symlink", file);
      const real = seams.lstat;
      // The emitter pass inspects the file first and the later check second; only the second fails.
      let reads = 0;
      const result = await withSeams(
        {
          lstat: (target) => {
            if (
              target === file &&
              calledFrom("attestationRules") &&
              !calledFrom("readText") &&
              (reads += 1) === 2
            )
              throw linked;
            return real(target);
          },
        },
        () => audit(root),
      );
      assert.deepEqual(result, { failures: [], error: linked.message }, rel);
    });
  }
});

test("only a link ends the later issue-mode and contract reads: any other failure leaves the check out", async () => {
  for (const rel of [ISSUE_MODE, CONTRACT]) {
    await withTriage(async (root) => {
      const file = abs(root, rel);
      const real = seams.lstat;
      // The emitter pass inspects the file first and the later check second; only the second fails.
      let reads = 0;
      const result = await withSeams(
        {
          lstat: (target) => {
            if (
              target === file &&
              calledFrom("attestationRules") &&
              !calledFrom("readText") &&
              (reads += 1) === 2
            )
              throw Object.assign(new Error("denied"), {
                code: "EACCES",
                syscall: "lstat",
                path: target,
              });
            return real(target);
          },
        },
        () => audit(root),
      );
      assert.deepEqual(result, { failures: [] }, rel);
    });
  }
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

test("a skills/triage that becomes a link before the markdown walk is refused by the walk's own check", async (t) => {
  await withTriage((root) =>
    withOutside(async (outside) => {
      const triageRoot = abs(root, "skills/triage");
      const lstat = linkOnInspection(triageRoot, outside, "dir", () =>
        calledFrom("markdownFiles"),
      );
      if (lstat === undefined) return skipForLink(t);
      assert.deepEqual(await withSeams({ lstat }, () => audit(root)), {
        failures: [],
        error:
          "skills: walking skills/triage: skills/triage: passes through a link",
      });
    }),
  );
});

/** Links `rel` to an outside file just before the attestation check reads it again after the emitter pass. */
async function laterReadMeetsLink(t: TestContext, rel: string): Promise<void> {
  await withTriage((root) =>
    withOutside(async (outside) => {
      const target = path.join(outside, "outside.md");
      writeFileSync(target, "# Outside\n");
      const file = abs(root, rel);
      // The emitter pass inspects the file first and the later check second; the link appears before the second.
      let reads = 0;
      const lstat = linkOnInspection(
        file,
        target,
        "file",
        () =>
          calledFrom("attestationRules") &&
          !calledFrom("readText") &&
          (reads += 1) === 2,
      );
      if (lstat === undefined) return skipForLink(t);
      assert.deepEqual(await withSeams({ lstat }, () => audit(root)), {
        failures: [],
        error: `${rel}: passes through a link`,
      });
    }),
  );
}

test("an issue-mode file that becomes a link after the emitter pass is refused by the later read", (t) =>
  laterReadMeetsLink(t, ISSUE_MODE));

test("a contract file that becomes a link after the emitter pass is refused by the later read", (t) =>
  laterReadMeetsLink(t, CONTRACT));

test("a source-detection file that becomes a link before the routing check is refused by its read", async (t) => {
  await withTriage((root) =>
    withOutside(async (outside) => {
      const target = path.join(outside, "outside.md");
      writeFileSync(target, "# Outside\n");
      const file = abs(root, DETECTION);
      // Only the routing check reads the file through readText outside the attestation rules.
      const lstat = linkOnInspection(
        file,
        target,
        "file",
        () => calledFrom("readText") && !calledFrom("attestationRules"),
      );
      if (lstat === undefined) return skipForLink(t);
      assert.deepEqual(await withSeams({ lstat }, () => audit(root)), {
        failures: [],
        error: `${DETECTION}: passes through a link`,
      });
    }),
  );
});

test("source directories are audited in byte order of their paths, not in discovery order", async () => {
  await withTriage(async (root) => {
    remove(root, CHECKS);
    writeFile(
      root,
      "skills/triage/references/pull-requests/aaa/notes.md",
      "# Notes\n",
    );
    assert.deepEqual(brief(await failuresOf(root)), [
      [FOOTER_RULE, "skills/triage/references/checks", NO_EMITTER],
      [FOOTER_RULE, "skills/triage/references/pull-requests/aaa", NO_EMITTER],
    ]);
  });
});

test("a regular file beside the source directories is not a source", async () => {
  await withTriage(async (root) => {
    writeFile(root, "skills/triage/references/pull-requests/stray.txt", "x\n");
    assert.deepEqual(await failuresOf(root), []);
  });
});

test("a directory named like a markdown file is not scanned as one", async () => {
  await withTriage(async (root) => {
    mkdir(root, "skills/triage/references/dir.md");
    assert.deepEqual(await failuresOf(root), []);
  });
});

test("a symlink named like a markdown file is not scanned as one", async (t) => {
  await withTriage(async (root) => {
    await withOutside(async (outside) => {
      const target = path.join(outside, "outside.md");
      writeFileSync(target, "# Outside\n\n" + BARE + "\n");
      if (!link(root, "skills/triage/references/link.md", target, "file"))
        return skipForLink(t);
      assert.deepEqual(await failuresOf(root), []);
    });
  });
});

test("a footer is nested when its prefix ends in two quote markers, whatever comes before them", async () => {
  await withTriage(async (root) => {
    writeFile(root, NOTES, "# Notes\n\nsee > > " + BARE + "\n");
    assert.deepEqual(brief(await failuresOf(root)), [
      [
        PREFIX,
        NOTES,
        `line 3: footer nested deeper than one blockquote level: ${JSON.stringify("see > > " + BARE)}`,
      ],
    ]);
  });
});

test("a line that only ends with the routing heading does not open the table", async () => {
  await withTriage(async (root) => {
    writeFile(
      root,
      DETECTION,
      "x### Bot routing table\n| checkmarx | Checkmarx |\n",
    );
    assert.deepEqual(await failuresOf(root), []);
  });
});

test("a directory named like an emitter is not an emitter", async () => {
  await withTriage(async (root) => {
    mkdir(root, "skills/triage/references/pull-requests/copilot/dir-triage.md");
    assert.deepEqual(await failuresOf(root), []);
  });
});

test("a symlink named like an emitter is not an emitter", async (t) => {
  await withTriage(async (root) => {
    await withOutside(async (outside) => {
      const target = path.join(outside, "outside.md");
      writeFileSync(target, "# Outside\n");
      if (
        !link(
          root,
          "skills/triage/references/pull-requests/copilot/link-triage.md",
          target,
          "file",
        )
      )
        return skipForLink(t);
      assert.deepEqual(await failuresOf(root), []);
    });
  });
});

test("a routing row that does not end in a pipe is still a row", async () => {
  await withTriage(async (root) => {
    mutateFile(
      root,
      DETECTION,
      "| copilot | Copilot |",
      "| copilot | Copilot |\n| checkmarx | Checkmarx",
    );
    assert.deepEqual(brief(await failuresOf(root)), [
      [ROUTING, DETECTION, routingMessage(10)],
    ]);
  });
});

// --- the triage scenarios of the skills domain audit ---------------------------------------------------------------

/** The slice that holds the triage family, whose families a slice scenario runs. */
const CONTENT_SLICE = sliceNamed("content");

/** The breaker of `rule`, which the slice and the full audit both apply. */
function breakerOf(rule: string): seeds.Mutation {
  const mutate = BREAKERS[rule];
  assert.ok(mutate !== undefined, `no breaker for ${rule}`);
  return mutate;
}

/** Asserts that the breaker of `rule` yields exactly `failures`, under the content slice and under every family. */
async function assertBreaker(rule: string, failures: Failure[]): Promise<void> {
  assert.deepEqual(await auditSlice(CONTENT_SLICE, breakerOf(rule)), {
    failures,
  });
  assert.deepEqual(await auditEveryFamily(breakerOf(rule)), { failures });
}

// The breaker of each triage rule, under the content slice and under every family.

test("triage-file-present", async () => {
  await assertBreaker("triage-file-present", [
    {
      rule: "triage-file-present",
      path: "skills/triage/references/source-detection.md",
      message: "required triage contract file is missing",
    },
  ]);
});

test("triage-detection-routing", async () => {
  await assertBreaker("triage-detection-routing", [
    {
      rule: "triage-detection-routing",
      path: "skills/triage/references/source-detection.md",
      message: `line 10: bot routing table must not carry a checkmarx login row ${EM} detection is marker-keyed`,
    },
  ]);
});

test("triage-enum-mapping", async () => {
  await assertBreaker("triage-enum-mapping", [
    {
      rule: "triage-enum-mapping",
      path: "skills/triage/references/rollup-template.md",
      message: "dispositions with no rollup mapping rule: deferred",
    },
  ]);
});

test("triage-attestation-footer", async () => {
  await assertBreaker("triage-attestation-footer", [
    {
      rule: "triage-attestation-footer",
      path: "skills/triage/references/pull-requests/copilot/copilot-triage.md",
      message: "missing the exact attestation footer",
    },
  ]);
});

test("triage-attestation-prefix", async () => {
  await assertBreaker("triage-attestation-prefix", [
    {
      rule: "triage-attestation-prefix",
      path: "skills/triage/references/notes.md",
      message:
        'line 3: footer missing its `> ` blockquote prefix: "_Triaged by an AI agent_"',
    },
  ]);
});

test("triage-attestation-legacy", async () => {
  await assertBreaker("triage-attestation-legacy", [
    {
      rule: "triage-attestation-legacy",
      path: "skills/triage/references/source-detection.md",
      message: "retains the superseded leading disclaimer",
    },
  ]);
});

test("triage-attestation-fence", async () => {
  await assertBreaker("triage-attestation-fence", [
    {
      rule: "triage-attestation-fence",
      path: "skills/triage/references/rollup-template.md",
      message:
        'markered fence must close with the footer (ends "Closing line.")',
    },
  ]);
});

test("triage-attestation-issue-mode", async () => {
  await assertBreaker("triage-attestation-issue-mode", [
    {
      rule: "triage-attestation-issue-mode",
      path: "skills/triage/references/issue-mode.md",
      message:
        'disposition template must close with the footer (ends "Disposition template: fix.")',
    },
  ]);
});

test("triage-attestation-contract-example", async () => {
  await assertBreaker("triage-attestation-contract-example", [
    {
      rule: "triage-attestation-contract-example",
      path: "skills/triage/references/subagent-contract.md",
      message:
        "JSON example body must open with the marker and close with the footer",
    },
  ]);
});

// The content slice, on the pasted-format fixture.

test("a rollup template with CRLF line endings", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      seeds.put(seeds.ROLLUP_TEMPLATE, seeds.crlf(rollupTemplateMD())),
    ),
    {
      failures: [
        {
          rule: "triage-enum-mapping",
          path: "skills/triage/references/rollup-template.md",
          message: "could not locate the rollup per-source mapping table",
        },
        {
          rule: "triage-rollup-shape",
          path: "skills/triage/references/rollup-template.md",
          message: "found 0 rendered rollup examples, want at least 3",
        },
        {
          rule: "triage-attestation-fence",
          path: "skills/triage",
          message: `parsed only 0 markered comment-body fences ${EM} shape changed?`,
        },
      ],
    },
  );
});

test("a footer line ending in a carriage return", async () => {
  assert.deepEqual(
    await auditSlice(CONTENT_SLICE, (r) =>
      mutateFile(
        r,
        seeds.ROLLUP_TEMPLATE,
        `Body for rollup:pr.\n\n${FOOTER}\n`,
        `Body for rollup:pr.\n\n${FOOTER}\r\n`,
      ),
    ),
    {
      failures: [
        {
          rule: "triage-attestation-fence",
          path: "skills/triage/references/rollup-template.md",
          message:
            'markered fence must close with the footer (ends "> _Triaged by an AI agent_\\r")',
        },
      ],
    },
  );
});

test("a footer nested two quote levels deep", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      seeds.triageRef("notes.md", `> > _Triaged by an AI agent_\n`),
    ),
    {
      failures: [
        {
          rule: "triage-attestation-prefix",
          path: "skills/triage/references/notes.md",
          message:
            'line 1: footer nested deeper than one blockquote level: "> > _Triaged by an AI agent_"',
        },
      ],
    },
  );
});

test("a bare footer and a quoted one on one line", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      seeds.triageRef("notes.md", `_Triaged by an AI agent_ then ${FOOTER}\n`),
    ),
    {
      failures: [
        {
          rule: "triage-attestation-prefix",
          path: "skills/triage/references/notes.md",
          message:
            'line 1: footer missing its `> ` blockquote prefix: "_Triaged by an AI agent_ then > _Triaged by an AI agent_"',
        },
      ],
    },
  );
});

test("a new source directory with no emitter", async () => {
  assert.deepEqual(
    await auditSlice(CONTENT_SLICE, (r) =>
      mkdir(r, `${seeds.TRIAGE_REFS}/pull-requests/newbot`),
    ),
    {
      failures: [
        {
          rule: "triage-attestation-footer",
          path: "skills/triage/references/pull-requests/newbot",
          message: "triaged source directory holds no *-triage.md emitter",
        },
      ],
    },
  );
});

test("a new source directory with an emitter that lacks the footer", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      seeds.triageRef(
        "pull-requests/newbot/newbot-triage.md",
        "# Emitter\n\nNothing.\n",
      ),
    ),
    {
      failures: [
        {
          rule: "triage-attestation-footer",
          path: "skills/triage/references/pull-requests/newbot/newbot-triage.md",
          message: "missing the exact attestation footer",
        },
      ],
    },
  );
});

test("a new source directory holding only a file that is not an emitter", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      seeds.triageRef("pull-requests/newbot/notes.md", "# Notes\n"),
    ),
    {
      failures: [
        {
          rule: "triage-attestation-footer",
          path: "skills/triage/references/pull-requests/newbot",
          message: "triaged source directory holds no *-triage.md emitter",
        },
      ],
    },
  );
});

test("a section fence with the footer inside", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      seeds.triageRef(
        "notes.md",
        `# Notes\n\n${seeds.FENCE}text\n<!-- agentics:triage:section:x -->\n\nBody.\n\n${FOOTER}\n${seeds.FENCE}\n`,
      ),
    ),
    {
      failures: [
        {
          rule: "triage-attestation-fence",
          path: "skills/triage/references/notes.md",
          message:
            "embedded section <!-- agentics:triage:section:x --> must not carry its own footer",
        },
      ],
    },
  );
});

test("the mapping table at the end of the file with one newline", async () => {
  // The template is cut just after its mapping table, so no blank line follows the table and no example is left.
  assert.deepEqual(
    await auditSlice(CONTENT_SLICE, (r) => {
      const text = rollupTemplateMD();
      const cut = text.indexOf("\n\n## Rendered rollup bodies");
      writeFile(r, seeds.ROLLUP_TEMPLATE, text.slice(0, cut + 1));
    }),
    {
      failures: [
        {
          rule: "triage-enum-mapping",
          path: "skills/triage/references/rollup-template.md",
          message: "could not locate the rollup per-source mapping table",
        },
        {
          rule: "triage-rollup-shape",
          path: "skills/triage/references/rollup-template.md",
          message: "found 0 rendered rollup examples, want at least 3",
        },
        {
          rule: "triage-attestation-footer",
          path: "skills/triage/references/rollup-template.md",
          message: "missing the exact attestation footer",
        },
        {
          rule: "triage-attestation-fence",
          path: "skills/triage",
          message: `parsed only 0 markered comment-body fences ${EM} shape changed?`,
        },
      ],
    },
  );
});

test("a mixed-case CheckMarx row in the routing table", async () => {
  assert.deepEqual(
    await auditSlice(CONTENT_SLICE, (r) =>
      mutateFile(
        r,
        `${seeds.TRIAGE_REFS}/source-detection.md`,
        "| copilot | Copilot |\n",
        "| copilot | Copilot |\n| CheckMarx | Code Security |\n",
      ),
    ),
    {
      failures: [
        {
          rule: "triage-detection-routing",
          path: "skills/triage/references/source-detection.md",
          message: `line 10: bot routing table must not carry a checkmarx login row ${EM} detection is marker-keyed`,
        },
      ],
    },
  );
});

test("the legacy emoji footer and the linked footer", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      seeds.triageRef(
        "notes.md",
        `# Notes\n\n${ROBOT} _Triaged by hand_\n\n_Triaged by [Agentics](https://example.invalid)_\n`,
      ),
    ),
    {
      failures: [
        {
          rule: "triage-attestation-legacy",
          path: "skills/triage/references/notes.md",
          message: "retains the superseded emoji-prefixed footer",
        },
        {
          rule: "triage-attestation-legacy",
          path: "skills/triage/references/notes.md",
          message:
            "retains the superseded footer that links to the Agentics repository",
        },
      ],
    },
  );
});

test("an emitter that is a dangling symlink", NEEDS_SYMLINK, async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      seeds.all(
        (r) => remove(r, seeds.COPILOT_EMITTER),
        seeds.symlinked(seeds.COPILOT_EMITTER, "nowhere-triage.md", "file"),
      ),
    ),
    {
      failures: [
        {
          rule: "triage-attestation-footer",
          path: "skills/triage/references/pull-requests/copilot",
          message: "triaged source directory holds no *-triage.md emitter",
        },
      ],
    },
  );
});

test(
  "an emitter that is a symlink to a regular file",
  NEEDS_SYMLINK,
  async () => {
    assert.deepEqual(
      await auditSlice(
        CONTENT_SLICE,
        seeds.all(
          (r) => remove(r, seeds.COPILOT_EMITTER),
          seeds.symlinked(
            seeds.COPILOT_EMITTER,
            "../../source-detection.md",
            "file",
          ),
        ),
      ),
      {
        failures: [
          {
            rule: "triage-attestation-footer",
            path: "skills/triage/references/pull-requests/copilot",
            message: "triaged source directory holds no *-triage.md emitter",
          },
        ],
      },
    );
  },
);
