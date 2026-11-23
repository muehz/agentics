import assert from "node:assert/strict";
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { test } from "node:test";
import {
  formatVersion,
  MANIFESTS,
  parseVersion,
  readSnapshot,
  rewriteVersion,
  setVersion,
} from "./version.ts";
import { repoRoot, scratchDir } from "./testing/support.ts";

const PATHS = MANIFESTS.map((manifest) => manifest.path);
const MARKETPLACE = ".claude-plugin/marketplace.json";
const CODEX = ".codex-plugin/plugin.json";
const GITHUB = ".github/plugin/plugin.json";
const ANTIGRAVITY = ".antigravity-plugin/plugin.json";

function put(root: string, path: string, contents: string | Buffer): void {
  const full = join(root, path);
  mkdirSync(dirname(full), { recursive: true });
  writeFileSync(full, contents);
}

// The manifests in four-space form, as the repository keeps them. The
// marketplace's metadata.version is 0.0.0.
function writeManifestsForSet(root: string, version: string): void {
  const plugin = `{\n    "name": "agentics",\n    "version": "${version}"\n}\n`;
  const marketplace = `{\n    "metadata": {\n        "version": "0.0.0"\n    },\n    "plugins": [\n        {\n            "name": "agentics",\n            "version": "${version}"\n        }\n    ]\n}\n`;
  for (const path of PATHS) {
    put(root, path, path.endsWith("marketplace.json") ? marketplace : plugin);
  }
}

function readAll(root: string): Record<string, string> {
  return Object.fromEntries(
    PATHS.map((path) => [path, readFileSync(join(root, path), "latin1")]),
  );
}

function messageOf(body: () => unknown): string {
  try {
    body();
  } catch (error) {
    return (error as Error).message;
  }
  assert.fail("expected an error");
}

test("setVersion writes exactly the five version fields", () => {
  const root = scratchDir("agentics-set-");
  writeManifestsForSet(root, "1.2.3");
  const before = readAll(root);

  const { from, to } = setVersion(root, "1.3.0");

  assert.equal(formatVersion(from), "1.2.3");
  assert.equal(formatVersion(to), "1.3.0");
  const after = readAll(root);
  for (const path of PATHS) {
    assert.equal(
      after[path],
      before[path]?.replace('"version": "1.2.3"', '"version": "1.3.0"'),
      path,
    );
  }
  assert.equal(readSnapshot(root).raw, "1.3.0");
  assert.match(after[MARKETPLACE] ?? "", /"version": "0\.0\.0"/);
});

test("setVersion refuses with a message naming the reason and writes nothing", () => {
  const cases: {
    name: string;
    target: string;
    breakCodex?: boolean;
    want: string;
  }[] = [
    {
      name: "malformed",
      target: "1.3",
      want: 'refusing to set version: "1.3": invalid agentics version',
    },
    {
      name: "not greater",
      target: "1.2.3",
      want: "refusing to set version: 1.2.3 is not greater than 1.2.3",
    },
    {
      name: "lower",
      target: "1.2.2",
      want: "refusing to set version: 1.2.2 is not greater than 1.2.3",
    },
    {
      name: "disagreement",
      target: "1.3.0",
      breakCodex: true,
      want: `refusing to set version: version mismatch: ${MARKETPLACE}="1.2.3" conflicts with ${CODEX}="1.2.4"`,
    },
    {
      name: "overflow",
      target: "18446744073709551616.0.0",
      want: 'refusing to set version: "18446744073709551616.0.0": invalid agentics version',
    },
  ];
  for (const { name, target, breakCodex, want } of cases) {
    const root = scratchDir("agentics-set-");
    writeManifestsForSet(root, "1.2.3");
    if (breakCodex === true) {
      put(
        root,
        CODEX,
        '{\n    "name": "agentics",\n    "version": "1.2.4"\n}\n',
      );
    }
    const before = readAll(root);
    assert.equal(
      messageOf(() => setVersion(root, target)),
      want,
      name,
    );
    assert.deepEqual(
      readAll(root),
      before,
      `${name}: a refused set wrote a file`,
    );
  }
});

test("setVersion refuses before writing when one manifest cannot be rewritten", () => {
  const root = scratchDir("agentics-set-");
  writeManifestsForSet(root, "1.2.3");
  // The JSON value is 1.2.3 but the text holds an escape, so no
  // `"version": "1.2.3"` token exists to splice.
  put(root, GITHUB, '{"name": "agentics", "version": "1\\u002e2.3"}\n');
  const before = readAll(root);
  assert.equal(
    messageOf(() => setVersion(root, "1.3.0")),
    `refusing to set version: ${GITHUB}: no "version": "1.2.3" field reads as the Agentics version`,
  );
  assert.deepEqual(readAll(root), before);
});

test("setVersion reports a manifest it cannot read as a refusal", () => {
  const root = scratchDir("agentics-set-");
  writeManifestsForSet(root, "1.2.3");
  const missing = join(root, "nowhere");
  assert.equal(
    messageOf(() => setVersion(missing, "1.3.0")),
    `refusing to set version: ${MARKETPLACE}: does not exist`,
  );
});

test("setVersion compares numerically across the 64-bit range", () => {
  const root = scratchDir("agentics-set-");
  writeManifestsForSet(root, "18446744073709551614.0.0");
  const { to } = setVersion(root, "18446744073709551615.0.0");
  assert.equal(formatVersion(to), "18446744073709551615.0.0");
  assert.equal(readSnapshot(root).raw, "18446744073709551615.0.0");
  assert.match(
    messageOf(() => setVersion(root, "18446744073709551615.0.0")),
    /is not greater than 18446744073709551615\.0\.0$/,
  );
});

// What the splice must not touch: line endings, indentation, key order, white
// space around the colon, a missing final newline.
test("setVersion keeps CRLF endings, tabs, key order and spacing byte for byte", () => {
  const root = scratchDir("agentics-set-");
  const crlf = '{\r\n\t"version"  :\t"1.2.3",\r\n\t"name": "agentics"\r\n}';
  const marketplace =
    '{"metadata":{"version":"1.2.3"},"plugins":[{"version":"1.2.3","name":"agentics"}]}\r\n';
  put(root, MARKETPLACE, marketplace);
  put(root, ".claude-plugin/plugin.json", crlf);
  put(root, CODEX, crlf);
  put(root, GITHUB, crlf);
  put(root, ANTIGRAVITY, crlf);

  setVersion(root, "1.2.4");

  const after = readAll(root);
  assert.equal(
    after[".claude-plugin/plugin.json"],
    crlf.replace("1.2.3", "1.2.4"),
  );
  assert.equal(after[CODEX], crlf.replace("1.2.3", "1.2.4"));
  assert.equal(after[ANTIGRAVITY], crlf.replace("1.2.3", "1.2.4"));
  assert.equal(
    after[MARKETPLACE],
    '{"metadata":{"version":"1.2.3"},"plugins":[{"version":"1.2.4","name":"agentics"}]}\r\n',
  );
});

test("setVersion rewrites the Agentics version, not an earlier member that also reads 1.2.3", () => {
  const root = scratchDir("agentics-set-");
  const nested =
    '{"meta":{"version":"1.2.3"},"name":"agentics","version":"1.2.3"}\n';
  put(
    root,
    MARKETPLACE,
    '{"plugins":[{"name":"agentics","version":"1.2.3"}]}\n',
  );
  put(root, ".claude-plugin/plugin.json", nested);
  put(root, CODEX, nested);
  put(root, GITHUB, nested);
  put(root, ANTIGRAVITY, nested);

  setVersion(root, "1.2.4");

  assert.equal(
    readFileSync(join(root, CODEX), "utf8"),
    '{"meta":{"version":"1.2.3"},"name":"agentics","version":"1.2.4"}\n',
  );
  assert.equal(
    readFileSync(join(root, ANTIGRAVITY), "utf8"),
    '{"meta":{"version":"1.2.3"},"name":"agentics","version":"1.2.4"}\n',
  );
});

// A byte-order mark is not part of JSON and the reader does not strip one, so
// set-version leaves every file alone.
test("setVersion refuses a manifest with a byte-order mark and writes nothing", () => {
  const root = scratchDir("agentics-set-");
  writeManifestsForSet(root, "1.2.3");
  put(
    root,
    ".claude-plugin/plugin.json",
    Buffer.concat([
      Buffer.from([0xef, 0xbb, 0xbf]),
      Buffer.from('{"name": "agentics", "version": "1.2.3"}\n'),
    ]),
  );
  const before = readAll(root);
  assert.equal(
    messageOf(() => setVersion(root, "1.3.0")),
    "refusing to set version: .claude-plugin/plugin.json: is not valid JSON",
  );
  assert.deepEqual(readAll(root), before);
});

test("rewriteVersion replaces only the matched token and keeps every other byte", () => {
  const data = Buffer.from([
    ...Buffer.from('{"name": "caf'),
    0xe9,
    ...Buffer.from('", "version" : "1.2.3"}'),
  ]);
  const out = rewriteVersion("x.json", data, "1.2.3", "1.2.4");
  assert.deepEqual(
    out,
    Buffer.from([
      ...Buffer.from('{"name": "caf'),
      0xe9,
      ...Buffer.from('", "version" : "1.2.4"}'),
    ]),
  );

  // Multi-byte UTF-8 before the token makes string offsets and byte offsets
  // differ, so a splice at string offsets would cut the wrong bytes.
  const wide = Buffer.from('{"name": "café 中文", "version" : "1.2.3"}');
  assert.deepEqual(
    rewriteVersion("x.json", wide, "1.2.3", "1.2.4"),
    Buffer.from('{"name": "café 中文", "version" : "1.2.4"}'),
  );
});

test("rewriteVersion refuses when no field reads as the Agentics version", () => {
  for (const text of [
    "{}",
    '{"name": "agentics"}',
    '{"version": "1.2.9"}',
    '{"version": 1}',
  ]) {
    assert.equal(
      messageOf(() =>
        rewriteVersion("x.json", Buffer.from(text), "1.2.3", "1.2.4"),
      ),
      'x.json: no "version": "1.2.3" field reads as the Agentics version',
      text,
    );
  }
});

// The byte-level assertions run against the formatting Prettier keeps, so they
// use this checkout's manifests.
function copyRepoManifests(): string {
  const root = scratchDir("agentics-set-");
  for (const path of PATHS) {
    put(root, path, readFileSync(join(repoRoot, path)));
  }
  return root;
}

test("setVersion changes exactly one line in each real manifest", () => {
  const root = copyRepoManifests();
  const before = readAll(root);
  const current = readSnapshot(root);
  const want = `${current.version.major}.${current.version.minor + 1n}.0`;

  const { from, to } = setVersion(root, want);

  assert.equal(formatVersion(from), current.raw);
  assert.equal(formatVersion(to), want);
  const after = readAll(root);
  let changed = 0;
  for (const path of PATHS) {
    const oldLines = (before[path] ?? "").split("\n");
    const newLines = (after[path] ?? "").split("\n");
    assert.equal(newLines.length, oldLines.length, path);
    oldLines.forEach((oldLine, index) => {
      const newLine = newLines[index];
      if (oldLine === newLine) return;
      changed += 1;
      assert.ok(oldLine.includes('"version"'), `${path}:${index + 1}`);
      assert.equal(
        newLine,
        oldLine.replace(`"${current.raw}"`, `"${want}"`),
        `${path}:${index + 1}`,
      );
    });
  }
  assert.equal(changed, PATHS.length);
  assert.equal(readSnapshot(root).raw, want);
});

test("setVersion leaves the marketplace metadata version alone even when it matches", () => {
  const root = copyRepoManifests();
  const current = readSnapshot(root);
  const marketplace = join(root, MARKETPLACE);
  const text = readFileSync(marketplace, "utf8");
  assert.ok(
    text.includes('"version": "0.0.0"'),
    "the fixture assumption changed",
  );
  // metadata.version describes the catalog format. Giving it the plugin's own
  // value means a rewrite matched by text alone finds two candidates and has
  // to pick the plugin entry.
  writeFileSync(
    marketplace,
    text.replace('"version": "0.0.0"', `"version": "${current.raw}"`),
  );
  const next = `${current.version.major}.${current.version.minor}.${current.version.patch + 1n}`;

  setVersion(root, next);

  const document = JSON.parse(readFileSync(marketplace, "utf8")) as {
    metadata: { version: string };
  };
  assert.equal(document.metadata.version, current.raw);
  assert.equal(readSnapshot(root).raw, next);
  assert.equal(parseVersion(next).patch, current.version.patch + 1n);
});

test("rewriteVersion refuses a rewrite after which the manifest no longer reads one Agentics version", () => {
  // With two Agentics entries no single replacement leaves exactly one, so every candidate reads as a shape error, even
  // the one whose leftover raw value happens to be the target.
  const data = Buffer.from(
    JSON.stringify({
      plugins: [
        { name: "agentics", version: "1.0.0" },
        { name: "agentics", version: "1.0.0" },
      ],
    }),
  );
  assert.throws(() => rewriteVersion(MARKETPLACE, data, "1.0.0", "2.0.0"), {
    message: `${MARKETPLACE}: no "version": "1.0.0" field reads as the Agentics version`,
  });
});
