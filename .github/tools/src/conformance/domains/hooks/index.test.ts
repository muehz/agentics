import assert from "node:assert/strict";
import { mkdirSync, mkdtempSync, readFileSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { test, type TestContext } from "node:test";
import type { RunOptions } from "../../check.ts";
import { main } from "../../run.ts";
import { closeError, run } from "./index.ts";
import { removeTree } from "../../testing/host.ts";

const clean =
  '{"version":1,"areas":[{"name":"ascii","failures":[],"error":null},{"name":"schema","failures":[],"error":null},{"name":"module","failures":[],"error":null},{"name":"freshness","failures":[],"error":null}]}';
const finding = {
  rule: "rule-exact",
  path: "dir/file",
  message: "exact message",
};
const violation = JSON.stringify(finding);
const withFinding = clean.replace('"failures":[]', `"failures":[${violation}]`);
const broken = withFinding.replace('"error":null', '"error":"read failed"');

function fixture(t: TestContext, output = clean, exit = "", notice = "") {
  const root = mkdtempSync(path.join(tmpdir(), "agentics-hookcommand-"));
  t.after(() => removeTree(root));
  let diagnostics = "";
  const options: RunOptions = {
    short: false,
    fast: false,
    env: {
      ...process.env,
      AGX_HOOK_VERIFIER: process.execPath,
      NODE_OPTIONS: `--import=${new URL("./testing/fake-verifier.ts", import.meta.url).href}`,
      HOOK_TEST_OUTPUT: output,
      HOOK_TEST_EXIT: exit,
      HOOK_TEST_NOTICE: notice,
      HOOK_TEST_ARGS: path.join(root, "args"),
    },
    diagnostics: (text) => {
      diagnostics += text;
    },
  };
  return { root, options, diagnostics: () => diagnostics };
}

test("the verifier runs as audit --repo <root> and a clean document has no failures", async (t) => {
  const f = fixture(t);
  assert.deepEqual(await run(f.root, f.options), { failures: [] });
  assert.equal(
    readFileSync(f.options.env.HOOK_TEST_ARGS!, "utf8"),
    `${path.resolve("audit")}\n--repo\n${f.root}`,
  );
});

test("fast mode passes --skip-tests to the verifier", async (t) => {
  const f = fixture(t);
  assert.deepEqual(await run(f.root, { ...f.options, fast: true }), {
    failures: [],
  });
  assert.equal(
    readFileSync(f.options.env.HOOK_TEST_ARGS!, "utf8"),
    `${path.resolve("audit")}\n--repo\n${f.root}\n--skip-tests`,
  );
  assert.deepEqual(await run(f.root, { ...f.options, fast: false }), {
    failures: [],
  });
  assert.equal(
    readFileSync(f.options.env.HOOK_TEST_ARGS!, "utf8"),
    `${path.resolve("audit")}\n--repo\n${f.root}`,
  );
});

test("hook command protocol", async (t) => {
  const cases: [string, string, string, string][] = [
    ["clean", clean, "", ""],
    ["malformed", "{", "", "document:"],
    [
      "version_missing",
      clean.replace('"version":1,', ""),
      "",
      "version must be 1",
    ],
    [
      "version_unknown",
      clean.replace('"version":1', '"version":2'),
      "",
      "version must be 1",
    ],
    [
      "version_wrong_type",
      clean.replace('"version":1', '"version":"1"'),
      "",
      "document:",
    ],
    ["areas_missing", '{"version":1}', "", "expected exactly four areas"],
    [
      "area_missing",
      clean.replace(',{"name":"freshness","failures":[],"error":null}', ""),
      "",
      "expected exactly four areas",
    ],
    [
      "area_extra",
      clean.replace("]}", ',{"name":"extra","failures":[],"error":null}]}'),
      "",
      "expected exactly four areas",
    ],
    [
      "area_unknown",
      clean.replace('"ascii"', '"unknown"'),
      "",
      "area 0 must be ascii",
    ],
    [
      "area_duplicate",
      clean.replace('"schema"', '"ascii"'),
      "",
      "area 1 must be schema",
    ],
    [
      "area_out_of_order",
      clean.replace('"ascii"', '"schema"'),
      "",
      "area 0 must be ascii",
    ],
    [
      "name_missing",
      clean.replace('"name":"ascii",', ""),
      "",
      "area 0 must be ascii",
    ],
    [
      "failures_missing",
      clean.replace('"failures":[],', ""),
      "",
      "missing failures array",
    ],
    [
      "failures_null",
      clean.replace('"failures":[]', '"failures":null'),
      "",
      "missing failures array",
    ],
    [
      "failures_type",
      clean.replace('"failures":[]', '"failures":{}'),
      "",
      "document:",
    ],
    ["error_missing", clean.replace(',"error":null', ""), "", "missing error"],
    [
      "error_type",
      clean.replace('"error":null', '"error":false'),
      "",
      "error must be string or null",
    ],
    [
      "failure_missing_field",
      clean.replace('"failures":[]', '"failures":[{"rule":"r","path":"p"}]'),
      "",
      "failure missing string field",
    ],
    [
      "failure_wrong_type",
      clean.replace(
        '"failures":[]',
        '"failures":[{"rule":5,"path":"p","message":"m"}]',
      ),
      "",
      "document:",
    ],
    ["trailing", clean + " {}", "", "trailing output"],
    [
      "exit_zero_error",
      clean.replace('"error":null', '"error":"broken"'),
      "",
      "errors with exit 0",
    ],
    ["nonzero", clean, "1", "hook audit exited with code 1"],
    ["nonzero_malformed", "", "1", "invalid hook audit result"],
  ];
  for (const [name, output, exit, want] of cases) {
    await t.test(name, async (t) => {
      const f = fixture(t, output, exit);
      const result = await run(f.root, f.options);
      if (want === "") assert.deepEqual(result, { failures: [] });
      else {
        assert.ok(result.error?.includes(want), JSON.stringify(result));
        assert.deepEqual(result.failures, []);
      }
    });
  }
});

test("hook command errors preserve findings", async (t) => {
  for (const exit of ["", "1"]) {
    const f = fixture(t, broken, exit);
    const result = await run(f.root, f.options);
    assert.match(result.error!, /read failed/);
    assert.deepEqual(result.failures, [finding]);
  }
});

test("explicit absolute verifier and repository arguments", async (t) => {
  const f = fixture(t);
  const root = path.join(f.root, "candidate with spaces");
  assert.deepEqual(await run(root, f.options), { failures: [] });
  assert.equal(
    readFileSync(f.options.env.HOOK_TEST_ARGS!, "utf8"),
    `${path.resolve("audit")}\n--repo\n${root}`,
  );
  f.options.env.AGX_HOOK_VERIFIER = path.join(f.root, "missing-verifier");
  assert.match(
    (await run(root, f.options)).error!,
    /^hook audit could not start: /,
  );
  for (const verifier of [
    "relative-verifier",
    "",
    ...(process.platform === "win32" ? ["\\root-relative"] : []),
  ]) {
    f.options.env.AGX_HOOK_VERIFIER = verifier;
    assert.match(
      (await run(root, f.options)).error!,
      /verifier and repository paths must be absolute/,
    );
  }
  f.options.env.AGX_HOOK_VERIFIER = process.execPath;
  for (const root of [
    "relative-root",
    "",
    ...(process.platform === "win32" ? ["\\root-relative"] : []),
  ]) {
    assert.match(
      (await run(root, f.options)).error!,
      /verifier and repository paths must be absolute/,
    );
  }
});

test("hook command returns exact findings", async (t) => {
  const f = fixture(t, withFinding);
  assert.deepEqual(await run(f.root, f.options), { failures: [finding] });
});

test("every failure string is required and a later invalid area discards earlier findings", async (t) => {
  for (const key of ["rule", "path", "message"] as const) {
    for (const replacement of [undefined, null, 42]) {
      const invalid = { ...finding, [key]: replacement };
      const f = fixture(
        t,
        clean.replace(
          '"failures":[]',
          `"failures":[${JSON.stringify(invalid)}]`,
        ),
      );
      const result = await run(f.root, f.options);
      assert.match(
        result.error!,
        replacement === 42
          ? /document: expected string/
          : /failure missing string field/,
      );
      assert.deepEqual(result.failures, []);
    }
  }
  const f = fixture(
    t,
    withFinding.replace(
      '"name":"schema","failures":[],"error":null',
      '"name":"schema","failures":[]',
    ),
  );
  const result = await run(f.root, f.options);
  assert.match(result.error!, /area schema missing error/);
  assert.deepEqual(result.failures, []);
});

test("the candidate checkout cannot select a missing environment verifier", async (t) => {
  const f = fixture(t);
  mkdirSync(path.join(f.root, ".claude-plugin"));
  writeFileSync(
    path.join(f.root, ".claude-plugin/plugin.json"),
    JSON.stringify({ AGX_HOOK_VERIFIER: process.execPath }),
  );
  delete f.options.env.AGX_HOOK_VERIFIER;
  assert.deepEqual(await run(f.root, f.options), {
    failures: [],
    error: "verifier and repository paths must be absolute",
  });
});

test("successful hook diagnostics reach the supplied writer", async (t) => {
  const notice = "SKIP: Windows notice\n";
  const f = fixture(t, clean, "", notice);
  assert.deepEqual(await run(f.root, f.options), { failures: [] });
  assert.equal(f.diagnostics(), notice);
});

test("hook diagnostic writer failure is an audit error", async (t) => {
  const f = fixture(t, clean, "", "SKIP: Windows notice\n");
  f.options.diagnostics = () => {
    throw new Error("diagnostic write failed");
  };
  assert.match(
    (await run(f.root, f.options)).error!,
    /write hook audit diagnostics: diagnostic write failed/,
  );
});

test("hook errors retain successful diagnostics and combined writer errors", async (t) => {
  for (const [output, want, findings] of [
    ["{", /invalid hook audit result/, []],
    [broken, /read failed/, [finding]],
  ] as const) {
    const f = fixture(t, output, "", "SKIP: exact notice\n");
    let result = await run(f.root, f.options);
    assert.match(result.error!, want);
    assert.equal(f.diagnostics(), "SKIP: exact notice\n");
    assert.deepEqual(result.failures, findings);
    f.options.diagnostics = () => {
      throw new Error("diagnostic write failed");
    };
    result = await run(f.root, f.options);
    assert.match(result.error!, want);
    assert.match(result.error!, /diagnostic write failed/);
    assert.deepEqual(result.failures, findings);
  }
});

test("duplicate and case-folded keys are rejected throughout the document", async (t) => {
  for (const upper of [false, true]) {
    const key = (s: string) => (upper ? s.toUpperCase() : s);
    const outputs = [
      clean.replace('"version":1', `"version":2,"${key("version")}":1`),
      clean.replace('"error":null', `"error":"hidden","${key("error")}":null`),
      clean.replace(
        '"failures":[]',
        `"failures":[${violation}],"${key("failures")}":[]`,
      ),
      clean.replace(
        '"failures":[]',
        `"failures":[{"rule":"hidden","${key("rule")}":"r","path":"p","message":"m"}]`,
      ),
    ];
    for (const output of outputs) {
      const f = fixture(t, output);
      const result = await run(f.root, f.options);
      assert.match(result.error!, /duplicate object key/);
      assert.deepEqual(result.failures, []);
    }
  }
  for (const output of [
    clean.replace('"failures":[]', `"failures":[${violation}],"failureſ":[]`),
    clean.replace('"version":1', '"version":1,"extra":{"K":1,"k":2}'),
    clean.replace('"version":1', '"version":1,"extra":[{"x":1,"x":2}]'),
  ]) {
    const f = fixture(t, output);
    const result = await run(f.root, f.options);
    assert.match(result.error!, /duplicate object key/);
    assert.deepEqual(result.failures, []);
  }
});

test("keys are compared and version is recognised after escape decoding", async (t) => {
  for (const [output, want] of [
    [
      clean.replace(
        '"failures":[]',
        `"failures":[${violation}],"\\u0066ailures":[]`,
      ),
      /duplicate object key/,
    ],
    [
      clean.replace(
        '"failures":[]',
        `"failures":[${violation}],"\\u0046ailures":[]`,
      ),
      /duplicate object key/,
    ],
    [
      clean.replace('"version":1', '"\\u0076ersion":1.0'),
      /version must be an integer/,
    ],
  ] as const) {
    // The verifier prints the escape itself, not the letter it stands for.
    assert.match(output, /\\u00/);
    for (const exit of ["", "1"]) {
      const f = fixture(t, output, exit);
      const result = await run(f.root, f.options);
      assert.match(result.error!, /^invalid hook audit result: document: /);
      assert.match(result.error!, want);
      assert.deepEqual(result.failures, []);
    }
  }
});

test("invalid nonzero output retains exit diagnostics without leaking findings", async (t) => {
  for (const output of [
    "",
    `{"version":1,"areas":[{"failures":[${violation}]}]}`,
  ]) {
    const f = fixture(t, output, "1");
    const result = await run(f.root, f.options);
    assert.match(result.error!, /invalid hook audit result/);
    assert.match(result.error!, /hook audit exited with code 1/);
    assert.match(result.error!, /fixture diagnostic/);
    assert.deepEqual(result.failures, []);
    assert.equal(f.diagnostics(), "");
  }
});

test("unknown fields, whitespace and empty strings are accepted with exact-case keys", async (t) => {
  const output = withFinding
    .replace('"rule-exact"', '""')
    .replace('"version":1', '"version":1,"extra":true');
  const f = fixture(t, ` \r\n${output}\t\n`);
  assert.deepEqual(await run(f.root, f.options), {
    failures: [{ ...finding, rule: "" }],
  });
});

test("a key written in another case is a missing key", async (t) => {
  for (const [output, want] of [
    [withFinding.replace('"version"', '"VERSION"'), "version must be 1"],
    [
      withFinding.replace('"failures"', '"Failures"'),
      "area ascii missing failures array",
    ],
    [withFinding.replace('"error"', '"Error"'), "area ascii missing error"],
    [
      withFinding.replace('"rule"', '"Rule"'),
      "area ascii failure missing string field",
    ],
  ] as const) {
    const f = fixture(t, output);
    assert.deepEqual(
      await run(f.root, f.options),
      { failures: [], error: `invalid hook audit result: ${want}` },
      output,
    );
  }
});

test("a number too large for a double in an unread field is not an audit error", async (t) => {
  const f = fixture(
    t,
    withFinding.replace('"version":1', '"version":1,"extra":1e400'),
  );
  assert.deepEqual(await run(f.root, f.options), { failures: [finding] });
});

test("integer version and structured field types are enforced", async (t) => {
  for (const output of [
    clean.replace('"version":1', '"version":1.0'),
    clean.replace('"version":1', '"version":1e0'),
    clean.replace('"name":"ascii"', '"name":false'),
    clean.replace('"areas":[', '"areas":{ "ignored":[').replace(/]}$/, "]}}"),
    clean.replace('"failures":[]', '"failures":[false]'),
  ]) {
    const f = fixture(t, output);
    assert.match((await run(f.root, f.options)).error!, /document:/);
  }
  const f = fixture(t, clean.replace('"error":null', '"error":""'));
  assert.match(
    (await run(f.root, f.options)).error!,
    /errors with exit 0: ascii:/,
  );
});

test("CLI hooks seam uses host environment and renders PASS, FAIL and ERROR", async (t) => {
  const previous = process.env.AGX_HOOK_VERIFIER;
  process.env.AGX_HOOK_VERIFIER = "process-environment-must-not-be-used";
  t.after(() => {
    if (previous === undefined) delete process.env.AGX_HOOK_VERIFIER;
    else process.env.AGX_HOOK_VERIFIER = previous;
  });
  for (const [output, exit, expectedStatus] of [
    [clean, "", 0],
    [withFinding, "", 1],
    [broken, "1", 1],
  ] as const) {
    const f = fixture(t, output, exit, "SKIP: exact notice\n");
    mkdirSync(path.join(f.root, ".claude-plugin"));
    writeFileSync(
      path.join(f.root, ".claude-plugin/plugin.json"),
      JSON.stringify({ AGX_HOOK_VERIFIER: "candidate-controlled" }),
    );
    let stdout = "";
    let stderr = "";
    const status = await main(
      ["all", "--repo", f.root],
      {
        cwd: f.root,
        env: f.options.env,
        out: (text) => {
          stdout += text;
        },
        err: (text) => {
          stderr += text;
        },
      },
      { hooks: run },
    );
    assert.equal(status, expectedStatus);
    if (expectedStatus === 0) {
      assert.equal(stdout, "PASS hooks\n");
      assert.match(stderr, /SKIP: exact notice/);
    } else {
      assert.equal(stdout, "");
      assert.match(
        stderr,
        /FAIL hooks \[rule-exact\] dir\/file: exact message/,
      );
      if (exit === "1") {
        assert.match(
          stderr,
          /ERROR hooks: audit could not run: hook audit exited with code 1/,
        );
        assert.match(stderr, /ascii: read failed/);
        assert.match(stderr, /fixture diagnostic/);
        assert.equal(stderr.split("SKIP: exact notice").length - 1, 1);
      } else {
        assert.match(stderr, /hooks: 1 failure\(s\)/);
        assert.doesNotMatch(stderr, /ERROR hooks/);
      }
    }
  }
});

test("document boundary and version spelling follow the exact JSON tokens", async (t) => {
  const integer =
    "invalid hook audit result: document: version must be an integer";
  for (const [output, want] of [
    // After an escaped backslash the brace is still string text, not the document's end.
    [clean.replace('"version":1', '"version":1,"extra":"\\\\}"'), undefined],
    [clean.replace('"version":1', '"version" : 1'), undefined],
    [clean.replace('"version":1', '"version":1e+0'), integer],
    [clean.replace('"version":1', '"version":1E-0'), integer],
    // Only the version member's own number decides its spelling.
    [clean.replace('"version":1', '"version":1.0,"y":1'), integer],
    [
      clean.replace('"version":1', '"version":1.0,"x":"version","y":1'),
      integer,
    ],
    [
      clean.replace(/}$/, ',"extra":0}') + "5",
      "invalid hook audit result: trailing output",
    ],
    [
      clean.replace('"error":null', '"error":"broken"'),
      "hook audit returned errors with exit 0: ascii: broken",
    ],
  ] as const) {
    const f = fixture(t, output);
    assert.deepEqual(
      await run(f.root, f.options),
      want === undefined ? { failures: [] } : { failures: [], error: want },
      output,
    );
  }
});

test("a verifier that closes with neither an exit code nor a signal fails the audit", () => {
  assert.equal(closeError(null, null), "hook audit exited with code -1");
  assert.equal(closeError(0, null), undefined);
});

test("a verifier killed by a signal says which signal", () => {
  assert.equal(closeError(null, "SIGKILL"), "hook audit was killed by SIGKILL");
  assert.equal(closeError(1, "SIGTERM"), "hook audit was killed by SIGTERM");
});

test("each invalid document gives its own exact audit error", async (t) => {
  const integer =
    "invalid hook audit result: document: version must be an integer";
  for (const [output, want] of [
    ["null", "invalid hook audit result: version must be 1"],
    [
      clean.replace('"version":1', '"version":null'),
      "invalid hook audit result: version must be 1",
    ],
    [
      clean.replace('"version":1', '"version":10'),
      "invalid hook audit result: version must be 1",
    ],
    [clean.replace('"version":1', '"version":"1"'), integer],
    [
      clean.replace('"name":"ascii"', '"name":false'),
      "invalid hook audit result: document: expected string",
    ],
    // Only the document's own version member is read; a nested one is an unknown field.
    [
      clean.replace('"version":1', '"version":1,"extra":{"version":1.0}'),
      undefined,
    ],
  ] as const) {
    const f = fixture(t, output);
    assert.deepEqual(
      await run(f.root, f.options),
      want === undefined ? { failures: [] } : { failures: [], error: want },
      output,
    );
  }
});

test("a key outside any object is invalid JSON, not a scan failure", async (t) => {
  for (const output of ['"version": 1', '["version": 1]']) {
    const f = fixture(t, output);
    const result = await run(f.root, f.options);
    assert.match(
      result.error!,
      /^invalid hook audit result: document: invalid JSON \(/,
      output,
    );
    assert.deepEqual(result.failures, []);
  }
});

test("a verifier that cannot start gives no failures and the start error", async (t) => {
  const f = fixture(t);
  f.options.env.AGX_HOOK_VERIFIER = path.join(f.root, "missing-verifier");
  const missing = await run(f.root, f.options);
  assert.deepEqual(missing.failures, []);
  assert.match(missing.error!, /^hook audit could not start: .*ENOENT/);
  // A path Node refuses before starting anything is a start error too.
  f.options.env.AGX_HOOK_VERIFIER = `${process.execPath}\0`;
  const refused = await run(f.root, f.options);
  assert.deepEqual(refused.failures, []);
  assert.match(refused.error!, /^hook audit could not start: .*null bytes/);
});

test("a verifier with no diagnostics never calls the writer", async (t) => {
  const f = fixture(t);
  f.options.diagnostics = () => {
    throw new Error("diagnostic write failed");
  };
  assert.deepEqual(await run(f.root, f.options), { failures: [] });
});

test(
  "a verifier that reads its input sees it closed, so the audit does not wait on it",
  { timeout: 30_000 },
  async (t) => {
    const f = fixture(t);
    const reader = [
      "await new Promise((done) => process.stdin.on('end', done).resume());",
      "process.stdout.write(process.env.HOOK_TEST_OUTPUT);",
      "process.exit(0);",
    ].join("");
    f.options.env.NODE_OPTIONS = `--import=data:text/javascript,${encodeURIComponent(reader)}`;
    assert.deepEqual(await run(f.root, f.options), { failures: [] });
  },
);

test("audit errors join area errors, the exit and trimmed diagnostics in a fixed form", async (t) => {
  const twoErrors = clean
    .replace('"error":null', '"error":"a"')
    .replace('"error":null', '"error":"b"');
  const exact = async (
    output: string,
    exit: string,
    notice: string,
    want: { failures: unknown[]; error: string },
    writerFails = false,
  ): Promise<void> => {
    const f = fixture(t, output, exit, notice);
    if (writerFails)
      f.options.diagnostics = () => {
        throw new Error("diagnostic write failed");
      };
    assert.deepEqual(await run(f.root, f.options), want);
  };
  await exact(twoErrors, "", "", {
    failures: [],
    error: "hook audit returned errors with exit 0: ascii: a; schema: b",
  });
  await exact(twoErrors, "1", "  note\n", {
    failures: [],
    error:
      "hook audit exited with code 1; ascii: a; schema: b; note\nfixture diagnostic",
  });
  await exact("null", "1", "  note\n", {
    failures: [],
    error:
      "invalid hook audit result: version must be 1; hook audit exited with code 1; note\nfixture diagnostic",
  });
  await exact(
    broken,
    "",
    "SKIP: exact notice\n",
    {
      failures: [finding],
      error:
        "hook audit returned errors with exit 0: ascii: read failed\nwrite hook audit diagnostics: diagnostic write failed",
    },
    true,
  );
});
