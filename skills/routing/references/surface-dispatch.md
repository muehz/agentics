# Native dispatch by surface

How to dispatch subagents (execute waves, routing delegation, and every other dispatching skill) through each surface's
native orchestration tools. The statements about the dispatch tools of Claude Code, Copilot CLI and Codex were checked
against Claude Code 2.1.322, Copilot CLI 1.0.109 and codex-cli 0.167.2 on 2026-11-12. Claude Code was checked from the
session tool schema, the vendor documentation and `claude --help`. Copilot CLI was checked from the installed package's
files and its help text. Codex was checked from the installed build, its help text and the vendor documentation. The
check ran no model. A statement marked unverified was not shown by that check and carries the date of the check it last
passed. The VS Code section marks the statements that were source-confirmed against VS Code's documentation on
2026-11-19; its other statements are unverified. The Antigravity section was checked from the Antigravity session tool
schema on 2026-11-20; the check ran no model. Tool rosters drift between versions. When a named dispatch tool is absent,
use the generic fallback at the end rather than invent an equivalent. A tool that does not dispatch follows **Tool
names**.

## Feature-detect, don't surface-detect

Choose the dispatch path by the tools present in this session's tool inventory. Do not guess from the product name.
Surfaces and versions differ in which tools they have. A session can see its own tools.

### Tool-presence index

Match the tools your session has. Then read that surface's section below for options and current limits. For every row,
**Concurrency and overlap** sets the batch size.

| Tools present in session                                                                                                                                   | Dispatch pattern                                                                                                                                                                                                                                                                   |
| ---------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `Agent` with `subagent_type` (Claude Code)                                                                                                                 | One `Agent` call per task, batched in a single message. Batch size and tasks that write the same files follow **Concurrency and overlap**. `SendMessage` resumes a finished subagent. Agent teams add live teammates.                                                              |
| `task` with `agent_type` (Copilot CLI / app / VS Code Copilot harness)                                                                                     | Multiple `task` calls in one turn. Built-in `rubber-duck` / `code-review` agent types serve the two review gates.                                                                                                                                                                  |
| `runSubagent` (VS Code Local harness)                                                                                                                      | Local harness only. Unverified: parallel dispatch not confirmed for this harness, so dispatch one at a time; no background mode. Source-confirmed: nested delegation off by default.                                                                                               |
| `spawn_agent` (Codex)                                                                                                                                      | Spawning needs permission first. See **Dispatch permission**.                                                                                                                                                                                                                      |
| `invoke_subagent` (with `TypeName`, `Role`, `Prompt`, `Model`, `Workspace`), `send_message`, `manage_subagents`, `swarm`, and `run_workflow` (Antigravity) | Batch `invoke_subagent` calls in one turn; subagents run in the background and report back via messages. `send_message` continues a subagent by `conversationId`; `define_subagent` registers custom types; `swarm` and `run_workflow` cover board-based teams and script fan-out. |
| None of the above                                                                                                                                          | No native dispatch. The dispatching skill takes its declared fallback. For example, `execute` stops and tells the user `pair` is the only mode that runs. It never picks `pair` itself.                                                                                            |

### User-only capabilities

These capabilities are never model-invoked on any surface:

- `/fleet` and `/delegate` (Copilot CLI)
- agent view (`claude agents`) and `claude --bg` (Claude Code)
- Codex Cloud tasks and best-of-N runs

Recommend them when they fit. Never claim to have run one.

## Dispatch permission

This rule applies on a surface that spawns only on a request, such as Codex, which spawns after a direct request or an
applicable `AGENTS.md` or skill instruction. It is the same for every skill. A plain `Agent`, `task`, `runSubagent`, or
`invoke_subagent` dispatch on a surface with no such gate needs no question.

- **A typed invocation is the request.** A user-typed invocation of a skill whose workflow dispatches is that request.
  On Codex a typed invocation of a plugin skill has a `$` prefix (`$<plugin>:<skill>`), distinct from built-in slash
  commands like `/hooks`.
- **Any other load asks once.** A skill loaded any other way, including an `execute` that `routing` selected, asks once
  for permission to dispatch. A yes is the request.
- **Restate the request.** The skill restates the request when it dispatches.
- **A refusal takes the fallback.** A refusal, or a spawn the host still refuses, takes the skill's declared fallback.
  See **Any other surface (generic fallback)**.

Evidence, read 2026-11-12: the `spawn_agent` text in codex-cli 0.167.2 also accepts an `AGENTS.md` or skill instruction
as the request ("Do not spawn sub-agents unless the user or applicable AGENTS.md/skill instructions explicitly ask for
sub-agents, delegation, or parallel agent work."). Codex's
[subagents page](https://learn.chatgpt.com/docs/agent-configuration/subagents), read 2026-11-19, agrees: "Current local
Codex releases spawn agents after a direct request or applicable project or skill instruction." Agentics still asks once
on a load the user did not type.

## Concurrency and overlap

These rules bound parallel dispatch on every surface. They are the same for every skill.

| Surface     | Recorded limit on subagents running at once                                                                                          | Setting                                                                   |
| ----------- | ------------------------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------- |
| Claude Code | 20                                                                                                                                   | `CLAUDE_CODE_MAX_CONCURRENT_SUBAGENTS`                                    |
| Codex       | Codex chooses the default when the setting is unset; the Codex source used 6 (2026-11-18)                                            | `agents.max_concurrent_threads_per_session`                               |
| Copilot CLI | By plan, 2 to 32: Free / Education 2, Pro / Pro+ 4, Max 8, Business 16, Enterprise 32, usage-based billing 32                        | `subagents.maxConcurrency`, usage-based billing only, capped at 32        |
| VS Code     | Copilot harness: follows the Copilot CLI row, unverified for VS Code. Local harness: 1 (one at a time; parallel dispatch unverified) | Copilot harness: as Copilot CLI, unverified. Local harness: none recorded |
| Cursor      | none recorded                                                                                                                        | none recorded                                                             |
| Antigravity | none recorded (no fixed vendor limit verified; use Agentics default 2 unless higher is confirmed)                                    | none recorded                                                             |

- **Stay under the limit.** No batch exceeds the surface's limit, counted over every subagent still running. The rest
  queue. Reviewer batches fall under the same limit. Copilot CLI's limit depends on a plan the session cannot see, so
  use 2 unless the plan is known. A VS Code Copilot-harness session inherits that rule.
- **No recorded limit.** A surface with no recorded limit uses 2, the lowest default recorded.
- **Count what is still open.** On Codex a completed agent stays open and counts toward the limit until `close_agent`
  closes it. Count it as running until then. Close a completed agent once its result is read. `resume_agent` reopens a
  closed agent for a delta pass.
- **A reached limit is not a refusal.** A spawn that fails because the concurrency limit is reached does not take the
  fallback. Its task waits in the queue and is tried again when a slot frees: when a running subagent finishes, or on
  Codex when a completed agent is closed.
- **Same files run in sequence.** Tasks that write the same files run one after another. Worktree isolation is for arena
  candidates and isolated packets. It does not make overlapping tasks parallel.

## Claude Code

- **Dispatch:** the `Agent` tool with `subagent_type`, `description` and `prompt`, one call per task, batched in a
  single message for parallelism.
- **Options:** per-call `model` and `effort`. [model-ladders.md](model-ladders.md) says how a tier is bound to them.
  `isolation: "worktree"` gives a subagent its own worktree. It is for arena candidates and isolated packets, not for
  tasks that write the same files. See **Concurrency and overlap**. Background execution is the default, and results
  arrive as notifications.
- **Nesting and concurrency:** a subagent can spawn its own subagents up to three layers below the main conversation
  (`CLAUDE_CODE_MAX_SUBAGENT_SPAWN_DEPTH`). At that depth `Agent` is withheld from an ordinary subagent. A fork keeps
  the tool but gets an error instead of a spawn. A session runs at most 20 subagents at once
  (`CLAUDE_CODE_MAX_CONCURRENT_SUBAGENTS`). A spawn past the cap fails. Claude must not retry it until one finishes.
  **Concurrency and overlap** says what its task does meanwhile.
- **Continuation:** `SendMessage` with the agent's ID or name resumes a completed subagent with its history intact. It
  does not need agent teams. Only team-protocol messages such as `shutdown_request` do. The built-in Explore and Plan
  agents are one-shot and return no agent ID, so they cannot be resumed. Dispatch a subagent that may be continued, such
  as a reviewer, as `general-purpose` or a custom type ([subagents page](https://code.claude.com/docs/en/sub-agents),
  read 2026-11-19).
- **Agent teams (experimental, `CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS=1`):** Spawn live teammates. Steer them with
  `SendMessage`. Where the task tools (`TaskCreate`/`TaskList`/`TaskUpdate`) are present, coordinate through the shared
  task list. They are on by default only on older models, so detect them by presence; without them, teammates coordinate
  through messages ([tools reference](https://code.claude.com/docs/en/tools-reference), read 2026-11-19). Constraints:
  no nested teams, in-process teammates do not survive `/resume`, and in-process teammates cannot start background
  subagents: their subagents run in the foreground ([agent teams page](https://code.claude.com/docs/en/agent-teams),
  read 2026-11-19). Whether split-pane teammates can start background subagents was not observed live (split panes need
  tmux or iTerm2, and the Windows host had neither), 2026-11-19. Prefer teams when workers need multi-turn steering.
  Prefer one-shot dispatch when a single bounded result suffices.
- **Workflow escalation:** for boards of roughly 10+ tasks, the `Workflow` tool runs a deterministic script fan-out. It
  requires the user's explicit multi-agent opt-in. That opt-in is met the way **Dispatch permission** states: a
  user-typed invocation of a dispatching skill is the request, and a skill loaded any other way asks once. A no keeps
  plain `Agent` dispatch, which needs no question.

## Copilot CLI and Copilot app

- **Dispatch:** the `task` tool with `agent_type`, `name`, `description`, `prompt` and `mode` (`sync` or `background`).
  It also takes `model`, `reasoning_effort` and `context_tier`. [model-ladders.md](model-ladders.md) says how a tier is
  bound to them. The installed package defines the agent types `code-review`, `explore`, `research`, `rubber-duck`,
  `security-review`, `task` and `rem-agent`. `general-purpose` is also an agent type. It has no definition file.
- **Parallelism:** issue multiple `task` calls in one turn. `mode: "background"` is available to the top-level agent
  only (unverified, 2026-07-06). The user can promote a running `sync` task to the background (Ctrl+X then B, or B in
  `/tasks`), so a `sync` dispatch may finish as a background agent. Never launch a single background agent and poll for
  it. Dispatch the wave. Then integrate.
- **Limits:** the
  [CLI command reference](https://docs.github.com/en/copilot/reference/copilot-cli-reference/cli-command-reference),
  read 2026-11-19, gives a subagent nesting depth of 6 by default, set by `subagents.maxDepth` and capped at 256. The
  same page documents `COPILOT_SUBAGENT_MAX_DEPTH` with a default of 4 (range 1 to 128). A Copilot CLI 1.0.109 session
  with neither set got depth 4 on 2026-11-19: the fourth nested subagent's `task` call was refused with "Maximum
  sub-agent depth of 4 reached." The depth can differ by plan and configuration, so feature-detect it, and treat a
  refused nested spawn as the depth limit. `subagents.maxConcurrency` sets how many subagents run at once. Its default
  depends on the plan, from 2 to 32, and it is capped at 32. See **Concurrency and overlap**. Both settings are ignored
  on a plan without usage-based billing. Subagents share the parent session's AI-credit limit (unverified, 2026-07-06).
  The billing multiplier guard may silently downgrade a requested `model` (unverified, 2026-07-06). Keep the tier
  intent.
- **Continuation:** `write_agent` sends a follow-up message to a background agent, addressed by its `agent_id`.
  `read_agent` reads a background agent's result. `write_agent` supports background agents only, so an agent dispatched
  in `sync` mode cannot be continued while it stays `sync`. Observed in Copilot CLI 1.0.109 on 2026-11-19: the `sync`
  task's result carries no agent ID, and `write_agent` on the ID that `list_agents` shows is refused with "write_agent
  only supports background agents." Whether a `sync` task the user promoted to the background then accepts `write_agent`
  is unverified (2026-11-18): try `write_agent` on its `agent_id`, and dispatch a fresh task only if that call is
  refused. An idle agent accepts `write_agent` only for a follow-up on the same task. Subagents can persist and
  communicate across parent, child, and sibling relationships. Write self-contained prompts anyway. A dispatched agent
  does not inherit the parent conversation. Do not assume that a follow-up requires a fresh one-shot handoff. Limits
  here are plan- and configuration-dependent.
- **Review gates:** the built-in `rubber-duck` agent type fits Stage 1 (spec review), and `code-review` fits Stage 2
  (quality review). The gates themselves are unchanged. Only the dispatch target is native.
- **Fleet contract (large boards):** `/fleet` is a user command. It sends a canned prompt that drives the same `task`
  tool plus the session `sql` tool (SQLite). Replicate the contract directly with these steps:
    1. Load the tasks into `todos` as marked records, with dependencies in `todo_deps`, as
       [native-plan-projection.md](native-plan-projection.md) defines. This is the same upsert `execute` runs.
    2. Dispatch independent rows in parallel.
    3. Have each subagent `UPDATE` its row to `done`.
    4. Validate the results.
    5. Dispatch whatever remains.

    Treat the SQL state, not subagent claims, as the board of record. A row stays `done` only when the task's Stage 2
    review passed and its commit evidence exists. The controller checks each subagent's `done` claim against that
    evidence. This is the one path where Agentics reads the task table back. Every other write to it follows
    [native-plan-projection.md](native-plan-projection.md).

- **User shortcuts to mention, never invoke:** `/fleet <objective>` interactively, or non-interactively
  `copilot -p "/fleet <objective>" --no-ask-user` (identical in bash and PowerShell). `/delegate` hands the session to
  the Copilot cloud agent for PR-shaped work. The model-invocable equivalent is the GitHub MCP server's
  `create_pull_request_with_copilot` tool (unverified, 2026-07-06), when enabled. Feature-detect it. Do not assume it.
- **Copilot app sessions:** when session tools are present (`create_session`, `send_session_message`, and
  `respond_to_session_plan`, which is unverified, 2026-07-06), the app's own convention is one session ≈ one branch ≈
  one PR. Use sessions for branch-sized work units. Use `task` subagents for work inside a session. Prefer idle
  notifications over waits.
- **No file locking:** parallel subagents share the filesystem. The last writer wins silently. Enforce the Step 0
  ownership boundaries strictly on this surface.

## VS Code (Copilot Chat)

VS Code runs a session on one of two harnesses. A statement marked source-confirmed was checked on 2026-11-19 against VS
Code's [1.143 release notes](https://code.visualstudio.com/updates/v1_143),
[subagents page](https://code.visualstudio.com/docs/agents/run/subagents) and
[custom agent file reference](https://code.visualstudio.com/docs/agents/reference/custom-agent-file-reference). No
session has exercised it. Every other statement in this section is unverified, and no check has covered it.

- **Copilot harness:** source-confirmed 2026-11-19: a Copilot-harness session (default since 1.143) uses `task`, the
  native dispatch tool, not `runSubagent`, and does not accept a custom agent's `agents:` frontmatter. Follow the
  **Copilot CLI** section for it. Which Copilot CLI controls VS Code exposes in that harness is unverified.
- **Local harness, dispatch:** source-confirmed 2026-11-19: the `runSubagent` tool (`agent/runSubagent`) belongs to the
  Local harness. Source-confirmed 2026-11-19: each invocation is stateless and returns its result to the main agent.
  Unverified: background is not supported, and the result returns synchronously.
- **Parallel:** "Delegate independent work in parallel only when the tasks don't depend on each other's results." (VS
  Code's subagents page, source-confirmed 2026-11-19). The docs do not confirm concurrent runs for the Local harness, so
  a Local-harness session dispatches one task at a time unless the session shows concurrent runs.
- **Local harness, limits:** source-confirmed 2026-11-19: nested delegation is off by default, and
  `chat.subagents.allowInvocationsFromSubagents` permits it up to depth five. The setting does not apply to the Copilot
  harness. A requested model is checked against the main model's cost tier, so it cannot exceed that tier.
- **Local harness, coordinator gating:** source-confirmed 2026-11-19: a custom agent's `agents:` frontmatter is a
  Local-only allowlist of the subagents it may dispatch. If subagent dispatch appears unavailable in a Local-harness
  session, the active agent's allowlist is the first thing to check.

## Codex

- **Dispatch:** multi-agent v1 tools (stable, on by default): `spawn_agent`, `send_input`, `wait_agent` (accepts
  multiple ids), `list_agents`, `close_agent`, `resume_agent`. `spawn_agent` takes `agent_type`, `model`,
  `reasoning_effort`, and the task as `message` or `items`. [model-ladders.md](model-ladders.md) says how a tier is
  bound to them. Roles: `default`, `worker` (execution-focused), `explorer` (read-heavy), plus a user's own custom TOML
  roles from `~/.codex/agents/` (user scope) or `.codex/agents/` (project scope).
- **Authorization gate:** Codex spawns subagents after a direct request or an applicable `AGENTS.md` or skill
  instruction ([subagents page](https://learn.chatgpt.com/docs/agent-configuration/subagents), read 2026-11-19).
  Observed in a codex-cli 0.170.0 `codex exec` session on 2026-11-19: a four-file task that did not ask for delegation
  ran without spawning a subagent; a second session with the same flags listed the spawn tools. Follow **Dispatch
  permission** for what counts as the request, when to ask, and what a refusal does.
- **Limits:** `agents.max_concurrent_threads_per_session` sets the number of concurrent agent threads, and
  `agents.max_threads` is a legacy alias. "When you leave agents.max_concurrent_threads_per_session unset, Codex chooses
  the default." (subagents page, read 2026-11-19). The Codex source used 6 on 2026-11-18. Depth defaults to 1, from the
  Codex source (2026-11-18). `agents.max_depth` is undocumented: the configuration reference does not list it, though
  codex 0.170.0 still reads `max_depth`, so it may change without notice. A spawn past either limit fails. A completed
  agent stays open and counts toward the thread limit until `close_agent` closes it. **Concurrency and overlap** says
  how to size a batch and what a spawn that reaches the thread limit does.
- **Version caution:** `multi_agent_v2` tool names have churned across releases. Never hardcode v2 names
  (`send_message`, `followup_task`, …). Describe intent with the v1 tools above.
- **Batch fan-out:** codex-cli 0.167.2 no longer has `spawn_agents_on_csv`. The tool name is absent from the installed
  build, and `codex features list` reports the `enable_fanout` feature as removed. Fan out with `spawn_agent` calls.
- **User-only:** Codex Cloud tasks and best-of-N runs (`codex cloud exec --attempts N`). Recommend them for PR-shaped
  fan-out. Never claim to have dispatched them.

## Cursor

No dispatch tool is verified for Cursor. A Cursor session that shows none takes the fallback in **Any other surface
(generic fallback)**. One that shows a dispatch tool detects it by presence, per **Feature-detect, don't
surface-detect**.

## Antigravity

Checked from the Antigravity session tool schema on 2026-11-20; the check ran no model.

- **Dispatch (one-shot and persistent):** `invoke_subagent` with `TypeName`, `Role`, `Prompt`, optional `Model`, and
  optional `Workspace`. Built-in types are `self` (inherits the parent configuration), `research` (read-only sandbox
  with no network access), and `image-generator`. `define_subagent` registers a reusable custom type for the
  conversation (`Name`, `Description`, `SystemPrompt`, `EnableWriteTools`, `EnableMCPTools`, `EnableSubagentTools`).
  Every `invoke_subagent` call runs in the background and returns a `conversationId`; results arrive automatically as
  messages, so do not poll in a loop. Use one-shot dispatch when a single bounded result suffices, and persistent
  continuation via `send_message` when the subagent needs multi-turn steering. `manage_subagents` (`Action`: `list`,
  `kill`, or `kill_all`, with `ConversationIds` for `kill`) inspects and terminates direct subagents; killing a subagent
  also terminates its descendants and deletes their branched workspaces.
- **Parallelism:** batch multiple `invoke_subagent` calls in a single turn for independent tasks, bounded by
  **Concurrency and overlap** (Agentics default 2 unless higher is confirmed). Tasks that write the same files still run
  in sequence.
- **Options and `Workspace` isolation:** per-call `Model` accepts `inherit` (default), `flash_lite`, `flash`, or `pro`,
  with no per-dispatch effort parameter ([model-ladders.md](model-ladders.md) says how tiers bind to those aliases).
  `Workspace` accepts `inherit` (default, shares the parent workspace), `share` (new workspace sharing the parent's
  underlying repository directory, like a git worktree), and `branch` (new isolated workspace cloned or branched from
  the parent). Use `share` or `branch` for arena candidates and isolated packets, not to parallelize tasks that write
  the same files. See **Concurrency and overlap**.
- **Continuation:** `send_message` with `Recipient` set to the subagent's `conversationId` and `Message` sends a
  follow-up or steering update to an existing subagent without launching a new one.
- **Orchestration (`swarm` and `run_workflow`):** `swarm` coordinates named workers on a shared message board (`action`:
  `status`, `models`, `add`, `model`, `kill`, `title`, with `workspace` accepting `inherit`, `share`, or `branch`).
  Prefer `swarm` when workers need shared-board coordination, and `invoke_subagent` for bounded tasks. `run_workflow`
  runs a Python orchestration script (`Script` or `ScriptPath`) that fans out across many independent or staged pieces
  of agent work; use it when orchestration itself is code, and `invoke_subagent` for a single delegated task.
- **Tool-name equivalents:** skill text uses Claude Code tool names. Where present in the session (detect by presence):
  `Read` → `read_file`, `Edit` → `edit_file`, `Write` → `write_file`, `Bash` → `run_command` (with `manage_task` for
  background tasks), `WebSearch` → `search_web`, `AskUserQuestion` → `ask_question`, `Agent` → `invoke_subagent` (with
  `define_subagent`, `send_message`, `manage_subagents`), `Workflow` → `run_workflow`, and agent teams → `swarm`.
  Dedicated `Glob` and `Grep` tools may be absent; detect them by presence or use shell commands through `run_command`.

## Any other surface (generic fallback)

These rules apply when dispatch is refused or the dispatch tool is missing. A reached concurrency limit is not a
refusal. It follows **Concurrency and overlap**.

- **One declared fallback.** Every dispatching skill declares exactly one fallback: a labelled in-session pass, or a
  stop that names what can still run. `execute` declares a stop: it tells the user `pair` is the only mode that runs
  here. It never selects `pair` on the user's behalf.
- **Label a self-read.** When a required independent review runs as a self-read, the record and the approval question
  both say `self-read (not independent)`.
- **A self-read does not earn a skip.** A later step that skips a check because an independent review ran does not skip
  it after a self-read.

## Tool names

Skill text uses Claude Code tool names. The translation tables are in [copilot-tools.md](copilot-tools.md). A surface
with no table detects tools by presence and never uses another surface's table.

## Cross-surface gotchas

- **Write every prompt as if the subagent is stateless.** Every prompt must be self-contained: objective, file
  boundaries, acceptance criteria, and reporting format.
- **No file locking anywhere.** Ownership boundaries from Step 0 are the only collision protection any surface provides.
- **Model naming differs; intent doesn't.** The four tiers are Light, Standard, Deep and Max.
  [model-ladders.md](model-ladders.md) says how each is bound on each surface. Expect some surfaces to silently
  downgrade out-of-tier requests.
- **Verification is surface-independent.** A subagent's "done" is a report, not a fact. The controller re-verifies
  integrated results itself, on every surface.

## Continuing a reviewer for a delta pass

`auto`'s integration review re-reviews with the first-pass agent, continued with the fix diff. It does not use a
cold-started reviewer that re-reads the whole group. The continuation path is native. It differs per surface:

| Tools present in session              | Continue the same agent with                                                                                                                                                                                                                              |
| ------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `Agent` (Claude Code)                 | `SendMessage` to the completed reviewer's agent ID or name; no agent teams needed. With agent teams, the same call steers a teammate. Explore and Plan return no ID: dispatch as `general-purpose` or custom                                              |
| `task` (Copilot CLI)                  | Dispatched with `mode: "background"`: `write_agent` to the reviewer's `agent_id`, then `read_agent` with `wait` set for the result. Any other dispatch, `sync` mode included: **Re-dispatch fallback.** Re-dispatch with the prior findings and the delta |
| `runSubagent` (VS Code Local harness) | No continuation: each invocation is stateless (source-confirmed 2026-11-19). Unverified: the result returns synchronously. Re-dispatch with prior findings and the delta                                                                                  |
| `spawn_agent` (Codex)                 | `send_input` to the live agent; for a closed one, `resume_agent` first, then `send_input`; then `wait_agent`                                                                                                                                              |
| `invoke_subagent` (Antigravity)       | `send_message` to the reviewer's `conversationId` with the fix diff; no fresh subagent needed                                                                                                                                                             |
| None of the above                     | The review ran in-session; continue in-session with the delta                                                                                                                                                                                             |

Name the path in the boundary report. Say explicitly when a surface forced a re-dispatch instead of a continuation.

**On Copilot CLI, only a background agent can be continued.**

A reviewer dispatched with `mode: "background"` is continued with `write_agent`, addressed by the `agent_id` from its
`task` result or notification. Its result is read with `read_agent` with `wait` set, because `read_agent` otherwise
returns the current status. `write_agent` supports background agents only, so a reviewer from any other dispatch, `sync`
mode included, cannot be continued. Take the re-dispatch fallback for it rather than invent an operation. Plan and
configuration limits apply, so feature-detect `write_agent` and `read_agent`. When either is absent, re-dispatch.

If a future release exposes an operation that continues a `sync` agent in the session's tool inventory, do these steps:

- Feature-detect it per **Feature-detect, don't surface-detect**.
- Update this row with the operation's real name.
