# Glossary format

## Structure

```md
# {Context Name}

{One or two sentence description of what this context is and why it exists.}

## Language

**Order**: {A concise description of the term} _Avoid_: Purchase, transaction

**Invoice**: A request for payment sent to a customer after delivery. _Avoid_: Bill, payment request

**Customer**: A person or organization that places orders. _Avoid_: Client, buyer, account

## Relationships

- An **Order** produces one or more **Invoices**
- An **Invoice** belongs to exactly one **Customer**

## Example dialogue

> **Dev:** "When a **Customer** places an **Order**, do we create the **Invoice** immediately?" **Domain expert:** "No —
> an **Invoice** is only generated once a **Fulfillment** is confirmed."

## Flagged ambiguities

- "account" was used to mean both **Customer** and **User** — resolved: these are distinct concepts.
```

## Rules

- **Be opinionated.** When multiple words exist for the same concept, pick the best one and list the others as aliases
  to avoid.
- **Flag conflicts explicitly.** If people use a term ambiguously, call it out in "Flagged ambiguities" with a clear
  resolution.
- **Keep definitions tight.** One sentence max. Define what it IS, not what it does.
- **Show relationships.** Use bold term names and express cardinality where obvious.
- **Only include terms specific to this project's context.** General programming concepts (timeouts, error types,
  utility patterns) don't belong even if the project uses them extensively.
- **Group terms under subheadings** when natural clusters emerge. If all terms belong to a single cohesive area, a flat
  list is fine.
- **Write an example dialogue** between a dev and a domain expert. It shows how the terms interact in normal use and
  clarifies boundaries between related concepts.

## Single vs multi-context repos

**Single context (most repos):** One root glossary (`GLOSSARY.md` or `CONTEXT.md`).

**Multiple contexts:** A context map (at the root, or at the path a `Context map:` pointer names) lists the contexts,
where they live, and how they relate:

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

Resolve the glossary and context map through [repo-conventions.md](../../routing/references/repo-conventions.md), which
accepts both `GLOSSARY.md` and `CONTEXT.md` naming:

- A resolved context map, at the root or at a `Context map:` pointer's path, means multiple contexts; read it to find
  them
- Only a resolved glossary means a single context
- If the contract resolves neither, create the default root `CONTEXT.md` lazily when the first term is resolved

When multiple contexts exist, infer which one the current topic relates to. If unclear, ask as a ❓ question naming the
candidate contexts. The ➡️ recommends the context whose relationships the topic's main use follows, per
[context-map.md](context-map.md).
