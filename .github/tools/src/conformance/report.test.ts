import assert from "node:assert/strict";
import { test } from "node:test";
import type { AuditResult, RunOptions } from "./check.ts";
import { DOMAIN_REGISTRATIONS } from "./registry.ts";
import { reportDomain } from "./report.ts";
import { fakeHost } from "./testing/fake-host.ts";

const options: RunOptions = {
  short: false,
  fast: false,
  diagnostics: () => {},
  env: {},
};
const result = (outcome: AuditResult, name = "") => ({
  ...DOMAIN_REGISTRATIONS[name],
  run: async () => outcome,
});

test("a domain with no failures prints PASS on stdout and returns 0", async () => {
  const host = fakeHost();
  assert.deepEqual(
    await reportDomain("tools", result({ failures: [] }), "/r", options, host),
    { status: 0, staleReviewOnly: false },
  );
  assert.equal(host.stdout(), "PASS tools\n");
  assert.equal(host.stderr(), "");
});

test("failures print FAIL lines and a count on stderr", async () => {
  const host = fakeHost();
  const run = result({
    failures: [
      {
        rule: "tools-module-vet",
        path: "hooks/go",
        message: "go vet ./... failed:\nbad",
      },
      { rule: "some-repo-rule", path: "", message: "no path here" },
    ],
  });
  assert.deepEqual(await reportDomain("tools", run, "/r", options, host), {
    status: 1,
    staleReviewOnly: false,
  });
  assert.equal(
    host.stderr(),
    "FAIL tools [tools-module-vet] hooks/go: go vet ./... failed:\nbad\n" +
      "FAIL tools [some-repo-rule] no path here\n" +
      "tools: 2 failure(s)\n",
  );
  assert.equal(host.stdout(), "");
});

test("an audit error prints the failures first, then ERROR, and never PASS", async () => {
  const host = fakeHost();
  const run = result({
    failures: [{ rule: "r", path: "p", message: "m" }],
    error: "go not found",
  });
  assert.deepEqual(await reportDomain("tools", run, "/r", options, host), {
    status: 1,
    staleReviewOnly: false,
  });
  assert.equal(
    host.stderr(),
    "FAIL tools [r] p: m\nERROR tools: audit could not run: go not found\n",
  );
  assert.equal(host.stdout(), "");
});

const stale = {
  rule: "review-age",
  path: "docs/surfaces/README.md",
  message: "old",
};

test("docs failures that are all review-age are stale-review-only", async () => {
  const host = fakeHost();
  const report = await reportDomain(
    "docs",
    result({ failures: [stale, stale] }, "docs"),
    "/r",
    options,
    host,
  );
  assert.deepEqual(report, { status: 1, staleReviewOnly: true });
  assert.equal(
    host.stderr(),
    "FAIL docs [review-age] docs/surfaces/README.md: old\n" +
      "FAIL docs [review-age] docs/surfaces/README.md: old\n" +
      "docs: 2 failure(s)\n",
  );
});

test("one other rule, an audit error, or another domain name defeats stale-review-only", async () => {
  const other = {
    rule: "ledger-row-date",
    path: "docs/surfaces/README.md",
    message: "bad",
  };
  const cases: Array<[string, AuditResult]> = [
    ["docs", { failures: [stale, other] }],
    ["docs", { failures: [stale], error: "could not finish" }],
    ["tools", { failures: [stale] }],
  ];
  for (const [name, outcome] of cases) {
    const report = await reportDomain(
      name,
      result(outcome, name),
      "/r",
      options,
      fakeHost(),
    );
    assert.deepEqual(
      report,
      { status: 1, staleReviewOnly: false },
      JSON.stringify([name, outcome]),
    );
  }
});

test("the renderer uses the registered classifier without deciding the domain policy", async () => {
  const outcome = {
    failures: [{ rule: "custom-age", path: "record", message: "old" }],
  };
  const host = fakeHost();
  const report = await reportDomain(
    "custom",
    {
      run: async () => outcome,
      staleReviewOnly: (result) => result === outcome,
    },
    "/r",
    options,
    host,
  );
  assert.deepEqual(report, { status: 1, staleReviewOnly: true });
  assert.equal(
    host.stderr(),
    "FAIL custom [custom-age] record: old\ncustom: 1 failure(s)\n",
  );
});
