---
name: tdd
description: "Use when asked to implement a change test-first or add tests that prove behavior. Start with a meaningful failing test, make the smallest implementation change that passes it, and refine the design while preserving the evidence."
version: "2.2.0"
user-invocable: true
disable-model-invocation: false
argument-hint: "[feature or bug description]"
metadata:
    surfaces: [claude-code, copilot-cli, vscode, codex, antigravity]
---

# Test-Driven Development (TDD)

Write the test first. Watch it fail. Write minimal code to pass. If you did not watch the test fail, you do not know if it tests the right thing.

## Don't use when

Before forwarding, classifying a delivery path, or handling a gate decline, load [forwarding.md](../routing/references/forwarding.md), [chain-phases.md](../routing/references/chain-phases.md), and [gates.md](../routing/references/contracts/gates.md) (and [tdd-phase-checklists.md](references/tdd-phase-checklists.md) before running the cycle). Read the rows in order; the first match wins. Each row is marked with its kind, as [forwarding.md](../routing/references/forwarding.md#explicit-invocation) defines it. A `cannot` row forwards by that protocol. A `better elsewhere` row, under an explicit invocation of this skill, gets one line naming the better route, and then this skill runs.

| Order | Input signal                                                                                                        | Use this instead                                                                                                                                                                                                         | Kind             |
| ----- | ------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ---------------- |
| 1     | A bug whose root cause is unknown or unconfirmed: competing hypotheses, or an intermittent reproduction             | `diagnose`, with the symptom, the timing window, the evidence and the hypotheses already tested. RED needs a confirmed cause to reproduce.                                                                               | cannot           |
| 2     | An audit of existing tests for assertions that survive the defects they should catch, when the audit is the request | `zealot` tests mode. It proves each finding by mutation; this skill writes the replacement tests.                                                                                                                        | cannot           |
| 3     | A change with no runtime behavior: documentation or formatting only                                                 | Documentation: `docs`, which checks it against the implementation. Formatting only: make the edit directly; no skill has a workflow for it ([forwarding.md](../routing/references/forwarding.md#target-classification)). | cannot           |
| 4     | A config file, a generated file, or a prototype labeled throwaway                                                   | Make the edit directly. The artifact does not ship as code, so a test adds little.                                                                                                                                       | better elsewhere |
| 5     | An exploratory spike whose goal is to learn, not to ship                                                            | Explore without this skill, then discard the spike. Code that ships from what it taught is rebuilt here, test-first.                                                                                                     | better elsewhere |
| 6     | Otherwise                                                                                                           | This skill runs. [Entry and delivery path](#entry-and-delivery-path) decides what the work runs under.                                                                                                                   | —                |

## Entry and delivery path

This skill runs every request to implement a change test-first or add tests. The delivery path ([chain-phases.md](../routing/references/chain-phases.md#small-fix-classification)) decides what wraps the work (branch, commits, review, approvals, `gate` hand-off), never whether TDD runs. Read rows in order; first match wins.

| Order | The work arrives                                                              | What runs                                                                                                                                                                                                                                                                                                                                                                                                            |
| ----- | ----------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1     | From `execute`: a task of an approved plan, an approved brief, or a small fix | Caller holds the path. Run [Step 0](#step-0-preflight) and the cycle on its packet, returning what it asks for.                                                                                                                                                                                                                                                                                                      |
| 2     | Existing work covers the item                                                 | Unless a `routing` handoff carries results, run the [continuation check](../routing/references/chain-phases.md#continuation-check): related work on another branch, PR, or work folder selects `recall`. A covering flow selects its phase via [Selecting the phase](../execute/references/flow-state.md#selecting-the-phase) (a `next` of `gate` is offered, not run). Announce and run it (`execute` applies TDD). |
| 3     | With a small-fix outcome, from `routing` or row 5                             | Take the supplied small-fix path directly: state only the one-line small-fix outcome without re-classifying or offering another delivery path. Invoke `execute fix` with the announced classification and evidence; it holds branch, trail, red commit, review, and `gate` hand-off while applying this skill's cycle.                                                                                               |
| 4     | With a `brief` or full-chain outcome, from `routing` or row 5                 | Needs approved brief or approved intent/spec/plan before any test or code. Announce the path and forward to `intent` with the outcome and test-first constraint ([forwarding.md](../routing/references/forwarding.md#a-auto-forward-announce-then-run)); `execute` applies TDD once approved.                                                                                                                        |
| 5     | Otherwise: no path attached, and continuation check found no work             | Run [small-fix classification](../routing/references/chain-phases.md#small-fix-classification), announce the outcome in one line, then take row 3 or 4.                                                                                                                                                                                                                                                              |

Adding tests for existing behavior changes no non-test file (usually small-fix) and follows [Tests for existing behavior](#tests-for-existing-behavior). Declining a test is a process gate ([When the user declines a step](#when-the-user-declines-a-step)).

## Step 0: Preflight

Track Step 0, Phases 1–3, and the Verification Checklist under [workflow steps](../routing/references/contracts/workflow-steps.md). Then:

1. **Behavior change?** New feature, bug fix (RED reproduces the bug), changed behavior, refactor (characterization tests first if missing), or tests for existing behavior. Anything else matches [Don't use when](#dont-use-when).
2. **Acceptance criteria exist?** Require at least one concrete, checkable statement carrying every parameter the first test needs (scope, threshold/window, expected response/error; a bare goal like "rate-limit uploads" is not a criterion).
    - **Sourced:** taken from a cited source (existing test, API client, documented behavior, published schema, approved plan/brief, dispatch prompt). Cite the source.
    - **Open:** unsourced values must be asked in one question (or bug reproduction requested); never guess a contract.
    - **Dispatched:** dispatcher answers ([unattended.md](../routing/references/contracts/unattended.md)). Start RED only on sourced criteria, write nothing depending on an open value, and return open values as assumptions to confirm; if nothing is sourced, return draft criteria and stop before RED. Urgency never changes the contract.
3. **Existing test?** Run and read any ambiguous existing test (skipped, empty, "might be passing"); never adopt its title as the spec. A skipped or assertion-free test is not a valid RED: unskip/rewrite or discard and write fresh in Phase 1.
4. **Principles.** Apply [principles check](../routing/references/contracts/principles-check.md) when a relevant artifact exists.

Skill text uses Claude Code tool names; on other surfaces see [surface-dispatch.md](../routing/references/surface-dispatch.md#tool-names).

## The Iron Law

```text
NO PRODUCTION CODE WITHOUT A FAILING TEST FIRST
```

Delete code written before its test—never keep, adapt, or consult it; implement fresh from the test. Once TDD runs a change, it shapes the whole build (production code included) and is not split with another implementer. The Iron Law is a process gate ([gates.md](../routing/references/contracts/gates.md#process-gate); see [When the user declines a step](#when-the-user-declines-a-step)).

**Vertical slices, not horizontal batches.** Never write all tests first, then all implementation; work one test → one implementation at a time so each cycle informs the next ([tdd-examples.md](references/tdd-examples.md#horizontal-vs-vertical-slices)).

## Red-Green-Refactor

**Which tests run.** In RED run the new test alone; in GREEN and after each refactor run the new test plus tests covering changed code. Run the full suite once after the last cycle is green (in a shared-tree `execute` wave, the controller runs the full suite on the committed snapshot). Use the [verification command](../routing/references/repo-conventions.md#verification-command) when it runs tests; otherwise discover the test command from manifests, `Makefile`, `justfile`, or CI config (ask if ambiguous), and follow [Running it](../routing/references/repo-conventions.md#running-it).

### Phase 1 RED: Write Failing Test

1. **Pick the next behavior** from acceptance criteria.
2. **Write one descriptive test** (`should_return_error_when_input_is_empty`, not `test1`).
3. **Run it** per [Running it](../routing/references/repo-conventions.md#running-it) (on someone else's branch, show command and ask first). It MUST fail.
4. **Verify failure reason.** Failure must prove missing behavior—not syntax, import, missing fixture, or runner errors. If code under test does not exist yet, add the minimal compiling stub (placeholder return) so the assertion fails.
5. **Bug fix: failing test lands first.** Commit test files alone before writing the fix under the [commit contract](../git/references/commit.md#the-commit-contract). Shape the test completely first—later edits to red-committed test files are an Important review/`gate` finding, Refactor included.
    - If a pre-commit hook rejects the failing test, make no red commit and never bypass the hook; record command, exit status, and failing output excerpt as red evidence, and land the test with the fix.
    - Under `execute` in a shared worktree, stop after RED and return test files + failing output for the controller to commit. In an isolated worktree, commit and return `Red: <sha>`. In `pair` or a small fix, commit and report the SHA.

**Passes immediately?** Trivial/wrong target → rewrite or discard. Sound test already covered by an earlier cycle or existing behavior → prove it can catch a defect with a deliberate, temporary break per [Tests for existing behavior](#tests-for-existing-behavior) (record the break and failing assertion output, revert immediately, and confirm it passes on restored code before Refactor); only when no temporary break is feasible, disclose RED not observed, keep as a regression check, and record it as an exception in the Completion Evidence Report.

**Phase exit:** expected assertion failure ([tdd-phase-checklists.md](references/tdd-phase-checklists.md#red-phase)).

### Phase 2 GREEN: Make the Test Pass

1. **Read the failing test** and failure output.
2. **Write the minimum code** to pass (hardcoded values are fine; add no scope; never weaken the test).
3. **Run the new test and covering tests.** All must pass; claim only tests actually run.

**Phase exit:** new and covering tests pass ([tdd-phase-checklists.md](references/tdd-phase-checklists.md#green-phase)).

### Phase 3 REFACTOR: Improve Design

1. **Review** Green code for duplication, unclear names, long methods, and structure.
2. **Apply one refactoring at a time**, re-running covering tests after each; revert and take smaller steps if any fail.
3. **Refactor tests too** (dedupe setup, extract fixtures, name helpers)—except red-committed bug-fix test files.
4. **Stop** when clean and green; add no behavior (new behavior starts the next RED).

**Phase exit:** clean code, passing tests, no duplication ([tdd-phase-checklists.md](references/tdd-phase-checklists.md#refactor-phase)).

### Repeat

One behavior per cycle; never batch RED/GREEN. The user may batch Refactor across adjacent cycles, and a no-op Refactor is fine, but deferred refactoring must run with passing tests before completion. After the last cycle, run the full suite once. Full-cycle bug-fix and new-feature examples: [tdd-examples.md](references/tdd-examples.md).

## Tests for existing behavior

A test for already-working behavior cannot fail against current code. Make one deliberate, temporary break in the code under test (invert a condition, return a wrong value), run the test to confirm it fails at its assertion, and immediately revert the break (never commit it). If it still passes against the break, rewrite it. Record the break and failing output as failing-before evidence, run against restored code to see it pass, then proceed to Refactor.

## When a Failing Test Is Impractical

**Prefer no new test over a bad test.** A test path is **impractical** when the only test requires broad harness setup, brittle mocks, slow E2E infra, production-only state, an irreproducible trigger, or large unrelated fixture churn ("annoying to write" is not impractical; examples in [tdd-examples.md](references/tdd-examples.md#when-a-failing-test-is-impractical)). Then:

1. **State why before writing production code**, name the substitute check, and ask the user once to confirm. If the user answers no, stop before production code and ask whether to build the test harness, narrow scope, or defer the task. In a dispatched run with an active dispatcher, return the confirmation question to the dispatcher; in any [unattended](../routing/references/contracts/unattended.md) run where no one will answer, proceed only on a substitute observed failing before and passing after, recorded as "impractical test, unconfirmed" (otherwise stop and report why).
2. **Run the closest executable substitute** (targeted script, repro command, snapshot diff, log assertion, focused integration check), observing failure before and pass after; record both runs and list the exception in handoff.
3. **Broader failure class:** land the focused check first and handle sibling coverage separately. Re-enter Phase 1 if a practical path appears mid-work.

`gate` reads the substitute from the trail: confirmed substitutes appear as `substitute for <task>: <check>` ([Step 7](../gate/references/branch/review-and-audit.md#step-7-verify-tests)); `"impractical test, unconfirmed"` surfaces as a `trail` finding to fix (with a real failing test), accept, or defer ([Step 8](../gate/references/branch/review-and-audit.md#step-8-review-and-audit)).

## Completion Evidence Report

Report evidence, not outcomes alone:

- **Failing-before artifact:** failing test or substitute check with its failure output, or the deliberate break for existing behavior.
- **Passing-after run:** same artifact passing, full-suite result, and any nearby checks (types, lint) run.
- **Bug-fix red evidence:** red commit SHA, or recorded failing run when a hook rejected the commit.
- **Exceptions and reasons:** declined test, impractical-test substitute, or RED not observed due to prior cycle coverage.

## Verification Checklist

Before marking work complete:

- [ ] Every new function or method has a test
- [ ] Watched each test fail first for the expected reason (missing behavior, not typo)
- [ ] Bug fix: failing test landed before fix code (red commit or recorded failing run)
- [ ] Minimal code written to pass each test
- [ ] Full suite ran once after the last cycle and passed with pristine output
- [ ] Real code tested (mocks only if unavoidable); edge cases and errors covered

An exception documented in the Completion Evidence Report satisfies its box; any box neither true nor disclosed means work is incomplete.

## Testing Anti-Patterns

If adding mocks or test utilities, read [testing-anti-patterns.md](references/testing-anti-patterns.md) first: never test mock behavior, and never add test-only methods to production classes. Quality rubric (`Good Tests`) and troubleshooting (`When Stuck`) tables live in [tdd-examples.md](references/tdd-examples.md#good-tests).

## When the user declines a step

Writing the test first, deleting code written before its test, and running Refactor are process gates ([gates.md](../routing/references/contracts/gates.md#process-gate)). On a decline (e.g., "ship my fix now, test later"), state its cost once in one sentence—the change ships with nothing that watched it fail, so a wrong fix looks like a right one—and offer the one failing test you would write first and how long it takes. On the next reply that still declines (any wording), honor it. On the small-fix path, small-fix [signal 4](../routing/references/chain-phases.md#small-fix-signals) then requires an existing covering test or the closest executable check observed failing before and passing after; if neither exists, the work moves to `intent`. Roles or titles change nothing.

The delivery-path skill records one `waive` row per decline—`waive test-first`, `waive delete-untested`, or `waive refactor` ([flow-state.md](../execute/references/flow-state.md#events); a single decline covering both test-first and keeping an already-written fix records one `waive test-first` row). Write it when this session owns `state.tsv`, otherwise note it for the delivery-path skill without claiming it is already written, cite it in the Completion Evidence Report, and continue any remaining undeclined cycle steps.

**Safety gate (holds):** never report a test as passing, a suite as green, or work as done unless this run executed it and saw the result. If asked without the run, state once what is held (status claim), the harm (the next person builds on an unseen result), and the alternative (run it now, or report it as not run).

## References

- [tdd-examples.md](references/tdd-examples.md): bug-fix and new-feature full-cycle examples, vertical-slice rule, impractical-test breakdown, good-tests rubric, and when-stuck table.
- [tdd-phase-checklists.md](references/tdd-phase-checklists.md): phase exit criteria and checklists.
- [tdd-pitfalls.md](references/tdd-pitfalls.md): rationalizations and red flags.
- [testing-anti-patterns.md](references/testing-anti-patterns.md): mock and test-utility guidance.
- [forwarding.md](../routing/references/forwarding.md): route-away protocol for `cannot` rows and entry row 4.
- [chain-phases.md](../routing/references/chain-phases.md#small-fix-classification): small-fix classification and delivery paths.
- [gates.md](../routing/references/contracts/gates.md) and [unattended.md](../routing/references/contracts/unattended.md): gate classes and unattended runs.
- [commit.md](../git/references/commit.md#the-commit-contract): commit contract, including red commits and hooks.
