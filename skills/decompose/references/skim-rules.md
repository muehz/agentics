# Skim rules for intent, spec, and plan artifacts

A new intent, spec, or plan lets an approver decide from its top lines. These rules set the shapes. They never move a
pinned label or section name, so every skill that reads an artifact keeps working. A skill that rewrites a legacy
artifact adds no card, no Task index, no fold, no Provenance split, and no `Source` line. A legacy artifact is one
written before the card rule: an intent or plan with no card (a plan with no `## Task index`), or a spec whose
`**Written by:**` line names an earlier `spec` version. An intent written after the card rule but before the `Source`
rule, whose card has no `Source` line, is legacy for a rewrite too. A legacy artifact converts only when the user asks.
If a rule here and the plan template differ, the template wins, and this file needs a fix.

## The card

Example for an intent:

    # Intent: add a --json flag to the status command

    - **Change:** The status command can print its result as JSON.
    - **Why now:** Scripts parse the text output, and a format change breaks them.
    - **Out of scope:** YAML output and any change to the text format.
    - **Risk:** A script already depends on the current text format.
    - **Source:** The conversation, where the user asked for it.

    ## Problem

The card answers five questions for a new intent and for a spec, and four for a plan, from the top of the file, so an
approver needs no scrolling:

| Question                  | Intent              | Spec           | Plan                                                    |
| ------------------------- | ------------------- | -------------- | ------------------------------------------------------- |
| What changes?             | `Change`            | `Change`       | `Goal` (existing)                                       |
| Why now?                  | `Why now`           | `Why now`      | `Why now`                                               |
| What is out of scope?     | `Out of scope`      | `Out of scope` | the `Out of scope` bullet of the existing `Scope` field |
| What do you need from me? | none: asked in chat | `Ask`          | none: the approval question is asked in chat            |
| What is the biggest risk? | `Risk`              | `Risk`         | `Biggest risk`                                          |
| Where did this come from? | `Source`            | -              | -                                                       |

- In an intent and a spec, the card is an unheaded bullet list directly under the `#` title line, before the first `##`
  heading. It adds no section. A new intent's card has five lines in this order: `Change`, `Why now`, `Out of scope`,
  `Risk`, `Source`. A spec's card has five lines in this order: `Change`, `Why now`, `Out of scope`, `Ask`, `Risk`. A
  spec and a plan carry no Source line. An intent card that already holds an `Ask` line keeps it, because removing it
  would change the approved text.
- In a plan, the card is the header field list in this order: `Goal`, `Why now`, `Biggest risk`, `Size`,
  `Outcome proof`, `Skeleton path`, `Scope`. `Architecture` and `Tech Stack` sit at the head of
  `## Approaches considered`. A new plan carries no `Ask` line: `decompose` asks its approval question in chat. A plan
  written with an `Ask` line keeps it, because removing it would change the approved text.
- The question lines are one sentence each: `Change`, `Why now`, `Out of scope`, and `Risk` in an intent and a spec,
  `Ask` in a spec only, `Source` in an intent only, and `Goal`, `Why now`, and `Biggest risk` in a plan. The plan's
  `Size` line is also one sentence. `Outcome proof`, `Skeleton path`, and `Scope` keep their current contracts.
- `Source` is one sentence that names an origin from a closed set: the conversation, `issue #N` (`owner/repo#N` for an
  issue in another repository), `pull request #N` (written the same way), `pasted text` (material of any kind that the
  user pasted, such as a thread, an email, or a requirements paste), or `document <repo path>`. A capture that rests on
  more than one origin names them in one sentence, for example "The conversation, which cites issue #1150." Like `Ask`,
  it is static text and holds no count, date, or approval state. Never invent an origin.
- A writer with no live session writes only the origin its input supports. An issue-driven run with no live session
  writes `issue #N`, and an input that names no origin reads `none stated`. No writer defaults to the conversation: the
  phrase is valid only when a live session supplied the capture.
- In a plan, each `Scope` bullet is one line. Longer scope detail goes in `## Requirements` or
  `## Approaches considered`.
- A line the source does not support reads `none stated`. Never invent one. For a plan from inline input with no source
  artifact, take `Why now` from `## Requirements`, or write `none stated`.
- `Ask`, in a spec, is static text. It names the question, the tier, and the next owner. It never holds a count or an
  approval state, because an approval binds the file's content hash. The `Status:` line carries only what its form below
  already carries.

## Plan shapes

Example of the top of a plan:

    **Goal:** `status --json` prints valid JSON that a script can parse.

    ...

    **Size:** 2 tasks (2 outcome, 0 support) in 2 parallel groups, with the skeleton path task-1 to task-2.

    ## Task index

    | ID     | Task              | Role    | Tier     | Dependencies | Parallel group |
    | ------ | ----------------- | ------- | -------- | ------------ | -------------- |
    | task-1 | Add the flag      | outcome | Standard | none         | A              |
    | task-2 | Cover it in tests | outcome | Light    | task-1       | B              |

Example of a task's steps:

    **Steps:**

    <details>
    <summary>Steps 1-3: failing test, implement, passing test</summary>

    **Step 1: Write the failing test**

    **Step 2: Write minimal implementation**

    **Step 3: Run the test to verify it passes**

    </details>

Example of the Provenance block:

    - **Challenged:** yes 2026-10-23
    - **Result:** pass
    - **Coverage:** 3 items, 0 uncovered, 0 partial, 0 orphan tasks, 0 contradictions; blocking: none; waived: none

    <details>
    <summary>Provenance record: written by, validated, plan review, proportionality</summary>

    - **Written by:** `decompose` v4.25.0
    - **Validated:** 2026-10-23, rows 1-10 all run
    - **Plan review:** READY — 0 critical, 0 important, 0 minor, 0 informational, rev 1, run 1
    - **Proportionality:** pass — skeleton `task-1`→`task-2`, 0 tasks ahead of it (0 support); 2 outcome / 0 support tasks; signals fired: none

    </details>

- **Size line.** One sentence:
  `<N> tasks (<O> outcome, <S> support) in <G> parallel groups, with the skeleton path <first task> to <last task>.` A
  sliced plan starts with `<K> slices`. A slicing-only plan reads `slicing only, no tasks`. The numbers come from the
  Proportionality row and the Parallel Groups table. Refill the line after the proportionality check, because a reorder,
  a resize, or a slice can change it.
- **Task index.** `## Task index` comes directly after the card. It has one row per task in plan order, with the columns
  `ID`, `Task`, `Role`, `Tier`, `Dependencies`, and `Parallel group`, named as the task's own fields. A plan written
  with the shorter headers `Depends on` and `Group` keeps them. A sliced plan adds `Slice` after `ID`. Checklist row 4
  checks each row against its task. Update the rows after any reorder, resize, or slice. The `Task` cell is the task
  title without its `Task N:` label. A slicing-only plan has no table, and the section reads `none: slicing only`.
- **Folded steps.** Under `**Steps:**`, one `<details>` block holds the step text. Its summary reads
  `Steps 1-<N>: <gist>`. Leave a blank line after the summary line and before the closing tag. `Files` and
  `Acceptance Criteria` stay in plain view. The gist lists the step headings in at most 12 words. A task with no steps
  has no block.
- **Provenance split.** `**Challenged:**`, `**Result:**`, and `**Coverage:**` stay in plain view. Every other Provenance
  row sits in one `<details>` block with the summary
  `Provenance record: written by, validated, plan review, proportionality`. No row is copied. On a legacy plan every row
  stays in plain view.

## Spec shapes

Example of the opener of `## Solution`:

    **At a glance**

    - **Behavior:** `status --json` prints one JSON object.
    - **Interfaces:** a new `--json` flag on the `status` command.
    - **Files and modules:** `cli.py` and `tests/test_status.py`.

The first element of `## Solution` is the bold line `**At a glance**`, then a list of three to six bullets: `Behavior`,
`Interfaces`, `Files and modules`, and optionally `Unchanged`, `Rejected shape`, or `Risk`.

## The Challenge record

Example of an intent's record:

    - **Challenge record:**
        1. Target `intent.md`, scope the direction of this work.
        2. Executed 2026-10-23, 1 round asked and answered, Q1 to Q3.
        3. Resolutions: ...
        4. Remaining questions: none.
        5. Concerns weighed and set aside: ...

In an intent and a spec, write the five fields as a numbered list nested under the unchanged `**Challenge record:**`
label, in the contract's order. The plan's record already uses a numbered list.

## Writing rules

- **Bold first term.** A list item or table row that states a fact or a rule starts with a bold label for its subject,
  such as `**Fails closed:**`. The rule applies to the card, the At a glance list, and decision entries.
- **One idea per paragraph.** About three sentences is guidance for writers. No check counts sentences or lines.
- **Example first.** Each shape rule in this file opens with a short example of the shape, then states the rule.
- Write every table padded, as Prettier formats it. Prettier rewrites an unpadded table, and the rewrite stales an
  approval.
- Follow `skills/docs/references/technical-writing.md` for all prose.

## Status glossary

These labels already exist. This glossary invents none.

| Label                                                                     | Where                  | Meaning                                                  |
| ------------------------------------------------------------------------- | ---------------------- | -------------------------------------------------------- |
| `blocker`                                                                 | intent, Open questions | An unanswered direction question that holds the handoff. |
| `deferred`                                                                | intent, Open questions | The user deferred it, and the direction stays clear.     |
| `downstream`                                                              | intent, Open questions | Solution or implementation work for the next owner.      |
| `assumed, pending author confirmation`                                    | spec                   | A target that nobody sourced.                            |
| `Resolution: open`                                                        | spec, Policy Conflicts | No resolution yet. It blocks approval.                   |
| `Resolution: resolved: <how>`                                             | spec, Policy Conflicts | The change that removed the conflict.                    |
| `Resolution: exception accepted <date>: <reason>`                         | spec, Policy Conflicts | The user's reason to proceed despite the conflict.       |
| `covered`, `partial`, `uncovered`                                         | plan, Coverage         | The disposition of a source item.                        |
| orphan                                                                    | plan, Coverage         | A task that traces to no source item.                    |
| `Challenged: yes <date>`, `pending <date>`, `declined by user`, `not run` | plan, Provenance       | The state of the plan challenge.                         |

## The status line

Before an approval question, a skill that has a status line prints it in the chat render and in an unattended report.
The line is written to no file. It asks nothing. A skill that asks no approval question prints none.

- `intent`: `Status: <n> open questions (<d> deferred, <w> left for the spec) · approval pending`, or
  `Status: no open questions · approval pending`. The line has no blocker count: a `blocker` holds the handoff, so none
  is open when the line prints.
- `spec` prints no status line. It states the tier in one line that starts `Tier: `, with its plain meaning, and shows
  the review's findings before its approval question.
- `decompose` prints no status line. Before its approval questions it shows each artifact by path and hash, with a short
  summary and the findings the reviews left open, as `references/approval.md` sets.

## What the rules forbid

- Hard length budgets, a cap on top-level headings, and claim-style headings on pinned section names.
- Emoji status markers. Use the labels in the glossary.
- Mandatory Mermaid. A diagram stays optional.
- A live status or an open-question count inside a file.
