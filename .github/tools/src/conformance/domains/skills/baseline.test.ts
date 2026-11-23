import assert from "node:assert/strict";
import { existsSync, mkdtempSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { afterEach, test } from "node:test";
import { writeFile } from "../../testing/files.ts";
import {
  applyBaseline,
  BASELINE_DIR,
  BASELINE_STALE_RULE,
  type BaselineEntry,
  type KeyedFailure,
  readBaseline,
  writeBaselineFiles,
} from "./baseline.ts";
import { isFileErrorOfKind } from "./files.ts";

const RULES: ReadonlySet<string> = new Set(["skill-xref", "other-rule"]);
const GATE = `${BASELINE_DIR}/gate.json`;
const OPS = `${BASELINE_DIR}/ops.json`;

const roots: string[] = [];

function tempRoot(): string {
  const root = mkdtempSync(path.join(tmpdir(), "agentics-baseline-"));
  roots.push(root);
  return root;
}

afterEach(() => {
  for (const root of roots.splice(0))
    rmSync(root, { recursive: true, force: true });
});

function finding(rel: string, key: string, rule = "skill-xref"): KeyedFailure {
  return { rule, path: rel, message: `${key} is broken`, key };
}

function entry(rel: string, key: string, rule = "skill-xref"): BaselineEntry {
  return { rule, path: rel, key };
}

function rootWith(rel: string, content: string): string {
  const root = tempRoot();
  writeFile(root, rel, content);
  return root;
}

test("constants name the baseline directory and the stale rule", () => {
  assert.equal(BASELINE_DIR, ".github/tools/src/conformance/skill-baseline");
  assert.equal(BASELINE_STALE_RULE, "skill-baseline-stale");
});

test("readBaseline returns an empty map when the directory is missing", () => {
  assert.deepEqual(readBaseline(tempRoot(), RULES), new Map());
});

test("readBaseline throws a not-a-directory file error when the baseline directory is a file", () => {
  const root = rootWith(BASELINE_DIR, "[]");
  assert.throws(
    () => readBaseline(root, RULES),
    (error: unknown) => {
      assert.ok(error instanceof Error);
      assert.equal(
        error.message,
        `skills: reading ${BASELINE_DIR}: ${BASELINE_DIR}: is not a directory`,
      );
      assert.ok(isFileErrorOfKind(error.cause, "not-a-directory"));
      return true;
    },
  );
});

test("readBaseline keys entries by baseline file path in default sort order", () => {
  const root = tempRoot();
  writeFile(root, OPS, JSON.stringify([entry("skills/ops/SKILL.md", "/b")]));
  writeFile(root, GATE, JSON.stringify([entry("skills/gate/SKILL.md", "/a")]));
  const got = readBaseline(root, RULES);
  assert.deepEqual(
    [...got],
    [
      [GATE, [entry("skills/gate/SKILL.md", "/a")]],
      [OPS, [entry("skills/ops/SKILL.md", "/b")]],
    ],
  );
});

test("applyBaseline drops a covered finding and keeps the rest without key", () => {
  const baseline = new Map([
    [GATE, [entry("skills/gate/SKILL.md", "/ops check")]],
  ]);
  const got = applyBaseline(
    [
      finding("skills/gate/SKILL.md", "/ops check"),
      finding("skills/gate/SKILL.md", "/ops nosuch"),
    ],
    baseline,
  );
  assert.deepEqual(got, [
    {
      rule: "skill-xref",
      path: "skills/gate/SKILL.md",
      message: "/ops nosuch is broken",
    },
  ]);
  assert.equal(Object.hasOwn(got[0] ?? {}, "key"), false);
});

test("applyBaseline matches on rule, path and key together", () => {
  const baseline = new Map([[GATE, [entry("skills/gate/SKILL.md", "/x")]]]);
  const got = applyBaseline(
    [
      finding("skills/gate/SKILL.md", "/x", "other-rule"),
      finding("skills/gate/other.md", "/x"),
      finding("skills/gate/SKILL.md", "/y"),
    ],
    baseline,
  );
  assert.deepEqual(
    got.map((f) => [f.rule, f.path, f.message]),
    [
      ["other-rule", "skills/gate/SKILL.md", "/x is broken"],
      ["skill-xref", "skills/gate/other.md", "/x is broken"],
      ["skill-xref", "skills/gate/SKILL.md", "/y is broken"],
      [
        BASELINE_STALE_RULE,
        GATE,
        'baseline entry matches no finding: {"rule":"skill-xref","path":"skills/gate/SKILL.md","key":"/x"}',
      ],
    ],
  );
});

test("one entry covers every finding with the same identity", () => {
  const baseline = new Map([
    [GATE, [entry("skills/gate/SKILL.md", "/ops check")]],
  ]);
  const got = applyBaseline(
    [
      finding("skills/gate/SKILL.md", "/ops check"),
      finding("skills/gate/SKILL.md", "/ops check"),
    ],
    baseline,
  );
  assert.deepEqual(got, []);
});

test("an entry that covers nothing is reported stale at the baseline file", () => {
  const baseline = new Map([
    [GATE, [entry("skills/gate/SKILL.md", "/ops gone")]],
  ]);
  assert.deepEqual(applyBaseline([], baseline), [
    {
      rule: "skill-baseline-stale",
      path: ".github/tools/src/conformance/skill-baseline/gate.json",
      message:
        'baseline entry matches no finding: {"rule":"skill-xref","path":"skills/gate/SKILL.md","key":"/ops gone"}',
    },
  ]);
});

test("a duplicate stale entry is reported once", () => {
  const e = entry("skills/gate/SKILL.md", "/ops gone");
  const got = applyBaseline([], new Map([[GATE, [e, { ...e }]]]));
  assert.equal(got.length, 1);
  assert.equal(got[0]?.rule, BASELINE_STALE_RULE);
});

test("applyBaseline orders kept findings by input, then stale by file and entry", () => {
  const baseline = new Map([
    [
      OPS,
      [entry("skills/ops/SKILL.md", "/z"), entry("skills/ops/SKILL.md", "/a")],
    ],
    [GATE, [entry("skills/gate/SKILL.md", "/m")]],
  ]);
  const got = applyBaseline(
    [finding("skills/ops/b.md", "/2"), finding("skills/gate/a.md", "/1")],
    baseline,
  );
  assert.deepEqual(
    got.map((f) => [f.rule, f.path, f.message]),
    [
      ["skill-xref", "skills/ops/b.md", "/2 is broken"],
      ["skill-xref", "skills/gate/a.md", "/1 is broken"],
      [
        BASELINE_STALE_RULE,
        GATE,
        'baseline entry matches no finding: {"rule":"skill-xref","path":"skills/gate/SKILL.md","key":"/m"}',
      ],
      [
        BASELINE_STALE_RULE,
        OPS,
        'baseline entry matches no finding: {"rule":"skill-xref","path":"skills/ops/SKILL.md","key":"/z"}',
      ],
      [
        BASELINE_STALE_RULE,
        OPS,
        'baseline entry matches no finding: {"rule":"skill-xref","path":"skills/ops/SKILL.md","key":"/a"}',
      ],
    ],
  );
});

test("readBaseline throws on a file that is not JSON", () => {
  const root = rootWith(GATE, "{");
  assert.throws(() => readBaseline(root, RULES), {
    message: `skills: ${GATE}: is not valid JSON`,
  });
});

test("readBaseline reports a directory named <skill>.json with the shared file error", () => {
  const root = rootWith(`${GATE}/inner.json`, "[]");
  assert.throws(() => readBaseline(root, RULES), {
    message: `skills: reading ${GATE}: ${GATE}: is not a regular file`,
  });
});

test("readBaseline throws on JSON that is not an array", () => {
  const root = rootWith(GATE, "{}");
  assert.throws(() => readBaseline(root, RULES), {
    message: `skills: ${GATE}: want a JSON array of entries`,
  });
});

test("readBaseline throws on an entry without a string key", () => {
  const root = rootWith(
    GATE,
    JSON.stringify([{ rule: "skill-xref", path: "skills/gate/SKILL.md" }]),
  );
  assert.throws(() => readBaseline(root, RULES), {
    message: `skills: ${GATE}: entry 0 needs string rule, path and key`,
  });
});

test("readBaseline throws on an entry that is not an object", () => {
  const root = rootWith(
    GATE,
    JSON.stringify([entry("skills/gate/a.md", "/a"), null]),
  );
  assert.throws(() => readBaseline(root, RULES), {
    message: `skills: ${GATE}: entry 1 needs string rule, path and key`,
  });
});

test("readBaseline throws on an entry with an unknown rule", () => {
  const root = rootWith(
    GATE,
    JSON.stringify([entry("skills/gate/SKILL.md", "/a", "nope")]),
  );
  assert.throws(() => readBaseline(root, RULES), {
    message: `skills: ${GATE}: entry 0 names unknown rule nope`,
  });
});

test("readBaseline refuses to baseline the stale rule", () => {
  const root = rootWith(
    GATE,
    JSON.stringify([entry("skills/gate/SKILL.md", "/a", BASELINE_STALE_RULE)]),
  );
  assert.throws(() => readBaseline(root, RULES), {
    message: `skills: ${GATE}: entry 0 names unknown rule skill-baseline-stale`,
  });
});

test("readBaseline throws on an entry path outside the file's skill", () => {
  const root = rootWith(
    GATE,
    JSON.stringify([entry("skills/ops/SKILL.md", "/a")]),
  );
  assert.throws(() => readBaseline(root, RULES), {
    message: `skills: ${GATE}: entry 0 path skills/ops/SKILL.md is not under skills/gate/`,
  });
});

test("readBaseline throws on a path that only shares the skill name prefix", () => {
  const root = rootWith(
    GATE,
    JSON.stringify([entry("skills/gatekeeper/a.md", "/a")]),
  );
  assert.throws(() => readBaseline(root, RULES), {
    message: `skills: ${GATE}: entry 0 path skills/gatekeeper/a.md is not under skills/gate/`,
  });
});

test("readBaseline throws on a file that is not <skill>.json", () => {
  const root = rootWith(`${BASELINE_DIR}/README.md`, "notes");
  assert.throws(() => readBaseline(root, RULES), {
    message: `skills: reading ${BASELINE_DIR}/README.md: want <skill>.json`,
  });
});

test("readBaseline throws on a directory in the baseline directory", () => {
  const root = rootWith(`${BASELINE_DIR}/nested/gate.json`, "[]");
  assert.throws(() => readBaseline(root, RULES), {
    message: `skills: reading ${BASELINE_DIR}/nested: want <skill>.json`,
  });
});

test("writeBaselineFiles writes one sorted, de-duplicated file per skill", () => {
  const root = tempRoot();
  writeFile(root, `${BASELINE_DIR}/stray.json`, "[]");
  writeBaselineFiles(root, [
    finding("skills/gate/SKILL.md", "/z"),
    finding("skills/gate/SKILL.md", "/z"),
    finding("skills/gate/SKILL.md", "/a"),
    finding("skills/ops/SKILL.md", "/q", "other-rule"),
  ]);
  const read = (rel: string) =>
    readFileSync(path.join(root, ...rel.split("/")), "utf8");
  const gateEntries = [
    entry("skills/gate/SKILL.md", "/a"),
    entry("skills/gate/SKILL.md", "/z"),
  ];
  const opsEntries = [entry("skills/ops/SKILL.md", "/q", "other-rule")];
  assert.equal(read(GATE), JSON.stringify(gateEntries, null, 4) + "\n");
  assert.equal(read(OPS), JSON.stringify(opsEntries, null, 4) + "\n");
  assert.equal(
    existsSync(path.join(root, ...`${BASELINE_DIR}/stray.json`.split("/"))),
    false,
  );
  assert.deepEqual(
    [...readBaseline(root, RULES)],
    [
      [GATE, gateEntries],
      [OPS, opsEntries],
    ],
  );
});

test("writeBaselineFiles sorts by path, then rule, then key", () => {
  const root = tempRoot();
  writeBaselineFiles(root, [
    finding("skills/gate/b.md", "/a", "skill-xref"),
    finding("skills/gate/a.md", "/b", "skill-xref"),
    finding("skills/gate/a.md", "/c", "other-rule"),
    finding("skills/gate/a.md", "/a", "skill-xref"),
  ]);
  assert.deepEqual(readBaseline(root, RULES).get(GATE), [
    entry("skills/gate/a.md", "/c", "other-rule"),
    entry("skills/gate/a.md", "/a", "skill-xref"),
    entry("skills/gate/a.md", "/b", "skill-xref"),
    entry("skills/gate/b.md", "/a", "skill-xref"),
  ]);
});

test("writeBaselineFiles with no findings leaves an empty directory", () => {
  const root = tempRoot();
  writeFile(root, GATE, "[]");
  writeBaselineFiles(root, []);
  assert.deepEqual(readBaseline(root, RULES), new Map());
  assert.equal(existsSync(path.join(root, ...BASELINE_DIR.split("/"))), true);
});
