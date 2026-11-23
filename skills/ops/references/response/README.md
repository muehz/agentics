# Response-mode references

`/agentics:ops response` loads these files to coordinate an active incident.

| File                                           | Purpose                                                                                           |
| ---------------------------------------------- | ------------------------------------------------------------------------------------------------- |
| [response-workflow.md](response-workflow.md)   | The 5-phase workflow shared by both branches                                                      |
| [reliability-branch.md](reliability-branch.md) | Outage / regression handling — rollback, failover, status page, customer comms, error-budget burn |
| [security-branch.md](security-branch.md)       | Security-incident handling — containment, forensics preservation, restricted comms, legal flags   |

The workflow branches at the **declare** phase on the reliability-vs-security question. Load the branch reference then.
