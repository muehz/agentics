import assert from "node:assert/strict";
import { test, type TestContext } from "node:test";
import {
  cpSync,
  existsSync,
  mkdirSync,
  mkdtempSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { run, type ProbeContext } from "./index.ts";

function fixture(t: TestContext) {
  const base = mkdtempSync(path.join(tmpdir(), "agentics-probe-test-"));
  t.after(() => rmSync(base, { recursive: true, force: true }));
  const installed = path.join(base, "installed");
  const source = path.join(base, "source");
  const skills = ["./skills/alpha", "./skills/beta"];
  const manifest: Record<string, unknown> = {
    name: "agentics",
    version: "1.0.0",
    skills,
    hooks: "./hooks/hooks.copilot.json",
  };
  const sourceManifest: Record<string, unknown> = { ...manifest };
  const claude: Record<string, unknown> = {
    name: "agentics",
    version: "1.0.0",
    skills,
  };
  const listed = skills.map((skill) => ({
    source: "plugin",
    enabled: true,
    path: path.join(installed, skill),
  }));
  const raw: Record<string, string | null> = {};
  const calls: { args: string[]; env: Record<string, string> }[] = [];
  let list: unknown = [
    {
      name: "agentics",
      marketplace: "muehz",
      source: "marketplace",
    },
  ];
  let failAt = "";
  let skillOutput: string | undefined;
  let output = "failed";
  const diagnostics: string[] = [];
  function put(root: string, rel: string, value: unknown) {
    const file = path.join(root, rel);
    mkdirSync(path.dirname(file), { recursive: true });
    writeFileSync(
      file,
      typeof value === "string" ? value : JSON.stringify(value),
    );
  }
  function prepare() {
    put(installed, ".github/plugin/plugin.json", manifest);
    put(installed, ".claude-plugin/plugin.json", claude);
    put(installed, ".claude-plugin/marketplace.json", {
      plugins: [{ name: "agentics", version: "1.0.0", source: "./" }],
    });
    put(installed, "hooks/hooks.copilot.json", "{}");
    put(source, ".github/plugin/plugin.json", sourceManifest);
    for (const skill of skills)
      mkdirSync(path.join(installed, skill), { recursive: true });
    for (const [rel, value] of Object.entries(raw)) {
      if (value === null) rmSync(path.join(installed, rel), { force: true });
      else put(installed, rel, value);
    }
    if (
      Array.isArray(list) &&
      list[0]?.source === "live" &&
      list[0]?.installedFrom === source
    ) {
      cpSync(installed, source, { recursive: true });
      put(source, ".github/plugin/plugin.json", sourceManifest);
    }
  }
  const context: ProbeContext = {
    lookPath: () => "copilot-fixture",
    execute: (_exe, args, options) => {
      calls.push({ args, env: options.env });
      if (args.join(" ") === failAt) return { status: 1, output };
      if (args[0] === "plugin" && args[1] === "list") {
        if (
          Array.isArray(list) &&
          list[0]?.source === "live" &&
          list[0]?.installedFrom === installed
        )
          list[0].installedFrom = source;
        return { status: 0, output: JSON.stringify(list) };
      }
      return {
        status: 0,
        output: args[0] === "skill" ? JSON.stringify(listed) : "",
      };
    },
  };
  return {
    installed,
    source,
    manifest,
    sourceManifest,
    claude,
    listed,
    raw,
    calls,
    diagnostics,
    setList: (value: unknown) => {
      list = value;
    },
    setSkillOutput: (text: string) => {
      skillOutput = text;
    },
    fail: (args: string, text = "failed") => {
      failAt = args;
      output = text;
    },
    async audit() {
      prepare();
      // A copied install is placed in the isolated provider home by the injected CLI.
      const execute = context.execute;
      const copyContext: ProbeContext = {
        ...context,
        execute: async (exe, args, options) => {
          const result = await execute(exe, args, options);
          if (
            args.join(" ") === "plugin list --json" &&
            Array.isArray(list) &&
            list[0]?.source !== "live"
          ) {
            const target = path.join(
              options.env.COPILOT_HOME!,
              "installed-plugins/muehz/agentics",
            );
            cpSync(installed, target, { recursive: true });
          }
          if (
            args[0] === "skill" &&
            result.status === 0 &&
            Array.isArray(list)
          ) {
            const target =
              list[0]?.source === "live"
                ? source
                : path.join(
                    options.env.COPILOT_HOME!,
                    "installed-plugins/muehz/agentics",
                  );
            result.output =
              skillOutput ??
              JSON.stringify(
                listed.map((skill) => ({
                  ...skill,
                  path: skill.path.replace(installed, target),
                })),
              );
          }
          return result;
        },
      };
      return run(
        source,
        {
          short: false,
          fast: false,
          diagnostics: (text) => diagnostics.push(text),
          env: {
            PATH: "fixture-path",
            APPDATA: "roaming",
            LOCALAPPDATA: "local",
            CLAUDE_CODE_SESSION_ACCESS_TOKEN: "must-not-leak",
          },
        },
        copyContext,
      );
    },
  };
}

test("a missing copilot with allow-missing skips the probes with a notice", async () => {
  const notices: string[] = [];
  assert.deepEqual(
    await run(
      ".",
      {
        short: false,
        fast: false,
        diagnostics: (text) => notices.push(text),
        env: {},
        allowMissing: true,
      },
      {
        lookPath: () => undefined,
        execute: () => {
          throw new Error("must not execute");
        },
      },
    ),
    { failures: [] },
  );
  assert.match(
    notices.join(""),
    /SKIP: copilot is not installed \(allow-missing\)/,
  );
});
test("a missing copilot without allow-missing is an audit error", async () => {
  assert.match(
    (
      await run(
        ".",
        { short: false, fast: false, diagnostics() {}, env: {} },
        {
          lookPath: () => undefined,
          execute: () => {
            throw new Error("must not execute");
          },
        },
      )
    ).error!,
    /copilot is required; pass allow-missing/,
  );
});
test("a faithful install passes every probe and leaves no Copilot home behind", async (t) => {
  const f = fixture(t);
  f.setList([
    { name: "agentics", marketplace: "muehz", source: "marketplace" },
  ]);
  assert.deepEqual(await f.audit(), { failures: [] });
  assert.equal(f.calls.length, 4);
  const home = f.calls[0]!.env.COPILOT_HOME!;
  assert.equal(existsSync(home), false);
});
const cases: [
  string,
  (f: ReturnType<typeof fixture>) => void,
  string,
  RegExp,
][] = [
  [
    "an installed manifest that differs from the source",
    (f) => {
      f.sourceManifest.skills = [
        "./skills/alpha",
        "./skills/beta",
        "./skills/gamma",
      ];
    },
    "probe-copilot-skills",
    /installed manifest does not match source manifest/,
  ],
  [
    "a registered skill missing from the skill list",
    (f) => {
      f.listed.pop();
    },
    "probe-copilot-skills",
    /expected exactly 2 enabled/,
  ],
  [
    "a disabled skill",
    (f) => {
      f.listed[1]!.enabled = false;
    },
    "probe-copilot-skills",
    /expected exactly 2 enabled/,
  ],
  [
    "a skill listed at another path",
    (f) => {
      f.listed[1]!.path = path.join(f.installed, "skills/elsewhere");
    },
    "probe-copilot-skills",
    /installed skill paths do not match/,
  ],
  [
    "a skill that is not from the plugin",
    (f) => {
      f.listed[1]!.source = "user";
    },
    "probe-copilot-skills",
    /expected exactly 2 enabled installed Agentics plugin skills, got 1/,
  ],
  [
    "a manifest naming the wrong hooks file",
    (f) => {
      f.manifest.hooks = "./hooks/hooks.claude.json";
    },
    "probe-copilot-hooks",
    /effective installed hooks must be/,
  ],
  [
    "a hooks file that is missing",
    (f) => {
      f.raw["hooks/hooks.copilot.json"] = null;
    },
    "probe-copilot-hooks",
    /effective installed hooks target does not exist/,
  ],
  [
    "a missing installed Claude manifest",
    (f) => {
      f.raw[".claude-plugin/plugin.json"] = null;
    },
    "probe-copilot-invariants",
    /installed .* is missing or unreadable/,
  ],
  [
    "a corrupt installed Copilot manifest",
    (f) => {
      f.raw[".github/plugin/plugin.json"] = "{bad";
    },
    "probe-copilot-invariants",
    /installed .* is not valid JSON/,
  ],
  [
    "a corrupt installed marketplace catalog",
    (f) => {
      f.raw[".claude-plugin/marketplace.json"] = "{bad";
    },
    "probe-copilot-invariants",
    /installed .* is not valid JSON/,
  ],
  [
    "a corrupt installed Claude manifest",
    (f) => {
      f.raw[".claude-plugin/plugin.json"] = "{bad";
    },
    "probe-copilot-invariants",
    /installed .* is not valid JSON/,
  ],
  [
    "installed skills that break Copilot skills parity",
    (f) => {
      f.claude.skills = ["./skills/alpha"];
    },
    "probe-copilot-invariants",
    /installed tree violates copilot-skills-parity/,
  ],
  [
    "a source manifest that declares no skills",
    (f) => {
      f.sourceManifest.skills = [];
    },
    "probe-copilot-skills",
    /declares no skills/,
  ],
];
for (const [name, mutate, rule, message] of cases)
  test(`the probe fails on ${name}`, async (t) => {
    const f = fixture(t);
    f.setList([
      { name: "agentics", marketplace: "muehz", source: "marketplace" },
    ]);
    mutate(f);
    const result = await f.audit();
    assert.equal(result.error, undefined);
    assert.ok(
      result.failures.some(
        (failure) => failure.rule === rule && message.test(failure.message),
      ),
      JSON.stringify(result),
    );
    assert.ok(
      result.failures.every(
        (failure) =>
          !failure.rule.startsWith("copilot-") &&
          !path.isAbsolute(failure.path),
      ),
    );
  });
test("a live install whose source is the checkout passes", async (t) => {
  const f = fixture(t);
  // The source and installed root coincide for a live install.
  f.setList([
    {
      name: "agentics",
      marketplace: "muehz",
      source: "live",
      installedFrom: f.source,
    },
  ]);
  const result = await f.audit();
  assert.deepEqual(result, { failures: [] });
});
test("a marketplace install copied from the checkout passes", async (t) => {
  const f = fixture(t);
  f.setList([
    { name: "agentics", marketplace: "muehz", source: "marketplace" },
  ]);
  assert.deepEqual(await f.audit(), { failures: [] });
});
test("plugin list validates every typed row before selecting Agentics", async (t) => {
  for (const malformed of [
    { name: 42 },
    { marketplace: false },
    { source: [] },
    { installedFrom: {} },
    [],
    { name: 42, NAME: "other" },
  ]) {
    await t.test(JSON.stringify(malformed), async (t) => {
      const f = fixture(t);
      f.setList([
        { name: "agentics", marketplace: "muehz", source: "marketplace" },
        malformed,
      ]);
      const result = await f.audit();
      assert.equal(result.failures.length, 1);
      assert.equal(result.failures[0]!.rule, "probe-copilot-install");
      assert.match(
        result.failures[0]!.message,
        /plugin list --json output unparseable/,
      );
    });
  }
});
test("plugin list skips null rows and ignores unknown fields", async (t) => {
  const f = fixture(t);
  f.setList([
    null,
    {
      name: "agentics",
      marketplace: "muehz",
      source: "marketplace",
      unknown: 42,
    },
  ]);
  assert.deepEqual(await f.audit(), { failures: [] });
});
test("plugin list reads field names in exact case", async (t) => {
  const f = fixture(t);
  f.setList([
    { Name: "agentics", Marketplace: "muehz", Source: "marketplace" },
  ]);
  assert.deepEqual(await f.audit(), {
    failures: [
      {
        rule: "probe-copilot-install",
        path: "",
        message: "copilot plugin list --json has no agentics@muehz entry",
      },
    ],
  });
});
test("a live install from another directory is not the audited checkout", async (t) => {
  const f = fixture(t);
  f.setList([
    {
      name: "agentics",
      marketplace: "muehz",
      source: "live",
      installedFrom: path.dirname(f.source),
    },
  ]);
  assert.match(
    (await f.audit()).failures[0]!.message,
    /not the audited checkout/,
  );
});
test("a plugin list without agentics@muehz fails the install probe", async (t) => {
  const f = fixture(t);
  f.setList([{ name: "other", marketplace: "muehz", source: "live" }]);
  assert.match(
    (await f.audit()).failures[0]!.message,
    /no agentics@muehz entry/,
  );
});
test("the probe environment keeps the Windows user directories and drops the session token", async (t) => {
  const f = fixture(t);
  await f.audit();
  for (const { env } of f.calls) {
    assert.equal(env.APPDATA, "roaming");
    assert.equal(env.LOCALAPPDATA, "local");
    assert.equal(env.CLAUDE_CODE_SESSION_ACCESS_TOKEN, undefined);
    assert.ok(env.COPILOT_HOME);
    assert.ok(env.COPILOT_CACHE_HOME);
  }
});
for (const args of [
  "plugin marketplace add",
  "plugin install agentics@muehz",
  "plugin list --json",
  "skill list --json",
])
  test(`CLI failure and cleanup: ${args}`, async (t) => {
    const f = fixture(t);
    const lines = Array.from({ length: 25 }, (_, i) => `line-${i}`);
    // Repeated trailing newlines must all go before the last 20 lines are kept.
    f.fail(
      args === "plugin marketplace add" ? `${args} ${f.source}` : args,
      `${lines.join("\n")}\n\n\n`,
    );
    const result = await f.audit();
    // The marketplace step is named without the checkout path.
    assert.deepEqual(result, {
      failures: [
        {
          rule: args.startsWith("skill")
            ? "probe-copilot-skills"
            : "probe-copilot-install",
          path: "",
          message: `copilot ${args} failed:\n${lines.slice(5).join("\n")}`,
        },
      ],
    });
    assert.equal(existsSync(f.calls[0]!.env.COPILOT_HOME!), false);
  });
test("plugin list rejects a non-object row with its reason", async (t) => {
  const f = fixture(t);
  f.setList([
    { name: "agentics", marketplace: "muehz", source: "marketplace" },
    "agentics",
  ]);
  assert.deepEqual(await f.audit(), {
    failures: [
      {
        rule: "probe-copilot-install",
        path: "",
        message:
          "copilot plugin list --json output unparseable: expected plugin objects",
      },
    ],
  });
});
test("skill list accepts null and rejects non-object rows", async (t) => {
  const unparseable =
    "copilot skill list --json output unparseable: expected an array of skill objects";
  for (const [output, message] of [
    // A JSON null lists no skills rather than failing to parse.
    [
      "null",
      "expected exactly 2 enabled installed Agentics plugin skills, got 0",
    ],
    ["{}", unparseable],
    ["[null]", unparseable],
    ['["skill"]', unparseable],
    ["[[]]", unparseable],
  ]) {
    await t.test(output!, async (t) => {
      const f = fixture(t);
      f.setSkillOutput(output!);
      const result = await f.audit();
      assert.equal(result.error, undefined);
      assert.deepEqual(
        result.failures
          .filter((failure) => failure.rule === "probe-copilot-skills")
          .map((failure) => failure.message)
          .filter((text) => !text.startsWith("installed skill paths")),
        [message],
      );
    });
  }
});
test("probe environment forwards only the allowlisted variables", async () => {
  const forwarded = Object.fromEntries(
    [
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
      "GH_TOKEN",
      "GITHUB_TOKEN",
      "XDG_CONFIG_HOME",
      "APPDATA",
      "LOCALAPPDATA",
    ].map((name) => [name, `value-of-${name}`]),
  );
  let seen: Record<string, string> | undefined;
  await run(
    ".",
    {
      short: false,
      fast: false,
      diagnostics() {},
      env: { ...forwarded, UNLISTED_VARIABLE: "dropped" },
    },
    {
      lookPath: () => "copilot-fixture",
      execute: (_exe, _args, options) => {
        seen = options.env;
        return { status: 1, output: "" };
      },
    },
  );
  const { COPILOT_HOME, COPILOT_CACHE_HOME, ...rest } = seen!;
  assert.ok(COPILOT_HOME);
  assert.ok(COPILOT_CACHE_HOME);
  assert.deepEqual(rest, forwarded);
});

/** Runs the probes against a CLI whose every step succeeds and whose plugin list prints `output`; nothing is installed. */
async function probeWithPluginList(output: string) {
  const names: string[] = [];
  const envs: Record<string, string>[] = [];
  const result = await run(
    ".",
    { short: false, fast: false, diagnostics() {}, env: {} },
    {
      lookPath: (name) => {
        names.push(name);
        return "copilot-fixture";
      },
      execute: (_exe, args, options) => {
        envs.push(options.env);
        return {
          status: 0,
          output: args.join(" ") === "plugin list --json" ? output : "[]",
        };
      },
    },
  );
  return { result, names, env: envs[0]! };
}

const installFailure = (message: string) => ({
  failures: [{ rule: "probe-copilot-install", path: "", message }],
});

test("plugin list output that is not a JSON array of plugins is unparseable", async () => {
  const notJson = (await probeWithPluginList("not json")).result;
  assert.equal(notJson.failures.length, 1);
  assert.match(
    notJson.failures[0]!.message,
    /^copilot plugin list --json output unparseable: invalid JSON \(/,
  );
  for (const [output, message] of [
    ["{}", "copilot plugin list --json output unparseable: expected an array"],
    [
      '"agentics"',
      "copilot plugin list --json output unparseable: expected an array",
    ],
    [
      '[{"name":42}]',
      "copilot plugin list --json output unparseable: plugin field name must be a string",
    ],
    // A JSON null lists no plugins.
    ["null", "copilot plugin list --json has no agentics@muehz entry"],
    [
      '[{"name":"agentics","marketplace":"other","source":"marketplace"}]',
      "copilot plugin list --json has no agentics@muehz entry",
    ],
  ] as const) {
    assert.deepEqual(
      (await probeWithPluginList(output)).result,
      installFailure(message),
      output,
    );
  }
});

test("a live install with no root, or a root that does not exist, is not the audited checkout", async (t) => {
  const missing = path.join(tmpdir(), "agentics-probe-no-such-root");
  for (const [entry, loads] of [
    [{}, ""],
    [{ installedFrom: missing }, missing],
  ] as const) {
    await t.test(JSON.stringify(entry), async () => {
      const output = JSON.stringify([
        { name: "agentics", marketplace: "muehz", source: "live", ...entry },
      ]);
      assert.deepEqual(
        (await probeWithPluginList(output)).result,
        installFailure(
          `live install loads ${JSON.stringify(loads)}, not the audited checkout .`,
        ),
      );
    });
  }
});

test("a marketplace install the CLI did not place in the probe home is not found", async () => {
  const { result, env } = await probeWithPluginList(
    '[{"name":"agentics","marketplace":"muehz","source":"marketplace"}]',
  );
  assert.deepEqual(
    result,
    installFailure(
      `Copilot installed root not found: ${path.join(env.COPILOT_HOME!, "installed-plugins", "muehz", "agentics")}`,
    ),
  );
});

test("the probe looks up copilot and gives it separate home and cache directories under the system temp directory", async () => {
  const { names, env } = await probeWithPluginList("null");
  assert.deepEqual(names, ["copilot"]);
  const { COPILOT_HOME: home, COPILOT_CACHE_HOME: cache, ...rest } = env;
  // Nothing the caller left unset is forwarded, not even as an empty entry.
  assert.deepEqual(rest, {});
  const inside = (parent: string, child: string): boolean => {
    const relative = path.relative(parent, child);
    return (
      relative !== "" &&
      !relative.startsWith("..") &&
      !path.isAbsolute(relative)
    );
  };
  assert.ok(inside(tmpdir(), home!), String(home));
  assert.ok(inside(tmpdir(), cache!), String(cache));
  assert.equal(inside(home!, cache!) || inside(cache!, home!), false);
  assert.notEqual(home, cache);
});

test("a missing copilot reports no failures alongside its error", async () => {
  assert.deepEqual(
    await run(
      ".",
      { short: false, fast: false, diagnostics() {}, env: {} },
      {
        lookPath: () => undefined,
        execute: () => {
          throw new Error("must not execute");
        },
      },
    ),
    {
      failures: [],
      error:
        "probes: copilot is required; pass allow-missing to skip unavailable provider CLIs",
    },
  );
});

test("a null plugin field reads as an absent one", async (t) => {
  const f = fixture(t);
  f.setList([
    {
      name: "agentics",
      marketplace: "muehz",
      source: "marketplace",
      installedFrom: null,
    },
  ]);
  assert.deepEqual(await f.audit(), { failures: [] });
});

const skillMessages = (result: {
  failures: { rule: string; message: string }[];
}): string[] =>
  result.failures
    .filter((failure) => failure.rule === "probe-copilot-skills")
    .map((failure) => failure.message);

test("an installed Copilot manifest that is not an object is unreadable, and a null one declares nothing", async (t) => {
  const unreadable =
    "installed .github/plugin/plugin.json unreadable: expected a JSON object";
  for (const [text, message] of [
    ["[]", unreadable],
    ["5", unreadable],
    [
      "null",
      'installed manifest does not match source manifest; missing=["skills/alpha","skills/beta"], extra=[]',
    ],
  ] as const) {
    await t.test(text, async (t) => {
      const f = fixture(t);
      f.raw[".github/plugin/plugin.json"] = text;
      const result = await f.audit();
      assert.equal(result.error, undefined);
      assert.ok(
        skillMessages(result).includes(message),
        JSON.stringify(result),
      );
    });
  }
});

test("a source skill entry that is not a string is ignored", async (t) => {
  const f = fixture(t);
  f.sourceManifest.skills = ["./skills/alpha", 5, "./skills/beta"];
  assert.deepEqual(await f.audit(), { failures: [] });
});

test("an installed manifest with an extra skill does not match the source", async (t) => {
  const f = fixture(t);
  f.manifest.skills = ["./skills/alpha", "./skills/beta", "./skills/gamma"];
  const messages = skillMessages(await f.audit());
  assert.ok(
    messages.includes(
      'installed manifest does not match source manifest; missing=[], extra=["skills/gamma"]',
    ),
    JSON.stringify(messages),
  );
  assert.ok(
    messages.includes(
      "expected effective installed manifest to declare exactly 2 skills, got 3",
    ),
    JSON.stringify(messages),
  );
});

test("missing skills are listed in sorted order", async (t) => {
  const f = fixture(t);
  f.sourceManifest.skills = [
    "./skills/alpha",
    "./skills/beta",
    "./skills/gamma",
    "./skills/delta",
  ];
  const messages = skillMessages(await f.audit());
  assert.ok(
    messages.includes(
      'installed manifest does not match source manifest; missing=["skills/delta","skills/gamma"], extra=[]',
    ),
    JSON.stringify(messages),
  );
});

test("a listed skill outside the installed skills directory is not an Agentics skill", async (t) => {
  const f = fixture(t);
  f.listed[1]!.path = path.join(f.installed, "other", "beta");
  const messages = skillMessages(await f.audit());
  assert.ok(
    messages.includes(
      "expected exactly 2 enabled installed Agentics plugin skills, got 1",
    ),
    JSON.stringify(messages),
  );
});

test("a listed skill whose path is not a string is not an Agentics skill", async (t) => {
  const f = fixture(t);
  f.setList([
    {
      name: "agentics",
      marketplace: "muehz",
      source: "live",
      installedFrom: f.source,
    },
  ]);
  f.setSkillOutput(
    JSON.stringify([
      ...f.listed.map((skill) => ({
        ...skill,
        path: skill.path.replace(f.installed, f.source),
      })),
      { source: "plugin", enabled: true, path: 5 },
    ]),
  );
  assert.deepEqual(await f.audit(), { failures: [] });
});

test("a manifest with no hooks entry names the empty value it found", async (t) => {
  const f = fixture(t);
  delete f.manifest.hooks;
  const result = await f.audit();
  assert.deepEqual(
    result.failures.filter((failure) => failure.rule === "probe-copilot-hooks"),
    [
      {
        rule: "probe-copilot-hooks",
        path: ".github/plugin/plugin.json",
        message:
          'effective installed hooks must be ./hooks/hooks.copilot.json, got ""',
      },
    ],
  );
});

test("only the Claude manifest is reported missing by the install probe itself", async (t) => {
  const f = fixture(t);
  f.raw[".claude-plugin/marketplace.json"] = null;
  const result = await f.audit();
  assert.equal(result.error, undefined);
  assert.equal(
    result.failures.some((failure) =>
      failure.message.startsWith(
        "installed .claude-plugin/marketplace.json is missing or unreadable",
      ),
    ),
    false,
    JSON.stringify(result),
  );
});

test("a skill list with an object and then a null row is unparseable", async (t) => {
  const f = fixture(t);
  f.setSkillOutput("[{}, null]");
  assert.deepEqual(skillMessages(await f.audit()), [
    "copilot skill list --json output unparseable: expected an array of skill objects",
  ]);
});

test("a source Copilot manifest that is not an object is unreadable", async (t) => {
  const base = mkdtempSync(path.join(tmpdir(), "agentics-probe-test-"));
  t.after(() => rmSync(base, { recursive: true, force: true }));
  const source = path.join(base, "source");
  mkdirSync(path.join(source, ".github", "plugin"), { recursive: true });
  writeFileSync(path.join(source, ".github", "plugin", "plugin.json"), "[]");
  const result = await run(
    source,
    { short: false, fast: false, diagnostics() {}, env: {} },
    {
      lookPath: () => "copilot-fixture",
      execute: (_exe, args, options) => {
        if (args.join(" ") !== "plugin list --json")
          return { status: 0, output: "[]" };
        // The CLI places the install in the probe home; its manifest is a valid object.
        const installed = path.join(
          options.env.COPILOT_HOME!,
          "installed-plugins",
          "muehz",
          "agentics",
        );
        mkdirSync(path.join(installed, ".github", "plugin"), {
          recursive: true,
        });
        writeFileSync(
          path.join(installed, ".github", "plugin", "plugin.json"),
          JSON.stringify({ name: "agentics", skills: ["./skills/alpha"] }),
        );
        return {
          status: 0,
          output: JSON.stringify([
            { name: "agentics", marketplace: "muehz", source: "marketplace" },
          ]),
        };
      },
    },
  );
  assert.equal(result.error, undefined);
  assert.ok(
    result.failures.some(
      (failure) =>
        failure.rule === "probe-copilot-skills" &&
        failure.message ===
          "source .github/plugin/plugin.json unreadable: expected a JSON object",
    ),
    JSON.stringify(result),
  );
});

test("a CLI that throws is an audit error, and the probe home is still removed", async () => {
  let home = "";
  const result = await run(
    ".",
    { short: false, fast: false, diagnostics() {}, env: {} },
    {
      lookPath: () => "copilot-fixture",
      execute: (_exe, _args, options) => {
        home = options.env.COPILOT_HOME!;
        throw new Error("cli crashed");
      },
    },
  );
  assert.deepEqual(result, { failures: [], error: "probes: cli crashed" });
  assert.equal(existsSync(home), false);
});
