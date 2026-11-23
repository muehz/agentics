# Postmortem template

`/agentics:ops postmortem` produces this structure. Each section has guidance comments for authors.

```markdown
# Postmortem: <plain-english title>

**Incident ID:** <e.g., issue #1234 from response-mode tracking> **Severity:** <P1 | P2 | P3 | P4> **Branch:**
<reliability | security | both> **Date:** <YYYY-MM-DD of incident> **Duration:** <H hours M minutes — declaration to
resolution> **Postmortem owner:** <name> **Postmortem date:** <YYYY-MM-DD of writing> **Distribution:** <public |
engineering | restricted>

---

## Summary

<!--
2–4 sentences. The single most-read section. Should be readable by an engineer
who wasn't on the incident and tells them: what was the customer-visible impact,
what was the root cause at a high level, what changed.

Do NOT speculate beyond what the investigation found.
-->

<Plain-English summary of impact, root cause, and resolution.>

## Customer impact

<!--
Quantify. Use the highest-resolution number you have access to.
Numbers anchor severity for future readers and feed trend analysis.
-->

- **Duration:** <UTC start time> → <UTC resolution time> (<H hours M minutes>)
- **User impact:** <e.g., "~12% of API requests failed", "checkout disabled for all signed-in users in EU region", "no
  impact — internal observability gap only">
- **Transactions affected:** <e.g., "~340 failed checkouts, ~1,200 retried successfully">
- **Revenue impact:** <internal-only for P1; omit for lower severity>
- **Data impact:** <e.g., "no data lost", "23 records required reconciliation", "n/a">

## Timeline (UTC)

<!--
Strict chronological order. Keep this section descriptive. Leave "why" out of it,
it goes under root cause / contributing factors. Cite evidence inline.

Convert any responder-channel chatter or chat-app timestamps into UTC.
-->

| Time  | Event                                                  | Notes                  |
| ----- | ------------------------------------------------------ | ---------------------- |
| HH:MM | Trigger event (deploy, traffic spike, scanner finding) | <evidence link>        |
| HH:MM | Alert fired                                            | <alert link>           |
| HH:MM | First responder acknowledged                           | <handle>               |
| HH:MM | Incident declared at <severity>                        | IC: <handle>           |
| HH:MM | First customer-facing comms posted                     | <link>                 |
| HH:MM | <Mitigation 1> attempted                               | Effect: <observed>     |
| HH:MM | <Mitigation 2> attempted                               | Effect: <observed>     |
| HH:MM | Containment achieved                                   | <evidence>             |
| HH:MM | Root cause identified                                  | <evidence>             |
| HH:MM | Fix deployed                                           | <commit / deploy link> |
| HH:MM | Customer-visible recovery confirmed                    | <metric link>          |
| HH:MM | Incident resolved                                      | Final comms posted     |

## Root cause

<!--
The deepest technical or process cause. Apply the 5 whys until the answer is
structural (a missing test, a wrong assumption baked into a system, an
operational gap) — not behavioral. "Human error" is not a root cause.

Frame in system terms. Example:
  ❌ "The on-call engineer didn't notice the staging deploy had failed."
  ✅ "Staging deploy failures emit a warning-level log but do not page; the
     deploy pipeline allows promotion to production with a warning state."
-->

<One to three paragraphs identifying the root cause, framed in system terms.>

## Contributing factors

<!--
Things that made the incident worse, longer, or harder to detect. Often more
numerous than root causes. Each factor gets one paragraph and links to evidence.
-->

### <Factor 1 — short title>

<Description, framed in system terms. Cite evidence.>

### <Factor 2>

<Description.>

### <Factor 3>

<Description.>

## What went well

<!--
Real positives only. What genuinely worked. Strong detection, fast comms,
effective coordination, useful tooling. Naming the wins is part of why the
team is willing to do honest postmortems.
-->

- <Bullet — concrete and specific.>
- <Bullet.>
- <Bullet.>

## What didn't go well

<!--
Counterpart to "what went well." Framed as system-level, not person-level.
These often map to action items below.
-->

- <Bullet.>
- <Bullet.>
- <Bullet.>

## Action items

<!--
Each item: a specific change, a named owner, a due date. No-owner items do not
ship. "Platform team" is not an owner — name a specific person.

One item is required: a regression test or eval that fails on this incident's
defect. When the incident had no defect to test, such as a provider outage with
no change on our side, write `n/a: <reason>` in its Action cell instead and
leave its Owner, Due, and Issue cells as "—". A postmortem without this item
is incomplete.

Each action item should also have a corresponding GitHub issue (linked).
-->

| #   | Action                                                                                               | Owner    | Due        | Issue |
| --- | ---------------------------------------------------------------------------------------------------- | -------- | ---------- | ----- |
| 1   | Regression test or eval: <the test or eval that fails on this incident's defect>, or `n/a: <reason>` | <handle> | YYYY-MM-DD | #<n>  |
| 2   | <Specific change — e.g., "Add SLO-based alert for payment-API 5xx rate">                             | <handle> | YYYY-MM-DD | #<n>  |
| 3   | <Action>                                                                                             | <handle> | YYYY-MM-DD | #<n>  |

## Detection and mitigation latency

<!--
Optional but valuable. Quantify the gaps that matter for trend analysis.
-->

- **Time to detect** (trigger → alert): <duration>
- **Time to acknowledge** (alert → first responder): <duration>
- **Time to declare** (acknowledge → incident declared): <duration>
- **Time to mitigate** (declare → containment): <duration>
- **Time to resolve** (mitigate → resolution): <duration>

## Evidence appendix

<!--
Links to artifacts that future readers will want. Dashboards lose data; capture
screenshots if retention is a concern.
-->

- Tracking issue: <link>
- Alert that fired: <link>
- Dashboards (peak impact): <links / screenshots>
- Deploy diff: <link>
- Logs: <link or excerpt>
- Customer reports: <links — anonymized if external visibility>

## Security-specific notes

<!--
Security incidents only. Delete this whole section for any other incident.
Restricted-distribution version only. Strip from any public version.
-->

- **IOCs observed:** <list — never publish externally>
- **Forensics state:** <preserved | partial | none>
- **Disclosure path:** <internal | customer notification | regulatory>
- **Detection-tuning gaps identified:** <bulleted list>
```

## Strip the template before publication

When you publish the postmortem, remove all `<!-- ... -->` HTML comment blocks. Keep the structure, with its headers and
tables. Strip only the explanatory comments. Then delete each optional or security-only section you did not fill in. Do
not publish a section that still holds placeholders.

## Variants

- **P3/P4 short form.** Keep summary, customer impact, timeline, root cause, and action items. Keep the regression test
  or eval item in the action items. Skip the "contributing factors," "what went well/didn't," and detection-latency
  sections.
- **Security restricted variant.** Keep the security-specific notes section above. Strip indicators of compromise from
  any version distributed beyond restricted scope.
- **Multi-team variant.** When multiple teams contributed, add a "Team-specific findings" section with one subsection
  per team. The per-team split keeps cross-team blame out of the findings.
