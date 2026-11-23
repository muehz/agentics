# Isolated packets

Rules for a packet whose implementer works in a worktree other than the controller's. Shared-worktree execution (`auto`
siblings and `pair`) does not load this file.

## Which packets

A packet is isolated when its implementer works in any worktree other than the controller's. The worktree is one of
these:

- one the controller created
- one a native `isolation: "worktree"` dispatch created
- an `arena` candidate's

No mode selects isolation; the dispatch does. `arena` candidates follow [arena-mode.md](arena-mode.md). The Start rule
here applies to their worktrees.

## Three commits

Record three commits for every isolated packet. Keep them distinguishable:

- **base:** the integration branch tip the packet must start from, captured at dispatch and written into the packet.
- **head:** the commit the worker returns.
- **integrated:** the integration branch commit after you integrate the head.

The task's `verify <task-id>` row in `state.tsv` carries them, with the patch-id of the worker's diff, as
`base=<sha> head=<sha> integrated=<sha> patch-id=<id>` ([flow-state.md](flow-state.md)). `head=` is the head you
actually integrate. `base=` is the dispatch base, unless the worker has since merged the integration tip into its branch
(see Repair). `<id>` is the first field of `git diff <base> <head> | git patch-id --stable`, recorded when the worker
returns its head and before you integrate it.

A bug-fix task's worker also returns `Red: <sha>` beside `Head: <sha>`: the commit that holds only its failing test,
made before the fix as `tdd` requires. Confirm the red commit touches only test files and its test fails at that commit.
Then record a `verify red-<task-id>` row with `evidence` `auto:<sha>`. `flow` computes it as
`commit=<sha> patch-id=<id>`. The red commit lies in the task row's `<base>..<head>` range. When a pre-commit hook
rejected the red commit, the worker returns `Red: none` with the failing run's command, exit status, and output excerpt,
and you record `verify red-<task-id>` in the no-commit form ([flow-state.md](flow-evidence.md#evidence-forms)).

## Start

Create the worktree yourself at the base through `/agentics:git worktrees`. Read the selected worktree path, task
branch, and validated base id from the packet as data in the same shell call. Pass them in `worktree_path`,
`task_branch`, and `base_oid` to `git worktree add -b "$task_branch" -- "$worktree_path" "$base_oid"`. Dispatch the
implementer into that path.

A native `isolation: "worktree"` dispatch does not choose its base from the controller. Every isolated packet, however
its worktree was made, compares `git rev-parse HEAD` with its base before any work. On a mismatch it returns
`BLOCKED: wrong base — expected <base>, found <sha>`. It changes nothing. It never resets, checks out another commit, or
rebases.

## Release

The worker's own verification run in its worktree is input to the quality review, as for any packet. Dependents are
released only on the wave's integration verification, run at the `integrated` commit. That `verify wave-<n>` row records
the commit it ran on.

When the integration branch advanced past the packet's base before you integrated its head, do these steps before you
release:

1. Integrate the head into the current tip.
2. Rerun any check whose evidence predates `integrated`.

Integration is always a merge of the worker's head, never a rebase or cherry-pick. A merge keeps the worker's commits
with their SHAs and the patch-ids recorded for them. `gate` audits them. A dependent starts only from the `integrated`
commit its dependency's evidence names.

## Repair

Return a merge conflict to the same worker as a continuation. Do the same for an integration-verification failure that
the tip's advance caused. The continuation tells the worker to do these steps:

1. Merge the current integration tip into its own branch.
2. Resolve the conflict or failure.
3. Re-verify.

The `verify` row then records the head you actually integrate. Its `base=` is the tip the worker last merged, so
`git diff <base> <head>` and its patch-id cover only the worker's own changes. Never rebase or reset.

A continuation that fails writes `verify <task-id>` with status `failed` and evidence `cause=repair`. It counts on the
task's one failure ladder in [two-strike-stop.md](two-strike-stop.md), together with every other failure of the task,
whatever its `**Role:**`. When the ladder calls for a fresh worker, give it a new worktree whose base is the current
integration tip and a rewritten packet that names the earlier head and each attempt's conflict or failing check. The
fresh worker's head gets its own patch-id. When the ladder stops the task, report each attempt's conflict or failing
check in the stop report. The task stays blocked with its dependents held, and the rest of the run continues.

## Cleanup

The skill that creates a worktree removes it. Once an isolated packet's head is integrated, or its task is dropped, read
the recorded worktree path into `worktree_path` in the removal call and run `git worktree remove -- "$worktree_path"`
without `--force`. When Git refuses because the worktree holds uncommitted changes, report the path and leave it:
removing it would discard work, which needs the user's confirmation of that target. While a task is stopped and waiting
for the user, keep its worktree.

## Missing inputs

A worktree holds tracked files only. Gitignored fixtures, local configuration, and services can be absent. A test that
skips there still exits 0. The verification preflight's comparison run
([verification-preflight.md](verification-preflight.md)) detects this before dispatch. Never copy credentials into a
worktree. Never move a packet into the main checkout unless the user chooses it.

## Tracker state

The SKILL's **Tracker state** rule applies to every run, isolated or not. For an isolated packet, the dependency is
finished at its `integrated` commit. A linked issue that stays open never holds a dependent.
