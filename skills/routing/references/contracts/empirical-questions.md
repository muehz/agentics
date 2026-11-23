# Empirical questions

Before asking the user or taking a question to `challenge`, separate facts an observation can settle from product or
preference choices. A check can establish behavior, timing, output shape, what a layout looks like, or whether an
approach works. Ask the decision owner for choices that observation cannot settle.

## When to run

Run an existing check in this session, such as reading a log, checking a job status, or running a measurement command.
When answering the question requires building an experiment, use [prototype](../../../prototype/SKILL.md).

- A cheap check takes one command or one throwaway script and minutes rather than hours. Run it without asking when the
  current permissions and scope already allow it.
- For a costly check in an attended session, offer the experiment as the recommended answer using the
  [question format](question-format.md). Give an estimate, for example: "I can prototype this in about N minutes. Want
  the measurement instead of deciding blind?" Start only when the user agrees. A decline or an unanswered offer leaves
  the question open.
- In an [unattended](unattended.md#when-a-run-is-unattended) run, keep a costly check as a `blocker`. Name the
  experiment and the evidence needed. Leave dependent decisions open and proceed only on work that can stand without the
  result.

A probe keeps the caller's scope and other permissions. If the check cannot run or returns no observation that settles
the question, name what is missing and leave the question open.

## Record the result

State what ran, what it showed, and any limit on what it establishes. Record the observed answer as a lookup in the
caller's result or artifact. Consent to run an experiment and a preference choice belong in the decision record
separately from the observation.
