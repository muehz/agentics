# Checks Failure Categories

Per-category guidance for failed PR check runs: the typical tier, false-positive patterns, and how log shape shows a
genuine or environmental failure.

Categories are signals, not commitments. The [tier policy](checks-tier-policy.md) decision flow is authoritative. A
"typically Auto-fix" lint failure can land in Suggest if the fix isn't obvious. A "typically Suggest" test failure can
land in Intervention if it looks flaky.

## Lint / Format

**Typical tier: Auto-fix.**

Lint and formatter fixes are deterministic, and the tool emits the diff.

| Tool                   | Auto-fix signal                                                                                 | Notes                                                                    |
| ---------------------- | ----------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------ |
| `prettier`             | `--check` failure → `--write` produces the fix                                                  | Always Auto-fix unless the file is `.prettierignore`-listed unexpectedly |
| `eslint`               | Rule's `meta.fixable` is `"code"` or `"whitespace"`                                             | Auto-fix only those rules. Rules without `fixable` → Suggest             |
| `ruff` / `black`       | `--check` failure with `--fix`-able rules                                                       | Auto-fix the `--fix`-able subset. Demote the rest                        |
| `gofmt` / `goimports`  | Diff is mechanical                                                                              | Auto-fix                                                                 |
| `rustfmt`              | `--check` failure                                                                               | Auto-fix                                                                 |
| `markdownlint` / `mdl` | Mechanical rules (whitespace, heading levels) → Auto-fix. Content rules (link checks) → Suggest | Per-rule decision                                                        |

**Demote to Suggest when:**

- The lint rule is opinionated and the fix would change semantics. For example, `no-implicit-coercion` rewrites `+x` to
  `Number(x)`: usually right, but worth a human glance.
- The fix would touch a generated file whose generator is unknown.

**False-positive patterns:**

- The lint job fails on a config file that the PR intentionally changes. Read the PR diff before you fix it.
- The lint job's cache is stale and the local rerun passes. The log shows the same set of files across reruns. Tier as
  Intervention with that observation.

## Test (unit / integration / e2e)

**Typical tier: Suggest** (and often Intervention for e2e).

Test failures usually need judgment about whether the test or the implementation is correct. A mechanical test fix can
be Auto-fix only when the change unambiguously follows from code already in the PR. An example of a mechanical test fix
is a renamed export that the test imports.

| Sub-type                           | Typical tier | Reasoning                                                                                           |
| ---------------------------------- | ------------ | --------------------------------------------------------------------------------------------------- |
| Unit test, single assertion drift  | Suggest      | Either the test needs an update or the implementation does. A human confirms.                       |
| Unit test, missing import / typo   | Auto-fix     | Mechanical, deterministic, behavior-preserving                                                      |
| Snapshot test diff                 | Suggest      | Update the snapshot only if the new output is intentional. Never auto-update it                     |
| Integration test                   | Suggest      | It often touches multiple modules. Verify the broader behavior change with the developer            |
| e2e / browser / visual diff        | Intervention | It requires environment, screenshots, or visual judgment that the specialist cannot evaluate        |
| Flaky test (intermittent failures) | Intervention | Never Auto-fix flakes. Surface them with `intervention_reason: "appears flaky — see {details_url}"` |

**Flake detection from log shape:**

- The same test passes locally and on retry but fails in CI → likely flaky.
- The failure references timing (`waitFor`, `setTimeout`, race condition language) without a deterministic root cause.
- The check has been re-run on the PR and failed at different points each time.
- `expected: <Promise>, received: <value>` patterns in async tests.

**Never:**

- Disable, skip, or delete a failing test (see DO NOT list).
- Auto-update a snapshot. The developer confirms the new output.

## Build / Compile

**Typical tier: split.** Mechanical missing-import / generated-artifact failures are Auto-fix. Everything else is
Suggest or Intervention.

| Sub-type                                 | Typical tier | Reasoning                                                                                   |
| ---------------------------------------- | ------------ | ------------------------------------------------------------------------------------------- |
| Missing import (TS, Go, Rust, Python)    | Auto-fix     | The compiler / linter usually points to the exact import. Resolution is mechanical          |
| Missing generated file (codegen, protoc) | Auto-fix     | If the generator command is committed and deterministic, re-run it. Then commit the result  |
| Type error (TS / Rust / Mypy / Pyright)  | Suggest      | The fix is small but the right shape needs human confirmation                               |
| Dependency resolution failure            | Intervention | A change to `package.json`, `Cargo.toml`, `go.mod`, or a similar file is on the DO NOT list |
| Cyclic-dependency / module-graph error   | Intervention | A restructure of the module graph is not behavior-preserving                                |
| Compile failure in unfamiliar code path  | Suggest      | Draft the obvious fix. Let the developer confirm the broader implications                   |
| Compiler / toolchain version mismatch    | Intervention | Toolchain config changes are out of scope                                                   |

**False-positive patterns:**

- Build cache poisoning produces the same error across reruns, even after a known fix is on the branch. Tier
  Intervention with that observation.
- A missing file that the PR intentionally adds in a later commit. Confirm against the PR's stated plan before you "fix"
  it.

## Deploy

**Typical tier: always Intervention.**

Deploy failures are environmental almost without exception: a misconfigured target, an expired credential, an
infrastructure outage, or conflicting state from a previous deploy. None is safe for automatic remediation.

Surface every deploy failure with `intervention_reason` set to the deploy target's first error line and a pointer to the
`details_url`. Do not attempt remediation. Do not draft a suggested fix.

**Never:**

- Re-run the deploy.
- Edit deploy config to "fix" the failure.
- Modify cloud-resource definitions, environment variables, or rollout strategies.

## Other

This category is a catch-all for failures that do not fit Lint, Test, Build, or Deploy. Examples are license checks,
vulnerability scanners, dependency-review actions, pre-commit hook validators, and custom org policy checks.

**Typical tier: Intervention.**

Tier as Intervention unless there is a strong reason to believe a remediation is deterministic and behavior-preserving.
Many "Other" checks, such as a vulnerability scanner finding, overlap with the Checkmarx specialist's domain. Name that
overlap in `intervention_reason`.

| Sub-type                                               | Typical tier | Reasoning                                                                      |
| ------------------------------------------------------ | ------------ | ------------------------------------------------------------------------------ |
| License compliance                                     | Intervention | License decisions are organizational, not mechanical                           |
| Dependency review (`actions/dependency-review-action`) | Intervention | It overlaps with Checkmarx. Defer with a pointer                               |
| Security scanner (non-Checkmarx)                       | Intervention | Same shape as Checkmarx but the specialist isn't this one                      |
| Spell-checker / docs linter                            | Suggest      | It drafts a fix but is rarely Auto-fix. Natural-language rules are noisy       |
| Custom org policy bot                                  | Intervention | It is org-specific. Surface it with the bot's message as `intervention_reason` |

## Common false-positive shapes (across all categories)

- **Transient network errors.** `ECONNREFUSED`, `getaddrinfo ENOTFOUND`, `SSL handshake timeout`. Tier Intervention with
  the network error as `intervention_reason`. Never re-run.
- **Infrastructure outages.** GitHub status banner red, repeated 5xx from the runner registry, `actions/checkout`
  failures. Tier Intervention.
- **Cache poisoning.** The same error across reruns even after a known fix has landed. Tier Intervention with
  `intervention_reason: "build cache may be stale — try clearing it"`.
- **Resource exhaustion.** `Cannot allocate memory`, `ENOSPC`, runner OOM. Tier Intervention.
- **Concurrency races.** Two parallel jobs write to the same cache or artifact. Tier Intervention.

In every false-positive shape, the check failure is not the developer's bug. The entire deliverable is to surface the
failure with a clear `intervention_reason`.
