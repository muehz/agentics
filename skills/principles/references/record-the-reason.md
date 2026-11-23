# Record the Reason

_Apply before you change behavior, defaults, interfaces, or contracts._

Before you make behavioral edits, record why the change is needed and the relevant trade-off. A diff shows what changed.
The reason lets the next reader judge whether to keep that choice when circumstances change.

Use the project's existing issue, design note, or change description. A few sentences may suffice. Reuse a recorded
decision that already covers the work. This principle requires neither Agentics files nor a new documentation suite. In
Agentics, follow the canonical [intent rule](../../intent/SKILL.md#when-the-chain-is-required), including its
exemptions, existing-approval handling, and waiver. Do not invent a second process here.

Name the problem or constraint, the choice it supports, and the relevant cost or rejected alternative. When evidence
informs the choice, include its source. Do not manufacture alternatives for a routine decision. Record only the
trade-offs that matter to it. Keep an assumption labeled as an assumption. State how you will check it.

For example, "raised the batch limit" describes an edit but does not explain it. A reason would connect that choice to
an observed workload constraint. It would also explain why a larger batch was rejected, if that comparison actually
happened. The record should let a reader find the underlying observation. A reader should not have to trust a plausible
story.

If the existing diff has no recorded reason, say so. Inspect the available history and evidence. Never invent past
measurements, motives, or rejected alternatives to justify it, even when a maintainer asks repeatedly. If new evidence
supports the change, record that as a decision made now, not as recovered history. If evidence is missing, name the gap
and the next check. Do not turn a guess into a historical fact.

A reason is not proof that the behavior works. Verify the changed path separately under
[Prove It Works](prove-it-works.md). Report a missing check even when the rationale is sound.

## Self-test

- Inspect the actual diff and its decision record. For each behavioral choice I changed, is there a recorded reason?
  Count the choices with none. The target is zero.
- Does each recorded reason name the problem or constraint behind the choice, and not only restate the edit?
- Does each claimed observation cite a source, and is each unknown part of the history and each untested assumption
  labeled as one?
- Does the record name the relevant trade-off or rejected alternative?
- Was the reason recorded before my behavioral edit, or marked as a later decision based on new evidence?
- Did I use an existing place for the record, and require no unrelated project artifact?
- Did a check exercise the changed behavior? Name the command. List what remains unverified, and count the items.
