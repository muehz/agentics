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

Skill commands here use the `/agentics:<skill>` form that Claude Code, Copilot CLI, and VS Code Chat accept. On Codex,
type `$agentics:<skill>` instead.

## Overview

Turns concrete requirements into delivery slices where needed, then a Markdown plan of ordered, testable tasks that `execute` consumes directly. Every task is independently verifiable with acceptance criteria, explicit dependencies, independent parallel groups, and deterministic, machine-parsable language (exact paths, function names, constants, tables, lists).

**Announce once, after Step 0.** When Step 0 accepts the input, state in one line what this run will do (write a plan, update one slice, write slices only, re-check coverage, or ask for approval of an existing plan) and name any steps authorized up front ([approval.md](../routing/references/contracts/approval.md#steps-named-up-front)). Runs that answer briefly or forward at Step 0 print no announce line.

Apply the [principles check](../routing/references/contracts/principles-check.md) when planning from an artifact or inspecting the relevant diff.

## Don't use when

Read in order; first match wins. Kinds follow [forwarding.md](../routing/references/forwarding.md#explicit-invocation): a **cannot** row forwards by [forwarding.md](../routing/references/forwarding.md) under any invocation; a **better elsewhere** row under explicit invocation gets one line naming the other skill and continues here.

1. **Hypothetical or exploratory ask** ("roughly", "ballpark", "not asking you to plan it") — _handled here_: answer briefly with no workflow (Step 0).
2. **No intent, spec, legacy design, or concrete inline requirements** (problem, outcome, or scope open) — _cannot_: `intent` (tasks cannot rest on unstated requirements).
3. **Six-section intent whose `intent.md` approval is not current in `flow status`** — _cannot_: `intent` (direction must be approved first).
4. **Spec with no current approval and no recorded `**Spec review:**`, open policy conflict, or no header** — _cannot_: `spec`, resumed at approval (`spec` owns review, approval, header repair).
5. **Approved `brief.md`** (one-task middle-path change) — _cannot_: `execute` (the brief replaces spec and plan).
6. **Bug or failure fix whose root cause is unproven** — _cannot_: `diagnose` (do not plan around hypotheses; proven fixes proceed from `diagnose`).
7. **Request to implement a plan with a current approval** — _cannot_: `execute` (this skill plans; approval settles the route).
8. **Request to verify finished work** — _cannot_: `gate`.
9. **Backlog story splitting for sprint refinement** with no intent, spec, design, or inline requirements — _cannot_: no skill; say so and help directly if asked.
10. **One action with nothing to sequence** — _better elsewhere_: no skill; do it directly (avoids unnecessary review/approval overhead).
11. **Six-section intent with current approval and no approved spec** — _handled here_: run `spec draft` first, then continue here (Step 0).
12. **Otherwise** (approved/routine-reviewed spec, in-flight intent, legacy design, inline requirements) — _handled here_: stay here (a slice issue limits the update to its slice).

## Plan Storage

Default to **repository storage** in the work folder ([references/chain-artifacts.md](references/chain-artifacts.md)); ask no storage question. Repository instructions set only the work root, never storage mode.

- **Repository (default):** `<plan-parent>/YYYY-MM/<purpose>-<topic>/plan.md` (committed once approved); see **Resolve the plan parent**.
- **Session (explicit request only):** ephemeral host session plan file, not committed; see [references/session-plan.md](references/session-plan.md).

### Resolve the plan parent

This runs for repository storage only. The work root is `docs/specs`. When `AGENTS.md` or `CLAUDE.md` names another directory for plans or work folders, use that directory instead and prefix every `flow` call with `FLOW_WORK_ROOT=<directory>`, relative to the repository root. A shell call keeps no variables from the one before, so every call needs the prefix ([flow-state.md](../execute/references/flow-state.md)). Create the root when it does not exist. Never create a work root in `.claude/`, `.codex/`, or `.github/copilot/`. They hold harness configuration.

---

## Decomposition Patterns

Apply in order. Structure: [references/plan-template.md](references/plan-template.md); walkthroughs: [references/worked-examples.md](references/worked-examples.md); slice examples, parallel-lane lifecycle, and bespoke workflow design: [references/step-procedures.md](references/step-procedures.md#decomposition-pattern-details).

- **Vertical slices over horizontal layers.** Thin end-to-end outcomes that ship separately; bounded, verifiable tasks inside them (never database/API/UI layer deliverables).
- **Dependency-first ordering.** Order slices by usefulness and dependency, then tasks by concrete prerequisites; put shared work on the first outcome's path and translate slice edges into enforcing task edges.
- **Parallel-safe grouping.** Require ALL three: **different files**, **no shared state**, **independent verification**. Label each group and state why its tasks are independent.
- **Parallel lanes.** Mark qualifying slices `**Lane:** yes` after **Dependencies** per **Lanes** in [references/chain-artifacts.md](references/chain-artifacts.md#slices-inside-planmd); never edit an approved plan when a lane merges.
- **Model selection for subagents.** Tag each task **Deep** (judgment about _what_), **Standard** (default execution), or **Light** (mechanical, cheap-to-verify); [model-ladders.md](../routing/references/model-ladders.md) resolves tiers and pins (**Max** is a reviewer rung only).
- **Bespoke workflow design.** When no existing workflow sequences a large cross-cutting effort, design and present the phase list before decomposing tasks ([references/step-procedures.md](references/step-procedures.md#bespoke-workflow-design)).
- **Test-first task sizing.** One agent turn (2–5 min): failing test, implementation, single-command verification, one reason to fail. One-task plans are valid.

---

## The Process

After Step 0, track the selected path's [workflow steps](../routing/references/contracts/workflow-steps.md), including each Step 5 sub-step as reached; record each as done, failed with what is missing, or waived.

### Gates

Classes follow [gates.md](../routing/references/contracts/gates.md). Hold a **safety** gate once in its fixed shape (held item, harm prevented, alternative); point repeated demands back in one line. A **process** gate names its cost once, yields on the next decline in any wording ("just do it" declines it), and logs one `waive <what>` row (`why` = user's reason or `none given`).

- **Plan-document review before approval question (Step 5) [safety]:** held — revise the plan or keep the saved draft unapproved.
- **User's own approval of spec and plan (Step 6) [safety]:** held — only the user's reply writes an `approve` row ([approval.md](../routing/references/contracts/approval.md)).
- **True status ("validated", "reviewed", recorded results) (Step 6) [safety]:** held — name the outstanding checks instead.
- **Plan Validation Checklist [process]:** `waive checklist`; `**Result:**` reads `not run`, so `execute` reviews every task at Stage 1.
- **No implementation or test code in the plan (row 8) [process]:** `waive no-code` naming the **actual concrete task IDs** (e.g., `task-1, task-2`, never `<all task ids>`); adding code changes reviewed scope (new plan revision, rerunning challenge/review), row 8 records the waiver with those IDs, the plan reviewer is informed, and never claim the review found no code in those waived tasks.
- **Plan challenge (Step 4b) [process]:** `challenge plan.md` row with status `declined`, logged **immediately** on the turn the user declines after the one-time cost warning (never wait for a third turn).
- **Proportionality check (Step 5) [process]:** `waive proportionality`; `**Proportionality:**` reads `override by user` with the real computed counts (never placeholder tokens like `<real counts>`).
- **Blocking coverage item (Step 5) [process]:** `waive coverage` with item IDs in `why`, listed under `waived` on `**Coverage:**`.

**Don't use when** is ownership, not a gate: this skill plans and does not implement ("while you're in there, just make the change" gets one `execute` offer while planning continues).

### End states

Every run ends in one state; report the state, plan path, and what remains:

- **Forwarded or answered:** Step 0 answered briefly or forwarded (no plan, no row).
- **Slices only:** user asked only for delivery slicing (slice sections, staged plan, `create` row).
- **Saved, not offered:** pending challenge, unresolved blocker, `failed` row, spent review budget, or unwaived blocking coverage item (staged draft, rows, `pause plan.md` row naming what remains).
- **Saved, stop stated:** user stated a stop before execution ("plan only") (staged plan, rows, no question).
- **Awaiting approval:** question asked and answer was no, "not yet", or none (staged plan; on no, `approve` row with `declined`).
- **Approved:** user approved (committed plan with approval and `order` rows, route to `execute`).

### Step 0: Preflight — Confirm the Input Fits

Answer hypothetical/exploratory asks briefly, skip the workflow, and offer a real planning pass in one line. Otherwise read **Don't use when** in order. A **cannot** forward writes no skeleton plan: forward once per [forwarding.md](../routing/references/forwarding.md) naming the target's open question. Read [references/step-procedures.md](references/step-procedures.md#step-0-preflight-edge-cases) for user-insistence fallback, input readiness (`spec draft`), routine-tier spec hand-off criteria, and re-entry on an unapproved plan or [standalone coverage re-run](references/coverage-check.md#standalone-re-run).

### Step 1: Gather Input

Read the input (`spec.md` + sibling `intent.md`, in-flight five-section intent, legacy `design.md`, or inline requirements) per [references/chain-artifacts.md](references/chain-artifacts.md). Identify goal, architecture, constraints, and existing code.

Record state with bare `agx flow …` ([flow-state.md](../execute/references/flow-commands.md#commands)). When resolving a flow `decompose` did not create, run `flow bind --folder <work-folder>` once (skip if refused on default branch or detached `HEAD`); pass `--folder <work-folder>` on every `flow` call.

Apply [empirical questions](../routing/references/contracts/empirical-questions.md) before planning any task depending on an observable answer, and load [references/slicing.md](references/slicing.md) before creating tasks. Read [references/step-procedures.md](references/step-procedures.md#step-1-source-handling-and-slices) for missing named inputs, source decisions, pending intent challenges, external sources, and updating existing or selected slices.

### Step 2: Map the Codebase Context

Explore and map exact repository-relative paths before writing tasks: **Files to create**, **Files to modify**, **Files to reference**, and **Test files** (location and framework). Unresolved paths are not plan-ready — explore further or hold the task out with a reason. Verify every design claim that code already exists; if absent, move it to Files to create and note the disagreement.

### Step 3: Identify Tasks

Use the source's defined solution; record chosen shape and rejected alternatives in `## Approaches considered` (or cite the spec). Return genuine solution forks to `spec draft` before decomposing dependent tasks. Then: (1) list deliverables, (2) group under Step 1's delivery outcomes, (3) order dependency-first, (4) form parallel-safe groups, (5) size each task to one agent turn.

### Step 4: Write and Challenge the Plan

#### Step 4a: Write the plan

Before the first write, record whether `plan.md` already exists in the directory `flow path --folder <work-folder>` prints (Step 6 needs this for `create`). Load and write the actual plan content against [references/plan-template.md](references/plan-template.md), [references/slicing.md](references/slicing.md) (every `## S<n>` slice carries `**Outcome:**` and `**Scope:**`, even in slicing-only runs), and [references/skim-rules.md](references/skim-rules.md) with padded tables, concrete steps/quotes (never narrative summaries in place of plan content), no implementation or test code (`## Code in artifacts` in [references/chain-artifacts.md](references/chain-artifacts.md)), and `**Validated:** not run` / `**Result:** not run`. Read [references/step-procedures.md](references/step-procedures.md#step-4a-task-fields-and-folder-hygiene) for `**Tier:**`, `**Packet review:**`, `**Review group:**`, `**Role:**` ([proportionality-check.md](references/proportionality-check.md#task-roles)), `**Reproduction:**`, and folder hygiene (`temp/`).

#### Step 4b: Challenge the written plan

Invoke `challenge` on `plan.md` (passing settled intent/spec decisions) to examine delivery boundaries, missing work, dependency/parallel graph, Outcome proof, verification gaps, and proportionality. A source `challenge` row never skips this pass. Never challenge commit cadence — `execute` makes one commit per task ([execute](../execute/SKILL.md)). Resolve findings before Step 5; send requirement changes to their owner. Read [references/step-procedures.md](references/step-procedures.md#step-4b-challenge-resolutions-blockers-and-unattended) for unanswered blockers, [unattended runs](../routing/references/contracts/unattended.md), and the five-field `**Challenge record:**`. Step 5 never reopens this interview.

### Step 5: Validate the Plan

Run and report items 1 to 7 in this exact order before any approval question (once each unless noted):

1. **Plan Validation Checklist.** Run all ten rows below; fix failures or leave unfixable rows `failed` with what they need.
2. **Proportionality check.** Load and run [references/proportionality-check.md](references/proportionality-check.md); on the very first proportionality report (including under override pressure), print the **actual computed count and evidence for all five signals** reconciled against the computed task graph (never placeholders like `N support tasks` or `<real counts>`, and if signal 5 is absent/unestablished state that status honestly rather than claiming all five passed), write every user-environment effect (such as browser authentication) unconditionally on `User-environment effects` (never "or dropped"), and offer dispositions when any fire. Rerun once after a reorder/resize/slice; reject any resize dropping the **Outcome proof**.
3. **Refill.** If tasks changed, re-mark lanes, refill `**Size:**` and `## Task index`, and rerun checklist rows on changed tasks.
4. **Coverage check.** When `spec.md` or `intent.md` sits beside `plan.md`, run [references/coverage-check.md](references/coverage-check.md). Unwaived blocking items hold approval.
5. **Artifact validator.** Run the repo's plan-artifact / provenance-header validator if present and record its real result (or state none exists).
6. **Plan-document review.** Dispatch a reviewer with [references/plan-document-reviewer-prompt.md](references/plan-document-reviewer-prompt.md) at Deep per [model-ladders.md](../routing/references/model-ladders.md) (or Max only if user-selected), timing dispatch and return in UTC. Read [references/step-procedures.md](references/step-procedures.md#step-5-plan-review-and-budget) for self-read fallback, the two-run per-revision budget, revision resets, and Minor/Informational presentation.
7. **Provenance block.** Fill from real results per [references/plan-template.md](references/plan-template.md#provenance).

### Step 6: Save and Hand Off

**Offer gate (safety-gated).** Offer execution approval ONLY when the challenge is completed/declined, findings are resolved/accepted, `**Result:**` is `pass` (or checklist waived), the last plan-review run has no open Critical/Important finding, and no coverage item is blocking and unwaived. Otherwise save the draft, log `pause plan.md`, report what remains, and ask no question. If the user stated a stop ("plan only"), save and skip the question. Never offer execution before write success or claim "validated"/"reviewed"/"ready to execute" without real evidence.

**Save, stage, and log.** Save to [session storage](references/session-plan.md) only if explicitly asked; otherwise save to repository storage with provenance header (`derived-from:`), stage, and log strictly per [references/save-and-stage.md](references/save-and-stage.md). Read [references/step-procedures.md](references/step-procedures.md#step-6-storage-tier-and-hand-off) for folder derivation, `review plan` evidence format, slicing-only logging, tier escalation (plain meaning in [vocabulary.md](../routing/references/contracts/vocabulary.md)), and failed-plan override handling.

**Project, approve, commit, and route.** Upsert the canonical plan into native plan/task tools per [native-plan-projection.md](../routing/references/native-plan-projection.md) (report unavailability rather than claiming success). Before every approval question, print the plan path, the `flow status` `text=<hash>`, a summary of at most five lines (goal, size, tier + plain meaning, what to look at), and the artifact validator result. Ask the approval question per [references/approval.md](references/approval.md) and [approval.md](../routing/references/contracts/approval.md) (when the checklist was waived or the plan challenge was declined, name that explicitly instead of calling the plan "ready"; edits require re-review before re-asking). On yes, write approval and `order` rows, stage `state.tsv`, and commit together under the [commit contract](../git/references/commit.md#the-commit-contract) on the [work-item branch](../git/references/branch.md#the-work-item-branch), then route to [execute](../execute/SKILL.md) (or `/agentics:git worktrees` first if isolation is needed). On no/"not yet", stop with nothing implemented.

---

## Plan Validation Checklist

Run every check before hand-off and record real results; never mark `[x]` on unrun checks even if asked (decline and name outstanding checks). Edge cases (legacy plans, waived row 8, unreachable services on row 9): [references/step-procedures.md](references/step-procedures.md#checklist-edge-cases).

1. **Clear acceptance criteria:** each task has `- [ ]` items testable with a command.
2. **No circular dependencies:** task graph, slice dependencies, and review-group graph have no cycles.
3. **No oversized tasks:** each task fits one agent turn (2–5 min).
4. **Explicit dependencies:** every task lists dependencies by task ID or "none"; Task index rows match their tasks.
5. **Independent parallel groups:** tasks in a group touch different files, share no state, and `**Lane:**` matches the lane rule.
6. **Top failure modes covered:** Risk Notes table has >= 3 rows (or 1 per task when < 3 tasks).
7. **Exact file paths:** every path is specific and repository-relative (no "somewhere in src/").
8. **Specific steps, no code:** steps name behaviors, test cases, commands + expected output, or pseudocode; only allowed code blocks; validation rules state what they reject.
9. **Exact commands and output:** inspect the repo's runner config first (or ask the user once for the test command when no runner config is available—never invent a command or write generic phrases like "the test command passes" or "the test suite passes"); every verification step names that exact command and its expected result, and steps needing absent credentials/hardware name the runner (user or CI).
10. **Testable goal:** one sentence a test could verify.

## Related Skills

`intent` -> `spec` -> `decompose` + `challenge` -> `execute` -> `gate`.

## References

- [references/step-procedures.md](references/step-procedures.md) — Steps 0–6 procedures, bespoke workflow design, lanes, task fields, review budget, tier escalation, checklist edge cases.
- [references/plan-template.md](references/plan-template.md) — plan structure, task fields, risk notes, Provenance rules.
- [references/approval.md](references/approval.md) — Step 6 approval questions and logged rows.
- [references/coverage-check.md](references/coverage-check.md) — Step 5 cross-artifact coverage check.
- [references/plan-document-reviewer-prompt.md](references/plan-document-reviewer-prompt.md) — plan reviewer inputs, criteria, return contract.
- [references/proportionality-check.md](references/proportionality-check.md) — two-sided scope check.
- [references/chain-artifacts.md](references/chain-artifacts.md) — work-folder and provenance-header rules.
- [references/save-and-stage.md](references/save-and-stage.md) — staging order and shell snippets.
- [references/session-plan.md](references/session-plan.md) — explicit session-storage plan format.
- [references/skim-rules.md](references/skim-rules.md) — card, task index, folded steps, Provenance split.
- [references/slicing.md](references/slicing.md) — delivery boundaries as `plan.md` sections.
- [references/worked-examples.md](references/worked-examples.md) — five end-to-end runs.
- [flow-state.md](../execute/references/flow-state.md) — `state.tsv` and `agx flow` helper.
- [approval.md](../routing/references/contracts/approval.md), [gates.md](../routing/references/contracts/gates.md), [unattended.md](../routing/references/contracts/unattended.md), [vocabulary.md](../routing/references/contracts/vocabulary.md), [workflow-steps.md](../routing/references/contracts/workflow-steps.md), [principles-check.md](../routing/references/contracts/principles-check.md), [empirical-questions.md](../routing/references/contracts/empirical-questions.md) — shared contracts.
- [forwarding.md](../routing/references/forwarding.md), [model-ladders.md](../routing/references/model-ladders.md), [native-plan-projection.md](../routing/references/native-plan-projection.md) — routing adapters.
- [commit.md](../git/references/commit.md), [branch.md](../git/references/branch.md) — commit and work-item branch rules.
- [execute](../execute/SKILL.md) — plan execution and commit cadence.
