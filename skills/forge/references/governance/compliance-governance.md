# Compliance governance for skill authoring

This reference gives the enterprise compliance requirements for skill authors. The parent [SKILL.md](../../SKILL.md)
holds the core framework to author skills.

## Compliance context

Skills represent your organization's engineering standards. They must reflect the rigor appropriate to your compliance
framework.

If your organization handles regulated data, do this step:

1. When you design a skill that touches sensitive data or regulated operations, consider the applicable regulatory
   requirements. Examples include SOC 2, industry-specific mandates, and organizational security policies.

**REQUIRED BACKGROUND:** The `forge` skill defines TDD-based skill creation. It covers SKILL.md structure, search
optimization, file organization, the Iron Law, code examples, and flowcharts. This reference adds governance, compliance
context, testing methodology, and persuasion principles.

## Routing guidance

- Use when you create or revise `SKILL.md` artifacts.
- Use when you design pressure tests or verify skill effectiveness before deployment.
- Use when you apply persuasion principles to discipline-enforcing skills.
- Route to `intent` when scope or ownership is unclear.
- Route to `gate` for final quality and policy checks.

## Core responsibilities

- Enforce concise frontmatter and third-person language across all skills. A "Use when …" trigger counts as third
  person.
- Define routing guidance and responsibilities without overlap.
- Prevent prohibited references. Maintain baseline governance standards.
- Ensure skills follow the test-driven development cycle before deployment.
- Require applicable compliance guardrails in skills that touch regulated data or sensitive operations.

## Naming, sorting, and indexing standard

- Name each skill with a short, lowercase kebab-case identifier that matches its directory (`skills/<name>/`). The
  `name` field in the SKILL.md front matter is the deduplication key.
- Keep one folder per skill. Avoid duplicate skill variants.

## Catalog maintenance expectations

- Register each new skill where the repository registers skills.

## Compliance-aware skill design

Skill authors must account for requirements appropriate to their organization's compliance framework:

- **Data classification.** Identify the level a skill's domain touches: public, internal, confidential, or regulated.
  Skills that touch regulated or confidential data must include handling constraints.
- **Compliance guardrails.** Skills that process, display, or reference sensitive data must embed compliance boundaries
  in the skill. A reference alone is not enough.
- **Regulatory context.** Your organization's applicable regulations and internal policies may constrain workflows.
  Skills must not instruct agents to bypass or shortcut regulated processes.
- **Prompt safety integration.** Any skill that handles data must cross-reference the applicable compliance guardrails.
  Without that cross-reference, agents can surface or log protected information by accident.
- **Organizational voice.** Skills represent your organization's engineering standards. Their language should be precise
  and authoritative. It should match the rigor your domain expects.

## Compliance-aware RED-GREEN-REFACTOR

[../quality/advanced-patterns.md](../quality/advanced-patterns.md) (canonical) defines the RED-GREEN-REFACTOR cycle:
baseline pressure tests without the skill, a minimal skill that addresses observed rationalizations, and loophole
closure. Compliance adds one requirement to each phase:

- **RED:** For compliance-sensitive skills, identify these two items:
    - The data classification level that the workflow touches.
    - Whether regulated data guardrails must be tested explicitly.
- **GREEN:** For skills that touch regulated or confidential data, embed applicable compliance guardrails directly in
  the skill.
- **REFACTOR:** For regulated workflows, close loopholes that would let an agent bypass, shortcut, or weaken required
  compliance steps.

For meta-testing, see [governance-details.md](governance-details.md).

## Common mistakes

[../quality/advanced-patterns.md](../quality/advanced-patterns.md) (canonical) catalogs the generic authoring
anti-patterns, such as narrative examples, multi-language dilution, code in flowcharts, and generic labels. Compliance
adds one mistake.

### Missing compliance context

A skill processes regulated data but does not reference the applicable compliance constraints. Regulated data handling
without guardrails creates compliance risk.

## Skill creation checklist with compliance gates

| Gate         | Check                                                                                                                            |
| ------------ | -------------------------------------------------------------------------------------------------------------------------------- |
| **RED**      | Pressure scenarios created, baseline behavior documented verbatim                                                                |
| **GREEN**    | Frontmatter valid, description starts with "Use when...", addresses baseline failures, agents comply with skill present          |
| **REFACTOR** | New rationalizations countered, rationalization table built, red flags list created                                              |
| **Quality**  | Data classification identified, applicable compliance guardrails if regulated data, guardrails cross-referenced if data handling |
| **Deploy**   | Test each skill individually. Do not batch-create skills without per-skill tests                                                 |

The full checklist is in [../quality/advanced-patterns.md](../quality/advanced-patterns.md) (canonical). The table above
holds the compliance-specific quality and deployment checks.

## STOP before you move to the next skill

**After the author writes ANY skill, they MUST STOP. They MUST then complete the deployment process.**

**Do NOT:**

- Create multiple skills in batch without a test of each
- Move to the next skill before you verify the current one
- Skip the tests because "batching is more efficient"

To deploy an untested skill is to deploy untested code, and it violates quality standards.

## Ethical compliance test

**Ethical test:** Would this technique serve the user's genuine interests if they fully understood it?
