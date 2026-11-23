# Antigravity Surface

Antigravity supports Agentics natively through `.antigravity-plugin/plugin.json` and the `agy` CLI. The package is
skills-first and discovers all 26 canonical Agentics skills directly from the repository root.

## Where the upstream docs live

As of 2026-11-21:

| Page                                                           | Scope                                                |
| -------------------------------------------------------------- | ---------------------------------------------------- |
| [Plugins](https://antigravity.google/docs/plugins)             | plugin overview, discovery, and component layout     |
| [Build plugins](https://antigravity.google/docs/build-plugins) | manifest schema, `SKILL.md` limits, and validation   |
| [Marketplace](https://antigravity.google/docs/marketplace)     | marketplace packaging, permissions, and distribution |
| [Hooks](https://antigravity.google/docs/hooks)                 | lifecycle hook configuration and payload contract    |

### Published requirements and Agentics against them

Reviewed 2026-11-21 against the four official Antigravity documentation pages above and exercised against `agy 1.5.13`:

| Requirement                                              | Agentics                                                                                                            |
| -------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------- |
| Manifest at `.antigravity-plugin/plugin.json`            | ✅ (`$schema: "https://antigravity.google/schemas/v1/plugin.json"`)                                                 |
| Manifest `name`, `displayName`, `version`, `description` | ✅ (`agentics`, `Agentics`, release-managed version)                                                                |
| Manifest `suggestedPrompts`                              | ✅ three starter prompts                                                                                            |
| Skills discovered under `skills/*/SKILL.md`              | ✅ all 26 canonical skills register                                                                                 |
| No root `plugin.json`                                    | ✅ omitted deliberately to preserve Copilot CLI manifest precedence                                                 |
| `SKILL.md` body within 24,000-byte load cap              | ✅ all 26 `SKILL.md` bodies are under 24,000 bytes (zero `SKILL.md` warnings, enforced in CI)                       |
| Native pre-tool hooks                                    | ✅ shipped via root `hooks.json` (`components.hooks: 1`; `PreToolUse` -> `hooks/run-hook.cmd antigravity pre-tool`) |
| MCP server configuration                                 | n/a — Agentics ships no MCP config on any surface                                                                   |

## Configuration

Antigravity uses:

```text
.antigravity-plugin/plugin.json     # Antigravity plugin manifest at the repo root
hooks.json                          # Antigravity PreToolUse hook registration (namespace "agentics")
skills/*/SKILL.md                   # Canonical skills, discovered directly from the plugin root
```

Antigravity uses a generic icon for local Agentics installs. Submission to the official Antigravity Marketplace requires
a bundled square icon and a `logo` field in the manifest that points to it.
[Build plugins](https://antigravity.google/docs/build-plugins)

Invoke an installed Agentics skill as `/<skill-name>` (for example, `/routing`, `/execute`, or `/decompose`).

There is deliberately no root `plugin.json`. A root `plugin.json` outranks `.github/plugin/plugin.json` in Copilot CLI's
manifest resolution order, so Agentics keeps the Antigravity manifest in `.antigravity-plugin/plugin.json`.

Agentics ships no MCP server configuration on Antigravity or any other surface. Connect GitHub, Playwright, or other MCP
servers in your own Antigravity MCP configuration (`agy mcp`).

Current rollout status:

- All 26 `SKILL.md` bodies are under Antigravity's 24,000-byte load cap with zero `SKILL.md` validator warnings,
  enforced by CI (`ci-antigravity-native.ts`).
- Agentics registers native Antigravity pre-tool hooks in the repository's root `hooks.json`.

`agy plugin validate` still emits one warning for repository-root `AGENTS.md` because Antigravity plugins load plugin
rules only from `rules/`. Agentics keeps `AGENTS.md` at the repository root as its canonical contributor-agent
instructions file rather than packaging it as a plugin rule.

## Hook protocol

Repository-root `hooks.json` registers one `PreToolUse` hook under the `"agentics"` namespace with matcher `"*"`,
command `"./hooks/run-hook.cmd" antigravity pre-tool`, and a 25-second timeout. `hooks/run-hook.cmd` resolves the
platform binary under `hooks/bin/agx-<os>-<arch>` (fetching from the matching GitHub Release and verifying against
`hooks/SHA256SUMS` when absent) and runs the grouped `pre-tool` event (`commands` for command-guardian and
release-guardian, and `secrets-guardian`). See [`hooks/README.md`](../../hooks/README.md) for the full rule bank,
environment variables, and `.agentics/hooks.json` workflow policy settings.

### Wire contract

- **Input (stdin JSON):** Optional `hookEventName` (`"PreToolUse"` when present), required `toolCall` object with string
  `name` and object (or absent/null) `args`, and optional `cwd`, `workspacePaths` (first entry used when `cwd` is absent
  or empty), `conversationId`, `turnId`, `transcriptPath`, and `modelName`.
- **Output (stdout JSON, always `exit 0`):**
    - Pass, continue, or neutral `context` (`warn` mode / info-tier advisory): `{"decision":"allow"}`. Antigravity's
      `PreToolUse` contract has no advisory context channel, so neutral context outcomes render as allow.
    - Deny (guard block, unreadable/malformed input, or grouped event fault): `{"decision":"deny","reason":"..."}`.

### Tool mapping

The Antigravity adapter normalizes native IDE, CLI, and ACP tool names to Agentics's neutral tool vocabulary while
keeping raw `toolCall.args` under `tool.input.value`:

| Antigravity tool             | Neutral tool                  | Lifted keys                                                                                                                                                                                                                                                                             |
| ---------------------------- | ----------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `run_command`                | `Bash`                        | `CommandLine` / `command` -> `command`; `Cwd` -> `cwd`                                                                                                                                                                                                                                  |
| `manage_task`                | `manage_task` / `Bash`        | Lifecycle `Action` (`list`, `status`, `kill`) with no `Input`/`input` stays `manage_task`; `send_input`, smuggled `Input`/`input`, or unknown/missing `Action` normalizes to `Bash`, lifts string `Input`/`input` -> `command`, and sets `unclassified_task_input: true` (fails closed) |
| `send_command_input`         | `send_command_input` / `Bash` | Pure termination (`Terminate: true` with absent, null, or empty `Input`/`input`) stays `send_command_input`; any stdin payload or non-termination normalizes to `Bash`, lifts string `Input`/`input` -> `command`, and sets `unclassified_task_input: true` (fails closed)              |
| `view_file`                  | `Read`                        | `AbsolutePath` / `Path` -> `file_path`                                                                                                                                                                                                                                                  |
| `read_file`                  | `Read`                        | `Path` / `AbsolutePath` -> `file_path`                                                                                                                                                                                                                                                  |
| `write_to_file`              | `Write`                       | `TargetFile` / `Path` -> `file_path`; `CodeContent` / `Content` -> `content`                                                                                                                                                                                                            |
| `write_file`                 | `Write`                       | `Path` / `TargetFile` -> `file_path`; `Content` / `CodeContent` -> `content`                                                                                                                                                                                                            |
| `replace_file_content`       | `Edit`                        | `TargetFile` / `Path` -> `file_path`; `TargetContent`/`ReplacementContent` -> `old_string`/`new_string`                                                                                                                                                                                 |
| `edit_file`                  | `Edit`                        | `Path` / `TargetFile` -> `file_path`; `Target`/`Replacement` -> `old_string`/`new_string`                                                                                                                                                                                               |
| `multi_replace_file_content` | `MultiEdit`                   | `TargetFile` / `Path` -> `file_path`; `ReplacementChunks` -> `edits` (malformed chunks fail closed)                                                                                                                                                                                     |
| `grep_search`                | `Grep`                        | `SearchPath` -> `path`; `Query` -> `pattern`                                                                                                                                                                                                                                            |
| `find_by_name`               | `Glob`                        | `SearchDirectory` -> `path`; `Pattern` -> `pattern`                                                                                                                                                                                                                                     |
| `list_dir`                   | `Glob`                        | `DirectoryPath` -> `path`                                                                                                                                                                                                                                                               |
| `search_web`                 | `WebSearch`                   | raw `args` under `value`                                                                                                                                                                                                                                                                |
| `read_url_content`           | `WebFetch`                    | raw `args` under `value`                                                                                                                                                                                                                                                                |
| `invoke_subagent`            | `Agent`                       | raw `args` under `value`                                                                                                                                                                                                                                                                |
| `ask_question`               | `AskUserQuestion`             | raw `args` under `value`                                                                                                                                                                                                                                                                |

Unknown and MCP tool names pass through unchanged with their raw `toolCall.args` preserved under `tool.input.value`, so
typed GitHub and filesystem MCP rules apply identically to other surfaces.

### Fail-closed stdout-JSON posture

Antigravity treats non-zero hook exits as non-blocking hook errors (failing open). To keep guard failures fail-closed,
both `hooks/run-hook.cmd` (Windows `cmd.exe` and POSIX Bash paths) and the `agx` binary emit a single-line JSON deny
decision on stdout and exit `0` whenever the guard binary is missing, cannot be fetched, or exits non-zero during
grouped `pre-tool`:

- Missing binary on Windows `cmd.exe`: `{"decision":"deny","reason":"agx: guard binary missing - failing closed"}` on
  stdout, matching diagnostic on stderr, exit `0`.
- Missing binary on POSIX Bash or unexpected non-zero child exit on either path:
  `{"decision":"deny","reason":"agx: guard runtime unavailable - failing closed"}` on stdout, matching diagnostic on
  stderr, exit `0`.

### Live hook check

Exercise the packaged dispatcher directly from the repository root with native `PreToolUse` payloads:

```bash
printf '%s\n' '{"hookEventName":"PreToolUse","toolCall":{"name":"run_command","args":{"CommandLine":"git status"}}}' \
  | bash ./hooks/run-hook.cmd antigravity pre-tool

printf '%s\n' '{"hookEventName":"PreToolUse","toolCall":{"name":"run_command","args":{"CommandLine":"rm -rf /"}}}' \
  | bash ./hooks/run-hook.cmd antigravity pre-tool

printf '%s\n' '{"hookEventName":"PreToolUse","toolCall":{"name":"view_file","args":{"AbsolutePath":".env"}}}' \
  | bash ./hooks/run-hook.cmd antigravity pre-tool

printf '%s\n' '{"hookEventName":"PreToolUse","toolCall":{"name":"run_command","args":{"CommandLine":"kubectl apply -f production.yaml"}}}' \
  | bash ./hooks/run-hook.cmd antigravity pre-tool
```

On Windows `cmd.exe`, invoke `"./hooks/run-hook.cmd" antigravity pre-tool` with the same JSON payloads on stdin (with
`AGX_RELEASE_RECORD` unset for the deploy check). To verify missing-binary fail-closed behavior, copy
`hooks/run-hook.cmd` and `hooks/fetch-hook.ps1` into a disposable directory without `hooks/bin/`, set `AGX_NO_FETCH=1`,
and send the allow payload: both `cmd.exe` and Bash must exit `0` with `decision: "deny"` on stdout and a diagnostic on
stderr.

Verified on agy 1.5.13 (2026-11-21): packaged-binary probes against `hooks/run-hook.cmd antigravity pre-tool` on both
Windows `cmd.exe` and Git Bash returned exit `0` with exact `{"decision":"allow"}` and empty stderr for `run_command`
(`git status`); exit `0` with `decision: "deny"` and empty stderr for command-guardian (`rm -rf /` and `npm publish`),
secrets-guardian (`view_file` `.env`), and release-guardian (`kubectl apply -f production.yaml` with
`AGX_RELEASE_RECORD` unset); and exit `0` with single-line JSON `decision: "deny"` on stdout plus non-empty stderr for
the missing-binary `AGX_NO_FETCH=1` fail-closed probe (`agx: guard binary missing - failing closed` on `cmd.exe`;
`agx: guard runtime unavailable - failing closed` on Git Bash).

## Packaged `bin/`

Beside `skills/`, the plugin packages a top-level `bin/` directory with the two `agx` launchers: `bin/agx` (POSIX `sh`)
and `bin/agx.exe` (native Windows). Antigravity does not put a plugin's `bin/` directory on the agent's `PATH` by
default, so bare `agx` may not resolve in shell commands. When bare `agx` is not found, skills use the launcher-path
fallback documented in [`flow-state.md`](../../skills/execute/references/flow-commands.md#commands), such as
`"<plugin-root>/bin/agx" flow status` on POSIX shells or `& "<plugin-root>/bin/agx.exe" flow status` from PowerShell on
Windows.

## Validation

Antigravity provides a native manifest and file validator through `agy`:

```bash
agy plugin validate .
agy plugin validate --json .
```

The command checks the manifest, discovered components, and loader warnings and exits non-zero on errors. Passing
`--strict` promotes warnings to errors, which fails on the intentional repository-root `AGENTS.md` warning.

Verified on agy 1.5.13 (2026-11-21): `agy plugin validate --json .` exited 0 against the repository root, resolved
`.antigravity-plugin/plugin.json` (`name: "agentics"`, `agentPluginsSpec: false`), reported `components.skills: 26` and
`components.hooks: 1` (`agents: 0`, `mcpServers: 0`, `rules: 0`, `sidecars: 0`), `0` error findings, and `1` warning
finding (root `AGENTS.md` only; `0` `SKILL.md` warnings).

## Install and discover

Install Agentics directly from the absolute package path printed by the
[verified setup guide](../guide/01-setup.md#verify-and-unpack).

```bash
agy plugin install "/path/to/verified/agentics-plugin-v1.230.0"
```

Verify what landed:

```bash
agy plugin list
agy plugin list --json
```

`agy plugin list --json` prints the installed plugin inventory, including `name`, `enabled`, `version`, `source`,
`path`, and the registered `skills` array. A native Antigravity install reports `"source": "antigravity"` (distinct from
imported `"claude-code"` plugins) and installs under `<home>/.gemini/config/plugins/agentics`.

Verified on agy 1.5.13 (2026-11-21): an isolated install from a clean `git archive` tree into a disposable
`HOME`/`USERPROFILE` via `agy plugin install <stage>` exited 0, installed to `<home>/.gemini/config/plugins/agentics`,
and `agy plugin list --json` reported `name: "agentics"`, `enabled: true`, `version: "1.228.0"`,
`source: "antigravity"`, and all 26 skills registered. Live prompt routing and `agy mcp list` were not exercised in this
probe.

## Enable, disable, and remove

```bash
agy plugin disable agentics
agy plugin enable agentics
agy plugin uninstall agentics
```

Start a new Antigravity session after installing, updating, disabling, or removing the plugin so the session reloads the
active skill inventory.

## Known limits

- **24,000-byte `SKILL.md` body limit.** Antigravity loads only the first 24,000 bytes of each `SKILL.md` body and emits
  a validator warning for longer files. All 26 Agentics `SKILL.md` bodies are under 24,000 bytes with zero `SKILL.md`
  warnings, enforced by CI (`ci-antigravity-native.ts`).
- **`bin/agx` PATH fallback.** Antigravity does not place a plugin's `bin/` directory on the agent's `PATH`, so skills
  rely on the `<plugin-root>/bin/agx` (or `bin/agx.exe` on Windows) fallback when bare `agx` is not recognized.
- **`0755` Marketplace permission behavior.** Antigravity Marketplace packaging expects `0755` executable mode bits on
  executable plugin files when archives are built and unpacked. `bin/agx` is tracked with mode `100755` in Git;
  repackaging or unpacking on hosts that strip POSIX mode bits can break direct POSIX execution unless `0755`
  permissions are preserved.
- **No `PreToolUse` advisory context channel.** Antigravity's `PreToolUse` decision document supports `allow` and `deny`
  only, so `warn`-mode and info-tier `context` outcomes render as `{"decision":"allow"}` with no model-visible warning
  (command-guardian still writes detections to `guard.log`).

## Live check

Manifest validation and `agy plugin list --json` prove packaging and registration, not live session routing. To exercise
the current working tree:

1. Run `agy mcp list` and save the server inventory. Use an isolated profile when practical.
2. Run `agy plugin validate --json .` and confirm exit `0`, `components.skills: 26`, `components.hooks: 1`, and `0`
   errors.
3. Install a clean staged copy of the working tree with `agy plugin install <stage>` and confirm
   `agy plugin list --json` reports `name: "agentics"`, `enabled: true`, `source: "antigravity"`, and 26 skills.
4. Start a fresh Antigravity session and send a routing prompt (or invoke `/routing`) for a small task. Confirm Agentics
   selects and follows a workflow.
5. Run `agy mcp list` again. Agentics should add no server; distinguish pre-existing or user-configured servers from the
   plugin install.
6. Remove the local install with `agy plugin uninstall agentics` when finished, even if a check fails.

Record the `agy --version` output and each result.

## Packaged Assets

- Skills: every canonical skill under `skills/`, discovered through `.antigravity-plugin/plugin.json` at the plugin root
  (26 skills).
- `bin/`: `bin/agx` (POSIX `sh`) and `bin/agx.exe` (native Windows), invoked via the launcher-path fallback when bare
  `agx` is not on `PATH`.
- Hooks: repository-root `hooks.json` (`PreToolUse` -> `"./hooks/run-hook.cmd" antigravity pre-tool`), shared dispatcher
  `hooks/run-hook.cmd`, Windows fetch helper `hooks/fetch-hook.ps1`, manifest `hooks/SHA256SUMS`, and the six platform
  binaries under `hooks/bin/`.
- MCP: Agentics ships no MCP server configuration.

## Orchestration

Checked from the Antigravity session tool schema on 2026-11-20; the check ran no model. Every surface doc answers these
five questions in the same order, so surfaces can be compared directly.

### Context

A dispatched subagent starts with a fresh conversation context and does not inherit the parent conversation; prompts
must carry everything the subagent needs.

### Continuation

`send_message` sends follow-up work or steering updates to an existing subagent by its `conversationId`, so multi-turn
continuation does not require a fresh spawn.

### Concurrency and nesting

Parallel work batches multiple `invoke_subagent` calls in a single turn; subagents run in the background and report back
via messages. No fixed vendor concurrency limit is verified, so Agentics uses its default batch bound of 2 unless a
higher limit is confirmed. Tasks that write the same files still run in sequence. `Workspace` supports `inherit`,
`share`, and `branch` isolation modes; `manage_subagents` lists or terminates direct subagents (killing a subagent also
terminates its descendants and deletes their branched workspaces).

### Native agent contract

Agentics registers no custom subagent types in its plugin manifest. Dispatch uses Antigravity's built-in subagent types
(`self`, `research`, `image-generator`) and session-scoped `define_subagent` types through `invoke_subagent`. For
shared-board team coordination or script-driven fan-out, Antigravity also exposes `swarm` and `run_workflow`.

### Capability policy

Per-call `Model` accepts `inherit`, `flash_lite`, `flash`, or `pro`, with no per-dispatch effort parameter; tier mapping
follows `skills/routing/references/model-ladders.md`. Agentics encodes no tool permissions in the plugin manifest;
restrictions come from host and user settings.

Dispatch guidance skills follow: `skills/routing/references/surface-dispatch.md`.
