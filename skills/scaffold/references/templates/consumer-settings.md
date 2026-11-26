# Offer consumer-owned settings and hooks

Use this guide for the consumer options in the [protection audit](../protections.md#consumer-owned-options). Audit
reports gaps, and its closing offer starts init when the user says yes. Init lists the options together in its one menu,
each selectable on its own, then previews the selected changes before confirmed writes. These examples are discussion
material, not activated defaults. Agentics installs no runtime hooks or provider settings from this guide.

## Discover the consumer's setup

Before a preview, record the installed provider/version, actual operating system, configuration scopes, trust state and
effective settings. Read existing files. Preserve authored rules and hooks. Discover the real git/build/test/lint
commands, secret paths, formatter/linter binaries, protected paths and person/team who can approve exceptions. Do not
assume this repository's commands or paths suit another consumer.

Check current official documentation for the selected client and version. If the capability or payload is unresolved, do
these steps:

1. Mark the option `blocked` or `unverified`.
2. Name the missing proof.

A source review establishes a published contract. It does not demonstrate the installed client's enforcement.

Preview every affected consumer-owned path, exact setting diff and any helper content. Include prerequisites, behavior,
limitations, verification commands and the approval route. Selection requests a preview. Confirmation approves only the
enumerated local writes. Installation, provider execution and enforcement probes keep their applicable consent
boundaries. Revised content needs a fresh preview. Keep declined options unchanged.

## Allow reviewed inner-loop commands

For a Claude Code consumer whose scripts have been reviewed, this is an illustrative settings fragment:

```json
{
    "permissions": {
        "allow": [
            "Bash(git status)",
            "Bash(git --no-pager diff --no-ext-diff --no-textconv)",
            "Bash(npm run build)",
            "Bash(npm test)",
            "Bash(npm run lint)"
        ]
    }
}
```

Use only commands the consumer actually runs. Exact rules reduce routine prompts and do not grant arbitrary `git *`,
shells, interpreters, package-manager commands or runtime execution. Build/test/lint scripts execute repository code.
Inspect their scripts, configuration and dependencies before you allow them. Git also uses repository and user
configuration. Recheck permissions when those inputs change. A command permission is not an OS access boundary. See
[Claude permission syntax and limitations](https://code.claude.com/docs/en/permissions).

## Deny selected secret-file reads

If the consumer selects these paths, preview a separate Claude Code fragment:

```json
{
    "permissions": {
        "deny": ["Read(./.env)", "Read(./secrets/**)"]
    }
}
```

Replace the examples with the consumer's actual paths and documented matching syntax. Preserve existing denies. Verify
nested locations, links and alternate read tools. Do not read secret values to test a rule. Tool-read denies do not
isolate arbitrary subprocesses: an allowed script or interpreter may read files itself. They also do not remove secret
data already available in Git objects, logs or prior context. Report those limits and any separate sandbox need.

## Offer an optional sandbox and network allowlist

Check the actual host, required services, dependencies and escape routes. Then offer **OS sandbox** and **network
allowlist** strictly independently — never bundle `"enabled": true` inside a network-allowlist-only preview. For a
supported Claude Code host, separate discussion fragments are:

```json
{
    "sandbox": {
        "enabled": true
    }
}
```

```json
{
    "sandbox": {
        "network": {
            "allowedDomains": ["registry.npmjs.org"]
        }
    }
}
```

The domain is an example to replace, not a required service or a trust endorsement. Claude documents sandbox support on
macOS, Linux and WSL2. Native Windows runs without that sandbox. Check dependencies, exclusions, unsandboxed retries and
whether new destinations can be approved. Project `allowedDomains` alone does not establish strict network lockdown. Use
the [sandbox contract](https://code.claude.com/docs/en/sandboxing) for the selected scope and host. Hold unsupported
configurations. Do not claim the setting provides isolation for them.

## Offer fast checks after edits

Offer a post-edit formatter and linter in the consumer’s own hook configuration. Use that host’s documented edited-file
payload. Then do these steps:

1. Normalize the path.
2. Contain the path.
3. Check the relevant extension.
4. Pass the single changed filename as an argument to an already installed formatter/linter.

Do not interpolate paths into shell source. Do not install a package from the hook. Keep the hook fast. Builds,
whole-suite tests and other heavy checks stay at commit or PR time. A post-edit hook runs after the edit and cannot
prevent or undo it. Before any write, preview a tailored configuration and any helper. An untested payload or
unavailable binary is a visible verification gap.

Use structured argument arrays for the formatter/linter subprocess and an installed, reviewed executable. If the host
only accepts a shell command to launch a hook, do these steps:

1. Keep that launcher fixed.
2. Parse the event in a reviewed helper.

Never splice an edited path into the launcher. Handle argument-option ambiguity with the selected tool's documented
filename convention. For a multi-file edit, do these steps:

1. Enumerate every changed file.
2. Validate every changed file.
3. Invoke checks per file.

Skip unsupported extensions and deleted files with an explanation. Retain the consumer's existing checks and a short
timeout. Report a timeout or missing binary as incomplete verification.

Illustrative Claude Code `PostToolUse` helper (`.claude/hooks/post-edit-check.cjs`, wired from `.claude/settings.json`
under `hooks.PostToolUse` with matcher `Edit|Write`):

```js
const fs = require("node:fs");
const path = require("node:path");
const { execFileSync } = require("node:child_process");
const root = fs.realpathSync(process.cwd());
const ev = JSON.parse(fs.readFileSync(0, "utf8") || "{}");
const rawPaths = Array.isArray(ev.tool_input?.files) ? ev.tool_input.files : [ev.tool_input?.file_path].filter(Boolean);
for (const p of rawPaths) {
    const abs = path.resolve(root, p);
    if (!fs.existsSync(abs)) continue;
    const real = fs.realpathSync(abs);
    if (real !== root && !real.startsWith(root + path.sep)) continue;
    if (!/\.(ts|tsx|js|jsx|json|md)$/.test(real)) continue;
    execFileSync(path.join(root, "node_modules/.bin/prettier"), ["--write", "--", real], {
        stdio: "inherit",
        timeout: 10000,
    });
}
```

Replace the binary path, flags, and extensions with the consumer's installed formatter/linter.

## Select protected paths and explain blocks

Ask the consumer which paths are protected and who can approve an exception. Generated code and frozen packages are
examples to discuss, not default protected paths. A selected pre-edit check must use the provider’s actual payload and
denial protocol. It must normalize paths. It must account for links/traversal. It must explain unsupported edit shapes.
Its block names the path, matched rule, reason, approving person/team and the approval route. Tool hooks alone do not
confine arbitrary subprocess writes. Explain any remaining shell, MCP, timeout or sandbox boundary. Do not present the
example as a universal ACL.

A tailored helper must check containment against the discovered repository root. Define separator/case normalization for
the host. Resolve existing links and the parent of new targets. Reject ambiguous or escaping paths. Check all affected
paths for creates, moves, renames, replacements and deletions. Do not inspect only the first path in a patch.
Unsupported edit shapes need a visible decision and approval route, not silent acceptance or a claim of full coverage.
Use the provider's documented blocking response. A printed warning alone may not deny the operation.

For example, after the consumer supplies its rule and owner, a block could read:

```text
Blocked edit: generated/client.ts matches generated/**. This client is regenerated from its schema.
Approval: the SDK owners named in CODEOWNERS. Route: request a scoped exception in the consumer's review process.
```

Illustrative Claude Code `PreToolUse` helper (`.claude/hooks/protect-paths.cjs`, wired from `.claude/settings.json`
under `hooks.PreToolUse` with matcher `Edit|Write`):

```js
const fs = require("node:fs");
const path = require("node:path");
const root = fs.realpathSync(process.cwd());
const ev = JSON.parse(fs.readFileSync(0, "utf8") || "{}");
const rawPaths = Array.isArray(ev.tool_input?.files) ? ev.tool_input.files : [ev.tool_input?.file_path].filter(Boolean);
for (const p of rawPaths) {
    const abs = path.resolve(root, p);
    const parent = fs.existsSync(abs) ? fs.realpathSync(abs) : fs.realpathSync(path.dirname(abs));
    const rel = path
        .relative(root, fs.existsSync(abs) ? parent : path.join(parent, path.basename(abs)))
        .replace(/\\/g, "/");
    if (rel.startsWith("..") || rel.startsWith("<protected-prefix>/")) {
        process.stdout.write(
            JSON.stringify({
                hookSpecificOutput: {
                    hookEventName: "PreToolUse",
                    permissionDecision: "deny",
                    permissionDecisionReason: `Blocked edit: ${rel} matches <protected-prefix>/**. Approval: <owner>. Route: <route>.`,
                },
            }),
        );
        process.exit(0);
    }
}
```

Replace every example path, reason, owner and route with discovered consumer decisions. Do not invent an approver. Do
not recommend that a hook be disabled. If the approval route is unknown, leave that option unresolved in the preview.

## Match the actual provider protocol

These source checks were made on 2026-10-14. Recheck capabilities for the installed consumer version before init.

| Provider      | Native contract and verification boundary                                                                                      |
| ------------- | ------------------------------------------------------------------------------------------------------------------------------ |
| Claude Code   | Settings use the Claude schema above.                                                                                          |
| Claude Code   | `Edit`/`Write` tool input has `file_path`. Other edit tools need their own coverage.                                           |
| Claude Code   | Use the documented pre-tool denial and post-tool event contracts. See [hooks](https://code.claude.com/docs/en/hooks).          |
| Codex         | Use native [permission profiles](https://learn.chatgpt.com/docs/permissions), not Claude JSON.                                 |
| Codex         | Profiles are beta and do not compose with legacy sandbox settings.                                                             |
| Codex         | Domain rules require an active network proxy. Networking enabled alone is unrestricted.                                        |
| Codex         | Web-search filters do not constrain shell networking.                                                                          |
| Codex         | Confirm host enforcement and denied-read glob expansion.                                                                       |
| Codex hooks   | Trusted project `.codex/hooks.json` or inline hooks are documented. Review/trust is required.                                  |
| Codex hooks   | `apply_patch` carries patch text in `tool_input.command`, so extract every affected path. Do not assume `file_path`.           |
| Codex hooks   | See [hook coverage and denial protocol](https://learn.chatgpt.com/docs/hooks).                                                 |
| Copilot CLI   | Use its [native tool permission rules](https://docs.github.com/en/copilot/how-tos/copilot-cli/use-copilot-cli/allowing-tools). |
| Copilot CLI   | A secret-file `Read` deny equivalent is not established here.                                                                  |
| Copilot CLI   | Do not invent `permissions.allow`/`permissions.deny` settings. Do not claim OS confinement.                                    |
| Copilot hooks | Native `toolArgs` and compatible `tool_input` payloads differ.                                                                 |
| Copilot hooks | Verify actual tool shapes and the [hook protocol](https://docs.github.com/en/copilot/reference/hooks-reference).               |
| Copilot hooks | Command pre-tool errors deny, but timeouts and HTTP failures can fail open.                                                    |
| Copilot hooks | CLI and cloud-agent coverage differ.                                                                                           |

Illustrative native Codex (`~/.codex/config.toml` + `.codex/hooks.json`) and Copilot CLI (`.github/hooks/hooks.json` +
`--allow-tool`) fragments (never copy Claude JSON):

```toml
# ~/.codex/config.toml (beta permission profile; does not compose with legacy sandbox settings)
[profiles.inner_loop]
approval_policy = "on-request"
# Restrict allowed commands to reviewed scripts only (e.g., npm run build, npm test, npm run lint).
```

```json
// .github/hooks/hooks.json (Copilot CLI; HTTP hooks and timeouts can fail open — prefer local command hooks)
{
    "version": 1,
    "hooks": {
        "postToolUse": [{ "type": "command", "bash": "node .github/hooks/post-edit-format.cjs" }]
    }
}
```

This guide defines requirements for a future selected consumer implementation. It does not ship a generic dispatcher or
path-guard library. After the consumer approves the concrete setup and checks, test any tailored helper. Run that test
with that provider's actual payloads and refusal protocol. Record versions, commands, results and remaining gaps. Do not
count these documentation examples as evidence that the installed provider enforces the rule.
