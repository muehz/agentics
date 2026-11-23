# Verification preflight

Before you compute waves, check that each task has an available feedback loop in its intended working directory and
shell. Reuse evidence for identical commands in the same environment. Availability is not a passing baseline. A runnable
test with failing assertions is a loop. A missing runner, module, script, or target is a gap.

## Probe the entry point

Use the project's own tooling and the cheapest sufficient checks. Resolve the executable. Then inspect the named script,
package script, build target, or runner configuration and required dependencies. A resolved `npm` does not prove
`npm run test:b` exists. Resolve relative paths from the task's directory, including directory changes in the command.
Tests or outputs the task explicitly creates need not exist yet if their runner and entry point are available.

On POSIX, use the actual shell's `command -v` and file checks such as `test -f`. On PowerShell, use `Get-Command` and
`Test-Path -LiteralPath`. On PowerShell, also inspect the project's script/target definitions. These check only what
they name. Neither an interpreter path nor a file's presence proves all dependencies resolve. Use a supported
discovery/import check when needed. Do not execute compound acceptance commands, install dependencies, or run the entire
suite without prior inspection, only to probe availability. If a cheap check cannot establish availability, report it as
unresolved with the missing evidence. Do not translate a POSIX observation into a claim that Windows was tested.

Check that the command's selection reaches the task's own files. The selection is the glob, include and exclude lists,
test filter, and working directory the command uses. A gap exists when the selection misses a file that the task creates
or changes and that the command should check. Two examples are a test file for a test runner and a source file for a
linter or typechecker. Report this gap in Step 1's **Gap / Blocked task(s) …** report, never as a pass.

If a readiness audit (`/agentics:scaffold audit`) ran in this session against all of these, reuse its Wiring evidence:

- The same command
- The same working directory
- The same HEAD
- The same selection configuration
- The same files

That evidence replaces a new derivation of the selection. If no such audit ran, inspect the selection directly. Do not
run the suite to answer this.

## Choose the next action

Use Step 1's **Gap / Blocked task(s) / May proceed / Question / Resolve** report. Hold affected tasks and all transitive
dependents. Independent, fully specified tasks with available verification continue. A command that resolves never
bypasses a dependency. Choose the smallest authorized bootstrap:

1. **Setup already in the plan.** Run that prerequisite task first, serially, through an implementer like any task. Its
   own checker need not preexist.
2. **Existing harness needs an in-scope repair.** Dispatch the repair as a bounded prerequisite packet to an
   implementer; the controller writes no code (**The controller rule**). Exercise the repaired loop before you release
   dependent work.
3. **No harness.** Choose `/agentics:tdd` for automated behavior tests or `/agentics:scaffold feedback-loops author` to
   launch and exercise the real application.
    - Name both routes and their purposes in **Resolve**. Recommend the route the task needs.
    - If the user already authorized the required setup, do these steps:
        1. Define a bounded prerequisite packet with file scope, acceptance criteria, verification command, and
           dependency edges.
        2. Proceed.
    - Otherwise, do these steps:
        1. Put only the unsettled scope/tooling choice in **Question**.
        2. Write that choice as a ❓ question with a ➡️ recommendation of the route the task needs.

For settled work, write **Question:** none — proceeding with <authorized action> in place of a repeat question. Mark
waiting tasks `blocked: <gap> — awaiting <prerequisite or decision>`, never `skip:`. Keep the accepted plan intact.
Record added setup, changed verification commands, and dependency edges through **Plan divergence**. Carry the updated
contract in the bootstrap and dependent packets. An authorized stdlib-only setup does not authorize the installation of
a third-party runner only because the original command named it. Prefer an existing supported runner (for example,
Python's `unittest` for stdlib-only behavior tests). Record the authorized command change. Do not emulate a missing
third-party tool or add a compatibility shim only to preserve the old command spelling.

The bootstrap exception covers only the bounded work that creates or repairs the loop, not the feature wave. Before
release, exercise the resulting loop against its acceptance criteria. Keep the normal Stage 1 and quality review rules.
File creation or the implementer's self-report does not prove the loop works. If that proof fails, hold the bootstrap's
dependents while other eligible work proceeds. After proof, recheck their commands. Then recompute waves. Confirm the
evidence applies in the workers' execution environment, such as a fresh worktree or a different shell.

**Isolated packets.** This comparison runs at dispatch, not with the probes above. It is the one deliberate exception to
the rule that a suite does not run only to learn about it. The first time an isolated packet uses a given verification
command, do these steps:

1. Run the command in the packet's worktree at its base, before any change.
2. Run the command in the controller's tree at a moment when no implementer is writing there: before the wave's first
   dispatch, or after every implementer in the wave has returned. A count taken while siblings write is not a baseline.

Each distinct command runs once per run. For a worktree you create per [isolated-packets.md](isolated-packets.md)
**Start**, run that command there before the implementer enters. A native `isolation: "worktree"` dispatch makes its
worktree only when the worker starts, so its packet carries the command. The worker runs it right after the base check
and before any change. The worker reports the counts in its normal return. Compare them before you integrate that head.
A gap holds the head unintegrated. The preview names the pending comparison and its cost. Compare executed and skipped
counts. Fewer executed or more skipped in the worktree is a gap: `worktree lacks inputs`. Name the command and the
counts. Report the gap in the Step 1 report shape. Hold the packet and its dependents. When the runner prints no counts,
use its verbose or report flag (for example `go test -v`). When no count can be had, do both of these:

- Report the comparison as unresolved with the missing evidence.
- Never infer "no gap" from matching exit codes.

**Resolve** offers three choices:

- Add a setup step to the plan through **Plan divergence**.
- Run the packet serially in the shared tree.
- Accept the task as unverified, under **The waiver** below.

Never copy credentials into the worktree. Never move the packet into the main checkout unless the user chooses it.
Shared-worktree packets skip this run.

## Preview, packets, and override

For each task, record its command, working directory, and shell. Also record its probe and observed result, and any gap,
bootstrap prerequisite, or override. Show this beside Stage 1 evidence in the dispatch preview, even when no feature
task is eligible. Include the task's evidence and limitations in every worker/reviewer packet. Retain gaps and overrides
in the run report. Never say that a successful availability probe means tests pass.

## Deferred verification

A task whose verification needs credentials, hardware, or a service this session cannot reach is valid when its packet
names the command, the expected result, and who runs it (the user, or CI). Do not wait on it. Write one
`waive "verification <task-id>"` row whose `why` reads `deferred to <owner>: <command> expects <result>`. The task's
`verify` row then comes from its group's quality review alone, and the run report and the handoff to `gate` list the
task as verification deferred, with its owner, so `gate` reports it as not run.

## The waiver

This preflight is a process gate ([gates.md](../../routing/references/contracts/gates.md#process-gate)). Name the cost
once: without a feedback loop, implementation can proceed, but no mechanical check can establish its correctness. On the
next reply that still declines, write one `waive verification-preflight` row whose `why` names the task IDs, and the
user's reason or `none given`. The preview, the packets, and the run report show
`Verification preflight waived — user override`, taken from that row. Proceed within the approved scope.

A waived task completes the same way as a deferred one: its `verify` row comes from its group's quality review alone,
and every report names it as unverified. The waiver covers availability, not output dependencies, the quality review, or
a truthful report. You may defer a prerequisite needed only for the waived loop; prerequisites that supply required
implementation inputs still hold. Never report a waived task as verified.

A deferred or waived task whose group's quality review is also waived has nothing to supply its `verify` row, so the row
stays `pending`, as the **Gates** table of [`SKILL.md`](../SKILL.md#gates) says. The user's own
`waive "verification <task-ids>"`, which declines a command that is available, also keeps the row `pending`; only the
deferral above (its `why` opens with `deferred to`) and this preflight waiver take the row from the quality review.
