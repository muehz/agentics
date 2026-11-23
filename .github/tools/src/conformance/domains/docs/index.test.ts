import assert from "node:assert/strict";
import { rmSync } from "node:fs";
import path from "node:path";
import { test } from "node:test";
import type { AuditResult, RunOptions } from "../../check.ts";
import { LEDGER, sixColumnLedger } from "../../testing/docs-seeds.ts";
import {
  auditDocs,
  PIN_ROW,
  PINS,
  readFixture,
  withDocsRepo,
  writeFile,
} from "../../testing/docs-repo.ts";
import { LADDER_PATH, PIN_PATH } from "./ladder.ts";
import { ENV_MAX_AGE_DAYS, run } from "./index.ts";

const NOW = () => new Date("2026-10-01T12:00:00Z");

const options = (env: Record<string, string | undefined> = {}) => ({
  short: false,
  fast: false,
  diagnostics: () => {},
  env,
  now: NOW,
});

test("the fixture passes, and an area's defect surfaces through run in the area's order", async () => {
  await withDocsRepo(async (root) => {
    assert.deepEqual(await run(root, options()), { failures: [] });
    // One defect per area: a drifted roster count, an unstamped doc, and an unpinned identifier.
    writeFile(
      root,
      "docs/surfaces/copilot-cli.md",
      "The manifest registers all 9 Agentics skills.\n",
    );
    writeFile(root, "docs/surfaces/cursor.md", "# Cursor\n\nNo stamp here.\n");
    writeFile(root, PIN_PATH, PINS.replace(PIN_ROW, ""));
    const result = await run(root, options());
    assert.equal(result.error, undefined);
    assert.deepEqual(
      result.failures.map((f) => f.rule),
      [
        "roster-count-drift",
        "probe-stamp-present",
        "probe-stamp-present",
        "model-ids-known",
        "model-ids-known",
      ],
    );
  });
});

test("the max-age knob arms the age check from the environment", async () => {
  await withDocsRepo(async (root) => {
    // The fixture's rows are 53 to 61 days old at NOW.
    assert.deepEqual(await run(root, options({ [ENV_MAX_AGE_DAYS]: "" })), {
      failures: [],
    });
    assert.deepEqual(
      await run(root, options({ [ENV_MAX_AGE_DAYS]: "36500" })),
      { failures: [] },
    );
    const stale = await run(root, options({ [ENV_MAX_AGE_DAYS]: "30" }));
    assert.deepEqual(
      stale.failures.map((f) => f.rule),
      ["review-age", "review-age", "review-age", "review-age"],
    );
    // A negative knob is valid and disarms the check, as an unset one does.
    assert.deepEqual(await run(root, options({ [ENV_MAX_AGE_DAYS]: "-1" })), {
      failures: [],
    });
    assert.deepEqual(
      await run(root, options({ [ENV_MAX_AGE_DAYS]: "+36500" })),
      { failures: [] },
    );
  });
});

test("a max-age knob that is not an integer is an audit error before any area runs", async () => {
  await withDocsRepo(async (root) => {
    // The roster is wrong too, to show that no area ran.
    writeFile(
      root,
      "docs/surfaces/copilot-cli.md",
      "The manifest registers all 9 Agentics skills.\n",
    );
    for (const raw of [
      "not-a-number",
      "1.5",
      "30 ",
      " 30",
      "1e3",
      "0x10",
      "99999999999999999999",
    ]) {
      const result = await run(root, options({ [ENV_MAX_AGE_DAYS]: raw }));
      assert.deepEqual(result.failures, [], raw);
      assert.match(
        result.error ?? "",
        /docs: AGX_LEDGER_MAX_AGE_DAYS=.* is not an integer/,
        raw,
      );
    }
  });
});

test("an audit error ends the run but keeps the failures found before it", async () => {
  await withDocsRepo(async (root) => {
    writeFile(
      root,
      "docs/surfaces/copilot-cli.md",
      "The manifest registers all 9 Agentics skills.\n",
    );
    for (const name of ["claude-code", "codex", "copilot-cli", "cursor"])
      rmSync(path.join(root, "docs", "surfaces", `${name}.md`));
    const result = await run(root, options());
    assert.match(
      result.error ?? "",
      /no surface docs found under docs\/surfaces/,
    );
    // The roster area ran first, so its failures survive; the model-ids area never ran.
    assert.deepEqual(
      result.failures.map((f) => f.rule),
      ["roster-claim-site-present", "roster-claim-site-present"],
    );
  });
});

test("run reads the clock only through the seam", async () => {
  await withDocsRepo(async (root) => {
    let reads = 0;
    const result = await run(root, {
      ...options({ [ENV_MAX_AGE_DAYS]: "30" }),
      now: () => {
        reads += 1;
        return new Date("2026-08-15T12:00:00Z");
      },
    } satisfies RunOptions & { now: () => Date });
    assert.deepEqual(result, { failures: [] });
    assert.equal(reads, 1);
  });
});

/** The docs audit of a fresh fixture after `mutate`, run with `env` on top of a disarmed age check. */
function auditMutated(
  mutate: (root: string) => void,
  env: Record<string, string> = {},
): Promise<AuditResult> {
  return withDocsRepo((root) => {
    mutate(root);
    return auditDocs(root, { env });
  });
}

const CLEAN: AuditResult = { failures: [] };

/** The result of an audit that stops with `error` before reporting any failure. */
const auditError = (error: string): AuditResult => ({ failures: [], error });

const writeCopilotManifest = (text: string) => (root: string) =>
  writeFile(root, ".github/plugin/plugin.json", text);

const writeRuneFile = (rune: string) => (root: string) =>
  writeFile(root, "hooks/go/rune.go", `package x\n\nconst a = ${rune}\n`);

test("a clean fixture", async () => {
  assert.deepEqual(await auditMutated(() => {}), CLEAN);
});

test("a clean fixture with a generous age budget", async () => {
  assert.deepEqual(
    await auditMutated(() => {}, { [ENV_MAX_AGE_DAYS]: "36500" }),
    CLEAN,
  );
});

test("a CRLF surface doc keeps its probe stamp", async () => {
  assert.deepEqual(
    await auditMutated((root) =>
      writeFile(
        root,
        "docs/surfaces/cursor.md",
        "# Cursor\r\n\r\nVerified on Cursor 3.15.6 (2026-08-09).\r\n",
      ),
    ),
    CLEAN,
  );
});

test("a review date padded with NEL is not a valid date", async () => {
  // A ledger cell is trimmed of spaces but keeps NEL, so the padded cell is not a date.
  assert.deepEqual(
    await auditMutated((root) =>
      writeFile(
        root,
        LEDGER,
        readFixture(root, LEDGER).replace(
          "| 2026-08-01 |",
          "|\u0085 2026-08-01\u0085 |",
        ),
      ),
    ),
    {
      failures: [
        {
          rule: "ledger-row-date",
          path: LEDGER,
          message:
            'ledger row for codex.md has invalid review date: invalid Last reviewed value "\u0085 2026-08-01\u0085"',
        },
      ],
    },
  );
});

test("a retired six-column row is exempt from age", async () => {
  assert.deepEqual(
    await auditMutated(
      (root) =>
        writeFile(
          root,
          LEDGER,
          sixColumnLedger("2000-01-01", "Retired: dropped."),
        ),
      { [ENV_MAX_AGE_DAYS]: "3650" },
    ),
    CLEAN,
  );
});

test("a roster claim wrapped across lines", async () => {
  assert.deepEqual(
    await auditMutated((root) =>
      writeFile(
        root,
        "docs/ARCHITECTURE.md",
        readFixture(root, "docs/ARCHITECTURE.md").replace(
          "canonical 4-skill tree",
          "canonical\r\n\t4-skill\ntree",
        ),
      ),
    ),
    CLEAN,
  );
});

test("a Skills key in the Copilot manifest", async () => {
  // Only the lower-case key names the roster, so this manifest registers no skill.
  assert.deepEqual(
    await auditMutated(
      writeCopilotManifest(
        JSON.stringify({ Skills: ["skills/alpha", "skills/beta"] }),
      ),
    ),
    auditError("docs: cannot derive roster counts from an empty roster"),
  );
});

test("a SKILLS key in the Copilot manifest", async () => {
  assert.deepEqual(
    await auditMutated(
      writeCopilotManifest(
        JSON.stringify({ SKILLS: ["skills/alpha", "skills/beta"] }),
      ),
    ),
    auditError("docs: cannot derive roster counts from an empty roster"),
  );
});

test("duplicate matching manifest keys preserve source order", async () => {
  // The last "skills" key wins, so the manifest registers the two skills the claims count.
  assert.deepEqual(
    await auditMutated(
      writeCopilotManifest(
        '{"skills":["a"],"SKILLS":["a","b","c"],"skills":["a","b"]}',
      ),
    ),
    CLEAN,
  );
});

test('the age knob written as "-5"', async () => {
  assert.deepEqual(
    await auditMutated(() => {}, { [ENV_MAX_AGE_DAYS]: "-5" }),
    CLEAN,
  );
});

test('the age knob written as "+36500"', async () => {
  assert.deepEqual(
    await auditMutated(() => {}, { [ENV_MAX_AGE_DAYS]: "+36500" }),
    CLEAN,
  );
});

test("a max-age knob that is not an integer", async () => {
  assert.deepEqual(
    await auditMutated(() => {}, { [ENV_MAX_AGE_DAYS]: "soon" }),
    auditError('docs: AGX_LEDGER_MAX_AGE_DAYS="soon" is not an integer'),
  );
});

test('an age knob written as " 7"', async () => {
  assert.deepEqual(
    await auditMutated(() => {}, { [ENV_MAX_AGE_DAYS]: " 7" }),
    auditError('docs: AGX_LEDGER_MAX_AGE_DAYS=" 7" is not an integer'),
  );
});

test('an age knob written as "7 "', async () => {
  assert.deepEqual(
    await auditMutated(() => {}, { [ENV_MAX_AGE_DAYS]: "7 " }),
    auditError('docs: AGX_LEDGER_MAX_AGE_DAYS="7 " is not an integer'),
  );
});

test('an age knob written as "1_0"', async () => {
  assert.deepEqual(
    await auditMutated(() => {}, { [ENV_MAX_AGE_DAYS]: "1_0" }),
    auditError('docs: AGX_LEDGER_MAX_AGE_DAYS="1_0" is not an integer'),
  );
});

test('an age knob written as "0x10"', async () => {
  assert.deepEqual(
    await auditMutated(() => {}, { [ENV_MAX_AGE_DAYS]: "0x10" }),
    auditError('docs: AGX_LEDGER_MAX_AGE_DAYS="0x10" is not an integer'),
  );
});

test('an age knob written as "1e3"', async () => {
  assert.deepEqual(
    await auditMutated(() => {}, { [ENV_MAX_AGE_DAYS]: "1e3" }),
    auditError('docs: AGX_LEDGER_MAX_AGE_DAYS="1e3" is not an integer'),
  );
});

test('an age knob written as "9223372036854775808"', async () => {
  // One past the largest signed 64-bit integer.
  assert.deepEqual(
    await auditMutated(() => {}, {
      [ENV_MAX_AGE_DAYS]: "9223372036854775808",
    }),
    auditError(
      'docs: AGX_LEDGER_MAX_AGE_DAYS="9223372036854775808" is not an integer',
    ),
  );
});

test("an invalid earlier matching manifest key", async () => {
  // Only the lower-case key is read, so the earlier mistyped one cannot break the roster.
  assert.deepEqual(
    await auditMutated(
      writeCopilotManifest('{"Skills":42,"skills":["a","b"]}'),
    ),
    CLEAN,
  );
});

test("no surface docs", async () => {
  const result = await auditMutated((root) => {
    for (const name of ["claude-code", "codex", "copilot-cli", "cursor"])
      rmSync(path.join(root, "docs", "surfaces", `${name}.md`));
  });
  assert.deepEqual(result, {
    failures: [
      {
        rule: "roster-claim-site-present",
        path: "docs/surfaces/claude-code.md",
        message:
          "Claude manifest registration cannot be audited because the file is missing",
      },
      {
        rule: "roster-claim-site-present",
        path: "docs/surfaces/copilot-cli.md",
        message:
          "Copilot manifest registration cannot be audited because the file is missing",
      },
    ],
    error: "docs: no surface docs found under docs/surfaces — nothing to audit",
  });
});

test("no ladder table", async () => {
  assert.deepEqual(
    await auditMutated((root) => writeFile(root, LADDER_PATH, "# Ladders\n")),
    auditError(
      `docs: modelids: ${LADDER_PATH} has 0 tables headed "Surface | Lower to higher model position", want 1`,
    ),
  );
});

test("no pin table", async () => {
  assert.deepEqual(
    await auditMutated((root) => writeFile(root, PIN_PATH, "# Pins\n")),
    auditError(
      `docs: modelids: ${PIN_PATH} has 0 tables headed "Identifier | Used in | Reason", want 1`,
    ),
  );
});

test("no skills directory", async () => {
  assert.deepEqual(
    await auditMutated((root) =>
      rmSync(path.join(root, "skills"), { recursive: true }),
    ),
    auditError("docs: skills: does not exist"),
  );
});

test("a Copilot manifest that is not JSON", async () => {
  assert.deepEqual(
    await auditMutated(writeCopilotManifest("{")),
    auditError("docs: .github/plugin/plugin.json: is not valid JSON"),
  );
});

test("a Copilot manifest with no skills", async () => {
  assert.deepEqual(
    await auditMutated(writeCopilotManifest("{}")),
    auditError("docs: cannot derive roster counts from an empty roster"),
  );
});

test("a Go file with an unterminated literal", async () => {
  assert.deepEqual(
    await auditMutated((root) =>
      writeFile(root, "hooks/go/broken.go", 'package x\n\nconst a = "open\n'),
    ),
    auditError(
      "docs: scanning hooks/go: hooks/go/broken.go: string literal not terminated",
    ),
  );
});

test("a Go file with a rune literal holding an unknown escape", async () => {
  assert.deepEqual(
    await auditMutated(writeRuneFile("'\\q'")),
    auditError(
      "docs: scanning hooks/go: hooks/go/rune.go: unknown escape sequence",
    ),
  );
});

test("a Go file with a rune literal holding an escaped double quote", async () => {
  assert.deepEqual(
    await auditMutated(writeRuneFile("'\\\"'")),
    auditError(
      "docs: scanning hooks/go: hooks/go/rune.go: unknown escape sequence",
    ),
  );
});

test("a Go file with a rune literal holding a short hex escape", async () => {
  assert.deepEqual(
    await auditMutated(writeRuneFile("'\\x4'")),
    auditError(
      "docs: scanning hooks/go: hooks/go/rune.go: escape sequence is incomplete",
    ),
  );
});
