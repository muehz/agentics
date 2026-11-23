# Parallel lanes and native dispatch

This file holds the rules that the `Parallel lanes` and `Native dispatch by surface` sections of
[`SKILL.md`](../SKILL.md) point to.

## Parallel lanes

A plan whose slices carry `**Lane:** yes` has lanes: slices the user runs in separate sessions at the same time, each on
its own branch. For such a plan, execute writes a launch packet per lane and prints how to start each one. It runs no
lane in this session unless invoked on that lane, and it never starts a lane session itself.

**Start point.** Do these steps in order:

1. Apply [The work-item branch](../../git/references/branch.md#the-work-item-branch) first: on a detached HEAD or a
   branch on the resolver's `protected` line, create the feature branch with no question, so no start-point commit lands
   on a protected branch. Step 1 sub-step 6 then finds it in place.
2. Settle **The union rule** below, before you write the packets. When the user accepts the union line, commit
   `.gitattributes` alone at once, in a commit that holds only that line.
3. Step 1 sub-step 6's artifact-only commit of the plan and `state.tsv` follows it and is the plan commit. When that
   step had nothing to commit, or the work folder is local and never committed, HEAD is the plan commit. Packets are
   written after it and before sub-step 8 dispatches Stage 1 reviewers.

Neither commit holds code, and each holds one kind of artifact, so gate's controller audit reads both as artifact-only
commits. Each lane branch starts at the plan commit, so the branch has the approved plan and the merge rule in its
history.

**Merged lanes.** A lane merges into the pull request base, the branch the plan targets, not into this session's branch.
Resolve the base branch and its remote with the [resolver](../../git/references/base-and-remote.md) and use its
`base-ref` line; never assume `origin`. Before you write packets, skip every lane that has already merged. Read the
lane's state by the first row that matches:

1. A `note lane-S<n>` row with status `ok` records the merge.
2. The lane is an ancestor of the base. Read the lane's fully qualified `lane_ref` and the resolved base into variables
   in this call. Resolve the base to `base_oid` with `git rev-parse --verify --end-of-options` and the `^{commit}`
   suffix. `git merge-base --is-ancestor "$lane_ref" "$base_oid"` exits 0. For a lane branch without a local ref, read
   its fully qualified remote-tracking ref into `lane_ref` instead.
3. The lane's pull request is merged. In the call that reads the lane's short branch name into `lane_branch`, run
   `gh pr view --json state -- "$lane_branch"`. The returned state is `MERGED`.
4. The base holds the lane's `verify <task-id>` rows with status `ok`. In the call that reads the repository-relative
   `work_folder` and validated `base_oid`, run `git show "${base_oid}:$work_folder/state.tsv"` to read those rows.
5. Otherwise, the lane has not merged.

When rows 2 to 4 find a merge that no `note lane-S<n>` row records, write one with evidence
`merged=<sha of the lane's tip>`, so the next run reads row 1. The row records the merge without touching the approved
plan. For each merged lane, say `S<n> merged, no packet`, and treat it as merged for its dependents. A slice whose lanes
have all merged runs in this session (**What this session runs**). A lane is merged in the base but not in HEAD when
either of these holds:

- `git merge-base --is-ancestor <lane-branch> HEAD` exits 1.
- Both of these hold:
    - The lane counts as merged by its pull request state or by the base branch's `state.tsv`.
    - HEAD's `state.tsv` lacks the lane's `verify` rows with status `ok`.

For such a lane, do these steps:

1. Say so.
2. Tell the user to merge the base into the session branch before its dependents run.

The dependents build on the lane's code and its `state.tsv` rows. The union rule applies in that local merge.

**Launch packets.** Write one file per lane at `temp/lanes/S<n>.md` inside the work folder, in its ignored `temp/`
directory. Each packet holds, in this order:

1. the lane's `## S<n>` section, copied verbatim from the plan with every task under it;
2. the standing-orders register, verbatim, under `## Standing Orders` (`None recorded.` when it is empty);
3. the open divergence entries, verbatim, under `## Open Divergences` (`None recorded.` when there are none);
4. the verification commands:
    - each of the lane's task commands, with its directory and shell from the **Verification preflight**;
    - the plan's Outcome proof, when the lane holds a skeleton-path task;
5. the branch, with these parts:
    - its name;
    - the plan commit's SHA (the last start-point commit, which carries the `.gitattributes` commit in its history);
    - the instruction to create the branch there in a new worktree beside this checkout;

    The name is this session's branch name with `-s<n>` appended (**Start point** has already moved off the default
    branch). When that branch already carries a slice-ID token (`s<n>` as decompose's branch rule bounds it), the name
    is `feat/<slug>-s<n>` from the work folder's slug instead. The name never appends a second token to the session
    branch, so the slice ID appears once, as decompose's branch rule requires (for example `feat/reports-s2`).

6. the instruction: "In that worktree, use the `agx` helper (see [flow-state.md](flow-commands.md#commands) for the
   fallback): run `agx flow bind --folder <work folder> --slice S<n>` before your first row, so this branch binds to the
   shared flow with its slice. Write out `<work folder>` in the packet. Pass `--folder <work folder>` on every
   `flow status` and `flow log` call of the lane run too. Read `$LASTEXITCODE` after each call in PowerShell. Then run
   `/agentics:execute auto <plan path> S<n>`. Execute only the tasks under `## S<n>`. Close out through `gate` to a pull
   request and stop there. Do not start another slice."

**Launch lines.** Then print, for each lane, one launch line per host. Use the packet's absolute path, because the
lane's new worktree does not hold this checkout's ignored `temp/`:

- Claude Code: `claude --bg 'Read <absolute packet path> and follow it.'`, or the same prompt in a new session from
  agent view (`claude agents`);
- Codex: `codex 'Read <absolute packet path> and follow it.'`;
- Copilot: `copilot -i 'Read <absolute packet path> and follow it.'`, or the same prompt in a new Copilot app session.

Before each lane's launch lines, print the worktree procedure. Give the branch name and sibling worktree path as data.
In the shell call that reads them into `lane_branch` and `lane_dir`, also read the validated plan commit id into
`plan_oid`, then run `git worktree add -b "$lane_branch" -- "$lane_dir" "$plan_oid"`. When the branch already exists,
verify `refs/heads/$lane_branch` first, then run `git worktree add -- "$lane_dir" "$lane_branch"` to attach the existing
local branch. Keep the short name as the final argument after `--` so Git attaches the branch. When a worktree already
holds the branch, print its path and the launch lines. Tell the user to launch from inside that worktree. A session
launched from this checkout must move into the lane worktree before `/agentics:execute`. The ways to move are:

- Claude Code: its worktree tool, or a new agent-view session opened in that directory.
- Codex and Copilot: `cd`.

Single-quote every argument built from a branch name or path in each printed shell command. Single quotes keep a name
that holds shell syntax from running as a command. These launches are user-only. Print them, but never run them. Never
report a lane as started. A launch line names the packet and nothing else.

**What this session runs.** Execute dispatches no task of a lane slice here. Slices that are not lanes run through Steps
1 and 2 as usual. A slice that depends on an unmerged lane is not run this session. Say so in the run report. The user
re-invokes execute on the plan after that lane merges. A slice whose lanes have all merged (**Merged lanes**) runs in
this session. When nothing is left for this session, the run ends after the launch lines.

**Running one lane.** Invoked with a slice ID (`/agentics:execute auto <plan path> S<n>`), execute runs one lane. It
runs only the tasks under that slice, through the same Steps 0 to 2. `S<n>` composes with any mode argument. The packet
names `auto`, and a user who types `pair` gets `mode=pair`, which covers this lane branch only. The lane run writes its
own `enter run` row, with `evidence` `mode=<mode> branch=<lane branch>`, as its first row after
`agx flow bind --slice S<n>`. The plan's existing approval covers the run (Step 0 still requires `approvals:` to list
`plan.md` as `current`), so it writes no second `approve plan.md` row. A lane is not an isolated packet. Its
implementers run in the lane session's own worktree and record the shared-worktree **Commit evidence** form. A packet's
draft-pull-request instruction is a close-out named in advance under **Safety gates**. A lane run applies none of this
section. It writes no packets, asks no union question, and makes no start-point commit.

**The union rule.** Every lane appends rows to the same `state.tsv`. Lane branches conflict on it at merge unless Git
merges that file by union. Check with `git check-attr merge -- <work folder>/state.tsv`. If the output ends in
`merge: union`, the rule is present. If the trail already holds a `waive union-rule` row, the user declined it. If
either holds, skip the question. Otherwise, before you write any packet, ask one question in the
[question format](../../routing/references/contracts/question-format.md). It offers to add
`<work-root>/**/state.tsv merge=union` to the root `.gitattributes`, created if absent. Recommend yes with ➡️, so each
lane's rows merge without a conflict.

On a yes, append the line only when `git status --porcelain -- .gitattributes` prints nothing. Then commit it on its
own, as **Start point** says. When the file has uncommitted changes, do these steps:

1. Say so.
2. Write no packet.
3. Stop until the user commits or discards the changes, or answers no.

A no answer writes the `waive union-rule` row below. On a no, do these steps:

1. Write nothing to `.gitattributes`.
2. Log `waive union-rule` with status `ok` and `why` set to the user's reason, or `none given`.
3. Say that each lane merged after the first will conflict on `state.tsv`.

After a yes or a no, tell the user these points:

- GitHub's merge button and merge API do not apply union merge drivers.
- A pull request whose only conflict is in `state.tsv` is resolved by a local merge of the base branch into the lane
  branch.
- The union rule applies in that local merge.
- A push follows that merge.

The merge never needs a re-sort: a reader orders rows by `ts`, counts identical rows once, and gives file order no
meaning ([flow-state.md](flow-state.md#what-a-reader-sees)).

**Reviews per lane.** Independent lanes merge in any order, each as its own pull request. Each lane session runs its own
group reviews and closes out through its own `gate`, which judges only that slice. The home session runs no `gate` on a
lane. Gate's stack hand-off applies only to a later slice that depends on a lane, never to the lanes themselves. When
the plan has two or more mutually independent groups, the final integration review belongs to the home session, on a
branch that has merged the base after every lane merged ([aggregate-wave-review.md](aggregate-wave-review.md)). The home
session runs it when it is next invoked on the plan with every lane merged.

## Native dispatch by surface

Pick the path by **which tools this session has**, not by product name. Full mechanics, limits, and per-surface gotchas
are in [surface dispatch](../../routing/references/surface-dispatch.md).

Read its **tool-presence index** first. It maps what is in your session to the dispatch pattern it supports, and it is
how you detect features in practice.

`auto` and `arena` cannot run without dispatch machinery, because they dispatch every implementer. When no dispatch
machinery is present, do these steps:

1. Implement nothing.
2. Stop.
3. Tell the user that `pair` is the only mode that runs on this host. State it as a fact, not a recommendation.

Never select `pair` yourself. Never start it until the user chooses it. In a `pair` run that the user chose on such a
host, each Stage 1, quality, and aggregate group review runs in this session. Each review is a separate pass that reads
only its review packet. Every boundary report says the reviews ran in session, rather than through reviewer subagents.

**Some capabilities are user-invoked and never model-invoked.** Recommend them when they fit, but never claim to have
run one. The adapter marks these per surface under **User-only**. It also records where the escalation of a large board
requires the user's explicit multi-agent opt-in. A user-typed skill invocation counts. Silent auto-selection of this
skill does not.

### Platform notes

Dispatch examples use **bash** syntax. The workflow itself is shell-agnostic. On Windows, adapt invocations to
**PowerShell** equivalents, or run agents inside **WSL** for full bash parity. In PowerShell, `&` and `|` are different
operators. `$env:VAR` replaces `$VAR`. Forward slashes work natively. Backslashes appear only in regex/JSON contexts.
