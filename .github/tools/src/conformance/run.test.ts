import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import {
  mkdirSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { test } from "node:test";
import { fileURLToPath } from "node:url";
import { type DomainRun, MARKER, type RunOptions } from "./check.ts";
import {
  DOMAIN_REGISTRATIONS,
  ordinaryDomainNames,
  runSkillsDomain,
  usage,
} from "./registry.ts";
import { SKIP_LINE } from "./domains/skills/eval-check.ts";
import { main, SHORT_ANNOUNCEMENT } from "./run.ts";
import { fakeHost } from "./testing/fake-host.ts";
import { withSkillsRepo } from "./testing/skills-repo.ts";
import { writeStandardsBaseline } from "./testing/standards-baseline.ts";

const passing: DomainRun = async () => ({ failures: [] });
const failing: DomainRun = async () => ({
  failures: [{ rule: "r", path: "p", message: "m" }],
});
const domains = { tools: passing };
const USAGE = `${usage(domains)}\n`;

function verifierHost() {
  return fakeHost({
    env: {
      ...process.env,
      // The Conformance job runs this suite with short mode exported, which
      // would add the short-mode announcement to the stderr checked below.
      AGX_CONFORMANCE_SHORT: "",
      AGX_HOOK_VERIFIER: process.execPath,
      NODE_OPTIONS: `--import=${new URL("./domains/hooks/testing/fake-verifier.ts", import.meta.url).href}`,
      HOOK_TEST_OUTPUT: JSON.stringify({
        version: 1,
        areas: ["ascii", "schema", "module", "freshness"].map((name) => ({
          name,
          failures: [],
          error: null,
        })),
      }),
      HOOK_TEST_ARGS: "",
      HOOK_TEST_EXIT: "",
      HOOK_TEST_NOTICE: "",
    },
  });
}

async function withCheckout<T>(body: (root: string) => Promise<T>): Promise<T> {
  const root = mkdtempSync(path.join(tmpdir(), "agentics-cli-"));
  try {
    const marker = path.join(root, ...MARKER.split("/"));
    mkdirSync(path.dirname(marker), { recursive: true });
    writeFileSync(marker, "{}\n");
    return await body(root);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
}

function missingMarker(root: string): string {
  return `conformance: ${root} does not look like an agentics checkout (missing ${path.join(root, ...MARKER.split("/"))})\n`;
}

test("the registry runs five domains and keeps probes opt-in", () => {
  assert.deepEqual(ordinaryDomainNames(), [
    "docs",
    "hooks",
    "manifests",
    "repo",
    "skills",
  ]);
  const registrations = Object.entries(DOMAIN_REGISTRATIONS);
  assert.deepEqual(
    registrations
      .filter(([, domain]) => domain.optIn)
      .map(([name]) => name)
      .sort(),
    ["probes"],
  );
  assert.deepEqual(
    registrations
      .filter(([, domain]) => domain.staleReviewOnly)
      .map(([name]) => name),
    ["docs"],
  );
});

test("the usage text lists all domains and identifies opt-in probes", () => {
  assert.equal(
    usage(),
    [
      "usage: conformance <domain> [--repo <path-to-agentics-checkout>] [--allow-missing] [--short] [--fast] [--exclude <domain>]... [--stale-review-exit-code]",
      "domains: docs, hooks, manifests, repo, skills, all; probes (opt-in)",
      "--fast: implies --short; also skips the hooks module tests",
      "--exclude: all only; skip the named domain",
      "--stale-review-exit-code: docs only; exit 3 for exclusively stale reviews",
    ].join("\n"),
  );
  assert.equal(
    usage({ tools: passing }).split("\n")[1],
    "domains: tools, all; probes (opt-in)",
  );
});

test("the registered skills domain hands its evals runner the run's environment, not process.env", async () => {
  const scratch = mkdtempSync(path.join(tmpdir(), "agentics-cli-skills-env-"));
  try {
    const out = path.join(scratch, "seen.txt");
    const script = path.join(scratch, "evals-cli.mjs");
    // The script stands in for the evals CLI: it records a variable only the run's environment holds and prints a clean
    // stream. With process.env instead, the seam variable is absent too, so the script would never run.
    writeFileSync(
      script,
      `import { writeFileSync } from "node:fs";\nwriteFileSync(${JSON.stringify(out)}, String(process.env.AGX_RUN_ONLY));\nprocess.stdout.write('{"done":true,"failures":0}\\n');\n`,
    );
    assert.equal(process.env["AGX_RUN_ONLY"], undefined);
    assert.equal(process.env["AGX_CONFORMANCE_EVALS_CLI"], undefined);
    // The green fixture, with its standards baseline written, passes every family before eval-check, so the registered
    // run reaches the evals runner.
    await withSkillsRepo(async (root) => {
      writeStandardsBaseline(root);
      const marker = path.join(root, MARKER);
      mkdirSync(path.dirname(marker), { recursive: true });
      writeFileSync(marker, "{}\n");
      const argv = ["skills", "--repo", root];
      const host = fakeHost({
        env: {
          ...process.env,
          AGX_CONFORMANCE_SHORT: "",
          AGX_CONFORMANCE_EVALS_CLI: script,
          AGX_RUN_ONLY: "from-options",
        },
      });
      assert.equal(await main(argv, host), 0);
      assert.equal(host.stdout(), "PASS skills\n");
      assert.equal(host.stderr(), "");
      assert.equal(readFileSync(out, "utf8"), "from-options");

      const skipped = fakeHost({
        env: { AGX_EVALS_ROOT: path.join(scratch, "missing-evals") },
      });
      assert.equal(await main(argv, skipped), 0);
      assert.equal(skipped.stdout(), "PASS skills\n");
      assert.equal(skipped.stderr(), `${SKIP_LINE}\n`);
    });
  } finally {
    rmSync(scratch, { recursive: true, force: true });
  }
});

const YAML_MISSING_LINE =
  "ERROR skills: audit could not run: the yaml package is not installed; run npm ci --ignore-scripts at the repository root";

function moduleNotFound(message: string): Error {
  return Object.assign(new Error(message), { code: "ERR_MODULE_NOT_FOUND" });
}

const quietOptions: RunOptions = {
  short: true,
  fast: false,
  diagnostics: () => {},
  env: {},
};

test("the skills domain imports its module only when it runs, through a replaceable importer", async () => {
  const seen: unknown[] = [];
  const result = await runSkillsDomain("/checkout", quietOptions, async () => ({
    run: async (...args: unknown[]) => {
      seen.push(...args);
      return { failures: [{ rule: "r", path: "p", message: "m" }] };
    },
  }));
  assert.deepEqual(result, {
    failures: [{ rule: "r", path: "p", message: "m" }],
  });
  assert.equal(seen[0], "/checkout");
  assert.equal(seen[1], quietOptions);
  assert.deepEqual(seen[2], { evalsInvocation: quietOptions });
});

test("a missing yaml package is a skills audit error that names the install command", async () => {
  await withCheckout(async (root) => {
    let imports = 0;
    const missingYaml = async (): Promise<never> => {
      imports++;
      throw moduleNotFound(
        "Cannot find package 'yaml' imported from /checkout/.github/tools/src/conformance/domains/skills/frontmatter.ts",
      );
    };
    const runners: Record<string, DomainRun> = {
      manifests: passing,
      skills: (repo, options) => runSkillsDomain(repo, options, missingYaml),
    };
    const manifests = fakeHost();
    assert.equal(
      await main(["manifests", "--repo", root], manifests, runners),
      0,
    );
    assert.equal(manifests.stdout(), "PASS manifests\n");
    assert.equal(imports, 0);
    const skills = fakeHost();
    assert.equal(await main(["skills", "--repo", root], skills, runners), 1);
    assert.equal(skills.stdout(), "");
    assert.equal(
      skills.stderr(),
      `${YAML_MISSING_LINE}
`,
    );
    assert.equal(imports, 1);
  });
});

test("any other failed import of the skills domain keeps the generic audit error", async () => {
  const cases: Array<[string, Error]> = [
    [
      "another package",
      moduleNotFound(
        "Cannot find package 'yaml-extra' imported from /checkout/frontmatter.ts",
      ),
    ],
    [
      "a relative file whose path contains yaml",
      moduleNotFound(
        "Cannot find module '/checkout/yaml/frontmatter.ts' imported from /checkout/index.ts",
      ),
    ],
    [
      "the yaml message without the module-not-found code",
      new Error("Cannot find package 'yaml' imported from /checkout/x.ts"),
    ],
  ];
  await withCheckout(async (root) => {
    for (const [name, error] of cases) {
      const host = fakeHost();
      const runners: Record<string, DomainRun> = {
        skills: (repo, options) =>
          runSkillsDomain(repo, options, async () => {
            throw error;
          }),
      };
      assert.equal(await main(["skills", "--repo", root], host, runners), 1);
      assert.equal(
        host.stderr(),
        `ERROR skills: audit could not run: ${error.message}
`,
        name,
      );
    }
  });
});

// Registered before the CLI loads, the hook makes the bare specifier "yaml" fail to resolve with the error Node raises
// for a package that is not installed, so these runs behave as a checkout without node_modules does.
const WITHOUT_YAML = `data:text/javascript,${encodeURIComponent(`import { registerHooks } from "node:module";
import { fileURLToPath } from "node:url";
registerHooks({
  resolve(specifier, context, nextResolve) {
    if (specifier !== "yaml") return nextResolve(specifier, context);
    const error = new Error("Cannot find package 'yaml' imported from " + fileURLToPath(context.parentURL));
    error.code = "ERR_MODULE_NOT_FOUND";
    throw error;
  },
});
`)}`;

function runWithoutYaml(...args: string[]) {
  const cli = fileURLToPath(new URL("./cli.ts", import.meta.url));
  const repo = fileURLToPath(new URL("../../../../", import.meta.url));
  const result = spawnSync(
    process.execPath,
    ["--import", WITHOUT_YAML, cli, ...args, "--repo", repo],
    {
      encoding: "utf8",
      env: { ...process.env, AGX_CONFORMANCE_SHORT: "1" },
    },
  );
  return {
    status: result.status,
    stdout: result.stdout.split("\n"),
    couldNotRun: result.stderr
      .split("\n")
      .filter((line) => line.includes("audit could not run")),
    stderr: result.stderr,
  };
}

test("without the yaml package, the domains other than skills load and run", () => {
  for (const domain of ["docs", "manifests", "repo"]) {
    const result = runWithoutYaml(domain);
    assert.deepEqual(result.couldNotRun, [], domain);
    assert.doesNotMatch(result.stderr, /ERR_MODULE_NOT_FOUND|yaml/, domain);
    assert.equal(result.status, 0, domain + result.stderr);
    assert.ok(result.stdout.includes(`PASS ${domain}`), domain);
  }
});

test("without the yaml package, the skills domain exits 1 with the install instruction", () => {
  const result = runWithoutYaml("skills");
  assert.equal(result.status, 1);
  assert.deepEqual(result.couldNotRun, [YAML_MISSING_LINE]);
  assert.ok(!result.stdout.includes("PASS skills"));
});

test("without the yaml package, all fails on skills alone", () => {
  const result = runWithoutYaml("all", "--exclude", "hooks");
  assert.equal(result.status, 1);
  assert.deepEqual(result.couldNotRun, [YAML_MISSING_LINE]);
  for (const domain of ["docs", "manifests", "repo"])
    assert.ok(result.stdout.includes(`PASS ${domain}`), domain);
});

test("no arguments print the usage and exit 1", async () => {
  const host = fakeHost();
  assert.equal(await main([], host, domains), 1);
  assert.equal(host.stdout(), "");
  assert.equal(host.stderr(), USAGE);
  assert.match(USAGE, /^usage: conformance <domain> /);
});

test("argument errors exit 1 and say what was wrong", async () => {
  const cases: Array<[string[], string]> = [
    [["tools", "--repo"], USAGE],
    [["tools", "--bogus"], `conformance: unknown argument "--bogus"\n${USAGE}`],
    [
      ["docs", "--stale-review-exit-code", "--stale-review-exit-code"],
      "conformance: --stale-review-exit-code repeated\n",
    ],
    [
      ["tools", "--stale-review-exit-code"],
      "conformance: --stale-review-exit-code requires docs\n",
    ],
  ];
  for (const [argv, expected] of cases) {
    const host = fakeHost();
    assert.equal(await main(argv, host, domains), 1, argv.join(" "));
    assert.equal(host.stdout(), "", argv.join(" "));
    assert.equal(host.stderr(), expected, argv.join(" "));
  }
});

test("sparse argument slots use the empty domain, option, and operand values", async () => {
  await withCheckout(async (root) => {
    const whole = { tools: passing };
    const usageText = `${usage(whole)}\n`;
    const withHole = (...args: string[]) => {
      args.length += 1;
      return args;
    };
    const cases: Array<[string[], string]> = [
      [withHole(), `conformance: unknown domain ""\n${usageText}`],
      [withHole("tools"), `conformance: unknown argument ""\n${usageText}`],
      [
        withHole("all", "--exclude"),
        `conformance: --exclude names unknown domain ""\n${usageText}`,
      ],
    ];
    for (const [args, expected] of cases) {
      const host = fakeHost({ cwd: root });
      assert.equal(await main(args, host, whole), 1);
      assert.equal(host.stdout(), "");
      assert.equal(host.stderr(), expected);
    }

    const host = fakeHost({ cwd: root });
    assert.equal(await main(withHole("tools", "--repo"), host, whole), 0);
    assert.equal(host.stdout(), "PASS tools\n");
    assert.equal(host.stderr(), "");
  });
});

test("a --repo without the marker exits 1 naming the marker", async () => {
  const empty = mkdtempSync(path.join(tmpdir(), "agentics-nomarker-"));
  try {
    const host = fakeHost();
    assert.equal(await main(["tools", "--repo", empty], host, domains), 1);
    assert.equal(host.stdout(), "");
    assert.equal(host.stderr(), missingMarker(empty));
  } finally {
    rmSync(empty, { recursive: true, force: true });
  }
});

test("a --repo without the marker and an unknown domain also prints the usage", async () => {
  const empty = mkdtempSync(path.join(tmpdir(), "agentics-nomarker-"));
  try {
    const host = fakeHost();
    assert.equal(await main(["nope", "--repo", empty], host, domains), 1);
    assert.equal(host.stderr(), `${missingMarker(empty)}${USAGE}`);
  } finally {
    rmSync(empty, { recursive: true, force: true });
  }
});

test("a passing domain reports PASS and exits 0; a failing one exits 1", async () => {
  await withCheckout(async (root) => {
    const ok = fakeHost();
    assert.equal(
      await main(["tools", "--repo", root], ok, { tools: passing }),
      0,
    );
    assert.equal(ok.stdout(), "PASS tools\n");
    assert.equal(ok.stderr(), "");
    const bad = fakeHost();
    assert.equal(
      await main(["tools", "--repo", root], bad, { tools: failing }),
      1,
    );
    assert.equal(bad.stdout(), "");
    assert.equal(bad.stderr(), "FAIL tools [r] p: m\ntools: 1 failure(s)\n");
  });
});

test("the checkout is found above the working directory when --repo is absent", async () => {
  await withCheckout(async (root) => {
    const host = fakeHost({ cwd: root });
    assert.equal(await main(["tools"], host, domains), 0);
    assert.equal(host.stdout(), "PASS tools\n");
  });
  const empty = mkdtempSync(path.join(tmpdir(), "agentics-nomarker-"));
  try {
    const host = fakeHost({ cwd: empty });
    assert.equal(await main(["tools"], host, domains), 1);
    assert.equal(
      host.stderr(),
      `conformance: no agentics checkout found above the working directory (missing ${MARKER}); pass --repo\n${USAGE}`,
    );
  } finally {
    rmSync(empty, { recursive: true, force: true });
  }
});

test("short mode is announced on stderr before the domain runs and reaches the domain", async () => {
  await withCheckout(async (root) => {
    const seen: Array<{ options: RunOptions; stderrAtStart: string }> = [];
    const hosts: Array<ReturnType<typeof fakeHost>> = [];
    const capture: DomainRun = async (_root, options) => {
      const host = hosts[hosts.length - 1];
      assert.ok(host);
      seen.push({ options, stderrAtStart: host.stderr() });
      return { failures: [] };
    };
    const run = async (argv: string[], env: Record<string, string>) => {
      const host = fakeHost({ env });
      hosts.push(host);
      assert.equal(await main(argv, host, { tools: capture }), 0);
      return host;
    };
    const byFlag = await run(["tools", "--repo", root, "--short"], {});
    const byEnv = await run(["tools", "--repo", root], {
      AGX_CONFORMANCE_SHORT: "1",
    });
    const normal = await run(["tools", "--repo", root], {});
    const envZero = await run(["tools", "--repo", root], {
      AGX_CONFORMANCE_SHORT: "0",
    });
    assert.deepEqual(
      seen.map((entry) => entry.options.short),
      [true, true, false, false],
    );
    assert.deepEqual(
      seen.map((entry) => entry.stderrAtStart),
      [SHORT_ANNOUNCEMENT, SHORT_ANNOUNCEMENT, "", ""],
    );
    assert.equal(
      SHORT_ANNOUNCEMENT,
      "conformance: short mode (AGX_CONFORMANCE_SHORT=1)\n",
    );
    assert.equal(byFlag.stderr(), SHORT_ANNOUNCEMENT);
    assert.equal(byEnv.stderr(), SHORT_ANNOUNCEMENT);
    assert.equal(byFlag.stdout(), "PASS tools\n");
    assert.equal(normal.stderr(), "");
    assert.equal(envZero.stderr(), "");
  });
});

test("fast mode implies short, is announced in place of short mode, and reaches the domain", async () => {
  await withCheckout(async (root) => {
    const seen: Array<{ options: RunOptions; stderrAtStart: string }> = [];
    const hosts: Array<ReturnType<typeof fakeHost>> = [];
    const capture: DomainRun = async (_root, options) => {
      const host = hosts[hosts.length - 1];
      assert.ok(host);
      seen.push({ options, stderrAtStart: host.stderr() });
      return { failures: [] };
    };
    const run = async (argv: string[], env: Record<string, string>) => {
      const host = fakeHost({ env });
      hosts.push(host);
      assert.equal(await main(argv, host, { docs: capture }), 0);
      return host;
    };
    const fastLine =
      "conformance: fast mode (--fast) — the hooks module tests are NOT run; this is not a full audit\n";
    const fast = await run(["docs", "--repo", root, "--fast"], {});
    const fastAndShort = await run(
      ["docs", "--repo", root, "--short", "--fast"],
      { AGX_CONFORMANCE_SHORT: "1" },
    );
    const normal = await run(["docs", "--repo", root], {});
    const shortOnly = await run(["docs", "--repo", root, "--short"], {});
    assert.deepEqual(
      seen.map((entry) => [entry.options.short, entry.options.fast]),
      [
        [true, true],
        [true, true],
        [false, false],
        [true, false],
      ],
    );
    assert.deepEqual(
      seen.map((entry) => entry.stderrAtStart),
      [fastLine, fastLine, "", SHORT_ANNOUNCEMENT],
    );
    assert.equal(fast.stderr(), fastLine);
    assert.equal(fastAndShort.stderr(), fastLine);
    assert.equal(normal.stderr(), "");
    assert.equal(shortOnly.stderr(), SHORT_ANNOUNCEMENT);
    for (const selector of ["tools", "all"]) {
      seen.length = 0;
      const host = fakeHost({ env: {} });
      hosts.push(host);
      assert.equal(
        await main([selector, "--repo", root, "--fast"], host, {
          tools: capture,
        }),
        0,
        selector,
      );
      assert.deepEqual(
        seen.map((entry) => [entry.options.short, entry.options.fast]),
        [[true, true]],
        selector,
      );
      assert.equal(host.stderr(), fastLine, selector);
    }
  });
});

test("hooks passes --skip-tests to the verifier only under --fast", async () => {
  await withCheckout(async (root) => {
    const fastLine =
      "conformance: fast mode (--fast) — the hooks module tests are NOT run; this is not a full audit\n";
    const cases: Array<[string[], Record<string, string>, boolean, string]> = [
      [["--fast"], {}, true, fastLine],
      [["--short", "--fast"], {}, true, fastLine],
      [["--short"], {}, false, SHORT_ANNOUNCEMENT],
      [[], { AGX_CONFORMANCE_SHORT: "1" }, false, SHORT_ANNOUNCEMENT],
    ];
    for (const [flags, env, skips, announcement] of cases) {
      const argsFile = path.join(root, "verifier-args");
      const base = verifierHost();
      const host = fakeHost({
        env: { ...base.env, ...env, HOOK_TEST_ARGS: argsFile },
      });
      const label = JSON.stringify([flags, env]);
      assert.equal(
        await main(["hooks", "--repo", root, ...flags], host),
        0,
        label,
      );
      assert.equal(host.stderr(), announcement, label);
      const argv = readFileSync(argsFile, "utf8").split("\n");
      assert.equal(argv.includes("--skip-tests"), skips, label);
      rmSync(argsFile);
    }
  });
});

test("no environment variable selects fast mode", async () => {
  await withCheckout(async (root) => {
    const seen: RunOptions[] = [];
    const capture: DomainRun = async (_root, options) => {
      seen.push(options);
      return { failures: [] };
    };
    const host = fakeHost({
      env: {
        AGX_CONFORMANCE_FAST: "1",
        AGX_CONFORMANCE_SHORT: "",
      },
    });
    assert.equal(
      await main(["docs", "--repo", root], host, { docs: capture }),
      0,
    );
    assert.equal(seen[0]?.fast, false);
    assert.equal(host.stderr(), "");
  });
});

test("short mode is announced even when the checkout marker is missing", async () => {
  const empty = mkdtempSync(path.join(tmpdir(), "agentics-nomarker-"));
  try {
    const host = fakeHost();
    assert.equal(
      await main(["tools", "--repo", empty, "--short"], host, domains),
      1,
    );
    assert.equal(host.stderr(), `${SHORT_ANNOUNCEMENT}${missingMarker(empty)}`);
  } finally {
    rmSync(empty, { recursive: true, force: true });
  }
});

test("the domain receives the host env and a diagnostics sink bound to the host stderr", async () => {
  await withCheckout(async (root) => {
    let received: RunOptions | undefined;
    const probe: DomainRun = async (_root, options) => {
      received = options;
      options.diagnostics("SKIP: from the domain\n");
      return { failures: [{ rule: "r", path: "p", message: "m" }] };
    };
    const env = { SOME_KEY: "value" };
    const host = fakeHost({ env });
    assert.equal(
      await main(["tools", "--repo", root], host, { tools: probe }),
      1,
    );
    assert.equal(received?.env, env);
    assert.equal(host.stdout(), "");
    assert.equal(
      host.stderr(),
      "SKIP: from the domain\nFAIL tools [r] p: m\ntools: 1 failure(s)\n",
    );
  });
});

test("an unknown domain exits 1 with the usage", async () => {
  await withCheckout(async (root) => {
    const host = fakeHost();
    assert.equal(await main(["nope", "--repo", root], host, domains), 1);
    assert.equal(host.stdout(), "");
    assert.equal(host.stderr(), `conformance: unknown domain "nope"\n${USAGE}`);
  });
});

test("names inherited from Object.prototype are unknown domains", async () => {
  await withCheckout(async (root) => {
    for (const name of [
      "constructor",
      "toString",
      "__proto__",
      "hasOwnProperty",
    ]) {
      const host = fakeHost();
      assert.equal(await main([name, "--repo", root], host, domains), 1, name);
      assert.equal(host.stdout(), "", name);
      assert.equal(
        host.stderr(),
        `conformance: unknown domain "${name}"\n${USAGE}`,
        name,
      );
    }
  });
  const empty = mkdtempSync(path.join(tmpdir(), "agentics-nomarker-"));
  try {
    const host = fakeHost();
    assert.equal(
      await main(["constructor", "--repo", empty], host, domains),
      1,
    );
    assert.equal(host.stderr(), `${missingMarker(empty)}${USAGE}`);
  } finally {
    rmSync(empty, { recursive: true, force: true });
  }
});

test("a relative --repo resolves against the host working directory", async () => {
  await withCheckout(async (root) => {
    const host = fakeHost({ cwd: path.dirname(root) });
    assert.notEqual(host.cwd, process.cwd());
    assert.equal(
      await main(["tools", "--repo", path.basename(root)], host, domains),
      0,
    );
    assert.equal(host.stdout(), "PASS tools\n");
  });
});

test("a domain that rejects ends in a printed error and exit 1", async () => {
  await withCheckout(async (root) => {
    const host = fakeHost();
    const boom: DomainRun = async () => {
      throw new Error("boom");
    };
    assert.equal(
      await main(["tools", "--repo", root], host, { tools: boom }),
      1,
    );
    assert.equal(host.stdout(), "");
    assert.equal(host.stderr(), "ERROR tools: audit could not run: boom\n");
  });
});

test("the entry file runs under type stripping", () => {
  const cli = fileURLToPath(new URL("./cli.ts", import.meta.url));
  const result = spawnSync(
    process.execPath,
    ["--disable-warning=ExperimentalWarning", cli],
    { encoding: "utf8" },
  );
  assert.equal(result.status, 1);
  assert.equal(result.stdout, "");
  assert.equal(result.stderr, `${usage()}\n`);
});

test("skills is not special-cased: missing from the domains seam, it is an unknown domain", async () => {
  await withCheckout(async (root) => {
    const host = fakeHost();
    assert.equal(
      await main(["skills", "--repo", root], host, { tools: passing }),
      1,
    );
    assert.equal(host.stdout(), "");
    assert.equal(
      host.stderr(),
      `conformance: unknown domain "skills"\n${USAGE}`,
    );
  });
});

const stale: DomainRun = async () => ({
  failures: [
    { rule: "review-age", path: "docs/surfaces/README.md", message: "old" },
  ],
});
const staleAndOther: DomainRun = async () => ({
  failures: [
    { rule: "review-age", path: "docs/surfaces/README.md", message: "old" },
    {
      rule: "ledger-row-date",
      path: "docs/surfaces/README.md",
      message: "bad",
    },
  ],
});
const staleThenError: DomainRun = async () => ({
  failures: [
    { rule: "review-age", path: "docs/surfaces/README.md", message: "old" },
  ],
  error: "could not finish",
});

test("docs --stale-review-exit-code exits 3 only when every failure is a stale review", async () => {
  const cases: Array<{
    name: string;
    docs: DomainRun;
    flag: boolean;
    exit: number;
  }> = [
    { name: "stale only, flag set", docs: stale, flag: true, exit: 3 },
    { name: "stale only, flag unset", docs: stale, flag: false, exit: 1 },
    {
      name: "stale plus another rule",
      docs: staleAndOther,
      flag: true,
      exit: 1,
    },
    {
      name: "stale plus an audit error",
      docs: staleThenError,
      flag: true,
      exit: 1,
    },
    { name: "passing, flag set", docs: passing, flag: true, exit: 0 },
    { name: "another rule only", docs: failing, flag: true, exit: 1 },
    {
      name: "a domain that throws",
      docs: async () => {
        throw new Error("boom");
      },
      flag: true,
      exit: 1,
    },
  ];
  await withCheckout(async (root) => {
    for (const { name, docs, flag, exit } of cases) {
      const host = fakeHost();
      const argv = [
        "docs",
        "--repo",
        root,
        ...(flag ? ["--stale-review-exit-code"] : []),
      ];
      assert.equal(await main(argv, host, { docs }), exit, name);
    }
  });
});

test("the stale-review exit code is for docs only, so another domain's stale-looking failure stays 1", async () => {
  await withCheckout(async (root) => {
    // `tools` rejects the flag outright. That a non-docs domain never counts as stale is tested in report.test.ts.
    const host = fakeHost();
    assert.equal(
      await main(["tools", "--repo", root, "--stale-review-exit-code"], host, {
        tools: stale,
      }),
      1,
    );
    assert.equal(
      host.stderr(),
      "conformance: --stale-review-exit-code requires docs\n",
    );
  });
});

test("named hooks runs the registered verifier through the CLI", async () => {
  await withCheckout(async (root) => {
    const host = verifierHost();
    assert.equal(await main(["hooks", "--repo", root], host), 0);
    assert.equal(host.stdout(), "PASS hooks\n");
    assert.equal(host.stderr(), "");
  });
});

test("probes runs only when named and receives allow-missing", async () => {
  await withCheckout(async (root) => {
    const calls: RunOptions[] = [];
    const probe: DomainRun = async (_root, options) => {
      calls.push(options);
      return { failures: [] };
    };
    const runners = { tools: passing, probes: probe };
    assert.equal(await main(["all", "--repo", root], fakeHost(), runners), 0);
    assert.equal(calls.length, 0);
    const host = fakeHost();
    assert.equal(
      await main(["probes", "--repo", root, "--allow-missing"], host, runners),
      0,
    );
    assert.equal(host.stdout(), "PASS probes\n");
    assert.equal(
      (calls[0] as RunOptions & { allowMissing?: boolean }).allowMissing,
      true,
    );
    assert.equal(
      await main(["probes", "--repo", root], fakeHost(), runners),
      0,
    );
    assert.equal(
      (calls[1] as RunOptions & { allowMissing?: boolean }).allowMissing,
      false,
    );
    const missing = fakeHost({ env: { PATH: "" } });
    assert.equal(
      await main(["probes", "--repo", root, "--allow-missing"], missing),
      0,
    );
    assert.match(missing.stderr(), /SKIP.*copilot/i);
    const required = fakeHost({ env: { PATH: "" } });
    assert.equal(await main(["probes", "--repo", root], required), 1);
    assert.match(required.stderr(), /ERROR probes:.*copilot is required/);

    const fallback = fakeHost({ env: { PATH: "" } });
    assert.equal(
      await main(["probes", "--repo", root, "--allow-missing"], fallback, {
        tools: passing,
      }),
      0,
    );
    assert.equal(fallback.stdout(), missing.stdout());
    assert.equal(fallback.stderr(), missing.stderr());
  });
});

test("known domains check the marker before running", async () => {
  const empty = mkdtempSync(path.join(tmpdir(), "agentics-nomarker-"));
  try {
    for (const name of ["hooks", "probes", "all"]) {
      const host = fakeHost();
      assert.equal(await main([name, "--repo", empty], host), 1, name);
      assert.equal(host.stderr(), missingMarker(empty), name);
    }
  } finally {
    rmSync(empty, { recursive: true, force: true });
  }
});

test("all runs five registered domains, uses the verifier and omits probes", async () => {
  await withCheckout(async (root) => {
    const host = verifierHost();
    const runners = Object.fromEntries(
      ordinaryDomainNames().map((name) => [
        name,
        name === "hooks" ? DOMAIN_REGISTRATIONS.hooks!.run : passing,
      ]),
    );
    assert.equal(
      await main(["all", "--repo", root, "--short"], host, runners),
      0,
    );
    assert.equal(
      host.stdout(),
      "PASS docs\nPASS hooks\nPASS manifests\nPASS repo\nPASS skills\n",
    );
    assert.equal(host.stderr(), SHORT_ANNOUNCEMENT);
  });
});

test("all runs the registered domains with nothing on stderr", async () => {
  await withCheckout(async (root) => {
    const host = fakeHost();
    assert.equal(
      await main(["all", "--repo", root], host, { tools: passing }),
      0,
    );
    assert.equal(host.stdout(), "PASS tools\n");
    assert.equal(host.stderr(), "");
  });
});

test("all exits 1 when a registered domain fails", async () => {
  await withCheckout(async (root) => {
    const host = fakeHost();
    assert.equal(
      await main(["all", "--repo", root], host, { tools: failing }),
      1,
    );
    assert.equal(host.stdout(), "");
    assert.equal(host.stderr(), "FAIL tools [r] p: m\ntools: 1 failure(s)\n");
  });
});

test("all announces short mode before the domain output", async () => {
  await withCheckout(async (root) => {
    const host = fakeHost();
    assert.equal(
      await main(["all", "--repo", root, "--short"], host, { tools: failing }),
      1,
    );
    assert.equal(
      host.stderr(),
      `${SHORT_ANNOUNCEMENT}FAIL tools [r] p: m\ntools: 1 failure(s)\n`,
    );
  });
});

test("all runs every registered domain in sorted order and survives a rejecting one", async () => {
  await withCheckout(async (root) => {
    const order: string[] = [];
    const record =
      (name: string, result: "pass" | "fail" | "throw"): DomainRun =>
      async () => {
        order.push(name);
        if (result === "throw") throw new Error("boom");
        return {
          failures:
            result === "fail" ? [{ rule: "r", path: "p", message: "m" }] : [],
        };
      };
    const host = fakeHost();
    assert.equal(
      await main(["all", "--repo", root], host, {
        zeta: record("zeta", "pass"),
        alpha: record("alpha", "throw"),
        mid: record("mid", "fail"),
      }),
      1,
    );
    assert.deepEqual(order, ["alpha", "mid", "zeta"]);
    assert.equal(host.stdout(), "PASS zeta\n");
    assert.equal(
      host.stderr(),
      "ERROR alpha: audit could not run: boom\n" +
        "FAIL mid [r] p: m\nmid: 1 failure(s)\n",
    );
  });
});

test("all skips a domain no longer registered when its turn arrives", async () => {
  await withCheckout(async (root) => {
    const registered: Record<string, DomainRun> = {
      alpha: async () => {
        delete registered.zeta;
        return { failures: [] };
      },
      zeta: async () => assert.fail("removed domain must not run"),
    };
    const host = fakeHost();
    assert.equal(await main(["all", "--repo", root], host, registered), 0);
    assert.equal(host.stdout(), "PASS alpha\n");
    assert.equal(host.stderr(), "");
  });
});

test("all never carries the stale-review exit code", async () => {
  await withCheckout(async (root) => {
    const host = fakeHost();
    assert.equal(
      await main(["all", "--repo", root, "--stale-review-exit-code"], host, {
        docs: passing,
      }),
      1,
    );
    assert.equal(
      host.stderr(),
      "conformance: --stale-review-exit-code requires docs\n",
    );
  });
});

test("all --exclude drops each named domain from the run and can repeat", async () => {
  const cases: Array<[string[], string[]]> = [
    [
      ["--exclude", "b"],
      ["a", "c"],
    ],
    [["--exclude", "a", "--exclude", "c"], ["b"]],
    [["--exclude", "a", "--exclude", "b", "--exclude", "c"], []],
  ];
  await withCheckout(async (root) => {
    for (const [flags, expected] of cases) {
      const order: string[] = [];
      const record =
        (name: string): DomainRun =>
        async () => {
          order.push(name);
          return { failures: [] };
        };
      const host = fakeHost();
      assert.equal(
        await main(["all", "--repo", root, ...flags], host, {
          a: record("a"),
          b: record("b"),
          c: record("c"),
        }),
        0,
        flags.join(" "),
      );
      assert.deepEqual(order, expected, flags.join(" "));
      assert.equal(
        host.stdout(),
        expected.map((name) => `PASS ${name}\n`).join(""),
        flags.join(" "),
      );
      assert.equal(host.stderr(), "", flags.join(" "));
    }
  });
});

test("--exclude errors exit 1 with a reason and the usage before any domain runs", async () => {
  const abc = { a: passing, b: passing, c: passing };
  const abcUsage = `${usage(abc)}\n`;
  const cases: Array<[string[], string]> = [
    [["a", "--exclude", "b"], "conformance: --exclude requires all\n"],
    [
      ["all", "--exclude", "zz"],
      'conformance: --exclude names unknown domain "zz"\n',
    ],
    [
      ["all", "--exclude", "probes"],
      'conformance: --exclude cannot name opt-in domain "probes"; all never runs it\n',
    ],
    [
      ["all", "--exclude", "a", "--exclude", "a"],
      'conformance: --exclude "a" repeated\n',
    ],
    [["all", "--exclude"], "conformance: --exclude needs a domain\n"],
  ];
  for (const [argv, reason] of cases) {
    let ran = false;
    const spy: DomainRun = async () => {
      ran = true;
      return { failures: [] };
    };
    const host = fakeHost();
    assert.equal(
      await main(argv, host, { a: spy, b: spy, c: spy }),
      1,
      argv.join(" "),
    );
    assert.equal(ran, false, argv.join(" "));
    assert.equal(host.stdout(), "", argv.join(" "));
    assert.equal(host.stderr(), `${reason}${abcUsage}`, argv.join(" "));
  }
});
