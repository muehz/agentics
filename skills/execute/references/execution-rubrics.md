# Execute reference rubrics and tables

Structured tables, checklists, and rubrics referenced by [SKILL.md](../SKILL.md).

## Entry points

| Entry                                                         | Takes                                                                                                             | Section                                                                              |
| ------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------ |
| `/agentics:execute [auto [pause]\|pair\|arena] <plan> [S<n>]` | An approved plan, or one lane slice of it                                                                         | [Step 0](../SKILL.md#step-0-preflight), [Parallel lanes](../SKILL.md#parallel-lanes) |
| `/agentics:execute <brief>` or one task                       | An approved `brief.md`, or one already-decomposed task                                                            | [Step 0](../SKILL.md#step-0-preflight)                                               |
| `/agentics:execute arena <task>`                              | One task: a single-task input, or one named task of an approved plan                                              | [Modes](../SKILL.md#modes), [arena-mode.md](arena-mode.md)                           |
| `/agentics:execute fix <one sentence>`                        | A small fix, with no plan                                                                                         | [The `fix` entry](../SKILL.md#the-fix-entry)                                         |
| A hand-off from `diagnose`                                    | A proven root cause, its evidence, the failing reproduction, and the test seam                                    | [A diagnosed fix](../SKILL.md#a-diagnosed-fix)                                       |
| A user's change from `diagnose`                               | A [user's change](fix-entry.md#a-users-change): the diagnose report and a `declined` or `no root cause` statement | [The `fix` entry](../SKILL.md#the-fix-entry)                                         |

## Modes comparison

| Mode             | Implementer                     | Reviewer                                                                                      | Parallelism                                       | User stop                             |
| ---------------- | ------------------------------- | --------------------------------------------------------------------------------------------- | ------------------------------------------------- | ------------------------------------- |
| `auto` (default) | Subagent per task               | One quality reviewer per review group, then the aggregate reviewer at review-group boundaries | Wave-parallel where verified, else one-task waves | None, or each wave's end with `pause` |
| `pair`           | This session, with the user     | Same as `auto`                                                                                | One task at a time                                | Each wave's end                       |
| `arena`          | N candidate subagents, one task | Cross-judge, then the quality review of one task as a group of one                            | All candidates concurrent                         | None                                  |

Every mode runs the same reviews. A user's wave review adds to the model reviews and never replaces one.

## Dispatch preview

Print the dispatch preview once, in Step 1 after waves, tiers, and the Stage 1 checks exist, and before the native task
load. It never waits. Include:

- the plan path, and its `**Proportionality:**` line verbatim when the `## Provenance` block carries one;
- the waves with task counts, each task's tier, and each demotion with its reason;
- the review groups and the result of the group cycle check;
- each task's verification command, directory, shell, probe evidence, and any gap, bootstrap prerequisite, or waiver
  ([verification-preflight.md](verification-preflight.md));
- each task's Stage 1 status: dispatched, skipped under [stage-1-dispatch-rule.md](stage-1-dispatch-rule.md) with the
  evidence, or waived by the user;
- the total dispatch count, counting implementers, Stage 1 reviewers, quality and aggregate reviewers, and any final
  review;
- one selection record per role and tier, with the fields of the ladder's reporting table
  ([model ladder](../../routing/references/model-ladders.md#unavailable-selections-and-reporting)). Print the actual
  binding only when the provider confirmed it; otherwise say once that this host does not expose actual bindings.

## Independence Assessment Checklist

Every pair of tasks in a claimed-parallel group must pass all five:

- [ ] **No shared file writes.** The tasks do not modify the same files or overlapping regions.
- [ ] **No output dependency.** Task B does not need Task A's output.
- [ ] **No shared mutable state.** They do not read and write the same rows, config values, or environment variables.
      Read each task's generation, registration, and codegen steps, not only its file list: a registry, index, or
      generated file that both tasks update is shared state even when no `**Files:**` list names it.
- [ ] **No ordering constraint.** The result is the same in either completion order.
- [ ] **Independent verification.** Each can be verified without the other's result.

File-disjointness is not independence: migration, then model, then serializer, then tests passes the first check and
fails the next two. A group that fails any check runs as one-task waves within `auto`, stays one review group, and gets
one `demote wave-<N>` row. Demotions never block. Execute never adds parallelism it did not verify itself, unless the
user waives the demotion (`waive "independence <group>"`; tasks that write the same files still run one after another).

## Implementer statuses

| Status               | Means                                                        | Do                                                                                                      |
| -------------------- | ------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------- |
| `DONE`               | Complete, ready for review                                   | Commit and verify                                                                                       |
| `DONE_WITH_CONCERNS` | Complete, with caveats                                       | Commit and verify; hand the concerns to the reviewer                                                    |
| `RED`                | The red leg of a bug-fix task stopped after its failing test | [Red commit for a bug fix](mode-procedures.md#red-commit-for-a-bug-fix)                                 |
| `NEEDS_CONTEXT`      | Missing information or a conflicting assumption              | Answer it in an updated packet and re-dispatch; a second return on the same packet counts on the ladder |
| `BLOCKED`            | The packet, tools, or dependencies must change               | Change the packet and re-dispatch; a wrong base is fixed by the controller                              |

## Model tiers

Spend the least capability that still gets each task done correctly. Standard is defined execution work, Deep is
judgment about what to do, Light is mechanical work that is cheap to verify, and Max is review of Deep work or an
explicit selection. Every rule for model, effort, driver, clamps, overrides, fallback, and reporting lives in the
[model ladder](../../routing/references/model-ladders.md). Apply it; do not restate it.

Execute's own choices:

- A Stage 1 reviewer runs at Deep, and at Max only on an explicit user selection.
- A review group's quality reviewer runs at Deep. The ladder's **Reviewer selection** raises it to the next position
  above the strongest actual implementer, so a group with a Deep implementer reviews at Max. It runs at Standard only
  when every task in the group is Light and carries `**Packet review:** no`.
- The aggregate reviewer and the final reviewer follow the quality reviewer's rule for their group.
- Keep a separate reviewer for each review group, whatever the bindings. A shared or clamped binding is disclosed, never
  a reason to drop the review.
- Batch mechanical work into one Light agent where it fits, and cap concurrent Deep and Max agents within the surface's
  limit.

## Gates

Each gate here has one class ([gates.md](../../routing/references/contracts/gates.md)).

| Gate                                                  | Class             | Exit                                                                                                                                                                                                                                                                                                            |
| ----------------------------------------------------- | ----------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| No status claim without evidence from this run        | Safety gate       | Report what was verified and what was not; `gate` makes the completion claim                                                                                                                                                                                                                                    |
| The plan review before the plan approval              | Safety gate       | `decompose` revises the plan                                                                                                                                                                                                                                                                                    |
| Commit hooks and signing                              | Safety gate       | The no-commit red evidence; a hook failure goes back to the implementer                                                                                                                                                                                                                                         |
| The quality review of a review group                  | Process gate      | One `waive "quality-review <group>"` row; each task's `verify` row takes `ok` from your verification alone, and the report names the tasks as not reviewed. A task with neither your verification nor the review keeps its `verify` row `pending`                                                               |
| A task's own verification command                     | Process gate      | One `waive "verification <task-ids>"` row; those tasks' `verify` rows stay `pending`, and the report names them as unverified. A deferred task and a task under the preflight waiver take theirs from the quality review instead ([verification-preflight.md](verification-preflight.md#deferred-verification)) |
| Stage 1, the wave pause, the verification preflight   | Process gate      | One `waive` row each, as its reference says                                                                                                                                                                                                                                                                     |
| The controller rule                                   | Process gate      | Name the cost once (the change has no packet, no task verification, and no review); on waiver, one `waive controller-rule` row and a `commit user` row                                                                                                                                                          |
| The independence demotion of a claimed-parallel group | Process gate      | One `waive "independence <group>"` row; tasks that write the same files still run one after another                                                                                                                                                                                                             |
| Draft pull requests, no merge                         | Repository policy | `gate` follows `policy.draft_prs`; execute never pushes or merges                                                                                                                                                                                                                                               |

**Process-gate waiver mechanics.** A process gate names its cost once, in one sentence, and yields on the next reply
that still declines, in any wording. The waiver needs no reason; a reason the user gives goes in the row's `why`, and
otherwise the `why` reads `none given`. Any reply that declines a process gate after the first ask is a waiver, and a
resumed run reads the `waive` rows and does not ask again. It skips a `waive` row that an `invalid:` line of
`flow status` names ([flow-state.md](flow-commands.md#invalid-rows)), so that gate is asked again.

**The completion handoff.** When every task is done or dropped and verified, run **The boundary check** once more, then
offer `gate` and wait. When a waiver left tasks unverified or unreviewed, the offer names them instead of saying "done
and verified". The plain offer reads: "All tasks are done and verified — ready to close this branch out with a
completion check using `gate`?" When the user already named the close-out ("commit, push, and open a draft PR", "then
gate it"), that is the answer: run `gate`, and let the named destination answer its Step 3 proposal. A draft PR or push
named earlier answers only the plain draft proposal and never a `gh stack`
([gate's Answers](../../gate/references/branch/propose-and-execute.md#answers)). Execute never pushes; neither an
approved plan nor a finished wave authorizes a push. A "not yet" writes a `pause` row and defers the close-out; it never
turns into an unverified "done".
