# Response workflow

Move to the next phase when the current one has met its exit criteria. Going back is normal: a severity is re-declared,
the incident commander (IC) changes, a mitigation fails and containment starts again. Log each one in the incident
record.

**The incident record** is the tracking issue once it is posted. Until then, or when the user declines to post one, it
is the timeline kept in the session, or in a private location the user names. Every later step writes to whichever
exists, and the postmortem reads it.

Posts follow [Writes outside the machine](../../SKILL.md#writes-outside-the-machine): drafted first, sent on a clear
affirmative or because the user named them up front, after the outbound checks. Comms to channels, status pages and
customers are drafts the user sends.

## Phase 1: Declare

**Goal:** Containment starts at once, and severity, branch, IC and the incident record are settled.

1. **Capture the trigger.** What fired, what users reported, what the user observed. One paragraph.
2. **Give the first reply in this order.** It never waits on a question before it gives containment guidance.
    1. **Severity, stated as a default.** Name the row of the severity table in
       [../shared-principles.md](../shared-principles.md#severity-p1-to-p4) that matches, and the IC (the user, unless
       they name someone). The user corrects it in their own words; no question is asked.
    2. **Immediate containment.** When nothing is impacted, this is one line saying there is no live impact to contain,
       recorded as `containment: not applicable`. Otherwise, give concrete reversible steps matching the trigger: for a
       deploy that correlates with onset, roll it back; for a flag that gates the new path, turn it off; for a regional
       latency or capacity degradation with normal errors and no security signal, give targeted reliability containment
       (check recent deploy/config changes, feature flag, regional scale/cache/traffic shift). When a security signal is
       stated or evidenced (or the user answers yes/unsure to Q1), the first step is the 5 to 15 minute snapshot that
       [security-branch.md](security-branch.md#evidence-preservation-run-before-containment) describes, and then the
       containment step.
    3. **The branch question,** unless the request explicitly settles the branch (e.g., "not security", "it's just an
       outage", or an explicit breach/leaked-key signal). Never skip Q1 on a bare outage or pure symptom telemetry, and
       never declare `reliability branch` in the same reply as asking Q1:

        > ❓ **Q1** - **Branch**: Is there any indication this involves unauthorized access, credential compromise,
        > malicious activity, or unexpected data exposure?
        >
        > ➡️ Unsure counts as yes: run the security branch, because a downgrade later costs little and a broadcast
        > during a breach cannot be taken back.

        When Q1 is asked, label the branch `Branch: pending Q1` until answered. Yes or unsure loads
        [security-branch.md](security-branch.md); no loads [reliability-branch.md](reliability-branch.md); both runs
        security comms and forensics with reliability customer recovery. The user's statement decides, including "it's
        just an outage". If Q1 was asked and remains unanswered on a later turn, treat the branch as unsure and run the
        security branch. When evidence in the session later points to security, say once what was observed and ask the
        question again.

    4. **The incident record.** Read the repository's visibility first. Every active-response first reply (P1–P4)
       includes the fenced incident-record draft verbatim rather than describing record handling only in prose. When the
       branch is security, unsure, or `pending Q1`, follow
       [security-branch.md](security-branch.md#where-the-record-lives) (render a fenced draft security advisory or
       private incident record when the repository is public, internal, or unreadable—never a public tracking issue).
       Otherwise render the fenced tracking issue draft from
       [../github-artifacts.md](../github-artifacts.md#tracking-issue-response-mode-declaration). Every draft named in a
       reply (incident record, first internal update, customer status-page draft) appears in a fenced block in that same
       reply, never promised for later. Post it on the user's yes, or at once when they named it up front.
    5. **Defaults and initial comms (every first reply, P1–P4):** in one line each, name the responder channel, wider
       engineering channel, and the cadence offer ("Say 'post the cadence updates' and each one goes on the tracking
       issue as it falls due, shown here."), using `<angle-bracket>` placeholders when channel names are not supplied.
       Include the fenced first internal update draft (labeled `Draft (not posted)`) in every active response first
       reply, and for settled non-security P1/P2 active outages also include the customer status-page draft from
       [../communication-templates.md](../communication-templates.md).

3. **Take the reply.** A partial reply settles what it names, and the rest keeps its default. When the user declares a
   severity other than the table's row, declare theirs, say the table row once, and record both in the incident record
   ("declared P3 at user direction; table indicates P2").

**Exit criteria:**

- Containment guidance given
- Severity and IC declared
- Branch chosen
- The incident record exists: the tracking issue, or the record in the session or a private location
- Responder channel named

## Phase 2: Assemble

**Goal:** The right people are present and the roles are explicit.

1. **Page or notify by severity.** P1 pages the on-call IC and the relevant subject experts. P2 pages the IC and
   notifies subject experts without paging. P3 is handled without paging. The user pages; the skill names whom.
2. **Assign roles.** IC, comms lead, scribe, subject experts; on the security branch also a security lead and a
   forensics scribe. Print the role table once in the incident record. A role nobody holds stays with the IC; one
   responder holding every role is valid.
3. **First internal update.** Draft the "we're paged, stand by" template from
   [../communication-templates.md](../communication-templates.md) for the user to send.
4. **Customer comms.** Customer-visible impact at P1 or P2 gets a status-page draft within 30 minutes of declaration.

**Exit criteria:**

- Roles in the incident record
- First internal update drafted
- Customer comms drafted, or deferred by the user

## Phase 3: Contain

**Goal:** Stop the ongoing damage, reduce the blast radius, and gain time to resolve.

The mechanics differ by branch: the containment tables in [reliability-branch.md](reliability-branch.md) and
[security-branch.md](security-branch.md). Across both branches:

1. **One change at a time.** Simultaneous mitigations hide which one worked. Log each with a timestamp and its reason.
2. **Reversible first.** A step that cannot be undone (a data delete, a forced rotation of everything) is a destructive
   step under [approval.md](../../../routing/references/contracts/approval.md#the-two-exceptions): show its resolved
   target and what will be lost, and act only on a reply that names the action or its target.
3. **The scribe logs each attempt:** time, action, observed effect, who decided.
4. **Cadence.** An update is due every 30 minutes at P1, every 60 at P2, and on a status change at P3, even when the
   news is "still investigating". The skill has no timer: at each user message it drafts any update that is due and says
   when the next one falls due. Under the cadence named up front, an update that changes nothing (same severity, scope
   and audience) posts as it falls due and is shown in chat. An update that changes severity, scope or audience is shown
   first.
5. **Reassess severity** after each attempt: worse, better, or no change. Worse is re-declared upward.

**Exit criteria:**

- The damage is stopped or bounded, or no untried containment option remains
- The hand-off decision in Phase 4 is made

## Phase 4: Resolve

**Goal:** Find the cause, ship a real fix, and confirm recovery that users can see.

### When root-cause work starts

This skill decides when `diagnose` takes over the root cause. Read the rows from the top; the first match wins.

| Order | State                                                                                                                           | What to do                                                                                                                                                                                                                                                                                                                                                                            |
| ----- | ------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1     | The user asks to start root-cause work now                                                                                      | Hand off now. Record the containment state as it stands.                                                                                                                                                                                                                                                                                                                              |
| 2     | Containment holds: the damage is stopped or bounded                                                                             | Record `containment: holding` and hand off to `diagnose` in the same reply, naming the branch (`unsure: security branch, Q1 pending` if unanswered).                                                                                                                                                                                                                                  |
| 3     | No untried containment option remains: every option in the selected branch's containment table has no target here or has failed | Load and inspect **both** `reliability-branch.md` and `security-branch.md` containment tables (when Q1 is unanswered), record `containment: unavailable`, state the branch (`unsure: security branch, Q1 pending` if unanswered), enumerate every checked/unavailable containment option inside the brief (including stop-write and read-only steps), and hand off now; keep cadence. |
| 4     | Otherwise: an untried option from that table exists                                                                             | Try it next, under the Phase 3 rules, and read this table again.                                                                                                                                                                                                                                                                                                                      |

For row 3, an option has no target here when the user's report rules it out or the incident gives it nothing to act on.
Restart, traffic shed and scale-out have a target only when the report shows a stuck process or a capacity signal. Do
not ask about each remaining option before handing off. List the untried options in the brief. An unanswered branch
question does not hold the hand-off: check both branches' containment tables, record `containment: unavailable`, and
hand off in the same reply.

The hand-off invokes `/agentics:diagnose` with a brief: the incident reference, the symptoms, the timeline so far, what
was tried, and the containment line. The containment tables are in
[reliability-branch.md](reliability-branch.md#containment-options-preferred-order) and
[security-branch.md](security-branch.md#containment-options); the data-integrity steps in reliability-branch.md (stop
the write path, switch to read-only) count as options too. `diagnose` takes that line as settled and does not ask about
containment again. The IC keeps coordinating while the root-cause work runs.

### The fix

1. **The fix follows `diagnose`.** `diagnose` carries a fix it has proven forward by its own rules. This skill does not
   choose the fix's path.
2. **Check before it ships.** The remediation branch goes through `/agentics:gate branch` before it merges. When the IC
   ships a fix ahead of that check to stop the damage, record the shipped commit in the incident record and run
   `/agentics:gate branch` on the branch afterwards.
3. **Roll forward carefully.** When containment was a rollback, the fix is the roll forward: stage it, canary it where
   possible, and watch the metrics.
4. **Confirm recovery.** Watch the dashboards, logs or metrics that showed the impact until they hold at baseline for a
   sustained window (15 to 30 minutes for P1).
5. **Mark resolved.** Move from `monitoring` to `resolved` after that window. Marking it resolved sooner is the user's
   call, a [process gate](../../../routing/references/contracts/gates.md#process-gate): name the cost once (a relapse
   inside the window reopens the incident in public), then proceed on the next reply that still asks, and record it in
   the incident record. Never state a recovery the data does not show.

**Exit criteria:**

- Root-cause hand-off recorded with its containment line
- Fix shipped, and its branch through `/agentics:gate branch` or recorded as checked afterwards
- Recovery confirmed, or the early resolve recorded
- `resolved` drafted or posted, with the resolution time in the incident record

## Phase 5: Hand off

**Goal:** Move from active response to the postmortem with nothing left open.

1. **Check the incident record:** timeline, comms log, severity changes, containment attempts.
2. **Name the postmortem owner,** usually the IC.
3. **Set the postmortem date:** typically within 5 business days for P1 and 10 for P2. Say that
   `/agentics:ops postmortem <incident>` runs it; the skill does not schedule anything itself.
4. **Close the tracking issue** with a final comment: severity, duration, one-line root cause, postmortem owner and
   date. Draft the comment and the close, and post them under
   [Writes outside the machine](../../SKILL.md#writes-outside-the-machine): on a clear affirmative to the draft, or at
   once when the user named the close-out up front.
5. **Stand down.** Draft the stand-down message for the responder channel. Release the roles. Any standing posting
   authorization for this incident ends here.

**Exit criteria:**

- Tracking issue closed with a summary, or the record handed to the user
- Postmortem owner and date named
- Stand-down drafted
