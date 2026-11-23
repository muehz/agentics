# Scaffold reference: GOVERNANCE.md

## Questions to ask

Ask what's open as one ❓ round, each question with a ➡️ recommendation drawn from the request and the repo. Skip a
topic the request already settles.

- **Primary owners or leads:** name + role, e.g. "Engineering Lead", "Product Owner".
- **Decision-making process:** consensus, lead approval, RFC process, architecture review board, etc.
- **RFC/ADR process,** if any, and where RFCs/ADRs live: the ADR directory
  [repo-conventions.md](../../../routing/references/repo-conventions.md) resolves, or its default when none exists.
- **Escalation path** for disagreements or blocked decisions.
- **External stakeholders or governance bodies** that must be consulted for major changes, if any (e.g. Platform Team,
  Security, Architecture Review Board).

Fill {ADR directory} with the resolved directory. Never generate a second ADR location beside an existing one.

> **Empty repo advisory:** Applies to a brand-new repo with no established team or decision history. Suggest that the
> team defer GOVERNANCE.md until it has made at least one key decision. A placeholder GOVERNANCE.md with no real content
> has little value.

## Structure

Follow this order:

1. **Overview:** one paragraph on how the project is governed and who owns it
2. **Roles and responsibilities:** table or bullet list of roles (Engineering Lead, Contributors, Reviewers, etc.)
3. **Decision making:** how day-to-day and architectural decisions are made
4. **RFC / ADR process** _(if applicable)_: how to propose, review, and record significant decisions; link to the
   decisions directory
5. **Escalation:** what to do when a decision is blocked or disputed
6. **External governance** _(if applicable)_: any ARB, Security, or organizational review requirements

## Example output

```markdown
# Governance: {repo-name}

## Overview

{repo-name} is owned and maintained by the {team-name} team. Day-to-day decisions are made by the Engineering Lead with
consensus from active contributors. Significant architectural changes go through the RFC process documented below.

## Roles and Responsibilities

| Role                   | Responsibility                                               |
| ---------------------- | ------------------------------------------------------------ |
| Engineering Lead       | Final decision authority; approves RFCs and breaking changes |
| Contributors           | Submit PRs; participate in RFC discussions                   |
| Reviewers (CODEOWNERS) | Required approvers for all PRs; see `CODEOWNERS`             |

## Decision making

- Routine changes: PR approval from any CODEOWNERS reviewer
- Architectural changes: RFC required (see below)
- Breaking changes: Engineering Lead sign-off required

## RFC Process

Significant decisions are recorded as Architecture Decision Records (ADRs) in `{ADR directory}`.

To propose a change:

1. Copy the ADR template from `{ADR directory}/template.md`
2. Open a draft PR with your proposal
3. Allow 5 business days for async review
4. Merge with Engineering Lead approval

## Escalation

Blocked decisions escalate to the Engineering Lead within 2 business days. Cross-team disagreements escalate to the
Platform Architecture Review Board.

## External governance

Major platform or security changes require review by the Architecture Review Board (or equivalent governance body in
your organization) before implementation. Contact the Architecture team via your organization's standard channel.
```

## Notes

- GOVERNANCE.md is valid in `.github/`, the repo root, or `docs/`, checked in that order. The idiomatic location is the
  **root**.
- This file is separate from CONTRIBUTING.md. GOVERNANCE covers _who decides_ and _how decisions are made_. CONTRIBUTING
  covers _how to submit changes_.
- **Do NOT duplicate CONTRIBUTING.md content.** Omit PR steps, branching, and commit conventions because they belong in
  CONTRIBUTING.md.

## Conformance check (audit mode)

✅ Conformant if the file contains at least one section that covers project roles or a decision-making process.
