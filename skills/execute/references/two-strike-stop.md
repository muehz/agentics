# Two-strike stop

This file holds the rules that the `Two-strike stop` section of [`SKILL.md`](../SKILL.md#two-strike-stop) points to.

The plan marks each task `**Role:** outcome` or `**Role:** support`. A `support` task is one the Outcome proof command
still passes without. The stop below applies to both roles.

## One ladder per task

Each task has one failure ladder. Its count is read from the task's rows, so it survives a new session. A **failure** is
any one of these, and each counts once against the task being worked, never against another task whose files a fix
touched:

| Failure                                                                    | The row that records it                                           |
| -------------------------------------------------------------------------- | ----------------------------------------------------------------- |
| A correction that the next review or verification still does not pass      | `verify <task-id>` with status `failed`, evidence `auto:<sha>`    |
| A commit that fails the controller's own verification                      | `verify <task-id>` with status `failed`, evidence `auto:<sha>`    |
| A failed continuation of an isolated packet (a conflict or a failed check) | `verify <task-id>` with status `failed`, evidence `cause=repair`  |
| A `REVISE` from the Stage 1 reviewer                                       | `review <task-id>` of kind `packet` with status `failed`          |
| A second `NEEDS_CONTEXT` return on the same packet                         | `verify <task-id>` with status `failed`, evidence `cause=context` |
| A red leg whose test passes, or fails for another reason                   | `verify <task-id>` with status `failed`, evidence `cause=red`     |

The count is the number of those rows written since the task's latest `pivot <task-id>` or `unblock <task-id>` row, or
since the run began when it has neither. After an `unblock` row the ladder is shorter, so the answer's two attempts
hold: the granted fresh implementer works from count 0, its first failure goes back to it, and its second failure stops
the task as count 4 does.

| Count | What happens                                                                                                           |
| ----- | ---------------------------------------------------------------------------------------------------------------------- |
| 1     | The same implementer gets the findings and tries again.                                                                |
| 2     | One fresh implementer, with a rewritten packet that carries each finding, each failed approach, and the latest output. |
| 3     | The fresh implementer tries again.                                                                                     |
| 4     | **Stop.** Write `block <task-id>`, report, and ask the user.                                                           |

The user hears after at most four failures and two workers. The isolated-packet repair stop is this same stop.

**A bug-fix task has a lower limit.** `diagnose` allows three fix attempts per defect, across implementers, before the
approach itself is in question. For a task with a `verify red-<task-id>` row, each failed `verify <task-id>` row after
the red row is a fix attempt, and a fresh implementer does not reset that count. The third failed attempt stops the task
with the same report, whatever the ladder's count.

**Only a replaced packet resets the count.** A packet the user replaces writes `pivot <task-id>`, and the ladder starts
again from zero.

## The stop

Report in this shape, then wait for the answer:

> **Two strikes:** `<task-id>` failed twice, then twice more after a fresh implementer. **Findings so far:** \<one line
> each\>. **What it delivers:** \<the outcome, or for a support task the outcome task or proof it exists for\>.
> **Question:** continue, drop, or replace the task? For a support task, also ask whether the machinery should exist or
> \<the named alternative\> covers it.

A second fresh context that also fails means the question belongs to the user. A task that turns green again and again
while each next review finds something else counts the same as one that will not go green.

The stop is not a code-quality judgment and never lowers the quality review's bar. Every failure it counted was real.

**The stop holds one task, not the run.** Name each consequence:

| What                                       | What happens                                                                                                                                                         |
| ------------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| The stopped task                           | Not done, not `skip:`. It stays `blocked: <the question>` until the user answers.                                                                                    |
| Siblings in its review group               | They stand. A passing task is not un-passed by a sibling's stop.                                                                                                     |
| Its review group's reviews                 | The quality review runs over the tasks that committed, and the aggregate review over those that passed, with no skip. Both name the stopped task as an open blocker. |
| Tasks that depend on the stopped task      | They wait, marked `blocked:`.                                                                                                                                        |
| Every other task, including in later waves | It continues.                                                                                                                                                        |
| The run's completion                       | It never reaches **Definition of done** while the task is open.                                                                                                      |

In `auto`, a stop blocks that task only. When every other task finishes with the question unanswered, report what
shipped, report the stopped task and its question, write `pause run`, and end there. That is a complete and honest end
state, not a failure.

## The answer

Read the answer by the first row that matches:

| Order | Answer                     | Do                                                                                                                                                                              |
| ----- | -------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1     | Drop                       | Write `drop <task-id>` with status `cancelled` and the reason. Each dependent gets its own `drop` row, or a `pivot` row when it is re-planned. A dropped task counts as closed. |
| 2     | Replace, with a new packet | Write `pivot <task-id>`. The ladder starts from zero.                                                                                                                           |
| 3     | Continue                   | Write `unblock <task-id>`. It grants one more fresh implementer with two attempts, not an unbounded run: two more failures stop the task again.                                 |
| 4     | Otherwise                  | Ask once which of the three the answer means.                                                                                                                                   |

A plan whose `**Proportionality:**` line reads `override by user` carries a size the user chose knowingly. Keep the
stop. The **Aggregate wave review** asks whether each review group changed the case for that size, and such a group
never takes its skip.

## The Outcome proof

The plan names an **Outcome proof** and the skeleton path that delivers it: one command and the observable result it
must produce. Run that command at the boundary of the wave that completes the skeleton path, and show its real output.
Until it produces its named result, later waves build on an unproven path, which is a blocker, not a note.
