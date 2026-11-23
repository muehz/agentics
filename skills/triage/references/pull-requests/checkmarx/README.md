# Checkmarx References

The Checkmarx One triage rules.

## Files

| File                                                                                           | Purpose                                                                                                                                                |
| ---------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------ |
| [`checkmarx-triage.md`](checkmarx-triage.md)                                                   | End-to-end Checkmarx triage workflow (locate comment → parse → assess → remediate → summarize). Entry point.                                           |
| [`checkmarx-comment-parsing.md`](checkmarx-comment-parsing.md)                                 | Checkmarx One PR-comment format: regex patterns, table parsing, edge cases.                                                                            |
| [`checkmarx-finding-categories.md`](checkmarx-finding-categories.md)                           | Known Checkmarx finding categories with assessment guidance and common false-positive patterns.                                                        |
| [`checkmarx-remediation-patterns-actions-js.md`](checkmarx-remediation-patterns-actions-js.md) | Safe remediation patterns scoped to GitHub Actions JS/TS code (`@actions/core` toolkit, workflow YAML, `${{ secrets.* }}` references).                 |
| [`checkmarx-remediation-patterns-generic.md`](checkmarx-remediation-patterns-generic.md)       | Language-agnostic remediation patterns with examples in Python, Go, Java, server-side Node.js, and Rust. Use for general application and service code. |

The Phase 3 heuristic in `checkmarx-triage.md` says when to load each remediation-patterns file and when to load both.

For Copilot review triage, see `../copilot/`. For orchestration, the rollup, and source detection across all sources,
see the root.
