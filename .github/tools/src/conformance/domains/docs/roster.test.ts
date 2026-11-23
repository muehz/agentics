import assert from "node:assert/strict";
import { mkdirSync, rmSync } from "node:fs";
import path from "node:path";
import { test } from "node:test";
import { type Failure } from "../../check.ts";
import { LEDGER, STAMP, staleRows } from "../../testing/docs-seeds.ts";
import { ENV_MAX_AGE_DAYS } from "./index.ts";
import {
  auditDocs,
  readFixture,
  withDocsRepo,
  writeFile,
} from "../../testing/docs-repo.ts";
const failuresOf = async (root: string): Promise<Failure[]> => {
  const result = await auditDocs(root);
  assert.equal(result.error, undefined);
  return result.failures;
};

test("the Copilot manifest's skills key matches only in exact case", async () => {
  for (const key of ["Skills", "SKILLS", "sKiLLs"]) {
    await withDocsRepo(async (root) => {
      writeFile(
        root,
        ".github/plugin/plugin.json",
        JSON.stringify({ [key]: ["skills/alpha", "skills/beta"] }),
      );
      assert.deepEqual(
        await auditDocs(root),
        {
          failures: [],
          error: "docs: cannot derive roster counts from an empty roster",
        },
        key,
      );
    });
  }
});

test("a skills value that is not a list says so in the audit error", async () => {
  await withDocsRepo(async (root) => {
    writeFile(root, ".github/plugin/plugin.json", '{"skills": "all"}');
    assert.match(
      (await auditDocs(root)).error ?? "",
      /docs: parsing Copilot manifest: skills is not an array/,
    );
  });
});

test("a wrong-case key neither hides nor replaces an invalid skills value", async () => {
  await withDocsRepo(async (root) => {
    writeFile(
      root,
      ".github/plugin/plugin.json",
      '{"skills":"bad","Skills":["a"]}',
    );
    assert.match((await auditDocs(root)).error ?? "", /skills is not an array/);
  });
});

test("a duplicate skills key resolves to its last value and a case variant is ignored", async () => {
  await withDocsRepo(async (root) => {
    writeFile(
      root,
      ".github/plugin/plugin.json",
      '{"skills":["a"],"skills":["a","b","c"],"Skills":["a","b"]}',
    );
    const failures = await failuresOf(root);
    assert.equal(failures.length, 2);
    assert.ok(
      failures.every((f) =>
        f.message.endsWith("Copilot-registered skills is 3"),
      ),
    );
  });
});

test("the fixture's claims match the counts derived from it", async () => {
  await withDocsRepo(async (root) => {
    assert.deepEqual(await failuresOf(root), []);
  });
});

const CLAIM_SITES = [
  {
    file: "docs/ARCHITECTURE.md",
    label: "canonical skill roster",
    from: "canonical 4-skill roster",
    to: "canonical 9-skill roster",
    count: "shipped skills",
    actual: 4,
  },
  {
    file: "docs/ARCHITECTURE.md",
    label: "canonical skill tree",
    from: "canonical 4-skill tree",
    to: "canonical 9-skill tree",
    count: "shipped skills",
    actual: 4,
  },
  {
    file: "docs/ARCHITECTURE.md",
    label: "Copilot skill coverage",
    from: "Copilot CLI ships all 2 skills",
    to: "Copilot CLI ships all 9 skills",
    count: "Copilot-registered skills",
    actual: 2,
  },
  {
    file: "docs/surfaces/claude-code.md",
    label: "Claude manifest registration",
    from: "registers all 4 skills",
    to: "registers all 9 skills",
    count: "shipped skills",
    actual: 4,
  },
  {
    file: "docs/surfaces/copilot-cli.md",
    label: "Copilot manifest registration",
    from: "registers all 2 Agentics skills",
    to: "registers all 9 Agentics skills",
    count: "Copilot-registered skills",
    actual: 2,
  },
] as const;

test("each of the five claims fires roster-count-drift on its own file when its number is wrong", async () => {
  for (const site of CLAIM_SITES) {
    await withDocsRepo(async (root) => {
      writeFile(
        root,
        site.file,
        readFixture(root, site.file).replace(site.from, site.to),
      );
      assert.deepEqual(
        await failuresOf(root),
        [
          {
            rule: "roster-count-drift",
            path: site.file,
            message: `${site.label} says 9; ${site.count} is ${site.actual}`,
          },
        ],
        site.label,
      );
    });
  }
});

test("each of the five claims fires roster-claim-site-present when its sentence is gone", async () => {
  for (const site of CLAIM_SITES) {
    await withDocsRepo(async (root) => {
      writeFile(
        root,
        site.file,
        readFixture(root, site.file).replace(site.from, "nothing to see here"),
      );
      assert.deepEqual(
        await failuresOf(root),
        [
          {
            rule: "roster-claim-site-present",
            path: site.file,
            message: `${site.label} no longer matches its current-state claim`,
          },
        ],
        site.label,
      );
    });
  }
});

test("every occurrence of a claim is checked, not just the first", async () => {
  await withDocsRepo(async (root) => {
    writeFile(
      root,
      "docs/ARCHITECTURE.md",
      "The canonical 4-skill roster is fine.\nThe canonical 5-skill roster is not.\nThe canonical 4-skill tree.\nCopilot CLI ships all 2 skills.\n",
    );
    assert.deepEqual(
      (await failuresOf(root)).map((f) => f.message),
      ["canonical skill roster says 5; shipped skills is 4"],
    );
  });
});

test("a claim a formatter wrapped across lines is still read", async () => {
  await withDocsRepo(async (root) => {
    writeFile(
      root,
      "docs/ARCHITECTURE.md",
      "The canonical 4-skill\nroster.\nThe canonical\n  4-skill\ttree.\nCopilot CLI ships\nall 2 skills.\n",
    );
    assert.deepEqual(await failuresOf(root), []);
  });
});

test("a claim-site file that is missing fails once per claim that lives in it", async () => {
  await withDocsRepo(async (root) => {
    rmSync(path.join(root, "docs", "ARCHITECTURE.md"));
    const missing = (await failuresOf(root)).filter(
      (f) => f.path === "docs/ARCHITECTURE.md",
    );
    assert.deepEqual(
      missing.map((f) => f.message),
      [
        "canonical skill roster cannot be audited because the file is missing",
        "canonical skill tree cannot be audited because the file is missing",
        "Copilot skill coverage cannot be audited because the file is missing",
      ],
    );
    assert.ok(missing.every((f) => f.rule === "roster-claim-site-present"));
  });
});

test("a claim-site file that cannot be read is an audit error naming it", async () => {
  await withDocsRepo(async (root) => {
    rmSync(path.join(root, "docs", "ARCHITECTURE.md"));
    mkdirSync(path.join(root, "docs", "ARCHITECTURE.md"));
    assert.deepEqual(await auditDocs(root), {
      failures: [],
      error: "docs: docs/ARCHITECTURE.md: is not a regular file",
    });
  });
});

test("skills count directories only, so a stray file does not move the roster", async () => {
  await withDocsRepo(async (root) => {
    writeFile(root, "skills/README.md", "# Skills\n");
    assert.deepEqual(await failuresOf(root), []);
    writeFile(root, "skills/delta/SKILL.md", "# delta\n");
    assert.equal((await failuresOf(root)).length, 3);
  });
});

test("counts that cannot be derived are audit errors, never passes", async () => {
  const cases: Array<{
    name: string;
    prepare: (root: string) => void;
    error: RegExp;
  }> = [
    {
      name: "no skills directory",
      prepare: (root) => rmSync(path.join(root, "skills"), { recursive: true }),
      error: /^docs: skills: does not exist$/,
    },
    {
      name: "an empty roster",
      prepare: (root) => {
        rmSync(path.join(root, "skills"), { recursive: true });
        writeFile(root, "skills/README.md", "# Skills\n");
      },
      error: /cannot derive roster counts from an empty roster/,
    },
    {
      name: "no Copilot manifest",
      prepare: (root) =>
        rmSync(path.join(root, ".github", "plugin", "plugin.json")),
      error: /^docs: \.github\/plugin\/plugin\.json: does not exist$/,
    },
    {
      name: "a Copilot manifest that is not JSON",
      prepare: (root) =>
        writeFile(root, ".github/plugin/plugin.json", "{\n\u001b[31mnot json"),
      error: /^docs: \.github\/plugin\/plugin\.json: is not valid JSON$/,
    },
    {
      name: "a Copilot manifest with no skills",
      prepare: (root) => writeFile(root, ".github/plugin/plugin.json", "{}"),
      error: /cannot derive roster counts from an empty roster/,
    },
    {
      name: "a Copilot manifest whose skills are not a list",
      prepare: (root) =>
        writeFile(root, ".github/plugin/plugin.json", '{"skills": "all"}'),
      error: /^docs: parsing Copilot manifest: skills is not an array$/,
    },
    {
      name: "a Copilot manifest whose skills are not strings",
      prepare: (root) =>
        writeFile(root, ".github/plugin/plugin.json", '{"skills": [1]}'),
      error:
        /^docs: parsing Copilot manifest: skills holds a value that is not a string$/,
    },
    {
      name: "a Copilot manifest with one skill that is not a string",
      prepare: (root) =>
        writeFile(
          root,
          ".github/plugin/plugin.json",
          '{"skills": ["skills/alpha", 1]}',
        ),
      error:
        /^docs: parsing Copilot manifest: skills holds a value that is not a string$/,
    },
    {
      name: "a Copilot manifest that is a number",
      prepare: (root) => writeFile(root, ".github/plugin/plugin.json", "5"),
      error:
        /^docs: parsing Copilot manifest: the manifest is not a JSON object$/,
    },
    {
      name: "a Copilot manifest that is JSON null",
      prepare: (root) => writeFile(root, ".github/plugin/plugin.json", "null"),
      error: /^docs: cannot derive roster counts from an empty roster$/,
    },
    {
      name: "a Copilot manifest that is a list",
      prepare: (root) => writeFile(root, ".github/plugin/plugin.json", "[]"),
      error:
        /^docs: parsing Copilot manifest: the manifest is not a JSON object$/,
    },
  ];
  for (const { name, prepare, error } of cases) {
    await withDocsRepo(async (root) => {
      prepare(root);
      const result = await auditDocs(root);
      assert.match(result.error ?? "", error, name);
      assert.deepEqual(result.failures, [], name);
    });
  }
});

test("a claimed number too large to be an integer is an audit error", async () => {
  await withDocsRepo(async (root) => {
    writeFile(
      root,
      "docs/ARCHITECTURE.md",
      "The canonical 99999999999999999999-skill roster.\n",
    );
    const result = await auditDocs(root);
    assert.match(
      result.error ?? "",
      /parsing roster claim "canonical skill roster"/,
    );
  });
});

test("the largest 64-bit integer is a drifted claim and one more is out of range", async () => {
  await withDocsRepo(async (root) => {
    const fixture = readFixture(root, "docs/ARCHITECTURE.md");
    const claim = (n: string): void =>
      writeFile(
        root,
        "docs/ARCHITECTURE.md",
        fixture.replace(
          "canonical 4-skill roster",
          `canonical ${n}-skill roster`,
        ),
      );

    claim("9223372036854775807");
    assert.deepEqual(await auditDocs(root), {
      failures: [
        {
          rule: "roster-count-drift",
          path: "docs/ARCHITECTURE.md",
          message:
            "canonical skill roster says 9223372036854775807; shipped skills is 4",
        },
      ],
    });

    claim("9223372036854775808");
    assert.deepEqual(await auditDocs(root), {
      failures: [],
      error:
        'docs: parsing roster claim "canonical skill roster": "9223372036854775808" is out of range',
    });
  });
});

test("a leading zero is the same number", async () => {
  await withDocsRepo(async (root) => {
    const text = readFixture(root, "docs/ARCHITECTURE.md").replace(
      "canonical 4-skill roster",
      "canonical 004-skill roster",
    );
    writeFile(root, "docs/ARCHITECTURE.md", text);
    assert.deepEqual(await failuresOf(root), []);
  });
});

test("a null skills element counts as a registration, not a shape error", async () => {
  await withDocsRepo(async (root) => {
    writeFile(
      root,
      ".github/plugin/plugin.json",
      '{"skills": ["skills/alpha", null]}',
    );
    assert.deepEqual(await failuresOf(root), []);
  });
});

const ROSTER_DRIFT: Failure = {
  rule: "roster-count-drift",
  path: "docs/ARCHITECTURE.md",
  message: "canonical skill roster says 5; shipped skills is 4",
};

/** Rewrites the fixture's architecture doc with the first `from` replaced by `to`. */
function editArchitecture(root: string, from: string, to: string): void {
  writeFile(
    root,
    "docs/ARCHITECTURE.md",
    readFixture(root, "docs/ARCHITECTURE.md").replace(from, to),
  );
}

test("a drifted roster count", async () => {
  await withDocsRepo(async (root) => {
    editArchitecture(
      root,
      "canonical 4-skill roster",
      "canonical 5-skill roster",
    );
    assert.deepEqual(await auditDocs(root), { failures: [ROSTER_DRIFT] });
  });
});

test("a roster claim reworded", async () => {
  await withDocsRepo(async (root) => {
    writeFile(
      root,
      "docs/surfaces/claude-code.md",
      `The manifest lists every skill.${STAMP}`,
    );
    assert.deepEqual(await auditDocs(root), {
      failures: [
        {
          rule: "roster-claim-site-present",
          path: "docs/surfaces/claude-code.md",
          message:
            "Claude manifest registration no longer matches its current-state claim",
        },
      ],
    });
  });
});

test("a roster claim split by a no-break space still matches its claim", async () => {
  await withDocsRepo(async (root) => {
    // A no-break space is whitespace to the claim matcher, as a wrapped line is.
    editArchitecture(
      root,
      "canonical 4-skill tree",
      "canonical\u{a0}4-skill tree",
    );
    assert.deepEqual(await auditDocs(root), { failures: [] });
  });
});

test("a roster drift together with a stale row", async () => {
  await withDocsRepo(async (root) => {
    editArchitecture(
      root,
      "canonical 4-skill roster",
      "canonical 5-skill roster",
    );
    // A roster drift is not structural to the ledger, so the age check still runs.
    const stale = [
      "claude-code.md: last reviewed 2026-08-09 (35 days ago; budget 1)",
      "codex.md: last reviewed 2026-08-01 (43 days ago; budget 1)",
      "copilot-cli.md: last reviewed 2026-08-09 (35 days ago; budget 1)",
      "cursor.md: last reviewed 2026-08-09 (35 days ago; budget 1)",
    ];
    assert.deepEqual(
      await auditDocs(root, { env: { [ENV_MAX_AGE_DAYS]: "1" } }),
      {
        failures: [
          ROSTER_DRIFT,
          ...stale.map((message, i) => ({
            rule: staleRows[i],
            path: LEDGER,
            message,
          })),
        ],
      },
    );
  });
});
