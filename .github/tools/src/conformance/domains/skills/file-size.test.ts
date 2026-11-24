import assert from "node:assert/strict";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { test } from "node:test";
import { fileURLToPath } from "node:url";
import {
  evalsCheck,
  withSkillsRepo,
  writeFile,
} from "../../testing/skills-repo.ts";
import type { Tracked } from "../repo/walk.ts";
import { BASELINE_DIR, readBaseline } from "./baseline.ts";
import {
  FAMILY,
  FILE_SIZE_RULE,
  FILE_SIZE_STALE_RULE,
  SKILL_FILE_BYTE_LIMIT,
  SKILL_MD_BYTE_LIMIT,
} from "./file-size.ts";
import { DOMAIN_FAMILIES, type ResolvedSkillsContext } from "./index.ts";
import { BASELINED_RULES } from "./standards.ts";

const ALLOWLIST = ".github/tools/src/conformance/skill-size-allowlist.json";
const GUIDE = "skills/forge/references/forging/forging-skills.md";

type Entry = { path: string; ceiling: number };

/** Runs the family on a fresh root holding `files` (path to byte count), with `tracked` as git's file list. */
async function audit(
  files: Record<string, number>,
  options: { tracked?: string[]; allowlist?: unknown } = {},
) {
  const root = mkdtempSync(path.join(tmpdir(), "agentics-filesize-"));
  try {
    for (const [rel, size] of Object.entries(files))
      writeFile(root, rel, "x".repeat(size));
    if (options.allowlist !== undefined)
      writeFile(root, ALLOWLIST, JSON.stringify(options.allowlist));
    const tracked: Tracked = {
      files: new Set(options.tracked ?? Object.keys(files)),
      dirs: new Set(),
    };
    const context: ResolvedSkillsContext = {
      families: [FAMILY],
      evalsCheck,
      tracked: () => tracked,
    };
    return await FAMILY.run(root, context);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
}

function overLimit(rel: string, bytes: number, limit = SKILL_FILE_BYTE_LIMIT) {
  return {
    rule: FILE_SIZE_RULE,
    path: rel,
    message: `${rel} is ${bytes} bytes; the limit is ${limit}. Split it along its sections; see ${GUIDE}`,
  };
}

test("limits are 24,000 bytes for SKILL.md and 30,000 bytes for other skills/ files", () => {
  assert.equal(SKILL_MD_BYTE_LIMIT, 24000);
  assert.equal(SKILL_FILE_BYTE_LIMIT, 30000);
});

test("the family has its own name and rules and runs after skill-shell-data", () => {
  assert.equal(FAMILY.name, "skill-file-size");
  assert.deepEqual(FAMILY.rules, ["skill-file-size", "skill-file-size-stale"]);
  const shellData = DOMAIN_FAMILIES.findIndex(
    (family) => family.name === "skill-shell-data",
  );
  assert.equal(DOMAIN_FAMILIES[shellData + 1], FAMILY);
});

test("the real skills tree is within its limits and the allowlist has no stale entry", async () => {
  const root = fileURLToPath(new URL("../../../../../../", import.meta.url));
  const context: ResolvedSkillsContext = { families: [FAMILY], evalsCheck };
  assert.deepEqual(await FAMILY.run(root, context), []);
});

test("an unlisted SKILL.md over the limit is reported", async () => {
  assert.deepEqual(await audit({ "skills/a/SKILL.md": 24001 }), [
    overLimit("skills/a/SKILL.md", 24001, SKILL_MD_BYTE_LIMIT),
  ]);
});

test("every unlisted file kind over the limit is reported, in path order", async () => {
  const files = [
    "skills/a/run.sh",
    "skills/a/references/tests/t.md",
    "skills/a/references/r.md",
  ];
  assert.deepEqual(
    await audit(Object.fromEntries(files.map((rel) => [rel, 30001]))),
    [
      overLimit("skills/a/references/r.md", 30001),
      overLimit("skills/a/references/tests/t.md", 30001),
      overLimit("skills/a/run.sh", 30001),
    ],
  );
});

test("a file of exactly the limit is not reported", async () => {
  assert.deepEqual(
    await audit({
      "skills/a/SKILL.md": 24000,
      "skills/a/references/r.md": 30000,
    }),
    [],
  );
});

test("an untracked file and a file outside skills/ are not sized", async () => {
  assert.deepEqual(
    await audit(
      { "skills/a/extra.md": 30001, "docs/big.md": 30001 },
      { tracked: ["docs/big.md"] },
    ),
    [],
  );
});

test("a tracked path missing from the working tree is not sized", async () => {
  assert.deepEqual(await audit({}, { tracked: ["skills/a/SKILL.md"] }), []);
});

test("a listed file at its ceiling passes; one byte more is reported", async () => {
  const allowlist: Entry[] = [{ path: "skills/a/SKILL.md", ceiling: 25000 }];
  assert.deepEqual(
    await audit({ "skills/a/SKILL.md": 25000 }, { allowlist }),
    [],
  );
  assert.deepEqual(await audit({ "skills/a/SKILL.md": 25001 }, { allowlist }), [
    {
      rule: FILE_SIZE_RULE,
      path: "skills/a/SKILL.md",
      message:
        "skills/a/SKILL.md is 25001 bytes; its allowlist ceiling is 25000 and the limit is 24000. A listed file may shrink but not grow",
    },
  ]);
});

test("an entry for an untracked path is stale", async () => {
  assert.deepEqual(
    await audit(
      { "skills/a/gone.md": 31000 },
      {
        tracked: [],
        allowlist: [{ path: "skills/a/gone.md", ceiling: 31000 }],
      },
    ),
    [
      {
        rule: FILE_SIZE_STALE_RULE,
        path: ALLOWLIST,
        message: "allowlist entry for skills/a/gone.md names no tracked file",
      },
    ],
  );
});

test("an entry for a file at or under the limit is stale", async () => {
  assert.deepEqual(
    await audit(
      { "skills/a/SKILL.md": 23000, "skills/b/SKILL.md": 24000 },
      {
        allowlist: [
          { path: "skills/a/SKILL.md", ceiling: 25000 },
          { path: "skills/b/SKILL.md", ceiling: 25000 },
        ],
      },
    ),
    [
      {
        rule: FILE_SIZE_STALE_RULE,
        path: ALLOWLIST,
        message:
          "allowlist entry for skills/a/SKILL.md is stale: the file is 23000 bytes, within the limit of 24000. Remove the entry",
      },
      {
        rule: FILE_SIZE_STALE_RULE,
        path: ALLOWLIST,
        message:
          "allowlist entry for skills/b/SKILL.md is stale: the file is 24000 bytes, within the limit of 24000. Remove the entry",
      },
    ],
  );
});

test("findings sort by path, then message", async () => {
  const result = await audit(
    { "skills/z/SKILL.md": 24001, "skills/a/SKILL.md": 23000 },
    {
      tracked: ["skills/z/SKILL.md", "skills/a/SKILL.md"],
      allowlist: [
        { path: "skills/a/SKILL.md", ceiling: 25000 },
        { path: "skills/b/gone.md", ceiling: 31000 },
      ],
    },
  );
  assert.deepEqual(
    result.map((failure) => [failure.path, failure.message.slice(0, 40)]),
    [
      [ALLOWLIST, "allowlist entry for skills/a/SKILL.md is"],
      [ALLOWLIST, "allowlist entry for skills/b/gone.md nam"],
      ["skills/z/SKILL.md", "skills/z/SKILL.md is 24001 bytes; the li"],
    ],
  );
});

for (const [label, allowlist, reason] of [
  ["is not an array", { path: "skills/a/SKILL.md" }, "expected a JSON array"],
  [
    "has an entry that is null",
    [null],
    "entry 0 needs a string path and an integer ceiling",
  ],
  [
    "has an entry without a path",
    [{ ceiling: 25000 }],
    "entry 0 needs a string path and an integer ceiling",
  ],
  [
    "has an entry with a non-integer ceiling",
    [{ path: "skills/a/SKILL.md", ceiling: 25000.5 }],
    "entry 0 needs a string path and an integer ceiling",
  ],
  [
    "has an entry outside skills/",
    [{ path: "docs/a.md", ceiling: 31000 }],
    "entry 0 names a path outside skills/",
  ],
  [
    "has a duplicate path",
    [
      { path: "skills/a/SKILL.md", ceiling: 25000 },
      { path: "skills/a/SKILL.md", ceiling: 26000 },
    ],
    "entry 1 duplicates an earlier entry for skills/a/SKILL.md",
  ],
  [
    "has a ceiling at the limit",
    [{ path: "skills/a/SKILL.md", ceiling: 24000 }],
    "entry 0 has ceiling 24000, at or below the limit of 24000",
  ],
  [
    "has a reference ceiling at the limit",
    [{ path: "skills/a/references/r.md", ceiling: 30000 }],
    "entry 0 has ceiling 30000, at or below the limit of 30000",
  ],
] as const) {
  test(`an allowlist that ${label} is a read error`, async () => {
    await assert.rejects(
      async () => audit({ "skills/a/SKILL.md": 25000 }, { allowlist }),
      { message: `skills: ${ALLOWLIST}: ${reason}` },
    );
  });
}

test("an allowlist that is not JSON is a read error", async () => {
  const root = mkdtempSync(path.join(tmpdir(), "agentics-filesize-"));
  try {
    writeFile(root, ALLOWLIST, "[");
    const context: ResolvedSkillsContext = {
      families: [FAMILY],
      evalsCheck,
      tracked: () => ({ files: new Set(), dirs: new Set() }),
    };
    await assert.rejects(async () => FAMILY.run(root, context), {
      message: `skills: ${ALLOWLIST}: invalid JSON`,
    });
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("the standards baseline rejects the file size rule", async () => {
  await withSkillsRepo((root) => {
    writeFile(
      root,
      `${BASELINE_DIR}/a.json`,
      JSON.stringify([
        { rule: FILE_SIZE_RULE, path: "skills/a/SKILL.md", key: "size" },
      ]),
    );
    assert.throws(() => readBaseline(root, BASELINED_RULES), {
      message: `skills: ${BASELINE_DIR}/a.json: entry 0 names unknown rule ${FILE_SIZE_RULE}`,
    });
  });
});
