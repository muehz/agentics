# Approach exploration

Pick between structurally different shapes before the work commits to one. `spec draft` loads this on a genuine fork in
product-shaped work. If `decompose` finds a new solution fork, it routes back to `spec draft` before it writes dependent
tasks. This is not a step of its own. The caller decides when the test below fires and records the outcome where its
artifact says.

## The genuine-fork test

Run this discipline only when all three hold. A fork that fails any of them is a decision already made. An exploration
of it is ceremony.

1. Two or more shapes are live. The conversation, the codebase, or the intent's open questions name at least two ways to
   decompose the work. The ways differ in decomposition, data flow, or ownership. One shape with knobs turned does not
   count.
2. The user has not chosen. A stated decision in `intent.md`'s **Decisions** section, or in the session, closes the
   fork.
3. The choice is expensive to reverse after the next artifact is written. It changes which modules exist, which seam
   carries the data, or who owns a behavior.

When the test fails, do these steps:

1. Say so in one line ("one viable shape; recording the foreclosed alternative").
2. Record the foreclosed alternative with its reason rather than stage a comparison.

## Structurally distinct candidates

Propose two or three candidates with trade-offs. Each must be a whole-shape alternative. It is a different decomposition
into modules, a different seam for the data, or a different owner for the policy. Two variants of the same architecture
are one candidate. Require a second, genuinely different shape even when the first looks sufficient. The first idea is
rarely the best, and it is still free to discard an alternative now.

## Screen before you compare

Screen every candidate against [design-red-flags.md](design-red-flags.md) for a shallow module, information leakage,
temporal decomposition, or a pass-through method. Name the flag when you revise or drop a flagged candidate. Compare the
survivors on depth. Prefer the shape that hides more complexity behind a smaller interface.

## Recommend, then the user picks

Present the candidates as one ❓ question, each with its trade-offs. Add a ➡️ line that names the recommended candidate
and its reasoning. Never collapse a genuine fork into a unilateral decision to move faster. Never resolve an open fork
to the recommendation on the user's silence. Name it as still open. Get the answer. In an
[unattended](../../routing/references/contracts/unattended.md) run, do not resolve the fork yourself. Record the fork
and the recommendation as an open question for the next artifact's owner.

## Record the outcome

The chosen shape goes where the caller's artifact keeps its decisions. The rejected shapes go beside it, one line each
with the reason they lost, because a later reader needs the alternatives to judge the choice:

- `spec draft` writes them as entries under the spec's **Scope Decisions** section, one per rejected approach. Each
  entry begins "Rejected: …".

A recorded alternative is the deliverable. A comparison nobody wrote down did not happen.
