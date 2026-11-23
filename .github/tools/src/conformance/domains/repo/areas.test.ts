import assert from "node:assert/strict";
import { mkdirSync, mkdtempSync, rmSync, symlinkSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { test } from "node:test";
import { RepoFileError } from "../../repo-file-error.ts";
import { initCheckout } from "../../testing/git-checkout.ts";
import { writeFile } from "../../testing/files.ts";
import { WORK_MOD } from "../../testing/repo-seeds.ts";
import { auditRepo, HOOKS_MOD, withRepoRepo } from "../../testing/repo-repo.ts";
import { areaGoModules } from "./areas.ts";
import { trackedPaths, type Tracked } from "./walk.ts";
import { canSymlink, removeTree, SYMLINK_SKIP } from "../../testing/host.ts";

const NONE: Tracked = { files: new Set(), dirs: new Set() };
const HOOKS = "module github.com/muehz/agentics/hooks/go\n\ngo 1.27.3\n";
const STRAY_MOD =
  "a Go module outside hooks/go; hooks/go is the only module root";
const STRAY_WORK =
  "a go.work file joins modules into one build; hooks/go is the only module root";

function tree(
  files: Record<string, string>,
  body: (root: string) => void,
): void {
  const root = mkdtempSync(path.join(tmpdir(), "agentics-repoarea-"));
  try {
    const all = { "hooks/go/go.mod": HOOKS, ...files };
    for (const [rel, content] of Object.entries(all))
      writeFile(root, rel, content);
    body(root);
  } finally {
    removeTree(root);
  }
}

const one = (files: Record<string, string>): string[] => {
  let found: string[] = [];
  tree(files, (root) => {
    found = areaGoModules(root, NONE).map(
      (f) => `${f.rule}|${f.path}|${f.message}`,
    );
  });
  return found;
};

test("a tree whose only module file is hooks/go/go.mod reports nothing", () =>
  assert.deepEqual(one({}), []));

test("hooks may require or replace nothing", () => {
  assert.deepEqual(
    one({
      "hooks/go/go.mod": `${HOOKS}require github.com/google/go-cmp v0.7.0\n`,
    }),
    [
      "area-go-require|hooks/go/go.mod|requires github.com/google/go-cmp; hooks/go requires no module",
    ],
  );
  assert.deepEqual(
    one({
      "hooks/go/go.mod": `${HOOKS}replace github.com/foo/bar => github.com/foo/baz v1.0.0\n`,
    }),
    [
      "area-go-require|hooks/go/go.mod|replaces github.com/foo/bar; hooks/go requires no module",
    ],
  );
});

test("a go.mod the reader rejects is an error naming it", () => {
  tree({ "hooks/go/go.mod": "require (\n\ta v1\n" }, (root) =>
    assert.throws(
      () => areaGoModules(root, NONE),
      /^Error: hooks\/go\/go\.mod: unterminated block/,
    ),
  );
});

test("go.work and go.mod files outside hooks/go are reported", () => {
  assert.deepEqual(one({ "go.work": "go 1.27.3\n" }), [
    `area-go-module-set|go.work|${STRAY_WORK}`,
  ]);
  assert.deepEqual(one({ "tools/cli/go.work": "go 1.27.3\n" }), [
    `area-go-module-set|tools/cli/go.work|${STRAY_WORK}`,
  ]);
  assert.deepEqual(one({ "skills/go.work": "x" }), [
    `area-go-module-set|skills/go.work|${STRAY_WORK}`,
  ]);
  assert.deepEqual(one({ "docs/scratch/go.work": "go 1.27.3\n" }), []);
  assert.deepEqual(one({ "go.mod": HOOKS }), [
    `area-go-module-set|go.mod|${STRAY_MOD}`,
  ]);
  assert.deepEqual(one({ "skills/execute/go.mod": HOOKS }), [
    `area-go-module-set|skills/execute/go.mod|${STRAY_MOD}`,
  ]);
  assert.deepEqual(one({ "tools/other/go.mod": HOOKS }), [
    `area-go-module-set|tools/other/go.mod|${STRAY_MOD}`,
  ]);
  assert.deepEqual(one({ "docs/x/go.mod": HOOKS }), []);
  assert.deepEqual(one({ "hooks/go/internal/x/testdata/go.mod": HOOKS }), []);
});

test("a go.mod or go.work under .github/tools is reported, and one elsewhere under .github is not", () => {
  assert.deepEqual(one({ ".github/tools/src/go.mod": HOOKS }), [
    `area-go-module-set|.github/tools/src/go.mod|${STRAY_MOD}`,
  ]);
  assert.deepEqual(one({ ".github/tools/src/go.work": "go 1.27.3\n" }), [
    `area-go-module-set|.github/tools/src/go.work|${STRAY_WORK}`,
  ]);
  assert.deepEqual(
    one({
      ".github/go.mod": HOOKS,
      ".github/toolsx/go.mod": HOOKS,
      ".github/workflows/go.work": "go 1.27.3\n",
    }),
    [],
  );
});

test("a module one directory down in evals or tools is reported, whatever its directory is called", () => {
  assert.deepEqual(one({ "evals/engine/go.mod": HOOKS }), [
    `area-go-module-set|evals/engine/go.mod|${STRAY_MOD}`,
  ]);
  assert.deepEqual(one({ "tools/cli/go.mod": HOOKS }), [
    `area-go-module-set|tools/cli/go.mod|${STRAY_MOD}`,
  ]);
});

test("a module file directly inside a covered directory is reported", () => {
  for (const dir of ["hooks", "evals", "tools", "skills", ".github/tools"]) {
    assert.deepEqual(one({ [`${dir}/go.mod`]: HOOKS }), [
      `area-go-module-set|${dir}/go.mod|${STRAY_MOD}`,
    ]);
    assert.deepEqual(one({ [`${dir}/go.work`]: "go 1.27.3\n" }), [
      `area-go-module-set|${dir}/go.work|${STRAY_WORK}`,
    ]);
  }
});

test("only hooks/go/go.mod itself is the module root", () => {
  assert.deepEqual(
    one({
      "hooks/go/go.work": "go 1.27.3\n",
      "hooks/go/internal/go.mod": HOOKS,
    }),
    [
      `area-go-module-set|hooks/go/go.work|${STRAY_WORK}`,
      `area-go-module-set|hooks/go/internal/go.mod|${STRAY_MOD}`,
    ],
  );
  assert.deepEqual(one({ "hooksx/go.mod": HOOKS }), []);
});

test("a tracked go.mod under a skipped directory is checked and an untracked one is local output", () => {
  tree(
    {
      "tools/node_modules/pkg/go.mod": HOOKS,
      ".github/tools/node_modules/other/go.mod": HOOKS,
    },
    (root) => {
      initCheckout(root, ["tools/node_modules/pkg/go.mod"]);
      const found = areaGoModules(root, trackedPaths(root, process.env));
      assert.deepEqual(
        found.map((f) => f.path),
        ["tools/node_modules/pkg/go.mod"],
      );
    },
  );
});

test("a missing hooks/go/go.mod is skipped", () => {
  tree({}, (root) => {
    rmSync(path.join(root, "hooks", "go", "go.mod"));
    assert.deepEqual(areaGoModules(root, NONE), []);
  });
});

test("a hooks/go/go.mod that cannot be read for another reason is rethrown", () => {
  tree({}, (root) => {
    const file = path.join(root, "hooks", "go", "go.mod");
    rmSync(file);
    mkdirSync(file);
    assert.throws(
      () => areaGoModules(root, NONE),
      (error) =>
        error instanceof RepoFileError &&
        error.kind === "not-a-file" &&
        error.message === "hooks/go/go.mod: is not a regular file",
    );
  });
});

test("only a file named exactly go.work or go.mod counts", () => {
  assert.deepEqual(
    one({
      ".github/tools/ugo.work": "x",
      ".github/tools/ugo.mod": HOOKS,
      "tools/cli/go.mod.bak": HOOKS,
    }),
    [],
  );
});

test("go.work under hooks and evals is reported", () => {
  for (const area of ["hooks", "evals"])
    assert.deepEqual(one({ [`${area}/x/go.work`]: "x" }), [
      `area-go-module-set|${area}/x/go.work|${STRAY_WORK}`,
    ]);
});

test("hooks reports the no-module message for a module path under the repository's own prefix", () => {
  const agentics = "github.com/muehz/agentics/evals/engine";
  assert.deepEqual(
    one({ "hooks/go/go.mod": `${HOOKS}require ${agentics} v0.0.0\n` }),
    [
      `area-go-require|hooks/go/go.mod|requires ${agentics}; hooks/go requires no module`,
    ],
  );
  assert.deepEqual(
    one({
      "hooks/go/go.mod": `${HOOKS}replace ${agentics} => github.com/foo/bar v1.0.0\n`,
    }),
    [
      `area-go-require|hooks/go/go.mod|replaces ${agentics}; hooks/go requires no module`,
    ],
  );
});

test("a second module in hooks or evals is reported", () => {
  for (const area of ["hooks", "evals"])
    assert.deepEqual(one({ [`${area}/other/go.mod`]: HOOKS }), [
      `area-go-module-set|${area}/other/go.mod|${STRAY_MOD}`,
    ]);
});

const finding = (rule: string, rel: string, message: string) => ({
  rule,
  path: rel,
  message,
});
const hooksRequires = (module: string) =>
  finding(
    "area-go-require",
    "hooks/go/go.mod",
    `requires ${module}; hooks/go requires no module`,
  );
const hooksReplaces = (module: string) =>
  finding(
    "area-go-require",
    "hooks/go/go.mod",
    `replaces ${module}; hooks/go requires no module`,
  );
const goWork = (rel: string) => finding("area-go-module-set", rel, STRAY_WORK);
const strayModule = (rel: string) =>
  finding("area-go-module-set", rel, STRAY_MOD);

test("area-go-require", async () => {
  await withRepoRepo(async (root) => {
    writeFile(
      root,
      "hooks/go/go.mod",
      `${HOOKS_MOD}require github.com/google/go-cmp v0.7.0\n`,
    );
    assert.deepEqual(await auditRepo(root), {
      failures: [hooksRequires("github.com/google/go-cmp")],
    });
  });
});

test("area-go-module-set", async () => {
  await withRepoRepo(async (root) => {
    writeFile(root, "go.work", "go 1.27.3\n");
    assert.deepEqual(await auditRepo(root), {
      failures: [goWork("go.work")],
    });
  });
});

test("a go.mod and a go.work under .github/tools", async () => {
  await withRepoRepo(async (root) => {
    writeFile(root, ".github/tools/pkg/go.mod", "module x\n");
    writeFile(root, ".github/tools/src/go.work", "go 1.27.3\n");
    assert.deepEqual(await auditRepo(root), {
      failures: [
        strayModule(".github/tools/pkg/go.mod"),
        goWork(".github/tools/src/go.work"),
      ],
    });
  });
});

test(
  "a symlink named go.mod counts as a go.mod",
  { skip: canSymlink() ? false : SYMLINK_SKIP },
  async () => {
    await withRepoRepo(async (root) => {
      mkdirSync(path.join(root, "tools", "other"), { recursive: true });
      symlinkSync(
        path.join(root, "hooks", "go", "go.mod"),
        path.join(root, "tools", "other", "go.mod"),
      );
      assert.deepEqual(await auditRepo(root), {
        failures: [strayModule("tools/other/go.mod")],
      });
    });
  },
);

test("a require block with a quoted path and a trailing comment", async () => {
  await withRepoRepo(async (root) => {
    writeFile(
      root,
      "hooks/go/go.mod",
      `${HOOKS_MOD}require (\n\t"github.com/foo/quoted" v0.0.0 // c\n\tgithub.com/foo/bare v0.0.0\n)\n`,
    );
    assert.deepEqual(await auditRepo(root), {
      failures: [
        hooksRequires("github.com/foo/quoted"),
        hooksRequires("github.com/foo/bare"),
      ],
    });
  });
});

test("every forward-slash replace shape", async () => {
  await withRepoRepo(async (root) => {
    writeFile(
      root,
      "hooks/go/go.mod",
      WORK_MOD(
        [
          "replace github.com/foo/p1 => ./x",
          "replace github.com/foo/p2 => ../x",
          "replace github.com/foo/p3 => /abs",
          "replace github.com/foo/p4 => .",
          "replace github.com/foo/p5 => ..",
          "replace github.com/foo/p6 => c:x",
          "replace github.com/foo/p7 v1.0.0 => ../y",
          "replace github.com/foo/p8 => github.com/muehz/agentics/evals/engine v0.0.0",
          "replace github.com/foo/p9 => github.com/foo/q v1.0.0",
        ].join("\n"),
      ),
    );
    assert.deepEqual(await auditRepo(root), {
      failures: [1, 2, 3, 4, 5, 6, 7, 8, 9].map((n) =>
        hooksReplaces(`github.com/foo/p${n}`),
      ),
    });
  });
});

test("a go.work at the root, in an area, in skills, and in docs/scratch", async () => {
  await withRepoRepo(async (root) => {
    for (const rel of [
      "go.work",
      "tools/cli/go.work",
      "skills/go.work",
      "docs/scratch/go.work",
    ])
      writeFile(root, rel, "go 1.27.3\n");
    assert.deepEqual(await auditRepo(root), {
      failures: [
        goWork("go.work"),
        goWork("skills/go.work"),
        goWork("tools/cli/go.work"),
      ],
    });
  });
});

test("a go.mod at the root, in skills, and a second one in each area", async () => {
  await withRepoRepo(async (root) => {
    for (const rel of [
      "go.mod",
      "skills/execute/go.mod",
      "hooks/other/go.mod",
      "evals/other/go.mod",
      "tools/other/go.mod",
    ])
      writeFile(root, rel, "module x\n");
    assert.deepEqual(await auditRepo(root), {
      failures: [
        strayModule("evals/other/go.mod"),
        strayModule("go.mod"),
        strayModule("hooks/other/go.mod"),
        strayModule("skills/execute/go.mod"),
        strayModule("tools/other/go.mod"),
      ],
    });
  });
});

test("a tracked go.mod under a skipped directory is checked", async () => {
  await withRepoRepo(async (root) => {
    writeFile(root, "tools/node_modules/pkg/go.mod", "module x\n");
    writeFile(root, ".github/tools/node_modules/other/go.mod", "module x\n");
    initCheckout(root, ["tools/node_modules/pkg/go.mod"]);
    assert.deepEqual(await auditRepo(root), {
      failures: [strayModule("tools/node_modules/pkg/go.mod")],
    });
  });
});

test("a go.mod with indirect comments and several require blocks", async () => {
  await withRepoRepo(async (root) => {
    writeFile(
      root,
      "hooks/go/go.mod",
      `${HOOKS_MOD}\nrequire (\n\tgithub.com/foo/a v1.0.0 // indirect\n)\n\nrequire (\n\tgithub.com/foo/x v0.0.0 // indirect\n\tgithub.com/foo/b v1.0.0\n)\n\nrequire github.com/foo/c v1.0.0 // indirect\n`,
    );
    assert.deepEqual(await auditRepo(root), {
      failures: ["a", "x", "b", "c"].map((name) =>
        hooksRequires(`github.com/foo/${name}`),
      ),
    });
  });
});

test(
  "a symlink named go.work counts as a go.work",
  { skip: canSymlink() ? false : SYMLINK_SKIP },
  async () => {
    await withRepoRepo(async (root) => {
      mkdirSync(path.join(root, "tools", "cli"), { recursive: true });
      symlinkSync(
        path.join(root, "README.md"),
        path.join(root, "tools", "cli", "go.work"),
      );
      assert.deepEqual(await auditRepo(root), {
        failures: [goWork("tools/cli/go.work")],
      });
    });
  },
);
