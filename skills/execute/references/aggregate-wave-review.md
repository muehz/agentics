# Aggregate wave review

This file holds the rules that the `Aggregate wave review` section of [`SKILL.md`](../SKILL.md#aggregate-wave-review)
points to.

This review runs at every review-group boundary in every mode, after the group's quality review clears. In `auto` it is
the substitute for a user stop. A **review group** is the set of tasks that one quality reviewer judges together: a
slice in a sliced plan, a wave in an unsliced plan (named `wave-<N>`). A task may override either default with
`**Review group:** <name>`, and tasks that share a name form one group. The review waits until you have run the
integration verification of the wave that completes the group yourself, plus the Outcome proof on the skeleton wave. It
also runs after a **Two-strike stop** leaves one or more tasks blocked while every other task in the group passed. A
stopped task stays an open blocker. The review covers the siblings that passed. The stopped task's dependents stay held.
The run never reaches **Definition of done** around it. A group whose quality review the user waived reaches this review
at its `waive "quality-review <group>"` row; the reviewer gets that row in place of the quality verdicts. The prompt is
[aggregate-reviewer-prompt.md](aggregate-reviewer-prompt.md). The tier follows the quality reviewer's rule in the
SKILL's **Model tiers**. The reviewer receives the list of the group's task and fix commits, each labelled with its
task, never a range.

**Scope it to what only a cross-task view can see.** The group's quality review already judged each task. It judged
correctness, tests, patterns, regression risk, and maintainability. Its verdicts travel to the aggregate reviewer as
inputs. The reviewer covers these items:

- cross-task contracts;
- integration seams;
- the integration verification and Outcome proof results;
- standing orders across tasks;
- when the plan's `**Proportionality:**` line reads `override by user`, whether anything this group shipped changed the
  case for that size.

The reviewer does not redo the quality review. It raises a per-task finding only where the cross-task view contradicts a
quality verdict.

**Delta re-review on the same agent.** The first pass is a fresh agent that reads the group. Every later pass continues
that same agent through the surface's native continuation path (named per surface in
[surface dispatch](../../routing/references/surface-dispatch.md)). Send it the fix commits, the re-run verification
output, and any standing-order line appended since. Ask it to confirm that each of its own findings is closed. Never
cold-start a reviewer that re-reads the whole group for a second pass. If a surface's subagents cannot be continued, do
these steps:

1. Re-dispatch with the prior findings and the delta.
2. Say so in the report.

A fix round that does not close the finding counts once on the ladder of the task the finding names, under the
**Two-strike stop**, and on no other task's.

**Skip the agent only when no material integration change exists.** Group size is never the evidence. A single-task
group can change a contract that code from an earlier group depends on. After the integration verification and any due
Outcome proof, inspect the group's shipped diff. Compare it against every other group's code, earlier or concurrent, for
these relationships:

- interfaces or contracts and their callers or consumers;
- configuration keys, defaults, registries, or feature flags and their readers;
- persisted formats, schemas, storage semantics, and migrations with their readers and writers;
- command or API call relationships;
- generated artifacts and package or install boundaries consumed elsewhere;
- shared behavioral assumptions, ordering, or side effects.

Dispatch a fresh aggregate reviewer when any of these holds:

- Any such relationship is introduced or altered.
- Compatibility or the absence of a relationship is uncertain. Uncertainty is never a reason to skip.
- The group joins two or more other groups, meaning it depends on tasks in at least two of them.
- The group holds a packet marked `**Packet review:** yes`.

Only an evidenced no-material-change result skips. When you skip, the boundary is the group's quality verdict plus the
integration verification and the Outcome proof you ran. No fresh Deep-tier agent is dispatched. Record
`aggregate review skipped: <evidence>` in the boundary report, the run report, and the decision trail, and write the
`waive aggregate-review` row. `<evidence>` names what the diff changes and which other-group relationships you
inspected. Never skip when the plan's `**Proportionality:**` line reads `override by user`. Never skip while a
**Two-strike stop** is open in the group. Those groups carry a question only the agent can answer.

**Verdict rows.** Write `verify group-<name>`, with status `ok` and evidence `commit=<integrated sha>` written by hand,
when the review returns CLEAR (after any fix rounds on the continued reviewer) and, on a recorded skip, together with
the `waive aggregate-review` row. A FINDINGS return is recorded as a `failed` `review` row, not as a
`verify group-<name>` row. A commit that fixes an integration finding gets its own `verify <task-id>` row (never a
`verify group-<name>` label) carrying `commit=<sha> patch-id=<id>` from the continued reviewer's verdict. Write a
`review` row with `kind=integration` each time the reviewer returns. Every review group boundary (including middle waves
such as `wave-2`) must have either a `review` (`kind=integration`) return or an evidenced `waive aggregate-review` row
before `verify group-<name>`. The row targets are in [flow-state.md](flow-state.md) and
[plan-divergence-and-trail.md](plan-divergence-and-trail.md).

**The final review.** Two groups are independent when no task in either depends, directly or through other tasks, on a
task in the other, so in a chain of groups the first and the last are not independent. A plan with two or more mutually
independent groups also gets one final integration review over the whole branch diff, after the last group clears. Its
owner is decided by the first row that matches:

| Order | Plan              | Owner                                                                                                                                                                                                                                                  |
| ----- | ----------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| 1     | A plan with lanes | The home session: the one that runs execute on the plan with no slice argument, on a branch that has merged the base after every lane merged. A lane session never runs it, because a lane stops at its draft pull request and cannot know it is last. |
| 2     | Otherwise         | The run's own session, after the last group clears.                                                                                                                                                                                                    |

Independent groups share no dependency edge, but they can still meet through shared configuration, registries, or side
effects that each group's aggregate review saw only one side of. Skip the final review only on the same evidence the
group skip needs, read over the whole branch diff: no relationship between groups is introduced or altered that no
aggregate review already saw, and none is uncertain. Record that evidence in one `waive final-review` row. The verdict
row is `verify final`, with evidence `commit=<integrated sha>`, written when the review returns CLEAR or with the skip.
The same prompt and the same failure ladder apply.
