# Experience First

_Apply when product, UX, or feature-scope trade-offs come up._

The product is the experience. Every technical decision either helps or hurts it. When implementation convenience
conflicts with user delight, choose delight.

- Say no to 1,000 things (every feature, control, and option must earn its place)
- Ship less, ship better (a polished experience with three features beats a rough one with ten)
- Prototype before you commit (design decisions are cheaper in throwaway HTML than production code)
- Sweat the details (transitions, alignment, spacing, feedback, error states)
- Tighten the core loop (every feature should serve the central workflow or get out of the way)

The user is whoever consumes the work. For a UI that is the end user. For a library or an internal API it is the
colleague who imports it. The engineer who maintains the code next is a user too. Weigh their experience the same way.
Explain the impact from their seat.

Foundations should serve the experience, not the other way around. [Foundational Thinking](foundational-thinking.md)
governs the _sequence_ of work. This principle governs the _target_.

## Self-test

- Can I name the user of this change and state in one sentence what they get? If no, the change serves the
  implementation.
- What did this change refuse or remove? Count it. Zero across a whole feature is a warning.
- Does every empty, loading, and error state in this change have a defined behavior? List them.
- Does this feature serve the central workflow, or does it sit beside it? If it sits beside it, say why it ships.
