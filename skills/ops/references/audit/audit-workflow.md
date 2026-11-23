# Audit workflow

Runs six checks over one runbook and produces a finding list. Each finding has a severity and a recommended action, plus
a drafted edit when the user asks for one.

## Inputs

- **Required:** path to the runbook (`/agentics:ops audit runbooks/<name>.md`)
- **Optional:** the staleness threshold (default derived from the runbook's severity grade; see Check 1)

If the user gives no path, ask for one. In an [unattended](../../../routing/references/contracts/unattended.md) run,
nobody can answer, so do not stop. Do these steps instead:

1. List the candidate runbooks under `runbooks/` and `docs/runbooks/`.
2. Audit the sole candidate, or else the one that references the changed scripts or alerts.
3. State that choice as an assumption at the top of the report.
4. Return the which-runbook question in the report.

Audit one runbook per pass, because a combined audit of many runbooks produces too much noise.

## Check 1: Freshness

**Goal:** Confirm the runbook was tested recently enough for an operator to trust it.

1. **Read the `Last tested` header field.**
2. **Read the severity grade.** If the runbook carries no `Severity grade`, ask for it in the audit's ❓ round. Do not
   ask for it in plain text, in another question's ➡️, or in next steps. The ➡️ recommends a grade from the runbook's
   own evidence (the escalation targets and the blast radius its mitigation describes). The freshness finding uses that
   recommended grade, marked provisional, until the user answers.
3. **Compare against the staleness threshold** derived from severity:
    - P1-grade: 90 days
    - P2-grade: 180 days
    - P3+: 365 days
4. **Flag the finding.** `age` is the number of days since the `Last tested` date and `T` is the threshold. Read the
   rows from the top; the first match wins.

    | Order | `Last tested` reads                                                        | Finding                                                        |
    | ----- | -------------------------------------------------------------------------- | -------------------------------------------------------------- |
    | 1     | Missing, or neither a date nor the author workflow's "not yet tested" text | CRITICAL: "no test record; do not rely on it without a drill"  |
    | 2     | The author workflow's "not yet tested" text                                | HIGH: "never drilled; drill it before relying on it"           |
    | 3     | `age` > 2 × `T`                                                            | CRITICAL: "untrusted; do not rely on it without a drill first" |
    | 4     | `T` < `age` ≤ 2 × `T`                                                      | HIGH: "schedule a drill within 2 weeks"                        |
    | 5     | `T` − 30 < `age` ≤ `T`                                                     | MEDIUM: "drill on the next cycle"                              |
    | 6     | Otherwise: `age` ≤ `T` − 30                                                | OK                                                             |

    A runbook fresh from `/agentics:ops author` therefore gets one HIGH freshness finding until its first drill, and no
    other finding from the header.

**Recommended action:** Schedule `/agentics:ops drill <path>` per the severity.

## Check 2: Command drift

**Goal:** Confirm the commands in the steps still reference live tools, paths, and services. This check costs the most
work and has the highest value.

Scan two inputs independently: command-bearing steps and the runbook's Escalation table.

For each step that contains a command:

1. **Identify referenced tools.** `kubectl`, `aws`, `gh`, `terraform`, custom scripts, etc.
2. **Identify referenced resources.** Cluster names, namespaces, account IDs, service names, env vars, and file paths.
3. **Spot-check resource existence.** Where possible without side effects (read-only operations only):
    - Cluster exists: `kubectl config get-contexts | grep <cluster>`
    - Namespace exists: `kubectl get ns <namespace>` (read-only)
    - AWS account / role exists: `aws sts get-caller-identity` (verify against expected)
    - File path mentioned: confirm path exists in current repo if it's a repo-relative path
4. **Flag mismatches:**
    - Referenced resource not found → HIGH
    - Tool version pinned but pinned version unavailable → MEDIUM
    - Command uses deprecated subcommand or flag → LOW

Once per runbook, confirm that every PagerDuty schedule, Slack channel, and on-call handle in the Escalation table still
resolves. Use a read-only API or directory lookup. For each target that cannot be verified this way, ask the user to
decide its fate. Use a ❓ question with a ➡️ recommendation. The choices are to keep it, replace it (with what), or drop
it. It is not enough to confirm that the target merely still exists. The user picks what happens to it. A stale
escalation target is a HIGH resource mismatch even when the runbook has no command-bearing steps.

**Recommended action:** Update the command inline.

If the resource was renamed, do these steps:

1. Update the placeholder.
2. Add a migration note to the change history.

## Check 3: Dead links

**Goal:** Confirm linked dashboards, docs, and escalation pages still resolve.

For each link in the runbook:

1. **Categorize:**
    - Internal repo links (relative paths) → check existence
    - External links (URLs): optionally fetch and verify HTTP 200. Fetch with the user's permission, because some URLs
      may be internal or auth-gated.
2. **Flag mismatches:**
    - Broken internal link → MEDIUM
    - Broken external link → MEDIUM (unless auth-gated, in which case note "unable to verify; user check")

**Recommended action:** Replace dead links. A dead dashboard link may mean the dashboard moved. Find the new path.

## Check 4: Structural gaps

**Goal:** Confirm the runbook has all nine mandatory sections from the template, and that each section has real content.

For each mandatory section (Header, Purpose, When to use, When NOT to use, Prerequisites, Steps, Verification, Rollback,
Escalation):

1. **Section present?** If not → HIGH
2. **Section has real content?**
    - "When NOT to use" with no anti-triggers → MEDIUM
    - "Rollback" empty or just "TODO" → HIGH (a runbook without rollback is incomplete)
    - "Escalation" with no concrete recipients → HIGH
    - "Prerequisites" empty → HIGH
3. **Per-step structure (each step in Steps section):**
    - No verify clause → HIGH
    - No if-verify-fails clause → MEDIUM
    - A command whose syntax differs between Bash and PowerShell, given for one shell only, in a runbook not marked
      platform-specific → LOW
    - Placeholders in prose instead of code → MEDIUM

**Recommended action:** Report each gap with its specific step or section. If there are more than 5 gaps, recommend a
re-author, not a patch.

## Check 5: Ownership

**Goal:** Confirm the named owner still exists and is reachable.

1. **Read the `Owner` header field.**
2. **Verify the handle:**
    - Check `CODEOWNERS` for the repo. Is the handle listed?
    - Check the org directory if accessible (via `gh api /orgs/<org>/members/<handle>` or equivalent)
    - If verifiable, confirm it
    - If unverifiable, ask the user who owns the runbook now, as a ❓ question. The answer is the current owner, if
      still on the team, or a successor. The ➡️ line recommends the basis, not a person. The basis is a named individual
      rather than a team, drawn from `CODEOWNERS` or the most active recent committer on this area, if any. Never name a
      candidate yourself.
3. **Flag.** Read the rows from the top; the first match wins. A repository with no `CODEOWNERS` file is normal, so its
   absence is never a finding.
    - The user or a lookup says the owner has left → HIGH ("owner gone; reassign")
    - The owner is a team name, not a person → MEDIUM ("owner should be a specific person")
    - No lookup could confirm the owner → `pending user decision`, through the question in step 2
    - The owner is confirmed but on extended absence (when knowable) → LOW ("reassign or add a backup")
    - Otherwise → OK

**Recommended action:** Reassign ownership through the ❓ question in step 2, never to a person the audit picked.

## Check 6: Scope and adjacent runbooks

**Goal:** Confirm the runbook hasn't drifted in scope, and that related runbooks are cross-linked.

1. **Title vs body coherence.** Does the title still describe what the body does? If the body has expanded to cover
   multiple procedures, the runbook should be split.
2. **Trigger overlap with adjacent runbooks.** For each adjacent runbook (from the Related runbooks section), check
   whether the triggers overlap. Two runbooks with the same trigger may lead operators to pick the wrong one.
3. **Missing cross-links.** Scan the repo for other runbooks. If any are obviously related but not linked, flag → LOW.

**Recommended action:**

- If scope drift detected → split into multiple runbooks (re-author)
- If trigger overlap → tighten triggers, or add explicit anti-trigger rows
- If missing cross-links → add them

## Output: the audit report

After all six checks, produce a report with two parts:

- **Step status.** One line for each step in the Steps section, in runbook order. Each step has exactly one status:
    - current (checked against its scripts and alert configs, no finding)
    - stale (a finding; cite the changed source)
    - uncertain (give the reason it could not be checked)
- **Findings.** The severity-ranked finding list from the six checks. Section-level findings (rollback, escalation,
  owner, structure) appear only here, not in the step status.

```markdown
# Audit: <runbook path>

**Audited on:** YYYY-MM-DD **Severity grade:** <P1 | P2 | P3 | P4> **Last tested:** <date from header> (<N> days ago,
threshold <M> days)

## Step status

- Step 1 (<step title>): current
- Step 2 (<step title>): stale - <changed source, such as `scripts/failover.sh` or the alert config>
- Step 3 (<step title>): uncertain - <reason the step could not be checked>

## Findings

### CRITICAL

- <finding 1: check name, specific location, and recommended action>

### HIGH

- <finding 2>
- <finding 3>

### MEDIUM

- <finding 4>

### LOW

- <finding 5>

## Recommended next steps

1. <Most important action, typically a drill or a specific edit>
2. <Next>
3. <Next>

## Drafts

<If the user requested edit drafts, render them here. Same draft-and-confirm rule applies.>
```

## When the audit blocks on user input

Some checks need user knowledge the audit cannot verify alone. These are a missing severity grade, an escalation
target's fate (keep, replace, or drop), and who takes over an unverifiable owner. Don't guess. Ask each as a ❓ question
with a ➡️ recommendation per Checks 1, 2, and 5 above. Mark the finding "pending user decision". Continue with other
checks.

## After the audit

Present the path forward as one ❓ question with these options in its body. Give it a ➡️ recommendation based on the
audit's findings:

1. **Patch the runbook.** Apply the drafted edits inline. Use the standard write-and-commit flow.
2. **Schedule a drill.** If drift suggests the runbook may not work as written, `/agentics:ops drill <path>` costs less
   than more inspection of the code.
3. **Retire the runbook.** Use this when the scope drift or owner-gone findings suggest the runbook is no longer
   relevant. Move it to an archive directory (`runbooks/archive/`) with an archive note.

When no finding is above LOW, the runbook is healthy. Update `Last tested` only if a drill was also run. **Audits alone
do not bump the last-tested date.**

## Composition with other modes and skills

- **`/agentics:ops drill <path>`**, when the audit can't confirm its findings by inspection. A drill shows what really
  happens.
- **`/agentics:ops author from-postmortem`**, if scope drift led to a re-author. The original postmortem may still be
  the right seed.
- **`/agentics:git commit`**, to land audit-driven edits.
- **`/agentics:gate branch`**, to close out the audit branch.
