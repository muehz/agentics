# Exhaust the Design Space

_Apply when you face a novel UI interaction or architectural decision with no precedent in the codebase._

When a novel interaction or architectural decision has no established precedent, build 2 to 3 competing prototypes or
sketches. Compare them before you commit. Building the wrong thing costs more than an exploration of three options.
Design it twice is this rule by another name. A second flavor of the first shape does not count.

When it applies:

- Novel UI interactions (no prior art in the codebase)
- Architectural choices with multiple viable approaches
- Product design decisions where user experience depends on feel, not logic

When it does not:

- Mechanical implementation where the pattern is established
- Bug fixes or refactors with a clear target state
- Changes where constraints dictate a single viable approach

In Agentics, `/agentics:prototype` is the natural vehicle for the competing sketches. The interface-design pass of
`/agentics:deepen` carries the same design-it-twice discipline at module level.

## Self-test

- Does the codebase already contain a pattern that solves this? If yes, follow it. This principle does not apply.
- How many candidate designs did I build or sketch before I chose? Fewer than two means the space was not explored.
- Do the candidates differ in structure, or only in naming and layout? Two flavors of one shape count as one candidate.
- Can I name what each rejected candidate did better than the one I picked? If no, the comparison was not real.
