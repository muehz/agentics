import assert from "node:assert/strict";
import { mkdtempSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { test } from "node:test";
import type { Failure } from "./check.ts";
import { seams, withSeams } from "./domains/skills/files.ts";
import {
  LEAN_CADENCE_PINS,
  leanCadenceRules,
} from "./domains/skills/lean-cadence.ts";
import {
  RECORD_KEEPING_PINS,
  recordKeepingRules,
} from "./domains/skills/record-keeping.ts";
import {
  SPEC_PASS_PINS,
  UNREADABLE_PLACEHOLDER,
  specPassRules,
} from "./domains/skills/spec-pass.ts";
import {
  WORK_FOLDER_PINS,
  workFolderRules,
} from "./domains/skills/work-folder.ts";
import { RepoFileError } from "./repo-file-error.ts";
import { writeFile } from "./testing/files.ts";
import { removeTree } from "./testing/host.ts";

const families = [
  {
    name: "lean cadence",
    run: leanCadenceRules,
    firstPath: LEAN_CADENCE_PINS[0]!.rel,
    readError:
      "skills: reading skills/decompose/references/plan-template.md: skills/decompose/references/plan-template.md: EACCES",
  },
  {
    name: "record keeping",
    run: recordKeepingRules,
    firstPath: RECORD_KEEPING_PINS[0]!.rel,
    readError:
      "cannot read skills/gate/references/branch/review-and-audit.md, which carries the rule that gate and `execute fix` account for the branch by calling the helper's audit instead of a hand procedure: skills/gate/references/branch/review-and-audit.md: EACCES",
  },
  {
    name: "spec pass",
    run: specPassRules,
    firstPath: SPEC_PASS_PINS[0]!.rel,
    readError:
      "cannot read skills/review/references/axes/spec-axis.md, which carries the spec-pass contract: skills/review/references/axes/spec-axis.md: EACCES",
  },
  {
    name: "work folder",
    run: workFolderRules,
    firstPath: WORK_FOLDER_PINS[0]!.rel,
    readError:
      "skills: reading skills/decompose/references/chain-artifacts.md: skills/decompose/references/chain-artifacts.md: EACCES",
  },
] as const;

function cleanSources(): Map<string, string> {
  const texts = new Map<string, string>();
  function append(rel: string, literal: string): void {
    texts.set(rel, `${texts.get(rel) ?? ""}${literal}\r\n`);
  }
  for (const pin of LEAN_CADENCE_PINS) {
    if (!pin.absent) append(pin.rel, pin.literal.replace(" ", "\t\r\n"));
    if (pin.absent && !pin.dir && !texts.has(pin.rel)) append(pin.rel, "");
  }
  for (const pin of RECORD_KEEPING_PINS) {
    if (pin.check.kind === "forbidden") continue;
    const times = pin.check.kind === "count" ? pin.check.times : 1;
    const literal =
      pin.check.kind === "line"
        ? pin.literal
        : pin.literal.toUpperCase().replace(" ", "\t\r\n");
    for (let i = 0; i < times; i++) append(pin.rel, literal);
  }
  for (const pin of SPEC_PASS_PINS)
    append(pin.rel, pin.literal.replace(" ", "\t\r\n"));
  for (const pin of WORK_FOLDER_PINS)
    append(pin.rel, pin.literal.replace(" ", "\t\r\n"));
  return texts;
}

function withSources(
  sources: ReadonlyMap<string, string>,
  check: (root: string) => void,
): void {
  const root = mkdtempSync(path.join(tmpdir(), "agentics-pinned-text-"));
  try {
    for (const [rel, text] of sources) writeFile(root, rel, text);
    check(root);
  } finally {
    removeTree(root);
  }
}

function expectFindings(
  sources: ReadonlyMap<string, string>,
  expected: Partial<Record<(typeof families)[number]["name"], Failure[]>>,
): void {
  withSources(sources, (root) => {
    for (const family of families)
      assert.deepEqual(
        family.run(root),
        expected[family.name] ?? [],
        family.name,
      );
  });
}

test("the four checks accept folded prose, CRLF exact lines and occurrence counts", () => {
  expectFindings(cleanSources(), {});
});

test("the families retain their ordered findings and exact diagnostics", () => {
  const sources = cleanSources();
  sources.set(LEAN_CADENCE_PINS[0]!.rel, "The required marker is gone.\n");
  sources.set("skills/execute/nested/z.md", "runs at every wave boundary\n");
  sources.set("skills/execute/a.md", "runs at every wave boundary\n");
  const execute = RECORD_KEEPING_PINS.find(
    (pin) => pin.check.kind === "count",
  )!;
  sources.set(execute.rel, "ARE THESE   INTENDED\n");
  const heading = RECORD_KEEPING_PINS.find((pin) => pin.check.kind === "line")!;
  sources.set(heading.rel, ` ${heading.literal}\r\nAGX FLOW AUDIT\r\n`);
  sources.delete(SPEC_PASS_PINS.at(-1)!.rel);
  sources.delete(
    WORK_FOLDER_PINS.find((pin) => pin.rel.startsWith("skills/gate/"))!.rel,
  );
  sources.set(SPEC_PASS_PINS[0]!.rel, "No review pins here.\n");
  // This family reports its missing source before these missing pins, unlike the single-pass families.
  sources.set(WORK_FOLDER_PINS[0]!.rel, "No contract pins here.\n");
  expectFindings(sources, {
    "lean cadence": [
      {
        rule: "lean-cadence-pins",
        path: "skills/decompose/references/plan-template.md",
        message: "missing: **Packet review:**",
      },
      {
        rule: "lean-cadence-pins",
        path: "skills/execute/SKILL.md",
        message: "missing: review group",
      },
      {
        rule: "lean-cadence-pins",
        path: "skills/execute/a.md",
        message: "retired text present: runs at every wave boundary",
      },
      {
        rule: "lean-cadence-pins",
        path: "skills/execute/nested/z.md",
        message: "retired text present: runs at every wave boundary",
      },
    ],
    "record keeping": [
      {
        rule: "gate-record-keeping",
        path: "skills/gate/references/branch/review-and-audit.md",
        message:
          "cannot read skills/gate/references/branch/review-and-audit.md, which carries the rule that gate and `execute fix` account for the branch by calling the helper's audit instead of a hand procedure: skills/gate: does not exist",
      },
      {
        rule: "gate-record-keeping",
        path: "skills/execute/references/fix-entry.md",
        message:
          'skills/execute/references/fix-entry.md no longer has the line "## Standalone fix commit audit"; the fix entry keeps the audit section that `execute`\'s SKILL.md links to',
      },
      {
        rule: "gate-record-keeping",
        path: "skills/execute/SKILL.md",
        message:
          "skills/execute/SKILL.md carries \"references/fix-entry.md#standalone-fix-commit-audit\" 0 times, not 2; `execute`'s boundary check and completion check each send a standalone fix to the fix entry's audit",
      },
      {
        rule: "gate-record-keeping",
        path: "skills/execute/SKILL.md",
        message:
          'skills/execute/SKILL.md carries "are these intended" again; gate and `execute fix` ask no record-keeping question; they record what can be recorded and report the rest',
      },
    ],
    "spec pass": [
      {
        rule: "spec-pass-pinned",
        path: "skills/review/references/axes/spec-axis.md",
        message:
          'skills/review/references/axes/spec-axis.md no longer carries the pinned sentence "## Per-requirement table"',
      },
      {
        rule: "spec-pass-pinned",
        path: "skills/review/references/axes/spec-axis.md",
        message:
          'skills/review/references/axes/spec-axis.md no longer carries the pinned sentence "`unrequested` lists a scope addition and is never counted as a gap."',
      },
      {
        rule: "spec-pass-pinned",
        path: "skills/gate/references/branch/review-and-audit.md",
        message:
          "cannot read skills/gate/references/branch/review-and-audit.md, which carries the spec-pass contract: skills/gate: does not exist",
      },
    ],
    "work folder": [
      {
        rule: "work-folder-source-present",
        path: "skills/gate/references/branch/tidy.md",
        message: "not found: the work-folder contract pins cannot be checked",
      },
      {
        rule: "work-folder-pin",
        path: "skills/decompose/references/chain-artifacts.md",
        message:
          "chain-artifacts.md must state that intent.md and plan.md are the required pair at the gate branch hand-off",
      },
      {
        rule: "work-folder-pin",
        path: "skills/decompose/references/chain-artifacts.md",
        message:
          "chain-artifacts.md must make intent.md required only when a present plan.md or spec.md derives from it",
      },
      {
        rule: "work-folder-pin",
        path: "skills/decompose/references/chain-artifacts.md",
        message:
          "chain-artifacts.md must define temp/ as the optional, Git-ignored scratch directory with unrestricted contents",
      },
      {
        rule: "work-folder-pin",
        path: "skills/decompose/references/chain-artifacts.md",
        message:
          "chain-artifacts.md must state the small-fix exception: a small fix has no work folder and needs neither required file at the hand-off",
      },
      {
        rule: "work-folder-pin",
        path: "skills/decompose/references/chain-artifacts.md",
        message:
          "chain-artifacts.md must state that the gate accepts a small-fix hand-off only when unassigned/state.tsv holds a skip chain row with no later pivot fix row",
      },
    ],
  });
});

test("the spec-pass check skips a file containing exactly its legacy placeholder", () => {
  const sources = cleanSources();
  sources.set(SPEC_PASS_PINS[0]!.rel, UNREADABLE_PLACEHOLDER);
  expectFindings(sources, {});
});

test("directory pins report one missing directory at its first use", () => {
  const sources = cleanSources();
  for (const rel of sources.keys())
    if (rel.startsWith("skills/execute/")) sources.delete(rel);
  withSources(sources, (root) => {
    assert.deepEqual(leanCadenceRules(root), [
      {
        rule: "lean-cadence-source-present",
        path: "skills/execute/references/stage-1-dispatch-rule.md",
        message: "not found: the lean-cadence pins cannot be checked",
      },
      {
        rule: "lean-cadence-source-present",
        path: "skills/execute/SKILL.md",
        message: "not found: the lean-cadence pins cannot be checked",
      },
      {
        rule: "lean-cadence-source-present",
        path: "skills/execute/references/flow-state.md",
        message: "not found: the lean-cadence pins cannot be checked",
      },
      {
        rule: "lean-cadence-source-present",
        path: "skills/execute",
        message: "not found: the lean-cadence pins cannot be checked",
      },
    ]);
  });
});

function expectAuditError(
  run: () => Failure[],
  message: string,
  cause: RepoFileError,
): void {
  assert.throws(run, (error: unknown) => {
    assert.ok(error instanceof Error);
    assert.equal(error.message, message);
    assert.strictEqual(error.cause, cause);
    return true;
  });
}

test("each check preserves its read audit error and typed cause", () => {
  withSources(cleanSources(), (root) => {
    const readFile = seams.readFile;
    for (const family of families) {
      const denied = path.join(root, ...family.firstPath.split("/"));
      const cause = new RepoFileError("unreadable", family.firstPath, {
        detail: "EACCES",
      });
      withSeams(
        {
          readFile(file) {
            if (file === denied) throw cause;
            return readFile(file);
          },
        },
        () => expectAuditError(() => family.run(root), family.readError, cause),
      );
    }
  });
});

test("a failed directory selection retains the scanning audit error", () => {
  withSources(cleanSources(), (root) => {
    const cause = new RepoFileError("unreadable", "skills/execute", {
      detail: "EACCES",
    });
    withSeams(
      {
        readDir() {
          throw cause;
        },
      },
      () =>
        expectAuditError(
          () => leanCadenceRules(root),
          "skills: scanning skills/execute: skills/execute: EACCES",
          cause,
        ),
    );
  });
});
