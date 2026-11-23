# Confidence scoring and false-positive suppression

Use the repository guidance supplied by the review entry point. The workflow, rubric, filters, and output below are
defaults where that guidance is silent. Preserve the source paths of that guidance through delegation and synthesis.

The post pipeline scores issues and filters noise with this file. Interactive reviewers can use it to calibrate what is
and isn't a real issue.

## Confidence scoring rubric

Score each issue on one of the five anchors below, for confidence that it is real and impactful. A score between two
anchors is not used: pick the anchor whose description fits.

- **0, Not confident:**
    - False positive that doesn't stand up to light scrutiny, or a pre-existing issue.
- **25, Somewhat confident:**
    - Might be a real issue but may also be a false positive.
    - Could not verify.
    - If stylistic, not explicitly called out in the relevant repository guidance.
- **50, Moderately confident:**
    - Verified as a real issue, but may be a nitpick or rarely hit in practice.
    - Not very important relative to the rest of the PR.
- **75, Highly confident:**
    - Double-checked and verified as very likely real.
    - Will be hit in practice.
    - The existing approach in the PR is insufficient.
    - Directly impacts functionality, or explicitly mentioned in repository guidance.
- **100, Absolutely certain:**
    - Definitively confirmed as a real issue that will happen frequently in practice.
    - Evidence directly confirms this.

## Filtering threshold

**Default threshold: 75.** An issue scored 75 or 100 is kept; 0, 25, and 50 are dropped. The rubric has no anchor
between 75 and 100, so a higher threshold would keep only "Absolutely certain" issues. Apply the base revision's
reporting criteria when the repository supplies them. If nothing qualifies, report that no findings met the applied
criteria. Do not claim there are no issues.

A subagent #8 security candidate is scored with [../security/security-scoring.md](../security/security-scoring.md) and
filtered at the threshold that file sets.

For an issue flagged under repository guidance, the scorer must double-check that the guidance calls out that specific
issue.

The score is confidence, not severity. Each kept issue also carries its shared severity from
[severity.md](../../../routing/references/contracts/severity.md), as its reviewer assigned it.

## False-positive suppression list

These are the default suppression rules. Repository instructions that require reporting a category override them. Keep
demonstrated defects apart from suggestions.

**Exception: a diff that weakens a check.**

Items 4, 6, 9, 10, and 11 do not suppress a finding that this diff weakens a check. CI cannot catch a check the diff
turned off. Items 1, 7, and 8 do not suppress it either. The finding is the loosening this diff adds, not the older code
it hides. Score it by [../check-weakening.md](../check-weakening.md#weight-and-reporting): 100 when no reason is stated,
75 when a stated reason is quoted beside it. A stated reason never drops it below the threshold.

1. **Pre-existing issues.** Problems that existed before this PR.
2. **Not actually bugs.** Something that looks wrong but is correct behavior.
3. **Pedantic nitpicks.** Things a senior engineer wouldn't call out.
4. **Linter/typechecker/compiler-catchable issues.** Missing or incorrect imports, type errors, broken tests, formatting
   issues, and pedantic style issues like newlines.
    - Assume CI runs these separately.
5. **General code quality issues.** Lack of test coverage, general security concerns, and poor documentation, unless
   repository guidance explicitly requires them. Reviewer #7 runs only when it does, so its gaps are scored as findings.
   A security issue that reviewer #8 traced is a finding, not a general concern.
    - This rule does not suppress an unaddressed request from a human reviewer.
    - The request was made on a previous PR that touched the same code. It is the pipeline's Agent #4 signal.
    - A human already judged that gap worth fixing.
    - Score it as its own finding, even when it looks like a coverage gap.
6. **Silenced repository guidance issues.** Issues called out in repository guidance but explicitly silenced in code
   (e.g., lint ignore comments).
7. **Intentional functionality changes.** Changes in functionality that are likely intentional or directly related to
   the broader change.
8. **Issues on unmodified lines.** Real issues on lines the PR did NOT modify.
9. **Build/typecheck signals.** Leave these to CI unless repository guidance requests verification.
10. **Missing imports or broken tests.** Assume CI catches these.
11. **Formatting issues.** Linters handle style and formatting.
12. **Out-of-scope quality suggestions.** General improvements not related to the change at hand.
