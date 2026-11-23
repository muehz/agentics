# GitHub Code Security References

GitHub Code Security (code scanning / GHAS) source-specific triage: alert-thread assessment, remediation via the shared
Checkmarx pattern files, triage workflow.

## Files

| File                                                 | Purpose                                                                                                                         |
| ---------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------- |
| [`code-security-triage.md`](code-security-triage.md) | End-to-end GitHub Code Security triage workflow (receive threads → assess → remediate → draft per-thread replies). Entry point. |

This adapter intentionally has no parsing or remediation reference of its own:

- **Finding assessment** reuses
  [`../checkmarx/checkmarx-finding-categories.md`](../checkmarx/checkmarx-finding-categories.md), because code scanning
  flags the same vulnerability classes (injection, path traversal, secrets in logs, workflow hardening).
- **Remediation patterns** reuse
  [`../checkmarx/checkmarx-remediation-patterns-generic.md`](../checkmarx/checkmarx-remediation-patterns-generic.md) and
  [`../checkmarx/checkmarx-remediation-patterns-actions-js.md`](../checkmarx/checkmarx-remediation-patterns-actions-js.md).
  The Phase 3 heuristic in `code-security-triage.md` picks between them.
- **Comment-body parsing is deliberately lenient**, because GitHub-native scanner body formats are not a stable
  contract. The workflow triages from thread context (path, line, diff) plus the code-scanning alerts API, not from body
  markup.

Detection identity and edge cases live in [`../../source-detection.md`](../../source-detection.md). For Copilot review
triage, see `../copilot/`. For cross-cutting orchestration/rollup, see the references root.
