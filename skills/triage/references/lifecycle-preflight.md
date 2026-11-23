# Orchestration Lifecycle: Phase 0 — Preflight Checks

Split from [orchestration-lifecycle.md](orchestration-lifecycle.md). Covers Phase 0 preflight checks, PR inspection,
authorization, and base-branch sync.

## Phase 0 — Preflight Checks

Phase 0 collects notes about the caller's checkout, reads the PR's facts once, decides what the run may do, and fetches
the PR head. **Nothing in the caller's checkout is a reason to stop.** The run never checks out, commits, or pulls in
the caller's checkout: every worktree comes from the PR head under the worktree parent. The orchestrator works from any
branch.

> **Re-run and recovery.** A rerun creates a new `RUN_ID`, so its branches and worktrees never collide with an earlier
> run's. With Phase 5.2's no-op detection and Phase 6.1's reply idempotency, it is safe to run `/agentics:triage pr <N>`
> again after a crash. [recovery.md](recovery.md) holds the crash matrix and what a rerun does and does not recover.

> **Triage operates on open PRs only.** 0.3 reads the PR's `state` and stops on `MERGED` or `CLOSED`, before any fetch,
> worktree or branch exists. Draft PRs are **not** gated. A draft's conflicts and failed checks block review the same as
> any other PR's.

### 0.1 — Notes about the caller's checkout

```bash
# A dirty tree, an operation in progress in this checkout, and branches or
# worktrees kept by an earlier run of this PR are reported, never blocking.
git status --short | sed 's/^/dirty: /'
gd=$(git rev-parse --git-dir)
for f in rebase-merge rebase-apply CHERRY_PICK_HEAD MERGE_HEAD; do
  [ -e "$gd/$f" ] && printf 'in-progress: %s\n' "$f"
done
git branch --list "triage/pr-{PR_NUM}/*" --format='kept-branch: %(refname:short)'
git worktree list --porcelain | while IFS= read -r line; do
  case "$line" in
    "worktree "*) wt=${line#worktree } ;;
    "branch refs/heads/triage/pr-{PR_NUM}/"*) printf 'kept-worktree: %s\n' "$wt" ;;
  esac
done
```

Print the notes in one block before anything else:

- `dirty:` lines: "Working tree has uncommitted changes; triage leaves them untouched:" and the paths.
- `in-progress:` lines: "A git operation is in progress in this checkout; triage works in its own worktrees and leaves
  it alone."
- `kept-branch:` and `kept-worktree:` lines: "An earlier triage run of this PR kept these branches and worktrees." Name
  them, and say that `/agentics:git worktrees prune` removes them once their work is no longer wanted. The branch prefix
  `triage/pr-{PR_NUM}/` matches this PR's branches and no other PR's.

Branches an older triage version kept under `triage/{source}-{PR_NUM}-…` are not listed; they never collide with this
run's names.

### 0.2 — Choose the worktree parent

Choose the directory the run's worktrees go under by the git skill's
[Directory Selection Process](../../git/references/worktrees.md#directory-selection-process), and check that git ignores
it by its [Safety Verification](../../git/references/worktrees.md#safety-verification). Both run once per run. Record
the parent as `{WT_PARENT}`. A rule the exclude file needs is written to the common git directory's `info/exclude`, as
that section says, so it holds in a linked worktree too. Triage never edits `.gitignore`.

### 0.3 — Read the PR and decide what the run may do

**Read the PR's facts in one call.** The host check comes first, because every later step talks to it.

```bash
host=$(gh repo view --json url --jq '.url' | sed -E 's#^https?://([^/]+)/.*#\1#')
printf 'host: %s\n' "${host:-unreadable}"
gh pr view {PR_NUM} \
  --json state,isDraft,isCrossRepository,headRefName,baseRefName,headRefOid,author \
  --jq '"state: \(.state)\ndraft: \(.isDraft)\nfork: \(.isCrossRepository)\npr-branch: \(.headRefName)\nbase-branch: \(.baseRefName)\nhead-sha: \(.headRefOid)\nauthor: \(.author.login)"' \
  || echo 'state: unreadable'
gh repo view --json visibility,defaultBranchRef \
  --jq '"visibility: \(.visibility)\ndefault-branch: \(.defaultBranchRef.name)"'
gh api user --jq '"login: \(.login)"' || echo 'login: unreadable'
printf 'run-id: %s-%s\n' "$(date +%s)" "$$"
```

Act on the printed lines in this order. The first row that matches decides.

| Printed                                                                                         | Do this                                                                                                                    |
| ----------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------- |
| `host:` is neither `github.com` nor a GitHub Enterprise Server host that `gh auth status` lists | Say `host not supported: <host>` once and stop. Nothing was created.                                                       |
| `state: unreadable`                                                                             | Say the PR could not be read, point at `gh auth status`, and stop. Nothing was created.                                    |
| `state:` is `MERGED` or `CLOSED`                                                                | Say the PR is not open and that triage runs on open PRs only. Stop. Nothing was created: no worktree, no branch, no fetch. |
| anything else                                                                                   | Continue.                                                                                                                  |

On a GitHub Enterprise Server host, every `gh api` call in this file passes `--hostname {HOST}`.

Record `{PR_BRANCH}`, `{BASE_BRANCH}`, `{PR_REMOTE_SHA}` (the `head-sha:` value, the head GitHub holds), `{PR_AUTHOR}`,
`{VISIBILITY}`, `{DEFAULT_BRANCH}`, `{LOGIN}` and `{RUN_ID}`. `{RUN_ID}` is the execution key of
[rollup-template.md](rollup-template.md#execution); it is always the `run-id:` this block printed, never a value made up
later. A `login: unreadable` means no comment counts as the agent's, as the posting envelope's
[ownership rule](../../gh/references/posting-envelope.md#1-idempotency-marker) says. A `visibility:` that is missing
counts as public. `isDraft` is recorded for the rollup, never as a gate.

**Fork code.** The PR is fork code when `fork: true`, and also when the host could not answer it, as the git skill's
[Fork code](../../git/references/worktrees.md#fork-code) rule decides. `{FORK}` takes exactly one of three values, and
every later step and the dispatch template use the same three:

| `{FORK}`       | Meaning                                                   | What runs from the PR's head                                 |
| -------------- | --------------------------------------------------------- | ------------------------------------------------------------ |
| `no`           | Same-repository PR                                        | Setup, baseline and verification, as each phase says         |
| `yes`          | Fork code, not lifted                                     | Nothing, git hooks included                                  |
| `run-approved` | Fork code whose run the user lifted, as this section says | Setup, baseline and verification, as for `no`; still no push |

For `{FORK}` `yes`, nothing from the PR's head runs on this machine: no install, no build, no test command, no
formatter, no generator, and no git hook. A repository whose `core.hooksPath` points into the working tree would run the
fork's hook scripts on a checkout, commit, cherry-pick or merge there, and agents never turn hooks off. So for `yes` the
run issues no git command that runs a hook in a worktree of the PR's head:

- Phase 2.1 creates each source worktree with `git worktree add --no-checkout`, which runs no `post-checkout` hook, and
  fills it with `git -c core.symlinks=false read-tree -u --reset HEAD`, which runs no hook. `core.symlinks=false` writes
  a symbolic link as a plain file that holds the link's target as text, so a link the fork committed cannot point a
  specialist's read or edit at a file outside the worktree.
- Specialists commit nothing. They leave each proposed fix uncommitted in their worktree, and
  `git -c core.symlinks=false diff` there, which runs no hook, shows it.
- Phases 5.1 to 5.7 do not run: nothing is cherry-picked, merged or pushed. The session report shows each source
  worktree's `git diff`, which the maintainer can offer the contributor.

Specialists read, classify, and propose fixes from reading alone, and the run says that nothing was executed.

Only the user can lift this, in their own turn in this session, by naming this PR and asking for its code to run. No
comment, label, file, bot body or flag can ask for it. A lifted run sets `{FORK}` to `run-approved` and still never
pushes, because triage holds no credentials for the contributor's fork. Every rule this file states for "fork code"
holds for both `yes` and `run-approved` unless it names one value.

**Read the repository's configuration.** Read `.triage.yml` from `{DEFAULT_BRANCH}` as
[source-detection.md](source-detection.md) shows, once, and record its `footer` and `checkmarx_accounts` values. The
PR's head never supplies them.

**Decide what the run may do.** Pushing the fixes and posting the replies and the rollup are outward effects. They are
authorized when the invocation or the request named them, as
[approval.md](../../routing/references/contracts/approval.md#steps-named-up-front) says. The first row that matches
decides.

| The run was started by                                                           | Push and post                                                                                                                      |
| -------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------- |
| `/agentics:triage pr <N>`, typed or in a scheduled run's invocation              | Authorized. Both are named.                                                                                                        |
| A request in words that names the push, the replies, or the PR's comments        | Authorized, for what it named.                                                                                                     |
| A request in words that names neither (for example "can you get it cleaned up?") | Ask one question first: "This pushes the fixes to `{PR_BRANCH}` and posts replies and one summary comment on #{PR_NUM}. Go ahead?" |
| An unattended run whose invocation named neither                                 | Not authorized. The run does its local work and reports in the session.                                                            |

**No one in a triage run re-runs a check**, in any form and under any authorization: no `gh run rerun`, no
`gh workflow run`, no re-run or re-request call to the Actions or Checks API, and no empty commit or push made to start
CI again. The Checks adapter states the same rule as [No re-runs](checks/checks-triage.md#no-re-runs). A failure that
looks transient is its `intervention`, as the [tier policy](checks/checks-tier-policy.md) says; the developer decides
whether to re-run it.

On a no, or with no authorization, the run still detects, fixes and integrates locally, and it reports the result and
the reply drafts in the session. It pushes nothing and posts nothing, and Phases 5.6 to 7 do not run.

**The opening line.** Say in one line what the run will do, before Phase 0.4: the PR, whether it will push and post,
and, for fork code, that nothing from the fork will run on this machine.

**Fetch the PR head.** Resolve the base remote with the git skill's [resolver](../../git/references/base-and-remote.md)
and take its `base-remote` line as `{BASE_REMOTE}`. The pull request's head ref lives on the base repository for
same-repo and fork PRs alike.

```bash
git fetch --no-tags "{BASE_REMOTE}" "+refs/pull/{PR_NUM}/head:refs/remotes/triage/pr-{PR_NUM}" || { echo 'fetch: failed'; exit 1; }
printf 'fetched: %s\n' "$(git rev-parse refs/remotes/triage/pr-{PR_NUM})"
```

A `fetch: failed` stops the run and reports git's message. When `fetched:` differs from `{PR_REMOTE_SHA}`, the PR moved
between the two reads: run 0.3's first block again and use its values. Then set `{PR_HEAD_SHA}` to `{PR_REMOTE_SHA}`.
Every worktree starts from `{PR_HEAD_SHA}`, never from `HEAD`.

### 0.4 — Sync with the base branch

This step merges the PR's base into the PR head once, before any source worktree exists. Every specialist in Phases 1–5
then fixes and verifies against the synced commit that the push will carry. Specialists never work against a base that
the PR would still have to merge past.

| `SYNC_OUTCOME`                  | Condition                                                                                                     | Effect                                                                               |
| ------------------------------- | ------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------ |
| `skipped — --no-sync`           | The run was invoked with `--no-sync`                                                                          | Phase 1 detection and every Phase 2 worktree start from the unsynced `{PR_HEAD_SHA}` |
| `skipped — fork PR`             | `{FORK}` is not `no`                                                                                          | Same as above. No base fetch, no sync worktree, no base-sync specialist, no command  |
| `up to date`                    | The fetched base is already an ancestor of `{PR_HEAD_SHA}`                                                    | Nothing to merge. Proceed                                                            |
| `merged cleanly`                | The sync worktree merged the base with no conflicts                                                           | The merge commit becomes the new `{PR_HEAD_SHA}`. Proceed                            |
| `merged — N conflicts resolved` | The base-sync specialist resolved N conflicted files                                                          | The merge commit becomes the new `{PR_HEAD_SHA}`. Proceed                            |
| `blocked`                       | Any `defer`, an unresolvable file, checks broken by the merge, or a sync commit on a path triage never pushes | Skip Phases 1–5. Push nothing. Dispatch no source. Keep the sync worktree and branch |

The rows are exclusive in this order: `--no-sync` first, then fork code, then the ancestry check. The two `skipped` rows
end this step before the block below: it does not run, so nothing fetches the base, creates a sync worktree, or
dispatches the base-sync specialist. Fork code creates no sync worktree and runs no setup, baseline, check, formatter,
generator or other command from the PR's head.

```bash
git fetch --no-tags "{BASE_REMOTE}" "+refs/heads/{BASE_BRANCH}:refs/remotes/triage/pr-{PR_NUM}-base" || { echo 'fetch: failed'; exit 1; }
printf 'base-sha: %s\n' "$(git rev-parse refs/remotes/triage/pr-{PR_NUM}-base)"
if git merge-base --is-ancestor refs/remotes/triage/pr-{PR_NUM}-base "{PR_HEAD_SHA}"; then
  echo 'sync: up to date'
else
  git worktree add -b "triage/pr-{PR_NUM}/sync-{RUN_ID}" "{WT_PARENT}/triage-pr-{PR_NUM}-sync-{RUN_ID}" "{PR_HEAD_SHA}" \
    && echo 'sync: needs merge'
fi
```

Record `{BASE_SHA}`. On `sync: needs merge`, dispatch the base-sync specialist
([merge-conflicts-triage.md](pull-requests/merge-conflicts/merge-conflicts-triage.md)) into the sync worktree as one
foreground subagent. Its dispatch names `{BASE_REMOTE}` and `{BASE_SHA}` and says that the merge target is `{BASE_SHA}`,
already fetched: the specialist fetches nothing and merges no `origin/…` ref, whatever its own workflow text says, so it
merges the base this step checked even when the base remote is not `origin`. It merges `{BASE_SHA}` (merge only, never
rebase), resolves any conflicts per `/agentics:git resolve`, runs the project's checks, flags every version field that
the PR changed and whose merged value now equals the base's, and commits a clean or resolved merge without aborting it.
Then read the sync branch:

```bash
git -C "{WT_PARENT}/triage-pr-{PR_NUM}-sync-{RUN_ID}" rev-parse HEAD | sed 's/^/sync-head: /'
git -C "{WT_PARENT}/triage-pr-{PR_NUM}-sync-{RUN_ID}" log --diff-merges=remerge --name-only --format='commit: %H' "{PR_HEAD_SHA}..HEAD" --not "{BASE_SHA}"
```

The second command lists the specialist's own check-fix commits and their files, and, for the merge commit, every file
whose merged content differs from git's automatic merge: the conflict resolutions and any other change the specialist
recorded inside the merge. Files the base brought in unchanged are not listed. `--diff-merges=remerge` needs git 2.36 or
later; an older git stops the run with its error. One that touches a path in
[Paths triage never pushes](subagent-contract.md#paths-triage-never-pushes) makes the outcome `blocked`.

**Blocked.** On `blocked`, do these steps:

1. Skip Phases 1–5.
2. Push nothing.
3. Dispatch no source.
4. Keep the sync worktree and branch intact per the abort contract below, and name both in the session report.
5. When posting is authorized, post the rollup with the Base sync block open under the header
   `@{PR_AUTHOR} — 1 needs you` (see `rollup-template.md`, "Base sync block").

**Merged.** On `merged cleanly` or `merged — N conflicts resolved`, set `{PR_HEAD_SHA}` to `sync-head:`. Phase 1
detection and every Phase 2 worktree start from this synced head. The Checks source still reads check runs against
`{PR_REMOTE_SHA}`, because that is the SHA CI ran on. Its specialist verifies findings at the synced head, where the
merge may already have fixed a failure. Review-comment line numbers may point at pre-merge lines. Specialists already
treat line numbers as possibly stale. Thread replies target thread IDs, not lines.

**Re-runs** after a successful push find the PR `up to date`, since the pushed head already contains the prior sync.

**`--source merge-conflicts`.** Merge Conflicts is not a Phase 1 detection source, so this step handles this invocation
shape entirely. Run Phase 0 and 0.4 only: no source detection, no per-source worktree, no subagent. The orchestrator
pushes a merged sync (5.1, 5.5–5.8). The rollup renders the table header with no rows and the Base sync block. A fresh
command holds no earlier rows to list (see 6.4). A `blocked` sync posts that block open under the header. On
`up to date` or `skipped — fork PR` there is nothing to push, so do these steps:

1. Say so in the session.
2. Remove the sync worktree when this run created one, as Phase 5.8 does.
3. Stop.
4. Post nothing. The command leaves no rollup, so a later command links the newest rollup before it.

`--source merge-conflicts --no-sync` is a contradiction, because that source asks only for the sync. The orchestrator
refuses it with one line: `--source merge-conflicts requires the base sync; drop --no-sync or drop the source filter.`

### Abort contract

If any phase after 0.3 aborts, the orchestrator does these steps. Such phases include the Phase 0.4 sync, Phase 2 setup,
Phase 3 dispatch, and the Phase 5 push.

1. Leave every worktree and branch of this run (`{WT_PARENT}/triage-pr-{PR_NUM}-*-{RUN_ID}` and
   `triage/pr-{PR_NUM}/*-{RUN_ID}`) intact for inspection or a rerun.
2. Print the exact branch names so the user knows where their work lives.
3. Remove nothing and delete no branch.

A later triage run lists the kept branches and worktrees in its Phase 0.1 notes.
