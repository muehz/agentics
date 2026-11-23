# Issue fields (GraphQL, private preview)

> **Private preview:** Issue fields are in private preview. Request access at
> <https://github.com/orgs/community/discussions/175366>

Issue fields are custom metadata (dates, text, numbers, single-select) defined at the organization level and set
per-issue. They are separate from labels, milestones, and assignees. Common examples: Start Date, Target Date, Priority,
Impact, Effort.

Earlier, the `GraphQL-Features: issue_fields` HTTP header was required. The header does no harm if you keep it. In
recent tests, reads succeed the same with or without it. For example, a query of `issueFieldValues` on an issue gives
the same result either way.

**Prefer issue fields over project fields** for issue metadata like dates, priority, or status. Issue fields live on the
issue and travel with it across projects and views. Project fields live on one project's item. Use project fields only
when issue fields are not available, or when the field is project-specific (for example, sprint iterations).

## Discovering available fields

List the org's fields before you set values:

```graphql
# Header: GraphQL-Features: issue_fields
{
    organization(login: "OWNER") {
        issueFields(first: 30) {
            nodes {
                __typename
                ... on IssueFieldDate {
                    id
                    name
                }
                ... on IssueFieldText {
                    id
                    name
                }
                ... on IssueFieldNumber {
                    id
                    name
                }
                ... on IssueFieldSingleSelect {
                    id
                    name
                    options {
                        id
                        name
                        color
                    }
                }
            }
        }
    }
}
```

To set a single-select field, you need the option `id`, not the name.

## Reading field values on an issue

```graphql
# Header: GraphQL-Features: issue_fields
{
    repository(owner: "OWNER", name: "REPO") {
        issue(number: 123) {
            issueFieldValues(first: 20) {
                nodes {
                    __typename
                    ... on IssueFieldDateValue {
                        value
                        field {
                            ... on IssueFieldDate {
                                id
                                name
                            }
                        }
                    }
                    ... on IssueFieldTextValue {
                        value
                        field {
                            ... on IssueFieldText {
                                id
                                name
                            }
                        }
                    }
                    ... on IssueFieldNumberValue {
                        value
                        field {
                            ... on IssueFieldNumber {
                                id
                                name
                            }
                        }
                    }
                    ... on IssueFieldSingleSelectValue {
                        name
                        color
                        field {
                            ... on IssueFieldSingleSelect {
                                id
                                name
                            }
                        }
                    }
                }
            }
        }
    }
}
```

## Setting field values

Use `setIssueFieldValue` to set one or more fields in one call. Batch the fields when possible. You need the issue's
node ID (`repository.issue.id`), plus the field IDs and option IDs from the discovery query above.

```graphql
# Header: GraphQL-Features: issue_fields
mutation {
    setIssueFieldValue(
        input: {
            issueId: "ISSUE_NODE_ID"
            issueFields: [
                { fieldId: "IFD_xxx", dateValue: "YYYY-MM-DD" }
                { fieldId: "IFT_xxx", textValue: "some text" }
                { fieldId: "IFN_xxx", numberValue: 3.0 }
                { fieldId: "IFSS_xxx", singleSelectOptionId: "OPTION_ID" }
            ]
        }
    ) {
        issue {
            id
            title
        }
    }
}
```

Each entry in `issueFields` takes a `fieldId` plus exactly one value parameter:

| Field type    | Value parameter        | Format                               |
| ------------- | ---------------------- | ------------------------------------ |
| Date          | `dateValue`            | ISO 8601 date string: `"YYYY-MM-DD"` |
| Text          | `textValue`            | String                               |
| Number        | `numberValue`          | Float                                |
| Single select | `singleSelectOptionId` | ID from the field's `options` list   |

To clear a field value, set `delete: true` instead of a value parameter.

## Example: set dates and priority on an issue

Replace each `YYYY-MM-DD` with a real date and each ID with one from the discovery query.

```bash
gh api graphql \
  -H "GraphQL-Features: issue_fields" \
  -f query='
mutation {
  setIssueFieldValue(input: {
    issueId: "I_kwDOxxx"
    issueFields: [
      { fieldId: "IFD_startDate", dateValue: "YYYY-MM-DD" }
      { fieldId: "IFD_targetDate", dateValue: "YYYY-MM-DD" }
      { fieldId: "IFSS_priority", singleSelectOptionId: "OPTION_P1" }
    ]
  }) {
    issue { id title }
  }
}'
```

## Searching by field values

### GraphQL bulk query (recommended)

Fetch issues via GraphQL. Filter them by `issueFieldValues`. The search qualifier syntax (`field.name:value`) is not yet
reliable across all environments.

```bash
# Find all open P1 issues in a repo. --paginate follows the cursor, so every page is read and
# the jq filter runs once per page.
gh api graphql --paginate -H "GraphQL-Features: issue_fields" -f query='
query($endCursor: String) {
  repository(owner: "OWNER", name: "REPO") {
    issues(first: 100, states: OPEN, after: $endCursor) {
      pageInfo { hasNextPage endCursor }
      nodes {
        number
        title
        updatedAt
        assignees(first: 3) { nodes { login } }
        issueFieldValues(first: 100) {
          nodes {
            __typename
            ... on IssueFieldSingleSelectValue {
              name
              field { ... on IssueFieldSingleSelect { name } }
            }
          }
        }
      }
    }
  }
}' --jq '
  [.data.repository.issues.nodes[] |
    select(.issueFieldValues.nodes[] |
      select(.field.name == "Priority" and .name == "P1")
    ) |
    {number, title, updatedAt, assignees: [.assignees.nodes[].login]}
  ]'
```

**Schema notes for `IssueFieldSingleSelectValue`:**

- The selected option's display text is in `.name`, not in `.value`.
- `.color`, `.description`, and `.id` are also available.
- The parent field reference is in `.field`. Use an inline fragment to get the field name.

### Search qualifier syntax (experimental)

Issue fields may also be searchable with dot notation. Dot notation requires `advanced_search=true` on REST, or the
`ISSUE_ADVANCED` search type on GraphQL. A search may return 0 results even when matching issues exist.

```text
field.priority:P0                  # Single-select equals value
field.target-date:>=YYYY-MM-DD     # Date comparison
has:field.priority                 # Has any value set
no:field.priority                  # Has no value set
```

Field names use the **slug** (lowercase, hyphens for spaces). For example, "Target Date" becomes `target-date`.

```bash
# REST API (may not return results in all environments)
gh api "search/issues?q=repo:owner/repo+field.priority:P0+is:open&advanced_search=true" \
  --jq '.items[] | "#\(.number): \(.title)"'
```

> **Warning:** The colon notation (`field:Priority:P1`) is silently ignored. If you use search qualifiers, always use
> dot notation (`field.priority:P1`). See [../search.md](../search.md) for the full search guide.
