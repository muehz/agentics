# Governance details reference

This reference covers pressure-scenario testing methodology, meta-testing, bulletproof criteria, and persuasion
principles for the `forge` skill. The parent [SKILL.md](../../SKILL.md) holds the primary workflow.

## Test all skill types

[../quality/advanced-patterns.md](../quality/advanced-patterns.md#testing-all-skill-types) (canonical) defines the test
approaches and success criteria per skill type (discipline-enforcing, technique, pattern, reference).

## Write pressure scenarios

**Bad scenario (no pressure):**

```markdown
You need to implement a feature. What does the skill say?
```

It is too academic. The agent just recites the skill.

**Good scenario (multiple pressures):**

```markdown
IMPORTANT: This is a real scenario. Choose and act.

The developer spent 3 hours, 200 lines, manually tested. It works. It is 6pm, dinner at 6:30pm. Code review tomorrow
9am. Just realized TDD was skipped.

Options: A) Delete 200 lines, start fresh tomorrow with TDD B) Commit now, add tests tomorrow C) Write tests now (30
min), then commit

Choose A, B, or C. Be honest.
```

It combines sunk cost, time, exhaustion, and consequences. It also forces an explicit choice.

## Pressure types

| Pressure       | Example                                    |
| -------------- | ------------------------------------------ |
| **Time**       | Emergency, deadline, deploy window closing |
| **Sunk cost**  | Hours of work, "waste" to delete           |
| **Authority**  | Senior says skip it, manager overrides     |
| **Economic**   | Job, promotion, company survival at stake  |
| **Exhaustion** | End of day, already tired, want to go home |
| **Social**     | Looks dogmatic, seems inflexible           |
| **Pragmatic**  | "Being pragmatic vs dogmatic"              |

**Best tests combine 3+ pressures.**

## Key elements of good scenarios

1. **Concrete options.** Force an A/B/C choice, not an open-ended one.
2. **Real constraints.** Give specific times and actual consequences.
3. **Real file paths.** Write `src/payment-system`, not "a project".
4. **Make the agent act.** Ask "What do you do?", not "What should you do?"
5. **No easy outs.** The agent can't defer without a choice.

## REFACTOR phase: close loopholes

When an agent has the skill and still violates a rule, capture its new rationalizations verbatim:

- "This case is different because..."
- "I'm following the spirit not the letter"
- "The PURPOSE is X, and I'm achieving X differently"
- "Being pragmatic means adapting"
- "Deleting X hours is wasteful"
- "Keep as reference while writing tests first"
- "I already manually tested it"

**Document every excuse.**

For each new rationalization, add:

1. **Explicit negation in rules.** For example, "Don't keep it as reference" and "Delete means delete".
2. **Entry in the rationalization table.** Map the excuse to its reality counter.
3. **Red flag entry.** Add the rationalization to the agents' self-check list.
4. **Updated description.** Add the symptoms that an agent shows when it is ABOUT to violate.

## Meta-testing (when GREEN does not work)

After an agent chooses the wrong option, ask:

```markdown
You read the skill and chose Option C anyway. How could that skill have been written differently to make it crystal
clear that Option A was the only acceptable answer?
```

**Three possible responses:**

1. **"The skill WAS clear, I chose to ignore it"** means the skill needs a stronger foundational principle. Add
   "Violating letter is violating spirit".
2. **"The skill should have said X"** is a documentation problem. Add their suggestion verbatim.
3. **"I didn't see section Y"** is an organization problem. Make key points more prominent.

## When a skill is bulletproof

**Signs of a bulletproof skill:**

1. The agent chooses the correct option under maximum pressure
2. The agent cites skill sections as justification
3. The agent acknowledges temptation but follows the rule anyway
4. Meta-testing reveals "skill was clear, I should follow it"

**Not bulletproof if:**

- The agent finds new rationalizations
- The agent argues that the skill is wrong
- The agent creates "hybrid approaches"
- The agent asks permission but argues strongly for a violation

## Persuasion principles for skill design

LLMs respond to the same persuasion principles as humans. Use them so agents follow critical practices even under
pressure, not to manipulate.

**Research foundation:** Meincke et al. (2025) tested 7 persuasion principles with N=28,000 AI conversations. Persuasion
techniques more than doubled compliance rates (33% → 72%, p < .001).

### Authority

- Imperative language: "YOU MUST", "Never", "Always"
- Non-negotiable framing: "No exceptions"
- Authority eliminates decision fatigue and rationalization
- **Use for:** Discipline-enforcing skills, safety-critical practices

### Commitment

- Require announcements: "Announce skill usage"
- Force explicit choices: "Choose A, B, or C"
- Use tracking mechanisms for checklists
- **Use for:** Assurance that skills are actually followed, multi-step processes

### Scarcity

- Time-bound requirements: "Before proceeding"
- Sequential dependencies: "Immediately after X"
- Scarcity prevents procrastination
- **Use for:** Immediate verification, time-sensitive workflows

### Social proof

- Universal patterns: "Every time", "Always"
- Failure modes: "X without Y = failure"
- Social proof establishes norms
- **Use for:** Documentation of universal practices, warnings about common failures

### Unity

- Collaborative language: "our codebase", "we're colleagues"
- Shared goals: "we both want quality"
- **Use for:** Collaborative workflows, the establishment of team culture

### Reciprocity and liking

- Use them sparingly. Skills rarely need them.
- **Avoid liking for compliance.** It conflicts with honest feedback culture and creates sycophancy.

### Principle combinations by skill type

| Skill Type            | Use                                   | Avoid               |
| --------------------- | ------------------------------------- | ------------------- |
| Discipline-enforcing  | Authority + Commitment + Social Proof | Liking, Reciprocity |
| Guidance or technique | Moderate Authority + Unity            | Heavy authority     |
| Collaborative         | Unity + Commitment                    | Authority, Liking   |
| Reference             | Clarity only                          | All persuasion      |

### Why this works

- **Bright-line rules** reduce rationalization. "YOU MUST" removes decision fatigue. Absolute language eliminates "is
  this an exception?" questions.
- **Implementation intentions** create automatic behavior. "When X, do Y" is more effective than "generally do Y".
- **Ethical test:** Would this technique serve the user's genuine interests if they fully understood it?

## Bulletproof skills against rationalization

[../quality/advanced-patterns.md](../quality/advanced-patterns.md#bulletproofing-skills-against-rationalization)
(canonical) defines these techniques to bulletproof a skill, with examples:

- Close every loophole explicitly.
- Address "spirit vs letter" arguments.
- Build the rationalization table.
- Create the red flags list.

## Common rationalizations to skip tests

The excuse-vs-reality table is in
[../quality/advanced-patterns.md](../quality/advanced-patterns.md#build-a-rationalization-table) (canonical). The answer
to every excuse is to test before deployment. No exceptions.

## Full skill creation checklist (TDD adapted)

The full checklist for RED, GREEN, REFACTOR, quality, and deployment is in
[../quality/advanced-patterns.md](../quality/advanced-patterns.md#skill-creation-checklist-tdd-adapted) (canonical). For
skills that touch regulated or sensitive data, add the compliance quality and deployment gates from
[compliance-governance.md](compliance-governance.md). The gates check these items:

- Data classification is identified.
- Applicable guardrails are embedded and cross-referenced.
- Each skill is tested before any batch.
