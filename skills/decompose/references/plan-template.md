---
derived-from: <source-list-or-none>
---

# Plan document template

> **For execution:** Use the `execute` skill. It implements tasks one at a time and parallelizes verified-independent
> groups.

**Goal:** [One sentence describing what success looks like when all tasks are complete]

**Why now:** [One sentence: the trigger that makes this plan worth approving today. Write "none stated" when the source
gives none.]

**Biggest risk:** [One sentence: the single largest risk and the gate that catches it]

**Size:** [One sentence in plain words: "<N> tasks (<O> outcome, <S> support) in <G> parallel groups, with the skeleton
path <first task> to <last task>." A sliced plan starts with "<K> slices". A slicing-only plan reads "slicing only, no
tasks". Take the numbers from the counts that the Proportionality row records and from the Parallel Groups table.]

**Outcome proof:** [The exact command a person runs, and the observable result it must produce]

**Skeleton path:** [The ordered task IDs that get that command producing that result. No support task runs before the
skeleton path completes.]

**Scope:** [Keep each bullet to one line. Put longer scope detail in `## Requirements` or `## Approaches considered`.]

- **In scope:** [What this plan covers; selected slice IDs for a per-slice update]
- **Out of scope:** [What this plan explicitly does NOT cover]
- **Non-negotiables:** [Constraints that cannot be relaxed]

## Task index

| ID     | Task        | Role                | Tier                  | Dependencies | Parallel group |
| ------ | ----------- | ------------------- | --------------------- | ------------ | -------------- |
| task-1 | [Task name] | [outcome / support] | [Deep/Standard/Light] | none         | A              |

[One row per task, in plan order. The "Task" cell is the task title without its "Task N:" label. A sliced plan adds a
"Slice" column after "ID". A slicing-only plan has no table, and this section reads "none: slicing only". Write the
table padded, as Prettier formats it. Plan Validation Checklist row 4 checks each row against its task's fields.]

---

## Provenance

- **Challenged:** [yes YYYY-MM-DD | pending YYYY-MM-DD | declined by user | not run]
- **Result:** not run
- **Coverage:** [n/a — <reason> | <N> items, <U> uncovered, <P> partial, <O> orphan tasks, <C> contradictions; blocking:
  <ids or none>; waived: <items or none>]

<details>
<summary>Provenance record: written by, validated, plan review, proportionality</summary>

- **Written by:** `decompose` v[skill version that wrote this plan]
- **Validated:** not run
- **Plan review:** [<READY | REVISE> — <c> critical, <i> important, <m> minor, <n> informational, rev <r>, run <k>]
- **Plan review finding (presented):** [one line per open Minor or Informational finding, as `<finding> (<location>)`;
  omit the line when there is none]
- **Proportionality:** [pass | reordered | resized | sliced | override by user] — skeleton `[task-id]`→`[task-id]`, [N]
  tasks ahead of it ([M] support); [N] outcome / [M] support tasks; signals fired: [list, or none]

</details>

> Fill each row in Step 5 from a check that actually ran, by these rules. `execute` reads every row, folded or not.
>
> - **Challenged:** the Step 4b result for this plan only: `yes <date>` for a completed pass, `pending <date>` for an
>   asked round that awaits answers, `declined by user`, or `not run` when no round was asked. Never inherit a source
>   artifact's date or verdict.
> - **Result:** `pass` when all ten checklist rows ran this session and passed; `failed — <rows and what each needs>`
>   when all ten ran and at least one still fails; `not run — <rows that did not run>` otherwise, a waived checklist
>   included. A row that ran and failed is `failed`, never `not run`.
> - **Coverage:** the real result of `coverage-check.md`, or its `n/a — …` form.
> - **Written by:** `decompose` and the skill's version. A plan whose existing line names a `decompose` version earlier
>   than 4.22.0, or that has no line, is a pre-4.22 plan: keep an existing line unchanged and add none. `execute` then
>   reviews every task at Stage 1.
> - **Validated:** the date the rows ran, then `rows 1-10 all run` when **Result** is `pass`. When **Result** is
>   `failed`, drop the `rows 1-10 all run` / `rows 1-10 ran` clause and write `<date> — row <n> failed, see Result`
>   instead, so the line never reads as a pass. Otherwise list the rows that ran and those still pending. `not run` for
>   an untouched draft.
> - **Plan review:** the last run's verdict, its tally by severity, its revision, and its run number. Add
>   `, self-read (not independent)` for a self-read. Keep an existing `not triggered` line on a plan written before
>   every plan was reviewed.
> - **Proportionality:** the check's disposition, then the real counts.
>
> A slicing-only plan reads `**Result:** not run — slicing only, no tasks`, `**Validated:** not run — slicing only`,
> `**Plan review:** not run — slicing only`, and `**Proportionality:** not run — slicing only`. On a legacy plan every
> row stays in plain view, with no `<details>` fold.
>
> `execute` skips its Stage 1 review for a task marked `**Packet review:** no` only when **Written by** names
> `decompose`, **Result** is `pass`, and the plan review was independent. A self-read also marks every task `yes`, so a
> plan reviewed only by its author gets the Stage 1 review from any `execute` version. A plan without this block is
> untrusted, so every task gets the Stage 1 review. `execute` refuses to start while the **Coverage** line's `blocking:`
> field names an item.

---

## Requirements

Use this section for inline input only. Record the settled requirements and acceptance checks once, as a numbered list,
so a task's `**Covers:**` field can name them. Slice sections and tasks cite this section. Delete the section when a
source artifact exists.

## Plan challenge

**Challenge record:**

[Open the section body with the literal label `**Challenge record:**` above. The shared contract requires that exact
label on every challenge entry. A materialized plan that drops it fails the record rule, however complete its fields
are. Then carry the five fields that the record rule names in references/chain-artifacts.md:]

1. [The target artifact and scope reviewed.]
2. [The execution reference, in one of three forms:]
    - [The session date, the rounds asked and answered, and the question numbers.]
    - [`prior record dated YYYY-MM-DD`.]
    - [`declined YYYY-MM-DD, no round asked`, when the user declined before any round.]
3. [Each resolution with its answer and source.]
4. [Remaining questions with their disposition, or "none".]
5. [Concerns weighed and set aside.]

[Record an explicit decline or a pending disposition here the same way. Reviews of the source intent and spec are
context, not this plan's challenge result.]

## Approaches considered

**Architecture:** [2-3 sentences about the technical approach]

**Tech Stack:** [Key technologies, libraries, frameworks involved]

**Existing tools checked:** [For each new state machine, persistent store, process manager, wire schema, extractor, or
protocol the plan builds: the repository framework, native platform tool, or provider tooling checked first, and one
line on why it does not fit. Write "none built" when the plan builds none.]

[Chosen implementation shape and relevant alternatives with rationale. Cite source solution decisions. Route an
undefined solution choice to spec rather than invent it while you decompose tasks.]

For sliced work, replace `## Tasks` with one `## S1`, `## S2`, ... section per slice. Each slice section opens with
these fields, then holds its `### Task N` entries:

- `**Title:**`
- `**Outcome:**`
- `**Scope:**` (source references and the boundary with siblings)
- `**Dependencies:**` (sibling IDs, named external decisions, or `none`)
- Optional `**Lane:**` (`yes` only on a slice that qualifies as a lane)
- Optional `**Issue:**` (a created or verified link only)

A retired slice keeps its `## S<n>` heading with `Retired: <reason>` and no tasks. Keep task IDs globally unique.
Express cross-slice dependencies as task edges. A slicing-only breakdown carries the sections with no tasks,
`**Skeleton path:** none — slicing only`, and the slicing-only Provenance values above. Its `## Task index` reads
`none: slicing only`. A sliced plan with tasks keeps one `## Task index` above the slice sections, with a `Slice` column
after `ID`.

## Dependency Graph

[Mermaid diagram or text description showing which tasks block which]

## Parallel Groups

| Group | Tasks          | Can run concurrently because...          |
| ----- | -------------- | ---------------------------------------- |
| A     | task-1, task-2 | No shared state, different files/modules |
| B     | task-3         | Depends on Group A output                |

## Risk Notes

| Risk                  | Impact  | Mitigation                  |
| --------------------- | ------- | --------------------------- |
| [What could go wrong] | [H/M/L] | [How to prevent or recover] |

[At least three rows, or one per task when the plan has fewer than three tasks. A risky operation's row names how to
recover from it.]

## Standing orders

[Optional. One numbered line per durable constraint the user stated for the whole run, such as "no new dependencies".
Omit the section when the user stated none. The section sits outside the approval hash. The user's yes to the plan
confirms these lines, and Step 6 writes one `order <n>` row for each.]

## Divergences

`execute` owns this section and appends to it at wave boundaries. `gate` marks an entry `Reconciled`. Leave it empty
when you write the plan. The section sits outside the approval hash, so an appended entry leaves the approval current.
After approval, never edit a task to record a divergence, because a body edit makes the approval `stale`, and a stale
plan approval stops the run. Append an entry instead.

## Documentation impact

List every doc this plan's changes touch, including README sections, `docs/` pages, and diagrams. Write "none" when no
doc is affected. `gate` branch mode reads this section for its automatic docs refresh. `docs` maintain mode consumes it
as the work list.

| Doc          | Affected by | Change needed |
| ------------ | ----------- | ------------- |
| [exact/path] | task-N      | [one line]    |

---

## Tasks

### Task 1: [Component Name]

**ID:** task-1 **Dependencies:** none **Parallel group:** A **Tier:** [Deep | Standard | Light] **Role:** [outcome |
support. Test: remove the task. Does the Outcome proof still pass? no = outcome, yes = support] **Packet review:** [no |
yes — one-line reason] **Review group:** [optional name] **Covers:** [the source items this task delivers, by location,
such as `spec.md Success Criteria 2` or `Requirements 3`; `none` when it delivers no source item] **Reproduction:**
[only on a task that fixes a bug: the failing command or the broken behavior to restore]

**Files:**

- Create: `exact/path/to/new-file`
- Modify: `exact/path/to/existing-file` (lines 123-145)
- Test: `tests/exact/path/to/test-file`

**Acceptance Criteria:**

- [ ] [Specific, testable criterion]
- [ ] [Specific, testable criterion]
- [ ] Tests pass: `[project test command] tests/path/to/test`

**Steps:**

<details>
<summary>Steps 1-4: [gist of the step headings, at most 12 words]</summary>

**Step 1: Write the failing test**

Test file: `tests/exact/path/to/test-file`. One line per case, in the form setup, action, expected result.

**Step 2: Run test to verify it fails**

Run: `[project test command] tests/path/to/test` Expected: FAIL with relevant error

**Step 3: Write minimal implementation**

The behavior to add, the existing code to follow cited as `path:lines`, and pseudocode (a fenced block whose info string
is `pseudocode`) for logic that prose would blur.

**Step 4: Run test to verify it passes**

Run: `[project test command] tests/path/to/test` Expected: PASS

</details>

[In the summary, "Steps 1-N" gives the number of steps in that task. A task with seven steps reads "Steps 1-7". A task
with no steps has no block. Number the steps from 1. Leave a blank line after the summary line and before the closing
tag. A task's steps end at its passing verification: `execute` stages and commits the files the implementer changed, one
task commit per task (the **Commit cadence** paragraph of [execute](../../execute/SKILL.md)). Write no Commit step.]

---

## Acceptance Criteria

- [ ] [Overall criterion spanning multiple tasks]
- [ ] All existing tests continue to pass
- [ ] New behavior is covered by tests

## Constraints

- **Performance:** [Any performance requirements]
- **Compatibility:** [Backward compatibility requirements]
- **Dependencies:** [External dependency constraints, with pinned versions]
- **Non-goals:** [What this work explicitly does NOT include]
- **Verification stop:** [The evidence that ends verification for this plan, naming the release-blocking verification
  tasks]
- **Optional confidence work:** [Verification beyond that stop, which does not block release, naming its tasks. Write
  "none".]
- **User-environment effects:** [Authentication prompts, browser launches, profile or global config mutation,
  installed-tool state changes this plan causes. Write "none".]
