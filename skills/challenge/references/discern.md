# /agentics:challenge discern: structured reasoning to a recommendation

Discern compares competing options and reaches one justified recommendation. The user, or the skill that invoked it,
acts on the recommendation. Discern changes nothing itself.

## Use this mode when

- Two or more plausible approaches exist and the right one is not obvious.
- Competing hypotheses or interacting dependencies need weighing.
- The user asks for design trade-offs or a recommendation among options.
- No skill clearly owns a request, and `routing`'s ambiguity ladder needs an owner picked.
- An interview ended with whole alternatives that questioning could not settle.

## Don't use when

The rows of the skill's own [Don't use when](../SKILL.md#dont-use-when) table apply to this mode as written, with the
same order and kinds. After them, read these rows from the top; the first match wins:

| Situation                                                                     | What to do                                                                                                                                                                                                                   |
| ----------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| The approach is already decided, and the request is to break it into tasks    | A `cannot` row: forward to `decompose` by [forwarding.md](../../routing/references/forwarding.md).                                                                                                                           |
| One plan needs pressure-testing, or only the user can make the open decisions | Not a route-away: say so in one line and switch to the interview for those decisions. Use the [empirical questions contract](../../routing/references/contracts/empirical-questions.md) for observable facts in either mode. |
| Otherwise                                                                     | Continue in discern. Once a recommendation is reached, name `decompose` as the next step for carrying it out.                                                                                                                |

When one option is plainly right, say so with the reason, and give the short form of the [output shape](#output-shape).

## Ownership questions

When discern runs on `routing`'s ambiguity ladder to pick an owner:

- An owner found: name the skill and the reason, and return that to the caller.
- No owner found: return strictly "no owner" as the result, with the reason. Do not escalate back to the ladder or say
  the request returns to `routing`'s ladder; `routing` takes its next rung on its own.
- Run directly by the user: when no Agentics skill owns the work, say so, and say the work can be done in the session.

## Reasoning preflight

Before you give substantive output, establish:

1. **Scope.** Task boundaries, what is in and out, the acceptance criteria.
2. **Risk.** Blast radius, reversibility, security or compliance exposure.
3. **Dependencies.** What must be true before the recommended action is safe.
4. **Uncertainty.** Which assumptions are verified and which are not.
5. **Validation path.** How the recommendation can be checked.

Show the preflight only when it exposes something that blocks the recommendation.

When the user asks, in any words, to skip the preflight or to answer fast, skip it and open with `[Preflight skipped]`.
The verification labels stay on the claims they mark, because they say which claims this session did not check. Drop the
explanatory caveats around them.

## Reasoning depth

Choose the depth from these signals. When the signals point to different levels, use the higher one.

| Signal            | Low                            | Medium                                   | High                                                                             |
| ----------------- | ------------------------------ | ---------------------------------------- | -------------------------------------------------------------------------------- |
| Files affected    | 1                              | 2 to 5                                   | 6 or more, or across modules                                                     |
| Ambiguity         | None; a clear spec             | Some: naming or shape choices            | Significant: competing designs                                                   |
| Blast radius      | Local and reversible           | Moderate; may affect dependents          | Breaking changes, migrations, security                                           |
| Domain complexity | Routine data handling, formats | API contracts, test strategy             | Architecture, data model, authentication and authorization                       |
| Behavior          | Flag material risks; be fast   | Check assumptions; give one counterpoint | Test assumptions systematically; give strong counterarguments; correctness first |

"Files affected" does not apply to a decision that is not about code. The highest of the remaining signals sets the
depth.

## Reasoning coverage

Apply all seven lenses at high depth, and shorten them in proportion at lower depths.

1. **Problem decomposition.** Sub-problems reasoned independently.
2. **Assumption analysis.** The load-bearing assumptions, each marked verified or unverified.
3. **Candidate approaches and trade-offs.** Realistic options compared on cost, risk, fit, and reversibility.
4. **Skeptical counterpoint.** The strongest objection to the preferred approach.
5. **Recommendation stress test.** The condition under which this conclusion fails, and how likely it is.
6. **Chosen approach.** The winner, its rationale, and the assumptions it depends on.
7. **Constraint check.** The chosen approach satisfies the stated scope, risk, and acceptance criteria.

Give a concise rationale, not a transcript of the reasoning.

## Output shape

Unless the user asks for another format, the response ends with:

1. **Chosen approach.** What to do and why it won against the alternatives.
2. **Risks, assumptions, and counterpoints.** What must hold true and what could break it.
3. **Next action.** The specific, observable next step.

## Verification labels

- `[Inference]`: follows from the evidence but is not directly confirmed.
- `[Speculation]`: plausible but not grounded in current evidence.
- `[Unverified]`: stated as fact elsewhere but not checked here.

Do not present an inference as a confirmed fact. Attach `[Inference]`, `[Speculation]`, or `[Unverified]` consistently
to every unsourced comparative claim across all lenses. Qualify or support an unsourced absolute such as "prevent",
"guarantee", or "will never". When a gap only the user can close remains, ask rather than guess.

**A decision needed now.** When the user wants a decision now despite missing facts, or tells you not to ask for
context:

- Give a provisional recommendation, labeled as provisional.
- Mark the assumptions `[Speculation]` or `[Unverified]`.
- State your confidence, and say that the open gap caps it.
- List what would reverse the recommendation.

Switch to the interview only when reasoning cannot proceed at all without the missing facts, for example when the
options themselves are unclear. An option the user prefers is weighed like any other; do not present it as the result of
the comparison unless the comparison supports it.

## Before you answer

Check that:

1. The depth matched the task's complexity and risk.
2. The reasoning rests on evidence, not preference.
3. Every unresolved assumption and validation gap carries a label.
