# Copilot CLI Surface

Copilot CLI supports Agentics through native Copilot plugin metadata and hook configuration.

## Configuration

Agentics's native Copilot files are:

```text
.github/plugin/plugin.json
hooks/hooks.copilot.json
```

Copilot CLI resolves plugin manifests in first-found order:

1. `.plugin/plugin.json`
2. `plugin.json`
3. `.github/plugin/plugin.json`
4. `.claude-plugin/plugin.json`

`.github/plugin/plugin.json` is the sole canonical Copilot component manifest. The two higher-precedence candidates are
absent, so Copilot resolves it. Copilot CLI reads the marketplace from `.claude-plugin/marketplace.json`, whose Agentics
source is `./`. The lower-precedence `.claude-plugin/plugin.json` registers components for Claude-compatible hosts;
first-found-wins keeps it out of Copilot's path. Keeping one Copilot component source of truth prevents manifest drift.

The Copilot manifest registers all 26 Agentics skills. The manifest declares no agents.

The plugin also packages a top-level `bin/` with the two `agx` launchers, `bin/agx` (POSIX `sh`) and `bin/agx.exe`
(native Windows). Copilot CLI does not put a plugin's `bin/` on the agent's `PATH`, so bare `agx` is not found and
skills use the launcher-path fallback that [`flow-state.md`](../../skills/execute/references/flow-commands.md#commands)
states, such as `& "<plugin-root>/bin/agx.exe" flow status` from PowerShell on Windows.

Verified on Copilot CLI 1.0.109 (2026-11-12), Windows 11 x64, with the plugin installed from this revision: PowerShell
reported bare `agx` as not recognized, and `& "<plugin-root>/bin/agx.exe" flow status` printed the status. A
throwaway-plugin probe on Copilot CLI 1.0.98 on 2026-11-11 gave the same result.
[ADR 0023](../adr/0023-agx-is-the-agentics-command-line-tool.md) records the probe.

Invoke an installed Agentics skill as `/agentics:<skill>`.

> **The exclusion works, but `install` output does not show it.** Verified on Copilot CLI 1.0.78 (2026-08-09,
> historical: superseded by the live-load record below) in an isolated `COPILOT_HOME` showed that the install reports
> `Installed 25 skills` and `skills/rubber-duck/` **is** present under `installed-plugins/muehz/agentics/`, because the
> `"./"` source copied the whole repository tree at the time. Registration is a separate step from copying, and it
> honors the manifest. `copilot skill list --json` returned 24 Agentics plugin skills with `rubber-duck` absent.
>
> So there were two different numbers, and only one of them was the effective roster. The install message counted
> **directories placed on disk**, while the manifest's `skills` array governs **what registers**. Presence in an install
> tree is not evidence a skill is active. Use `copilot skill list --json` to check the effective roster; never infer it
> from the install summary or from `find`.
>
> **Copilot CLI 1.0.81 changed how a local-path marketplace source installs**, per the
> [1.0.81 release notes](https://github.com/github/copilot-cli/releases/tag/v1.0.81): "Path-sourced plugins in a local
> (directory-source) marketplace now load live from their real directory... no `/plugin update`." Verified on Copilot
> CLI 1.0.88 (2026-09-24) in an isolated `COPILOT_HOME`/`COPILOT_CACHE_HOME`: `copilot plugin install agentics@muehz`
> reports `Installed 34 skills` (the repository's on-disk `skills/` directory count, `peer` included, since nothing is
> copied) and prints
> `Enabled "agentics@muehz". It is loaded live from <repo path>, so edits take effect on the next session — nothing was copied.`.
> `copilot plugin list --json` shows `"source": "live"` with `"installedFrom": "<repo path>"`. **No
> `installed-plugins/muehz/agentics/` directory is created** for this local-path source on 1.0.88, so the copied-tree
> record above is historical. `copilot skill list --json` still returns exactly 33 Agentics plugin skills with `peer`
> absent (plus 2 Copilot built-in skills). The effective roster is unaffected.

> `peer` has since been removed (#1156), so the counts and exclusion notes in this record are historical.

`hooks/hooks.copilot.json` uses Copilot's native hook shape:

- `version: 1`
- lower-camel event names such as `preToolUse`
- `bash` and `powershell` command fields
- `${COPILOT_PLUGIN_ROOT}` / `$env:COPILOT_PLUGIN_ROOT` for plugin-relative hook dispatch

Agentics registers one `preToolUse` callback and none afterward in `hooks/hooks.copilot.json`. The command is
`run-hook.cmd copilot pre-tool`, with no flags. It has `bash` and `powershell` alternatives, yielding 2 alternatives
here and 5 across all three providers. The event starts one Agentics binary for its selected logical checks.

Pre uses matcher `.*` with `timeoutSec: 25`. See
[matcher filtering](https://docs.github.com/en/copilot/reference/hooks-reference#matcher-filtering), reviewed
2026-10-14.

Actual normalized input selects the checks, and the guards have separate 10-second worker limits. See
[event routing and failures](../../hooks/README.md#event-routing-and-failures) for order, cleanup, aggregation, and
retained single-check commands.

Source/config process proofs on Windows and Linux simulate provider selection, then run the real wrapper and source
binary. They establish source behavior only. They do not establish packaged binary or live-host acceptance. Newly
packaged macOS and ARM checks remain pending. Default-on SessionStart, opt-in Stop, and the PreToolUse commit reminder
remain held for #1101; no placeholders are installed. Workflow policy settings are described in
[hooks/README.md](../../hooks/README.md#settings).

### Agent Plugins 1.0

Copilot CLI also documents a second manifest format: Agent Plugins 1.0. A `plugin.json` opts in by declaring the
canonical `"$schema": "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json"`, and the
[plugin reference](https://docs.github.com/en/copilot/reference/copilot-cli-reference/cli-plugin-reference), read
2026-10-30, also recognizes the v1.1.0 URL. In Copilot CLI, "a manifest without the Agent Plugins `$schema` uses this
format", meaning the legacy format this page describes. See
[Creating plugins for Copilot CLI](https://docs.github.com/en/copilot/how-tos/copilot-cli/customize-copilot/plugins-creating),
read 2026-10-30.

`.github/plugin/plugin.json` declares no `$schema`, so Copilot CLI loads it in the legacy format. Agentics does not
migrate to Agent Plugins 1.0, for two reasons:

- The 1.0 top-level schema allows only `$schema`, `name`, `version`, `description`, `author`, `homepage`, `repository`,
  `license`, `keywords`, and `extensions`. It removes the explicit component lists (`agents`, `skills`, `hooks`) the
  legacy format uses to register components.
- Copilot-specific components move to a `com.github.copilot/` directory at the plugin root:
  `com.github.copilot/hooks/hooks.json` for hooks and `com.github.copilot/agents/` for custom agents, which Agentics
  does not ship. Agentics's Copilot hook config lives at `hooks/hooks.copilot.json`, beside the other providers'
  configs; adopting 1.0 would require a second copy under `com.github.copilot/hooks/` plus a sync check to keep the two
  from drifting.

Agent Plugins 1.0 also loads every immediate subdirectory of `skills/` that contains a `SKILL.md`.

Revisit this decision if GitHub publishes a deprecation or end date for the legacy format, if Agent Plugins 1.0 exposes
a capability the legacy format lacks, or if 1.0 gains configurable component paths.

## Hook protocol

What Copilot CLI does with a command hook's exit code and streams, from the
[hooks reference](https://docs.github.com/en/copilot/reference/hooks-reference) and
[Using hooks with Copilot CLI](https://docs.github.com/en/copilot/how-tos/copilot-cli/customize-copilot/use-hooks), read
2026-09-24. What Agentics emits for each outcome is the Provider Output Matrix in each guardian section of the hooks
README: [command guardian](../../hooks/README.md#command-guardian-provider-output-matrix),
[release guardian](../../hooks/README.md#release-guardian-provider-output-matrix), and
[secrets guardian](../../hooks/README.md#secrets-guardian-provider-output-matrix).
[ADR-0004](../adr/0004-go-hooks-is-the-reference-implementation.md) makes this section the contract.

| Exit  | `preToolUse`                                                   |
| ----- | -------------------------------------------------------------- |
| 0     | stdout JSON is the hook output; empty stdout is the default    |
| 2     | Denied, even if stdout JSON says `permissionDecision: "allow"` |
| other | Denied with `Denied by preToolUse hook (hook errored)`         |

`preToolUse` output is a flat object: `permissionDecision` (`allow`, `deny`, `ask`) with `permissionDecisionReason`,
required on `deny`. `timeoutSec` defaults to 30 (`hooks/hooks.copilot.json` sets 25). Host timeouts are fail-open for
every event, `preToolUse` included: a warning is surfaced and processing proceeds as if the hook had not run.

The [hooks reference](https://docs.github.com/en/copilot/reference/hooks-reference), read 2026-10-30, states that
`permissionDecision` controls whether the tool executes and that empty output uses the default behavior. It defines
`additionalContext` for other events, such as `sessionStart` and `postToolUseFailure`, which Agentics does not register,
but not for `preToolUse`. Agentics therefore emits no document for a neutral `context` outcome on `preToolUse`, which
leaves the normal permission flow intact. Copilot `preToolUse` has no advisory channel, so the legacy single-check
advisory is not surfaced on this event. Grouped Pre-tool retains warning context on stderr without fabricating an allow
document. This preserves the normal permission flow; stderr is not a documented model-context channel.

Two facts shape the Agentics adapter on this surface. Any non-zero exit on `preToolUse` is a deny, so a deny is a
`permissionDecision: "deny"` document on exit 0, and an adapter or core fault exits 0 with its diagnostic on stderr so
the legacy single-check command stays fail-open rather than becoming a deny the host attributes to an errored hook.
Grouped Pre-tool intentionally converts selected guard errors, timeouts, and panics into a deny document. A hook that
overruns its host timeout is skipped with a warning, so hook latency on this surface is a guardrail property, not a
performance nicety.

Copilot CLI names its tools in lowercase, so the adapter maps them into the neutral vocabulary the hook cores reason
about before a core sees the event. Lookup is exact — Copilot emits these spellings — and a name not in the table passes
through unchanged.

| Copilot tool                                | Neutral name      |
| ------------------------------------------- | ----------------- |
| `bash`, `powershell`                        | `Bash`            |
| `view`                                      | `Read`            |
| `create`                                    | `Write`           |
| `edit`, `str_replace_editor`, `apply_patch` | `Edit`            |
| `grep`, `rg`                                | `Grep`            |
| `glob`                                      | `Glob`            |
| `web_fetch`                                 | `WebFetch`        |
| `web_search`                                | `WebSearch`       |
| `ask_user`                                  | `AskUserQuestion` |
| `update_todo`                               | `TodoWrite`       |
| `task`                                      | `Agent`           |

## No native validator

Copilot CLI ships **no native manifest validator**. There is no `copilot plugin validate`. A successful install is not
validation. It proves the manifest parsed and resolved, not that it is correct.

Agentics requires its own semantic checks for repository validation on this surface:

```bash
npm run plugin:validate
```

That run covers manifest schemas, the Copilot precedence invariant above, and Copilot/Claude component parity. Those are
the invariants a validator would otherwise enforce.

## Install and discover

Current GitHub documentation calls `copilot plugins` a legacy alias for `copilot plugin`; both resolve to the same
command. This page uses the singular form for Agentics install and marketplace commands. Copilot CLI 1.0.85 removed the
cross-kind `--kind`, `--scope`, `--mcp`, and `--skill` flags from `copilot plugins`; since that release
`copilot plugins list` is an alias of `copilot plugin list` and reports only plugins. Use `copilot mcp list` and
`copilot skill list` directly for those other inventories.

| Operation                                          | Current command                                                            |
| -------------------------------------------------- | -------------------------------------------------------------------------- |
| Install, list, update, or uninstall a plugin       | `copilot plugin install` / `list` / `update` / `uninstall`                 |
| Enable or disable a plugin                         | `copilot plugin enable` / `disable`                                        |
| Add, browse, list, update, or remove a marketplace | `copilot plugin marketplace add` / `browse` / `list` / `update` / `remove` |
| List MCP servers or skills directly                | `copilot mcp list` / `copilot skill list`                                  |

> On Copilot CLI 1.0.78 (2026-08-09), the plural `copilot plugins` group printed complete help and then returned
> `The plugins command is not available.` with exit status 0. The singular group worked, but did not expose `enable` or
> `disable`. This records a version-specific runtime mismatch; it predates the 1.0.85 alias behavior above and is not
> the current command contract.

Use the absolute package path printed by the [verified setup guide](../guide/01-setup.md#verify-and-unpack). Register
the marketplace, then install.

```bash
copilot plugin marketplace add "/path/to/verified/agentics-plugin-v1.230.0"
copilot plugin marketplace browse muehz
copilot plugin install agentics@muehz
```

`browse` lists the marketplace's catalog and is the quickest confirmation that `.claude-plugin/marketplace.json`
resolved. Copilot's docs say it also reads `.claude-plugin/`; a hosted add of this repository after #1175 has not been
observed. Note that `awesome-copilot` is registered by default (`copilot-plugins` is now an ordinary marketplace you add
yourself), so `muehz` is an addition to a non-empty set. Always qualify installs as `agentics@muehz`.

For a development checkout, use [`npm run plugin:setup`](../../CONTRIBUTING.md#local-setup) to generate and register the
staging directory.

Inspect what is configured:

```bash
copilot plugin list
```

`copilot plugin list --json` (and its `copilot plugins list --json` alias) emits a flat array of plugin objects; it does
not separate user-scoped from organization-supplied resources. Custom agents and session-scoped hooks still require a
live session for inspection.

In-session, `/plugin` opens the dashboard interactively. Copilot CLI 1.0.81 removed the `/plugins` command; its
resources moved to `/plugin`, `/mcp`, and `/skills`. See the
[1.0.81 release notes](https://github.com/github/copilot-cli/releases/tag/v1.0.81).

Verified on Copilot CLI 1.0.78 (2026-08-09, historical): into an isolated `COPILOT_HOME`, `marketplace add` +
`marketplace browse muehz` listed Agentics with its description, `plugin install agentics@muehz` succeeded (25 skills),
and `plugin list` showed `agentics@muehz (v1.2.0)`. The `/plugins` dashboard itself was not exercised then, and that
command no longer exists.

Verified on Copilot CLI 1.0.88 (2026-09-24, historical): into a fresh isolated `COPILOT_HOME`/`COPILOT_CACHE_HOME`,
`marketplace add` + `marketplace browse muehz` lists Agentics with its description, `plugin install agentics@muehz`
succeeds (`Installed 34 skills`, see the live-load record above), and `plugin list` shows
`agentics@muehz (v1.128.0) (enabled)` with `from <repo path>` instead of a copied-tree path. `plugin list --json` and
`plugins list --json` returned identical flat arrays. The `/plugin` dashboard itself was not exercised; it needs an
authenticated interactive session.

Verified on Copilot CLI 1.0.98 (2026-10-24): a local-directory marketplace add of a tree with no
`.github/plugin/marketplace.json` installed 26 skills.

Verified on Copilot CLI 1.0.98 (2026-10-30): into a fresh isolated `COPILOT_HOME`/`COPILOT_CACHE_HOME`,
`plugin marketplace list` showed only `awesome-copilot` before the add. `marketplace add` + `marketplace browse muehz`
listed Agentics, `plugin install agentics@muehz` installed 26 skills live from the checkout, and `skill list --json`
reported 26 plugin skills. A hosted add of `muehz/agentics` and the `/plugin` dashboard were not exercised.

## Enable, disable, and refresh

```bash
copilot plugin list
copilot plugin disable agentics@muehz
copilot plugin enable agentics@muehz
copilot plugin uninstall agentics
copilot plugin install agentics@muehz
```

Current GitHub examples identify plugins by their bare installed name, and the `/plugin update` slash form documents an
optional `@MARKETPLACE` qualifier. `uninstall` also accepts the bare plugin name, as shown above. Enablement changes
apply to future sessions; start a new CLI session afterwards so the effective components reload.

Use the [update guide](../guide/10-update-and-troubleshoot.md#copilot-cli) to switch the marketplace to a newly verified
release package.

## Enterprise onboarding

Give managed clients the verified release package through your organization's distribution channel. Have the
administrator check which marketplace sources the organization's policy allows before clients register the local
package. The `strictKnownMarketplaces` policy controls the allowlist, and `enabledPlugins` controls organization-managed
plugin enablement. Check those policies when Agentics is missing from a managed client's inventory.

## PR-comment coding agent

A user with repository write access can mention `@copilot` in a comment on any open pull request to start a Copilot
cloud agent session. By default, the agent pushes commits directly to that pull request's branch. It can also resolve
merge conflicts. From a failed GitHub Actions job, **Fix with Copilot** starts the same kind of session, investigates
the failure, and pushes a fix. See
[Using Copilot cloud agent on GitHub](https://docs.github.com/en/copilot/how-tos/use-copilot-agents/cloud-agent/use-cloud-agent-on-github),
read 2026-09-01.

That cloud session has repository and pull-request context, not the conversation from an active Copilot CLI session.
GitHub documents each `@copilot` request in a PR comment as starting a new session and separately says that Copilot
remembers earlier cloud sessions on the same pull request. It does not claim that a cloud session continues a local CLI
conversation. See
[Using Copilot cloud agent on GitHub](https://docs.github.com/en/copilot/how-tos/use-copilot-agents/cloud-agent/use-cloud-agent-on-github),
read 2026-09-01. The agent overlaps with `triage` on review feedback, failed checks, and merge conflicts, but Agentics
does not route or respond to `@copilot` comments. The user chooses which workflow handles them.

Repository instructions are the shared context boundary. The Copilot cloud agent reads `**/AGENTS.md`, and Copilot CLI
loads `AGENTS.md` from the Git root and current working directory. See
[Get the best results with Copilot cloud agent](https://docs.github.com/en/copilot/tutorials/cloud-agent/get-the-best-results)
and the
[Copilot CLI command reference](https://docs.github.com/en/copilot/reference/copilot-cli-reference/cli-command-reference),
read 2026-09-01. Claude Code (v2.1.277 and later) and Codex read `AGENTS.md` directly. See
[How Claude remembers your project](https://code.claude.com/docs/en/memory), read 2026-09-26, and
[Custom instructions with AGENTS.md](https://learn.chatgpt.com/docs/agent-configuration/agents-md), read 2026-09-01.
This cross-surface readership is why the chain boundary design assigns its planned work-folder pointer to `AGENTS.md`,
not to a generated skill that a PR-comment session might never load. The pointer is design work, not current content in
this repository's `AGENTS.md`.

## Telemetry export

Copilot CLI can export OpenTelemetry traces and metrics to an OTLP endpoint or a JSON-lines file. Configure export in
the consumer environment with `COPILOT_OTEL_ENABLED`, `OTEL_EXPORTER_OTLP_ENDPOINT`, or
`COPILOT_OTEL_FILE_EXPORTER_PATH`; enterprise administrators can use `managed-settings.json`. OTel is off by default.
See the
[Copilot CLI command reference](https://docs.github.com/en/copilot/reference/copilot-cli-reference/cli-command-reference#opentelemetry-monitoring),
read 2026-09-01. Agentics ships no exporter and does not configure a destination.

## Validation

Validate the effective manifest and precedence rules after changing Copilot packaging:

```bash
node .github/tools/src/conformance/cli.ts manifests --repo .
```

Copilot hook behavior lives in the Go engine; when changing it, run the Go tests (the Copilot adapter's payload handling
is pinned by goldens in `hooks/go/internal/provider/`) and the conformance `hooks freshness` check:

```bash
npm run test:hooks
```

Build `hooks/go/cmd/hooktool` from a trusted checkout and set `AGX_HOOK_VERIFIER` to its absolute path before running
`node .github/tools/src/conformance/cli.ts hooks --repo <absolute-root>`.

Run the provider probe for an isolated live marketplace install and skill-list check. It sets temporary `COPILOT_HOME`
and `COPILOT_CACHE_HOME` values; the allow-missing option skips provider CLIs that are not installed.

```bash
node .github/tools/src/conformance/cli.ts probes --repo . --allow-missing
```

Verified on Copilot CLI 1.0.78 (2026-08-09, historical). An isolated local marketplace install selected
`.plugin/plugin.json`; `copilot skill list --json` exposed exactly 24 registered Agentics plugin skills without
`rubber-duck`. The install summary's 25 counted the directories the `"./"` source copied, not what registers; and the
manifest's `hooks` target resolved to the existing `hooks/hooks.copilot.json`. The native Copilot hook fixture also
passed. This probe did not invoke a prompt or hook lifecycle event.

## Live check

The provider probe stops at installation and registration. To exercise the current working tree, use the repository's
staged local-install flow:

1. Run `copilot mcp list` and save the server inventory. Use an isolated Copilot profile when practical.
2. Run `npm run plugin:setup`. It stages the current working tree and may update every installed CLI it detects.
3. Start a fresh authenticated Copilot session. Ask Agentics to route a small task and confirm that routing chooses a
   workflow.
4. Run the **Live check** on the [command guardian](../../hooks/README.md#command-guardian-live-check),
   [release guardian](../../hooks/README.md#release-guardian-live-check), and
   [secrets guardian](../../hooks/README.md#secrets-guardian-live-check).
5. Run `copilot mcp list` again. Agentics should add no server; distinguish anything pre-existing or user-configured
   from the plugin install.
6. Run `npm run plugin:teardown` even if a check fails.

Record the Copilot version and what happened. The install summary isn't evidence that a prompt or lifecycle event ran.

## Orchestration

Verified on Copilot CLI 1.0.67 (2026-07-06). Every surface doc answers these five questions in the same order, so
surfaces can be compared directly.

### Context

A dispatched subagent does not inherit the parent conversation. Prompts must be self-contained. _(live-confirmed)_

### Continuation

**Subagents can persist and communicate across parent, child, and sibling relationships** (source-confirmed, Copilot CLI
1.0.78, 2026-08-07). Follow-up work does not require a fresh one-shot handoff. Limits here are plan- and
configuration-dependent, so feature-detect rather than assuming a tier.

An earlier revision of this doc denied that persistence, and the error propagated into shared skills before being
corrected.

### Concurrency and nesting

Parallel dispatch uses multiple `task` calls in one turn. A running synchronous task can be promoted to the background,
including from the `/tasks` view, and nested subagents appear as a task tree. Agentics workflow policy is to dispatch a
useful wave and integrate its results, not launch one background agent solely to poll it.

The configuration reference gives `subagents.maxDepth` a default of 6 and lets usage-based users configure it up to 256.
The environment-variable reference separately documents `COPILOT_SUBAGENT_MAX_DEPTH` with a different default and range,
so feature-detect the active limit instead of treating 6 as a fixed cap.

The same reference sets the default number of subagents running at once by plan, from 2 to 32: Free / Education 2, Pro /
Pro+ 4, Max 8, Business 16, Enterprise 32, and usage-based billing 32. `subagents.maxConcurrency` is capped at 32, and
both settings are ignored on a plan without usage-based billing. A session cannot see its plan, so Agentics uses 2
unless the plan is known. Source:
[Copilot CLI command reference](https://docs.github.com/en/copilot/reference/copilot-cli-reference/cli-command-reference),
read 2026-11-19.

### Native agent contract

Dispatch uses the `task` tool's built-in `agent_type` values (`explore`, `task`, `general-purpose`, `code-review`,
`rubber-duck`, `research`, `security-review`) and is model-invocable. _(live-confirmed)_ Agentics registers no
`agent_type` values of its own.

### Capability policy

Subagent calls contribute to the parent session's AI-credit usage. The
[model ladder](../../skills/routing/references/model-ladders.md#copilot-cli-and-vs-code-role-resolution) defines the
model to request for each role and work class, the fallback chains, and how to report the binding the host confirms.

Current GitHub documentation lists `/fleet` for parallel subagents and `/delegate` for handing work to the Copilot cloud
agent. Agentics skills use the model-invocable `task` tool directly; they do not invoke either slash command. A prior
live review also recorded a `/batch` alias and internal SQL coordination for `/fleet`, but those details are not part of
the current documented command contract. The model-invocable equivalent of `/delegate` is the GitHub MCP server's
`create_pull_request_with_copilot` tool when enabled. Feature-detect it; do not assume it is present.

Dispatch guidance skills follow: `skills/routing/references/surface-dispatch.md`.
