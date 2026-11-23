import assert from "node:assert/strict";
import { mkdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { test } from "node:test";
import {
  formatVersion,
  lessThan,
  MANIFESTS,
  parseVersion,
  readSnapshot,
  readSnapshotAt,
  readValues,
  tagVersion,
  type ValueResult,
  type Version,
} from "./version.ts";
import { initRepo, commitFiles, scratchDir, sh } from "./testing/support.ts";

const FIXTURE = "9.8.7";
const PATHS = MANIFESTS.map((manifest) => manifest.path);
const MARKETPLACE = ".claude-plugin/marketplace.json";
const CODEX = ".codex-plugin/plugin.json";
const GITHUB = ".github/plugin/plugin.json";
const CLAUDE = ".claude-plugin/plugin.json";
const ANTIGRAVITY = ".antigravity-plugin/plugin.json";

function fixtureData(path: string, version: string): string {
  if (path.endsWith("marketplace.json")) {
    return JSON.stringify({
      name: "muehz",
      plugins: [
        { name: "other", version: "0.0.0" },
        { name: "agentics", version, source: "./" },
      ],
    });
  }
  return JSON.stringify({ name: "agentics", version });
}

function writeVersionTree(version: string): string {
  const root = scratchDir("agentics-version-");
  for (const path of PATHS) {
    const full = join(root, path);
    mkdirSync(dirname(full), { recursive: true });
    writeFileSync(full, fixtureData(path, version));
  }
  return root;
}

type Json = Record<string, unknown>;

function mutateJSON(
  root: string,
  path: string,
  mutate: (object: Json) => void,
) {
  const full = join(root, path);
  const object = JSON.parse(readFileSync(full, "utf8")) as Json;
  mutate(object);
  writeFileSync(full, JSON.stringify(object));
}

function agenticsPlugin(object: Json): Json {
  const plugins = object.plugins as Json[];
  const found = plugins.find((plugin) => plugin.name === "agentics");
  assert.ok(found, "fixture has no Agentics plugin");
  return found;
}

function setVersion(root: string, path: string, version: string) {
  mutateJSON(root, path, (object) => {
    if (path.endsWith("marketplace.json"))
      agenticsPlugin(object).version = version;
    else object.version = version;
  });
}

function messageOf(body: () => unknown): string {
  try {
    body();
  } catch (error) {
    return (error as Error).message;
  }
  assert.fail("expected an error");
}

test("parseVersion accepts canonical versions", () => {
  assert.deepEqual(parseVersion("0.0.0"), { major: 0n, minor: 0n, patch: 0n });
  assert.deepEqual(parseVersion("1.23.456"), {
    major: 1n,
    minor: 23n,
    patch: 456n,
  });
  const max = 18446744073709551615n;
  assert.deepEqual(
    parseVersion(
      "18446744073709551615.18446744073709551615.18446744073709551615",
    ),
    { major: max, minor: max, patch: max },
  );
});

test("parseVersion rejects everything else as an invalid agentics version", () => {
  for (const input of [
    "v1.2.3",
    "1.2.3-dev",
    "1.2",
    "1..3",
    ".2.3",
    "01.2.3",
    "1.02.3",
    "1.2.03",
    "",
    "-1.2.3",
    " 1.2.3",
    "1.2.3\n",
    "1.2.3 ",
    "18446744073709551616.2.3",
    "1.18446744073709551616.3",
    "1.2.18446744073709551616",
  ]) {
    assert.equal(
      messageOf(() => parseVersion(input)),
      "invalid agentics version",
      JSON.stringify(input),
    );
  }
});

test("formatVersion prints major.minor.patch", () => {
  assert.equal(
    formatVersion({ major: 1n, minor: 23n, patch: 456n }),
    "1.23.456",
  );
  assert.equal(
    formatVersion(parseVersion("18446744073709551615.0.1")),
    "18446744073709551615.0.1",
  );
});

test("lessThan orders by major, then minor, then patch", () => {
  const v = (text: string): Version => parseVersion(text);
  const cases: [string, string, boolean][] = [
    ["1.2.3", "1.2.4", true],
    ["1.2.3", "1.3.0", true],
    ["1.2.3", "2.0.0", true],
    ["1.1.9", "1.2.0", true],
    ["1.2.3", "1.2.3", false],
    ["1.2.4", "1.2.3", false],
    ["1.3.0", "1.2.9", false],
    ["2.0.0", "1.9.9", false],
  ];
  for (const [a, b, want] of cases) {
    assert.equal(lessThan(v(a), v(b)), want, `${a} < ${b}`);
  }
});

test("MANIFESTS lists the marketplace first, then the four plugin manifests", () => {
  assert.deepEqual(MANIFESTS, [
    { path: MARKETPLACE, kind: "marketplace" },
    { path: CLAUDE, kind: "plugin" },
    { path: CODEX, kind: "plugin" },
    { path: GITHUB, kind: "plugin" },
    { path: ANTIGRAVITY, kind: "plugin" },
  ]);
});

// check-commit's handling of non-stable and unorderable tags depends on this reading.
test("tagVersion orders stable tags and rejects the rest", () => {
  assert.deepEqual(tagVersion("v1.2.3"), [1n, 2n, 3n]);
  assert.deepEqual(tagVersion("v1.03.0"), [1n, 3n, 0n]);
  assert.deepEqual(tagVersion("v9223372036854775807.0.0"), [
    9223372036854775807n,
    0n,
    0n,
  ]);
  for (const tag of [
    "v1.4.0-rc.1",
    "vfoo",
    "v2.0",
    "1.2.3",
    "v1.2.3\n",
    "v2.99999999999999999999.0",
    "v9223372036854775808.0.0",
    "v0.0.9223372036854775808",
  ]) {
    assert.equal(tagVersion(tag), undefined, tag);
  }
});

test("readSnapshot reads all five manifests", () => {
  const root = writeVersionTree(FIXTURE);
  assert.deepEqual(readSnapshot(root), {
    version: { major: 9n, minor: 8n, patch: 7n },
    raw: FIXTURE,
  });
});

test("readSnapshot reads the plugin entry, not the marketplace metadata", () => {
  const root = writeVersionTree(FIXTURE);
  mutateJSON(root, MARKETPLACE, (object) => {
    object.metadata = { version: "0.0.0" };
  });
  assert.equal(readSnapshot(root).raw, FIXTURE);
});

test("readSnapshot refuses a manifest it cannot read, naming the path", () => {
  const cases: {
    name: string;
    path: string;
    prepare: (root: string) => void;
    want: (root: string) => string;
  }[] = [
    {
      name: "missing file",
      path: CODEX,
      prepare: (root) => rmSync(join(root, CODEX)),
      want: () => `${CODEX}: does not exist`,
    },
    {
      name: "malformed JSON",
      path: GITHUB,
      prepare: (root) => writeFileSync(join(root, GITHUB), "{"),
      want: () => `${GITHUB}: is not valid JSON`,
    },
    {
      name: "missing version",
      path: CLAUDE,
      prepare: (root) => mutateJSON(root, CLAUDE, (o) => void delete o.version),
      want: () => `${CLAUDE}: has no "version" field`,
    },
    {
      name: "wrong version type",
      path: CODEX,
      prepare: (root) => mutateJSON(root, CODEX, (o) => void (o.version = 181)),
      want: () => `${CODEX}: "version" is not a string`,
    },
    {
      name: "missing marketplace version",
      path: MARKETPLACE,
      prepare: (root) =>
        mutateJSON(
          root,
          MARKETPLACE,
          (o) => void delete agenticsPlugin(o).version,
        ),
      want: () => `${MARKETPLACE}: has no "version" field`,
    },
    {
      name: "wrong marketplace version type",
      path: MARKETPLACE,
      prepare: (root) =>
        mutateJSON(
          root,
          MARKETPLACE,
          (o) => void (agenticsPlugin(o).version = 181),
        ),
      want: () => `${MARKETPLACE}: "version" is not a string`,
    },
    {
      name: "zero Agentics marketplace entries",
      path: MARKETPLACE,
      prepare: (root) =>
        mutateJSON(root, MARKETPLACE, (o) => {
          o.plugins = (o.plugins as Json[]).filter(
            (p) => p.name !== "agentics",
          );
        }),
      want: () =>
        `${MARKETPLACE}: expected exactly one Agentics marketplace entry, found 0`,
    },
    {
      name: "duplicate Agentics marketplace entries",
      path: MARKETPLACE,
      prepare: (root) =>
        mutateJSON(root, MARKETPLACE, (o) => {
          (o.plugins as Json[]).push({ ...agenticsPlugin(o) });
        }),
      want: () =>
        `${MARKETPLACE}: expected exactly one Agentics marketplace entry, found 2`,
    },
    {
      name: "invalid version",
      path: GITHUB,
      prepare: (root) => setVersion(root, GITHUB, `v${FIXTURE}`),
      want: () => `${GITHUB}: version "v${FIXTURE}": invalid agentics version`,
    },
  ];
  for (const { name, prepare, want } of cases) {
    const root = writeVersionTree(FIXTURE);
    prepare(root);
    assert.equal(
      messageOf(() => readSnapshot(root)),
      want(root),
      name,
    );
  }
});

test("readSnapshot reports the first failing manifest in manifest order", () => {
  const root = writeVersionTree(FIXTURE);
  writeFileSync(join(root, GITHUB), "{");
  rmSync(join(root, CLAUDE));
  assert.equal(
    messageOf(() => readSnapshot(root)),
    `${CLAUDE}: does not exist`,
  );
});

test("readSnapshot refuses a version that differs between manifests", () => {
  for (const path of PATHS) {
    const root = writeVersionTree(FIXTURE);
    setVersion(root, path, "9.8.8");
    const [common, ...others] =
      path === MARKETPLACE ? ["9.8.8", FIXTURE] : [FIXTURE, "9.8.8"];
    const conflicting = path === MARKETPLACE ? CLAUDE : path;
    const want = `version mismatch: ${MARKETPLACE}=${JSON.stringify(common)} conflicts with ${conflicting}=${JSON.stringify(others[0])}`;
    assert.equal(
      messageOf(() => readSnapshot(root)),
      want,
      path,
    );
  }
});

test("readSnapshot names the file and the failure for text that is not JSON and JSON of the wrong shape", () => {
  const cases: [string, string, string][] = [
    [CODEX, "", "is not valid JSON"],
    [CODEX, '{"version":"1.2.3",}', "is not valid JSON"],
    [CODEX, "[]", "is not a JSON object"],
    [CODEX, "true", "is not a JSON object"],
    [CODEX, "null", 'has no "version" field'],
    [CODEX, "{}", 'has no "version" field'],
    [CODEX, '{"version":null}', '"version" is not a string'],
    [CODEX, '{"version":181}', '"version" is not a string'],
    [CODEX, '{"version":[1]}', '"version" is not a string'],
    [CODEX, '{"Version":"9.8.7"}', 'has no "version" field'],
    [MARKETPLACE, "null", "missing plugins array"],
    [MARKETPLACE, "{}", "missing plugins array"],
    [MARKETPLACE, "5", "is not a JSON object"],
    [MARKETPLACE, "[]", "is not a JSON object"],
    [
      MARKETPLACE,
      '{"plugins":null}',
      "expected exactly one Agentics marketplace entry, found 0",
    ],
    [
      MARKETPLACE,
      '{"plugins":{}}',
      'invalid plugins array: "plugins" is not an array',
    ],
    [
      MARKETPLACE,
      '{"plugins":"x"}',
      'invalid plugins array: "plugins" is not an array',
    ],
    [
      MARKETPLACE,
      '{"plugins":["x"]}',
      "invalid plugins array: entry 0 is not an object",
    ],
    [
      MARKETPLACE,
      '{"plugins":[{"name":"other"},[1]]}',
      "invalid plugins array: entry 1 is not an object",
    ],
    [
      MARKETPLACE,
      '{"plugins":[{"name":5}]}',
      'invalid plugins array: entry 0 "name" is not a string',
    ],
    [
      MARKETPLACE,
      '{"plugins":[null,{"name":null}]}',
      "expected exactly one Agentics marketplace entry, found 0",
    ],
    [
      MARKETPLACE,
      '{"plugins":[{"name":"agentics"}]}',
      'has no "version" field',
    ],
    [
      MARKETPLACE,
      '{"plugins":[{"name":"agentics","version":null}]}',
      '"version" is not a string',
    ],
  ];
  for (const [path, text, tail] of cases) {
    const root = writeVersionTree(FIXTURE);
    writeFileSync(join(root, path), text);
    assert.equal(
      messageOf(() => readSnapshot(root)),
      `${path}: ${tail}`,
      `${path} ${text}`,
    );
  }
});

// A byte-order mark is not part of JSON, and the reader does not strip one.
test("readSnapshot refuses a manifest that starts with a byte-order mark", () => {
  const root = writeVersionTree(FIXTURE);
  writeFileSync(
    join(root, CLAUDE),
    `${String.fromCharCode(0xfeff)}${fixtureData(CLAUDE, FIXTURE)}`,
  );
  assert.equal(
    messageOf(() => readSnapshot(root)),
    `${CLAUDE}: is not valid JSON`,
  );
});

test("readSnapshot takes the last of a repeated key", () => {
  const root = writeVersionTree(FIXTURE);
  writeFileSync(
    join(root, CODEX),
    `{"name":"agentics","version":"1.0.0","version":"${FIXTURE}"}`,
  );
  assert.equal(readSnapshot(root).raw, FIXTURE);
});

test("readValues returns one result per location, whatever its neighbors hold", () => {
  const root = writeVersionTree(FIXTURE);
  rmSync(join(root, CODEX));
  writeFileSync(join(root, GITHUB), "{");
  const results = readValues(root);
  assert.deepEqual(
    results.map((result) => result.path),
    PATHS,
  );
  assert.deepEqual(
    results.map((result) => result.kind),
    ["none", "none", "read", "json", "none"],
  );
  assert.equal(results[0]?.raw, FIXTURE);
  assert.equal(results[1]?.error, undefined);
  results[0]!.raw = "mutated";
  assert.equal(readValues(root)[0]?.raw, FIXTURE);
});

// A duplicate marketplace entry and a non-canonical version string both keep
// their raw value.
test("readValues classifies shape and version errors and keeps the raw value", () => {
  const root = writeVersionTree(FIXTURE);
  setVersion(root, CLAUDE, `v${FIXTURE}`);
  mutateJSON(root, CODEX, (o) => void (o.version = 181));
  mutateJSON(root, MARKETPLACE, (o) => {
    (o.plugins as Json[]).push(
      { name: "agentics", version: "9.8.8", source: "./" },
      { name: "agentics", version: 181, source: "./" },
    );
  });
  const results = readValues(root);
  assert.deepEqual(
    results.map((result) => [result.kind, result.raw]),
    [
      ["shape", "9.8.8"],
      ["version", `v${FIXTURE}`],
      ["shape", ""],
      ["none", FIXTURE],
      ["none", FIXTURE],
    ],
  );
});

test("readSnapshotAt reads the commit and leaves the repository alone", async () => {
  const root = scratchDir("agentics-version-");
  initRepo(root);
  const files: Record<string, string> = {};
  for (const path of PATHS) files[path] = fixtureData(path, FIXTURE);
  files["working-tree-sentinel.txt"] = "committed sentinel\n";
  const commit = commitFiles(root, files, "fixture");
  writeFileSync(
    join(root, "working-tree-sentinel.txt"),
    "working tree sentinel\n",
  );
  setVersion(root, CODEX, "9.8.8");
  const status = sh(root, "git", "status", "--porcelain");
  const manifest = readFileSync(join(root, CODEX), "utf8");

  assert.deepEqual(await readSnapshotAt(root, commit), {
    version: { major: 9n, minor: 8n, patch: 7n },
    raw: FIXTURE,
  });

  assert.equal(sh(root, "git", "rev-parse", "HEAD").trim(), commit);
  assert.equal(sh(root, "git", "branch", "--show-current").trim(), "main");
  assert.equal(sh(root, "git", "status", "--porcelain"), status);
  assert.equal(readFileSync(join(root, CODEX), "utf8"), manifest);
  assert.equal(
    readFileSync(join(root, "working-tree-sentinel.txt"), "utf8"),
    "working tree sentinel\n",
  );
});

// The root is not a repository, so a message that is not the commit check's own
// would show git was run.
test("readSnapshotAt rejects a malformed commit before running git", async () => {
  const root = scratchDir("agentics-version-");
  for (const commit of [
    "",
    "0123456789abcdef0123456789abcdef0123456",
    "0123456789abcdef0123456789abcdef012345678",
    "0123456789ABCDEF0123456789abcdef01234567",
    "0123456789abcdef0123456789abcdef0123456g",
    "0123456789abcdef0123456789abcdef01234567:.",
    "--output=HEAD",
  ]) {
    await assert.rejects(
      readSnapshotAt(root, commit),
      { message: `invalid commit SHA: ${JSON.stringify(commit)}` },
      commit,
    );
  }
});

function fixtureRepo(mutate: (files: Record<string, string>) => void): {
  root: string;
  commit: string;
} {
  const root = scratchDir("agentics-version-");
  initRepo(root);
  const files: Record<string, string> = {};
  for (const path of PATHS) files[path] = fixtureData(path, FIXTURE);
  mutate(files);
  return { root, commit: commitFiles(root, files, "fixture") };
}

test("readSnapshotAt reports a manifest missing at the commit", async () => {
  const { root, commit } = fixtureRepo(() => {});
  rmSync(join(root, CODEX));
  sh(root, "git", "rm", "--quiet", "--cached", CODEX);
  sh(root, "git", "commit", "--quiet", "-m", "drop");
  const dropped = sh(root, "git", "rev-parse", "HEAD").trim();
  assert.notEqual(dropped, commit);
  await assert.rejects(readSnapshotAt(root, dropped), {
    message: `${CODEX}: git show: git exited with code 128`,
  });
});

test("readSnapshotAt reports a version mismatch at the commit", async () => {
  const { root, commit } = fixtureRepo((files) => {
    files[GITHUB] = fixtureData(GITHUB, "9.8.8");
  });
  await assert.rejects(readSnapshotAt(root, commit), {
    message: `version mismatch: ${MARKETPLACE}="9.8.7" conflicts with ${GITHUB}="9.8.8"`,
  });
});

test("readSnapshotAt reports a malformed manifest at the commit", async () => {
  const { root, commit } = fixtureRepo((files) => {
    files[GITHUB] = "{";
  });
  await assert.rejects(readSnapshotAt(root, commit), {
    message: `${GITHUB}: is not valid JSON`,
  });
});

const nest = (depth: number): string => "[".repeat(depth) + "]".repeat(depth);
const entries = (list: string): string => `{"plugins":[${list}]}`;
const agenticsEntry = (version: string): string =>
  entries(`{"name":"agentics","version":${version}}`);

// Writes `text` (or removes the file when it is null) over one manifest of a valid tree and reads that manifest.
function readOne(path: string, text: string | Buffer | null): ValueResult {
  const root = writeVersionTree(FIXTURE);
  if (text === null) rmSync(join(root, path));
  else writeFileSync(join(root, path), text);
  const result = readValues(root).find((value) => value.path === path);
  assert.ok(result, path);
  return result;
}

const UNSET = { set: false, json: "" };
const tokenOf = (text: string): string => {
  const marketplace = readOne(MARKETPLACE, agenticsEntry(text));
  const plugin = readOne(CODEX, `{"version":${text}}`);
  assert.equal(marketplace.value.set, true, text);
  assert.deepEqual(plugin.value, marketplace.value, text);
  return marketplace.value.json;
};

test("readValues carries a version value token that is equal exactly when the values are", () => {
  // Numbers compare by value, strings by code points with a lone surrogate read as U+FFFD, objects by key set.
  for (const same of [
    ["1", "1.0", "1e0", "10e-1"],
    ["100", "1e2"],
    ["0", "-0", "-0.0", "0e5"],
    ["9007199254740992", "9007199254740993"],
    ['"A"', '"\\u0041"'],
    ['"a/b"', '"a\\/b"'],
    ['"\\ud800"', '"\\ufffd"'],
    ['{"b":1,"a":[0,{"d":2,"c":-0}]}', '{"a":[0.0,{"c":0,"d":2}],"b":1.0}'],
    ['{"a":1,"a":2}', '{"a":2}'],
    ['{"\\ud800":["\\udc00"]}', '{"\\ufffd":["\\ufffd"]}'],
    ["1e400", "2e400"],
  ])
    assert.equal(new Set(same.map(tokenOf)).size, 1, same.join(" = "));
  for (const [left, right] of [
    ["1", '"1"'],
    ["1", "2"],
    ["[1,2]", "[2,1]"],
    ["[]", "{}"],
    ['["a,b"]', '["a","b"]'],
    ['""', "0"],
    ["false", "0"],
    ["1e400", "-1e400"],
    ["1e400", '"Infinity"'],
    ["[1e400]", "[null]"],
    ['{"a":1e400}', '{"a":null}'],
    ['{"a":[{"b":-1e400}]}', '{"a":[{"b":null}]}'],
  ])
    assert.notEqual(tokenOf(left!), tokenOf(right!), `${left} != ${right}`);
  assert.equal(tokenOf('"1.0.0"'), '"1.0.0"');
  assert.equal(tokenOf('{"b":1,"a":2}'), '{"a":2,"b":1}');
  assert.equal(tokenOf("-0"), "0");
  assert.equal(tokenOf("1e400"), "Infinity");
  assert.equal(tokenOf("-1e400"), "-Infinity");
  assert.equal(
    tokenOf('{"b":[1e400,{"d":"\\ud800","c":-0}],"a":null}'),
    '{"a":null,"b":[Infinity,{"c":0,"d":"\ufffd"}]}',
  );
  // A missing field, an explicit null, an unreadable file and a malformed one have no token.
  const unset = (result: ValueResult): void =>
    assert.deepEqual(result.value, UNSET, result.path);
  unset(readOne(MARKETPLACE, agenticsEntry("null")));
  unset(readOne(MARKETPLACE, entries('{"name":"agentics"}')));
  unset(readOne(CODEX, '{"version":null}'));
  unset(readOne(CODEX, "{}"));
  unset(readOne(CODEX, "null"));
  unset(readOne(CODEX, "{"));
  unset(readOne(CODEX, "[]"));
  unset(readOne(CODEX, null));
  // A string that is not a canonical version still has a token, and so does a version kind of error.
  const noncanonical = readOne(CODEX, '{"version":"v1.0.0"}');
  assert.equal(noncanonical.kind, "version");
  assert.deepEqual(noncanonical.value, { set: true, json: '"v1.0.0"' });
  // A number that underflows to zero is a decodable number.
  assert.deepEqual(readOne(CODEX, '{"version":1e-400}').value, {
    set: true,
    json: "0",
  });
});

test("readValues says a file is missing only when it does not exist", () => {
  for (const path of PATHS) {
    const gone = readOne(path, null);
    assert.equal(gone.kind, "read", path);
    assert.equal(gone.missing, true, path);
  }
  const root = writeVersionTree(FIXTURE);
  rmSync(join(root, CODEX));
  mkdirSync(join(root, CODEX));
  writeFileSync(join(root, GITHUB), "{");
  assert.deepEqual(
    readValues(root).map((result) => [result.kind, result.missing]),
    [
      ["none", false],
      ["none", false],
      ["read", false],
      ["json", false],
      ["none", false],
    ],
  );
});

test("readValues matches keys in exact case, a repeated key taking its last value", () => {
  const read = (text: string): ValueResult => readOne(MARKETPLACE, text);
  const missing = `${MARKETPLACE}: has no "version" field`;
  const none = `${MARKETPLACE}: expected exactly one Agentics marketplace entry, found 0`;
  // A key in another case is not the key, for plugins, name or version.
  assert.equal(
    read('{"Plugins":[{"name":"agentics","version":"1.0.0"}]}').error,
    `${MARKETPLACE}: missing plugins array`,
  );
  assert.equal(
    read(entries('{"Name":"agentics","version":"1.0.0"}')).error,
    none,
  );
  const capital = read(entries('{"name":"agentics","Version":"1.0.0"}'));
  assert.deepEqual(
    [capital.kind, capital.raw, capital.value, capital.error],
    ["shape", "", UNSET, missing],
  );
  assert.equal(
    read('{"plugin\u017f":[{"name":"agentics","version":"1.0.0"}]}').error,
    `${MARKETPLACE}: missing plugins array`,
  );
  assert.equal(
    read(entries('{"name":"agentics","ver\u017fion":"1.0.0"}')).error,
    missing,
  );
  // A member in another case changes nothing, whichever comes last.
  const two = (first: string, second: string): string =>
    entries(`{"name":"agentics",${first},${second}}`);
  assert.equal(
    read(two('"version":"1.0.0"', '"Version":"2.0.0"')).raw,
    "1.0.0",
  );
  assert.equal(
    read(two('"Version":"2.0.0"', '"version":"1.0.0"')).raw,
    "1.0.0",
  );
  assert.equal(read(two('"version":"1.0.0"', '"VERSION":null')).raw, "1.0.0");
  assert.equal(
    read('{"plugins":[{"name":"agentics","version":"1.0.0"}],"PLUGINS":null}')
      .raw,
    "1.0.0",
  );
  assert.equal(
    read(entries('{"name":"agentics","NAME":5,"version":"1.0.0"}')).raw,
    "1.0.0",
  );
  // The same key repeated takes its last value, null and other types included.
  assert.equal(
    read(two('"version":"1.0.0"', '"version":"2.0.0"')).raw,
    "2.0.0",
  );
  assert.equal(
    read(two('"version":"1.0.0"', '"version":null')).error,
    `${MARKETPLACE}: "version" is not a string`,
  );
  assert.equal(
    read('{"plugins":[],"plugins":[{"name":"agentics","version":"1.0.0"}]}')
      .raw,
    "1.0.0",
  );
  assert.equal(
    read(entries('{"name":"agentics","name":null,"version":"1.0.0"}')).error,
    none,
  );
  assert.equal(
    read(entries('{"name":5,"name":"agentics","version":"1.0.0"}')).raw,
    "1.0.0",
  );
  assert.equal(
    read(entries('{"name":"agentics","name":5,"version":"1.0.0"}')).error,
    `${MARKETPLACE}: invalid plugins array: entry 0 "name" is not a string`,
  );
  // A null entry is skipped, and any other entry that is not an object fails the catalog.
  assert.equal(
    read(entries('null,{"name":"agentics","version":"1.0.0"}')).raw,
    "1.0.0",
  );
  assert.equal(
    read(entries("[]")).error,
    `${MARKETPLACE}: invalid plugins array: entry 0 is not an object`,
  );
  // A plugin manifest follows the same rules.
  assert.equal(
    readOne(CODEX, '{"Version":"1.0.0"}').error,
    `${CODEX}: has no "version" field`,
  );
  assert.equal(
    readOne(CODEX, '{"Version":"1.0.0","version":"2.0.0"}').raw,
    "2.0.0",
  );
  assert.equal(
    readOne(CODEX, '{"version":"2.0.0","VERSION":"1.0.0"}').raw,
    "2.0.0",
  );
  assert.equal(
    readOne(CODEX, '{"version":"1.0.0","version":"2.0.0"}').raw,
    "2.0.0",
  );
  assert.equal(readOne(CODEX, '{"\\u0076ersion":"3.0.0"}').raw, "3.0.0");
});

test("readValues reads only members the document itself holds", () => {
  for (const text of [
    '{"__proto__":{"version":"1.0.0"}}',
    '{"constructor":{"prototype":{"version":"1.0.0"}}}',
  ]) {
    const plugin = readOne(CODEX, text);
    assert.deepEqual(
      [plugin.kind, plugin.raw, plugin.value, plugin.error],
      ["shape", "", UNSET, `${CODEX}: has no "version" field`],
      text,
    );
  }
  assert.equal(
    readOne(MARKETPLACE, '{"__proto__":{"plugins":[]}}').error,
    `${MARKETPLACE}: missing plugins array`,
  );
  assert.equal(
    readOne(
      MARKETPLACE,
      entries('{"__proto__":{"name":"agentics","version":"1.0.0"}}'),
    ).error,
    `${MARKETPLACE}: expected exactly one Agentics marketplace entry, found 0`,
  );
  assert.equal(
    readOne(
      MARKETPLACE,
      entries('{"name":"agentics","__proto__":{"version":"1.0.0"}}'),
    ).error,
    `${MARKETPLACE}: has no "version" field`,
  );
});

test("readValues keeps the token of the last duplicate marketplace entry", () => {
  const read = (list: string): ValueResult =>
    readOne(MARKETPLACE, entries(list));
  const text = `${MARKETPLACE}: expected exactly one Agentics marketplace entry, found 2`;
  const numbered = read(
    '{"name":"agentics","version":"9.8.8"},{"name":"agentics","version":181}',
  );
  assert.deepEqual(
    [numbered.kind, numbered.raw, numbered.error, numbered.value],
    ["shape", "9.8.8", text, { set: true, json: "181" }],
  );
  const nulled = read(
    '{"name":"agentics","version":"9.8.8"},{"name":"agentics","version":null}',
  );
  assert.deepEqual([nulled.raw, nulled.value], ["9.8.8", UNSET]);
  const empty = read(
    '{"name":"agentics","version":""},{"name":"agentics","version":""}',
  );
  assert.deepEqual([empty.raw, empty.value], ["", { set: true, json: '""' }]);
  assert.deepEqual(read('{"name":"other","version":"1.0.0"}').value, UNSET);
});

test("readValues reads an overflowing number as a version that is not a string, and ignores one elsewhere", () => {
  for (const [literal, json] of [
    ["1e400", "Infinity"],
    ["-1e400", "-Infinity"],
  ] as const) {
    const value = { set: true, json };
    const marketplace = readOne(MARKETPLACE, agenticsEntry(literal));
    assert.deepEqual(
      [marketplace.kind, marketplace.raw, marketplace.value, marketplace.error],
      ["shape", "", value, `${MARKETPLACE}: "version" is not a string`],
    );
    const plugin = readOne(CODEX, `{"version":${literal}}`);
    assert.deepEqual(
      [plugin.kind, plugin.raw, plugin.value, plugin.error],
      ["shape", "", value, `${CODEX}: "version" is not a string`],
    );
  }
  // An overflowing number anywhere else does not matter.
  const elsewhere = readOne(CODEX, '{"version":"2.0.0","x":1e400}');
  assert.deepEqual([elsewhere.kind, elsewhere.raw], ["none", "2.0.0"]);
  const nested = readOne(
    MARKETPLACE,
    entries('{"name":"agentics","version":"1.0.0","x":1e400}'),
  );
  assert.deepEqual([nested.kind, nested.raw], ["none", "1.0.0"]);
  const document = readOne(
    MARKETPLACE,
    '{"plugins":[{"name":"agentics","version":"1.0.0"}],"x":1e400}',
  );
  assert.deepEqual([document.kind, document.raw], ["none", "1.0.0"]);
  const other = readOne(
    MARKETPLACE,
    entries(
      '{"name":"other","version":1e400},{"name":"agentics","version":"1.0.0"}',
    ),
  );
  assert.deepEqual([other.kind, other.raw], ["none", "1.0.0"]);
});

test("readValues accepts any nesting depth and never throws on a deeply nested version", () => {
  for (const depth of [10_001, 200_000]) {
    const plugin = readOne(CODEX, `{"version":"1.0.0","x":${nest(depth)}}`);
    assert.deepEqual([plugin.kind, plugin.raw], ["none", "1.0.0"], `${depth}`);
    const marketplace = readOne(
      MARKETPLACE,
      entries(`{"name":"agentics","version":"9.8.7","x":${nest(depth)}}`),
    );
    assert.deepEqual(
      [marketplace.kind, marketplace.raw],
      ["none", "9.8.7"],
      `${depth}`,
    );
  }
  // A version nested too deep for its token to be built is an error that names the file, not an exception.
  for (const [path, text] of [
    [CODEX, `{"version":${nest(200_000)}}`],
    [MARKETPLACE, agenticsEntry(nest(200_000))],
    [
      MARKETPLACE,
      entries(
        `{"name":"agentics","version":"1.0.0"},{"name":"agentics","version":${nest(200_000)}}`,
      ),
    ],
  ] as const) {
    const deep = readOne(path, text);
    assert.deepEqual(
      [deep.kind, deep.raw, deep.value, deep.error],
      ["json", "", UNSET, `${path}: could not be read`],
      path,
    );
  }
  // Unbalanced brackets are a syntax error at any depth.
  assert.equal(
    readOne(CODEX, `{"version":"1.0.0","x":${"[".repeat(10_001)}`).error,
    `${CODEX}: is not valid JSON`,
  );
});

test("readValues reads a lone surrogate as U+FFFD and refuses text outside the JSON grammar", () => {
  // A lone surrogate escape reads as U+FFFD, the same as an escaped U+FFFD.
  const lone = readOne(CODEX, '{"version":"\\ud800"}');
  assert.deepEqual([lone.kind, lone.raw], ["version", "\ufffd"]);
  assert.deepEqual(
    readOne(MARKETPLACE, agenticsEntry('"\\ud800"')).value,
    readOne(MARKETPLACE, agenticsEntry('"\\ufffd"')).value,
  );
  // A byte order mark is not skipped, and no message repeats the input or the engine's own wording.
  for (const text of [
    Buffer.from('\ufeff{"version":"1.0.0"}'),
    '{"version":"1.0.0"} x',
    "",
    "{'version':'1.0.0'}",
    '{"version":"1.0.0",}',
    '{"version":01}',
    '{"version":"1.0.0"}{"version":"2.0.0"}',
  ]) {
    const result = readOne(CODEX, text);
    assert.deepEqual(
      [result.kind, result.raw, result.value, result.error],
      ["json", "", UNSET, `${CODEX}: is not valid JSON`],
      String(text),
    );
  }
});

test("the five manifests of this repository read as the version in .claude-plugin/plugin.json", () => {
  const root = resolve(import.meta.dirname, "../../../..");
  const want = (
    JSON.parse(readFileSync(join(root, CLAUDE), "utf8")) as { version: string }
  ).version;
  assert.match(want, /^[0-9]+\.[0-9]+\.[0-9]+$/);
  assert.deepEqual(
    readValues(root),
    PATHS.map((path) => ({
      path,
      raw: want,
      kind: "none",
      error: undefined,
      value: { set: true, json: JSON.stringify(want) },
      missing: false,
    })),
  );
  assert.equal(readSnapshot(root).raw, want);
});

test("readValues replaces a lone surrogate next to an ordinary character", () => {
  for (const [escaped, raw] of [
    ["\ud800x", "�x"],
    ["x\udc00", "x�"],
  ] as const) {
    const result = readOne(CODEX, `{"version":"${escaped}"}`);
    assert.deepEqual([result.kind, result.raw], ["version", raw], escaped);
  }
});

test("readValues names each marketplace shape fault and keeps no raw value", () => {
  for (const [text, kind, message] of [
    ["null", "shape", "missing plugins array"],
    ["[]", "json", "is not a JSON object"],
    ["{}", "shape", "missing plugins array"],
    [
      '{"plugins":5}',
      "shape",
      'invalid plugins array: "plugins" is not an array',
    ],
  ] as const) {
    const result = readOne(MARKETPLACE, text);
    assert.deepEqual(
      [result.kind, result.raw, result.value, result.error, result.missing],
      [kind, "", UNSET, `${MARKETPLACE}: ${message}`, false],
      text,
    );
  }
});

test("readValues keeps the last non-empty duplicate version and an unset token for entries without one", () => {
  const noVersions = readOne(
    MARKETPLACE,
    '{"plugins":[{"name":"agentics"},{"name":"agentics"}]}',
  );
  assert.deepEqual(
    [noVersions.kind, noVersions.raw, noVersions.value, noVersions.error],
    [
      "shape",
      "",
      UNSET,
      `${MARKETPLACE}: expected exactly one Agentics marketplace entry, found 2`,
    ],
  );
  const emptyLast = readOne(
    MARKETPLACE,
    '{"plugins":[{"name":"agentics","version":"1.0.0"},{"name":"agentics","version":""}]}',
  );
  assert.deepEqual(
    [emptyLast.raw, emptyLast.value],
    ["1.0.0", { set: true, json: '""' }],
  );
});

test("readValues keeps no raw value for a missing file and says a bad version string is not missing", () => {
  const gone = readOne(CODEX, null);
  assert.deepEqual([gone.kind, gone.raw, gone.missing], ["read", "", true]);
  const short = readOne(CODEX, '{"version":"1.2"}');
  assert.deepEqual(
    [short.kind, short.raw, short.missing],
    ["version", "1.2", false],
  );
});
