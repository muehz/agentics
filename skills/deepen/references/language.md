# Language

Shared vocabulary for every suggestion this skill makes. Use these terms exactly. Don't substitute "component,"
"service," "API," or "boundary."

## Terms

**Module** Anything with an interface and an implementation. Deliberately scale-agnostic. It applies equally to a
function, class, package, or tier-spanning slice. _Avoid_: unit, component, service.

**Interface** Everything a caller must know to use the module correctly. Includes the type signature, but also
invariants, ordering constraints, error modes, required configuration, and performance characteristics. _Avoid_: API,
signature (too narrow, because both refer only to the type-level part of the interface).

**Implementation** What's inside a module: its body of code. It is distinct from **Adapter**. A thing can be a small
adapter with a large implementation (a Postgres repo) or a large adapter with a small implementation (an in-memory
fake). Reach for "adapter" when the seam is the topic. Use "implementation" otherwise.

**Depth** The amount of behaviour a caller (or test) can exercise per unit of interface they have to learn. A module is
**deep** when a large amount of behaviour sits behind a small interface. A module is **shallow** when the interface is
nearly as complex as the implementation.

**Seam** _(from Michael Feathers)_ A place where you can alter behaviour without an edit in that place. The _location_
at which a module's interface lives. The choice of where to put the seam is its own design decision, distinct from what
goes behind it. _Avoid_: boundary (overloaded with DDD's bounded context).

**Adapter** A concrete thing that satisfies an interface at a seam. Describes _role_ (what slot it fills), not substance
(what's inside).

**Leverage** What callers get from depth. More capability per unit of interface they have to learn. One implementation
pays back across N call sites and M tests.

**Locality** What maintainers get from depth. Change, bugs, knowledge, and verification concentrate at one place rather
than spread across callers. Fix once, fixed everywhere.

## Principles

- **Depth is a property of the interface, not the implementation.** A deep module can be internally composed of small,
  mockable, swappable parts. Those parts just aren't part of the interface. A module can have **internal seams**
  (private to its implementation, used by its own tests) as well as the **external seam** at its interface.
- **The deletion test.** Imagine that you delete the module. If complexity vanishes, the module hid nothing (it was a
  pass-through). If complexity reappears across N callers, the module hid real complexity. At design time, apply it
  _forward_. If you can't name the complexity that the module you are about to write would concentrate, you may not need
  it.
- **The interface is the test surface.** Callers and tests cross the same seam. If you want to test _past_ the
  interface, the module is probably the wrong shape. Redesign before you write it.
- **One adapter means a hypothetical seam. Two adapters means a real one.** Don't add the port until the second adapter
  is **running or committed code that exists today**. Planned, roadmapped, or promised adapters are hypothetical, as are
  stubs, placeholders, or TODO-shaped interfaces that reserve the seam. If someone reframes a forecasted need as "the
  second adapter," name it. A speculative port is an abstraction you pay for on every read and may never use. Add the
  port when the second real adapter lands, when it is cheap because the module is already deep. This is a recommendation
  about the user's design: state it and its cost once, and when the user still wants the port, design it and record that
  they chose it against the recommendation.
- **When you design an interface, ask whether you can do any of these:**
    - Reduce the number of methods
    - Simplify the parameters
    - Hide more complexity inside

## Designing for testability

Decide these while the interface is still on paper:

1. **Accept dependencies, don't create them.**

    ```typescript
    // Testable
    function processOrder(order, paymentGateway) {}

    // Hard to test
    function processOrder(order) {
        const gateway = new StripeGateway();
    }
    ```

2. **Return results, don't produce side effects.**

    ```typescript
    // Testable
    function calculateDiscount(cart): Discount {}

    // Hard to test
    function applyDiscount(cart): void {
        cart.total -= discount;
    }
    ```

3. **Small interface.** Fewer methods = fewer tests needed. Fewer params = simpler test setup.

## Relationships

- A **Module** has exactly one **Interface** (what it presents to callers and tests).
- **Depth** is a property of a **Module**, measured against its **Interface**.
- A **Seam** is where a **Module**'s **Interface** lives.
- An **Adapter** sits at a **Seam** and satisfies the **Interface**.
- **Depth** produces **Leverage** for callers and **Locality** for maintainers.

## Rejected framings

- **Depth as ratio of implementation-lines to interface-lines** (Ousterhout): This rewards a padded implementation. We
  use depth-as-leverage instead.
- **"Interface" as the TypeScript `interface` keyword or a class's public methods**: This is too narrow. Interface here
  includes every fact a caller must know.
- **"Boundary"**: overloaded with DDD's bounded context. Say **seam** or **interface**.
