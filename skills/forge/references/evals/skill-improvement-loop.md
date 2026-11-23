# Skill improvement loop

Run this loop to improve a skill after initial authoring or a `needs-revision` eval, when the user asked for the
revision ([Eval mode](../../SKILL.md#eval-mode) step 7).

1. **Retain the control before any revision.** Capture a repository-shaped snapshot that holds
   `skills/<skill-name>/SKILL.md` and its owned references. Copy the files with a filesystem copy method. Verify the
   copy against the originals, for example with a hash comparison. When no copy method is available, stop and ask the
   user. Never recreate the files from displayed content. Keep this snapshot immutable for the loop. For a new skill,
   select a `without_skill` control instead. This step is step 1 of the linked benchmarking workflow for an existing
   skill. Historical scorecards cannot replace a fresh paired control.
2. **Freeze the scenario set**: the pinned bank, or the four scenarios that [Eval mode](../../SKILL.md#eval-mode) step 3
   designs when no bank exists, so score deltas stay comparable. When available, reuse the same scenarios from the eval
   run that flagged the skill. Do not add a scenario, even a regression scenario, to a frozen loop. A new scenario
   requires a new baseline and a new bank version.
3. **Establish the behavioral gap.** Use retained eval evidence when available, or run the unchanged skill in fresh
   sessions with [skill-eval-runner-prompt.md](skill-eval-runner-prompt.md) and grade each run with
   [grader-prompt.md](grader-prompt.md). For a new skill, observe the absence control before you author the skill. Score
   with [skill-eval-criteria.md](skill-eval-criteria.md), counting only scorecards that passed the
   [independent semantic review](README.md#independent-semantic-review). Identify the targeted assertions whose failures
   the revision must fix. Preserve the rubric, harness, inputs, and shared facts for the paired comparison.
4. **Link the retained report before you choose a revision.** Consume available human feedback alongside evaluator
   findings. A human or evaluator reads full runner evidence, including transcripts, for repeated improvisation and
   wasted steps. Keep full scenario-rich reports on the human or evaluator side. Connect the observations to the
   criteria with the lowest scores. Prefer to delete a misleading instruction rather than add another one. When two or
   more runners independently wrote the same helper, consolidate it under the skill's `scripts/` directory. Later runs
   can reuse it from there. For workflow-completeness or pressure-resilience gaps, work through
   [../quality/skill-hardening-checklist.md](../quality/skill-hardening-checklist.md).
5. **Revise the skill to target the weak areas**, with no bloat in unrelated sections. A fresh subagent makes the
   revision from the extracted behavioral gaps and relevant feedback, never from the frozen bank, trigger queries, or
   holdout. When the session that ran the bank revises in place, label the rerun `reviser saw the bank`. Keep the
   snapshot and frozen inputs unchanged, also when you change owned references. The revised files are written under the
   asset-write rule in [Shared rules](../../SKILL.md#shared-rules).
6. **Cross-check revised rules against the skills they name** before the rerun, as [Eval mode](../../SKILL.md#eval-mode)
   step 7 defines. The check covers each skill the rules name and each other skill with the same trigger. Confirm that
   no added or changed rule delays, narrows, or contradicts when such a skill starts. Fix each conflict. Record the
   check in the before/after report.
7. **Run both configurations in fresh sessions** on the same scenarios and selected provider/model combinations. Pair
   the current skill with the retained old content, or with absence for a new skill. A current-only run cannot establish
   improvement. Use one fresh subject run per user turn, scenario and configuration, with explicit identities and the
   same runner template, and a separate grader run for each. For absence, override the runner's target-read instruction
   and prohibit any indirect load of current target content or owned references. Keep descriptive scenario names. Do not
   automatically retry failures or overwrite retained evidence.
8. **Separate completeness, quality, and improvement.** Complete controls with low scores are valid comparison evidence.
   Check current against the unchanged rubric. Then inspect the retained assertions. Every targeted assertion must pass
   current. At least one control failure must become a pass per selected provider. No safety or process gate may
   regress. Assertion ties do not establish improvement. A table of paired rubric and cost deltas does not establish
   paired assertion improvement either. A positive cost delta means more time or tokens. Report criterion before/after
   deltas and assertion evidence separately before you recommend `passing`.
9. **Stop after at most two focused revisions** against one frozen bank, whatever the scores, as
   [Eval mode](../../SKILL.md#eval-mode) step 7 sets. Then report: recommend release when every criterion meets the bar,
   and otherwise name the remaining gaps and recommend a re-scope, a new bank version, or a backlog item. Stop sooner
   when a focused revision produces no meaningful gain. See
   [eval-benchmarking-workflow.md](eval-benchmarking-workflow.md#when-to-stop).

## Practical guidance

- Fix routing and workflow-completeness gaps before you polish tone or structure
- Prefer small, testable revisions over wholesale rewrites
- When the cap is reached below the bar, reconsider the skill's scope, structure, or reference layout before a new loop
