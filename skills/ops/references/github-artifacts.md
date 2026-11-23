# GitHub artifacts

Templates for the four GitHub artifacts the skill produces. **The skill drafts every artifact and presents it to the
user. It posts the artifact only when the user has authorized it**: with a clear affirmative to the draft, or by naming
the post up front ([Writes outside the machine](../SKILL.md#writes-outside-the-machine)). The skill never invokes
`gh issue create`, `gh issue comment`, or any other write for a post that is not authorized.

## The draft-don't-post rule (load-bearing)

1. Render the artifact in a fenced markdown block in chat.
2. Tell the user which command the skill will run once the post is authorized (e.g.,
   `gh issue create --title "$issue_title" --label incident,P1 --body-file "$draft_file"`). Take `issue_title` from the
   rendered draft and `draft_file` from the file producer that saved its body. Pass both as data. First verify that the
   labels exist (`gh label list --limit 200`). Omit missing labels so the create does not fail mid-incident.
3. Wait for the user's approval. Any clear affirmative approves, in any wording, as
   [approval.md](../../routing/references/contracts/approval.md#confirmation-words) says. A post the user named up front
   needs no second question, as that contract's
   [steps named up front](../../routing/references/contracts/approval.md#steps-named-up-front) says.
4. Once the post is authorized, delegate the write to the `gh` skill (preferred) or invoke `gh` directly.
5. After the post, paste the issue or comment URL back into chat. Then update the in-memory tracking state.

If the user edits the draft, do these steps:

1. Re-render the edited version.
2. Ask again.

Never post a stale draft.

## Shared posting envelope

Every issue body and comment this skill posts follows the shared marker and body rules in
[skills/gh/references/posting-envelope.md](../../gh/references/posting-envelope.md). Every comment this skill posts,
each status comment included, ends with the envelope's AI footer. An issue body (the tracking issue, the postmortem
issue and the drill debrief issue) carries none.

## Tracking issue (response mode, declaration)

Create it at the end of the declare phase. It is the source of truth for the timeline, comms log, and severity changes
throughout the incident.

**Title:** `[INCIDENT][<severity>] <short descriptor>`

- Example: `[INCIDENT][P1] Checkout 5xx errors — investigating`

**Labels (if available in repo):** `incident`, `<severity>` (e.g., `P1`), `reliability` or `security`, `status:active`

**Body:**

```markdown
## Summary

<One-line summary of impact, updated as the picture clarifies.>

## Status

<investigating | identified | mitigating | monitoring | resolved>

## Severity

<P1 | P2 | P3 | P4>

## Branch

<reliability | security | both>

## Roles

- **IC:** <name>
- **Comms lead:** <name>
- **Scribe:** <name>
- **Subject expert(s):** <names>

<!-- Security incidents only: -->

- **Security lead:** <name>
- **Forensics scribe:** <name>

## Impact

- **User-visible:** <one line, quantified if possible>
- **Internal:** <one line>

## Timeline (UTC)

| Time | Event             | Notes                |
| ---- | ----------------- | -------------------- |
| <ts> | Alert fired       | <link>               |
| <ts> | Incident declared | <severity, IC named> |
| <ts> | <event>           | <notes>              |

## Comms log

| Time | Channel     | Audience  | Status        |
| ---- | ----------- | --------- | ------------- |
| <ts> | Status page | Customers | Investigating |
| <ts> | #eng        | Internal  | First update  |

## Action items (running list)

- [ ] <action> — owner: <name> — due: <date>

## Postmortem

<Link to postmortem issue once filed.>
```

**Security-incident variant:** draft this issue only after
[Where the record lives](response/security-branch.md#where-the-record-lives) says where the record goes. GitHub has no
private issue. On a public, internal or unreadable repository the record goes to a draft security advisory or a private
location the user names, never to an issue. On a private repository it is an issue only on the terms that section sets.
When it is an issue, replace the comms log with a "restricted comms log" that does not link to channels by name. Never
put attacker IOCs in an issue on any repository.

## Status comment (response mode, during)

At each major event (severity change, comms send, mitigation attempt, hand-off), post the status comment on the tracking
issue. The scribe drafts it. The IC confirms it.

```markdown
**<UTC timestamp> — <Status | Mitigation | Comms | Severity change>**

<One paragraph: what happened, what was done, what's next.>

<Optional: links to comms posts, deploy diffs, dashboards used.>
```

## Postmortem issue (postmortem mode)

File it after the response-mode tracking record closes. That record is the tracking issue, or for a security incident
the draft advisory or private location that [Where the record lives](response/security-branch.md#where-the-record-lives)
names in its place. Link the postmortem back to the tracking record when that record can carry a link. A security
incident's postmortem goes to the same place, not automatically to an issue.

**Title:** `[POSTMORTEM] <descriptor> (<incident ID>)`

**Labels:** `postmortem`, `<severity>`, `reliability` or `security`

**Body:** fill it from [postmortem/postmortem-template.md](postmortem/postmortem-template.md). The template covers the
full structure (summary, impact, timeline, root cause, contributing factors, action items, what went well, what didn't).

**Cross-link:** when the tracking record is an issue, comment a link to the postmortem on it, and comment a link back to
it on the postmortem. When the tracking record is a draft advisory or a private location, link there only if it can
carry a link.

## Drill debrief issue (drill mode, optional)

The default output of drill mode is a markdown report in chat. If the user wants a tracked artifact, draft an issue:

**Title:** `[DRILL] <scenario name> — <date>`

**Labels:** `drill`, `<reliability | security>`

**Body:**

```markdown
## Scenario

<scenario name + one-line description>

## Participants

- **IC:** <name>
- **Comms lead:** <name>
- **Subject experts:** <names>

## Inject timeline

| Time | Inject     | Response   | Time-to-respond |
| ---- | ---------- | ---------- | --------------- |
| T+0  | <inject 1> | <response> | <duration>      |

## What worked

- <bullet>

## What didn't

- <bullet>

## Action items

- [ ] <action> — owner: <name> — due: <date>

## Lessons learned

<One paragraph.>
```
