import assert from "node:assert/strict";
import { mkdtempSync, rmSync, symlinkSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { test } from "node:test";
import { writeFile } from "../../testing/files.ts";
import {
  anchors,
  boldLeads,
  githubSlug,
  headings,
  proseLines,
  skillDocs,
} from "./xref-md.ts";

const SAMPLE = "---\na: b\n---\nx\n```\ny\n```\nz";

test("proseLines drops frontmatter and fenced code and keeps line numbers", () => {
  assert.deepEqual(proseLines(SAMPLE), [
    { number: 4, text: "x" },
    { number: 8, text: "z" },
  ]);
});

test("proseLines reads CRLF text the same as LF text", () => {
  assert.deepEqual(proseLines(SAMPLE.replaceAll("\n", "\r\n")), [
    { number: 4, text: "x" },
    { number: 8, text: "z" },
  ]);
});

test("proseLines drops a fence indented inside a list item", () => {
  const text = "- item\n\n    ```\n    # inside\n    ```\nafter";
  assert.deepEqual(proseLines(text), [
    { number: 1, text: "- item" },
    { number: 2, text: "" },
    { number: 6, text: "after" },
  ]);
});

test("proseLines drops a tilde fence and a long backtick fence", () => {
  const text = "a\n~~~\nt\n~~~\nb\n````\n```\nu\n````\nc";
  assert.deepEqual(proseLines(text), [
    { number: 1, text: "a" },
    { number: 5, text: "b" },
    { number: 10, text: "c" },
  ]);
});

test("proseLines keeps a fence open until a closer at least as long", () => {
  const text = "````\n```\nhidden\n````\nshown";
  assert.deepEqual(proseLines(text), [{ number: 5, text: "shown" }]);
});

test("proseLines drops a fence inside a blockquote up to its quoted closer", () => {
  const text =
    "> intro\n>\n> ```md\n> [x](#not-a-real-anchor)\n> ```\n>\n> after\nout";
  assert.deepEqual(proseLines(text), [
    { number: 1, text: "> intro" },
    { number: 2, text: ">" },
    { number: 6, text: ">" },
    { number: 7, text: "> after" },
    { number: 8, text: "out" },
  ]);
});

test("proseLines drops a fence inside a nested blockquote", () => {
  const text = "> > ~~~\n> > hidden\n> > ~~~\nshown";
  assert.deepEqual(proseLines(text), [{ number: 4, text: "shown" }]);
});

test("proseLines closes a quoted fence on the first line outside the quote", () => {
  const text = "> ```\n> hidden\nshown\n```\nlater";
  assert.deepEqual(proseLines(text), [{ number: 3, text: "shown" }]);
});

test("proseLines returns no lines for unclosed frontmatter", () => {
  assert.deepEqual(proseLines("---\na: b\nx\n# h"), []);
});

test("githubSlug follows the GitHub anchor rules", () => {
  assert.equal(githubSlug("Step 0 — Preflight"), "step-0--preflight");
  assert.equal(githubSlug("The `fix` entry"), "the-fix-entry");
  assert.equal(githubSlug("⚠️ Needs you"), "-needs-you");
  assert.equal(
    githubSlug("The four Diátaxis document types"),
    "the-four-diátaxis-document-types",
  );
});

test("anchors numbers repeated headings in document order", () => {
  assert.deepEqual([...anchors("## A\n## A\n## A")], ["a", "a-1", "a-2"]);
});

test("headings skips fenced code", () => {
  assert.deepEqual(headings("```\n# not\n```\n# yes"), ["yes"]);
});

test("headings skips frontmatter", () => {
  assert.deepEqual(headings("---\n# c: comment\n---\n# h"), ["h"]);
});

test("headings strips a closing hash run", () => {
  assert.deepEqual(headings("## Title ##\n####### no"), ["Title"]);
});

test("boldLeads reads a bold lead on a list item and on a plain line", () => {
  assert.deepEqual(
    boldLeads("- **Plan artifact identity.** text\n**Step 0. Preflight.**"),
    ["Plan artifact identity.", "Step 0. Preflight."],
  );
});

test("skillDocs keeps skill Markdown files and marks the scanned ones", (t) => {
  const root = mkdtempSync(path.join(tmpdir(), "agentics-xrefmd-"));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  writeFile(root, "skills/a/SKILL.md", "# A\n");
  writeFile(root, "skills/a/references/r.md", "# R\n");
  writeFile(root, "skills/a/assets/t.md", "# T\n");
  writeFile(root, "skills/a/scripts/x.sh", "echo\n");
  const docs = skillDocs(root);
  assert.deepEqual(
    [...docs.values()],
    [
      {
        rel: "skills/a/SKILL.md",
        skill: "a",
        text: "# A\n",
        scanned: true,
      },
      {
        rel: "skills/a/assets/t.md",
        skill: "a",
        text: "# T\n",
        scanned: false,
      },
      {
        rel: "skills/a/references/r.md",
        skill: "a",
        text: "# R\n",
        scanned: true,
      },
    ],
  );
  assert.deepEqual(
    [...docs.keys()],
    ["skills/a/SKILL.md", "skills/a/assets/t.md", "skills/a/references/r.md"],
  );
});

test("skillDocs does not read a linked skill file", (t) => {
  const root = mkdtempSync(path.join(tmpdir(), "agentics-xrefmd-"));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  writeFile(root, "skills/a/SKILL.md", "# A\n");
  writeFile(root, "outside/r.md", "# Outside\n");
  try {
    symlinkSync(
      path.join(root, "outside", "r.md"),
      path.join(root, "skills", "a", "linked.md"),
      "file",
    );
  } catch {
    t.skip("this host cannot create a symlink");
    return;
  }
  assert.deepEqual([...skillDocs(root).keys()], ["skills/a/SKILL.md"]);
});

test("skillDocs lets the walk error propagate", (t) => {
  const root = mkdtempSync(path.join(tmpdir(), "agentics-xrefmd-"));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  writeFile(root, "skills/README.md", "# R\n");
  assert.throws(() => skillDocs(root), {
    message: "skills: no files under skills/ to audit for skill standards",
  });
});
