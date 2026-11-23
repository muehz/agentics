# Branches

## Overview

This file holds the workflows of `branch` mode and the work-item branch rule that other skills link to.

| Request                       | Section                                       |
| ----------------------------- | --------------------------------------------- |
| `branch <name> [start-point]` | [Create and switch](#create-and-switch)       |
| `branch rename <new>`         | [Rename](#rename)                             |
| `branch delete <name>`        | [Delete](#delete)                             |
| A skill about to commit       | [The work-item branch](#the-work-item-branch) |

Every workflow here acts on local branches only. A branch on a remote is changed through [push.md](push.md).

Use the examples for your shell. Put a requested branch name or start point in the work item's scratch files with a
file-writing tool, one value per file and a final newline. Pass their paths as `AGX_BRANCH_FILE` and `AGX_START_FILE`
through structured environment input. Alternatively read the files from fixed, trusted scratch paths. Read the data
again in each shell call that uses it, and stop when a required read fails. The only direct substitutions below are the
generated work-item branch names covered by that step's character rule.

## Create and switch

`branch <name> [start-point]` creates the branch and switches to it. When a local branch of that name already exists, it
switches to that branch and creates nothing.

### 1. Create or switch

Leave the start point empty when the request gave none. The new branch then starts at the current commit.

```bash
name=$(cat -- "$AGX_BRANCH_FILE") || exit 1
start=
if [ -n "${AGX_START_FILE:-}" ]; then start=$(cat -- "$AGX_START_FILE") || exit 1; fi
git check-ref-format "refs/heads/$name" || exit 1
if git show-ref --verify --quiet "refs/heads/$name"; then
  git switch -- "$name" && echo "switched to existing branch $name"
elif [ -n "$start" ]; then
  git switch -c "$name" -- "$start" && echo "created $name at $start"
else
  git switch -c "$name" && echo "created $name at the current commit"
fi
```

```powershell
$name = (Get-Content -Raw -LiteralPath $env:AGX_BRANCH_FILE -ErrorAction Stop).TrimEnd("`r", "`n")
$start = if ($env:AGX_START_FILE) { (Get-Content -Raw -LiteralPath $env:AGX_START_FILE -ErrorAction Stop).TrimEnd("`r", "`n") } else { '' }
git check-ref-format "refs/heads/$name"
if ($LASTEXITCODE -ne 0) { return }
git show-ref --verify --quiet "refs/heads/$name"
if ($LASTEXITCODE -eq 0) {
  git switch -- "$name"
  if ($LASTEXITCODE -eq 0) { "switched to existing branch $name" }
} elseif ($start) {
  git switch -c "$name" -- "$start"
  if ($LASTEXITCODE -eq 0) { "created $name at $start" }
} else {
  git switch -c "$name"
  if ($LASTEXITCODE -eq 0) { "created $name at the current commit" }
}
```

When git refuses the name or the start point, report git's message and stop.

### 2. When uncommitted changes block the switch

Git refuses a switch that would overwrite uncommitted changes, and it names the files. Do these steps:

1. Report git's message as git printed it.
2. Ask as a ❓ question whether to commit the changes or to stash them. The ➡️ recommends the commit when the changes
   belong to the branch you are leaving, and the stash when they do not.
3. Follow [the commit contract](commit.md#the-commit-contract) for a commit, or the
   [stash workflow](history-operations.md#stash) for a stash. Then run step 1 again.

Discard nothing to make the switch succeed. Do not add `--discard-changes`, `--force` or `--merge` to the switch, and do
not run `git restore`, `git clean` or `git checkout` over the changed files.

### 3. Verify and report

```bash
git branch --show-current
```

Report the branch only when this prints the requested name. Say which happened: the branch was created, and at which
commit, or the branch already existed and git switched to it. When the branch already existed and the request gave a
start point, say that the start point was not used.

## Rename

`branch rename <new>` renames the current branch. The rename is local.

### 1. Read the current branch

```bash
old=$(git branch --show-current)
printf 'current branch: %s\n' "${old:-none (detached HEAD)}"
```

```powershell
$old = "$(git branch --show-current)"
if ($old) { "current branch: $old" } else { 'current branch: none (detached HEAD)' }
```

On a detached HEAD there is no branch to rename. Report that and stop.

### 2. Rename

```bash
new=$(cat -- "$AGX_BRANCH_FILE") || exit 1
git check-ref-format "refs/heads/$new" && git branch -m -- "$new"
```

```powershell
$new = (Get-Content -Raw -LiteralPath $env:AGX_BRANCH_FILE -ErrorAction Stop).TrimEnd("`r", "`n")
git check-ref-format "refs/heads/$new"
if ($LASTEXITCODE -eq 0) { git branch -m -- "$new" }
```

Git refuses when a branch named `<new>` already exists. Report git's message and stop. Never use `-M`, which would
overwrite that branch.

### 3. Report the upstream

Git moves the branch's upstream setting to the new name, so read it from there.

```bash
new=$(git branch --show-current) || exit 1
[ -n "$new" ] || exit 1
printf '%s\n' "$new"
upstream_remote=$(git config --get "branch.$new.remote" || true)
upstream_ref=$(git config --get "branch.$new.merge" || true)
printf 'upstream: %s %s\n' "${upstream_remote:-none}" "$upstream_ref"
```

```powershell
$new = git branch --show-current
if ($LASTEXITCODE -ne 0 -or -not $new) { return }
$new
$upstreamRemote = "$(git config --get "branch.$new.remote")"
$upstreamRef = "$(git config --get "branch.$new.merge")"
if ($upstreamRemote) { "upstream: $upstreamRemote $upstreamRef" } else { 'upstream: none' }
```

Report the old name and the new name. Then report the upstream:

- **No upstream.** Say that the branch has none.
- **The upstream remote is `.`.** The upstream is another local branch. Say so. No remote holds either name.
- **Any other remote.** Say that this remote still holds the branch under the name the upstream line shows, that the
  renamed branch still tracks that name, and that nothing on the remote was changed.

## Delete

`branch delete <name>` deletes a local branch. A merged branch is deleted directly. An unmerged branch is a destructive
step.

### 1. Record the tip and try the safe delete

```bash
name=$(cat -- "$AGX_BRANCH_FILE") || exit 1
git check-ref-format "refs/heads/$name" || exit 1
tip=$(git rev-parse --verify --quiet --end-of-options "refs/heads/$name")
printf 'tip of %s: %s\n' "$name" "${tip:-no such local branch}"
[ -n "$tip" ] && git branch -d -- "$name"
```

```powershell
$name = (Get-Content -Raw -LiteralPath $env:AGX_BRANCH_FILE -ErrorAction Stop).TrimEnd("`r", "`n")
git check-ref-format "refs/heads/$name"
if ($LASTEXITCODE -ne 0) { return }
$tip = git rev-parse --verify --quiet --end-of-options "refs/heads/$name"
if ($tip) { "tip of ${name}: $tip"; git branch -d -- "$name" } else { "tip of ${name}: no such local branch" }
```

`git branch -d` deletes only a merged branch. Read its answer:

- **It deleted the branch.** Go to step 4.
- **There is no such local branch.** Report that and stop.
- **The branch is checked out,** here or in another worktree. Report git's message and stop.
- **The branch is not fully merged.** Go to step 2.

### 2. Show what the branch alone holds

List the commits that no other ref holds. They are what the deletion loses.

```bash
name=$(cat -- "$AGX_BRANCH_FILE") || exit 1
git check-ref-format "refs/heads/$name" || exit 1
git log --oneline "refs/heads/$name" --not "--exclude=refs/heads/$name" --all --
```

```powershell
$name = (Get-Content -Raw -LiteralPath $env:AGX_BRANCH_FILE -ErrorAction Stop).TrimEnd("`r", "`n")
git check-ref-format "refs/heads/$name"
if ($LASTEXITCODE -eq 0) { git log --oneline "refs/heads/$name" --not "--exclude=refs/heads/$name" --all -- }
```

Show the user the branch name, its tip SHA and this list, as git printed it. When the list is empty, say that every
commit of the branch is also held by another ref, and still confirm.

### 3. Confirm once, then delete

Ask once whether to delete the branch. The reply confirms only when it names the action or the branch in the user's own
words. A bare "yes" does not confirm. A request that already named the deletion still gets this one confirmation, and
gets no second one. Without a confirming reply, leave the branch in place and say so.

```bash
name=$(cat -- "$AGX_BRANCH_FILE") || exit 1
git check-ref-format "refs/heads/$name" && git branch -D -- "$name"
```

```powershell
$name = (Get-Content -Raw -LiteralPath $env:AGX_BRANCH_FILE -ErrorAction Stop).TrimEnd("`r", "`n")
git check-ref-format "refs/heads/$name"
if ($LASTEXITCODE -eq 0) { git branch -D -- "$name" }
```

### 4. Verify and report

```bash
name=$(cat -- "$AGX_BRANCH_FILE") || exit 1
git check-ref-format "refs/heads/$name" || exit 1
git show-ref --verify --quiet "refs/heads/$name" && echo "still present: $name" || echo "deleted: $name"
```

```powershell
$name = (Get-Content -Raw -LiteralPath $env:AGX_BRANCH_FILE -ErrorAction Stop).TrimEnd("`r", "`n")
git check-ref-format "refs/heads/$name"
if ($LASTEXITCODE -ne 0) { return }
git show-ref --verify --quiet "refs/heads/$name"
if ($LASTEXITCODE -eq 0) { "still present: $name" } else { "deleted: $name" }
```

Report the deletion only when this prints `deleted`. Give the tip SHA from step 1 in the report, so the user can create
the branch again at that commit.

## The work-item branch

This is the rule a skill follows before the first commit of a work item. It is step 2 of
[the commit contract](commit.md#the-commit-contract). It keeps work-item commits off a detached HEAD and off the
protected branches.

### 1. Read the current branch

```bash
current=$(git branch --show-current)
printf 'current branch: %s\n' "${current:-none (detached HEAD)}"
```

```powershell
$current = "$(git branch --show-current)"
if ($current) { "current branch: $current" } else { 'current branch: none (detached HEAD)' }
```

### 2. Read the protected branches

Run the resolver in [base-and-remote.md](base-and-remote.md). Pass the base recorded for the work item when there is
one. This rule uses three kinds of result line: `base-branch`, `base-remote` and `protected`, and the `question` lines
for the first two.

When `base-branch` or `base-remote` reads `unresolved`, the resolver cannot say what the base is. Ask its `question`
lines for those two names, as it printed them, in one round. Then pass the answers to the resolver and run it again, as
that file describes. Ask once: the answers hold for the session.

That question is about the repository. It is not a question about whether to create the branch. Never ask the
`push-remote` question for this rule.

The protected branches are the names on the `protected` line, between `protected:` and `host:`, separated by commas.

### 3. Decide

- **The current branch is not on the `protected` line.** Stay on it. The rule is satisfied.
- **Detached HEAD, or the current branch is on the `protected` line.** Create a feature branch in step 5.

Do not ask whether to create the branch.

### 4. Name the branch

Take the first of these that gives a pattern:

1. The repository's branch naming, as its instruction files or contribution files state it.
2. The pattern of its existing branches. Read their names:

    ```bash
    git for-each-ref --format='%(refname:short)' refs/heads refs/remotes
    ```

3. `<type>/<slug>`, with the issue number when there is one.

The slug comes from the work folder or the issue.

### 5. Create and announce

Use the literal assignments here only for a name you generate with lowercase ASCII letters, digits, hyphens, and `/`
separators. Each component starts with a letter or digit. For any other name, including a name supplied by the user, use
the scratch-file input from [Create and switch](#create-and-switch) before running `git switch -c "$name"`.

```bash
name='<name>'
git switch -c "$name"
git branch --show-current
```

```powershell
$name = '<name>'
git switch -c "$name"
git branch --show-current
```

The new branch starts at the current commit, and uncommitted changes come along unchanged.

Announce the branch once the last command prints its name: say that you created it, give the name, and say that the user
can rename it. A rename request follows [Rename](#rename). Then continue with the commit. Do not wait for a reply.

The resolver's lines were resolved for the branch you left. Run the resolver again on the new branch before you use any
of them for another purpose.
