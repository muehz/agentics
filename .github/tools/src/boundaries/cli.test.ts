import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import {
  existsSync,
  mkdirSync,
  mkdtempSync,
  rmSync,
  symlinkSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { test } from "node:test";
import { fileURLToPath } from "node:url";
import {
  runValidate,
  VALIDATE_STAGES,
  type Command,
  type CommandResult,
} from "../plugin/validate.ts";
import { run, type TscRunner } from "./cli.ts";

const REPO = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "../../../..",
);
const CODEOWNERS = [
  "* @o",
  "/hooks/ @o",
  "/evals/ @o",
  "/.github/tools/ @o",
  "/skills/ @o",
  "",
].join("\n");

function write(root: string, rel: string, text: string) {
  const file = path.join(root, rel);
  mkdirSync(path.dirname(file), { recursive: true });
  writeFileSync(file, text);
}

function workspace(extra: Record<string, string> = {}) {
  const root = mkdtempSync(path.join(tmpdir(), "agentics-boundaries-"));
  write(
    root,
    "package.json",
    JSON.stringify({ workspaces: [".github/tools"] }),
  );
  // evals is not a workspace; it gets a package so a fixture can cross into it by package name.
  for (const [area, dir] of [
    ["tools", ".github/tools"],
    ["evals", "evals"],
  ] as const) {
    write(
      root,
      `${dir}/package.json`,
      JSON.stringify({
        name: `agentics-${area}`,
        type: "module",
        exports: { ".": "./src/index.ts" },
      }),
    );
    write(root, `${dir}/src/index.ts`, `export const ${area} = 1;\n`);
    write(
      root,
      `${dir}/tsconfig.json`,
      JSON.stringify({
        compilerOptions: {
          target: "ES2023",
          lib: ["ES2023"],
          module: "NodeNext",
          moduleResolution: "NodeNext",
          types: ["node"],
          strict: true,
          noEmit: true,
          allowImportingTsExtensions: true,
          verbatimModuleSyntax: true,
          skipLibCheck: true,
        },
        include: ["src/**/*.ts"],
      }),
    );
  }
  write(root, "CODEOWNERS", CODEOWNERS);
  for (const [rel, text] of Object.entries(extra)) write(root, rel, text);
  return root;
}

const noTsc: TscRunner = () => ({ stdout: "", error: "tsc must not run" });

test("without TypeScript the tsc rules skip and the file rules still run", () => {
  const root = workspace({ CODEOWNERS: "* @o\n" });
  const report = run({ root, hasTsc: false, tsc: noTsc });
  assert.ok(
    report.lines.includes("SKIP area-ts-import: TypeScript is not installed"),
  );
  assert.ok(
    report.lines.includes(
      "SKIP area-skill-import: TypeScript is not installed",
    ),
  );
  assert.ok(
    report.lines.some((l) =>
      l.startsWith(
        "FAIL area-codeowners: CODEOWNERS: no owner entry for /hooks/",
      ),
    ),
  );
  assert.equal(report.code, 1);
});

test("a tsc run that prints no files is could-not-run, never a pass", () => {
  const root = workspace();
  const report = run({
    root,
    hasTsc: true,
    tsc: () => ({ stdout: "", error: null }),
  });
  assert.equal(report.code, 2);
  assert.ok(
    report.lines.some((l) => l.startsWith("ERROR area-ts-import: tools:")),
  );
});

test("crashed tsc or node output that parses as file entries is still could-not-run", () => {
  const root = workspace();
  const crashed =
    "node:internal/modules/cjs/loader:1\nError: Cannot find module '/x/node_modules/typescript/bin/tsc'\n    at Module._resolveFilename (node:internal/modules/cjs/loader:1)\n";
  const report = run({
    root,
    hasTsc: true,
    tsc: () => ({ stdout: crashed, error: null }),
  });
  assert.equal(report.code, 2);
  assert.ok(
    report.lines.some((l) =>
      l.startsWith(
        "ERROR area-ts-import: tools: tsc did not list any file of tools",
      ),
    ),
  );
});

/** A minimal healthy listing for the area that args name, so a canned run for one area leaves the others passing. */
const healthy = (args: string[]) =>
  `${path.posix.dirname(args[args.indexOf("-p") + 1]!)}/src/index.ts\n`;

test("only the canonical tools workspace runs the area TypeScript audit", (t) => {
  const root = workspace({
    "tools/package.json": "{}",
    "tools/tsconfig.json": "{}",
    "tools/src/index.ts": "export {};\n",
  });
  t.after(() => rmSync(root, { recursive: true, force: true }));
  const calls: string[][] = [];
  const report = run({
    root,
    hasTsc: true,
    tsc: (args, cwd) => {
      assert.equal(cwd, root);
      calls.push(args);
      return { stdout: healthy(args), error: null };
    },
  });
  assert.deepEqual(calls, [
    ["--noEmit", "--explainFiles", "-p", ".github/tools/tsconfig.json"],
  ]);
  assert.deepEqual(report, { lines: ["boundaries: 0 failures"], code: 0 });
});

test("a canned tsc listing with a crossing edge fails with the importer and specifier", () => {
  const root = workspace();
  const tsc: TscRunner = (args) => ({
    stdout: args.includes(".github/tools/tsconfig.json")
      ? ".github/tools/src/a.ts\nevals/src/index.ts\n   Imported via \"agentics-evals\" from file '.github/tools/src/a.ts'\n"
      : healthy(args),
    error: null,
  });
  const report = run({ root, hasTsc: true, tsc });
  assert.deepEqual(
    report.lines.filter((l) => l.startsWith("FAIL")),
    [
      'FAIL area-ts-import: .github/tools/src/a.ts: imports "agentics-evals", which resolves to evals/src/index.ts outside tools',
    ],
  );
  assert.equal(report.code, 1);
  assert.equal(report.lines.at(-1), "boundaries: 1 failures");
});

// The integration test runs the real compiler. It needs the repository's node_modules, so it skips without them.
const realTsc = path.join(REPO, "node_modules", "typescript", "bin", "tsc");
test(
  "with the real tsc, relative, bare, and dynamic crossings and a skill require all fail",
  { skip: !existsSync(realTsc) && "run npm ci --ignore-scripts first" },
  () => {
    const root = workspace({
      ".github/tools/src/rel.ts":
        'import { evals } from "../../../evals/src/index.ts";\nexport const a = evals;\n',
      ".github/tools/src/pkg.ts":
        'import { evals } from "agentics-evals";\nexport const b = evals;\n',
      ".github/tools/src/dyn.ts":
        'export const c = await import("../../../evals/src/index.ts");\n',
      "skills/demo/scripts/s.cjs":
        'const y = require("yaml");\nmodule.exports = y;\n',
      "skills/demo/scripts/u.cjs":
        'const a = require("axios");\nmodule.exports = a;\n',
    });
    mkdirSync(path.join(root, "node_modules", "@types"), { recursive: true });
    for (const name of ["typescript", "yaml"]) {
      const source = path.join(REPO, "node_modules", name);
      if (existsSync(source))
        symlinkSync(source, path.join(root, "node_modules", name), "junction");
    }
    symlinkSync(
      path.join(REPO, "node_modules", "@types", "node"),
      path.join(root, "node_modules", "@types", "node"),
      "junction",
    );
    for (const [area, dir] of [
      ["tools", ".github/tools"],
      ["evals", "evals"],
    ] as const) {
      symlinkSync(
        path.join(root, dir),
        path.join(root, "node_modules", `agentics-${area}`),
        "junction",
      );
    }
    const report = run({ root });
    const fails = report.lines.filter((l) => l.startsWith("FAIL"));
    assert.ok(
      fails.some(
        (l) =>
          l.includes(".github/tools/src/rel.ts") &&
          l.includes("area-ts-import"),
      ),
      fails.join("\n"),
    );
    assert.ok(
      fails.some(
        (l) =>
          l.includes(".github/tools/src/pkg.ts") &&
          l.includes('"agentics-evals"'),
      ),
      fails.join("\n"),
    );
    assert.ok(
      fails.some((l) => l.includes(".github/tools/src/dyn.ts")),
      fails.join("\n"),
    );
    assert.ok(
      fails.some((l) =>
        l.startsWith("FAIL area-skill-import: skills/demo/scripts/s.cjs"),
      ),
      fails.join("\n"),
    );
    assert.ok(
      fails.some(
        (l) =>
          l.startsWith("FAIL area-skill-import: skills/demo/scripts/u.cjs") &&
          l.includes('"axios"'),
      ),
      fails.join("\n"),
    );
    assert.equal(report.code, 1);
  },
);

test(
  "with the real tsc, a skill script that requires a JSON file in another area fails",
  { skip: !existsSync(realTsc) && "run npm ci --ignore-scripts first" },
  () => {
    const root = workspace({
      "skills/demo/scripts/j.cjs":
        'const p = require("../../../.github/tools/package.json");\nmodule.exports = p;\n',
      "skills/demo/scripts/j.mjs":
        'import p from "../../../.github/tools/package.json" with { type: "json" };\nexport default p;\n',
    });
    mkdirSync(path.join(root, "node_modules", "@types"), { recursive: true });
    symlinkSync(
      path.join(REPO, "node_modules", "typescript"),
      path.join(root, "node_modules", "typescript"),
      "junction",
    );
    symlinkSync(
      path.join(REPO, "node_modules", "@types", "node"),
      path.join(root, "node_modules", "@types", "node"),
      "junction",
    );
    const report = run({ root });
    const fails = report.lines.filter((l) => l.startsWith("FAIL"));
    assert.ok(
      fails.some(
        (l) =>
          l.startsWith("FAIL area-skill-import: skills/demo/scripts/j.cjs") &&
          l.includes(".github/tools/package.json"),
      ),
      `a required JSON file in another area is a dependency too:\n${fails.join("\n")}`,
    );
    assert.ok(
      fails.some(
        (l) =>
          l.startsWith("FAIL area-skill-import: skills/demo/scripts/j.mjs") &&
          l.includes(".github/tools/package.json"),
      ),
      `an imported JSON file in another area is a dependency too:\n${fails.join("\n")}`,
    );
    assert.equal(report.code, 1);
  },
);

test("a skill import the compiler reports as unresolved fails, and a missing relative file does not", () => {
  const root = workspace({
    "skills/demo/scripts/s.cjs": "module.exports = 1;\n",
  });
  const tsc: TscRunner = (args) => ({
    stdout: args.includes("--allowJs")
      ? [
          "skills/demo/scripts/s.cjs",
          "skills/demo/scripts/s.cjs(1,19): error TS2307: Cannot find module 'axios' or its corresponding type declarations.",
          "skills/demo/scripts/s.cjs(2,19): error TS2307: Cannot find module './gone.cjs' or its corresponding type declarations.",
          "",
        ].join("\n")
      : healthy(args),
    error: null,
  });
  const report = run({ root, hasTsc: true, tsc });
  const fails = report.lines.filter((l) => l.startsWith("FAIL"));
  assert.equal(fails.length, 1, fails.join("\n"));
  assert.match(
    fails[0]!,
    /area-skill-import: skills\/demo\/scripts\/s\.cjs.*"axios"/,
  );
  assert.equal(report.code, 1);
});

const isBoundariesCall = (c: Command) =>
  c.tool === "node" &&
  c.args[0]?.replaceAll("\\", "/").endsWith("boundaries/cli.ts") === true;
const isToolsTestsCall = (c: Command) =>
  c.tool === "npm" &&
  c.args.at(-1) === "test" &&
  c.args.at(-2)?.replaceAll("\\", "/").endsWith(".github/tools") === true;

function validatePath(withTools: boolean, boundariesOutput = "") {
  const root = mkdtempSync(path.join(tmpdir(), "agentics-validate-path-"));
  try {
    if (withTools) write(root, ".github/tools/package.json", "{}");
    const calls: Command[] = [];
    let stdout = "";
    const code = runValidate({
      mode: "strict",
      speed: "full",
      repoRoot: root,
      toolCacheDir: path.join(root, "cache"),
      nodeVersion: "26.12.1",
      enginesNode: "^22.23.5 || ^24.22.3 || >=26.12.1",
      env: {},
      runner: (command): CommandResult => {
        calls.push(command);
        return {
          found: true,
          status: 0,
          output: isBoundariesCall(command) ? boundariesOutput : "",
        };
      },
      write: (stream, text) => {
        if (stream === "stdout") stdout += text;
      },
    });
    return { code, calls, stdout };
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
}

test("validate path: the boundaries run after the tools tests", () => {
  const keys = VALIDATE_STAGES.map((stage) => stage.key);
  assert.ok(keys.indexOf("boundaries") > keys.indexOf("tools_tests"));
  const { code, calls } = validatePath(true);
  assert.equal(code, 0);
  const tools = calls.findIndex(isToolsTestsCall);
  const boundaries = calls.findIndex(isBoundariesCall);
  assert.ok(tools >= 0 && boundaries > tools, JSON.stringify(calls));
});

test("validate path: the boundaries run only with the tools package", () => {
  const { code, calls } = validatePath(false);
  assert.equal(code, 0);
  assert.equal(calls.some(isBoundariesCall), false);
});

test("validate path: a skipped rule is not reported as held", () => {
  const { stdout } = validatePath(
    true,
    "SKIP area-skill-import: no tsc\nboundaries: 0 failures\n",
  );
  const skip = stdout.indexOf("SKIP area-skill-import: no tsc");
  const note = stdout.indexOf(
    "ℹ Some area boundary rules were skipped (see the SKIP lines above). Run npm ci --ignore-scripts to enable them.",
  );
  assert.ok(skip >= 0 && note > skip, stdout);
  assert.doesNotMatch(stdout, /✅ Area boundaries hold\./);
});

test("validate path: the closing line follows the CLI's output", () => {
  const { stdout, calls } = validatePath(true, "boundaries: 0 failures\n");
  const output = stdout.indexOf("boundaries: 0 failures");
  const closing = stdout.indexOf("✅ Area boundaries hold.");
  assert.ok(output >= 0 && closing > output, stdout);
  assert.equal(calls.findLastIndex(isBoundariesCall), calls.length - 1);
});

test(
  "the real repository passes",
  { skip: !existsSync(realTsc) && "run npm ci --ignore-scripts first" },
  () => {
    const result = spawnSync(
      process.execPath,
      [path.join(REPO, ".github/tools/src/boundaries/cli.ts"), "--repo", REPO],
      { encoding: "utf8" },
    );
    assert.equal(result.status, 0, result.stdout + result.stderr);
    assert.match(result.stdout, /boundaries: 0 failures/);
  },
);

test("a could-not-run error outranks a failure for the exit code", () => {
  const root = workspace({
    "skills/demo/scripts/s.cjs": "module.exports = 1;\n",
  });
  const tsc: TscRunner = (args) => ({
    stdout: args.includes(".github/tools/tsconfig.json")
      ? ".github/tools/src/a.ts\nevals/src/index.ts\n   Imported via \"agentics-evals\" from file '.github/tools/src/a.ts'\n"
      : "",
    error: null,
  });
  const report = run({ root, hasTsc: true, tsc });
  assert.ok(report.lines.some((l) => l.startsWith("FAIL area-ts-import")));
  assert.ok(
    report.lines.some((l) => l.startsWith("ERROR area-skill-import: demo:")),
  );
  assert.equal(report.code, 2);
});

test("a skill whose tsc run lists none of its files is could-not-run", () => {
  const root = workspace({
    "skills/demo/scripts/s.cjs": "module.exports = 1;\n",
  });
  const tsc: TscRunner = (args) => ({
    stdout: args.includes("--allowJs")
      ? ".github/tools/src/index.ts\n"
      : healthy(args),
    error: null,
  });
  const report = run({ root, hasTsc: true, tsc });
  assert.ok(
    report.lines.some((l) => l.startsWith("ERROR area-skill-import: demo:")),
  );
  assert.equal(report.code, 2);
});
