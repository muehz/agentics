# History Operations

## Overview

This file holds one workflow each for the `rebase`, `stash`, `cherry-pick` and `bisect` modes. Each workflow is a
numbered list of steps and ends with a report that states only what a command has just shown.

A command spelled the same in Bash and in PowerShell 7 appears once, in a `bash` block, and runs unchanged in either
shell. A command whose syntax differs has a Bash block and a PowerShell block. Where a block is marked as a Bash script,
save the block unchanged and pass its inputs as separate arguments through the command runner. When launching Git Bash
directly from a Windows process, set `MSYS=noglob` in the child environment so Git Bash leaves literal paths intact.
Inside a running Bash shell, pass paths by quoted expansion. Keep values you read from the repository or user out of
command text. Verify that a commit ID is hexadecimal or a stash ordinal is decimal before using it in another command.

## Rules for every workflow

- **No editor and no prompt.** No step opens an interactive editor or waits for typed input. Do not run `git rebase -i`,
  `git stash push -p`, `git add -p` or `git cherry-pick -e`. Where git would open an editor to confirm a message, the
  step sets `GIT_EDITOR` to `true` for that one command, and git keeps the message it already has. Passing
  `-c core.editor=true` is not enough, because an editor named in `GIT_EDITOR` outranks it.
- **Signing.** A rebase and a cherry-pick write commits, so the signing rule of
  [the commit contract](commit.md#the-commit-contract) applies: a commit that fails to sign stops the run once, with the
  exact command for the user to run.
- **One operation at a time.** Start no workflow while `git status` reports a merge, rebase, cherry-pick, revert or
  bisect in progress. Report what is in progress. Unmerged paths go to [merge-conflicts.md](merge-conflicts.md).
- **Clean tree.** Where a workflow asks for a clean tree, `git status --porcelain --untracked-files=no` prints nothing.
  When it prints paths, stop and report them. The user chooses whether to commit them through
  [the commit contract](commit.md#the-commit-contract) or to save them with [Stash](#stash). Do not let git stash them
  for you with `--autostash`, because that entry carries no message.
- **Destructive steps.** A step marked destructive runs only after one confirmation. First show the target and what will
  be lost, taken from the output of the commands in that step. Then ask once. The reply must name the action or its
  target in the user's own words. A request that already named them still gets this one confirmation, and gets no second
  one.
- **Abort.** Abort a stopped operation only when the user asks for the abort. An abort discards the resolution work, so
  it is a destructive step and gets the one confirmation described above.
- **No push.** None of these workflows pushes. A push is its own request and follows [push.md](push.md).

## Check session artifacts before replaying

Before a rebase or cherry-pick that can create commits, use read-only Git data to resolve the selected commits and the
paths and content the replay can change. Use the artifact identification rules in the
[session artifact check](commit.md#check-session-artifacts-before-committing), including titles at custom paths. If the
replay is known to write no session artifact, continue with the existing workflow.

When an automatic replay can write a session artifact, establish and check the exact resulting candidate bytes for every
such commit before a command that may create those commits. An original commit's blob or an earlier staged resolution
does not establish what a later replay will write. If any relevant candidate cannot be established or fails the check,
hold the command and give a sanitized reason. Preserve the current authorization, index, operation state and history. Do
not start or continue a rewrite to discover the candidate.

For a stopped operation, [Finish the operation](merge-conflicts.md#5-finish-the-operation) checks the current resolved
staged candidate. If its continue command may commit later replayed changes as well, check the remaining scope as
described above before continuing.

## Rebase

The rebase replays the commits of the current branch onto the resolved base, or onto the base the user named.

### 1. Check the tree and record the start

```bash
git status --short --branch
git status --porcelain --untracked-files=no
git rev-parse HEAD
```

The second command must print nothing: the rebase starts from a clean tree. Keep the commit that the third command
prints as `<start-commit>`. It is the branch as it was before the rebase.

### 2. Resolve the base

Run the resolver in [base-and-remote.md](base-and-remote.md). Put a base the user named on line 1. Read `base-branch`,
`base-remote` and `base-ref`. Ask the `question` lines for the base remote and the base branch in one round, as
[How to call it](base-and-remote.md#how-to-call-it) says, which also says when a pointer answers one. Then fill in the
answers and run the resolver again.

Take the first word of the `base-ref` line as data. In the same command that consumes it, pass it as one argument to
`git rev-parse --verify --end-of-options` with `^{commit}` appended. Keep the full hexadecimal commit ID as `base_oid`
in Bash or PowerShell. Use that ID in the steps below. Stop if it cannot be resolved.

- When `fetched` reads that the ref is absent locally, stop and report it.
- When `fetched` reads that a stale ref was used, continue and state in the report that the base may be behind its
  remote.

### 3. List the commits the rebase rewrites

```bash
git rev-list --count "HEAD..$base_oid"
git log --oneline "$base_oid..HEAD"
git log --oneline "$base_oid..HEAD" --not --remotes
```

- When the count is `0`, the branch already holds the base. Report that and stop.
- The first log lists the commits the rebase rewrites. The second lists those that no remote-tracking ref holds.
- A commit in the first list that is missing from the second is already pushed.

### 4. Confirm before rewriting pushed commits

Skip this step when step 3 found no pushed commit. Rewriting commits that are already pushed is a destructive step.

```bash
git branch -r --contains "<pushed-commit>"
```

Show the pushed commits from step 3 and the remote branches this command lists. Say what is lost: each of those commits
gets a new identity, the local branch no longer matches those remote branches, and anyone who has pulled them must
reconcile. Say that updating the remote branch afterwards needs `push force` under its own request. Then ask once.

### 5. Rebase

Run the [replay artifact check](#check-session-artifacts-before-replaying) for this rebase before starting it.

```bash
git rebase "$base_oid"
```

When the rebase stops on a conflict, follow [merge-conflicts.md](merge-conflicts.md). That file resolves the conflict
and finishes the rebase, so do not continue it a second time here. Its
[continue step](merge-conflicts.md#5-finish-the-operation) checks the staged candidate and remaining replay, then runs
without an editor in this form:

```bash
GIT_EDITOR=true git rebase --continue
```

```powershell
$editorBefore = $env:GIT_EDITOR
$env:GIT_EDITOR = 'true'
git rebase --continue
$env:GIT_EDITOR = $editorBefore
```

When the rebase stops again on a later commit, follow that file again, until git reports that the rebase is complete.

### 6. Verify and report

```bash
git status --short --branch
git merge-base --is-ancestor "$base_oid" HEAD && echo "base is an ancestor"
git log --oneline "$base_oid..HEAD"
```

```powershell
git status --short --branch
git merge-base --is-ancestor "$base_oid" HEAD
if ($LASTEXITCODE -eq 0) { 'base is an ancestor' }
git log --oneline "$base_oid..HEAD"
```

Report, from this output (report completion only after the proving commands have run; if they did not run, say so and
give the commands instead of claiming completion or inventing a SHA):

- the branch, the base it now sits on, and both the `source` and `fetched` fields the resolver printed for that base
- the commits now on the branch above the base
- `<start-commit>`, with the note that a new branch at that commit recovers the branch as it was
- when step 4 ran: that the remote branch still holds the old commits, and that updating it needs `push force` under its
  own request, described in [push.md](push.md#force-push)

When the status still shows a rebase in progress, report that instead and say which commit it stopped on.

## Stash

The stash stack is shared by every worktree of the repository, so the top entry may belong to other work. Every action
on an existing entry names that entry as the list shows it, for example `stash@{2}`. Do not run `git stash pop`,
`git stash apply` or `git stash drop` without an entry name.

### 1. Read the stack and the tree

```bash
git stash list
git status --short --branch
```

Each line of the list starts with the entry's name, followed by the branch it was saved on and its message. Find the
entry the request means by its message and branch. When more than one entry fits, show them and ask which one.

An entry's number changes whenever any worktree saves or drops an entry. Read the list again immediately before each
command that names an entry, and find the entry by its message.

### 2. Save

Every save carries a message that says what the work is, so the entry can be found again in the shared list. Write the
message as data to a scratch file. Keep its path as a separate argument. Save these scripts unchanged and pass the
arguments through the command runner's argument list.

When every changed and untracked path that step 1 listed belongs to the work, run this Bash script with the message file
as its one argument:

```bash
[ "$#" -eq 1 ] || exit 2
message=$(cat -- "$1") || exit 1
git stash push --include-untracked -m "$message"
```

#### Check and save a path-limited stash

When some paths belong to other work, take the exact repository-relative names of the paths to save. Use the user's
request or parse the NUL-delimited output of `git status --porcelain=v1 -z --no-renames --untracked-files=all`. Pass
each name as its own argument. The selected paths can include untracked files.

A path-limited stash records the whole index. Changes staged outside the selected paths can land in the stash and stay
staged in the tree. Run this Bash script with `check`, the message file path, and one or more selected paths. It prints
the staged paths outside the selection as NUL-separated repository-relative data. The positive `:(top)` pathspec keeps
the check repository-wide when run from a subdirectory.

```bash
[ "$#" -ge 3 ] || exit 2
action=$1
message_file=$2
shift 2
include_paths=() exclude_paths=()
for path in "$@"; do
  [ -n "$path" ] || exit 2
  include_paths+=(":(top,literal)$path")
  exclude_paths+=(":(top,exclude,literal)$path")
done

case "$action" in
  check)
    git diff --cached --name-only --no-renames --no-relative -z -- ':(top)' "${exclude_paths[@]}"
    ;;
  save)
    if ! git diff --cached --quiet --no-renames --no-relative -- ':(top)' "${exclude_paths[@]}"; then
      printf '%s\n' 'Cannot save: recheck staged paths outside the selection.' >&2
      exit 1
    fi
    message=$(cat -- "$message_file") || exit 1
    git stash push --include-untracked -m "$message" -- "${include_paths[@]}"
    ;;
  *) exit 2 ;;
esac
```

When the check succeeds and prints nothing, run the script again with `save` and the same other arguments. When it
prints paths, show those paths and ask once, as a ❓ question with these options: stash them too, unstage them first, or
stop. The ➡️ recommends stopping. To stash them too, add the specifically approved names to the selection and recheck.

#### Unstage a confirmed path outside the stash

Use this Bash script only after the user confirms the path to unstage. Pass that exact name from the check output as the
first argument, then the original stash selection as separate arguments. Repeat once per confirmed path. The script
reads a fresh staged list and stops if the confirmed path is no longer outside the selection. Recheck before saving.

```bash
[ "$#" -ge 2 ] && [ -n "$1" ] || exit 2
confirmed_path=$1
shift
exclude_paths=()
for path in "$@"; do
  [ -n "$path" ] || exit 2
  exclude_paths+=(":(top,exclude,literal)$path")
done

staged=$(mktemp) || exit 1
trap 'rm -f -- "$staged"' EXIT
git diff --cached --name-only --no-renames --no-relative -z -- ':(top)' "${exclude_paths[@]}" > "$staged" || exit 1
while IFS= read -r -d '' path; do
  if [ "$path" = "$confirmed_path" ]; then
    git restore --staged -- ":(top,literal)$path"
    exit $?
  fi
done < "$staged"
printf '%s\n' 'Cannot unstage: the confirmed path is absent from the staged paths outside the selection.' >&2
exit 1
```

When git answers that there are no local changes to save, nothing was saved. Report that.

### 3. Inspect or restore

Show what an entry holds:

```bash
git stash show --include-untracked --stat 'stash@{<n>}'
```

Restore an entry and keep it in the list:

```bash
git stash apply 'stash@{<n>}'
```

When the user asked to pop the entry, restore it with `pop`. Git removes the entry only after it applied cleanly, so
nothing is lost:

```bash
git stash pop 'stash@{<n>}'
```

When the restore stops on a conflict, follow [merge-conflicts.md](merge-conflicts.md). Git keeps the entry in the list
after a conflict. Removing it is step 4.

### 4. Drop one entry

Dropping an entry is a destructive step. Show the entry's line from the list and what it holds:

```bash
git stash list
git stash show --include-untracked --stat 'stash@{<n>}'
```

Say that the changes in those files exist nowhere else once the entry is gone. Ask once. Then read the list again and
drop the entry by its current name:

```bash
git stash list
git stash drop 'stash@{<n>}'
```

Keep the commit that git prints in its `Dropped` line for the report.

### 5. Clear the stack

Clearing the stack is a destructive step. It removes every entry, including entries saved from other worktrees.

```bash
git stash list
```

Show the whole list. Say that every entry in it is removed for every worktree. Ask once. Then clear:

```bash
git stash clear
```

### 6. Verify and report

```bash
git stash list
git status --short --branch
```

Report, from this output:

- after a save: the entry's name and message as the list shows them, and that the saved paths no longer show as changed
- after a restore: the paths that now show as changed, and whether the entry is still in the list
- after a drop: that the list no longer shows the entry, and the commit from git's `Dropped` line
- after a clear: that the list is empty

## Cherry-pick

The cherry-pick copies the named commits onto the current branch, in the order the user gave them.

### 1. Record the start

```bash
git status --short --branch
git rev-parse HEAD
```

Keep the commit that the second command prints as `<start-commit>`.

### 2. Show the commits

Show every named commit before any of them is applied:

```bash
git show --stat --date=short --format='%h %an %ad %s' "<commit>" "<other-commit>"
```

For a range, list its commits oldest first. The range leaves out `<first-commit>` itself:

```bash
git log --reverse --stat --date=short --format='%h %an %ad %s' "<first-commit>..<last-commit>"
```

Give this output to the user. When a name does not resolve to a commit, stop and report that name. Apply nothing.

### 3. Apply

Run the [replay artifact check](#check-session-artifacts-before-replaying) for all selected commits before applying
them.

```bash
git cherry-pick -x "<commit>" "<other-commit>"
```

A range is applied the same way:

```bash
git cherry-pick -x "<first-commit>..<last-commit>"
```

When the cherry-pick stops on a conflict, follow [merge-conflicts.md](merge-conflicts.md). That file resolves the
conflict and finishes the cherry-pick, so do not continue it a second time here. Its
[continue step](merge-conflicts.md#5-finish-the-operation) checks the staged candidate and remaining replay, then runs
without an editor in this form:

```bash
GIT_EDITOR=true git cherry-pick --continue
```

```powershell
$editorBefore = $env:GIT_EDITOR
$env:GIT_EDITOR = 'true'
git cherry-pick --continue
$env:GIT_EDITOR = $editorBefore
```

When the cherry-pick stops again on a later commit, follow that file again, until every named commit is applied.

### 4. Verify and report

```bash
git status --short --branch
git log --reverse --format="%h %s%n%b" "<start-commit>..HEAD"
```

The `-x` flag in step 3 adds a `(cherry picked from commit <sha>)` line to each new commit's message, so the log shows
each new commit with its source. Report, from this output:

- the branch and the new commits on it, each beside the commit it was copied from
- whether the number of new commits equals the number of commits named
- `<start-commit>`, the branch as it was before

When the status still shows a cherry-pick in progress, report that instead and say which commit it stopped on.

## Bisect

The bisect searches the commits between a known good commit and a bad commit for the first one that fails a test.

### 1. Check what the search needs

The search needs three things:

- **A known good commit.** The user names it. When none is named, ask for it. Do not guess one.
- **A bad commit.** It is the current commit unless the user named another.
- **A test.** It is a command that exits `0` on a good commit and with `1` to `127` on a bad one. Exit `125` marks a
  commit that cannot be tested, and git skips it. The test must leave tracked files unchanged.

Ask for everything that is missing in one round.

```bash
git status --short --branch
git status --porcelain --untracked-files=no
git branch --show-current
git rev-parse HEAD
```

The second command must print nothing: the bisect starts from a clean tree. Keep the branch that the third command
prints as the starting branch. When it prints nothing, `HEAD` is detached and the starting point is the commit that the
fourth command prints.

### 2. Start

```bash
git bisect start "<bad-commit>" "<good-commit>"
```

From this command on, step 5 always runs, including when a later step fails or is stopped.

### 3. Run the test at each step

```bash
git bisect run <test-command>
```

Git checks out one commit after another and runs the test on each. Git runs the test through its own shell on every
platform, so `<test-command>` is a program with its arguments, not a PowerShell expression.

When the test cannot run unattended, run it yourself on each commit git checks out, and mark the result with
`git bisect good`, `git bisect bad` or `git bisect skip` until git names the first bad commit.

### 4. Read the result

```bash
git bisect log
git show --stat --date=short --format='%H %an %ad %s' refs/bisect/bad
```

The log ends with a `first bad commit` line when the search finished, and the second command shows that commit. When
skipped commits kept git from naming one commit, the log lists the candidates that remain. Keep this output for the
report, because step 5 removes the bisect state.

### 5. Return to the starting branch

```bash
git bisect reset
git branch --show-current
git rev-parse HEAD
git status --short --branch
```

The branch must be the starting branch from step 1, or the starting commit when `HEAD` was detached. When it is not,
report where the checkout is and stop.

### 6. Report

Report, from the output of steps 4 and 5:

- the first bad commit, with its author, date, subject and changed files, or the remaining candidates when git named no
  single commit
- the good commit, the bad commit and the test that were used
- that the checkout is back on the starting branch at the starting commit

When the search did not finish, report the step it stopped at and what git printed, after step 5 has run.
