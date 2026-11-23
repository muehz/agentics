# Issue linkage on PR creation

Every PR created through this skill resolves issue linkage explicitly. The result is a set of detected issues that gh
applies and states, a user-supplied issue, or a recorded decision to proceed unlinked. An unlinked PR is a legal
outcome; a _forgotten_ linkage is not.

Applies to both tool paths, MCP `create_pull_request` and the `gh` CLI, and to `/agentics:gate branch`'s draft-PR
destination.

## Step 1: Detect candidates deterministically

Issue numbers in conversation context are not detection. A plan-only session has none, and plan-only sessions are the
ones that produce unlinked PRs. Scan three branch-local signals instead. `<base>` is the branch this PR will target. Use
the base already resolved for the create operation and state it by name. `/agentics:gate branch`'s draft-PR destination
determines one. Otherwise it is the value about to be passed to `--base`. Ask only when none resolves. If a PR targets
`release/1.x` and you scan it as `<default>..HEAD`, the scan pulls unrelated release-branch commits and files into
candidate detection. `gh pr view` can read the base back only once a PR exists.

Use the comparison point from [the base and remote resolver](../../../git/references/base-and-remote.md). In each call,
take `$base_ref` from that producer or its scratch record, as a fully qualified ref or validated commit ID. Resolve it
to a commit before scanning. For PowerShell, load `$plan_paths_file` with the absolute path of a session scratch file.

```bash
# bash
base_oid=$(git rev-parse --verify --end-of-options "${base_ref}^{commit}") || exit 1

# 1. Commit messages on the branch
git log --pretty=%B "$base_oid..HEAD" -- | grep -oE '#[0-9]+' | sort -u

# 2. Plan files touched on the branch
plan_pattern='(-plan\.md|/[0-9]{4}-[0-9]{2}(-[0-9]{2}-[a-z0-9-]+|/[a-z]+-[a-z0-9-]+)/plan\.md)$'
git diff --name-only -z "$base_oid..HEAD" -- | while IFS= read -r -d '' plan_file; do
  if [[ "$plan_file" =~ $plan_pattern && -f "$plan_file" ]]; then
    grep -hoE '#[0-9]+' -- "$plan_file"
  fi
done | sort -u

# 3. Branch name: a number that starts a path segment, optionally after issue- or gh-; a date segment yields none
git rev-parse --abbrev-ref HEAD | tr '/' '\n' | grep -vE '^[0-9]{4}-[0-9]{2}([^0-9]|$)' \
  | sed -nE 's/^(issue-|gh-)?([0-9]{1,6})([^[:alnum:]].*)?$/\2/p' | sort -u
```

```powershell
# PowerShell
$base_oid = git rev-parse --verify --end-of-options "${base_ref}^{commit}"
if ($LASTEXITCODE -ne 0) { throw 'Could not resolve the base commit' }

(git log --pretty=%B "$base_oid..HEAD" -- | Select-String -Pattern '#\d+' -AllMatches).Matches.Value | Sort-Object -Unique

git diff --name-only -z "--output=$plan_paths_file" "$base_oid..HEAD" --
if ($LASTEXITCODE -ne 0) { throw 'Could not list touched plan files' }
$PlanFiles = [IO.File]::ReadAllText($plan_paths_file) -split "`0" | Where-Object {
    $_ -match '(-plan\.md|/\d{4}-\d{2}(-\d{2}-[a-z0-9-]+|/[a-z]+-[a-z0-9-]+)/plan\.md)$' -and
        (Test-Path -LiteralPath $_ -PathType Leaf)
}
if ($PlanFiles) {
    (Select-String -LiteralPath $PlanFiles -Pattern '#\d+' -AllMatches).Matches.Value | Sort-Object -Unique
}

# Branch name: a number that starts a path segment, optionally after issue- or gh-; a date segment yields none
(git rev-parse --abbrev-ref HEAD) -split '/' |
    Where-Object { $_ -notmatch '^[0-9]{4}-[0-9]{2}([^0-9]|$)' } |
    ForEach-Object { if ($_ -cmatch '^(issue-|gh-)?([0-9]{1,6})([^0-9A-Za-z]|$)') { $Matches[2] } } |
    Sort-Object -Unique
```

A number in the branch name is a candidate only when it is a whole token at the start of a path segment, optionally
after `issue-` or `gh-`: `feat/417-search` and `issue-417` both yield 417. A segment that starts with a date
(`2026-11-…`) yields no candidate, and neither does a number anywhere else in a segment.

Do not hardcode a plan directory. A committed plan lives in the work root. Match either shape on the branch diff
instead: the flat `*-plan.md` file, or `plan.md` inside a monthly work folder (`YYYY-MM/<purpose>-<topic>/`). The folder
shape comes from [../../../decompose/references/chain-artifacts.md](../../../decompose/references/chain-artifacts.md).
Check the relative path from that file's location. Plans default to the repository, so a touched plan file is the common
case ([session-storage.md](../../../routing/references/session-storage.md)).

## Step 2: Validate every candidate

A `#N` may be a pull request, a closed issue, a cross-repo reference, or not a reference at all. It may be
`#!/bin/bash`, a CSS `#fff`, or a Markdown heading. Confirm each before you propose it:

```bash
gh issue view <N> --repo {owner}/{repo} --json number,title,state,stateReason
```

```powershell
gh issue view <N> --repo {owner}/{repo} --json number,title,state,stateReason
```

| Result                         | Disposition                                                     |
| ------------------------------ | --------------------------------------------------------------- |
| Not found, or resolves to a PR | Drop silently; it was never a linkage candidate                 |
| Open issue                     | Candidate                                                       |
| Closed issue                   | Surface separately: "found #N, already closed. Link it anyway?" |

Branch-name digits are the weakest signal, often noise such as a date or another tracker's id. Drop a number that fails
validation. Never propose it.

When neither `gh` nor the GitHub MCP tools are available, say so. Then fall through to the Step 3 question without
validation. Asking is always available. Never propose unvalidated candidates as if they were confirmed.

## Step 3: Resolve three ways

**Candidates found.** Apply the validated set and state it. It is not confirmed separately: it goes in the one-line
statement that precedes creation, beside the template and the base, or in the preview when the create operation has one.
State the set in this form:

```text
Detected issue linkage:
  #123: Add CSV export to the audit report (open)     → Closes
  #456: Improve audit reporting (open, parent)        → Part of
```

**No candidates.** Ask once, as one decision question in the format of
[Cross-cutting write safety](../../SKILL.md#cross-cutting-write-safety), with "proceed unlinked" as the ➡️
recommendation:

```text
❓ **Q1** - **Issue linkage**: No issue linkage was detected on this branch. How should the pull request link?
  1. Proceed unlinked: record it in the template's linkage section when it has one
  2. Attach an existing issue: give me the number
  3. Create one now: use `/agentics:gh` for a plain issue or `/agentics:spec draft` when the work needs product framing

➡️ Proceed unlinked: nothing on the branch points at an issue, and an unlinked pull request is a legal outcome
```

When the user already said how to link ("I'll link it later", "no issue"), ask nothing and record the pull request as
unlinked.

**Proceeding unlinked** needs no justification. When the PR template has a linkage section, record the decision there in
the form it asks for. Agentics's own template asks for `Issue linkage: none — <reason>`. With no linkage section, add no
line. The user's answer or earlier statement is the record, and the run reports the PR as unlinked.

## Step 4: Guard the closing keyword

`Closes #N` closes the issue the moment the PR merges, and a reopen does not restore the sub-issue state a reviewer
relied on. Reserve it for issues this PR **fully** resolves: every acceptance item or checklist entry in the issue body
is met by this PR's diff. When any item is left for another PR, or the body states no items to check, use `Part of #N`.
GitHub acts on a closing keyword only when the pull request merges into the repository's default branch. On a PR that
targets any other branch the keyword closes nothing at merge, so for an issue such a PR fully resolves, write `Refs #N`
and say that the issue closes when the change reaches the default branch.

| Candidate                                           | Keyword                                                                                                                        |
| --------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------ |
| Issue this PR fully resolves                        | `Closes #N`                                                                                                                    |
| Parent spec or epic with sibling slices outstanding | `Part of #N` (plain mention)                                                                                                   |
| Issue only partially addressed                      | `Part of #N` (plain mention)                                                                                                   |
| Issue referenced for context only                   | Plain mention, no keyword                                                                                                      |
| Open issue no branch signal touches, user-demanded  | State the zero-signal finding and the cost once, then write the keyword the user asked for and name the override in the report |

Establish parentage rather than infer it from titles. A candidate that has sub-issues is a parent:

```bash
gh api repos/{owner}/{repo}/issues/<N>/sub_issues --jq 'length'
```

```powershell
gh api repos/{owner}/{repo}/issues/<N>/sub_issues --jq 'length'
```

A non-zero count means parent, so use a plain mention. When the query returns nothing and there are two or more
candidates, ask which is the parent rather than guess.

The guard is a process gate, and the user can override it. When the user demands `Closes` on a parent or on an issue no
branch signal touches ("I own the repo, I'll reopen what shouldn't have closed"), state the cost once: a merge closes
the epic, and a reopen does not restore its sub-issue state. Then write the user's keyword set. Name the override in the
report, and say there that no `waive` row was recorded for it.

GitHub honors a closing keyword only from the PR **body** or a commit message on the branch, and only within the same
repository. A cross-repo close needs the `Closes {owner}/{repo}#N` form. See
[../issues/sub-issues.md](../issues/sub-issues.md) for the sub-issue API.
