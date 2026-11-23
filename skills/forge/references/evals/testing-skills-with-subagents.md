# Testing skills with subagents

When you want evidence that a skill works before release, use this reference.

## What this test proves

A subagent test answers two questions:

1. **Discovery.** Would a fresh agent find and choose the skill?
2. **Execution.** Once loaded, does the skill lead to the right behavior?

## Recommended loop

### 1. Define the scenario

Create a realistic prompt that should trigger the skill. Include enough context for the agent to act, not just recite
the rule.

Capture:

- the user request
- any relevant files, constraints, or environment details
- what success looks like
- which failure modes you expect to catch

### 2. Dispatch a test subagent

Give the subagent the scenario. Tell it to work as if the request were real. Do not coach it toward the answer.

A good test prompt includes:

- the scenario itself
- the expected deliverable shape
- an instruction to note which skills it chose and why
- a requirement to list tools used and major decisions made

### 3. Evaluate the output against criteria

Check the result for:

- correct skill selection
- correct use of the skill's workflow
- appropriate tool choices
- completeness and correctness of the response
- signs that the subagent guessed around missing instructions

### 4. Revise the skill

Use the misses to decide what to change.

- If the skill was never selected, fix the description or routing language
- If it was selected but applied poorly, tighten the workflow or examples
- If the output looked confused, add missing guardrails or reference material

### 5. Re-run the same scenario

Re-run it after each material revision to see whether the change improved behavior.

## Scenario design tips

- Use representative requests, not idealized summaries
- Include at least one scenario that is adjacent to the skill boundary to test false positives
- If the skill needs to hold under stress, include one scenario with realistic pressure or ambiguity

## Lightweight evaluation template

```text
Scenario: [name]
Expected skill trigger: [skill name]
Observed skill trigger: [what the subagent used]
Outcome: pass | partial | fail
Notes:
- discovery:
- execution:
- tools:
- gaps to fix:
```

## When to stop testing

Stop when:

- the skill is selected reliably on intended scenarios
- false positives are under control on neighboring scenarios
- repeated runs no longer reveal new instruction gaps

If the skill continues to drift, move up to the benchmark workflow in
[eval-benchmarking-workflow.md](eval-benchmarking-workflow.md).
