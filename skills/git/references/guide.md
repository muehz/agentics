# Git guidance

General git guidance by topic. This reference advises; it performs nothing. The `branch`, `push`, `rebase`, `stash`,
`cherry-pick`, `bisect` and `resolve` modes perform the operations, following [`branch.md`](branch.md),
[`push.md`](push.md) and [`history-operations.md`](history-operations.md). The commands below illustrate the advice.
Resolve the remote and branch using the relevant mode, then read their values into variables in each shell call that
uses them.

## Branching strategies

### Trunk-based development

All developers commit to `main` (or a short-lived feature branch that merges within a day or two). Best for strong CI,
frequent deploys, and feature flags.

### GitHub Flow

The flow has three steps: create a feature branch from `main`, open a PR, and merge after review. There are no
long-lived release branches.

### GitFlow

Long-lived `develop` and `main` branches with `feature/`, `release/`, and `hotfix/` branches. Best for scheduled
releases with multiple supported versions.

### Branch naming conventions

```text
feature/<short-description>     # New feature work
bugfix/<short-description>      # Bug fix on development branch
hotfix/<short-description>      # Urgent fix on production
release/<version>               # Release preparation
chore/<short-description>       # Maintenance, tooling, CI
```

Keep branch names lowercase and hyphen-separated. When a ticket number is available, include it (e.g.,
`feature/PROJ-123-add-auth`).

---

## Rebase vs. merge

### When to rebase

- **Local feature branches** before you open a PR, for a clean linear history
- **Updates to a feature branch** with upstream changes when the branch is not shared
- **Interactive rebase** (`git rebase -i`) to squash WIP commits before review

### When to merge

- **Shared branches** where others work, so merge preserves context
- **Main/develop integration**, where merge commits mark clear integration points
- **When history accuracy matters** more than cleanliness

To merge the base into the branch, run `/agentics:git resolve <base>`.

### Golden rule

**Never rebase branches that others have pulled or work on.** A rebase rewrites commit SHAs and forces everyone else to
reconcile diverged history.

### Interactive rebase for cleanup

```bash
# Rebase last N commits interactively
git rebase -i HEAD~N

# Common operations in the editor:
# pick   - keep commit as-is
# squash - combine with previous commit
# fixup  - combine with previous, discard this message
# reword - keep commit, edit message
# drop   - remove commit entirely
```

**Agent note:** `git rebase -i` opens an interactive editor that agent harnesses cannot drive, so it hangs or fails when
an agent runs it. Hand the command to the user.

---

## Stash management

### Basic operations

```bash
# Stash working changes with a descriptive message
git stash push -m "WIP: auth refactor"

# List all stashes
git stash list

# Show what's in a stash
git stash show -p 'stash@{0}'

# Apply a stash and keep it in the stash list
git stash apply 'stash@{0}'

# Apply a stash and remove it from the stash list
git stash pop 'stash@{0}'

# Apply an older stash
git stash apply 'stash@{2}'

# Remove a specific stash
git stash drop 'stash@{1}'

# Clear all stashes
git stash clear
```

Address each entry by the name `git stash list` shows for it; the rule is in
[`history-operations.md`](history-operations.md#stash).

### Partial stashes

```bash
# Stash only specific files
git stash push -m "just these files" -- path/to/file1 path/to/file2
```

### Stash vs. WIP commit

| Approach   | Pros                                    | Cons                                       |
| ---------- | --------------------------------------- | ------------------------------------------ |
| `stash`    | Invisible to history, quick             | Easy to forget, can conflict on apply      |
| WIP commit | Visible in log, can be pushed to remote | Needs cleanup before merge (squash/rebase) |

**Guideline:** Use stash for quick context switches (minutes to hours). Use WIP commits for longer interruptions or to
share the work.

---

## Cherry-pick workflows

### Basic cherry-pick

```bash
# Cherry-pick a single commit
git cherry-pick <commit-sha>

# Cherry-pick without committing (stage changes only)
git cherry-pick --no-commit <commit-sha>

# Cherry-pick a range of commits
git cherry-pick <start-sha>..<end-sha>
```

### Resolving conflicts during cherry-pick

```bash
# After resolving conflicts in the files:
git add <resolved-files>
git cherry-pick --continue

# Abort the cherry-pick entirely
git cherry-pick --abort
```

The `resolve` mode finishes a stopped cherry-pick; see [merge-conflicts.md](merge-conflicts.md).

### When to cherry-pick

- **Hotfixes:** apply a specific fix from `develop` to a `release` or `main` branch
- **Backports:** port a fix to an older supported version
- **Selective migration:** move specific commits between diverged branches

### When NOT to cherry-pick

- When you need the full branch history, use merge or rebase instead.
- When the commit depends on prior commits not in the target branch, it can conflict or produce broken code.

---

## Bisect for debugging

`git bisect` binary-searches commit history to find which commit introduced a bug.

### Manual bisect

```bash
# Start bisecting
git bisect start

# Mark the current (broken) commit as bad
git bisect bad

# Mark a known-good commit
git bisect good <commit-sha>

# Git checks out a middle commit. Test it, then:
git bisect good   # if this commit works
git bisect bad    # if this commit is broken

# Repeat until git identifies the first bad commit

# When done, return to your branch
git bisect reset
```

### Automated bisect

```bash
# Run a test script automatically at each step
git bisect start
git bisect bad HEAD
git bisect good <commit-sha>
git bisect run <test-script>

# Example with a test command
git bisect run npm test
git bisect run pytest tests/test_auth.py
```

### Tips

- `git bisect skip` when a commit can't be tested (won't build, irrelevant)
- The test script should exit 0 for good, non-zero for bad
- Bisect works best with small, atomic commits

---

## Tag management

### Lightweight vs. annotated tags

```bash
# Lightweight tag - just a pointer to a commit
git tag v1.0.0

# Annotated tag - includes tagger, date, and message (recommended for releases)
git tag -a v1.0.0 -m "Release 1.0.0"

# Tag a specific commit
git tag -a v1.0.0 <commit-sha> -m "Release 1.0.0"
```

**Use annotated tags for releases.** They carry metadata (tagger, date, message) and can be GPG-signed.

### Working with tags

```bash
# List all tags
git tag -l

# List tags matching a pattern
git tag -l "v1.*"

# Show tag details
git show v1.0.0

# Delete a local tag
git tag -d v1.0.0
```

Use [`push.md`](push.md) to push one tag, push all tags on request, or delete a tag from a remote. It resolves the push
remote and reads the tag name as data.

### Semantic versioning with tags

Follow `vMAJOR.MINOR.PATCH`:

- **MAJOR**: breaking changes
- **MINOR**: new features, backwards compatible
- **PATCH**: bug fixes, backwards compatible

---

## Undoing changes safely

### Working tree changes (not staged)

```bash
# Discard changes to a specific file
git restore path/to/file

# Discard all working tree changes
git restore .
```

### Staged changes (not committed)

```bash
# Unstage a file (keep working tree changes)
git restore --staged path/to/file

# Unstage everything
git restore --staged .
```

### Committed changes

```bash
# Revert a commit (creates a new "undo" commit, safe for shared history)
git revert <commit-sha>

# Revert without auto-committing
git revert --no-commit <commit-sha>
```

### Reset (local branches only)

```bash
# Soft reset - move HEAD, keep changes staged
git reset --soft HEAD~1

# Mixed reset (default) - move HEAD, unstage changes, keep in working tree
git reset HEAD~1
```

**Do not discard work with a hard reset.** It destroys uncommitted changes with no way back. To drop working tree
changes, use `git restore`; to set them aside so they can come back, follow the
[stash workflow](history-operations.md#stash), which reads the message into a quoted variable.

### The reflog safety net

View recent `HEAD` movements with `git reflog`. Choose the commit, verify its full ID as `reflog_oid`, and read the new
branch's name into `recovery_branch` in the same call. Then run `git switch -c "$recovery_branch" "$reflog_oid"` to
recover the commit on that branch.

The reflog keeps references to recent commits for ~90 days, even after resets or branch deletions, as the last recovery
tool.

---

## Log and history

### Useful log formats

```bash
# Compact one-line log
git log --oneline

# Graph view with branch topology
git log --oneline --graph --all

# Log with dates and authors
git log --format="%h %ad %an: %s" --date=short

# Commits in a date range
git log --after="2024-01-01" --before="2024-02-01"

# Commits by a specific author
git log --author="name"
```

### Investigating specific changes

```bash
# Show who last modified each line
git blame path/to/file

# Follow a file through renames
git log --follow path/to/file

# Search commit messages
git log --grep="fix auth"

# Search for changes to a string in code
git log -S "functionName" --oneline

# Show full diff for a specific commit
git show <commit-sha>
```

---

## Mode quick reference

| Mode                                                                    | Use it to                                                               | Takes in, produces                                                                                                                                         | Detail                                                                                                           |
| ----------------------------------------------------------------------- | ----------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------- |
| `commit`                                                                | Make one focused commit in the repository's commit convention.          | In: the request and a non-empty diff. Out: one commit, with its SHA, message, and files reported.                                                          | [/agentics:git commit](../SKILL.md#agenticsgit-commit); [commit.md](commit.md)                                   |
| `commit batch`                                                          | Split outstanding changes into multiple logical commits.                | In: uncommitted changes. Out: a commit plan, one commit per group, and a summary table.                                                                    | [/agentics:git commit](../SKILL.md#agenticsgit-commit); [commit.md](commit.md)                                   |
| `branch [<name> [start-point] \| rename <new> \| delete <name>]`        | Create, switch to, rename, or delete a branch.                          | In: a branch name, and an optional start point. Out: the branch state, and whether the branch was created, switched to, renamed, or deleted.               | [/agentics:git branch](../SKILL.md#agenticsgit-branch); [branch.md](branch.md)                                   |
| `push [<branch>] \| tag <name> \| delete <branch> \| delete tag <name>` | Send a branch or one tag to the push remote, or delete one remote ref.  | In: an authorized push request. Out: the remote ref, verified equal to the local SHA, or the deleted ref.                                                  | [/agentics:git push](../SKILL.md#agenticsgit-push); [push.md](push.md)                                           |
| `push force [<branch>]`                                                 | Replace a branch's remote history after one confirmation.               | In: a named force-push of a branch outside the protected set. Out: a push pinned by `--force-with-lease` to the SHA shown.                                 | [/agentics:git push](../SKILL.md#agenticsgit-push); [push.md](push.md)                                           |
| `worktrees [<branch> [start-point [pr <number>]]]`                      | Create an isolated workspace for parallel branch development.           | In: an optional branch and start point. Out: a worktree, and a report of its path, whether the branch was created or reused, and the baseline test result. | [/agentics:git worktrees](../SKILL.md#agenticsgit-worktrees); [worktrees.md](worktrees.md)                       |
| `worktrees prune`                                                       | Remove merged or abandoned worktrees, with safety gates.                | In: the linked worktrees. Out: the confirmed set removed, branch refs kept, and a report.                                                                  | [/agentics:git worktrees](../SKILL.md#agenticsgit-worktrees); [worktrees.md](worktrees.md)                       |
| `resolve [<pull-request> \| <base>]`                                    | Resolve a conflict, starting the merge itself when none is open.        | In: a stopped operation with unmerged paths, or a pull request or base to merge. Out: a resolved tree, checked by the project's checks unless fork code.   | [/agentics:git resolve](../SKILL.md#agenticsgit-resolve); [merge-conflicts.md](merge-conflicts.md)               |
| `rebase [<base>]`                                                       | Rebase the current branch onto the resolved or named base.              | In: a clean tree and a base. Out: the rebased branch and its new commits.                                                                                  | [/agentics:git rebase](../SKILL.md#agenticsgit-rebase); [history-operations.md](history-operations.md)           |
| `stash [push \| list \| show \| apply \| pop \| drop]`                  | Save and restore uncommitted work.                                      | In: uncommitted work or a stash entry. Out: the entry saved, shown, applied, or dropped, named as the list shows it.                                       | [/agentics:git stash](../SKILL.md#agenticsgit-stash); [history-operations.md](history-operations.md)             |
| `cherry-pick <commit>...`                                               | Apply named commits to the current branch.                              | In: one or more commits. Out: the commits shown first, then applied, and the new commits reported.                                                         | [/agentics:git cherry-pick](../SKILL.md#agenticsgit-cherry-pick); [history-operations.md](history-operations.md) |
| `bisect <good> [<bad>]`                                                 | Find the first bad commit.                                              | In: a known good commit, an optional bad one, a clean tree, and a test. Out: the first bad commit, and the starting branch checked out again.              | [/agentics:git bisect](../SKILL.md#agenticsgit-bisect); [history-operations.md](history-operations.md)           |
| `guide [topic]`                                                         | Get advice on branching strategies, rebase or merge, tags, and undoing. | In: an optional topic. Out: the matching guide section, or an answer from general git knowledge with the Git Safety Protocol applied.                      | [/agentics:git guide](../SKILL.md#agenticsgit-guide); [guide.md](guide.md)                                       |
