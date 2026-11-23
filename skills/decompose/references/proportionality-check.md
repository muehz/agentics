# Proportionality check

The rubric Step 5 runs on the finished plan, after the Plan Validation Checklist and before the coverage check and the
plan review.

The Plan Validation Checklist asks whether each task can be executed. This check asks whether the plan as a whole is the
right size for the outcome the user asked for. A plan can pass all ten checklist rows and still be four times the work
the request needs. The plan can also aim at machinery nobody asked for.

The check is **two-sided**. It rejects a plan that carries machinery the outcome does not need. It also rejects a
smaller replacement that no longer performs the outcome. A plan cut down to a document that describes how a person could
do the work by hand fails this check. The requested command still does not run.

## The one required artifact

Every plan names two things in its Goal block:

- **Outcome proof.** The exact command a person runs, and the observable result it must produce. Not "the feature
  works".

**Bound the proof to the request.**

Every clause in the Outcome proof traces to a phrase in the request or the approved design's goal. A clause you cannot
trace is scope the plan added. It belongs in optional confidence work, not in the proof.

The removal test measures against the proof, so a wider proof makes previously-optional machinery on-path. Every task
turns `outcome`. All five signals go quiet on a plan that grew exactly as much as before.

"Publishes a release with its artifacts attached" is a proof. "Publishes a release, revalidates every checksum, reruns
without duplicates, and prints cleanup commands" is that proof with three optional guarantees attached.

**One test decides whether a candidate proof is real: could this command pass if the thing the user asked for were never
wired up?** If yes, it is not an Outcome proof. A suite that drives the real path and fails when the deliverable is
missing qualifies. A suite whose collaborator is a stand-in that always answers correctly does not qualify, because it
passes on the machinery alone.

The third scenario in **Worked findings** is an example, at a smaller size: a branch full of green tests and no command
anyone could run.

Name the real command. Point it at a scratch target when the real one is expensive or destructive. Do not replace the
thing that the command proves with a stub of itself.

- **Skeleton path.** The ordered task IDs that get that command to produce that result.

The skeleton path is the walking skeleton: the thinnest route that runs end to end. It may be slow, unhardened, and
missing every optional guarantee. It must run. It is one task when one task can carry it, and a short serial chain of
`outcome` tasks when the work crosses layers. Work that the command cannot run without is part of the path, not
something ahead of it. Use the removal test below to decide, never how plumbing-like the task looks.

**Rule: nothing supports a thing that does not run yet.** No `support` task runs before the skeleton path completes.
Reliability, evidence capture, isolation, and hardening need something that already works to build on. Without it, they
rest on a guess.

**This rule never changes task sizing.** Checklist row 3 still governs: each task fits one agent turn. Never merge
server, client, and UI tasks into one oversized task so the skeleton fits a single slot. Three small serial outcome
tasks with nothing else ahead of them satisfy this rule exactly.

If the plan cannot name an Outcome proof, that is the finding. With no one-command, one-result measure of success, the
plan has nothing to be proportionate against, so every later signal is unreadable.

## Task roles

Each task carries `**Role:** outcome` or `**Role:** support`. One test decides it.

**The removal test.** Delete the task from the plan. Does the Outcome proof command still produce its named result?

| Answer | Role      | Meaning                                                                                                                          |
| ------ | --------- | -------------------------------------------------------------------------------------------------------------------------------- |
| No     | `outcome` | The task is on the path. The command cannot produce its result without it.                                                       |
| Yes    | `support` | The command still works. The task makes the outcome safer, faster, more observable, or more provable, and it is not the outcome. |

Plumbing on the path is `outcome` and belongs in the skeleton path:

- a schema the new hook cannot register without
- a dispatcher case the event cannot reach without
- a minimal staging directory the publish step writes to.

The durable, resumable, exhaustively-validated versions of these tasks are `support`. The same test separates the two
halves of one subject:

| Task                                                       | Removal test                                | Role      |
| ---------------------------------------------------------- | ------------------------------------------- | --------- |
| A temporary directory the publish step writes artifacts to | Publishing stops                            | `outcome` |
| Atomic staging with a resume protocol for interrupted runs | Publishing still works                      | `support` |
| A dispatcher case so the new event reaches the handler     | The hook never fires                        | `outcome` |
| A transcript extractor that proves the hook fired          | The hook still fires                        | `support` |
| Retry with backoff around the publish call                 | Publishing still works on a healthy network | `support` |

**A task's own tests carry that task's role.**

The plan template puts a failing test inside every task. That test is part of the task, not a separate thing to count.
`support` names a task whose _own deliverable_ is machinery. This check never asks anyone to write fewer tests for
outcome work.

`execute` reads the same field. A `support` task that repeatedly fails review is the signal this check exists to catch.
`execute`'s **Two-strike stop** asks the user whether that machinery should exist.

## The five signals

Count every signal on every plan. Report all five results, fired or not, so the numbers are visible even on a plan that
passes.

| #   | Signal                   | Fires when                                                                                    |
| --- | ------------------------ | --------------------------------------------------------------------------------------------- |
| 1   | Skeleton position        | Any `support` task runs before the skeleton path completes, or no skeleton path is named.     |
| 2   | Support share            | Support tasks equal or outnumber outcome tasks.                                               |
| 3   | Unexamined bespoke build | Any component has no named alternative, or the reason is "we need more control".              |
| 4   | Environment fit          | Any defence has no matching actor, or any user-environment effect is not written in the plan. |
| 5   | Verification stop        | No stop condition, or the plan does not separate the two kinds of evidence.                   |

Count each signal as follows:

- **Signal 1, skeleton position.**
    - Count the tasks that run before the skeleton path completes.
    - Report how many, and how many of those are `support`.
- **Signal 2, support share.**
    - Compare `count(support tasks)` against `count(outcome tasks)`.
    - Report both numbers and the plan's total.
- **Signal 3, unexamined bespoke build.**
    - List every new state machine, persistent store, process manager, wire schema, extractor, or protocol the plan
      builds.
    - For each one, the plan's `**Existing tools checked:**` line in `## Approaches considered` names the existing
      repository framework, native platform tool, or provider tooling it checked, with one line for why that tool does
      not fit.
- **Signal 4, environment fit.**
    - List every task that defends against an actor or failure the stated execution environment cannot produce.
    - List every task that changes the user's own environment:
        - an authentication prompt
        - a browser launch
        - a profile or global config mutation
        - an installed-tool state change.
- **Signal 5, verification stop.**
    - The plan's `**Verification stop:**` constraint names the evidence that ends verification and the release-blocking
      verification tasks.
    - Its `**Optional confidence work:**` constraint names every other verification task, or reads "none".

### Reading signal 3

Look first at the existing repository framework, the alternative plans most often skip. Before a plan builds a
dispatcher, a manifest format, a normalization layer, or a conformance runner, it says which one the repository already
ships. It also says why that one cannot carry the change.

### Reading signal 4

Two different findings share this row.

- **A threat with no actor.** A local temporary file defended against a same-user attacker, on a machine where that
  attacker already has the user's shell. Name the actor the environment can actually produce. Cut the defence that
  answers a different one.
- **An effect on the person running this.** A validation strategy touches the user's working session if it does any of
  these:
    - opens browser windows for authentication
    - swaps an installed plugin
    - mutates a trust store.

    The strategy goes in the plan where the user can see it before it happens, or it does not go in the plan.

### Reading signal 5

Verification without a stop condition grows for as long as anyone looks at it. The plan says which evidence closes the
question, and what is optional beyond it. "One smoke test against the real thing" is a complete answer.

## Dispositions

When one or more signals fire, do not record `pass` silently. Take one of these, in this order of preference (the
attended-session exception below changes which comes first when signal 2, 3, or 4 fires):

1. **Reorder.** Push every support task behind the skeleton path. When only signal 1 fires, that reordering alone is
   often the fix, and it costs nothing but the ordering. Reorder, never merge. The tasks keep their sizes.
2. **Resize.** Move support tasks out of this plan into a named follow-up. Defer them. Do not delete them. For each one,
   write the demonstrated demand that would bring it back. "Add retry when a real run fails on a transient error" is a
   demand. "Add retry for robustness" is not. A resize never defers a task whose `**Covers:**` field names a source item
   that no remaining `outcome` task covers. The source requested that item, so it is not machinery nobody asked for, and
   deferring it would only make the coverage check block on it.
3. **Slice.** The signals fire because the input is more than one shippable increment. Revisit Step 1's delivery
   boundaries with `slicing.md`. Preserve the source. Update the shared plan.
4. **Override.** The user says the size is right. Record it. Then continue.

In an attended session, when signal 2, 3, or 4 fires, the Proportionality statement is the first thing the user reads,
and its **Smallest version** is the resize: the deferred support work, each item with the demonstrated demand that would
bring it back. A reorder cannot cure these signals, because it keeps the machinery in the plan. Reorder stays the first
disposition when only signal 1 fires. Signal 5 alone keeps the handling above. An
[unattended](../../routing/references/contracts/unattended.md) run never resizes. It reorders what it can and records
`reordered` with the unresolved signal named (see Waivability).

After any reorder, resize, or slice, **run the check once more on the changed plan.** A resize that removed the
skeleton, or replaced execution with documentation, fails the second side of the check. The second run reports what
remains and takes no further disposition, so the check ends.

## Waivability

This is a **process gate** ([gates.md](../../routing/references/contracts/gates.md)). State the cost once, in the shape
below. Then let the user answer:

> **Proportionality:** \<the signals that fired, with their numbers\>. **Cost of proceeding:** \<what the extra work
> buys nobody\>. **Smallest version:** \<the reordered or resized plan in one line\>. Proceed as planned, or take the
> smaller one?

In an attended session, when signal 2, 3, or 4 fires, this statement is the first thing the user reads, and the
**Smallest version** is the resize: the deferred support work, each item with the demonstrated demand that would bring
it back. When only signal 1 fires, the **Smallest version** is the reorder.

When the next reply still declines the smaller version, in any wording, do these steps:

1. Comply immediately.
2. Record `override by user` in the Provenance line, and write one `waive proportionality` row with the reason the user
   gave, or `none given`. Ask for no reason.
3. Plan the full version.

Do not argue a second time. The user may know a constraint the plan does not carry.

**Write `override by user` only when a user actually overrode.**

It records something a person said, under the same honesty rule as every other field in the plan. A signal you could not
resolve, an assumption you made on the user's behalf, or an unattended session is not an override. Record the real
disposition instead. Name the unresolved signal. Let `execute` see it.

An overridden proportionality finding never waives a checklist row.

## What goes in the Provenance block

One line, which `execute` reads:

```text
- **Proportionality:** pass — skeleton `task-1`→`task-3`, nothing ahead of it; 6 outcome / 2 support tasks; signals fired: none
- **Proportionality:** resized — skeleton `task-1`, nothing ahead of it; 4 outcome / 1 support tasks; signals fired: 2, 3 (9 support tasks deferred to follow-up)
- **Proportionality:** override by user — skeleton `task-58`→`task-61`, 57 tasks ahead of it, 44 of them support; 12 outcome / 118 support tasks; signals fired: 1, 2, 3, 4
```

Write the real counts. Never claim `pass` on a check that did not run. Such a line removes a stop that would otherwise
fire in `execute`.

## Worked findings

The three plan shapes below are the source of this check. Each plan passed task-level validation.

**A release workflow decomposed into 130 serial tasks.** Signals 1, 2, 3, and 4 fired. The skeleton path was the actual
integration with the provider's release tooling. That path started past task 60, behind support tasks that hardened
local temporary files against a same-user attacker and normalized license identifiers exhaustively. The plan never named
native release tooling. Disposition: resize first (signals 2 to 4 fired in an attended session), then reorder what
remains.

**A session-start hook on an existing hook framework.** The product change was small, correct, and already running.
Signals 2 and 5 fired. The plan's verification half grew with no stop condition, until it became the dominant work
product. It added a transcript extractor, an adversarial mutation suite, isolated provider profiles, and lineage
hashing. Signal 4 also caught the plan's authentication steps, which opened browser windows during the user's unrelated
work. Disposition: resize to one practical smoke test against the real provider, which passed immediately.

**A cross-provider evaluation launcher.** Signals 1, 2, 3, and 4 fired. The branch held 48 files and 14,238 lines of
scheduler, storage, retry, provenance, isolation, and process-tree containment, with no user-runnable command anywhere.
The machine already had installed, authenticated CLIs: signal 3's named alternative, already in place. The first
correction replaced the whole branch with a manual document. **That correction failed the second side of this check**,
because the requested command still did not run. The proportionate plan is the small launcher: it starts the installed
CLIs, hands each the same frozen packet, and collects each response. Disposition: resize to the launcher, with the
scheduler, resume protocol, and containment work deferred against named demands.
