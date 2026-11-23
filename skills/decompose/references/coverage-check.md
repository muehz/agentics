# Coverage check

Step 5 runs this read-only pass whenever `spec.md` or `intent.md` sits beside the plan. It runs after the
proportionality check and its refill, and before the artifact validator and the plan review, so the review sees the plan
the coverage fixes produced. The pass inventories what the source committed to, gives each item a disposition, and
reports orphan tasks and contradictions. It never edits `spec.md` or `intent.md`.

## When it runs

With a `spec.md` sibling, the pass inventories the spec. When `intent.md` is also present, the pass checks the intent's
`## Decisions` for contradiction. With only an `intent.md` sibling, which happens only for an in-flight five-section
intent, the pass inventories the intent alone. The pass does not inventory a legacy `design.md` that the plan read in
place with no spec or intent beside it. The pass records `n/a — legacy design.md, not inventoried` for it. That value
wins over `n/a — no sibling source`. A slicing-only plan (no `### Task` entries) records `n/a — slicing only`. A plan
with no sibling source records `n/a — no sibling source`.

## Inventory units

With a spec present:

- Each `## Success Criteria` item.
- Each `## User Stories` story, every priority, together with its **Independent test** line.
    - User Stories is an optional section. Its absence means none, never a gap, and Success Criteria carry the
      inventory.
- `## Scope Decisions` entries, checked for contradiction against the plan, never for coverage. A decision is a
  constraint, not a deliverable.

With only an intent present:

- Each `## Outcome` item.
- Each user decision in `## Decisions`, excluding the assistant-selected routing entry and the `**Challenge record:**`
  entry. Neither entry is a requirement a task can cover.

## The trace

A task covers an item when its `**Covers:**` field names the item's location, such as `spec.md Success Criteria 2` or
`spec.md User Stories 3`. The pass reads the field and infers nothing, so a re-run on the same plan returns the same
result. A task with no `**Covers:**` field, on a plan written before the field existed, is traced by reading its
acceptance criteria against the item, and the report says which tasks were traced that way.

## Dispositions

| Disposition                                   | Meaning                                                                                                                          |
| --------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------- |
| `covered` (task IDs)                          | Every clause of the item traces to at least one `outcome` task outside a retired slice.                                          |
| `partial`                                     | Some clause of the item has no task, or only `support` tasks or only tasks in a retired slice cover the item.                    |
| `uncovered`                                   | No task traces to the item.                                                                                                      |
| `n/a — constraint, checked for contradiction` | A negative decision or an exclusion ("X is out of scope"). No task can cover it, so the pass checks it against the plan instead. |
| `n/a — outside selected slice`                | The source assigns the item to a slice outside the current request's selected set.                                               |

The `n/a — outside selected slice` disposition has two more rules:

- The item is never `uncovered` or `blocking`.
- The pass judges an item with no slice owner normally.

## Orphans and contradictions

**Orphan tasks** are tasks that trace to no inventoried item. **Contradictions** are an intent decision or a spec
`## Scope Decisions` entry that the spec or the plan contradicts. Name the artifact and the location within it.

## Severity

Read the rows in order. The first match wins.

| #   | Finding                                                                                                       | Severity   |
| --- | ------------------------------------------------------------------------------------------------------------- | ---------- |
| 1   | An `uncovered` or `partial` item whose text carries align's `(policy: <skill>: <id>)` suffix, for a `must` id | `blocking` |
| 2   | A contradiction                                                                                               | `blocking` |
| 3   | An `uncovered` user story whose priority is P3, a follow-up nobody blocks on                                  | `warning`  |
| 4   | Any other `uncovered` item                                                                                    | `blocking` |
| 5   | Otherwise: a `partial` item or an orphan task                                                                 | `warning`  |

For row 1, use the skill recorded on the spec's **Policy skills** line. Never re-run policy discovery for this check. A
recorded skill that cannot be loaded, or a policy id absent from that skill's index, gets no escalation, and the report
says so.

## The report

Show a table in the Step 5 chat report. Give one row per inventoried item, orphan task, and contradiction:

| Source item | Location | Disposition and task IDs | Severity |
| ----------- | -------- | ------------------------ | -------- |

## The gate

A `blocking` finding holds Step 6's execution question until a plan edit fixes it or the user waives it. This is a
process gate ([gates.md](../../routing/references/contracts/gates.md)): name the cost once, which is that `execute` will
build a plan that leaves the item undelivered, and yield on the next reply that still declines. A waiver needs no
reason. Record it as one row, with the reason the user gave or `none given`:

    agx flow log decompose waive coverage ok "<item ids>: <reason, or none given>" none "decompose ask for approval" --folder <work-folder>

A fix or waiver changes only the plan or the recorded waiver, never `spec.md` or `intent.md`.

## The `**Coverage:**` line

Record the result on the Provenance block's `**Coverage:**` line:

```text
<N> items, <U> uncovered, <P> partial, <O> orphan tasks, <C> contradictions; blocking: <ids or none>; waived: <item — reason; …> or none
```

or one of the `n/a — …` forms above. A waived item appears under `waived`, with its reason or `none given`, never under
`blocking`. A plan update scoped to one slice still inventories the whole source and counts tasks across every slice, so
the line always reflects the whole plan. Items the source assigns outside the selected set take
`n/a — outside selected slice` and never block the execution offer.

## Standalone re-run

A re-check requested on its own ("re-check coverage"), for example after `/agentics:spec align` or a plan edit, re-runs
this pass and does not redo decomposition. Where the result goes depends on the plan's approval in `flow status`:

| Plan approval                   | Where the result goes                                                                                                                                            |
| ------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `current` or `unconfirmed here` | A `note coverage` row: status `ok` and evidence `none` when nothing blocks, else status `failed` and the blocking item ids in its `why`. The plan is not edited. |
| Otherwise: none, or `stale`     | The `**Coverage:**` line is rewritten, and the plan is staged as an update per [save-and-stage.md](save-and-stage.md).                                           |

An approved plan is not edited, because an edit makes its approval stale and would ask the user to approve the same
tasks again. When the re-run finds a blocking item on an approved plan, report it with its items and offer the fix: a
plan update, which then needs a new approval, or a waiver row. Emit no `create plan.md` row and no `challenge` row. This
pass does not change the plan's header.
