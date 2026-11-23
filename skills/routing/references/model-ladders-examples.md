# Model ladders: selector evidence, worked resolutions, and non-consumers

Split from [model-ladders.md](model-ladders.md). Holds per-surface selector verification records, worked tier resolution
tables and boundary cases, and non-consumer notes.

## Selector evidence and limitations

These records state the current verification boundary for each surface's selectors and dispatch controls. They do not
establish current provider availability. No per-model execution probe is claimed. Catalog and documentation checks
establish selectors, not successful model execution.

- **Claude Code:** The `Agent` dispatch tool accepts `model` (`haiku`, `sonnet`, `opus`, `fable`) and `effort` (`low`,
  `medium`, `high`, `xhigh`, `max`), verified from the Claude Code 2.1.322 session tool schema (2026-11-12; no model was
  run). The ladder uses these four aliases rather than full identifiers; aliases follow the host's current model for
  each family. Claude Code 2.1.293 accepted `--model haiku` with `--effort medium` and `--effort high` and answered both
  (2026-10-07), which shows flag acceptance only, not that the alias resolves to a specific version or applies the
  effort.
- **Codex:** `gpt-6-luna`, `gpt-6.1-terra`, `gpt-6.2-sol`, and `gpt-6.2-astra` are the current Codex rungs. The T3 Codex
  catalog lists `gpt-6.1-terra`, `gpt-6.2-sol`, and `gpt-6.2-astra` with `low`, `medium`, `high`, `xhigh`, `max`, and
  `ultra` effort; the ladder uses `low` through `high` for defaults, and additional host-supported efforts remain
  available to explicit pins. A signature check of `codex-cli 0.167.2` (2026-11-12; binary strings, `--help`, and vendor
  docs; no model was run) confirmed that `spawn_agent` takes `model` and `reasoning_effort`, omitting `reasoning_effort`
  inherits the parent effort, and no accepted value list is recorded for either parameter. The `spawn_agent` tool text
  tells the agent to leave `model` unset unless the user explicitly asks for a different model; that text does not
  change this ladder's tier binding. No per-model execution probe is claimed.
- **Copilot CLI and VS Code:** The five role defaults (`gpt-6-luna`, `gpt-6.1-terra`, `gpt-6.2-sol`,
  `claude-sonnet-5.5`, `claude-opus-5.5`), backup-only entries (`gemini-4-argon`, `grok-4.7`), and pin-only
  `claude-haiku-5.5` are Agentics policy at `high` effort; no per-model execution or effort probe is claimed. Copilot
  CLI 1.0.88 has no `--long-context` flag (its controls are `--context <default|long_context>`, the persisted
  `contextTier` setting, the `/model` picker, and per-subagent `subagents.agents.<name>.contextTier`), and accepted a
  one-shot `copilot -p ... --model claude-haiku-5.5 --context long_context --reasoning-effort high` (flag acceptance
  only, not confirmation that the window grew or effort applied). The Claude CLI rejects dotted identifiers like
  `claude-sonnet-5.5` with an API 404. Copilot CLI 1.0.98 `--help` lists no model catalog. The
  [GitHub announcement](https://github.blog/changelog/2026-10-26-gpt-6-1-terra-in-github-copilot/) lists `gpt-6.1-terra`
  with gradual rollout, and [OpenAI's model page](https://developers.openai.com/api/docs/models/gpt-6.1-terra) records
  the identifier. GitHub's
  [supported models list](https://docs.github.com/en/copilot/reference/ai-models/supported-models) did not yet list
  `gpt-6.2-sol` (checked 2026-11-17), so dispatch support remains unverified; use the fallback chain if the host reports
  it unavailable. A signature check of Copilot CLI 1.0.109 (2026-11-12; installed package files and `--help`; no model
  was run) confirmed that `task` takes `model`, `reasoning_effort`, and `context_tier` (`default` or `long_context`),
  omitting `model` applies `/subagents` preferences, `--reasoning-effort` (not `--effort`) is the CLI flag, and no
  accepted value list or unified `task` schema is printed. VS Code shares Copilot's backend and selectors, and its
  `runSubagent` and Copilot-harness model, effort, and context controls remain unverified.
- **Cursor:** `Claude Haiku 5.5` → `Grok 4.7` → `Claude Opus 5.5` is the current default order at `high` effort. No
  picker reading is recorded for these effort choices; bind effort only where the picker exposes it. No Cursor install
  was read in the 2026-11-12 signature check, so no dispatch tool is verified and no model was run.
- **Antigravity:** The Antigravity model catalog lists `gemini-4-hydrogen`, `gemini-4-neon`, and `gemini-4-carbon`
  (checked 2026-11-22; `thought_level` supports `low`, `medium`, `high`, plus `xhigh` on Hydrogen and Neon). The
  `invoke_subagent` tool schema (checked 2026-11-20; no model was run) accepts only `Model`: `inherit` (default),
  `flash_lite`, `flash`, and `pro`, with no per-dispatch effort parameter (`unbound`); bind those native aliases when
  dispatching through `invoke_subagent` (`flash_lite` for `gemini-4-hydrogen`, `flash` for `gemini-4-neon`, `pro` for
  `gemini-4-carbon`). `swarm` accepts full model names or display names via `model`.

Default rungs use provider-documented identifiers. A user-selected default can have unverified support on its dispatch
surface, as recorded above. If a provider check shows a default rung unavailable, apply the fallback rules above.

## Worked resolutions

`⟂` marks a positional clamp. Copilot CLI and VS Code resolve by role and work class, so they have no row here. See
**Copilot CLI and VS Code: role resolution**.

| Surface and driver              | Light                 | Standard            | Deep                | Max                 |
| ------------------------------- | --------------------- | ------------------- | ------------------- | ------------------- |
| Claude Code `haiku`             | `haiku` ⟂             | `haiku`             | `sonnet`            | `opus`              |
| Claude Code `sonnet`            | `haiku`               | `sonnet`            | `opus`              | `fable`             |
| Claude Code `opus`              | `sonnet`              | `opus`              | `fable`             | `fable` ⟂           |
| Claude Code `fable`             | `opus`                | `fable`             | `fable` ⟂           | `fable` ⟂           |
| Codex `gpt-6-luna`              | `gpt-6-luna` ⟂        | `gpt-6-luna`        | `gpt-6.1-terra`     | `gpt-6.2-sol`       |
| Codex `gpt-6.1-terra`           | `gpt-6-luna`          | `gpt-6.1-terra`     | `gpt-6.2-sol`       | `gpt-6.2-astra`     |
| Codex `gpt-6.2-sol`             | `gpt-6.1-terra`       | `gpt-6.2-sol`       | `gpt-6.2-astra`     | `gpt-6.2-astra` ⟂   |
| Codex `gpt-6.2-astra`           | `gpt-6.2-sol`         | `gpt-6.2-astra`     | `gpt-6.2-astra` ⟂   | `gpt-6.2-astra` ⟂   |
| Cursor `Claude Haiku 5.5`       | `Claude Haiku 5.5` ⟂  | `Claude Haiku 5.5`  | `Grok 4.7`          | `Claude Opus 5.5`   |
| Cursor `Grok 4.7`               | `Claude Haiku 5.5`    | `Grok 4.7`          | `Claude Opus 5.5`   | `Claude Opus 5.5` ⟂ |
| Cursor `Claude Opus 5.5`        | `Grok 4.7`            | `Claude Opus 5.5`   | `Claude Opus 5.5` ⟂ | `Claude Opus 5.5` ⟂ |
| Antigravity `gemini-4-hydrogen` | `gemini-4-hydrogen` ⟂ | `gemini-4-hydrogen` | `gemini-4-neon`     | `gemini-4-carbon`   |
| Antigravity `gemini-4-neon`     | `gemini-4-hydrogen`   | `gemini-4-neon`     | `gemini-4-carbon`   | `gemini-4-carbon` ⟂ |
| Antigravity `gemini-4-carbon`   | `gemini-4-neon`       | `gemini-4-carbon`   | `gemini-4-carbon` ⟂ | `gemini-4-carbon` ⟂ |

Other boundary cases:

- Codex Astra at `medium` keeps `gpt-6.2-astra` at `medium` for Standard. It advances a clamped Deep reviewer to `high`
  and keeps Max at the supported `high`. Astra at `high` keeps `high` for Standard. Standard never resets to `low`.
- A `spec.md` or `plan.md` author and its independent reviewer follow the Standard and Deep columns above (see
  **Reviewer selection**). A Claude Code `sonnet` driver authors on `sonnet` and dispatches the reviewer on `opus`. A
  `fable` driver at `medium` authors on `fable` at `medium` and advances the clamped reviewer to `fable` at `high` in a
  separate dispatch. A `fable` driver at `high` has no stronger effort, so it keeps the separate reviewer and discloses
  the shared binding. On Copilot CLI and VS Code the spec and plan reviewers run on `claude-sonnet-5.5` whatever the
  author's tier.
- An unknown, unlisted, or `Auto` driver uses flat inheritance wherever the host guarantees it. Copilot CLI and VS Code
  use their fixed role defaults for every dispatch (see **Copilot role resolutions**). Report the driver as unknown;
  report the actual model as unknown only when the host does not confirm it. No driver position is invented.
- A user request to dispatch with a supported model outside the surface's table is honored immediately as an explicit
  out-of-table pin and is never inserted into the default order.
- A user request for `high` effort on a model or host that does not expose `high` records requested `high` and actual
  `unknown`/unavailable. It then pauses that explicit dispatch, unless the user authorized a fallback. The request is
  never silently changed to `medium`.
- A Copilot Deep code implementation or investigation runs on `gpt-6.1-terra`. Its review group's code-quality review
  runs on `gpt-6.2-sol`; its per-task specification review runs on `claude-sonnet-5.5`. A skill-assigned Max review
  label does not change those models. A user Max pin selects `claude-opus-5.5` instead.
- A Copilot whole-PR review that mixes code and prose runs on `gpt-6.2-sol`. A security review or final code
  verification does too. If Sol is unavailable at every effort, try `gemini-4-argon`, then `grok-4.7`, and report the
  substitution.
- A Copilot Deep task that rewrites documentation runs on `claude-sonnet-5.5`. Its artifact review also runs on Sonnet
  in a separate dispatch. If Sonnet is unavailable at every effort, try `gemini-4-argon`, then `grok-4.7`.
- Copilot Standard analysis runs on `gpt-6-luna` whether the driver is Sol, Opus, Haiku, or unknown/`Auto`.
- A Copilot Light or Standard implementer dispatch runs on `gpt-6-luna`. If `gpt-6-luna` is unavailable at every effort,
  the dispatch blocks and asks, because `claude-haiku-5.5` is pin-only.
- A user who pins `claude-haiku-5.5` on Copilot gets it. The report names the requested binding, including the requested
  context tier. It discloses that the host has not confirmed `long_context`, unless it does.
- A full Claude API identifier (rather than one of the four aliases) is an out-of-table pin on Claude Code. The Agent
  dispatch `model` parameter accepts only the four aliases. Honor the pin where the dispatch surface accepts it. Where
  the surface does not accept the pin, the explicit-selection branch of **Unavailable selections and reporting**
  applies:
    - Use only a fallback the user already authorized.
    - Otherwise, block the dispatch and ask for a replacement.

## Non-consumers

- **The `peer` dev tool** (`.github/tools/src/peer`) pins alternate-family models by design. Its `models.ts` owns those
  bindings. This ladder does not rewrite them.
- **Evaluation runners** pin their models deliberately so a score remains comparable across sessions. Keep their
  explicit provider pins independent of this ladder.
- **Auto mode** is an unknown driver, not a rung (see **Standard inheritance**).
- **A host without model choice** must not receive a fabricated `model:` parameter. Its model binding is unbound or
  unverified, as the paragraph under the parameter table in **Effort defaults and controls** defines. Use a verified
  effort parameter when available. Otherwise, control cost through structure, such as fewer agents, batched mechanical
  work, and tighter fan-out.
