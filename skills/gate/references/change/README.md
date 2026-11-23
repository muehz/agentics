# Change mode references

`/agentics:gate change` writes a change record for an infrastructure or production change: what changes, why, the blast
radius, the approval chain, the rollback plan, the comms plan and the schedule.

| File                                                   | Purpose                                                                                                     |
| ------------------------------------------------------ | ----------------------------------------------------------------------------------------------------------- |
| [change-workflow.md](change-workflow.md)               | The eight parts of the record, the one question round and one confirmation, the write, the post, the rows   |
| [change-record-template.md](change-record-template.md) | The record itself: the full Markdown structure with guidance for each section                               |
| [classification-guide.md](classification-guide.md)     | The standard, normal and emergency tiers, the ordered decision tree, and mapping to an organization's tiers |
| [adapters/](adapters/README.md)                        | How a record is posted to an external system: the GitHub issue adapter, and a stub that documents the shape |

## When to use

- An infrastructure or production change needs an audit-traceable record (SOC 2, ISO 27001, ITIL, a regulated industry).
- The organization requires a change record before `/agentics:gate release`, or the hooks gate deploy commands on one.
- An emergency change needs its record written afterwards for audit.

## When not to use

- A normal feature pull request. `/agentics:gate branch` produces its record, and the pull request is the artifact.
- A routine deploy with no infrastructure change. `/agentics:gate release` is enough.
- A procedure to document for repeated use. `/agentics:ops author` writes runbooks.

## Storage

A record lives at `change-records/<YYYY-MM-DD>-<slug>.md` in the repository, or in the folder the user names for their
organization's layout (`changes/`, `cab-records/`, `audit/`). When the user says the repository does not keep change
records, the record is rendered in chat for them to move.

## Composition with other modes

- **`/agentics:gate release`** shows a record that names its artifact in Section 1, and never requires one.
- **`/agentics:gate branch`** neither produces nor reads change records.
