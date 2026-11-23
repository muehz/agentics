# Bot Review References

Bot Review source-specific triage: finding assessment, specialised-source duplicate recognition, fixes, and replies.

## Files

| File                                           | Purpose                                                                                                                                          |
| ---------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------ |
| [`bot-review-triage.md`](bot-review-triage.md) | End-to-end Bot Review triage workflow (receive threads → validate → recognise duplicates → apply fixes → draft per-thread replies). Entry point. |

This adapter reuses the Copilot Review disposition vocabulary and decision cognition, because unlisted bots' findings
have the shape of review suggestions. See
[`../copilot/copilot-finding-dispositions.md`](../copilot/copilot-finding-dispositions.md). Specialised sources own
cross-bot duplicates. Bot Review leaves those fixes and dispositions to their owners.

Detection identity, edge cases, payload fields and each login's lane live in
[`../../source-detection.md`](../../source-detection.md), "Bot Review". Only the logins its routing table names for the
automatic lane get fixes pushed and replies posted without a question; every other bot's threads become drafts the
developer accepts first. Severity badges map to the shared severities, label and order findings in the rollup only, and
never appear in a reply. They do not change a disposition.
