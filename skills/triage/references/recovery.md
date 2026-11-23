# Re-running and Recovery

What happens when a triage run crashes or stops early, how to re-run on the same PR, and what a re-run does and does not
recover. The triage skill keeps no persistent state file. Every recovery path uses the worktree, branch, and
idempotency-marker mechanisms in [orchestration-lifecycle.md](orchestration-lifecycle.md).

## TL;DR

- **A re-run is safe by default** (see "How re-runs stay safe").
- **Phase 0.1 reports kept branches and worktrees from earlier runs of this PR as notes.** It never stops on them and
  never removes them. [Clearing leftovers](#clearing-leftovers) says how to remove them.
- **There is no separate recovery mode.** Re-invoke `/agentics:triage pr <N>` after a crash.

## How re-runs stay safe

1. **`RUN_ID` per invocation.** Phase 0.3 prints `run-id: <seconds>-<pid>` on every invocation, and the run uses that
   value as `{RUN_ID}` in every later phase. Branch and worktree names carry it as a suffix
   (`triage/pr-{PR_NUM}/{source}-{RUN_ID}` and `{WT_PARENT}/triage-pr-{PR_NUM}-{source}-{RUN_ID}`), so a re-run gets
   fresh names and never collides with an earlier run's state on disk.

2. **Phase 5.2 skips a commit that is already on the branch.** If a fix from an earlier run is already on the PR branch,
   the cherry-pick stops with nothing to commit. The orchestrator runs `git cherry-pick --skip` and records the source
   SHA as `already_applied`, so the run creates no duplicate commits and no empty commit.

3. **Idempotency markers in Phase 6.** Every thread reply carries the HTML marker
   `<!-- agentics:triage:reply:{source}:{threadId} -->`. Before it posts a reply, the orchestrator looks for an existing
   reply on that thread with the same marker and the author `{LOGIN}` (Phase 6.1). On a hit, it updates that reply in
   place and posts no duplicate. A rollup is not found by marker. Each execution creates its own rollup under its key,
   `<!-- agentics:triage:execution:{RUN_ID} -->`, and links the earlier rollup posted by the authenticated login (see
   [rollup-template.md](rollup-template.md) "Execution").

4. **A push is a plain fast-forward, and a moved branch is never overwritten.** Triage pushes through the git skill's
   [push workflow](../../git/references/push.md) with no force flag. When the PR branch moved during a run, Phase 5.6
   pushes nothing and keeps every branch of the run. Triage never merges the new tip, rebases, or force-pushes.

Together, these make **a re-run the recovery path** for almost every failure mode below.

## Clearing leftovers

Phase 0.1 of a re-run prints a note for each branch and worktree that an earlier run of this PR kept. The notes match
the prefix `triage/pr-{PR_NUM}/`, so they never include another PR's state. They do not stop the run: the fresh run
starts from the PR head and inherits nothing beyond what is already on the PR branch and in posted PR comments.

Before you remove a kept branch, check it for work you want. A kept branch holds one of these:

- fixes that a push never carried (a blocked push, a fork PR, or a run with no push authorization);
- a draft the developer skipped in Phase 4.2;
- a commit that conflicted in Phase 5.2 or that a path rule held;
- a blocked base sync's partial resolution.

Remove the worktrees with `/agentics:git worktrees prune`, which audits each one and removes it with a plain removal. A
worktree that holds uncommitted changes is never removed in bulk. To discard one, name it to the git skill, which
confirms once. Branch refs survive a worktree removal. Delete a branch you no longer want with
`/agentics:git branch delete <name>`, one branch at a time.

To list what a run kept without starting a run, print the notes of Phase 0.1:

```bash
git branch --list "triage/pr-{PR_NUM}/*" --format='kept-branch: %(refname:short)'
git worktree list --porcelain | while IFS= read -r line; do
  case "$line" in
    "worktree "*) wt=${line#worktree } ;;
    "branch refs/heads/triage/pr-{PR_NUM}/"*) printf 'kept-worktree: %s\n' "$wt" ;;
  esac
done
```

## What a stopped run keeps

A run that reaches Phase 5.8 removes what it created, as that step's table says. These are the rows that leave state
behind, and what each leaves:

| How the run ended                                                                             | Kept branches and worktrees                                                          |
| --------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------ |
| Crash or abort in any phase after 0.3                                                         | Every worktree and branch of the run (the abort contract). Nothing is removed.       |
| `push_blocked`: failed verification, failed inspection, a moved PR branch, or a rejected push | Everything, with the merge branch holding the fixes that did not land.               |
| `fork_skipped`                                                                                | For `run-approved`, the merge branch and worktree. For `yes`, every source worktree. |
| No push authorization                                                                         | The merge branch and worktree.                                                       |
| Push landed (`ok`)                                                                            | A source branch with a skipped draft, a held commit or a conflict, and its worktree. |
| Blocked base sync                                                                             | The sync branch `triage/pr-{PR_NUM}/sync-{RUN_ID}` and its worktree, on purpose.     |

## When the PR merged or closed between runs

Phase 0.3 stops any run whose PR is not `OPEN`, before it creates any worktree or branch or starts any fetch. Phase 0.1
runs first and only reads, so a re-invoke on a merged or closed PR prints the notes for what earlier runs kept, then
reports that the PR is not open and stops. It strands nothing, and one re-invoke is enough to see what is left.

Fixes that were never pushed were authored against a PR that no longer accepts them. Before you clear a kept branch
under [Clearing leftovers](#clearing-leftovers), look at its commits. Cherry-pick anything of value onto a fresh branch
first.

## Crash matrix

By phase, what happens when a triage run crashes before it finishes. In every case below, the recovery has two steps:

1. Fix the immediate cause, if any.
2. Re-invoke `/agentics:triage pr <N>`.

The mechanisms above do the rest.

### Crashed during Phase 0.1 to 0.3 (notes, PR facts, fetch)

No worktree or branch exists yet. The fetch writes only the remote-tracking ref `refs/remotes/triage/pr-{PR_NUM}`, and
the next run's fetch overwrites it. Re-invoke. Nothing to clean up.

### Crashed during Phase 0.4 (base sync)

The sync worktree (`{WT_PARENT}/triage-pr-{PR_NUM}-sync-{RUN_ID}`) and its branch (`triage/pr-{PR_NUM}/sync-{RUN_ID}`)
may exist, with or without a merge commit, according to where the crash landed. The base fetch also wrote the
remote-tracking ref `refs/remotes/triage/pr-{PR_NUM}-base`, which the next fetch overwrites. Re-invoke. The new run gets
a fresh `RUN_ID`, lists the old sync worktree and branch in its Phase 0.1 notes, and redoes the sync against the current
base.

A **`blocked`** sync is a normal stop and not a crash. It occurs when the specialist hits a `defer` that it cannot
resolve, when the merge breaks a check that it cannot fix, or when a sync commit touches a path triage never pushes. A
blocked sync leaves its branch and worktree behind on purpose, for the developer to inspect and finish by hand. A later
run lists them in its Phase 0.1 notes like any other leftover. Removing them discards the specialist's partial
resolution, so pull anything of value first. If you re-invoke without removing them, the new run re-syncs from scratch
in a new worktree. It does not reuse the earlier attempt's partial resolution.

A re-run after a successful push finds the PR `up to date`. Phase 0.4's ancestry check (`git merge-base --is-ancestor`,
base ref against `{PR_HEAD_SHA}`) now succeeds, so `SYNC_OUTCOME` is `up to date` and the sync is a no-op. There is
nothing to merge and nothing to clean up.

### Crashed during Phase 1 (detection)

No commits, no comments, and no worktrees yet. Re-invoke. Nothing to clean up.

### Crashed during Phase 2 (worktree setup)

Some source worktrees may exist under `{WT_PARENT}/triage-pr-{PR_NUM}-{source}-{RUN_ID}` with their
`triage/pr-{PR_NUM}/{source}-{RUN_ID}` branches, with no commits yet (subagents have not run). Re-invoke. The new run
lists them as kept and gets a fresh `RUN_ID`.

### Crashed during Phase 3 (subagent dispatch)

The state matches Phase 2, plus possibly some commits on source branches if subagents had begun to commit. Re-invoke.
Subagents re-run, re-detect the same findings, and produce the same set of commits on fresh
`triage/pr-{PR_NUM}/{source}-{NEW_RUN_ID}` branches. A subagent that did not return within the wait limit is `Failed`
for that run, and its branch is kept (Phase 3.3).

### Crashed during Phase 4 (collect results and draft decisions)

Nothing has been integrated or posted. The developer's accept, edit and skip decisions of Phase 4.2 exist only in the
session, so a crash loses them. A re-run presents every draft again. An unattended re-run dispatches no draft-lane
source and reports it as `Pending Review`.

### Crashed during Phase 5 (cherry-pick and push)

Three cases:

- **Crashed or stopped before the push** (Phases 5.1 to 5.6). Cherry-picked commits exist on
  `triage/pr-{PR_NUM}/merge-{OLD_RUN_ID}` and were never pushed. The PR branch is unchanged. Re-invoke. The new run
  integrates fresh and pushes once. That push carries the base sync's merge commit from the new run's own Phase 0.4,
  since that commit was never pushed either.

- **The PR branch moved during the run.** Phase 5.6 found that someone pushed since Phase 0. The run set `PUSH_STATUS`
  to `push_blocked` and pushed nothing. The kept merge branch is not a recovery path for the run itself: triage never
  merges the new tip, rebases, or force-pushes. Re-invoke `/agentics:triage pr {PR_NUM}`, which starts from the new tip.
  Or integrate the kept merge branch yourself, with `/agentics:git`.

- **Crashed after the push, before Phase 5.8 cleanup.** Commits are on the PR branch, but the merge worktree and the
  source branches still exist. Re-invoke. Phase 5.2 detects each of those commits as already on the branch and records
  it as `already_applied`, so the run creates no duplicate commits. A pushed base sync heals the same way: the ancestry
  check of Phase 0.4 reads `up to date`.

A rejected or failed push is never retried inside the run. After you fix the cause, re-invoke, or push the kept merge
branch with `/agentics:git push`.

### Crashed during Phase 6 (post)

Commits are on the PR branch. Some replies or the rollup may or may not have posted. Re-invoke. The re-run is a new
command, so a new execution with a fresh key:

- A crash before Phase 6.4 leaves no rollup. The new run posts the first one.
- A crash inside Phase 6.4 after the create call succeeded leaves a posted rollup. The new run posts a second rollup
  that links it.
- The new run links an earlier rollup only when its author is the login the session is authenticated as.
- The new run updates a previously posted thread reply in place (marker hit in Phase 6.1) and does not post a duplicate.
  Its text then says "already applied" where the first reply cited a SHA, because the fix was already on the branch when
  the new run integrated it.
- Human-feedback drafts were never posted, so an interactive re-run drafts them again and walks the developer through
  accept, edit or skip. An unattended re-run reports the source as `Pending Review` instead.

### Crashed after Phase 6.2 (replies posted) but before Phase 6.4 (rollup)

The replies posted, but the rollup never did. Re-invoke. The new run re-integrates (every commit is `already_applied`),
re-drafts the replies (each one hits its marker and updates in place), and posts the first rollup under its own
execution key.

### Crashed during Phase 7 (posted-format verification)

Replies and the rollup are already posted. Phase 7 is read-only and non-blocking, so a crash here leaves no partial git
state. Re-invoke after you fix the immediate cause. The re-run is a new execution: it posts a new rollup under its own
`<!-- agentics:triage:execution:{RUN_ID} -->` key, and that rollup links the posted one when the authenticated login
posted it. Marker idempotency keeps protecting thread replies, and the new run repeats the verification check against
its own posted bodies.

## What re-runs do NOT recover

- **A rewound PR head between runs.** The user or a teammate may force-push the PR branch to discard the pushed fixes.
  After such a force-push, a re-run integrates the same fixes again and posts a fresh rollup. The skill does not detect
  this case, because it cannot distinguish "the user discarded our fixes on purpose" from "the user rebased on top of
  our fixes." Triage itself never makes this force-push.

- **Manually edited PR comments.** If a user removes the idempotency marker from a posted triage comment, the next run
  treats the comment as absent. On a thread reply, the run posts a duplicate reply. On a rollup, the comment drops out
  of earlier-rollup link discovery, so the next rollup does not link it. Preserving the marker is the user's
  responsibility.

- **Replies for a fork PR.** Triage never pushes to a contributor's fork. For fork code (`PUSH_STATUS` is
  `fork_skipped`), Phase 6.2 withholds the thread replies, because the SHAs they cite do not exist on the remote, and
  only the rollup posts. The proposed fixes stay on this machine, as Phase 5.7 says. After the contributor applies them,
  a re-run starts from the new PR head and posts its replies normally.

- **Subagent JSON output that was malformed.** The orchestrator skips the malformed source's findings and continues with
  the rest. A re-invoke does not repair the malformed output. It re-dispatches the subagent, which produces fresh JSON.
  If the subagent itself causes the malformation (for example, a prompt-injection issue), a re-run reproduces the
  problem. Fix the subagent prompt or input data. Then re-invoke.

- **A check that failed for a transient reason.** No one in a triage run re-runs a check, and a re-invoke does not
  either. The Checks section recommends a re-run for the developer to start.

## Cross-references

- [orchestration-lifecycle.md](orchestration-lifecycle.md): Phase 0.1 notes, the abort contract, Phase 5.2
  `already_applied` skip, Phase 5.6 moved branch, Phase 5.7 push, Phase 5.8 cleanup, Phase 6.1 idempotency check
- [rollup-template.md](rollup-template.md): marker conventions, the execution and its key line, and the earlier-rollup
  link
- [subagent-contract.md](subagent-contract.md): subagent constraints (no push, no post, no re-run) that make re-runs
  safe to dispatch

## Design notes

- **No persistent state file.** The design rejected a `.triage-state-{PR_NUM}.json` file. It would have to be in
  `.gitignore` (otherwise it pollutes the PR). It would need atomic writes to be crash-safe. It would duplicate state
  that already exists on disk (worktrees, branches) and in posted comments (markers). It costs more to build and
  maintain than it adds over the existing mechanisms.

- **`RUN_ID` is intentionally volatile across runs.** Two runs of the same PR get different `RUN_ID` values. That makes
  Phase 5.2's `already_applied` path safe, because the new run does not need to know about the old run's branches at
  all.

- **Markers are the contract, not the database.** When a reply marker is missing or edited away, the next run posts a
  duplicate reply. An execution's rollup is identified by its key, `<!-- agentics:triage:execution:{RUN_ID} -->`, under
  the authenticated author, not found by a marker search. This trade-off is deliberate. The alternative, a server-side
  database keyed by PR number, is heavy infrastructure for a use case where duplicate-and-update is the worst plausible
  outcome.
