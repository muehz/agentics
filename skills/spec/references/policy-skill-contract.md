# Policy skill authoring contract

This is the consumer-facing contract for writing a policy skill that `spec align` ([align.md](align.md)) reads
deterministically. It defines the fields a policy skill exposes so `align` can find it, follow it, and audit against it.
Decompose's coverage check and gate's converge pass read the same fields to escalate `must` findings. This is a field
contract, not a constitution file. The policies stay in the organization's own sources. The skill only indexes them.

## Description

The description must state both halves of align's match rule in one sentence. The halves are:

- The skill is an organization-level source of policy, standards, or architecture.
- The skill aligns, audits, or checks designs, specs, or work against that source.

**Passing example:** "The enterprise architecture continuum; aligns designs and specs against organizational policy and
standards before implementation."

**Failing example:** "Tracks the organization's architecture continuum and its history of decisions." The failing
example states only the first half (a policy source). Nothing says the skill aligns or audits work against it, so
`align` cannot tell it apart from a reference library.

## Policy index

The skill body carries a Markdown table with exactly these columns, in this order:

| id  | strength | rule | source | exception |
| --- | -------- | ---- | ------ | --------- |

- `id`: stable across amendments. It is never reused, even after a row is removed.
- `strength`: `must` or `should`.
- `rule`: one line.
- `source`: a repository path or URL to the existing ADR, standard, or CI check the rule points at.
- `exception`: the route to request an exception: a board, an issue template, a person or role, or `none`.

Example:

| id                 | strength | rule                                                              | source                                | exception          |
| ------------------ | -------- | ----------------------------------------------------------------- | ------------------------------------- | ------------------ |
| payments-isolation | must     | Payment traffic never shares an ingress with non-payment traffic. | `docs/adr/0012-payments-isolation.md` | architecture board |
| commit-style       | should   | Commit subjects use the imperative mood.                          | `CONTRIBUTING.md#commits`             | none               |

## Version

The skill body carries `version: <semver>` and `amended: YYYY-MM-DD` lines. Every change to a policy row or to the index
moves the version. `amended` moves with it to that day's date. Bump rules:

- **major**: any of these changes:
    - A `must` row is removed.
    - A row's strength is raised to `must`.
    - A `must` row's `rule` changes what it requires.
- **minor**: any of these changes:
    - A row is added.
    - A `should` row is removed.
    - A strength is lowered.
    - A `should` row's `rule` changes what it asks.
    - A row's `source` or `exception` points somewhere new.
- **patch**: wording that changes no requirement and no pointer (a typo fix, a clearer sentence in `rule`).

When one release holds several changes, the largest bump wins.

## Point, don't restate

Each row's `rule` is a one-line pointer to its `source`. The skill body never copies the standard's text. The source
stays the one place where the policy changes. A drift between a policy skill and its source is a doc bug. The skill is
not a second place to reconcile.

## Precedence

Where a policy skill comes from decides nothing on its own. When a repo-local policy skill, declared under
`.github/skills/`, `.claude/skills/`, or `.agents/skills/`, and a plugin-provided one both match, align asks the user
which to load and says whether the repo-local file was added or changed on the current branch
([align.md](align.md#1-discover-the-policy-skill)). Anyone who can open a pull request can add a repository file, so it
does not displace the organization's installed skill without that question. A contradiction between two selected skills,
repo-local included, is a conflict entry that names both. The choice decides which skill is loaded. It never decides
which policy wins a disagreement.

## What align does with each field

| Field       | Where it lands                                                                                                                 |
| ----------- | ------------------------------------------------------------------------------------------------------------------------------ |
| `strength`  | Shown on each conflict entry, so a `must` violation reads apart from a `should` one.                                           |
| `version`   | Recorded on the **Policy skills** line; a later `/agentics:spec align` reports a change and records it as a `note policy` row. |
| `exception` | Quoted in the conflict entry's "resolution requires" clause.                                                                   |

## Absent fields

A policy skill written without a policy index, `version`, or `exception` column still aligns. Conflict entries carry no
strength. The **Policy skills** line carries no version. The "resolution requires" clause keeps align's own wording
rather than a quoted route. Adding these fields later is additive. Nothing in [align.md](align.md) requires them.
