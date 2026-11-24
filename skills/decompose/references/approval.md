# Approval questions

Step 6 of `skills/decompose/SKILL.md` applies this file after the offer gate passes and the tier is known. What counts
as an approval, an edit request, and asking again is in [approval.md](../../routing/references/contracts/approval.md).
How a row is written, hashed, and guarded is in [flow-state.md](../../execute/references/flow-state.md).

## Which question

Read the rows in order. The first match wins.

| #   | Case                                                                                                                   | Questions                                                          |
| --- | ---------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------ |
| 1   | The user named execution up front ("plan this, then hand it to execute"), and the spec, if any, has a current approval | None. Show the plan as below, then write the `approve plan.md` row |
| 2   | The plan derives from a spec with no current approval, and the final tier is routine                                   | The combined question                                              |
| 3   | The plan derives from a spec with no current approval, and the final tier is elevated                                  | The spec question, then, after a yes, the plan question            |
| 4   | Otherwise: the spec has a current approval, or the plan derives from no spec                                           | The plan question                                                  |

Ask each question through exactly one mechanism, once per version of the content. On Copilot CLI and the Copilot app,
when the plan load succeeded, ask it through the session's plan-approval tool if it exposes one, else through
`ask_user`. On every other host, or when the load or the tool failed, ask it in chat.
[native-plan-projection.md](../../routing/references/native-plan-projection.md) names the tool for each host. Every
mechanism writes the same rows.

## Before a question

A session plan ([session-plan.md](session-plan.md)) has no work folder, so steps 2 and 3 do not apply to it. Show it by
its absolute path with no hash, and leave the plan's `(<hash>)` out of the question. A yes to it writes no
`approve plan.md` row and no `order <n>` row, and nothing is staged or committed: `execute` materializes the plan into a
work folder and records the approval there. A spec that the same question covers still takes every step and its own
rows.

1. Format the artifact with the repository's own formatter when one covers its path, so a commit-time formatter finds
   nothing to change.
2. Confirm the artifact is staged, or in the work folder under the `local` storage setting.
3. Run `agx flow status --folder <work-folder>` and note the hash on its `hashes:` line for each artifact the question
   covers. That hash names the content the user approves.
4. Show the artifact by its path and that hash, with a summary of at most five lines: the goal, the size, the tier with
   its plain meaning from [vocabulary.md](../../routing/references/contracts/vocabulary.md), and what the user should
   look at. The full text goes to chat only on request.
5. Show what the reviews left open. For the plan: each `**Plan review finding (presented):**` line, each coverage
   warning, and `self-read (not independent)` when the plan review was a self-read. For a spec: each finding the spec
   records under **Further Notes** as `Spec review finding (presented)`, the `**Spec review:**` line's counts, and
   `self-read (not independent)` when that line records a self-read.

On Copilot, when the combined question goes through the question tool, show the summary and the findings in a chat
message first. The load writes the first lines of the Copilot plan file, so it runs after the tier check, and those
lines state that a yes also approves `spec.md` at `<spec-path>`.

## The questions

**The plan question:**

"Plan `<plan-path>` (`<hash>`) is ready. Approve it for execution (via `/agentics:execute`)? Your yes is the approval
`execute` requires, and implementation starts on it."

When the Plan Validation Checklist was waived or the plan challenge was declined, replace `is ready` with the actual
status naming what was waived or declined (for example,
`Plan <plan-path> (<hash>) is saved (checklist waived; plan challenge declined). Approve it for execution (via /agentics:execute)?`).

**The spec question:**

"Approve spec `<spec-path>` (`<hash>`)? This plan raised the tier to elevated (`<signals>`), so the spec is approved on
its own before the plan."

**The combined question.** It covers two artifacts, so it offers an answer for each:

"Spec `<spec-path>` (`<spec-hash>`) and plan `<plan-path>` (`<plan-hash>`) are written. Approve both: the spec, and the
plan for execution (via `/agentics:execute`)? You can approve both, the spec alone, or neither. A yes to both is the
approval `execute` requires, and implementation starts on it."

`<plan-path>` is the destination actually written this run: the repository path, or the session plan file when the user
asked for an ephemeral plan.

## The rows each answer writes

Every `approve` row with status `ok` passes `auto` as the evidence and the noted hash as `--expect-blob`, so the
[approval guard](../../execute/references/flow-approval-and-audit.md#the-approval-guard) writes it only when the
artifact is the one the user was shown. When the guard refuses, the artifact changed after it was shown: run
`flow status` again, show it again, and ask again. Never write the row with other evidence to get past the guard.

The `why` names the tier, the signals that fired, and what the approval rested on, as
`<tier>: <signals or no signal fired>; <basis>`, for example
`elevated: Deep task; approved after the plan challenge and one plan review`. A plan with no spec reads
`n/a: no spec; <basis>`.

| Answer                                       | Rows, in order                                                                                                     |
| -------------------------------------------- | ------------------------------------------------------------------------------------------------------------------ |
| Yes to the plan                              | `approve plan.md` ok, then one `order <n>` row per standing order, as [save-and-stage.md](save-and-stage.md) shows |
| Yes to the spec question                     | `approve spec.md` ok, with `next` `decompose ask for approval`; then ask the plan question                         |
| Yes to both on the combined question         | `approve spec.md` ok, then `approve plan.md` ok, then the `order <n>` rows                                         |
| The spec alone on the combined question      | `approve spec.md` ok, then `approve plan.md` declined                                                              |
| No to a question, or neither on the combined | `approve <artifact>` declined for each artifact the question covered                                               |
| "Not yet", or no answer                      | No `approve` row                                                                                                   |
| An edit request ("yes, but change X")        | No `approve` row. Make the change, rerun what Step 5 says a revision needs, show it, and ask again                 |

The commands:

    agx flow log decompose approve plan.md ok "<tier>: <signals>; <basis>" auto "execute run the plan" --folder <work-folder> --expect-blob <plan-hash>
    agx flow log decompose approve spec.md ok "<tier>: <signals>; <basis>" auto "<next>" --folder <work-folder> --expect-blob <spec-hash>
    agx flow log decompose approve plan.md declined "<one line: what the user said>" none "decompose revise the plan" --folder <work-folder>
    agx flow log decompose approve spec.md declined "<one line: what the user said>" none "spec revise the spec" --folder <work-folder>

The `<next>` of an `approve spec.md` row is `execute run the plan` when the plan row follows in the same answer, and
`decompose ask for approval` otherwise. A no records no reason the user did not give: the `why` quotes or summarizes
what they said.

After the rows, Step 6 stages `state.tsv` and, on a yes to the plan, commits. A turn that ends with no plan approval
ends with a `pause plan.md` row whose `next` names what remains.

## Asking again

After a no or a "not yet", do not ask again about the same text. Any revision reopens the question, and the user can
approve at any time by saying so. A user who later withdraws an approval they gave, in this skill's turn, writes
`revoke <artifact>`:

    agx flow log decompose revoke plan.md ok "<one line: what the user said>" none "decompose revise the plan" --folder <work-folder>

To reapprove a stale or unconfirmed plan or spec, follow
[Stale approvals and reapproval](../../execute/references/flow-approval-and-audit.md#stale-approvals-and-reapproval).
