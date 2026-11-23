# Shared review principles

Every review mode loads this file. Repository guidance that the review entry point loads governs the review; this file
supplies defaults where that guidance is silent.

## Priority order

Review in this order of importance:

1. **Bugs.** Broken behavior, regressions, data loss, or missing requirements.
2. **Security.** Vulnerabilities, unsafe defaults, auth or secret handling issues.
3. **Correctness.** Logic errors, invalid assumptions, edge-case failures.
4. **Performance.** Meaningful latency, memory, scale, or cost risks.
5. **Maintainability.** Readability, coupling, duplication, testability, future change cost.
6. **Style.** Naming, formatting, or minor consistency issues that do not affect behavior.

Do not let style feedback crowd out higher-severity issues.

## Severity

Every finding carries one of the four severities in [severity.md](../../routing/references/contracts/severity.md):
Critical, Important, Minor, or Informational. A lane or reviewer may print its own label beside it, as that file's
producer mapping shows. Do not mark everything Critical; the severity follows the concrete failure the finding causes.

## Scope discipline

- Review what is in the diff and any directly affected surrounding context.
- Call out adjacent risks only when they are caused by, exposed by, or required to validate the change.
- Do not turn the review into a redesign of unrelated code.
- Note a separate problem as follow-up work. Do not block the current change on it, unless it makes the diff unsafe.

## Reviewer conduct

**Do:**

- Be specific: `file:line`, not "somewhere in the handler".
- Explain why each issue matters.
- Acknowledge strengths.
- Give a clear verdict where the mode has one.
- Keep a finding's severity where the evidence puts it. A request to lower it, without evidence that the finding is
  wrong, gets one line that says what evidence would move it. A repeated request gets a one-line pointer back to that
  line, never a new argument and never a quiet relabel or drop. The user may still merge over the finding; that decision
  is theirs.

**Don't:**

- Say "looks good" about code you did not check.
- Comment on code you did not review.
- Be vague ("improve error handling").

## Anti-patterns

- **Nitpicking.** A review full of low-value style comments that misses important issues.
- **Bike-shedding.** A debate over personal preferences with no clear engineering reason.
- **Rubber-stamping.** Approval with no reading, validation, or thought about the change.
- **Drive-by comments without context.** Vague criticism that comes with no evidence, explanation, or guidance.
