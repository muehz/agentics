# 08. Extend

Agentics is a baseline, not a ceiling. Three skills make it fit your repo.

## Forge your own assets

```text
Forge a skill for our deploy checklist, so every session runs the same steps in the same order.
```

Why this phrasing works: a repeated workflow with a fixed order is exactly what a skill captures, and
[`forge`](../../skills/forge/SKILL.md) owns authoring skills, agents, instruction files, and MCP servers in any
repository, plus the hardening checklist and evals that keep them honest. Name the workflow and why it must be
repeatable. Forge interviews for the rest.

## Mine a session for lessons

```text
Reflect on this session and propose durable lessons for the Agentics assets it used.
```

Why this phrasing works: after a session where something went unusually well or badly, reflect mode mines the transcript
for durable lessons and proposes edits to existing assets. Every edit waits for your explicit approval, and anything a
lint rule or hook would enforce more reliably is set aside as backlog rather than written into prose. It can also
propose improvements to your own repository's agent environment, such as a missing pointer or an unwired check; reflect
itself never changes your repository; pointer additions you select are applied only through scaffold init's per-item
previews.

## Give your repo a real-app verification loop

```text
Use scaffold feedback-loops to create a verification skill for this app.
```

Why this phrasing works: [`scaffold`](../../skills/scaffold/SKILL.md)'s feedback-loops author mode interviews your
repository and generates a project-local skill that launches your real app, drives it, and captures evidence, plus a
feature map. That turns "confirm the change works in the actual app" from an ad-hoc effort into a scripted, repeatable
step. Later, its feedback-loops audit mode keeps the generated skill honest against the current app.

**Pitfall:** don't forge a skill for what structure enforces better. A rule that a hook, lint check, or CI gate can
enforce mechanically should live there. Prose instructions drift, get skipped under pressure, and cost context every
session; structure fires every time. Forge's own reflect mode applies this test to its proposals. Apply it to yours
before authoring.

Next: [09. Recipes and pitfalls](09-recipes-and-pitfalls.md)
