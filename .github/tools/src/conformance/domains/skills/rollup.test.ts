import assert from "node:assert/strict";
import {
  mkdtempSync,
  readdirSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { test } from "node:test";
import { fileURLToPath } from "node:url";
import { RepoFileError } from "../../repo-file-error.ts";
import type { AuditResult, Failure } from "../../check.ts";
import {
  BREAKERS,
  link,
  mkdir,
  mutateFile,
  remove,
  rollupMappingMD,
  VALID_ROLLUP as FIXTURE_VALID_ROLLUP,
  withSkillsRepo,
  writeFile,
} from "../../testing/skills-repo.ts";
import * as seeds from "../../testing/skills-seeds.ts";
import {
  auditEveryFamily,
  auditSlice,
  NEEDS_SYMLINK,
  sliceNamed,
} from "../../testing/skills-seeds.ts";
import { withSeams } from "./files.ts";
import type {} from "./index.ts";
import {
  ATTESTATION_FOOTER,
  checkRollupBody,
  ROLLUP_SHAPE_RULE,
  rollupShapeFailures,
} from "./rollup.ts";

const EM = String.fromCodePoint(0x2014);
const DOT = String.fromCodePoint(0x00b7);

const TEMPLATE = "skills/triage/references/rollup-template.md";
const MAX_INT64 = "9223372036854775807";

const HEADER = `@jdoe ${EM} 6 fixed ${DOT} 1 needs you`;
const VERDICT = `6 fixed ${DOT} 1 needs you`;
const NEEDS_HEADING = `### Needs you`;
const BULLET = `- **Checks ${EM} Test / Go (xcode-27, tests):** fails non-deterministically. [Failed job](https://example.invalid/job/1)`;
const COLUMNS =
  "| Source | Status | Found | Fixed | Accepted | Dismissed | Needs you |";
const SEPARATOR =
  "| ------ | ------ | ----- | ----- | -------- | --------- | --------- |";
const COPILOT_ROW = "| Copilot Review | Completed | 6 | 5 | 1 | 0 | 0 |";
const CHECKS_ROW = `| Checks | Partial | 2 | 1 | ${EM} | ${EM} | 1 |`;
const TOTAL_ROW = "| **Total** | | **8** | **6** | **1** | **0** | **1** |";
const TABLE = [COLUMNS, SEPARATOR, COPILOT_ROW, CHECKS_ROW, TOTAL_ROW].join(
  "\n",
);
const BASE_SYNC_BLOCK = `#### Base sync

<!-- agentics:triage:section:base-sync -->
Up to date ${EM} \`main\` was already an ancestor of the PR head.
<!-- agentics:triage:section-end:base-sync -->

`;
const DETAILS_OPEN = "<details><summary>Details</summary>";
const PENDING_BULLET = `- **Human Feedback:** not handled ${EM} replies here need someone to approve them, and no one was present.`;
const PENDING_SENTENCE = `Not handled ${EM} no one was present to approve replies, so nothing was drafted or posted.`;
const PENDING_ROW = `| Human Feedback | Pending Review | ${EM} | ${EM} | ${EM} | ${EM} | ${EM} |`;
const SKIPPED_ROW = `| Copilot Review | Skipped | ${EM} | ${EM} | ${EM} | ${EM} | ${EM} |`;

/** The base valid rollup: the mixed configuration, two sources, one item needing you, base sync inside Details. */
const VALID_ROLLUP = `<!-- agentics:triage:rollup -->
<!-- agentics:triage:execution:1790000000-12345 -->

${HEADER}

${NEEDS_HEADING}

${BULLET}

${TABLE}

${DETAILS_OPEN}

${BASE_SYNC_BLOCK}#### Copilot Review

- **Fixed (5):** \`8bf374e5\`
- **Accepted (1):** \`api/cors.go:14\` ${EM} deliberate.

<sub>6 replies posted</sub>

#### Checks

- **Fixed (1):** Format drift ${EM} \`a1b2c3d4\`
- **Needs you (1):** Test / Go (xcode-27, tests)

</details>

${ATTESTATION_FOOTER}`;

/** `body` with its first `from` replaced by `to`, throwing when `from` is absent so a case cannot pass on a no-op. */
function swap(body: string, from: string, to: string): string {
  const at = body.indexOf(from);
  if (at < 0)
    throw new Error(`mutation target ${JSON.stringify(from)} not found`);
  return body.slice(0, at) + to + body.slice(at + from.length);
}

/** `body` with the text from the anchor `from` up to the anchor `to` removed, throwing when either is absent. */
function cut(body: string, from: string, to: string): string {
  const i = body.indexOf(from);
  const j = body.indexOf(to);
  if (i < 0 || j < i) throw new Error(`cut anchors ${from}..${to} not found`);
  return body.slice(0, i) + body.slice(j);
}

/** The message of a verdict that does not parse, for the header line `line`. */
const verdictMessage = (line: string): string =>
  `header verdict is not \`N fixed ${DOT} M needs you\` or \`Nothing needs you\`: ${line}`;

/** The base body with a Pending Review source added, its count in the header and its bullet and section in place. */
function pendingBody(): string {
  let body = swap(VALID_ROLLUP, CHECKS_ROW, `${CHECKS_ROW}\n${PENDING_ROW}`);
  body = swap(body, `6 fixed ${DOT} 1 needs you`, `6 fixed ${DOT} 2 needs you`);
  body = swap(
    body,
    "[Failed job](https://example.invalid/job/1)\n",
    `[Failed job](https://example.invalid/job/1)\n${PENDING_BULLET}\n`,
  );
  return swap(
    body,
    "</details>\n\n> _Triaged",
    `#### Human Feedback\n\n${PENDING_SENTENCE}\n\n</details>\n\n> _Triaged`,
  );
}

/** A blocked base sync body, with the table rows given. */
function blockedBody(rows: string[], header = `1 needs you`): string {
  const table =
    rows.length === 0 ? "" : [COLUMNS, SEPARATOR, ...rows].join("\n") + "\n\n";
  return `<!-- agentics:triage:rollup -->\n<!-- agentics:triage:execution:1790000000-12345 -->\n\n@jdoe ${EM} ${header}\n\n<!-- agentics:triage:section:base-sync -->\n**Base sync ${EM} blocked (run 2026-10-02)**\n\nMerge stopped.\n<!-- agentics:triage:section-end:base-sync -->\n\n${table}${ATTESTATION_FOOTER}`;
}

type Case = { name: string; body: string; want: string[] };

function assertCases(cases: Case[]): void {
  for (const one of cases)
    assert.deepEqual(checkRollupBody(one.body), one.want, one.name);
}

// The base body here and the one the shared fixture writes must agree, so a case here also holds for the fixture.
test("the base body is the valid rollup the shared fixture holds", () => {
  assert.equal(VALID_ROLLUP, FIXTURE_VALID_ROLLUP);
  assert.equal(ATTESTATION_FOOTER, "> _Triaged by an AI agent_");
});

// --- the fixtures ---------------------------------------------------------------------------------------------------

const FIXTURES = path.join(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
  "..",
  "testing",
  "fixtures",
  "skill-rollups",
);

function fixtureNames(kind: "valid" | "invalid"): string[] {
  return readdirSync(path.join(FIXTURES, kind))
    .filter((name) => name.endsWith(".md"))
    .sort();
}

test("the fixture directories hold the 21 valid and 16 invalid rollup fixtures", () => {
  const valid = fixtureNames("valid");
  const invalid = fixtureNames("invalid");
  assert.ok(
    valid.length >= 16 && invalid.length >= 6,
    `found ${valid.length} valid and ${invalid.length} invalid fixtures, want at least 16 and 6`,
  );
  assert.equal(valid.length, 21);
  assert.equal(invalid.length, 16);
});

test("every valid fixture yields no message", () => {
  for (const name of fixtureNames("valid")) {
    const text = readFileSync(path.join(FIXTURES, "valid", name), "utf8");
    assert.deepEqual(checkRollupBody(text), [], name);
  }
});

test("every invalid fixture yields exactly one message, the one its first line names", () => {
  for (const name of fixtureNames("invalid")) {
    const text = readFileSync(path.join(FIXTURES, "invalid", name), "utf8");
    const normalized = text.replaceAll("\r\n", "\n");
    const cutAt = normalized.indexOf("\n");
    const first = cutAt < 0 ? normalized : normalized.slice(0, cutAt);
    const body = cutAt < 0 ? "" : normalized.slice(cutAt + 1);
    assert.ok(first.startsWith("want: "), `${name}: first line is ${first}`);
    const want = first.slice("want: ".length);
    assert.notEqual(want.trim(), "", `${name}: blank substring`);
    const msgs = checkRollupBody(body);
    assert.equal(msgs.length, 1, `${name}: ${JSON.stringify(msgs)}`);
    assert.ok(msgs[0]?.includes(want), `${name}: ${JSON.stringify(msgs)}`);
  }
});

// --- the shape checks, one group of violations at a time ------------------------------------------------------------

test("the valid rollup, the detection-notes placement and the pending-review body report nothing", () => {
  assert.deepEqual(checkRollupBody(VALID_ROLLUP), []);
  const notes = swap(
    VALID_ROLLUP,
    "#### Copilot Review\n",
    "#### Detection notes\n\n- One bot has no adapter.\n\n#### Copilot Review\n",
  );
  assert.deepEqual(checkRollupBody(notes), []);
  assert.deepEqual(checkRollupBody(pendingBody()), []);
});

test("each single-replacement break reports the message of the check it breaks", () => {
  const rejects = (from: string, to: string): string =>
    swap(VALID_ROLLUP, from, to);
  assertCases([
    {
      name: "verdict grammar",
      body: rejects(VERDICT, "**6 replies posted** (bot findings)"),
      want: [
        verdictMessage("@jdoe " + EM + " **6 replies posted** (bot findings)"),
      ],
    },
    {
      name: "fixed count",
      body: rejects(`6 fixed ${DOT}`, `7 fixed ${DOT}`),
      want: [
        "header says 7 fixed, but the table's landed Fixed cells sum to 6",
      ],
    },
    {
      name: "needs count",
      body: rejects(
        CHECKS_ROW,
        `| Checks | Partial | 2 | 2 | ${EM} | ${EM} | 0 |`,
      ),
      want: [
        "Total Fixed is 6, rows sum to 7",
        "Total Needs you is 1, rows sum to 0",
        "header says 6 fixed, but the table's landed Fixed cells sum to 7",
        "header says 1 needs you, but the table implies 0",
      ],
    },
    {
      name: "bullet count",
      body: rejects(BULLET + "\n", ""),
      want: ["header says 1 needs you, but the needs-you list has 0 bullets"],
    },
    {
      name: "status vocabulary",
      body: rejects("| Checks | Partial |", "| Checks | Complete |"),
      want: [
        'row "Checks" has status "Complete", outside the eight statuses',
        "header says 6 fixed, but the table's landed Fixed cells sum to 5",
      ],
    },
    {
      name: "row balance",
      body: rejects(
        COPILOT_ROW,
        "| Copilot Review | Completed | 6 | 5 | 0 | 0 | 0 |",
      ),
      want: [
        'row "Copilot Review" does not balance: Fixed + Accepted + Dismissed + Needs you != Found',
        "Total Accepted is 1, rows sum to 0",
      ],
    },
    {
      name: "total sum",
      body: rejects("| **Total** | | **8** |", "| **Total** | | **9** |"),
      want: ["Total Found is 9, rows sum to 8"],
    },
    {
      name: "total required",
      body: rejects(TOTAL_ROW + "\n", ""),
      want: ["two or more rows need a Total row"],
    },
    {
      name: "columns",
      body: rejects(
        "| Source | Status | Found |",
        "| Source | Status | Findings |",
      ),
      want: [
        "table columns are [Source Status Findings Fixed Accepted Dismissed Needs you], want [Source Status Found Fixed Accepted Dismissed Needs you]",
        "header says 6 fixed, but the table's landed Fixed cells sum to 0",
        "header says 1 needs you, but the table implies 0",
      ],
    },
    {
      name: "drafts",
      body: rejects(
        "</details>\n\n> _Triaged",
        "</details>\n\n<!-- agentics:triage:drafts -->\n\n> _Triaged",
      ),
      want: ["carries a drafts marker or a Drafted Responses section"],
    },
    {
      name: "footer last",
      body: rejects(ATTESTATION_FOOTER, ATTESTATION_FOOTER + "\n\ntrailing"),
      want: ["the attestation footer is not the last line"],
    },
    {
      name: "details heading",
      body: rejects(
        `${DETAILS_OPEN}\n\n#### Base sync`,
        `${DETAILS_OPEN}\n\n**Base sync**`,
      ),
      want: [
        "Details does not open with a `####` heading",
        "base sync is not the first block in Details",
      ],
    },
    {
      name: "source heading",
      body: rejects("#### Checks\n", "**Checks**\n"),
      want: ["Details has no `#### Checks` heading inside the fold"],
    },
  ]);
});

test("a block out of the layout order is named, and the base sync above the table is outside Details", () => {
  const syncBlock = BASE_SYNC_BLOCK;
  let moved = swap(VALID_ROLLUP, syncBlock, "");
  moved = swap(
    moved,
    NEEDS_HEADING,
    `<!-- agentics:triage:section:base-sync -->\nUp to date.\n<!-- agentics:triage:section-end:base-sync -->\n\n${NEEDS_HEADING}`,
  );
  const swapped = swap(
    swap(VALID_ROLLUP, TABLE + "\n\n", ""),
    NEEDS_HEADING,
    TABLE + "\n\n" + NEEDS_HEADING,
  );
  assertCases([
    {
      name: "base sync above the table",
      body: moved,
      want: ["base sync renders outside Details; only a blocked sync may"],
    },
    {
      name: "table above needs-you",
      body: swapped,
      want: [
        "table appears before needs-you list; the layout order is header, link line, blocked base sync, supersedes, needs-you, table, Details",
      ],
    },
  ]);
});

test("a blocked base sync renders alone, with a needs-you header and no table", () => {
  const blocked = blockedBody([]);
  const withDetails = swap(
    blocked,
    "\n\n" + ATTESTATION_FOOTER,
    `\n\n${DETAILS_OPEN}\n\n#### Copilot Review\n\n</details>\n\n${ATTESTATION_FOOTER}`,
  );
  assertCases([
    { name: "valid blocked body", body: blocked, want: [] },
    {
      name: "Details",
      body: withDetails,
      want: [
        "a blocked base sync renders alone: no needs-you list and no Details",
      ],
    },
    {
      name: "a table of Skipped dash rows",
      body: blockedBody([SKIPPED_ROW]),
      want: ["a blocked rollup renders no table"],
    },
    {
      name: "a fixed count in the header",
      body: blockedBody([], `1 fixed ${DOT} 1 needs you`),
      want: ["a blocked base sync's header is `1 needs you`"],
    },
  ]);
});

test("an unattended Pending Review source is pinned to one bullet and one sentence", () => {
  const body = pendingBody();
  assertCases([
    {
      name: "a bullet that names a reviewer",
      body: swap(
        body,
        "no one was present.",
        "no one was present (mchen, 2 threads).",
      ),
      want: [
        "pending review bullet is not the pinned text: - **Human Feedback:** not handled " +
          EM +
          " replies here need someone to approve them, and no one was present (mchen, 2 threads).",
      ],
    },
    {
      name: "a Details sentence that names a reviewer",
      body: swap(
        body,
        PENDING_SENTENCE,
        `Not handled ${EM} mchen left two threads.`,
      ),
      want: [
        "pending review Details is not the pinned sentence: Not handled " +
          EM +
          " mchen left two threads.",
        "pending review is missing the pinned Details sentence",
      ],
    },
    {
      name: "a missing bullet",
      body: swap(
        body,
        PENDING_BULLET,
        "- Human feedback needs an interactive rerun.",
      ),
      want: ["pending review is missing the pinned needs-you bullet"],
    },
    {
      name: "a missing sentence",
      body: swap(body, PENDING_SENTENCE, ""),
      want: ["pending review is missing the pinned Details sentence"],
    },
    {
      name: "a duplicate bullet",
      body: swap(body, PENDING_BULLET, PENDING_BULLET + "\n" + PENDING_BULLET),
      want: [
        "header says 2 needs you, but the needs-you list has 3 bullets",
        "pending review repeats the pinned needs-you bullet",
      ],
    },
    {
      name: "a duplicate sentence",
      body: swap(
        body,
        PENDING_SENTENCE,
        PENDING_SENTENCE + "\n" + PENDING_SENTENCE,
      ),
      want: ["pending review repeats the pinned Details sentence"],
    },
    {
      name: "counts on a Pending Review row",
      body: swap(
        body,
        PENDING_ROW,
        "| Human Feedback | Pending Review | 3 | 0 | 0 | 0 | 3 |",
      ),
      want: [
        "Total Found is 8, rows sum to 11",
        "Total Needs you is 1, rows sum to 4",
        "header says 2 needs you, but the table implies 5",
        "a `Pending Review` row renders `" +
          EM +
          '` in every count cell; row "Human Feedback" has counts',
      ],
    },
  ]);
});

test("the structural checks the single replacements cannot reach", () => {
  const checksBlock = "#### Checks\n";
  assertCases([
    {
      name: "no table",
      body: cut(VALID_ROLLUP, "| Source |", DETAILS_OPEN),
      want: [
        "no source table",
        "header says 6 fixed, but the table's landed Fixed cells sum to 0",
        "header says 1 needs you, but the table implies 0",
      ],
    },
    {
      name: "no Details",
      body: cut(VALID_ROLLUP, DETAILS_OPEN, "> _Triaged"),
      want: [
        "no base sync block: every rollup that is not blocked carries <!-- agentics:triage:section:base-sync -->",
        "no Details block",
      ],
    },
    {
      name: "cell count",
      body: swap(VALID_ROLLUP, CHECKS_ROW, CHECKS_ROW + " extra |"),
      want: [
        "row has 8 cells, want 7: | Checks | Partial | 2 | 1 | " +
          EM +
          " | " +
          EM +
          " | 1 | extra |",
        "a Total row needs two or more rows with counts",
        "header says 6 fixed, but the table's landed Fixed cells sum to 5",
        "header says 1 needs you, but the table implies 0",
      ],
    },
    {
      name: "lone Total",
      body: swap(
        swap(VALID_ROLLUP, CHECKS_ROW + "\n", ""),
        TOTAL_ROW,
        "| **Total** | | **6** | **5** | **1** | **0** | **0** |",
      ),
      want: [
        "a Total row needs two or more rows with counts",
        "header says 6 fixed, but the table's landed Fixed cells sum to 5",
        "header says 1 needs you, but the table implies 0",
      ],
    },
    {
      name: "base sync not first",
      body: swap(
        cut(VALID_ROLLUP, "#### Base sync", "#### Copilot Review"),
        checksBlock,
        "#### Base sync\n\n<!-- agentics:triage:section:base-sync -->\nUp to date.\n<!-- agentics:triage:section-end:base-sync -->\n\n" +
          checksBlock,
      ),
      want: ["base sync is not the first block in Details"],
    },
    {
      name: "no header",
      body: swap(VALID_ROLLUP, HEADER + "\n", ""),
      want: ["no header verdict line"],
    },
    {
      name: "supersedes after table",
      body: swap(
        VALID_ROLLUP,
        DETAILS_OPEN,
        "Earlier triage runs posted per-source comments on this PR; this rollup supersedes them.\n\n" +
          DETAILS_OPEN,
      ),
      want: [
        "needs-you list appears before supersedes line; the layout order is header, link line, blocked base sync, supersedes, needs-you, table, Details",
      ],
    },
    {
      name: "notes after a source",
      body: swap(
        VALID_ROLLUP,
        checksBlock,
        "#### Detection notes\n\n- One bot has no adapter.\n\n" + checksBlock,
      ),
      want: [
        "the `#### Copilot Review` block comes before `#### Detection notes`",
      ],
    },
    {
      name: "sources out of order",
      body: VALID_ROLLUP.replaceAll("#### Copilot Review", "@@A")
        .replaceAll("#### Checks", "#### Copilot Review")
        .replaceAll("@@A", "#### Checks"),
      want: [
        "source blocks are out of table order: `#### Checks` comes before `#### Copilot Review`",
      ],
    },
    {
      name: "no separator",
      body: swap(
        VALID_ROLLUP,
        SEPARATOR + "\n",
        "These rows are not a table.\n",
      ),
      want: [
        "the table has no separator row under its header",
        "header says 6 fixed, but the table's landed Fixed cells sum to 0",
        "header says 1 needs you, but the table implies 0",
      ],
    },
    {
      name: "malformed cell",
      body: swap(
        VALID_ROLLUP,
        CHECKS_ROW,
        `| Checks | Partial | 2 | 1 | pending | ${EM} | 1 |`,
      ),
      want: [
        'row "Checks" has a malformed Accepted cell "pending"; a count is a whole number or `' +
          EM +
          "`",
      ],
    },
    {
      name: "heading in a sibling fold",
      body: swap(
        swap(VALID_ROLLUP, checksBlock, ""),
        "</details>\n\n> _Triaged",
        "</details>\n\n<details><summary>More</summary>\n\n#### Checks\n\n</details>\n\n> _Triaged",
      ),
      want: ["Details has no `#### Checks` heading inside the fold"],
    },
    {
      name: "heading below the fold",
      body: swap(
        swap(VALID_ROLLUP, checksBlock, ""),
        "</details>\n\n> _Triaged",
        "</details>\n\n#### Checks\n\n> _Triaged",
      ),
      want: ["Details has no `#### Checks` heading inside the fold"],
    },
  ]);
});

// --- each check alone -----------------------------------------------------------------------------------------------

const NOTHING_NEEDS_YOU = `Nothing needs you`;
const ALPHA_ROW = "| Alpha | No new findings | 0 | 0 | 0 | 0 | 0 |";
const QUIET_TOTAL = "| **Total** | | **0** | **0** | **0** | **0** | **0** |";

/** A table of the given rows under the standard header and separator. */
const tableOf = (...rows: string[]): string =>
  [COLUMNS, SEPARATOR, ...rows].join("\n");

/**
 * A rollup that needs nothing: no needs-you list, no landed fixes, so a table or a row that is wrong cannot also make the
 * header disagree. `sources` get a Details heading each.
 */
function quietBody(table: string, sources: string[] = []): string {
  const tablePart = table === "" ? "" : table + "\n\n";
  const blocks = sources.map((source) => `#### ${source}\n\n`).join("");
  return `<!-- agentics:triage:rollup -->\n<!-- agentics:triage:execution:1790000000-12345 -->\n\nTriage ${EM} ${NOTHING_NEEDS_YOU}\n\n${tablePart}${DETAILS_OPEN}\n\n${BASE_SYNC_BLOCK}${blocks}</details>\n\n${ATTESTATION_FOOTER}`;
}

test("each check of the checker fires alone on some input, with exactly its own message", () => {
  const afterSync = swap(
    VALID_ROLLUP,
    "<!-- agentics:triage:section-end:base-sync -->\n\n#### Copilot Review",
    "<!-- agentics:triage:section-end:base-sync -->\n\nstray text\n\n#### Copilot Review",
  );
  const noDetails = swap(
    cut(VALID_ROLLUP, DETAILS_OPEN, "> _Triaged"),
    ATTESTATION_FOOTER,
    BASE_SYNC_BLOCK + ATTESTATION_FOOTER,
  );
  const pending = pendingBody();
  const countedPending = swap(
    swap(
      swap(
        swap(
          pending,
          PENDING_ROW,
          "| Human Feedback | Pending Review | 3 | 0 | 0 | 0 | 3 |",
        ),
        TOTAL_ROW,
        "| **Total** | | **11** | **6** | **1** | **0** | **4** |",
      ),
      `2 needs you`,
      `5 needs you`,
    ),
    PENDING_BULLET + "\n",
    PENDING_BULLET +
      "\n- **Checks A:** one\n- **Checks B:** two\n- **Checks C:** three\n",
  );
  assertCases([
    {
      name: "the quiet body",
      body: quietBody(tableOf(ALPHA_ROW), ["Alpha"]),
      want: [],
    },
    {
      name: "table columns",
      body: quietBody(
        [COLUMNS.replace("Found", "Findings"), SEPARATOR, ALPHA_ROW].join("\n"),
      ),
      want: [
        "table columns are [Source Status Findings Fixed Accepted Dismissed Needs you], want [Source Status Found Fixed Accepted Dismissed Needs you]",
      ],
    },
    {
      name: "table separator",
      body: quietBody(
        [COLUMNS, "These rows are not a table.", ALPHA_ROW].join("\n"),
      ),
      want: ["the table has no separator row under its header"],
    },
    { name: "no table", body: quietBody(""), want: ["no source table"] },
    {
      name: "a row of six cells",
      body: quietBody(tableOf("| Alpha | No new findings | 0 | 0 | 0 | 0 |")),
      want: [
        "row has 6 cells, want 7: | Alpha | No new findings | 0 | 0 | 0 | 0 |",
      ],
    },
    {
      name: "a status outside the vocabulary",
      body: quietBody(tableOf("| Alpha | Mystery | 0 | 0 | 0 | 0 | 0 |"), [
        "Alpha",
      ]),
      want: ['row "Alpha" has status "Mystery", outside the eight statuses'],
    },
    {
      name: "a malformed cell",
      body: quietBody(
        tableOf("| Alpha | No new findings | 0 | 0 | pending | 0 | 0 |"),
        ["Alpha"],
      ),
      want: [
        'row "Alpha" has a malformed Accepted cell "pending"; a count is a whole number or `' +
          EM +
          "`",
      ],
    },
    {
      name: "a row that does not balance",
      body: quietBody(
        tableOf("| Alpha | No new findings | 1 | 0 | 0 | 0 | 0 |"),
        ["Alpha"],
      ),
      want: [
        'row "Alpha" does not balance: Fixed + Accepted + Dismissed + Needs you != Found',
      ],
    },
    {
      name: "a Total row over one row",
      body: quietBody(tableOf(ALPHA_ROW, QUIET_TOTAL), ["Alpha"]),
      want: ["a Total row needs two or more rows with counts"],
    },
    {
      name: "a status outside the vocabulary beside a matching header",
      body: swap(
        swap(VALID_ROLLUP, "| Checks | Partial |", "| Checks | Complete |"),
        `6 fixed`,
        `5 fixed`,
      ),
      want: ['row "Checks" has status "Complete", outside the eight statuses'],
    },
    {
      name: "a row that does not balance beside a matching Total",
      body: swap(
        swap(
          VALID_ROLLUP,
          COPILOT_ROW,
          "| Copilot Review | Completed | 6 | 5 | 0 | 0 | 0 |",
        ),
        TOTAL_ROW,
        "| **Total** | | **8** | **6** | **0** | **0** | **1** |",
      ),
      want: [
        'row "Copilot Review" does not balance: Fixed + Accepted + Dismissed + Needs you != Found',
      ],
    },
    {
      name: "a table implying fewer needs-you items than the header",
      body: swap(
        swap(
          swap(
            VALID_ROLLUP,
            CHECKS_ROW,
            `| Checks | Partial | 2 | 2 | ${EM} | ${EM} | 0 |`,
          ),
          TOTAL_ROW,
          "| **Total** | | **8** | **7** | **1** | **0** | **0** |",
        ),
        `6 fixed`,
        `7 fixed`,
      ),
      want: ["header says 1 needs you, but the table implies 0"],
    },
    {
      name: "no base sync block",
      body: swap(VALID_ROLLUP, BASE_SYNC_BLOCK, ""),
      want: [
        "no base sync block: every rollup that is not blocked carries <!-- agentics:triage:section:base-sync -->",
      ],
    },
    { name: "no Details block", body: noDetails, want: ["no Details block"] },
    {
      name: "content after the base sync section",
      body: afterSync,
      want: [
        "content after the base sync section has no heading; detection notes need a `#### Detection notes` heading: stray text",
      ],
    },
    {
      name: "a Details sentence beyond the pinned one",
      body: swap(
        pending,
        PENDING_SENTENCE,
        `${PENDING_SENTENCE}\n\nNot handled ${EM} mchen left two threads.`,
      ),
      want: [
        `pending review Details is not the pinned sentence: Not handled ${EM} mchen left two threads.`,
      ],
    },
    {
      name: "the pinned bullet repeated in the Details block",
      body: swap(
        pending,
        PENDING_SENTENCE,
        `${PENDING_SENTENCE}\n\n${PENDING_BULLET}`,
      ),
      want: ["pending review repeats the pinned needs-you bullet"],
    },
    {
      name: "counts on a Pending Review row that agree with the header and the list",
      body: countedPending,
      want: [
        "a `Pending Review` row renders `" +
          EM +
          '` in every count cell; row "Human Feedback" has counts',
      ],
    },
  ]);
});

// --- line endings, counts, fences and other edge cases ---------------------------------------------------------------

test("CRLF line endings and trailing blank lines are normalized before the checks", () => {
  assert.deepEqual(checkRollupBody(VALID_ROLLUP.replaceAll("\n", "\r\n")), []);
  assert.deepEqual(checkRollupBody(VALID_ROLLUP + "\n\n\n"), []);
  assert.deepEqual(checkRollupBody(pendingBody().replaceAll("\n", "\r\n")), []);
});

test("a body with no header line reports that and nothing after it", () => {
  const noHeader = cut(
    swap(VALID_ROLLUP, HEADER + "\n", ""),
    DETAILS_OPEN,
    "> _Triaged",
  );
  assertCases([
    {
      name: "no header",
      body: noHeader,
      want: ["no header verdict line"],
    },
    {
      name: "drafts and a bad footer come before it",
      body: "Drafted Responses\nno header\nbad footer",
      want: [
        "carries a drafts marker or a Drafted Responses section",
        "the attestation footer is not the last line",
        "no execution key line directly after the rollup marker",
        "no header verdict line",
      ],
    },
  ]);
});

test("the drafts message comes before the footer message", () => {
  const drafts = "carries a drafts marker or a Drafted Responses section";
  const footer = "the attestation footer is not the last line";
  assertCases([
    {
      name: "a Drafted Responses section and a bad footer",
      body: swap(
        swap(VALID_ROLLUP, ATTESTATION_FOOTER, "bad footer"),
        "</details>",
        "</details>\n\nDrafted Responses",
      ),
      want: [drafts, footer],
    },
    {
      name: "the Drafted Responses heading alone",
      body: swap(VALID_ROLLUP, "</details>", "</details>\n\nDrafted Responses"),
      want: [drafts],
    },
  ]);
});

test("a count in the header starts at 1 and has no leading zero", () => {
  const verdicts = [
    `0 fixed ${DOT} 1 needs you`,
    `01 fixed ${DOT} 1 needs you`,
    `6 fixed ${DOT} 0 needs you`,
    `6 fixed ${DOT} 01 needs you`,
    `6 fixed ${DOT} 1 fixed`,
    `1 needs you ${DOT} 6 fixed`,
    `6 fixed ${DOT} 1 needs you ${DOT} 1 fixed`,
  ];
  assertCases(
    verdicts.map((verdict) => ({
      name: verdict,
      body: swap(VALID_ROLLUP, VERDICT, verdict),
      want: [verdictMessage(`@jdoe ${EM} ${verdict}`)],
    })),
  );
});

test("a header count that overflows 64 bits is clamped to the maximum", () => {
  assertCases([
    {
      name: "fixed",
      body: swap(VALID_ROLLUP, `6 fixed`, `1234567890123456789012345 fixed`),
      want: [
        "header says 9223372036854775807 fixed, but the table's landed Fixed cells sum to 6",
      ],
    },
    {
      name: "needs you, one past the maximum",
      body: swap(VALID_ROLLUP, `1 needs you`, `9223372036854775808 needs you`),
      want: [
        "header says 9223372036854775807 needs you, but the table implies 1",
        "header says 9223372036854775807 needs you, but the needs-you list has 1 bullets",
      ],
    },
  ]);
});

test("a Total cell is a whole number with balanced bold markers, or a dash", () => {
  const total = (cells: string): string =>
    swap(VALID_ROLLUP, TOTAL_ROW, `| **Total** | | ${cells}`);
  assertCases([
    {
      name: "bold",
      body: total("**8** | **6** | **1** | **0** | **1** |"),
      want: [],
    },
    {
      name: "plain",
      body: total("8 | 6 | 1 | 0 | 1 |"),
      want: [],
    },
    {
      name: "a bold marker only before the number",
      body: total("**8 | **6 | **1 | **0 | **1 |"),
      want: [
        "Total Found is -1, rows sum to 8",
        "Total Fixed is -1, rows sum to 6",
        "Total Accepted is -1, rows sum to 1",
        "Total Dismissed is -1, rows sum to 0",
        "Total Needs you is -1, rows sum to 1",
      ],
    },
    {
      name: "one cell with a marker before and one after",
      body: total("**8 | 6** | **1** | **0** | **1** |"),
      want: [
        "Total Found is -1, rows sum to 8",
        "Total Fixed is -1, rows sum to 6",
      ],
    },
    {
      name: "a dash",
      body: total(`${EM} | **6** | **1** | **0** | **1** |`),
      want: ["Total Found is -1, rows sum to 8"],
    },
    {
      name: "a leading zero",
      body: total("**08** | **6** | **1** | **0** | **1** |"),
      want: ["Total Found is -1, rows sum to 8"],
    },
  ]);
});

test("counts are 64-bit integers: a cell clamps at the maximum and a sum wraps", () => {
  const big = "1000000000000000000000000";
  const rows = (found: string): string =>
    swap(
      swap(
        VALID_ROLLUP,
        COPILOT_ROW,
        `| Copilot Review | Completed | ${found} | 0 | 0 | 0 | 0 |`,
      ),
      CHECKS_ROW,
      `| Checks | Partial | ${found} | 0 | 0 | 0 | 0 |`,
    );
  const unbalanced = (source: string): string =>
    `row "${source}" does not balance: Fixed + Accepted + Dismissed + Needs you != Found`;
  const tail = [
    "Total Found is 8, rows sum to -2",
    "Total Fixed is 6, rows sum to 0",
    "Total Accepted is 1, rows sum to 0",
    "Total Needs you is 1, rows sum to 0",
    "header says 6 fixed, but the table's landed Fixed cells sum to 0",
    "header says 1 needs you, but the table implies 0",
  ];
  const want = [unbalanced("Copilot Review"), unbalanced("Checks"), ...tail];
  assertCases([
    { name: "25-digit Found cells", body: rows(big), want },
    { name: "the maximum itself", body: rows(MAX_INT64), want },
  ]);
});

test("a row with the wrong number of cells is reported with its raw line and skipped", () => {
  const rowText = (line: string): string =>
    swap(VALID_ROLLUP, CHECKS_ROW, line);
  const skipped = [
    "a Total row needs two or more rows with counts",
    "header says 6 fixed, but the table's landed Fixed cells sum to 5",
    "header says 1 needs you, but the table implies 0",
  ];
  const six = `| Checks | Partial | 2 | 1 | ${EM} | ${EM} |`;
  const eight = `| Checks | Partial | 2 | 1 | ${EM} | ${EM} | 1 | x |`;
  const openRow = `| Checks | Partial | 2 | 1 | ${EM} | ${EM}`;
  assertCases([
    {
      name: "six cells",
      body: rowText(six),
      want: [`row has 6 cells, want 7: ${six}`, ...skipped],
    },
    {
      name: "eight cells",
      body: rowText(eight),
      want: [`row has 8 cells, want 7: ${eight}`, ...skipped],
    },
    {
      name: "a missing cell and no closing pipe",
      body: rowText(openRow),
      want: [`row has 6 cells, want 7: ${openRow}`, ...skipped],
    },
    {
      name: "seven cells and no closing pipe, which is accepted",
      body: rowText(CHECKS_ROW.slice(0, -1)),
      want: [],
    },
  ]);
});

test("the last Total row is the one compared", () => {
  const wrongTotal = "| **Total** | | **9** | **6** | **1** | **0** | **1** |";
  assertCases([
    {
      name: "the first one is wrong",
      body: swap(VALID_ROLLUP, TOTAL_ROW, wrongTotal + "\n" + TOTAL_ROW),
      want: [],
    },
    {
      name: "the last one is wrong",
      body: swap(VALID_ROLLUP, TOTAL_ROW, TOTAL_ROW + "\n" + wrongTotal),
      want: ["Total Found is 9, rows sum to 8"],
    },
  ]);
});

test("the pinned bullet names the row's own source, and a bullet is collected wherever it stands", () => {
  const single = (source: string): string =>
    [
      "<!-- agentics:triage:rollup -->",
      "<!-- agentics:triage:execution:1790000000-12345 -->",
      "",
      `@jdoe ${EM} 1 needs you`,
      "",
      NEEDS_HEADING,
      "",
      PENDING_BULLET.replace("Human Feedback", source),
      "",
      COLUMNS,
      SEPARATOR,
      PENDING_ROW.replace("Human Feedback", source),
      "",
      DETAILS_OPEN,
      "",
      BASE_SYNC_BLOCK + `#### ${source}`,
      "",
      PENDING_SENTENCE,
      "",
      "</details>",
      "",
      ATTESTATION_FOOTER,
    ].join("\n");
  const inChecks = swap(
    pendingBody(),
    "- **Fixed (1):** Format drift",
    "- **Human Feedback:** reviewer mchen left notes\n- **Fixed (1):** Format drift",
  );
  assertCases([
    { name: "Human Feedback", body: single("Human Feedback"), want: [] },
    {
      name: "another source",
      body: single("Bot Review (acme[bot])"),
      want: [],
    },
    {
      name: "a bullet that does not name its own source",
      body: swap(
        single("Bot Review (acme[bot])"),
        PENDING_BULLET.replace("Human Feedback", "Bot Review (acme[bot])"),
        PENDING_BULLET.replace("Human Feedback", "Bot Review (acme[bot]) x"),
      ),
      want: [
        `pending review bullet is not the pinned text: ${PENDING_BULLET.replace("Human Feedback", "Bot Review (acme[bot]) x")}`,
      ],
    },
    {
      name: "a bullet inside another source's block",
      body: inChecks,
      want: [
        "pending review bullet is not the pinned text: - **Human Feedback:** reviewer mchen left notes",
        "pending review repeats the pinned needs-you bullet",
      ],
    },
  ]);
});

test("a table whose columns differ is reported with both column lists in brackets", () => {
  const columnsOf = (cells: string[]): string =>
    swap(VALID_ROLLUP, COLUMNS, `| ${cells.join(" | ")} |`);
  const wantList = "[Source Status Found Fixed Accepted Dismissed Needs you]";
  const skipped = [
    "header says 6 fixed, but the table's landed Fixed cells sum to 0",
    "header says 1 needs you, but the table implies 0",
  ];
  assertCases([
    {
      name: "space-separated names",
      body: columnsOf([
        "Source",
        "Status",
        "Findings",
        "Fixed",
        "Accepted",
        "Dismissed",
        "Needs you",
      ]),
      want: [
        `table columns are [Source Status Findings Fixed Accepted Dismissed Needs you], want ${wantList}`,
        ...skipped,
      ],
    },
    {
      name: "one column",
      body: columnsOf(["Source"]),
      want: [`table columns are [Source], want ${wantList}`, ...skipped],
    },
  ]);
});

// --- the rule -------------------------------------------------------------------------------------------------------

const SKELETON = `\`\`\`text\n<!-- agentics:triage:rollup -->\n\n@{PR_AUTHOR} ${EM} {VERDICT}\n\n${ATTESTATION_FOOTER}\n\`\`\`\n\n`;
const EXAMPLE = "```text\n" + VALID_ROLLUP + "\n```\n\n";
const BROKEN_HEADER = `@jdoe ${EM} PR triage complete: **6 replies posted** (bot findings).`;
const BROKEN = swap(VALID_ROLLUP, HEADER, BROKEN_HEADER);
const BROKEN_MESSAGE = verdictMessage(BROKEN_HEADER);
const BROKEN_FENCE = "```text\n" + BROKEN + "\n```\n\n";

/** The template text the rule reads: the mapping tables, then the fences given. */
function template(fences: string): string {
  return rollupMappingMD() + "## Examples\n\n" + fences;
}

async function ruleFailures(text: string | undefined): Promise<Failure[]> {
  return withSkillsRepo((root) => {
    if (text !== undefined) writeFile(root, TEMPLATE, text);
    return rollupShapeFailures(root);
  });
}

const failure = (message: string): Failure => ({
  rule: ROLLUP_SHAPE_RULE,
  path: TEMPLATE,
  message,
});

test("the rule is named triage-rollup-shape and the green fixture passes it", async () => {
  assert.equal(ROLLUP_SHAPE_RULE, "triage-rollup-shape");
  assert.deepEqual(await ruleFailures(undefined), []);
  assert.deepEqual(
    await ruleFailures(template(EXAMPLE + EXAMPLE + EXAMPLE)),
    [],
  );
});

test("a template with fewer than three rendered examples reports the count", async () => {
  assert.deepEqual(await ruleFailures(template(SKELETON)), [
    failure("found 0 rendered rollup examples, want at least 3"),
  ]);
  assert.deepEqual(await ruleFailures(template(EXAMPLE + EXAMPLE)), [
    failure("found 2 rendered rollup examples, want at least 3"),
  ]);
});

test("an example in the pre-redesign shape fails with its number", async () => {
  assert.deepEqual(
    await ruleFailures(template(BROKEN_FENCE + EXAMPLE + EXAMPLE)),
    [failure("example 1: " + BROKEN_MESSAGE)],
  );
  assert.deepEqual(
    await ruleFailures(template(EXAMPLE + BROKEN_FENCE + EXAMPLE)),
    [failure("example 2: " + BROKEN_MESSAGE)],
  );
});

test("a skeleton fence is not counted when it comes first", async () => {
  assert.deepEqual(
    await ruleFailures(template(SKELETON + BROKEN_FENCE + EXAMPLE + EXAMPLE)),
    [failure("example 1: " + BROKEN_MESSAGE)],
  );
});

test("a template with CRLF line endings has no fence the rule matches", async () => {
  const crlf = template(EXAMPLE + EXAMPLE + EXAMPLE).replaceAll("\n", "\r\n");
  assert.deepEqual(await ruleFailures(crlf), [
    failure("found 0 rendered rollup examples, want at least 3"),
  ]);
});

test("a fence closes on the first line that starts with three backticks, and only a text fence is an example", async () => {
  const fenced = (open: string, close: string): string =>
    open + "\n" + VALID_ROLLUP + "\n" + close + "\n\n";
  const two = failure("found 2 rendered rollup examples, want at least 3");
  assert.deepEqual(
    await ruleFailures(template(EXAMPLE + EXAMPLE + fenced("```text", "````"))),
    [],
    "a four-backtick closing line still closes the block",
  );
  assert.deepEqual(
    await ruleFailures(
      template(EXAMPLE + EXAMPLE + fenced("````text", "````")),
    ),
    [two],
    "a four-backtick opening line is not a text fence",
  );
  assert.deepEqual(
    await ruleFailures(template(EXAMPLE + EXAMPLE + fenced("~~~text", "~~~"))),
    [two],
    "a tilde fence is not an example",
  );
  const early =
    "```text\n" +
    swap(VALID_ROLLUP, "#### Checks\n", "```\n#### Checks\n") +
    "\n```\n\n";
  assert.deepEqual(await ruleFailures(template(EXAMPLE + EXAMPLE + early)), [
    failure("example 3: the attestation footer is not the last line"),
    failure("example 3: Details has no `#### Checks` heading inside the fold"),
  ]);
});

test("a read error on the template is thrown as a file error, never turned into a failure", async () => {
  const sentinel = new Error("the template could not be read");
  await withSkillsRepo((root) => {
    assert.throws(
      () =>
        withSeams(
          {
            readFile: () => {
              throw sentinel;
            },
          },
          () => rollupShapeFailures(root),
        ),
      (error) =>
        error instanceof RepoFileError &&
        error.kind === "unreadable" &&
        error.path === TEMPLATE &&
        error.cause === sentinel,
    );
  });
  await withSkillsRepo((root) => {
    remove(root, TEMPLATE);
    mkdir(root, TEMPLATE);
    assert.throws(
      () => rollupShapeFailures(root),
      (error) =>
        error instanceof RepoFileError &&
        error.kind === "not-a-file" &&
        error.message === `${TEMPLATE}: is not a regular file`,
    );
  });
  await withSkillsRepo((root) => {
    remove(root, TEMPLATE);
    assert.throws(
      () => rollupShapeFailures(root),
      (error) =>
        error instanceof RepoFileError &&
        error.kind === "missing" &&
        error.path === TEMPLATE,
    );
  });
});

test("a template that is a link is a symlink file error and is not read", async (t) => {
  await withSkillsRepo(async (root) => {
    const outside = mkdtempSync(path.join(tmpdir(), "agentics-rollup-out-"));
    try {
      const target = path.join(outside, "rollup-template.md");
      writeFileSync(target, "# A template with no rendered example\n");
      remove(root, TEMPLATE);
      if (!link(root, TEMPLATE, target, "file")) {
        t.skip("this host cannot create a symlink");
        return;
      }
      assert.throws(() => rollupShapeFailures(root), {
        kind: "symlink",
        message: `${TEMPLATE}: passes through a link`,
      });
    } finally {
      rmSync(outside, { recursive: true, force: true });
    }
  });
});

test("a template edited through mutateFile is the one the rule reads", async () => {
  await withSkillsRepo((root) => {
    mutateFile(root, TEMPLATE, HEADER, BROKEN_HEADER);
    assert.deepEqual(
      rollupShapeFailures(root).map((one) => one.message),
      [1, 2, 3].map((n) => `example ${n}: ${BROKEN_MESSAGE}`),
    );
  });
});

test("rendered examples in rollup-details-and-examples.md are checked and attributed to that file", async (t) => {
  const EXAMPLES = "skills/triage/references/rollup-details-and-examples.md";
  const fence = (body: string) => "```text\n" + body + "\n```\n";
  await withSkillsRepo((root) => {
    writeFile(root, TEMPLATE, "# Rollup template\n\nLayout skeleton only.\n");
    writeFile(
      root,
      EXAMPLES,
      [
        "# Rollup details and examples\n",
        fence(FIXTURE_VALID_ROLLUP),
        fence(FIXTURE_VALID_ROLLUP),
        fence(FIXTURE_VALID_ROLLUP),
      ].join("\n"),
    );
    assert.deepEqual(rollupShapeFailures(root), []);
    mutateFile(root, EXAMPLES, HEADER, BROKEN_HEADER);
    assert.deepEqual(
      rollupShapeFailures(root),
      [1, 2, 3].map((n) => ({
        rule: ROLLUP_SHAPE_RULE,
        path: EXAMPLES,
        message: `example ${n}: ${BROKEN_MESSAGE}`,
      })),
    );
  });
  await withSkillsRepo((root) => {
    const outside = mkdtempSync(path.join(tmpdir(), "agentics-rollup-ex-"));
    try {
      const target = path.join(outside, "rollup-details-and-examples.md");
      writeFileSync(target, "# External examples\n");
      if (!link(root, EXAMPLES, target, "file")) {
        t.skip("this host cannot create a symlink");
        return;
      }
      assert.throws(() => rollupShapeFailures(root), {
        kind: "symlink",
        message: `${EXAMPLES}: passes through a link`,
      });
    } finally {
      rmSync(outside, { recursive: true, force: true });
    }
  });
});

// --- white space and line starts: the character classes the patterns spell out --------------------------------------

const VT = String.fromCharCode(0x0b);
const NBSP = String.fromCodePoint(0xa0);
const LS = String.fromCodePoint(0x2028);
const CR = String.fromCharCode(0x0d);

test("white space is tab, LF, FF, CR and space: a vertical tab or a no-break space does not pad a separator row", () => {
  const separator = (edit: (row: string) => string): string =>
    quietBody(tableOf(ALPHA_ROW).replace(SEPARATOR, edit(SEPARATOR)), [
      "Alpha",
    ]);
  const noSeparator = ["the table has no separator row under its header"];
  assertCases([
    {
      name: "tab and form feed pad a cell",
      body: separator((row) =>
        row.replace("| ------", "|\t------").replace("| -----", "|\f-----"),
      ),
      want: [],
    },
    {
      name: "a vertical tab pads a cell",
      body: separator((row) => row.replace("| ------", "|" + VT + "------")),
      want: noSeparator,
    },
    {
      name: "a no-break space pads a cell",
      body: separator((row) => row.replace("| ------", "|" + NBSP + "------")),
      want: noSeparator,
    },
    {
      name: "a vertical tab ends the row",
      body: separator((row) => row + VT),
      want: noSeparator,
    },
    {
      name: "a no-break space ends the row",
      body: separator((row) => row + NBSP),
      want: noSeparator,
    },
  ]);
});

test("a non-space is everything but tab, LF, FF, CR and space: the author of the header may hold a vertical tab or a no-break space", () => {
  const author = (name: string): string =>
    swap(VALID_ROLLUP, "@jdoe", "@" + name);
  assertCases([
    { name: "a no-break space", body: author("j" + NBSP + "doe"), want: [] },
    { name: "a vertical tab", body: author("j" + VT + "doe"), want: [] },
    { name: "U+2028", body: author("j" + LS + "doe"), want: [] },
    {
      name: "U+2028 after the verdict is part of the verdict, which then fails to parse",
      body: swap(VALID_ROLLUP, "1 needs you\n", "1 needs you" + LS + "\n"),
      want: [verdictMessage(`${HEADER}${LS}`)],
    },
    {
      name: "a tab ends the author",
      body: author("j\tdoe"),
      want: [verdictMessage(`@j\tdoe ${EM} ${VERDICT}`)],
    },
  ]);
});

test("a line start is after LF only: a lone CR or U+2028 does not open or close a fence", async () => {
  const two = failure("found 2 rendered rollup examples, want at least 3");
  const opener = (separator: string): string =>
    template(
      EXAMPLE +
        EXAMPLE +
        "prose" +
        separator +
        "```text\n" +
        VALID_ROLLUP +
        "\n```\n\n",
    );
  const closer = (separator: string): string =>
    template(
      EXAMPLE +
        EXAMPLE +
        "```text\n" +
        swap(
          VALID_ROLLUP,
          "<sub>6 replies posted</sub>",
          "<sub>6 replies posted</sub>" + separator + "```",
        ) +
        "\n```\n\n",
    );
  assert.deepEqual(await ruleFailures(opener(CR)), [two], "an opener after CR");
  assert.deepEqual(
    await ruleFailures(opener(LS)),
    [two],
    "an opener after U+2028",
  );
  assert.deepEqual(await ruleFailures(opener("\n")), [], "an opener after LF");
  assert.deepEqual(await ruleFailures(closer(CR)), [], "a closer after CR");
  assert.deepEqual(await ruleFailures(closer(LS)), [], "a closer after U+2028");
  assert.deepEqual(
    await ruleFailures(closer("\n")),
    [
      failure("example 3: the attestation footer is not the last line"),
      failure(
        "example 3: Details has no `#### Checks` heading inside the fold",
      ),
    ],
    "a closer after LF",
  );
});

// --- layout, blocked-sync and list edges, rows added by the mutation pass ----------------------------------------------

const LAYOUT_ORDER =
  "the layout order is header, link line, blocked base sync, supersedes, needs-you, table, Details";

test("a header on the first line is still a header", () => {
  const body = VALID_ROLLUP.slice(VALID_ROLLUP.indexOf("@jdoe"));
  assert.deepEqual(checkRollupBody(body), [
    "no execution key line directly after the rollup marker",
  ]);
});

// --- the execution key and the earlier-pass link ----------------------------------------------------------------------

const KEY_LINE = "<!-- agentics:triage:execution:1790000000-12345 -->";
const LINK_LINE =
  "Earlier triage pass: [rollup of 2026-11-04](https://github.com/acme/payments-api/pull/230#issuecomment-123456)";
const KEY_POSITION = "no execution key line directly after the rollup marker";
const KEY_COUNT = "more than one execution key line";
const LINK_COUNT = "more than one earlier-pass link line";
const LINK_UNDER = "the link line is not directly under the header";

test("the execution key line is the second line and appears once, and one link line sits under the header", () => {
  const withLink = swap(
    VALID_ROLLUP,
    HEADER + "\n\n",
    `${HEADER}\n\n${LINK_LINE}\n\n`,
  );
  assertCases([
    { name: "one link line under the header", body: withLink, want: [] },
    {
      name: "no key line",
      body: swap(VALID_ROLLUP, KEY_LINE + "\n", ""),
      want: [KEY_POSITION],
    },
    {
      name: "a malformed key on the second line",
      body: swap(
        VALID_ROLLUP,
        KEY_LINE,
        "<!-- agentics:triage:execution:1790000000 -->",
      ),
      want: [KEY_POSITION],
    },
    {
      name: "a key line with a leading space",
      body: swap(VALID_ROLLUP, KEY_LINE, " " + KEY_LINE),
      want: [KEY_POSITION],
    },
    {
      name: "a key line with a trailing space",
      body: swap(VALID_ROLLUP, KEY_LINE, KEY_LINE + " "),
      want: [KEY_POSITION],
    },
    {
      name: "a duplicate key line",
      body: swap(VALID_ROLLUP, KEY_LINE, KEY_LINE + "\n" + KEY_LINE),
      want: [KEY_COUNT],
    },
    {
      name: "two link lines",
      body: swap(
        VALID_ROLLUP,
        HEADER + "\n\n",
        `${HEADER}\n\n${LINK_LINE}\n${LINK_LINE}\n\n`,
      ),
      want: [LINK_COUNT],
    },
    {
      name: "prose between the header and the link line",
      body: swap(
        VALID_ROLLUP,
        HEADER + "\n\n",
        `${HEADER}\n\nThis pass re-checked the Copilot threads.\n\n${LINK_LINE}\n\n`,
      ),
      want: [LINK_UNDER],
    },
    {
      name: "a link line straight under the header",
      body: swap(VALID_ROLLUP, HEADER + "\n\n", `${HEADER}\n${LINK_LINE}\n\n`),
      want: [],
    },
    {
      name: "prose between the header and the link line of a blocked rollup",
      body: swap(
        blockedBody([]),
        "<!-- agentics:triage:section:base-sync -->",
        `Stray.\n\n${LINK_LINE}\n\n<!-- agentics:triage:section:base-sync -->`,
      ),
      want: [LINK_UNDER],
    },
    {
      name: "a link line above the header",
      body: swap(VALID_ROLLUP, HEADER, `${LINK_LINE}\n\n${HEADER}`),
      want: [`link line appears before header; ${LAYOUT_ORDER}`],
    },
    {
      name: "two key lines, neither on the second line",
      body: swap(
        swap(VALID_ROLLUP, KEY_LINE + "\n", ""),
        HEADER + "\n",
        `${HEADER}\n${KEY_LINE}\n${KEY_LINE}\n`,
      ),
      want: [KEY_POSITION, KEY_COUNT],
    },
    {
      name: "no key line and two link lines",
      body: swap(
        swap(VALID_ROLLUP, KEY_LINE + "\n", ""),
        HEADER + "\n\n",
        `${HEADER}\n\n${LINK_LINE}\n${LINK_LINE}\n\n`,
      ),
      want: [KEY_POSITION, LINK_COUNT],
    },
    {
      name: "a link line after a blocked base sync",
      body: swap(
        blockedBody([]),
        "<!-- agentics:triage:section-end:base-sync -->\n",
        `<!-- agentics:triage:section-end:base-sync -->\n\n${LINK_LINE}\n`,
      ),
      want: [`blocked base sync appears before link line; ${LAYOUT_ORDER}`],
    },
    {
      name: "a second key line with another value inside Details",
      body: swap(
        VALID_ROLLUP,
        "#### Checks\n",
        "<!-- agentics:triage:execution:1790000001-54321 -->\n\n#### Checks\n",
      ),
      want: [KEY_COUNT],
    },
    {
      name: "a line starting like a link line inside Details",
      body: swap(
        swap(VALID_ROLLUP, HEADER + "\n\n", `${HEADER}\n\n${LINK_LINE}\n\n`),
        "#### Checks\n",
        "Earlier triage pass: see the Copilot block above.\n\n#### Checks\n",
      ),
      want: [LINK_COUNT],
    },
  ]);
});

test("the key and link checks run before the header search, so a body with no header still reports them", () => {
  const headerMessage = "no header verdict line";
  assertCases([
    {
      name: "two keys off the second line, two link lines and no header",
      body: [
        "<!-- agentics:triage:rollup -->",
        "x",
        KEY_LINE,
        KEY_LINE,
        LINK_LINE,
        LINK_LINE,
        ATTESTATION_FOOTER,
      ].join("\n"),
      want: [KEY_POSITION, KEY_COUNT, LINK_COUNT, headerMessage],
    },
    {
      name: "the empty body",
      body: "",
      want: [
        "the attestation footer is not the last line",
        KEY_POSITION,
        headerMessage,
      ],
    },
    {
      name: "the marker alone",
      body: "<!-- agentics:triage:rollup -->",
      want: [
        "the attestation footer is not the last line",
        KEY_POSITION,
        headerMessage,
      ],
    },
  ]);
});

test("a block named Details out of order is named in the layout message", () => {
  const start = VALID_ROLLUP.indexOf(DETAILS_OPEN);
  const stop = VALID_ROLLUP.indexOf("</details>") + "</details>\n\n".length;
  const details = VALID_ROLLUP.slice(start, stop);
  const moved = swap(
    VALID_ROLLUP.slice(0, start) + VALID_ROLLUP.slice(stop),
    TABLE,
    details + TABLE,
  );
  assert.ok(
    checkRollupBody(moved).includes(
      `Details appears before table; ${LAYOUT_ORDER}`,
    ),
    checkRollupBody(moved).join("\n"),
  );
});

test("a source block that is the detection notes block is not before itself", () => {
  const body = VALID_ROLLUP.replaceAll("Copilot Review", "Detection notes");
  const messages = checkRollupBody(body);
  assert.deepEqual(
    messages.filter((message) => message.includes("comes before")),
    [],
  );
});

test("a source block ends where the next heading begins, so a later block is not read as its own", () => {
  const body = pendingBody();
  const block = `#### Human Feedback\n\n${PENDING_SENTENCE}\n\n`;
  const moved = swap(
    swap(body, block, ""),
    "#### Checks",
    block + "#### Checks",
  );
  assert.deepEqual(
    checkRollupBody(moved).filter((message) =>
      message.startsWith("pending review Details is not the pinned sentence"),
    ),
    [],
  );
});

test("a blank line after the needs-you bullets ends the list", () => {
  const body = swap(
    VALID_ROLLUP,
    BULLET + "\n\n",
    `${BULLET}\n\n- a stray bullet after a blank line\n\n`,
  );
  assert.deepEqual(checkRollupBody(body), []);
});

test("a blocked base sync with a clean header but too many needs you is named", () => {
  assert.deepEqual(checkRollupBody(blockedBody([], `2 needs you`)), [
    "a blocked base sync's header is `1 needs you`",
  ]);
});

test("a blocked rollup renders no table, whatever its rows hold", () => {
  for (const row of [
    "| Copilot Review | Skipped | 1 | 1 | 0 | 0 | 0 |",
    `| Copilot Review | Completed | ${EM} | ${EM} | ${EM} | ${EM} | ${EM} |`,
    "| Copilot Review | Completed | 1 | 1 | 0 | 0 | 0 |",
  ])
    assert.deepEqual(checkRollupBody(blockedBody([row])), [
      "a blocked rollup renders no table",
    ]);
});

test("a blocked base sync renders alone whichever of the needs-you list and Details is present, and wherever it stands", () => {
  const alone =
    "a blocked base sync renders alone: no needs-you list and no Details";
  const blocked = blockedBody([]);
  const withNeeds = swap(
    blocked,
    "\n\n" + ATTESTATION_FOOTER,
    `\n\n${NEEDS_HEADING}\n\n${BULLET}\n\n${ATTESTATION_FOOTER}`,
  );
  assert.ok(checkRollupBody(withNeeds).includes(alone), "a needs-you list");
  const detailsFirst = `${DETAILS_OPEN}\n</details>\n${blocked}`;
  assert.ok(checkRollupBody(detailsFirst).includes(alone), "Details first");
  const needsFirst = `${NEEDS_HEADING}\n${blocked}`;
  assert.ok(checkRollupBody(needsFirst).includes(alone), "needs-you first");
});

// --- the mention and the emoji rule ---------------------------------------------------------------------------------

test("the header mentions the author exactly when something needs them", () => {
  const quiet = quietBody(tableOf(ALPHA_ROW), ["Alpha"]);
  assertCases([
    { name: "a quiet header with nothing needed", body: quiet, want: [] },
    {
      name: "a mention with nothing needed",
      body: swap(quiet, `Triage ${EM} `, `@jdoe ${EM} `),
      want: [
        `the header mentions the author although nothing needs them: @jdoe ${EM} ${NOTHING_NEEDS_YOU}`,
      ],
    },
    {
      name: "no mention although something is needed",
      body: swap(VALID_ROLLUP, HEADER, `Triage ${EM} ${VERDICT}`),
      want: [
        `the header must lead with the author's mention when something needs them: Triage ${EM} ${VERDICT}`,
      ],
    },
    {
      name: "a lead that is neither a mention nor the quiet word",
      body: swap(quiet, `Triage ${EM} `, `Summary ${EM} `),
      want: ["no header verdict line"],
    },
  ]);
});

test("an emoji anywhere in the body is reported once, with its first line", () => {
  const check = String.fromCodePoint(0x2705);
  const warning = String.fromCodePoint(0x26a0, 0xfe0f);
  assertCases([
    {
      name: "the old verdict marks",
      body: swap(
        VALID_ROLLUP,
        HEADER,
        `@jdoe ${EM} ${check} 6 fixed ${DOT} ${warning} 1 needs you`,
      ),
      want: [
        `carries an emoji, which no posted body may: @jdoe ${EM} ${check} 6 fixed ${DOT} ${warning} 1 needs you`,
        `header verdict is not \`N fixed ${DOT} M needs you\` or \`Nothing needs you\`: @jdoe ${EM} ${check} 6 fixed ${DOT} ${warning} 1 needs you`,
      ],
    },
    {
      name: "an emoji in a Details bullet",
      body: swap(
        VALID_ROLLUP,
        "fails non-deterministically.",
        `fails non-deterministically ${warning}.`,
      ),
      want: [
        `carries an emoji, which no posted body may: ${BULLET.replace("fails non-deterministically.", `fails non-deterministically ${warning}.`)}`,
      ],
    },
    {
      name: "a middle dot and an em dash are not emoji",
      body: VALID_ROLLUP,
      want: [],
    },
  ]);
});

test("a flag and a keycap, which hold no pictographic code point, are emoji too", () => {
  const flag = String.fromCodePoint(0x1f1fa, 0x1f1f8);
  const keycap = String.fromCodePoint(0x31, 0xfe0f, 0x20e3);
  const plain = "fails non-deterministically.";
  for (const [name, mark] of [
    ["a flag", flag],
    ["a keycap", keycap],
  ] as const) {
    const marked = `fails non-deterministically ${mark}.`;
    assertCases([
      {
        name,
        body: swap(VALID_ROLLUP, plain, marked),
        want: [
          `carries an emoji, which no posted body may: ${BULLET.replace(plain, marked)}`,
        ],
      },
    ]);
  }
});

// --- the rollup scenarios of the skills domain audit ---------------------------------------------------------------

/** The slice that holds the triage family, whose families a slice scenario runs. */
const CONTENT_SLICE = sliceNamed("content");

/** The `triage-rollup-shape` finding on the rollup template, once per message. */
const shapeFindings = (...messages: string[]): AuditResult => ({
  failures: messages.map((message) => ({
    rule: "triage-rollup-shape",
    path: "skills/triage/references/rollup-template.md",
    message,
  })),
});

/** `check`'s message for each of the template's three rendered examples. */
const everyExample = (check: string): string[] =>
  [1, 2, 3].map((example) => `example ${example}: ${check}`);

test("triage-rollup-shape", async () => {
  const breaker = BREAKERS["triage-rollup-shape"];
  assert.ok(breaker !== undefined);
  const expected = shapeFindings(
    `example 1: header verdict is not \`N fixed ${DOT} M needs you\` or \`Nothing needs you\`: @jdoe ${EM} all done ${DOT} 1 needs you`,
  );
  assert.deepEqual(await auditSlice(CONTENT_SLICE, breaker), expected);
  assert.deepEqual(await auditEveryFamily(breaker), expected);
});

test("rollup examples without an execution key line", async () => {
  assert.deepEqual(
    await auditSlice(CONTENT_SLICE, (r) =>
      mutateFile(r, seeds.ROLLUP_TEMPLATE, `${seeds.EXECUTION_KEY}\n`, ""),
    ),
    shapeFindings(
      ...everyExample("no execution key line directly after the rollup marker"),
    ),
  );
});

test("rollup examples with a duplicated execution key and two link lines", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      seeds.all(
        (r) =>
          mutateFile(
            r,
            seeds.ROLLUP_TEMPLATE,
            `${seeds.EXECUTION_KEY}\n`,
            `${seeds.EXECUTION_KEY}\n${seeds.EXECUTION_KEY}\n`,
          ),
        (r) =>
          mutateFile(
            r,
            seeds.ROLLUP_TEMPLATE,
            " 1 needs you\n\n### ",
            ` 1 needs you\n\n${seeds.EARLIER_PASS_LINK}\n${seeds.EARLIER_PASS_LINK}\n\n### `,
          ),
      ),
    ),
    shapeFindings(
      "example 1: more than one execution key line",
      "example 1: more than one earlier-pass link line",
      "example 2: more than one execution key line",
      "example 2: more than one earlier-pass link line",
      "example 3: more than one execution key line",
      "example 3: more than one earlier-pass link line",
    ),
  );
});

test("rollup examples with prose between the header and the link line", async () => {
  assert.deepEqual(
    await auditSlice(CONTENT_SLICE, (r) =>
      mutateFile(
        r,
        seeds.ROLLUP_TEMPLATE,
        " 1 needs you\n\n### ",
        ` 1 needs you\n\nThis pass re-checked the Copilot threads.\n\n${seeds.EARLIER_PASS_LINK}\n\n### `,
      ),
    ),
    shapeFindings(
      ...everyExample("the link line is not directly under the header"),
    ),
  );
});

test("rollup examples with a link line below the needs-you list", async () => {
  assert.deepEqual(
    await auditSlice(CONTENT_SLICE, (r) =>
      mutateFile(
        r,
        seeds.ROLLUP_TEMPLATE,
        "[Failed job](https://example.invalid/job/1)\n",
        `[Failed job](https://example.invalid/job/1)\n\n${seeds.EARLIER_PASS_LINK}\n`,
      ),
    ),
    shapeFindings(
      ...everyExample(
        "needs-you list appears before link line; the layout order is header, link line, blocked base sync, supersedes, needs-you, table, Details",
      ),
    ),
  );
});

test("references/orchestration-lifecycle.md is removed", async () => {
  // The rollup template's examples are all valid; only the guard's listed emitter is missing.
  assert.deepEqual(
    await auditSlice(CONTENT_SLICE, (r) =>
      remove(r, `${seeds.TRIAGE_REFS}/orchestration-lifecycle.md`),
    ),
    {
      failures: [
        {
          rule: "triage-attestation-footer",
          path: "skills/triage/references/orchestration-lifecycle.md",
          message: "emitter listed in the guard does not exist",
        },
      ],
    },
  );
});

test(
  "docs/guide is a symlink to a directory holding a trailer",
  NEEDS_SYMLINK,
  async () => {
    // The trailer is reachable only through the link, and the audit reports nothing, the rollup examples included.
    assert.deepEqual(
      await auditSlice(
        CONTENT_SLICE,
        seeds.all(
          seeds.put("guide-target/commits.md", "Assisted-by: Claude\n"),
          seeds.symlinked("docs/guide", "../guide-target", "dir"),
        ),
      ),
      { failures: [] },
    );
  },
);
