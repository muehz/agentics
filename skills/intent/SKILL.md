---
name: intent
description: "Use when someone asks to add, change, or build something that could alter behavior, an interface, or a contract, even when the request already seems clear. Record the problem, desired outcome, and constraints, and challenge assumptions before any code changes. A one-task change gets a short brief instead."
version: "4.3.0"
user-invocable: true
disable-model-invocation: false
argument-hint: "[topic or problem]"
metadata:
    surfaces: [claude-code, copilot-cli, vscode, codex, antigravity]
    artifact: intent.md
---

# Capture: problems into intents

This skill captures and challenges the problem, desired outcome, affected users and systems, scope, and constraints in `intent.md`, resolves the direction, and asks the user to approve it. The approved intent goes to `spec draft`. A one-task change on the middle path gets a short `brief.md` instead, and its approved brief goes to `execute`. Intent does not choose the solution, approve implementation, author delivery slices, or build a task graph.

Skill commands here use the `/agentics:<skill>` form that Claude Code, Copilot CLI, and VS Code Chat accept. On Codex,
type `$agentics:<skill>` instead.

## Don't use when

Read from the top; the first match wins. The **Kind** column governs explicit invocations ([forwarding.md](../routing/references/forwarding.md#explicit-invocation)): a `cannot` row forwards, and a `better elsewhere` row is named in one line and the run continues here. When the model loaded this skill on its own, every row forwards.

| Input signal                                                                                                                                                                  | Use this instead       | Kind             | Why                                                                                                            |
| ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------- | ---------------- | -------------------------------------------------------------------------------------------------------------- |
| Something is already broken (bug, error, exception, test failure, regression) needing its cause, and not a [user's change](../execute/references/fix-entry.md#a-users-change) | `diagnose`             | cannot           | Intent forms a new direction. It does not investigate a live failure.                                          |
| An approved spec or a legacy design needs only decomposition into ordered tasks                                                                                               | `decompose`            | cannot           | Intent ends at the approved intent. It writes no plan.                                                         |
| An approved intent exists, and the solution's behavior or design needs definition                                                                                             | `spec`                 | cannot           | `spec draft` specifies the solution. Intent captures the problem.                                              |
| A plan, approach, or scope that already exists needs holes poked in it before commitment                                                                                      | `challenge`            | better elsewhere | Challenge stress-tests an existing direction. Under `/agentics:intent`, capture the direction the plan serves. |
| The only open work is UX deliverables: flows, wireframes, information architecture, or mockups                                                                                | No skill               | better elsewhere | Agentics has no workflow for UX deliverables. Under `/agentics:intent`, capture the problem they serve.        |
| Musing about or asking a question about an idea, with no ask to work it toward implementation                                                                                 | Answer directly        | better elsewhere | A question deserves an answer, not an interview. Under `/agentics:intent`, capture it.                         |
| A change that cannot alter behavior, in the exempt class that [the chain rule](#when-the-chain-is-required) names                                                             | Make the edit directly | better elsewhere | The chain records why a change was made. A change with no behavioral effect has no "why" to record.            |

A concrete new request matches none of the `decompose`, `spec`, or `challenge` rows, even when it already states its outcome, scope, or design — capture it here. Frontend implementation is an ordinary mutating change (not UX deliverables), and layout experiments go to `prototype`. When musing grammar leaves the outcome unclear, `routing`'s **Commission, not grammar** rule applies: one clarifying line, never a phase.

## Workflow

Track Steps 0 to 4 under [workflow steps](../routing/references/contracts/workflow-steps.md) on the intent path. The brief path uses [brief.md](references/brief.md) (a folder holding `brief.md` and no `intent.md` resumes via [brief.md](references/brief.md#ask), not Step 4).

### When the chain is required

Mutating work enters the chain when the change could alter what the software does, an interface it exposes, or a contract someone depends on (anything where a reviewer would ask "why this way?"). The **exempt class** needs no chain: changes that cannot alter behavior (formatting, typos, comments, dead links, or semantic-neutral renames). Outside the exempt class, size decides only the path, via [small-fix classification](../routing/references/chain-phases.md#small-fix-classification):

| Classification outcome | Path                                                                                            |
| ---------------------- | ----------------------------------------------------------------------------------------------- |
| small fix              | `execute fix`, after the [announcement](../routing/references/chain-phases.md#the-announcement) |
| `brief`                | the middle path: this skill writes `brief.md`, as [brief.md](references/brief.md) says          |
| full chain             | this skill writes `intent.md`, Steps 0 to 4 below                                               |

A one-line change to a documented default is mutating; a thousand-line reformat is not. The user can move work onto a heavier path at any point. An approved plan or brief already in the work folder satisfies the rule; work that turns mutating mid-session enters here.

**Waiving the chain entry.** Entering the chain is a [process gate](../routing/references/contracts/gates.md#process-gate). When the user declines it, name the cost once (no recorded problem, no challenged assumptions, nothing for a reviewer to reconstruct the decision from). On the next reply that still declines, in any wording, comply:

1. Record one `waive` row (ask for no reason; `why` is the user's reason or `none given`; substitute the resolved `<work-root>`, never emit a literal placeholder):

        agx flow log intent waive chain ok "<the user's reason, or none given>" none "done change made with the chain entry waived" --folder <work-root>/unassigned

2. Make the change in this session. Write no `intent.md` and no other artifact.
3. Name the waiver in the session's report, derived from that row.

Commits, pushes, and pull requests for the waived change keep their own rules.

### Step 0. Preflight

Check the input against **Don't use when**. Forward matching rows via [forwarding.md](../routing/references/forwarding.md#target-classification) (carry input, forward once, ask no starter question for the target). Under an explicit invocation, a `better elsewhere` row gets one line naming the other route and continues at Step 1. On the exempt-class row when model-loaded, make the edit directly, name the exempt class in one short line, and write no intent.

### Step 1. Gather the direction

Read current project state, any open issue, PR, or repository file the user names, and any existing intent before asking a question. Extract problem, beneficiary, outcome, scope, constraints, and decisions from conversation and evidence; separate user decisions from recommendations, and look up repository facts yourself (never ask the user to paste an issue or file you can read). An existing intent with unresolved direction stays here.

**How the run arrived** decides which pre-interview checks run (continuation check, phase selection, small-fix classification): read the arrival table in [references/capture-procedures.md](references/capture-procedures.md#step-1-arrival-and-binding) and run every required check before asking any direction question. Reuse any classification or findings a handoff or forward carries rather than re-deriving them.

**A user's change.** When the input carries a [user's change](../execute/references/fix-entry.md#a-users-change) that alters runtime behavior and lacks the user's yes, ask one yes/no question under [question-format.md](../routing/references/contracts/question-format.md) before the interview, any edit, or any row. On no or no answer, write nothing, report it was not captured, and never send it back to `diagnose`. Unattended runs ask nothing and record the missing yes as a `blocker`.

**Binding the flow.** Record state with bare `agx flow …` ([flow-state.md](../execute/references/flow-commands.md#commands)). When resolving a flow `intent` did not create, run `agx flow bind` once (refuses on default branch or detached `HEAD`). Pass `--folder <work-folder>` on every `flow` call, using the resolved folder or the new folder under [chain-artifacts.md](../decompose/references/chain-artifacts.md#the-work-folder).

### Step 2. Challenge the intent

Keep this step on the direction. The [principles check](../routing/references/contracts/principles-check.md) belongs to a phase inspecting an artifact.

**The empirical fork.** Apply the [empirical questions contract](../routing/references/contracts/empirical-questions.md) before the challenge, recording observed answers as lookups without widening scope.

**The challenge.** Read and invoke [challenge](../challenge/SKILL.md) (and [chain-artifacts.md](../decompose/references/chain-artifacts.md) for the Challenge record's exact five fields — never improvise round format or record fields from memory) on why the work matters, observable success outcome, scope inclusions/exclusions, constraints, and goal-invalidating assumptions. Ask the direction questions and wait for the user's actual answers — never invent, assume, or simulate user answers or resolutions in the same turn, and never record a resolution the user did not give. Carry forward answered questions and resume unanswered ones until direction questions resolve (no round limit); keep solution design, interfaces, slicing, and tasks downstream.

**Execution safety gate.** The direction challenge must actually run with at least one round put to the user (or cite a carried-forward `prior record dated YYYY-MM-DD` per [chain-artifacts.md](../decompose/references/chain-artifacts.md)). Naming `challenge`, reading it, or weighing risks privately is not execution. Marking a challenge `ok` without running it is a status claim without evidence ([safety gate](../routing/references/contracts/gates.md#safety-gate)): hold it in that file's shape, offering the round now or a `declined` record.

**Open questions.** Label every remaining item in **Open questions**:

- `blocker`: unanswered direction question naming the decision or evidence needed (stays here).
- `deferred`: user-deferred (or waived under Step 4) with reason and resolver; silence is not deferral.
- `downstream`: solution behavior/design for `spec draft` or implementation detail for `decompose`, naming the owner (never disguise an unsettled problem, outcome, or scope).

**Declines, ended rounds, and unattended runs.** Honor a user decline immediately, save what is known, record `declined`, and leave unanswered questions as `blocker`. Read [references/capture-procedures.md](references/capture-procedures.md#step-2-challenge-edge-cases) for rounds the user ends mid-way (recommendations are never user answers) and [unattended runs](../routing/references/contracts/unattended.md#when-a-run-is-unattended).

### Step 3. Write the intent

Write or update `intent.md` per [chain-artifacts.md](../decompose/references/chain-artifacts.md#intentmd) with the six sections in order: **Problem**, **Outcome**, **Affected users and systems**, **Constraints**, **Decisions**, **Open questions**. Open a new intent with `# Intent: <topic>` (no front matter) and the five-line card from [skim-rules.md](../decompose/references/skim-rules.md#the-card) (`Change`, `Why now`, `Out of scope`, `Risk`, `Source` — no `Ask` line). Preserve existing intents' existing layout unless conversion is requested. When delivering or drafting a new `intent.md`, present the actual `# Intent:` title, five-line card, six sections, and `**Challenge record:**` rather than only describing them.

Read [references/capture-procedures.md](references/capture-procedures.md#step-3-writing-and-logging) for section-by-section content rules (allowed code under [Code in artifacts](../decompose/references/chain-artifacts.md#code-in-artifacts), forwarded constraints, user's change recording, Decisions, and the five-field `**Challenge record:**`), plus the `diagnose` waiver row and staging hygiene.

**Store, stage, and record.** Resolve storage and stage per [Write, stage, commit](../decompose/references/chain-artifacts.md#write-stage-commit), following [Feature branch before the first commit](../routing/references/chain-phases.md#feature-branch-before-the-first-commit). Note whether `intent.md` already existed (`create` logs only on first write). On a new file, stage `intent.md`, confirm it via `git diff --cached --name-only`, then log:

    agx flow log intent create intent.md ok "<one line: why the work exists>" intent.md "intent record the challenge result" --folder <work-folder>

After the interview ends, log the `challenge` row (`ok`, `declined`, or `pending` with `next` = `intent resume the challenge`):

    agx flow log intent challenge intent.md <ok|declined|pending> "<one line: what the challenge settled>" intent.md "intent ask for approval" --folder <work-folder>

Stage `state.tsv` separately (`git add -- <work-folder>/state.tsv`); never stage a local-store folder. Write no sibling files besides `state.tsv` (and an outgrown `brief.md`); use ignored `temp/` for scratch.

**After approval.** Never edit an approved `intent.md` body to record a divergence (editing stales approval); `execute` appends to `## Divergences` and `gate` reconciles per [chain-artifacts.md](../decompose/references/chain-artifacts.md#the-divergences-section).

### Step 4. Route to spec and ask for approval

**Route.** Every resolved intent goes to `spec draft`. Record the route in **Decisions** on one labelled line (e.g., `- **Next owner (assistant routing judgment):** spec — <what the solution must define>`).

**Open blockers (process gate).** While any `blocker` remains, write no `Next owner` line and ask no approval question. If the user asks to proceed anyway, name the cost once (`spec` would have to invent the answer). On the next reply still asking to proceed, relabel each blocker `deferred` (`waived by the user`, resolver `spec draft`), write the `Next owner` line, log one waiver row, and continue:

    agx flow log intent waive direction-blockers ok "<the user's reason, or none given>" none "intent ask for approval" --folder <work-folder>

**Status line and question.** Print one chat/report line before asking: `Status: <n> open questions (<d> deferred, <w> left for the spec) · approval pending` (or `Status: no open questions · approval pending`). Re-stage `intent.md`, run `agx flow status --folder <work-folder>`, note `hashes: intent.md text=<hash>`, show the concrete resolved path (never `<work-root>/<folder>`), the hash (never `<from flow status>` or `…`), and a direction summary (full text only on request), and ask:

> "Intent written to `<intent-path>`. Approve this direction? Your yes lets `spec draft` define the solution; nothing
> is implemented."

Ask in every attended run (including capture-only); unattended runs ask nothing and report approval pending. Intent never opens issues or authors slices.

**The answer and guarded approval.** Interpret the reply under [approval.md](../routing/references/contracts/approval.md) (full reply matrix, declined logging, re-asking, re-entry with approval pending, and [stale reapproval](../execute/references/flow-approval-and-audit.md#stale-approvals-and-reapproval) are in [references/capture-procedures.md](references/capture-procedures.md#step-4-approval-and-re-entry)). Third-party sign-off claims are not the user's approval; an affirmative with a change is an edit request (revise, re-stage, re-show, re-ask). On a clear affirmative, write the guarded row using the noted hash and stage `state.tsv`:

    agx flow log intent approve intent.md ok "<what the approval rested on>" auto "spec define the solution" --folder <work-folder> --expect-blob <hash>

If the [approval guard](../execute/references/flow-approval-and-audit.md#the-approval-guard) refuses, `intent.md` changed after display: re-run `flow status`, re-show, and re-ask (never bypass the guard).

**After a yes.** Explain the next step in plain language and name `spec draft` once. If the user authorized continuing past intent (up front or in the approving reply), load and run [spec](../spec/SKILL.md) (`spec draft`) in this turn without asking again or stopping at a description of what it would do; otherwise save and report the route without starting `spec draft`. No hand-off authorizes implementation, publication, or scope expansion.

## References

- [references/capture-procedures.md](references/capture-procedures.md) — arrival matrix, ended-round and unattended rules, Step 3 section/logging details, Step 4 reply/re-entry tables, and worked example.
- [references/brief.md](references/brief.md) — middle path's `brief.md`, review, approval, and hand-off to `execute`.
- [references/brief-reviewer-charter.md](references/brief-reviewer-charter.md) — independent reviewer's charter for a brief.
- [chain-artifacts.md](../decompose/references/chain-artifacts.md) — storage, provenance, intent sections, and challenge record rule.
- [skim-rules.md](../decompose/references/skim-rules.md) — card, `Source` line, and origin rules.
- [flow-state.md](../execute/references/flow-state.md) — `state.tsv` record, `agx flow` helper, and approval guard.
- [fix-entry.md](../execute/references/fix-entry.md) — user's change definition and waiver check.
- [chain-phases.md](../routing/references/chain-phases.md) — continuation check, small-fix classification, middle path, and feature-branch rule.
- [forwarding.md](../routing/references/forwarding.md) — route-away protocol and explicit invocation.
- [approval.md](../routing/references/contracts/approval.md), [gates.md](../routing/references/contracts/gates.md), [unattended.md](../routing/references/contracts/unattended.md), [question-format.md](../routing/references/contracts/question-format.md), [workflow-steps.md](../routing/references/contracts/workflow-steps.md), [principles-check.md](../routing/references/contracts/principles-check.md), [empirical-questions.md](../routing/references/contracts/empirical-questions.md) — shared contracts.
- [challenge](../challenge/SKILL.md) — scoped questioning, recommendations, and resolved-decision summary.
- [spec](../spec/SKILL.md) — next owner of every approved intent.
