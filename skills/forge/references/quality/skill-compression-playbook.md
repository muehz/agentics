# Skill compression playbook

Rules to cut a skill's token footprint with no change to its behavior. Every word in a SKILL.md is a per-invocation
cost; every word in `description:` is a per-session cost. The goal is the shortest text that still passes the same eval
bar. Compression that costs a passing verdict is a regression.

Companion to the [skill-hardening-checklist](skill-hardening-checklist.md), which decides **what** a skill must say.
This playbook decides **how briefly** it says it. Compression must never delete a hardening-mandated element (preflight
gate, abort output, "why the boundary matters"). Keep each of the skill's gates as its one statement with its class
(safety gate, repository policy, or process gate). Delete the second, third, and fourth restatements instead.

## The edit rules

1. **One statement per rule.** Heavy skills state their central gate 3–4× (Iron Law block, phase enforcement, Common
   Mistakes, Quick Reference, "Holding the line"). Keep the single most enforceable statement, usually the one where the
   workflow enforces it, together with its class. Delete the rest. A gate said once, where it fires, beats a gate said
   four times.
2. **Imperatives, not essays.** Collapse rationale paragraphs into the rule itself. Keep at most one "why" clause per
   boundary. Cut the surrounding justification. That clause is the concrete failure mode, or for a gate the harm or cost
   it names once.
3. **One example per pattern.** Second and third examples, full walkthroughs, and rarely-run sub-modes move to the
   skill's existing `references/`. Create the file if needed. Leave a one-line pointer in the skill's established link
   style at each move. Inline stays reserved for the one example the operator needs on the happy path.
4. **One encoding per fact.** Where a table duplicates prose (or another table), keep whichever is denser, usually the
   table. Delete the other. Tables are for enumerable facts; prose is for decision logic.
5. **Merge the routing furniture.** That furniture is "Don't use when", the Step-0 preflight, and Common Mistakes. These
   sections often encode the same routing rules two or three ways. Collapse to the one routing section, the
   `## Don't use when` table that every `SKILL.md` has (`skill-routing-section`), plus a minimal preflight that points
   at it. (`handoff` is the target shape: one routing table, no redundant gate sections.)
6. **Cut throat-clearing.** Delete purpose restatements of the frontmatter description, "This skill helps you…" openers,
   section summaries, and transitions. The H1 plus one purpose line is enough.
7. **Descriptions are triggers, not summaries.** Apply the [skill-description-optimizer](skill-description-optimizer.md)
   pattern (verb phrase + scope + differentiator). Then stop. Aim for about 25 words; that is a compression target, not
   a gate item. The only limit is 1,024 characters (`skill-description-length`), as the
   [SKILL.md structural rules](../forging/forging-skills.md#skillmd-structural-rules) state. Keep every trigger keyword
   and differentiator. Cut workflow summary, rationale, and redundant phrasings of the same trigger.

## Preserve verbatim: never compress

- Frontmatter: every key and value of the contract in
  [Required frontmatter](../forging/forging-skills.md#required-frontmatter), plus any other key the skill already
  carries, such as the `modes`, `mode-aliases`, and `flags` that conformance reads to check references to the skill.
  Only `description` wording may change under rule 7. No eval key sits in frontmatter; the evaluation record is kept
  apart from the skill file and follows the re-evaluation contract below.
- Shell command blocks: a command that is the same in Bash and PowerShell is written once; a second block appears only
  where the syntax differs. Keep a block that marks a real syntax difference. Commands themselves are not prose. Do not
  "tighten" flags or arguments.
- `references/` links and cross-skill `/name` pointers. Every pointer must still resolve after the edit.
- Decision questions: link the [shared question format](../../../routing/references/contracts/question-format.md) for
  the reusable template, exemptions, and answer collection. Keep workflow-specific questions with their own wording or
  choices.
- Anything a fixture pins (roster tables, canonical section names like `## Don't use when`).
- Pinned scenario banks are off-limits to read and to edit.

## Done means

The target is the same eval verdict at materially fewer tokens. Report before/after word counts with every compression
pass. A pass that saves under ~15% on an already-lean skill is fine. Do not invent cuts to hit a quota. Do not leave
duplication to preserve "voice".

Compression changes the content digest. Where an asset already reads `passing`, do both of these:

- Set `not-run`.
- Preserve the historical evaluation date and digest until a structured re-evaluation returns a passing result.

That result earns a new `passing` status and digest only where the repository's convention records them.
