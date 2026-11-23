# Author mode references

`/agentics:ops author` loads these references to create a new runbook, from scratch or seeded from a postmortem.

| File                                     | Purpose                                                                          |
| ---------------------------------------- | -------------------------------------------------------------------------------- |
| [author-workflow.md](author-workflow.md) | New runbook from scratch: scaffold → fill steps → verify → rollback → escalation |
| [from-postmortem.md](from-postmortem.md) | Variant: ingest a postmortem issue, draft skeleton from action items + timeline  |

Use the from-postmortem variant when an incident already supplied the procedure's raw material. Most runbooks should be
written after the incident that showed the need for them. The variant cuts drafting work because it pre-fills the
template from real data.
