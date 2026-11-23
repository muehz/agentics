# Scaffold reference: docs/CONSTITUTION.md

## Early-stage advisory

> **Heads up:** CONSTITUTION.md captures architectural principles and core decisions. It is most useful after the team
> has made real architectural decisions. For a brand-new repo with no code yet, ask: **"Do you have at least one
> architectural decision to document? (yes/skip)"** If the user says skip, note it in the summary. Then move on.

If the user says yes, or if this is an existing repo, ask the questions below.

## Questions to ask

Ask what's open as one ❓ round. Give each question a ➡️ recommendation drawn from the request and the repo. Skip a
topic the request already settles.

- **Core architectural principles:** the 2–5 that govern this project (examples: "adapter-first", "config-as-code",
  "test-first", "fail-closed").
- **Per-principle rationale and requirements:** for each principle, a 1–2 sentence rationale and concrete
  MUST/SHOULD/MUST NOT requirements.
- **Hard constraints,** if any, on tech choices, patterns, or integrations.

## Structure

```markdown
[Home](../README.md) > Constitution

# {Project Name} Constitution

## Core Principles

### I. {Principle Name}

{Description and rationale, 1–3 paragraphs}

**MUST:**

- **{Label}**: {Specific, actionable requirement}

**SHOULD:**

- **{Label}**: {Best practice guidance}

**MUST NOT:**

- **{Label}**: {Explicitly forbidden action}

### II. {Next Principle}

...

## Constraints

- **{Constraint}**: {Hard limit on a tech choice, pattern, or integration, and its reason}
```

## Rules

- The breadcrumb link at the top is required: `[Home](../README.md) > Constitution`
- Number principles with Roman numerals (I, II, III...)
- Every principle needs a rationale and at least one MUST/SHOULD/MUST NOT requirement
- Always include the `## Constraints` section. If the user names no hard constraints, write `None yet.` under it
- Use actionable verbs such as _configure_, _implement_, _validate_, _use_, and _enforce_, not vague phrases.

## Conformance check (audit mode)

✅ Conformant if the file contains at least one core principles section and at least one constraints/decisions section.
