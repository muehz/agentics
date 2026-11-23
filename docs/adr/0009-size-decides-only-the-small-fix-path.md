# Size decides only the small-fix path, and only by its signals

> **Amended (2026-11-11, #1311) by [0013](0013-proportional-delivery-paths.md).** A one-task change that fails only the
> size signal now takes the middle path, and that record changes how the fifth signal is measured and who may write the
> fix.

> **Amended (2026-11-11, #1311) by [0016](0016-git-write-ownership.md).** A bug fix commits its failing test on its own
> unless a pre-commit hook rejects that commit, in which case the recorded failing run stands in for it.

> **Amended (2026-11-11, #1311) by [0017](0017-flow-state-v2.md).** Under the `local` storage setting a small fix's rows
> go to the clone-local store; under `committed`, the default, they stay in the committed `unassigned/state.tsv`.

> **Amended (2026-10-17, #1111).** The trail is now `state.tsv`, written by `flow`, so there is no month-level
> `events.tsv`. A small fix writes its rows to `<work-root>/unassigned/state.tsv`, and `gate branch` finds the
> `skip chain <branch>` row there (see `skills/execute/references/flow-state.md`). The next phase is read from the
> current row's `next`, and no header carries a `next-owner`. The chain-state code under `go/hooks/internal/chain` is
> gone. This record keeps its original wording for the decision it made.

Every change that could alter behavior used to open with `intent`, and intent's **When the chain is required** rule said
"Size does not decide this". A one-line fix to a default carried the same intent, spec, and plan as a feature. The rule
now reads "Size decides only the small-fix path, and only by its signals".

A mutating request is a small fix only when all five signals hold: the diff can be described in one sentence; it
addresses one concern; it changes no public interface, contract, trust boundary, or data handling; existing tests cover
the touched code, or the fix adds a regression test; and, from `git diff --numstat`, it changes at most 3 non-test files
and 50 non-test lines, with no non-test file more than half rewritten. `routing` announces the classification and every
signal. `execute fix` then builds a one-task packet with no work folder, a dispatched subagent implements it, the
controller verifies, and review runs. A bug fix still starts with `diagnose` and still commits its failing test on its
own. The trail goes to the month-level `<plan-parent>/YYYY-MM/events.tsv`: a `skip chain <branch>` row, `verify fix`,
`verify red-fix` for a bug fix, and the gate verdict. `gate branch` accepts the hand-off with no work folder only when
that `skip chain` row names the branch.

## Why

The pull request reviewer is always a second person, even though the in-flight session has none. That was the user's
reason (Q33). For a change a reviewer can read in one sitting, three artifacts and their approvals cost more than they
protect. The signals are stricter than the routine tier, and size is one of five: no change leaves the chain because it
is small alone.

## What this costs, stated plainly

**A small fix has no intent, spec, or plan.** Its reason lives in one trail row and the pull request description. A
later reader who wants more context reads the diff and the review, not a recorded problem statement.

**No second human sees the change before the pull request.** Inside the session, the independent checks are model
reviews and gate's audits.

**Signals 3 to 5 are a forecast when routing classifies.** A fix can grow after it starts. It is caught only when
`execute fix` checks the signals again on the real diff, and gate re-checks size. When `execute fix` catches it, the fix
stops and enters the chain.

**Small fixes share one month-level log.** Rows from different branches sit side by side, and gate tells them apart by
the branch named in `skip chain` and by which recorded commits the branch can reach.

**A new non-test file always fails the size signal.** It has no existing lines, so any change to it is more than half of
it.

## Considered and declined

**Keep "Size does not decide this".** The assistant argued that a skipped chain has no second reviewer. The user
rejected that, because the pull request reviewer is that second person (Q33).

**Routing dispatches the fix itself.** Declined because it duplicates `execute` (spec, Scope Decisions S1).

**A minimal work folder with a one-task plan.** Declined because it contradicts Q33 and adds folders that #1123 would
then have to consolidate (spec, Scope Decisions S1).

**File and line counts alone.** The user added the limit on rewriting more than half of a file ("what if those files are
completely rewritten"), spec Scope Decisions S2.

## Consequences

`routing` runs the small-fix check after phase selection picks `intent` for an item no chain artifact covers. `intent`'s
chain rule names the small-fix path. `execute` gains the `fix` entry. `gate branch` accepts a small-fix hand-off with no
work folder and checks its trail rows, the controller audit, the red commit for a bug fix, and the pull request
description. `chain-artifacts.md` carries one exception sentence for that hand-off, pinned by conformance. The
decision-trail verb set does not change: `skip` also covers a small fix leaving the chain.

Reverting this decision restores "Size does not decide this" in intent, removes the small-fix check from routing, the
`fix` entry from `execute`, the small-fix hand-off from `gate`, and the exception sentence with its pin. Rows a small
fix already wrote stay in the month-level logs, because the trail is append-only.
