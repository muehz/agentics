# Instruction eval criteria

Score each category from 0 to 5. 0 means the instruction actively fails the scenario. 5 means it performs reliably
across realistic use cases.

## Scoring rubric

### Clarity (0-5)

Are the instructions unambiguous and easy to follow, even when the user scenario is only partially specified?

### Completeness (0-5)

Do the instructions cover the necessary scenarios, expected behaviors, and edge cases with no critical gaps?

### Platform compatibility (0-5)

Do the instructions work across CLI, VS Code, and cloud agents, with platform-specific examples or caveats where needed?

### Conflict detection (0-5)

Do the instructions avoid contradictions with other instruction files in the same scope? Do they explain precedence or
handoff rules when overlap exists?

### Scoping precision (0-5)

Are the instructions appropriately scoped so they are neither too broad nor too narrow for the intended `applyTo`
pattern and repository area?

### Actionability (0-5)

Do the instructions provide concrete guidance that a model can follow directly instead of abstract principles with no
operational behavior?

## Suggested output format

| Criterion              | Score (0-5) | Evidence | Improvement Needed? |
| ---------------------- | ----------- | -------- | ------------------- |
| Clarity                |             |          |                     |
| Completeness           |             |          |                     |
| Platform compatibility |             |          |                     |
| Conflict detection     |             |          |                     |
| Scoping precision      |             |          |                     |
| Actionability          |             |          |                     |

Add a short summary that names the strongest criterion, weakest criterion, and whether another revision cycle is
required.
