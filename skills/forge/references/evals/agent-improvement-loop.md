# Agent improvement loop

Run this loop to improve an agent after initial authoring, when the user asked for the revision
([Eval mode](../../SKILL.md#eval-mode) step 7).

1. **Fix the scenario set**: the pinned bank, or the four scenarios that [Eval mode](../../SKILL.md#eval-mode) step 3
   designs when no bank exists. Keep it frozen for the loop.
2. **Run the eval against the current agent instructions**, one fresh subject run per user turn of each scenario with
   [agent-eval-runner-prompt.md](agent-eval-runner-prompt.md) and a separate grader run with
   [grader-prompt.md](grader-prompt.md).
3. **Score the results with the rubric** in [agent-eval-criteria.md](agent-eval-criteria.md), counting only scorecards
   that passed the [independent semantic review](README.md#independent-semantic-review).
4. **Identify the lowest-scoring criteria.** Determine which instruction gaps caused them.
5. **Revise the agent instructions to target the weak areas.** Do not bloat unrelated sections. A fresh subagent revises
   from the extracted gaps, never from the scenarios themselves, and the revised file is written under the asset-write
   rule in [Shared rules](../../SKILL.md#shared-rules).
6. **Cross-check revised rules against the skills they name** before the rerun, as [Eval mode](../../SKILL.md#eval-mode)
   step 7 defines. Confirm that no added or changed rule delays, narrows, or contradicts when a named skill starts.
   Confirm the same for another skill with the same trigger. Fix each conflict. Record the check in the before/after
   report.
7. **Re-run the same scenarios** to verify improvement. A regression found during the loop goes into the next bank
   version, not into this frozen set.
8. **Stop when the agent meets the calibrated agent release bar, or after two focused revisions**, whichever comes
   first, as [Eval mode](../../SKILL.md#eval-mode) step 7 sets. The bar has three conditions:
    - The cross-scenario aggregate must be ≥ 4.25.
    - No criterion aggregate may fall below 3.5.
    - No confirmed uncorrected defect may remain.

    See [agent-eval-criteria.md](agent-eval-criteria.md#release-bar-agents). When the cap is reached below the bar, name
    the remaining gaps and recommend a re-scope, a new bank version, or a backlog item.

## Practical guidance

- Fix routing and boundary errors before you polish tone or structure
- Prefer small, testable revisions over wholesale rewrites
- Keep a short changelog of which edits were intended to improve which rubric criteria
- When the cap is reached below the bar, reconsider the agent pattern, scope, or required references before a new loop
