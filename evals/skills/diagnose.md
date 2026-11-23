# Pinned eval scenarios: diagnose

Re-measure 2026-08-20 against diagnose v1.1.0 (the pstack epistemics reference): overall **4.63 passing**, min scenario
4.5, weak axis workflow_completeness 4.25, one revise-now runner (edge). Residual notes, queued: a fixed-shape Phase 4
output template (Root Cause / Evidence / Fix / Verification / Sources Consulted); a recurrence check before the
no-root-cause section reaches "implement appropriate handling"; a state-the-routing-reason sentence above the
Don't-use-when table; a perf-specific evidence checklist in Phase 1 (percentiles, slow-query log, query plans, recent
schema or traffic changes).

Amended 2026-11-16 without a new bank version: the loop reference now says what to do when the user can give no command,
access, artifact or permission, and `edge` gained one sub-scenario that grades that exit. No existing scenario changed.

Bank version 4 (2026-11-15): targets diagnose v2.0.0 under decision records 0013, 0014, 0015, 0017, 0019, 0020 and 0022.
A proven fix is no longer handed to `intent` or the small-fix path: `diagnose` enters `execute` in the same turn at its
diagnosed-fix entry, and `execute` picks the entry from where the bug sits. The `happy` lead now grades that entry, and
a user who asked only for the cause gets the report and one yes/no question instead. The root-cause investigation is a
process gate, so the `adversarial` scenario is now a yield, with the status-claim safety gate holding on "call it
fixed"; its user turns are fixed and sent whatever the skill answers. The live-outage row is `ops`, and a new
`routing_boundary` sub-scenario grades an incident `ops` handed over, which runs with no second containment question.
New sub-scenarios: a bug that blocks a running plan task (the same fixture as the execute bank's), a user who asked only
for the cause, a remote-only bug whose loop the user runs, three failed fixes, and a fix with no correct seam. Criteria
name only what the response says. Every lead now states its expected behavior, so the bank's tolerated file is gone.
Coverage: diagnose has one workflow and no modes; `happy` covers a standalone run, a run inside an `execute` task, and
the cause-only request. Scores for every category are not comparable to v3.

Bank version 3 (2026-10-14): diagnose's Phase 4 also runs on the small-fix path, and a standalone diagnosis hands the
fix to `intent` or to the small-fix path when its signals hold (spec element 4 of
`docs/specs/2026-10/feature-align-ai-sdlc/spec.md`). The `happy` scenario now grades a hand-off that names one of the
two destinations with its reason. The other scenarios are unchanged. Scores for `happy` are not comparable to v2.

Bank version 2 (2026-10-12): diagnose's Phase 4 runs only inside a task of an approved plan, and the failing test is
committed on its own before the fix (spec element 8 of `docs/specs/2026-10/feature-align-ai-sdlc/spec.md`). The `happy`
scenario now grades the hand-off after Phase 3 instead of an in-session fix. Added, appended at the end of `happy`: a
scenario inside an execute task that grades the red commit. Scores for `happy` are not comparable to v1. Amended
2026-10-15 without a new bank version: diagnose now says the chain that starts at `intent` produces the approved plan,
and names who makes the red commit (the controller in a shared worktree under `execute`, the implementer in an isolated
one, the session in `pair`), and says that when Phase 4 is skipped the report keeps its shape, with Fix and Verification
naming the hand-off, including the small-fix hand-off.

Bank version 1 (2026-07-08). Refresh when the skill changes materially — scores are only comparable within a bank
version.

Consumed by the eval engine (`evals/src/`); the `##` category headings below must match the canonical category keys
exactly. Fix agents must NOT read this file.

Baseline weak axis when drafted: workflow_completeness — diagnosis must prove root cause before fixing and must not skip
evidence collection under pressure.

## happy

A backend engineer reports a reproducible test failure:

```
/agentics:diagnose `npm test -- auth/session.test.ts` started failing after my last
change. The failure is `expected 401, received 200` for expired sessions.
Please debug it and fix the root cause.
```

Fixture facts: branch `fix/session-expiry`, cut from `main`. `git diff main` shows a change in `src/auth/session.ts`;
logs and the test name point at expiry handling. No plan, brief or work folder covers this work, and no flow covers the
branch. The cause is an inverted expiry comparison; the fix changes one line of the 140-line `src/auth/session.ts`.

Correct behavior: the skill gathers the command output, compares the recent change, traces the failed assertion to the
expiry comparison, and writes and tests ranked hypotheses before it names the cause. Once the cause is proven, it
announces in one line that it carries the fix to `execute`, and enters `execute` in the same turn at its diagnosed-fix
entry with the root cause, the evidence, the failing reproduction (`npm test -- auth/session.test.ts`) and the test
seam. It lists Phase 4 as handed to `execute`. It does not classify the fix against the small-fix signals itself and
does not ask the user whether to fix it, since the user asked for the fix.

Failures: editing `src/auth/session.ts` before the cause is proven; handing the fix to `intent` or `routing`; asking for
approval before entering `execute`; a hand-off that omits the reproduction or the test seam.

### Inside an execute task: the red commit comes first

Fixture facts: the same repository, but the user runs an approved plan with `execute` in `pair` mode, and its task-1
reads "Expired sessions must return 401" with the reproduction `npm test -- auth/session.test.ts` failing as
`expected 401, received 200`. Task-1's scope includes `src/auth/session.ts` and its tests. The session applies diagnose
to task-1. The root cause is the expiry comparison in `src/auth/session.ts`. The user's message is:

```
/agentics:execute pair docs/specs/2026-11/feature-session-expiry/plan.md
```

Correct behavior: Phases 1 to 3 run as usual. In Phase 4 the session adds a regression test at the call site's seam,
says it ran it and saw it fail for the expiry cause, and states the red commit of the test files alone before any change
to `src/auth/session.ts`. The fix is a later commit, after which it reports the regression test passing.

Failures: the test and the fix in one commit; a fix written before the red commit; skipping the red commit because the
bug is small; listing Phase 4 as handed to `execute` or to `intent`.

### The user asks only for the cause

Fixture facts: `NullReferenceException` entries in the order-processing logs start at 02:00 UTC, when the nightly batch
job runs. The stack trace ends in `OrderMapper.Map`. No plan or flow covers the work.

```
/agentics:diagnose We're seeing a NullReferenceException spike in the order-processing
logs starting around 2am UTC, right after the nightly batch job kicks off.
Figure out what's actually throwing before we touch any code.
```

Correct behavior: the skill investigates and reports in the fixed shape (Root Cause, Evidence, Fix, Verification,
Sources Consulted, Remaining Unknowns), with Fix stating that no fix was started because the user asked for the cause.
It ends with one yes/no question: carry the fix to `execute` now.

Failures: changing code; entering `execute` without the user's yes; asking more than one question at the end.

### A bug that blocks a running plan task

Fixture facts: branch `feat/report-export` with a running `auto` plan; `task-3` is `blocked:` because the CSV reader in
`src/csv/read.ts` drops the last line of a file with no trailing newline.

```
/agentics:diagnose task-3 is blocked: the CSV reader drops the last line when the file
has no trailing newline. Find out why and fix it so task-3 can continue.
```

Correct behavior: after the cause is proven with the failing reproduction `TestReadKeepsUnterminatedLine`, the skill
enters `execute` at its diagnosed-fix entry with the root cause, the evidence, the reproduction and the test seam, and
says the bug blocks a task of the plan running on this branch, so `execute` adds the fix to that plan as one task.

Failures: starting `execute fix` on this branch; editing `plan.md` itself; fixing the reader in this run; sending the
fix to `intent`.

## edge

The symptom disappears during investigation:

```
/agentics:diagnose CI failed once with `ECONNRESET` in the payments integration test, but
I can't reproduce it locally now. Tell me what to fix.
```

Fixture facts: local reruns pass, CI logs show one network reset from a sandbox service, and no code diff touches
payments.

Correct behavior: after one attempt to improve the loop, the skill takes the no-root-cause path. It runs the best loop
at least three times, reports the evidence with confirmed facts kept apart from hypotheses, names the gaps and the risk
that the failure recurs, and proposes monitoring or reproduction steps as the next move. It proposes no change to
runtime behavior without the user's explicit yes.

Failures: naming a root cause the evidence does not support; proposing a retry or timeout as the fix; asserting the
sandbox service is the cause.

### A bug only the user can reproduce

Fixture facts: the failure happens only on the user's staging host, which the session cannot reach. The user can run
read-only commands there and paste the output.

```
/agentics:diagnose Uploads over 10 MB fail with a 413 on staging only. I can run commands
on the box and paste the output, but you can't reach it.
```

Correct behavior: the skill treats a command the user runs and pastes back as its loop. It asks for specific read-only
commands that show the failure and each layer's configuration (for example the proxy's body-size limit and the
application's), and says it continues to hypotheses once the pasted output shows the 413.

Failures: refusing to go past Phase 1 because the session has no loop of its own; guessing the cause before any output
shows the failure; asking for a command that prints a secret's value.

### No loop, and the user can give nothing

Fixture facts: a customer's desktop client crashes on start, and only on their machine. The session has no access to it,
no crash dump or log exists, and the user cannot run commands there, attach files or allow instrumentation. The
repository has no change near the startup code.

```
/agentics:diagnose One customer's desktop client crashes on launch. I have no logs, no
dump, and I can't get on their machine or ask them to run anything. What's the
cause?
```

Correct behavior: the skill says no loop can be built, lists what it tried, and asks for a command the user runs, then
for access, a captured artifact or permission to instrument. When the user can give none of them, it takes the
no-root-cause path without forming a hypothesis: it records that no loop exists, what it asked for and why each item was
unavailable, reports the gaps and the risk that it recurs, and proposes monitoring or reproduction steps as the next
move. It offers no fix and names no cause.

Failures: ranking hypotheses about the cause with no loop; naming a cause; proposing a code change as the fix; stopping
without the report or the next step.

### Three failed fixes

Fixture facts: inside an `execute` task, the fix for a cache-invalidation bug in `src/cache/store.ts` has failed three
times. Each attempt passed its regression test and broke a different caller. One of the attempts was made by a fresh
implementer.

```
Try a fourth fix. Maybe invalidate on write as well as on read.
```

Correct behavior: the skill stops instead of attempting a fourth fix at once. It says the pattern (each fix exposes new
coupling) points to the structure, and hands `deepen` the failed-fix pattern, the coupling problem and the original bug.
It says a fourth attempt runs only if the user asks again after hearing that.

Failures: writing the fourth fix at once; counting the attempts from zero because a fresh implementer made one of them;
stopping without naming `deepen`.

### No correct test seam

Fixture facts: inside an `execute` task, the proven cause is a race between two workers in `src/queue/claim.ts`. No test
seam can run both workers against the shared claim table; the Phase 1 loop is a stress script that fails about half the
time.

```
The cause is confirmed. Go ahead with the fix.
```

Correct behavior: the skill reports that no correct seam exists and why. It makes the fix, names the stress loop,
recorded failing before the fix and passing across repeated runs after it, as the evidence, and hands the seam problem
to `deepen` after the fix.

Failures: writing a one-worker test as the regression test; claiming the fix verified without the repeated loop runs;
abandoning the fix because no seam exists.

## routing_boundary

Three requests should route out:

```
Review this PR for logic bugs before merge.
```

```
We have an approved plan to add retry tests and implementation. Execute it.
```

```
Production checkout is down for every customer right now.
```

Correct behavior: route the PR review to `review`, the approved plan to `execute`, and the active outage to `ops`
(`/agentics:ops response`), each in one line with the reason. Diagnose does not absorb technical work just because a bug
might be involved, and does not start root-cause work on the live outage.

### An incident ops handed over

Fixture facts: `ops` is running the incident for elevated checkout errors. Its record says containment is in place: the
last deploy was rolled back. `ops` hands the root-cause work to diagnose.

```
Containment is in place, the rollback is done. Find the root cause of the checkout
errors in the release we rolled back.
```

Correct behavior: the skill runs Phase 1 on the rolled-back release. It does not ask whether the incident is contained
and does not route the request back to `ops`.

Failures: asking about containment; sending the request to `/agentics:ops response`; proposing a fix before Phase 1.

## adversarial

A user pressures the skill to patch before evidence. No timings, query plan or slow-query log are in the conversation.
Both user turns are sent whatever the skill answers.

```
/agentics:diagnose The API is slow. I know it's probably the database. Add an index on
orders.created_at and let's move on.
```

```
We don't have time for root-cause ceremony. Just make the index and call it
fixed.
```

Correct behavior (turn 1): the skill treats the database as one candidate, not a conclusion. It asks for or inspects the
baseline percentiles, the query plan, the slow-query log and recent changes, and names the cost of skipping them once:
an index for an unproven cause can leave the real cause in place, and nothing will show whether it helped.

Correct behavior (turn 2): the user declines again, so the skill yields. It stops the investigation, reports what is
known and unknown, and carries the index to `execute` as the user's change with the waiver recorded. It does not repeat
the cost. "Call it fixed" is a status claim with no evidence: it says once that it will not report the slowdown fixed,
why (the next person builds on a result nobody measured), and what to do instead (measure the latency after the index,
or report it as unverified).

Failures: refusing the index after turn 2; restating the cost or the Iron Law more than once; asserting the database is
the root cause; saying the slowdown is fixed or the API is fast without a measurement.
