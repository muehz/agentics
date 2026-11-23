# Runbook template

`/agentics:ops author` produces this structure. Strip each section's guidance comments before you publish.

The nine mandatory sections (the header block and eight `##` sections) are non-negotiable. The template marks each
optional section.

````markdown
# Runbook: <plain-English procedure name>

<!--
Use action-shaped names: "Failover the payments primary database",
"Rotate the OAuth client secret", "Recover from queue-depth runaway".
Avoid noun-only titles like "Database failover". They are harder to
match against an active incident's symptoms.
-->

<!--
Last tested is the date of the last drill, never the date of an audit.
Until the first drill, keep the text the author workflow writes
("YYYY-MM-DD (not yet tested; run /agentics:ops drill before relying on this in
production)"). `/agentics:ops audit` needs a real date here, so a runbook that
has never been drilled gets a freshness finding until its first drill.
-->

**Version:** 1.0 **Last tested:** YYYY-MM-DD **Owner:** <github handle of one person, not a team> **Severity grade:**
<P1 | P2 | P3 | P4; same taxonomy as `/agentics:ops response`>

---

## Purpose

<!--
One sentence. What does this runbook do? Resist explaining why or
how here. Purpose is the elevator pitch only.
-->

<One sentence describing what this procedure does.>

## When to use

<!--
Concrete triggers. Alerts that fire, symptoms a user sees, signals from
other systems. Each trigger gets one bullet.
-->

- <Trigger 1, e.g., "PagerDuty alert `payments-api: 5xx rate > 5%` fires">
- <Trigger 2, e.g., "Customer support reports checkout failures clustered in one region">
- <Trigger 3, e.g., "Scheduled quarterly failover rehearsal">

## When NOT to use

<!--
Anti-triggers are situations that look similar but need a different
procedure. This section prevents misapplication, which is the single
biggest cause of runbook-induced incidents.
-->

- <Anti-trigger 1, e.g., "Payment failures from a single downstream provider. Use the provider-failover runbook
  instead">
- <Anti-trigger 2, e.g., "Database CPU spike without 5xx. Use the runaway-query runbook instead">

## Prerequisites

<!--
What does the operator need before starting? Access, tools, network
location, credentials, context. Be explicit.
-->

- **Access:**
    - <e.g., `kubectl` access to the `payments-prod` cluster>
    - <e.g., AWS IAM role `IncidentResponder` assumed>
    - <e.g., PagerDuty schedule access (to confirm on-call)>
- **Tools:**
    - <e.g., `kubectl` v1.28 or later>
    - <e.g., `gh` CLI authenticated>
    - <e.g., `aws` CLI v2 with profile `prod`>
- **Context:**
    - <e.g., "You must be in the responder VPN or on an authorized network">
    - <e.g., "Incident must be declared via `/agentics:ops response` first">
- **Platform:**
    - <e.g., "macOS / Linux only", only if platform-specific; otherwise omit>

## Steps

<!--
Numbered, atomic steps with verify-after-each-step baked in. Write a command
once, in one bash block, when it is the same in Bash and PowerShell (most
kubectl, gh, git and terraform calls). Add a labeled PowerShell block only
where the syntax differs (environment variables, pipes, path separators);
step 2 below shows that shape, with a label above each block.

Every step follows this shape:

  N. <Action, one sentence>

     <Command, exact, copy-pasteable, with placeholders in <angle-brackets>>

     **Verify:** <Observable signal that confirms success>

     **If verify fails:** <Next step, usually rollback or escalation>
-->

### 1. <First action, one sentence>

```bash
<command 1> <args>
```

**Verify:** <How to confirm step 1 succeeded, e.g., "command returns a single line containing the new revision number">

**If verify fails:** <Next action, e.g., "abort the procedure; do NOT continue to step 2; escalate per the Escalation
section">

### 2. <Second action, a command whose syntax differs between shells>

**Bash (macOS / Linux):**

```bash
<command 2, Bash form>
```

**PowerShell (Windows):**

```powershell
<command 2, PowerShell form>
```

**Verify:** <observable signal>

**If verify fails:** <next action>

### 3. <Third action>

<!-- Continue numbered steps as needed. Aim for 5–15 steps per runbook;
beyond 15, consider splitting into multiple runbooks. -->

## Verification

<!--
How does the operator know the entire procedure succeeded, not just the
individual steps? This is the end-to-end check.
-->

After completing all steps:

- <Check 1, e.g., "Dashboard X shows error rate back to baseline (<0.5%) for at least 5 minutes">
- <Check 2, e.g., "Synthetic check `payment.completion` passes for 3 consecutive runs">
- <Check 3, e.g., "No new alerts fired on the impacted service in the last 10 minutes">

If any verification check fails: proceed to **Rollback**.

## Rollback

<!--
How to undo if the procedure didn't work or made things worse. Mirrors
the Steps section's shape: numbered, atomic, verified.

If any step is irreversible, state that explicitly with a warning.
-->

### 1. <First undo action>

```bash
<undo command 1>
```

**Verify:** <observable rollback signal>

### 2. <Second undo action>

<!-- continue as needed -->

### Irreversible steps

<!-- If any step in the procedure cannot be undone, give each one a warning
in this shape. If every step can be undone, replace the warning with
"None. Every step above has an undo." -->

> **Warning:** Step <N> (<what the step does>) is irreversible. <What recovery takes after it runs and what it costs,
> for example "rollback requires restore from backup with up to <N> minutes of data loss">. Do not execute step <N>
> without explicit IC approval.

## Escalation

<!--
When does the runbook give up and hand off? Each row covers one escalation
trigger.
-->

| Trigger                                            | Action                                                                             |
| -------------------------------------------------- | ---------------------------------------------------------------------------------- |
| Step N fails after 2 retries                       | Page <role / handle>; do NOT continue the procedure                                |
| Verification fails AND rollback fails              | Declare P1 via `/agentics:ops response`; page on-call IC + <subject expert handle> |
| Procedure takes longer than <SLA> (default 30 min) | Page secondary on-call; reassess severity                                          |
| Operator is unsure at any step                     | Stop. Page <role>. Ambiguity in production is worth a page.                        |

## Related runbooks

<!-- Optional but high-value. -->

- [<related runbook 1>](<relative path>), <one-line relationship description>
- [<related runbook 2>](<relative path>), <description>

## Last incident this was used in

<!-- Optional. The actual track record of the runbook. Updated each time it's used. -->

| Date       | Incident        | Outcome                        | Drift observed                      |
| ---------- | --------------- | ------------------------------ | ----------------------------------- |
| YYYY-MM-DD | <incident link> | <succeeded / partial / failed> | <yes / no; link to follow-up audit> |

## Change history

<!-- Optional. Useful for high-risk runbooks. -->

| Date       | Author   | Change                            |
| ---------- | -------- | --------------------------------- |
| YYYY-MM-DD | <handle> | Initial version                   |
| YYYY-MM-DD | <handle> | <what changed, and in which step> |
````

## Strip the template before publication

Before you publish the runbook, remove every `<!-- ... -->` HTML comment block. These blocks hold the guidance for
authors. Keep the structure. Remove only the guidance comments.

Then delete each optional section you have no content for (Related runbooks, Last incident this was used in, Change
history). Do not publish an optional section with placeholder rows. `/agentics:ops audit` reads a `<relative path>`
placeholder as a dead link.

## Section-by-section author guidance

### Header

- Write the **Title** as a verb phrase ("Failover the payments primary", not "Payments failover").
- **Last tested** is required. `/agentics:ops audit` checks freshness against this date.
- The **Owner** must be a real person (a handle), not a team name.

### Purpose

- Write one sentence. If it needs two, the runbook does too much. Split it.

### When to use / When NOT to use

- These sections have the highest impact. Misapplied runbooks cause more incidents than missing runbooks.
- Make each trigger observable (an alert name, a symptom, or a metric threshold), not subjective.

### Prerequisites

- Be explicit. "kubectl access" is too vague. Write "kubectl access to the payments-prod cluster with the SRE role
  assumed".
- If you forget a prerequisite, the operator can fail at step 1 with no way to recover.

### Steps

- Aim for 5–15 steps. Beyond 15, the runbook gets hard to follow under stress.
- Every step needs a **Verify** line and an **If verify fails** line. This pattern is non-negotiable.
- Put each command in a fenced block. Make it exact and copy-pasteable. Write it once when it is the same in Bash and
  PowerShell, and add a labeled block per shell only where the syntax differs, as the cross-platform rule in
  [shared-principles.md](shared-principles.md#cross-platform-commands) says.
- Put placeholders in `<angle-brackets>`, never in plain prose.

### Verification

- Check the whole procedure end to end. This check is separate from the Verify line in each step.
- Give each check a time bound ("for at least 5 minutes"). A check at a single point in time can miss a relapse.

### Rollback

- Every step that changes state needs a matching undo.
- Irreversible steps get explicit warnings. `/agentics:ops audit` flags a missing warning.

### Escalation

- State when the operator stops and hands off.
- The row "Operator is unsure at any step" with the action "page" is always a valid escalation.

### Related runbooks

- Link forward. Operators find a runbook more often by a link from an adjacent runbook than by a search.

### Last incident this was used in

- This table is the runbook's real track record. Update it each time the runbook is used. Drift observed during real use
  is the best input to `/agentics:ops audit`.

### Change history

- High-risk runbooks (anything P1-grade) should keep a change history. Low-stakes runbooks can skip it.
