import assert from "node:assert/strict";
import path from "node:path";
import { test } from "node:test";
import type { ValueResult } from "../../../release/version.ts";
import {
  type AuditResult,
  type Failure,
  type RunOptions,
} from "../../check.ts";
import {
  AGENTS_CATALOG,
  ANTIGRAVITY_PLUGIN,
  auditManifests,
  BREAKERS,
  CLAUDE_CATALOG,
  CLAUDE_PLUGIN,
  CODEX_PLUGIN,
  COPILOT_PLUGIN,
  editJson,
  mkdir,
  remove,
  withManifestsRepo as withCheckout,
  writeFile,
} from "../../testing/manifests-repo.ts";
import {
  asDirectory,
  agenticsEntry,
  claudeCatalog,
  editCodex,
  link,
  nest,
} from "../../testing/manifests-seeds.ts";
import {
  initCheckout,
  tempIsInsideRepository,
} from "../../testing/git-checkout.ts";
import { PartialAuditError, type JsonObject } from "./files.ts";
import { run, STEPS, type Steps } from "./index.ts";
import {
  canSymlink,
  reportsNotADirectory,
  SYMLINK_SKIP,
  withOutside,
} from "../../testing/host.ts";

const options: RunOptions = {
  short: false,
  fast: false,
  diagnostics: () => {},
  env: process.env,
};

const failure = (rule: string): Failure => ({ rule, path: "", message: rule });
const rulesOf = (failures: Failure[]): string[] =>
  failures.map((found) => found.rule);

test("the steps report in order: catalogs, plugin.json, sources, paths, registration, hooks, Copilot, MCP, parity, CLAUDE.md", async () => {
  await withCheckout(async (root) => {
    writeFile(root, ".agents/plugins/marketplace.json", '{"plugins":[]}');
    writeFile(root, "docs/plugin.json", "{");
    editJson(root, ".claude-plugin/marketplace.json", (value) => {
      (value.plugins as Array<Record<string, unknown>>)[0]!.source = "missing";
    });
    editJson(root, ".codex-plugin/plugin.json", (value) => {
      value.version = "2.0.0";
      value.skills = "./nope/";
    });
    mkdir(root, "skills/extra");
    editJson(root, ".claude-plugin/plugin.json", (value) => {
      value.hooks = "./hooks/hooks.codex.json";
    });
    writeFile(root, ".mcp.json", "{}");
    writeFile(root, "CLAUDE.md", "x");
    const result = await run(root, options);
    assert.equal(result.error, undefined);
    assert.deepEqual(rulesOf(result.failures), [
      "marketplace-entry",
      "plugin-json-valid",
      "marketplace-source-dir",
      "plugin-path-references",
      "plugin-registers-shipped",
      "claude-hooks-field",
      "copilot-marketplace-source",
      "no-mcp-json",
      "provider-version-parity",
      "no-claude-md",
    ]);
  });
});

test(
  "an audit error in the registration step keeps the earlier failures and drops the later steps",
  {
    skip: reportsNotADirectory()
      ? "this host reports a path below a file as ENOTDIR, so the path-reference step fails first; the injected-step tests below cover the same survival rule"
      : false,
  },
  async () => {
    await withCheckout(async (root) => {
      writeFile(root, "docs/plugin.json", "{");
      remove(root, "skills");
      writeFile(root, "skills", "x");
      writeFile(root, "CLAUDE.md", "x");
      const result = await run(root, options);
      assert.deepEqual(rulesOf(result.failures), [
        "plugin-json-valid",
        "plugin-path-references",
        "plugin-path-references",
        "plugin-path-references",
      ]);
      assert.match(result.error ?? "", /^enumerate skills: /);
    });
  },
);

/** Every step returns one failure named for it, so a test reads which steps ran and which failures survived. */
function fakeSteps(overrides: Partial<Steps> = {}): Steps {
  return {
    readValues: () => [],
    catalogRules: () => ({ parsed: new Map(), failures: [failure("s2")] }),
    pluginJsonRules: () => ({ found: [], failures: [failure("s3")] }),
    marketplaceSourceRules: () => [failure("s4")],
    pathReferenceRules: () => [failure("s5-paths")],
    registrationRules: () => [failure("s5-registration")],
    claudeProviderRules: () => [failure("s5-claude")],
    copilotProviderRules: () => [failure("s5-copilot")],
    noMcpRules: () => [failure("s6")],
    parityRules: () => [failure("s7")],
    claudeMdRule: () => [failure("s8")],
    ...overrides,
  };
}

const boom = (): never => {
  throw new Error("boom");
};
const partial = (): never => {
  throw new PartialAuditError("boom", [failure("partial")]);
};

test("with no error every step's failures are reported in step order", async () => {
  const result = await run("/r", options, fakeSteps());
  assert.deepEqual(result, {
    failures: [
      failure("s2"),
      failure("s3"),
      failure("s4"),
      failure("s5-paths"),
      failure("s5-registration"),
      failure("s5-claude"),
      failure("s5-copilot"),
      failure("s6"),
      failure("s7"),
      failure("s8"),
    ],
  });
});

test("an audit error keeps the failures the step order says it keeps", async () => {
  const cases: Array<[string, Partial<Steps>, string[]]> = [
    ["the catalog step drops all", { catalogRules: boom }, []],
    ["the plugin.json step drops all", { pluginJsonRules: boom }, []],
    [
      "the source step keeps the earlier failures",
      { marketplaceSourceRules: boom },
      ["s2", "s3"],
    ],
    [
      "the source step adds its own partial list",
      { marketplaceSourceRules: partial },
      ["s2", "s3", "partial"],
    ],
    [
      "the path step keeps the earlier failures",
      { pathReferenceRules: boom },
      ["s2", "s3", "s4"],
    ],
    [
      "the path step adds its own partial list",
      { pathReferenceRules: partial },
      ["s2", "s3", "s4", "partial"],
    ],
    [
      "the registration step keeps the path failures",
      { registrationRules: boom },
      ["s2", "s3", "s4", "s5-paths"],
    ],
    [
      "the Claude hooks step keeps the registration failures",
      { claudeProviderRules: boom },
      ["s2", "s3", "s4", "s5-paths", "s5-registration"],
    ],
    [
      "the Copilot step keeps the Claude hooks failures",
      { copilotProviderRules: boom },
      ["s2", "s3", "s4", "s5-paths", "s5-registration", "s5-claude"],
    ],
    ["the MCP step drops all", { noMcpRules: boom }, []],
    ["the parity step drops all", { parityRules: boom }, []],
  ];
  for (const [name, overrides, kept] of cases) {
    const result = await run("/r", options, fakeSteps(overrides));
    assert.deepEqual(rulesOf(result.failures), kept, name);
    assert.equal(result.error, "boom", name);
  }
});

test("a version file that cannot be read is an audit error with no failures, and a missing one is not", async () => {
  const value = (missing: boolean): ValueResult => ({
    path: "p.json",
    raw: "",
    kind: "read",
    error: "p.json: open p.json: gone",
    value: { set: false, json: "" },
    missing,
  });
  const unreadable = await run(
    "/r",
    options,
    fakeSteps({ readValues: () => [value(false)] }),
  );
  assert.deepEqual(unreadable, {
    failures: [],
    error: "p.json: open p.json: gone",
  });
  const missing = await run(
    "/r",
    options,
    fakeSteps({ readValues: () => [value(true)] }),
  );
  assert.equal(missing.error, undefined);
  assert.equal(missing.failures.length, 10);
});

test("the version files are read once and the same snapshot reaches the Copilot and parity steps", async () => {
  const snapshot: ValueResult[] = [];
  let reads = 0;
  const seen: unknown[] = [];
  await run(
    "/r",
    options,
    fakeSteps({
      readValues: () => {
        reads += 1;
        return reads === 1 ? snapshot : [];
      },
      copilotProviderRules: (_root, read) => {
        seen.push(read);
        return [];
      },
      parityRules: (_root, _catalogs, read) => {
        seen.push(read);
        return [];
      },
    }),
  );
  assert.equal(reads, 1);
  assert.equal(seen.length, 2);
  assert.ok(seen.every((read) => read === snapshot));
});

test("an exception while walking a parsed document is an audit error, never a pass", async () => {
  await withCheckout(async (root) => {
    const hostile: JsonObject = Object.defineProperty({}, "plugins", {
      enumerable: true,
      get: (): never => {
        throw new TypeError("cannot read plugins");
      },
    });
    const parsed = new Map([[AGENTS_CATALOG, hostile]]);
    const result = await run(root, options, {
      ...STEPS,
      catalogRules: () => ({ parsed, failures: [] }),
    });
    assert.deepEqual(result, { failures: [], error: "cannot read plugins" });
  });
});

test("each step receives what the earlier steps produced", async () => {
  const values: ValueResult[] = [];
  const parsed = new Map<string, JsonObject>();
  const seen: Record<string, unknown> = {};
  await run(
    "/r",
    options,
    fakeSteps({
      readValues: () => values,
      catalogRules: () => ({ parsed, failures: [] }),
      pluginJsonRules: () => ({ found: ["a/plugin.json"], failures: [] }),
      marketplaceSourceRules: (_root, catalogs) => {
        seen.sources = catalogs;
        return [];
      },
      copilotProviderRules: (_root, read) => {
        seen.copilot = read;
        return [];
      },
      noMcpRules: (_root, found) => {
        seen.found = found;
        return [];
      },
      parityRules: (_root, catalogs, read) => {
        seen.parity = [catalogs, read];
        return [];
      },
    }),
  );
  assert.equal(seen.sources, parsed);
  assert.equal(seen.copilot, values);
  assert.deepEqual(seen.found, ["a/plugin.json"]);
  assert.deepEqual(seen.parity, [parsed, values]);
});

type Mutate = (root: string) => void;

/** An agents catalog whose one entry carries `extra` under a key: the document's depth is 3 plus the extra's. */
const entryWith = (extra: string): string =>
  `{"plugins":[{"name":"agentics","source":".","x":${extra}}]}`;
const catalogWith = (fields: string): string =>
  `{"name":"muehz","plugins":[{"name":"agentics",${fields}"source":"./"}]}`;
/** The Claude catalog's agentics version and the Copilot manifest's version, as the given JSON text. */
const versions =
  (catalog: string, copilot: string): Mutate =>
  (root) => {
    writeFile(root, CLAUDE_CATALOG, catalogWith(`"version":${catalog},`));
    writeFile(
      root,
      COPILOT_PLUGIN,
      `{"name":"agentics","version":${copilot},"skills":["./skills/tdd"],"hooks":"./hooks/hooks.copilot.json"}`,
    );
  };

/**
 * How the manifests' JSON is read (key case, repeated keys, number tokens, escapes, nesting depth), reached through
 * files. `rules` is what the audit reports for each.
 */
const JSON_READING: Array<{
  name: string;
  mutate: Mutate;
  rules: string[];
}> = [
  {
    name: "a capitalized version key in a catalog entry is not the entry's version",
    mutate: (root) =>
      writeFile(root, CLAUDE_CATALOG, catalogWith('"Version":"2.0.0",')),
    rules: ["copilot-marketplace-version-parity"],
  },
  {
    name: "a version key in another case after the exact one changes nothing",
    mutate: (root) =>
      writeFile(
        root,
        CLAUDE_CATALOG,
        catalogWith('"version":"1.0.0","VERSION":"2.0.0",'),
      ),
    rules: [],
  },
  {
    name: "a capitalized plugins key is not the catalog's plugins",
    mutate: (root) =>
      writeFile(
        root,
        CLAUDE_CATALOG,
        '{"PLUGINS":[{"name":"agentics","version":"2.0.0","source":"./"}]}',
      ),
    rules: ["marketplace-entry", "copilot-marketplace-source"],
  },
  {
    name: "a repeated plugins key takes the last",
    mutate: (root) =>
      writeFile(
        root,
        CLAUDE_CATALOG,
        '{"plugins":[{"name":"agentics","version":"1.0.0","source":"./"}],"plugins":[]}',
      ),
    rules: ["marketplace-entry", "copilot-marketplace-source"],
  },
  {
    name: "the versions 1 and 1.0 are one token",
    mutate: versions("1", "1.0"),
    rules: ["provider-version-parity"],
  },
  {
    name: "the versions 1e2 and 100 are one token",
    mutate: versions("1e2", "100"),
    rules: ["provider-version-parity"],
  },
  {
    name: "the versions 1 and 2 are two",
    mutate: versions("1", "2"),
    rules: ["copilot-marketplace-version-parity", "provider-version-parity"],
  },
  {
    name: "the string 1 and the number 1 are two",
    mutate: versions('"1"', "1"),
    rules: ["copilot-marketplace-version-parity", "provider-version-parity"],
  },
  {
    name: "a lone surrogate escape reads as U+FFFD",
    mutate: versions('"\\ud800"', '"\\ufffd"'),
    rules: ["provider-version-parity"],
  },
  {
    name: "an escaped slash reads as a slash",
    mutate: versions('"a\\/b"', '"a/b"'),
    rules: ["provider-version-parity"],
  },
  {
    name: "a version that overflows float64 is a version that is not a string",
    mutate: versions("1.5", "1e400"),
    rules: ["copilot-marketplace-version-parity", "provider-version-parity"],
  },
  {
    name: "a plugin.json nested 10,001 deep is valid too",
    mutate: (root) => writeFile(root, "docs/plugin.json", nest(10001)),
    rules: [],
  },
  {
    name: "a catalog nested 10,001 deep is valid too",
    mutate: (root) => writeFile(root, AGENTS_CATALOG, entryWith(nest(9998))),
    rules: [],
  },
  {
    name: "a Claude catalog that is null is not an agentics catalog",
    mutate: (root) => writeFile(root, CLAUDE_CATALOG, "null"),
    rules: ["marketplace-entry", "copilot-marketplace-source"],
  },
  {
    name: "mcpServers of null is ignored and of {} is a path that escapes",
    mutate: (root) => {
      editJson(root, CODEX_PLUGIN, (value) => {
        value.mcpServers = null;
      });
      editJson(root, CLAUDE_PLUGIN, (value) => {
        value.mcpServers = {};
      });
    },
    rules: ["plugin-path-references"],
  },
  {
    name: "skills lists equal up to number form",
    mutate: (root) => {
      const text = (n: string, hook: string): string =>
        `{"name":"agentics","version":"1.0.0","skills":[${n},"./skills/tdd"],"hooks":"./hooks/hooks.${hook}.json"}`;
      writeFile(root, CLAUDE_PLUGIN, text("1.0", "claude"));
      writeFile(root, COPILOT_PLUGIN, text("1", "copilot"));
    },
    rules: ["plugin-path-references", "plugin-path-references"],
  },
];

/** Directory names `repo` skips and this domain walks, and near misses of the skipped names. */
const WALKED = [
  ".worktrees",
  "docs/handoffs",
  "docs/probes",
  "evals/runs",
  ".agentics",
  ".audit",
  "node_modules2",
  ".gitx",
  "claude",
];

/** `mcpServers` values that are not truthy, and values that are. */
const FALSY = ["false", "0", '""', "[]", "{}", "null"];
const TRUTHY = ["true", "1", '"x"', "[0]", '{"a":1}'];

/** Points both skills lists at `tdd` plus `entries`, over a checkout that ships `skills/extra` and `skills/other`. */
const registering =
  (entries: unknown[], dirs: string[]): Mutate =>
  (root) => {
    for (const dir of ["skills/extra", "skills/other", ...dirs])
      mkdir(root, dir);
    for (const manifest of [CLAUDE_PLUGIN, COPILOT_PLUGIN])
      editJson(root, manifest, (value) => {
        value.skills = ["./skills/tdd", ...entries];
      });
  };

/** `plugin-registers-shipped` names a shipped directory by the base name of a skills entry. */
const REGISTRATION: Array<{
  name: string;
  mutate: Mutate;
  rules: string[];
}> = [
  {
    name: "a ./skills/ entry for each shipped directory",
    mutate: registering(["./skills/extra", "./skills/other"], []),
    rules: [],
  },
  {
    name: "an entry with a trailing slash",
    mutate: registering(["./skills/extra/", "./skills/other"], []),
    rules: [],
  },
  {
    name: "a bare directory name that exists",
    mutate: registering(["extra", "./skills/other"], ["extra"]),
    rules: [],
  },
  {
    name: "another path with the same base name",
    mutate: registering(
      ["./elsewhere/extra", "./skills/other"],
      ["elsewhere/extra"],
    ),
    rules: [],
  },
  {
    name: "a shipped directory no entry names",
    mutate: registering(["./skills/other"], []),
    rules: ["plugin-registers-shipped"],
  },
  {
    name: "a non-string entry has the base name of an empty path",
    mutate: registering([5, "./skills/other"], []),
    rules: [
      "plugin-path-references",
      "plugin-path-references",
      "plugin-registers-shipped",
    ],
  },
  {
    name: "a missing skills directory is tolerated",
    mutate: (root) => remove(root, "skills"),
    rules: [
      "plugin-path-references",
      "plugin-path-references",
      "plugin-path-references",
    ],
  },
];

/** What the audit reports for each breaker, with its path and message. */
const EXPECTED: Record<string, Failure[]> = {
  "marketplace-present": [
    {
      rule: "marketplace-present",
      path: ".agents/plugins/marketplace.json",
      message: "missing",
    },
  ],
  "marketplace-json-valid": [
    {
      rule: "marketplace-json-valid",
      path: ".agents/plugins/marketplace.json",
      message: "invalid JSON",
    },
  ],
  "marketplace-entry": [
    {
      rule: "marketplace-entry",
      path: ".agents/plugins/marketplace.json",
      message: "plugins must contain exactly one object named agentics",
    },
  ],
  "plugin-json-valid": [
    {
      rule: "plugin-json-valid",
      path: "docs/plugin.json",
      message: "invalid JSON",
    },
  ],
  "marketplace-source-dir": [
    {
      rule: "marketplace-source-dir",
      path: ".agents/plugins/marketplace.json",
      message: "missing — directory not found",
    },
  ],
  "plugin-path-references": [
    {
      rule: "plugin-path-references",
      path: ".codex-plugin/plugin.json",
      message:
        "skills path not found → ./nope/ (string form must be a directory)",
    },
  ],
  "plugin-registers-shipped": [
    {
      rule: "plugin-registers-shipped",
      path: ".claude-plugin/plugin.json",
      message:
        "skills/extra/ ships but no skills entry registers it — consumers never load it",
    },
  ],
  "claude-hooks-field": [
    {
      rule: "claude-hooks-field",
      path: ".claude-plugin/plugin.json",
      message: "hooks must be ./hooks/hooks.claude.json",
    },
  ],
  "no-mcp-json": [
    {
      rule: "no-mcp-json",
      path: ".mcp.json",
      message: "Agentics ships no MCP configuration — remove it",
    },
  ],
  "no-mcp-servers": [
    {
      rule: "no-mcp-servers",
      path: "docs/plugin.json",
      message: "mcpServers must not be registered in plugin.json",
    },
  ],
  "marketplace-version-parity": [
    {
      rule: "marketplace-version-parity",
      path: ".agents/plugins/marketplace.json",
      message:
        "agentics version 2.0.0 != 1.0.0 (reference: .claude-plugin/marketplace.json)",
    },
  ],
  "provider-version-parity": [
    {
      rule: "provider-version-parity",
      path: ".codex-plugin/plugin.json",
      message: 'version "2.0.0" != .claude-plugin/plugin.json version "1.0.0"',
    },
  ],
  "no-claude-md": [
    {
      rule: "no-claude-md",
      path: "CLAUDE.md",
      message:
        "AGENTS.md is canonical; Claude Code reads this file instead of AGENTS.md, so remove it",
    },
  ],
  "copilot-shadow-manifest": [
    {
      rule: "copilot-shadow-manifest",
      path: "plugin.json",
      message:
        "higher-precedence Copilot shadow manifest plugin.json must be absent",
    },
  ],
  "copilot-effective-manifest": [
    {
      rule: "copilot-effective-manifest",
      path: ".claude-plugin/plugin.json",
      message: "effective Copilot manifest must be .github/plugin/plugin.json",
    },
    {
      rule: "provider-version-parity",
      path: ".github/plugin/plugin.json",
      message: "is missing",
    },
  ],
  "copilot-marketplace-present": [
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
  "copilot-marketplace-source": [
    {
      rule: "copilot-marketplace-source",
      path: ".claude-plugin/marketplace.json",
      message: "agentics marketplace source must be exactly './'",
    },
  ],
  "copilot-marketplace-name-parity": [
    {
      rule: "copilot-marketplace-name-parity",
      path: ".github/plugin/plugin.json",
      message: "effective manifest name must match Copilot marketplace entry",
    },
  ],
  "copilot-marketplace-version-parity": [
    {
      rule: "copilot-marketplace-version-parity",
      path: ".github/plugin/plugin.json",
      message:
        "effective manifest version must match Copilot marketplace entry",
    },
    {
      rule: "provider-version-parity",
      path: ".github/plugin/plugin.json",
      message: 'version "2.0.0" != .claude-plugin/plugin.json version "1.0.0"',
    },
  ],
  "copilot-hooks-field": [
    {
      rule: "copilot-hooks-field",
      path: ".github/plugin/plugin.json",
      message: "effective hooks must be exactly './hooks/hooks.copilot.json'",
    },
  ],
  "copilot-hooks-target": [
    {
      rule: "plugin-path-references",
      path: ".github/plugin/plugin.json",
      message: "hooks path not found → ./hooks/hooks.copilot.json",
    },
    {
      rule: "copilot-hooks-target",
      path: "hooks/hooks.copilot.json",
      message: "effective hooks target does not exist",
    },
  ],
  "copilot-skills-parity": [
    {
      rule: "copilot-skills-parity",
      path: ".github/plugin/plugin.json",
      message: "effective skills must match Claude skills",
    },
  ],
};

const rulesSorted = (failures: Failure[]): string[] => rulesOf(failures).sort();

function breaker(rule: string): (root: string) => void {
  const found = BREAKERS[rule];
  if (found === undefined) throw new Error(`no breaker for ${rule}`);
  return found;
}

test("the table of breakers and the table of expected failures name the same 22 rules", () => {
  assert.equal(Object.keys(BREAKERS).length, 22);
  assert.deepEqual(Object.keys(EXPECTED).sort(), Object.keys(BREAKERS).sort());
});

for (const [rule, mutate] of Object.entries(BREAKERS)) {
  test(`${rule} fires with its own path and message`, async () => {
    await withCheckout(async (root) => {
      mutate(root);
      assert.deepEqual(await run(root, options), { failures: EXPECTED[rule] });
    });
  });
}

test("provider-manifest-present fires when the Claude manifest is absent, a state the CLI never reaches", async () => {
  await withCheckout(async (root) => {
    remove(root, CLAUDE_PLUGIN);
    assert.deepEqual(await run(root, options), {
      failures: [
        {
          rule: "provider-manifest-present",
          path: ".claude-plugin/plugin.json",
          message: "missing",
        },
      ],
    });
  });
});

/** The Copilot rules in the order they run. Each one that fires ends the Copilot rules. */
const COPILOT_ORDER = [
  "copilot-shadow-manifest",
  "copilot-effective-manifest",
  "copilot-marketplace-present",
  "copilot-marketplace-source",
  "copilot-marketplace-name-parity",
  "copilot-marketplace-version-parity",
  "copilot-hooks-field",
  "copilot-hooks-target",
  "copilot-skills-parity",
];

COPILOT_ORDER.forEach((rule, index) => {
  test(`${rule} stops the Copilot rules that follow it`, async () => {
    await withCheckout(async (root) => {
      // Break this rule and every later one. The later changes go first because an earlier rule's change can remove
      // the file a later change edits.
      for (const later of COPILOT_ORDER.slice(index).reverse())
        breaker(later)(root);
      const result = await run(root, options);
      assert.equal(result.error, undefined);
      assert.deepEqual(
        rulesOf(result.failures).filter((found) =>
          found.startsWith("copilot-"),
        ),
        [rule],
      );
    });
  });
});

for (const reading of JSON_READING) {
  test(`JSON reading: ${reading.name}`, async () => {
    await withCheckout(async (root) => {
      reading.mutate(root);
      const result = await run(root, options);
      assert.equal(result.error, undefined);
      assert.deepEqual(rulesSorted(result.failures), [...reading.rules].sort());
    });
  });
}

for (const name of WALKED) {
  test(`a hit under ${name} is walked`, async () => {
    await withCheckout(async (root) => {
      writeFile(root, `${name}/.mcp.json`, "{}");
      writeFile(root, `${name}/plugin.json`, "{");
      const result = await run(root, options);
      assert.deepEqual(result.failures, [
        {
          rule: "plugin-json-valid",
          path: `${name}/plugin.json`,
          message: "invalid JSON",
        },
        {
          rule: "no-mcp-json",
          path: `${name}/.mcp.json`,
          message: "Agentics ships no MCP configuration — remove it",
        },
      ]);
    });
  });
}

const TEMP = "docs/specs/2026-11/feature-x/temp";
const TEMP_HITS = [`${TEMP}/plugin.json`, `${TEMP}/.mcp.json`];

/** Writes an invalid plugin.json and an .mcp.json into a work folder's temp directory. */
function writeTempHits(root: string): void {
  writeFile(root, TEMP_HITS[0]!, "{");
  writeFile(root, TEMP_HITS[1]!, "{}");
}

test("untracked files in a work folder's temp directory are not walked, in a checkout or outside one", async () => {
  await withCheckout(async (root) => {
    writeTempHits(root);
    assert.deepEqual(await run(root, options), { failures: [] });
    initCheckout(root);
    assert.deepEqual(await run(root, options), { failures: [] });
  });
});

test("tracked files in a work folder's temp directory are walked", async () => {
  await withCheckout(async (root) => {
    writeTempHits(root);
    initCheckout(root, TEMP_HITS);
    const result = await run(root, options);
    assert.equal(result.error, undefined);
    assert.deepEqual(
      result.failures.map((found) => [found.rule, found.path]),
      [
        ["plugin-json-valid", TEMP_HITS[0]],
        ["no-mcp-json", TEMP_HITS[1]],
      ],
    );
  });
});

test(
  "a .git that is not a repository passes without a work folder temp directory and is an audit error with one",
  {
    skip: tempIsInsideRepository()
      ? "the temp directory is inside a git repository"
      : false,
  },
  async () => {
    await withCheckout(async (root) => {
      mkdir(root, ".git");
      mkdir(root, "docs/specs/2026-11/feature-x/notes");
      assert.deepEqual(await run(root, options), { failures: [] });
      mkdir(root, TEMP);
      const result = await run(root, options);
      assert.deepEqual(result.failures, []);
      assert.match(result.error ?? "", /^listing tracked files: /);
    });
  },
);

for (const value of FALSY) {
  test(`mcpServers of ${value} is not registered`, async () => {
    await withCheckout(async (root) => {
      writeFile(root, "docs/plugin.json", `{"mcpServers":${value}}`);
      assert.deepEqual(await run(root, options), { failures: [] });
    });
  });
}

for (const value of TRUTHY) {
  test(`mcpServers of ${value} is registered`, async () => {
    await withCheckout(async (root) => {
      writeFile(root, "docs/plugin.json", `{"mcpServers":${value}}`);
      assert.deepEqual(rulesOf((await run(root, options)).failures), [
        "no-mcp-servers",
      ]);
    });
  });
}

for (const registration of REGISTRATION) {
  test(`registration: ${registration.name}`, async () => {
    await withCheckout(async (root) => {
      registration.mutate(root);
      const result = await run(root, options);
      assert.equal(result.error, undefined);
      assert.deepEqual(
        rulesSorted(result.failures),
        [...registration.rules].sort(),
      );
    });
  });
}

/** Runs the manifests audit on a passing checkout after `mutate` changes it. */
const auditAfter = (mutate: Mutate): Promise<AuditResult> =>
  withCheckout(async (root) => {
    mutate(root);
    return auditManifests(root);
  });

const CLEAN: AuditResult = { failures: [] };

const needsSymlink = { skip: canSymlink() ? false : SYMLINK_SKIP };
const needsNotADirectory = {
  skip: reportsNotADirectory()
    ? false
    : "this host reports a path below a regular file as absent, not as an error",
};
const needsAbsent = {
  skip: reportsNotADirectory()
    ? "this host reports a path below a regular file as an error, not as absent"
    : false,
};

/** Writes an invalid plugin.json and an .mcp.json under `name`, at the root and below `deep/`. */
function hitsUnder(name: string): Mutate {
  return (root) => {
    for (const base of [name, `deep/${name}`]) {
      writeFile(root, `${base}/.mcp.json`, "{}");
      writeFile(root, `${base}/plugin.json`, "{");
    }
  };
}

/** Replaces the skills directory with a regular file of the same name. */
const skillsAsFile: Mutate = (r) => {
  remove(r, "skills");
  writeFile(r, "skills", "x");
};

test("a clean fixture", async () => {
  assert.deepEqual(await auditAfter(() => {}), CLEAN);
});

test("a version key repeated in another case, the lower-case one last", async () => {
  assert.deepEqual(
    await auditAfter((r) =>
      writeFile(
        r,
        CLAUDE_CATALOG,
        claudeCatalog(agenticsEntry('"VERSION":"2.0.0","version":"1.0.0",')),
      ),
    ),
    CLEAN,
  );
});

test("a hooks key repeated, the valid one last", async () => {
  assert.deepEqual(
    await auditAfter((r) =>
      writeFile(
        r,
        CLAUDE_PLUGIN,
        '{"name":"agentics","version":"1.0.0","skills":["./skills/tdd"],"hooks":"./bad.json","hooks":"./hooks/hooks.claude.json"}',
      ),
    ),
    CLEAN,
  );
});

test("a version key repeated in a plugin manifest, the first one wrong", async () => {
  assert.deepEqual(
    await auditAfter((r) =>
      writeFile(
        r,
        CODEX_PLUGIN,
        '{"name":"agentics","version":"2.0.0","version":"1.0.0","skills":"./skills/","hooks":"./hooks/hooks.codex.json"}',
      ),
    ),
    CLEAN,
  );
});

test("a plugin.json nested 10,000 deep", async () => {
  assert.deepEqual(
    await auditAfter((r) => writeFile(r, "docs/plugin.json", nest(10000))),
    CLEAN,
  );
});

test("a catalog nested 10,000 deep", async () => {
  assert.deepEqual(
    await auditAfter((r) =>
      writeFile(r, AGENTS_CATALOG, entryWith(nest(9997))),
    ),
    CLEAN,
  );
});

test("a plugin.json that is null", async () => {
  assert.deepEqual(
    await auditAfter((r) => writeFile(r, "docs/plugin.json", "null")),
    CLEAN,
  );
});

test("a plugin.json that is an array", async () => {
  assert.deepEqual(
    await auditAfter((r) => writeFile(r, "docs/plugin.json", "[]")),
    CLEAN,
  );
});

test("a plugin.json that is a number", async () => {
  assert.deepEqual(
    await auditAfter((r) => writeFile(r, "docs/plugin.json", "5")),
    CLEAN,
  );
});

test("the Copilot manifest is an array", async () => {
  assert.deepEqual(
    await auditAfter((r) => writeFile(r, COPILOT_PLUGIN, "[]")),
    CLEAN,
  );
});

test("a pluginRoot in the Claude catalog metadata", async () => {
  assert.deepEqual(
    await auditAfter((r) => {
      mkdir(r, "pkg");
      writeFile(
        r,
        CLAUDE_CATALOG,
        '{"name":"muehz","metadata":{"pluginRoot":"pkg"},"plugins":[{"name":"agentics","version":"1.0.0","source":"./"}]}',
      );
    }),
    CLEAN,
  );
});

test("a skills entry that is a file", async () => {
  assert.deepEqual(
    await auditAfter((r) => editCodex(r, (v) => (v.skills = ["./AGENTS.md"]))),
    CLEAN,
  );
});

test("a skills value of another type", async () => {
  assert.deepEqual(
    await auditAfter((r) => editCodex(r, (v) => (v.skills = 5))),
    CLEAN,
  );
});

test("an mcpServers of null", async () => {
  assert.deepEqual(
    await auditAfter((r) => editCodex(r, (v) => (v.mcpServers = null))),
    CLEAN,
  );
});

test("mcpServers truthiness: false, 0, empty string, empty list", async () => {
  assert.deepEqual(
    await auditAfter((r) => {
      writeFile(r, "a/plugin.json", '{"mcpServers":false}');
      writeFile(r, "b/plugin.json", '{"mcpServers":0}');
      writeFile(r, "c/plugin.json", '{"mcpServers":""}');
      writeFile(r, "d/plugin.json", '{"mcpServers":[]}');
      writeFile(r, "e/plugin.json", '{"mcpServers":{}}');
    }),
    CLEAN,
  );
});

test("a skills directory registered by an entry with a trailing slash", async () => {
  assert.deepEqual(
    await auditAfter((r) => {
      mkdir(r, "skills/extra");
      for (const manifest of [CLAUDE_PLUGIN, COPILOT_PLUGIN])
        editJson(
          r,
          manifest,
          (v) => (v.skills = ["./skills/tdd", "./skills/extra/"]),
        );
    }),
    CLEAN,
  );
});

test("a file under skills is not a skill", async () => {
  assert.deepEqual(
    await auditAfter((r) => writeFile(r, "skills/README.md", "x")),
    CLEAN,
  );
});

test(
  "a skills entry that is a link inside the checkout",
  needsSymlink,
  async () => {
    assert.deepEqual(
      await auditAfter((r) => {
        link(r, "tdd", "skills/alias", "dir");
        editCodex(r, (v) => (v.skills = ["./skills/alias"]));
      }),
      CLEAN,
    );
  },
);

test("a skills entry that is a link to a file", needsSymlink, async () => {
  assert.deepEqual(
    await auditAfter((r) => {
      link(r, "../AGENTS.md", "skills/filelink", "file");
      editCodex(r, (v) => (v.skills = ["./skills/filelink"]));
    }),
    CLEAN,
  );
});

test(
  "a link to a directory under skills is not a shipped skill",
  needsSymlink,
  async () => {
    assert.deepEqual(
      await auditAfter((r) => link(r, "tdd", "skills/alias", "dir")),
      CLEAN,
    );
  },
);

test("a plugin.json link to a valid file", needsSymlink, async () => {
  assert.deepEqual(
    await auditAfter((r) =>
      link(r, "../.claude-plugin/plugin.json", "docs/plugin.json", "file"),
    ),
    CLEAN,
  );
});

test(
  "a catalog source that is a link inside the checkout",
  needsSymlink,
  async () => {
    assert.deepEqual(
      await auditAfter((r) => {
        link(r, "skills", "alias", "dir");
        writeFile(
          r,
          AGENTS_CATALOG,
          '{"plugins":[{"name":"agentics","source":"./alias"}]}',
        );
      }),
      CLEAN,
    );
  },
);

test("a hit under .git is not walked, at the root or nested", async () => {
  assert.deepEqual(await auditAfter(hitsUnder(".git")), CLEAN);
});

test("a hit under node_modules is not walked, at the root or nested", async () => {
  assert.deepEqual(await auditAfter(hitsUnder("node_modules")), CLEAN);
});

test("a hit under .claude is not walked, at the root or nested", async () => {
  assert.deepEqual(await auditAfter(hitsUnder(".claude")), CLEAN);
});

test("a directory named plugin.json is ignored", async () => {
  assert.deepEqual(
    await auditAfter((r) => mkdir(r, "docs/plugin.json")),
    CLEAN,
  );
});

test("a directory named .mcp.json is ignored", async () => {
  assert.deepEqual(await auditAfter((r) => mkdir(r, "docs/.mcp.json")), CLEAN);
});

test("a CLAUDE.md in a subdirectory is not the rule's path", async () => {
  assert.deepEqual(
    await auditAfter((r) => writeFile(r, "docs/CLAUDE.md", "x")),
    CLEAN,
  );
});

test(
  "a directory link to an outside tree holding an invalid plugin.json and a .mcp.json is not walked",
  needsSymlink,
  async () => {
    await withOutside(async (outside) => {
      writeFile(outside, "tree/plugin.json", "{");
      writeFile(outside, "tree/.mcp.json", "{}");
      assert.deepEqual(
        await auditAfter((r) =>
          link(r, path.join(outside, "tree"), "docs/outside", "dir"),
        ),
        CLEAN,
      );
    });
  },
);

test("the Claude catalog is a directory", async () => {
  assert.deepEqual(await auditAfter((r) => asDirectory(r, CLAUDE_CATALOG)), {
    failures: [],
    error: ".claude-plugin/marketplace.json: is not a regular file",
  });
});

test("the Claude manifest is a directory", async () => {
  assert.deepEqual(await auditAfter((r) => asDirectory(r, CLAUDE_PLUGIN)), {
    failures: [],
    error: ".claude-plugin/plugin.json: is not a regular file",
  });
});

test("the Codex manifest is a directory", async () => {
  assert.deepEqual(await auditAfter((r) => asDirectory(r, CODEX_PLUGIN)), {
    failures: [],
    error: ".codex-plugin/plugin.json: is not a regular file",
  });
});

test("the Copilot manifest is a directory", async () => {
  assert.deepEqual(await auditAfter((r) => asDirectory(r, COPILOT_PLUGIN)), {
    failures: [],
    error: ".github/plugin/plugin.json: is not a regular file",
  });
});

test("the agents catalog is a directory", async () => {
  assert.deepEqual(await auditAfter((r) => asDirectory(r, AGENTS_CATALOG)), {
    failures: [],
    error: ".agents/plugins/marketplace.json: is not a regular file",
  });
});

// Where a path below a regular file is absent, the path-reference step reports the three skills entries as missing and
// the registration step then fails to list skills; where it is an error, the path-reference step raises first.
test(
  "skills is a file (a host that reports a path through a file as absent)",
  needsAbsent,
  async () => {
    assert.deepEqual(await auditAfter(skillsAsFile), {
      failures: [
        {
          rule: "plugin-path-references",
          path: ".claude-plugin/plugin.json",
          message: "skills path not found → ./skills/tdd",
        },
        {
          rule: "plugin-path-references",
          path: ".codex-plugin/plugin.json",
          message:
            "skills path not found → ./skills/ (string form must be a directory)",
        },
        {
          rule: "plugin-path-references",
          path: ".github/plugin/plugin.json",
          message: "skills path not found → ./skills/tdd",
        },
      ],
      error: "enumerate skills: skills: is not a directory",
    });
  },
);

test(
  "skills is a file (a host that reports a path through a file as an error)",
  needsNotADirectory,
  async () => {
    assert.deepEqual(await auditAfter(skillsAsFile), {
      failures: [],
      error:
        "audit .claude-plugin/plugin.json skills reference: skills/tdd: is not a directory",
    });
  },
);

test(
  "a regular file where .plugin should be a directory",
  needsNotADirectory,
  async () => {
    assert.deepEqual(await auditAfter((r) => writeFile(r, ".plugin", "x")), {
      failures: [],
      error:
        "discover Copilot manifest: .plugin/plugin.json: is not a directory",
    });
  },
);

test("the repository root is a symlink", needsSymlink, async () => {
  await withCheckout(async (root) => {
    await withOutside(async (outside) => {
      link(outside, root, "link", "dir");
      assert.deepEqual(await auditManifests(path.join(outside, "link")), {
        failures: [],
        error: ".: passes through a link",
      });
    });
  });
});

test("a link loop in a manifest path", needsSymlink, async () => {
  const result = await auditAfter((r) => {
    link(r, "b", "skills/a", "dir");
    link(r, "a", "skills/b", "dir");
    // The Claude manifest is checked first, so its escaping entry is reported before the loop in the Codex one raises.
    editJson(r, CLAUDE_PLUGIN, (v) => (v.skills = ["../x"]));
    // A catalog failure, which an error in the path-reference step keeps.
    writeFile(r, AGENTS_CATALOG, "[]");
    editCodex(r, (v) => (v.skills = ["./skills/a"]));
  });
  assert.deepEqual(result, {
    failures: [
      {
        rule: "marketplace-json-valid",
        path: ".agents/plugins/marketplace.json",
        message: "invalid JSON",
      },
      {
        rule: "plugin-path-references",
        path: ".claude-plugin/plugin.json",
        message: 'skills path escapes checkout: "../x"',
      },
    ],
    error: "audit .codex-plugin/plugin.json skills reference: skills/a: ELOOP",
  });
});

test("a link loop in a catalog source", needsSymlink, async () => {
  const result = await auditAfter((r) => {
    link(r, "b", "a", "dir");
    link(r, "a", "b", "dir");
    // The Claude catalog is walked first; its escaping source is found before the agents catalog's loop raises.
    writeFile(
      r,
      CLAUDE_CATALOG,
      claudeCatalog('{"name":"agentics","version":"1.0.0","source":"../x"}'),
    );
    writeFile(
      r,
      AGENTS_CATALOG,
      '{"plugins":[{"name":"agentics","source":"./a"}]}',
    );
  });
  assert.deepEqual(result, {
    failures: [
      {
        rule: "marketplace-source-dir",
        path: ".claude-plugin/marketplace.json",
        message: 'path escapes checkout: "../x"',
      },
    ],
    error:
      "audit marketplace source .agents/plugins/marketplace.json: a: ELOOP",
  });
});

// An error in the plugin.json step drops the failures an earlier step found, so the catalog failure is not reported.
test(
  "a link loop in a plugin.json found by the walk, after a catalog failure",
  needsSymlink,
  async () => {
    const result = await auditAfter((r) => {
      writeFile(r, AGENTS_CATALOG, "[]");
      link(r, "b.json", "docs/plugin.json", "file");
      link(r, "plugin.json", "docs/b.json", "file");
    });
    assert.deepEqual(result, {
      failures: [],
      error: "docs/plugin.json: ELOOP",
    });
  },
);

test("five-manifest parity enforces .antigravity-plugin/plugin.json version, logo, and mcp_config.json without disturbing copilot-shadow-manifest", async () => {
  await withCheckout(async (root) => {
    editJson(root, ANTIGRAVITY_PLUGIN, (value) => {
      value.version = "2.0.0";
    });
    assert.deepEqual(await run(root, options), {
      failures: [
        {
          rule: "provider-version-parity",
          path: ANTIGRAVITY_PLUGIN,
          message:
            'version "2.0.0" != .claude-plugin/plugin.json version "1.0.0"',
        },
      ],
    });
  });

  await withCheckout(async (root) => {
    remove(root, ANTIGRAVITY_PLUGIN);
    assert.deepEqual(await run(root, options), {
      failures: [
        {
          rule: "provider-version-parity",
          path: ANTIGRAVITY_PLUGIN,
          message: "is missing",
        },
      ],
    });
  });

  await withCheckout(async (root) => {
    editJson(root, ANTIGRAVITY_PLUGIN, (value) => {
      value.logo = "assets/missing.svg";
    });
    assert.deepEqual(await run(root, options), {
      failures: [
        {
          rule: "plugin-path-references",
          path: ANTIGRAVITY_PLUGIN,
          message: "logo path not found → assets/missing.svg",
        },
      ],
    });
  });

  await withCheckout(async (root) => {
    writeFile(root, "mcp_config.json", "{}");
    assert.deepEqual(await run(root, options), {
      failures: [
        {
          rule: "no-mcp-json",
          path: "mcp_config.json",
          message: "Agentics ships no MCP configuration — remove it",
        },
      ],
    });
  });

  for (const shadow of ["plugin.json", ".plugin/plugin.json"]) {
    await withCheckout(async (root) => {
      writeFile(root, shadow, "{}");
      assert.deepEqual(await run(root, options), {
        failures: [
          {
            rule: "copilot-shadow-manifest",
            path: shadow,
            message: `higher-precedence Copilot shadow manifest ${shadow} must be absent`,
          },
        ],
      });
    });
  }
});
