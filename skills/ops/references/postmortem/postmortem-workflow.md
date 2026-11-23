# Postmortem workflow

The skill builds the artifact from [postmortem-template.md](postmortem-template.md) one phase at a time. Fields fill in
across phases.

## Foundational rule: blameless

The blameless framing in [../shared-principles.md](../shared-principles.md) is required here. Re-read that rule before
each phase.

## Phase 1: Prepare

**Goal:** Gather inputs before the meeting, or before the draft if there is no meeting. The postmortem owner does this
work. The rest of the team should arrive ready to discuss, not ready to read.

**Solo path.** When one person supplies the timeline, root cause, and action items directly in the conversation, there
is no meeting to prepare. Skip the meeting schedule and the pre-read. Draft from what they give you. The blameless rule
and the anti-fabrication rule still hold in full. A fact nobody can produce stays marked unknown. One person's
recollection is a source to attribute, not a verified timeline entry.

1. **Pull the tracking issue.** The response-mode tracking issue is the primary input. Extract severity, branch
   (reliability / security), roles, timeline, comms log, attempted mitigations, and resolution. **If no tracking issue
   exists** (the response was ad hoc / untracked), don't stall or fabricate one. Reconstruct severity, branch, and
   timeline from alert history, deploy logs, and chat exports the user provides. Mark unverifiable entries in the
   timeline. Record "response was untracked" as a candidate contributing factor.
2. **Pull supporting evidence.** Dashboards (screenshots at peak impact + at recovery), logs (key events, error
   messages), deploy diffs, customer reports, alert history. Record each input with one of these labels:
    - **confirmed**: you saw it this session
    - **reported**: someone stated it, attributed to them
    - **unavailable**: named, searched for, not found

    Sparse evidence produces a short postmortem with marked gaps. It never produces a full-looking one where inferred
    detail fills the holes.

3. **Build the draft timeline.** Convert the tracking issue's timeline into a single chronological table with UTC
   timestamps. Include alert-fire time, declaration time, every mitigation attempt, every comms event, resolution time.
   Reconstruct gaps from logs and chat history if needed.
4. **Pre-populate the template.** Fill what's already known: title, severity, dates, impact, timeline. Leave root cause,
   contributing factors, and action items for the meeting.
5. **Identify attendees.** IC, comms lead, scribe, subject experts who responded, and anyone whose work touched the
   failed system. For security incidents, add the security lead and the forensics scribe. Add legal/compliance if their
   work was involved.
6. **Schedule** within the SLA (5 business days for P1, 10 for P2, async-only for P3 typically).

**Exit criteria:**

- Draft timeline complete in postmortem document
- Template pre-populated with known fields
- Meeting scheduled (or async-comment process started)

## Phase 2: Meet

**Goal:** Reconstruct the incident together. Identify the root cause. Surface contributing factors. Generate action
items.

Prefer a live meeting for P1/P2. Async (collaborative doc, threaded discussion) is acceptable for P3 and for
time-zone-distributed teams.

### Meeting structure (60–90 min for P1, 30–45 for P2)

1. **Reset on blameless framing (2 min).** Read the rule out loud, even on experienced teams. Incident postmortems can
   trigger defensive instincts.
2. **Walk the timeline (15–20 min).** The owner reads the timeline. Attendees correct it, fill gaps, and add details.
   Stay descriptive: record what happened, in what order, and with what observations. Resist any jump to "why" until the
   timeline is complete.
3. **Identify root cause (10–15 min).** What is the deepest technical or process cause? Use the "5 whys" technique. Ask
   "why" repeatedly until the answer is structural. Structural answers include a missing test, a wrong assumption in a
   system, or an operational gap.
4. **Identify contributing factors (15–20 min).** Things that made the incident worse, longer, or harder to detect.
   Often more numerous than root causes. Examples: monitoring gap, runbook absent, dependency on a person who was OOO,
   alert routed to wrong channel, dashboard didn't show the right metric.
5. **Generate action items (15–20 min).** For each contributing factor, ask: what change would prevent recurrence or
   reduce severity? Each action item gets an explicit owner and due date in the meeting. One action item is always a
   regression test or eval that fails on this incident's defect. If the incident had no defect to test, record
   `n/a: <reason>` in its place. An example is a provider outage with no change on your side. For an `n/a: <reason>`
   item, record "—" for its owner, due date, and issue.
6. **What went well (5 min).** Record real positives only. Note strong detection, fast comms, effective coordination, or
   useful tooling.
7. **What didn't (5 min).** Record what specifically didn't work, framed at the system level rather than the person
   level. These gaps often map to action items.

### Async variant

Run the same structure in a shared document over 24–48 hours. The postmortem owner does these steps:

1. Pre-fill the timeline.
2. Set prompts for each phase.
3. Ask attendees to leave comments.
4. Consolidate the comments.
5. Resolve disagreements.
6. Send any points that remain open to the IC to decide.

**Exit criteria:**

- Timeline locked
- Root cause identified
- Contributing factors enumerated
- Action items have owners and due dates, except an `n/a: <reason>` regression item, which has "—"
- A regression test or eval item exists, or `n/a: <reason>`
- "Went well" / "didn't go well" populated

## Phase 3: Write

**Goal:** Produce the document. The owner writes it. It is not a group-write activity.

1. **Fill out the full template** ([postmortem-template.md](postmortem-template.md)) from the meeting outputs. Don't
   paraphrase the action items. Keep the exact language agreed in the meeting, with the same owners and dates.
2. **Write the summary last.** It is the one section most readers read, so write it when you have full context.
3. **Quantify impact** where possible. Numbers anchor severity for future readers. % of users, $ revenue (P1 only, often
   internal-only), duration in minutes, number of failed transactions.
4. **Cite evidence inline.** Link to the tracking issue, deploy commits, dashboard URLs (screenshot if dashboard data
   has retention limits), specific log entries.
5. **Resist editorial comments in the timeline.** Use "At 14:32 UTC, the deploy was rolled back," not "At 14:32 UTC, the
   team finally rolled back the deploy."

**Exit criteria:**

- Template fully populated
- Action items have owners and due dates, except an `n/a: <reason>` regression item, which has "—"
- A regression test or eval item exists, or `n/a: <reason>`
- Evidence linked / cited

## Phase 4: Review

**Goal:** Catch errors. Surface honest disagreements. Validate action items. Get sign-off.

1. **Self-review against the blameless rule.** Re-read every section. For each one, ask: does this name a person where
   it should name a system? If yes, re-write it.
2. **Distribute the draft for review.** Send it to attendees from Phase 2, plus stakeholders who didn't attend
   (engineering manager, parent team). For security postmortems, also send it to the security lead and legal/compliance.
3. **Time-box the review.** 24–72 hours. Action items shouldn't drift while they wait for review.
4. **Resolve disagreements.** If two attendees disagree on a contributing factor, surface both perspectives in the doc
   rather than pick one. Disagreements that stay unresolved (e.g., on a value of an action item) escalate to the IC for
   the call.
5. **Sign-off.** IC and postmortem owner approve. For security incidents, security lead also approves.

**Exit criteria:**

- All review comments addressed
- IC sign-off (and security lead, if applicable)

## Phase 5: Publish

**Goal:** The document is filed in the right place at the right access level. Action items are tracked and visible.

1. **Determine distribution scope.**
    - **Reliability postmortems:** typically full engineering visibility. Some orgs publish P1s externally to build
      trust and credibility.
    - **Security postmortems:** restricted by default, filed where
      [Where the record lives](../response/security-branch.md#where-the-record-lives) says, never as an issue on a
      public repository. A public version goes through PR and legal review first and carries no indicators of
      compromise.
2. **File the postmortem** with the template in [../github-artifacts.md](../github-artifacts.md), by
   [Writes outside the machine](../../SKILL.md#writes-outside-the-machine): draft it in chat and post it on the user's
   yes, or at once when they named the post up front.
3. **Cross-link the original tracking issue** with a comment that links the postmortem, when a tracking issue exists.
   The comment is a separate post: draft it and post it under the same rule, on its own clear affirmative or because the
   user named the cross-link up front. Approving the postmortem does not authorize it.
4. **Track action items.** Draft one GitHub issue for each action item, preview them together, and post them on one
   confirmation. Link the postmortem to each. An `n/a: <reason>` line in place of the regression test or eval gets no
   issue. Its Issue cell stays "—". On a security incident the action items go where the postmortem went, by
   [Where the record lives](../response/security-branch.md#where-the-record-lives): no action-item issue is drafted on a
   public, internal or unreadable repository. An item the outbound checks withhold is recorded in that location instead,
   and its Issue cell names the location.
5. **Seed runbooks for missing procedures.** Scan the action items for "create runbook for X," "document the procedure
   for Y," or "we needed a runbook." For each, suggest
   `/agentics:ops author from-postmortem <this-postmortem-issue-id>`. It fetches this postmortem and extracts the
   timeline, action items, and remediation steps. Then it drafts a runbook skeleton. Don't auto-invoke. Show the
   suggestion. Let the user decide.
6. **Announce publication.** The comms lead drafts the postmortem-publish announcement from
   [../communication-templates.md](../communication-templates.md). The comms lead then posts it to the engineering
   channel, or to the restricted channel for security incidents.
7. **Schedule the action-items review.** Set a follow-up checkpoint in 30/60/90 days to verify that action items get
   done. The owner is typically the IC.

**Exit criteria:**

- Postmortem filed where Phase 5 step 1 placed it: an issue, or for a security incident the advisory or private location
- Each action item is tracked: a GitHub issue, or the security record location, except an `n/a: <reason>` line
- Publication announcement posted
- Action-items review scheduled

## Common pitfalls

- **The 5 whys stops at "human error."** Continue. Why was the human in a position to make the error? Why didn't the
  system catch the error? "Human error" is never the root cause. It is only where a deeper cause showed itself.
- **Action items without owners.** If a meeting ends with "we should improve monitoring" and no owner, that's an
  aspiration, not an action item. Do not publish until every action item other than an `n/a: <reason>` line has a named
  owner.
- **Action items with vague owners.** "Platform team" is not an owner. A specific person on the platform team is an
  owner. Even if the platform-team manager is the assignee, name them.
- **No regression test.** An incident caused by a defect leaves a check behind: a regression test or eval that fails on
  that defect. Do not publish without it. `n/a` is only for an incident with no defect to test. Its reason says why, and
  "no time" is not a reason.
- **No "what went well" section.** It can feel self-congratulatory, but it builds the goodwill that makes future
  blameless postmortems possible.
- **Cross-contamination of incidents.** If two incidents shared a contributing factor, the postmortem mentions the other
  but doesn't blur into it. Each incident gets its own postmortem.

## Composition with other modes and skills

- **`docs`.** For major P1s, the owner may also write the postmortem as a long-form doc (architecture / lessons-learned
  doc). Invoke the docs skill if the value extends beyond a single issue.
- **`/agentics:ops author from-postmortem`.** Action items often produce runbook gaps. Phase 5 step 5 suggests
  `/agentics:ops author from-postmortem <id>` for each missing-runbook action item.
- **`triage`.** If the action items include "triage these 30 outstanding issues that all turned out to be related,"
  `triage issue` is the right next step.
