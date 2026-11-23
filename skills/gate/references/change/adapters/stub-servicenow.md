# ServiceNow adapter (stub, not functional)

> ⚠️ **This is a stub.** It documents the adapter pattern for future implementation. It does **not** ship as a working
> adapter.
>
> Files prefixed with `stub-` (like this one) are templates, not functional adapters. The change-workflow does not
> surface `stub-`-prefixed files in the adapter selection.
>
> To turn this into a real adapter, do these steps:
>
> 1. Copy it to `servicenow.md` (drop the `stub-` prefix).
> 2. Implement each section.
> 3. Test against a sandbox instance.
> 4. Update [README.md](README.md) to list it.

## 1. Identity

- **Name:** `servicenow`
- **Display name:** "Post to ServiceNow Change Request"
- **Target system:** ServiceNow ITSM, at `https://<instance>.service-now.com/change_request.do`

## 2. Inputs

From the change-record artifact (see [../change-record-template.md](../change-record-template.md)):

- **Required:**
    - `ID` → mapped to ServiceNow `number` or `correlation_id`
    - `Classification` → mapped to ServiceNow `type` field (Standard / Normal / Emergency align directly with
      ServiceNow's taxonomy)
    - `Summary` → ServiceNow `short_description`
    - `Scope` → ServiceNow `description` + `cmdb_ci` (configuration items affected; requires CMDB lookup)
    - `Risk` → ServiceNow `risk` + `impact` fields
    - `Approval chain` → ServiceNow approval rules (typically configured at instance level; the adapter surfaces them
      but does not construct routing)
    - `Rollback` → ServiceNow `backout_plan`
    - `Schedule` → ServiceNow `start_date` + `end_date`
- **ServiceNow-specific (beyond the change-record):**
    - `instance` (the ServiceNow instance URL, typically configured per-org)
    - `assignment_group` (which group owns the CR, required by some workflows)
    - `cmdb_ci` references (the affected configuration items; requires a CMDB lookup step)

## 3. Authentication

A real implementation picks one auth pattern:

- **Basic auth with username + token** (deprecated in many orgs; check before you rely on it)
- **OAuth 2.0** (preferred for interactive use; refresh-token flow)
- **MCP server** if an Agentics-published ServiceNow MCP exists (preferred, because it needs no custom auth handling)
- **Mid-Server** for on-prem ServiceNow with restricted external access (out of scope for this adapter)

A real implementation should document:

- Required env vars or config file location (e.g., `SERVICENOW_INSTANCE`, `SERVICENOW_TOKEN`)
- Failure mode for missing auth (clear error message + remediation command)
- How to test auth before you attempt a post (`servicenow whoami` equivalent, if MCP-based)

## 4. API call

A minimal ServiceNow REST post:

```text
POST https://<instance>.service-now.com/api/now/table/change_request
Headers:
  Content-Type: application/json
  Accept: application/json
  Authorization: <auth header — Basic | Bearer>
Body:
{
  "type": "<standard|normal|emergency>",
  "short_description": "<from change-record summary>",
  "description": "<rendered markdown or sanitized HTML — depends on instance>",
  "risk": "<low|medium|high>",
  "impact": "<low|medium|high>",
  "assignment_group": "<group>",
  "cmdb_ci": "<CI sys_id from lookup>",
  "start_date": "<YYYY-MM-DD HH:MM:SS>",
  "end_date": "<YYYY-MM-DD HH:MM:SS>",
  "backout_plan": "<from change-record rollback section>",
  "correlation_id": "<change-record ID>"
}
```

Paired commands would surface both `curl` (or `gh api` if a ServiceNow MCP wraps it) for bash/PowerShell.

A real implementation should:

- Construct the JSON body from the change-record fields with correct field mappings
- Sanitize or convert markdown, because most instances do not render full markdown
- Add the CMDB-CI lookup step (a separate `GET /api/now/table/cmdb_ci?...` query) before the post if org policy requires
  CI references

## 5. Idempotency

Set `correlation_id` to the change-record ID. Before the post, check with this request:

```text
GET https://<instance>.service-now.com/api/now/table/change_request?sysparm_query=correlation_id=<id>
```

If a result exists, ask one ❓ question titled "Existing change request" whose body lists the three options:

- update the existing CR (PUT to its `sys_id`)
- supersede (create new + close old with a link)
- skip (local file is updated; ServiceNow CR untouched)

The ➡️ recommends update when the existing CR is open and describes the same change, supersede otherwise. Wait for the
user's choice, as in the GitHub adapter.

## 6. Confirmation gate

As the adapters README's [confirmation gate](README.md#6-confirmation-gate) decides, after the outbound checks run:

1. Render the JSON body in a fenced code block in chat
2. Render the exact HTTP request that will be made (method + URL + body)
3. Render the idempotency check result
4. On a confirmation, act on the reply:
    - Any clear affirmative, in any language: post.
    - A decline: post nothing, keep the local file, and do not switch to another adapter.
    - A reply that asks for a change: render the updated draft and ask again.

A real implementation also surfaces the CMDB-CI lookup result for confirmation before the post, because a wrong CI is a
common ServiceNow data-quality issue.

## 7. Failure mode

ServiceNow-specific failure modes a real implementation should handle:

- **Auth expired** (401) → surface the re-auth command. For OAuth, trigger the refresh-token flow.
- **Validation error** (400 with field-specific message) → surface the field and the expected value.
    - Common causes are an invalid `type`, a missing required `assignment_group`, and malformed dates.
- **Permission error** (403) → tell the user to check their role on the instance, because many ServiceNow ops require
  specific ITIL roles.
- **CMDB-CI not found** (validation error that references `cmdb_ci`) → surface the CI lookup that failed.
    - Ask the user to provide a valid CI name or sys_id.
- **Instance unreachable** (DNS, timeout) → use standard network-error handling. Surface the retry command.

In all cases:

1. The local change-record markdown file already exists, so no data is lost
2. Clear "post failed: <reason>" message
3. Exact retry command
4. Do not auto-retry. Do not auto-fall-back.

## Post-CR follow-ups

After a successful post:

- Surface the CR number and URL
- Optionally update the local change-record markdown with the ServiceNow CR number for cross-reference
- Optionally update the related GitHub PR / runbook with the CR link (use `gh` for the GitHub side)

## ServiceNow-specific gotchas (notes for future implementer)

- ServiceNow rate limits API calls per-instance per-minute. High-volume change-record submission may hit limits.
- Different ServiceNow instances have different required fields, which the org admin configures.
- The adapter should support a configurable field map (e.g., `gate-servicenow-config.yml` in the consuming repo) rather
  than hardcode the fields above.
- ServiceNow's markdown rendering varies. Some instances support rich markdown, and others sanitize to plain text. The
  body field may need transformation.
- Approval routing is typically configured at the instance level (workflow engine + approval rules).
- The adapter cannot reliably trigger named-individual approvals. Surface that limit to the user as a constraint.
- For Emergency changes, ServiceNow often has a separate fast-track workflow.
- The adapter should detect the `Emergency` classification. If the instance has an emergency endpoint configured, the
  adapter should route to it.

---

## To implement this adapter

1. **Determine your auth pattern** based on org policy
2. **Get a ServiceNow sandbox instance** to test with (avoid live CMDB during development)
3. **Map the change-record fields** to your instance's CR fields (these vary)
4. **Build the CMDB-CI lookup helper** (this is often the hardest part)
5. **Implement the adapter** as all seven sections above describe
6. **Test the full draft-and-confirm flow** end-to-end against the sandbox
7. **Rename this file** from `stub-servicenow.md` to `servicenow.md`
8. **Update [README.md](README.md)** to add `servicenow` to the status table
9. **Update the post step of [../change-workflow.md](../change-workflow.md#step-3-post-through-an-adapter)** to name
   `servicenow` in its question
10. **Document the org-specific config** the adapter expects (env vars, config file format)

To contribute an implementation upstream, follow the Agentics plugin's contribution conventions.
