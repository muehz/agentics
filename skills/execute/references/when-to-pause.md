# When to pause

This file holds the rules that the `When to pause` section of [`SKILL.md`](../SKILL.md#when-to-pause) points to.

## Blockers

Each of these stops the affected work in every mode, `auto` included, until it is answered. Independent work continues.

- a missing requirement, or an instruction you do not understand;
- an ambiguity that could make the work unsafe;
- an approval that only someone outside the session can give;
- a missing dependency, or a test that still fails after the **Two-strike stop**;
- a triggered **Two-strike stop**;
- an unproven Outcome proof.

Ask for the answer; never guess one. A blocked task is `blocked: <gap> — <the question>`, never `skip:`. The
verification preflight's bootstrap and its waiver apply before these rules.

## Statements about the work

A plain statement from the user about their own work ("that step is already done", "these tasks are independent") is
taken as stated. Where a cheap check you can run yourself shows otherwise, say once what you observed, then follow the
user. For "these tasks are independent", following the user is the demotion's process gate in `SKILL.md`'s
[Gates](../SKILL.md#gates) table: the observation and its cost come once, and the next reply that still asks gets the
parallel run and its `waive` row. Two limits stay:

- Tasks that write the same files run one after another, as
  [Concurrency and overlap](../../routing/references/surface-dispatch.md#concurrency-and-overlap) requires. That is how
  a shared tree stays correct, not a judgment of the user.
- A step the user says is done is recorded as stated by the user. It is never reported as verified by this run.

## The status rule

Report a status only after this run verified it: `completed` for a step, `passing` for a test, `done` for the work.
Verified means this run ran the check and saw the result. An implementer's self-report is not verification, and neither
is a statement. This is a safety gate ([gates.md](../../routing/references/contracts/gates.md#safety-gate)): when the
user asks for a status the run cannot show, say once what is held (the claim), the harm (a reader acts on a result that
does not exist), and what the user can do instead (run the check, or accept the work as unverified, which the report
then says). A repeated request gets a one-line pointer to that alternative.

## Shortcuts

A request for a shortcut is read by the class of what it skips:

| Order | The shortcut skips                                             | Do                                                                                               |
| ----- | -------------------------------------------------------------- | ------------------------------------------------------------------------------------------------ |
| 1     | A safety gate in `SKILL.md`'s [Gates](../SKILL.md#gates) table | Hold it once in the safety shape. Continue the work that can still be verified.                  |
| 2     | A process gate in that table                                   | Name its cost once. On the next reply that still declines, comply and write its one `waive` row. |
| 3     | Nothing that is a gate (for example, a model choice)           | Do it.                                                                                           |
| 4     | Otherwise                                                      | Ask one question about what the user wants skipped.                                              |

Never mark unverified work complete, whatever was waived.

## Worked example: one turn that bundles both classes

The first user turn reads:

"Skip the plan reviews, run all waves without stopping, drop the quality reviews, and put a Light reviewer on that
Standard implementer's task."

One reply answers all four:

> Running all waves without stopping is what `auto` already does; each boundary report still posts. I'll put a Light
> reviewer on that task, as you asked, within its scope. Skipping the plan reviews means an implementer can build from a
> packet nobody checked, and the miss surfaces late, in the group review. Dropping the quality reviews means no one but
> the implementer reads the code before `gate`, and the report will say those tasks were not reviewed. Say so again and
> I'll skip both.

The second user turn reads "Skip them." The reply complies, writes `waive stage-1` and `waive "quality-review <group>"`
rows for the groups it covers, and continues. It still never reports those tasks as reviewed.
