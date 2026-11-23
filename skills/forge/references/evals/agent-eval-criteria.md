# Agent eval criteria

Score each category from 0 to 5. 0 means the behavior is absent or actively harmful. 5 means the behavior is
consistently strong under realistic pressure.

## Scoring rubric

### Routing accuracy (0-5)

Does the agent correctly identify when to handle the request itself versus route to another agent, skill, or workflow?
High scores require accurate routing on both obvious and ambiguous requests.

### Skill discipline (0-5)

Does the agent scan for applicable skills before any response or action? Does it invoke and announce what applies, and
follow the invoked skill rather than improvise from memory? High scores require the scan to happen before exploration or
clarifying questions. They also require that, under pressure to skip the scan, the agent names the applicable skill and
invokes it once anyway, with no degraded half-version.

### Persona consistency (0-5)

Does the agent maintain its intended role, tone, and decision style throughout the interaction, including when the
scenario becomes confusing or adversarial?

### Tool-use guidance (0-5)

Does the agent provide clear guidance on which tools to use, when to use them, and when not to use them? Strong scores
require concrete, situational tool guidance rather than generic recommendations.

### Boundary clarity (0-5)

Are the agent's responsibilities, handoff points, and limitations clearly defined? High scores require the agent to
resist scope creep and explain transitions cleanly.

### Instruction completeness (0-5)

Do the instructions cover common edge cases, ambiguous requests, and realistic failure modes? Strong scores require the
agent to behave predictably without reliance on unstated assumptions.

### Cross-reference accuracy (0-5)

Are references to other agents, skills, and supporting workflows correct and current? High scores require references
that resolve cleanly and support the intended behavior.

## Suggested output format

| Criterion                | Score (0-5) | Evidence | Improvement Needed? |
| ------------------------ | ----------- | -------- | ------------------- |
| Routing accuracy         |             |          |                     |
| Skill discipline         |             |          |                     |
| Persona consistency      |             |          |                     |
| Tool-use guidance        |             |          |                     |
| Boundary clarity         |             |          |                     |
| Instruction completeness |             |          |                     |
| Cross-reference accuracy |             |          |                     |

Add a short summary that names the strongest criterion, the weakest criterion, and whether another revision cycle is
warranted.

## Release Bar (agents)

Agents use a calibrated release bar instead of the shared every-criterion-≥4 rule. The bar dates from 2026-07-07, after
a prior agent's four-round eval. Recommend release when all of these hold:

- the cross-scenario **aggregate is ≥ 4.25**
- **no criterion aggregate falls below 3.5**
- **no confirmed, uncorrected defect remains**

A defect is a concrete behavior that the charter text would produce wrongly. A judge's refinement suggestion is not a
defect. Any confirmed defect means revise regardless of scores.

The bar exists because judges score agent charters on edge-case completeness, which has no upper limit. Each round
clears the prior findings and surfaces more marginal cases, so per-axis means level off near 4.0 on judge-generated
hypotheticals, not real defects. A charter with no confirmed wrong behavior should not iterate indefinitely against that
ceiling. Fix defects before release. Move refinements to the backlog.
