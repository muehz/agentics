# Verification before a completion claim

This file is the evidence standard behind **Verify before claim** in
[../SKILL.md](../SKILL.md#cross-cutting-rules-all-four-modes). A claim of status with no evidence from this session is a
safety gate: no request in the session turns an unverified check into a passed one.

## The standard

Before stating that something passed, succeeded, or is done:

1. Name the command whose output proves the claim.
2. Run the whole command in this session, fresh.
3. Read the full output: the exit code and the failure count.
4. State the claim with that evidence, or state the actual status with that evidence.

The standard covers paraphrases too. "Should pass", "probably works", and "looks done" are claims, and need the same
evidence or an explicit "not verified".

| Claim                 | Evidence                                  | Not evidence                  |
| --------------------- | ----------------------------------------- | ----------------------------- |
| Tests pass            | the test command's output with 0 failures | an earlier run, "should pass" |
| Linter clean          | the linter's output with 0 errors         | a partial check               |
| Build succeeds        | the build command's exit 0                | a linter pass, plausible logs |
| Bug fixed             | the original symptom's test now passes    | a changed line, an assumption |
| Regression test works | the test seen failing, then passing       | one passing run               |
| Delegated work done   | the diff and the checks, read by the gate | the worker's "success"        |
| Requirements met      | a line-by-line check against the spec     | passing tests alone           |

A regression test proves something only after it was seen to fail. The red-green check is: run the test with the fix
reverted and see it fail, then with the fix and see it pass.

## Claims handed in

A claim someone else makes ("CI passed", "the CAB approved it") is recorded as passed or approved only with a pointer: a
run link, a check's output, or an approval record. A pointer satisfies the check it points at and no other. A CI run or
a test result is never a review.

## Reporting what was not verified

- Name every check that did not run, and why. Never leave it out.
- Report the command, the exit code, and the output that support each claim.
- Before calling verification complete, look for what contradicts it: a failing check, a requirement with no coverage, a
  partial test scope, or a warning that was discounted.
- Absence of evidence is not evidence of absence. Running out of time is a reason to report "not verified", never a
  reason to report "verified".

## Checking against the repository's standards

When verification includes the project's coding standards, read every applicable instruction in `AGENTS.md`,
`.github/copilot-instructions.md`, and the repository's instruction directories, and compare the code with them. Report
each deviation as a finding with its file and line. Gate never refactors or fixes code: a fix goes to `execute`, or to
`execute fix` for a small fix.
