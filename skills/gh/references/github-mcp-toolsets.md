# GitHub MCP toolsets and common workflows: reference for the `gh` skill

## Toolsets

Six MCP toolsets are on by default:

| Toolset         | Coverage                                                        |
| --------------- | --------------------------------------------------------------- |
| `context`       | Identity and team awareness (`get_me`, `get_teams`)             |
| `repos`         | Files, tree, branches, commits, tags, forks, code/commit search |
| `issues`        | Issues, comments, labels, sub-issues, fields, search            |
| `pull_requests` | PRs, reviews, diffs, merges, status checks, search              |
| `users`         | User search                                                     |
| `copilot`       | Copilot issue assignment and review requests                    |

Others (`actions`, `code_quality`, `code_security`, `discussions`, `git`, `secret_protection`, and more) are enabled on
request. There is **no `search` toolset**. Each search tool sits in the toolset it searches. See
[github-mcp-server.md](github-mcp-server.md) for the complete list and for the server-side controls that narrow what it
exposes.

## Common workflows

### Issues

| Action              | MCP Tool                            |
| ------------------- | ----------------------------------- |
| Create issue        | `issue_write` (method: create)      |
| Update issue        | `issue_write` (method: update)      |
| Get issue details   | `issue_read` (method: get)          |
| List issue comments | `issue_read` (method: get_comments) |
| Add comment         | `add_issue_comment`                 |
| Search issues       | `search_issues`                     |
| Add sub-issue       | `sub_issue_write`                   |
| List issue fields   | `list_issue_fields`                 |
| Assign Copilot      | `assign_copilot_to_issue`           |

### Pull requests

| Action                        | MCP Tool                                                                                   |
| ----------------------------- | ------------------------------------------------------------------------------------------ |
| Create PR                     | `create_pull_request`                                                                      |
| Get PR details                | `pull_request_read` (method: get)                                                          |
| Get PR diff                   | `pull_request_read` (method: get_diff)                                                     |
| Get PR files                  | `pull_request_read` (method: get_files)                                                    |
| Review comments               | `pull_request_read` (method: get_review_comments)                                          |
| Get combined PR commit status | `pull_request_read` (method: get_status)                                                   |
| Get PR check runs             | `pull_request_read` (method: get_check_runs)                                               |
| Get check-run annotations     | Checks API fallback: `gh api /repos/{owner}/{repo}/check-runs/{id}/annotations --paginate` |
| Submit review                 | `pull_request_review_write`                                                                |
| Request a Copilot review      | `request_copilot_review`                                                                   |
| Merge PR                      | `merge_pull_request` — merging follows [Repository policy](../SKILL.md#repository-policy)  |
| Update PR branch              | `update_pull_request_branch`                                                               |

### Repositories

| Action             | MCP Tool                              |
| ------------------ | ------------------------------------- |
| Get file contents  | `get_file_contents`                   |
| List branches      | `list_branches`                       |
| Create branch      | `create_branch`                       |
| Push files         | `push_files`                          |
| List commits       | `list_commits`                        |
| Search code        | `search_code`                         |
| Search commits     | `search_commits`                      |
| Repository tree    | `get_repository_tree` (`git` toolset) |
| List collaborators | `list_repository_collaborators`       |
| List tags          | `list_tags`                           |
| List releases      | `list_releases`                       |
