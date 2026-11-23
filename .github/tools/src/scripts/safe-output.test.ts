import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import test from "node:test";
import { fileURLToPath, pathToFileURL } from "node:url";
import { scratchDir } from "../release/testing/support.ts";
import { createClient, createGh } from "./backlog-grooming-client.ts";
import {
  readSafeOutput,
  runSafeGh,
  safeOutputRepo,
  type SafeGhRunner,
} from "./safe-output.ts";

const notes = "update_release_notes";
const grooming = "apply_grooming";
type Type = typeof notes | typeof grooming;
const repo = "owner/repo";
const secret = "ghp_" + "s".repeat(36);
const dir = scratchDir("agentics-safe-output-");
const envelope = join(dir, "output.json");
const log = join(dir, "calls.jsonl");
const baseline = join(dir, "baseline.md");
const preload = join(dir, "fake-gh.mjs");
writeFileSync(baseline, "baseline");

// Preload only in the CLI subprocess. Keeping its real spawn call lets
// the fixture observe the buffer cap and both output streams on every platform.
writeFileSync(
  preload,
  `
import cp from "node:child_process";
import { appendFileSync } from "node:fs";
import { syncBuiltinESMExports } from "node:module";
let body = "baseline";
let gets = 0;
cp.spawnSync = (command, args, options) => {
  if (command !== "gh") throw new Error("unexpected executable");
  if (args[1] === "graphql") {
    appendFileSync(process.env.CALLS, JSON.stringify({ operation: "graphql" }) + "\\n");
    return { status: Number(process.env.CHILD_STATUS), stdout: process.env.CHILD_OUT, stderr: process.env.CHILD_ERR };
  }
  const method = args.includes("PATCH") ? "PATCH" : "GET";
  if (method === "GET") gets++;
  const operation = method === "PATCH" ? "PATCH" : gets === 1 ? "release GET" : "verification GET";
  const bound = args.includes("repos/owner/repo/releases/42");
  let bodyOnly = false;
  if (method === "PATCH") {
    const payload = JSON.parse(options.input);
    bodyOnly = Object.keys(payload).join() === "body" && payload.body.includes("highlights");
    body = payload.body;
  }
  appendFileSync(process.env.CALLS, JSON.stringify({ operation, bound, bodyOnly, cap: options.maxBuffer }) + "\\n");
  if (!bound) return { status: 1, stderr: "unexpected resource" };
  if (operation === process.env.FAIL_OPERATION)
    return { status: Number(process.env.CHILD_STATUS), stdout: process.env.CHILD_OUT, stderr: process.env.CHILD_ERR };
  return { status: 0, stdout: JSON.stringify({
    id: 42, tag_name: "v1.2.3", name: "v1.2.3", draft: false, target_commitish: "a".repeat(40),
    prerelease: false, body
  }) };
};
syncBuiltinESMExports();
`,
);

function runCli(type: Type, env: NodeJS.ProcessEnv = {}) {
  const script = type === notes ? "release-notes" : "backlog-grooming";
  writeFileSync(log, "");
  const child = spawnSync(
    process.execPath,
    [
      "--import",
      pathToFileURL(preload).href,
      fileURLToPath(new URL(`./${script}.ts`, import.meta.url)),
      ...(type === grooming ? ["apply"] : []),
    ],
    {
      cwd: dir,
      encoding: "utf8",
      env: {
        SystemRoot: process.env.SystemRoot,
        PATH: dir,
        GH_REPO: repo,
        GITHUB_REPOSITORY: repo,
        GH_AW_AGENT_OUTPUT: envelope,
        CALLS: log,
        TARGET_SHA: "a".repeat(40),
        TAG: "v1.2.3",
        RELEASE_ID: "42",
        BASELINE_NOTES: baseline,
        RELEASE_PRERELEASE: "false",
        GITHUB_RUN_ID: "1",
        GITHUB_SERVER_URL: "https://example.test",
        NOTES_ENGINE: "engine",
        NOTES_MODEL: "model",
        ...env,
      },
    },
  );
  assert.ok(!child.error, "fixture CLI must launch");
  assert.equal(child.stdout === "", true, "CLI stdout must stay empty");
  const calls = readFileSync(log, "utf8").trim();
  return {
    exit: child.status,
    diagnostic: child.stderr.trimEnd(),
    calls: calls ? calls.split("\n").map((line) => JSON.parse(line)) : [],
  };
}

function message(run: () => unknown): string | undefined {
  try {
    run();
  } catch (error) {
    assert.ok(error instanceof Error);
    return error.message;
  }
  return undefined;
}

function compareDiagnostic(actual: string | undefined, expected: string) {
  // Keep captured text out of assertion output even if redaction regresses.
  assert.equal(actual?.includes(secret), false, "credential in diagnostic");
  assert.equal(
    actual === expected,
    true,
    `expected fixed diagnostic: ${expected}`,
  );
}

const staged = "safe outputs are staged; no API calls made";
const noNotes = "no matching update_release_notes item; no-op";
const noGrooming = "no apply_grooming item; no-op";
const arrayError = "GH_AW_AGENT_OUTPUT.items must be an array";
const objectError = "GH_AW_AGENT_OUTPUT is not a JSON object";
const fieldError = {
  [notes]: "update_release_notes body must be a nonempty string",
  [grooming]: "apply_grooming plan must be a string",
};

for (const type of [notes, grooming] as const) {
  const prefix = type === notes ? "release-notes" : "backlog-grooming";
  const noItem = type === notes ? noNotes : noGrooming;
  const field = type === notes ? "body" : "plan";
  const invalid = { type, [field]: 1 };
  const valid = { type, [field]: "highlights" };
  const rows: [string, unknown, string, NodeJS.ProcessEnv?][] = [
    [
      "staged before read",
      undefined,
      staged,
      { GH_AW_SAFE_OUTPUTS_STAGED: "true", GH_AW_AGENT_OUTPUT: "" },
    ],
    [
      "missing path",
      undefined,
      "GH_AW_AGENT_OUTPUT is required",
      { GH_AW_AGENT_OUTPUT: "" },
    ],
    [
      "only literal true stages",
      {},
      type === notes ? arrayError : noItem,
      { GH_AW_SAFE_OUTPUTS_STAGED: "TRUE" },
    ],
    ["null", null, type === notes ? arrayError : objectError],
    ["primitive", 5, type === notes ? arrayError : objectError],
    ["array root", [], type === notes ? arrayError : noItem],
    ["non-array items", { items: "bad" }, type === notes ? arrayError : noItem],
    ["empty", { items: [] }, noItem],
    [
      "unrelated items",
      { items: [null, true, {}, { type: "another" }] },
      noItem,
    ],
    ["invalid field", { items: [invalid] }, fieldError[type]],
    [
      "count before field",
      { items: [invalid, valid] },
      `multiple ${type} items`,
    ],
    [
      "invalid repo when staged",
      {},
      type === notes ? staged : "GH_REPO is invalid",
      { GH_REPO: "/bad", GH_AW_SAFE_OUTPUTS_STAGED: "true" },
    ],
    [
      "missing repo when unmatched",
      { items: [] },
      type === notes ? noItem : "GH_REPO is required",
      { GH_REPO: "" },
    ],
    [
      "invalid repo when matched",
      { items: [valid] },
      "GH_REPO is invalid",
      { GH_REPO: "/bad" },
    ],
    [
      "invalid repo and field",
      { items: [invalid] },
      type === notes ? fieldError[type] : "GH_REPO is invalid",
      { GH_REPO: "/bad" },
    ],
    [
      "workflow mismatch before or after staged mode",
      {},
      type === notes ? staged : "GH_REPO does not match GITHUB_REPOSITORY",
      { GITHUB_REPOSITORY: "other/repo", GH_AW_SAFE_OUTPUTS_STAGED: "true" },
    ],
    [
      "workflow mismatch before or after matching",
      { items: [] },
      type === notes ? noItem : "GH_REPO does not match GITHUB_REPOSITORY",
      { GITHUB_REPOSITORY: "other/repo" },
    ],
    [
      "matched item with a workflow mismatch",
      { items: [valid] },
      "GH_REPO does not match GITHUB_REPOSITORY",
      { GITHUB_REPOSITORY: "other/repo" },
    ],
  ];
  if (type === notes) {
    rows.push(
      [
        "release identity before repository mismatch",
        { items: [valid] },
        "RELEASE_ID must be a positive decimal number",
        {
          RELEASE_ID: "0",
          TARGET_SHA: "invalid",
          GITHUB_REPOSITORY: "other/repo",
        },
      ],
      [
        "target identity before repository mismatch",
        { items: [valid] },
        "TARGET_SHA must be exactly 40 lowercase hexadecimal characters",
        {
          TARGET_SHA: "invalid",
          TAG: "invalid",
          GITHUB_REPOSITORY: "other/repo",
        },
      ],
      [
        "tag identity before repository mismatch",
        { items: [valid] },
        "TAG must match vX.Y.Z",
        { TAG: "invalid", GITHUB_REPOSITORY: "other/repo" },
      ],
      ["whitespace body", { items: [{ type, body: " \n" }] }, fieldError[type]],
      [
        "UTF-8 over byte limit",
        { items: [{ type, body: "é".repeat(6001) }] },
        "update_release_notes body exceeds 12000 bytes",
      ],
    );
  }
  test(`${prefix} CLI preserves staging, envelope, count, field and repository precedence`, () => {
    for (const [name, raw, expected, override] of rows) {
      writeFileSync(envelope, JSON.stringify(raw ?? null));
      const result = runCli(type, override);
      assert.equal(
        result.exit,
        expected === staged || expected === noItem ? 0 : 1,
        name,
      );
      compareDiagnostic(result.diagnostic, `${prefix}: ${expected}`);
      assert.deepEqual(result.calls, [], name);
    }
  });
}

test("both CLIs reject malformed or unreadable input and the shared error omits its contents and path", () => {
  for (const type of [notes, grooming] as const) {
    for (const missing of [false, true]) {
      const path = missing ? join(dir, secret) : envelope;
      writeFileSync(envelope, secret);
      const env = { GH_AW_AGENT_OUTPUT: path };
      const result = runCli(type, env);
      const expected = missing
        ? "could not read GH_AW_AGENT_OUTPUT"
        : "GH_AW_AGENT_OUTPUT is not valid JSON";
      assert.equal(result.exit, 1);
      assert.deepEqual(result.calls, []);
      const prefix = type === notes ? "release-notes" : "backlog-grooming";
      compareDiagnostic(result.diagnostic, prefix + ": " + expected);
      compareDiagnostic(
        message(() => readSafeOutput(env, type)),
        expected,
      );
    }
  }
});

test("notes binds state before staged or unmatched output", () => {
  writeFileSync(envelope, '{"items":[]}');
  for (const staged of ["true", "false"]) {
    const result = runCli(notes, {
      RELEASE_STATE_FILE: "",
      GITHUB_REPOSITORY: "other/repo",
      GH_AW_SAFE_OUTPUTS_STAGED: staged,
    });
    assert.equal(result.exit, 1);
    compareDiagnostic(
      result.diagnostic,
      "release-notes: RELEASE_STATE_FILE is required",
    );
    assert.deepEqual(result.calls, []);
  }
});

test("matched items forward only the requested string, including a grooming plan the consumer must report", () => {
  const env = { GH_AW_AGENT_OUTPUT: envelope, GH_REPO: repo };
  for (const plan of ["", "not json", "{}"]) {
    writeFileSync(
      envelope,
      JSON.stringify({
        items: [{ type: grooming, plan, repo: "other/repo", staged: true }],
      }),
    );
    assert.deepEqual(readSafeOutput(env, grooming), {
      kind: "matched",
      value: plan,
    });
    const result = runCli(grooming);
    assert.equal(result.exit, 1);
    compareDiagnostic(result.diagnostic, "backlog-grooming: MODE is required");
    assert.deepEqual(result.calls, []);
  }
  const atLimit = "highlights" + "é".repeat(5995);
  assert.equal(Buffer.byteLength(atLimit), 12000);
  writeFileSync(
    envelope,
    JSON.stringify({
      items: [
        {
          type: notes,
          body: atLimit,
          release_id: "99",
          repo: "other/repo",
          staged: true,
        },
      ],
    }),
  );
  assert.deepEqual(readSafeOutput(env, notes), {
    kind: "matched",
    value: atLimit,
  });
  assert.equal(safeOutputRepo(env), repo);
  const result = runCli(notes);
  assert.equal(result.exit, 0);
  compareDiagnostic(
    result.diagnostic,
    "release-notes: release notes updated and verified",
  );
  assert.deepEqual(
    result.calls,
    ["release GET", "PATCH", "verification GET"].map((operation) => ({
      operation,
      bound: true,
      bodyOnly: operation === "PATCH",
      cap: 2 * 1024 * 1024,
    })),
  );
});

test("the shared repository check rejects workflow mismatches before caller effects", () => {
  for (const type of [notes, grooming] as const) {
    writeFileSync(
      envelope,
      JSON.stringify({ items: [{ type, body: "highlights", plan: "{}" }] }),
    );
    const env = {
      GH_AW_AGENT_OUTPUT: envelope,
      GH_REPO: repo,
      GITHUB_REPOSITORY: "other/repo",
    };
    const result = runCli(type, env);
    assert.equal(result.exit, 1);
    const prefix = type === notes ? "release-notes" : "backlog-grooming";
    compareDiagnostic(
      result.diagnostic,
      prefix + ": GH_REPO does not match GITHUB_REPOSITORY",
    );
    assert.deepEqual(result.calls, []);
  }
  assert.equal(
    safeOutputRepo({ GH_REPO: repo, GITHUB_REPOSITORY: repo }),
    repo,
  );
  assert.equal(safeOutputRepo({ GH_REPO: repo }), repo);
  compareDiagnostic(
    message(() => safeOutputRepo({ GH_REPO: repo, GITHUB_REPOSITORY: "" })),
    "GH_REPO does not match GITHUB_REPOSITORY",
  );
});

test("notes child output and failures retain their individual operation, JSON policy and cap", () => {
  writeFileSync(
    envelope,
    JSON.stringify({ items: [{ type: notes, body: "highlights" }] }),
  );
  for (const operation of [
    "release GET",
    "PATCH",
    "verification GET",
  ] as const) {
    for (const [status, stdout] of [
      [0, ""],
      [0, secret],
      [1, "{}"],
    ] as const) {
      const failure = status === 0 ? "returned invalid JSON" : "failed";
      const args = ["api", secret];
      let callOptions;
      const run: SafeGhRunner = (name, actual, options) => {
        callOptions = {
          gh: name === "gh",
          args: actual === args,
          input: options.input === secret,
          maxBuffer: options.maxBuffer,
          windowsHide: options.windowsHide,
          encoding: options.encoding,
        };
        return { status, stdout, stderr: secret };
      };
      const result = runCli(notes, {
        FAIL_OPERATION: operation,
        CHILD_STATUS: String(status),
        CHILD_OUT: stdout,
        CHILD_ERR: secret,
      });
      assert.equal(result.exit, 1);
      compareDiagnostic(
        result.diagnostic,
        `release-notes: ${operation} ${failure}`,
      );
      const last = result.calls.at(-1);
      assert.equal(last?.operation, operation);
      assert.equal(last?.cap, 2 * 1024 * 1024);
      compareDiagnostic(
        message(() => runSafeGh(operation, args, secret, run)),
        `${operation} ${failure}`,
      );
      assert.deepEqual(callOptions, {
        gh: true,
        args: true,
        input: true,
        maxBuffer: last.cap,
        windowsHide: true,
        encoding: "utf8",
      });
    }
  }
  assert.deepEqual(
    runSafeGh("PATCH", [], undefined, () => ({
      status: 0,
      stdout: '{"ok":true}',
    })),
    { ok: true },
  );
});

test("grooming child results agree on JSON, empty output and process options", () => {
  const args = ["api", "graphql"];
  for (const [stdout, expected] of [
    [" \n ", null],
    [' {"ok":true}\n', { ok: true }],
  ] as const) {
    let calls = 0;
    const run: SafeGhRunner = (command, actual, options) => {
      calls++;
      assert.equal(command, "gh");
      assert.ok(
        actual === args && options.input === secret,
        "caller controls command inputs",
      );
      assert.equal(options.maxBuffer, 64 * 1024 * 1024);
      assert.equal(options.windowsHide, true);
      assert.equal(options.encoding, "utf8");
      return { status: 0, stdout };
    };
    assert.deepEqual(createGh(run)(args, secret), expected);
    assert.deepEqual(runSafeGh("grooming", args, secret, run), expected);
    assert.equal(calls, 2);
  }
});

test("grooming reports only a fixed operation and recognized HTTP status", () => {
  const args = ["api", secret];
  const rows: [SafeGhRunner, string][] = [
    [
      () => ({ status: 1, stderr: `gh: ${secret} (HTTP 403)` }),
      "gh api failed: HTTP 403",
    ],
    [() => ({ status: 1, stderr: secret }), "gh api failed"],
    [
      () => ({ status: 1, stderr: "gh: Forbidden (HTTP 403)\nHTTP 404" }),
      "gh api failed: HTTP 403",
    ],
    [() => ({ status: 1, stderr: "HTTP 4040" }), "gh api failed"],
    [
      () => ({ status: 1, stderr: `cannot read ${secret}\nHTTP 404` }),
      "gh api failed",
    ],
    [() => ({ status: 0, stdout: secret }), "gh api returned invalid JSON"],
    [
      () => ({ status: null, stderr: "HTTP 404", error: new Error(secret) }),
      "gh api failed",
    ],
    [
      () => {
        throw new Error(secret);
      },
      "gh api failed",
    ],
  ];
  for (const [run, expected] of rows) {
    compareDiagnostic(
      message(() => createGh(run)(args, secret)),
      expected,
    );
    compareDiagnostic(
      message(() => runSafeGh("grooming", args, secret, run)),
      expected,
    );
  }
});

test("label lookup and creation use the authorized client and only a missing label causes POST", () => {
  const rows: [ReturnType<SafeGhRunner>, string[]][] = [
    [{ status: 0, stdout: "{}" }, ["GET"]],
    [{ status: 1, stderr: "gh: Not Found (HTTP 404)" }, ["GET", "POST"]],
    [
      {
        status: 1,
        stderr: "gh: Not Found (HTTP 404)\nSee the API documentation.",
      },
      ["GET", "POST"],
    ],
    [{ status: 1, stderr: "gh: Forbidden (HTTP 403)" }, ["GET"]],
    [{ status: 0, stdout: "invalid JSON" }, ["GET"]],
    [{ status: 1, stderr: "gh: Forbidden (HTTP 403)\nHTTP 404" }, ["GET"]],
    [{ status: 1, stderr: "HTTP 4040" }, ["GET"]],
    [
      { status: null, error: new Error("spawn failed"), stderr: "HTTP 404" },
      ["GET"],
    ],
  ];
  for (const [first, expected] of rows) {
    const calls: string[] = [];
    const run: SafeGhRunner = (_command, args, options) => {
      const method = args[2]!;
      calls.push(method);
      assert.equal(
        args[3],
        `repos/${repo}/labels${method === "GET" ? "/project%20label" : ""}`,
      );
      if (method === "GET") return first;
      assert.deepEqual(JSON.parse(String(options.input)), {
        name: "project label",
        color: "123456",
        description: "label",
      });
      return { status: 0, stdout: "" };
    };
    const gh = createGh(run);
    const failure = message(() =>
      createClient(gh, repo).ensureLabel("project label", "123456", "label"),
    );
    assert.deepEqual(calls, expected);
    assert.equal(
      failure === undefined,
      first.stdout === "{}" || expected.includes("POST"),
    );
  }
});

test("grooming CLI redacts child data and keeps the provider status on its exit path", () => {
  writeFileSync(
    envelope,
    JSON.stringify({ items: [{ type: grooming, plan: "invalid" }] }),
  );
  const meta = join(dir, "meta.json");
  writeFileSync(meta, "{}");
  for (const [status, output, stderr, expected] of [
    [1, "", "gh: " + secret + " (HTTP 403)", "gh api failed: HTTP 403"],
    [1, "", secret, "gh api failed"],
    [0, secret, "", "gh api returned invalid JSON"],
  ] as const) {
    const result = runCli(grooming, {
      MODE: "apply",
      FETCH_META: meta,
      CHILD_STATUS: String(status),
      CHILD_OUT: output,
      CHILD_ERR: stderr,
    });
    assert.equal(result.exit, 1);
    compareDiagnostic(result.diagnostic, "backlog-grooming: " + expected);
    assert.deepEqual(result.calls, [{ operation: "graphql" }]);
  }
});
