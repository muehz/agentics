# Incident & Postmortem Context

This playbook is not a separate source. It is a **cross-cutting angle**. Incidents often motivate defensive code ("we
added this check after the X outage"). Defensive code includes null checks, retry logic, timeout handling, rate
limiting, and feature flags. If the target looks defensive, hunt specifically for incident history across every
available source:

- **Notion**: search for postmortems that mention the target file, feature, or error string
- **Linear**: look for tickets labeled `incident`, `sev-*`, `postmortem-action-item`, `reliability`
- **Slack**: search `#sev-*` and `#incident-*` channels around the dates the target code was added
- **Git**: commits with messages like "fix for incident", "add defensive check", "revert" followed by "re-apply with..."
  are strong signals
- **Datadog**: search with `search_datadog_incidents` for formal incident records with timelines. Also check the
  dashboards and monitors created as postmortem action items.
- **Sentry**: look for issues whose first-seen/last-seen window aligns with the target's PR ship date. Look also for
  stack traces through the target.
- **Databricks**: product-analytics events that classify an error condition (client-reported failures, user-visible
  retry events, etc.) often spike during an incident window. A drop in that event count after the target PR ships is
  circumstantial support that the target code resolved the user-visible symptom. The support stands even when
  Datadog/Sentry signal is noisy.

If you find an incident link, fetch the full postmortem. Postmortems typically have an "Action Items" section that ties
directly to code changes. When multiple sources corroborate each other, the evidence is especially strong. For example:

- A Datadog incident ID appears in a Linear ticket.
- The Linear ticket appears in a Notion postmortem.
- The Notion postmortem appears in a Slack thread that links to the target PR.
- The Databricks error-event count drops after the fix.

The angle is worth your time when the defensive character of the code makes an incident-driven origin plausible. Skip it
for code that doesn't look defensive.
