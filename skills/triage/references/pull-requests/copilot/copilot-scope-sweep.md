# Copilot Scope Sweep

Before you apply a fix for a pattern-type Copilot finding, sweep the repository for more instances of the same pattern.
The sweep prevents partial fixes.

## When to sweep

Sweep for any finding where the issue is a repeatable pattern that could exist elsewhere:

- Version pins (e.g., `@v0` should be `@v1`)
- Broken config entry patterns (e.g., wrong jsonpath syntax)
- Stale GitHub Action references
- Incorrect documentation descriptions
- Repeated anti-patterns in similar files

If the finding is purely localized (a logic bug in one function), skip the sweep.

## Sweep commands

### Version pins

```bash
# Find all version pins in workflow files
grep -rn '@v0' . --include='*.yml' --include='*.yaml'

# Find specific action version references
grep -rn 'uses:.*@v' .github/workflows/
```

```powershell
# PowerShell equivalent
Get-ChildItem -Recurse -Include *.yml,*.yaml | Select-String '@v0'
```

### Config entry patterns

```bash
# Find all instances of a specific config pattern
grep -rn 'PATTERN' . --include='*.json' --include='*.yml' --include='*.yaml' --include='*.toml'
```

### Stale action references

```bash
# Find all GitHub Action references
grep -rn 'uses:.*@' .github/workflows/

# Find specific deprecated actions
grep -rn 'uses: actions/checkout@v2' .github/
```

### Documentation patterns

```bash
# Check sibling files in the same directory for similar issues
ls -la $(dirname {FLAGGED_FILE})

# Find similar description patterns across docs
grep -rn 'DESCRIPTION_PATTERN' docs/ README.md
```

An instance of the flagged pattern is part of the flagged finding, so fixing it is not fixing something the review did
not flag. A different issue the sweep happens to show is out of scope for this run.

Run each search from the worktree root, and read what it prints. A search is the evidence for the counts a reply states.

## Promote sweep results

For each additional instance:

1. Add a new row to the decision table with disposition `fix`, under the same finding.
2. Group all instances of the same pattern into a single commit.
3. After the commit, run the same search again.
4. State both counts in the thread reply: how many more instances the first search found, and what the second search
   finds now. Never claim a count no search printed.

An instance on a path in [Paths triage never pushes](../../subagent-contract.md#paths-triage-never-pushes), such as a
workflow file, is not changed. The reply names it as left for the developer.

## Example

**Finding:** Copilot flags a deprecated `parseConfig(path)` call in `src/cli/main.ts:15`

**Sweep:** `grep -rn 'parseConfig(' src/` finds 3 more instances:

- `src/cli/init.ts:8`
- `src/server/boot.ts:22`
- `src/server/reload.ts:4`

**Action:** All 4 instances fixed in one commit (`abc1234`). The same search now prints nothing.

**Reply:** "Fixed in abc1234, together with 3 more instances of the same pattern that a repository search found. The
same search now finds 0."

A version pin in `.github/workflows/` is the same kind of finding, but the workflow files are paths triage never
changes: the reply lists the instances the search found and leaves them for the developer.
