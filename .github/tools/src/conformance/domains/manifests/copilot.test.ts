import assert from "node:assert/strict";
import { mkdtempSync, statSync, symlinkSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { test } from "node:test";
import {
  readValues,
  UNSET,
  type ErrorKind,
  type ValueResult,
  type ValueToken,
} from "../../../release/version.ts";
import type { Failure } from "../../check.ts";
import { RepoFileError } from "../../repo-file-error.ts";
import { writeFile } from "../../testing/files.ts";
import {
  checkedEffectiveCopilotManifest,
  copilotProviderFailures,
  copilotProviderRules,
  effectiveCopilotManifest,
  sharedValue,
  type StatFn,
} from "./copilot.ts";
import { removeTree } from "../../testing/host.ts";
import {
  auditManifests,
  BREAKERS,
  CLAUDE_CATALOG,
  CLAUDE_PLUGIN,
  COPILOT_HOOKS,
  COPILOT_PLUGIN,
  editJson,
  mkdir,
  remove,
  withManifestsRepo,
} from "../../testing/manifests-repo.ts";
import {
  agenticsEntry,
  claudeCatalog,
  versions,
} from "../../testing/manifests-seeds.ts";

const CATALOG = ".claude-plugin/marketplace.json";
const CLAUDE = ".claude-plugin/plugin.json";
const GITHUB = ".github/plugin/plugin.json";
const HOOKS = "hooks/hooks.copilot.json";

const catalogJson = (entry: Record<string, unknown>): string =>
  JSON.stringify({ plugins: [entry] });

/** The files of a checkout that passes every Copilot rule. */
const pristine = (): Record<string, string> => ({
  [CATALOG]: catalogJson({ name: "agentics", version: "1.0.0", source: "./" }),
  [GITHUB]: JSON.stringify({
    name: "agentics",
    version: "1.0.0",
    skills: ["./skills/tdd"],
    hooks: "./hooks/hooks.copilot.json",
  }),
  [CLAUDE]: JSON.stringify({
    name: "agentics",
    version: "1.0.0",
    skills: ["./skills/tdd"],
    hooks: "./hooks/hooks.claude.json",
  }),
  [HOOKS]: "{}",
});

function withRoot<T>(
  files: Record<string, string>,
  body: (root: string) => T,
): T {
  const root = mkdtempSync(path.join(tmpdir(), "agentics-copilot-"));
  try {
    for (const [rel, content] of Object.entries(files))
      writeFile(root, rel, content);
    return body(root);
  } finally {
    removeTree(root);
  }
}

/** The rules over `pristine()` with `changes` applied; a null removes the file. */
function rules(changes: Record<string, string | null> = {}): Failure[] {
  const files: Record<string, string> = { ...pristine() };
  for (const [rel, content] of Object.entries(changes)) {
    if (content === null) delete files[rel];
    else files[rel] = content;
  }
  return withRoot(files, (root) =>
    copilotProviderRules(root, readValues(root)),
  );
}

/** The pristine Copilot manifest with some members replaced; an undefined member is dropped. */
const github = (changes: Record<string, unknown>): string =>
  JSON.stringify({
    ...(JSON.parse(pristine()[GITHUB] ?? "{}") as object),
    ...changes,
  });

test("a checkout that passes every Copilot rule has no failure", () => {
  assert.deepEqual(rules(), []);
});

/** Everything the rules can find wrong, in the order they run, with the failure each one produces when it comes first. */
interface State {
  github: Record<string, unknown> | null;
  claude: Record<string, unknown>;
  catalog: Record<string, unknown> | null;
  hooksFile: boolean;
  extra: Record<string, string>;
}
const STOPS: Array<{
  rule: string;
  path: string;
  message: string;
  apply: (state: State) => void;
}> = [
  {
    rule: "copilot-shadow-manifest",
    path: ".plugin/plugin.json",
    message:
      "higher-precedence Copilot shadow manifest .plugin/plugin.json must be absent",
    apply: (s) => void (s.extra[".plugin/plugin.json"] = "{}"),
  },
  {
    rule: "copilot-effective-manifest",
    path: CLAUDE,
    message: "effective Copilot manifest must be .github/plugin/plugin.json",
    apply: (s) => void (s.github = null),
  },
  {
    rule: "copilot-marketplace-present",
    path: CATALOG,
    message: `${CATALOG} must be present`,
    apply: (s) => void (s.catalog = null),
  },
  {
    rule: "copilot-marketplace-source",
    path: CATALOG,
    message: "agentics marketplace source must be exactly './'",
    apply: (s) => {
      if (s.catalog !== null)
        s.catalog = { name: "agentics", version: "1.0.0", source: "./x" };
    },
  },
  {
    rule: "copilot-marketplace-name-parity",
    path: GITHUB,
    message: "effective manifest name must match Copilot marketplace entry",
    apply: (s) => void (s.github && (s.github.name = "other")),
  },
  {
    rule: "copilot-marketplace-version-parity",
    path: GITHUB,
    message: "effective manifest version must match Copilot marketplace entry",
    apply: (s) => void (s.github && (s.github.version = "2.0.0")),
  },
  {
    rule: "copilot-hooks-field",
    path: GITHUB,
    message: "effective hooks must be exactly './hooks/hooks.copilot.json'",
    apply: (s) => void (s.github && (s.github.hooks = "./hooks/other.json")),
  },
  {
    rule: "copilot-hooks-target",
    path: HOOKS,
    message: "effective hooks target does not exist",
    apply: (s) => void (s.hooksFile = false),
  },
  {
    rule: "copilot-skills-parity",
    path: GITHUB,
    message: "effective skills must match Claude skills",
    apply: (s) => void (s.claude.skills = []),
  },
];

function stopFailures(first: number): Failure[] {
  const state: State = {
    github: JSON.parse(pristine()[GITHUB] ?? "{}") as Record<string, unknown>,
    claude: JSON.parse(pristine()[CLAUDE] ?? "{}") as Record<string, unknown>,
    catalog: { name: "agentics", version: "1.0.0", source: "./" },
    hooksFile: true,
    extra: {},
  };
  // The later faults go in first, so a fault that removes a file wins over one that edits it.
  for (const stop of STOPS.slice(first).reverse()) stop.apply(state);
  const files: Record<string, string> = { ...state.extra };
  if (state.github !== null) files[GITHUB] = JSON.stringify(state.github);
  files[CLAUDE] = JSON.stringify(state.claude);
  if (state.catalog !== null) files[CATALOG] = catalogJson(state.catalog);
  if (state.hooksFile) files[HOOKS] = "{}";
  return withRoot(files, (root) =>
    copilotProviderRules(root, readValues(root)),
  );
}

test("each Copilot rule reports its failure and stops the rules that follow it", () => {
  STOPS.forEach((stop, index) => {
    assert.deepEqual(
      stopFailures(index),
      [{ rule: stop.rule, path: stop.path, message: stop.message }],
      stop.rule,
    );
  });
  assert.equal(STOPS.length, 9);
});

test("a shadow manifest is found at the second candidate, the first one found is reported, and a directory counts", () => {
  const shadow = (rel: string) =>
    `higher-precedence Copilot shadow manifest ${rel} must be absent`;
  assert.deepEqual(rules({ "plugin.json": "{}" }), [
    {
      rule: "copilot-shadow-manifest",
      path: "plugin.json",
      message: shadow("plugin.json"),
    },
  ]);
  assert.deepEqual(
    rules({ "plugin.json": "{}", ".plugin/plugin.json": "{}" }).map(
      (f) => f.path,
    ),
    [".plugin/plugin.json"],
  );
  assert.deepEqual(
    rules({ ".plugin/plugin.json/inner": "x" }).map((f) => f.path),
    [".plugin/plugin.json"],
  );
});

test("the rules are silent when a file the effective manifest rules read is missing or not valid", () => {
  for (const text of ["{", "[]", "7", ""]) {
    assert.deepEqual(
      rules({ [CATALOG]: text }),
      [],
      `catalog ${JSON.stringify(text)}`,
    );
    assert.deepEqual(
      rules({ [GITHUB]: text }),
      [],
      `manifest ${JSON.stringify(text)}`,
    );
    assert.deepEqual(
      rules({ [CLAUDE]: text }),
      [],
      `claude ${JSON.stringify(text)}`,
    );
  }
});

test("the marketplace source rule counts every agentics entry that is an object, whatever else the catalog holds", () => {
  const agentics = { name: "agentics", version: "1.0.0", source: "./" };
  const two = JSON.stringify({ plugins: [agentics, agentics] });
  const extra = JSON.stringify({
    plugins: [agentics, { name: "other" }, null],
  });
  const none = JSON.stringify({ plugins: [{ name: "other" }] });
  const message = "agentics marketplace source must be exactly './'";
  assert.deepEqual(
    rules({ [CATALOG]: two }).map((f) => f.message),
    [message],
  );
  assert.deepEqual(
    rules({ [CATALOG]: none }).map((f) => f.message),
    [message],
  );
  assert.deepEqual(rules({ [CATALOG]: extra }), []);
  // A non-object entry makes the version reader reject the whole catalog (an unset token), so the source rule passes
  // and the version rule, which compares tokens, is the one that fires.
  assert.deepEqual(
    rules({ [CATALOG]: JSON.stringify({ plugins: [agentics, "x"] }) }).map(
      (f) => f.rule,
    ),
    ["copilot-marketplace-version-parity"],
  );
  for (const source of [5, null, ".", "./x", "./ "])
    assert.deepEqual(
      rules({ [CATALOG]: catalogJson({ ...agentics, source }) }).map(
        (f) => f.message,
      ),
      [message],
      JSON.stringify(source),
    );
});

test("the name rule is a deep equality, so a missing or non-string name differs", () => {
  for (const name of [undefined, 5, "Agentics", null, ["agentics"]])
    assert.deepEqual(
      rules({ [GITHUB]: github({ name }) }).map((f) => f.rule),
      ["copilot-marketplace-name-parity"],
      JSON.stringify(name),
    );
});

test("the version rule compares tokens: 1 equals 1.0, a missing version equals null, and a string is not a number", () => {
  const entry = (version?: unknown): string =>
    catalogJson({
      name: "agentics",
      ...(version === undefined ? {} : { version }),
      source: "./",
    });
  /** The manifest text carries the version as written, so `1.0` stays `1.0`. */
  const manifest = (version: string): string =>
    github({ version: undefined }).replace("{", `{"version":${version},`);
  const parity = (catalog: string, version: string): string[] =>
    rules({ [CATALOG]: catalog, [GITHUB]: manifest(version) }).map(
      (f) => f.rule,
    );
  const differs = ["copilot-marketplace-version-parity"];
  assert.deepEqual(parity(entry(1), "1.0"), []);
  assert.deepEqual(parity(entry(1), "1e0"), []);
  assert.deepEqual(parity(entry(), "null"), []);
  assert.deepEqual(parity(entry(null), "null"), []);
  assert.deepEqual(parity(entry("1"), "1"), differs);
  assert.deepEqual(parity(entry(1), "2"), differs);
  assert.deepEqual(parity(entry("1.0.0"), "null"), differs);
  assert.deepEqual(parity(entry(), '"1.0.0"'), differs);
});

function result(
  file: string,
  kind: ErrorKind,
  observed: ValueToken = UNSET,
): ValueResult {
  return {
    path: file,
    raw: "",
    kind,
    error: undefined,
    value: observed,
    missing: kind === "read",
  };
}

test("sharedValue gives no token for a file that was not read or is not JSON, and keeps a shape result", () => {
  const one = { set: true, json: "1" };
  const values = [
    result("a", "read"),
    result("b", "json"),
    result("c", "shape", one),
    result("d", "none", one),
    result("e", "version", one),
    result("f", "shape"),
  ];
  assert.equal(sharedValue(values, "a"), undefined);
  assert.equal(sharedValue(values, "b"), undefined);
  assert.deepEqual(sharedValue(values, "c"), one);
  assert.deepEqual(sharedValue(values, "d"), one);
  assert.deepEqual(sharedValue(values, "e"), one);
  assert.deepEqual(sharedValue(values, "f"), UNSET);
  assert.equal(sharedValue(values, "absent"), undefined);
});

test("two files that cannot be read never read as version parity, and one that can is compared only with another that can", () => {
  const set = { set: true, json: '"1.0.0"' };
  const run = (catalog: ValueResult, manifest: ValueResult): string[] =>
    withRoot(pristine(), (root) =>
      copilotProviderRules(root, [catalog, manifest]).map((f) => f.rule),
    );
  // The files on disk agree on everything else, so only the token comparison decides.
  const catalogRead = result(CATALOG, "read");
  const manifestJson = result(GITHUB, "json");
  assert.deepEqual(run(catalogRead, manifestJson), []);
  assert.deepEqual(run(result(CATALOG, "none", set), manifestJson), []);
  assert.deepEqual(run(catalogRead, result(GITHUB, "none", set)), []);
  assert.deepEqual(
    run(result(CATALOG, "none", set), result(GITHUB, "none", set)),
    [],
  );
  assert.deepEqual(
    run(
      result(CATALOG, "none", set),
      result(GITHUB, "none", { set: true, json: '"2"' }),
    ),
    ["copilot-marketplace-version-parity"],
  );
  assert.deepEqual(run(result(CATALOG, "none", set), result(GITHUB, "shape")), [
    "copilot-marketplace-version-parity",
  ]);
});

test("the hooks rules need the exact field value and a hooks target that is not a directory", () => {
  for (const hooks of [
    undefined,
    5,
    "./hooks/hooks.copilot.json ",
    ["./hooks/hooks.copilot.json"],
    null,
  ])
    assert.deepEqual(
      rules({ [GITHUB]: github({ hooks }) }).map((f) => f.rule),
      ["copilot-hooks-field"],
      JSON.stringify(hooks),
    );
  assert.deepEqual(
    rules({ [HOOKS]: null, [`${HOOKS}/inner`]: "x" }).map((f) => f.rule),
    ["copilot-hooks-target"],
  );
  assert.deepEqual(
    rules({ [HOOKS]: null }).map((f) => f.rule),
    ["copilot-hooks-target"],
  );
});

test("the skills rule compares the two lists in order, and a value that is not an array counts as absent", () => {
  const claude = (skills?: unknown): string =>
    JSON.stringify({
      name: "agentics",
      version: "1.0.0",
      ...(skills === undefined ? {} : { skills }),
    });
  const verdict = (claudeSkills: unknown, copilotSkills: unknown): string[] =>
    rules({
      [CLAUDE]: claude(claudeSkills),
      [GITHUB]: github({ skills: copilotSkills }),
    }).map((f) => f.rule);
  const failed = ["copilot-skills-parity"];
  assert.deepEqual(verdict(["a", "b"], ["a", "b"]), []);
  assert.deepEqual(verdict(["a", "b"], ["b", "a"]), failed);
  assert.deepEqual(verdict(["a", "b"], ["a"]), failed);
  assert.deepEqual(verdict(["a"], ["a", "b"]), failed);
  assert.deepEqual(verdict([1], [1.0]), []);
  assert.deepEqual(verdict([{ a: 1, b: 2 }], [{ b: 2, a: 1 }]), []);
  assert.deepEqual(verdict([[1]], [[2]]), failed);
  assert.deepEqual(verdict([], []), []);
  assert.deepEqual(verdict(undefined, undefined), []);
  assert.deepEqual(verdict([], undefined), failed);
  assert.deepEqual(verdict(undefined, []), failed);
  assert.deepEqual(verdict("./skills/", undefined), []);
  assert.deepEqual(verdict(undefined, "./skills/"), []);
  assert.deepEqual(verdict(null, { a: 1 }), []);
  assert.deepEqual(verdict(["a"], null), failed);
});

test("the effective manifest is the first candidate that is not a directory, in Copilot's order", () => {
  const effective = (files: Record<string, string>): string =>
    withRoot(files, (root) => effectiveCopilotManifest(root));
  assert.equal(effective({}), "");
  assert.equal(effective({ [CLAUDE]: "{}" }), CLAUDE);
  assert.equal(effective({ [CLAUDE]: "{}", [GITHUB]: "{}" }), GITHUB);
  assert.equal(
    effective({ [CLAUDE]: "{}", [GITHUB]: "{}", "plugin.json": "{}" }),
    "plugin.json",
  );
  assert.equal(
    effective({
      [CLAUDE]: "{}",
      [GITHUB]: "{}",
      "plugin.json": "{}",
      ".plugin/plugin.json": "{}",
    }),
    ".plugin/plugin.json",
  );
  // A candidate that is a directory is passed over.
  assert.equal(effective({ [CLAUDE]: "{}", [`${GITHUB}/inner`]: "x" }), CLAUDE);
  assert.equal(effective({ [`${GITHUB}/inner`]: "x" }), "");
});

const denied = Object.assign(new Error("denied"), { code: "EACCES" });
const gone = Object.assign(new Error("gone"), { code: "ENOENT" });

test("a candidate that cannot be examined is a discovery error, read as none by effectiveCopilotManifest", () => {
  withRoot(pristine(), (root) => {
    const failOn =
      (rel: string, error: Error): StatFn =>
      (file) => {
        if (file === path.join(root, ...rel.split("/"))) throw error;
        return statSync(file);
      };
    const stat = failOn(".plugin/plugin.json", denied);
    assert.equal(effectiveCopilotManifest(root, stat), "");
    assert.throws(
      () => checkedEffectiveCopilotManifest(root, stat),
      (error) =>
        error instanceof Error &&
        error.message ===
          "discover Copilot manifest: .plugin/plugin.json: EACCES" &&
        error.cause instanceof RepoFileError &&
        error.cause.kind === "unreadable",
    );
    assert.throws(
      () => copilotProviderRules(root, readValues(root), stat),
      /^Error: discover Copilot manifest: \.plugin\/plugin\.json: EACCES$/,
    );
    // A candidate reported as absent is skipped.
    assert.equal(
      effectiveCopilotManifest(root, failOn(".plugin/plugin.json", gone)),
      GITHUB,
    );
  });
});

test("a shadow candidate that cannot be examined is an audit error naming it", () => {
  withRoot(pristine(), (root) => {
    const lstat: StatFn = (file) => {
      if (file === path.join(root, ".plugin", "plugin.json")) throw denied;
      return statSync(file);
    };
    assert.throws(
      () => copilotProviderRules(root, readValues(root), statSync, lstat),
      (error) =>
        error instanceof Error &&
        error.message ===
          "inspect Copilot shadow manifest: .plugin/plugin.json: EACCES" &&
        error.cause instanceof RepoFileError &&
        error.cause.kind === "unreadable",
    );
  });
});

test("a dangling symlink at a shadow candidate is a shadow manifest, because the check is an lstat", (t) => {
  withRoot(pristine(), (root) => {
    try {
      symlinkSync(
        path.join(root, "no-such-target.json"),
        path.join(root, "plugin.json"),
      );
    } catch (error) {
      t.skip(`this host cannot create a symlink: ${String(error)}`);
      return;
    }
    assert.deepEqual(copilotProviderRules(root, readValues(root)), [
      {
        rule: "copilot-shadow-manifest",
        path: "plugin.json",
        message:
          "higher-precedence Copilot shadow manifest plugin.json must be absent",
      },
    ]);
  });
});

test("discovery runs before the shadow check, so a discovery error wins over a shadow failure", () => {
  withRoot({ ...pristine(), ".plugin/plugin.json/inner": "x" }, (root) => {
    const stat: StatFn = (file) => {
      if (file === path.join(root, "plugin.json")) throw denied;
      return statSync(file);
    };
    assert.throws(
      () => copilotProviderRules(root, readValues(root), stat),
      /^Error: discover Copilot manifest: plugin\.json: EACCES$/,
    );
  });
});

test("an unreadable version file is an audit error to the rules, and the single copilot-audit failure to copilotProviderFailures", () => {
  const files = { ...pristine() };
  delete files[CATALOG];
  files[`${CATALOG}/inner`] = "x"; // a directory where the catalog should be
  withRoot(files, (root) => {
    assert.throws(
      () => copilotProviderRules(root, readValues(root)),
      /^Error: \.claude-plugin\/marketplace\.json: is not a regular file$/,
    );
    const failures = copilotProviderFailures(root);
    assert.equal(failures.length, 1);
    assert.equal(failures[0]?.rule, "copilot-audit");
    assert.equal(failures[0]?.path, root);
    assert.match(
      failures[0]?.message ?? "",
      /^\.claude-plugin\/marketplace\.json: is not a regular file$/,
    );
  });
});

test("copilotProviderFailures reads the version values itself and returns the rule failures otherwise", () => {
  assert.deepEqual(
    withRoot(pristine(), (root) => copilotProviderFailures(root)),
    [],
  );
  const files = { ...pristine(), [GITHUB]: github({ hooks: "./x" }) };
  assert.deepEqual(
    withRoot(files, (root) => copilotProviderFailures(root)).map((f) => f.rule),
    ["copilot-hooks-field"],
  );
});

test("a version file that does not exist is not an audit error", () => {
  const files = { ...pristine() };
  delete files[GITHUB];
  assert.deepEqual(
    withRoot(files, (root) => copilotProviderFailures(root)).map((f) => f.rule),
    ["copilot-effective-manifest"],
  );
});

test("copilot-shadow-manifest reports a plugin.json at the checkout root", async () => {
  await withManifestsRepo(async (root) => {
    BREAKERS["copilot-shadow-manifest"]!(root);
    assert.deepEqual(await auditManifests(root), {
      failures: [
        {
          rule: "copilot-shadow-manifest",
          path: "plugin.json",
          message:
            "higher-precedence Copilot shadow manifest plugin.json must be absent",
        },
      ],
    });
  });
});

test("copilot-effective-manifest reports a missing Copilot manifest, and its version parity fails too", async () => {
  await withManifestsRepo(async (root) => {
    BREAKERS["copilot-effective-manifest"]!(root);
    assert.deepEqual(await auditManifests(root), {
      failures: [
        {
          rule: "copilot-effective-manifest",
          path: ".claude-plugin/plugin.json",
          message:
            "effective Copilot manifest must be .github/plugin/plugin.json",
        },
        {
          rule: "provider-version-parity",
          path: ".github/plugin/plugin.json",
          message: "is missing",
        },
      ],
    });
  });
});

test("copilot-marketplace-source reports a Claude catalog source other than ./", async () => {
  await withManifestsRepo(async (root) => {
    BREAKERS["copilot-marketplace-source"]!(root);
    assert.deepEqual(await auditManifests(root), {
      failures: [
        {
          rule: "copilot-marketplace-source",
          path: ".claude-plugin/marketplace.json",
          message: "agentics marketplace source must be exactly './'",
        },
      ],
    });
  });
});

test("copilot-marketplace-name-parity reports a Copilot manifest named other than its catalog entry", async () => {
  await withManifestsRepo(async (root) => {
    BREAKERS["copilot-marketplace-name-parity"]!(root);
    assert.deepEqual(await auditManifests(root), {
      failures: [
        {
          rule: "copilot-marketplace-name-parity",
          path: ".github/plugin/plugin.json",
          message:
            "effective manifest name must match Copilot marketplace entry",
        },
      ],
    });
  });
});

test("copilot-marketplace-version-parity reports a Copilot manifest version other than its catalog entry, and provider parity fails too", async () => {
  await withManifestsRepo(async (root) => {
    BREAKERS["copilot-marketplace-version-parity"]!(root);
    assert.deepEqual(await auditManifests(root), {
      failures: [
        {
          rule: "copilot-marketplace-version-parity",
          path: ".github/plugin/plugin.json",
          message:
            "effective manifest version must match Copilot marketplace entry",
        },
        {
          rule: "provider-version-parity",
          path: ".github/plugin/plugin.json",
          message:
            'version "2.0.0" != .claude-plugin/plugin.json version "1.0.0"',
        },
      ],
    });
  });
});

test("copilot-hooks-field reports a Copilot manifest whose hooks field names another provider file", async () => {
  await withManifestsRepo(async (root) => {
    BREAKERS["copilot-hooks-field"]!(root);
    assert.deepEqual(await auditManifests(root), {
      failures: [
        {
          rule: "copilot-hooks-field",
          path: ".github/plugin/plugin.json",
          message:
            "effective hooks must be exactly './hooks/hooks.copilot.json'",
        },
      ],
    });
  });
});

test("copilot-skills-parity reports a Copilot skills list that differs from the Claude one", async () => {
  await withManifestsRepo(async (root) => {
    BREAKERS["copilot-skills-parity"]!(root);
    assert.deepEqual(await auditManifests(root), {
      failures: [
        {
          rule: "copilot-skills-parity",
          path: ".github/plugin/plugin.json",
          message: "effective skills must match Claude skills",
        },
      ],
    });
  });
});

test("a catalog entry whose version key is capitalized", async () => {
  await withManifestsRepo(async (root) => {
    writeFile(
      root,
      CLAUDE_CATALOG,
      claudeCatalog(agenticsEntry('"Version":"2.0.0",')),
    );
    assert.deepEqual(await auditManifests(root), {
      failures: [
        {
          rule: "copilot-marketplace-version-parity",
          path: ".github/plugin/plugin.json",
          message:
            "effective manifest version must match Copilot marketplace entry",
        },
      ],
    });
  });
});

// The version reader matches `version` exactly, so a member spelled `VERSION` does not change what it reads.
test("a version key repeated in another case, the upper-case one last", async () => {
  await withManifestsRepo(async (root) => {
    writeFile(
      root,
      CLAUDE_CATALOG,
      claudeCatalog(agenticsEntry('"version":"1.0.0","VERSION":"2.0.0",')),
    );
    assert.deepEqual(await auditManifests(root), { failures: [] });
  });
});

// The reader matches `name` exactly, so a member spelled `NAME` does not change the entry or its version.
test("a name key repeated in another case with another type", async () => {
  await withManifestsRepo(async (root) => {
    writeFile(
      root,
      CLAUDE_CATALOG,
      claudeCatalog(
        '{"name":"agentics","NAME":5,"version":"1.0.0","source":"./"}',
      ),
    );
    assert.deepEqual(await auditManifests(root), { failures: [] });
  });
});

test("version 1 and 2", async () => {
  await withManifestsRepo(async (root) => {
    versions(root, "1", "2");
    assert.deepEqual(await auditManifests(root), {
      failures: [
        {
          rule: "copilot-marketplace-version-parity",
          path: ".github/plugin/plugin.json",
          message:
            "effective manifest version must match Copilot marketplace entry",
        },
        {
          rule: "provider-version-parity",
          path: ".github/plugin/plugin.json",
          message: 'version "" != .claude-plugin/plugin.json version "1.0.0"',
        },
      ],
    });
  });
});

test("a string version and a number", async () => {
  await withManifestsRepo(async (root) => {
    versions(root, '"1"', "1");
    assert.deepEqual(await auditManifests(root), {
      failures: [
        {
          rule: "copilot-marketplace-version-parity",
          path: ".github/plugin/plugin.json",
          message:
            "effective manifest version must match Copilot marketplace entry",
        },
        {
          rule: "provider-version-parity",
          path: ".github/plugin/plugin.json",
          message: 'version "" != .claude-plugin/plugin.json version "1.0.0"',
        },
      ],
    });
  });
});

test("the Copilot manifest is null", async () => {
  await withManifestsRepo(async (root) => {
    writeFile(root, COPILOT_PLUGIN, "null");
    assert.deepEqual(await auditManifests(root), {
      failures: [
        {
          rule: "copilot-marketplace-name-parity",
          path: ".github/plugin/plugin.json",
          message:
            "effective manifest name must match Copilot marketplace entry",
        },
        {
          rule: "provider-version-parity",
          path: ".github/plugin/plugin.json",
          message: 'version "" != .claude-plugin/plugin.json version "1.0.0"',
        },
      ],
    });
  });
});

test("a shadow manifest at .plugin/plugin.json and one at plugin.json", async () => {
  await withManifestsRepo(async (root) => {
    writeFile(root, ".plugin/plugin.json", "{}");
    writeFile(root, "plugin.json", "{}");
    assert.deepEqual(await auditManifests(root), {
      failures: [
        {
          rule: "copilot-shadow-manifest",
          path: ".plugin/plugin.json",
          message:
            "higher-precedence Copilot shadow manifest .plugin/plugin.json must be absent",
        },
      ],
    });
  });
});

test("a shadow manifest that is a directory at .plugin/plugin.json", async () => {
  await withManifestsRepo(async (root) => {
    mkdir(root, ".plugin/plugin.json");
    assert.deepEqual(await auditManifests(root), {
      failures: [
        {
          rule: "copilot-shadow-manifest",
          path: ".plugin/plugin.json",
          message:
            "higher-precedence Copilot shadow manifest .plugin/plugin.json must be absent",
        },
      ],
    });
  });
});

test("a shadow manifest that is a directory at plugin.json", async () => {
  await withManifestsRepo(async (root) => {
    mkdir(root, "plugin.json");
    assert.deepEqual(await auditManifests(root), {
      failures: [
        {
          rule: "copilot-shadow-manifest",
          path: "plugin.json",
          message:
            "higher-precedence Copilot shadow manifest plugin.json must be absent",
        },
      ],
    });
  });
});

test("a shadow manifest stops the Copilot rules before the hooks field", async () => {
  await withManifestsRepo(async (root) => {
    writeFile(root, "plugin.json", "{}");
    editJson(
      root,
      COPILOT_PLUGIN,
      (v) => (v.hooks = "./hooks/hooks.codex.json"),
    );
    assert.deepEqual(await auditManifests(root), {
      failures: [
        {
          rule: "copilot-shadow-manifest",
          path: "plugin.json",
          message:
            "higher-precedence Copilot shadow manifest plugin.json must be absent",
        },
      ],
    });
  });
});

test("a name mismatch stops the Copilot rules before the version", async () => {
  await withManifestsRepo(async (root) => {
    editJson(root, COPILOT_PLUGIN, (v) => {
      v.name = "other";
      v.version = "2.0.0";
    });
    assert.deepEqual(await auditManifests(root), {
      failures: [
        {
          rule: "copilot-marketplace-name-parity",
          path: ".github/plugin/plugin.json",
          message:
            "effective manifest name must match Copilot marketplace entry",
        },
        {
          rule: "provider-version-parity",
          path: ".github/plugin/plugin.json",
          message:
            'version "2.0.0" != .claude-plugin/plugin.json version "1.0.0"',
        },
      ],
    });
  });
});

test("a version mismatch stops the Copilot rules before the hooks field", async () => {
  await withManifestsRepo(async (root) => {
    editJson(root, COPILOT_PLUGIN, (v) => {
      v.version = "2.0.0";
      v.hooks = "./hooks/hooks.codex.json";
    });
    assert.deepEqual(await auditManifests(root), {
      failures: [
        {
          rule: "copilot-marketplace-version-parity",
          path: ".github/plugin/plugin.json",
          message:
            "effective manifest version must match Copilot marketplace entry",
        },
        {
          rule: "provider-version-parity",
          path: ".github/plugin/plugin.json",
          message:
            'version "2.0.0" != .claude-plugin/plugin.json version "1.0.0"',
        },
      ],
    });
  });
});

test("a hooks field stops the Copilot rules before the hooks target", async () => {
  await withManifestsRepo(async (root) => {
    remove(root, COPILOT_HOOKS);
    editJson(
      root,
      COPILOT_PLUGIN,
      (v) => (v.hooks = "./hooks/hooks.codex.json"),
    );
    assert.deepEqual(await auditManifests(root), {
      failures: [
        {
          rule: "copilot-hooks-field",
          path: ".github/plugin/plugin.json",
          message:
            "effective hooks must be exactly './hooks/hooks.copilot.json'",
        },
      ],
    });
  });
});

test("skills in a different order", async () => {
  await withManifestsRepo(async (root) => {
    mkdir(root, "skills/b");
    editJson(
      root,
      CLAUDE_PLUGIN,
      (v) => (v.skills = ["./skills/tdd", "./skills/b"]),
    );
    editJson(
      root,
      COPILOT_PLUGIN,
      (v) => (v.skills = ["./skills/b", "./skills/tdd"]),
    );
    assert.deepEqual(await auditManifests(root), {
      failures: [
        {
          rule: "copilot-skills-parity",
          path: ".github/plugin/plugin.json",
          message: "effective skills must match Claude skills",
        },
      ],
    });
  });
});

test("an empty skills list against none", async () => {
  await withManifestsRepo(async (root) => {
    remove(root, "skills/tdd");
    editJson(root, CLAUDE_PLUGIN, (v) => (v.skills = []));
    editJson(root, COPILOT_PLUGIN, (v) => delete v.skills);
    assert.deepEqual(await auditManifests(root), {
      failures: [
        {
          rule: "copilot-skills-parity",
          path: ".github/plugin/plugin.json",
          message: "effective skills must match Claude skills",
        },
      ],
    });
  });
});

test("a Copilot manifest name of null", async () => {
  await withManifestsRepo(async (root) => {
    editJson(root, COPILOT_PLUGIN, (v) => (v.name = null));
    assert.deepEqual(await auditManifests(root), {
      failures: [
        {
          rule: "copilot-marketplace-name-parity",
          path: ".github/plugin/plugin.json",
          message:
            "effective manifest name must match Copilot marketplace entry",
        },
      ],
    });
  });
});
