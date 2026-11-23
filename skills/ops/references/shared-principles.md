# Shared ops principles

SKILL.md loads this file for every `ops` mode once it dispatches one.

## Incidents

### Severity (P1 to P4)

This table is the one definition of `P1` to `P4`. Other skills that name an incident priority link here. Read the rows
from the top; the first row whose reliability or security trigger matches sets the severity.

| Level | Reliability trigger                                                                                     | Security trigger                                                        | On-call response                            | Comms                                                               |
| ----- | ------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------- | ------------------------------------------- | ------------------------------------------------------------------- |
| P1    | Outage for all or most users; data loss; severe data-integrity damage                                   | Confirmed breach; active exploitation; data exfiltration                | Page the IC at once; all hands              | Status page and customer comms within 30 min; exec brief; legal     |
| P2    | Partial outage; major degradation: error rate above SLO, or latency above 2× baseline for 5 min or more | Suspected breach; a credential leaked in a private scope                | Page the IC; assemble a small responder set | Internal update; status page when users can see it                  |
| P3    | Any other degradation that reaches users, however few                                                   | Low-risk finding, such as a vulnerable dependency with no known exploit | Respond within 4 hours without paging       | Internal channel; status page only on resolution, when users saw it |
| P4    | Otherwise: no user impact (cosmetic, observability gap, near-miss, isolated bug)                        | Otherwise: an informational finding, such as a hardening opportunity    | Track for postmortem trend analysis         | None                                                                |

**Severity changes as the incident does.** Re-declare upward as scope or impact grows. Downgrade at a checkpoint the IC
names. Log every change in the incident record.

**The user's declaration stands.** When the user declares a severity other than the table's row, declare theirs, say the
row once, and record both: "declared P3 at user direction; table indicates P2".

### Reliability or security

The declare phase of [response/response-workflow.md](response/response-workflow.md#phase-1-declare) asks the branch
question. Signals that flip a reliability incident into security, and that make the skill say once what it observed and
ask the branch question again:

- Unexplained authentication anomalies (mass-failure, geographic, off-hours)
- Outbound traffic to unfamiliar destinations
- New / modified accounts, keys, or roles not tied to a known change
- Logs that show access to data the impacted service does not normally read
- A vulnerability disclosure or scanner finding that matches the incident timeline
- Anything that "doesn't look like our app's normal behavior"

### Owner roles

A single incident has up to four named roles. A security incident adds two more, listed below. For P3 and P4, one person
can hold more than one role. For P1 and P2, different people should hold the roles when they are available; one
responder holding every role is still a valid incident.

| Role                        | Responsibility                                                                            |
| --------------------------- | ----------------------------------------------------------------------------------------- |
| **IC** (Incident Commander) | Owns the incident. Sets priorities. Makes calls. Single source of truth for severity.     |
| **Comms lead**              | Drafts and sends all status updates. Owns customer-facing and exec comms cadence.         |
| **Scribe**                  | Maintains the timeline (events, decisions, attempted remediations) in the tracking issue. |
| **Subject expert(s)**       | Domain responder(s) who do the actual debugging / containment work.                       |

When another responder is present, the IC does not debug: an IC who starts debugging hands the IC role to someone else
first. A lone responder holds every role and debugs too.

For security incidents, add:

| Role                 | Responsibility                                                                          |
| -------------------- | --------------------------------------------------------------------------------------- |
| **Security lead**    | Owns containment decisions (revoke, isolate, rotate). Coordinates with legal if needed. |
| **Forensics scribe** | Preserves evidence (snapshots, logs, hashes) before any system is mutated.              |

### Blameless framing

This rule applies in response, postmortem, and drill modes:

- **Focus on systems, not people.** Say "The deploy script bypassed the canary check," not "Alex bypassed the canary
  check."
- **Assume good intent.** Every action made sense at the time given what the operator knew.
- **Remove names from causal claims.** Names appear in the timeline, not in "root cause" or "contributing factors."
- **Blameless does not mean incomplete.** A material contributing factor stays in the draft, reframed in terms of the
  system ("the alert had no escalation path", not "we ignored the alert"). Dropping it, or writing a cause the session's
  evidence contradicts, is held by the status-honesty rule in [SKILL.md](../SKILL.md#status-honesty).
- **Mistakes are visible symptoms of system gaps.** If a person could make the mistake, the system allowed the mistake.

If a postmortem reads like a list of who-did-what-wrong, it failed.

### Evidence preservation (security-incident foundation)

Before any containment action that mutates state, the forensics scribe captures:

- **Timestamps:** event time + observation time + remediation time, in UTC
- **Process and network state:** `ps`, `netstat`/`ss`, `lsof` output where applicable
- **Logs:** application, system (auth, syslog), and cloud audit (CloudTrail, Audit Logs) for the relevant time window.
  Pull them to a separate, write-protected location.
- **System images:** for compromised hosts, snapshot before isolation where forensic value is high
- **Hashes:** of any artifact suspected to be malicious (binaries, scripts, packages)
- **Chain of custody note:** who pulled what, when, where it is stored

Containment does not wait indefinitely on evidence. The IC sets a clock, typically 5–15 min for a snapshot-and-go
pattern. Partial forensics is better than no forensics.

### Scope discipline

A single `/agentics:ops response` session drives **one** incident. If a second incident emerges:

- Open a second tracking issue.
- Either hand the new incident to a separate IC or explicitly suspend the first to handle the second.
- Keep the timelines and comms of the two incidents separate.

### False-positive policy (reliability)

If the alert that triggered the incident turns out to be a false positive:

- Do not silently close the incident. The postmortem still applies. The alert is the visible symptom.
- Treat alert noise as an incident in its own right (typically P3). Add an action item to tune the alert.

### False-positive policy (security)

If a suspected security incident turns out to be benign (e.g., a real but expected admin action that looked anomalous):

- Document why the signal looked malicious and how it was ruled out.
- Add the pattern to the detection-tuning backlog so the next legitimate instance does not trigger again. Do this only
  if the tuning does not hide a real attacker variant.

## Runbooks

### What a runbook is for

A runbook captures a **procedure** the team needs to execute reliably. It is not documentation, training material, or
post-hoc explanation. The audience is an operator under time pressure who:

- May not be the runbook's author
- May not be deeply familiar with the system
- Cannot afford to be wrong

If a reader cannot execute the procedure successfully under stress, the runbook has failed, regardless of how
technically accurate it is.

### What makes a good runbook

1. **A single, clear trigger.** "Use when X" should be unambiguous. If the trigger is fuzzy ("use when something feels
   off"), operators will invoke the runbook at the wrong times.
2. **Numbered, atomic steps.** Each step does one thing. The operator can check off each completed step before the next
   one.
3. **Verify after each step.** Tell the operator how to confirm each command had the intended effect.
4. **Commands ready to run.** Do not bury an instruction in prose, such as "you'll want to update the deployment." Give
   the exact command, with placeholders clearly marked.
5. **Rollback paths.** Every state change has an undo. If undo is impossible, say so in a clear warning.
6. **Escalation triggers.** "If step N fails after retry, page <role>. Don't keep trying."
7. **A last-tested date.** Untested runbooks decay. Readers use the date to judge how far to trust the runbook.
8. **An owner.** A real person. "Platform team" is not an owner.

### What makes a bad runbook worse than no runbook

- **Wrong, but confidently written.** A reader follows it through three steps before they realize the fourth step
  references a service that was deleted in 2024. They are now in a half-changed state with no rollback.
- **Vague enough to require interpretation.** "Verify the data layer is healthy." The operator reads it one way, and the
  original author meant another. Both readings are reasonable. The system is now in an inconsistent state.
- **No rollback.** Mid-procedure failure leaves the operator stranded.
- **Stale commands.** The runbook says `kubectl get pods -n payments`; the namespace was renamed to `payments-prod` six
  months ago. The command returns "no resources found". The operator concludes the service is dead and starts a
  wrong-direction recovery.
- **Owner gone.** The named owner left the company 18 months ago. There is no one to ask when a question comes up.

The audit mode (`/agentics:ops audit`) and drill mode (`/agentics:ops drill`) exist to catch these failure modes before
an operator hits them at 3 a.m.

### The verify-after-each-step pattern

Every step in a runbook should follow this shape:

```
N. <Action, one sentence>

   <Command, exact and copy-pasteable, with placeholders in <angle-brackets>>

   **Verify:** <How to confirm the action succeeded, a specific observable signal>

   **If verify fails:** <Next step, usually a rollback or escalation>
```

The pattern is always required.

### Rollback discipline

A runbook without a rollback section is incomplete. Each step falls into one of three categories:

| Step type        | Rollback expectation                                                                    |
| ---------------- | --------------------------------------------------------------------------------------- |
| **Reversible**   | Document the exact undo command. (Most steps.)                                          |
| **Hard to undo** | Document the compensation, such as a restore from backup or a replay from an event log. |
| **Irreversible** | Say so explicitly. Require a confirmation gate before the step runs.                    |

An irreversible step with no warning to the operator is a runbook bug. The audit mode catches it.

### Staleness signals

A runbook is stale when any of these is true:

- **Last-tested date older than the staleness threshold** (default: 90 days for P1-grade runbooks, 180 for P2, 365 for
  P3+).
- **Command drift:** commands reference tools, paths, namespaces, environment variables, or services that no longer
  exist.
- **Dead links:** links to dashboards, docs, or external resources return 404.
- **Owner gone:** the named owner is not in `CODEOWNERS`, the organization directory, or the active contributors.
- **Trigger-system drift:** the alert the runbook reacts to was renamed or removed.
- **Edit-age drift:** the runbook has not been edited in 12+ months while the system it covers is actively developed. It
  is likely out of sync with the code, even if no individual step is obviously broken.

### Cross-platform commands

A runbook serves operators on macOS, Linux and Windows. A command that is the same in Bash and PowerShell (most
`kubectl`, `gh`, `git` and `terraform` calls) is written once. A command whose syntax differs (environment variables,
pipes, path separators, `systemd` against `sc.exe`) gets one labeled block per shell:

````markdown
**Bash (macOS / Linux):**

```bash
export KUBECONFIG=<path>
```

**PowerShell (Windows):**

```powershell
$env:KUBECONFIG = '<path>'
```
````

If a runbook is platform-specific (a Windows-only or Linux-only procedure), do these steps:

1. State that explicitly in the **Prerequisites** section.
2. Include only the relevant block.

### When to write a runbook

- A procedure has been done manually more than twice and will be done again.
- An incident postmortem identifies a missing runbook as an action item.
- A drill (`/agentics:ops drill`) reveals a gap.
- A new service ships and has at least one expected operational event (deploy rollback, failover, scaling).
- A compliance or audit requirement demands a documented procedure (change-control records reference runbooks).

### When NOT to write a runbook

- A one-off operation that will not recur. Capture it in a postmortem timeline or commit message instead.
- A procedure that is fully automated and has no manual fallback. Document the automation. Do not duplicate it in a
  manual runbook unless the automation can fail and need human takeover.
- A procedure so common and stable that it is part of normal development workflow (e.g., "how to run tests"). That is
  developer docs, not a runbook.

## Composability with other skills

- **Upstream signal sources:** `triage` (a flagged PR or check-run failure that escalates), automated alerts, customer
  reports.
- **Inside response:** `diagnose` for root-cause; `/agentics:review axes <ref> --security --composed` for
  security-branch evidence (envelope drafted, not auto-posted); `gh` for artifact operations.
- **Downstream:** `gate` for remediation branch close-out; `docs` for long-form postmortem.
