import assert from "node:assert/strict";
import { mkdirSync, mkdtempSync, rmSync, symlinkSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { test, type TestContext } from "node:test";
import type { Failure } from "../../check.ts";
import { RepoFileError } from "../../repo-file-error.ts";
import { writeFile } from "../../testing/files.ts";
import {
  auditManifests,
  BREAKERS,
  CLAUDE_PLUGIN,
  CODEX_PLUGIN,
  COPILOT_HOOKS,
  COPILOT_PLUGIN,
  editJson,
  mkdir,
  remove,
  withManifestsRepo,
} from "../../testing/manifests-repo.ts";
import {
  editCodex,
  link as linkInFixture,
  nest,
  versions,
} from "../../testing/manifests-seeds.ts";
import type { Tracked } from "../repo/walk.ts";
import { PartialAuditError } from "./files.ts";
import {
  claudeMdRule,
  claudeProviderRules,
  noMcpRules,
  pathReferenceRules,
  pluginJsonRules,
  registrationRules,
} from "./plugins.ts";
import { canSymlink, removeTree, SYMLINK_SKIP } from "../../testing/host.ts";

const CLAUDE = ".claude-plugin/plugin.json";
const CODEX = ".codex-plugin/plugin.json";
const GITHUB = ".github/plugin/plugin.json";
const ANTIGRAVITY = ".antigravity-plugin/plugin.json";

/** Runs `body` on a temp tree holding `files`, then removes it. */
function withTree<T>(
  files: Record<string, string>,
  body: (root: string) => T,
): T {
  const root = mkdtempSync(path.join(tmpdir(), "agentics-plugins-"));
  try {
    for (const [rel, content] of Object.entries(files))
      writeFile(root, rel, content);
    return body(root);
  } finally {
    removeTree(root);
  }
}

/** Makes a symlink, or skips the test with the reason when the host cannot. */
function link(t: TestContext, target: string, at: string): boolean {
  try {
    mkdirSync(path.dirname(at), { recursive: true });
    symlinkSync(target, at);
    return true;
  } catch {
    t.skip("this host cannot create a symlink");
    return false;
  }
}

test("plugin-json-valid accepts JSON of any kind and rejects text that is not JSON", () => {
  const files: Record<string, string> = {};
  const good = [
    "[]",
    "7",
    "null",
    "{}",
    '"x"',
    '{"a":[1,2]}',
    '{"a":1e400}',
    "[".repeat(10_001) + "]".repeat(10_001),
  ];
  const bad = ["{", "", " ", "{'a':1}", "[1,]", "\ufeff{}"];
  good.forEach((text, i) => (files[`good${i}/plugin.json`] = text));
  bad.forEach((text, i) => (files[`bad${i}/plugin.json`] = text));
  withTree(files, (root) => {
    const { found, failures } = pluginJsonRules(root);
    assert.equal(found.length, good.length + bad.length);
    assert.deepEqual(
      failures.map((f) => f.path),
      bad.map((_, i) => `bad${i}/plugin.json`),
    );
    for (const failure of failures) {
      assert.equal(failure.rule, "plugin-json-valid");
      assert.equal(failure.message, "invalid JSON");
    }
  });
});

test("plugin-json-valid lists every plugin.json in default sort order, ignores a directory of that name, and skips three directory names at any depth", () => {
  withTree(
    {
      "plugin.json": "{}",
      "b/plugin.json": "{}",
      "a/c/plugin.json": "{}",
      "a-b/plugin.json": "{}",
      "a/plugin.json/inner.txt": "x",
      ".git/plugin.json": "{",
      "node_modules/plugin.json": "{",
      "a/node_modules/plugin.json": "{",
      ".claude/plugin.json": "{",
      "x/.claude/y/plugin.json": "{",
      ".worktrees/w/plugin.json": "{",
      "docs/handoffs/plugin.json": "{",
    },
    (root) => {
      const { found, failures } = pluginJsonRules(root);
      assert.deepEqual(found, [
        ".worktrees/w/plugin.json",
        "a-b/plugin.json",
        "a/c/plugin.json",
        "b/plugin.json",
        "docs/handoffs/plugin.json",
        "plugin.json",
      ]);
      assert.deepEqual(
        failures.map((f) => f.path),
        [".worktrees/w/plugin.json", "docs/handoffs/plugin.json"],
      );
    },
  );
});

test("plugin-json-valid fails a plugin.json symlink that points nowhere as invalid JSON", (t) => {
  withTree({ "keep.txt": "x" }, (root) => {
    if (!link(t, "missing.json", path.join(root, "d", "plugin.json"))) return;
    const { found, failures } = pluginJsonRules(root);
    assert.deepEqual(found, ["d/plugin.json"]);
    assert.deepEqual(failures, [
      {
        rule: "plugin-json-valid",
        path: "d/plugin.json",
        message: "invalid JSON",
      },
    ]);
  });
});

test("plugin-json-valid reads a plugin.json symlink that points at a directory, and the read fails", (t) => {
  withTree({ "outside/keep.txt": "x" }, (root) => {
    const at = path.join(root, "d", "plugin.json");
    if (!link(t, path.join(root, "outside"), at)) return;
    assert.throws(
      () => pluginJsonRules(root),
      (error: unknown) =>
        error instanceof RepoFileError &&
        error.kind === "not-a-file" &&
        error.message === "d/plugin.json: is not a regular file",
    );
  });
});

const refFixture = (manifest: string): Record<string, string> => ({
  [CLAUDE]: manifest,
  "skills/tdd/SKILL.md": "x",
  "hooks/h.json": "{}",
});
const refFailures = (files: Record<string, string>) =>
  withTree(files, (root) => pathReferenceRules(root));
const refMessages = (manifest: string) =>
  refFailures(refFixture(manifest)).map((f) => f.message);

test("plugin-path-references accepts paths of the right kind", () => {
  assert.deepEqual(
    refFailures(
      refFixture(
        '{"skills":["./skills/tdd","./hooks/h.json"],"apps":"./skills","hooks":"./hooks/h.json","mcpServers":["./hooks/h.json"]}',
      ),
    ),
    [],
  );
  assert.deepEqual(refMessages('{"mcpServers":"./hooks/h.json"}'), []);
});

test("plugin-path-references reports a missing path and a path of the wrong kind, naming each", () => {
  assert.deepEqual(refMessages('{"skills":["./skills/nope"]}'), [
    "skills path not found → ./skills/nope",
  ]);
  assert.deepEqual(refMessages('{"skills":"./hooks/h.json"}'), [
    "skills path not found → ./hooks/h.json (string form must be a directory)",
  ]);
  assert.deepEqual(refMessages('{"apps":"./nope"}'), [
    "apps path not found → ./nope (string form must be a directory)",
  ]);
  assert.deepEqual(refMessages('{"hooks":"./skills"}'), [
    "hooks path not found → ./skills",
  ]);
  assert.deepEqual(refMessages('{"mcpServers":"./skills"}'), [
    "mcpServers path not found → ./skills",
  ]);
  assert.deepEqual(refMessages('{"mcpServers":"./nope.json"}'), [
    "mcpServers path not found → ./nope.json",
  ]);
  assert.deepEqual(
    refFailures(refFixture('{"hooks":"./nope"}')).map((f) => [f.rule, f.path]),
    [["plugin-path-references", CLAUDE]],
  );
});

test("plugin-path-references treats an empty or non-string element as an empty path, which escapes", () => {
  assert.deepEqual(refMessages('{"skills":["",null,5,{}]}'), [
    'skills path escapes checkout: ""',
    'skills path escapes checkout: ""',
    'skills path escapes checkout: ""',
    'skills path escapes checkout: ""',
  ]);
  assert.deepEqual(refMessages('{"mcpServers":{}}'), [
    'mcpServers path escapes checkout: ""',
  ]);
  assert.deepEqual(refMessages('{"mcpServers":[5]}'), [
    'mcpServers path escapes checkout: ""',
  ]);
  assert.deepEqual(refMessages('{"hooks":"../x","apps":["/abs"]}'), [
    'apps path escapes checkout: "/abs"',
    'hooks path escapes checkout: "../x"',
  ]);
});

test("plugin-path-references ignores a field of any other type, and mcpServers null", () => {
  assert.deepEqual(
    refMessages('{"skills":5,"apps":true,"hooks":5,"mcpServers":null}'),
    [],
  );
  assert.deepEqual(
    refMessages('{"skills":{"a":1},"apps":null,"hooks":[]}'),
    [],
  );
});

test("plugin-path-references checks the Claude, Codex, GitHub and Antigravity manifests in that order and skips one that is absent or not an object", () => {
  const missing = '{"hooks":"./nope"}';
  const failures = refFailures({
    [ANTIGRAVITY]: missing,
    [GITHUB]: missing,
    [CODEX]: missing,
    [CLAUDE]: missing,
    "other/plugin.json": missing,
  });
  assert.deepEqual(
    failures.map((f) => f.path),
    [CLAUDE, CODEX, GITHUB, ANTIGRAVITY],
  );
  for (const text of ["[]", "{", "7", "", "null"])
    assert.deepEqual(refMessages(text), [], text);
  assert.deepEqual(refFailures({}), []);
});

test("plugin-path-references validates string logo paths as existing contained files", () => {
  assert.deepEqual(
    refFailures({
      [ANTIGRAVITY]: '{"logo":"assets/logo.svg"}',
      "assets/logo.svg": "<svg></svg>\n",
    }),
    [],
  );
  assert.deepEqual(
    refFailures({
      [ANTIGRAVITY]: '{"logo":"assets/missing.svg"}',
    }),
    [
      {
        rule: "plugin-path-references",
        path: ANTIGRAVITY,
        message: "logo path not found → assets/missing.svg",
      },
    ],
  );
  assert.deepEqual(
    refFailures({
      [ANTIGRAVITY]: '{"logo":"./skills"}',
      "skills/tdd/SKILL.md": "x",
    }),
    [
      {
        rule: "plugin-path-references",
        path: ANTIGRAVITY,
        message: "logo path not found → ./skills",
      },
    ],
  );
  assert.deepEqual(
    refFailures({
      [ANTIGRAVITY]: '{"logo":"../outside.svg"}',
    }),
    [
      {
        rule: "plugin-path-references",
        path: ANTIGRAVITY,
        message: 'logo path escapes checkout: "../outside.svg"',
      },
    ],
  );
  // Non-string logo values are ignored, matching hooks.
  assert.deepEqual(
    refFailures({
      [ANTIGRAVITY]: '{"logo":null}',
      [CLAUDE]: '{"logo":5}',
    }),
    [],
  );
});

test("plugin-path-references accepts a link that stays inside the checkout and reports one that leaves it", (t) => {
  withTree(refFixture('{"skills":["./inside","./outside"]}'), (root) => {
    const outside = mkdtempSync(path.join(tmpdir(), "agentics-outside-"));
    try {
      if (!link(t, path.join(root, "skills", "tdd"), path.join(root, "inside")))
        return;
      if (!link(t, outside, path.join(root, "outside"))) return;
      assert.deepEqual(
        pathReferenceRules(root).map((f) => f.message),
        ['skills path escapes checkout: "./outside"'],
      );
    } finally {
      rmSync(outside, { recursive: true, force: true });
    }
  });
});

test("plugin-path-references throws a PartialAuditError that carries the failures found before it", (t) => {
  withTree(refFixture('{"skills":["../x","./loop"]}'), (root) => {
    if (!link(t, "loop", path.join(root, "loop"))) return;
    assert.throws(
      () => pathReferenceRules(root),
      (error) => {
        assert.ok(error instanceof PartialAuditError);
        assert.equal(
          error.message,
          "audit .claude-plugin/plugin.json skills reference: loop: ELOOP",
        );
        assert.ok(
          error.cause instanceof RepoFileError &&
            error.cause.kind === "unreadable",
        );
        assert.deepEqual(error.failures, [
          {
            rule: "plugin-path-references",
            path: CLAUDE,
            message: 'skills path escapes checkout: "../x"',
          },
        ]);
        return true;
      },
    );
  });
});

const registered = (
  manifest: string,
  skills: string[],
  extra: Record<string, string> = {},
) =>
  withTree(
    {
      [CLAUDE]: manifest,
      ...Object.fromEntries(skills.map((s) => [`skills/${s}/SKILL.md`, "x"])),
      ...extra,
    },
    (root) => registrationRules(root),
  );

test("plugin-registers-shipped names each skills directory the manifest does not register, in default sort order", () => {
  assert.deepEqual(
    registered('{"skills":["./skills/tdd"]}', ["tdd", "Zeta", "alpha"]),
    [
      {
        rule: "plugin-registers-shipped",
        path: CLAUDE,
        message:
          "skills/Zeta/ ships but no skills entry registers it — consumers never load it",
      },
      {
        rule: "plugin-registers-shipped",
        path: CLAUDE,
        message:
          "skills/alpha/ ships but no skills entry registers it — consumers never load it",
      },
    ],
  );
});

test("plugin-registers-shipped registers a skill by the last element of an entry, ignoring trailing slashes", () => {
  assert.deepEqual(
    registered('{"skills":["./skills/tdd/","other/dir/review","build//"]}', [
      "tdd",
      "review",
      "build",
    ]),
    [],
  );
  // A non-string element is the empty string, whose base is ".", so it registers no directory.
  assert.equal(registered('{"skills":[5,null,""]}', ["tdd"]).length, 1);
  // A value that is not an array registers nothing.
  assert.equal(registered('{"skills":"./skills/"}', ["tdd"]).length, 1);
  assert.equal(registered("{}", ["tdd"]).length, 1);
});

test("plugin-registers-shipped ignores files under skills, tolerates a missing skills directory and skips an unreadable manifest", () => {
  assert.deepEqual(
    registered('{"skills":[]}', [], { "skills/README.md": "x" }),
    [],
  );
  assert.deepEqual(registered('{"skills":[]}', []), []);
  for (const text of ["[]", "{", "7"])
    assert.deepEqual(registered(text, ["tdd"]), [], text);
  // A null manifest decodes as an empty object, so nothing is registered.
  assert.equal(registered("null", ["tdd"]).length, 1);
  assert.deepEqual(
    withTree({}, (root) => registrationRules(root)),
    [],
  );
});

test("plugin-registers-shipped is an audit error when skills exists but cannot be listed", () => {
  withTree({ [CLAUDE]: "{}", skills: "not a directory" }, (root) => {
    assert.throws(
      () => registrationRules(root),
      (error: unknown) =>
        error instanceof Error &&
        error.message === "enumerate skills: skills: is not a directory" &&
        error.cause instanceof RepoFileError &&
        error.cause.kind === "not-a-directory",
    );
  });
});

const hooksFailures = (files: Record<string, string>) =>
  withTree(files, (root) => claudeProviderRules(root));

test("claude-hooks-field requires the exact string, and is silent for a manifest it cannot read", () => {
  const rule = {
    rule: "claude-hooks-field",
    path: CLAUDE,
    message: "hooks must be ./hooks/hooks.claude.json",
  };
  assert.deepEqual(
    hooksFailures({ [CLAUDE]: '{"hooks":"./hooks/hooks.claude.json"}' }),
    [],
  );
  for (const text of [
    "{}",
    '{"hooks":"./hooks/hooks.codex.json"}',
    '{"hooks":5}',
    '{"hooks":["./hooks/hooks.claude.json"]}',
    '{"hooks":null}',
    "null",
  ])
    assert.deepEqual(hooksFailures({ [CLAUDE]: text }), [rule], text);
  for (const text of ["{", "[]", "7"])
    assert.deepEqual(hooksFailures({ [CLAUDE]: text }), [], text);
  assert.deepEqual(hooksFailures({}), []);
});

test("no-mcp-json flags a non-directory named .mcp.json or mcp_config.json anywhere in the walk, and not a directory of that name or one in a skipped directory", () => {
  withTree(
    {
      ".mcp.json": "{}",
      "mcp_config.json": "{}",
      "a/b/.mcp.json": "{}",
      "a/b/mcp_config.json": "{}",
      "c/.mcp.json/inner": "x",
      "c/mcp_config.json/inner": "x",
      "node_modules/.mcp.json": "{}",
      "node_modules/mcp_config.json": "{}",
      ".git/.mcp.json": "{}",
      ".git/mcp_config.json": "{}",
      ".claude/.mcp.json": "{}",
      ".claude/mcp_config.json": "{}",
      "d/not.mcp.json": "{}",
      "d/not_mcp_config.json": "{}",
    },
    (root) => {
      assert.deepEqual(noMcpRules(root, []), [
        {
          rule: "no-mcp-json",
          path: ".mcp.json",
          message: "Agentics ships no MCP configuration — remove it",
        },
        {
          rule: "no-mcp-json",
          path: "a/b/.mcp.json",
          message: "Agentics ships no MCP configuration — remove it",
        },
        {
          rule: "no-mcp-json",
          path: "a/b/mcp_config.json",
          message: "Agentics ships no MCP configuration — remove it",
        },
        {
          rule: "no-mcp-json",
          path: "mcp_config.json",
          message: "Agentics ships no MCP configuration — remove it",
        },
      ]);
    },
  );
});

test("no-mcp-servers fires on a truthy mcpServers in the listed plugin.json files only", () => {
  const truthy = [
    "true",
    '"x"',
    "1",
    "-1",
    "[1]",
    '[""]',
    '{"a":1}',
    '{"":null}',
  ];
  const falsy = ["false", '""', "0", "-0", "[]", "{}", "null"];
  const files: Record<string, string> = {};
  truthy.forEach(
    (v, i) => (files[`t${i}/plugin.json`] = `{"mcpServers":${v}}`),
  );
  falsy.forEach((v, i) => (files[`f${i}/plugin.json`] = `{"mcpServers":${v}}`));
  files["none/plugin.json"] = "{}";
  files["array/plugin.json"] = "[1]";
  files["bad/plugin.json"] = "{";
  files["unlisted/plugin.json"] = '{"mcpServers":true}';
  withTree(files, (root) => {
    const listed = Object.keys(files)
      .filter((p) => p !== "unlisted/plugin.json")
      .sort();
    const failures = noMcpRules(root, listed);
    assert.deepEqual(
      failures.map((f) => f.path).sort(),
      truthy.map((_, i) => `t${i}/plugin.json`).sort(),
    );
    for (const failure of failures) {
      assert.equal(failure.rule, "no-mcp-servers");
      assert.equal(
        failure.message,
        "mcpServers must not be registered in plugin.json",
      );
    }
  });
  // A listed file that no longer exists is skipped.
  withTree({}, (root) =>
    assert.deepEqual(noMcpRules(root, ["gone/plugin.json"]), []),
  );
});

test("no-claude-md flags each of the two paths, a directory included, in that order", () => {
  const message =
    "AGENTS.md is canonical; Claude Code reads this file instead of AGENTS.md, so remove it";
  assert.deepEqual(
    withTree({ "AGENTS.md": "x" }, (root) => claudeMdRule(root)),
    [],
  );
  assert.deepEqual(
    withTree({ "CLAUDE.md": "x" }, (root) => claudeMdRule(root)),
    [{ rule: "no-claude-md", path: "CLAUDE.md", message }],
  );
  assert.deepEqual(
    withTree({ ".claude/CLAUDE.md": "x", "CLAUDE.md/inner": "x" }, (root) =>
      claudeMdRule(root),
    ).map((f) => f.path),
    ["CLAUDE.md", ".claude/CLAUDE.md"],
  );
});

test("no-claude-md counts a dangling symlink as present", (t) => {
  withTree({ "keep.txt": "x" }, (root) => {
    if (!link(t, "nowhere", path.join(root, "CLAUDE.md"))) return;
    assert.deepEqual(
      claudeMdRule(root).map((f) => f.path),
      ["CLAUDE.md"],
    );
  });
});

const TEMP = "docs/specs/2026-11/feature-x/temp";

/** A tracked lookup over `files`, with every directory that holds one, as `trackedPaths` reports them. */
function trackedLookup(files: string[]): () => Tracked {
  const dirs = new Set<string>();
  for (const file of files)
    for (
      let dir = path.posix.dirname(file);
      dir !== ".";
      dir = path.posix.dirname(dir)
    )
      dirs.add(dir);
  return () => ({ files: new Set(files), dirs });
}

test("plugin-json-valid and no-mcp-json skip the untracked files of a work folder's temp directory and check the tracked ones", () => {
  withTree(
    {
      [`${TEMP}/plugin.json`]: "{",
      [`${TEMP}/.mcp.json`]: "{}",
      [`${TEMP}/kept/plugin.json`]: "{",
      [`${TEMP}/kept/.mcp.json`]: "{}",
      [`${TEMP}/loose/plugin.json`]: "{",
      "docs/specs/2026-11/feature-x/plugin.json": "{",
    },
    (root) => {
      const untracked = trackedLookup([]);
      assert.deepEqual(pluginJsonRules(root, untracked).found, [
        "docs/specs/2026-11/feature-x/plugin.json",
      ]);
      assert.deepEqual(noMcpRules(root, [], untracked), []);

      const tracked = trackedLookup([
        `${TEMP}/plugin.json`,
        `${TEMP}/kept/.mcp.json`,
      ]);
      const plugins = pluginJsonRules(root, tracked);
      assert.deepEqual(plugins.found, [
        "docs/specs/2026-11/feature-x/plugin.json",
        `${TEMP}/plugin.json`,
      ]);
      assert.deepEqual(
        plugins.failures.map((f) => f.path),
        plugins.found,
      );
      assert.deepEqual(
        noMcpRules(root, [], tracked).map((f) => [f.rule, f.path]),
        [["no-mcp-json", `${TEMP}/kept/.mcp.json`]],
      );
    },
  );
});

test("the walk asks for the tracked files only when it reaches a work folder's temp directory, and a lookup failure propagates", () => {
  const failing = (): never => {
    throw new Error("listing tracked files: boom");
  };
  withTree(
    { "temp/plugin.json": "{}", "docs/2026-11/x/tempo/.mcp.json": "{}" },
    (root) => {
      assert.equal(pluginJsonRules(root, failing).found.length, 1);
      assert.equal(noMcpRules(root, [], failing).length, 1);
    },
  );
  withTree({ [`${TEMP}/a.md`]: "x" }, (root) => {
    assert.throws(
      () => pluginJsonRules(root, failing),
      /listing tracked files: boom/,
    );
    assert.throws(
      () => noMcpRules(root, [], failing),
      /listing tracked files: boom/,
    );
  });
});

/**
 * Audits a passing manifests fixture after `seed` changes it, and asserts the findings the domain reports, in its
 * order, with no audit error.
 */
async function assertSeeded(
  seed: (root: string) => void,
  expected: Failure[],
): Promise<void> {
  await withManifestsRepo(async (root) => {
    seed(root);
    const result = await auditManifests(root);
    assert.equal(result.error, undefined);
    assert.deepEqual(result.failures, expected);
  });
}

test("plugin-json-valid reports a plugin.json the walk finds that is not JSON", async () => {
  await assertSeeded(
    (root) => {
      BREAKERS["plugin-json-valid"]!(root);
    },
    [
      {
        rule: "plugin-json-valid",
        path: "docs/plugin.json",
        message: "invalid JSON",
      },
    ],
  );
});

test("plugin-path-references reports a Codex skills path that is missing", async () => {
  await assertSeeded(
    (root) => {
      BREAKERS["plugin-path-references"]!(root);
    },
    [
      {
        rule: "plugin-path-references",
        path: CODEX_PLUGIN,
        message:
          "skills path not found → ./nope/ (string form must be a directory)",
      },
    ],
  );
});

test("plugin-registers-shipped reports a skills directory the Claude manifest does not register", async () => {
  await assertSeeded(
    (root) => {
      BREAKERS["plugin-registers-shipped"]!(root);
    },
    [
      {
        rule: "plugin-registers-shipped",
        path: CLAUDE_PLUGIN,
        message:
          "skills/extra/ ships but no skills entry registers it — consumers never load it",
      },
    ],
  );
});

test("claude-hooks-field reports a Claude manifest whose hooks field names another provider's file", async () => {
  await assertSeeded(
    (root) => {
      BREAKERS["claude-hooks-field"]!(root);
    },
    [
      {
        rule: "claude-hooks-field",
        path: CLAUDE_PLUGIN,
        message: "hooks must be ./hooks/hooks.claude.json",
      },
    ],
  );
});

test("no-mcp-json reports a .mcp.json at the checkout root", async () => {
  await assertSeeded(
    (root) => {
      BREAKERS["no-mcp-json"]!(root);
    },
    [
      {
        rule: "no-mcp-json",
        path: ".mcp.json",
        message: "Agentics ships no MCP configuration — remove it",
      },
    ],
  );
});

test("no-mcp-servers reports a plugin.json the walk finds with an mcpServers object", async () => {
  await assertSeeded(
    (root) => {
      BREAKERS["no-mcp-servers"]!(root);
    },
    [
      {
        rule: "no-mcp-servers",
        path: "docs/plugin.json",
        message: "mcpServers must not be registered in plugin.json",
      },
    ],
  );
});

test("no-claude-md reports a CLAUDE.md at the checkout root", async () => {
  await assertSeeded(
    (root) => {
      BREAKERS["no-claude-md"]!(root);
    },
    [
      {
        rule: "no-claude-md",
        path: "CLAUDE.md",
        message:
          "AGENTS.md is canonical; Claude Code reads this file instead of AGENTS.md, so remove it",
      },
    ],
  );
});

test("a missing Copilot hooks file fails its path reference before the Copilot hooks target", async () => {
  await assertSeeded(
    (root) => {
      BREAKERS["copilot-hooks-target"]!(root);
    },
    [
      {
        rule: "plugin-path-references",
        path: COPILOT_PLUGIN,
        message: "hooks path not found → ./hooks/hooks.copilot.json",
      },
      {
        rule: "copilot-hooks-target",
        path: COPILOT_HOOKS,
        message: "effective hooks target does not exist",
      },
    ],
  );
});

// JSON.parse has no nesting limit, so depth alone never makes a plugin.json invalid.
test("a plugin.json nested 10,001 deep", async () => {
  await assertSeeded((root) => {
    writeFile(root, "docs/plugin.json", nest(10001));
  }, []);
});

test("a plugin.json that is empty", async () => {
  await assertSeeded(
    (root) => {
      writeFile(root, "docs/plugin.json", "");
    },
    [
      {
        rule: "plugin-json-valid",
        path: "docs/plugin.json",
        message: "invalid JSON",
      },
    ],
  );
});

test("a plugin.json with a byte order mark", async () => {
  await assertSeeded(
    (root) => {
      writeFile(root, "docs/plugin.json", `${String.fromCharCode(0xfeff)}{}`);
    },
    [
      {
        rule: "plugin-json-valid",
        path: "docs/plugin.json",
        message: "invalid JSON",
      },
    ],
  );
});

test("a plugin.json with trailing text", async () => {
  await assertSeeded(
    (root) => {
      writeFile(root, "docs/plugin.json", "{} x");
    },
    [
      {
        rule: "plugin-json-valid",
        path: "docs/plugin.json",
        message: "invalid JSON",
      },
    ],
  );
});

// JSON.parse reads a number too large for a double as Infinity rather than rejecting it, so these manifests are valid
// JSON and only a version mismatch is reported.
test("a version that overflows float64", async () => {
  await assertSeeded(
    (root) => {
      versions(root, "1.5", "1e400");
    },
    [
      {
        rule: "copilot-marketplace-version-parity",
        path: COPILOT_PLUGIN,
        message:
          "effective manifest version must match Copilot marketplace entry",
      },
      {
        rule: "provider-version-parity",
        path: COPILOT_PLUGIN,
        message: 'version "" != .claude-plugin/plugin.json version "1.0.0"',
      },
    ],
  );
});

test("an overflowing number in another member of a plugin manifest", async () => {
  await assertSeeded((root) => {
    writeFile(
      root,
      CODEX_PLUGIN,
      '{"name":"agentics","version":"1.0.0","x":1e400,"skills":"./skills/","hooks":"./hooks/hooks.codex.json"}',
    );
  }, []);
});

test("an overflowing member next to a mismatched version", async () => {
  await assertSeeded(
    (root) => {
      writeFile(
        root,
        CODEX_PLUGIN,
        '{"name":"agentics","version":"2.0.0","x":1e400,"skills":"./skills/","hooks":"./hooks/hooks.codex.json"}',
      );
    },
    [
      {
        rule: "provider-version-parity",
        path: CODEX_PLUGIN,
        message:
          'version "2.0.0" != .claude-plugin/plugin.json version "1.0.0"',
      },
    ],
  );
});

test("a skills entry that escapes", async () => {
  await assertSeeded(
    (root) => {
      editCodex(root, (v) => (v.skills = ["../x"]));
    },
    [
      {
        rule: "plugin-path-references",
        path: CODEX_PLUGIN,
        message: 'skills path escapes checkout: "../x"',
      },
    ],
  );
});

test("a skills entry that is an absolute path", async () => {
  await assertSeeded(
    (root) => {
      editCodex(root, (v) => (v.skills = ["/etc"]));
    },
    [
      {
        rule: "plugin-path-references",
        path: CODEX_PLUGIN,
        message: 'skills path escapes checkout: "/etc"',
      },
    ],
  );
});

test("a skills value in string form that is a file", async () => {
  await assertSeeded(
    (root) => {
      editCodex(root, (v) => (v.skills = "./AGENTS.md"));
    },
    [
      {
        rule: "plugin-path-references",
        path: CODEX_PLUGIN,
        message:
          "skills path not found → ./AGENTS.md (string form must be a directory)",
      },
    ],
  );
});

test("a skills entry that is not a string", async () => {
  await assertSeeded(
    (root) => {
      editCodex(root, (v) => (v.skills = [1]));
    },
    [
      {
        rule: "plugin-path-references",
        path: CODEX_PLUGIN,
        message: 'skills path escapes checkout: ""',
      },
    ],
  );
});

test("an apps entry that is missing", async () => {
  await assertSeeded(
    (root) => {
      editCodex(root, (v) => (v.apps = ["./apps"]));
    },
    [
      {
        rule: "plugin-path-references",
        path: CODEX_PLUGIN,
        message: "apps path not found → ./apps",
      },
    ],
  );
});

test("a hooks value that is a directory", async () => {
  await assertSeeded(
    (root) => {
      editCodex(root, (v) => (v.hooks = "./skills"));
    },
    [
      {
        rule: "plugin-path-references",
        path: CODEX_PLUGIN,
        message: "hooks path not found → ./skills",
      },
    ],
  );
});

test("a hooks value that is missing", async () => {
  await assertSeeded(
    (root) => {
      editCodex(root, (v) => (v.hooks = "./hooks/none.json"));
    },
    [
      {
        rule: "plugin-path-references",
        path: CODEX_PLUGIN,
        message: "hooks path not found → ./hooks/none.json",
      },
    ],
  );
});

test("a hooks value that is empty", async () => {
  await assertSeeded(
    (root) => {
      editCodex(root, (v) => (v.hooks = ""));
    },
    [
      {
        rule: "plugin-path-references",
        path: CODEX_PLUGIN,
        message: 'hooks path escapes checkout: ""',
      },
    ],
  );
});

test("an mcpServers path that is a file", async () => {
  await assertSeeded(
    (root) => {
      editCodex(root, (v) => (v.mcpServers = "./AGENTS.md"));
    },
    [
      {
        rule: "no-mcp-servers",
        path: CODEX_PLUGIN,
        message: "mcpServers must not be registered in plugin.json",
      },
    ],
  );
});

test("an mcpServers path that is a directory", async () => {
  await assertSeeded(
    (root) => {
      editCodex(root, (v) => (v.mcpServers = "./skills"));
    },
    [
      {
        rule: "plugin-path-references",
        path: CODEX_PLUGIN,
        message: "mcpServers path not found → ./skills",
      },
      {
        rule: "no-mcp-servers",
        path: CODEX_PLUGIN,
        message: "mcpServers must not be registered in plugin.json",
      },
    ],
  );
});

test("an mcpServers object", async () => {
  await assertSeeded(
    (root) => {
      editCodex(root, (v) => (v.mcpServers = {}));
    },
    [
      {
        rule: "plugin-path-references",
        path: CODEX_PLUGIN,
        message: 'mcpServers path escapes checkout: ""',
      },
    ],
  );
});

test("an mcpServers list of files", async () => {
  await assertSeeded(
    (root) => {
      editCodex(root, (v) => (v.mcpServers = ["./AGENTS.md"]));
    },
    [
      {
        rule: "no-mcp-servers",
        path: CODEX_PLUGIN,
        message: "mcpServers must not be registered in plugin.json",
      },
    ],
  );
});

test("mcpServers truthiness: true, 1, a string, a list, an object", async () => {
  await assertSeeded(
    (root) => {
      writeFile(root, "a/plugin.json", '{"mcpServers":true}');
      writeFile(root, "b/plugin.json", '{"mcpServers":1}');
      writeFile(root, "c/plugin.json", '{"mcpServers":"x"}');
      writeFile(root, "d/plugin.json", '{"mcpServers":[0]}');
      writeFile(root, "e/plugin.json", '{"mcpServers":{"a":1}}');
    },
    [
      {
        rule: "no-mcp-servers",
        path: "a/plugin.json",
        message: "mcpServers must not be registered in plugin.json",
      },
      {
        rule: "no-mcp-servers",
        path: "b/plugin.json",
        message: "mcpServers must not be registered in plugin.json",
      },
      {
        rule: "no-mcp-servers",
        path: "c/plugin.json",
        message: "mcpServers must not be registered in plugin.json",
      },
      {
        rule: "no-mcp-servers",
        path: "d/plugin.json",
        message: "mcpServers must not be registered in plugin.json",
      },
      {
        rule: "no-mcp-servers",
        path: "e/plugin.json",
        message: "mcpServers must not be registered in plugin.json",
      },
    ],
  );
});

test("a skills directory registered by a non-string entry", async () => {
  await assertSeeded(
    (root) => {
      editJson(root, CLAUDE_PLUGIN, (v) => (v.skills = ["./skills/tdd", 5]));
      mkdir(root, "skills/x");
    },
    [
      {
        rule: "plugin-path-references",
        path: CLAUDE_PLUGIN,
        message: 'skills path escapes checkout: ""',
      },
      {
        rule: "plugin-registers-shipped",
        path: CLAUDE_PLUGIN,
        message:
          "skills/x/ ships but no skills entry registers it — consumers never load it",
      },
      {
        rule: "copilot-skills-parity",
        path: COPILOT_PLUGIN,
        message: "effective skills must match Claude skills",
      },
    ],
  );
});

test("skills is removed so the listing returns no entries", async () => {
  await assertSeeded(
    (root) => {
      remove(root, "skills");
    },
    [
      {
        rule: "plugin-path-references",
        path: CLAUDE_PLUGIN,
        message: "skills path not found → ./skills/tdd",
      },
      {
        rule: "plugin-path-references",
        path: CODEX_PLUGIN,
        message:
          "skills path not found → ./skills/ (string form must be a directory)",
      },
      {
        rule: "plugin-path-references",
        path: COPILOT_PLUGIN,
        message: "skills path not found → ./skills/tdd",
      },
    ],
  );
});

test("a skills entry that is a link outside the checkout", async (t) => {
  if (!canSymlink()) return t.skip(SYMLINK_SKIP);
  await assertSeeded(
    (root) => {
      linkInFixture(root, path.dirname(root), "skills/out", "dir");
      editCodex(root, (v) => (v.skills = ["./skills/out"]));
    },
    [
      {
        rule: "plugin-path-references",
        path: CODEX_PLUGIN,
        message: 'skills path escapes checkout: "./skills/out"',
      },
    ],
  );
});

test("a dangling plugin.json link", async (t) => {
  if (!canSymlink()) return t.skip(SYMLINK_SKIP);
  await assertSeeded(
    (root) => {
      linkInFixture(root, "missing.json", "docs/plugin.json", "file");
    },
    [
      {
        rule: "plugin-json-valid",
        path: "docs/plugin.json",
        message: "invalid JSON",
      },
    ],
  );
});

test("a dangling CLAUDE.md link", async (t) => {
  if (!canSymlink()) return t.skip(SYMLINK_SKIP);
  await assertSeeded(
    (root) => {
      linkInFixture(root, "missing.md", "CLAUDE.md", "file");
    },
    [
      {
        rule: "no-claude-md",
        path: "CLAUDE.md",
        message:
          "AGENTS.md is canonical; Claude Code reads this file instead of AGENTS.md, so remove it",
      },
    ],
  );
});

test("a .mcp.json link", async (t) => {
  if (!canSymlink()) return t.skip(SYMLINK_SKIP);
  await assertSeeded(
    (root) => {
      linkInFixture(root, "AGENTS.md", ".mcp.json", "file");
    },
    [
      {
        rule: "no-mcp-json",
        path: ".mcp.json",
        message: "Agentics ships no MCP configuration — remove it",
      },
    ],
  );
});

test("a hooks target that is a directory", async () => {
  await assertSeeded(
    (root) => {
      remove(root, COPILOT_HOOKS);
      mkdir(root, COPILOT_HOOKS);
    },
    [
      {
        rule: "plugin-path-references",
        path: COPILOT_PLUGIN,
        message: "hooks path not found → ./hooks/hooks.copilot.json",
      },
      {
        rule: "copilot-hooks-target",
        path: COPILOT_HOOKS,
        message: "effective hooks target does not exist",
      },
    ],
  );
});

test("a hooks target stops the Copilot rules before the skills parity", async () => {
  await assertSeeded(
    (root) => {
      remove(root, COPILOT_HOOKS);
      editJson(root, COPILOT_PLUGIN, (v) => (v.skills = []));
    },
    [
      {
        rule: "plugin-path-references",
        path: COPILOT_PLUGIN,
        message: "hooks path not found → ./hooks/hooks.copilot.json",
      },
      {
        rule: "copilot-hooks-target",
        path: COPILOT_HOOKS,
        message: "effective hooks target does not exist",
      },
    ],
  );
});

test("a skills string against none", async () => {
  await assertSeeded(
    (root) => {
      editJson(root, CLAUDE_PLUGIN, (v) => (v.skills = "./skills/"));
      editJson(root, COPILOT_PLUGIN, (v) => delete v.skills);
    },
    [
      {
        rule: "plugin-registers-shipped",
        path: CLAUDE_PLUGIN,
        message:
          "skills/tdd/ ships but no skills entry registers it — consumers never load it",
      },
    ],
  );
});

test("skills lists equal up to number form", async () => {
  await assertSeeded(
    (root) => {
      const text = (n: string): string =>
        `{"name":"agentics","version":"1.0.0","skills":[${n},"./skills/tdd"],"hooks":"./hooks/hooks.`;
      writeFile(root, CLAUDE_PLUGIN, `${text("1.0")}claude.json"}`);
      writeFile(root, COPILOT_PLUGIN, `${text("1")}copilot.json"}`);
    },
    [
      {
        rule: "plugin-path-references",
        path: CLAUDE_PLUGIN,
        message: 'skills path escapes checkout: ""',
      },
      {
        rule: "plugin-path-references",
        path: COPILOT_PLUGIN,
        message: 'skills path escapes checkout: ""',
      },
    ],
  );
});

test("a dangling link at .github/plugin/plugin.json leaves the Claude manifest effective", async (t) => {
  if (!canSymlink()) return t.skip(SYMLINK_SKIP);
  await assertSeeded(
    (root) => {
      remove(root, COPILOT_PLUGIN);
      linkInFixture(root, "missing.json", COPILOT_PLUGIN, "file");
    },
    [
      {
        rule: "plugin-json-valid",
        path: COPILOT_PLUGIN,
        message: "invalid JSON",
      },
      {
        rule: "copilot-effective-manifest",
        path: CLAUDE_PLUGIN,
        message:
          "effective Copilot manifest must be .github/plugin/plugin.json",
      },
      {
        rule: "provider-version-parity",
        path: COPILOT_PLUGIN,
        message: "is missing",
      },
    ],
  );
});

test("a dangling link at plugin.json is still a shadow manifest", async (t) => {
  if (!canSymlink()) return t.skip(SYMLINK_SKIP);
  await assertSeeded(
    (root) => {
      linkInFixture(root, "missing.json", "plugin.json", "file");
    },
    [
      {
        rule: "plugin-json-valid",
        path: "plugin.json",
        message: "invalid JSON",
      },
      {
        rule: "copilot-shadow-manifest",
        path: "plugin.json",
        message:
          "higher-precedence Copilot shadow manifest plugin.json must be absent",
      },
    ],
  );
});

test("a hit under .worktrees is walked", async () => {
  await assertSeeded(
    (root) => {
      writeFile(root, ".worktrees/.mcp.json", "{}");
      writeFile(root, ".worktrees/plugin.json", "{");
    },
    [
      {
        rule: "plugin-json-valid",
        path: ".worktrees/plugin.json",
        message: "invalid JSON",
      },
      {
        rule: "no-mcp-json",
        path: ".worktrees/.mcp.json",
        message: "Agentics ships no MCP configuration — remove it",
      },
    ],
  );
});

test("a hit under docs/handoffs is walked", async () => {
  await assertSeeded(
    (root) => {
      writeFile(root, "docs/handoffs/.mcp.json", "{}");
      writeFile(root, "docs/handoffs/plugin.json", "{");
    },
    [
      {
        rule: "plugin-json-valid",
        path: "docs/handoffs/plugin.json",
        message: "invalid JSON",
      },
      {
        rule: "no-mcp-json",
        path: "docs/handoffs/.mcp.json",
        message: "Agentics ships no MCP configuration — remove it",
      },
    ],
  );
});

test("a hit under docs/probes is walked", async () => {
  await assertSeeded(
    (root) => {
      writeFile(root, "docs/probes/.mcp.json", "{}");
      writeFile(root, "docs/probes/plugin.json", "{");
    },
    [
      {
        rule: "plugin-json-valid",
        path: "docs/probes/plugin.json",
        message: "invalid JSON",
      },
      {
        rule: "no-mcp-json",
        path: "docs/probes/.mcp.json",
        message: "Agentics ships no MCP configuration — remove it",
      },
    ],
  );
});

test("a hit under evals/runs is walked", async () => {
  await assertSeeded(
    (root) => {
      writeFile(root, "evals/runs/.mcp.json", "{}");
      writeFile(root, "evals/runs/plugin.json", "{");
    },
    [
      {
        rule: "plugin-json-valid",
        path: "evals/runs/plugin.json",
        message: "invalid JSON",
      },
      {
        rule: "no-mcp-json",
        path: "evals/runs/.mcp.json",
        message: "Agentics ships no MCP configuration — remove it",
      },
    ],
  );
});

test("a hit under .agentics is walked", async () => {
  await assertSeeded(
    (root) => {
      writeFile(root, ".agentics/.mcp.json", "{}");
      writeFile(root, ".agentics/plugin.json", "{");
    },
    [
      {
        rule: "plugin-json-valid",
        path: ".agentics/plugin.json",
        message: "invalid JSON",
      },
      {
        rule: "no-mcp-json",
        path: ".agentics/.mcp.json",
        message: "Agentics ships no MCP configuration — remove it",
      },
    ],
  );
});

test("a hit under .audit is walked", async () => {
  await assertSeeded(
    (root) => {
      writeFile(root, ".audit/.mcp.json", "{}");
      writeFile(root, ".audit/plugin.json", "{");
    },
    [
      {
        rule: "plugin-json-valid",
        path: ".audit/plugin.json",
        message: "invalid JSON",
      },
      {
        rule: "no-mcp-json",
        path: ".audit/.mcp.json",
        message: "Agentics ships no MCP configuration — remove it",
      },
    ],
  );
});

test("a hit under node_modules2 is walked", async () => {
  await assertSeeded(
    (root) => {
      writeFile(root, "node_modules2/.mcp.json", "{}");
      writeFile(root, "node_modules2/plugin.json", "{");
    },
    [
      {
        rule: "plugin-json-valid",
        path: "node_modules2/plugin.json",
        message: "invalid JSON",
      },
      {
        rule: "no-mcp-json",
        path: "node_modules2/.mcp.json",
        message: "Agentics ships no MCP configuration — remove it",
      },
    ],
  );
});

test("a hit under .gitx is walked", async () => {
  await assertSeeded(
    (root) => {
      writeFile(root, ".gitx/.mcp.json", "{}");
      writeFile(root, ".gitx/plugin.json", "{");
    },
    [
      {
        rule: "plugin-json-valid",
        path: ".gitx/plugin.json",
        message: "invalid JSON",
      },
      {
        rule: "no-mcp-json",
        path: ".gitx/.mcp.json",
        message: "Agentics ships no MCP configuration — remove it",
      },
    ],
  );
});

test("a hit under claude is walked", async () => {
  await assertSeeded(
    (root) => {
      writeFile(root, "claude/.mcp.json", "{}");
      writeFile(root, "claude/plugin.json", "{");
    },
    [
      {
        rule: "plugin-json-valid",
        path: "claude/plugin.json",
        message: "invalid JSON",
      },
      {
        rule: "no-mcp-json",
        path: "claude/.mcp.json",
        message: "Agentics ships no MCP configuration — remove it",
      },
    ],
  );
});

test("a CLAUDE.md under .claude", async () => {
  await assertSeeded(
    (root) => {
      writeFile(root, ".claude/CLAUDE.md", "x");
    },
    [
      {
        rule: "no-claude-md",
        path: ".claude/CLAUDE.md",
        message:
          "AGENTS.md is canonical; Claude Code reads this file instead of AGENTS.md, so remove it",
      },
    ],
  );
});

test("a plugin.json anywhere with mcpServers and an invalid sibling", async () => {
  await assertSeeded(
    (root) => {
      writeFile(root, "a/plugin.json", '{"mcpServers":{"a":1}}');
      writeFile(root, "b/plugin.json", "{");
    },
    [
      {
        rule: "plugin-json-valid",
        path: "b/plugin.json",
        message: "invalid JSON",
      },
      {
        rule: "no-mcp-servers",
        path: "a/plugin.json",
        message: "mcpServers must not be registered in plugin.json",
      },
    ],
  );
});
