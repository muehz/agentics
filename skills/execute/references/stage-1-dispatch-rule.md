# Stage 1 dispatch rule

This file holds the rules that the `Stage 1 dispatch rule` section of [`SKILL.md`](../SKILL.md#stage-1-dispatch-rule)
points to.

Stage 1 dispatches one Deep-tier packet reviewer per task, with [spec-reviewer-prompt.md](spec-reviewer-prompt.md).
Before any Stage 1 or implementer dispatch, the controller pastes into the packet the text of every spec contract the
task cites (a signature, schema, state table, or example payload), so the reviewer and the implementer both see it.

## When Stage 1 runs

Decide each task by the first row that matches:

| Order | The task                                                                                                                      | Stage 1 |
| ----- | ----------------------------------------------------------------------------------------------------------------------------- | ------- |
| 1     | A **Plan divergence** entry other than a `rule` entry touches its packet                                                      | Runs    |
| 2     | It comes from an approved brief whose `**Brief review:**` line under **Decisions** says `self-read`                           | Runs    |
| 3     | It comes from an approved brief                                                                                               | Skipped |
| 4     | Its plan is trusted (below) and the task carries `**Packet review:** no`                                                      | Skipped |
| 5     | Its plan is trusted, or it is an inline task, and it has no `**Packet review:**` field and a stated **Tier** that is not Deep | Skipped |
| 6     | Otherwise                                                                                                                     | Runs    |

A plan is **trusted** when all three hold:

1. `flow status` lists its `plan.md` approval as `current`, so it was approved in this clone for this text.
2. Its `## Provenance` block has `**Written by:**` naming `decompose` and `**Result:** pass`. `failed` and `not run`
   both fail this.
3. Its plan review was independent: the latest `review plan` row does not say `by=self`, and the `**Plan review:**` line
   does not say `self-read`. A self-read earns no skip, so every task of such a plan gets Stage 1.

An inline task is the user's own already-decomposed task, given in this session. A task marked `**Packet review:** yes`
falls to row 6 and runs. Tier still selects the models.

## The self-containment check

For each skip, run the packet self-containment check in the session. The packet names every file path, acceptance
criterion, and verification command, and it does not point at the plan file. A task that cites a spec contract carries
that contract's text. A packet that fails this check gets Stage 1. Step 1 sub-step 8 runs these checks and records each
result and skip for the preview.

## Re-review after a divergence

A Stage 1 verdict from Step 1 holds for the rest of the run unless a divergence touches the packet. An entry touches a
packet when it adds or removes one of the packet's edges, or when a shipped change touches a file the packet's
`**Files:**` list names.

A packet whose implementer has not been dispatched is reviewed again each time an entry other than a `rule` entry
touches it, up to two re-reviews. Count them as the packet's `review` rows of kind `packet` written after its first `ok`
packet review. A third touch writes `block <task-id>` and asks the user whether to review the packet again, run it as it
is, or replace it. No packet waits for a review its budget forbids. Packets the entry does not touch keep their
verdicts. A packet whose implementer is already dispatched is not reviewed again; the entry travels to its implementer
and its group's reviewers.

## The user's waiver

Skipping Stage 1 is a process gate ([gates.md](../../routing/references/contracts/gates.md#process-gate)). Name the cost
once: an unreviewed packet can send an implementer to build from a wrong or incomplete packet, and the group's quality
review then catches the miss at the expensive end. On the next reply that still declines, comply and write one
`waive stage-1` row whose `why` names the tasks, and the user's reason or `none given`. The preview and the run report
show `Stage 1 review skipped — user override`, taken from that row. A resumed run reads the row and does not ask again.

The waiver covers Stage 1 only. The group's quality review still runs.

In `pair`, Stage 1 follows the same rule through a reviewer subagent. Announce each skip and its evidence before you
implement, so the user can ask for the review anyway.
