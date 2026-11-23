# Laziness Protocol

_Apply when you refactor, evaluate diff size, or are tempted to add abstractions, layers, or signal threading._

Writing code is cheap for you, which makes over-engineering easy. To counter it, borrow a human maintainer's fatigue.
Aim for the most result with the least code and complexity.

- **Prefer deletion.** When asked to refactor or improve, look for removals before additions.
- **Maintain a flat call hierarchy.** Avoid deep call chains. A rich interface that hides substantial work is not a deep
  call chain. If you must trace through more than 3 files or layers to answer a question, flatten it.
- **Consolidate decisions.** Do not repeat the same choice in several places. Put it behind one source of truth. Pass
  the result as a simple flag.
- **Minimize the diff.** Make the smallest change that solves the problem. Fewer lines beat "elegant" boilerplate.
- **Question the threading.** If a task asks you to pass a new signal through types, schemas, pipelines, or similar
  layers, stop. Look for a more direct path.
- **Sweat the small leaks.** Remove tiny pass-throughs, representation leaks, and duplicated choices before they spread.
  Small leaks compound into permanent coordination costs.

If a human developer would find the code exhausting to maintain, it is a bad solution.

## Self-test

- Does this diff delete more lines than it adds? If no, name what the added lines buy.
- How many files or layers must a reader open to answer "what does this call actually do?" More than three means flatten
  it.
- Does the new abstraction have a second caller today? If no, inline it. The exception is a pure function that
  [Boundary Discipline](boundary-discipline.md) extracts from I/O or framework wiring. That one stays.
- Is the same decision made in more than one place in this diff? Count the sites. More than one needs a single source of
  truth.
