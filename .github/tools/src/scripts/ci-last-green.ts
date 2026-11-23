#!/usr/bin/env node
// Prints the step output line `sha=<commit>`: the head commit of the newest successful push run of validate.yml on
// this branch, or `sha=` when there is none or the lookup fails. The workflow appends the line to its step outputs.
//   GH_TOKEN=<token> node .github/tools/src/scripts/ci-last-green.ts >> "$GITHUB_OUTPUT"
// ci-scope.ts scopes a push against that commit. An empty value makes the push run in full, so a failed lookup can
// only widen a run, and this script never exits nonzero.

// The token goes only to this host. The request never follows a host from the environment.
const API = "https://api.github.com";
const WORKFLOW = "validate.yml";
const REPOSITORY = /^[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+$/;
const BRANCH = /^[A-Za-z0-9_.\/-]+$/;

// The URL of the lookup, or null when the repository or branch name is not a plain name.
export interface LookupResponse {
  ok: boolean;
  json(): Promise<unknown>;
}
export type LookupRequest = (
  url: string,
  options: { headers: Record<string, string> },
) => Promise<LookupResponse>;

function property(value: unknown, key: string): unknown {
  return value === null || value === undefined
    ? undefined
    : Reflect.get(Object(value), key);
}

function runsUrl(env: NodeJS.ProcessEnv): string | null {
  const repository = env.GITHUB_REPOSITORY ?? "";
  const branch = env.GITHUB_REF_NAME ?? "";
  if (!REPOSITORY.test(repository) || !BRANCH.test(branch)) return null;
  const [owner, name] = repository.split("/");
  const query = new URLSearchParams({
    branch,
    event: "push",
    status: "success",
    per_page: "1",
  });
  return `${API}/repos/${encodeURIComponent(owner!)}/${encodeURIComponent(name!)}/actions/workflows/${WORKFLOW}/runs?${query}`;
}

async function lastGreen(
  env: NodeJS.ProcessEnv = process.env,
  request: LookupRequest = fetch,
): Promise<string> {
  const url = runsUrl(env);
  if (!url) return "";
  try {
    const response = await request(url, {
      headers: {
        accept: "application/vnd.github+json",
        authorization: `Bearer ${env.GH_TOKEN}`,
        "x-github-api-version": "2022-11-28",
      },
    });
    if (!response.ok) return "";
    const value = await response.json();
    const runs = property(value, "workflow_runs");
    const first = property(runs, "0");
    const sha = property(first, "head_sha");
    return typeof sha === "string" && /^[0-9a-f]{40}$/.test(sha) ? sha : "";
  } catch {
    return "";
  }
}

async function main() {
  const sha = await lastGreen();
  console.error(sha ? `last validated commit: ${sha}` : "no validated commit");
  console.log(`sha=${sha}`);
}

if (import.meta.main) main();

export { lastGreen, runsUrl };
