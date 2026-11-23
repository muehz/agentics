# Issue dependencies (blocked by / blocking)

A dependency marks an issue as blocked by another issue, visible in the UI and trackable via API. No MCP tools exist for
dependencies. Use REST or GraphQL directly.

## Simplest path

For everyday dependency management, the `gh issue edit`/`gh issue create` native flags are simpler than REST or GraphQL.
Write the title and body to scratch files with the file-write tool. Load their locations into `$title_file` and
`$body_file` in the create call.

```bash
# Set dependencies at creation time
issue_title=$(cat -- "$title_file") || exit 1
gh issue create --title "$issue_title" --body-file "$body_file" --blocked-by 200,201 --blocking 300

# Add/remove on an existing issue
gh issue edit <issue_number> --add-blocked-by <number>
gh issue edit <issue_number> --remove-blocked-by <number>
gh issue edit <issue_number> --add-blocking <number>
gh issue edit <issue_number> --remove-blocking <number>
```

Use the REST/GraphQL sections below for bulk operations or to read `issueDependenciesSummary`/tracked-issue fields. The
CLI flags do not cover those operations.

## Use the REST API

**List issues that block this issue:**

```text
GET /repos/{owner}/{repo}/issues/{issue_number}/dependencies/blocked_by
```

**Add a blocking dependency:**

```text
POST /repos/{owner}/{repo}/issues/{issue_number}/dependencies/blocked_by
Body: { "issue_id": 12345 }
```

The `issue_id` is the numeric issue **ID** (not the issue number).

**Remove a blocking dependency:**

```text
DELETE /repos/{owner}/{repo}/issues/{issue_number}/dependencies/blocked_by/{issue_id}
```

## Use GraphQL

**Read dependencies:**

```graphql
{
    repository(owner: "OWNER", name: "REPO") {
        issue(number: 123) {
            blockedBy(first: 10) {
                nodes {
                    number
                    title
                    state
                }
            }
            blocking(first: 10) {
                nodes {
                    number
                    title
                    state
                }
            }
            issueDependenciesSummary {
                blockedBy
                blocking
                totalBlockedBy
                totalBlocking
            }
        }
    }
}
```

**Add a dependency:**

```graphql
mutation {
    addBlockedBy(input: { issueId: "BLOCKED_ISSUE_NODE_ID", blockingIssueId: "BLOCKING_ISSUE_NODE_ID" }) {
        blockingIssue {
            number
            title
        }
    }
}
```

**Remove a dependency:**

```graphql
mutation {
    removeBlockedBy(input: { issueId: "BLOCKED_ISSUE_NODE_ID", blockingIssueId: "BLOCKING_ISSUE_NODE_ID" }) {
        blockingIssue {
            number
            title
        }
    }
}
```

## Tracked issues (read-only)

GraphQL exposes task-list tracking relationships as read-only fields:

- `trackedIssues(first: N)`: issues tracked in this issue's task list
- `trackedInIssues(first: N)`: issues whose task lists reference this issue

GitHub sets these fields automatically when a task list references an issue (`- [ ] #123`). No mutation manages them.
