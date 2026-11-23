import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import path from "node:path";
import { test } from "node:test";
import { fileURLToPath } from "node:url";
import { parse } from "yaml";
import { ReleaseEngineModel } from "../../../release/engine-model.ts";
import { repoRoot } from "../../check.ts";
import {
  auditDocs,
  LADDER,
  PIN_ROW,
  PINS,
  withDocsRepo,
  writeFile,
} from "../../testing/docs-repo.ts";
import {
  isKnown,
  LADDER_PATH,
  loadLadder,
  parseLadder,
  PIN_PATH,
} from "./ladder.ts";

test("modelids parse preserves order and pin structures", () => {
  const pins = [
    "| Identifier | Used in | Reason |",
    "| --- | --- | --- |",
    "| `gpt-5.6-luna` | `smoke.go` | release pin |",
  ].join("\n");
  const ladder = parseLadder(LADDER, pins);
  assert.deepEqual(
    [...ladder.surfaces],
    [
      ["Codex", ["gpt-6-luna", "gpt-6.1-sol"]],
      ["Cursor", ["Composer 2.5", "Grok 4.7"]],
    ],
    "surface rows retain every identifier in order, including spaced names",
  );
  assert.deepEqual(
    [...ladder.pins],
    [["gpt-5.6-luna", { usedIn: "`smoke.go`", reason: "release pin" }]],
    "pin metadata retains both cells verbatim",
  );
  for (const id of ["gpt-6.1-sol", "Grok 4.7", "gpt-5.6-luna"])
    assert.equal(isKnown(ladder, id), true, `${id} is known from a row or pin`);
  assert.equal(
    isKnown(ladder, "gpt-9-nova"),
    false,
    "unknown IDs stay unknown",
  );
});

test("modelids load preserves maintainer pin structures", async () => {
  await withDocsRepo(async (root) => {
    writeFile(
      root,
      LADDER_PATH,
      "| Surface | Lower to higher model position |\n| --- | --- |\n| Codex | `gpt-6-luna`, `gpt-6.1-sol` |\n",
    );
    writeFile(
      root,
      PIN_PATH,
      "| Identifier | Used in | Reason |\n| --- | --- | --- |\n| `gpt-5.5` | `evals/README.md` | historical probe |\n",
    );
    const ladder = loadLadder(root);
    assert.deepEqual(
      [...ladder.surfaces],
      [["Codex", ["gpt-6-luna", "gpt-6.1-sol"]]],
      "load reads the order file",
    );
    assert.deepEqual(
      [...ladder.pins],
      [
        [
          "gpt-5.5",
          { usedIn: "`evals/README.md`", reason: "historical probe" },
        ],
      ],
      "load reads the separate maintainer pin file and retains its metadata",
    );
    assert.equal(isKnown(ladder, "gpt-5.5"), true, "loaded pins are known");
  });
});

const checkoutRoot = repoRoot(path.dirname(fileURLToPath(import.meta.url)));

test("modelids defaults follow the checked-in ladder", () => {
  assert.notEqual(checkoutRoot, "", "the test locates its own checkout");
  const ladder = loadLadder(checkoutRoot);
  const record = parse(
    readFileSync(path.join(checkoutRoot, "evals", "skills.yaml"), "utf8"),
  ) as { settings: { "copilot-model": string } };
  const row = ladder.surfaces.get("Copilot CLI and VS Code") ?? [];
  for (const id of [record.settings["copilot-model"], ReleaseEngineModel])
    assert.ok(row.includes(id), `${id} must be on the actual Copilot row`);
});

test("modelids Copilot default literal is first", () => {
  const defaultImplementer = "gpt-6-luna";
  assert.notEqual(checkoutRoot, "", "the test locates its own checkout");
  const row = loadLadder(checkoutRoot).surfaces.get("Copilot CLI and VS Code");
  assert.equal(
    row?.[0],
    defaultImplementer,
    "the first Copilot rung is the established default implementer",
  );
});

test("modelids checked-in ladder carries the Antigravity model order", () => {
  assert.notEqual(checkoutRoot, "", "the test locates its own checkout");
  const ladder = loadLadder(checkoutRoot);
  assert.deepEqual(
    ladder.surfaces.get("Antigravity"),
    ["gemini-4-hydrogen", "gemini-4-neon", "gemini-4-carbon"],
    "Antigravity rungs are ordered from lower to higher",
  );
  for (const id of ["gemini-4-hydrogen", "gemini-4-neon", "gemini-4-carbon"])
    assert.equal(isKnown(ladder, id), true, `${id} is known on the ladder`);
});

test("modelids ignores unquoted annotations", () => {
  const ladder = parseLadder(
    [
      "| Surface | Lower to higher model position |",
      "| --- | --- |",
      "| Copilot | `gpt-6-luna` (default, high effort) ≈ `claude-haiku-5.5` (user pin only, long context) → the session driver → `gpt-6.1-sol` (high effort) and `claude-opus-5.5` → `Composer 2.5` |",
    ].join("\n"),
    "| Identifier | Used in | Reason |\n| --- | --- | --- |\n",
  );
  assert.deepEqual(
    ladder.surfaces.get("Copilot"),
    [
      "gpt-6-luna",
      "claude-haiku-5.5",
      "gpt-6.1-sol",
      "claude-opus-5.5",
      "Composer 2.5",
    ],
    "only backticked IDs form rungs; annotations never do",
  );
});

/** Runs `docs` on a fixture whose ladder file and pin file hold the given text. */
async function audit(ladder: string, pins: string) {
  return withDocsRepo(async (root) => {
    writeFile(root, LADDER_PATH, ladder);
    writeFile(root, PIN_PATH, pins);
    return auditDocs(root);
  });
}

test("CRLF text parses like LF", async () => {
  const result = await audit(
    LADDER.replaceAll("\n", "\r\n"),
    PINS.replaceAll("\n", "\r\n"),
  );
  assert.deepEqual(result, { failures: [] });
});

test("a table ends at the first line that is not a table row", async () => {
  const result = await audit(`${LADDER}\nNot a row | here |\n`, PINS);
  assert.deepEqual(result, { failures: [] });
});

test("an empty pin table is valid: the unpinned identifier is a failure, not an audit error", async () => {
  const result = await audit(LADDER, PINS.replace(PIN_ROW, ""));
  assert.equal(result.error, undefined);
  assert.deepEqual(
    result.failures.map((f) => f.rule),
    ["model-ids-known", "model-ids-known"],
  );
});

const rejects: Array<{
  name: string;
  ladder: string;
  pins: string;
  error: RegExp;
}> = [
  {
    name: "no order table",
    ladder: "# Ladders",
    pins: PINS,
    error: new RegExp(
      `${LADDER_PATH} has 0 tables headed "Surface \\| Lower to higher model position", want 1`,
    ),
  },
  {
    name: "two order tables",
    ladder: `${LADDER}\n${LADDER}`,
    pins: PINS,
    error: /has 2 tables headed "Surface/,
  },
  {
    name: "an order table with no rows",
    ladder: "| Surface | Lower to higher model position |\n| --- | --- |\n",
    pins: PINS,
    error: /order table has no rows/,
  },
  {
    name: "an order row with no identifier",
    ladder: `${LADDER}| Gemini | none |\n`,
    pins: PINS,
    error: /order row "Gemini" names no backticked identifier/,
  },
  {
    name: "an order row with a wrong cell count",
    ladder: `${LADDER}| Gemini | \`gemini-3\` | extra |\n`,
    pins: PINS,
    error: /has 3 cells, want 2/,
  },
  {
    name: "no pin table",
    ladder: LADDER,
    pins: "# Pins",
    error: new RegExp(
      `${PIN_PATH} has 0 tables headed "Identifier \\| Used in \\| Reason", want 1`,
    ),
  },
  {
    name: "a pin row with two identifiers",
    ladder: LADDER,
    pins: `${PINS}| \`a-1\` \`b-2\` | here | why |\n`,
    error: /pin row .* must name one backticked identifier/,
  },
  {
    name: "a pin row with no reason",
    ladder: LADDER,
    pins: `${PINS}| \`gpt-7\` | here |  |\n`,
    error:
      /must name one backticked identifier, where it is used, and a reason/,
  },
  {
    name: "a duplicate pin",
    ladder: LADDER,
    pins: `${PINS}| \`gpt-5.6-luna\` | there | again |\n`,
    error: /identifier "gpt-5.6-luna" is pinned twice/,
  },
  {
    name: "the pin table inside the ladder file",
    ladder: `${LADDER}\n${PINS}`,
    pins: PINS,
    error: new RegExp(
      `${LADDER_PATH} has 1 tables headed "Identifier .*want 0; the pin table belongs in ${PIN_PATH}`,
    ),
  },
  {
    name: "the order table inside the pin file",
    ladder: LADDER,
    pins: `${PINS}\n${LADDER}`,
    error: new RegExp(
      `${PIN_PATH} has 1 tables headed "Surface .*want 0; the order table belongs in ${LADDER_PATH}`,
    ),
  },
];

for (const { name, ladder, pins, error } of rejects) {
  test(`a ladder or pin file with ${name} is an audit error`, async () => {
    const result = await audit(ladder, pins);
    assert.match(result.error ?? "", error);
    assert.deepEqual(result.failures, []);
  });
}

test("a line holding only the first header cells is not a table header", () => {
  const ladder = parseLadder(
    `${LADDER}\n| Surface |\n`,
    `${PINS}\n| Identifier | Used in |\n`,
  );
  assert.deepEqual(
    [...ladder.surfaces.keys()],
    ["Codex", "Cursor"],
    "a line with fewer cells than a header neither opens nor duplicates a table",
  );
  assert.deepEqual([...ladder.pins.keys()], ["gpt-5.6-luna"]);
});

test("table cells are trimmed as String.prototype.trim trims", () => {
  const ladder = parseLadder(
    `${LADDER}|\u{feff}Gemini\u{feff}| \`gemini-3\` |\n|\u{85}| \`gpt-7\` |\n`,
    PINS,
  );
  assert.deepEqual(
    [...ladder.surfaces.keys()],
    ["Codex", "Cursor", "Gemini", "\u{85}"],
    "U+FEFF is trimmed and U+0085 is kept",
  );
});

test("an order row with an empty surface cell is an error even when it names an identifier", () => {
  assert.throws(() => parseLadder(`${LADDER}|  | \`gpt-7\` |\n`, PINS), {
    message: 'modelids: order row "" names no backticked identifier',
  });
});

test("a pin row error quotes the row's identifier cell", () => {
  assert.throws(
    () => parseLadder(LADDER, `${PINS}| \`a-1\` \`b-2\` | here | why |\n`),
    {
      message:
        'modelids: pin row "`a-1` `b-2`" must name one backticked identifier, where it is used, and a reason',
    },
  );
});
