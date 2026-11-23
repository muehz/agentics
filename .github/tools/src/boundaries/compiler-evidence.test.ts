import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import {
  cpSync,
  mkdirSync,
  mkdtempSync,
  rmSync,
  symlinkSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { test, type TestContext } from "node:test";
import { fileURLToPath, pathToFileURL } from "node:url";
import { defaultTsc as cliTsc, run, type TscRunner } from "./cli.ts";
import { collectCompilerEvidence, defaultTsc } from "./compiler-evidence.ts";

const areaDir = ".github/tools";
const areaFile = `${areaDir}/src/input.ts`;
const areaArgs = [
  "--noEmit",
  "--explainFiles",
  "-p",
  `${areaDir}/tsconfig.json`,
];
const skillDir = "skills/demo";
const skillFile = `${skillDir}/scripts/input.cjs`;
const skillArgs = [
  "--noEmit",
  "--allowJs",
  "--checkJs",
  "--resolveJsonModule",
  "--explainFiles",
  "--module",
  "nodenext",
  "--moduleResolution",
  "nodenext",
  "--skipLibCheck",
  "--allowImportingTsExtensions",
  "--types",
  "node",
  skillFile,
];

function write(root: string, relative: string, text: string) {
  const target = path.join(root, relative);
  mkdirSync(path.dirname(target), { recursive: true });
  writeFileSync(target, text);
}

function fixture(t: TestContext): string {
  const root = mkdtempSync(path.join(tmpdir(), "agentics compiler evidence-"));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  write(root, "package.json", JSON.stringify({ workspaces: [areaDir] }));
  write(root, `${areaDir}/package.json`, "{}");
  write(root, areaFile, "export {};\n");
  write(root, skillFile, "module.exports = 1;\n");
  write(
    root,
    "CODEOWNERS",
    ["/hooks/", "/evals/", `/${areaDir}/`, "/skills/"]
      .map((ownerPath) => `${ownerPath} @owner\n`)
      .join(""),
  );
  return root;
}

const areaViolation = `FAIL area-ts-import: ${areaFile}: imports "agentics-evals", which resolves to evals/src/index.ts outside tools`;
const skillViolation = `FAIL area-skill-import: ${skillFile}: imports "uninstalled", which does not resolve to an installed package; a skill script may import only its own files and Node built-ins`;

function installCompiler(root: string, exitCode: number) {
  // Both separator styles occur in compiler output. Use absolute paths to check
  // that edges and diagnostics are normalized as well as listed files.
  const abs = (file: string) => path.join(root, file).replaceAll("/", "\\");
  const notFound = (file: string, specifier: string) =>
    `${abs(file)}(1,19): error TS2307: Cannot find module '${specifier}' or its corresponding type declarations.\n`;
  const invocations = [
    {
      args: areaArgs,
      stdout: [
        abs(areaFile),
        abs("evals/src/index.ts"),
        `  Imported via "agentics-evals" from file '${abs(areaFile)}'`,
        "",
      ].join("\r\n"),
      stderr: notFound(areaFile, "area-uninstalled"),
    },
    {
      args: skillArgs,
      stdout: `${abs(skillFile)}\n`,
      stderr:
        notFound(skillFile, "uninstalled") +
        notFound(skillFile, "node:fs") +
        notFound(skillFile, "./gone.cjs"),
    },
  ];
  write(
    root,
    "node_modules/typescript/bin/tsc",
    `const calls = ${JSON.stringify(invocations)};
const actual = JSON.stringify(process.argv.slice(2));
const call = calls.find(({ args }) => JSON.stringify(args) === actual);
if (!call) throw new Error("unexpected compiler arguments: " + actual);
process.stdout.write(call.stdout);
process.stderr.write(call.stderr);
process.exitCode = ${exitCode};
`,
  );
}

const cli = fileURLToPath(new URL("./cli.ts", import.meta.url));

function runCli(root: string, entrypoint = cli) {
  const child = spawnSync(process.execPath, [entrypoint, "--repo", root], {
    encoding: "utf8",
  });
  assert.ifError(child.error);
  assert.equal(child.signal, null);
  return { code: child.status, stdout: child.stdout, stderr: child.stderr };
}

for (const exitCode of [0, 23]) {
  test(`both audits use compiler listings with child exit ${exitCode}`, (t) => {
    const root = fixture(t);
    installCompiler(root, exitCode);
    const lines = [skillViolation, areaViolation, "boundaries: 2 failures"];
    assert.deepEqual(run({ root }), {
      lines,
      code: 1,
    });
    assert.deepEqual(runCli(root), {
      stdout: [...lines, ""].join("\n"),
      stderr: "",
      code: 1,
    });
    assert.deepEqual(
      collectCompilerEvidence({ root, args: areaArgs, directory: areaDir }),
      {
        ok: true,
        files: [
          { file: areaFile, reasons: [] },
          {
            file: "evals/src/index.ts",
            reasons: [{ specifier: "agentics-evals", importer: areaFile }],
          },
        ],
        unresolved: [],
      },
    );
    assert.deepEqual(
      collectCompilerEvidence({
        root,
        args: skillArgs,
        directory: skillDir,
        includeUnresolved: true,
      }),
      {
        ok: true,
        files: [{ file: skillFile, reasons: [] }],
        unresolved: [
          { specifier: "uninstalled", importer: skillFile },
          { specifier: "node:fs", importer: skillFile },
        ],
      },
    );
  });
}

test("the boundaries CLI runs through an alias and imports without side effects", (t) => {
  const root = fixture(t);
  installCompiler(root, 23);
  const directory = path.join(root, "boundary-alias");
  symlinkSync(path.dirname(cli), directory, "junction");
  const aliasedCli = path.join(directory, path.basename(cli));
  assert.deepEqual(runCli(root, aliasedCli), {
    code: 1,
    stdout: [skillViolation, areaViolation, "boundaries: 2 failures", ""].join(
      "\n",
    ),
    stderr: "",
  });

  const imported = spawnSync(
    process.execPath,
    [
      "--input-type=module",
      "-e",
      "await import(process.argv[1]);",
      pathToFileURL(aliasedCli).href,
    ],
    { cwd: root, encoding: "utf8" },
  );
  assert.ifError(imported.error);
  assert.equal(imported.status, 0, imported.stderr);
  assert.equal(imported.stdout, "");
  assert.equal(imported.stderr, "");
});

test("both import audits skip when the compiler is absent", (t) => {
  const root = fixture(t);
  assert.deepEqual(
    run({ root, tsc: () => assert.fail("compiler ran while absent") }),
    {
      lines: [
        "SKIP area-ts-import: TypeScript is not installed",
        "SKIP area-skill-import: TypeScript is not installed",
        "boundaries: 0 failures",
      ],
      code: 0,
    },
  );
});

const areaError =
  "ERROR area-ts-import: tools: tsc did not list any file of tools";
const skillError = `ERROR area-skill-import: demo: tsc did not list any file of ${skillDir}`;

test("the CLI takes both audits' evidence from the shared collector", (t) => {
  const root = fixture(t);
  installCompiler(root, 0);
  const source = path.dirname(cli);
  const checkout = path.join(root, "boundary-cli");
  const copy = path.join(checkout, "boundaries");
  cpSync(source, copy, {
    recursive: true,
    filter: (file) => !file.endsWith(".test.ts"),
  });
  cpSync(
    path.join(source, "..", "repository-areas.ts"),
    path.join(checkout, "repository-areas.ts"),
  );
  write(checkout, "package.json", '{"type":"module"}\n');
  write(
    copy,
    "compiler-evidence.ts",
    `export function defaultTsc() {
  throw new Error("the fixture collector supplies the evidence");
}
export function collectCompilerEvidence({ directory, includeUnresolved = false }) {
  return {
    ok: false,
    detail: "shared evidence for " + directory + " (unresolved=" + includeUnresolved + ")",
  };
}
`,
  );
  assert.deepEqual(runCli(root, path.join(copy, "cli.ts")), {
    stdout: [
      `${areaError}: shared evidence for ${areaDir} (unresolved=false)`,
      `${skillError}: shared evidence for ${skillDir} (unresolved=true)`,
      "boundaries: 0 failures",
      "",
    ].join("\n"),
    stderr: "",
    code: 2,
  });
});

test("a process error overrides even an apparently complete listing", (t) => {
  const root = fixture(t);
  const tsc: TscRunner = (args, cwd) => {
    assert.equal(cwd, root);
    const isArea = args.includes("-p");
    assert.deepEqual(args, isArea ? areaArgs : skillArgs);
    return {
      stdout: `${isArea ? areaFile : skillFile}\n`,
      error: "spawn failed",
    };
  };
  assert.deepEqual(run({ root, tsc, hasTsc: true }), {
    lines: [
      `${areaError}: spawn failed`,
      `${skillError}: spawn failed`,
      "boundaries: 0 failures",
    ],
    code: 2,
  });
  for (const [directory, args] of [
    [areaDir, areaArgs],
    [skillDir, skillArgs],
  ] as const) {
    assert.deepEqual(collectCompilerEvidence({ root, args, directory, tsc }), {
      ok: false,
      detail: "spawn failed",
    });
  }
});

test("the default runner retains the native spawn diagnostic", (t) => {
  const root = path.join(fixture(t), "absent working directory");
  const child = spawnSync(process.execPath, [], {
    cwd: root,
    encoding: "utf8",
  });
  assert.ok(child.error);
  assert.equal(cliTsc, defaultTsc);
  assert.deepEqual(
    collectCompilerEvidence({ root, args: areaArgs, directory: areaDir }),
    { ok: false, detail: child.error.message },
  );
});

test("incomplete listings show the final five lines and fail both audits", (t) => {
  const root = fixture(t);
  const sibling = (dir: string) => `${dir}-archive/input.ts`;
  const stdout = (dir: string) =>
    [
      "first line excluded",
      "compiler output",
      "node_modules/another-area.ts",
      "stack frame",
      sibling(dir),
      "last line",
      "",
    ].join("\n");
  const detail = (dir: string) =>
    `compiler output | node_modules/another-area.ts | stack frame | ${sibling(dir)} | last line`;
  const tsc: TscRunner = (args) => ({
    stdout: stdout(args.includes("-p") ? areaDir : skillDir),
    error: null,
  });
  assert.deepEqual(run({ root, tsc, hasTsc: true }), {
    lines: [
      `${areaError}: ${detail(areaDir)}`,
      `${skillError}: ${detail(skillDir)}`,
      "boundaries: 0 failures",
    ],
    code: 2,
  });
  for (const [directory, args] of [
    [areaDir, areaArgs],
    [skillDir, skillArgs],
  ] as const) {
    assert.deepEqual(collectCompilerEvidence({ root, args, directory, tsc }), {
      ok: false,
      detail: detail(directory),
    });
  }
});
