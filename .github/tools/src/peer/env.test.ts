import assert from "node:assert/strict";
import { test } from "node:test";
import { childEnvironment } from "./env.ts";

const host = {
  HOME: "/home/u",
  PATH: "/usr/bin",
  EMPTY_BUT_SET: "x",
  LC_ALL: "",
  ANTHROPIC_API_KEY: "a",
  OPENAI_API_KEY: "o",
  CODEX_HOME: "/c",
  CLAUDE_CODE_SESSION_ACCESS_TOKEN: "s",
  COPILOT_PLUGIN_ROOT: "/p",
  PLUGIN_ROOT: "/p",
  CLAUDE_PLUGIN_ROOT: "/p",
  FAKE_CODEX_MODE: "hang",
  UNRELATED: "u",
};

test("claude receives the base names and its own vendor's credentials", () => {
  const env = childEnvironment("claude", host);
  assert.deepEqual(env, {
    HOME: "/home/u",
    PATH: "/usr/bin",
    LC_ALL: "",
    ANTHROPIC_API_KEY: "a",
    FAKE_CODEX_MODE: "hang",
  });
});

test("codex receives the base names and its own vendor's credentials", () => {
  const env = childEnvironment("codex", host);
  assert.deepEqual(env, {
    HOME: "/home/u",
    PATH: "/usr/bin",
    LC_ALL: "",
    OPENAI_API_KEY: "o",
    CODEX_HOME: "/c",
    FAKE_CODEX_MODE: "hang",
  });
});

test("no target receives the host session token, plugin roots, or unlisted names", () => {
  for (const target of ["claude", "codex"] as const) {
    const env = childEnvironment(target, host);
    for (const name of [
      "CLAUDE_CODE_SESSION_ACCESS_TOKEN",
      "COPILOT_PLUGIN_ROOT",
      "PLUGIN_ROOT",
      "CLAUDE_PLUGIN_ROOT",
      "UNRELATED",
      "EMPTY_BUT_SET",
    ]) {
      assert.equal(name in env, false, `${target} received ${name}`);
    }
  }
});

// Every base name and all six vendor names are defined, so a name dropped from an allowlist, or handed to the wrong
// vendor, changes the result.
test("each target gets every base name, its own three vendor names, and none of the other vendor's", () => {
  const base = [
    "HOME",
    "LANG",
    "LC_ALL",
    "LC_CTYPE",
    "LOGNAME",
    "PATH",
    "SHELL",
    "TERM",
    "TMPDIR",
    "TZ",
    "USER",
    "__CF_USER_TEXT_ENCODING",
  ];
  const claude = [
    "ANTHROPIC_API_KEY",
    "ANTHROPIC_AUTH_TOKEN",
    "ANTHROPIC_BASE_URL",
  ];
  const codex = ["CODEX_HOME", "OPENAI_API_KEY", "OPENAI_BASE_URL"];
  const full = Object.fromEntries(
    [...base, ...claude, ...codex].map((name) => [name, `v-${name}`]),
  );
  assert.deepEqual(
    Object.keys(childEnvironment("claude", full)).sort(),
    [...base, ...claude].sort(),
  );
  assert.deepEqual(
    Object.keys(childEnvironment("codex", full)).sort(),
    [...base, ...codex].sort(),
  );
});

test("an unset name is not exported, but a set-and-empty one is", () => {
  assert.deepEqual(childEnvironment("codex", { LANG: "" }), { LANG: "" });
  assert.deepEqual(childEnvironment("codex", {}), {});
});

test("unset harness names stay absent while empty harness values are preserved", () => {
  const child = childEnvironment("codex", {
    FAKE_UNSET: undefined,
    FAKE_EMPTY: "",
  });
  assert.deepEqual(child, { FAKE_EMPTY: "" });
  assert.equal(Object.hasOwn(child, "FAKE_UNSET"), false);
});
