# Branch mode: Steps 11–14 — Propose, execute, clean up, and record

Split from [branch-workflow.md](branch-workflow.md). Covers Step 11 (propose destination), Step 12 (execute choice),
Step 13 (worktree cleanup), and Step 14 (write trail rows).

## Step 11: Propose the next step

The exit is one message: the status lines that apply, every finding with its severity, the advice list, and then either
the questions or the proposal.

**Status lines.** Print a line only for a step that ran or was waived. A step that did not apply prints nothing, and one
closing sentence names the steps that did not run and why, when any normally runs. The lines are `Gate status:`,
`Tests:`, `Work folder:`, `Reconcile:`, `Docs:`, `Review:`, `Security:`, `Controller audit:`, `Red commit:`,
`Converge:`, and `Divergences: <N> recorded` with each record's `Divergence:` line. None of them ever enters a pull
request body or a merge message.

**Gate status.** The first row that matches decides the line, and [../emission.md](../emission.md#the-verdict-row) maps
it to the verdict row:

| Gate status                         | When                                                                                                                                                                                                      |
| ----------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `BLOCKED (risk accepted by <user>)` | tests failed, not run or deferred, and the user accepted the risk in writing                                                                                                                              |
| `BLOCKED`                           | tests failed, not run or deferred, with no acceptance                                                                                                                                                     |
| `NOT READY`                         | a Critical or Important finding has no fix and no answer, converge gaps are withheld, the Security lane is incomplete and not waived, or the controller audit was not computed or, on a flow run, not run |
| `READY`                             | otherwise                                                                                                                                                                                                 |

`NOT READY` names the count or the hold: `NOT READY: <N> findings await a decision`,
`NOT READY: <G> spec gaps withheld`, `NOT READY: Security lane incomplete (<U> unsettled)` or
`NOT READY: controller audit not computed` or `NOT READY: controller audit not run (branch not bound)`.

**Questions.** While a Critical or Important finding has no fix and no answer, the message asks one question per finding
and ends there, before any proposal. A named destination waits too. A finding has an answer when the user gave one in
this run, or when an `attested` `waivers:` line answers it as Step 8 item 7 says; that finding gets no question and is
listed as `accepted <date>` with the row's reason. Every other finding with a recorded `waive` row is asked again,
naming the earlier answer and its date. The ➡️ line recommends from the finding itself, never from the row, and the new
answer writes a new row.

```text
❓ **Q1** - **<severity> finding: <lane> <locator>**: <the finding in one line>. Fix it, accept it, or defer it to an issue?

➡️ <the recommended answer and why>
```

- **Fix.** Gate never fixes a finding. The fix comes from `execute`, as a fix task under the same plan, or from
  `execute fix` for a small fix, or from the user, and its commit gets the `commit fix` row its author writes. When the
  user says the fix landed, the checks re-run once under the re-run budget.
- **Accept.** For an `audit` finding from an `unaccounted:` line, write `commit user` with `auto:<sha>` for that commit,
  in the form [Commit rows](branch-workflow.md#commit-rows) gives, with `--folder <work-root>/unassigned` on a small
  fix, and no `waive` row. `flow log` attests the row, so the commit is accounted for and this clone does not ask again.
  Every other Accept, a `mismatch:` or `red-commit` finding on `commit:<sha>` included, writes
  `waive finding <lane> <locator>`, with the locator [emission.md](../emission.md#the-waive-row) defines, and the user's
  reason or `none given`. No reason is asked for.
- **Defer.** Write `waive finding <lane> <locator>` with the issue link in its evidence (`issue=<link>`), for an
  unaccounted commit too. A deferral with no link is recorded as an accept.

A plan or brief approval that is not `current` gets no such question: it is the safety gate Step 8 holds, and it closes
only by a new approval of the artifact. A blocking finding of a spec or plan review closes the same way, by a fix or a
revision of the artifact ([severity.md](../../../routing/references/contracts/severity.md#rules)).

A Minor finding gets no question. When the trigger in
[Instruction lessons](../../SKILL.md#cross-cutting-rules-all-four-modes) fired, its question joins this round as a
question of its own.

**The proposal.** Once every question has an answer, end the message with one or two plain sentences that name one
destination and mention the others in one clause at most: no ❓, no ➡️ line, no numbered list. Discovery runs first:
[Find sibling slice branches](stack-handoff.md#find-sibling-slice-branches-read-only), then
[Choose the proposal](stack-handoff.md#choose-the-proposal). The first row that matches picks the destination:

| Situation                                                | Proposed destination                                                |
| -------------------------------------------------------- | ------------------------------------------------------------------- |
| A plan or brief approval is held (Step 8)                | keep the branch, with the revision path from Step 8                 |
| Converge withholds merge and pull request                | keep the branch, with the ways forward from Step 9                  |
| The Security lane is incomplete and not waived           | keep the branch, with the ways forward from Step 8                  |
| The controller audit was not computed, or not run        | keep the branch, with the ways forward from Step 8                  |
| The user declared an embargo for this work item          | keep the branch; nothing that describes the vulnerability is pushed |
| The base has no remote counterpart                       | Option 1, merge locally                                             |
| The forge host is not supported, or `gh` cannot run      | push the branch, and hand the user the create command               |
| The branch already has an open pull request              | Option 2: push to it and refresh its body                           |
| Discovery found dependent pull requests or a slice chain | Option 2's stack path, as the proposal table words it               |
| Otherwise                                                | Option 2: a draft pull request against `<base-branch>`              |

### Answers

- A yes to the proposed draft pull request, push, or refresh authorizes the push and the pull request write for this
  branch. A yes to a stack offer authorizes `link` and the pushes and draft pull requests the offer named.
- A destination the user named earlier answers the step when the request named this branch, or was made during this gate
  run. State the proposal once as the record and continue into Step 12. A destination named for another branch, or in an
  earlier gate run, authorizes nothing here. A named push or pull request covers the plain draft path only; a stack
  offer still needs its own yes.
- Another destination named in reply (merge locally, keep, discard) runs that option. While converge, the Security hold
  or an audit not computed withholds merge and pull request, a merge or pull request gets that hold again in one line.
- Option 1 on a base that has a remote counterpart runs only on the user's explicit choice, after one line that says the
  local base will then be ahead of its remote and that pushing the base is the user's step.
- "Not yet" or "stop" ends the run as [Stops](branch-workflow.md#stops) says.
- An unclear reply gets one clarifying line, never a list of options.

## Step 12: Execute the choice

When a change set gate did not author landed after the statuses were recorded, apply the re-run rule of Step 8 before
Option 1 merges or Option 2 pushes.

When `<compare>` is not `<base-sha>` and the chosen destination puts the lower pull requests' commits on `<base-branch>`
without stacking on them (Option 1, or an Option 2 draft against `<base-branch>`), set `<compare>` to `<base-sha>` and
run Steps 7 to 9 again before the merge or push. Their review, audit and convergence saw only this branch's layer, and
the delivery carries every layer below it. This run counts as the one automatic re-run of
[Rules for every step](branch-workflow.md#rules-for-every-step); its findings go through Step 11 again.

### Option 1: Merge locally

Before the merge, run [Find the destination](#find-the-destination) from the feature worktree and report the folder: the
merge moves the shell to the base, where `flow where` finds nothing for this branch. Commit the `bind` row, when
`flow bind` wrote one, on the feature branch as a one-path commit, so the merge carries it.

Find the checkout that holds `<base-branch>` in `git worktree list --porcelain`. When none does and the feature branch
is in the only checkout, switch that checkout to the base. When neither applies, ask the user which checkout to use.
Then merge, reading both names from session storage:

```bash
BASE_BRANCH=$(cat '<scratch>/gate-base')
FEATURE_BRANCH=$(cat '<scratch>/gate-branch')
git -C '<base-checkout>' switch "$BASE_BRANCH"
git -C '<base-checkout>' merge --no-ff "$FEATURE_BRANCH"
```

```powershell
$BaseBranch = (Get-Content -Raw -LiteralPath '<scratch>/gate-base').TrimEnd("`r", "`n")
$FeatureBranch = (Get-Content -Raw -LiteralPath '<scratch>/gate-branch').TrimEnd("`r", "`n")
git -C '<base-checkout>' switch $BaseBranch
git -C '<base-checkout>' merge --no-ff $FeatureBranch
```

- **The switch fails** (uncommitted changes in that checkout): report git's message and stop before the merge. The rows
  are written as for Option 3.
- **The merge stops on conflicts:** run `git -C '<base-checkout>' merge --abort`, report the conflicting paths, and
  write the rows as for Option 3, with `why` naming the failed merge and `next` `gate`. Resolving is `git resolve`'s
  job.
- **The merge succeeds:** run the verification command on the merged result. The merged tree holds the feature branch's
  code, so the command runs without asking only when the base is the default branch or otherwise the user's under
  [Running it](../../../routing/references/repo-conventions.md#running-it), and Step 7 ran the command on the feature
  branch or the user approved the run. Otherwise apply Running it to the feature branch's code first. When it does not
  authorize the run, stop as on a failure: report the result unverified with the command that would have run, keep the
  branch and the worktree, write the verdict `pending` with `next` `gate` and no `done`, and do not continue to Step 13
  or Step 14. On a failure, report it, keep the branch and the worktree, and write the verdict `failed` with `next`
  `execute` and no `done`. On a pass, continue to Step 13, then Step 14.

### Option 2: Push and create a draft PR

A yes to the proposal, or a destination named as [Answers](#answers) allows, authorizes this option. A push reached any
other way asks first, as [push.md](../../../git/references/push.md#authorization) says.

**Before anything leaves the machine.** Read the repository's visibility, then apply the
[outbound text rule](../../SKILL.md#cross-cutting-rules-all-four-modes) to every text this option sends: the body, the
evidence, and each finding sentence.

**The push.** Run git's push workflow for this branch: [push.md](../../../git/references/push.md#before-the-push) and
its checks, the resolved push remote, and the verification after the push. Never force-push from this option. When the
branch already has an open pull request, the same workflow pushes to it.

**The pull request.** Create it, or refresh the open one, by `gh`'s
[Pull request creation workflow](../../../gh/SKILL.md#pull-request-creation-workflow), loaded in this session. That
workflow owns the draft rule, issue linkage, the merge-settings read, the template, the body contract, and the
[repository policy](../../../gh/SKILL.md#repository-policy) on drafts and merging. The yes to the proposal is the
authorization for the pull request, so the workflow asks no second confirmation for it, and still resolves linkage as it
always does. On a host or plan with no draft pull requests, the workflow hands the user the create command.

**The evidence gate passes to `gh`.** The before and after are Step 7's command and result on the branch, and a before
exists only when the same command ran on the base in this session; otherwise the body says no before was run. Beyond
that, pass only the decisions a reviewer must weigh, each as one plain sentence:

- a check that failed or did not run, with its cause and who accepted the risk
- each substitute check that stands in for a failing test, with the task it covers
- a Security lane left incomplete, as the count of unsettled candidates and nothing about where they are
- each accepted or deferred finding, with the reason or the issue link
- each recorded user-authorized divergence, as its `Divergence:` line
- each Security finding of severity Minor, unless the visibility rule above withholds it

The status lines stay out of the body in every form. The small-fix description lines are the description's own fields
and go in.

Then write the rows (Step 14), whose commit is pushed to the same branch inside the same authorization, keep the
worktree, and arm the one-shot check-back by [watch.md](watch.md) when this run created the pull request or moved its
head. A run that pushed nothing arms nothing. A yes to a stack offer runs the
[stack path](stack-handoff.md#stacking-on-an-open-pr) in place of the push and the creation above.

### Option 3: Keep as-is

Report the branch and the worktree path. Write the rows (Step 14). The worktree stays.

### Option 4: Discard

Discarding is a destructive step, held as a safety gate. Show what will be lost and confirm once, at this point, even
when the discard was named up front:

```text
This deletes branch <name>, its <N> commits not on <base-branch> (<short shas and subjects>), the worktree at <path>,
<the uncommitted paths, or "no uncommitted changes">, and the ignored files under its temp/ folders. <Pushed: the branch
also exists on <remote>, and that copy stays | Not pushed: no other copy exists>.
```

The reply must name the action or the branch in the user's own words; a bare "yes" does not confirm it. After a bare
yes, ask once more for a reply that names the discard or the branch. On anything else, nothing is deleted and the run
ends as Option 3.

On a confirmation, run [Find the destination](#find-the-destination) from the feature worktree first and report the
folder. Run no `flow bind` and commit nothing on the feature branch: a row there is deleted with the branch. Then move
to a checkout that does not hold the feature branch, and run Step 13 and Step 14.

## Step 13: Clean up the worktree

This step runs for Option 1 after a merge that passed, and for Option 4 after its confirmation. The commands run from a
checkout other than the feature worktree.

**Option 1 keeps what the tidy moved.** When Step 4 printed `moved:` lines, list the feature worktree's
`<session-folder>/temp/` and compare the names with the last component of each `moved:` destination by reading them. A
listed name means an ignored file the removal would delete without a warning: keep the worktree and the branch, report
the path and each entry, and say that the user can move them out and remove the worktree. Uncommitted paths that Step 3
left out stop the removal the same way, because git refuses it. Step 14 still runs.

Otherwise remove the worktree and delete the branch. Run only the lines for the chosen option. A failed removal is
reported and never retried with `--force`; Option 4 passes `--force` from the start, because the user confirmed the
loss. When the branch has no linked worktree, skip the removal.

```bash
FEATURE_BRANCH=$(cat '<scratch>/gate-branch')
FEATURE_WORKTREE=$(cat '<scratch>/gate-worktree')
# Option 1
git worktree remove "$FEATURE_WORKTREE"
git branch -d "$FEATURE_BRANCH"
# Option 4
git worktree remove --force "$FEATURE_WORKTREE"
git branch -D "$FEATURE_BRANCH"
```

```powershell
$FeatureBranch = (Get-Content -Raw -LiteralPath '<scratch>/gate-branch').TrimEnd("`r", "`n")
$FeatureWorktree = (Get-Content -Raw -LiteralPath '<scratch>/gate-worktree').TrimEnd("`r", "`n")
# Option 1
git worktree remove $FeatureWorktree
git branch -d $FeatureBranch
# Option 4
git worktree remove --force $FeatureWorktree
git branch -D $FeatureBranch
```

When the branch deletion fails, report it, keep the branch, and write no `done` row. On Options 2 and 3 the worktree
stays; a feature worktree that outlives its run is removed later by `git worktrees prune`.

## Step 14: Write the trail rows

Every run that resolved a flow, or wrote to `<work-root>/unassigned`, reaches this step, including the stops. The cells,
the folder rule, and the commit and push table are in [../emission.md](../emission.md). The rows of this step are, in
order:

1. the verdict row
2. on a stop, the `pause` row
3. on Option 1 after the branch was deleted, and on Option 4 after the branch was deleted, the `done` row, last

The `waive`, `commit`, `approve` and `escape` rows were written when they happened, so a run that stops early keeps
them.

### Find the destination

The folder is the one [Where the rows go](../emission.md#where-the-rows-go) resolves, including the fallback to
`<work-root>/unassigned` after a discard. Options 1 and 4 resolve it from the feature worktree before the merge or the
discard.

### Write the rows

```bash
FEATURE_BRANCH=$(cat '<scratch>/gate-branch')
agx flow log gate branch "$FEATURE_BRANCH" <status> "<one line: what the verdict rests on>" "<PR number, or none>" "<next>" --folder '<folder>'
```

```powershell
$FeatureBranch = (Get-Content -Raw -LiteralPath '<scratch>/gate-branch').TrimEnd("`r", "`n")
agx flow log gate branch $FeatureBranch <status> '<one line: what the verdict rests on>' '<PR number, or none>' '<next>' --folder '<folder>'
```

The `pause` and `done` rows take the same form with the event, status and cells that emission.md gives.

### Land the rows

Stage the `state.tsv` and land it as
[Stage and commit what the run wrote](../emission.md#stage-and-commit-what-the-run-wrote) says for the option. Report
the commit's SHA, and report a failed commit or push.

Remove the session files at the end, for every option. The command is the same in both shells:

```bash
rm -f -- '<scratch>/gate-branch' '<scratch>/gate-worktree' '<scratch>/gate-base' '<scratch>/gate-push-remote'
```

Remove each `<scratch>/reconcile-<artifact>` copy that Step 5 made the same way, naming each file.
