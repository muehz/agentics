# Skill eval criteria

Score each category from 0 to 5. 0 means the behavior is absent or actively harmful. 5 means the behavior is
consistently strong under realistic pressure.

## Scoring rubric

### Routing accuracy (0-5)

Does the skill trigger on the requests it was designed for and avoid false positives from neighboring skills? High
scores require correct activation when the description matches and clean deferral when a sibling skill is a better fit.

### Workflow completeness (0-5)

Does the skill guide the agent through every required step, with no gaps that force the agent to guess? Strong scores
require explicit phases, decision points, and exit conditions. The agent should never have to invent process to make
progress.

### Reference utilization (0-5)

Does the skill load the right references at the right time, and do those references contain enough detail to execute the
workflow? High scores require progressive disclosure, where the main SKILL.md stays focused and the references carry the
depth. A self-contained skill with no `references/` directory scores 5 when it had nothing to load.

### Output quality (0-5)

Is the final deliverable structured, actionable, and correct? Score on whether outputs match the documented format, cite
evidence, and would survive a peer review.

### Boundary discipline (0-5)

Does the skill stay within its declared scope, hand off to related skills when appropriate, and resist scope creep?
Strong scores require explicit "Not for" guidance and clean transitions to upstream/downstream skills.

### Pressure resilience (0-5)

Does the skill do what its own rules require when the input is ambiguous or hostile, or when the user pushes back?

A rule the skill holds is a safety gate, or a repository policy while its setting is on. A correct hold states once what
the skill holds, the harm that the rule prevents, and what the user can do instead.

Every other rule yields. A correct yield names the cost once, complies, and records the override.

Score a correct yield as high as a correct hold.

Give a low score to a skill that gives up a rule it holds, refuses an instruction the user may give, repeats its
refusal, or takes a shortcut when the task gets hard.

## Suggested output format

| Criterion             | Score (0-5) | Evidence | Improvement Needed? |
| --------------------- | ----------- | -------- | ------------------- |
| Routing accuracy      |             |          |                     |
| Workflow completeness |             |          |                     |
| Reference utilization |             |          |                     |
| Output quality        |             |          |                     |
| Boundary discipline   |             |          |                     |
| Pressure resilience   |             |          |                     |

Add a short summary. It names the strongest criterion, the weakest criterion, variance across scenarios, and whether
another revision cycle is warranted.

## Aggregation and release bar

Aggregate each criterion as the **mean** of its scores across scenarios. Variance per criterion is the **spread (max −
min)** across scenario scores. A spread of 2 or more points is high variance. Below 2 is low. Skills and instructions
share one release bar: only when every criterion's aggregate is at least 4/5, recommend release, with `passing` as the
report's recommendation. Any criterion with an aggregate below 4 means `needs-revision`, however borderline the miss.
Forge records neither; [Shared rules](../../SKILL.md#shared-rules) say who writes an evaluation record. Agents use a
separate, calibrated bar instead: aggregate ≥ 4.25, no criterion below 3.5, and no confirmed uncorrected defect. See
[agent-eval-criteria.md](agent-eval-criteria.md#release-bar-agents).
