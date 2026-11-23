import assert from "node:assert/strict";
import path from "node:path";
import test from "node:test";
import { spawnSync } from "node:child_process";
import { type LookupRequest, lastGreen, runsUrl } from "./ci-last-green.ts";

const ENV = {
  GITHUB_REPOSITORY: "owner/repo",
  GITHUB_REF_NAME: "main",
  GH_TOKEN: "token",
};
const SHA = "a".repeat(40);

function respond(body: unknown, ok = true): LookupRequest {
  return async () => ({ ok, json: async () => body });
}

test("the lookup asks api.github.com for the newest successful push run of validate.yml on the branch", () => {
  assert.equal(
    runsUrl(ENV),
    "https://api.github.com/repos/owner/repo/actions/workflows/validate.yml/runs?branch=main&event=push&status=success&per_page=1",
  );
});

test("the host is fixed: an API URL from the environment is ignored", () => {
  assert.equal(
    runsUrl({ ...ENV, GITHUB_API_URL: "https://attacker.example.invalid" }),
    runsUrl(ENV),
  );
});

test("a repository or branch that is not a plain name sends no request", async () => {
  let requests = 0;
  const count = async () => {
    requests += 1;
    return { ok: true, json: async () => ({}) };
  };
  for (const env of [
    { ...ENV, GITHUB_REPOSITORY: "owner/repo/../../other" },
    { ...ENV, GITHUB_REPOSITORY: "https://attacker.example.invalid/x" },
    { ...ENV, GITHUB_REPOSITORY: "owner" },
    { ...ENV, GITHUB_REPOSITORY: undefined },
    { ...ENV, GITHUB_REF_NAME: "main?per_page=100" },
    { ...ENV, GITHUB_REF_NAME: "" },
  ]) {
    assert.equal(runsUrl(env), null);
    assert.equal(await lastGreen(env, count), "");
  }
  assert.equal(requests, 0);
});

test("the head commit of the first run is the last validated commit, sent with the token", async () => {
  let seen: { url: string; options: Parameters<LookupRequest>[1] } | undefined;
  const sha = await lastGreen(ENV, async (url, options) => {
    seen = { url, options };
    return {
      ok: true,
      json: async () => ({ workflow_runs: [{ head_sha: SHA }] }),
    };
  });
  assert.equal(sha, SHA);
  assert.ok(seen);
  assert.equal(seen.url, runsUrl(ENV));
  assert.equal(seen.options.headers.authorization, "Bearer token");
});

test("no run, a failed request, a thrown error, and a malformed commit all give no commit", async () => {
  assert.equal(await lastGreen(ENV, respond({ workflow_runs: [] })), "");
  assert.equal(await lastGreen(ENV, respond({})), "");
  assert.equal(
    await lastGreen(
      ENV,
      respond({ workflow_runs: [{ head_sha: SHA }] }, false),
    ),
    "",
  );
  assert.equal(
    await lastGreen(ENV, async () => {
      throw new Error("network");
    }),
    "",
  );
  for (const bad of ["abc", `${SHA}\nmode=asset-only`, 7, null])
    assert.equal(
      await lastGreen(ENV, respond({ workflow_runs: [{ head_sha: bad }] })),
      "",
    );
});

test("the script prints only the output line on stdout and exits zero when the lookup cannot run", () => {
  const result = spawnSync(
    process.execPath,
    [path.join(import.meta.dirname, "ci-last-green.ts")],
    {
      encoding: "utf8",
      env: { PATH: process.env.PATH, GITHUB_REPOSITORY: "not a repository" },
    },
  );
  assert.equal(result.status, 0, result.stderr);
  assert.equal(result.stdout, "sha=\n");
  assert.match(result.stderr, /no validated commit/);
});
