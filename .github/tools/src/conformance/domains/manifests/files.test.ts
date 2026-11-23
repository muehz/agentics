import assert from "node:assert/strict";
import {
  mkdirSync,
  mkdtempSync,
  realpathSync,
  rmSync,
  symlinkSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { test } from "node:test";
import { UNSET, type ValueResult } from "../../../release/version.ts";
import { RepoFileError } from "../../repo-file-error.ts";
import { writeFile } from "../../testing/files.ts";
import {
  assertVersionsReadable,
  containedManifestPath,
  ifPresent,
  isJsonObject,
  loadJson,
  MARKETPLACES,
  member,
  PartialAuditError,
  pluginList,
  readDirIfPresent,
  referenceRel,
  SKIPPED_DIRS,
  walkTree,
  type JsonObject,
  type JsonValue,
} from "./files.ts";
import {
  DIRECTORY_LINK_SKIP,
  linkDirectory,
  removeTree,
} from "../../testing/host.ts";

function withDir<T>(body: (root: string) => T): T {
  const root = mkdtempSync(path.join(tmpdir(), "agentics-manifests-files-"));
  try {
    return body(root);
  } finally {
    removeTree(root);
  }
}

const nest = (depth: number): string => "[".repeat(depth) + "]".repeat(depth);

/** An `assert.throws` check that the thrown value is a `RepoFileError` with this kind, path and message. */
const fileError =
  (kind: RepoFileError["kind"], rel: string, message: string) =>
  (error: unknown): boolean => {
    assert.ok(error instanceof RepoFileError, String(error));
    assert.equal(error.kind, kind, error.message);
    assert.equal(error.path, rel, error.message);
    assert.equal(error.message, message);
    return true;
  };

test("MARKETPLACES and SKIPPED_DIRS name the two catalogs and the three skipped directories", () => {
  assert.deepEqual(
    [...MARKETPLACES],
    [".claude-plugin/marketplace.json", ".agents/plugins/marketplace.json"],
  );
  assert.deepEqual([...SKIPPED_DIRS].sort(), [
    ".claude",
    ".git",
    "node_modules",
  ]);
});

test("ifPresent gives nothing for a missing path and a file error naming the path for any other failure", () => {
  const failing = (code?: string) => (): never => {
    throw Object.assign(new Error("x"), code === undefined ? {} : { code });
  };
  assert.equal(ifPresent("/abs/a", "a", failing("ENOENT")), undefined);
  assert.throws(
    () => ifPresent("/abs/a", "a/b", failing("EACCES")),
    fileError("unreadable", "a/b", "a/b: EACCES"),
  );
  assert.throws(
    () => ifPresent("/abs/a", "a/b", failing("ENOTDIR")),
    fileError("not-a-directory", "a/b", "a/b: is not a directory"),
  );
  assert.throws(
    () => ifPresent("/abs/a", "a", failing()),
    fileError("unreadable", "a", "a: unreadable"),
  );
  const info = { isDirectory: () => true };
  const seen: string[] = [];
  assert.equal(
    ifPresent("/abs/a", "a", (file) => {
      seen.push(file);
      return info;
    }),
    info,
  );
  assert.deepEqual(seen, ["/abs/a"]);
});

test("readDirIfPresent lists a directory, gives nothing for a missing one and calls a file not a directory", () => {
  withDir((root) => {
    writeFile(root, "d/x.txt", "x");
    writeFile(root, "f", "x");
    assert.deepEqual(
      readDirIfPresent(root, "d")?.map((entry) => entry.name),
      ["x.txt"],
    );
    assert.equal(readDirIfPresent(root, "gone"), undefined);
    assert.throws(
      () => readDirIfPresent(root, "f"),
      fileError("not-a-directory", "f", "f: is not a directory"),
    );
  });
});

test("referenceRel normalizes a reference to a slash-separated path with no trailing slash", () => {
  const backslash = String.fromCharCode(92);
  assert.equal(referenceRel("./skills/tdd/", "linux"), "skills/tdd");
  assert.equal(referenceRel("skills/../hooks/h.json", "linux"), "hooks/h.json");
  assert.equal(referenceRel(".", "linux"), ".");
  assert.equal(referenceRel("./", "linux"), ".");
  assert.equal(referenceRel(`a${backslash}b`, "linux"), `a${backslash}b`);
  assert.equal(referenceRel(`.${backslash}a${backslash}b`, "win32"), "a/b");
});

test("loadJson distinguishes a missing file, an invalid one and an audit error", () => {
  withDir((root) => {
    assert.deepEqual(loadJson(root, "d/x.json", "object"), {
      found: false,
      ok: false,
      value: {},
    });
    assert.deepEqual(loadJson(root, "d/x.json", "any"), {
      found: false,
      ok: false,
      value: undefined,
    });
    mkdirSync(path.join(root, "d", "x.json"), { recursive: true });
    assert.throws(
      () => loadJson(root, "d/x.json", "object"),
      fileError("not-a-file", "d/x.json", "d/x.json: is not a regular file"),
    );
  });
});

test("loadJson reads an object, the last of a repeated key winning", () => {
  withDir((root) => {
    const load = (text: string) => {
      writeFileSync(path.join(root, "x.json"), text);
      return loadJson(root, "x.json", "object");
    };
    const valid = load('{"a":1,"a":2,"b":[true]}');
    assert.equal(valid.found, true);
    assert.equal(valid.ok, true);
    assert.deepEqual(valid.value, { a: 2, b: [true] });
    // null reads as an empty object.
    assert.deepEqual(load("null"), { found: true, ok: true, value: {} });
    // Anything else at the root and text that is not JSON are not ok, with an empty object to read.
    for (const text of [
      "[]",
      "[1]",
      '"x"',
      "5",
      "true",
      "",
      "{",
      "{} x",
      "\ufeff{}",
      "{'a':1}",
      '{"a":1,}',
    ])
      assert.deepEqual(
        load(text),
        { found: true, ok: false, value: {} },
        text.slice(0, 30),
      );
    // Nesting is not limited and a number that overflows reads as infinite.
    assert.equal(load(`{"a":${nest(10_001)}}`).ok, true);
    assert.deepEqual(load('{"a":1e400}').value, { a: Infinity });
    assert.deepEqual(load('{"a":1e-400}').value, { a: 0 });
  });
});

test("loadJson keeps a key in its own case, so a wrong-case key reads as missing", () => {
  withDir((root) => {
    writeFileSync(
      path.join(root, "x.json"),
      '{"Plugins":[],"VERSION":"2.0.0","plugin\\u017f":1,"__proto__":{"name":"x"}}',
    );
    const { ok, value } = loadJson(root, "x.json", "object");
    assert.equal(ok, true);
    assert.equal(member(value, "plugins"), undefined);
    assert.equal(member(value, "version"), undefined);
    assert.equal(member(value, "Plugins") !== undefined, true);
    // A key the document wrote as `__proto__` is its own member; an inherited name is not one.
    assert.deepEqual(member(value, "__proto__"), { name: "x" });
    assert.equal(member(value, "constructor"), undefined);
    assert.equal(member(value, "toString"), undefined);
  });
});

test("isJsonObject is true for an object and false for null, an array and every scalar", () => {
  assert.equal(isJsonObject({}), true);
  for (const value of [null, undefined, [], [{}], "x", 0, true])
    assert.equal(isJsonObject(value), false, JSON.stringify(value));
});

test("loadJson with the any shape accepts every root JSON.parse reads", () => {
  withDir((root) => {
    const load = (text: string) => {
      writeFileSync(path.join(root, "x.json"), text);
      return loadJson(root, "x.json", "any");
    };
    const results: Array<[string, boolean, JsonValue | undefined]> = [
      ["null", true, null],
      ["[]", true, []],
      ["5", true, 5],
      ['"x"', true, "x"],
      ["{}", true, {}],
      ["[1e400]", true, [Infinity]],
      ["", false, undefined],
      ["{", false, undefined],
      ["[1,]", false, undefined],
    ];
    for (const [text, ok, value] of results) {
      const loaded = load(text);
      assert.equal(loaded.found, true, text);
      assert.equal(loaded.ok, ok, text);
      assert.deepEqual(loaded.value, ok ? value : undefined, text);
    }
    assert.equal(load(nest(100_000)).ok, true);
  });
});

test("loadJson treats a dangling symlink as a file that does not exist", (t) => {
  withDir((root) => {
    const link = path.join(root, "plugin.json");
    try {
      symlinkSync(path.join(root, "nowhere"), link, "file");
    } catch {
      t.skip("this host cannot create a symlink");
      return;
    }
    assert.equal(loadJson(root, "plugin.json", "any").found, false);
  });
});

test("walkTree visits depth first in default sort order and skips the three directories at any depth", () => {
  withDir((root) => {
    for (const rel of [
      "b/y.txt",
      "a/x.txt",
      "a/inner/z.txt",
      "B.txt",
      "z.txt",
      "\u00e9.txt",
      ".git/config",
      "sub/.git/config",
      "sub/node_modules/pkg/index.js",
      "a/.claude/settings.json",
      ".claude/worktrees/w/plugin.json",
      ".github/workflows/v.yml",
      ".claudex/y.txt",
      "node_modules.txt",
    ])
      writeFile(root, rel, "x");
    const seen: string[] = [];
    walkTree(root, (rel) => seen.push(rel));
    assert.deepEqual(seen, [
      ".claudex",
      ".claudex/y.txt",
      ".github",
      ".github/workflows",
      ".github/workflows/v.yml",
      "B.txt",
      "a",
      "a/inner",
      "a/inner/z.txt",
      "a/x.txt",
      "b",
      "b/y.txt",
      "node_modules.txt",
      "sub",
      "z.txt",
      "\u00e9.txt",
    ]);
  });
});

test("walkTree reports a file named like a skipped directory, and tells files from directories", () => {
  withDir((root) => {
    writeFile(root, ".git", "gitdir: ../elsewhere\n");
    writeFile(root, "sub/.claude", "not a directory");
    writeFile(root, "sub/node_modules", "not a directory");
    const seen: Array<[string, boolean]> = [];
    walkTree(root, (rel, entry) => seen.push([rel, entry.isDirectory()]));
    assert.deepEqual(seen, [
      [".git", false],
      ["sub", true],
      ["sub/.claude", false],
      ["sub/node_modules", false],
    ]);
  });
});

test("walkTree enters a work folder's temp directory in full without a tracked lister", () => {
  withDir((root) => {
    writeFile(root, "w/2026-11/feature-x/temp/scratch.md", "x");
    const seen: string[] = [];
    walkTree(root, (rel) => seen.push(rel));
    assert.deepEqual(seen, [
      "w",
      "w/2026-11",
      "w/2026-11/feature-x",
      "w/2026-11/feature-x/temp",
      "w/2026-11/feature-x/temp/scratch.md",
    ]);
  });
});

test("walkTree with a tracked lister visits only the tracked part of a work folder's temp directory", () => {
  withDir((root) => {
    writeFile(root, "w/2026-11/feature-x/temp/kept.md", "x");
    writeFile(root, "w/2026-11/feature-x/temp/scratch.md", "x");
    writeFile(root, "w/2026-11/feature-y/temp/scratch.md", "x");
    // A regular file named temp is not a scratch directory, so it is reported whether or not git tracks it.
    writeFile(root, "w/2026-11/feature-z/temp", "x");
    let calls = 0;
    const seen: string[] = [];
    walkTree(
      root,
      (rel) => seen.push(rel),
      () => {
        calls += 1;
        return {
          files: new Set(["w/2026-11/feature-x/temp/kept.md"]),
          dirs: new Set([
            "w",
            "w/2026-11",
            "w/2026-11/feature-x",
            "w/2026-11/feature-x/temp",
          ]),
        };
      },
    );
    assert.deepEqual(seen, [
      "w",
      "w/2026-11",
      "w/2026-11/feature-x",
      "w/2026-11/feature-x/temp",
      "w/2026-11/feature-x/temp/kept.md",
      "w/2026-11/feature-y",
      "w/2026-11/feature-z",
      "w/2026-11/feature-z/temp",
    ]);
    assert.equal(calls, 2);
  });
});

test("walkTree never follows a symlink", (t) => {
  withDir((root) => {
    mkdirSync(path.join(root, "real"));
    mkdirSync(path.join(root, "sub"));
    writeFile(root, "real/inside.txt", "x");
    const links = [
      "link",
      ".git",
      "node_modules",
      "sub/.git",
      "sub/node_modules",
    ];
    for (const rel of links)
      if (
        linkDirectory(path.join(root, "real"), path.join(root, rel)) ===
        undefined
      ) {
        t.skip(DIRECTORY_LINK_SKIP);
        return;
      }
    const seen = new Map<string, boolean>();
    walkTree(root, (rel, entry) => seen.set(rel, entry.isDirectory()));
    // A symlink is reported, as a non-directory, even when its name is on the skip list; nothing below it is.
    for (const rel of links) {
      assert.ok(seen.has(rel), `${rel} is reported`);
      assert.equal(seen.get(rel), false, `${rel} is not a directory`);
    }
    assert.deepEqual(
      [...seen.keys()].filter((rel) =>
        links.some((l) => rel.startsWith(`${l}/`)),
      ),
      [],
    );
    assert.ok(seen.has("real/inside.txt"));
  });
});

test("walkTree orders names by UTF-16 code units, as the default sort does", () => {
  withDir((root) => {
    // U+FF21 is the one code unit 0xFF21; U+1F600 is the pair 0xD83D 0xDE00, so by code units the emoji sorts first.
    const wide = `${String.fromCodePoint(0xff21)}.txt`;
    const emoji = `${String.fromCodePoint(0x1f600)}.txt`;
    writeFile(root, emoji, "x");
    writeFile(root, wide, "x");
    const seen: string[] = [];
    walkTree(root, (rel) => seen.push(rel));
    assert.deepEqual(seen, [emoji, wide]);
  });
});

test("walkTree refuses a root that is not a directory, and a root that does not exist", (t) => {
  withDir((root) => {
    const file = path.join(root, "file");
    writeFileSync(file, "x");
    assert.throws(
      () => walkTree(file, () => {}),
      fileError("not-a-directory", ".", ".: is not a directory"),
    );
    assert.throws(
      () => walkTree(path.join(root, "gone"), () => {}),
      fileError("missing", ".", ".: does not exist"),
    );
    mkdirSync(path.join(root, "real"));
    const link = path.join(root, "link");
    if (linkDirectory(path.join(root, "real"), link) === undefined) {
      t.diagnostic("symlink root: skipped, this host cannot create a symlink");
      return;
    }
    // A symlink is not followed, so a symlink root is refused as a link.
    assert.throws(
      () => walkTree(link, () => {}),
      fileError("symlink", ".", ".: passes through a link"),
    );
  });
});

test("walkTree throws when a directory cannot be read", () => {
  withDir((root) => {
    mkdirSync(path.join(root, "a"));
    mkdirSync(path.join(root, "b"));
    // The walker already listed the root when it reaches "a", so removing "b" there makes "b" unreadable.
    assert.throws(
      () =>
        walkTree(root, (rel) => {
          if (rel === "a") rmSync(path.join(root, "b"), { recursive: true });
        }),
      fileError("missing", "b", "b: does not exist"),
    );
  });
});

test("containedManifestPath resolves a reference inside the checkout", () => {
  withDir((root) => {
    writeFile(root, "skills/tdd/SKILL.md", "x");
    const real = realpathSync(root);
    assert.deepEqual(containedManifestPath(root, "skills/tdd"), {
      kind: "ok",
      target: path.join(real, "skills", "tdd"),
    });
    assert.deepEqual(containedManifestPath(root, "./skills/tdd/SKILL.md"), {
      kind: "ok",
      target: path.join(real, "skills", "tdd", "SKILL.md"),
    });
    assert.deepEqual(containedManifestPath(root, "skills/tdd/"), {
      kind: "ok",
      target: path.join(real, "skills", "tdd"),
    });
    assert.deepEqual(containedManifestPath(root, "skills/../skills/tdd"), {
      kind: "ok",
      target: path.join(real, "skills", "tdd"),
    });
    assert.deepEqual(containedManifestPath(root, "."), {
      kind: "ok",
      target: real,
    });
    assert.deepEqual(containedManifestPath(root, "skills/none"), {
      kind: "missing",
    });
    assert.deepEqual(containedManifestPath(root, "none/deeper"), {
      kind: "missing",
    });
  });
});

test("containedManifestPath says a non-local reference escapes, quoted as JSON", () => {
  withDir((root) => {
    for (const reference of ["", "..", "../x", "a/../../x", "/abs", "/"])
      assert.deepEqual(
        containedManifestPath(root, reference),
        { kind: "escape", message: `path escapes checkout: "${reference}"` },
        reference,
      );
    assert.deepEqual(containedManifestPath(root, "../a\tb\u00e9\u200b"), {
      kind: "escape",
      message: 'path escapes checkout: "../a\\tb\u00e9\u200b"',
    });
    // The Windows rules apply when asked for, on any host, before the file system is touched.
    for (const reference of [
      "C:foo",
      "a/b:c",
      "a\\..\\..\\b",
      "nul",
      "x/aux",
      "\\a",
    ])
      assert.deepEqual(
        containedManifestPath(root, reference, "win32"),
        {
          kind: "escape",
          message: `path escapes checkout: ${JSON.stringify(reference)}`,
        },
        reference,
      );
  });
});

test("containedManifestPath follows a link that stays inside and refuses one that leaves", (t) => {
  withDir((base) => {
    const root = path.join(base, "checkout");
    const outside = path.join(base, "outside");
    mkdirSync(root);
    mkdirSync(outside);
    writeFile(root, "real/file.txt", "x");
    writeFile(outside, "secret.txt", "x");
    const inside = linkDirectory(
      path.join(root, "real"),
      path.join(root, "inner"),
    );
    const leaving = linkDirectory(outside, path.join(root, "leaves"));
    if (inside === undefined || leaving === undefined) {
      t.skip(DIRECTORY_LINK_SKIP);
      return;
    }
    assert.deepEqual(containedManifestPath(root, "inner/file.txt"), {
      kind: "ok",
      target: path.join(realpathSync(root), "real", "file.txt"),
    });
    assert.deepEqual(containedManifestPath(root, "leaves/secret.txt"), {
      kind: "escape",
      message: 'path escapes checkout: "leaves/secret.txt"',
    });
    assert.deepEqual(containedManifestPath(root, "leaves"), {
      kind: "escape",
      message: 'path escapes checkout: "leaves"',
    });
    // A link whose target is gone is a path that does not exist.
    rmSync(path.join(root, "real"), { recursive: true });
    assert.deepEqual(containedManifestPath(root, "inner/file.txt"), {
      kind: "missing",
    });
  });
});

test("containedManifestPath says a path below a root that does not exist is missing", () => {
  withDir((base) => {
    assert.deepEqual(containedManifestPath(path.join(base, "gone"), "skills"), {
      kind: "missing",
    });
  });
});

test("containedManifestPath resolves a root that is itself given through a symlink", (t) => {
  withDir((base) => {
    const checkout = path.join(base, "checkout");
    const outside = path.join(base, "outside");
    mkdirSync(checkout);
    mkdirSync(outside);
    writeFile(checkout, "skills/tdd/SKILL.md", "x");
    const alias = path.join(base, "alias");
    const leaving = path.join(checkout, "leaves");
    if (
      linkDirectory(checkout, alias) === undefined ||
      linkDirectory(outside, leaving) === undefined
    ) {
      t.skip(DIRECTORY_LINK_SKIP);
      return;
    }
    const real = realpathSync(checkout);
    assert.deepEqual(containedManifestPath(alias, "skills/tdd"), {
      kind: "ok",
      target: path.join(real, "skills", "tdd"),
    });
    assert.deepEqual(containedManifestPath(alias, "."), {
      kind: "ok",
      target: real,
    });
    assert.deepEqual(containedManifestPath(alias, "leaves"), {
      kind: "escape",
      message: 'path escapes checkout: "leaves"',
    });
    assert.deepEqual(containedManifestPath(alias, "skills/none"), {
      kind: "missing",
    });
  });
});

test("containedManifestPath throws a file error, naming the reference, for a link loop", (t) => {
  withDir((root) => {
    try {
      symlinkSync("b", path.join(root, "a"), "dir");
      symlinkSync("a", path.join(root, "b"), "dir");
    } catch {
      t.skip("this host cannot create a symlink");
      return;
    }
    assert.throws(
      () => containedManifestPath(root, "./a/x/"),
      fileError("unreadable", "a/x", "a/x: ELOOP"),
    );
  });
});

test("containedManifestPath throws a file error, naming the root, when the root itself is a link loop", (t) => {
  withDir((base) => {
    const root = path.join(base, "a");
    try {
      symlinkSync("b", root, "dir");
      symlinkSync("a", path.join(base, "b"), "dir");
    } catch {
      t.skip("this host cannot create a symlink");
      return;
    }
    assert.throws(
      () => containedManifestPath(root, "x"),
      fileError("unreadable", ".", ".: ELOOP"),
    );
  });
});

test("containedManifestPath applies the rules of the platform it is given, not the host's", () => {
  withDir((root) => {
    const backslash = String.fromCharCode(92);
    const dotted = ["a", "..", "..", "agentics-no-such-entry"];
    // On Unix a backslash is an ordinary character, so the reference is not an escape. It resolves to `missing` because
    // the entry does not exist; on a Windows host `path.join` still climbs out of the root, so the result there is
    // `missing` only for that reason.
    assert.deepEqual(
      containedManifestPath(root, dotted.join(backslash), "linux"),
      { kind: "missing" },
    );
    // A slash is the separator on Unix, so the same reference written with slashes climbs out.
    assert.deepEqual(containedManifestPath(root, dotted.join("/"), "linux"), {
      kind: "escape",
      message: 'path escapes checkout: "a/../../agentics-no-such-entry"',
    });
    // On Windows either separator climbs out.
    assert.deepEqual(containedManifestPath(root, dotted.join("/"), "win32"), {
      kind: "escape",
      message: 'path escapes checkout: "a/../../agentics-no-such-entry"',
    });
    assert.deepEqual(
      containedManifestPath(root, dotted.join(backslash), "win32"),
      {
        kind: "escape",
        message: `path escapes checkout: ${JSON.stringify(dotted.join(backslash))}`,
      },
    );
  });
});

test("pluginList returns the plugins that are objects, by the exact key", () => {
  const entry = (name: string): JsonObject => ({ name });
  const catalog = (text: string): JsonObject => JSON.parse(text) as JsonObject;
  assert.deepEqual(
    pluginList(
      catalog('{"plugins":[{"name":"a"},null,1,"x",[],{"name":"b"}]}'),
    ),
    [entry("a"), entry("b")],
  );
  assert.deepEqual(pluginList(catalog('{"Plugins":[{"name":"a"}]}')), []);
  assert.deepEqual(pluginList(catalog('{"plugins":{"name":"a"}}')), []);
  assert.deepEqual(pluginList(catalog('{"plugins":null}')), []);
  assert.deepEqual(pluginList(catalog("{}")), []);
  assert.deepEqual(
    pluginList(catalog('{"plugins":[{"name":"a"}],"plugins":[{"name":"b"}]}')),
    [entry("b")],
  );
});

test("assertVersionsReadable throws for a manifest that exists but cannot be read, and for nothing else", () => {
  const result = (over: Partial<ValueResult>): ValueResult => ({
    path: ".codex-plugin/plugin.json",
    raw: "",
    kind: "none",
    error: undefined,
    value: UNSET,
    missing: false,
    ...over,
  });
  const unreadable = result({
    kind: "read",
    error: ".codex-plugin/plugin.json: open /x: permission denied",
  });
  assert.throws(() => assertVersionsReadable([result({}), unreadable]), {
    message: ".codex-plugin/plugin.json: open /x: permission denied",
  });
  assertVersionsReadable([
    result({
      kind: "read",
      missing: true,
      error: "x: no such file or directory",
    }),
    // A read that succeeded carries no error.
    result({ kind: "read" }),
    result({ kind: "json", error: "x: malformed JSON" }),
    result({ kind: "shape", error: "x: missing version" }),
    result({ kind: "version", error: "x: version" }),
    result({}),
  ]);
  assertVersionsReadable([]);
});

test("PartialAuditError is an Error that carries the failures found before it", () => {
  const failures = [{ rule: "r", path: "p", message: "m" }];
  const error = new PartialAuditError("audit x: boom", failures);
  assert.ok(error instanceof Error);
  assert.equal(error.message, "audit x: boom");
  assert.equal(error.name, "PartialAuditError");
  assert.equal(error.failures, failures);
});
