# Projects V2

GitHub Projects V2 uses a GraphQL API. Three MCP tools wrap it, so you typically do not need raw GraphQL.

## Using MCP tools (preferred)

The tools are named as the server names them. Your host may add a prefix that holds the server's name. Every call takes
`method`. Every call also takes `owner`, except two `projects_get` lookups that take a node ID in place of `owner` and
`project_number`: `get_project_status_update` takes `status_update_id`, and `get_project_view` takes `view_id`.
`owner_type` (`user` or `org`) is optional because the server detects it, except for `create_project`. The parameters
below are those of server v1.10.1.

- To list projects, call `projects_list` with `method: "list_projects"` and `owner`. Its `query` filters by title text
  and state (for example `roadmap is:open`).
- To get one project's details, call `projects_get` with `method: "get_project"`, `owner`, and `project_number`.
- To list project fields, call `projects_list` with `method: "list_project_fields"`, `owner`, and `project_number`.
- To list project items, call `projects_list` with `method: "list_project_items"`, `owner`, `project_number`, and
  `field_names` (for example `["Status"]`) or `fields` (field IDs). Without one of them each item carries only its
  title. Page with `after`, set to the previous page's `pageInfo.nextCursor`.
- To add an issue or PR to a project, call `projects_write` with `method: "add_project_item"`, `owner`,
  `project_number`, `item_type` (`issue` or `pull_request`), `item_owner`, `item_repo`, and `issue_number` (or
  `pull_request_number` for a PR).
- To update a project item field value, call `projects_write` with `method: "update_project_item"`, `owner`,
  `project_number`, `item_id`, and `updated_field`. `updated_field` is `{"name": "Status", "value": "In Progress"}` or
  `{"id": <field ID>, "value": ...}`. The name form accepts an option name for a single-select field. The ID form
  expects an option ID. Instead of `item_id`, pass `item_owner`, `item_repo`, and `issue_number` to find the item from
  its issue.
- To delete a project item, call `projects_write` with `method: "delete_project_item"`, `owner`, `project_number`, and
  `item_id`.

The MCP `item_id` is the numeric project item ID. It does not accept the GraphQL node ID (`PVTI_…`) that the GraphQL
sections below use.

## Workflow for project operations

1. **Find the project.** See [Finding a project by name](#finding-a-project-by-name) below.
2. **Discover fields.** Use `projects_list` with `list_project_fields` to get field IDs and option IDs.
3. **Find items.** Use `projects_list` with `list_project_items` to get item IDs.
4. **Mutate.** Use `projects_write` to add, update, or delete items.

## Finding a project by name

> **Known issue:** `projectsV2(query: "…")` does keyword search, not exact name match, and returns results sorted by
> recency. Common words like "issue" or "bug" return hundreds of false positives. The target project may sit dozens of
> pages deep.

Use this priority order:

### 1. Direct lookup (if you know the number)

```bash
gh api graphql -f query='{
  organization(login: "ORG") {
    projectV2(number: 42) { id title }
  }
}' --jq '.data.organization.projectV2'
```

### 2. Reverse lookup from a known issue (most reliable)

If the user mentions an issue, epic, or milestone in the project, query that issue's `projectItems` to find the project:

```bash
gh api graphql -f query='{
  repository(owner: "OWNER", name: "REPO") {
    issue(number: 123) {
      projectItems(first: 10) {
        nodes {
          id
          project { number title id }
        }
      }
    }
  }
}' --jq '.data.repository.issue.projectItems.nodes[] | {number: .project.number, title: .project.title, id: .project.id}'
```

### 3. GraphQL name search with client-side filtering (fallback)

Query a large page and filter client-side for an exact title match:

```bash
gh api graphql -f query='{
  organization(login: "ORG") {
    projectsV2(first: 100, query: "search term") {
      nodes { number title id }
    }
  }
}' --jq '.data.organization.projectsV2.nodes[] | select(.title | test("(?i)^exact name$"))'
```

If this returns nothing, paginate with `after` cursor (older projects sit on later pages) or broaden the regex.

### 4. MCP tool (small orgs only)

Call `projects_list` with `method: "list_projects"` and `owner`. Its `query` matches title text as a keyword, not as an
exact name, so compare the returned titles yourself. Without `query` you must scan all results. It works well only for
orgs with <50 projects.

## Project discovery for progress reports

For a project progress update (e.g., "Give me a progress update for Project X"):

1. **Find the project.** Use the [finding a project](#finding-a-project-by-name) strategies above. Ask the user for a
   known issue number if name search fails.

2. **Discover fields.** Call `projects_list` with `list_project_fields` to find the Status field and any Iteration
   field. The Status field options give the workflow stages. Use any Iteration field to scope the report to the current
   sprint.

3. **Get all items.** Call `projects_list` with `list_project_items` and `field_names` set to the fields you need (for
   example `["Status", "Iteration"]`), so each item includes those field values. For large projects (100+ items),
   paginate through all pages.

4. **Build the report.** Group items by Status field value. Count the items in each group. For iteration-based projects,
   filter to the current iteration first. Present a breakdown like:

    ```text
    Project: Issue Fields (Iteration 42, <start date> to <end date>)
    15 actionable items:
      🎉 Done:        4 (27%)
      In Review:      3
      In Progress:    3
      Ready:          2
      Blocked:        2
    ```

5. **Add context.** If items have sub-issues, include `subIssuesSummary` counts. If items have dependencies, note
   blocked items and what blocks them.

## OAuth scope requirements

| Operation                                    | Required scope |
| -------------------------------------------- | -------------- |
| Read projects, fields, items                 | `read:project` |
| Add/update/delete items, change field values | `project`      |

**Common pitfall:** The default `gh auth` token often only has `read:project`. Mutations will fail with
`INSUFFICIENT_SCOPES`. To add the write scope:

```bash
gh auth refresh -h {host} -s project
```

`{host}` is the resolved forge host. This opens a browser-based OAuth flow. Mutations fail until it completes.

## Finding an issue's project item ID

To find a known issue's project item ID (e.g., to update its Status), query from the issue side:

```bash
gh api graphql -f query='
{
  repository(owner: "OWNER", name: "REPO") {
    issue(number: 123) {
      projectItems(first: 5) {
        nodes {
          id
          project { title number }
          fieldValues(first: 10) {
            nodes {
              ... on ProjectV2ItemFieldSingleSelectValue {
                name
                field { ... on ProjectV2SingleSelectField { name } }
              }
            }
          }
        }
      }
    }
  }
}' --jq '.data.repository.issue.projectItems.nodes'
```

## Using GraphQL via gh api (fallback)

The MCP tools are the preferred write path. Use `gh api graphql` when the MCP tools are absent or a call fails.

**Find a project and its Status field options:**

```bash
gh api graphql -f query='
{
  organization(login: "ORG") {
    projectV2(number: 5) {
      id
      title
      field(name: "Status") {
        ... on ProjectV2SingleSelectField {
          id
          options { id name }
        }
      }
    }
  }
}' --jq '.data.organization.projectV2'
```

**List all fields (including iterations):**

```bash
gh api graphql -f query='
{
  node(id: "PROJECT_ID") {
    ... on ProjectV2 {
      fields(first: 20) {
        nodes {
          ... on ProjectV2Field { id name }
          ... on ProjectV2SingleSelectField { id name options { id name } }
          ... on ProjectV2IterationField { id name configuration { iterations { id startDate } } }
        }
      }
    }
  }
}' --jq '.data.node.fields.nodes'
```

**Update a field value (e.g., set Status to "In Progress"):**

```bash
gh api graphql -f query='
mutation {
  updateProjectV2ItemFieldValue(input: {
    projectId: "PROJECT_ID"
    itemId: "ITEM_ID"
    fieldId: "FIELD_ID"
    value: { singleSelectOptionId: "OPTION_ID" }
  }) {
    projectV2Item { id }
  }
}'
```

The value accepts one of: `text`, `number`, `date`, `singleSelectOptionId`, `iterationId`.

**Add an item:**

```bash
gh api graphql -f query='
mutation {
  addProjectV2ItemById(input: {
    projectId: "PROJECT_ID"
    contentId: "ISSUE_OR_PR_NODE_ID"
  }) {
    item { id }
  }
}'
```

**Delete an item:**

```bash
gh api graphql -f query='
mutation {
  deleteProjectV2Item(input: {
    projectId: "PROJECT_ID"
    itemId: "ITEM_ID"
  }) {
    deletedItemId
  }
}'
```

## End-to-end example: set issue status to "In Progress"

```bash
# 1. Get the issue's project item ID, project ID, and current status
gh api graphql -f query='{
  repository(owner: "OWNER", name: "REPO") {
    issue(number: 123) {
      projectItems(first: 1) {
        nodes { id project { id title } }
      }
    }
  }
}' --jq '.data.repository.issue.projectItems.nodes[0]'

# 2. Get the Status field ID and "In Progress" option ID
gh api graphql -f query='{
  node(id: "PROJECT_ID") {
    ... on ProjectV2 {
      field(name: "Status") {
        ... on ProjectV2SingleSelectField { id options { id name } }
      }
    }
  }
}' --jq '.data.node.field'

# 3. Update the status
gh api graphql -f query='mutation {
  updateProjectV2ItemFieldValue(input: {
    projectId: "PROJECT_ID"
    itemId: "ITEM_ID"
    fieldId: "FIELD_ID"
    value: { singleSelectOptionId: "IN_PROGRESS_OPTION_ID" }
  }) { projectV2Item { id } }
}'
```
