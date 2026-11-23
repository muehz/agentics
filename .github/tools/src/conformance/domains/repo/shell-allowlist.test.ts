import assert from "node:assert/strict";
import { mkdirSync, mkdtempSync, symlinkSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { test } from "node:test";
import type { Failure } from "../../check.ts";
import { RepoFileError } from "../../repo-file-error.ts";
import { canSymlink, removeTree, SYMLINK_SKIP } from "../../testing/host.ts";
import {
  SHELL_ALLOWLIST,
  SHELL_RULE,
  shellAllowlist,
  STALE_RULE,
} from "./shell-allowlist.ts";
import type { Tracked } from "./walk.ts";

type Entry = { path: string; reason: string };
type Options = {
  tracked?: string[];
  allowlist?: unknown;
  /** The allowlist's exact text, for a file that is not JSON. */
  allowlistText?: string;
  read?: (file: string) => Buffer;
  before?: (root: string) => void;
};

/** Runs the rule on a fresh root holding `files`, with `tracked` (default: every file) as git's file list. */
function audit(
  files: Record<string, string | Buffer>,
  options: Options = {},
): Failure[] {
  const root = mkdtempSync(path.join(tmpdir(), "agentics-shellallow-"));
  try {
    for (const [rel, content] of Object.entries(files)) put(root, rel, content);
    if (options.allowlist !== undefined)
      put(root, SHELL_ALLOWLIST, JSON.stringify(options.allowlist));
    if (options.allowlistText !== undefined)
      put(root, SHELL_ALLOWLIST, options.allowlistText);
    options.before?.(root);
    const tracked: Tracked = {
      files: new Set(options.tracked ?? Object.keys(files)),
      dirs: new Set(),
    };
    return shellAllowlist(
      root,
      tracked,
      options.read === undefined ? {} : { read: options.read },
    );
  } finally {
    removeTree(root);
  }
}

function put(root: string, rel: string, content: string | Buffer): void {
  const target = path.join(root, ...rel.split("/"));
  mkdirSync(path.dirname(target), { recursive: true });
  writeFileSync(target, content);
}

const unlisted = (rel: string): Failure => ({
  rule: SHELL_RULE,
  path: rel,
  message: `${rel} is a shell or PowerShell file and is not listed in ${SHELL_ALLOWLIST}. Port it to TypeScript, or add an entry with the reason it must stay`,
});

const stale = (rel: string): Failure => ({
  rule: STALE_RULE,
  path: SHELL_ALLOWLIST,
  message: `allowlist entry for ${rel} names no tracked file. Remove the entry`,
});

const entry = (rel: string): Entry => ({ path: rel, reason: "a reason" });

test("the constants name the allowlist file and the two rules", () => {
  assert.equal(
    SHELL_ALLOWLIST,
    ".github/tools/src/conformance/shell-allowlist.json",
  );
  assert.equal(SHELL_RULE, "shell-allowlist");
  assert.equal(STALE_RULE, "shell-allowlist-stale");
});

test("an unlisted .ps1 is reported", () => {
  assert.deepEqual(audit({ "x.ps1": "Write-Output 1\n" }), [unlisted("x.ps1")]);
});

test("an unlisted extensionless bash shebang file is reported", () => {
  assert.deepEqual(audit({ "bin/agx": "#!/usr/bin/env bash\necho hi\n" }), [
    unlisted("bin/agx"),
  ]);
});

test("an allowlist entry naming no tracked file is stale", () => {
  assert.deepEqual(
    audit({ "a.txt": "text\n" }, { allowlist: [entry("gone.sh")] }),
    [stale("gone.sh")],
  );
});

test("an entry whose file exists on disk but is untracked is stale", () => {
  assert.deepEqual(
    audit(
      { "run.sh": "echo\n" },
      { tracked: [], allowlist: [entry("run.sh")] },
    ),
    [stale("run.sh")],
  );
});

test("a listed shell file passes", () => {
  assert.deepEqual(
    audit(
      { "run.sh": "echo\n", "bin/agx": "#!/bin/sh\n" },
      { allowlist: [entry("run.sh"), entry("bin/agx")] },
    ),
    [],
  );
});

test("a listed entry that is not a shell file is not stale", () => {
  assert.deepEqual(
    audit({ "notes.txt": "text\n" }, { allowlist: [entry("notes.txt")] }),
    [],
  );
});

for (const name of [
  "a.sh",
  "a.bash",
  "a.ps1",
  "a.psm1",
  "a.cmd",
  "a.bat",
  "A.PS1",
  "run.CMD",
  "Mixed.Sh",
  "dir.d/x.bat",
]) {
  test(`the file ${name} counts as shell by its extension`, () => {
    assert.deepEqual(audit({ [name]: "anything\n" }), [unlisted(name)]);
  });
}

test("other extensions do not count", () => {
  assert.deepEqual(
    audit({
      "a.txt": "#!/bin/sh\n",
      "a.py": "#!/usr/bin/env python3\n",
      "a.shx": "x\n",
      "a.zsh": "#!/bin/zsh\n",
      sh: "plain\n",
    }),
    [],
  );
});

for (const [label, first] of [
  ["sh", "#!/bin/sh"],
  ["env bash", "#!/usr/bin/env bash"],
  ["env zsh", "#!/usr/bin/env zsh"],
  ["env pwsh", "#!/usr/bin/env pwsh"],
  ["env powershell", "#!/usr/bin/env powershell"],
  ["bash", "#!/bin/bash"],
  ["zsh", "#!/bin/zsh"],
  ["pwsh", "#!/usr/bin/pwsh"],
  ["a space after the marker", "#! /bin/sh"],
  ["a trailing CR", "#!/bin/sh\r"],
  ["env with a flag", "#!/usr/bin/env -S bash -e"],
  [
    "env -S with an assignment before the command",
    "#!/usr/bin/env -S MODE=test sh",
  ],
  [
    "env -S with an option that takes an operand",
    "#!/usr/bin/env -S -u MODE sh",
  ],
  ["env with an assignment", "#!/usr/bin/env VAR=x bash"],
  ["env -u with its operand", "#!/usr/bin/env -u NAME bash"],
  ["env --unset with an attached operand", "#!/usr/bin/env --unset=NAME bash"],
  ["env --unset with a separate operand", "#!/usr/bin/env --unset NAME bash"],
  ["env -uNAME with an attached operand", "#!/usr/bin/env -uNAME bash"],
  ["env -C with its operand", "#!/usr/bin/env -C /tmp bash"],
  ["env --chdir with an attached operand", "#!/usr/bin/env --chdir=/tmp bash"],
  ["env -CDIR with an attached operand", "#!/usr/bin/env -C/tmp bash"],
  ["env -P with its operand", "#!/usr/bin/env -P /bin:/usr/bin bash"],
  ["env -a with its operand", "#!/usr/bin/env -a NAME bash"],
  ["env --argv0 with an attached operand", "#!/usr/bin/env --argv0=NAME bash"],
  ["env -i with an assignment", "#!/usr/bin/env -i PATH=/bin sh"],
  [
    "env -S with an option, an operand and an assignment",
    "#!/usr/bin/env -S -u MODE FOO=1 pwsh",
  ],
  [
    "env with a clustered option and its operand",
    "#!/usr/bin/env -iu NAME bash",
  ],
  ["env with a string attached to -S", "#!/usr/bin/env -SMODE=1 bash"],
  [
    "env --split-string with an attached string",
    "#!/usr/bin/env --split-string=MODE=1 bash",
  ],
  ["env after -0, -v and --", "#!/usr/bin/env -0 -v -- bash"],
  ["env after -- and an assignment", "#!/usr/bin/env -- VAR=x bash"],
  ["env with a lone dash", "#!/usr/bin/env - bash"],
  ["env running a path to bash", "#!/usr/bin/env /bin/bash"],
  ["an interpreter argument", "#!/bin/sh -eu"],
  ["a byte order mark", "\uFEFF#!/bin/sh"],
]) {
  test(`an extensionless file starting ${label} is shell`, () => {
    assert.deepEqual(audit({ "bin/tool": `${first}\nbody\n` }), [
      unlisted("bin/tool"),
    ]);
  });
}

for (const [label, first] of [
  ["env python3", "#!/usr/bin/env python3"],
  ["env bashful", "#!/usr/bin/env bashful"],
  ["env -u with a python3 command", "#!/usr/bin/env -u NAME python3"],
  [
    "env with an assignment and a python3 command",
    "#!/usr/bin/env VAR=x python3",
  ],
  [
    "env -S with an assignment and a python3 command",
    "#!/usr/bin/env -S MODE=test python3",
  ],
  ["env whose operand is a shell name", "#!/usr/bin/env -u bash"],
  ["env with only assignments", "#!/usr/bin/env VAR=x"],
  ["env with an option after an assignment", "#!/usr/bin/env VAR=x -i bash"],
  ["env with a non-identifier assignment", "#!/usr/bin/env 1VAR=x bash"],
  ["env -C with an operand and a dash command", "#!/usr/bin/env -C /tmp dash"],
  ["dash", "#!/bin/dash"],
  ["node", "#!/usr/bin/env node"],
  ["a bare env", "#!/usr/bin/env"],
  ["an empty interpreter", "#!"],
  ["no marker", "bash"],
  ["a marker after a blank line", "\n#!/bin/sh"],
  ["a marker after a space", " #!/bin/sh"],
]) {
  test(`an extensionless file starting ${label} is not shell`, () => {
    assert.deepEqual(audit({ "bin/tool": `${first}\nbody\n` }), []);
  });
}

test("a shebang on the first line of a file with no newline counts", () => {
  assert.deepEqual(audit({ "bin/tool": "#!/bin/sh" }), [unlisted("bin/tool")]);
});

test("a shell shebang only on a later line does not count", () => {
  assert.deepEqual(audit({ "bin/tool": "text\n#!/bin/sh\n" }), []);
});

test("an extensionless file with a NUL byte is binary and not shell", () => {
  assert.deepEqual(
    audit({ "bin/tool": Buffer.from("#!/bin/sh\n\0binary", "utf8") }),
    [],
  );
});

test("a NUL byte before the first newline hides the shebang", () => {
  assert.deepEqual(audit({ "bin/tool": Buffer.from("#!/bin/sh\0\n") }), []);
});

test("extensionless is judged on the basename, so a dotted directory does not hide a file", () => {
  assert.deepEqual(audit({ "pkg.d/bin/tool": "#!/bin/sh\n" }), [
    unlisted("pkg.d/bin/tool"),
  ]);
});

test("a dotfile with no extension is read as extensionless", () => {
  assert.deepEqual(audit({ ".envrc": "#!/bin/bash\n" }), [unlisted(".envrc")]);
});

test("a file with a dotted basename and no shell extension is not read", () => {
  const reads: string[] = [];
  assert.deepEqual(
    audit(
      { "bin/tool.v2": "#!/bin/sh\n" },
      {
        read: (file) => {
          reads.push(file);
          return Buffer.from("#!/bin/sh\n");
        },
      },
    ),
    [],
  );
  assert.deepEqual(reads, []);
});

test("a tracked shell file under a skipped directory is still reported", () => {
  assert.deepEqual(
    audit({
      "node_modules/pkg/run.sh": "x\n",
      ".git/hooks/pre-commit": "#!/bin/sh\n",
      "evals/runs/a/run.ps1": "x\n",
    }),
    [
      unlisted(".git/hooks/pre-commit"),
      unlisted("evals/runs/a/run.ps1"),
      unlisted("node_modules/pkg/run.sh"),
    ],
  );
});

test("an untracked shell file on disk is not reported", () => {
  assert.deepEqual(
    audit(
      { "run.sh": "x\n", "bin/tool": "#!/bin/sh\n", "ok.txt": "x\n" },
      { tracked: ["ok.txt"] },
    ),
    [],
  );
});

test("an allowlist path is compared exactly, so a different case is unlisted and stale", () => {
  assert.deepEqual(audit({ "X.PS1": "x\n" }, { allowlist: [entry("x.ps1")] }), [
    stale("x.ps1"),
    unlisted("X.PS1"),
  ]);
});

test("an allowlist path with a backslash is not normalised", () => {
  assert.deepEqual(
    audit({ "dir/run.sh": "x\n" }, { allowlist: [entry("dir\\run.sh")] }),
    [stale("dir\\run.sh"), unlisted("dir/run.sh")],
  );
});

test("an extension-marked tracked file missing from the tree is reported without a read", () => {
  assert.deepEqual(audit({}, { tracked: ["gone.sh"] }), [unlisted("gone.sh")]);
});

test("an extensionless tracked path missing from the tree is skipped", () => {
  assert.deepEqual(audit({}, { tracked: ["bin/gone"] }), []);
});

test("a tracked directory with no extension, such as a submodule, is skipped", () => {
  assert.deepEqual(
    audit(
      {},
      {
        tracked: ["vendor/sub"],
        before: (root) =>
          mkdirSync(path.join(root, "vendor", "sub"), { recursive: true }),
      },
    ),
    [],
  );
});

test("a symlink named x.sh counts by its name", (t) => {
  if (!canSymlink()) return t.skip(SYMLINK_SKIP);
  assert.deepEqual(
    audit(
      { "target.txt": "x\n" },
      {
        tracked: ["x.sh"],
        before: (root) =>
          symlinkSync(path.join(root, "target.txt"), path.join(root, "x.sh")),
      },
    ),
    [unlisted("x.sh")],
  );
});

test("an extensionless symlink to a shell script is skipped and never read", (t) => {
  if (!canSymlink()) return t.skip(SYMLINK_SKIP);
  const reads: string[] = [];
  assert.deepEqual(
    audit(
      { "real/tool": "#!/bin/sh\n" },
      {
        tracked: ["link"],
        before: (root) =>
          symlinkSync(path.join(root, "real", "tool"), path.join(root, "link")),
        read: (file) => {
          reads.push(file);
          return Buffer.from("#!/bin/sh\n");
        },
      },
    ),
    [],
  );
  assert.deepEqual(reads, []);
});

test("only a bounded prefix of an extensionless file is read", () => {
  const padded = `#!/usr/bin/env${" ".repeat(300)}bash\n`;
  assert.deepEqual(audit({ "bin/tool": padded }), []);
  const near = `#!/usr/bin/env${" ".repeat(200)}bash\n`;
  assert.deepEqual(audit({ "bin/tool": near }), [unlisted("bin/tool")]);
});

test("a read that fails names the file", () => {
  assert.throws(
    () =>
      audit(
        { "bin/tool": "#!/bin/sh\n" },
        {
          read: () => {
            throw Object.assign(new Error("denied"), { code: "EACCES" });
          },
        },
      ),
    (error: unknown) =>
      error instanceof RepoFileError &&
      error.kind === "unreadable" &&
      error.message === "bin/tool: EACCES",
  );
});

test("findings sort by path, then message", () => {
  const result = audit(
    { "z.sh": "x\n", "a/b.sh": "x\n", "m.txt": "x\n" },
    { allowlist: [entry("b/gone.sh"), entry("m.txt")] },
  );
  assert.deepEqual(result, [
    stale("b/gone.sh"),
    unlisted("a/b.sh"),
    unlisted("z.sh"),
  ]);
  assert.deepEqual(
    result.map((failure) => failure.path),
    [SHELL_ALLOWLIST, "a/b.sh", "z.sh"],
  );
});

test("two findings on one path sort by message", () => {
  const result = audit(
    {},
    {
      tracked: [],
      allowlist: [entry("b.sh"), entry("a.sh")],
    },
  );
  assert.deepEqual(result, [stale("a.sh"), stale("b.sh")]);
});

test("a tree with no allowlist reads it as empty, so every shell file is reported", () => {
  const failures = audit({
    "a.sh": "x\n",
    "bin/tool": "#!/bin/bash\n",
    "c.txt": "x\n",
  });
  assert.deepEqual(failures, [unlisted("a.sh"), unlisted("bin/tool")]);
  for (const failure of failures)
    assert.ok(failure.message.includes(SHELL_ALLOWLIST));
});

test("an allowlist file that is empty of entries reports every shell file", () => {
  assert.deepEqual(audit({ "a.sh": "x\n" }, { allowlist: [] }), [
    unlisted("a.sh"),
  ]);
});

for (const [label, allowlist, reason] of [
  ["is not an array", { path: "a.sh", reason: "r" }, "expected a JSON array"],
  ["has an entry that is null", [null], "entry 0 is not an object"],
  ["has an entry that is a string", ["a.sh"], "entry 0 is not an object"],
  ["has an entry that is an array", [[]], "entry 0 is not an object"],
  [
    "has an entry without a path",
    [{ reason: "r" }],
    "entry 0 needs a non-empty string path",
  ],
  [
    "has an entry with a numeric path",
    [{ path: 1, reason: "r" }],
    "entry 0 needs a non-empty string path",
  ],
  [
    "has an entry with an empty path",
    [{ path: "", reason: "r" }],
    "entry 0 needs a non-empty string path",
  ],
  [
    "has an entry without a reason",
    [{ path: "a.sh" }],
    "entry 0 needs a non-empty reason",
  ],
  [
    "has an entry with a numeric reason",
    [{ path: "a.sh", reason: 1 }],
    "entry 0 needs a non-empty reason",
  ],
  [
    "has an entry with an empty reason",
    [{ path: "a.sh", reason: "" }],
    "entry 0 needs a non-empty reason",
  ],
  [
    "has an entry with a whitespace-only reason",
    [{ path: "a.sh", reason: " \t\n" }],
    "entry 0 needs a non-empty reason",
  ],
  [
    "has a duplicate path",
    [entry("a.sh"), entry("b.sh"), entry("a.sh")],
    "entry 2 duplicates an earlier entry for a.sh",
  ],
] as const) {
  test(`an allowlist that ${label} is a read error`, () => {
    assert.throws(() => audit({ "a.sh": "x\n" }, { allowlist }), {
      message: `${SHELL_ALLOWLIST}: ${reason}`,
    });
  });
}

test("an allowlist that is not JSON is a read error", () => {
  assert.throws(() => audit({}, { allowlistText: "[" }), {
    message: `${SHELL_ALLOWLIST}: invalid JSON`,
  });
});

test("an entry with an empty reason fails even when its file is tracked and listed", () => {
  assert.throws(
    () =>
      audit({ "a.sh": "x\n" }, { allowlist: [{ path: "a.sh", reason: "" }] }),
    { message: `${SHELL_ALLOWLIST}: entry 0 needs a non-empty reason` },
  );
});
