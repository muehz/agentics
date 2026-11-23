# Audit mode references

`/agentics:ops audit` loads these references to review an existing runbook for staleness, drift, and structural gaps.

| File                                   | Purpose                                                                                  |
| -------------------------------------- | ---------------------------------------------------------------------------------------- |
| [audit-workflow.md](audit-workflow.md) | Six-check audit: freshness, command drift, dead links, structural gaps, ownership, scope |

An audit takes 15–30 min per runbook. Audit on a fixed schedule. The schedule is at least quarterly for P1-grade
runbooks, twice a year for P2, and once a year for P3+.

Each finding leads to a runbook edit (the most common result) or a `/agentics:ops drill`. A drill fits when the audit
can't tell whether the runbook still works without a run.
