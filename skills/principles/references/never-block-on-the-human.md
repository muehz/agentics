# Never Block on the Human

_Apply when tempted to ask "should I do X?" on reversible work._

The human supervises asynchronously. Make reasonable decisions. Proceed. Let the human course-correct after the fact.
Code is cheap, and a wait is expensive.

Every permission pause stalls the pipeline and makes the human the bottleneck. Code changes are reversible and
reviewable, so a wrong decision usually costs less than a block.

Pattern:

- **Proceed, then present.** Do the work. Show the result. Do not ask "should I do X?" Do X. Explain why.
- **Reserve questions for genuine ambiguity.** Ask only when you truly cannot infer intent from context.
- **Make the system self-healing.** When you notice a problem, log it. Fix it in the next round.
- The human reviews plans, diffs, and changes on their own schedule. Design workflows for review-after-the-fact.
- A wrong implementation costs minutes to fix. A blocked agent costs the human's attention to unblock.

Boundaries:

- **Irreversible actions** (force-push, delete production data, send external messages) still require confirmation.
- **Reversible actions** (write code, edit notes, split tasks) should proceed and not block.
- **Product direction** comes from the human; _execution_ should not block.

This principle does not waive Agentics gates. Every gate is one of three classes, and the class decides what happens:

- **Safety gate.** It protects something the user cannot take back or accept for others: loss of work or history that
  cannot be undone, a change outside the machine that nobody authorized, a claim of status with no evidence from this
  session, secrets and text from outside the trust boundary, a bypassed commit hook or signature, and the user's own
  approval together with the independent reviews that come before it. It holds. Say once what is held, the harm it
  prevents, and what the user can do instead.
- **Repository policy.** A settings file switches it, such as draft-only pull requests. It holds while the setting is
  on, whatever is said in the session. Name the settings change once. Record no waiver, because nothing was waived.
- **Process gate.** Every other gate. Name its cost once. When the user's next reply still declines, yield and record
  one `waive <what>` row through `flow log`, where an Agentics gate uses its head and any other rule uses `rule <name>`
  ([Operands](../../execute/references/flow-state.md#operands)). Never ask for a reason.

This principle targets the reflexive "should I do X?" on ordinary reversible work, not these gates.

## Self-test

- Can I undo this with a revert, a reset, or a second edit? If yes, do it and present the result.
- Can I infer the intent from the request, the code, and the repository's conventions? If yes, do not ask. Only a no is
  a real question.
- Is this action irreversible: a force-push, a data deletion, an external message? Those still stop and ask.
- Is the gate a safety gate or a repository policy? If yes, it holds and this principle does not waive it.
- Is the gate a process gate, and has the user's next reply declined it after I named its cost once? If yes, yield and
  record the `waive` row.
