# Issue types (advanced GraphQL)

An organization defines issue types (Bug, Feature, Task, Epic, etc.), and its repositories inherit them.

For basic use, the MCP tools support issue types directly. Call `list_issue_types` to discover types. Pass `type: "Bug"`
to `issue_write`, with `method: "create"` or `method: "update"`. This reference covers advanced GraphQL operations.

## GraphQL feature header

GitHub required the `GraphQL-Features: issue_types` HTTP header while this API was in preview. The header still does no
harm if you send it. Recent testing shows the same result without it for reads, such as a query of `issueType` on an
issue.

## List types (org or repo level)

```graphql
# Header: GraphQL-Features: issue_types
{
    organization(login: "OWNER") {
        issueTypes(first: 20) {
            nodes {
                id
                name
                color
                description
                isEnabled
            }
        }
    }
}
```

You can also list types per repo with `repository.issueTypes`, or look up one type by name with
`repository.issueType(name: "Bug")`.

## Read an issue's type

```graphql
# Header: GraphQL-Features: issue_types
{
    repository(owner: "OWNER", name: "REPO") {
        issue(number: 123) {
            issueType {
                id
                name
                color
            }
        }
    }
}
```

## Set type on an existing issue

```graphql
# Header: GraphQL-Features: issue_types
mutation {
    updateIssueIssueType(input: { issueId: "ISSUE_NODE_ID", issueTypeId: "IT_xxx" }) {
        issue {
            id
            issueType {
                name
            }
        }
    }
}
```

## Create issue with type

```graphql
# Header: GraphQL-Features: issue_types
mutation {
    createIssue(input: { repositoryId: "REPO_NODE_ID", title: "Fix login bug", issueTypeId: "IT_xxx" }) {
        issue {
            id
            number
            issueType {
                name
            }
        }
    }
}
```

To clear the type, set `issueTypeId` to `null`.

## Available colors

`GRAY`, `BLUE`, `GREEN`, `YELLOW`, `ORANGE`, `RED`, `PINK`, `PURPLE`
