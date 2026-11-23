# Severity

Every finding that leaves the skill that produced it carries one of four severities. A producer may print its own scale
beside the shared one. A consumer reads only the shared one.

| Severity      | Meaning                                                                        | Effect                                 |
| ------------- | ------------------------------------------------------------------------------ | -------------------------------------- |
| Critical      | Data loss, a security compromise, or the changed behaviour broken for everyone | Blocks. Judgment alone never lowers it |
| Important     | A defect in required behaviour, a broken contract, or a failed required check  | Blocks until fixed, accepted, deferred |
| Minor         | A real defect or deviation that does not block                                 | Counted and listed                     |
| Informational | Context                                                                        | Listed, never counted                  |

## Producer mapping

The word Nit is retired in favour of Minor. Each producer maps as follows.

| Producer                                                           | Critical   | Important          | Minor           | Informational |
| ------------------------------------------------------------------ | ---------- | ------------------ | --------------- | ------------- |
| Security lane                                                      | Critical   | High               | Medium, Low     | context       |
| Standards lane                                                     |            | `HARD`             | `SOFT`          |               |
| Spec lane (its status decides; it has no separate severity column) |            | `MISSING`, `WRONG` |                 | `ADDED`       |
| Check-weakening, controller audit, red-commit check                |            | Important          | Minor           |               |
| `execute` quality and aggregate reviewers                          | critical   | important          | minor           |               |
| A source with `CRITICAL`, `HIGH`, `MEDIUM`, `LOW`, as in `triage`  | `CRITICAL` | `HIGH`             | `MEDIUM`, `LOW` |               |
| CI, Copilot or a person, which state no severity                   | by meaning | by meaning         | by meaning      |               |

For the last row, the skill that records the finding assigns the severity from the meanings above.

## Rules

- A review bucket says what the lead recommends; it does not rename the severity. A Standards `HARD` finding moved to
  Consider is still Important for `review` and for `gate`.
- A Minor finding from the Security lane goes into the pull-request body where it may be posted. Where it is withheld
  (an unfixed finding on a public repository, or an embargo), it goes to the user in the session. Other Minor findings
  stay in the report.
- A blocking finding of a spec or plan review is the exception to "accepted, deferred" in the Important row: it is
  closed only by a fix or a revision of the artifact.
- `P1` to `P4` name only an `ops` incident priority. No other skill uses those labels.
- A consumer reads the open tally, never a verdict word. Verdicts are derived from it.

## Verdicts

| Producer                                     | No open Critical or Important | Open Important, each a bounded fix | Anything else   |
| -------------------------------------------- | ----------------------------- | ---------------------------------- | --------------- |
| Packet, plan and spec reviewers              | `READY`                       | `REVISE`                           | `REVISE`        |
| Quality and aggregate reviewers              | `CLEAR`                       | `FINDINGS`                         | `FINDINGS`      |
| `review` interactive                         | Approve                       | Approve with fixes                 | Request changes |
| `review` axes (tallies per lane, no verdict) | none                          | none                               | none            |
| `gate`                                       | READY                         | NOT READY                          | NOT READY       |

A bounded fix changes neither the approach nor an interface, so a fix that changes an interface is Request changes. The
plan reviewer's third verdict, "Major rework", is removed; a plan that needs it returns `REVISE` with a Critical
finding.
