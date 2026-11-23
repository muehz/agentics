# Runbook examples

Illustrative examples ship with the skill. They are not Agentics's own operational runbooks. Agentics is a marketplace,
not an operational repo. Agentics never gains a `runbooks/` directory at its root.

## Available examples

| File                                                                         | Description                                                                                                                                                                            |
| ---------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| [example-database-failover-runbook.md](example-database-failover-runbook.md) | A procedure to fail over a primary Postgres instance to a standby. It demonstrates the full template, including a deliberately stale `Last tested` date that exercises the audit mode. |

## How to use these

- **Reading reference.** See what the template produces when you fill it in.
- **Audit dry-run.** Invoke `/agentics:ops audit skills/ops/references/examples/example-database-failover-runbook.md`.
  The example surfaces staleness findings by design.
- **Drill dry-run.** The example has enough detail to run the drill-mode workflow as a tabletop drill. No real procedure
  is needed.

## What examples should NOT be

- Do not copy them to the Agentics repo root.
- They are generic and not authoritative for any system in the user's stack.
- Nobody keeps them fresh. The `Last tested` dates are deliberately old to surface audit findings.

## Adding new examples

A new example follows these rules:

- Put it under `skills/ops/references/examples/`
- Use the filename prefix `example-` so it is never confused with a real runbook
- Include a header note: `> EXAMPLE: Not a live runbook. Illustrative use only.`
- Use `<placeholder>` style for any system-specific reference. Do not name real services
