# Change-record adapters

Adapters define **where the change-record gets posted** after the markdown artifact is rendered. Each adapter takes the
record's fields and transforms them to the target system's shape. It shows the API call and, on the user's named request
or one confirmation, it posts.

The framework is **pluggable**. The GitHub adapter ships. ServiceNow, Jira, or another system can be added. The
change-mode workflow keeps its structure.

## Status

| Adapter                                    | Status    |
| ------------------------------------------ | --------- |
| [`github-issue.md`](github-issue.md)       | Working   |
| [`stub-servicenow.md`](stub-servicenow.md) | Stub only |

The stub is **not functional**. It is a copy-paste template that documents the contract.

## Adapter interface (what every adapter must answer)

Every adapter file specifies these seven items, in this order.

### 1. Identity

- **Name:** short identifier (`github`, `servicenow`, `jira`, etc.), used when the user picks this adapter in the change
  workflow's post question
- **Display name:** text used in chat ("Post to GitHub Issue", "Post to ServiceNow CR")
- **Target system:** full name and URL convention

### 2. Inputs

The change-record fields the adapter needs, named as in [../change-record-template.md](../change-record-template.md). A
minimal adapter needs: ID, classification, summary, scope, approval chain, rollback summary, schedule. Some need more
(ServiceNow needs CMDB CI references; Jira needs issue-type + project key).

### 3. Authentication

| Auth pattern        | Examples                                                |
| ------------------- | ------------------------------------------------------- |
| **CLI tool**        | `gh` CLI authenticated to GitHub                        |
| **MCP server**      | A host MCP server for ServiceNow or Jira, if available  |
| **API key / token** | Direct REST call with token from env var or config file |
| **OAuth flow**      | One-time browser-based auth; refresh-token cached       |

Document the **failure mode for missing auth**. Include the error message the adapter surfaces and the command the user
runs to fix it.

### 4. API call

The exact command or API request, with paired bash + PowerShell where applicable. It takes one of these forms:

- The literal command with placeholders (CLI tool)
- HTTP method + URL + headers + body shape (REST)
- MCP tool name + parameters (MCP)

### 5. Idempotency

How the adapter handles duplicate posts:

- **Re-run with same change-record ID:** does the adapter update the existing entry, error, or create a duplicate?
- **Re-run after partial failure:** does the adapter recognize a previously-failed post and resume?

The expectation is to **never create a duplicate without user confirmation**. If the adapter detects a possible
duplicate, do these steps:

1. Surface the existing entry.
2. Ask as a ❓ question whether to update, supersede, or skip it.
3. In the ➡️ line, recommend update when the existing entry is open and describes the same change, and supersede
   otherwise.

### 6. Confirmation gate

A post leaves the machine, so it needs the user's named request or one confirmation, as the authorization rule in
[approval.md](../../../../routing/references/contracts/approval.md) defines. The user has already confirmed the record's
text before the file was written.

1. Run the outbound checks of the [posting envelope](../../../../gh/references/posting-envelope.md#outbound-checks) on
   the body first.
2. Render one preview: the exact command or API call, the title, the labels (and any left out), the assignees, the
   idempotency check's result, and what the outbound checks redacted or withheld. Show the body again only where the
   checks changed it.
3. Decide whether to ask:

    | Situation (the first row that matches wins)                                   | Action                                   |
    | ----------------------------------------------------------------------------- | ---------------------------------------- |
    | The idempotency check found an existing entry                                 | Ask its question (Section 5) first.      |
    | The outbound checks redacted or withheld text                                 | Ask for one confirmation of the preview. |
    | The user named the post up front, or picked this adapter in the post question | Post, with no further question.          |
    | Anything else                                                                 | Ask for one confirmation of the preview. |

4. Act on a reply to a confirmation:
    - Any clear affirmative, in any language: post.
    - A decline: post nothing, keep the local file, ask nothing again about the same draft, and do not switch to another
      adapter.
    - An affirmative that carries a change, or any other reply that asks for a change: revise the draft, show it, and
      ask again.

An adapter that posts with neither a named request nor this confirmation does not ship.

### 7. Failure mode

The default contract when the external system is unreachable or rejects the post:

1. Capture the error message and HTTP status (or equivalent)
2. Render in chat with a clear "post failed: <reason>"
3. Surface the retry command and any pre-fix the user needs to do (re-auth, network check, etc.)
4. Do **not** retry automatically. Do **not** silently fall back to a different adapter.
5. The local change-record file already exists at this point: the change workflow writes the file before it attempts the
   post. A failed external post loses no data.

## How to add a new adapter

1. **Copy `stub-servicenow.md` to `<your-system>.md`** in this directory.
2. **Fill in all seven sections** of the interface contract.
3. **Implement** the adapter logic (typically CLI command or HTTP request construction, with cross-platform code
   blocks).
4. **Update this README** to add the new adapter to the status table.
5. **Update the post step of [../change-workflow.md](../change-workflow.md#step-3-post-through-an-adapter)** to include
   the new adapter in its question.
6. **Test against a sandbox account.** Verify that the draft-and-confirm flow renders correctly and the post lands.
7. **Document failure modes** specific to the target system (e.g., ServiceNow CMDB lookup errors, Jira
   project-permission errors).

## Selecting an adapter

The user picks the adapter in the post step of
[../change-workflow.md](../change-workflow.md#step-3-post-through-an-adapter), which holds the question and its answers.
For an unknown adapter name, list the adapters in this directory, leaving out files whose names start with `stub-`,
because those are templates.

## What this framework does NOT do

- **No auto-discovery of which adapter to use.** The user picks. Heuristics like "if a Jira project ID is in env, use
  Jira" are out of scope because they are easy to misroute.
- **No multi-target posts.** One adapter per invocation. To post to two systems, run the change-mode flow twice.
- **No state synchronization.** External state (approval status, comments) does not auto-sync back to the local markdown
  file. The user updates the file manually if needed.
- **No retry logic.** The adapter surfaces the failure. The user retries.
- **No credential management.** Adapters use the user's existing auth. The framework does not store, rotate, or validate
  credentials.
