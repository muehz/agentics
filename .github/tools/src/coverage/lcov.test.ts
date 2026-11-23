import assert from "node:assert/strict";
import { readFileSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { resolve, join } from "node:path";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import { test } from "node:test";
import { parseLcov, mergeLcov } from "./lcov.ts";
import { parseInput } from "./cli.ts";

const root = fileURLToPath(new URL("../../../../", import.meta.url));
const fixture = (name: string) =>
  fileURLToPath(new URL(`./fixtures/${name}`, import.meta.url));

test("input separator distinguishes drive prefixes from the workspace separator", () => {
  assert.deepEqual(parseInput("C:\\repo\\tools:C:\\tmp\\coverage.info"), {
    workspace: "C:\\repo\\tools",
    file: "C:\\tmp\\coverage.info",
  });
  assert.deepEqual(parseInput("tools:/tmp/coverage.info"), {
    workspace: "tools",
    file: "/tmp/coverage.info",
  });
  assert.deepEqual(parseInput("tools:C:/tmp/coverage.info"), {
    workspace: "tools",
    file: "C:/tmp/coverage.info",
  });
  assert.throws(() => parseInput("C:/repo/tools"), /workspace.*lcov/i);
  assert.throws(() => parseInput("tools:a:b"), /ambiguous/i);
});

test("Windows absolute and parent paths normalize against the workspace", () => {
  const report = parseLcov(readFileSync(fixture("windows.lcov"), "utf8"), {
    root: "C:\\repo",
    workspace: "C:\\repo\\.github\\tools",
    label: "windows.lcov",
  });
  assert.deepEqual(
    [...report.keys()],
    [".github/tools/src/sample.ts", ".github/tools/src/cli/main.ts"],
  );
});

test("duplicate inputs sum line and branch hits without duplicate files", () => {
  const options = {
    root,
    workspace: join(root, ".github", "tools"),
    label: "tools-run.lcov",
  };
  const report = mergeLcov([
    parseLcov(readFileSync(fixture("tools-run.lcov"), "utf8"), options),
    parseLcov(readFileSync(fixture("second-run.lcov"), "utf8"), options),
  ]);
  assert.equal(report.size, 3);
  const main = report.get(".github/tools/src/cli/main.ts")!;
  assert.deepEqual(
    [...main.lines],
    [
      [10, 5],
      [11, 0],
    ],
  );
  assert.deepEqual(
    [...main.branches],
    [
      ["10,0,0", 3],
      ["10,0,1", 0],
    ],
  );
  assert.ok(
    ![...report.keys()].some((name) =>
      /test|testing|node_modules|outside|evals/.test(name),
    ),
  );
});

test("lcov command writes the pinned Cobertura XML", () => {
  const temp = mkdtempSync(join(tmpdir(), "agentics-lcov-"));
  try {
    const output = join(temp, "cobertura.xml");
    const result = spawnSync(
      process.execPath,
      [
        resolve(root, ".github/tools/src/coverage/cli.ts"),
        "lcov",
        "--root",
        root,
        "--input",
        `.github/tools:${fixture("tools-run.lcov")}`,
        "--input",
        `.github/tools:${fixture("second-run.lcov")}`,
        "--out",
        output,
      ],
      { encoding: "utf8", cwd: root },
    );
    assert.equal(result.status, 0, result.stderr);
    assert.deepEqual(
      readFileSync(output),
      readFileSync(fixture("merged.expected.xml")),
    );
  } finally {
    rmSync(temp, { recursive: true, force: true });
  }
});

test(
  "Windows command accepts absolute workspace and input drive paths",
  { skip: process.platform !== "win32" },
  () => {
    const temp = mkdtempSync(join(tmpdir(), "agentics-lcov-windows-"));
    try {
      const output = join(temp, "out.xml");
      const result = spawnSync(
        process.execPath,
        [
          resolve(root, ".github/tools/src/coverage/cli.ts"),
          "lcov",
          "--root",
          "C:\\repo",
          "--input",
          `C:\\repo\\.github\\tools:${fixture("windows.lcov")}`,
          "--out",
          output,
        ],
        { encoding: "utf8" },
      );
      assert.equal(result.status, 0, result.stderr);
      const xml = readFileSync(output, "utf8");
      assert.match(xml, /filename="\.github\/tools\/src\/sample.ts"/);
      assert.match(xml, /filename="\.github\/tools\/src\/cli\/main.ts"/);
      assert.doesNotMatch(xml, /filename="[^"\n]*(?:\\|\.\.|[A-Za-z]:)/);
    } finally {
      rmSync(temp, { recursive: true, force: true });
    }
  },
);

test("missing SF input exits one and names the offending file", () => {
  const temp = mkdtempSync(join(tmpdir(), "agentics-lcov-invalid-"));
  try {
    const input = join(temp, "invalid.info");
    writeFileSync(input, "TN:empty\nLF:0\nend_of_record\n");
    const result = spawnSync(
      process.execPath,
      [
        resolve(root, ".github/tools/src/coverage/cli.ts"),
        "lcov",
        "--root",
        root,
        "--input",
        `.github/tools:${input}`,
        "--out",
        join(temp, "out.xml"),
      ],
      { encoding: "utf8" },
    );
    assert.equal(result.status, 1);
    assert.match(result.stderr, /no SF: record/);
    assert.ok(result.stderr.includes(input));
  } finally {
    rmSync(temp, { recursive: true, force: true });
  }
});
