# Eval benchmarking workflow

When one-off spot checks are not enough to show, repeatably, whether a skill revision is better, use this workflow.

## What a skill eval is

A skill eval is a structured test where a subject subagent attempts to use the skill on a representative scenario. A
separate grader scores the subject's output against predefined criteria.

## Eval cycle

1. **Write the skill, or keep the current version unchanged as the control**
2. **Define representative test scenarios**: the pinned bank, or the four that [Eval mode](../../SKILL.md#eval-mode)
   step 3 designs
3. **Record the baseline eval**
4. **Score the results**
5. **Revise the skill**
6. **Cross-check revised rules against the skills they name** before the rerun, as [Eval mode](../../SKILL.md#eval-mode)
   step 7 defines
7. **Re-run the same scenarios**

Keep the scenarios stable long enough to compare revisions fairly.

## Account for a full skill cycle

Before you revise the skill, start with a baseline against the frozen bank. Process one skill at a time. Give every
attempt a fresh session. Account for every attempt, including failed ones, and state the dispatch count before the first
one.

Do not treat a retry as automatic. Do not add resume, stamping, commits, or gratuitous reruns to the campaign. Keep
provider lanes separate when you score them.

## Scoring criteria and rubric

Score each scenario with the canonical 0-5 rubric for the asset type:

- [skill-eval-criteria.md](skill-eval-criteria.md) for skills
- [agent-eval-criteria.md](agent-eval-criteria.md) for agents
- [instruction-eval-criteria.md](instruction-eval-criteria.md) for instructions

Those files also define aggregation, variance, and the release bar. Record both the numeric score and a short note about
why.

## Benchmark worksheet

```text
Scenario: [name]
Revision: [commit or version]

[One line per criterion from the canonical rubric]
[Criterion]: [0-5] - [note]

Overall summary:
Decision: keep revising | ready for release
```

## When to stop

Stop at the first of these:

- The benchmark reaches the release bar.
- Two focused revisions have run against the frozen bank, whatever the scores ([Eval mode](../../SKILL.md#eval-mode)
  step 7).
- A focused revision produces no meaningful score increase across the critical categories, and the remaining issues are
  cosmetic or scenario-specific rather than systemic.

At that point, report, then re-scope, write a new bank version, or move the open issues into a later improvement
backlog. A further loop starts only when the user asks for it.
