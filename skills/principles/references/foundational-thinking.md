# Foundational Thinking

_Apply before you write logic. Use it when you choose core types and data structures, sequence scaffold-vs-feature work,
or ask what concurrent actors share._

Structural decisions protect option value. Code-level decisions protect simplicity. Over-engineering is often a
premature decision that closes doors. The right foundational data structure keeps doors open.

Get the data shape right before you write logic. The right shape makes downstream code obvious. Define core types early.
Trace every access pattern. Choose structures that match the dominant paths. A data-structure change is a rewrite when
it comes late. When it comes early, it is often a one-line diff.

At code level, DRY the structure, not every line. Types and data models should converge. Three similar statements still
beat a premature abstraction. Prefer explicit over clever. Test behavior and edge cases, not line counts.

Before you share state between actors, ask "what happens if another actor modifies this concurrently?" If the answer is
not "nothing," isolate.

Ask "does every subsequent phase benefit from this existing?" CI, linting, test infrastructure, and shared types are
scaffold and come first. Sequence for option value: setup before features, tests before fixes. Keep commits small and
single-purpose.

Each increment should land a coherent abstraction or deepen one that exists. Do not spread a new capability across
callers as special-case coordination.

[Subtraction](subtract-before-you-add.md) comes before you scaffold: remove dead weight first, then lay foundations.

## Self-test

- Are the core types defined before the logic that consumes them? If the logic came first, expect a rewrite.
- Count the lines and files a later change to this data structure would touch. More than a one-line diff means decide it
  now.
- Does every later phase of this work benefit from this piece existing? If yes, it is scaffold. Do it first.
- Does a concurrent write by another actor change what this code reads or writes? If yes, isolate it.
