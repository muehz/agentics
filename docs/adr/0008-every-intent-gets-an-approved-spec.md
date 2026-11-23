# Every intent gets an approved spec

> **Amended (2026-11-11, #1311) by [0013](0013-proportional-delivery-paths.md).** A middle-path change gets a reviewed
> and approved brief in place of the intent, the spec and the plan; every other change keeps the intent approval, spec
> review and tiers below.

> **Amended (2026-11-11, #1311) by [0021](0021-delegation-and-severity.md).** The spec reviewer runs at Deep unless the
> user selects Max, not on the most capable model rung every time; the labelled self-read on a host that cannot dispatch
> now covers the plan review too.

> **Amended (2026-10-17, #1111).** The trail is now `state.tsv`, written by `flow`, so the `events.tsv` rows and the
> `next-owner` header field named below no longer exist. An `approve intent.md` row goes to `state.tsv`, and the next
> phase is the first word of the current row's `next` (see `skills/execute/references/flow-state.md`). The chain-state
> code under `go/hooks/internal/chain` is gone. This record keeps its original wording for the decision it made.

Two written chain rules are reversed together on 2026-10-11. Both shaped what happened between capturing a problem and
planning its tasks.

The first was in the provenance contract (`skills/decompose/references/chain-artifacts.md`): "An intent carries no
approval gate, because nothing executes an intent. The gate lives on `spec`'s `approved:` and on `decompose`'s
phase-list review." The user's first explicit approval came at the spec, or, when there was no spec, at the plan.

The second was in `skills/intent/SKILL.md`: intent chose its next owner, `spec draft` when solution behavior or
interfaces were still undefined and `decompose` when the requirements already supported concrete tasks. "Size and
increment count do not select the route." A spec was optional, and an intent judged concrete enough went straight to a
plan.

## Decision

**An intent is approved.** Intent ends with one approval question once the direction is resolved. A yes appends an
`approve intent.md` row to the work folder's `events.tsv`; an intent header gains no `approved:` field. No spec work
starts without that row. An unattended run leaves the intent unapproved and says so.

**Every intent-started work item gets an approved spec.** Intent always records `next-owner: spec`. `decompose` no
longer accepts an intent as its source when the work started at intent. A small change gets a short spec; the spec is
sized by its content.

**Every spec gets an independent review.** A fresh subagent, with none of the drafting session's context, reviews the
spec against a fixed charter before any approval question, whatever the tier. The independent spec review is not
waivable. On a host that cannot dispatch a subagent, the author reads the spec against the same charter once, records it
under **Scope Decisions** as a self-read, and the approval question says the review was not independent.

**Approval follows a risk tier.** A spec is elevated when it changes a public interface or contract, touches auth, a
trust boundary, or data handling, or involves a migration; `decompose` adds plan-level signals. Otherwise it is routine.
The user can raise the tier and can never lower it. On the elevated tier, the spec and the plan are approved separately.
On the routine tier, `decompose` drafts the plan against the reviewed, unapproved spec and asks one question that
approves both. Each `approve spec.md` and `approve plan.md` row names the tier and the signals that fired. An
`approve intent.md` row names no tier, because the tier is unknown until the spec exists.

## Why

An unapproved intent let the agent build a spec on a direction the user had not confirmed. The challenge settles the
direction, but only the user's yes makes it the user's. Asking once, at the end of capture, costs one question and stops
a whole spec from being drafted on a misread problem.

The optional spec made the route a judgment call about the work's precision, made by the same agent that would then plan
it. An intent read as concrete skipped the one artifact that defines behavior, interfaces, and success criteria before
tasks exist, and skipped the review that would have caught what the intent left open. A required spec, sized by its
content, makes the definition step uniform. The independent review exists because the author of a spec is the
worst-placed reader of its gaps. Tiers keep the cost proportionate: a routine change pays one approval question for the
spec and the plan together, and an elevated change keeps two.

## What this costs, stated plainly

**One more question for every intent.** Capture used to end without asking. It now ends with an approval question, and a
refusal holds the work at intent across sessions.

**One more artifact for small work.** A change that intent used to send straight to `decompose` now gets a spec, even
when that spec is a few lines.

**A subagent dispatch for every spec.** The review runs on the most capable model rung, every time. On a host that
cannot dispatch, the self-read is weaker, and the approval question says so rather than hiding it.

**A plan drafted before its spec is approved.** On the routine tier, a no to the combined question can discard a plan
already written.

## Considered and declined

**Keep intent's choice between spec and decompose.** Declined. The choice rested on how concrete the intent looked, and
nothing checked that judgment before planning started.

**Choose the route by size.** Declined. Size is not the risk. A short change to a public contract needs a reviewed
definition more than a long mechanical one does.

**Let the user waive the spec review.** Declined. A waivable review becomes the default under time pressure, and the
routine tier's combined approval relies on the review having run.

**Approve the intent through a header field.** Declined. An `approved:` field on the intent would make intent look
executable to readers that check that field. The approval is recorded as an event instead.

## Consequences

Routing holds a six-section intent with no `approve intent.md` row at `intent` and asks only the approval question. The
`## Affected users and systems` section marks an intent written under these rules.

Work folders written before this change keep their recorded route. A five-section intent with `next-owner: decompose`
still selects `decompose`, needs no `approve intent.md` row, and may be planned from the intent. The chain-state code
under `go/hooks/internal/chain` still honors `next-owner: decompose` for that reason.

Reverting this decision means restoring both rules in `chain-artifacts.md` and `skills/intent/SKILL.md`, letting
`decompose` accept a new intent as its source again, and deciding whether existing `approve intent.md` rows stay as
history. They are append-only and would be left in place.
