// The PR metadata job in validate.yml previews the release level from a PR title and
// body. The release job computes the real level with commit-analyzer. This
// test holds the two equal, so the preview a reviewer reads is the release
// that ships.
//
// It runs the pinned analyzer, which the plain tools install does not provide:
// run `npm ci --ignore-scripts --prefix .github/release`.
import assert from "node:assert/strict";
import { spawnSync, type SpawnSyncReturns } from "node:child_process";
import { existsSync, readFileSync } from "node:fs";
import { createRequire } from "node:module";
import { join } from "node:path";
import { test } from "node:test";
import { pathToFileURL } from "node:url";
import {
  inlineNodeScript,
  lookupWorkflowStep,
} from "../scripts/testing/workflow-steps.ts";
import { prerequisite } from "./testing/support.ts";

const repoRoot = join(import.meta.dirname, "../../../..");
const releaseDir = join(repoRoot, ".github/release");

const skip = prerequisite(
  ".github/release/node_modules",
  existsSync(join(releaseDir, "node_modules")),
  "run: npm ci --ignore-scripts --prefix .github/release",
);

const WORKFLOW = ".github/workflows/validate.yml";
const workflow = readFileSync(join(repoRoot, WORKFLOW), "utf8");
const script = inlineNodeScript(
  lookupWorkflowStep(
    workflow,
    WORKFLOW,
    "pr-metadata",
    "Check the release markers",
  ),
);

interface AnalyzerOptions {
  preset: string;
  releaseRules?: unknown;
}

type Analyze = (
  options: AnalyzerOptions,
  context: {
    cwd: string;
    commits: { hash: string; message: string }[];
    logger: { log(): void };
  },
) => Promise<string | null>;

// The analyzer runs with the options the release job gives it.
const analyzerOptions: AnalyzerOptions = (() => {
  const config = JSON.parse(
    readFileSync(join(releaseDir, ".releaserc.json"), "utf8"),
  ) as { plugins: (string | [string, AnalyzerOptions])[] };
  const entry = config.plugins.find(
    (plugin) =>
      (Array.isArray(plugin) ? plugin[0] : plugin) ===
      "@semantic-release/commit-analyzer",
  );
  assert.ok(Array.isArray(entry), "commit-analyzer entry with options");
  assert.equal(typeof entry[1].preset, "string");
  return entry[1];
})();

// The analyzer is an ES module installed under .github/release, which is no
// ancestor of this file, so it is resolved from that directory.
let analyzer: Promise<Analyze> | undefined;
function loadAnalyzer(): Promise<Analyze> {
  analyzer ??= (async () => {
    const resolved = createRequire(join(releaseDir, "package.json")).resolve(
      "@semantic-release/commit-analyzer",
    );
    const loaded = (await import(pathToFileURL(resolved).href)) as {
      analyzeCommits: Analyze;
    };
    return loaded.analyzeCommits;
  })();
  return analyzer;
}

function previewResult(
  title: string,
  body: string,
  source: string = script,
): SpawnSyncReturns<string> {
  const result = spawnSync(process.execPath, ["-e", source], {
    env: {
      ...process.env,
      PR_TITLE: title,
      PR_BODY: body,
      GITHUB_OUTPUT: "",
    },
    encoding: "utf8",
    shell: false,
  });
  assert.ifError(result.error);
  return result;
}

function previewLevel(
  title: string,
  body: string,
  source: string = script,
): string {
  const result = previewResult(title, body, source);
  const match = /Release level: (\w+)/u.exec(result.stdout);
  assert.ok(match?.[1], result.stdout + result.stderr);
  return match[1];
}

async function analyzerLevel(
  subject: string,
  body: string,
  options: AnalyzerOptions = analyzerOptions,
): Promise<string> {
  const analyzeCommits = await loadAnalyzer();
  const message = body ? `${subject}\n\n${body}` : subject;
  const type = await analyzeCommits(options, {
    cwd: releaseDir,
    commits: [{ hash: "a".repeat(40), message }],
    logger: { log() {} },
  });
  return type ?? "none";
}

// The line GitHub's revert button writes. Under the analyzer's default rules
// it makes a revert release a patch; the release rule in the configuration
// is what keeps it at none.
const REVERT_BODY = `## What changed

x

This reverts commit ${"a".repeat(40)}.
`;

const MATRIX: readonly (readonly [string, string])[] = [
  ["feat(skills): add x", ""],
  ["feat: add x", "## What changed\n\nx\n"],
  ["fix(hooks): stop y", ""],
  ["perf: faster z", ""],
  ["docs: clarify", ""],
  ["refactor(hooks): tidy", ""],
  ["chore(deps): bump", ""],
  ["test: add a case", ""],
  ["ci: tweak a job", ""],
  ["build: pin a tool", ""],
  ["style: reflow", ""],
  ["revert: undo", ""],
  ["revert: undo", REVERT_BODY],
  ["fix!: drop an old flag", ""],
  ["feat(skills)!: retire wrap", "BREAKING CHANGE: wrap is gone"],
  ["feat(skills)!: retire wrap", "BREAKING-CHANGE: wrap is gone"],
];

// The description every pull request must carry; a matrix row with an empty
// body gets this minimum, for the preview and the analyzer alike.
const MINIMUM_BODY = "## What changed\n\nx\n";

async function mismatches(source: string): Promise<string[]> {
  const found: string[] = [];
  for (const [title, body] of MATRIX) {
    const effective = body || MINIMUM_BODY;
    const preview = previewLevel(title, effective, source);
    const level = await analyzerLevel(title, effective);
    if (preview !== level) {
      found.push(`${title}: preview ${preview}, analyzer ${level}`);
    }
  }
  return found;
}

test("the preview level equals the analyzer's type", { skip }, async () => {
  assert.deepEqual(await mismatches(script), []);
});

test(
  "the parity check dies when the preview level calculation is wrong",
  { skip },
  async () => {
    const mutated = script.replace('level = "minor"', 'level = "patch"');
    assert.notEqual(mutated, script);
    const found = await mismatches(mutated);
    assert.ok(
      found.some((row) => row.startsWith("feat: add x: preview patch")),
      found.join("\n"),
    );
  },
);

// Body lines the parser reads as a breaking-change note. With a ! in the
// title the preview must say major, as the analyzer does. Without one the
// analyzer still says major, so the job must fail: that is what keeps a
// major release visible in the title.
const FOOTER_BODIES = [
  "BREAKING CHANGE: wrap is gone",
  "BREAKING-CHANGE: wrap is gone",
  "Breaking change: none",
  "breaking change: x",
  "Breaking Change: x",
  "* BREAKING CHANGE: x",
  "  BREAKING CHANGE: x",
  "**BREAKING CHANGE:** x",
  "BREAKING CHANGE no colon",
];

// Lines the parser does not read as a note, so the analyzer says none.
const NOT_FOOTER_BODIES = [
  "BREAKING CHANGES: x",
  "see BREAKING CHANGE: x",
  "BREAKING CHANGE\nmore text",
  "BREAKING_CHANGE: x",
  "> BREAKING CHANGE: x",
];

const FOOTER_TITLE = "docs: clarify intent";

async function footerMismatches(source: string): Promise<string[]> {
  const found: string[] = [];
  const shapes: (readonly [string, string])[] = [
    ...FOOTER_BODIES.map((line): readonly [string, string] => [line, "\n"]),
    [FOOTER_BODIES[0] ?? "", "\r\n"],
  ];
  for (const [line, eol] of shapes) {
    const body = `## What changed\n\nx\n\n${line}\n`.replace(/\n/g, eol);
    const label = JSON.stringify(line) + (eol === "\n" ? "" : " (CRLF)");
    const level = await analyzerLevel(FOOTER_TITLE, body);
    if (level !== "major") {
      found.push(`${label}: analyzer ${level}, expected major`);
    }
    const withBang = previewResult("docs!: clarify intent", body, source);
    if (
      withBang.status !== 0 ||
      !/^Release level: major$/m.test(withBang.stdout)
    ) {
      found.push(`${label}: preview with ! did not report major`);
    }
    const without = previewResult(FOOTER_TITLE, body, source);
    if (without.status !== 1) {
      found.push(`${label}: preview without ! exited ${without.status}`);
    }
  }
  for (const line of NOT_FOOTER_BODIES) {
    const body = `## What changed\n\nx\n\n${line}\n`;
    const level = await analyzerLevel(FOOTER_TITLE, body);
    const result = previewResult(FOOTER_TITLE, body, source);
    const preview =
      result.status === 0
        ? /^Release level: (\w+)$/m.exec(result.stdout)?.[1]
        : `exit ${result.status}`;
    if (level !== "none" || preview !== "none") {
      found.push(
        `${JSON.stringify(line)}: preview ${preview}, analyzer ${level}`,
      );
    }
  }
  return found;
}

test(
  "every footer shape is major for the analyzer and fails the preview without a !",
  { skip },
  async () => {
    assert.deepEqual(await footerMismatches(script), []);
  },
);

test(
  "the footer parity check dies when the footer pattern is narrowed or widened",
  { skip },
  async () => {
    const mutants = [
      ["case-sensitive", "/im.test(body)", "/m.test(body)"],
      ["no leading run", "^[\\s|*]*BREAKING", "^BREAKING"],
      ["no hyphen form", "BREAKING[ -]CHANGE", "BREAKING CHANGE"],
      ["colon only", "(?::|[^\\S\\r\\n])+", ":"],
      ["no multiline flag", "/im.test(body)", "/i.test(body)"],
      ["no line anchor", "/^[\\s|*]*BREAKING", "/[\\s|*]*BREAKING"],
      ["crosses a line break", "(?::|[^\\S\\r\\n])+", "[:\\s]+"],
    ] as const;
    for (const [name, from, to] of mutants) {
      const mutated = script.replace(from, to);
      assert.notEqual(mutated, script, name);
      const found = await footerMismatches(mutated);
      assert.ok(found.length > 0, `${name} survived`);
    }
  },
);

test(
  "the revert rows die when the analyzer options lose the release rule",
  { skip },
  async () => {
    const { releaseRules, ...withoutRule } = analyzerOptions;
    assert.ok(releaseRules, "the shipped analyzer options carry the rule");
    assert.equal(
      await analyzerLevel("revert: undo", REVERT_BODY, withoutRule),
      "patch",
    );
    assert.equal(await analyzerLevel("revert: undo", REVERT_BODY), "none");
  },
);
