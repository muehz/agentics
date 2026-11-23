# Type System Discipline

_Apply when you design types, review a function signature, or write code in any statically-typed language._

The type checker is a proof assistant. Use it to eliminate impossible states, mismatched primitives, and unhandled
variants at compile time. A case the types let you ignore becomes a runtime failure the compiler could have stopped.
Prefer to define errors and special cases out of existence instead of multiplying handlers. Unrepresentable states,
total functions, and interface redesign (the patterns below) are the tools.

This applies to any typed language, and a project's own language style guide grounds it in specific syntax.

Patterns:

- **Make illegal states unrepresentable.** Model variants as sum types. They are discriminated unions in TypeScript,
  enums with payloads in Rust/Swift/Kotlin, sealed classes in Scala, and ADTs in Haskell/OCaml. Do not model state as a
  bag of optional fields where contradictory combinations compile. Here is a subtle anti-pattern:
  `{ completed: boolean; completedAt?: Date }` admits `completed: true; completedAt: undefined`, which is meaningless.
  Derive the boolean from a single source like `completedAt !== null`. Alternatively, model the variants explicitly as
  `{ kind: 'open' } | { kind: 'done'; at: Date }`. If a bug forces the question "wait, can this combination actually
  happen?", the type is too loose.
- **Types are constructions, not restrictions.** Build the type up from the values you want. Do not carve them out of a
  looser type with checks. The invariant that seems to need a refinement type is usually a construction away. A
  non-empty list is a head plus a rest, not a list with a length check. A valid time range is a start plus a duration,
  not two timestamps you must keep ordered. No representation is privileged. A list of pairs is an even-length list if
  you interpret it that way. Choose the shape that cannot build the illegal value. Then expose the interface callers
  need on top.
- **Brand semantic primitives.** `UserId` and `OrderId` are strings underneath but should not be interchangeable. The
  idioms are newtypes in Rust, opaque types in Swift, value classes in Kotlin, phantom types in Haskell, and branded
  intersections in TypeScript. Validate once at creation. Then trust the type downstream.
- **External data is untyped until parsed.** Examples include RPC payloads, JSON, IPC messages, CLI args, config files,
  environment variables, and database rows. Have a parse function at every boundary that turns unstructured input into
  the typed model. See [Boundary Discipline](boundary-discipline.md) for where to put validation.
- **Don't lie to the type system.** Casts, unsafe coercions, and assertion functions that bypass the compiler are latent
  runtime crashes. If the compiler cannot prove a fact, prove it (validate, narrow, or refine the model). Otherwise,
  accept that the cast is a hazard. The cast you bury today is the postmortem you write next week.
- **Exhaustive matching is the compiler's job.** When you match on a sum type, the compiler must fail compilation when a
  new variant has no handler. Use the idiom your language provides. Examples are a `never`-typed binding in TypeScript,
  an unannotated `match` in Rust, `-Wincomplete-patterns` in Haskell, and sealed-class match exhaustiveness in Kotlin.
- **Derive types from authoritative schemas.** Authoritative schemas include protocol buffers, OpenAPI specs, GraphQL
  schemas, database migrations, and design-system token files. When one of them defines a shape, derive from it. Do not
  hand-roll a parallel type. Manual duplication drifts. See
  [Encode Lessons in Structure](encode-lessons-in-structure.md).
- **Strengthen a type only where partiality appears.** A runtime assertion, null check, or "this should never happen"
  throw marks the place a type is too weak. Push that check up into the type. Then stop. The type system's job is to
  track the cases each use site must handle, not to describe the data as precisely as possible. Prefer total functions.
  `sum` of an empty list is 0, so it takes the plain list. `head` of an empty list has no answer, so it demands the
  non-empty one. Extra precision costs reuse and ceremony and buys no safety.

## Self-test

- Can I write a comment that explains when this combination of fields is valid? If yes, the type is too loose. Split it
  into a sum type.
- Do two of my function arguments share a primitive type but mean different things? Brand them.
- How many `any`s, casts, and non-null assertions does this diff add? For each one, trace it to the boundary where the
  value enters and validate there instead. The target is zero.
- If a new variant is added next month, will the compiler tell the next agent where to add a case? If no, the match is
  not exhaustive.
- Does this type duplicate a shape that another file owns? Derive the type from that shape instead.
- Would an operation panic or throw without this stronger type? If no, keep the plain type.
