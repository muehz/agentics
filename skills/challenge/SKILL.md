---
name: challenge
description: "Use when a plan or decision needs pressure-testing before someone commits to it. Expose weak assumptions, compare credible alternatives, and ask the questions that could change the answer. Include a recommendation for each."
version: "4.0.0"
user-invocable: true
disable-model-invocation: false
argument-hint: "[plan, design, approach, or scoped work to stress-test] | discern [ambiguous or high-risk problem]"
metadata:
    surfaces: [claude-code, copilot-cli, vscode, codex, antigravity]
    modes: [discern]
---

# Challenge: pressure-test a plan before commitment

This skill questions a plan, a design, an approach, or a scoped piece of work before anyone commits to it. The interview
mode finds weak assumptions, open decisions, and unstated dependencies, and asks the user about each one with a
recommended answer. The discern mode compares competing options and recommends one. Challenge writes no file, approves
nothing, and changes nothing. The decisions stay with the user.

`intent`, `spec`, and `decompose` invoke challenge on their own artifacts, and `routing` invokes it on an ambiguous
request. A user can also invoke it on any plan or scope with `/agentics:challenge` or with words such as "poke holes in this".

Skill commands here use the `/agentics:<skill>` form that Claude Code, Copilot CLI, and VS Code Chat accept. On Codex,
type `$agentics:<skill>` instead.

## Don't use when

Read the rows from the top; the first match wins. The **Kind** column says what an explicit invocation does with the
row, as [forwarding.md](../routing/references/forwarding.md#explicit-invocation) defines it: a `cannot` row forwards,
and a `better elsewhere` row is named in one line and the run continues here. When the model loaded this skill on its
own, every row forwards. A request to sign off work with no artifact in the conversation is not a bare goal either; it
skips the bare-goal row and goes to [Step 0](#step-0-confirm-there-is-something-to-question). The `safety stop` row is not a route-away and gets no yes/no offer: announce `/agentics:ops response` in
one line and invoke it with the input, because a confirmation turn costs response time during a live event. The bare-goal row is never matched against a scope that a calling skill passed; see
[Called by another skill](#called-by-another-skill).

| Input signal                                                                                                                   | Use this instead         | Kind             | Why                                                                                                      |
| ------------------------------------------------------------------------------------------------------------------------------ | ------------------------ | ---------------- | -------------------------------------------------------------------------------------------------------- |
| A failure with current user impact that needs a response: an outage, an active security incident, "SEV-1"                      | `/agentics:ops response` | safety stop      | The deliverable is containment and response. Challenge produces questions.                               |
| A failure with no current user impact whose cause needs finding: a bug, an error, a failing test, a regression                 | `diagnose`               | cannot           | The deliverable is a root cause. Challenge questions decisions that are not made yet.                    |
| A finished pull request, branch, or diff needs a code review                                                                   | `/agentics:review`       | cannot           | Review reads finished work. Challenge questions a plan before the work.                                  |
| A request to make the change: apply fixes, edit the plan, or implement                                                         | `routing`                | cannot           | Challenge changes nothing. `routing` selects the phase that makes the change.                            |
| A bare goal, wish, or symptom with none of the three scope markers in [Step 0](#step-0-confirm-there-is-something-to-question) | `intent`                 | better elsewhere | Intent forms a direction first. Under `/agentics:challenge`, ask the one question for the missing scope. |
| A conversational or opinion question, with no plan, decision, or options of the user's in play                                 | No skill                 | cannot           | Neither mode has anything to question. Answer the question directly.                                     |
| Otherwise                                                                                                                      | this skill               | —                | Continue to [Which mode?](#which-mode)                                                                   |

A plan or hypothesis about a failure is still a plan. When the user asks to pressure-test their own hypothesis
("challenge my assumption that it's the load balancer"), the first two rows still fire, because the failure needs its
response or its cause. For a failure with no current user impact, when the user declines or stops the forward, stay here
and question the hypothesis, as
[forwarding.md](../routing/references/forwarding.md#b-offer-to-forward-one-yesno-question) says for a decline. During
an active incident, the hypothesis is not interviewed here: name it as deferred, and question it once the incident is
contained or in its postmortem, as routing's [incident precedence](../routing/SKILL.md#step-1--check-for-agentics-native-workflows-incl-active-incident-precedence)
says.

## Called by another skill

When `intent`, `spec`, `decompose`, or `routing` invokes challenge, the scope the caller passes is the scope. Challenge
questions that scope and returns its summary to the caller. The bare-goal row does not fire for it, and challenge never
forwards the caller's input back to the caller. Decisions the caller marks as settled are not asked again.

The caller records the result: `intent`, `spec`, and `decompose` each write the challenge record and the `challenge`
row for their own artifact, as [chain-artifacts.md](../decompose/references/chain-artifacts.md) defines them. `routing`
has no artifact and records neither. Challenge writes no row itself.

The caller's own rules decide what an unanswered question means for its artifact. A question that took its recommended
answer when the user ended the round (see [Ending the interview](#ending-the-interview)) is returned with that source,
never as the user's answer, and the caller may keep it open: `intent` keeps it a `blocker`, and `decompose` keeps the
plan challenge `pending`.

## Which mode?

Read the rows from the top; the first match wins. Say the mode in one line when the invocation did not name it.

| The input                                                                                          | Mode      |
| -------------------------------------------------------------------------------------------------- | --------- |
| `/agentics:challenge discern`, or a request in words to use discern                                | Discern   |
| The user names the interview for this input ("run your normal interview")                          | Interview |
| Two or more whole alternatives, hypotheses, or owners, and the deliverable is one pick among them  | Discern   |
| Otherwise: one plan, design, approach, or scope, including one with a single open choice inside it | Interview |

Discern follows [references/discern.md](references/discern.md). Load [references/discern.md](references/discern.md) for
discern, [forwarding.md](../routing/references/forwarding.md) before any route-away,
[question-format.md](../routing/references/contracts/question-format.md) before asking any frontier round, and
[gates.md](../routing/references/contracts/gates.md) before any safety hold. In discern mode, a "don't ask for context"
or "skip the caveats" request skips the reasoning preflight (opening with `[Preflight skipped]`), drops explanatory
caveats, and keeps verification labels on every unverified claim. A single open choice inside a plan becomes one
interview question with a recommended answer. When an interview ends and two or more whole alternatives remain that
questioning cannot settle, say so in one line and continue in discern on those alternatives.

## Workflow

### Step 0. Confirm there is something to question

**A signoff with no artifact comes first.** Check it before the scope markers and before the bare-goal row, which it
overrides. A request to sign off finished work with no proposal, design, diff, or pull request in the conversation gets
no verdict and no route. A verdict on work nobody has shown is a claim of status with no evidence, a
[safety gate](../routing/references/contracts/gates.md#safety-gate). Say once that no verdict is given on unseen work,
because it would read as a check that never happened, and ask **only** for the proposal or design in one explicit
`❓`/`➡️` question (state in one line that a finished diff or pull request goes to `/agentics:review` instead—never
invite a PR link as input to this skill). A described proposal then runs here.

A plan, a design, or an approach in hand passes this step. So does a scope from a caller. Otherwise, read the input for
the three scope markers:

1. A named target.
2. At least one stated inclusion or exclusion.
3. At least one decision the work cannot proceed without.

"Audit the docs outside `skills/` and `scripts/` against the implementation, report first" has all three. The third
marker counts when you infer it, as long as the stated work cannot start without it: "Rate-limit the public API at 100
requests a minute, leave internal callers alone" leaves open what happens on a breach (reject, queue, or shed). Ask an
inferred decision in the first round so the user can correct it. A question that is only interesting is not a marker.
Read the rows from the top; the first match wins.

| Markers present                   | Action                                                                                                                                                                                                                                                                                                                                                 |
| --------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| All three                         | Go to Step 1.                                                                                                                                                                                                                                                                                                                                          |
| A named target, but not all three | If the user explicitly named an interview over a named set of alternatives, start the interview and put any missing scope marker into the first round. Otherwise ask one question that names every missing marker. The reply reopens the work here: map the tree within the scope given and ask the first round. Do not ask for the same marker again. |
| Otherwise: no named target        | The bare-goal row above applies. Under `/agentics:challenge`, name `intent` in one line, then ask the same one question for a target, a boundary, and the open decision.                                                                                                                                                                               |

### Step 1. Gather the plan

Take the plan or the scope in any form: a GitHub issue, a document, a file in the workspace, or the invocation text
itself. The facts are yours to find. Look up anything the repository, git, or the tools can answer, and never ask the
user for it. The decisions are the user's.

### Step 2. Map the decision tree

Map the decisions in the interview's scope. Do not open or read any principles files in challenge—leave the
[principles check](../routing/references/contracts/principles-check.md) to the phase inspecting the work.

Identify every decision point, assumption, and dependency in the plan, and sort them into five branches:

- **Scope.** What is in and out.
- **Technical.** Architecture, technology choices, and patterns.
- **Behavioral.** Defaults, edge cases, and error handling.
- **Dependency.** What must exist first, and the integration points.
- **Risk.** What could go wrong, and the mitigations.

With a scope from a caller or the user, map only inside it, and skip branches the scope already settles. Do not reopen a
settled decision unless new evidence contradicts it.

### Step 3. Ask in frontier rounds

The **frontier** is every decision whose prerequisites are settled, so you can ask it now without guessing at an answer
you have not heard.

1. Compute the frontier. Drop from the current round any question whose answer **or** whose recommended answer depends
   on another open question in this round, and defer it to a later round.
2. Ask the whole frontier as one round, in the exact format (`❓ **Q<n>** - **<title>**:` followed by a `➡️`
   recommendation line grounded only in facts present in the input) and with the answer collection that
   [question-format.md](../routing/references/contracts/question-format.md) defines. Number the questions across the
   whole interview (Q1, Q2, and on), and order each round by risk. Ask only decisions whose answer changes the plan. A
   point with an obvious answer is not asked: it goes into the summary's **Assumptions** as a stated default the user can
   override.
3. When a question needs a fact from the environment, dispatch a subagent to find it and ask the rest of the round now.
   Only a question whose answer the fact can change waits. Name a held question in the round: "Q4 on the dedup key is
   held while a check of the webhook scaffold runs." When you cannot name what the fact would change, the question is not
   held. When the host has no subagent dispatch, look the fact up inline before you post the round. When the fact
   arrives, ask what it unblocked as a follow-up round, and cite the fact wherever a round or the summary uses it.
4. Read the replies under [Collecting answers](../routing/references/contracts/question-format.md#collecting-answers).
   For a vague answer, or "I don't know", ask once for the specific case or for who would know. Then record what you have.
5. Recompute the frontier and ask the next round. The interview is complete when the frontier is empty.

In an [unattended](../routing/references/contracts/unattended.md#when-a-run-is-unattended) run, the round goes to the
dispatcher, and the dispatcher's answers are recorded with the dispatcher as their source. When nobody can answer,
return the mapped tree as open questions and write no summary.

### Step 4. Challenge weak spots

Question these patterns directly:

- **Handwaving.** "We'll figure it out later": ask what is still to be figured out.
- **Optimistic assumptions.** "This should be simple": ask what makes it simple.
- **Missing error paths.** Only the happy path is described: ask what happens when each step fails.
- **Scope creep.** "We could also...": ask whether it is in scope for this iteration.
- **Unstated dependencies.** Reliance on another system or team: make it explicit.
- **Confident but unverified claims.** "That's already atomic": ask for the guarantee, document, or test that shows it.

### Step 5. Summarize the shared understanding

Write the summary when the interview is complete, or when the user ends it after answering at least one question, as
[Ending the interview](#ending-the-interview) says. A summary with no answered question behind it would tell the caller
that decisions were examined when none were, so it is a claim of status with no evidence, a
[safety gate](../routing/references/contracts/gates.md#safety-gate). When it is asked for before any answer, say so
once, and offer the two ways on: answer the first round now, or end with nothing examined. A caller records an interview
ended before any answer as `declined`.

The summary opens with one **Scope examined** line, which the caller copies into its challenge record. It names the
target artifact's filename, or `standalone: no artifact`, the scope questioned, and the rounds asked and answered with
their question numbers, for example: `intent.md`, direction, 2 rounds asked and answered, Q1 to Q5. When the interview
ended early, the line also lists the frontier left unasked.

Then the sections below, in this order. Leave out a section that has nothing in it.

- **Resolved decisions.** Each decision with its source: the user's answer, an accepted recommendation, a recommendation
  taken when the user ended the round, or a lookup.
- **Open questions.** Decisions that still need someone's input, with who that is when known.
- **Risks.** Each with the guarantee, test, or decision still needed to close it. Name the gap, not the fix.
- **Assumptions.** The stated defaults from Step 3, and anything else the plan takes as given.
- **Concerns weighed and set aside.** Each weak spot from Step 4 that an answer resolved or that you judged not worth
  acting on, with a one-line reason tagged `resolved-by-answer` (cite the question) or `judged-not-worth-acting` (one
  line of likelihood times cost). Say when several independent sources raised the same concern, such as a lookup, the
  user's answer, and your reading of the plan; agreement between independent sources is a strong signal.
- **Suggested next steps.** What the user or the caller can do with this.

## Ending the interview

The user can end the interview at any point, in any words.

| When the user ends it                 | What happens                                                                                                                                                                                                                                                                                                                                                                                                                                                               |
| ------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Before answering any question         | Say in one sentence what was not examined, and stop. Write no summary.                                                                                                                                                                                                                                                                                                                                                                                                     |
| After answering at least one question | Immediately emit the full [Step 5](#step-5-summarize-the-shared-understanding) summary opening with the **Scope examined** line. Each unanswered question of the current round takes its recommended answer, explicitly labeled by source (distinguishing user answers from taken recommendations and unresolved open questions). A question whose recommended answer would publish, post, or halt the work stays open. Unasked questions are listed as the open frontier. |

Ending the interview never approves the plan.

## Verdicts and approval

Challenge approves nothing. An approval is the user's, as [approval.md](../routing/references/contracts/approval.md)
defines it, and the phase that owns the artifact asks for it. Per-question recommendations do not add up to an approval.

When the user asks for an overall verdict ("is this safe?", "just say approved"), give your overall assessment of what
was examined: the decisions that hold, the risks still open, and what went unexamined. Call it an assessment, not an
approval. Do not call the plan safe or approved beyond what the session examined. That word, with nothing behind it,
is a claim of status with no evidence, a [safety gate](../routing/references/contracts/gates.md#safety-gate). Say once
what is held, the harm (someone acts on a check that did not happen), and the alternatives: the user makes the call on
their own plan, or answers the open questions first. A repeated demand gets a one-line pointer back to those
alternatives.

## References

- [references/discern.md](references/discern.md): discern mode, its preflight, depth, lenses, output shape, and
  verification labels.
- [forwarding.md](../routing/references/forwarding.md): the route-away protocol and explicit invocation.
- [question-format.md](../routing/references/contracts/question-format.md): the question format and how answers are
  collected.
- [approval.md](../routing/references/contracts/approval.md), [gates.md](../routing/references/contracts/gates.md),
  [unattended.md](../routing/references/contracts/unattended.md): the shared contracts this skill applies.
- [chain-artifacts.md](../decompose/references/chain-artifacts.md): the challenge record a caller writes from the
  summary.
