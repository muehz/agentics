# Model identifiers outside the ladder

The `routing` skill's [model ladder](../../skills/routing/references/model-ladders.md) holds every rung, default or
backup-only, in its order table. This page holds the other half: the identifiers Agentics binds or cites deliberately
off the ladder. They live here rather than in the skill because they record where this repository copies an identifier,
and a skill must not describe the repository it ships from.

## Pinned identifiers

A rung, default or backup-only, lives only in the ladder's order table. An identifier Agentics binds or cites
deliberately off the ladder gets one row here, saying where it is used and why it differs. Conformance rule
`model-ids-known` (the `docs` domain) reads both tables. It fails on any model identifier that neither table names in
`docs/surfaces/*.md`, non-test Go string literals under `hooks/go`, `.github/tools/src/peer/models.ts`,
`evals/README.md`, `evals/skills.yaml`, `.github/workflows/release.md`, or `.github/workflows/backlog-grooming.md`. It
matches API identifiers (`gpt-`, `claude-<family>-`, `grok-`, `gemini-`) and display names (`GPT-<n>`, `Composer`,
`Grok`, `Claude <Family>`). Claude Code's bare aliases are ordinary words in prose and are not matched. Rule
`model-ids-pin-used` fails on a pin row whose identifier none of those files names, so a pin cannot outlive its last
copy.

| Identifier         | Used in                            | Reason                                                                                                                                             |
| ------------------ | ---------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------- |
| `claude-haiku-5-5` | `evals/README.md`                  | The Claude lane in the `evals cycle` examples; evaluation runners pin their models so scores stay comparable (see the ladder's **Non-consumers**). |
| `claude-haiku-5.5` | `evals/README.md`                  | The Copilot CLI Haiku eval lane; `claude-haiku-5.5` is pin-only on Copilot and sits off the ladder's order table.                                  |
| `gpt-6.1-sol`      | `.github/tools/src/peer/models.ts` | The `peer` tool pins its Codex model independently of the dispatch ladder.                                                                         |

## Renaming a model

To rename a model, edit its rows in the ladder's order, effort, and worked-resolution tables and in the ladder's
**Copilot CLI and VS Code: role resolution** section, **Reviewer selection** paragraph, boundary cases, and selector
evidence, and update any pin row here that names it, or delete the row when the rename removes the last copy. Change
`settings.copilot-model` in `evals/skills.yaml` only when the renamed identifier is its value. Then run
`node .github/tools/src/conformance/cli.ts docs --repo .`. Every `model-ids-known` failure names a copy the rename
missed in the files listed above. Copies elsewhere are outside the rule, so find them with `git grep -n '<old-id>'`.
Update Go test fixtures by hand. A frozen eval bank under `evals/` changes only with a new bank version, and
`docs/specs/` history and vendored text stay as they are.
