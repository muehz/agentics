# Drill-mode references

`/agentics:ops drill` loads these files. A drill is practice before a real incident: a tabletop walkthrough or a
game-day exercise with real systems.

| File                                     | Purpose                                                                                                                                                                  |
| ---------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| [drill-workflow.md](drill-workflow.md)   | One six-phase workflow for both drill types: select → assign → run → capture → debrief → act. It also covers the delta report, the debrief format, and the action items. |
| [drill-scenarios.md](drill-scenarios.md) | Starter library: 3 reliability scenarios + 3 security scenarios                                                                                                          |

The `/agentics:ops drill` argument picks the drill type:

- `/agentics:ops drill <scenario>` is a **scenario drill**. A scenario drives role play that rehearses the IR workflow:
  declaration, comms, coordination, and role assignment. The runbook is one input.
- `/agentics:ops drill <path>` is a **runbook drill**. It exercises the runbook itself against a real or simulated
  scenario: every step, every verify, and every rollback path. Its output focuses on the deltas between what the runbook
  prescribes and what happens.

Run both. Run at least one drill quarterly (monthly if practical). Rotate scenarios so each is exercised at least
annually. Drill each runbook on its severity schedule (P1 90, P2 180, P3+ 365 days; see
[drill-workflow.md](drill-workflow.md) Phase 6). Also drill each runbook at these times:

- When it is newly authored
- After edits
- On the audit cadence
- Whenever something changed in the system it covers

Drills carry lower stakes and run more often than real incidents. Their action items often catch the same gaps at lower
cost.
