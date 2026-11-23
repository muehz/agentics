import assert from "node:assert/strict";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { test } from "node:test";
import type { Failure, RunOptions } from "../../check.ts";
import {
  auditEveryFamily,
  auditSlice,
  sliceNamed,
  addX,
  adapterText,
  all,
  type Mutation,
} from "../../testing/skills-seeds.ts";
import {
  addSkill,
  BREAKERS,
  link,
  mkdir,
  remove,
  withSkillsRepo,
  writeFile,
} from "../../testing/skills-repo.ts";
import { RepoFileError } from "../../repo-file-error.ts";
import { isFileErrorOfKind, seams, withSeams } from "./files.ts";
import { run } from "./index.ts";
import {
  ADAPTER_FILES,
  checkOrchestrationFile,
  FAMILY,
  NATIVE_IDENT_RES,
  SHARED_FILES_EXTRA,
  sharedSet,
  STALE_PHRASES,
  SURFACE_FILES,
} from "./orchestration.ts";
import { canSymlink, SYMLINK_SKIP, withOutside } from "../../testing/host.ts";

const options: RunOptions = {
  short: false,
  fast: false,
  diagnostics: () => {},
  env: process.env,
};

const FILE_PRESENT = "orchestration-file-present";
const STALE = "orchestration-stale-phrase";
const NATIVE = "orchestration-native-identifier";
const MISSING = "exempted in the contract arrays but does not exist";
const LE = String.fromCodePoint(0x2264);
const EM = String.fromCodePoint(0x2014);
const ZERO_SELECTION =
  "skills: the orchestration scan selected 0 files " +
  EM +
  " a guard that inspects nothing must never report success";

const COPILOT_TOOLS = "skills/routing/references/copilot-tools.md";
const MODEL_LADDERS = "skills/routing/references/model-ladders.md";
const AGENT_SENTENCE = "Dispatch with the `Agent` tool.\n";

const audit = (root: string) => run(root, options, { families: [FAMILY] });

const brief = (found: Failure[]): string[][] =>
  found.map((one) => [one.rule, one.path, one.message]);

const NO_LINKS = canSymlink() ? false : SYMLINK_SKIP;

/** Runs `body` on a fresh empty directory and removes it afterwards, once an asynchronous body has settled. */
function withDir<T>(body: (dir: string) => T): T {
  const dir = mkdtempSync(path.join(tmpdir(), "agentics-orchestration-"));
  const cleanup = (): void => rmSync(dir, { recursive: true, force: true });
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

/** An empty scratch checkout. */
const withScratch = withDir;

function mustLink(
  root: string,
  rel: string,
  target: string,
  type?: "file" | "dir" | "junction",
): void {
  assert.equal(link(root, rel, target, type), true, `link ${rel}`);
}

test("the pinned lists hold their committed values", () => {
  assert.deepEqual(SHARED_FILES_EXTRA, [
    "AGENTS.md",
    "README.md",
    "docs/ARCHITECTURE.md",
  ]);
  assert.deepEqual(ADAPTER_FILES, [
    "skills/routing/references/copilot-tools.md",
    "skills/routing/references/surface-dispatch.md",
    "skills/routing/references/model-ladders.md",
    "skills/routing/references/model-ladders-examples.md",
  ]);
  assert.deepEqual(SURFACE_FILES, [
    "docs/surfaces/claude-code.md",
    "docs/surfaces/codex.md",
    "docs/surfaces/copilot-cli.md",
    "docs/surfaces/cursor.md",
    "docs/surfaces/vscode.md",
  ]);
  assert.deepEqual(STALE_PHRASES, [
    "subagents are stateless",
    "one-shot stays the default everywhere",
    "Copilot CLI always uses one-shot dispatch",
    "waves " + LE + "6, depth 1",
  ]);
  assert.equal(STALE_PHRASES[3]?.charCodeAt(6), 0x2264);
  assert.equal(NATIVE_IDENT_RES.length, 5);
});

test("the family is named orchestration and reports its three rules", () => {
  assert.equal(FAMILY.name, "orchestration");
  assert.deepEqual(FAMILY.rules, [FILE_PRESENT, STALE, NATIVE]);
});

test("the green fixture passes", async () => {
  await withSkillsRepo(async (root) => {
    assert.deepEqual(await audit(root), { failures: [] });
  });
});

for (const rel of [...ADAPTER_FILES, ...SURFACE_FILES]) {
  test(`a missing ${rel} is one file-present failure at that path`, async () => {
    await withSkillsRepo(async (root) => {
      remove(root, rel);
      assert.deepEqual(await audit(root), {
        failures: [{ rule: FILE_PRESENT, path: rel, message: MISSING }],
      });
    });
  });
}

test("file-present failures come in declared order, adapters before surfaces, not sorted", async () => {
  await withSkillsRepo(async (root) => {
    remove(root, "docs/surfaces/claude-code.md");
    remove(root, MODEL_LADDERS);
    assert.deepEqual(brief((await audit(root)).failures), [
      [FILE_PRESENT, MODEL_LADDERS, MISSING],
      [FILE_PRESENT, "docs/surfaces/claude-code.md", MISSING],
    ]);
  });
  await withSkillsRepo(async (root) => {
    remove(root, MODEL_LADDERS);
    remove(root, COPILOT_TOOLS);
    assert.deepEqual(brief((await audit(root)).failures), [
      [FILE_PRESENT, COPILOT_TOOLS, MISSING],
      [FILE_PRESENT, MODEL_LADDERS, MISSING],
    ]);
  });
});

test("a missing shared extra is simply absent: no failure and no error", async () => {
  await withSkillsRepo(async (root) => {
    remove(root, "README.md");
    assert.deepEqual(await audit(root), { failures: [] });
  });
});

test("a directory named AGENTS.md is an audit error from the read", async () => {
  await withSkillsRepo(async (root) => {
    remove(root, "AGENTS.md");
    mkdir(root, "AGENTS.md");
    const result = await audit(root);
    assert.deepEqual(result.failures, []);
    assert.ok(
      result.error?.startsWith("skills: reading AGENTS.md: "),
      String(result.error),
    );
  });
});

test(
  "a dangling symlink at an adapter path is the symlink audit error, not a file-present failure",
  { skip: NO_LINKS },
  async () => {
    await withSkillsRepo(async (root) => {
      remove(root, COPILOT_TOOLS);
      mustLink(root, COPILOT_TOOLS, path.join(root, "nowhere.md"), "file");
      assert.deepEqual(await audit(root), {
        failures: [],
        error: `skills: checking ${COPILOT_TOOLS}: ${COPILOT_TOOLS}: passes through a link`,
      });
    });
  },
);

test(
  "a surface file that is a symlink is the symlink audit error",
  { skip: NO_LINKS },
  async () => {
    await withSkillsRepo(async (root) => {
      remove(root, "docs/surfaces/cursor.md");
      mustLink(
        root,
        "docs/surfaces/cursor.md",
        path.join(root, "README.md"),
        "file",
      );
      assert.deepEqual(await audit(root), {
        failures: [],
        error:
          "skills: checking docs/surfaces/cursor.md: docs/surfaces/cursor.md: passes through a link",
      });
    });
  },
);

test(
  "a linked AGENTS.md is a symlink audit error with no findings, while the same text in a regular file is one native finding",
  { skip: NO_LINKS },
  async () => {
    await withSkillsRepo(async (root) => {
      writeFile(root, "AGENTS.md", AGENT_SENTENCE);
      assert.deepEqual(brief((await audit(root)).failures), [
        [
          NATIVE,
          "AGENTS.md",
          "line 1: native orchestration identifier in a shared file",
        ],
      ]);
      await withOutside(async (outside) => {
        writeFile(outside, "AGENTS.md", AGENT_SENTENCE);
        remove(root, "AGENTS.md");
        mustLink(root, "AGENTS.md", path.join(outside, "AGENTS.md"), "file");
        assert.deepEqual(await audit(root), {
          failures: [],
          error: "skills: checking AGENTS.md: AGENTS.md: passes through a link",
        });
      });
    });
  },
);

test(
  "a linked docs directory is a symlink audit error naming the link",
  { skip: NO_LINKS },
  async () => {
    await withSkillsRepo(async (root) => {
      await withOutside(async (outside) => {
        writeFile(outside, "ARCHITECTURE.md", "# Elsewhere\n");
        remove(root, "docs");
        mustLink(root, "docs", outside, "dir");
        assert.deepEqual(await audit(root), {
          failures: [],
          error:
            "skills: checking docs/ARCHITECTURE.md: docs: passes through a link",
        });
      });
    });
  },
);

test("a missing skills directory is an audit error from the walk", async () => {
  await withSkillsRepo(async (root) => {
    remove(root, "skills");
    assert.deepEqual(await audit(root), {
      failures: [],
      error: "skills: walking skills: skills: does not exist",
    });
  });
});

test(
  "a linked skills directory is skipped by the shared set, then the first adapter path fails the symlink check",
  { skip: NO_LINKS },
  async () => {
    await withSkillsRepo(async (root) => {
      await withOutside(async (outside) => {
        writeFile(outside, "sentinel.md", "# Sentinel\n");
        remove(root, "skills");
        mustLink(root, "skills", outside, "dir");
        assert.deepEqual(await audit(root), {
          failures: [],
          error: `skills: checking ${COPILOT_TOOLS}: skills: passes through a link`,
        });
      });
    });
  },
);

test("sharedSet of a checkout holding only skills/a.md is that file", () => {
  withScratch((root) => {
    writeFile(root, "skills/a.md", "# A\n");
    assert.deepEqual(sharedSet(root), ["skills/a.md"]);
  });
});

test("sharedSet with an empty skills directory and no extras is the zero-selection audit error", () => {
  withScratch((root) => {
    mkdir(root, "skills");
    assert.throws(() => sharedSet(root), { message: ZERO_SELECTION });
  });
});

test("sharedSet with no skills at all and no extras fails the walk, and with an extra it fails the walk too", () => {
  withScratch((root) => {
    assert.throws(() => sharedSet(root), {
      message: "skills: walking skills: skills: does not exist",
    });
    writeFile(root, "AGENTS.md", "# A\n");
    assert.throws(() => sharedSet(root), /^Error: skills: walking skills: /);
  });
});

test("sharedSet with skills as a regular file selects nothing from it", () => {
  withScratch((root) => {
    writeFile(root, "skills", "text");
    assert.throws(() => sharedSet(root), { message: ZERO_SELECTION });
    writeFile(root, "README.md", "# R\n");
    assert.deepEqual(sharedSet(root), ["README.md"]);
  });
});

test(
  "sharedSet with a linked skills root selects only the extras, or nothing",
  { skip: NO_LINKS },
  () => {
    withScratch((root) => {
      withOutside((outside) => {
        writeFile(outside, "sentinel.md", "# Sentinel\n");
        mustLink(root, "skills", outside, "dir");
        assert.throws(() => sharedSet(root), { message: ZERO_SELECTION });
        writeFile(root, "AGENTS.md", "# A\n");
        assert.deepEqual(sharedSet(root), ["AGENTS.md"]);
      });
    });
  },
);

test(
  "sharedSet selects regular markdown files only: a linked directory and a linked file are not selected",
  { skip: NO_LINKS },
  () => {
    withScratch((root) => {
      withOutside((outside) => {
        writeFile(root, "skills/good.md", "# Good\n");
        writeFile(outside, "linked/bad.md", "# Bad\n");
        writeFile(outside, "leaf.md", "# Leaf\n");
        mustLink(root, "skills/linked", path.join(outside, "linked"), "dir");
        mustLink(root, "skills/leaf.md", path.join(outside, "leaf.md"), "file");
        assert.deepEqual(sharedSet(root), ["skills/good.md"]);
      });
    });
  },
);

test("sharedSet matches the .md suffix case-sensitively, skips directories named like markdown and skips no directory name", () => {
  withScratch((root) => {
    writeFile(root, "skills/NOTES.MD", "x");
    writeFile(root, "skills/a.txt", "x");
    mkdir(root, "skills/guide.md");
    writeFile(root, "skills/x/tests/y.md", "x");
    writeFile(root, "skills/x/node_modules/z.md", "x");
    writeFile(root, "skills/.git/w.md", "x");
    assert.deepEqual(sharedSet(root), [
      "skills/.git/w.md",
      "skills/x/node_modules/z.md",
      "skills/x/tests/y.md",
    ]);
  });
});

test("sharedSet is sorted in byte order without duplicates, and includes the extras that exist", () => {
  withScratch((root) => {
    writeFile(root, "skills/tdd/SKILL.md", "x");
    writeFile(root, "skills/Z.md", "x");
    writeFile(root, "skills/a.md", "x");
    writeFile(root, "docs/ARCHITECTURE.md", "x");
    writeFile(root, "README.md", "x");
    writeFile(root, "AGENTS.md", "x");
    const set = sharedSet(root);
    assert.deepEqual(set, [
      "AGENTS.md",
      "README.md",
      "docs/ARCHITECTURE.md",
      "skills/Z.md",
      "skills/a.md",
      "skills/tdd/SKILL.md",
    ]);
    assert.equal(new Set(set).size, set.length);
  });
});

test("sharedSet includes a directory named AGENTS.md, which passes the stat", () => {
  withScratch((root) => {
    mkdir(root, "AGENTS.md");
    writeFile(root, "skills/a.md", "x");
    assert.deepEqual(sharedSet(root), ["AGENTS.md", "skills/a.md"]);
  });
});

for (const [index, phrase] of STALE_PHRASES.entries()) {
  test(`stale phrase ${index + 1} is reported at its line with the quoted phrase`, async () => {
    await withSkillsRepo(async (root) => {
      writeFile(root, "AGENTS.md", `# Shared\n\nNote that ${phrase} here.\n`);
      assert.deepEqual(await audit(root), {
        failures: [
          {
            rule: STALE,
            path: "AGENTS.md",
            message: `line 3: stale universal claim: "${phrase}"`,
          },
        ],
      });
    });
  });
}

test("a stale phrase in a surface file is reported once", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(
      root,
      "docs/surfaces/codex.md",
      "# Adapter\n\nNote that subagents are stateless here.\n",
    );
    assert.deepEqual(brief((await audit(root)).failures), [
      [
        STALE,
        "docs/surfaces/codex.md",
        'line 3: stale universal claim: "subagents are stateless"',
      ],
    ]);
  });
});

for (const rel of ADAPTER_FILES) {
  test(`a stale phrase in ${rel} is reported twice, because adapters are scanned twice`, async () => {
    await withSkillsRepo(async (root) => {
      writeFile(
        root,
        rel,
        "# Adapter\n\nCopilot CLI always uses one-shot dispatch.\n",
      );
      const message =
        'line 3: stale universal claim: "Copilot CLI always uses one-shot dispatch"';
      assert.deepEqual(await audit(root), {
        failures: [
          { rule: STALE, path: rel, message },
          { rule: STALE, path: rel, message },
        ],
      });
    });
  });
}

test("one line holding two phrases gives two failures in phrase order, not text order", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(
      root,
      "README.md",
      "one-shot stays the default everywhere and subagents are stateless\n",
    );
    assert.deepEqual(
      brief((await audit(root)).failures).map((row) => row[2]),
      [
        'line 1: stale universal claim: "subagents are stateless"',
        'line 1: stale universal claim: "one-shot stays the default everywhere"',
      ],
    );
  });
});

test("the phrase match is case-sensitive, once per line and phrase, and counts LF only", () => {
  assert.deepEqual(
    checkOrchestrationFile("x.md", "Subagents are stateless\n", false),
    [],
  );
  assert.deepEqual(
    brief(
      checkOrchestrationFile(
        "x.md",
        "subagents are stateless, subagents are stateless\nok\nsubagents are stateless\n",
        false,
      ),
    ).map((row) => row[2]),
    [
      'line 1: stale universal claim: "subagents are stateless"',
      'line 3: stale universal claim: "subagents are stateless"',
    ],
  );
  assert.deepEqual(
    brief(
      checkOrchestrationFile(
        "x.md",
        "a\r\nb\r\nsubagents are stateless\r\n",
        false,
      ),
    ).map((row) => row[2]),
    ['line 3: stale universal claim: "subagents are stateless"'],
  );
  assert.deepEqual(
    brief(
      checkOrchestrationFile("x.md", "a\rsubagents are stateless\n", false),
    ).map((row) => row[2]),
    ['line 1: stale universal claim: "subagents are stateless"'],
  );
});

test("within a file the stale failures come before the native ones", () => {
  const text =
    "# T\n\nDispatch with the Agent tool.\n\nok\nsubagents are stateless\n";
  assert.deepEqual(
    brief(checkOrchestrationFile("AGENTS.md", text, false)).map((row) => [
      row[0],
      row[2],
    ]),
    [
      [STALE, 'line 6: stale universal claim: "subagents are stateless"'],
      [NATIVE, "line 3: native orchestration identifier in a shared file"],
    ],
  );
});

test("files are scanned in byte order, and every file-present failure precedes every scan failure", async () => {
  await withSkillsRepo(async (root) => {
    const sentence = "# T\n\nsubagents are stateless\n";
    for (const rel of [
      "README.md",
      "AGENTS.md",
      "docs/ARCHITECTURE.md",
      "docs/surfaces/cursor.md",
      "skills/tdd/SKILL.md",
    ])
      writeFile(root, rel, sentence);
    remove(root, "docs/surfaces/codex.md");
    assert.deepEqual(
      (await audit(root)).failures.map((one) => [one.rule, one.path]),
      [
        [FILE_PRESENT, "docs/surfaces/codex.md"],
        [STALE, "AGENTS.md"],
        [STALE, "README.md"],
        [STALE, "docs/ARCHITECTURE.md"],
        [STALE, "docs/surfaces/cursor.md"],
        [STALE, "skills/tdd/SKILL.md"],
      ],
    );
  });
});

test("a native identifier in AGENTS.md is one failure", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(root, "AGENTS.md", "# Shared\n\n" + AGENT_SENTENCE);
    assert.deepEqual(await audit(root), {
      failures: [
        {
          rule: NATIVE,
          path: "AGENTS.md",
          message: "line 3: native orchestration identifier in a shared file",
        },
      ],
    });
  });
});

test("a new skill and a new reference file are audited with no edit to the rule", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(
      root,
      "skills/newskill/SKILL.md",
      "# New skill\n\nDispatch two readers with the Agent tool.\n",
    );
    assert.deepEqual(await audit(root), {
      failures: [
        {
          rule: NATIVE,
          path: "skills/newskill/SKILL.md",
          message: "line 3: native orchestration identifier in a shared file",
        },
      ],
    });
  });
  await withSkillsRepo(async (root) => {
    writeFile(
      root,
      "skills/newskill/references/how.md",
      "# How\n\nCall spawn_agent for each reader.\n",
    );
    assert.deepEqual(await audit(root), {
      failures: [
        {
          rule: NATIVE,
          path: "skills/newskill/references/how.md",
          message: "line 3: native orchestration identifier in a shared file",
        },
      ],
    });
  });
});

const DENY_SAMPLES = [
  "Note that subagents are stateless and cannot see this conversation.",
  "On every host, one-shot stays the default everywhere.",
  "Copilot CLI always uses one-shot dispatch.",
  "Limits are waves " + LE + "6, depth 1 on this surface.",
  "Dispatch with the Agent tool.",
  "Dispatch with the `Agent` tool.",
  "Use the Task tool for this.",
  "Dispatch with the `task` tool.",
  "Issue multiple **task** tool calls in one turn.",
  "Pass run_in_background: true to background it.",
  "Call spawn_agent to start one.",
  "Reply with SendMessage to continue.",
  "The runSubagent tool handles dispatch.",
  "Use invoke_subagent here.",
  "Run /fleet to fan out.",
];

test("all fifteen deny samples fail as a shared file", () => {
  assert.equal(DENY_SAMPLES.length, 15);
  for (const sample of DENY_SAMPLES)
    assert.ok(
      checkOrchestrationFile("sample.md", sample + "\n", false).length >= 1,
      sample,
    );
});

test("both clean samples pass", () => {
  for (const sample of [
    "Hand the specialist a self-contained prompt; see the surface adapter for names.",
    "Teammates get messaging and task tools automatically.",
  ])
    assert.deepEqual(
      checkOrchestrationFile("clean.md", sample + "\n", false),
      [],
      sample,
    );
});

test("an adapter keeps its native names but not a stale phrase", () => {
  assert.deepEqual(
    checkOrchestrationFile(
      "adapter.md",
      "Dispatch: the `runSubagent` tool. Limits: depth <=5.\n",
      true,
    ),
    [],
  );
  assert.equal(
    checkOrchestrationFile(
      "adapter.md",
      "Note subagents are stateless here.\n",
      true,
    ).length,
    1,
  );
});

test("adapters and surface files own native names through the family, a shared file does not", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(
      root,
      COPILOT_TOOLS,
      "# Adapter\n\nPass run_in_background: true to background it.\n",
    );
    writeFile(
      root,
      "docs/surfaces/vscode.md",
      "# Adapter\n\nPass run_in_background: true to background it.\n",
    );
    assert.deepEqual(await audit(root), { failures: [] });
    writeFile(
      root,
      "README.md",
      "# Shared\n\nPass run_in_background: true to background it.\n",
    );
    assert.deepEqual(brief((await audit(root)).failures), [
      [
        NATIVE,
        "README.md",
        "line 3: native orchestration identifier in a shared file",
      ],
    ]);
  });
});

test("two matching identifiers on one line give one failure", () => {
  assert.equal(
    checkOrchestrationFile(
      "x.md",
      "Use the Agent tool and spawn_agent and /fleet here.\n",
      false,
    ).length,
    1,
  );
});

const fires = (line: string): boolean =>
  checkOrchestrationFile("x.md", line, false).some(
    (one) => one.rule === NATIVE,
  );

const VT = String.fromCharCode(0x0b);
const NBSP = String.fromCharCode(0xa0);

function toolRows(word: string): Array<[string, boolean]> {
  return [
    [`${word}${VT}tool`, true],
    [`${word}${NBSP}tool`, false],
    [`Sub${word} tools`, true],
    [`${word} toolkit`, true],
    [`${word}s tool`, false],
    [`${word}tool`, false],
    [`**${word}** tool`, true],
    [`${word.toLowerCase()} tool`, false],
    [`${word} tool`, true],
  ];
}

test("patterns 1 and 2 (the Agent and Task tool) match the capitalized word, ASCII space and tool", () => {
  for (const word of ["Agent", "Task"])
    for (const [line, expected] of toolRows(word))
      assert.equal(fires(line), expected, JSON.stringify(line));
});

test("pattern 3 (the emphasised lowercase task tool) needs a marker on both sides", () => {
  for (const [line, expected] of [
    ["`task` tool", true],
    ["**task** tool", true],
    ["__task__ tool", true],
    ["task tool", false],
    ["`task tool", false],
    ["task` tool", false],
  ] as Array<[string, boolean]>)
    assert.equal(fires(line), expected, line);
});

test("pattern 4 (the eight native names) is bounded by non-word characters and ASCII", () => {
  for (const [line, expected] of [
    ["spawn_agent", true],
    ["my_spawn_agent", false],
    ["spawn_agent2", false],
    ["spawn_agent-x", true],
    [String.fromCharCode(0xe9) + "spawn_agent", true],
    ["(spawn_agent)", true],
    ["xspawn_agent", false],
  ] as Array<[string, boolean]>)
    assert.equal(fires(line), expected, JSON.stringify(line));
  for (const name of [
    "run_in_background",
    "spawn_agent",
    "send_input",
    "wait_agent",
    "write_agent",
    "SendMessage",
    "runSubagent",
    "invoke_subagent",
  ]) {
    assert.equal(fires(`use ${name} here`), true, name);
    assert.equal(fires(name), true, name);
    assert.equal(fires(`a${name}`), false, name);
  }
});

test("pattern 5 (/fleet) needs whitespace or the line start before it and no word character or dash after", () => {
  for (const [line, expected] of [
    ["/fleet", true],
    ["\t/fleet.", true],
    ["x/fleet", false],
    ["/fleet-y", false],
    ["/fleety", false],
    ["/fleet_y", false],
    ["run /fleet now", true],
  ] as Array<[string, boolean]>)
    assert.equal(fires(line), expected, JSON.stringify(line));
});

// --- audit error texts and causes, rows added by the mutation pass -----------------------------------------------------

/** The inspection and read errors a file-system call can raise on `target`, shown as `shown`, with the reason each gives. */
function failureRows(
  target: string,
  shown: string,
): [label: string, error: unknown, text: string][] {
  return [
    [
      "a coded failure",
      Object.assign(new Error("EACCES: permission denied, lstat"), {
        code: "EACCES",
        syscall: "lstat",
        path: target,
      }),
      `${shown}: EACCES`,
    ],
    [
      "a coded failure with no path",
      Object.assign(new Error("EACCES: permission denied"), {
        code: "EACCES",
        syscall: "lstat",
      }),
      `${shown}: EACCES`,
    ],
    [
      "a failure with a syscall and no code",
      Object.assign(new Error("odd failure"), { syscall: "lstat" }),
      `${shown}: unreadable`,
    ],
    [
      "a failure with a code and no syscall",
      Object.assign(new Error("read failed"), { code: "EACCES" }),
      `${shown}: EACCES`,
    ],
    ["no error object", null, `${shown}: unreadable`],
  ];
}

test("a shared root whose inspection fails is an audit error naming it and why", async () => {
  await withSkillsRepo(async (root) => {
    const target = path.join(root, "skills");
    const real = seams.lstat;
    for (const [label, error, text] of failureRows(target, "skills")) {
      const result = await withSeams(
        {
          lstat: (candidate) => {
            if (candidate === target) throw error;
            return real(candidate);
          },
        },
        () => audit(root),
      );
      assert.deepEqual(result.failures, [], label);
      assert.equal(result.error, `skills: walking skills: ${text}`, label);
    }
  });
});

test("a scanned file that cannot be read is an audit error naming it and why", async () => {
  await withSkillsRepo(async (root) => {
    const target = path.join(root, "AGENTS.md");
    const real = seams.readFile;
    for (const [label, error, text] of failureRows(target, "AGENTS.md")) {
      const result = await withSeams(
        {
          readFile: (candidate) => {
            if (candidate === target) throw error;
            return real(candidate);
          },
        },
        () => audit(root),
      );
      assert.deepEqual(result.failures, [], label);
      assert.equal(result.error, `skills: reading AGENTS.md: ${text}`, label);
    }
  });
});

test("a scanned file whose read fails as missing is skipped, from a code or a file error", async () => {
  await withSkillsRepo(async (root) => {
    const target = path.join(root, "AGENTS.md");
    const real = seams.readFile;
    const gone = (): Error =>
      Object.assign(new Error("gone"), { code: "ENOENT" });
    const rows: [string, unknown][] = [
      ["the not-exist code", gone()],
      [
        "a missing file error",
        new RepoFileError("missing", "elsewhere", { cause: gone() }),
      ],
    ];
    for (const [label, error] of rows) {
      const result = await withSeams(
        {
          readFile: (candidate) => {
            if (candidate === target) throw error;
            return real(candidate);
          },
        },
        () => audit(root),
      );
      assert.deepEqual(result, { failures: [] }, label);
    }
    const other = await withSeams(
      {
        readFile: (candidate) => {
          if (candidate === target)
            throw new Error("read failed", {
              cause: Object.assign(new Error("denied"), { code: "EACCES" }),
            });
          return real(candidate);
        },
      },
      () => audit(root),
    );
    assert.equal(
      other.error,
      "skills: reading AGENTS.md: AGENTS.md: unreadable",
    );
  });
});

test("a walk of a shared root that fails is an audit error naming the directory", async () => {
  await withSkillsRepo(async (root) => {
    const skillsDir = path.join(root, "skills");
    const real = seams.readDir;
    const result = await withSeams(
      {
        readDir: (dir) => {
          if (dir === skillsDir)
            throw Object.assign(new Error("denied"), { code: "EACCES" });
          return real(dir);
        },
      },
      () => audit(root),
    );
    assert.deepEqual(result.failures, []);
    assert.equal(result.error, "skills: walking skills: skills: EACCES");
  });
});

test("an audit error keeps the failure that caused it as its cause", async () => {
  await withSkillsRepo(async (root) => {
    const context = { families: [], evalsCheck: () => [] };
    // A failure the file reader wraps is the cause of the file error that the audit error carries.
    const causeOf = (error: Error): unknown =>
      error.cause instanceof RepoFileError && error.cause.cause !== undefined
        ? error.cause.cause
        : error.cause;
    const expectCause = (body: () => unknown, cause: unknown, label: string) =>
      assert.throws(
        body,
        (error: unknown) => error instanceof Error && causeOf(error) === cause,
        label,
      );
    const skillsDir = path.join(root, "skills");
    const agents = path.join(root, "AGENTS.md");
    const realLstat = seams.lstat;
    const realReadDir = seams.readDir;
    const denied = Object.assign(new Error("denied"), {
      code: "EACCES",
      syscall: "lstat",
      path: skillsDir,
    });
    withSeams(
      {
        lstat: (candidate) => {
          if (candidate === skillsDir) throw denied;
          return realLstat(candidate);
        },
      },
      () => expectCause(() => sharedSet(root), denied, "a root"),
    );
    const linked = new RepoFileError("symlink", agents);
    withSeams(
      {
        lstat: (candidate) => {
          if (candidate === agents) throw linked;
          return realLstat(candidate);
        },
      },
      () => expectCause(() => sharedSet(root), linked, "an extra"),
    );
    const unreadable = Object.assign(new Error("denied"), { code: "EACCES" });
    withSeams(
      {
        readDir: (dir) => {
          if (dir === skillsDir) throw unreadable;
          return realReadDir(dir);
        },
      },
      () =>
        assert.throws(
          () => sharedSet(root),
          (error: unknown) =>
            error instanceof Error &&
            isFileErrorOfKind(error.cause, "unreadable") &&
            error.cause.message === "skills: EACCES",
          "a walk",
        ),
    );
    const unread = new Error("read failed");
    withSeams(
      {
        readFile: () => {
          throw unread;
        },
      },
      () => expectCause(() => FAMILY.run(root, context), unread, "a read"),
    );
  });
});

// --- the whole-domain scenarios ------------------------------------------------------------------------------------

const METADATA_SLICE = sliceNamed("metadata");

const finding = (rule: string, at: string, message: string): Failure => ({
  rule,
  path: at,
  message,
});

const nativeAt = (at: string, line: number): Failure =>
  finding(
    NATIVE,
    at,
    `line ${line}: native orchestration identifier in a shared file`,
  );

const staleAt = (at: string, line: number, phrase: string): Failure =>
  finding(
    STALE,
    at,
    `line ${line}: stale universal claim: ${JSON.stringify(phrase)}`,
  );

const STATELESS = "subagents are stateless";
const ONE_SHOT = "one-shot stays the default everywhere";

test("orchestration-file-present", async () => {
  assert.deepEqual(await auditEveryFamily(BREAKERS[FILE_PRESENT]!), {
    failures: [finding(FILE_PRESENT, MODEL_LADDERS, MISSING)],
  });
});

test("orchestration-stale-phrase", async () => {
  assert.deepEqual(await auditEveryFamily(BREAKERS[STALE]!), {
    failures: [staleAt("docs/surfaces/codex.md", 3, STATELESS)],
  });
});

test("orchestration-native-identifier", async () => {
  assert.deepEqual(await auditEveryFamily(BREAKERS[NATIVE]!), {
    failures: [nativeAt("AGENTS.md", 3)],
  });
});

test("two stale phrases in one adapter", async () => {
  // Each stale phrase of an adapter is reported twice.
  const once = [
    staleAt(COPILOT_TOOLS, 3, STATELESS),
    staleAt(COPILOT_TOOLS, 5, ONE_SHOT),
  ];
  assert.deepEqual(
    await auditSlice(
      METADATA_SLICE,
      adapterText(`${STATELESS}\n\n${ONE_SHOT}`),
    ),
    { failures: [...once, ...once] },
  );
});

test("a stale phrase in each of the three adapters", async () => {
  const adapters = [
    COPILOT_TOOLS,
    "skills/routing/references/surface-dispatch.md",
    MODEL_LADDERS,
  ];
  assert.deepEqual(
    await auditSlice(
      METADATA_SLICE,
      all(
        ...adapters.map(
          (rel): Mutation =>
            (r) =>
              writeFile(r, rel, `# Adapter\n\n${STATELESS}\n`),
        ),
      ),
    ),
    {
      failures: [
        COPILOT_TOOLS,
        COPILOT_TOOLS,
        MODEL_LADDERS,
        MODEL_LADDERS,
        "skills/routing/references/surface-dispatch.md",
        "skills/routing/references/surface-dispatch.md",
      ].map((rel) => staleAt(rel, 3, STATELESS)),
    },
  );
});

test("two stale phrases on one line", async () => {
  const once = [
    staleAt(COPILOT_TOOLS, 3, STATELESS),
    staleAt(COPILOT_TOOLS, 3, ONE_SHOT),
  ];
  assert.deepEqual(
    await auditSlice(
      METADATA_SLICE,
      adapterText(`${STATELESS} and ${ONE_SHOT}`),
    ),
    { failures: [...once, ...once] },
  );
});

test("a stale phrase on line 5 and an identifier on line 3", async () => {
  assert.deepEqual(
    await auditSlice(METADATA_SLICE, (r) =>
      writeFile(
        r,
        "AGENTS.md",
        `# Shared\n\nUse the Agent tool.\n\n${STATELESS}\n`,
      ),
    ),
    {
      failures: [staleAt("AGENTS.md", 5, STATELESS), nativeAt("AGENTS.md", 3)],
    },
  );
});

test("Agent, a vertical tab and tool", async () => {
  assert.deepEqual(
    await auditSlice(METADATA_SLICE, (r) =>
      writeFile(r, "AGENTS.md", "# Shared\n\nAgent\x0btool\n"),
    ),
    { failures: [nativeAt("AGENTS.md", 3)] },
  );
});

test("spawn_agent-x", async () => {
  assert.deepEqual(
    await auditSlice(METADATA_SLICE, (r) =>
      writeFile(r, "AGENTS.md", "# Shared\n\nspawn_agent-x\n"),
    ),
    { failures: [nativeAt("AGENTS.md", 3)] },
  );
});

test("/fleet", async () => {
  assert.deepEqual(
    await auditSlice(METADATA_SLICE, (r) =>
      writeFile(r, "AGENTS.md", "# Shared\n\nrun /fleet now\n"),
    ),
    { failures: [nativeAt("AGENTS.md", 3)] },
  );
});

test("an identifier in a tests directory", async () => {
  assert.deepEqual(
    await auditSlice(
      METADATA_SLICE,
      all(addX, (r) =>
        writeFile(r, "skills/x/tests/y.md", "run_in_background\n"),
      ),
    ),
    { failures: [nativeAt("skills/x/tests/y.md", 1)] },
  );
});

test("an identifier in a node_modules directory", async () => {
  assert.deepEqual(
    await auditSlice(
      METADATA_SLICE,
      all(addX, (r) =>
        writeFile(r, "skills/x/node_modules/z.md", "run_in_background\n"),
      ),
    ),
    { failures: [nativeAt("skills/x/node_modules/z.md", 1)] },
  );
});

test("an identifier in README.md", async () => {
  assert.deepEqual(
    await auditSlice(METADATA_SLICE, (r) =>
      writeFile(r, "README.md", "# Shared\n\nrun_in_background\n"),
    ),
    { failures: [nativeAt("README.md", 3)] },
  );
});

test("an identifier in a nested new reference file", async () => {
  assert.deepEqual(
    await auditSlice(
      METADATA_SLICE,
      all(
        (r) => addSkill(r, "newskill"),
        (r) =>
          writeFile(
            r,
            "skills/newskill/references/how.md",
            "# How\n\nrun_in_background\n",
          ),
      ),
    ),
    { failures: [nativeAt("skills/newskill/references/how.md", 3)] },
  );
});
