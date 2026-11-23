# Change-record template

`/agentics:gate change` produces this structure. The record is the audit trail. In an incident, an audit, or a
regulatory review, future readers must be able to reconstruct these facts exactly:

- What the change was
- Why it happened
- Who approved it
- What the rollback would have looked like

````markdown
# Change Record: <plain-English title>

<!--
Verb-led titles work best: "Rotate production AWS access keys",
"Migrate payments-db to Postgres 16", "Enable rate-limit-v2 feature flag".
-->

- **ID:** <YYYY-MM-DD>-<slug>
- **Classification:** <Standard | Normal | Emergency>
- **Requestor:** <handle>
- **Date submitted:** YYYY-MM-DD
- **Date executed:** YYYY-MM-DD (or "scheduled for YYYY-MM-DD HH:MM UTC")
- **Status:** <Draft | Pending approval | Approved | Executed | Rolled back | Cancelled>

---

## Summary

<!--
One paragraph. What is changing and why. Future readers should grok this
in 15 seconds. Defer detail to the sections below.
-->

<One-paragraph description of the change and its motivation.>

## Scope and blast radius

<!--
Specific. Avoid "all systems" — list what's affected.
-->

- **Systems / services affected:** <list>
- **Environments affected:** <prod | staging | both | specific cluster>
- **Direct user impact:** <count or "all customers" or "internal only">
- **Indirect impact:** <downstream services, batch jobs, integrations>
- **Geographic / tenant scope:** <single region | all regions | specific customers>

## Related artifacts

<!--
Cross-references. The change-record is one node in a larger graph.
-->

- **Related release:** <tag or "n/a">
- **Related PR(s):** <links>
- **Related runbook:** <link to runbook that documents the executable procedure, if applicable>
- **Related incident:** <link, if this change is in response to an incident>
- **Related issue:** <link to tracking issue, if applicable>

## Risk and impact assessment

### Worst plausible outcome

<What's the worst thing that could happen if this goes wrong?>

### Most likely failure mode

<If something goes wrong, what's most likely to be the cause?>

### Reversibility

<!-- Pick one. -->

- [ ] **Fully reversible** — rollback path tested; see Rollback section
- [ ] **Hard to undo** — compensation possible; see Rollback section for compensation steps
- [ ] **Irreversible** — acknowledged in the Irreversibility note under Rollback plan

### Security-sensitive surface?

<!-- Yes if any of: auth, crypto, input handling, IaC, dependencies, secrets, IAM, public-facing endpoints. -->

- **Yes / No:** <answer>
- **Security scan findings (if yes):** <link to the findings, "no high-confidence findings", "not scanned: no diff", or
  "incomplete: <U> unsettled, full depth not run">
- **Scan limits (always, even with no findings):** <the limits line the scan returned. What it read, what did not run,
  and how many candidates it dropped at the confidence bar. A blank here reads as "security cleared it", which no scan
  can establish.>

### Compliance touchpoints

<!-- SOC 2, ISO 27001, GDPR, HIPAA, PCI, sector-specific. -->

- <touchpoint 1 — or "none">

## Approval chain

<!--
Approvers + status. Every approver starts as Pending. A status changes only when
that approver answers (a comment on the posted issue, or an out-of-band update
written to this record).
-->

| Approver        | Role / Reason                        | Status                       | Date         |
| --------------- | ------------------------------------ | ---------------------------- | ------------ |
| <handle / role> | <service owner of <surface>>         | <Pending \| Approved \| N/A> | <YYYY-MM-DD> |
| <handle>        | <security lead — security-sensitive> | <status>                     | <date>       |

<!-- For Standard changes, name the standing policy: -->

> Pre-approved under policy: <link to policy doc>. No individual approvers required.

## Rollback plan

<!--
Same rigor as a release-mode rollback. Stage the command; estimate ETA.
If irreversible, state so loudly.
-->

### Rollback target

<What state would we revert to?>

### Rollback command

**Bash (macOS / Linux):**

```bash
<rollback command, exact, copy-pasteable, placeholders in <angle-brackets>>
```

**PowerShell (Windows):**

```powershell
<paired rollback command>
```

### Time to rollback

<Estimated duration from "decided" to "customer-visible recovery">

### Irreversibility note (if applicable)

<!--
If the change is irreversible, this section is required. State what's
irreversible and what the recovery path is (restore-from-backup runbook, etc.).
-->

> <aspect of the change> is irreversible. Recovery requires <restore-from-backup procedure / accepted risk with no
> recovery>.
>
> Acknowledged by: <handle> on <date> | Acknowledgment open.

## Comms plan

<!--
Who knows, when, through what channel. Drafts attached or linked.
-->

| Audience              | When               | Channel             | Status                   |
| --------------------- | ------------------ | ------------------- | ------------------------ |
| Affected service team | Pre-change (T-24h) | <channel>           | <Drafted \| Sent \| N/A> |
| Engineering broadly   | At-change (T-0)    | <channel>           | <status>                 |
| Customers             | Pre-change (T-7d)  | Status page / email | <status>                 |
| Security team         | Pre-change         | Restricted channel  | <status>                 |
| Compliance / legal    | Pre-change         | Email               | <status>                 |

### Communication drafts

<!-- Inline drafts, or links to where the drafts live. -->

**Internal pre-change:**

```text
<draft>
```

**Customer-facing pre-change (if applicable):**

```text
<draft>
```

**At-change announcement:**

```text
<draft>
```

## Schedule

- **Proposed window:** <YYYY-MM-DD HH:MM UTC> – <HH:MM UTC> (<duration>)
- **Timezone for stakeholders:** <list>
- **Day / time policy compliance:** <pass | exception with reason>
- **Conflicting changes:** <none | <conflict description>>
- **Freeze policies:** <none | <policy name + status>>

## Execution log

<!--
Updated post-execution. Captures what actually happened vs. what was planned.
-->

- **Actual start time:** <YYYY-MM-DD HH:MM UTC>
- **Actual end time:** <YYYY-MM-DD HH:MM UTC>
- **Executor:** <handle>
- **Outcome:** <Succeeded | Partial | Rolled back | Failed>
- **Deviations from plan:** <none | <description>>
- **Post-execution verification:** <links to dashboards, synthetic checks, customer reports>

## Lessons learned

<!--
Updated post-execution. If this was a Standard change, the entry is brief.
If anything didn't go to plan, capture it here for the next change of this shape.
-->

- <bullet, if applicable>

## Change history

<!--
The record itself may evolve as approvals come in, schedules shift, etc.
-->

| Date       | Editor   | Change                                                                  |
| ---------- | -------- | ----------------------------------------------------------------------- |
| YYYY-MM-DD | <handle> | Initial draft                                                           |
| YYYY-MM-DD | <handle> | <Security lead approved \| Schedule moved to ... \| Added rollback ETA> |
````

## Strip the template before publication

When you publish the record, remove every `<!-- ... -->` guidance comment. You publish it when you write it to
`change-records/<YYYY-MM-DD>-<slug>.md` or post it via an adapter. Keep the structure: headers, tables, fenced blocks.
Keep the `# Change Record:` heading exactly: a deploy hook that requires a change record recognizes the file by it.

The published record is read by people who have never seen Agentics. It names no Agentics command, phase or step. Where
it cites a scan, it says what the scan read and its limits.

## Variants

- **Standard change short form.**
    - Sections: Header, Summary, Scope, Related artifacts, brief Risk (one paragraph), Approval (reference standing
      policy), Rollback (link to runbook), Schedule.
    - Skip detailed Risk subsections, individual Approval rows, Comms plan (n/a for pre-approved standing changes).
    - The short form suits high-volume routine changes (e.g., quarterly secret rotation per a runbook).
- **Emergency change short form (post-hoc).**
    - Sections: Header, Summary, Scope, Related incident, brief Risk + Rollback (often "executed; record retroactive"),
      Approval (post-hoc names + dates), Execution log, Lessons learned.
    - Skip Schedule (it already happened) and Comms drafts (already sent).
    - File within 72h of the emergency change for audit traceability.
- **Multi-system change.**
    - Add a "Per-system breakdown" section with a risk + rollback subsection per affected system.
    - Use that section when one change touches three services with independent failure modes.

## Where the record lives

- **Default:** `change-records/<YYYY-MM-DD>-<slug>.md` in the consuming repo
- **A repository the user says does not keep change records:** no file. The record is rendered in chat for the user to
  move.
- **Org-specific path** (e.g., `changes/`, `cab-records/`, `audit/`). Pass the path explicitly. The skill respects the
  user's layout.

## What the record is NOT

- Not a runbook (procedures live in runbooks; change-records reference them)
- Not a postmortem (postmortems follow incidents; change-records precede changes)
- Not a PRD (PRDs scope features; change-records scope operational events)
- Not a deploy log (deploy logs are ephemeral; change-records are the durable audit artifact)
