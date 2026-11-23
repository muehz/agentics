# Classification guide

`/agentics:gate change` loads this guide in Phase 2 (Classify). The tier drives approval chain, comms scope, and freeze
policy. The default is ITIL's three tiers. Orgs with their own tiers adapt through "Org-tier mapping" below.

## The three tiers (ITIL default)

### Standard

**Definition:** Pre-approved, repeatable, low-risk changes that follow a documented procedure (typically a runbook). The
approval is **standing**: a policy pre-authorizes this class of change. The record exists for audit traceability, not
approval routing.

**Characteristics:**

- Procedure is documented in a current, drilled runbook
- Outcome is highly predictable (the procedure has been executed N times without incident)
- No new infrastructure, no new dependencies, no security-surface changes
- Reversible with a tested rollback

**Examples:**

- Quarterly TLS certificate rotation per runbook
- Scheduled secret rotation (DB passwords, API keys) on a routine cadence per runbook
- Patch-level dependency bumps (vetted by Dependabot + CI green)
- Capacity adds within pre-approved bands (e.g., "add up to 2 more replicas of <service>")
- Routine database vacuum / maintenance per runbook

**Approval shape:** Standing policy. Name the policy in the record; no individual approvers.

**Comms scope:** Minimal. Internal notification of execution; no customer comms.

**Freeze policy:** Subject to deploy freezes, but otherwise unrestricted.

### Normal

**Definition:** Planned, non-emergency changes that need case-by-case evaluation and explicit approval. The default for
anything that doesn't clearly fit Standard or Emergency.

**Characteristics:**

- New infrastructure, new dependencies, or security-surface changes
- Outcome has some uncertainty (first execution, or executed rarely)
- Reversible with a documented rollback, OR irreversible with explicit acknowledgment
- Affects customer-facing systems or downstream integrations

**Examples:**

- Major version dependency upgrade (e.g., framework v3 → v4)
- New microservice deployment
- IAM role / policy changes
- Database schema migration (additive or destructive)
- Feature flag rollout to a significant audience cohort
- DNS / load-balancer reconfiguration
- New external integration / vendor onboarding

**Approval shape:** Named individual approvers based on affected systems and risk.

**Comms scope:** Pre-change (internal at minimum; customer-facing if customer-visible); at-change announcement;
post-change confirmation.

**Freeze policy:** Subject to all org freezes.

### Emergency

**Definition:** Unplanned change that must happen now to address an active issue (typically a security or reliability
incident). Documentation is the priority; formal approval is post-hoc.

**Characteristics:**

- Driven by an active P1/P2 incident or imminent customer-impact threat
- Time to execute < time to route normal approval
- Change-record may be drafted in parallel with execution, or after the fact (within 72h is typical)
- May be irreversible

**Examples:**

- Revocation of a compromised credential during an active security incident
- Forced failover of a database during a primary outage
- A hotfix push for a P1 customer-impact bug
- Disablement of a feature flag in response to an active fire
- Removal of a malicious dependency or container image from production

**Approval shape:** Post-hoc. The IC (or operator under IC instruction) executes. The change-record names the approvers
who confirm retrospectively, typically within 24–72h.

**Comms scope:** During-incident comms apply (see `/agentics:ops response`: restricted for security incidents, status
page for reliability). Post-incident, the change-record may inform postmortem messaging.

**Freeze policy:** Not subject to freezes. The emergency tier exists to operate outside normal change-control timing.

## Classification decision tree

Walk this in order:

1. **Is this happening in response to an active P1 or P2 incident?**
    - Yes → **Emergency**. Stop here.
    - No → continue.
2. **Is this a change that has a documented, drilled runbook AND a standing approval policy?**
    - Yes, and the runbook covers exactly this procedure → **Standard**.
    - Yes, but the runbook is stale (failed `/agentics:ops audit`) or hasn't been drilled in 6+ months → **Normal**. The
      reason: the "standing approval" assumes the runbook is trustworthy, and this runbook is not.
    - No standing policy applies → continue.
3. **Default:** **Normal**.

When uncertain between Standard and Normal, pick Normal. Over-classification as Standard breaks audit trust ("we said
this was pre-approved but it wasn't really"). Over-classification as Normal only adds friction.

## When to escalate the classification

If new information during the workflow suggests the tier was wrong, **re-classify** the change. **Announce** the
re-classification, because the approval chain and comms scope change with it. Don't let a Standard change quietly become
Normal as risk grows during planning.

Example:

```text
Re-classifying from Standard → Normal: the runbook for this procedure was last tested 9 months ago and `/agentics:ops audit` flags command drift. Standing approval is voided when the runbook is stale.
```

## Org-tier mapping

If your org uses other tier names (Tier 1/2/3, P1/P2/P3, Low/Med/High), document the mapping at the top of the
change-record. The mapping lets auditors interpret the record across orgs. A common informal mapping:

| ITIL default | Common informal equivalents                        |
| ------------ | -------------------------------------------------- |
| Standard     | Tier 3 / Low-risk / Routine / Pre-approved         |
| Normal       | Tier 2 / Medium-risk / Planned                     |
| Emergency    | Tier 1 / High-risk / Urgent / Hotfix / Break-glass |

For SOX or other regulated contexts, defer to the audit team's prescribed taxonomy, if any. In those contexts, document
the mapping inline in the change-record.

## Common classification mistakes

- **Everything is Normal.** Standard exists specifically to reduce friction for high-volume routine changes. If every
  dependency bump is Normal, the approval chain is performative and people stop reading.
- **Everything is Standard.** "We've done this before" is not the bar. Standard without a current runbook AND standing
  approval is just unapproved Normal.
- **Misuse of Emergency.** "I really need this approved fast" is not Emergency. It's Normal with a tight schedule.
- **Reclassification down without explanation.** A move from Normal → Standard during planning needs a reason. Examples:
  a new runbook landed, a policy was updated. A bare relabel breaks audit trust.
