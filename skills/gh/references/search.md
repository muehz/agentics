# Advanced issue search

The `search_issues` MCP tool uses GitHub's issue search query format for cross-repo searches.

## When to use search vs list vs advanced search

| Capability                                    | `list_issues` (MCP)   | `search_issues` (MCP)   | Advanced search (`gh api`)           |
| --------------------------------------------- | --------------------- | ----------------------- | ------------------------------------ |
| **Scope**                                     | Single repo only      | Cross-repo, cross-org   | Cross-repo, cross-org                |
| **Issue field filters** (`field.priority:P0`) | No                    | No                      | **Yes** (dot notation, experimental) |
| **Issue type filter** (`type:Bug`)            | No                    | Yes                     | Yes                                  |
| **Boolean logic** (AND/OR/NOT, nesting)       | No                    | Yes (implicit AND only) | **Yes** (explicit AND/OR/NOT)        |
| **Label/state/date filters**                  | Yes                   | Yes                     | Yes                                  |
| **Assignee/author/mentions**                  | No                    | Yes                     | Yes                                  |
| **Negation** (`-label:x`, `no:label`)         | No                    | Yes                     | Yes                                  |
| **Text search** (title/body/comments)         | No                    | Yes                     | Yes                                  |
| **`since` filter**                            | Yes                   | No                      | No                                   |
| **Result limit**                              | No cap (paginate all) | 1,000 max               | 1,000 max                            |
| **How to call**                               | MCP tool directly     | MCP tool directly       | `gh api` with `advanced_search=true` |

**Decision guide:**

- Use `list_issues` for a single repo with simple filters (state, labels, recent updates).
- Use `search_issues` for cross-repo search, text search, author/assignee, or issue types.
- Use `gh api` with `advanced_search=true` for issue field values (Priority, dates, custom fields) or complex boolean
  logic. Field qualifiers are experimental and can return 0 results for matching issues, so read
  [Issue field search](#issue-field-search) before you trust an empty result.

## Query syntax

The `query` parameter is a string of search terms and qualifiers. A space between terms is implicit AND.

### Scoping

```text
repo:owner/repo       # Single repo (auto-added if you pass owner+repo params)
org:github            # All repos in an org
user:octocat          # All repos owned by user
in:title              # Search only in title
in:body               # Search only in body
in:comments           # Search only in comments
```

### State & close reason

```text
is:open               # Open issues (auto-added: is:issue)
is:closed             # Closed issues
reason:completed      # Closed as completed
reason:"not planned"  # Closed as not planned
```

### People

```text
author:username       # Created by
assignee:username     # Assigned to
mentions:username     # Mentions user
commenter:username    # Has comment from
involves:username     # Author OR assignee OR mentioned OR commenter
author:@me            # Current authenticated user
team:org/team         # Team mentioned
```

### Labels, milestones, projects, types

```text
label:"bug"                 # Has label (quote multi-word labels)
label:bug label:priority    # Has BOTH labels (AND)
label:bug,enhancement       # Has EITHER label (OR)
-label:wontfix              # Does NOT have label
milestone:"v2.0"            # In milestone
project:github/57           # In project board
type:"Bug"                  # Issue type
```

### Missing metadata

```text
no:label              # No labels assigned
no:milestone          # No milestone
no:assignee           # Unassigned
no:project            # Not in any project
```

### Dates

All date qualifiers support `>`, `<`, `>=`, `<=`, and range (`..`) operators with ISO 8601 format:

```text
created:>YYYY-MM-DD              # Created after that date
updated:>=YYYY-MM-DD             # Updated on or after that date
closed:YYYY-MM-DD..YYYY-MM-DD    # Closed within the range
created:<YYYY-MM-DD              # Created before that date
```

Replace every `YYYY-MM-DD` with a real date before you run the query. The search rejects the literal text with HTTP 422.
For a relative window, compute the date from today:

```bash
date -d '7 days ago' +%F   # GNU date (Linux, Git Bash)
date -v-7d +%F             # BSD date (macOS)
```

```powershell
(Get-Date).AddDays(-7).ToString('yyyy-MM-dd')
```

### Linked content

```text
linked:pr             # Issue has a linked PR
-linked:pr            # Issues not yet linked to any PR
linked:issue          # PR is linked to an issue
```

### Numeric filters

```text
comments:>10          # More than 10 comments
comments:0            # No comments
interactions:>100     # Reactions + comments > 100
reactions:>50         # More than 50 reactions
```

### Boolean logic & nesting

> **Note:** Boolean operators apply to **advanced search** via `gh api` only. The `search_issues` MCP tool uses
> implicit-AND and does not support explicit `OR`/`NOT`.

Use `AND`, `OR`, and parentheses (up to 5 levels deep, max 5 operators):

```text
label:bug AND assignee:octocat
assignee:octocat OR assignee:hubot
(type:"Bug" AND label:P1) OR (type:"Feature" AND label:P1)
-author:app/dependabot                      # Exclude issues from the Dependabot app
```

## Common query patterns

**Unassigned bugs:**

```text
repo:owner/repo type:"Bug" no:assignee is:open
```

**Issues closed this week** (`YYYY-MM-DD` is seven days before today):

```text
repo:owner/repo is:closed closed:>=YYYY-MM-DD
```

**Stale open issues (no updates in 90 days)** (`YYYY-MM-DD` is 90 days before today):

```text
repo:owner/repo is:open updated:<YYYY-MM-DD
```

**Open issues without a linked PR (needs work):**

```text
repo:owner/repo is:open -linked:pr
```

**Issues I'm involved in across an org:**

```text
org:github involves:@me is:open
```

**High-activity issues:**

```text
repo:owner/repo is:open comments:>20
```

**Issues by type and priority label:**

```text
repo:owner/repo type:"Epic" label:P1 is:open
```

## Issue field search

> **Reliability warning:** The `field.name:value` search qualifier syntax is experimental and may return 0 results even
> when issues match. To filter reliably by field values, use the GraphQL bulk query approach in
> [issue-fields.md](issues/issue-fields.md#searching-by-field-values).

In theory, the `field.name:value` qualifier can search issue fields in **advanced search mode**. This works in the web
UI, but results from the API are inconsistent.

### REST API

Add `advanced_search=true` as a query parameter:

```bash
gh api "search/issues?q=org:github+field.priority:P0+type:Epic+is:open&advanced_search=true" \
  --jq '.items[] | "#\(.number): \(.title)"'
```

### GraphQL

Use `type: ISSUE_ADVANCED` instead of `type: ISSUE`:

```graphql
{
    search(query: "org:github field.priority:P0 type:Epic is:open", type: ISSUE_ADVANCED, first: 10) {
        issueCount
        nodes {
            ... on Issue {
                number
                title
            }
        }
    }
}
```

### Issue field qualifiers

The syntax uses **dot notation** with the field's slug name (lowercase, hyphens for spaces):

```text
field.priority:P0                  # Single-select field equals value
field.priority:P1                  # Different option value
field.target-date:>=YYYY-MM-DD     # Date comparison
has:field.priority                 # Has any value set
no:field.priority                  # Has no value set
```

### Common field search patterns

**P0 epics across an org:**

```text
org:github field.priority:P0 type:Epic is:open
```

**Issues with a target date this quarter** (the two dates are the first and last day of the quarter):

```text
org:github field.target-date:>=YYYY-MM-DD field.target-date:<=YYYY-MM-DD is:open
```

**Open bugs missing priority:**

```text
org:github no:field.priority type:Bug is:open
```

## Limitations

- Query text: max 256 characters (operators and qualifiers do not count)
- Boolean operators: max 5 AND/OR/NOT per query
- Results: max 1,000 total (use `list_issues` if you need all issues)
- Repo scan: searches up to 4,000 repositories that match
- Rate limit: 30 requests/minute for authenticated search
- Issue field search requires `advanced_search=true` (REST) or `ISSUE_ADVANCED` (GraphQL); not available through MCP
  `search_issues`
