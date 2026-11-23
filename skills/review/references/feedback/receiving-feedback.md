<!-- metadata.source: "obra/superpowers (adapted)" -->

# Receiving feedback

Improve the change set. Do not accept every suggestion automatically. Do not defend the current code by reflex.

## Core principles

1. Read the full review before replying or editing.
2. When the intent is not obvious, restate the technical issue in your own words.
3. Verify comments against the actual code, tests, and requirements before acting.
4. Separate real blocking issues from style preferences and local conventions.
5. Respond with evidence, not flattery or defensiveness.

## Anti-sycophancy rules

Do not treat reviewer comments as correct by default.

- Do **not** agree just to appear collaborative.
- Do **not** promise a fix before you confirm the feedback is technically sound.
- Do **not** use praise as a substitute for analysis.
- Do acknowledge valid concerns clearly. Do fix them efficiently.
- When a suggestion would introduce regressions, violate requirements, or add unnecessary complexity, do push back.

Good responses are factual:

- `Fixed the null-handling path in auth.ts and added coverage for the failure case.`
- `Checked this flow against the current requirements; the existing behavior is intentional because the API must stay backward compatible.`

## Triage: blocking, preference, or out-of-scope?

Sort each comment into one of the three categories below.

### Usually blocking

- Broken behavior or missing requirement coverage
- Security, privacy, compliance, or data-loss risk
- Accessibility regressions or serious usability defects
- Incorrect business logic or edge-case handling
- Missing or misleading tests for critical paths

### Usually non-blocking (preference)

- Formatting or naming preferences already handled by tooling
- Equivalent implementation alternatives with no measurable risk difference
- Minor stylistic consistency issues that do not reduce readability

If a comment is preference-based, do one of these two things:

- Apply it, because it improves consistency at low cost.
- Explain why it is out of scope for the current review.

### Usually out-of-scope

- Suggestions that are better suited for a follow-up refactor than the current change set
- Valid concerns about code the diff touches only incidentally, not the change itself
- Improvements that would expand the change beyond its stated requirements

Treat out-of-scope comments as their own category, not a subset of preference. Acknowledge the concern. Note it as
follow-up work. Explain why it stays out of the current review. Do not fold it into a fix. Do not drop it silently.

## Process feedback in batches, not randomly

Do not jump back and forth between unrelated comments. Recommended order:

1. Clarify unclear feedback first.
2. Group blocking issues by area of the codebase.
3. When closely related fixes share context, apply them together.
4. Re-run relevant validation after each batch.
5. Once the batch is complete and verified, reply in review threads.

Batches reduce repeated context switches and prevent half-fixes. They make interactions between comments easier to spot.

## When to push back

When you can show that the requested change would make the code worse or violate constraints, push back.

Use evidence such as:

- requirement text or acceptance criteria
- current tests and their intended behavior
- compatibility obligations
- measured performance or accessibility impact
- repository conventions already established elsewhere

A strong pushback response is calm and specific:

- explain what you checked
- identify the concrete trade-off
- propose an alternative when helpful

Example:

> I checked the existing API contract and downstream consumers still rely on this field. Renaming it in this PR would
> create a breaking change. I can add an adapter now or stage the rename behind a versioned follow-up.

## Before requesting review again

Do a short self-review before you ask reviewers to look again:

- Does the diff still match the original requirement and scope?
- Did I resolve the actual concern, not just the surface comment?
- Did I re-run the relevant tests, build, or validation steps?
- Did I update screenshots, docs, or examples if the change altered behavior?
- Did I remove temporary debug code, dead branches, and accidental formatting churn?
- Are my replies concise, factual, and tied to the change that was made?

## Practical response pattern

```text
1. Read all comments.
2. Clarify anything ambiguous.
3. Triage each item: blocking issue, preference, or out-of-scope follow-up.
4. Fix related items in batches.
5. Validate each batch.
6. Reply with evidence and the resulting change.
```

Step 6 posts a reply to GitHub only as the skill's [Posting](../../SKILL.md#posting) rule allows. Otherwise it drafts
the reply and delivers it locally.

## Bottom line

A constructive review response is cooperative **and** technically rigorous. Accept valid feedback quickly. Challenge
weak feedback with evidence. Always verify before you say a comment is resolved.
