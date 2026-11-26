---
name: deepen
description: "Use when changes keep spreading across modules, a module's seams feel wrong, a new module's interface needs shaping before it is written, or people use conflicting names for the same concept. Find the underlying design problem and propose refactors grounded in the code. It writes no code: applying a decided refactor is an ordinary change."
version: "3.1.0"
user-invocable: true
disable-model-invocation: false
argument-hint: "[codebase area, module being designed, or term to pin down]"
metadata:
    surfaces: [claude-code, copilot-cli, vscode, codex, antigravity]
---

# Deepen

Design **deep modules**: a lot of behaviour behind a small interface, across the module lifecycle. Place seams and size interfaces as you write code. In existing code, surface architectural friction with **deepening refactors** that turn shallow modules into deep ones. The aim is testability and AI-navigability.

Deepen **proposes**. It does not apply a refactor, write a module, or write a decision record, and it never reports the work as done, merged, or landed. Its deliverables are a candidate list, a settled-design summary, glossary entries, and decisions handed to `docs` to record.

Skill commands here use the `/agentics:<skill>` form that Claude Code, Copilot CLI, and VS Code Chat accept. On Codex,
type `$agentics:<skill>` instead.

## When to use

- You need to place the seam, size the interface, or design for testability before a module's code exists
- Module seams are unclear and changes ripple across many files
- Tests are hard to write because dependencies are tightly coupled
- A "small" feature touches an unreasonable number of unrelated files
- The mental model needed to navigate the codebase has grown faster than the code
- A term means different things to different people, or a design decision needs judging for a decision record

## Don't use when

Read the rows from the top; the first match wins. The **Kind** column says what an explicit invocation does with the row, as [forwarding.md](../routing/references/forwarding.md#explicit-invocation) defines it: a `cannot` row forwards, and a `better elsewhere` row is named in one line and the run continues here. When the model loaded this skill on its own, every row forwards.

| If the task is…                                                                                     | Use this instead                          | Kind             | Why                                                                                                                |
| --------------------------------------------------------------------------------------------------- | ----------------------------------------- | ---------------- | ------------------------------------------------------------------------------------------------------------------ |
| Implementing a refactor whose design is decided: writing the module, moving callers, deleting files | `routing`, as an ordinary mutating change | cannot           | Deepen writes no code. `routing` picks the delivery path for the change.                                           |
| Breaking approved work into ordered tasks                                                           | `decompose`                               | cannot           | Deepen writes no task plan.                                                                                        |
| Throwaway code to answer a question you can only feel by running it                                 | `prototype`                               | cannot           | A prototype learns something fast and is deleted. Deepen designs code meant to survive and runs nothing.           |
| A review of one pull request for bugs or security                                                   | `review`                                  | cannot           | A different scope and outcome.                                                                                     |
| Open exploration of what to build, with no module, area, or term in view                            | `intent`                                  | better elsewhere | Intent shapes a new direction. Under `/agentics:deepen`, treat it as a bare goal: ask for the area or a repo scan. |
| Writing a decision record the user already wants written                                            | `docs`                                    | better elsewhere | `docs` writes records. Under `/agentics:deepen`, judge the decision and hand it to `docs`, as Step 3 says.         |

## Glossary

Use these terms consistently in every candidate, question, and summary: **module**, **interface**, **implementation**, **depth** (deep/shallow), **seam**, **adapter**, **leverage**, and **locality**. In that output, never use "component," "service," "API," or "boundary"; the user's own words in a request are not corrected. Definitions, principles (the deletion test, "the interface is the test surface," one adapter = hypothetical seam / two = real) live in [language.md](references/language.md); a worked shallow-vs-deep example is in [deepening.md](references/deepening.md#recognizing-shallow-modules).

Domain language names good seams. Decision records hold decisions not to re-litigate.

## Process

**Shared contracts.** Ask every decision question in the `❓ **Q<n>** - **<title>**:` format with a `➡️` recommendation line ([question format](../routing/references/contracts/question-format.md)). Each gate below names its class from [gates.md](../routing/references/contracts/gates.md). Deepen has no safety gate of its own: every gate here is a process gate (name its cost once; yield on the next reply that still declines, in any wording). Because the flow helper has no phase for deepen, record any waiver as one line in the settled-design summary or pause recap: `Waived by the user: <gate>` with the user's reason, or `none given` (ask for no reason). Steps named in the request are authorized when their turn comes ([approval.md](../routing/references/contracts/approval.md#steps-named-up-front)). A reply handing a decision question back ("pick one", "I don't care", "you choose") answers it: take the ➡️ recommendation, say which, and never re-ask. A pre-question "wherever" or "any is fine" before options are shown answers nothing. In an [unattended](../routing/references/contracts/unattended.md) run, the dispatcher answers questions.

### 0. Preflight: pick the path

Read the rows from the top; first match wins. The last row catches everything else.

| The request                                                                                                                                                        | Path and next step                                                                                                                                                                                                                                                                                                                                           |
| ------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Matches a `## Don't use when` row                                                                                                                                  | Forward per [forwarding.md](../routing/references/forwarding.md#target-classification): carry input, forward once, no starter question. For `better elsewhere` under explicit invocation, name the other skill in one line and continue at the matching row below. A forward to `prototype` is an offer: ask one yes/no question naming what it builds/runs. |
| Carries a recap from an earlier pause of this skill                                                                                                                | Resume at [Step 3](#3-grilling-loop) from the recap; settled points stay settled, and grill every open point listed in `Open:` rather than treating it as already decided.                                                                                                                                                                                   |
| Reports trouble in a settled design, or asks whether to scrap it                                                                                                   | [The scrap trigger](#the-scrap-trigger), which judges whether the trouble is a pattern. Reports can come from the user, `execute`, or `diagnose`.                                                                                                                                                                                                            |
| A term to pin down, glossary clash, or decision to judge for a record, with no refactor in view                                                                    | **Language path.** Skip Steps 1–2. Read the glossary resolved by [repo-conventions.md](../routing/references/repo-conventions.md) first (via the context map in a multi-context repo). Work language in [Step 3](#3-grilling-loop) with its side-effect rules only. Never manufacture an exploration.                                                        |
| A module not written yet, or a seam to sharpen while writing it                                                                                                    | **Design-time path.** Skip Steps 1–2. The candidate is the module you shape. Read the glossary and relevant decision records first (via the context map if multi-context), then open [Step 3](#3-grilling-loop). Apply the deletion test _forward_: if you cannot name the complexity it concentrates, it may not be needed.                                 |
| Names an existing code area, or a diff/change to inspect before a refactor, including a forward from `diagnose` after repeated failed fixes or a missing test seam | **Refactor path.** Continue at [Step 1](#1-explore) on that area. For a diff or change, read the diff itself, not only its description, before judging it.                                                                                                                                                                                                   |
| Names an area that does not exist or cannot be located                                                                                                             | Ask which module or directory to explore as a decision question, basing ➡️ on the request's wording or active conversation area. Wait; never explore a guessed area.                                                                                                                                                                                         |
| Otherwise: a bare goal with no area, such as "make this cleaner"                                                                                                   | Ask one `❓ **Q1** - **Area**:` decision question with two options—name the area or scan the whole repo—with a `➡️` recommendation line from repo size and goal wording. Process gate: cost is spending exploration budget on code the user may not care about. If the next reply still names no area, take the recommendation and say which.                |

On the refactor path and at the scrap trigger, apply the [principles check](../routing/references/contracts/principles-check.md) when there is code to inspect.

### 1. Explore

First read the project's domain glossary and any decision records in the area you touch, as [repo-conventions.md](../routing/references/repo-conventions.md) resolves them. A root context map means **multiple contexts**: route the topic to the right context first per [context-map.md](references/context-map.md) so you do not edit the wrong context's glossary.

Then dispatch one read-only exploration subagent via this session's path in [surface-dispatch.md](../routing/references/surface-dispatch.md) (it starts no subagents of its own; where no dispatch path exists, explore in-session). Explore organically for friction—bouncing across tiny modules, shallow wrappers, pure functions extracted without **locality**, leaky seams, or untestable interfaces (full checklist in [deepening.md](references/deepening.md#exploration-friction-signals)).

Apply the **deletion test** ([language.md](references/language.md)) to each suspect: if deleting it makes its complexity vanish, it is a pass-through deepening candidate; if that complexity reappears across callers, it is already deep and not a candidate, however large.

### 2. Present candidates

**No candidates?** If no suspect failed the deletion test, say plainly: _"this area doesn't show shallow-module friction right now."_ Don't manufacture a marginal candidate; offer a different area and stop unless the user names one.

Otherwise, present a numbered list of deepening opportunities, each with **Files**, **Problem**, **Solution** (plain English), and **Benefits** (locality, leverage, testability). Use glossary terms for the domain and [language.md](references/language.md) terms for architecture.

**Decision-record conflicts.** Surface a candidate that contradicts a decision record only when friction warrants a revisit, marked clearly (e.g., _"contradicts ADR-0007, but worth reopening because..."_). Don't list every forbidden refactor.

Then ask, as a decision question with a ➡️ recommendation, which candidate to explore. Propose no interface before the pick.

**Candidate pick (process gate).** If the user skips it ("just tell me the fix"), name its cost once: the first interface that comes to mind gets locked in without comparison, and the user gives up their choice. On the next skip, take the recommended candidate, say which, record `Waived by the user: Candidate pick`, and continue at Step 3 in the same turn.

A reply stating an already-chosen candidate or interface direction is a pick, not a skip: take it into Step 3, note the user-supplied direction in the summary, and start grilling its design constraints in the same turn. Naming a candidate by number or subject—even while continuing straight into a term or design point—is also a pick: take it, say which, and start grilling in that same turn alongside any glossary question.

### 3. Grilling loop

Once a candidate is picked (or on the design-time path), grill constraints, dependencies, module shape, what sits behind the seam, and which tests survive. When the interface is still on paper, apply Designing-for-testability rules in [language.md](references/language.md). The **language path** carries no design tree: apply only the side-effect rules below and its own done rule.

- **Classify each dependency** using [deepening.md](references/deepening.md) (in-process, local-substitutable, remote-but-owned, true-external) to pick the seam and adapter strategy.
- **Recommend, then follow the user.** Principles in [language.md](references/language.md) (including one-adapter = hypothetical seam) are recommendations. State the recommendation and its cost once; name a forecast plainly when offered as a second adapter (planned is not existing). If the user still chooses otherwise, design what they chose and note it in the summary.

**Side effects.** Record each as its decision settles, not in a batch at the end:

- **Glossary writes.** Write terms ([context-format.md](references/context-format.md)) to the glossary resolved by [repo-conventions.md](../routing/references/repo-conventions.md). On the language path when the user asked to pin down/record a term: if overloaded/ambiguous with no user pick yet, ask one `❓` sharpening question + `➡️` recommendation, and once resolved (or if already clear/settled), write the concrete one-sentence entry (marked to confirm) directly into the resolved glossary (`GLOSSARY.md` / `CONTEXT.md`) without asking permission first and report the written entry. Otherwise ask once on the first settled term (naming path and creation); a yes covers the run. Create the default glossary only when none resolves. On a glossary conflict from `repo-conventions.md`, ask its conflict question first and write nothing until answered, then write immediately upon answer.
- **Sharpen fuzzy terms.** Don't accept the first word offered. For a glossary conflict, ask a decision question recommending consistency; for an overloaded term, propose a canonical term and concrete disambiguating scenarios, surfacing code/user contradictions ([grilling examples](references/deepening.md#grilling-examples)).
- **Decision records.** Judge decisions against the three-part test in [adr-format.md](references/adr-format.md) and hand passing ones to `docs` (`docs` never requires deepen first). Offer a record only when a decision passes (e.g., a load-bearing rejection): _"Record this as an ADR so future architecture reviews don't re-suggest it? (via `/agentics:docs build`)"_. On yes, hand `docs` the decision, reason, and resolved directory (context's own directory in a multi-context repo; root holds system-wide decisions per [context-map.md](references/context-map.md)). When the user asks for records directly, state each decision's three-part test result in one line and hand every requested record to `docs` (`build` mode) with no yes/no question first ([forwarding.md](../routing/references/forwarding.md#the-route-is-already-settled)); name any failing decision's cost once.
- **Design it twice.** Per [interface-design.md](references/interface-design.md), before settling any non-trivial interface (2+ callers, or expensive to change: public interface, persisted format, published schema), dispatch **2 to 4 parallel sub-agents** with distinct constraints to produce structurally distinct whole-shape candidates. Fall back to [surface-dispatch.md](../routing/references/surface-dispatch.md#any-other-surface-generic-fallback) (`self-read (not independent)`) only when the host environment cannot dispatch sub-agents, and still complete the comparison in the same turn. Screen every candidate against all four [design-red-flags.md](references/design-red-flags.md) flags (shallow module, information leakage, temporal decomposition, pass-through method), and compare survivors on depth before presenting the settled design (or record the excluded alternative if only one survives). A user-chosen port takes the same test. **Process gate:** if declined, name its cost once; on the next decline, settle the single shape and record the waiver.

**Request to pause early?** ("let's pick this up tomorrow") is a pause, not a skip. Don't push to finish. Print a recap with **Settled**, **Open**, and **Recorded** (glossary writes and records handed to `docs`), plus any waiver lines. State plainly that the design is not finalized, nothing goes to planning, recorded items stay as-is, and `/agentics:deepen` with this recap resumes grilling. Write no file unless asked.

**Done when** all hold:

1. Module shape, what sits behind the seam, and surviving tests are settled, and design-it-twice (or its recorded waiver) has run for any non-trivial interface.
2. Side effects above are recorded.
3. You wrote a concrete settled-design summary: interface, seam, adapters, surviving tests, alternatives considered, and any user-supplied direction or waiver (omit resolved open-question caveats once confirmed).

Then ask one yes/no question: _"Turn this design into a task plan (via `/agentics:decompose`)?"_ (If the user already named planning, announce the hand-off instead.) On yes, run `/agentics:decompose` with the summary as inline requirements (`derived-from: none`). On no, the summary is the deliverable and the run ends. For more candidates, loop to Step 2.

**Language-path done when** requested language work is recorded (settled glossary terms written and qualifying decisions handed to `docs`). No module design or planning question applies.

### The scrap trigger

A settled design can still be wrong; the signal is a **pattern of friction, not single hard cases**. When implementation (`execute`, or reported back) repeatedly hits the same shape of trouble—repeated workarounds across callers, multiple special-case edge branches, type escape hatches, callers knowing internals, or 2+ similar implementation deviations (full tells and redesign checklist in [deepening.md](references/deepening.md#the-scrap-trigger))—the design is the problem. Add no more fixes.

If the report shows no pattern (a few edge cases or inherent data complexity), say so, name what would show one, and end. When the trigger fires: re-explore built code (Step 1), redesign with implementation lessons as day-one constraints, subtract before adding, and complete the full Step 3 design-it-twice comparison across at least two structurally distinct smaller shapes (screening each against the four red flags and comparing survivors on depth). Name the scrapped shape and reason in the new settled-design summary; a load-bearing rejection may merit an ADR.

## References

- [language.md](references/language.md): vocabulary, principles, and design-for-testability rules (single source of truth).
- [deepening.md](references/deepening.md): dependency categories, seam/test strategy, shallow-module example, friction signals, grilling examples, and scrap-trigger checklist.
- [interface-design.md](references/interface-design.md): "Design It Twice" parallel sub-agent pattern for alternative interfaces.
- [design-red-flags.md](references/design-red-flags.md): the four shape red flags screening candidates before synthesis.
- [approach-exploration.md](references/approach-exploration.md): genuine-fork test and structurally distinct candidate discipline for `spec draft` (`decompose` routes new forks back to `spec draft`).
- [context-format.md](references/context-format.md): domain glossary format.
- [context-map.md](references/context-map.md): multi-context map structure and topic routing.
- [adr-format.md](references/adr-format.md): decision-record template and three-part test (also read by `docs`).
- [forwarding.md](../routing/references/forwarding.md): route-away protocol and explicit invocation.
- [approval.md](../routing/references/contracts/approval.md), [gates.md](../routing/references/contracts/gates.md), [unattended.md](../routing/references/contracts/unattended.md), [question-format.md](../routing/references/contracts/question-format.md): shared contracts.
