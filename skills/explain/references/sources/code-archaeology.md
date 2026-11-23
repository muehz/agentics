# Code Archaeology (git + in-repo)

## What this source contains

- Commit history (messages, dates, authors, diffs)
- PR descriptions, review comments, and discussion threads (via `gh`)
- Inline code comments, TODOs, FIXMEs, deprecation notes
- ADRs (architectural decision records) if the repo keeps them
- Tests. Names and assertions often encode the edge cases that motivated a change
- Related files modified in the same commits (co-change signal)
- CHANGELOG entries, release notes in the repo
- Issue/ticket IDs mentioned in commit messages and PR bodies

This is the most trustworthy source, tied directly to the code, and the most complete. Everything that went through the
repo should be here.

## How to search it

Expand the seed commit list from the repository root. Read the repository-relative `source_path`, `needle`, and
`pattern` as data from the file locator or search input in the call that uses them. Validate `first` and `last` as line
numbers, and resolve both range refs to `old_oid` and `new_oid` first.

```bash
# Full history of the file through renames
git --literal-pathspecs log --follow --oneline -- "$source_path"

# Pickaxe: commits that added or removed this exact text
git --literal-pathspecs log -S "$needle" -- "$source_path"

# Or for patterns:
git --literal-pathspecs log -G "$pattern" -- "$source_path"

# Who wrote each line and when
git blame -L "$first,$last" -- "$source_path"

# The full diff of a specific commit
git show <hash>

# Commits between two points affecting this file
git --literal-pathspecs log -p "$old_oid..$new_oid" -- "$source_path"
```

For each substantive commit, pull the PR context:

```bash
# Find the PR number from the merge commit or branch
git log -1 --format=%B <hash>

# Full PR context: body, review comments, linked issues
gh pr view <number> --json title,body,author,createdAt,mergedAt,labels,closingIssuesReferences,comments,reviews,files

# The --json reviews and comments fields are where the real signal is
```

Look for out-of-band docs:

```bash
# ADRs live in the directory skills/routing/references/repo-conventions.md resolves
rg -l -i 'architecture.decision' --glob '*.md'

# TODOs and FIXMEs near the target
rg -n -C2 '(TODO|FIXME|HACK|XXX|NOTE)' <target_file>

# Related tests. Names often encode the "why"
rg -l '<symbol>' --glob '*test*'
```

## What good evidence looks like here

- A PR description that explains the problem it solves, not only the change ("This fixes the pagination bug that caused
  X")
- A long review thread that debates alternatives
- An inline comment near the target line that explains a non-obvious constraint
- A test named `test_handles_edge_case_when_X` that reveals an edge case that motivated the code
- A commit message that references a ticket or incident ID
- A CHANGELOG entry that summarizes the user-visible rationale

## Common pitfalls

- **Squash-merge flatlands.** If the repo squashes PRs, the branch history loses the individual commits. Fall back to PR
  body and comments.
- **Misleading commit messages.** "Small refactor" sometimes hides an intentional behavior change. Look at the diff, not
  the message.
- **Cargo-culted patterns.** The author may have copied a pattern and not understood why. Check if the pattern
  originated earlier in the codebase. If it did, investigate _that_ commit.
- **Bot commits and auto-merges.** Dependabot, Renovate, and automated backports usually don't carry motivation. Skip
  them when you try to find intent.
- **Code as evidence of intent.** The code itself isn't evidence for why it exists. Evidence comes from commit messages,
  PRs, comments, tests, docs. Don't cite "the function is named X" as evidence of intent.

## What to return

Every commit/PR/comment that bears on the question, with:

- The exact text (quoted)
- The hash / PR number / file:line
- Author and date
- Whether it's direct (explicitly addresses the question) or circumstantial
