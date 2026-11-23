import assert from "node:assert/strict";
import {
  mkdtempSync,
  readFileSync,
  renameSync,
  rmSync,
  symlinkSync,
} from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { test, type TestContext } from "node:test";
import type { AuditResult, Failure, RunOptions } from "../../check.ts";
import {
  auditEveryFamily,
  auditSlice,
  sliceNamed,
  all,
  breaker,
  crlf,
  PROTOTYPE_REFS,
  ref,
  symlinked,
  TRIAGE_REFS,
  xBody,
  staleBaseline,
} from "../../testing/skills-seeds.ts";
import {
  evalsCheck,
  link,
  mkdir,
  mutateFile,
  remove,
  withSkillsRepo,
  writeFile,
} from "../../testing/skills-repo.ts";
import { RepoFileError } from "../../repo-file-error.ts";
import { seams, withSeams, type Seams } from "./files.ts";
import { run } from "./index.ts";
import {
  APACHE_ATTRIBUTED_FILES,
  extractRelativeLinks,
  NOTICES_FILE,
  REFERENCE_LINK_FAMILY,
  REFERENCE_NAMING_FAMILY,
  TEMPLATE_LINK_ALLOWLIST,
} from "./references.ts";
import {
  canSymlink,
  linkDirectory,
  removeTree,
  SYMLINK_SKIP,
} from "../../testing/host.ts";

const options: RunOptions = {
  short: false,
  fast: false,
  diagnostics: () => {},
  env: process.env,
};

const NOTES = "skills/prototype/references/notes.md";
const DOCS_SKILL = "skills/docs/SKILL.md";
const HEADING = "## Per-file provenance manifest";

const VT = String.fromCharCode(0x0b);
const NBSP = String.fromCodePoint(0xa0);
const EMOJI = String.fromCodePoint(0x1f600);
const LONG_S = String.fromCodePoint(0x17f);
const SUPPLEMENTARY = String.fromCodePoint(0x10000);
const LAST_BMP = String.fromCodePoint(0xffff);

function audit(root: string): Promise<AuditResult> {
  return run(root, options, { families: [REFERENCE_LINK_FAMILY] });
}

/** The failures of a run that must not have raised an audit error. */
async function failuresOf(root: string): Promise<Failure[]> {
  const result = await audit(root);
  assert.equal(result.error, undefined);
  return result.failures;
}

const only = (failures: Failure[], rule: string): Failure[] =>
  failures.filter((one) => one.rule === rule);

const rules = (failures: Failure[]): string[] =>
  failures.map((one) => one.rule);

const dangling = (failures: Failure[]): string[] =>
  only(failures, "reference-link").map(
    (one) => `${one.path} ${one.message.replace("dangling link -> ", "")}`,
  );

const MISSING_LINK = "[gone](./gone.md)";

function skipForLink(t: TestContext, what: string): void {
  t.skip(`the host refuses to create ${what}`);
}

test("the family is named reference-link and lists its four rules in emission order", () => {
  assert.equal(REFERENCE_LINK_FAMILY.name, "reference-link");
  assert.deepEqual(REFERENCE_LINK_FAMILY.rules, [
    "reference-link",
    "attribution-centralized",
    "reference-attribution",
    "notices-manifest",
  ]);
});

test("the template allowlist, the notices file and the Apache files hold their pinned values", () => {
  assert.deepEqual(TEMPLATE_LINK_ALLOWLIST, [
    "skills/scaffold/references/governance/constitution.md",
  ]);
  assert.equal(NOTICES_FILE, "THIRD_PARTY_NOTICES.md");
  assert.deepEqual(
    APACHE_ATTRIBUTED_FILES.map((entry) => entry.rel),
    ["skills/docs/SKILL.md"],
  );
});

test("a clean clone of the green fixture gives no failure and no error", async () => {
  await withSkillsRepo(async (root) => {
    assert.deepEqual(await audit(root), { failures: [] });
  });
});

test("a minimal tree of the Apache files and a notices file with the heading and each path is clean", async () => {
  const root = mkdtempSync(path.join(tmpdir(), "agentics-references-min-"));
  try {
    let notices = HEADING + "\n";
    for (const entry of APACHE_ATTRIBUTED_FILES) {
      writeFile(root, entry.rel, entry.notice + "\n");
      notices += entry.rel.replace(/^skills\//, "") + "\n";
    }
    writeFile(root, NOTICES_FILE, notices);
    assert.deepEqual(await audit(root), { failures: [] });
  } finally {
    removeTree(root);
  }
});

// --- file selection ---------------------------------------------------------

test("a SKILL.md at any depth and any .md with /references/ in its path are scanned; other markdown files are not", async () => {
  await withSkillsRepo(async (root) => {
    const scanned = [
      "skills/a/SKILL.md",
      "skills/a/deep/er/SKILL.md",
      "skills/a/references/x/SKILL.md",
      "skills/a/references/x/notes.md",
      "skills/references/top.md",
    ];
    const ignored = [
      "skills/README.md",
      "skills/a/other.md",
      "skills/a/references.md",
      "skills/a/references/x.MD",
      "skills/a/references/x.txt",
    ];
    for (const rel of [...scanned, ...ignored])
      writeFile(root, rel, MISSING_LINK + "\n");
    mkdir(root, "skills/a/references/dir.md");
    writeFile(root, "skills/a/references/dir.md/inner.txt", MISSING_LINK);
    const failures = await failuresOf(root);
    assert.deepEqual(
      only(failures, "reference-link").map((one) => one.path),
      [...scanned].sort(),
    );
    assert.equal(failures.length, scanned.length);
  });
});

test("a directory named like a markdown file is not read as a file", async () => {
  await withSkillsRepo(async (root) => {
    mkdir(root, "skills/a/references/x.md");
    assert.deepEqual(await audit(root), { failures: [] });
  });
});

test("a symlink leaf under references is not scanned and a link to an existing file counts as present", async (t) => {
  await withSkillsRepo(async (root) => {
    const outside = mkdtempSync(
      path.join(tmpdir(), "agentics-references-out-"),
    );
    try {
      writeFile(outside, "bad.md", MISSING_LINK + "\n");
      writeFile(outside, "target.md", "# Target\n");
      if (
        !link(
          root,
          "skills/a/references/bad.md",
          path.join(outside, "bad.md"),
          "file",
        )
      )
        return skipForLink(t, "a file symlink");
      assert.ok(
        link(
          root,
          "skills/a/linked.md",
          path.join(outside, "target.md"),
          "file",
        ),
      );
      writeFile(root, "skills/a/SKILL.md", "[link](linked.md)\n");
      assert.deepEqual(await audit(root), { failures: [] });
      remove(root, "skills/a/linked.md");
      assert.deepEqual(dangling(await failuresOf(root)), [
        "skills/a/SKILL.md linked.md",
      ]);
      writeFile(root, "skills/a/linked.md", "ordinary\n");
      assert.deepEqual(await audit(root), { failures: [] });
    } finally {
      removeTree(outside);
    }
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

test("a references file that becomes a link after the walk is refused by the read", async (t) => {
  await withSkillsRepo(async (root) => {
    const outside = mkdtempSync(
      path.join(tmpdir(), "agentics-references-out-"),
    );
    try {
      const rel = "skills/a/references/notes.md";
      writeFile(root, rel, "# Notes\n");
      writeFile(outside, "notes.md", MISSING_LINK + "\n");
      const file = path.join(root, ...rel.split("/"));
      // The walk lists the file without inspecting it; the link appears before the read inspects it.
      const lstat = linkOnInspection(
        file,
        path.join(outside, "notes.md"),
        "file",
        () => true,
      );
      if (lstat === undefined) return skipForLink(t, "a file symlink");
      assert.deepEqual(await withSeams({ lstat }, () => audit(root)), {
        failures: [],
        error: `skills: reading ${rel}: ${rel}: passes through a link`,
      });
    } finally {
      removeTree(outside);
    }
  });
});

test("a missing skills directory leaves one missing-Apache failure and still runs the notices rules", async () => {
  await withSkillsRepo(async (root) => {
    remove(root, "skills");
    const failures = await failuresOf(root);
    assert.deepEqual(
      failures.map((one) => [one.rule, one.path, one.message]),
      APACHE_ATTRIBUTED_FILES.map((entry) => [
        "reference-attribution",
        entry.rel,
        "attributed third-party file missing",
      ]),
    );
    remove(root, NOTICES_FILE);
    assert.deepEqual(
      (await failuresOf(root))
        .slice(1)
        .map((one) => [one.rule, one.path, one.message]),
      [["notices-manifest", NOTICES_FILE, "third-party notices file missing"]],
    );
  });
});

test("skills as a regular file leaves one missing-Apache failure on Windows and an audit error on POSIX", async () => {
  await withSkillsRepo(async (root) => {
    remove(root, "skills");
    writeFile(root, "skills", "not a directory\n");
    if (process.platform !== "win32") {
      // A path below a regular file is ENOTDIR on POSIX, so the first Apache file read is an audit error.
      const result = await audit(root);
      assert.equal(
        result.error,
        "skills: reading skills/docs/SKILL.md: skills/docs: is not a directory",
      );
      return;
    }
    const failures = await failuresOf(root);
    assert.equal(failures.length, 1);
    for (const one of failures) assert.equal(one.rule, "reference-attribution");
  });
});

test("skills linked elsewhere is an audit error naming the first Apache file and the link", async (t) => {
  await withSkillsRepo(async (root) => {
    const outside = mkdtempSync(
      path.join(tmpdir(), "agentics-references-out-"),
    );
    try {
      writeFile(outside, "a/SKILL.md", "[sentinel](missing.md)");
      remove(root, "skills");
      if (linkDirectory(outside, path.join(root, "skills")) === undefined)
        return skipForLink(t, "a directory link");
      assert.deepEqual(await audit(root), {
        failures: [],
        error: `skills: reading ${DOCS_SKILL}: skills: passes through a link`,
      });
    } finally {
      removeTree(outside);
    }
  });
});

test("a failed directory read while walking skills is the audit error skills: walking <directory>", async () => {
  await withSkillsRepo(async (root) => {
    const real = seams.readDir;
    const result = await withSeams(
      {
        readDir: (dir) => {
          if (path.basename(dir) === "prototype")
            throw Object.assign(new Error("denied"), { code: "EACCES" });
          return real(dir);
        },
      },
      () => audit(root),
    );
    assert.deepEqual(result, {
      failures: [],
      error: "skills: walking skills: skills/prototype: EACCES",
    });
  });
});

test("a failed read of a scanned file is the audit error skills: reading <rel>", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(root, NOTES, "# Notes\n");
    const real = seams.readFile;
    const result = await withSeams(
      {
        readFile: (target) => {
          if (target.endsWith("notes.md"))
            throw Object.assign(new Error("boom"), { code: "EIO" });
          return real(target);
        },
      },
      () => audit(root),
    );
    assert.deepEqual(result, {
      failures: [],
      error: `skills: reading ${NOTES}: ${NOTES}: EIO`,
    });
  });
});

test("the allowlisted template may hold a dangling link and a notices pointer there still fails", async () => {
  await withSkillsRepo(async (root) => {
    const rel = TEMPLATE_LINK_ALLOWLIST[0]!;
    writeFile(root, rel, MISSING_LINK + "\n");
    assert.deepEqual(await audit(root), { failures: [] });
    writeFile(root, rel, MISSING_LINK + "\nSee THIRD_PARTY_NOTICES.md.\n");
    const failures = await failuresOf(root);
    assert.deepEqual(
      failures.map((one) => [one.rule, one.path]),
      [["attribution-centralized", rel]],
    );
  });
});

// --- link resolution ----------------------------------------------------------

test("a dangling relative link is one failure naming the missing target", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(root, NOTES, "See [the missing doc](./missing-doc.md).\n");
    assert.deepEqual(await failuresOf(root), [
      {
        rule: "reference-link",
        path: NOTES,
        message: "dangling link -> ./missing-doc.md",
      },
    ]);
  });
});

test("targets resolve against the file's directory: up one level, a directory and the two spellings of one file", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(root, "skills/prototype/up.md", "# Up\n");
    mkdir(root, "skills/prototype/references/d.md");
    writeFile(root, NOTES, "[u](../up.md) [d](d.md)\n");
    assert.deepEqual(await audit(root), { failures: [] });
    writeFile(root, NOTES, "[a](x.md) [b](./x.md)\n");
    assert.deepEqual(dangling(await failuresOf(root)), [
      `${NOTES} ./x.md`,
      `${NOTES} x.md`,
    ]);
    writeFile(root, "skills/prototype/references/x.md", "# X\n");
    assert.deepEqual(await audit(root), { failures: [] });
  });
});

test("a link to a symlink that points at a missing file is dangling", async (t) => {
  await withSkillsRepo(async (root) => {
    const outside = mkdtempSync(
      path.join(tmpdir(), "agentics-references-out-"),
    );
    try {
      if (
        !link(
          root,
          "skills/prototype/references/gone.md",
          path.join(outside, "nothing.md"),
          "file",
        )
      )
        return skipForLink(t, "a file symlink");
      writeFile(root, NOTES, "[g](gone.md)\n");
      assert.deepEqual(dangling(await failuresOf(root)), [`${NOTES} gone.md`]);
    } finally {
      removeTree(outside);
    }
  });
});

test("within a file the failures follow the byte order of the unique targets and files report in byte order of their paths", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(
      root,
      "skills/prototype/references/b.md",
      "[x](none-b.md) [y](None-a.md) [z](none-b.md)\n",
    );
    writeFile(
      root,
      "skills/prototype/references/a.md",
      "[x](none-z.md) [y](none-a.md)\n",
    );
    writeFile(root, "skills/prototype/references/Z.md", "[x](only.md)\n");
    assert.deepEqual(dangling(await failuresOf(root)), [
      "skills/prototype/references/Z.md only.md",
      "skills/prototype/references/a.md none-a.md",
      "skills/prototype/references/a.md none-z.md",
      "skills/prototype/references/b.md None-a.md",
      "skills/prototype/references/b.md none-b.md",
    ]);
  });
});

// --- extractRelativeLinks -----------------------------------------------------

const LINK_VECTORS: ReadonlyArray<readonly [string, string, string[]]> = [
  ["title makes the target not end in .md", '[a](a.md "t")', []],
  ["a space in the target", "[c](c d.md)", []],
  ["angle brackets", "[e](<e.md>)", []],
  ["the anchor is removed", "[f](f.md#frag)", ["f.md"]],
  [
    "a .md inside an anchor-like tail is the target",
    "[x](a.md#x.md)",
    ["a.md#x.md"],
  ],
  ["an empty anchor", "[a](a.md#)", ["a.md"]],
  [
    "both spellings of one file are kept",
    "[g](./g.md) [g](g.md)",
    ["./g.md", "g.md"],
  ],
  ["an image link", "![i](img.md)", ["img.md"]],
  ["http", "[h](http://x.md)", []],
  ["https", "[h](https://x.md)", []],
  ["mailto", "[m](mailto:x.md)", []],
  ["an absolute path", "[a](/abs.md)", []],
  ["the scheme match is case-sensitive", "[h](HTTP://x.md)", ["HTTP://x.md"]],
  ["file: is kept", "[f](file:t.md)", ["file:t.md"]],
  ["percent escapes stay undecoded", "[u](%20u.md)", ["%20u.md"]],
  ["the suffix is case-sensitive", "[v](v.MD)", []],
  ["a bare .md target", "[a](.md)", []],
  ["a doubled suffix", "[a](x.md.md)", ["x.md.md"]],
  ["a vertical tab is whitespace", `[v](a${VT}b.md)`, []],
  ["a tab is whitespace", "[s](a\tb.md)", []],
  ["a no-break space is not whitespace", `[n](a${NBSP}b.md)`, [`a${NBSP}b.md`]],
  ["templated class: letters of the word space", "[s](space.md)", ["space.md"]],
  ["templated class: a colon", "[s](a:b.md)", ["a:b.md"]],
  ["templated class: square brackets", "[s](s[p].md)", ["s[p].md"]],
  ["templated class: less-than", "[s](a<b.md)", []],
  ["templated class: greater-than", "[s](a>b.md)", []],
  ["templated class: braces", "[s](a{b}.md)", []],
  ["templated class: dollar", "[s](a$b.md)", []],
  ["templated class: star", "[s](a*b.md)", []],
  ["templated class: pipe", "[s](a|b.md)", []],
  ["templated class: a space", "[s](a b.md)", []],
  ["an inline code span hides its link", "`[x](x.md)`", []],
  [
    "an empty double-backtick span leaves the link after it",
    "``code`` [b](b.md)",
    ["b.md"],
  ],
  ["a link between two spans", "`a` [k](k.md) `b`", ["k.md"]],
  [
    "a backtick opened on one line and closed on the next strips nothing",
    "`open\n[l](l.md) close`",
    ["l.md"],
  ],
  [
    "duplicates collapse and the result is sorted",
    "[a](z.md) [b](a.md) [c](z.md)",
    ["a.md", "z.md"],
  ],
  [
    "the default sort, by UTF-16 code units: U+10000 sorts before U+FFFF",
    `[a](${SUPPLEMENTARY}.md) [b](${LAST_BMP}.md)`,
    [`${SUPPLEMENTARY}.md`, `${LAST_BMP}.md`],
  ],
  [
    "a CR at a line end is not part of a target",
    "[d](d.md)\r\n[e](e.md)\r\n",
    ["d.md", "e.md"],
  ],
  ["a stray bracket after a link", "[a](a.md)](b.md)", ["a.md", "b.md"]],
];

for (const [name, text, expected] of LINK_VECTORS)
  test(`extractRelativeLinks: ${name}`, () => {
    assert.deepEqual(extractRelativeLinks(text), expected);
  });

// --- fences ---------------------------------------------------------------------

const FENCE_CASES: ReadonlyArray<readonly [string, string[], string[]]> = [
  [
    "a plain fence, inline code, a template and an absolute link",
    [
      "```",
      "[inside a fence](./fenced-away.md)",
      "```",
      "",
      "`[inline code](./coded-away.md)` and [templated](<path>/tpl.md) and [abs](/abs.md).",
      "",
      "Intent-level guidance",
    ],
    [],
  ],
  [
    "a four-backtick block with three backticks inside exposes the link after it",
    [
      "````markdown",
      "```bash",
      "echo inner",
      "````",
      "",
      "See [the missing doc](./missing-after-fence.md). Intent-level guidance",
    ],
    ["./missing-after-fence.md"],
  ],
  [
    "a link inside a complete inner block of a four-backtick block stays fenced",
    [
      "````markdown",
      "```bash",
      "[still fenced](./missing-inside.md)",
      "```",
      "````",
      "",
      "Intent-level guidance",
    ],
    [],
  ],
  [
    "a fence after quote markers stays fenced",
    [
      "> ```text",
      "> [inside a quoted fence](./quoted-away.md)",
      "> ```",
      "",
      "Intent-level guidance",
    ],
    [],
  ],
  [
    "a quoted fence without a closer ends with its quote",
    [
      "> ```text",
      "> fenced",
      "See [the missing doc](./missing-after-quote.md). Intent-level guidance",
    ],
    ["./missing-after-quote.md"],
  ],
  [
    "a fence inside a list item stays fenced",
    [
      "1. Step one:",
      "",
      "    ```markdown",
      "    [inside a list fence](./fenced-away.md)",
      "    ```",
      "",
      "Intent-level guidance",
    ],
    [],
  ],
  [
    "outdent: a line at the list's indentation ends its fence",
    [
      "1. Example:",
      "    ```markdown",
      "    [example](./fake.md)",
      "[real](./missing-real.md)",
      "Intent-level guidance",
    ],
    ["./missing-real.md"],
  ],
  [
    "same-line: a fence opened on the list marker's line",
    [
      "- ```markdown",
      "  [example](./fake.md)",
      "  ```",
      "[real](./missing-real.md)",
      "Intent-level guidance",
    ],
    ["./missing-real.md"],
  ],
  [
    "a nested quote fence ends at the lower quote depth",
    [
      "> > ```",
      "> > [example](./example-only.md)",
      "> [real](./missing-after-nested-quote.md)",
      "Intent-level guidance",
    ],
    ["./missing-after-nested-quote.md"],
  ],
];

for (const [name, lines, expected] of FENCE_CASES)
  for (const [ending, label] of [
    ["\n", "LF"],
    ["\r\n", "CRLF"],
  ] as const)
    test(`fences, ${label}: ${name}`, async () => {
      const text = lines.join(ending);
      assert.deepEqual(extractRelativeLinks(text), expected);
      await withSkillsRepo(async (root) => {
        writeFile(root, NOTES, text);
        assert.deepEqual(
          (await failuresOf(root)).map((one) => [one.rule, one.message]),
          expected.map((target) => [
            "reference-link",
            `dangling link -> ${target}`,
          ]),
        );
      });
    });

// --- attribution-centralized ------------------------------------------------------

test("a clean fixture has no attribution-centralized failure", async () => {
  await withSkillsRepo(async (root) => {
    assert.deepEqual(
      only(await failuresOf(root), "attribution-centralized"),
      [],
    );
  });
});

test("a file with an in-file attribution marker names the matched text and the notices file", async () => {
  await withSkillsRepo(async (root) => {
    const rel = "skills/git/references/merge-conflicts.md";
    writeFile(
      root,
      rel,
      "# Merge conflicts\n\nAdapted from mattpocock/skills resolving-merge-conflicts (MIT).\n",
    );
    assert.deepEqual(await failuresOf(root), [
      {
        rule: "attribution-centralized",
        path: rel,
        message:
          'in-file third-party attribution ("Adapted from mattpocock/skills resolving-merge-conflicts (MIT)"); record it in THIRD_PARTY_NOTICES.md instead',
      },
    ]);
  });
});

test("a file that points at the notices file is one pointer failure", async () => {
  await withSkillsRepo(async (root) => {
    const rel = "skills/scaffold/references/feedback-loops/audit.md";
    writeFile(
      root,
      rel,
      "# Audit\n\nSee THIRD_PARTY_NOTICES.md for attribution.\n",
    );
    assert.deepEqual(await failuresOf(root), [
      {
        rule: "attribution-centralized",
        path: rel,
        message:
          'points into the Agentics repository ("THIRD_PARTY_NOTICES"); a skill must stay self-contained',
      },
    ]);
  });
});

test("an Apache file may carry its own notice but a pointer at the notices file is one failure", async () => {
  await withSkillsRepo(async (root) => {
    const pointer = {
      rule: "attribution-centralized",
      path: "skills/docs/SKILL.md",
      message:
        'points into the Agentics repository ("THIRD_PARTY_NOTICES"); a skill must stay self-contained',
    };
    writeFile(
      root,
      "skills/docs/SKILL.md",
      readFileSync(path.join(root, "skills", "docs", "SKILL.md"), "utf8") +
        "\nSee THIRD_PARTY_NOTICES.md.\n",
    );
    assert.deepEqual(await failuresOf(root), [pointer]);
  });
});

test("a pointer written inside the Apache notice itself fails the pointer rule and the pinned notice", async () => {
  await withSkillsRepo(async (root) => {
    mutateFile(
      root,
      "skills/docs/SKILL.md",
      "(Apache-2.0); modified. -->",
      "(Apache-2.0); modified. See THIRD_PARTY_NOTICES.md. -->",
    );
    assert.deepEqual(
      (await failuresOf(root)).map((one) => [one.rule, one.path]),
      [
        ["attribution-centralized", "skills/docs/SKILL.md"],
        ["reference-attribution", "skills/docs/SKILL.md"],
      ],
    );
  });
});

test("an Apache file carrying an attribution marker is exempt from the marker check", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(
      root,
      DOCS_SKILL,
      "<!-- Adapted from anthropics/skills doc-coauthoring (Apache-2.0); modified. -->\nPorted from elsewhere (MIT)\n",
    );
    assert.deepEqual(await audit(root), { failures: [] });
  });
});

test("a file with both a pointer and a marker gives the pointer failure first", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(
      root,
      NOTES,
      "Ported from a (MIT)\nSee THIRD_PARTY_NOTICES.md.\n",
    );
    const failures = await failuresOf(root);
    assert.deepEqual(
      failures.map((one) => [one.rule, one.path, one.message.slice(0, 24)]),
      [
        ["attribution-centralized", NOTES, "points into the Agentics"],
        ["attribution-centralized", NOTES, "in-file third-party attr"],
      ],
    );
  });
});

const MARKER_VECTORS: ReadonlyArray<readonly [string, string, string | null]> =
  [
    [
      "the shortest span ends at the first license name",
      "Ported from a (MIT) b (Apache-2.0)",
      "Ported from a (MIT)",
    ],
    ["lowercase license name", "Ported from a (mit)", "Ported from a (mit)"],
    [
      "lowercase Apache-2.0",
      "Ported from a (apache-2.0)",
      "Ported from a (apache-2.0)",
    ],
    ["a dot is literal in Apache-2.0", "Ported from a (Apache-2x0)", null],
    [
      "the verb is case-insensitive",
      "ADAPTED FROM x (APACHE-2.0)",
      "ADAPTED FROM x (APACHE-2.0)",
    ],
    [
      "no text between from and the license",
      "Ported from (MIT)",
      "Ported from (MIT)",
    ],
    [
      "200 emoji between from and the license match, counted as code points",
      `Ported from${EMOJI.repeat(200)}(MIT)`,
      `Ported from${EMOJI.repeat(200)}(MIT)`,
    ],
    ["201 emoji do not", `Ported from${EMOJI.repeat(201)}(MIT)`, null],
    [
      "200 other characters match",
      `Ported from${"x".repeat(200)}(MIT)`,
      `Ported from${"x".repeat(200)}(MIT)`,
    ],
    ["201 other characters do not", `Ported from${"x".repeat(201)}(MIT)`, null],
    ["a newline between from and the license", "Ported from\n(MIT)", null],
    [
      "a carriage return does not end the span",
      "Ported from a\rb (MIT)",
      "Ported from a\rb (MIT)",
    ],
    ["vendored", "vendored from x (MIT)", "vendored from x (MIT)"],
    ["adopted", "adopted from x (MIT)", "adopted from x (MIT)"],
    ["generalized", "generalized from x (MIT)", "generalized from x (MIT)"],
    ["forked is not a verb", "forked from x (MIT)", null],
    ["no space before from", "adaptedfrom x (MIT)", null],
    ["two spaces before from", "adapted  from x (MIT)", null],
    [
      "only the first line matches when the verbs repeat",
      "Ported from x (MIT)\nAdapted from y (MIT)",
      "Ported from x (MIT)",
    ],
  ];

for (const [name, text, matched] of MARKER_VECTORS)
  test(`attributionMarkerRe: ${name}`, async () => {
    await withSkillsRepo(async (root) => {
      writeFile(root, NOTES, text);
      const failures = await failuresOf(root);
      if (matched === null) {
        assert.deepEqual(failures, []);
        return;
      }
      assert.deepEqual(failures, [
        {
          rule: "attribution-centralized",
          path: NOTES,
          message: `in-file third-party attribution (${JSON.stringify(matched).replaceAll("\r", "\\r")}); record it in THIRD_PARTY_NOTICES.md instead`,
        },
      ]);
    });
  });

const POINTER_VECTORS: ReadonlyArray<readonly [string, string, string | null]> =
  [
    [
      "upper case with an extension",
      "THIRD_PARTY_NOTICES.md",
      "THIRD_PARTY_NOTICES",
    ],
    [
      "lower case, echoed as written",
      "third_party_notices",
      "third_party_notices",
    ],
    ["mixed case", "Third_Party_Notices", "Third_Party_Notices"],
    [
      "the long s folds to S",
      `THIRD_PARTY_NOTICE${LONG_S}`,
      `THIRD_PARTY_NOTICE${LONG_S}`,
    ],
    ["a missing final letter", "THIRD_PARTY_NOTICE", null],
    ["hyphens instead of underscores", "THIRD-PARTY-NOTICES", null],
  ];

for (const [name, text, matched] of POINTER_VECTORS)
  test(`noticesPointerRe: ${name}`, async () => {
    await withSkillsRepo(async (root) => {
      writeFile(root, NOTES, text);
      const failures = await failuresOf(root);
      if (matched === null) {
        assert.deepEqual(failures, []);
        return;
      }
      assert.deepEqual(failures, [
        {
          rule: "attribution-centralized",
          path: NOTES,
          message: `points into the Agentics repository ("${matched}"); a skill must stay self-contained`,
        },
      ]);
    });
  });

// --- reference-attribution ----------------------------------------------------------

test("changing the Adapted-from header of an Apache file is one pinned-notice failure", async () => {
  await withSkillsRepo(async (root) => {
    mutateFile(root, DOCS_SKILL, "Adapted from", "Based on");
    assert.deepEqual(await failuresOf(root), [
      {
        rule: "reference-attribution",
        path: DOCS_SKILL,
        message:
          "pinned Apache-2.0 attribution notice (source + license) missing",
      },
    ]);
  });
});

test("removing an Apache file is one missing-file failure", async () => {
  await withSkillsRepo(async (root) => {
    remove(root, DOCS_SKILL);
    assert.deepEqual(await failuresOf(root), [
      {
        rule: "reference-attribution",
        path: DOCS_SKILL,
        message: "attributed third-party file missing",
      },
    ]);
  });
});

test("the retired pointer form of the docs notice gives the pointer failure then the missing-notice failure", async () => {
  await withSkillsRepo(async (root) => {
    mutateFile(
      root,
      "skills/docs/SKILL.md",
      "(Apache-2.0); modified. -->",
      "(Apache-2.0). See THIRD_PARTY_NOTICES.md. -->",
    );
    const failures = await failuresOf(root);
    assert.deepEqual(
      failures.map((one) => [one.rule, one.path]),
      [
        ["attribution-centralized", "skills/docs/SKILL.md"],
        ["reference-attribution", "skills/docs/SKILL.md"],
      ],
    );
    assert.equal(
      failures[1]!.message,
      "pinned Apache-2.0 attribution notice (source + license) missing",
    );
  });
});

test("removing the whole docs skill leaves one missing-file failure", async () => {
  await withSkillsRepo(async (root) => {
    remove(root, "skills/docs");
    assert.deepEqual(
      (await failuresOf(root)).map((one) => [one.rule, one.path, one.message]),
      [
        [
          "reference-attribution",
          DOCS_SKILL,
          "attributed third-party file missing",
        ],
      ],
    );
  });
});

test("an Apache file that is a directory is an audit error", async () => {
  await withSkillsRepo(async (root) => {
    remove(root, DOCS_SKILL);
    mkdir(root, DOCS_SKILL);
    const result = await audit(root);
    assert.deepEqual(result.failures, []);
    assert.equal(
      result.error,
      `skills: reading ${DOCS_SKILL}: ${DOCS_SKILL}: is not a regular file`,
    );
  });
});

test("skills/docs linked elsewhere is an audit error naming the link", async (t) => {
  await withSkillsRepo(async (root) => {
    const outside = mkdtempSync(
      path.join(tmpdir(), "agentics-references-out-"),
    );
    try {
      writeFile(outside, "SKILL.md", "SENTINEL");
      remove(root, "skills/docs");
      if (linkDirectory(outside, path.join(root, "skills/docs")) === undefined)
        return skipForLink(t, "a directory link");
      assert.deepEqual(await audit(root), {
        failures: [],
        error: `skills: reading ${DOCS_SKILL}: skills/docs: passes through a link`,
      });
    } finally {
      removeTree(outside);
    }
  });
});

// --- notices-manifest ---------------------------------------------------------------

test("a renamed manifest heading is one failure with the heading quoted", async () => {
  await withSkillsRepo(async (root) => {
    mutateFile(root, NOTICES_FILE, HEADING, "## Something else");
    assert.deepEqual(await failuresOf(root), [
      {
        rule: "notices-manifest",
        path: NOTICES_FILE,
        message: 'missing "## Per-file provenance manifest" section',
      },
    ]);
  });
});

test("a removed notices file is exactly one failure", async () => {
  await withSkillsRepo(async (root) => {
    remove(root, NOTICES_FILE);
    assert.deepEqual(await failuresOf(root), [
      {
        rule: "notices-manifest",
        path: NOTICES_FILE,
        message: "third-party notices file missing",
      },
    ]);
  });
});

test("notices content without the heading or any path gives the heading failure then a row failure per Apache file", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(root, NOTICES_FILE, "# x");
    const failures = await failuresOf(root);
    assert.deepEqual(
      failures.map((one) => [one.rule, one.path, one.message]),
      [
        [
          "notices-manifest",
          NOTICES_FILE,
          'missing "## Per-file provenance manifest" section',
        ],
        ...APACHE_ATTRIBUTED_FILES.map((entry) => [
          "notices-manifest",
          NOTICES_FILE,
          `no manifest row for ${entry.rel}`,
        ]),
      ],
    );
  });
});

test("deleting only the row of skills/docs/SKILL.md gives one failure for it", async () => {
  await withSkillsRepo(async (root) => {
    const target = path.join(root, NOTICES_FILE);
    const original = readFileSync(target, "utf8");
    const kept = original
      .split("\n")
      .filter((line) => !line.includes("docs/SKILL.md"))
      .join("\n");
    assert.notEqual(kept, original);
    writeFile(root, NOTICES_FILE, kept);
    assert.deepEqual(await failuresOf(root), [
      {
        rule: "notices-manifest",
        path: NOTICES_FILE,
        message: "no manifest row for skills/docs/SKILL.md",
      },
    ]);
  });
});

test("a notices file that is a link is an audit error naming the link", async (t) => {
  await withSkillsRepo(async (root) => {
    const outside = mkdtempSync(
      path.join(tmpdir(), "agentics-references-out-"),
    );
    try {
      writeFile(outside, NOTICES_FILE, HEADING + "\n");
      remove(root, NOTICES_FILE);
      if (!link(root, NOTICES_FILE, path.join(outside, NOTICES_FILE), "file"))
        return skipForLink(t, "a file symlink");
      assert.deepEqual(await audit(root), {
        failures: [],
        error: `skills: reading ${NOTICES_FILE}: ${NOTICES_FILE}: passes through a link`,
      });
    } finally {
      removeTree(outside);
    }
  });
});

test("a notices file that is a directory is an audit error", async () => {
  await withSkillsRepo(async (root) => {
    remove(root, NOTICES_FILE);
    mkdir(root, NOTICES_FILE);
    const result = await audit(root);
    assert.deepEqual(result.failures, []);
    assert.ok(
      result.error?.startsWith(`skills: reading ${NOTICES_FILE}: `),
      result.error ?? "no audit error",
    );
  });
});

// --- family order ---------------------------------------------------------------------

test("one failure of each rule comes out in the family order", async () => {
  await withSkillsRepo(async (root) => {
    mutateFile(root, NOTICES_FILE, HEADING, "## Something else");
    mutateFile(root, DOCS_SKILL, "Adapted from", "Based on");
    writeFile(
      root,
      "skills/scaffold/references/audit.md",
      "See THIRD_PARTY_NOTICES.md.\n",
    );
    writeFile(root, NOTES, "[zz](./zz.md)\n");
    assert.deepEqual(rules(await failuresOf(root)), [
      "reference-link",
      "attribution-centralized",
      "reference-attribution",
      "notices-manifest",
    ]);
  });
});

// --- reference-naming ---------------------------------------------------------------------

// The naming messages hold an em dash, built here from its code point.

const EM_DASH = String.fromCodePoint(0x2014);
const E_ACUTE = String.fromCodePoint(0xe9);
const DIR_MESSAGE = `reference directory name is not lowercase kebab-case ${EM_DASH} rename with 'git mv'`;
const FILE_MESSAGE = `reference file name is not lowercase kebab-case (exempt: README.md, LICENSE) ${EM_DASH} rename with 'git mv'`;
const NO_REFERENCES = `skills: no reference files found ${EM_DASH} the reference-naming glob is wrong`;
const REFS = "skills/prototype/references";

function naming(root: string): Promise<AuditResult> {
  return run(root, options, { families: [REFERENCE_NAMING_FAMILY] });
}

/** The failure paths of a naming run that must not have raised an audit error. */
async function namingPaths(root: string): Promise<string[]> {
  const result = await naming(root);
  assert.equal(result.error, undefined);
  return result.failures.map((one) => one.path);
}

const fileFailure = (rel: string): Failure => ({
  rule: "reference-naming",
  path: rel,
  message: FILE_MESSAGE,
});

const dirFailure = (rel: string): Failure => ({
  rule: "reference-naming",
  path: rel,
  message: DIR_MESSAGE,
});

/** Runs `body` on an empty directory that is removed afterwards. */
async function withBareRoot<T>(body: (root: string) => Promise<T>): Promise<T> {
  const root = mkdtempSync(path.join(tmpdir(), "agentics-naming-"));
  try {
    return await body(root);
  } finally {
    removeTree(root);
  }
}

test("the family is named reference-naming and has the one rule reference-naming", () => {
  assert.equal(REFERENCE_NAMING_FAMILY.name, "reference-naming");
  assert.deepEqual(REFERENCE_NAMING_FAMILY.rules, ["reference-naming"]);
});

test("a clean clone of the green fixture gives no naming failure and no error", async () => {
  await withSkillsRepo(async (root) => {
    assert.deepEqual(await naming(root), { failures: [] });
  });
});

test("a badly named file is one failure with the file message", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(root, `${REFS}/Bad_Name.md`, "# bad\n");
    assert.deepEqual(await naming(root), {
      failures: [fileFailure(`${REFS}/Bad_Name.md`)],
    });
  });
});

test("a badly named directory is one failure with the directory message and the walk still descends into it", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(root, `${REFS}/BadDir/fine.md`, "# fine\n");
    assert.deepEqual(await naming(root), {
      failures: [dirFailure(`${REFS}/BadDir`)],
    });
    writeFile(root, `${REFS}/BadDir/Worse_Name.md`, "# worse\n");
    assert.deepEqual(await naming(root), {
      failures: [
        dirFailure(`${REFS}/BadDir`),
        fileFailure(`${REFS}/BadDir/Worse_Name.md`),
      ],
    });
  });
});

test("README.md and LICENSE are exempt by exact name and every near miss gives the file message", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(root, `${REFS}/README.md`, "# index\n");
    writeFile(root, `${REFS}/LICENSE`, "MIT\n");
    assert.deepEqual(await naming(root), { failures: [] });
    remove(root, REFS);
    // One name at a time: on a case-insensitive host README.MD and README.md would be the same file.
    for (const name of [
      "README.MD",
      "LICENSE.md",
      "NOTICE",
      "a.test.md",
      ".gitkeep",
      "a_b.md",
    ]) {
      writeFile(root, `${REFS}/${name}`, "x\n");
      assert.deepEqual(
        await naming(root),
        { failures: [fileFailure(`${REFS}/${name}`)] },
        name,
      );
      remove(root, `${REFS}/${name}`);
    }
    mkdir(root, `${REFS}/README.md`);
    assert.deepEqual(await naming(root), {
      failures: [dirFailure(`${REFS}/README.md`)],
    });
  });
});

test("the file name pattern is lowercase ASCII kebab-case with one extension", async () => {
  await withSkillsRepo(async (root) => {
    const valid = ["a-b.md", "a1-2.md", "a.md"];
    const invalid = [
      "a--b.md",
      "-a.md",
      "a-.md",
      "A.md",
      "a.MD",
      `${E_ACUTE}.md`,
    ];
    // One name at a time: A.md and a.md are one file on a case-insensitive host.
    for (const name of [...valid, ...invalid]) {
      writeFile(root, `${REFS}/${name}`, "x\n");
      assert.deepEqual(
        await naming(root),
        {
          failures: invalid.includes(name)
            ? [fileFailure(`${REFS}/${name}`)]
            : [],
        },
        name,
      );
      remove(root, `${REFS}/${name}`);
    }
  });
});

test("the directory name pattern is lowercase ASCII kebab-case", async () => {
  await withSkillsRepo(async (root) => {
    for (const name of ["a-b", "a.b", "a_b", "A"]) {
      mkdir(root, `${REFS}/${name}`);
      assert.deepEqual(
        await naming(root),
        {
          failures: name === "a-b" ? [] : [dirFailure(`${REFS}/${name}`)],
        },
        name,
      );
      remove(root, `${REFS}/${name}`);
    }
  });
});

test("failures come in pre-order with byte order at each level and no global sort", async () => {
  await withSkillsRepo(async (root) => {
    for (const name of ["A.md", "B-dir/c_d.md", "B-dir.md", "a_b.md"])
      writeFile(root, `${REFS}/${name}`, "x\n");
    assert.deepEqual(await namingPaths(root), [
      `${REFS}/A.md`,
      `${REFS}/B-dir`,
      `${REFS}/B-dir/c_d.md`,
      `${REFS}/B-dir.md`,
      `${REFS}/a_b.md`,
    ]);
  });
});

test("skills with bad names report in byte order of their skill directories", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(root, "skills/alpha/references/Bad_Name.md", "x\n");
    writeFile(root, "skills/Zeta/references/Bad_Name.md", "x\n");
    writeFile(root, "skills/docs/references/Bad_Name.md", "x\n");
    assert.deepEqual(await namingPaths(root), [
      "skills/Zeta/references/Bad_Name.md",
      "skills/alpha/references/Bad_Name.md",
      "skills/docs/references/Bad_Name.md",
    ]);
  });
});

test("a directory named Reference is not the references root and a regular file named references is skipped", async () => {
  await withBareRoot(async (root) => {
    writeFile(root, "skills/a/Reference/Bad_Name.md", "x\n");
    writeFile(root, "skills/b/references", "not a directory\n");
    assert.deepEqual(await naming(root), {
      failures: [],
      error: NO_REFERENCES,
    });
    writeFile(root, "skills/c/references/ok.md", "x\n");
    assert.deepEqual(await naming(root), { failures: [] });
  });
});

// --- reference-naming: the zero-entries audit error ---------------------------------------------

test("a checkout whose skills have no references anywhere is the audit error", async () => {
  await withBareRoot(async (root) => {
    writeFile(root, "skills/solo/SKILL.md", "---\nname: solo\n---\n");
    assert.deepEqual(await naming(root), {
      failures: [],
      error: NO_REFERENCES,
    });
  });
  await withBareRoot(async (root) => {
    assert.deepEqual(await naming(root), {
      failures: [],
      error: NO_REFERENCES,
    });
  });
});

test("exempt files and an empty subdirectory still count as entries", async () => {
  await withBareRoot(async (root) => {
    writeFile(root, "skills/a/references/README.md", "# index\n");
    assert.deepEqual(await naming(root), { failures: [] });
  });
  await withBareRoot(async (root) => {
    mkdir(root, "skills/a/references/sub");
    assert.deepEqual(await naming(root), { failures: [] });
  });
});

test("an empty references directory and a regular file named references each count nothing", async () => {
  await withBareRoot(async (root) => {
    mkdir(root, "skills/a/references");
    assert.deepEqual(await naming(root), {
      failures: [],
      error: NO_REFERENCES,
    });
  });
  await withBareRoot(async (root) => {
    writeFile(root, "skills/a/references", "not a directory\n");
    assert.deepEqual(await naming(root), {
      failures: [],
      error: NO_REFERENCES,
    });
  });
});

// --- reference-naming: symlinks -----------------------------------------------------------------

test("naming ignores linked roots and linked entries", async (t) => {
  await withBareRoot(async (root) => {
    await withBareRoot(async (outside) => {
      writeFile(outside, "Bad_Name.md", "SENTINEL\n");
      writeFile(outside, "references/Bad_Name.md", "SENTINEL\n");
      writeFile(root, "skills/good/references/valid.md", "ok\n");
      if (
        linkDirectory(outside, path.join(root, "skills/linked")) === undefined
      )
        return skipForLink(t, "a directory link");
      assert.ok(
        linkDirectory(outside, path.join(root, "skills/other/references")) !==
          undefined,
      );
      assert.ok(
        link(
          root,
          "skills/good/references/Bad_Link.md",
          path.join(outside, "Bad_Name.md"),
          "file",
        ),
      );
      assert.deepEqual(await naming(root), { failures: [] });
    });
  });
});

test("a checkout whose only reference entry is a linked references directory is the audit error", async (t) => {
  await withBareRoot(async (root) => {
    await withBareRoot(async (outside) => {
      writeFile(outside, "good.md", "SENTINEL\n");
      if (
        linkDirectory(outside, path.join(root, "skills/a/references")) ===
        undefined
      )
        return skipForLink(t, "a directory link");
      assert.deepEqual(await naming(root), {
        failures: [],
        error: NO_REFERENCES,
      });
    });
  });
});

// --- reference-naming: how the skill directories and references roots are enumerated ------------

test("a listing that fails for one skill directory without names selects nothing there", async () => {
  await withBareRoot(async (root) => {
    writeFile(root, "skills/blocked/SKILL.md", "blocked");
    writeFile(root, "skills/blocked/references/Bad_Name.md", "blocked");
    writeFile(root, "skills/z/SKILL.md", "z");
    writeFile(root, "skills/a/SKILL.md", "a");
    writeFile(root, "skills/a/references/good.md", "ok");
    writeFile(root, "skills/case/skill.md", "wrong case");
    const real = seams.readSkillDirectory;
    const result = await withSeams(
      {
        readSkillDirectory: (dir) =>
          dir === path.join(root, "skills", "blocked")
            ? {
                names: [],
                error: Object.assign(new Error("denied"), { code: "EACCES" }),
              }
            : real(dir),
      },
      () => naming(root),
    );
    assert.deepEqual(result, { failures: [] });
  });
});

test("a listing of a skill directory that returns its names with an error is still used", async () => {
  await withBareRoot(async (root) => {
    writeFile(root, "skills/a/SKILL.md", "ok");
    writeFile(root, "skills/a/references/good.md", "ok");
    mkdir(root, "skills/missing");
    const real = seams.readSkillDirectory;
    const result = await withSeams(
      {
        readSkillDirectory: (dir) => {
          const listing = real(dir);
          return dir === path.join(root, "skills", "a")
            ? { names: listing.names, error: new Error("denied") }
            : listing;
        },
      },
      () => naming(root),
    );
    assert.deepEqual(result, { failures: [] });
  });
});

test("a listing of skills that returns its names with an error still yields the failure", async () => {
  await withBareRoot(async (root) => {
    writeFile(root, "skills/a/SKILL.md", "ok");
    writeFile(root, "skills/a/references/Bad_Name.md", "bad");
    writeFile(root, "skills/z/SKILL.md", "ok");
    const real = seams.readSkillDirectory;
    const result = await withSeams(
      {
        readSkillDirectory: (dir) => {
          const listing = real(dir);
          return dir === path.join(root, "skills")
            ? { names: listing.names, error: new Error("denied") }
            : listing;
        },
      },
      () => naming(root),
    );
    assert.deepEqual(result, {
      failures: [fileFailure("skills/a/references/Bad_Name.md")],
    });
  });
});

test("a listed references directory that does not exist is the audit error carrying a missing cause", async () => {
  await withBareRoot(async (root) => {
    writeFile(root, "skills/a/SKILL.md", "ok");
    writeFile(root, "skills/a/references/good.md", "ok");
    mkdir(root, "skills/missing");
    const real = seams.readSkillDirectory;
    await withSeams(
      {
        readSkillDirectory: (dir) =>
          real(
            dir === path.join(root, "skills", "missing")
              ? path.join(root, "skills", "a")
              : dir,
          ),
      },
      async () => {
        const result = await naming(root);
        assert.deepEqual(result.failures, []);
        assert.equal(
          result.error,
          "skills: checking skills/missing/references: skills/missing/references: does not exist",
        );
        await assert.rejects(
          async () =>
            REFERENCE_NAMING_FAMILY.run(root, { families: [], evalsCheck }),
          (error: unknown) =>
            error instanceof Error &&
            error.cause instanceof RepoFileError &&
            error.cause.kind === "missing",
        );
      },
    );
  });
});

// --- reference-naming: the repository root as a glob pattern -------------------------------------

test("a checkout under a directory named [ is the unchanged pattern error", async () => {
  await withBareRoot(async (base) => {
    const root = path.join(base, "[");
    writeFile(root, "skills/a/references/good.md", "ok");
    assert.deepEqual(await naming(root), {
      failures: [],
      error: "syntax error in pattern",
    });
  });
});

test("a checkout under a directory named [x] is read as the literal name", async () => {
  await withBareRoot(async (base) => {
    const root = path.join(base, "[x]");
    writeFile(root, "skills/a/references/Bad_Name.md", "bad");
    writeFile(
      path.join(base, "x"),
      "skills/other/references/Bad_Other.md",
      "x",
    );
    assert.deepEqual(await naming(root), {
      failures: [fileFailure("skills/a/references/Bad_Name.md")],
    });
  });
});

// --- audit error texts and causes, rows added by the mutation pass -----------------------------------------------------

test("a notices file whose read fails as missing is the missing-file failure, from a code or a file error", async () => {
  await withSkillsRepo(async (root) => {
    const target = path.join(root, NOTICES_FILE);
    const real = seams.readFile;
    const gone = (): Error =>
      Object.assign(new Error("gone"), { code: "ENOENT" });
    for (const [label, error] of [
      ["the not-exist code", gone()],
      ["a missing file error", new RepoFileError("missing", NOTICES_FILE)],
    ] as const) {
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
          failures: [
            {
              rule: "notices-manifest",
              path: NOTICES_FILE,
              message: "third-party notices file missing",
            },
          ],
        },
        label,
      );
    }
  });
});

test("a notices file that cannot be inspected or read is an audit error naming it and why", async () => {
  await withSkillsRepo(async (root) => {
    const target = path.join(root, NOTICES_FILE);
    const realLstat = seams.lstat;
    const rows: [string, unknown, string][] = [
      [
        "a coded failure",
        Object.assign(new Error("EACCES: permission denied, lstat"), {
          code: "EACCES",
          syscall: "lstat",
          path: target,
        }),
        `${NOTICES_FILE}: EACCES`,
      ],
      [
        "a failure with no code",
        new Error("odd failure"),
        `${NOTICES_FILE}: unreadable`,
      ],
      [
        "a file error for another path",
        new RepoFileError("symlink", "somewhere"),
        "somewhere: passes through a link",
      ],
      ["no error object", null, `${NOTICES_FILE}: unreadable`],
    ];
    for (const [label, error, text] of rows) {
      const result = await withSeams(
        {
          lstat: (candidate) => {
            if (candidate === target) throw error;
            return realLstat(candidate);
          },
        },
        () => audit(root),
      );
      assert.equal(
        result.error,
        `skills: reading ${NOTICES_FILE}: ${text}`,
        label,
      );
    }
    const realRead = seams.readFile;
    const readNull = await withSeams(
      {
        readFile: (candidate) => {
          if (candidate === target) throw null;
          return realRead(candidate);
        },
      },
      () => audit(root),
    );
    assert.equal(
      readNull.error,
      `skills: reading ${NOTICES_FILE}: ${NOTICES_FILE}: unreadable`,
    );
  });
});

test("an audit error from a walk keeps the walk's failure as its cause", async () => {
  await withBareRoot(async (root) => {
    writeFile(root, "skills/a/SKILL.md", "ok");
    writeFile(root, "skills/a/references/good.md", "ok");
    const context = { families: [], evalsCheck };
    const expectWalkCause = (
      family: typeof REFERENCE_LINK_FAMILY,
      dir: string,
      shown: string,
      label: string,
    ) => {
      const real = seams.readDir;
      withSeams(
        {
          readDir: (candidate) => {
            if (candidate === dir)
              throw Object.assign(new Error("denied"), { code: "EACCES" });
            return real(candidate);
          },
        },
        () =>
          assert.throws(
            () => family.run(root, context),
            (error: unknown) =>
              error instanceof Error &&
              error.cause instanceof RepoFileError &&
              error.cause.message === `${shown}: EACCES`,
            label,
          ),
      );
    };
    expectWalkCause(
      REFERENCE_LINK_FAMILY,
      path.join(root, "skills"),
      "skills",
      "the skills walk",
    );
    expectWalkCause(
      REFERENCE_NAMING_FAMILY,
      path.join(root, "skills", "a", "references"),
      "skills/a/references",
      "the references walk",
    );
  });
});

test("a failed walk of a references directory is the audit error skills: walking <directory>", async () => {
  await withBareRoot(async (root) => {
    writeFile(root, "skills/a/SKILL.md", "ok");
    writeFile(root, "skills/a/references/good.md", "ok");
    const refs = path.join(root, "skills", "a", "references");
    const real = seams.readDir;
    const result = await withSeams(
      {
        readDir: (dir) => {
          if (dir === refs)
            throw Object.assign(new Error("denied"), { code: "EACCES" });
          return real(dir);
        },
      },
      () => naming(root),
    );
    assert.deepEqual(result, {
      failures: [],
      error: "skills: walking skills/a/references: skills/a/references: EACCES",
    });
  });
});

test("an audit error from a read keeps the read failure as its cause", async () => {
  await withSkillsRepo(async (root) => {
    const unread = new Error("read failed");
    withSeams(
      {
        readFile: () => {
          throw unread;
        },
      },
      () =>
        assert.throws(
          () => REFERENCE_LINK_FAMILY.run(root, { families: [], evalsCheck }),
          (error: unknown) =>
            error instanceof Error &&
            error.cause instanceof RepoFileError &&
            error.cause.cause === unread,
        ),
    );
  });
});

test("the files are checked in sort order of their whole paths, not in walk order", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(root, "skills/a/SKILL.md", "[gone](./gone.md)\n");
    writeFile(root, "skills/a-b/SKILL.md", "[gone](./gone.md)\n");
    const result = await audit(root);
    assert.equal(result.error, undefined);
    assert.deepEqual(
      result.failures.map((one) => one.path),
      ["skills/a-b/SKILL.md", "skills/a/SKILL.md"],
    );
  });
});

// --- the whole-domain scenarios ------------------------------------------------------------------------------------

const METADATA_SLICE = sliceNamed("metadata");
const CONTENT_SLICE = sliceNamed("content");

const A_MD = `${PROTOTYPE_REFS}/a.md`;

/** The result of an audit whose only failures are the given ones, without an audit error. */
const failing = (...failures: Failure[]): AuditResult => ({ failures });

const danglingIn = (rel: string, target: string): Failure => ({
  rule: "reference-link",
  path: rel,
  message: `dangling link -> ${target}`,
});

const markerIn = (rel: string, text: string): Failure => ({
  rule: "attribution-centralized",
  path: rel,
  message: `in-file third-party attribution ("${text}"); record it in THIRD_PARTY_NOTICES.md instead`,
});

const pointerIn = (rel: string, text: string): Failure => ({
  rule: "attribution-centralized",
  path: rel,
  message: `points into the Agentics repository ("${text}"); a skill must stay self-contained`,
});

const noticesFailure = (message: string): Failure => ({
  rule: "notices-manifest",
  path: NOTICES_FILE,
  message,
});

const MISSING_HEADING = `missing "${HEADING}" section`;
const NO_DOCS_ROW = `no manifest row for ${DOCS_SKILL}`;

const outsideSkills = (rel: string, line: number, token: string): Failure => ({
  rule: "skill-repo-independence",
  path: rel,
  message: `line ${line} names "${token}": a skill must not depend on an Agentics hook or on a file outside skills/`,
});

// Each rule's breaker, through every family on the green fixture.

test("attribution-centralized", async () => {
  assert.deepEqual(
    await auditEveryFamily(breaker("attribution-centralized")),
    failing(
      markerIn(
        `${PROTOTYPE_REFS}/origin.md`,
        "Adapted from example/project (MIT)",
      ),
    ),
  );
});

test("notices-manifest", async () => {
  assert.deepEqual(
    await auditEveryFamily(breaker("notices-manifest")),
    failing(noticesFailure(MISSING_HEADING)),
  );
});

test("reference-attribution", async () => {
  assert.deepEqual(
    await auditEveryFamily(breaker("reference-attribution")),
    failing({
      rule: "reference-attribution",
      path: DOCS_SKILL,
      message:
        "pinned Apache-2.0 attribution notice (source + license) missing",
    }),
  );
});

test("reference-link", async () => {
  assert.deepEqual(
    await auditEveryFamily(breaker("reference-link")),
    failing(danglingIn(NOTES, "./missing.md")),
  );
});

test("reference-naming", async () => {
  assert.deepEqual(
    await auditEveryFamily(breaker("reference-naming")),
    failing(fileFailure(`${PROTOTYPE_REFS}/Bad_Name.md`)),
  );
});

test("skills/execute is removed", async () => {
  // Three skills link into the removed flow-state file, the lean-cadence and gate record-keeping sources are gone, and
  // the standards baseline still lists findings of the removed skill.
  const leanSource = (rel: string): Failure => ({
    rule: "lean-cadence-source-present",
    path: rel,
    message: "not found: the lean-cadence pins cannot be checked",
  });
  const stale = (rule: string, key: string): Failure =>
    staleBaseline("execute", rule, "skills/execute/SKILL.md", key);
  const cannotRead = (rel: string, carries: string): Failure => ({
    rule: "gate-record-keeping",
    path: rel,
    message: `cannot read ${rel}, which carries the rule that ${carries}: skills/execute: does not exist`,
  });
  const flowState = "../execute/references/flow-state.md";
  assert.deepEqual(
    await auditEveryFamily((r) => remove(r, "skills/execute")),
    failing(
      danglingIn("skills/decompose/SKILL.md", flowState),
      danglingIn("skills/gate/SKILL.md", flowState),
      danglingIn("skills/triage/SKILL.md", flowState),
      leanSource("skills/execute/references/stage-1-dispatch-rule.md"),
      leanSource("skills/execute/SKILL.md"),
      leanSource("skills/execute/references/flow-state.md"),
      leanSource("skills/execute"),
      stale("skill-frontmatter-key", "version"),
      stale("skill-references-index", "references"),
      stale("skill-routing-section", "routing"),
      cannotRead(
        "skills/execute/references/fix-entry.md",
        "gate and `execute fix` account for the branch by calling the helper's audit instead of a hand procedure",
      ),
      cannotRead(
        "skills/execute/SKILL.md",
        "`execute`'s boundary check and completion check each send a standalone fix to the fix entry's audit",
      ),
      cannotRead(
        "skills/execute/references/flow-state.md",
        "gate never carries an approval over its own edit",
      ),
    ),
  );
});

// The metadata slice families, on the pasted-format fixture: fences and code spans.

test("a link after a plain fence", async () => {
  assert.deepEqual(
    await auditSlice(
      METADATA_SLICE,
      ref("a.md", "```\n[x](./gone.md)\n```\n[y](./after.md)\n"),
    ),
    failing(danglingIn(A_MD, "./after.md")),
  );
});

test("a four-backtick fence nesting a three-backtick one", async () => {
  assert.deepEqual(
    await auditSlice(
      METADATA_SLICE,
      ref(
        "a.md",
        "````\n```\n[x](./inside.md)\n```\n[y](./still.md)\n````\n[z](./after.md)\n",
      ),
    ),
    failing(danglingIn(A_MD, "./after.md")),
  );
});

test("a quote fence with a closer", async () => {
  assert.deepEqual(
    await auditSlice(
      METADATA_SLICE,
      ref("a.md", "> ```\n> [x](./inside.md)\n> ```\n[y](./after.md)\n"),
    ),
    failing(danglingIn(A_MD, "./after.md")),
  );
});

test("a quote fence without a closer", async () => {
  assert.deepEqual(
    await auditSlice(
      METADATA_SLICE,
      ref("a.md", "> ```\n> [x](./inside.md)\n\n[y](./after.md)\n"),
    ),
    failing(danglingIn(A_MD, "./after.md")),
  );
});

test("a list-nested fence", async () => {
  assert.deepEqual(
    await auditSlice(
      METADATA_SLICE,
      ref(
        "a.md",
        "- item\n\n  ```\n  [x](./inside.md)\n  ```\n\n[y](./after.md)\n",
      ),
    ),
    failing(danglingIn(A_MD, "./after.md")),
  );
});

test("a list-nested fence with an outdented line", async () => {
  // The outdented line is outside the fence; the closer below it opens a fence that hides the last link.
  assert.deepEqual(
    await auditSlice(
      METADATA_SLICE,
      ref(
        "a.md",
        "- item\n  ```\n[x](./outdented.md)\n  ```\n[y](./after.md)\n",
      ),
    ),
    failing(danglingIn(A_MD, "./outdented.md")),
  );
});

test("an opener and a link on the same line", async () => {
  assert.deepEqual(
    await auditSlice(
      METADATA_SLICE,
      ref(
        "a.md",
        "```[x](./same.md)\n[y](./inside.md)\n```\n[z](./after.md)\n",
      ),
    ),
    failing(danglingIn(A_MD, "./after.md")),
  );
});

test("a nested quote fence", async () => {
  assert.deepEqual(
    await auditSlice(
      METADATA_SLICE,
      ref("a.md", ">> ```\n>> [x](./inside.md)\n>> ```\n[y](./after.md)\n"),
    ),
    failing(danglingIn(A_MD, "./after.md")),
  );
});

test("a CRLF fenced file", async () => {
  assert.deepEqual(
    await auditSlice(
      METADATA_SLICE,
      ref("a.md", crlf("```\n[x](./inside.md)\n```\n[y](./after.md)\n")),
    ),
    failing(danglingIn(A_MD, "./after.md")),
  );
});

test("a link after an unpaired backtick", async () => {
  assert.deepEqual(
    await auditSlice(METADATA_SLICE, ref("a.md", "`x [y](./gone.md)\n")),
    failing(danglingIn(A_MD, "./gone.md")),
  );
});

test("a link between two code spans", async () => {
  assert.deepEqual(
    await auditSlice(METADATA_SLICE, ref("a.md", "`a` [y](./gone.md) `b`\n")),
    failing(danglingIn(A_MD, "./gone.md")),
  );
});

// The metadata slice families, on the pasted-format fixture: link targets.

test("a target with a colon", async () => {
  assert.deepEqual(
    await auditSlice(METADATA_SLICE, ref("a.md", "[x](a:b.md)\n")),
    failing(danglingIn(A_MD, "a:b.md")),
  );
});

test("a target with an NBSP", async () => {
  assert.deepEqual(
    await auditSlice(METADATA_SLICE, ref("a.md", `[x](a${NBSP}b.md)\n`)),
    failing(danglingIn(A_MD, `a${NBSP}b.md`)),
  );
});

test("an upper-case HTTP scheme", async () => {
  assert.deepEqual(
    await auditSlice(METADATA_SLICE, ref("a.md", "[x](HTTP://x.md)\n")),
    failing(danglingIn(A_MD, "HTTP://x.md")),
  );
});

test("a target with an anchor that ends in .md", async () => {
  assert.deepEqual(
    await auditSlice(METADATA_SLICE, ref("a.md", "[x](a.md#x.md)\n")),
    failing(danglingIn(A_MD, "a.md#x.md")),
  );
});

test("duplicate dangling links in reverse order", async () => {
  // One failure per unique target, in byte order of the targets.
  assert.deepEqual(
    await auditSlice(
      METADATA_SLICE,
      ref("a.md", "[z](./z.md) [y](./y.md) [z](./z.md)\n"),
    ),
    failing(danglingIn(A_MD, "./y.md"), danglingIn(A_MD, "./z.md")),
  );
});

test("a directory named d.md as a link target", async () => {
  // The directory satisfies the link but not the naming rule.
  assert.deepEqual(
    await auditSlice(
      METADATA_SLICE,
      all(ref("a.md", "[x](./d.md)\n"), (r) =>
        mkdir(r, `${PROTOTYPE_REFS}/d.md`),
      ),
    ),
    failing(dirFailure(`${PROTOTYPE_REFS}/d.md`)),
  );
});

test(
  "a symlinked target that dangles",
  { skip: canSymlink() ? false : SYMLINK_SKIP },
  async () => {
    assert.deepEqual(
      await auditSlice(
        METADATA_SLICE,
        all(
          ref("a.md", "[x](./ln.md)\n"),
          symlinked(`${PROTOTYPE_REFS}/ln.md`, "nowhere.md", "file"),
        ),
      ),
      failing(danglingIn(A_MD, "./ln.md")),
    );
  },
);

// The metadata slice families, on the pasted-format fixture: attribution markers and notices pointers.

test("the shortest attribution marker span", async () => {
  assert.deepEqual(
    await auditSlice(METADATA_SLICE, ref("a.md", "adapted from (MIT)\n")),
    failing(markerIn(A_MD, "adapted from (MIT)")),
  );
});

test("a lower-case attribution marker", async () => {
  assert.deepEqual(
    await auditSlice(METADATA_SLICE, ref("a.md", "adapted from x (mit)\n")),
    failing(markerIn(A_MD, "adapted from x (mit)")),
  );
});

test("an attribution marker 200 characters wide", async () => {
  const marker = `adapted from${EMOJI.repeat(200)}(MIT)`;
  assert.deepEqual(
    await auditSlice(METADATA_SLICE, ref("a.md", `${marker}\n`)),
    failing(markerIn(A_MD, marker)),
  );
});

test("a notices pointer spelled with U+017F", async () => {
  assert.deepEqual(
    await auditSlice(
      METADATA_SLICE,
      ref("a.md", `see THIRD_PARTY_NOTICE${LONG_S}\n`),
    ),
    failing(pointerIn(A_MD, `THIRD_PARTY_NOTICE${LONG_S}`)),
  );
});

test("a notices pointer in an Apache file", async () => {
  const notice =
    "<!-- Adapted from anthropics/skills doc-coauthoring (Apache-2.0); modified. -->";
  assert.deepEqual(
    await auditSlice(METADATA_SLICE, (r) =>
      mutateFile(
        r,
        DOCS_SKILL,
        notice,
        `${notice}\nSee THIRD_PARTY_NOTICES.md.`,
      ),
    ),
    failing(pointerIn(DOCS_SKILL, "THIRD_PARTY_NOTICES")),
  );
});

// The metadata slice families, on the pasted-format fixture: the notices file.

test("the notices file removed", async () => {
  assert.deepEqual(
    await auditSlice(METADATA_SLICE, (r) => remove(r, NOTICES_FILE)),
    failing(noticesFailure("third-party notices file missing")),
  );
});

test("the manifest row of one file removed", async () => {
  assert.deepEqual(
    await auditSlice(METADATA_SLICE, (r) =>
      mutateFile(
        r,
        NOTICES_FILE,
        "| `docs/SKILL.md` | fixture | Apache-2.0 |\n",
        "",
      ),
    ),
    failing(noticesFailure(NO_DOCS_ROW)),
  );
});

test("the notices content replaced", async () => {
  assert.deepEqual(
    await auditSlice(METADATA_SLICE, (r) =>
      writeFile(r, NOTICES_FILE, "# Nothing\n"),
    ),
    failing(noticesFailure(MISSING_HEADING), noticesFailure(NO_DOCS_ROW)),
  );
});

// The metadata slice families, on the pasted-format fixture: reference names.

test("the order of badly named entries", async () => {
  // Pre-order with byte order at each level, so the contents of B-dir come before B-dir.md.
  assert.deepEqual(
    await auditSlice(
      METADATA_SLICE,
      all(
        ref("A.md", "# A\n"),
        ref("B-dir/c_d.md", "# C\n"),
        ref("B-dir.md", "# B\n"),
        ref("a_b.md", "# A\n"),
      ),
    ),
    failing(
      fileFailure(`${PROTOTYPE_REFS}/A.md`),
      dirFailure(`${PROTOTYPE_REFS}/B-dir`),
      fileFailure(`${PROTOTYPE_REFS}/B-dir/c_d.md`),
      fileFailure(`${PROTOTYPE_REFS}/B-dir.md`),
      fileFailure(`${PROTOTYPE_REFS}/a_b.md`),
    ),
  );
});

test("a file name with a second dot", async () => {
  assert.deepEqual(
    await auditSlice(METADATA_SLICE, ref("a.test.md", "# A\n")),
    failing(fileFailure(`${PROTOTYPE_REFS}/a.test.md`)),
  );
});

test("an accented file name", async () => {
  assert.deepEqual(
    await auditSlice(METADATA_SLICE, ref(`caf${E_ACUTE}.md`, "# Cafe\n")),
    failing(fileFailure(`${PROTOTYPE_REFS}/caf${E_ACUTE}.md`)),
  );
});

test("a bad directory name with a bad file below", async () => {
  assert.deepEqual(
    await auditSlice(METADATA_SLICE, ref("Bad_Dir/Bad_File.md", "# Bad\n")),
    failing(
      dirFailure(`${PROTOTYPE_REFS}/Bad_Dir`),
      fileFailure(`${PROTOTYPE_REFS}/Bad_Dir/Bad_File.md`),
    ),
  );
});

// The content slice families, on the pasted-format fixture. The reference rules are not among them, so a dangling
// escape link or a misnamed directory reports nothing here; the independence and triage findings remain.

test("a source-detection file that is a directory", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      all(
        (r) => remove(r, `${TRIAGE_REFS}/source-detection.md`),
        (r) => mkdir(r, `${TRIAGE_REFS}/source-detection.md`),
      ),
    ),
    failing({
      rule: "triage-file-present",
      path: `${TRIAGE_REFS}/source-detection.md`,
      message: "required triage contract file is missing",
    }),
  );
});

test("an escape link from the skill file at depth 2", async () => {
  assert.deepEqual(
    await auditSlice(CONTENT_SLICE, xBody("See [e](../../evals/x.md).\n")),
    failing(outsideSkills("skills/x/SKILL.md", 11, "evals/")),
  );
});

test("an escape link from a reference file at depth 3 that stays inside", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      ref("a.md", "See [e](../../evals/x.md).\n"),
    ),
    failing(),
  );
});

test("an escape link from a reference file at depth 3 that leaves", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      ref("a.md", "See [e](../../../evals/x.md).\n"),
    ),
    failing(outsideSkills(A_MD, 1, "evals/")),
  );
});

test("an escape link to hooks from a nested file at depth 4 that stays inside", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      ref("deep/a.md", "See [e](../../../hooks/x.md).\n"),
    ),
    failing(),
  );
});

test("an escape link to hooks from a nested file at depth 4 that leaves", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      ref("deep/a.md", "See [e](../../../../hooks/x.md).\n"),
    ),
    failing(outsideSkills(`${PROTOTYPE_REFS}/deep/a.md`, 1, "hooks/")),
  );
});
