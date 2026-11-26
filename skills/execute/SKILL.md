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

Runs approved work to "implemented and verified": approved plan in verified parallel waves, `brief.md`, decomposed task, or small fix. One quality review per review group before tasks count as done; `gate` owns completion.

Once the mode is known, announce in one line what this run does, authorized up-front steps, and steps still needing confirmation ([approval.md](../routing/references/contracts/approval.md#steps-named-up-front)). Apply [principles check](../routing/references/contracts/principles-check.md) when inspecting the plan or diff.

**Contracts:** [approval](../routing/references/contracts/approval.md), [gates](../routing/references/contracts/gates.md), [output-discipline](../routing/references/contracts/output-discipline.md) (`/agentics:<skill>`, `$agentics:<skill>` on Codex), [question-format](../routing/references/contracts/question-format.md), [unattended](../routing/references/contracts/unattended.md), [untrusted-data](../routing/references/contracts/untrusted-data.md), [severity](../routing/references/contracts/severity.md), [vocabulary](../routing/references/contracts/vocabulary.md).

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

New content re-reads from row 1; a plain repeat gets the same one-line forward, never a refusal script. Render each concrete forward explicitly across multi-request or pushback turns.

## Entry points

Plan (`/agentics:execute [auto [pause]|pair|arena] <plan> [S<n>]`), `brief.md` or decomposed task, `arena <task>` ([arena-mode.md](references/arena-mode.md)), `fix <one sentence>` ([The `fix` entry](#the-fix-entry)), or `diagnose` hand-off ([A diagnosed fix](#a-diagnosed-fix)). Table: [execution-rubrics.md](references/execution-rubrics.md#entry-points).

## Modes

- **`auto` (default):** fresh subagent/task, verified parallel waves (else 1-task waves), 1 quality reviewer/group, aggregate review at boundaries. **`auto pause`** pauses at wave ends ([Wave pauses](#wave-pauses)).
- **`pair`:** session implements serially with user, still dispatches Stage 1/quality/aggregate reviewers, pauses at wave ends, commits `mode=pair`, offers `handoff` + `/clear` after two same-issue corrections. User-selected only.
- **`arena`:** races N concurrent candidates on one task, cross-judges, grafts ([arena-mode.md](references/arena-mode.md)). Unavailable on `fix`.

Every mode runs the same model reviews; user reviews never replace them. Load [mode-procedures.md](references/mode-procedures.md#modes) and [execution-rubrics.md](references/execution-rubrics.md#modes-comparison).

**The controller rule.** In `auto`/`arena`, only subagents write code. Controller writes only flow artifacts (`state.tsv`, `## Divergences`, `## Standing orders`) and host task records, making only task, red, and `git revert --no-edit <sha>` commits. Findings, failed verifications, and harness repairs go to an implementer. Requesting controller patches is a process gate ([gates.md](../routing/references/contracts/gates.md#process-gate)): state cost once; on next asking reply, patch, commit, log `commit user` (`auto:<sha>`) + `waive controller-rule`. `fix` is exempt ([fix-entry.md](references/fix-entry.md)).

## Mode selection & dispatch preview

Pick first match (no menu): (1) explicit `pair`, `arena`, or wave pauses; (2) stored harness-memory preference (name it); (3) `auto`. Offer others only after user redirect ([question-format](../routing/references/contracts/question-format.md)).

**Dispatch permission.** Typed `/agentics:execute` grants dispatch on request-gated surfaces; `routing`/forwarded runs ask once ([surface-dispatch](../routing/references/surface-dispatch.md#dispatch-permission)), stopping per [Native dispatch by surface](#native-dispatch-by-surface) on no.

**Dispatch preview.** Print once in Step 1 (before native task load, without waiting) with concrete computed values ([output-discipline](../routing/references/contracts/output-discipline.md)): plan path, proportionality, waves/tiers/demotions, review groups & cycle check, verification preflight, Stage 1 statuses, total dispatch count, and selection records ([model-ladders](../routing/references/model-ladders.md#unavailable-selections-and-reporting)). Checklist: [execution-rubrics.md](references/execution-rubrics.md#dispatch-preview).

**Demotions never block.** A claimed-parallel group failing independence runs as 1-task waves in `auto`, stays one review group, logs `demote wave-<N>`. Overriding is a process gate: state finding/cost once; on next asking reply, run parallel with `waive "independence <group>"` (shared-file tasks stay serial).

## Step 0: Preflight

Call `agx flow … --folder <plan folder>` ([flow-commands](references/flow-commands.md#commands), [decision trail](references/plan-divergence-and-trail.md#decision-trail)). Load [mode-procedures.md](references/mode-procedures.md#step-0-preflight) before first `agx flow status`.

- **Examine before approving:** run all read-only refusing checks (**Don't use when**, Step 1 coverage, group cycles, dispatch availability for `auto`/`arena`) before asking approval or logging rows.
- **Approval:** from `agx flow status`: open blocking `review plan` findings -> safety hold (`decompose` revises); `current` runs; `stale`/`unconfirmed here` re-approves ([stale approvals](references/flow-approval-and-audit.md#stale-approvals-and-reapproval)); direct invocation approves (show path/hash); `routing`/forwarded unapproved plans forward to `decompose` ([target classification](../routing/references/forwarding.md#target-classification)); else ask once. Unattended logs `pause`. Unapproved routine-tier `spec.md` takes one combined approval; elevated-tier forwards to `spec`.
- **Resume:** reconcile `mismatch:`, stack-filtered `unaccounted:`, dirty tree outside work folder, and `pending` `verify` rows before dispatch ([resume reconciliation](references/mode-procedures.md#resume-reconciliation)).
- **Pushback:** follow [gates.md](../routing/references/contracts/gates.md) and [output-discipline.md](../routing/references/contracts/output-discipline.md#gate-response-shape) (process-gate yield logs one `waive` row).

## The `fix` entry

`/agentics:execute fix` runs a small fix with no plan: session edits, dispatched reviewer reviews, `gate` gets the offer. Load [fix-entry.md](references/fix-entry.md).

## A diagnosed fix

`diagnose` enters with root cause, evidence, failing reproduction, and test seam: (1) if blocking a running plan task, ask once to append a `## Divergences` task + `diverge` row (`--entry last`) with reproduction as red test; (2) if small-fix signals hold with no active flow, run [The `fix` entry](#the-fix-entry); (3) else forward findings to `intent`. Details: [mode-procedures.md](references/mode-procedures.md#a-diagnosed-fix). Three-attempt limit spans implementers ([Two-strike stop](#two-strike-stop)).

## Step 1: Load, review, and compute waves

Load [mode-procedures.md](references/mode-procedures.md#step-1-load-review-and-compute-waves) and run sub-steps 1–10:

1. **Read & coverage:** load input and [standing-orders.md](references/standing-orders.md); stop on unwaived blocking coverage items (`note coverage` or `**Coverage:**`).
2. **Persist:** keep in-repo plans; else persist via [Resolve the plan parent](../decompose/SKILL.md#resolve-the-plan-parent), [plan-artifact-identity.md](references/plan-artifact-identity.md), and [chain-artifacts.md](../decompose/references/chain-artifacts.md) without editing approved text.
3. **Verification preflight:** load [verification-preflight.md](references/verification-preflight.md) before probing; missing verification narrows work, never blocks bootstrap.
4. **Concerns:** pause unspecified tasks/dependents with **Gap / Blocked / May proceed / Question / Resolve**; continue only **May proceed**.
5. **Waves & groups:** apply **Parallel lanes** first if `**Lane:** yes`; build waves (no shared-file tasks together, never pull earlier), assign review groups, check cycles twice (cycle -> `decompose`), run **Independence Assessment Checklist** in `auto`/`arena`.
6. **Record run:** ensure feature branch ([work-item branch](../git/references/branch.md#the-work-item-branch)), log approval and confirm standing orders (`standing-orders.md`), log `enter run` + `agx flow bind` ([plan-divergence-and-trail.md](references/plan-divergence-and-trail.md)), commit committed work-folder artifacts (stop on failure).
7. **Tiers:** assign [Model tiers](#model-tiers).
8. **Stage 1:** apply [Stage 1 dispatch rule](#stage-1-dispatch-rule) within [Concurrency and overlap](../routing/references/surface-dispatch.md#concurrency-and-overlap).
9. **Preview:** print dispatch preview; do not wait.
10. **Native tasks & todo:** seed tasks via [native-plan-projection.md](../routing/references/native-plan-projection.md) and track [workflow steps](../routing/references/contracts/workflow-steps.md).

### Independence Assessment Checklist

Every claimed-parallel pair must pass all five checks (no shared writes, no output dependency, no shared mutable/generated state, no ordering constraint, independent verification): [execution-rubrics.md](references/execution-rubrics.md#independence-assessment-checklist).

## Step 2: execute waves

Load [mode-procedures.md](references/mode-procedures.md#step-2-execute-waves) before wave 1; load each prompt on first dispatch (never from memory): [spec-reviewer](references/spec-reviewer-prompt.md), [implementer](references/implementer-prompt.md), [code-quality-reviewer](references/code-quality-reviewer-prompt.md), [aggregate-reviewer](references/aggregate-reviewer-prompt.md) ([review-gated-process](references/review-gated-process.md), [controller-example](references/controller-example.md)).

- **Fresh context & propagation:** inline full task contract and acceptance criteria (bulk grounding as pointers), full `## Standing Orders` verbatim (or `None recorded.`), open divergences fenced as data. Mid-run changes: constraint -> `order` row; rule exception -> [user-authorized divergence](references/plan-divergence-and-trail.md#user-authorized-divergence); scope/criteria -> `decompose`.
- **Verify, commit, account:** load [flow-evidence.md](references/flow-evidence.md) before first `verify` row. On `DONE`/`DONE_WITH_CONCERNS`, commit only implementer-changed files with explicit pathspec ([commit contract](../git/references/commit.md#the-commit-contract)), verify yourself ([Running it](../routing/references/repo-conventions.md#running-it)), log `verify <task-id>` `pending` (`auto:<sha>`), then `ok`/`failed` (`commit=<sha> patch-id=<id>`) at verdict (never amend; undo via `git revert --no-edit <sha>`). One commit/task, never push; other commits log `commit <fix|triage|user|housekeeping>` (`auto:<sha>`, [commit accounting](references/flow-approval-and-audit.md#commit-accounting)). Isolated worktrees: [isolated-packets.md](references/isolated-packets.md).
- **Red commit (bug fix):** shared worktree runs `RED` leg first, controller confirms failure and commits tests (`verify red-<task-id>`, or no-commit form [evidence forms](references/flow-evidence.md#evidence-forms) if hook rejects — never bypass hooks), then fix leg.
- **Snapshot reviews & groups:** pass explicit commit lists, never live tree/ranges. One fresh quality reviewer/group once non-stopped tasks are verified or waived (`waive "quality-review <group>"` clears quality, not aggregate). Log UTC `review` rows (`kind`, `dispatched`, `round`, `blocked`) and `escape` rows for later Critical/Important defects. Statuses: [execution-rubrics.md](references/execution-rubrics.md#implementer-statuses).
- **Boundary check:** at each review-group boundary and before handoff, check `agx flow status` `unaccounted:` (stack-filtered via [stack-base resolver](references/plan-divergence-and-trail.md#the-stack-base-resolver); `not computed` != `none`) and diff commit paths vs packet `**Files:**` (`diverge` with target `<task-id> file scope` for extra paths) before releasing dependents. Standalone fixes run [standalone fix commit audit](references/fix-entry.md#standalone-fix-commit-audit) before handoff.
- **Wave end & state:** run wave integration verification (`verify wave-<N>`), skeleton Outcome proof, and plan divergence before releasing later tasks. Read/close no tracker issues; update host task tool per `native-plan-projection.md`.

### Stage 1 dispatch rule

Reviews task packets against approved plan at Deep tier before implementation. Load [stage-1-dispatch-rule.md](references/stage-1-dispatch-rule.md) at Step 1 sub-step 8.

### Aggregate wave review

Runs at **every** review-group boundary after quality review clears—each group gets a `review` (`kind=integration`) return or evidenced `waive aggregate-review` row before `verify group-<name>`; 2+ mutually independent groups also get one final integration review. Load [aggregate-wave-review.md](references/aggregate-wave-review.md) before first boundary.

### Two-strike stop

Two failures bring one fresh implementer; two more stop that task (not the run) and ask the user. Load [two-strike-stop.md](references/two-strike-stop.md) on first failure.

## Wave pauses

In `pair` and `auto pause`, stop at each wave's end for user review. Load [wave-pauses.md](references/wave-pauses.md) at first boundary.

## Plan divergence

Stale artifacts disagree with the diff. Load [plan-divergence-and-trail.md](references/plan-divergence-and-trail.md) before each wave boundary report and before releasing later-wave tasks.

## Repeated trouble in a settled design

Two `diverge`/`pivot`/`block` rows, findings, or concerns of the same shape across different tasks signal a design problem (not [Two-strike stop](#two-strike-stop)). Hold affected tasks, let independent tasks run, and ask once ([question-format](../routing/references/contracts/question-format.md)) to forward evidence to `deepen` (recommend yes). Yes -> forward + `pause run`; no -> release + `waive "deepen <shape>"`. Unattended logs `pause`. Details: [mode-procedures.md](references/mode-procedures.md#repeated-trouble-in-a-settled-design).

## Decision trail

Log decisions to `state.tsv` via `agx flow log` with a one-line `why` on every controller-written row ([flow-state.md](../execute/references/flow-state.md), [decision trail](references/plan-divergence-and-trail.md#decision-trail)). Task commits log `verify <task-id>` `pending` (`auto:<sha>`), then close `ok`/`failed` (`commit=<sha> patch-id=<id>`) at group verdict; dropped tasks log `drop <task-id>` `cancelled`; unfinished turns log `pause run`.

## Model tiers

Load [model-ladders.md](../routing/references/model-ladders.md) before resolving any tier, model, or effort (never from memory; distinguish user effort overrides from model base efforts; report observed bindings). Stage 1 = Deep; quality/aggregate/final = Deep, escalating to Max if group had a Deep implementer (Standard only when all tasks are Light with `**Packet review:** no`). Separate reviewer per group. Rubric: [execution-rubrics.md](references/execution-rubrics.md#model-tiers).

## Parallel lanes

Plans with `**Lane:** yes` slices use lanes; execute writes launch packets and starts no lane session. Load [parallel-lanes.md](references/parallel-lanes.md) for lanes or `S<n>`.

## Native dispatch by surface

Dispatch by available tools ([surface-dispatch.md](../routing/references/surface-dispatch.md)). If absent/refused, `auto`/`arena` implement nothing, log nothing, and state only `pair` runs here (never auto-select `pair`). In-session `pair` reviews: [parallel-lanes.md](references/parallel-lanes.md#native-dispatch-by-surface).

## Gates

Safety gates (status evidence; plan review before approval; commit hooks and signing) and repository policy (draft PRs, no push/merge) never waive. Process gates yield on the next declining reply with one `waive` row (ignoring `flow status` `invalid:`, [invalid rows](references/flow-commands.md#invalid-rows)): `waive "quality-review <group>"`, `waive "verification <task-ids>"` ([deferred verification](references/verification-preflight.md#deferred-verification)), `waive controller-rule` (+ `commit user`), `waive "independence <group>"`, `waive "deepen <shape>"`, plus Stage 1, wave-pause, and preflight waivers (carrying unverified-waiver notice in preview, packets, and report). Table: [execution-rubrics.md](references/execution-rubrics.md#gates).

**The completion handoff.** After final **boundary check**, offer `gate` and wait (naming waived unverified/unreviewed tasks). If close-out was named ("commit, push, and open a draft PR", "then gate it"), run `gate` directly; earlier draft-PR/push naming answers only plain draft proposal, never `gh stack` ([gate Answers](../gate/references/branch/propose-and-execute.md#answers)). Execute never pushes; "not yet" logs `pause`.

## When to pause

Load [when-to-pause.md](references/when-to-pause.md) on blockers, gaps, unclear instructions, shortcut pressure, or before claiming verified status.

## Definition of done

- Every task's latest `verify` row is `ok`, `drop`, or waived `pending` (named unverified in report/`gate` offer); all `DONE_WITH_CONCERNS` resolved or accepted.
- Every review group cleared quality review (or waived) and every triggered integration/final review cleared (or waived).
- Verification re-run over combined changes for all runnable checks.
- Plan runs have no stack-filtered unaccounted commit in `agx flow status` and every out-of-packet path has a `diverge` row. Standalone fixes passed [standalone fix commit audit](references/fix-entry.md#standalone-fix-commit-audit) (exit 0) and carry its findings.

Open `NEEDS_CONTEXT`, `BLOCKED`, or stopped tasks mean not done: report open, then hand off per [Gates](#gates).

## Integration

Upstream: `decompose`, `diagnose` ([A diagnosed fix](#a-diagnosed-fix)). Mid-run: `deepen` ([Repeated trouble in a settled design](#repeated-trouble-in-a-settled-design)), `tdd`, `review`, `git`, `handoff`. Downstream: `gate`.

## References

- **Procedures & rubrics:** [mode-procedures](references/mode-procedures.md), [execution-rubrics](references/execution-rubrics.md), [controller-example](references/controller-example.md), [review-gated-process](references/review-gated-process.md), [when-to-pause](references/when-to-pause.md)
- **Prompts:** [spec-reviewer](references/spec-reviewer-prompt.md), [implementer](references/implementer-prompt.md), [code-quality-reviewer](references/code-quality-reviewer-prompt.md), [aggregate-reviewer](references/aggregate-reviewer-prompt.md)
- **Flow & audit:** [flow-state](references/flow-state.md), [flow-evidence](references/flow-evidence.md), [flow-approval-and-audit](references/flow-approval-and-audit.md), [flow-commands](references/flow-commands.md), [plan-artifact-identity](references/plan-artifact-identity.md), [plan-divergence-and-trail](references/plan-divergence-and-trail.md), [standing-orders](references/standing-orders.md)
- **Execution controls:** [stage-1-dispatch-rule](references/stage-1-dispatch-rule.md), [aggregate-wave-review](references/aggregate-wave-review.md), [verification-preflight](references/verification-preflight.md), [two-strike-stop](references/two-strike-stop.md), [wave-pauses](references/wave-pauses.md), [isolated-packets](references/isolated-packets.md), [parallel-lanes](references/parallel-lanes.md), [arena-mode](references/arena-mode.md), [fix-entry](references/fix-entry.md)
- **Shared:** [model-ladders](../routing/references/model-ladders.md), [surface-dispatch](../routing/references/surface-dispatch.md), [copilot-tools](../routing/references/copilot-tools.md), [chain-artifacts](../decompose/references/chain-artifacts.md)
