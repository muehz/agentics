# Boundary Discipline

_Apply when you wire validation, error handling, or framework adapters._

Place validation, type narrowing, and error handling at system boundaries. Trust internal code unconditionally. Business
logic lives in pure functions. The shell is thin and mechanical.

Scattered validation is noisy and redundant, and it gives a false sense of safety. Boundary validation keeps logic out
of framework wiring, so you can test the logic without the framework.

Use this split:

- **At boundaries** (CLI args, config files, external APIs, network protocols): Validate the data. Return errors. Handle
  failures defensively.
- **Inside the system:** Use typed data and error propagation. Do not re-validate. Trust the types.
- **Across the boundary.** Expose domain concepts, not the boundary's private representation. Keep general-purpose
  mechanism inside and special-purpose policy at the edge.

Validation and error handling:

- Validate config at parse time (the boundary), not inside business logic
- Parse raw data into domain types at the boundary
- Do not re-export transport, storage, framework, or wire types through the public surface
- If the boundary already validated the data, do not add redundant nil checks deep in call chains

Code organization:

- Business logic in pure functions with no framework dependencies
- Parse functions: pure transforms from raw bytes to typed state
- Prompt construction: structured state in, string out
- Scoring and assessment: pure transforms from state to results

Precedence:

- **Extraction against inlining.** The [Laziness Protocol](laziness-protocol.md) and
  [Minimize Reader Load](minimize-reader-load.md) inline an abstraction that has one caller. This principle wins when
  the logic is tangled with I/O or framework wiring, because the extraction is what lets you test the logic without the
  framework. The inline rule covers wrappers and adapters that add no logic.
- **Internal validation.** Trust for internal code is the default. The one exception follows a diagnosed bug in which
  invalid data crossed several layers. `/agentics:diagnose` may then add a check at each layer that the data passed
  through.

## Self-test

- Does this data cross a system boundary right now? If no, did a diagnosed bug show invalid data crossing this layer? If
  no to both, the validation is redundant. Delete it.
- Is this logic tangled with I/O or framework wiring, and can it be a pure function that the shell just calls? If yes,
  extract it, even with one caller.
- Does the public surface expose a transport, storage, framework, or wire type? If yes, map it to a domain type at the
  edge.
