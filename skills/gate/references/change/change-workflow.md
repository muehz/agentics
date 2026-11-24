# Change workflow

Change mode writes a change record in the [change-record-template.md](change-record-template.md) shape. The record is
the audit trail. The one-line announcement is the one [SKILL.md](../../SKILL.md#which-mode) defines.

The eight phases below are the parts of the record, not eight stops. Gate drafts every part from what the request, the
repository and the related artifact already say, asks one round of questions for what it cannot fill or infer, renders
the record once, and takes one confirmation before it writes the file.

## Inputs

- **The change name:** a short kebab-case slug for the file name and the record ID, such as `rotate-prod-aws-keys`.
- **Optional:** a related artifact (a release tag, a pull request, an issue, a runbook), a classification the user
  chose, and the folder the record goes to.

With no name given, derive one from the related artifact's branch or title, or from the change summary, and state it in
the rendered record; the user changes it by saying so. Ask for a name only when there is nothing to derive it from.

## The flow

| Step | What happens                                                                                                                                                                             |
| ---- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1    | Draft Phases 1 to 7 from what is known. Run the security scan of Phase 3 when it applies.                                                                                                |
| 2    | Ask one question round for every field that cannot be filled or inferred, and for each decision.                                                                                         |
| 3    | Render the full record once, with all eight sections written out in full (never eliding sections with `...` or placeholders), the classification's reasoning, and every open gap marked. |
| 4    | Take one confirmation that names the file path; an edit request revises the record and renders it again.                                                                                 |
| 5    | Write the file (Phase 8), then the adapter post and the back-links when the user chose them.                                                                                             |

The round uses the [question format](../../../routing/references/contracts/question-format.md) and its collection rules.
A field the user already gave in the request is not asked again. A statement the user makes about their own organization
(who owns a system, which policy applies, who approves) is taken as true and recorded as stated by the user.

The run stops in the middle only for the irreversibility acknowledgment of Phase 5 and for a schedule with no acceptable
window in Phase 7.

## Phase 1: Identify the change

```text
What changes: <one paragraph>
Systems and services: <list>
Environments: <prod | staging | both | named clusters>
Related artifact: <tag, pull request, runbook, issue | none>
Requester: <handle>
```

A vague identification misroutes approvals and misses an audience, so a field gate cannot fill from the request or the
related artifact goes into the question round.

## Phase 2: Classify

The tier sets the approval chain, the comms scope and the freeze policy.
[classification-guide.md](classification-guide.md) holds the three tiers and the ordered decision tree. Infer the tier
from the decision tree, and put it and its one-line reason in the record:

```text
Classification: <Standard | Normal | Emergency>, because <the decision-tree step that decided it>
```

A classification the user names wins, and the record says it was set by the user. A tier that new information changes
during the run is re-classified and announced, as the guide says.

## Phase 3: Risk and impact

Before this phase, the **Check principles before verification** rule in
[SKILL.md](../../SKILL.md#cross-cutting-rules-all-four-modes) has run.

1. **Blast radius:** the users affected directly (a count, or "all customers"), the systems affected indirectly
   (downstream services, batch jobs, integrations), and the scope (one region, all regions, named tenants).
2. **Failure modes:** the worst plausible outcome, the most likely failure, and the second most likely (the one people
   forget).
3. **Reversibility:** fully reversible (a tested rollback exists), hard to undo (compensation is possible but not
   trivial), or irreversible (data loss, a one-way migration). An irreversible change is acknowledged in Phase 5.
4. **Security-sensitive surface:** auth, cryptography, input handling, infrastructure code, dependencies, secrets, IAM,
   or a public endpoint. When it is, and the related artifact has a diff, a pull request or a SHA, run
   `/agentics:review axes <base> --security --fast` against it and put the findings and the scan's limits line in this
   section. A section with neither looks the same as one that never ran, so the limits line is always carried. When the
   surface is sensitive and there is nothing to scan, the record says `not scanned: no diff`.

    **An incomplete scan is open, not clean.** The fast scan marks its tally
    `incomplete: <U> unsettled, settle with full  depth` when it could not settle a candidate, so zeros from it say
    nothing about the unsettled path. The section records `incomplete: <U> unsettled, full depth not run` beside the
    limits line, and the question round asks whether to run full depth, `/agentics:review axes <base> --security`. On
    the user's yes, run it and record its result in place of the fast one. On a no, or when the round gets no answer,
    write `skip security-scan`: the record is not `ok` while the scan is incomplete, and the verdict reads as the table
    below says for a skipped field.

5. **Compliance:** whether the change touches SOC 2, ISO 27001, GDPR, HIPAA, PCI or sector scope, and whether the
   compliance team must know.

## Phase 4: Approval chain

The record names who signs off; gate routes and approves nothing.

| Tier      | Approvers named in the record                                                                                                                                                                                                                    |
| --------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Standard  | The standing policy that pre-approves the change, with a link to it. No individual approvers.                                                                                                                                                    |
| Normal    | The owner of each affected system, from CODEOWNERS or the user's word; the security lead for a sensitive surface; the database or infrastructure lead for a data-tier or infrastructure change; compliance or legal for a compliance touchpoint. |
| Emergency | Post-hoc: the incident commander when the change runs during an incident; the system owner, told within 24 hours; the security lead for a sensitive surface, told at once.                                                                       |

An approver for an emergency change is someone other than the requester, with change authority for the affected system.
When the only person available is the requester, the record says so and names who approves after the fact.

Every approver starts as `Pending`. A record that says `Approved` before the approvers answered misstates the audit
trail, so gate never sets an approver's status itself.

## Phase 5: Rollback plan

Apply [../release/rollback-readiness.md](../release/rollback-readiness.md): the target state, whether it is still
reachable, the staged command, the time to recover, and irreversibility. Its questions join the round. The plan goes in
the record, so an incident or audit reader can see exactly what was planned.

**Irreversibility acknowledgment.** When any part of the change is irreversible, the record needs the user's
acknowledgment, and the run stops for it:

```text
Irreversible: <what, for example "the migration drops the legacy_payments table">
Recovery: <restore runbook at <path> | none; accepted risk>

To record your acknowledgment, reply in your own words naming the irreversible part. Anything else leaves the record's acknowledgment open.
```

A reply that names the irreversible part or the action acknowledges it, in any language. A bulk "use the recommended
answers" never acknowledges it. Without the acknowledgment, the record is written with the acknowledgment marked open
and its status stays `Draft`.

## Phase 6: Comms plan

Draft who needs to know, when, and through which channel:

| Audience              | When                      | Channel              |
| --------------------- | ------------------------- | -------------------- |
| Affected service team | Before the change (T-24h) | <channel>            |
| Engineering           | At the change (T-0)       | <channel>            |
| Customers             | Before (T-7d) and at      | Status page or email |
| Security team         | Before, when relevant     | Restricted channel   |
| Compliance or legal   | Before, when in scope     | Email                |

Leave out a row that does not apply to the tier ([classification-guide.md](classification-guide.md) gives the scope of
each). Write each draft inline in the record. Gate sends none of them; the user sends them, or asks gate to post one
through a tool the session has, which is an unnamed post confirmed once after it is shown.

## Phase 7: Schedule and freeze check

1. **Window:** date, time, duration and time zone.
2. **Day and time policy:** the organization's rules, such as no Friday afternoons, no holidays, no customer business
   hours for a B2B service, or no end of quarter for finance systems.
3. **Conflicts:** another change in flight or scheduled for the same systems in the same window.
4. **Freezes:** code, compliance or management freezes.

A conflict or a policy concern goes into the round with a proposed different window. When the user names no acceptable
window, the run stops: the record is not written, and the report says the change needs a new plan. A conflict is never
buried in the record.

## Phase 8: Write and submit the record

### Step 1: Confirm the record

Render the full record once, from the template, in a fenced block. Under it, name the path it will be written to and ask
for one confirmation. Any clear affirmative confirms. "Yes, but change X" is an edit request: revise, render again, and
ask again.

**The path.** The default is `change-records/<YYYY-MM-DD>-<slug>.md`, or the folder the user named. When the folder does
not exist, the confirmation says it will be created; it is not a second question. When the user says the repository is
not where their change records live, render the record in chat only and write no file.

### Step 2: Write the file

Write the record with the template's guidance comments stripped. Report the path, not the record again.

**The deploy hook.** Where the Agentics hooks gate deploy commands, a deploy run from the coding tool is refused until
`AGX_RELEASE_RECORD` names a fresh change record. The hook checks the file's heading and age, not its status, so gate
gives this pointer only for a record that can go ahead. Gate cannot see the environment that started the tool, so say
once, after writing a record whose status is not `Draft`:

```text
If your deploy commands are gated on a change record, set AGX_RELEASE_RECORD to this file in the environment that starts your coding tool, not on the deploy command line, and restart the tool. The record counts for 24 hours after it was last written.
```

For a `Draft` record, such as one whose irreversibility acknowledgment is open, say instead that the record is not ready
for a gated deploy until its open items are settled and it is written again. When the record was rendered in chat only,
no file exists: say that the deploy hook needs a written record, so a chat-only record cannot satisfy it.

### Step 3: Post through an adapter

When the user named an adapter up front, use it. Otherwise ask one question:

```text
❓ **Q1** - **Post the record**: Post this change record to an external system? Answer `github` (a GitHub issue with the change-record label), `none` (the file is the record), or the name of another adapter.

➡️ `github` when the repository already has a `change-record` label, because the team tracks records as issues; otherwise `none`.
```

| Answer          | Action                                                                   |
| --------------- | ------------------------------------------------------------------------ |
| `github`        | Load [adapters/github-issue.md](adapters/github-issue.md) and follow it. |
| `none`          | The file is the record.                                                  |
| A known adapter | Load its file in [adapters/](adapters/README.md) and follow it.          |
| Any other name  | List the adapters that exist, leaving out `stub-` files, and ask again.  |

### Step 4: Link the related artifacts

After the file is written (and the post, when there was one), offer the back-links as one batch for one confirmation; a
back-link the user named up front needs no question:

- **A related pull request:** a comment that links the record, through `gh`, in the
  [posting envelope](../../../gh/references/posting-envelope.md) with the marker
  `<!-- agentics:gate:change:{record-id} -->`. Search the pull request for a comment with that marker by the
  authenticated login first, and update it in place when one exists:

    ```text
    <!-- agentics:gate:change:{record-id} -->

    Change record: {issue URL or file path}

    > _Written by an AI agent_
    ```

- **A related runbook:** one entry in its change history that names the record.
- **The record's issue,** when the record was posted: a comment that carries the rollback command, when the body left it
  out for length.

Every posted text first passes the outbound rule in [SKILL.md](../../SKILL.md#cross-cutting-rules-all-four-modes).

## Write the rows

Write the rows through `agx flow log`, as [emission.md](../emission.md) says, and leave them staged. A field that does
not apply is `na <field>`, and a field that applies but could not be filled from evidence or the user's answer is
`skip <field>`, where `<field>` is the record section: `scope`, `risk`, `security-scan`, `compliance`, `approvals`,
`rollback`, `comms`, or `schedule`.

The verdict row's `target` is the record ID, its `evidence` the file path or the issue URL, and its `status` the first
match:

| Situation                                                                                       | `status`    |
| ----------------------------------------------------------------------------------------------- | ----------- |
| The user stopped the run before the record was written                                          | `cancelled` |
| The record was not written: no acceptable window, or the path could not be written              | `blocked`   |
| The security scan left a Critical or Important finding open                                     | `failed`    |
| A field was skipped, a required scan did not run, or the irreversibility acknowledgment is open | `pending`   |

A sensitive surface with no diff, recorded as `not scanned: no diff`, is a scan with nothing to run, not a skipped scan,
so it leaves the status `ok`. | The record was written with every applicable field filled | `ok` |

A failed adapter post does not change the status: the file is the record, and the report names the failure.

## Common pitfalls

- **A rushed classification.** A wrong tier gives a wrong approval chain and comms scope.
- **An approval chain read as approval.** Naming an approver approves nothing; the record documents who will.
- **A copied rollback plan.** A stale plan is how an irreversible change ships unnoticed. Walk it every time.
- **A wishful schedule.** "We'll fit it in" is not a window.
- **Posting before the record is reviewed.** The post is the last step; a draft posted for visibility muddies the audit
  trail.

## Composition with other skills

- **`/agentics:review`** runs in Phase 3 for a security-sensitive change.
- **`/agentics:gate release`** looks for this record in its Section 1.
- **`/agentics:ops response`** is where an emergency change usually starts; the incident commander may co-author the
  record afterwards.
- **`/agentics:ops`** owns the runbooks a record points to.
- **`/agentics:gh`** carries the GitHub adapter's post and the back-link comment.
- **`/agentics:git`** commits the record file under its commit contract when the user asks.
