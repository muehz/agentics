---
name: execute
description: "Use when an approved plan, an approved brief, or one already-decomposed task is ready to implement, or when a small fix is ready to make. Carry tasks through implementation and verification, run independent work in parallel where useful, and preserve the agreed scope and approval boundaries."
version: "5.6.0"
user-invocable: true
disable-model-invocation: false
argument-hint: "[auto [pause]|pair|arena] [plan path, brief path, inline task list, or one task] [S<n>] | fix <one-sentence change>"
metadata:
    surfaces: [claude-code, copilot-cli, vscode, codex, antigravity]
    modes: [auto, pair, arena, fix]
---

# Executing plans

Execute runs approved work to "implemented and verified": an approved plan in verified parallel waves, a `brief.md` or one decomposed task, or a small fix. Each review group gets one quality review before tasks count as done; `gate` owns completion.

Once the mode is known, announce in one line what this run does, which up-front steps are authorized, and which still need confirmation ([approval.md](../routing/references/contracts/approval.md#steps-named-up-front)). Apply the [principles check](../routing/references/contracts/principles-check.md) when inspecting the plan or diff.

**Shared contracts:** [approval.md](../routing/references/contracts/approval.md), [gates.md](../routing/references/contracts/gates.md), [question-format.md](../routing/references/contracts/question-format.md), [unattended.md](../routing/references/contracts/unattended.md), [untrusted-data.md](../routing/references/contracts/untrusted-data.md), [severity.md](../routing/references/contracts/severity.md), [vocabulary.md](../routing/references/contracts/vocabulary.md).

Use `/agentics:<skill>` (`$agentics:<skill>` on Codex).

## Don't use when

First match wins. On explicit invocation ([forwarding.md](../routing/references/forwarding.md#explicit-invocation)): **cannot** forwards; **better elsewhere** notes the owner in one line and runs. Otherwise every row forwards.

| Order | Input signal                                                                              | Kind             | Owner               |
| ----- | ----------------------------------------------------------------------------------------- | ---------------- | ------------------- |
| 1     | Unknown-cause bug/failure, unless [user's change](references/fix-entry.md#a-users-change) | cannot           | `diagnose`          |
| 2     | Only a goal or wish (no plan, brief, task, or small fix)                                  | cannot           | `intent`            |
| 3     | Intent or spec without a task plan                                                        | cannot           | `decompose`         |
| 4     | Tasks not decomposed (**Step 0**), or slices without `### Task`                           | cannot           | `decompose`         |
| 5     | Approved plan with unwaived blocking coverage item (Step 1 sub-step 1)                    | cannot           | `decompose`         |
| 6     | All tasks done and verified; request is completion, readiness, or PR                      | cannot           | `gate`              |
| 7     | Separate checkout wanted before coding                                                    | better elsewhere | `git` (`worktrees`) |
| 8     | Otherwise                                                                                 | runs             | execute             |

New content re-reads from row 1; a plain repeat gets the same one-line forward, never a refusal script.

## Entry points

Accepts a plan (`/agentics:execute [auto [pause]|pair|arena] <plan> [S<n>]`), a `brief.md` or decomposed task, `arena <task>` ([arena-mode.md](references/arena-mode.md)), `fix <one sentence>` ([The `fix` entry](#the-fix-entry)), or a `diagnose` hand-off ([A diagnosed fix](#a-diagnosed-fix)). Table: [execution-rubrics.md](references/execution-rubrics.md#entry-points).

## Modes

- **`auto` (default):** fresh subagent per task, verified parallel waves (else one-task waves), one quality reviewer per review group, then aggregate review at boundaries. **`auto pause`** pauses at each wave's end ([Wave pauses](#wave-pauses)).
- **`pair`:** session implements serially with the user, still dispatches Stage 1/quality/aggregate reviewers, pauses at planned wave ends, commits with `mode=pair`, and offers `handoff` + `/clear` after two same-issue corrections. User-selected only.
- **`arena`:** races N concurrent candidates on one task, cross-judges, and grafts ([arena-mode.md](references/arena-mode.md)). Unavailable on `fix`.

Every mode runs the same model reviews; user reviews never replace them. Load [mode-procedures.md](references/mode-procedures.md#modes) and [execution-rubrics.md](references/execution-rubrics.md#modes-comparison).

**The controller rule.** In `auto` and `arena`, only subagents write code. The controller writes only flow artifacts (`state.tsv`, `## Divergences`, `## Standing orders`) and host task records, making only task, red, and `git revert --no-edit <sha>` commits. Findings, failed verifications, and harness repairs go to an implementer. A user request for the controller to patch code is a process gate ([gates.md](../routing/references/contracts/gates.md#process-gate)): state cost once; on the next asking reply, patch, commit, and log `commit user` + `waive controller-rule`. `fix` is the exception ([fix-entry.md](references/fix-entry.md)).

## Mode selection & dispatch preview

Pick first match (no mode menu): (1) explicit argument or request for `pair`, `arena`, or wave pauses; (2) stored harness-memory preference (name it); (3) `auto`. Offer other modes only after a user redirect ([question format](../routing/references/contracts/question-format.md)).

**Dispatch permission.** Typed `/agentics:execute` grants dispatch on request-gated surfaces; `routing`-selected or forwarded runs ask once ([surface dispatch](../routing/references/surface-dispatch.md#dispatch-permission)), stopping per [Native dispatch by surface](#native-dispatch-by-surface) on no.

**Dispatch preview.** Print once in Step 1 (before native task load, without waiting) with plan path, proportionality, waves/tiers/demotions, review groups & cycle check, verification preflight, Stage 1 statuses, total dispatch count, and selection records ([model ladder](../routing/references/model-ladders.md#unavailable-selections-and-reporting)). Checklist: [execution-rubrics.md](references/execution-rubrics.md#dispatch-preview).

**Demotions never block.** A claimed-parallel group failing independence runs as one-task waves in `auto`, stays one review group, and logs `demote wave-<N>`. Overriding is a process gate: state finding and cost once; on the next asking reply, run parallel with `waive "independence <group>"` (shared-file tasks still run serially).

## Step 0: Preflight

Call `agx flow … --folder <plan folder>` ([flow-state.md](references/flow-commands.md#commands), [plan-divergence-and-trail.md](references/plan-divergence-and-trail.md#decision-trail)). Load [mode-procedures.md](references/mode-procedures.md#step-0-preflight) before the first `agx flow status` for input, approval, and resume tables.

- **Examine before approving.** Run all read-only refusing checks (**Don't use when**, Step 1 coverage, group cycles, dispatch availability for `auto`/`arena`) before asking approval or logging any row.
- **Approval.** From `agx flow status`: open blocking `review plan` findings are a safety hold (`decompose` revises); `current` runs; `stale`/`unconfirmed here` re-approves ([flow-state.md](references/flow-approval-and-audit.md#stale-approvals-and-reapproval)); direct invocation approves (show path & hash); `routing`/forwarded unapproved plans forward to `decompose` ([forwarding.md](../routing/references/forwarding.md#target-classification)); else ask once. Unattended runs log `pause`. Unapproved routine-tier `spec.md` takes one combined approval; elevated-tier forwards to `spec`.
- **Resume.** Reconcile `mismatch:`, stack-filtered `unaccounted:`, dirty tree outside work folder, and `pending` `verify` rows before dispatch ([mode-procedures.md](references/mode-procedures.md#resume-reconciliation)).
- **Pushback.** State a safety hold once ([gates.md](../routing/references/contracts/gates.md#safety-gate)); state a process gate's cost once and yield with one `waive` row on the next declining reply.

## The `fix` entry

`/agentics:execute fix` runs a small fix with no plan: this session edits, a dispatched reviewer reviews, and `gate` gets the offer. Load [fix-entry.md](references/fix-entry.md).

## A diagnosed fix

`diagnose` enters with root cause, evidence, failing reproduction, and test seam: (1) if blocking a running plan task, ask once to append a `## Divergences` task + `diverge` row (`--entry last`) with the reproduction as red test; (2) if small-fix signals hold with no active flow, run [The `fix` entry](#the-fix-entry); (3) else forward findings to `intent`. Details: [mode-procedures.md](references/mode-procedures.md#a-diagnosed-fix). Three-attempt limit spans implementers ([Two-strike stop](#two-strike-stop)).

## Step 1: Load, review, and compute waves

Load [mode-procedures.md](references/mode-procedures.md#step-1-load-review-and-compute-waves) and execute sub-steps 1–10:

1. **Read & coverage:** load input and [standing-orders.md](references/standing-orders.md); stop on unwaived blocking coverage items (`note coverage` or `**Coverage:**`).
2. **Persist:** keep in-repo plans; else persist via [Resolve the plan parent](../decompose/SKILL.md#resolve-the-plan-parent), [plan-artifact-identity.md](references/plan-artifact-identity.md), and [chain-artifacts.md](../decompose/references/chain-artifacts.md) without editing approved text.
3. **Verification preflight:** load [verification-preflight.md](references/verification-preflight.md) before probing; missing verification narrows work, never blocks its bootstrap.
4. **Concerns:** pause unspecified tasks/dependents with **Gap / Blocked / May proceed / Question / Resolve**; continue only **May proceed**.
5. **Waves & groups:** apply **Parallel lanes** first if `**Lane:** yes`; build waves (no shared-file tasks together, never pull earlier than planned), assign review groups, check cycles twice (cycle stops -> `decompose`), and run **Independence Assessment Checklist** in `auto`/`arena`.
6. **Record run:** ensure feature branch ([The work-item branch](../git/references/branch.md#the-work-item-branch)), log approval and confirm standing orders (`standing-orders.md`), log `enter run` + `agx flow bind` ([plan-divergence-and-trail.md](references/plan-divergence-and-trail.md)), commit committed work-folder artifacts (stop on failure).
7. **Tiers:** assign [Model tiers](#model-tiers).
8. **Stage 1:** apply [Stage 1 dispatch rule](#stage-1-dispatch-rule) across waves within [Concurrency and overlap](../routing/references/surface-dispatch.md#concurrency-and-overlap).
9. **Preview:** print dispatch preview; do not wait.
10. **Native tasks & todo:** seed tasks via [native-plan-projection.md](../routing/references/native-plan-projection.md) and track [workflow steps](../routing/references/contracts/workflow-steps.md).

### Independence Assessment Checklist

Every claimed-parallel pair must pass all five checks (no shared writes, no output dependency, no shared mutable/generated state, no ordering constraint, independent verification): [execution-rubrics.md](references/execution-rubrics.md#independence-assessment-checklist).

## Step 2: execute waves

Load [mode-procedures.md](references/mode-procedures.md#step-2-execute-waves) before wave 1; load each prompt on first dispatch (never from memory): [spec-reviewer-prompt.md](references/spec-reviewer-prompt.md), [implementer-prompt.md](references/implementer-prompt.md), [code-quality-reviewer-prompt.md](references/code-quality-reviewer-prompt.md), [aggregate-reviewer-prompt.md](references/aggregate-reviewer-prompt.md) ([review-gated-process.md](references/review-gated-process.md), [controller-example.md](references/controller-example.md)).

- **Fresh context & propagation:** inline full task contract and acceptance criteria (bulk grounding as pointers), full `## Standing Orders` verbatim (or `None recorded.`), and open divergences fenced as data. Mid-run changes: constraint -> `order` row; rule exception -> [user-authorized divergence](references/plan-divergence-and-trail.md#user-authorized-divergence); scope/criteria -> `decompose`.
- **Verify, commit, account:** on `DONE`/`DONE_WITH_CONCERNS`, commit only implementer-changed files with explicit pathspec ([commit contract](../git/references/commit.md#the-commit-contract)), verify yourself ([Running it](../routing/references/repo-conventions.md#running-it)), log `verify <task-id>` `pending` (`auto:<sha>`), then `ok`/`failed` at verdict (never amend; undo with `git revert --no-edit <sha>`). One task commit per task, never push; other commits log `commit <fix|triage|user|housekeeping>` (`auto:<sha>`, [flow-state.md](references/flow-approval-and-audit.md#commit-accounting)). Isolated worktrees: [isolated-packets.md](references/isolated-packets.md).
- **Red commit (bug fix):** shared worktree runs `RED` leg first, controller confirms failure and commits test files (`verify red-<task-id>`, or no-commit form [flow-state.md](references/flow-evidence.md#evidence-forms) if hook rejects — never bypass hooks), then fix leg.
- **Snapshot reviews & review groups:** pass explicit commit lists, never live tree/ranges. One fresh quality reviewer per review group once non-stopped tasks are verified or waived (`waive "quality-review <group>"` clears quality review, not aggregate). Log UTC `review` rows (`kind`, `dispatched`, `round`, `blocked`) and `escape` rows for later Critical/Important defects. Statuses: [execution-rubrics.md](references/execution-rubrics.md#implementer-statuses).
- **Boundary check:** at each review-group boundary and before handoff, check `agx flow status` `unaccounted:` (stack-filtered via [the stack-base resolver](references/plan-divergence-and-trail.md#the-stack-base-resolver); `not computed` != `none`) and diff commit paths vs packet `**Files:**` (`diverge` for extra paths) before releasing dependents. Standalone fixes run [standalone fix commit audit](references/fix-entry.md#standalone-fix-commit-audit) before handoff.
- **Wave end & state:** run wave integration verification (`verify wave-<N>`), skeleton Outcome proof, and plan divergence before releasing later tasks. Read/close no tracker issues; update host task tool per `native-plan-projection.md`.

### Stage 1 dispatch rule

Reviews task packets against the approved plan at Deep tier before implementation. Load [stage-1-dispatch-rule.md](references/stage-1-dispatch-rule.md) at Step 1 sub-step 8.

### Aggregate wave review

Runs at each review-group boundary after quality review clears; 2+ mutually independent groups also get one final integration review. Load [aggregate-wave-review.md](references/aggregate-wave-review.md) before the first boundary.

### Two-strike stop

Per-task failure ladder: two failures bring one fresh implementer; two more stop that task (not the run) and ask the user. Load [two-strike-stop.md](references/two-strike-stop.md) on first failure.

## Wave pauses

In `pair` and `auto pause`, stop at each wave's end for user review. Load [wave-pauses.md](references/wave-pauses.md) at the first wave boundary.

## Plan divergence

Stale artifacts disagree with the diff. Load [plan-divergence-and-trail.md](references/plan-divergence-and-trail.md) before each wave boundary report and before releasing later-wave tasks.

## Repeated trouble in a settled design

Two `diverge`/`pivot`/`block` rows, findings, or concerns of the same shape across different tasks signal a design problem (not [Two-strike stop](#two-strike-stop)). Hold affected tasks, let independent tasks run, and ask once ([question format](../routing/references/contracts/question-format.md)) to forward evidence to `deepen` (recommend yes). Yes -> forward + `pause run`; no -> release + `waive "deepen <shape>"`. Unattended runs log `pause`. Details: [mode-procedures.md](references/mode-procedures.md#repeated-trouble-in-a-settled-design).

## Decision trail

Log decisions to the plan folder's `state.tsv` via `agx flow log` ([flow-state.md](../execute/references/flow-state.md), [plan-divergence-and-trail.md](references/plan-divergence-and-trail.md#decision-trail)). Task commits log `verify <task-id>` `pending`; dropped tasks/dependents log `drop <task-id>` `cancelled`; unfinished turns log `pause run`.

## Model tiers

Apply [model ladder](../routing/references/model-ladders.md) without restating it. Stage 1 = Deep; quality/aggregate/final = Deep, escalating to Max if the group had a Deep implementer (Standard only when all tasks are Light with `**Packet review:** no`). Keep a separate reviewer per review group. Rubric: [execution-rubrics.md](references/execution-rubrics.md#model-tiers).

## Parallel lanes

Plans with `**Lane:** yes` slices use lanes; execute writes launch packets and starts no lane session. Load [parallel-lanes.md](references/parallel-lanes.md) for lanes or `S<n>`.

## Native dispatch by surface

Dispatch by available tools ([Surface dispatch](../routing/references/surface-dispatch.md)). If dispatch is absent or refused, `auto`/`arena` implement nothing, log nothing, and state only `pair` runs here (never auto-select `pair`). In-session `pair` reviews: [parallel-lanes.md](references/parallel-lanes.md#native-dispatch-by-surface).

## Gates

Safety gates (no status claim without run evidence; plan review before approval; commit hooks and signing) and repository policy (draft PRs, no push/merge) never waive. Process gates state cost once and yield on the next declining reply with one `waive` row (ignoring rows in `flow status` `invalid:`, [flow-state.md](references/flow-commands.md#invalid-rows)): `waive "quality-review <group>"`, `waive "verification <task-ids>"` ([deferred verification](references/verification-preflight.md#deferred-verification)), `waive controller-rule` (+ `commit user`), `waive "independence <group>"`, `waive "deepen <shape>"`, plus Stage 1, wave-pause, and preflight waivers. Table: [execution-rubrics.md](references/execution-rubrics.md#gates).

**The completion handoff.** After the final **boundary check**, offer `gate` and wait (naming waived unverified/unreviewed tasks). If close-out was already named ("commit, push, and open a draft PR", "then gate it"), run `gate` directly; earlier draft-PR/push naming answers only the plain draft proposal, never `gh stack` ([gate's Answers](../gate/references/branch/propose-and-execute.md#answers)). Execute never pushes; "not yet" logs `pause`.

## When to pause

Load [when-to-pause.md](references/when-to-pause.md) on blockers, gaps, unclear instructions, shortcut pressure, or before claiming verified status.

## Definition of done

- Every task's latest `verify` row is `ok`, `drop`, or a waived `pending` row named unverified in the report and `gate` offer; all `DONE_WITH_CONCERNS` resolved or accepted.
- Every review group cleared quality review (or waived) and every triggered integration/final review cleared (or waived).
- Verification re-run over combined changes for all runnable checks.
- Plan runs have no stack-filtered unaccounted commit in `agx flow status` and every out-of-packet path has a `diverge` row. Standalone fixes passed [standalone fix commit audit](references/fix-entry.md#standalone-fix-commit-audit) (exit 0) and carry its findings.

Open `NEEDS_CONTEXT`, `BLOCKED`, or stopped tasks mean not done: report open, then hand off per [Gates](#gates).

## Integration

Upstream: `decompose`, `diagnose` ([A diagnosed fix](#a-diagnosed-fix)). Mid-run: `deepen` ([Repeated trouble in a settled design](#repeated-trouble-in-a-settled-design)), `tdd`, `review`, `git`, `handoff`. Downstream: `gate`.

## References

- [mode-procedures.md](references/mode-procedures.md): mode rules, Step 0–2 mechanics, boundary check, red commit, settled-design escalation.
- [execution-rubrics.md](references/execution-rubrics.md): entry points, modes, preview/independence checklists, statuses, tiers, gates.
- [aggregate-reviewer-prompt.md](references/aggregate-reviewer-prompt.md): aggregate reviewer prompt and delta.
- [aggregate-wave-review.md](references/aggregate-wave-review.md): group-boundary and final integration review.
- [arena-mode.md](references/arena-mode.md): bakeoff phases, cost preview, re-run, synthesis.
- [code-quality-reviewer-prompt.md](references/code-quality-reviewer-prompt.md): review-group quality reviewer prompt.
- [controller-example.md](references/controller-example.md): worked `auto` run.
- [fix-entry.md](references/fix-entry.md): `/agentics:execute fix`, records, audit.
- [flow-state.md](references/flow-state.md): flow-state layout, storage, events, operands, and phase selection.
- [flow-evidence.md](references/flow-evidence.md): evidence forms, `verify` validation, rows each skill writes, pre-v2 notes.
- [flow-approval-and-audit.md](references/flow-approval-and-audit.md): approval guard, reapproval, clone ledger, commit accounting, branch audit.
- [flow-commands.md](references/flow-commands.md): `agx flow` commands, exit codes, flow lookup, `flow status`, slice scope.
- [implementer-prompt.md](references/implementer-prompt.md): implementer prompt, red leg, sketch rule.
- [isolated-packets.md](references/isolated-packets.md): isolated worktree commits, release, repair, cleanup.
- [parallel-lanes.md](references/parallel-lanes.md): lanes, launch packets, union rule, native dispatch.
- [plan-artifact-identity.md](references/plan-artifact-identity.md): non-repository plan destination.
- [plan-divergence-and-trail.md](references/plan-divergence-and-trail.md): divergence comparison and trail rows.
- [review-gated-process.md](references/review-gated-process.md): review-gated flow diagrams.
- [spec-reviewer-prompt.md](references/spec-reviewer-prompt.md): Stage 1 packet reviewer prompt.
- [stage-1-dispatch-rule.md](references/stage-1-dispatch-rule.md): Stage 1 skip rules, budget, waiver.
- [standing-orders.md](references/standing-orders.md): standing-orders register and plan approval record.
- [two-strike-stop.md](references/two-strike-stop.md): failure ladder, stop, answers.
- [verification-preflight.md](references/verification-preflight.md): verification probe, bootstrap, waiver.
- [wave-pauses.md](references/wave-pauses.md): wave pause, feedback, waiver.
- [when-to-pause.md](references/when-to-pause.md): blockers, unverified claims, status rule.
- [model ladder](../routing/references/model-ladders.md): tiers, bindings, overrides, reporting.
- [surface dispatch](../routing/references/surface-dispatch.md): surface dispatch, concurrency, permission.
- [chain-artifacts.md](../decompose/references/chain-artifacts.md): work folder and provenance header.
- [copilot-tools.md](../routing/references/copilot-tools.md): Copilot CLI tool roster.
