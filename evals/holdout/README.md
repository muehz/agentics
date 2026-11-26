# Holdout eval scenario banks

This directory holds **unseen holdout scenario banks** (`evals/holdout/<skill>.md`) used to detect overfitting after a
skill revision or optimization pass against the primary pinned banks in [`evals/skills/`](../skills/README.md).

## Blinding rule

**Fix agents, iterative skill-optimization sweeps, and revision loops MUST NEVER read this directory.**

- Optimization and fix passes operate strictly from extracted behavioral gap lists produced against
  `evals/skills/<skill>.md` (never reading `evals/skills/` or `evals/holdout/` directly).
- Holdout banks are reserved for **post-freeze generalization checks** run by a maintainer or independent evaluator
  after skill edits are frozen (for example, via `evals observe` cases or independent review runs).
- Never copy phrasing, edge-case examples, or fixture names from `evals/holdout/` into `SKILL.md` or `references/`. If a
  holdout run uncovers a genuine defect, express it as a general behavioral principle or template bug fix—never a
  card-specific patch.

## Relationship to `evals/skills/`

- **`evals/skills/<skill>.md`** is the pinned development/regression bank consumed by `evals cycle`, hashed into
  `evals/skills.yaml` digests, and audited by `evals check`.
- **`evals/holdout/<skill>.md`** is intentionally outside `evals/skills/` so holdout additions and rotations never
  perturb `evals/skills.yaml` digests or leak into standard `evals cycle` development packets.
- Holdout banks follow the same Markdown contract as [`evals/skills/README.md`](../skills/README.md) (`## happy`,
  `## edge`, `## routing_boundary`, `## adversarial`, fixed fenced user turns, observable criteria per
  [ADR 0022](../../docs/adr/0022-eval-strategy.md), pressure classes per
  [ADR 0014](../../docs/adr/0014-approval-and-pushback.md), and concrete fixture values checked statically by
  `eval-bank-fixture-completeness`).

## Promotion and rotation

- Keep a holdout bank untouched while iterating on a skill.
- If a holdout scenario is exposed during debugging or promoted into `evals/skills/<skill>.md` as a permanent regression
  test, remove it from `evals/holdout/<skill>.md` and replace it with a fresh unseen scenario exercising a different
  domain or failure mode.
