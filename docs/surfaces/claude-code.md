# Claude Code Surface

This page documents Agentics's Claude Code marketplace, plugin, hooks, and orchestration behavior.

## Configuration

Claude Code uses the Claude marketplace catalog, plugin manifest, and hook config:

```text
.claude-plugin/marketplace.json
.claude-plugin/plugin.json
hooks/hooks.claude.json
```

`.claude-plugin/marketplace.json` is the marketplace discovery path; `.claude-plugin/plugin.json` registers all 26
skills. It also registers the Claude hook config at `./hooks/hooks.claude.json` explicitly. The provider-qualified
filename aligns Claude Code with the Copilot and Codex hook adapters while keeping the shipped hook surface visible to
consumers.

Invoke an installed Agentics skill as `/agentics:<skill>`.

## Packaged `bin/`

Beside `skills/` and `hooks/`, the plugin packages a top-level `bin/` with the two `agx` launchers, `bin/agx` (POSIX
`sh`) and `bin/agx.exe` (native Windows). Claude Code puts a plugin's `bin/` on the agent's `PATH`, so skills call bare
`agx flow ...` here. The hook config does not use this route; it calls `run-hook.cmd` by its absolute path.

Verified on Claude Code 2.1.322 (2026-11-12), Windows 11 x64, with the plugin installed from this revision: bare
`agx flow status` ran through the real launchers in both the Bash and the PowerShell tool. A throwaway-plugin probe on
the same version on 2026-11-11 gave the same result: the bare name ran `bin/agx-probe` from the Bash tool and
`bin/agx-probe.cmd` from the PowerShell tool. macOS and Linux were not probed.
[ADR 0023](../adr/0023-agx-is-the-agentics-command-line-tool.md) records the probe.

## Validation

Claude Code provides a native manifest validator. It is the only provider-native validation check Agentics currently
configures in CI:

```bash
claude plugin validate . --strict
```

`--strict` promotes warnings to errors, so it catches placement mistakes a plain run tolerates. For example, a
`category` field on `plugin.json` instead of the marketplace entry is ignored at load time but fails here.

Verified on Claude Code 2.1.314 (2026-10-30): `claude plugin validate . --strict` exited 0 against the manifests at this
revision. Retained historical probes: Claude Code 2.1.280 (2026-09-24) and 2.1.220 (2026-08-09) also passed clean.

## Install and discover

Use the absolute package path printed by the [verified setup guide](../guide/01-setup.md#verify-and-unpack). Register
the marketplace, then install its plugin.

```bash
claude plugin marketplace add "/path/to/verified/agentics-plugin-v1.230.0"
claude plugin install agentics@muehz
```

`marketplace add` takes a GitHub repo, a URL, or a local path, and declares it at `--scope user` (default), `project`,
or `local`. `install` takes the same `--scope` flag, so `agentics@muehz` can be user-wide or pinned to one project.

Verify what landed:

```bash
claude plugin details agentics@muehz
claude plugin list --json
```

`details` prints the component inventory and projected token cost. `list --json` is the scriptable form; add
`--available` to include marketplace plugins that are not installed.

Historical probe record. This is not a current guarantee. On Claude Code 2.1.220 (2026-08-09), `marketplace add` +
`install` into an isolated `CLAUDE_CONFIG_DIR` registered 25 skills, and `details` reported `Skills (25)`.

In-session, `/plugin` opens the plugin manager with four panes: **Discover** (marketplace plugins available to install),
**Installed**, **Marketplaces** (configured sources), and **Errors** (manifests that failed to load. Check here first
when Agentics is installed but its skills do not appear).

For local development, [`npm run plugin:setup`](../../CONTRIBUTING.md#local-setup) builds and registers a stage from the
current checkout. `claude --plugin-dir <package-root>` loads an existing stage or verified package for one session.

## Enable, disable, and refresh

```bash
claude plugin disable agentics@muehz
claude plugin enable agentics@muehz
```

Disabling keeps the plugin installed but stops loading its components. Use it to bisect whether Agentics is responsible
for a behavior.

Install snapshots the plugin tree. To upgrade releases, verify a new package and follow the
[update guide](../guide/10-update-and-troubleshoot.md#claude-code). For a clean reinstall from the same registered
package, use these commands in the scope of your installation.

```bash
claude plugin uninstall agentics@muehz --keep-data --scope user
claude plugin install agentics@muehz --scope user
```

Historical probe record. On Claude Code 2.1.220 (2026-08-09), `disable` → `enable` moved `enabled` true → false → true
in `plugin list --json`, and the uninstall/reinstall cycle emptied then restored the installed set.

`--keep-data` preserves `~/.claude/plugins/data/{id}/` across the cycle. In an active session, `/reload-plugins` can
activate updated plugin components. Only monitor changes require a restart, and Agentics ships no monitors.

## Live check

Build the checkout you intend to test with `npm run plugin:setup`. Start Claude with `claude --plugin-dir <stage-path>`
using the path shown by `npm run plugin:status`.

1. Ask Agentics to route a small task. Confirm that routing selects a workflow.
2. Run the **Live check** on the [command guardian](../../hooks/README.md#command-guardian-live-check),
   [release guardian](../../hooks/README.md#release-guardian-live-check), and
   [secrets guardian](../../hooks/README.md#secrets-guardian-live-check).
3. Open `/mcp` in the running session and inspect the configured servers. Agentics should add none. Use an isolated
   Claude profile when practical; otherwise distinguish pre-existing or user-configured servers from anything the plugin
   supplied.

Record the host version and what happened. A manifest validation or install listing doesn't prove that a prompt or hook
ran.

## PR-comment coding agents

Claude has two PR-comment integrations with different effects. By default, a user with repository write access can use
`@claude` in an issue or pull-request comment when the repository configures `anthropics/claude-code-action`. Claude can
then analyze code, implement changes, and push commits. The workflow grants the repository checkout and GitHub
permissions that make those changes possible. See
[Claude Code GitHub Actions](https://code.claude.com/docs/en/github-actions), read 2026-09-01.

The managed Claude Code Review service is a reviewer, not the commit-pushing path. In an enabled repository, an
authorized collaborator can use a top-level `@claude review` or `@claude review once` comment on an open pull request.
The review posts findings on the pull request. Replies to inline findings do not prompt Claude to change the branch. See
[Code Review](https://code.claude.com/docs/en/code-review), read 2026-09-01. Both integrations overlap with `triage` on
review feedback, but Agentics does not route or respond to `@claude` comments. The user chooses which workflow handles
them.

These integrations have repository and pull-request context, not the conversation from an active Claude Code session.
The GitHub Action runs Claude inside a GitHub Actions workflow with a fresh checkout and the triggering comment. Managed
Code Review analyzes the pull request against the repository and reads `CLAUDE.md` and `REVIEW.md`. Its page doesn't
name `AGENTS.md`, and Agentics keeps no `CLAUDE.md`, so don't assume this repository's instructions reach it. Neither
integration is documented as a continuation of a local Claude Code conversation. See
[Claude Code GitHub Actions](https://code.claude.com/docs/en/github-actions), read 2026-09-01, and
[Customize Code Review](https://code.claude.com/docs/en/code-review-customize), read 2026-10-30.

Repository instructions are the shared context boundary. Claude Code v2.1.277 and later reads `AGENTS.md` directly when
the repository has no `CLAUDE.md`, `.claude/CLAUDE.md`, or `CLAUDE.local.md`; a `CLAUDE.md` makes it read that file
instead. Agentics keeps no `CLAUDE.md` for that reason. Older versions, a disabled built-in
`cc-plugin-agents-md@builtin` plugin (`agents-md@builtin` before v2.1.285), or the first session after an upgrade read
`CLAUDE.md` only. See [How Claude remembers your project](https://code.claude.com/docs/en/memory), read 2026-10-30. The
Copilot cloud agent and Copilot CLI read `AGENTS.md`, and Codex reads it directly. See
[Get the best results with Copilot cloud agent](https://docs.github.com/en/copilot/tutorials/cloud-agent/get-the-best-results),
the
[Copilot CLI command reference](https://docs.github.com/en/copilot/reference/copilot-cli-reference/cli-command-reference),
and [Custom instructions with AGENTS.md](https://learn.chatgpt.com/docs/agent-configuration/agents-md), read 2026-09-01.
This cross-surface readership is why the chain boundary design assigns its planned work-folder pointer to `AGENTS.md`,
not to a generated skill that a PR-comment session might never load. The pointer is design work, not current content in
this repository's `AGENTS.md`.

## Telemetry export

Claude Code can export OpenTelemetry metrics and log events, plus optional beta traces, to an OTLP collector. Configure
export in the consumer environment with `CLAUDE_CODE_ENABLE_TELEMETRY`, the applicable `OTEL_*_EXPORTER` variables, and
`OTEL_EXPORTER_OTLP_ENDPOINT`. Beta traces additionally require `CLAUDE_CODE_ENHANCED_TELEMETRY_BETA=1` and
`OTEL_TRACES_EXPORTER`; administrators can set the same variables in managed settings. See
[Monitoring](https://code.claude.com/docs/en/monitoring-usage), read 2026-09-01. Agentics ships no exporter and does not
configure a destination.

## Hooks

`hooks/hooks.claude.json` registers one `PreToolUse` callback and none afterward. It starts one Agentics binary for the
selected checks. Claude has 1 command; Codex and Copilot have 2 command alternatives each, for 5 overall:

| Command                             | Event        | Host budget |
| ----------------------------------- | ------------ | ----------- |
| `run-hook.cmd claude-code pre-tool` | `PreToolUse` | 25 seconds  |

The command uses the quoted `${CLAUDE_PLUGIN_ROOT}/hooks/run-hook.cmd` path and accepts no flags. The matcher is
`^(?:Read|Grep|Glob|NotebookRead|Bash|PowerShell|Write|Edit|MultiEdit|NotebookEdit|mcp__.*)$`. It includes MCP tools and
anchors the complete union. See [matcher patterns](https://code.claude.com/docs/en/hooks#matcher-patterns), reviewed
2026-10-14.

Actual normalized input selects the checks, and the guard workers have independent 10-second limits. See
[event routing and failures](../../hooks/README.md#event-routing-and-failures) for order, cleanup, aggregation, and
retained single-check commands.

Current source/config process proofs exercise the real wrapper and source binary after simulated provider selection on
Windows and Linux. They do not establish live Claude scheduling or packaged binary behavior. Native macOS and ARM
acceptance also remain separate. See [workflow policy settings](../../hooks/README.md#settings) for the guard settings
and the planned workflow reminders.

Retained legacy dispatch probe: on Claude Code 2.1.247 on Windows 11 (2026-08-28), Claude Code ran `run-hook.cmd`
through `cmd.exe`, which execs the committed Windows binary; `command-guardian` and `secrets-guardian` produced their
documented outcomes on the Live check. `release-guardian` did not exist yet.

## Hook protocol

What Claude Code does with a command hook's exit code and streams, from the
[Hooks reference](https://code.claude.com/docs/en/hooks), read 2026-10-30. What Agentics emits for each outcome is the
Provider Output Matrix in each guardian section of the hooks README:
[command guardian](../../hooks/README.md#command-guardian-provider-output-matrix),
[release guardian](../../hooks/README.md#release-guardian-provider-output-matrix), and
[secrets guardian](../../hooks/README.md#secrets-guardian-provider-output-matrix).
[ADR-0004](../adr/0004-go-hooks-is-the-reference-implementation.md) makes this section the contract.

| Exit  | `PreToolUse`                                                                                               |
| ----- | ---------------------------------------------------------------------------------------------------------- |
| 0     | No decision unless stdout JSON sets one; stderr goes to the debug log                                      |
| 2     | Blocks the tool call; stderr is the blocking reason when no JSON decision is present                       |
| other | Non-blocking error; the call proceeds and the transcript shows a hook error, with the two exceptions below |

On exit 0, stdout is parsed as JSON when it begins with `{` and ends with `}`. `PreToolUse` reads
`hookSpecificOutput.permissionDecision` (`allow`, `deny`, `ask`, `defer`) with `permissionDecisionReason`, and
`hookSpecificOutput.additionalContext`. Empty stdout is the default behavior. `timeout` is in seconds
(`hooks/hooks.claude.json` sets 25). A hook that reaches it is canceled, its output is discarded, and the tool call
continues through the normal permission flow, except in the two cases below.

A failed hook or timeout blocks in two cases. The first is a hook with `onFailure: "block"` (Claude Code 2.1.295 or
later), which Agentics does not set. The second is a shell-form `PreToolUse` command hook on Windows without Git Bash
when `onFailure` is unset; there, any exit other than 0 or 2 counts as a failure. Agentics has not exercised that host.

One fact shapes the Agentics adapter on this surface. Exit 1 does not block outside those two cases. The legacy
single-check command uses that status for adapter or core faults. Grouped Pre-tool intentionally converts infrastructure
and selected guard faults into exit 2 denials before the outer host timeout.

## Orchestration

Every surface doc answers these five questions in the same order, so surfaces can be compared directly.

### Context

By default, a dispatched subagent starts with fresh isolated context, so prompts for non-fork dispatch must be
self-contained. A fork subagent (`subagent_type: "fork"`) inherits the parent conversation. A skill with `context: fork`
does not: despite the name, it starts without the conversation history. This is a property of dispatch, not a claim that
context never carries. See **Continuation**.

### Continuation

`SendMessage` can resume a completed subagent by its agent ID or name, with its prior conversation history, tool calls,
and results intact; this does not require agent teams. The built-in Explore and Plan agents are one-shot and return no
agent ID, so they cannot be resumed; a subagent that may be continued, such as a reviewer, is dispatched as
`general-purpose` or a custom type. Agent teams is the separate, still-experimental feature
(`CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS=1`) whose teammates stay addressable across turns. Teammates coordinate through
the shared task list only where the task tools (`TaskCreate`/`TaskList`/`TaskUpdate`) are present. They are on by
default only on older models, and agents without them coordinate through messages instead. Without `SendMessage`, a new
`Agent` invocation starts a new subagent. In-process teammates do not survive `/resume` and cannot start background
subagents: their subagents run in the foreground. Nested teams are not supported. Source:
[Subagents](https://code.claude.com/docs/en/sub-agents), [Agent teams](https://code.claude.com/docs/en/agent-teams) and
[Tools reference](https://code.claude.com/docs/en/tools-reference), read 2026-11-19.

### Concurrency and nesting

Agentics dispatches parallel work with one `Agent` call per task, batched in a single message. Subagents can run in the
foreground or background. Fork subagents are on by default in the terminal, `claude -p`, the Agent SDK, the desktop app,
and VS Code (`CLAUDE_CODE_FORK_SUBAGENT=0` turns them off). A subagent can spawn subagents of its own by default, up to
three layers below the main conversation (`CLAUDE_CODE_MAX_SUBAGENT_SPAWN_DEPTH`); at that depth the `Agent` tool is
withheld from an ordinary subagent, while a fork keeps `Agent` in its inherited tool list but the tool errors instead of
spawning. A session also caps concurrent subagents at 20 by default (`CLAUDE_CODE_MAX_CONCURRENT_SUBAGENTS`). Agent
teams are the separate mechanism for workers that must communicate with each other. Agentics still requires explicit
authorization for multi-agent execution. A user-typed `/agentics:execute` invocation is one authorization route; silent
auto-selection of the Agentics skill is not. Source: [Subagents](https://code.claude.com/docs/en/sub-agents), read
2026-10-30.

### Native agent contract

Agentics registers no subagent types of its own. Dispatch uses the `Agent` tool's built-in and user-defined subagent
types, and it is model-invocable. Skills can instruct it directly.

### Capability policy

Agentics encodes no tool permissions; restrictions are host and user settings. Per-call `model` and
`isolation: "worktree"` are available where tasks would touch overlapping files. Agent view (`claude agents`) and
`claude --bg` background sessions are **user-only**. Recommend them, never claim to have run them.

Dispatch guidance skills follow: `skills/routing/references/surface-dispatch.md`.
