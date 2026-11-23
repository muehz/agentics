# Skill description optimizer

When the `description:` field is too vague, too broad, or too crowded to route the skill reliably, use this reference.

## Why the description matters

The description is the primary discovery mechanism. If it is weak, the rest of the skill may never be loaded.

## Recommended pattern

Write descriptions as **verb phrase + scope + key differentiator**. Examples:

- `Use when creating new skills, revising existing skills, or benchmarking skill quality before release.`
- `Use when diagnosing Azure production issues with logs, health signals, and safe triage workflows.`
- `Use when selecting Azure RBAC roles and generating least-privilege assignments.`

## Anti-patterns

### Too vague

- `Helps with skills`
- `Useful for Azure`

### Too long

A description that summarizes the full implementation workflow is hard to scan and easier to misroute.

### Overlap with neighboring skills

If the description could apply equally well to two skills, add the differentiator. A description states its own triggers
and names no precedence over another skill; precedence between skills lives in `routing`.

Bad:

- `Use when reviewing code changes.`

Better:

- `Use when reviewing code changes for substantive bugs, security issues, or logic regressions.`

## Discoverability test

Ask: **Would an AI router select this skill for the right queries and ignore it for adjacent ones?** Pressure-test with:

- a direct hit scenario
- a synonym-rich scenario
- a near miss that should route elsewhere

If the router guesses wrong, revise the description before you touch the body.

For measured discovery, follow [Evaluate skill discovery](trigger-eval.md), which sets the query counts, the split
between development and holdout queries, and the attempts per query. Before the baseline, freeze and version the query
set. Use the development queries to guide revisions. Reserve the holdout queries for the final report. Fix agents
receive extracted development gaps, never frozen queries or holdout contents. This is a manual revision workflow; the
runner does not optimize descriptions or stamp eval status.

## Editing checklist

1. Does the description start with a recognizable action or trigger?
2. Does it name the scope clearly?
3. Does it include the differentiator that separates this skill from nearby skills?
4. Could the same wording accidentally trigger on unrelated requests?
5. Is the sentence short enough to scan quickly in a long skill list?
