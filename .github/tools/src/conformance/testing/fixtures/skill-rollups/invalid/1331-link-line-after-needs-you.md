want: needs-you list appears before link line
<!-- agentics:triage:rollup -->
<!-- agentics:triage:execution:1790000000-12345 -->

@jdoe — 6 fixed · 1 needs you

### Needs you

- **Checks — Test / Go (ubuntu-latest, tests):** `TestRunCycleClassifiesProviderOutcomes` fails non-deterministically; no source fix is supported by the run. [Failed job](https://github.com/acme/payments-api/actions/runs/1/job/2)

Earlier triage pass: [rollup of 2026-11-04](https://github.com/acme/payments-api/pull/230#issuecomment-123456)

| Source | Status | Found | Fixed | Accepted | Dismissed | Needs you |
| ------ | ------ | ----- | ----- | -------- | --------- | --------- |
| Copilot Review | Completed | 6 | 5 | 1 | 0 | 0 |
| Checks | Completed | 2 | 1 | — | — | 1 |
| **Total** | | **8** | **6** | **1** | **0** | **1** |

<details><summary>Details</summary>

#### Base sync

<!-- agentics:triage:section:base-sync -->
Up to date — `main` was already an ancestor of the PR head.
<!-- agentics:triage:section-end:base-sync -->

#### Copilot Review

- **Fixed (5):** `8bf374e5` transaction scope · `c8f94ee4` raw-batch replay · `8e432a58` emergency record · `5857b0b7` hand-off scope · `f9f6e0e3` fresh-read
- **Accepted (1):** `api/cors.go:14` — the permissive CORS on the dev-only endpoint is deliberate.

<sub>6 replies posted</sub>

#### Checks

- **Fixed (1):** `a1b2c3d4` Format
- **Needs you (1):** Test / Go (ubuntu-latest, tests) — fails non-deterministically; no source fix is supported.

<!-- agentics:triage:section:checks:108511819201 -->
<details><summary>Format: fixed (run 2026-10-02)</summary>

**Category:** Formatting · **Tier:** Auto-fix

`npx prettier --check .` failed on one file the base sync merged. Fixed in `a1b2c3d4` by running Prettier on it.

</details>
<!-- agentics:triage:section-end:checks:108511819201 -->

<!-- agentics:triage:section:checks:108511819244 -->
<details><summary>Test / Go (ubuntu-latest, tests): needs you (run 2026-10-02)</summary>

**Category:** Test · **Tier:** Intervention

The job failed in `TestRunCycleClassifiesProviderOutcomes/needs` (`cycle exit = 1`, empty stderr). This PR changes no Go files, and the identical shard passed on the previous run. [Failed job](https://github.com/acme/payments-api/actions/runs/1/job/2)

</details>
<!-- agentics:triage:section-end:checks:108511819244 -->

</details>

> _Triaged by an AI agent_
