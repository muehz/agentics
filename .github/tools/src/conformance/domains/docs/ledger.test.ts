import assert from "node:assert/strict";
import {
  mkdirSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  symlinkSync,
} from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { test } from "node:test";
import type { AuditResult, Failure } from "../../check.ts";
import { main, STALE_REVIEW_EXIT } from "../../run.ts";
import { sixColumnLedger, STAMP } from "../../testing/docs-seeds.ts";
import {
  auditDocs,
  ledgerRow,
  LEDGER_HEADER,
  readFixture,
  withDocsRepo,
  writeFile,
} from "../../testing/docs-repo.ts";
import { fakeHost } from "../../testing/fake-host.ts";
import { ENV_MAX_AGE_DAYS, run } from "./index.ts";
import { LEDGER_REL, ledgerRules } from "./ledger.ts";
import { removeTree } from "../../testing/host.ts";

const NOW = new Date("2026-09-13T12:00:00Z");

const audit = (root: string, maxAge = -1, now = NOW): Promise<AuditResult> =>
  auditDocs(root, { maxAge, now });

/** The failures of an audit that must have run to completion. */
async function failuresOf(
  root: string,
  maxAge = -1,
  now = NOW,
): Promise<Failure[]> {
  const result = await audit(root, maxAge, now);
  assert.equal(result.error, undefined);
  return result.failures;
}

/** Exactly one failure, of `rule`, on the ledger, whose message names `doc`. */
function assertOnly(failures: Failure[], rule: string, doc: string): void {
  assert.equal(failures.length, 1, JSON.stringify(failures));
  assert.equal(failures[0]?.rule, rule);
  assert.equal(failures[0]?.path, LEDGER_REL);
  assert.ok(failures[0]?.message.includes(doc), failures[0]?.message);
}

const SURFACES = ["claude-code", "codex", "copilot-cli", "cursor"] as const;

test("a directory named like a surface doc is skipped by the ledger and is an audit error in the model-id scan", async () => {
  await withDocsRepo(async (root) => {
    mkdirSync(path.join(root, "docs", "surfaces", "x.md"));
    writeFile(
      root,
      "docs/surfaces/cursor.md",
      "Try gpt-9-nova.\n\nVerified on Cursor 3.15.6 (2026-08-09).\n",
    );
    const result = await audit(root);
    assert.match(
      result.error ?? "",
      /docs: docs\/surfaces\/x\.md: is not a regular file/,
    );
    assert.deepEqual(
      result.failures.map((f) => [f.rule, f.path]),
      [["model-ids-known", "docs/surfaces/cursor.md"]],
    );
  });
});

test("a link named like a surface doc is a surface doc even when it points at a directory", async (t) => {
  await withDocsRepo(async (root) => {
    const outside = mkdtempSync(path.join(tmpdir(), "agentics-ledger-out-"));
    try {
      try {
        symlinkSync(
          outside,
          path.join(root, "docs", "surfaces", "x.md"),
          "dir",
        );
      } catch {
        t.skip("this host cannot create a symlink");
        return;
      }
      const result = ledgerRules(root, -1, NOW);
      assert.deepEqual(result.failures, []);
      assert.equal(
        result.error,
        "docs: docs/surfaces/x.md: is not a regular file",
      );
    } finally {
      removeTree(outside);
    }
  });
});

test("a roster drift does not suppress the age check", async () => {
  await withDocsRepo(async (root) => {
    writeFile(
      root,
      "docs/surfaces/copilot-cli.md",
      "The manifest registers all 9 Agentics skills.\n\nVerified on Copilot CLI 1.0.78 (2026-08-09).\n",
    );
    assert.deepEqual(
      (await failuresOf(root, 30, NOW)).map((f) => f.rule),
      [
        "roster-count-drift",
        "review-age",
        "review-age",
        "review-age",
        "review-age",
      ],
    );
  });
});

/** Rewrites the ledger with `codex` carrying `stamp`, in four or six columns; the other rows are fresh. */
function writeLedger(
  root: string,
  columns: 4 | 6,
  stamp: string,
  options: {
    sourceDate?: string;
    result?: string;
    badDuplicate?: boolean;
  } = {},
): void {
  const header =
    columns === 4
      ? "| Surface | Agentics doc | Upstream requirement sources | Last reviewed |"
      : "| Surface | Agentics doc | Upstream requirement sources | Last reviewed | Requirements review / retained probe | Result |";
  const separator =
    columns === 4
      ? "| --- | --- | --- | --- |"
      : "| --- | --- | --- | --- | --- | --- |";
  const rows = [header, separator];
  const tail = (result: string) =>
    columns === 6
      ? ` Published docs; retained probe: CLI 1.0 (2026-08-09) | ${result} |`
      : "";
  for (const name of SURFACES) {
    const codex = name === "codex";
    const date = codex ? stamp : "2026-09-12";
    const sourceDate = codex
      ? (options.sourceDate ?? "2026-09-12")
      : "2026-09-12";
    rows.push(
      `| ${name} | [${name}.md](${name}.md) | [docs](https://example.com/${name}/${sourceDate}) | ${date} |${tail(codex ? (options.result ?? "No packaging drift.") : "No packaging drift.")}`,
    );
    if (codex && options.badDuplicate)
      rows.push(
        `| codex | [codex.md](codex.md) | [docs](https://example.com/2026-09-13/) | not-a-date |${tail("No drift.")}`,
      );
  }
  writeFile(root, LEDGER_REL, `${rows.join("\n")}\n`);
}

test("the fixture ledger is green, with the age check off and on", async () => {
  await withDocsRepo(async (root) => {
    assert.deepEqual(await failuresOf(root), []);
    assert.deepEqual(
      await failuresOf(root, 30, new Date("2026-08-15T12:00:00Z")),
      [],
    );
  });
});

test("a review date is trimmed as String.prototype.trim trims", async () => {
  await withDocsRepo(async (root) => {
    writeLedger(root, 4, "\u{feff}2026-08-01\u{feff}");
    assert.deepEqual(await failuresOf(root), []);
    writeLedger(root, 4, "\u{85}2026-08-01\u{85}");
    const failures = await failuresOf(root);
    assertOnly(failures, "ledger-row-date", "codex.md");
    assert.match(
      failures[0]?.message ?? "",
      /invalid Last reviewed value "\u{85}2026-08-01\u{85}"/u,
    );
  });
});

test("a missing ledger fires ledger-present and nothing else", async () => {
  await withDocsRepo(async (root) => {
    rmSync(path.join(root, ...LEDGER_REL.split("/")));
    const failures = await failuresOf(root);
    assert.deepEqual(
      failures.map((f) => [f.rule, f.path]),
      [["ledger-present", LEDGER_REL]],
    );
  });
});

test("a surface doc without a ledger row fires ledger-row-present", async () => {
  await withDocsRepo(async (root) => {
    writeFile(
      root,
      "docs/surfaces/vscode.md",
      "# VS Code\n\nVerified on VS Code 1.99.0 (2026-08-08).\n",
    );
    assertOnly(await failuresOf(root), "ledger-row-present", "vscode.md");
  });
});

test("a row with no https source fires ledger-row-source, and one source among several rows satisfies it", async () => {
  await withDocsRepo(async (root) => {
    writeFile(
      root,
      LEDGER_REL,
      [
        ...LEDGER_HEADER,
        ledgerRow("claude-code", "2026-08-09"),
        ledgerRow("codex", "2026-08-01", "none pinned"),
        ledgerRow("copilot-cli", "2026-08-09"),
        ledgerRow("cursor", "2026-08-09"),
        "",
      ].join("\n"),
    );
    assertOnly(await failuresOf(root), "ledger-row-source", "codex.md");
    // The source check reads all of a doc's rows together.
    writeFile(
      root,
      LEDGER_REL,
      `${readFixture(root, LEDGER_REL)}${ledgerRow("codex", "2026-08-01")}\n`,
    );
    assert.deepEqual(await failuresOf(root), []);
  });
});

test("an unstamped surface doc fires probe-stamp-present on that doc only", async () => {
  await withDocsRepo(async (root) => {
    writeFile(root, "docs/surfaces/codex.md", "# Codex\n\nNo stamp here.\n");
    const failures = await failuresOf(root);
    assert.deepEqual(
      failures.map((f) => [f.rule, f.path]),
      [["probe-stamp-present", "docs/surfaces/codex.md"]],
    );
  });
});

test("a stamp needs a CLI name and an ISO date", async () => {
  await withDocsRepo(async (root) => {
    for (const body of [
      "Verified on (2026-08-01).",
      "Verified on  (2026-08-01).",
      "Verified on codex-cli 0.147.0 (Aug 1).",
      "Verified on codex-cli 0.147.0 2026-08-01.",
    ]) {
      writeFile(root, "docs/surfaces/codex.md", `# Codex\n\n${body}\n`);
      assert.equal(
        (await failuresOf(root))[0]?.rule,
        "probe-stamp-present",
        body,
      );
    }
    // A CRLF checkout keeps the carriage return out of the way of the stamp.
    writeFile(
      root,
      "docs/surfaces/codex.md",
      "# Codex\r\n\r\nVerified on codex-cli 0.147.0 (2026-08-01).\r\n",
    );
    assert.deepEqual(await failuresOf(root), []);
  });
});

test("a row that points at a missing doc fires ledger-doc-exists once per target", async () => {
  await withDocsRepo(async (root) => {
    const gone = ledgerRow("vscode", "2026-08-09");
    writeFile(
      root,
      LEDGER_REL,
      `${readFixture(root, LEDGER_REL)}${gone}\n${gone}\n`,
    );
    const failures = await failuresOf(root);
    assert.deepEqual(
      failures.map((f) => [f.rule, f.path]),
      [["ledger-doc-exists", LEDGER_REL]],
    );
    assert.match(failures[0]?.message ?? "", /docs\/surfaces\/vscode\.md/);
  });
});

test("no surface docs is an audit error, never a pass", async () => {
  await withDocsRepo(async (root) => {
    for (const name of SURFACES)
      rmSync(path.join(root, "docs", "surfaces", `${name}.md`));
    const result = await audit(root);
    assert.match(
      result.error ?? "",
      /no surface docs found under docs\/surfaces/,
    );
    assert.deepEqual(
      result.failures.map((f) => [f.rule, f.path]),
      [
        ["roster-claim-site-present", "docs/surfaces/claude-code.md"],
        ["roster-claim-site-present", "docs/surfaces/copilot-cli.md"],
      ],
    );
  });
});

test("the review date is the fifth field of the row, in four and in six columns", async () => {
  const cases: Array<{
    name: string;
    stamp: string;
    sourceDate?: string;
    want: string;
  }> = [
    { name: "fresh control", stamp: "2026-09-12", want: "" },
    { name: "blank", stamp: "", want: "ledger-row-date" },
    { name: "invalid", stamp: "not-a-date", want: "ledger-row-date" },
    { name: "impossible day", stamp: "2026-02-30", want: "ledger-row-date" },
    { name: "month 13", stamp: "2026-13-01", want: "ledger-row-date" },
    { name: "month 00", stamp: "2026-00-10", want: "ledger-row-date" },
    { name: "day 00", stamp: "2026-03-00", want: "ledger-row-date" },
    { name: "31 April", stamp: "2026-04-31", want: "ledger-row-date" },
    { name: "short month", stamp: "2026-9-12", want: "ledger-row-date" },
    {
      name: "trailing text",
      stamp: "2026-09-12 (re-verified)",
      want: "ledger-row-date",
    },
    {
      name: "stale stamp, fresh link",
      stamp: "2026-07-01",
      sourceDate: "2026-09-13",
      want: "review-age",
    },
    {
      name: "future stamp, fresh link",
      stamp: "2026-09-20",
      sourceDate: "2026-09-13",
      want: "review-date-future",
    },
    {
      name: "fresh stamp, old link",
      stamp: "2026-09-12",
      sourceDate: "2020-01-01",
      want: "",
    },
    {
      name: "fresh stamp, future link",
      stamp: "2026-09-12",
      sourceDate: "2030-01-01",
      want: "",
    },
    { name: "one day of grace", stamp: "2026-09-14", want: "" },
    {
      name: "two days is the future",
      stamp: "2026-09-15",
      want: "review-date-future",
    },
    { name: "leap day", stamp: "2024-02-29", want: "review-age" },
    { name: "not a leap day", stamp: "2026-02-29", want: "ledger-row-date" },
  ];
  for (const columns of [4, 6] as const) {
    for (const { name, stamp, sourceDate, want } of cases) {
      await withDocsRepo(async (root) => {
        writeLedger(root, columns, stamp, { sourceDate });
        const failures = await failuresOf(root, 30);
        if (want === "") assert.deepEqual(failures, [], `${columns}/${name}`);
        else assertOnly(failures, want, "codex.md");
      });
    }
  }
});

test("the age budget is inclusive, and an age check below zero is off", async () => {
  await withDocsRepo(async (root) => {
    writeLedger(root, 4, "2026-08-14");
    // 30 days before NOW: within a budget of 30, over a budget of 29.
    assert.deepEqual(await failuresOf(root, 30), []);
    assertOnly(await failuresOf(root, 29), "review-age", "codex.md");
    assert.deepEqual(await failuresOf(root, -1), []);
    assert.deepEqual(await failuresOf(root, -5), []);
    // Zero is armed: every row reviewed before today is over budget, and all four are.
    assert.deepEqual(
      (await failuresOf(root, 0)).map((f) => f.rule),
      ["review-age", "review-age", "review-age", "review-age"],
    );
  });
});

test("the age message names the doc, the date, the age and the budget", async () => {
  await withDocsRepo(async (root) => {
    writeLedger(root, 4, "2026-07-01");
    assert.equal(
      (await failuresOf(root, 30))[0]?.message,
      "codex.md: last reviewed 2026-07-01 (74 days ago; budget 30)",
    );
    writeLedger(root, 4, "2026-09-20");
    assert.equal(
      (await failuresOf(root, 30))[0]?.message,
      "codex.md: review date 2026-09-20 is in the future (invalid stamp)",
    );
  });
});

test("the age check does not run while a structural check is failing", async () => {
  await withDocsRepo(async (root) => {
    writeLedger(root, 4, "2026-07-01");
    writeFile(root, "docs/surfaces/codex.md", "# Codex\n\nNo stamp here.\n");
    assert.deepEqual(
      (await failuresOf(root, 30)).map((f) => f.rule),
      ["probe-stamp-present"],
    );
  });
});

test("a row whose Result opens with 'Retired:' is exempt from age only", async () => {
  await withDocsRepo(async (root) => {
    writeLedger(root, 6, "2026-07-01", {
      result: "Retired: surface dropped. No further reviews.",
    });
    assert.deepEqual(await failuresOf(root, 30), []);
    for (const result of [
      "No drift.",
      "Not Retired: still shipping.",
      "Retired the old probe claim. No drift.",
      "retired: lower case",
    ]) {
      writeLedger(root, 6, "2026-07-01", { result });
      assertOnly(await failuresOf(root, 30), "review-age", "codex.md");
    }
    // A four-column row has no Result cell, so it is never retired.
    writeLedger(root, 4, "2026-07-01");
    assertOnly(await failuresOf(root, 30), "review-age", "codex.md");
  });
});

test("a retired row still has its date validated", async () => {
  await withDocsRepo(async (root) => {
    const retired = "Retired: surface dropped. No further reviews.";
    writeLedger(root, 6, "2026-09-20", { result: retired });
    assertOnly(await failuresOf(root, 30), "review-date-future", "codex.md");
    // With the age check off, only the structural date check can catch an invalid stamp.
    writeLedger(root, 6, "not-a-date", { result: retired });
    assertOnly(await failuresOf(root, -1), "ledger-row-date", "codex.md");
  });
});

test("every row of a doc is date-checked, so a valid duplicate cannot hide a bad one", async () => {
  for (const columns of [4, 6] as const) {
    await withDocsRepo(async (root) => {
      writeLedger(root, columns, "2026-09-12", { badDuplicate: true });
      const failures = await failuresOf(root);
      assertOnly(failures, "ledger-row-date", "codex.md");
      assert.match(failures[0]?.message ?? "", /not-a-date/);
    });
  }
});

test("a row with the wrong number of columns is an invalid date, not a crash", async () => {
  await withDocsRepo(async (root) => {
    writeFile(
      root,
      LEDGER_REL,
      [
        ...LEDGER_HEADER,
        "| claude-code | [claude-code.md](claude-code.md) | [docs](https://example.com) | 2026-08-09 | extra |",
        ledgerRow("codex", "2026-08-01"),
        ledgerRow("copilot-cli", "2026-08-09"),
        ledgerRow("cursor", "2026-08-09"),
        "",
      ].join("\n"),
    );
    const failures = await failuresOf(root);
    assertOnly(failures, "ledger-row-date", "claude-code.md");
    assert.match(failures[0]?.message ?? "", /four or six columns/);
  });
});

test("a row whose last field is not closed by a pipe is an invalid date", async () => {
  await withDocsRepo(async (root) => {
    writeFile(
      root,
      LEDGER_REL,
      [
        ...LEDGER_HEADER,
        ledgerRow("claude-code", "2026-08-09"),
        // Six fields, as a four-column row splits into, but the last holds text instead of closing the row.
        "| codex | [codex.md](codex.md) | [docs](https://example.com/codex) | 2026-08-01 | trailing",
        ledgerRow("copilot-cli", "2026-08-09"),
        ledgerRow("cursor", "2026-08-09"),
        "",
      ].join("\n"),
    );
    const failures = await failuresOf(root);
    assertOnly(failures, "ledger-row-date", "codex.md");
    assert.equal(
      failures[0]?.message,
      "ledger row for codex.md has invalid review date: ledger row must have four or six columns",
    );
  });
});

test("a README, a directory and a non-markdown file under docs/surfaces are not surface docs", async () => {
  await withDocsRepo(async (root) => {
    writeFile(root, "docs/surfaces/notes.txt", "x");
    writeFile(root, "docs/surfaces/archive/old.md", "no stamp");
    assert.deepEqual(await failuresOf(root), []);
  });
});

/** The audit result with the age budget written in the environment as `knob`, as a maintainer sets it. */
const auditWithKnob = (root: string, knob?: string): Promise<AuditResult> =>
  auditDocs(root, {
    env: knob === undefined ? {} : { [ENV_MAX_AGE_DAYS]: knob },
  });

/** Rewrites the fixture ledger with the first `from` replaced by `to`. */
function editLedger(root: string, from: string, to: string): void {
  writeFile(root, LEDGER_REL, readFixture(root, LEDGER_REL).replace(from, to));
}

/** The four `review-age` failures of the fixture ledger at NOW under `budget`. */
const fixtureStale = (budget: number): Failure[] =>
  (
    [
      ["claude-code", "2026-08-09", 35],
      ["codex", "2026-08-01", 43],
      ["copilot-cli", "2026-08-09", 35],
      ["cursor", "2026-08-09", 35],
    ] as const
  ).map(([name, date, days]) => ({
    rule: "review-age",
    path: LEDGER_REL,
    message: `${name}.md: last reviewed ${date} (${days} days ago; budget ${budget})`,
  }));

const invalidDate = (doc: string, reason: string): Failure => ({
  rule: "ledger-row-date",
  path: LEDGER_REL,
  message: `ledger row for ${doc} has invalid review date: ${reason}`,
});

test("a surface doc with no ledger row", async () => {
  await withDocsRepo(async (root) => {
    writeFile(root, "docs/surfaces/vscode.md", `# VS Code${STAMP}`);
    assert.deepEqual(await auditWithKnob(root), {
      failures: [
        {
          rule: "ledger-row-present",
          path: LEDGER_REL,
          message: "no ledger row for docs/surfaces/vscode.md",
        },
      ],
    });
  });
});

test("an invalid review date", async () => {
  await withDocsRepo(async (root) => {
    editLedger(root, "| 2026-08-01 |", "| recently |");
    assert.deepEqual(await auditWithKnob(root), {
      failures: [
        invalidDate("codex.md", 'invalid Last reviewed value "recently"'),
      ],
    });
  });
});

test("an impossible review date", async () => {
  await withDocsRepo(async (root) => {
    editLedger(root, "| 2026-08-01 |", "| 2026-02-30 |");
    assert.deepEqual(await auditWithKnob(root), {
      failures: [
        invalidDate("codex.md", 'invalid Last reviewed value "2026-02-30"'),
      ],
    });
  });
});

test("a review date wrapped in a byte order mark", async () => {
  await withDocsRepo(async (root) => {
    // A cell is trimmed as String.prototype.trim trims, and that trims U+FEFF, so the date reads as valid.
    editLedger(root, "| 2026-08-01 |", "| \u{feff}2026-08-01 |");
    assert.deepEqual(await auditWithKnob(root), { failures: [] });
  });
});

test("a row with no https source", async () => {
  await withDocsRepo(async (root) => {
    editLedger(root, "[docs](https://example.com/codex)", "none pinned");
    assert.deepEqual(await auditWithKnob(root), {
      failures: [
        {
          rule: "ledger-row-source",
          path: LEDGER_REL,
          message:
            "ledger row for codex.md pins no upstream source (https:// link)",
        },
      ],
    });
  });
});

const NO_STAMP: Failure = {
  rule: "probe-stamp-present",
  path: "docs/surfaces/codex.md",
  message:
    "carries no inline 'Verified on <CLI> <version> (YYYY-MM-DD)' probe stamp",
};

test("a surface doc with no probe stamp", async () => {
  await withDocsRepo(async (root) => {
    writeFile(root, "docs/surfaces/codex.md", "# Codex\n");
    assert.deepEqual(await auditWithKnob(root), { failures: [NO_STAMP] });
  });
});

test("a ledger row for a doc that is gone", async () => {
  await withDocsRepo(async (root) => {
    writeFile(
      root,
      LEDGER_REL,
      `${readFixture(root, LEDGER_REL)}${ledgerRow("vscode", "2026-08-09")}\n`,
    );
    assert.deepEqual(await auditWithKnob(root), {
      failures: [
        {
          rule: "ledger-doc-exists",
          path: LEDGER_REL,
          message:
            "ledger row references missing docs/surfaces/vscode.md (renamed or removed doc?)",
        },
      ],
    });
  });
});

test("a row with the wrong number of columns", async () => {
  await withDocsRepo(async (root) => {
    editLedger(root, "| 2026-08-09 |\n| codex", "| 2026-08-09 | x |\n| codex");
    assert.deepEqual(await auditWithKnob(root), {
      failures: [
        invalidDate(
          "claude-code.md",
          "ledger row must have four or six columns",
        ),
      ],
    });
  });
});

test("stale reviews with the age check armed", async () => {
  await withDocsRepo(async (root) => {
    assert.deepEqual(await auditWithKnob(root, "1"), {
      failures: fixtureStale(1),
    });
  });
});

test("a review date in the future with the age check armed", async () => {
  await withDocsRepo(async (root) => {
    editLedger(root, "| 2026-08-01 |", "| 2099-01-01 |");
    assert.deepEqual(await auditWithKnob(root, "36500"), {
      failures: [
        {
          rule: "review-date-future",
          path: LEDGER_REL,
          message:
            "codex.md: review date 2099-01-01 is in the future (invalid stamp)",
        },
      ],
    });
  });
});

test("a six-column row that only starts with the word Retired is not exempt", async () => {
  await withDocsRepo(async (root) => {
    writeFile(
      root,
      LEDGER_REL,
      sixColumnLedger("2000-01-01", "Retired the old probe claim."),
    );
    assert.deepEqual(await auditWithKnob(root, "3650"), {
      failures: [
        {
          rule: "review-age",
          path: LEDGER_REL,
          message:
            "codex.md: last reviewed 2000-01-01 (9752 days ago; budget 3650)",
        },
      ],
    });
  });
});

test("a retired row with an invalid date", async () => {
  await withDocsRepo(async (root) => {
    writeFile(
      root,
      LEDGER_REL,
      sixColumnLedger("not-a-date", "Retired: dropped."),
    );
    assert.deepEqual(await auditWithKnob(root), {
      failures: [
        invalidDate("codex.md", 'invalid Last reviewed value "not-a-date"'),
      ],
    });
  });
});

test("a missing ledger", async () => {
  await withDocsRepo(async (root) => {
    rmSync(path.join(root, ...LEDGER_REL.split("/")));
    assert.deepEqual(await auditWithKnob(root), {
      failures: [
        {
          rule: "ledger-present",
          path: LEDGER_REL,
          message: `${LEDGER_REL} (upstream review ledger) is missing`,
        },
      ],
    });
  });
});

test('the age knob written as "007"', async () => {
  await withDocsRepo(async (root) => {
    assert.deepEqual(await auditWithKnob(root, "007"), {
      failures: fixtureStale(7),
    });
  });
});

test('the age knob written as "-0"', async () => {
  await withDocsRepo(async (root) => {
    // Negative zero is zero, which arms the check, not a negative budget, which turns it off.
    assert.deepEqual(await auditWithKnob(root, "-0"), {
      failures: fixtureStale(0),
    });
  });
});

/** The exit code of the conformance CLI auditing `root` for docs at NOW with an age budget of one day. */
async function staleExit(root: string, flagged: boolean): Promise<number> {
  const host = fakeHost({ env: { [ENV_MAX_AGE_DAYS]: "1" } });
  const argv = ["docs", "--repo", root];
  if (flagged) argv.push("--stale-review-exit-code");
  return main(argv, host, {
    docs: (repo, options) => run(repo, { ...options, now: () => NOW }),
  });
}

test("stale reviews flagged", async () => {
  await withDocsRepo(async (root) => {
    assert.equal(await staleExit(root, true), STALE_REVIEW_EXIT);
    assert.deepEqual(await auditWithKnob(root, "1"), {
      failures: fixtureStale(1),
    });
  });
});

test("stale reviews plain", async () => {
  await withDocsRepo(async (root) => {
    assert.equal(await staleExit(root, false), 1);
    assert.deepEqual(await auditWithKnob(root, "1"), {
      failures: fixtureStale(1),
    });
  });
});

test("stale reviews mixed", async () => {
  await withDocsRepo(async (root) => {
    // A structural failure suppresses the age check, so the run is not stale-only and the flag keeps exit 1.
    writeFile(root, "docs/surfaces/codex.md", "# Codex\n");
    assert.equal(await staleExit(root, true), 1);
    assert.deepEqual(await auditWithKnob(root, "1"), { failures: [NO_STAMP] });
  });
});

test("repository ledger contains an antigravity row pinning all four official Antigravity doc URLs", () => {
  const repoRoot = path.resolve(import.meta.dirname, "../../../../../..");
  const ledger = readFileSync(
    path.join(repoRoot, ...LEDGER_REL.split("/")),
    "utf8",
  );
  const rows = ledger
    .split("\n")
    .filter(
      (line) => line.startsWith("|") && line.includes("](antigravity.md)"),
    );
  assert.equal(
    rows.length,
    1,
    `expected exactly one antigravity ledger row in ${LEDGER_REL}, found ${rows.length}`,
  );
  const row = rows[0] ?? "";
  const cells = row
    .trim()
    .split("|")
    .map((cell) => cell.trim());
  assert.equal(cells[1], "antigravity");
  for (const url of [
    "https://antigravity.google/docs/plugins",
    "https://antigravity.google/docs/build-plugins",
    "https://antigravity.google/docs/marketplace",
    "https://antigravity.google/docs/hooks",
  ]) {
    assert.ok(row.includes(url), `antigravity ledger row must pin ${url}`);
  }
});
