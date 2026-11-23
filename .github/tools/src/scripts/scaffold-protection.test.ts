import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test, { type TestContext } from "node:test";
import { fixtureGitEnv, git } from "../conformance/testing/git-checkout.ts";
import {
  findInterpreter,
  requireInterpreter,
  twinPowerShell,
} from "../entrypoint/interpreters.ts";

const repo = path.resolve(import.meta.dirname, "../../../..");
const scripts = path.join(repo, "skills/scaffold/scripts");
const template = path.join(
  repo,
  "skills/scaffold/references/planning-protection.gitignore",
);
const BEGIN = "# BEGIN AGENTICS PLANNING PROTECTION";
const END = "# END AGENTICS PLANNING PROTECTION";
const TEMPLATE = fs.readFileSync(template, "utf8");

/**
 * Runs a program with exact arguments. Windows hands a child one command line,
 * which an MSYS program splits at a raw CR or LF unless the argument is quoted,
 * so such arguments get CRT quoting there and nowhere else.
 */
function spawnExact(file: string, args: string[]) {
  const quote = (arg: string) =>
    `"${arg.replace(/(\\*)"/g, '$1$1\\"').replace(/(\\+)$/, "$1$1")}"`;
  if (process.platform === "win32" && args.some((arg) => /[\r\n]/.test(arg)))
    return spawnSync(file, args.map(quote), {
      encoding: "buffer",
      windowsVerbatimArguments: true,
      argv0: quote(file),
    });
  return spawnSync(file, args, { encoding: "buffer" });
}

/** The exact usage line for a program name; the `.sh` and `.ps1` twins differ only in this name. */
function usageLine(program: string): string {
  return `Usage: ${program} --parent <relative-parent> [--gitignore <existing-root-.gitignore>]\n`;
}
const LIMITATION =
  "Limitation: existing ignored ancestors can prevent discovery; this preview does not unignore ancestors. Ignores cannot untrack files or prevent force-adds; native validation remains separate.\n";
const INVALID_PARENT =
  "Invalid planning parent: use a nonempty repository-relative slash-separated path\n";
const TRAVERSAL =
  "Invalid planning parent: traversal components are forbidden\n";
const BAD_MARKERS =
  "Invalid planning protection markers: expected exactly one complete standalone block\n";
const BAD_ENCODING = "Invalid .gitignore encoding: expected UTF-8\n";

/** Forward slashes keep the path valid for an MSYS `sh` and for the PowerShell twin alike. */
function forward(file: string): string {
  return file.split(path.sep).join("/");
}

interface Leg {
  name: string;
  /** The program and its leading arguments; the row's arguments follow. */
  command: () => [string, string[]];
  skip: false | string;
  usage: string;
}

const shPath = findInterpreter("sh");
const powerShell = twinPowerShell();
const LEGS: Leg[] = [
  {
    name: ".sh",
    command: () => [
      shPath ?? "sh",
      [forward(path.join(scripts, "planning-protection.sh"))],
    ],
    skip: requireInterpreter("sh"),
    usage: usageLine("planning-protection.sh"),
  },
  {
    name: ".ps1",
    command: () => [
      powerShell.path ?? powerShell.name,
      [
        "-NoProfile",
        "-ExecutionPolicy",
        "Bypass",
        "-File",
        path.join(scripts, "planning-protection.ps1"),
      ],
    ],
    skip: powerShell.skip,
    usage: usageLine("planning-protection.ps1"),
  },
];

/** Stands for the path of the row's input `.gitignore` in its argument list. */
const GITIGNORE = Symbol("gitignore");

/** Paths, relative to the repository root, that Git must or must not ignore once the preview is its `.gitignore`. */
interface GitCheck {
  notIgnored: string[];
  ignored: string[];
  /** Directories created before checking that each, and a file inside it, is ignored. */
  dirs?: string[];
}

interface Row {
  name: string;
  args: (string | typeof GITIGNORE)[];
  /** Bytes of the input `.gitignore`; the row passes its path where `GITIGNORE` stands. */
  input?: string | Buffer;
  /** A directory stands where the input `.gitignore` would be. */
  inputIsDirectory?: true;
  stdout?: string;
  stderr: string | ((leg: Leg, gitignore: string) => string);
  status: 0 | 1;
  git?: GitCheck;
}

/** The protection block for an already escaped parent, with the given line ending. */
function block(escapedParent: string, eol: "\n" | "\r\n" = "\n"): string {
  return TEMPLATE.replaceAll("{{PARENT}}", () => escapedParent).replaceAll(
    "\n",
    eol,
  );
}

function protection(parent: string): GitCheck {
  const canonical = ["intent.md", "spec.md", "plan.md", "state.tsv"];
  const months = ["2026-01", "2026-09", "2026-10", "2026-12"];
  return {
    notIgnored: [
      "history.md",
      "unassigned/state.tsv",
      ...months.flatMap((month) =>
        canonical.map((file) => `${month}/work/${file}`),
      ),
    ].map((entry) => `${parent}/${entry}`),
    ignored: [
      "extra.txt",
      "nested/2026-10/work/plan.md",
      "2026-10/extra.txt",
      "2026-10/plan.md",
      "2026-10/work/extra.txt",
      "2026-10/work/nested/plan.md",
      "2026-10/work/nested/spec.md",
      "unassigned/intent.md",
      "unassigned/spec.md",
      "unassigned/plan.md",
      "unassigned/nested/state.tsv",
      "2026-00/work/plan.md",
      "2026-13/work/plan.md",
      "2026-19/work/plan.md",
      "2026-20/work/plan.md",
      "2026-1/work/plan.md",
      "2026-10-01/work/plan.md",
      "year-10/work/plan.md",
    ].map((entry) => `${parent}/${entry}`),
    dirs: [
      "history.md",
      ...canonical.map((name) => `2026-10/work/${name}`),
      "unassigned/state.tsv",
    ].map((entry) => `${parent}/${entry}`),
  };
}

function literalParentProtection(parent: string): GitCheck {
  return {
    notIgnored: [
      `${parent}/2026-12/work/plan.md`,
      "engineering/l plans/extra.txt",
    ],
    ignored: [`${parent}/extra.txt`],
  };
}

function usageRow(name: string, args: Row["args"]): Row {
  return {
    name: `usage: ${name}`,
    args,
    stderr: (leg) => leg.usage,
    status: 1,
  };
}

function parentRow(name: string, parent: string, stderr = INVALID_PARENT): Row {
  return {
    name: `parent rejected: ${name}`,
    args: ["--parent", parent],
    stderr,
    status: 1,
  };
}

/** A `--gitignore` the helper cannot read; both twins name the argument. */
function unreadableRow(name: string, directory: boolean): Row {
  return {
    name: `unreadable gitignore: ${name}`,
    args: ["--parent", "docs/specs", "--gitignore", GITIGNORE],
    ...(directory ? { inputIsDirectory: true as const } : {}),
    stderr: (_leg, gitignore) =>
      `Cannot read the .gitignore file: ${gitignore}\n`,
    status: 1,
  };
}

function mergeRow(
  name: string,
  parent: string,
  input: string | Buffer,
  stdout: string,
): Row {
  return {
    name: `merge: ${name}`,
    args: ["--parent", parent, "--gitignore", GITIGNORE],
    input,
    stdout,
    stderr: LIMITATION,
    status: 0,
  };
}

function markerRow(name: string, input: string): Row {
  return {
    name: `markers rejected: ${name}`,
    args: ["--parent", "docs/specs", "--gitignore", GITIGNORE],
    input,
    stderr: BAD_MARKERS,
    status: 1,
  };
}

function encodingRow(name: string, bytes: number[]): Row {
  return {
    name: `encoding rejected: ${name}`,
    args: ["--parent", "docs/specs", "--gitignore", GITIGNORE],
    input: Buffer.from(bytes),
    stderr: BAD_ENCODING,
    status: 1,
  };
}

const DOCS = block("docs/specs");
const DOCS_CRLF = block("docs/specs", "\r\n");
const OWNER = "\ufeff# Owner comment café\r\nnode_modules/\r\n";
const STALE = `${BEGIN}\nstale line\n/old/*\n${END}\n`;

const ROWS: Row[] = [
  {
    name: "default parent renders the canonical block",
    args: ["--parent", "docs/specs"],
    stdout: DOCS,
    stderr: LIMITATION,
    status: 0,
    git: protection("docs/specs"),
  },
  {
    name: "a conventional trailing slash is dropped",
    args: ["--parent", "docs/specs/"],
    stdout: DOCS,
    stderr: LIMITATION,
    status: 0,
  },
  {
    name: "nondefault parent",
    args: ["--parent", "engineering/plans"],
    stdout: block("engineering/plans"),
    stderr: LIMITATION,
    status: 0,
    git: {
      ...protection("engineering/plans"),
      notIgnored: [
        ...protection("engineering/plans").notIgnored,
        ...literalParentProtection("engineering/plans").notIgnored,
      ],
    },
  },
  {
    name: "glob characters in the parent are escaped",
    args: ["--parent", "engineering/[literal] plans"],
    stdout: block("engineering/\\[literal\\]\\ plans"),
    stderr: LIMITATION,
    status: 0,
    git: literalParentProtection("engineering/[literal] plans"),
  },
  {
    name: "replacement patterns in the parent stay literal",
    args: ["--parent", "engineering/$&-plans"],
    stdout: block("engineering/$&-plans"),
    stderr: LIMITATION,
    status: 0,
    git: literalParentProtection("engineering/$&-plans"),
  },
  {
    name: "every escaped character gets its own backslash",
    args: ["--parent", "a*b?c[d]e!f#g h"],
    stdout: block("a\\*b\\?c\\[d\\]e\\!f\\#g\\ h"),
    stderr: LIMITATION,
    status: 0,
  },
  {
    name: "non-ASCII parent bytes pass through",
    args: ["--parent", "docs/café/計画"],
    stdout: block("docs/café/計画"),
    stderr: LIMITATION,
    status: 0,
  },
  {
    name: "a single component parent",
    args: ["--parent", "plans"],
    stdout: block("plans"),
    stderr: LIMITATION,
    status: 0,
  },

  parentRow("traversal in the middle", "docs/../plans", TRAVERSAL),
  parentRow("leading traversal", "../plans", TRAVERSAL),
  parentRow("trailing traversal", "docs/..", TRAVERSAL),
  parentRow("parent-only traversal", "..", TRAVERSAL),
  parentRow("current directory component", "docs/./plans", TRAVERSAL),
  parentRow("current directory alone", ".", TRAVERSAL),
  parentRow("empty", ""),
  parentRow("a lone slash", "/"),
  parentRow("absolute", "/docs/specs"),
  parentRow("drive letter", "C:/docs/specs"),
  parentRow("colon", "docs:specs"),
  parentRow("backslashes", "\\server\\share"),
  parentRow("newline", "docs\n/plans"),
  parentRow("carriage return", "docs\r/plans"),
  parentRow("empty component", "docs//plans"),
  parentRow("empty component after the trailing slash", "docs/plans//"),

  mergeRow(
    "CRLF input keeps its bytes and gets a CRLF block",
    "docs/specs",
    OWNER,
    OWNER + DOCS_CRLF,
  ),
  mergeRow(
    "rerunning on a merged CRLF file is byte-identical",
    "docs/specs",
    `${OWNER}${DOCS_CRLF}# Tail\r\n`,
    `${OWNER}${DOCS_CRLF}# Tail\r\n`,
  ),
  mergeRow(
    "a changed parent replaces the one block in place",
    "engineering/plans",
    `${OWNER}${DOCS_CRLF}# Tail\r\n`,
    `${OWNER}${block("engineering/plans", "\r\n")}# Tail\r\n`,
  ),
  mergeRow(
    "an LF file gets an LF block",
    "docs/specs",
    "node_modules/\n",
    `node_modules/\n${DOCS}`,
  ),
  mergeRow("an empty file gets the bare block", "docs/specs", "", DOCS),
  mergeRow(
    "an unterminated LF line is terminated first",
    "docs/specs",
    "node_modules/",
    `node_modules/\n${DOCS}`,
  ),
  mergeRow(
    "an unterminated CRLF file is terminated with CRLF",
    "docs/specs",
    "a\r\nnode_modules/",
    `a\r\nnode_modules/\r\n${DOCS_CRLF}`,
  ),
  mergeRow(
    "a final CR without LF does not make the file CRLF",
    "docs/specs",
    "a\r",
    `a\r\n${DOCS}`,
  ),
  mergeRow(
    "a lone CR inside a line does not make the file CRLF",
    "docs/specs",
    "a\rb\n",
    `a\rb\n${DOCS}`,
  ),
  mergeRow(
    "a BOM-only file is terminated before the block",
    "docs/specs",
    "\ufeff",
    `\ufeff\n${DOCS}`,
  ),
  mergeRow(
    "a BOM and non-ASCII text survive an LF merge",
    "docs/specs",
    "\ufeff# 日本語 😀\n",
    `\ufeff# 日本語 😀\n${DOCS}`,
  ),
  mergeRow(
    "one CRLF anywhere makes the whole block CRLF",
    "docs/specs",
    "a\nb\r\nc\n",
    `a\nb\r\nc\n${DOCS_CRLF}`,
  ),
  mergeRow(
    "an LF block is replaced between other lines",
    "docs/specs",
    `head\n${STALE}tail\n`,
    `head\n${DOCS}tail\n`,
  ),
  mergeRow(
    "a block at the start is replaced",
    "docs/specs",
    `${STALE}tail\n`,
    `${DOCS}tail\n`,
  ),
  mergeRow(
    "a block at the end without a final newline is replaced",
    "docs/specs",
    `head\n${BEGIN}\nstale\n${END}`,
    `head\n${DOCS}`,
  ),
  mergeRow(
    "an LF block in a file with CRLF elsewhere becomes CRLF",
    "docs/specs",
    `a\r\n${STALE}tail\n`,
    `a\r\n${DOCS_CRLF}tail\n`,
  ),
  mergeRow(
    "an empty block is replaced",
    "docs/specs",
    `${BEGIN}\n${END}\n`,
    DOCS,
  ),
  mergeRow(
    "lower-case marker text is an ordinary comment",
    "docs/specs",
    "# begin agentics planning protection\n# end agentics planning protection\n",
    `# begin agentics planning protection\n# end agentics planning protection\n${DOCS}`,
  ),
  mergeRow(
    "lower-case marker lines around a real block are kept",
    "docs/specs",
    `# begin agentics planning protection\n${STALE}# end agentics planning protection\n`,
    `# begin agentics planning protection\n${DOCS}# end agentics planning protection\n`,
  ),
  {
    name: "merge: existing ignored ancestors stay ignored",
    args: ["--parent", "engineering/plans", "--gitignore", GITIGNORE],
    input: "/engineering/\n",
    stdout: `/engineering/\n${block("engineering/plans")}`,
    stderr: LIMITATION,
    status: 0,
    git: {
      notIgnored: [],
      ignored: ["engineering/plans/2026-10/work/plan.md"],
    },
  },

  markerRow("begin without end", `${BEGIN}\n`),
  markerRow("end without begin", `${END}\n`),
  markerRow("end before begin", `${END}\n${BEGIN}\n`),
  markerRow("two begins", `${BEGIN}\n${BEGIN}\n${END}\n`),
  markerRow("text before the begin marker", `prefix ${BEGIN}\n${END}\n`),
  markerRow("text after the end marker", `${BEGIN}\n${END} suffix\n`),
  markerRow("two complete blocks", `${BEGIN}\n${END}\n${BEGIN}\n${END}\n`),
  markerRow("a BOM before the first marker", `\ufeff${BEGIN}\n${END}\n`),
  markerRow(
    "an unterminated end line with a trailing CR",
    `${BEGIN}\n${END}\r`,
  ),
  markerRow("a marker inside a longer line", `x ${END} y\n${BEGIN}\n${END}\n`),

  encodingRow("a lone invalid byte", [0x23, 0x20, 0xff, 0x0a]),
  encodingRow("an overlong two-byte form", [0xc0, 0x80]),
  encodingRow("an overlong three-byte form", [0xe0, 0x80, 0x80]),
  encodingRow("an encoded surrogate", [0xed, 0xa0, 0x80]),
  encodingRow("a code point above U+10FFFF", [0xf4, 0x90, 0x80, 0x80]),
  encodingRow("a lead byte above F4", [0xf5, 0x80, 0x80, 0x80]),
  encodingRow("a stray continuation byte", [0x61, 0x80, 0x62]),
  encodingRow("a truncated sequence at the end", [0x61, 0xc3]),
  encodingRow("a truncated sequence before ASCII", [0xe2, 0x82, 0x41]),
  {
    name: "merge: the extreme valid sequences are accepted",
    args: ["--parent", "docs/specs", "--gitignore", GITIGNORE],
    input: Buffer.from([
      0x23, 0xc2, 0x80, 0xdf, 0xbf, 0xe0, 0xa0, 0x80, 0xed, 0x9f, 0xbf, 0xee,
      0x80, 0x80, 0xef, 0xbf, 0xbf, 0xf0, 0x90, 0x80, 0x80, 0xf4, 0x8f, 0xbf,
      0xbf, 0x0a,
    ]),
    stdout: `#\u0080\u07ff\u0800\ud7ff\ue000\uffff\u{10000}\u{10ffff}\n${DOCS}`,
    stderr: LIMITATION,
    status: 0,
  },
  {
    name: "merge: a NUL byte is ordinary UTF-8",
    args: ["--parent", "docs/specs", "--gitignore", GITIGNORE],
    input: Buffer.from([0x61, 0x00, 0x62, 0x0a]),
    stdout: `a\u0000b\n${DOCS}`,
    stderr: LIMITATION,
    status: 0,
  },

  usageRow("no arguments", []),
  usageRow("a flag without a value", ["--parent"]),
  usageRow("a positional argument", ["docs/specs"]),
  usageRow("only the gitignore flag", ["--gitignore", "x"]),
  usageRow("an unknown flag", ["--parent", "docs/specs", "--apply"]),
  usageRow("an unknown flag with a value", [
    "--parent",
    "docs/specs",
    "--apply",
    "yes",
  ]),
  usageRow("a repeated parent", [
    "--parent",
    "docs/specs",
    "--parent",
    "other",
  ]),
  usageRow("a repeated gitignore", [
    "--parent",
    "docs/specs",
    "--gitignore",
    "a",
    "--gitignore",
    "b",
  ]),
  usageRow("a gitignore flag without a value", [
    "--parent",
    "docs/specs",
    "--gitignore",
  ]),
  usageRow("a value that looks like a flag", ["--parent", "--gitignore", "x"]),
  usageRow("a bare double dash before the flags", ["--", "--parent", "x"]),
  usageRow("a bare double dash after the flags", ["--parent", "x", "--"]),
  usageRow("a single-dash flag", ["-h"]),

  unreadableRow("a missing file", false),
  unreadableRow("a directory", true),
];

function fixture(t: TestContext): string {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "agentics-protection-"));
  t.after(() => {
    assert.equal(path.dirname(root), path.resolve(os.tmpdir()));
    assert.ok(path.basename(root).startsWith("agentics-protection-"));
    fs.rmSync(root, { recursive: true, force: true });
  });
  return root;
}

/** The subset of `paths` that Git ignores, in order, with the index left out of the decision. */
function ignoredAmong(root: string, paths: string[]): string[] {
  const result = spawnSync(
    "git",
    ["check-ignore", "--no-index", "--stdin", "-z"],
    {
      cwd: root,
      encoding: "utf8",
      input: [...paths, ""].join("\0"),
      env: fixtureGitEnv(),
    },
  );
  assert.ok(result.status === 0 || result.status === 1, result.stderr);
  return result.stdout.split("\0").filter((entry) => entry !== "");
}

function assertGit(root: string, stdout: Buffer, check: GitCheck): void {
  fs.writeFileSync(path.join(root, ".gitignore"), stdout);
  git(root, "init", "--quiet");
  assert.deepEqual(
    ignoredAmong(root, [...check.notIgnored, ...check.ignored]),
    check.ignored,
    "Git must ignore exactly the extras",
  );
  for (const dir of check.dirs ?? []) {
    fs.mkdirSync(path.join(root, dir), { recursive: true });
    assert.deepEqual(
      ignoredAmong(root, [`${dir}/`, `${dir}/extra.txt`]),
      [`${dir}/`, `${dir}/extra.txt`],
      `canonical-file directory ${dir} must be ignored`,
    );
  }
}

for (const leg of LEGS)
  test(
    `planning-protection${leg.name} follows the case table`,
    { skip: leg.skip },
    async (t) => {
      for (const row of ROWS)
        await t.test(row.name, (t) => {
          const root = fixture(t);
          const inputPath = path.join(root, "input.gitignore");
          const input =
            typeof row.input === "string"
              ? Buffer.from(row.input, "utf8")
              : row.input;
          if (input !== undefined) fs.writeFileSync(inputPath, input);
          if (row.inputIsDirectory) fs.mkdirSync(inputPath);
          const [file, lead] = leg.command();
          const result = spawnExact(file, [
            ...lead,
            ...row.args.map((arg) =>
              arg === GITIGNORE ? forward(inputPath) : arg,
            ),
          ]);
          const stderr = result.stderr.toString("utf8");
          const expectedStderr =
            typeof row.stderr === "string"
              ? row.stderr
              : row.stderr(leg, forward(inputPath));
          assert.equal(stderr, expectedStderr, "stderr");
          assert.equal(result.status, row.status, stderr);
          assert.deepEqual(
            result.stdout,
            Buffer.from(row.stdout ?? "", "utf8"),
            "stdout bytes",
          );
          if (input !== undefined)
            assert.deepEqual(
              fs.readFileSync(inputPath),
              input,
              "the helper must never write",
            );
          if (row.git !== undefined) assertGit(root, result.stdout, row.git);
        });
    },
  );

test("the table has no duplicate row names and the template is the only template", () => {
  const names = ROWS.map((row) => row.name);
  assert.equal(new Set(names).size, names.length);
  assert.equal(TEMPLATE.split("{{PARENT}}").length - 1, 21);
  assert.deepEqual(
    fs
      .readdirSync(path.join(repo, "skills/scaffold/references"))
      .filter((name) => name.endsWith(".gitignore")),
    ["planning-protection.gitignore"],
  );
});
