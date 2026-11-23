# Checks References

Source-specific triage for failed PR check runs from GitHub Actions and any other CI provider that reports through the
GitHub Checks API.

| File                                           | Purpose                                                                                        |
| ---------------------------------------------- | ---------------------------------------------------------------------------------------------- |
| [checks-triage.md](checks-triage.md)           | Specialist workflow: what the subagent does                                                    |
| [checks-tier-policy.md](checks-tier-policy.md) | Auto-fix vs Suggest vs Intervention tier policy and the strict "DO NOT" list                   |
| [checks-categories.md](checks-categories.md)   | Lint / Test / Build / Deploy / Other category-specific assessment guidance                     |
| [checks-log-parsing.md](checks-log-parsing.md) | How to fetch logs and annotations (`gh run view --log-failed`, check-runs API), provider notes |

Copilot review triage lives in `../pull-requests/copilot/`. Checkmarx One triage lives in `../pull-requests/checkmarx/`.
The cross-cutting orchestration, rollup, and source-detection files live in the root `references/`.

## Provider scope

GitHub Actions is the primary log-bearing provider. The skill detects any other CI provider that reports check runs
through the GitHub Checks API. Examples are CircleCI and Buildkite. The skill triages such a provider on annotations
only, with "view in external CI" pointers instead of full logs. Log retrieval for another provider is a new entry in
`checks-log-parsing.md`, not a structural change.
