# Gate emission: which rows, where they go, and how they land

Every completion-mode run writes its rows to a `state.tsv` through `agx flow log`. `check` mode writes nothing: it
produces a pass or fail note, not a completion artifact. [flow-state.md](../../execute/references/flow-state.md) states
the commands, the columns, their closed sets, the exit codes and the append-only contract once. This page says which
rows gate writes, which file takes them, and how the gate stages and commits that file. Call bare `agx flow …`, with the
launcher-path fallback [flow-state.md](../../execute/references/flow-commands.md#commands) gives.

## The rows

| Row                                        | Written when                                                                         | Mode            |
| ------------------------------------------ | ------------------------------------------------------------------------------------ | --------------- |
| the verdict: `branch`, `release`, `change` | once per run, after the mode's artifact exists, or at a stop                         | all three       |
| `waive <what>`                             | when the user declines a process gate or accepts or defers a finding, at that moment | branch, release |
| `commit <kind>`                            | right after a commit gate made that is not artifact-only                             | branch          |
| `approve <artifact>`                       | on the user's answer to Step 5's question, or to Step 8's approval question          | branch          |
| `escape`                                   | when Step 8 finds a Critical or Important defect that an earlier review cleared      | branch          |
| `pause`                                    | after the verdict row, when the run stops before a destination                       | branch          |
| `done`                                     | last, after a local merge or a discard has deleted the branch                        | branch          |
| `skip <item>`, `na <item>`                 | for a release checklist item or a change record section skipped or not applicable    | release, change |
| `bind`                                     | through `flow bind`, once, after the gate resolves a flow it did not create          | branch          |

A row written mid-run stays written when the run stops later, so a stop never loses a decision the user already made.

## The verdict row

| Cell       | Value                                                                                                          |
| ---------- | -------------------------------------------------------------------------------------------------------------- |
| `phase`    | `gate`                                                                                                         |
| `event`    | the mode: `branch`, `release`, or `change`                                                                     |
| `target`   | the mode's operand: the branch name, the tag, or the change id                                                 |
| `status`   | from the table below                                                                                           |
| `why`      | one line that names what the verdict rests on, e.g. `tests 31/31, review 0 Important open, 1 finding accepted` |
| `evidence` | the PR number, tag, or change-record path the mode produced, or `none`                                         |
| `next`     | the first word names the phase that does the remaining work, then one line says what to do                     |

When branch mode can no longer read the branch name (the branch was deleted and the session file is gone), the `target`
is `name:lost`. No real branch can match it, because git forbids `:` in branch names.

For branch mode, the gate status of Step 11 decides `status` and `next`. The first row that matches wins:

| Situation                                                               | `status`    | `next` starts with |
| ----------------------------------------------------------------------- | ----------- | ------------------ |
| The user confirmed a discard                                            | `cancelled` | `done`             |
| Tests failed (accepted or not), or a local merge's merged result failed | `failed`    | `execute`          |
| Tests not run or deferred, with no pointer                              | `blocked`   | `gate`             |
| `NOT READY`, or a stop before a destination                             | `pending`   | `gate`             |
| `READY`, with a local merge                                             | `ok`        | `done`             |
| `READY`, with any other destination                                     | `ok`        | `gate`             |

A waived step does not change the status by itself: the `waive` row is its record, and the verdict's `why` names it. No
status records an unverified check as passed. Release and change use the same five statuses, mapped by the tables in
[release-workflow.md](release/release-workflow.md#write-the-rows) and
[change-workflow.md](change/change-workflow.md#write-the-rows).

A `next` that starts with `gate` is offered by `routing`, never run, because completion is an explicit request.

On a slice or lane branch bound with a slice, the helper appends the branch's `slice=` pair to the verdict row, so the
verdict covers that slice only ([Slice scope](../../execute/references/flow-commands.md#slice-scope)).

## The `waive` row

| Cell       | Value                                                                                               |
| ---------- | --------------------------------------------------------------------------------------------------- |
| `event`    | `waive`                                                                                             |
| `target`   | what was declined: a branch target below, or a release checklist item                               |
| `status`   | `ok`                                                                                                |
| `why`      | the user's reason, in their words when they ask for that, or `none given`; one line, with no secret |
| `evidence` | `issue=<link>` for a deferred finding, else `none`                                                  |
| `next`     | the `next` of the row before it                                                                     |

Branch mode's targets are `docs-refresh`, `review`, `converge`, `tests`, `work-folder`, `security-depth`, and
`finding <lane> <locator>`. Release mode writes `waive <item>` when the user proceeds past a failed or skipped checklist
item. `<item>` is the section's name (`artifact`, `migrations`, `flags`, `rollback`, `ci`, `on-call`, `comms` or
`window`), followed by `:<check>` when the section has more than one check, as in `waive ci:required-checks`
([release-workflow.md](release/release-workflow.md#item-outcomes)). Change mode writes no `waive` row; its `skip` and
`na` rows name a record section.

A finding's `<lane>` is the review lane that reported it, or `audit`, `red-commit` or `trail` for the Step 8 checks. Its
`<locator>` is the first of these that the finding has: `<path>:<line>`, `<path>`, `commit:<sha>` for a finding about
one commit (an unaccounted or mismatched commit, or a red commit's later test change), or `task:<task-id>` for a trail
finding about a task. An audit finding from an `invalid:` line takes `task:<target>` with the invalid row's target, as
in `task:task-3` for an invalid `verify task-3` row, `task:red-task-3` for an invalid `verify red-task-3` row, or
`task:fix` and `task:red-fix` on a small fix. The row records the answer. `flow status` prints one `waivers:` line per
valid row, marked `attested` when this clone's ledger holds it
([Reading the state](../../execute/references/flow-commands.md#reading-the-state)). When several lines name one target,
the latest decides. Only when its locator names the finding exactly does an `attested` line answer the finding, so that
a later run in this clone does not ask again: `commit:<sha>` or `task:<task-id>`, the forms of the `audit`, `red-commit`
and `trail` lanes. A `<path>` or `<path>:<line>` locator, even attested, and any `unattested` line leave the finding to
be asked again, naming the earlier answer ([Step 8 item 7](branch/review-and-audit.md#step-8-review-and-audit)).

## The `commit` row

Written by the step that made the commit, in the same turn, or by Step 11's Accept for an unaccounted commit, as the
[Commit rows](branch/branch-workflow.md#commit-rows) table says. The evidence is `auto:<sha>`.

## The `approve` row

Gate writes an `approve` row only on the user's answer. A yes passes `--expect-blob` with the `text=` hash that
`flow status` printed after the edit, as
[the approval guard](../../execute/references/flow-approval-and-audit.md#the-approval-guard) requires. A no writes
status `declined` with evidence `none`. Gate never carries an earlier approval forward over an edit.

## The `escape` row

When the gate finds a Critical or Important defect that an earlier review cleared, it writes one `escape` row with phase
`gate`, status `failed`, and the evidence
`found-by=gate severity=<critical|important> review=<plan|packet|quality|integration> word=<the lane's own word>`. The
row copies the `next` of the row before it. `target` is the task, group or `final` whose commit introduced the defect,
from the flow's `verify` rows. `review=` names the review that cleared it: `quality` for a group's quality review,
`integration` for a group or final integration review, and `packet` or `plan` when only those covered it.

## The `done` row

After a local merge or a discard, and only once the branch is deleted, write `done` last. Nothing writes after it,
because a later row makes the flow open again. A merge that left the branch in place writes no `done`.

| Cell       | Value                                                |
| ---------- | ---------------------------------------------------- |
| `event`    | `done`                                               |
| `target`   | the branch                                           |
| `status`   | `ok`                                                 |
| `why`      | the destination, e.g. `merged locally into the base` |
| `evidence` | the merge commit as `commit=<sha>`, or `none`        |
| `next`     | `done merged` or `done discarded`                    |

A draft pull request writes no `done`. Its flow closes when the pull request merges and the default branch's tip holds
an `ok` verdict for the whole plan, or, under the `local` storage setting, when `routing` writes the `done` row
([Closing a flow](../../execute/references/flow-commands.md#closing-a-flow)). A slice branch's local merge writes no
`done` either: the flow closes when every slice has merged.

## Where the rows go

Run `flow where` from the feature worktree. The rows go to the printed flow's `state.tsv`. On exit 3, after the user
named no folder, on a small fix (exit 5), and on the default branch or a detached `HEAD`, pass
`--folder <work-root>/unassigned`. Gate never creates a work folder.

Options 1 and 4 resolve the folder before the merge or the discard and pass `--folder` on every later call, because the
shell then stands on the base, where `flow where` finds no flow for this branch. After a discard, a folder that existed
only on the deleted branch holds no `state.tsv` in the current checkout, and `flow log --folder` would create it; the
rows go to `<work-root>/unassigned` instead, and the report says so.

`flow bind` runs in branch mode's Step 2, right after the flow is resolved and before any row, and never after `done`,
because `bind` copies the `next` of the last row and would make the flow open again. It refuses the default branch, so
Option 1 commits the `bind` row on the feature branch before its merge. A discard named in advance binds nothing,
because a `bind` row on the feature branch is deleted with it.

## Stage and commit what the run wrote

Under the `local` storage setting nothing is staged or committed: the clone-local store is not in the repository. Under
`committed`, run `git add -- <the state.tsv just written>` on its own after the last row, and report a failure.

| Mode and destination                 | Commits                                                        | Pushes                                                                         |
| ------------------------------------ | -------------------------------------------------------------- | ------------------------------------------------------------------------------ |
| branch, Option 1 (merge)             | the `bind` row before the merge; the rows on the base after it | no                                                                             |
| branch, Option 2 (draft PR or stack) | on the feature branch                                          | to the pull request branch, through `git push`, under Option 2's authorization |
| branch, Option 3 (keep), or a stop   | on the feature branch                                          | no                                                                             |
| branch, Option 4 (discard)           | no; the rows stay staged                                       | no                                                                             |
| `release`, `change`                  | no; the rows stay staged                                       | no                                                                             |

The commit names the one `state.tsv` path, runs every hook, and stops for a decision when another path is staged. It is
artifact-only, so it needs no row. A run with no push authorization commits and stops before the push.

**Why the rest stay staged.** A discarded branch is gone, and a commit in the base checkout would change a branch the
user never asked to change. `release` and `change` usually run on the default branch and make no commit of their own.
For these, report the rows as staged and uncommitted, for the user's next commit to carry.
