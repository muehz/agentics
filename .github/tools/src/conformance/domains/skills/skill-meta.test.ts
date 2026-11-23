import assert from "node:assert/strict";
import { mkdtempSync, renameSync, rmSync, symlinkSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { test } from "node:test";
import { fileURLToPath } from "node:url";
import { repoRoot } from "../../check.ts";
import { writeFile } from "../../testing/files.ts";
import { assetFiles, seams, withSeams, type Seams } from "./files.ts";
import {
  METADATA_RULE,
  readDeclarations,
  type SkillDeclaration,
} from "./skill-meta.ts";

/** Builds a repo with one skill per entry, each `SKILL.md` holding exactly the given text. */
function withSkills<T>(
  skills: Record<string, string>,
  body: (root: string) => T,
): T {
  const root = mkdtempSync(path.join(tmpdir(), "agentics-skillmeta-"));
  try {
    for (const [name, text] of Object.entries(skills))
      writeFile(root, `skills/${name}/SKILL.md`, text);
    return body(root);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
}

/** A `SKILL.md` with name, description and the given metadata lines (each already indented). */
function skillMd(name: string, metadata: string[]): string {
  const meta = metadata.length > 0 ? ["metadata:", ...metadata] : [];
  return [
    "---",
    `name: ${name}`,
    "description: A test skill.",
    ...meta,
    "---",
    "",
    "# Body",
    "",
  ].join("\n");
}

const SURFACES = "  surfaces: [claude-code, vscode]";

/** The declarations and finding keys for a single skill `s` with the given metadata lines. */
function readOne(metadata: string[]): {
  declaration: SkillDeclaration;
  keys: string[];
  findings: ReturnType<typeof readDeclarations>["findings"];
} {
  return withSkills({ s: skillMd("s", metadata) }, (root) => {
    const { declarations, findings } = readDeclarations(root);
    const declaration = declarations.get("s");
    assert.ok(declaration);
    return { declaration, keys: findings.map((f) => f.key), findings };
  });
}

test("a valid declaration gives no finding and holds every value", () => {
  const { declaration, findings } = readOne([
    SURFACES,
    "  modes: [branch, release]",
    "  mode-aliases:",
    "    pr: branch",
    "    deploy: release",
    "  flags: [--no-docs, --dry-run]",
  ]);
  assert.deepEqual(findings, []);
  assert.equal(declaration.skill, "s");
  assert.equal(declaration.rel, "skills/s/SKILL.md");
  assert.ok(declaration.frontmatter);
  assert.deepEqual(declaration.surfaces, ["claude-code", "vscode"]);
  assert.deepEqual(declaration.modes, new Set(["branch", "release"]));
  assert.deepEqual(
    declaration.aliases,
    new Map([
      ["pr", "branch"],
      ["deploy", "release"],
    ]),
  );
  assert.deepEqual(declaration.flags, new Set(["--no-docs", "--dry-run"]));
});

test("an unknown surface is reported and left out", () => {
  const { declaration, findings } = readOne([
    "  surfaces: [claude-code, cursor]",
  ]);
  assert.equal(findings.length, 1);
  const finding = findings[0];
  assert.ok(finding);
  assert.equal(finding.rule, METADATA_RULE);
  assert.equal(finding.path, "skills/s/SKILL.md");
  assert.equal(finding.key, "metadata.surfaces: cursor");
  assert.match(finding.message, /cursor/);
  assert.deepEqual(declaration.surfaces, ["claude-code"]);
});

test("antigravity is accepted as a shipped surface while retired surfaces stay rejected", () => {
  const { declaration, findings } = readOne([
    "  surfaces: [antigravity, claude-code]",
  ]);
  assert.deepEqual(findings, []);
  assert.deepEqual(declaration.surfaces, ["antigravity", "claude-code"]);

  const retired = readOne(["  surfaces: [antigravity, cursor]"]);
  assert.deepEqual(retired.keys, ["metadata.surfaces: cursor"]);
  assert.deepEqual(retired.declaration.surfaces, ["antigravity"]);
});

test("missing or empty surfaces are reported", () => {
  assert.deepEqual(readOne(["  modes: [a]"]).keys, ["metadata.surfaces"]);
  assert.deepEqual(readOne([]).keys, ["metadata.surfaces"]);
  assert.deepEqual(readOne(["  surfaces: []"]).keys, ["metadata.surfaces"]);
  assert.deepEqual(readOne(["  surfaces: codex"]).keys, ["metadata.surfaces"]);
});

test("a repeated surface is reported once and kept once", () => {
  const { declaration, keys } = readOne(["  surfaces: [codex, codex]"]);
  assert.deepEqual(keys, ["metadata.surfaces: codex"]);
  assert.deepEqual(declaration.surfaces, ["codex"]);
});

test("modes that are not a list are reported", () => {
  const { declaration, keys } = readOne([SURFACES, "  modes: branch"]);
  assert.deepEqual(keys, ["metadata.modes"]);
  assert.deepEqual(declaration.modes, new Set());
});

test("a mode that is not a word is reported and left out", () => {
  const { declaration, keys } = readOne([SURFACES, "  modes: [Branch, ok]"]);
  assert.deepEqual(keys, ["metadata.modes: Branch"]);
  assert.deepEqual(declaration.modes, new Set(["ok"]));
});

test("a repeated mode is reported", () => {
  const { declaration, keys } = readOne([
    SURFACES,
    "  modes: [branch, branch]",
  ]);
  assert.deepEqual(keys, ["metadata.modes: branch"]);
  assert.deepEqual(declaration.modes, new Set(["branch"]));
});

test("an alias to an undeclared mode is reported and left out", () => {
  const { declaration, keys } = readOne([
    SURFACES,
    "  modes: [branch]",
    "  mode-aliases:",
    "    pr: nosuch",
    "    feature: branch",
  ]);
  assert.deepEqual(keys, ["metadata.mode-aliases: pr"]);
  assert.deepEqual(declaration.aliases, new Map([["feature", "branch"]]));
});

test("an alias that is itself a mode is reported", () => {
  const { declaration, keys } = readOne([
    SURFACES,
    "  modes: [branch]",
    "  mode-aliases:",
    "    branch: branch",
  ]);
  assert.deepEqual(keys, ["metadata.mode-aliases: branch"]);
  assert.equal(declaration.aliases.size, 0);
});

test("an alias that is not a word is reported", () => {
  const { keys } = readOne([
    SURFACES,
    "  modes: [branch]",
    "  mode-aliases:",
    "    PR: branch",
  ]);
  assert.deepEqual(keys, ["metadata.mode-aliases: PR"]);
});

test("mode-aliases that are not a map are reported", () => {
  const { keys } = readOne([
    SURFACES,
    "  modes: [branch]",
    "  mode-aliases: x",
  ]);
  assert.deepEqual(keys, ["metadata.mode-aliases"]);
});

test("a flag without the double dash is reported and left out", () => {
  const { declaration, keys } = readOne([SURFACES, "  flags: [no-docs, --ok]"]);
  assert.deepEqual(keys, ["metadata.flags: no-docs"]);
  assert.deepEqual(declaration.flags, new Set(["--ok"]));
});

test("a repeated flag is reported", () => {
  const { declaration, keys } = readOne([SURFACES, "  flags: [--x, --x]"]);
  assert.deepEqual(keys, ["metadata.flags: --x"]);
  assert.deepEqual(declaration.flags, new Set(["--x"]));
});

test("flags that are not a list are reported", () => {
  const { keys } = readOne([SURFACES, "  flags: x"]);
  assert.deepEqual(keys, ["metadata.flags"]);
});

test("modes absent leaves modes undefined", () => {
  const { declaration, keys } = readOne([SURFACES]);
  assert.deepEqual(keys, []);
  assert.equal(declaration.modes, undefined);
  assert.equal(declaration.aliases.size, 0);
  assert.equal(declaration.flags.size, 0);
});

test("missing, unclosed or broken frontmatter gives an empty declaration and no finding", () => {
  withSkills(
    {
      none: "# No frontmatter\n",
      unclosed: "---\nname: unclosed\ndescription: x\n",
      broken: "---\nname: [broken\n---\n",
    },
    (root) => {
      const { declarations, findings } = readDeclarations(root);
      assert.deepEqual(findings, []);
      assert.deepEqual([...declarations.keys()].sort(), [
        "broken",
        "none",
        "unclosed",
      ]);
      for (const declaration of declarations.values()) {
        assert.equal(declaration.frontmatter, undefined);
        assert.deepEqual(declaration.surfaces, []);
        assert.equal(declaration.modes, undefined);
        assert.equal(declaration.aliases.size, 0);
        assert.equal(declaration.flags.size, 0);
      }
    },
  );
});

/**
 * An lstat seam that replaces `at` with a real link to `target` the first time `when` holds for an inspection of `at`,
 * just before that inspection runs. It stands in for a link that appears after an earlier check vetted the path, so
 * only the guarded read's own check can refuse it. Undefined when the host cannot create the link.
 */
function linkOnInspection(
  at: string,
  target: string,
  type: "file" | "dir",
  when: () => boolean,
): Seams["lstat"] | undefined {
  const staged = `${target}.staged-link`;
  try {
    symlinkSync(target, staged, type);
  } catch {
    return undefined;
  }
  const real = seams.lstat;
  let swapped = false;
  return (candidate) => {
    if (!swapped && candidate === at && when()) {
      swapped = true;
      rmSync(at, { recursive: true, force: true });
      renameSync(staged, at);
    }
    return real(candidate);
  };
}

test("a SKILL.md that becomes a link after the asset listing is refused by the read", (t) => {
  withSkills({ a: skillMd("a", []) }, (root) => {
    const outside = mkdtempSync(path.join(tmpdir(), "agentics-skillmeta-out-"));
    try {
      const target = path.join(outside, "SKILL.md");
      writeFile(outside, "SKILL.md", skillMd("outside", ["  modes: 7"]));
      const file = path.join(root, "skills", "a", "SKILL.md");
      // The asset listing inspects the file first; the link appears before the read inspects it again.
      let inspections = 0;
      const lstat = linkOnInspection(
        file,
        target,
        "file",
        () => (inspections += 1) === 2,
      );
      if (lstat === undefined) {
        t.skip("this host cannot create a symlink");
        return;
      }
      assert.throws(() => withSeams({ lstat }, () => readDeclarations(root)), {
        kind: "symlink",
        message: "skills/a/SKILL.md: passes through a link",
      });
    } finally {
      rmSync(outside, { recursive: true, force: true });
    }
  });
});

test("this checkout gives one declaration per skill and only skill-metadata findings", () => {
  const root = repoRoot(path.dirname(fileURLToPath(import.meta.url)));
  assert.notEqual(root, "");
  const { declarations, findings } = readDeclarations(root);
  assert.equal(declarations.size, assetFiles(root).length);
  for (const finding of findings) assert.equal(finding.rule, METADATA_RULE);
});

test("an escape character in a reported value is printed as an escape", () => {
  const { declaration, keys, findings } = readOne([
    SURFACES,
    '  modes: ["a\\eb", ok]',
    "  mode-aliases:",
    '    "p\\er": ok',
  ]);
  assert.deepEqual(keys, [
    "metadata.modes: a\\u001bb",
    "metadata.mode-aliases: p\\u001br",
  ]);
  assert.deepEqual(
    findings.map((f) => f.message.split(",")[0]),
    [
      "metadata.modes lists a\\u001bb",
      "metadata.mode-aliases has the alias p\\u001br",
    ],
  );
  assert.deepEqual(declaration.modes, new Set(["ok"]));
  assert.deepEqual(declaration.aliases, new Map());
});
