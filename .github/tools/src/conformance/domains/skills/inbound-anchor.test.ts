import assert from "node:assert/strict";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { test } from "node:test";
import type { Failure, RunOptions } from "../../check.ts";
import { evalsCheck } from "../../testing/skills-repo.ts";
import { writeFile } from "../../testing/files.ts";
import type { Tracked } from "../repo/walk.ts";
import { FAMILY, INBOUND_ANCHOR_RULE } from "./inbound-anchor.ts";
import { FAMILY as FILE_SIZE_FAMILY } from "./file-size.ts";
import { DOMAIN_FAMILIES, FAMILIES, run } from "./index.ts";

const SOURCE = "docs/a.md";
const TARGET = "skills/x/references/r.md";
const SCRIPT = "skills/x/scripts/run.sh";

/** The files every case starts from: a Markdown target with one heading and a non-Markdown file beside it. */
const BASE: Record<string, string> = {
  [TARGET]: "# R\n\n## Good\n",
  [SCRIPT]: "echo hi\n",
};

const options: RunOptions = {
  short: false,
  fast: false,
  diagnostics: () => {},
  env: process.env,
};

/**
 * Runs the family through `run` on a fresh root. `files` are written and tracked; `untracked` paths are tracked but
 * never written, which is how a tracked file deleted from the working tree looks.
 */
async function audit(
  files: Record<string, string>,
  untracked: string[] = [],
): Promise<Failure[]> {
  const root = mkdtempSync(path.join(tmpdir(), "agentics-inboundanchor-"));
  try {
    const all = { ...BASE, ...files };
    for (const [rel, text] of Object.entries(all)) writeFile(root, rel, text);
    const listed = [...Object.keys(all), ...untracked];
    const dirs = new Set<string>();
    for (const rel of listed)
      for (
        let dir = path.posix.dirname(rel);
        dir !== ".";
        dir = path.posix.dirname(dir)
      )
        dirs.add(dir);
    const tracked: Tracked = { files: new Set(listed), dirs };
    const result = await run(root, options, {
      families: [FAMILY],
      evalsCheck,
      tracked: () => tracked,
    });
    assert.equal(result.error, undefined);
    return result.failures;
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
}

function missing(source: string, target: string): Failure {
  return {
    rule: INBOUND_ANCHOR_RULE,
    path: source,
    message: `${source} links to ${target}, which is not a tracked file or directory`,
  };
}

function noHeading(source: string, target: string, anchor: string): Failure {
  return {
    rule: INBOUND_ANCHOR_RULE,
    path: source,
    message: `${source} links to ${target}#${anchor}, which names no heading there`,
  };
}

test("the family has its own name and rule and runs right after skill-file-size", () => {
  assert.equal(INBOUND_ANCHOR_RULE, "skill-inbound-anchor");
  assert.equal(FAMILY.name, "skill-inbound-anchor");
  assert.deepEqual(FAMILY.rules, [INBOUND_ANCHOR_RULE]);
  const fileSize = DOMAIN_FAMILIES.indexOf(FILE_SIZE_FAMILY);
  assert.equal(DOMAIN_FAMILIES[fileSize + 1], FAMILY);
  assert.equal(FAMILIES.includes(FAMILY), false);
});

test("a link whose anchor names a heading in the target passes", async () => {
  assert.deepEqual(
    await audit({ [SOURCE]: "See [r](../skills/x/references/r.md#good).\n" }),
    [],
  );
});

test("a link to a missing file gives only the missing-target finding, even with an anchor", async () => {
  assert.deepEqual(
    await audit({
      [SOURCE]: "See [r](../skills/x/references/gone.md#good).\n",
    }),
    [missing(SOURCE, "../skills/x/references/gone.md")],
  );
});

test("a link whose anchor names no heading in the target is reported", async () => {
  assert.deepEqual(
    await audit({ [SOURCE]: "See [r](../skills/x/references/r.md#bad).\n" }),
    [noHeading(SOURCE, "../skills/x/references/r.md", "bad")],
  );
});

test("a bad link inside a fenced block is not read", async () => {
  assert.deepEqual(
    await audit({
      [SOURCE]: "```md\n[r](../skills/x/references/r.md#bad)\n```\n",
    }),
    [],
  );
});

test("a bad link inside an inline code span is reported", async () => {
  assert.deepEqual(
    await audit({ [SOURCE]: "Use `[r](../skills/x/references/r.md#bad)`.\n" }),
    [noHeading(SOURCE, "../skills/x/references/r.md", "bad")],
  );
});

test("a bad link from a work folder under docs/specs is not read", async () => {
  assert.deepEqual(
    await audit({
      "docs/specs/2026-11/f/plan.md":
        "See [r](../../../../skills/x/references/r.md#bad).\n",
    }),
    [],
  );
});

test("a link to a directory that holds a tracked file passes, with or without a trailing slash", async () => {
  assert.deepEqual(
    await audit({
      [SOURCE]:
        "See [refs](../skills/x/references/) and [refs](../skills/x/references).\n",
    }),
    [],
  );
});

test("an anchor on a directory names no heading there", async () => {
  assert.deepEqual(
    await audit({ [SOURCE]: "See [refs](../skills/x/references/#good).\n" }),
    [noHeading(SOURCE, "../skills/x/references/", "good")],
  );
});

test("an anchor on a tracked file that is not Markdown names no heading there", async () => {
  assert.deepEqual(
    await audit({ [SOURCE]: "See [run](../skills/x/scripts/run.sh#L10).\n" }),
    [noHeading(SOURCE, "../skills/x/scripts/run.sh", "L10")],
  );
});

test("an empty fragment is skipped", async () => {
  assert.deepEqual(
    await audit({ [SOURCE]: "See [r](../skills/x/references/r.md#).\n" }),
    [],
  );
});

test("bare anchors, scheme links, reference-style links and angle-bracket targets are skipped", async () => {
  assert.deepEqual(
    await audit({
      [SOURCE]: [
        "[a](#nope)",
        "[b](https://example.com/skills/x/gone.md#bad)",
        "[c](mailto:someone@example.com)",
        "[d][ref]",
        "",
        "[ref]: ../skills/x/gone.md#bad",
        "[e](<../skills/x/gone.md#bad>)",
        "",
      ].join("\n"),
    }),
    [],
  );
});

test("a link that resolves outside skills is not checked", async () => {
  assert.deepEqual(await audit({ [SOURCE]: "See [b](./gone.md#bad).\n" }), []);
});

test("a source under skills is not read", async () => {
  assert.deepEqual(
    await audit({
      "skills/y/SKILL.md": "See [r](../x/references/r.md#bad).\n",
    }),
    [],
  );
});

test("a tracked source missing on disk is skipped", async () => {
  assert.deepEqual(await audit({}, ["docs/gone.md"]), []);
});

test("targets and anchors are not percent-decoded", async () => {
  assert.deepEqual(
    await audit({
      [SOURCE]:
        "See [r](../skills/x/references/r%2Emd) and [g](../skills/x/references/r.md#go%6Fd).\n",
    }),
    [
      missing(SOURCE, "../skills/x/references/r%2Emd"),
      noHeading(SOURCE, "../skills/x/references/r.md", "go%6Fd"),
    ],
  );
});

test("findings are sorted by path, then message", async () => {
  assert.deepEqual(
    await audit({
      "docs/b.md":
        "[z](../skills/x/references/r.md#zz) [a](../skills/x/references/r.md#aa)\n",
      [SOURCE]: "[m](../skills/x/missing.md)\n",
    }),
    [
      missing(SOURCE, "../skills/x/missing.md"),
      noHeading("docs/b.md", "../skills/x/references/r.md", "aa"),
      noHeading("docs/b.md", "../skills/x/references/r.md", "zz"),
    ],
  );
});

test("a link from a file at the repository root resolves against the root", async () => {
  assert.deepEqual(
    await audit({ "README.md": "[r](skills/x/references/r.md#bad)\n" }),
    [noHeading("README.md", "skills/x/references/r.md", "bad")],
  );
});
