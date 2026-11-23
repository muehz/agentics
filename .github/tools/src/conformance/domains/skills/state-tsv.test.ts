import assert from "node:assert/strict";
import { mkdtempSync, renameSync, rmSync, symlinkSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { test } from "node:test";
import type { Failure, RunOptions } from "../../check.ts";
import {
  auditEveryFamily,
  all,
  BAD_STATE,
  breaker,
  put,
} from "../../testing/skills-seeds.ts";
import { link, mkdir, writeFile } from "../../testing/skills-repo.ts";
import { RepoFileError } from "../../repo-file-error.ts";
import { seams, withSeams, type Seams } from "./files.ts";
import { run } from "./index.ts";
import { checkStateFile, FAMILY, inWorkFolderTemp } from "./state-tsv.ts";
import {
  canSymlink,
  removeTree,
  SYMLINK_SKIP,
  withOutside,
} from "../../testing/host.ts";

const options: RunOptions = {
  short: false,
  fast: false,
  diagnostics: () => {},
  env: process.env,
};

const RULE = "state-tsv-shape";
const REL = "docs/specs/2026-10/feature-x/state.tsv";

const HEADER = "ts\tsession\tphase\tevent\ttarget\tstatus\twhy\tevidence\tnext";
const VERSION = "# flow-state 1";
const GOOD_ROW =
  "2026-10-14T10:00:00Z\ts1\tintent\tcreate\tintent.md\tok\tcapture\tintent.md\tspec define";

// A backslash, so an expected message's escaped control character (a backslash then `t` or `r`) is built without a
// backslash-u escape in the source.
const BS = String.fromCharCode(92);
const BOM = String.fromCodePoint(0xfeff);
const NEL = String.fromCodePoint(0x85);

/** A state file: the version line, the header row and the stated rows. */
function stateFile(...rows: string[]): string {
  return `${VERSION}\n${HEADER}\n${rows.join("\n")}\n`;
}

/** The good row with one cell (zero-based) replaced. */
function withCell(column: number, value: string): string {
  const cells = GOOD_ROW.split("\t");
  cells[column] = value;
  return cells.join("\t");
}

const TS = 0;
const SESSION = 1;
const PHASE = 2;
const EVENT = 3;
const TARGET = 4;
const STATUS = 5;
const WHY = 6;
const EVIDENCE = 7;
const NEXT = 8;

function messages(body: string): string[] {
  return checkStateFile(REL, body).map((one) => one.message);
}

test("the family is named state-tsv and reports its one rule", () => {
  assert.equal(FAMILY.name, "state-tsv");
  assert.deepEqual(FAMILY.rules, [RULE]);
});

test("every failure carries the rule and the path it was given", () => {
  const failures = checkStateFile(REL, stateFile(withCell(TS, "yesterday")));
  assert.equal(failures.length, 1);
  assert.equal(failures[0]?.rule, RULE);
  assert.equal(failures[0]?.path, REL);
});

// --- checkStateFile ----------------------------------------------------------------------------

test("a good row is clean", () => {
  assert.deepEqual(messages(stateFile(GOOD_ROW)), []);
});

test("a wrong version line names line 1 and both wanted first lines", () => {
  assert.deepEqual(
    messages(stateFile(GOOD_ROW).replace(VERSION, "# flow-state 3")),
    [
      'line 1 is "# flow-state 3", want the header row or the version line "# flow-state 1"',
    ],
  );
});

test("a file whose first line is the version 2 line fails on line 1", () => {
  assert.deepEqual(
    messages(stateFile(GOOD_ROW).replace(VERSION, "# flow-state 2")),
    [
      'line 1 is "# flow-state 2", want the header row or the version line "# flow-state 1"',
    ],
  );
});

test("a file that starts with the header row is clean, alone or merged with a version 1 file in either order", () => {
  const headerFirst = `${HEADER}\n${GOOD_ROW}\n`;
  assert.deepEqual(messages(headerFirst), []);
  assert.deepEqual(messages(`${HEADER}\n`), []);
  assert.deepEqual(messages(headerFirst + stateFile(GOOD_ROW)), []);
  assert.deepEqual(messages(stateFile(GOOD_ROW) + headerFirst), []);
});

test("a header-first file numbers its rows from line 2", () => {
  assert.deepEqual(messages(`${HEADER}\n${withCell(PHASE, "plan")}\n`), [
    'line 2: phase "plan" is outside the closed set',
  ]);
});

test("a line 1 that is another header row fails on line 1 and leaves the rows checked", () => {
  const other = HEADER.replace("session", "sess");
  assert.deepEqual(messages(`${other}\n${GOOD_ROW}\n`), [
    `line 1 is "${other.replaceAll("\t", `${BS}t`)}", want the header row or the version line "# flow-state 1"`,
  ]);
});

test("a merge that repeats the version 1 line mid-file is clean, and any other version line is a row failure", () => {
  assert.deepEqual(
    messages(
      `${HEADER}\n${GOOD_ROW}\n${VERSION}\n${HEADER}\n${GOOD_ROW}\n${HEADER}\n`,
    ),
    [],
  );
  assert.deepEqual(messages(stateFile(GOOD_ROW, "# flow-state 2", GOOD_ROW)), [
    "line 4 has 1 cells, want 9",
  ]);
});

// --- the events version 2 adds --------------------------------------------------------------------

const ORDER_ROW =
  "2026-11-12T10:00:00Z\ts1\texecute\torder\t3\tok\tstanding order\tentry=abc123\texecute next";
const COMMIT_ROW =
  "2026-11-12T10:01:00Z\ts1\texecute\tcommit\tfix\tok\treview fix\tcommit=0123abc patch-id=4567def\texecute next";
const DROP_ROW =
  "2026-11-12T10:02:00Z\ts1\texecute\tdrop\ttask-7\tcancelled\tout of scope\tnone\texecute next";
const NOTE_ROW =
  "2026-11-12T10:03:00Z\ts1\tdecompose\tnote\tslices\tok\tslice list\tslices=S1,S2 final=yes\texecute start";

test("rows with the order, commit, drop and note events are clean", () => {
  assert.deepEqual(
    messages(stateFile(ORDER_ROW, COMMIT_ROW, DROP_ROW, NOTE_ROW)),
    [],
  );
});

test("a note row with status failed is clean", () => {
  assert.deepEqual(
    messages(
      stateFile(NOTE_ROW.replace("\tslices\tok\t", "\tcoverage\tfailed\t")),
    ),
    [],
  );
});

test("a commit row accepts each of the four kinds as its target", () => {
  for (const kind of ["fix", "triage", "user", "housekeeping"])
    assert.deepEqual(
      messages(stateFile(COMMIT_ROW.replace("\tfix\t", `\t${kind}\t`))),
      [],
      kind,
    );
});

test("a drop row with a status other than cancelled fails", () => {
  for (const status of ["ok", "failed", "pending"])
    assert.deepEqual(
      messages(stateFile(DROP_ROW.replace("\tcancelled\t", `\t${status}\t`))),
      [`line 3: a drop row has status "${status}", want cancelled`],
      status,
    );
});

test("a commit row whose target is not one of the four kinds fails", () => {
  for (const target of ["other", "Fix", "task-1"])
    assert.deepEqual(
      messages(stateFile(COMMIT_ROW.replace("\tfix\t", `\t${target}\t`))),
      [
        `line 3: a commit row has target "${target}", want fix, triage, user or housekeeping`,
      ],
      target,
    );
});

test("the drop and commit rules read only their own event", () => {
  // The status rule belongs to drop and the target rule to commit; neither applies to the other or to a third event.
  assert.deepEqual(
    messages(
      stateFile(
        COMMIT_ROW.replace("\tok\t", "\tcancelled\t"),
        DROP_ROW.replace("\ttask-7\t", "\thousekeeping\t"),
        withCell(TARGET, "other"),
      ),
    ),
    [],
  );
});

test("a drop row with a bad status reports the closed-set failure and the drop failure", () => {
  assert.deepEqual(
    messages(stateFile(DROP_ROW.replace("\tcancelled\t", "\tstaged\t"))),
    [
      'line 3: status "staged" is outside the closed set',
      'line 3: a drop row has status "staged", want cancelled',
    ],
  );
});

test("an approve row with no hash in its evidence is clean", () => {
  // Committed files hold approve rows whose evidence is a pointer or text and carries no hash.
  const approve =
    "2026-11-12T10:04:00Z\ts1\tspec\tapprove\tspec.md\tok\tuser approved\tuser said yes\tdecompose plan";
  const declined =
    "2026-11-12T10:05:00Z\ts1\tspec\tapprove\tspec.md\tdeclined\tuser said no\tnone\tspec define";
  assert.deepEqual(messages(stateFile(approve, declined)), []);
});

test("header drift is a line 2 failure", () => {
  assert.deepEqual(messages(stateFile(GOOD_ROW).replace("session", "sess")), [
    "line 2 is not the header row",
  ]);
});

test("an empty file and only newlines fail on line 1, and a lone version line fails on line 2", () => {
  for (const body of ["", "\n\n\n"]) {
    assert.deepEqual(
      messages(body),
      [
        'line 1 is "", want the header row or the version line "# flow-state 1"',
      ],
      JSON.stringify(body),
    );
  }
  for (const body of [`${VERSION}\n`, VERSION]) {
    assert.deepEqual(
      messages(body),
      ["line 2 is not the header row"],
      JSON.stringify(body),
    );
  }
});

test("a version line and header with no row are clean", () => {
  assert.deepEqual(messages(`${VERSION}\n${HEADER}\n`), []);
});

test("a byte order mark before the version line is kept as it is in the quoted line", () => {
  assert.deepEqual(messages(BOM + stateFile(GOOD_ROW)), [
    `line 1 is "${BOM}# flow-state 1", want the header row or the version line "# flow-state 1"`,
  ]);
});

test("a CRLF file fails on its first line, its header and its row, and never on a trimmed carriage return", () => {
  assert.deepEqual(messages(stateFile(GOOD_ROW).replaceAll("\n", "\r\n")), [
    `line 1 is "# flow-state 1${BS}r", want the header row or the version line "# flow-state 1"`,
    "line 2 is not the header row",
    "line 3 contains a carriage return",
  ]);
});

test("a carriage return inside a cell is a line failure that skips the other row checks", () => {
  assert.deepEqual(messages(stateFile(withCell(WHY, "cap\rture"))), [
    "line 3 contains a carriage return",
  ]);
  // The row is also short of a phase, which would be reported if the carriage return did not end the row's checks.
  assert.deepEqual(messages(stateFile("a\rb\tc")), [
    "line 3 contains a carriage return",
  ]);
});

test("a short row, a blank row and a long row are cell-count failures that end the row's checks", () => {
  assert.deepEqual(
    messages(stateFile(GOOD_ROW.split("\t").slice(0, 8).join("\t"))),
    ["line 3 has 8 cells, want 9"],
  );
  assert.deepEqual(messages(stateFile(GOOD_ROW, "", GOOD_ROW)), [
    "line 4 has 1 cells, want 9",
  ]);
  assert.deepEqual(messages(stateFile(`${GOOD_ROW}\textra`)), [
    "line 3 has 10 cells, want 9",
  ]);
});

test("a union merge that repeats the version line and the header is clean", () => {
  const later = GOOD_ROW.replace("10:00:00", "11:00:00");
  const earlier = GOOD_ROW.replace("10:00:00", "09:00:00");
  const body = `${VERSION}\n${HEADER}\n${later}\n${VERSION}\n${HEADER}\n${earlier}\n${VERSION}\n${HEADER}\n`;
  assert.deepEqual(messages(body), []);
});

test("ts must be four-digit 20xx ISO 8601 UTC with no fraction and a capital Z, and is not range checked", () => {
  for (const value of [
    "yesterday",
    "2569-10-14T10:00:00Z",
    "2026-10-14T10:00:00z",
    "2026-10-14T10:00:00.5Z",
    "2026-10-14 10:00:00Z",
    "2026-10-14T10:00:00",
    "2026-10-14T10:00Z",
  ]) {
    assert.deepEqual(
      messages(stateFile(withCell(TS, value))),
      [`line 3: ts "${value}" is not ISO 8601 UTC`],
      value,
    );
  }
  assert.deepEqual(
    messages(stateFile(withCell(TS, "2026-13-45T99:99:99Z"))),
    [],
  );
});

test("a phase, an event and a status outside their closed sets fail with the cell quoted", () => {
  assert.deepEqual(messages(stateFile(withCell(PHASE, "plan"))), [
    'line 3: phase "plan" is outside the closed set',
  ]);
  assert.deepEqual(messages(stateFile(withCell(EVENT, "created"))), [
    'line 3: event "created" is outside the closed set',
  ]);
  assert.deepEqual(messages(stateFile(withCell(STATUS, "staged"))), [
    'line 3: status "staged" is outside the closed set',
  ]);
});

test("every member of the three closed sets is accepted", () => {
  const phases = ["intent", "spec", "decompose", "execute", "gate"];
  const events = [
    "create",
    "challenge",
    "approve",
    "revoke",
    "diverge",
    "verify",
    "revert",
    "demote",
    "block",
    "unblock",
    "pivot",
    "waive",
    "branch",
    "release",
    "change",
    "skip",
    "na",
    "enter",
    "bind",
    "pause",
    "done",
    "review",
    "escape",
  ];
  const statuses = [
    "ok",
    "failed",
    "blocked",
    "pending",
    "declined",
    "cancelled",
  ];
  assert.equal(phases.length, 5);
  assert.equal(events.length, 23);
  assert.equal(statuses.length, 6);
  for (const phase of phases)
    assert.deepEqual(messages(stateFile(withCell(PHASE, phase))), [], phase);
  for (const event of events)
    assert.deepEqual(messages(stateFile(withCell(EVENT, event))), [], event);
  for (const status of statuses)
    assert.deepEqual(messages(stateFile(withCell(STATUS, status))), [], status);
});

test("the review and escape events carry the evidence forms the writer produces", () => {
  const review =
    "2026-10-30T10:00:00Z\ts1\texecute\treview\ttask-1\tok\tquality review\tkind=quality dispatched=2026-10-30T10:00:00Z round=1 blocked=40\tgate check";
  const escape =
    "2026-10-30T10:05:00Z\ts1\texecute\tescape\ttask-1\tfailed\tfound after review\tfound-by=copilot severity=important review=quality\texecute work";
  assert.deepEqual(messages(stateFile(review, escape)), []);
});

test("an empty target, why, evidence or next is reported by cell number, and an empty next also fails the next check", () => {
  assert.deepEqual(messages(stateFile(withCell(TARGET, ""))), [
    "line 3: cell 5 is empty",
  ]);
  assert.deepEqual(messages(stateFile(withCell(WHY, ""))), [
    "line 3: cell 7 is empty",
  ]);
  assert.deepEqual(messages(stateFile(withCell(EVIDENCE, ""))), [
    "line 3: cell 8 is empty",
  ]);
  assert.deepEqual(messages(stateFile(withCell(NEXT, ""))), [
    "line 3: cell 9 is empty",
    'line 3: next "" must start with a phase name or done',
  ]);
});

test("next must start with a phase name or done, as a whole word of the cell", () => {
  assert.deepEqual(messages(stateFile(withCell(NEXT, "review it"))), [
    'line 3: next "review it" must start with a phase name or done',
  ]);
  assert.deepEqual(messages(stateFile(withCell(NEXT, "Gate x"))), [
    'line 3: next "Gate x" must start with a phase name or done',
  ]);
  assert.deepEqual(messages(stateFile(withCell(NEXT, "done later"))), []);
  assert.deepEqual(messages(stateFile(withCell(NEXT, "gate x"))), []);
});

test("whitespace in a cell is reported before the cell's other checks, and the other cells are still checked", () => {
  const whitespace = (column: number, cell: string): string =>
    `line 3: cell ${column + 1} (${cell}) carries leading or trailing whitespace; the writer trims before it applies the formula-injection guard, so whitespace here means the guard was bypassed`;
  assert.deepEqual(messages(stateFile(withCell(EVENT, "create "))), [
    whitespace(EVENT, '"create "'),
    'line 3: event "create " is outside the closed set',
  ]);
  assert.deepEqual(messages(stateFile(withCell(WHY, `${BOM}capture`))), [
    whitespace(WHY, `"${BOM}capture"`),
  ]);
  // JavaScript's trim keeps U+0085, so a cell that starts with it carries no whitespace.
  assert.deepEqual(messages(stateFile(withCell(WHY, `${NEL}capture`))), []);
  // The whitespace failure ends the cell's checks, so a trailing space after a trigger gives no guard failure too.
  assert.deepEqual(messages(stateFile(withCell(WHY, "=1+1 "))), [
    whitespace(WHY, '"=1+1 "'),
  ]);
  // A space hides the trigger from the guard check; the whitespace failure stands in for it.
  assert.deepEqual(messages(stateFile(withCell(WHY, " =1+1"))), [
    whitespace(WHY, '" =1+1"'),
  ]);
});

test("a whitespace failure ends that cell's checks, so a whitespace-only target gives the whitespace and the empty failures", () => {
  assert.deepEqual(messages(stateFile(withCell(TARGET, " "))), [
    'line 3: cell 5 (" ") carries leading or trailing whitespace; the writer trims before it applies the formula-injection guard, so whitespace here means the guard was bypassed',
    "line 3: cell 5 is empty",
  ]);
});

test("a cell beginning with a formula trigger or a lone quote and no guard prefix is a failure", () => {
  for (const value of ["=1+1", "+1", "-1", "@sum", "'capture", "'"]) {
    assert.deepEqual(
      messages(stateFile(withCell(WHY, value))),
      [
        `line 3: cell 7 ("${value}") begins with =, +, -, @, or a quote but carries no guard prefix`,
      ],
      value,
    );
  }
});

test("guarded cells are accepted", () => {
  for (const value of ["'=1+1", "'+1", "'-1", "'@sum", "''quoted", "it's fine"])
    assert.deepEqual(messages(stateFile(withCell(WHY, value))), [], value);
});

test("a lone dash passes in the session column only", () => {
  assert.deepEqual(messages(stateFile(withCell(SESSION, "-"))), []);
  for (const value of ["-1", "-x", "--"])
    assert.deepEqual(
      messages(stateFile(withCell(SESSION, value))),
      [
        `line 3: cell 2 ("${value}") begins with =, +, -, @, or a quote but carries no guard prefix`,
      ],
      value,
    );
  assert.deepEqual(messages(stateFile(withCell(TARGET, "-"))), [
    'line 3: cell 5 ("-") begins with =, +, -, @, or a quote but carries no guard prefix',
  ]);
  assert.deepEqual(messages(stateFile(withCell(WHY, "-"))), [
    'line 3: cell 7 ("-") begins with =, +, -, @, or a quote but carries no guard prefix',
  ]);
});

test("one row with several defects reports them in check order", () => {
  const row = [
    "yesterday",
    "s1",
    "plan",
    "created",
    "",
    "staged",
    "capture",
    "intent.md",
    "",
  ].join("\t");
  assert.deepEqual(messages(stateFile(row)), [
    'line 3: ts "yesterday" is not ISO 8601 UTC',
    'line 3: phase "plan" is outside the closed set',
    'line 3: event "created" is outside the closed set',
    'line 3: status "staged" is outside the closed set',
    "line 3: cell 5 is empty",
    "line 3: cell 9 is empty",
    'line 3: next "" must start with a phase name or done',
  ]);
});

test("a cell guard failure comes before the row's ts failure", () => {
  const row = withCell(TS, "yesterday").replace("capture", "=capture");
  assert.deepEqual(messages(stateFile(row)), [
    'line 3: cell 7 ("=capture") begins with =, +, -, @, or a quote but carries no guard prefix',
    'line 3: ts "yesterday" is not ISO 8601 UTC',
  ]);
});

test("failures name the line of their row", () => {
  const bad = withCell(TS, "yesterday");
  assert.deepEqual(messages(stateFile(bad, GOOD_ROW, bad, GOOD_ROW, bad)), [
    'line 3: ts "yesterday" is not ISO 8601 UTC',
    'line 5: ts "yesterday" is not ISO 8601 UTC',
    'line 7: ts "yesterday" is not ISO 8601 UTC',
  ]);
});

test("trailing newlines are trimmed but a trailing carriage return is not", () => {
  assert.deepEqual(messages(`${stateFile(GOOD_ROW)}\n\n`), []);
  assert.deepEqual(messages(`${VERSION}\n${HEADER}\n${GOOD_ROW}\r\n`), [
    "line 3 contains a carriage return",
  ]);
});

// --- inWorkFolderTemp --------------------------------------------------------------------------

test("inWorkFolderTemp matches a path under a work folder's temp directory only", () => {
  const cases: Array<[string, boolean]> = [
    ["docs/specs/2026-09/feature-x/temp/2026-09/y/plan.md", true],
    ["docs/specs/2026-09/feature-x/temp/notes.md", true],
    ["docs/specs/2026-09/feature-x/temp/2026-09/y/state.tsv", true],
    ["docs/specs/2026-09/feature-x/plan.md", false],
    ["docs/specs/2026-09/feature-x/state.tsv", false],
    ["docs/specs/2026-09/temp/state.tsv", false],
    ["docs/specs/unassigned/temp/state.tsv", false],
    ["temp/notes.md", false],
    ["docs/specs/2026-09/feature-x/template/plan.md", false],
    ["docs/specs/2026-09/feature-x/template/state.tsv", false],
    ["docs/specs/2026-09/Feature-X/temp/state.tsv", false],
    ["docs/specs/2026-09/x/temp", false],
    ["2026-09/feature-x/temp/notes.md", true],
  ];
  for (const [rel, want] of cases)
    assert.equal(inWorkFolderTemp(rel), want, rel);
});

// --- discovery ---------------------------------------------------------------------------------

const audit = (root: string) => run(root, options, { families: [FAMILY] });

function withRoot<T>(body: (root: string) => Promise<T> | T): Promise<T> {
  const root = mkdtempSync(path.join(tmpdir(), "agentics-statetsv-"));
  return Promise.resolve()
    .then(() => body(root))
    .finally(() => removeTree(root));
}

const NO_LINKS = canSymlink() ? false : SYMLINK_SKIP;

/** A state.tsv whose version line carries `tag`, so the failure says which file it came from. */
function bad(tag: string): string {
  return `# flow-state ${tag}\n${HEADER}\n${GOOD_ROW}\n`;
}

function badVersion(tag: string): string {
  return `line 1 is "# flow-state ${tag}", want the header row or the version line "# flow-state 1"`;
}

/** The (path, message) pairs of a run that ended without an audit error. */
async function found(root: string): Promise<string[][]> {
  const result = await audit(root);
  assert.equal(result.error, undefined);
  return result.failures.map((one: Failure) => [one.path, one.message]);
}

test("a clean tree and a tree with no state.tsv pass", async () => {
  await withRoot(async (root) => {
    assert.deepEqual(await found(root), []);
    writeFile(root, REL, stateFile(GOOD_ROW, GOOD_ROW.replace(":00Z", ":01Z")));
    writeFile(root, "docs/specs/unassigned/state.tsv", stateFile(GOOD_ROW));
    assert.deepEqual(await found(root), []);
  });
});

test("a failing file is reported with its path relative to the repository", async () => {
  await withRoot(async (root) => {
    writeFile(root, REL, bad("3"));
    assert.deepEqual(await found(root), [[REL, badVersion("3")]]);
  });
});

test("a docs/specs that is missing, a file, or has a NUL byte in the root passes silently", async () => {
  await withRoot(async (root) => {
    assert.deepEqual(await found(root), []);
    writeFile(root, "docs/specs", bad("3"));
    assert.deepEqual(await found(root), []);
  });
  await withRoot(async (root) => {
    assert.deepEqual(await found(root + String.fromCharCode(0) + "x"), []);
  });
});

test(
  "a docs that is a link, or a docs/specs that is a link, passes silently",
  { skip: NO_LINKS },
  async () => {
    await withRoot((root) =>
      withOutside(async (outside) => {
        writeFile(outside, "specs/bad/state.tsv", bad("3"));
        assert.equal(link(root, "docs", outside, "dir"), true);
        assert.deepEqual(await found(root), []);
      }),
    );
    await withRoot((root) =>
      withOutside(async (outside) => {
        writeFile(outside, "bad/state.tsv", bad("3"));
        mkdir(root, "docs");
        assert.equal(link(root, "docs/specs", outside, "dir"), true);
        assert.deepEqual(await found(root), []);
      }),
    );
  },
);

test("folders are walked directory by directory, not in global path order", async () => {
  await withRoot(async (root) => {
    writeFile(root, "docs/specs/a-b/state.tsv", bad("b"));
    writeFile(root, "docs/specs/a/state.tsv", bad("a"));
    // A global sort of the paths would put a-b first, because "-" sorts before "/".
    assert.deepEqual(await found(root), [
      ["docs/specs/a/state.tsv", badVersion("a")],
      ["docs/specs/a-b/state.tsv", badVersion("b")],
    ]);
  });
});

test(
  "a symlinked state.tsv is never read and a symlinked directory is never entered",
  { skip: NO_LINKS },
  async () => {
    await withRoot((root) =>
      withOutside(async (outside) => {
        writeFile(outside, "leak.tsv", "bad\theader\n");
        writeFile(outside, "linked/state.tsv", bad("leak"));
        writeFile(root, "docs/specs/2026-10/feature-y/state.tsv", bad("y"));
        assert.equal(
          link(
            root,
            "docs/specs/2026-10/feature-x/state.tsv",
            path.join(outside, "leak.tsv"),
            "file",
          ),
          true,
        );
        assert.equal(
          link(
            root,
            "docs/specs/2026-10/linked",
            path.join(outside, "linked"),
            "dir",
          ),
          true,
        );
        assert.deepEqual(await found(root), [
          ["docs/specs/2026-10/feature-y/state.tsv", badVersion("y")],
        ]);
      }),
    );
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
  "a state.tsv that becomes a link after the walk is refused by the read",
  { skip: NO_LINKS },
  async () => {
    await withRoot((root) =>
      withOutside(async (outside) => {
        writeFile(outside, "leak.tsv", bad("leak"));
        writeFile(root, REL, stateFile(GOOD_ROW));
        const file = path.join(root, ...REL.split("/"));
        // The walk lists the file without inspecting it; the link appears before the read inspects it.
        const lstat = linkOnInspection(
          file,
          path.join(outside, "leak.tsv"),
          "file",
          () => true,
        );
        assert.ok(lstat, "the host cannot create a file symlink");
        assert.deepEqual(await withSeams({ lstat }, () => audit(root)), {
          failures: [],
          error: `${REL}: passes through a link`,
        });
      }),
    );
  },
);

test("a work folder's temp directory is exempt, and a temp directory with no purpose folder is not", async () => {
  await withRoot(async (root) => {
    const folder = "docs/specs/2026-09/feature-x";
    writeFile(root, `${folder}/temp/2026-09/y/state.tsv`, "bad\theader\n");
    writeFile(root, `${folder}/temp/state.tsv`, "bad\theader\n");
    writeFile(root, `${folder}/state.tsv`, bad("root"));
    writeFile(root, "docs/specs/2026-09/temp/state.tsv", bad("loose"));
    assert.deepEqual(await found(root), [
      [`${folder}/state.tsv`, badVersion("root")],
      ["docs/specs/2026-09/temp/state.tsv", badVersion("loose")],
    ]);
  });
});

test("no directory is skipped by name", async () => {
  await withRoot(async (root) => {
    for (const dir of [".git", "node_modules", "tests"])
      writeFile(root, `docs/specs/x/${dir}/state.tsv`, bad(dir));
    assert.deepEqual(await found(root), [
      ["docs/specs/x/.git/state.tsv", badVersion(".git")],
      ["docs/specs/x/node_modules/state.tsv", badVersion("node_modules")],
      ["docs/specs/x/tests/state.tsv", badVersion("tests")],
    ]);
  });
});

test("only the exact file name state.tsv is checked, and a directory of that name is not a file", async () => {
  await withRoot(async (root) => {
    writeFile(root, "docs/specs/a/State.tsv", bad("upper"));
    writeFile(root, "docs/specs/b/state.tsv.bak", bad("bak"));
    writeFile(root, "docs/specs/c/state.tsv/notes.md", bad("dir"));
    writeFile(root, "docs/specs/d/mystate.tsv", bad("prefix"));
    assert.deepEqual(await found(root), []);
  });
});

test("an unreadable directory is an audit error naming it, without a skills prefix, and drops the findings so far", async () => {
  await withRoot(async (root) => {
    writeFile(root, "docs/specs/a/state.tsv", bad("a"));
    mkdir(root, "docs/specs/b");
    const denied = path.join(root, "docs", "specs", "b");
    const real = seams.readDir;
    const result = await withSeams(
      {
        readDir: (dir) => {
          if (dir === denied)
            throw Object.assign(new Error("denied"), { code: "EACCES" });
          return real(dir);
        },
      },
      () => audit(root),
    );
    assert.deepEqual(result.failures, []);
    assert.equal(result.error, "docs/specs/b: EACCES");
  });
});

test("a file that cannot be read is an audit error naming it and the error code", async () => {
  await withRoot(async (root) => {
    writeFile(root, REL, bad("3"));
    const result = await withSeams(
      {
        readFile: () => {
          throw Object.assign(new Error("denied"), { code: "EACCES" });
        },
      },
      () => audit(root),
    );
    assert.deepEqual(result.failures, []);
    assert.equal(result.error, `${REL}: EACCES`);
  });
});

// --- guard cells, audit error texts and causes, rows added by the mutation pass -------------------------------------------

test("a cell of two apostrophes, or an apostrophe and a trigger, is already guarded", () => {
  for (const cell of ["''", "'=", "'+", "'-", "'@"])
    assert.deepEqual(messages(stateFile(withCell(TARGET, cell))), [], cell);
  for (const cell of ["'", "'x", "'1"])
    assert.deepEqual(
      messages(stateFile(withCell(TARGET, cell))),
      [
        `line 3: cell 5 (${JSON.stringify(cell)}) begins with =, +, -, @, or a quote but carries no guard prefix`,
      ],
      cell,
    );
});

/** Runs the audit with the read of the state file failing with `error`. */
async function auditFailingRead(root: string, error: unknown) {
  const target = path.join(root, ...REL.split("/"));
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

test("a state file that cannot be read is an audit error naming it and why", async () => {
  await withRoot(async (root) => {
    writeFile(root, REL, stateFile(GOOD_ROW));
    const target = path.join(root, ...REL.split("/"));
    const rows: [string, unknown, string][] = [
      [
        "a coded failure",
        Object.assign(new Error("EACCES: permission denied, lstat"), {
          code: "EACCES",
          syscall: "lstat",
          path: target,
        }),
        `${REL}: EACCES`,
      ],
      [
        "a failure with no code",
        new Error("odd failure"),
        `${REL}: unreadable`,
      ],
      [
        "a file error for another path",
        new RepoFileError("symlink", "somewhere"),
        "somewhere: passes through a link",
      ],
      ["no error object", null, `${REL}: unreadable`],
    ];
    for (const [label, error, text] of rows)
      assert.deepEqual(
        await auditFailingRead(root, error),
        { failures: [], error: text },
        label,
      );
  });
});

test("an audit error keeps the failure that caused it as its cause", async () => {
  await withRoot(async (root) => {
    writeFile(root, REL, stateFile(GOOD_ROW));
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

/** The state-tsv finding for a state file whose version line is `# flow-state 2`. */
const badVersionAt = (rel: string): Failure => ({
  rule: RULE,
  path: rel,
  message:
    'line 1 is "# flow-state 2", want the header row or the version line "# flow-state 1"',
});

test("state-tsv-shape", async () => {
  assert.deepEqual(await auditEveryFamily(breaker("state-tsv-shape")), {
    failures: [badVersionAt("docs/specs/2026-01/feature-x/state.tsv")],
  });
});

test("a bad state file below a node_modules directory", async () => {
  // The walk does not skip a node_modules directory.
  assert.deepEqual(
    await auditEveryFamily(
      put("docs/specs/x/node_modules/state.tsv", BAD_STATE),
    ),
    { failures: [badVersionAt("docs/specs/x/node_modules/state.tsv")] },
  );
});

test("two bad state files in the directories a and a-b", async () => {
  // The walk visits a before a-b, though "a/" sorts after "a-b/" as text.
  assert.deepEqual(
    await auditEveryFamily(
      all(
        put("docs/specs/a-b/state.tsv", BAD_STATE),
        put("docs/specs/a/state.tsv", BAD_STATE),
      ),
    ),
    {
      failures: [
        badVersionAt("docs/specs/a/state.tsv"),
        badVersionAt("docs/specs/a-b/state.tsv"),
      ],
    },
  );
});
