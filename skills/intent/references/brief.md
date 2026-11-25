# The middle path: `brief.md`

A change takes the middle path when the small-fix classification returns `brief`: it fails only the size signal, fires
no elevated-tier signal, and fits one task. The conditions are in
[Middle path](../../routing/references/chain-phases.md#middle-path). On this path `intent` writes `brief.md` in place of
`intent.md`. The brief gets one independent review and one approval question, and the approved brief goes to `execute`.
No spec and no plan are written.

Track these [workflow steps](../../routing/references/contracts/workflow-steps.md): gather, write, review, ask, hand
off.

## Gather

Run [Step 1](../SKILL.md#step-1-gather-the-direction) of the skill, including its flow binding. The middle path runs no
direction interview. Ask the user only a question the brief cannot be written without, one decision per question, in the
[question format](../../routing/references/contracts/question-format.md). Settle an observable question by the empirical
fork in [Step 2](../SKILL.md#step-2-challenge-the-intent) instead of asking it. A question with no answer stays in the
brief's **Open questions** list, and no approval question is asked while it is open. An open question is a
[process gate](../../routing/references/contracts/gates.md#process-gate): when the user asks to go on anyway, name the
cost once (the implementer would have to invent the answer). On the next reply that still asks to go on, record the
question under **Decisions** as `waived by the user`, left to the implementer under `execute`, run
`agx flow log intent waive brief-questions ok "<the user's reason, or none given>" none "intent ask for approval" --folder <work-folder>`,
and continue.

## Write

Write `<work-folder>/brief.md` in this shape. Each section is short. A section with nothing to say is left out, except
the four the hand-off needs: **Problem**, **Behavior**, **Acceptance criteria**, and **Task**.

    # Brief: <topic>

    - **Source:** <one sentence, by the Source rule in skim-rules.md>

    ## Problem

    ## Behavior

    ## Acceptance criteria

    ## Task

    - **Files:** <the exact paths the task changes>
    - **Acceptance criterion:** <the one verifiable criterion>
    - **Verification:** <one command and the result it must show>
    - **Constraints:** <how the task must be done, when a forward or the user set a requirement; left out when none>

    ## Decisions

    ## Open questions

- **Problem** says what is wrong or missing and who it affects.
- **Behavior** says what the software does after the change, in observable terms.
- **Acceptance criteria** lists what a reviewer checks. Each item is observable.
- **Task** is the single task `execute` runs: one file scope, one verifiable acceptance criterion, and one concrete
  executable shell or test command (e.g., `go test ...` or `npm test ...` — never a prose description, bracketed
  placeholder, or "to confirm" stub) that **fails before the change and passes after it** (never a pre-existing passing
  suite that passes unchanged). In a repository with no test suite, the command is the closest executable check meeting
  the same fail-before/pass-after rule. A requirement on how the task is done goes on the **Constraints** line, because
  `execute` builds its packet from this section: a forwarded requirement such as `tdd`'s test-first, or the failing
  reproduction of a diagnosed fix as the task's red test, named by its command or test. When the findings carry a
  reproduction, it is the **Verification** command too, and the Problem cites the root cause and evidence the forward
  gave rather than deriving them again.
- **Decisions** records only what the user actually settled and where, and the brief review line below. Never invent,
  assume, or silently fill in an unspecified parameter or default as a user decision; if the brief cannot be written
  without it, keep it in **Open questions** or verify it empirically first.

The [Source rule](../../decompose/references/skim-rules.md) decides what the `Source` line names. The
[Code in artifacts rule](../../decompose/references/chain-artifacts.md#code-in-artifacts) limits the code the brief may
hold.

Stage the brief and record its first write the way [Step 3](../SKILL.md#step-3-write-the-intent) records an intent, with
`brief.md` as the target:

    agx flow log intent create brief.md ok "<one line: why the change exists>" brief.md "intent review the brief" --folder <work-folder>

A [user's change](../../execute/references/fix-entry.md#a-users-change) follows the user's-change rules of
[Step 1](../SKILL.md#step-1-gather-the-direction) (the runtime-change yes) and
[Step 3](../SKILL.md#step-3-write-the-intent) (the **Decisions** record, and the waiver row after `create brief.md`,
keyed on the work item's branch).

Stage `state.tsv` with its own `git add`. Before this skill makes the item's first commit, follow
[Feature branch before the first commit](../../routing/references/chain-phases.md#feature-branch-before-the-first-commit).

**The change outgrows the path.** When writing or the review shows an elevated-tier signal, or more than one task, stop
the middle path and say which condition failed. The work enters the full chain at
[Step 2](../SKILL.md#step-2-challenge-the-intent) of the skill, with the brief's content as input to the intent. Its
**Constraints** line moves under the intent's **Constraints**, with the same source. The brief stays unapproved. The
change is never finished on the middle path.

## Review

Every brief gets one independent review before its approval question. Dispatch one fresh general-purpose subagent with
[brief-reviewer-charter.md](brief-reviewer-charter.md) and the brief's absolute path, complete the review pass (showing
the reviewer dispatch, verdict `READY`/`REVISE`, findings tally, recorded `- **Brief review:**` line, and — whenever a
review finding triggers a revision such as adding a fail-before/pass-after verification test — rendering the **complete
revised `brief.md` verbatim** with the corrected verification command), and then present the concrete brief path,
`flow status` hash, direction summary, and single approval question naming `execute` — never stop with "independent
review not completed" or omit the brief path/hash/approval question when the review clears. The reviewer runs at the
Deep tier, and at Max only when the user selects Max for the review. **Reviewer selection** in the
[model ladder](../../routing/references/model-ladders.md#reviewer-selection) resolves the tier, and
[surface-dispatch.md](../../routing/references/surface-dispatch.md) gives the host's dispatch mechanism.

Resolve each finding in the brief, or keep it open; the budget below says which open findings may go with the approval
question. Record the review under **Decisions** as
`- **Brief review:** <YYYY-MM-DD>, <N> findings, <R> resolved, <O> presented, run <k>`, and each presented finding as
`- Brief review finding (presented, <severity>): <finding> (<location>)`, with the reviewer's severity. Re-stage the
brief after the last edit.

**A revision after the review.** An edit that changes the behavior, a criterion, or the task after a review, an edit
request at the approval question included, makes the review stale. Rewrite its line as
`- **Brief review:** stale, <YYYY-MM-DD>, revised after run <k>` and review the revision under its new budget before the
approval question. A wording fix keeps the line.

**Budget.** Each revision of the brief has its own budget of two runs. Run 2 is allowed only when the fixes for run 1's
findings changed the behavior, a criterion, or the task. A wording fix never causes run 2. A finding left after the last
run goes to the user with the approval question only when it is Minor or Informational. A Critical or Important finding
left after the last run blocks the question: present it and ask the user to revise the brief or stop. A revision the
user asks for, such as an edit request at the approval question, starts a new budget.

**No subagent.** Only when the host environment genuinely cannot dispatch a subagent (never in response to a user asking
to skip the review), say so once, read the brief against the charter yourself in one pass, record
`- **Brief review:** <YYYY-MM-DD>, self-read: no subagent available, <N> findings, run <k>`, and say in the approval
question that the review was not independent. `execute` reads that line and reviews the task's packet itself.

**The review is a safety gate**, one of the reviews before an approval question in
[gates.md](../../routing/references/contracts/gates.md#safety-gate). A request to skip it is held, said once: the review
is held, because an approval of an unreviewed brief sends unchecked scope to implementation; the user can instead waive
the chain entry for this change, as [the waiver](../SKILL.md#when-the-chain-is-required) says. Then immediately dispatch
the independent review subagent with [brief-reviewer-charter.md](brief-reviewer-charter.md) at the Deep tier (do not
fall back to a self-read when dispatch is available, and do not ask the approval question before the review completes).
A repeated demand gets a one-line pointer back to that alternative.

## Ask

Run [Step 4](../SKILL.md#step-4-route-to-spec-and-ask-for-approval)'s approval procedure with `brief.md` in place of
`intent.md`: no question while an open question remains, the status line, the noted `hashes: brief.md text=<hash>`, the
answer table, the approval guard, and asking again by content. Before asking, show the brief's full resolved path, its
actual hash (never an ellipsis or placeholder), a concrete summary of its behavior, task, and decisions, and the review
line. The question is:

> "Brief written to `<brief-path>`. Approve this change? Your yes lets `execute` make this one change; commits, the pull
> request, and the merge keep their own gates."

The rows are:

    agx flow log intent approve brief.md ok "<what the approval rested on>" auto "execute run the brief" --folder <work-folder> --expect-blob <hash>
    agx flow log intent approve brief.md declined "<one line: why the user declined>" none "intent revisit the brief" --folder <work-folder>

An unattended run writes and reviews the brief, asks nothing, and reports that approval is pending.

**Re-entry.** A later session that finds `brief.md` with no current approval resumes at the first row that holds:

| The brief                                                                                 | Resume at                                                                             |
| ----------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------- |
| An **Open questions** item has no answer and no `waived by the user` record in Decisions  | [Gather](#gather): ask it, or take the process gate there when the user asks to go on |
| It has no `Brief review` line, or the line reads `stale`                                  | [Review](#review)                                                                     |
| A `Brief review finding (presented, …)` line is Critical or Important, or has no severity | The budget's block: present the finding and ask the user to revise the brief or stop  |
| Otherwise                                                                                 | [Ask](#ask): the approval question and nothing else                                   |

## Hand off

The approved brief is the consent to run its one task, as an approved plan is for its tasks. Announce `execute` with the
brief's path and run it. A capture-only, another-session, or paused request saves and reports the route instead. `gate`
follows `execute`, as for any other path.
