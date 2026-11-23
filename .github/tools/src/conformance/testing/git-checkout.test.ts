import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { mkdirSync, readFileSync, readdirSync, writeFileSync } from "node:fs";
import path from "node:path";
import { test } from "node:test";
import { INLINE_FAMILY } from "../domains/skills/frontmatter.ts";
import { writeFile } from "./files.ts";
import { git, initCheckout } from "./git-checkout.ts";
import { withOutside } from "./host.ts";
import { auditManifests, withManifestsRepo } from "./manifests-repo.ts";
import { auditRepo, withRepoRepo } from "./repo-repo.ts";
import { auditSkills, withSkillsRepo } from "./skills-repo.ts";

test("host Git settings cannot redirect staging, change bytes or run hooks in a fixture", () => {
  withOutside((base) => {
    const root = path.join(base, "fixture");
    const other = path.join(base, "other");
    const hooks = path.join(base, "hooks");
    for (const dir of [root, other, hooks]) mkdirSync(dir);
    const cleanEnv = {
      ...Object.fromEntries(
        Object.entries(process.env).filter(([key]) => !/^GIT_/i.test(key)),
      ),
      HOME: base,
      XDG_CONFIG_HOME: base,
      GIT_CONFIG_GLOBAL: path.join(base, "empty-config"),
      GIT_CONFIG_NOSYSTEM: "1",
    };
    const inspect = (dir: string, ...args: string[]) =>
      execFileSync("git", ["-C", dir, ...args], { env: cleanEnv });
    inspect(other, "init", "-q", "--template=");
    writeFileSync(path.join(other, "sentinel"), "outside\n");
    inspect(other, "add", "sentinel");
    inspect(
      other,
      "-c",
      "user.name=Other",
      "-c",
      "user.email=other@example.invalid",
      "commit",
      "-qm",
      "outside",
    );
    const gitDir = path.join(other, ".git");
    const snapshot = () => ({
      head: inspect(other, "rev-parse", "HEAD"),
      index: readFileSync(path.join(gitDir, "index")),
      objects: readdirSync(path.join(gitDir, "objects"), {
        recursive: true,
      }).sort(),
      sentinel: readFileSync(path.join(other, "sentinel")),
    });
    const before = snapshot();
    const globalConfig = path.join(base, "global-config");
    const systemConfig = path.join(base, "system-config");
    inspect(other, "config", "--file", globalConfig, "core.hooksPath", hooks);
    inspect(
      other,
      "config",
      "--file",
      globalConfig,
      "fixture.global",
      "leaked",
    );
    inspect(
      other,
      "config",
      "--file",
      systemConfig,
      "fixture.system",
      "leaked",
    );
    writeFileSync(
      path.join(hooks, "pre-commit"),
      "#!/bin/sh\necho unexpected-fixture-hook >&2\nexit 1\n",
      { mode: 0o755 },
    );
    writeFileSync(path.join(root, "bytes.txt"), "one\r\ntwo\r\n");
    writeFileSync(path.join(root, "lf.txt"), "three\n");
    writeFileSync(path.join(root, ".gitattributes"), "lf.txt text\n");
    writeFileSync(path.join(root, "untracked"), "leave out\n");

    // Only this child inherits the sacrificial checkout, index, objects, identity and configuration.
    const module = new URL("./git-checkout.ts", import.meta.url).href;
    const child = `
      import { unlinkSync } from "node:fs";
      import { join } from "node:path";
      import { git, initCheckout } from ${JSON.stringify(module)};
      const root = process.argv[1];
      initCheckout(root, [".gitattributes", "bytes.txt", "lf.txt"]);
      const staged = git(root, "diff", "--cached", "--name-only");
      git(root, "commit", "-qm", "fixture");
      unlinkSync(join(root, "lf.txt"));
      git(root, "checkout-index", "-af");
      const config = git(root, "config", "--list");
      console.log(JSON.stringify({ staged, config }));
    `;
    const output = execFileSync(
      process.execPath,
      ["--input-type=module", "-e", child, root],
      {
        encoding: "utf8",
        env: {
          ...cleanEnv,
          GIT_DIR: gitDir,
          GIT_COMMON_DIR: gitDir,
          GIT_WORK_TREE: other,
          GIT_INDEX_FILE: path.join(gitDir, "index"),
          GIT_OBJECT_DIRECTORY: path.join(gitDir, "objects"),
          GIT_CONFIG_NOSYSTEM: "0",
          GIT_CONFIG_GLOBAL: globalConfig,
          GIT_CONFIG_SYSTEM: systemConfig,
          GIT_CONFIG_COUNT: "4",
          GIT_CONFIG_KEY_0: "commit.gpgsign",
          GIT_CONFIG_VALUE_0: "true",
          GIT_CONFIG_KEY_1: "core.autocrlf",
          GIT_CONFIG_VALUE_1: "true",
          GIT_CONFIG_KEY_2: "core.eol",
          GIT_CONFIG_VALUE_2: "crlf",
          GIT_CONFIG_KEY_3: "fixture.command",
          GIT_CONFIG_VALUE_3: "leaked",
          GIT_AUTHOR_NAME: "Other",
          GIT_AUTHOR_EMAIL: "other@example.invalid",
          GIT_COMMITTER_NAME: "Other",
          GIT_COMMITTER_EMAIL: "other@example.invalid",
        },
      },
    );
    const { staged, config } = JSON.parse(output);
    assert.deepEqual(staged.trimEnd().split("\n"), [
      ".gitattributes",
      "bytes.txt",
      "lf.txt",
    ]);
    assert.doesNotMatch(config, /fixture\.(?:global|system|command)=leaked/);
    assert.deepEqual(
      inspect(root, "ls-tree", "--name-only", "HEAD").toString(),
      staged,
    );
    assert.deepEqual(
      inspect(root, "cat-file", "blob", "HEAD:bytes.txt"),
      Buffer.from("one\r\ntwo\r\n"),
    );
    assert.deepEqual(
      readFileSync(path.join(root, "lf.txt")),
      Buffer.from("three\n"),
    );
    assert.equal(
      inspect(root, "log", "-1", "--format=%an <%ae> %cn <%ce> %s")
        .toString()
        .trim(),
      "Agentics Test <test@example.com> Agentics Test <test@example.com> fixture",
    );
    assert.deepEqual(snapshot(), before);
  });
});

test("a fixture command cannot discover a checkout above its root", () => {
  withOutside((parent) => {
    initCheckout(parent);
    const child = path.join(parent, "uninitialized");
    mkdirSync(child);
    assert.throws(
      () => git(child, "rev-parse", "--git-dir"),
      /not a git repository/,
    );
  });
});

test("the temp probe finds real ancestors under hostile Git config and discovery settings", () => {
  withOutside((base) => {
    const repo = path.join(base, "repo");
    const inside = path.join(repo, "tmp");
    const outside = path.join(base, "plain");
    mkdirSync(inside, { recursive: true });
    mkdirSync(outside);
    // Keep the control outside a repository even when the host temp directory itself is in a checkout.
    writeFileSync(path.join(base, ".git"), "gitdir: missing\n");
    initCheckout(repo);
    const module = new URL("./git-checkout.ts", import.meta.url).href;
    const child = `
      import { tempIsInsideRepository } from ${JSON.stringify(module)};
      const env = process.env;
      console.log(JSON.stringify({
        config: tempIsInsideRepository({ ...env, GIT_CONFIG_COUNT: "invalid" }),
        ceiling: tempIsInsideRepository({ ...env, GIT_CEILING_DIRECTORIES: process.argv[1] }),
      }));
    `;
    const probe = (temp: string) =>
      JSON.parse(
        execFileSync(
          process.execPath,
          ["--input-type=module", "-e", child, repo],
          {
            encoding: "utf8",
            env: {
              ...process.env,
              TMPDIR: temp,
              TEMP: temp,
              TMP: temp,
              GIT_DIR: path.join(repo, ".git"),
            },
          },
        ),
      );
    assert.deepEqual(
      { inside: probe(inside), outside: probe(outside) },
      {
        inside: { config: true, ceiling: true },
        outside: { config: false, ceiling: false },
      },
    );
  });
});

test("conformance fixtures read their own index even with invalid inherited Git config", async () => {
  const env = { ...process.env, GIT_CONFIG_COUNT: "invalid" };
  const temp = "docs/specs/2026-11/feature-x/temp";
  await withRepoRepo(async (root) => {
    const file = `${temp}/conflict.md`;
    writeFile(root, file, "<<<<<<< HEAD\n");
    initCheckout(root, [file]);
    const result = await auditRepo(root, { env });
    assert.equal(result.error, undefined);
    assert.deepEqual(
      result.failures.map((f) => [f.rule, f.path]),
      [["conflict-marker", file]],
    );
  });
  await withManifestsRepo(async (root) => {
    const file = `${temp}/.mcp.json`;
    writeFile(root, file, "{}\n");
    initCheckout(root, [file]);
    const result = await auditManifests(root, { env });
    assert.equal(result.error, undefined);
    assert.deepEqual(
      result.failures.map((f) => [f.rule, f.path]),
      [["no-mcp-json", file]],
    );
  });
  await withSkillsRepo(async (root) => {
    const dir = `${temp}/skills/missing`;
    const file = `${dir}/README.md`;
    writeFile(root, file, "fixture\n");
    initCheckout(root, [file]);
    const result = await auditSkills(root, { env, families: [INLINE_FAMILY] });
    assert.equal(result.error, undefined);
    assert.deepEqual(
      result.failures.map((f) => [f.rule, f.path]),
      [["skill-md-present", dir]],
    );
  });
});
