import assert from "node:assert/strict";
import { mkdtempSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { test } from "node:test";
import {
  UNSET,
  type ErrorKind,
  type ValueResult,
} from "../../../release/version.ts";
import { writeFile } from "../../testing/files.ts";
import {
  isJsonObject,
  MARKETPLACES,
  type JsonObject,
  type JsonValue,
} from "./files.ts";
import { marketplaceParity, parityRules, providerParity } from "./parity.ts";
import { removeTree } from "../../testing/host.ts";
import {
  auditManifests,
  BREAKERS,
  CODEX_PLUGIN,
  COPILOT_PLUGIN,
  withManifestsRepo,
} from "../../testing/manifests-repo.ts";
import {
  agenticsEntry,
  claudeCatalog,
  versions,
} from "../../testing/manifests-seeds.ts";

const [CLAUDE_CATALOG, AGENTS_CATALOG] = MARKETPLACES;
const CLAUDE = ".claude-plugin/plugin.json";
const CODEX = ".codex-plugin/plugin.json";
const GITHUB = ".github/plugin/plugin.json";
const ANTIGRAVITY = ".antigravity-plugin/plugin.json";

function obj(text: string): JsonObject {
  const parsed = JSON.parse(text) as JsonValue;
  assert.ok(isJsonObject(parsed), text);
  return parsed;
}

function value(
  file: string,
  raw: string,
  kind: ErrorKind = "none",
): ValueResult {
  return {
    path: file,
    raw,
    kind,
    error: undefined,
    value: UNSET,
    missing: false,
  };
}

const catalogs = (claude: string, agents: string) =>
  new Map([
    [CLAUDE_CATALOG, obj(claude)],
    [AGENTS_CATALOG, obj(agents)],
  ]);

const plugins = (...entries: string[]): string =>
  `{"plugins":[${entries.join(",")}]}`;

test("marketplace-name-parity reports the names missing from and extra in the second catalog, sorted, as comma-separated JSON strings", () => {
  const ref = plugins('{"name":"agentics"}', '{"name":"c"}', '{"name":"b"}');
  const reference = ` (reference: ${CLAUDE_CATALOG})`;
  const run = (other: string) =>
    marketplaceParity(catalogs(ref, other), []).map((f) => [
      f.rule,
      f.path,
      f.message,
    ]);
  assert.deepEqual(
    run(
      plugins(
        '{"name":"agentics"}',
        '{"name":"b"}',
        '{"name":"c"}',
        '{"name":"z"}',
        '{"name":"y"}',
      ),
    ),
    [
      [
        "marketplace-name-parity",
        AGENTS_CATALOG,
        `extra plugins: "y", "z"${reference}`,
      ],
    ],
  );
  assert.deepEqual(run(plugins('{"name":"agentics"}')), [
    [
      "marketplace-name-parity",
      AGENTS_CATALOG,
      `missing plugins: "b", "c"${reference}`,
    ],
  ]);
  assert.deepEqual(run(plugins('{"name":"agentics"}', '{"name":"q"}')), [
    [
      "marketplace-name-parity",
      AGENTS_CATALOG,
      `missing plugins: "b", "c"; extra plugins: "q"${reference}`,
    ],
  ]);
  // A plugin whose name is not a string has the empty name, which is quoted so it stays visible.
  assert.deepEqual(
    marketplaceParity(
      catalogs(
        plugins('{"name":"agentics"}'),
        plugins('{"name":"agentics"}', '{"name":5}'),
      ),
      [],
    ).map((f) => f.message),
    [`extra plugins: ""${reference}`],
  );
});

test("marketplace-name-parity ends the comparison, so a version difference is not also reported", () => {
  const failures = marketplaceParity(
    catalogs(
      plugins('{"name":"agentics","version":"1.0.0"}', '{"name":"b"}'),
      plugins('{"name":"agentics","version":"2.0.0"}'),
    ),
    [value(CLAUDE_CATALOG, "1.0.0")],
  );
  assert.deepEqual(
    failures.map((f) => f.rule),
    ["marketplace-name-parity"],
  );
});

test("marketplace-version-parity compares the string versions of plugins with the same name", () => {
  const failures = marketplaceParity(
    catalogs(
      plugins(
        '{"name":"agentics"}',
        '{"name":"b","version":"1.0.0"}',
        '{"name":"c","version":"1.0.0"}',
        '{"name":"d"}',
      ),
      plugins(
        '{"name":"agentics"}',
        '{"name":"b","version":"2.0.0"}',
        '{"name":"c","version":"1.0.0"}',
        '{"name":"d","version":"9"}',
      ),
    ),
    [],
  );
  assert.deepEqual(failures, [
    {
      rule: "marketplace-version-parity",
      path: AGENTS_CATALOG,
      message: `b version 2.0.0 != 1.0.0 (reference: ${CLAUDE_CATALOG})`,
    },
  ]);
  // A plugin whose version is absent or empty in the second catalog is not compared with the reference's version.
  for (const other of ['{"name":"b"}', '{"name":"b","version":""}'])
    assert.deepEqual(
      marketplaceParity(
        catalogs(
          plugins('{"name":"agentics"}', '{"name":"b","version":"1.0.0"}'),
          plugins('{"name":"agentics"}', other),
        ),
        [],
      ),
      [],
      other,
    );
  // A version that is empty, or not a string, is not compared.
  assert.deepEqual(
    marketplaceParity(
      catalogs(
        plugins('{"name":"agentics"}', '{"name":"b","version":""}'),
        plugins('{"name":"agentics"}', '{"name":"b","version":"2"}'),
      ),
      [],
    ),
    [],
  );
  assert.deepEqual(
    marketplaceParity(
      catalogs(
        plugins('{"name":"agentics"}', '{"name":"b","version":1}'),
        plugins('{"name":"agentics"}', '{"name":"b","version":2}'),
      ),
      [],
    ),
    [],
  );
});

test("marketplace-version-parity takes the Claude catalog's agentics version from the version reader, and the other catalog's from its own entry", () => {
  const agents = (version: string) =>
    plugins(`{"name":"agentics","version":"${version}"}`);
  const claude = plugins('{"name":"agentics","version":"9.9.9"}');
  const message = (other: string, ref: string) =>
    `agentics version ${other} != ${ref} (reference: ${CLAUDE_CATALOG})`;
  // The catalog's own 9.9.9 is ignored: the first non-empty result for the Claude catalog wins.
  const values = [
    value(CLAUDE_CATALOG, ""),
    value(CLAUDE_CATALOG, "1.0.0"),
    value(CLAUDE_CATALOG, "5.0.0"),
    value(GITHUB, "7.0.0"),
  ];
  assert.deepEqual(
    marketplaceParity(catalogs(claude, agents("2.0.0")), values).map(
      (f) => f.message,
    ),
    [message("2.0.0", "1.0.0")],
  );
  assert.deepEqual(
    marketplaceParity(catalogs(claude, agents("1.0.0")), values),
    [],
  );
  // With no result for the Claude catalog there is no agentics version to compare.
  assert.deepEqual(
    marketplaceParity(catalogs(claude, agents("2.0.0")), [
      value(GITHUB, "7.0.0"),
    ]),
    [],
  );
});

test("marketplace parity is skipped unless both catalogs are parsed", () => {
  const only = (rel: string) =>
    new Map([[rel, obj(plugins('{"name":"agentics"}'))]]);
  assert.deepEqual(marketplaceParity(only(CLAUDE_CATALOG), []), []);
  assert.deepEqual(marketplaceParity(only(AGENTS_CATALOG), []), []);
  assert.deepEqual(marketplaceParity(new Map(), []), []);
});

const withRoot = <T>(
  files: Record<string, string>,
  body: (root: string) => T,
): T => {
  const root = mkdtempSync(path.join(tmpdir(), "agentics-parity-"));
  try {
    for (const [rel, content] of Object.entries(files))
      writeFile(root, rel, content);
    return body(root);
  } finally {
    removeTree(root);
  }
};

const provider = (files: Record<string, string>, values: ValueResult[]) =>
  withRoot(files, (root) => providerParity(root, values));

const GOOD = [
  value(CLAUDE, "1.0.0"),
  value(CODEX, "1.0.0"),
  value(GITHUB, "1.0.0"),
  value(ANTIGRAVITY, "1.0.0"),
];

test("provider-manifest-present fires when the Claude manifest does not exist, and nothing else is compared", () => {
  // These values would report "has no version", "is missing" and a mismatch if the comparison ran.
  const failing = [value(CODEX, "", "read"), value(GITHUB, "2.0.0")];
  for (const values of [GOOD, failing, []])
    assert.deepEqual(provider({}, values), [
      { rule: "provider-manifest-present", path: CLAUDE, message: "missing" },
    ]);
});

test("provider-version-parity is silent when the Claude manifest is present but not an object, and when the versions agree", () => {
  for (const text of ["{", "[]", "7"])
    assert.deepEqual(
      provider({ [CLAUDE]: text }, [value(CLAUDE, "", "json")]),
      [],
      text,
    );
  assert.deepEqual(provider({ [CLAUDE]: "{}" }, GOOD), []);
});

test("provider-version-parity reports a Claude manifest with no version", () => {
  assert.deepEqual(
    provider({ [CLAUDE]: "{}" }, [
      value(CLAUDE, "", "shape"),
      value(CODEX, ""),
      value(GITHUB, ""),
      value(ANTIGRAVITY, ""),
    ]),
    [
      {
        rule: "provider-version-parity",
        path: CLAUDE,
        message: "has no version",
      },
    ],
  );
  assert.deepEqual(
    provider({ [CLAUDE]: "{}" }, []).map((f) => f.message),
    ["has no version"],
  );
});

test("provider-version-parity names each differing provider, GitHub before Codex before Antigravity, quoting each version as JSON", () => {
  const failures = provider({ [CLAUDE]: "{}" }, [
    value(CLAUDE, "1.0.0\u200b"),
    value(ANTIGRAVITY, "4.0.0"),
    value(CODEX, '2.0.0"x'),
    value(GITHUB, "3.0.0"),
  ]);
  assert.deepEqual(failures, [
    {
      rule: "provider-version-parity",
      path: GITHUB,
      message: `version "3.0.0" != ${CLAUDE} version "1.0.0\u200b"`,
    },
    {
      rule: "provider-version-parity",
      path: CODEX,
      message: `version "2.0.0\\"x" != ${CLAUDE} version "1.0.0\u200b"`,
    },
    {
      rule: "provider-version-parity",
      path: ANTIGRAVITY,
      message: `version "4.0.0" != ${CLAUDE} version "1.0.0\u200b"`,
    },
  ]);
  // A provider with no usable version (shape, or a non-canonical version) still differs from the canonical one.
  assert.deepEqual(
    provider({ [CLAUDE]: "{}" }, [
      value(CLAUDE, "1.0.0"),
      value(CODEX, "", "shape"),
      value(GITHUB, "1.0", "version"),
      value(ANTIGRAVITY, "", "shape"),
    ]).map((f) => f.message),
    [
      `version "1.0" != ${CLAUDE} version "1.0.0"`,
      `version "" != ${CLAUDE} version "1.0.0"`,
      `version "" != ${CLAUDE} version "1.0.0"`,
    ],
  );
});

test("provider-version-parity calls a provider whose file could not be read missing, and skips one with invalid JSON", () => {
  assert.deepEqual(
    provider({ [CLAUDE]: "{}" }, [
      value(CLAUDE, "1.0.0"),
      value(CODEX, "", "read"),
      value(GITHUB, "", "json"),
      value(ANTIGRAVITY, "", "read"),
    ]),
    [
      { rule: "provider-version-parity", path: CODEX, message: "is missing" },
      {
        rule: "provider-version-parity",
        path: ANTIGRAVITY,
        message: "is missing",
      },
    ],
  );
  assert.deepEqual(
    provider({ [CLAUDE]: "{}" }, [
      value(CLAUDE, "1.0.0"),
      value(CODEX, "1.0.0"),
      value(GITHUB, "1.0.0"),
      value(ANTIGRAVITY, "", "json"),
    ]),
    [],
  );
});

test("parityRules runs the catalog rules, then the provider rules", () => {
  const failures = withRoot({ [CLAUDE]: "{}" }, (root) =>
    parityRules(
      root,
      catalogs(
        plugins('{"name":"agentics","version":"1.0.0"}'),
        plugins('{"name":"agentics","version":"2.0.0"}'),
      ),
      [
        value(CLAUDE_CATALOG, "1.0.0"),
        value(CLAUDE, "1.0.0"),
        value(CODEX, "3.0.0"),
        value(GITHUB, "1.0.0"),
      ],
    ),
  );
  assert.deepEqual(
    failures.map((f) => [f.rule, f.path]),
    [
      ["marketplace-version-parity", AGENTS_CATALOG],
      ["provider-version-parity", CODEX],
    ],
  );
});

test("marketplace-version-parity reports an agents catalog version other than the Claude catalog version", async () => {
  await withManifestsRepo(async (root) => {
    BREAKERS["marketplace-version-parity"]!(root);
    assert.deepEqual(await auditManifests(root), {
      failures: [
        {
          rule: "marketplace-version-parity",
          path: ".agents/plugins/marketplace.json",
          message:
            "agentics version 2.0.0 != 1.0.0 (reference: .claude-plugin/marketplace.json)",
        },
      ],
    });
  });
});

test("provider-version-parity reports a Codex manifest version other than the Claude manifest version", async () => {
  await withManifestsRepo(async (root) => {
    BREAKERS["provider-version-parity"]!(root);
    assert.deepEqual(await auditManifests(root), {
      failures: [
        {
          rule: "provider-version-parity",
          path: ".codex-plugin/plugin.json",
          message:
            'version "2.0.0" != .claude-plugin/plugin.json version "1.0.0"',
        },
      ],
    });
  });
});

test("a plugin manifest whose version key is capitalized", async () => {
  await withManifestsRepo(async (root) => {
    writeFile(
      root,
      CODEX_PLUGIN,
      '{"name":"agentics","Version":"1.0.0","skills":"./skills/","hooks":"./hooks/hooks.codex.json"}',
    );
    assert.deepEqual(await auditManifests(root), {
      failures: [
        {
          rule: "provider-version-parity",
          path: ".codex-plugin/plugin.json",
          message: 'version "" != .claude-plugin/plugin.json version "1.0.0"',
        },
      ],
    });
  });
});

test("version 1 in the catalog and 1.0 in the Copilot manifest", async () => {
  await withManifestsRepo(async (root) => {
    versions(root, "1", "1.0");
    assert.deepEqual(await auditManifests(root), {
      failures: [
        {
          rule: "provider-version-parity",
          path: ".github/plugin/plugin.json",
          message: 'version "" != .claude-plugin/plugin.json version "1.0.0"',
        },
      ],
    });
  });
});

test("version 1e0 and 1", async () => {
  await withManifestsRepo(async (root) => {
    versions(root, "1e0", "1");
    assert.deepEqual(await auditManifests(root), {
      failures: [
        {
          rule: "provider-version-parity",
          path: ".github/plugin/plugin.json",
          message: 'version "" != .claude-plugin/plugin.json version "1.0.0"',
        },
      ],
    });
  });
});

test("version -0.0 and 0", async () => {
  await withManifestsRepo(async (root) => {
    versions(root, "-0.0", "0");
    assert.deepEqual(await auditManifests(root), {
      failures: [
        {
          rule: "provider-version-parity",
          path: ".github/plugin/plugin.json",
          message: 'version "" != .claude-plugin/plugin.json version "1.0.0"',
        },
      ],
    });
  });
});

test("version 1e2 and 100", async () => {
  await withManifestsRepo(async (root) => {
    versions(root, "1e2", "100");
    assert.deepEqual(await auditManifests(root), {
      failures: [
        {
          rule: "provider-version-parity",
          path: ".github/plugin/plugin.json",
          message: 'version "" != .claude-plugin/plugin.json version "1.0.0"',
        },
      ],
    });
  });
});

test("two integers that are one float64", async () => {
  await withManifestsRepo(async (root) => {
    versions(root, "9007199254740993", "9007199254740992");
    assert.deepEqual(await auditManifests(root), {
      failures: [
        {
          rule: "provider-version-parity",
          path: ".github/plugin/plugin.json",
          message: 'version "" != .claude-plugin/plugin.json version "1.0.0"',
        },
      ],
    });
  });
});

test("version -0 and 0", async () => {
  await withManifestsRepo(async (root) => {
    versions(root, "-0", "0");
    assert.deepEqual(await auditManifests(root), {
      failures: [
        {
          rule: "provider-version-parity",
          path: ".github/plugin/plugin.json",
          message: 'version "" != .claude-plugin/plugin.json version "1.0.0"',
        },
      ],
    });
  });
});

test("a catalog version of null and a manifest with none", async () => {
  await withManifestsRepo(async (root) => {
    writeFile(
      root,
      CLAUDE_CATALOG,
      claudeCatalog(agenticsEntry('"version":null,')),
    );
    writeFile(
      root,
      COPILOT_PLUGIN,
      '{"name":"agentics","skills":["./skills/tdd"],"hooks":"./hooks/hooks.copilot.json"}',
    );
    assert.deepEqual(await auditManifests(root), {
      failures: [
        {
          rule: "provider-version-parity",
          path: ".github/plugin/plugin.json",
          message: 'version "" != .claude-plugin/plugin.json version "1.0.0"',
        },
      ],
    });
  });
});

test("a lone surrogate escape and U+FFFD", async () => {
  await withManifestsRepo(async (root) => {
    versions(root, '"\\ud800"', '"\\ufffd"');
    assert.deepEqual(await auditManifests(root), {
      failures: [
        {
          rule: "provider-version-parity",
          path: ".github/plugin/plugin.json",
          message:
            'version "\ufffd" != .claude-plugin/plugin.json version "1.0.0"',
        },
      ],
    });
  });
});

test("an escaped slash and a plain one", async () => {
  await withManifestsRepo(async (root) => {
    versions(root, '"a\\/b"', '"a/b"');
    assert.deepEqual(await auditManifests(root), {
      failures: [
        {
          rule: "provider-version-parity",
          path: ".github/plugin/plugin.json",
          message:
            'version "a/b" != .claude-plugin/plugin.json version "1.0.0"',
        },
      ],
    });
  });
});

test("an escaped letter and the letter", async () => {
  await withManifestsRepo(async (root) => {
    versions(root, '"\\u0041"', '"A"');
    assert.deepEqual(await auditManifests(root), {
      failures: [
        {
          rule: "provider-version-parity",
          path: ".github/plugin/plugin.json",
          message: 'version "A" != .claude-plugin/plugin.json version "1.0.0"',
        },
      ],
    });
  });
});
