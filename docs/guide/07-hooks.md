# 07. Hooks

Agentics hooks run before tool calls when your host loads and enables them. See the
[canonical hook roster](../../hooks/README.md#hook-index) for the full set.
[`command-guardian`](../../hooks/README.md#command-guardian-overview) blocks destructive commands before they run,
[`release-guardian`](../../hooks/README.md#release-guardian-overview) requires a change record for production deploy
commands, and [`secrets-guardian`](../../hooks/README.md#secrets-guardian-overview) blocks reads of credential files and
writes of secrets. You don't invoke them. This page is about what to do when one fires.

## When a command is blocked

```text
That command was blocked. Explain why, and run the safe form instead.
```

Why this phrasing works: every pattern `command-guardian` blocks ships with a suggested safer alternative in the
diagnostic, so "run the safe form" is a real instruction, not hope. A blocked `git clean` becomes a dry run plus a
targeted pathspec. A blocked `git reset --hard <ref>` becomes `git restore` or a stash. The block is the start of a
safer workflow, not a dead end.

## When a credential read or secret write is blocked

```text
That call was blocked by secrets-guardian. Tell me what it matched, and do the task without reading or writing the secret.
```

Why this phrasing works: the block names the path or the rule and a redacted value, and the response is yours to direct.
A blocked call exposed nothing. If the secret already sits in a commit, a log, or a paste, ask whether it needs
rotating.

## False positives have a designed path

Test fixtures and example keys can trip the guards. Use the relevant hook's documented control:
[`SECRETS_GUARD_ALLOWLIST`](../../hooks/README.md#secrets-guardian-configuration) for guardian path substrings and
[`COMMAND_GUARD_ALLOWLIST`](../../hooks/README.md#command-guardian-environment-variables) for command patterns. Scope
entries narrowly. A guardian path allowlist that includes real credential files skips their inspection.

## Turning a workflow rule off

Some blocks enforce your team's workflow rather than safety: draft-only pull requests, no package publish, a change
record before a deploy. If you own the workflow, switch the rule off in `.agentics/hooks.json` at the repository root,
or in `~/.agentics/hooks.json` for every repository. The repository file overrides the home file. The block message
names the key, for example `{"version": 1, "policy": {"draft_prs": false}}`. A typo in the file never turns a rule off:
a file the hook cannot read keeps every default and is named in the next policy block. Safety blocks have no switch.
Only you change these files: an agent that needs a rule off should ask you, and the guard denies an agent's attempt to
write them. See [Settings](../../hooks/README.md#settings) for the keys.

## Consumer-owned permissions and hooks

You can choose narrow permissions for reviewed git/build/test/lint commands, secret-file read denies, an optional
supported sandbox and network allowlist, quick changed-file format/lint checks, and protected paths with an approval
route. Ask scaffold to audit these options first. Explicit init previews each selected change in your configuration
before you confirm it. The [consumer setup guidance](../../skills/scaffold/references/templates/consumer-settings.md)
explains the provider-specific requirements and limits. Agentics adds no default runtime hooks or settings for these
options.

Choose the protected paths and who approves exceptions. A useful block names the path, matched rule, reason, approver
and approval route. Keep formatting scoped to changed files and installed tools. Post-edit checks cannot undo edits, and
tool-read denies or edit hooks do not confine arbitrary subprocesses. Verify your actual provider, platform and payload
protocol before relying on a configuration. Published documentation is separate from a client enforcement test.

## Team and managed configuration

For a team rollout, assign an owner for plugin distribution and another for hook exceptions, or explicitly give one
person both responsibilities. Record why each allowlist or skip setting exists and when to remove it.

Check the effective hook environment and each teammate's `.agentics/hooks.json` files when behavior differs between
teammates. A workflow rule that one teammate switched off in their home file still blocks for the others.
`COMMAND_GUARD_MODE` and `SECRETS_GUARD_MODE` override the configured `--mode` word; the documented skip variables
disable their hooks. Those are Agentics configuration rules, owned by the
[command guardian](../../hooks/README.md#command-guardian-environment-variables) and
[secrets guardian](../../hooks/README.md#secrets-guardian-configuration). On Copilot CLI, guardian `warn` mode permits a
match without displaying an advisory, as the [hooks README](../../hooks/README.md) explains.

Choose the configuration scope for the host you deploy. These are published configuration contracts checked on
2026-09-13, not new live rollout tests:

- **Claude Code team configuration:** commit shared hooks and the `env` block containing Agentics settings in
  `.claude/settings.json`. Keep personal overrides in `.claude/settings.local.json`, outside version control.
  [Settings files and precedence](https://code.claude.com/docs/en/settings) describes both scopes.
- **Claude Code organization configuration:** administrators deliver hooks and environment settings through managed
  settings, using `managed-settings.json` in the provider's system location or a supported managed delivery channel.
  Managed `allowManagedHooksOnly` restricts hook sources; force-enabled plugins have a documented exception. Consult
  [hook locations and managed restrictions](https://code.claude.com/docs/en/hooks#hook-locations) before applying that
  policy to Agentics. A committed project file is not organization enforcement.
- **Copilot CLI team configuration:** commit hook definitions in `.github/hooks/*.json` or the `hooks` key of
  `.github/copilot/settings.json`. Personal hooks belong under `~/.copilot/hooks/`. See the
  [hooks reference](https://docs.github.com/en/copilot/reference/hooks-reference) and
  [configuration scopes](https://docs.github.com/en/copilot/reference/copilot-cli-reference/cli-config-dir-reference).
- **Copilot CLI organization distribution:** managed `enabledPlugins`, `extraKnownMarketplaces`, and
  `strictKnownMarketplaces` control plugin distribution. File-based managed settings belong at
  `%ProgramFiles%\GitHubCopilot\managed-settings.json` on Windows,
  `/Library/Application Support/GitHubCopilot/managed-settings.json` on macOS, or
  `/etc/github-copilot/managed-settings.json` on Linux. The provider also supports MDM sources. Use the
  [managed settings contract](https://docs.github.com/en/copilot/reference/copilot-cli-reference/cli-config-dir-reference#mdm-managed-settings)
  with Agentics's [enterprise onboarding example](../surfaces/copilot-cli.md#enterprise-onboarding). This establishes
  plugin policy, not a managed equivalent for every Agentics hook environment variable.

For Codex, the verified Agentics setup uses the plugin's hook manifest and requires the
[hook trust procedure](../surfaces/codex.md#local-install). Current official documentation checked on 2026-10-14 now
describes administrator-owned
[managed hooks in requirements.toml](https://learn.chatgpt.com/docs/hooks#managed-hooks-from-requirementstoml). This is
an administrator-facing pointer, not a scaffold consumer write or a verified managed Agentics rollout. Equivalent team
or managed Agentics hook rollout on VS Code is also unverified; consult its [surface records](../surfaces/README.md)
rather than copying another host's settings paths.

Configuration precedence does not change the host's handling of failures. Do not treat managed settings or `block` mode
as a promise that a timed-out hook prevents execution. The surface records linked below document that boundary.

## What to know about failure and surfaces

Separate a missing binary, a guard decision, and a host failure when diagnosing a hook:

- **Missing binary:** after the [binary fetch path](../../hooks/README.md#when-a-binary-is-missing) cannot restore it,
  the [dispatcher](../../hooks/run-hook.cmd) emits a diagnostic and exits 2 for `command-guardian`, `release-guardian`,
  and `secrets-guardian`, and for `pre-tool`, which runs them as one event. The recorded
  [Claude Code](../surfaces/claude-code.md#hook-protocol), [Copilot CLI](../surfaces/copilot-cli.md#hook-protocol), and
  [Codex](../surfaces/codex.md#hook-protocol) protocols each describe exit 2 as blocking on the pre-tool event. This
  combines current dispatcher code with the recorded host contracts; it is not a new live missing-binary test.
- **Guard decision or adapter fault:** a dispatched binary renders outcomes through its provider adapter. For example,
  the [command guardian output matrix](../../hooks/README.md#command-guardian-provider-output-matrix) distinguishes a
  deny from a malformed payload or adapter/core error, whose response remains non-blocking. Codex's normal deny uses a
  JSON decision at exit 0, unlike the dispatcher's missing-binary exit 2.
- **Host timeout:** the recorded protocols for [Claude Code](../surfaces/claude-code.md#hook-protocol),
  [Copilot CLI](../surfaces/copilot-cli.md#hook-protocol), and [Codex](../surfaces/codex.md#hook-protocol) say a timeout
  does not block the operation. A configured guard therefore does not guarantee refusal on every failure path.
- **Merge, forced push, and release publication:** the hooks do not gate them. Control of merging and forced pushes to
  `main` or `master` is left to the repository's own branch protection and review rules, which the hooks do not replace;
  where a repository has none, or they exempt the caller (an org admin, for example), the hooks do not gate the merge or
  the forced push. Publishing a GitHub release is left to the repository's permissions and any tag or release rulesets.

**Pitfall:** don't respond to a block by disabling the hook. `SKIP_SECRETS_GUARD=true` and its siblings exist for
controlled situations, not as the everyday answer to friction. Reach for the narrow allowlist entry or the suggested
safe form first. If a block looks wrong, the matching guardian section of the [hooks README](../../hooks/README.md)
documents what it matched and why.

Next: [08. Extend](08-extend.md)
