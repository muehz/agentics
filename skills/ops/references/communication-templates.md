# Communication templates

Drafts for the comms lead. The skill drafts every template and presents it to the user. It **posts a template only after
explicit confirmation**. Edit the fields in `<>` before you confirm.

## Internal status update (Slack / Teams / chat)

Use it for the responder channel and the broader engineering channel. Cadence: every 30 min for P1, every 60 min for P2,
on status change for P3.

```
🚨 INCIDENT UPDATE — <P1|P2|P3> — <short title>

Status: <investigating | identified | mitigating | monitoring | resolved>
Started: <UTC timestamp>
Impact: <user-facing / internal / unknown — one line>
Current actions: <one or two bullet points>
Next update: <UTC timestamp or "on status change">

Tracking: <link to tracking issue>
IC: <name or handle>
```

## Customer-facing status update (status page)

Write it in plain, non-technical English. Update it each time `Status:` changes. Do not name a cause until it is
confirmed.

```
<Investigating | Identified | Monitoring | Resolved> — <plain-english title>

We are currently <action verb: investigating | working on a fix | monitoring after a fix> an issue affecting <product surface, user-visible only>.

<One sentence on user-visible impact, e.g. "Some users may see slow page loads or errors when signing in.">

We will post the next update by <UTC timestamp + format e.g. "16:30 UTC">.
```

**Resolved variant:**

```
Resolved — <plain-english title>

The issue affecting <surface> has been resolved as of <UTC timestamp>. <One sentence on what users should see now.> A postmortem will be published within <SLA window — typically 5 business days for P1, 10 for P2>.
```

## Exec brief (P1 only)

Send it to leadership at declaration and at each later severity change or resolution. Keep it to ~5 lines.

```
P1 INCIDENT — <short title>

What: <one sentence — what is broken, who is impacted>
Started: <UTC timestamp>
Impact: <quantified if possible: "~12% of API requests failing", "checkout disabled for all users">
Status: <investigating | mitigating | monitoring | resolved>
ETA to mitigation: <best estimate or "unknown — investigating">
IC: <name>

Tracking: <link>
```

## Security-incident restricted comms

**Use only on the security branch.** Name every recipient explicitly. Each must have a need-to-know. Do not post to
general engineering channels until the security lead approves.

```
🔒 RESTRICTED — SECURITY INCIDENT — <severity> — <internal-only short title>

Recipients: <explicit list of names / roles — e.g. "security-oncall, sre-oncall, eng-lead, legal-counsel">
Status: <triaging | containing | recovering | resolved>
Initial signal: <one sentence>
What is currently known: <2–4 bullet points, evidence-based only>
What is currently unknown: <2–4 bullet points — explicit gaps>
Current actions: <one or two bullets>
Forensics state: <"snapshots in progress" | "preserved" | "n/a">

DO NOT discuss outside this list. DO NOT post in public channels. Coordinate further comms through <security lead name>.

Tracking (private): <link to private/restricted issue or doc>
IC: <name>
```

**Why restricted.** A broadcast of "we think we have a breach" tips off attackers and contaminates evidence. It can also
trigger premature regulatory clocks.

## Status-page snippet for security incident (post only with security lead approval)

Use this skeleton when a security incident needs external comms. It does **not** speculate on cause.

```
We are investigating reports of <user-visible symptom — e.g. "unusual account-access notifications">. Out of caution, we have <action taken — e.g. "logged out all sessions and required re-authentication">. We will post the next update by <UTC time>.
```

## Internal-channel "we're paged, standby" (within 5 min of declaration)

Post it to the responder channel before the team knows much.

```
🚨 <P1|P2|P3> incident declared — <short title>. Investigating now.

What we know: <one line, often "alert fired — root cause unknown">
IC: <name>
Channel for updates: <#channel>

Standby; first real update by <UTC time, typically +30 min>.
```

## Postmortem-publish announcement

Send it when the postmortem is published, typically within the per-severity SLA.

```
📝 Postmortem published — <incident title> (<incident ID>)

Severity: <P1|P2|P3>
Customer impact: <one line>
Root cause summary: <one to two sentences>
Top action items:
- <highest-leverage item with owner and due date>
- <next item>
- <next item>

Full postmortem: <link>
```
