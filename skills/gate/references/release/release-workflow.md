# Release workflow

Release mode runs three phases: the pre-deploy gate, the user's deploy, and the monitor window. Gate never deploys and
never rolls back. It checks, stages the commands, and keeps the record. The one-line announcement is the one
[SKILL.md](../../SKILL.md#which-mode) defines.

## Inputs

- **The release artifact:** a tag (`v2.1.0`), a commit SHA, or a release-candidate build ID.
- **Optional:** the deploy target, which gate infers from the repository or asks for, and the monitor window length.

With no artifact named, ask one question in the
[question format](../../../routing/references/contracts/question-format.md):

> ❓ **Q1** - **Release artifact**: Which artifact is deploying? A tag, a commit SHA, or a build ID, for example
> `v2.1.0`.
>
> ➡️ The release tag at `HEAD` when there is one; otherwise the `HEAD` commit SHA.

## Phase 1: Pre-deploy gate

Before this step, the **Check principles before verification** rule in
[SKILL.md](../../SKILL.md#cross-cutting-rules-all-four-modes) has run.

Run every section of [pre-deploy-checklist.md](pre-deploy-checklist.md), in order:

1. Artifact identification, with the change-record lookup
2. Migrations
3. Feature flags
4. Rollback readiness, through [rollback-readiness.md](rollback-readiness.md)
5. CI and security scans
6. On-call readiness
7. Comms drafts
8. Deploy window

### One pass, one confirmation

Run all eight sections before you report. Collect every question the sections raise (the rollback target when it cannot
be inferred, staging, schema compatibility, a restore runbook, full-depth security when the fast scan is incomplete, the
on-call, the window) into one round, asked with the
[question-format collection rules](../../../routing/references/contracts/question-format.md#collecting-answers). Then
render the checklist once, as the go or no-go summary below, and take one confirmation at its end.

The run stops in the middle of the pass only for these two:

| Stop                                               | What happens                                                                                    |
| -------------------------------------------------- | ----------------------------------------------------------------------------------------------- |
| The user does not confirm the artifact (Section 1) | The run ends with no further check, and writes the verdict row with status `cancelled`.         |
| An irreversible migration (Section 2)              | One confirmation at that point, by a reply that names the migration or the irreversible action. |

A failed item does not stop the pass. The remaining sections still run, so the summary shows everything that needs
fixing at once.

### Item outcomes

Every checklist item ends in exactly one of these outcomes. The rows are written through `agx flow log`, as
[emission.md](../emission.md) says; `<item>` is the section's name from the list above (`artifact`, `migrations`,
`flags`, `rollback`, `ci`, `on-call`, `comms`, `window`), followed by `:<check>` when the section has more than one
check, in lowercase words joined by hyphens, for example `ci:required-checks`, `ci:security-scan` or
`migrations:staging`.

| Outcome                                                                   | Reported as      | Row                                    |
| ------------------------------------------------------------------------- | ---------------- | -------------------------------------- |
| Checked in this run, or with a pointer to a run of it, and it holds       | `pass`           | none                                   |
| The item does not apply to this artifact, for example no migrations       | `n/a: <reason>`  | `na <item>`                            |
| The item applies, and this run could not check it (no access, no pointer) | `skip: <reason>` | `skip <item>`                          |
| The item was checked and does not hold                                    | `fail: <reason>` | none, until the user decides           |
| The user proceeds past a `fail` or a `skip` item                          | unchanged        | one `waive <item>` row, at that moment |

The user's statement about their own organization (who is on call, which window the team approved) is taken as true and
reported as `stated by the user`. A statement that a check ran or passed (staging ran, the scan is clean) is a claim of
status: with a pointer to the run it is `pass`, and without one it is `skip: stated by the user, not verified`. A claim
of status from no evidence and no statement is never reported as `pass`.

### Go or no-go summary

The summary is the checklist's one report. Its first line is `GO` when every item is `pass` or `n/a`, and `NO-GO`
otherwise.

```text
<GO | NO-GO> — pre-deploy gate for <ref> (<short SHA>) to <target>

Artifact: <ref>, <N> commits since <previous tag>; surface: <areas>
Change record: <path or URL, status as the record states it | none found>
Migrations: <pass | n/a | skip | fail>: <one line>
Flags: <result>: <one line>
Rollback: <result>: target <ref>, about <duration> to recover
CI: <result>: <N/N required checks green; security scan result, any incomplete mark, and its limits line>
On-call: <result>: <who, reachable, briefed>
Comms: <result>: <drafts ready | n/a>
Window: <result>: <inside the approved window, no conflicting deploy>

<For NO-GO: one line per failed or skipped item, each with the fix that would clear it.>
```

**On GO,** the confirmation of the summary moves the run to Phase 2.

**On NO-GO,** the checklist is a process gate, as
[gates.md](../../../routing/references/contracts/gates.md#process-gate) defines. Name its cost once: the deploy goes out
with the listed items unchecked or failing. Offer the two exits: fix the items and re-run `/agentics:gate release`, or
proceed anyway. A reply that proceeds, in any wording, writes one `waive <item>` row for each failed or skipped item
with the user's reason or `none given`, and moves the run to Phase 2. Ask for no reason. The summary, the deploy summary
and the verdict row still read those items as failed or not verified.

An unconfirmed irreversible migration is the one item a proceed reply does not waive. It is a destructive step, so it
stays held at the point of action, as
[approval.md](../../../routing/references/contracts/approval.md#the-two-exceptions) says: ask once more for the reply
that names the migration. That reply confirms it and the run moves on; any other reply ends the run before the deploy
hand-off, with the verdict row's status `failed`.

A re-run starts the checklist fresh. Conditions change during a fix, so no earlier section's result carries over.

### Short form

A small, low-risk deploy renders a shorter summary and gets a shorter default monitor window. Every item still runs.

**Eligibility.** Every condition holds, each checked against the artifact's diff and CI state in this run:

- no database, schema, or data migration;
- no change to auth, secrets handling, cryptography, or infrastructure code;
- no dependency upgrade and no feature-flag default change;
- every required CI check green on the artifact, none skipped;
- rollback is one known step (redeploying the previous artifact, or one flag flip) already used on this target;
- inside an approved window, with no conflicting deploy in flight.

When every condition holds, the summary puts the eligibility result on top and gives one line per section. The monitor
window defaults to 15 minutes. When any condition fails or cannot be checked, the run uses the full summary, and the
window takes the first default that matches in [monitor-window.md](monitor-window.md#window-length): 120 or 60 minutes
for a riskier deploy, 30 otherwise.

## Phase 2: The user deploys

1. **Render the deploy command.** Take it from the repository's conventions or ask the user for it. Write it once when
   it is the same in Bash and PowerShell 7, and give a second block only where the syntax differs.
2. **Hand it over.**

    ```text
    Ready to deploy <ref> to <target>. I will not run this command.

    <deploy command>

    Run it yourself, and tell me when the deploy has started.
    ```

3. **Wait for the user's next message.** Any reply that says the deploy started opens the monitor window. A reply that
   says the deploy failed to start skips the monitor window and goes to the [escalation](#escalation) below.

Gate never runs the deploy command, and a request to run it is forwarded once, as
[forwarding.md](../../../routing/references/forwarding.md) describes, to the user's own pipeline. Running a deploy is an
outward, irreversible effect that the release mode does not own.

## Phase 3: Monitor window

[monitor-window.md](monitor-window.md) holds the full guidance: the window length, what to watch, how to classify an
observation, and the rollback shortcut.

1. **Open the window.** State its length and its end time in UTC, the dashboards to watch, and that gate acts on the
   user's next message. Gate sets no timer and sends no message on its own: it has no way to speak until the user
   writes.

    ```text
    Monitor window open: <N> min, from <UTC start> to <UTC end>.

    Watch: <dashboards and metrics for this surface>.

    Tell me what you see at any point. After <UTC end>, tell me whether anything looked wrong, and I will close the window.
    ```

2. **On each message,** act on what it says:

    | The user's message                                                  | Action                                                                                  |
    | ------------------------------------------------------------------- | --------------------------------------------------------------------------------------- |
    | Reports an anomaly, at any time                                     | Classify it by [monitor-window.md](monitor-window.md#anomaly-classification).           |
    | Asks for more time, even alongside a no-anomaly report              | Extend the window, as [monitor-window.md](monitor-window.md#extending-the-window) says. |
    | Reports no anomaly, at or after the end time                        | Close the window on the success path below.                                             |
    | Reports no anomaly before the end time and asks to close the window | Close it on the success path; the summary records the shortened window.                 |
    | Reports no anomaly before the end time and does not ask to close    | Say how long remains, and keep the window open.                                         |

    The rows are checked in order and the first match wins.

3. **Success path.** Render the deploy summary below. Then offer the follow-ups as one batch for one confirmation: the
   comms updates that Section 7 drafted, and a close comment on the release tracking issue when one exists. The close
   comment goes through `gh`, follows the [posting envelope](../../../gh/references/posting-envelope.md), and is posted
   only on that confirmation. Suggest `/agentics:gate branch` when a release branch must be merged back.

### Escalation

When an anomaly is classified for escalation, or the deploy failed to start:

1. Close the window, and stop saying "monitoring".
2. Render the hand-off context from [monitor-window.md](monitor-window.md#escalation).
3. Forward to `/agentics:ops response` once, as [forwarding.md](../../../routing/references/forwarding.md) describes.

When the user wants to roll back without the incident workflow, show the staged rollback command, as
[monitor-window.md](monitor-window.md#rollback-shortcut) says. The user runs it.

Gate does not re-monitor after an escalation. It renders the deploy summary and ends.

### Deploy summary

Render the summary once, when the release ends: on the success path, after an escalation, or after a rollback.

```text
DEPLOY SUMMARY — <deployed | rolled back | handed to /agentics:ops response>

Artifact: <ref> (<short SHA>) to <target>
Pre-deploy gate: <GO | NO-GO, proceeded with: <items>>
Change record: <path or URL | none found>; it should now read <Executed | Rolled back>
Monitor window: <N> min<, shortened to <M> min>; <no anomaly reported by the user | anomaly at <UTC>: <one line>>
Rollback: <not needed | command shown; the user reports it <ran | did not run>>
```

Gate does not edit the change record. The summary names the status the record should now carry.

The summary never says the deploy succeeded. What gate can report is that the user saw no anomaly in the window.

## Write the rows

Write the rows through `agx flow log`, as [emission.md](../emission.md) says, and leave them staged. The rows are the
`na`, `skip` and `waive` rows written as the items resolved, and one verdict row when the release ends. The verdict
row's `target` is the artifact and its `status` is the first match:

| Situation                                                                                                       | `status`    |
| --------------------------------------------------------------------------------------------------------------- | ----------- |
| The user cancelled the artifact, or said to stop the release before the deploy                                  | `cancelled` |
| An item failed (whether the user goes to fix it or proceeds), an anomaly was escalated, or the user rolled back | `failed`    |
| The artifact could not be read, or the deploy failed to start                                                   | `blocked`   |
| An item was skipped, or the user closed the window before its end time                                          | `pending`   |
| Every item passed or did not apply, and the window closed with no anomaly reported                              | `ok`        |

A run that ends while the window is still open writes no verdict row, and the report says the release has no verdict
yet. The next `/agentics:gate release` on the same artifact starts a fresh checklist.

## Composition with other skills

- **`/agentics:ops response`** takes over when the monitor window escalates.
- **`/agentics:ops drill`** tests a runbook the checklist relies on, such as a restore procedure, before release mode
  trusts it.
- **`/agentics:review axes`** runs in Section 5 for the security scan of the artifact's diff.
- **`/agentics:gh`** posts the tracking issue's close comment on the success path.
- **`/agentics:gate change`** writes the change record that Section 1 looks for.
- **`/agentics:gate branch`** closes out a release branch after the release.
