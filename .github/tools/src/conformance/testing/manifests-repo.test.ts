import assert from "node:assert/strict";
import { existsSync, readdirSync, readFileSync } from "node:fs";
import path from "node:path";
import { test } from "node:test";
import type { RunOptions } from "../check.ts";
import { run } from "../domains/manifests/index.ts";
import {
  auditManifests,
  CLAUDE_PLUGIN,
  editJson,
  mkdir,
  remove,
  withManifestsRepo,
} from "./manifests-repo.ts";

const options: RunOptions = {
  short: false,
  fast: false,
  diagnostics: () => {},
  env: process.env,
};

test("the fixture passes every rule through run", async () => {
  await withManifestsRepo(async (root) => {
    assert.deepEqual(await run(root, options), { failures: [] });
  });
});

test("auditManifests finds no failures and no error on the clean fixture", async () => {
  await withManifestsRepo(async (root) => {
    assert.deepEqual(await auditManifests(root), { failures: [] });
  });
});

test("the fixture holds exactly the listed files and directories and nothing else", async () => {
  await withManifestsRepo((root) => {
    const files = readdirSync(root, { recursive: true, withFileTypes: true })
      .filter((entry) => entry.isFile())
      .map((entry) =>
        path
          .relative(root, path.join(entry.parentPath, entry.name))
          .split(path.sep)
          .join("/"),
      )
      .sort();
    assert.deepEqual(files, [
      ".agents/plugins/marketplace.json",
      ".antigravity-plugin/plugin.json",
      ".claude-plugin/marketplace.json",
      ".claude-plugin/plugin.json",
      ".codex-plugin/plugin.json",
      ".github/plugin/plugin.json",
      "AGENTS.md",
      "assets/logo.svg",
      "hooks/hooks.claude.json",
      "hooks/hooks.codex.json",
      "hooks/hooks.copilot.json",
      "hooks/run-hook.cmd",
    ]);
    assert.ok(existsSync(path.join(root, "skills", "tdd")));
    assert.ok(existsSync(path.join(root, ".claude")));
  });
});

test("withManifestsRepo gives each body its own directory and removes it, even when the body throws", async () => {
  let seen = "";
  await assert.rejects(
    withManifestsRepo((root) => {
      seen = root;
      assert.ok(existsSync(root));
      throw new Error("body failed");
    }),
    /body failed/,
  );
  assert.equal(existsSync(seen), false);
  await withManifestsRepo((first) =>
    withManifestsRepo((second) => {
      assert.notEqual(first, second);
    }),
  );
});

test("editJson changes one field and keeps the others", async () => {
  await withManifestsRepo((root) => {
    editJson(root, CLAUDE_PLUGIN, (value) => {
      value.version = "9.9.9";
    });
    const written = JSON.parse(
      readFileSync(path.join(root, ".claude-plugin", "plugin.json"), "utf8"),
    );
    assert.equal(written.version, "9.9.9");
    assert.equal(written.name, "agentics");
    assert.deepEqual(written.skills, ["./skills/tdd"]);
  });
});

test("mkdir creates parents and remove accepts a path that is already gone", async () => {
  await withManifestsRepo((root) => {
    mkdir(root, "a/b/c");
    assert.ok(existsSync(path.join(root, "a", "b", "c")));
    remove(root, "a");
    assert.equal(existsSync(path.join(root, "a")), false);
    remove(root, "a");
  });
});
