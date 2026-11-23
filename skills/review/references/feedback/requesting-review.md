# Requesting code review

Dispatch a dedicated reviewer agent to catch issues before later work builds on them.

## When to request review

**Mandatory:**

- After each review group in `execute`'s review-gated workflows
- After completing a major feature
- Before merge to `main`

**Optional but valuable:**

- When stuck (fresh perspective)
- Before refactoring (baseline check)
- After fixing a complex bug

## How to request

**1. Get git SHAs:**

```bash
BASE_SHA=$(git rev-parse HEAD~1)  # the last commit; for the whole branch use $(git merge-base origin/main HEAD)
HEAD_SHA=$(git rev-parse HEAD)
```

**2. Dispatch a reviewer agent:**

Dispatch a dedicated reviewer through this session's native dispatch path, with the template in
[`../interactive/interactive-review.md`](../interactive/interactive-review.md).
[Surface dispatch](../../../routing/references/surface-dispatch.md) names the built-in review agent type on surfaces
that provide one, and the general-purpose target everywhere else.

**Placeholders:**

- `{WHAT_WAS_IMPLEMENTED}`: what you just built
- `{PLAN_OR_REQUIREMENTS}`: what it should do
- `{BASE_SHA}`: starting commit
- `{HEAD_SHA}`: ending commit
- `{DESCRIPTION}`: brief summary

**3. Act on feedback:**

- Fix Critical issues immediately
- Fix Important issues before proceeding
- Note Minor issues for later
- If the reviewer is wrong, push back with technical reasoning

## Example

```text
[Just completed Task 2: Add verification function]

You: Let me request code review before proceeding.

BASE_SHA=$(git log --oneline | grep "Task 1" | head -1 | awk '{print $1}')
HEAD_SHA=$(git rev-parse HEAD)

[Dispatch reviewer agent]
  WHAT_WAS_IMPLEMENTED: Verification and repair functions for conversation index
  PLAN_OR_REQUIREMENTS: Task 2 from <plan-parent>/deployment-plan.md
  BASE_SHA: a7981ec
  HEAD_SHA: 3df7661
  DESCRIPTION: Added verifyIndex() and repairIndex() with 4 issue types

[Agent returns]:
  Strengths: Clean architecture, real tests
  Issues:
    Important: Missing progress indicators
    Minor: Magic number (100) for reporting interval
  Assessment: Ready to proceed

You: [Fix progress indicators]
[Continue to Task 3]
```

## Integration with workflows

- **Async:** Review after each independently delivered task. Fix before the next task, so issues do not compound.
- **Executing plans:** Review after each review group. Apply the feedback. Then continue.
- **Ad-hoc development:** Review before merge and when stuck.

## Red flags

**Never:**

- Skip review because "it's simple"
- Ignore Critical issues
- Proceed with unfixed Important issues
- Argue with valid technical feedback

**If the reviewer is wrong:**

- Show code or tests that prove it works
- Request clarification
