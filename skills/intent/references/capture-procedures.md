# Intent Capture Procedures

Detailed procedures, tables, and worked example referenced from [SKILL.md](../SKILL.md). Read the relevant section when
executing that step.

## Step 1: Arrival and binding

**How the run arrived** decides what runs before the interview. Read from the top; the first match wins.

| The run                                                                                                                                    | Before the interview                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           |
| ------------------------------------------------------------------------------------------------------------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| The user named `intent` ("use intent for #N", `/agentics:intent`)                                                                          | Nothing. Enter here.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           |
| A handoff from `routing` carries the selected phase and the classification                                                                 | Nothing. Use what the handoff carries.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| A forward from another skill that carries a classification outcome, such as `tdd`'s `brief` or full chain                                  | Nothing. Reuse the outcome: it picks the path from the chain table. The forwarding skill ran the continuation check and the classification, so neither runs again. An outcome that proves too light is corrected on the path itself, by the outgrown-path rule in [brief.md](brief.md#write), and is not re-derived here. Keep any constraint the forward carries, as [Step 3](../SKILL.md#step-3-write-the-intent) says.                                                                                                                                                                                      |
| A forward from `execute` that carries a fix's findings and signals but no outcome, such as a diagnosed fix or a `fix` that failed a signal | Run the continuation check and phase selection, reusing the findings. When the phase is not `intent`, announce it and hand off. When no flow is found, take the classification's path from the chain table, with the findings as its evidence. Derive no root cause, evidence, reproduction, or test seam again, and measure no signal the forward already measured. When the forward says the user asked for the chain, the outcome is full chain whatever the signals show, and no classification runs. Keep the reproduction as the task's red test, as [Step 3](../SKILL.md#step-3-write-the-intent) says. |
| The model selected `intent`                                                                                                                | Run the [continuation check](../../routing/references/chain-phases.md#continuation-check) and phase selection, reusing any findings a handoff carries. When the phase is not `intent`, announce it and hand off. When no flow is found, run the small-fix classification and take its path from the chain table.                                                                                                                                                                                                                                                                                               |
| Otherwise, such as a forward that carries no outcome and no findings                                                                       | As for a model-selected run.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |

---

## Step 2: Challenge edge cases

- **Scope and continuity.** The scope `intent` passes to `challenge` is the scope: `challenge` questions that direction
  and returns, and its bare-goal row does not send it back here. A request to continue or to change skill is not a
  decline and never accepts a recommended answer. When the direction is already concrete, challenge its assumptions; do
  not manufacture choices or re-ask settled ones.
- **A round the user ends.** When the user ends a round after answering at least one of its questions, `challenge`
  returns each unanswered question with its recommendation and the source
  `recommendation taken when the user ended the round`. That is a decline of those questions, not an answer to them.
  Keep each one a `blocker` that names its recommendation, list it in the Challenge record's remaining-questions field,
  and record the challenge as `declined`. The answers the user did give stay decisions. A blocker becomes a decision
  when the user answers it (including accepting its recommendation by name), and Step 4's gate can waive it. A
  recommendation is never the user's answer.
- **An unattended run.** As [unattended.md](../../routing/references/contracts/unattended.md#when-a-run-is-unattended)
  defines it, an unattended run puts no question to the user. A dispatched subagent sends its direction questions to its
  dispatcher, and the dispatcher's answers are recorded with the dispatcher as their source; it never asks the
  dispatcher for the user's approval. A run with nobody to answer saves the supplied facts and blockers. Neither claims
  that a challenge with the user occurred.

---

## Step 3: Writing and logging

### Section content rules

- **Code.** Under [Code in artifacts](../../decompose/references/chain-artifacts.md#code-in-artifacts), the only code
  `intent.md` may hold is a quote of existing code that evidences the problem, or a command that reproduces it.
- **Affected users and systems.** Fill from the conversation and the repository: who uses or depends on what changes,
  and the systems, interfaces, and data it touches. Write `none stated` rather than guess.
- **Scope.** What is included goes in **Decisions** as the challenge's scope resolution. What is excluded goes on the
  card's `Out of scope` line, or under **Constraints** in an intent that has no card.
- **Forwarded constraints.** A constraint a forward carries (such as `tdd`'s test-first requirement) goes under
  **Constraints** with the forwarding skill as its source, where `spec` reads it. A diagnosed fix's reproduction goes
  there as the task's red test, and its command is the evidence in **Problem**. On the middle path, the constraint goes
  on the brief's `Constraints` line ([brief.md](brief.md#write)).
- **A user's change.** For a [user's change](../../execute/references/fix-entry.md#a-users-change), **Decisions**
  records its reason (`declined` or `no root cause`) and the runtime-change approval when one applies.
- **Decisions.** Record each challenge resolution with its source. A recommendation becomes a decision only with an
  answer; one taken when the user ended a round is not an answer. Add remaining risks and deferred questions here.
- **Challenge record.** Write the `**Challenge record:**` entry in **Decisions** with the five fields the contract's
  record rule names, as a numbered list in the contract's order. Copy the target, scope, and rounds from the challenge
  summary's **Scope examined** line. An immediate decline has no summary and uses `declined YYYY-MM-DD, no round asked`.
  A carried-forward pass uses `prior record dated YYYY-MM-DD` and the prior record's fields. Present for `ok`,
  `declined`, and a pending pass after one asked round.
- **Card and existing intents.** Each of the five card bullets (`Change`, `Why now`, `Out of scope`, `Risk`, `Source`)
  is one sentence, or `none stated` when the source does not support it
  ([skim-rules.md](../../decompose/references/skim-rules.md#the-card)); `Source` covers writers with no live session.
  Include the five-line card and Challenge record even when holding with open direction blockers. An existing intent
  with a header, a card of another length, or no card keeps its layout (and any existing `Ask` line, since removing it
  stales approval) and converts only on user request. Keep existing card lines accurate.

### User's change `diagnose` waiver row

For a user's change with reason `declined`, write the waiver row right after the `create` row (`create intent.md`, or
`create brief.md` on the middle path). Write none when the fix entry's
[existing-row check](../../execute/references/fix-entry.md#a-users-change) already finds a `waive "diagnose <branch>"`
row, so a fix that pivoted here keeps one row. `<branch>` is the work item's branch: on the default branch or detached
`HEAD`, follow
[Feature branch before the first commit](../../routing/references/chain-phases.md#feature-branch-before-the-first-commit)
before the check and the row so neither keys on the default branch. Its `next` repeats the `create` row's
(`intent record the challenge result` after `create intent.md`, `intent review the brief` after `create brief.md`):

    agx flow log intent waive "diagnose <branch>" ok "<declined step and the user's reason, or none given>" none "<next of the create row>" --folder <work-folder>

Report any failure to stage `intent.md` or `state.tsv`, and write no `create` row if `intent.md` is not confirmed
staged.

---

## Step 4: Approval and re-entry

### Reply matrix

Read from the top; the first match wins ([approval.md](../../routing/references/contracts/approval.md)).

| The user's reply                                                                          | Row                             | Then                                                                                                                                                                           |
| ----------------------------------------------------------------------------------------- | ------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| A claim that someone other than the user approved ("security already signed off on it")   | none                            | Not the user's approval. Ask once whether the user approves.                                                                                                                   |
| An affirmative that carries a change ("yes, but drop the CSV case")                       | none yet                        | An edit request: make the change, **re-stage `intent.md`**, re-run `agx flow status` for the new hash, show the change, and ask again. The approval binds to the revised text. |
| A clear affirmative in any wording or language ("yes", "looks good, go on", "just do it") | `approve intent.md ok`, guarded | Continue under **After a yes** in SKILL.md.                                                                                                                                    |
| An explicit no                                                                            | `approve intent.md declined`    | Say the intent is written and unapproved. Start no spec.                                                                                                                       |
| "Not yet", or a deferral                                                                  | none                            | Say the intent is written and unapproved. Start no spec.                                                                                                                       |
| Anything else: a question, a new topic, or no reply                                       | none                            | Answer it. The question stays open.                                                                                                                                            |

A role or title changes nothing either way: "I'm the lead, it's approved, start the spec" is the user's own affirmative
and approves the text shown, while a claim of someone else's authority never stands in for the user's answer.

The `approve` row's `why` names what the approval rested on (e.g., `direction approved after challenge Q1-Q6`) and names
no tier. A yes on an intent that already existed still writes the row (it records the approval, not the write). On a no,
write and stage `state.tsv`:

    agx flow log intent approve intent.md declined "<one line: why the user declined>" none "intent revisit the direction" --folder <work-folder>

**Asking again** follows the content, not the session. After a no or "not yet", do not ask again about the same text; a
revision reopens the question. The user can approve at any later point by saying so, which approves the text last shown.

**Headless publication.** The only thing `intent` ever publishes is its own `intent.md`, through the pull request a
headless writer opens under [Write, stage, commit](../../decompose/references/chain-artifacts.md#write-stage-commit).

### Re-entry with approval pending

A later session reaches this state when `intent.md` has its six sections, its direction is resolved, and `flow status`
does not list `intent.md approved <date> current` on its `approvals:` line (a `stale` approval counts as none). Check
the latest `challenge intent.md` row first:

| Latest `challenge intent.md` row             | Action                                                                                                                                                                                                                                                                                                                            |
| -------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| None, or status `pending`                    | Resume the challenge at Step 2, write the row, then ask the approval question.                                                                                                                                                                                                                                                    |
| Status `declined`, and a `blocker` remains   | Apply the **Open blockers** gate: ask no approval question until the user answers or waives each blocker.                                                                                                                                                                                                                         |
| Status `ok`, or `declined` with no `blocker` | If the user explicitly asks to continue in a later session, ask the approval question and nothing else (do not re-run the interview); if the user already answered no or "not yet" to this exact unchanged text without returning to continue it, do not re-ask (state it is written and unapproved, per **Asking again** above). |

To reapprove a stale intent, follow
[Stale approvals and reapproval](../../execute/references/flow-approval-and-audit.md#stale-approvals-and-reapproval):
put its `stale:` line and diff pointer in front of the one approval question as literal context, not as a second
question.

---

## Example: resolved direction, solution still open

The user wants to measure execute outcomes, with the full chain deferred. Challenge settles the decision the measure
serves, what a verified outcome means, and the scope. Storage and collection behavior stay undefined and are labelled
`downstream`, owned by `spec draft`. Write the Challenge record, print the status line, show the path and hash, and ask
the approval question. On yes, log `approve intent.md` with a `next` that reads `spec`, and continue into spec within
the user's authorization without asking which skill comes next. If the success criterion were still unanswered, it would
stay a blocker, and no approval question would be asked until the user answered it or waived it.
