# Minimize Reader Load

_Apply when you review or shape code that is hard to trace._

Maintainability is the work a reader must do to understand code. Track two axes:

1. **Layers to trace.** How many indirections sit between the question and the answer.
2. **State to hold.** How much hidden or mutable context the reader must keep in their head.

Code is read far more than it is written. LOC, cyclomatic complexity, and "clean architecture" are proxies. Reader load
is what matters. The two axes are independent. A flat file with 50 globals can be as hard to reason about as a 6-layer
adapter stack. Guard both. This is the human analog of [Guard the Context Window](guard-the-context-window.md). Working
memory is finite for readers too.

Pattern:

- **Collapse layers** that do not earn their keep. Examples are wrappers with one caller, adapters with no second
  implementation, and indirection for a future that never came. Inline them.
- **Make adjacent layers change the abstraction.** A layer that repeats the same methods and arguments adds reader load
  without compression. Collapse pass-through layers.
- **Demand interface compression.** A broad interface that hides little complexity makes readers learn both the surface
  and the implementation. Prefer boundaries that hide meaningful decisions.
- **Shrink state scope.** Prefer pure functions (returns over mutations). Prefer locals over fields, fields over module
  state, and module state over globals. Derive instead of sync.
- **Name the invariant at the boundary,** not in every consumer, so the reader learns it once.

## Self-test

- Pick one value or behavior this change touches. Trace it from where it is used to where it originates, and count the
  files you open on the way. Then count the places that can change it. Is the origin more than 3 files away, or are
  there more than 2 places that change it? If yes, cut layers or cut state.
- How many callers does this wrapper or adapter have? One caller means inline it, unless it is a pure function that
  [Boundary Discipline](boundary-discipline.md) extracts from I/O or framework wiring.
- Does the layer above repeat the same method names and arguments as the layer below? A layer that does not change the
  abstraction is a pass-through. Collapse it.
- For each layer or piece of mutable state this change adds: does it remove a layer to trace or state to hold somewhere
  else, or hide a decision the reader then need not learn? Count the additions that do neither. More than zero means do
  not add them.
