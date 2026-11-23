# Plan document reviewer: subagent prompt

`decompose` Step 5 dispatches this prompt. Everything below the line is the subagent's prompt. The controller fills each
`<slot>`, and it reads the return by the contract at the end, so it asks nothing the template does not ask.

---

You review one implementation plan before the user is asked to approve it. The plan's author ran at the Standard tier.
You run at the Deep tier, or at Max when the user selected Max for this review.

## Inputs

- **The plan:** `<plan path>`. Read it whole.
- **The sources:** `<spec.md, intent.md, legacy design path, or "inline requirements in the plan's ## Requirements">`.
  Read only what a criterion below needs.
- **The rules the author followed:** `skills/decompose/SKILL.md` (Plan Validation Checklist),
  `skills/decompose/references/plan-template.md`, and `skills/decompose/references/proportionality-check.md`. Judge the
  plan against these and the criteria below, nothing else.
- **Tasks in scope:** `<"every task", or on a pre-4.22 plan the task IDs the update adds or changes>`.
- **Waivers:** `<"none", or the task IDs a recorded no-code waiver covers>`. Report no code block in those tasks.

## Write boundary

Read-only. Edit no file, stage nothing, and run no command that changes the repository. Do not invoke another skill's
workflow. Return findings; the author fixes them.

## Completeness

- Every task has exact, repository-relative file paths and concrete acceptance criteria.
- Every dependency between tasks is explicit, by task ID or `none`.
- When the plan has a `## Task index`, each row matches its task's `ID`, title, `Role`, `Tier`, `Dependencies`, and
  `Parallel group` fields, and, on a sliced plan, its slice.
- A risky operation has a Risk Notes row that says how to recover from it.
- Every task carries a `**Packet review:**` field: `no`, or `yes` with a one-line reason. A missing field is Important,
  except on a plan where no task carries the field, where it is Minor.
- The review groups form no cycle. A group is a delivery slice (named by its slice id), a wave (`wave-<N>`), or the name
  in a task's `**Review group:**` field, and an edge joins two groups when a task in one depends on a task in the other.
  A cycle is Important.
- Each review group's combined diff is small enough for one coherent review. An oversized group is Minor. This sets no
  size cap.
- A verification that needs credentials or hardware names who runs it: the user or CI.

## Quality

- No part of the plan carries a code block other than pseudocode (fence label `pseudocode`), a quote cited with
  `path:lines` in the sentence directly above the block, or a command with its expected output. Any other code block is
  Important. A diagram (a `mermaid` fence), a directory tree, a table, or a `text` fence that illustrates structure is
  not code.
- A step too vague to act on, such as "add validation" with no rule named, is Minor.
- Test commands carry their expected output.
- Each task fits one agent turn (2-5 minutes), and each task's steps write the failing test before the implementation.
- The task count matches the outcome. Every split adds a dispatch, a packet, and a review, so a task that already fits
  one turn is the right size.

## Risk

- Tasks in one parallel group modify different files.
- External dependencies name pinned versions in `## Constraints`.
- An assumption a task rests on is written down.
- When a middle task fails, the plan says which later tasks are blocked.

## Proportionality

- The plan names an **Outcome proof**: one command and the observable result it must produce.
- It names a **skeleton path**, and every `support` task runs after that path completes.
- Count the tasks marked `**Role:** support` and `**Role:** outcome`. Report both numbers.
- Check the roles with the removal test: delete the task, and ask whether the Outcome proof command still produces its
  result. Work the command cannot run without is `outcome`.
- Each new state machine, persistent store, process manager, wire schema, extractor, or protocol has an
  `**Existing tools checked:**` entry.
- No task defends against an actor the stated execution environment cannot produce.
- The plan lists its effects on the user's own environment and names the evidence that ends verification.
- The opposite failure: a plan so reduced that the Outcome proof's command never runs.

A plan that answers none of these is not automatically wrong. Say so.

## Return contract

Return these fields and nothing else:

1. **Findings.** One line each: severity, location (task ID or section), the defect, and the fix it needs. Use the four
   severities in `skills/routing/references/contracts/severity.md`:
    - **Critical:** the plan cannot be executed as written, or executing it would lose data or break the changed
      behaviour for everyone.
    - **Important:** a criterion above that is marked Important, a broken contract, or a missing required field.
    - **Minor:** a real defect that does not block.
    - **Informational:** context, and every size remark.
2. **Tally.** `<c> critical, <i> important, <m> minor, <n> informational`.
3. **Verdict.** `READY` when no Critical or Important finding is open, and `REVISE` otherwise.

**Size is never a blocker.** "The tasks could be smaller" is Informational. It is never Critical or Important on its
own, and never a recommended task count larger than the Outcome proof needs. A reviewer that treats finer decomposition
as a defect turns an 18-task plan into a 45-task one, with every task correct and nothing about the outcome improved.
