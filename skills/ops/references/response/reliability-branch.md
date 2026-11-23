# Reliability branch

If the declare-phase branch question gets "no", load this file. It covers an outage, performance regression, data
integrity issue, or other reliability failure with no indication of unauthorized access or malicious activity.

## Reliability triggers (illustrative)

- Service returns 5xx above SLO
- Latency spike beyond P99 SLO ceiling
- Error budget burns at 5–10× normal rate
- Background job queue depth grows without bound
- Database replication lag beyond tolerable window
- Data corruption / inconsistency that user reports or invariant checks show
- Cost anomaly that suggests infinite loop / runaway

## Containment options (preferred order)

Run reversible mitigations first. Run an irreversible one only with explicit IC approval.

| Mitigation                                  | Reversible        | Typical TTR | When to use                                                          |
| ------------------------------------------- | ----------------- | ----------- | -------------------------------------------------------------------- |
| **Feature flag flip**                       | Yes               | seconds     | A flag gates the bad behavior. If applicable, always try this first. |
| **Deploy rollback**                         | Yes               | 5–15 min    | A recent deploy correlates in time with onset                        |
| **Traffic shed (rate-limit, shed-by-tier)** | Yes               | 1–5 min     | Capacity-related — shed lower-tier traffic to preserve higher-tier   |
| **Failover** (region, primary, replica)     | Yes (re-failover) | 5–30 min    | Infrastructure-level fault localized to one zone or instance         |
| **Scale up / scale out**                    | Yes               | 1–10 min    | Capacity-driven; load spike or resource exhaustion                   |
| **Manual data fix**                         | Often no          | varies      | Data corruption — only after evidence preservation and IC approval   |
| **Restart / cycle**                         | Yes               | 1–5 min     | Last resort if you do not yet know why; expect re-occurrence         |

## Rollback playbook

1. **Identify the offending deploy.** What was the most recent change to the impacted surface? Cross-reference deploy
   timestamps with onset.
2. **Confirm the rollback target is known-good.** The previous deploy is the default target. Verify that it is not also
   broken, because the bug may have been latent before the recent deploy exposed it.
3. **Communicate the rollback BEFORE you execute it.** The comms lead notes "rolling back deploy X" in the responder
   channel. Customer comms move to "mitigation in progress" through the normal draft, approve, and confirm chain.
4. **Execute via the project's standard rollback path.** Do not invent a new rollback path mid-incident.
5. **Verify customer-facing metrics recover.** Wait for sustained baseline before you declare containment successful.
6. **Pin the rolled-back version.** Block accidental re-application of the bad deploy. Record the pin in the tracking
   issue.

## Failover playbook

1. **Confirm the failover target is healthy.** A failover to an unhealthy target turns a single-zone incident into a
   multi-zone incident.
2. **Confirm replication state.** For data-tier failover, lag must be tolerable for your RPO. If it is not, the choice
   between data loss and continued outage is an explicit IC call.
3. If your tooling supports it, **drain traffic from the failing target before you flip.**
4. **Update DNS / load balancer / service mesh** as the project's runbooks dictate.
5. **Validate from the customer side.** Use synthetic checks and real-user monitoring. Internal checks can pass while
   customers still see failures.

## Status-page cadence

Use this public-status cadence:

| Severity | First update                           | Subsequent updates               |
| -------- | -------------------------------------- | -------------------------------- |
| P1       | Within 30 min of declaration           | Every 30 min until resolved      |
| P2       | Within 60 min if customer-visible      | Every 60 min if customer-visible |
| P3       | Only on resolution if customer-visible | n/a                              |
| P4       | Not applicable                         | n/a                              |

Each update increments the status field (`investigating` → `identified` → `monitoring` → `resolved`). Use the
customer-facing template in [../communication-templates.md](../communication-templates.md). Do not speculate on cause in
the public update.

## Customer-impact assessment

Quantify impact at each comms cycle with the highest-resolution number you have:

- **Request-level:** % of requests that fail, % above latency SLO
- **User-level:** number of users affected, geographic distribution
- **Transaction-level:** orders failed, sign-ups blocked, payments stuck
- **Revenue-level:** P1 only, used in exec brief, not customer comms

Update the impact field in the tracking issue each time the number changes materially.

## Error-budget burn check

For services with SLOs, check error-budget burn rate at containment:

- **Burn rate ≤ 1×:** acceptable; no action beyond standard postmortem
- **Burn rate 1–5×:** slowdown indicated; postmortem action items should reduce risk of recurrence
- **Burn rate 5–10×:** halt non-essential deploys until postmortem and mitigation
- **Burn rate >10×:** policy-driven freeze on deploys to the impacted surface; treat as engineering-wide priority

The IC notes burn rate in the tracking issue. The IC flags freezes to the comms lead for engineering-channel posting.

## Performance-regression specifics

Latency / throughput regressions often lack a clear "rollback this deploy" target. That leaves fewer containment
options:

1. **Capacity-add** to absorb the regression (scale out, increase resources).
2. **Disable the new code path** if feature-flagged.
3. **Cache aggressively.** Increase TTLs, pre-warm, and fall back to stale-while-revalidate.
4. **Shed lower-priority traffic** to preserve customer-visible paths.

When none of these applies, no containment option remains: Phase 4 hands the root cause to `diagnose` with the impact
still running. Performance regressions almost always need diagnosis, not just containment.

## Data-integrity specifics

If invariant checks fail or user reports show corrupted data:

1. **Stop the write path** to the affected data first. Switch to read-only mode, or feature-flag the writer off.
2. **Snapshot before any data modification.** It applies on this branch too, because you may need to reconstruct what
   was lost.
3. **Investigate scope.** How many rows / objects / users? When did corruption start? Is the corruption ongoing or
   stopped?
4. **Plan repair.** Options include a migration script, restore-from-backup, and manual reconciliation. Rehearse on a
   copy before you run it on live data.

## Hand-off triggers (re-evaluate the branch)

Evidence that suggests security involvement includes any of these:

- Authentication anomalies
- Unexpected outbound traffic
- Logs that show access patterns inconsistent with normal app behavior
- Findings from a security scanner that align with the incident timeline

If new evidence at any phase suggests security involvement, say once what was observed and ask the branch question
again. On yes or unsure:

1. Re-declare on the security branch.
2. Name the security on-call and the security lead to page.
3. Load [security-branch.md](security-branch.md).

Reliability containment may continue, but security-branch comms restrictions and forensics preservation now apply.
