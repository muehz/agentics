---
name: decompose
description: 'Use when the direction is settled and the next question is "how do we carry this out?" Turn the agreed solution into ordered, testable tasks with clear dependencies and delivery checkpoints.'
version: "4.28.0"
user-invocable: true
disable-model-invocation: false
argument-hint: "[design document, spec, or requirements]"
metadata:
    surfaces: [claude-code, copilot-cli, vscode, codex, antigravity]
    artifact: plan.md
---

# Decompose approved work into implementation tasks

Follow [output-discipline.md](../routing/references/contracts/output-discipline.md) (`/agentics:<skill>`, `$agentics:<skill>` on Codex).

## Overview

Turns concrete requirements into delivery slices where needed, then an ordered, testable Markdown plan that `execute` consumes directly: independently verifiable tasks, acceptance criteria, explicit dependencies, parallel groups, and deterministic paths/symbols/commands.

**Announce once, after Step 0.** When Step 0 accepts input, state in one line what this run will do (write plan, update slice, slices only, re-check coverage, or ask approval of an existing plan) and name authorized up-front steps ([approval.md](../routing/references/contracts/approval.md#steps-named-up-front)). Brief answers or Step 0 forwards print no announce line. Apply [principles check](../routing/references/contracts/principles-check.md) when planning from an artifact or inspecting a diff: read the principles index and matching leaf reference file(s) in full before writing tasks, apply their self-tests to the observed states, and turn each resulting recovery or check into a concrete task citing what changed.

## Don't use when

Read in order; first match wins. Kinds follow [forwarding.md](../routing/references/forwarding.md#explicit-invocation): **cannot** forwards via [forwarding.md](../routing/references/forwarding.md); **better elsewhere** under explicit invocation notes the other skill in one line and runs here.

1. **Hypothetical/exploratory ask** ("roughly", "ballpark", "not asking you to plan it") — _handled here_: answer briefly, no workflow (Step 0).
2. **No intent, spec, legacy design, or concrete inline requirements** — _cannot_: `intent`.
3. **Six-section intent whose `intent.md` approval is not current in `flow status`** — _cannot_: `intent`.
4. **Spec with no current approval and no recorded `**Spec review:**`, open policy conflict, or no header** — _cannot_: `spec` (resumed at approval).
5. **Approved `brief.md`** (one-task middle-path change) — _cannot_: `execute`.
6. **Bug/failure fix with unproven root cause** — _cannot_: `diagnose` (proven fixes proceed from `diagnose`).
7. **Request to implement a plan with current approval** — _cannot_: `execute`.
8. **Request to verify finished work** — _cannot_: `gate`.
9. **Backlog story splitting for sprint refinement** without intent/spec/design/inline requirements — _cannot_: no skill; say so and help directly if asked.
10. **One action with nothing to sequence** — _better elsewhere_: no skill; do directly.
11. **Six-section intent with current approval and no approved spec** — _handled here_: run `spec draft` first, then continue (Step 0).
12. **Otherwise** (approved/routine-reviewed spec, in-flight intent, legacy design, inline requirements) — _handled here_: stay here (slice issue limits update to its slice).

## Plan Storage

Default to **repository storage** in the work folder ([references/chain-artifacts.md](references/chain-artifacts.md)); ask no storage question. Repo instructions set only the work root, never storage mode.

- **Repository (default):** `<plan-parent>/YYYY-MM/<purpose>-<topic>/plan.md` (committed once approved); see **Resolve the plan parent**.
- **Session (explicit request only):** when the user asks for an ephemeral or session-only plan (kept out of the repo), load [references/session-plan.md](references/session-plan.md) directly with no `state.tsv` or repo commit.

### Resolve the plan parent

Repository storage only. Default work root is `docs/specs`. When `AGENTS.md`/`CLAUDE.md` names another directory for plans/work folders, use it and prefix **every** `agx flow` command (`flow path`, `flow status`, `flow bind`, `flow log`) with `FLOW_WORK_ROOT=<directory>` relative to repo root ([flow-state.md](../execute/references/flow-state.md)). Create the root if absent. Never use `.claude/`, `.codex/`, or `.github/copilot/`.

---

## Decomposition Patterns

Apply in order. Structure: [plan-template.md](references/plan-template.md); walkthroughs: [worked-examples.md](references/worked-examples.md); details: [step-procedures.md](references/step-procedures.md#decomposition-pattern-details).

- **Vertical slices over horizontal layers:** thin end-to-end shippable outcomes with verifiable tasks inside (never DB/API/UI layers).
- **Dependency-first ordering:** order slices by usefulness/dependency, tasks by concrete prerequisites; translate slice edges into task edges.
- **Parallel-safe grouping:** require ALL three: **different files**, **no shared state**, **independent verification**; label group and state why.
- **Parallel lanes:** mark qualifying slices `**Lane:** yes` after **Dependencies** ([Lanes](references/chain-artifacts.md#slices-inside-planmd)); never edit an approved plan when a lane merges.
- **Model selection:** tag each task **Deep** (judgment), **Standard** (default), or **Light** (mechanical); [model-ladders.md](../routing/references/model-ladders.md) resolves tiers (**Max** is reviewer-only).
- **Bespoke workflow design:** present phase list before decomposing large cross-cutting efforts ([bespoke-workflow-design](references/step-procedures.md#bespoke-workflow-design)).
- **Test-first task sizing:** one agent turn (2–5 min): failing test, implementation, single-command verification. One-task plans are valid.

---

## The Process

After Step 0, track the selected path's [workflow steps](../routing/references/contracts/workflow-steps.md), including each Step 5 sub-step as reached (done, failed with what is missing, or waived).

### Gates

Classes follow [gates.md](../routing/references/contracts/gates.md) and [output-discipline.md](../routing/references/contracts/output-discipline.md#gate-response-shape) (process-gate yields log `waive <what>`, `why` = user's reason or `none given`).

- **Plan-document review before approval question (Step 5) [safety]:** held — revise plan or keep saved draft unapproved.
- **User's own approval of spec and plan (Step 6) [safety]:** held — only user's reply writes `approve` ([approval.md](../routing/references/contracts/approval.md)).
- **True status ("validated", "reviewed", recorded results) (Step 6) [safety]:** held — name outstanding checks instead.
- **Plan Validation Checklist [process]:** `waive checklist`; `**Result:**` reads `not run` (`execute` reviews every task at Stage 1). Never write `checklist waived` or `waive checklist` before the user declines the checklist.
- **No implementation/test code in plan (row 8) [process]:** `waive no-code` naming concrete task IDs (e.g., `task-1, task-2`, no placeholders); write the concrete code into every waived task's steps, start a new plan revision (rerunning challenge/review), record row 8 as `pass (no-code waived by user: <task ids>)`, inform the plan reviewer, and never claim no code in waived tasks.
- **Plan challenge (Step 4b) [process]:** on the first decline, name the challenge cost once (unstated assumptions are what read as solid); on a repeated decline, yield immediately without restating the cost and log `challenge plan.md` with status `declined`.
- **Proportionality check (Step 5) [process]:** `waive proportionality`; `**Proportionality:**` reads `override by user` with real computed counts.
- **Blocking coverage item (Step 5) [process]:** `waive coverage` with item IDs in `why`, listed under `waived` on `**Coverage:**`.

**Don't use when** is ownership, not a gate: this skill plans and does not implement ("while you're in there, make the change" gets one `execute` offer while planning continues).

### End states

Report state, plan path, and what remains:

- **Forwarded or answered:** Step 0 answered briefly or forwarded (no plan, no row).
- **Slices only:** delivery slicing only (slice sections, staged plan, `create` row).
- **Saved, not offered:** pending challenge, unresolved blocker, `failed` row, spent review budget, or unwaived blocking coverage item (staged draft, rows, `pause plan.md` naming what remains).
- **Saved, stop stated:** user stated stop before execution ("plan only") (staged plan, rows, no question).
- **Awaiting approval:** question asked and answer was no, "not yet", or none (staged plan; on no, `approve` `declined`).
- **Approved:** user approved (committed plan with approval and `order` rows, route to `execute`).

### Step 0: Preflight — Confirm the Input Fits

Answer hypothetical/exploratory asks briefly, skip workflow, offer real planning in one line. Otherwise match **Don't use when** in order. A **cannot** forward writes no skeleton plan: forward once per [forwarding.md](../routing/references/forwarding.md) naming the target's open question. When the user insists on a plan after a missing-requirements forward (even asking you to assume values), ask once for the open requirements the plan would rest on without assuming them or forwarding again; only write the plan after the user replies with concrete requirements (or confirms assumptions after that ask). Details (`spec draft`, routine-tier hand-off, re-entry, [standalone coverage re-run](references/coverage-check.md#standalone-re-run)): [step-procedures.md](references/step-procedures.md#step-0-preflight-edge-cases).

### Step 1: Gather Input

Read input (`spec.md` + sibling `intent.md`, five-section intent, legacy `design.md`, or inline requirements) per [chain-artifacts.md](references/chain-artifacts.md). Record state with bare `agx flow …` ([flow-commands.md](../execute/references/flow-commands.md#commands)); run `flow bind --folder <work-folder>` once for existing flows (skip if refused on default branch/detached `HEAD`); pass `--folder <work-folder>` on every call. Apply [empirical questions](../routing/references/contracts/empirical-questions.md) before tasks needing observable answers; load [slicing.md](references/slicing.md) before creating tasks. Source/slice edge cases: [step-procedures.md](references/step-procedures.md#step-1-source-handling-and-slices).

### Step 2: Map the Codebase Context

Map exact repo-relative paths before writing tasks: **Files to create**, **Files to modify**, **Files to reference**, and **Test files** (location/framework). Unresolved paths are not plan-ready — explore further or hold the task out with a reason. Verify claims that code exists; move absent paths to Files to create and note the disagreement.

### Step 3: Identify Tasks

Use the source's defined solution; record chosen shape and rejected alternatives in `## Approaches considered` (or cite spec). Return genuine solution forks to `spec draft` before decomposing dependents. List deliverables -> group under delivery outcomes -> order dependency-first -> form parallel-safe groups -> size to one turn.

### Step 4: Write and Challenge the Plan

#### Step 4a: Write the plan

Before first write, record whether `plan.md` already exists in `flow path --folder <work-folder>` (for Step 6 `create`). Write actual plan content against [plan-template.md](references/plan-template.md), [slicing.md](references/slicing.md) (`**Outcome:**` and `**Scope:**` on every `## S<n>` slice, even slicing-only; preserve the source's exact slice dependency graph rather than serializing independent slices), and [skim-rules.md](references/skim-rules.md): padded tables, `## Risk Notes` (`>= 1` row, even for one-task plans), concrete steps/quotes ([output-discipline.md](../routing/references/contracts/output-discipline.md)), no implementation/test code (`## Code in artifacts` in [chain-artifacts.md](references/chain-artifacts.md)), opening Provenance with `**Validated:** not run` / `**Result:** not run` in Step 4a and updating Provenance to `**Result:** pass` (with date) once Step 5 passes. Task fields (`**Tier:**`, `**Packet review:**`, `**Review group:**`, `**Role:**` per [proportionality-check.md](references/proportionality-check.md#task-roles), `**Reproduction:**`) and `temp/` hygiene: [step-procedures.md](references/step-procedures.md#step-4a-task-fields-and-folder-hygiene).

#### Step 4b: Challenge the written plan

Invoke `challenge` on `plan.md` (passing settled intent/spec decisions) for delivery boundaries, missing work, dependency/parallel graph, Outcome proof, verification gaps, and proportionality. A source `challenge` row never skips this pass. Never challenge commit cadence — `execute` makes one commit per task. Resolve findings before Step 5; route requirement changes to their owner. For an unanswered round (`pending`), field (2) of `**Challenge record:**` reads `<date>, 1 round asked, Q1–Q<n> unanswered` (naming the unanswered questions with the date; never `and answered`). Unanswered blockers, [unattended runs](../routing/references/contracts/unattended.md), and five-field `**Challenge record:**`: [step-procedures.md](references/step-procedures.md#step-4b-challenge-resolutions-blockers-and-unattended). Step 5 never reopens this interview.

### Step 5: Validate the Plan

Run and report items 1–7 in exact order before any approval question:

1. **Plan Validation Checklist:** run all ten rows below; fix failures or leave unfixable rows `failed` with what they need. When any row fails, `**Validated:**` names the failed row(s) without claiming `rows 1–10 ran`, `**Result:**` records `failed`, and the run pauses.
2. **Proportionality check:** load/run [proportionality-check.md](references/proportionality-check.md); on the first report (including under override pressure), print actual computed counts and evidence for all five signals (1–5) reconciled against the task graph (no placeholders; list every fired signal; state signal 5 honestly if unestablished), write every user-environment effect unconditionally on `User-environment effects` (never "or dropped"), and offer dispositions when any fire. Rerun once after reorder/resize/slice; reject any resize dropping **Outcome proof**.
3. **Refill:** if tasks changed, re-mark lanes, refill `**Size:**` and `## Task index`, rerun affected checklist rows.
4. **Coverage check:** when `spec.md`/`intent.md` sits beside `plan.md`, run [coverage-check.md](references/coverage-check.md). Unwaived blocking items hold approval.
5. **Artifact validator:** run repo plan/provenance validator if present and record real result (or state none exists).
6. **Plan-document review:** dispatch Deep reviewer ([plan-document-reviewer-prompt.md](references/plan-document-reviewer-prompt.md), [model-ladders.md](../routing/references/model-ladders.md); Max only if user-selected), timing dispatch/return in UTC. Self-read fallback, two-run budget, resets, Minor/Info display: [step-procedures.md](references/step-procedures.md#step-5-plan-review-and-budget).
7. **Provenance block:** fill from real results ([plan-template.md](references/plan-template.md#provenance)).

### Step 6: Save and Hand Off

**Offer gate (safety-gated).** Offer execution approval ONLY when challenge is completed/declined, findings resolved/accepted, `**Result:**` is `pass` (or checklist waived), last plan-review has no open Critical/Important finding, and no blocking coverage item is unwaived. Else save draft, log `pause plan.md`, report what remains, ask no question. On stated stop ("plan only"), save and skip question. Never offer before write success or claim unrun checks.

**Save, stage, and log.** Use [session storage](references/session-plan.md) only if explicitly asked; else repository storage with provenance header (`derived-from:`), stage, and log in [save-and-stage.md](references/save-and-stage.md)'s exact row order using the exact action `review plan` (never `review plan.md`). Folder derivation, `review plan` format, slicing-only logging, tier escalation ([vocabulary.md](../routing/references/contracts/vocabulary.md)), failed-plan override: [step-procedures.md](references/step-procedures.md#step-6-storage-tier-and-hand-off).

**Project, approve, commit, and route.** Upsert into native plan/task tools ([native-plan-projection.md](../routing/references/native-plan-projection.md); when projecting into a session plan file, start its very first line with the canonical repository path before any `---` frontmatter; report unavailability). Explicitly check the approval tier **once in Step 6 against the final plan** (never earlier). Before every approval question, print plan path, `flow status` `text=<hash>` for repo plans (for session-only plans per [session-plan.md](references/session-plan.md), show absolute path without hash and create no repo state), <= 5-line summary (goal, size, tier + plain meaning, what to look at), `**Spec review:**` counts when asking combined routine approval, and artifact validator result. Ask approval per [references/approval.md](references/approval.md) and [approval.md](../routing/references/contracts/approval.md) (name waived checklist or declined challenge explicitly instead of calling plan "ready"; edits require re-review). Never invent a follow-up user turn or `approve` row when the user has not yet answered the approval question. On yes, for repo plans write approval rows (`phase=decompose`, `evidence=auto`, `--expect-blob <hash>`, `why="routine: no signal fired; <basis>"` or `"elevated: <signals>; <basis>"`) and `order` rows, stage `state.tsv`, commit together ([commit contract](../git/references/commit.md#the-commit-contract), [work-item branch](../git/references/branch.md#the-work-item-branch); session plans write/commit nothing), then route to `execute` (or `/agentics:git worktrees` first). On no/"not yet", stop with nothing implemented.

---

## Plan Validation Checklist

Run every check and record real results; never mark `[x]` on unrun checks (decline and name outstanding checks). Edge cases: [step-procedures.md](references/step-procedures.md#checklist-edge-cases).

1. **Clear acceptance criteria:** `- [ ]` items testable with a command.
2. **No circular dependencies:** task, slice, and review-group graphs acyclic.
3. **No oversized tasks:** each fits one turn (2–5 min).
4. **Explicit dependencies:** task IDs or "none"; Task index matches tasks.
5. **Independent parallel groups:** different files, no shared state, `**Lane:**` matches lane rule.
6. **Top failure modes covered:** Risk Notes has >= 3 rows (or 1/task when < 3 tasks).
7. **Exact file paths:** specific repo-relative paths (no "somewhere in src/").
8. **Specific steps, no code:** behaviors, test cases, commands + expected output, or pseudocode; only allowed code blocks; validation rules state what they reject.
9. **Exact commands and output:** inspect repo runner config first (or ask user once when absent—never invent or write generic "the test suite passes"); exact command + expected result; steps needing absent credentials/hardware name runner (user/CI).
10. **Testable goal:** one test-verifiable sentence.

## Related Skills

`intent` -> `spec` -> `decompose` + `challenge` -> `execute` -> `gate`.

## References

- **Core:** [step-procedures](references/step-procedures.md), [plan-template](references/plan-template.md), [approval](references/approval.md), [coverage-check](references/coverage-check.md), [plan-document-reviewer-prompt](references/plan-document-reviewer-prompt.md), [proportionality-check](references/proportionality-check.md), [chain-artifacts](references/chain-artifacts.md), [save-and-stage](references/save-and-stage.md), [session-plan](references/session-plan.md), [skim-rules](references/skim-rules.md), [slicing](references/slicing.md), [worked-examples](references/worked-examples.md)
- **Flow & git:** [flow-state](../execute/references/flow-state.md), [commit](../git/references/commit.md), [branch](../git/references/branch.md)
- **Contracts & adapters:** [approval](../routing/references/contracts/approval.md), [gates](../routing/references/contracts/gates.md), [output-discipline](../routing/references/contracts/output-discipline.md), [unattended](../routing/references/contracts/unattended.md), [vocabulary](../routing/references/contracts/vocabulary.md), [workflow-steps](../routing/references/contracts/workflow-steps.md), [principles-check](../routing/references/contracts/principles-check.md), [empirical-questions](../routing/references/contracts/empirical-questions.md), [forwarding](../routing/references/forwarding.md), [model-ladders](../routing/references/model-ladders.md), [native-plan-projection](../routing/references/native-plan-projection.md)
