# Using Git Worktrees

## Overview

Git worktrees create isolated workspaces that share one repository, so you can work on multiple branches at the same
time with no need to switch.

**Core principle:**

- Detect existing isolation first.
- Prefer native tools.
- Fall back to git.
- Verify safety.

## When to Use

- Any work will occur on the same files via asynchronous or fleet execution
- Parallel development on multiple branches is needed, with no need to stash or switch
- Setup of an isolated workspace for a feature branch is needed

**Not for:**

- Single-branch development with no parallel work needed
- Temporary file edits that do not require branch isolation

## Step 0: Detect Existing Isolation

**Before you create anything, check whether you are already in an isolated workspace.**

Use the `WT_` prefix below verbatim. `GIT_DIR` and `GIT_COMMON_DIR` are git's own environment variables. Reuse of those
names risks an export that repoints later git commands in the session.

```bash
WT_DIR=$(cd "$(git rev-parse --git-dir)" 2>/dev/null && pwd -P)
WT_COMMON=$(cd "$(git rev-parse --git-common-dir)" 2>/dev/null && pwd -P)
BRANCH=$(git branch --show-current)
```

```powershell
# PowerShell
$WT_DIR = (Resolve-Path (git rev-parse --git-dir) -ErrorAction SilentlyContinue).Path
$WT_COMMON = (Resolve-Path (git rev-parse --git-common-dir) -ErrorAction SilentlyContinue).Path
$BRANCH = git branch --show-current
```

**Submodule guard:** `$WT_DIR != $WT_COMMON` is also true inside git submodules. Before you conclude "already in a
worktree," verify that you are not in a submodule:

```bash
# If this returns a path, you're in a submodule, not a worktree. Treat it as a normal repo.
git rev-parse --show-superproject-working-tree 2>/dev/null
```

**If `$WT_DIR != $WT_COMMON` (and not a submodule):** You are already in a linked worktree. Compare the branch the
request names with `$BRANCH`:

- **The branch already checked out, or no branch:** Reuse this worktree. Do NOT create another one. When the request
  carries a start point, decide [Fork code](#fork-code) first, from this worktree; for fork code, run no setup and no
  baseline, and say so in the report. Otherwise skip to project setup.
- **A different branch:** Create a sibling worktree beside this one. Proceed to Step 1. Never run setup or tests for
  that request in the current worktree. A caller such as execute gets a sibling this way for an isolated packet.

When you reuse the worktree, report the branch state:

- On a branch: "Already in isolated workspace at `<path>` on branch `<name>`."
- Detached HEAD: "Already in isolated workspace at `<path>` (detached HEAD, externally managed)."

**If `$WT_DIR == $WT_COMMON` (or in a submodule):** You are in a normal repo checkout. Proceed to Step 1.

## Step 1: Prefer Native Worktree Tools

Before you fall back to manual `git worktree add`, check for native isolation tools:

- **Claude Code:** `EnterWorktree` handles directory placement, branch creation, and cleanup automatically.
- **Other platforms:** Look for tools like `WorktreeCreate`, a `/worktree` command, or a `--worktree` flag.

If a native tool is available, no start point was given, and the request is not for a sibling of a linked worktree, do
these steps:

1. Use the native tool.
2. Skip to project setup.

Native tools choose their own base and their own placement, so a `$START_POINT` and a sibling of a linked worktree
always take the manual path below. The git command creates phantom state that the harness cannot see or manage. Native
tools track their own worktrees correctly.

**Only proceed to directory selection if no native worktree tool is available, a start point was given, or the request
is for a sibling of a linked worktree.**

## Directory Selection Process

Honor an explicit placement before you scan the defaults. Check the user's specified parent or destination for manual
creation first. Then check the applicable repository instructions (canonical `AGENTS.md` and any already-loaded scoped
instructions). Either way, do these steps:

1. Set `$LOCATION` to that parent or exact destination.
2. Record which kind it is.
3. Proceed to safety verification before creation, even when the directory does not yet exist.

If repository instructions conflict without a precedence rule, resolve that conflict before you create anything.

The default scan below applies only when neither the user nor repository instructions specified placement. It never
relocates an existing worktree. Run the scan below from the directory that `git rev-parse --show-toplevel` prints. Run
the Safety Verification ignore check from that directory too. Inside a submodule, that directory is the submodule root,
not the superproject.

**Sibling of a linked worktree:** With no explicit placement, skip the scan, because its candidates would nest the new
worktree inside the current one. Set `$LOCATION` to the directory that holds the current worktree, as an absolute path,
and proceed to safety verification:

```bash
LOCATION=$(dirname "$(git rev-parse --show-toplevel)")
```

```powershell
# PowerShell
$LOCATION = Split-Path -Parent (git rev-parse --show-toplevel)
```

### 1. Check for an existing worktree parent

Scan in priority order. The first match wins:

```bash
LOCATION=""
for d in .worktrees .claude/worktrees .codex/worktrees .github/copilot/worktrees; do
  if [ -d "$d" ]; then
    LOCATION="$d"; break
  fi
done
```

```powershell
# PowerShell
$LOCATION = $null
foreach ($d in '.worktrees', '.claude/worktrees', '.codex/worktrees', '.github/copilot/worktrees') {
    if (Test-Path $d) { $LOCATION = $d; break }
}
```

**Why this order:** `.worktrees/` gives every surface the same default parent when it already exists. Otherwise, reuse
the first existing surface-specific parent rather than create the generic fallback.

| Surface                      | Worktree parent              |
| ---------------------------- | ---------------------------- |
| None / generic               | `.worktrees/`                |
| Claude Code / Claude Desktop | `.claude/worktrees/`         |
| Codex CLI                    | `.codex/worktrees/`          |
| GitHub Copilot CLI / VS Code | `.github/copilot/worktrees/` |

### 2. Default when none exist

If `$LOCATION` is still unset after the scan, create `.worktrees/`:

```bash
if [ -z "$LOCATION" ]; then
  LOCATION=".worktrees"
  mkdir -p "$LOCATION"
fi
```

```powershell
# PowerShell
if (-not $LOCATION) {
    $LOCATION = '.worktrees'
    New-Item -ItemType Directory -Force -Path $LOCATION | Out-Null
}
```

Once user and repository preferences are exhausted, default selection does not prompt the user or search unrelated
project docs. Existing directories then determine the fallback.

## Safety Verification

All four default candidate parents live inside the working tree, so git must ignore each one. Apply the same check to
user-specified placement inside the repository. An external destination needs no ignore rule in this repository.

Before the snippets below, resolve the requested placement against the repository root.

For a project-local location, do these steps:

1. Set `$LOCATION` to its repository-relative path with forward slashes.
2. Set `$CHECK_ROOT` to the repository root and `$IGNORE_PATH` to `$LOCATION`.

For example, `C:\repo\custom-worktrees` becomes `custom-worktrees`, so the ignore rule receives no absolute path.

For an exact destination, do these steps:

1. Check that destination rather than its parent.
2. Ignore that destination rather than its parent.

For an external location, do these steps:

1. Skip these snippets entirely.
2. Retain its resolved path for creation.

For a sibling of a linked worktree, `$LOCATION` is external unless it lies inside the main working tree, which is the
first `worktree` line of `git worktree list --porcelain`. When it lies inside, do these steps:

1. Keep `$LOCATION` absolute, because creation runs from the current worktree.
2. Set `$CHECK_ROOT` to the main working tree, and `$IGNORE_PATH` to the path of `$LOCATION` relative to it, with
   forward slashes.

**MUST check that git ignores a project-local location before you create a worktree.** When git does not ignore it yet,
add it to the repository's local exclude file. That file is `info/exclude` inside the common git directory, so one rule
covers the main working tree and every linked worktree, and nothing is committed. Never append the rule to `.gitignore`,
and never commit a `.gitignore` for it.

```bash
if ! git -C "$CHECK_ROOT" check-ignore -q -- "$IGNORE_PATH/worktree-probe"; then
  exclude="$(git rev-parse --path-format=absolute --git-common-dir)/info/exclude"
  mkdir -p "$(dirname "$exclude")" && printf '\n/%s/\n' "$IGNORE_PATH" >>"$exclude"
  if git -C "$CHECK_ROOT" check-ignore -q -- "$IGNORE_PATH/worktree-probe"; then
    echo "Added /$IGNORE_PATH/ to $exclude"
  else
    echo "Could not add /$IGNORE_PATH/ to $exclude; the path stays untracked"
  fi
fi
```

```powershell
# PowerShell
git -C "$CHECK_ROOT" check-ignore -q -- "$IGNORE_PATH/worktree-probe"
if ($LASTEXITCODE -ne 0) {
    $exclude = "$(git rev-parse --path-format=absolute --git-common-dir)/info/exclude"
    try {
        New-Item -ItemType Directory -Force -Path (Split-Path -Parent "$exclude") -ErrorAction Stop | Out-Null
        [IO.File]::AppendAllText("$exclude", "`n/$IGNORE_PATH/`n")
    } catch {
        'Write failed: ' + $_.Exception.Message
    }
    git -C "$CHECK_ROOT" check-ignore -q -- "$IGNORE_PATH/worktree-probe"
    if ($LASTEXITCODE -eq 0) {
        "Added /$IGNORE_PATH/ to $exclude"
    } else {
        "Could not add /$IGNORE_PATH/ to $exclude; the path stays untracked"
    }
}
```

**Why critical:** The check prevents an accidental commit of worktree contents to the repository.

The check asks about a path beneath the location, because git answers for a directory that does not exist yet only
through a path inside it. The rule is written with line feeds in both shells, because git does not read a carriage
return as a line ending. The snippets run the check again after the write, so report "ignored" only from that second
check.

When the exclude file cannot be written, report the failure with the message the shell printed. Write no other ignore
file. The path stays untracked: it shows in `git status`, and nothing stages it.

## Creation Steps

> **Cross-platform note:** `git worktree` commands work identically on all platforms. The shell commands below show bash
> and PowerShell equivalents. Use whichever matches the user's environment.
>
> On Windows, keep worktree paths short to avoid long-path edge cases (`MAX_PATH` 260) in older tools and scripts. On
> Windows, prefer `C:\wt\feature-name` over deeply nested directories. Symlink behavior also differs by platform. macOS
> and Linux usually allow symlinks by default, while Windows may require Developer Mode or elevated privileges. Do not
> assume symlink-heavy tooling behaves identically everywhere.

### 1. Detect Project Name

```bash
project=$(basename "$(git rev-parse --show-toplevel)")
```

```powershell
# PowerShell
$project = Split-Path -Leaf (git rev-parse --show-toplevel)
```

### 2. Create Worktree

`$BRANCH_NAME` is the branch argument from the invocation (`/agentics:git worktrees <branch>`). An optional second
argument, `$START_POINT`, is the commit or branch that the new branch starts from, and execute passes it for isolated
packets. Without it, the branch starts from the current HEAD.

The start point is a commit or a branch that already exists locally. This mode never fetches a pull request head. A
caller that fetched one says so by adding `pr <number>` after the start point:
`/agentics:git worktrees <branch> <start-point> pr <number>`. With a start point, decide [Fork code](#fork-code) before
you create the worktree.

If no branch was given, ask as a ❓ question for the branch name. In that question, include a ➡️ recommendation drawn
from the request (e.g. an issue slug or the feature described). Never invent a branch name without asking.

The snippets below append the branch name to a selected **parent**. If the user supplied an exact destination instead,
do these steps:

1. Replace the first assignment with `path="$LOCATION"` (Bash) or `$path = $LOCATION` (PowerShell).
2. Do not append the branch name again.

Project-local paths that safety verification normalized still refer to the requested destination when you use them from
the repository root.

```bash
path="$LOCATION/$BRANCH_NAME"
git worktree add "$path" -b "$BRANCH_NAME" ${START_POINT:+"$START_POINT"}
cd "$path"
```

```powershell
# PowerShell
$path = "$LOCATION/$BRANCH_NAME"
if ($START_POINT) { git worktree add $path -b $BRANCH_NAME $START_POINT } else { git worktree add $path -b $BRANCH_NAME }
Set-Location $path
```

**Branch already exists:** If `-b` fails with `fatal: a branch named '<name>' already exists`, do not invent a new name
silently. Check `git worktree list`. If the branch is checked out elsewhere, see Troubleshooting. Otherwise, do these
steps:

1. Confirm with the user that reuse is intended.
2. Attach the existing branch with `git worktree add "$path" "$BRANCH_NAME"` (no `-b`).

**Creation fails:** If the worktree cannot be created, for example because `git worktree add` fails with a permission
error (sandbox denial), do these steps:

1. Report git's message and what blocked the creation.
2. Stop when a calling skill asked for the worktree, or when the request was for a sibling of a linked worktree. The
   caller asked for isolation, so do not work in the current directory, and run no setup and no tests there.
3. Otherwise the user invoked `/agentics:git worktrees`. Offer, as a ❓ question, to work in the current directory
   instead. Only on a yes, proceed with setup and baseline tests in place.

#### Fork code

A start point can hold code from another repository. Setup and the baseline run that code, so decide this before the
`cd` into the new worktree, from the worktree the request came from. A request with no start point is not fork code.

A start point is fork code in three cases:

1. It carries `pr <number>`, and the host reports that pull request as coming from another repository.
2. It carries `pr <number>`, and the host cannot be read: `gh` is missing, is not signed in, or fails.
3. It is a remote-tracking ref of a remote that is neither the base remote nor a push remote that git's push
   configuration or the user named.

For a start point that carries `pr <number>`, ask the host. `true` is case 1. A failure or an empty answer is case 2.

```bash
gh pr view "$PR_NUMBER" --json isCrossRepository --jq '.isCrossRepository'
```

```powershell
# PowerShell
gh pr view "$PR_NUMBER" --json isCrossRepository --jq '.isCrossRepository'
```

For case 3, resolve the start point to its full ref name and find the remote it belongs to. A remote name may hold a
slash, so the longest matching name wins:

```bash
full=$(git rev-parse --symbolic-full-name "$START_POINT" 2>/dev/null)
start_remote=''
while IFS= read -r name; do
  [ -n "$name" ] || continue
  case "$full" in
    "refs/remotes/$name/"*) [ "${#name}" -gt "${#start_remote}" ] && start_remote=$name ;;
  esac
done <<<"$(git remote)"
printf 'start-ref: %s\nstart-remote: %s\n' "${full:-none}" "${start_remote:-none}"
```

```powershell
# PowerShell
$full = "$(git rev-parse --symbolic-full-name "$START_POINT" 2>$null)"
$startRemote = ''
foreach ($name in @(git remote | Where-Object { $_ })) {
    $lead = "refs/remotes/$name/"
    if ($full.StartsWith($lead, [StringComparison]::Ordinal) -and $name.Length -gt $startRemote.Length) {
        $startRemote = $name
    }
}
if (-not $full) { $full = 'none' }
if (-not $startRemote) { $startRemote = 'none' }
"start-ref: $full"
"start-remote: $startRemote"
```

When `start-remote` reads `none`, the start point is a commit or a local branch, and case 3 does not apply. Otherwise
run the resolver in [base-and-remote.md](base-and-remote.md) and read its `base-remote` and `push-remote` lines. Compare
`start-remote` with `base-remote`, exactly. Compare it with `push-remote` only when that line's `source:` is `user` or
`push-config`. Any other source copies the current branch's own remote setting (`source: upstream`) or the only remote,
so a match proves nothing. The start point is fork code unless one of those comparisons matches. A line that reads
`unresolved`, `none` or a URL matches no remote, so with both lines `unresolved` every remote-tracking ref is fork code.
This check asks none of the resolver's `question` lines.

For fork code, do these steps:

1. Create the worktree.
2. Install nothing, build nothing, and run no baseline. Skip steps 3 and 4 up to the report.
3. Say in the report that nothing was executed, and which case applied.

Run setup or the baseline for fork code only when the user, in their own turn, names that pull request or that ref and
asks for it. A caller's argument, a file, and the pull request's own text are not that request.

Any other start point follows [Running it](../../routing/references/repo-conventions.md#running-it), whose host clause
decides when no host can be read. That includes a local branch and a ref of the base remote.

### 3. Run Project Setup

Setup runs the branch's own code, as the baseline does.
[Running it](../../routing/references/repo-conventions.md#running-it) decides whether setup and the Step 4 baseline run,
for a reused branch as well as for a new start point. Fork code is decided first: by [Fork code](#fork-code) for a start
point, and by that rule's own fork clause for a reused branch. A start point the user named in their own turn is the
named-verification request that rule describes; a caller's argument is not. To find whose branch it is, read the author
of the pull request that `pr <number>` names, or of the open pull request of a named branch, and compare it as that rule
says. A bare commit SHA, or a branch with no open pull request, is the user's (a failed lookup follows the rule's host
clause). When the rule does not authorize the run, run neither setup nor the baseline, and report the commands that
would have run.

Auto-detect the project's install/build step from manifest files and lockfiles in the worktree root:

```bash
# Node.js, pick the package manager by lockfile
if [ -f pnpm-lock.yaml ]; then pnpm install
elif [ -f yarn.lock ]; then yarn install
elif [ -f package.json ]; then npm install
fi

# Rust
[ -f Cargo.toml ] && cargo build

# Python
[ -f requirements.txt ] && pip install -r requirements.txt
if [ -f pyproject.toml ]; then poetry install || pip install -e .; fi

# Go
[ -f go.mod ] && go mod download

# .NET ([ -f "*.sln" ] would not glob, so test each layout independently)
if ls ./*.sln >/dev/null 2>&1 || ls ./*.csproj >/dev/null 2>&1; then dotnet restore; fi
```

```powershell
# PowerShell
if (Test-Path pnpm-lock.yaml) { pnpm install }
elseif (Test-Path yarn.lock) { yarn install }
elseif (Test-Path package.json) { npm install }
if (Test-Path Cargo.toml) { cargo build }
if (Test-Path requirements.txt) { pip install -r requirements.txt }
if (Test-Path pyproject.toml) { poetry install; if (-not $?) { pip install -e . } }
if (Test-Path go.mod) { go mod download }
if ((Get-ChildItem *.sln, *.csproj -ErrorAction SilentlyContinue)) { dotnet restore }
```

If no recognized manifest is present, skip this step and proceed.

### 4. Run Baseline Tests and Report

The test command is the one that
[repo-conventions.md](../../routing/references/repo-conventions.md#verification-command) resolves. When none resolves,
discover the project's test command (manifest scripts, `Makefile`, `justfile`, CI config). Run it to confirm a clean
starting point, when [Running it](../../routing/references/repo-conventions.md#running-it) authorizes the run. Report
what happened, and name the command. Include whether `$BRANCH_NAME` was created or an existing branch was reused. If
tests fail or no test command exists, say so. Never report "tests passing" unless you ran them in this worktree.

```text
Worktree ready at <full-path> on branch <name> (created)   # or: "(existing branch reused)"
Tests passing (<N> tests, 0 failures): <command>   # or: "No test command found" / "Baseline failing: <summary>"
Ready to implement <feature-name>
```

When Running it does not authorize the run on another person's branch, the second line reads:

```text
Baseline not run: <setup>, <test> await approval (another person's branch). No install, no build.
```

For fork code, the second line reads:

```text
Nothing was executed: the start point is fork code (<case>). No install, no build, no baseline.
```

## Quick Reference

| Situation                                         | Action                                                                                     |
| ------------------------------------------------- | ------------------------------------------------------------------------------------------ |
| In a linked worktree, same or no branch requested | Reuse it, skip creation (Step 0)                                                           |
| In a linked worktree, different branch requested  | Create a sibling beside it; no setup or tests in the current one                           |
| In a submodule                                    | Treat as normal repo (Step 0 guard)                                                        |
| Native worktree tool available                    | Use it (Step 1), unless a start point was given or a sibling is needed                     |
| No native tool                                    | Git worktree fallback (directory selection)                                                |
| User specified manual placement                   | Honor it before repository preferences and defaults                                        |
| Repository specifies placement                    | Honor it before defaults, even if absent on disk                                           |
| `.worktrees/` exists                              | Use it (verify ignored)                                                                    |
| `.claude/worktrees/` exists                       | Use it (verify ignored)                                                                    |
| `.codex/worktrees/` exists                        | Use it (verify ignored)                                                                    |
| `.github/copilot/worktrees/` exists               | Use it (verify ignored)                                                                    |
| Multiple exist                                    | Use first match in priority order above                                                    |
| None exist                                        | Create `.worktrees/` (verify ignored)                                                      |
| Directory not ignored                             | Add it to the local exclude file; report a failed write                                    |
| Start point is fork code                          | Create the worktree; install, build and run nothing                                        |
| Worktree cannot be created                        | Caller or sibling: stop and report. `/agentics:git worktrees` user: offer to work in place |
| No setup step apparent                            | Skip and proceed                                                                           |

## Example Workflow

```text
[No explicit placement; scan candidates: .worktrees and .claude/worktrees exist, so use .worktrees]
[Verify ignored: git check-ignore confirms .worktrees is ignored]
[Create worktree: git worktree add .worktrees/auth -b feature/auth]
[Running it: a new branch from the user's own start point is the user's, so setup and the baseline run]
[Run project setup command (auto-detected from project files)]
[Run the verification command, npm test - 47 passing]

Worktree ready at /home/dev/myproject/.worktrees/auth
Tests passing (47 tests, 0 failures): npm test
Ready to implement auth feature
```

## Troubleshooting

### Detached HEAD after checkout

- **Symptom:** `git status` shows `HEAD detached at ...` inside the worktree.
- **Fix:** In the switch call, read the selected branch name into `name` from a scratch file or the branch listing. Use
  `git switch -- "$name"` if it exists, or `git switch -c "$name"` when creating it.
- **Prevent it:** In the call that reads the worktree path and branch name as `path` and `name`, create an attached
  branch with `git worktree add -b "$name" -- "$path"`.

### Stale lock files

- **Symptom:** Git reports a locked or missing worktree that no longer exists.
- **Fix:** Run `git worktree prune` from the main repository first. If the lock is still present and you have confirmed
  that the worktree is gone, do these steps:
    1. Remove the stale lock file under `.git/worktrees/<name>/locked`.
    2. Run `git worktree prune` again.

### `fatal: '<branch>' is already checked out`

- **Symptom:** You try to create or switch a worktree for a branch Git already has checked out elsewhere.
- **Fix:** Find the other checkout with `git worktree list`. Then do one of these:
    - Use that existing worktree.
    - Remove/prune the stale one.
    - Create a different branch name for the new worktree.
- **Prevent it:** Use one active worktree per branch.

### Worktree not showing in `git worktree list`

- **Symptom:** The directory exists, but Git does not list it as a registered worktree.
- **Fix:** From the repository root, run `git worktree prune` to clear stale metadata. Then re-register the worktree
  with `git worktree repair <path-to-worktree>`. If the worktree was moved manually, repair both the repository root and
  the moved path before you continue.

## Removal

The skill that creates a worktree removes it when its work there is done, with a plain removal. Run it from outside the
worktree, with the worktree's path in `$wt`:

```bash
git worktree remove "$wt" || echo "Kept $wt: git refused the removal (message above)"
```

```powershell
# PowerShell
git worktree remove "$wt"
if ($LASTEXITCODE -ne 0) { "Kept ${wt}: git refused the removal (message above)" }
```

Git refuses a plain removal when the worktree holds changes or untracked files. On a refusal, leave the worktree in
place and report git's message. Never retry a failed removal with `--force`.

A feature worktree that outlives its run is removed by [Prune Mode](#prune-mode-agenticsgit-worktrees-prune). A skill
that finds one whose branch has merged offers `/agentics:git worktrees prune`.

### Remove a worktree the user discarded

A forced removal is only for a worktree the user discarded by name. It is a destructive step, because the uncommitted
work in the worktree is gone for good. Do these steps:

1. Show the worktree's resolved path and what will be lost, from `git -C "$wt" status --short`.
2. Confirm once, at this point. The reply must name the worktree or the removal in the user's own words. A decision to
   discard that worktree by name in Prune Mode step 3 is this confirmation, so do not ask again.
3. Remove it:

```bash
git worktree remove --force "$wt"
```

```powershell
# PowerShell
git worktree remove --force "$wt"
```

## Prune Mode (`/agentics:git worktrees prune`)

Remove merged or abandoned worktrees to reclaim disk. Deletion is irreversible, so every step guards against the removal
of anything that is in use or holds uncommitted work. **The buckets below are advice, not permission.** This workflow
destroys user state with no code review to catch a slip. The confirmation steps are the review.

### 1. Snapshot and audit

Record free space first (`df -h .` / `Get-PSDrive -Name (Get-Location).Drive.Name`). Then enumerate **from
`git worktree list --porcelain`, never from hand-typed or remembered paths**. A hand-typed parent directory misses
worktrees created elsewhere. Skip the main checkout.

The integration branch comes from the resolver that [Fork code](#fork-code) links, never from an assumed name. Run it
once, from the directory where prune was invoked, and read its `base-branch` and `base-ref` lines. Resolve the selected
base from its full ref or SHA to a verified commit ID using `git rev-parse --verify --end-of-options` with the
`^{commit}` suffix. Write that ID to a work-item scratch file with the file tool, then pass the file path as
`AGX_BASE_OID_FILE` through structured environment input, or read it from a fixed scratch path. For an unresolved base
use an empty file. With an empty base, every worktree reads `unmerged`. The `unmerged` reading is the safe direction,
because nothing is proposed for deletion on a merge claim nobody verified.

For each linked worktree, gather:

```bash
BASE=$(cat -- "$AGX_BASE_OID_FILE") || exit 1
printf 'compared against: %s\n' "${BASE:-nothing, so every worktree reads unmerged}"
git worktree list --porcelain | sed -n 's/^worktree //p' | tail -n +2 | while IFS= read -r wt; do
  branch=$(git -C "$wt" branch --show-current)
  wip=$(git -C "$wt" status --porcelain | wc -l | tr -d ' ')
  merged="unmerged"
  [ -n "$BASE" ] && git merge-base --is-ancestor "$(git -C "$wt" rev-parse HEAD)" "$BASE" 2>/dev/null && merged="merged"
  printf '%s\tbranch:%s\twip:%s\t%s\n' "$wt" "${branch:-detached}" "$wip" "$merged"
done
```

```powershell
# PowerShell
$BASE = (Get-Content -Raw -LiteralPath $env:AGX_BASE_OID_FILE -ErrorAction Stop)
if ($BASE) { $BASE = $BASE.TrimEnd("`r", "`n") }
$compared = if ($BASE) { $BASE } else { 'nothing, so every worktree reads unmerged' }
"compared against: $compared"
$wts = (git worktree list --porcelain | Select-String '^worktree ').Line -replace '^worktree ' | Select-Object -Skip 1
foreach ($wt in $wts) {
    $branch = git -C $wt branch --show-current
    $wip = (git -C $wt status --porcelain | Measure-Object -Line).Lines
    $merged = 'unmerged'
    if ($BASE) {
        git merge-base --is-ancestor (git -C $wt rev-parse HEAD) $BASE 2>$null
        if ($?) { $merged = 'merged' }
    }
    "{0}`tbranch:{1}`twip:{2}`t{3}" -f $wt, ($branch ?? 'detached'), $wip, $merged
}
```

When the resolver prints a `question` line for the base remote or the base branch, ask those lines as ❓ questions in
one round, and leave its push remote question unasked. The ➡️ for the base branch recommends the branch most PRs target
(`gh pr list --state merged --json baseRefName`). Then pass the answers to the resolver and run it again, as that file
describes. When its `fetched` field reads that the ref is absent locally, stop and report it. Do not substitute a guess.

State the `base-branch` and the `compared against` line when you present the audit.

Where a remote exists, add PR state per branch. In the same call, capture `branch` with
`git -C "$wt" branch --show-current` for that worktree. If it is nonempty, run `gh pr list --state all "--head=$branch"`
via `/agentics:gh`. Classify each worktree into a suggested bucket:

- **safe**: merged or PR-closed, no uncommitted work.
- **verify**: unmerged but stale, or state unclear.
- **hold**: uncommitted work, or plausibly in use by another session or agent.

### 2. Confirm the in-use set, a safety gate

The audit cannot see every consumer. A parallel session, a fleet run, or a background agent may work in a worktree whose
name never surfaced. Present the classified list. Ask as a ❓ question which worktrees are in use, before you touch
anything. Include a ➡️ recommendation to treat every `hold` and `verify` worktree as in use, unless the user names it
free. The user's answer **overrides every `safe` bucket**. The audit is advice, not fact. A named-target invocation
("prune these three") satisfies this gate for the named set. The audit still runs on those targets, to surface anything
surprising (an open PR or unmerged commits). Step 3 still gets its per-worktree decision for any `wip > 0`.

### 3. Pause on irreversible loss, a safety gate

`wip: N` means N uncommitted changes. The removal of a **clean** worktree is recoverable, because its branch ref
survives. Uncommitted work is gone for good. For every candidate with `wip > 0`, do these steps:

1. Show the diff (`git -C <wt> status --short`).
2. Get an explicit per-worktree decision: commit it ([the commit contract](commit.md#the-commit-contract)), save it with
   [Stash](history-operations.md#stash), or discard it by name.

Never fold a `wip` worktree into a bulk yes.

### 4. Prune the confirmed set

A worktree the user discarded by name in step 3 is removed by
[Remove a worktree the user discarded](#remove-a-worktree-the-user-discarded).

Every other confirmed worktree gets the plain removal of [Removal](#removal), under its rule for a refusal. A refusal
here means the worktree holds changes or untracked files the audit did not show.

After the removals, run `git worktree prune`.

Branch refs survive removal, so no commits are lost. Say so in the report. Offer branch deletion separately: follow
[branch.md](branch.md#delete). Never bundle it into the worktree removal.

### 5. Report

Report these items:

- Free space before and after, with the amount reclaimed.
- The worktrees removed.
- A one-line reason for each worktree held back (in use by whom, or uncommitted work that awaits a decision).

## Integration

**Called by:**

- **intent**, when the intent is captured and implementation follows
- Any skill that needs an isolated workspace

**Pairs with:**

- The skill that asked for the worktree, which removes it under [Removal](#removal)
- **execute**, where work happens in this worktree
