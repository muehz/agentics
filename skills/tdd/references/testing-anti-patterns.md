# Testing Anti-Patterns

**Use this reference when:** you write or change tests, add mocks, or consider test-only methods in production code.

## Overview

**Core principle:** Test what the code does, not what the mocks do. Mocks isolate, but they are not the thing tested.

The TDD cycle helps prevent these anti-patterns.

## The Iron Laws

```text
1. NEVER test mock behavior
2. NEVER add test-only methods to production classes
3. NEVER mock without understanding dependencies
```

## Anti-Pattern 1: Testing Mock Behavior

**The violation:**

```typescript
// ❌ BAD: Testing that the mock exists
test('renders sidebar', () => {
  render(<Page />);
  expect(screen.getByTestId('sidebar-mock')).toBeInTheDocument();
});
```

**Why this is wrong:** It verifies that the mock exists, not that the component works. The test passes when the mock is
present and fails when it is not. It proves nothing about real behavior.

**Checkpoint question:** "Are we testing the behavior of a mock?"

**The fix:**

```typescript
// ✅ GOOD: Test real component or don't mock it
test('renders sidebar', () => {
  render(<Page />);  // Don't mock sidebar
  expect(screen.getByRole('navigation')).toBeInTheDocument();
});

// OR if sidebar must be mocked for isolation:
// Don't assert on the mock - test Page's behavior with sidebar present
```

### Gate Function

```text
BEFORE asserting on any mock element:
  Ask: "Am I testing real component behavior or just mock existence?"

  IF testing mock existence:
    STOP - Delete the assertion or unmock the component

  Test real behavior instead
```

## Anti-Pattern 2: Test-Only Methods in Production

**The violation:**

```typescript
// ❌ BAD: destroy() only used in tests
class Session {
    async destroy() {
        // Looks like production API!
        await this._workspaceManager?.destroyWorkspace(this.id);
        // ... cleanup
    }
}

// In tests
afterEach(() => session.destroy());
```

**Why this is wrong:** Test-only code in a production class is dangerous if anything calls it in production by accident.
It violates YAGNI and separation of concerns. It also confuses object lifecycle with entity lifecycle.

**The fix:**

```typescript
// ✅ GOOD: Test utilities handle test cleanup
// Session has no destroy() - it's stateless in production

// In test-utils/
export async function cleanupSession(session: Session) {
    const workspace = session.getWorkspaceInfo();
    if (workspace) {
        await workspaceManager.destroyWorkspace(workspace.id);
    }
}

// In tests
afterEach(() => cleanupSession(session));
```

### Gate Function

```python
BEFORE adding any method to production class:
  Ask: "Is this only used by tests?"

  IF yes:
    STOP - Don't add it
    Put it in test utilities instead

  Ask: "Does this class own this resource's lifecycle?"

  IF no:
    STOP - Wrong class for this method
```

## Anti-Pattern 3: Mocking Without Understanding

**The violation:**

```typescript
// ❌ BAD: Mock breaks test logic
test("detects duplicate server", () => {
    // Mock prevents config write that test depends on!
    vi.mock("ToolCatalog", () => ({
        discoverAndCacheTools: vi.fn().mockResolvedValue(undefined),
    }));

    await addServer(config);
    await addServer(config); // Should throw - but won't!
});
```

**Why this is wrong:**

- The mocked method had a side effect that the test depended on (writing config)
- Over-mocking to "be safe" breaks actual behavior
- The test passes for the wrong reason or fails mysteriously

**The fix:**

```typescript
// ✅ GOOD: Mock at correct level
test("detects duplicate server", () => {
    // Mock the slow part, preserve behavior test needs
    vi.mock("MCPServerManager"); // Just mock slow server startup

    await addServer(config); // Config written
    await addServer(config); // Duplicate detected ✓
});
```

### Gate Function

```text
BEFORE mocking any method:
  STOP - Don't mock yet

  1. Ask: "What side effects does the real method have?"
  2. Ask: "Does this test depend on any of those side effects?"
  3. Ask: "Do I fully understand what this test needs?"

  IF depends on side effects:
    Mock at lower level (the actual slow/external operation)
    OR use test doubles that preserve necessary behavior
    NOT the high-level method the test depends on

  IF unsure what test depends on:
    Run test with real implementation FIRST
    Observe what actually needs to happen
    THEN add minimal mocking at the right level

  Red flags:
    - "I'll mock this to be safe"
    - "This might be slow, better mock it"
    - Mocking without understanding the dependency chain
```

## Anti-Pattern 4: Incomplete Mocks

**The violation:**

```typescript
// ❌ BAD: Partial mock - only fields you think you need
const mockResponse = {
    status: "success",
    data: { userId: "123", name: "Alice" },
    // Missing: metadata that downstream code uses
};

// Later: breaks when code accesses response.metadata.requestId
```

**Why this is wrong:** Partial mocks hide structural assumptions, so downstream code that depends on omitted fields
fails silently. Tests pass while integration fails, so they give false confidence.

**The Iron Rule:** Mock the COMPLETE data structure, not just the fields your test uses.

**The fix:**

```typescript
// ✅ GOOD: Mirror real API completeness
const mockResponse = {
    status: "success",
    data: { userId: "123", name: "Alice" },
    metadata: { requestId: "req-789", timestamp: 1234567890 },
    // All fields real API returns
};
```

### Gate Function

```python
BEFORE creating mock responses:
  Check: "What fields does the real API response contain?"

  Actions:
    1. Examine actual API response from docs/examples
    2. Include ALL fields system might consume downstream
    3. Verify mock matches real response schema completely

  Critical:
    If you're creating a mock, you must understand the ENTIRE structure
    Partial mocks fail silently when code depends on omitted fields

  If uncertain: Include all documented fields
```

## Anti-Pattern 5: Integration Tests as Afterthought

**The violation:**

```text
✅ Implementation complete
❌ No tests written
"Ready for testing"
```

**Why this is wrong:** Testing is part of implementation, not an optional follow-up. Do not claim that work is complete
until the red-green-refactor cycle has run (see [SKILL.md](../SKILL.md)).

## Anti-Pattern 6: Test Interdependence

**The violation:** One test passes alone but fails after another test mutates shared state, global config, or execution
order.

**Why this is wrong:**

- Order-dependent tests hide real failures
- Parallel runs expose random breakage
- Debugging becomes about sequence, not behavior

**The fix:** Reset all mutable state between tests. Isolate fixtures. Prove the suite passes in random or parallel
order.

## Anti-Pattern 7: Overmocking

**The violation:** A test mocks so many collaborators that it only proves the mock graph was wired together the way the
test expected.

**Why this is wrong:**

- The test verifies assumptions about internals, not user-visible behavior
- Refactors break tests even when behavior stays correct
- Real integration failures stay invisible

**The fix:** Prefer real collaborators inside the process boundary. Mock only slow, external, or nondeterministic
dependencies. Mock them only at the narrowest seam.

## Anti-Pattern 8: Assertion-Free Tests

**The violation:** A test calls code, awaits it, and ends. It checks nothing meaningful.

```typescript
// ❌ BAD
test("creates account", async () => {
    await createAccount(input);
});
```

**Why this is wrong:** A test that only runs the code does not verify it. A test without assertions can pass while the
feature is broken.

**The fix:** Assert on outcomes that matter: return values, persisted state, emitted events, visible UI, or thrown
errors.

## Anti-Pattern 9: Flaky Tests

**The violation:** Tests rely on timing guesses, ordering assumptions, real network calls, wall-clock time, or unstable
external state.

**Why this is wrong:**

- Failures are no longer trustworthy
- Teams re-run tests, and the failures stay unfixed
- CI time rises while confidence falls

**The fix:** Do all of these:

- Control time.
- Remove network dependence.
- Use condition-based waiting.
- Make setup deterministic.

Treat every flake as a real defect.

## Anti-Pattern 10: Implementation Coupling

**The violation:** Tests assert private method calls, exact helper counts, or internal data shapes that users never see.

**Why this is wrong:** A refactor that changes no behavior should not require large rewrites of the test suite. Behavior
is the contract, but implementation details are not.

**The fix:** Assert on public APIs and observable outcomes. If a test breaks after a safe refactor, ask whether the test
was coupled to the wrong thing.

## Anti-Pattern 11: Giant Test Methods

**The violation:** One test verifies many behaviors, branches, and edge cases at once.

**Why this is wrong:**

- Failures are hard to diagnose
- Setup becomes bloated and repetitive
- One broken assertion can hide others

**The fix:** Split by behavior. Keep each test focused enough that one failure tells one story. Accept several short
tests instead of one huge one.

## Anti-Pattern 12: Missing Edge Cases

**The violation:** Only the happy path is tested. Boundary values, null or empty inputs, errors, retries, and permission
failures are ignored.

**Why this is wrong:** Production issues usually live in the edges, not the demo path.

**The fix:** For every new behavior, deliberately ask: what are the minimum, maximum, empty, invalid, duplicate,
timeout, and failure cases? Add the ones that can break real users.

## Anti-Pattern 13: Snapshot Overuse

**The violation:** Large snapshots get updated automatically without review, so "snapshot updated" becomes meaningless
churn.

**Why this is wrong:**

- Reviewers no longer read snapshot diffs
- Unintended behavior changes slip through
- Tests become approval rubber stamps

**The fix:** Keep snapshots small and intentional. Prefer explicit assertions for critical behavior. Review every
snapshot diff as if it were hand-written code.

## Anti-Pattern 14: Test Data Coupling

**The violation:** Tests share mutable fixtures, database rows, or filesystem paths, so one test changes the starting
conditions for another.

**Why this is wrong:** Shared test data creates hidden dependencies and makes local reproduction unreliable.

**The fix:** Create fresh data per test. Use factories or builders. Clean up aggressively. If two tests need the same
data shape, share the factory, not the instance.

## When Mocks Become Too Complex

**Warning signs:**

- Mock setup is longer than the test logic
- Everything is mocked to make the test pass
- The mocks lack methods that the real components have
- The test breaks when the mock changes

**Checkpoint question:** "Do we need to be using a mock here?"

**Consider:** Integration tests with real components are often simpler than complex mocks.

## Quick Reference

| Anti-Pattern                    | Fix                                                     |
| ------------------------------- | ------------------------------------------------------- |
| Assert on mock elements         | Test real component or unmock it                        |
| Test-only methods in production | Move to test utilities                                  |
| Mock without understanding      | Understand the dependencies first. Mock minimally.      |
| Overmocking                     | Keep real collaborators where behavior matters          |
| Incomplete mocks                | Mirror real API completely                              |
| Test interdependence            | Isolate state. Make order irrelevant.                   |
| Assertion-free tests            | Assert on observable outcomes                           |
| Flaky tests                     | Remove nondeterminism and timing guesses                |
| Implementation coupling         | Test public behavior, not private structure             |
| Giant test methods              | Split by behavior                                       |
| Missing edge cases              | Add boundary, error, and null coverage                  |
| Snapshot overuse                | Prefer explicit assertions. Review every snapshot diff. |
| Test data coupling              | Create fresh data per test                              |
| Tests as afterthought           | Use TDD. Write tests first.                             |
| Over-complex mocks              | Consider integration tests                              |

## Red Flags

- An assertion checks for `*-mock` test IDs
- Methods that only test files call
- Mock setup is more than 50% of the test
- The test fails when you remove the mock
- You cannot explain why the test needs the mock
- You mock "just to be safe"
- The test passes alone but fails in suite order
- The test body performs work without meaningful assertions
- The snapshot update is larger than the behavior change
- Other tests mutate shared fixtures or rows
