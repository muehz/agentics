# VS Code (Copilot Chat) Surface

Register a verified Agentics package in your VS Code user settings, then check what the active harness loads.

Verified on VS Code 1.140.0 (2026-11-23) on Windows x64. The CLI accepted isolated `--user-data-dir` and
`--extensions-dir` arguments and returned an empty extension list with `--list-extensions`. Use the live check below to
verify Agent Plugins loading and dispatch.

Provider behavior in this document was source-reviewed against the current official documentation and the release notes
through 1.144 on 2026-10-30. None of it is Agentics-live-confirmed unless stated otherwise. Agentics makes no claim of
behavioral parity with the CLI surfaces. The host controls Agent Plugins behavior, settings, and organization policy
independently.

When VS Code exposes an installed Agentics skill, invoke it as `/agentics:<skill>`.

## Two harnesses

VS Code runs agent sessions on one of two harnesses, and the answer to most "how does Agentics behave here" questions
depends on which one a session uses. Since 1.143 the Copilot harness is the default for new sessions, after a
progressive rollout in 1.142.

| Question                   | Copilot harness                                                                          | Local harness                                               |
| -------------------------- | ---------------------------------------------------------------------------------------- | ----------------------------------------------------------- |
| What runs the session      | The Copilot SDK, in VS Code's Agent Host, the harness Copilot CLI uses                   | VS Code's built-in agent harness                            |
| Lifetime                   | Runs independently of the editor window and continues after it closes                    | Tied to the editor window; closing it stops the agent       |
| Models                     | Per the Copilot harness; not every built-in or extension tool is there                   | The models configured in VS Code                            |
| Subagents, context, effort | The native `task` tool; other controls unconfirmed; see [copilot-cli.md](copilot-cli.md) | The `agent/runSubagent` contract in **Orchestration** below |
| Hooks                      | Configuration and payloads depend on the harness                                         | SDK policy hooks are not loaded                             |
| Customization settings     | Their own set                                                                            | A different set from the Copilot harness                    |

Source-confirmed against VS Code's agents and agent plugins pages. Agentics has not confirmed which skills or hooks load
in either harness. The rest of this page is a Local-harness description unless it says otherwise, so most of it covers
the non-default case.

The maintainer reported startup working in VS Code Local during compatible preparation. That report does not identify a
verified release archive or cover the Copilot harness.

## Configuration

Use the absolute package path printed by the [verified setup guide](../guide/01-setup.md#verify-and-unpack). Open
**Preferences: Open User Settings (JSON)**, enable Agent Plugins, and add the package path to `chat.pluginLocations`.
Keep existing entries in that object.

```json
{
    "chat.plugins.enabled": true,
    "chat.pluginLocations": {
        "/path/to/verified/agentics-plugin-v1.230.0": true
    }
}
```

On Windows, use a JSON path with escaped backslashes, such as `C:\\Agentics\\verified\\agentics-plugin-v1.230.0`. Reload
VS Code, then follow the live check below. Organization policy can control Agent Plugins and allowed sources.

### Installing through Copilot CLI instead

VS Code documents that it automatically discovers plugins installed with the GitHub Copilot CLI, which keeps them under
`~/.copilot/installed-plugins/`, and that a Copilot-harness session accepts `/plugin install <source>`. If you install
Agentics that way (see [copilot-cli.md](copilot-cli.md)), confirm what VS Code actually discovers. Agentics has not
exercised this route in VS Code.

## What a settings edit does not establish

Registration is a static settings fact. It does not establish that VS Code loaded Agentics, that the plugin is enabled
at runtime, or that organization policy allows it. `chat.plugins.enabled` is organization-manageable, and reading a
settings file cannot establish runtime activation.

This has a direct consequence for freshness stamps. **Effective enablement can never carry a `Verified on` stamp from
settings evidence.** A stamp records a claim that was exercised, and no amount of settings inspection exercises runtime
activation. Establishing enablement needs the live check below.

## Live check

Settings alone cannot prove that VS Code loaded Agentics. After changing VS Code packaging:

1. In a disposable VS Code profile when practical, record the configured MCP servers before installing Agentics.
2. Verify a release archive and add its unpacked package path to the user settings above.
3. Reload VS Code.
4. In Copilot Chat, confirm that Agentics is discoverable and invoke one Agentics skill. Do this once in a Local-harness
   session and once in a Copilot-harness session, and record which harness each used.
5. Inspect the MCP server list again. Agentics should add none.
6. Delete the package entry from `chat.pluginLocations` and confirm the local registration is gone.

In Insiders, open that editor's user settings. Record the VS Code version and the invocation result. Remove the local
registration even if the runtime check fails.

## Orchestration

Every surface doc answers these five questions in the same order. This lets readers compare them directly. The answers
below describe the **Local harness** and its `agent/runSubagent` tool. A Copilot-harness session follows
[copilot-cli.md](copilot-cli.md) instead, and Agentics has not confirmed that VS Code exposes every Copilot CLI control
there. **Nothing below is established by the settings edit. Registration is a static fact; it does not establish plugin
discovery, loading, or dispatch.**

### Context

Source-confirmed. A dispatched subagent receives its own instruction and does not inherit the parent conversation. The
retained evidence does not establish this behavior for Agentics.

### Continuation

Source-confirmed. Each invocation is stateless and returns its final result to the main agent. The main agent cannot
send follow-up messages to the same subagent, so write self-contained prompts.

### Concurrency and nesting

VS Code's docs say "Delegate independent work in parallel only when the tasks don't depend on each other's results."
They don't confirm concurrent runs for Local. Source-confirmed: nested delegation is off by default through
`chat.subagents.allowInvocationsFromSubagents`, with depth ≤5. A requested model cannot exceed the session's cost tier.
The retained evidence does not establish these behaviors for Agentics.

### Native agent contract

Source-confirmed. Dispatch uses the `agent/runSubagent` tool. A custom agent's `agents:` frontmatter can restrict which
subagents it may dispatch; Copilot-harness sessions use `task` instead and don't accept `agents:`. If subagent dispatch
appears unavailable, first confirm that `agent/runSubagent` is enabled, then check the allowlist. The retained evidence
has no available UI probe, so it does not establish Agentics skill registration or invocation.

### Capability policy

Source-confirmed: cost-tier capping bounds requested models, and the Local harness uses the models configured in VS
Code. Agentics policy resolves Copilot and VS Code dispatches by role and work class, per
`skills/routing/references/model-ladders.md`, which also holds the fallback chains. A first clear explicit model,
effort, or tier pin takes precedence immediately at its stated scope, including a supported below-default or
out-of-table model. Organization policy can disable `chat.plugins.enabled` centrally, which is why `--status` reports
enablement as `unknown` rather than asserting it. In the Local harness, `runSubagent` has no verified per-dispatch
effort or context control; bind the model where accepted and disclose any limitation. Agentics has not verified which
Copilot CLI controls a Copilot-harness session in VS Code exposes. The role mapping is Agentics policy, not a current
availability or live-binding claim.

## Hooks

Agentics local setup writes no VS Code-specific hook setting or configuration, and Agentics has no VS Code-dedicated
hook adapter. No `surface:"vscode"` stamp is written to captured records. VS Code itself supports plugin hooks, and
their configuration and payloads depend on the session's harness; SDK policy hooks are not loaded in Local-harness
sessions. That host capability does not establish that Agentics's hooks are loaded or exercised by this setup. Agentics
has no root `plugin.json`, so VS Code's manifest lookup would select `.claude-plugin/plugin.json`, whose `hooks` field
names the Claude config. VS Code tells authors to use "the hook schema supported by the plugin's target runtime", and
what happens to that config in a Copilot-harness session is unobserved.

## Reversibility

Delete the Agentics package entry from `chat.pluginLocations` and reload. Keep other plugin locations and Agent Plugins
settings that you use for other packages.

## Not Established by Retained Evidence

The retained record does not establish skill invocation end-to-end; plugin discovery inside a live Chat session; which
harness a session used or whether Agentics's skills and hooks load in each; whether organization policy blocked
enablement on the probed account; `agent/runSubagent` dispatch, nesting, or cost-tier behavior in practice;
project-scope settings; the Agent Host's behavior; whether a plugin installed through Copilot CLI loads in VS Code;
whether the `muehz/agentics` marketplace entry resolves now that the repository carries its catalog under
`.claude-plugin/marketplace.json`; or behavior on other VS Code versions and platforms. Use the live check to establish
runtime activation.

## Packaged Assets

- Skills - the canonical `skills/` tree is registered for discovery through the plugin location; the retained evidence
  does not prove that VS Code loaded them.
- Hooks - local setup writes no VS Code-specific hook configuration.
- MCP - none packaged or auto-registered.
