# Reference: refresh documentation

## Scope

When the invoking context supplies a `## Documentation impact` work list, the run is **scoped**. `gate` branch mode
supplies such a list. Apply the reconciliation rules and output contract only to the listed docs. Skip repo-wide checks,
such as breadcrumb trails, config-driven route sweeps, and cross-repository references, and leave them out of the
report. Without a work list, the run is repo-wide. Every section below applies.

## Reconciliation rules

### Direction

The direction is one-way by default. Update the documentation to reflect **current implementation reality**. Unless an
explicit instruction says otherwise, do not modify implementation artifacts to match the docs.

### Evidence-backed discrepancy reconciliation

If the work, at any size, touches any of these, create a claim ledger before you edit:

- A versioned claim
- A provider contract
- A retained historical observation

A multi-file refresh of local claims you can check now needs no ledger: edit those claims directly.

Each entry needs a path and line, one independently falsifiable claim, an evidence class, direct evidence, and a
disposition. The disposition is `correct`, `qualify`, `preserve historical`, `unresolved`, or `no change`.

Use the source that controls the claim:

- Current repository behavior: implementation, tests, tracked configuration, or a reproducible local command.
- Provider or external behavior: current first-party documentation. Silence is not a contradiction.
- Dated probe, historical result, or provenance: a retained transcript, result artifact, or commit-qualified record that
  establishes the exact event. Current behavior and adjacent Git history do not establish an old version, date, SHA, or
  run result.

Split a sentence that mixes local and external facts. Confirm the local portion. Do not present the external portion as
confirmed. A candidate finding does not automatically require an edit. Make the smallest correction only when evidence
provides replacement wording. Otherwise, qualify the statement or record the missing evidence.

Use explicit language for evidence boundaries: `Current implementation:`, `Current official documentation:`,
`Retained probe:` with its version and date, and `Not established by retained evidence:`. Never refresh a probe date
merely because you reread the documentation. Set a verification date only on a claim this run checked against retained
evidence. Never infer unknown historical precision.

### Batch discipline

Work in small, disjoint batches. A batch is normally three to five files. Capture the baseline revision. Preserve
unrelated changes. For every changed file, run focused verification and `git diff --check`. At every batch boundary,
recompute the remaining queue. Do this because merges, deletions, and behavior changes invalidate an old file list.
Record why you left reviewed files unchanged, so you can explain a claim-level audit at file level.

### Instruction-file copies

When a repository keeps copies or symlinks of its canonical instruction file:

- Edit only the canonical file.
- Validate that each symlink target resolves. Validate that each copy matches the canonical file.
- Report a broken symlink or a differing copy as an unresolved item.

### Breadcrumb trails

If the repository uses top-of-file breadcrumb navigation:

- Pattern: `[Home](../../README.md) > [Section](../README.md) > Page Title`
- Match the breadcrumb format the repository already uses. Where it has none to copy, write plain text labels, because
  the slop catalog removes decorative emoji.
- Add breadcrumbs to files that do not have them. Skip repo-root files such as README, CHANGELOG, and CONTRIBUTING.
- Make sure that relative paths resolve correctly.

### Config-driven route documentation

When a repository has config-defined routes, such as routing rules, API endpoints, or dispatch events:

1. Parse rule identifiers from the config source.
2. Confirm that each rule has a corresponding doc page.
3. Flag missing docs as **missing coverage**. Create each missing doc from an existing route doc.
4. Flag orphaned route docs as **stale references**. An orphaned doc has no rule that matches it.

### Cross-repository references

When workspace context includes peer repositories:

- Add cross-references only when concrete evidence exists. Evidence includes imports, shared infra, workflow
  dependencies, operational handoffs, and onboarding steps.
- Leave unevidenced references out of the report.

### Fleet and parallel worker boundaries

When you run as a fleet worker:

- Read workspace context freely. Write only within the assigned repository.
- Cross-repository normalization is coordinator-only.

## Output contract

Use this contract for a run with a `## Documentation impact` work list and for a repository scan. A refresh of one or
two files does not use it: report the changed files and the unresolved items in a few lines, with no empty sections.
That rule, in `SKILL.md` under **Refresh against code**, takes precedence over this contract.

Return a plain-English markdown report, not JSON. Print a section only when it has something to report. Leave out a
section that has nothing to report, and do not print `None` or `not applicable` in its place. Keep the numbers of the
sections you print, so a reader can refer to a section by number. The sections are:

### 1. Status

Always print this section.

- Repository name and overall status: `success`, `partial`, or `blocked`
- `success`: all applicable updates completed with no unresolved items
- `partial`: at least one update completed, but unresolved items remain
- `blocked`: no safe implementation-backed update can proceed

### 2. Files Updated

For each changed file, list the path, the applicability (`required` or `optional`), and a brief reason.

### 3. Files Unchanged

Files reviewed and verified accurate.

### 4. Files Not Applicable

Files intentionally skipped, with the reason for each.

### 5. Instruction-File Status

Print this section only when the repository keeps copies or symlinks of its canonical instruction file and the run
checked them. Name the canonical file. State whether each copy or symlink is intact, broken, or differing.

### 6. Breadcrumb Status

Print this section only when the run checked breadcrumb trails. Give the breadcrumbs added or updated, the files you
verified, and intentional exceptions.

### 7. Cross-Repository References

Print this section only when the run found a peer reference with implementation-backed evidence. Give each peer and the
status of its reference: `existing`, `added`, or `updated`.

### 8. Unresolved Items

Human decisions or blockers. Print this section only when the run has any.

### 9. Change Summary

A concise, grouped summary of all changes that you made. Print this section only when the run changed files.

### 10. Reconciliation Summary

Print this section only for an evidence-backed refresh. Add these items:

- Candidate claims
- Distinct files
- Changed files
- Intentionally unchanged files
- Removed or deferred files

State the baseline revision. List deferred claims, each with the missing evidence and the owner.

### Formatting rules

- Use only repository-relative POSIX paths.
- Keep file lists sorted in ascending lexicographic order.
