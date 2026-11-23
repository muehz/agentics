import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import {
  cpSync,
  mkdirSync,
  readdirSync,
  readFileSync,
  writeFileSync,
} from "node:fs";
import { join } from "node:path";
import { describe, test } from "node:test";
import { parse } from "yaml";
import {
  findInterpreter,
  requireInterpreter,
} from "../entrypoint/interpreters.ts";
import {
  ASSET_FIXTURES,
  ASSET_TAG,
  validChecksumLines,
  writeAsset,
  writeAssetDir,
} from "./testing/assets.ts";
import { installReleaseRun } from "./testing/action-run.ts";
import { loadReleaseAction, runStep } from "./testing/action.ts";
import { mutationCases } from "./testing/mutation-cases.ts";
import {
  assertMutationCaught,
  contractCase,
} from "./testing/mutation-support.ts";
import { readRepoText, repoRoot } from "./testing/repo.ts";
import { scratchDir } from "./testing/support.ts";
import {
  detectionBootstrapViolations,
  jobKeySection,
  releaseWorkflowContractViolations,
  replaceFirst,
} from "./testing/workflow.ts";

const sources = {
  md: readRepoText(".github/workflows/release.md"),
  lock: readRepoText(".github/workflows/release.lock.yml"),
};

// Only the hosted artifact service and target checkout are fakes. Run the
// compiled steps against files in separate handoff and writer workspaces.
describe(
  "compiled notes handoff and stage binding before checkout",
  { skip: requireInterpreter("bash") },
  () => {
    const handoff = parse(jobKeySection(sources.lock, "update_release_notes"));
    const writer = parse(jobKeySection(sources.lock, "write-notes"));
    const sha = "a".repeat(40);
    const body = JSON.stringify({
      sha,
      tag: "v1.2.3",
      release_id: "42",
      prerelease: false,
    });
    const digest = (s: string) => createHash("sha256").update(s).digest("hex");
    const original = {
      target_sha: sha,
      state_artifact_id: "51",
      state_sha256: digest(body),
    };
    const approved = JSON.stringify({
      items: [
        { type: "update_release_notes", body: "Highlights", staged: true },
      ],
    });

    for (const scenario of [
      { name: "accepted update", success: true },
      { name: "staged update", staged: "true", success: true },
      {
        name: "identical bytes in a foreign run",
        override: { state_artifact_id: "52" },
        error: "artifact ID unavailable in this run",
      },
      {
        name: "missing stage SHA",
        override: { target_sha: "" },
        error: "Invalid stage identity outputs",
      },
      {
        name: "missing stage ID",
        override: { state_artifact_id: "" },
        error: "Invalid stage identity outputs",
      },
      {
        name: "missing stage digest",
        override: { state_sha256: "" },
        error: "Invalid stage identity outputs",
      },
      {
        name: "invalid SHA shape on both sides",
        override: { target_sha: "A".repeat(40) },
        artifactSha: "A".repeat(40),
        error: "Invalid stage identity outputs",
      },
      {
        name: "different artifact SHA with matching digest",
        artifactSha: "b".repeat(40),
        error: "Release identity SHA mismatch",
      },
      {
        name: "different digest with matching SHA",
        override: { state_sha256: "0".repeat(64) },
        error: "Release identity digest mismatch",
      },
      {
        name: "another producer already owns the handoff name",
        collision: true,
        error: "artifact name is occupied",
      },
      {
        name: "handoff unavailable in the writer",
        missing: true,
        error: "handoff unavailable",
      },
    ]) {
      test(scenario.name, () => {
        const from = scratchDir("agentics-notes-handoff-");
        const to = scratchDir("agentics-notes-writer-");
        const state = scenario.artifactSha
          ? JSON.stringify({ ...JSON.parse(body), sha: scenario.artifactSha })
          : body;
        const outputs: Record<string, string> = {
          ...original,
          state_sha256: digest(state),
          ...scenario.override,
        };
        const resolve = (value: string): string =>
          value.replace(
            /\$\{\{ needs\.stage\.outputs\.(\w+) \}\}/g,
            (_, key: string) => outputs[key]!,
          );
        const env = Object.fromEntries(
          Object.entries<string>(writer.env).map(([k, v]) => [k, resolve(v)]),
        );
        const shell = (
          run: string,
          cwd: string,
          vars: NodeJS.ProcessEnv,
          node = false,
        ) => {
          const res = spawnSync(
            node ? process.execPath : findInterpreter("bash")!,
            node
              ? ["-e", run]
              : ["--noprofile", "--norc", "-eo", "pipefail", "-c", run],
            { cwd, encoding: "utf8", env: { ...process.env, ...vars } },
          );
          assert.ifError(res.error);
          if (res.status !== 0) throw Error(res.stderr);
        };
        writeFileSync(join(from, "approved.json"), approved);
        const stored = new Map<string, string>();
        if (scenario.collision)
          stored.set("release-notes-handoff", "other-producer");
        // Both runs carry identical identity bytes. The action exposes only the
        // current run unless a token and cross-run selection are supplied.
        const identities = [
          { run: "7", id: "51", state },
          { run: "8", id: "52", state },
        ];
        let checkedOut = false;
        const attempt = () => {
          const prepare = handoff.steps.find(
            (s: { name: string }) =>
              s.name === "Prepare detector-approved notes handoff",
          );
          shell(prepare.run, from, {
            GH_AW_AGENT_OUTPUT: "approved.json",
            GH_AW_SAFE_OUTPUTS_STAGED: scenario.staged,
          });
          const upload = handoff.steps.find((s: { uses?: string }) =>
            s.uses?.startsWith("actions/upload-artifact@"),
          ).with;
          if (stored.has(upload.name) && !upload.overwrite)
            throw Error("artifact name is occupied");
          stored.set(upload.name, join(from, upload.path));
          if (scenario.missing) stored.clear();
          for (const step of writer.steps) {
            if (step.uses?.startsWith("actions/checkout@")) {
              checkedOut = true;
              assert.equal(resolve(step.with.ref), sha);
              break;
            }
            if (step.uses?.startsWith("actions/download-artifact@")) {
              const input = step.with;
              if (input["artifact-ids"] !== undefined) {
                const match = identities.find(
                  (a) =>
                    a.id === resolve(input["artifact-ids"]) &&
                    a.run === (input["run-id"] ?? "7"),
                );
                if (!match) throw Error("artifact ID unavailable in this run");
                mkdirSync(join(to, input.path));
                writeFileSync(join(to, input.path, "state.json"), match.state);
              } else {
                const source = stored.get(input.name);
                if (!source) throw Error("handoff unavailable");
                cpSync(source, join(to, input.path), { recursive: true });
              }
            } else if (step.run) {
              shell(
                step.run,
                to,
                {
                  ...env,
                  GITHUB_ENV: "runner-env",
                  GITHUB_SERVER_URL: "https://github.com",
                },
                step.shell === "node {0}",
              );
            }
          }
        };
        if (scenario.error) assert.throws(attempt, new RegExp(scenario.error));
        else attempt();
        assert.equal(checkedOut, scenario.success === true);
        if (checkedOut) {
          assert.equal(
            readFileSync(join(to, env.GH_AW_AGENT_OUTPUT!), "utf8"),
            approved,
          );
          assert.ok(
            readFileSync(join(to, "runner-env"), "utf8")
              .split(/\r?\n/)
              .includes(
                `GH_AW_SAFE_OUTPUTS_STAGED=${scenario.staged ?? "false"}`,
              ),
          );
        }
      });
    }
  },
);

// Every case edits a copy of the real, currently passing file and expects
// the checker to report a violation.
test("release.md and release.lock.yml satisfy their own contracts before any mutation", () => {
  assert.deepEqual(releaseWorkflowContractViolations(sources.md), []);
  assert.deepEqual(detectionBootstrapViolations(sources.lock), []);
});

const names = Object.keys(ASSET_FIXTURES);
const hookPaths = names
  .filter((name) => name.startsWith("agx-"))
  .map((name) => `hooks/bin/${name}`);
const stage = parse(jobKeySection(sources.lock, "stage"));
const rebuildIndex = stage.steps.findIndex(
  (step: { name: string }) =>
    step.name === "Compare assets in the release roster",
);
const rebuild: string = stage.steps[rebuildIndex].run;
const verify = stage.steps[rebuildIndex + 1];

// The trusted bundle step supplies rebuilt/. Exercise the roster command,
// each byte comparison and the subsequent verify action on a separate candidate.
const fixtures = `
node() {
  if [[ "\${2-}" == asset-roster ]]; then
    [[ "$*" == ".github/tools/src/release/cli.ts asset-roster --tag $TAG" ]] || return 64
    echo query >> trace
    if [[ -f reply ]]; then cat reply; return "$QUERY_STATUS"; fi
  fi
  command node "$@"
}
cmp() {
  command cmp "$@" || return $?
  echo "cmp $1 $2" >> trace
}
`;

interface Scenario {
  label: string;
  roster?: { hookBinaryPaths: string[]; derivableAssetNames?: string[] };
  queryStatus?: number;
  mutate?: (script: string) => string;
  candidate?: (dir: string) => void;
  error?: RegExp;
  verified?: boolean;
}

const validRoster = { hookBinaryPaths: hookPaths, derivableAssetNames: names };
const cases: Scenario[] = [
  {
    label: "nine comparisons and ten verified assets reach attestation",
    verified: true,
  },
  {
    label: "a failed producer with valid JSON stops before using its output",
    roster: validRoster,
    queryStatus: 23,
  },
  ...Object.entries({
    missing: undefined,
    empty: [],
    partial: names.slice(1),
    duplicate: [...names.slice(0, -1), names[0]!],
    unexpected: [...names.slice(0, -1), "bin/agx.exe"],
  }).map(([label, list]) => ({
    label: `rejects derivation list: ${label}`,
    roster: { ...validRoster, derivableAssetNames: list },
    error: /derivable asset names/,
  })),
  {
    label: "rejects a substituted hook source",
    roster: {
      ...validRoster,
      hookBinaryPaths: [...hookPaths.slice(1), "bin/agx.exe"],
    },
    error: /hook binary paths/,
  },
  ...Object.entries({
    "zero comparisons": "continue",
    "one missing comparison":
      '[[ "$name" != THIRD_PARTY_NOTICES.md ]] || continue',
    "nine comparisons with one repeated":
      '[[ "$name" != THIRD_PARTY_NOTICES.md ]] || name=agx-linux-amd64',
  }).map(([label, mutation]) => ({
    label: `rejects ${label} with a valid candidate`,
    mutate: (script: string) =>
      script.replace(
        '  cmp "rebuilt/$name"',
        `  ${mutation}\n  cmp "rebuilt/$name"`,
      ),
    error: /incomplete rebuilt-asset comparisons/,
  })),
  {
    label: "rejects a byte mismatch",
    candidate: (dir) => writeAsset(dir, "agx-linux-amd64", "wrong"),
    error: /agx-linux-amd64 does not match the target commit/,
  },
  {
    label: "rejects an extra asset before attestation",
    candidate: (dir) => writeAsset(dir, "extra", "extra"),
    error: /exactly the ten expected names/,
    verified: true,
  },
  {
    label: "rejects a bad digest before attestation",
    candidate: (dir) =>
      writeAsset(
        dir,
        "SHA256SUMS",
        validChecksumLines()
          .join("\n")
          .replace(/^[0-9a-f]{64}/, "0".repeat(64)) + "\n",
      ),
    error: /digest mismatch/,
    verified: true,
  },
];

describe(
  "compiled stage derivation through the attestation boundary",
  { skip: requireInterpreter("bash") },
  () => {
    for (const scenario of cases) {
      test(scenario.label, () => {
        const dir = scratchDir("agentics-stage-derive-");
        const tools = ".github/tools/src";
        cpSync(join(repoRoot, tools, "release"), join(dir, tools, "release"), {
          recursive: true,
        });
        cpSync(
          join(repoRoot, tools, "plugin-manifests.ts"),
          join(dir, tools, "plugin-manifests.ts"),
        );
        installReleaseRun(dir);
        cpSync(writeAssetDir(validChecksumLines()), join(dir, "rebuilt"), {
          recursive: true,
        });
        const candidate = writeAssetDir(validChecksumLines());
        scenario.candidate?.(candidate);
        cpSync(candidate, join(dir, "release-assets"), { recursive: true });
        mkdirSync(join(dir, "temp"));
        writeFileSync(join(dir, "trace"), "");
        if (scenario.roster)
          writeFileSync(join(dir, "reply"), JSON.stringify(scenario.roster));
        const script = scenario.mutate?.(rebuild) ?? rebuild;
        if (scenario.mutate) assert.notEqual(script, rebuild);
        assert.equal(verify.uses, "./.github/actions/release");
        assert.equal(verify.with.tag, "${{ needs.prepare.outputs.tag }}");
        assert.equal(stage.steps[rebuildIndex + 2].id, "attest-check");
        assert.match(
          stage.steps[rebuildIndex + 3].uses,
          /^actions\/attest-build-provenance@/,
        );
        const result = spawnSync(
          findInterpreter("bash")!,
          [
            "--noprofile",
            "--norc",
            "-eo",
            "pipefail",
            "-c",
            [
              fixtures,
              script,
              "echo verify >> trace",
              runStep(loadReleaseAction().action).run,
              "echo attest >> trace",
            ].join("\n"),
          ],
          {
            cwd: dir,
            encoding: "utf8",
            env: {
              ...process.env,
              TAG: ASSET_TAG,
              TARGET_SHA: "a".repeat(40),
              RUNNER_TEMP: "temp",
              CHECKOUT: ".",
              COMMAND: verify.with.command,
              ASSETS: verify.with.assets,
              QUERY_STATUS: String(scenario.queryStatus ?? 0),
            },
          },
        );
        assert.ifError(result.error);
        const trace = readFileSync(join(dir, "trace"), "utf8")
          .trim()
          .split("\n");
        if (scenario.error) assert.match(result.stderr, scenario.error);
        if (scenario.queryStatus) {
          assert.equal(result.status, scenario.queryStatus, result.stderr);
          assert.deepEqual(trace, ["query"]);
        }
        const success = !scenario.error && !scenario.queryStatus;
        assert.equal(result.status === 0, success, result.stderr);
        assert.equal(trace.includes("attest"), success);
        assert.equal(trace.includes("verify"), scenario.verified === true);
        if (scenario.verified) {
          assert.deepEqual(trace, [
            "query",
            ...names.map(
              (name) => `cmp rebuilt/${name} release-assets/${name}`,
            ),
            "verify",
            ...(success ? ["attest"] : []),
          ]);
        }
        if (success)
          assert.equal(readdirSync(join(dir, "release-assets")).length, 10);
      });
    }
  },
);

describe("release workflow mutations are caught", () => {
  for (const tc of mutationCases) {
    test(tc.name, () => {
      assertMutationCaught(tc, sources);
    });
  }
});

// The runner is the mutation-sensitive part: these three tests show it rejects each way a case can pass vacuously.
test("the runner rejects a mutation that changes nothing", () => {
  const noop = contractCase("a no-op", (s) => s, "anything");
  assert.throws(
    () => assertMutationCaught(noop, sources),
    /mutation had no effect/,
  );
  const missing = contractCase(
    "a miss",
    (s) => replaceFirst(s, "no such text", "x"),
    "anything",
  );
  assert.throws(
    () => assertMutationCaught(missing, sources),
    /mutation had no effect/,
  );
});

test("the runner rejects a checker that finds nothing", () => {
  const relaxed = {
    ...contractCase(
      "relaxed",
      (s) => replaceFirst(s, "group: agentics-release", "group: other"),
      undefined,
    ),
    check: () => [],
  };
  assert.throws(
    () => assertMutationCaught(relaxed, sources),
    /at least one contract violation/,
  );
});

test("the runner rejects a violation that does not name the targeted check", () => {
  const wrongTarget = contractCase(
    "wrong target",
    (s) => replaceFirst(s, "group: agentics-release", "group: other"),
    "a violation no check emits",
  );
  assert.throws(
    () => assertMutationCaught(wrongTarget, sources),
    /lack the targeted one/,
  );
});

test("the case names are unique", () => {
  const names = mutationCases.map((tc) => tc.name);
  assert.equal(new Set(names).size, names.length);
});

// The table held 68 cases when it was first written. The contract may grow but never shrink: deleting a case to make a
// change pass would otherwise go unnoticed.
const MIN_CASE_COUNT = 68;

test("the table keeps at least its original 68 cases", () => {
  assert.ok(
    mutationCases.length >= MIN_CASE_COUNT,
    `${mutationCases.length} cases, the table needs at least ${MIN_CASE_COUNT}`,
  );
});
