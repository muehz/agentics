# Model the Domain

_Apply when you write stateful logic, or when code branches a lot or repeats a shape assumption across files._

Encode the real domain in a data structure. Do not scatter it across conditionals.

Scattered booleans, repeated shape assumptions, and branching spread across files are accidental complexity. A structure
that matches the domain makes invalid states unrepresentable and deletes branches. It is cheap to choose that structure
at write time, but the work to recover it later reads as a refactor and gets deferred.

Reach for structures like these:

- A state machine instead of scattered booleans, phases, or lifecycle checks.
- A typed object/model instead of loose parameters or repeated shape assumptions.
- A map, registry, lookup table, or discriminated union instead of branches spread across files.
- A reducer or command/event model instead of ad hoc state mutations.
- A module organized around one body of domain knowledge instead of a sequence such as load, validate, transform, and
  save. Execution order is not ownership.
- A small module boundary that gathers repeated behavior, ownership, or invariants.
- A queue, cache, index, graph/tree, or normalized collection where the data access pattern calls for it.
- Any other structure that fits. The list above covers the common cases only. When none fits, work out what the code
  must never allow and how the data gets read. Then find the structure that encodes exactly that.

Do not force an abstraction. If the current shape is already clear, local, and unlikely to grow, prefer boring code. Be
skeptical of an abstraction that adds indirection but does not remove branches, duplicated rules, invalid states, or
lifecycle risk.

Temporal decomposition is another tell that you skipped this. It shows as phase-named modules that repeat the same
domain rules across steps. Those modules sit alongside a growing if/else chain or a second boolean kept in sync with the
first.

## Self-test

- Does this change add one more branch to an existing if/else chain, or a second boolean that must stay in sync with the
  first? Either one is the tell.
- Can I write down a combination of these fields that the code must never produce? If yes, the current structure permits
  an illegal state.
- Count what the proposed structure deletes: branches, duplicated rules, invalid states. If the count is zero, it is
  indirection, not a model.
- Are the modules named for steps: load, validate, transform, save? If yes, check whether the same domain rule repeats
  in each one.
