# Author workflow

Six phases produce a runbook that follows [../runbook-template.md](../runbook-template.md) and the principles in
[../shared-principles.md](../shared-principles.md).

For the from-postmortem variant, run the ingestion in [from-postmortem.md](from-postmortem.md) before Phase 1.

When the request names the sections it wants and asks for no interview, present the complete draft in the first reply.
Put an `<angle-bracket>` placeholder wherever the request gives no value. After the draft, ask the open owner, severity
and trigger questions as one round. The phases below still set what the draft contains.

A section the user asks for that the template lacks, such as detection, goes in as an extra section after Steps.

## Phase 1: Scaffold

**Goal:** Create the file with template structure in place. Populate the header.

1. **Confirm the storage location** by [Where runbooks live](../../SKILL.md#where-runbooks-live): `runbooks/<name>.md`
   in the consuming repo, its existing layout, or the path the user names. In the Agentics plugin repo, the draft goes
   to chat unless the user still asks for the file after hearing the cost once.

2. **Pick a name.** Action-shaped: "failover-payments-primary.md", "rotate-oauth-client-secret.md",
   "recover-from-queue-runaway.md". Verb-led, not noun-led.
3. **Check for collisions.** `ls runbooks/` (or equivalent). If a runbook with a similar name exists, show it to the
   user. The user may want to edit it rather than create a new one.
4. **Skim house conventions.** Skim 1–2 existing runbooks in the target directory for local conventions. Local
   conventions include heading style, section order, and extra locally adopted sections. Match them where they don't
   conflict with the nine mandatory sections. This rule covers document style, not just the path.
5. **Create the file with the template stub.** Pre-populate:
    - Title (from name)
    - Version 1.0
    - Last tested: `not yet tested; run /agentics:ops drill before relying on this in production` until a drill happens.
      When the user gives the date of their team's own drill of this procedure, write that date, recorded as given by
      them. `/agentics:ops audit` reads the "not yet tested" text as one HIGH freshness finding.

    Ask the owner and severity grade that the request leaves open as one ❓ round. Give each a ➡️ recommendation drawn
    from the request and the repo. For example, use `CODEOWNERS` for the owner and a seeding postmortem's severity for
    the grade. Skip a topic the request already settles.

    - **Owner:** the person who maintains this runbook. Name a single person, not a team, because team members rotate.
      The named owner is the person you ask when something is unclear.
    - **Severity grade:** the P1–P4 taxonomy from response mode (`/agentics:ops response`).

**Exit criteria:**

- File exists at the agreed path with the template structure
- Header fields populated

## Phase 2: Trigger (When to use / When NOT to use)

**Goal:** The runbook is invoked at the right times and not at the wrong times.

This section has the highest impact, because a runbook used at the wrong time can cause more harm than no runbook. Spend
extra effort here. Authors tend to write the trigger last and rush it, but operators read it first.

1. **Capture the triggers.** Ask what specifically makes someone reach for this runbook. Ask it as a ❓ question with a
   ➡️ recommendation drawn from the postmortem or adjacent runbooks, if any.
    - Named alerts (with the exact alert name)
    - Symptoms users report (specific phrasings if known)
    - Metrics crossing thresholds
    - Scheduled events (quarterly rehearsals, post-deploy checks)

    Give each trigger one bullet. Make each trigger observable, not subjective.

2. **Capture the anti-triggers.** Ask what situations look similar but need a different procedure. Ask it as a ❓
   question with a ➡️ recommendation drawn from adjacent runbooks' triggers, if known.
    - Adjacent runbooks that share symptoms but diverge in root cause
    - Symptom clusters that need investigation before runbook execution
    - Severity floors ("don't use this for P3; it's overkill")
3. **Cross-reference adjacent runbooks.** If the user can name 2–3 adjacent runbooks, link them in the **Related
   runbooks** section in advance.

**Exit criteria:**

- At least 2 triggers, each observable
- At least 2 anti-triggers
- Adjacent runbooks linked (if known)

## Phase 3: Prerequisites

**Goal:** The operator can confirm they have what they need before they start.

1. **Access.** Specific. "AWS access" → "AWS IAM role `IncidentResponder` assumed in account `<acct-id>`". "kubectl
   access" → "kubectl context `payments-prod`, role `sre-operator`".
2. **Tools.** Named tools with version floors where it matters. "`kubectl` v1.28 or later"; "`gh` CLI authenticated as a
   user with write access to `<owner>/<repo>`".
3. **Context.** Where you need to be. "On the responder VPN." "Inside an incident channel." "Have the incident ID
   handy."
4. **Platform.** Include it only if the runbook is platform-specific. The template defaults to cross-platform.

**Exit criteria:**

- Access requirements listed
- Tool requirements listed
- Context requirements listed

## Phase 4: Steps

**Goal:** Each step is atomic and has a verify clause and an if-verify-fails clause. Commands are exact and
cross-platform.

Do these steps:

1. Write every step in full.
2. Present the complete sequence as one draft.
3. Ask one review question as a ❓ block, shown below.

> ❓ **Q1** - **Step review**: Which steps need correction?
>
> ➡️ None, if the draft already matches what you told me.

Iterate only on the steps the user flags. Go into detail on a single step only when the user names it. One review of the
whole procedure draws out what the user knows and no document records, with fewer interruptions than a per-step loop.

**Paste-it-in path.** If the user already has the procedure, accept all of it rather than draft from questions. The
procedure can be a wiki page, script, shell history, or doc. To normalize it into the template, do these steps:

1. Split bundled actions into atomic steps.
2. Give a second shell's block where the command's syntax differs, by the cross-platform rule in `shared-principles.md`.
3. Move placeholders into code blocks.
4. Add the missing verify and if-verify-fails clauses.
5. Present the normalized draft with every added or changed clause marked.
6. Run the same single review pass.

The added clauses are guesses about intent until the user confirms them.

For each step:

1. **One sentence that describes the action.**
    - "Drain traffic from the primary."
    - "Verify replication lag."
    - "Initiate failover."

    One action per step: split "Drain traffic and update the DNS." into two steps.

2. **Exact commands.** Write concrete executable commands, using `<angle-brackets>` inside code blocks only for values
   the operator must substitute (such as `<namespace>`, `<pod>`, `<region>`). Write a command once when it is the same
   in Bash and PowerShell, and once per shell where the syntax differs, per the cross-platform rule in
   `shared-principles.md`. Operators copy from code blocks, so never leave placeholders in prose.
3. **Verify clause.** A specific observable signal that the command had the intended effect, never just "Verify it
   worked."
    - "Command output contains the new revision number."
    - "Dashboard `payment-latency` shows P99 below 200ms for at least 3 consecutive samples."

    Never skip this clause: "Run command, then move on" is not safe under time pressure.

4. **If-verify-fails clause.** Next action, usually a rollback step, an escalation, or a retry policy. Be specific.
   "Retry once; if it fails again, escalate to the Escalation section, trigger 'Step N fails after retries'."

Do not assume context in any step. Be specific. "Verify it worked" is not a step.

Aim for 5 to 15 steps. Past 15, propose splitting the runbook; the user decides.

**Exit criteria:**

- All steps numbered, atomic
- Each step has verify + if-verify-fails
- Each command given once, or once per shell where the syntax differs
- More than 15 steps split, or kept at the user's word

## Phase 5: Verification, Rollback, Escalation

**Goal:** The runbook covers end-to-end success, end-to-end failure, and the case where the operator needs help.

### Verification (end-to-end)

These checks, unlike step-level verifies, pass only if the whole procedure succeeded. Typical checks:

- A dashboard returns to baseline for a sustained window (5–10 min)
- A synthetic check passes 3 consecutive runs
- No new alerts fire on the affected system for 10 min after completion

### Rollback

Every step that changes state needs an undo. Walk through the steps in reverse. For each:

- If it's reversible, document the undo command (once per shell only where the syntax differs)
- If it's hard to undo, document the compensation
- If it's irreversible, **state that explicitly with a warning callout**. The operator needs to know this before they
  execute the forward step.

### Escalation

Each row needs a concrete trigger and a concrete action. The default escalation matrix:

| Trigger                                              | Action                                                               |
| ---------------------------------------------------- | -------------------------------------------------------------------- |
| Step N fails after 2 retries                         | Page <named on-call>; do not continue the procedure                  |
| Verification fails AND rollback fails                | Declare P1 via `/agentics:ops response`; war room                    |
| Procedure takes longer than the SLA (default 30 min) | Page secondary on-call; reassess severity                            |
| Operator is unsure at any step                       | Stop. Page <named on-call>. Ambiguity in production is worth a page. |

Customize the SLA and the on-call names per the user's org.

**Exit criteria:**

- End-to-end verification has 2+ checks
- Every state-changing step has rollback or explicit irreversibility warning
- Escalation matrix complete with concrete triggers + named recipients

## Phase 6: Review and commit

**Goal:** The runbook is committed in a clean branch, ready to be exercised.

1. **Self-review.** Walk the whole runbook top-to-bottom as the operator at 3 a.m. Alternatively, walk it as someone who
   has never seen the system. A runbook written from the author's perspective fails that operator. Did the trigger
   section make it obvious whether this is the right runbook? Are all placeholders in code blocks? Does every state
   change have a rollback? Is the escalation matrix concrete? Does the runbook mix two procedures ("Failover the
   database, then verify queue depth.")? If so, split it into two runbooks.
2. **Optional prose review.** Invoke `docs` if the runbook is high-stakes (P1-grade) to get a second review for clarity.
3. **Cross-link.** Add the new runbook to the **Related runbooks** section of any adjacent runbooks. Operators can
   follow links in both directions to the right runbook.
4. **Name the first drill.** A runbook whose `Last tested` field still says `not yet tested` is not trusted. Name
   `/agentics:ops drill <path>` within the next two weeks as the next step in the closing summary. The skill schedules
   nothing itself; the user puts it on their tracker.
5. **Commit.** Use `/agentics:git commit`. State that the commit adds the runbook `<name>`. `git` words it in the
   repository's convention.
6. **Open a PR** (if the team's workflow is PR-based). Use `/agentics:gate branch` to close out the branch.

**Exit criteria:**

- Self-review passed
- Cross-links added to adjacent runbooks
- First drill named as the next step
- Commit, and a pull request where the team works through them

## Composition with other modes and skills

- **`/agentics:ops drill <path>`**, the first drill of the new runbook. Run it within 2 weeks after you author the
  runbook.
- **`/agentics:ops audit <path>`**, to schedule recurring audits per [../audit/README.md](../audit/README.md) (quarterly
  for P1-grade runbooks, twice a year for P2, once a year for P3+). The staleness threshold is configurable.
- **`docs`**, for prose review of high-stakes runbooks.
- **`/agentics:git commit`** and **`/agentics:gate branch`**, to land the runbook on the main branch.
