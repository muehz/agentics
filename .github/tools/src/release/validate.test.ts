import assert from "node:assert/strict";
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { test } from "node:test";
import {
  runValidate,
  type Command,
  type CommandResult,
} from "../plugin/validate.ts";

const isToolsTests = (c: Command) =>
  c.tool === "npm" &&
  c.args.at(-1) === "test" &&
  c.args.at(-2)?.replaceAll("\\", "/").endsWith(".github/tools") === true;
const isBoundaries = (c: Command) =>
  c.tool === "node" &&
  c.args[0]?.replaceAll("\\", "/").endsWith("boundaries/cli.ts") === true;

for (const areas of [[], ["tools"]]) {
  for (const failure of [
    "",
    ...areas,
    ...(areas.includes("tools") ? ["boundary"] : []),
  ]) {
    const fails = failure !== "";
    test(`validate runs ${areas.join("+") || "no areas"}, failure=${failure || "none"}`, (t) => {
      const root = mkdtempSync(join(tmpdir(), "agentics-validate-"));
      t.after(() => rmSync(root, { recursive: true, force: true }));
      for (const area of areas) {
        const file = join(root, ".github", area, "package.json");
        mkdirSync(dirname(file), { recursive: true });
        writeFileSync(file, "{}");
      }
      const calls: string[] = [];
      let stdout = "";
      const runner = (command: Command): CommandResult => {
        if (isToolsTests(command)) {
          calls.push("tools");
          return {
            found: true,
            status: failure === "tools" ? 1 : 0,
            output: "",
          };
        }
        if (isBoundaries(command)) {
          calls.push("boundary");
          return {
            found: true,
            status: failure === "boundary" ? 1 : 0,
            output: "",
          };
        }
        return { found: true, status: 0, output: "" };
      };
      const code = runValidate({
        mode: "strict",
        speed: "full",
        repoRoot: root,
        toolCacheDir: join(root, "cache"),
        nodeVersion: "26.12.1",
        enginesNode: "^22.23.5 || ^24.22.3 || >=26.12.1",
        env: {},
        runner,
        write: (stream, text) => {
          if (stream === "stdout") stdout += text;
        },
      });
      assert.equal(code, fails ? 1 : 0, stdout);
      assert.equal(calls.includes("tools"), areas.includes("tools"), stdout);
      assert.equal(
        calls.includes("boundary"),
        areas.includes("tools") && failure !== "tools",
        stdout,
      );
      if (fails)
        assert.doesNotMatch(
          stdout,
          new RegExp(
            `✅ ${failure === "tools" ? "Tools" : "Area boundaries"}.*(?:passed|hold)`,
          ),
        );
    });
  }
}
