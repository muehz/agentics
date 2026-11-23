# GitHub MCP server instructions

**Source**: [github/github-mcp-server](https://github.com/github/github-mcp-server/tree/main/pkg/github) **Mapping
File**:
[pkg/workflow/data/github_toolsets_permissions.json](https://github.com/github/gh-aw/blob/main/pkg/workflow/data/github_toolsets_permissions.json)
**Server version**: v1.10.1 **Last Updated**: 2026-08-21

## Overview

The server runs in two modes:

- **Remote mode**: Connects to GitHub's hosted MCP endpoint (`https://api.githubcopilot.com/mcp/`)
- **Local mode**: Runs the server as a local subprocess: the `github-mcp-server` binary (`github-mcp-server stdio`) or
  its Docker image `ghcr.io/github/github-mcp-server`

### Authentication

**Remote mode**: Uses a Bearer token in the Authorization header:

```text
Authorization: Bearer <github-token>
```

**Read-only mode**: Add the `X-MCP-Readonly: true` header to restrict to read operations only:

```text
X-MCP-Readonly: true
```

**Local mode**: Reads a token from the `GITHUB_PERSONAL_ACCESS_TOKEN` environment variable. The upstream README also
documents an OAuth login for the local server, which the token overrides when both are set.

## Configuration

### In agentic workflows

```yaml
tools:
    github:
        toolsets: [default] # or specific toolsets
        # Optional: GitHub App authentication
        app-id: ${{ vars.APP_ID }}
        private-key: ${{ secrets.APP_PRIVATE_KEY }}
```

> **Do NOT use `mode: remote`** in GitHub Actions workflows. Remote mode requires a PAT or GitHub App token with MCP
> access, so `GITHUB_TOKEN` does not work. The default `mode: local` (Docker-based) works with `GITHUB_TOKEN`. Always
> use `mode: local`.

### Toolset options

- `[default]`: the six default toolsets below
- `[all]`: Enable all toolsets
- Specific toolsets: `[repos, issues, pull_requests, discussions]`
- Extend defaults: `[default, discussions, actions]`

## Recommended default toolsets

The server marks these six toolsets `Default: true`. An unconfigured server exposes them:

| Toolset         | Rationale                                                                                     |
| --------------- | --------------------------------------------------------------------------------------------- |
| `context`       | Identity and team awareness (`get_me`, `get_teams`), which GitHub-aware agents need           |
| `repos`         | Core repository operations (read files, list commits/branches), which most workflows need     |
| `issues`        | Issue management (read, comment, create), common in CI/CD and automation workflows            |
| `pull_requests` | PR operations (read, create, review), which are critical for code review and merge automation |
| `users`         | User lookup (`search_users`)                                                                  |
| `copilot`       | Copilot assignment and review requests (`assign_copilot_to_issue`, `request_copilot_review`)  |

**Enable explicitly when needed** (not in defaults):

| Toolset                 | When to Enable                                         |
| ----------------------- | ------------------------------------------------------ |
| `actions`               | Workflow introspection and the ability to trigger runs |
| `code_quality`          | Code quality findings                                  |
| `code_security`         | Code scanning alert management                         |
| `copilot_issue_intents` | Intent-aware Copilot issue assignment (opt-in)         |
| `dependabot`            | Dependency vulnerability management                    |
| `discussions`           | Community discussion workflows                         |
| `gists`                 | Gist creation and management                           |
| `git`                   | Low-level Git reads (`get_repository_tree` only)       |
| `labels`                | Label management automation                            |
| `notifications`         | Notification processing agents                         |
| `orgs`                  | Organization-level security advisories                 |
| `projects`              | GitHub Projects automation (requires PAT)              |
| `secret_protection`     | Secret scanning alert management                       |
| `security_advisories`   | Advisory database queries                              |
| `stargazers`            | Star/unstar repository operations                      |

There is **no `search` toolset**. Each search tool lives in the toolset it searches:

- `repos`: `search_code`, `search_commits`, and `search_repositories`
- `issues`: `search_issues`
- `pull_requests`: `search_pull_requests`
- `orgs`: `search_orgs`
- `users`: `search_users`

## Tools by toolset

### context

**Description**: GitHub context and environment (current user, teams) **Source**:
[`pkg/github/context_tools.go`](https://github.com/github/github-mcp-server/blob/main/pkg/github/context_tools.go)

| Tool               | Purpose                                      | Key Parameters     |
| ------------------ | -------------------------------------------- | ------------------ |
| `get_me`           | Get details of the authenticated user        | —                  |
| `get_team_members` | List members of a GitHub team                | `org`, `team_slug` |
| `get_teams`        | List teams the authenticated user belongs to | `org`              |

---

### repos

**Description**: Repository operations **Source**:
[`pkg/github/repositories.go`](https://github.com/github/github-mcp-server/blob/main/pkg/github/repositories.go)

| Tool                            | Purpose                                 | Key Parameters                                          |
| ------------------------------- | --------------------------------------- | ------------------------------------------------------- |
| `create_branch`                 | Create a new branch                     | `owner`, `repo`, `branch`, `from_branch`                |
| `create_or_update_file`         | Create or update a file in a repository | `owner`, `repo`, `path`, `content`, `message`, `branch` |
| `create_repository`             | Create a new GitHub repository          | `name`, `description`, `private`, `auto_init`           |
| `delete_file`                   | Delete a file from a repository         | `owner`, `repo`, `path`, `message`, `branch`            |
| `delete_repository`             | Delete a repository                     | `owner`, `repo`                                         |
| `fork_repository`               | Fork a repository                       | `owner`, `repo`, `organization`                         |
| `get_commit`                    | Get details of a specific commit        | `owner`, `repo`, `sha`                                  |
| `get_file_contents`             | Read file or directory contents         | `owner`, `repo`, `path`, `ref`                          |
| `get_latest_release`            | Get the latest release for a repository | `owner`, `repo`                                         |
| `get_release_by_tag`            | Get a release by its tag name           | `owner`, `repo`, `tag`                                  |
| `get_tag`                       | Get details of a specific tag           | `owner`, `repo`, `tag`                                  |
| `list_branches`                 | List branches in a repository           | `owner`, `repo`, `page`, `per_page`                     |
| `list_commits`                  | List commits in a repository            | `owner`, `repo`, `sha`, `path`, `page`                  |
| `list_releases`                 | List all releases for a repository      | `owner`, `repo`, `page`, `per_page`                     |
| `list_tags`                     | List tags in a repository               | `owner`, `repo`, `page`, `per_page`                     |
| `list_repository_collaborators` | List collaborators on a repository      | `owner`, `repo`, `page`, `per_page`                     |
| `push_files`                    | Push multiple files in a single commit  | `owner`, `repo`, `branch`, `files`, `message`           |
| `search_code`                   | Search code across repositories         | `query`, `page`, `per_page`                             |
| `search_commits`                | Search commits across repositories      | `query`, `page`, `per_page`                             |
| `search_repositories`           | Search for repositories                 | `query`, `page`, `per_page`                             |

---

### issues

**Description**: Issue management **Source**:
[`pkg/github/issues.go`](https://github.com/github/github-mcp-server/blob/main/pkg/github/issues.go)

| Tool                | Purpose                                     | Key Parameters                                          |
| ------------------- | ------------------------------------------- | ------------------------------------------------------- |
| `add_issue_comment` | Add a comment to an issue                   | `owner`, `repo`, `issue_number`, `body`                 |
| `issue_read`        | Read issue details and comments             | `owner`, `repo`, `issue_number`                         |
| `issue_write`       | Create or update an issue                   | `owner`, `repo`, `title`, `body`, `labels`, `assignees` |
| `get_label`         | Get details of a specific label             | `owner`, `repo`, `name`                                 |
| `list_issue_fields` | List issue field definitions for a repo/org | `owner`, `repo`                                         |
| `list_issue_types`  | List available issue types for a repository | `owner`, `repo`                                         |
| `list_issues`       | List issues in a repository                 | `owner`, `repo`, `state`, `labels`, `page`              |
| `search_issues`     | Search issues across GitHub                 | `query`, `page`, `per_page`                             |
| `sub_issue_write`   | Create or manage sub-issues                 | `owner`, `repo`, `issue_number`                         |

---

### pull_requests

**Description**: Pull request operations **Source**:
[`pkg/github/pullrequests.go`](https://github.com/github/github-mcp-server/blob/main/pkg/github/pullrequests.go)

| Tool                                           | Purpose                                | Key Parameters                                       |
| ---------------------------------------------- | -------------------------------------- | ---------------------------------------------------- |
| `add_comment_to_pending_review`                | Add a comment to a pending PR review   | `owner`, `repo`, `pull_number`, `review_id`          |
| `add_reply_to_pull_request_comment`            | Reply to a PR review comment           | `owner`, `repo`, `pull_number`, `comment_id`, `body` |
| `create_pull_request`                          | Create a new pull request              | `owner`, `repo`, `title`, `body`, `head`, `base`     |
| `list_pull_requests`                           | List pull requests in a repository     | `owner`, `repo`, `state`, `head`, `base`             |
| `merge_pull_request`                           | Merge a pull request                   | `owner`, `repo`, `pull_number`, `merge_method`       |
| `pull_request_read`                            | Read PR details, reviews, and comments | `owner`, `repo`, `pull_number`                       |
| `pull_request_read` (method: `get_status`)     | Get the combined commit status         | `owner`, `repo`, `pull_number`, `method`             |
| `pull_request_read` (method: `get_check_runs`) | Get individual check runs              | `owner`, `repo`, `pull_number`, `method`             |
| `pull_request_review_write`                    | Create or submit a PR review           | `owner`, `repo`, `pull_number`, `event`, `body`      |
| `search_pull_requests`                         | Search pull requests across GitHub     | `query`, `page`, `per_page`                          |
| `update_pull_request`                          | Update PR title, body, or state        | `owner`, `repo`, `pull_number`, `title`, `body`      |
| `update_pull_request_branch`                   | Update PR branch with latest base      | `owner`, `repo`, `pull_number`                       |

Neither `get_status` nor `get_check_runs` returns check-run annotations. Fetch them through the Checks API fallback:
`gh api /repos/{owner}/{repo}/check-runs/{id}/annotations --paginate`.

---

### actions

**Description**: GitHub Actions workflows **Source**:
[`pkg/github/actions.go`](https://github.com/github/github-mcp-server/blob/main/pkg/github/actions.go)

| Tool                  | Purpose                                   | Key Parameters                                  |
| --------------------- | ----------------------------------------- | ----------------------------------------------- |
| `actions_get`         | Get details of a specific workflow run    | `owner`, `repo`, `run_id`                       |
| `actions_list`        | List GitHub Actions workflows and runs    | `owner`, `repo`, `workflow_id`                  |
| `actions_run_trigger` | Trigger a workflow run                    | `owner`, `repo`, `workflow_id`, `ref`, `inputs` |
| `get_job_logs`        | Download logs for a specific workflow job | `owner`, `repo`, `job_id`                       |

---

### code_quality

**Description**: Code quality findings **Source**:
[`pkg/github/code_quality.go`](https://github.com/github/github-mcp-server/blob/main/pkg/github/code_quality.go)

| Tool                       | Purpose                                       | Key Parameters                |
| -------------------------- | --------------------------------------------- | ----------------------------- |
| `get_code_quality_finding` | Get one code quality finding for a repository | `owner`, `repo`, `finding_id` |

---

### code_security

**Description**: Code scanning alerts **Source**:
[`pkg/github/code_scanning.go`](https://github.com/github/github-mcp-server/blob/main/pkg/github/code_scanning.go)

| Tool                        | Purpose                                       | Key Parameters                       |
| --------------------------- | --------------------------------------------- | ------------------------------------ |
| `get_code_scanning_alert`   | Get details of a specific code scanning alert | `owner`, `repo`, `alert_number`      |
| `list_code_scanning_alerts` | List code scanning alerts for a repository    | `owner`, `repo`, `state`, `severity` |

---

### copilot

**Description**: Copilot coding agent and review requests (a **default** toolset) **Source**:
[`pkg/github/copilot.go`](https://github.com/github/github-mcp-server/blob/main/pkg/github/copilot.go)

| Tool                      | Purpose                               | Key Parameters                 |
| ------------------------- | ------------------------------------- | ------------------------------ |
| `assign_copilot_to_issue` | Assign Copilot to an issue            | `owner`, `repo`, `issueNumber` |
| `request_copilot_review`  | Request a Copilot code review on a PR | `owner`, `repo`, `pullNumber`  |

---

### copilot_issue_intents

**Description**: Intent-aware Copilot assignment, kept out of the defaults so its extra inputs add no schema weight to
the default tools. **Source**:
[`pkg/github/copilot.go`](https://github.com/github/github-mcp-server/blob/main/pkg/github/copilot.go)

| Tool                                  | Purpose                                                                                                    | Key Parameters                              |
| ------------------------------------- | ---------------------------------------------------------------------------------------------------------- | ------------------------------------------- |
| `assign_copilot_to_issue_with_intent` | Assign Copilot with a rationale and confidence. Prefer this over `assign_copilot_to_issue` where available | `owner`, `repo`, `issueNumber`, `rationale` |

---

### dependabot

**Description**: Dependabot alerts **Source**:
[`pkg/github/dependabot.go`](https://github.com/github/github-mcp-server/blob/main/pkg/github/dependabot.go)

| Tool                     | Purpose                                    | Key Parameters                       |
| ------------------------ | ------------------------------------------ | ------------------------------------ |
| `get_dependabot_alert`   | Get details of a specific Dependabot alert | `owner`, `repo`, `alert_number`      |
| `list_dependabot_alerts` | List Dependabot alerts for a repository    | `owner`, `repo`, `state`, `severity` |

---

### discussions

**Description**: GitHub Discussions **Source**:
[`pkg/github/discussions.go`](https://github.com/github/github-mcp-server/blob/main/pkg/github/discussions.go)

| Tool                         | Purpose                                     | Key Parameters                       |
| ---------------------------- | ------------------------------------------- | ------------------------------------ |
| `discussion_comment_write`   | Add, reply, update, delete, or mark answer  | `discussionId`, `method`, `body`     |
| `get_discussion`             | Get details of a specific discussion        | `owner`, `repo`, `discussion_number` |
| `get_discussion_comments`    | Get comments for a specific discussion      | `owner`, `repo`, `discussion_number` |
| `list_discussion_categories` | List discussion categories for a repository | `owner`, `repo`                      |
| `list_discussions`           | List discussions in a repository            | `owner`, `repo`, `category_id`       |

---

### gists

**Description**: Gist operations **Source**:
[`pkg/github/gists.go`](https://github.com/github/github-mcp-server/blob/main/pkg/github/gists.go)

| Tool          | Purpose                   | Key Parameters                    |
| ------------- | ------------------------- | --------------------------------- |
| `create_gist` | Create a new gist         | `description`, `files`, `public`  |
| `get_gist`    | Get a specific gist by ID | `gist_id`                         |
| `list_gists`  | List gists for a user     | `username`, `page`, `per_page`    |
| `update_gist` | Update an existing gist   | `gist_id`, `description`, `files` |

---

### git

**Description**: Low-level Git reads **Source**:
[`pkg/github/git.go`](https://github.com/github/github-mcp-server/blob/main/pkg/github/git.go)

| Tool                  | Purpose                                         | Key Parameters                      |
| --------------------- | ----------------------------------------------- | ----------------------------------- |
| `get_repository_tree` | Get the file and directory tree at a ref or SHA | `owner`, `repo`, `ref`, `recursive` |

This is the whole toolset. Branches, commits, and tags live in `repos`.

---

### labels

**Description**: Label management **Source**:
[`pkg/github/labels.go`](https://github.com/github/github-mcp-server/blob/main/pkg/github/labels.go)

| Tool          | Purpose                         | Key Parameters                                  |
| ------------- | ------------------------------- | ----------------------------------------------- |
| `get_label`   | Get details of a specific label | `owner`, `repo`, `name`                         |
| `label_write` | Create or update a label        | `owner`, `repo`, `name`, `color`, `description` |
| `list_label`  | List labels in a repository     | `owner`, `repo`, `page`, `per_page`             |

---

### notifications

**Description**: Notification management **Source**:
[`pkg/github/notifications.go`](https://github.com/github/github-mcp-server/blob/main/pkg/github/notifications.go)

| Tool                                          | Purpose                                       | Key Parameters                 |
| --------------------------------------------- | --------------------------------------------- | ------------------------------ |
| `dismiss_notification`                        | Dismiss a specific notification               | `notification_id`              |
| `get_notification_details`                    | Get details of a specific notification        | `notification_id`              |
| `list_notifications`                          | List user notifications                       | `all`, `participating`, `page` |
| `manage_notification_subscription`            | Manage notification subscription for a thread | `thread_id`, `subscribed`      |
| `manage_repository_notification_subscription` | Manage notifications for a repository         | `owner`, `repo`, `subscribed`  |
| `mark_all_notifications_read`                 | Mark all notifications as read                | `last_read_at`                 |

---

### orgs

**Description**: Organization operations **Source**:
[`pkg/github/security_advisories.go`](https://github.com/github/github-mcp-server/blob/main/pkg/github/security_advisories.go)
(for `list_org_repository_security_advisories`)

| Tool                                      | Purpose                                          | Key Parameters              |
| ----------------------------------------- | ------------------------------------------------ | --------------------------- |
| `list_org_repository_security_advisories` | List security advisories for all repos in an org | `org`, `state`              |
| `search_orgs`                             | Search GitHub organizations                      | `query`, `page`, `per_page` |

---

### projects

**Description**: GitHub Projects (requires a PAT; `GITHUB_TOKEN` does not support it) **Source**:
[`pkg/github/projects.go`](https://github.com/github/github-mcp-server/blob/main/pkg/github/projects.go)

| Tool             | Purpose                                                                                                                               | Key Parameters                                                                                                                        |
| ---------------- | ------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------- |
| `projects_get`   | Get details of a specific project, field, item, status update, or view (by `method`)                                                  | `method`, `owner`, `project_number`. Status-update and view lookups take the node ID `status_update_id` or `view_id` in place of both |
| `projects_list`  | List projects, fields, items, status updates, or views (by `method`)                                                                  | `method`, `owner`, `per_page`                                                                                                         |
| `projects_write` | Create a project. Add, update, or bulk-update items. Post a status update. Create an iteration field. Create, update, or delete views | `owner`, `project_number`, `method`                                                                                                   |

---

### secret_protection

**Description**: Secret scanning **Source**:
[`pkg/github/secret_scanning.go`](https://github.com/github/github-mcp-server/blob/main/pkg/github/secret_scanning.go)

| Tool                          | Purpose                                         | Key Parameters                  |
| ----------------------------- | ----------------------------------------------- | ------------------------------- |
| `get_secret_scanning_alert`   | Get details of a specific secret scanning alert | `owner`, `repo`, `alert_number` |
| `list_secret_scanning_alerts` | List secret scanning alerts for a repository    | `owner`, `repo`, `state`        |

---

### security_advisories

**Description**: Security advisories **Source**:
[`pkg/github/security_advisories.go`](https://github.com/github/github-mcp-server/blob/main/pkg/github/security_advisories.go)

| Tool                                  | Purpose                                            | Key Parameters                  |
| ------------------------------------- | -------------------------------------------------- | ------------------------------- |
| `get_global_security_advisory`        | Get a specific global security advisory            | `ghsa_id`                       |
| `list_global_security_advisories`     | List advisories from the GitHub Advisory Database  | `type`, `severity`, `ecosystem` |
| `list_repository_security_advisories` | List security advisories for a specific repository | `owner`, `repo`, `state`        |

---

### stargazers

**Description**: Repository stars **Source**:
[`pkg/github/repositories.go`](https://github.com/github/github-mcp-server/blob/main/pkg/github/repositories.go)

| Tool                        | Purpose                             | Key Parameters                 |
| --------------------------- | ----------------------------------- | ------------------------------ |
| `list_starred_repositories` | List repositories starred by a user | `username`, `page`, `per_page` |
| `star_repository`           | Star a repository                   | `owner`, `repo`                |
| `unstar_repository`         | Unstar a repository                 | `owner`, `repo`                |

---

### users

**Description**: User information (a **default** toolset) **Source**:
[`pkg/github/search.go`](https://github.com/github/github-mcp-server/blob/main/pkg/github/search.go)

| Tool           | Purpose                                              | Key Parameters              |
| -------------- | ---------------------------------------------------- | --------------------------- |
| `search_users` | Find users by username, real name, or profile detail | `query`, `page`, `per_page` |

---

### Not in the local server

Four tools ship only on the **remote** endpoint. A local server subprocess does not register them, so a workflow that
depends on one must run in remote mode.

| Tool                               | Toolset                    | Purpose                                   |
| ---------------------------------- | -------------------------- | ----------------------------------------- |
| `create_pull_request_with_copilot` | Copilot                    | Hand a task to the Copilot coding agent   |
| `get_copilot_space`                | Copilot Spaces             | Read one Copilot space                    |
| `list_copilot_spaces`              | Copilot Spaces             | List Copilot spaces                       |
| `github_support_docs_search`       | GitHub Support Docs Search | Search GitHub's own support documentation |

### Feature-flagged granular tools

The server can replace three consolidated tools with sets of single-purpose ones:

- `issue_write` splits into `create_issue`, `update_issue_title`, `update_issue_body`, `update_issue_state`,
  `update_issue_labels`, `update_issue_assignees`, `update_issue_milestone`, `update_issue_type`, and
  `set_issue_fields`.
- `update_pull_request` and `pull_request_review_write` split into `update_pull_request_title`,
  `update_pull_request_body`, `update_pull_request_state`, `update_pull_request_draft_state`,
  `request_pull_request_reviewers`, `create_pull_request_review`, `submit_pending_pull_request_review`,
  `delete_pending_pull_request_review`, `add_pull_request_review_comment`, and the two review-thread tools.

`set_issue_fields` writes the org custom fields `list_issue_fields` reads.

## Best practices

### Controls that narrow what the server exposes

Agentics ships **no MCP server configuration**. The consumer configures the server in host-level or project-level
config. The server-side controls below are advisory.

| Control                 | Local flag        | Remote header          | Effect                                                                                       |
| ----------------------- | ----------------- | ---------------------- | -------------------------------------------------------------------------------------------- |
| Read-only               | `--read-only`     | `X-MCP-Readonly: true` | Strictest. Every write tool disappears. It overrides everything else.                        |
| Exclude specific tools  | `--exclude-tools` | `X-MCP-Exclude-Tools`  | Hard removal by name. Beats both toolset enablement and per-tool enablement.                 |
| Enable toolsets         | `--toolsets`      | `X-MCP-Toolsets`       | Which groups are registered.                                                                 |
| Enable individual tools | `--tools`         | `X-MCP-Tools`          | Adds named tools on top of the enabled toolsets.                                             |
| Lockdown                | `--lockdown-mode` | `X-MCP-Lockdown: true` | Filters untrusted content. **Not a security boundary** — GitHub describes it as best-effort. |

**Excluded tools win over both `--toolsets` and `--tools`**: an excluded name stays gone even when its toolset is
enabled and the tool is named explicitly. In HTTP mode an operator-set lockdown is an upper bound. A request header can
turn lockdown on for itself but can never turn off what the operator set.

**Recommended exclusions for an automated session**: `merge_pull_request` and `delete_repository`. Merging follows
[Repository policy](../SKILL.md#repository-policy). Deleting a repository is an unconditional human action. A missing
tool cannot be called at all.

Treat prompt-injection risk from issue and PR bodies as unmitigated by lockdown.

### Permission requirements

Most toolsets work with the default `GITHUB_TOKEN` in GitHub Actions. Exceptions:

- `projects`: Requires a PAT (Personal Access Token) with `project` scope. `GITHUB_TOKEN` lacks it
- Security toolsets (`code_security`, `dependabot`, `secret_protection`, `security_advisories`): Require
  `security-events` permission. `security_advisories` (write) requires `security-events: write`
- `actions` (write for `actions_run_trigger`): Requires `actions: write` permission
- Other write operations: Require the token permissions listed under `write_permissions` in the JSON mapping

### Token scopes for remote mode

Remote mode with a PAT uses these token scopes:

- Basic read: `repo` scope
- Issues/PRs write: `repo` scope (covers everything)
- Projects: `project` scope
- Gists: `gist` scope
- Notifications: `notifications` scope
