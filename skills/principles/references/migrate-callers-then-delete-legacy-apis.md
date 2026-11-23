# Migrate Callers Then Delete Legacy APIs

_Apply when you introduce a new internal API while old callers still exist._

When we decide a new API is the right design, do these steps in the same refactor wave:

1. Migrate the callers.
2. Remove the old API.

Do this in place of compatibility layers.

Rule:

- Do not keep legacy API paths alive only because internal callers still exist
- Inventory the callers. Migrate them. Delete the old API immediately
- Treat temporary adapters as exceptional and time-boxed, not default architecture
- Update tests to assert the new contract. Delete tests that only protect pre-refactor implementation details

When this applies:

- No external users depend on backward compatibility
- The project can absorb coordinated breaking changes
- The new API is part of a simplification or refactor initiative

If you keep both the old and the new API, you create dual-path complexity, slow cleanup, and make the codebase feel
append-only.

## Self-test

- How many callers of the old API remain after this change? Grep for it. Anything above zero means the wave is
  unfinished.
- Is the old API still in the tree? If yes, name the external consumer that needs it. Name the date it goes.
- Do the tests assert the new contract, or do they still pin the old implementation? Delete the ones that only protect
  the old shape.
- Does any adapter or shim in this diff exist only to spare internal callers? If yes, migrate them instead.
