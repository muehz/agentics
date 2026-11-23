# Instruction improvement loop

Run this loop to improve an instruction file after initial authoring, when the user asked for the revision
([Eval mode](../../SKILL.md#eval-mode) step 7).

1. **Fix the scenario set**: the pinned bank, or the four scenarios that [Eval mode](../../SKILL.md#eval-mode) step 3
   designs when no bank exists, covering overlapping scopes and platform-specific behavior among them. Keep it frozen
   for the loop.
2. **Run the eval against the current instruction file**, one fresh subject run per user turn of each scenario with
   [instruction-eval-runner-prompt.md](instruction-eval-runner-prompt.md) and a separate grader run with
   [grader-prompt.md](grader-prompt.md).
3. **Score the results with the rubric** in [instruction-eval-criteria.md](instruction-eval-criteria.md), counting only
   scorecards that passed the [independent semantic review](README.md#independent-semantic-review).
4. **Check for conflicts against every other instruction file in scope.** Document any precedence or overlap issues.
5. **Verify platform compatibility.** A shell command that is the same in Bash and PowerShell appears once; a second
   form appears only where the syntax differs. Confirm that CLI, VS Code, and cloud expectations are coherent.
6. **Identify the lowest-scoring criteria.** Determine which wording, scope, or structure problems caused them.
7. **Revise the instruction file to target the weak areas.** Do not broaden scope unnecessarily. A fresh subagent
   revises from the extracted gaps, never from the scenarios themselves, and the revised file is written under the
   asset-write rule in [Shared rules](../../SKILL.md#shared-rules).
8. **Cross-check revised rules against the skills they name** before the rerun, as [Eval mode](../../SKILL.md#eval-mode)
   step 7 defines. Confirm that no added or changed rule delays, narrows, or contradicts when a named skill starts.
   Confirm the same for another skill with the same trigger. Fix each conflict. Record the check in the before/after
   report.
9. **Re-run the same scenarios** to verify improvement. A regression found during the loop goes into the next bank
   version, not into this frozen set.
10. **Stop when every criterion is at least 4/5, or after two focused revisions**, whichever comes first, as
    [Eval mode](../../SKILL.md#eval-mode) step 7 sets. When the cap is reached below the bar, name the remaining gaps
    and recommend a re-scope, a new bank version, or a backlog item.

## Practical guidance

- Resolve conflicts and scoping errors before you polish tone
- Prefer concrete requirement language over abstract principles
- Treat platform drift as a test failure, not a documentation nit
- If repeated evals still fail because the scope is too broad, split the instruction into smaller scoped files
