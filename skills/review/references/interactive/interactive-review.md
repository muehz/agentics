# Interactive review

A guided, single-reviewer review. It reads the change against its requirements, checks quality, security, and tests,
sorts each finding by severity and lead judgment, and ends in a review verdict.

Apply the repository guidance supplied by the review entry point. The workflow, filtering, and output below are defaults
where that guidance is silent. Keep repository-required findings through final synthesis; the one exception is a
recorded user-authorized divergence.

## Your task

1. Review {WHAT_WAS_IMPLEMENTED}
2. Compare against {PLAN_OR_REQUIREMENTS}
3. Check code quality, architecture, security, and testing, with the checklist below
4. Give each finding its severity
5. Run the lead-judgment pass over those findings (see below)
6. Derive the review verdict from the open tally

## What was implemented

{DESCRIPTION}

## Requirements/Plan

{PLAN_OR_REQUIREMENTS}

## Git range to review

**Base:** {BASE_SHA} **Head:** {HEAD_SHA}

```bash
git diff --stat {BASE_SHA}...{HEAD_SHA}
git diff {BASE_SHA}...{HEAD_SHA}
```

The three-dot form compares the head against its merge base, so changes made on the base after the branch point do not
show up as part of the change.

## Review checklist

**Code quality:** separation of concerns, error handling, type safety where it applies, duplication, edge cases.

**Architecture:** design decisions, performance implications, security concerns, scale where it matters to the change.

**Testing:**

- Tests exercise the logic, not only mocks, and cover edge cases; integration tests exist where needed.
- No check loosened and no test deleted to make the suite pass, as [../check-weakening.md](../check-weakening.md) says:
  Important with no stated reason, Minor with the reason quoted, and Important for a weakened red-commit test whatever
  the reason ([../check-weakening.md#red-commit-tests](../check-weakening.md#red-commit-tests)).

**Requirements:** every plan requirement met, the implementation matches the spec, no scope creep, breaking changes
documented.

**Production readiness:** a migration strategy for schema changes, backward compatibility, documentation.

## Severity and lead judgment

Give every finding one severity from [severity.md](../../../routing/references/contracts/severity.md): Critical,
Important, Minor, or Informational.

You are the only reviewer here, so nothing else filters your findings before the user reads them. Run the lead-judgment
pass from [../verdict-synthesis.md](../verdict-synthesis.md) over your own findings before you write the output:

1. **Filter.** Dismiss any finding you can show is factually wrong. Then test the rest against the five
   [dismissal filters](../verdict-synthesis.md#dismissal-filters), the recorded-divergence filter included. Mark a
   finding that fails one as Dismissed, with that reason on one line.
2. **Bucket.** Put every surviving finding in exactly one of **Act on**, **Consider**, or **Noted**. The bucket is your
   recommendation; it does not change the severity. A Critical or Important finding moves to Dismissed only with cited
   evidence that it is factually wrong, or when a recorded divergence covers it under the filter, with the record cited.
3. **Calibrate.** More than 5 Act-on items means you under-filtered. Refilter before you deliver. When more than 5 still
   survive with cited evidence, keep them all and say in one line why the count is high.

## The review verdict

The verdict follows from the open tally, as [severity.md](../../../routing/references/contracts/severity.md#verdicts)
defines it. An open finding is a Critical or Important finding that is not Dismissed, whatever its bucket.

| Open tally                                                     | Review verdict     |
| -------------------------------------------------------------- | ------------------ |
| No open Critical or Important finding                          | Approve            |
| Open Important findings only, each a bounded fix               | Approve with fixes |
| Any open Critical finding, or an Important that is not bounded | Request changes    |

A bounded fix changes neither the approach nor an interface.
[review-conduct-guidelines.md](review-conduct-guidelines.md#review-verdict-criteria) holds what each verdict means to
the author.

## Output format

First check the repository's local review skill for output instructions. When it has them, use its format for all
findings, and do not repeat the same findings in this format. For example, if it requests a Rule / Evidence / Source
table, put the findings in that table. Use the format below only when repository guidance leaves the output format
unspecified.

Each finding appears once, in its bucket, with its severity in front, its `file:line`, what is wrong, why it matters,
and how to fix it when that is not obvious.

```markdown
### Strengths

[What is well done, with file:line references.]

### Act on

[Findings the lead recommends fixing before the merge. Five or fewer, or one line saying why more.]

### Consider

[Legitimate points whose fix cost may outweigh the benefit now. The user's call.]

### Noted

[Valid but not actionable at this stage. Brief list.]

### Dismissed

[Every rejected finding, one line each, naming the filter or the evidence that rejected it. Never omitted.]

### Assessment

**Open:** <C> Critical, <I> Important, <M> Minor

**Review verdict:** Approve | Approve with fixes | Request changes

**Reasoning:** [1-2 sentences that name each blocker, or say that nothing blocks.]
```

## Example output

```text
### Strengths
- Clean database schema with proper migrations (db.ts:15-42)
- Error handling with fallbacks (summarizer.ts:85-92)

### Act on
- **Important** Date validation missing (search.ts:25-27). Invalid dates silently return no results. Fix: validate the
  ISO format and throw an error with an example.
- **Important** No --help in the CLI wrapper (index-conversations:1-31). --concurrency cannot be discovered. Fix: add a
  --help case with usage examples.

### Consider
- **Minor** No "X of Y" counter for long indexing runs (indexer.ts:130). A real gap, but it only shows on long runs.

### Noted
- **Informational** Excluded projects are hard-coded (config.ts:12); a config file would help portability later.

### Dismissed
- "summarize() should take an options object": I would have done it differently; the two current arguments cause no
  named failure.
- "fetchAll() could return null here": hypothetical; the only caller (indexer.ts:88) passes a checked array.

### Assessment

**Open:** 0 Critical, 2 Important, 1 Minor

**Review verdict:** Approve with fixes

**Reasoning:** The two Important findings (date validation, help text) are bounded fixes that must land before the
merge; neither changes the approach or an interface.
```
