# Skill authoring best practices

A platform-agnostic checklist for structure, routing, and maintainability when you draft or refactor a skill.

## 1. Front matter conventions

Keep front matter stable and discoverable.

- The name rule and the frontmatter keys are the contract in the
  [SKILL.md structural rules](../forging/forging-skills.md#skillmd-structural-rules) and
  [Required frontmatter](../forging/forging-skills.md#required-frontmatter); follow them there
- `name` should also be specific
- `description` should explain **when to load the skill**, not summarize the whole workflow
- Keep the most important trigger words in the description because routers see it first
- The evaluation record is kept apart from the skill file; no eval key sits in frontmatter
- An unevaluated edit is never reported as passing.

## 2. Lead with routing guidance

Make the skill's boundary obvious. Routing guidance answers:

- When should this skill win?
- Which neighboring skills should handle adjacent cases?
- What conditions mean the agent should not use this skill?

If a skill overlaps with another, say so directly in the skill's `## Don't use when` section. Do not make the router
infer the distinction from examples alone. The description states the skill's own triggers and names no precedence over
another skill; precedence between skills lives in `routing`.

To enforce the declared boundary, carry a `## Don't use when` table, which every `SKILL.md` has
(`skill-routing-section`). Check it as a Step 0 preflight before the workflow runs. See
[skill-hardening-checklist.md](skill-hardening-checklist.md) for the workflow-completeness and pressure-resilience items
that skills most often miss.

## 3. Use `When to Use` and `Don't use when` deliberately

These sections are routing aids as well as documentation headings. The routing section is named `## Don't use when`; no
other name is used for it.

A strong `When to Use` section:

- names concrete triggers, symptoms, and request shapes
- groups similar triggers together
- helps the agent recognize the skill in natural language requests

A strong `## Don't use when` section:

- blocks common false positives
- points toward the better skill or artifact when possible

## 4. Organize reference docs by purpose

Give each reference doc a single job, for example:

- one file for authoring conventions
- one file for evaluation workflow
- one file for rubric or scoring
- one file for domain-specific governance

If a reference doc does three jobs, split it.

## 5. Prefer progressive disclosure over giant SKILL files

SKILL.md should contain:

- the routing story
- the core workflow
- the most important rules
- the map to deeper references

Move heavy examples, long checklists, or scoring rubrics into references, so the invoked skill stays readable.

## 6. Write for cross-platform execution

If a skill includes commands, state the platform differences.

- Write a command that is the same in Bash and PowerShell once; add a second block only where the syntax differs
- Call out platform-specific paths or shell syntax
- When the workflow spans environments, avoid the assumption of one shell, one package manager, or one OS

## 7. Keep examples realistic and scoped

Examples should look like the real tasks the agent will see.

- Use real file paths or repo-relative examples
- Keep the example small enough to scan quickly
- Show the exact trigger and expected response shape when possible

## 8. Design for maintenance

A skill stays easy to maintain when:

- the title still matches the behavior
- the description still routes correctly
- references can evolve without a rewrite of the main SKILL
- evaluation artifacts make it easy to tell whether a change helped or hurt

## Quick review questions

1. Can a router tell when to load this skill from the description alone?
2. Does the main SKILL explain the boundary with neighboring skills?
3. Are the `When to Use` and `## Don't use when` sections concrete enough?
4. Are large details moved into reference docs with clear names?
5. Are shell or platform differences spelled out where needed?
