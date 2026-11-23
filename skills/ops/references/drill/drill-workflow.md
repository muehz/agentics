# Drill workflow

Six phases, one workflow for both drill types. The `/agentics:ops drill` argument picks the type:

- A runbook path (`/agentics:ops drill runbooks/<name>.md`) starts a **runbook drill**. It exercises one runbook step by
  step. Its deliverable is a delta report that compares each step's actual behavior with the runbook's prescription. The
  report feeds back into the runbook as edits or queues a follow-up audit.
- A scenario name (`/agentics:ops drill <scenario>`) starts a **scenario drill**. It simulates an incident closely
  enough to exercise the response workflow, role assignments, comms cadence, and tooling. The simulation causes no real
  customer impact. Its deliverables are debrief findings and action items for the incident-response process.

Steps tagged _(runbook)_ or _(scenario)_ apply to that drill type only. Untagged steps apply to both.

## Tabletop or game day

| Type         | Description                                                                                                                                                                                                                  | When to use                                                                                                                                                                                                  |
| ------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| **Tabletop** | Verbal walkthrough only. No real systems are touched. Runbook drill: the operator narrates each step as if the operator ran it. Scenario drill: participants narrate what they would do.                                     | Default. It is cheap and fast. Runbook drill: the first drill of a freshly authored runbook. Scenario drill: every 1–3 months. The whole session takes 1–2 hours. Hold the run itself to the time-box below. |
| **Game day** | Real execution in a non-production environment. Runbook drill: run the actual commands. Capture diffs. Scenario drill: inject real failure into a non-production environment. Participants respond against actual telemetry. | When the team has run enough tabletops and is ready for live exercise. Quarterly to twice-yearly (runbook drill: for P1-grade runbooks).                                                                     |

Time-box the drill run. Stop it on time. The debrief has a separate time-box and does not extend the drill run. Use
these time-boxes for the run:

- Tabletop: 30–60 min. Game day: 1–4 hours. The length depends on the procedure. _(runbook)_
- Tabletop: 60–90 minutes. Game day: 2–4 hours. Part of the practice is to stop on time. _(scenario)_

At the run deadline, the facilitator does these steps:

1. Call stand-down.
2. Complete any required safety or cleanup step.
3. Record the current state.

Do not continue execution just to reach a phase boundary.

Game-day mode adds an environment pre-flight in Phase 1 and inject mechanics in Phase 3.

## Phase 1: Select

**Goal:** The drill has a scope, a schedule, and (for game day) a safe environment. _(runbook)_

**Goal:** Pick a scenario that exercises the right parts of the response. Avoid the same scenario twice in a row.
_(scenario)_

1. **Confirm the runbook path** (`/agentics:ops drill runbooks/<name>.md`). If multiple runbooks could apply, ask which
   one as a ❓ question. Add a ➡️ recommendation drawn from the best name or symptom match. _(runbook)_
2. **Pick a scenario** from [drill-scenarios.md](drill-scenarios.md), or accept the user's custom scenario. Confirm the
   scenario in chat. _(scenario)_
3. **Pick the focus.** Each scenario can emphasize different things: _(scenario)_
    - **Detection.** How fast does the alert fire? Is it routed correctly?
    - **Coordination.** Does the IC keep control? Are roles clear?
    - **Containment.** Does the team know the rollback, failover, or revoke path?
    - **Comms.** Is the cadence held? Is the customer-facing message clear?
    - **Hand-off.** Is the postmortem path clean?
4. **Set the drill mode** (tabletop or game day). Confirm.

    > ❓ **Q1** - **Drill mode**: Tabletop (verbal walkthrough) or game day (real execution in a non-production
    > environment)?
    >
    > ➡️ Tabletop, unless this isn't the first drill of this runbook or scenario and the team is ready for live
    > exercise.

5. **Set the time-box** from the table above.
6. **For game day only, pre-flight the environment:**
    - **Use staging or a dedicated drill environment.** A game day in this skill never targets production, because an
      inject there harms real users who did not agree to it. Fault injection in production is chaos engineering, with
      its own approval chain, and this skill has no workflow for it: say so once, and offer a staging game day or a
      tabletop of the same scenario instead.
    - **Notify upstream and downstream teams** before the drill to prevent confusion. Drills can affect shared
      infrastructure.
    - **Have a one-command off-switch.** If the inject misbehaves and real customer impact starts to appear, the
      facilitator can stop the drill and revert.
    - **Brief participants** on what is real and what is simulated. ("This alert is real. The customer impact is fake —
      there are no real customers here.")
    - Confirm the operator has all Prerequisites from the runbook. _(runbook)_

**Exit criteria:**

- Runbook path confirmed _(runbook)_
- Scenario chosen _(scenario)_
- Focus areas identified _(scenario)_
- Drill mode chosen
- Time-box set
- (Game day) environment safe; off-switch confirmed

## Phase 2: Assign

**Goal:** Name the operator and the facilitator. _(runbook)_

**Goal:** Use the same role structure as a real incident: IC, comms lead, scribe, and subject experts. Use drills to
rotate roles. _(scenario)_

1. **Identify the facilitator** (both drill types). The facilitator is separate from the participants and runs the
   clock.
    - Runbook drill: the facilitator takes notes and intervenes only if the operator strays from the runbook. The
      facilitator is often the runbook's owner or a senior on-call.
    - Scenario drill: the facilitator drives injects and observes. The facilitator does not intervene unless someone
      asks the wrong question of the wrong person. This person is often an engineering manager or senior IC who does not
      play themselves.
2. **Identify the operator.** The operator is the person who will execute or narrate. For a first drill, the operator
   should be someone who **did not author the runbook**. The author fills in implicit knowledge and does not notice it.
   An outsider finds the gap. _(runbook)_
3. **Pick the IC.** The IC is often someone who has not been IC recently. Drills are training, so rotate the IC role.
   Let many people act as IC. _(scenario)_
4. **Assign comms lead, scribe, subject experts.** For security scenarios, also assign a security lead and a forensics
   scribe. _(scenario)_
5. **Identify observers** (optional): people who watch but do not participate, for cross-team learning. Tell them to
   stay silent until the debrief. _(scenario)_

**Exit criteria:**

- Facilitator named
- Operator named _(runbook)_
- All roles assigned _(scenario)_
- Participants and observers introduced _(scenario)_

## Phase 3: Run

**Goal:** Execute (or narrate) every step of the runbook in order. Record what the operator actually did and what
actually happened. _(runbook)_

**Goal:** Execute the scenario. Participants follow the response workflow as if real. _(scenario)_

### Tabletop run

_(runbook)_ For each step in the runbook:

1. **Operator reads the step aloud.**
2. **Operator narrates what they would do:** which command, where to run it, and what placeholders to substitute.
3. **Operator narrates how they would verify.** Does the verify clause make sense to them? Could they execute it?
4. **Facilitator captures any of these:**
    - **Ambiguity:** the operator paused and asked a question. The runbook has a gap.
    - **Implicit knowledge:** the operator filled in a step from memory that the runbook did not include. The runbook
      has a gap.
    - **Wrong narration:** the operator described the step incorrectly. Either the step is ambiguous, or the operator
      does not know enough. Both are findings.
    - **Skipped verify:** the operator narrated past a verify clause. The verify clause may be unactionable.

_(scenario)_ The facilitator narrates the scenario inject. The IC declares. The team walks through what they would do,
out loud. The scribe takes notes as if it is real. The team writes comms drafts but does not send them (or sends them to
a private drill channel).

Cadence, in elapsed drill time. The skill has no clock: when it facilitates in chat, it delivers the next inject on the
participant's next message, at the elapsed time the participants report, and says which inject comes next and when.

1. **T+0:** The facilitator delivers the initial inject. ("PagerDuty fires at 14:32 UTC: payment-api 5xx rate at 8% over
   the last 5 minutes.")
2. **T+5–10 min:** The IC should declare. If the IC does not, the facilitator nudges with another signal ("at T+8,
   customer support reports 12 tickets about checkout").
3. **T+15–30 min:** Containment phase. The facilitator delivers complications as they would happen. Examples are a
   dependency outage, an unrelated alert that is actually related, or a comms-channel issue.
4. **T+30–60 min:** Resolution. The facilitator lets the team find the cause through verbal walkthrough. Alternatively,
   the facilitator cuts it short with "you found the cause: <reveal>" once enough investigation has happened.
5. **T+60–90 min:** Hand-off and stand-down. It is the same as in a real incident.

### Game-day run

_(runbook)_ For each step:

1. **Operator executes the actual command** in the drill environment.
2. **Operator records the actual output** (paste or screenshot).
3. **Operator runs the verify clause** as written.
4. **Facilitator captures:**
    - **Command failures:** the exit code is non-zero, or the output does not match the runbook's expectation.
    - **Verify ambiguity:** the verify clause passes when it should not, or fails when it should pass.
    - **Timing drift:** the step took much longer or much shorter than the runbook implies.
    - **Side effects:** the command had effects the runbook did not anticipate.
5. **If a step fails:** follow the runbook's "if verify fails" path exactly. Do not skip it. The recovery path is part
   of the drill.

_(scenario)_ The structure is the same, but injects are real failures in a non-production environment:

- A real alert fires against staging.
- A real dashboard shows real impact that test traffic drives.
- Real rollback, failover, or revoke commands run.
- Real comms are drafted and sent to a drill channel, never to customers.

### Game-day safety rules

- **Do not fix bugs mid-drill.** If a step is broken, do these steps:
    1. Capture the delta.
    2. Follow the runbook's prescribed recovery.

    If the runbook prescribes no recovery, that is itself a finding. Repairs come after the drill.

- **Do not deviate from the runbook.** The operator may know a better way. The drill still tests the runbook, not the
  operator. Captured deviations become edits in Phase 6.
- **Stop on irreversible step failures.** If an irreversible step fails in the drill environment, do these steps:
    1. Halt.
    2. Treat the failure as real.
    3. Exercise the escalation path.

**Exit criteria:**

- Every step exercised (tabletop) or executed (game day) _(runbook)_
- Delta log captured per step _(runbook)_
- Verify clauses tested _(runbook)_
- Time-box reached or scenario completed _(scenario)_
- Scribe's notes captured the timeline _(scenario)_
- Drafts of all comms preserved (not sent to real audiences) _(scenario)_

## Phase 4: Capture

**Goal:** Convert the running notes into a structured delta report. Use one row per step, with prescription and
observation. _(runbook)_

**Goal:** Collect the observers' notes before memory fades. _(scenario)_

### Delta report _(runbook)_

Render the delta report:

```markdown
# Drill: <runbook path>

**Drilled on:** YYYY-MM-DD **Drill mode:** <tabletop | game day> **Operator:** <handle> **Facilitator:** <handle>
**Duration:** <minutes>

## Per-step deltas

| Step | Prescription       | Observation                  | Delta           | Severity                              |
| ---- | ------------------ | ---------------------------- | --------------- | ------------------------------------- |
| 1    | <runbook's action> | <what operator actually did> | <gap or "none"> | <CRITICAL / HIGH / MEDIUM / LOW / OK> |
| 2    | ...                | ...                          | ...             | ...                                   |

## Verify clause testing

| Step | Verify clause as written | Test result                 | Notes     |
| ---- | ------------------------ | --------------------------- | --------- |
| 1    | <verify text>            | <pass / ambiguous / failed> | <details> |

## Rollback path testing

<If a step failed during game day and rollback was exercised:>

| Rollback step | Prescription | Observation | Delta |
| ------------- | ------------ | ----------- | ----- |
| ...           | ...          | ...         | ...   |

## Findings summary

### CRITICAL

- <findings that would have caused harm in a real incident>

### HIGH

- <findings that would have caused stranding or wrong-direction action>

### MEDIUM

- <ambiguity that slowed the operator down>

### LOW

- <cosmetic, missing cross-links, formatting>
```

**Severity calibration:**

- **CRITICAL:** a step that, as written, would cause harm in production. Examples are a wrong command, a missing
  irreversibility warning, or a rollback that would have failed.
- **HIGH:** a step that would have stranded the operator. Verify failed and there was no escalation. Command failed and
  the runbook documented no rollback.
- **MEDIUM:** a step that slowed the operator down with ambiguity but did not fail. The verify clause was vague. A
  placeholder was unclear.
- **LOW:** formatting, dead links, missing cross-links to adjacent runbooks.

### Observers' contribution _(scenario)_

Observers saw things participants could not see. Common observer findings:

- "you all missed alert X firing in the corner of the dashboard,"
- "the IC was debugging by minute 30,"
- "comms cadence dropped during minute 20–40."

Collect the observers' notes on detection, coordination, and containment now. Observers read them out last in the
debrief.

**Exit criteria:**

- Delta report rendered, with every finding given a severity _(runbook)_
- Observers' notes collected _(scenario)_

## Phase 5: Debrief

**Goal:** The drill's value is the debrief. A skipped debrief loses 80% of the learning. Always debrief. Do not extend
the drill run to make room for it.

Reserve a separate 30–60-minute debrief block. When that block is scheduled, start it immediately after the drill. If
the run reaches its deadline or the debrief cannot fit, do these steps:

1. Preserve the timeline, observations, and current state.
2. Schedule the debrief as the earliest follow-up, ideally the same day and before another drill starts.

### Debrief structure (30–60 min)

1. **Reset on blameless framing.** The rule is the same as in postmortem mode. Drills are practice, and mistakes during
   practice are exactly the point.
2. **Walk the timeline.** The scribe reads. Participants correct it and fill gaps. In a runbook drill, walk the delta
   report step by step instead.
3. **What worked?** Name only real positives. Be specific. ("The IC clearly named who was doing what at the start." "The
   runbook for redis failover was where it needed to be.")
4. **What didn't?** Name specific gaps. ("We didn't have a way to find the on-call DBA quickly." "The status-page draft
   took 15 minutes — too slow.")
5. **What surprised us?** This is the most valuable category: things participants did not predict. These often point to
   the gaps that matter most.
6. **Action items.** They have the same shape as postmortem action items: specific change, named owner, due date.

Observers speak last, after participants (see Phase 4).

### Update `Last tested` _(runbook)_

The field exists for trust. Read the rows from the top; the first match wins.

| Order | The drill                                                                             | `Last tested:`                                                         |
| ----- | ------------------------------------------------------------------------------------- | ---------------------------------------------------------------------- |
| 1     | Could not complete (the environment broke, or the operator could not get past a step) | Unchanged                                                              |
| 2     | Completed with a CRITICAL finding: a step that, as written, would cause harm          | Unchanged until the fix lands and a drill of the fixed steps completes |
| 3     | Otherwise: completed, with or without lesser findings                                 | The drill date; the findings are tracked separately as edits           |

### Update `Last incident this was used in` _(runbook)_

This update is optional but valuable. Append a row:

```markdown
| YYYY-MM-DD | Drill ([drill report](url-if-published)) | <succeeded / partial / failed> | <yes / no; see drill
findings> |
```

**Exit criteria:**

- Debrief notes captured
- Action items have owners and due dates
- Surprises explicitly named
- `Last tested:` set by the table above _(runbook)_

## Phase 6: Act

**Goal:** Action items ship. A drill program that produces 30 untracked action items per year is worse than no drill
program. The team learns that the program is only for show.

**Goal:** Decide what to do with the deltas. Present the three paths as one ❓ question. Base the ➡️ recommendation on
the findings (re-author when > 5 HIGH or any CRITICAL; otherwise patch). _(runbook)_

### Action items

1. **Optionally draft a debrief issue.** Use the template in [../github-artifacts.md](../github-artifacts.md). The issue
   posts to the engineering channel and tracks action items. _(scenario)_ A runbook drill's tracked artifact is the
   delta report and the runbook edits (or a queued `/agentics:ops audit`), not a debrief issue. _(runbook)_
2. **Draft the action-item issues.** Each gets a GitHub issue with an owner and a due date. Preview them together and
   post them on one confirmation, by [Writes outside the machine](../../SKILL.md#writes-outside-the-machine).
3. **Address runbook gaps.** Runbook gaps are the most common drill output: _(scenario)_
    - **Missing runbook** → suggest `/agentics:ops author <gap-name>`. The ops skill seeds a new procedure from the
      drill's findings.
    - **Stale runbook** that the drill exposed → suggest `/agentics:ops audit <path>` to flag and patch.
    - **Untested runbook** referenced during the drill → schedule `/agentics:ops drill <path>` next cycle. Do not
      auto-invoke. Show the suggestion. Let the user decide.
4. **Log the drill** somewhere durable, such as a `drills/` directory in the repo or a tracker. Future drills learn from
   this history.

### Decide what to do with the deltas _(runbook)_

1. **Patch the runbook.** This is the most common path. The deltas become drafted edits. Render them in chat with the
   standard draft-and-confirm rule. Commit on approval. Use `/agentics:git commit`. State that the commit applies
   drill-driven updates to the runbook `<name>`. `git` words the commit message.
2. **Re-author.** If the deltas are too numerous (> 5 HIGH or any CRITICAL), the runbook needs more than a patch. Invoke
   `/agentics:ops author <name>` to rewrite it. Use the drill report as input.
3. **Retire.** If the drill reveals the runbook is no longer applicable, move the runbook to `runbooks/archive/`. Add an
   archive note that links the drill report. Examples: the system has changed, the procedure is now fully automated, or
   the scope has drifted.

If findings are concentrated in one area and need a wider look than a patch, queue `/agentics:ops audit <path>`. Do not
leave the deltas open.

### Schedule the next drill

- Quarterly minimum. If this drill's action items are large, the next drill exercises whatever was changed. _(scenario)_

The next drill date depends on the runbook's severity: _(runbook)_

- P1-grade: next drill in 90 days
- P2-grade: next drill in 180 days
- P3+: next drill in 365 days

The next drill can come sooner if findings warrant a re-drill after edits.

**Exit criteria:**

- Action items have GitHub issues
- Next drill date named
- Runbook gaps identified and assigned _(scenario)_
- Path chosen (patch / re-author / retire) _(runbook)_
- Edits drafted, confirmed, committed _(runbook)_

## Common pitfalls

- **Skipping verify-clause testing.** The drill tests the verify clauses on purpose. Do not skip past them.
- **Mid-drill fixing.** The drill tests the runbook as-is. Capture the delta. Fix it after the drill.
- **Same scenario every time.** The team gets good at one scenario and stays unprepared for the next failure mode.
  Rotate the scenarios.
- **No observer.** Participants cannot see their own blind spots.
- **Drill on production by accident.** Game-day environments must be unambiguous and isolated.
- **No follow-up.** A drill with 15 findings and no edits is paperwork. Either patch, re-author, or retire.

## Composition with other skills

- **`/agentics:ops audit <path>`**, the natural next step if drill findings are concentrated in one area, such as
  command drift or ownership.
- **`/agentics:ops author <name>`**, when re-authoring is the right path.
- **The other drill type.** Phase 6 routes missing, stale, or untested runbooks that a scenario drill surfaces to the
  corresponding `/agentics:ops` mode.
- **`/agentics:git commit`** and **`/agentics:gate branch`**, to land drill-driven edits.
- **`review`'s Security lane.** Security drill scenarios may invoke `/agentics:review axes <ref> --security --composed`
  on a fake-vulnerable code path. Composed does not post to a real PR automatically during the drill. It is useful to
  exercise the envelope handling, not the review itself.
- **`triage`**, for drills that simulate an inflow of suspicious PRs or check failures. Those drills exercise the triage
  skill alongside incident response.
