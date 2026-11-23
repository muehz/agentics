# Linear tickets

## What this source contains

- Issues that describe features, bugs, and their motivation
- Project docs attached to issues (often PRDs or specs)
- Parent/sub-issue relationships (broader initiative → specific tickets)
- Comments on issues (clarifications, scope changes, "why we're doing this" rationale)
- Labels (e.g., `compliance`, `customer-request`, `perf`) that signal the type of motivation
- Status updates that explain scope changes
- Attachments and linked GitHub PRs

Linear often holds the product/business context, such as "we're doing this because customer X asked" or "this is for the
Q3 compliance initiative".

## How to search it

Use the Linear MCP. For another tracker, adapt these steps for the MCP of that tracker. When the project tracks issues
on GitHub and no tracker MCP is configured, use the `gh` CLI instead:

- `gh issue view <number> --comments`
- `gh issue list --search "<terms>"`
- `gh api` for fields the CLI does not expose

GitHub Issues has sub-issues, so step 3 applies. `gh issue view <number> --json parent` returns the parent issue. Use
`gh api repos/<owner>/<repo>/issues/<number>/sub_issues` to list an issue's sub-issues. Also follow issues that the text
links, and check the milestone.

1. **Start with linked tickets.** Fetch ticket IDs the seed commits or PRs reference (e.g., `ENG-1234`, `[BUG-567]`)
   first with `get_issue`. Read each full issue, including comments.
2. **List related issues by keyword.** Use `list_issues` with text search for the feature name, key symbol, or business
   term. Try multiple phrasings.
3. **Walk the issue tree.** If you land on a sub-issue, fetch its parent. Sub-issues are tactical. Parents often carry
   the "why."
4. **Read project docs.** If the issue belongs to a project, use `get_project` to check its attached docs. They most
   often hold specs and rationale.
5. **Check labels and milestones.** Labels hint at the category of motivation (customer-request, incident-followup,
   compliance). Milestones tie work to deadlines, which often reveal motivation.

## What good evidence looks like here

- An issue description that states the business problem: "Customer Acme needs X because of their SOC2 audit"
- A comment that records a decision: "We decided to go with approach B because approach A would require touching the
  billing service"
- A parent issue titled like an initiative: "Q3 Enterprise Readiness" or "Reduce Payment Failures"
- An attached PRD or spec
- Labels like `customer:acme`, `incident-followup`, `compliance`, `perf-regression`

## Common pitfalls

- **Scope drift.** The PR's ticket may have been closed and reopened with a different scope. Read the whole history.
- **Mechanical templates.** Some teams require "Why" sections but fill them with boilerplate. Generic text ("improve
  user experience") is probably not a real answer.
- **Stale tickets.** Old tickets often reflect a version of the plan that changed. Check dates. Cross-reference them
  with the code's ship date.
- **Closed-as-duplicate chains.** Follow the duplicate-of relationships back to the canonical ticket.
- **Private workspace content.** If you can't access an issue, note that as a gap rather than guess.

## What to return

For each relevant ticket:

- Ticket ID and title
- The problem/motivation quoted from the description or comments (not paraphrased; the synthesizer needs the exact text
  to cite)
- Labels, parent issue, project
- Author, created date, closed date
- Link to the ticket if available
