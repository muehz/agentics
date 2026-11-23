import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { pathToFileURL } from "node:url";
import test from "node:test";
import { MUTATION_CONFIG_FILES } from "./mutation-config-values.ts";
import { selectScope } from "./mutation-scope.ts";

test("every configuration implementation stays outside mutation scope", () => {
  assert.deepEqual(selectScope([...MUTATION_CONFIG_FILES]), {
    stryker: [],
    gremlins: [],
  });
});

test("irrelevant malformed configuration environment does not initialize scope or reporting", (t) => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "mutation-lazy-config-"));
  t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
  const scope = path.join(dir, "scope.json");
  fs.writeFileSync(
    scope,
    JSON.stringify({ stryker: [], gremlins: ["./cmd/hook"] }),
  );
  const selected = path.join(dir, "selected.json");
  fs.writeFileSync(
    selected,
    JSON.stringify({ stryker: [".github/tools/src/a.ts"], gremlins: [] }),
  );
  const scopeURL = pathToFileURL(
    path.join(import.meta.dirname, "mutation-scope.ts"),
  ).href;
  const valuesURL = pathToFileURL(
    path.join(import.meta.dirname, "mutation-config-values.ts"),
  ).href;
  const report = path.join(import.meta.dirname, "mutation-report.ts");
  for (const invalid of [
    { AGX_STRYKER_TEST_COST: path.join(dir, "absent-cost.json") },
    { AGX_MUTATION_TEST_FILES: "not-json" },
  ]) {
    const env = {
      ...process.env,
      AGX_STRYKER_TEST_COST: "",
      AGX_MUTATION_TEST_FILES: "",
      ...invalid,
    };
    const pure = spawnSync(
      process.execPath,
      [
        "--input-type=module",
        "-e",
        "const m=await import(process.argv[1]); console.log(m.MUTATION_CONFIG_FILE)",
        valuesURL,
      ],
      { encoding: "utf8", env, timeout: 10000 },
    );
    assert.equal(pure.status, 0, pure.stderr);
    assert.equal(
      pure.stdout.trim(),
      ".github/tools/src/scripts/mutation-config.ts",
    );
    const scopeRun = spawnSync(
      process.execPath,
      [
        "--input-type=module",
        "-e",
        "const m=await import(process.argv[1]); console.log(JSON.stringify(m.selectScope(['.github/tools/src/a.ts'])))",
        scopeURL,
      ],
      { encoding: "utf8", env, timeout: 10000 },
    );
    assert.equal(scopeRun.status, 0, scopeRun.stderr);
    assert.deepEqual(JSON.parse(scopeRun.stdout), {
      stryker: [".github/tools/src/a.ts"],
      gremlins: [],
    });
    for (const input of [path.join(dir, "missing.json"), scope]) {
      const run = spawnSync(process.execPath, [report, "--scope", input], {
        encoding: "utf8",
        env,
        timeout: 10000,
      });
      assert.equal(run.status, 0, run.stderr);
      assert.match(
        run.stdout,
        input === scope ? /gremlins: no result/ : /Scope: no result/,
      );
    }
    const needed = spawnSync(process.execPath, [report, "--scope", selected], {
      encoding: "utf8",
      env,
      timeout: 10000,
    });
    assert.equal(needed.status, 1, needed.stdout);
    assert.match(
      needed.stderr,
      "AGX_STRYKER_TEST_COST" in invalid ? /absent-cost\.json/ : /SyntaxError/,
    );
  }
});
