import assert from "node:assert/strict";
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import path from "node:path";
import { test } from "node:test";
import { bashSkip, extractBashBlock, templateRepo } from "./template-run.ts";

test("extracts the requested Bash fence from the named section", () => {
  const markdown = [
    "# Earlier section",
    "```bash",
    "printf earlier",
    "```",
    "## Chosen section",
    "```text",
    "explanation",
    "```",
    "```bash",
    "printf first",
    "```",
    "```bash",
    "printf second",
    "```",
    "## Later section",
    "```bash",
    "printf later",
    "```",
  ].join("\n");
  assert.equal(
    extractBashBlock(markdown, "Chosen section", 1).trim(),
    "printf second",
  );
  assert.throws(
    () => extractBashBlock(markdown, "Chosen section", 2),
    /Chosen section/,
  );
});

const skills = new URL("../../../../../skills/", import.meta.url);
const bash = { skip: bashSkip };

function block(file: string, heading: string): string {
  return extractBashBlock(readFileSync(new URL(file, skills), "utf8"), heading);
}

test(
  "stack-base resolver accepts the tracking ref and refuses missing or malformed names",
  bash,
  (t) => {
    const repo = templateRepo(t);
    repo.git("commit", "--allow-empty", "-m", "base");
    const expected = repo.git("rev-parse", "HEAD").trim();
    repo.git("update-ref", "refs/remotes/origin/team/base", expected);
    const remoteFile = path.join(repo.base, "remote.txt");
    writeFileSync(remoteFile, "origin\n");
    repo.stub("gh", "printf '%s\\n' \"$AGX_FIXTURE_BASE\"");
    const resolver = block(
      "execute/references/plan-divergence-and-trail.md",
      "The stack-base resolver",
    );
    const remote = { AGX_STACK_REMOTE_FILE: remoteFile };
    const there = repo.run(resolver, [], repo.root, {
      ...remote,
      AGX_FIXTURE_BASE: "team/base",
    });
    assert.equal(
      there.status,
      0,
      there.stderr || there.error?.message || "unexpected script status",
    );
    assert.equal(there.stdout.trim(), expected);
    for (const name of ["team/missing", "team/not a ref", "team/base\nother"]) {
      const absent = repo.run(resolver, [], repo.root, {
        ...remote,
        AGX_FIXTURE_BASE: name,
      });
      assert.equal(
        absent.status,
        1,
        absent.stderr || absent.error?.message || name,
      );
      assert.equal(absent.stdout, "", name);
      assert.equal(absent.stderr.trim(), "not computed", name);
    }
  },
);

for (const fromSubdir of [false, true]) {
  for (const target of ["commit", "stash"]) {
    test(
      `${target} unstages exactly the selected literal name from ${fromSubdir ? "a subdirectory" : "the root"}`,
      bash,
      (t) => {
        const repo = templateRepo(t);
        repo.git("commit", "--allow-empty", "-m", "base");
        const sub = path.join(repo.root, "sub");
        mkdirSync(sub);
        const literal = process.platform === "win32" ? "[a].txt" : "*.txt";
        const untouched = ["a.txt", "sub/quoted ' name.txt"];
        for (const file of [literal, ...untouched])
          writeFileSync(path.join(repo.root, file), "fixture\n");
        repo.git(
          "add",
          "--",
          ...[literal, ...untouched].map((name) => `:(top,literal)${name}`),
        );
        const cwd = fromSubdir ? sub : repo.root;
        const command =
          target === "commit"
            ? block("git/references/commit.md", "Unstage one out-of-scope path")
            : block(
                "git/references/history-operations.md",
                "Unstage a confirmed path outside the stash",
              );
        const selected = target === "commit" ? [] : [untouched[1]!];
        const absent = repo.run(command, ["missing.txt", ...selected], cwd);
        assert.equal(
          absent.status,
          1,
          absent.stderr || absent.error?.message || "unexpected script status",
        );
        const staged = () =>
          repo
            .git("diff", "--cached", "--name-only", "-z")
            .split("\0")
            .filter(Boolean)
            .sort();
        assert.deepEqual(staged(), [literal, ...untouched].sort());
        const result = repo.run(command, [literal, ...selected], cwd);
        assert.equal(
          result.status,
          0,
          result.stderr || result.error?.message || "unexpected script status",
        );
        assert.deepEqual(staged(), [...untouched].sort());
        assert.equal(
          readFileSync(path.join(repo.root, literal), "utf8"),
          "fixture\n",
        );
      },
    );
  }
}
