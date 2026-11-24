# Skill creator

**By default, skill creation is test-driven development applied to process documentation.**

A skill is not finished when the markdown reads well. It is finished when representative subagents can find the skill,
apply it correctly, and survive realistic evaluation.

Test-first is the default order: record how an agent behaves without the skill before you write it. Choosing another
order is a process gate: name the evidence cost once, and when the next reply still declines, follow the user's order
and record the yield as
[section B of the hardening checklist](../quality/skill-hardening-checklist.md#b-pressure-resilience--hold-the-boundary-under-adversarial-input)
describes. The workflow below drafts the skill in steps 1 to 3 and tests it in steps 4 and 5. When you work test-first,
run a step 4 scenario without the skill before step 2.

**Background:**

- `tdd` defines the RED -> GREEN -> REFACTOR cycle that test-first skill work follows.
- `execute` is recommended when you run review or evaluation loops with dedicated subagents.

## When to create or update a skill

**Create or invest further when:**

- The technique is reusable across repos or teams
- Agents routinely miss the same workflow, guardrail, or decision point
- A reusable reference beats the same explanation repeated in every session
- A current skill exists, but routing, clarity, or evaluation results show that it underperforms

**Do not create a new skill for:**

- One-off project notes
- Repository-specific conventions that belong in instructions or README files
- Rules that should be automated with linting or validation instead of prose

A skill meant to be found by `spec align` follows the
[policy-skill contract](../../../spec/references/policy-skill-contract.md).

## Authoring workflow

### 0. Intake

Before you draft, establish these points:

- what the skill enables
- when it should trigger
- the output format
- whether the output can be checked objectively enough for test cases

When the conversation already describes the workflow, extract its tools, step order, corrections, and formats first. Ask
about the gaps the conversation leaves open in one ❓ round. Give each question a ➡️ recommendation inferred from the
conversation. Skip any topic that the conversation already answers. Then confirm the resulting scope with the user. Do
not repeat questions the conversation has answered. When the session needs a broader review of which lessons belong in
reusable assets, use [Forge reflect mode](../../SKILL.md#reflect-mode).

### 1. Define the trigger and discovery story

Start with the `description:` field, the primary discovery mechanism. Draft the trigger language. Then pressure-test
whether an AI router would choose the skill.

**Put the keyword anchors in the description, not in the body.**

A host registers only the frontmatter and loads the body after a trigger match. A keyword below the frontmatter cannot
cause the match. Spend the description's characters on error messages, symptom phrases, tool names, and verb cues an
agent would type. Pick anchors bound tightly to the skill's job (`merge conflict`, `failing CI`, `requirements drift`).
Avoid generic terms ("good", "fix").

When you need help to write or tighten the description, load
[../quality/skill-description-optimizer.md](../quality/skill-description-optimizer.md). Load
[../quality/cso-guide.md](../quality/cso-guide.md) for the keyword-coverage checklist.

Use [../quality/trigger-eval.md](../quality/trigger-eval.md) to measure discovery with a frozen, versioned query set
against the full skill roster. Use development evidence for description revisions. Reserve holdout evidence for the
final report. Trigger results do not stamp behavioral eval status.

### 2. Write the minimal durable skill

Author the smallest instruction set that can teach the behavior. Keep the main SKILL focused on these items:

- the job-to-be-done
- the routing boundary
- the core workflow
- the most important rules
- the most common failure modes

Put large examples, rubrics, and edge-case catalogs in reference docs so they load only when needed.

Load [../quality/best-practices.md](../quality/best-practices.md) for structure, routing guidance, front matter
conventions, and cross-platform authoring patterns.

### 3. Make the wording persuasive enough to hold under pressure

A skill should be easy to follow, hard to misread, and resistant to avoidable rationalization. Clear language,
well-placed examples, and explicit guardrails matter more than clever prose.

When you need to strengthen instruction quality and not bloat the skill, load
[../quality/persuasion-principles.md](../quality/persuasion-principles.md).

By default, explain the reason for an instruction. Reserve rationalization tables and red-flag lists for safety gates
that a measured adversarial run broke. Target them at the failure that the run demonstrated. A process gate still yields
under the
[three gate classes](../quality/skill-hardening-checklist.md#b-pressure-resilience--hold-the-boundary-under-adversarial-input)
and does not gain authority from louder wording. Apply this rule before the
[loophole-closing techniques](../quality/advanced-patterns.md#bulletproofing-skills-against-rationalization).

### 4. Test the skill with subagents

Before you trust the skill, watch an agent use it on a realistic scenario. Capture misses. Revise the instructions.
Re-run the scenario until the behavior stabilizes.

Load [../evals/testing-skills-with-subagents.md](../evals/testing-skills-with-subagents.md) to design scenarios,
dispatch evaluation subagents, and turn failures into revisions.

For a structured eval with scores across multiple scenarios, use `forge eval <skill-name>`. It dispatches the
skill-eval-runner-prompt template and applies the skill-eval-criteria rubric automatically.

### 5. Benchmark the skill before release

Go beyond one happy-path test. Define representative scenarios. Score the outputs. Compare revisions. Stop only when the
gains flatten out.

When you need a repeatable evaluation loop and scoring rubric, load
[../evals/eval-benchmarking-workflow.md](../evals/eval-benchmarking-workflow.md).

The `forge eval <skill-name>` mode implements this loop with a 6-dimension rubric
([../evals/skill-eval-criteria.md](../evals/skill-eval-criteria.md)) and variance analysis to identify fragile vs.
consistently weak behaviors.

## SKILL.md structural rules

Every `SKILL.md` follows these rules, whether or not you run the full workflow above. Each rule names the conformance
rule that checks it.

| Rule                 | Value                                                                                                          | Checked by                                                                                                       |
| -------------------- | -------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------- |
| Size                 | At most 24,000 bytes for `SKILL.md` (30,000 for other `skills/` files). Move deep material into `references/`. | `skill-file-size`                                                                                                |
| Description          | At most 1,024 characters.                                                                                      | `skill-description-length`                                                                                       |
| Name                 | Lowercase kebab-case, at most 64 characters, equal to the skill's directory name.                              | `skill-name-format`                                                                                              |
| Frontmatter contract | The keys and values in [Required frontmatter](#required-frontmatter).                                          | `skill-frontmatter`, `skill-frontmatter-key`, `skill-frontmatter-value`, `skill-metadata`, `skill-argument-hint` |
| Routing section      | A `## Don't use when` section.                                                                                 | `skill-routing-section`                                                                                          |
| Reference index      | Exactly one `## References`, and it is the last level-two heading.                                             | `skill-references-index`                                                                                         |
| Restating block      | No `Capability summary` heading.                                                                               | `skill-summary-block`                                                                                            |
| Duplicate block      | No paragraph or list item that repeats body text verbatim.                                                     | `skill-duplicate-block`                                                                                          |

Every `SKILL.md` is capped at 24,000 bytes (Antigravity's `SKILL.md` body load cap), and every other file under
`skills/` (references, scripts, and any other tracked file) is capped at 30,000 bytes (`skill-file-size`), leaving
headroom for line-number/tool framing under Codex's 40,000-byte single-read cutoff, as the
[size limit](../quality/cso-guide.md) explains. When a step's body moves to a reference file to fit the limit, the step
keeps its heading and a one-line pointer to where the text went.

These are targets, which no script checks:

- The description states what the skill does and when to use it ("Use when…").
- The description is about 25 words. This is a compression target, not a limit.
- No block restates the body in other words; the verbatim copies are the only ones a rule reports.
- The workflow opens with the Step 0 preflight described under
  [Hardening](#hardening-workflow-completeness-and-pressure-resilience).
- Instructions use the imperative mood.
- A question that offers a next step describes the step in plain English first and names the invocation in parentheses
  at the end, with the leading slash and only the skill and mode, never argument placeholders, as the
  [handoff question item](../quality/skill-hardening-checklist.md#a-workflow-completeness-close-the-gaps-that-force-guessing)
  shows.

### Required frontmatter

```yaml
---
name: webapp-testing
description: >
    Use when asked to verify frontend functionality, debug UI behavior, capture browser screenshots, or check for visual
    regressions.
version: "1.0.0"
metadata:
    surfaces: [claude-code, copilot-cli, vscode]
---
```

The evaluation record is kept apart from the skill file, as [Evaluation record](#3-evaluation-record) describes.

| Key                 | Required                                             | Value                                                                                             | Checked by                                                    |
| ------------------- | ---------------------------------------------------- | ------------------------------------------------------------------------------------------------- | ------------------------------------------------------------- |
| `name`              | Yes                                                  | The name rule above, unique among this repository's `SKILL.md` files                              | `skill-frontmatter`, `skill-name-format`, `skill-name-unique` |
| `description`       | Yes                                                  | The description rule above                                                                        | `skill-frontmatter`, `skill-description-length`               |
| `version`           | Yes                                                  | Semantic version; the rule checks only that the key is present                                    | `skill-frontmatter`                                           |
| `argument-hint`     | When the skill declares modes, mode aliases or flags | A non-empty string that holds each declared mode, mode alias and flag as a whole word             | `skill-argument-hint`, `skill-frontmatter-value`              |
| `allowed-tools`     | No                                                   | A string or list of `Name` or `Name(pattern)` entries, with at least one entry                    | `skill-frontmatter-value`                                     |
| `context`           | No                                                   | Exactly `fork`                                                                                    | `skill-frontmatter-value`                                     |
| `metadata.surfaces` | Yes                                                  | A non-empty list of shipped surfaces (`claude-code`, `codex`, `copilot-cli`, `vscode`), each once | `skill-metadata`                                              |

A top-level key must be one that at least one of the skill's listed surfaces documents (`skill-frontmatter-key`).

### Layout, registration, anti-patterns

```text
skills/<skill-name>/
├── SKILL.md              # Required — entry point
├── references/           # Context docs loaded on demand
├── scripts/              # Executable automation
└── templates/            # Starter code
```

Register the skill where the repository registers skills. Follow the repository's own convention (its README,
`AGENTS.md`, or manifest). Add nothing that the repository does not use. A shell command that is the same in Bash and
PowerShell is written once; a second block appears only where the syntax differs. Avoid descriptions without trigger
phrases (they won't be discovered) and hardcoded platform-specific paths.

## Required SKILL.md conventions

This section gives the form of the sections that the [structural rules](#skillmd-structural-rules) require, and of the
evaluation record. Only what names a conformance rule is checked; the recommended forms are targets, which no script
checks. Text that several skills share, such as the decision-question format and the gate contract, lives in one shared
file, and a skill links it in one line instead of pasting it.

### 1. `## Don't use when`

The section must exist (`skill-routing-section`). The rest is the recommended form, a target: place the section before
any mode-selection table or deep workflow content, and format it as a 3-column table:
`| Input signal | Use this instead | Why |`. Each row redirects a specific kind of task to a sibling skill with a
one-line rationale. Put every redirect in this section, so routing-time agents find it on first scan. A section or label
named "Not for" is not used.

Illustrative example:

```markdown
## Don't use when

| Input signal                                                     | Use this instead            | Why                                                                          |
| ---------------------------------------------------------------- | --------------------------- | ---------------------------------------------------------------------------- |
| Exactly 1 unresolved review thread on a PR (no bot/check signal) | `/agentics:review feedback` | Triage's worktree/orchestration overhead isn't justified for a single thread |
| Code-review concerns on a brand-new PR with no prior signals     | `/agentics:review`          | Triage is reactive (responds to posted signal); review is proactive          |
| A single complex test/build failure needing root-cause work      | `/agentics:diagnose`        | Triage classifies and routes; diagnose drills into one failure               |
```

A skill with only one neighbor still benefits: a 1-row redirect helps routing.

### 2. `## References` index

The skill has exactly one `## References`, and it is the last level-two heading (`skill-references-index`). The rest is
the recommended form, a target: list each reference file with a one-line purpose, so a maintainer need not open each one
to pick the right reference. When the skill uses both its own references and other skills' references, group them as
"Core references" and "Cross-skill references". A skill with one reference may also link it in the workflow step that
uses it.

The index is the skill's only one. An entry-point table that only names modes and links to their sections may stay; a
`Capability summary` does not (`skill-summary-block`), and no other block restates the body (a target).

Illustrative example:

```markdown
## References

Reference files in [`references/`](../../../deepen/references/):

- [`language.md`](../../../deepen/references/language.md) — shared vocabulary for naming modules, interfaces, seams,
  adapters, depth, leverage, and locality.
- [`deepening.md`](../../../deepen/references/deepening.md) — how to deepen a cluster of shallow modules safely,
  classified by dependency category.
- [`interface-design.md`](../../../deepen/references/interface-design.md) — "Design It Twice" parallel sub-agent pattern
  for exploring alternative interfaces.

Cross-skill references used during the grilling loop:

- [`context-format.md`](../../../deepen/references/context-format.md) — format for domain glossary updates.
- [`adr-format.md`](../../../deepen/references/adr-format.md) — ADR template used when a candidate is rejected with a
  load-bearing reason.
```

### 3. Evaluation record

The evaluation record exists only where the repository defines one. There, it is kept apart from the skill file, keyed
by skill name, where the repository's own convention keeps it. A run can then be recorded with no edit to the skill. No
eval key sits in frontmatter: `eval-frontmatter-clean` rejects one. A `passing` record carries the digest of the
evaluated content, computed with the repository's own digest tool over the skill, its references and its pinned bank, so
a check can reject a claim that outlived that content. If the skill changes without a new evaluation, set its status to
`not-run` and keep its earlier date and digest. `forge eval` reports the scorecard in place of a stamp.

## Hardening: workflow completeness and pressure resilience

Beyond the required conventions, skills systematically fail two criteria: **workflow completeness** and **pressure
resilience** (fleet-eval evidence in
[../quality/skill-hardening-checklist.md](../quality/skill-hardening-checklist.md)). A skill that scores
`needs-revision` almost always has a gap on one of these two axes.

Two conditional requirements close them:

- **Deferral + preflight (workflow completeness).** A skill with routing neighbors must do both of these things:
    1. Carry the `## Don't use when` table.
    2. Enforce it as a **Step 0 preflight** in the workflow.

    In that preflight, the skill must do these things:
    - Check the input against the table before it runs.
    - Route out on a mismatch instead of execution on the wrong input.
    - Define the abort output explicitly (mismatch line + one starter question + handoff, not an empty deliverable).

- **Pressure guardrails (pressure resilience).** A skill that takes ambiguous, adversarial, or authority-laden input
  must follow these rules:
    - State why its boundaries matter.
    - Link the shared gate contract, and state each gate once with one of the
      [three gate classes](../quality/skill-hardening-checklist.md#b-pressure-resilience--hold-the-boundary-under-adversarial-input):
      safety gate, repository policy, or process gate.
    - Never assert a status it did not itself produce.
    - Accept and act on a plain statement from the user about their own organisation, ownership, intent, plans or
      history. When evidence in the session contradicts it, say once what was observed, then follow the user. A
      statement never overrides a safety gate.

Apply both **conditionally**. A pure reference or lookup skill needs neither. When you author or revise a skill, do
these steps:

- Work the full [../quality/skill-hardening-checklist.md](../quality/skill-hardening-checklist.md).
- Before you claim `passing`, re-run `/agentics:forge eval`.

## TDD mapping for skill work

| TDD concept            | Skill work                                                         |
| ---------------------- | ------------------------------------------------------------------ |
| Failing test           | Baseline scenario that reveals a routing or behavior gap           |
| Minimal implementation | Smallest skill or revision that addresses the failure              |
| Green test             | Subagent now finds and applies the skill correctly                 |
| Refactor               | Tighten wording, examples, and references with no loss of behavior |
| Regression suite       | Benchmark scenarios that keep future edits honest                  |

## Skill design heuristics

### Prefer reusable guidance over session narratives

A skill should tell future agents what to do, when to do it, and how to recognize it. It should not retell one past
debugging story.

### Evaluate discoverability separately from correctness

A technically correct skill can still fail if the description is vague or overlaps heavily with neighboring skills.
Treat routing quality as its own test stream.

## Evaluation and release criteria

A skill is ready when:

- The description reliably triggers on the intended scenarios
- Subagents can apply the skill with no guesses about missing context
- Evaluation results are consistently strong across representative scenarios
- Remaining weaknesses are minor enough that another iteration would have low payoff

If those conditions are not true, follow [../evals/skill-improvement-loop.md](../evals/skill-improvement-loop.md):

1. Revise the skill.
2. Re-run the same scenarios.
3. Stop when every criterion meets the [release bar in Forge eval mode](../../SKILL.md#eval-mode) or returns diminish.

## Governance integration

When the skill has compliance, catalog, or audit implications, use the existing governance references:

- [../governance/compliance-governance.md](../governance/compliance-governance.md) for compliance governance guardrails,
  requirements, and release checks
- [../governance/compliance-audit-procedures.md](../governance/compliance-audit-procedures.md) for audit workflow and
  scoring
- [../governance/governance-details.md](../governance/governance-details.md) for detailed governance and refactor
  guidance

## References

### Core references

- [../quality/trigger-eval.md](../quality/trigger-eval.md). Frozen discovery queries, provider campaigns, and separate
  development and holdout reporting.
- [../quality/skill-description-optimizer.md](../quality/skill-description-optimizer.md). Description-writing patterns,
  anti-patterns, and discoverability checks.
- [../quality/best-practices.md](../quality/best-practices.md). Front matter, routing, reference organization, and
  cross-platform authoring guidance.
- [../quality/persuasion-principles.md](../quality/persuasion-principles.md). How to make instructions clearer, more
  actionable, and more resilient.
- [../quality/skill-hardening-checklist.md](../quality/skill-hardening-checklist.md). Conditional checklist for the two
  axes skills most often fail: workflow completeness and pressure resilience.
- [../quality/adversarial-pair-pattern.md](../quality/adversarial-pair-pattern.md). Wrapper-plus-extremist-finder
  contract (leash, burden of proof, capability bound, wrapper audit, rerun budget) for destructive or judgment-heavy
  sweeps.
- [../evals/testing-skills-with-subagents.md](../evals/testing-skills-with-subagents.md). Subagent-driven evaluation
  workflow for skill testing.
- [../evals/skill-improvement-loop.md](../evals/skill-improvement-loop.md). Revise-and-re-run loop that stops at the
  [release bar in Forge eval mode](../../SKILL.md#eval-mode).
- [../evals/eval-benchmarking-workflow.md](../evals/eval-benchmarking-workflow.md). Scenario design, scoring rubric, and
  iteration loop for skill benchmarks.

### Extended references

- [../quality/advanced-patterns.md](../quality/advanced-patterns.md). Deeper testing patterns, anti-patterns, and
  loophole-closing tactics.
- [../quality/cso-guide.md](../quality/cso-guide.md). Detailed search optimization and naming guidance.
- The three governance references under [Governance integration](#governance-integration).

## Creator family

All `forge` authoring workflows, for agents, skills, and instructions, follow the same eval-driven pattern. The pattern
is: draft → test with subagents → score against rubric → iterate until quality gate passes.
