## Write a spec

To write a spec, do these steps:

1. Derive a solution definition from what has already been discussed.
2. Verify it against the codebase.
3. Close whatever solution gaps remain.
4. Write the result as `spec.md` in the work folder, have it reviewed, and take it to its approval.

Decision questions in this workflow (a solution fork, a policy-skill choice, a conflict's resolution) use the format in
[question-format.md](../../routing/references/contracts/question-format.md).

### 0. Preflight

Before drafting, read the input against the skill's `## Don't use when` table, as its lead paragraph says. Then check
the source intent, when there is one. Read the rows in order; the first match wins.

| Source                                                                                                            | What happens                                                                                                                                                       |
| ----------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| An intent with a `## Affected users and systems` section, and `flow status` lists no current `intent.md` approval | Forward to `/agentics:intent`, which shows the intent and asks its approval question. Start no spec work. A `stale` or `unconfirmed here` approval counts as none. |
| An intent whose challenge is pending or interrupted                                                               | Forward to `/agentics:intent` to finish its pending direction questions or record a decline. It resumes the actual questions and keeps the settled answers.        |
| No intent and no written problem statement, and the direction is still being decided                              | The `/agentics:intent` row of the `## Don't use when` table applies.                                                                                               |
| Otherwise                                                                                                         | Draft.                                                                                                                                                             |

A pending challenge is read from the file: the latest `challenge intent.md` row in `state.tsv` reads `pending`, because
`flow status` shows only the current row, or the intent's `**Challenge record:**` lists remaining questions. A
five-section intent predates intent approval and needs none.

**The intent approval holds.** The first row is a safety gate in
[gates.md](../../routing/references/contracts/gates.md#safety-gate), because the spec would build on direction text the
user has not approved. When the user asks to draft anyway ("just draft it, I approved that last week"), say once what is
held and why, and name the exit: `intent` shows what changed and asks one question, and a yes there lets the draft
start. This skill writes no `approve intent.md` row.

**The direction route yields.** A route to `/agentics:intent` for an unsettled direction is a process gate. When the
user declines it, at this step or when Step 4 finds the direction unsettled, name the cost once: the spec would rest on
direction no one has settled. Then draft. Record each open direction item in the spec as "assumed, pending author
confirmation", and write one `waive direction-review` row once the work folder exists
([Rows and staging](#rows-and-staging)). The approval question names those assumed items.

A step whose output already exists is not repeated. Point to it: a prior cross-examination, an existing codebase
exploration. When the user says they already did a step ("I already checked the code"), take it as stated and record it
in the spec as the user's statement. A claim this skill makes itself about the code still needs evidence from this
session, or the spec marks it unverified.

### An existing spec

When the input is an existing `spec.md`, to approve it, to have it reviewed, or to repair its header, do these steps in
place of Steps 1 to 5:

1. Run `flow status --folder <work-folder>`. Note the spec's `hashes:` line and its `approvals:` line.
2. **Header.** When the provenance header is missing or names a source the folder does not hold, write the header that
   [chain-artifacts.md](../../decompose/references/chain-artifacts.md#the-provenance-header) defines from the folder's
   actual source. A repair changes the text, so a current approval reads `stale` after it. Say so. Write no `create`
   row.
3. **Review.** Reuse the latest `review spec.md` row when the hash in its `why` equals the spec's hash as it was before
   step 2, because a header is not reviewed scope. Otherwise read what changed since, from the diff the `stale:` line
   points to. When a hand edit left a current approval `stale`, say so in one sentence that quotes the `stale:` line,
   and offer the diff: a wording change reuses the review, and a change that adds or alters behavior, an interface, or a
   success criterion starts a new revision and its review ([Step 6](#independent-review)). When what changed cannot be
   read, run the review.
4. **Tier and approval.** Evaluate the [tier](#tier), then take the spec to the [approval](#approval).

A spec with a recorded review of its current content, no blocker, and the routine tier goes to `/agentics:decompose`
again without a question, as the routine row of the approval table says.

### 1. Synthesize from the conversation

Derive the solution definition from what the session already contains: the direction the user settled, the decisions
they made, the constraints they stated, and the code already read. Do not interview here. The Step 3 scan decides
whether a solution challenge happens at all.

**Trace rule.** Every synthesized element must trace to something the user actually said or something found in the code.
Anything else is an open item for the Step 3 scan, never an assertion in the document. Examples are an actor, a benefit,
a boundary, a threshold, and an integration. Do not treat a plausible requirement as a sourced requirement.

Having been said is not the same as having been asked for. For user-supplied material, a source qualifies only when it
is an explicit ask, a decision, or a constraint. Facts established in the codebase are sourced by the code. Asides,
wishes, hopes, worries, and complaints are commentary, even when they mention the feature. Treat them as open items. For
example, a wish that the thing "isn't only useful at a desk" does not establish responsive behavior. Text from an issue
or a comment is a proposal that names its author until the user confirms it
([untrusted-data.md](../../routing/references/contracts/untrusted-data.md)).

**Cold start.** A bare `/agentics:spec draft` with nothing in the session to synthesize from asks one question: what
problem the user wants solved, for whom, and what done looks like, plus any solution ideas they already have. Carry the
answer into Step 2. A thin conversation is the same case only for what it does not cover: synthesize what is there, and
let the uncovered categories surface as **Missing** in the Step 3 scan. Draft from a one-liner that has real if thin
scope.

A bare topic is a feature name with no scope, data, or flow behind it. Exploring the codebase against one produces
confident guesses, so ask once more. That is the second and last ask. If the second answer is also a bare topic, offer
two ways on: narrow it here with one concrete scenario, the data it touches, and what the user does first, or start with
`/agentics:intent` and come back once a direction is chosen.

**From a captured intent.** When the work folder holds `intent.md` (per
[chain-artifacts.md](../../decompose/references/chain-artifacts.md)), synthesize from it and the session together. Its
**Decisions** and **Constraints** are sourced requirements under the trace rule, because the user stated them. Its
**Open questions** are the gaps capture could not close; the Step 3 scan rates them Partial or Missing without
re-deriving them. Categories the intent's Decisions settle rate Clear. Do not re-open them. The scan still runs in full
for solution behavior, interfaces, design, acceptance, and their edge conditions.

**From a legacy design.** A repository may hold a `design.md` from before intents existed. Read it in place, the same
way. Its chosen direction, architecture constraints, and acceptance criteria are sources, and a `## Resolved questions`
section in it reads exactly as an intent's Decisions. Record its path under **Scope Decisions**.

### 2. Explore the codebase to verify

Explore the repository to verify the assertions carried from Step 1 and to learn the current state of the code. An
inference that survives here has a source. One that does not is an open item for Step 3.

When the code contradicts the user's premise (the code or feature they described does not exist, exists in another form,
or lives in another repository), say so before going further, and carry the corrected premise into Steps 3 to 6.

**Discover the policy skill.** Run step 1 of [align.md](align.md) here and disclose the search in one line. This step
only identifies which skill matches. Its policies are read at Step 5b, so a match alone cannot rate the Non-functional
row Clear.

### 3. Ambiguity scan

**Principles.** Apply the [principles check](../../routing/references/contracts/principles-check.md) to an artifact
relevant to the solution. Keep settled source decisions closed.

Rate the synthesized solution in each category as **Clear**, **Partial**, or **Missing**. The six categories describe
solution work only. The intent's settled problem, outcome, scope, and direction are sources, and the scan does not
challenge them again:

| Category                        | Covers                                                                                    |
| ------------------------------- | ----------------------------------------------------------------------------------------- |
| **Functional Scope & Behavior** | What the feature does, what it explicitly does not do, and how "done" is judged           |
| **Domain & Data Model**         | The concepts involved, what they're called, what they hold, how they relate               |
| **Interaction & UX Flow**       | Entry points, the path through the feature, what the user sees at each step               |
| **Non-functional**              | Performance, scale, availability, security, accessibility, compliance targets             |
| **Integrations**                | Systems, services, and teams this touches, and the contracts at each boundary             |
| **Edge Cases**                  | Empty, concurrent, failed, partial, and hostile inputs; migration and backwards behaviour |

A category is **Clear** when each element in it has a source under the trace rule, or when the feature does not touch
it, which the scan says explicitly. "I can guess what they meant" is **Partial**. A category with no sourced element is
**Missing**. A prior record (an intent's **Decisions**, a legacy design's `## Resolved questions`, an interview run this
session) is an input: a category is open only where its gaps fall outside that record, and Step 4 receives only those
new gaps.

**The empirical fork.** Apply the
[empirical questions contract](../../routing/references/contracts/empirical-questions.md) in the ambiguity scan. Record
the observed answer in the spec as a lookup. Keep unresolved empirical gaps in the scan with the evidence needed to
close them.

**The threat-model offer.** When the solution touches auth,
[data handling](../../routing/references/contracts/vocabulary.md#data-handling), or a trust boundary, offer
`threat-model` before the design is final. On yes, run `/agentics:threat-model --composed` on the solution as drafted
and read the envelope it returns, so no threat-model document is written. Agreeing to run the model accepts none of its
mitigations: present each proposed mitigation to the user, carry into the spec as a decision only the ones the user
accepts, and record the rest as declined or pending. On no, record the answer and continue. This skill never writes a
threat model of its own, and the offer adds no requirement the user did not state.

Show the rated scan. Both offers ride in the same message as recommendations the user answers. When the scan is Clear
throughout, the threat-model offer is the one question before Step 5. Render the scan and the Step 4 announcement in the
same message, then continue: the user can redirect before questioning starts.

### 4. Close the gaps with `/agentics:challenge`

If any category came back Partial or Missing, announce, in the same message as the rated scan, the exact solution
categories `/agentics:challenge` will cross-examine and that the spec will be written afterward. The announcement is
informational. Then invoke `/agentics:challenge` with exactly that scope and the settled decisions of any prior record,
so it asks only the new solution gaps. If the scan came back Clear throughout, skip the challenge and go to Step 5.

The challenge runs against this `spec.md`. An intent's challenge is direction evidence, never proof that this solution
was reviewed. A completed pass carried forward on the same solution scope stays valid: cite it as
`prior record dated YYYY-MM-DD`, with the prior record's own fields, as the carried-forward rule in
[chain-artifacts.md](../../decompose/references/chain-artifacts.md) says.

Escalate to [`/agentics:challenge discern`](../../challenge/references/discern.md) rather than press the same questions
again when ambiguity survives the cross-examination and the user is weighing genuinely competing options. When the user
still weighs structurally different solution shapes, Step 4b runs that fork; it is not a reroute.

When the cross-examination shows the direction was never settled, the direction route in Step 0 applies, and so does its
yield. Unresolved solution blockers are recorded in **Further Notes** and block the approval question.

**Declined.** The challenge is a process gate. When the user declines it, name the cost once, record each open
category's elements as "assumed, pending author confirmation", and write the `challenge spec.md` row with status
`declined`. Do not ask its questions again.

**Unattended.** With nobody to answer, the challenge's questions go into the run's report. Each open element is recorded
as "assumed, pending author confirmation", the `challenge spec.md` row reads `pending` with a `next` of
`spec resume the challenge`, and the approval table's unattended row applies.

### 4b. Explore approaches on a genuine fork

Before you sketch seams, apply the genuine-fork test in
[approach-exploration.md](../../deepen/references/approach-exploration.md). It holds when all three are true:

- Two or more structurally distinct shapes are live.
- The user has not chosen (no Decision in `intent.md` and none in the session).
- The choice is expensive to reverse once the spec is written.

When the test fails, say so in one line and continue. When it holds, load that reference, run it with two or three
whole-shape candidates, screen them against its red flags, put the recommendation on the ➡️ line, and let the user pick.
Record the chosen shape in **Solution** and each rejected shape as one **Scope Decisions** entry that begins "Rejected:
…", with the reason. This step is the only place `spec draft` weighs solution shapes, and it never runs on a fork the
user closed.

### 5. Sketch the test seams

Before writing, sketch where this feature will be tested from:

- **Prefer an existing seam** over a new seam.
- **Prefer the highest seam** that can still observe the behaviour. Test the feature the way a user meets it, not the
  internals underneath.
- Use as few seams as possible. One seam is ideal. Justify each additional seam.
- **No existing seam?** Name the one the repository would use rather than leave it to inference. For a script or CLI
  with no harness, that is usually a fixture-style script that follows the convention of the repository's existing test
  runners. A seam described in the abstract is not a seam anyone can write.
- **No test suite at all?** Name the closest executable check, which is seen to fail before the change and pass after
  it.

The sketch becomes the seams half of **Verification**. Also name the one end-to-end verification step: the command or
action a person runs and the observable result that proves the whole solution works. The plan's **Outcome proof**
implements it.

### 5b. Align to policy

Run steps 2 to 4 of [align.md](align.md) against the synthesized solution: follow the policy skill that Step 2 found,
classify each applicable requirement, prepare the align writes with their citations, and compose the **Policy
Conflicts** section. With no policy skill found, the section is the single line `n/a: no policy skills declared`.
Nothing here resolves a conflict or reopens an intent decision.

### 6. Write the spec

Two authoring rules apply throughout:

- **Speak the project's language.** Use the terms in the project's domain glossary and respect decisions recorded in its
  ADRs. [repo-conventions.md](../../routing/references/repo-conventions.md) resolves both. A missing term, a clashing
  term, or a scope decision that itself qualifies as an ADR goes to [`/agentics:deepen`](../../deepen/SKILL.md), not to
  an inline resolution.
- **Prose first.** Contract notation may sit in `## Solution` where prose would be less precise than the thing it
  describes: a signature, a schema, a state table, or an example payload. It is the contract kind of the rule in
  [Code in artifacts](../../decompose/references/chain-artifacts.md#code-in-artifacts). The spec never holds a function
  body or test code, whatever tag the block carries. This is a held rule: when the user asks for one anyway, name the
  cost once (the spec turns into code nobody reviews as code) and offer what the spec can carry instead, a signature, a
  state table, or an example payload, then write that.

Before writing, check that every required template section is populated, that contradictions from Steps 2 to 4 are
resolved, and that every deferred scope item is an explicit **Out of Scope** entry. Evaluate the [tier](#tier) now,
because the card's `Ask` line names it.

The work folder is the one that holds this item's `intent.md`, or a legacy `design.md` (which stays where it is). When
none exists, create `<plan-parent>/YYYY-MM/<purpose>-<topic>/`:

- `YYYY-MM` is the current month.
- `<plan-parent>` comes from the **Resolve the plan parent** step in `skills/decompose/SKILL.md`.
- `<purpose>` is `feature`, unless the feature's name begins with another purpose word from
  [chain-artifacts.md](../../decompose/references/chain-artifacts.md).
- `<topic>` is the slug that the **Plan artifact identity** rule in
  `skills/execute/references/plan-artifact-identity.md` makes, from the current non-default branch name when there is
  one, else from the feature's name, capped at 40 characters.

Write `<work-folder>/spec.md`, opening with the provenance header. This is one instance:

```
derived-from: [intent.md]
```

Set `derived-from` to `[intent.md]` when the work folder holds one, and to `none` otherwise, including a legacy design
read in place. The header holds no other field.

Write the `**Challenge record:**` entry under **Scope Decisions** when the Step 4 challenge ran, was declined, or was
left pending, with the contract's five fields. A Clear scan writes no entry. Never copy an intent's challenge as proof
of a new solution review.

Then show the user the card, the **At a glance** list, the Verification step with its named seams, and the path. The
full text is shown on request. Ask no permission question for the reversible draft.

#### Rows and staging

In a committed work folder, stage `spec.md` with `git add -- <work-folder>/spec.md` and confirm that
`git diff --cached --name-only` names it before any row that describes it. After each row, stage `state.tsv` with its
own `git add -- <work-folder>/state.tsv`. If either `git add` fails, report it and do not go on. Under `local` storage,
skip staging; the rows are written the same way. After the last edit before a row that depends on the spec's content,
stage `spec.md` again and confirm that `git diff --name-only -- <work-folder>/spec.md` prints nothing.

The calls, each with `--folder <work-folder>`, are these:

```text
agx flow log spec create spec.md ok "<core choice>" spec.md "spec review the draft"
agx flow log spec challenge spec.md <ok|declined|pending> "<what the challenge settled>" spec.md "<next>"
agx flow log spec challenge spec.md ok "scan clear: nothing to challenge" scan=clear "spec review the draft"
agx flow log spec waive direction-review ok "<the user's reason, or none given>" none "spec review the draft"
agx flow log spec review spec.md <ok|failed> "<N> findings, <B> blocking; reviewed text=<hash>" "kind=spec dispatched=<ts> round=<k> blocked=<seconds> rev=<n> by=<independent|self>" "<next>"
agx flow log spec approve spec.md ok "<tier>: <signals>; <basis>" auto "decompose write the plan" --expect-blob <hash>
agx flow log spec approve spec.md declined "<the user's reason, or none given>" none "spec revise the spec"
agx flow log spec pause spec.md pending "<what is pending>" none "<next>"
```

- `create`'s `why` names the solution's core choice, for example `one committed trail; why becomes free text`.
- A `challenge` row's `next` is `spec review the draft`, or `spec resume the challenge` for `pending`.
- A `review` row's `<ts>` is the dispatch time, read from a clock at dispatch (`date -u +%Y-%m-%dT%H:%M:%SZ`, or
  `(Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ')` in PowerShell). `blocked` is the whole latency in
  seconds, because no implementer runs while a spec is reviewed. `<hash>` is the spec's `text=` hash from the `hashes:`
  line of `flow status`. The status is `ok` when the run reports no Critical or Important finding, and `failed`
  otherwise.
- `approve` takes `--expect-blob` with the hash shown to the user, as [the approval guard](#approval) requires.

#### Independent review

Every spec gets an independent review before any approval question, whatever its tier. After the spec is written and
staged, dispatch one fresh general-purpose subagent with [spec-reviewer-charter.md](spec-reviewer-charter.md), the spec
path, and the intent path when one exists. This session authors the spec at its own tier, which is Standard. The
reviewer runs at the Deep tier, and at Max only when the user selects Max for the review. The **Reviewer selection**
section of the [model ladder](../../routing/references/model-ladders.md) resolves the tier for the host. Write one
`review spec.md` row per return.

Each finding carries a severity from [severity.md](../../routing/references/contracts/severity.md). A Critical or
Important finding blocks: fix it in the spec, or ask the user the decision it names. A Minor finding is fixed or
presented with the approval question. Record each presented finding under **Further Notes**, one line each, as
`- Spec review finding (presented): <finding> (<location>)`. A finding that would change a settled intent decision goes
back to `/agentics:intent`, never into the spec. Record the review under **Scope Decisions**, after **Written by**, as
`- **Spec review:** <YYYY-MM-DD>, <N> findings, <R> resolved, <O> presented, run <k>`, with the counts of the latest
run.

When the host cannot dispatch a subagent, say so once, read the spec against the charter yourself in one pass, record
`- **Spec review:** <YYYY-MM-DD>, self-read: no subagent available, <N> findings, run <k>` and a `review` row with
`by=self`, and say in the approval question that the review was not independent.

**The review holds.** The review is a safety gate: it is the one independent read of the spec before the user approves
it. When the user asks to skip it, say once that it is held, that an approval without it rests on a spec nobody else has
read, and that the review takes a few minutes, after which the approval question comes. Keep the draft. Ask no approval
question and write no `approve` row until a review is recorded. A repeated demand gets one line pointing back to that.

**Review budget.** Runs are counted per revision of the spec, from the `review spec.md` rows and their `rev=`, never
from the session or the `**Spec review:**` line.

1. **A revision.** Revision 1 starts with the first write. Run 1 reviews it. Run 2 runs only when the fixes for run 1's
   findings added or altered behavior, an interface, or a success criterion. A wording fix causes no run. A revision has
   no run 3.
2. **Reset.** A new revision, with a fresh budget, starts when reviewed scope changes for any other reason: a change the
   user asks for, including the change in an edit request at the approval question, or a `spec align` write that adds or
   alters behavior, an interface, or a success criterion. Scope no review covered is never approved as reviewed.
3. **A spent budget.** When the last run of a revision still reports a blocking finding, present the findings and ask
   the user to revise the spec or stop. Ask no approval question. A revision the user orders starts the next budget.
4. **On request.** The user may ask for one more run at any time, and it runs whatever the budget, with the next
   `round=`. No other run repeats on content whose hash matches the latest `review` row.
5. **Self-read.** A self-read spends no independent run.
6. **Resume.** A resumed session reads the rows. It reuses a run whose reviewed hash matches the current spec, and
   dispatches a reviewer when the revision has a run left.

#### Tier

The tier is evaluated here, once, from the spec and the intent's `## Affected users and systems` section. `decompose`
takes it as given and adds only plan-level signals. The tier is elevated when any of these holds, and routine otherwise:

- the spec names a change to a public interface or contract;
- it touches auth, a trust boundary, or [data handling](../../routing/references/contracts/vocabulary.md#data-handling);
- it involves a migration or a change to a persisted format.

The user can raise the tier at any point and can never lower it. When a review fix fires a new signal, update the card's
`Ask` line before the approval question. State the tier before the approval question or the hand-off, in one line with
its plain meaning from [vocabulary.md](../../routing/references/contracts/vocabulary.md), for example
`Tier: elevated (public contract): the spec and the plan are approved separately.`

#### Approval

Whether this skill asks is decided by the approval table in [SKILL.md](../SKILL.md#approval). When it asks:

1. Show the review's findings: each presented finding with its location, or `spec review: no findings`, and say so when
   the review was a self-read. Name any element labeled "assumed, pending author confirmation".
2. Run `agx flow status --folder <work-folder>` and note the hash on the spec's `hashes:` line. Show the path and that
   hash beside the summary. That is the content the user approves.
3. Ask once: "Approve this spec? Your yes records the approval, which permits the handoff to `decompose`."

The answer decides what follows, under [approval.md](../../routing/references/contracts/approval.md):

| Answer                                | What follows                                                                                                                                                                                                                                                                                              |
| ------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| A yes, in any wording                 | Confirm `spec.md` is staged and unchanged, write the `approve spec.md` row with `--expect-blob <hash>`, stage `state.tsv`, and commit `spec.md` and `state.tsv` together under [the commit contract](../../git/references/commit.md#the-commit-contract). Then hand off, or stop for a spec-only request. |
| An edit request ("yes, but change X") | Make the change. When it changes reviewed scope, it starts a new review revision. Show the result and its new hash, and ask again.                                                                                                                                                                        |
| A no                                  | Write the `approve spec.md` row with status `declined`. Say the spec is written and unapproved. Do not ask again about the same text; a revision reopens the question.                                                                                                                                    |
| "Not yet"                             | Write a `pause spec.md` row whose `next` reads `spec ask the approval again`. Do not ask again about the same text.                                                                                                                                                                                       |
| No answer                             | Write no row.                                                                                                                                                                                                                                                                                             |

When [the approval guard](../../execute/references/flow-approval-and-audit.md#the-approval-guard) refuses the row, the
spec changed after it was shown. Run `flow status` again, show the spec's new hash and what changed, and ask again.
Never write the row with other evidence to get past the guard. An approval the user gave before the spec was written and
reviewed is not an approval of its content: ask the question after the review.

**Routine hand-off.** On the routine row, show the review findings first, then say that the tier is routine, that
`decompose` drafts the plan against the unapproved spec, and that one question will approve both. Write a
`pause spec.md` row whose `why` reads `routine: <signals or none>; reviewed, awaiting the combined approval` and whose
`next` reads `decompose write the plan`, unless `flow status` already shows a `next:` that starts with `decompose`. Hand
off to `decompose` within the authorized planning scope. When the request is for capture only, for another session, or
paused, report the route and do not start it. The spec stays staged and uncommitted until the combined approval, whose
commit carries it.

**After approval.** Never edit an approved `spec.md` to record a divergence. A body edit makes the spec approval
`stale`, and the phase that needs it asks again. `execute` appends each divergence to the `## Divergences` section at
the end of the file, which the approval hash leaves out, and `gate` reconciles the entries. The format is in
[chain-artifacts.md](../../decompose/references/chain-artifacts.md#the-divergences-section).

### When the user wants it faster

Time or authority pressure ("just write it now", "skip the scan") meets process gates and safety gates differently, as
[gates.md](../../routing/references/contracts/gates.md) defines them.

Every step of this workflow is a process gate except the three below. When the user declines one, name its cost once and
do as asked. A declined challenge is a `challenge spec.md` row with status `declined`. Any other skipped step is one
`waive spec-step <step>` row naming it, where `<step>` is one hyphenated word and the direction review keeps its own
`direction-review` head. A skipped required section is still written, as `none stated: left out at the user's request`,
because the reviewer and `decompose` read it; the review may report it.

Three things hold under any pressure, because each is a safety gate:

- **The spec asserts nothing without a source.** Content that traces to no statement or code is labeled "assumed,
  pending author confirmation". Never fabricate User Stories, Scope Decisions, or a Success Criteria number.
- **The independent review runs before the approval question**, as [The review holds](#independent-review) says.
- **Only the user's answer approves the spec.** Write no `approve spec.md` row without it.

> **Note:** This spec defines the solution: behavior, interfaces, design, and acceptance. The source intent remains the
> authority for the problem, outcome, constraints, and direction. Delivery slicing and implementation tasks belong to
> `/agentics:decompose` after the spec is approved.

<spec-template>

The template has seven required sections, in this order, then two optional sections that appear only with content.

The file opens with the provenance header, then a `# Spec: <topic>` title line. Directly under the title, before
`## Problem Statement`, an unheaded bullet list holds the card: `- **Change:**`, `- **Why now:**`,
`- **Out of scope:**`, `- **Ask:**`, `- **Risk:**`. Each is one sentence, and a line the source does not support reads
`none stated`. `Ask` is static text: it names the approval, the tier, and the next owner, and holds no live status. The
card adds no section. A spec written by an earlier `spec` version stays valid, and `/agentics:spec align` and any
rewrite add no card to it. A rewrite keeps the spec's original `**Written by:**` version line, so the reviewer's version
test still exempts a rewritten legacy spec. The rules are in [skim-rules.md](../../decompose/references/skim-rules.md).

## Problem Statement

The problem in brief and the constraints that bind the solution, restated so the spec reads on its own. Link `intent.md`
for the full record when one exists. Do not introduce a new direction or requirements here.

## Solution

Open the section with the bold line `**At a glance**`, then a list of three to six bullets: `Behavior`, `Interfaces`,
and `Files and modules`, and optionally `Unchanged`, `Rejected shape`, or `Risk`. The prose follows the list.

The selected solution, at design level: its externally observable behavior, the interfaces it exposes or changes, and
the files and modules involved. Task-level file lists belong to the plan.

## Success Criteria

Observable acceptance results that prove the solution works. Each states a result and its target or test. Each reads
like "a returning user reaches their dashboard in under 2 seconds at p95", not like "the query is indexed". Where no
target has been stated by the user or found in the code, name the criterion and label the target "assumed, pending
author confirmation" rather than invent a number.

A success the user stated qualitatively is already sourced. Record it as stated, reframed as an observable or binary
check where the words support one, and leave it unlabeled. For example, "nobody triggers a real install by accident"
becomes "no run reaches a real install without an explicit confirmation step". The assumed label marks an unsourced
target, not an unquantified one.

## Verification

One end-to-end verification step: the command or action a person runs and the observable result that proves the solution
works as a whole. The plan's **Outcome proof** implements it. Then the test seams agreed in Step 5:

- each seam and what it covers;
- what makes a good test here (external behavior, not implementation details);
- prior art for the tests in the codebase.

## Scope Decisions

Open the section with ``- **Written by:** `spec` v<this skill's version>``. A rewrite of an existing spec keeps its
existing line. Then add the `- **Spec review:**` line from Step 6. Then list the solution decisions made while behavior,
interfaces, and design were pinned down:

- Solution boundaries and what is explicitly in or out of scope
- Behavioral decisions (e.g., how the system should respond to edge cases)
- Interface and contract decisions
- Design choices and rejected alternatives

Keep the decisions at the level needed to implement and verify the solution. Delivery sequencing, task dependencies, and
slice boundaries belong to `/agentics:decompose`. The `**Challenge record:**` entry goes here too, its five fields as a
numbered list nested under the label.

## Policy Conflicts

The output of Step 5b, in the shape [align.md](align.md) defines. Present in every spec. With no policy skill found it
reads exactly `n/a: no policy skills declared`. Otherwise it lists the policy skills loaded, the candidates considered
and not used, the unverified targets, the count of align writes, and one numbered entry per conflict.

Each entry names the policies or the intent decision in tension, the spec element affected, and what a resolution would
require. Nothing here is resolved by `spec`.

## Out of Scope

A description of the things that are out of scope for this spec.

## User Stories

Optional: include it only when distinct actors or flows are worth testing one by one. Omit the heading otherwise.
Readers treat a missing section as "none" and read Success Criteria instead. When present, each numbered item **must**
use the canonical sentence form. It then carries its priority and an independent test:

```
<N>. As a <actor>, I want a <feature>, so that <benefit>
    - **Priority:** P1 | P2 | P3
    - **Independent test:** <one line — how this story is verified on its own>
```

<user-story-example>
1. As a mobile bank customer, I want to see balance on my accounts, so that I can make better informed decisions about my spending
    - **Priority:** P1
    - **Independent test:** signed in with two accounts, both balances render on the account list without opening either account
</user-story-example>

Priority is release-relative:

- **P1**: the feature doesn't ship without it.
- **P2**: it ships degraded without it.
- **P3**: it's a follow-up nobody blocks on.

The independent test states what proves that one story. It does not require the other stories to be built. Cover the
feature's real footprint. Don't manufacture stories to pad the count.

## Further Notes

Optional: include it only when there is something to note. Omit the heading otherwise.

</spec-template>
