# Deepening

How to deepen a cluster of shallow modules safely, given its dependencies. Vocabulary per [language.md](language.md):
**module**, **interface**, **seam**, and **adapter**.

## Dependency categories

Classify a candidate's dependencies. The category determines how you test the deepened module across its seam.

### 1. In-process

Pure computation, in-memory state, and no I/O. Always deepenable. Merge the modules. Test through the new interface
directly. No adapter is needed.

### 2. Local-substitutable

Dependencies that have local test stand-ins (PGLite for Postgres, in-memory filesystem). Deepenable if the stand-in
exists. Test the deepened module with the stand-in that runs in the test suite. The seam is internal, and the module's
external interface has no port.

### 3. Remote but owned (Ports & Adapters)

Your own services across a network boundary (microservices, internal APIs). The deep module owns the logic. Inject the
transport as an **adapter** at a private seam. Tests use an in-memory adapter. Production uses an HTTP, gRPC, or queue
adapter. The in-memory test adapter keeps the seam private. Only a second production adapter makes it a public **port**
([Seam discipline](#seam-discipline)).

Recommendation shape: _"Inject the transport at the seam. Use an HTTP adapter in production and an in-memory adapter in
tests. The logic sits in one deep module even though it's deployed across a network. Expose a public port only when a
second production adapter exists today."_

### 4. True external (Mock)

Third-party services (Stripe, Twilio, and others) you don't control. The deepened module takes the external dependency
through a private injected seam. Tests provide a mock adapter. The mock never makes that seam a public port.

## Seam discipline

- **One adapter means a hypothetical seam. Two adapters means a real one.** Don't expose a public port unless the second
  adapter is running or committed code that exists today ([language.md](language.md)'s bar). A test double injected at a
  DI seam supports testability. The remote-but-owned and true-external patterns above use such seams. A test double
  never counts as the second adapter in the question of a public port for alternate production implementations. A
  single-production-adapter seam is just indirection.
- **Internal seams vs external seams.** Don't expose internal seams through the interface just because tests use them.
  Internal seams are private to the implementation and used by its own tests.

## Testing strategy: replace, don't layer

- Old unit tests on shallow modules become waste once tests at the deepened module's interface exist. Delete them.
- Write new tests at the deepened module's interface. The **interface is the test surface**.
- Tests assert on observable outcomes through the interface, not internal state.
- Tests should survive internal refactors. If a test has to change when the implementation changes, it tests past the
  interface.

## Recognizing shallow modules

```ts
class UserService {
    updateProfile(userId, flag) { ... }  // shallow: just routes calls
    changePassword(userId, flag) { ... } // shallow: just routes calls
    deactivate(userId, flag) { ... }     // shallow: just routes calls
}
```

Three thin methods orchestrate `UserRepository`, `EmailClient`, and `AuditLogger`: a **shallow module** with a wide
interface and little owned work. Every consumer couples to the orchestration details. Tests must mock all three
collaborators. Tech debt accumulates as flag combinations multiply. The **deep-module** refactor hides the orchestration
and downstream coupling behind one `applyUserAction({userId, action, context})` method with a narrower interface.

## Exploration friction signals

Explore organically, not by rigid heuristics. Note where you feel friction:

- Where must you bounce between many small modules to understand one concept?
- Where are modules **shallow**, with an interface nearly as complex as the implementation?
- Where were pure functions extracted just for testability while the real bugs hide in how they're called (no
  **locality**)?
- Where do tightly-coupled modules leak across their seams?
- Which parts of the codebase are untested, or hard to test through their current interface?

## Grilling examples

- **Glossary vocabulary in candidates:** if the glossary defines "Order," say "the Order intake module," not "the
  FooBarHandler" or "the Order service."
- **Glossary conflict question:** "The glossary defines 'cancellation' as X, but you mean Y. Which is it?" (Recommend
  keeping the existing glossary definition and naming a new term for the new meaning.)
- **Overloaded term question:** "'Account' is used for two concepts—is this the Customer or the User?" Invent concrete
  scenarios that force precision about where one concept ends and the next begins.

## The scrap trigger

A settled design is wrong when implementation repeatedly produces the **same shape of trouble**—a pattern of friction,
not single hard cases. Tells:

- The same shape of workaround, repeated across unrelated callers.
- Multiple unrelated edge cases that all need special-case branches.
- Types that need escape hatches (`any`, casts, optional fields always set in practice) to compile.
- Callers that have to know the module's internal rules to use it.
- Two or more independent implementation deviations of the same shape.

When the trigger fires:

1. Scrap deliberately; add no more point fixes.
2. Re-explore what was built (Step 1).
3. Redesign with the implementation lessons as day-one constraints, not vibes.
4. Subtract before you add: make the new sketch smaller than the old one before it grows.
5. Re-run the design-it-twice comparison in Step 3 and name the scrapped shape and reason in the new settled-design
   summary.
