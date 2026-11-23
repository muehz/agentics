# Monitor window

Phase 3 of `/agentics:gate release`. After the user says the deploy has started, gate opens a window of observation,
then closes it on the success path or escalates to `/agentics:ops response`.

Gate cannot see dashboards unless the session has an observability tool, and it cannot speak until the user writes. It
suggests what to watch, and it acts on what the user reports in their next message. It sets no timer and promises no
message at a later time, as [unattended.md](../../../routing/references/contracts/unattended.md#what-a-skill-does)
requires.

## Window length

State the length when the window opens; ask no question. The user changes it by saying so.

| Risk profile (the first row that matches wins)                      | Default |
| ------------------------------------------------------------------- | ------- |
| An irreversible migration was confirmed, or a most-critical service | 120 min |
| A migration, a security-sensitive path, or a large diff             | 60 min  |
| A short-form deploy, as the workflow's eligibility test decides     | 15 min  |
| Any other deploy                                                    | 30 min  |

## What to watch

Base the suggestion on the surface Section 1 of the checklist computed:

| Surface         | What to watch                                                                            |
| --------------- | ---------------------------------------------------------------------------------------- |
| Web or API      | Error rate, latency at P50, P95 and P99, request volume, 5xx by endpoint, real-user data |
| Background jobs | Queue depth, job success and failure rate, processing latency, dead-letter queue         |
| Database        | Connection pool saturation, replication lag, query latency, error rate                   |
| Frontend        | JavaScript error rate, page-load timings, conversion funnel                              |
| Infrastructure  | Pod or container health, restart counts, resource use                                    |

A deploy that spans several surfaces lists the rows for each. With an observability tool in the session, read the
metrics yourself and report what you read; the user's report still closes the window.

For a customer-visible deploy, the status page stays current through the window, even when the update only says
"deployed; monitoring". Silence on the status page reads to customers as an outage.

## Anomaly classification

Classify each observation the user reports. The rows are checked in order and the first match wins; an observation that
matches no row is escalated, because an unneeded incident response costs less than a missed incident.

| Observation                                                | Severity guess | Action                                   |
| ---------------------------------------------------------- | -------------- | ---------------------------------------- |
| A customer reports broken behavior                         | P1 or P2       | Escalate.                                |
| Error rate above twice the baseline for 5 minutes or more  | P1             | Escalate.                                |
| P99 latency above twice the baseline for 5 minutes or more | P2             | Escalate.                                |
| Queue depth growing without bound                          | P1 or P2       | Escalate.                                |
| One error spike that recovered within 60 seconds           | P3 or noise    | Note it, and keep the window open.       |
| An alert that fired and recovered on its own               | P3             | Note it, and keep the window open.       |
| Resource use at its ceiling but stable                     | watch          | Keep the window open; reassess at close. |
| Unexpected log volume                                      | watch          | Keep the window open; name it at close.  |

The severity guess is a starting point for `/agentics:ops response`, which owns the severity.

## Escalation

1. **Close the window.** Gate stops saying "monitoring" while the user works the incident.
2. **Render the hand-off context** in one block:

    ```text
    Anomaly during the monitor window. Closing the window; /agentics:ops response takes it from here.

    Artifact: <ref> (<short SHA>) to <target>
    Deploy started: <UTC>, <duration> ago
    Anomaly: <the user's description>
    Severity guess: <P1 | P2 | P3>
    Rollback: <staged in Section 4 | not staged: <the reason the checklist recorded>>
    ```

3. **Forward once** to `/agentics:ops response`, as the workflow says.
4. **Stop monitoring.** Gate does not re-monitor this release.

## Rollback shortcut

For a severe anomaly where rolling back is plainly right, such as a customer-visible outage, the user may want to roll
back before the incident workflow. When the user asks for the rollback, in any wording:

1. Show the rollback command that [rollback-readiness.md](rollback-readiness.md#step-3-stage-the-rollback-command)
   staged. The user runs it; gate never does.
2. When the user reports the rollback finished, forward to `/agentics:ops response` for the follow-up. A rollback
   restores service; it does not explain the cause.

## Closing on the success path

When the user reports no anomaly at or after the end time, or asks to close the window early:

1. **Report the close** without claiming success the run did not observe:

    ```text
    Monitor window closed at <UTC> after <N> min. You reported no anomaly.
    ```

    An early close says so and names the minutes that were not watched.

2. **Offer the follow-ups** as one batch for one confirmation: the "deploy complete" comms updates of Section 7, and the
   close comment on the release tracking issue when one exists. A follow-up the user named earlier needs no second
   question.
3. **Suggest `/agentics:gate branch`** when a release branch must be merged back and cleaned up.

## Extending the window

When the user wants more time, for example to watch a slow trend, extend by the minutes they name and state the new end
time. After the third extension in one release, suggest once that `/agentics:ops response` can track the open question
more formally than another extension.

## Out of scope

- **Polling metrics on a schedule.** The window acts on the user's messages.
- **Several environments in one run.** One `/agentics:gate release` covers one environment. A promotion from staging to
  production is a second run.
- **The incident itself.** `/agentics:ops response` owns it.
