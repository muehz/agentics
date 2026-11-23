# Every git write has one owning skill, and chain work happens on a feature branch

This record decides who commits, creates branches, pushes, force-pushes, merges and removes worktrees, which of those
needs its own authorization, how the base branch and the remote are resolved, and where an ignore rule may be written.
The skill audit in `docs/designs/skill-audit-2026-11/consolidated-issues.md` found that these writes had several owners
or none: `git` refused every push, `gh` gave three answers on merging, the main chain never created a branch, and five
skills each had a different rule for `.gitignore`.

What counts as an authorization, how it is worded and how long it stands is decided in
[0014](0014-approval-and-pushback.md). This record only says which operation needs one of its own.

## Rules

### The git skill owns every push, and no authorization extends to a stronger operation

- **Answers:** C5.1
- **Rule:** Each git write has one owner, and each of the four classes below needs its own authorization. An
  authorization for one class never covers a later class in the list.
    - **Commit, branch and worktree creation** are local and belong to `git`. The request or the approved plan that
      produced the change authorizes them.
    - **Push** belongs to `git`, which gains a `push` mode. The mode covers the first push of a branch, a follow-up push
      to a branch that already has a pull request, a push with no pull request, and a tag push. It needs an
      authorization that names the push or the pull request. `gate branch` calls the mode for its draft-PR destination
      and no longer is the only route to a push. The sentence "Never push from this skill" is removed.
    - **Force-push** belongs to `git push` and needs an explicit request that names the operation and the branch. It
      always uses `--force-with-lease` pinned to the remote SHA that was read, and it is refused on the branches the
      base-branch rule below protects. No skill uses a force flag on its normal path. `triage` integrates by merge, so
      its push is a fast-forward and runs as a plain push, which already fails when the remote moved. When the pull
      request branch has diverged, triage merges the remote tip and never rebases or force-pushes; the user who wants a
      rewrite asks `git` for it.
    - **Merge.** Merging a pull request and marking it ready for review are repository policy, as
      [0014](0014-approval-and-pushback.md) decides: no agent does either while the hook settings leave the rules on, a
      request in the session does not switch them off, and the exits are the settings change and the user's own click.
      `gh` is the only skill that states the rule, it states it once, and its other two sentences on merging are
      removed. A local merge into the base branch belongs to `gate branch`. It needs an explicit choice of that
      destination, and gate offers it only when the base branch has no remote counterpart, because a merge into a branch
      that tracks a remote can only be delivered by a push to the base that nothing authorized. Merging the base into a
      feature branch is an ordinary commit under the first class. (verified: skills/git/SKILL.md:143,
      skills/gh/SKILL.md:21, skills/gh/SKILL.md:117, skills/gh/SKILL.md:206, skills/triage/references/pr-mode.md:226,
      skills/triage/references/orchestration-lifecycle.md:884, 2026-11-11)
- **Default / Rejected:** `git` owns the push, because the force-push rules already live in its safety protocol and a
  push works the same on every host; rejected: keeping the push inside `gate branch`, which makes a follow-up push cost
  a full gate run and leaves the tag push, the push with no pull request and every force-push with no owner. Rejected:
  giving the push to `gh`, which would tie a plain git operation to GitHub.
- **Later work changes:** `skills/git/SKILL.md`, `skills/git/references/guide.md`, `skills/gh/SKILL.md`,
  `skills/gh/references/stack.md`, `skills/gate/references/branch/branch-workflow.md`,
  `skills/triage/references/pr-mode.md`, `skills/triage/references/orchestration-lifecycle.md`, `evals/skills/git.md`,
  `evals/skills/gh.md`, `evals/skills/triage.md`

### The first commit of a work item creates its feature branch, through a branch mode in git

- **Answers:** C5.2
- **Rule:** No chain work is committed on the default branch or on a detached HEAD. The skill that is about to make the
  first commit for a work item checks `git branch --show-current`, and when it is on the default branch or detached it
  creates a feature branch first, with no question. It builds the name from the work folder or the issue in the
  repository's branch naming, and it announces the name so that the user can rename it. This applies to the whole chain,
  to `execute fix`, to parallel lanes and to `hillclimb`, whose per-win commits had no branch rule. `git` gains a
  `branch` mode that creates, switches and renames a branch, so the callers that say "create the branch through `git`"
  name a mode that exists. Deleting a branch stays under git's destructive-operation rule. `flow bind` keeps refusing
  the default branch. (verified: skills/execute/references/fix-entry.md:25, skills/git/SKILL.md:7,
  hooks/go/internal/flow/bind.go:32, 2026-11-11)
- **Default / Rejected:** create the branch without asking, because a new branch is local, costs nothing and is undone
  by a rename or a switch; rejected: the question `execute fix` asks today, which adds a stop to every run for a
  decision almost nobody changes. Rejected: sending these callers to `git worktrees`, which makes a second checkout when
  only a branch was wanted.
- **Later work changes:** `skills/git/SKILL.md`, `skills/git/references/guide.md`, `skills/execute/SKILL.md`,
  `skills/execute/references/fix-entry.md`, `skills/execute/references/parallel-lanes.md`, `skills/intent/SKILL.md`,
  `skills/hillclimb/SKILL.md`, `skills/scaffold/SKILL.md`, `skills/routing/references/chain-phases.md`

### The base branch and the remote are resolved from the repository, never from literal names

- **Answers:** C5.3
- **Rule:** Three resolutions replace the literal names `main`, `master` and `origin`.
    - **Base branch.** The first of these that exists: a base the user named or one already recorded for the work item;
      the base of the branch's open pull request; the default branch of the base remote, read from that remote's `HEAD`
      ref. When none exists, the skill asks one question and does not guess. The result is stated by name and compared
      as the remote-tracking ref after a fetch, so a stale local copy of the base is never the comparison point. A
      branch stacked on another pull request therefore resolves to that pull request's branch.
    - **Remote.** A push goes to the branch's configured push remote, then to its upstream's remote, then to the only
      remote when there is one. The base remote is the repository the work is delivered to, which in a fork clone is the
      parent and not the fork. With several remotes and nothing configured, the skill asks.
    - **Force-push protection.** A force-push is refused on the default branch of every remote, on the resolved base
      branch, and on a branch the host reports as protected. (verified:
      skills/gate/references/branch/branch-workflow.md:66, skills/gate/references/branch/branch-workflow.md:1647,
      skills/git/SKILL.md:127, 2026-11-11)
- **Default / Rejected:** read the answer from the pull request, the configuration and the remote, and ask only when
  they are silent; rejected: a longer list of names to try (`develop`, `trunk`), which still picks the wrong base on a
  backport or in a git-flow repository and still never names what it picked.
- **Later work changes:** `skills/gate/references/branch/branch-workflow.md`,
  `skills/gate/references/branch/stack-handoff.md`, `skills/git/SKILL.md`, `skills/git/references/worktrees.md`,
  `skills/triage/references/orchestration-lifecycle.md`, `skills/gh/SKILL.md`

### One commit contract for every skill, and chain artifacts are committed when approved

- **Answers:** C5.4
- **Rule:** Four things hold for commits.
    - **Chain artifacts.** An intent, a spec, a plan or the middle path's `brief.md` of
      [0013](0013-proportional-delivery-paths.md) is committed by the skill that wrote it, in the commit that carries
      its approval row, on the feature branch. The terminal probe that gated this commit is removed, because it reports
      a non-interactive session under every agent harness and so the artifacts were only ever staged. An artifact that
      is not approved stays uncommitted. Under the `local` storage setting that [0017](0017-flow-state-v2.md) decides,
      the artifact and its approval row are saved in the clone-local store and no commit is made for them.
    - **One contract.** A skill may run `git commit` itself, but only under the contract in
      `skills/git/references/commit.md`: the repository's commit convention, a scope check, and explicitly named paths.
      A bare `git commit` and `git add -A` over the whole tree are never used. Skills link the contract and do not
      restate it.
    - **No unrequested commits.** A commit needs a request or an approved plan that produced the change. A skill makes
      no commit for its own tooling setup.
    - **Process-only commits.** A change that only records process (trail rows, a work-folder tidy, a verdict) goes into
      the commit of the work it records. When no such commit is left to carry it, one skill run makes at most one
      bookkeeping commit on the branch. (verified: skills/decompose/SKILL.md:126,
      skills/decompose/references/save-and-stage.md:103, skills/git/references/worktrees.md:184,
      skills/hillclimb/SKILL.md:128, skills/ops/references/github-publishing.md:94, 2026-11-11)
- **Default / Rejected:** a shared contract that any skill applies in place; rejected: routing every commit through an
  invocation of the `git` skill, which adds a skill load to each task commit in `execute` and to each win in
  `hillclimb`. Rejected: keeping the terminal probe so that "a commit needs a person", since the approval is that
  person's decision and the probe never sees them.
- **Later work changes:** `skills/git/references/commit.md`, `skills/decompose/SKILL.md`,
  `skills/decompose/references/save-and-stage.md`, `skills/decompose/references/chain-artifacts.md`,
  `skills/intent/SKILL.md`, `skills/spec/SKILL.md`, `skills/gh/SKILL.md`, `skills/ops/references/github-publishing.md`,
  `skills/triage/references/orchestration-lifecycle.md`, `skills/hillclimb/SKILL.md`,
  `skills/gate/references/branch/branch-workflow.md`

### The skill that creates a worktree removes it, and tooling never edits `.gitignore`

- **Answers:** C5.5
- **Rule:** Three things hold for what a skill leaves in the repository.
    - **Worktrees.** The skill that creates a worktree removes it. `execute` removes an isolated-packet or arena
      worktree once its result is integrated or dropped. `gate branch` removes the feature worktree on a local merge or
      a discard. A feature worktree that outlives its run, which is the normal case after a draft pull request, is
      removed by `git worktrees prune`, and a skill that finds one whose branch has merged offers the prune. A removal
      is forced only for a worktree the user discarded; a failed removal is reported and never retried with `--force`.
    - **Ignore rules.** A path that exists only for Agentics tooling (a worktree parent, a work folder's `temp/`, a
      decision log) is ignored through the repository's local exclude file. No skill appends to a tracked `.gitignore`
      on its own or commits one. `.gitignore` is edited by `scaffold`, with its preview and confirmation, or on the
      user's explicit request. When the exclude file cannot be written, the skill reports it and leaves the path
      untracked.
    - **Storage.** The ban on git metadata as an artifact store stands, except for the stores
      [0017](0017-flow-state-v2.md) names: its approval ledger and the clone-local store that holds the work folder
      under the `local` storage setting. Git's own mechanisms used for their purpose (the exclude file, worktree
      administration) are not artifact stores. A file a skill invents inside the git directory is one, so gate's
      recorded branch and base names move to session storage. (verified:
      skills/execute/references/isolated-packets.md:86, skills/gate/references/branch/branch-workflow.md:398,
      skills/git/references/worktrees.md:182, skills/git/references/worktrees.md:468,
      skills/triage/references/orchestration-lifecycle.md:499, skills/hillclimb/SKILL.md:100,
      skills/routing/references/session-storage.md:37, skills/gate/references/branch/branch-workflow.md:114, 2026-11-11)
- **Default / Rejected:** the rule `triage` already follows, the local exclude file, for all five skills; rejected:
  appending to `.gitignore` and committing it, which puts an Agentics-only line in a shared file, adds a commit nobody
  asked for, and is reported by gate as an unaccounted commit on the same branch. Rejected: making `gate` the one
  cleanup owner, since on the default destination gate ends before the worktree can go.
- **Later work changes:** `skills/git/references/worktrees.md`, `skills/gate/references/branch/branch-workflow.md`,
  `skills/gate/references/branch/stack-handoff.md`, `skills/execute/references/isolated-packets.md`,
  `skills/hillclimb/SKILL.md`, `skills/triage/references/orchestration-lifecycle.md`,
  `skills/routing/references/session-storage.md`, `skills/scaffold/references/init.md`

### Hooks and signing are never bypassed; a rejected red commit is replaced by recorded evidence

- **Answers:** C5.6
- **Rule:** No commit skips a hook or disables signing, and neither rule gets an exception for the red commit.
    - **Red commit.** The red commit of a bug fix is attempted as it is today. When a pre-commit hook rejects it because
      the new test fails, the commit is not made. The test and the fix then land in one commit, and the evidence of red
      is the recorded failing run of that test against the tree before the fix. Review and gate accept that record in
      place of the commit and do not report a missing red commit. The record is written in the row that would have named
      the red commit: `verify red-<task-id>` for a plan task and `verify red-fix` on the fix entry. The evidence of
      those rows is `commit=<sha> patch-id=<id>` today, and it gains a no-commit form that holds the failing run's
      command, its exit status and an excerpt of its output.
    - **Signing.** Before the first commit of a run, the skill reads `commit.gpgsign`. A commit that fails to sign, or
      that waits on a passphrase prompt the session cannot answer, stops the run once with the exact command for the
      user to run in their own terminal. The agent never retries unsigned, never passes `--no-gpg-sign`, and never
      changes signing configuration. The same holds for `tag.gpgsign` and a tag. (verified: skills/tdd/SKILL.md:177,
      skills/git/SKILL.md:128, skills/git/references/guide.md:230, 2026-11-11)
- **Default / Rejected:** keep hooks and signing absolute and change what counts as red evidence; rejected: letting the
  red commit pass `--no-verify`, because one flag skips every hook and the hook that runs the tests cannot be skipped
  apart from the one that scans for secrets. Rejected: asking the user to make each red commit by hand, which stops
  every bug fix in a repository that tests on commit.
- **Later work changes:** `skills/tdd/SKILL.md`, `skills/execute/references/fix-entry.md`,
  `skills/git/references/commit.md`, `skills/git/SKILL.md`, `skills/review/references/check-weakening.md`,
  `skills/gate/references/branch/branch-workflow.md`, `hooks/go/internal/flow/`,
  `skills/execute/references/flow-state.md`

## Existing records

- 0005: consistent — release publication stays a maintainer's act, and the push mode gives no skill a release or tag
  push without its own authorization.
- 0006: consistent — this record does not touch release promotion, which 0006 decided and 0010 has since superseded.
- 0007: consistent — the version is still computed from squash subjects, and the single commit contract keeps the
  repository's commit convention.
- 0008: consistent — the approvals it requires are unchanged; this record adds only that an approved artifact is
  committed with its approval row.
- 0009: amended — a bug fix commits its failing test on its own unless a pre-commit hook rejects that commit, in which
  case the recorded failing run stands in for it.
- 0010: consistent — a dispatched release is pushed by the workflow, not by a skill.
- 0012: consistent — `flow bind` in the hook binary still refuses the default branch, which the feature-branch rule
  relies on.

## What this costs

**The git skill is no longer local-only.** It gains a `push` and a `branch` mode. Its description, its routing rows and
its eval bank say today that it never pushes, and all three change.

**A branch appears without a question.** A user who wanted to work on the default branch finds a new branch and has to
say so. The announcement is the only notice.

**The local merge is withdrawn where the base tracks a remote.** That is nearly every repository, so `gate branch`
offers three destinations there instead of four.

**Triage loses the force-push.** A pull request branch that needs a rewrite is no longer recovered inside triage; the
run merges, or stops and names `git push`.

**Ignore rules are per clone.** A line in the local exclude file is not shared, so each clone gets it the first time a
skill needs it, and a teammate without Agentics sees a tooling directory as untracked if one is ever copied to them.

**Some bug fixes have no red commit.** In a repository whose hooks run tests, the proof that the test failed first is a
recorded run and not a commit a reviewer can check out.

**A signing failure stops the run.** In a session that cannot answer a passphrase prompt, every run ends at its first
commit until the user fixes the signing setup or commits by hand.

## Considered and declined

**Ask the host for every protected branch before any push.** Declined as the only source. The query needs a host and
credentials that a plain git remote does not have, so the default branch and the resolved base are protected from the
repository alone, and the host's answer is added when it can be read.

**Commit chain artifacts as soon as they are written.** Declined. An unapproved intent or plan in history reads as
decided to anyone who finds it, and the approval hash is taken from the file the commit then carries.

**Mark the red test as an expected failure so that the hook passes.** Declined. The commit would then hold a test edited
to pass, and the fix would have to edit the test again, which review reports as a change to red-committed test files.
