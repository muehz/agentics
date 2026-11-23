# Model ladders: surface defaults and tier resolution

This file is the only file that maps a tier to a model or an effort. It is the canonical home for the ordered model
choices, tier offsets, effort defaults, override precedence, and fallback rules. `routing` owns it. A skill names one
tier for each dispatch role it has, helpers included, and nothing else. Every skill that binds a model for dispatch
consumes this file: `execute` and its dispatch and review references, `review`, `triage`, `recall`, `explain`,
`decompose`, `forge`, and `hillclimb`. Change the policy here once. Then reconcile the consumers.

An identifier that has no row in the order table is not a rung. A rung is a default entry, or on the Copilot CLI and VS
Code row a backup-only entry named there. Treat any other identifier as an explicit out-of-table pin. Honor the pin
where the dispatch surface accepts it. Never insert it into the default ordering (see **Standard inheritance** and
**Non-consumers**).

## Ordered models and tiers

Claude Code, Codex, Cursor, and Antigravity are ordered from lower to higher model position. Copilot CLI and VS Code use
fixed roles instead of a positional ladder. These are Agentics policy choices, not a vendor ranking or a guarantee of
current provider availability. Copilot backup models are used only when a default is unavailable.

| Surface                 | Lower to higher model position                                                                                                                                                                                             |
| ----------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Claude Code             | `haiku` → `sonnet` → `opus` → `fable`                                                                                                                                                                                      |
| Codex                   | `gpt-6-luna` → `gpt-6.1-terra` → `gpt-6.2-sol` → `gpt-6.2-astra`                                                                                                                                                           |
| Copilot CLI and VS Code | Routine: `gpt-6-luna`; Deep code work: `gpt-6.1-terra`; code review: `gpt-6.2-sol`; substantial prose and artifact review: `claude-sonnet-5.5`; explicit Max: `claude-opus-5.5`; backup only: `gemini-4-argon`, `grok-4.7` |
| Cursor                  | `Claude Haiku 5.5` → `Grok 4.7` → `Claude Opus 5.5`                                                                                                                                                                        |
| Antigravity             | `gemini-4-hydrogen` → `gemini-4-neon` → `gemini-4-carbon`                                                                                                                                                                  |

For a known driver at position `d`, resolve the unpinned tiers as follows. Clamp at either ladder end.

| Tier     | Position |
| -------- | -------- |
| Light    | `d - 1`  |
| Standard | `d`      |
| Deep     | `d + 1`  |
| Max      | `d + 2`  |

The tiers mean this:

- Standard is defined work.
- Deep is judgment about what to do, including plan, quality, architecture, and security review.
- Light is mechanical work whose output is cheap to verify.
- Max is review of Deep work or an explicit selection.
- A reviewer of a packet, a plan, or a spec runs at Deep. It runs at Max only on an explicit user selection.
- A quality reviewer may still resolve to Max when the work it reviews was implemented at Deep, by the rule in
  **Reviewer selection** (the next position above the actual implementer).
- A helper that only looks up a fact runs at Light.

A tier for one named skill's role, such as the `zealot` tests finder, is not written here. That skill names it.

A tier is a default selection preference. It does not veto a valid explicit model, effort, or tier pin.

These offsets apply to every row except Copilot CLI and VS Code. That row resolves by role and work class (see **Copilot
CLI and VS Code: role resolution**).

If the driver changes, resolve defaults again before a later dispatch. Agents that already run keep their recorded
bindings. A model pin remains that model when the driver changes. A tier pin resolves against the new driver on
positional surfaces. Copilot defaults do not change with the driver.

### Standard inheritance

On positional surfaces, Standard inherits the current driver model and effort only when the dispatch tool guarantees
both forms of inheritance. When it does not, bind the known driver and known effort explicitly. An optional `model` or
effort field alone is not proof of inheritance.

If the driver model is known but its effort is not observable, do these steps:

- When an explicit binding is required, use the documented base effort.
- When the host cannot confirm the effort, report the effort as unknown.

On positional surfaces, an unknown, unlisted, or `Auto` driver has no position in the table. For every unpinned tier, do
these steps:

- Use flat inheritance where the tool provides it.
- Disclose the uncertainty.
- Omit unsupported model or effort fields.

Do not invent a driver, selector, position, or unattended default. A supported explicit model outside this table is
accepted as a pin. It is not inserted into the default ordering.

Copilot CLI and VS Code resolve every dispatch by role and work class, including Standard work and sessions with an
unknown or `Auto` driver (see **Copilot CLI and VS Code: role resolution**).

## Copilot CLI and VS Code: role resolution

Copilot CLI and VS Code use five defaults. The session driver never selects a default. Skills still classify work as
Light, Standard, Deep, or Max, but a skill-assigned Max label does not select Opus. Every default runs at `high` effort
where the host supports it. Apply user overrides before these defaults.

| Dispatch                                                                                                        | Model               |
| --------------------------------------------------------------------------------------------------------------- | ------------------- |
| Routine non-review work: Light or Standard implementation, extraction, fact-gathering, or analysis              | `gpt-6-luna`        |
| Deep code work that is not a review: implementation, debugging, or investigation                                | `gpt-6.1-terra`     |
| Every code review, including review-group code quality, security, aggregate review, and final code verification | `gpt-6.2-sol`       |
| Substantial prose work (Deep) and every artifact review, including per-task spec compliance                     | `claude-sonnet-5.5` |
| A dispatch the user explicitly selects at Max                                                                   | `claude-opus-5.5`   |

- **Work class.** The class of work that is not a review is the class of its subject. The class of a review is the one
  the lists below name.
    - Code class is:
        - code, tests, configuration, and infrastructure
        - diffs and whole-PR review
        - the `execute` code-quality and aggregate reviewers
        - `gate`
        - the `review` Standards and Security lanes and its `post`, `interactive`, and `impact` modes
        - tests and mutation review (`zealot`)
        - configuration and infrastructure review
        - the `arena` judge
        - `triage` checks of fixes
    - Artifact class is:
        - plans, specs, and intents
        - skill and instruction text
        - documentation
        - the `execute` per-task spec-compliance reviewer
        - the `review` Spec lane
        - `decompose` and `spec` review, including the `spec` independent reviewer
        - `intent` and `challenge` review
        - `forge` reviewers
        - `explain` critics
    - A change that mixes code and prose is code class.
    - Deep work that is neither a change to code nor a change to prose, such as an investigation, a reconstruction, or
      an analysis, is code class when it reads or runs code to reach its answer. Otherwise it is artifact class.
- **Reviews and tier labels.** Every unpinned reviewer resolves by review class. A Deep implementer does not change
  either execute reviewer: spec compliance uses Sonnet and code quality uses Sol. Skill-assigned Max labels on plan,
  quality, aggregate, or independent reviewers do not promote them to Opus. Non-review work labeled Max by a skill uses
  the Deep default for its class.
- **Explicit selections.** A user model pin always wins. A user tier pin at Light or Standard selects Luna; Deep selects
  Terra for non-review code work, Sol for code review, or Sonnet for artifact work and review. A user Max tier pin
  selects Opus for any role. An explicit effort pin is preserved separately. These overrides still apply when a skill's
  normal reviewer preference would be stronger.
- **Haiku pin.** `claude-haiku-5.5` is supported only as an explicit user pin, with the long-context tier. On Copilot
  CLI the per-dispatch control is the `context_tier` argument of `task`, which takes `default` or `long_context`. The
  `--context long_context` flag, the persisted `contextTier` setting (also the `/model` picker), and a
  `subagents.agents.<name>.contextTier` value of `long_context` are session or agent-definition settings that a user
  sets. They are not parameters a dispatch passes. Report the requested context tier. Disclose that the host has not
  confirmed it, unless it does. A Haiku session driver does not select Haiku for child work.
- **VS Code.** The same rows apply. In a Local harness session, `runSubagent` has no verified effort or context
  parameter, so the report discloses the effort and context tier as unverified (see **Effort defaults and controls**). A
  Copilot-harness session, the default since 1.143, dispatches with `task`. Agentics has not verified which model and
  effort controls a Copilot-harness session in VS Code exposes, so the report discloses them as unverified too. Haiku
  runs only on a user pin.
- **Reporting.** Use the existing dispatch fields. The Limitation/reason cell names the cause, for example
  `fell back: gpt-6.2-sol unavailable, gemini-4-argon used`, `Haiku pinned: context tier unverified`, or
  `code review: gpt-6.2-sol, regardless of implementer tier`.

### Fallbacks

This is the unavailable-selection rule for Copilot CLI and VS Code. A fallback applies only when an unpinned default is
unavailable at every effort. It never changes a default. No default cell above names a fallback.

| Unavailable default                                    | Fallback order                    |
| ------------------------------------------------------ | --------------------------------- |
| `gpt-6.1-terra`, `gpt-6.2-sol`, or `claude-sonnet-5.5` | `gemini-4-argon`, then `grok-4.7` |

- `gemini-4-argon` and `grok-4.7` run at `high` effort. Each stands in for the other when one is unavailable.
- When every model in the chain is unavailable, the dispatch blocks and asks.
- An unavailable `claude-opus-5.5` blocks and asks. Opus is in no chain.
- An unavailable `gpt-6-luna` at every effort blocks and asks, because Haiku is pin-only.
- A fallback that lands on the model that implemented the work, or that drives the session, keeps the separate reviewer
  dispatch. The report discloses the shared binding (see **Reviewer selection**).
- The report names every substitution.
- An explicit pin that is unavailable follows the explicit-selection rule in **Unavailable selections and reporting**.

### Copilot role resolutions

The same task gets the same default under every session driver, including an unknown, unlisted, Haiku, or `Auto` driver.
Bind the selected role model explicitly where the host accepts it. If the host cannot bind a model or effort, the
binding is unbound or unverified, as the paragraph under the parameter table in **Effort defaults and controls**
defines. Report it that way; do not present inherited execution as the selected default. User pins retain their normal
scope.

## Effort defaults and controls

When a model has several documented effort choices, its base is the lowest one. Stronger tiers that clamp onto the same
model advance through supported choices. Weaker tiers reduce effort only where supported. At an ordinary model
transition, use the selected model's documented base, unless Standard preserves the driver's known effort.

| Surface                 | Model               | Documented effort choices (weakest → strongest) |
| ----------------------- | ------------------- | ----------------------------------------------- |
| Claude Code             | `haiku`             | `high`                                          |
| Claude Code             | `sonnet`            | `medium`                                        |
| Claude Code             | `opus`              | `medium`                                        |
| Claude Code             | `fable`             | `medium` → `high`                               |
| Codex                   | `gpt-6-luna`        | `high`                                          |
| Codex                   | `gpt-6.1-terra`     | `low` → `medium` → `high`                       |
| Codex                   | `gpt-6.2-sol`       | `low` → `medium` → `high`                       |
| Codex                   | `gpt-6.2-astra`     | `low` → `medium` → `high`                       |
| Copilot CLI and VS Code | `claude-haiku-5.5`  | `high`, with the long-context tier              |
| Copilot CLI and VS Code | `gpt-6-luna`        | `high`                                          |
| Copilot CLI and VS Code | `gpt-6.1-terra`     | `high`                                          |
| Copilot CLI and VS Code | `gpt-6.2-sol`       | `high`                                          |
| Copilot CLI and VS Code | `claude-sonnet-5.5` | `high`                                          |
| Copilot CLI and VS Code | `claude-opus-5.5`   | `high`                                          |
| Copilot CLI and VS Code | `gemini-4-argon`    | `high`                                          |
| Copilot CLI and VS Code | `grok-4.7`          | `high`                                          |
| Cursor                  | `Claude Haiku 5.5`  | `high`                                          |
| Cursor                  | `Grok 4.7`          | `high`                                          |
| Cursor                  | `Claude Opus 5.5`   | `high`                                          |
| Antigravity             | `gemini-4-hydrogen` | `high`                                          |
| Antigravity             | `gemini-4-neon`     | `high`                                          |
| Antigravity             | `gemini-4-carbon`   | `high`                                          |

When a stronger tier clamps to the top model, move to the next supported effort for each stronger offset. Stop at that
model's strongest documented effort. At a bottom clamp, lower effort for weaker offsets only if the model supports a
lower choice.

If no stronger effort or per-dispatch effort control exists, do these steps:

- Keep the model.
- Say that the binding is shared or effort is unobservable.
- Preserve the separate reviewer.

Agentics recommends an additional reviewer at a collapsed clamp. An additional reviewer is not mandatory solely because
the model or effort is shared.

**Per-dispatch model and effort parameters.** The table lists only parameters of each surface's dispatch tool. A
command-line flag, a configuration key, or a model picker sets a session. It is not a per-dispatch control and has no
cell here.

| Surface                  | Dispatch tool     | Model parameter                                                                                                                | Effort parameter                                    |
| ------------------------ | ----------------- | ------------------------------------------------------------------------------------------------------------------------------ | --------------------------------------------------- |
| Claude Code              | `Agent`           | `model`: `haiku`, `sonnet`, `opus` or `fable`                                                                                  | `effort`: `low`, `medium`, `high`, `xhigh` or `max` |
| Codex                    | `spawn_agent`     | `model`                                                                                                                        | `reasoning_effort`                                  |
| Copilot CLI              | `task`            | `model`                                                                                                                        | `reasoning_effort`                                  |
| VS Code, Local harness   | `runSubagent`     | unverified: accepted where the tool takes a model                                                                              | unverified: no effort parameter recorded            |
| VS Code, Copilot harness | `task`            | unverified: which model control VS Code exposes                                                                                | unverified: which effort control VS Code exposes    |
| Cursor                   | none verified     | no dispatch tool verified                                                                                                      | no dispatch tool verified                           |
| Antigravity              | `invoke_subagent` | `Model`: `inherit`, `flash_lite`, `flash`, or `pro` (tier aliases for `gemini-4-hydrogen`, `gemini-4-neon`, `gemini-4-carbon`) | `unbound` (no per-dispatch effort parameter)        |

Copilot CLI's `task` also takes `context_tier`, with `default` or `long_context`. It binds only the context tier of a
Haiku pin (see **Haiku pin**), so it has no column here. No other surface's dispatch tool has a verified context
parameter.

A cell that names a parameter is verified. A cell without a value list names a parameter whose accepted values no check
has recorded. Unbound means the dispatch tool has no parameter for that binding. The skill omits it, uses the
inheritance the tool provides, and reports `unbound` in the Actual binding field. A verified parameter that the
session's own tool schema does not show is reported as unbound for that session. Unverified means no check has shown the
parameter either way. The skill passes it only when its own session's tool schema shows it. Otherwise the skill treats
it as unbound and reports `unverified`. Here the word describes a dispatch parameter. On Copilot CLI, the report string
`Haiku pinned: context tier unverified` means something else: the host has not confirmed the requested value.

Capabilities come from the actual dispatch surface. Never copy a selector or effort parameter between providers because
the names look alike. If a parameter is absent, omit it and report the binding as the paragraph above defines.

## Reviewer selection

Always retain a separate quality reviewer and the completion handoff. For an unpinned reviewer, choose the stronger of:

1. the task's review tier; and
2. the next model position above the actual implementer, when that actual model is ranked on the surface.

At a top clamp, increase effort above the implementer's known effort where the selected model supports it.

If the implementer's effort is unknown, the host cannot bind effort (the binding is unbound or unverified, as the
paragraph under the parameter table in **Effort defaults and controls** defines), or the model is unranked, do these
steps:

- Do not claim a stronger effort or independence that was not observed.
- Keep the separate reviewer.
- Disclose the shared or unranked binding.
- Recommend an additional reviewer, but do not require one.

A failed implementation, review finding, or provider response does not by itself prove model unavailability.

The drafting session authors `spec.md` and `plan.md` at its own tier, which is Standard, because Standard is the
driver's own position. The independent reviewer of each artifact runs at the Deep tier, the stronger of the two choices
above: Deep is both the review tier and the next position above a Standard author. The reviewer runs at Max only when
the user explicitly selects Max for that review, which is a tier pin. On Claude Code, Codex, Cursor, and Antigravity the
reviewer therefore sits one model position above the author, read from the Deep column of **Worked resolutions**. At a
top clamp, the rules above apply: raise effort where the model supports it; otherwise keep the separate reviewer and
disclose the shared binding. An unknown, unlisted, or `Auto` driver on those surfaces has no position, so the author and
the reviewer use flat inheritance and disclose the uncertainty (see **Standard inheritance**).

On Copilot CLI and VS Code every unpinned reviewer follows review class: Sol for code, Sonnet for artifacts. It does not
follow the position or tier of the implementer, so the `spec.md` and `plan.md` reviewers run on `claude-sonnet-5.5`
whatever the author's tier. Only an explicit user Max selection chooses Opus (see **Copilot CLI and VS Code: role
resolution**).

## User overrides and scope

Honor the first clear explicit model, effort, or tier instruction for implementation or review immediately. User pins
override every implementer or reviewer floor, including a below-floor or supported out-of-table selection. Apply the pin
to its stated scope: task, wave, role, run, or session.

A task-scoped pin expires with that task. A persistent role, run, or session pin survives driver changes. For an
affected dispatch, a later task-specific pin overrides an earlier applicable role, run, or session pin for that task.
The persistent pin resumes when the task ends. More generally, a later request governs within its stated scope. Within
an identical scope, the latest conflicting instruction wins. A model-only pin does not erase a separately pinned effort.

Do not ask the user to repeat a clear pin. Resolve an actual scope conflict from the conversation when possible. Ask
only when the scopes cannot be determined. An override controls model selection only. It does not waive verification,
separate quality review, or the completion handoff.

## Unavailable selections and reporting

For an unpinned default whose selected model is unavailable, move to the nearest lower available model on that surface.
Skip every effort variant of the unavailable model. Explain the reason. Report the actual fallback. Do not confuse a
model-level outage with a failed task or review. If every lower default is unavailable, block the affected dispatch
rather than loop or fabricate success. Independent work may continue. Copilot CLI and VS Code are the exception. Their
fallbacks are in **Copilot CLI and VS Code: role resolution**.

Use only a fallback the user already authorized in these two cases:

- An explicit model pin or tier pin whose requested model is unavailable.
- An explicit effort pin that the selected model or host cannot support.

A tier pin whose resolved model is unavailable follows this explicit-selection branch. It does not become an unpinned
default. Without an authorized fallback, block the affected dispatch and ask for a replacement. Preserve an unsupported
effort request. Report the limitation. Do not silently discard or replace the request. An explicit fallback list
authorizes only the listed alternatives.

Dispatch previews and run reports distinguish these fields:

| Field                | Meaning                                                                                                                                                             |
| -------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Task/role and driver | What is being dispatched and the known driver, or unknown/Auto                                                                                                      |
| Requested tier       | The task classification or explicit tier pin                                                                                                                        |
| Default binding      | What the ladder would select before overrides or degradation                                                                                                        |
| Override and scope   | The applicable model, effort, or tier pin and its task/wave/role/run/session scope                                                                                  |
| Requested binding    | The model and effort actually requested after applying the override                                                                                                 |
| Actual binding       | The provider-confirmed model and effort, or `unknown` when the surface does not expose it, or `unbound` or `unverified` as **Effort defaults and controls** defines |
| Limitation/reason    | Inheritance uncertainty, an unbound or unverified parameter, unavailable model/effort, fallback, or shared reviewer binding                                         |

Keep default degradation separate from an explicit override. Never present a requested selector as provider-confirmed
when actual binding is unobservable.

## Selector evidence and limitations

Moved to
[model-ladders-examples.md § Selector evidence and limitations](model-ladders-examples.md#selector-evidence-and-limitations).

## Worked resolutions

Moved to [model-ladders-examples.md § Worked resolutions](model-ladders-examples.md#worked-resolutions).

## Non-consumers

Moved to [model-ladders-examples.md § Non-consumers](model-ladders-examples.md#non-consumers).
