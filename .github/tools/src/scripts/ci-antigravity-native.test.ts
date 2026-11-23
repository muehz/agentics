import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import {
  AGY_ARCHIVE_MEMBER,
  type CommandResult,
  type CommandRun,
  PINNED_AGY_URL,
  main,
  validateReport,
} from "./ci-antigravity-native.ts";

const MODULE = path.join(import.meta.dirname, "ci-antigravity-native.ts");
const BINARY = Buffer.from("not really agy");
const ARCHIVE = Buffer.from("not really a tarball");
const SHA512 = createHash("sha512").update(ARCHIVE).digest("hex");

function tempDir(t: test.TestContext): string {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "agentics-agy-native-"));
  t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
  return dir;
}

interface Call {
  command: string;
  args: readonly string[];
  cwd: string | undefined;
}

interface Harness {
  temp: string;
  githubPath: string;
  env: Record<string, string>;
  calls: Call[];
  errors: string[];
  workDirs: string[];
  run: CommandRun;
}

function harness(
  t: test.TestContext,
  options: {
    url?: string;
    sha?: string;
    curlStatus?: number;
    curlError?: Error;
    tarStatus?: number;
    tarError?: Error;
  } = {},
): Harness {
  const temp = tempDir(t);
  const githubPath = path.join(temp, "github-path");
  const calls: Call[] = [];
  const workDirs: string[] = [];
  return {
    temp,
    githubPath,
    env: {
      AGY_URL: options.url ?? PINNED_AGY_URL,
      AGY_SHA512: options.sha ?? SHA512,
      RUNNER_TEMP: temp,
      GITHUB_PATH: githubPath,
    },
    calls,
    errors: [],
    workDirs,
    run: (command, args, runOptions) => {
      calls.push({ command, args, cwd: runOptions.cwd });
      if (runOptions.cwd && !workDirs.includes(runOptions.cwd)) {
        workDirs.push(runOptions.cwd);
      }
      if (command === "curl") {
        if (options.curlError) {
          return { status: null, stdout: "", error: options.curlError };
        }
        if (options.curlStatus !== undefined) {
          return { status: options.curlStatus, stdout: "" };
        }
        const outIndex = args.indexOf("-o");
        const outName = args[outIndex + 1]!;
        fs.writeFileSync(path.join(runOptions.cwd!, outName), ARCHIVE);
        return { status: 0, stdout: "" };
      }
      if (command === "tar") {
        if (options.tarError) {
          return { status: null, stdout: "", error: options.tarError };
        }
        if (options.tarStatus !== undefined) {
          return { status: options.tarStatus, stdout: "" };
        }
        const destIndex = args.indexOf("-C");
        const dest = args[destIndex + 1]!;
        const member = args[destIndex + 2]!;
        const extracted = path.join(runOptions.cwd!, dest, member);
        fs.mkdirSync(path.dirname(extracted), { recursive: true });
        fs.writeFileSync(extracted, BINARY);
        return { status: 0, stdout: "" };
      }
      return { status: 0, stdout: "" };
    },
  };
}

const installed = (h: Harness) => path.join(h.temp, "bin", "agy");

test("install downloads the pinned archive, verifies SHA-512, extracts antigravity, cleans up and extends PATH", (t) => {
  const h = harness(t);
  const status = main(["install"], {
    env: h.env,
    run: h.run,
    report: (line) => h.errors.push(line),
  });
  assert.equal(status, 0, h.errors.join("\n"));
  assert.equal(h.calls.length, 2);
  assert.equal(h.calls[0]!.command, "curl");
  assert.deepEqual(h.calls[0]!.args, [
    "-fsSL",
    "-o",
    "agy.tar.gz",
    PINNED_AGY_URL,
  ]);
  assert.equal(h.calls[1]!.command, "tar");
  assert.deepEqual(h.calls[1]!.args, [
    "-xzf",
    "agy.tar.gz",
    "-C",
    "extract",
    AGY_ARCHIVE_MEMBER,
  ]);
  assert.equal(h.calls[0]!.cwd, h.calls[1]!.cwd);
  assert.deepEqual(fs.readFileSync(installed(h)), BINARY);
  assert.equal(fs.existsSync(h.workDirs[0]!), false);
  assert.equal(
    fs.readFileSync(h.githubPath, "utf8"),
    `${path.join(h.temp, "bin")}\n`,
  );
});

test("install sets mode 0755", (t) => {
  const h = harness(t);
  main(["install"], { env: h.env, run: h.run });
  if (process.platform === "win32") {
    assert.ok(fs.existsSync(installed(h)));
    return;
  }
  assert.equal(fs.statSync(installed(h)).mode & 0o777, 0o755);
});

test("install accepts an upper-case SHA-512 checksum", (t) => {
  const h = harness(t, { sha: SHA512.toUpperCase() });
  assert.equal(main(["install"], { env: h.env, run: h.run }), 0);
});

test("install rejects an unpinned AGY_URL before running curl or tar", (t) => {
  const h = harness(t, { url: "https://example.invalid/agy.tar.gz" });
  const status = main(["install"], {
    env: h.env,
    run: h.run,
    report: (line) => h.errors.push(line),
  });
  assert.notEqual(status, 0);
  assert.deepEqual(h.calls, []);
  assert.equal(fs.existsSync(installed(h)), false);
  assert.match(h.errors.join("\n"), /^ci-antigravity-native: AGY_URL/);
});

test("a SHA-512 checksum mismatch extracts and installs nothing", (t) => {
  const h = harness(t, { sha: "0".repeat(128) });
  const status = main(["install"], {
    env: h.env,
    run: h.run,
    report: (line) => h.errors.push(line),
  });
  assert.notEqual(status, 0);
  assert.equal(h.calls.length, 1);
  assert.equal(h.calls[0]!.command, "curl");
  assert.equal(fs.existsSync(h.workDirs[0]!), false);
  assert.equal(fs.existsSync(installed(h)), false);
  assert.equal(fs.existsSync(h.githubPath), false);
  assert.match(h.errors.join("\n"), /^ci-antigravity-native: .*checksum/);
});

test("a failed download extracts and installs nothing", (t) => {
  const h = harness(t, { curlStatus: 22 });
  const status = main(["install"], {
    env: h.env,
    run: h.run,
    report: (line) => h.errors.push(line),
  });
  assert.equal(status, 22);
  assert.equal(h.calls.length, 1);
  assert.equal(fs.existsSync(h.workDirs[0]!), false);
  assert.equal(fs.existsSync(installed(h)), false);
  assert.equal(fs.existsSync(h.githubPath), false);
});

test("a failing tar installs nothing, cleans up, and returns its status", (t) => {
  const h = harness(t, { tarStatus: 2 });
  const status = main(["install"], {
    env: h.env,
    run: h.run,
  });
  assert.equal(status, 2);
  assert.equal(fs.existsSync(installed(h)), false);
  assert.equal(fs.existsSync(h.githubPath), false);
  assert.equal(fs.existsSync(h.workDirs[0]!), false);
});

test("a tar that cannot start is reported and exits nonzero", (t) => {
  const h = harness(t, { tarError: new Error("spawn tar ENOENT") });
  const errors: string[] = [];
  const status = main(["install"], {
    env: h.env,
    run: h.run,
    report: (line) => errors.push(line),
  });
  assert.notEqual(status, 0);
  assert.match(errors.join("\n"), /^ci-antigravity-native: cannot start tar/);
});

test("install without its pinned inputs exits nonzero and downloads nothing", (t) => {
  for (const missing of [
    "AGY_URL",
    "AGY_SHA512",
    "RUNNER_TEMP",
    "GITHUB_PATH",
  ]) {
    const h = harness(t);
    delete h.env[missing];
    const status = main(["install"], {
      env: h.env,
      run: h.run,
      report: (line) => h.errors.push(line),
    });
    assert.notEqual(status, 0, missing);
    assert.deepEqual(h.calls, [], missing);
    assert.match(
      h.errors.join("\n"),
      new RegExp(`^ci-antigravity-native: ${missing}`),
    );
  }
});

function checkFakes(result: Partial<CommandResult>) {
  const calls: Call[] = [];
  const run: CommandRun = (command, args, options) => {
    calls.push({ command, args, cwd: options.cwd });
    return { status: 0, stdout: "", ...result };
  };
  return { calls, run };
}

const VALID_PAYLOAD = {
  components: { skills: 26 },
  findings: [
    {
      severity: "warning",
      file: "AGENTS.md",
      message: "never loaded: plugins load rules only from rules/.",
    },
    {
      severity: "note",
      file: "README.md",
      message: "informational note",
    },
  ],
};

test("check runs agy plugin validate --json . and accepts allowed findings", async () => {
  const f = checkFakes({ stdout: JSON.stringify(VALID_PAYLOAD) });
  const out: string[] = [];
  const errors: string[] = [];
  const status = await main(["check"], {
    env: {},
    run: f.run,
    write: (line) => out.push(line),
    report: (line) => errors.push(line),
  });
  assert.equal(status, 0, errors.join("\n"));
  assert.deepEqual(
    f.calls.map((c) => [c.command, ...c.args]),
    [["agy", "plugin", "validate", "--json", "."]],
  );
  assert.deepEqual(out, [
    "Antigravity plugin valid: 26 skills, 2 allowed finding(s).",
  ]);
  assert.deepEqual(errors, []);
});

test("validateReport rejects errors, unexpected warnings, missing skills, and malformed payloads", () => {
  assert.equal(validateReport(VALID_PAYLOAD).ok, true);
  const failCases: [string, unknown][] = [
    [
      "severity error",
      {
        components: { skills: 26 },
        findings: [
          {
            severity: "error",
            file: ".antigravity-plugin/plugin.json",
            message: "invalid manifest",
          },
        ],
      },
    ],
    [
      "unexpected warning on README.md",
      {
        components: { skills: 26 },
        findings: [
          {
            severity: "warning",
            file: "README.md",
            message: "unexpected warning",
          },
        ],
      },
    ],
    [
      "oversized SKILL.md body truncation warning",
      {
        components: { skills: 26 },
        findings: [
          {
            severity: "warning",
            file: "skills/execute/SKILL.md",
            message: "the body is 24001 bytes; only the first 24000 are loaded",
          },
        ],
      },
    ],
    [
      "non-size warning on skills/execute/SKILL.md",
      {
        components: { skills: 26 },
        findings: [
          {
            severity: "warning",
            file: "skills/execute/SKILL.md",
            message: "missing description",
          },
        ],
      },
    ],
    [
      "components.skills is 0",
      {
        components: { skills: 0 },
        findings: [],
      },
    ],
    [
      "missing findings",
      {
        components: { skills: 26 },
      },
    ],
    [
      "non-array findings",
      {
        components: { skills: 26 },
        findings: {},
      },
    ],
    [
      "null finding entry",
      {
        components: { skills: 26 },
        findings: [null],
      },
    ],
    ["null payload", null],
  ];
  for (const [label, payload] of failCases) {
    const outcome = validateReport(payload);
    assert.equal(outcome.ok, false, `expected failure for ${label}`);
  }
});

test("check fails on unexpected findings, invalid JSON, non-zero agy exit, or spawn failure", async () => {
  const unexpected = checkFakes({
    stdout: JSON.stringify({
      components: { skills: 26 },
      findings: [
        {
          severity: "warning",
          file: "README.md",
          message: "unexpected warning",
        },
      ],
    }),
  });
  const unexpectedErrors: string[] = [];
  assert.equal(
    await main(["check"], {
      env: {},
      run: unexpected.run,
      report: (line) => unexpectedErrors.push(line),
    }),
    1,
  );
  assert.match(
    unexpectedErrors.join("\n"),
    /Unexpected agy plugin validate findings/,
  );

  const badJson = checkFakes({ stdout: "{not-json" });
  const badJsonErrors: string[] = [];
  assert.equal(
    await main(["check"], {
      env: {},
      run: badJson.run,
      report: (line) => badJsonErrors.push(line),
    }),
    1,
  );
  assert.match(
    badJsonErrors.join("\n"),
    /^ci-antigravity-native: invalid JSON/,
  );

  const nonzero = checkFakes({ status: 3, stdout: '{"error":"broken"}\n' });
  const nonzeroErrors: string[] = [];
  assert.equal(
    await main(["check"], {
      env: {},
      run: nonzero.run,
      report: (line) => nonzeroErrors.push(line),
    }),
    3,
  );
  assert.deepEqual(nonzeroErrors, ['{"error":"broken"}']);

  for (const [dying, expected] of [
    [
      { status: null, error: new Error("spawn agy ENOENT") },
      /^ci-antigravity-native: cannot start agy/,
    ],
    [
      { status: null, signal: "SIGKILL" as const },
      /^ci-antigravity-native: agy was killed by SIGKILL/,
    ],
  ] as const) {
    const f = checkFakes(dying);
    const errors: string[] = [];
    assert.equal(
      await main(["check"], {
        env: {},
        run: f.run,
        report: (line) => errors.push(line),
      }),
      1,
    );
    assert.match(errors.join("\n"), expected);
  }
});

test("a missing or unknown subcommand prints the usage and exits 2", async () => {
  for (const argv of [[], ["bogus"], ["check", "extra"]]) {
    const errors: string[] = [];
    const status = main(argv, {
      env: {},
      run: () => assert.fail("nothing may start"),
      report: (line) => errors.push(line),
    });
    assert.equal(status, 2);
    assert.deepEqual(errors, ["usage: ci-antigravity-native.ts install|check"]);
  }
});

test("run as a script, the module sets the exit status", () => {
  const result = spawnSync(process.execPath, [MODULE], { encoding: "utf8" });
  assert.equal(result.status, 2);
  assert.match(
    result.stderr,
    /usage: ci-antigravity-native\.ts install\|check/,
  );
});
