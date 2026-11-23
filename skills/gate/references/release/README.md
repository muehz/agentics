# Release mode references

`/agentics:gate release` loads these files.

| File                                               | Purpose                                                                                                      |
| -------------------------------------------------- | ------------------------------------------------------------------------------------------------------------ |
| [release-workflow.md](release-workflow.md)         | The three phases, the one-pass checklist and its one confirmation, item outcomes, the summaries and the rows |
| [pre-deploy-checklist.md](pre-deploy-checklist.md) | What each of the eight sections checks: artifact, migrations, flags, rollback, CI, on-call, comms, window    |
| [monitor-window.md](monitor-window.md)             | The window after the deploy: its length, what to watch, anomaly classification, escalation, rollback         |
| [rollback-readiness.md](rollback-readiness.md)     | Checklist Section 4: the rollback target, the staged command, the time to recover, irreversibility           |

Gate does not deploy and does not roll back. It stages the commands, and the user runs them. Automatic deploys belong to
the CI or CD pipeline.

When the monitor window finds an anomaly, release mode forwards to `/agentics:ops response`, which owns the incident.
