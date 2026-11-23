# TDD Pitfalls

**Use this reference when:** you consider a shortcut around TDD, review test-first work, or check common mistakes.

## Why Order Matters

**"I'll write tests after to verify it works."** Tests written after code pass immediately. A test you never watched
fail proves nothing. It may test the wrong thing, mirror the implementation instead of the behavior, or miss edge cases
you forgot.

**"I already manually tested all the edge cases."** Manual testing is ad-hoc, not systematic. It leaves no record of
what you tested and nothing to re-run when code changes. The claim "it worked when I tried it" ≠ comprehensive.

**"Deleting X hours of work is wasteful."** Sunk cost fallacy: the time is already gone. The real waste is code that you
keep but cannot trust. Working code without real tests is technical debt.

**"TDD is dogmatic, being pragmatic means adapting."** TDD is pragmatic: it finds bugs before commit, prevents
regressions, documents behavior, and enables refactoring. Shortcuts move the debugging cost later.

**"Tests after achieve the same goals - it's spirit not ritual."** Tests after answer "What does this do?" and verify
edge cases you remembered. Tests-first answer "What should this do?" and force edge-case discovery before
implementation. Coverage without proof the tests work is not TDD.

## Common Rationalizations

| Excuse                                 | Reality                                                                 |
| -------------------------------------- | ----------------------------------------------------------------------- |
| "Too simple to test"                   | Simple code breaks. Test takes 30 seconds.                              |
| "I'll test after"                      | Tests that pass immediately prove nothing.                              |
| "Tests after achieve same goals"       | Tests-after = "what does this do?" Tests-first = "what should this do?" |
| "Already manually tested"              | Ad-hoc ≠ systematic. No record, can't re-run.                           |
| "Deleting X hours is wasteful"         | Sunk cost fallacy. Unverified code that you keep is technical debt.     |
| "Keep as reference, write tests first" | You'll adapt it. That's testing after. Delete means delete.             |
| "Need to explore first"                | Fine. Throw away the exploration. Start with TDD.                       |
| "Test hard = design unclear"           | Listen to the test. Hard to test = hard to use.                         |
| "TDD will slow me down"                | TDD faster than debugging. Pragmatic = test-first.                      |
| "Manual test faster"                   | Manual doesn't prove edge cases. You'll re-test every change.           |
| "Existing code has no tests"           | Your change improves it. Add tests for existing code.                   |

## Common Mistakes

Per-phase mistakes and fixes are canonical in the decision tables and checklists of
[tdd-phase-checklists](tdd-phase-checklists.md). Mock mistakes are in [testing-anti-patterns](testing-anti-patterns.md).

## Red Flags: Stop and Start Over

- Code before test
- Test after implementation of new behavior. A test for behavior that already shipped follows
  [Tests for existing behavior](../SKILL.md#tests-for-existing-behavior) instead.
- A new test passes immediately and you cannot say why (the cases are in
  [SKILL.md](../SKILL.md#phase-1-red-write-failing-test))
- Can't explain why test failed
- Tests added "later"
- The rationalization "just this once"
- "I already manually tested it"
- "Tests after achieve the same purpose"
- "It's about spirit not ritual"
- "Keep as reference" or "adapt existing code"
- "Already spent X hours, deleting is wasteful"
- "TDD is dogmatic, I'm being pragmatic"
- "This is different because..."

Each of these means: delete the code written before its test, and start that behavior again at RED. These are reasons
you give yourself. When the user declines the test for a change, that is their decision, handled as
[When the user declines a step](../SKILL.md#when-the-user-declines-a-step) says.
