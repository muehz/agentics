import assert from "node:assert/strict";
import path from "node:path";
import { test } from "node:test";
import { fileURLToPath } from "node:url";
import { link, withSkillsRepo, writeFile } from "../../testing/skills-repo.ts";
import { BASELINE_DIR, readBaseline } from "./baseline.ts";
import { FAMILY, SHELL_DATA_RULE, shellDataRules } from "./shell-data.ts";
import { BASELINED_RULES } from "./standards.ts";
import { withOutside } from "../../testing/host.ts";

const FIXTURE = "skills/example/references/commands.md";
const REVIEWED = ".github/tools/src/conformance/skill-shell-reviewed.json";
const STALE = "skill-shell-data-stale";

test("the real skills tree has no shell-template findings or stale reviewed entries", () => {
  const root = fileURLToPath(new URL("../../../../../../", import.meta.url));
  assert.deepEqual(shellDataRules(root), []);
});

test("a fence accepted by the shared parser still has a label with an extra carriage return", async () => {
  const markdown = ["```bash", "git show <ref>", "```", ""].join("\r\r\n");
  assert.deepEqual(await audit(markdown), [finding(2, "ref")]);
});
const NAMES = [
  "branch",
  "base-branch",
  "head-branch",
  "layer-branch",
  "recovery-branch",
  "base-ref",
  "ref",
  "remote",
  "push-url",
  "tag",
  "artifact-tag",
  "name",
  "file",
  "path",
  "staged-path",
  "other-path",
  "title",
  "subject",
  "body",
  "message",
];

function audit(text: string) {
  return withSkillsRepo((root) => {
    writeFile(root, FIXTURE, text);
    return shellDataRules(root);
  });
}

function finding(line: number, name: string) {
  return {
    rule: SHELL_DATA_RULE,
    path: FIXTURE,
    message: `line ${line} writes <${name}> into command text: take it from a producer in the same command, or add a reviewed entry with a reason`,
  };
}

for (const [label, lines] of [
  ["Bash", (name: string) => ["```bash", `git show '<${name}>'`, "```"]],
  [
    "PowerShell",
    (name: string) => ["```powershell", `git show '<${name}>'`, "```"],
  ],
  ["unlabeled", (name: string) => ["```", `git show '<${name}>'`, "```"]],
  ["inline", (name: string) => ["", `Run \`git show '<${name}>'\`.`, ""]],
] as const) {
  test(`flags every data name in ${label} command text`, async () => {
    const result = await audit(NAMES.flatMap(lines).join("\n"));
    const expected = NAMES.map((name, index) => finding(index * 3 + 2, name));
    assert.deepEqual(
      result,
      expected.sort((a, b) => (a.message < b.message ? -1 : 1)),
    );
  });
}

test("uses the first fence info word and reports each occurrence", async () => {
  assert.deepEqual(
    await audit("~~~pwsh title=Example\nWrite-Output '<body> <body>'\n~~~\n"),
    [finding(2, "body"), finding(2, "body")],
  );
});

test("other shell fence labels emit the published shell data finding", async () => {
  assert.deepEqual(FAMILY.rules, [
    "skill-shell-data",
    "skill-shell-data-stale",
  ]);
  for (const label of ["sh", "shell", "zsh", "console"]) {
    assert.deepEqual(await audit(`\`\`\`${label}\ngit show <ref>\n\`\`\``), [
      { ...finding(2, "ref"), rule: "skill-shell-data" },
    ]);
  }
});

test("checks inline gh and agx commands, including more than one span on a line", async () => {
  assert.deepEqual(
    await audit("Run `gh pr view <branch>` or `agx show <path>`.\n"),
    [finding(1, "branch"), finding(1, "path")],
  );
});

test("reports each multiline inline placeholder on its physical line", async () => {
  assert.deepEqual(
    await audit(
      [
        "Use `git show <ref>",
        "  -- <path>` or ``gh pr view",
        "<branch>`` and `agx",
        "flow status --folder <file>`.",
      ].join("\n"),
    ),
    [
      finding(1, "ref"),
      finding(2, "path"),
      finding(3, "branch"),
      finding(4, "file"),
    ],
  );
});

test("never pairs inline delimiters across blank lines or fenced blocks", async () => {
  for (const separator of [[""], ["```text", "<branch>", "```"]]) {
    assert.deepEqual(
      await audit(["Use `git show", ...separator, "<ref>`."].join("\n")),
      [],
    );
  }
});

test("never pairs inline delimiters across thematic breaks or Setext underlines", async () => {
  for (const separator of ["***", "- - -", "___", "---", "==="]) {
    assert.deepEqual(
      await audit(`Use \`git show\n${separator}\n<ref>\`.`),
      [],
      separator,
    );
  }
});

test("mutation triage preserves fenced command boundaries in nested and indented examples", async () => {
  const fixtures = [
    {
      lines: [
        "12. ```bash",
        "\tgit show <ref>",
        "    ```",
        "",
        "> ```bash",
        "> git show <ref>",
        "```bash",
        "git show <ref>",
        "```",
      ],
      findings: [finding(2, "ref"), finding(6, "ref"), finding(8, "ref")],
    },
    {
      lines: [
        "> - ```bash",
        ">   git show <ref>",
        ">   ```",
        "> `git show <ref>`",
      ],
      findings: [finding(2, "ref"), finding(4, "ref")],
    },
    {
      lines: [
        "\t```bash",
        "\tgit show <ref>",
        "\t```",
        "Use `gh view <branch>`.",
      ],
      findings: [finding(4, "branch")],
    },
    {
      lines: [
        "   ```bash",
        "   git show <ref>",
        "   ```",
        "Use `gh view <branch>`.",
      ],
      findings: [finding(2, "ref"), finding(4, "branch")],
    },
  ];
  for (const { lines, findings } of fixtures) {
    assert.deepEqual(await audit(lines.join("\n")), findings, lines.join("\n"));
  }
});

test("mutation triage distinguishes inline padding, prose, and thematic breaks", async () => {
  assert.deepEqual(await audit("Text ` git show <ref> `"), [finding(1, "ref")]);
  assert.deepEqual(await audit("Use `git show\nprefix ***\n<ref>`."), [
    finding(3, "ref"),
  ]);
  for (const separator of ["*\t*\t*", "_ _ _", " --- "]) {
    assert.deepEqual(
      await audit(`Use \`git show\n${separator}\n<ref>\`.`),
      [],
      separator,
    );
  }
});

test("mutation triage leaves later commands visible after a comment containing a redirect", async () => {
  assert.deepEqual(
    await audit(
      [
        "```bash",
        "cat # <<'NOTE'",
        "git show <ref>",
        "NOTE",
        "git show <branch>",
        "```",
      ].join("\n"),
    ),
    [finding(3, "ref"), finding(5, "branch")],
  );
});

test("checks a fence opened on its enclosing list marker", async () => {
  assert.deepEqual(
    await audit(["1. ```bash", "   git show <ref>", "   ```"].join("\n")),
    [finding(2, "ref")],
  );
});

test("checks a nested list fence and resumes prose after list dedent", async () => {
  assert.deepEqual(
    await audit(
      [
        "- Item",
        "  1. Nested item",
        "     ```json",
        "     <path>",
        "",
        "Use `git show <ref>`.",
      ].join("\n"),
    ),
    [finding(6, "ref")],
  );
});

test("a dedented fence opens after an implicitly closed list fence", async () => {
  assert.deepEqual(
    await audit(
      [
        "- Item",
        "  ```text",
        "  <path>",
        "```bash",
        "git show <ref>",
        "```",
      ].join("\n"),
    ),
    [finding(5, "ref")],
  );
});

test("does not carry a fence beyond its blockquote", async () => {
  assert.deepEqual(
    await audit(
      ["> ```text", "> <path>", "Use `gh pr view <branch>`."].join("\n"),
    ),
    [finding(3, "branch")],
  );
});

test("an indented marker outside a list cannot start a shell fence", async () => {
  assert.deepEqual(
    await audit(
      [
        "    ```bash",
        "    git show <ref>",
        "    ```",
        "Use `git show <path>`.",
      ].join("\n"),
    ),
    [finding(4, "path")],
  );
});

test("ignores prose, non-command spans, unlisted names, variables and non-shell fences", async () => {
  const result = await audit(
    [
      "Choose <branch> or show `a <branch>` token.",
      "```bash",
      "git show <n>",
      'git show "$base"',
      "```",
      "```json",
      "`git show <branch>`",
      "```",
      "",
    ].join("\n"),
  );
  assert.deepEqual(result, []);
});

test("skips quoted heredoc bodies and checks expandable bodies and subsequent commands", async () => {
  const result = await audit(
    [
      "```bash",
      "cat <<'ONE'",
      "<body>",
      "ONE",
      'cat <<"TWO"',
      "<body>",
      "TWO",
      "cat <<THREE",
      "<body>",
      "THREE",
      "git show <path>",
      "```",
      "",
    ].join("\n"),
  );
  assert.deepEqual(result, [finding(11, "path"), finding(9, "body")]);
});

test("only dash heredocs strip tabs and queued bodies keep their own quoting", async () => {
  const result = await audit(
    [
      "```bash",
      "cat <<-'LITERAL' <<DATA",
      "\t<body>",
      "\tLITERAL",
      "An expandable body can mention <<'LOOKALIKE' in text.",
      "<path>",
      "DATA",
      "cat <<'TAIL'",
      "\tTAIL",
      "<message>",
      "TAIL",
      "# A comment mentioning <<'IGNORED' does not open a body.",
      "git show <ref>",
      "```",
    ].join("\n"),
  );
  assert.deepEqual(result, [finding(13, "ref"), finding(6, "path")]);
});

test("a Bash-looking string in a PowerShell fence cannot hide later commands", async () => {
  for (const label of ["powershell", "pwsh"]) {
    const lines = [
      `\`\`\`${label}`,
      "$example = @'",
      "<<'TEXT'",
      "'@",
      "Set-Location -LiteralPath <path>",
      "```",
    ];
    assert.deepEqual(await audit(lines.join("\n")), [finding(5, "path")]);
  }
});

const ENTRY = {
  path: FIXTURE,
  heading: "Create",
  key: "git branch <name>",
  count: 1,
  reason: "The generated name is restricted to [a-z0-9-].",
};

function auditReviewed(lines: string[], reviewed: unknown = [ENTRY]) {
  return withSkillsRepo((root) => {
    writeFile(root, FIXTURE, lines.join("\n"));
    writeFile(root, REVIEWED, JSON.stringify(reviewed));
    return shellDataRules(root);
  });
}

test("reviewing a multiline inline command uses the trimmed physical source line", async () => {
  const key = "<name>` or `agx show <file>`.";
  const lines = ["# Create", "Use `git branch", `  ${key}  `];
  assert.deepEqual(await auditReviewed(lines, [{ ...ENTRY, key }]), []);
  assert.deepEqual(await auditReviewed(lines, []), [
    finding(3, "file"),
    finding(3, "name"),
  ]);
});

test("an entry covers one command line even when it produces multiple findings", async () => {
  const key = `${ENTRY.key} <branch>`;
  assert.deepEqual(
    await auditReviewed(
      ["# Create", "", "```bash", key, "```"],
      [{ ...ENTRY, key }],
    ),
    [],
  );
});

test("identical lines count only within the named heading", async () => {
  assert.deepEqual(
    await auditReviewed([
      "# Create",
      "",
      "```",
      ENTRY.key,
      "```",
      "# Existing",
      "",
      "```",
      ENTRY.key,
      "```",
    ]),
    [finding(9, "name")],
  );
});

for (const copies of [0, 2]) {
  test(`reports a stale entry with ${copies} eligible lines and suppresses none`, async () => {
    const actual = await auditReviewed([
      "# Create",
      "```bash",
      ...Array.from({ length: copies }, () => ENTRY.key),
      "```",
    ]);
    assert.deepEqual(
      actual.filter((result) => result.rule === SHELL_DATA_RULE),
      Array.from({ length: copies }, (_, index) => finding(index + 3, "name")),
    );
    assert.deepEqual(
      actual.filter((result) => result.rule === STALE),
      [
        {
          path: FIXTURE,
          rule: STALE,
          message: `reviewed entry for "git branch <name>" under "Create" expects 1 command line, found ${copies}`,
        },
      ],
    );
  });
}

for (const reason of [" ", undefined]) {
  test(`rejects ${reason === undefined ? "a missing" : "an empty"} reviewed reason`, async () => {
    await assert.rejects(
      auditReviewed([], [{ ...ENTRY, reason }]),
      /entry 0 needs a non-empty reason/,
    );
  });
}

test("a malformed reviewed list cannot silently grant exceptions", async () => {
  await assert.rejects(auditReviewed([], ENTRY), /expected a JSON array/);
  for (const value of [null, "entry", []]) {
    await assert.rejects(auditReviewed([], [value]), /must be an object/);
  }
  for (const fields of [
    { path: 1 },
    { path: "docs/example.md" },
    { path: "skills/example/commands.txt" },
    { heading: null },
    { heading: " Create" },
    { key: 1 },
    { key: "" },
    { key: ` ${ENTRY.key}` },
    { count: "1" },
    { count: 1.5 },
    { count: 0 },
  ]) {
    await assert.rejects(
      auditReviewed([], [{ ...ENTRY, ...fields }]),
      /needs a skill Markdown path, heading, trimmed key and positive count/,
    );
  }
});

test("a reviewed list that is a link is a symlink file error and is not read", async (t) => {
  await withSkillsRepo((root) =>
    withOutside((outside) => {
      const target = path.join(outside, "reviewed.json");
      writeFile(outside, "reviewed.json", "not JSON");
      if (!link(root, REVIEWED, target, "file")) {
        t.skip("this host cannot create a symlink");
        return;
      }
      assert.throws(() => shellDataRules(root), {
        name: "RepoFileError",
        kind: "symlink",
        message: `${REVIEWED}: passes through a link`,
      });
    }),
  );
});

test("a linked skill file is not scanned", async (t) => {
  await withSkillsRepo((root) =>
    withOutside((outside) => {
      const target = path.join(outside, "commands.md");
      writeFile(outside, "commands.md", "```bash\ngit show <ref>\n```\n");
      if (!link(root, FIXTURE, target, "file")) {
        t.skip("this host cannot create a symlink");
        return;
      }
      assert.deepEqual(shellDataRules(root), []);
    }),
  );
});

test("an unheaded command can be reviewed once, and duplicate keys are rejected", async () => {
  const lines = ["```bash", ENTRY.key, "```"];
  const unheaded = { ...ENTRY, heading: "" };
  assert.deepEqual(await auditReviewed(lines, [unheaded]), []);
  await assert.rejects(
    auditReviewed(lines, [unheaded, { ...unheaded, count: 2 }]),
    /duplicates an earlier reviewed line/,
  );
});

test("excludes only the named human-operator examples", async () => {
  await withSkillsRepo((root) => {
    const command = "```bash\ngit branch <name>\n```\n";
    for (const file of [
      "skills/ops/references/examples/nested/a.md",
      "skills/ops/references/runbook-template.md",
      "skills/ops/references/communication-templates.md",
      "skills/gate/references/change/change-record-template.md",
      "skills/ops/references/examples-alt/a.md",
      "skills/example/references/example.txt",
    ])
      writeFile(root, file, command);
    assert.deepEqual(shellDataRules(root), [
      {
        ...finding(2, "name"),
        path: "skills/ops/references/examples-alt/a.md",
      },
    ]);
  });
});

test("the standards baseline rejects both shell data rule IDs", async () => {
  await withSkillsRepo((root) => {
    for (const rule of [SHELL_DATA_RULE, STALE]) {
      writeFile(
        root,
        `${BASELINE_DIR}/example.json`,
        JSON.stringify([{ rule, path: FIXTURE, key: ENTRY.key }]),
      );
      assert.throws(() => readBaseline(root, BASELINED_RULES), {
        message: `skills: ${BASELINE_DIR}/example.json: entry 0 names unknown rule ${rule}`,
      });
    }
  });
});
