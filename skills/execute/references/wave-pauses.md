# Wave pauses

This file holds the rules that the `Wave pauses` section of [`SKILL.md`](../SKILL.md#wave-pauses) points to.

A **wave pause** is the stop at the end of each planned wave in `pair` and in `auto pause`. It comes after the wave's
integration verification and after every group review the wave completed has cleared or recorded its skip. In `pair`, a
planned wave is never split into one-task waves, so the pause comes once per planned wave, never after each task. At a
wave pause, do these steps:

1. Show what shipped.
2. Show the verification output and the review verdicts of the groups this wave completed. Name each group whose review
   is still pending.
3. Append any **Plan divergence** entry, and show it.
4. Say "Ready for feedback."
5. **Wait for the response before you continue.**

## Applying the feedback

Read the feedback by the first row that matches:

| Order | Feedback                                                                 | Do                                                                                                                                                                                                                   |
| ----- | ------------------------------------------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1     | It changes what a task builds: its scope, its criteria, or the task list | Follow **When the user changes the work mid-run** in `SKILL.md`: the revision goes to `decompose`, and the revised plan needs a new approval before the run resumes at Step 0.                                       |
| 2     | It asks for a fix to work that shipped                                   | Send the fix to the implementer of the task it names (in `pair`, the session makes it). Commit it, then continue the group's quality reviewer with the fix commits. The reviewer's verdict writes the `verify` rows. |
| 3     | It adds a constraint or an exception to a repository rule                | Append the standing order or the user-authorized divergence, with its row.                                                                                                                                           |
| 4     | It approves the wave, or asks for nothing                                | Continue.                                                                                                                                                                                                            |
| 5     | Otherwise                                                                | Ask one question.                                                                                                                                                                                                    |

A fix made from feedback is reviewed like any other fix. It never lands after the group's verdict with no review and no
`verify` row.

The last wave pauses like every other wave. Make the completion handoff only after that response, so the user keeps the
last review moment.

The wave pause is the user's review, not the quality review. A user who reviews the wave never stands in for the group's
reviewer.

## The waiver

The wave pause is a process gate ([gates.md](../../routing/references/contracts/gates.md#process-gate)). On "run all
waves, don't stop", name the cost once: feedback then arrives only after everything has shipped, so rework found late
spans every wave. On the next reply that still declines, run the remaining waves with no stop, post each boundary's
report as you pass it, and write one `waive wave-pauses` row with the user's reason or `none given`. The run report
shows `Wave pauses waived by user`, taken from that row. A resumed run reads the row and does not pause.
