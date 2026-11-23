# Auto-detect: which mode does a bare `/agentics:gate` need?

When the user runs `/agentics:gate` with no argument, infer the mode from git state. This file is the only statement of
the rules. An explicit mode or alias (`branch`, `pr`, `feature`, `release`, `deploy`, `deployment`, `change`, `cab`,
`cr`, `check`, `validate`, `verify`) skips detection. A readiness question is not a bare invocation: `SKILL.md` Step 0
answers it.

## Step 1: Gather the signals

Only git state counts: the branch, its position against the base, a tag at `HEAD`, recent commit messages, and
`change-records/` paths. A file name or file content elsewhere in the repository (a `release-checklist.md`, say) is
never a signal; name it as context in the announcement if it seems relevant.

Run these read-only commands. Each is the same in both shells.

- **The branch.** `git branch --show-current`. Empty output means a detached `HEAD`.
- **Any commit at all.** `git rev-parse --verify -q HEAD`. Failure means a repository with no commits.
- **The base.** Run the resolver in [base-and-remote.md](../../git/references/base-and-remote.md#how-to-call-it) and
  take its `base-branch` and `base-ref` lines. The default branch is the base remote's default, which is the
  `base-branch` line when its source is `remote-default`. When the resolver reads `unresolved`, ask its `question` line.
  Never guess `main` or `master`.
- **Work on the branch.** Pass the resolver's `base-ref` as a separate argument with `^{commit}` appended to
  `git rev-parse --verify --end-of-options`. Retain the resulting commit ID as `base_oid`. Use
  `git rev-list --count "$base_oid..HEAD"` for commits ahead and `git status --porcelain` for uncommitted changes. When
  the ref cannot be resolved, the count is unavailable.
- **A release tag at `HEAD`.** `git tag --points-at HEAD`. A tag matches when its name matches `v[0-9]*`, `release-*`,
  `release/*`, or `rc-*`. Only a tag at `HEAD` counts; a tag on an earlier commit is never a release signal.
- **A change-record signal.** `git status --porcelain -- change-records/` prints a path, or a subject or body in
  `git log -5 --format=%s%n%b` contains `change-record`. A missing `change-records/` directory is not an error.

## Step 2: Apply the rules

Signals: **B** is "the current branch is not the default branch and has commits ahead or uncommitted changes". **T** is
"a release tag is at `HEAD`". **C** is "a change-record signal exists". The rules run in order and the first match wins,
so every combination of signals has exactly one row.

| Rule | Matches                       | Mode                                                                     |
| ---- | ----------------------------- | ------------------------------------------------------------------------ |
| 0    | the repository has no commits | none: say there is nothing to gate yet, and offer `/agentics:git commit` |
| 1    | B                             | `branch`                                                                 |
| 2    | T                             | `release`                                                                |
| 3    | C                             | suggest `change`, and wait for a yes                                     |
| 4    | otherwise                     | ask one question                                                         |

Rule 1 comes first because unmerged work on the branch is the immediate gate target, even when a release tag exists.
Uncommitted changes with no commits ahead still match rule 1, and branch mode's Step 3 handles them. Rule 2 matches on
any branch, a detached `HEAD` included, once rule 1 has not matched. Rule 3 is weak on purpose, because most change-mode
runs are explicit, so it suggests and waits instead of running.

When two or more tags match rule 2, take the one created most recently (`git tag --points-at HEAD --sort=-creatordate`)
and name the others in the announcement.

Rule 4's question:

> ❓ **Q1** - **Mode**: Gate a branch, a release, or a change?
>
> ➡️ The mode the request's wording points at (deploy or tag → `release`, CAB or change record → `change`); `branch`
> otherwise.

On a detached `HEAD` that matches no rule, the recommendation names the nearest branch or tag that `git describe --all`
reports.

## Step 3: Announce once, then run

Rules 1 and 2 run in the same turn. The announcement is the skill's one announce line for the run, so the mode's
workflow does not announce again. It names the signal, the mode, and how to switch:

```text
Detected <signal summary> → running gate in <mode> mode. Say `branch`, `release`, or `change` to switch.
```

Examples:

- `Detected branch feat/add-runbook with 3 commits ahead of main → running gate in branch mode. Say release or change to switch.`
- `Detected branch fix/typo with no commits ahead of main and 3 uncommitted files → running gate in branch mode. Say release or change to switch.`
- `Detected release tag v2.1.0 at HEAD → running gate in release mode. Say branch or change to switch.`

When a weaker rule also matched, add one line that names it, such as
`Also seen: an uncommitted change record at change-records/2026-05-10-payment.md. Say change to run change mode.`

Rule 3 announces its suggestion and waits for a yes. Rule 4 waits for the answer.

## When the user switches

A next message that says `branch`, `release` or `change` re-runs gate in that mode explicitly, and anything the inferred
mode already did is reported, not undone: a commit it made stays, and a row it wrote stays. Any other message is input
to the mode already running. Within the same conversation, a later bare `/agentics:gate` does not repeat an inference
the user overrode; it runs the mode the user chose.
