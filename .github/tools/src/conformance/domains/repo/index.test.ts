import assert from "node:assert/strict";
import fs, {
  chmodSync,
  mkdtempSync,
  mkdirSync,
  rmSync,
  unlinkSync,
  writeFileSync,
} from "node:fs";
import { syncBuiltinESMExports } from "node:module";
import { tmpdir } from "node:os";
import path from "node:path";
import { test } from "node:test";
import type { RunOptions } from "../../check.ts";
import {
  initCheckout,
  tempIsInsideRepository,
} from "../../testing/git-checkout.ts";
import { auditRepo, HOOKS_MOD, withRepoRepo } from "../../testing/repo-repo.ts";
import {
  CONFLICT_BLOCK,
  OUTSIDE_INVISIBLE_EDGES,
  WORK_MOD,
  writeBytes,
} from "../../testing/repo-seeds.ts";
import { writeFile } from "../../testing/files.ts";
import { run } from "./index.ts";
import { SHELL_ALLOWLIST } from "./shell-allowlist.ts";
import {
  canLinkDirectory,
  DIRECTORY_LINK_SKIP,
  linkDirectory,
  removeTree,
  makesUnreadableFilesOnLinux,
  withOutside,
} from "../../testing/host.ts";

const options: RunOptions = {
  short: false,
  fast: false,
  diagnostics: () => {},
  env: process.env,
};

async function withDir(body: (root: string) => Promise<void>): Promise<void> {
  const root = mkdtempSync(path.join(tmpdir(), "agentics-repoindex-"));
  try {
    await body(root);
  } finally {
    removeTree(root);
  }
}

test("a checkout that scans nothing is an audit error", async () => {
  await withDir(async (root) => {
    assert.deepEqual(await run(root, options), {
      failures: [],
      error:
        "repo: the hygiene scan read 0 files — a guard that inspects nothing must never report success",
    });
  });
});

test("a tree holding only a binary file scans nothing and is an audit error", async () => {
  await withDir(async (root) => {
    writeFile(root, "blob.bin", "\0binary\n");
    assert.deepEqual(await run(root, options), {
      failures: [],
      error:
        "repo: the hygiene scan read 0 files — a guard that inspects nothing must never report success",
    });
  });
});

test("an audit error in a later step drops earlier failures and carries its own prefix", async () => {
  await withDir(async (root) => {
    writeFile(root, "a.md", "<<<<<<< HEAD\n");
    writeFile(root, "hooks/go/go.mod", "require (\n");
    const result = await run(root, options);
    assert.deepEqual(result.failures, []);
    assert.match(
      result.error ?? "",
      /^repo: checking area Go modules: hooks\/go\/go\.mod: unterminated block/,
    );
  });
});

test("a failed hygiene read returns the named audit error", async (t) => {
  await withDir(async (root) => {
    writeFile(root, "unreadable.md", "text\n");
    const target = path.join(root, "unreadable.md");
    const open = fs.openSync;
    const mocked = t.mock.method(
      fs,
      "openSync",
      (...args: Parameters<typeof open>) => {
        if (args[0] === target)
          throw Object.assign(new Error("fixture read refused"), {
            code: "EACCES",
          });
        return open(...args);
      },
    );
    syncBuiltinESMExports();
    try {
      assert.deepEqual(await run(root, options), {
        failures: [],
        error: "repo: unreadable.md: EACCES",
      });
    } finally {
      mocked.mock.restore();
      syncBuiltinESMExports();
    }
  });
});

test(
  "a .git that is not a repository is an audit error",
  {
    skip: tempIsInsideRepository()
      ? "the temp directory is inside a git repository"
      : false,
  },
  async () => {
    await withDir(async (root) => {
      writeFile(root, "a.md", "x\n");
      mkdirSync(path.join(root, ".git"));
      assert.match(
        (await run(root, options)).error ?? "",
        /^repo: listing tracked files: /,
      );
    });
  },
);

const block =
  "before\n<<<<<<< HEAD\nmine\n=======\ntheirs\n>>>>>>> other\nafter\n";
const cp = (...points: number[]) => String.fromCodePoint(...points);

/** The failures of one run as `rule|path`, asserting the audit itself did not error. */
async function found(root: string): Promise<string[]> {
  const result = await run(root, options);
  assert.equal(result.error, undefined);
  return result.failures.map((f) => `${f.rule}|${f.path}`);
}

test("the fixture passes", async () => {
  await withRepoRepo(async (root) => {
    assert.deepEqual(await run(root, options), { failures: [] });
  });
});

test("conflict-marker fires on the opening and closing line, with the line number, and not on look-alikes", async () => {
  await withRepoRepo(async (root) => {
    writeFile(root, "AGENTS.md", block);
    const result = await run(root, options);
    assert.deepEqual(
      result.failures.map(
        (f) => `${f.rule}|${f.path}|${f.message.slice(0, 28)}`,
      ),
      [
        'conflict-marker|AGENTS.md|line 2 begins with "<<<<<<<"',
        'conflict-marker|AGENTS.md|line 6 begins with ">>>>>>>"',
      ],
    );
    writeFile(
      root,
      "AGENTS.md",
      "Title\n=======\n    <<<<<<< HEAD\ninline >>>>>>> x\n",
    );
    assert.deepEqual(await found(root), []);
  });
});

test("invisible-unicode names the line and code point, and a selector is exempt only after a symbol", async () => {
  await withRepoRepo(async (root) => {
    writeFile(root, "docs/x.md", `a${cp(0x200b)}b\n`);
    assert.deepEqual((await run(root, options)).failures, [
      {
        rule: "invisible-unicode",
        path: "docs/x.md",
        message:
          "line 1 holds U+200B (zero-width character), invisible in review but read by a model; delete it, or write it as a \\u or \\U escape in code.",
      },
    ]);
    writeFile(root, "docs/x.md", `${cp(0x27a1, 0xfe0f)}\n`);
    assert.deepEqual(await found(root), []);
    writeFile(root, "docs/x.md", `a${cp(0xfe0f)}\n`);
    assert.deepEqual(await found(root), ["invisible-unicode|docs/x.md"]);
  });
});

test("a NUL in the first 8000 bytes hides a file from both scans, and byte 8000 does not", async () => {
  await withRepoRepo(async (root) => {
    for (const offset of [7999, 8000]) {
      const bytes = Buffer.alloc(8001, 0x78);
      bytes[offset] = 0;
      writeFileSync(
        path.join(root, `n${offset}.txt`),
        Buffer.concat([bytes, Buffer.from("\n<<<<<<< HEAD\n")]),
      );
    }
    assert.deepEqual(await found(root), ["conflict-marker|n8000.txt"]);
  });
});

test("every skip-list entry hides a file in a tree that is not a checkout, and a near miss does not", async () => {
  await withRepoRepo(async (root) => {
    for (const entry of [
      "sub/.git",
      ".claude/worktrees",
      "node_modules",
      "a/node_modules",
      ".worktrees",
      "evals/runs",
      "docs/handoffs",
      "docs/probes",
      "docs/reviews/peer",
      ".agentics",
      ".audit",
    ])
      writeFile(root, `${entry}/x.md`, block);
    writeFile(root, "docs/specs/2026-10/feature-x/temp/x.md", block);
    assert.deepEqual(await found(root), []);
    writeFile(root, "docs/handoffs.md", block);
    writeFile(root, "a/evals/runs/y.md", block);
    writeFile(root, "docs/temp/z.md", block);
    assert.deepEqual((await found(root)).sort(), [
      "conflict-marker|a/evals/runs/y.md",
      "conflict-marker|a/evals/runs/y.md",
      "conflict-marker|docs/handoffs.md",
      "conflict-marker|docs/handoffs.md",
      "conflict-marker|docs/temp/z.md",
      "conflict-marker|docs/temp/z.md",
    ]);
  });
});

test("in a checkout a tracked file under a skipped directory or a work folder's temp is scanned and an untracked one is not", async () => {
  await withRepoRepo(async (root) => {
    const forced = [
      "node_modules/pkg/forced.md",
      "docs/specs/2026-10/feature-x/temp/forced.md",
    ];
    for (const rel of [
      ...forced,
      "node_modules/pkg/loose.md",
      "docs/specs/2026-10/feature-x/temp/loose.md",
    ])
      writeFile(root, rel, block);
    initCheckout(root, forced);
    writeFile(root, ".git/MERGE_MSG", block);
    assert.deepEqual((await found(root)).sort(), [
      "conflict-marker|docs/specs/2026-10/feature-x/temp/forced.md",
      "conflict-marker|docs/specs/2026-10/feature-x/temp/forced.md",
      "conflict-marker|node_modules/pkg/forced.md",
      "conflict-marker|node_modules/pkg/forced.md",
    ]);
  });
});

test("the area rules fire on a stray go.work and a hooks require", async () => {
  await withRepoRepo(async (root) => {
    writeFile(root, "go.work", "go 1.27.3\n");
    writeFile(
      root,
      "hooks/go/go.mod",
      `${HOOKS_MOD}require github.com/google/go-cmp v0.7.0\n`,
    );
    assert.deepEqual(await found(root), [
      "area-go-require|hooks/go/go.mod",
      "area-go-module-set|go.work",
    ]);
  });
});

test("the Go module audit covers both canonical and legacy tools directories", async () => {
  await withRepoRepo(async (root) => {
    writeFile(root, ".github/tools/src/go.work", "go 1.27.3\n");
    writeFile(root, "tools/src/go.mod", HOOKS_MOD);
    writeFile(root, ".github/tools-extra/src/go.work", "go 1.27.3\n");
    writeFile(root, "tools-extra/src/go.mod", HOOKS_MOD);
    assert.deepEqual(await found(root), [
      "area-go-module-set|.github/tools/src/go.work",
      "area-go-module-set|tools/src/go.mod",
    ]);
  });
});

test("a go.work under a work folder's temp is reported only when tracked", async () => {
  await withRepoRepo(async (root) => {
    writeFile(root, "tools/2026-11/feature-x/temp/go.work", "go 1.27.3\n");
    initCheckout(root);
    assert.deepEqual(await found(root), []);
  });
  await withRepoRepo(async (root) => {
    writeFile(root, "tools/2026-11/feature-x/go.work", "go 1.27.3\n");
    initCheckout(root);
    assert.deepEqual(await found(root), [
      "area-go-module-set|tools/2026-11/feature-x/go.work",
    ]);
  });
  await withRepoRepo(async (root) => {
    writeFile(root, "tools/2026-11/feature-x/temp/go.work", "go 1.27.3\n");
    initCheckout(root, ["tools/2026-11/feature-x/temp/go.work"]);
    assert.deepEqual(await found(root), [
      "area-go-module-set|tools/2026-11/feature-x/temp/go.work",
    ]);
  });
});

test("hygiene findings precede area findings", async () => {
  await withRepoRepo(async (root) => {
    writeFile(root, "go.work", "go 1.27.3\n");
    writeFile(root, "AGENTS.md", "<<<<<<< HEAD\n");
    assert.deepEqual(await found(root), [
      "conflict-marker|AGENTS.md",
      "area-go-module-set|go.work",
    ]);
  });
});

test("a tracked shell file that the allowlist does not list is reported", async () => {
  await withRepoRepo(async (root) => {
    writeFile(root, "x.ps1", "Write-Output 1\n");
    initCheckout(root, ["x.ps1"]);
    assert.deepEqual(await found(root), ["shell-allowlist|x.ps1"]);
  });
});

test("a tracked shell file that the allowlist lists passes", async () => {
  await withRepoRepo(async (root) => {
    writeFile(root, "x.ps1", "Write-Output 1\n");
    writeFile(
      root,
      SHELL_ALLOWLIST,
      '[{ "path": "x.ps1", "reason": "a fixture" }]\n',
    );
    initCheckout(root, ["x.ps1"]);
    assert.deepEqual(await found(root), []);
  });
});

test("an allowlist entry that no tracked file matches is reported", async () => {
  await withRepoRepo(async (root) => {
    writeFile(
      root,
      SHELL_ALLOWLIST,
      '[{ "path": "gone.sh", "reason": "a fixture" }]\n',
    );
    initCheckout(root);
    assert.deepEqual(await found(root), [
      `shell-allowlist-stale|${SHELL_ALLOWLIST}`,
    ]);
  });
});

test("an untracked shell file is not reported", async () => {
  await withRepoRepo(async (root) => {
    writeFile(root, "x.ps1", "Write-Output 1\n");
    initCheckout(root);
    assert.deepEqual(await found(root), []);
  });
});

test("a malformed allowlist is an audit error that names the shell allowlist step", async () => {
  await withRepoRepo(async (root) => {
    writeFile(root, SHELL_ALLOWLIST, "{}\n");
    initCheckout(root);
    assert.deepEqual(await run(root, options), {
      failures: [],
      error: `repo: checking the shell allowlist: ${SHELL_ALLOWLIST}: expected a JSON array`,
    });
  });
});

test("an allowlist that cannot be read as a file is an audit error that names the shell allowlist step", async () => {
  await withRepoRepo(async (root) => {
    mkdirSync(path.join(root, ...SHELL_ALLOWLIST.split("/")), {
      recursive: true,
    });
    initCheckout(root);
    assert.deepEqual(await run(root, options), {
      failures: [],
      error: `repo: checking the shell allowlist: ${SHELL_ALLOWLIST}: is not a regular file`,
    });
  });
});

test("shell findings follow hygiene and area findings", async () => {
  await withRepoRepo(async (root) => {
    writeFile(root, "go.work", "go 1.27.3\n");
    writeFile(root, "AGENTS.md", "<<<<<<< HEAD\n");
    writeFile(root, "x.ps1", "Write-Output 1\n");
    initCheckout(root, ["x.ps1"]);
    assert.deepEqual(await found(root), [
      "conflict-marker|AGENTS.md",
      "area-go-module-set|go.work",
      "shell-allowlist|x.ps1",
    ]);
  });
});

const PASS = { failures: [] };

test("a clean fixture", async () => {
  await withRepoRepo(async (root) => {
    assert.deepEqual(await auditRepo(root), PASS);
  });
});

test("a clean fixture that is a git checkout", async () => {
  await withRepoRepo(async (root) => {
    initCheckout(root, []);
    assert.deepEqual(await auditRepo(root), PASS);
  });
});

test("an indented, an inline and a middle marker", async () => {
  await withRepoRepo(async (root) => {
    writeFile(root, "AGENTS.md", "    <<<<<<< a\ninline <<<<<<< b\n=======\n");
    assert.deepEqual(await auditRepo(root), PASS);
  });
});

test("a NUL at byte 7999 hides a marker", async () => {
  await withRepoRepo(async (root) => {
    const data = Buffer.alloc(8001, 0x78);
    data[7999] = 0;
    writeBytes(
      root,
      "w.txt",
      Buffer.concat([data, Buffer.from("\n<<<<<<< HEAD\n")]),
    );
    assert.deepEqual(await auditRepo(root), PASS);
  });
});

test("a variation selector after a symbol", async () => {
  await withRepoRepo(async (root) => {
    writeFile(root, "docs/x.md", `${cp(0x27a1, 0xfe0f)}\n`);
    assert.deepEqual(await auditRepo(root), PASS);
  });
});

test("a selector after an invalid byte", async () => {
  await withRepoRepo(async (root) => {
    writeBytes(
      root,
      "docs/x.md",
      Buffer.concat([Buffer.from([0xff]), Buffer.from(cp(0xfe0f))]),
    );
    assert.deepEqual(await auditRepo(root), PASS);
  });
});

test("an invisible character and a marker in a skipped directory", async () => {
  await withRepoRepo(async (root) => {
    writeFile(root, "node_modules/x.md", cp(0x200b));
    writeFile(root, "sub/.git/x.md", CONFLICT_BLOCK);
    assert.deepEqual(await auditRepo(root), PASS);
  });
});

test("git's own directory in a checkout is skipped", async () => {
  await withRepoRepo(async (root) => {
    initCheckout(root, []);
    writeFile(root, ".git/MERGE_MSG", CONFLICT_BLOCK);
    assert.deepEqual(await auditRepo(root), PASS);
  });
});

test("a go.mod under testdata, tracked or not, is ignored", async () => {
  await withRepoRepo(async (root) => {
    writeFile(root, "hooks/go/internal/x/testdata/go.mod", "module x\n");
    writeFile(root, "hooks/go/internal/y/testdata/go.mod", "module y\n");
    initCheckout(root, ["hooks/go/internal/x/testdata/go.mod"]);
    assert.deepEqual(await auditRepo(root), PASS);
  });
});

test("the code point just outside every invisible range", async () => {
  await withRepoRepo(async (root) => {
    for (const point of OUTSIDE_INVISIBLE_EDGES)
      writeFile(root, `inv-out/${point.toString(16)}.md`, `a${cp(point)}b\n`);
    assert.deepEqual(await auditRepo(root), PASS);
  });
});

test("a checkout with every fixture file tracked and nothing seeded", async () => {
  await withRepoRepo(async (root) => {
    initCheckout(root, ["."]);
    assert.deepEqual(await auditRepo(root), PASS);
  });
});

test(
  "a .git directory that is not a repository",
  {
    skip: tempIsInsideRepository()
      ? "the temp directory is inside a git repository, so git finds that one instead of failing"
      : false,
  },
  async () => {
    await withRepoRepo(async (root) => {
      mkdirSync(path.join(root, ".git"));
      assert.deepEqual(await auditRepo(root), {
        failures: [],
        error:
          "repo: listing tracked files: git exited with code 128: fatal: not a git repository (or any of the parent directories): .git",
      });
    });
  },
);

test("a checkout with no readable text file", async () => {
  await withRepoRepo(async (root) => {
    removeTree(root);
    writeBytes(root, ".claude-plugin/plugin.json", Buffer.from([0]));
    assert.deepEqual(await auditRepo(root), {
      failures: [],
      error:
        "repo: the hygiene scan read 0 files — a guard that inspects nothing must never report success",
    });
  });
});

test("an unterminated go.mod block", async () => {
  await withRepoRepo(async (root) => {
    writeFile(root, "hooks/go/go.mod", "require (\n");
    assert.deepEqual(await auditRepo(root), {
      failures: [],
      error:
        "repo: checking area Go modules: hooks/go/go.mod: unterminated block",
    });
  });
});

test("a go.mod with a block comment", async () => {
  await withRepoRepo(async (root) => {
    writeFile(root, "hooks/go/go.mod", `${HOOKS_MOD}/* c */\n`);
    assert.deepEqual(await auditRepo(root), {
      failures: [],
      error:
        "repo: checking area Go modules: hooks/go/go.mod: mod files must use // comments (not /* */ comments)",
    });
  });
});

test("a replace with a glued arrow", async () => {
  await withRepoRepo(async (root) => {
    writeFile(root, "hooks/go/go.mod", WORK_MOD("replace a v1=>b"));
    assert.deepEqual(await auditRepo(root), {
      failures: [],
      error:
        "repo: checking area Go modules: hooks/go/go.mod: usage: replace module/path [v1.2.3] => other/module v1.4 or ../local/directory",
    });
  });
});

test("a hooks go.mod that is a directory", async () => {
  await withRepoRepo(async (root) => {
    rmSync(path.join(root, "hooks", "go", "go.mod"));
    mkdirSync(path.join(root, "hooks", "go", "go.mod"));
    assert.deepEqual(await auditRepo(root), {
      failures: [],
      error:
        "repo: checking area Go modules: hooks/go/go.mod: is not a regular file",
    });
  });
});

test(
  "a repository path that is itself a symlink scans nothing",
  { skip: canLinkDirectory() ? false : DIRECTORY_LINK_SKIP },
  async () => {
    await withRepoRepo((root) =>
      withOutside(async (outside) => {
        const link = path.join(outside, "repo-link");
        assert.notEqual(linkDirectory(root, link), undefined);
        try {
          assert.deepEqual(await auditRepo(link), {
            failures: [],
            error:
              "repo: the hygiene scan read 0 files — a guard that inspects nothing must never report success",
          });
        } finally {
          unlinkSync(link);
        }
      }),
    );
  },
);

test(
  "a regular file the audit cannot read",
  {
    skip: makesUnreadableFilesOnLinux()
      ? false
      : "only Linux as a non-root user denies the read: Windows ignores the mode, root reads the file anyway, and the expected reason is pinned to Linux",
  },
  async () => {
    await withRepoRepo(async (root) => {
      writeFile(root, "docs/locked.md", "x\n");
      chmodSync(path.join(root, "docs", "locked.md"), 0o000);
      assert.deepEqual(await auditRepo(root), {
        failures: [],
        error: "repo: docs/locked.md: EACCES",
      });
    });
  },
);
