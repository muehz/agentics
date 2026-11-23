import assert from "node:assert/strict";
import { existsSync, mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { test, type TestContext } from "node:test";
import type { AuditResult, Failure, RunOptions } from "../../check.ts";
import {
  auditEveryFamily,
  auditSlice,
  sliceNamed,
  breaker,
  crlf,
  PROTOTYPE_REFS,
  put,
  ref,
} from "../../testing/skills-seeds.ts";
import { link, mkdir, skillMD, writeFile } from "../../testing/skills-repo.ts";
import { run } from "./index.ts";
import {
  FAMILY,
  LEGACY_NAME_TOKENS,
  LEGACY_NAMES_RULE,
  legacyNamesRules,
} from "./legacy-names.ts";
import { linkDirectory, removeTree } from "../../testing/host.ts";

const options: RunOptions = {
  short: false,
  fast: false,
  diagnostics: () => {},
  env: process.env,
};

const TAIL =
  ": this state mechanism is retired; use state.tsv and the flow helper";
const NO_FILES = "skills: no files under skills/ to audit for legacy names";
const LEAK = "skills/example/references/leak.md";

const audit = (root: string): Promise<AuditResult> =>
  run(root, options, { families: [FAMILY] });

async function failuresOf(root: string): Promise<Failure[]> {
  const result = await audit(root);
  assert.equal(result.error, undefined);
  return result.failures;
}

const names = (line: number, token: string): string =>
  `line ${line} names ${JSON.stringify(token)}${TAIL}`;

const brief = (found: Failure[]): string[][] =>
  found.map((one) => [one.rule, one.path, one.message]);

/** A minimal skills tree free of every legacy name, so a failure names this rule alone. */
function withCleanRepo<T>(body: (root: string) => T): T {
  const root = mkdtempSync(path.join(tmpdir(), "agentics-legacynames-"));
  const cleanup = (): void => removeTree(root);
  let result: T;
  try {
    writeFile(root, "skills/example/SKILL.md", skillMD("example"));
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

/** The names a vector input may use for the characters the source keeps out of string literals. */
const PLACEHOLDERS: Readonly<Record<string, number>> = {
  LS: 0x2028,
  PS: 0x2029,
  NBSP: 0xa0,
  EACUTE: 0xe9,
  BOM: 0xfeff,
  NEL: 0x85,
  EMOJI: 0x1f600,
  KELVIN: 0x212a,
  LONGS: 0x17f,
};

function expand(input: string): string {
  return input.replace(/\{([A-Z]+)\}/g, (whole, key: string) => {
    const code = PLACEHOLDERS[key];
    return code === undefined ? whole : String.fromCodePoint(code);
  });
}

type Vector = readonly [string, ReadonlyArray<ReadonlyArray<string | number>>];

test("the family is named legacy-names and reports skill-legacy-names", () => {
  assert.equal(FAMILY.name, "legacy-names");
  assert.deepEqual(FAMILY.rules, ["skill-legacy-names"]);
  assert.equal(LEGACY_NAMES_RULE, "skill-legacy-names");
});

const FLAGGED: ReadonlyArray<readonly [string, string, number, string]> = [
  ["chain state", "Call run the chain state verb.", 1, "chain state"],
  ["internal chain", "The package is internal/chain.", 1, "internal/chain"],
  ["events tsv", "Append to events.tsv.", 1, "events.tsv"],
  ["log row", "Use log-row.sh.", 1, "log-row"],
  ["later line", "First.\nSecond uses events.tsv.", 2, "events.tsv"],
];

for (const [name, body, line, token] of FLAGGED) {
  test(`flags ${name} as one finding at the file with line ${line}`, async () => {
    await withCleanRepo(async (root) => {
      writeFile(root, LEAK, body);
      assert.deepEqual(await failuresOf(root), [
        { rule: LEGACY_NAMES_RULE, path: LEAK, message: names(line, token) },
      ]);
    });
  });
}

test("a file with chain state on line 2 names the line and the token", async () => {
  await withCleanRepo(async (root) => {
    writeFile(root, LEAK, "First.\nSee chain state here.\n");
    assert.deepEqual(await failuresOf(root), [
      {
        rule: "skill-legacy-names",
        path: LEAK,
        message:
          'line 2 names "chain state": this state mechanism is retired; use state.tsv and the flow helper',
      },
    ]);
  });
});

test("each of internal/chain, events.tsv and log-row gives one finding", async () => {
  await withCleanRepo(async (root) => {
    writeFile(root, LEAK, "internal/chain\nevents.tsv\nlog-row\n");
    assert.deepEqual(
      (await failuresOf(root)).map((one) => one.message),
      [names(1, "internal/chain"), names(2, "events.tsv"), names(3, "log-row")],
    );
  });
});

const NEAR_MISSES: ReadonlyArray<readonly [string, string]> = [
  ["supply chain", "Review the supply chain."],
  ["chain of tasks", "A chain of dependent tasks."],
  ["keychain state", 'echo "=== Keychain state: ==="'],
  ["state tsv", "Write state.tsv."],
  ["flow helper", "Run agx flow status."],
  ["events tsv with a suffix", "Open events.tsvx."],
  ["upper-case", "Chain State and EVENTS.TSV and LOG-ROW."],
];

for (const [name, body] of NEAR_MISSES) {
  test(`allows the bare word and near misses: ${name}`, async () => {
    await withCleanRepo(async (root) => {
      writeFile(root, "skills/example/references/ok.md", body);
      assert.deepEqual(await failuresOf(root), []);
    });
  });
}

test("two tokens on lines 2 and 10 are ordered as strings, line 10 first", async () => {
  await withCleanRepo(async (root) => {
    const lines = Array.from({ length: 10 }, () => "filler");
    lines[1] = "events.tsv";
    lines[9] = "events.tsv";
    writeFile(root, LEAK, lines.join("\n"));
    assert.deepEqual(
      (await failuresOf(root)).map((one) => one.message),
      [names(10, "events.tsv"), names(2, "events.tsv")],
    );
  });
});

test("findings are ordered by path first and keep every occurrence", async () => {
  await withCleanRepo(async (root) => {
    writeFile(root, "skills/zeta/x.md", "log-row log-row");
    writeFile(root, "skills/alpha/x.md", "x\nx\nx\nx\nx\nx\nx\nx\nx\nlog-row");
    assert.deepEqual(brief(await failuresOf(root)), [
      [LEGACY_NAMES_RULE, "skills/alpha/x.md", names(10, "log-row")],
      [LEGACY_NAMES_RULE, "skills/zeta/x.md", names(1, "log-row")],
      [LEGACY_NAMES_RULE, "skills/zeta/x.md", names(1, "log-row")],
    ]);
  });
});

test("the top-level skills README is skipped and a nested README is scanned", async () => {
  await withCleanRepo(async (root) => {
    writeFile(
      root,
      "skills/README.md",
      "events.tsv and log-row are described here.",
    );
    assert.deepEqual(await failuresOf(root), []);
    writeFile(root, "skills/example/README.md", "events.tsv");
    assert.deepEqual(brief(await failuresOf(root)), [
      [LEGACY_NAMES_RULE, "skills/example/README.md", names(1, "events.tsv")],
    ]);
  });
});

test("a file with a NUL byte is skipped", async () => {
  await withCleanRepo(async (root) => {
    writeFile(root, "skills/example/assets/blob.bin", "events.tsv\x00log-row");
    assert.deepEqual(await failuresOf(root), []);
  });
});

test("a symlinked file is skipped", async (t) => {
  await withCleanRepo(async (root) => {
    writeFile(root, "outside/leak.md", "events.tsv");
    const target = path.join(root, "outside", "leak.md");
    if (!link(root, "skills/example/leak.md", target, "file"))
      return skipForLink(t);
    assert.deepEqual(await failuresOf(root), []);
  });
});

test("line numbers count LF only", async () => {
  await withCleanRepo(async (root) => {
    writeFile(root, LEAK, "a\r\nb\rc\r\nlog-row\r\n");
    assert.deepEqual(
      (await failuresOf(root)).map((one) => one.message),
      [names(3, "log-row")],
    );
  });
});

test("nothing scannable is the no-files audit error", async () => {
  const root = mkdtempSync(path.join(tmpdir(), "agentics-legacynames-empty-"));
  try {
    writeFile(root, "skills/README.md", "# Skills\n");
    assert.deepEqual(await audit(root), { failures: [], error: NO_FILES });
    writeFile(root, "skills/example/blob.bin", "events.tsv\x00");
    assert.deepEqual(await audit(root), { failures: [], error: NO_FILES });
    rmSync(path.join(root, "skills"), { recursive: true, force: true });
    mkdir(root, "skills");
    assert.deepEqual(await audit(root), { failures: [], error: NO_FILES });
  } finally {
    removeTree(root);
  }
});

test("a skills directory that is a link is the no-files audit error", async (t) => {
  const root = mkdtempSync(path.join(tmpdir(), "agentics-legacynames-link-"));
  try {
    writeFile(root, "outside/example/SKILL.md", "events.tsv");
    if (
      linkDirectory(path.join(root, "outside"), path.join(root, "skills")) ===
      undefined
    )
      return skipForLink(t);
    assert.deepEqual(await audit(root), { failures: [], error: NO_FILES });
  } finally {
    removeTree(root);
  }
});

test("a missing skills directory is the missing file error", async () => {
  const root = mkdtempSync(
    path.join(tmpdir(), "agentics-legacynames-missing-"),
  );
  try {
    const result = await audit(root);
    assert.deepEqual(result.failures, []);
    assert.equal(result.error, "skills: does not exist");
  } finally {
    removeTree(root);
  }
});

test("this checkout names no retired state mechanism", (t) => {
  const root = path.resolve(
    import.meta.dirname,
    "..",
    "..",
    "..",
    "..",
    "..",
    "..",
  );
  if (!existsSync(path.join(root, "skills", "execute", "SKILL.md"))) {
    t.skip(`real tree not reachable from ${import.meta.dirname}`);
    return;
  }
  assert.deepEqual(legacyNamesRules(root), []);
});

export const legacyVectors: readonly Vector[] = [
  ["chain state", [[0, "chain state"]]],
  ["Chain state", []],
  ["chain  state", []],
  ["chain\nstate", []],
  ["chain state,", [[0, "chain state"]]],
  ["(chain state)", [[1, "chain state"]]],
  ["chain statex", []],
  ["xchain state", []],
  ["_chain state", []],
  ["-chain state", [[1, "chain state"]]],
  ["9chain state", []],
  ["Keychain state", []],
  ["supply chain", []],
  ["chain of dependent tasks", []],
  ["internal/chain", [[0, "internal/chain"]]],
  ["internal/chains", []],
  ["internal/chain/x", [[0, "internal/chain"]]],
  ["internal/chain-x", [[0, "internal/chain"]]],
  ["xinternal/chain", []],
  ["a/internal/chain", [[2, "internal/chain"]]],
  ["events.tsv", [[0, "events.tsv"]]],
  ["events.tsvx", []],
  ["xevents.tsv", []],
  ["events_tsv", []],
  ["eventsXtsv", []],
  ["state.tsv", []],
  ["log-row", [[0, "log-row"]]],
  ["log-rows", []],
  ["log-row.sh", [[0, "log-row"]]],
  ["backlog-row", []],
  ["log-row_x", []],
  [
    "chain state chain state",
    [
      [0, "chain state"],
      [12, "chain state"],
    ],
  ],
  ["{EACUTE}chain state", [[1, "chain state"]]],
  ["chain state{EACUTE}", [[0, "chain state"]]],
  ["{KELVIN}chain state", [[1, "chain state"]]],
  ["{LONGS}chain state", [[1, "chain state"]]],
  ["chain state{LONGS}", [[0, "chain state"]]],
  ["x\rchain state", [[2, "chain state"]]],
  ["x{LS}events.tsv", [[2, "events.tsv"]]],
  ["x{NBSP}log-row", [[2, "log-row"]]],
  ["{BOM}internal/chain", [[1, "internal/chain"]]],
  [
    "chain state\r\nlog-row\r\nevents.tsv",
    [
      [0, "chain state"],
      [13, "log-row"],
      [22, "events.tsv"],
    ],
  ],
];

test("legacyNameTokens matches each token span, with ASCII word boundaries only", () => {
  assert.equal(legacyVectors.length, 42);
  for (const [raw, expected] of legacyVectors) {
    const input = expand(raw);
    const actual = [...input.matchAll(LEGACY_NAME_TOKENS)].map((match) => [
      match.index,
      match[0],
    ]);
    assert.deepEqual(actual, expected, JSON.stringify(raw));
  }
});

// --- the whole-domain scenarios ------------------------------------------------------------------------------------

/** The slice whose families the slice-only scenarios below run: the legacy-names family and its neighbours. */
const CONTENT_SLICE = sliceNamed("content");

/** The legacy-names findings on `rel`, one per `[line, token]`, in the order given. */
const legacyOf = (
  rel: string,
  ...found: [line: number, token: string][]
): AuditResult => ({
  failures: found.map(([line, token]) => ({
    rule: LEGACY_NAMES_RULE,
    path: rel,
    message: names(line, token),
  })),
});

const PROTOTYPE_A = `${PROTOTYPE_REFS}/a.md`;

test("skill-legacy-names", async () => {
  assert.deepEqual(
    await auditEveryFamily(breaker("skill-legacy-names")),
    legacyOf(`${PROTOTYPE_REFS}/state-notes.md`, [3, "events.tsv"]),
  );
});

test("supply chain state", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      ref("a.md", "The supply chain state is not.\n"),
    ),
    legacyOf(PROTOTYPE_A, [1, "chain state"]),
  );
});

test("the four retired names", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      ref("a.md", "chain state\ninternal/chain\nevents.tsv\nlog-row\n"),
    ),
    legacyOf(
      PROTOTYPE_A,
      [1, "chain state"],
      [2, "internal/chain"],
      [3, "events.tsv"],
      [4, "log-row"],
    ),
  );
});

test("retired names on line 10 and line 2", async () => {
  // The messages sort as text, so line 10 comes before line 2.
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      ref(
        "a.md",
        "# A\nUse events.tsv.\nl3\nl4\nl5\nl6\nl7\nl8\nl9\nUse log-row.\n",
      ),
    ),
    legacyOf(PROTOTYPE_A, [10, "log-row"], [2, "events.tsv"]),
  );
});

test("a nested README naming a retired name", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      put("skills/prototype/README.md", "Rows go to events.tsv.\n"),
    ),
    legacyOf("skills/prototype/README.md", [1, "events.tsv"]),
  );
});

test("a retired name in a CRLF file", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      ref("a.md", crlf("# A\nevents.tsv\nlog-row\n")),
    ),
    legacyOf(PROTOTYPE_A, [2, "events.tsv"], [3, "log-row"]),
  );
});

test("a retired name in a tests directory", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      put("skills/prototype/references/tests/t.md", "events.tsv\n"),
    ),
    legacyOf("skills/prototype/references/tests/t.md", [1, "events.tsv"]),
  );
});
