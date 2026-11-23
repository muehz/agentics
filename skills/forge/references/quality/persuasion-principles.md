# Persuasion principles for skill instructions

When a skill is technically correct but still easy to skim past, misunderstand, or rationalize away, use this reference.

## Core idea

Persuasive skill writing makes the right behavior obvious, concrete, and difficult to misinterpret, without hype.

## 1. Clarity over cleverness

Choose direct language over memorable-but-vague phrasing.

- Prefer `Run the repository's existing tests for the files you changed.`
- Avoid `Close the loop with confidence.`

## 2. Concrete examples over abstract rules

Abstract rules are easy to agree with and easy to misapply. Concrete examples show what the rule looks like in practice.

- State the rule first
- Follow it with one concise example
- Add a counter-example only when the failure mode is common

Example:

```markdown
When a skill has more than one trigger, list them as concrete user situations.

Good: "Use when the user asks for a cost forecast, cost breakdown, or spend anomaly analysis." Bad: "Use for
cost-related workflows."
```

## 3. Progressive disclosure keeps the main skill short

Put the routing logic and non-negotiable behavior in SKILL.md. Move deep reference material into supporting docs.

The core skill stays short enough to read end to end. The deeper content still exists when an agent needs examples,
rubrics, or edge cases.

## 4. Action-oriented language wins

Skills work best when the instruction starts with a recognizable trigger and ends with a clear action.

Patterns that help:

- `Use when ...`
- `If X is true, do Y`
- `Before coding, verify ...`
- `Do not continue until ...`

Patterns that weaken execution:

- `Consider ...`
- `It may be helpful ...`
- `Try to ...`

Use softer language only when legitimate judgment is required.

## 5. Guardrails prevent avoidable errors

If a mistake is predictable, name it and block it explicitly.

Good guardrails usually include:

- The failure pattern
- Why it is risky
- What to do instead

Example:

```markdown
Do not point multiple subagents at the same file set. Shared edits erase the value of parallelization and create merge
churn.
```

## 6. Match firmness to the job

- Use firm language for safety gates
- Treat an ordering constraint as a process gate: name its cost once, and when the next reply still declines, yield to
  the user
- Use flexible language for heuristics, trade-offs, and context-sensitive advice

Overuse of absolute language makes a skill brittle. Underuse makes a safety-critical skill easy to bypass.

## Quick checklist

Before you ship a skill section, ask:

1. Is the trigger obvious?
2. Is the next action explicit?
3. Did I use a concrete example where confusion is likely?
4. Did I move optional depth into a reference doc?
5. Did I add guardrails for the most common failure mode?
