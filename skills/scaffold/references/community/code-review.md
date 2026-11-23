# Repository code-review skill

Offer this optional customization only through init, as a selectable row in init's menu, and write it only after its
exact text is shown alone and the user says yes ([init.md](../init.md#phase-2-generate-show-and-confirm) step 3). Select
All never selects it. Audit inspects and reports. It never authors a skill. First find existing skills under
`.github/skills`, `.claude/skills`, then `.agents/skills`. Use the first existing `code-review/SKILL.md` in that order.
Report an unreadable or invalid first match rather than fall through to a lower-priority match. Recognize valid freeform
skills. Do not move, merge, rewrite, or normalize them.

Read the applicable `AGENTS.md` and the repository context. Ask what review-specific nuances or workflows the owner
wants captured that are not already covered there. Reuse information already supplied. If the owner has no additional
guidance, leave the skill absent. Do not invent rules to fill a template.

Generate `code-review/SKILL.md` in the repository's project skill directory, chosen as
[author.md](../feedback-loops/author.md) chooses it for a verification skill: the one of `.claude/skills/`,
`.github/skills/` and `.agents/skills/` that already holds the repository's project skills, or `.claude/skills/` when
none does, and one question when more than one does. Review discovers the skill in all three directories, so the choice
does not hide it. Give it ordinary skill frontmatter (`name: code-review` and a description that explains when to use
it). Put the owner's instructions in whatever structure fits. Reference existing repository instructions and relevant
resources rather than duplicate them. Optional supporting files are part of the same preview. If the owner wants review
passes, severity categories, a nit cap, or exclusions, do one of these:

- Write them as the owner states them.
- Offer the [`REVIEW.md` starter](review-md.md).

Review reads `REVIEW.md` before this skill. Do not add headings the owner did not ask for. Do not add Agentics
evaluation metadata.

Before you write, preview the exact proposed files. Then obtain confirmation through scaffold's walkthrough. Preserve an
existing valid skill as authored. Do not silently create a higher-precedence skill that shadows another location.

In audit, optional absence is `inapplicable`, with `Optional — not configured` evidence. It is not a compliance failure.
An existing skill needs readable Markdown instructions and ordinary skill frontmatter with `name` and `description`.
Report actual structural defects with their path and reason, not differences from Agentics's style. Init retains the
same structural checks. It treats optional absence as a separate offer, never an automatic creation.

GitHub recommends `AGENTS.md` for shared standing rules and skills for detailed task-specific workflows. The
`code-review` directory name is a documented review discovery convention, not documented as reserved. GitHub-hosted
Copilot reviews read instructions and skills from the PR head branch. Verification of a hosted review is separate from
CLI testing.

- [Shared instructions and skills](https://docs.github.com/en/copilot/concepts/agents/code-review#choosing-between-custom-instructions-agentsmd-and-skills)
- [Skill authoring and discovery](https://docs.github.com/en/copilot/how-tos/copilot-on-github/customize-copilot/customize-cloud-agent/add-skills)
- [Hosted review customization](https://docs.github.com/en/copilot/how-tos/use-copilot-agents/request-a-code-review/use-code-review)
