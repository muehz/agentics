# MCP Tool Design Patterns

This reference covers two dominant patterns, plus naming, parameter, and pagination conventions.

## Pattern 1: One Tool Per Action

When the server exposes a small, fixed set of operations (under ~15), use this pattern.

**Naming:** `verb_noun` — `create_issue`, `list_pull_requests`, `update_branch_policy`.

**Strengths:**

- The LLM understands intent immediately from the tool name
- Each tool can have a focused parameter schema
- Tool-level documentation is easy to add

**Weaknesses:**

- The tool list grows linearly with the operation count
- Beyond ~15 tools, the LLM struggles to keep them all in working memory

**Example:**

```
list_repos(owner: string)
get_repo(owner: string, repo: string)
create_issue(owner: string, repo: string, title: string, body: string)
list_issues(owner: string, repo: string, state?: "open" | "closed")
```

## Pattern 2: Search + Execute

When the server exposes many operations or unbounded resource sets, use this pattern.

**Naming:** Two tools: `search_<entity>` and `execute_<entity_action>`. Or three: `search`, `get`, `act`.

**Strengths:**

- The tool count stays small (2-3) regardless of the operation count
- The LLM can discover capabilities at runtime through search
- Pagination works naturally

**Weaknesses:**

- It adds an extra round-trip (search → execute)
- Search must return enough metadata for the LLM to choose intelligently
- It requires good ranking in search results

**Example:**

```
search_resources(query: string, type?: string, limit?: number)
get_resource(id: string)
execute_action(resource_id: string, action: string, params: object)
```

## Parameter Design

### Required vs optional

- Mark parameters required only if the operation cannot proceed without them
- Provide sensible defaults for everything else
- Document the default in the parameter description, not just in code

### Typed schemas

- Use JSON Schema for parameter typing
- Use enums for known-finite value sets. They let the LLM see the options.
- Use patterns (regex) for string formats. They catch errors early.
- Use min/max for numeric ranges

### Object parameters

When there are 4+ related fields, group the related parameters into objects. Flat parameter lists with 8+ fields confuse
the LLM.

```
// Avoid
create_issue(owner, repo, title, body, labels, assignees, milestone, project)

// Prefer
create_issue(owner, repo, title, body, options: { labels?, assignees?, milestone?, project? })
```

## Error Responses

Return errors as MCP tool errors, not as success responses with error fields. Let the LLM see the failure structurally.

**Include in errors:**

- A short, human-readable message
- The category (auth, not_found, validation, rate_limit, server_error)
- An actionable next step, if it is obvious ("retry after N seconds", "check the resource ID")

**Don't include:**

- Stack traces
- Internal IDs the LLM cannot use
- Sensitive data (credentials, PII)

## Pagination

For tools that return lists, paginate consistently:

```
{
  items: [...],
  next_cursor?: string,  // present iff more pages exist
  total?: number          // optional total count
}
```

The LLM can pass `next_cursor` back to fetch the next page. Avoid offset/limit pagination, because cursor-based
pagination survives item insertions.

## Tool Description Conventions

- Lead with the action verb in the description
- State preconditions and side effects explicitly
- If relevant, note rate limits, idempotency, and cost
- Reference parameter constraints once, in the description, and not per param

## Anti-Patterns

- Generic `execute_query` or `run_command` tools: they are too unconstrained, and the LLM will misuse them
- Tools that change behavior based on parameter values (e.g., a `manage_user` that does 5 different things). Split them
  into separate tools.
- Tools that return free-form text the LLM has to parse. Return structured data.
- Optional parameters with no documented default: they lead to inconsistent behavior
