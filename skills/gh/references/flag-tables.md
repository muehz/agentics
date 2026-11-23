# GitHub tool coverage, flags, and API reference

Tool selection matrices, CLI/MCP flags, Actions commands, direct API calls, and policy settings referenced by
[SKILL.md](../SKILL.md).

## Tool coverage

```text
1. GitHub MCP server tools (preferred for structured operations without a shell)
2. gh CLI commands (fallback for scripting, Actions, auth, and uncovered operations)
```

| Path           | Covered areas                                                                                                                                                                                                                                                                                                                                                                                                       |
| -------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **MCP server** | Issues (create, update, search, comment, sub-issues, labels, org custom fields); PRs (create, administrative review, merge, diff/files/status); repo contents/tree/branches/commits/tags/push files/collaborators; code/commit/issue/user search; releases; users/teams; code scanning, code quality, secret scanning; Projects v2 (items, views, status updates, iteration fields); Actions workflow-run triggers. |
| **`gh` CLI**   | `gh run watch` (stream a run to completion); check-run annotations (`gh api .../annotations --paginate`, standing exception); Codespaces; extensions; org settings; auth (login, token, SSH keys); `gh api` (REST/GraphQL); branch ref deletion (`DELETE`); JSON, `jq`, and table formatting.                                                                                                                       |

## Host and repository flags

| Condition                    | Required flag / behavior                                                                                                                                   |
| ---------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Valid forge host syntax      | DNS labels (`[A-Za-z0-9-]`, inner hyphens) with optional `:<port>`. Reject whitespace, `;`, `$`, quotes, backticks, or `/` as unsupported; run no command. |
| Supported host               | `github.com`, or any valid host known to `gh auth status --hostname '<host>'`.                                                                             |
| Unsupported host             | Report `host not supported: <host>` once, name `gh auth login --hostname <host>`, finish local work, and run no `gh` command.                              |
| Off `github.com` (`gh api`)  | Pass `--hostname {host}` on every call (`gh api` defaults to `github.com`).                                                                                |
| Off `github.com` (`--repo`)  | Pass `--repo {host}/{owner}/{repo}` so reads and writes never cross hosts.                                                                                 |
| Off `github.com` (MCP tools) | MCP tools take no host; switch to the CLI path unless the user confirms the session's MCP server targets that host.                                        |

## Repository policy settings

Read-only settings live in `.agentics/hooks.json` (repository root) and `~/.agentics/hooks.json`. A repository key
overrides the user key; unset keys default to on; missing files change nothing; unreadable or invalid JSON ignores both
files and keeps every setting on. `gh` never edits either file.

| Setting                                 | Governs                                                     | When off                                                                                                                                |
| --------------------------------------- | ----------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------- |
| `policy.draft_prs`                      | Draft PR creation, marking PRs ready, and `gh`'s merge rule | Ready creation and marking ready on named request; merge only on named request after verifying target, review decision, and check state |
| `policy.block_review_thread_changes`    | Resolving or reopening review threads                       | Allowed on user request                                                                                                                 |
| `policy.block_security_alert_dismissal` | Dismissing code/secret/Dependabot security alerts           | Allowed on user request                                                                                                                 |

Returning a ready PR to draft (`gh pr ready <number> --undo`) is not governed by a policy setting: check
`gh pr view <number> --json isDraft` before and after, run only on named request, and never convert unasked.

## Running GitHub Actions

MCP handles workflow triggers, re-runs, and cancels via `actions_run_trigger` (`run_workflow`, re-run, cancel). Use the
CLI when no MCP server is configured, or to watch a run (CLI only):

```bash
# Trigger a workflow manually (MCP: actions_run_trigger, method run_workflow)
gh workflow run deploy.yml --ref main -f environment=staging

# Watch a running workflow — CLI only; no MCP tool streams a run to completion
gh run watch 12345
```

## Direct API access

Off `github.com`, every call below passes `--hostname {host}` with the host resolved in Part 1 preflight.

```bash
# REST: set issue type directly (gh issue create --type / gh issue edit --type cover the common case;
# this is only needed for scripting or when bypassing the CLI's built-in type support)
gh api repos/{owner}/{repo}/issues/{number} -X PATCH -f type="Bug"

# GraphQL: query org-level custom issue fields (dates, priority, etc.) when no MCP server is
# configured. The CLI exposes only the built-in issue type. With MCP, prefer `list_issue_fields`
# and `set_issue_fields`; see references/issues/issue-fields.md
gh api graphql -f query='{ ... }'
```

## Stack mode commands and flags

| Command / flag                            | Behavior and safety rule                                                                                                                                                                                    |
| ----------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `gh stack link --base <trunk> <pr> <b>`   | Links one new branch onto an existing PR (or `gh stack link <N> <b>` into stack `N`). Hand each branch to `git push` first so `link` only links.                                                            |
| `gh stack init` / `add` / `submit --auto` | Builds a new stack from scratch; hand each branch to `git push` first so `submit --auto` only creates/links PRs.                                                                                            |
| `--open` on `gh stack link` / `submit`    | Marks new **and existing** PRs (including lower PRs) ready for review; governed by [Repository policy](../SKILL.md#repository-policy). Default creates drafts; verify with `gh pr view <n> --json isDraft`. |
| `gh stack push` / `gh stack sync`         | **Never run by `gh`**: both force-push.                                                                                                                                                                     |
| Exit code `9`                             | Stacked PRs are not enabled on the repository; report "base-chained, not a stack: stacked PRs aren't enabled here" and the null `stack` result.                                                             |

Full stack command map, prerequisites, and rebase-ripple rules: [stack.md](stack.md).
