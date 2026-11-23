# GitHub Code Quality References

GitHub Code Quality source-specific triage: quality-finding assessment, suggested-autofix verification, triage workflow.

## Files

| File                                               | Purpose                                                                                                                                         |
| -------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------- |
| [`code-quality-triage.md`](code-quality-triage.md) | End-to-end GitHub Code Quality triage workflow (receive threads → validate → verify autofixes → apply → draft per-thread replies). Entry point. |

This adapter reuses the Copilot Review disposition vocabulary and decision cognition, because quality findings have the
shape of review suggestions (see
[`../copilot/copilot-finding-dispositions.md`](../copilot/copilot-finding-dispositions.md)). Its one adapter-specific
behavior is autofix handling. The bot ships suggested autofixes. This workflow treats them as proposals that need
independent verification. It never auto-accepts them.

Detection identity and edge cases live in [`../../source-detection.md`](../../source-detection.md). Comment-body parsing
is deliberately lenient because the format is not a stable contract.
