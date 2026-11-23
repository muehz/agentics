# TDD Examples and Reference Tables

Worked full-cycle examples and quick-reference tables. See [SKILL.md](../SKILL.md) for the workflow.

## Example: New Feature

Two red-green-refactor cycles for a new feature:

```text
Behavior 1: "User login returns a session token"

RED:
  1. Write test: loginUser("valid@email.com", "password") → returns { token: string }
  2. Run the new test → FAIL (loginUser returns the stub's placeholder, not a token)
  3. Verify: the failure is at the assertion, not a syntax or import error ✓

GREEN:
  1. Implement loginUser() with a hardcoded token
  2. Run the new test and the tests that cover the login module → PASS ✓

REFACTOR:
  1. Extract the token literal into a named constant
  2. Run the covering tests → still green ✓

Behavior 2: "Two logins return different tokens"

RED:
  1. Write test: two loginUser() calls → tokens differ
  2. Run the new test → FAIL (the hardcoded token repeats) ✓

GREEN:
  1. Replace the hardcoded token with real token generation
  2. Run the covering tests → PASS ✓

REFACTOR:
  1. Extract token generation into a helper
  2. Run the covering tests → still green ✓

LAST CYCLE GREEN: run the full suite once → all green ✓
```

Real token generation is new behavior, so it arrives through its own failing test, not through Refactor.

## Example: Bug Fix

**Bug:** Empty input accepted

```text
RED
test("rejects empty input")
  result = submitForm({ field: "" })
  assert result.error == "Field required"

$ [project test command] <new test file>   # the new test only
FAIL: expected "Field required", got undefined

$ git add <new test file>
$ git commit -m "test: reproduce empty input accepted"   # the test alone, before any fix

GREEN
function submitForm(data):
    if not data.field.trim():
        return { error: "Field required" }
    ...

$ [project test command] <form tests>   # the new test and the tests that cover submitForm
PASS

REFACTOR
If needed, extract validation for multiple fields, then run the full suite once.
```

## Horizontal vs. Vertical Slices

Bulk-written tests assert imagined behavior and shape (signatures, data structures) instead of actual behavior, so they
pass when behavior breaks. Work in vertical slices: one test, one implementation, repeat.

```text
WRONG (horizontal):
  RED:   test1, test2, test3, test4, test5
  GREEN: impl1, impl2, impl3, impl4, impl5

RIGHT (vertical):
  RED→GREEN: test1→impl1
  RED→GREEN: test2→impl2
  ...
```

## When a Failing Test Is Impractical

A bad test mostly tests mocks, encodes implementation details instead of behavior, depends on timing or unrelated global
state, needs expensive infrastructure for a small change, or would be deleted right after it proves the fix. It passes
when behavior breaks, fails when behavior holds, and teaches the next reader the wrong contract.

Impractical means the only available test needs broad harness setup, brittle mocks, slow end-to-end infrastructure,
production-only state, a vague or irreproducible trigger, or large unrelated fixture churn—not merely that the test is
annoying to write.

## Good Tests

| Quality          | Good                                | Bad                                                 |
| ---------------- | ----------------------------------- | --------------------------------------------------- |
| **Minimal**      | One thing. "and" in name? Split it. | `test('validates email and domain and whitespace')` |
| **Clear**        | Name describes behavior             | `test('test1')`                                     |
| **Shows intent** | Demonstrates desired API            | Obscures what code should do                        |

## When Stuck

| Problem                | Solution                                                  |
| ---------------------- | --------------------------------------------------------- |
| Don't know how to test | Write the wished-for API. Write the assertion first. Ask. |
| Test too complicated   | Design too complicated. Simplify the interface.           |
| Must mock everything   | Code too coupled. Use dependency injection.               |
| Test setup huge        | Extract helpers. Still complex? Simplify the design.      |
