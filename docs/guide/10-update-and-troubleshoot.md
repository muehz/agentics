# 10. Update and troubleshoot

Use this page when Agentics needs refreshing or a workflow or hook does not behave as expected. Start with your coding
tool below, then check the symptom table. [Setup](01-setup.md) covers first-time installation.

## Update or remove Agentics

Verify and unpack the new release with [Setup](01-setup.md#verify-and-unpack). Use the new absolute package path in the
commands below. After updating each tool, check its plugin inventory and repeat the
[setup verification](01-setup.md#verify).

### Claude Code

Remove the old installation while preserving its data, then register the new package.

```bash
claude plugin uninstall agentics@muehz --keep-data --scope user
claude plugin marketplace remove muehz
claude plugin marketplace add "/path/to/new/verified/package" --scope user
claude plugin install agentics@muehz --scope user
```

Run `/reload-plugins` in the active session. Match the scope of your installation. To remove Agentics while keeping its
plugin data, run only the first command. See
[Claude Code refresh details](../surfaces/claude-code.md#enable-disable-and-refresh).

### Copilot CLI

Repoint the local marketplace to the new version, then install from it.

```bash
copilot plugin uninstall agentics
copilot plugin marketplace remove muehz --force
copilot plugin marketplace add "/path/to/new/verified/package"
copilot plugin install agentics@muehz
```

Start a new session after changing the installation. To remove Agentics, run the first command. See
[Copilot refresh details](../surfaces/copilot-cli.md#enable-disable-and-refresh) and its
[local-path behavior](../surfaces/copilot-cli.md#configuration).

### Codex

Replace the registered marketplace and reinstall Agentics.

```bash
codex plugin remove agentics@muehz
codex plugin marketplace remove muehz
codex plugin marketplace add "/path/to/new/verified/package"
codex plugin add agentics@muehz
```

To remove Agentics, run only `codex plugin remove agentics@muehz`. Start a new session after installation or removal.
After installing, use `/hooks` to review and trust Agentics hooks. Repeat `/hooks` when hook entries change. See
[Codex removal and refresh details](../surfaces/codex.md#toggling-and-removing).

### Antigravity

Run `agy plugin uninstall agentics`, then `agy plugin install "/path/to/new/verified/package"` and check
`agy plugin list --json`. See [Antigravity installation](../surfaces/antigravity.md#install-and-discover).

### Cursor

The Agentics Cursor plugin package is retired. See [Cursor](../surfaces/cursor.md).

### VS Code Copilot Chat

Replace the old Agentics package path in `chat.pluginLocations` and reload VS Code. Check that Agentics is discoverable
and responds in Copilot Chat. Settings alone do not prove that it loaded.

To remove the **local registration**, delete your Agentics package entry from `chat.pluginLocations` and keep other
entries. See [VS Code configuration](../surfaces/vscode.md#configuration) and its
[live check](../surfaces/vscode.md#live-check). The maintainer reported startup working in the Local harness. Check the
installed package in your own harness.

## Find the symptom

| Symptom                                  | Possible cause                                                                                | What to check or do                                                                                                                                                                                                                                                                                       |
| ---------------------------------------- | --------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Skill missing on one CLI                 | Stale installation or session, a surface-specific inventory, or managed policy                | Check the effective inventory in your [surface doc](../surfaces/README.md), refresh the appropriate source above, and reload or start a new session. A directory on disk does not prove registration.                                                                                                     |
| Generic verification response            | Agentics may not have loaded, or the session still has an older inventory                     | Repeat [setup verification](01-setup.md#verify) and check the surface's runtime evidence. A generic response does not establish a successful install.                                                                                                                                                     |
| Hook binary missing or download rejected | Release download unavailable, fetching disabled, or checksum mismatch                         | Follow [missing-binary details](../../hooks/README.md#when-a-binary-is-missing). Check the installed version and the reported fetch error. Keep integrity checks intact; a rejected download is discarded. Guards have their own [failure postures](07-hooks.md#what-to-know-about-failure-and-surfaces). |
| Codex hooks do not run                   | Hook entries have not been trusted, or changed since trust was granted                        | Start a new session, run `/hooks`, and review and trust the Agentics entries. See [Codex setup](../surfaces/codex.md#local-install).                                                                                                                                                                      |
| Hook block looks wrong                   | The action may match a guard rule, or the command spelling may be outside its supported forms | Read [07. Hooks](07-hooks.md) and the named guardian's section of the [hooks README](../../hooks/README.md) for its rule, supported forms, and safe exceptions. Use the reported reason to narrow the action.                                                                                             |

Back to the [index](README.md).
