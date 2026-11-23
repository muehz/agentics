# Outcome-Oriented Execution

_Apply during planned rewrites and migrations with explicit phase boundaries._

Optimize for the intended, verifiable end state, not for smooth intermediate states.

Full stability at every intermediate step often creates temporary compatibility code that becomes long-lived debt.
Converge on the target architecture. Prove correctness at explicit verification boundaries.

Core rule:

- Prioritize end-state integrity over transitional stability
- Intermediate breakage is acceptable when it is planned, scoped, and reversible
- Always run final verification before you declare done

Guardrails:

- Use this for planned rewrites and migrations with explicit phase boundaries
- Declare where temporary breakage is acceptable
- Keep high-signal checks for actively touched areas while the migration runs
- Require full static and runtime verification at plan completion

Precedence: [Sequence Work Into Verifiable Units](sequence-verifiable-units.md) holds during a migration. This principle
is its one declared exception. A unit may stay red only when the plan lists it, with the phase boundary at which it must
pass again. Every unit the plan does not list is green before the next one starts.

## Self-test

- Is the target end state written down before the work starts? If no, there is nothing to converge on.
- Does the plan list each intermediate state that may break, with the phase boundary at which it must pass again? Count
  the entries. Zero entries means no breakage is permitted.
- Does the diff contain compatibility code the target state does not need? Mark it for deletion in this same wave.
- Did the full static and runtime verification run at plan completion, not only per phase? Record the result.
