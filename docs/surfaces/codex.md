# Codex Surface

Codex support is packaged as the `agentics@muehz` Codex plugin. The package is skills-first by default and includes
Agentics skills and verified hooks.

## Where the upstream docs live

The Codex desktop app was folded into the ChatGPT app, and the plugin documentation moved with it. As of 2026-08-21:

| Page                                          | Status                                                                 |
| --------------------------------------------- | ---------------------------------------------------------------------- |
| `developers.openai.com/codex/plugins`         | **308-redirects** to `learn.chatgpt.com/docs/plugins`, a thin overview |
| `developers.openai.com/plugins`               | the unified ChatGPT + Codex plugin docs                                |
| `developers.openai.com/plugins/build/plugins` | manifest and marketplace-entry requirements                            |
| `developers.openai.com/codex/plugins/build`   | redirects to the unified package-your-plugin page                      |

The unified `/plugins/build/plugins` page covers curated repository marketplaces, manual local installs, workspace
sharing, and public-directory packaging. The former `/codex/plugins/build` URL now redirects there, so Agentics treats
the unified page as the controlling package and marketplace source.

### Published requirements and Agentics against them

Verified 2026-08-21 against `/plugins/build/plugins`; the `Entry name + source` row re-verified 2026-09-24 after the
entry moved to `"./"` (codex-cli 0.156.0, isolated install); the manifest row re-read 2026-10-30:

| Requirement                                                                 | Agentics                                   |
| --------------------------------------------------------------------------- | ------------------------------------------ |
| Manifest at `.codex-plugin/plugin.json` (compatibility fallback; see below) | ✅                                         |
| Manifest `name`, `version`, `description`                                   | ✅                                         |
| Component paths start with `./` and stay inside the plugin root             | ✅ `./skills/`, `./hooks/hooks.codex.json` |
| Entry `name` + `source`                                                     | ✅ `{"source": "local", "path": "./"}`     |
| Entry `policy.installation`                                                 | ✅ `AVAILABLE`                             |
| Entry `policy.authentication`                                               | ✅ `ON_INSTALL`                            |
| Entry `category`                                                            | ✅ `Engineering`                           |

Marketplaces are read from `$REPO_ROOT/.agents/plugins/marketplace.json` (repo-scoped) and
`~/.agents/plugins/marketplace.json` (per-user); `$REPO_ROOT/.claude-plugin/marketplace.json` is documented as a legacy
fallback. Agentics uses the repo-scoped path.

## Configuration

Codex uses:

```text
.agents/plugins/marketplace.json    # marketplace entry, source.path is "./" (the repo root)
.codex-plugin/plugin.json           # Codex plugin manifest at the repo root (hand-authored)
hooks/hooks.codex.json              # Codex hook config, referenced by the manifest's `hooks` field
skills/*/SKILL.md                   # canonical skills, discovered directly at the plugin root
```

Invoke an installed Agentics skill as `$agentics:<skill>`.

Codex installs Agentics **directly from the repo root**. The checked-in marketplace entry sets `source.path` to `"./"`,
which resolves the plugin root to the repository root and matches the published `./`-prefixed path form. Codex
materializes the tree into its install cache under a directory named for the manifest version
(`~/.codex/plugins/cache/<marketplace>/agentics/<version>/`, observed on Codex CLI 0.153.3).

This replaces the earlier arrangement, where Codex installed from a committed, generated `.codex/` subtree kept in sync
by `scripts/sync-codex-plugin.sh`. Root install works cleanly here because the on-disk `skills/` directory already holds
exactly the Agentics-registered roster (no unregistered upstream skills sit in it, so Codex's directory-level skill
discovery exposes the right set), and the canonical hook binary runs unchanged through `hooks/run-hook.cmd`. The sync
script, the committed `.codex/` tree, and the CI parity check have all been removed.

The manifest's `hooks` field points at `./hooks/hooks.codex.json`, the Codex hook config, rather than the
`hooks/hooks.json` that Codex discovers by default. Agentics ships no file at that default path.

The [build page](https://developers.openai.com/plugins/build/plugins), read 2026-10-30, now prefers a root `plugin.json`
and says existing `.codex-plugin/plugin.json` files "remain supported as a compatibility fallback". Agentics stays on
the fallback, which an isolated 0.166.0 install still accepts.

Codex is skills-first. Since the retirement of the `wayfinder` SessionStart hook, every other surface is too. Skill
discovery is harness-native everywhere.

`npm run plugin:validate` runs conformance and needs Go for the trusted hook verifier. Agentics no longer runs a
standalone inline JSON-syntax check.

Run the Go engine's tests when changing packaged hook behavior. The hook entrypoint tests in `hooks/go/cmd/hook/` cover
Codex patch and ordinary-tool payloads, deferred input validation, pre-tool patch write extraction, and the recorded
hook behavior corpus:

```bash
npm run test:hooks
```

Build `hooks/go/cmd/hooktool` from a trusted checkout and set `AGX_HOOK_VERIFIER` to its absolute path before running
`node .github/tools/src/conformance/cli.ts hooks --repo <absolute-root>`.

The conformance `hooks freshness` check proves each platform binary committed under `hooks/bin/` embeds the checked-out
source digest and matches `hooks/SHA256SUMS`; the hooks module gates prove a forced rebuild of that source reproduces
the manifest and those binaries. Codex materializes those exact artifacts into its cache.

## Hook protocol

What Codex does with a command hook's exit code and streams, from [Hooks](https://learn.chatgpt.com/docs/hooks), read
2026-09-24; the former `developers.openai.com/codex/hooks` address redirects there. What Agentics emits for each outcome
is the Provider Output Matrix in each guardian section of the hooks README:
[command guardian](../../hooks/README.md#command-guardian-provider-output-matrix),
[release guardian](../../hooks/README.md#release-guardian-provider-output-matrix), and
[secrets guardian](../../hooks/README.md#secrets-guardian-provider-output-matrix).
[ADR-0004](../adr/0004-go-hooks-is-the-reference-implementation.md) makes this section the contract.

| Exit  | `PreToolUse`                                                                 |
| ----- | ---------------------------------------------------------------------------- |
| 0     | Success; stdout JSON, if any, is processed; exit 0 with no output is success |
| 2     | Blocking decision; the reason is read from stderr                            |
| other | Failure; the hook error is reported and the operation is not blocked         |

`PreToolUse` reads `hookSpecificOutput.permissionDecision` (`allow`, `deny`) with `permissionDecisionReason`. `timeout`
is in seconds (default 600; `hooks/hooks.codex.json` sets 25). Provider-level errors, missing servers, and host timeouts
do not block the operation. Grouped Pre-tool intentionally converts its own infrastructure and selected guard faults
into a deny response before that outer timeout.

Codex reads a decision from the response document. The renderer emits `{}` when it has nothing to say and renders a deny
as `permissionDecision: "deny"` on exit 0. The renderer never uses exit 2. The legacy single-check command returns
adapter or core faults at exit 0 with `{}` and a diagnostic on stderr. Grouped Pre-tool instead renders a deny for such
faults. If no runtime is available, the wrapper exits 2 for Pre-tool and for the three single-check guard commands.

Codex names its tools differently from the neutral vocabulary the hook cores reason about, so the adapter maps them
before a core sees the event; a name not in the table passes through unchanged.

| Codex tool     | Neutral name |
| -------------- | ------------ |
| `apply_patch`  | `Edit`       |
| `exec_command` | `Bash`       |

For `apply_patch`, the Codex adapter normalizes added and replacement text into per-file entries before the secrets
guardian scans it. Removed lines and unchanged context do not count as a secret write. A malformed inspected patch
denies instead of becoming an empty scan. The source tests in `hooks/go/cmd/hook/hardening_test.go` cover the normalized
payload, per-file allowlisting, and forged parse markers. These are current implementation facts; they do not establish
a new live-host probe.

## Local Install

> **Version evidence.** Agentics root-layout install succeeded in a Codex CLI 0.147.0 probe and reproduced unchanged on
> 0.156.0 (2026-09-24). Repository history attributes root-layout support to 0.140.0 and recommends 0.142.2 or newer,
> but the 2026-08-21 official-source review did not verify those exact release boundaries. The checked-in marketplace
> entry now uses the published `./` path form; the published `/plugins/build/plugins` page (read 2026-09-24) states it
> explicitly: "Keep `source.path` relative to the marketplace root, start it with `./`, and keep it inside that root."
> The isolated install re-verified with `"./"` on codex-cli 0.156.0 (2026-09-24).

Use the absolute package path printed by the [verified setup guide](../guide/01-setup.md#verify-and-unpack). Register it
as a local marketplace and confirm Codex sees it.

```bash
codex plugin marketplace add "/path/to/verified/agentics-plugin-v1.230.0"
codex plugin marketplace list
codex plugin list
```

Install the local Agentics plugin:

```bash
codex plugin add agentics@muehz
```

Restart Codex after installing. Then run `/hooks`, review the Agentics hooks, and trust them before expecting the guards
to run automatically. Re-run `/hooks` after any Agentics upgrade. Codex records trust against the hook's hash, and since
0.164.0 a plugin hook's trust also binds to the plugin's contents, so a changed plugin needs renewed approval
([#54619](https://github.com/openai/codex/pull/54619)).

### Confirming what installed

`codex plugin list` reads the configured marketplace snapshots. Two flags make it precise:

```bash
codex plugin list --marketplace muehz --available --json
codex plugin list --marketplace muehz --json
```

`--available` includes catalog entries that are not installed, so the same command answers both "is Agentics
discoverable?" (`installed: false` before install) and "did it land?" (`installed: true` after). `--json` is the
scriptable form.

Both `add` and `remove` accept either `PLUGIN@MARKETPLACE` or a bare plugin name with `--marketplace`, so
`codex plugin add agentics@muehz` and `codex plugin add agentics --marketplace muehz` are equivalent.

Verified on codex-cli 0.147.0 (2026-08-09, historical): root local marketplace install into an isolated `CODEX_HOME`
yields `installed: true, enabled: true`; `remove` empties the set and reinstall restores it.

Verified on codex-cli 0.156.0 (2026-09-24, historical): the same sequence reproduces in an isolated `CODEX_HOME`:
`marketplace add` on the repo root resolves and installs `agentics@muehz` at the manifest's current version
(`installed: true, enabled: true`), `remove` empties the installed set, and reinstall restores it.

Verified on codex-cli 0.166.0 (2026-10-30): the same sequence reproduces. `--available` lists `agentics@muehz` with
`installed: false`, and the install lands under `plugins/cache/muehz/agentics/<manifest version>/`. The `/hooks` trust
step and hook execution were not exercised.

> Historical (0.147.0): the `--available` query returned an empty plugin set before install and after removal. On
> codex-cli 0.156.0 (2026-09-24) it lists `agentics@muehz` with `installed: false` in both states; `--available`
> requires `--json` and otherwise exits with a usage error.

### Toggling and removing

`/plugins` is the interactive dashboard. It groups entries by marketplace, installs and uninstalls them, and toggles an
already-installed plugin. **Space** changes the enabled state without removing the cache.

```bash
codex plugin remove agentics@muehz
```

**Start a new session** after installing, removing, or re-enabling. Codex resolves skills at session start; a running
session will not pick up the change, which is the usual reason a freshly installed Agentics appears to be missing.

Marketplace snapshots are cached. Use the [update guide](../guide/10-update-and-troubleshoot.md#codex) when switching
the marketplace to a new verified package path.

## Live check

Codex reads a hook's decision from the response document, so a golden test can't prove that a live host honours the
denial. To exercise the current working tree:

1. Run `codex mcp list` and save the server inventory. Use an isolated `CODEX_HOME` when practical.
2. Run `npm run plugin:setup`. It stages the current working tree and may update every installed CLI it detects.
3. Restart Codex and trust the Agentics hooks through `/hooks`.
4. Ask Agentics to route a small task. Confirm that it selects and follows a workflow.
5. Run the **Live check** on the [command guardian](../../hooks/README.md#command-guardian-live-check),
   [release guardian](../../hooks/README.md#release-guardian-live-check), and
   [secrets guardian](../../hooks/README.md#secrets-guardian-live-check).
6. Run `codex mcp list` again. Agentics should add no server; distinguish anything pre-existing or user-configured from
   the plugin install.
7. Run `npm run plugin:teardown` even if a check fails.

Record the Codex version and each result. This live check is the evidence that the host honours the denial document end
to end.

## Codex desktop (ChatGPT)

The repo marketplace is discoverable from the ChatGPT desktop app and the CLI. The two do not share discovery state, so
a plugin visible in one may need a separate step in the other.

1. Open **Work** or **Codex** in the ChatGPT desktop app.
2. Restart the app after any change to `.agents/plugins/marketplace.json`. Desktop reads the marketplace at startup.
3. Open **Plugins**, select the `muehz` marketplace, and confirm Agentics appears with its interface metadata.

`.agents/plugins/marketplace.json` is repo-scoped. Codex discovers it from the repository, and its `source.path`
resolves relative to the marketplace root. For Agentics, the entry's `"./"` resolves to the repo root itself.

## Sharing with a workspace

Codex offers a third distribution path, parallel to the repo marketplace and distinct from the public directory:

| Path                                       | How a developer gets Agentics                                | Requires git?                      |
| ------------------------------------------ | ------------------------------------------------------------ | ---------------------------------- |
| Repo marketplace (**what Agentics ships**) | `codex plugin marketplace add` + `plugin add agentics@muehz` | yes                                |
| Workspace sharing                          | a workspace admin publishes it from ChatGPT Plugins          | no                                 |
| Universal Plugins Directory                | submission, review, approval                                 | n/a, **Agentics is not submitted** |

The current published flow needs a workspace admin. Go to [ChatGPT Plugins](https://chatgpt.com/plugins), select
**Personal**, open the plugin's three-dot menu, choose **Publish**, and specify the workspace roles that should have
access. The docs are explicit that this "doesn't publish it to the universal public Plugins Directory." Access stays
inside the workspace and organization boundary. Accounts not signed in to that workspace cannot reach it.

Workspace admins can disable workspace plugin publishing with `features.plugin_sharing = false` in `requirements.toml`.
The docs don't say how the **Publish** action then appears.

> **Unverified for Agentics.** The published docs do not say whether a plugin installed from a repo-scoped
> `marketplace.json`, which is how Agentics is distributed, appears in the **Personal** publishing flow, and that path
> has not been exercised live. Treat this section as the provider's documented capability, not as a supported Agentics
> install route, until a probe says otherwise.

## Packaged Assets

- Skills: every canonical skill under `skills/`, discovered through the Codex manifest's `"./skills/"` path.
- Hooks: Codex registers one `PreToolUse` callback and none afterward. It runs two checks, `commands` and
  `secrets-guardian`, and starts one Agentics binary. The command is `run-hook.cmd codex pre-tool`, with no flags. It
  has `command` and `commandWindows` alternatives bound to `PLUGIN_ROOT`, yielding 2 alternatives here and 5 across all
  three providers.

    Pre uses matcher `*` and a 25-second host budget. See
    [matcher patterns](https://learn.chatgpt.com/docs/hooks#matcher-patterns), reviewed 2026-10-14.

    Actual normalized input selects the checks, and the guard workers have separate 10-second limits. See
    [event routing and failures](../../hooks/README.md#event-routing-and-failures) for order, cleanup, aggregation, and
    legacy commands.

    Source/config process proofs on Windows and Linux simulate provider selection, then run the real wrapper and source
    binary. They establish source behavior only. They do not establish packaged binary or live-host acceptance. Newly
    packaged macOS and ARM checks remain pending. A normalized PowerShell test shape is not proof that Codex delivers a
    native PowerShell tool. See [workflow policy settings](../../hooks/README.md#settings) for the guard settings and
    the planned workflow reminders.

    The guards ran in `--mode warn` until the deny path was exercised. The adapter had always implemented it.
    `codexRender` emits `permissionDecision: "deny"` on `PreToolUse`, and the recorded Codex dispatch corpus pins the
    exact document, so enabling it was a flag change, not new code.

    **What is verified and what is not.** The rendered document is pinned and exercised by the recorded dispatch corpus.
    What no test can establish from here is whether a live Codex CLI honours that document. Run the
    [live check](#live-check) against a real install to confirm it end to end.

- `bin/`: the two `agx` launchers, `bin/agx` (POSIX `sh`) and `bin/agx.exe` (native Windows). Codex does not put a
  plugin's `bin/` on the agent's `PATH`, so bare `agx` is not found and skills use the launcher-path fallback that
  [`flow-state.md`](../../skills/execute/references/flow-commands.md#commands) states, such as
  `& "<plugin-root>/bin/agx.exe" flow status` from PowerShell on Windows.

    Verified on Codex CLI 0.167.2 (2026-11-12), Windows 11 x64, with the plugin installed from this revision: PowerShell
    reported bare `agx` as not recognized, and `& "<plugin-root>/bin/agx.exe" flow status` printed the status. A
    throwaway-plugin probe on Codex CLI 0.167.1 on 2026-11-11 gave the same result.
    [ADR 0023](../adr/0023-agx-is-the-agentics-command-line-tool.md) records the probe.

- MCP: Agentics ships no MCP server configuration. A Codex workspace that needs GitHub, Azure DevOps, Playwright, or
  another server declares it in its own Codex MCP config.

## Orchestration

The historical observations below were verified on codex-cli 0.133.0 (2026-07-06); current provider contracts are called
out separately. Every surface doc answers these five questions in the same order, so surfaces can be compared directly.

### Context

A spawned subagent does not inherit the parent conversation; the spawn prompt must carry everything it needs.
_(live-confirmed)_

### Continuation

An existing subagent can receive follow-up work and be waited on, so continuation does not require a fresh spawn. Exact
tool names have varied across Codex releases; the current host exposes follow-up, message, and wait operations, while
the 0.133.0 probe exposed `resume_agent`, `send_input`, and `wait_agent`.

### Concurrency and nesting

The current configuration reference documents concurrency as `agents.max_concurrent_threads_per_session` and retains
`agents.max_threads` as a legacy alias. Size waves to the configured limit or ask the user to raise it rather than
assuming a fixed number. When the key is unset, Codex chooses the default (the Codex source used 6 on 2026-11-18). The
documented `[agents]` schema does not list a depth key, but the Codex source reads `agents.max_depth` with a default of
1 (checked 2026-11-18). Treat it as undocumented and subject to change. The 0.133.0 probe observed six concurrent
threads, depth one, and an experimental `spawn_agents_on_csv` operation that ran one local worker per CSV row and waited
for the rows to finish; treat those as historical observations, not current configuration guarantees.

### Native agent contract

**Agentics ships no Codex roles.** Current Codex custom roles are TOML files discovered from `~/.codex/agents/` for
per-user agents or `.codex/agents/` for project-scoped agents. A 0.133.0 probe found only the per-user scope; a
2026-08-19 probe on codex-cli 0.148.0 confirmed both scopes load. The Codex plugin system ships no agent roles.

Agentics generates no role TOMLs and cannot install any. The plugin manifest schema is closed and carries no agents
field, and role files placed inside a plugin's install cache are not discovered. The 2026-08-19 probe confirmed both.

### Capability policy

**Authorization gate.** Codex may delegate after a direct user request or when applicable `AGENTS.md` or skill
instructions authorize subagents. A user-typed `$agentics:execute` invocation is one direct authorization route. Codex
invokes plugin skills with a `$` prefix, distinct from built-in slash commands like `/hooks`. Agentics treats a
user-typed `$agentics:zealot` invocation as the same kind of route for Zealot's finder and reviewer dispatch; if
spawning is refused or limits are hit, Zealot runs its labelled in-session fallback (`roles not separated by context`).
Skills restate that authorization when dispatching. If spawning is refused, the host cannot dispatch, so `execute` stops
and names `pair` as the only mode that runs there. The 0.133.0 probe had `multi_agent_v2` off; because tool names have
varied across releases, dispatch guidance describes the operation rather than treating an experimental tool name as
stable.

- **User-only.** Codex Cloud tasks and best-of-N runs (`codex cloud exec --attempts N`) are dispatched by the user,
  never the model.
- **Observability.** Codex exposes `SubagentStart` and `SubagentStop` hook events. Agentics's packaged
  `hooks.codex.json` currently registers only `PreToolUse`, so it does not attach handlers to the subagent lifecycle
  events.

Dispatch guidance skills follow: `skills/routing/references/surface-dispatch.md`.
