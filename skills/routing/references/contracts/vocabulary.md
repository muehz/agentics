# Vocabulary

User-visible text uses the terms in this file. An internal term (a tier, a row kind, a step number) appears only with
its plain meaning beside it.

| Term              | Plain meaning                                                                                                                                                                                                                                                                |
| ----------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| routine tier      | The approval path for a spec that is not elevated: one question approves the spec and the plan together.                                                                                                                                                                     |
| elevated tier     | The approval path in which the spec and the plan are approved separately: for a spec that changes a public interface or contract, touches auth, a trust boundary or [data handling](#data-handling), or involves a migration, or one that planning or the user raised to it. |
| safety gate       | A gate that protects something the user cannot take back or accept on behalf of others.                                                                                                                                                                                      |
| repository policy | A rule a settings file switches, held while the setting is on, whatever is said in the session.                                                                                                                                                                              |
| process gate      | Any gate that is neither a safety gate nor a repository policy: it names its cost once and yields on the next reply that still declines.                                                                                                                                     |
| waiver            | The user's decline of a process gate, which needs no reason and has one record: a `waive` row, or the artifact or report line for a skill that writes no rows, in the order [gates.md](gates.md#process-gate) sets.                                                          |
| unattended        | A run in which the dispatching prompt says no user will answer, or the invocation is non-interactive and the prompt or host context says so, or the user said in this conversation that they will not answer.                                                                |
| Critical          | A finding of data loss, a security compromise, or the changed behaviour broken for everyone.                                                                                                                                                                                 |
| Important         | A finding of a defect in required behaviour, a broken contract, or a failed required check.                                                                                                                                                                                  |
| Minor             | A finding of a real defect or deviation that does not block.                                                                                                                                                                                                                 |
| Informational     | A finding that gives context.                                                                                                                                                                                                                                                |

A term that is not in the table is written with its plain meaning beside it until the table gains it.

## Data handling

Storing, exporting, or deleting personal, customer, or credential data, or changing who can read that data.
