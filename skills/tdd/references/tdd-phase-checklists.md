# TDD Phase Checklists, Decision Criteria, and Anti-Patterns

Phase gates for the red-green-refactor cycle. See [SKILL.md](../SKILL.md) for the core workflow.

---

## Red Phase

### Test Examples

**Good**

```
test('retries failed operations 3 times', async () => {
  let attempts = 0;
  const operation = () => {
    attempts++;
    if (attempts < 3) throw new Error('fail');
    return 'success';
  };

const result = await retryOperation(operation);

expect(result).toBe('success');
expect(attempts).toBe(3);
});

```

Clear name, real behavior, one thing checked.

**Bad**

```
test('retry works', async () => {
const mock = jest.fn()
.mockRejectedValueOnce(new Error())
.mockRejectedValueOnce(new Error())
.mockResolvedValueOnce('success');
await retryOperation(mock);
expect(mock).toHaveBeenCalledTimes(3);
});

```

Vague name, and it checks the mock, not the retry behavior.

### Red Phase Checklist

```text
☐ Test describes exactly one behavior
☐ Test name clearly states the expected behavior
☐ Test follows AAA pattern (Arrange → Act → Assert)
☐ Test compiles/parses without errors (failure is at runtime/assertion level)
☐ Failure message confirms missing behavior, not broken test setup
☐ No production code was written or modified beyond the stub that lets the test reach its assertion
☐ Bug fix: the failing test landed before any fix code, as a red commit or a recorded failing run
☐ No mocks were added speculatively (mock only what is needed for isolation)
```

### Decision Criteria: Is the Red Phase Complete?

| Condition                                                                                            | Action                                                                                                                                                                                                    |
| ---------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Test fails with expected assertion error                                                             | ✅ Proceed to Green                                                                                                                                                                                       |
| Test fails with import/syntax/setup error in the test file                                           | ❌ Fix test infrastructure. Re-run the test. If the code under test does not exist yet, add the smallest stub that compiles. Make the stub return a placeholder value, so the test fails at its assertion |
| Test command itself fails to run (runner missing, config crash, no assertion output)                 | ❌ Fix test tooling. Re-run the test. A run that never executed is not a valid RED                                                                                                                        |
| Test passes immediately — trivial, tautological, or aimed at the wrong thing                         | ❌ The test does not test the intended behavior. Rewrite or discard it                                                                                                                                    |
| Test passes immediately — sound test, but a prior cycle's implementation already covers the behavior | ⚠️ Do not fabricate a Green phase. Disclose that RED was not observed for this criterion. Keep the test as a regression check. Proceed to Refactor                                                        |
| Test passes immediately — it covers behavior that already exists                                     | ⚠️ Break the code under test on purpose, see the test fail at its assertion, and revert the break ([SKILL.md](../SKILL.md#tests-for-existing-behavior))                                                   |
| Multiple behaviors in one test                                                                       | ❌ Split into separate tests, one per behavior                                                                                                                                                            |

[testing-anti-patterns](testing-anti-patterns.md) covers mock anti-patterns: testing mock behavior and speculative
mocking.

---

## Green Phase

### Implementation Examples

**Good**

```
async function retryOperation<T>(fn: () => Promise<T>): Promise<T> {
  for (let i = 0; i < 3; i++) {
    try {
      return await fn();
    } catch (e) {
      if (i === 2) throw e;
    }
  }
  throw new Error('unreachable');
}
```

Minimum implementation to pass the test.

**Bad**

```
async function retryOperation<T>(
  fn: () => Promise<T>,
  options?: {
    maxRetries?: number;
    backoff?: 'linear' | 'exponential';
    onRetry?: (attempt: number) => void;
  }
): Promise<T> {
  // Unrequested configuration adds complexity without a behavior to test.
}
```

Adds configuration and callbacks the failing test does not require.

### Green Phase Checklist

```text
☐ Only the code required by the failing test was written
☐ No speculative features, extra parameters, or premature abstractions added
☐ The previously failing test now passes
☐ The tests that cover the changed code still pass
☐ No test assertions were weakened or removed to force green
☐ Test intent is unchanged — code was fixed to match test, not the reverse
```

### Decision Criteria: Is the Green Phase Complete?

| Condition                                 | Action                                                     |
| ----------------------------------------- | ---------------------------------------------------------- |
| Failing test and covering tests pass      | ✅ Proceed to Refactor                                     |
| Failing test passes but other tests broke | ❌ Fix the regression. Do not expand scope                 |
| Test was modified to make it pass         | ❌ Revert the test change. Fix production code instead     |
| Implementation adds unrequested features  | ❌ Remove the extras. Implement only what the test demands |

---

## Refactor Phase

### Refactor Phase Checklist

```text
☐ The covering tests pass after every change
☐ No new behavior was added (no new assertions needed)
☐ Duplication reduced (DRY applied where appropriate)
☐ Method/variable names clearly express intent
☐ Long methods extracted into focused, named functions
☐ A missing check found here (input validation, error handling) is the next RED, not added now
☐ No test-only methods introduced in production classes
☐ Test code reviewed and cleaned up alongside production code
```

### Decision Criteria: Is the Refactor Phase Complete?

| Condition                                  | Action                                                                                                       |
| ------------------------------------------ | ------------------------------------------------------------------------------------------------------------ |
| Code is clean, tests green, no duplication | ✅ The cycle is complete. Return to Red for the next behavior; after the last cycle, run the full suite once |
| Tests broke during refactoring             | ❌ Revert the last change. Try a smaller refactoring step                                                    |
| New behavior is needed                     | ❌ Stop the refactor. Start a new Red phase                                                                  |
| Refactoring reveals a bug                  | ❌ Write a failing test for the bug (new Red phase). Then fix it                                             |

### Refactor Phase Anti-Patterns

- **Premature abstraction:** Do not introduce patterns (Strategy, Factory, etc.) until duplication demands it. Apply the
  rule of three. Tolerate duplication twice. Abstract on the third occurrence.
- **Ignoring test quality:** Duplicated setup, unclear names, and magic values are code smells. Refactor tests with the
  same rigor as production code.
