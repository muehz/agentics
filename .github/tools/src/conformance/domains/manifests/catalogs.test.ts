import assert from "node:assert/strict";
import {
  lstatSync,
  mkdirSync,
  mkdtempSync,
  rmSync,
  symlinkSync,
} from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { test } from "node:test";
import { RepoFileError } from "../../repo-file-error.ts";
import { writeFile } from "../../testing/files.ts";
import { catalogRules, marketplaceSourceRules } from "./catalogs.ts";
import { isJsonObject, PartialAuditError } from "./files.ts";
import {
  AGENTS_CATALOG,
  auditManifests,
  BREAKERS,
  CLAUDE_CATALOG,
  withManifestsRepo,
} from "../../testing/manifests-repo.ts";
import {
  agenticsEntry,
  claudeCatalog,
  fixturePath,
  link,
  nest,
} from "../../testing/manifests-seeds.ts";
import {
  canLinkDirectory,
  canSymlink,
  DIRECTORY_LINK_SKIP,
  removeTree,
  SYMLINK_SKIP,
} from "../../testing/host.ts";

const CLAUDE = ".claude-plugin/marketplace.json";
const AGENTS = ".agents/plugins/marketplace.json";
const AGENTICS = '{"name":"agentics","source":"./"}';

/** Runs `body` on a temp tree holding `files`, then removes it. */
function withTree<T>(
  files: Record<string, string>,
  body: (root: string) => T,
): T {
  const root = mkdtempSync(path.join(tmpdir(), "agentics-catalogs-"));
  try {
    for (const [rel, content] of Object.entries(files))
      writeFile(root, rel, content);
    return body(root);
  } finally {
    removeTree(root);
  }
}

const both = (
  claude: string,
  agents: string = `{"plugins":[${AGENTICS}]}`,
) => ({
  [CLAUDE]: claude,
  [AGENTS]: agents,
});

test("a catalog that is missing is marketplace-present, each one on its own path", () => {
  withTree({}, (root) => {
    assert.deepEqual(catalogRules(root).failures, [
      { rule: "marketplace-present", path: CLAUDE, message: "missing" },
      { rule: "marketplace-present", path: AGENTS, message: "missing" },
    ]);
  });
});

test("a catalog that is not a JSON object is marketplace-json-valid", () => {
  for (const text of ["{", "", "[]", "7", '"x"', "\ufeff{}"]) {
    withTree(both(text), (root) => {
      const { failures, parsed } = catalogRules(root);
      assert.deepEqual(
        failures,
        [
          {
            rule: "marketplace-json-valid",
            path: CLAUDE,
            message: "invalid JSON",
          },
        ],
        JSON.stringify(text),
      );
      assert.deepEqual([...parsed.keys()], [AGENTS], JSON.stringify(text));
    });
  }
});

test("a catalog without exactly one object named agentics is marketplace-entry and is left out of the parsed set", () => {
  const cases = [
    "null",
    "{}",
    '{"plugins":null}',
    '{"plugins":{}}',
    '{"plugins":[]}',
    `{"plugins":[${AGENTICS},${AGENTICS}]}`,
    `{"plugins":[${AGENTICS},{"name":"other"}]}`,
    '{"plugins":["agentics"]}',
    '{"plugins":[null]}',
    '{"plugins":[{"name":"other"}]}',
    '{"plugins":[{"name":1}]}',
    '{"plugins":[{"name":null}]}',
    '{"plugins":[{}]}',
    '{"plugins":[{"Name":"agentics"}]}',
    `{"plugins":[${AGENTICS}],"plugins":[]}`,
    '{"plugins":1e400}',
  ];
  for (const text of cases) {
    withTree(both(text), (root) => {
      const { failures, parsed } = catalogRules(root);
      assert.deepEqual(
        failures,
        [
          {
            rule: "marketplace-entry",
            path: CLAUDE,
            message: "plugins must contain exactly one object named agentics",
          },
        ],
        text,
      );
      assert.equal(parsed.has(CLAUDE), false, text);
    });
  }
});

test("a catalog with one agentics entry is parsed, and the last duplicate plugins key wins", () => {
  for (const text of [
    `{"plugins":[${AGENTICS}]}`,
    `{"plugins":[],"plugins":[${AGENTICS}]}`,
    `{"name":"muehz","metadata":{"version":"0.0.0"},"plugins":[{"name":"agentics","version":"1.0.0","source":"./"}]}`,
  ]) {
    withTree(both(text), (root) => {
      const { failures, parsed } = catalogRules(root);
      assert.deepEqual(failures, [], text);
      assert.deepEqual([...parsed.keys()], [CLAUDE, AGENTS], text);
      assert.ok(isJsonObject(parsed.get(CLAUDE)), text);
    });
  }
});

const sources = (
  claude: string,
  agents: string = `{"plugins":[${AGENTICS}]}`,
): Record<string, string> => ({
  ...both(claude, agents),
  "skills/tdd/SKILL.md": "x",
});

const sourceFailures = (files: Record<string, string>) =>
  withTree(files, (root) =>
    marketplaceSourceRules(root, catalogRules(root).parsed),
  );

test("a source that is an existing directory passes, as a string or as source.path", () => {
  assert.deepEqual(
    sourceFailures(
      sources(
        '{"plugins":[{"name":"agentics","source":"./skills"}]}',
        '{"plugins":[{"name":"agentics","source":{"path":"skills/"}}]}',
      ),
    ),
    [],
  );
});

test("a source that is empty, absent or not a string is a failure naming the plugin", () => {
  for (const source of ['""', "null", "5", "{}", '{"path":3}', '{"path":""}']) {
    const failures = sourceFailures(
      sources(`{"plugins":[{"name":"agentics","source":${source}}]}`),
    );
    assert.deepEqual(
      failures,
      [
        {
          rule: "marketplace-source-dir",
          path: CLAUDE,
          message: 'plugin "agentics" has no local source path',
        },
      ],
      source,
    );
  }
  assert.deepEqual(
    sourceFailures(sources(`{"plugins":[{"name":"agentics"}]}`)).map(
      (f) => f.message,
    ),
    ['plugin "agentics" has no local source path'],
  );
});

test("a source that is missing or is a file is 'directory not found', joined and cleaned with a slash", () => {
  const failures = sourceFailures(
    sources(
      '{"plugins":[{"name":"agentics","source":"./nope/"}]}',
      '{"plugins":[{"name":"agentics","source":"skills/tdd/SKILL.md"}]}',
    ),
  );
  assert.deepEqual(failures, [
    {
      rule: "marketplace-source-dir",
      path: CLAUDE,
      message: "nope — directory not found",
    },
    {
      rule: "marketplace-source-dir",
      path: AGENTS,
      message: "skills/tdd/SKILL.md — directory not found",
    },
  ]);
});

test("a source that escapes the checkout carries the escape text, quoted", () => {
  for (const source of ["../x", "a/../../x", "/abs"]) {
    assert.deepEqual(
      sourceFailures(
        sources(
          `{"plugins":[{"name":"agentics","source":${JSON.stringify(source)}}]}`,
        ),
      ),
      [
        {
          rule: "marketplace-source-dir",
          path: CLAUDE,
          message: `path escapes checkout: ${JSON.stringify(source)}`,
        },
      ],
      source,
    );
  }
});

test("metadata.pluginRoot applies to the Claude catalog only, and only when it is a non-empty string", () => {
  const meta = (root: unknown) =>
    `{"metadata":{"pluginRoot":${JSON.stringify(root)}},"plugins":[{"name":"agentics","source":"./tdd"}]}`;
  const agents = (root: unknown) =>
    `{"metadata":{"pluginRoot":${JSON.stringify(root)}},"plugins":[{"name":"agentics","source":"./skills"}]}`;
  // The Claude source is skills/tdd under pluginRoot "skills"; the Agents catalog ignores its own pluginRoot.
  assert.deepEqual(
    sourceFailures(sources(meta("skills"), agents("elsewhere"))),
    [],
  );
  // An empty or non-string pluginRoot leaves the root at ".", where ./tdd does not exist.
  for (const root of ["", 5, null])
    assert.deepEqual(
      sourceFailures(sources(meta(root), agents("elsewhere"))).map(
        (f) => f.message,
      ),
      ["tdd — directory not found"],
      String(root),
    );
  // A missing pluginRoot directory reports the joined path; one that escapes reports the escape.
  assert.deepEqual(
    sourceFailures(sources(meta("./pl"), agents("elsewhere"))).map(
      (f) => f.message,
    ),
    ["pl/tdd — directory not found"],
  );
  assert.deepEqual(
    sourceFailures(sources(meta("../pl"), agents("elsewhere"))).map(
      (f) => f.message,
    ),
    ['path escapes checkout: "../pl"'],
  );
});

test("an entry left out of the parsed set is not checked", () => {
  assert.deepEqual(
    sourceFailures(
      sources(
        '{"plugins":[{"name":"other","source":"./nope"}]}',
        '{"plugins":[{"name":"agentics","source":"./nope"}]}',
      ),
    ),
    [
      {
        rule: "marketplace-source-dir",
        path: AGENTS,
        message: "nope — directory not found",
      },
    ],
  );
});

const BACKSLASH = String.fromCharCode(92);

test("a backslash in a source is a separator on Windows, and the not-found message prints a slash", (t) => {
  if (process.platform !== "win32")
    return t.skip("a backslash is a literal character outside Windows");
  assert.deepEqual(
    sourceFailures(
      sources(
        `{"plugins":[{"name":"agentics","source":"skills${BACKSLASH}${BACKSLASH}nope"}]}`,
      ),
    ),
    [
      {
        rule: "marketplace-source-dir",
        path: CLAUDE,
        message: "skills/nope — directory not found",
      },
    ],
  );
});

test("an escape below a pluginRoot quotes the joined path with the host separator", (t) => {
  if (process.platform !== "win32")
    return t.skip("only Windows joins the escape path with a backslash");
  const outside = mkdtempSync(
    path.join(tmpdir(), "agentics-catalogs-outside-"),
  );
  try {
    withTree(
      sources(
        '{"metadata":{"pluginRoot":"pl"},"plugins":[{"name":"agentics","source":"link"}]}',
      ),
      (root) => {
        mkdirSync(path.join(root, "pl"));
        try {
          symlinkSync(outside, path.join(root, "pl", "link"), "junction");
        } catch {
          return t.skip("this host cannot create a directory symlink");
        }
        assert.deepEqual(
          marketplaceSourceRules(root, catalogRules(root).parsed),
          [
            {
              rule: "marketplace-source-dir",
              path: CLAUDE,
              message: `path escapes checkout: "pl${BACKSLASH}${BACKSLASH}link"`,
            },
          ],
        );
      },
    );
  } finally {
    rmSync(outside, { recursive: true, force: true });
  }
});

test("an audit error carries the failures found before it", (t) => {
  withTree(
    sources(
      '{"plugins":[{"name":"agentics","source":"../x"}]}',
      '{"plugins":[{"name":"agentics","source":"loop"}]}',
    ),
    (root) => {
      if (!canLinkDirectory()) return t.skip(DIRECTORY_LINK_SKIP);
      // A link that points at itself cannot be resolved: ELOOP is not "does not exist".
      try {
        symlinkSync("loop", path.join(root, "loop"));
      } catch {
        return t.skip("this host cannot create a symlink");
      }
      const parsed = catalogRules(root).parsed;
      assert.throws(
        () => marketplaceSourceRules(root, parsed),
        (error) => {
          assert.ok(error instanceof PartialAuditError);
          assert.equal(
            error.message,
            "audit marketplace source .agents/plugins/marketplace.json: loop: ELOOP",
          );
          assert.ok(
            error.cause instanceof RepoFileError &&
              error.cause.kind === "unreadable",
          );
          assert.deepEqual(error.failures, [
            {
              rule: "marketplace-source-dir",
              path: CLAUDE,
              message: 'path escapes checkout: "../x"',
            },
          ]);
          return true;
        },
      );
    },
  );
});

test("marketplace-present reports an agents catalog that is missing", async () => {
  await withManifestsRepo(async (root) => {
    BREAKERS["marketplace-present"]!(root);
    assert.deepEqual(await auditManifests(root), {
      failures: [
        {
          rule: "marketplace-present",
          path: ".agents/plugins/marketplace.json",
          message: "missing",
        },
      ],
    });
  });
});

test("marketplace-json-valid reports an agents catalog that is not JSON", async () => {
  await withManifestsRepo(async (root) => {
    BREAKERS["marketplace-json-valid"]!(root);
    assert.deepEqual(await auditManifests(root), {
      failures: [
        {
          rule: "marketplace-json-valid",
          path: ".agents/plugins/marketplace.json",
          message: "invalid JSON",
        },
      ],
    });
  });
});

test("marketplace-entry reports an agents catalog with no agentics entry", async () => {
  await withManifestsRepo(async (root) => {
    BREAKERS["marketplace-entry"]!(root);
    assert.deepEqual(await auditManifests(root), {
      failures: [
        {
          rule: "marketplace-entry",
          path: ".agents/plugins/marketplace.json",
          message: "plugins must contain exactly one object named agentics",
        },
      ],
    });
  });
});

test("marketplace-source-dir reports an agents catalog source directory that is missing", async () => {
  await withManifestsRepo(async (root) => {
    BREAKERS["marketplace-source-dir"]!(root);
    assert.deepEqual(await auditManifests(root), {
      failures: [
        {
          rule: "marketplace-source-dir",
          path: ".agents/plugins/marketplace.json",
          message: "missing — directory not found",
        },
      ],
    });
  });
});

test("a missing Claude catalog fails both the catalog presence rule and copilot-marketplace-present", async () => {
  await withManifestsRepo(async (root) => {
    BREAKERS["copilot-marketplace-present"]!(root);
    assert.deepEqual(await auditManifests(root), {
      failures: [
        {
          rule: "marketplace-present",
          path: ".claude-plugin/marketplace.json",
          message: "missing",
        },
        {
          rule: "copilot-marketplace-present",
          path: ".claude-plugin/marketplace.json",
          message: ".claude-plugin/marketplace.json must be present",
        },
      ],
    });
  });
});

test("a catalog entry whose name key is capitalized", async () => {
  await withManifestsRepo(async (root) => {
    writeFile(
      root,
      CLAUDE_CATALOG,
      claudeCatalog('{"Name":"agentics","version":"1.0.0","source":"./"}'),
    );
    assert.deepEqual(await auditManifests(root), {
      failures: [
        {
          rule: "marketplace-entry",
          path: ".claude-plugin/marketplace.json",
          message: "plugins must contain exactly one object named agentics",
        },
        {
          rule: "copilot-marketplace-source",
          path: ".claude-plugin/marketplace.json",
          message: "agentics marketplace source must be exactly './'",
        },
      ],
    });
  });
});

test("a catalog whose plugins key is capitalized", async () => {
  await withManifestsRepo(async (root) => {
    writeFile(
      root,
      CLAUDE_CATALOG,
      '{"PLUGINS":[{"name":"agentics","version":"2.0.0","source":"./"}]}',
    );
    assert.deepEqual(await auditManifests(root), {
      failures: [
        {
          rule: "marketplace-entry",
          path: ".claude-plugin/marketplace.json",
          message: "plugins must contain exactly one object named agentics",
        },
        {
          rule: "copilot-marketplace-source",
          path: ".claude-plugin/marketplace.json",
          message: "agentics marketplace source must be exactly './'",
        },
      ],
    });
  });
});

test("a plugins key spelled with a long s escape", async () => {
  await withManifestsRepo(async (root) => {
    writeFile(
      root,
      CLAUDE_CATALOG,
      '{"plugin\\u017f":[{"name":"agentics","version":"2.0.0","source":"./"}]}',
    );
    assert.deepEqual(await auditManifests(root), {
      failures: [
        {
          rule: "marketplace-entry",
          path: ".claude-plugin/marketplace.json",
          message: "plugins must contain exactly one object named agentics",
        },
        {
          rule: "copilot-marketplace-source",
          path: ".claude-plugin/marketplace.json",
          message: "agentics marketplace source must be exactly './'",
        },
      ],
    });
  });
});

test("a plugins key repeated, the empty list last", async () => {
  await withManifestsRepo(async (root) => {
    writeFile(
      root,
      CLAUDE_CATALOG,
      '{"plugins":[{"name":"agentics","version":"1.0.0","source":"./"}],"plugins":[]}',
    );
    assert.deepEqual(await auditManifests(root), {
      failures: [
        {
          rule: "marketplace-entry",
          path: ".claude-plugin/marketplace.json",
          message: "plugins must contain exactly one object named agentics",
        },
        {
          rule: "copilot-marketplace-source",
          path: ".claude-plugin/marketplace.json",
          message: "agentics marketplace source must be exactly './'",
        },
      ],
    });
  });
});

// A number too large for a double is still valid JSON, so the catalog parses and its entry is checked as usual.
test("an overflowing number in a catalog", async () => {
  await withManifestsRepo(async (root) => {
    writeFile(
      root,
      AGENTS_CATALOG,
      '{"plugins":[{"name":"agentics","source":{"path":"."},"x":1e400}]}',
    );
    assert.deepEqual(await auditManifests(root), { failures: [] });
  });
});

// The JSON reader sets no nesting limit, so a deep member is valid JSON and the catalog passes.
test("a catalog nested 10,001 deep", async () => {
  await withManifestsRepo(async (root) => {
    writeFile(
      root,
      AGENTS_CATALOG,
      `{"plugins":[{"name":"agentics","source":".","x":${nest(9998)}}]}`,
    );
    assert.deepEqual(await auditManifests(root), { failures: [] });
  });
});

test("the Claude catalog is null", async () => {
  await withManifestsRepo(async (root) => {
    writeFile(root, CLAUDE_CATALOG, "null");
    assert.deepEqual(await auditManifests(root), {
      failures: [
        {
          rule: "marketplace-entry",
          path: ".claude-plugin/marketplace.json",
          message: "plugins must contain exactly one object named agentics",
        },
        {
          rule: "copilot-marketplace-source",
          path: ".claude-plugin/marketplace.json",
          message: "agentics marketplace source must be exactly './'",
        },
      ],
    });
  });
});

test("the agents catalog is an array", async () => {
  await withManifestsRepo(async (root) => {
    writeFile(root, AGENTS_CATALOG, "[]");
    assert.deepEqual(await auditManifests(root), {
      failures: [
        {
          rule: "marketplace-json-valid",
          path: ".agents/plugins/marketplace.json",
          message: "invalid JSON",
        },
      ],
    });
  });
});

test("a catalog with two agentics entries", async () => {
  await withManifestsRepo(async (root) => {
    writeFile(
      root,
      CLAUDE_CATALOG,
      claudeCatalog(
        `${agenticsEntry('"version":"1.0.0",')},${agenticsEntry('"version":"1.0.0",')}`,
      ),
    );
    assert.deepEqual(await auditManifests(root), {
      failures: [
        {
          rule: "marketplace-entry",
          path: ".claude-plugin/marketplace.json",
          message: "plugins must contain exactly one object named agentics",
        },
        {
          rule: "copilot-marketplace-source",
          path: ".claude-plugin/marketplace.json",
          message: "agentics marketplace source must be exactly './'",
        },
      ],
    });
  });
});

test("a catalog with an extra entry", async () => {
  await withManifestsRepo(async (root) => {
    writeFile(
      root,
      CLAUDE_CATALOG,
      claudeCatalog(
        `${agenticsEntry('"version":"1.0.0",')},{"name":"other","source":"./"}`,
      ),
    );
    assert.deepEqual(await auditManifests(root), {
      failures: [
        {
          rule: "marketplace-entry",
          path: ".claude-plugin/marketplace.json",
          message: "plugins must contain exactly one object named agentics",
        },
      ],
    });
  });
});

test("a source object without a path", async () => {
  await withManifestsRepo(async (root) => {
    writeFile(
      root,
      AGENTS_CATALOG,
      '{"plugins":[{"name":"agentics","source":{"url":"x"}}]}',
    );
    assert.deepEqual(await auditManifests(root), {
      failures: [
        {
          rule: "marketplace-source-dir",
          path: ".agents/plugins/marketplace.json",
          message: 'plugin "agentics" has no local source path',
        },
      ],
    });
  });
});

test("a source that escapes the checkout", async () => {
  await withManifestsRepo(async (root) => {
    writeFile(
      root,
      AGENTS_CATALOG,
      '{"plugins":[{"name":"agentics","source":"../x"}]}',
    );
    assert.deepEqual(await auditManifests(root), {
      failures: [
        {
          rule: "marketplace-source-dir",
          path: ".agents/plugins/marketplace.json",
          message: 'path escapes checkout: "../x"',
        },
      ],
    });
  });
});

test("a catalog source that names a regular file", async () => {
  await withManifestsRepo(async (root) => {
    assert.ok(
      lstatSync(fixturePath(root, "AGENTS.md")).isFile(),
      "AGENTS.md is a file",
    );
    writeFile(
      root,
      AGENTS_CATALOG,
      '{"plugins":[{"name":"agentics","source":"./AGENTS.md"}]}',
    );
    assert.deepEqual(await auditManifests(root), {
      failures: [
        {
          rule: "marketplace-source-dir",
          path: ".agents/plugins/marketplace.json",
          message: "AGENTS.md — directory not found",
        },
      ],
    });
  });
});

test("a catalog source that is a link outside the checkout", async (t) => {
  if (!canSymlink()) return t.skip(SYMLINK_SKIP);
  await withManifestsRepo(async (root) => {
    link(root, path.dirname(root), "alias", "dir");
    writeFile(
      root,
      AGENTS_CATALOG,
      '{"plugins":[{"name":"agentics","source":"./alias"}]}',
    );
    assert.deepEqual(await auditManifests(root), {
      failures: [
        {
          rule: "marketplace-source-dir",
          path: ".agents/plugins/marketplace.json",
          message: 'path escapes checkout: "alias"',
        },
      ],
    });
  });
});

test("a catalog entry named Agentics", async () => {
  await withManifestsRepo(async (root) => {
    writeFile(
      root,
      CLAUDE_CATALOG,
      claudeCatalog('{"name":"Agentics","version":"1.0.0","source":"./"}'),
    );
    assert.deepEqual(await auditManifests(root), {
      failures: [
        {
          rule: "marketplace-entry",
          path: ".claude-plugin/marketplace.json",
          message: "plugins must contain exactly one object named agentics",
        },
        {
          rule: "copilot-marketplace-source",
          path: ".claude-plugin/marketplace.json",
          message: "agentics marketplace source must be exactly './'",
        },
      ],
    });
  });
});
