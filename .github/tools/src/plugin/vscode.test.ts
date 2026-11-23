import assert from "node:assert/strict";
import { test } from "node:test";
import {
  repositoryKey,
  vscodeInstructions,
  vscodeSettingsPath,
} from "./vscode.ts";

test("darwin settings live under Application Support", () => {
  assert.equal(
    vscodeSettingsPath("code", { HOME: "/Users/u" }, "darwin"),
    "/Users/u/Library/Application Support/Code/User/settings.json",
  );
});

test("linux settings use XDG_CONFIG_HOME when set", () => {
  assert.equal(
    vscodeSettingsPath(
      "code",
      { HOME: "/home/u", XDG_CONFIG_HOME: "/cfg" },
      "linux",
    ),
    "/cfg/Code/User/settings.json",
  );
});

test("linux treats an empty XDG_CONFIG_HOME as unset", () => {
  assert.equal(
    vscodeSettingsPath(
      "code",
      { HOME: "/home/u", XDG_CONFIG_HOME: "" },
      "linux",
    ),
    "/home/u/.config/Code/User/settings.json",
  );
});

test("linux without HOME or XDG_CONFIG_HOME prints a literal tilde", () => {
  for (const env of [{}, { HOME: "", XDG_CONFIG_HOME: "" }])
    assert.equal(
      vscodeSettingsPath("code", env, "linux"),
      "~/.config/Code/User/settings.json",
    );
});

test("win32 settings resolve under APPDATA with native separators", () => {
  assert.equal(
    vscodeSettingsPath(
      "code",
      { APPDATA: "C:\\Users\\u\\AppData\\Roaming" },
      "win32",
    ),
    "C:\\Users\\u\\AppData\\Roaming\\Code\\User\\settings.json",
  );
});

test("win32 with an empty APPDATA prints the literal variable", () => {
  assert.equal(
    vscodeSettingsPath("code", { APPDATA: "" }, "win32"),
    "%APPDATA%\\Code\\User\\settings.json",
  );
});

test("insiders replaces Code with Code - Insiders on every platform", () => {
  const env = { HOME: "/h", XDG_CONFIG_HOME: "/c", APPDATA: "C:\\A" };
  assert.equal(
    vscodeSettingsPath("insiders", env, "darwin"),
    "/h/Library/Application Support/Code - Insiders/User/settings.json",
  );
  assert.equal(
    vscodeSettingsPath("insiders", env, "linux"),
    "/c/Code - Insiders/User/settings.json",
  );
  assert.equal(
    vscodeSettingsPath("insiders", env, "win32"),
    "C:\\A\\Code - Insiders\\User\\settings.json",
  );
});

test("an unknown platform resolves no path", () => {
  assert.equal(
    vscodeSettingsPath("code", { HOME: "/h" }, "freebsd"),
    undefined,
  );
});

test("the repository key uses forward slashes on win32 only", () => {
  assert.equal(repositoryKey("C:\\Users\\u\\repo", "win32"), "C:/Users/u/repo");
  assert.equal(repositoryKey("/home/u/repo", "linux"), "/home/u/repo");
  assert.equal(repositoryKey("/home/u/a\\b", "linux"), "/home/u/a\\b");
});

test("an unknown platform prints the manual instruction instead of a path", () => {
  const lines = vscodeInstructions("insiders", {}, "freebsd", "/r");
  assert.ok(
    lines.includes(
      "⚠ Cannot determine the VS Code Insiders settings path on this platform.",
    ),
  );
  assert.ok(
    lines.includes(
      "  Open the file with: Preferences: Open User Settings (JSON)",
    ),
  );
  assert.ok(!lines.includes("Settings file:"));
});

test("the win32 block holds JSON-valid forward-slash repository keys", () => {
  const lines = vscodeInstructions(
    "code",
    { APPDATA: "C:\\A" },
    "win32",
    "C:\\Users\\u\\repo",
  );
  assert.ok(lines.includes('    "C:/Users/u/repo": true'));
  assert.ok(lines.includes("  C:\\A\\Code\\User\\settings.json"));
});
