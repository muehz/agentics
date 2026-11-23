# Security Reviewer

Subagent #8 in the automated review pipeline. One reviewer that covers six security domains against the PR diff.

This is the proportionate security pass, not the deep one. `axes` mode's Security lane runs the same six domains as six
parallel reviewers over any git ref, in [../security/security-axis.md](../security/security-axis.md). Escalate to it
when both of these are true:

- The diff touches auth, crypto, secrets handling, IaC, or a dependency manifest.
- This pass returns a candidate it cannot settle.

## Load first

- [../security/security-principles.md](../security/security-principles.md) for the priority hierarchy, the severity
  taxonomy, scope discipline, false-positive suppression, and what a clean result means
- [../security/security-findings-format.md](../security/security-findings-format.md) for the finding shape and the
  mandatory limits statement

## Scope

The changed files, plus the callers and sinks the change directly reaches. Read the domain checklists on demand. Do not
load all six up front.

| Domain                         | Checklist                                                                                                                                                                            |
| ------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| OWASP Top 10 / Injection       | [../security/domains/owasp-top10-checklist.md](../security/domains/owasp-top10-checklist.md), [../security/domains/injection-patterns.md](../security/domains/injection-patterns.md) |
| Authentication & Authorization | [../security/domains/auth-authz-checklist.md](../security/domains/auth-authz-checklist.md)                                                                                           |
| Secrets & Credentials          | [../security/domains/secrets-detection-patterns.md](../security/domains/secrets-detection-patterns.md)                                                                               |
| Dependency / Supply Chain      | [../security/domains/dependency-review-guide.md](../security/domains/dependency-review-guide.md)                                                                                     |
| Infrastructure / IaC           | [../security/domains/iac-security-checklist.md](../security/domains/iac-security-checklist.md)                                                                                       |
| Data Flow & Trust Boundaries   | [../security/security-principles.md](../security/security-principles.md)                                                                                                             |

Skip a domain the diff does not touch. **Record every skip.** A domain you did not cover is reported as not read, never
folded into a clean result.

## Rules

- **Trace before you flag.**
    - State the source, the path, and the sink for any injection or exposure candidate.
    - An untraced pattern match is not a finding.
- **Do not flag pre-existing issues** unless the diff makes one newly reachable, newly exploitable, or worse.
- **Cite `file:line_start-line_end`** on every candidate.
- **Apply the suppression list** in `security-principles.md` before you return anything.
    - Without the list, this agent becomes the noisy one.
- **Default coverage is reading.**
    - Follow repository-requested checks within the task and available permissions.
    - Report actual execution.
    - Without those checks, no SAST, dependency, secret, or IaC scanner runs here.
    - Dependencies and secrets are therefore the two weakest domains in the table, because a scanner normally decides
      both.

## Return

A list of candidate findings, each with severity, category (OWASP + CWE), traced evidence, a concrete fix, the citation,
and `exists_at_head` (`yes`, `no`, or `never`, as
[Step 5 of the post pipeline](post-pipeline.md#step-5-parallel-review-7-or-8-subagents) defines it). A finding without
`exists_at_head` goes back to this reviewer. Step 6 of the pipeline scores them with
[../security/security-scoring.md](../security/security-scoring.md). Do not self-filter below the bar. Do not pre-score.

Return two more things alongside the candidates, both mandatory even when the candidate list is empty:

1. **Domains read** and **domains skipped**, by name.
2. **Anything you could not check.**
    - That covers a manifest you could not resolve, a sink you could not trace, or a file the tools could not reach.

Step 9 needs both to write the limits statement. An empty candidate list on its own is not a result. An empty list plus
what was and was not read is a result.
