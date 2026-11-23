import assert from "node:assert/strict";
import { mkdtempSync, rmSync, symlinkSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { test } from "node:test";
import { RepoFileError } from "../../repo-file-error.ts";
import { writeFile } from "../../testing/files.ts";
import { initCheckout } from "../../testing/git-checkout.ts";
import {
  CONFLICT_BLOCK,
  INVISIBLE_EDGES,
  PATH_SKIPPED,
  SCRATCH,
  writeBytes,
} from "../../testing/repo-seeds.ts";
import { auditRepo, withRepoRepo } from "../../testing/repo-repo.ts";
import {
  conflictMarkers,
  hygieneWalk,
  invisibleUnicode,
  readText,
} from "./hygiene.ts";
import { type Tracked } from "./walk.ts";
import { canSymlink, removeTree, SYMLINK_SKIP } from "../../testing/host.ts";

const NONE: Tracked = { files: new Set(), dirs: new Set() };
const cp = (...points: number[]) => String.fromCodePoint(...points);

function withDir(body: (dir: string) => void): void {
  const dir = mkdtempSync(path.join(tmpdir(), "agentics-repohygiene-"));
  try {
    body(dir);
  } finally {
    removeTree(dir);
  }
}

test("readText reads the whole file, keeps a byte order mark, and marks a NUL in the first 8000 bytes binary", () => {
  withDir((dir) => {
    const file = (name: string, bytes: Buffer) => {
      writeFileSync(path.join(dir, name), bytes);
      return path.join(dir, name);
    };
    assert.equal(readText(file("empty", Buffer.alloc(0))), "");
    assert.equal(
      readText(file("bom", Buffer.from(`${cp(0xfeff)}x`))),
      `${cp(0xfeff)}x`,
    );
    assert.equal(
      readText(file("big", Buffer.alloc(5 << 20, 0x78)))?.length,
      5 << 20,
    );
    const edge = (offset: number) => {
      const bytes = Buffer.alloc(8001, 0x78);
      bytes[offset] = 0;
      return readText(file(`nul-${offset}`, bytes));
    };
    assert.equal(edge(7999), undefined);
    assert.equal(edge(8000)?.length, 8001);
  });
});

test("conflictMarkers fires on each opening and closing marker line and on nothing else", () => {
  const text = [
    "- keep",
    "<<<<<<< HEAD",
    "mine",
    "=======",
    "theirs",
    ">>>>>>> other\r",
    "    <<<<<<< indented",
    "inline <<<<<<< mid",
    "<<<<<<",
    "",
  ].join("\n");
  assert.deepEqual(conflictMarkers("AGENTS.md", text), [
    {
      rule: "conflict-marker",
      path: "AGENTS.md",
      message:
        'line 2 begins with "<<<<<<<" — an unresolved git conflict block. Resolve the merge and delete every marker line; one side of the merge is missing until you do.',
    },
    {
      rule: "conflict-marker",
      path: "AGENTS.md",
      message:
        'line 6 begins with ">>>>>>>" — an unresolved git conflict block. Resolve the merge and delete every marker line; one side of the merge is missing until you do.',
    },
  ]);
});

const message = (line: number, hex: string, klass: string, more = "") =>
  `line ${line} holds U+${hex} (${klass})${more}, invisible in review but read by a model; delete it, or write it as a \\u or \\U escape in code.`;

test("invisibleUnicode flags the first and last code point of every range with its class", () => {
  const ranges: Array<[number, number, string]> = [
    [0x061c, 0x061c, "bidi mark"],
    [0x115f, 0x1160, "Hangul filler"],
    [0x180e, 0x180e, "Mongolian vowel separator"],
    [0x200b, 0x200d, "zero-width character"],
    [0x200e, 0x200f, "bidi mark"],
    [0x202a, 0x202e, "bidi control"],
    [0x2060, 0x2064, "invisible format character"],
    [0x2066, 0x2069, "bidi isolate"],
    [0x3164, 0x3164, "Hangul filler"],
    // U+FE0F is the production range's last member; it is exempt after a symbol, so its own case is the next test.
    [0xfe00, 0xfe0e, "variation selector"],
    [0xfeff, 0xfeff, "zero-width no-break space"],
    [0xe0000, 0xe007f, "Unicode tag character"],
    [0xe0100, 0xe01ef, "variation selector"],
  ];
  for (const [lo, hi, klass] of ranges)
    for (const point of new Set([lo, hi])) {
      const hex = point.toString(16).toUpperCase().padStart(4, "0");
      assert.deepEqual(
        invisibleUnicode("f.md", `a${cp(point)}b\n`).map((f) => f.message),
        [message(1, hex, klass)],
        hex,
      );
    }
  // Just outside each range is clean.
  for (const point of [
    0x061b, 0x061d, 0x115e, 0x1161, 0x180d, 0x180f, 0x200a, 0x2010, 0x2029,
    0x202f, 0x205f, 0x2065, 0x206a, 0x3163, 0x3165, 0xe0080, 0xe00ff, 0xe01f0,
  ])
    assert.deepEqual(
      invisibleUnicode("f.md", cp(point)),
      [],
      point.toString(16),
    );
});

test("invisibleUnicode reports a line once, naming the first code point and counting the rest, with the right line number", () => {
  const text = `clean\n${cp(0x200b, 0x200c, 0x200d)}x${cp(0xe0041)}\nclean\n${cp(0x202e)}\n${cp(0x200b)}y${cp(0x200c)}\n`;
  assert.deepEqual(
    invisibleUnicode("f.md", text).map((f) => f.message),
    [
      message(2, "200B", "zero-width character", " and 3 more"),
      message(4, "202E", "bidi control"),
      message(5, "200B", "zero-width character", " and 1 more"),
    ],
  );
});

test("U+FE0F is exempt only directly after a symbol", () => {
  const fe0f = cp(0xfe0f);
  const flagged = (text: string) => invisibleUnicode("f.md", text).length;
  assert.equal(flagged(`${cp(0x27a1)}${fe0f}`), 0);
  assert.equal(flagged(`${cp(0x26a0)}${fe0f}`), 0);
  assert.equal(flagged(`${cp(0x1f642)}${fe0f}`), 0);
  assert.equal(flagged(`${cp(0xfffd)}${fe0f}`), 0);
  // The second selector of a run follows a selector, not a symbol.
  assert.deepEqual(
    invisibleUnicode("f.md", `${cp(0x27a1)}${fe0f}${fe0f}\n`).map(
      (f) => f.message,
    ),
    [message(1, "FE0F", "variation selector")],
  );
  assert.equal(flagged(`a${fe0f}`), 1);
  assert.equal(flagged(fe0f), 1);
  assert.equal(flagged(`${cp(0x27a1)}\n${fe0f}`), 1);
  assert.equal(flagged(`${cp(0x203c)}${fe0f}`), 1);
});

test("invalid UTF-8 decodes to a symbol, so a selector after it is exempt", () => {
  withDir((dir) => {
    writeFileSync(
      path.join(dir, "bad"),
      Buffer.concat([Buffer.from([0xff]), Buffer.from(cp(0xfe0f))]),
    );
    assert.deepEqual(
      invisibleUnicode("bad", readText(path.join(dir, "bad")) ?? ""),
      [],
    );
  });
});

test("a marker on the first line after a byte order mark does not start the line", async () => {
  assert.deepEqual(conflictMarkers("f.md", cp(0xfeff) + "<<<<<<< HEAD\n"), []);
  await withRepoRepo(async (root) => {
    writeFile(root, "AGENTS.md", `${cp(0xfeff)}<<<<<<< HEAD\nmine\n`);
    assert.deepEqual(await auditRepo(root), {
      failures: [
        {
          rule: "invisible-unicode",
          path: "AGENTS.md",
          message: message(1, "FEFF", "zero-width no-break space"),
        },
      ],
    });
  });
});

test("hygieneWalk names the file whose read fails", () => {
  withDir((dir) => {
    writeFile(dir, "a.md", "ok\n");
    assert.throws(
      () =>
        hygieneWalk(dir, NONE, () => {
          throw new Error("boom");
        }),
      (error) =>
        error instanceof RepoFileError &&
        error.kind === "unreadable" &&
        error.message === "a.md: unreadable",
    );
    assert.throws(
      () =>
        hygieneWalk(dir, NONE, () => {
          throw Object.assign(new Error("EACCES: open"), { code: "EACCES" });
        }),
      { message: "a.md: EACCES" },
    );
  });
});

test("hygieneWalk reports a file's conflict markers ahead of its invisible characters", () => {
  withDir((dir) => {
    writeFile(dir, "a.md", `<<<<<<< HEAD\n${cp(0x200b)}\n`);
    assert.deepEqual(
      hygieneWalk(dir, NONE).failures.map((f) => f.rule),
      ["conflict-marker", "invisible-unicode"],
    );
  });
});

test("hygieneWalk counts scanned files and skips binary ones", () => {
  withDir((dir) => {
    writeFile(dir, "a.md", `ok\n${cp(0x200b)}\n`);
    writeFileSync(path.join(dir, "bin"), Buffer.from([0x3c, 0, 0x3c]));
    const found = hygieneWalk(dir, NONE);
    assert.equal(found.scanned, 1);
    assert.deepEqual(
      found.failures.map((f) => f.rule),
      ["invisible-unicode"],
    );
    rmSync(path.join(dir, "a.md"));
    writeFile(dir, "dirlike/x", "x");
    assert.equal(hygieneWalk(dir, NONE).scanned, 1);
  });
});

const marker = (rel: string, line: number, sign: string) => ({
  rule: "conflict-marker",
  path: rel,
  message: `line ${line} begins with "${sign}" — an unresolved git conflict block. Resolve the merge and delete every marker line; one side of the merge is missing until you do.`,
});
/** The two findings `CONFLICT_BLOCK` raises in `rel`: its opening line 2 and its closing line 6. */
const blockAt = (rel: string) => [
  marker(rel, 2, "<<<<<<<"),
  marker(rel, 6, ">>>>>>>"),
];
const invisible = (
  rel: string,
  line: number,
  hex: string,
  klass: string,
  more = "",
) => ({
  rule: "invisible-unicode",
  path: rel,
  message: message(line, hex, klass, more),
});

test("conflict-marker", async () => {
  await withRepoRepo(async (root) => {
    writeFile(root, "AGENTS.md", CONFLICT_BLOCK);
    assert.deepEqual(await auditRepo(root), {
      failures: blockAt("AGENTS.md"),
    });
  });
});

test("invisible-unicode", async () => {
  await withRepoRepo(async (root) => {
    writeFile(root, "docs/x.md", `a${cp(0x200b)}b\n`);
    assert.deepEqual(await auditRepo(root), {
      failures: [invisible("docs/x.md", 1, "200B", "zero-width character")],
    });
  });
});

test("a tracked conflict block under a skipped directory", async () => {
  await withRepoRepo(async (root) => {
    writeFile(root, "node_modules/pkg/forced.md", CONFLICT_BLOCK);
    writeFile(root, "node_modules/pkg/loose.md", CONFLICT_BLOCK);
    initCheckout(root, ["node_modules/pkg/forced.md"]);
    assert.deepEqual(await auditRepo(root), {
      failures: blockAt("node_modules/pkg/forced.md"),
    });
  });
});

test("a conflict block with CRLF line endings", async () => {
  await withRepoRepo(async (root) => {
    writeFile(root, "AGENTS.md", CONFLICT_BLOCK.replaceAll("\n", "\r\n"));
    assert.deepEqual(await auditRepo(root), {
      failures: blockAt("AGENTS.md"),
    });
  });
});

test("a NUL at byte 8000 does not", async () => {
  await withRepoRepo(async (root) => {
    const data = Buffer.alloc(8001, 0x78);
    data[8000] = 0;
    writeBytes(
      root,
      "w.txt",
      Buffer.concat([data, Buffer.from("\n<<<<<<< HEAD\n")]),
    );
    assert.deepEqual(await auditRepo(root), {
      failures: [marker("w.txt", 2, "<<<<<<<")],
    });
  });
});

test("the first and last code point of every invisible range", async () => {
  await withRepoRepo(async (root) => {
    for (const point of INVISIBLE_EDGES)
      writeFile(root, `inv/${point.toString(16)}.md`, `a${cp(point)}b\n`);
    // In the walk's order of file names. U+FE0F follows a letter here, so it is not exempt.
    const expected: Array<[string, string, string]> = [
      ["115f", "115F", "Hangul filler"],
      ["1160", "1160", "Hangul filler"],
      ["180e", "180E", "Mongolian vowel separator"],
      ["200b", "200B", "zero-width character"],
      ["200d", "200D", "zero-width character"],
      ["200e", "200E", "bidi mark"],
      ["200f", "200F", "bidi mark"],
      ["202a", "202A", "bidi control"],
      ["202e", "202E", "bidi control"],
      ["2060", "2060", "invisible format character"],
      ["2064", "2064", "invisible format character"],
      ["2066", "2066", "bidi isolate"],
      ["2069", "2069", "bidi isolate"],
      ["3164", "3164", "Hangul filler"],
      ["61c", "061C", "bidi mark"],
      ["e0000", "E0000", "Unicode tag character"],
      ["e007f", "E007F", "Unicode tag character"],
      ["e0100", "E0100", "variation selector"],
      ["e01ef", "E01EF", "variation selector"],
      ["fe00", "FE00", "variation selector"],
      ["fe0f", "FE0F", "variation selector"],
      ["feff", "FEFF", "zero-width no-break space"],
    ];
    assert.deepEqual(await auditRepo(root), {
      failures: expected.map(([name, hex, klass]) =>
        invisible(`inv/${name}.md`, 1, hex, klass),
      ),
    });
  });
});

test("a run of selectors after a symbol", async () => {
  await withRepoRepo(async (root) => {
    writeFile(root, "docs/x.md", `${cp(0x27a1, 0xfe0f, 0xfe0f)}\n`);
    assert.deepEqual(await auditRepo(root), {
      failures: [invisible("docs/x.md", 1, "FE0F", "variation selector")],
    });
  });
});

test("a selector after an astral symbol, after U+FFFD, and after a letter", async () => {
  await withRepoRepo(async (root) => {
    writeFile(
      root,
      "docs/x.md",
      `${cp(0x1f642, 0xfe0f)}\n${cp(0xfffd, 0xfe0f)}\na${cp(0xfe0f)}\n`,
    );
    assert.deepEqual(await auditRepo(root), {
      failures: [invisible("docs/x.md", 3, "FE0F", "variation selector")],
    });
  });
});

test("every skip-list entry hides a marker at its own path, and a nested copy is hidden only for an entry without a slash", async () => {
  await withRepoRepo(async (root) => {
    for (const entry of PATH_SKIPPED) {
      writeFile(root, `${entry}/x.md`, CONFLICT_BLOCK);
      writeFile(root, `a/${entry}/near.md`, CONFLICT_BLOCK);
    }
    assert.deepEqual(await auditRepo(root), {
      failures: [
        "a/.claude/worktrees/near.md",
        "a/docs/handoffs/near.md",
        "a/docs/probes/near.md",
        "a/docs/reviews/peer/near.md",
        "a/evals/runs/near.md",
      ].flatMap(blockAt),
    });
  });
});

test("near misses of the entries with a slash: a directory sharing a prefix and a nested copy", async () => {
  await withRepoRepo(async (root) => {
    writeFile(root, "docs/handoffs-old/x.md", CONFLICT_BLOCK);
    writeFile(root, "a/evals/runs/y.md", CONFLICT_BLOCK);
    assert.deepEqual(await auditRepo(root), {
      failures: ["a/evals/runs/y.md", "docs/handoffs-old/x.md"].flatMap(
        blockAt,
      ),
    });
  });
});

test("a work folder's scratch file is hidden and a temp directory elsewhere is not", async () => {
  await withRepoRepo(async (root) => {
    writeFile(root, `${SCRATCH}/a.txt`, CONFLICT_BLOCK);
    writeFile(root, "docs/temp/b.txt", CONFLICT_BLOCK);
    assert.deepEqual(await auditRepo(root), {
      failures: blockAt("docs/temp/b.txt"),
    });
  });
});

test("a tracked scratch file under a work folder's temp directory is scanned", async () => {
  await withRepoRepo(async (root) => {
    writeFile(root, `${SCRATCH}/forced.txt`, CONFLICT_BLOCK);
    writeFile(root, `${SCRATCH}/loose.txt`, CONFLICT_BLOCK);
    initCheckout(root, [`${SCRATCH}/forced.txt`]);
    assert.deepEqual(await auditRepo(root), {
      failures: blockAt(`${SCRATCH}/forced.txt`),
    });
  });
});

test("a tracked file under every skipped directory is scanned and an untracked one is not", async () => {
  await withRepoRepo(async (root) => {
    for (const entry of PATH_SKIPPED) {
      writeFile(root, `${entry}/forced.md`, CONFLICT_BLOCK);
      writeFile(root, `${entry}/loose.md`, CONFLICT_BLOCK);
    }
    initCheckout(
      root,
      PATH_SKIPPED.map((entry) => `${entry}/forced.md`),
    );
    assert.deepEqual(await auditRepo(root), {
      failures: [
        ".agentics/forced.md",
        ".audit/forced.md",
        ".claude/worktrees/forced.md",
        ".worktrees/forced.md",
        "docs/handoffs/forced.md",
        "docs/probes/forced.md",
        "docs/reviews/peer/forced.md",
        "evals/runs/forced.md",
        "node_modules/forced.md",
      ].flatMap(blockAt),
    });
  });
});

test("a partial directory enters only directories that hold a tracked file", async () => {
  await withRepoRepo(async (root) => {
    writeFile(root, "node_modules/pkg/forced.md", CONFLICT_BLOCK);
    writeFile(root, "node_modules/pkg/sub/x.md", CONFLICT_BLOCK);
    initCheckout(root, ["node_modules/pkg/forced.md"]);
    assert.deepEqual(await auditRepo(root), {
      failures: blockAt("node_modules/pkg/forced.md"),
    });
  });
});

test(
  "a symlink to a file holding a marker is not followed",
  { skip: canSymlink() ? false : SYMLINK_SKIP },
  async () => {
    await withRepoRepo(async (root) => {
      writeFile(root, "real/AGENTS.md", CONFLICT_BLOCK);
      symlinkSync(
        path.join(root, "real", "AGENTS.md"),
        path.join(root, "link.md"),
      );
      assert.deepEqual(await auditRepo(root), {
        failures: blockAt("real/AGENTS.md"),
      });
    });
  },
);

test("a byte order mark before the package clause of the command", async () => {
  await withRepoRepo(async (root) => {
    writeFile(
      root,
      "hooks/go/cmd/hooktool/main.go",
      `${cp(0xfeff)}package main\n\nimport "github.com/muehz/agentics/hooks/go/internal/cli"\n\nfunc main() { cli.Run() }\n`,
    );
    assert.deepEqual(await auditRepo(root), {
      failures: [
        invisible(
          "hooks/go/cmd/hooktool/main.go",
          1,
          "FEFF",
          "zero-width no-break space",
        ),
      ],
    });
  });
});

test("a conflict block under a testdata directory is still read", async () => {
  await withRepoRepo(async (root) => {
    writeFile(root, "hooks/go/internal/x/testdata/m.md", CONFLICT_BLOCK);
    assert.deepEqual(await auditRepo(root), {
      failures: blockAt("hooks/go/internal/x/testdata/m.md"),
    });
  });
});

test("an untracked conflict block outside the skipped directories is still scanned in a checkout", async () => {
  await withRepoRepo(async (root) => {
    writeFile(root, "docs/x.md", CONFLICT_BLOCK);
    initCheckout(root, []);
    assert.deepEqual(await auditRepo(root), {
      failures: blockAt("docs/x.md"),
    });
  });
});
