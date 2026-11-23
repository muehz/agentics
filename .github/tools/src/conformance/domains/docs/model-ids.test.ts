import assert from "node:assert/strict";
import fs, { readFileSync, rmSync } from "node:fs";
import { syncBuiltinESMExports } from "node:module";
import path from "node:path";
import { test, type TestContext } from "node:test";
import { fileURLToPath } from "node:url";
import { repoRoot, type Failure } from "../../check.ts";
import { STAMP } from "../../testing/docs-seeds.ts";
import {
  auditDocs,
  FIXED_FILES,
  GO_ROOTS,
  PIN_ROW,
  PINS,
  withDocsRepo,
  writeFile,
} from "../../testing/docs-repo.ts";
import { isKnown, LADDER_PATH, loadLadder, PIN_PATH } from "./ladder.ts";
import { findModelIds, modelIdRules } from "./model-ids.ts";
import { withOutside } from "../../testing/host.ts";
const failuresOf = async (root: string): Promise<Failure[]> => {
  const result = await auditDocs(root);
  assert.equal(result.error, undefined);
  return result.failures;
};

/** Exactly one model-ids-known failure, on `file`, naming `id`. */
function assertOnlyUnknown(failures: Failure[], file: string, id: string) {
  assert.equal(failures.length, 1, JSON.stringify(failures));
  assert.equal(failures[0]?.rule, "model-ids-known");
  assert.equal(failures[0]?.path, file);
  assert.ok(failures[0]?.message.includes(JSON.stringify(id)));
}

function assertOnlyUnusedPin(failures: Failure[], id: string) {
  assert.equal(failures.length, 1, JSON.stringify(failures));
  assert.equal(failures[0]?.rule, "model-ids-pin-used");
  assert.equal(failures[0]?.path, PIN_PATH);
  assert.ok(
    failures[0]?.message.includes(
      `${JSON.stringify(id)}, which no file model-ids-known reads names`,
    ),
  );
}

test("the fixture is green: a comment and a wrapped display name are not findings", async () => {
  await withDocsRepo(async (root) => {
    assert.deepEqual(await failuresOf(root), []);
  });
});

test("bare aliases and identifiers inside longer words are not model identifiers", async () => {
  await withDocsRepo(async (root) => {
    writeFile(
      root,
      "docs/surfaces/cursor.md",
      "opus, sonnet, haiku and fable are aliases. xgpt-9 and my_gpt-9 and v1.gpt-9 are not identifiers.\n\nVerified on Cursor 3.15.6 (2026-08-09).\n",
    );
    assert.deepEqual(await failuresOf(root), []);
  });
});

test("Antigravity native aliases stay clean while unpinned gemini IDs fire model-ids-known", async () => {
  assert.deepEqual(findModelIds("inherit, flash_lite, flash, pro"), []);
  await withDocsRepo(async (root) => {
    writeFile(
      root,
      "docs/surfaces/cursor.md",
      "Native aliases: `inherit`, `flash_lite`, `flash`, and `pro`.\n\nVerified on Cursor 3.15.6 (2026-08-09).\n",
    );
    assert.deepEqual(await failuresOf(root), []);
    writeFile(
      root,
      "docs/surfaces/cursor.md",
      "Unpinned literal: `gemini-4-neon`.\n\nVerified on Cursor 3.15.6 (2026-08-09).\n",
    );
    assertOnlyUnknown(
      await failuresOf(root),
      "docs/surfaces/cursor.md",
      "gemini-4-neon",
    );
  });
});

const checkoutRoot = repoRoot(path.dirname(fileURLToPath(import.meta.url)));

test("Antigravity routing references introduce no unpinned bare gemini model IDs", () => {
  assert.notEqual(checkoutRoot, "", "the test locates its own checkout");
  const dispatchText = readFileSync(
    path.join(
      checkoutRoot,
      "skills",
      "routing",
      "references",
      "surface-dispatch.md",
    ),
    "utf8",
  );
  assert.match(
    dispatchText,
    /^## Antigravity$/m,
    "surface-dispatch.md carries an Antigravity section",
  );
  assert.deepEqual(
    findModelIds(dispatchText),
    [],
    "surface-dispatch.md carries no bare model IDs",
  );

  const ladder = loadLadder(checkoutRoot);
  const ladderText = readFileSync(
    path.join(checkoutRoot, ...LADDER_PATH.split("/")),
    "utf8",
  );
  const unknownGemini = findModelIds(ladderText).filter(
    (id) => id.startsWith("gemini-") && !isKnown(ladder, id),
  );
  assert.deepEqual(
    unknownGemini,
    [],
    "model-ladders.md introduces no unpinned bare gemini-* literals",
  );
});

test("the same unknown identifier in two literals of one Go file is reported once", async () => {
  await withDocsRepo(async (root) => {
    writeFile(
      root,
      "hooks/go/twice.go",
      'package x\n\nconst a = "gpt-9-nova"\nconst b = "use gpt-9-nova"\n',
    );
    assertOnlyUnknown(
      await failuresOf(root),
      "hooks/go/twice.go",
      "gpt-9-nova",
    );
  });
});

test("an unknown identifier in any surface doc fires model-ids-known on that doc", async () => {
  await withDocsRepo(async (root) => {
    writeFile(
      root,
      "docs/surfaces/codex.md",
      "Runs on `gpt-6-luna` and `gpt-9-nova`; the engine stays on gpt-5.6-luna.\n\nVerified on codex-cli 0.147.0 (2026-08-01).\n",
    );
    assertOnlyUnknown(
      await failuresOf(root),
      "docs/surfaces/codex.md",
      "gpt-9-nova",
    );
  });
});

test("every identifier family is recognized", async () => {
  for (const id of [
    "gpt-9-nova",
    "gpt-9.1",
    "claude-sonnet-9",
    "claude-sonnet-9-1",
    "grok-9.1",
    "gemini-9.1-pro",
    "GPT-9",
    "Composer 9",
    "Grok 9.9",
    "Claude Opus 9",
  ]) {
    await withDocsRepo(async (root) => {
      writeFile(
        root,
        "docs/surfaces/cursor.md",
        `Try ${id}.\n\nVerified on Cursor 3.15.6 (2026-08-09).\n`,
      );
      const failures = (await failuresOf(root)).filter(
        (f) => f.path === "docs/surfaces/cursor.md",
      );
      assertOnlyUnknown(failures, "docs/surfaces/cursor.md", id);
    });
  }
});

test("a repeated unknown identifier in one file is reported once", async () => {
  await withDocsRepo(async (root) => {
    writeFile(
      root,
      "docs/surfaces/cursor.md",
      "gpt-9-nova and gpt-9-nova again.\n\nVerified on Cursor 3.15.6 (2026-08-09).\n",
    );
    assertOnlyUnknown(
      (await failuresOf(root)).filter(
        (f) => f.path === "docs/surfaces/cursor.md",
      ),
      "docs/surfaces/cursor.md",
      "gpt-9-nova",
    );
  });
});

test("model ID extraction keeps distinct IDs in first-appearance order", () => {
  assert.deepEqual(findModelIds("gpt-9-nova GPT-9 gpt-9-nova GPT-9"), [
    "gpt-9-nova",
    "GPT-9",
  ]);
});

test("every fixed file is read", async () => {
  for (const rel of FIXED_FILES) {
    await withDocsRepo(async (root) => {
      writeFile(root, rel, 'MODEL="${MODEL:-gpt-9-nova}"\n');
      assertOnlyUnknown(await failuresOf(root), rel, "gpt-9-nova");
    });
  }
});

test("Go string literals are read in the Go module, and comments, tests and testdata are not", async () => {
  for (const goRoot of GO_ROOTS) {
    await withDocsRepo(async (root) => {
      writeFile(
        root,
        `${goRoot}/default.go`,
        "package x\n\nconst model = `gpt-9-nova`\n",
      );
      writeFile(
        root,
        `${goRoot}/default_test.go`,
        'package x\n\nconst fixture = "gpt-9-nova"\n',
      );
      writeFile(
        root,
        `${goRoot}/testdata/fixture.go`,
        'package x\n\nconst fixture = "gpt-9-nova"\n',
      );
      writeFile(
        root,
        `${goRoot}/comment.go`,
        '// gpt-9-nova\n/* gpt-9-nova */\npackage x\n\nconst rune = \'"\'\n// "gpt-9-nova"\n',
      );
      assertOnlyUnknown(
        await failuresOf(root),
        `${goRoot}/default.go`,
        "gpt-9-nova",
      );
    });
  }
});

test("an identifier in an interpreted literal with an escape is read", async () => {
  await withDocsRepo(async (root) => {
    writeFile(
      root,
      "hooks/go/escape.go",
      'package x\n\nconst model = "pre\\n\\tgpt-9-nova\\u0021"\n',
    );
    assertOnlyUnknown(
      await failuresOf(root),
      "hooks/go/escape.go",
      "gpt-9-nova",
    );
  });
});

test("model IDs in nested Go directories are audited", (t) => {
  return withDocsRepo((root) => {
    const rel = "hooks/go/nested/deeper/model.go";
    writeFile(root, rel, 'package x\nconst model = "gpt-9-nova"\n');
    writeFile(
      root,
      "hooks/go/nested/testdata/fixture.go",
      'package x\nconst fixture = "gpt-9-nova"\n',
    );

    // The checkout can allow access to known paths without permitting a root listing.
    const readdir = fs.readdirSync;
    const listing = t.mock.method(
      fs,
      "readdirSync",
      (...args: Parameters<typeof fs.readdirSync>) => {
        if (args[0] === root)
          throw Object.assign(new Error("checkout root cannot be listed"), {
            code: "EACCES",
          });
        return readdir(...args);
      },
    );
    syncBuiltinESMExports();
    try {
      const result = modelIdRules(root);
      assert.equal(result.error, undefined);
      assertOnlyUnknown(result.failures, rel, "gpt-9-nova");
    } finally {
      listing.mock.restore();
      syncBuiltinESMExports();
    }
  });
});

test(
  "a non-Go filename with raw bytes does not interrupt the Go audit",
  {
    skip: process.platform !== "linux" && "requires Linux raw filename bytes",
  },
  () => {
    return withDocsRepo((root) => {
      const rawName = Buffer.concat([
        Buffer.from(path.join(root, "hooks", "go", "ignore-")),
        Buffer.from([0xff]),
        Buffer.from(".txt"),
      ]);
      fs.writeFileSync(rawName, "ignore\n");
      const rel = "hooks/go/model.go";
      writeFile(root, rel, 'package x\nconst model = "gpt-9-nova"\n');

      const result = modelIdRules(root);
      assert.equal(result.error, undefined);
      assertOnlyUnknown(result.failures, rel, "gpt-9-nova");
    });
  },
);

test("Go model ID findings follow lexical order despite unordered directory entries", (t) => {
  return withDocsRepo((root) => {
    const goRoot = path.join(root, "hooks", "go");
    const lexical = ["a.go", "m.go", "z.go"];
    for (const name of lexical) {
      writeFile(
        root,
        `hooks/go/${name}`,
        'package x\nconst id = "gpt-9-nova"\n',
      );
    }

    const readdir = fs.readdirSync;
    const listing = t.mock.method(
      fs,
      "readdirSync",
      (...args: Parameters<typeof fs.readdirSync>) => {
        const entries = readdir(...args);
        if (args[0] !== goRoot || args[1]?.withFileTypes !== true) {
          return entries;
        }
        const order = ["m.go", "z.go", "a.go"];
        return entries.sort(
          (a, b) =>
            order.indexOf(String(a.name)) - order.indexOf(String(b.name)),
        );
      },
    );
    syncBuiltinESMExports();
    try {
      const result = modelIdRules(root);
      assert.equal(result.error, undefined);
      assert.deepEqual(
        result.failures.map(({ rule, path }) => [rule, path]),
        lexical.map((name) => ["model-ids-known", `hooks/go/${name}`]),
      );
    } finally {
      listing.mock.restore();
      syncBuiltinESMExports();
    }
  });
});

test("surface model ID findings follow lexical order despite unordered directory entries", (t) => {
  return withDocsRepo((root) => {
    const surfaces = path.join(root, "docs", "surfaces");
    const files = ["docs/surfaces/alpha.md", "docs/surfaces/zeta.md"];
    for (const rel of files) writeFile(root, rel, "Runs on gpt-9-nova.\n");

    const readdir = fs.readdirSync;
    const listing = t.mock.method(
      fs,
      "readdirSync",
      (...args: Parameters<typeof fs.readdirSync>) => {
        const entries = readdir(...args);
        return args[0] === surfaces ? entries.sort().reverse() : entries;
      },
    );
    syncBuiltinESMExports();
    try {
      const result = modelIdRules(root);
      assert.equal(result.error, undefined);
      assert.deepEqual(
        result.failures.map(({ rule, path }) => [rule, path]),
        files.map((rel) => ["model-ids-known", rel]),
      );
    } finally {
      listing.mock.restore();
      syncBuiltinESMExports();
    }
  });
});

test("an off-ladder identifier needs a pin row", async () => {
  await withDocsRepo(async (root) => {
    writeFile(root, PIN_PATH, PINS.replace(PIN_ROW, ""));
    const failures = await failuresOf(root);
    assert.deepEqual(
      failures.map((f) => [f.rule, f.path]),
      [
        ["model-ids-known", "docs/surfaces/codex.md"],
        ["model-ids-known", "hooks/go/pin.go"],
      ],
    );
    assert.ok(failures.every((f) => f.message.includes('"gpt-5.6-luna"')));
  });
});

test("a pin whose last scanned copy went fires model-ids-pin-used", async () => {
  await withDocsRepo(async (root) => {
    writeFile(
      root,
      "docs/surfaces/codex.md",
      "Runs on `gpt-6-luna`.\n\nVerified on codex-cli 0.147.0 (2026-08-01).\n",
    );
    writeFile(
      root,
      "hooks/go/pin.go",
      'package tools\n\nconst pin = "gpt-6-luna"\n',
    );
    assertOnlyUnusedPin(await failuresOf(root), "gpt-5.6-luna");
  });
});

test("unused model ID pins are reported in identifier order", () => {
  return withDocsRepo((root) => {
    writeFile(
      root,
      PIN_PATH,
      PINS +
        "| `gpt-9-zeta` | `hooks/go/zeta.go` | fixture pin |\n" +
        "| `gpt-9-alpha` | `hooks/go/alpha.go` | fixture pin |\n",
    );

    const result = modelIdRules(root);
    assert.equal(result.error, undefined);
    assert.deepEqual(
      result.failures.map(({ rule, path }) => [rule, path]),
      [
        ["model-ids-pin-used", PIN_PATH],
        ["model-ids-pin-used", PIN_PATH],
      ],
    );
    assert.match(result.failures[0]?.message ?? "", /"gpt-9-alpha"/);
    assert.match(result.failures[1]?.message ?? "", /"gpt-9-zeta"/);
  });
});

test("copies the area does not read cannot keep a pin alive", async () => {
  await withDocsRepo(async (root) => {
    writeFile(
      root,
      "docs/surfaces/codex.md",
      "Runs on `gpt-6-luna`.\n\nVerified on codex-cli 0.147.0 (2026-08-01).\n",
    );
    writeFile(
      root,
      "hooks/go/pin.go",
      'package tools\n\n// gpt-5.6-luna was the old pin.\nconst pin = "gpt-6-luna"\n',
    );
    writeFile(
      root,
      "hooks/go/pin_test.go",
      'package tools\n\nconst fixture = "gpt-5.6-luna"\n',
    );
    writeFile(root, "docs/specs/old-plan.md", "Pinned gpt-5.6-luna.\n");
    assertOnlyUnusedPin(await failuresOf(root), "gpt-5.6-luna");
  });
});

test("one surviving copy in any scanned file keeps a pin alive", async () => {
  for (const rel of FIXED_FILES) {
    await withDocsRepo(async (root) => {
      writeFile(
        root,
        "docs/surfaces/codex.md",
        "Runs on `gpt-6-luna`.\n\nVerified on codex-cli 0.147.0 (2026-08-01).\n",
      );
      writeFile(
        root,
        "hooks/go/pin.go",
        'package tools\n\nconst pin = "gpt-6-luna"\n',
      );
      writeFile(root, rel, 'MODEL="${MODEL:-gpt-5.6-luna}"\n');
      assert.deepEqual(await failuresOf(root), [], rel);
    });
  }
});

test("a missing scanned site fires model-ids-site-present on that site", async () => {
  for (const rel of ["docs/surfaces", ...FIXED_FILES, ...GO_ROOTS]) {
    await withDocsRepo(async (root) => {
      rmSync(path.join(root, ...rel.split("/")), { recursive: true });
      const hit = (await failuresOf(root)).find(
        (f) => f.rule === "model-ids-site-present" && f.path === rel,
      );
      const why =
        rel === "docs/surfaces" ? "holds no surface doc" : "is missing";
      assert.equal(
        hit?.message,
        `${why}; model-ids-known cannot audit it`,
        `removing ${rel} did not fire model-ids-site-present`,
      );
    });
  }
});

test("a Go root with only tests and testdata fires as loudly as a missing one", async () => {
  for (const rel of GO_ROOTS) {
    await withDocsRepo(async (root) => {
      rmSync(path.join(root, ...rel.split("/")), { recursive: true });
      writeFile(root, `${rel}/only_test.go`, "package only\n");
      writeFile(root, `${rel}/testdata/fixture.go`, "package fixture\n");
      const failures = await failuresOf(root);
      assert.equal(failures.length, 1, JSON.stringify(failures));
      assert.equal(failures[0]?.rule, "model-ids-site-present");
      assert.equal(failures[0]?.path, rel);
      assert.match(failures[0]?.message ?? "", /holds no non-test Go source/);
    });
  }
});

test("a Go root that is a file holds no Go source", async () => {
  await withDocsRepo(async (root) => {
    rmSync(path.join(root, "hooks", "go"), { recursive: true });
    writeFile(root, "hooks/go", "not a directory\n");
    assert.deepEqual(
      (await failuresOf(root)).map((f) => [f.rule, f.path]),
      [["model-ids-site-present", "hooks/go"]],
    );
  });
});

/** Makes `at` a link to the directory `target`, or skips the calling test with a logged reason when the host refuses. */
function linkDirOrSkip(t: TestContext, target: string, at: string): boolean {
  try {
    fs.mkdirSync(path.dirname(at), { recursive: true });
    fs.symlinkSync(target, at, "dir");
    return true;
  } catch {
    t.skip("this host cannot create a symlink");
    return false;
  }
}

test("a Go root that is a link to a directory is not entered and holds no Go source", async (t) => {
  await withDocsRepo(async (root) => {
    await withOutside(async (outside) => {
      const goRoot = path.join(root, "hooks", "go");
      writeFile(outside, "go/model.go", 'package x\nconst m = "gpt-9-nova"\n');
      rmSync(goRoot, { recursive: true });
      if (!linkDirOrSkip(t, path.join(outside, "go"), goRoot)) return;
      assert.deepEqual(
        (await failuresOf(root)).map((f) => [f.rule, f.path]),
        [["model-ids-site-present", "hooks/go"]],
      );
    });
  });
});

test("a Go file whose literals cannot be read is an audit error naming the file", async () => {
  for (const [name, source] of [
    ["an unterminated string", 'package x\n\nconst a = "open\n'],
    ["an unterminated raw string", "package x\n\nconst a = `open\n"],
    ["an unterminated comment", "package x\n\n/* open\n"],
    ["an unknown escape", 'package x\n\nconst a = "\\q"\n'],
  ] as const) {
    await withDocsRepo(async (root) => {
      writeFile(root, "hooks/go/broken.go", source);
      const result = await auditDocs(root);
      assert.match(
        result.error ?? "",
        /docs: scanning hooks\/go: hooks\/go\/broken\.go: /,
        name,
      );
    });
  }
});

test("a ladder or pin table that cannot be read is an audit error naming it", async () => {
  await withDocsRepo(async (root) => {
    writeFile(root, LADDER_PATH, "# Model ladders\n");
    assert.match(
      (await auditDocs(root)).error ?? "",
      /0 tables headed "Surface \| Lower to higher model position"/,
    );
  });
  await withDocsRepo(async (root) => {
    writeFile(root, PIN_PATH, "# Model identifiers\n");
    assert.match(
      (await auditDocs(root)).error ?? "",
      new RegExp(
        `${PIN_PATH} has 0 tables headed "Identifier \\| Used in \\| Reason"`,
      ),
    );
  });
  await withDocsRepo(async (root) => {
    rmSync(path.join(root, ...LADDER_PATH.split("/")));
    const result = await auditDocs(root);
    assert.equal(
      result.error,
      "docs: modelids: skills/routing/references/model-ladders.md: does not exist",
    );
    assert.deepEqual(result.failures, []);
  });
});

test("each family reads multi-digit numbers, every version part and every suffix", () => {
  const ids = [
    "gpt-10",
    "gpt-5.10",
    "claude-opus-10",
    "claude-opus-4-10",
    "grok-10",
    "grok-4",
    "grok-4.10",
    "gemini-10",
    "gemini-3",
    "gemini-2.10",
    "gemini-2.5",
    "gemini-2.5-flash-lite",
    "GPT-10",
    "GPT-5.10",
    "Composer 10",
    "Grok 4.10",
  ];
  assert.deepEqual(findModelIds(ids.join(", ")), ids);
});

/** The `model-ids-known` failure for `id` named in `file`. */
const unknownId = (file: string, id: string): Failure => ({
  rule: "model-ids-known",
  path: file,
  message: `names model identifier ${JSON.stringify(id)}, which is on no ladder row and has no pin row in ${PIN_PATH}; add a pin row with its reason, or correct this copy`,
});

/** The `model-ids-site-present` failure for `site`, with the reason it cannot be audited. */
const siteGone = (site: string, reason = "is missing"): Failure => ({
  rule: "model-ids-site-present",
  path: site,
  message: `${reason}; model-ids-known cannot audit it`,
});

test("an unknown identifier in a surface doc", async () => {
  await withDocsRepo(async (root) => {
    writeFile(root, "docs/surfaces/cursor.md", `Try gpt-9-nova.${STAMP}`);
    assert.deepEqual(await auditDocs(root), {
      failures: [unknownId("docs/surfaces/cursor.md", "gpt-9-nova")],
    });
  });
});

test("a display name wrapped across lines", async () => {
  await withDocsRepo(async (root) => {
    writeFile(root, "docs/surfaces/cursor.md", `Try Claude\nOpus 9.${STAMP}`);
    assert.deepEqual(await auditDocs(root), {
      failures: [unknownId("docs/surfaces/cursor.md", "Claude Opus 9")],
    });
  });
});

test("an identifier next to a no-break space", async () => {
  await withDocsRepo(async (root) => {
    writeFile(root, "docs/surfaces/cursor.md", `Try\u{a0}gpt-9-nova.${STAMP}`);
    assert.deepEqual(await auditDocs(root), {
      failures: [unknownId("docs/surfaces/cursor.md", "gpt-9-nova")],
    });
  });
});

/** Writes a shell-style default naming an unknown identifier into the fixed site `rel` and expects it reported there. */
async function assertUnknownInFixedFile(rel: string): Promise<void> {
  await withDocsRepo(async (root) => {
    writeFile(root, rel, 'MODEL="${MODEL:-gpt-9-nova}"\n');
    assert.deepEqual(await auditDocs(root), {
      failures: [unknownId(rel, "gpt-9-nova")],
    });
  });
}

test("an unknown identifier in .github/tools/src/peer/models.ts", async () => {
  await assertUnknownInFixedFile(".github/tools/src/peer/models.ts");
});

test("an unknown identifier in evals/README.md", async () => {
  await assertUnknownInFixedFile("evals/README.md");
});

test("an unknown identifier in evals/skills.yaml", async () => {
  await assertUnknownInFixedFile("evals/skills.yaml");
});

test("an unknown identifier in .github/workflows/release.md", async () => {
  await assertUnknownInFixedFile(".github/workflows/release.md");
});

test("an unknown identifier in .github/workflows/backlog-grooming.md", async () => {
  await assertUnknownInFixedFile(".github/workflows/backlog-grooming.md");
});

test("Go literals: raw, escaped, and hidden behind a rune quote", async () => {
  await withDocsRepo(async (root) => {
    writeFile(root, "hooks/go/raw.go", "package x\n\nconst a = `gpt-9-nova`\n");
    writeFile(
      root,
      "hooks/go/esc.go",
      'package x\n\nconst a = "\\x67pt-9-nova\\n"\n',
    );
    // The rune holding a double quote must not open a string, or the comments after it would read as literals.
    writeFile(
      root,
      "hooks/go/rune.go",
      'package x\n\nconst q = \'"\'\n\nconst a = "claude-opus-9-1"\n// "gpt-8-nova"\n/* "gpt-7-nova" */\n',
    );
    writeFile(
      root,
      "hooks/go/testdata/skip.go",
      'package x\n\nconst a = "gpt-6-nova"\n',
    );
    writeFile(
      root,
      "hooks/go/skip_test.go",
      'package x\n\nconst a = "gpt-5-nova"\n',
    );
    assert.deepEqual(await auditDocs(root), {
      failures: [
        unknownId("hooks/go/esc.go", "gpt-9-nova"),
        unknownId("hooks/go/raw.go", "gpt-9-nova"),
        unknownId("hooks/go/rune.go", "claude-opus-9-1"),
      ],
    });
  });
});

test("a Go file with a byte order mark and CRLF line endings", async () => {
  await withDocsRepo(async (root) => {
    writeFile(
      root,
      "hooks/go/bom.go",
      '\u{feff}package x\r\n\r\nconst a = "gpt-9-nova"\r\n',
    );
    assert.deepEqual(await auditDocs(root), {
      failures: [unknownId("hooks/go/bom.go", "gpt-9-nova")],
    });
  });
});

test("a pin with no scanned copy left", async () => {
  await withDocsRepo(async (root) => {
    writeFile(
      root,
      "docs/surfaces/codex.md",
      "Runs on `gpt-6-luna`.\n\nVerified on codex-cli 0.147.0 (2026-08-01).\n",
    );
    writeFile(
      root,
      "hooks/go/pin.go",
      'package tools\n\nconst pin = "gpt-6-luna"\n',
    );
    assert.deepEqual(await auditDocs(root), {
      failures: [
        {
          rule: "model-ids-pin-used",
          path: PIN_PATH,
          message:
            'pins model identifier "gpt-5.6-luna", which no file model-ids-known reads names; delete the pin row, or restore the copy it records',
        },
      ],
    });
  });
});

test("an off-ladder identifier with no pin row", async () => {
  await withDocsRepo(async (root) => {
    writeFile(root, PIN_PATH, PINS.replace(PIN_ROW, ""));
    assert.deepEqual(await auditDocs(root), {
      failures: [
        unknownId("docs/surfaces/codex.md", "gpt-5.6-luna"),
        unknownId("hooks/go/pin.go", "gpt-5.6-luna"),
      ],
    });
  });
});

test("a missing fixed file", async () => {
  await withDocsRepo(async (root) => {
    rmSync(path.join(root, "evals", "README.md"));
    assert.deepEqual(await auditDocs(root), {
      failures: [siteGone("evals/README.md")],
    });
  });
});

test("a Go root with only tests", async () => {
  await withDocsRepo(async (root) => {
    rmSync(path.join(root, "hooks", "go"), { recursive: true });
    writeFile(root, "hooks/go/only_test.go", "package only\n");
    assert.deepEqual(await auditDocs(root), {
      failures: [siteGone("hooks/go", "holds no non-test Go source")],
    });
  });
});

test("a missing Go root", async () => {
  await withDocsRepo(async (root) => {
    rmSync(path.join(root, "hooks", "go"), { recursive: true });
    assert.deepEqual(await auditDocs(root), {
      failures: [siteGone("hooks/go")],
    });
  });
});
