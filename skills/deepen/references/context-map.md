# Context map format

Most repos have a **single context**: one root glossary (`GLOSSARY.md` or `CONTEXT.md`) holds the whole ubiquitous
language. Reach for a context map only when the codebase genuinely splits into separate domains that each own their own
vocabulary.

## When a map applies

Resolve the files through [repo-conventions.md](../../routing/references/repo-conventions.md). It owns precedence, both
naming schemes, and conflicts. The result has three cases:

- A resolved map (root `GLOSSARY-MAP.md` or `CONTEXT-MAP.md`, or the path a `Context map:` pointer names) means
  **multiple contexts**. Read the map to find them.
- Only a resolved glossary means a **single context**. It needs no map.
- Neither means a single context by default. Create a root `CONTEXT.md` lazily when the first term is resolved.

Don't introduce a map preemptively. One clean glossary is the preferred case. Add a map once a term means different
things in different parts of the system. That signals more than one bounded context. An example is an "Order" in
Ordering vs. an "Order" line item in Fulfillment.

## Structure

A context map lists the contexts, where each one lives, and how they relate:

```md
# Context Map

## Contexts

- [Ordering](./src/ordering/CONTEXT.md), which receives and tracks customer orders
- [Billing](./src/billing/CONTEXT.md), which generates invoices and processes payments
- [Fulfillment](./src/fulfillment/CONTEXT.md), which manages warehouse picking and shipping

## Relationships

- **Ordering → Fulfillment**: Ordering emits `OrderPlaced` events; Fulfillment consumes them to start picking
- **Fulfillment → Billing**: Fulfillment emits `ShipmentDispatched` events; Billing consumes them to generate invoices
- **Ordering ↔ Billing**: Shared types for `CustomerId` and `Money`
```

## Rules

- **One entry per context.** Each bullet links to that context's own glossary. It gives a one-line description of what
  the context is responsible for.
- **Name the relationships and their direction.** Use `→` for a one-way dependency. Examples are events that flow, and
  one context that consumes another's output. Use `↔` for a genuinely shared concern. Examples are shared types and a
  two-way contract. State _what_ crosses the boundary: the event, shared type, or call, not just that a line exists.
- **Keep it a map, not a spec.** The detailed vocabulary lives in each context's glossary. The decisions live in ADRs.
  Don't inline glossary terms or implementation detail here.
- **Each context keeps its own glossary and ADRs.** A multi-context repo has a glossary per context (see
  [`context-format.md`](context-format.md) for the format) plus context-specific ADR directories. System-wide decisions
  go in the root ADR directory that [repo-conventions.md](../../routing/references/repo-conventions.md) resolves.

## Routing a topic to a context

When multiple contexts exist, infer which one the current topic relates to before you touch a glossary. If you resolve a
term in the wrong context, it pollutes the wrong language. Use the map's descriptions and relationships to place the
topic. If the topic is genuinely ambiguous or straddles a boundary, **ask** which context owns it, rather than guess. An
example of a straddling topic is a concept that both Ordering and Billing lay claim to. Pose that ask as a ❓ question.
The ➡️ recommends a basis, not a pick: the context whose relationships the topic's main use follows. A straddling
concept often signals that the boundary itself needs a decision, which may be worth an ADR.
