import assert from "node:assert/strict";
import { mkdtempSync } from "node:fs";
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
  E_ACUTE,
  PROTOTYPE_REFS,
  put,
  ref,
} from "../../testing/skills-seeds.ts";
import {
  link,
  mkdir,
  withSkillsRepo,
  writeFile,
} from "../../testing/skills-repo.ts";
import {
  FAMILY,
  INDEPENDENCE_RULE,
  leavesSkills,
  NAME_TOKENS,
  overlapsAny,
  PATH_TOKENS,
  RELATIVE_ESCAPE_TOKENS,
  RELATIVE_PATH_TOKENS,
  scanText,
} from "./independence.ts";
import { run } from "./index.ts";
import { linkDirectory, removeTree } from "../../testing/host.ts";

const options: RunOptions = {
  short: false,
  fast: false,
  diagnostics: () => {},
  env: process.env,
};

const TAIL =
  ": a skill must not depend on an Agentics hook or on a file outside skills/";
const NO_FILES =
  "skills: no files under skills/ to audit for repository independence";

const LEAK = "skills/example/references/leak.md";

const audit = (root: string): Promise<AuditResult> =>
  run(root, options, { families: [FAMILY] });

/** The failures of a run that must not have raised an audit error. */
async function failuresOf(root: string): Promise<Failure[]> {
  const result = await audit(root);
  assert.equal(result.error, undefined);
  return result.failures;
}

/** The message for a token on a line. */
const names = (line: number, token: string): string =>
  `line ${line} names ${JSON.stringify(token)}${TAIL}`;

const brief = (found: Failure[]): string[][] =>
  found.map((one) => [one.rule, one.path, one.message]);

/** Runs `body` on a fresh empty directory and removes it afterwards, once an asynchronous body has settled. */
function withDir<T>(body: (dir: string) => T): T {
  const dir = mkdtempSync(path.join(tmpdir(), "agentics-independence-"));
  const cleanup = (): void => removeTree(dir);
  let result: T;
  try {
    result = body(dir);
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

const LS = String.fromCodePoint(0x2028);
const NBSP = String.fromCodePoint(0xa0);
const EACUTE = String.fromCodePoint(0xe9);
const BOM = String.fromCodePoint(0xfeff);
const PRIVATE_USE = String.fromCodePoint(0xe000);
const SUPPLEMENTARY = String.fromCodePoint(0x10000);

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

test("the family is named independence and reports skill-repo-independence", () => {
  assert.equal(FAMILY.name, "independence");
  assert.deepEqual(FAMILY.rules, ["skill-repo-independence"]);
  assert.equal(INDEPENDENCE_RULE, "skill-repo-independence");
});

test("the green fixture passes", async () => {
  await withSkillsRepo(async (root) => {
    assert.deepEqual(await audit(root), { failures: [] });
  });
});

// --- Agentics paths and names that a skill must not mention -----------------------------------------------------------

const FLAGGED: ReadonlyArray<readonly [string, string, number, string]> = [
  ["evals engine path", "Run node evals/src/cli/main.ts.", 1, "evals/"],
  ["hooks go path", "The core lives in hooks/go.", 1, "hooks/"],
  ["evals bank path", "Read `evals/skills/spec.md`.", 1, "evals/"],
  ["hook launcher", "Call run-hook.cmd chain state.", 1, "run-hook"],
  [
    "agentics hook binary",
    "The agentics-hook binary answers.",
    1,
    "agentics-hook",
  ],
  ["guardian name", "command-guardian blocks it.", 1, "command-guardian"],
  ["release guardian", "release-guardian blocks it.", 1, "release-guardian"],
  ["secrets guardian", "secrets-guardian blocks it.", 1, "secrets-guardian"],
  ["secrets scanner", "secrets-scanner blocks it.", 1, "secrets-scanner"],
  ["hook config", "See hooks.claude.json.", 1, "hooks.claude.json"],
  ["hooks directory", "The page under `hooks/` says so.", 1, "hooks/"],
  [
    "repository url",
    "[Agentics](https://github.com/muehz/agentics)",
    1,
    "muehz/agentics",
  ],
  ["notices file", "See THIRD_PARTY_NOTICES.md.", 1, "THIRD_PARTY_NOTICES"],
  ["manifest path", "Edit .claude-plugin/plugin.json.", 1, ".claude-plugin"],
  ["copilot manifest", "Edit .github/plugin/plugin.json.", 1, ".github/plugin"],
  [
    "plugin script",
    "Run ./.github/scripts/plugin.sh --validate.",
    1,
    "plugin.sh",
  ],
  ["charter", "See docs/CHARTER.md.", 1, "docs/CHARTER"],
  ["renames ledger", "Add a row to docs/RENAMES.md.", 1, "docs/RENAMES"],
  [
    "maintainer docs",
    "See docs/maintainers/model-ids.md.",
    1,
    "docs/maintainers/",
  ],
  ["surface docs", "See docs/surfaces/codex.md.", 1, "docs/surfaces/"],
  [
    "relative charter link",
    "[charter](../../docs/CHARTER.md)",
    1,
    "docs/CHARTER",
  ],
  [
    "relative surfaces link",
    "[codex](../docs/surfaces/codex.md)",
    1,
    "docs/surfaces/",
  ],
  ["token at start of line", "docs/CHARTER.md holds it.", 1, "docs/CHARTER"],
  ["dot-slash charter", "[charter](./docs/CHARTER.md)", 1, "docs/CHARTER"],
  [
    "dot-slash surfaces",
    "[codex](./docs/surfaces/codex.md)",
    1,
    "docs/surfaces/",
  ],
  ["dot-slash renames", "Add a row to ./docs/RENAMES.md.", 1, "docs/RENAMES"],
  [
    "dot-slash maintainers",
    "See ./docs/maintainers/model-ids.md.",
    1,
    "docs/maintainers/",
  ],
  [
    "dot-slash evals bank",
    "Read `./evals/skills/spec.md`.",
    1,
    "evals/skills/",
  ],
  [
    "token on a later line",
    "First line.\nSecond line names docs/RENAMES.md.",
    2,
    "docs/RENAMES",
  ],
];

test("the flagged table holds 29 bodies", () => {
  assert.equal(FLAGGED.length, 29);
});

for (const [name, body, line, token] of FLAGGED) {
  test(`flags ${name}: ${JSON.stringify(token)} on line ${line}`, async () => {
    await withSkillsRepo(async (root) => {
      writeFile(root, LEAK, body);
      const found = await failuresOf(root);
      assert.ok(
        found.some(
          (one) =>
            one.rule === INDEPENDENCE_RULE &&
            one.path === LEAK &&
            one.message === names(line, token),
        ),
        JSON.stringify(found),
      );
      assert.ok(found.every((one) => one.rule === INDEPENDENCE_RULE));
      assert.ok(found.every((one) => one.path === LEAK));
    });
  });
}

// --- relative links to evals/ and hooks/ ---------------------------------------------------------------------------

const ESCAPING: ReadonlyArray<
  readonly [string, string, string, number, string]
> = [
  [
    "evals bank from a reference",
    "skills/forge/references/x.md",
    "[bank](../../../evals/skills/forge.md)",
    1,
    "evals/",
  ],
  [
    "hooks page from a reference",
    "skills/forge/references/x.md",
    "(../../../hooks/README.md)",
    1,
    "hooks/",
  ],
  [
    "hooks go from a reference",
    "skills/forge/references/x.md",
    "(../../../hooks/go/internal/x.go)",
    1,
    "hooks/",
  ],
  [
    "evals from a SKILL.md",
    "skills/forge/SKILL.md",
    "See (../../evals/README.md).",
    1,
    "evals/",
  ],
  [
    "climb past the root",
    "skills/forge/references/x.md",
    "(../../../../evals/x.md)",
    1,
    "evals/",
  ],
  [
    "relative link on a later line",
    "skills/forge/references/x.md",
    "First.\n[hooks](../../../hooks/README.md)",
    2,
    "hooks/",
  ],
];

for (const [name, rel, body, line, token] of ESCAPING) {
  test(`flags a relative link that leaves skills: ${name}, and the token excludes the climb`, async () => {
    await withSkillsRepo(async (root) => {
      writeFile(root, rel, body);
      const found = await failuresOf(root);
      assert.deepEqual(brief(found), [
        [INDEPENDENCE_RULE, rel, names(line, token)],
      ]);
      assert.ok(found.every((one) => !one.message.includes('"../')));
    });
  });
}

const INSIDE: ReadonlyArray<readonly [string, string, string]> = [
  [
    "sibling evals folder",
    "skills/forge/references/foo.md",
    "(../evals/README.md#anchor)",
  ],
  [
    "climb of depth minus one",
    "skills/forge/references/x.md",
    "(../../evals/x.md)",
  ],
  [
    "deep file short climb",
    "skills/forge/references/a/b.md",
    "[x](../../evals/x.md) and (../../../hooks/y.md)",
  ],
  ["SKILL.md one level up", "skills/forge/SKILL.md", "(../hooks/README.md)"],
  [
    "plain boundary unaffected",
    "skills/forge/references/x.md",
    "Load references/evals/README.md and ./hooks/x.sh.",
  ],
  [
    "dot-slash references evals",
    "skills/forge/references/x.md",
    "Read ./references/evals/x.md first.",
  ],
  [
    "dot-slash local evals",
    "skills/forge/references/x.md",
    "Read ./evals/x.md and ./hooks/x.md.",
  ],
  [
    "scripts hooks folder",
    "skills/forge/references/x.md",
    "Run scripts/hooks/x.sh or ./scripts/hooks/x.sh.",
  ],
  [
    "dot-slash local hooks go",
    "skills/forge/references/x.md",
    "Read ./hooks/go/internal/x.go.",
  ],
];

test("the inside table holds 9 negatives", () => {
  assert.equal(INSIDE.length, 9);
});

for (const [name, rel, body] of INSIDE) {
  test(`allows a relative evals or hooks link inside skills: ${name}`, async () => {
    await withSkillsRepo(async (root) => {
      writeFile(root, rel, body);
      assert.deepEqual(await failuresOf(root), []);
    });
  });
}

// --- files the walk skips and the audit errors -----------------------------------------------------------------------

test("a file with a NUL byte is skipped", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(
      root,
      "skills/example/assets/blob.bin",
      "docs/CHARTER\x00docs/RENAMES",
    );
    assert.deepEqual(await failuresOf(root), []);
  });
});

test("a NUL byte anywhere in the file skips it, the first byte included", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(root, LEAK, "\x00docs/CHARTER");
    writeFile(root, "skills/example/references/two.md", "docs/CHARTER\x00");
    assert.deepEqual(await failuresOf(root), []);
  });
});

test("scanning only NUL files is the no-files audit error", () => {
  withDir((root) => {
    writeFile(root, "skills/example/blob.bin", "docs/CHARTER\x00");
    return audit(root).then((result) => {
      assert.deepEqual(result, { failures: [], error: NO_FILES });
    });
  });
});

test("a symlinked file under skills is skipped", (t) => {
  withDir((root) => {
    writeFile(root, "outside/leak.md", "See docs/CHARTER.md.");
    if (
      !link(
        root,
        "skills/example/leak.md",
        path.join(root, "outside", "leak.md"),
        "file",
      )
    )
      return skipForLink(t);
    writeFile(root, "skills/example/SKILL.md", "# Example\n");
    return audit(root).then((result) => {
      assert.deepEqual(result, { failures: [] });
    });
  });
});

test("skills holding only the README is the no-files audit error", async () => {
  await withDir(async (root) => {
    writeFile(root, "skills/README.md", "# Skills\n");
    assert.deepEqual(await audit(root), { failures: [], error: NO_FILES });
  });
});

test("an empty skills directory is the no-files audit error", async () => {
  await withDir(async (root) => {
    mkdir(root, "skills");
    assert.deepEqual(await audit(root), { failures: [], error: NO_FILES });
  });
});

test("a skills directory that is a link is the no-files audit error, even when the target holds a leak", async (t) => {
  await withDir(async (root) => {
    writeFile(root, "outside/example/SKILL.md", "See docs/CHARTER.md.");
    if (
      linkDirectory(path.join(root, "outside"), path.join(root, "skills")) ===
      undefined
    )
      return skipForLink(t);
    assert.deepEqual(await audit(root), { failures: [], error: NO_FILES });
  });
});

test("a missing skills directory is the missing file error, not the no-files text", async () => {
  await withDir(async (root) => {
    const result = await audit(root);
    assert.deepEqual(result.failures, []);
    assert.equal(result.error, "skills: does not exist");
    assert.ok(!(result.error ?? "").includes("no files under skills/"));
  });
});

// --- messages, ordering and the README exemption ------------------------------------------------------------------

test("a finding is reported once with the file path, the line and the quoted token", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(
      root,
      LEAK,
      "First line.\nSecond line.\nSee [charter](../../docs/CHARTER.md).\n",
    );
    assert.deepEqual(await failuresOf(root), [
      {
        rule: "skill-repo-independence",
        path: LEAK,
        message:
          'line 3 names "docs/CHARTER": a skill must not depend on an Agentics hook or on a file outside skills/',
      },
    ]);
  });
});

test("the token on line 3 of a reference is reported at that path", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(
      root,
      "skills/forge/references/x.md",
      "a\nb\nSee docs/CHARTER.\n",
    );
    assert.deepEqual(brief(await failuresOf(root)), [
      [
        "skill-repo-independence",
        "skills/forge/references/x.md",
        'line 3 names "docs/CHARTER": a skill must not depend on an Agentics hook or on a file outside skills/',
      ],
    ]);
  });
});

test("findings on lines 2 and 10 of one file are ordered as strings, line 10 first, and both are kept", async () => {
  await withSkillsRepo(async (root) => {
    const lines = Array.from({ length: 10 }, () => "filler");
    lines[1] = "See docs/CHARTER.";
    lines[9] = "See docs/CHARTER.";
    writeFile(root, LEAK, lines.join("\n"));
    assert.deepEqual(
      (await failuresOf(root)).map((one) => one.message),
      [names(10, "docs/CHARTER"), names(2, "docs/CHARTER")],
    );
  });
});

test("findings are ordered by path first and by message second", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(root, "skills/zeta/x.md", "docs/CHARTER");
    writeFile(root, "skills/alpha/x.md", "docs/CHARTER");
    assert.deepEqual(
      (await failuresOf(root)).map((one) => one.path),
      ["skills/alpha/x.md", "skills/zeta/x.md"],
    );
  });
  await withSkillsRepo(async (root) => {
    writeFile(root, "skills/a/y.md", "x\nSee docs/CHARTER.");
    writeFile(
      root,
      "skills/b/x.md",
      "x\nx\nx\nx\nx\nx\nx\nx\nx\nSee docs/CHARTER.",
    );
    assert.deepEqual(brief(await failuresOf(root)), [
      ["skill-repo-independence", "skills/a/y.md", names(2, "docs/CHARTER")],
      ["skill-repo-independence", "skills/b/x.md", names(10, "docs/CHARTER")],
    ]);
  });
});

test("paths follow the default sort order, so a supplementary-plane name comes before a private-use name", async () => {
  await withDir(async (root) => {
    writeFile(root, `skills/${SUPPLEMENTARY}/x.md`, "docs/CHARTER");
    writeFile(root, `skills/${PRIVATE_USE}/x.md`, "docs/CHARTER");
    assert.deepEqual(
      (await failuresOf(root)).map((one) => one.path),
      [`skills/${SUPPLEMENTARY}/x.md`, `skills/${PRIVATE_USE}/x.md`],
    );
  });
});

test("two identical tokens on one line give two identical findings", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(root, LEAK, "docs/CHARTER docs/CHARTER");
    const found = await failuresOf(root);
    assert.equal(found.length, 2);
    assert.deepEqual(found[0], found[1]);
    assert.equal(found[0]?.message, names(1, "docs/CHARTER"));
  });
});

test("the top-level skills README is never scanned and a nested README is", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(
      root,
      "skills/README.md",
      "See docs/CHARTER.md and docs/RENAMES.md.",
    );
    assert.deepEqual(await failuresOf(root), []);
    writeFile(root, "skills/example/README.md", "See docs/CHARTER.md.");
    assert.deepEqual(brief(await failuresOf(root)), [
      [
        "skill-repo-independence",
        "skills/example/README.md",
        names(1, "docs/CHARTER"),
      ],
    ]);
  });
});

test("the exemption is the exact path skills/README.md, not the name README.md", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(root, "skills/readme.md", "See docs/CHARTER.md.");
    writeFile(root, "skills/README.md.bak", "See docs/CHARTER.md.");
    assert.deepEqual(
      (await failuresOf(root)).map((one) => one.path),
      ["skills/README.md.bak", "skills/readme.md"],
    );
  });
});

// --- relative links that leave skills --------------------------------------------------------------------------------

const LEAVING: ReadonlyArray<
  readonly [string, string, string, number, string]
> = [
  [
    "readme from a SKILL.md",
    "skills/forge/SKILL.md",
    "See [readme](../../README.md).",
    1,
    "../../README.md",
  ],
  [
    "readme from a reference",
    "skills/forge/references/x.md",
    "See [readme](../../../README.md).",
    1,
    "../../../README.md",
  ],
  [
    "contributing from a SKILL.md",
    "skills/forge/SKILL.md",
    "Read (../../CONTRIBUTING.md#rules).",
    1,
    "../../CONTRIBUTING.md",
  ],
  [
    "contributing from a reference",
    "skills/forge/references/x.md",
    "Read (../../../CONTRIBUTING.md).",
    1,
    "../../../CONTRIBUTING.md",
  ],
  [
    "codex manifest from a SKILL.md",
    "skills/forge/SKILL.md",
    "Edit `../../.codex-plugin/plugin.json`.",
    1,
    "../../.codex-plugin/plugin.json",
  ],
  [
    "codex manifest from a reference",
    "skills/forge/references/x.md",
    "Edit (../../../.codex-plugin/plugin.json).",
    1,
    "../../../.codex-plugin/plugin.json",
  ],
  [
    "climb past the root",
    "skills/forge/references/x.md",
    "(../../../../LICENSE)",
    1,
    "../../../../LICENSE",
  ],
  [
    "repository root directory",
    "skills/forge/SKILL.md",
    "Open [the repo](../../).",
    1,
    "../../",
  ],
  [
    "dot-slash prefix",
    "skills/forge/SKILL.md",
    "(./../../README.md)",
    1,
    "./../../README.md",
  ],
  [
    "climb after a folder name",
    "skills/forge/references/x.md",
    "(refs/../../../../README.md)",
    1,
    "refs/../../../../README.md",
  ],
  [
    "relative link on a later line",
    "skills/forge/references/x.md",
    "First.\n[readme](../../../README.md)",
    2,
    "../../../README.md",
  ],
  [
    "unlisted folder at the repo root",
    "skills/forge/SKILL.md",
    "(../../scripts/release.sh)",
    1,
    "../../scripts/release.sh",
  ],
];

for (const [name, rel, body, line, token] of LEAVING) {
  test(`flags any relative link that leaves skills: ${name}`, async () => {
    await withSkillsRepo(async (root) => {
      writeFile(root, rel, body);
      assert.deepEqual(brief(await failuresOf(root)), [
        [INDEPENDENCE_RULE, rel, names(line, token)],
      ]);
    });
  });
}

const STAYING: ReadonlyArray<readonly [string, string, string]> = [
  ["sibling reference", "skills/forge/references/x.md", "(../references/a.md)"],
  [
    "sibling skill",
    "skills/forge/references/x.md",
    "(../../other-skill/SKILL.md)",
  ],
  [
    "sibling skill from SKILL",
    "skills/forge/SKILL.md",
    "(../other-skill/SKILL.md)",
  ],
  ["dot-slash then one up", "skills/forge/references/x.md", "(./../a.md)"],
  ["down then up", "skills/forge/references/x.md", "(a/../b.md)"],
  [
    "url with parent segments",
    "skills/forge/SKILL.md",
    "(https://example.com/a/../../../b)",
  ],
  [
    "plugin root variable",
    "skills/forge/SKILL.md",
    "Run ${CLAUDE_PLUGIN_ROOT}/../../x.sh.",
  ],
  [
    "ellipsis and version span",
    "skills/forge/SKILL.md",
    "Waiting... see 1..5 and a/.../b.",
  ],
  [
    "parent only inside a dir",
    "skills/forge/references/a/b.md",
    "(../../x.md)",
  ],
];

test("the staying table holds its 9 relative links that stay inside skills/", () => {
  assert.equal(STAYING.length, 9);
});

for (const [name, rel, body] of STAYING) {
  test(`allows a relative link that stays inside skills: ${name}`, async () => {
    await withSkillsRepo(async (root) => {
      writeFile(root, rel, body);
      assert.deepEqual(await failuresOf(root), []);
    });
  });
}

const GENERIC: ReadonlyArray<readonly [string, string]> = [
  ["relative evals link", "See [guide](../evals/README.md#anchor)."],
  ["bare relative evals", "(../evals/README.md#anchor)"],
  ["nested evals folder", "Load references/evals/README.md first."],
  ["generic go test", "Run `go test ./internal/evals/...`."],
  ["consumer workflows", "Edit .github/workflows/ci.yml."],
  ["consumer adr", "Record it in docs/adr/0001.md."],
  ["git hooks word", "NEVER skip hooks with --no-verify."],
  ["peer trust flag", "Pass --dangerously-bypass-hook-trust."],
  ["git hooks directory", "Look in .git/hooks/ for the script."],
  ["flow helper", "Run `agx flow log gate branch ok` and agx flow status."],
  ["plugin root variable", "Read ${CLAUDE_PLUGIN_ROOT}/skills/forge/SKILL.md."],
  ["path with no token", "See tooling/notes.md."],
  ["consumer docs folder", "Write docs/guide/setup.md."],
  ["word before evals dir", "The skill-evals/ folder is local."],
];

// No hook binary path is exempt any more: skills call the flow helper as `agx flow`, so every spelling of the old
// binary path names a hook.
test("rejects every hook binary path for every platform and suffix the binaries ship for", async () => {
  for (const platform of [
    "<os>-<arch>",
    "darwin-arm64",
    "darwin-amd64",
    "linux-amd64",
    "linux-arm64",
    "windows-amd64",
    "windows-arm64",
  ]) {
    for (const suffix of ["", ".exe"]) {
      const binary = `hooks/bin/agentics-hook-${platform}${suffix}`;
      for (const body of [
        binary,
        `Call \`${binary}\`.`,
        `Run "$root/${binary}".`,
        `[helper](../../${binary})`,
      ]) {
        await withSkillsRepo(async (root) => {
          writeFile(root, "skills/example/SKILL.md", body);
          assert.ok(
            (await failuresOf(root)).some(
              (one) => one.message === names(1, "agentics-hook"),
            ),
            body,
          );
        });
      }
    }
  }
});

test("rejects near misses and mixed flow mentions, naming the token each finding reports", async () => {
  const cases: readonly (readonly [string, string])[] = [
    ["agentics-hook", "agentics-hook"],
    ["hooks/run-hook.cmd", "run-hook"],
    ["hooks/hooks.claude.json", "hooks.claude.json"],
    ["hooks/bin/other", "hooks/"],
    ["hooks/bin/agx-linux-amd64", "hooks/"],
    ["hooks/bin/agentics-hook-", "agentics-hook"],
    ["hooks/bin/agentics-hook-freebsd-amd64", "agentics-hook"],
    ["otherhooks/bin/agentics-hook-linux-amd64", "agentics-hook"],
    ["agx flow status hooks/bin/agentics-hook-linux-amd64", "agentics-hook"],
    ["agx flow status run-hook", "run-hook"],
    ["agx flow status hooks.codex.json", "hooks.codex.json"],
    ["agx flow status command-guardian", "command-guardian"],
    ["agx flow status release-guardian", "release-guardian"],
    ["agx flow status secrets-guardian", "secrets-guardian"],
    ["agx flow status secrets-scanner", "secrets-scanner"],
    ["agx flow status hooks/bin/other", "hooks/"],
  ];
  for (const [body, token] of cases) {
    await withSkillsRepo(async (root) => {
      writeFile(root, "skills/example/SKILL.md", body);
      assert.ok(
        (await failuresOf(root)).some((one) => one.message === names(1, token)),
        body,
      );
    });
  }
});

test("a hook binary path names the hook whatever character surrounds it", () => {
  const binary = "hooks/bin/agentics-hook-linux-amd64";
  const emoji = String.fromCodePoint(0x1f600);
  for (const around of [EACUTE, emoji, LS, NBSP]) {
    for (const text of [around + binary, binary + around]) {
      assert.ok(
        scanText("skills/example/SKILL.md", text).some((one) =>
          one.message.includes('names "agentics-hook"'),
        ),
        JSON.stringify(text),
      );
    }
  }
});

test("the generic table holds 14 negatives", () => {
  assert.equal(GENERIC.length, 14);
});

for (const [name, body] of GENERIC) {
  test(`allows relative links and generic paths: ${name}`, async () => {
    await withSkillsRepo(async (root) => {
      writeFile(root, "skills/example/references/ok.md", body);
      assert.deepEqual(await failuresOf(root), []);
    });
  });
}

test("an escaping link is reported once when a known name follows the climb", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(
      root,
      "skills/forge/references/x.md",
      "(../../../evals/skills/forge.md) and (../../../docs/CHARTER.md)",
    );
    assert.deepEqual(
      (await failuresOf(root)).map((one) => one.message).sort(),
      [names(1, "docs/CHARTER"), names(1, "evals/")].sort(),
    );
  });
});

test("a link that reaches the root from depth 2 fires and the same link from depth 3 does not", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(root, "skills/x/SKILL.md", "See ../../evals/x.");
    writeFile(root, "skills/x/references/a.md", "See ../../evals/x.");
    assert.deepEqual(brief(await failuresOf(root)), [
      [INDEPENDENCE_RULE, "skills/x/SKILL.md", names(1, "evals/")],
    ]);
  });
});

test("a climb of four from depth 3 reports evals/ and not the climb", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(root, "skills/x/references/a.md", "See ../../../../evals/x.");
    assert.deepEqual(brief(await failuresOf(root)), [
      [INDEPENDENCE_RULE, "skills/x/references/a.md", names(1, "evals/")],
    ]);
  });
});

test("a relative path is judged by its running depth, and a ... segment counts as a name", async () => {
  const at = async (rel: string, body: string): Promise<string[]> =>
    withSkillsRepo(async (root) => {
      writeFile(root, rel, body);
      return (await failuresOf(root)).map((one) => one.message);
    });
  assert.deepEqual(await at("skills/x/SKILL.md", "(../../a/b)"), [
    names(1, "../../a/b"),
  ]);
  assert.deepEqual(await at("skills/x/SKILL.md", "(a/../../../x)"), [
    names(1, "a/../../../x"),
  ]);
  assert.deepEqual(await at("skills/x/SKILL.md", "(.../../../x)"), []);
  assert.deepEqual(await at("skills/x/SKILL.md", "(x/../../README.md)"), []);
  assert.deepEqual(await at("skills/x/references/a.md", "(a/../../x)"), []);
  assert.deepEqual(await at("skills/x/references/a.md", "(a/../../../../x)"), [
    names(1, "a/../../../../x"),
  ]);
});

test("a token behind a slash, a word character, a dot or a hyphen is not a finding", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(
      root,
      LEAK,
      [
        "x/evals/ and x/hooks/ and x/docs/CHARTER",
        "xevals/ and xhooks/ and xdocs/CHARTER",
        "x.evals/ and x.hooks/",
        "x-evals/ and x-hooks/ and x-docs/CHARTER",
        "https://example.com/docs/CHARTER and https://example.com/evals/",
      ].join("\n"),
    );
    assert.deepEqual(await failuresOf(root), []);
  });
});

test("a colon is a boundary for the path tokens, though it is not one for the relative-link pattern", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(root, LEAK, "see:evals/x and path=hooks/y");
    assert.deepEqual(
      (await failuresOf(root)).map((one) => one.message),
      [names(1, "evals/"), names(1, "hooks/")],
    );
  });
});

test("upper-case or mixed-case tokens are not findings", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(
      root,
      LEAK,
      "See EVALS/ and Hooks/ and DOCS/CHARTER and TOOLS/GO.",
    );
    assert.deepEqual(await failuresOf(root), []);
  });
});

test("line numbers count LF only: a CRLF file counts each pair once and a lone CR does not count", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(root, LEAK, "a\r\nb\r\nSee docs/CHARTER.\r\n");
    assert.deepEqual(
      (await failuresOf(root)).map((one) => one.message),
      [names(3, "docs/CHARTER")],
    );
    writeFile(root, LEAK, "a\rb\rSee docs/CHARTER.\r");
    assert.deepEqual(
      (await failuresOf(root)).map((one) => one.message),
      [names(1, "docs/CHARTER")],
    );
  });
});

test("a byte order mark acts as a boundary and does not shift the line", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(root, LEAK, `${BOM}evals/x\na${BOM}docs/CHARTER`);
    assert.deepEqual(
      (await failuresOf(root)).map((one) => one.message),
      [names(1, "evals/"), names(2, "docs/CHARTER")],
    );
  });
});

test("a non-ASCII character before a token is a boundary, and a line separator or NBSP is too", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(
      root,
      LEAK,
      `${EACUTE}evals/x\nq${LS}hooks/y\nq${NBSP}docs/CHARTER`,
    );
    assert.deepEqual(
      (await failuresOf(root)).map((one) => one.message),
      [names(1, "evals/"), names(2, "hooks/"), names(3, "docs/CHARTER")],
    );
  });
});

// --- the helpers -----------------------------------------------------------------------------------------------------

test("leavesSkills counts the directories of the file with / splitting, whatever the host separator", () => {
  assert.equal(leavesSkills("skills/x/SKILL.md", "../../a"), true);
  assert.equal(leavesSkills("skills/x/SKILL.md", "../a"), false);
  assert.equal(leavesSkills("skills/x/references/a.md", "../../a"), false);
  assert.equal(leavesSkills("skills/x/references/a.md", "../../../a"), true);
  assert.equal(leavesSkills("skills/x/references/a.md", "./../a"), false);
  assert.equal(
    leavesSkills("skills/x/references/a.md", "a/../../../../b"),
    true,
  );
  assert.equal(leavesSkills("skills/x/SKILL.md", ".../../../x"), false);
  assert.equal(leavesSkills("skills/x/SKILL.md", "../.."), true);
  assert.equal(leavesSkills("skills/x/SKILL.md", ""), false);
  assert.equal(leavesSkills("skills", "../x"), true);
});

test("overlapsAny shares a position, and spans that only touch do not overlap", () => {
  assert.equal(overlapsAny([[2, 5]], 4, 8), true);
  assert.equal(overlapsAny([[2, 5]], 0, 3), true);
  assert.equal(overlapsAny([[2, 5]], 5, 8), false);
  assert.equal(overlapsAny([[2, 5]], 0, 2), false);
  assert.equal(overlapsAny([[2, 5]], 3, 4), true);
  assert.equal(overlapsAny([], 0, 9), false);
});

test("scanText returns path and name tokens, then escapes, then other relative links, before the final sort", () => {
  const found = scanText(
    "skills/x/SKILL.md",
    "../../README.md docs/CHARTER plugin.sh ../../evals/y",
  );
  assert.deepEqual(
    found.map((one) => one.message),
    [
      names(1, "docs/CHARTER"),
      names(1, "plugin.sh"),
      names(1, "evals/"),
      names(1, "../../README.md"),
    ],
  );
});

// --- the settings a skill may name -------------------------------------------------------------------------------------

// A skill that describes a policy has to be able to say which setting switches it off, so these names stay out of the
// rule while the hook that enforces the policy stays in it.
const SETTINGS: readonly string[] = [
  "Set `policy.draft_prs` to `false` to open the pull request ready for review.",
  "The policy.draft_prs setting decides whether a pull request opens as a draft.",
  "The repository switches a policy off in `.agentics/hooks.json`.",
  "Read .agentics/hooks.json before assuming that a policy is on.",
  "Export `AGX_RELEASE_RECORD` with the path of the release record before publishing.",
  "A release is refused while AGX_RELEASE_RECORD is unset.",
  "Set `policy.draft_prs` in `.agentics/hooks.json`, or point `AGX_RELEASE_RECORD` at the record.",
];

test("a skill may name a policy key, the settings file and the release-record variable", () => {
  for (const rel of ["skills/gh/SKILL.md", "skills/gh/references/guide.md"]) {
    for (const line of SETTINGS) {
      assert.deepEqual(scanText(rel, line), [], `${rel}: ${line}`);
    }
    assert.deepEqual(scanText(rel, SETTINGS.join("\n")), [], rel);
  }
});

test("the guardian and binary names stay reported", () => {
  const rel = "skills/gh/SKILL.md";
  const lines: readonly (readonly [string, string])[] = [
    ["The command-guardian refuses a force push.", "command-guardian"],
    ["The release-guardian refuses an unrecorded release.", "release-guardian"],
    ["The secrets-guardian refuses a key in a file.", "secrets-guardian"],
    ["Ask the `agentics-hook` binary for the policy.", "agentics-hook"],
    ["The launcher is `run-hook`.", "run-hook"],
    ["The hooks are registered in `hooks.claude.json`.", "hooks.claude.json"],
  ];
  for (const [line, token] of lines) {
    assert.deepEqual(
      brief(scanText(rel, line)),
      [[INDEPENDENCE_RULE, rel, names(1, token)]],
      line,
    );
  }
});

// --- the patterns, probed for the spans they match ---------------------------------------------------------------

/** The first participating capture group of a match, as the position and the text a finding reports. */
function firstGroup(match: RegExpMatchArray): [number, string] {
  for (let i = 1; i < match.length; i += 1) {
    const span = match.indices?.[i];
    const text = match[i];
    if (span !== undefined && text !== undefined) return [span[0], text];
  }
  return [match.index ?? -1, match[0]];
}

function spansOf(re: RegExp, input: string): Array<[number, string]> {
  return [...input.matchAll(re)].map(firstGroup);
}

function escapesOf(input: string): Array<[number, string, string]> {
  return [...input.matchAll(RELATIVE_ESCAPE_TOKENS)].map((match) => [
    match.indices?.[2]?.[0] ?? -1,
    match[2] ?? "",
    match[1] ?? "",
  ]);
}

function wholeOf(re: RegExp, input: string): Array<[number, string]> {
  return [...input.matchAll(re)].map((match) => [match.index, match[0]]);
}

function checkTable(
  table: readonly Vector[],
  actual: (input: string) => unknown[],
): void {
  for (const [raw, expected] of table) {
    assert.deepEqual(actual(expand(raw)), expected, JSON.stringify(raw));
  }
}

export const pathTokensVectors: readonly Vector[] = [
  ["tooling/notes.md", []],
  [".docs/CHARTER", []],
  ["../docs/RENAMES", [[3, "docs/RENAMES"]]],
  [".../docs/CHARTER", [[4, "docs/CHARTER"]]],
  ["(docs/CHARTER)", [[1, "docs/CHARTER"]]],
  ["docs/CHARTER.md", [[0, "docs/CHARTER"]]],
  ["./docs/CHARTER", [[2, "docs/CHARTER"]]],
  ["../../docs/CHARTER.md", [[6, "docs/CHARTER"]]],
  ["docs/charter", []],
  ["docs/CHARTERS", [[0, "docs/CHARTER"]]],
  ["adocs/CHARTER", []],
  ["a/docs/CHARTER", []],
  ["-docs/CHARTER", []],
  ["_docs/CHARTER", []],
  ["docs/surfaces/codex.md", [[0, "docs/surfaces/"]]],
  ["docs/surfaces", []],
  ["docs/RENAMES.md", [[0, "docs/RENAMES"]]],
  ["docs/maintainers/x.md", [[0, "docs/maintainers/"]]],
  ["docs/maintainers", []],
  [
    "x docs/RENAMES docs/surfaces/",
    [
      [2, "docs/RENAMES"],
      [15, "docs/surfaces/"],
    ],
  ],
  ["evals/x", [[0, "evals/"]]],
  ["EVALS/x", []],
  ["Hooks/x", []],
  ["hooks/", [[0, "hooks/"]]],
  ["hooks", []],
  ["(evals/x)", [[1, "evals/"]]],
  ["a evals/", [[2, "evals/"]]],
  ["references/evals/", []],
  ["x-evals/", []],
  ["x.evals/", []],
  ["x_evals/", []],
  ["./evals/x", []],
  ["../evals/x", []],
  ["xevals/", []],
  ["`hooks/`", [[1, "hooks/"]]],
  ["evals/skills/x", [[0, "evals/"]]],
  ["./evals/skills/spec.md", [[2, "evals/skills/"]]],
  ["`./evals/skills/spec.md`", [[3, "evals/skills/"]]],
  ["a./evals/skills/x", []],
  ["a ./evals/skills/x", [[4, "evals/skills/"]]],
  ["./hooks/x", []],
  ["../evals/skills/x", []],
  ["x/./evals/skills/x", []],
  [
    "hooks/ hooks/",
    [
      [0, "hooks/"],
      [7, "hooks/"],
    ],
  ],
  ["evals/evals/", [[0, "evals/"]]],
  [
    "hooks/\nhooks/",
    [
      [0, "hooks/"],
      [7, "hooks/"],
    ],
  ],
  [
    "evals/ evals/skills/",
    [
      [0, "evals/"],
      [7, "evals/"],
    ],
  ],
  [
    "docs/CHARTER\ndocs/RENAMES",
    [
      [0, "docs/CHARTER"],
      [13, "docs/RENAMES"],
    ],
  ],
  ["x\revals/", [[2, "evals/"]]],
  ["x\r\nevals/", [[3, "evals/"]]],
  ["\nevals/", [[1, "evals/"]]],
  ["\revals/", [[1, "evals/"]]],
  ["x{LS}evals/", [[2, "evals/"]]],
  ["x{PS}hooks/", [[2, "hooks/"]]],
  ["x{NBSP}evals/", [[2, "evals/"]]],
  ["{EACUTE}evals/", [[1, "evals/"]]],
  ["{BOM}evals/", [[1, "evals/"]]],
  ["{NEL}hooks/", [[1, "hooks/"]]],
  ["{EMOJI}evals/", [[2, "evals/"]]],
  ["a{LS}docs/CHARTER", [[2, "docs/CHARTER"]]],
  ["{NBSP}docs/CHARTER", [[1, "docs/CHARTER"]]],
  ["x{NBSP}./evals/skills/", [[4, "evals/skills/"]]],
  ["\r\n../docs/CHARTER", [[5, "docs/CHARTER"]]],
  ["{LS}../docs/RENAMES", [[4, "docs/RENAMES"]]],
  ["..{NBSP}docs/RENAMES", [[3, "docs/RENAMES"]]],
];

export const relativeEscapeTokensVectors: readonly Vector[] = [
  ["../evals/", [[3, "evals/", "../"]]],
  ["../../evals/x", [[6, "evals/", "../../"]]],
  ["../hooks/README.md", [[3, "hooks/", "../"]]],
  ["a/../evals/x", []],
  ["x ../hooks/", [[5, "hooks/", "../"]]],
  ["./../evals/", []],
  ["(../../../evals/skills/x)", [[10, "evals/", "../../../"]]],
  ["\n../evals/", [[4, "evals/", "../"]]],
  ["\r../evals/", [[4, "evals/", "../"]]],
  ["x\r\n../evals/", [[6, "evals/", "../"]]],
  ["a../evals/", []],
  ["-../evals/", []],
  ["_../evals/", []],
  ["/../evals/", []],
  [".../evals/", []],
  ["../../evals", []],
  [
    "../../hooks/x ../evals/y",
    [
      [6, "hooks/", "../../"],
      [17, "evals/", "../"],
    ],
  ],
  ["../x/../evals/", []],
  ["{EACUTE}../evals/", [[4, "evals/", "../"]]],
  ["{LS}../evals/", [[4, "evals/", "../"]]],
  ["{NBSP}../hooks/", [[4, "hooks/", "../"]]],
  ["{BOM}../hooks/", [[4, "hooks/", "../"]]],
  ["../../../../evals/x", [[12, "evals/", "../../../../"]]],
  [
    "x ../../../../hooks/ a ../evals/",
    [
      [14, "hooks/", "../../../../"],
      [26, "evals/", "../"],
    ],
  ],
  ["EVALS ../evals/", [[9, "evals/", "../"]]],
  ["../EVALS/", []],
  ["..evals/", []],
  [".././evals/", []],
  ["../../ evals/", []],
  ["a ../../../evals/skills/", [[11, "evals/", "../../../"]]],
  ["{EMOJI}../../hooks/", [[8, "hooks/", "../../"]]],
];

export const relativePathTokensVectors: readonly Vector[] = [
  ["a/b", [[0, "a/b"]]],
  ["a/", [[0, "a/"]]],
  ["/a", []],
  ["a", []],
  ["x ../../README.md", [[2, "../../README.md"]]],
  ["https://x.y/a/b", []],
  ["(https://example.com/a/../../../b)", []],
  ["(a/b)", [[1, "a/b"]]],
  ["`a/b/`", [[1, "a/b/"]]],
  ["a/b/c.md#anchor", [[0, "a/b/c.md"]]],
  [
    "a/b?x=1/c",
    [
      [0, "a/b"],
      [6, "1/c"],
    ],
  ],
  ["${CLAUDE_PLUGIN_ROOT}/skills/x", []],
  ["~/x/y", [[0, "~/x/y"]]],
  ["x ~/a/b", [[2, "~/a/b"]]],
  ["a//b", [[0, "a/"]]],
  ["a/b/..", [[0, "a/b/.."]]],
  ["./a", [[0, "./a"]]],
  [".../x", [[0, ".../x"]]],
  ["@scope/pkg", [[0, "@scope/pkg"]]],
  ["a%20b/c", [[0, "a%20b/c"]]],
  ["a+b/c", [[0, "a+b/c"]]],
  ["{EACUTE}/a", []],
  ["{EACUTE}a/b", [[1, "a/b"]]],
  ["a{EACUTE}/b", []],
  ["\ra/b", [[1, "a/b"]]],
  ["\r\na/b", [[2, "a/b"]]],
  ["x{LS}a/b", [[2, "a/b"]]],
  ["x{NBSP}a/b", [[2, "a/b"]]],
  ["x{NEL}a/b", [[2, "a/b"]]],
  ["x{BOM}a/b", [[2, "a/b"]]],
  [
    "a/b a/b",
    [
      [0, "a/b"],
      [4, "a/b"],
    ],
  ],
  ["a./b", [[0, "a./b"]]],
  [".../.../", [[0, ".../.../"]]],
  ["../", [[0, "../"]]],
  ["a/../../b", [[0, "a/../../b"]]],
  ["line1\nA/b", [[6, "A/b"]]],
  ["-a/b", [[0, "-a/b"]]],
  [" -a/b", [[1, "-a/b"]]],
  ["a-b/c", [[0, "a-b/c"]]],
  ["x:a/b", []],
  ["x:/a/b", []],
  ["x/a/b", [[0, "x/a/b"]]],
  ["x.a/b", [[0, "x.a/b"]]],
  ["x~a/b", [[0, "x~a/b"]]],
  ["x_a/b", [[0, "x_a/b"]]],
  ["(../../README.md)", [[1, "../../README.md"]]],
  ["(./../../README.md)", [[1, "./../../README.md"]]],
  ["(refs/../../../../README.md)", [[1, "refs/../../../../README.md"]]],
  ["/etc/passwd", []],
  ["/a/b", []],
  [
    "a b/c d/e",
    [
      [2, "b/c"],
      [6, "d/e"],
    ],
  ],
  ["Waiting... see 1..5 and a/.../b.", [[24, "a/.../b."]]],
  ["{EMOJI}a/b", [[2, "a/b"]]],
  ["a/{EMOJI}/b", [[0, "a/"]]],
  [
    "[x](../a.md)#y/z",
    [
      [4, "../a.md"],
      [13, "y/z"],
    ],
  ],
  ["path.to/file.txt", [[0, "path.to/file.txt"]]],
  ["a/bc/d", [[0, "a/bc/d"]]],
  [
    "a/b|c/d",
    [
      [0, "a/b"],
      [4, "c/d"],
    ],
  ],
  [
    "a/b,c/d",
    [
      [0, "a/b"],
      [4, "c/d"],
    ],
  ],
  [
    "a/b;c/d",
    [
      [0, "a/b"],
      [4, "c/d"],
    ],
  ],
];

export const nameTokensVectors: readonly Vector[] = [
  [".claude-plugin", [[0, ".claude-plugin"]]],
  ["x.claude-plugin", [[1, ".claude-plugin"]]],
  ["claude-plugin", []],
  [".github/plugin", [[0, ".github/plugin"]]],
  [".github/plugins", [[0, ".github/plugin"]]],
  ["plugin.sh", [[0, "plugin.sh"]]],
  ["xplugin.sh", [[1, "plugin.sh"]]],
  ["plugin_sh", []],
  ["PLUGIN.SH", []],
  ["THIRD_PARTY_NOTICES.md", [[0, "THIRD_PARTY_NOTICES"]]],
  ["third_party_notices", []],
  ["muehz/agentics", [[0, "muehz/agentics"]]],
  ["xmuehz/agentics/y", [[1, "muehz/agentics"]]],
  ["muehz/agenticss", [[0, "muehz/agentics"]]],
  ["run-hook.cmd", [[0, "run-hook"]]],
  ["run-hooks", [[0, "run-hook"]]],
  ["agentics-hook", [[0, "agentics-hook"]]],
  ["agentics-hooks", [[0, "agentics-hook"]]],
  ["hooks.claude.json", [[0, "hooks.claude.json"]]],
  ["hooks.Claude.json", []],
  ["hooks.1.json", []],
  ["hooks..json", []],
  ["hooks.a.json", [[0, "hooks.a.json"]]],
  ["hooks.codex.jsonx", [[0, "hooks.codex.json"]]],
  ["hooks.a.b.json", []],
  ["hooks.{EACUTE}.json", []],
  ["hooks.{KELVIN}.json", []],
  ["command-guardian", [[0, "command-guardian"]]],
  ["release-guardian", [[0, "release-guardian"]]],
  ["secrets-guardian", [[0, "secrets-guardian"]]],
  ["secrets-scanner", [[0, "secrets-scanner"]]],
  ["secrets-scannerx", [[0, "secrets-scanner"]]],
  ["Command-Guardian", []],
  ["{EACUTE}plugin.sh", [[1, "plugin.sh"]]],
  ["plugin.sh{EACUTE}", [[0, "plugin.sh"]]],
  [
    "a plugin.sh plugin.sh",
    [
      [2, "plugin.sh"],
      [12, "plugin.sh"],
    ],
  ],
  [
    "run-hook\nrun-hook",
    [
      [0, "run-hook"],
      [9, "run-hook"],
    ],
  ],
  [
    "plugin.sh\r\nplugin.sh",
    [
      [0, "plugin.sh"],
      [11, "plugin.sh"],
    ],
  ],
];

test("pathTokens matches path spans after a boundary, including the carriage return, line separator and NBSP cases", () => {
  assert.equal(pathTokensVectors.length, 65);
  checkTable(pathTokensVectors, (input) => spansOf(PATH_TOKENS, input));
});

test("relativeEscapeTokens matches escaping links to evals/ and hooks/ with their climbs", () => {
  assert.equal(relativeEscapeTokensVectors.length, 31);
  checkTable(relativeEscapeTokensVectors, escapesOf);
});

test("relativePathTokens matches relative link spans, including URL and variable-rooted inputs", () => {
  assert.equal(relativePathTokensVectors.length, 60);
  checkTable(relativePathTokensVectors, (input) =>
    spansOf(RELATIVE_PATH_TOKENS, input),
  );
});

test("nameTokens matches Agentics file and hook names case-sensitively and without a boundary", () => {
  assert.equal(nameTokensVectors.length, 38);
  checkTable(nameTokensVectors, (input) => wholeOf(NAME_TOKENS, input));
});

// --- the whole-domain scenarios ------------------------------------------------------------------------------------

/** The slice whose families the slice-only scenarios below run: the independence family and its neighbours. */
const CONTENT_SLICE = sliceNamed("content");

/** The independence findings on `rel`, one per `[line, token]`, in the order given. */
const independenceOf = (
  rel: string,
  ...found: [line: number, token: string][]
): AuditResult => ({
  failures: found.map(([line, token]) => ({
    rule: INDEPENDENCE_RULE,
    path: rel,
    message: names(line, token),
  })),
});

const PROTOTYPE_A = `${PROTOTYPE_REFS}/a.md`;

test("skill-repo-independence", async () => {
  assert.deepEqual(
    await auditEveryFamily(breaker("skill-repo-independence")),
    independenceOf(`${PROTOTYPE_REFS}/layout.md`, [3, "hooks/"]),
  );
});

test("one file with a token on line 10 and one on line 2", async () => {
  // The messages sort as text, so line 10 comes before line 2.
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      ref(
        "a.md",
        "# A\nSee hooks/second.md\nl3\nl4\nl5\nl6\nl7\nl8\nl9\nSee hooks/tenth.md\n",
      ),
    ),
    independenceOf(PROTOTYPE_A, [10, "hooks/"], [2, "hooks/"]),
  );
});

test("a named path suppressing the relative path that holds it", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      ref("a.md", "See ../../../docs/CHARTER.md here.\n"),
    ),
    independenceOf(PROTOTYPE_A, [1, "docs/CHARTER"]),
  );
});

test("a relative path leaving skills under no named token", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      ref("a.md", "See ../../../other/x.md here.\n"),
    ),
    independenceOf(PROTOTYPE_A, [1, "../../../other/x.md"]),
  );
});

test("a token after a non-ASCII character", async () => {
  assert.deepEqual(
    await auditSlice(CONTENT_SLICE, ref("a.md", `caf${E_ACUTE}hooks/x.md\n`)),
    independenceOf(PROTOTYPE_A, [1, "hooks/"]),
  );
});

test("a nested README naming a hook path", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      put("skills/prototype/README.md", "See hooks/x.md\n"),
    ),
    independenceOf("skills/prototype/README.md", [1, "hooks/"]),
  );
});

test("a CRLF file with two hook paths", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      ref("a.md", crlf("# A\nSee hooks/a.md\nSee hooks/b.md\n")),
    ),
    independenceOf(PROTOTYPE_A, [2, "hooks/"], [3, "hooks/"]),
  );
});

test("an Agentics name with no boundary", async () => {
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      ref("a.md", "Run xplugin.sh and run-hook now.\n"),
    ),
    independenceOf(PROTOTYPE_A, [1, "plugin.sh"], [1, "run-hook"]),
  );
});

test("a dot-slash evals skills link", async () => {
  // Only the evals/skills/ path is reported, not ./evals/other.md.
  assert.deepEqual(
    await auditSlice(
      CONTENT_SLICE,
      ref("a.md", "See ./evals/skills/x.md and ./evals/other.md.\n"),
    ),
    independenceOf(PROTOTYPE_A, [1, "evals/skills/"]),
  );
});

test("flags .antigravity-plugin references under skill-repo-independence", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(root, LEAK, "Edit .antigravity-plugin/plugin.json.\n");
    assert.deepEqual(brief(await failuresOf(root)), [
      [INDEPENDENCE_RULE, LEAK, names(1, ".antigravity-plugin")],
    ]);
  });
  await withSkillsRepo(async (root) => {
    writeFile(
      root,
      "skills/example/SKILL.md",
      "First line.\nSee `.antigravity-plugin/plugin.json`.\n",
    );
    assert.deepEqual(brief(await failuresOf(root)), [
      [
        INDEPENDENCE_RULE,
        "skills/example/SKILL.md",
        names(2, ".antigravity-plugin"),
      ],
    ]);
  });
  assert.deepEqual(wholeOf(NAME_TOKENS, ".antigravity-plugin"), [
    [0, ".antigravity-plugin"],
  ]);
  assert.deepEqual(wholeOf(NAME_TOKENS, "x.antigravity-plugin"), [
    [1, ".antigravity-plugin"],
  ]);
  assert.deepEqual(wholeOf(NAME_TOKENS, "antigravity-plugin"), []);
  assert.deepEqual(wholeOf(NAME_TOKENS, ".ANTIGRAVITY-PLUGIN"), []);
});
